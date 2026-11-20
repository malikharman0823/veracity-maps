import sys
import os
import json
import math
import sqlite3
import zipfile
import tempfile
import shutil
import warnings

warnings.filterwarnings("ignore")

try:
    import shapely
    from shapely.geometry import shape, mapping, LineString, Polygon, MultiPolygon, Point, box
    from shapely.ops import split as shapely_split, unary_union, snap as shapely_snap
    from shapely.validation import explain_validity, make_valid
    HAS_SHAPELY = True
except Exception:
    HAS_SHAPELY = False

try:
    import pyogrio
    HAS_PYOGRIO = True
except Exception:
    HAS_PYOGRIO = False

try:
    import geopandas as gpd
    HAS_GEOPANDAS = True
except Exception:
    HAS_GEOPANDAS = False

try:
    import rasterio
    from rasterio.crs import CRS
    from rasterio.warp import transform_geom, calculate_default_transform, reproject, Resampling, transform_bounds
    HAS_RASTERIO = True
except Exception:
    HAS_RASTERIO = False

try:
    import ezdxf
    HAS_EZDXF = True
except Exception:
    HAS_EZDXF = False


def normalize_crs_str(crs_obj):
    if crs_obj is None:
        return ""
    try:
        if hasattr(crs_obj, "to_epsg"):
            epsg = crs_obj.to_epsg()
            if epsg:
                return f"EPSG:{epsg}"
        if hasattr(crs_obj, "to_string"):
            s = crs_obj.to_string()
            if s:
                return str(s)
    except Exception:
        pass
    s = str(crs_obj).strip()
    if s.isdigit():
        return f"EPSG:{s}"
    return s


def normalize_geom_type(raw_type):
    if not raw_type:
        return "Unknown"
    t = str(raw_type).strip()
    tl = t.lower()
    if "multipolygon" in tl:
        return "MultiPolygon"
    if "polygon" in tl:
        return "Polygon"
    if "multilinestring" in tl or "multiline" in tl:
        return "MultiLineString"
    if "linestring" in tl or "line" in tl:
        return "LineString"
    if "multipoint" in tl:
        return "MultiPoint"
    if "point" in tl:
        return "Point"
    if tl in ("none", "null", "table", "attributes"):
        return "Table"
    if tl in ("raster", "tiles", "2d-gridded-coverage"):
        return "Raster"
    return t


def json_safe_value(val):
    if val is None:
        return None
    if isinstance(val, (bool, int, str)):
        return val
    if isinstance(val, float):
        if math.isnan(val) or math.isinf(val):
            return None
        return val
    if isinstance(val, bytes):
        try:
            return val.decode("utf-8", errors="replace")
        except Exception:
            return str(val)
    # Handle pandas/numpy scalars & timestamps
    try:
        if hasattr(val, "isna") and callable(val.isna) and val.isna():
            return None
    except Exception:
        pass
    try:
        import pandas as pd
        if pd.isna(val):
            return None
    except Exception:
        pass
    if hasattr(val, "item") and callable(val.item):
        try:
            return json_safe_value(val.item())
        except Exception:
            pass
    if hasattr(val, "isoformat") and callable(val.isoformat):
        try:
            return val.isoformat()
        except Exception:
            pass
    return str(val)


def strip_z_coords(coords):
    """Recursively strips Z/M dimensions from GeoJSON coordinates, keeping [x, y]."""
    if not isinstance(coords, (list, tuple)):
        return coords
    if len(coords) >= 2 and isinstance(coords[0], (int, float)) and isinstance(coords[1], (int, float)):
        return [float(coords[0]), float(coords[1])]
    return [strip_z_coords(c) for c in coords]


def force_2d_geojson(geom_dict):
    """Normalizes a GeoJSON geometry dict to 2D coordinates and extracts Points from GeometryCollections."""
    if not isinstance(geom_dict, dict):
        return geom_dict
    gtype = geom_dict.get("type")
    if gtype == "GeometryCollection":
        geoms = geom_dict.get("geometries") or []
        pts = []
        for sub in geoms:
            sub2d = force_2d_geojson(sub)
            if not isinstance(sub2d, dict):
                continue
            st = sub2d.get("type")
            if st == "Point" and isinstance(sub2d.get("coordinates"), list) and len(sub2d["coordinates"]) >= 2:
                pts.append(sub2d["coordinates"][:2])
            elif st == "MultiPoint" and isinstance(sub2d.get("coordinates"), list):
                pts.extend([p[:2] for p in sub2d["coordinates"] if isinstance(p, list) and len(p) >= 2])
        if len(pts) == 1:
            return {"type": "Point", "coordinates": pts[0]}
        if len(pts) > 1:
            return {"type": "MultiPoint", "coordinates": pts}
        if geoms:
            return force_2d_geojson(geoms[0])
        return None
    if "coordinates" in geom_dict:
        return {
            "type": gtype,
            "coordinates": strip_z_coords(geom_dict["coordinates"]),
        }
    return geom_dict


