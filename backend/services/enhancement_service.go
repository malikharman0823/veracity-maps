package services

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

func resolveDatasetDirectory(sm *storage.StorageManager, dataset *models.Dataset) string {
	if dataset == nil {
		return ""
	}
	if sm != nil && dataset.SessionID != "" && dataset.ID != "" {
		dir := sm.GetDatasetDir(dataset.SessionID, dataset.ID)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	if dataset.StoragePath != "" {
		return filepath.Dir(dataset.StoragePath)
	}
	return ""
}

func extractAffineTransform(meta map[string]interface{}) ([]float64, bool) {
	if meta == nil {
		return nil, false
	}
	raw := meta["transform"]
	if raw == nil {
		return nil, false
	}
	if arr, ok := raw.([]float64); ok && len(arr) == 6 {
		return arr, true
	}
	if arr, ok := raw.([]interface{}); ok && len(arr) == 6 {
		out := make([]float64, 6)
		for i, v := range arr {
			out[i] = getDim(v)
		}
		return out, true
	}
	return nil, false
}

// ProjectPixelToWGS84 projects an original-raster pixel coordinate (px, py)
// to geographic WGS84 (longitude, latitude) using the standard 6-element affine transform.
func ProjectPixelToWGS84(transform []float64, px, py float64) (float64, float64) {
	lng := transform[0]*px + transform[1]*py + transform[2]
	lat := transform[3]*px + transform[4]*py + transform[5]
	return lng, lat
}

func recommendedResamplingForDataset(dataset *models.Dataset) string {
	if dataset == nil || dataset.ExtractedMetadata == nil {
		return "cubic"
	}
	if q, ok := dataset.ExtractedMetadata["quality"].(map[string]interface{}); ok {
		if r, ok := q["recommended_resampling"].(string); ok && r != "" {
			return r
		}
	}
	if qStruct, ok := dataset.ExtractedMetadata["quality"].(*models.QualityMetrics); ok && qStruct != nil && qStruct.RecommendedResampling != "" {
		return qStruct.RecommendedResampling
	}
	docType, _ := dataset.ExtractedMetadata["document_type"].(string)
	if docType == "pdf" || docType == "cad" {
		return "lanczos"
	}
	if c, ok := dataset.ExtractedMetadata["content"].(map[string]interface{}); ok {
		cls, _ := c["classification"].(string)
		switch cls {
		case "categorical_raster":
			return "nearest"
		case "engineering_drawing", "scanned_document", "vector_pdf", "hybrid_pdf", "cad_vector":
			return "lanczos"
		}
	}
	return "cubic"
}

// ProjectDatasetVectorsAndAnnotations projects stored image-space annotations and PDF/CAD vector geometry
// into EPSG:4326 GeoJSON overlays whenever a dataset has a valid 6-parameter affine transform.
func ProjectDatasetVectorsAndAnnotations(sm *storage.StorageManager, dataset *models.Dataset) {
	if dataset == nil {
		return
	}
	datasetDir := resolveDatasetDirectory(sm, dataset)
	if datasetDir == "" {
		return
	}
	vectorDir := filepath.Join(datasetDir, "vector")
	transform, hasTransform := extractAffineTransform(dataset.ExtractedMetadata)
	isGeoref := dataset.GeoreferenceStatus == "completed" && hasTransform

	// 1. Project annotations.json -> annotations.geojson
	annPath := filepath.Join(vectorDir, "annotations.json")
	if fileExists(annPath) {
		if raw, err := os.ReadFile(annPath); err == nil {
			var anns []*models.DatasetAnnotation
			if json.Unmarshal(raw, &anns) == nil {
				var features []map[string]interface{}
				for _, a := range anns {
					if a == nil {
						continue
					}
					if isGeoref {
						lng, lat := ProjectPixelToWGS84(transform, a.ImageX, a.ImageY)
						if !math.IsNaN(lng) && !math.IsNaN(lat) && lng >= -180 && lng <= 180 && lat >= -90 && lat <= 90 {
							lngRound := math.Round(lng*1e7) / 1e7
							latRound := math.Round(lat*1e7) / 1e7
							a.Longitude = &lngRound
							a.Latitude = &latRound
							a.HasGeo = true
						}
					}
					if a.HasGeo && a.Longitude != nil && a.Latitude != nil {
						textColor := "#0f172a"
						haloColor := "#ffffff"
						textSize := 12.0
						if a.FontSize > 0 {
							textSize = math.Max(10, math.Min(18, a.FontSize))
						}
						if a.Style != nil {
							if a.Style.TextColor != "" {
								textColor = a.Style.TextColor
							}
							if a.Style.HaloColor != "" {
								haloColor = a.Style.HaloColor
							}
							if a.Style.TextSize > 0 {
								textSize = a.Style.TextSize
							}
						}
						features = append(features, map[string]interface{}{
							"type": "Feature",
							"geometry": map[string]interface{}{
								"type":        "Point",
								"coordinates": []float64{*a.Longitude, *a.Latitude},
							},
							"properties": map[string]interface{}{
								"id":            a.ID,
								"dataset_id":    dataset.ID,
								"text":          a.Text,
								"original_text": a.OriginalText,
								"category":      a.Category,
								"source":        a.Source,
								"confidence":    a.Confidence,
								"rotation":      a.Rotation,
								"verified":      a.Verified,
								"visible":       a.Visible,
								"text_color":    textColor,
								"halo_color":    haloColor,
								"text_size":     textSize,
								"cad_layer":     a.CADLayer,
							},
						})
					}
				}
				if updatedBytes, err := json.MarshalIndent(anns, "", "  "); err == nil {
					_ = os.MkdirAll(vectorDir, 0755)
					_ = os.WriteFile(annPath, updatedBytes, 0644)
				}
				if isGeoref {
					if features == nil {
						features = []map[string]interface{}{}
					}
					fc := map[string]interface{}{
						"type":     "FeatureCollection",
						"features": features,
					}
					if gjBytes, err := json.MarshalIndent(fc, "", "  "); err == nil {
						_ = os.WriteFile(filepath.Join(vectorDir, "annotations.geojson"), gjBytes, 0644)
					}
				}
			}
		}
	}

	// 2. Project source_vectors.json -> vectors.geojson
	srcVecPath := filepath.Join(vectorDir, "source_vectors.json")
	if isGeoref && fileExists(srcVecPath) {
		if raw, err := os.ReadFile(srcVecPath); err == nil {
			var doc struct {
				Features []struct {
					GeometryType string      `json:"geometry_type"`
					CoordsPx     [][]float64 `json:"coords_px"`
					StrokeColor  string      `json:"stroke_color"`
					FillColor    *string     `json:"fill_color"`
					LineWidth    float64     `json:"line_width"`
					Layer        string      `json:"layer"`
				} `json:"features"`
			}
			if json.Unmarshal(raw, &doc) == nil {
				features := make([]map[string]interface{}, 0, len(doc.Features))
				for idx, f := range doc.Features {
					if len(f.CoordsPx) < 2 {
						continue
					}
					geoCoords := make([][]float64, 0, len(f.CoordsPx))
					valid := true
					for _, pt := range f.CoordsPx {
						if len(pt) < 2 {
							valid = false
							break
						}
						lng, lat := ProjectPixelToWGS84(transform, pt[0], pt[1])
						if math.IsNaN(lng) || math.IsNaN(lat) {
							valid = false
							break
						}
						geoCoords = append(geoCoords, []float64{
							math.Round(lng*1e7) / 1e7,
							math.Round(lat*1e7) / 1e7,
						})
					}
					if !valid || len(geoCoords) < 2 {
						continue
					}

					stroke := f.StrokeColor
					if stroke == "" {
						stroke = "#334155"
					}
					lw := f.LineWidth
					if lw <= 0 {
						lw = 1.25
					}

					var geom map[string]interface{}
					if f.GeometryType == "Polygon" && len(geoCoords) >= 4 {
						first := geoCoords[0]
						last := geoCoords[len(geoCoords)-1]
						if first[0] != last[0] || first[1] != last[1] {
							geoCoords = append(geoCoords, []float64{first[0], first[1]})
						}
						geom = map[string]interface{}{
							"type":        "Polygon",
							"coordinates": [][][]float64{geoCoords},
						}
					} else {
						geom = map[string]interface{}{
							"type":        "LineString",
							"coordinates": geoCoords,
						}
					}

					props := map[string]interface{}{
						"id":           fmt.Sprintf("vec_%s_%d", dataset.ID, idx),
						"dataset_id":   dataset.ID,
						"stroke_color": stroke,
						"line_width":   lw,
						"layer":        f.Layer,
					}
					if f.FillColor != nil && *f.FillColor != "" {
						props["fill_color"] = *f.FillColor
					}

					features = append(features, map[string]interface{}{
						"type":       "Feature",
						"geometry":   geom,
						"properties": props,
					})
				}

				fc := map[string]interface{}{
					"type":     "FeatureCollection",
					"features": features,
				}
				if gjBytes, err := json.Marshal(fc); err == nil {
					_ = os.MkdirAll(vectorDir, 0755)
					_ = os.WriteFile(filepath.Join(vectorDir, "vectors.geojson"), gjBytes, 0644)
					if dataset.ExtractedMetadata != nil {
						dataset.ExtractedMetadata["has_geo_vectors"] = len(features) > 0
					}
				}
			}
		}
	}
}

// SaveExtractedVectorsPayload persists raw PDF/CAD extracted vector paths and positioned text annotations
// into the dataset's vector/ subfolder without altering original raster files.
func (s *DatasetService) SaveExtractedVectorsPayload(sessionID string, dataset *models.Dataset, rawVectorJSONPath string) error {
	if dataset == nil || rawVectorJSONPath == "" || !fileExists(rawVectorJSONPath) {
		return nil
	}
	rawBytes, err := os.ReadFile(rawVectorJSONPath)
	if err != nil {
		return err
	}

	var payload struct {
		Features    []interface{} `json:"features"`
		Annotations []struct {
			Text         string    `json:"text"`
			OriginalText string    `json:"original_text"`
			Category     string    `json:"category"`
			Source       string    `json:"source"`
			Confidence   float64   `json:"confidence"`
			CADLayer     string    `json:"cad_layer"`
			ImageX       float64   `json:"image_x"`
			ImageY       float64   `json:"image_y"`
			BBoxPx       []float64 `json:"bbox_px"`
			Rotation     float64   `json:"rotation"`
			FontName     string    `json:"font_name"`
			FontSize     float64   `json:"font_size"`
			TextColor    string    `json:"text_color"`
			Verified     bool      `json:"verified"`
			Visible      bool      `json:"visible"`
		} `json:"annotations"`
	}
	if err := json.Unmarshal(rawBytes, &payload); err != nil {
		return err
	}

	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	vectorDir := filepath.Join(datasetDir, "vector")
	if err := os.MkdirAll(vectorDir, 0755); err != nil {
		return err
	}

	// Store clean copy at vector/source_vectors.json
	targetSrcVec := filepath.Join(vectorDir, "source_vectors.json")
	if filepath.Clean(rawVectorJSONPath) != filepath.Clean(targetSrcVec) {
		_ = os.WriteFile(targetSrcVec, rawBytes, 0644)
	}

	now := time.Now()
	anns := make([]*models.DatasetAnnotation, 0, len(payload.Annotations))
	for _, item := range payload.Annotations {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		var style *models.AnnotationStyle
		if item.TextColor != "" || item.FontSize > 0 {
			style = &models.AnnotationStyle{
				TextColor: item.TextColor,
				HaloColor: "#ffffff",
				TextSize:  math.Max(10, math.Min(18, item.FontSize)),
			}
		}
		anns = append(anns, &models.DatasetAnnotation{
			ID:           "ann_" + repository.GenerateDatasetID(),
			DatasetID:    dataset.ID,
			Text:         item.Text,
			OriginalText: item.OriginalText,
			Category:     item.Category,
			Source:       item.Source,
			Confidence:   item.Confidence,
			CADLayer:     item.CADLayer,
			ImageX:       item.ImageX,
			ImageY:       item.ImageY,
			BBoxPx:       item.BBoxPx,
			Rotation:     item.Rotation,
			FontName:     item.FontName,
			FontSize:     item.FontSize,
			Verified:     item.Verified,
			Visible:      item.Visible,
			Style:        style,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}

	if len(anns) > 0 {
		if annBytes, err := json.MarshalIndent(anns, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(vectorDir, "annotations.json"), annBytes, 0644)
		}
	}

	if dataset.ExtractedMetadata == nil {
		dataset.ExtractedMetadata = make(map[string]interface{})
	}
	dataset.ExtractedMetadata["has_vectors"] = len(payload.Features) > 0
	dataset.ExtractedMetadata["vector_feature_count"] = len(payload.Features)
	dataset.ExtractedMetadata["has_annotations"] = len(anns) > 0
	dataset.ExtractedMetadata["annotation_count"] = len(anns)

	ProjectDatasetVectorsAndAnnotations(s.storage, dataset)
	_ = s.repo.UpdateDataset(sessionID, dataset)
	return nil
}

// ResolveRenderableRasterPath resolves the browser- and tile-renderable image file for a dataset
// according to the requested variant ("original", "enhanced", or "" for active), without ever altering StoragePath.
func (s *DatasetService) ResolveRenderableRasterPath(dataset *models.Dataset, variant string) string {
	if dataset == nil {
		return ""
	}
	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	processedDir := filepath.Join(datasetDir, "processed")

	enhancedPNG := filepath.Join(processedDir, "enhanced.png")
	transparentPNG := filepath.Join(processedDir, "processed_transparent.png")
	displayPNG := filepath.Join(processedDir, "display.png")
	mbtilesPreviewPNG := filepath.Join(processedDir, "mbtiles_preview.png")

	storageExt := strings.ToLower(filepath.Ext(dataset.StoragePath))
	isBrowserNativeStorage := storageExt == ".png" || storageExt == ".jpg" || storageExt == ".jpeg" || storageExt == ".webp"

	if variant == "original" {
		if isBrowserNativeStorage && fileExists(dataset.StoragePath) {
			return dataset.StoragePath
		}
		if (storageExt == ".tif" || storageExt == ".tiff") && fileExists(dataset.StoragePath) {
			if !fileExists(displayPNG) {
				_ = gis.ConvertRasterToDisplayPNG(dataset.StoragePath, displayPNG)
			}
			if fileExists(displayPNG) {
				return displayPNG
			}
		}
		if fileExists(mbtilesPreviewPNG) {
			return mbtilesPreviewPNG
		}
		return dataset.StoragePath
	}

	if variant == "enhanced" {
		if fileExists(enhancedPNG) {
			return enhancedPNG
		}
	}

	// Default ("active"): respect background removal and active enhancement variant
	useTransparent := false
	if dataset.ExtractedMetadata != nil {
		if bg, ok := dataset.ExtractedMetadata["background_removal"].(map[string]interface{}); ok {
			if enabled, _ := bg["enabled"].(bool); enabled {
				useTransparent = true
			}
		}
	}

	useEnhanced := false
	if dataset.ExtractedMetadata != nil {
		if enhMap, ok := dataset.ExtractedMetadata["enhancement"].(map[string]interface{}); ok {
			enabled, _ := enhMap["enabled"].(bool)
			activeVar, _ := enhMap["active_variant"].(string)
			if enabled && activeVar != "original" {
				useEnhanced = true
			}
		} else if enhState, ok := dataset.ExtractedMetadata["enhancement"].(*models.EnhancementState); ok && enhState != nil {
			if enhState.Enabled && enhState.ActiveVariant != "original" {
				useEnhanced = true
			}
		}
	}

	if useTransparent && fileExists(transparentPNG) {
		return transparentPNG
	}
	if useEnhanced && fileExists(enhancedPNG) {
		return enhancedPNG
	}
	if !isBrowserNativeStorage {
		if fileExists(displayPNG) {
			return displayPNG
		}
		if (storageExt == ".tif" || storageExt == ".tiff") && fileExists(dataset.StoragePath) {
			if err := gis.ConvertRasterToDisplayPNG(dataset.StoragePath, displayPNG); err == nil && fileExists(displayPNG) {
				return displayPNG
			}
		}
		if fileExists(mbtilesPreviewPNG) {
			return mbtilesPreviewPNG
		}
	}
	if dataset.ProcessedPath != "" && fileExists(dataset.ProcessedPath) {
		pExt := strings.ToLower(filepath.Ext(dataset.ProcessedPath))
		if pExt == ".png" || pExt == ".jpg" || pExt == ".jpeg" || pExt == ".webp" {
			return dataset.ProcessedPath
		}
	}
	return dataset.StoragePath
}

// GetUniversalInspection returns the complete universal file inspection summary for a dataset.
func (s *DatasetService) GetUniversalInspection(sessionID, datasetID string) (map[string]interface{}, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return nil, repository.ErrDatasetNotFound
	}
	if dataset.ExtractedMetadata == nil {
		dataset.ExtractedMetadata = make(map[string]interface{})
	}

	// Lazily enrich quality/content metrics if missing on raster datasets
	storageExt := strings.ToLower(filepath.Ext(dataset.StoragePath))
	if dataset.ExtractedMetadata["quality"] == nil && storageExt != ".mbtiles" && fileExists(dataset.StoragePath) {
		if meta, inspErr := gis.InspectRasterFile(dataset.StoragePath); inspErr == nil && meta != nil {
			if meta.Content != nil && dataset.ExtractedMetadata["content"] == nil {
				dataset.ExtractedMetadata["content"] = meta.Content
			}
			if meta.Quality != nil {
				dataset.ExtractedMetadata["quality"] = meta.Quality
			}
			if meta.Recommendation != nil {
				dataset.ExtractedMetadata["recommendation"] = meta.Recommendation
			}
			if meta.ColorDepthBits > 0 {
				dataset.ExtractedMetadata["color_depth_bits"] = meta.ColorDepthBits
			}
			if meta.Compression != "" {
				dataset.ExtractedMetadata["compression"] = meta.Compression
			}
			if len(meta.DPI) > 0 {
				dataset.ExtractedMetadata["dpi"] = meta.DPI
				dataset.ExtractedMetadata["dpi_x"] = meta.DPI["x"]
				dataset.ExtractedMetadata["dpi_y"] = meta.DPI["y"]
			}
			_ = s.repo.UpdateDataset(sessionID, dataset)
		}
	}

	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	hasEnhancedFile := fileExists(filepath.Join(datasetDir, "processed", "enhanced.png"))
	anns, _ := s.ListAnnotations(sessionID, datasetID)

	return map[string]interface{}{
		"dataset_id":           dataset.ID,
		"filename":             dataset.Filename,
		"file_size":            dataset.FileSize,
		"content_type":         dataset.ContentType,
		"georeference_status":  dataset.GeoreferenceStatus,
		"is_georeferenced":     dataset.GeoreferenceStatus == "completed",
		"tile_status":          dataset.TileStatus,
		"width":                dataset.ExtractedMetadata["width"],
		"height":               dataset.ExtractedMetadata["height"],
		"bands":                dataset.ExtractedMetadata["bands"],
		"dtype":                dataset.ExtractedMetadata["dtype"],
		"color_depth_bits":     dataset.ExtractedMetadata["color_depth_bits"],
		"compression":          dataset.ExtractedMetadata["compression"],
		"dpi_x":                dataset.ExtractedMetadata["dpi_x"],
		"dpi_y":                dataset.ExtractedMetadata["dpi_y"],
		"rendering_dpi":        dataset.ExtractedMetadata["rendering_dpi"],
		"crs":                  dataset.ExtractedMetadata["crs"],
		"native_crs":           dataset.ExtractedMetadata["native_crs"],
		"bounds":               dataset.ExtractedMetadata["bounds"],
		"georeference_source":  dataset.ExtractedMetadata["georeference_source"],
		"document_type":        dataset.ExtractedMetadata["document_type"],
		"content":              dataset.ExtractedMetadata["content"],
		"quality":              dataset.ExtractedMetadata["quality"],
		"recommendation":       dataset.ExtractedMetadata["recommendation"],
		"enhancement":          dataset.ExtractedMetadata["enhancement"],
		"has_enhanced_image":   hasEnhancedFile,
		"has_vectors":          dataset.ExtractedMetadata["has_vectors"],
		"has_geo_vectors":      dataset.ExtractedMetadata["has_geo_vectors"],
		"vector_feature_count": dataset.ExtractedMetadata["vector_feature_count"],
		"has_annotations":      len(anns) > 0,
		"annotation_count":     len(anns),
		"recommended_resample": recommendedResamplingForDataset(dataset),
		"cad_units":            dataset.ExtractedMetadata["cad_units"],
		"cad_layers":           dataset.ExtractedMetadata["cad_layers"],
		"cad_extents":          dataset.ExtractedMetadata["cad_extents"],
		"pdf_page_number":      dataset.ExtractedMetadata["pdf_page_number"],
		"pdf_processing_path":  dataset.ExtractedMetadata["pdf_processing_path"],
	}, nil
}

// EnhanceDataset executes non-destructive OpenCV enhancement and optional Real-ESRGAN 2x/4x upscaling,
// saving outputs strictly under processed/enhanced.png while leaving dataset.StoragePath untouched.
func (s *DatasetService) EnhanceDataset(sessionID, datasetID string, opts models.EnhancementOptions) (*models.EnhancementState, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return nil, repository.ErrDatasetNotFound
	}
	if dataset.ExtractedMetadata == nil {
		dataset.ExtractedMetadata = make(map[string]interface{})
	}

	storageExt := strings.ToLower(filepath.Ext(dataset.StoragePath))
	if storageExt == ".mbtiles" {
		return nil, fmt.Errorf("UNSUPPORTED_TARGET: Standalone MBTiles tile archives cannot be re-enhanced as raw images")
	}

	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	processedDir := filepath.Join(datasetDir, "processed")
	if err := os.MkdirAll(processedDir, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: %w", err)
	}
	enhancedPath := filepath.Join(processedDir, "enhanced.png")

	action := strings.ToLower(strings.TrimSpace(opts.Action))
	if action == "" {
		action = "apply"
	}

	// Load existing state if present
	var currentState models.EnhancementState
	if rawEnh, ok := dataset.ExtractedMetadata["enhancement"]; ok && rawEnh != nil {
		if b, mErr := json.Marshal(rawEnh); mErr == nil {
			_ = json.Unmarshal(b, &currentState)
		}
	}

	if action == "reset" {
		currentState.Enabled = false
		currentState.ActiveVariant = "original"
		currentState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		dataset.ExtractedMetadata["enhancement"] = &currentState
		_ = s.repo.UpdateDataset(sessionID, dataset)
		return &currentState, nil
	}

	if action == "set_variant" {
		targetVar := strings.ToLower(strings.TrimSpace(opts.ActiveVariant))
		if targetVar != "original" && targetVar != "enhanced" {
			return nil, fmt.Errorf("INVALID_VARIANT: active_variant must be 'original' or 'enhanced'")
		}
		if targetVar == "enhanced" && !fileExists(enhancedPath) {
			return nil, fmt.Errorf("ENHANCED_NOT_FOUND: No enhanced output exists yet for this dataset")
		}
		currentState.Enabled = (targetVar == "enhanced")
		currentState.ActiveVariant = targetVar
		currentState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		dataset.ExtractedMetadata["enhancement"] = &currentState
		_ = s.repo.UpdateDataset(sessionID, dataset)
		return &currentState, nil
	}

	inputPath := dataset.StoragePath
	transparentPath := filepath.Join(processedDir, "processed_transparent.png")
	if dataset.ExtractedMetadata != nil {
		if bg, ok := dataset.ExtractedMetadata["background_removal"].(map[string]interface{}); ok {
			if enabled, _ := bg["enabled"].(bool); enabled && fileExists(transparentPath) {
				inputPath = transparentPath
			}
		}
	}
	if !fileExists(inputPath) {
		inputPath = dataset.StoragePath
	}
	if !fileExists(inputPath) {
		return nil, fmt.Errorf("SOURCE_MISSING: Original dataset file not found on disk")
	}

	if opts.Preset == "" {
		opts.Preset = "balanced"
	}
	if opts.UpscaleFactor != 2 && opts.UpscaleFactor != 4 {
		opts.UpscaleFactor = 1
	}

	optsBytes, err := json.Marshal(opts)
	if err != nil {
		return nil, fmt.Errorf("INVALID_OPTIONS: %w", err)
	}

	var pyRes struct {
		Success               bool     `json:"success"`
		Error                 string   `json:"error"`
		Code                  string   `json:"code"`
		Preset                string   `json:"preset"`
		OriginalWidth         int      `json:"original_width"`
		OriginalHeight        int      `json:"original_height"`
		EnhancedWidth         int      `json:"enhanced_width"`
		EnhancedHeight        int      `json:"enhanced_height"`
		ScaleRatio            float64  `json:"scale_ratio"`
		AIUpscaled            bool     `json:"ai_upscaled"`
		AIUpscaleFactor       int      `json:"ai_upscale_factor"`
		AIUpscaleEngine       string   `json:"ai_upscale_engine"`
		AIVerifiedEngineering bool     `json:"ai_verified_engineering"`
		AIDisclaimer          string   `json:"ai_disclaimer"`
		SkippedHighResUpscale bool     `json:"skipped_high_res_upscale"`
		BlurScoreBefore       float64  `json:"blur_score_before"`
		BlurScoreAfter        float64  `json:"blur_score_after"`
		OperationsApplied     []string `json:"operations_applied"`
	}

	timeout := 90 * time.Second
	if opts.UpscaleFactor > 1 {
		timeout = 240 * time.Second
	}

	if err := gis.RunPythonJSON(timeout, "raster_tool.py", []string{"enhance", inputPath, enhancedPath, string(optsBytes)}, &pyRes); err != nil {
		return nil, fmt.Errorf("ENHANCEMENT_FAILED: %w", err)
	}
	if !pyRes.Success || !fileExists(enhancedPath) {
		return nil, fmt.Errorf("%s: %s", pyRes.Code, pyRes.Error)
	}

	state := &models.EnhancementState{
		Enabled:               true,
		ActiveVariant:         "enhanced",
		Preset:                pyRes.Preset,
		EnhancedPath:          enhancedPath,
		OriginalWidth:         pyRes.OriginalWidth,
		OriginalHeight:        pyRes.OriginalHeight,
		EnhancedWidth:         pyRes.EnhancedWidth,
		EnhancedHeight:        pyRes.EnhancedHeight,
		ScaleRatio:            pyRes.ScaleRatio,
		AIUpscaled:            pyRes.AIUpscaled,
		AIUpscaleFactor:       pyRes.AIUpscaleFactor,
		AIUpscaleEngine:       pyRes.AIUpscaleEngine,
		AIVerifiedEngineering: false, // Never treat AI-upscaled details as verified engineering data
		AIDisclaimer:          pyRes.AIDisclaimer,
		SkippedHighResUpscale: pyRes.SkippedHighResUpscale,
		BlurScoreBefore:       pyRes.BlurScoreBefore,
		BlurScoreAfter:        pyRes.BlurScoreAfter,
		OperationsApplied:     pyRes.OperationsApplied,
		Options:               opts,
		UpdatedAt:             time.Now().UTC().Format(time.RFC3339),
	}

	dataset.ExtractedMetadata["enhancement"] = state
	dataset.UpdatedAt = time.Now()

	// If background removal was enabled, re-apply transparency onto the newly generated enhanced image
	// so transparent background is retained alongside AI upscaling and enhancement.
	if dataset.ExtractedMetadata != nil {
		if bgMap, ok := dataset.ExtractedMetadata["background_removal"].(map[string]interface{}); ok {
			if enabled, _ := bgMap["enabled"].(bool); enabled {
				colorHex, _ := bgMap["color"].(string)
				thresh, _ := bgMap["threshold"].(float64)
				softness, _ := bgMap["edge_softness"].(float64)
				if colorHex == "" {
					colorHex = "#ffffff"
				}
				if thresh <= 0 {
					thresh = 40.0
				}
				if softness <= 0 {
					softness = 15.0
				}
				if targetClr, cErr := gis.ParseHexColor(colorHex); cErr == nil {
					transparentPNG := filepath.Join(processedDir, "processed_transparent.png")
					if bgErr := gis.ProcessRasterBackgroundRemoval(enhancedPath, transparentPNG, targetClr, thresh, softness); bgErr == nil {
						dataset.ProcessedPath = transparentPNG
						bgMap["updated_at"] = time.Now().UTC().Format(time.RFC3339)
						dataset.ExtractedMetadata["background_removal"] = bgMap
					}
				}
			}
		}
	}

	ProjectDatasetVectorsAndAnnotations(s.storage, dataset)
	_ = s.repo.UpdateDataset(sessionID, dataset)

	if opts.RunOCR {
		_, _ = s.RunDatasetOCR(sessionID, datasetID, 0.45)
	}

	// If dataset already had generated MBTiles tiles (not a standalone uploaded .mbtiles), refresh them
	if dataset.GeoreferenceStatus == "completed" && dataset.TilePath != "" && dataset.TilePath != dataset.StoragePath {
		_, _, _ = s.GenerateDatasetMBTiles(sessionID, datasetID, opts.Resampling)
	}

	return state, nil
}

