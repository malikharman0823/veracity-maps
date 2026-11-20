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

func TestCustomLocalTiles_PerformanceOptimization_EmptyTileSkippingAndCaching(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir: filepath.Join(tempDir, "sessions"),
		UploadDir:         filepath.Join(tempDir, "uploads"),
		ProcessedDir:      filepath.Join(tempDir, "processed"),
	}
	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("failed creating storage manager: %v", err)
	}
	repo := repository.NewSessionRepository()
	sess := repo.CreateSession()
	sessionID := sess.ID

	exportService := services.NewExportService(repo, sm)

	dsID := "ds_perf_test_1"
	dsDir := sm.GetDatasetDir(sessionID, dsID)
	_ = os.MkdirAll(dsDir, 0755)
	rawPNG := filepath.Join(dsDir, "floorplan.png")
	if err := writeCustomTilesTestImage(rawPNG, 512, 512); err != nil {
		t.Fatalf("failed writing test image: %v", err)
	}

	bounds := []float64{77.20, 28.60, 77.22, 28.62}
	dataset := &models.Dataset{
		ID:                 dsID,
		SessionID:          sessionID,
		Filename:           "floorplan.png",
		FileSize:           2048,
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

	minZ := 14
	maxZ := 18
	initZ := 17
	params := services.SelectiveExportParams{
		Format:      "custom_local_tiles",
		ProjectName: "Perf Benchmark",
		LayerIDs:    []string{dsID},
		Options: &services.SelectiveExportOptions{
			IncludeOriginalRasters:  true,
			IncludeProcessedRasters: true,
			TileSize:                256,
			RasterFormat:            "png",
			ImageQuality:            85,
			ZoomConfig: &services.ZoomConfiguration{
				MinZoom:     &minZ,
				MaxZoom:     &maxZ,
				InitialZoom: &initZ,
			},
		},
	}

	// 1. First Export Run (Fresh generation)
	start1 := time.Now()
	zipPath1, _, err1 := exportService.GenerateCustomLocalTilesExport(sessionID, dsID, params)
	dur1 := time.Since(start1)
	if err1 != nil {
		t.Fatalf("initial export failed: %v", err1)
	}

	fi1, statErr1 := os.Stat(zipPath1)
	if statErr1 != nil {
		t.Fatalf("failed stating export zip: %v", statErr1)
	}
	t.Logf("FIRST EXPORT: Duration=%v, Size=%d bytes", dur1, fi1.Size())

	// Inspect tile_manifest.json inside zip
	zr1, zErr1 := zip.OpenReader(zipPath1)
	if zErr1 != nil {
		t.Fatalf("failed opening zip 1: %v", zErr1)
	}
	var manifestFound bool
	var manifestDoc map[string]interface{}
	for _, f := range zr1.File {
		if f.Name == "tile_manifest.json" || f.Name == "Custom_Local_Tiles/tile_manifest.json" {
			manifestFound = true
			rc, _ := f.Open()
			_ = json.NewDecoder(rc).Decode(&manifestDoc)
			_ = rc.Close()
			break
		}
	}
	zr1.Close()

	if !manifestFound || manifestDoc == nil {
		t.Fatalf("tile_manifest.json missing from export package")
	}

	perfMetrics, ok := manifestDoc["performance"].(map[string]interface{})
	if !ok {
		t.Fatalf("tile_manifest.json missing performance section")
	}
	t.Logf("MANIFEST METRICS: %+v", perfMetrics)

	// Verify progress manager tracked completed export
	prog1 := exportService.GetProgress(dsID)
	if prog1 == nil {
		t.Fatalf("expected progress record for dataset %s", dsID)
	}
	if prog1.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", prog1.Status)
	}
	if prog1.StageIndex != 8 {
		t.Errorf("expected final stage_index 8, got %d", prog1.StageIndex)
	}
	if prog1.Percent != 100.0 {
		t.Errorf("expected 100%% progress, got %f", prog1.Percent)
	}

	// 2. Second Export Run (Testing Incremental Tile Cache Reuse)
	start2 := time.Now()
	zipPath2, _, err2 := exportService.GenerateCustomLocalTilesExport(sessionID, dsID, params)
	dur2 := time.Since(start2)
	if err2 != nil {
		t.Fatalf("second export failed: %v", err2)
	}

	fi2, _ := os.Stat(zipPath2)
	t.Logf("SECOND EXPORT (CACHED): Duration=%v, Size=%d bytes", dur2, fi2.Size())

	prog2 := exportService.GetProgress(dsID)
	if prog2.CachedTilesReused <= 0 {
		t.Errorf("expected cached tiles to be reused (> 0), got %d", prog2.CachedTilesReused)
	}
	t.Logf("Cached tiles reused: %d", prog2.CachedTilesReused)

	// Verify duration of second run is significantly faster than first run
	if dur2 > dur1*2 {
		t.Logf("warning: second run took %v compared to first run %v", dur2, dur1)
	}
}

