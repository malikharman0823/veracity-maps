import sys
import os
os.environ.setdefault("OPENCV_LOG_LEVEL", "OFF")
import json
import math
import pickle
import zipfile
import urllib.request
import warnings

warnings.filterwarnings("ignore")

import numpy as np
import cv2
from PIL import Image
import rasterio
from rasterio.transform import Affine
from rasterio.warp import transform as rio_transform, transform_bounds

REALESRGAN_PTH_URL = (
    "https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.5.0/realesr-general-x4v3.pth"
)


def _models_cache_dir():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    cache_dir = os.path.join(os.path.dirname(script_dir), "models_cache")
    os.makedirs(cache_dir, exist_ok=True)
    return cache_dir


def _load_image_rgba_or_rgb(path):
    """
    Loads any supported raster (PNG, JPG, WebP, BMP, 8/16-bit TIFF/GeoTIFF)
    into (bgr_or_gray_uint8, alpha_uint8_or_None, bit_depth, band_count, dtype_str).
    """
    raw = cv2.imread(path, cv2.IMREAD_UNCHANGED)
    if raw is not None:
        dtype_str = str(raw.dtype)
        bit_depth = raw.dtype.itemsize * 8
        if raw.dtype == np.uint16:
            # Normalize 16-bit image safely preserving dynamic range
            p_low, p_high = np.percentile(raw, (0.5, 99.5))
            if p_high <= p_low:
                raw8 = (raw >> 8).astype(np.uint8)
            else:
                raw8 = np.clip((raw.astype(np.float32) - p_low) / (p_high - p_low) * 255.0, 0, 255).astype(np.uint8)
        elif raw.dtype != np.uint8:
            p_low, p_high = np.percentile(raw, (1.0, 99.0))
            denom = max(float(p_high - p_low), 1e-6)
            raw8 = np.clip((raw.astype(np.float32) - p_low) / denom * 255.0, 0, 255).astype(np.uint8)
        else:
            raw8 = raw

        if raw8.ndim == 2:
            bgr = cv2.cvtColor(raw8, cv2.COLOR_GRAY2BGR)
            return bgr, None, bit_depth, 1, dtype_str
        elif raw8.shape[2] == 4:
            return raw8[:, :, :3], raw8[:, :, 3], bit_depth * 4, 4, dtype_str
        elif raw8.shape[2] == 3:
            return raw8, None, bit_depth * 3, 3, dtype_str

    # Fallback via rasterio for multi-band / scientific GeoTIFFs
    with rasterio.open(path) as src:
        count = src.count
        dtype_str = str(src.dtypes[0]) if count > 0 else "uint8"
        item_bits = np.dtype(dtype_str).itemsize * 8
        data = src.read()  # (bands, H, W)
        alpha = None

        def to_u8(arr):
            if arr.dtype == np.uint8:
                return arr
            finite = arr[np.isfinite(arr)]
            if finite.size == 0:
                return np.zeros(arr.shape, dtype=np.uint8)
            lo, hi = np.percentile(finite, (1.0, 99.0))
            if hi <= lo:
                hi = lo + 1.0
            return np.clip((arr.astype(np.float32) - lo) / (hi - lo) * 255.0, 0, 255).astype(np.uint8)

        if count == 1:
            g = to_u8(data[0])
            bgr = cv2.cvtColor(g, cv2.COLOR_GRAY2BGR)
            return bgr, None, item_bits, 1, dtype_str
        elif count == 2:
            g = to_u8(data[0])
            alpha = to_u8(data[1])
            bgr = cv2.cvtColor(g, cv2.COLOR_GRAY2BGR)
            return bgr, alpha, item_bits * 2, 2, dtype_str
        else:
            r = to_u8(data[0])
            g = to_u8(data[1])
            b = to_u8(data[2])
            if count >= 4:
                alpha = to_u8(data[3])
            bgr = np.stack([b, g, r], axis=-1)
            return bgr, alpha, item_bits * count, count, dtype_str


