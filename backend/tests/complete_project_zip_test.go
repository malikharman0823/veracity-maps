package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/models"
)

// TestCompleteProjectZipExport verifies the new Complete Project (.zip) export
// and its compatibility with the offline Viewscape manifest format and re-import workflow.
func TestCompleteProjectZipExport(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)
	client := server.Client()

	sess := repo.CreateSession()
	sessionID := sess.ID

	// 1. Upload primary and secondary floor rasters
	uploadDS := func(filename string) string {
		pngBytes := createTestPNG(256, 256)
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", filename)
		_, _ = part.Write(pngBytes)
		_ = writer.Close()

		req, _ := http.NewRequest("POST", server.URL+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-Session-ID", sessionID)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed uploading %s: %v", filename, err)
		}
		var res struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		_ = resp.Body.Close()
		return res.Data.ID
	}

	primaryID := uploadDS("campus_ground.png")
	secondID := uploadDS("campus_floor2.png")

	// 2. Set control points on primary dataset
	addCP := func(ix, iy float64, lng, lat float64) {
		cpData := map[string]interface{}{
			"image_x":   ix,
			"image_y":   iy,
			"longitude": lng,
			"latitude":  lat,
		}
		cpBytes, _ := json.Marshal(cpData)
		req, _ := http.NewRequest("POST", server.URL+"/datasets/"+primaryID+"/control-points", bytes.NewReader(cpBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", sessionID)
		r, err := client.Do(req)
		if err != nil || r.StatusCode != http.StatusCreated {
			t.Fatalf("add CP failed: %v", err)
		}
		_ = r.Body.Close()
	}

	addCP(0, 0, -122.4200, 37.7800)
	addCP(250, 0, -122.4100, 37.7800)
	addCP(250, 250, -122.4100, 37.7700)
	addCP(0, 250, -122.4200, 37.7700)

	// Calculate georeference
	geoBody, _ := json.Marshal(map[string]string{
		"action":         "calculate",
		"transformation": "affine",
	})
	geoReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+primaryID+"/georeference", bytes.NewReader(geoBody))
	geoReq.Header.Set("Content-Type", "application/json")
	geoReq.Header.Set("X-Session-ID", sessionID)
	geoResp, err := client.Do(geoReq)
	if err != nil || geoResp.StatusCode != http.StatusOK {
		t.Fatalf("georeference calculation failed: %v, status: %d", err, geoResp.StatusCode)
	}
	_ = geoResp.Body.Close()

	// 3. Create a building with two floors
	bld, err := repo.CreateBuilding(sessionID, &models.Building{
		Name: "Innovation Hub",
		Floors: []*models.BuildingFloor{
			{ID: "fl_ground", Level: 0, Name: "Ground Floor", DatasetIDs: []string{primaryID}},
			{ID: "fl_second", Level: 1, Name: "Second Floor", DatasetIDs: []string{secondID}},
		},
	})
	if err != nil {
		t.Fatalf("failed creating building: %v", err)
	}

	// 4. Create Landmark Icon Points with custom properties
	_, err = repo.CreateIconPoint(sessionID, &models.IconPoint{
		ID:          "ip_main_entry",
		DatasetID:   primaryID,
		BuildingID:  bld.ID,
		Name:        "Main Entrance",
		IconID:      "001-camera",
		Description: "Visitor Security & Check-in",
		Scope:       "building",
		FloorLevel:  0,
		FloorLabel:  "Ground Floor",
		Longitude:   -122.4150,
		Latitude:    37.7750,
	})
	if err != nil {
		t.Fatalf("failed creating landmark point: %v", err)
	}

	// 5. Perform Complete Project (.zip) export with format: "project_zip"
	initZ := 19
	minZ := 14
	maxZ := 21
	exportPayload, _ := json.Marshal(export.ExportRequest{
		Format:      "project_zip",
		ProjectName: "Veracity Test Campus",
		LayerIDs:    []string{primaryID, secondID},
		BuildingIDs: []string{bld.ID},
		Options: &export.ExportOptions{
			IncludeBuildingConfig:  true,
			IncludeControlPoints:   true,
			IncludeIcons:           true,
			IncludeLayerStyles:     true,
			IncludeOriginalRaster:  true,
			IncludeProcessedRaster: true,
			IncludeBaseMapSettings: true,
			IncludeProjectMetadata: true,
		},
	})

	expReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+primaryID+"/export", bytes.NewReader(exportPayload))
	expReq.Header.Set("Content-Type", "application/json")
	expReq.Header.Set("Accept", "application/zip")
	expReq.Header.Set("X-Session-ID", sessionID)

	expResp, err := client.Do(expReq)
	if err != nil || expResp.StatusCode != http.StatusOK {
		t.Fatalf("complete project export failed: %v, status: %d", err, expResp.StatusCode)
	}
	defer expResp.Body.Close()

	zipBytes, err := io.ReadAll(expResp.Body)
	if err != nil {
		t.Fatalf("failed reading export zip bytes: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("invalid zip archive output: %v", err)
	}

	entries := make(map[string]*zip.File)
	for _, f := range zipReader.File {
		entries[f.Name] = f
	}

	// Verify required structure from specification
	requiredFiles := []string{
		"project.json",
		"style.json",
		"viewer.html",
		"index.html",
		"README.txt",
		"layers/landmarks.geojson",
		"layers/control_points.geojson",
		"metadata/layer_config.json",
		"metadata/georeferencing.json",
		"metadata.json",
		"mapstudio.json",
	}

	for _, reqName := range requiredFiles {
		if entries[reqName] == nil {
			t.Errorf("expected required file '%s' is missing in complete project zip", reqName)
		}
	}

	// 6. Verify project.json manifest schema
	projEntry := entries["project.json"]
	if projEntry == nil {
		t.Fatalf("project.json missing")
	}
	rc, _ := projEntry.Open()
	var projDoc struct {
		Format        string                 `json:"format"`
		SchemaVersion int                    `json:"schema_version"`
		Name          string                 `json:"name"`
		Project       map[string]interface{} `json:"project"`
		Map           struct {
			Center []float64 `json:"center"`
			Zoom   int       `json:"zoom"`
			MinZ   int       `json:"min_zoom"`
			MaxZ   int       `json:"max_zoom"`
			Preset string    `json:"preset"`
			CRS    string    `json:"crs"`
		} `json:"map"`
		Resources map[string]string `json:"resources"`
		Buildings []interface{}     `json:"buildings"`
		Layers    []string          `json:"layers"`
	}
	_ = json.NewDecoder(rc).Decode(&projDoc)
	_ = rc.Close()

	if projDoc.Format != "veracity-project" {
		t.Errorf("expected format 'veracity-project', got '%s'", projDoc.Format)
	}
	if projDoc.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", projDoc.SchemaVersion)
	}
	if projDoc.Resources["style"] != "style.json" {
		t.Errorf("expected style.json in resources, got %s", projDoc.Resources["style"])
	}
	if projDoc.Resources["landmarks"] != "layers/landmarks.geojson" {
		t.Errorf("expected layers/landmarks.geojson in resources, got %s", projDoc.Resources["landmarks"])
	}
	if projDoc.Resources["control_points"] != "layers/control_points.geojson" {
		t.Errorf("expected layers/control_points.geojson in resources, got %s", projDoc.Resources["control_points"])
	}

	// 7. Verify GeoJSON landmark properties
	lmEntry := entries["layers/landmarks.geojson"]
	if lmEntry != nil {
		lmRc, _ := lmEntry.Open()
		var lmFC struct {
			Type     string `json:"type"`
			Features []struct {
				Properties struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					IconID string `json:"icon_id"`
				} `json:"properties"`
			} `json:"features"`
		}
		_ = json.NewDecoder(lmRc).Decode(&lmFC)
		_ = lmRc.Close()

		if lmFC.Type != "FeatureCollection" || len(lmFC.Features) == 0 {
			t.Errorf("invalid or empty landmarks.geojson: %v", lmFC)
		} else {
			if lmFC.Features[0].Properties.ID != "ip_main_entry" {
				t.Errorf("expected landmark ID 'ip_main_entry', got %s", lmFC.Features[0].Properties.ID)
			}
			if lmFC.Features[0].Properties.Name != "Main Entrance" {
				t.Errorf("expected landmark Name 'Main Entrance', got %s", lmFC.Features[0].Properties.Name)
			}
		}
	}

	// 8. Verify Control Points GeoJSON
	cpEntry := entries["layers/control_points.geojson"]
	if cpEntry != nil {
		cpRc, _ := cpEntry.Open()
		var cpFC struct {
			Type     string `json:"type"`
			Features []struct {
				Properties struct {
					ID     string  `json:"id"`
					ImageX float64 `json:"image_x"`
				} `json:"properties"`
			} `json:"features"`
		}
		_ = json.NewDecoder(cpRc).Decode(&cpFC)
		_ = cpRc.Close()

		if cpFC.Type != "FeatureCollection" || len(cpFC.Features) < 2 {
			t.Errorf("invalid or incomplete control_points.geojson: %v", cpFC)
		}
	}

	// 9. Verify rasters/ directory contains source rasters
	hasRasterInRastersDir := false
	for name := range entries {
		if len(name) > 8 && name[:8] == "rasters/" {
			hasRasterInRastersDir = true
			break
		}
	}
	if !hasRasterInRastersDir {
		t.Errorf("expected source or processed rasters in rasters/ directory")
	}

	// 10. Roundtrip Reopen / Reconstruction Test
	// Import the complete project ZIP back into the system to verify reconstruction
	newSess := repo.CreateSession()
	reopenBody := new(bytes.Buffer)
	mpWriter := multipart.NewWriter(reopenBody)
	filePart, _ := mpWriter.CreateFormFile("file", "veracity_reopen_test.zip")
	_, _ = filePart.Write(zipBytes)
	_ = mpWriter.Close()

	reopenReq, _ := http.NewRequest("POST", server.URL+"/map-projects/import", reopenBody)
	reopenReq.Header.Set("Content-Type", mpWriter.FormDataContentType())
	reopenReq.Header.Set("X-Session-ID", newSess.ID)

	reopenResp, err := client.Do(reopenReq)
	if err != nil || reopenResp.StatusCode != http.StatusCreated {
		t.Fatalf("reopening complete project zip failed: %v, status: %d", err, reopenResp.StatusCode)
	}
	var reopenResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(reopenResp.Body).Decode(&reopenResult)
	_ = reopenResp.Body.Close()

	reopenedDatasetID := reopenResult.Data.ID
	if reopenedDatasetID == "" {
		t.Fatalf("reopened dataset ID is empty")
	}

	// Verify restored control points and landmarks in the new session
	restoredCPs, _ := repo.ListControlPoints(newSess.ID, reopenedDatasetID)
	if len(restoredCPs) == 0 {
		t.Errorf("control points were not restored upon re-importing complete project zip")
	}
	restoredIPs, _ := repo.ListIconPoints(newSess.ID, reopenedDatasetID, "")
	if len(restoredIPs) == 0 {
		t.Errorf("landmarks were not restored upon re-importing complete project zip")
	}

	_ = initZ
	_ = minZ
	_ = maxZ
}

