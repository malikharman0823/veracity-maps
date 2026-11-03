import sys
import os
import re
import math
import json
import ezdxf
from ezdxf import bbox as ezdxf_bbox
from ezdxf.addons.drawing import RenderContext, Frontend
from ezdxf.addons.drawing.matplotlib import MatplotlibBackend
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
from PIL import Image

os.environ.setdefault("OPENCV_LOG_LEVEL", "OFF")

UNITS_MAP = {
    0: "Unspecified",
    1: "Inches",
    2: "Feet",
    3: "Miles",
    4: "Millimeters",
    5: "Centimeters",
    6: "Meters",
    7: "Kilometers",
    8: "Microinches",
    9: "Mils",
    10: "Yards",
    14: "Decimeters",
}

ACI_BASIC_HEX = {
    1: "#ef4444",
    2: "#eab308",
    3: "#22c55e",
    4: "#06b6d4",
    5: "#3b82f6",
    6: "#d946ef",
    7: "#1e293b",
}


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


def resolve_layer_color(doc, layer_name, entity_color=256):
    c = entity_color
    if c in (0, 256) and layer_name in doc.layers:
        try:
            c = abs(int(doc.layers.get(layer_name).color))
        except Exception:
            c = 7
    return ACI_BASIC_HEX.get(abs(int(c or 7)), "#334155")


def iter_flattened_entities(msp, max_depth=2):
    def _walk(entities, depth):
        for ent in entities:
            dxftype = ent.dxftype()
            if dxftype == "INSERT" and depth < max_depth:
                yield ent
                try:
                    yield from _walk(ent.virtual_entities(), depth + 1)
                except Exception:
                    continue
            else:
                yield ent
    yield from _walk(msp, 0)


def sample_arc_points(cx, cy, r, start_deg, end_deg, steps=24):
    if end_deg < start_deg:
        end_deg += 360.0
    span = end_deg - start_deg
    n = max(6, int(math.ceil(steps * (span / 360.0))))
    pts = []
    for i in range(n + 1):
        ang = math.radians(start_deg + span * (i / float(n)))
        pts.append((cx + r * math.cos(ang), cy + r * math.sin(ang)))
    return pts


