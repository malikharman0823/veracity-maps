import sys
import os
import re
import math
import json
import pymupdf

os.environ.setdefault("OPENCV_LOG_LEVEL", "OFF")

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if SCRIPT_DIR not in sys.path:
    sys.path.insert(0, SCRIPT_DIR)


def classify_engineering_label(text):
    t = (text or "").strip()
    if not t:
        return "general"
    upper = t.upper()
    if re.search(r"\b(EL\.?|ELEV\.?|RL|IL|GL|FFL|SSL|TOC|INV\.?|BM|DATUM)\b", upper) or re.search(r"[+-]\d+\.\d{2,3}\s*M?\b", upper):
        return "elevation_level"
    if re.search(r"\b(PIPE|CULVERT|PVC|HDPE|RCP|DN\s*\d+|DIA\.?|Ø\s*\d+|\d+\s*MM\s*Ø)\b", upper):
        return "pipe_culvert"
    if re.search(r"\b(ROAD|RD\.?|STREET|ST\.?|AVE\.?|AVENUE|HIGHWAY|HWY|LANE|LN\.?|BLVD|ROW|CARRIAGEWAY)\b", upper):
        return "road_label"
    if re.search(r"^\d+(\.\d+)?\s*(MM|CM|M|KM|FT|IN|'|\"|°)?$", upper) or re.search(r"\d+\s*[xX×]\s*\d+", upper):
        return "dimension"
    return "general"


def rgb_tuple_to_hex(rgb, default="#1e293b"):
    if not rgb or not isinstance(rgb, (list, tuple)) or len(rgb) < 3:
        return default
    try:
        r = max(0, min(255, int(round(float(rgb[0]) * 255.0))))
        g = max(0, min(255, int(round(float(rgb[1]) * 255.0))))
        b = max(0, min(255, int(round(float(rgb[2]) * 255.0))))
        return f"#{r:02x}{g:02x}{b:02x}"
    except Exception:
        return default


def int_color_to_hex(c, default="#0f172a"):
    try:
        val = int(c) & 0xFFFFFF
        return f"#{val:06x}"
    except Exception:
        return default


def _analyze_page_structure(doc, page):
    rect = page.rect
    w_pt = max(1.0, float(rect.width))
    h_pt = max(1.0, float(rect.height))

    has_geo = False
    try:
        obj_str = doc.xref_object(page.xref)
        if any(k in obj_str for k in ("/LGIDict", "/VP", "/Measure", "/GPTS")):
            has_geo = True
    except Exception:
        pass

    try:
        drawings = page.get_drawings()
    except Exception:
        drawings = []
    vector_path_count = len(drawings)

    text_span_count = 0
    try:
        tdict = page.get_text("dict")
        for blk in tdict.get("blocks", []):
            if blk.get("type") == 0:
                for line in blk.get("lines", []):
                    for sp in line.get("spans", []):
                        if (sp.get("text") or "").strip():
                            text_span_count += 1
    except Exception:
        tdict = {"blocks": []}

    try:
        images = page.get_images(full=True)
    except Exception:
        images = []
    image_count = len(images)

    estimated_dpi = 0
    if images:
        max_px = 0
        for img in images:
            try:
                iw = int(img[2] or 0)
                ih = int(img[3] or 0)
                max_px = max(max_px, iw, ih)
            except Exception:
                continue
        max_inches = max(w_pt, h_pt) / 72.0
        if max_px > 0 and max_inches > 0:
            estimated_dpi = int(round(max_px / max_inches))

    fonts = []
    try:
        font_set = set()
        for f in page.get_fonts():
            if len(f) > 3 and f[3]:
                font_set.add(str(f[3]))
        fonts = sorted(font_set)
    except Exception:
        fonts = []

    has_vector = vector_path_count > 0
    has_text = text_span_count > 0
    has_raster = image_count > 0 or (not has_vector and not has_text)

    if (has_vector or has_text) and image_count == 0:
        processing_path = "vector"
        classification = "vector_pdf"
    elif (vector_path_count > 5 or has_text) and image_count > 0:
        processing_path = "hybrid"
        classification = "hybrid_pdf"
    else:
        processing_path = "raster"
        classification = "scanned_document"

    # Compute adaptive DPI preserving original aspect ratio and avoiding OOM
    base_dpi = 240 if processing_path in ("vector", "hybrid") else 260
    if estimated_dpi > 0 and processing_path == "raster":
        base_dpi = max(200, min(300, estimated_dpi))

    max_inches = max(w_pt, h_pt) / 72.0
    max_allowed_dpi = int(4096.0 / max_inches) if max_inches > 0 else 300
    recommended_dpi = max(144, min(base_dpi, max(144, max_allowed_dpi)))

    ocr_recommended = (text_span_count == 0)

    return {
        "w_pt": w_pt,
        "h_pt": h_pt,
        "has_geo": has_geo,
        "drawings": drawings,
        "text_dict": tdict,
        "vector_path_count": vector_path_count,
        "text_span_count": text_span_count,
        "image_count": image_count,
        "fonts": fonts,
        "estimated_dpi": estimated_dpi,
        "recommended_dpi": recommended_dpi,
        "processing_path": processing_path,
        "ocr_recommended": ocr_recommended,
        "content": {
            "has_raster": has_raster,
            "has_vector": has_vector,
            "has_embedded_text": has_text,
            "classification": classification,
        },
    }


