#!/usr/bin/env python3
"""
Batch-render a strict V-Mapplotter / VTAS Custom Local Tiles XYZ ZIP package:
  <out>.zip
  ├── metadata.json
  ├── tilemapresource.xml
  ├── leaflet.html
  ├── index.html
  └── {z}/{x}/{y}.png   (256x256 RGBA PNG, XYZ scheme, tms: false)

Combines ALL visible raster layers and ALL visible vector layers (polygons &
lines ONLY — points/multipoints are excluded) into a SINGLE unified tile layer
respecting the exact bottom-to-top Photoshop layer_stack order.

Usage:
    python render_local_webtiles_zip.py <spec_json_path> <output_zip_path>
"""

import io
import json
import math
import os
import sys
import warnings
import zipfile

warnings.filterwarnings("ignore")

import cv2
import geopandas as gpd
import numpy as np
from PIL import Image, ImageDraw
from shapely.geometry import box

ORIGIN_SHIFT = 20037508.342789244
INITIAL_RESOLUTION = 156543.03392804097
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


def lonlat_to_3857(lon: float, lat: float):
    lat_c = max(-85.05112878, min(85.05112878, lat))
    x = lon * ORIGIN_SHIFT / 180.0
    y = math.log(math.tan((90.0 + lat_c) * math.pi / 360.0)) / (math.pi / 180.0)
    y = y * ORIGIN_SHIFT / 180.0
    return x, y


def lonlat_to_xyz(lon: float, lat: float, z: int):
    lat_c = max(-85.05112878, min(85.05112878, lat))
    n = 2 ** z
    x = int(math.floor((lon + 180.0) / 360.0 * n))
    lat_rad = math.radians(lat_c)
    y = int(math.floor((1.0 - math.asinh(math.tan(lat_rad)) / math.pi) / 2.0 * n))
    x = max(0, min(n - 1, x))
    y = max(0, min(n - 1, y))
    return x, y


def tile_bounds_3857(z: int, x: int, y: int):
    n = 2 ** z
    tile_span = (2.0 * ORIGIN_SHIFT) / n
    minx = -ORIGIN_SHIFT + x * tile_span
    maxx = minx + tile_span
    maxy = ORIGIN_SHIFT - y * tile_span
    miny = maxy - tile_span
    return (minx, miny, maxx, maxy)


def xml_escape(text: str) -> str:
    return (
        str(text)
        .replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
        .replace("'", "&apos;")
    )


def build_metadata_json(name: str, bounds, min_z: int, max_z: int) -> str:
    west, south, east, north = [round(float(v), 8) for v in bounds]
    doc = {
        "name": name,
        "version": "1.1.0",
        "description": "V-Mapplotter Output for VTAS",
        "type": "overlay",
        "format": "png",
        "minzoom": str(min_z),
        "maxzoom": str(max_z),
        "scale": "1.000000",
        "profile": "mercator",
        "bounds": [west, south, east, north],
        "crs": "EPSG:3857",
    }
    return json.dumps(doc, indent=2) + "\n"


def build_tilemapresource_xml(name: str, bounds, min_z: int, max_z: int) -> str:
    west, south, east, north = [float(v) for v in bounds]
    tilesets = []
    for z in range(min_z, max_z + 1):
        upp = INITIAL_RESOLUTION / (2 ** z)
        tilesets.append(f'    <TileSet href="{z}" units-per-pixel="{upp:.10f}" order="{z}"/>')
    tilesets_xml = "\n".join(tilesets)
    return (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<TileMap version="1.0.0" tilemapservice="http://tms.osgeo.org/1.0.0">\n'
        f"  <Title>{xml_escape(name)}</Title>\n"
        "  <Abstract>V-Mapplotter Local WebTiles Output</Abstract>\n"
        "  <SRS>EPSG:3857</SRS>\n"
        f'  <BoundingBox minx="{west:.7f}" miny="{south:.7f}" maxx="{east:.7f}" maxy="{north:.7f}"/>\n'
        f'  <Origin x="{west:.7f}" y="{south:.7f}"/>\n'
        '  <TileFormat width="256" height="256" mime-type="image/png" extension="png"/>\n'
        '  <TileSets profile="global-mercator">\n'
        f"{tilesets_xml}\n"
        "  </TileSets>\n"
        "</TileMap>\n"
    )


