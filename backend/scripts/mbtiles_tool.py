import sys
import os
os.environ.setdefault("OPENCV_LOG_LEVEL", "OFF")
import io
import json
import math
import gzip
import sqlite3
import warnings

warnings.filterwarnings("ignore")

import numpy as np
import cv2
from PIL import Image
import rasterio
from rasterio.crs import CRS
from rasterio.transform import Affine, from_bounds
from rasterio.enums import Resampling
from rasterio.warp import reproject, transform_bounds
import mercantile


RESAMPLING_MAP = {
    "lanczos": Resampling.lanczos,
    "cubic": Resampling.cubic,
    "bilinear": Resampling.bilinear,
    "nearest": Resampling.nearest,
    "nearest_neighbour": Resampling.nearest,
    "nearest-neighbour": Resampling.nearest,
}


def _select_resampling(method_str, classification=""):
    if method_str:
        key = str(method_str).strip().lower()
        if key in RESAMPLING_MAP:
            return RESAMPLING_MAP[key], key
    cls = str(classification or "").strip().lower()
    if cls == "categorical_raster":
        return Resampling.nearest, "nearest"
    if cls in ("engineering_drawing", "scanned_document", "vector_pdf", "cad_vector", "raster_imagery"):
        return Resampling.lanczos, "lanczos"
    return Resampling.cubic, "cubic"


def _xyz_to_tms(y_xyz, z):
    return (1 << z) - 1 - y_xyz


def _tms_to_xyz(y_tms, z):
    return (1 << z) - 1 - y_tms


def _lnglat_to_mercator(lng, lat):
    lat = max(-85.05112878, min(85.05112878, float(lat)))
    lng = max(-180.0, min(180.0, float(lng)))
    x = lng * 20037508.342789244 / 180.0
    y = math.log(math.tan((90.0 + lat) * math.pi / 360.0)) / (math.pi / 180.0)
    y = y * 20037508.342789244 / 180.0
    return x, y


def _tile_mercator_bounds(x, y_xyz, z):
    b = mercantile.xy_bounds(mercantile.Tile(x=x, y=y_xyz, z=z))
    return b.left, b.bottom, b.right, b.top


def _fetch_basemap_tile(z, x, y_xyz, cache_dir="temp/basemap_cache"):
    try:
        os.makedirs(cache_dir, exist_ok=True)
        cache_path = os.path.join(cache_dir, f"{z}_{x}_{y_xyz}.png")
        if os.path.exists(cache_path):
            cached = cv2.imread(cache_path, cv2.IMREAD_UNCHANGED)
            if cached is not None and cached.ndim >= 3:
                return cv2.cvtColor(cached, cv2.COLOR_BGR2RGBA)
        import urllib.request
        url = f"https://tile.openstreetmap.org/{z}/{x}/{y_xyz}.png"
        req = urllib.request.Request(url, headers={"User-Agent": "VeracityMaps/2.0 (Offline Packager)"})
        with urllib.request.urlopen(req, timeout=1.5) as resp:
            data = resp.read()
        im = Image.open(io.BytesIO(data)).convert("RGBA")
        arr = np.array(im)
        im.save(cache_path)
        return arr
    except Exception:
        fallback = np.full((256, 256, 4), 255, dtype=np.uint8)
        fallback[:, :, 0] = 242
        fallback[:, :, 1] = 239
        fallback[:, :, 2] = 233
        return fallback


def _compute_native_zoom_range(bounds_wgs84, width, height):
    west, south, east, north = [float(v) for v in bounds_wgs84[:4]]
    center_lat = max(-80.0, min(80.0, (south + north) / 2.0))
    cos_lat = max(0.05, math.cos(math.radians(center_lat)))

    span_deg_x = max(1e-9, abs(east - west))
    span_deg_y = max(1e-9, abs(north - south))
    meters_w = span_deg_x * 111319.49079 * cos_lat
    meters_h = span_deg_y * 111132.954

    gsd_x = meters_w / float(max(1, width))
    gsd_y = meters_h / float(max(1, height))
    gsd = max(1e-5, min(gsd_x, gsd_y))

    # At zoom z, 256px tile resolution is (156543.033928 * cos_lat) / 2^z meters/pixel
    equator_res = 156543.03392804097 * cos_lat
    native_max_z = int(math.ceil(math.log2(equator_res / gsd)))
    native_max_z = max(1, min(22, native_max_z))

    # Base zoom where the entire extent fits inside ~1-2 tiles
    max_span_deg = max(span_deg_x, span_deg_y)
    fit_z = int(math.floor(math.log2(360.0 / max_span_deg)))
    fit_z = max(0, min(native_max_z, fit_z))
    min_z = max(0, fit_z - 2)

    return min_z, native_max_z, round(gsd, 5)