// RunDatasetOCR executes PaddleOCR (PP-OCR ONNX) on the dataset's enhanced or original raster,
// storing bounding boxes, confidence scores, and projected WGS84 coordinates as vector label overlays.
func (s *DatasetService) RunDatasetOCR(sessionID, datasetID string, minConfidence float64) ([]*models.DatasetAnnotation, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return nil, repository.ErrDatasetNotFound
	}
	if minConfidence <= 0 || minConfidence >= 1.0 {
		minConfidence = 0.45
	}

	targetImg := s.ResolveRenderableRasterPath(dataset, "")
	if targetImg == "" || !fileExists(targetImg) {
		return nil, fmt.Errorf("SOURCE_MISSING: Renderable image not found for OCR")
	}

	origW := int(getDim(dataset.ExtractedMetadata["width"]))
	origH := int(getDim(dataset.ExtractedMetadata["height"]))

	var pyRes struct {
		Success     bool   `json:"success"`
		Error       string `json:"error"`
		Code        string `json:"code"`
		Engine      string `json:"engine"`
		Annotations []struct {
			Text         string    `json:"text"`
			OriginalText string    `json:"original_text"`
			Category     string    `json:"category"`
			Source       string    `json:"source"`
			Confidence   float64   `json:"confidence"`
			ImageX       float64   `json:"image_x"`
			ImageY       float64   `json:"image_y"`
			BBoxPx       []float64 `json:"bbox_px"`
			Rotation     float64   `json:"rotation"`
			FontSize     float64   `json:"font_size"`
		} `json:"annotations"`
	}

	err = gis.RunPythonJSON(
		120*time.Second,
		"ocr_tool.py",
		[]string{
			"extract",
			targetImg,
			strconv.Itoa(origW),
			strconv.Itoa(origH),
			fmt.Sprintf("%.2f", minConfidence),
		},
		&pyRes,
	)
	if err != nil {
		return nil, fmt.Errorf("OCR_FAILED: %w", err)
	}
	if !pyRes.Success {
		return nil, fmt.Errorf("%s: %s", pyRes.Code, pyRes.Error)
	}

	// Preserve existing verified, manual, or PDF/CAD vector annotations (idempotent re-OCR)
	existing, _ := s.ListAnnotations(sessionID, datasetID)
	merged := make([]*models.DatasetAnnotation, 0, len(existing)+len(pyRes.Annotations))
	for _, ex := range existing {
		if ex == nil {
			continue
		}
		if ex.Source != "paddleocr" || ex.Verified {
			merged = append(merged, ex)
		}
	}

	now := time.Now()
	for _, det := range pyRes.Annotations {
		merged = append(merged, &models.DatasetAnnotation{
			ID:           "ann_" + repository.GenerateDatasetID(),
			DatasetID:    datasetID,
			Text:         det.Text,
			OriginalText: det.OriginalText,
			Category:     det.Category,
			Source:       "paddleocr",
			Confidence:   det.Confidence,
			ImageX:       det.ImageX,
			ImageY:       det.ImageY,
			BBoxPx:       det.BBoxPx,
			Rotation:     det.Rotation,
			FontSize:     det.FontSize,
			Verified:     false,
			Visible:      true,
			Style: &models.AnnotationStyle{
				TextColor: "#0f172a",
				HaloColor: "#ffffff",
				TextSize:  math.Max(11, math.Min(16, det.FontSize)),
			},
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	vectorDir := filepath.Join(datasetDir, "vector")
	_ = os.MkdirAll(vectorDir, 0755)
	if data, mErr := json.MarshalIndent(merged, "", "  "); mErr == nil {
		_ = os.WriteFile(filepath.Join(vectorDir, "annotations.json"), data, 0644)
	}

	if dataset.ExtractedMetadata == nil {
		dataset.ExtractedMetadata = make(map[string]interface{})
	}
	dataset.ExtractedMetadata["has_annotations"] = len(merged) > 0
	dataset.ExtractedMetadata["annotation_count"] = len(merged)
	dataset.ExtractedMetadata["ocr_engine"] = pyRes.Engine

	ProjectDatasetVectorsAndAnnotations(s.storage, dataset)
	_ = s.repo.UpdateDataset(sessionID, dataset)

	return s.ListAnnotations(sessionID, datasetID)
}

// ListAnnotations returns all OCR, PDF/CAD vector, and user-edited label annotations for a dataset.
func (s *DatasetService) ListAnnotations(sessionID, datasetID string) ([]*models.DatasetAnnotation, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return nil, repository.ErrDatasetNotFound
	}
	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	annPath := filepath.Join(datasetDir, "vector", "annotations.json")
	if !fileExists(annPath) {
		return []*models.DatasetAnnotation{}, nil
	}
	raw, err := os.ReadFile(annPath)
	if err != nil {
		return []*models.DatasetAnnotation{}, nil
	}
	var anns []*models.DatasetAnnotation
	if err := json.Unmarshal(raw, &anns); err != nil {
		return []*models.DatasetAnnotation{}, nil
	}
	if anns == nil {
		anns = []*models.DatasetAnnotation{}
	}
	return anns, nil
}

// UpsertAnnotation updates an existing OCR/vector annotation (text, visibility, verified flag, style)
// or creates a new manual annotation without touching original raster files.
func (s *DatasetService) UpsertAnnotation(sessionID, datasetID, annotationID string, patch map[string]interface{}) (*models.DatasetAnnotation, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return nil, repository.ErrDatasetNotFound
	}
	anns, _ := s.ListAnnotations(sessionID, datasetID)

	var target *models.DatasetAnnotation
	for _, a := range anns {
		if a != nil && a.ID == annotationID {
			target = a
			break
		}
	}

	now := time.Now()
	if target == nil {
		if annotationID == "" {
			annotationID = "ann_" + repository.GenerateDatasetID()
		}
		target = &models.DatasetAnnotation{
			ID:         annotationID,
			DatasetID:  datasetID,
			Source:     "manual",
			Category:   "general",
			Confidence: 1.0,
			Verified:   true,
			Visible:    true,
			CreatedAt:  now,
		}
		anns = append(anns, target)
	}

	if v, ok := patch["text"].(string); ok {
		target.Text = strings.TrimSpace(v)
	}
	if v, ok := patch["category"].(string); ok && v != "" {
		target.Category = v
	}
	if v, ok := patch["visible"].(bool); ok {
		target.Visible = v
	}
	if v, ok := patch["verified"].(bool); ok {
		target.Verified = v
	}
	if _, ok := patch["image_x"]; ok {
		target.ImageX = getDim(patch["image_x"])
	}
	if _, ok := patch["image_y"]; ok {
		target.ImageY = getDim(patch["image_y"])
	}
	if _, ok := patch["rotation"]; ok {
		target.Rotation = getDim(patch["rotation"])
	}
	if _, ok := patch["longitude"]; ok && patch["longitude"] != nil {
		lng := getDim(patch["longitude"])
		target.Longitude = &lng
		target.HasGeo = true
	}
	if _, ok := patch["latitude"]; ok && patch["latitude"] != nil {
		lat := getDim(patch["latitude"])
		target.Latitude = &lat
		target.HasGeo = true
	}
	if styleRaw, ok := patch["style"].(map[string]interface{}); ok {
		if target.Style == nil {
			target.Style = &models.AnnotationStyle{}
		}
		if tc, ok := styleRaw["text_color"].(string); ok {
			target.Style.TextColor = tc
		}
		if hc, ok := styleRaw["halo_color"].(string); ok {
			target.Style.HaloColor = hc
		}
		if ts := getDim(styleRaw["text_size"]); ts > 0 {
			target.Style.TextSize = ts
		}
	}
	target.UpdatedAt = now

	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	vectorDir := filepath.Join(datasetDir, "vector")
	_ = os.MkdirAll(vectorDir, 0755)
	if data, err := json.MarshalIndent(anns, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(vectorDir, "annotations.json"), data, 0644)
	}
	if dataset.ExtractedMetadata == nil {
		dataset.ExtractedMetadata = make(map[string]interface{})
	}
	dataset.ExtractedMetadata["has_annotations"] = len(anns) > 0
	dataset.ExtractedMetadata["annotation_count"] = len(anns)
	ProjectDatasetVectorsAndAnnotations(s.storage, dataset)
	_ = s.repo.UpdateDataset(sessionID, dataset)

	return target, nil
}

