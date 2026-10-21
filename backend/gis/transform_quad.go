package gis

import (
	"fmt"
	"math"

	"github.com/malikharman0823/veracity-maps/models"
)

type CoordinatePoint struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type RasterCorners struct {
	TopLeft     CoordinatePoint `json:"top_left"`
	TopRight    CoordinatePoint `json:"top_right"`
	BottomRight CoordinatePoint `json:"bottom_right"`
	BottomLeft  CoordinatePoint `json:"bottom_left"`
}

// PolygonIsSimpleAndConvex validates that the 4-corner quadrilateral is non-degenerate and convex.
func PolygonIsSimpleAndConvex(corners RasterCorners) bool {
	pts := [][2]float64{
		{corners.TopLeft.Longitude, corners.TopLeft.Latitude},
		{corners.TopRight.Longitude, corners.TopRight.Latitude},
		{corners.BottomRight.Longitude, corners.BottomRight.Latitude},
		{corners.BottomLeft.Longitude, corners.BottomLeft.Latitude},
	}

	// 1. Check distinct points
	for i := 0; i < 4; i++ {
		if math.IsNaN(pts[i][0]) || math.IsNaN(pts[i][1]) || math.IsInf(pts[i][0], 0) || math.IsInf(pts[i][1], 0) {
			return false
		}
		if pts[i][0] < -180 || pts[i][0] > 180 || pts[i][1] < -90 || pts[i][1] > 90 {
			return false
		}
		for j := i + 1; j < 4; j++ {
			dx := pts[i][0] - pts[j][0]
			dy := pts[i][1] - pts[j][1]
			if math.Abs(dx) < 1e-9 && math.Abs(dy) < 1e-9 {
				return false
			}
		}
	}

	// 2. Check cross products of consecutive edges for consistent sign (convexity & no self-intersection)
	var crossSigns []int
	for i := 0; i < 4; i++ {
		p1 := pts[i]
		p2 := pts[(i+1)%4]
		p3 := pts[(i+2)%4]

		v1x := p2[0] - p1[0]
		v1y := p2[1] - p1[1]
		v2x := p3[0] - p2[0]
		v2y := p3[1] - p2[1]

		cp := v1x*v2y - v1y*v2x
		if math.Abs(cp) < 1e-12 {
			return false // Collinear edge or zero area
		}
		if cp > 0 {
			crossSigns = append(crossSigns, 1)
		} else {
			crossSigns = append(crossSigns, -1)
		}
	}

	first := crossSigns[0]
	for _, s := range crossSigns {
		if s != first {
			return false
		}
	}

	return true
}