def detect_lat_lon_columns(col_names):
    """Detects (lat_col, lon_col) names in non-spatial attribute tables."""
    lat_candidates = ("latitude", "lat", "lat_dd", "lat_deg", "y_lat", "point_y")
    lon_candidates = ("longitude", "lon", "lng", "long", "lon_dd", "lon_deg", "x_lon", "point_x")
    lower_map = {str(c).strip().lower(): str(c) for c in col_names}
    lat_col = None
    lon_col = None
    for cand in lat_candidates:
        if cand in lower_map:
            lat_col = lower_map[cand]
            break
    for cand in lon_candidates:
        if cand in lower_map:
            lon_col = lower_map[cand]
            break
    return lat_col, lon_col


def inspect_gpkg_sqlite_extras(file_path):
    """
    Inspects standard OGC GeoPackage metadata tables (gpkg_contents, gpkg_geometry_columns)
    to detect raster tile layers, non-spatial attribute tables (including lat/lon point tables), and CRS SRIDs.
    """
    extras = {"rasters": [], "tables": [], "point_tables": {}, "srid_by_layer": {}, "bounds_by_layer": {}}
    try:
        conn = sqlite3.connect(f"file:{file_path}?mode=ro", uri=True)
        cur = conn.cursor()
        cur.execute("SELECT name FROM sqlite_master WHERE type='table' AND name='gpkg_contents'")
        if not cur.fetchone():
            conn.close()
            return extras

        cur.execute(
            "SELECT table_name, data_type, identifier, description, min_x, min_y, max_x, max_y, srs_id FROM gpkg_contents"
        )
        rows = cur.fetchall()
        # Some producers omit ordinary attribute tables from gpkg_contents.
        registered = {row[0] for row in rows}
        cur.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
        for (name,) in cur.fetchall():
            if name not in registered and not name.lower().startswith(("gpkg_", "sqlite_", "rtree_")):
                rows.append((name, "attributes", name, "", None, None, None, None, None))
        for table_name, data_type, identifier, description, min_x, min_y, max_x, max_y, srs_id in rows:
            crs_str = f"EPSG:{srs_id}" if srs_id and int(srs_id) > 0 else ""
            extras["srid_by_layer"][table_name] = crs_str
            b = None
            if None not in (min_x, min_y, max_x, max_y):
                try:
                    fx = [float(min_x), float(min_y), float(max_x), float(max_y)]
                    if all(math.isfinite(v) for v in fx) and fx[0] < fx[2] and fx[1] < fx[3]:
                        b = fx
                        extras["bounds_by_layer"][table_name] = b
                except Exception:
                    pass

            dt = (data_type or "").lower()
            if dt in ("tiles", "2d-gridded-coverage"):
                extras["rasters"].append({
                    "layer_name": table_name,
                    "display_name": identifier or table_name,
                    "description": description or "",
                    "geometry_type": "Raster",
                    "layer_kind": "raster",
                    "crs": crs_str,
                    "feature_count": 1,
                    "bounds": b,
                    "fields": [],
                })
            elif dt == "attributes":
                # Count rows & inspect columns
                cnt = 0
                fields = []
                safe_tbl = table_name.replace('"', '""')
                try:
                    cur.execute(f'SELECT COUNT(*) FROM "{safe_tbl}"')
                    cnt = int(cur.fetchone()[0])
                    cur.execute(f'PRAGMA table_info("{safe_tbl}")')
                    for col in cur.fetchall():
                        ctype = (col[2] or "").lower()
                        norm_t = "string"
                        if "int" in ctype:
                            norm_t = "integer"
                        elif "real" in ctype or "float" in ctype or "double" in ctype:
                            norm_t = "float"
                        elif "bool" in ctype:
                            norm_t = "boolean"
                        fields.append({"name": str(col[1]), "type": norm_t})
                except Exception:
                    pass

                # Check if this attribute table stores point coordinates in lat/lon columns
                lat_col, lon_col = detect_lat_lon_columns([f["name"] for f in fields])
                is_point_table = False
                pt_bounds = b
                if lat_col and lon_col and cnt > 0:
                    try:
                        safe_lat = lat_col.replace('"', '""')
                        safe_lon = lon_col.replace('"', '""')
                        cur.execute(
                            f'SELECT MIN(CAST("{safe_lon}" AS REAL)), MIN(CAST("{safe_lat}" AS REAL)), '
                            f'MAX(CAST("{safe_lon}" AS REAL)), MAX(CAST("{safe_lat}" AS REAL)), COUNT(*) '
                            f'FROM "{safe_tbl}" '
                            f'WHERE "{safe_lat}" IS NOT NULL AND "{safe_lon}" IS NOT NULL '
                            f'AND CAST("{safe_lat}" AS REAL) BETWEEN -90 AND 90 '
                            f'AND CAST("{safe_lon}" AS REAL) BETWEEN -180 AND 180'
                        )
                        row_b = cur.fetchone()
                        if row_b and row_b[4] and int(row_b[4]) > 0 and None not in row_b[:4]:
                            fx = [float(row_b[0]), float(row_b[1]), float(row_b[2]), float(row_b[3])]
                            if all(math.isfinite(v) for v in fx):
                                is_point_table = True
                                pt_bounds = fx
                                extras["point_tables"][table_name] = {
                                    "lat_col": lat_col,
                                    "lon_col": lon_col,
                                    "bounds": pt_bounds,
                                }
                    except Exception:
                        pass

                extras["tables"].append({
                    "layer_name": table_name,
                    "display_name": identifier or table_name,
                    "description": description or "",
                    "geometry_type": "Point" if is_point_table else "Table",
                    "layer_kind": "vector" if is_point_table else "table",
                    "crs": "EPSG:4326" if is_point_table else crs_str,
                    "feature_count": cnt,
                    "bounds": pt_bounds,
                    "wgs84_bounds": pt_bounds if is_point_table else None,
                    "fields": fields,
                })
        conn.close()
    except Exception:
        pass
    return extras


