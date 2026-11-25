#!/usr/bin/env python3
"""
Render a single 256x256 RGBA PNG composite tile for (z, x, y) combining:
  - Raster layers (existing XYZ tiles or georeferenced images)
  - Vector layers (polygons with fill+outline, lines, and colored point markers with white halos)

Works cleanly across z0-z18 for Viewscape Custom Web Tile URLs:
    http://<host>:8000/tiles/{map_id}/composite/{0}/{1}/{2}.png

Usage:
    python render_composite_tile.py <spec_json_path> <z> <x> <y> <output_png_path>
"""

import json
import math
import os
import sys
import warnings

warnings.filterwarnings("ignore")

from PIL import Image, ImageDraw
import geopandas as gpd
import pyogrio
from shapely.geometry import box

ORIGIN_SHIFT = 20037508.342789244
PALETTE = [
    "#3ecf8e",
    "#f59e0b",
    "#8b5cf6",
    "#06b6d4",
    "#ef4444",
    "#ec4899",
    "#3b82f6",
    "#10b981",
]


def parse_hex_rgba(hex_color: str, opacity: float = 1.0):
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


def tile_bounds_3857(z: int, x: int, y: int):
    n = 2 ** z
    tile_span = (2.0 * ORIGIN_SHIFT) / n
    minx = -ORIGIN_SHIFT + x * tile_span
    maxx = minx + tile_span
    maxy = ORIGIN_SHIFT - y * tile_span
    miny = maxy - tile_span
    return (minx, miny, maxx, maxy)


def lonlat_to_3857(lon: float, lat: float):
    lat_c = max(-85.05112878, min(85.05112878, lat))
    x = lon * ORIGIN_SHIFT / 180.0
    y = math.log(math.tan((90.0 + lat_c) * math.pi / 360.0)) / (math.pi / 180.0)
    y = y * ORIGIN_SHIFT / 180.0
    return x, y