// ExtractRasterCornersWGS84 extracts the 4 corners, bounds, width, and height of a dataset in WGS84.
func ExtractRasterCornersWGS84(dataset *models.Dataset) (RasterCorners, []float64, int, int, error) {
	if dataset == nil {
		return RasterCorners{}, nil, 0, 0, fmt.Errorf("DATASET_NOT_FOUND: Dataset is nil")
	}

	meta := dataset.ExtractedMetadata
	if meta == nil {
		meta = make(map[string]interface{})
	}

	width := 0
	height := 0
	if w, ok := meta["width"].(float64); ok {
		width = int(w)
	} else if wInt, ok := meta["width"].(int); ok {
		width = wInt
	}
	if h, ok := meta["height"].(float64); ok {
		height = int(h)
	} else if hInt, ok := meta["height"].(int); ok {
		height = hInt
	}

	if width <= 0 || height <= 0 {
		// Read dimensions from storage file
		w, h, err := ImageDimensions(dataset.StoragePath)
		if err == nil && w > 0 && h > 0 {
			width = w
			height = h
		} else {
			width = 1000
			height = 800
		}
	}

	// 1. Try MapLayer coordinates if present
	if dataset.MapLayer != nil {
		if coordsRaw, ok := dataset.MapLayer["coordinates"].([][]float64); ok && len(coordsRaw) == 4 {
			corners := RasterCorners{
				TopLeft:     CoordinatePoint{Longitude: coordsRaw[0][0], Latitude: coordsRaw[0][1]},
				TopRight:    CoordinatePoint{Longitude: coordsRaw[1][0], Latitude: coordsRaw[1][1]},
				BottomRight: CoordinatePoint{Longitude: coordsRaw[2][0], Latitude: coordsRaw[2][1]},
				BottomLeft:  CoordinatePoint{Longitude: coordsRaw[3][0], Latitude: coordsRaw[3][1]},
			}
			bounds := CalculateBoundsFromCorners(corners)
			return corners, bounds, width, height, nil
		}
	}

	// 2. Try affine transform from metadata
	transform := meta["transform"]
	if transform != nil {
		var t []float64
		if tFloat, ok := transform.([]float64); ok && len(tFloat) == 6 {
			t = tFloat
		} else if tArr, ok := transform.([]interface{}); ok && len(tArr) == 6 {
			for _, v := range tArr {
				if f, ok := v.(float64); ok {
					t = append(t, f)
				}
			}
		}
		if len(t) == 6 {
			a, b, c := t[0], -t[1], t[2]
			d, e, f := t[3], -t[4], t[5]
			wF := float64(width)
			hF := float64(height)
			project := func(ix, iy float64) CoordinatePoint {
				return CoordinatePoint{
					Longitude: a*ix + b*(-iy) + c,
					Latitude:  d*ix + e*(-iy) + f,
				}
			}
			corners := RasterCorners{
				TopLeft:     project(0, 0),
				TopRight:    project(wF, 0),
				BottomRight: project(wF, hF),
				BottomLeft:  project(0, hF),
			}
			bounds := CalculateBoundsFromCorners(corners)
			return corners, bounds, width, height, nil
		}
	}

	// 3. Fallback to bounds
	var minX, minY, maxX, maxY float64
	hasBounds := false
	if bMap, ok := meta["bounds"].(map[string]interface{}); ok {
		if x1, ok1 := bMap["min_x"].(float64); ok1 {
			if y1, ok2 := bMap["min_y"].(float64); ok2 {
				if x2, ok3 := bMap["max_x"].(float64); ok3 {
					if y2, ok4 := bMap["max_y"].(float64); ok4 && x2 > x1 && y2 > y1 {
						minX, minY, maxX, maxY = x1, y1, x2, y2
						hasBounds = true
					}
				}
			}
		}
	}

	if !hasBounds {
		// Default to [0, 0, 0.01, 0.01] or around equator if completely ungeoreferenced
		minX, minY, maxX, maxY = -0.005, -0.005, 0.005, 0.005
	}

	corners := RasterCorners{
		TopLeft:     CoordinatePoint{Longitude: minX, Latitude: maxY},
		TopRight:    CoordinatePoint{Longitude: maxX, Latitude: maxY},
		BottomRight: CoordinatePoint{Longitude: maxX, Latitude: minY},
		BottomLeft:  CoordinatePoint{Longitude: minX, Latitude: minY},
	}
	bounds := []float64{minX, minY, maxX, maxY}
	return corners, bounds, width, height, nil
}

// CalculateBoundsFromCorners calculates the [west, south, east, north] bounding box of 4 corners.
func CalculateBoundsFromCorners(corners RasterCorners) []float64 {
	minLng := math.Min(math.Min(corners.TopLeft.Longitude, corners.TopRight.Longitude), math.Min(corners.BottomRight.Longitude, corners.BottomLeft.Longitude))
	maxLng := math.Max(math.Max(corners.TopLeft.Longitude, corners.TopRight.Longitude), math.Max(corners.BottomRight.Longitude, corners.BottomLeft.Longitude))
	minLat := math.Min(math.Min(corners.TopLeft.Latitude, corners.TopRight.Latitude), math.Min(corners.BottomRight.Latitude, corners.BottomLeft.Latitude))
	maxLat := math.Max(math.Max(corners.TopLeft.Latitude, corners.TopRight.Latitude), math.Max(corners.BottomRight.Latitude, corners.BottomLeft.Latitude))
	return []float64{minLng, minLat, maxLng, maxLat}
}

