package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func setupMatrixTestServer(t *testing.T) (*httptest.Server, *repository.SessionRepository, string) {
	tempDir, err := os.MkdirTemp("", "maptiler-matrix-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		Port:               "0",
		Host:               "127.0.0.1",
		SessionStorageDir:  filepath.Join(tempDir, "sessions"),
		UploadDir:          filepath.Join(tempDir, "uploads"),
		ProcessedDir:       filepath.Join(tempDir, "processed"),
		MaxUploadSizeBytes: 10 * 1024 * 1024,
	}

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("failed to create storage manager: %v", err)
	}

	repo := repository.NewSessionRepository()
	mw := api.NewMiddleware(repo)

	dsService := services.NewDatasetService(cfg, repo, sm)
	geoService := services.NewGeoreferenceService(repo, sm)
	expService := services.NewExportService(repo, sm)

	uh := upload.NewHandler(dsService, repo, sm)
	dh := datasets.NewHandler(dsService, repo)
	gh := georeference.NewHandler(geoService, repo)
	eh := export.NewHandler(expService, repo)

	router := api.NewRouter(repo, mw, uh, dh, gh, eh)
	server := httptest.NewServer(router.BuildHandler())

	return server, repo, tempDir
}

func uploadFixtureFile(t *testing.T, serverURL, sessionID, fixturePath string) (*http.Response, map[string]interface{}) {
	fileData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed reading fixture file %s: %v", fixturePath, err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(fixturePath))
	if err != nil {
		t.Fatalf("failed creating form file: %v", err)
	}
	_, _ = part.Write(fileData)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", serverURL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if sessionID != "" {
		req.Header.Set("X-Session-ID", sessionID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed executing upload request: %v", err)
	}

	var jsonResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&jsonResp)
	return resp, jsonResp
}

