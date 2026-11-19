import sys
import os
import json
import math
import warnings

warnings.filterwarnings("ignore")

try:
    import shapely
    from shapely.geometry import shape, mapping, Point, LineString, Polygon, MultiPolygon, GeometryCollection
    from shapely.ops import unary_union, polygonize
    from shapely.validation import explain_validity, make_valid
    HAS_SHAPELY = True
except Exception:
    HAS_SHAPELY = False


def extract_Matching_Geoms(geom):
    """Extracts valid non-empty geometries from a Shapely geometry or GeometryCollection."""
    if geom is None or geom.is_empty:
        return []
    if not geom.is_valid:
        geom = make_valid(geom)
    if geom.geom_type == "GeometryCollection":
        out = []
        for sub in geom.geoms:
            if not sub.is_empty and sub.geom_type in ("Polygon", "MultiPolygon", "LineString", "MultiLineString", "Point", "MultiPoint"):
                out.append(sub)
        return out
    return [geom]


def cmd_overlay(payload):
    if not HAS_SHAPELY:
        return {"success": False, "error": "Shapely not available"}
    op = payload.get("operation", "intersection").lower()
    feats_a = payload.get("features_a", [])
    feats_b = payload.get("features_b", [])
    preserve = payload.get("preserve_attributes", "all")

    geoms_a = []
    for f in feats_a:
        try:
            g = shape(f["geometry"])
            if not g.is_valid:
                g = make_valid(g)
            if not g.is_empty:
                geoms_a.append((g, f.get("properties") or {}))
        except Exception:
            pass

    geoms_b = []
    for f in feats_b:
        try:
            g = shape(f["geometry"])
            if not g.is_valid:
                g = make_valid(g)
            if not g.is_empty:
                geoms_b.append((g, f.get("properties") or {}))
        except Exception:
            pass

    results = []

    if op == "intersection":
        for ga, pa in geoms_a:
            for gb, pb in geoms_b:
                if ga.intersects(gb):
                    inter = ga.intersection(gb)
                    for part in extract_Matching_Geoms(inter):
                        props = {}
                        if preserve in ("all", "layer_a"):
                            props.update(pa)
                        if preserve == "all":
                            for k, v in pb.items():
                                out_k = k if k not in props else f"b_{k}"
                                props[out_k] = v
                        results.append({"geometry": mapping(part), "properties": props})

    elif op == "clip":
        if geoms_b:
            clip_mask = unary_union([gb for gb, _ in geoms_b])
            for ga, pa in geoms_a:
                if ga.intersects(clip_mask):
                    clipped = ga.intersection(clip_mask)
                    for part in extract_Matching_Geoms(clipped):
                        results.append({"geometry": mapping(part), "properties": dict(pa) if preserve != "none" else {}})

    elif op == "difference":
        if geoms_b:
            sub_mask = unary_union([gb for gb, _ in geoms_b])
            for ga, pa in geoms_a:
                diff = ga.difference(sub_mask)
                for part in extract_Matching_Geoms(diff):
                    results.append({"geometry": mapping(part), "properties": dict(pa) if preserve != "none" else {}})
        else:
            for ga, pa in geoms_a:
                results.append({"geometry": mapping(ga), "properties": dict(pa)})

    elif op == "symmetrical_difference":
        union_a = unary_union([ga for ga, _ in geoms_a]) if geoms_a else Polygon()
        union_b = unary_union([gb for gb, _ in geoms_b]) if geoms_b else Polygon()
        sym = union_a.symmetric_difference(union_b)
        for part in extract_Matching_Geoms(sym):
            results.append({"geometry": mapping(part), "properties": {"operation": "symmetrical_difference"}})

    elif op == "union":
        # Pairwise intersection parts + A-only parts + B-only parts
        union_b = unary_union([gb for gb, _ in geoms_b]) if geoms_b else None
        union_a = unary_union([ga for ga, _ in geoms_a]) if geoms_a else None
        for ga, pa in geoms_a:
            for gb, pb in geoms_b:
                if ga.intersects(gb):
                    inter = ga.intersection(gb)
                    for part in extract_Matching_Geoms(inter):
                        props = dict(pa)
                        for k, v in pb.items():
                            props[k if k not in props else f"b_{k}"] = v
                        results.append({"geometry": mapping(part), "properties": props})
            if union_b is not None:
                rem_a = ga.difference(union_b)
                for part in extract_Matching_Geoms(rem_a):
                    results.append({"geometry": mapping(part), "properties": dict(pa)})
            else:
                results.append({"geometry": mapping(ga), "properties": dict(pa)})
        for gb, pb in geoms_b:
            if union_a is not None:
                rem_b = gb.difference(union_a)
                for part in extract_Matching_Geoms(rem_b):
                    results.append({"geometry": mapping(part), "properties": dict(pb)})
            else:
                results.append({"geometry": mapping(gb), "properties": dict(pb)})

    return {"success": True, "features": results}


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "error": "Usage: processing_tool.py <cmd> <json_payload>"}))
        sys.exit(1)
    cmd = sys.argv[1]
    payload = json.loads(sys.argv[2])
    if cmd == "overlay":
        print(json.dumps(cmd_overlay(payload)))
    else:
        print(json.dumps({"success": False, "error": f"Unknown command: {cmd}"}))


if __name__ == "__main__":
    main()