// TestMBTilesAndBaseLayerExport verifies Single MBTiles File and Base Layer Only export options
func TestMBTilesAndBaseLayerExport(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)
	client := server.Client()

	sess := repo.CreateSession()
	sessionID := sess.ID

	// Upload test raster
	pngBytes := createTestPNG(256, 256)
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test_mbtiles.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed uploading raster: %v", err)
	}
	var res struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	_ = resp.Body.Close()
	datasetID := res.Data.ID

	// Georeference with 4 control points
	addCP := func(ix, iy, lng, lat float64) {
		cpData := map[string]interface{}{"image_x": ix, "image_y": iy, "longitude": lng, "latitude": lat}
		b, _ := json.Marshal(cpData)
		r, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/control-points", bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Session-ID", sessionID)
		_, _ = client.Do(r)
	}
	addCP(0, 0, -122.4200, 37.7800)
	addCP(250, 0, -122.4100, 37.7800)
	addCP(250, 250, -122.4100, 37.7700)
	addCP(0, 250, -122.4200, 37.7700)

	// Trigger georeference calculation
	geoReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/georeference", nil)
	geoReq.Header.Set("X-Session-ID", sessionID)
	geoResp, err := client.Do(geoReq)
	if err != nil || geoResp.StatusCode != http.StatusOK {
		t.Fatalf("georeference failed: %v", err)
	}
	_ = geoResp.Body.Close()

	// 1. Test Base Layer Only Export
	baseReqBody := map[string]interface{}{
		"format": "base_layer",
		"options": map[string]interface{}{
			"include_base_map_settings": true,
		},
	}
	baseBytes, _ := json.Marshal(baseReqBody)
	bReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/export", bytes.NewReader(baseBytes))
	bReq.Header.Set("Content-Type", "application/json")
	bReq.Header.Set("Accept", "application/zip")
	bReq.Header.Set("X-Session-ID", sessionID)
	bResp, err := client.Do(bReq)
	if err != nil || bResp.StatusCode != http.StatusOK {
		t.Fatalf("base_layer export failed: %v, status: %d", err, bResp.StatusCode)
	}
	baseZipBytes, _ := io.ReadAll(bResp.Body)
	_ = bResp.Body.Close()

	baseZip, err := zip.NewReader(bytes.NewReader(baseZipBytes), int64(len(baseZipBytes)))
	if err != nil {
		t.Fatalf("invalid base_layer zip: %v", err)
	}
	hasBaseMeta := false
	hasBaseStyle := false
	for _, f := range baseZip.File {
		if f.Name == "metadata.json" {
			hasBaseMeta = true
		}
		if f.Name == "style.json" {
			hasBaseStyle = true
		}
	}
	if !hasBaseMeta {
		t.Errorf("base_layer export missing metadata.json")
	}
	if !hasBaseStyle {
		t.Errorf("base_layer export missing style.json")
	}

	// 2. Test MBTiles / Tile Package Export
	mbReqBody := map[string]interface{}{
		"format": "mbtiles",
		"options": map[string]interface{}{
			"include_processed_rasters": true,
		},
	}
	mbBytes, _ := json.Marshal(mbReqBody)
	mReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/export", bytes.NewReader(mbBytes))
	mReq.Header.Set("Content-Type", "application/json")
	mReq.Header.Set("Accept", "application/zip")
	mReq.Header.Set("X-Session-ID", sessionID)
	mResp, err := client.Do(mReq)
	if err != nil || mResp.StatusCode != http.StatusOK {
		t.Fatalf("mbtiles export failed: %v, status: %d", err, mResp.StatusCode)
	}
	mbZipBytes, _ := io.ReadAll(mResp.Body)
	_ = mResp.Body.Close()

	mbZip, err := zip.NewReader(bytes.NewReader(mbZipBytes), int64(len(mbZipBytes)))
	if err != nil {
		t.Fatalf("invalid mbtiles zip: %v", err)
	}
	hasMbMeta := false
	for _, f := range mbZip.File {
		if f.Name == "metadata.json" {
			hasMbMeta = true
		}
	}
	if !hasMbMeta {
		t.Errorf("mbtiles export missing metadata.json")
	}
}