def build_leaflet_html(name: str, bounds, min_z: int, max_z: int) -> str:
    west, south, east, north = [float(v) for v in bounds]
    center_lat = (south + north) / 2.0
    center_lon = (west + east) / 2.0
    safe_title = xml_escape(name)
    return f"""<!DOCTYPE html>
<html>
<head>
  <title>{safe_title}</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no" />
  <link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" />
  <script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js"></script>
  <style>
    html, body, #map {{
      width: 100%;
      height: 100%;
      margin: 0;
      padding: 0;
    }}
    .opacity-control {{
      position: absolute;
      top: 10px;
      right: 10px;
      z-index: 1000;
      background: white;
      padding: 6px 12px;
      border-radius: 6px;
      box-shadow: 0 2px 8px rgba(0,0,0,0.2);
      font-family: sans-serif;
      font-size: 12px;
      display: flex;
      align-items: center;
      gap: 8px;
    }}
  </style>
</head>
<body>
  <div id="map"></div>
  <div class="opacity-control">
    <label for="slider"><b>Layer Opacity:</b></label>
    <input id="slider" type="range" min="0" max="1" step="0.05" value="1" oninput="if(overlayLayer) overlayLayer.setOpacity(this.value)">
  </div>

  <script>
    var overlayLayer;

    function init() {{
      var osm = L.tileLayer('https://tile.openstreetmap.org/{{z}}/{{x}}/{{y}}.png', {{
        maxZoom: 19,
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
      }});

      var satellite = L.tileLayer('https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{{z}}/{{y}}/{{x}}', {{
        maxZoom: 20,
        attribution: '&copy; Esri World Imagery'
      }});

      var googleHybrid = L.tileLayer('https://mt1.google.com/vt/lyrs=y&x={{x}}&y={{y}}&z={{z}}', {{
        maxZoom: 20,
        attribution: '&copy; Google Maps'
      }});

      var streetMap = L.tileLayer('https://a.basemaps.cartocdn.com/rastertiles/voyager/{{z}}/{{x}}/{{y}}.png', {{
        maxZoom: 20,
        attribution: '&copy; CartoDB &copy; OpenStreetMap'
      }});

      var bounds = new L.LatLngBounds(
        new L.LatLng({south:.7f}, {west:.7f}),
        new L.LatLng({north:.7f}, {east:.7f})
      );

      var map = L.map('map', {{
        center: [{center_lat:.7f}, {center_lon:.7f}],
        zoom: {min_z},
        layers: [osm]
      }}).fitBounds(bounds);

      L.control.layers({{
        "🗺️ OpenStreetMap": osm,
        "🌍 Google Hybrid": googleHybrid,
        "🛰️ Satellite (Esri)": satellite,
        "🏙️ Voyager Streets": streetMap
      }}, null, {{ position: 'topright' }}).addTo(map);

      overlayLayer = L.tileLayer('{{z}}/{{x}}/{{y}}.png', {{
        minZoom: {min_z},
        maxZoom: {max_z},
        opacity: 1.0,
        tms: false
      }}).addTo(map);
    }}

    window.onload = init;
  </script>
</body>
</html>
"""


def build_index_html(name: str) -> str:
    safe_title = xml_escape(name)
    return f"""<!DOCTYPE html>
<html>
  <head>
    <title>{safe_title}</title>
    <meta charset="utf-8"/>
    <meta http-equiv="refresh" content="0; url=leaflet.html" />
  </head>
  <body>
    <p>Opening <a href="leaflet.html">Map Viewer</a>...</p>
  </body>
</html>
"""


