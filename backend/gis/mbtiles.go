package gis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sqliteMagicHeader = "SQLite format 3\x00"

// MBTilesLayerInput defines one raster layer input for GDAL/rasterio MBTiles generation.
type MBTilesLayerInput struct {
	ImagePath      string    `json:"image_path"`
	Bounds         []float64 `json:"bounds"`                    // [west, south, east, north] in EPSG:4326
	Transform      []float64 `json:"transform,omitempty"`       // 6-element affine [a, b, c, d, e, f]
	OriginalWidth  int       `json:"original_width,omitempty"`  // original pixel width matching Transform
	OriginalHeight int       `json:"original_height,omitempty"` // original pixel height matching Transform
	Opacity        float64   `json:"opacity"`
	Resampling     string    `json:"resampling,omitempty"`     // "lanczos" | "cubic" | "bilinear" | "nearest"
	Classification string    `json:"classification,omitempty"` // "engineering_drawing" | "raster_imagery" | "categorical_raster"
}

// MBTilesCreateSpec defines parameters for generating an MBTiles 1.3 SQLite database.
type MBTilesCreateSpec struct {
	Name              string                 `json:"name"`
	Description       string                 `json:"description,omitempty"`
	Attribution       string                 `json:"attribution,omitempty"`
	Type              string                 `json:"type,omitempty"`        // "overlay" | "baselayer"
	TileFormat        string                 `json:"tile_format,omitempty"` // "png" | "jpg" | "webp"
	MinZoom           *int                   `json:"min_zoom,omitempty"`
	MaxZoom           *int                   `json:"max_zoom,omitempty"`
	InitialZoom       *int                   `json:"initial_zoom,omitempty"`
	AllowUpscaleZooms bool                   `json:"allow_upscale_zooms,omitempty"`
	MaxTotalTiles     int                    `json:"max_total_tiles,omitempty"`
	Layers            []MBTilesLayerInput    `json:"layers"`
	ExtraMetadata     map[string]interface{} `json:"extra_metadata,omitempty"`
}

// MBTilesCreateResult captures output telemetry from MBTiles generation.
type MBTilesCreateResult struct {
	Success              bool      `json:"success"`
	Code                 string    `json:"code,omitempty"`
	Error                string    `json:"error,omitempty"`
	MBTilesPath          string    `json:"mbtiles_path"`
	TileFormat           string    `json:"tile_format"`
	Scheme               string    `json:"scheme"`
	Bounds               []float64 `json:"bounds"`
	Center               []float64 `json:"center"`
	MinZoom              int       `json:"min_zoom"`
	MaxZoom              int       `json:"max_zoom"`
	NativeMaxZoom        int       `json:"native_max_zoom"`
	UpscaledBeyondNative bool      `json:"upscaled_beyond_native"`
	UpscaledZooms        []int     `json:"upscaled_zooms,omitempty"`
	TileCount            int       `json:"tile_count"`
	AvailableZooms       []int     `json:"available_zooms"`
	Resampling           string    `json:"resampling"`
}

// MBTilesInspection captures validation and metadata read from an existing .mbtiles file.
type MBTilesInspection struct {
	Success          bool                     `json:"success"`
	Code             string                   `json:"code,omitempty"`
	Error            string                   `json:"error,omitempty"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description"`
	Attribution      string                   `json:"attribution"`
	Type             string                   `json:"type"`
	TileFormat       string                   `json:"tile_format"` // "png" | "jpg" | "webp" | "pbf"
	IsGzippedPBF     bool                     `json:"is_gzipped_pbf"`
	Scheme           string                   `json:"scheme"` // "tms" | "xyz"
	Bounds           []float64                `json:"bounds"` // [west, south, east, north]
	Center           []float64                `json:"center"` // [lng, lat]
	InitialZoom      float64                  `json:"initial_zoom"`
	MinZoom          int                      `json:"min_zoom"`
	MaxZoom          int                      `json:"max_zoom"`
	DBMinZoom        int                      `json:"db_min_zoom"`
	DBMaxZoom        int                      `json:"db_max_zoom"`
	TileCount        int                      `json:"tile_count"`
	AvailableZooms   []int                    `json:"available_zooms"`
	VectorLayers     []map[string]interface{} `json:"vector_layers,omitempty"`
	RawMetadata      map[string]string        `json:"raw_metadata,omitempty"`
	PreviewGenerated bool                     `json:"preview_generated"`
	PreviewPath      string                   `json:"preview_path,omitempty"`
	PreviewWidth     int                      `json:"preview_width,omitempty"`
	PreviewHeight    int                      `json:"preview_height,omitempty"`
	UnpackedCache    bool                     `json:"unpacked_cache"`
}

