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


def _slugify_layer_name(name):
    import re
    s = re.sub(r"[^A-Za-z0-9._-]+", "_", str(name or "layer").strip())
    s = s.strip("._-").lower()
    if len(s) > 80:
        s = s[:80]
    return s or "layer"


def _build_vector_tile_style(layer_slug, display_name, geom_type, color="#3ecf8e", fields=None, min_zoom=0, max_zoom=14):
    """Generates a MapLibre-compatible style.json with interactive point/line/polygon layers."""
    gt = (geom_type or "").lower()
    popup_fields = [f["name"] for f in (fields or []) if isinstance(f, dict) and f.get("name") and not str(f["name"]).startswith("_")]
    layers = []

    if "polygon" in gt or gt in ("geometry", "unknown", ""):
        layers.append({
            "id": f"{layer_slug}-fill",
            "type": "fill",
            "source": layer_slug,
            "source-layer": layer_slug,
            "filter": ["in", ["geometry-type"], ["literal", ["Polygon", "MultiPolygon"]]],
            "paint": {
                "fill-color": color,
                "fill-opacity": 0.28,
                "fill-outline-color": color,
            },
        })
    if "line" in gt or "polygon" in gt or gt in ("geometry", "unknown", ""):
        layers.append({
            "id": f"{layer_slug}-line",
            "type": "line",
            "source": layer_slug,
            "source-layer": layer_slug,
            "filter": ["in", ["geometry-type"], ["literal", ["LineString", "MultiLineString", "Polygon", "MultiPolygon"]]],
            "paint": {
                "line-color": color,
                "line-width": 2.2,
                "line-opacity": 0.92,
            },
        })
    if "point" in gt or gt in ("geometry", "unknown", ""):
        layers.append({
            "id": f"{layer_slug}-circle",
            "type": "circle",
            "source": layer_slug,
            "source-layer": layer_slug,
            "filter": ["in", ["geometry-type"], ["literal", ["Point", "MultiPoint"]]],
            "paint": {
                "circle-color": color,
                "circle-radius": [
                    "interpolate", ["linear"], ["zoom"],
                    3, 4,
                    8, 5.5,
                    12, 7,
                    16, 9.5,
                ],
                "circle-stroke-color": "#ffffff",
                "circle-stroke-width": 2,
                "circle-opacity": 0.95,
            },
        })

    return {
        "version": 8,
        "name": display_name or layer_slug,
        "source_layer": layer_slug,
        "geometry_type": geom_type or "Geometry",
        "minzoom": int(min_zoom),
        "maxzoom": int(max_zoom),
        "color": color,
        "interaction": {
            "interactive": True,
            "clickable": True,
            "hoverable": True,
            "promote_id": "feature_id",
            "source_layer": layer_slug,
            "popup_fields": popup_fields,
            "hover_highlight": {
                "circle_radius_boost": 2.5,
                "circle_stroke_color": "#facc15",
                "circle_stroke_width": 3,
                "line_color": "#facc15",
                "line_width": 4,
                "fill_opacity": 0.45,
            },
        },
        "layers": layers,
    }


