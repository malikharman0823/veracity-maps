#!/usr/bin/env python3
"""
Convert all layers (polygons, lines, and points) inside one or more GeoPackage (.gpkg)
or vector files into a single multi-layer Vector MBTiles (.mbtiles, format=pbf, MBTiles 1.3).

Usage:
    python gpkg_to_vector_mbtiles.py <input.gpkg> <output.mbtiles> [min_zoom] [max_zoom] [name]
"""

import gzip
import json
import math
import os
import re
import shutil
import sqlite3
import sys
import tempfile
import warnings

warnings.filterwarnings("ignore")

import geopandas as gpd
import pyogrio
from shapely import force_2d


def safe_slug(name: str) -> str:
    s = re.sub(r"[^a-zA-Z0-9_-]+", "_", str(name or "layer").strip().lower()).strip("_")
    return s or "layer"


def normalize_crs(crs_obj) -> str:
    if crs_obj is None:
        return "EPSG:4326"
    try:
        epsg = crs_obj.to_epsg()
        if epsg:
            return f"EPSG:{epsg}"
    except Exception:
        pass
    s = str(crs_obj).strip()
    return s or "EPSG:4326"


def field_type_label(dtype) -> str:
    s = str(dtype).lower()
    if "int" in s or "float" in s or "double" in s:
        return "Number"
    if "bool" in s:
        return "Boolean"
    return "String"