func TestAPI_FullMatrixCoverage(t *testing.T) {
	server, repo, tempDir := setupMatrixTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	fixtureBase := filepath.Join("..", "..", "testdata", "maptiler-mvp")
	if _, err := os.Stat(fixtureBase); os.IsNotExist(err) {
		fixtureBase = filepath.Join("..", "testdata", "maptiler-mvp")
	}

	client := server.Client()

	// 1. Health Endpoints
	for _, path := range []string{"/health", "/api/health"} {
		resp, err := client.Get(server.URL + path)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s failed: %v", path, err)
		}
	}

	// 2. Session Management
	resp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create session failed: status %d", resp.StatusCode)
	}
	var sessData struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessData)
	sessionID1 := sessData.Data.SessionID
	if sessionID1 == "" {
		t.Fatalf("empty sessionID1")
	}

	// Create second session for isolation testing
	resp, _ = client.Post(server.URL+"/sessions", "application/json", nil)
	var sessData2 struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessData2)
	sessionID2 := sessData2.Data.SessionID

	// Get Session
	req, _ := http.NewRequest("GET", server.URL+"/sessions/"+sessionID1, nil)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get session failed: status %d", resp.StatusCode)
	}

	// Get Current Session
	req, _ = http.NewRequest("GET", server.URL+"/sessions/current", nil)
	req.Header.Set("X-Session-ID", sessionID1)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get current session failed: status %d", resp.StatusCode)
	}

	// Map Config PUT and GET
	mapConfig := map[string]interface{}{"zoom": 12, "center": []float64{-74.0, 40.7}}
	configBytes, _ := json.Marshal(mapConfig)
	req, _ = http.NewRequest("PUT", server.URL+"/sessions/"+sessionID1+"/map-config", bytes.NewReader(configBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("put map-config failed: status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest("GET", server.URL+"/sessions/"+sessionID1+"/map-config", nil)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get map-config failed: status %d", resp.StatusCode)
	}

	// 3. Fixture Uploads Matrix
	// A: Ordinary PNG
	resp, jsonResp := uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "ordinary_grid.png"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload ordinary_grid.png failed: %d (%v)", resp.StatusCode, jsonResp)
	}
	dataMap, _ := jsonResp["data"].(map[string]interface{})
	datasetID := dataMap["id"].(string)

	// B: Ordinary JPEG
	resp, jsonResp = uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "ordinary_photo.jpg"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload ordinary_photo.jpg failed: %d (%v)", resp.StatusCode, jsonResp)
	}

	// C: Valid Bundle ZIP
	resp, jsonResp = uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "valid_bundle.zip"))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload valid_bundle.zip failed: %d (%v)", resp.StatusCode, jsonResp)
	}
	bundleData := jsonResp["data"].(map[string]interface{})
	if isGeo, ok := bundleData["is_georeferenced"].(bool); !ok || !isGeo {
		t.Fatalf("expected valid_bundle.zip to be georeferenced")
	}

	// D: Negative: Malicious Zip-Slip
	resp, _ = uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "zip_slip_malicious.zip"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for zip_slip_malicious.zip, got %d", resp.StatusCode)
	}

	// E: Negative: Truncated PNG
	resp, _ = uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "truncated.png"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for truncated.png, got %d", resp.StatusCode)
	}

	// F: Negative: Text pretending to be PNG
	resp, _ = uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "text_as_png.png"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for text_as_png.png, got %d", resp.StatusCode)
	}

	// G: Negative: Zero byte file
	resp, _ = uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "zero_byte.png"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for zero_byte.png, got %d", resp.StatusCode)
	}

	// H: Negative: Unsupported extension
	resp, _ = uploadFixtureFile(t, server.URL, sessionID1, filepath.Join(fixtureBase, "unsupported.xyz"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported.xyz, got %d", resp.StatusCode)
	}

	// 4. Dataset Listing & Cross-Session Isolation
	req, _ = http.NewRequest("GET", server.URL+"/datasets", nil)
	req.Header.Set("X-Session-ID", sessionID1)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("list datasets failed: %v", err)
	}

	// Session 2 listing must NOT contain Session 1's dataset
	req, _ = http.NewRequest("GET", server.URL+"/datasets", nil)
	req.Header.Set("X-Session-ID", sessionID2)
	resp, err = client.Do(req)
	var listResp2 struct {
		Data struct {
			Datasets []interface{} `json:"datasets"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&listResp2)
	if len(listResp2.Data.Datasets) != 0 {
		t.Fatalf("session isolation violation: session 2 sees %d datasets", len(listResp2.Data.Datasets))
	}

	// Session 2 attempting to access Session 1's dataset ID must return 404
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID, nil)
	req.Header.Set("X-Session-ID", sessionID2)
	resp, err = client.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 cross-session dataset access, got %d", resp.StatusCode)
	}

	// 5. Source Image Streaming
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/source-image", nil)
	req.Header.Set("X-Session-ID", sessionID1)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("source image get failed: %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png content type, got %s", resp.Header.Get("Content-Type"))
	}

	// 6. Control Points Matrix
	// Negative: Out of bounds point
	badPt := map[string]interface{}{"image_x": 99999.0, "image_y": 10.0, "longitude": -74.0, "latitude": 40.7}
	badPtBytes, _ := json.Marshal(badPt)
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/control-points", bytes.NewReader(badPtBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID1)
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for out-of-bounds control point, got %d", resp.StatusCode)
	}

	// Add 4 valid affine points from fixture
	cpFixturePath := filepath.Join(fixtureBase, "control_points_fixture.json")
	cpDataBytes, _ := os.ReadFile(cpFixturePath)
	var cpDoc struct {
		ValidAffinePoints []map[string]interface{} `json:"valid_affine_points"`
	}
	_ = json.Unmarshal(cpDataBytes, &cpDoc)

	var pointIDs []string
	for _, pt := range cpDoc.ValidAffinePoints {
		ptBytes, _ := json.Marshal(pt)
		req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/control-points", bytes.NewReader(ptBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", sessionID1)
		resp, err = client.Do(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed inserting control point: %d", resp.StatusCode)
		}
		var ptResp struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&ptResp)
		pointIDs = append(pointIDs, ptResp.Data.ID)
	}

	// Update control point
	if len(pointIDs) > 0 {
		updatePt := map[string]interface{}{"image_x": 55.0, "image_y": 55.0, "longitude": -73.979, "latitude": 40.759}
		upBytes, _ := json.Marshal(updatePt)
		req, _ = http.NewRequest("PUT", server.URL+"/control-points/"+pointIDs[0], bytes.NewReader(upBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", sessionID1)
		resp, err = client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("update control point failed: %d", resp.StatusCode)
		}
	}

	// 7. Georeference Calculation
	geoReq := map[string]string{"action": "calculate", "transformation": "affine"}
	geoBytes, _ := json.Marshal(geoReq)
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/georeference", bytes.NewReader(geoBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID1)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("georeference calculation failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// 8. Layers & Layer Update
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/layers", nil)
	req.Header.Set("X-Session-ID", sessionID1)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get layers failed: %d", resp.StatusCode)
	}
	var layerResp struct {
		Data struct {
			Layers []struct {
				ID string `json:"id"`
			} `json:"layers"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&layerResp)
	if len(layerResp.Data.Layers) > 0 {
		lyrID := layerResp.Data.Layers[0].ID
		patchData := map[string]interface{}{"visible": false, "opacity": 0.5}
		patchBytes, _ := json.Marshal(patchData)
		req, _ = http.NewRequest("PATCH", server.URL+"/datasets/"+datasetID+"/layers/"+lyrID, bytes.NewReader(patchBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", sessionID1)
		resp, err = client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("patch layer failed: %d", resp.StatusCode)
		}
	}

	// 9. Export Generation & Download
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/export", nil)
	req.Header.Set("X-Session-ID", sessionID1)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("export GET failed: %d", resp.StatusCode)
	}
	exportZipBytes, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(exportZipBytes), int64(len(exportZipBytes)))
	if err != nil {
		t.Fatalf("export is not a valid zip: %v", err)
	}
	hasMeta, hasViewer := false, false
	for _, f := range zr.File {
		if f.Name == "metadata.json" {
			hasMeta = true
		}
		if f.Name == "leaflet.html" {
			hasViewer = true
		}
	}
	if !hasMeta || !hasViewer {
		t.Fatalf("export zip missing metadata.json or leaflet.html")
	}

	// 10. Deletion Lifecycle
	req, _ = http.NewRequest("DELETE", server.URL+"/datasets/"+datasetID, nil)
	req.Header.Set("X-Session-ID", sessionID1)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("delete dataset failed: %d", resp.StatusCode)
	}

	// Subsequent GET must return 404
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID, nil)
	req.Header.Set("X-Session-ID", sessionID1)
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after deletion, got %d", resp.StatusCode)
	}

	// End Session
	req, _ = http.NewRequest("DELETE", server.URL+"/sessions/"+sessionID1, nil)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("delete session failed: %d", resp.StatusCode)
	}
	if _, err := repo.GetSession(sessionID1); err != repository.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound for ended session")
	}
}