def _convert_single_layer_to_mvt(src_path, layer_root_dir, layer_name=None, min_zoom=0, max_zoom=14, color="#3ecf8e"):
    """
    Converts a single vector layer into:
      <layer_root_dir>/
        metadata.json
        style.json
        tiles/
          {z}/{x}/{y}.pbf
    Reprojects geometries to EPSG:3857 for MVT tile generation while preserving
    original CRS in metadata, feature IDs, geometry types, and all attributes.
    """
    if not HAS_PYOGRIO or not HAS_GEOPANDAS:
        return {"success": False, "error": "pyogrio and geopandas required for vector tile generation"}
    if not os.path.exists(src_path):
        return {"success": False, "error": f"Source file does not exist: {src_path}"}

    ext = os.path.splitext(src_path)[1].lower()
    read_kwargs = {"fid_as_index": True}
    actual_layer_name = layer_name
    if layer_name and ext in (".gpkg", ".kml", ".gml", ".fgb"):
        try:
            available_layers = [str(r[0]) for r in pyogrio.list_layers(src_path)]
            if layer_name in available_layers:
                read_kwargs["layer"] = layer_name
                actual_layer_name = layer_name
            else:
                target_low = layer_name.lower().replace("-", "_").replace(" ", "_")
                for av in available_layers:
                    av_low = av.lower().replace("-", "_").replace(" ", "_")
                    if av_low == target_low or target_low in av_low or av_low in target_low:
                        read_kwargs["layer"] = av
                        actual_layer_name = av
                        break
        except Exception:
            pass

    source_crs = ""
    try:
        info_kwargs = {"layer": read_kwargs["layer"]} if "layer" in read_kwargs else {}
        info = pyogrio.read_info(src_path, force_feature_count=True, **info_kwargs)
        source_crs = normalize_crs_str(info.get("crs"))
    except Exception:
        info = {}

    try:
        gdf = pyogrio.read_dataframe(src_path, **read_kwargs)
    except Exception:
        read_kwargs.pop("fid_as_index", None)
        gdf = pyogrio.read_dataframe(src_path, **read_kwargs)

    # Handle non-spatial attribute tables with lat/lon columns
    has_geom = hasattr(gdf, "geometry") and gdf.geometry is not None and gdf._geometry_column_name in gdf.columns
    if not has_geom:
        lat_col, lon_col = detect_lat_lon_columns(list(gdf.columns))
        if lat_col and lon_col and HAS_SHAPELY:
            pts = []
            valid_mask = []
            for _, row in gdf.iterrows():
                try:
                    lat_v = float(row[lat_col])
                    lon_v = float(row[lon_col])
                    if math.isfinite(lat_v) and math.isfinite(lon_v) and -90 <= lat_v <= 90 and -180 <= lon_v <= 180:
                        pts.append(Point(lon_v, lat_v))
                        valid_mask.append(True)
                    else:
                        valid_mask.append(False)
                except Exception:
                    valid_mask.append(False)
            gdf = gdf.loc[valid_mask].copy()
            gdf = gpd.GeoDataFrame(gdf, geometry=pts, crs="EPSG:4326")
            source_crs = source_crs or "EPSG:4326"
            has_geom = True
        else:
            return {"success": False, "error": f"Layer '{actual_layer_name or src_path}' has no geometry"}

    if gdf.empty:
        return {"success": False, "error": "Vector dataset contains 0 features"}

    if not source_crs and gdf.crs is not None:
        source_crs = normalize_crs_str(gdf.crs)
    if not source_crs:
        source_crs = "EPSG:4326"
    if gdf.crs is None:
        gdf = gdf.set_crs(source_crs, allow_override=True)

    # Preserve original feature IDs cleanly in attributes for MVT queryRenderedFeatures
    orig_fids = [str(idx) for idx in gdf.index.tolist()]
    gdf = gdf.reset_index(drop=True)
    if "feature_id" not in gdf.columns:
        gdf["feature_id"] = orig_fids
    gdf["_source_feature_id"] = orig_fids
    layer_slug = _slugify_layer_name(actual_layer_name or os.path.splitext(os.path.basename(src_path))[0])
    gdf["_source_layer"] = layer_slug

    # Force 2D and repair invalid geometries (vectorized for high throughput on large GeoPackages)
    geom_col = gdf.geometry.name
    gdf = gdf[gdf[geom_col].notnull() & ~gdf[geom_col].is_empty].copy()
    if gdf.empty:
        return {"success": False, "error": "Vector dataset contains 0 valid non-empty geometries"}

    if HAS_SHAPELY and hasattr(shapely, "force_2d"):
        try:
            gdf[geom_col] = shapely.force_2d(gdf[geom_col].values)
        except Exception:
            gdf[geom_col] = gdf[geom_col].apply(lambda g: shapely.force_2d(g) if g is not None and not g.is_empty else g)

    # Detect all geometry types present (vectorized unique)
    observed_types = []
    seen_t = set()
    for gt in gdf[geom_col].geom_type.unique().tolist():
        norm_t = normalize_geom_type(gt)
        if norm_t not in seen_t:
            seen_t.add(norm_t)
            observed_types.append(norm_t)

    # Only run make_valid on non-point layers if any invalid geometries exist
    if HAS_SHAPELY and hasattr(shapely, "make_valid") and not set(observed_types).issubset({"Point", "MultiPoint"}):
        try:
            invalid_mask = ~gdf[geom_col].is_valid
            if invalid_mask.any():
                gdf.loc[invalid_mask, geom_col] = shapely.make_valid(gdf.loc[invalid_mask, geom_col].values)
        except Exception:
            pass

    primary_geom_type = observed_types[0] if len(observed_types) == 1 else (
        "Point" if set(observed_types).issubset({"Point", "MultiPoint"}) else
        "LineString" if set(observed_types).issubset({"LineString", "MultiLineString"}) else
        "Polygon" if set(observed_types).issubset({"Polygon", "MultiPolygon"}) else
        "Geometry"
    )

    # Compute WGS84 bounds for metadata and MapLibre source bounds
    gdf_wgs84 = gdf.to_crs("EPSG:4326") if gdf.crs.to_epsg() != 4326 else gdf
    tb = gdf_wgs84.total_bounds
    wgs84_bounds = [float(tb[0]), float(tb[1]), float(tb[2]), float(tb[3])] if len(tb) == 4 and all(math.isfinite(x) for x in tb) else None

    # Reproject explicitly to EPSG:3857 for MVT tile generation
    gdf_3857 = gdf.to_crs("EPSG:3857") if gdf.crs.to_epsg() != 3857 else gdf

    # Collect attribute schema and sanitize non-serializable types (vectorized)
    fields_meta = []
    for col in gdf_3857.columns:
        if col == geom_col:
            continue
        dt = str(gdf_3857[col].dtype).lower()
        norm_t = "string"
        if "int" in dt:
            norm_t = "integer"
        elif "float" in dt or "double" in dt:
            norm_t = "float"
        elif "bool" in dt:
            norm_t = "boolean"
        if not str(col).startswith("_"):
            fields_meta.append({"name": str(col), "type": norm_t})
        if "datetime" in dt or "object" in dt:
            gdf_3857[col] = gdf_3857[col].fillna("").astype(str)

    # Adaptive max zoom keeps large/wide-extent GeoPackages fast while MapLibre overzooms cleanly to z24
    max_zoom_val = min(max(int(max_zoom), 0), 14)
    if wgs84_bounds and len(wgs84_bounds) == 4:
        span = max(abs(wgs84_bounds[2] - wgs84_bounds[0]), abs(wgs84_bounds[3] - wgs84_bounds[1]))
        if span > 10.0:
            max_zoom_val = min(max_zoom_val, 6)
        elif span > 2.0:
            max_zoom_val = min(max_zoom_val, 8)
        elif span > 0.25:
            max_zoom_val = min(max_zoom_val, 10)
        else:
            max_zoom_val = min(max_zoom_val, 12)
    if len(gdf_3857) > 10000:
        max_zoom_val = min(max_zoom_val, 6)
    elif len(gdf_3857) > 5000:
        max_zoom_val = min(max_zoom_val, 8)
    min_zoom_val = min(max(int(min_zoom), 0), max_zoom_val)

    # Prepare <layer_root_dir>/tiles directory (GDAL MVT creates the target dir)
    tiles_dir = os.path.join(layer_root_dir, "tiles")
    shutil.rmtree(tiles_dir, ignore_errors=True)
    os.makedirs(layer_root_dir, exist_ok=True)

    pyogrio.write_dataframe(
        gdf_3857,
        tiles_dir,
        driver="MVT",
        layer=layer_slug,
        MINZOOM=min_zoom_val,
        MAXZOOM=max_zoom_val,
        COMPRESS="YES",
    )

    tile_count = 0
    zoom_levels = set()
    for root, dirs, files in os.walk(tiles_dir):
        for f in files:
            if f.endswith(".pbf") or f.endswith(".mvt"):
                tile_count += 1
                rel = os.path.relpath(os.path.join(root, f), tiles_dir)
                parts = rel.replace("\\", "/").split("/")
                if len(parts) >= 3 and parts[0].isdigit():
                    zoom_levels.add(int(parts[0]))

    # Remove GDAL's internal tiles/metadata.json so tiles/ only holds z/x/y.pbf
    gdal_meta_path = os.path.join(tiles_dir, "metadata.json")
    gdal_tile_meta = None
    if os.path.exists(gdal_meta_path):
        try:
            with open(gdal_meta_path, "r", encoding="utf-8") as mf:
                gdal_tile_meta = json.load(mf)
        except Exception:
            pass
        try:
            os.remove(gdal_meta_path)
        except Exception:
            pass

    center = None
    if wgs84_bounds and len(wgs84_bounds) == 4:
        center = [(wgs84_bounds[0] + wgs84_bounds[2]) / 2.0, (wgs84_bounds[1] + wgs84_bounds[3]) / 2.0]

    metadata_doc = {
        "layer_slug": layer_slug,
        "layer_name": actual_layer_name or layer_slug,
        "source_layer": layer_slug,
        "format": "pbf",
        "encoding": "gzip",
        "tile_scheme": "xyz",
        "tile_crs": "EPSG:3857",
        "source_crs": source_crs,
        "bounds_crs": "EPSG:4326",
        "bounds": wgs84_bounds,
        "center": center,
        "min_zoom": min_zoom_val,
        "max_zoom": max_zoom_val,
        "available_zooms": sorted(zoom_levels),
        "geometry_type": primary_geom_type,
        "geometry_types": observed_types,
        "feature_count": int(len(gdf_3857)),
        "tile_count": tile_count,
        "fields": fields_meta,
        "promote_id": "feature_id",
        "interactive": True,
        "tiles_path": "tiles/{z}/{x}/{y}.pbf",
        "style_path": "style.json",
    }
    if gdal_tile_meta is not None:
        metadata_doc["mvt_generator_metadata"] = gdal_tile_meta

    metadata_path = os.path.join(layer_root_dir, "metadata.json")
    with open(metadata_path, "w", encoding="utf-8") as mf:
        json.dump(metadata_doc, mf, indent=2)

    style_doc = _build_vector_tile_style(
        layer_slug=layer_slug,
        display_name=actual_layer_name or layer_slug,
        geom_type=primary_geom_type,
        color=color,
        fields=fields_meta,
        min_zoom=min_zoom_val,
        max_zoom=max_zoom_val,
    )
    style_path = os.path.join(layer_root_dir, "style.json")
    with open(style_path, "w", encoding="utf-8") as sf:
        json.dump(style_doc, sf, indent=2)

    return {
        "success": True,
        "layer_slug": layer_slug,
        "layer_name": actual_layer_name or layer_slug,
        "source_layer": layer_slug,
        "source_crs": source_crs,
        "tile_crs": "EPSG:3857",
        "geometry_type": primary_geom_type,
        "geometry_types": observed_types,
        "feature_count": int(len(gdf_3857)),
        "tile_count": tile_count,
        "bounds": wgs84_bounds,
        "center": center,
        "min_zoom": min_zoom_val,
        "max_zoom": max_zoom_val,
        "fields": fields_meta,
        "layer_dir": layer_root_dir,
        "tiles_dir": tiles_dir,
        "metadata_path": metadata_path,
        "style_path": style_path,
    }