def resolve_zip_spatial_file(zip_path):
    """Extracts a ZIP to a temp directory and returns (target_file, temp_dir)."""
    tmp_dir = tempfile.mkdtemp(prefix="veracity_vec_")
    try:
        with zipfile.ZipFile(zip_path, "r") as zf:
            for member in zf.namelist():
                target = os.path.realpath(os.path.join(tmp_dir, member))
                if not target.startswith(os.path.realpath(tmp_dir)):
                    continue
                if member.endswith("/"):
                    os.makedirs(target, exist_ok=True)
                else:
                    os.makedirs(os.path.dirname(target), exist_ok=True)
                    with zf.open(member) as src, open(target, "wb") as dst:
                        shutil.copyfileobj(src, dst)
        for root, _, files in os.walk(tmp_dir):
            for f in files:
                fl = f.lower()
                if fl.endswith((".gpkg", ".shp", ".geojson", ".kml", ".osm", ".dxf")):
                    return os.path.join(root, f), tmp_dir
    except Exception:
        pass
    shutil.rmtree(tmp_dir, ignore_errors=True)
    return None, None


def transform_bounds_to_wgs84(bounds, crs_str):
    if not bounds or len(bounds) != 4:
        return None
    if not crs_str or crs_str.upper() == "EPSG:4326":
        return bounds
    if HAS_RASTERIO:
        try:
            src = CRS.from_user_input(crs_str)
            dst = CRS.from_epsg(4326)
            if src != dst:
                wb = transform_bounds(src, dst, bounds[0], bounds[1], bounds[2], bounds[3], densify_pts=8)
                out = [float(x) for x in wb]
                if all(math.isfinite(v) for v in out) and out[0] < out[2] and out[1] < out[3]:
                    return out
        except Exception:
            pass
    return bounds


