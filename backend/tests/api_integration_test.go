package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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

func createTestServer(t *testing.T) (*httptest.Server, *repository.SessionRepository, string) {
	tempDir, err := os.MkdirTemp("", "maptiler-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		Port:               "0",
		Host:               "127.0.0.1",
		SessionStorageDir:  filepath.Join(tempDir, "sessions"),
		UploadDir:          filepath.Join(tempDir, "uploads"),
		ProcessedDir:       filepath.Join(tempDir, "processed"),
		MaxUploadSizeBytes: 50 * 1024 * 1024,
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

	uh := upload.NewHandler(dsService, repo)
	dh := datasets.NewHandler(dsService, repo)
	gh := georeference.NewHandler(geoService, repo)
	eh := export.NewHandler(expService, repo)

	router := api.NewRouter(repo, mw, uh, dh, gh, eh)
	server := httptest.NewServer(router.BuildHandler())

	return server, repo, tempDir
}

func createTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestAPI_CompleteWorkflow(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Health
	resp, err := client.Get(server.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("health check failed: %v, status: %d", err, resp.StatusCode)
	}

	// 2. Create Session
	resp, err = client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("session creation failed: %v, status: %d", err, resp.StatusCode)
	}
	var sessResult struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessResult)
	sessionID := sessResult.Data.SessionID
	if sessionID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	// 3. Upload PNG
	pngBytes := createTestPNG(200, 200)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test_raster.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %v, status: %d", err, resp.StatusCode)
	}

	var uploadResult struct {
		Data struct {
			ID              string `json:"id"`
			IsGeoreferenced bool   `json:"is_georeferenced"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResult)
	datasetID := uploadResult.Data.ID
	if datasetID == "" {
		t.Fatalf("expected dataset ID from upload")
	}
	if uploadResult.Data.IsGeoreferenced {
		t.Fatalf("raw PNG should not be georeferenced initially")
	}

	// 4. Source Image
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/source-image", nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("source image get failed: %v, status: %d", err, resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png content type, got %s", resp.Header.Get("Content-Type"))
	}

	// 5. Add Control Points
	points := []map[string]float64{
		{"image_x": 20, "image_y": 20, "longitude": -74.0, "latitude": 40.8},
		{"image_x": 180, "image_y": 20, "longitude": -73.8, "latitude": 40.8},
		{"image_x": 180, "image_y": 180, "longitude": -73.8, "latitude": 40.6},
		{"image_x": 20, "image_y": 180, "longitude": -74.0, "latitude": 40.6},
	}
	for _, pt := range points {
		ptBytes, _ := json.Marshal(pt)
		req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/control-points", bytes.NewReader(ptBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", sessionID)
		resp, err = client.Do(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed adding control point: %v, status: %d", err, resp.StatusCode)
		}
	}

	// 6. Georeference
	geoReqBytes, _ := json.Marshal(map[string]string{"action": "calculate", "transformation": "affine"})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/georeference", bytes.NewReader(geoReqBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("georeference failed: %v, status: %d", err, resp.StatusCode)
	}

	// 7. Check Layers
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/layers", nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("layers check failed: %v, status: %d", err, resp.StatusCode)
	}

	// 8. Export MTMAP ZIP
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/export", nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("export failed: %v, status: %d", err, resp.StatusCode)
	}
	exportBytes, _ := io.ReadAll(resp.Body)
	zipReader, err := zip.NewReader(bytes.NewReader(exportBytes), int64(len(exportBytes)))
	if err != nil {
		t.Fatalf("exported file is not a valid ZIP: %v", err)
	}
	foundMetadata := false
	for _, f := range zipReader.File {
		if f.Name == "metadata.json" {
			foundMetadata = true
		}
	}
	if !foundMetadata {
		t.Fatalf("export ZIP does not contain metadata.json")
	}

	// 9. Delete Dataset
	req, _ = http.NewRequest("DELETE", server.URL+"/datasets/"+datasetID, nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("delete failed: %v, status: %d", err, resp.StatusCode)
	}

	// 10. Verify Deleted
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID, nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted dataset, got: %d", resp.StatusCode)
	}
}

func TestAPI_ZipUploadWithSidecars(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// Create zip with PNG + .pgw + .prj
	zipBuf := &bytes.Buffer{}
	zipWriter := zip.NewWriter(zipBuf)

	// Image
	imgEntry, _ := zipWriter.Create("plan.png")
	_, _ = imgEntry.Write(createTestPNG(100, 100))

	// World file
	pgwEntry, _ := zipWriter.Create("plan.pgw")
	_, _ = pgwEntry.Write([]byte("0.001\n0.0\n0.0\n-0.001\n-74.0\n40.8\n"))

	// Projection
	prjEntry, _ := zipWriter.Create("plan.prj")
	_, _ = prjEntry.Write([]byte("EPSG:4326"))

	_ = zipWriter.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "plan_bundle.zip")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("zip upload failed: %v, status: %d", err, resp.StatusCode)
	}

	var uploadResult struct {
		Data struct {
			ID              string `json:"id"`
			IsGeoreferenced bool   `json:"is_georeferenced"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResult)
	if !uploadResult.Data.IsGeoreferenced {
		t.Fatalf("bundle with .pgw and .prj should be detected as georeferenced")
	}
}

func TestAPI_ZipSlipRejection(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// Create zip with malicious traversal path
	zipBuf := &bytes.Buffer{}
	zipWriter := zip.NewWriter(zipBuf)
	evilEntry, _ := zipWriter.Create("../../../evil.png")
	_, _ = evilEntry.Write([]byte("malicious"))
	_ = zipWriter.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "evil.zip")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for zip-slip, got %d", resp.StatusCode)
	}
}
