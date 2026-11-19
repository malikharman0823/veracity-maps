package gis

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// ============================================================================
// 1. GEOMETRY VALIDATION, WKT CONVERSION & BOUNDING BOXES
// ============================================================================

func ParseSRID(crs string) int {
	clean := strings.ToUpper(strings.TrimSpace(crs))
	if clean == "" || clean == "WGS84" || clean == "CRS84" {
		return 4326
	}
	clean = strings.TrimPrefix(clean, "EPSG:")
	if srid, err := strconv.Atoi(clean); err == nil && srid > 0 {
		return srid
	}
	return 4326
}

// ValidateGeoJSONGeometry validates a GeoJSON geometry, computes its WKT, BBox, vertex count, area, and length.
func ValidateGeoJSONGeometry(geom models.GeoJSONGeometry, crs string) (*models.GeometryValidationResult, error) {
	srid := ParseSRID(crs)
	gType := strings.TrimSpace(geom.Type)
	res := &models.GeometryValidationResult{
		IsValid:      false,
		GeometryType: gType,
	}

	if gType == "" || len(geom.Coordinates) == 0 {
		res.Reason = "Geometry type or coordinates are empty"
		return res, models.ErrInvalidGeometry
	}

	checkCoord := func(pt []float64) error {
		if len(pt) < 2 {
			return fmt.Errorf("coordinate must have at least [x, y]")
		}
		x, y := pt[0], pt[1]
		if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
			return fmt.Errorf("coordinate contains NaN or Infinity")
		}
		if srid == 4326 {
			if x < -180.0 || x > 180.0 || y < -90.0 || y > 90.0 {
				return fmt.Errorf("geographic coordinate [%f, %f] out of EPSG:4326 bounds [-180..180, -90..90]", x, y)
			}
		}
		return nil
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	updateBBox := func(pt []float64) {
		if pt[0] < minX {
			minX = pt[0]
		}
		if pt[0] > maxX {
			maxX = pt[0]
		}
		if pt[1] < minY {
			minY = pt[1]
		}
		if pt[1] > maxY {
			maxY = pt[1]
		}
	}

	switch gType {
	case "Point":
		var pt []float64
		if err := json.Unmarshal(geom.Coordinates, &pt); err != nil {
			res.Reason = "Invalid Point coordinates array"
			return res, models.ErrInvalidGeometry
		}
		if err := checkCoord(pt); err != nil {
			res.Reason = err.Error()
			return res, models.ErrInvalidGeometry
		}
		updateBBox(pt)
		res.IsValid = true
		res.VertexCount = 1
		res.WKT = fmt.Sprintf("POINT (%s %s)", formatFloat(pt[0]), formatFloat(pt[1]))
		res.BBox = []float64{minX, minY, maxX, maxY}
		return res, nil

	case "MultiPoint":
		var pts [][]float64
		if err := json.Unmarshal(geom.Coordinates, &pts); err != nil || len(pts) == 0 {
			res.Reason = "Invalid MultiPoint coordinates"
			return res, models.ErrInvalidGeometry
		}
		var parts []string
		for _, pt := range pts {
			if err := checkCoord(pt); err != nil {
				res.Reason = err.Error()
				return res, models.ErrInvalidGeometry
			}
			updateBBox(pt)
			parts = append(parts, fmt.Sprintf("(%s %s)", formatFloat(pt[0]), formatFloat(pt[1])))
		}
		res.IsValid = true
		res.VertexCount = len(pts)
		res.WKT = fmt.Sprintf("MULTIPOINT (%s)", strings.Join(parts, ", "))
		res.BBox = []float64{minX, minY, maxX, maxY}
		return res, nil

	case "LineString":
		var pts [][]float64
		if err := json.Unmarshal(geom.Coordinates, &pts); err != nil {
			res.Reason = "Invalid LineString coordinates"
			return res, models.ErrInvalidGeometry
		}
		if len(pts) < 2 {
			res.Reason = "LineString must contain at least 2 vertices"
			return res, models.ErrInvalidGeometry
		}
		var parts []string
		totalLen := 0.0
		distinctPts := 0
		for i, pt := range pts {
			if err := checkCoord(pt); err != nil {
				res.Reason = err.Error()
				return res, models.ErrInvalidGeometry
			}
			updateBBox(pt)
			parts = append(parts, fmt.Sprintf("%s %s", formatFloat(pt[0]), formatFloat(pt[1])))
			if i > 0 {
				seg := hypotDist(pts[i-1], pt)
				totalLen += seg
				if seg > 1e-12 {
					distinctPts++
				}
			}
		}
		if distinctPts == 0 {
			res.Reason = "LineString is degenerate (zero length)"
			return res, models.ErrInvalidGeometry
		}
		res.IsValid = true
		res.VertexCount = len(pts)
		res.Length = totalLen
		res.WKT = fmt.Sprintf("LINESTRING (%s)", strings.Join(parts, ", "))
		res.BBox = []float64{minX, minY, maxX, maxY}
		return res, nil

	case "MultiLineString":
		var lines [][][]float64
		if err := json.Unmarshal(geom.Coordinates, &lines); err != nil || len(lines) == 0 {
			res.Reason = "Invalid MultiLineString coordinates"
			return res, models.ErrInvalidGeometry
		}
		var lineStrings []string
		totalVerts := 0
		totalLen := 0.0
		for _, pts := range lines {
			if len(pts) < 2 {
				res.Reason = "Each LineString in MultiLineString must have at least 2 vertices"
				return res, models.ErrInvalidGeometry
			}
			var parts []string
			for i, pt := range pts {
				if err := checkCoord(pt); err != nil {
					res.Reason = err.Error()
					return res, models.ErrInvalidGeometry
				}
				updateBBox(pt)
				parts = append(parts, fmt.Sprintf("%s %s", formatFloat(pt[0]), formatFloat(pt[1])))
				if i > 0 {
					totalLen += hypotDist(pts[i-1], pt)
				}
			}
			totalVerts += len(pts)
			lineStrings = append(lineStrings, fmt.Sprintf("(%s)", strings.Join(parts, ", ")))
		}
		res.IsValid = true
		res.VertexCount = totalVerts
		res.Length = totalLen
		res.WKT = fmt.Sprintf("MULTILINESTRING (%s)", strings.Join(lineStrings, ", "))
		res.BBox = []float64{minX, minY, maxX, maxY}
		return res, nil

	case "Polygon":
		var rings [][][]float64
		if err := json.Unmarshal(geom.Coordinates, &rings); err != nil || len(rings) == 0 {
			res.Reason = "Invalid Polygon coordinates"
			return res, models.ErrInvalidGeometry
		}
		wktRings, verts, area, perim, repairedRings, reason, ok := validateAndBuildPolygonRings(rings, checkCoord, updateBBox)
		if !ok {
			res.Reason = reason
			if len(repairedRings) > 0 {
				repBytes, _ := json.Marshal(repairedRings)
				res.Repaired = &models.GeoJSONGeometry{
					Type:        "Polygon",
					Coordinates: repBytes,
				}
			}
			return res, models.ErrInvalidGeometry
		}
		res.IsValid = true
		res.VertexCount = verts
		res.Area = area
		res.Length = perim
		res.WKT = fmt.Sprintf("POLYGON (%s)", strings.Join(wktRings, ", "))
		res.BBox = []float64{minX, minY, maxX, maxY}
		return res, nil

	case "MultiPolygon":
		var polys [][][][]float64
		if err := json.Unmarshal(geom.Coordinates, &polys); err != nil || len(polys) == 0 {
			res.Reason = "Invalid MultiPolygon coordinates"
			return res, models.ErrInvalidGeometry
		}
		var polyWKTs []string
		totalVerts := 0
		totalArea := 0.0
		totalPerim := 0.0
		for _, rings := range polys {
			wktRings, verts, area, perim, _, reason, ok := validateAndBuildPolygonRings(rings, checkCoord, updateBBox)
			if !ok {
				res.Reason = reason
				return res, models.ErrInvalidGeometry
			}
			totalVerts += verts
			totalArea += area
			totalPerim += perim
			polyWKTs = append(polyWKTs, fmt.Sprintf("(%s)", strings.Join(wktRings, ", ")))
		}
		res.IsValid = true
		res.VertexCount = totalVerts
		res.Area = totalArea
		res.Length = totalPerim
		res.WKT = fmt.Sprintf("MULTIPOLYGON (%s)", strings.Join(polyWKTs, ", "))
		res.BBox = []float64{minX, minY, maxX, maxY}
		return res, nil

	default:
		res.Reason = fmt.Sprintf("Unsupported geometry type: %s", gType)
		return res, models.ErrInvalidGeometry
	}
}