def inspect_vector_dataset(file_path):
    """
    Dynamically inspects GeoPackage (.gpkg), Shapefile (.shp), ZIP archive, GeoJSON, KML, OSM, or DXF
    using GDAL/OGR (pyogrio), SQLite (GeoPackage metadata), or ezdxf.
    """
    if not os.path.exists(file_path):
        return {"success": False, "error": f"File not found: {file_path}"}

    ext = os.path.splitext(file_path)[1].lower()
    tmp_dir = None
    actual_path = file_path

    if ext == ".zip":
        resolved, tmp_dir = resolve_zip_spatial_file(file_path)
        if not resolved:
            return {"success": False, "error": "No supported spatial dataset found inside ZIP archive"}
        actual_path = resolved
        ext = os.path.splitext(actual_path)[1].lower()

    try:
        if ext in (".geojson", ".json"):
            with open(actual_path, "r", encoding="utf-8") as f:
                data = json.load(f)
            features = data.get("features", []) if data.get("type") == "FeatureCollection" else ([data] if data.get("type") == "Feature" else [])
            geom_type = "Unknown"
            fields_map = {}
            minx, miny, maxx, maxy = float("inf"), float("inf"), float("-inf"), float("-inf")
            for feat in features:
                geom = feat.get("geometry")
                if geom and geom.get("type"):
                    if geom_type == "Unknown":
                        geom_type = normalize_geom_type(geom["type"])
                    if HAS_SHAPELY:
                        try:
                            g = shape(geom)
                            b = g.bounds
                            if len(b) == 4:
                                minx = min(minx, b[0])
                                miny = min(miny, b[1])
                                maxx = max(maxx, b[2])
                                maxy = max(maxy, b[3])
                        except Exception:
                            pass
                props = feat.get("properties") or {}
                for k, v in props.items():
                    if k not in fields_map:
                        ftype = "string"
                        if isinstance(v, bool):
                            ftype = "boolean"
                        elif isinstance(v, int):
                            ftype = "integer"
                        elif isinstance(v, float):
                            ftype = "float"
                        fields_map[k] = {"name": k, "type": ftype}
            bounds = [minx, miny, maxx, maxy] if minx <= maxx and miny <= maxy else None
            crs_str = "EPSG:4326"
            if isinstance(data.get("crs"), dict):
                crs_name = data["crs"].get("properties", {}).get("name", "")
                if crs_name:
                    crs_str = crs_name
            layer_name = os.path.splitext(os.path.basename(actual_path))[0]
            return {
                "success": True,
                "driver": "GeoJSON",
                "layers": [
                    {
                        "layer_name": layer_name,
                        "geometry_type": geom_type,
                        "layer_kind": "vector",
                        "crs": crs_str,
                        "feature_count": len(features),
                        "bounds": bounds,
                        "wgs84_bounds": transform_bounds_to_wgs84(bounds, crs_str),
                        "fields": list(fields_map.values()),
                    }
                ],
            }

        if ext == ".dxf" and HAS_EZDXF:
            doc = ezdxf.readfile(actual_path)
            msp = doc.modelspace()
            layers_dict = {}
            for entity in msp:
                lname = entity.dxf.layer if entity.dxf.hasattr("layer") else "0"
                dtype = entity.dxftype()
                gtype = "LineString"
                if dtype in ("POINT", "INSERT", "TEXT", "MTEXT"):
                    gtype = "Point"
                elif dtype in ("HATCH", "SOLID", "3DFACE"):
                    gtype = "Polygon"
                if lname not in layers_dict:
                    layers_dict[lname] = {"layer_name": lname, "geometry_type": gtype, "count": 0}
                layers_dict[lname]["count"] += 1
            out_layers = []
            for lname, info in layers_dict.items():
                out_layers.append({
                    "layer_name": lname,
                    "geometry_type": info["geometry_type"],
                    "layer_kind": "vector",
                    "crs": "LOCAL_CAD",
                    "feature_count": info["count"],
                    "bounds": None,
                    "wgs84_bounds": None,
                    "fields": [{"name": "layer", "type": "string"}, {"name": "entity_type", "type": "string"}],
                })
            if not out_layers:
                out_layers.append({
                    "layer_name": "0",
                    "geometry_type": "LineString",
                    "layer_kind": "vector",
                    "crs": "LOCAL_CAD",
                    "feature_count": 0,
                    "bounds": None,
                    "wgs84_bounds": None,
                    "fields": [],
                })
            return {"success": True, "driver": "DXF", "layers": out_layers}

        if HAS_PYOGRIO and ext in (".gpkg", ".shp", ".kml", ".gml", ".fgb", ".tab", ".mif"):
            gpkg_extras = (
                inspect_gpkg_sqlite_extras(actual_path)
                if ext == ".gpkg"
                else {"rasters": [], "tables": [], "point_tables": {}, "srid_by_layer": {}, "bounds_by_layer": {}}
            )
            raw_layers = pyogrio.list_layers(actual_path)
            out_layers = []
            seen_names = set()
            for row in raw_layers:
                lname = str(row[0])
                seen_names.add(lname)
                raw_gtype = str(row[1]) if row[1] is not None else "Table"
                gtype = normalize_geom_type(raw_gtype)
                info = pyogrio.read_info(actual_path, layer=lname, force_feature_count=True, force_total_bounds=True)
                crs_val = normalize_crs_str(info.get("crs")) or gpkg_extras["srid_by_layer"].get(lname, "")
                fcount = int(info.get("features", 0))
                total_bounds = info.get("total_bounds")
                bounds = (
                    list(map(float, total_bounds))
                    if total_bounds is not None and len(total_bounds) == 4 and not any(math.isnan(x) for x in total_bounds)
                    else gpkg_extras["bounds_by_layer"].get(lname)
                )
                # Promote attribute tables with numeric lat/lon columns to Point vector layers
                pt_meta = gpkg_extras.get("point_tables", {}).get(lname)
                if gtype == "Table" and pt_meta is not None:
                    gtype = "Point"
                    crs_val = "EPSG:4326"
                    bounds = pt_meta["bounds"]
                # Deep-scan generic Geometry/Unknown layers to accurately detect Point layers
                elif gtype in ("Unknown", "Geometry", "GeometryCollection") and HAS_GEOPANDAS and fcount > 0:
                    try:
                        observed = set()
                        scan_offset = 0
                        while True:
                            sample_gdf = pyogrio.read_dataframe(actual_path, layer=lname, columns=[], skip_features=scan_offset, max_features=5000)
                            if len(sample_gdf) == 0:
                                break
                            for g_item in sample_gdf.geometry:
                                if hasattr(g_item, "is_empty") and not g_item.is_empty:
                                    normalized = force_2d_geojson(mapping(g_item))
                                    if normalized:
                                        observed.add(normalize_geom_type(normalized.get("type")))
                            scan_offset += len(sample_gdf)
                        if observed.issubset({"Point", "MultiPoint"}) and observed:
                            gtype = "MultiPoint" if observed == {"MultiPoint"} else "Point"
                        elif "Point" in observed or "MultiPoint" in observed:
                            gtype = "Geometry"
                        elif len(observed) == 1:
                            gtype = next(iter(observed))
                    except Exception as e:
                        raise ValueError(f"Could not scan layer {lname}: {e}") from e
                wgs84_bounds = transform_bounds_to_wgs84(bounds, crs_val) if bounds else None
                fields_list = []
                fnames = info.get("fields", [])
                ftypes = info.get("dtypes", [])
                for idx, fn in enumerate(fnames):
                    dt = str(ftypes[idx]) if idx < len(ftypes) else "string"
                    norm_t = "string"
                    if "int" in dt.lower():
                        norm_t = "integer"
                    elif "float" in dt.lower() or "double" in dt.lower() or "real" in dt.lower():
                        norm_t = "float"
                    elif "bool" in dt.lower():
                        norm_t = "boolean"
                    fields_list.append({"name": str(fn), "type": norm_t})
                out_layers.append({
                    "layer_name": lname,
                    "geometry_type": gtype,
                    "layer_kind": "table" if gtype == "Table" else "vector",
                    "crs": crs_val,
                    "feature_count": fcount,
                    "bounds": bounds,
                    "wgs84_bounds": wgs84_bounds,
                    "fields": fields_list,
                })
            # Append any GeoPackage raster tile layers & non-spatial tables not already listed
            for r in gpkg_extras["rasters"]:
                if r["layer_name"] not in seen_names:
                    r["wgs84_bounds"] = transform_bounds_to_wgs84(r.get("bounds"), r.get("crs")) if r.get("bounds") else None
                    out_layers.append(r)
                    seen_names.add(r["layer_name"])
            for tbl in gpkg_extras["tables"]:
                if tbl["layer_name"] not in seen_names:
                    out_layers.append(tbl)
                    seen_names.add(tbl["layer_name"])
            return {
                "success": True,
                "driver": "GPKG" if ext == ".gpkg" else ext.lstrip(".").upper(),
                "layers": out_layers,
            }

        if ext in (".kml", ".osm"):
            import xml.etree.ElementTree as ET
            tree = ET.parse(actual_path)
            root = tree.getroot()
            layer_name = os.path.splitext(os.path.basename(actual_path))[0]
            if ext == ".kml":
                placemarks = [el for el in root.iter() if el.tag.endswith("Placemark")]
                return {
                    "success": True,
                    "driver": "KML",
                    "layers": [
                        {
                            "layer_name": layer_name,
                            "geometry_type": "Geometry",
                            "layer_kind": "vector",
                            "crs": "EPSG:4326",
                            "feature_count": len(placemarks),
                            "bounds": None,
                            "wgs84_bounds": None,
                            "fields": [{"name": "name", "type": "string"}, {"name": "description", "type": "string"}],
                        }
                    ],
                }
            else:
                nodes = [el for el in root.iter() if el.tag == "node"]
                ways = [el for el in root.iter() if el.tag == "way"]
                return {
                    "success": True,
                    "driver": "OSM",
                    "layers": [
                        {
                            "layer_name": f"{layer_name}_nodes",
                            "geometry_type": "Point",
                            "layer_kind": "vector",
                            "crs": "EPSG:4326",
                            "feature_count": len(nodes),
                            "bounds": None,
                            "wgs84_bounds": None,
                            "fields": [{"name": "id", "type": "string"}],
                        },
                        {
                            "layer_name": f"{layer_name}_ways",
                            "geometry_type": "LineString",
                            "layer_kind": "vector",
                            "crs": "EPSG:4326",
                            "feature_count": len(ways),
                            "bounds": None,
                            "wgs84_bounds": None,
                            "fields": [{"name": "id", "type": "string"}],
                        },
                    ],
                }

        return {"success": False, "error": f"Unsupported vector format or missing driver for {ext}"}
    except Exception as e:
        return {"success": False, "error": str(e)}
    finally:
        if tmp_dir:
            shutil.rmtree(tmp_dir, ignore_errors=True)


