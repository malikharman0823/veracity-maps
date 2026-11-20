package tests

import (
	"archive/zip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func writeCustomTilesTestImage(filePath string, w, h int) error {
	_ = os.MkdirAll(filepath.Dir(filePath), 0755)
	b := createTestPNG(w, h)
	return os.WriteFile(filePath, b, 0644)
}

func TestCustomLocalTiles_ExportAndReimportWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir:  filepath.Join(tempDir, "sessions"),
		UploadDir:          filepath.Join(tempDir, "uploads"),
		ProcessedDir:       filepath.Join(tempDir, "processed"),
		MaxUploadSizeBytes: 100 * 1024 * 1024,
	}
	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("failed creating storage manager: %v", err)
	}
	repo := repository.NewSessionRepository()
	sess := repo.CreateSession()
	sessionID := sess.ID

	dsService := services.NewDatasetService(cfg, repo, sm)
	exportService := services.NewExportService(repo, sm)

	dsID := "ds_test_custom_tiles"
	dsDir := sm.GetDatasetDir(sessionID, dsID)
	_ = os.MkdirAll(dsDir, 0755)
	rawPNG := filepath.Join(dsDir, "floorplan.png")
	if err := writeCustomTilesTestImage(rawPNG, 512, 512); err != nil {
		t.Fatalf("failed writing test image: %v", err)
	}

	bounds := []float64{77.20, 28.60, 77.22, 28.62} // [west, south, east, north]
	dataset := &models.Dataset{
		ID:                 dsID,
		SessionID:          sessionID,
		Filename:           "floorplan.png",
		FileSize:           1024,
		ContentType:        "image/png",
		StoragePath:        rawPNG,
		GeoreferenceStatus: "completed",
		TileStatus:         "completed",
		ExtractedMetadata: map[string]interface{}{
			"width":        512,
			"height":       512,
			"crs":          "EPSG:4326",
			"initial_zoom": 17,
			"minzoom":      14,
			"maxzoom":      18,
			"bounds": map[string]interface{}{
				"min_x": bounds[0],
				"min_y": bounds[1],
				"max_x": bounds[2],
				"max_y": bounds[3],
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.InsertDataset(sessionID, dataset); err != nil {
		t.Fatalf("failed inserting dataset: %v", err)
	}

	// Add test control points
	cps := []*models.ControlPoint{
		{
			ID:        "cp_1",
			DatasetID: dsID,
			ImageX:    100,
			ImageY:    100,
			Longitude: 77.205,
			Latitude:  28.605,
			CreatedAt: time.Now(),
		},
		{
			ID:        "cp_2",
			DatasetID: dsID,
			ImageX:    400,
			ImageY:    400,
			Longitude: 77.215,
			Latitude:  28.615,
			CreatedAt: time.Now(),
		},
	}
	_ = repo.SetControlPoints(sessionID, dsID, cps)

	// Add test icon point
	_, _ = repo.CreateIconPoint(sessionID, &models.IconPoint{
		ID:          "ip_1",
		SessionID:   sessionID,
		DatasetID:   dsID,
		Scope:       "poi",
		Name:        "Main Entrance",
		Description: "Primary access point",
		IconID:      "entrance_icon",
		Longitude:   77.210,
		Latitude:    28.610,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})

	// 1. Export Custom Local Tiles package
	minZ := 15
	maxZ := 17
	initZ := 16
	zipPath, exportName, err := exportService.GenerateSelectiveExport(sessionID, dsID, services.SelectiveExportParams{
		Format:      "custom_local_tiles",
		ProjectName: "Test Delhi Campus",
		LayerIDs:    []string{dsID},
		Options: &services.SelectiveExportOptions{
			IncludeControlPoints:    true,
			IncludeIcons:            true,
			IncludeLayerStyles:      true,
			IncludeOriginalRasters:  true,
			IncludeProcessedRasters: true,
			IncludeProjectMetadata:  true,
			ZoomConfig: &services.ZoomConfiguration{
				MinZoom:     &minZ,
				MaxZoom:     &maxZ,
				InitialZoom: &initZ,
			},
		},
	})
	if err != nil {
		t.Fatalf("GenerateSelectiveExport failed: %v", err)
	}
	if !strings.HasSuffix(exportName, ".zip") {
		t.Fatalf("expected .zip extension, got %s", exportName)
	}
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		t.Fatalf("exported zip does not exist at %s", zipPath)
	}

	// 2. Validate zip structure & entries
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("failed opening exported zip: %v", err)
	}
	defer zr.Close()

	entryNames := make(map[string]bool)
	for _, f := range zr.File {
		entryNames[f.Name] = true
	}

	if !entryNames["tile_config.json"] && !entryNames["Custom_Local_Tiles/tile_config.json"] {
		t.Fatalf("exported package missing tile_config.json. Entries: %v", zr.File)
	}
	if !entryNames["geojson/control_points.geojson"] && !entryNames["Custom_Local_Tiles/geojson/control_points.geojson"] {
		t.Fatalf("exported package missing control_points.geojson")
	}
	if !entryNames["geojson/icon_points.geojson"] && !entryNames["Custom_Local_Tiles/geojson/icon_points.geojson"] {
		t.Fatalf("exported package missing icon_points.geojson")
	}
	if !entryNames["styles/map_style.json"] && !entryNames["Custom_Local_Tiles/styles/map_style.json"] {
		t.Fatalf("exported package missing map_style.json")
	}

	// Verify tile_config.json content
	var cfgFile *zip.File
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(f.Name), "tile_config.json") {
			cfgFile = f
			break
		}
	}
	if cfgFile == nil {
		t.Fatalf("tile_config.json file not found in zip")
	}
	rc, err := cfgFile.Open()
	if err != nil {
		t.Fatalf("failed opening tile_config.json: %v", err)
	}
	var cfgDoc services.CustomLocalTilesConfig
	if err := json.NewDecoder(rc).Decode(&cfgDoc); err != nil {
		_ = rc.Close()
		t.Fatalf("failed decoding tile_config.json: %v", err)
	}
	_ = rc.Close()

	if cfgDoc.Format != "custom_local_tiles" {
		t.Errorf("expected format custom_local_tiles, got %s", cfgDoc.Format)
	}
	if cfgDoc.MinZoom != minZ || cfgDoc.MaxZoom != maxZ || cfgDoc.InitialZoom != initZ {
		t.Errorf("zoom settings mismatch: got min=%d, max=%d, init=%d", cfgDoc.MinZoom, cfgDoc.MaxZoom, cfgDoc.InitialZoom)
	}
	if len(cfgDoc.RasterLayers) == 0 {
		t.Errorf("expected at least 1 raster layer in tile_config.json")
	}

	// 3. Test ValidateExport
	valRes, valErr := services.ValidateExport(zipPath, "custom_local_tiles", dsID)
	if valErr != nil || !valRes.Valid {
		t.Fatalf("ValidateExport failed: %v", valErr)
	}

	// 4. Test Re-Import via DatasetService.ReopenProjectZipFromPath
	newSession := repo.CreateSession()
	reopenDsID := "ds_reopened_custom_tiles"
	reopenDir := sm.GetDatasetDir(newSession.ID, reopenDsID)
	_ = os.MkdirAll(reopenDir, 0755)

	stagedZip := filepath.Join(reopenDir, "imported.zip")
	if err := storage.CopyFile(zipPath, stagedZip); err != nil {
		t.Fatalf("failed copying staged zip: %v", err)
	}
	fi, _ := os.Stat(stagedZip)

	restoredDs, err := dsService.ReopenProjectZipFromPath(newSession.ID, reopenDsID, reopenDir, stagedZip, "imported.zip", fi.Size(), "hash123")
	if err != nil {
		t.Fatalf("ReopenProjectZipFromPath failed: %v", err)
	}
	if restoredDs == nil {
		t.Fatalf("restored dataset is nil")
	}

	// Verify restored control points
	restoredCPs, _ := repo.ListControlPoints(newSession.ID, reopenDsID)
	if len(restoredCPs) != 2 {
		t.Errorf("expected 2 restored control points, got %d", len(restoredCPs))
	}

	// Verify restored icon points
	restoredIPs, _ := repo.ListIconPoints(newSession.ID, reopenDsID, "")
	if len(restoredIPs) != 1 {
		t.Errorf("expected 1 restored icon point, got %d", len(restoredIPs))
	}
	if len(restoredIPs) > 0 && restoredIPs[0].Name != "Main Entrance" {
		t.Errorf("expected icon point name 'Main Entrance', got '%s'", restoredIPs[0].Name)
	}

	// Verify restored map layer
	if restoredDs.MapLayer == nil {
		t.Fatalf("expected restored MapLayer to be non-nil")
	}
	layerType, _ := restoredDs.MapLayer["type"].(string)
	if layerType != "custom_local_tiles" {
		t.Errorf("expected layer type custom_local_tiles, got %s", layerType)
	}
}