def load_raster_entry(rl: dict):
    src_img_path = rl.get("image_path") or ""
    bounds = rl.get("bounds") or []
    corners = rl.get("corners") or []
    tile_dir = rl.get("tile_dir") or ""
    opacity = float(rl.get("opacity", 1.0) or 1.0)

    loaded = {
        "kind": "raster",
        "layer_id": rl.get("layer_id") or "",
        "tile_dir": tile_dir,
        "opacity": opacity,
        "bounds": bounds,
        "corners_3857": None,
        "bbox_3857": None,
        "src_arr": None,
        "src_pil": None,
    }
    if src_img_path and os.path.isfile(src_img_path):
        try:
            pil_img = Image.open(src_img_path).convert("RGBA")
            loaded["src_pil"] = pil_img
            loaded["src_arr"] = np.array(pil_img, dtype=np.uint8)
        except Exception:
            pass

    if isinstance(corners, list) and len(corners) == 4:
        pts = [lonlat_to_3857(float(c[0]), float(c[1])) for c in corners]
        loaded["corners_3857"] = pts
        loaded["bbox_3857"] = (
            min(p[0] for p in pts),
            min(p[1] for p in pts),
            max(p[0] for p in pts),
            max(p[1] for p in pts),
        )
    elif len(bounds) == 4:
        w, s, e, n = [float(v) for v in bounds]
        rminx, rminy = lonlat_to_3857(w, s)
        rmaxx, rmaxy = lonlat_to_3857(e, n)
        loaded["bbox_3857"] = (rminx, rminy, rmaxx, rmaxy)
    return loaded


def load_vector_entry(vl: dict, idx: int, seen_sources: set):
    src_file = vl.get("source_file") or ""
    src_layer = vl.get("source_layer") or ""
    if not src_file or not os.path.isfile(src_file):
        return None
    dedup_key = (os.path.normcase(os.path.abspath(src_file)), src_layer.lower())
    if dedup_key in seen_sources:
        return None
    seen_sources.add(dedup_key)

    try:
        read_kwargs = {}
        if src_layer and src_file.lower().endswith(".gpkg"):
            read_kwargs["layer"] = src_layer
        gdf = gpd.read_file(src_file, **read_kwargs)
        if gdf is None or len(gdf) == 0:
            return None
        gdf = gdf[gdf.geometry.notnull() & ~gdf.geometry.is_empty]
        # STRICT RULE: exclude Point / MultiPoint geometries (treat vector polygons/lines as raster layer only)
        gdf = gdf[~gdf.geometry.geom_type.isin(["Point", "MultiPoint"])]
        if len(gdf) == 0:
            return None
        if gdf.crs is None:
            gdf = gdf.set_crs("EPSG:4326")
        gdf_3857 = gdf.to_crs("EPSG:3857")
        sindex = gdf_3857.sindex
    except Exception:
        return None

    color_hex = vl.get("color") or PALETTE[idx % len(PALETTE)]
    opacity = float(vl.get("opacity", 1.0) or 1.0)
    return {
        "kind": "vector",
        "layer_id": vl.get("layer_id") or "",
        "gdf": gdf_3857,
        "sindex": sindex,
        "fill_rgba": parse_hex_rgba(color_hex, 0.34 * opacity),
        "stroke_rgba": parse_hex_rgba(color_hex, 0.95 * opacity),
    }


def composite_loaded_raster(canvas: Image.Image, rl: dict, z: int, x: int, y: int, tminx, tminy, tmaxx, tmaxy):
    bbox = rl["bbox_3857"]
    if bbox is not None:
        if bbox[2] <= tminx or bbox[0] >= tmaxx or bbox[3] <= tminy or bbox[1] >= tmaxy:
            return

    opacity = rl["opacity"]
    cw, ch = canvas.size
    tspan_x = tmaxx - tminx
    tspan_y = tmaxy - tminy

    # 1. Exact rotated/affine projection when 4 corners + source image exist
    if rl["src_arr"] is not None and rl["corners_3857"] is not None:
        try:
            src_arr = rl["src_arr"]
            sh, sw = src_arr.shape[:2]
            dst_pts = np.array(
                [
                    [(px - tminx) / tspan_x * cw, (tmaxy - py) / tspan_y * ch]
                    for (px, py) in rl["corners_3857"]
                ],
                dtype=np.float32,
            )
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
            if warped[:, :, 3].any():
                canvas.alpha_composite(Image.fromarray(warped, mode="RGBA"))
            return
        except Exception:
            pass

    # 2. Fast path: pre-rendered raster tile
    tile_dir = rl["tile_dir"]
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

    # 3. Axis-aligned source raster projection
    if rl["src_pil"] is not None and bbox is not None:
        rminx, rminy, rmaxx, rmaxy = bbox
        ix0, iy0 = max(tminx, rminx), max(tminy, rminy)
        ix1, iy1 = min(tmaxx, rmaxx), min(tmaxy, rmaxy)
        if ix1 <= ix0 or iy1 <= iy0:
            return
        try:
            src = rl["src_pil"]
            sw, sh = src.size
            rspan_x = rmaxx - rminx
            rspan_y = rmaxy - rminy
            sx0 = max(0, int(math.floor((ix0 - rminx) / rspan_x * sw)))
            sx1 = min(sw, int(math.ceil((ix1 - rminx) / rspan_x * sw)))
            sy0 = max(0, int(math.floor((rmaxy - iy1) / rspan_y * sh)))
            sy1 = min(sh, int(math.ceil((rmaxy - iy0) / rspan_y * sh)))
            if sx1 <= sx0 or sy1 <= sy0:
                return
            cropped = src.crop((sx0, sy0, sx1, sy1))
            dx0 = int(round((ix0 - tminx) / tspan_x * cw))
            dx1 = int(round((ix1 - tminx) / tspan_x * cw))
            dy0 = int(round((tmaxy - iy1) / tspan_y * ch))
            dy1 = int(round((tmaxy - iy0) / tspan_y * ch))
            resized = cropped.resize((max(1, dx1 - dx0), max(1, dy1 - dy0)), Image.Resampling.BILINEAR)
            if opacity < 0.999:
                r, g, b, a = resized.split()
                a = a.point(lambda p: int(p * opacity))
                resized = Image.merge("RGBA", (r, g, b, a))
            overlay = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
            overlay.paste(resized, (dx0, dy0), resized)
            canvas.alpha_composite(overlay)
        except Exception:
            pass


