package gis

import (
	"bufio"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type RasterMetadata struct {
	Driver              string             `json:"driver"`
	Width               int                `json:"width"`
	Height              int                `json:"height"`
	Bands               int                `json:"bands"`
	DType               string             `json:"dtype"`
	CRS                 *string            `json:"crs"`
	Bounds              map[string]float64 `json:"bounds"`
	Resolution          map[string]float64 `json:"resolution,omitempty"`
	Transform           []float64          `json:"transform,omitempty"`
	GeoreferenceSource  string             `json:"georeference_source,omitempty"`
	IsGeoreferenced     bool               `json:"is_georeferenced"`
	ColorInterpretation []string           `json:"color_interpretation,omitempty"`
}

type GDALTools struct {
	Available      bool   `json:"available"`
	GDALInfoPath   string `json:"gdalinfo_path,omitempty"`
	TranslatePath  string `json:"gdal_translate_path,omitempty"`
	WarpPath       string `json:"gdalwarp_path,omitempty"`
	VersionMessage string `json:"version_message,omitempty"`
}

func DetectGDAL() GDALTools {
	infoPath, errInfo := exec.LookPath("gdalinfo")
	translatePath, errTrans := exec.LookPath("gdal_translate")
	warpPath, errWarp := exec.LookPath("gdalwarp")

	if errInfo == nil && errTrans == nil && errWarp == nil {
		out, _ := exec.Command(infoPath, "--version").CombinedOutput()
		return GDALTools{
			Available:      true,
			GDALInfoPath:   infoPath,
			TranslatePath:  translatePath,
			WarpPath:       warpPath,
			VersionMessage: strings.TrimSpace(string(out)),
		}
	}
	return GDALTools{
		Available:      false,
		VersionMessage: "GDAL command line utilities (gdalinfo, gdal_translate, gdalwarp) were not found in PATH.",
	}
}

func decodeTiffDimensions(filePath string) (int, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	header := make([]byte, 8)
	if _, err := io.ReadFull(f, header); err != nil {
		return 0, 0, err
	}

	var isLE bool
	if header[0] == 'I' && header[1] == 'I' {
		isLE = true
	} else if header[0] == 'M' && header[1] == 'M' {
		isLE = false
	} else {
		return 0, 0, fmt.Errorf("not a tiff")
	}

	readUint16 := func(b []byte) uint16 {
		if isLE {
			return uint16(b[0]) | uint16(b[1])<<8
		}
		return uint16(b[1]) | uint16(b[0])<<8
	}

	readUint32 := func(b []byte) uint32 {
		if isLE {
			return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		}
		return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
	}

	if readUint16(header[2:4]) != 42 {
		return 0, 0, fmt.Errorf("invalid tiff magic")
	}

	ifdOffset := int64(readUint32(header[4:8]))
	if _, err := f.Seek(ifdOffset, io.SeekStart); err != nil {
		return 0, 0, err
	}

	countBuf := make([]byte, 2)
	if _, err := io.ReadFull(f, countBuf); err != nil {
		return 0, 0, err
	}
	numEntries := int(readUint16(countBuf))

	width := 0
	height := 0
	entryBuf := make([]byte, 12)
	for i := 0; i < numEntries; i++ {
		if _, err := io.ReadFull(f, entryBuf); err != nil {
			break
		}
		tag := readUint16(entryBuf[0:2])
		typ := readUint16(entryBuf[2:4])
		var val int
		if typ == 3 { // SHORT
			val = int(readUint16(entryBuf[8:10]))
		} else if typ == 4 { // LONG
			val = int(readUint32(entryBuf[8:12]))
		}
		if tag == 256 { // ImageWidth
			width = val
		} else if tag == 257 { // ImageLength
			height = val
		}
		if width > 0 && height > 0 {
			return width, height, nil
		}
	}

	if width > 0 && height > 0 {
		return width, height, nil
	}
	return 0, 0, fmt.Errorf("tiff dimensions not found")
}

// InspectRasterFile inspects a PNG, JPEG or TIFF raster
func InspectRasterFile(filePath string) (*RasterMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open raster file: %w", err)
	}
	defer file.Close()

	cfg, format, err := image.DecodeConfig(file)
	if err != nil {
		w, h, tiffErr := decodeTiffDimensions(filePath)
		if tiffErr == nil && w > 0 && h > 0 {
			cfg = image.Config{Width: w, Height: h}
			format = "tiff"
		} else {
			return nil, fmt.Errorf("unsupported or corrupted raster image: %w", err)
		}
	}

	driverName := strings.ToUpper(format)
	if driverName == "JPEG" {
		driverName = "JPG"
	}

	meta := &RasterMetadata{
		Driver:              driverName,
		Width:               cfg.Width,
		Height:              cfg.Height,
		Bands:               3,
		DType:               "uint8",
		ColorInterpretation: []string{"Red", "Green", "Blue"},
		IsGeoreferenced:     false,
	}

	// Check for sidecars (.pgw, .jgw, .wld, .prj)
	dir := filepath.Dir(filePath)
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	sidecarWorld := findWorldFile(dir, base, filepath.Ext(filePath))
	sidecarPrj := filepath.Join(dir, base+".prj")

	if sidecarWorld != "" {
		transform, err := ParseWorldFile(sidecarWorld)
		if err == nil && len(transform) == 6 {
			meta.Transform = transform
			meta.Resolution = map[string]float64{
				"x": math.Abs(transform[0]),
				"y": math.Abs(transform[4]),
			}
			meta.Bounds = CalculateBoundsFromTransform(transform, cfg.Width, cfg.Height)
			meta.GeoreferenceSource = "sidecar"
			crsStr := "EPSG:4326"
			if prjBytes, prjErr := os.ReadFile(sidecarPrj); prjErr == nil {
				prjContent := strings.ToUpper(string(prjBytes))
				if strings.Contains(prjContent, "3857") {
					crsStr = "EPSG:3857"
				} else if strings.Contains(prjContent, "4326") || strings.Contains(prjContent, "WGS") {
					crsStr = "EPSG:4326"
				}
			}
			meta.CRS = &crsStr
			meta.IsGeoreferenced = true
		}
	}

	return meta, nil
}