def extract_gpkg_table(file_path, layer_name, max_features, offset):
    """Read coordinate/attribute tables even when OGR cannot discover them."""
    conn = sqlite3.connect(f"file:{file_path}?mode=ro", uri=True)
    try:
        safe_table = layer_name.replace('"', '""')
        columns = conn.execute(f'PRAGMA table_info("{safe_table}")').fetchall()
        names = [col[1] for col in columns]
        primary_key = next((col[1] for col in columns if col[5]), None)
        lat_col, lon_col = detect_lat_lon_columns(names)
        total = conn.execute(f'SELECT COUNT(*) FROM "{safe_table}"').fetchone()[0]
        rows = conn.execute(f'SELECT * FROM "{safe_table}" LIMIT ? OFFSET ?', (max_features, offset))
        features = []
        bounds = None
        for idx, row in enumerate(rows):
            props = dict(zip(names, (json_safe_value(v) for v in row)))
            geometry = None
            if lat_col and lon_col:
                try:
                    lat, lon = float(props[lat_col]), float(props[lon_col])
                    if math.isfinite(lat) and math.isfinite(lon) and -90 <= lat <= 90 and -180 <= lon <= 180:
                        geometry = {"type": "Point", "coordinates": [lon, lat]}
                        if bounds is None:
                            bounds = [lon, lat, lon, lat]
                        else:
                            bounds = [min(bounds[0], lon), min(bounds[1], lat), max(bounds[2], lon), max(bounds[3], lat)]
                except (TypeError, ValueError):
                    pass
            fid = props.get(primary_key) if primary_key else offset + idx + 1
            features.append({"feature_id": str(fid), "geometry": geometry, "properties": props})
        return {"success": True, "layer_name": layer_name, "source_crs": "EPSG:4326" if lat_col and lon_col else "",
                "geometry_type": "Point" if bounds else "Table", "total_features": total, "bounds": bounds, "features": features}
    finally:
        conn.close()


