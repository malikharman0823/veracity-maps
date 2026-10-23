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

func TestAPI_MultiFloorUploadAllFloorsParsed(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed creating session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 1. Create a zip with 3 floors in a single building
	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)

	writeImg := func(entryName string, w, h int) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 80, G: 160, B: 240, A: 255})
			}
		}
		f, _ := zw.Create(entryName)
		_ = png.Encode(f, img)
	}

	writeStr := func(entryName, content string) {
		f, _ := zw.Create(entryName)
		_, _ = f.Write([]byte(content))
	}

	// Floor 0 (Ground) with .pgw sidecar
	writeImg("Engineering_Tower/Floor_G/floor_g.png", 100, 100)
	writeStr("Engineering_Tower/Floor_G/floor_g.pgw", "0.0001\n0.0\n0.0\n-0.0001\n-122.4194\n37.7749\n")

	// Floor 1 (First Floor) - no sidecar
	writeImg("Engineering_Tower/Floor_1/floor_1.png", 100, 100)

	// Floor 2 (Second Floor) - no sidecar
	writeImg("Engineering_Tower/Floor_2/floor_2.png", 100, 100)

	// Also add junk macOS files that must be ignored
	writeStr("__MACOSX/Engineering_Tower/._floor_g.png", "junk macos resource fork")
	writeStr("Engineering_Tower/.DS_Store", "junk ds store")

	_ = zw.Close()

	// 2. Upload directly to /upload (or /datasets/import-zip)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "engineering_tower.zip")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 or 201 from /upload, got %d", resp.StatusCode)
	}

	// 3. Verify datasets in session: all 3 floors must exist
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

	if len(listDoc.Data.Datasets) < 3 {
		t.Fatalf("expected at least 3 datasets parsed from zip, found %d", len(listDoc.Data.Datasets))
	}

	// 4. Verify all 3 floors have georeferenced status completed (inherited from Floor G)
	for _, ds := range listDoc.Data.Datasets {
		status, _ := ds["georeference_status"].(string)
		fn, _ := ds["filename"].(string)
		if status != "completed" {
			t.Errorf("expected dataset '%s' to have georeference_status 'completed' (inherited), got '%s'", fn, status)
		}
		if ds["map_layer"] == nil {
			t.Errorf("expected dataset '%s' to have map_layer populated", fn)
		}
	}

	// 5. Verify Building and floors were registered
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
	if len(bldDoc.Data) == 0 {
		t.Fatalf("expected at least 1 building created, found 0")
	}
	if len(bldDoc.Data[0].Floors) < 3 {
		t.Fatalf("expected building to have at least 3 floors, found %d", len(bldDoc.Data[0].Floors))
	}
}