def _sample_cubic_bezier(p0, p1, p2, p3, steps=6):
    pts = []
    for s in range(steps + 1):
        t = s / float(steps)
        mt = 1.0 - t
        x = (mt ** 3) * p0.x + 3 * (mt ** 2) * t * p1.x + 3 * mt * (t ** 2) * p2.x + (t ** 3) * p3.x
        y = (mt ** 3) * p0.y + 3 * (mt ** 2) * t * p1.y + 3 * mt * (t ** 2) * p2.y + (t ** 3) * p3.y
        pts.append((x, y))
    return pts


def _extract_page_vectors_and_text(page, analysis, pix_w, pix_h):
    rect = page.rect
    x0_pt, y0_pt = float(rect.x0), float(rect.y0)
    sx = float(pix_w) / max(1.0, float(rect.width))
    sy = float(pix_h) / max(1.0, float(rect.height))

    def to_px(pt_x, pt_y):
        return [round((float(pt_x) - x0_pt) * sx, 2), round((float(pt_y) - y0_pt) * sy, 2)]

    features = []
    max_features = 8000
    for d in analysis["drawings"]:
        if len(features) >= max_features:
            break
        b = d.get("rect")
        if b is not None:
            diag_px = math.hypot(float(b.width) * sx, float(b.height) * sy)
            if diag_px < 1.2:
                continue

        stroke_hex = rgb_tuple_to_hex(d.get("color"), "#334155")
        fill_hex = rgb_tuple_to_hex(d.get("fill"), "") if d.get("fill") else None
        width_px = max(0.75, round(float(d.get("width") or 1.0) * ((sx + sy) * 0.5), 2))
        layer_name = str(d.get("layer") or "pdf_vector")

        current_line = []
        for item in d.get("items", []):
            op = item[0]
            if op == "l":
                p1, p2 = item[1], item[2]
                c1 = to_px(p1.x, p1.y)
                c2 = to_px(p2.x, p2.y)
                if not current_line:
                    current_line = [c1, c2]
                elif current_line[-1] == c1:
                    current_line.append(c2)
                else:
                    if len(current_line) >= 2:
                        features.append({
                            "geometry_type": "LineString",
                            "coords_px": current_line,
                            "stroke_color": stroke_hex,
                            "fill_color": None,
                            "line_width": width_px,
                            "layer": layer_name,
                        })
                    current_line = [c1, c2]
            elif op == "re":
                if len(current_line) >= 2:
                    features.append({
                        "geometry_type": "LineString",
                        "coords_px": current_line,
                        "stroke_color": stroke_hex,
                        "fill_color": None,
                        "line_width": width_px,
                        "layer": layer_name,
                    })
                    current_line = []
                r = item[1]
                ring = [
                    to_px(r.x0, r.y0),
                    to_px(r.x1, r.y0),
                    to_px(r.x1, r.y1),
                    to_px(r.x0, r.y1),
                    to_px(r.x0, r.y0),
                ]
                features.append({
                    "geometry_type": "Polygon" if fill_hex else "LineString",
                    "coords_px": ring,
                    "stroke_color": stroke_hex,
                    "fill_color": fill_hex,
                    "line_width": width_px,
                    "layer": layer_name,
                })
            elif op == "qu":
                q = item[1]
                ring = [
                    to_px(q.ul.x, q.ul.y),
                    to_px(q.ur.x, q.ur.y),
                    to_px(q.lr.x, q.lr.y),
                    to_px(q.ll.x, q.ll.y),
                    to_px(q.ul.x, q.ul.y),
                ]
                features.append({
                    "geometry_type": "Polygon" if fill_hex else "LineString",
                    "coords_px": ring,
                    "stroke_color": stroke_hex,
                    "fill_color": fill_hex,
                    "line_width": width_px,
                    "layer": layer_name,
                })
            elif op == "c":
                pts = _sample_cubic_bezier(item[1], item[2], item[3], item[4], steps=6)
                px_pts = [to_px(px, py) for px, py in pts]
                if not current_line:
                    current_line = px_pts
                elif current_line[-1] == px_pts[0]:
                    current_line.extend(px_pts[1:])
                else:
                    if len(current_line) >= 2:
                        features.append({
                            "geometry_type": "LineString",
                            "coords_px": current_line,
                            "stroke_color": stroke_hex,
                            "fill_color": None,
                            "line_width": width_px,
                            "layer": layer_name,
                        })
                    current_line = px_pts

            if len(features) >= max_features:
                break

        if len(current_line) >= 2 and len(features) < max_features:
            is_closed = fill_hex is not None and (current_line[0] == current_line[-1] or d.get("closePath"))
            if is_closed and current_line[0] != current_line[-1]:
                current_line.append(current_line[0])
            features.append({
                "geometry_type": "Polygon" if is_closed else "LineString",
                "coords_px": current_line,
                "stroke_color": stroke_hex,
                "fill_color": fill_hex if is_closed else None,
                "line_width": width_px,
                "layer": layer_name,
            })

    annotations = []
    tdict = analysis["text_dict"]
    for blk in tdict.get("blocks", []):
        if blk.get("type") != 0:
            continue
        for line in blk.get("lines", []):
            ldir = line.get("dir") or (1.0, 0.0)
            angle_deg = round(math.degrees(math.atan2(float(ldir[1]), float(ldir[0]))), 2)
            for sp in line.get("spans", []):
                txt = (sp.get("text") or "").strip()
                if not txt:
                    continue
                sb = sp.get("bbox") or [0, 0, 0, 0]
                p_min = to_px(sb[0], sb[1])
                p_max = to_px(sb[2], sb[3])
                bx0 = min(p_min[0], p_max[0])
                by0 = min(p_min[1], p_max[1])
                bx1 = max(p_min[0], p_max[0])
                by1 = max(p_min[1], p_max[1])
                cx = round((bx0 + bx1) * 0.5, 2)
                cy = round((by0 + by1) * 0.5, 2)
                font_size_pt = round(float(sp.get("size") or 10.0), 2)
                annotations.append({
                    "text": txt,
                    "original_text": txt,
                    "category": classify_engineering_label(txt),
                    "source": "pdf_vector",
                    "confidence": 1.0,
                    "image_x": cx,
                    "image_y": cy,
                    "bbox_px": [bx0, by0, bx1, by1],
                    "rotation": angle_deg,
                    "font_name": str(sp.get("font") or ""),
                    "font_size": font_size_pt,
                    "text_color": int_color_to_hex(sp.get("color", 0)),
                    "verified": True,
                    "visible": True,
                })

    return {
        "coordinate_space": "image_pixels",
        "width": pix_w,
        "height": pix_h,
        "page_width_pt": round(float(rect.width), 3),
        "page_height_pt": round(float(rect.height), 3),
        "feature_count": len(features),
        "annotation_count": len(annotations),
        "features": features,
        "annotations": annotations,
    }