// DeleteAnnotation removes a single annotation from a dataset's vector label overlay.
func (s *DatasetService) DeleteAnnotation(sessionID, datasetID, annotationID string) error {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return repository.ErrDatasetNotFound
	}
	anns, _ := s.ListAnnotations(sessionID, datasetID)
	kept := make([]*models.DatasetAnnotation, 0, len(anns))
	found := false
	for _, a := range anns {
		if a != nil && a.ID == annotationID {
			found = true
			continue
		}
		kept = append(kept, a)
	}
	if !found {
		return fmt.Errorf("NOT_FOUND: Annotation %s not found", annotationID)
	}

	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	vectorDir := filepath.Join(datasetDir, "vector")
	_ = os.MkdirAll(vectorDir, 0755)
	if data, err := json.MarshalIndent(kept, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(vectorDir, "annotations.json"), data, 0644)
	}
	if dataset.ExtractedMetadata != nil {
		dataset.ExtractedMetadata["has_annotations"] = len(kept) > 0
		dataset.ExtractedMetadata["annotation_count"] = len(kept)
	}
	ProjectDatasetVectorsAndAnnotations(s.storage, dataset)
	_ = s.repo.UpdateDataset(sessionID, dataset)
	return nil
}

// GetDatasetVectorsGeoJSON returns the projected EPSG:4326 GeoJSON FeatureCollection for PDF/CAD vector layers.
func (s *DatasetService) GetDatasetVectorsGeoJSON(sessionID, datasetID string) (map[string]interface{}, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return nil, repository.ErrDatasetNotFound
	}
	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	gjPath := filepath.Join(datasetDir, "vector", "vectors.geojson")
	if !fileExists(gjPath) && dataset.GeoreferenceStatus == "completed" {
		ProjectDatasetVectorsAndAnnotations(s.storage, dataset)
	}
	if fileExists(gjPath) {
		if raw, err := os.ReadFile(gjPath); err == nil {
			var fc map[string]interface{}
			if json.Unmarshal(raw, &fc) == nil && fc != nil {
				return fc, nil
			}
		}
	}
	return map[string]interface{}{
		"type":     "FeatureCollection",
		"features": []interface{}{},
	}, nil
}