func findWorldFile(dir, base, ext string) string {
	ext = strings.ToLower(ext)
	candidates := []string{
		filepath.Join(dir, base+".wld"),
		filepath.Join(dir, base+".WLD"),
	}
	switch ext {
	case ".png":
		candidates = append(candidates, filepath.Join(dir, base+".pgw"), filepath.Join(dir, base+".PGW"))
	case ".jpg", ".jpeg":
		candidates = append(candidates, filepath.Join(dir, base+".jgw"), filepath.Join(dir, base+".JGW"))
	case ".tif", ".tiff":
		candidates = append(candidates, filepath.Join(dir, base+".tfw"), filepath.Join(dir, base+".TFW"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// ParseWorldFile parses a 6-line standard world file
func ParseWorldFile(path string) ([]float64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var values []float64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		val, err := strconv.ParseFloat(line, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid world file number '%s': %w", line, err)
		}
		values = append(values, val)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading world file: %w", err)
	}
	if len(values) < 6 {
		return nil, fmt.Errorf("world file must have at least 6 lines, found %d", len(values))
	}
	// World file: A, D, B, E, C, F
	// Transform array standard: a, b, c, d, e, f
	// A = a, D = d, B = b, E = e, C = c, F = f
	return []float64{
		values[0], // a (pixel width / x scale)
		values[2], // b (x rotation / skew x)
		values[4], // c (x origin)
		values[1], // d (y rotation / skew y)
		values[3], // e (pixel height / negative y scale)
		values[5], // f (y origin)
	}, nil
}

func CalculateBoundsFromTransform(transform []float64, width, height int) map[string]float64 {
	a, b, c := transform[0], transform[1], transform[2]
	d, e, f := transform[3], transform[4], transform[5]

	w := float64(width)
	h := float64(height)

	// Corners: (0,0), (w,0), (0,h), (w,h)
	cornersX := []float64{
		c,
		a*w + c,
		b*h + c,
		a*w + b*h + c,
	}
	cornersY := []float64{
		f,
		d*w + f,
		e*h + f,
		d*w + e*h + f,
	}

	minX := cornersX[0]
	maxX := cornersX[0]
	minY := cornersY[0]
	maxY := cornersY[0]

	for i := 1; i < 4; i++ {
		if cornersX[i] < minX {
			minX = cornersX[i]
		}
		if cornersX[i] > maxX {
			maxX = cornersX[i]
		}
		if cornersY[i] < minY {
			minY = cornersY[i]
		}
		if cornersY[i] > maxY {
			maxY = cornersY[i]
		}
	}

	return map[string]float64{
		"min_x": minX,
		"min_y": minY,
		"max_x": maxX,
		"max_y": maxY,
	}
}

// NormalizeBounds validates and returns [west, south, east, north] (MapLibre format)
func NormalizeBounds(bounds map[string]float64) ([]float64, error) {
	if bounds == nil {
		return nil, fmt.Errorf("bounds map is nil")
	}
	minX, okX1 := bounds["min_x"]
	minY, okY1 := bounds["min_y"]
	maxX, okX2 := bounds["max_x"]
	maxY, okY2 := bounds["max_y"]

	if !okX1 || !okY1 || !okX2 || !okY2 {
		return nil, fmt.Errorf("missing bounds coordinate keys")
	}
	if math.IsNaN(minX) || math.IsNaN(minY) || math.IsNaN(maxX) || math.IsNaN(maxY) {
		return nil, fmt.Errorf("bounds contain NaN")
	}
	if minX >= maxX || minY >= maxY {
		return nil, fmt.Errorf("inverted bounds: [%f, %f, %f, %f]", minX, minY, maxX, maxY)
	}
	return []float64{minX, minY, maxX, maxY}, nil
}