def composite_raster_layer(canvas: Image.Image, rl: dict, z: int, x: int, y: int, tminx, tminy, tmaxx, tmaxy):
    tile_dir = rl.get("tile_dir") or ""
    opacity = float(rl.get("opacity", 1.0) or 1.0)
    src_img_path = rl.get("image_path") or ""
    corners = rl.get("corners") or []
    bounds = rl.get("bounds") or []

    # 1. Exact rotated/affine projection when 4 WGS84 corners [TL, TR, BR, BL] + source image exist
    if src_img_path and os.path.isfile(src_img_path) and isinstance(corners, list) and len(corners) == 4:
        try:
            import cv2
            import numpy as np

            dst_3857 = [lonlat_to_3857(float(c[0]), float(c[1])) for c in corners]
            c_minx = min(p[0] for p in dst_3857)
            c_maxx = max(p[0] for p in dst_3857)
            c_miny = min(p[1] for p in dst_3857)
            c_maxy = max(p[1] for p in dst_3857)
            if c_maxx <= tminx or c_minx >= tmaxx or c_maxy <= tminy or c_miny >= tmaxy:
                return

            cw, ch = canvas.size
            tspan_x = tmaxx - tminx
            tspan_y = tmaxy - tminy
            dst_pts = np.array(
                [
                    [
                        (px - tminx) / tspan_x * cw,
                        (tmaxy - py) / tspan_y * ch,
                    ]
                    for (px, py) in dst_3857
                ],
                dtype=np.float32,
            )

            src = Image.open(src_img_path).convert("RGBA")
            sw, sh = src.size
            src_arr = np.array(src, dtype=np.uint8)
            src_pts = np.array(
                [[0.0, 0.0], [float(sw), 0.0], [float(sw), float(sh)], [0.0, float(sh)]],
                dtype=np.float32,
            )

            M = cv2.getPerspectiveTransform(src_pts, dst_pts)
            warped = cv2.warpPerspective(
                src_arr,
                M,
                (cw, ch),
                flags=cv2.INTER_LINEAR,
                borderMode=cv2.BORDER_CONSTANT,
                borderValue=(0, 0, 0, 0),
            )
            if opacity < 0.999:
                warped[:, :, 3] = np.clip(warped[:, :, 3].astype(np.float32) * opacity, 0, 255).astype(np.uint8)
            overlay = Image.fromarray(warped, mode="RGBA")
            canvas.alpha_composite(overlay)
            return
        except Exception:
            pass

    # 2. Fast path: exact matching pre-rendered raster tile exists at z/x/y
    if tile_dir and os.path.isdir(tile_dir):
        for ext in (".png", ".jpg", ".jpeg", ""):
            cand = os.path.join(tile_dir, str(z), str(x), f"{y}{ext}")
            if os.path.isfile(cand):
                try:
                    timg = Image.open(cand).convert("RGBA")
                    if timg.size != canvas.size:
                        timg = timg.resize(canvas.size, Image.Resampling.BILINEAR)
                    if opacity < 0.999:
                        r, g, b, a = timg.split()
                        a = a.point(lambda p: int(p * opacity))
                        timg = Image.merge("RGBA", (r, g, b, a))
                    canvas.alpha_composite(timg)
                    return
                except Exception:
                    pass

    # 3. Fallback for any zoom (z0-z22): project axis-aligned source raster image into this tile's 3857 bbox
    if not src_img_path or not os.path.isfile(src_img_path) or len(bounds) != 4:
        return

    w, s, e, n = [float(v) for v in bounds]
    rminx, rminy = lonlat_to_3857(w, s)
    rmaxx, rmaxy = lonlat_to_3857(e, n)

    # Check intersection
    ix0 = max(tminx, rminx)
    iy0 = max(tminy, rminy)
    ix1 = min(tmaxx, rmaxx)
    iy1 = min(tmaxy, rmaxy)
    if ix1 <= ix0 or iy1 <= iy0:
        return

    try:
        src = Image.open(src_img_path).convert("RGBA")
        sw, sh = src.size
        rspan_x = rmaxx - rminx
        rspan_y = rmaxy - rminy
        if rspan_x <= 0 or rspan_y <= 0:
            return

        # Crop source region intersecting tile
        sx0 = max(0, int(math.floor((ix0 - rminx) / rspan_x * sw)))
        sx1 = min(sw, int(math.ceil((ix1 - rminx) / rspan_x * sw)))
        sy0 = max(0, int(math.floor((rmaxy - iy1) / rspan_y * sh)))
        sy1 = min(sh, int(math.ceil((rmaxy - iy0) / rspan_y * sh)))
        if sx1 <= sx0 or sy1 <= sy0:
            return

        cropped = src.crop((sx0, sy0, sx1, sy1))
        cw, ch = canvas.size
        tspan_x = tmaxx - tminx
        tspan_y = tmaxy - tminy
        dx0 = int(round((ix0 - tminx) / tspan_x * cw))
        dx1 = int(round((ix1 - tminx) / tspan_x * cw))
        dy0 = int(round((tmaxy - iy1) / tspan_y * ch))
        dy1 = int(round((tmaxy - iy0) / tspan_y * ch))
        tw = max(1, dx1 - dx0)
        th = max(1, dy1 - dy0)

        resized = cropped.resize((tw, th), Image.Resampling.BILINEAR)
        if opacity < 0.999:
            r, g, b, a = resized.split()
            a = a.point(lambda p: int(p * opacity))
            resized = Image.merge("RGBA", (r, g, b, a))

        overlay = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
        overlay.paste(resized, (dx0, dy0), resized)
        canvas.alpha_composite(overlay)
    except Exception:
        pass