def convert_gpkg_to_vector_tiles(src_path, vector_tiles_root, layer_names=None, min_zoom=0, max_zoom=14):
    """
    Converts all (or specified) internal vector layers of a GeoPackage or vector dataset into:
      <vector_tiles_root>/
        <layer_slug>/
          metadata.json
          style.json
          tiles/{z}/{x}/{y}.pbf
    """
    try:
        if not HAS_PYOGRIO or not HAS_GEOPANDAS:
            return {"success": False, "error": "pyogrio and geopandas required for vector tile generation"}
        if not os.path.exists(src_path):
            return {"success": False, "error": f"Source file does not exist: {src_path}"}

        os.makedirs(vector_tiles_root, exist_ok=True)
        palette = ["#3ecf8e", "#3b82f6", "#f59e0b", "#ec4899", "#8b5cf6", "#06b6d4", "#ef4444", "#84cc16"]

        target_layers = []
        if layer_names:
            target_layers = [ln.strip() for ln in layer_names if ln and ln.strip()]
        else:
            try:
                raw_lyrs = pyogrio.list_layers(src_path)
                target_layers = [str(r[0]) for r in raw_lyrs if r is not None and len(r) > 0]
            except Exception:
                target_layers = [None]

        results = []
        errors = []
        for idx, lname in enumerate(target_layers):
            slug = _slugify_layer_name(lname or os.path.splitext(os.path.basename(src_path))[0])
            layer_dir = os.path.join(vector_tiles_root, slug)
            color = palette[idx % len(palette)]
            res = _convert_single_layer_to_mvt(
                src_path=src_path,
                layer_root_dir=layer_dir,
                layer_name=lname,
                min_zoom=min_zoom,
                max_zoom=max_zoom,
                color=color,
            )
            if res.get("success"):
                results.append(res)
            else:
                errors.append({"layer_name": lname or slug, "error": res.get("error", "conversion failed")})

        if not results:
            return {"success": False, "error": errors[0]["error"] if errors else "No vector layers could be converted", "errors": errors}

        return {
            "success": True,
            "vector_tiles_root": vector_tiles_root,
            "layers": results,
            "errors": errors,
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

        read_kwargs = {"fid_as_index": True}
        if layer_name:
            # Check if layer_name matches an actual layer in the GeoPackage/vector container
            try:
                available_layers = [str(r[0]) for r in pyogrio.list_layers(src_path)]
                if layer_name in available_layers:
                    read_kwargs["layer"] = layer_name
                else:
                    target_low = layer_name.lower().replace("-", "_").replace(" ", "_")
                    for av in available_layers:
                        av_low = av.lower().replace("-", "_").replace(" ", "_")
                        if av_low == target_low or target_low in av_low or av_low in target_low:
                            read_kwargs["layer"] = av
                            break
            except Exception:
                pass

        try:
            gdf = pyogrio.read_dataframe(src_path, **read_kwargs)
        except Exception:
            read_kwargs.pop("fid_as_index", None)
            gdf = pyogrio.read_dataframe(src_path, **read_kwargs)
        if gdf.empty:
            return {"success": False, "error": "Vector dataset contains 0 features"}

        orig_fids = [str(idx) for idx in gdf.index.tolist()]
        gdf = gdf.reset_index(drop=True)
        if "feature_id" not in gdf.columns:
            gdf["feature_id"] = orig_fids
        gdf["_source_feature_id"] = orig_fids

        layer_slug = _slugify_layer_name(layer_name or os.path.basename(os.path.normpath(out_dir)))
        gdf["_source_layer"] = layer_slug

        source_crs = normalize_crs_str(gdf.crs) if gdf.crs is not None else "EPSG:4326"
        if gdf.crs is None:
            gdf = gdf.set_crs("EPSG:4326", allow_override=True)

        def _to_2d(geom):
            if geom is None or geom.is_empty:
                return geom
            if HAS_SHAPELY:
                if hasattr(shapely, "force_2d"):
                    geom = shapely.force_2d(geom)
                if hasattr(shapely, "make_valid") and not geom.is_valid:
                    geom = shapely.make_valid(geom)
            return geom

        gdf["geometry"] = gdf["geometry"].apply(_to_2d)

        # Detect primary geometry type
        geom_type = "Unknown"
        non_empty = gdf[gdf.geometry.notnull() & ~gdf.geometry.is_empty]
        if not non_empty.empty:
            first_gtype = str(non_empty.geometry.iloc[0].geom_type)
            if "Point" in first_gtype:
                geom_type = "Point"
            elif "Line" in first_gtype:
                geom_type = "LineString"
            elif "Polygon" in first_gtype:
                geom_type = "Polygon"
            else:
                geom_type = first_gtype

        # Convert non-serializable columns
        for col in gdf.columns:
            if col != "geometry" and col != gdf.geometry.name:
                dt = str(gdf[col].dtype).lower()
                if "datetime" in dt or "object" in dt:
                    gdf[col] = gdf[col].astype(str)

        gdf_wgs84 = gdf.to_crs("EPSG:4326") if gdf.crs.to_epsg() != 4326 else gdf
        gdf_3857 = gdf.to_crs("EPSG:3857") if gdf.crs.to_epsg() != 3857 else gdf

        # In standard MVT specifications (Mapbox/MapLibre), vector tiles are generated up to
        # zoom 14 (or 15). MapLibre GL natively overzooms vector tiles smoothly to zoom 24+
        # with zero distortion, avoiding exponential tile generation beyond zoom 14.
        max_zoom_val = min(int(max_zoom), 14)
        min_zoom_val = min(int(min_zoom), max_zoom_val)
        if min_zoom_val < 0:
            min_zoom_val = 0

        pyogrio.write_dataframe(
            gdf_3857,
            out_dir,
            driver="MVT",
            layer=layer_slug,
            MINZOOM=min_zoom_val,
            MAXZOOM=max_zoom_val,
            COMPRESS="YES",
        )

        tile_count = 0
        for root, dirs, files in os.walk(out_dir):
            for f in files:
                if f.endswith(".pbf") or f.endswith(".mvt"):
                    tile_count += 1

        # Also write GeoJSON for this vector layer into sibling geojson directory
        geojson_dir = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(out_dir))), "geojson")
        os.makedirs(geojson_dir, exist_ok=True)
        geojson_path = os.path.join(geojson_dir, f"{layer_slug}.geojson")
        has_geojson = False
        try:
            gdf_wgs84.to_file(geojson_path, driver="GeoJSON")
            has_geojson = os.path.exists(geojson_path)
        except Exception:
            pass

        return {
            "success": True,
            "layer_name": layer_name or layer_slug,
            "source_layer": layer_slug,
            "source_crs": source_crs,
            "feature_count": len(gdf_3857),
            "tile_count": tile_count,
            "geometry_type": geom_type,
            "geojson_file": geojson_path if has_geojson else "",
            "min_zoom": min_zoom_val,
            "max_zoom": max_zoom_val,
            "out_dir": out_dir,
        }
    except Exception as e:
        return {"success": False, "error": str(e)}


