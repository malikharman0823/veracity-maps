package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

// Helper to set up a test environment
func setupZoomPointsTestEnv(t *testing.T) (*repository.SessionRepository, *storage.StorageManager, http.Handler, string) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir:  tempDir + "/sessions",
		UploadDir:          tempDir + "/uploads",
		ProcessedDir:       tempDir + "/processed",
		MaxUploadSizeBytes: 50 * 1024 * 1024,
	}

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("failed creating storage manager: %v", err)
	}

	repo := repository.NewSessionRepository()
	repo.SetStorageDir(cfg.SessionStorageDir)
	mw := api.NewMiddleware(repo)

	dsService := services.NewDatasetService(cfg, repo, sm)
	geoService := services.NewGeoreferenceService(repo, sm)
	expService := services.NewExportService(repo, sm)

	uHandler := upload.NewHandler(dsService, repo, sm)
	dHandler := datasets.NewHandler(dsService, repo)
	gHandler := georeference.NewHandler(geoService, repo)
	eHandler := export.NewHandler(expService, repo)

	router := api.NewRouter(repo, mw, uHandler, dHandler, gHandler, eHandler)
	handler := router.BuildHandler()

	sess := repo.CreateSession()
	return repo, sm, handler, sess.ID
}

func createTestPNGImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8((x * 255) / width), G: uint8((y * 255) / height), B: 150, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// TestExport_ZoomConfigurationDecoding tests the ZoomConfiguration parameters and validation
func TestExport_ZoomConfigurationDecoding(t *testing.T) {
	jsonPayload := `{
		"format": "mtmap",
		"project_name": "Test Zoom Project",
		"options": {
			"include_control_points": true,
			"include_icons": true,
			"zoom_config": {
				"initial_zoom": 18,
				"min_zoom": 12,
				"max_zoom": 21,
				"preset": "neighborhood"
			}
		}
	}`

	var req export.ExportRequest
	if err := json.Unmarshal([]byte(jsonPayload), &req); err != nil {
		t.Fatalf("failed decoding ExportRequest: %v", err)
	}

	if req.Options == nil || req.Options.ZoomConfig == nil {
		t.Fatalf("expected ZoomConfig to be populated")
	}

	zc := req.Options.ZoomConfig
	if zc.InitialZoom == nil || *zc.InitialZoom != 18 {
		t.Errorf("expected initial_zoom 18, got %v", zc.InitialZoom)
	}
	if zc.MinZoom == nil || *zc.MinZoom != 12 {
		t.Errorf("expected min_zoom 12, got %v", zc.MinZoom)
	}
	if zc.MaxZoom == nil || *zc.MaxZoom != 21 {
		t.Errorf("expected max_zoom 21, got %v", zc.MaxZoom)
	}
	if zc.Preset != "neighborhood" {
		t.Errorf("expected preset 'neighborhood', got '%s'", zc.Preset)
	}
}