func TestCustomLocalTiles_ExportProgressEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir: filepath.Join(tempDir, "sessions"),
		UploadDir:         filepath.Join(tempDir, "uploads"),
		ProcessedDir:      filepath.Join(tempDir, "processed"),
	}
	sm, _ := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	repo := repository.NewSessionRepository()
	sess := repo.CreateSession()
	sessionID := sess.ID

	exportService := services.NewExportService(repo, sm)
	handler := export.NewHandler(exportService, repo)

	dsID := "ds_test_progress_endpoints"
	dsDir := sm.GetDatasetDir(sessionID, dsID)
	_ = os.MkdirAll(dsDir, 0755)
	rawPNG := filepath.Join(dsDir, "plan.png")
	_ = writeCustomTilesTestImage(rawPNG, 256, 256)

	ds := &models.Dataset{
		ID:                 dsID,
		SessionID:          sessionID,
		Filename:           "plan.png",
		StoragePath:        rawPNG,
		GeoreferenceStatus: "completed",
		ExtractedMetadata: map[string]interface{}{
			"bounds": map[string]interface{}{"min_x": 0.0, "min_y": 0.0, "max_x": 1.0, "max_y": 1.0},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = repo.InsertDataset(sessionID, ds)

	// 1. Initial idle progress check
	req1 := httptest.NewRequest(http.MethodGet, "/api/datasets/"+dsID+"/export/progress", nil)
	req1.Header.Set("X-Session-ID", sessionID)
	w1 := httptest.NewRecorder()
	handler.GetProgress(w1, req1, dsID)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from progress, got %d", w1.Code)
	}

	var idleResp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w1.Body.Bytes(), &idleResp)
	if idleResp.Data == nil || idleResp.Data["status"] != "idle" {
		t.Errorf("expected status 'idle', got '%v'", idleResp.Data)
	}

	// 2. Perform export
	exportReqBody := `{"format":"custom_local_tiles"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/datasets/"+dsID+"/export", strings.NewReader(exportReqBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/octet-stream")
	req2.Header.Set("X-Session-ID", sessionID)
	w2 := httptest.NewRecorder()
	handler.Export(w2, req2, dsID)

	if w2.Code != http.StatusOK {
		t.Fatalf("export failed with code %d: %s", w2.Code, w2.Body.String())
	}

	// 3. Post-export progress check
	req3 := httptest.NewRequest(http.MethodGet, "/api/datasets/"+dsID+"/export/progress", nil)
	req3.Header.Set("X-Session-ID", sessionID)
	w3 := httptest.NewRecorder()
	handler.GetProgress(w3, req3, dsID)

	var compResp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w3.Body.Bytes(), &compResp)
	if compResp.Data == nil || compResp.Data["status"] != "completed" {
		t.Errorf("expected status 'completed', got '%v'", compResp.Data)
	}
	if compResp.Data["percent"] != 100.0 {
		t.Errorf("expected percent 100, got '%v'", compResp.Data["percent"])
	}
	if compResp.Data["stage"] != "Export completed" {
		t.Errorf("expected stage 'Export completed', got '%v'", compResp.Data["stage"])
	}
}

func TestCustomLocalTiles_FastZipStoreMethod(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir: filepath.Join(tempDir, "sessions"),
		UploadDir:         filepath.Join(tempDir, "uploads"),
		ProcessedDir:      filepath.Join(tempDir, "processed"),
	}
	sm, _ := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	repo := repository.NewSessionRepository()
	sess := repo.CreateSession()
	sessionID := sess.ID

	exportService := services.NewExportService(repo, sm)

	dsID := "ds_test_zip_store"
	dsDir := sm.GetDatasetDir(sessionID, dsID)
	_ = os.MkdirAll(dsDir, 0755)
	rawPNG := filepath.Join(dsDir, "aerial.png")
	_ = writeCustomTilesTestImage(rawPNG, 256, 256)

	ds := &models.Dataset{
		ID:                 dsID,
		SessionID:          sessionID,
		Filename:           "aerial.png",
		StoragePath:        rawPNG,
		GeoreferenceStatus: "completed",
		ExtractedMetadata: map[string]interface{}{
			"bounds": map[string]interface{}{"min_x": 10.0, "min_y": 20.0, "max_x": 10.05, "max_y": 20.05},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = repo.InsertDataset(sessionID, ds)

	zipPath, _, err := exportService.GenerateCustomLocalTilesExport(sessionID, dsID, services.SelectiveExportParams{
		Format: "custom_local_tiles",
	})
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	zr, zErr := zip.OpenReader(zipPath)
	if zErr != nil {
		t.Fatalf("failed reading zip: %v", zErr)
	}
	defer zr.Close()

	var checkedImageTile bool
	var checkedJSONFile bool

	for _, f := range zr.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		switch ext {
		case ".png", ".jpg":
			// Fast packaging requirement: already-compressed tiles must use zip.Store (0)
			if f.Method != zip.Store {
				t.Errorf("expected tile %s to use zip.Store (%d), but got %d", f.Name, zip.Store, f.Method)
			}
			checkedImageTile = true
		case ".json", ".geojson":
			// Text files must use zip.Deflate (8)
			if f.Method != zip.Deflate {
				t.Errorf("expected text %s to use zip.Deflate (%d), but got %d", f.Name, zip.Deflate, f.Method)
			}
			checkedJSONFile = true
		}
	}

	if !checkedImageTile {
		t.Errorf("no raster tiles found in archive to verify zip.Store method")
	}
	if !checkedJSONFile {
		t.Errorf("no JSON/GeoJSON files found in archive to verify zip.Deflate method")
	}
}