// HasSQLiteHeader checks whether filePath begins with the 16-byte SQLite 3 magic header.
func HasSQLiteHeader(filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 16)
	if _, err := io.ReadFull(f, buf); err != nil {
		return false
	}
	return string(buf) == sqliteMagicHeader
}

// CreateMBTiles generates a genuine SQLite MBTiles 1.3 file from one or more georeferenced layers.
func CreateMBTiles(outputMBTilesPath string, spec MBTilesCreateSpec) (*MBTilesCreateResult, error) {
	if len(spec.Layers) == 0 {
		return nil, fmt.Errorf("NO_VALID_LAYERS: At least one georeferenced layer is required to generate MBTiles")
	}

	// Write spec JSON to a temporary sidecar file to avoid Windows CLI argument length limits
	specFile := outputMBTilesPath + ".spec.json"
	_ = os.MkdirAll(filepath.Dir(outputMBTilesPath), 0755)
	specBytes, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("MARSHAL_ERROR: %w", err)
	}
	if err := os.WriteFile(specFile, specBytes, 0644); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: %w", err)
	}
	defer os.Remove(specFile)

	var res MBTilesCreateResult
	if err := RunPythonJSON(180*time.Second, "mbtiles_tool.py", []string{"create", outputMBTilesPath, specFile}, &res); err != nil {
		return nil, err
	}
	if !res.Success {
		code := res.Code
		if code == "" {
			code = "TILE_GENERATION_FAILED"
		}
		return nil, fmt.Errorf("%s: %s", code, res.Error)
	}
	return &res, nil
}

// InspectMBTiles validates an .mbtiles SQLite database, reads metadata & scheme,
// optionally renders a preview PNG, and optionally unpacks small tile sets into unpackDir.
func InspectMBTiles(mbtilesPath, previewOutPath, unpackDir string) (*MBTilesInspection, error) {
	args := []string{"inspect", mbtilesPath}
	if previewOutPath != "" || unpackDir != "" {
		args = append(args, previewOutPath)
	}
	if unpackDir != "" {
		args = append(args, unpackDir)
	}

	var res MBTilesInspection
	if err := RunPythonJSON(60*time.Second, "mbtiles_tool.py", args, &res); err != nil {
		return nil, err
	}
	if !res.Success {
		code := res.Code
		if code == "" {
			code = "INVALID_MBTILES"
		}
		return nil, fmt.Errorf("%s: %s", code, res.Error)
	}
	return &res, nil
}

// OptimizeGeoTIFF generates a GDAL-optimized GeoTIFF with DEFLATE compression and multi-scale overviews.
func OptimizeGeoTIFF(inputPath, outputTifPath string, bounds []float64, transform []float64, resampling string) error {
	spec := map[string]interface{}{
		"bounds":     bounds,
		"transform":  transform,
		"resampling": resampling,
	}
	specBytes, _ := json.Marshal(spec)
	var res struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Error   string `json:"error"`
	}
	if err := RunPythonJSON(90*time.Second, "mbtiles_tool.py", []string{"optimize_tif", inputPath, outputTifPath, string(specBytes)}, &res); err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("%s: %s", res.Code, res.Error)
	}
	return nil
}

// Bounded in-memory tile cache for large external MBTiles (>2500 tiles)
type tileMemCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	order   []string
	maxSize int
}

var globalTileCache = &tileMemCache{
	entries: make(map[string][]byte),
	maxSize: 512,
}

func (c *tileMemCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.entries[key]
	return val, ok
}