// TestExport_IconAssetBundlingAndZoomViewscape verifies full ZIP generation with bundled custom icons,
// separate GeoJSON point layers, and custom camera zoom limits in Viewscape.
func TestExport_IconAssetBundlingAndZoomViewscape(t *testing.T) {
	repo, _, handler, sessionID := setupZoomPointsTestEnv(t)

	// 1. Upload a test raster map
	imgBytes := createTestPNGImage(400, 300)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "campus_map.png")
	_, _ = part.Write(imgBytes)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("upload failed with status %d: %s", w.Code, w.Body.String())
	}

	var uploadRes struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(w.Body).Decode(&uploadRes)
	dsID := uploadRes.Data.ID

	// 2. Add ground control points
	gcpCoords := []struct {
		ix, iy   float64
		lng, lat float64
	}{
		{0, 0, -73.9855, 40.7488},
		{399, 0, -73.9845, 40.7488},
		{399, 299, -73.9845, 40.7480},
		{0, 299, -73.9855, 40.7480},
	}
	for _, pt := range gcpCoords {
		pBody := fmt.Sprintf(`{"image_x": %f, "image_y": %f, "longitude": %f, "latitude": %f}`, pt.ix, pt.iy, pt.lng, pt.lat)
		cpReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/datasets/%s/control-points", dsID), strings.NewReader(pBody))
		cpReq.Header.Set("Content-Type", "application/json")
		cpReq.Header.Set("X-Session-ID", sessionID)
		wCP := httptest.NewRecorder()
		handler.ServeHTTP(wCP, cpReq)
		if wCP.Code != http.StatusCreated {
			t.Fatalf("create control point failed: %s", wCP.Body.String())
		}
	}

	// 3. Georeference dataset
	geoReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/datasets/%s/georeference", dsID), strings.NewReader(`{}`))
	geoReq.Header.Set("Content-Type", "application/json")
	geoReq.Header.Set("X-Session-ID", sessionID)
	wGeo := httptest.NewRecorder()
	handler.ServeHTTP(wGeo, geoReq)
	if wGeo.Code != http.StatusOK {
		t.Fatalf("georeference failed: %s", wGeo.Body.String())
	}

	// 4. Add custom icon points
	iconPoints := []struct {
		iconID   string
		desc     string
		lng, lat float64
	}{
		{"001-camera.png", "Security Camera NW", -73.9852, 40.7485},
		{"emergency_exit.png", "East Fire Exit", -73.9847, 40.7482},
	}
	for _, ip := range iconPoints {
		ipPayload := fmt.Sprintf(`{"icon_id": "%s", "description": "%s", "longitude": %f, "latitude": %f, "scope": "building"}`,
			ip.iconID, ip.desc, ip.lng, ip.lat)
		ipReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/datasets/%s/icon-points", dsID), strings.NewReader(ipPayload))
		ipReq.Header.Set("Content-Type", "application/json")
		ipReq.Header.Set("X-Session-ID", sessionID)
		wIP := httptest.NewRecorder()
		handler.ServeHTTP(wIP, ipReq)
		if wIP.Code != http.StatusCreated {
			t.Fatalf("create icon point failed: %s", wIP.Body.String())
		}
	}

	// 5. Export with Zoom Configuration
	exportPayload := `{
		"format": "mtmap",
		"project_name": "Campus Security Map",
		"layer_ids": ["` + dsID + `"],
		"options": {
			"include_control_points": true,
			"include_icons": true,
			"include_original_rasters": true,
			"include_processed_rasters": true,
			"zoom_config": {
				"initial_zoom": 19,
				"min_zoom": 14,
				"max_zoom": 22,
				"preset": "street"
			}
		}
	}`

	expReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/datasets/%s/export", dsID), strings.NewReader(exportPayload))
	expReq.Header.Set("Content-Type", "application/json")
	expReq.Header.Set("Accept", "application/zip")
	expReq.Header.Set("X-Session-ID", sessionID)
	wExp := httptest.NewRecorder()
	handler.ServeHTTP(wExp, expReq)

	if wExp.Code != http.StatusOK {
		t.Fatalf("export failed with code %d: %s", wExp.Code, wExp.Body.String())
	}

	// 6. Inspect ZIP contents
	zipBytes := wExp.Body.Bytes()
	zipR, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("failed reading export zip: %v", err)
	}

	files := make(map[string]*zip.File)
	for _, f := range zipR.File {
		files[f.Name] = f
	}

	// Verify required root files
	requiredFiles := []string{
		"index.html",
		"viewer.html",
		"style.json",
		"mapstudio.json",
		"metadata.json",
		"README.txt",
	}
	for _, rf := range requiredFiles {
		if _, ok := files[rf]; !ok {
			t.Errorf("missing required export file: %s", rf)
		}
	}

	// Verify overview zoom 7 tiles exist for regional and Viewscape viewports
	hasZoom7 := false
	for name := range files {
		if strings.HasPrefix(name, "7/") && strings.HasSuffix(name, ".png") {
			hasZoom7 = true
			break
		}
	}
	if !hasZoom7 {
		t.Errorf("expected zoom 7 overview tiles for Viewscape default position compatibility")
	}

	// Verify metadata.json contains zoom settings, control points, and icon points
	if metaF, ok := files["metadata.json"]; ok {
		rc, _ := metaF.Open()
		var metaDoc map[string]interface{}
		_ = json.NewDecoder(rc).Decode(&metaDoc)
		_ = rc.Close()

		if initZ, ok := metaDoc["initial_zoom"].(float64); !ok || int(initZ) != 19 {
			t.Errorf("expected metadata.json initial_zoom 19, got %v", metaDoc["initial_zoom"])
		}
		if minZ, ok := metaDoc["minzoom"]; !ok || fmt.Sprintf("%v", minZ) != "14" {
			t.Errorf("expected metadata.json minzoom 14, got %v", minZ)
		}
		if maxZ, ok := metaDoc["maxzoom"]; !ok || fmt.Sprintf("%v", maxZ) != "22" {
			t.Errorf("expected metadata.json maxzoom 22, got %v", maxZ)
		}
		if preset, ok := metaDoc["zoom_preset"].(string); !ok || preset != "street" {
			t.Errorf("expected metadata.json zoom_preset 'street', got %v", metaDoc["zoom_preset"])
		}

		// Check control points in metadata
		if cps, ok := metaDoc["control_points"].([]interface{}); !ok || len(cps) != 4 {
			t.Errorf("expected 4 control points in metadata.json, got %v", metaDoc["control_points"])
		}
		// Check icon points in metadata
		if ips, ok := metaDoc["icon_points"].([]interface{}); !ok || len(ips) != 2 {
			t.Errorf("expected 2 icon points in metadata.json, got %v", metaDoc["icon_points"])
		}
	}

	// Verify viewer.html contains MapLibre GL setup, configured zooms, and point layers
	if viewerF, ok := files["viewer.html"]; ok {
		rc, _ := viewerF.Open()
		contentBytes, _ := io.ReadAll(rc)
		_ = rc.Close()
		content := string(contentBytes)

		// Must contain MapLibre
		if !strings.Contains(content, "maplibregl") {
			t.Errorf("viewer.html does not contain maplibregl")
		}
		// Must configure initial zoom 19
		if !strings.Contains(content, "zoom: 19") && !strings.Contains(content, "initialZoom = 19") {
			t.Errorf("viewer.html does not configure initialZoom 19")
		}
		// Must contain separate control-points and icon-points layers
		if !strings.Contains(content, "control-points-layer") {
			t.Errorf("viewer.html missing control-points-layer")
		}
		if !strings.Contains(content, "icon-points-layer") {
			t.Errorf("viewer.html missing icon-points-layer")
		}
		// Must contain visibility toggle controls for both
		if !strings.Contains(content, "control-points-toggle") && !strings.Contains(content, "Control Points Visible") {
			t.Errorf("viewer.html missing control points visibility toggle")
		}
		if !strings.Contains(content, "icon-points-toggle") && !strings.Contains(content, "Icon Points Visible") {
			t.Errorf("viewer.html missing icon points visibility toggle")
		}
		// Must contain interactive popups
		if !strings.Contains(content, "maplibregl.Popup") {
			t.Errorf("viewer.html missing MapLibre Popup support")
		}
		// Must not contain forbidden legacy libraries
		for _, bad := range []string{"leaflet.js", "L.map", "L.tileLayer", "google.maps", "ol.layer"} {
			if strings.Contains(content, bad) {
				t.Errorf("viewer.html contains forbidden legacy code '%s'", bad)
			}
		}
	}

	// 7. Verify Roundtrip Re-Import Preserves Zoom and Points
	newSess := repo.CreateSession()
	var importBody bytes.Buffer
	impWriter := multipart.NewWriter(&importBody)
	impPart, _ := impWriter.CreateFormFile("file", "exported_project.zip")
	_, _ = impPart.Write(zipBytes)
	_ = impWriter.Close()

	impReq := httptest.NewRequest(http.MethodPost, "/upload", &importBody)
	impReq.Header.Set("Content-Type", impWriter.FormDataContentType())
	impReq.Header.Set("X-Session-ID", newSess.ID)
	wImp := httptest.NewRecorder()
	handler.ServeHTTP(wImp, impReq)

	if wImp.Code != http.StatusCreated {
		t.Fatalf("reopening exported zip failed: %s", wImp.Body.String())
	}

	var impRes struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wImp.Body).Decode(&impRes)
	reopenedDsID := impRes.Data.ID

	reopenedDs, err := repo.GetDataset(newSess.ID, reopenedDsID)
	if err != nil {
		t.Fatalf("reopened dataset not found: %v", err)
	}

	// Verify restored zoom configuration in extracted metadata
	if initZ, ok := reopenedDs.ExtractedMetadata["initial_zoom"].(int); !ok || initZ != 19 {
		// It might be float64 or int
		if fz, ok := reopenedDs.ExtractedMetadata["initial_zoom"].(float64); !ok || int(fz) != 19 {
			t.Errorf("reopened dataset initial_zoom != 19: %v", reopenedDs.ExtractedMetadata["initial_zoom"])
		}
	}

	// Verify restored control points
	restoredCPs, _ := repo.ListControlPoints(newSess.ID, reopenedDsID)
	if len(restoredCPs) != 4 {
		t.Errorf("expected 4 restored control points, got %d", len(restoredCPs))
	}

	// Verify restored icon points
	restoredIPs, _ := repo.ListIconPoints(newSess.ID, reopenedDsID, "")
	if len(restoredIPs) != 2 {
		t.Errorf("expected 2 restored icon points, got %d", len(restoredIPs))
	}
}