def _load_rgba_array(image_path):
    """Loads any raster into (4, H, W) uint8 RGBA array."""
    raw = cv2.imread(image_path, cv2.IMREAD_UNCHANGED)
    if raw is not None:
        if raw.dtype == np.uint16:
            raw = (raw >> 8).astype(np.uint8)
        elif raw.dtype != np.uint8:
            lo, hi = np.percentile(raw, (1.0, 99.0))
            raw = np.clip((raw.astype(np.float32) - lo) / max(1e-6, hi - lo) * 255.0, 0, 255).astype(np.uint8)

        if raw.ndim == 2:
            rgba = cv2.cvtColor(raw, cv2.COLOR_GRAY2RGBA)
        elif raw.shape[2] == 3:
            rgba = cv2.cvtColor(raw, cv2.COLOR_BGR2RGBA)
        else:
            rgba = cv2.cvtColor(raw, cv2.COLOR_BGRA2RGBA)
        return np.transpose(rgba, (2, 0, 1))  # (4, H, W)

    with rasterio.open(image_path) as src:
        data = src.read()
        c, h, w = data.shape

        def u8(a):
            if a.dtype == np.uint8:
                return a
            lo, hi = np.percentile(a[np.isfinite(a)], (1.0, 99.0)) if np.any(np.isfinite(a)) else (0, 1)
            return np.clip((a.astype(np.float32) - lo) / max(1e-6, hi - lo) * 255.0, 0, 255).astype(np.uint8)

        if c == 1:
            g = u8(data[0])
            return np.stack([g, g, g, np.full((h, w), 255, dtype=np.uint8)], axis=0)
        elif c == 2:
            g = u8(data[0])
            return np.stack([g, g, g, u8(data[1])], axis=0)
        elif c == 3:
            return np.stack([u8(data[0]), u8(data[1]), u8(data[2]), np.full((h, w), 255, dtype=np.uint8)], axis=0)
        else:
            return np.stack([u8(data[0]), u8(data[1]), u8(data[2]), u8(data[3])], axis=0)


