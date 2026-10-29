package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestZipExport_ExactRootStructureAndViewers(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)
	client := server.Client()

	sess := repo.CreateSession()
	sessionID := sess.ID

	// 1. Upload a test image
	pngBytes := createTestPNG(256, 256)
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "campus_map.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %v", err)
	}
	var uploadRes struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadRes)
	_ = resp.Body.Close()
	datasetID := uploadRes.Data.ID

	// 2. Add 4 control points to make it georeferenced
	controlPts := []map[string]interface{}{
		{"image_x": 0.0, "image_y": 0.0, "longitude": -122.42, "latitude": 37.78},
		{"image_x": 250.0, "image_y": 0.0, "longitude": -122.40, "latitude": 37.78},
		{"image_x": 250.0, "image_y": 250.0, "longitude": -122.40, "latitude": 37.76},
		{"image_x": 0.0, "image_y": 250.0, "longitude": -122.42, "latitude": 37.76},
	}
	for _, pt := range controlPts {
		b, _ := json.Marshal(pt)
		r, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/control-points", bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Session-ID", sessionID)
		cpResp, err := client.Do(r)
		if err != nil || cpResp.StatusCode != http.StatusCreated {
			t.Fatalf("failed creating control point: %v", err)
		}
		_ = cpResp.Body.Close()
	}

	// Calculate georeference
	geoReq := map[string]string{"action": "calculate", "transformation": "affine"}
	geoBytes, _ := json.Marshal(geoReq)
	r, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/georeference", bytes.NewReader(geoBytes))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Session-ID", sessionID)
	gResp, err := client.Do(r)
	if err != nil || gResp.StatusCode != http.StatusOK {
		t.Fatalf("failed georeferencing dataset: %v", err)
	}
	_ = gResp.Body.Close()

	// 3. Export as ZIP
	expReq, _ := http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/export", nil)
	expReq.Header.Set("X-Session-ID", sessionID)
	expResp, err := client.Do(expReq)
	if err != nil || expResp.StatusCode != http.StatusOK {
		t.Fatalf("export GET failed: %v", err)
	}
	defer expResp.Body.Close()

	zipBytes, err := io.ReadAll(expResp.Body)
	if err != nil {
		t.Fatalf("failed reading export zip bytes: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("exported data is not a valid zip archive: %v", err)
	}

	files := make(map[string]*zip.File)
	numberedDirZooms := make(map[int]bool)

	for _, f := range zipReader.File {
		files[f.Name] = f

		// Check for no wrapping parent directory
		if strings.HasPrefix(f.Name, "export/") || strings.HasPrefix(f.Name, "output/") || strings.HasPrefix(f.Name, "veracity-maps-export/") {
			t.Errorf("unwanted wrapping parent folder found in archive path: %s", f.Name)
		}

		// Check for forward slash separators
		if strings.Contains(f.Name, "\\") {
			t.Errorf("archive entry contains backslash: %s", f.Name)
		}

		// Detect numbered zoom directories
		parts := strings.Split(f.Name, "/")
		if len(parts) >= 2 {
			if z, err := strconv.Atoi(parts[0]); err == nil {
				numberedDirZooms[z] = true
			}
		}
	}

	// 4. Verify all root-level viewer and metadata files
	expectedRootFiles := []string{
		"index.html",
		"viewer.html",
		"style.json",
		"mapstudio.json",
		"metadata.json",
		"README.txt",
	}

	expectedRootMap := make(map[string]bool)
	for _, name := range expectedRootFiles {
		expectedRootMap[name] = true
		zf, exists := files[name]
		if !exists {
			t.Errorf("expected root file missing: %s", name)
			continue
		}
		if zf.UncompressedSize64 == 0 {
			t.Errorf("root file is empty: %s", name)
		}
	}

	// 5. Verify NO unwanted folders or extra root files exist (strict Viewscape compatibility)
	for name := range files {
		parts := strings.Split(name, "/")
		if len(parts) == 1 {
			if !expectedRootMap[name] {
				t.Errorf("unwanted root file found in export archive: %s", name)
			}
		} else {
			// Top-level directory must be either a numeric zoom level or 'assets'
			if parts[0] != "assets" {
				if _, err := strconv.Atoi(parts[0]); err != nil {
					t.Errorf("unwanted non-numeric folder '%s' in archive entry: %s", parts[0], name)
				}
			}
		}
	}

	// Verify legacy viewers are completely removed
	legacyViewers := []string{"leaflet.html", "openlayers.html", "googlemaps.html"}
	for _, lv := range legacyViewers {
		if _, exists := files[lv]; exists {
			t.Errorf("obsolete viewer '%s' MUST NOT be included in export archive", lv)
		}
	}

	// 6. Verify metadata.json contents
	if metaFile, ok := files["metadata.json"]; ok {
		rc, err := metaFile.Open()
		if err != nil {
			t.Fatalf("failed opening metadata.json: %v", err)
		}
		defer rc.Close()

		var metaDoc map[string]interface{}
		if err := json.NewDecoder(rc).Decode(&metaDoc); err != nil {
			t.Fatalf("metadata.json is not valid JSON: %v", err)
		}

		if metaDoc["name"] == nil || metaDoc["bounds"] == nil {
			t.Errorf("metadata.json missing required fields: %v", metaDoc)
		}
		if metaDoc["format"] != "png" {
			t.Errorf("expected format 'png', got: %v", metaDoc["format"])
		}
		if metaDoc["scale"] != "1.000000" {
			t.Errorf("expected scale '1.000000', got: %v", metaDoc["scale"])
		}
		if metaDoc["control_points"] == nil {
			t.Errorf("expected control_points preserved in metadata.json")
		}
	}

	// 7. Verify mapstudio.json and style.json
	if msFile, ok := files["mapstudio.json"]; ok {
		rc, err := msFile.Open()
		if err == nil {
			defer rc.Close()
			var msDoc map[string]interface{}
			if err := json.NewDecoder(rc).Decode(&msDoc); err != nil {
				t.Errorf("mapstudio.json is not valid JSON: %v", err)
			} else {
				if msDoc["format"] != "mapstudio-folder" {
					t.Errorf("expected mapstudio.json format 'mapstudio-folder', got: %v", msDoc["format"])
				}
			}
		}
	} else {
		t.Errorf("expected mapstudio.json in export archive")
	}

	if styleFile, ok := files["style.json"]; ok {
		rc, err := styleFile.Open()
		if err == nil {
			defer rc.Close()
			var styleDoc map[string]interface{}
			if err := json.NewDecoder(rc).Decode(&styleDoc); err != nil {
				t.Errorf("style.json is not valid JSON: %v", err)
			} else {
				if v, ok := styleDoc["version"].(float64); !ok || int(v) != 8 {
					t.Errorf("expected style.json version 8, got: %v", styleDoc["version"])
				}
				sources, _ := styleDoc["sources"].(map[string]interface{})
				if sources == nil || sources["overlay"] == nil || sources["osm"] == nil {
					t.Errorf("expected style.json to define 'osm' and 'overlay' sources: %v", sources)
				}
			}
		}
	} else {
		t.Errorf("expected style.json in export archive")
	}

	// 8. Verify numbered zoom directories exist and tiles are {z}/{x}/{y}.png
	if len(numberedDirZooms) == 0 {
		t.Errorf("expected at least one numbered zoom folder (e.g. 15, 16, etc.) in georeferenced export, got 0")
	}
	for name := range files {
		parts := strings.Split(name, "/")
		if len(parts) == 3 && parts[0] != "assets" {
			if !strings.HasSuffix(name, ".png") {
				t.Errorf("tile entry should end with .png, but got: %s", name)
			}
		}
	}

	// 9. Verify HTML files use MapLibre GL JS and have no Leaflet references
	checkHTML := func(name string, requiredKeywords []string, forbiddenKeywords []string) {
		zf, ok := files[name]
		if !ok {
			t.Errorf("missing expected viewer file: %s", name)
			return
		}
		rc, err := zf.Open()
		if err != nil {
			t.Errorf("failed opening %s: %v", name, err)
			return
		}
		defer rc.Close()
		contentBytes, _ := io.ReadAll(rc)
		content := string(contentBytes)

		for _, kw := range requiredKeywords {
			if !strings.Contains(content, kw) {
				t.Errorf("%s missing expected keyword '%s'", name, kw)
			}
		}

		for _, fkw := range forbiddenKeywords {
			if strings.HasPrefix(fkw, "L.") {
				if strings.Contains(content, fkw) {
					t.Errorf("%s contains forbidden keyword '%s'", name, fkw)
				}
			} else if strings.Contains(strings.ToLower(content), strings.ToLower(fkw)) {
				t.Errorf("%s contains forbidden keyword '%s'", name, fkw)
			}
		}
	}

	checkHTML("viewer.html", []string{"maplibregl", "MapLibre", "overlay-tiles", "opacity-slider"}, []string{"leaflet", "L.map", "L.tileLayer", "google.maps", "openlayers"})
	checkHTML("index.html", []string{"maplibregl", "MapLibre", "overlay-tiles"}, []string{"leaflet", "L.map", "L.tileLayer", "google.maps", "openlayers"})

	// 9. Verify Roundtrip Re-opening: Uploading this exported ZIP to a fresh session succeeds
	newSess := repo.CreateSession()
	var reimportBuf bytes.Buffer
	mw := multipart.NewWriter(&reimportBuf)
	formPart, err := mw.CreateFormFile("file", "viewscape_export.zip")
	if err != nil {
		t.Fatalf("failed creating form file: %v", err)
	}
	_, _ = formPart.Write(zipBytes)
	_ = mw.Close()

	reimportReq, _ := http.NewRequest("POST", server.URL+"/upload", &reimportBuf)
	reimportReq.Header.Set("Content-Type", mw.FormDataContentType())
	reimportReq.Header.Set("X-Session-ID", newSess.ID)
	reimportResp, err := client.Do(reimportReq)
	if err != nil || reimportResp.StatusCode != http.StatusCreated {
		t.Fatalf("re-uploading exported zip failed: %v, status: %d", err, reimportResp.StatusCode)
	}
	defer reimportResp.Body.Close()

	var reimportDoc struct {
		Data struct {
			ID                 string `json:"id"`
			GeoreferenceStatus string `json:"georeference_status"`
		} `json:"data"`
	}
	_ = json.NewDecoder(reimportResp.Body).Decode(&reimportDoc)
	if reimportDoc.Data.ID == "" {
		t.Errorf("re-imported dataset ID is empty")
	}
	if reimportDoc.Data.GeoreferenceStatus != "completed" {
		t.Errorf("expected re-imported dataset georeference_status 'completed', got: %s", reimportDoc.Data.GeoreferenceStatus)
	}

	// Verify control points restored in new session
	restoredPts, err := repo.ListControlPoints(newSess.ID, reimportDoc.Data.ID)
	if err != nil || len(restoredPts) != 4 {
		t.Errorf("expected 4 restored control points in re-imported session, got %d (err: %v)", len(restoredPts), err)
	}
}