func TestCustomLocalTiles_HTTPHandlerExportDefault(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir:  filepath.Join(tempDir, "sessions"),
		UploadDir:          filepath.Join(tempDir, "uploads"),
		ProcessedDir:       filepath.Join(tempDir, "processed"),
		MaxUploadSizeBytes: 50 * 1024 * 1024,
	}
	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("storage manager error: %v", err)
	}
	repo := repository.NewSessionRepository()
	sess := repo.CreateSession()

	exportSvc := services.NewExportService(repo, sm)
	exportH := export.NewHandler(exportSvc, repo)

	dsID := "ds_http_export"
	dsDir := sm.GetDatasetDir(sess.ID, dsID)
	_ = os.MkdirAll(dsDir, 0755)
	imgFile := filepath.Join(dsDir, "map.png")
	if err := writeCustomTilesTestImage(imgFile, 256, 256); err != nil {
		t.Fatalf("failed writing test image: %v", err)
	}

	dataset := &models.Dataset{
		ID:                 dsID,
		SessionID:          sess.ID,
		Filename:           "map.png",
		StoragePath:        imgFile,
		GeoreferenceStatus: "completed",
		TileStatus:         "completed",
		ExtractedMetadata: map[string]interface{}{
			"width":  256,
			"height": 256,
			"bounds": map[string]interface{}{
				"min_x": 0.0,
				"min_y": 0.0,
				"max_x": 1.0,
				"max_y": 1.0,
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = repo.InsertDataset(sess.ID, dataset)

	// POST /datasets/{id}/export without specifying format -> defaults to custom_local_tiles
	reqBody := `{"project_name":"HTTP Custom Tiles"}`
	req := httptest.NewRequest(http.MethodPost, "/datasets/"+dsID+"/export", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sess.ID)
	w := httptest.NewRecorder()

	exportH.Export(w, req, dsID)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d. Body: %s", resp.StatusCode, w.Body.String())
	}

	var apiResp struct {
		Success bool                  `json:"success"`
		Data    export.ExportResponse `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed decoding export response: %v", err)
	}
	expResp := apiResp.Data

	if expResp.Format != "custom_local_tiles" {
		t.Errorf("expected default export format custom_local_tiles, got %s", expResp.Format)
	}
	if expResp.Filename != "Custom_Local_Tiles.zip" {
		t.Errorf("expected filename Custom_Local_Tiles.zip, got %s", expResp.Filename)
	}
}
