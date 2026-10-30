package gis

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/malikharman0823/veracity-maps/models"
)

type TransformResult struct {
	Transformation    string             `json:"transformation"`
	Parameters        map[string]float64 `json:"parameters"`
	Transform         []float64          `json:"transform"` // [a, b, c, d, e, f]
	TargetCRS         string             `json:"target_crs"`
	Bounds            map[string]float64 `json:"bounds"`
	MapLibreBounds    []float64          `json:"maplibre_bounds"` // [west, south, east, north]
	ControlPointCount int                `json:"control_point_count,omitempty"`
}

// CalculateGeoreference validates points and computes similarity or affine transformation
func CalculateGeoreference(points []*models.ControlPoint, width, height int, requestedTransform string) (*TransformResult, error) {
	if len(points) == 0 {
		return nil, fmt.Errorf("NO_CONTROL_POINTS: No control points were found")
	}
	if len(points) < 2 {
		return nil, fmt.Errorf("INSUFFICIENT_CONTROL_POINTS: Add at least 2 complete control-point pairs")
	}
	if len(points) > 10 {
		return nil, fmt.Errorf("TOO_MANY_CONTROL_POINTS: Visual georeferencer supports a maximum of 10 control points")
	}

	transformation := "similarity"
	if len(points) >= 3 {
		transformation = "affine"
	}
	if requestedTransform != "" && requestedTransform != "auto" && requestedTransform != transformation {
		return nil, fmt.Errorf("INVALID_TRANSFORMATION: %d points require the %s transformation", len(points), transformation)
	}

	// Validate points
	for _, p := range points {
		if math.IsNaN(p.ImageX) || math.IsNaN(p.ImageY) || math.IsNaN(p.Longitude) || math.IsNaN(p.Latitude) {
			return nil, fmt.Errorf("INVALID_CONTROL_POINT: Coordinates must be finite numbers")
		}
		if p.ImageX < 0 || p.ImageX >= float64(width) || p.ImageY < 0 || p.ImageY >= float64(height) {
			return nil, fmt.Errorf("INVALID_CONTROL_POINT: Image coordinates must fall within raster dimensions (%dx%d)", width, height)
		}
		if p.Longitude < -180 || p.Longitude > 180 || p.Latitude < -90 || p.Latitude > 90 {
			return nil, fmt.Errorf("INVALID_CONTROL_POINT: Longitude must be [-180, 180] and latitude [-90, 90]")
		}
	}

	// Validate distribution and collinearity
	if err := validateDistribution(points, width, height); err != nil {
		return nil, err
	}

	var params map[string]float64
	var err error
	if transformation == "similarity" {
		params, err = solveSimilarity(points)
	} else {
		params, err = solveAffine(points)
	}
	if err != nil {
		return nil, err
	}

	// Transform format: a, -b, c, d, -e, f
	transform := []float64{
		params["a"],
		-params["b"],
		params["c"],
		params["d"],
		-params["e"],
		params["f"],
	}

	bounds := CalculateBoundsFromTransform(transform, width, height)
	maplibreBounds, err := NormalizeBounds(bounds)
	if err != nil {
		return nil, fmt.Errorf("INVALID_BOUNDS: %w", err)
	}

	return &TransformResult{
		Transformation:    transformation,
		Parameters:        params,
		Transform:         transform,
		TargetCRS:         "EPSG:4326",
		Bounds:            bounds,
		MapLibreBounds:    maplibreBounds,
		ControlPointCount: len(points),
	}, nil
}