def _compute_quality_metrics(bgr, band_count, has_palette=False):
    h, w = bgr.shape[:2]
    max_side = max(h, w)
    if max_side > 1600:
        scale = 1600.0 / float(max_side)
        sample = cv2.resize(bgr, (max(1, int(w * scale)), max(1, int(h * scale))), interpolation=cv2.INTER_AREA)
    else:
        sample = bgr

    gray = cv2.cvtColor(sample, cv2.COLOR_BGR2GRAY)
    blur_score = float(cv2.Laplacian(gray, cv2.CV_64F).var())
    if blur_score < 90.0:
        blur_status = "blurry"
    elif blur_score < 300.0:
        blur_status = "moderate"
    else:
        blur_status = "sharp"

    contrast_score = float(np.std(gray))
    p2, p98 = np.percentile(gray, (2, 98))
    dynamic_range = float(p98 - p2)

    # Robust high-frequency noise estimate (Median Absolute Deviation)
    smooth = cv2.GaussianBlur(gray.astype(np.float32), (3, 3), 0.8)
    residual = np.abs(gray.astype(np.float32) - smooth)
    noise_estimate = float(np.median(residual) / 0.6745)

    # Fine-stroke / text annotation density
    edges = cv2.Canny(gray, 60, 160)
    edge_ratio = float(np.count_nonzero(edges)) / float(max(1, edges.size))
    # Morphological blackhat + tophat highlights thin text & dimension ticks
    kernel = cv2.getStructuringElement(cv2.MORPH_RECT, (9, 9))
    blackhat = cv2.morphologyEx(gray, cv2.MORPH_BLACKHAT, kernel)
    tophat = cv2.morphologyEx(gray, cv2.MORPH_TOPHAT, kernel)
    fine_strokes = np.maximum(blackhat, tophat)
    stroke_ratio = float(np.count_nonzero(fine_strokes > 28)) / float(max(1, gray.size))
    text_density = round(min(1.0, (edge_ratio * 2.5 + stroke_ratio * 1.5)), 4)

    if text_density >= 0.14:
        text_level = "high"
    elif text_density >= 0.05:
        text_level = "medium"
    else:
        text_level = "low"

    # Saturation check to distinguish drawings/scans from color photos
    hsv = cv2.cvtColor(sample, cv2.COLOR_BGR2HSV)
    mean_sat = float(np.mean(hsv[:, :, 1]))
    is_grayscale_like = band_count == 1 or mean_sat < 18.0

    # Background dominance check (engineering drawings & scanned sheets have dominant light/dark background)
    hist = cv2.calcHist([gray], [0], None, [16], [0, 256]).flatten()
    top2_share = float(np.sum(np.sort(hist)[-2:])) / float(max(1, gray.size))

    unique_vals = len(np.unique(gray[::4, ::4]))
    if has_palette or unique_vals <= 16:
        classification = "categorical_raster"
        resampling = "nearest"
    elif top2_share >= 0.62 and is_grayscale_like:
        if noise_estimate > 4.2 or dynamic_range < 150:
            classification = "scanned_document"
        else:
            classification = "engineering_drawing"
        resampling = "lanczos"
    elif top2_share >= 0.70 and stroke_ratio >= 0.025:
        classification = "engineering_drawing"
        resampling = "lanczos"
    else:
        classification = "raster_imagery"
        resampling = "lanczos" if blur_score >= 120 else "cubic"

    return {
        "blur_score": round(blur_score, 2),
        "blur_status": blur_status,
        "contrast_score": round(contrast_score, 2),
        "dynamic_range": round(dynamic_range, 2),
        "noise_estimate": round(noise_estimate, 2),
        "text_annotation_density": text_density,
        "text_density_level": text_level,
        "is_grayscale_like": bool(is_grayscale_like),
        "recommended_resampling": resampling,
        "classification": classification,
    }


def _build_recommendation(width, height, dpi, quality):
    max_dim = max(width, height)
    megapixels = (width * height) / 1_000_000.0
    is_high_res = max_dim >= 3600 or megapixels >= 10.0

    dpi_val = None
    if isinstance(dpi, dict):
        dpi_val = dpi.get("x") or dpi.get("y")

    upscale_rec = False
    suggested_upscale = 1
    if not is_high_res and (max_dim < 1400 or (dpi_val is not None and dpi_val < 130)):
        upscale_rec = True
        suggested_upscale = 2 if max_dim >= 700 else 4

    cls = quality["classification"]
    blur_status = quality["blur_status"]
    noise = quality["noise_estimate"]
    contrast = quality["contrast_score"]
    text_level = quality["text_density_level"]

    ocr_rec = cls in ("engineering_drawing", "scanned_document") and text_level in ("medium", "high")

    if cls == "categorical_raster":
        return {
            "recommended_preset": "none",
            "reason": "Categorical/paletted raster detected; preserve exact pixel classes with nearest-neighbour resampling.",
            "upscale_recommended": False,
            "suggested_upscale": 1,
            "ocr_recommended": False,
            "is_high_resolution": is_high_res,
        }

    if cls == "scanned_document":
        return {
            "recommended_preset": "text_clarity" if blur_status == "blurry" else "noise_reduction",
            "reason": "Scanned drawing detected; illumination normalization, gentle denoising, and edge-safe text clarity recommended.",
            "upscale_recommended": upscale_rec,
            "suggested_upscale": suggested_upscale,
            "ocr_recommended": True,
            "is_high_resolution": is_high_res,
        }

    if cls == "engineering_drawing":
        if blur_status == "blurry" or upscale_rec:
            return {
                "recommended_preset": "text_clarity",
                "reason": "Engineering drawing with soft edges or low pixel density; text clarity enhancement preserves fine lines and dimension labels.",
                "upscale_recommended": upscale_rec,
                "suggested_upscale": suggested_upscale,
                "ocr_recommended": ocr_rec,
                "is_high_resolution": is_high_res,
            }
        return {
            "recommended_preset": "line_preservation",
            "reason": "Sharp engineering drawing detected; line preservation keeps fine strokes, symbols, and dimensions crisp.",
            "upscale_recommended": False,
            "suggested_upscale": 1,
            "ocr_recommended": ocr_rec,
            "is_high_resolution": is_high_res,
        }

    # Continuous-tone raster imagery
    if noise > 6.5:
        preset = "noise_reduction"
        reason = "Elevated high-frequency sensor/compression noise detected; gentle bilateral noise reduction recommended."
    elif blur_status == "blurry" or contrast < 38.0:
        preset = "balanced"
        reason = "Low contrast or moderate softness detected; balanced CLAHE and edge-gated sharpening recommended."
    elif is_high_res and blur_status == "sharp":
        preset = "none"
        reason = "High-resolution sharp imagery detected; original raster quality is already optimal."
    else:
        preset = "balanced"
        reason = "Balanced contrast and gentle detail enhancement recommended."

    return {
        "recommended_preset": preset,
        "reason": reason,
        "upscale_recommended": upscale_rec,
        "suggested_upscale": suggested_upscale,
        "ocr_recommended": ocr_rec,
        "is_high_resolution": is_high_res,
    }