def extract_layer_features(file_path, layer_name=None, max_features=5000, offset=0):
    """
    Extracts features from a specific named layer in any supported GIS dataset (.gpkg, .shp, .zip,
    .geojson, .kml, .osm), reprojecting geometries to EPSG:4326 for MapLibre while preserving
    source_crs metadata and all dynamic attribute fields.
    """
    if not os.path.exists(file_path):
        return {"success": False, "error": f"File not found: {file_path}"}

    ext = os.path.splitext(file_path)[1].lower()
    tmp_dir = None
    actual_path = file_path

    if ext == ".zip":
        resolved, tmp_dir = resolve_zip_spatial_file(file_path)
        if not resolved:
            return {"success": False, "error": "No supported spatial dataset found inside ZIP archive"}
        actual_path = resolved
        ext = os.path.splitext(actual_path)[1].lower()

    try:
        if ext in (".geojson", ".json"):
            with open(actual_path, "r", encoding="utf-8") as f:
                data = json.load(f)
            raw_feats = data.get("features", []) if data.get("type") == "FeatureCollection" else ([data] if data.get("type") == "Feature" else [])
            total = len(raw_feats)
            sliced = raw_feats[offset : offset + max_features]
            out_feats = []
            geom_type = "Unknown"
            for idx, f_obj in enumerate(sliced):
                geom = f_obj.get("geometry")
                if geom and geom.get("type") and geom_type == "Unknown":
                    geom_type = normalize_geom_type(geom["type"])
                props = {str(k): json_safe_value(v) for k, v in (f_obj.get("properties") or {}).items()}
                fid = f_obj.get("id")
                out_feats.append({
                    "feature_id": str(fid) if fid is not None else str(offset + idx + 1),
                    "geometry": geom,
                    "properties": props,
                })
            return {
                "success": True,
                "layer_name": layer_name or os.path.splitext(os.path.basename(actual_path))[0],
                "source_crs": "EPSG:4326",
                "geometry_type": geom_type,
                "total_features": total,
                "features": out_feats,
            }

        if ext == ".gpkg" and layer_name:
            extras = inspect_gpkg_sqlite_extras(actual_path)
            if any(table["layer_name"] == layer_name for table in extras["tables"]):
                return extract_gpkg_table(actual_path, layer_name, max_features, offset)

        if HAS_GEOPANDAS and HAS_PYOGRIO and ext in (".gpkg", ".shp", ".kml", ".gml", ".fgb"):
            read_kwargs = {"skip_features": int(offset), "max_features": int(max_features)}
            if layer_name:
                read_kwargs["layer"] = layer_name
            info = pyogrio.read_info(actual_path, layer=layer_name, force_feature_count=True)
            total = int(info.get("features", 0))
            source_crs = normalize_crs_str(info.get("crs")) or "EPSG:4326"
            gdf = pyogrio.read_dataframe(actual_path, **read_kwargs)

            has_geom = hasattr(gdf, "geometry") and gdf.geometry is not None and gdf._geometry_column_name in gdf.columns
            if has_geom and gdf.crs is not None:
                try:
                    epsg = gdf.crs.to_epsg()
                    if epsg != 4326:
                        gdf = gdf.to_crs(epsg=4326)
                except Exception:
                    try:
                        gdf = gdf.to_crs("EPSG:4326")
                    except Exception:
                        pass

            geom_col = gdf._geometry_column_name if has_geom else None
            prop_cols = [c for c in gdf.columns if c != geom_col]
            lat_col, lon_col = (None, None) if has_geom else detect_lat_lon_columns(prop_cols)
            out_feats = []
            geom_type = normalize_geom_type(info.get("geometry_type"))
            minx, miny, maxx, maxy = float("inf"), float("inf"), float("-inf"), float("-inf")

            for row_idx, row in gdf.iterrows():
                geom_dict = None
                if has_geom:
                    g = row[geom_col]
                    if hasattr(g, "is_empty") and not g.is_empty:
                        geom_dict = force_2d_geojson(json.loads(json.dumps(mapping(g))))
                        if geom_dict and geom_type in ("Unknown", "Geometry", "GeometryCollection", "Table", ""):
                            geom_type = normalize_geom_type(geom_dict.get("type"))
                        try:
                            b = g.bounds
                            if len(b) == 4:
                                minx = min(minx, b[0])
                                miny = min(miny, b[1])
                                maxx = max(maxx, b[2])
                                maxy = max(maxy, b[3])
                        except Exception:
                            pass
                elif lat_col and lon_col:
                    raw_lat = json_safe_value(row[lat_col])
                    raw_lon = json_safe_value(row[lon_col])
                    try:
                        lat_v = float(raw_lat)
                        lon_v = float(raw_lon)
                        if (
                            math.isfinite(lat_v)
                            and math.isfinite(lon_v)
                            and -90.0 <= lat_v <= 90.0
                            and -180.0 <= lon_v <= 180.0
                        ):
                            geom_dict = {"type": "Point", "coordinates": [lon_v, lat_v]}
                            geom_type = "Point"
                            minx = min(minx, lon_v)
                            miny = min(miny, lat_v)
                            maxx = max(maxx, lon_v)
                            maxy = max(maxy, lat_v)
                    except (TypeError, ValueError):
                        pass
                props = {}
                for col in prop_cols:
                    props[str(col)] = json_safe_value(row[col])
                fid_val = props.get("fid") or props.get("id") or props.get("OBJECTID") or (offset + int(row_idx) + 1)
                out_feats.append({
                    "feature_id": str(fid_val),
                    "geometry": geom_dict,
                    "properties": props,
                })

            bounds = [minx, miny, maxx, maxy] if minx <= maxx and miny <= maxy else None
            return {
                "success": True,
                "layer_name": layer_name or os.path.splitext(os.path.basename(actual_path))[0],
                "source_crs": source_crs,
                "geometry_type": geom_type,
                "total_features": total,
                "bounds": bounds,
                "features": out_feats,
            }

        return {"success": False, "error": f"Feature extraction not supported for {ext}"}
    except Exception as e:
        return {"success": False, "error": str(e)}
    finally:
        if tmp_dir:
            shutil.rmtree(tmp_dir, ignore_errors=True)


