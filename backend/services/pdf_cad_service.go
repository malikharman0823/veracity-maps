package services

import (
	"fmt"
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

// InspectPdfDocument inspects a PDF file, classifies each page as vector, hybrid, or scanned raster,
// extracts page dimensions, fonts, and embedded image DPI, and generates page thumbnails.
func (s *DatasetService) InspectPdfDocument(sessionID, docID, pdfPath, originalFilename string, size int64) (*models.PdfInspectionResult, error) {
	sessionDir := s.storage.GetSessionDir(sessionID)
	thumbDir := filepath.Join(sessionDir, "pdf_thumbnails", docID)
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: Failed to create thumbnail dir: %w", err)
	}

	var res struct {
		Success   bool                  `json:"success"`
		Error     string                `json:"error"`
		Code      string                `json:"code"`
		PageCount int                   `json:"page_count"`
		Pages     []*models.PdfPageInfo `json:"pages"`
	}

	if err := gis.RunPythonJSON(45*time.Second, "pdf_tool.py", []string{"inspect", pdfPath, thumbDir}, &res); err != nil {
		return nil, fmt.Errorf("PDF_INSPECTION_FAILED: %w", err)
	}

	if !res.Success {
		code := res.Code
		if code == "" {
			code = "PDF_ERROR"
		}
		return nil, fmt.Errorf("%s: %s", code, res.Error)
	}

	return &models.PdfInspectionResult{
		DocumentID:  docID,
		Filename:    originalFilename,
		FileSize:    size,
		PageCount:   res.PageCount,
		Pages:       res.Pages,
		StoragePath: pdfPath,
	}, nil
}

