package tests

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

func createTransparentTestPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 && y < h/2 {
				// Pure white background
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			} else if x >= w/2 && y < h/2 {
				// Near-white antialiased transition
				img.Set(x, y, color.RGBA{R: 245, G: 245, B: 245, A: 255})
			} else if x < w/2 && y >= h/2 {
				// Red foreground content
				img.Set(x, y, color.RGBA{R: 220, G: 20, B: 30, A: 255})
			} else {
				// Existing transparent pixel
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 0})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestTransparency_AlgorithmUnits(t *testing.T) {
	imgBytes := createTransparentTestPNG(100, 100)
	src, err := png.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		t.Fatalf("failed to decode png: %v", err)
	}

	targetColor := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	processed := gis.RemoveBackground(src, targetColor, 30.0, 10.0)
	if processed == nil {
		t.Fatalf("RemoveBackground returned nil")
	}

	// 1. Pure white (0, 0) should be transparent (alpha = 0)
	c0 := processed.NRGBAAt(0, 0)
	if c0.A != 0 {
		t.Errorf("expected white pixel to have alpha 0, got %d", c0.A)
	}

	// 2. Red foreground (10, 80) should remain fully opaque (alpha = 255, red preserved)
	cRed := processed.NRGBAAt(10, 80)
	if cRed.A != 255 || cRed.R < 200 {
		t.Errorf("expected red foreground to remain opaque with R>200, got R=%d A=%d", cRed.R, cRed.A)
	}

	// 3. Existing transparent pixel (80, 80) should stay transparent (alpha = 0)
	cTrans := processed.NRGBAAt(80, 80)
	if cTrans.A != 0 {
		t.Errorf("expected existing transparent pixel to stay alpha 0, got %d", cTrans.A)
	}

	// 4. Color distance parse
	parsed, err := gis.ParseHexColor("#FFFFFF")
	if err != nil || parsed.R != 255 || parsed.G != 255 || parsed.B != 255 {
		t.Errorf("failed to parse #FFFFFF: %v", err)
	}

	_, err = gis.ParseHexColor("invalid-color")
	if err == nil {
		t.Errorf("expected error for invalid hex color")
	}
}

func TestTransparency_APIEndpoints(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Create Session
	resp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("session creation failed: %v", err)
	}
	var sessResult struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessResult)
	sessionID := sessResult.Data.SessionID

	// 2. Upload PNG
	pngBytes := createTransparentTestPNG(200, 200)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "floorplan.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %v", err)
	}
	var uploadResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResult)
	datasetID := uploadResult.Data.ID

	// 3. GET /datasets/{id}/background-removal (initial state)
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/background-removal", nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET background-removal failed: %v", err)
	}

	var statusResp struct {
		Data struct {
			Enabled bool   `json:"enabled"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&statusResp)
	if statusResp.Data.Enabled {
		t.Errorf("expected initial enabled=false")
	}

	// 4. POST preview
	previewBody, _ := json.Marshal(map[string]interface{}{
		"action":        "preview",
		"color":         "#ffffff",
		"threshold":     30,
		"edge_softness": 10,
	})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/background-removal", bytes.NewReader(previewBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST preview failed: %v", err)
	}
	var prevResp struct {
		Data struct {
			PreviewDataURL string `json:"preview_data_url"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&prevResp)
	if prevResp.Data.PreviewDataURL == "" {
		t.Errorf("expected preview data URL, got empty")
	}

	// 5. POST apply
	applyBody, _ := json.Marshal(map[string]interface{}{
		"action":        "apply",
		"color":         "#ffffff",
		"threshold":     30,
		"edge_softness": 10,
	})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/background-removal", bytes.NewReader(applyBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST apply failed: %v", err)
	}

	ds, err := repo.GetDataset(sessionID, datasetID)
	if err != nil {
		t.Fatalf("failed to get dataset: %v", err)
	}
	if ds.ProcessedPath == "" {
		t.Errorf("expected ProcessedPath to be set after apply")
	}
	if _, err := os.Stat(ds.ProcessedPath); os.IsNotExist(err) {
		t.Errorf("expected processed file to exist at %s", ds.ProcessedPath)
	}

	// 6. GET source-image should now serve processed transparent image
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/source-image", nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET source-image failed: %v", err)
	}
	servedImg, err := png.Decode(resp.Body)
	if err != nil {
		t.Fatalf("failed to decode served image: %v", err)
	}
	// Check that top-left pixel is transparent
	_, _, _, a := servedImg.At(0, 0).RGBA()
	if a != 0 {
		t.Errorf("expected served source-image to be transparent at (0,0), got alpha=%d", a)
	}

	// 7. POST reset
	resetBody, _ := json.Marshal(map[string]interface{}{
		"action": "reset",
	})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/background-removal", bytes.NewReader(resetBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST reset failed: %v", err)
	}
	dsReset, _ := repo.GetDataset(sessionID, datasetID)
	if dsReset.ProcessedPath != "" {
		t.Errorf("expected ProcessedPath to be cleared after reset")
	}
}