// BuildMBTilesLayerSpec constructs a gis.MBTilesLayerInput for a georeferenced dataset,
// scaling the affine transform automatically if the active enhanced image was AI-upscaled (2x/4x).
func (s *DatasetService) BuildMBTilesLayerSpec(dataset *models.Dataset, opacity float64, resampling string) (*gis.MBTilesLayerInput, error) {
	if dataset == nil {
		return nil, fmt.Errorf("nil dataset")
	}
	if dataset.GeoreferenceStatus != "completed" {
		return nil, fmt.Errorf("NOT_GEOREFERENCED: Dataset %s is not georeferenced", dataset.Filename)
	}

	imgPath := s.ResolveRenderableRasterPath(dataset, "")
	if imgPath == "" || !fileExists(imgPath) {
		return nil, fmt.Errorf("SOURCE_MISSING: Renderable image missing for %s", dataset.Filename)
	}

	_, bounds := parseBoundsMap(dataset.ExtractedMetadata["bounds"])
	if len(bounds) != 4 && dataset.MapLayer != nil {
		_, bounds = parseBoundsMap(dataset.MapLayer["bounds"])
	}
	if len(bounds) != 4 {
		return nil, fmt.Errorf("MISSING_BOUNDS: Valid WGS84 bounds missing for %s", dataset.Filename)
	}

	origW := int(getDim(dataset.ExtractedMetadata["width"]))
	origH := int(getDim(dataset.ExtractedMetadata["height"]))
	transform, _ := extractAffineTransform(dataset.ExtractedMetadata)

	if resampling == "" {
		resampling = recommendedResamplingForDataset(dataset)
	}
	if opacity <= 0 {
		opacity = 1.0
	}

	return &gis.MBTilesLayerInput{
		ImagePath:      imgPath,
		Bounds:         bounds,
		Transform:      transform,
		OriginalWidth:  origW,
		OriginalHeight: origH,
		Opacity:        opacity,
		Resampling:     resampling,
	}, nil
}

