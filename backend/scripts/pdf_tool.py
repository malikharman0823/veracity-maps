import sys
import os
import json
import pymupdf

def inspect_pdf(pdf_path, thumb_dir):
    try:
        doc = pymupdf.open(pdf_path)
    except Exception as e:
        return {"success": False, "error": f"Failed to open PDF: {str(e)}", "code": "CORRUPTED_PDF"}

    if doc.is_encrypted:
        return {"success": False, "error": "Password-protected or encrypted PDF is not supported", "code": "ENCRYPTED_PDF"}

    page_count = len(doc)
    if page_count == 0:
        return {"success": False, "error": "PDF has no pages", "code": "EMPTY_PDF"}

    os.makedirs(thumb_dir, exist_ok=True)
    pages = []

    for i in range(page_count):
        page = doc[i]
        rect = page.rect
        rotation = page.rotation
        w = float(rect.width)
        h = float(rect.height)

        # Check for geospatial PDF metadata
        has_geo = False
        try:
            # Check for standard geospatial dictionary keys
            if "/LGIDict" in page.get_text("rawjson") or "/VP" in page.get_text("rawjson"):
                has_geo = True
        except:
            pass

        # Generate lightweight thumbnail (e.g. 150px wide or 72 dpi)
        pix = page.get_pixmap(dpi=72)
        thumb_filename = f"thumb_page_{i+1}.png"
        thumb_path = os.path.join(thumb_dir, thumb_filename)
        pix.save(thumb_path)

        pages.append({
            "page_number": i + 1,
            "width": int(w),
            "height": int(h),
            "rotation": int(rotation),
            "has_georeference": has_geo,
            "thumbnail_filename": thumb_filename
        })

    doc.close()
    return {
        "success": True,
        "page_count": page_count,
        "pages": pages
    }

def render_pages(pdf_path, selected_pages, out_dir, dpi=150):
    try:
        doc = pymupdf.open(pdf_path)
    except Exception as e:
        return {"success": False, "error": f"Failed to open PDF: {str(e)}", "code": "CORRUPTED_PDF"}

    if doc.is_encrypted:
        return {"success": False, "error": "PDF is encrypted", "code": "ENCRYPTED_PDF"}

    os.makedirs(out_dir, exist_ok=True)
    rendered = []

    for page_num in selected_pages:
        idx = page_num - 1
        if idx < 0 or idx >= len(doc):
            continue

        page = doc[idx]
        pix = page.get_pixmap(dpi=dpi)
        filename = f"page_{page_num}.png"
        out_path = os.path.join(out_dir, filename)
        pix.save(out_path)

        rendered.append({
            "page_number": page_num,
            "filename": filename,
            "path": out_path,
            "width": pix.width,
            "height": pix.height,
            "dpi": dpi,
            "rotation": page.rotation
        })

    doc.close()
    return {
        "success": True,
        "rendered": rendered
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
        pages_arg = sys.argv[3] # comma-separated page numbers
        out_dir = sys.argv[4]
        dpi = int(sys.argv[5]) if len(sys.argv) > 5 else 150
        selected = [int(p.strip()) for p in pages_arg.split(",") if p.strip().isdigit()]
        res = render_pages(pdf_path, selected, out_dir, dpi)
        print(json.dumps(res))
    else:
        print(json.dumps({"success": False, "error": f"Unknown command: {cmd}"}))
        sys.exit(1)

if __name__ == "__main__":
    main()