def inspect_pdf(pdf_path, thumb_dir):
    try:
        doc = pymupdf.open(pdf_path)
    except Exception as e:
        return {"success": False, "error": f"Failed to open PDF: {str(e)}", "code": "CORRUPTED_PDF"}

    if doc.is_encrypted:
        doc.close()
        return {"success": False, "error": "Password-protected or encrypted PDF is not supported", "code": "ENCRYPTED_PDF"}

    page_count = len(doc)
    if page_count == 0:
        doc.close()
        return {"success": False, "error": "PDF has no pages", "code": "EMPTY_PDF"}

    os.makedirs(thumb_dir, exist_ok=True)
    pages = []

    for i in range(page_count):
        page = doc[i]
        rotation = int(page.rotation)
        analysis = _analyze_page_structure(doc, page)

        # Generate clear thumbnail preserving exact page aspect ratio
        thumb_dpi = 96 if max(analysis["w_pt"], analysis["h_pt"]) <= 1200 else 72
        pix = page.get_pixmap(dpi=thumb_dpi)
        thumb_filename = f"thumb_page_{i+1}.png"
        thumb_path = os.path.join(thumb_dir, thumb_filename)
        pix.save(thumb_path)

        pages.append({
            "page_number": i + 1,
            "width": int(round(analysis["w_pt"])),
            "height": int(round(analysis["h_pt"])),
            "width_pt": round(analysis["w_pt"], 2),
            "height_pt": round(analysis["h_pt"], 2),
            "rotation": rotation,
            "has_georeference": analysis["has_geo"],
            "thumbnail_filename": thumb_filename,
            "processing_path": analysis["processing_path"],
            "vector_path_count": analysis["vector_path_count"],
            "text_span_count": analysis["text_span_count"],
            "image_count": analysis["image_count"],
            "fonts": analysis["fonts"],
            "estimated_dpi": analysis["estimated_dpi"],
            "recommended_dpi": analysis["recommended_dpi"],
            "ocr_recommended": analysis["ocr_recommended"],
            "content": analysis["content"],
        })

    doc.close()
    return {
        "success": True,
        "page_count": page_count,
        "pages": pages,
    }


