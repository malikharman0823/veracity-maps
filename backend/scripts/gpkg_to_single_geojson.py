#!/usr/bin/env python3
"""
Convert all layers (polygons, lines, and points) inside a GeoPackage (.gpkg)
into a single combined EPSG:4326 GeoJSON FeatureCollection.

Usage:
    python gpkg_to_single_geojson.py <input.gpkg> [output.geojson]
"""

import json
import math
import os
import re
import sys
import warnings

warnings.filterwarnings("ignore")

import geopandas as gpd
import pyogrio
from shapely import force_2d
from shapely.geometry import mapping


def safe_slug(name: str) -> str:
    s = re.sub(r"[^a-zA-Z0-9_-]+", "_", str(name or "layer").strip().lower()).strip("_")
    return s or "layer"


def json_safe(val):
    if val is None:
        return None
    if isinstance(val, (bool, int, str)):
        return val
    if isinstance(val, float):
        return None if (math.isnan(val) or math.isinf(val)) else val
    if isinstance(val, bytes):
        return val.decode("utf-8", errors="replace")
    if hasattr(val, "isna") and callable(val.isna):
        try:
            if val.isna():
                return None
        except Exception:
            pass
    if hasattr(val, "item") and callable(val.item):
        try:
            return json_safe(val.item())
        except Exception:
            pass
    return str(val)


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


def convert_gpkg_to_single_geojson(input_path: str, output_path: str = None) -> dict:
    if not os.path.exists(input_path):
        return {"success": False, "error": f"Input file not found: {input_path}"}

    if not output_path:
        base, _ = os.path.splitext(input_path)
        output_path = f"{base}.geojson"

    out_dir = os.path.dirname(os.path.abspath(output_path))
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)

    try:
        raw_layers = pyogrio.list_layers(input_path)
        layer_names = [str(row[0]) for row in raw_layers]
    except Exception as e:
        return {"success": False, "error": f"Failed listing GeoPackage layers: {e}"}

    all_features = []
    layer_summaries = []
    min_x, min_y, max_x, max_y = float("inf"), float("inf"), float("-inf"), float("-inf")

    for lname in layer_names:
        try:
            gdf = gpd.read_file(input_path, layer=lname)
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

        slug = safe_slug(lname)
        geom_types = sorted(set(str(t) for t in gdf_wgs84.geometry.geom_type.unique()))
        bounds = [float(v) for v in gdf_wgs84.total_bounds]
        if len(bounds) == 4 and all(math.isfinite(v) for v in bounds):
            min_x = min(min_x, bounds[0])
            min_y = min(min_y, bounds[1])
            max_x = max(max_x, bounds[2])
            max_y = max(max_y, bounds[3])

        non_geom_cols = [c for c in gdf_wgs84.columns if c != "geometry"]
        layer_count = 0
        for idx, row in gdf_wgs84.iterrows():
            geom = row.geometry
            props = {str(col): json_safe(row[col]) for col in non_geom_cols}
            fid = props.get("feature_id") or props.get("fid") or props.get("id") or f"{slug}_{idx}"
            props["feature_id"] = str(fid)
            props["_source_layer"] = slug
            props["_layer_name"] = lname
            props["_geometry_type"] = str(geom.geom_type)
            props["_source_crs"] = source_crs

            all_features.append({
                "type": "Feature",
                "id": f"{slug}:{fid}",
                "geometry": mapping(geom),
                "properties": props,
            })
            layer_count += 1

        layer_summaries.append({
            "layer_name": lname,
            "layer_slug": slug,
            "geometry_types": geom_types,
            "feature_count": layer_count,
            "source_crs": source_crs,
            "bounds": bounds,
        })

    total_bounds = [min_x, min_y, max_x, max_y] if math.isfinite(min_x) else None

    fc = {
        "type": "FeatureCollection",
        "name": os.path.splitext(os.path.basename(input_path))[0],
        "crs": {"type": "name", "properties": {"name": "urn:ogc:def:crs:OGC:1.3:CRS84"}},
        "metadata": {
            "source_file": os.path.basename(input_path),
            "layer_count": len(layer_summaries),
            "feature_count": len(all_features),
            "bounds": total_bounds,
            "layers": layer_summaries,
        },
        "features": all_features,
    }

    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(fc, f, ensure_ascii=False)

    return {
        "success": True,
        "input_file": input_path,
        "output_file": output_path,
        "layer_count": len(layer_summaries),
        "feature_count": len(all_features),
        "bounds": total_bounds,
        "layers": layer_summaries,
    }


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"success": False, "error": "Usage: gpkg_to_single_geojson.py <input.gpkg> [output.geojson]"}))
        sys.exit(1)

    input_path = sys.argv[1]
    output_path = sys.argv[2] if len(sys.argv) > 2 and sys.argv[2].strip() != "" else None
    result = convert_gpkg_to_single_geojson(input_path, output_path)
    print(json.dumps(result, indent=2))
    if not result.get("success"):
        sys.exit(1)


if __name__ == "__main__":
    main()