def _parse_hex_rgba(hex_color, opacity=1.0):
    c = str(hex_color or "#3ecf8e").strip().lstrip("#")
    if len(c) == 3:
        c = "".join(ch * 2 for ch in c)
    if len(c) != 6:
        c = "3ecf8e"
    try:
        r = int(c[0:2], 16)
        g = int(c[2:4], 16)
        b = int(c[4:6], 16)
    except Exception:
        r, g, b = 62, 207, 142
    a = max(0, min(255, int(round(float(opacity) * 255.0))))
    return (r, g, b, a)


def _lonlat_to_tile(lon, lat, z):
    lat_c = max(-85.05112878, min(85.05112878, lat))
    n = 2 ** z
    x = int(math.floor((lon + 180.0) / 360.0 * n))
    lat_rad = math.radians(lat_c)
    y = int(math.floor((1.0 - math.log(math.tan(lat_rad) + 1.0 / math.cos(lat_rad)) / math.pi) / 2.0 * n))
    return max(0, min(n - 1, x)), max(0, min(n - 1, y))


def _tile_bounds_3857(z, x, y):
    ORIGIN_SHIFT = 20037508.342789244
    n = 2 ** z
    tile_span = (2.0 * ORIGIN_SHIFT) / n
    minx = -ORIGIN_SHIFT + x * tile_span
    maxx = minx + tile_span
    maxy = ORIGIN_SHIFT - y * tile_span
    miny = maxy - tile_span
    return (minx, miny, maxx, maxy)


