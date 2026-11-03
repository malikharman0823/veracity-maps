import sys
import os
import re
import math
import json
import numpy as np

os.environ.setdefault("OPENCV_LOG_LEVEL", "OFF")
import cv2


def classify_engineering_label(text):
    t = (text or "").strip()
    if not t:
        return "general"
    upper = t.upper()
    if re.search(r"\b(EL\.?|ELEV\.?|RL|IL|GL|FFL|SSL|TOC|INV\.?|BM|DATUM)\b", upper) or re.search(r"[+-]\d+\.\d{2,3}\s*M?\b", upper):
        return "elevation_level"
    if re.search(r"\b(PIPE|CULVERT|PVC|HDPE|RCP|DN\s*\d+|DIA\.?|Ø\s*\d+|\d+\s*MM\s*Ø)\b", upper):
        return "pipe_culvert"
    if re.search(r"\b(ROAD|RD\.?|STREET|ST\.?|AVE\.?|AVENUE|HIGHWAY|HWY|LANE|LN\.?|BLVD|ROW|CARRIAGEWAY)\b", upper):
        return "road_label"
    if re.search(r"^\d+(\.\d+)?\s*(MM|CM|M|KM|FT|IN|'|\"|°)?$", upper) or re.search(r"\d+\s*[xX×]\s*\d+", upper):
        return "dimension"
    return "general"


class PaddleOcrAdapter:
    """
    Executes PaddleOCR PP-OCRv4/v5 models.
    Prefers native paddleocr when paddlepaddle C++ runtime is available, and
    seamlessly uses RapidOCR (official PaddleOCR PP-OCRv4/v5 ONNX weights on
    onnxruntime) when running on Python 3.14+ where paddlepaddle wheels are absent.
    """

    def __init__(self):
        self.engine_name = ""
        self._mode = ""
        self._engine = None

        try:
            import paddle  # noqa: F401
            from paddleocr import PaddleOCR
            self._engine = PaddleOCR(use_angle_cls=True, lang="en", show_log=False)
            self._mode = "paddleocr"
            self.engine_name = "paddleocr-ppocr"
            return
        except Exception:
            pass

        import logging
        from rapidocr import RapidOCR
        for lname in ("RapidOCR", "rapidocr"):
            lg = logging.getLogger(lname)
            lg.setLevel(logging.ERROR)
            for h in list(lg.handlers):
                if isinstance(h, logging.StreamHandler):
                    h.stream = sys.stderr
        self._engine = RapidOCR(params={"Global.log_level": "error"})
        self._mode = "rapidocr"
        self.engine_name = "paddleocr-ppocr-onnx"

    def detect_and_recognize(self, bgr_img):
        items = []
        if self._mode == "paddleocr":
            res = self._engine.ocr(bgr_img, cls=True)
            if not res:
                return items
            lines = res[0] if isinstance(res, list) and len(res) > 0 and isinstance(res[0], list) else res
            for entry in (lines or []):
                if not entry or len(entry) < 2:
                    continue
                quad = entry[0]
                txt, score = entry[1][0], float(entry[1][1])
                items.append((quad, str(txt), score))
            return items

        out = self._engine(bgr_img)
        boxes = getattr(out, "boxes", None)
        txts = getattr(out, "txts", None)
        scores = getattr(out, "scores", None)
        if boxes is None or txts is None or scores is None:
            return items
        for quad, txt, score in zip(boxes, txts, scores):
            q_list = quad.tolist() if hasattr(quad, "tolist") else list(quad)
            items.append((q_list, str(txt), float(score)))
        return items


def _box_iou(a, b):
    ix0 = max(a[0], b[0])
    iy0 = max(a[1], b[1])
    ix1 = min(a[2], b[2])
    iy1 = min(a[3], b[3])
    iw = max(0.0, ix1 - ix0)
    ih = max(0.0, iy1 - iy0)
    inter = iw * ih
    if inter <= 0.0:
        return 0.0
    area_a = max(1.0, (a[2] - a[0]) * (a[3] - a[1]))
    area_b = max(1.0, (b[2] - b[0]) * (b[3] - b[1]))
    union = area_a + area_b - inter
    return max(inter / max(1.0, union), inter / min(area_a, area_b) * 0.75)


