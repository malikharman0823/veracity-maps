package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

func resolveScript(scriptName string) string {
	candidates := []string{
		filepath.Join("scripts", scriptName),
		filepath.Join("backend", "scripts", scriptName),
		filepath.Join("..", "scripts", scriptName),
		filepath.Join("..", "backend", "scripts", scriptName),
	}
	for _, c := range candidates {
		if fileExists(c) {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	return filepath.Join("backend", "scripts", scriptName)
}

// InspectPdfDocument inspects a PDF file, extracts page dimensions & rotation, and generates thumbnails
func (s *DatasetService) InspectPdfDocument(sessionID, docID, pdfPath, originalFilename string, size int64) (*models.PdfInspectionResult, error) {
	sessionDir := s.storage.GetSessionDir(sessionID)
	thumbDir := filepath.Join(sessionDir, "pdf_thumbnails", docID)
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: Failed to create thumbnail dir: %w", err)
	}

	pythonScript := resolveScript("pdf_tool.py")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", pythonScript, "inspect", pdfPath, thumbDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("PDF_INSPECTION_FAILED: %s (%v)", strings.TrimSpace(string(out)), err)
	}

	var res struct {
		Success   bool                  `json:"success"`
		Error     string                `json:"error"`
		Code      string                `json:"code"`
		PageCount int                   `json:"page_count"`
		Pages     []*models.PdfPageInfo `json:"pages"`
	}

	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("PDF_PARSE_ERROR: Failed to parse inspect output: %w", err)
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

// ImportPdfPages processes selected PDF pages into logical Dataset raster layers
func (s *DatasetService) ImportPdfPages(sessionID string, req *models.ImportPdfPagesRequest) (*models.ImportPdfPagesResponse, error) {
	if req.DocumentID == "" {
		return nil, fmt.Errorf("MISSING_DOCUMENT_ID: document_id is required")
	}
	if len(req.SelectedPages) == 0 {
		return nil, fmt.Errorf("EMPTY_SELECTION: At least one page must be selected")
	}

	sessionDir := s.storage.GetSessionDir(sessionID)
	rawDocsDir := filepath.Join(sessionDir, "raw_docs")

	// Locate source PDF
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

	pythonScript := resolveScript("pdf_tool.py")

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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", pythonScript, "render", pdfPath, pagesArg, renderTempDir, "150")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("PDF_RENDER_FAILED: %s (%v)", strings.TrimSpace(string(out)), err)
	}

	var res struct {
		Success  bool   `json:"success"`
		Error    string `json:"error"`
		Code     string `json:"code"`
		Rendered []struct {
			PageNumber int    `json:"page_number"`
			Filename   string `json:"filename"`
			Path       string `json:"path"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			DPI        int    `json:"dpi"`
			Rotation   int    `json:"rotation"`
		} `json:"rendered"`
	}

	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("PDF_PARSE_ERROR: Failed to parse render output: %w", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("%s: %s", res.Code, res.Error)
	}

	// Determine building assignment
	bldName := req.BuildingName
	if bldName == "" {
		bldName = strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	}

	var bld *models.Building
	if req.BuildingID != "" {
		bld, _ = s.repo.GetBuilding(sessionID, req.BuildingID)
	}
	if bld == nil && bldName != "" {
		// Look up existing building or create
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
	origBase := filepath.Base(pdfPath)

	for idx, rPage := range res.Rendered {
		datasetID := repository.GenerateDatasetID()
		datasetDir := s.storage.GetDatasetDir(sessionID, datasetID)
		_ = os.MkdirAll(datasetDir, 0755)

		pageFilename := fmt.Sprintf("%s - Page %d.png", strings.TrimSuffix(origBase, filepath.Ext(origBase)), rPage.PageNumber)
		targetPath := filepath.Join(datasetDir, pageFilename)

		if cpErr := storage.CopyFile(rPage.Path, targetPath); cpErr != nil {
			continue
		}

		fi, _ := os.Stat(targetPath)
		fileSize := int64(0)
		if fi != nil {
			fileSize = fi.Size()
		}

		meta, _ := gis.InspectRasterFile(targetPath)
		w := rPage.Width
		h := rPage.Height
		if meta != nil && meta.Width > 0 {
			w = meta.Width
			h = meta.Height
		}

		floorLvl := rPage.PageNumber - 1
		floorLbl := fmt.Sprintf("Floor %d", floorLvl)
		if floorLvl == 0 {
			floorLbl = "Ground Floor"
		}

		extractedMeta := map[string]interface{}{
			"driver":               "PNG",
			"width":                w,
			"height":               h,
			"bands":                4,
			"dtype":                "uint8",
			"color_interpretation": []string{"Red", "Green", "Blue", "Alpha"},
			"format":               "png",
			"document_type":        "pdf",
			"source_document_id":   req.DocumentID,
			"source_document_type": "pdf",
			"source_filename":      origBase,
			"original_source_path": pdfPath,
			"pdf_page_number":      rPage.PageNumber,
			"rendering_dpi":        rPage.DPI,
			"rotation":             rPage.Rotation,
			"georeferencing_mode":  req.GeoreferencingMode,
			"floor_level":          floorLvl,
			"floor_label":          floorLbl,
			"building_name":        bldName,
			"is_building_floor":    true,
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
			GeoreferenceStatus: "not_started",
			TileStatus:         "not_generated",
			ExtractedMetadata:  extractedMeta,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		_ = s.repo.InsertDataset(sessionID, dataset)

		// Add to building floor
		if bld != nil {
			_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
				Level:      floorLvl,
				Name:       floorLbl,
				DatasetIDs: []string{datasetID},
			})
		}

		createdDatasets = append(createdDatasets, dataset)
	}

	// Attach as members to workspace project
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
				"floor_label":            ds.ExtractedMetadata["floor_label"],
				"floor_level":            ds.ExtractedMetadata["floor_level"],
				"visible":                true,
				"raster_visible":         true,
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

// ConvertCadDxfToDataset converts a DXF drawing to a high-resolution PNG dataset
func (s *DatasetService) ConvertCadDxfToDataset(sessionID, datasetID, datasetDir, dxfPath, originalFilename string, size int64, fileHash string) (*models.Dataset, error) {
	pythonScript := resolveScript("cad_tool.py")

	baseName := strings.TrimSuffix(originalFilename, filepath.Ext(originalFilename))
	renderedPngPath := filepath.Join(datasetDir, baseName+".png")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python", pythonScript, "convert", dxfPath, renderedPngPath, "200")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("CAD_CONVERSION_FAILED: %s (%v)", strings.TrimSpace(string(out)), err)
	}

	var res struct {
		Success    bool      `json:"success"`
		Error      string    `json:"error"`
		Code       string    `json:"code"`
		Filename   string    `json:"filename"`
		Width      int       `json:"width"`
		Height     int       `json:"height"`
		DPI        int       `json:"dpi"`
		Units      string    `json:"units"`
		Extents    []float64 `json:"extents"`
		Layers     []string  `json:"layers"`
		DxfVersion string    `json:"dxf_version"`
	}

	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("CAD_PARSE_ERROR: Failed to parse converter output: %w", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("%s: %s", res.Code, res.Error)
	}

	fi, _ := os.Stat(renderedPngPath)
	rasterSize := size
	if fi != nil {
		rasterSize = fi.Size()
	}

	extractedMeta := map[string]interface{}{
		"driver":               "PNG",
		"width":                res.Width,
		"height":               res.Height,
		"bands":                4,
		"dtype":                "uint8",
		"color_interpretation": []string{"Red", "Green", "Blue", "Alpha"},
		"format":               "png",
		"document_type":        "cad",
		"source_document_id":   datasetID,
		"source_document_type": "cad_dxf",
		"source_filename":      originalFilename,
		"original_source_path": dxfPath,
		"cad_format":           "dxf",
		"cad_units":            res.Units,
		"cad_extents":          res.Extents,
		"cad_layers":           res.Layers,
		"cad_version":          res.DxfVersion,
		"rendering_dpi":        res.DPI,
		"floor_level":          0,
		"floor_label":          "Ground Floor",
		"building_name":        baseName,
		"is_building_floor":    false,
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

	_ = s.repo.InsertDataset(sessionID, dataset)

	return dataset, nil
}
