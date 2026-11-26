import json
import math
import os
import sqlite3
import sys
import warnings

warnings.filterwarnings("ignore")
os.environ.setdefault("OPENCV_LOG_LEVEL", "OFF")


def fail(stage, code, error):
    return {"success": False, "stage": stage, "code": code, "error": str(error)}


def inspect_gpkg(req):
    path = req["file_path"]
    max_features = int(req.get("max_gpkg_features") or 500000)

    try:
        conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    except Exception as e:
        return fail("parsing", "CORRUPTED_GPKG", f"Unable to open GeoPackage SQLite database: {e}")

    try:
        cur = conn.cursor()
        # 1. SQLite integrity check
        integ = cur.execute("PRAGMA integrity_check").fetchone()
        if not integ or str(integ[0]).lower() != "ok":
            return fail("validation", "CORRUPTED_GPKG", f"SQLite integrity check failed: {integ}")

        # 2. Verify required GeoPackage metadata tables
        tables = {row[0] for row in cur.execute("SELECT name FROM sqlite_master WHERE type='table'").fetchall()}
        required_tables = {"gpkg_contents", "gpkg_geometry_columns"}
        missing = sorted(required_tables - tables)
        if missing:
            return fail("validation", "INVALID_GPKG_SCHEMA", f"Missing required GeoPackage metadata tables: {', '.join(missing)}")

        # 3. Discover feature layers from gpkg_contents + gpkg_geometry_columns
        rows = cur.execute(
            """
            SELECT c.table_name, c.identifier, c.srs_id, c.min_x, c.min_y, c.max_x, c.max_y,
                   g.column_name, g.geometry_type_name
            FROM gpkg_contents c
            JOIN gpkg_geometry_columns g ON c.table_name = g.table_name
            WHERE c.data_type = 'features'
            """
        ).fetchall()

        if not rows:
            return fail("validation", "NO_FEATURE_LAYERS", "GeoPackage contains no valid feature layers")

        sub_layers = []
        total_features = 0
        geom_types = set()

        for (tname, ident, srs_id, min_x, min_y, max_x, max_y, geom_col, geom_type) in rows:
            if tname not in tables:
                return fail("validation", "MISSING_LAYER_TABLE", f"GeoPackage metadata references missing table '{tname}'")
            count = int(cur.execute(f'SELECT COUNT(*) FROM "{tname}"').fetchone()[0])
            total_features += count
            if total_features > max_features:
                return fail("validation", "FEATURE_LIMIT_EXCEEDED", f"GeoPackage feature count ({total_features}) exceeds limit ({max_features})")

            col_info = cur.execute(f'PRAGMA table_info("{tname}")').fetchall()
            fields = [c[1] for c in col_info if c[1] != geom_col]

            crs_str = ""
            if srs_id is not None and int(srs_id) > 0:
                crs_str = f"EPSG:{int(srs_id)}"

            extent = None
            if all(v is not None and math.isfinite(float(v)) for v in (min_x, min_y, max_x, max_y)):
                extent = [float(min_x), float(min_y), float(max_x), float(max_y)]

            gtype = str(geom_type or "Unknown")
            geom_types.add(gtype)
            sub_layers.append({
                "name": tname,
                "geometry_type": gtype,
                "crs": crs_str,
                "feature_count": count,
                "extent": extent,
                "fields": fields,
                "has_geospatial": bool(crs_str),
            })

        return {
            "success": True,
            "inspection": {
                "result_layer_type": "vector",
                "feature_count": total_features,
                "crs": sub_layers[0]["crs"] if sub_layers else "",
                "extent": sub_layers[0]["extent"] if sub_layers else None,
                "requires_georeferencing": not bool(sub_layers[0]["crs"]) if sub_layers else True,
                "sub_layers": sub_layers,
                "geometry_types": sorted(geom_types),
                "required_processing_ops": ["gpkg_extract_and_validate"],
            },
        }
    except Exception as e:
        return fail("parsing", "CORRUPTED_GPKG", f"Failed to inspect GeoPackage: {e}")
    finally:
        conn.close()