// TestExport_InvalidZoomConfigurationNormalization tests normalization of invalid/inverted zoom ranges
func TestExport_InvalidZoomConfigurationNormalization(t *testing.T) {
	_, _, handler, sessionID := setupZoomPointsTestEnv(t)

	imgBytes := createTestPNGImage(200, 200)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "test_plan.png")
	_, _ = part.Write(imgBytes)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var uploadRes struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(w.Body).Decode(&uploadRes)
	dsID := uploadRes.Data.ID

	// Inverted min/max (min=22, max=10) and out-of-range initial (30)
	exportPayload := `{
		"format": "mtmap",
		"project_name": "Inverted Zooms Project",
		"layer_ids": ["` + dsID + `"],
		"options": {
			"include_original_rasters": true,
			"zoom_config": {
				"initial_zoom": 30,
				"min_zoom": 22,
				"max_zoom": 10
			}
		}
	}`

	expReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/datasets/%s/export", dsID), strings.NewReader(exportPayload))
	expReq.Header.Set("Content-Type", "application/json")
	expReq.Header.Set("Accept", "application/zip")
	expReq.Header.Set("X-Session-ID", sessionID)
	wExp := httptest.NewRecorder()
	handler.ServeHTTP(wExp, expReq)

	if wExp.Code != http.StatusOK {
		t.Fatalf("export failed: %s", wExp.Body.String())
	}

	zipBytes := wExp.Body.Bytes()
	zipR, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("failed reading zip: %v", err)
	}

	var metaF *zip.File
	for _, f := range zipR.File {
		if f.Name == "metadata.json" {
			metaF = f
			break
		}
	}
	if metaF == nil {
		t.Fatalf("metadata.json missing")
	}

	rc, _ := metaF.Open()
	var metaDoc map[string]interface{}
	_ = json.NewDecoder(rc).Decode(&metaDoc)
	_ = rc.Close()

	// Min and Max should be swapped/normalized so min <= max
	minZ := int(metaDoc["minzoom"].(float64))
	maxZ := int(metaDoc["maxzoom"].(float64))
	initZ := int(metaDoc["initial_zoom"].(float64))

	if minZ > maxZ {
		t.Errorf("expected minZoom <= maxZoom, got min=%d max=%d", minZ, maxZ)
	}
	if initZ < minZ || initZ > maxZ {
		t.Errorf("expected initial_zoom inside [%d, %d], got %d", minZ, maxZ, initZ)
	}
}