def inspect_raster(path):
    if not os.path.exists(path):
        return {"success": False, "code": "FILE_NOT_FOUND", "error": f"File not found: {path}"}

    dpi = None
    compression = "none"
    has_palette = False
    pil_format = None

    try:
        with Image.open(path) as pil_img:
            pil_format = pil_img.format
            has_palette = pil_img.mode == "P"
            raw_dpi = pil_img.info.get("dpi")
            if raw_dpi and len(raw_dpi) >= 2:
                dx, dy = float(raw_dpi[0]), float(raw_dpi[1])
                if dx > 1 and dy > 1 and math.isfinite(dx) and math.isfinite(dy):
                    dpi = {"x": round(dx, 2), "y": round(dy, 2)}
            comp = pil_img.info.get("compression")
            if comp:
                compression = str(comp).lower()
    except Exception:
        pass

    driver = (pil_format or os.path.splitext(path)[1].lstrip(".")).upper()
    if driver == "JPEG":
        driver = "JPG"
    if driver in ("TIF", "TIFF"):
        driver = "GTiff"

    width = 0
    height = 0
    crs_str = None
    native_crs_str = None
    is_georeferenced = False
    georef_source = ""
    bounds_wgs84 = None
    transform_wgs84 = None
    resolution_map = None
    color_interp = []

    try:
        with rasterio.open(path) as src:
            width = int(src.width)
            height = int(src.height)
            if src.driver:
                d_up = src.driver.upper()
                if d_up == "JPEG":
                    driver = "JPG"
                elif d_up == "GTIFF":
                    driver = "GTiff"
                else:
                    driver = d_up
            comp_tag = src.profile.get("compress") or src.tags(ns="IMAGE_STRUCTURE").get("COMPRESSION")
            if comp_tag:
                compression = str(comp_tag).lower()
            color_interp = [ci.name.capitalize() for ci in src.colorinterp]

            # Extract TIFF resolution tags if PIL didn't catch them
            if dpi is None:
                tags = src.tags()
                xres = tags.get("TIFFTAG_XRESOLUTION")
                yres = tags.get("TIFFTAG_YRESOLUTION")
                unit = tags.get("TIFFTAG_RESOLUTIONUNIT", "")
                if xres and yres:
                    try:
                        fx, fy = float(xres), float(yres)
                        if "cm" in str(unit).lower() or str(unit) == "3":
                            fx *= 2.54
                            fy *= 2.54
                        if fx > 1 and fy > 1:
                            dpi = {"x": round(fx, 2), "y": round(fy, 2)}
                    except Exception:
                        pass

            # Check embedded georeferencing (GeoTIFF CRS + Affine or GCPs)
            has_non_identity_transform = src.transform is not None and src.transform != Affine.identity()
            if src.crs is not None and has_non_identity_transform:
                native_crs_str = src.crs.to_string()
                t = src.transform
                # Project 4 image corners to EPSG:4326
                cols = [0.0, float(width), float(width), 0.0]
                rows = [0.0, 0.0, float(height), float(height)]
                xs = [t.a * c + t.b * r + t.c for c, r in zip(cols, rows)]
                ys = [t.d * c + t.e * r + t.f for c, r in zip(cols, rows)]
                lngs, lats = rio_transform(src.crs, "EPSG:4326", xs, ys)

                west, south, east, north = transform_bounds(
                    src.crs, "EPSG:4326", src.bounds.left, src.bounds.bottom, src.bounds.right, src.bounds.top, densify_pts=21
                )
                if (
                    all(math.isfinite(v) for v in (west, south, east, north))
                    and -180.001 <= west < east <= 180.001
                    and -90.001 <= south < north <= 90.001
                ):
                    west = max(-180.0, min(180.0, float(west)))
                    east = max(-180.0, min(180.0, float(east)))
                    south = max(-90.0, min(90.0, float(south)))
                    north = max(-90.0, min(90.0, float(north)))
                    bounds_wgs84 = {
                        "min_x": west,
                        "min_y": south,
                        "max_x": east,
                        "max_y": north,
                    }
                    # Construct WGS84 6-parameter affine [a, b, c, d, e, f] from corners:
                    # TL=(lngs[0], lats[0]), TR=(lngs[1], lats[1]), BL=(lngs[3], lats[3])
                    a_wgs = (lngs[1] - lngs[0]) / max(1.0, float(width))
                    b_wgs = (lngs[3] - lngs[0]) / max(1.0, float(height))
                    c_wgs = float(lngs[0])
                    d_wgs = (lats[1] - lats[0]) / max(1.0, float(width))
                    e_wgs = (lats[3] - lats[0]) / max(1.0, float(height))
                    f_wgs = float(lats[0])
                    transform_wgs84 = [a_wgs, b_wgs, c_wgs, d_wgs, e_wgs, f_wgs]
                    resolution_map = {"x": abs(a_wgs), "y": abs(e_wgs)}
                    crs_str = "EPSG:4326"
                    is_georeferenced = True
                    georef_source = "embedded_geotiff"
            elif src.gcps and len(src.gcps[0]) >= 3 and src.gcps[1] is not None:
                gcps, gcp_crs = src.gcps
                native_crs_str = gcp_crs.to_string()
                gx = [g.x for g in gcps]
                gy = [g.y for g in gcps]
                lngs, lats = rio_transform(gcp_crs, "EPSG:4326", gx, gy)
                west, east = min(lngs), max(lngs)
                south, north = min(lats), max(lats)
                if -180.0 <= west < east <= 180.0 and -90.0 <= south < north <= 90.0:
                    bounds_wgs84 = {"min_x": west, "min_y": south, "max_x": east, "max_y": north}
                    crs_str = "EPSG:4326"
                    is_georeferenced = True
                    georef_source = "embedded_gcps"
    except Exception:
        pass

    try:
        bgr, alpha, color_depth_bits, band_count, dtype_str = _load_image_rgba_or_rgb(path)
    except Exception as e:
        return {"success": False, "code": "CORRUPTED_RASTER", "error": f"Failed to decode raster: {str(e)}"}

    if height <= 0 or width <= 0:
        height, width = bgr.shape[:2]

    if not color_interp:
        if band_count == 1:
            color_interp = ["Gray"]
        elif band_count == 4 or alpha is not None:
            color_interp = ["Red", "Green", "Blue", "Alpha"]
        else:
            color_interp = ["Red", "Green", "Blue"]

    quality = _compute_quality_metrics(bgr, band_count, has_palette=has_palette)
    cls = quality.pop("classification")
    recommendation = _build_recommendation(width, height, dpi, {**quality, "classification": cls})

    return {
        "success": True,
        "driver": driver,
        "width": width,
        "height": height,
        "bands": band_count,
        "dtype": dtype_str,
        "color_depth_bits": color_depth_bits,
        "compression": compression,
        "dpi": dpi,
        "color_interpretation": color_interp,
        "is_georeferenced": is_georeferenced,
        "georeference_source": georef_source,
        "crs": crs_str,
        "native_crs": native_crs_str,
        "bounds": bounds_wgs84,
        "transform": transform_wgs84,
        "resolution": resolution_map,
        "content": {
            "has_raster": True,
            "has_vector": False,
            "has_embedded_text": False,
            "classification": cls,
        },
        "quality": quality,
        "recommendation": recommendation,
    }