func TestRasterCorners_ValidationAndAffineFit(t *testing.T) {
	// 1. Valid rectangle
	corners := gis.RasterCorners{
		TopLeft:     gis.CoordinatePoint{Longitude: -122.42, Latitude: 37.78},
		TopRight:    gis.CoordinatePoint{Longitude: -122.40, Latitude: 37.78},
		BottomRight: gis.CoordinatePoint{Longitude: -122.40, Latitude: 37.76},
		BottomLeft:  gis.CoordinatePoint{Longitude: -122.42, Latitude: 37.76},
	}
	if !gis.PolygonIsSimpleAndConvex(corners) {
		t.Errorf("expected valid rectangle to pass PolygonIsSimpleAndConvex")
	}

	// 2. Degenerate self-intersecting polygon (crossing corners)
	badCorners := gis.RasterCorners{
		TopLeft:     gis.CoordinatePoint{Longitude: -122.42, Latitude: 37.78},
		TopRight:    gis.CoordinatePoint{Longitude: -122.40, Latitude: 37.76}, // Crossed!
		BottomRight: gis.CoordinatePoint{Longitude: -122.40, Latitude: 37.78},
		BottomLeft:  gis.CoordinatePoint{Longitude: -122.42, Latitude: 37.76},
	}
	if gis.PolygonIsSimpleAndConvex(badCorners) {
		t.Errorf("expected crossed self-intersecting polygon to fail PolygonIsSimpleAndConvex")
	}

	// 3. Affine transform fit
	transform, bounds, err := gis.CalculateTransformFromCorners(corners, 1000, 800)
	if err != nil {
		t.Fatalf("CalculateTransformFromCorners failed: %v", err)
	}
	if len(transform) != 6 {
		t.Fatalf("expected 6 affine coefficients, got %d", len(transform))
	}
	if bounds[0] != -122.42 || bounds[1] != 37.76 || bounds[2] != -122.40 || bounds[3] != 37.78 {
		t.Errorf("unexpected bounds: %v", bounds)
	}

	// 4. Synchronize control points
	cps := []*models.ControlPoint{
		{ImageX: 0, ImageY: 0},
		{ImageX: 1000, ImageY: 0},
		{ImageX: 1000, ImageY: 800},
		{ImageX: 0, ImageY: 800},
		{ImageX: 500, ImageY: 400},
	}
	gis.SynchronizeControlPoints(cps, transform)

	// Top-left pixel (0,0) should be at (-122.42, 37.78)
	if mathAbs(cps[0].Longitude-(-122.42)) > 1e-4 || mathAbs(cps[0].Latitude-37.78) > 1e-4 {
		t.Errorf("cp[0] (0,0) expected (-122.42, 37.78), got (%f, %f)", cps[0].Longitude, cps[0].Latitude)
	}
	// Center pixel (500, 400) should be at center (-122.41, 37.77)
	if mathAbs(cps[4].Longitude-(-122.41)) > 1e-4 || mathAbs(cps[4].Latitude-37.77) > 1e-4 {
		t.Errorf("cp[4] (500,400) expected center (-122.41, 37.77), got (%f, %f)", cps[4].Longitude, cps[4].Latitude)
	}
}