// TestExport_LegacyZipWithoutZoomSettings ensures older exported ZIPs without zoom settings
// import cleanly without crashes and receive sensible defaults.
func TestExport_LegacyZipWithoutZoomSettings(t *testing.T) {
	repo, _, handler, sessionID := setupZoomPointsTestEnv(t)

	// Create a minimal legacy zip in-memory containing only image and basic metadata.json (no zoom settings)
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	imgBytes := createTestPNGImage(100, 100)
	iw, _ := zw.Create("legacy_map.png")
	_, _ = iw.Write(imgBytes)

	legacyMeta := map[string]interface{}{
		"name":    "Legacy Project",
		"bounds":  []float64{-73.99, 40.74, -73.98, 40.75},
		"profile": "mercator",
	}
	mw, _ := zw.Create("metadata.json")
	mBytes, _ := json.Marshal(legacyMeta)
	_, _ = mw.Write(mBytes)
	_ = zw.Close()

	var importBody bytes.Buffer
	impWriter := multipart.NewWriter(&importBody)
	impPart, _ := impWriter.CreateFormFile("file", "legacy_project.zip")
	_, _ = impPart.Write(zipBuf.Bytes())
	_ = impWriter.Close()

	impReq := httptest.NewRequest(http.MethodPost, "/upload", &importBody)
	impReq.Header.Set("Content-Type", impWriter.FormDataContentType())
	impReq.Header.Set("X-Session-ID", sessionID)
	wImp := httptest.NewRecorder()
	handler.ServeHTTP(wImp, impReq)

	if wImp.Code != http.StatusCreated {
		t.Fatalf("import legacy zip failed: %s", wImp.Body.String())
	}

	var impRes struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wImp.Body).Decode(&impRes)
	ds, err := repo.GetDataset(sessionID, impRes.Data.ID)
	if err != nil {
		t.Fatalf("imported dataset not found: %v", err)
	}

	// Georeference status should be completed because bounds existed
	if ds.GeoreferenceStatus != "completed" {
		t.Errorf("expected completed georeference status, got %s", ds.GeoreferenceStatus)
	}
}