def process_gpkg(req):
    path = req["file_path"]
    staging_dir = req["staging_dir"]
    selected_layers = req.get("selected_layers") or []

    insp = inspect_gpkg(req)
    if not insp.get("success"):
        return insp

    all_layers = insp["inspection"]["sub_layers"]
    if not selected_layers:
        selected_layers = [l["name"] for l in all_layers]

    available_map = {l["name"]: l for l in all_layers}
    for lname in selected_layers:
        if lname not in available_map:
            return fail("validation", "INVALID_LAYER_SELECTION", f"Requested GeoPackage layer '{lname}' does not exist")

    try:
        import pyogrio
        import geopandas as gpd
        from shapely.geometry import mapping
    except Exception as e:
        return fail("processing", "MISSING_GIS_DEPENDENCY", f"GeoPackage processing requires pyogrio/geopandas/shapely: {e}")

    os.makedirs(staging_dir, exist_ok=True)
    outputs = []

    for lname in selected_layers:
        try:
            gdf = gpd.read_file(path, layer=lname)
        except Exception as e:
            return fail("parsing", "GPKG_LAYER_PARSE_FAILED", f"Failed to read GeoPackage layer '{lname}': {e}")

        if gdf.crs is None:
            return fail("validation", "MISSING_CRS", f"GeoPackage layer '{lname}' has no valid spatial reference (CRS)")

        # Validate every feature geometry strictly before staging output
        if gdf.geometry is None or gdf.geometry.isna().any():
            return fail("validation", "INVALID_GEOMETRY", f"GeoPackage layer '{lname}' contains null/missing geometries")

        for idx, geom in enumerate(gdf.geometry):
            if geom is None or geom.is_empty:
                return fail("validation", "EMPTY_GEOMETRY", f"Layer '{lname}' feature {idx} has an empty geometry")
            if not geom.is_valid:
                return fail("validation", "INVALID_GEOMETRY", f"Layer '{lname}' feature {idx} has invalid geometry")

        # Normalize to WGS84 for workspace GeoJSON representation
        try:
            gdf_wgs = gdf.to_crs("EPSG:4326")
        except Exception as e:
            return fail("processing", "CRS_TRANSFORM_FAILED", f"Failed to transform layer '{lname}' to EPSG:4326: {e}")

        bounds = gdf_wgs.total_bounds  # [minx, miny, maxx, maxy]
        if len(bounds) != 4 or not all(math.isfinite(float(b)) for b in bounds):
            return fail("validation", "INVALID_EXTENT", f"Layer '{lname}' produced invalid spatial extent")

        out_file = os.path.join(staging_dir, f"layer_{lname}.geojson")
        try:
            gdf_wgs.to_file(out_file, driver="GeoJSON")
        except Exception as e:
            return fail("processing", "GEOJSON_WRITE_FAILED", f"Failed to write normalized GeoJSON for '{lname}': {e}")

        outputs.append({
            "name": lname,
            "layer_type": "vector",
            "source_format": "gpkg",
            "file_path": out_file,
            "crs": "EPSG:4326",
            "extent": [float(bounds[0]), float(bounds[1]), float(bounds[2]), float(bounds[3])],
            "feature_count": int(len(gdf_wgs)),
            "requires_georeferencing": False,
        })

    return {"success": True, "outputs": outputs}


def inspect_pdf(req):
    path = req["file_path"]
    max_pages = int(req.get("max_pdf_pages") or 100)
    max_dim_pt = float(req.get("max_pdf_dim_pt") or 14400)

    try:
        import pymupdf
    except Exception as e:
        return fail("parsing", "MISSING_PDF_ENGINE", f"PyMuPDF is required for PDF inspection: {e}")

    try:
        doc = pymupdf.open(path)
    except Exception as e:
        return fail("parsing", "CORRUPTED_PDF", f"Failed to open PDF document: {e}")

    try:
        if doc.needs_pass or doc.is_encrypted:
            return fail("validation", "PDF_ENCRYPTED", "Encrypted or password-protected PDF documents are not supported")

        page_count = int(doc.page_count)
        if page_count <= 0:
            return fail("validation", "EMPTY_PDF", "PDF document contains 0 pages")
        if page_count > max_pages:
            return fail("validation", "PDF_PAGE_LIMIT_EXCEEDED", f"PDF page count ({page_count}) exceeds maximum limit ({max_pages})")

        sub_layers = []
        any_geo = False
        for i in range(page_count):
            page = doc.load_page(i)
            rect = page.rect
            w_pt = float(rect.width)
            h_pt = float(rect.height)
            if w_pt <= 0 or h_pt <= 0:
                return fail("validation", "INVALID_PDF_PAGE", f"PDF page {i + 1} has invalid dimensions")
            if w_pt > max_dim_pt or h_pt > max_dim_pt:
                return fail("validation", "PDF_PAGE_TOO_LARGE", f"PDF page {i + 1} dimensions ({w_pt:.0f}x{h_pt:.0f}pt) exceed limit ({max_dim_pt:.0f}pt)")

            has_geo = False
            try:
                obj_str = doc.xref_object(page.xref)
                if any(k in obj_str for k in ("/LGIDict", "/VP", "/Measure", "/GPTS")):
                    has_geo = True
                    any_geo = True
            except Exception:
                pass

            sub_layers.append({
                "name": f"Page {i + 1}",
                "page_number": i + 1,
                "width_pt": round(w_pt, 2),
                "height_pt": round(h_pt, 2),
                "rotation": int(page.rotation or 0),
                "has_geospatial": has_geo,
            })

        return {
            "success": True,
            "inspection": {
                "result_layer_type": "raster",
                "page_count": page_count,
                "requires_georeferencing": not any_geo,
                "sub_layers": sub_layers,
                "required_processing_ops": ["pdf_render_pages"],
            },
        }
    except Exception as e:
        return fail("parsing", "CORRUPTED_PDF", f"PDF inspection failed: {e}")
    finally:
        doc.close()