// GenerateDatasetMBTiles builds an optimized SQLite 1.3 .mbtiles archive for a georeferenced dataset
// and populates its fast XYZ tile cache for MapLibre rendering.
func (s *DatasetService) GenerateDatasetMBTiles(sessionID, datasetID, resampling string) (*gis.MBTilesInspection, string, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || dataset == nil {
		return nil, "", repository.ErrDatasetNotFound
	}
	if strings.EqualFold(filepath.Ext(dataset.StoragePath), ".mbtiles") && fileExists(dataset.StoragePath) {
		datasetDir := resolveDatasetDirectory(s.storage, dataset)
		insp, iErr := gis.InspectMBTiles(dataset.StoragePath, filepath.Join(datasetDir, "processed", "mbtiles_preview.png"), filepath.Join(datasetDir, "tiles_xyz"))
		return insp, dataset.StoragePath, iErr
	}

	spec, err := s.BuildMBTilesLayerSpec(dataset, 1.0, resampling)
	if err != nil {
		return nil, "", err
	}

	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	tilesDir := filepath.Join(datasetDir, "tiles")
	_ = os.MkdirAll(tilesDir, 0755)
	mbtilesPath := filepath.Join(tilesDir, "layer.mbtiles")

	res, err := gis.CreateMBTiles(mbtilesPath, gis.MBTilesCreateSpec{
		Name:       strings.TrimSuffix(dataset.Filename, filepath.Ext(dataset.Filename)),
		TileFormat: "png",
		Layers:     []gis.MBTilesLayerInput{*spec},
	})
	if err != nil {
		return nil, "", err
	}

	xyzCacheDir := filepath.Join(datasetDir, "tiles_xyz")
	_ = storage.SafeRemoveAll(xyzCacheDir)
	insp, err := gis.InspectMBTiles(mbtilesPath, "", xyzCacheDir)
	if err != nil {
		return nil, "", err
	}

	dataset.TilePath = mbtilesPath
	dataset.TileStatus = "completed"
	if dataset.ExtractedMetadata == nil {
		dataset.ExtractedMetadata = make(map[string]interface{})
	}
	dataset.ExtractedMetadata["tile_min_zoom"] = res.MinZoom
	dataset.ExtractedMetadata["tile_max_zoom"] = res.MaxZoom
	dataset.ExtractedMetadata["native_max_zoom"] = res.NativeMaxZoom
	dataset.ExtractedMetadata["upscaled_beyond_native"] = res.UpscaledBeyondNative
	dataset.ExtractedMetadata["tile_count"] = res.TileCount
	dataset.ExtractedMetadata["tile_format"] = res.TileFormat
	dataset.ExtractedMetadata["tile_scheme"] = "tms"
	if dataset.MapLayer != nil {
		dataset.MapLayer["tile_url_template"] = fmt.Sprintf("/datasets/%s/tiles/{z}/{x}/{y}", dataset.ID)
		dataset.MapLayer["tile_min_zoom"] = res.MinZoom
		dataset.MapLayer["tile_max_zoom"] = res.MaxZoom
		dataset.MapLayer["tile_format"] = res.TileFormat
	}
	_ = s.repo.UpdateDataset(sessionID, dataset)

	return insp, mbtilesPath, nil
}