// ImportPdfPages processes selected PDF pages via dual-path processing:
// - Path A (Vector/Hybrid PDF): adaptive high-DPI rendering + vector linework & positioned text extraction.
// - Path B (Raster/Scanned PDF): adaptive DPI rendering + non-destructive OpenCV enhancement + optional OCR.
// Each selected page becomes an ordered multi-floor layer while preserving original page ordering.
func (s *DatasetService) ImportPdfPages(sessionID string, req *models.ImportPdfPagesRequest) (*models.ImportPdfPagesResponse, error) {
	if req.DocumentID == "" {
		return nil, fmt.Errorf("MISSING_DOCUMENT_ID: document_id is required")
	}
	if len(req.SelectedPages) == 0 {
		return nil, fmt.Errorf("EMPTY_SELECTION: At least one page must be selected")
	}

	sessionDir := s.storage.GetSessionDir(sessionID)
	rawDocsDir := filepath.Join(sessionDir, "raw_docs")

	var pdfPath string
	entries, err := os.ReadDir(rawDocsDir)
	if err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), req.DocumentID) {
				pdfPath = filepath.Join(rawDocsDir, e.Name())
				break
			}
		}
	}
	if pdfPath == "" || !fileExists(pdfPath) {
		return nil, fmt.Errorf("DOCUMENT_NOT_FOUND: PDF document %s not found in active session", req.DocumentID)
	}

	renderTempDir := filepath.Join(sessionDir, "temp_pdf_render", req.DocumentID)
	_ = os.MkdirAll(renderTempDir, 0755)
	defer func() {
		_ = storage.SafeRemoveAll(renderTempDir)
	}()

	var pageStrs []string
	for _, p := range req.SelectedPages {
		pageStrs = append(pageStrs, strconv.Itoa(p))
	}
	pagesArg := strings.Join(pageStrs, ",")

	extractVectors := true
	if req.ExtractVectors != nil {
		extractVectors = *req.ExtractVectors
	}

	var res struct {
		Success  bool   `json:"success"`
		Error    string `json:"error"`
		Code     string `json:"code"`
		Rendered []struct {
			PageNumber         int                        `json:"page_number"`
			Filename           string                     `json:"filename"`
			Path               string                     `json:"path"`
			EnhancedPath       string                     `json:"enhanced_path"`
			Enhancement        map[string]interface{}     `json:"enhancement"`
			VectorsPath        string                     `json:"vectors_path"`
			VectorFeatureCount int                        `json:"vector_feature_count"`
			AnnotationCount    int                        `json:"annotation_count"`
			Width              int                        `json:"width"`
			Height             int                        `json:"height"`
			WidthPt            float64                    `json:"width_pt"`
			HeightPt           float64                    `json:"height_pt"`
			DPI                int                        `json:"dpi"`
			Rotation           int                        `json:"rotation"`
			HasGeoreference    bool                       `json:"has_georeference"`
			ProcessingPath     string                     `json:"processing_path"`
			Fonts              []string                   `json:"fonts"`
			OcrRecommended     bool                       `json:"ocr_recommended"`
			Content            *models.ContentComposition `json:"content"`
			Quality            map[string]interface{}     `json:"quality"`
			Recommendation     map[string]interface{}     `json:"recommendation"`
		} `json:"rendered"`
	}

	err = gis.RunPythonJSON(
		120*time.Second,
		"pdf_tool.py",
		[]string{
			"render",
			pdfPath,
			pagesArg,
			renderTempDir,
			strconv.Itoa(req.DPI),
			req.EnhancePreset,
			strconv.FormatBool(extractVectors),
		},
		&res,
	)
	if err != nil {
		return nil, fmt.Errorf("PDF_RENDER_FAILED: %w", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("%s: %s", res.Code, res.Error)
	}

	origBase := filepath.Base(pdfPath)
	// Strip leading "<docID>_" prefix from display building/layer names when present
	cleanOrigBase := origBase
	if strings.HasPrefix(cleanOrigBase, req.DocumentID+"_") {
		cleanOrigBase = strings.TrimPrefix(cleanOrigBase, req.DocumentID+"_")
	}

	bldName := req.BuildingName
	if bldName == "" {
		bldName = strings.TrimSuffix(cleanOrigBase, filepath.Ext(cleanOrigBase))
	}

	var bld *models.Building
	if req.BuildingID != "" {
		bld, _ = s.repo.GetBuilding(sessionID, req.BuildingID)
	}
	if bld == nil && bldName != "" {
		blds, _ := s.repo.ListBuildings(sessionID)
		for _, b := range blds {
			if strings.EqualFold(b.Name, bldName) {
				bld = b
				break
			}
		}
		if bld == nil {
			bld, _ = s.repo.CreateBuilding(sessionID, &models.Building{
				Name:      bldName,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			})
		}
	}

	var createdDatasets []*models.Dataset
	var primaryDatasetID string

	for idx, rPage := range res.Rendered {
		datasetID := repository.GenerateDatasetID()
		datasetDir := s.storage.GetDatasetDir(sessionID, datasetID)
		_ = os.MkdirAll(datasetDir, 0755)

		pageFilename := fmt.Sprintf("%s - Page %d.png", strings.TrimSuffix(cleanOrigBase, filepath.Ext(cleanOrigBase)), rPage.PageNumber)
		targetPath := filepath.Join(datasetDir, pageFilename)

		if cpErr := storage.CopyFile(rPage.Path, targetPath); cpErr != nil {
			continue
		}

		fi, _ := os.Stat(targetPath)
		fileSize := int64(0)
		if fi != nil {
			fileSize = fi.Size()
		}

		w := rPage.Width
		h := rPage.Height
		floorLvl := rPage.PageNumber - 1
		floorLbl := fmt.Sprintf("Page %d (Floor %d)", rPage.PageNumber, floorLvl)
		if floorLvl == 0 {
			floorLbl = "Page 1 (Ground Floor)"
		}

		extractedMeta := map[string]interface{}{
			"driver":               "PNG",
			"width":                w,
			"height":               h,
			"width_pt":             rPage.WidthPt,
			"height_pt":            rPage.HeightPt,
			"bands":                3,
			"color_depth_bits":     24,
			"compression":          "deflate",
			"dtype":                "uint8",
			"color_interpretation": []string{"Red", "Green", "Blue"},
			"format":               "png",
			"document_type":        "pdf",
			"source_document_id":   req.DocumentID,
			"source_document_type": "pdf",
			"source_filename":      cleanOrigBase,
			"original_source_path": pdfPath,
			"pdf_page_number":      rPage.PageNumber,
			"pdf_processing_path":  rPage.ProcessingPath,
			"pdf_fonts":            rPage.Fonts,
			"ocr_recommended":      rPage.OcrRecommended,
			"rendering_dpi":        rPage.DPI,
			"dpi_x":                float64(rPage.DPI),
			"dpi_y":                float64(rPage.DPI),
			"rotation":             rPage.Rotation,
			"georeferencing_mode":  req.GeoreferencingMode,
			"floor_level":          floorLvl,
			"floor_label":          floorLbl,
			"building_name":        bldName,
			"is_building_floor":    true,
			"content":              rPage.Content,
			"quality":              rPage.Quality,
			"recommendation":       rPage.Recommendation,
		}

		var processedPath string
		if rPage.EnhancedPath != "" && fileExists(rPage.EnhancedPath) {
			procDir := filepath.Join(datasetDir, "processed")
			_ = os.MkdirAll(procDir, 0755)
			destEnh := filepath.Join(procDir, "enhanced.png")
			if storage.CopyFile(rPage.EnhancedPath, destEnh) == nil {
				processedPath = destEnh
				presetUsed := req.EnhancePreset
				if presetUsed == "" {
					presetUsed = "text_clarity"
				}
				extractedMeta["enhancement"] = &models.EnhancementState{
					Enabled:           true,
					ActiveVariant:     "enhanced",
					Preset:            presetUsed,
					EnhancedPath:      destEnh,
					OriginalWidth:     w,
					OriginalHeight:    h,
					EnhancedWidth:     w,
					EnhancedHeight:    h,
					ScaleRatio:        1.0,
					OperationsApplied: []string{"pdf_scanned_opencv_enhancement"},
					UpdatedAt:         time.Now().UTC().Format(time.RFC3339),
				}
			}
		}

		if idx == 0 {
			primaryDatasetID = datasetID
		}
		if req.GeoreferencingMode == "shared" && primaryDatasetID != "" {
			extractedMeta["shared_reference_id"] = primaryDatasetID
		}

		dataset := &models.Dataset{
			ID:                 datasetID,
			SessionID:          sessionID,
			Filename:           pageFilename,
			FileSize:           fileSize,
			ContentType:        "image/png",
			StoragePath:        targetPath,
			ProcessedPath:      processedPath,
			GeoreferenceStatus: "not_started",
			TileStatus:         "not_generated",
			ExtractedMetadata:  extractedMeta,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		_ = s.repo.InsertDataset(sessionID, dataset)

		if rPage.VectorsPath != "" && fileExists(rPage.VectorsPath) {
			_ = s.SaveExtractedVectorsPayload(sessionID, dataset, rPage.VectorsPath)
		}
		if req.RunOCR {
			_, _ = s.RunDatasetOCR(sessionID, dataset.ID, 0.45)
		}

		if bld != nil {
			_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
				Level:        floorLvl,
				Name:         floorLbl,
				DatasetIDs:   []string{datasetID},
				DisplayOrder: idx,
			})
		}

		createdDatasets = append(createdDatasets, dataset)
	}

	if len(createdDatasets) > 0 {
		masterID := createdDatasets[0].ID
		proj, _ := s.repo.GetMapProject(sessionID, masterID)
		if proj == nil {
			proj = &models.MapProject{
				ID:            "proj_" + repository.GenerateDatasetID(),
				DatasetID:     masterID,
				Name:          bldName,
				Configuration: map[string]interface{}{},
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
		}
		if proj.Configuration == nil {
			proj.Configuration = make(map[string]interface{})
		}
		var members []interface{}
		for i, ds := range createdDatasets {
			members = append(members, map[string]interface{}{
				"dataset_id":             ds.ID,
				"name":                   ds.Filename,
				"building_name":          bldName,
				"is_building_floor":      true,
				"floor_label":            ds.ExtractedMetadata["floor_label"],
				"floor_level":            ds.ExtractedMetadata["floor_level"],
				"visible":                i == 0,
				"raster_visible":         i == 0,
				"raster_opacity":         1.0,
				"control_points_visible": true,
				"icon_points_visible":    true,
				"sort_order":             i,
			})
		}
		proj.Configuration["members"] = members
		_ = s.repo.SaveMapProject(sessionID, proj)
	}

	return &models.ImportPdfPagesResponse{
		Success:            true,
		DocumentID:         req.DocumentID,
		GeoreferencingMode: req.GeoreferencingMode,
		BuildingID:         bldName,
		Datasets:           createdDatasets,
	}, nil
}