func validateDistribution(points []*models.ControlPoint, width, height int) error {
	w := float64(width)
	h := float64(height)

	minX, maxX := points[0].ImageX, points[0].ImageX
	minY, maxY := points[0].ImageY, points[0].ImageY

	for _, p := range points {
		if p.ImageX < minX {
			minX = p.ImageX
		}
		if p.ImageX > maxX {
			maxX = p.ImageX
		}
		if p.ImageY < minY {
			minY = p.ImageY
		}
		if p.ImageY > maxY {
			maxY = p.ImageY
		}
	}

	horizontal := maxX - minX
	vertical := maxY - minY
	imageDiag := math.Hypot(w, h)
	extentDiag := math.Hypot(horizontal, vertical)

	if len(points) == 2 {
		if extentDiag < imageDiag*0.12 {
			return fmt.Errorf("POOR_CONTROL_POINT_DISTRIBUTION: Move the two points farther apart across the source image")
		}
		return nil
	}

	if horizontal < w*0.1 || vertical < h*0.1 || extentDiag < imageDiag*0.2 {
		return fmt.Errorf("POOR_CONTROL_POINT_DISTRIBUTION: Move the points farther apart across the source image")
	}

	// Check 3-point triangle area for collinearity
	largestArea := 0.0
	n := len(points)
	for i := 0; i < n-2; i++ {
		p1 := points[i]
		for j := i + 1; j < n-1; j++ {
			p2 := points[j]
			for k := j + 1; k < n; k++ {
				p3 := points[k]
				area := math.Abs((p2.ImageX-p1.ImageX)*(p3.ImageY-p1.ImageY) - (p2.ImageY-p1.ImageY)*(p3.ImageX-p1.ImageX))
				if area > largestArea {
					largestArea = area
				}
			}
		}
	}

	if largestArea < w*h*0.002 {
		return fmt.Errorf("COLLINEAR_CONTROL_POINTS: Move at least one point away from the shared line")
	}
	return nil
}

func solveSimilarity(points []*models.ControlPoint) (map[string]float64, error) {
	first := points[0]
	second := points[1]

	// Apply conformal local/Web-Mercator cos(meanLat) scaling so 1° of longitude
	// is weighted by its true ground/Google Maps horizontal scale relative to 1° of latitude.
	meanLatRad := ((first.Latitude + second.Latitude) / 2.0) * math.Pi / 180.0
	cosLat := math.Cos(meanLatRad)
	if cosLat < 1e-6 {
		cosLat = 1e-6
	}

	srcX := second.ImageX - first.ImageX
	srcY := -second.ImageY - (-first.ImageY)
	tgtX := (second.Longitude - first.Longitude) * cosLat
	tgtY := second.Latitude - first.Latitude

	srcLen := math.Hypot(srcX, srcY)
	tgtLen := math.Hypot(tgtX, tgtY)

	if srcLen <= 0 || tgtLen <= 0 {
		return nil, fmt.Errorf("POOR_CONTROL_POINT_DISTRIBUTION: Each point pair must identify two distinct locations")
	}

	scale := tgtLen / srcLen
	cosTheta := (srcX*tgtX + srcY*tgtY) / (srcLen * tgtLen)
	sinTheta := (srcX*tgtY - srcY*tgtX) / (srcLen * tgtLen)

	a := (scale * cosTheta) / cosLat
	b := (-scale * sinTheta) / cosLat
	d := scale * sinTheta
	e := scale * cosTheta

	firstY := -first.ImageY
	c := first.Longitude - a*first.ImageX - b*firstY
	f := first.Latitude - d*first.ImageX - e*firstY

	return map[string]float64{
		"a":                a,
		"b":                b,
		"c":                c,
		"d":                d,
		"e":                e,
		"f":                f,
		"scale":            scale,
		"rotation_degrees": math.Atan2(sinTheta, cosTheta) * 180 / math.Pi,
	}, nil
}