func TestAPI_SingleGeoreferencePropagatesToEntireBuilding(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	sessResp, _ := client.Post(server.URL+"/sessions", "application/json", nil)
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 1. Create a Building with 2 floors (Ground Floor and Floor 1)
	bldBody, _ := json.Marshal(map[string]interface{}{
		"name": "Innovation Center",
	})
	bldReq, _ := http.NewRequest(http.MethodPost, server.URL+"/buildings", bytes.NewReader(bldBody))
	bldReq.Header.Set("Content-Type", "application/json")
	bldReq.Header.Set("X-Session-ID", sessionID)
	bldResp, _ := client.Do(bldReq)
	var bldDoc struct {
		Data *models.Building `json:"data"`
	}
	_ = json.NewDecoder(bldResp.Body).Decode(&bldDoc)
	bldResp.Body.Close()
	buildingID := bldDoc.Data.ID

	// Helper to upload a raw PNG
	uploadPNG := func(fn string) string {
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		buf := &bytes.Buffer{}
		_ = png.Encode(buf, img)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", fn)
		_, _ = part.Write(buf.Bytes())
		_ = writer.Close()

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-Session-ID", sessionID)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed uploading PNG: %v", err)
			return ""
		}
		defer resp.Body.Close()
		var resDoc struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&resDoc)
		return resDoc.Data.ID
	}

	dsID1 := uploadPNG("floor_ground.png")
	dsID2 := uploadPNG("floor_one.png")

	// Associate both datasets with building floors
	fl1Body, _ := json.Marshal(map[string]interface{}{
		"level":       0,
		"name":        "Ground Floor",
		"dataset_ids": []string{dsID1},
	})
	fl1Req, _ := http.NewRequest(http.MethodPost, server.URL+"/buildings/"+buildingID+"/floors", bytes.NewReader(fl1Body))
	fl1Req.Header.Set("Content-Type", "application/json")
	fl1Req.Header.Set("X-Session-ID", sessionID)
	fl1Resp, err := client.Do(fl1Req)
	if err != nil {
		t.Fatalf("failed adding floor 1: %v", err)
	}
	fl1Resp.Body.Close()

	fl2Body, _ := json.Marshal(map[string]interface{}{
		"level":       1,
		"name":        "Floor 1",
		"dataset_ids": []string{dsID2},
	})
	fl2Req, _ := http.NewRequest(http.MethodPost, server.URL+"/buildings/"+buildingID+"/floors", bytes.NewReader(fl2Body))
	fl2Req.Header.Set("Content-Type", "application/json")
	fl2Req.Header.Set("X-Session-ID", sessionID)
	fl2Resp, err := client.Do(fl2Req)
	if err != nil {
		t.Fatalf("failed adding floor 2: %v", err)
	}
	fl2Resp.Body.Close()

	// 2. Georeference only Floor Ground (dsID1) with 4 control points
	controlPoints := []struct {
		ImageX    float64 `json:"image_x"`
		ImageY    float64 `json:"image_y"`
		Longitude float64 `json:"longitude"`
		Latitude  float64 `json:"latitude"`
	}{
		{ImageX: 0, ImageY: 0, Longitude: -122.42, Latitude: 37.78},
		{ImageX: 99, ImageY: 0, Longitude: -122.41, Latitude: 37.78},
		{ImageX: 99, ImageY: 99, Longitude: -122.41, Latitude: 37.77},
		{ImageX: 0, ImageY: 99, Longitude: -122.42, Latitude: 37.77},
	}

	for _, cp := range controlPoints {
		cpData, _ := json.Marshal(cp)
		cpReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/"+dsID1+"/control-points", bytes.NewReader(cpData))
		cpReq.Header.Set("Content-Type", "application/json")
		cpReq.Header.Set("X-Session-ID", sessionID)
		cpResp, err := client.Do(cpReq)
		if err != nil {
			t.Fatalf("failed adding control point: %v", err)
		}
		cpResp.Body.Close()
	}

	geoReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/"+dsID1+"/georeference", bytes.NewReader([]byte("{}")))
	geoReq.Header.Set("Content-Type", "application/json")
	geoReq.Header.Set("X-Session-ID", sessionID)
	geoResp, err := client.Do(geoReq)
	if err != nil || geoResp.StatusCode != http.StatusOK {
		t.Fatalf("failed georeferencing floor ground: %v", err)
	}
	geoResp.Body.Close()

	// 3. Verify that Floor 1 (dsID2) AUTOMATICALLY inherited georeferencing from Floor Ground!
	ds2Req, _ := http.NewRequest(http.MethodGet, server.URL+"/datasets/"+dsID2, nil)
	ds2Req.Header.Set("X-Session-ID", sessionID)
	ds2Resp, err := client.Do(ds2Req)
	if err != nil {
		t.Fatalf("failed getting dataset 2: %v", err)
	}
	defer ds2Resp.Body.Close()

	var ds2Doc struct {
		Data *models.Dataset `json:"data"`
	}
	_ = json.NewDecoder(ds2Resp.Body).Decode(&ds2Doc)

	if ds2Doc.Data.GeoreferenceStatus != "completed" {
		t.Fatalf("expected Floor 1 to have georeference_status 'completed' inherited from Ground Floor, got '%s'", ds2Doc.Data.GeoreferenceStatus)
	}
	if ds2Doc.Data.MapLayer == nil {
		t.Fatalf("expected Floor 1 to have map_layer populated after sibling georeferenced")
	}

	coords, ok := ds2Doc.Data.MapLayer["coordinates"].([]interface{})
	if !ok || len(coords) != 4 {
		t.Fatalf("expected Floor 1 map_layer to have 4 coordinates, got %v", ds2Doc.Data.MapLayer["coordinates"])
	}

	// 4. Verify workspace manifest for dsID1 and dsID2 both contain each other with full map layer
	wsReq, _ := http.NewRequest(http.MethodGet, server.URL+"/datasets/"+dsID2+"/workspace", nil)
	wsReq.Header.Set("X-Session-ID", sessionID)
	wsResp, err := client.Do(wsReq)
	if err != nil || wsResp.StatusCode != http.StatusOK {
		t.Fatalf("failed getting workspace for dsID2: %v", err)
	}
	defer wsResp.Body.Close()

	var wsDoc struct {
		Data struct {
			Members []struct {
				DatasetID string                 `json:"dataset_id"`
				MapLayer  map[string]interface{} `json:"map_layer"`
				Bounds    interface{}            `json:"bounds"`
			} `json:"members"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wsResp.Body).Decode(&wsDoc)
	if len(wsDoc.Data.Members) < 2 {
		t.Fatalf("expected workspace to contain both building floor members, found %d", len(wsDoc.Data.Members))
	}
	for _, m := range wsDoc.Data.Members {
		if m.MapLayer == nil && m.Bounds == nil {
			t.Errorf("member %s missing map layer and bounds", m.DatasetID)
		}
	}
}

func TestAPI_MultiFloorSampleWithBuildingIDAndDatasetManifest(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed creating session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// Create ZIP exactly matching the user's veracity_maps_multifloor_sample format
	zipBuf := &bytes.Buffer{}
	zw := zip.NewWriter(zipBuf)

	writeImg := func(entryName string, w, h int) {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 100, G: 120, B: 210, A: 255})
			}
		}
		f, _ := zw.Create(entryName)
		_ = png.Encode(f, img)
	}

	writeStr := func(entryName, content string) {
		f, _ := zw.Create(entryName)
		_, _ = f.Write([]byte(content))
	}

	writeImg("Building_A/Basement_-1.png", 100, 100)
	writeImg("Building_A/First_1.png", 100, 100)
	writeImg("Building_A/Ground_0.png", 100, 100)
	writeStr("Building_A/Ground_0.pgw", "0.0001\n0.0\n0.0\n-0.0001\n77.2090\n28.6139\n")
	writeStr("Building_A/Ground_0.prj", "GEOGCS[\"WGS 84\",DATUM[\"WGS_1984\",SPHEROID[\"WGS 84\",6378137,298.257223563]],PRIMEM[\"Greenwich\",0],UNIT[\"degree\",0.0174532925199433]]")
	writeImg("Building_A/Second_2.png", 100, 100)
	writeImg("Building_B/Ground_0.png", 100, 100)

	manifestJSON := `{
  "project": "Veracity Maps sample multi-floor import",
  "version": 1,
  "buildings": [
    {
      "building_id": "building_a",
      "name": "Building A",
      "floors": [
        {
          "floor_id": "a_basement",
          "name": "Basement",
          "level": -1,
          "dataset": "Building_A/Basement_-1.png",
          "georeferenced": false
        },
        {
          "floor_id": "a_ground",
          "name": "Ground Floor",
          "level": 0,
          "dataset": "Building_A/Ground_0.png",
          "georeferenced": true
        },
        {
          "floor_id": "a_first",
          "name": "First Floor",
          "level": 1,
          "dataset": "Building_A/First_1.png",
          "georeferenced": false
        },
        {
          "floor_id": "a_second",
          "name": "Second Floor",
          "level": 2,
          "dataset": "Building_A/Second_2.png",
          "georeferenced": false
        }
      ]
    },
    {
      "building_id": "building_b",
      "name": "Building B",
      "floors": [
        {
          "floor_id": "b_ground",
          "name": "Ground Floor",
          "level": 0,
          "dataset": "Building_B/Ground_0.png",
          "georeferenced": false
        }
      ]
    }
  ]
}`
	writeStr("manifest.json", manifestJSON)
	_ = zw.Close()

	// Upload ZIP
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "sample_multifloor.zip")
	_, _ = part.Write(zipBuf.Bytes())
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200/201 from /upload, got %d", resp.StatusCode)
	}

	// Verify all 5 datasets exist and are georeferenced via propagation
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
	if len(listDoc.Data.Datasets) != 5 {
		t.Fatalf("expected 5 datasets extracted, got %d", len(listDoc.Data.Datasets))
	}

	// Verify Building A floors all inherited georeferencing
	var firstDatasetID string
	for _, ds := range listDoc.Data.Datasets {
		fname, _ := ds["filename"].(string)
		status, _ := ds["georeference_status"].(string)
		id, _ := ds["id"].(string)
		if firstDatasetID == "" {
			firstDatasetID = id
		}
		em, _ := ds["extracted_metadata"].(map[string]interface{})
		bName, _ := em["building_name"].(string)
		switch bName {
		case "Building A":
			if status != "completed" {
				t.Fatalf("expected Building A dataset %s to have georeference_status 'completed' via propagation, got '%s'", fname, status)
			}
		case "Building B":
			if status != "not_started" {
				t.Fatalf("expected Building B dataset %s to have georeference_status 'not_started', got '%s'", fname, status)
			}
		}
	}

	// Verify workspace returns all 5 members with valid buildings
	wsReq, _ := http.NewRequest(http.MethodGet, server.URL+"/datasets/"+firstDatasetID+"/workspace", nil)
	wsReq.Header.Set("X-Session-ID", sessionID)
	wsResp, err := client.Do(wsReq)
	if err != nil || wsResp.StatusCode != http.StatusOK {
		t.Fatalf("failed getting workspace: %v", err)
	}
	defer wsResp.Body.Close()

	var wsDoc struct {
		Data struct {
			Members []map[string]interface{} `json:"members"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wsResp.Body).Decode(&wsDoc)
	if len(wsDoc.Data.Members) != 5 {
		t.Fatalf("expected workspace to contain all 5 members, got %d", len(wsDoc.Data.Members))
	}
}