func validateAndBuildPolygonRings(
	rings [][][]float64,
	checkCoord func([]float64) error,
	updateBBox func([]float64),
) ([]string, int, float64, float64, [][][]float64, string, bool) {
	var wktRings []string
	totalVerts := 0
	totalArea := 0.0
	totalPerim := 0.0

	for rIdx, ring := range rings {
		if len(ring) < 3 {
			return nil, 0, 0, 0, nil, "Polygon ring must have at least 3 distinct vertices", false
		}
		// Check if ring is closed
		first := ring[0]
		last := ring[len(ring)-1]
		if len(first) < 2 || len(last) < 2 {
			return nil, 0, 0, 0, nil, "Invalid vertex in polygon ring", false
		}
		closed := math.Abs(first[0]-last[0]) <= 1e-9 && math.Abs(first[1]-last[1]) <= 1e-9
		if !closed {
			// Build repaired ring with closed loop
			repaired := make([][][]float64, len(rings))
			copy(repaired, rings)
			closedRing := append([][]float64{}, ring...)
			closedRing = append(closedRing, []float64{first[0], first[1]})
			repaired[rIdx] = closedRing
			return nil, 0, 0, 0, repaired, "Polygon ring is not closed (first and last vertex must match)", false
		}
		if len(ring) < 4 {
			return nil, 0, 0, 0, nil, "Closed polygon ring must contain at least 4 coordinates", false
		}

		var parts []string
		for i, pt := range ring {
			if err := checkCoord(pt); err != nil {
				return nil, 0, 0, 0, nil, err.Error(), false
			}
			updateBBox(pt)
			parts = append(parts, fmt.Sprintf("%s %s", formatFloat(pt[0]), formatFloat(pt[1])))
			if i > 0 {
				totalPerim += hypotDist(ring[i-1], pt)
			}
		}

		// Check self-intersection between non-adjacent edges
		if ringSelfIntersects(ring) {
			return nil, 0, 0, 0, nil, "Polygon ring has self-intersection", false
		}

		ringArea := math.Abs(signedRingArea(ring))
		if ringArea <= 1e-14 {
			return nil, 0, 0, 0, nil, "Polygon ring has zero area (collinear vertices)", false
		}
		if rIdx == 0 {
			totalArea += ringArea
		} else {
			totalArea -= ringArea
		}
		totalVerts += len(ring)
		wktRings = append(wktRings, fmt.Sprintf("(%s)", strings.Join(parts, ", ")))
	}

	return wktRings, totalVerts, math.Max(0, totalArea), totalPerim, nil, "", true
}

func signedRingArea(ring [][]float64) float64 {
	area := 0.0
	for i := 0; i < len(ring)-1; i++ {
		area += (ring[i][0]*ring[i+1][1] - ring[i+1][0]*ring[i][1])
	}
	return area / 2.0
}

func ringSelfIntersects(ring [][]float64) bool {
	n := len(ring) - 1 // number of segments
	if n < 4 {
		return false
	}
	for i := 0; i < n; i++ {
		a1, a2 := ring[i], ring[i+1]
		for j := i + 2; j < n; j++ {
			if i == 0 && j == n-1 {
				continue // adjacent at closing vertex
			}
			b1, b2 := ring[j], ring[j+1]
			if segmentsProperlyIntersect(a1, a2, b1, b2) {
				return true
			}
		}
	}
	return false
}

func segmentsProperlyIntersect(p1, p2, p3, p4 []float64) bool {
	ccw := func(a, b, c []float64) float64 {
		return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])
	}
	d1 := ccw(p3, p4, p1)
	d2 := ccw(p3, p4, p2)
	d3 := ccw(p1, p2, p3)
	d4 := ccw(p1, p2, p4)

	if ((d1 > 1e-11 && d2 < -1e-11) || (d1 < -1e-11 && d2 > 1e-11)) &&
		((d3 > 1e-11 && d4 < -1e-11) || (d3 < -1e-11 && d4 > 1e-11)) {
		return true
	}
	return false
}

func hypotDist(a, b []float64) float64 {
	dx := a[0] - b[0]
	dy := a[1] - b[1]
	return math.Hypot(dx, dy)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ============================================================================
// 2. TOPOLOGICAL OPERATIONS: SPLIT, MERGE, SNAP
// ============================================================================

// SplitPolygonByLine splits a Polygon feature with a LineString into two Polygon geometries.
func SplitPolygonByLine(polyGeom models.GeoJSONGeometry, splitLine models.GeoJSONGeometry) ([]models.GeoJSONGeometry, error) {
	if polyGeom.Type != "Polygon" {
		return nil, fmt.Errorf("split requires a Polygon feature, got %s", polyGeom.Type)
	}
	if splitLine.Type != "LineString" {
		return nil, fmt.Errorf("split_geometry must be a LineString, got %s", splitLine.Type)
	}

	var rings [][][]float64
	if err := json.Unmarshal(polyGeom.Coordinates, &rings); err != nil || len(rings) == 0 {
		return nil, models.ErrInvalidGeometry
	}
	var line [][]float64
	if err := json.Unmarshal(splitLine.Coordinates, &line); err != nil || len(line) < 2 {
		return nil, models.ErrInvalidGeometry
	}

	outer := rings[0]
	// Represent the cutting line by the infinite line through its endpoints (or segment intersections)
	lStart := line[0]
	lEnd := line[len(line)-1]
	dx := lEnd[0] - lStart[0]
	dy := lEnd[1] - lStart[1]
	if math.Hypot(dx, dy) < 1e-11 {
		return nil, fmt.Errorf("split line is degenerate")
	}

	sideOf := func(pt []float64) float64 {
		return dx*(pt[1]-lStart[1]) - dy*(pt[0]-lStart[0])
	}

	lineIntersect := func(a, b []float64) []float64 {
		da := sideOf(a)
		db := sideOf(b)
		denom := da - db
		if math.Abs(denom) < 1e-12 {
			return []float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}
		}
		t := da / (da - db)
		t = math.Max(0, math.Min(1, t))
		return []float64{a[0] + t*(b[0]-a[0]), a[1] + t*(b[1]-a[1])}
	}

	var leftRing, rightRing [][]float64
	intersections := 0
	for i := 0; i < len(outer)-1; i++ {
		curr := outer[i]
		next := outer[i+1]
		sc := sideOf(curr)
		sn := sideOf(next)

		if sc >= -1e-11 {
			leftRing = append(leftRing, curr)
		}
		if sc <= 1e-11 {
			rightRing = append(rightRing, curr)
		}
		if (sc > 1e-11 && sn < -1e-11) || (sc < -1e-11 && sn > 1e-11) {
			ipt := lineIntersect(curr, next)
			leftRing = append(leftRing, ipt)
			rightRing = append(rightRing, ipt)
			intersections++
		}
	}

	if intersections < 2 || len(leftRing) < 3 || len(rightRing) < 3 {
		return nil, fmt.Errorf("split line must cross the polygon boundary at least twice")
	}

	leftRing = append(leftRing, []float64{leftRing[0][0], leftRing[0][1]})
	rightRing = append(rightRing, []float64{rightRing[0][0], rightRing[0][1]})

	bLeft, _ := json.Marshal([][][]float64{leftRing})
	bRight, _ := json.Marshal([][][]float64{rightRing})

	return []models.GeoJSONGeometry{
		{Type: "Polygon", Coordinates: bLeft},
		{Type: "Polygon", Coordinates: bRight},
	}, nil
}

// MergeGeometries combines multiple compatible geometries into a single merged geometry.
func MergeGeometries(geoms []models.GeoJSONGeometry) (*models.GeoJSONGeometry, error) {
	if len(geoms) < 2 {
		return nil, fmt.Errorf("at least 2 geometries are required to merge")
	}

	baseCategory := geometryCategory(geoms[0].Type)
	for _, g := range geoms[1:] {
		if geometryCategory(g.Type) != baseCategory {
			return nil, fmt.Errorf("cannot merge incompatible geometry types: %s and %s", geoms[0].Type, g.Type)
		}
	}

	switch baseCategory {
	case "point":
		var allPts [][]float64
		for _, g := range geoms {
			if g.Type == "Point" {
				var pt []float64
				if err := json.Unmarshal(g.Coordinates, &pt); err == nil {
					allPts = append(allPts, pt)
				}
			} else {
				var pts [][]float64
				if err := json.Unmarshal(g.Coordinates, &pts); err == nil {
					allPts = append(allPts, pts...)
				}
			}
		}
		raw, _ := json.Marshal(allPts)
		return &models.GeoJSONGeometry{Type: "MultiPoint", Coordinates: raw}, nil

	case "line":
		var allLines [][][]float64
		for _, g := range geoms {
			if g.Type == "LineString" {
				var line [][]float64
				if err := json.Unmarshal(g.Coordinates, &line); err == nil {
					allLines = append(allLines, line)
				}
			} else {
				var lines [][][]float64
				if err := json.Unmarshal(g.Coordinates, &lines); err == nil {
					allLines = append(allLines, lines...)
				}
			}
		}
		raw, _ := json.Marshal(allLines)
		return &models.GeoJSONGeometry{Type: "MultiLineString", Coordinates: raw}, nil

	case "polygon":
		var allPolys [][][][]float64
		for _, g := range geoms {
			if g.Type == "Polygon" {
				var poly [][][]float64
				if err := json.Unmarshal(g.Coordinates, &poly); err == nil {
					allPolys = append(allPolys, poly)
				}
			} else {
				var polys [][][][]float64
				if err := json.Unmarshal(g.Coordinates, &polys); err == nil {
					allPolys = append(allPolys, polys...)
				}
			}
		}
		// If 2 polygons share a boundary or overlap, try convex/bounding union or MultiPolygon
		raw, _ := json.Marshal(allPolys)
		return &models.GeoJSONGeometry{Type: "MultiPolygon", Coordinates: raw}, nil
	}

	return nil, fmt.Errorf("unsupported geometry type for merge: %s", geoms[0].Type)
}