// ConvertCadDxfToDataset processes a CAD (.dxf) drawing by extracting CAD layers, units, scale,
// block references, vector geometry, and text/dimension annotations, while rendering a zero-margin
// aspect-locked reference raster so CAD coordinates map linearly 1-to-1 to map coordinates.
func (s *DatasetService) ConvertCadDxfToDataset(sessionID, datasetID, datasetDir, dxfPath, originalFilename string, size int64, fileHash string) (*models.Dataset, error) {
	baseName := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename))
	renderedPngPath := filepath.Join(datasetDir, baseName+".png")
	rawVecPath := filepath.Join(datasetDir, "vector", "source_vectors.json")

	var res struct {
		Success             bool                       `json:"success"`
		Error               string                     `json:"error"`
		Code                string                     `json:"code"`
		Filename            string                     `json:"filename"`
		Width               int                        `json:"width"`
		Height              int                        `json:"height"`
		DPI                 int                        `json:"dpi"`
		Units               string                     `json:"units"`
		DimScale            float64                    `json:"dim_scale"`
		LtScale             float64                    `json:"lt_scale"`
		Extents             []float64                  `json:"extents"`
		ViewBoundsCAD       []float64                  `json:"view_bounds_cad"`
		CADTransform        map[string]interface{}     `json:"cad_transform"`
		Layers              []string                   `json:"layers"`
		LayerDetails        []map[string]interface{}   `json:"layer_details"`
		EntityCounts        map[string]int             `json:"entity_counts"`
		BlockReferenceCount int                        `json:"block_reference_count"`
		VectorFeatureCount  int                        `json:"vector_feature_count"`
		AnnotationCount     int                        `json:"annotation_count"`
		VectorsPath         string                     `json:"vectors_path"`
		DxfVersion          string                     `json:"dxf_version"`
		Content             *models.ContentComposition `json:"content"`
	}

	if err := gis.RunPythonJSON(60*time.Second, "cad_tool.py", []string{"convert", dxfPath, renderedPngPath, "220", rawVecPath}, &res); err != nil {
		return nil, fmt.Errorf("CAD_CONVERSION_FAILED: %w", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("%s: %s", res.Code, res.Error)
	}

	fi, _ := os.Stat(renderedPngPath)
	rasterSize := size
	if fi != nil {
		rasterSize = fi.Size()
	}

	var quality map[string]interface{}
	var recommend map[string]interface{}
	if rMeta, inspErr := gis.InspectRasterFile(renderedPngPath); inspErr == nil && rMeta != nil {
		quality = rMeta.Quality
		recommend = rMeta.Recommendation
	}

	extractedMeta := map[string]interface{}{
		"driver":                    "PNG",
		"width":                     res.Width,
		"height":                    res.Height,
		"bands":                     4,
		"color_depth_bits":          32,
		"compression":               "deflate",
		"dtype":                     "uint8",
		"color_interpretation":      []string{"Red", "Green", "Blue", "Alpha"},
		"format":                    "png",
		"document_type":             "cad",
		"source_document_id":        datasetID,
		"source_document_type":      "cad_dxf",
		"source_filename":           originalFilename,
		"original_source_path":      dxfPath,
		"cad_format":                "dxf",
		"cad_units":                 res.Units,
		"cad_dim_scale":             res.DimScale,
		"cad_lt_scale":              res.LtScale,
		"cad_extents":               res.Extents,
		"cad_view_bounds":           res.ViewBoundsCAD,
		"cad_transform":             res.CADTransform,
		"cad_layers":                res.Layers,
		"cad_layer_details":         res.LayerDetails,
		"cad_entity_counts":         res.EntityCounts,
		"cad_block_reference_count": res.BlockReferenceCount,
		"cad_version":               res.DxfVersion,
		"rendering_dpi":             res.DPI,
		"dpi_x":                     float64(res.DPI),
		"dpi_y":                     float64(res.DPI),
		"floor_level":               0,
		"floor_label":               "Ground Floor",
		"building_name":             baseName,
		"is_building_floor":         true,
		"content":                   res.Content,
		"quality":                   quality,
		"recommendation":            recommend,
	}

	dataset := &models.Dataset{
		ID:                 datasetID,
		SessionID:          sessionID,
		Filename:           res.Filename,
		FileSize:           rasterSize,
		ContentType:        "image/png",
		FileHash:           fileHash,
		StoragePath:        renderedPngPath,
		GeoreferenceStatus: "not_started",
		TileStatus:         "not_generated",
		ExtractedMetadata:  extractedMeta,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
		return nil, err
	}

	if res.VectorsPath != "" && fileExists(res.VectorsPath) {
		_ = s.SaveExtractedVectorsPayload(sessionID, dataset, res.VectorsPath)
	}

	buildings, _ := s.repo.ListBuildings(sessionID)
	var bld *models.Building
	for _, b := range buildings {
		if strings.EqualFold(b.Name, baseName) {
			bld = b
			break
		}
	}
	if bld == nil {
		bld, _ = s.repo.CreateBuilding(sessionID, &models.Building{
			Name:              baseName,
			DefaultFloorLevel: 0,
		})
	}
	if bld != nil {
		_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
			Level:      0,
			Name:       "Ground Floor",
			DatasetIDs: []string{datasetID},
		})
	}

	// Create MapProject for the CAD dataset so it is registered in workspace manifest and export hierarchy
	proj, _ := s.repo.GetMapProject(sessionID, datasetID)
	if proj == nil {
		proj = &models.MapProject{
			ID:            "proj_" + repository.GenerateDatasetID(),
			DatasetID:     datasetID,
			Name:          baseName,
			Configuration: map[string]interface{}{},
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
	}
	if proj.Configuration == nil {
		proj.Configuration = make(map[string]interface{})
	}
	proj.Configuration["members"] = []interface{}{
		map[string]interface{}{
			"dataset_id":             datasetID,
			"name":                   dataset.Filename,
			"building_name":          baseName,
			"is_building_floor":      true,
			"floor_label":            "Ground Floor",
			"floor_level":            0,
			"visible":                true,
			"raster_visible":         true,
			"raster_opacity":         1.0,
			"control_points_visible": true,
			"icon_points_visible":    true,
			"sort_order":             0,
		},
	}
	_ = s.repo.SaveMapProject(sessionID, proj)

	return dataset, nil
}