# ---------------------------------------------------------------------------
# Real-ESRGAN (realesr-general-x4v3 SRVGGNetCompact) Pure-ONNX Super-Resolution
# ---------------------------------------------------------------------------

class _TorchlessStateDictLoader:
    """Reads official PyTorch .pth state_dict archives without importing torch."""

    @staticmethod
    def load(pth_path):
        tensors = {}
        with zipfile.ZipFile(pth_path, "r") as zf:
            names = zf.namelist()
            pkl_name = next((n for n in names if n.endswith("data.pkl")), None)
            if not pkl_name:
                raise RuntimeError("Invalid .pth archive: missing data.pkl")
            prefix = pkl_name[:-8]  # e.g. "archive/"

            def read_storage(storage_key, numel, dtype):
                data_entry = f"{prefix}data/{storage_key}"
                raw_bytes = zf.read(data_entry)
                return np.frombuffer(raw_bytes, dtype=dtype, count=numel).copy()

            class StorageRef:
                def __init__(self, dtype, key, size):
                    self.dtype = dtype
                    self.key = key
                    self.size = size

            def rebuild_tensor_v2(storage, storage_offset, size, stride, requires_grad, backward_hooks, metadata=None):
                arr = read_storage(storage.key, storage.size, storage.dtype)
                total = 1
                for s in size:
                    total *= s
                sliced = arr[storage_offset : storage_offset + total]
                return sliced.reshape(size)

            class Unpickler(pickle.Unpickler):
                def find_class(self, module, name):
                    if module == "torch._utils" and name == "_rebuild_tensor_v2":
                        return rebuild_tensor_v2
                    if module == "torch" and "FloatStorage" in name:
                        return np.float32
                    if module == "torch" and "HalfStorage" in name:
                        return np.float16
                    if module == "torch" and "LongStorage" in name:
                        return np.int64
                    if module == "collections" and name == "OrderedDict":
                        from collections import OrderedDict
                        return OrderedDict
                    return super().find_class(module, name)

                def persistent_load(self, pid):
                    # pid format: ('storage', storage_type, key, location, size)
                    if isinstance(pid, tuple) and pid[0] == "storage":
                        _, storage_dtype, key, _, size = pid
                        return StorageRef(storage_dtype, key, size)
                    raise pickle.UnpicklingError(f"Unsupported persistent id: {pid}")

            with zf.open(pkl_name, "r") as pf:
                obj = Unpickler(pf).load()

        if isinstance(obj, dict) and "params" in obj:
            tensors = obj["params"]
        elif isinstance(obj, dict) and "params_ema" in obj:
            tensors = obj["params_ema"]
        else:
            tensors = obj
        return tensors