def optimize_geotiff(input_path, output_tif_path, bounds_wgs84, transform_6=None, resampling_method="lanczos"):
    """
    Phase 5.1 & 5.2: Builds a GDAL-optimized GeoTIFF with DEFLATE compression,
    proper CRS/Affine georeferencing, alpha band, and internal multi-scale overviews.
    """
    rgba = _load_rgba_array(input_path)
    _, h, w = rgba.shape
    resamp_enum, resamp_name = _select_resampling(resampling_method)

    if transform_6 and len(transform_6) == 6:
        # Note: if image was upscaled (e.g. 2x), scale pixel sizes a,b,d,e proportionally if caller passes original transform
        aff = Affine(
            float(transform_6[0]),
            float(transform_6[1]),
            float(transform_6[2]),
            float(transform_6[3]),
            float(transform_6[4]),
            float(transform_6[5]),
        )
    else:
        west, south, east, north = [float(v) for v in bounds_wgs84[:4]]
        aff = from_bounds(west, south, east, north, w, h)

    os.makedirs(os.path.dirname(os.path.abspath(output_tif_path)), exist_ok=True)

    # Choose appropriate overview factors based on image dimensions
    max_dim = max(w, h)
    factors = []
    f = 2
    while (max_dim // f) >= 128 and f <= 64:
        factors.append(f)
        f *= 2

    profile = {
        "driver": "GTiff",
        "height": h,
        "width": w,
        "count": 4,
        "dtype": "uint8",
        "crs": CRS.from_epsg(4326),
        "transform": aff,
        "compress": "deflate",
        "tiled": True,
        "blockxsize": min(256, max(16, (w // 16) * 16)) if w >= 256 else w,
        "blockysize": min(256, max(16, (h // 16) * 16)) if h >= 256 else h,
    }
    if w < 256 or h < 256:
        profile.pop("tiled", None)
        profile.pop("blockxsize", None)
        profile.pop("blockysize", None)

    with rasterio.open(output_tif_path, "w", **profile) as dst:
        dst.write(rgba)
        if factors:
            dst.build_overviews(factors, resamp_enum)
            dst.update_tags(ns="rio_overview", resampling=resamp_name)

    return {
        "success": True,
        "output_path": output_tif_path,
        "width": w,
        "height": h,
        "overview_factors": factors,
        "resampling": resamp_name,
    }


def _prepare_warped_source(layer):
    """
    Prepares in-memory RGBA source array + Affine + CRS for GDAL reproject.
    Respects 6-parameter affine rotation/skew AND accounts for enhanced/upscaled dimensions!
    """
    img_path = layer["image_path"]
    rgba = _load_rgba_array(img_path)
    _, h, w = rgba.shape

    opacity = float(layer.get("opacity", 1.0))
    if 0.0 <= opacity < 0.999:
        rgba[3] = np.clip(rgba[3].astype(np.float32) * opacity, 0, 255).astype(np.uint8)

    bounds = [float(v) for v in layer["bounds"][:4]]
    west, south, east, north = bounds

    t6 = layer.get("transform")
    orig_w = float(layer.get("original_width") or w)
    orig_h = float(layer.get("original_height") or h)

    if t6 and len(t6) == 6:
        # Scale pixel step if enhanced image dimensions differ from original georeferenced dimensions
        sx = orig_w / float(max(1, w))
        sy = orig_h / float(max(1, h))
        aff = Affine(
            float(t6[0]) * sx,
            float(t6[1]) * sy,
            float(t6[2]),
            float(t6[3]) * sx,
            float(t6[4]) * sy,
            float(t6[5]),
        )
    else:
        aff = from_bounds(west, south, east, north, w, h)

    resamp_enum, resamp_name = _select_resampling(
        layer.get("resampling"), layer.get("classification", "")
    )

    return {
        "rgba": rgba,
        "width": w,
        "height": h,
        "bounds": bounds,
        "affine": aff,
        "crs": CRS.from_epsg(4326),
        "resampling": resamp_enum,
        "resampling_name": resamp_name,
    }


def create_mbtiles(output_mbtiles_path, spec):
    """
    Phase 5.3 & Phase 7: Generates a genuine SQLite MBTiles 1.3 database using GDAL (rasterio.warp.reproject)
    with data-aware resampling, blank-tile pruning, TMS tile_row orientation, and native-resolution tracking.
    """
    layers_cfg = spec.get("layers") or []
    if not layers_cfg and spec.get("image_path"):
        layers_cfg = [spec]

    valid_sources = []
    for lyr in layers_cfg:
        if not lyr.get("image_path") or not os.path.exists(lyr["image_path"]):
            continue
        if not lyr.get("bounds") or len(lyr["bounds"]) < 4:
            continue
        valid_sources.append(_prepare_warped_source(lyr))

    if not valid_sources:
        return {"success": False, "code": "NO_VALID_LAYERS", "error": "No valid georeferenced raster layers available for MBTiles generation"}

    # Compute union geographic bounds [west, south, east, north]
    u_west = min(s["bounds"][0] for s in valid_sources)
    u_south = min(s["bounds"][1] for s in valid_sources)
    u_east = max(s["bounds"][2] for s in valid_sources)
    u_north = max(s["bounds"][3] for s in valid_sources)

    u_west = max(-180.0, min(180.0, u_west))
    u_east = max(-180.0, min(180.0, u_east))
    u_south = max(-85.05112878, min(85.05112878, u_south))
    u_north = max(-85.05112878, min(85.05112878, u_north))

    if u_west >= u_east or u_south >= u_north:
        return {"success": False, "code": "INVALID_BOUNDS", "error": f"Invalid geographic bounds: [{u_west}, {u_south}, {u_east}, {u_north}]"}

    union_bounds = [u_west, u_south, u_east, u_north]

    # Compute native zoom range across layers
    native_mins = []
    native_maxs = []
    for s in valid_sources:
        mn, mx, _ = _compute_native_zoom_range(s["bounds"], s["width"], s["height"])
        native_mins.append(mn)
        native_maxs.append(mx)

    native_min_z = min(native_mins)
    native_max_z = max(native_maxs)

    req_min_z = spec.get("min_zoom")
    req_max_z = spec.get("max_zoom")
    allow_upscale = bool(spec.get("allow_upscale_zooms", False))

    min_z = min(7, int(req_min_z)) if req_min_z is not None else min(7, native_min_z)
    if min_z > native_min_z and req_min_z is None:
        min_z = native_min_z

    if req_max_z is not None:
        max_z = int(req_max_z)
        if not allow_upscale and max_z > native_max_z:
            # Cap at native_max_z unless explicitly marked/requested
            max_z = min(max_z, native_max_z + 1)
    else:
        max_z = native_max_z

    min_z = max(0, min(22, min_z))
    max_z = max(min_z, min(22, max_z))

    tile_format = str(spec.get("tile_format") or "png").lower()
    if tile_format not in ("png", "jpg", "jpeg", "webp"):
        tile_format = "png"
    if tile_format == "jpeg":
        tile_format = "jpg"

    max_total_tiles = int(spec.get("max_total_tiles") or 1200)
    dst_crs = CRS.from_epsg(3857)

    os.makedirs(os.path.dirname(os.path.abspath(output_mbtiles_path)), exist_ok=True)
    if os.path.exists(output_mbtiles_path):
        os.remove(output_mbtiles_path)

    conn = sqlite3.connect(output_mbtiles_path)
    try:
        cur = conn.cursor()
        cur.execute("PRAGMA journal_mode=DELETE;")
        cur.execute("PRAGMA synchronous=NORMAL;")
        cur.execute("CREATE TABLE metadata (name text, value text);")
        cur.execute("CREATE UNIQUE INDEX name ON metadata (name);")
        cur.execute(
            "CREATE TABLE tiles (zoom_level integer, tile_column integer, tile_row integer, tile_data blob);"
        )
        cur.execute("CREATE UNIQUE INDEX tile_index ON tiles (zoom_level, tile_column, tile_row);")

        total_tiles_written = 0
        generated_zooms = []
        upscaled_zooms = []
        include_basemap = False
        if spec.get("extra_metadata") and isinstance(spec["extra_metadata"], dict):
            include_basemap = bool(spec["extra_metadata"].get("include_basemap"))

        # Super-tile chunk size (8x8 tiles = 2048x2048 px per GDAL C++ warp call)
        CHUNK_TILES = 8

        for z in range(min_z, max_z + 1):
            ul_tile = mercantile.tile(u_west, u_north, z)
            lr_tile = mercantile.tile(u_east, u_south, z)
            tx_min = min(ul_tile.x, lr_tile.x)
            tx_max = max(ul_tile.x, lr_tile.x)
            ty_min = min(ul_tile.y, lr_tile.y)
            ty_max = max(ul_tile.y, lr_tile.y)

            zoom_grid_count = (tx_max - tx_min + 1) * (ty_max - ty_min + 1)
            if total_tiles_written + zoom_grid_count > max_total_tiles and len(generated_zooms) >= 2:
                break

            zoom_wrote = 0

            for cx0 in range(tx_min, tx_max + 1, CHUNK_TILES):
                cx1 = min(tx_max, cx0 + CHUNK_TILES - 1)
                for cy0 in range(ty_min, ty_max + 1, CHUNK_TILES):
                    cy1 = min(ty_max, cy0 + CHUNK_TILES - 1)

                    tiles_w = cx1 - cx0 + 1
                    tiles_h = cy1 - cy0 + 1
                    px_w = tiles_w * 256
                    px_h = tiles_h * 256

                    left_m, _, _, top_m = _tile_mercator_bounds(cx0, cy0, z)
                    _, bottom_m, right_m, _ = _tile_mercator_bounds(cx1, cy1, z)
                    dst_transform = from_bounds(left_m, bottom_m, right_m, top_m, px_w, px_h)

                    canvas = np.zeros((4, px_h, px_w), dtype=np.uint8)
                    any_layer_hit = False

                    for src in valid_sources:
                        # Quick bbox intersection check in WGS84
                        sb = src["bounds"]
                        c_ul = mercantile.ul(cx0, cy0, z)
                        c_lr = mercantile. bounds(cx1, cy1, z)
                        if sb[2] <= c_ul.lng or sb[0] >= c_lr.east or sb[3] <= c_lr.south or sb[1] >= c_ul.lat:
                            continue

                        warped = np.zeros((4, px_h, px_w), dtype=np.uint8)
                        reproject(
                            source=src["rgba"],
                            destination=warped,
                            src_transform=src["affine"],
                            src_crs=src["crs"],
                            dst_transform=dst_transform,
                            dst_crs=dst_crs,
                            resampling=src["resampling"],
                            src_nodata=None,
                            dst_nodata=0,
                        )

                        if np.max(warped[3]) == 0:
                            continue

                        # Line preservation for overview zooms:
                        # When a raster with transparency (e.g. CAD drawing or transparent PDF) is downsampled
                        # at overview zooms (z < native_max_z), thin 1-pixel lines are diluted into near-zero
                        # opacity (alpha 1-15), rendering them invisible on map viewers.
                        # Non-linearly boost surviving line pixels so drawings remain crisp across all zoom levels.
                        if z < native_max_z:
                            line_mask = warped[3] > 4
                            if np.any(line_mask):
                                boost = min(8.0, 1.0 + float(native_max_z - z) * 1.5)
                                boosted_a = np.clip(warped[3].astype(np.float32) * boost, 0, 255).astype(np.uint8)
                                warped[3] = np.where(line_mask, boosted_a, warped[3])
                                dark_mask = line_mask & (warped[0] < 220) & (warped[1] < 220) & (warped[2] < 220)
                                if np.any(dark_mask):
                                    for ch in range(3):
                                        warped[ch] = np.where(dark_mask, np.clip(warped[ch].astype(np.float32) * 0.75, 0, 255).astype(np.uint8), warped[ch])

                        any_layer_hit = True
                        if np.max(canvas[3]) == 0:
                            canvas = warped
                        else:
                            # Alpha-over composite (src over canvas)
                            sa = warped[3].astype(np.float32) / 255.0
                            da = canvas[3].astype(np.float32) / 255.0
                            out_a = sa + da * (1.0 - sa)
                            safe_a = np.maximum(out_a, 1e-6)
                            for ch in range(3):
                                sc = warped[ch].astype(np.float32)
                                dc = canvas[ch].astype(np.float32)
                                canvas[ch] = np.clip((sc * sa + dc * da * (1.0 - sa)) / safe_a, 0, 255).astype(np.uint8)
                            canvas[3] = np.clip(out_a * 255.0, 0, 255).astype(np.uint8)

                    if not any_layer_hit:
                        # If at very low overview zoom where dataset footprint is sub-pixel (< 1px),
                        # ensure at least the center tile renders a crisp footprint pixel/overview dot
                        if z < native_min_z:
                            c_lng = (u_west + u_east) / 2.0
                            c_lat = (u_south + u_north) / 2.0
                            ct = mercantile.tile(c_lng, c_lat, z)
                            if cx0 <= ct.x <= cx1 and cy0 <= ct.y <= cy1:
                                mx, my = _lnglat_to_mercator(c_lng, c_lat)
                                px_x = int((mx - left_m) / max(1e-6, right_m - left_m) * px_w)
                                px_y = int((top_m - my) / max(1e-6, top_m - bottom_m) * px_h)
                                px_x = max(2, min(px_w - 3, px_x))
                                px_y = max(2, min(px_h - 3, px_y))
                                # Sample average opaque color from primary source
                                prim = valid_sources[0]["rgba"]
                                mask = prim[3] > 16
                                if np.any(mask):
                                    r_m = int(np.mean(prim[0][mask]))
                                    g_m = int(np.mean(prim[1][mask]))
                                    b_m = int(np.mean(prim[2][mask]))
                                else:
                                    r_m, g_m, b_m = 30, 100, 210
                                canvas[0, px_y - 1 : px_y + 2, px_x - 1 : px_x + 2] = r_m
                                canvas[1, px_y - 1 : px_y + 2, px_x - 1 : px_x + 2] = g_m
                                canvas[2, px_y - 1 : px_y + 2, px_x - 1 : px_x + 2] = b_m
                                canvas[3, px_y - 1 : px_y + 2, px_x - 1 : px_x + 2] = 255
                            else:
                                continue
                        else:
                            continue

                    # Slice 256x256 tiles and prune blank tiles
                    batch_rows = []
                    for dy in range(tiles_h):
                        y_xyz = cy0 + dy
                        y_tms = _xyz_to_tms(y_xyz, z)
                        y0, y1 = dy * 256, (dy + 1) * 256
                        for dx in range(tiles_w):
                            x_col = cx0 + dx
                            x0, x1 = dx * 256, (dx + 1) * 256
                            tile_rgba = canvas[:, y0:y1, x0:x1]
                            if np.max(tile_rgba[3]) == 0:
                                continue

                            tile_hwc = np.transpose(tile_rgba, (1, 2, 0))  # RGBA
                            if include_basemap and np.min(tile_hwc[:, :, 3]) < 255:
                                bm_rgba = _fetch_basemap_tile(z, x_col, y_xyz)
                                if bm_rgba is not None:
                                    over_a = tile_hwc[:, :, 3:4].astype(np.float32) / 255.0
                                    comp_rgb = (tile_hwc[:, :, :3].astype(np.float32) * over_a + bm_rgba[:, :, :3].astype(np.float32) * (1.0 - over_a)).astype(np.uint8)
                                    tile_hwc = np.dstack([comp_rgb, np.full((256, 256, 1), 255, dtype=np.uint8)])
                            if tile_format == "jpg":
                                # Composite onto white background for JPEG
                                alpha_f = tile_hwc[:, :, 3:4].astype(np.float32) / 255.0
                                rgb = (tile_hwc[:, :, :3].astype(np.float32) * alpha_f + 255.0 * (1.0 - alpha_f)).astype(np.uint8)
                                bgr_t = cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)
                                ok, enc = cv2.imencode(".jpg", bgr_t, [int(cv2.IMWRITE_JPEG_QUALITY), 90])
                            elif tile_format == "webp":
                                bgra_t = cv2.cvtColor(tile_hwc, cv2.COLOR_RGBA2BGRA)
                                ok, enc = cv2.imencode(".webp", bgra_t, [int(cv2.IMWRITE_WEBP_QUALITY), 90])
                            else:
                                bgra_t = cv2.cvtColor(tile_hwc, cv2.COLOR_RGBA2BGRA)
                                ok, enc = cv2.imencode(".png", bgra_t, [int(cv2.IMWRITE_PNG_COMPRESSION), 6])

                            if ok:
                                batch_rows.append((z, x_col, y_tms, sqlite3.Binary(enc.tobytes())))

                    if batch_rows:
                        cur.executemany(
                            "INSERT OR REPLACE INTO tiles (zoom_level, tile_column, tile_row, tile_data) VALUES (?, ?, ?, ?);",
                            batch_rows,
                        )
                        zoom_wrote += len(batch_rows)
                        total_tiles_written += len(batch_rows)

            if zoom_wrote > 0:
                generated_zooms.append(z)
                if z > native_max_z:
                    upscaled_zooms.append(z)

        actual_min_z = generated_zooms[0] if generated_zooms else min_z
        actual_max_z = generated_zooms[-1] if generated_zooms else max_z
        init_z = spec.get("initial_zoom")
        if init_z is None:
            init_z = min(actual_max_z, max(actual_min_z, native_max_z - 1))

        c_lng = (u_west + u_east) / 2.0
        c_lat = (u_south + u_north) / 2.0
        name = str(spec.get("name") or os.path.splitext(os.path.basename(output_mbtiles_path))[0])
        desc = str(spec.get("description") or "Generated by Veracity Maps GDAL/Rasterio Engine")
        attr = str(spec.get("attribution") or "Veracity Maps")
        layer_type = str(spec.get("type") or ("baselayer" if include_basemap else "overlay"))
        resamp_used = valid_sources[0]["resampling_name"]

        meta_rows = [
            ("name", name),
            ("format", tile_format),
            ("type", layer_type),
            ("version", "1.3.0"),
            ("description", desc),
            ("attribution", attr),
            ("bounds", f"{u_west:.8f},{u_south:.8f},{u_east:.8f},{u_north:.8f}"),
            ("center", f"{c_lng:.8f},{c_lat:.8f},{init_z}"),
            ("minzoom", str(actual_min_z)),
            ("maxzoom", str(actual_max_z)),
            ("scheme", "tms"),
            ("crs", "EPSG:3857"),
            ("resampling", resamp_used),
            ("native_maxzoom", str(native_max_z)),
            ("upscaled_beyond_native", "true" if upscaled_zooms else "false"),
        ]
        if upscaled_zooms:
            meta_rows.append(("upscaled_zooms", ",".join(str(z) for z in upscaled_zooms)))
        if spec.get("extra_metadata") and isinstance(spec["extra_metadata"], dict):
            meta_rows.append(("veracity_json", json.dumps(spec["extra_metadata"])))

        cur.executemany("INSERT OR REPLACE INTO metadata (name, value) VALUES (?, ?);", meta_rows)
        conn.commit()
    finally:
        conn.close()

    return {
        "success": True,
        "mbtiles_path": output_mbtiles_path,
        "tile_format": tile_format,
        "scheme": "tms",
        "bounds": union_bounds,
        "center": [c_lng, c_lat, init_z],
        "min_zoom": actual_min_z,
        "max_zoom": actual_max_z,
        "native_max_zoom": native_max_z,
        "upscaled_beyond_native": bool(upscaled_zooms),
        "upscaled_zooms": upscaled_zooms,
        "tile_count": total_tiles_written,
        "available_zooms": generated_zooms,
        "resampling": resamp_used,
    }


def _sniff_tile_format(blob):
    if not blob or len(blob) < 4:
        return "unknown"
    b = bytes(blob[:16])
    if b.startswith(b"\x89PNG\r\n\x1a\n"):
        return "png"
    if b.startswith(b"\xff\xd8\xff"):
        return "jpg"
    if b[:4] == b"RIFF" and b[8:12] == b"WEBP":
        return "webp"
    if b.startswith(b"\x1f\x8b"):
        return "pbf"
    # Raw uncompressed Mapbox Vector Tile protobuf varint tag check
    if (b[0] >> 3) in (1, 2, 3, 15) and (b[0] & 0x07) in (0, 2):
        return "pbf"
    return "unknown"


def inspect_mbtiles(mbtiles_path, preview_out_path="", unpack_dir=""):
    """
    Phase 7.1: Validates an SQLite/MBTiles database, inspects metadata, detects true tile format,
    verifies TMS vs XYZ coordinate orientation, computes geographic bounds if absent,
    renders an overview preview PNG (for raster MBTiles), and optionally unpacks small tile sets
    to an XYZ disk cache for sub-millisecond Go HTTP serving without lingering SQLite file locks.
    """
    if not os.path.exists(mbtiles_path):
        return {"success": False, "code": "FILE_NOT_FOUND", "error": f"MBTiles file not found: {mbtiles_path}"}

    file_size = os.path.getsize(mbtiles_path)
    if file_size < 100:
        return {"success": False, "code": "INVALID_MBTILES", "error": "File is too small to be a valid SQLite MBTiles database"}

    with open(mbtiles_path, "rb") as f:
        header = f.read(16)
    if header != b"SQLite format 3\x00":
        return {"success": False, "code": "INVALID_MBTILES", "error": "File is not a valid SQLite 3 database (invalid magic header)"}

    conn = None
    try:
        conn = sqlite3.connect(mbtiles_path)
        cur = conn.cursor()

        # Verify schema
        cur.execute("SELECT name FROM sqlite_master WHERE type IN ('table', 'view');")
        tables = {row[0].lower() for row in cur.fetchall()}
        if "tiles" not in tables:
            return {"success": False, "code": "INVALID_MBTILES", "error": "SQLite database is missing required 'tiles' table or view"}

        meta = {}
        if "metadata" in tables:
            try:
                cur.execute("SELECT name, value FROM metadata;")
                for k, v in cur.fetchall():
                    if k is not None and v is not None:
                        meta[str(k).strip().lower()] = str(v).strip()
            except Exception:
                pass

        cur.execute("SELECT COUNT(*), MIN(zoom_level), MAX(zoom_level) FROM tiles WHERE tile_data IS NOT NULL AND LENGTH(tile_data) > 0;")
        row = cur.fetchone()
        tile_count = int(row[0] or 0)
        if tile_count <= 0:
            return {"success": False, "code": "EMPTY_MBTILES", "error": "MBTiles database contains zero usable tiles"}

        db_min_z = int(row[1])
        db_max_z = int(row[2])

        min_z = db_min_z
        max_z = db_max_z
        if "minzoom" in meta:
            try:
                min_z = max(0, min(db_min_z, int(float(meta["minzoom"]))))
            except Exception:
                pass
        if "maxzoom" in meta:
            try:
                max_z = min(24, max(db_max_z, int(float(meta["maxzoom"]))))
            except Exception:
                pass

        # Sniff actual tile format from tile_data
        cur.execute("SELECT tile_data FROM tiles WHERE tile_data IS NOT NULL AND LENGTH(tile_data) > 0 LIMIT 1;")
        sample_blob = cur.fetchone()[0]
        sniffed_fmt = _sniff_tile_format(sample_blob)
        declared_fmt = meta.get("format", "").lower()
        if declared_fmt == "jpeg":
            declared_fmt = "jpg"
        if declared_fmt == "mvt":
            declared_fmt = "pbf"

        tile_format = sniffed_fmt if sniffed_fmt != "unknown" else (declared_fmt or "png")
        is_gzipped_pbf = tile_format == "pbf" and bytes(sample_blob[:2]) == b"\x1f\x8b"

        # Parse declared bounds if valid
        bounds = None
        if "bounds" in meta:
            try:
                parts = [float(p.strip()) for p in meta["bounds"].split(",")]
                if len(parts) == 4 and -180.01 <= parts[0] < parts[2] <= 180.01 and -90.01 <= parts[1] < parts[3] <= 90.01:
                    bounds = [
                        max(-180.0, min(180.0, parts[0])),
                        max(-85.05112878, min(85.05112878, parts[1])),
                        max(-180.0, min(180.0, parts[2])),
                        max(-85.05112878, min(85.05112878, parts[3])),
                    ]
            except Exception:
                bounds = None

        # Determine TMS vs XYZ scheme (MBTiles 1.3 default is TMS; verify geometrically against bounds when available)
        declared_scheme = meta.get("scheme", "tms").lower()
        scheme = "xyz" if declared_scheme == "xyz" else "tms"

        cur.execute(
            "SELECT MIN(tile_column), MAX(tile_column), MIN(tile_row), MAX(tile_row) FROM tiles WHERE zoom_level = ?;",
            (db_max_z,),
        )
        min_col, max_col, min_row, max_row = [int(v) for v in cur.fetchone()]

        if bounds is not None and db_max_z >= 2:
            c_lng = (bounds[0] + bounds[2]) / 2.0
            c_lat = (bounds[1] + bounds[3]) / 2.0
            expected_xyz = mercantile.tile(c_lng, c_lat, db_max_z)
            expected_tms_row = _xyz_to_tms(expected_xyz.y, db_max_z)

            tms_matches = (min_row - 1) <= expected_tms_row <= (max_row + 1)
            xyz_matches = (min_row - 1) <= expected_xyz.y <= (max_row + 1)
            if xyz_matches and not tms_matches:
                scheme = "xyz"
            elif tms_matches and not xyz_matches:
                scheme = "tms"

        # If bounds were not in metadata, derive exact geographic bounds from highest zoom tiles
        if bounds is None:
            if scheme == "tms":
                y_north_xyz = _tms_to_xyz(max_row, db_max_z)
                y_south_xyz = _tms_to_xyz(min_row, db_max_z)
            else:
                y_north_xyz = min_row
                y_south_xyz = max_row
            b_nw = mercantile.bounds(min_col, y_north_xyz, db_max_z)
            b_se = mercantile.bounds(max_col, y_south_xyz, db_max_z)
            bounds = [b_nw.west, b_se.south, b_se.east, b_nw.north]

        # Parse center
        c_lng = (bounds[0] + bounds[2]) / 2.0
        c_lat = (bounds[1] + bounds[3]) / 2.0
        init_zoom = float(min(db_max_z, max(db_min_z, (db_min_z + db_max_z) // 2 + 1)))
        if "center" in meta:
            try:
                c_parts = [float(p.strip()) for p in meta["center"].split(",")]
                if len(c_parts) >= 2 and -180 <= c_parts[0] <= 180 and -90 <= c_parts[1] <= 90:
                    c_lng, c_lat = c_parts[0], c_parts[1]
                if len(c_parts) >= 3:
                    init_zoom = c_parts[2]
            except Exception:
                pass

        # Parse vector_layers JSON for PBF vector MBTiles
        vector_layers = []
        if "json" in meta:
            try:
                jdoc = json.loads(meta["json"])
                if isinstance(jdoc, dict) and isinstance(jdoc.get("vector_layers"), list):
                    vector_layers = jdoc["vector_layers"]
            except Exception:
                pass

        # Query distinct zoom levels
        cur.execute("SELECT DISTINCT zoom_level FROM tiles ORDER BY zoom_level ASC;")
        available_zooms = [int(r[0]) for r in cur.fetchall()]

        # Generate overview preview PNG for raster MBTiles
        preview_width = 0
        preview_height = 0
        preview_generated = False
        if preview_out_path and tile_format in ("png", "jpg", "webp"):
            # Pick highest zoom level with <= 36 tiles (up to 6x6 = 1536x1536) for sharp preview
            chosen_z = available_zooms[0]
            for z_cand in available_zooms:
                cur.execute("SELECT COUNT(*) FROM tiles WHERE zoom_level = ?;", (z_cand,))
                cnt = int(cur.fetchone()[0])
                if 1 <= cnt <= 36:
                    chosen_z = z_cand

            cur.execute(
                "SELECT tile_column, tile_row, tile_data FROM tiles WHERE zoom_level = ?;",
                (chosen_z,),
            )
            z_rows = cur.fetchall()
            if z_rows:
                xyz_tiles = []
                for col, r_val, blob in z_rows:
                    y_xyz = _tms_to_xyz(int(r_val), chosen_z) if scheme == "tms" else int(r_val)
                    xyz_tiles.append((int(col), y_xyz, blob))

                xs = [t[0] for t in xyz_tiles]
                ys = [t[1] for t in xyz_tiles]
                x0_t, x1_t = min(xs), max(xs)
                y0_t, y1_t = min(ys), max(ys)
                # Clamp grid to max 8x8
                x1_t = min(x1_t, x0_t + 7)
                y1_t = min(y1_t, y0_t + 7)

                grid_w = (x1_t - x0_t + 1) * 256
                grid_h = (y1_t - y0_t + 1) * 256
                mosaic = np.zeros((grid_h, grid_w, 4), dtype=np.uint8)

                for col, y_xyz, blob in xyz_tiles:
                    if not (x0_t <= col <= x1_t and y0_t <= y_xyz <= y1_t):
                        continue
                    arr = np.frombuffer(blob, dtype=np.uint8)
                    dec = cv2.imdecode(arr, cv2.IMREAD_UNCHANGED)
                    if dec is None:
                        continue
                    if dec.ndim == 2:
                        dec = cv2.cvtColor(dec, cv2.COLOR_GRAY2BGRA)
                    elif dec.shape[2] == 3:
                        dec = cv2.cvtColor(dec, cv2.COLOR_BGR2BGRA)
                    if dec.shape[0] != 256 or dec.shape[1] != 256:
                        dec = cv2.resize(dec, (256, 256), interpolation=cv2.INTER_AREA)
                    ox = (col - x0_t) * 256
                    oy = (y_xyz - y0_t) * 256
                    mosaic[oy : oy + 256, ox : ox + 256] = dec

                os.makedirs(os.path.dirname(os.path.abspath(preview_out_path)), exist_ok=True)
                cv2.imwrite(preview_out_path, mosaic)
                preview_width = grid_w
                preview_height = grid_h
                preview_generated = True

        # Optional fast XYZ cache unpack for tile sets <= 25000 tiles
        unpacked_cache = False
        if unpack_dir and tile_count <= 25000:
            ext = tile_format
            cur.execute("SELECT zoom_level, tile_column, tile_row, tile_data FROM tiles WHERE tile_data IS NOT NULL;")
            for z_val, col, r_val, blob in cur:
                z_int = int(z_val)
                x_int = int(col)
                y_xyz = _tms_to_xyz(int(r_val), z_int) if scheme == "tms" else int(r_val)
                t_dir = os.path.join(unpack_dir, str(z_int), str(x_int))
                os.makedirs(t_dir, exist_ok=True)
                t_file = os.path.join(t_dir, f"{y_xyz}.{ext}")
                with open(t_file, "wb") as tf:
                    tf.write(blob)
            unpacked_cache = True

        return {
            "success": True,
            "name": meta.get("name") or os.path.splitext(os.path.basename(mbtiles_path))[0],
            "description": meta.get("description", ""),
            "attribution": meta.get("attribution", ""),
            "type": meta.get("type", "overlay"),
            "tile_format": tile_format,
            "is_gzipped_pbf": is_gzipped_pbf,
            "scheme": scheme,
            "bounds": bounds,
            "center": [c_lng, c_lat],
            "initial_zoom": init_zoom,
            "min_zoom": min_z,
            "max_zoom": max_z,
            "db_min_zoom": db_min_z,
            "db_max_zoom": db_max_z,
            "tile_count": tile_count,
            "available_zooms": available_zooms,
            "vector_layers": vector_layers,
            "raw_metadata": meta,
            "preview_generated": preview_generated,
            "preview_path": preview_out_path if preview_generated else "",
            "preview_width": preview_width,
            "preview_height": preview_height,
            "unpacked_cache": unpacked_cache,
        }
    except sqlite3.DatabaseError as db_err:
        return {"success": False, "code": "INVALID_MBTILES", "error": f"Corrupted SQLite MBTiles database: {str(db_err)}"}
    except Exception as e:
        return {"success": False, "code": "MBTILES_INSPECT_FAILED", "error": str(e)}
    finally:
        if conn is not None:
            conn.close()


def extract_single_tile_to_stdout(mbtiles_path, z, x_xyz, y_xyz, scheme="tms"):
    """Reads a single tile for large (>2500 tile) external MBTiles files and writes raw bytes to stdout.buffer."""
    row = _xyz_to_tms(y_xyz, z) if scheme == "tms" else y_xyz
    conn = sqlite3.connect(mbtiles_path)
    try:
        cur = conn.cursor()
        cur.execute(
            "SELECT tile_data FROM tiles WHERE zoom_level = ? AND tile_column = ? AND tile_row = ? LIMIT 1;",
            (z, x_xyz, row),
        )
        rec = cur.fetchone()
        if not rec or not rec[0]:
            sys.exit(2)  # Not found
        sys.stdout.buffer.write(bytes(rec[0]))
        sys.stdout.buffer.flush()
    finally:
        conn.close()


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "code": "INVALID_ARGS", "error": "Usage: mbtiles_tool.py <create|inspect|optimize_tif|get_tile> ..."}))
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "create":
        out_path = sys.argv[2]
        spec_json = sys.argv[3]
        if os.path.exists(spec_json) and spec_json.endswith(".json"):
            with open(spec_json, "r", encoding="utf-8") as sf:
                spec = json.load(sf)
        else:
            spec = json.loads(spec_json)
        res = create_mbtiles(out_path, spec)
        print(json.dumps(res))
    elif cmd == "inspect":
        mb_path = sys.argv[2]
        prev_path = sys.argv[3] if len(sys.argv) > 3 else ""
        unpack_path = sys.argv[4] if len(sys.argv) > 4 else ""
        res = inspect_mbtiles(mb_path, prev_path, unpack_path)
        print(json.dumps(res))
    elif cmd == "optimize_tif":
        in_p = sys.argv[2]
        out_p = sys.argv[3]
        spec = json.loads(sys.argv[4])
        res = optimize_geotiff(
            in_p,
            out_p,
            spec["bounds"],
            spec.get("transform"),
            spec.get("resampling", "lanczos"),
        )
        print(json.dumps(res))
    elif cmd == "get_tile":
        mb_path = sys.argv[2]
        z = int(sys.argv[3])
        x = int(sys.argv[4])
        y = int(sys.argv[5])
        scheme = sys.argv[6] if len(sys.argv) > 6 else "tms"
        extract_single_tile_to_stdout(mb_path, z, x, y, scheme)
    else:
        print(json.dumps({"success": False, "code": "UNKNOWN_COMMAND", "error": f"Unknown command: {cmd}"}))
        sys.exit(1)


if __name__ == "__main__":
    main()