func TestRasterTransform_APIEndpoints(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// Create Session
	resp, _ := client.Post(server.URL+"/sessions", "application/json", nil)
	var sessResult struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessResult)
	sessionID := sessResult.Data.SessionID

	// Upload PNG
	pngBytes := createTransparentTestPNG(200, 200)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "floorplan.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	resp, _ = client.Do(req)
	var uploadResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResult)
	datasetID := uploadResult.Data.ID

	// Add 2 ground control points to dataset
	cp1 := &models.ControlPoint{ImageX: 0, ImageY: 0, Longitude: 10.0, Latitude: 50.0}
	cp2 := &models.ControlPoint{ImageX: 200, ImageY: 200, Longitude: 11.0, Latitude: 49.0}
	_ = repo.InsertControlPoint(sessionID, datasetID, cp1)
	_ = repo.InsertControlPoint(sessionID, datasetID, cp2)

	// 1. GET /datasets/{id}/transform
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/transform", nil)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET transform failed: %v", err)
	}
	var getResp struct {
		Data struct {
			Status  string            `json:"status"`
			Corners gis.RasterCorners `json:"corners"`
			CanUndo bool              `json:"can_undo"`
			CanRedo bool              `json:"can_redo"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&getResp)
	if getResp.Data.CanUndo || getResp.Data.CanRedo {
		t.Errorf("expected can_undo and can_redo to be false initially")
	}

	initialCorners := getResp.Data.Corners

	// 2. POST apply move (+0.05 lon, +0.02 lat)
	movedCorners := gis.RasterCorners{
		TopLeft:     gis.CoordinatePoint{Longitude: initialCorners.TopLeft.Longitude + 0.05, Latitude: initialCorners.TopLeft.Latitude + 0.02},
		TopRight:    gis.CoordinatePoint{Longitude: initialCorners.TopRight.Longitude + 0.05, Latitude: initialCorners.TopRight.Latitude + 0.02},
		BottomRight: gis.CoordinatePoint{Longitude: initialCorners.BottomRight.Longitude + 0.05, Latitude: initialCorners.BottomRight.Latitude + 0.02},
		BottomLeft:  gis.CoordinatePoint{Longitude: initialCorners.BottomLeft.Longitude + 0.05, Latitude: initialCorners.BottomLeft.Latitude + 0.02},
	}
	applyBody, _ := json.Marshal(map[string]interface{}{
		"action":  "apply",
		"mode":    "move",
		"corners": movedCorners,
	})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/transform", bytes.NewReader(applyBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	var rawApplyResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&rawApplyResp)
	if err != nil || resp.StatusCode != http.StatusOK {
		rawJSON, _ := json.Marshal(rawApplyResp)
		t.Fatalf("POST transform apply failed: status %d body %s: %v", resp.StatusCode, string(rawJSON), err)
	}
	applyData, _ := rawApplyResp["data"].(map[string]interface{})
	canUndo, _ := applyData["can_undo"].(bool)
	if !canUndo {
		t.Errorf("expected CanUndo to be true after apply")
	}

	// Verify control points synchronized with moved corners
	cpsAfter, _ := repo.ListControlPoints(sessionID, datasetID)
	if len(cpsAfter) != 2 {
		t.Fatalf("expected 2 control points, got %d", len(cpsAfter))
	}
	// Check cp[0] moved by the same geographic displacement
	if mathAbs(cpsAfter[0].Longitude-movedCorners.TopLeft.Longitude) > 1e-4 ||
		mathAbs(cpsAfter[0].Latitude-movedCorners.TopLeft.Latitude) > 1e-4 {
		t.Errorf("control point not synchronized: expected (%f, %f), got (%f, %f)",
			movedCorners.TopLeft.Longitude, movedCorners.TopLeft.Latitude, cpsAfter[0].Longitude, cpsAfter[0].Latitude)
	}

	// 3. POST undo
	undoBody, _ := json.Marshal(map[string]interface{}{"action": "undo"})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/transform", bytes.NewReader(undoBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST undo failed: %v", err)
	}
	var undoResp struct {
		Data struct {
			Corners gis.RasterCorners `json:"corners"`
			CanUndo bool              `json:"can_undo"`
			CanRedo bool              `json:"can_redo"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&undoResp)
	if undoResp.Data.CanUndo || !undoResp.Data.CanRedo {
		t.Errorf("expected CanUndo=false, CanRedo=true after undo")
	}
	if mathAbs(undoResp.Data.Corners.TopLeft.Longitude-initialCorners.TopLeft.Longitude) > 1e-4 {
		t.Errorf("corners not restored after undo")
	}

	// 4. POST redo
	redoBody, _ := json.Marshal(map[string]interface{}{"action": "redo"})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/transform", bytes.NewReader(redoBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST redo failed: %v", err)
	}

	// 5. POST reset
	resetBody, _ := json.Marshal(map[string]interface{}{"action": "reset"})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/transform", bytes.NewReader(resetBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST reset failed: %v", err)
	}
	var resetResp struct {
		Data struct {
			Status  string            `json:"status"`
			Corners gis.RasterCorners `json:"corners"`
			CanUndo bool              `json:"can_undo"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&resetResp)
	if resetResp.Data.CanUndo {
		t.Errorf("expected CanUndo=false after reset")
	}
	if mathAbs(resetResp.Data.Corners.TopLeft.Longitude-initialCorners.TopLeft.Longitude) > 1e-4 {
		t.Errorf("corners not restored to initial on reset")
	}

	// 6. Degenerate geometry rejection
	badBody, _ := json.Marshal(map[string]interface{}{
		"action": "apply",
		"corners": gis.RasterCorners{
			TopLeft:     gis.CoordinatePoint{Longitude: 10, Latitude: 50},
			TopRight:    gis.CoordinatePoint{Longitude: 10, Latitude: 50}, // Collapsed!
			BottomRight: gis.CoordinatePoint{Longitude: 11, Latitude: 49},
			BottomLeft:  gis.CoordinatePoint{Longitude: 11, Latitude: 49},
		},
	})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/transform", bytes.NewReader(badBody))
	req.Header.Set("X-Session-ID", sessionID)
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for degenerate corners, got %d", resp.StatusCode)
	}
}

func TestMapProject_ImportEndpoint(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Create Session
	resp, _ := client.Post(server.URL+"/sessions", "application/json", nil)
	var sessResult struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessResult)
	sessionID := sessResult.Data.SessionID

	// 2. Upload PNG
	pngBytes := createTransparentTestPNG(200, 200)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "floorplan.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	resp, _ = client.Do(req)
	var uploadResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResult)
	datasetID := uploadResult.Data.ID

	// 3. Add Control Points
	points := []map[string]interface{}{
		{"image_x": 0.0, "image_y": 0.0, "latitude": 37.78, "longitude": -122.42},
		{"image_x": 200.0, "image_y": 0.0, "latitude": 37.78, "longitude": -122.40},
		{"image_x": 200.0, "image_y": 200.0, "latitude": 37.76, "longitude": -122.40},
		{"image_x": 0.0, "image_y": 200.0, "latitude": 37.76, "longitude": -122.42},
	}
	for _, p := range points {
		pBytes, _ := json.Marshal(p)
		req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/control-points", bytes.NewReader(pBytes))
		req.Header.Set("X-Session-ID", sessionID)
		_, _ = client.Do(req)
	}

	// Georeference
	geoBody, _ := json.Marshal(map[string]string{"transformation_type": "affine"})
	req, _ = http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/georeference", bytes.NewReader(geoBody))
	req.Header.Set("X-Session-ID", sessionID)
	_, _ = client.Do(req)

	// 4. Export as mtmap ZIP
	req, _ = http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/export?format=mtmap", nil)
	req.Header.Set("X-Session-ID", sessionID)
	exportResp, err := client.Do(req)
	if err != nil || exportResp.StatusCode != http.StatusOK {
		t.Fatalf("failed exporting dataset: %v, status: %d", err, exportResp.StatusCode)
	}
	exportZipBytes, _ := io.ReadAll(exportResp.Body)
	_ = exportResp.Body.Close()

	// 5. Test POST /map-projects/import with the exported project zip
	importBody := &bytes.Buffer{}
	importWriter := multipart.NewWriter(importBody)
	filePart, _ := importWriter.CreateFormFile("file", "reopened_project.zip")
	_, _ = filePart.Write(exportZipBytes)
	_ = importWriter.Close()

	importReq, _ := http.NewRequest("POST", server.URL+"/map-projects/import", importBody)
	importReq.Header.Set("Content-Type", importWriter.FormDataContentType())
	importReq.Header.Set("X-Session-ID", sessionID)
	importResp, err := client.Do(importReq)
	if err != nil {
		t.Fatalf("POST /map-projects/import failed: %v", err)
	}
	if importResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /map-projects/import expected status 201, got %d", importResp.StatusCode)
	}

	var importResult struct {
		Success bool `json:"success"`
		Data    struct {
			ID                 string `json:"id"`
			IsGeoreferenced    bool   `json:"is_georeferenced"`
			GeoreferenceStatus string `json:"georeference_status"`
		} `json:"data"`
	}
	_ = json.NewDecoder(importResp.Body).Decode(&importResult)
	if !importResult.Success || importResult.Data.ID == "" {
		t.Errorf("expected successful import with valid dataset ID, got %+v", importResult)
	}
	if !importResult.Data.IsGeoreferenced || importResult.Data.GeoreferenceStatus != "completed" {
		t.Errorf("expected imported project to have completed georeference, got %+v", importResult.Data)
	}
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