// CalculateTransformFromCorners fits an affine transformation matrix [a, -b, c, d, -e, f] from 4 target corners.
func CalculateTransformFromCorners(corners RasterCorners, width, height int) ([]float64, []float64, error) {
	if width <= 0 || height <= 0 {
		return nil, nil, fmt.Errorf("INVALID_DIMENSIONS: Width and height must be positive")
	}

	wF := float64(width)
	hF := float64(height)

	// Pixel coords:
	// P0: (0, 0)       -> corners.TopLeft
	// P1: (w, 0)       -> corners.TopRight
	// P2: (w, h)       -> corners.BottomRight
	// P3: (0, h)       -> corners.BottomLeft
	pts := []struct {
		x, y, lon, lat float64
	}{
		{0, 0, corners.TopLeft.Longitude, corners.TopLeft.Latitude},
		{wF, 0, corners.TopRight.Longitude, corners.TopRight.Latitude},
		{wF, hF, corners.BottomRight.Longitude, corners.BottomRight.Latitude},
		{0, hF, corners.BottomLeft.Longitude, corners.BottomLeft.Latitude},
	}

	// Least-squares fit for:
	// a*x + b*(-y) + c = lon
	// d*x + e*(-y) + f = lat
	var sxx, sxy, sx, syy, sy, s1 float64
	var sxLon, syLon, sLon float64
	var sxLat, syLat, sLat float64

	for _, p := range pts {
		x := p.x
		y := -p.y
		lon := p.lon
		lat := p.lat

		sxx += x * x
		sxy += x * y
		sx += x
		syy += y * y
		sy += y
		s1 += 1.0

		sxLon += x * lon
		syLon += y * lon
		sLon += lon

		sxLat += x * lat
		syLat += y * lat
		sLat += lat
	}

	det := sxx*(syy*s1-sy*sy) - sxy*(sxy*s1-sx*sy) + sx*(sxy*sy-sx*syy)
	if math.Abs(det) < 1e-12 {
		return nil, nil, fmt.Errorf("SINGULAR_MATRIX: Degenerate corner geometry")
	}

	inv00 := (syy*s1 - sy*sy) / det
	inv01 := -(sxy*s1 - sx*sy) / det
	inv02 := (sxy*sy - sx*syy) / det

	inv10 := -(sxy*s1 - sy*sx) / det
	inv11 := (sxx*s1 - sx*sx) / det
	inv12 := -(sxx*sy - sx*sxy) / det

	inv20 := (sxy*sy - sx*syy) / det
	inv21 := -(sxx*sy - sx*sxy) / det
	inv22 := (sxx*syy - sxy*sxy) / det

	a := inv00*sxLon + inv01*syLon + inv02*sLon
	b := inv10*sxLon + inv11*syLon + inv12*sLon
	c := inv20*sxLon + inv21*syLon + inv22*sLon

	d := inv00*sxLat + inv01*syLat + inv02*sLat
	e := inv10*sxLat + inv11*syLat + inv12*sLat
	f := inv20*sxLat + inv21*syLat + inv22*sLat

	transform := []float64{
		a,
		-b,
		c,
		d,
		-e,
		f,
	}

	bounds := CalculateBoundsFromCorners(corners)
	return transform, bounds, nil
}

// SynchronizeControlPoints recalculates each control point's geographic coordinates (longitude, latitude)
// from its authoritative source pixel coordinates (image_x, image_y) using the new affine transformation.
func SynchronizeControlPoints(cps []*models.ControlPoint, transform []float64) {
	if len(transform) != 6 {
		return
	}
	a, b, c := transform[0], -transform[1], transform[2]
	d, e, f := transform[3], -transform[4], transform[5]

	for _, cp := range cps {
		if cp == nil {
			continue
		}
		cp.Longitude = a*cp.ImageX + b*(-cp.ImageY) + c
		cp.Latitude = d*cp.ImageX + e*(-cp.ImageY) + f
	}
}