def reproject_geojson(geojson_str, source_crs, target_crs):
    if not HAS_RASTERIO:
        return {"success": False, "error": "rasterio/PROJ not available"}
    try:
        src = CRS.from_user_input(source_crs)
        dst = CRS.from_user_input(target_crs)
        obj = json.loads(geojson_str)
        if obj.get("type") == "FeatureCollection":
            out_features = []
            for feat in obj.get("features", []):
                geom = feat.get("geometry")
                if geom:
                    feat["geometry"] = transform_geom(src, dst, geom, precision=7)
                out_features.append(feat)
            obj["features"] = out_features
            return {"success": True, "data": obj}
        elif obj.get("type") == "Feature":
            if obj.get("geometry"):
                obj["geometry"] = transform_geom(src, dst, obj["geometry"], precision=7)
            return {"success": True, "data": obj}
        else:
            return {"success": True, "data": transform_geom(src, dst, obj, precision=7)}
    except Exception as e:
        return {"success": False, "error": str(e)}


def reproject_raster_file(src_path, dst_path, source_crs, target_crs):
    if not HAS_RASTERIO:
        return {"success": False, "error": "rasterio/GDAL Warp not available"}
    try:
        dst_crs = CRS.from_user_input(target_crs)
        with rasterio.open(src_path) as src:
            src_crs = src.crs
            if source_crs:
                src_crs = CRS.from_user_input(source_crs)
            if not src_crs:
                return {"success": False, "error": "Source raster has no CRS and source_epsg was not specified"}
            transform, width, height = calculate_default_transform(
                src_crs, dst_crs, src.width, src.height, *src.bounds
            )
            kwargs = src.meta.copy()
            kwargs.update({
                "crs": dst_crs,
                "transform": transform,
                "width": width,
                "height": height,
                "driver": "GTiff",
            })
            os.makedirs(os.path.dirname(os.path.abspath(dst_path)), exist_ok=True)
            with rasterio.open(dst_path, "w", **kwargs) as dst:
                for i in range(1, src.count + 1):
                    reproject(
                        source=rasterio.band(src, i),
                        destination=rasterio.band(dst, i),
                        src_transform=src.transform,
                        src_crs=src_crs,
                        dst_transform=transform,
                        dst_crs=dst_crs,
                        resampling=Resampling.bilinear,
                    )
                new_bounds = dst.bounds
                wgs84_bounds = transform_bounds(dst_crs, CRS.from_epsg(4326), *new_bounds)
                return {
                    "success": True,
                    "width": width,
                    "height": height,
                    "source_crs": src_crs.to_string(),
                    "target_crs": dst_crs.to_string(),
                    "bounds": [new_bounds.left, new_bounds.bottom, new_bounds.right, new_bounds.top],
                    "bounds_wgs84": list(wgs84_bounds),
                }
    except Exception as e:
        return {"success": False, "error": str(e)}


