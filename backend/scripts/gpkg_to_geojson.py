#!/usr/bin/env python3
"""
Standalone GeoPackage (.gpkg) -> GeoJSON converter for Veracity Maps / Viewscape.

Usage:
    python gpkg_to_geojson.py <input.gpkg> <output_dir> [layer_name]

Outputs:
    <output_dir>/<layer_slug>.geojson   One EPSG:4326 GeoJSON FeatureCollection per layer
    <output_dir>/points.geojson         Combined Point/MultiPoint GeoJSON (if any points exist)
    <output_dir>/layers_summary.json    Metadata summary (CRS, geometry type, feature count, bounds)
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


def convert_gpkg_to_geojson(input_path: str, output_dir: str, target_layer: str = None) -> dict:
    if not os.path.exists(input_path):
        return {"success": False, "error": f"Input file not found: {input_path}"}

    os.makedirs(output_dir, exist_ok=True)
    ext = os.path.splitext(input_path)[1].lower()

    if target_layer:
        layer_names = [target_layer]
    elif ext == ".gpkg":
        try:
            raw_layers = pyogrio.list_layers(input_path)
            layer_names = [str(row[0]) for row in raw_layers]
        except Exception as e:
            return {"success": False, "error": f"Failed listing GeoPackage layers: {e}"}
    else:
        layer_names = [None]

    exported_layers = []
    combined_points = []
    used_slugs = {}

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

        # Force 2D coordinates for maximum GeoJSON / Viewscape compatibility
        try:
            gdf_wgs84["geometry"] = force_2d(gdf_wgs84.geometry)
        except Exception:
            pass

        display_name = lname or os.path.splitext(os.path.basename(input_path))[0]
        base_slug = safe_slug(display_name)
        used_slugs[base_slug] = used_slugs.get(base_slug, 0) + 1
        final_slug = base_slug if used_slugs[base_slug] == 1 else f"{base_slug}_{used_slugs[base_slug]}"

        geom_types = sorted(set(str(t) for t in gdf_wgs84.geometry.geom_type.unique()))
        primary_geom = geom_types[0] if geom_types else "Geometry"
        bounds = [float(v) for v in gdf_wgs84.total_bounds]

        features = []
        non_geom_cols = [c for c in gdf_wgs84.columns if c != "geometry"]
        for idx, row in gdf_wgs84.iterrows():
            geom = row.geometry
            props = {str(col): json_safe(row[col]) for col in non_geom_cols}
            fid = props.get("feature_id") or props.get("fid") or props.get("id") or f"{final_slug}_{idx}"
            props["feature_id"] = str(fid)
            props["_source_layer"] = final_slug
            props["_layer_name"] = display_name
            props["_source_crs"] = source_crs

            feat = {
                "type": "Feature",
                "id": str(fid),
                "geometry": mapping(geom),
                "properties": props,
            }
            features.append(feat)
            if geom.geom_type in ("Point", "MultiPoint"):
                combined_points.append(feat)

        out_file = os.path.join(output_dir, f"{final_slug}.geojson")
        fc = {
            "type": "FeatureCollection",
            "name": display_name,
            "crs": {"type": "name", "properties": {"name": "urn:ogc:def:crs:OGC:1.3:CRS84"}},
            "metadata": {
                "source_layer": final_slug,
                "layer_name": display_name,
                "source_crs": source_crs,
                "geometry_type": primary_geom,
                "geometry_types": geom_types,
                "feature_count": len(features),
                "bounds": bounds,
            },
            "features": features,
        }
        with open(out_file, "w", encoding="utf-8") as f:
            json.dump(fc, f, ensure_ascii=False)

        exported_layers.append({
            "layer_name": display_name,
            "layer_slug": final_slug,
            "geojson_path": out_file,
            "filename": f"{final_slug}.geojson",
            "source_crs": source_crs,
            "geometry_type": primary_geom,
            "geometry_types": geom_types,
            "feature_count": len(features),
            "bounds": bounds,
        })

    points_path = ""
    if combined_points:
        points_path = os.path.join(output_dir, "points.geojson")
        with open(points_path, "w", encoding="utf-8") as f:
            json.dump(
                {
                    "type": "FeatureCollection",
                    "name": "Points",
                    "crs": {"type": "name", "properties": {"name": "urn:ogc:def:crs:OGC:1.3:CRS84"}},
                    "features": combined_points,
                },
                f,
                ensure_ascii=False,
            )

    summary = {
        "success": True,
        "input_file": input_path,
        "output_dir": output_dir,
        "layer_count": len(exported_layers),
        "point_count": len(combined_points),
        "points_geojson_path": points_path,
        "layers": exported_layers,
    }
    summary_path = os.path.join(output_dir, "layers_summary.json")
    with open(summary_path, "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)

    return summary


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "error": "Usage: gpkg_to_geojson.py <input.gpkg> <output_dir> [layer_name]"}))
        sys.exit(1)
    input_path = sys.argv[1]
    output_dir = sys.argv[2]
    target_layer = sys.argv[3] if len(sys.argv) > 3 and sys.argv[3].strip() != "" else None
    result = convert_gpkg_to_geojson(input_path, output_dir, target_layer)
    print(json.dumps(result))
    if not result.get("success"):
        sys.exit(1)


if __name__ == "__main__":
    main()