def run_ocr(image_path, orig_width=0, orig_height=0, min_confidence=0.45):
    if not os.path.exists(image_path):
        return {"success": False, "error": f"Image not found: {image_path}", "code": "FILE_NOT_FOUND"}

    data = np.fromfile(image_path, dtype=np.uint8)
    raw = cv2.imdecode(data, cv2.IMREAD_UNCHANGED)
    if raw is None:
        return {"success": False, "error": "Failed to decode image for OCR", "code": "DECODE_FAILED"}

    if raw.ndim == 2:
        bgr = cv2.cvtColor(raw, cv2.COLOR_GRAY2BGR)
    elif raw.shape[2] == 4:
        # Composite transparent pixels onto clean white background for crisp text contrast
        alpha = raw[:, :, 3:4].astype(np.float32) / 255.0
        rgb = raw[:, :, :3].astype(np.float32)
        white = np.full_like(rgb, 255.0)
        bgr = np.clip(rgb * alpha + white * (1.0 - alpha), 0, 255).astype(np.uint8)
    else:
        bgr = raw[:, :, :3]

    img_h, img_w = bgr.shape[:2]
    target_w = int(orig_width) if int(orig_width) > 0 else img_w
    target_h = int(orig_height) if int(orig_height) > 0 else img_h
    sx = float(target_w) / float(max(1, img_w))
    sy = float(target_h) / float(max(1, img_h))

    try:
        adapter = PaddleOcrAdapter()
    except Exception as e:
        return {"success": False, "error": f"Failed to initialize PaddleOCR engine: {str(e)}", "code": "OCR_INIT_FAILED"}

    raw_detections = []

    # 1. Full-image pass
    for quad, txt, score in adapter.detect_and_recognize(bgr):
        raw_detections.append((quad, txt, score, 0, 0))

    # 2. Tiled pass for large engineering sheets so small labels remain legible
    if max(img_w, img_h) > 2000:
        tile_size = 1600
        overlap = 240
        step = tile_size - overlap
        xs = list(range(0, img_w, step))[:3]
        ys = list(range(0, img_h, step))[:3]
        for ty in ys:
            for tx in xs:
                x1 = min(img_w, tx + tile_size)
                y1 = min(img_h, ty + tile_size)
                x0 = max(0, x1 - tile_size)
                y0 = max(0, y1 - tile_size)
                crop = bgr[y0:y1, x0:x1]
                if crop.shape[0] < 64 or crop.shape[1] < 64:
                    continue
                for quad, txt, score in adapter.detect_and_recognize(crop):
                    raw_detections.append((quad, txt, score, x0, y0))

    candidates = []
    for quad, txt, score, ox, oy in raw_detections:
        clean_txt = (txt or "").strip()
        if not clean_txt or score < min_confidence:
            continue
        if len(quad) < 4:
            continue

        pts = [[(float(p[0]) + ox) * sx, (float(p[1]) + oy) * sy] for p in quad[:4]]
        xs_p = [p[0] for p in pts]
        ys_p = [p[1] for p in pts]
        bx0 = round(max(0.0, min(xs_p)), 2)
        by0 = round(max(0.0, min(ys_p)), 2)
        bx1 = round(min(float(target_w), max(xs_p)), 2)
        by1 = round(min(float(target_h), max(ys_p)), 2)
        if (bx1 - bx0) < 2.0 or (by1 - by0) < 2.0:
            continue

        dx = pts[1][0] - pts[0][0]
        dy = pts[1][1] - pts[0][1]
        angle = round(math.degrees(math.atan2(dy, dx)), 2)
        if abs(angle) < 2.5:
            angle = 0.0

        box_h = max(8.0, by1 - by0)
        candidates.append({
            "text": clean_txt,
            "original_text": clean_txt,
            "category": classify_engineering_label(clean_txt),
            "source": "paddleocr",
            "confidence": round(float(score), 4),
            "image_x": round((bx0 + bx1) * 0.5, 2),
            "image_y": round((by0 + by1) * 0.5, 2),
            "bbox_px": [bx0, by0, bx1, by1],
            "rotation": angle,
            "font_size": round(min(24.0, max(10.0, box_h * 0.75)), 1),
            "verified": False,
            "visible": True,
        })

    # Deduplicate overlapping detections (prefer higher confidence)
    candidates.sort(key=lambda c: c["confidence"], reverse=True)
    deduped = []
    for cand in candidates:
        duplicate = False
        for kept in deduped:
            if _box_iou(cand["bbox_px"], kept["bbox_px"]) >= 0.45:
                duplicate = True
                break
        if not duplicate:
            deduped.append(cand)

    # Sort reading order (top-to-bottom, left-to-right)
    deduped.sort(key=lambda c: (round(c["image_y"] / 20.0), c["image_x"]))

    return {
        "success": True,
        "engine": adapter.engine_name,
        "image_width": img_w,
        "image_height": img_h,
        "coordinate_width": target_w,
        "coordinate_height": target_h,
        "annotation_count": len(deduped),
        "annotations": deduped,
    }


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"success": False, "error": "Usage: ocr_tool.py <image_path> [orig_w] [orig_h] [min_conf]"}))
        sys.exit(1)

    cmd_or_path = sys.argv[1]
    if cmd_or_path == "extract":
        if len(sys.argv) < 3:
            print(json.dumps({"success": False, "error": "Missing image_path"}))
            sys.exit(1)
        img_path = sys.argv[2]
        ow = int(sys.argv[3]) if len(sys.argv) > 3 and sys.argv[3].isdigit() else 0
        oh = int(sys.argv[4]) if len(sys.argv) > 4 and sys.argv[4].isdigit() else 0
        mc = float(sys.argv[5]) if len(sys.argv) > 5 else 0.45
    else:
        img_path = cmd_or_path
        ow = int(sys.argv[2]) if len(sys.argv) > 2 and sys.argv[2].isdigit() else 0
        oh = int(sys.argv[3]) if len(sys.argv) > 3 and sys.argv[3].isdigit() else 0
        mc = float(sys.argv[4]) if len(sys.argv) > 4 else 0.45

    res = run_ocr(img_path, ow, oh, mc)
    print(json.dumps(res))


if __name__ == "__main__":
    main()