def generate_vector_tiles(src_path, out_dir, layer_name=None, min_zoom=0, max_zoom=16):
    try:
        if not HAS_PYOGRIO or not HAS_GEOPANDAS:
            return {"success": False, "error": "pyogrio and geopandas required for vector tile generation"}
        if not os.path.exists(src_path):
            return {"success": False, "error": f"Source file does not exist: {src_path}"}

        # Target directory must NOT exist beforehand because GDAL MVT driver creates it
        shutil.rmtree(out_dir, ignore_errors=True)
        parent_dir = os.path.dirname(os.path.abspath(out_dir))
        os.makedirs(parent_dir, exist_ok=True)

        read_kwargs = {}
        if layer_name:
            read_kwargs["layer"] = layer_name
        gdf = pyogrio.read_dataframe(src_path, **read_kwargs)
        if gdf.empty:
            return {"success": False, "error": "Vector dataset contains 0 features"}

        if gdf.crs is not None and not gdf.crs.equals("EPSG:4326"):
            gdf = gdf.to_crs("EPSG:4326")

        def _to_2d(geom):
            if geom is None or geom.is_empty:
                return geom
            if HAS_SHAPELY and hasattr(shapely, "force_2d"):
                return shapely.force_2d(geom)
            return geom

        gdf["geometry"] = gdf["geometry"].apply(_to_2d)

        # Convert non-serializable columns
        for col in gdf.columns:
            if col != "geometry" and col != gdf.geometry.name:
                dt = str(gdf[col].dtype).lower()
                if "datetime" in dt or "object" in dt:
                    gdf[col] = gdf[col].astype(str)

        pyogrio.write_dataframe(
            gdf,
            out_dir,
            driver="MVT",
            MINZOOM=int(min_zoom),
            MAXZOOM=int(max_zoom),
        )

        tile_count = 0
        for root, dirs, files in os.walk(out_dir):
            for f in files:
                if f.endswith(".pbf") or f.endswith(".mvt"):
                    tile_count += 1

        return {
            "success": True,
            "layer_name": layer_name or "vector",
            "feature_count": len(gdf),
            "tile_count": tile_count,
            "min_zoom": int(min_zoom),
            "max_zoom": int(max_zoom),
            "out_dir": out_dir,
        }
    except Exception as e:
        return {"success": False, "error": str(e)}


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"success": False, "error": "Missing command"}))
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "inspect":
        res = inspect_vector_dataset(sys.argv[2])
        print(json.dumps(res))
    elif cmd == "extract_layer":
        fp = sys.argv[2]
        lname = sys.argv[3] if len(sys.argv) > 3 and sys.argv[3] != "" else None
        max_f = int(sys.argv[4]) if len(sys.argv) > 4 else 5000
        off = int(sys.argv[5]) if len(sys.argv) > 5 else 0
        res = extract_layer_features(fp, lname, max_f, off)
        print(json.dumps(res))
    elif cmd == "reproject_geojson":
        res = reproject_geojson(sys.argv[2], sys.argv[3], sys.argv[4])
        print(json.dumps(res))
    elif cmd == "reproject_raster":
        res = reproject_raster_file(sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5])
        print(json.dumps(res))
    elif cmd == "generate_vector_tiles":
        fp = sys.argv[2]
        out_dir = sys.argv[3]
        lname = sys.argv[4] if len(sys.argv) > 4 and sys.argv[4] != "" else None
        min_z = int(sys.argv[5]) if len(sys.argv) > 5 else 0
        max_z = int(sys.argv[6]) if len(sys.argv) > 6 else 16
        res = generate_vector_tiles(fp, out_dir, lname, min_z, max_z)
        print(json.dumps(res))
    else:
        print(json.dumps({"success": False, "error": f"Unknown command: {cmd}"}))


if __name__ == "__main__":
    main()