def convert_dxf_to_png(dxf_path, out_png_path, dpi=220, out_vectors_json=""):
    try:
        doc = ezdxf.readfile(dxf_path)
    except Exception as e:
        return {"success": False, "error": f"Failed to parse DXF: {str(e)}", "code": "CORRUPTED_DXF"}

    msp = doc.modelspace()

    units_code = doc.header.get("$INSUNITS", 0)
    unit_str = UNITS_MAP.get(units_code, "Unspecified")
    dim_scale = float(doc.header.get("$DIMSCALE", 1.0) or 1.0)
    lt_scale = float(doc.header.get("$LTSCALE", 1.0) or 1.0)

    layers = []
    layer_details = []
    for layer in doc.layers:
        lname = layer.dxf.name
        layers.append(lname)
        layer_details.append({
            "name": lname,
            "visible": not layer.is_off() and not layer.is_frozen(),
            "color": resolve_layer_color(doc, lname, 256),
        })

    # Compute exact entity bounding box via ezdxf.bbox so CAD -> pixel mapping is 1-to-1 linear
    extents = None
    try:
        cache = ezdxf_bbox.Cache()
        box = ezdxf_bbox.extents(msp, cache=cache)
        if box.has_data:
            extents = [float(box.extmin.x), float(box.extmin.y), float(box.extmax.x), float(box.extmax.y)]
    except Exception:
        extents = None

    if not extents:
        try:
            extmin = doc.header.get("$EXTMIN", (0.0, 0.0, 0.0))
            extmax = doc.header.get("$EXTMAX", (100.0, 100.0, 0.0))
            extents = [float(extmin[0]), float(extmin[1]), float(extmax[0]), float(extmax[1])]
        except Exception:
            extents = [0.0, 0.0, 100.0, 100.0]

    xmin, ymin, xmax, ymax = extents
    span_x = xmax - xmin
    span_y = ymax - ymin
    if span_x <= 1e-6:
        span_x = 100.0
        xmax = xmin + span_x
    if span_y <= 1e-6:
        span_y = 100.0
        ymax = ymin + span_y

    # Add exact 3% symmetric viewport padding
    pad_x = span_x * 0.03
    pad_y = span_y * 0.03
    view_xmin = xmin - pad_x
    view_xmax = xmax + pad_x
    view_ymin = ymin - pad_y
    view_ymax = ymax + pad_y
    view_w = view_xmax - view_xmin
    view_h = view_ymax - view_ymin

    # Preserve exact CAD aspect ratio at crisp engineering resolution
    max_px = min(4096, max(1200, int(round(12.0 * dpi))))
    aspect = view_w / view_h
    if aspect >= 1.0:
        target_w = max_px
        target_h = max(256, int(round(max_px / aspect)))
    else:
        target_h = max_px
        target_w = max(256, int(round(max_px * aspect)))

    # Adjust view window minimally if integer rounding changed aspect by fraction of a pixel
    pixel_aspect = float(target_w) / float(target_h)
    if pixel_aspect > aspect:
        extra_w = view_h * pixel_aspect - view_w
        view_xmin -= extra_w * 0.5
        view_xmax += extra_w * 0.5
        view_w = view_xmax - view_xmin
    elif pixel_aspect < aspect:
        extra_h = view_w / pixel_aspect - view_h
        view_ymin -= extra_h * 0.5
        view_ymax += extra_h * 0.5
        view_h = view_ymax - view_ymin

    try:
        fig = plt.figure(figsize=(target_w / float(dpi), target_h / float(dpi)), dpi=dpi, facecolor="white")
        ax = fig.add_axes([0.0, 0.0, 1.0, 1.0], facecolor="white")
        ax.axis("off")

        ctx = RenderContext(doc)
        out = MatplotlibBackend(ax)
        frontend = Frontend(ctx, out)
        frontend.draw_layout(msp, finalize=True)

        # Enforce exact zero-margin limits AFTER draw_layout so CAD -> pixel transform is exact
        ax.set_aspect("equal", adjustable="box")
        ax.set_xlim(view_xmin, view_xmax)
        ax.set_ylim(view_ymin, view_ymax)

        out_dir = os.path.dirname(out_png_path)
        if out_dir:
            os.makedirs(out_dir, exist_ok=True)
        fig.savefig(out_png_path, dpi=dpi, facecolor="white")
        plt.close(fig)

        with Image.open(out_png_path) as img:
            rendered_w, rendered_h = img.size

        def cad_to_px(cx, cy):
            px = ((float(cx) - view_xmin) / view_w) * rendered_w
            py = ((view_ymax - float(cy)) / view_h) * rendered_h
            return [round(px, 2), round(py, 2)]

        entity_counts = {}
        block_refs = []
        features = []
        annotations = []
        max_features = 10000

        for ent in iter_flattened_entities(msp, max_depth=2):
            etype = ent.dxftype()
            entity_counts[etype] = entity_counts.get(etype, 0) + 1
            lname = getattr(ent.dxf, "layer", "0")
            color_hex = resolve_layer_color(doc, lname, getattr(ent.dxf, "color", 256))

            if etype == "INSERT":
                bname = getattr(ent.dxf, "name", "")
                ins = getattr(ent.dxf, "insert", None)
                if bname and len(block_refs) < 500:
                    pt = [round(float(ins.x), 3), round(float(ins.y), 3)] if ins is not None else [0.0, 0.0]
                    block_refs.append({
                        "name": bname,
                        "layer": lname,
                        "insert_cad": pt,
                        "insert_px": cad_to_px(pt[0], pt[1]),
                        "xscale": round(float(getattr(ent.dxf, "xscale", 1.0)), 4),
                        "yscale": round(float(getattr(ent.dxf, "yscale", 1.0)), 4),
                        "rotation": round(float(getattr(ent.dxf, "rotation", 0.0)), 2),
                    })
                continue

            if etype in ("TEXT", "MTEXT", "DIMENSION"):
                txt = ""
                cx, cy = 0.0, 0.0
                rot = 0.0
                char_h = 2.5
                try:
                    if etype == "TEXT":
                        txt = (ent.dxf.text or "").strip()
                        ins = ent.dxf.insert
                        cx, cy = float(ins.x), float(ins.y)
                        rot = float(getattr(ent.dxf, "rotation", 0.0) or 0.0)
                        char_h = float(getattr(ent.dxf, "height", 2.5) or 2.5)
                    elif etype == "MTEXT":
                        txt = (ent.plain_text() or "").replace("\n", " ").strip()
                        ins = ent.dxf.insert
                        cx, cy = float(ins.x), float(ins.y)
                        rot = float(getattr(ent, "get_rotation", lambda: 0.0)() or 0.0)
                        char_h = float(getattr(ent.dxf, "char_height", 2.5) or 2.5)
                    elif etype == "DIMENSION":
                        raw_txt = (getattr(ent.dxf, "text", "") or "").strip()
                        meas = getattr(ent, "get_measurement", lambda: None)()
                        if raw_txt and raw_txt != "<>":
                            txt = raw_txt
                        elif isinstance(meas, (int, float)):
                            txt = f"{meas:.2f}"
                        tp = getattr(ent.dxf, "text_midpoint", None) or getattr(ent.dxf, "defpoint", None)
                        if tp is not None:
                            cx, cy = float(tp.x), float(tp.y)
                except Exception:
                    txt = ""

                if txt:
                    px_xy = cad_to_px(cx, cy)
                    px_h = max(10.0, (char_h / view_h) * rendered_h)
                    px_w = max(12.0, px_h * 0.6 * len(txt))
                    bx0 = round(px_xy[0], 2)
                    by0 = round(px_xy[1] - px_h, 2)
                    bx1 = round(px_xy[0] + px_w, 2)
                    by1 = round(px_xy[1], 2)
                    annotations.append({
                        "text": txt,
                        "original_text": txt,
                        "category": "dimension" if etype == "DIMENSION" else classify_engineering_label(txt),
                        "source": "cad_vector",
                        "confidence": 1.0,
                        "cad_layer": lname,
                        "cad_xy": [round(cx, 4), round(cy, 4)],
                        "image_x": round((bx0 + bx1) * 0.5, 2),
                        "image_y": round((by0 + by1) * 0.5, 2),
                        "bbox_px": [bx0, by0, bx1, by1],
                        "rotation": round(-rot, 2),
                        "font_size": round(px_h, 2),
                        "text_color": color_hex,
                        "verified": True,
                        "visible": True,
                    })
                continue

            if len(features) >= max_features:
                continue

            try:
                if etype == "LINE":
                    s = ent.dxf.start
                    e = ent.dxf.end
                    features.append({
                        "geometry_type": "LineString",
                        "coords_px": [cad_to_px(s.x, s.y), cad_to_px(e.x, e.y)],
                        "coords_cad": [[round(float(s.x), 4), round(float(s.y), 4)], [round(float(e.x), 4), round(float(e.y), 4)]],
                        "stroke_color": color_hex,
                        "fill_color": None,
                        "line_width": 1.25,
                        "layer": lname,
                    })
                elif etype == "LWPOLYLINE":
                    pts_cad = [[round(float(p[0]), 4), round(float(p[1]), 4)] for p in ent.get_points("xy")]
                    if len(pts_cad) >= 2:
                        if ent.closed and pts_cad[0] != pts_cad[-1]:
                            pts_cad.append(pts_cad[0])
                        pts_px = [cad_to_px(p[0], p[1]) for p in pts_cad]
                        features.append({
                            "geometry_type": "LineString",
                            "coords_px": pts_px,
                            "coords_cad": pts_cad,
                            "stroke_color": color_hex,
                            "fill_color": None,
                            "line_width": 1.35,
                            "layer": lname,
                        })
                elif etype == "POLYLINE":
                    pts_cad = [[round(float(v.dxf.location.x), 4), round(float(v.dxf.location.y), 4)] for v in ent.vertices]
                    if len(pts_cad) >= 2:
                        if ent.is_closed and pts_cad[0] != pts_cad[-1]:
                            pts_cad.append(pts_cad[0])
                        pts_px = [cad_to_px(p[0], p[1]) for p in pts_cad]
                        features.append({
                            "geometry_type": "LineString",
                            "coords_px": pts_px,
                            "coords_cad": pts_cad,
                            "stroke_color": color_hex,
                            "fill_color": None,
                            "line_width": 1.35,
                            "layer": lname,
                        })
                elif etype == "CIRCLE":
                    c = ent.dxf.center
                    r = float(ent.dxf.radius)
                    pts = sample_arc_points(float(c.x), float(c.y), r, 0.0, 360.0, steps=28)
                    pts_cad = [[round(x, 4), round(y, 4)] for x, y in pts]
                    pts_px = [cad_to_px(x, y) for x, y in pts]
                    features.append({
                        "geometry_type": "LineString",
                        "coords_px": pts_px,
                        "coords_cad": pts_cad,
                        "stroke_color": color_hex,
                        "fill_color": None,
                        "line_width": 1.25,
                        "layer": lname,
                    })
                elif etype == "ARC":
                    c = ent.dxf.center
                    r = float(ent.dxf.radius)
                    pts = sample_arc_points(float(c.x), float(c.y), r, float(ent.dxf.start_angle), float(ent.dxf.end_angle), steps=24)
                    pts_cad = [[round(x, 4), round(y, 4)] for x, y in pts]
                    pts_px = [cad_to_px(x, y) for x, y in pts]
                    features.append({
                        "geometry_type": "LineString",
                        "coords_px": pts_px,
                        "coords_cad": pts_cad,
                        "stroke_color": color_hex,
                        "fill_color": None,
                        "line_width": 1.25,
                        "layer": lname,
                    })
            except Exception:
                continue

        cad_transform = {
            "view_bounds_cad": [round(view_xmin, 6), round(view_ymin, 6), round(view_xmax, 6), round(view_ymax, 6)],
            "entity_extents_cad": [round(xmin, 6), round(ymin, 6), round(xmax, 6), round(ymax, 6)],
            "rendered_width_px": rendered_w,
            "rendered_height_px": rendered_h,
            "units_per_pixel_x": round(view_w / float(rendered_w), 8),
            "units_per_pixel_y": round(view_h / float(rendered_h), 8),
        }

        if out_vectors_json:
            vec_dir = os.path.dirname(out_vectors_json)
            if vec_dir:
                os.makedirs(vec_dir, exist_ok=True)
            with open(out_vectors_json, "w", encoding="utf-8") as vf:
                json.dump({
                    "coordinate_space": "image_pixels",
                    "width": rendered_w,
                    "height": rendered_h,
                    "units": unit_str,
                    "dim_scale": dim_scale,
                    "lt_scale": lt_scale,
                    "cad_transform": cad_transform,
                    "layers": layer_details,
                    "entity_counts": entity_counts,
                    "block_references": block_refs,
                    "feature_count": len(features),
                    "annotation_count": len(annotations),
                    "features": features,
                    "annotations": annotations,
                }, vf)

        return {
            "success": True,
            "filename": os.path.basename(out_png_path),
            "width": rendered_w,
            "height": rendered_h,
            "dpi": dpi,
            "units": unit_str,
            "dim_scale": dim_scale,
            "lt_scale": lt_scale,
            "extents": [round(xmin, 4), round(ymin, 4), round(xmax, 4), round(ymax, 4)],
            "view_bounds_cad": cad_transform["view_bounds_cad"],
            "cad_transform": cad_transform,
            "layers": layers,
            "layer_details": layer_details,
            "entity_counts": entity_counts,
            "block_reference_count": len(block_refs),
            "vector_feature_count": len(features),
            "annotation_count": len(annotations),
            "vectors_path": out_vectors_json if out_vectors_json and os.path.exists(out_vectors_json) else "",
            "dxf_version": doc.dxfversion,
            "content": {
                "has_raster": False,
                "has_vector": len(features) > 0,
                "has_embedded_text": len(annotations) > 0,
                "classification": "cad_vector",
            },
        }
    except Exception as e:
        plt.close("all")
        return {"success": False, "error": f"Rendering DXF failed: {str(e)}", "code": "RENDER_FAILED"}


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "error": "Missing arguments"}))
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "convert":
        dxf_path = sys.argv[2]
        out_png = sys.argv[3]
        dpi = int(sys.argv[4]) if len(sys.argv) > 4 and sys.argv[4].isdigit() else 220
        out_vectors = sys.argv[5] if len(sys.argv) > 5 else ""
        res = convert_dxf_to_png(dxf_path, out_png, dpi, out_vectors)
        print(json.dumps(res))
    else:
        print(json.dumps({"success": False, "error": f"Unknown command: {cmd}"}))
        sys.exit(1)


if __name__ == "__main__":
    main()
