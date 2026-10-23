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

func TestAPI_MultiFloorInheritanceAndWorkspaceMembers(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Session
	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed creating session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 2. Generate multi-floor ZIP with manifest.json
	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)

	writeImg := func(entryName string, w, h int) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 200, G: 150, B: 50, A: 255})
			}
		}
		f, _ := zw.Create(entryName)
		_ = png.Encode(f, img)
	}

	writeStr := func(entryName, content string) {
		f, _ := zw.Create(entryName)
		_, _ = f.Write([]byte(content))
	}

	writeImg("Building_A/Floor_B1/building_a_floor_b1.png", 100, 100)
	writeImg("Building_A/Floor_G/building_a_floor_g.png", 100, 100)
	writeStr("Building_A/Floor_G/building_a_floor_g.pgw", "0.0001\n0.0\n0.0\n-0.0001\n-122.4194\n37.7749\n")
	writeImg("Building_A/Floor_1/building_a_floor_1.png", 100, 100)

	manifestContent := `{
  "project": "Multi-floor test",
  "buildings": [
    {
      "id": "building_a",
      "name": "Building A",
      "floors": [
        { "id": "b1", "name": "Basement", "level": -1, "raster": "Building_A/Floor_B1/building_a_floor_b1.png" },
        { "id": "g", "name": "Ground Floor", "level": 0, "raster": "Building_A/Floor_G/building_a_floor_g.png", "georeferenced": true },
        { "id": "1", "name": "First Floor", "level": 1, "raster": "Building_A/Floor_1/building_a_floor_1.png" }
      ]
    }
  ]
}`
	writeStr("manifest.json", manifestContent)
	_ = zw.Close()

	// 3. Inspect Archive
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "multifloor.zip")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	inspectReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/inspect-archive", body)
	inspectReq.Header.Set("Content-Type", writer.FormDataContentType())
	inspectReq.Header.Set("X-Session-ID", sessionID)

	inspectResp, err := client.Do(inspectReq)
	if err != nil || inspectResp.StatusCode != http.StatusOK {
		t.Fatalf("inspect-archive failed: %v", err)
	}

	var inspectDoc struct {
		Data struct {
			ArchiveToken string `json:"archive_token"`
			TotalRasters int    `json:"total_rasters"`
			Datasets     []struct {
				EntryPath           string `json:"entry_path"`
				Name                string `json:"name"`
				SuggestedBuilding   string `json:"suggested_building"`
				SuggestedFloor      int    `json:"suggested_floor"`
				SuggestedFloorLabel string `json:"suggested_floor_label"`
				IsGeoreferenced     bool   `json:"is_georeferenced"`
			} `json:"datasets"`
		} `json:"data"`
	}
	_ = json.NewDecoder(inspectResp.Body).Decode(&inspectDoc)
	inspectResp.Body.Close()

	if inspectDoc.Data.TotalRasters != 3 {
		t.Fatalf("expected 3 rasters, got %d", inspectDoc.Data.TotalRasters)
	}

	// Verify manifest.json parsed suggested attributes
	for _, ds := range inspectDoc.Data.Datasets {
		if ds.SuggestedBuilding != "Building A" {
			t.Errorf("expected suggested building 'Building A', got '%s'", ds.SuggestedBuilding)
		}
	}

	// 4. Import all 3 selections
	var selections []map[string]interface{}
	for _, ds := range inspectDoc.Data.Datasets {
		selections = append(selections, map[string]interface{}{
			"entry_path":    ds.EntryPath,
			"name":          ds.Name,
			"building_name": ds.SuggestedBuilding,
			"floor_level":   ds.SuggestedFloor,
			"floor_label":   ds.SuggestedFloorLabel,
		})
	}

	selBytes, _ := json.Marshal(map[string]interface{}{
		"archive_token": inspectDoc.Data.ArchiveToken,
		"selections":    selections,
	})

	importReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/import-archive-selected", bytes.NewReader(selBytes))
	importReq.Header.Set("Content-Type", "application/json")
	importReq.Header.Set("X-Session-ID", sessionID)

	importResp, err := client.Do(importReq)
	if err != nil || importResp.StatusCode != http.StatusOK {
		t.Fatalf("import-archive-selected failed: %v", err)
	}

	var importDoc struct {
		Data struct {
			ImportedCount int `json:"imported_count"`
			Results       []struct {
				EntryPath string          `json:"entry_path"`
				DatasetID string          `json:"dataset_id"`
				Status    string          `json:"status"`
				Dataset   *models.Dataset `json:"dataset"`
			} `json:"results"`
		} `json:"data"`
	}
	_ = json.NewDecoder(importResp.Body).Decode(&importDoc)
	importResp.Body.Close()

	if importDoc.Data.ImportedCount != 3 {
		t.Fatalf("expected 3 imported datasets, got %d", importDoc.Data.ImportedCount)
	}

	var floor1DatasetID string
	for _, res := range importDoc.Data.Results {
		if res.Dataset == nil {
			t.Fatalf("expected non-nil dataset in import results")
		}
		if res.Dataset.GeoreferenceStatus != "completed" {
			t.Errorf("dataset %s expected georeference_status completed (inherited), got %s", res.EntryPath, res.Dataset.GeoreferenceStatus)
		}
		if res.Dataset.MapLayer == nil {
			t.Errorf("dataset %s expected map_layer to be populated", res.EntryPath)
		}
		if res.EntryPath == "Building_A/Floor_1/building_a_floor_1.png" {
			floor1DatasetID = res.DatasetID
		}
	}

	if floor1DatasetID == "" {
		t.Fatalf("floor 1 dataset ID not found")
	}

	// 5. Open Workspace for Floor 1 - should automatically contain all 3 building floors as members!
	wsReq, _ := http.NewRequest(http.MethodGet, server.URL+"/datasets/"+floor1DatasetID+"/workspace", nil)
	wsReq.Header.Set("X-Session-ID", sessionID)

	wsResp, err := client.Do(wsReq)
	if err != nil || wsResp.StatusCode != http.StatusOK {
		t.Fatalf("failed calling get workspace for floor 1: %v", err)
	}

	var wsDoc struct {
		Data struct {
			Members []struct {
				DatasetID       string                 `json:"dataset_id"`
				Name            string                 `json:"name"`
				FloorLevel      int                    `json:"floor_level"`
				FloorLabel      string                 `json:"floor_label"`
				BuildingName    string                 `json:"building_name"`
				IsBuildingFloor bool                   `json:"is_building_floor"`
				MapLayer        map[string]interface{} `json:"map_layer"`
			} `json:"members"`
			Buildings []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"buildings"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wsResp.Body).Decode(&wsDoc)
	wsResp.Body.Close()

	if len(wsDoc.Data.Members) != 3 {
		t.Fatalf("expected workspace to contain all 3 building floors as members, got %d", len(wsDoc.Data.Members))
	}

	if len(wsDoc.Data.Buildings) == 0 {
		t.Fatalf("expected buildings list in workspace manifest to include Building A")
	}

	foundB1, foundG, found1 := false, false, false
	for _, m := range wsDoc.Data.Members {
		if m.BuildingName != "Building A" {
			t.Errorf("expected member building_name 'Building A', got '%s'", m.BuildingName)
		}
		if !m.IsBuildingFloor {
			t.Errorf("expected is_building_floor true")
		}
		if m.MapLayer == nil {
			t.Errorf("expected member map_layer to be present")
		}
		if m.FloorLevel == -1 {
			foundB1 = true
		}
		if m.FloorLevel == 0 {
			foundG = true
		}
		if m.FloorLevel == 1 {
			found1 = true
		}
	}

	if !foundB1 || !foundG || !found1 {
		t.Errorf("missing floor in workspace members: B1=%v, G=%v, 1=%v", foundB1, foundG, found1)
	}
}
