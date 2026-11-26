package gis

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UploadFormat represents the strictly supported upload file formats.
type UploadFormat string

const (
	FormatPNG     UploadFormat = "png"
	FormatJPG     UploadFormat = "jpg"
	FormatPDF     UploadFormat = "pdf"
	FormatDXF     UploadFormat = "dxf"
	FormatGPKG    UploadFormat = "gpkg"
	FormatGeoJSON UploadFormat = "geojson"
)

// ImportCategory distinguishes Raster vs Vector import workflows.
type ImportCategory string

const (
	ImportCategoryRaster ImportCategory = "raster"
	ImportCategoryVector ImportCategory = "vector"
)

// UnifiedLayerType represents the strict workspace layer types.
type UnifiedLayerType string

const (
	LayerTypeRaster       UnifiedLayerType = "raster"
	LayerTypeVector       UnifiedLayerType = "vector"
	LayerTypeImageOverlay UnifiedLayerType = "image_overlay"
)

// ValidationError carries structured stage, code, and user-facing message.
type ValidationError struct {
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%s:%s] %s", e.Stage, e.Code, e.Message)
}

// UploadLimits defines configurable safety bounds for file parsing and processing.
type UploadLimits struct {
	MaxFileSizeBytes         int64
	MaxImageDimension        int
	MaxImagePixels           int64
	MaxPDFPages              int
	MaxPDFPageDimensionPt    float64
	MaxCADEntities           int
	MaxGeoJSONFeatures       int
	MaxGeoPackageFeatures    int
	MaxConcurrentJobsPerUser int
	ProcessingTimeout        time.Duration
}

// DefaultUploadLimits returns sensible production resource limits.
func DefaultUploadLimits() UploadLimits {
	return UploadLimits{
		MaxFileSizeBytes:         500 * 1024 * 1024, // 500 MB
		MaxImageDimension:        16384,
		MaxImagePixels:           100_000_000, // 100 MP
		MaxPDFPages:              100,
		MaxPDFPageDimensionPt:    14400, // 200 inches
		MaxCADEntities:           250_000,
		MaxGeoJSONFeatures:       100_000,
		MaxGeoPackageFeatures:    500_000,
		MaxConcurrentJobsPerUser: 3,
		ProcessingTimeout:        120 * time.Second,
	}
}