def rasterize_vector_layer_to_tiles(
    filepath,
    out_root,
    layer_name=None,
    min_zoom=0,
    max_zoom=12,
    tile_format="png",
    color_hex="#3ecf8e",
    fill_opacity=0.35,
    stroke_width=2.0,
):
    """Rasterizes polygon/line features of a vector layer into per-tile 256x256 PNG/JPG XYZ tiles
    (extensionless {z}/{x}/{y} matching MapTiler Engine reference format) and exports point features
    to a clean EPSG:4326 GeoJSON FeatureCollection."""
    if not HAS_GEOPANDAS or not HAS_SHAPELY:
        return {"success": False, "error": "geopandas and shapely are required"}
    try:
        from PIL import Image, ImageDraw
        from shapely.strtree import STRtree
    except Exception as e:
        return {"success": False, "error": f"Pillow / STRtree unavailable: {e}"}

    try:
        read_kwargs = {}
        if layer_name:
            read_kwargs["layer"] = layer_name
        gdf = gpd.read_file(filepath, **read_kwargs)
        if gdf is None or len(gdf) == 0:
            return {"success": True, "has_raster_tiles": False, "has_points": False, "tile_count": 0, "point_count": 0}

        gdf = gdf[gdf.geometry.notnull() & ~gdf.geometry.is_empty].copy()
        if len(gdf) == 0:
            return {"success": True, "has_raster_tiles": False, "has_points": False, "tile_count": 0, "point_count": 0}

        if gdf.crs is None:
            gdf = gdf.set_crs("EPSG:4326")
        gdf_wgs84 = gdf.to_crs("EPSG:4326")
        gdf_3857 = gdf_wgs84.to_crs("EPSG:3857")

        base_name = layer_name or os.path.splitext(os.path.basename(filepath))[0]
        layer_slug = "".join(ch if (ch.isalnum() or ch in ("_", "-")) else "_" for ch in base_name.strip().lower()).strip("_") or "layer"

        # Split into Points vs Polygons/Lines
        point_mask = gdf_wgs84.geometry.geom_type.isin(["Point", "MultiPoint"])
        gdf_points = gdf_wgs84[point_mask]
        gdf_shapes_3857 = gdf_3857[~point_mask]
        gdf_shapes_wgs84 = gdf_wgs84[~point_mask]

        os.makedirs(out_root, exist_ok=True)
        points_geojson_path = ""
        point_count = 0

        if len(gdf_points) > 0:
            pts_dir = os.path.join(out_root, "points")
            os.makedirs(pts_dir, exist_ok=True)
            points_geojson_path = os.path.join(pts_dir, f"{layer_slug}.geojson")
            features = []
            for idx, row in gdf_points.iterrows():
                geom = row.geometry
                props = {}
                for col in gdf_points.columns:
                    if col == "geometry":
                        continue
                    props[str(col)] = json_safe_value(row[col])
                fid = props.get("feature_id") or props.get("fid") or props.get("id") or f"{layer_slug}_{idx}"
                props["feature_id"] = str(fid)
                props["_source_layer"] = layer_slug
                props["_layer_name"] = base_name
                props["_color"] = color_hex
                features.append({
                    "type": "Feature",
                    "id": str(fid),
                    "geometry": mapping(geom),
                    "properties": props,
                })
            fc = {
                "type": "FeatureCollection",
                "name": base_name,
                "crs": {"type": "name", "properties": {"name": "urn:ogc:def:crs:OGC:1.3:CRS84"}},
                "features": features,
            }
            with open(points_geojson_path, "w", encoding="utf-8") as f:
                json.dump(fc, f, ensure_ascii=False, indent=2)
            point_count = len(features)

        tile_dir = ""
        tile_count = 0
        out_bounds = [float(v) for v in gdf_wgs84.total_bounds]
        actual_min_z = max(0, int(min_zoom))
        actual_max_z = max(actual_min_z, min(18, int(max_zoom)))

        if len(gdf_shapes_3857) > 0:
            out_bounds = [float(v) for v in gdf_shapes_wgs84.total_bounds]
            w, s, e, n = out_bounds
            # Guard against runaway tile counts on very wide extents
            MAX_TILES = 600
            while actual_max_z > actual_min_z:
                est = 0
                for z in range(actual_min_z, actual_max_z + 1):
                    x0, y0 = _lonlat_to_tile(w, n, z)
                    x1, y1 = _lonlat_to_tile(e, s, z)
                    est += (abs(x1 - x0) + 1) * (abs(y1 - y0) + 1)
                if est <= MAX_TILES:
                    break
                actual_max_z -= 1

            tile_dir = os.path.join(out_root, "tiles", layer_slug)
            os.makedirs(tile_dir, exist_ok=True)

            geoms_3857 = list(gdf_shapes_3857.geometry.values)
            tree = STRtree(geoms_3857)

            fill_rgba = _parse_hex_rgba(color_hex, fill_opacity)
            stroke_rgba = _parse_hex_rgba(color_hex, 0.95)
            fmt_lower = "jpg" if str(tile_format).lower() in ("jpg", "jpeg") else "png"
            SS = 2  # 2x supersampling for crisp anti-aliased strokes
            SIZE = 256 * SS
            sw = max(1, int(round(float(stroke_width) * SS)))

            def to_px(x_val, y_val, minx, miny, span):
                px = (x_val - minx) / span * SIZE
                py = (1.0 - (y_val - miny) / span) * SIZE
                return (px, py)

            def draw_geom(draw_obj, g, minx, miny, span):
                gtype = g.geom_type
                if gtype == "Polygon":
                    ext = [to_px(x, y, minx, miny, span) for x, y in g.exterior.coords]
                    if len(ext) >= 3:
                        draw_obj.polygon(ext, fill=fill_rgba, outline=stroke_rgba)
                        draw_obj.line(ext, fill=stroke_rgba, width=sw)
                    for interior in g.interiors:
                        ring = [to_px(x, y, minx, miny, span) for x, y in interior.coords]
                        if len(ring) >= 3:
                            draw_obj.polygon(ring, fill=(0, 0, 0, 0), outline=stroke_rgba)
                            draw_obj.line(ring, fill=stroke_rgba, width=sw)
                elif gtype == "MultiPolygon":
                    for poly in g.geoms:
                        draw_geom(draw_obj, poly, minx, miny, span)
                elif gtype == "LineString":
                    pts = [to_px(x, y, minx, miny, span) for x, y in g.coords]
                    if len(pts) >= 2:
                        draw_obj.line(pts, fill=stroke_rgba, width=sw)
                elif gtype == "MultiLineString":
                    for ls in g.geoms:
                        draw_geom(draw_obj, ls, minx, miny, span)
                elif gtype == "GeometryCollection":
                    for sub in g.geoms:
                        draw_geom(draw_obj, sub, minx, miny, span)

            for z in range(actual_min_z, actual_max_z + 1):
                x0, y0 = _lonlat_to_tile(w, n, z)
                x1, y1 = _lonlat_to_tile(e, s, z)
                for tx in range(min(x0, x1), max(x0, x1) + 1):
                    for ty in range(min(y0, y1), max(y0, y1) + 1):
                        tminx, tminy, tmaxx, tmaxy = _tile_bounds_3857(z, tx, ty)
                        tile_box = box(tminx, tminy, tmaxx, tmaxy)
                        cand_idx = tree.query(tile_box, predicate="intersects")
                        if len(cand_idx) == 0:
                            continue

                        img = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
                        draw = ImageDraw.Draw(img, "RGBA")
                        span = tmaxx - tminx
                        for ci in cand_idx:
                            draw_geom(draw, geoms_3857[int(ci)], tminx, tminy, span)

                        out_img = img.resize((256, 256), Image.Resampling.LANCZOS)
                        # Skip empty transparent tiles
                        if out_img.getbbox() is None:
                            continue

                        z_x_dir = os.path.join(tile_dir, str(z), str(tx))
                        os.makedirs(z_x_dir, exist_ok=True)
                        tile_path = os.path.join(z_x_dir, str(ty))
                        if fmt_lower == "jpg":
                            bg = Image.new("RGB", (256, 256), (255, 255, 255))
                            bg.paste(out_img, mask=out_img.split()[3])
                            bg.save(tile_path, format="JPEG", quality=88)
                        else:
                            out_img.save(tile_path, format="PNG", optimize=True)
                        tile_count += 1

        return {
            "success": True,
            "layer_name": base_name,
            "layer_slug": layer_slug,
            "has_raster_tiles": tile_count > 0,
            "raster_tile_dir": tile_dir if tile_count > 0 else "",
            "tile_count": tile_count,
            "min_zoom": actual_min_z,
            "max_zoom": actual_max_z,
            "bounds": out_bounds,
            "format": tile_format,
            "has_points": point_count > 0,
            "point_count": point_count,
            "points_geojson_path": points_geojson_path,
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
    elif cmd == "convert_gpkg_tiles":
        fp = sys.argv[2]
        vt_root = sys.argv[3]
        raw_layers = sys.argv[4] if len(sys.argv) > 4 and sys.argv[4] != "" else ""
        layer_list = [x.strip() for x in raw_layers.split(",") if x.strip()] if raw_layers else None
        min_z = int(sys.argv[5]) if len(sys.argv) > 5 else 0
        max_z = int(sys.argv[6]) if len(sys.argv) > 6 else 14
        res = convert_gpkg_to_vector_tiles(fp, vt_root, layer_list, min_z, max_z)
        print(json.dumps(res))
    elif cmd == "rasterize_vector_layers":
        fp = sys.argv[2]
        out_root = sys.argv[3]
        lname = sys.argv[4] if len(sys.argv) > 4 and sys.argv[4] != "" else None
        min_z = int(sys.argv[5]) if len(sys.argv) > 5 else 0
        max_z = int(sys.argv[6]) if len(sys.argv) > 6 else 12
        fmt = sys.argv[7] if len(sys.argv) > 7 and sys.argv[7] != "" else "png"
        color = sys.argv[8] if len(sys.argv) > 8 and sys.argv[8] != "" else "#3ecf8e"
        fill_op = float(sys.argv[9]) if len(sys.argv) > 9 else 0.35
        sw = float(sys.argv[10]) if len(sys.argv) > 10 else 2.0
        res = rasterize_vector_layer_to_tiles(fp, out_root, lname, min_z, max_z, fmt, color, fill_op, sw)
        print(json.dumps(res))
    else:
        print(json.dumps({"success": False, "error": f"Unknown command: {cmd}"}))


if __name__ == "__main__":
    main()