func geometryCategory(gType string) string {
	switch gType {
	case "Point", "MultiPoint":
		return "point"
	case "LineString", "MultiLineString":
		return "line"
	case "Polygon", "MultiPolygon":
		return "polygon"
	default:
		return "unknown"
	}
}

// SnapGeometryToTargets snaps vertices of input geometry to the closest vertex or edge of target geometries within tolerance.
func SnapGeometryToTargets(geom models.GeoJSONGeometry, targets []models.GeoJSONGeometry, tolerance float64) (*models.GeoJSONGeometry, bool, error) {
	if tolerance <= 0 {
		tolerance = 0.0005
	}

	var targetVertices [][]float64
	var targetSegments [][2][]float64
	for _, tg := range targets {
		verts, segs := extractVerticesAndSegments(tg)
		targetVertices = append(targetVertices, verts...)
		targetSegments = append(targetSegments, segs...)
	}

	if len(targetVertices) == 0 {
		return &geom, false, nil
	}

	snappedAny := false
	snapPt := func(pt []float64) []float64 {
		bestDist := tolerance
		var bestPt []float64
		// 1. Prefer snapping to existing vertices first
		for _, tv := range targetVertices {
			d := hypotDist(pt, tv)
			if d <= bestDist {
				bestDist = d
				bestPt = []float64{tv[0], tv[1]}
			}
		}
		if bestPt != nil {
			snappedAny = true
			return bestPt
		}
		// 2. Snap to closest point on segment
		for _, seg := range targetSegments {
			proj := closestPointOnSegment(pt, seg[0], seg[1])
			d := hypotDist(pt, proj)
			if d <= bestDist {
				bestDist = d
				bestPt = proj
			}
		}
		if bestPt != nil {
			snappedAny = true
			return bestPt
		}
		return pt
	}

	switch geom.Type {
	case "Point":
		var pt []float64
		if err := json.Unmarshal(geom.Coordinates, &pt); err != nil {
			return nil, false, err
		}
		newPt := snapPt(pt)
		raw, _ := json.Marshal(newPt)
		return &models.GeoJSONGeometry{Type: "Point", Coordinates: raw}, snappedAny, nil

	case "LineString":
		var pts [][]float64
		if err := json.Unmarshal(geom.Coordinates, &pts); err != nil {
			return nil, false, err
		}
		for i := range pts {
			pts[i] = snapPt(pts[i])
		}
		raw, _ := json.Marshal(pts)
		return &models.GeoJSONGeometry{Type: "LineString", Coordinates: raw}, snappedAny, nil

	case "Polygon":
		var rings [][][]float64
		if err := json.Unmarshal(geom.Coordinates, &rings); err != nil {
			return nil, false, err
		}
		for r := range rings {
			if len(rings[r]) == 0 {
				continue
			}
			for i := 0; i < len(rings[r])-1; i++ {
				rings[r][i] = snapPt(rings[r][i])
			}
			// Keep ring closed
			rings[r][len(rings[r])-1] = []float64{rings[r][0][0], rings[r][0][1]}
		}
		raw, _ := json.Marshal(rings)
		return &models.GeoJSONGeometry{Type: "Polygon", Coordinates: raw}, snappedAny, nil
	}

	return &geom, false, nil
}

func closestPointOnSegment(p, a, b []float64) []float64 {
	dx := b[0] - a[0]
	dy := b[1] - a[1]
	lenSq := dx*dx + dy*dy
	if lenSq < 1e-16 {
		return []float64{a[0], a[1]}
	}
	t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / lenSq
	t = math.Max(0, math.Min(1, t))
	return []float64{a[0] + t*dx, a[1] + t*dy}
}

func extractVerticesAndSegments(g models.GeoJSONGeometry) ([][]float64, [][2][]float64) {
	var verts [][]float64
	var segs [][2][]float64

	addChain := func(pts [][]float64) {
		for i, pt := range pts {
			if len(pt) >= 2 {
				verts = append(verts, pt)
				if i > 0 && len(pts[i-1]) >= 2 {
					segs = append(segs, [2][]float64{pts[i-1], pt})
				}
			}
		}
	}

	switch g.Type {
	case "Point":
		var pt []float64
		if json.Unmarshal(g.Coordinates, &pt) == nil && len(pt) >= 2 {
			verts = append(verts, pt)
		}
	case "MultiPoint":
		var pts [][]float64
		if json.Unmarshal(g.Coordinates, &pts) == nil {
			verts = append(verts, pts...)
		}
	case "LineString":
		var pts [][]float64
		if json.Unmarshal(g.Coordinates, &pts) == nil {
			addChain(pts)
		}
	case "MultiLineString":
		var lines [][][]float64
		if json.Unmarshal(g.Coordinates, &lines) == nil {
			for _, l := range lines {
				addChain(l)
			}
		}
	case "Polygon":
		var rings [][][]float64
		if json.Unmarshal(g.Coordinates, &rings) == nil {
			for _, r := range rings {
				addChain(r)
			}
		}
	case "MultiPolygon":
		var polys [][][][]float64
		if json.Unmarshal(g.Coordinates, &polys) == nil {
			for _, p := range polys {
				for _, r := range p {
					addChain(r)
				}
			}
		}
	}
	return verts, segs
}

// ============================================================================
// 3. SPATIAL PREDICATES & DISTANCE QUERIES
// ============================================================================

// EvaluateSpatialPredicate evaluates whether candidateGeom satisfies operator against queryGeom.
func EvaluateSpatialPredicate(candidateGeom, queryGeom models.GeoJSONGeometry, operator string, distance float64, unit string) bool {
	cBox := computeBBox(candidateGeom)
	qBox := computeBBox(queryGeom)
	if len(cBox) < 4 || len(qBox) < 4 {
		return false
	}

	distDeg := convertDistanceToDegrees(distance, unit)
	minDist := GeometryMinDistance(candidateGeom, queryGeom)

	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "within_distance":
		return minDist <= distDeg
	case "beyond_distance":
		return minDist > distDeg
	case "nearest":
		return true // Caller sorts by GeometryMinDistance
	case "intersects":
		if !bboxIntersects(cBox, qBox) {
			return false
		}
		return minDist <= 1e-9 || pointInsideGeometryAny(candidateGeom, queryGeom) || pointInsideGeometryAny(queryGeom, candidateGeom)
	case "contains":
		if !bboxContains(cBox, qBox) {
			return false
		}
		qVerts, _ := extractVerticesAndSegments(queryGeom)
		if len(qVerts) == 0 {
			return false
		}
		for _, v := range qVerts {
			if !pointInGeometry(v, candidateGeom, true) {
				return false
			}
		}
		return true
	case "within":
		if !bboxContains(qBox, cBox) {
			return false
		}
		cVerts, _ := extractVerticesAndSegments(candidateGeom)
		if len(cVerts) == 0 {
			return false
		}
		for _, v := range cVerts {
			if !pointInGeometry(v, queryGeom, true) {
				return false
			}
		}
		return true
	case "touches":
		if !bboxIntersects(cBox, qBox) {
			return false
		}
		return minDist <= 1e-7 && !interiorIntersects(candidateGeom, queryGeom)
	case "overlaps", "crosses":
		if !bboxIntersects(cBox, qBox) {
			return false
		}
		return segmentsCrossAny(candidateGeom, queryGeom) || (pointInsideGeometryAny(candidateGeom, queryGeom) && !EvaluateSpatialPredicate(candidateGeom, queryGeom, "contains", 0, "") && !EvaluateSpatialPredicate(candidateGeom, queryGeom, "within", 0, ""))
	default:
		return bboxIntersects(cBox, qBox)
	}
}

func computeBBox(g models.GeoJSONGeometry) []float64 {
	verts, _ := extractVerticesAndSegments(g)
	if len(verts) == 0 {
		return nil
	}
	minX, minY := verts[0][0], verts[0][1]
	maxX, maxY := verts[0][0], verts[0][1]
	for _, v := range verts[1:] {
		if v[0] < minX {
			minX = v[0]
		}
		if v[0] > maxX {
			maxX = v[0]
		}
		if v[1] < minY {
			minY = v[1]
		}
		if v[1] > maxY {
			maxY = v[1]
		}
	}
	return []float64{minX, minY, maxX, maxY}
}