func TestAPI_ConsolidatePseudoBuildingsAndPropagateGeoreference(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	sessResp, _ := client.Post(server.URL+"/sessions", "application/json", nil)
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 1. Create 3 datasets: Floor B1 (unreferenced), Floor G (georeferenced), Floor 1 (unreferenced)
	dsB1 := &models.Dataset{
		ID:                 "ds_test_b1",
		SessionID:          sessionID,
		Filename:           "building_a_floor_b1.png",
		GeoreferenceStatus: "not_started",
		ExtractedMetadata: map[string]interface{}{
			"archive_entry":     "Building_A/Floor_B1/building_a_floor_b1.png",
			"building_name":     "Floor B1",
			"floor_level":       -1,
			"floor_label":       "Basement 1",
			"is_building_floor": true,
			"width":             1400.0,
			"height":            1000.0,
		},
	}
	dsG := &models.Dataset{
		ID:                 "ds_test_g",
		SessionID:          sessionID,
		Filename:           "building_a_floor_g.png",
		GeoreferenceStatus: "completed",
		ExtractedMetadata: map[string]interface{}{
			"archive_entry":     "Building_A/Floor_G/building_a_floor_g.png",
			"building_name":     "Floor G",
			"floor_level":       0,
			"floor_label":       "Ground Floor",
			"is_building_floor": true,
			"width":             1400.0,
			"height":            1000.0,
			"crs":               "EPSG:4326",
			"bounds": map[string]interface{}{
				"min_x": 77.2,
				"min_y": 28.619,
				"max_x": 77.2014,
				"max_y": 28.62,
			},
		},
		MapLayer: map[string]interface{}{
			"id":         "lyr_ds_test_g",
			"name":       "building_a_floor_g.png",
			"type":       "raster",
			"bounds_crs": "EPSG:4326",
			"bounds": map[string]interface{}{
				"min_x": 77.2,
				"min_y": 28.619,
				"max_x": 77.2014,
				"max_y": 28.62,
			},
			"coordinates": [][]float64{
				{77.2, 28.62},
				{77.2014, 28.62},
				{77.2014, 28.619},
				{77.2, 28.619},
			},
		},
	}
	ds1 := &models.Dataset{
		ID:                 "ds_test_1",
		SessionID:          sessionID,
		Filename:           "building_a_floor_1.png",
		GeoreferenceStatus: "not_started",
		ExtractedMetadata: map[string]interface{}{
			"archive_entry":     "Building_A/Floor_1/building_a_floor_1.png",
			"building_name":     "Floor 1",
			"floor_level":       1,
			"floor_label":       "Floor 1",
			"is_building_floor": true,
			"width":             1400.0,
			"height":            1000.0,
		},
	}

	_ = repo.InsertDataset(sessionID, dsB1)
	_ = repo.InsertDataset(sessionID, dsG)
	_ = repo.InsertDataset(sessionID, ds1)

	// Create 3 pseudo-buildings as happened during earlier parsing
	bldB1, _ := repo.CreateBuilding(sessionID, &models.Building{Name: "Floor B1"})
	bldG, _ := repo.CreateBuilding(sessionID, &models.Building{Name: "Floor G"})
	bld1, _ := repo.CreateBuilding(sessionID, &models.Building{Name: "Floor 1"})

	_, _ = repo.AddFloor(sessionID, bldB1.ID, &models.BuildingFloor{Level: -1, Name: "Basement 1", DatasetIDs: []string{dsB1.ID}})
	_, _ = repo.AddFloor(sessionID, bldG.ID, &models.BuildingFloor{Level: 0, Name: "Ground Floor", DatasetIDs: []string{dsG.ID}})
	_, _ = repo.AddFloor(sessionID, bld1.ID, &models.BuildingFloor{Level: 1, Name: "Floor 1", DatasetIDs: []string{ds1.ID}})

	// 2. Call GET /buildings which triggers consolidation
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

	// Verify only 1 consolidated building ("Building A") exists with 3 floors
	if len(bldDoc.Data) != 1 {
		t.Fatalf("expected 1 consolidated building, got %d", len(bldDoc.Data))
	}
	bld := bldDoc.Data[0]
	if bld.Name != "Building A" {
		t.Errorf("expected consolidated building name 'Building A', got '%s'", bld.Name)
	}
	if len(bld.Floors) != 3 {
		t.Errorf("expected 3 floors in consolidated building, got %d", len(bld.Floors))
	}

	// 3. Verify that both Floor B1 and Floor 1 are now georeferenced via automatic spatial propagation!
	updatedB1, _ := repo.GetDataset(sessionID, dsB1.ID)
	if updatedB1.GeoreferenceStatus != "completed" {
		t.Errorf("expected dsB1 to have georeference_status 'completed', got '%s'", updatedB1.GeoreferenceStatus)
	}
	if updatedB1.MapLayer == nil {
		t.Errorf("expected dsB1 to have map_layer populated")
	}

	updated1, _ := repo.GetDataset(sessionID, ds1.ID)
	if updated1.GeoreferenceStatus != "completed" {
		t.Errorf("expected ds1 to have georeference_status 'completed', got '%s'", updated1.GeoreferenceStatus)
	}
	if updated1.MapLayer == nil {
		t.Errorf("expected ds1 to have map_layer populated")
	}
}