def process_pdf(req):
    path = req["file_path"]
    staging_dir = req["staging_dir"]
    selected_pages = req.get("selected_pages") or []
    dpi = int(req.get("dpi") or 180)
    max_dim = int(req.get("max_image_dimension") or 16384)

    insp = inspect_pdf(req)
    if not insp.get("success"):
        return insp

    page_count = insp["inspection"]["page_count"]
    if not selected_pages:
        selected_pages = list(range(1, page_count + 1))

    for p in selected_pages:
        if not isinstance(p, int) or p < 1 or p > page_count:
            return fail("validation", "INVALID_PAGE_SELECTION", f"Requested page {p} is outside valid range 1..{page_count}")

    import pymupdf
    os.makedirs(staging_dir, exist_ok=True)
    doc = pymupdf.open(path)
    outputs = []
    try:
        for p in selected_pages:
            page = doc.load_page(p - 1)
            scale = dpi / 72.0
            # Bound output pixel dimensions safely
            rect = page.rect
            if rect.width * scale > max_dim or rect.height * scale > max_dim:
                scale = min(max_dim / max(rect.width, 1.0), max_dim / max(rect.height, 1.0))
            pix = page.get_pixmap(matrix=pymupdf.Matrix(scale, scale), alpha=False)
            if pix.width <= 0 or pix.height <= 0:
                return fail("processing", "PDF_RENDER_FAILED", f"Rendered page {p} produced empty image")
            out_png = os.path.join(staging_dir, f"page_{p}.png")
            pix.save(out_png)
            if not os.path.isfile(out_png) or os.path.getsize(out_png) == 0:
                return fail("processing", "PDF_RENDER_FAILED", f"Failed to persist rendered PNG for page {p}")
            outputs.append({
                "name": f"Page {p}",
                "layer_type": "raster",
                "source_format": "pdf",
                "file_path": out_png,
                "width": int(pix.width),
                "height": int(pix.height),
                "requires_georeferencing": True,
            })
    except Exception as e:
        return fail("processing", "PDF_RENDER_FAILED", f"PDF rendering failed: {e}")
    finally:
        doc.close()

    return {"success": True, "outputs": outputs}


SUPPORTED_DXF_ENTITIES = {
    "LINE", "LWPOLYLINE", "POLYLINE", "ARC", "CIRCLE", "ELLIPSE",
    "SPLINE", "POINT", "TEXT", "MTEXT", "INSERT", "HATCH", "SOLID", "DIMENSION", "LEADER",
}
UNSUPPORTED_CRITICAL_DXF = {"3DSOLID", "BODY", "REGION", "OLE2FRAME", "PROXY_ENTITY", "ACAD_PROXY_ENTITY"}