func (c *tileMemCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists {
		if len(c.order) >= c.maxSize {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}
	c.entries[key] = data
}

// TileContentType resolves HTTP Content-Type and whether gzip Content-Encoding applies.
func TileContentType(tileFormat string, rawBytes []byte) (contentType string, isGzipped bool) {
	fmtLower := strings.ToLower(strings.TrimSpace(tileFormat))
	if len(rawBytes) >= 4 {
		if bytes.HasPrefix(rawBytes, []byte{0x89, 'P', 'N', 'G'}) {
			return "image/png", false
		}
		if bytes.HasPrefix(rawBytes, []byte{0xFF, 0xD8, 0xFF}) {
			return "image/jpeg", false
		}
		if bytes.HasPrefix(rawBytes, []byte("RIFF")) && len(rawBytes) >= 12 && string(rawBytes[8:12]) == "WEBP" {
			return "image/webp", false
		}
		if bytes.HasPrefix(rawBytes, []byte{0x1F, 0x8B}) {
			return "application/x-protobuf", true
		}
	}
	switch fmtLower {
	case "jpg", "jpeg":
		return "image/jpeg", false
	case "webp":
		return "image/webp", false
	case "pbf", "mvt":
		return "application/x-protobuf", len(rawBytes) >= 2 && rawBytes[0] == 0x1F && rawBytes[1] == 0x8B
	default:
		return "image/png", false
	}
}

// ReadMBTilesTile serves a single XYZ tile (z, x, y_xyz) from an unpacked XYZ cache directory
// if present (sub-millisecond, zero subprocess), falling back to querying mbtilesPath directly.
func ReadMBTilesTile(mbtilesPath, unpackDir, scheme, tileFormat string, z, x, yXYZ int) ([]byte, string, bool, error) {
	if z < 0 || z > 25 || x < 0 || yXYZ < 0 {
		return nil, "", false, os.ErrNotExist
	}

	// 1. Check unpacked XYZ tile directory first (< 0.2ms, zero SQLite locks)
	if unpackDir != "" {
		exts := []string{tileFormat, "png", "jpg", "webp", "pbf"}
		seen := make(map[string]bool)
		for _, ext := range exts {
			ext = strings.ToLower(strings.TrimPrefix(ext, "."))
			if ext == "" || seen[ext] {
				continue
			}
			seen[ext] = true
			candidate := filepath.Join(unpackDir, strconv.Itoa(z), strconv.Itoa(x), fmt.Sprintf("%d.%s", yXYZ, ext))
			if data, err := os.ReadFile(candidate); err == nil && len(data) > 0 {
				ct, gz := TileContentType(ext, data)
				return data, ct, gz, nil
			}
		}
	}

	if mbtilesPath == "" {
		return nil, "", false, os.ErrNotExist
	}
	if _, err := os.Stat(mbtilesPath); err != nil {
		return nil, "", false, os.ErrNotExist
	}

	if scheme == "" {
		scheme = "tms"
	}
	cacheKey := fmt.Sprintf("%s|%s|%d/%d/%d", mbtilesPath, scheme, z, x, yXYZ)
	if cached, ok := globalTileCache.get(cacheKey); ok {
		if len(cached) == 0 {
			return nil, "", false, os.ErrNotExist
		}
		ct, gz := TileContentType(tileFormat, cached)
		return cached, ct, gz, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pyExe := ResolvePythonInterpreter()
	script := ResolveScriptPath("mbtiles_tool.py")
	cmd := exec.CommandContext(ctx, pyExe, script, "get_tile", mbtilesPath, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(yXYZ), scheme)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		// Exit code 2 means tile not present in coverage
		globalTileCache.set(cacheKey, []byte{})
		return nil, "", false, os.ErrNotExist
	}

	data := stdout.Bytes()
	if len(data) == 0 {
		globalTileCache.set(cacheKey, []byte{})
		return nil, "", false, os.ErrNotExist
	}
	globalTileCache.set(cacheKey, data)
	ct, gz := TileContentType(tileFormat, data)
	return data, ct, gz, nil
}