def render_composite_tile(spec_path: str, z: int, x: int, y: int, out_png_path: str) -> dict:
    with open(spec_path, "r", encoding="utf-8") as f:
        spec = json.load(f)

    SS = 2  # 2x supersampling for crisp anti-aliased polygons, lines, and point halos
    SIZE = 256 * SS
    tminx, tminy, tmaxx, tmaxy = tile_bounds_3857(z, x, y)
    span = tmaxx - tminx
    # Slight buffer so point markers / thick strokes on tile borders don't clip
    pad = span * 0.08
    tile_box_3857 = box(tminx - pad, tminy - pad, tmaxx + pad, tmaxy + pad)

    canvas = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    layer_order = spec.get("layer_order") or ["vector", "raster", "base"]
    raster_on_top = (
        isinstance(layer_order, list)
        and len(layer_order) > 0
        and str(layer_order[0]).strip().lower() == "raster"
    )

    def draw_all_rasters():
        for rl in spec.get("raster_layers", []):
            if rl.get("visible") is False:
                continue
            composite_raster_layer(canvas, rl, z, x, y, tminx, tminy, tmaxx, tmaxy)

    def draw_all_vectors():
        draw = ImageDraw.Draw(canvas, "RGBA")

        def to_px(px_x, px_y):
            return (
                (px_x - tminx) / span * SIZE,
                (1.0 - (px_y - tminy) / span) * SIZE,
            )

        def draw_shape(g, fill_rgba, stroke_rgba, stroke_w):
            gtype = g.geom_type
            if gtype == "Polygon":
                ext = [to_px(vx, vy) for vx, vy in g.exterior.coords]
                if len(ext) >= 3:
                    draw.polygon(ext, fill=fill_rgba, outline=stroke_rgba)
                    draw.line(ext, fill=stroke_rgba, width=stroke_w)
                for interior in g.interiors:
                    ring = [to_px(vx, vy) for vx, vy in interior.coords]
                    if len(ring) >= 3:
                        draw.line(ring, fill=stroke_rgba, width=stroke_w)
            elif gtype == "MultiPolygon":
                for poly in g.geoms:
                    draw_shape(poly, fill_rgba, stroke_rgba, stroke_w)
            elif gtype == "LineString":
                pts = [to_px(vx, vy) for vx, vy in g.coords]
                if len(pts) >= 2:
                    draw.line(pts, fill=stroke_rgba, width=stroke_w)
            elif gtype == "MultiLineString":
                for ls in g.geoms:
                    draw_shape(ls, fill_rgba, stroke_rgba, stroke_w)
            elif gtype == "GeometryCollection":
                for sub in g.geoms:
                    draw_shape(sub, fill_rgba, stroke_rgba, stroke_w)

        # Collect points to draw on top of all polygons/lines
        deferred_points = []
        seen_sources = set()

        for idx, vl in enumerate(spec.get("vector_layers", [])):
            if vl.get("visible") is False:
                continue
            src_file = vl.get("source_file") or ""
            src_layer = vl.get("source_layer") or ""
            if not src_file or not os.path.isfile(src_file):
                continue
            dedup_key = (os.path.normcase(os.path.abspath(src_file)), src_layer.lower())
            if dedup_key in seen_sources:
                continue
            seen_sources.add(dedup_key)

            color_hex = vl.get("color") or PALETTE[idx % len(PALETTE)]
            opacity = float(vl.get("opacity", 1.0) or 1.0)
            fill_rgba = parse_hex_rgba(color_hex, 0.34 * opacity)
            stroke_rgba = parse_hex_rgba(color_hex, 0.95 * opacity)
            point_rgba = parse_hex_rgba(color_hex, 0.98 * opacity)
            stroke_w = max(2, int(round(2.0 * SS)))

            try:
                read_kwargs = {}
                if src_layer and src_file.lower().endswith(".gpkg"):
                    read_kwargs["layer"] = src_layer
                gdf = gpd.read_file(src_file, **read_kwargs)
                if gdf is None or len(gdf) == 0:
                    continue
                gdf = gdf[gdf.geometry.notnull() & ~gdf.geometry.is_empty]
                if len(gdf) == 0:
                    continue
                if gdf.crs is None:
                    gdf = gdf.set_crs("EPSG:4326")
                gdf_3857 = gdf.to_crs("EPSG:3857")
                # Spatial filter to tile bbox
                hits = gdf_3857[gdf_3857.geometry.intersects(tile_box_3857)]
                if len(hits) == 0:
                    continue
                for geom in hits.geometry.values:
                    gt = geom.geom_type
                    if gt == "Point":
                        deferred_points.append((geom.x, geom.y, point_rgba))
                    elif gt == "MultiPoint":
                        for pt in geom.geoms:
                            deferred_points.append((pt.x, pt.y, point_rgba))
                    else:
                        draw_shape(geom, fill_rgba, stroke_rgba, stroke_w)
            except Exception:
                continue

        # Draw point markers on top with crisp white halo
        pt_radius = (3.5 if z <= 6 else (5.0 if z <= 10 else 6.5)) * SS
        halo_r = pt_radius + 1.8 * SS
        for px_x, px_y, pt_rgba in deferred_points:
            cx, cy = to_px(px_x, px_y)
            draw.ellipse((cx - halo_r, cy - halo_r, cx + halo_r, cy + halo_r), fill=(255, 255, 255, 240))
            draw.ellipse((cx - pt_radius, cy - pt_radius, cx + pt_radius, cy + pt_radius), fill=pt_rgba)

    if raster_on_top:
        draw_all_vectors()
        draw_all_rasters()
    else:
        draw_all_rasters()
        draw_all_vectors()

    out_img = canvas.resize((256, 256), Image.Resampling.LANCZOS)
    os.makedirs(os.path.dirname(os.path.abspath(out_png_path)), exist_ok=True)
    out_img.save(out_png_path, format="PNG", optimize=True)
    return {"success": True, "path": out_png_path}


def main():
    if len(sys.argv) < 6:
        print(json.dumps({"success": False, "error": "Usage: render_composite_tile.py <spec.json> <z> <x> <y> <out.png>"}))
        sys.exit(1)
    res = render_composite_tile(sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4]), sys.argv[5])
    print(json.dumps(res))


if __name__ == "__main__":
    main()