def draw_loaded_vector(canvas: Image.Image, vl: dict, tminx, tminy, tmaxx, tmaxy, ss: int):
    span = tmaxx - tminx
    pad = span * 0.04
    qbox = box(tminx - pad, tminy - pad, tmaxx + pad, tmaxy + pad)
    cand_idxs = list(vl["sindex"].intersection(qbox.bounds))
    if not cand_idxs:
        return
    sub = vl["gdf"].iloc[cand_idxs]
    hits = sub[sub.geometry.intersects(qbox)]
    if len(hits) == 0:
        return

    size = canvas.size[0]
    draw = ImageDraw.Draw(canvas, "RGBA")
    fill_rgba = vl["fill_rgba"]
    stroke_rgba = vl["stroke_rgba"]
    stroke_w = max(2, int(round(2.0 * ss)))

    def to_px(vx, vy):
        return (
            (vx - tminx) / span * size,
            (1.0 - (vy - tminy) / span) * size,
        )

    def draw_shape(g):
        gt = g.geom_type
        if gt == "Polygon":
            ext = [to_px(vx, vy) for vx, vy in g.exterior.coords]
            if len(ext) >= 3:
                draw.polygon(ext, fill=fill_rgba, outline=stroke_rgba)
                draw.line(ext, fill=stroke_rgba, width=stroke_w)
            for interior in g.interiors:
                ring = [to_px(vx, vy) for vx, vy in interior.coords]
                if len(ring) >= 3:
                    draw.line(ring, fill=stroke_rgba, width=stroke_w)
        elif gt == "MultiPolygon":
            for poly in g.geoms:
                draw_shape(poly)
        elif gt == "LineString":
            pts = [to_px(vx, vy) for vx, vy in g.coords]
            if len(pts) >= 2:
                draw.line(pts, fill=stroke_rgba, width=stroke_w)
        elif gt == "MultiLineString":
            for ls in g.geoms:
                draw_shape(ls)
        elif gt == "GeometryCollection":
            for item in g.geoms:
                if item.geom_type not in ("Point", "MultiPoint"):
                    draw_shape(item)

    for geom in hits.geometry.values:
        if geom.geom_type not in ("Point", "MultiPoint"):
            draw_shape(geom)