def render_pages(pdf_path, selected_pages, out_dir, dpi=0, enhance_preset="", extract_vectors=True):
    try:
        doc = pymupdf.open(pdf_path)
    except Exception as e:
        return {"success": False, "error": f"Failed to open PDF: {str(e)}", "code": "CORRUPTED_PDF"}

    if doc.is_encrypted:
        doc.close()
        return {"success": False, "error": "PDF is encrypted", "code": "ENCRYPTED_PDF"}

    os.makedirs(out_dir, exist_ok=True)
    rendered = []

    raster_tool = None
    try:
        import raster_tool as rt
        raster_tool = rt
    except Exception:
        raster_tool = None

    for page_num in selected_pages:
        idx = page_num - 1
        if idx < 0 or idx >= len(doc):
            continue

        page = doc[idx]
        analysis = _analyze_page_structure(doc, page)
        actual_dpi = int(dpi) if int(dpi) > 0 else analysis["recommended_dpi"]
        # Safety cap so gigantic architectural sheets stay <= 4200px max side
        max_inches = max(analysis["w_pt"], analysis["h_pt"]) / 72.0
        if max_inches > 0 and max_inches * actual_dpi > 4200:
            actual_dpi = max(120, int(4200.0 / max_inches))

        pix = page.get_pixmap(dpi=actual_dpi)
        filename = f"page_{page_num}.png"
        out_path = os.path.join(out_dir, filename)
        pix.save(out_path)

        vectors_path = ""
        vector_count = 0
        annotation_count = 0
        if extract_vectors and analysis["processing_path"] in ("vector", "hybrid"):
            vec_payload = _extract_page_vectors_and_text(page, analysis, pix.width, pix.height)
            vector_count = vec_payload["feature_count"]
            annotation_count = vec_payload["annotation_count"]
            if vector_count > 0 or annotation_count > 0:
                vectors_path = os.path.join(out_dir, f"page_{page_num}_vectors.json")
                with open(vectors_path, "w", encoding="utf-8") as vf:
                    json.dump(vec_payload, vf)

        quality = None
        recommendation = None
        enhanced_path = ""
        enhancement_info = None

        if raster_tool is not None:
            insp = raster_tool.inspect_raster(out_path)
            if insp.get("success"):
                quality = insp.get("quality")
                recommendation = insp.get("recommendation")

            chosen_preset = enhance_preset
            if not chosen_preset and analysis["processing_path"] == "raster":
                chosen_preset = "text_clarity"

            if chosen_preset and chosen_preset != "none":
                candidate_enh = os.path.join(out_dir, f"page_{page_num}_enhanced.png")
                enh_res = raster_tool.enhance_raster(out_path, candidate_enh, {"preset": chosen_preset})
                if enh_res.get("success") and os.path.exists(candidate_enh):
                    enhanced_path = candidate_enh
                    enhancement_info = enh_res

        rendered.append({
            "page_number": page_num,
            "filename": filename,
            "path": out_path,
            "enhanced_path": enhanced_path,
            "enhancement": enhancement_info,
            "vectors_path": vectors_path,
            "vector_feature_count": vector_count,
            "annotation_count": annotation_count,
            "width": pix.width,
            "height": pix.height,
            "width_pt": round(analysis["w_pt"], 2),
            "height_pt": round(analysis["h_pt"], 2),
            "dpi": actual_dpi,
            "rotation": int(page.rotation),
            "has_georeference": analysis["has_geo"],
            "processing_path": analysis["processing_path"],
            "fonts": analysis["fonts"],
            "ocr_recommended": analysis["ocr_recommended"],
            "content": analysis["content"],
            "quality": quality,
            "recommendation": recommendation,
        })

    doc.close()
    return {
        "success": True,
        "rendered": rendered,
    }


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "error": "Missing command arguments"}))
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "inspect":
        pdf_path = sys.argv[2]
        thumb_dir = sys.argv[3]
        res = inspect_pdf(pdf_path, thumb_dir)
        print(json.dumps(res))
    elif cmd == "render":
        pdf_path = sys.argv[2]
        pages_arg = sys.argv[3]
        out_dir = sys.argv[4]
        dpi = int(sys.argv[5]) if len(sys.argv) > 5 and sys.argv[5].lstrip("-").isdigit() else 0
        enhance_preset = sys.argv[6] if len(sys.argv) > 6 else ""
        extract_vectors = True
        if len(sys.argv) > 7 and sys.argv[7].lower() in ("false", "0", "no"):
            extract_vectors = False
        selected = [int(p.strip()) for p in pages_arg.split(",") if p.strip().isdigit()]
        res = render_pages(pdf_path, selected, out_dir, dpi, enhance_preset, extract_vectors)
        print(json.dumps(res))
    else:
        print(json.dumps({"success": False, "error": f"Unknown command: {cmd}"}))
        sys.exit(1)


if __name__ == "__main__":
    main()