def _ensure_realesrgan_onnx():
    """
    Ensures official Real-ESRGAN (realesr-general-x4v3, SRVGGNetCompact x4) ONNX model
    is cached in backend/models_cache/realesr-general-x4v3.onnx.
    """
    import onnx
    from onnx import helper, TensorProto, numpy_helper

    cache_dir = _models_cache_dir()
    onnx_path = os.path.join(cache_dir, "realesr-general-x4v3.onnx")
    if os.path.exists(onnx_path) and os.path.getsize(onnx_path) > 1_000_000:
        return onnx_path

    pth_path = os.path.join(cache_dir, "realesr-general-x4v3.pth")
    if not os.path.exists(pth_path) or os.path.getsize(pth_path) < 1_000_000:
        urllib.request.urlretrieve(REALESRGAN_PTH_URL, pth_path)

    weights = _TorchlessStateDictLoader.load(pth_path)
    # SRVGGNetCompact: body.0 (Conv 3->64), body.1 (PReLU 64), ..., body.64 (Conv 64->64), body.65 (PReLU 64), body.66 (Conv 64->48)
    # Count conv/prelu layers dynamically from state_dict keys
    max_idx = max(int(k.split(".")[1]) for k in weights.keys() if k.startswith("body."))

    initializers = []
    nodes = []

    prev_out = "input"
    layer_idx = 0
    while layer_idx <= max_idx:
        w_key = f"body.{layer_idx}.weight"
        b_key = f"body.{layer_idx}.bias"
        w_arr = weights[w_key].astype(np.float32)

        if w_arr.ndim == 4:
            # Conv2d 3x3, pad=1
            b_arr = weights[b_key].astype(np.float32)
            initializers.append(numpy_helper.from_array(w_arr, name=w_key))
            initializers.append(numpy_helper.from_array(b_arr, name=b_key))
            conv_out = f"conv_out_{layer_idx}"
            nodes.append(
                helper.make_node(
                    "Conv",
                    inputs=[prev_out, w_key, b_key],
                    outputs=[conv_out],
                    kernel_shape=[3, 3],
                    pads=[1, 1, 1, 1],
                )
            )
            prev_out = conv_out
        elif w_arr.ndim == 1:
            # PReLU(num_parameters=C) -> reshape slope to (C, 1, 1) for ONNX PRelu unidirectional broadcast
            slope = w_arr.reshape(-1, 1, 1).astype(np.float32)
            s_name = f"prelu_slope_{layer_idx}"
            initializers.append(numpy_helper.from_array(slope, name=s_name))
            prelu_out = f"prelu_out_{layer_idx}"
            nodes.append(helper.make_node("PRelu", inputs=[prev_out, s_name], outputs=[prelu_out]))
            prev_out = prelu_out
        layer_idx += 1

    # DepthToSpace (CRD mode = PyTorch PixelShuffle, blocksize=4)
    nodes.append(
        helper.make_node(
            "DepthToSpace",
            inputs=[prev_out],
            outputs=["residual_hr"],
            blocksize=4,
            mode="CRD",
        )
    )

    # Base nearest-neighbor x4 skip connection (exact match to SRVGGNetCompact F.interpolate(..., scale_factor=4, mode='nearest'))
    scales = np.array([1.0, 1.0, 4.0, 4.0], dtype=np.float32)
    roi = np.array([], dtype=np.float32)
    initializers.append(numpy_helper.from_array(scales, name="resize_scales"))
    initializers.append(numpy_helper.from_array(roi, name="resize_roi"))
    nodes.append(
        helper.make_node(
            "Resize",
            inputs=["input", "resize_roi", "resize_scales"],
            outputs=["base_hr"],
            mode="nearest",
        )
    )

    nodes.append(helper.make_node("Add", inputs=["residual_hr", "base_hr"], outputs=["sum_hr"]))

    clip_min = np.array(0.0, dtype=np.float32)
    clip_max = np.array(1.0, dtype=np.float32)
    initializers.append(numpy_helper.from_array(clip_min, name="clip_min"))
    initializers.append(numpy_helper.from_array(clip_max, name="clip_max"))
    nodes.append(helper.make_node("Clip", inputs=["sum_hr", "clip_min", "clip_max"], outputs=["output"]))

    inp_info = helper.make_tensor_value_info("input", TensorProto.FLOAT, [1, 3, "H", "W"])
    out_info = helper.make_tensor_value_info("output", TensorProto.FLOAT, [1, 3, "H_out", "W_out"])

    graph = helper.make_graph(nodes, "RealESRGAN_x4v3_SRVGGNetCompact", [inp_info], [out_info], initializers)
    model = helper.make_model(graph, opset_imports=[helper.make_opsetid("", 13)])
    model.ir_version = 8
    onnx.save(model, onnx_path)
    return onnx_path


