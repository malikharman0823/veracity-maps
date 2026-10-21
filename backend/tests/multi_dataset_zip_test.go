package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
)

func TestAPI_MultiDatasetZipInspectionAndImport(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Create session
	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed creating session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 2. Generate multi-dataset ZIP in memory
	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)

	// Helper to write a small PNG to zip
	writeZipImage := func(entryName string, w, h int) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 50, G: 100, B: 200, A: 255})
			}
		}
		f, _ := zw.Create(entryName)
		_ = png.Encode(f, img)
	}

	writeZipString := func(entryName, content string) {
		f, _ := zw.Create(entryName)
		_, _ = f.Write([]byte(content))
	}

	// Dataset 1: building-a/ground-floor.png + .pgw
	writeZipImage("campus/building-a/ground-floor.png", 100, 100)
	writeZipString("campus/building-a/ground-floor.pgw", "0.0001\n0.0\n0.0\n-0.0001\n-122.4194\n37.7749\n")

	// Dataset 2: building-a/first-floor.png (no sidecar)
	writeZipImage("campus/building-a/first-floor.png", 120, 120)

	// Dataset 3: building-b/basement.png + .pgw
	writeZipImage("campus/building-b/basement.png", 80, 80)
	writeZipString("campus/building-b/basement.pgw", "0.0002\n0.0\n0.0\n-0.0002\n-122.4200\n37.7750\n")

	// Unsupported file to ensure it's skipped
	writeZipString("campus/notes.txt", "Campus architectural reference notes")

	_ = zw.Close()

	// 3. Test POST /datasets/inspect-archive
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "campus_bundle.zip")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	inspectReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/inspect-archive", body)
	inspectReq.Header.Set("Content-Type", writer.FormDataContentType())
	inspectReq.Header.Set("X-Session-ID", sessionID)

	inspectResp, err := client.Do(inspectReq)
	if err != nil {
		t.Fatalf("failed calling /datasets/inspect-archive: %v", err)
	}
	defer inspectResp.Body.Close()
	if inspectResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from inspect-archive, got %d", inspectResp.StatusCode)
	}

	var inspectDoc struct {
		Data struct {
			ArchiveToken string `json:"archive_token"`
			ArchiveName  string `json:"archive_name"`
			TotalRasters int    `json:"total_rasters"`
			Datasets     []struct {
				EntryPath         string   `json:"entry_path"`
				Name              string   `json:"name"`
				Format            string   `json:"format"`
				Width             int      `json:"width"`
				Height            int      `json:"height"`
				IsGeoreferenced   bool     `json:"is_georeferenced"`
				Sidecars          []string `json:"sidecars"`
				SuggestedBuilding string   `json:"suggested_building"`
				SuggestedFloor    int      `json:"suggested_floor"`
			} `json:"datasets"`
		} `json:"data"`
	}

	if err := json.NewDecoder(inspectResp.Body).Decode(&inspectDoc); err != nil {
		t.Fatalf("failed decoding inspect response: %v", err)
	}

	token := inspectDoc.Data.ArchiveToken
	if token == "" {
		t.Fatalf("expected non-empty archive token")
	}
	if inspectDoc.Data.TotalRasters != 3 {
		t.Fatalf("expected 3 rasters detected in zip, got %d", inspectDoc.Data.TotalRasters)
	}

	// Verify ground-floor detected sidecar and georeference
	foundGround := false
	for _, ds := range inspectDoc.Data.Datasets {
		if ds.EntryPath == "campus/building-a/ground-floor.png" {
			foundGround = true
			if !ds.IsGeoreferenced {
				t.Fatalf("expected ground-floor to be marked as georeferenced via pgw")
			}
			if len(ds.Sidecars) == 0 {
				t.Fatalf("expected sidecars list to include ground-floor.pgw")
			}
			if ds.SuggestedFloor != 0 {
				t.Fatalf("expected suggested floor 0 for ground floor, got %d", ds.SuggestedFloor)
			}
		}
	}
	if !foundGround {
		t.Fatalf("ground-floor dataset not found in inspect response")
	}

	// 4. Test POST /datasets/import-archive-selected
	// Select only 2 datasets (ground-floor and basement)
	selBody, _ := json.Marshal(map[string]interface{}{
		"archive_token": token,
		"selections": []map[string]interface{}{
			{
				"entry_path":    "campus/building-a/ground-floor.png",
				"name":          "Building A Ground Floor",
				"building_name": "Building A",
				"floor_level":   0,
				"floor_label":   "Ground Floor",
			},
			{
				"entry_path":    "campus/building-b/basement.png",
				"name":          "Building B Basement Plan",
				"building_name": "Building B",
				"floor_level":   -1,
				"floor_label":   "Basement 1",
			},
		},
	})

	importReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/import-archive-selected", bytes.NewReader(selBody))
	importReq.Header.Set("Content-Type", "application/json")
	importReq.Header.Set("X-Session-ID", sessionID)

	importResp, err := client.Do(importReq)
	if err != nil {
		t.Fatalf("failed calling import-archive-selected: %v", err)
	}
	defer importResp.Body.Close()
	if importResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from import-archive-selected, got %d", importResp.StatusCode)
	}

	var importDoc struct {
		Data struct {
			ImportedCount int `json:"imported_count"`
			FailedCount   int `json:"failed_count"`
			Results       []struct {
				EntryPath string `json:"entry_path"`
				DatasetID string `json:"dataset_id"`
				Name      string `json:"name"`
				Status    string `json:"status"`
			} `json:"results"`
		} `json:"data"`
	}

	if err := json.NewDecoder(importResp.Body).Decode(&importDoc); err != nil {
		t.Fatalf("failed decoding import response: %v", err)
	}

	if importDoc.Data.ImportedCount != 2 {
		t.Fatalf("expected 2 imported datasets, got %d", importDoc.Data.ImportedCount)
	}
	if importDoc.Data.FailedCount != 0 {
		t.Fatalf("expected 0 failed datasets, got %d", importDoc.Data.FailedCount)
	}

	// 5. Verify both imported datasets are listed in session datasets
	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/datasets", nil)
	listReq.Header.Set("X-Session-ID", sessionID)
	listResp, err := client.Do(listReq)
	if err != nil {
		t.Fatalf("failed listing datasets: %v", err)
	}
	defer listResp.Body.Close()
	var listDoc struct {
		Data struct {
			Datasets []map[string]interface{} `json:"datasets"`
		} `json:"data"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&listDoc)
	if len(listDoc.Data.Datasets) != 2 {
		t.Fatalf("expected 2 datasets listed in session, found %d", len(listDoc.Data.Datasets))
	}

	// 6. Verify buildings and floors were registered in session
	bldReq, _ := http.NewRequest(http.MethodGet, server.URL+"/buildings", nil)
	bldReq.Header.Set("X-Session-ID", sessionID)
	bldResp, err := client.Do(bldReq)
	if err != nil {
		t.Fatalf("failed listing buildings: %v", err)
	}
	defer bldResp.Body.Close()
	var bldDoc struct {
		Data []*models.Building `json:"data"`
	}
	_ = json.NewDecoder(bldResp.Body).Decode(&bldDoc)
	if len(bldDoc.Data) != 2 {
		t.Fatalf("expected 2 buildings created during import, found %d", len(bldDoc.Data))
	}
}

func TestAPI_MultiDatasetZipSlipDefense(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	sessResp, _ := client.Post(server.URL+"/sessions", "application/json", nil)
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// Malicious ZIP with ../ traversal entry
	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)
	f, _ := zw.Create("../../../etc/passwd.png")
	_, _ = f.Write([]byte("malicious content"))
	_ = zw.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "malicious.zip")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/inspect-archive", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for malicious zip-slip archive, got %d", resp.StatusCode)
	}
}
