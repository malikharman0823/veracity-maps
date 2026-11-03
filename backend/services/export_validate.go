package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/malikharman0823/veracity-maps/gis"
)

// ValidationResult captures the telemetry of strict format validation.
type ValidationResult struct {
	Valid      bool   `json:"valid"`
	Format     string `json:"format"`
	FilePath   string `json:"file_path"`
	FailedStep string `json:"failed_step,omitempty"`
	Reason     string `json:"reason,omitempty"`
	DatasetID  string `json:"dataset_id,omitempty"`
	Details    string `json:"details,omitempty"`
}

// ValidateExport performs centralized strict geospatial format validation before any
// generated file is returned to the user or saved as a permanent export.
func ValidateExport(filePath, format, datasetID string) (*ValidationResult, error) {
	normFormat := strings.ToLower(strings.TrimSpace(format))
	if normFormat == "" {
		normFormat = "project_zip"
	}

	fi, err := os.Stat(filePath)
	if err != nil || fi.Size() == 0 {
		return &ValidationResult{
			Valid:      false,
			Format:     normFormat,
			FilePath:   filePath,
			FailedStep: "file_exists",
			Reason:     "Exported file does not exist or is empty (0 bytes)",
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'file_exists': Exported file is missing or 0 bytes (dataset: %s)", normFormat, datasetID)
	}

	switch normFormat {
	case "mbtiles":
		return validateMBTilesExport(filePath, datasetID)
	case "project_zip", "zip", "project", "complete_project", "mtmap", "base_layer", "basemap":
		return validateZipExport(filePath, normFormat, datasetID)
	case "geotiff", "tif", "tiff":
		return validateGeoTIFFExport(filePath, datasetID)
	case "png", "jpeg", "jpg":
		return validateRasterExport(filePath, normFormat, datasetID)
	default:
		// Unknown or generic format: verify file existence and non-empty
		return &ValidationResult{
			Valid:    true,
			Format:   normFormat,
			FilePath: filePath,
		}, nil
	}
}

func validateMBTilesExport(filePath, datasetID string) (*ValidationResult, error) {
	// 1. Check SQLite 3 magic header
	if !gis.HasSQLiteHeader(filePath) {
		return &ValidationResult{
			Valid:      false,
			Format:     "mbtiles",
			FilePath:   filePath,
			FailedStep: "sqlite_magic_header",
			Reason:     "File does not begin with standard 16-byte SQLite 3 header",
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [mbtiles] Failed at step 'sqlite_magic_header': Missing SQLite 3 header (dataset: %s)", datasetID)
	}

	// 2. Perform deep inspection of metadata, tables, tile records, and bounds
	insp, err := gis.InspectMBTiles(filePath, "", "")
	if err != nil {
		return &ValidationResult{
			Valid:      false,
			Format:     "mbtiles",
			FilePath:   filePath,
			FailedStep: "sqlite_inspection",
			Reason:     fmt.Sprintf("MBTiles database failed internal inspection: %v", err),
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [mbtiles] Failed at step 'sqlite_inspection': %w (dataset: %s)", err, datasetID)
	}

	if insp.TileCount <= 0 {
		return &ValidationResult{
			Valid:      false,
			Format:     "mbtiles",
			FilePath:   filePath,
			FailedStep: "tile_records_count",
			Reason:     "MBTiles database contains 0 tiles",
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [mbtiles] Failed at step 'tile_records_count': Database contains 0 tiles (dataset: %s)", datasetID)
	}

	if insp.MinZoom < 0 || insp.MaxZoom > 25 || insp.MinZoom > insp.MaxZoom {
		return &ValidationResult{
			Valid:      false,
			Format:     "mbtiles",
			FilePath:   filePath,
			FailedStep: "zoom_levels",
			Reason:     fmt.Sprintf("Invalid zoom levels [min=%d, max=%d]", insp.MinZoom, insp.MaxZoom),
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [mbtiles] Failed at step 'zoom_levels': Invalid range [%d..%d] (dataset: %s)", insp.MinZoom, insp.MaxZoom, datasetID)
	}

	if len(insp.Bounds) == 4 {
		if insp.Bounds[0] >= insp.Bounds[2] || insp.Bounds[1] >= insp.Bounds[3] {
			return &ValidationResult{
				Valid:      false,
				Format:     "mbtiles",
				FilePath:   filePath,
				FailedStep: "geographic_bounds",
				Reason:     fmt.Sprintf("Invalid bounds [%.4f, %.4f, %.4f, %.4f]", insp.Bounds[0], insp.Bounds[1], insp.Bounds[2], insp.Bounds[3]),
				DatasetID:  datasetID,
			}, fmt.Errorf("VALIDATION_FAILED: [mbtiles] Failed at step 'geographic_bounds': Inverted bounds (dataset: %s)", datasetID)
		}
	}

	return &ValidationResult{
		Valid:    true,
		Format:   "mbtiles",
		FilePath: filePath,
		Details:  fmt.Sprintf("Valid SQLite MBTiles 1.3: %d tiles, zooms %d-%d", insp.TileCount, insp.MinZoom, insp.MaxZoom),
	}, nil
}

func validateZipExport(filePath, format, datasetID string) (*ValidationResult, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return &ValidationResult{
			Valid:      false,
			Format:     format,
			FilePath:   filePath,
			FailedStep: "zip_open",
			Reason:     fmt.Sprintf("Cannot open ZIP archive: %v", err),
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'zip_open': %w (dataset: %s)", format, err, datasetID)
	}
	defer zr.Close()

	if len(zr.File) == 0 {
		return &ValidationResult{
			Valid:      false,
			Format:     format,
			FilePath:   filePath,
			FailedStep: "zip_empty",
			Reason:     "ZIP archive contains zero entries",
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'zip_empty': Archive contains 0 entries (dataset: %s)", format, datasetID)
	}

	hasManifest := false
	hasMBTiles := false
	entriesMap := make(map[string]*zip.File)

	for _, f := range zr.File {
		cleanSlash := filepath.ToSlash(f.Name)

		// 1. Check for ZIP-Slip path traversal vulnerability
		if strings.HasPrefix(cleanSlash, "/") || strings.Contains(cleanSlash, "../") || strings.Contains(cleanSlash, `..\`) {
			return &ValidationResult{
				Valid:      false,
				Format:     format,
				FilePath:   filePath,
				FailedStep: "zip_slip_check",
				Reason:     fmt.Sprintf("Unsafe path traversal entry detected: '%s'", f.Name),
				DatasetID:  datasetID,
			}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'zip_slip_check': Path traversal in '%s' (dataset: %s)", format, f.Name, datasetID)
		}

		entriesMap[cleanSlash] = f
		baseLower := strings.ToLower(filepath.Base(cleanSlash))
		if baseLower == "manifest.json" || baseLower == "project.json" || baseLower == "metadata.json" {
			hasManifest = true
		}
		if strings.HasSuffix(strings.ToLower(cleanSlash), ".mbtiles") {
			hasMBTiles = true
		}
	}

	if !hasManifest {
		return &ValidationResult{
			Valid:      false,
			Format:     format,
			FilePath:   filePath,
			FailedStep: "manifest_presence",
			Reason:     "ZIP archive lacks manifest.json, project.json, or metadata.json",
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'manifest_presence': Missing manifest metadata (dataset: %s)", format, datasetID)
	}

	// 2. Validate manifest JSON parses
	if mFile, ok := entriesMap["manifest.json"]; ok {
		rc, err := mFile.Open()
		if err == nil {
			var manDoc map[string]interface{}
			decErr := json.NewDecoder(rc).Decode(&manDoc)
			_ = rc.Close()
			if decErr != nil {
				return &ValidationResult{
					Valid:      false,
					Format:     format,
					FilePath:   filePath,
					FailedStep: "manifest_json_parse",
					Reason:     fmt.Sprintf("manifest.json contains invalid JSON: %v", decErr),
					DatasetID:  datasetID,
				}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'manifest_json_parse': %w (dataset: %s)", format, decErr, datasetID)
			}
		}
	}

	// 3. If MBTiles is packaged inside, verify its SQLite 3 header
	if hasMBTiles {
		for name, f := range entriesMap {
			if strings.HasSuffix(strings.ToLower(name), ".mbtiles") {
				rc, err := f.Open()
				if err != nil {
					continue
				}
				hdr := make([]byte, 16)
				_, _ = io.ReadFull(rc, hdr)
				_ = rc.Close()
				if string(hdr) != "SQLite format 3\x00" {
					return &ValidationResult{
						Valid:      false,
						Format:     format,
						FilePath:   filePath,
						FailedStep: "embedded_mbtiles_header",
						Reason:     fmt.Sprintf("Embedded MBTiles database '%s' does not begin with SQLite format 3 header", name),
						DatasetID:  datasetID,
					}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'embedded_mbtiles_header': '%s' corrupt (dataset: %s)", format, name, datasetID)
				}
			}
		}
	}

	return &ValidationResult{
		Valid:    true,
		Format:   format,
		FilePath: filePath,
		Details:  fmt.Sprintf("Valid ZIP project archive with %d entries", len(zr.File)),
	}, nil
}

func validateGeoTIFFExport(filePath, datasetID string) (*ValidationResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return &ValidationResult{
			Valid:      false,
			Format:     "geotiff",
			FilePath:   filePath,
			FailedStep: "file_open",
			Reason:     err.Error(),
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [geotiff] Failed at step 'file_open': %w (dataset: %s)", err, datasetID)
	}
	defer f.Close()

	// Check standard TIFF magic bytes (II*\x00 or MM\x00*)
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return &ValidationResult{
			Valid:      false,
			Format:     "geotiff",
			FilePath:   filePath,
			FailedStep: "tiff_header",
			Reason:     "File too short to contain TIFF header",
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [geotiff] Failed at step 'tiff_header': Unexpected EOF (dataset: %s)", datasetID)
	}

	isLittleEndian := hdr[0] == 'I' && hdr[1] == 'I' && hdr[2] == 42 && hdr[3] == 0
	isBigEndian := hdr[0] == 'M' && hdr[1] == 'M' && hdr[2] == 0 && hdr[3] == 42
	if !isLittleEndian && !isBigEndian {
		return &ValidationResult{
			Valid:      false,
			Format:     "geotiff",
			FilePath:   filePath,
			FailedStep: "tiff_magic_bytes",
			Reason:     fmt.Sprintf("File header %q does not match standard TIFF magic bytes", hdr),
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [geotiff] Failed at step 'tiff_magic_bytes': Invalid TIFF signature (dataset: %s)", datasetID)
	}

	meta, inspectErr := gis.InspectRasterFile(filePath)
	if inspectErr != nil || meta == nil || meta.Width <= 0 || meta.Height <= 0 {
		return &ValidationResult{
			Valid:      false,
			Format:     "geotiff",
			FilePath:   filePath,
			FailedStep: "geotiff_dimensions",
			Reason:     "Failed inspecting GeoTIFF raster dimensions",
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [geotiff] Failed at step 'geotiff_dimensions': Corrupt raster (dataset: %s)", datasetID)
	}

	return &ValidationResult{
		Valid:    true,
		Format:   "geotiff",
		FilePath: filePath,
		Details:  fmt.Sprintf("Valid GeoTIFF %dx%d px", meta.Width, meta.Height),
	}, nil
}

func validateRasterExport(filePath, format, datasetID string) (*ValidationResult, error) {
	// If packaged as a zip with companion world file:
	if strings.HasSuffix(strings.ToLower(filePath), ".zip") {
		zr, err := zip.OpenReader(filePath)
		if err != nil {
			return &ValidationResult{
				Valid:      false,
				Format:     format,
				FilePath:   filePath,
				FailedStep: "raster_zip_open",
				Reason:     err.Error(),
				DatasetID:  datasetID,
			}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'raster_zip_open': %w (dataset: %s)", format, err, datasetID)
		}
		defer zr.Close()

		hasImage := false
		hasWorldFile := false
		for _, f := range zr.File {
			ext := strings.ToLower(filepath.Ext(f.Name))
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
				rc, rErr := f.Open()
				if rErr == nil {
					cfg, _, decErr := image.DecodeConfig(rc)
					_ = rc.Close()
					if decErr == nil && cfg.Width > 0 && cfg.Height > 0 {
						hasImage = true
					}
				}
			}
			if ext == ".pgw" || ext == ".jgw" || ext == ".tfw" || ext == ".wld" {
				rc, rErr := f.Open()
				if rErr == nil {
					buf := new(bytes.Buffer)
					_, _ = io.Copy(buf, rc)
					_ = rc.Close()
					lines := strings.Fields(buf.String())
					if len(lines) >= 6 {
						hasWorldFile = true
					}
				}
			}
		}

		if !hasImage {
			return &ValidationResult{
				Valid:      false,
				Format:     format,
				FilePath:   filePath,
				FailedStep: "raster_image_decode",
				Reason:     "No valid decodable image found in export zip",
				DatasetID:  datasetID,
			}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'raster_image_decode': Image failed decoding (dataset: %s)", format, datasetID)
		}

		return &ValidationResult{
			Valid:    true,
			Format:   format,
			FilePath: filePath,
			Details:  fmt.Sprintf("Valid georeferenced raster package (image=%t, world_file=%t)", hasImage, hasWorldFile),
		}, nil
	}

	// Raw image file:
	f, err := os.Open(filePath)
	if err != nil {
		return &ValidationResult{
			Valid:      false,
			Format:     format,
			FilePath:   filePath,
			FailedStep: "raw_image_open",
			Reason:     err.Error(),
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'raw_image_open': %w (dataset: %s)", format, err, datasetID)
	}
	defer f.Close()

	cfg, _, decErr := image.DecodeConfig(f)
	if decErr != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return &ValidationResult{
			Valid:      false,
			Format:     format,
			FilePath:   filePath,
			FailedStep: "raw_image_decode",
			Reason:     fmt.Sprintf("Image decoding failed: %v", decErr),
			DatasetID:  datasetID,
		}, fmt.Errorf("VALIDATION_FAILED: [%s] Failed at step 'raw_image_decode': %w (dataset: %s)", format, decErr, datasetID)
	}

	return &ValidationResult{
		Valid:    true,
		Format:   format,
		FilePath: filePath,
		Details:  fmt.Sprintf("Valid image %dx%d px", cfg.Width, cfg.Height),
	}, nil
}