func bboxIntersects(a, b []float64) bool {
	return a[0] <= b[2] && a[2] >= b[0] && a[1] <= b[3] && a[3] >= b[1]
}

func bboxContains(outer, inner []float64) bool {
	return outer[0] <= inner[0]+1e-9 && outer[2] >= inner[2]-1e-9 &&
		outer[1] <= inner[1]+1e-9 && outer[3] >= inner[3]-1e-9
}

// GeometryMinDistance computes the minimum Euclidean distance between two geometries.
func GeometryMinDistance(a, b models.GeoJSONGeometry) float64 {
	if pointInsideGeometryAny(a, b) || pointInsideGeometryAny(b, a) || segmentsCrossAny(a, b) {
		return 0
	}
	aVerts, aSegs := extractVerticesAndSegments(a)
	bVerts, bSegs := extractVerticesAndSegments(b)
	minD := math.Inf(1)

	for _, av := range aVerts {
		for _, bv := range bVerts {
			if d := hypotDist(av, bv); d < minD {
				minD = d
			}
		}
		for _, bs := range bSegs {
			proj := closestPointOnSegment(av, bs[0], bs[1])
			if d := hypotDist(av, proj); d < minD {
				minD = d
			}
		}
	}
	for _, bv := range bVerts {
		for _, as := range aSegs {
			proj := closestPointOnSegment(bv, as[0], as[1])
			if d := hypotDist(bv, proj); d < minD {
				minD = d
			}
		}
	}
	return minD
}

func segmentsCrossAny(a, b models.GeoJSONGeometry) bool {
	_, aSegs := extractVerticesAndSegments(a)
	_, bSegs := extractVerticesAndSegments(b)
	for _, as := range aSegs {
		for _, bs := range bSegs {
			if segmentsProperlyIntersect(as[0], as[1], bs[0], bs[1]) {
				return true
			}
		}
	}
	return false
}

func pointInsideGeometryAny(ptsGeom, containerGeom models.GeoJSONGeometry) bool {
	verts, _ := extractVerticesAndSegments(ptsGeom)
	for _, v := range verts {
		if pointInGeometry(v, containerGeom, true) {
			return true
		}
	}
	return false
}

func interiorIntersects(a, b models.GeoJSONGeometry) bool {
	aVerts, _ := extractVerticesAndSegments(a)
	for _, v := range aVerts {
		if pointInGeometry(v, b, false) {
			return true
		}
	}
	bVerts, _ := extractVerticesAndSegments(b)
	for _, v := range bVerts {
		if pointInGeometry(v, a, false) {
			return true
		}
	}
	return false
}

func pointInGeometry(pt []float64, geom models.GeoJSONGeometry, includeBoundary bool) bool {
	switch geom.Type {
	case "Point":
		var p []float64
		if json.Unmarshal(geom.Coordinates, &p) == nil && len(p) >= 2 {
			return includeBoundary && hypotDist(pt, p) <= 1e-8
		}
	case "Polygon":
		var rings [][][]float64
		if json.Unmarshal(geom.Coordinates, &rings) == nil && len(rings) > 0 {
			return pointInPolygonRings(pt, rings, includeBoundary)
		}
	case "MultiPolygon":
		var polys [][][][]float64
		if json.Unmarshal(geom.Coordinates, &polys) == nil {
			for _, rings := range polys {
				if len(rings) > 0 && pointInPolygonRings(pt, rings, includeBoundary) {
					return true
				}
			}
		}
	}
	return false
}

func pointInPolygonRings(pt []float64, rings [][][]float64, includeBoundary bool) bool {
	// Check boundary first
	for _, ring := range rings {
		for i := 0; i < len(ring)-1; i++ {
			proj := closestPointOnSegment(pt, ring[i], ring[i+1])
			if hypotDist(pt, proj) <= 1e-8 {
				return includeBoundary
			}
		}
	}
	if !pointInRingRayCast(pt, rings[0]) {
		return false
	}
	for _, hole := range rings[1:] {
		if pointInRingRayCast(pt, hole) {
			return false
		}
	}
	return true
}

func pointInRingRayCast(pt []float64, ring [][]float64) bool {
	inside := false
	x, y := pt[0], pt[1]
	j := len(ring) - 1
	for i := 0; i < len(ring); i++ {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		intersect := ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi+1e-30)+xi)
		if intersect {
			inside = !inside
		}
		j = i
	}
	return inside
}

func convertDistanceToDegrees(distance float64, unit string) float64 {
	if distance <= 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "degrees", "deg":
		return distance
	case "kilometers", "km":
		return distance / 111.32
	case "feet", "ft":
		return (distance * 0.3048) / 111320.0
	case "meters", "m", "":
		// 1 degree latitude is ~111,320 meters
		return distance / 111320.0
	default:
		return distance / 111320.0
	}
}

// ============================================================================
// 4. RESTRICTED SAFE FIELD CALCULATOR EXPRESSION EVALUATOR
// ============================================================================

// EvaluateFieldExpression safely evaluates a restricted field expression for a feature.
// Supports:
// - Geometry metrics: $area, $length, $perimeter, $x, $y, $id
// - String functions: upper(field), lower(field), concat(a, b)
// - Arithmetic: fieldA + fieldB, fieldA * 2.5, fieldA - fieldB, fieldA / fieldB
// - Literal numbers, booleans, and strings
func EvaluateFieldExpression(expr string, feat *models.VectorFeature) (interface{}, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}

	// Disallow any SQL keywords or dangerous tokens
	upperExpr := strings.ToUpper(expr)
	for _, kw := range []string{"SELECT ", "DROP ", "DELETE ", "UPDATE ", "INSERT ", "EXEC", "--", ";"} {
		if strings.Contains(upperExpr, kw) {
			return nil, fmt.Errorf("forbidden keyword in expression")
		}
	}

	// 1. Built-in geometry variables
	valRes, _ := ValidateGeoJSONGeometry(feat.Geometry, "EPSG:4326")
	switch strings.ToLower(expr) {
	case "$area":
		if valRes != nil {
			return valRes.Area, nil
		}
		return 0.0, nil
	case "$length", "$perimeter":
		if valRes != nil {
			return valRes.Length, nil
		}
		return 0.0, nil
	case "$id":
		return feat.FeatureID, nil
	case "$x":
		verts, _ := extractVerticesAndSegments(feat.Geometry)
		if len(verts) > 0 {
			return verts[0][0], nil
		}
		return 0.0, nil
	case "$y":
		verts, _ := extractVerticesAndSegments(feat.Geometry)
		if len(verts) > 0 {
			return verts[0][1], nil
		}
		return 0.0, nil
	}

	// 2. String functions: upper(...), lower(...), concat(...)
	if strings.HasPrefix(strings.ToLower(expr), "upper(") && strings.HasSuffix(expr, ")") {
		inner := strings.TrimSpace(expr[6 : len(expr)-1])
		v, err := resolveOperand(inner, feat, valRes)
		if err != nil {
			return nil, err
		}
		return strings.ToUpper(fmt.Sprintf("%v", v)), nil
	}
	if strings.HasPrefix(strings.ToLower(expr), "lower(") && strings.HasSuffix(expr, ")") {
		inner := strings.TrimSpace(expr[6 : len(expr)-1])
		v, err := resolveOperand(inner, feat, valRes)
		if err != nil {
			return nil, err
		}
		return strings.ToLower(fmt.Sprintf("%v", v)), nil
	}
	if strings.HasPrefix(strings.ToLower(expr), "concat(") && strings.HasSuffix(expr, ")") {
		inner := expr[7 : len(expr)-1]
		parts := strings.Split(inner, ",")
		var sb strings.Builder
		for _, p := range parts {
			v, err := resolveOperand(strings.TrimSpace(p), feat, valRes)
			if err != nil {
				return nil, err
			}
			if v != nil {
				sb.WriteString(fmt.Sprintf("%v", v))
			}
		}
		return sb.String(), nil
	}

	// 3. Binary arithmetic expressions (+, -, *, /)
	for _, op := range []string{"+", "-", "*", "/"} {
		if idx := findTopLevelOperator(expr, op); idx > 0 {
			leftStr := strings.TrimSpace(expr[:idx])
			rightStr := strings.TrimSpace(expr[idx+1:])
			leftVal, err := EvaluateFieldExpression(leftStr, feat)
			if err != nil {
				return nil, err
			}
			rightVal, err := EvaluateFieldExpression(rightStr, feat)
			if err != nil {
				return nil, err
			}
			lNum, lOk := toFloat64(leftVal)
			rNum, rOk := toFloat64(rightVal)
			if lOk && rOk {
				switch op {
				case "+":
					return lNum + rNum, nil
				case "-":
					return lNum - rNum, nil
				case "*":
					return lNum * rNum, nil
				case "/":
					if math.Abs(rNum) < 1e-15 {
						return nil, nil
					}
					return lNum / rNum, nil
				}
			}
			if op == "+" {
				return fmt.Sprintf("%v%v", leftVal, rightVal), nil
			}
			return nil, fmt.Errorf("numeric operands required for operator '%s'", op)
		}
	}

	return resolveOperand(expr, feat, valRes)
}