def _run_realesrgan_upscale(bgr, factor):
    """
    Runs tiled Real-ESRGAN ONNX super-resolution (factor 2 or 4) with seamless overlap padding.
    """
    import onnxruntime as ort

    onnx_path = _ensure_realesrgan_onnx()
    sess_opts = ort.SessionOptions()
    sess_opts.intra_op_num_threads = max(1, min(8, (os.cpu_count() or 4)))
    sess = ort.InferenceSession(onnx_path, sess_options=sess_opts, providers=["CPUExecutionProvider"])

    rgb = cv2.cvtColor(bgr, cv2.COLOR_BGR2RGB).astype(np.float32) / 255.0
    h, w = rgb.shape[:2]

    tile = 192
    pad = 12
    scale = 4
    out_rgb = np.zeros((h * scale, w * scale, 3), dtype=np.float32)

    tiles_y = math.ceil(h / tile)
    tiles_x = math.ceil(w / tile)

    for ty in range(tiles_y):
        for tx in range(tiles_x):
            x0 = tx * tile
            y0 = ty * tile
            x1 = min(x0 + tile, w)
            y1 = min(y0 + tile, h)

            px0 = max(x0 - pad, 0)
            py0 = max(y0 - pad, 0)
            px1 = min(x1 + pad, w)
            py1 = min(y1 + pad, h)

            patch = rgb[py0:py1, px0:px1, :]
            inp = np.transpose(patch, (2, 0, 1))[np.newaxis, :, :, :]
            sr_patch = sess.run(["output"], {"input": inp})[0][0]
            sr_patch = np.transpose(sr_patch, (1, 2, 0))

            # Crop away overlap padding
            cx0 = (x0 - px0) * scale
            cy0 = (y0 - py0) * scale
            cx1 = cx0 + (x1 - x0) * scale
            cy1 = cy0 + (y1 - y0) * scale

            out_rgb[y0 * scale : y1 * scale, x0 * scale : x1 * scale, :] = sr_patch[cy0:cy1, cx0:cx1, :]

    out_u8 = np.clip(out_rgb * 255.0 + 0.5, 0, 255).astype(np.uint8)
    out_bgr = cv2.cvtColor(out_u8, cv2.COLOR_RGB2BGR)

    if factor == 2:
        out_bgr = cv2.resize(out_bgr, (w * 2, h * 2), interpolation=cv2.INTER_AREA)

    return out_bgr


# ---------------------------------------------------------------------------
# Phase 2.2 — Engineering-Safe OpenCV Enhancement Pipeline
# ---------------------------------------------------------------------------

def _edge_gated_unsharp(bgr, sigma=0.85, amount=0.55, threshold=6):
    blurred = cv2.GaussianBlur(bgr, (0, 0), sigmaX=sigma, sigmaY=sigma)
    diff = bgr.astype(np.int16) - blurred.astype(np.int16)
    mask = (np.abs(diff) >= threshold).astype(np.float32)
    sharpened = bgr.astype(np.float32) + (diff.astype(np.float32) * float(amount) * mask)
    return np.clip(sharpened, 0, 255).astype(np.uint8)