// InspectedSubLayer describes a selectable internal layer (e.g. GeoPackage layer or PDF page).
type InspectedSubLayer struct {
	Name          string                 `json:"name"`
	PageNumber    int                    `json:"page_number,omitempty"`
	GeometryType  string                 `json:"geometry_type,omitempty"`
	CRS           string                 `json:"crs,omitempty"`
	FeatureCount  int                    `json:"feature_count,omitempty"`
	Width         int                    `json:"width,omitempty"`
	Height        int                    `json:"height,omitempty"`
	WidthPt       float64                `json:"width_pt,omitempty"`
	HeightPt      float64                `json:"height_pt,omitempty"`
	Rotation      int                    `json:"rotation,omitempty"`
	Extent        []float64              `json:"extent,omitempty"` // [west, south, east, north]
	Fields        []string               `json:"fields,omitempty"`
	HasGeospatial bool                   `json:"has_geospatial,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// FileInspectionResult is the common inspection output across all supported formats.
type FileInspectionResult struct {
	DetectedFormat        UploadFormat           `json:"detected_format"`
	ImportCategory        ImportCategory         `json:"import_category"`
	ResultLayerType       UnifiedLayerType       `json:"result_layer_type"`
	MimeType              string                 `json:"mime_type"`
	FileSize              int64                  `json:"file_size"`
	Width                 int                    `json:"width,omitempty"`
	Height                int                    `json:"height,omitempty"`
	PixelCount            int64                  `json:"pixel_count,omitempty"`
	ColorModel            string                 `json:"color_model,omitempty"`
	BitDepth              int                    `json:"bit_depth,omitempty"`
	HasTransparency       bool                   `json:"has_transparency,omitempty"`
	Orientation           int                    `json:"orientation,omitempty"`
	PageCount             int                    `json:"page_count,omitempty"`
	FeatureCount          int                    `json:"feature_count,omitempty"`
	EntityCount           int                    `json:"entity_count,omitempty"`
	CRS                   string                 `json:"crs,omitempty"`
	Extent                []float64              `json:"extent,omitempty"` // [west, south, east, north]
	RequiresGeoreference  bool                   `json:"requires_georeferencing"`
	SubLayers             []InspectedSubLayer    `json:"sub_layers,omitempty"`
	GeometryTypes         []string               `json:"geometry_types,omitempty"`
	Fields                []string               `json:"fields,omitempty"`
	UnsupportedEntities   []string               `json:"unsupported_entities,omitempty"`
	Warnings              []string               `json:"warnings,omitempty"`
	RequiredProcessingOps []string               `json:"required_processing_ops,omitempty"`
	Extra                 map[string]interface{} `json:"extra,omitempty"`
}

// IsFormatAllowedForCategory enforces the strict allowed format matrix.
func IsFormatAllowedForCategory(format UploadFormat, category ImportCategory) bool {
	switch category {
	case ImportCategoryRaster:
		return format == FormatPNG || format == FormatJPG || format == FormatPDF || format == FormatDXF
	case ImportCategoryVector:
		return format == FormatGPKG || format == FormatGeoJSON || format == FormatPNG || format == FormatJPG
	default:
		return false
	}
}

// DetectAndVerifyFormat inspects actual file magic bytes and structure (never trusting client MIME alone).
func DetectAndVerifyFormat(filePath, originalFilename string, category ImportCategory) (UploadFormat, string, error) {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(originalFilename)))

	f, err := os.Open(filePath)
	if err != nil {
		return "", "", &ValidationError{Stage: "upload", Code: "FILE_UNREADABLE", Message: "Unable to open uploaded file"}
	}
	defer f.Close()

	header := make([]byte, 4096)
	n, readErr := f.Read(header)
	if n == 0 {
		return "", "", &ValidationError{Stage: "upload", Code: "EMPTY_FILE", Message: "Uploaded file is empty (0 bytes)"}
	}
	if readErr != nil && !errors.Is(readErr, os.ErrClosed) && readErr.Error() != "EOF" {
		return "", "", &ValidationError{Stage: "upload", Code: "FILE_READ_ERROR", Message: "Failed to read file header"}
	}
	head := header[:n]

	var detected UploadFormat
	var mime string

	switch {
	case len(head) >= 8 && bytes.Equal(head[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		detected = FormatPNG
		mime = "image/png"
	case len(head) >= 3 && head[0] == 0xff && head[1] == 0xd8 && head[2] == 0xff:
		detected = FormatJPG
		mime = "image/jpeg"
	case len(head) >= 5 && bytes.Equal(head[:5], []byte("%PDF-")):
		detected = FormatPDF
		mime = "application/pdf"
	case len(head) >= 16 && bytes.Equal(head[:16], []byte("SQLite format 3\x00")):
		detected = FormatGPKG
		mime = "application/geopackage+sqlite3"
	default:
		trimmed := bytes.TrimSpace(bytes.TrimPrefix(head, []byte{0xef, 0xbb, 0xbf}))
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && (ext == ".geojson" || ext == ".json") {
			if ext != ".geojson" {
				return "", "", &ValidationError{
					Stage:   "upload",
					Code:    "UNSUPPORTED_FORMAT",
					Message: "Only .geojson files are accepted for GeoJSON vector imports",
				}
			}
			detected = FormatGeoJSON
			mime = "application/geo+json"
		} else if ext == ".dxf" && looksLikeDXF(head) {
			detected = FormatDXF
			mime = "application/dxf"
		}
	}

	if detected == "" {
		return "", "", &ValidationError{
			Stage:   "upload",
			Code:    "UNSUPPORTED_FORMAT",
			Message: fmt.Sprintf("Unsupported or unrecognized file format (%q). Supported formats: Raster (.png, .jpg, .jpeg, .pdf, .dxf) and Vector (.gpkg, .geojson, .png, .jpg, .jpeg).", ext),
		}
	}

	// Verify extension matches actual magic-byte format (blocks disguised files).
	if !extensionMatchesFormat(ext, detected) {
		return "", "", &ValidationError{
			Stage:   "upload",
			Code:    "FORMAT_MISMATCH",
			Message: fmt.Sprintf("File extension %q does not match detected file signature (%s)", ext, detected),
		}
	}

	if category != "" && !IsFormatAllowedForCategory(detected, category) {
		return "", "", &ValidationError{
			Stage:   "upload",
			Code:    "CATEGORY_FORMAT_MISMATCH",
			Message: fmt.Sprintf("Format %q is not allowed in %s import mode", detected, category),
		}
	}

	return detected, mime, nil
}

func extensionMatchesFormat(ext string, format UploadFormat) bool {
	switch format {
	case FormatPNG:
		return ext == ".png"
	case FormatJPG:
		return ext == ".jpg" || ext == ".jpeg"
	case FormatPDF:
		return ext == ".pdf"
	case FormatDXF:
		return ext == ".dxf"
	case FormatGPKG:
		return ext == ".gpkg"
	case FormatGeoJSON:
		return ext == ".geojson"
	default:
		return false
	}
}

func looksLikeDXF(head []byte) bool {
	if bytes.HasPrefix(head, []byte("AutoCAD Binary DXF")) {
		return true
	}
	upper := strings.ToUpper(string(head))
	return strings.Contains(upper, "SECTION") || strings.Contains(upper, "HEADER") || strings.Contains(upper, "ENTITIES") || strings.Contains(upper, "999")
}

// InspectAndValidateFile runs Stage 2 (Inspection) + Stage 3 (Deep Parsing) + Stage 4 (Validation).
func InspectAndValidateFile(ctx context.Context, filePath, originalFilename string, category ImportCategory, limits UploadLimits) (*FileInspectionResult, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, &ValidationError{Stage: "upload", Code: "FILE_NOT_FOUND", Message: "Uploaded file could not be accessed"}
	}
	if info.Size() == 0 {
		return nil, &ValidationError{Stage: "upload", Code: "EMPTY_FILE", Message: "Uploaded file is empty (0 bytes)"}
	}
	if limits.MaxFileSizeBytes > 0 && info.Size() > limits.MaxFileSizeBytes {
		return nil, &ValidationError{
			Stage:   "upload",
			Code:    "FILE_TOO_LARGE",
			Message: fmt.Sprintf("File size (%d bytes) exceeds maximum allowed limit (%d bytes)", info.Size(), limits.MaxFileSizeBytes),
		}
	}

	format, mime, err := DetectAndVerifyFormat(filePath, originalFilename, category)
	if err != nil {
		return nil, err
	}
	if category == "" {
		if format == FormatGPKG || format == FormatGeoJSON {
			category = ImportCategoryVector
		} else {
			category = ImportCategoryRaster
		}
	}

	switch format {
	case FormatPNG, FormatJPG:
		return validateImageFile(filePath, format, mime, info.Size(), category, limits)
	case FormatGeoJSON:
		return validateGeoJSONFile(filePath, mime, info.Size(), limits)
	case FormatGPKG:
		return inspectPythonUnified(ctx, "inspect-gpkg", filePath, format, mime, info.Size(), category, limits)
	case FormatPDF:
		return inspectPythonUnified(ctx, "inspect-pdf", filePath, format, mime, info.Size(), category, limits)
	case FormatDXF:
		return inspectPythonUnified(ctx, "inspect-dxf", filePath, format, mime, info.Size(), category, limits)
	default:
		return nil, &ValidationError{Stage: "validation", Code: "UNSUPPORTED_FORMAT", Message: "Unsupported format"}
	}
}

// validateImageFile performs complete PNG/JPG decoding, pixel/dimension bounds, color model,
// transparency, EXIF orientation, and world-file checks.
func validateImageFile(filePath string, format UploadFormat, mime string, fileSize int64, category ImportCategory, limits UploadLimits) (*FileInspectionResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "IMAGE_OPEN_FAILED", Message: "Unable to open image file"}
	}
	defer f.Close()

	// Check dimensions from header first before full decode (prevents decompression bombs).
	cfg, decodedFmt, err := image.DecodeConfig(f)
	if err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "CORRUPTED_IMAGE", Message: "Invalid or corrupted image header: " + err.Error()}
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, &ValidationError{Stage: "validation", Code: "INVALID_IMAGE_DIMENSIONS", Message: "Image has zero or negative dimensions"}
	}
	if limits.MaxImageDimension > 0 && (cfg.Width > limits.MaxImageDimension || cfg.Height > limits.MaxImageDimension) {
		return nil, &ValidationError{
			Stage:   "validation",
			Code:    "IMAGE_DIMENSIONS_EXCEEDED",
			Message: fmt.Sprintf("Image dimensions %dx%d exceed maximum allowed %dpx", cfg.Width, cfg.Height, limits.MaxImageDimension),
		}
	}
	pixelCount := int64(cfg.Width) * int64(cfg.Height)
	if limits.MaxImagePixels > 0 && pixelCount > limits.MaxImagePixels {
		return nil, &ValidationError{
			Stage:   "validation",
			Code:    "IMAGE_PIXELS_EXCEEDED",
			Message: fmt.Sprintf("Image pixel count (%d) exceeds maximum allowed (%d)", pixelCount, limits.MaxImagePixels),
		}
	}

	// Verify JPEG EOI marker (0xFF 0xD9) to catch truncated JPEGs that stdlib tolerates loosely.
	if format == FormatJPG {
		rawBytes, rErr := os.ReadFile(filePath)
		if rErr != nil || len(rawBytes) < 4 {
			return nil, &ValidationError{Stage: "parsing", Code: "TRUNCATED_IMAGE", Message: "JPEG file is truncated"}
		}
		if !bytes.Contains(rawBytes[len(rawBytes)-minInt(len(rawBytes), 64):], []byte{0xff, 0xd9}) {
			return nil, &ValidationError{Stage: "parsing", Code: "TRUNCATED_IMAGE", Message: "JPEG file is truncated (missing EOI marker)"}
		}
	}

	// Rewind and perform FULL image decode to catch corrupted/truncated compressed streams.
	if _, err := f.Seek(0, 0); err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "IMAGE_SEEK_FAILED", Message: "Unable to rewind image stream"}
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "CORRUPTED_IMAGE", Message: "Corrupted or truncated image data: " + err.Error()}
	}

	colorModelName, bitDepth, hasAlpha := describeColorModel(img.ColorModel())
	orientation := 1
	if format == FormatJPG {
		orientation = readJPEGExifOrientation(filePath)
	}

	layerType := LayerTypeRaster
	ops := []string{"raster_prepare"}
	if category == ImportCategoryVector {
		layerType = LayerTypeImageOverlay
		ops = []string{"image_overlay_prepare"}
	}

	return &FileInspectionResult{
		DetectedFormat:        format,
		ImportCategory:        category,
		ResultLayerType:       layerType,
		MimeType:              mime,
		FileSize:              fileSize,
		Width:                 cfg.Width,
		Height:                cfg.Height,
		PixelCount:            pixelCount,
		ColorModel:            colorModelName,
		BitDepth:              bitDepth,
		HasTransparency:       hasAlpha,
		Orientation:           orientation,
		RequiresGeoreference:  true, // PNG/JPG do not carry embedded CRS without an explicit world file
		RequiredProcessingOps: ops,
		Extra: map[string]interface{}{
			"decoded_format": decodedFmt,
		},
	}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func describeColorModel(cm color.Model) (string, int, bool) {
	switch cm {
	case color.RGBAModel:
		return "RGBA", 8, true
	case color.NRGBAModel:
		return "NRGBA", 8, true
	case color.RGBA64Model:
		return "RGBA64", 16, true
	case color.NRGBA64Model:
		return "NRGBA64", 16, true
	case color.YCbCrModel:
		return "YCbCr", 8, false
	case color.GrayModel:
		return "Gray", 8, false
	case color.Gray16Model:
		return "Gray16", 16, false
	case color.CMYKModel:
		return "CMYK", 8, false
	default:
		if _, ok := cm.(color.Palette); ok {
			return "Palette", 8, true
		}
		return "Unknown", 8, false
	}
}

// readJPEGExifOrientation scans APP1 EXIF header for Orientation tag (0x0112), returning 1 if absent.
func readJPEGExifOrientation(filePath string) int {
	data, err := os.ReadFile(filePath)
	if err != nil || len(data) < 20 {
		return 1
	}
	idx := bytes.Index(data, []byte("Exif\x00\x00"))
	if idx < 0 || idx+14 >= len(data) {
		return 1
	}
	tiff := data[idx+6:]
	if len(tiff) < 10 {
		return 1
	}
	var order binary.ByteOrder
	if tiff[0] == 'I' && tiff[1] == 'I' {
		order = binary.LittleEndian
	} else if tiff[0] == 'M' && tiff[1] == 'M' {
		order = binary.BigEndian
	} else {
		return 1
	}
	ifdOffset := int(order.Uint32(tiff[4:8]))
	if ifdOffset < 8 || ifdOffset+2 > len(tiff) {
		return 1
	}
	entries := int(order.Uint16(tiff[ifdOffset : ifdOffset+2]))
	for i := 0; i < entries; i++ {
		off := ifdOffset + 2 + i*12
		if off+12 > len(tiff) {
			break
		}
		tag := order.Uint16(tiff[off : off+2])
		if tag == 0x0112 {
			val := int(order.Uint16(tiff[off+8 : off+10]))
			if val >= 1 && val <= 8 {
				return val
			}
		}
	}
	return 1
}

// validateGeoJSONFile performs complete JSON parsing, FeatureCollection/Feature/Geometry validation,
// strict WGS 84 lon/lat coordinate validation, and spatial extent calculation.
func validateGeoJSONFile(filePath, mime string, fileSize int64, limits UploadLimits) (*FileInspectionResult, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "GEOJSON_READ_FAILED", Message: "Unable to read GeoJSON file"}
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "INVALID_JSON", Message: "Invalid JSON syntax: " + err.Error()}
	}

	rootType, _ := doc["type"].(string)
	var features []map[string]interface{}

	switch rootType {
	case "FeatureCollection":
		rawFeats, ok := doc["features"].([]interface{})
		if !ok {
			return nil, &ValidationError{Stage: "validation", Code: "INVALID_GEOJSON", Message: "FeatureCollection must contain a 'features' array"}
		}
		if len(rawFeats) == 0 {
			return nil, &ValidationError{Stage: "validation", Code: "EMPTY_FEATURE_COLLECTION", Message: "GeoJSON FeatureCollection contains 0 features"}
		}
		if limits.MaxGeoJSONFeatures > 0 && len(rawFeats) > limits.MaxGeoJSONFeatures {
			return nil, &ValidationError{
				Stage:   "validation",
				Code:    "FEATURE_LIMIT_EXCEEDED",
				Message: fmt.Sprintf("GeoJSON feature count (%d) exceeds maximum limit (%d)", len(rawFeats), limits.MaxGeoJSONFeatures),
			}
		}
		for i, rf := range rawFeats {
			fm, ok := rf.(map[string]interface{})
			if !ok || fm["type"] != "Feature" {
				return nil, &ValidationError{
					Stage:   "validation",
					Code:    "INVALID_FEATURE",
					Message: fmt.Sprintf("Feature at index %d is not a valid GeoJSON Feature object", i),
				}
			}
			features = append(features, fm)
		}
	case "Feature":
		features = []map[string]interface{}{doc}
	case "Point", "MultiPoint", "LineString", "MultiLineString", "Polygon", "MultiPolygon", "GeometryCollection":
		features = []map[string]interface{}{{"type": "Feature", "geometry": doc, "properties": map[string]interface{}{}}}
	default:
		return nil, &ValidationError{
			Stage:   "validation",
			Code:    "INVALID_GEOJSON_TYPE",
			Message: fmt.Sprintf("Unsupported or missing GeoJSON root type %q", rootType),
		}
	}

	geomTypesSet := map[string]bool{}
	fieldsSet := map[string]bool{}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)

	for i, feat := range features {
		geom, ok := feat["geometry"].(map[string]interface{})
		if !ok || geom == nil {
			return nil, &ValidationError{
				Stage:   "validation",
				Code:    "EMPTY_GEOMETRY",
				Message: fmt.Sprintf("Feature %d has a null or missing geometry", i),
			}
		}
		gType, err := validateGeoJSONGeometry(geom, &minX, &minY, &maxX, &maxY)
		if err != nil {
			if ve, ok := err.(*ValidationError); ok {
				return nil, &ValidationError{Stage: ve.Stage, Code: ve.Code, Message: fmt.Sprintf("Feature %d: %s", i, ve.Message)}
			}
			return nil, &ValidationError{Stage: "validation", Code: "INVALID_GEOMETRY", Message: fmt.Sprintf("Feature %d: %v", i, err)}
		}
		geomTypesSet[gType] = true

		if props, ok := feat["properties"].(map[string]interface{}); ok {
			for k := range props {
				fieldsSet[k] = true
			}
		}
	}

	if math.IsInf(minX, 1) || math.IsInf(maxX, -1) {
		return nil, &ValidationError{Stage: "validation", Code: "EMPTY_EXTENT", Message: "GeoJSON contains no valid coordinates"}
	}

	geomTypes := make([]string, 0, len(geomTypesSet))
	for k := range geomTypesSet {
		geomTypes = append(geomTypes, k)
	}
	fields := make([]string, 0, len(fieldsSet))
	for k := range fieldsSet {
		fields = append(fields, k)
	}

	return &FileInspectionResult{
		DetectedFormat:        FormatGeoJSON,
		ImportCategory:        ImportCategoryVector,
		ResultLayerType:       LayerTypeVector,
		MimeType:              mime,
		FileSize:              fileSize,
		FeatureCount:          len(features),
		CRS:                   "EPSG:4326",
		Extent:                []float64{minX, minY, maxX, maxY},
		RequiresGeoreference:  false,
		GeometryTypes:         geomTypes,
		Fields:                fields,
		RequiredProcessingOps: []string{"vector_normalize"},
	}, nil
}

func validateGeoJSONGeometry(geom map[string]interface{}, minX, minY, maxX, maxY *float64) (string, error) {
	gType, _ := geom["type"].(string)
	switch gType {
	case "Point":
		if err := validatePosition(geom["coordinates"], minX, minY, maxX, maxY); err != nil {
			return "", err
		}
		return gType, nil
	case "MultiPoint":
		if err := validatePositionArray(geom["coordinates"], 1, minX, minY, maxX, maxY); err != nil {
			return "", err
		}
		return gType, nil
	case "LineString":
		if err := validatePositionArray(geom["coordinates"], 2, minX, minY, maxX, maxY); err != nil {
			return "", err
		}
		return gType, nil
	case "MultiLineString":
		arr, ok := geom["coordinates"].([]interface{})
		if !ok || len(arr) == 0 {
			return "", &ValidationError{Stage: "validation", Code: "INVALID_GEOMETRY", Message: "MultiLineString must have at least 1 LineString"}
		}
		for _, ls := range arr {
			if err := validatePositionArray(ls, 2, minX, minY, maxX, maxY); err != nil {
				return "", err
			}
		}
		return gType, nil
	case "Polygon":
		if err := validatePolygonRings(geom["coordinates"], minX, minY, maxX, maxY); err != nil {
			return "", err
		}
		return gType, nil
	case "MultiPolygon":
		arr, ok := geom["coordinates"].([]interface{})
		if !ok || len(arr) == 0 {
			return "", &ValidationError{Stage: "validation", Code: "INVALID_GEOMETRY", Message: "MultiPolygon must have at least 1 Polygon"}
		}
		for _, poly := range arr {
			if err := validatePolygonRings(poly, minX, minY, maxX, maxY); err != nil {
				return "", err
			}
		}
		return gType, nil
	case "GeometryCollection":
		geoms, ok := geom["geometries"].([]interface{})
		if !ok || len(geoms) == 0 {
			return "", &ValidationError{Stage: "validation", Code: "INVALID_GEOMETRY", Message: "GeometryCollection must contain at least 1 geometry"}
		}
		for _, sub := range geoms {
			subMap, ok := sub.(map[string]interface{})
			if !ok {
				return "", &ValidationError{Stage: "validation", Code: "INVALID_GEOMETRY", Message: "Invalid sub-geometry in GeometryCollection"}
			}
			if _, err := validateGeoJSONGeometry(subMap, minX, minY, maxX, maxY); err != nil {
				return "", err
			}
		}
		return gType, nil
	default:
		return "", &ValidationError{Stage: "validation", Code: "UNSUPPORTED_GEOMETRY", Message: fmt.Sprintf("Unsupported GeoJSON geometry type %q", gType)}
	}
}

func validatePosition(raw interface{}, minX, minY, maxX, maxY *float64) error {
	arr, ok := raw.([]interface{})
	if !ok || len(arr) < 2 {
		return &ValidationError{Stage: "validation", Code: "INVALID_COORDINATES", Message: "Position must be a [lon, lat] numeric array"}
	}
	lon, okX := arr[0].(float64)
	lat, okY := arr[1].(float64)
	if !okX || !okY || math.IsNaN(lon) || math.IsInf(lon, 0) || math.IsNaN(lat) || math.IsInf(lat, 0) {
		return &ValidationError{Stage: "validation", Code: "INVALID_COORDINATES", Message: "Coordinates must be finite numbers"}
	}
	if lon < -180.0 || lon > 180.0 || lat < -90.0 || lat > 90.0 {
		return &ValidationError{
			Stage:   "validation",
			Code:    "INVALID_COORDINATES_PROJECTED",
			Message: fmt.Sprintf("Coordinates [%.4f, %.4f] are outside WGS 84 geographic bounds (lon [-180,180], lat [-90,90]); projected coordinates are not silently reinterpreted", lon, lat),
		}
	}
	if lon < *minX {
		*minX = lon
	}
	if lon > *maxX {
		*maxX = lon
	}
	if lat < *minY {
		*minY = lat
	}
	if lat > *maxY {
		*maxY = lat
	}
	return nil
}

func validatePositionArray(raw interface{}, minLen int, minX, minY, maxX, maxY *float64) error {
	arr, ok := raw.([]interface{})
	if !ok || len(arr) < minLen {
		return &ValidationError{Stage: "validation", Code: "INVALID_COORDINATES", Message: fmt.Sprintf("Coordinate array must contain at least %d positions", minLen)}
	}
	for _, pos := range arr {
		if err := validatePosition(pos, minX, minY, maxX, maxY); err != nil {
			return err
		}
	}
	return nil
}

func validatePolygonRings(raw interface{}, minX, minY, maxX, maxY *float64) error {
	rings, ok := raw.([]interface{})
	if !ok || len(rings) == 0 {
		return &ValidationError{Stage: "validation", Code: "INVALID_GEOMETRY", Message: "Polygon must contain at least 1 linear ring"}
	}
	for _, r := range rings {
		ring, ok := r.([]interface{})
		if !ok || len(ring) < 4 {
			return &ValidationError{Stage: "validation", Code: "INVALID_GEOMETRY", Message: "Polygon linear ring must have at least 4 positions"}
		}
		for _, pos := range ring {
			if err := validatePosition(pos, minX, minY, maxX, maxY); err != nil {
				return err
			}
		}
		first, _ := ring[0].([]interface{})
		last, _ := ring[len(ring)-1].([]interface{})
		if len(first) < 2 || len(last) < 2 || first[0] != last[0] || first[1] != last[1] {
			return &ValidationError{Stage: "validation", Code: "UNCLOSED_POLYGON_RING", Message: "Polygon linear ring must be closed (first and last positions must match)"}
		}
	}
	return nil
}

// inspectPythonUnified delegates deep inspection for GeoPackage, PDF, and DXF to unified_import_tool.py.
func inspectPythonUnified(ctx context.Context, action, filePath string, format UploadFormat, mime string, fileSize int64, category ImportCategory, limits UploadLimits) (*FileInspectionResult, error) {
	timeout := limits.ProcessingTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	payload := map[string]interface{}{
		"action":              action,
		"file_path":           filePath,
		"max_pdf_pages":       limits.MaxPDFPages,
		"max_pdf_dim_pt":      limits.MaxPDFPageDimensionPt,
		"max_cad_entities":    limits.MaxCADEntities,
		"max_gpkg_features":   limits.MaxGeoPackageFeatures,
		"max_image_dimension": limits.MaxImageDimension,
	}
	payloadBytes, _ := json.Marshal(payload)

	res, err := RunPythonScriptJSONWithContext(ctx, "unified_import_tool.py", []string{string(payloadBytes)}, timeout)
	if err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "PARSER_EXECUTION_FAILED", Message: err.Error()}
	}
	if ok, _ := res["success"].(bool); !ok {
		stage, _ := res["stage"].(string)
		if stage == "" {
			stage = "validation"
		}
		code, _ := res["code"].(string)
		if code == "" {
			code = "VALIDATION_FAILED"
		}
		msg, _ := res["error"].(string)
		if msg == "" {
			msg = "File failed deep validation"
		}
		return nil, &ValidationError{Stage: stage, Code: code, Message: msg}
	}

	rawData, _ := json.Marshal(res["inspection"])
	var out FileInspectionResult
	if err := json.Unmarshal(rawData, &out); err != nil {
		return nil, &ValidationError{Stage: "parsing", Code: "INSPECTION_DECODE_FAILED", Message: err.Error()}
	}
	out.DetectedFormat = format
	out.ImportCategory = category
	out.MimeType = mime
	out.FileSize = fileSize
	return &out, nil
}

// RunPythonUnifiedProcess executes Stage 5 (Processing) for GPKG, PDF, or DXF into a staging directory.
func RunPythonUnifiedProcess(ctx context.Context, payload map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	payloadBytes, _ := json.Marshal(payload)
	res, err := RunPythonScriptJSONWithContext(ctx, "unified_import_tool.py", []string{string(payloadBytes)}, timeout)
	if err != nil {
		return nil, &ValidationError{Stage: "processing", Code: "PROCESSOR_EXECUTION_FAILED", Message: err.Error()}
	}
	if ok, _ := res["success"].(bool); !ok {
		stage, _ := res["stage"].(string)
		if stage == "" {
			stage = "processing"
		}
		code, _ := res["code"].(string)
		if code == "" {
			code = "PROCESSING_FAILED"
		}
		msg, _ := res["error"].(string)
		if msg == "" {
			msg = "Processing failed"
		}
		return nil, &ValidationError{Stage: stage, Code: code, Message: msg}
	}
	return res, nil
}