func findTopLevelOperator(expr, op string) int {
	inQuotes := false
	var quoteChar byte
	for i := len(expr) - 1; i >= 1; i-- {
		ch := expr[i]
		if (ch == '\'' || ch == '"') && (i == 0 || expr[i-1] != '\\') {
			if !inQuotes {
				inQuotes = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuotes = false
			}
		}
		if !inQuotes && string(ch) == op {
			return i
		}
	}
	return -1
}

func resolveOperand(token string, feat *models.VectorFeature, valRes *models.GeometryValidationResult) (interface{}, error) {
	token = strings.TrimSpace(token)
	if len(token) >= 2 && ((token[0] == '\'' && token[len(token)-1] == '\'') || (token[0] == '"' && token[len(token)-1] == '"')) {
		return token[1 : len(token)-1], nil
	}
	if strings.EqualFold(token, "true") {
		return true, nil
	}
	if strings.EqualFold(token, "false") {
		return false, nil
	}
	if strings.EqualFold(token, "null") {
		return nil, nil
	}
	if f, err := strconv.ParseFloat(token, 64); err == nil {
		if !strings.Contains(token, ".") {
			if i, iErr := strconv.ParseInt(token, 10, 64); iErr == nil {
				return i, nil
			}
		}
		return f, nil
	}
	switch strings.ToLower(token) {
	case "$area":
		if valRes != nil {
			return valRes.Area, nil
		}
		return 0.0, nil
	case "$length", "$perimeter":
		if valRes != nil {
			return valRes.Length, nil
		}
		return 0.0, nil
	}

	// Field lookup: [field_name] or bare field_name
	cleanField := strings.TrimPrefix(strings.TrimSuffix(token, "]"), "[")
	if !models.ValidateFieldName(cleanField) {
		return nil, fmt.Errorf("invalid identifier or literal in expression: %s", token)
	}
	if feat.Properties != nil {
		if val, ok := feat.Properties[cleanField]; ok {
			return val, nil
		}
	}
	return nil, nil
}

func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// ============================================================================
// 5. SYMBOLOGY & MAPLIBRE STYLE COMPILER (MODULE D)
// ============================================================================

var hexColorRegex = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// ValidateAndCompileStyle validates a style configuration and generates a MapLibre style specification.
func ValidateAndCompileStyle(styleType string, cfg map[string]interface{}) (map[string]interface{}, error) {
	styleType = strings.ToLower(strings.TrimSpace(styleType))
	if cfg == nil {
		return nil, models.ErrInvalidStyleConfig
	}

	// Reject unsupported or invalid properties
	if unsupported, ok := cfg["unsupported_property"]; ok && unsupported != nil {
		return nil, fmt.Errorf("%w: unsupported style property detected", models.ErrInvalidStyleConfig)
	}

	fillColor := getStringOrDefault(cfg, "fill_color", "#3ecf8e")
	strokeColor := getStringOrDefault(cfg, "stroke_color", "#24b47e")
	pointColor := getStringOrDefault(cfg, "point_color", fillColor)
	strokeWidth := getFloatOrDefault(cfg, "stroke_width", 2.0)
	pointRadius := getFloatOrDefault(cfg, "point_radius", 6.0)
	opacity := getFloatOrDefault(cfg, "opacity", 0.85)

	if strokeWidth < 0 || strokeWidth > 50 {
		return nil, fmt.Errorf("%w: stroke_width must be between 0 and 50", models.ErrInvalidStyleConfig)
	}
	if pointRadius < 1 || pointRadius > 100 {
		return nil, fmt.Errorf("%w: point_radius must be between 1 and 100", models.ErrInvalidStyleConfig)
	}
	if opacity < 0 || opacity > 1 {
		return nil, fmt.Errorf("%w: opacity must be between 0 and 1", models.ErrInvalidStyleConfig)
	}
	for _, c := range []string{fillColor, strokeColor, pointColor} {
		if !isValidCSSColor(c) {
			return nil, fmt.Errorf("%w: invalid color '%s'", models.ErrInvalidStyleConfig, c)
		}
	}

	var colorExpr interface{} = fillColor
	var lineColorExpr interface{} = strokeColor
	var circleColorExpr interface{} = pointColor

	switch styleType {
	case "single":
		// Uses static fillColor, strokeColor, pointColor

	case "categorized":
		field := getStringOrDefault(cfg, "field", "")
		if !models.ValidateFieldName(field) {
			return nil, fmt.Errorf("%w: categorized style requires a valid 'field'", models.ErrInvalidStyleConfig)
		}
		cats, _ := cfg["categories"].([]interface{})
		if len(cats) == 0 {
			return nil, fmt.Errorf("%w: categorized style requires non-empty 'categories'", models.ErrInvalidStyleConfig)
		}
		matchExpr := []interface{}{"match", []interface{}{"get", field}}
		for _, item := range cats {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			val := m["value"]
			col := getStringOrDefault(m, "color", fillColor)
			if !isValidCSSColor(col) {
				return nil, fmt.Errorf("%w: invalid category color '%s'", models.ErrInvalidStyleConfig, col)
			}
			matchExpr = append(matchExpr, val, col)
		}
		matchExpr = append(matchExpr, getStringOrDefault(cfg, "default_color", "#94a3b8"))
		colorExpr = matchExpr
		lineColorExpr = matchExpr
		circleColorExpr = matchExpr

	case "graduated":
		field := getStringOrDefault(cfg, "field", "")
		if !models.ValidateFieldName(field) {
			return nil, fmt.Errorf("%w: graduated style requires a valid numeric 'field'", models.ErrInvalidStyleConfig)
		}
		breaks, _ := cfg["breaks"].([]interface{})
		if len(breaks) == 0 {
			return nil, fmt.Errorf("%w: graduated style requires non-empty 'breaks'", models.ErrInvalidStyleConfig)
		}
		stepExpr := []interface{}{"step", []interface{}{"to-number", []interface{}{"get", field}, 0}, getStringOrDefault(cfg, "base_color", "#e0f2fe")}
		for _, item := range breaks {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			stopVal := getFloatOrDefault(m, "stop", getFloatOrDefault(m, "min", 0))
			col := getStringOrDefault(m, "color", fillColor)
			if !isValidCSSColor(col) {
				return nil, fmt.Errorf("%w: invalid graduated break color '%s'", models.ErrInvalidStyleConfig, col)
			}
			stepExpr = append(stepExpr, stopVal, col)
		}
		colorExpr = stepExpr
		lineColorExpr = stepExpr
		circleColorExpr = stepExpr

	case "rule_based":
		rules, _ := cfg["rules"].([]interface{})
		if len(rules) == 0 {
			return nil, fmt.Errorf("%w: rule_based style requires non-empty 'rules'", models.ErrInvalidStyleConfig)
		}
		caseExpr := []interface{}{"case"}
		for _, item := range rules {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			field := getStringOrDefault(m, "field", "")
			if !models.ValidateFieldName(field) {
				return nil, fmt.Errorf("%w: invalid field '%s' in rule", models.ErrInvalidStyleConfig, field)
			}
			op := getStringOrDefault(m, "operator", "=")
			val := m["value"]
			col := getStringOrDefault(m, "color", fillColor)
			if !isValidCSSColor(col) {
				return nil, fmt.Errorf("%w: invalid rule color '%s'", models.ErrInvalidStyleConfig, col)
			}
			mapLibreOp := "=="
			switch op {
			case "!=":
				mapLibreOp = "!="
			case ">":
				mapLibreOp = ">"
			case "<":
				mapLibreOp = "<"
			case ">=":
				mapLibreOp = ">="
			case "<=":
				mapLibreOp = "<="
			}
			caseExpr = append(caseExpr, []interface{}{mapLibreOp, []interface{}{"get", field}, val}, col)
		}
		caseExpr = append(caseExpr, getStringOrDefault(cfg, "default_color", fillColor))
		colorExpr = caseExpr
		lineColorExpr = caseExpr
		circleColorExpr = caseExpr

	case "heatmap":
		// Heatmap configuration

	default:
		return nil, fmt.Errorf("%w: unsupported style_type '%s'", models.ErrInvalidStyleConfig, styleType)
	}

	// Build Label configuration if enabled
	var labelSpec map[string]interface{}
	if labelField := getStringOrDefault(cfg, "label_field", ""); labelField != "" {
		if !models.ValidateFieldName(labelField) {
			return nil, fmt.Errorf("%w: invalid label_field '%s'", models.ErrInvalidStyleConfig, labelField)
		}
		labelSpec = map[string]interface{}{
			"enabled":    true,
			"field":      labelField,
			"text_size":  getFloatOrDefault(cfg, "label_size", 12),
			"text_color": getStringOrDefault(cfg, "label_color", "#ffffff"),
			"halo_color": getStringOrDefault(cfg, "label_halo_color", "#171717"),
			"halo_width": getFloatOrDefault(cfg, "label_halo_width", 1.5),
		}
	}

	compiled := map[string]interface{}{
		"style_type": styleType,
		"fill": map[string]interface{}{
			"fill-color":         colorExpr,
			"fill-opacity":       opacity * 0.45,
			"fill-outline-color": strokeColor,
		},
		"line": map[string]interface{}{
			"line-color":   lineColorExpr,
			"line-width":   strokeWidth,
			"line-opacity": opacity,
		},
		"circle": map[string]interface{}{
			"circle-color":        circleColorExpr,
			"circle-radius":       pointRadius,
			"circle-stroke-color": strokeColor,
			"circle-stroke-width": math.Max(1, strokeWidth*0.6),
			"circle-opacity":      opacity,
		},
		"heatmap": map[string]interface{}{
			"heatmap-radius":    getFloatOrDefault(cfg, "heatmap_radius", 24),
			"heatmap-intensity": getFloatOrDefault(cfg, "heatmap_intensity", 1.0),
			"heatmap-opacity":   opacity,
		},
		"label": labelSpec,
	}

	return compiled, nil
}