def inspect_dxf(req):
    path = req["file_path"]
    max_entities = int(req.get("max_cad_entities") or 250000)

    try:
        import ezdxf
    except Exception as e:
        return fail("parsing", "MISSING_CAD_ENGINE", f"ezdxf is required for DXF inspection: {e}")

    try:
        doc = ezdxf.readfile(path)
    except Exception as e:
        return fail("parsing", "CORRUPTED_DXF", f"Failed to parse DXF drawing: {e}")

    msp = doc.modelspace()
    entity_count = 0
    supported_count = 0
    unsupported_found = set()
    cad_layers = [l.dxf.name for l in doc.layers]

    for ent in msp:
        entity_count += 1
        if entity_count > max_entities:
            return fail("validation", "CAD_ENTITY_LIMIT_EXCEEDED", f"DXF entity count exceeds limit ({max_entities})")
        et = ent.dxftype()
        if et in SUPPORTED_DXF_ENTITIES:
            supported_count += 1
        elif et in UNSUPPORTED_CRITICAL_DXF:
            unsupported_found.add(et)
        else:
            unsupported_found.add(et)

    if entity_count == 0:
        return fail("validation", "EMPTY_CAD_DRAWING", "DXF drawing contains 0 model-space entities")

    if supported_count == 0 and unsupported_found:
        return fail(
            "validation",
            "UNSUPPORTED_CAD_ENTITIES",
            f"DXF drawing contains only unsupported entity types: {', '.join(sorted(unsupported_found))}",
        )

    warnings_list = []
    if unsupported_found:
        warnings_list.append(f"Unsupported CAD entities detected: {', '.join(sorted(unsupported_found))}")

    return {
        "success": True,
        "inspection": {
            "result_layer_type": "raster",
            "entity_count": entity_count,
            "requires_georeferencing": True,
            "unsupported_entities": sorted(unsupported_found),
            "warnings": warnings_list,
            "required_processing_ops": ["cad_parse_and_rasterize"],
            "extra": {
                "dxf_version": getattr(doc, "dxfversion", ""),
                "units_code": int(doc.header.get("$INSUNITS", 0) or 0),
                "cad_layers": cad_layers,
            },
        },
    }


def process_dxf(req):
    path = req["file_path"]
    staging_dir = req["staging_dir"]

    insp = inspect_dxf(req)
    if not insp.get("success"):
        return insp

    os.makedirs(staging_dir, exist_ok=True)
    out_png = os.path.join(staging_dir, "cad_raster.png")

    try:
        import ezdxf
        from ezdxf.addons.drawing import RenderContext, Frontend
        from ezdxf.addons.drawing.matplotlib import MatplotlibBackend
        import matplotlib
        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
        from PIL import Image

        doc = ezdxf.readfile(path)
        msp = doc.modelspace()
        fig = plt.figure(figsize=(10, 10), dpi=180)
        ax = fig.add_axes([0, 0, 1, 1])
        ctx = RenderContext(doc)
        out = MatplotlibBackend(ax)
        Frontend(ctx, out).draw_layout(msp, finalize=True)
        ax.set_aspect("equal")
        ax.axis("off")
        fig.savefig(out_png, dpi=180, bbox_inches="tight", pad_inches=0.05)
        plt.close(fig)

        with Image.open(out_png) as im:
            w, h = im.size
        if w <= 0 or h <= 0:
            return fail("processing", "CAD_RASTERIZE_FAILED", "CAD rasterization produced empty image")

        return {
            "success": True,
            "outputs": [
                {
                    "name": os.path.splitext(os.path.basename(path))[0],
                    "layer_type": "raster",
                    "source_format": "dxf",
                    "file_path": out_png,
                    "width": int(w),
                    "height": int(h),
                    "requires_georeferencing": True,
                }
            ],
        }
    except Exception as e:
        return fail("processing", "CAD_RASTERIZE_FAILED", f"CAD rasterization failed: {e}")


def main():
    if len(sys.argv) < 2:
        print(json.dumps(fail("parsing", "MISSING_PAYLOAD", "Missing JSON payload argument")))
        return
    try:
        req = json.loads(sys.argv[1])
    except Exception as e:
        print(json.dumps(fail("parsing", "INVALID_PAYLOAD", f"Invalid JSON argument: {e}")))
        return

    action = req.get("action", "")
    handlers = {
        "inspect-gpkg": inspect_gpkg,
        "process-gpkg": process_gpkg,
        "inspect-pdf": inspect_pdf,
        "process-pdf": process_pdf,
        "inspect-dxf": inspect_dxf,
        "process-dxf": process_dxf,
    }
    fn = handlers.get(action)
    if not fn:
        print(json.dumps(fail("parsing", "UNKNOWN_ACTION", f"Unsupported action '{action}'")))
        return
    print(json.dumps(fn(req)))


if __name__ == "__main__":
    main()