func solveAffine(points []*models.ControlPoint) (map[string]float64, error) {
	// Fit linear regression:
	// a*x + b*(-y) + c = lon
	// d*x + e*(-y) + f = lat
	// Matrix X has columns [x, -y, 1.0]
	var sxx, sxy, sx, syy, sy, s1 float64
	var sxLon, syLon, sLon float64
	var sxLat, syLat, sLat float64

	for _, p := range points {
		x := p.ImageX
		y := -p.ImageY
		lon := p.Longitude
		lat := p.Latitude

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

	// 3x3 normal matrix M:
	// [ sxx, sxy, sx ]
	// [ sxy, syy, sy ]
	// [ sx,  sy,  s1 ]
	det := sxx*(syy*s1-sy*sy) - sxy*(sxy*s1-sy*sx) + sx*(sxy*sy-syy*sx)
	if math.Abs(det) < 1e-9 {
		return nil, fmt.Errorf("COLLINEAR_CONTROL_POINTS: Control points are collinear and cannot produce an affine transformation")
	}

	// Inverse of 3x3 matrix
	inv00 := (syy*s1 - sy*sy) / det
	inv01 := (sx*sy - sxy*s1) / det
	inv02 := (sxy*sy - sx*syy) / det

	inv10 := (sy*sx - sxy*s1) / det
	inv11 := (sxx*s1 - sx*sx) / det
	inv12 := (sx*sxy - sxx*sy) / det

	inv20 := (sxy*sy - sx*syy) / det
	inv21 := (sx*sxy - sxx*sy) / det
	inv22 := (sxx*syy - sxy*sxy) / det

	a := inv00*sxLon + inv01*syLon + inv02*sLon
	b := inv10*sxLon + inv11*syLon + inv12*sLon
	c := inv20*sxLon + inv21*syLon + inv22*sLon

	d := inv00*sxLat + inv01*syLat + inv02*sLat
	e := inv10*sxLat + inv11*syLat + inv12*sLat
	f := inv20*sxLat + inv21*syLat + inv22*sLat

	return map[string]float64{
		"a": a,
		"b": b,
		"c": c,
		"d": d,
		"e": e,
		"f": f,
	}, nil
}

// WriteWorldFile writes a standard 6-line world file
func WriteWorldFile(path string, transform []float64) error {
	// transform: a, b, c, d, e, f
	// world file: A, D, B, E, C, F
	content := fmt.Sprintf("%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n",
		transform[0], // A = a
		transform[3], // D = d
		transform[1], // B = b
		transform[4], // E = e
		transform[2], // C = c
		transform[5], // F = f
	)
	return os.WriteFile(path, []byte(content), 0644)
}

// WriteGDALVrt generates a GDAL VRT XML for georeferencing
func WriteGDALVrt(vrtPath, sourceImagePath string, width, height int, transform []float64, crs string) error {
	relPath, err := filepath.Rel(filepath.Dir(vrtPath), sourceImagePath)
	if err != nil {
		relPath = filepath.Base(sourceImagePath)
	}

	geoTransformStr := fmt.Sprintf("%.10f, %.10f, %.10f, %.10f, %.10f, %.10f",
		transform[2], // c (origin x)
		transform[0], // a (pixel width)
		transform[1], // b (rotation)
		transform[5], // f (origin y)
		transform[3], // d (rotation)
		transform[4], // e (pixel height)
	)

	vrtContent := fmt.Sprintf(`<VRTDataset rasterXSize="%d" rasterYSize="%d">
  <SRS dataAxisToSRSAxisMapping="2,1">%s</SRS>
  <GeoTransform>%s</GeoTransform>
  <VRTRasterBand dataType="Byte" band="1">
    <ColorInterp>Red</ColorInterp>
    <SimpleSource>
      <SourceFilename relativeToVRT="1">%s</SourceFilename>
      <SourceBand>1</SourceBand>
      <SourceProperties RasterXSize="%d" RasterYSize="%d" DataType="Byte" BlockXSize="%d" BlockYSize="1" />
      <SrcRect xOff="0" yOff="0" xSize="%d" ySize="%d" />
      <DstRect xOff="0" yOff="0" xSize="%d" ySize="%d" />
    </SimpleSource>
  </VRTRasterBand>
  <VRTRasterBand dataType="Byte" band="2">
    <ColorInterp>Green</ColorInterp>
    <SimpleSource>
      <SourceFilename relativeToVRT="1">%s</SourceFilename>
      <SourceBand>2</SourceBand>
      <SourceProperties RasterXSize="%d" RasterYSize="%d" DataType="Byte" BlockXSize="%d" BlockYSize="1" />
      <SrcRect xOff="0" yOff="0" xSize="%d" ySize="%d" />
      <DstRect xOff="0" yOff="0" xSize="%d" ySize="%d" />
    </SimpleSource>
  </VRTRasterBand>
  <VRTRasterBand dataType="Byte" band="3">
    <ColorInterp>Blue</ColorInterp>
    <SimpleSource>
      <SourceFilename relativeToVRT="1">%s</SourceFilename>
      <SourceBand>3</SourceBand>
      <SourceProperties RasterXSize="%d" RasterYSize="%d" DataType="Byte" BlockXSize="%d" BlockYSize="1" />
      <SrcRect xOff="0" yOff="0" xSize="%d" ySize="%d" />
      <DstRect xOff="0" yOff="0" xSize="%d" ySize="%d" />
    </SimpleSource>
  </VRTRasterBand>
</VRTDataset>
`, width, height, crs, geoTransformStr,
		relPath, width, height, width, width, height, width, height,
		relPath, width, height, width, width, height, width, height,
		relPath, width, height, width, width, height, width, height)

	return os.WriteFile(vrtPath, []byte(vrtContent), 0644)
}