def convert_gpkg_to_vector_mbtiles(
    input_path: str,
    output_path: str,
    min_zoom: int = 0,
    max_zoom: int = 12,
    map_name: str = None,
) -> dict:
    if not os.path.exists(input_path):
        return {"success": False, "error": f"Input file not found: {input_path}"}

    out_dir = os.path.dirname(os.path.abspath(output_path))
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    if os.path.exists(output_path):
        os.remove(output_path)

    ext = os.path.splitext(input_path)[1].lower()
    if ext == ".gpkg":
        try:
            raw_layers = pyogrio.list_layers(input_path)
            layer_names = [str(row[0]) for row in raw_layers]
        except Exception as e:
            return {"success": False, "error": f"Failed listing GeoPackage layers: {e}"}
    else:
        layer_names = [None]

    tmp_root = tempfile.mkdtemp(prefix="veracity_vec_mbtiles_")
    merged_tiles = {}  # (z, x, y) -> bytearray of concatenated raw MVT protobuf layers
    vector_layers_meta = []
    used_slugs = {}
    min_x, min_y, max_x, max_y = float("inf"), float("inf"), float("-inf"), float("-inf")
    actual_min_z = None
    actual_max_z = None
    total_features = 0

    try:
        for lname in layer_names:
            try:
                read_kwargs = {"layer": lname} if lname else {}
                gdf = gpd.read_file(input_path, **read_kwargs)
            except Exception:
                continue

            if gdf is None or len(gdf) == 0:
                continue

            gdf = gdf[gdf.geometry.notnull() & ~gdf.geometry.is_empty].copy()
            if len(gdf) == 0:
                continue

            source_crs = normalize_crs(gdf.crs)
            if gdf.crs is None:
                gdf = gdf.set_crs("EPSG:4326")

            gdf_wgs84 = gdf.to_crs("EPSG:4326")
            try:
                gdf_wgs84["geometry"] = force_2d(gdf_wgs84.geometry)
            except Exception:
                pass
            gdf_3857 = gdf_wgs84.to_crs("EPSG:3857")

            display_name = lname or os.path.splitext(os.path.basename(input_path))[0]
            base_slug = safe_slug(display_name)
            used_slugs[base_slug] = used_slugs.get(base_slug, 0) + 1
            slug = base_slug if used_slugs[base_slug] == 1 else f"{base_slug}_{used_slugs[base_slug]}"

            # Ensure clean feature_id and source-layer attributes
            if "feature_id" not in gdf_3857.columns:
                gdf_3857["feature_id"] = [f"{slug}_{i}" for i in range(len(gdf_3857))]
            gdf_3857["_source_layer"] = slug
            gdf_3857["_layer_name"] = display_name
            gdf_3857["_source_crs"] = source_crs

            # Coerce non-primitive columns to strings for GDAL MVT driver safety
            fields_meta = {}
            for col in list(gdf_3857.columns):
                if col == "geometry":
                    continue
                if str(gdf_3857[col].dtype) == "object":
                    gdf_3857[col] = gdf_3857[col].apply(lambda v: None if v is None else str(v))
                fields_meta[str(col)] = field_type_label(gdf_3857[col].dtype)

            bounds = [float(v) for v in gdf_wgs84.total_bounds]
            if len(bounds) == 4 and all(math.isfinite(v) for v in bounds):
                min_x = min(min_x, bounds[0])
                min_y = min(min_y, bounds[1])
                max_x = max(max_x, bounds[2])
                max_y = max(max_y, bounds[3])

            # Adaptive zoom cap for very large layers so export stays fast
            layer_min_z = max(0, int(min_zoom))
            layer_max_z = max(layer_min_z, min(14, int(max_zoom)))
            if len(gdf_3857) > 10000 and layer_max_z > 8:
                layer_max_z = 8

            layer_mvt_dir = os.path.join(tmp_root, slug)
            try:
                pyogrio.write_dataframe(
                    gdf_3857,
                    layer_mvt_dir,
                    driver="MVT",
                    layer=slug,
                    dataset_options={
                        "MINZOOM": str(layer_min_z),
                        "MAXZOOM": str(layer_max_z),
                        "COMPRESS": "NO",
                        "TILE_EXTENSION": "pbf",
                        "MAX_FEATURES": "500000",
                        "MAX_SIZE": "2000000",
                    },
                )
            except Exception:
                continue

            layer_tile_count = 0
            for root_d, _, files in os.walk(layer_mvt_dir):
                for fn in files:
                    if not fn.endswith(".pbf"):
                        continue
                    full_p = os.path.join(root_d, fn)
                    rel = os.path.relpath(full_p, layer_mvt_dir).replace("\\", "/")
                    parts = rel.split("/")
                    if len(parts) != 3:
                        continue
                    try:
                        z = int(parts[0])
                        x = int(parts[1])
                        y = int(os.path.splitext(parts[2])[0])
                    except ValueError:
                        continue

                    with open(full_p, "rb") as tf:
                        raw_bytes = tf.read()
                    if not raw_bytes:
                        continue
                    # Decompress if GDAL still wrote gzipped bytes before merging layers
                    if len(raw_bytes) >= 2 and raw_bytes[0] == 0x1F and raw_bytes[1] == 0x8B:
                        raw_bytes = gzip.decompress(raw_bytes)

                    key = (z, x, y)
                    if key not in merged_tiles:
                        merged_tiles[key] = bytearray(raw_bytes)
                    else:
                        merged_tiles[key].extend(raw_bytes)

                    layer_tile_count += 1
                    if actual_min_z is None or z < actual_min_z:
                        actual_min_z = z
                    if actual_max_z is None or z > actual_max_z:
                        actual_max_z = z

            if layer_tile_count > 0:
                total_features += len(gdf_3857)
                geom_types = sorted(set(str(t) for t in gdf_wgs84.geometry.geom_type.unique()))
                vector_layers_meta.append({
                    "id": slug,
                    "description": display_name,
                    "minzoom": layer_min_z,
                    "maxzoom": layer_max_z,
                    "fields": fields_meta,
                    "geometry_type": geom_types[0] if geom_types else "Geometry",
                    "feature_count": len(gdf_3857),
                    "source_crs": source_crs,
                })

        if not merged_tiles:
            return {"success": False, "error": "No vector tiles were generated from input dataset"}

        if actual_min_z is None:
            actual_min_z = min_zoom
        if actual_max_z is None:
            actual_max_z = max_zoom

        bounds_list = [min_x, min_y, max_x, max_y] if math.isfinite(min_x) else [-180.0, -85.0511, 180.0, 85.0511]
        center_lon = (bounds_list[0] + bounds_list[2]) / 2.0
        center_lat = (bounds_list[1] + bounds_list[3]) / 2.0
        center_z = min(actual_max_z, max(actual_min_z, 10))
        final_name = map_name or os.path.splitext(os.path.basename(input_path))[0]

        # Build standards-compliant MBTiles 1.3 SQLite database
        conn = sqlite3.connect(output_path)
        cur = conn.cursor()
        cur.execute("PRAGMA journal_mode = OFF;")
        cur.execute("PRAGMA synchronous = OFF;")
        cur.execute("CREATE TABLE metadata (name TEXT, value TEXT);")
        cur.execute("CREATE TABLE tiles (zoom_level INTEGER, tile_column INTEGER, tile_row INTEGER, tile_data BLOB);")
        cur.execute("CREATE UNIQUE INDEX tile_index ON tiles (zoom_level, tile_column, tile_row);")

        # Strip extra keys for strict TileJSON vector_layers inside MBTiles metadata.json
        strict_vl = [
            {
                "id": v["id"],
                "description": v["description"],
                "minzoom": v["minzoom"],
                "maxzoom": v["maxzoom"],
                "fields": v["fields"],
            }
            for v in vector_layers_meta
        ]
        meta_rows = [
            ("name", final_name),
            ("format", "pbf"),
            ("version", "1.3"),
            ("type", "overlay"),
            ("description", f"Multi-layer vector MBTiles exported from {os.path.basename(input_path)}"),
            ("minzoom", str(actual_min_z)),
            ("maxzoom", str(actual_max_z)),
            ("bounds", f"{bounds_list[0]},{bounds_list[1]},{bounds_list[2]},{bounds_list[3]}"),
            ("center", f"{center_lon},{center_lat},{center_z}"),
            ("json", json.dumps({"vector_layers": strict_vl}, ensure_ascii=False)),
        ]
        cur.executemany("INSERT INTO metadata (name, value) VALUES (?, ?);", meta_rows)

        # Insert gzip-compressed MVT tiles using TMS row convention: tms_y = (2^z - 1) - xyz_y
        tile_rows = []
        for (z, x, y), raw_buf in merged_tiles.items():
            tms_y = (1 << z) - 1 - y
            compressed = gzip.compress(bytes(raw_buf), compresslevel=6)
            tile_rows.append((z, x, tms_y, sqlite3.Binary(compressed)))
        cur.executemany(
            "INSERT OR REPLACE INTO tiles (zoom_level, tile_column, tile_row, tile_data) VALUES (?, ?, ?, ?);",
            tile_rows,
        )
        conn.commit()
        conn.close()

        return {
            "success": True,
            "input_file": input_path,
            "output_file": output_path,
            "format": "pbf",
            "layer_count": len(vector_layers_meta),
            "feature_count": total_features,
            "tile_count": len(merged_tiles),
            "min_zoom": actual_min_z,
            "max_zoom": actual_max_z,
            "bounds": bounds_list,
            "layers": vector_layers_meta,
        }
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)


def main():
    if len(sys.argv) < 3:
        print(json.dumps({
            "success": False,
            "error": "Usage: gpkg_to_vector_mbtiles.py <input.gpkg> <output.mbtiles> [min_zoom] [max_zoom] [name]",
        }))
        sys.exit(1)
    input_path = sys.argv[1]
    output_path = sys.argv[2]
    min_z = int(sys.argv[3]) if len(sys.argv) > 3 and sys.argv[3].strip() != "" else 0
    max_z = int(sys.argv[4]) if len(sys.argv) > 4 and sys.argv[4].strip() != "" else 12
    name = sys.argv[5] if len(sys.argv) > 5 and sys.argv[5].strip() != "" else None
    result = convert_gpkg_to_vector_mbtiles(input_path, output_path, min_z, max_z, name)
    print(json.dumps(result, indent=2))
    if not result.get("success"):
        sys.exit(1)


if __name__ == "__main__":
    main()