def enhance_raster(input_path, output_path, options):
    if not os.path.exists(input_path):
        return {"success": False, "code": "FILE_NOT_FOUND", "error": f"Input file not found: {input_path}"}

    try:
        bgr, alpha, _, band_count, _ = _load_image_rgba_or_rgb(input_path)
    except Exception as e:
        return {"success": False, "code": "DECODE_FAILED", "error": str(e)}

    orig_h, orig_w = bgr.shape[:2]
    gray_before = cv2.cvtColor(bgr, cv2.COLOR_BGR2GRAY)
    blur_before = float(cv2.Laplacian(gray_before, cv2.CV_64F).var())

    preset = str(options.get("preset") or "balanced").lower()
    ops = []

    # Resolve preset defaults
    defaults = {
        "balanced": {
            "denoise": True,
            "denoise_mode": "bilateral",
            "denoise_strength": 20.0,
            "contrast": True,
            "clahe": True,
            "clahe_clip": 1.6,
            "sharpen": True,
            "sharpen_amount": 0.50,
            "sharpen_sigma": 0.90,
            "preserve_lines": True,
            "adaptive_threshold": False,
            "illumination_norm": False,
        },
        "text_clarity": {
            "denoise": True,
            "denoise_mode": "bilateral",
            "denoise_strength": 16.0,
            "contrast": True,
            "clahe": True,
            "clahe_clip": 2.0,
            "sharpen": True,
            "sharpen_amount": 0.78,
            "sharpen_sigma": 0.78,
            "preserve_lines": True,
            "adaptive_threshold": False,
            "illumination_norm": False,
        },
        "line_preservation": {
            "denoise": True,
            "denoise_mode": "bilateral",
            "denoise_strength": 12.0,
            "contrast": True,
            "clahe": False,
            "clahe_clip": 1.3,
            "sharpen": True,
            "sharpen_amount": 0.45,
            "sharpen_sigma": 0.72,
            "preserve_lines": True,
            "adaptive_threshold": False,
            "illumination_norm": False,
        },
        "noise_reduction": {
            "denoise": True,
            "denoise_mode": "nlmeans",
            "denoise_strength": 5.0,
            "contrast": True,
            "clahe": True,
            "clahe_clip": 1.5,
            "sharpen": True,
            "sharpen_amount": 0.40,
            "sharpen_sigma": 0.85,
            "preserve_lines": True,
            "adaptive_threshold": False,
            "illumination_norm": False,
        },
        "scanned_drawing": {
            "denoise": True,
            "denoise_mode": "bilateral",
            "denoise_strength": 18.0,
            "contrast": True,
            "clahe": True,
            "clahe_clip": 1.8,
            "sharpen": True,
            "sharpen_amount": 0.65,
            "sharpen_sigma": 0.80,
            "preserve_lines": True,
            "adaptive_threshold": True,
            "illumination_norm": True,
        },
        "none": {
            "denoise": False,
            "denoise_mode": "bilateral",
            "denoise_strength": 0.0,
            "contrast": False,
            "clahe": False,
            "clahe_clip": 1.0,
            "sharpen": False,
            "sharpen_amount": 0.0,
            "sharpen_sigma": 0.8,
            "preserve_lines": True,
            "adaptive_threshold": False,
            "illumination_norm": False,
        },
    }
    cfg = dict(defaults.get(preset, defaults["balanced"]))

    if options.get("denoise") is not None:
        cfg["denoise"] = bool(options["denoise"])
    if options.get("denoise_strength"):
        cfg["denoise_strength"] = float(options["denoise_strength"])
    if options.get("contrast") is not None:
        cfg["contrast"] = bool(options["contrast"])
    if options.get("clahe") is not None:
        cfg["clahe"] = bool(options["clahe"])
    if options.get("clahe_clip_limit"):
        cfg["clahe_clip"] = max(1.0, min(3.5, float(options["clahe_clip_limit"])))
    if options.get("sharpen_text") is not None:
        cfg["sharpen"] = bool(options["sharpen_text"])
    if options.get("sharpen_amount"):
        cfg["sharpen_amount"] = max(0.0, min(1.2, float(options["sharpen_amount"])))
    if options.get("adaptive_threshold") is not None:
        cfg["adaptive_threshold"] = bool(options["adaptive_threshold"])

    grayscale_opt = bool(options.get("grayscale", False))
    work = bgr.copy()

    # 1. Optional grayscale normalization
    if grayscale_opt:
        g = cv2.cvtColor(work, cv2.COLOR_BGR2GRAY)
        work = cv2.cvtColor(g, cv2.COLOR_GRAY2BGR)
        ops.append("grayscale")

    # 2. Edge-preserving denoising (preserves fine lines, small symbols, and control points)
    if cfg["denoise"]:
        if cfg["denoise_mode"] == "nlmeans":
            h_val = max(2.0, min(8.0, float(cfg["denoise_strength"])))
            work = cv2.fastNlMeansDenoisingColored(work, None, h_val, h_val, 7, 21)
            ops.append(f"nlmeans_denoise(h={h_val:.1f})")
        else:
            sigma_c = max(8.0, min(32.0, float(cfg["denoise_strength"])))
            work = cv2.bilateralFilter(work, d=5, sigmaColor=sigma_c, sigmaSpace=sigma_c)
            ops.append(f"bilateral_denoise(sigma={sigma_c:.0f})")

    # 3. Scanned drawing illumination normalization (removes uneven paper lighting without clipping ink)
    if cfg["illumination_norm"]:
        lab = cv2.cvtColor(work, cv2.COLOR_BGR2LAB)
        l_ch, a_ch, b_ch = cv2.split(lab)
        bg = cv2.GaussianBlur(l_ch, (0, 0), sigmaX=35, sigmaY=35)
        norm_l = cv2.divide(l_ch, np.maximum(bg, 16), scale=245)
        work = cv2.cvtColor(cv2.merge([norm_l, a_ch, b_ch]), cv2.COLOR_LAB2BGR)
        ops.append("illumination_normalization")

    # 4. Gentle luminance contrast stretch + CLAHE
    if cfg["contrast"] or cfg["clahe"]:
        lab = cv2.cvtColor(work, cv2.COLOR_BGR2LAB)
        l_ch, a_ch, b_ch = cv2.split(lab)

        if cfg["contrast"]:
            p1, p99 = np.percentile(l_ch, (1.0, 99.0))
            if p99 - p1 > 15 and (p1 > 8 or p99 < 247):
                stretched = np.clip((l_ch.astype(np.float32) - p1) / (p99 - p1) * 255.0, 0, 255).astype(np.uint8)
                # Blend 70% stretched + 30% original luminance so fine tonal cues remain
                l_ch = cv2.addWeighted(stretched, 0.70, l_ch, 0.30, 0)
                ops.append("contrast_stretch")

        if cfg["clahe"]:
            clip = max(1.1, min(3.0, float(cfg["clahe_clip"])))
            clahe = cv2.createCLAHE(clipLimit=clip, tileGridSize=(8, 8))
            l_clahe = clahe.apply(l_ch)
            # Blend 65% CLAHE + 35% base to avoid amplifying paper grain in empty areas
            l_ch = cv2.addWeighted(l_clahe, 0.65, l_ch, 0.35, 0)
            ops.append(f"clahe(clip={clip:.1f})")

        work = cv2.cvtColor(cv2.merge([l_ch, a_ch, b_ch]), cv2.COLOR_LAB2BGR)

    # 5. Soft-blended adaptive thresholding for scanned engineering drawings
    if cfg["adaptive_threshold"]:
        g = cv2.cvtColor(work, cv2.COLOR_BGR2GRAY)
        adapt = cv2.adaptiveThreshold(
            g, 255, cv2.ADAPTIVE_THRESH_GAUSSIAN_C, cv2.THRESH_BINARY, blockSize=35, C=10
        )
        # Soft blend (32% binary + 68% continuous) preserves faint elevation ticks and thin dashed lines
        blended_g = cv2.addWeighted(adapt, 0.32, g, 0.68, 0)
        if grayscale_opt:
            work = cv2.cvtColor(blended_g, cv2.COLOR_GRAY2BGR)
        else:
            lab = cv2.cvtColor(work, cv2.COLOR_BGR2LAB)
            _, a_ch, b_ch = cv2.split(lab)
            work = cv2.cvtColor(cv2.merge([blended_g, a_ch, b_ch]), cv2.COLOR_LAB2BGR)
        ops.append("soft_adaptive_threshold")

    # 6. Line & text edge sharpening (edge-gated unsharp mask avoids halos on 1px engineering lines)
    if cfg["sharpen"] and cfg["sharpen_amount"] > 0:
        work = _edge_gated_unsharp(
            work,
            sigma=cfg["sharpen_sigma"],
            amount=cfg["sharpen_amount"],
            threshold=6 if cfg["preserve_lines"] else 4,
        )
        ops.append(f"edge_sharpen(amount={cfg['sharpen_amount']:.2f})")

    # 7. Optional AI Upscaling (Real-ESRGAN 2x / 4x)
    upscale_factor = int(options.get("upscale_factor") or 1)
    force_upscale = bool(options.get("force_upscale", False))
    ai_upscaled = False
    ai_engine = ""
    skipped_high_res = False

    if upscale_factor in (2, 4):
        max_dim = max(orig_w, orig_h)
        megapixels = (orig_w * orig_h) / 1_000_000.0
        # Guard against unnecessary bloat on already high-resolution images
        if (max_dim >= 3600 or megapixels >= 10.0) and not force_upscale:
            skipped_high_res = True
            upscale_factor = 1
        elif (max_dim * upscale_factor) > 8192:
            # Cap 4x -> 2x if 4x would exceed 8192px texture limit
            if upscale_factor == 4 and (max_dim * 2) <= 8192:
                upscale_factor = 2
            else:
                skipped_high_res = True
                upscale_factor = 1

        if upscale_factor in (2, 4) and not skipped_high_res:
            try:
                work = _run_realesrgan_upscale(work, upscale_factor)
                ai_upscaled = True
                ai_engine = "Real-ESRGAN (realesr-general-x4v3 ONNX)"
                ops.append(f"realesrgan_{upscale_factor}x")
            except Exception as sr_err:
                return {
                    "success": False,
                    "code": "AI_UPSCALE_FAILED",
                    "error": f"Real-ESRGAN {upscale_factor}x upscaling failed: {str(sr_err)}",
                }
            if alpha is not None:
                alpha = cv2.resize(
                    alpha,
                    (work.shape[1], work.shape[0]),
                    interpolation=cv2.INTER_LANCZOS4,
                )

    out_h, out_w = work.shape[:2]
    gray_after = cv2.cvtColor(work, cv2.COLOR_BGR2GRAY)
    blur_after = float(cv2.Laplacian(gray_after, cv2.CV_64F).var())

    out_dir = os.path.dirname(os.path.abspath(output_path))
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)

    if alpha is not None:
        out_img = np.dstack([work, alpha])
    else:
        out_img = work

    ok = cv2.imwrite(output_path, out_img)
    if not ok:
        return {"success": False, "code": "WRITE_FAILED", "error": f"Failed writing enhanced image to {output_path}"}

    return {
        "success": True,
        "output_path": output_path,
        "preset": preset,
        "original_width": orig_w,
        "original_height": orig_h,
        "enhanced_width": out_w,
        "enhanced_height": out_h,
        "scale_ratio": round(float(out_w) / float(max(1, orig_w)), 4),
        "ai_upscaled": ai_upscaled,
        "ai_upscale_factor": upscale_factor if ai_upscaled else 1,
        "ai_upscale_engine": ai_engine,
        "ai_verified_engineering": False,
        "ai_disclaimer": (
            "AI-upscaled details are generated for visual legibility only and must not be treated as verified engineering measurements."
            if ai_upscaled
            else ""
        ),
        "skipped_high_res_upscale": skipped_high_res,
        "blur_score_before": round(blur_before, 2),
        "blur_score_after": round(blur_after, 2),
        "operations_applied": ops,
    }


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "code": "INVALID_ARGS", "error": "Usage: raster_tool.py <inspect|enhance> ..."}))
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "inspect":
        res = inspect_raster(sys.argv[2])
        print(json.dumps(res))
    elif cmd == "to_png":
        if len(sys.argv) < 4:
            print(json.dumps({"success": False, "code": "INVALID_ARGS", "error": "Usage: raster_tool.py to_png <in> <out>"}))
            sys.exit(1)
        try:
            bgr, alpha, _, _, _ = _load_image_rgba_or_rgb(sys.argv[2])
            os.makedirs(os.path.dirname(os.path.abspath(sys.argv[3])), exist_ok=True)
            out_img = np.dstack([bgr, alpha]) if alpha is not None else bgr
            cv2.imwrite(sys.argv[3], out_img)
            print(json.dumps({"success": True, "output_path": sys.argv[3], "width": int(bgr.shape[1]), "height": int(bgr.shape[0])}))
        except Exception as e:
            print(json.dumps({"success": False, "code": "CONVERT_FAILED", "error": str(e)}))
            sys.exit(1)
    elif cmd == "enhance":
        if len(sys.argv) < 4:
            print(json.dumps({"success": False, "code": "INVALID_ARGS", "error": "Usage: raster_tool.py enhance <in> <out> [options_json]"}))
            sys.exit(1)
        in_path = sys.argv[2]
        out_path = sys.argv[3]
        opts = {}
        if len(sys.argv) >= 5 and sys.argv[4].strip():
            try:
                opts = json.loads(sys.argv[4])
            except Exception as e:
                print(json.dumps({"success": False, "code": "INVALID_JSON", "error": str(e)}))
                sys.exit(1)
        res = enhance_raster(in_path, out_path, opts)
        print(json.dumps(res))
    else:
        print(json.dumps({"success": False, "code": "UNKNOWN_COMMAND", "error": f"Unknown command: {cmd}"}))
        sys.exit(1)


if __name__ == "__main__":
    main()