def build_zip(spec_path: str, out_zip_path: str) -> dict:
    with open(spec_path, "r", encoding="utf-8") as f:
        spec = json.load(f)

    name = str(spec.get("name") or "veracity_map").strip()
    bounds = spec.get("bounds") or []
    if len(bounds) != 4:
        raise ValueError("Valid 4-element WGS84 bounds [west, south, east, north] are required")

    west, south, east, north = [float(v) for v in bounds]
    min_z = int(spec.get("min_zoom", 12))
    max_z = int(spec.get("max_zoom", 16))
    if max_z < min_z:
        max_z = min_z

    # Pre-load all visible rasters & non-point vectors ONCE
    raster_by_id = {}
    for rl in spec.get("raster_layers", []):
        if rl.get("visible") is False:
            continue
        loaded = load_raster_entry(rl)
        raster_by_id[loaded["layer_id"]] = loaded

    vector_by_id = {}
    seen_sources = set()
    for idx, vl in enumerate(spec.get("vector_layers", [])):
        if vl.get("visible") is False:
            continue
        loaded = load_vector_entry(vl, idx, seen_sources)
        if loaded is not None:
            vector_by_id[loaded["layer_id"]] = loaded

    # Build exact bottom-to-top render pipeline from Photoshop layer_stack (or group layer_order fallback)
    pipeline = []
    layer_stack = spec.get("layer_stack") or []
    used_r, used_v = set(), set()
    if isinstance(layer_stack, list) and len(layer_stack) > 0:
        for item in reversed(layer_stack):
            kind = str(item.get("kind") or "").lower()
            lid = item.get("id") or ""
            if kind == "raster" and lid in raster_by_id and lid not in used_r:
                pipeline.append(raster_by_id[lid])
                used_r.add(lid)
            elif kind == "vector" and lid in vector_by_id and lid not in used_v:
                pipeline.append(vector_by_id[lid])
                used_v.add(lid)
    # Append any remaining layers not listed in layer_stack
    layer_order = spec.get("layer_order") or ["vector", "raster", "base"]
    raster_on_top = len(layer_order) > 0 and str(layer_order[0]).lower() == "raster"
    rem_r = [v for k, v in raster_by_id.items() if k not in used_r]
    rem_v = [v for k, v in vector_by_id.items() if k not in used_v]
    if raster_on_top:
        pipeline.extend(rem_v)
        pipeline.extend(rem_r)
    else:
        pipeline.extend(rem_r)
        pipeline.extend(rem_v)

    os.makedirs(os.path.dirname(os.path.abspath(out_zip_path)), exist_ok=True)
    SS = 2
    SIZE = 256 * SS
    tiles_written = 0

    with zipfile.ZipFile(out_zip_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        # 1. Write the exact 4 root metadata & viewer files matching p3_webtiles_xyz format
        zf.writestr("metadata.json", build_metadata_json(name, bounds, min_z, max_z))
        zf.writestr("tilemapresource.xml", build_tilemapresource_xml(name, bounds, min_z, max_z))
        zf.writestr("leaflet.html", build_leaflet_html(name, bounds, min_z, max_z))
        zf.writestr("index.html", build_index_html(name))

        # 2. Render single combined XYZ tile pyramid {z}/{x}/{y}.png directly at ZIP root
        for z in range(min_z, max_z + 1):
            x0, y0 = lonlat_to_xyz(west, north, z)
            x1, y1 = lonlat_to_xyz(east, south, z)
            if x0 > x1:
                x0, x1 = x1, x0
            if y0 > y1:
                y0, y1 = y1, y0

            for x in range(x0, x1 + 1):
                for y in range(y0, y1 + 1):
                    tminx, tminy, tmaxx, tmaxy = tile_bounds_3857(z, x, y)
                    canvas = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
                    for entry in pipeline:
                        if entry["kind"] == "raster":
                            composite_loaded_raster(canvas, entry, z, x, y, tminx, tminy, tmaxx, tmaxy)
                        else:
                            draw_loaded_vector(canvas, entry, tminx, tminy, tmaxx, tmaxy, SS)

                    # Skip completely transparent tiles
                    alpha_extrema = canvas.getchannel("A").getextrema()
                    if alpha_extrema[1] == 0:
                        continue

                    out_img = canvas.resize((256, 256), Image.Resampling.LANCZOS)
                    buf = io.BytesIO()
                    out_img.save(buf, format="PNG", optimize=True)
                    zf.writestr(f"{z}/{x}/{y}.png", buf.getvalue())
                    tiles_written += 1

    return {
        "success": True,
        "zip_path": out_zip_path,
        "tiles_written": tiles_written,
        "min_zoom": min_z,
        "max_zoom": max_z,
    }


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "error": "Usage: render_local_webtiles_zip.py <spec.json> <out.zip>"}))
        sys.exit(1)
    res = build_zip(sys.argv[1], sys.argv[2])
    print(json.dumps(res))


if __name__ == "__main__":
    main()