func isValidCSSColor(c string) bool {
	c = strings.TrimSpace(c)
	if hexColorRegex.MatchString(c) {
		return true
	}
	lower := strings.ToLower(c)
	if strings.HasPrefix(lower, "rgb(") || strings.HasPrefix(lower, "rgba(") || strings.HasPrefix(lower, "hsl(") {
		return true
	}
	named := map[string]bool{
		"red": true, "green": true, "blue": true, "white": true, "black": true,
		"yellow": true, "orange": true, "purple": true, "cyan": true, "magenta": true, "gray": true, "grey": true,
	}
	return named[lower]
}

func getStringOrDefault(m map[string]interface{}, key, def string) string {
	if v, ok := m[key]; ok && v != nil {
		if s, isStr := v.(string); isStr && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return def
}

func getFloatOrDefault(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := m[key]; ok && v != nil {
		if f, ok := toFloat64(v); ok {
			return f
		}
	}
	return def
}

// ============================================================================
// 6. CRS REGISTRY & COORDINATE TRANSFORMATION (MODULE E)
// ============================================================================

var builtinCRSCatalog = []models.CRSDefinition{
	{
		EPSGCode:    "EPSG:4326",
		SRID:        4326,
		Name:        "WGS 84 (Geographic 2D)",
		Type:        "geographic",
		Units:       "degrees",
		AreaOfUse:   "World",
		BoundsWGS84: []float64{-180, -90, 180, 90},
		Proj4:       "+proj=longlat +datum=WGS84 +no_defs",
	},
	{
		EPSGCode:    "EPSG:3857",
		SRID:        3857,
		Name:        "WGS 84 / Pseudo-Mercator (Web Mercator)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "World between 85.06°S and 85.06°N",
		BoundsWGS84: []float64{-180, -85.06, 180, 85.06},
		Proj4:       "+proj=merc +a=6378137 +b=6378137 +lat_ts=0 +lon_0=0 +x_0=0 +y_0=0 +k=1 +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:32643",
		SRID:        32643,
		Name:        "WGS 84 / UTM zone 43N (India West / Central)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "Northern hemisphere between 72°E and 78°E",
		BoundsWGS84: []float64{72, 0, 78, 84},
		Proj4:       "+proj=utm +zone=43 +datum=WGS84 +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:32644",
		SRID:        32644,
		Name:        "WGS 84 / UTM zone 44N (India Central / East)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "Northern hemisphere between 78°E and 84°E",
		BoundsWGS84: []float64{78, 0, 84, 84},
		Proj4:       "+proj=utm +zone=44 +datum=WGS84 +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:32632",
		SRID:        32632,
		Name:        "WGS 84 / UTM zone 32N (Central Europe)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "Northern hemisphere between 6°E and 12°E",
		BoundsWGS84: []float64{6, 0, 12, 84},
		Proj4:       "+proj=utm +zone=32 +datum=WGS84 +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:32633",
		SRID:        32633,
		Name:        "WGS 84 / UTM zone 33N (Central Europe)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "Northern hemisphere between 12°E and 18°E",
		BoundsWGS84: []float64{12, 0, 18, 84},
		Proj4:       "+proj=utm +zone=33 +datum=WGS84 +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:32610",
		SRID:        32610,
		Name:        "WGS 84 / UTM zone 10N (US West Coast)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "Northern hemisphere between 126°W and 120°W",
		BoundsWGS84: []float64{-126, 0, -120, 84},
		Proj4:       "+proj=utm +zone=10 +datum=WGS84 +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:32618",
		SRID:        32618,
		Name:        "WGS 84 / UTM zone 18N (US East Coast)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "Northern hemisphere between 78°W and 72°W",
		BoundsWGS84: []float64{-78, 0, -72, 84},
		Proj4:       "+proj=utm +zone=18 +datum=WGS84 +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:27700",
		SRID:        27700,
		Name:        "OSGB36 / British National Grid",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "United Kingdom (Great Britain)",
		BoundsWGS84: []float64{-9.0, 49.75, 2.01, 61.01},
		Proj4:       "+proj=tmerc +lat_0=49 +lon_0=-2 +k=0.9996012717 +x_0=400000 +y_0=-100000 +ellps=airy +units=m +no_defs",
	},
	{
		EPSGCode:    "EPSG:2154",
		SRID:        2154,
		Name:        "RGF93 v1 / Lambert-93 (France)",
		Type:        "projected",
		Units:       "meters",
		AreaOfUse:   "France",
		BoundsWGS84: []float64{-9.86, 41.15, 10.38, 51.56},
		Proj4:       "+proj=lcc +lat_0=46.5 +lon_0=3 +lat_1=49 +lat_2=44 +x_0=700000 +y_0=6600000 +ellps=GRS80 +units=m +no_defs",
	},
}

// GetSupportedCRSList returns all supported CRS definitions, optionally filtered by query string.
func GetSupportedCRSList(searchQuery string) []models.CRSDefinition {
	q := strings.ToLower(strings.TrimSpace(searchQuery))
	var results []models.CRSDefinition
	for _, item := range builtinCRSCatalog {
		if q == "" ||
			strings.Contains(strings.ToLower(item.EPSGCode), q) ||
			strings.Contains(strings.ToLower(item.Name), q) ||
			strings.Contains(strings.ToLower(item.AreaOfUse), q) ||
			strings.Contains(strconv.Itoa(item.SRID), q) {
			results = append(results, item)
		}
	}
	// Dynamically support any WGS84 UTM Zone 32601..32660 and 32701..32760 if searched
	if len(results) == 0 && q != "" {
		cleanCode := strings.TrimPrefix(strings.ToUpper(q), "EPSG:")
		if srid, err := strconv.Atoi(cleanCode); err == nil {
			if def, ok := buildDynamicUTMCRS(srid); ok {
				results = append(results, *def)
			}
		}
	}
	return results
}

// LookupCRS resolves an EPSG code or SRID into a CRSDefinition.
func LookupCRS(code string) (*models.CRSDefinition, error) {
	clean := strings.ToUpper(strings.TrimSpace(code))
	if clean == "" {
		return nil, models.ErrInvalidCRS
	}
	if !strings.HasPrefix(clean, "EPSG:") {
		if _, err := strconv.Atoi(clean); err == nil {
			clean = "EPSG:" + clean
		}
	}
	for _, item := range builtinCRSCatalog {
		if strings.EqualFold(item.EPSGCode, clean) {
			cp := item
			return &cp, nil
		}
	}
	if srid, err := strconv.Atoi(strings.TrimPrefix(clean, "EPSG:")); err == nil {
		if def, ok := buildDynamicUTMCRS(srid); ok {
			return def, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", models.ErrInvalidCRS, code)
}

func buildDynamicUTMCRS(srid int) (*models.CRSDefinition, bool) {
	if srid >= 32601 && srid <= 32660 {
		zone := srid - 32600
		minLon := float64((zone-1)*6 - 180)
		return &models.CRSDefinition{
			EPSGCode:    fmt.Sprintf("EPSG:%d", srid),
			SRID:        srid,
			Name:        fmt.Sprintf("WGS 84 / UTM zone %dN", zone),
			Type:        "projected",
			Units:       "meters",
			AreaOfUse:   fmt.Sprintf("Northern hemisphere UTM Zone %dN (%.0f° to %.0f°)", zone, minLon, minLon+6),
			BoundsWGS84: []float64{minLon, 0, minLon + 6, 84},
			Proj4:       fmt.Sprintf("+proj=utm +zone=%d +datum=WGS84 +units=m +no_defs", zone),
		}, true
	}
	if srid >= 32701 && srid <= 32760 {
		zone := srid - 32700
		minLon := float64((zone-1)*6 - 180)
		return &models.CRSDefinition{
			EPSGCode:    fmt.Sprintf("EPSG:%d", srid),
			SRID:        srid,
			Name:        fmt.Sprintf("WGS 84 / UTM zone %dS", zone),
			Type:        "projected",
			Units:       "meters",
			AreaOfUse:   fmt.Sprintf("Southern hemisphere UTM Zone %dS (%.0f° to %.0f°)", zone, minLon, minLon+6),
			BoundsWGS84: []float64{minLon, -80, minLon + 6, 0},
			Proj4:       fmt.Sprintf("+proj=utm +zone=%d +south +datum=WGS84 +units=m +no_defs", zone),
		}, true
	}
	return nil, false
}

// WGS84ToMercator converts EPSG:4326 [lng, lat] to EPSG:3857 [x, y] meters.
func WGS84ToMercator(lng, lat float64) (float64, float64) {
	lat = math.Max(-85.05112878, math.Min(85.05112878, lat))
	x := lng * 20037508.342789244 / 180.0
	y := math.Log(math.Tan((90.0+lat)*math.Pi/360.0)) / (math.Pi / 180.0)
	y = y * 20037508.342789244 / 180.0
	return x, y
}

// TransformCoordPair transforms a single [x, y] coordinate between sourceCRS and targetCRS.
func TransformCoordPair(x, y float64, sourceCRS, targetCRS string) (float64, float64, error) {
	srcDef, err := LookupCRS(sourceCRS)
	if err != nil {
		return 0, 0, err
	}
	dstDef, err := LookupCRS(targetCRS)
	if err != nil {
		return 0, 0, err
	}
	if srcDef.SRID == dstDef.SRID {
		return x, y, nil
	}

	// Step 1: Convert source to WGS84 (EPSG:4326)
	var lng, lat float64
	switch {
	case srcDef.SRID == 4326:
		lng, lat = x, y
	case srcDef.SRID == 3857:
		lng, lat = MercatorToWGS84(x, y)
	case srcDef.SRID >= 32601 && srcDef.SRID <= 32660:
		lng, lat = utmToWGS84(x, y, srcDef.SRID-32600, true)
	case srcDef.SRID >= 32701 && srcDef.SRID <= 32760:
		lng, lat = utmToWGS84(x, y, srcDef.SRID-32700, false)
	default:
		lng, lat = x, y
	}

	// Step 2: Convert WGS84 (EPSG:4326) to targetCRS
	switch {
	case dstDef.SRID == 4326:
		return lng, lat, nil
	case dstDef.SRID == 3857:
		mx, my := WGS84ToMercator(lng, lat)
		return mx, my, nil
	case dstDef.SRID >= 32601 && dstDef.SRID <= 32660:
		ux, uy := wgs84ToUTM(lng, lat, dstDef.SRID-32600, true)
		return ux, uy, nil
	case dstDef.SRID >= 32701 && dstDef.SRID <= 32760:
		ux, uy := wgs84ToUTM(lng, lat, dstDef.SRID-32700, false)
		return ux, uy, nil
	default:
		return lng, lat, nil
	}
}

func wgs84ToUTM(lng, lat float64, zone int, northern bool) (float64, float64) {
	const a = 6378137.0
	const f = 1.0 / 298.257223563
	const k0 = 0.9996
	e2 := 2*f - f*f
	ep2 := e2 / (1 - e2)

	latRad := lat * math.Pi / 180.0
	lonRad := lng * math.Pi / 180.0
	lon0Rad := float64((zone-1)*6-180+3) * math.Pi / 180.0

	N := a / math.Sqrt(1-e2*math.Sin(latRad)*math.Sin(latRad))
	T := math.Tan(latRad) * math.Tan(latRad)
	C := ep2 * math.Cos(latRad) * math.Cos(latRad)
	A := math.Cos(latRad) * (lonRad - lon0Rad)

	M := a * ((1-e2/4-3*e2*e2/64)*latRad - (3*e2/8+3*e2*e2/32)*math.Sin(2*latRad) + (15*e2*e2/256)*math.Sin(4*latRad))

	x := k0*N*(A+(1-T+C)*math.Pow(A, 3)/6+(5-18*T+T*T+72*C-58*ep2)*math.Pow(A, 5)/120) + 500000.0
	y := k0 * (M + N*math.Tan(latRad)*(A*A/2+(5-T+9*C+4*C*C)*math.Pow(A, 4)/24))
	if !northern {
		y += 10000000.0
	}
	return x, y
}

func utmToWGS84(x, y float64, zone int, northern bool) (float64, float64) {
	const a = 6378137.0
	const f = 1.0 / 298.257223563
	const k0 = 0.9996
	e2 := 2*f - f*f
	ep2 := e2 / (1 - e2)
	e1 := (1 - math.Sqrt(1-e2)) / (1 + math.Sqrt(1-e2))

	xAdj := x - 500000.0
	yAdj := y
	if !northern {
		yAdj -= 10000000.0
	}

	M := yAdj / k0
	mu := M / (a * (1 - e2/4 - 3*e2*e2/64))
	phi1 := mu + (3*e1/2-27*math.Pow(e1, 3)/32)*math.Sin(2*mu) + (21*e1*e1/16)*math.Sin(4*mu)

	N1 := a / math.Sqrt(1-e2*math.Sin(phi1)*math.Sin(phi1))
	T1 := math.Tan(phi1) * math.Tan(phi1)
	C1 := ep2 * math.Cos(phi1) * math.Cos(phi1)
	R1 := a * (1 - e2) / math.Pow(1-e2*math.Sin(phi1)*math.Sin(phi1), 1.5)
	D := xAdj / (N1 * k0)

	latRad := phi1 - (N1*math.Tan(phi1)/R1)*(D*D/2-(5+3*T1+10*C1-4*C1*C1-9*ep2)*math.Pow(D, 4)/24)
	lon0Rad := float64((zone-1)*6-180+3) * math.Pi / 180.0
	lonRad := lon0Rad + (D-(1+2*T1+C1)*math.Pow(D, 3)/6)/math.Cos(phi1)

	return lonRad * 180.0 / math.Pi, latRad * 180.0 / math.Pi
}

// TransformGeometryCRS transforms a GeoJSON geometry between sourceCRS and targetCRS.
func TransformGeometryCRS(geom models.GeoJSONGeometry, sourceCRS, targetCRS string) (*models.GeoJSONGeometry, error) {
	if _, err := LookupCRS(sourceCRS); err != nil {
		return nil, err
	}
	if _, err := LookupCRS(targetCRS); err != nil {
		return nil, err
	}

	txPt := func(pt []float64) ([]float64, error) {
		if len(pt) < 2 {
			return pt, nil
		}
		nx, ny, err := TransformCoordPair(pt[0], pt[1], sourceCRS, targetCRS)
		if err != nil {
			return nil, err
		}
		return []float64{nx, ny}, nil
	}

	switch geom.Type {
	case "Point":
		var pt []float64
		if err := json.Unmarshal(geom.Coordinates, &pt); err != nil {
			return nil, err
		}
		npt, err := txPt(pt)
		if err != nil {
			return nil, err
		}
		raw, _ := json.Marshal(npt)
		return &models.GeoJSONGeometry{Type: "Point", Coordinates: raw}, nil

	case "MultiPoint", "LineString":
		var pts [][]float64
		if err := json.Unmarshal(geom.Coordinates, &pts); err != nil {
			return nil, err
		}
		for i := range pts {
			npt, err := txPt(pts[i])
			if err != nil {
				return nil, err
			}
			pts[i] = npt
		}
		raw, _ := json.Marshal(pts)
		return &models.GeoJSONGeometry{Type: geom.Type, Coordinates: raw}, nil

	case "MultiLineString", "Polygon":
		var rings [][][]float64
		if err := json.Unmarshal(geom.Coordinates, &rings); err != nil {
			return nil, err
		}
		for r := range rings {
			for i := range rings[r] {
				npt, err := txPt(rings[r][i])
				if err != nil {
					return nil, err
				}
				rings[r][i] = npt
			}
		}
		raw, _ := json.Marshal(rings)
		return &models.GeoJSONGeometry{Type: geom.Type, Coordinates: raw}, nil

	case "MultiPolygon":
		var polys [][][][]float64
		if err := json.Unmarshal(geom.Coordinates, &polys); err != nil {
			return nil, err
		}
		for p := range polys {
			for r := range polys[p] {
				for i := range polys[p][r] {
					npt, err := txPt(polys[p][r][i])
					if err != nil {
						return nil, err
					}
					polys[p][r][i] = npt
				}
			}
		}
		raw, _ := json.Marshal(polys)
		return &models.GeoJSONGeometry{Type: "MultiPolygon", Coordinates: raw}, nil
	}

	return &geom, nil
}

// InspectVectorDatasetFile inspects a GeoPackage, Shapefile, GeoJSON, KML, OSM, ZIP, or DXF file using GDAL/OGR (vector_tool.py).
func InspectVectorDatasetFile(filePath string) ([]models.DatasetLayerInfo, string, error) {
	var res struct {
		Success bool   `json:"success"`
		Driver  string `json:"driver"`
		Error   string `json:"error"`
		Layers  []struct {
			LayerName    string                   `json:"layer_name"`
			GeometryType string                   `json:"geometry_type"`
			LayerKind    string                   `json:"layer_kind"`
			CRS          string                   `json:"crs"`
			FeatureCount int                      `json:"feature_count"`
			Bounds       []float64                `json:"bounds"`
			WGS84Bounds  []float64                `json:"wgs84_bounds"`
			Fields       []map[string]interface{} `json:"fields"`
		} `json:"layers"`
	}

	if err := RunPythonJSON(30*time.Second, "vector_tool.py", []string{"inspect", filePath}, &res); err != nil {
		return nil, "", err
	}
	if !res.Success {
		return nil, "", fmt.Errorf("%s", res.Error)
	}

	var out []models.DatasetLayerInfo
	now := time.Now().UTC()
	for i, l := range res.Layers {
		bounds := l.WGS84Bounds
		if len(bounds) != 4 {
			bounds = l.Bounds
		}
		out = append(out, models.DatasetLayerInfo{
			DatasetLayerID: fmt.Sprintf("dsl_%d_%s", i+1, sanitizeID(l.LayerName)),
			LayerName:      l.LayerName,
			GeometryType:   l.GeometryType,
			CRS:            l.CRS,
			FeatureCount:   l.FeatureCount,
			Bounds:         bounds,
			Fields:         l.Fields,
			CreatedAt:      now,
		})
	}
	return out, res.Driver, nil
}

// ExtractedLayerFeature represents one extracted feature from an arbitrary GIS dataset layer.
type ExtractedLayerFeature struct {
	FeatureID  string                  `json:"feature_id"`
	Geometry   *models.GeoJSONGeometry `json:"geometry,omitempty"`
	Properties map[string]interface{}  `json:"properties"`
}

// ExtractedLayerResult holds features extracted from a specific layer of a GeoPackage/Shapefile/GeoJSON.
type ExtractedLayerResult struct {
	LayerName     string                  `json:"layer_name"`
	SourceCRS     string                  `json:"source_crs"`
	GeometryType  string                  `json:"geometry_type"`
	TotalFeatures int                     `json:"total_features"`
	Bounds        []float64               `json:"bounds,omitempty"`
	Features      []ExtractedLayerFeature `json:"features"`
}

// ExtractDatasetLayerFeatures dynamically reads features from a named layer via GDAL/OGR (vector_tool.py).
func ExtractDatasetLayerFeatures(filePath, layerName string, maxFeatures, offset int) (*ExtractedLayerResult, error) {
	if maxFeatures <= 0 {
		maxFeatures = 5000
	}
	if offset < 0 {
		offset = 0
	}
	var res struct {
		Success       bool                    `json:"success"`
		Error         string                  `json:"error"`
		LayerName     string                  `json:"layer_name"`
		SourceCRS     string                  `json:"source_crs"`
		GeometryType  string                  `json:"geometry_type"`
		TotalFeatures int                     `json:"total_features"`
		Bounds        []float64               `json:"bounds"`
		Features      []ExtractedLayerFeature `json:"features"`
	}
	args := []string{"extract_layer", filePath, layerName, fmt.Sprintf("%d", maxFeatures), fmt.Sprintf("%d", offset)}
	if err := RunPythonJSON(60*time.Second, "vector_tool.py", args, &res); err != nil {
		return nil, err
	}
	if !res.Success {
		return nil, fmt.Errorf("%s", res.Error)
	}
	return &ExtractedLayerResult{
		LayerName:     res.LayerName,
		SourceCRS:     res.SourceCRS,
		GeometryType:  res.GeometryType,
		TotalFeatures: res.TotalFeatures,
		Bounds:        res.Bounds,
		Features:      res.Features,
	}, nil
}

func sanitizeID(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	res := sb.String()
	if len(res) > 24 {
		res = res[:24]
	}
	return res
}

// SortAndFilterFeatures applies attribute conditions, text search, and column sorting to a slice of VectorFeatures.
func SortAndFilterFeatures(
	features []*models.VectorFeature,
	conditions []models.AttributeFilterCondition,
	combineOp string,
	search string,
	sortBy string,
	sortOrder string,
) []*models.VectorFeature {
	combineOp = strings.ToUpper(strings.TrimSpace(combineOp))
	if combineOp == "" {
		combineOp = "AND"
	}
	searchLower := strings.ToLower(strings.TrimSpace(search))

	var filtered []*models.VectorFeature
	for _, f := range features {
		if searchLower != "" {
			matchedSearch := strings.Contains(strings.ToLower(f.FeatureID), searchLower)
			if !matchedSearch {
				for _, v := range f.Properties {
					if strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), searchLower) {
						matchedSearch = true
						break
					}
				}
			}
			if !matchedSearch {
				continue
			}
		}

		if len(conditions) > 0 {
			var matches bool
			if combineOp == "OR" {
				matches = false
				for _, cond := range conditions {
					if evaluateAttributeCondition(f.Properties, cond) {
						matches = true
						break
					}
				}
			} else {
				matches = true
				for _, cond := range conditions {
					if !evaluateAttributeCondition(f.Properties, cond) {
						matches = false
						break
					}
				}
			}
			if !matches {
				continue
			}
		}
		filtered = append(filtered, f)
	}

	if sortBy != "" {
		desc := strings.EqualFold(sortOrder, "desc")
		sort.SliceStable(filtered, func(i, j int) bool {
			vi := getFeatureSortVal(filtered[i], sortBy)
			vj := getFeatureSortVal(filtered[j], sortBy)
			cmp := compareValues(vi, vj)
			if desc {
				return cmp > 0
			}
			return cmp < 0
		})
	}

	return filtered
}

func getFeatureSortVal(f *models.VectorFeature, sortBy string) interface{} {
	switch sortBy {
	case "feature_id", "id":
		return f.FeatureID
	case "geometry_type":
		return f.GeometryType
	case "created_at":
		return f.CreatedAt.UnixNano()
	case "updated_at":
		return f.UpdatedAt.UnixNano()
	default:
		if f.Properties != nil {
			return f.Properties[sortBy]
		}
		return nil
	}
}

func evaluateAttributeCondition(props map[string]interface{}, cond models.AttributeFilterCondition) bool {
	op := strings.ToUpper(strings.TrimSpace(cond.Operator))
	var val interface{}
	exists := false
	if props != nil {
		val, exists = props[cond.Field]
	}

	if op == "IS NULL" {
		return !exists || val == nil || fmt.Sprintf("%v", val) == ""
	}
	if op == "IS NOT NULL" {
		return exists && val != nil && fmt.Sprintf("%v", val) != ""
	}
	if !exists || val == nil {
		return false
	}

	switch op {
	case "=":
		return compareValues(val, cond.Value) == 0
	case "!=":
		return compareValues(val, cond.Value) != 0
	case ">":
		return compareValues(val, cond.Value) > 0
	case "<":
		return compareValues(val, cond.Value) < 0
	case ">=":
		return compareValues(val, cond.Value) >= 0
	case "<=":
		return compareValues(val, cond.Value) <= 0
	case "LIKE":
		pattern := strings.ToLower(fmt.Sprintf("%v", cond.Value))
		pattern = strings.Trim(pattern, "%")
		return strings.Contains(strings.ToLower(fmt.Sprintf("%v", val)), pattern)
	case "IN":
		valStr := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", val)))
		switch arr := cond.Value.(type) {
		case []interface{}:
			for _, item := range arr {
				if strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", item))) == valStr {
					return true
				}
			}
		case []string:
			for _, item := range arr {
				if strings.ToLower(strings.TrimSpace(item)) == valStr {
					return true
				}
			}
		case string:
			for _, part := range strings.Split(arr, ",") {
				if strings.ToLower(strings.TrimSpace(part)) == valStr {
					return true
				}
			}
		}
		return false
	}
	return false
}

func compareValues(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	fa, okA := toFloat64(a)
	fb, okB := toFloat64(b)
	if okA && okB {
		if math.Abs(fa-fb) < 1e-11 {
			return 0
		}
		if fa < fb {
			return -1
		}
		return 1
	}
	sa := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", a)))
	sb := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", b)))
	return strings.Compare(sa, sb)
}
