package tests

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/storage"
)

// TestTMS_XYZ_Conversion verifies the mathematical correctness and grid limits of TMS-to-XYZ conversion.
func TestTMS_XYZ_Conversion(t *testing.T) {
	// Zoom 0: 1x1 grid (tms 0 -> xyz 0)
	xyz0, err := gis.TMSToXYZ(0, 0)
	if err != nil || xyz0 != 0 {
		t.Fatalf("expected TMSToXYZ(0, 0) == 0, got %d, err: %v", xyz0, err)
	}
	tms0, err := gis.XYZToTMS(0, 0)
	if err != nil || tms0 != 0 {
		t.Fatalf("expected XYZToTMS(0, 0) == 0, got %d, err: %v", tms0, err)
	}

	// Zoom 1: 2x2 grid (limit 2). tms 0 (south) -> xyz 1 (south); tms 1 (north) -> xyz 0 (north)
	xyz1_0, err := gis.TMSToXYZ(0, 1)
	if err != nil || xyz1_0 != 1 {
		t.Errorf("expected TMSToXYZ(0, 1) == 1, got %d", xyz1_0)
	}
	xyz1_1, err := gis.TMSToXYZ(1, 1)
	if err != nil || xyz1_1 != 0 {
		t.Errorf("expected TMSToXYZ(1, 1) == 0, got %d", xyz1_1)
	}

	// Roundtrip verification across various zoom levels
	testZooms := []int{2, 5, 10, 15, 18}
	for _, z := range testZooms {
		limit := 1 << z
		testRows := []int{0, 1, limit / 2, limit - 2, limit - 1}
		for _, row := range testRows {
			xyz, err := gis.TMSToXYZ(row, z)
			if err != nil {
				t.Errorf("TMSToXYZ failed for z=%d, row=%d: %v", z, row, err)
				continue
			}
			reverted, err := gis.XYZToTMS(xyz, z)
			if err != nil || reverted != row {
				t.Errorf("roundtrip failed for z=%d, row=%d: xyz=%d, reverted=%d, err=%v", z, row, xyz, reverted, err)
			}
		}
	}

	// Error cases: out-of-bounds row and invalid zoom
	if _, err := gis.TMSToXYZ(-1, 5); err == nil {
		t.Errorf("expected error for negative row")
	}
	if _, err := gis.TMSToXYZ(32, 5); err == nil {
		t.Errorf("expected error for row >= (1<<5)")
	}
	if _, err := gis.TMSToXYZ(0, -1); err == nil {
		t.Errorf("expected error for negative zoom")
	}
	if _, err := gis.TMSToXYZ(0, 31); err == nil {
		t.Errorf("expected error for zoom > 30")
	}
}

// TestMapLibreZipExport_FullContentVerification tests that exporting a georeferenced dataset
// produces a 100% MapLibre-compliant ZIP package matching the MapTiler folder structure.
func TestMapLibreZipExport_FullContentVerification(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer storage.SafeRemoveAll(tempDir)

	client := server.Client()

	// 1. Create anonymous session
	sess := repo.CreateSession()
	sessionID := sess.ID

	// 2. Upload valid synthetic test raster
	gridBytes := createTestPNG(256, 256)

	var uploadBuf bytes.Buffer
	mw := multipart.NewWriter(&uploadBuf)
	filePart, err := mw.CreateFormFile("file", "test_campus.png")
	if err != nil {
		t.Fatalf("failed creating multipart form: %v", err)
	}
	_, _ = filePart.Write(gridBytes)
	_ = mw.Close()

	upReq, _ := http.NewRequest("POST", server.URL+"/upload", &uploadBuf)
	upReq.Header.Set("Content-Type", mw.FormDataContentType())
	upReq.Header.Set("X-Session-ID", sessionID)
	upResp, err := client.Do(upReq)
	if err != nil || upResp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %v, status: %d", err, upResp.StatusCode)
	}

	var upData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(upResp.Body).Decode(&upData)
	_ = upResp.Body.Close()

	datasetID := upData.Data.ID
	if datasetID == "" {
		t.Fatalf("expected dataset ID from upload")
	}

	// 3. Add Control Points (San Francisco campus coordinates)
	addCP := func(ix, iy, lng, lat float64) {
		cpBody, _ := json.Marshal(map[string]interface{}{
			"image_x":   ix,
			"image_y":   iy,
			"longitude": lng,
			"latitude":  lat,
		})
		req, _ := http.NewRequest("POST", fmt.Sprintf("%s/datasets/%s/control-points", server.URL, datasetID), bytes.NewReader(cpBody))
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

	// 4. Trigger Georeferencing
	geoData := map[string]string{"action": "calculate", "transformation": "affine"}
	geoBytes, _ := json.Marshal(geoData)
	geoReq, _ := http.NewRequest("POST", fmt.Sprintf("%s/datasets/%s/georeference", server.URL, datasetID), bytes.NewReader(geoBytes))
	geoReq.Header.Set("Content-Type", "application/json")
	geoReq.Header.Set("X-Session-ID", sessionID)
	geoResp, err := client.Do(geoReq)
	if err != nil || geoResp.StatusCode != http.StatusOK {
		t.Fatalf("georeference failed: %v", err)
	}
	_ = geoResp.Body.Close()

	// 5. Add an Icon Point
	iconBody, _ := json.Marshal(map[string]interface{}{
		"name":        "Main Library",
		"description": "Campus Central Library",
		"icon_id":     "library",
		"longitude":   -122.4150,
		"latitude":    37.7750,
	})
	iconReq, _ := http.NewRequest("POST", fmt.Sprintf("%s/datasets/%s/icon-points", server.URL, datasetID), bytes.NewReader(iconBody))
	iconReq.Header.Set("Content-Type", "application/json")
	iconReq.Header.Set("X-Session-ID", sessionID)
	iconResp, err := client.Do(iconReq)
	if err != nil || iconResp.StatusCode != http.StatusCreated {
		t.Fatalf("icon point creation failed: %v", err)
	}
	_ = iconResp.Body.Close()

	// 6. Export as MapTiler-style ZIP package
	expReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/datasets/%s/export?format=mtmap", server.URL, datasetID), nil)
	expReq.Header.Set("X-Session-ID", sessionID)
	expResp, err := client.Do(expReq)
	if err != nil || expResp.StatusCode != http.StatusOK {
		t.Fatalf("export GET failed: %v, status: %d", err, expResp.StatusCode)
	}
	defer expResp.Body.Close()

	zipBytes, err := io.ReadAll(expResp.Body)
	if err != nil {
		t.Fatalf("failed reading export zip bytes: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("export is not a valid zip archive: %v", err)
	}

	entries := make(map[string]*zip.File)
	tileFiles := []string{}
	for _, f := range zipReader.File {
		entries[f.Name] = f
		if strings.HasSuffix(f.Name, ".png") && !strings.Contains(f.Name, "assets/") {
			tileFiles = append(tileFiles, f.Name)
		}
	}

	// 7. Verify Root Package Contents
	expectedFiles := []string{
		"metadata.json",
		"mapstudio.json",
		"style.json",
		"viewer.html",
		"index.html",
		"README.txt",
	}
	for _, exp := range expectedFiles {
		if _, ok := entries[exp]; !ok {
			t.Errorf("expected package file '%s' is missing from ZIP", exp)
		}
	}

	// 8. Verify ZERO Leaflet / OpenLayers / GoogleMaps references
	forbiddenEntries := []string{"leaflet.html", "openlayers.html", "googlemaps.html"}
	for _, fb := range forbiddenEntries {
		if _, ok := entries[fb]; ok {
			t.Errorf("legacy/forbidden entry '%s' found in export package", fb)
		}
	}

	// 9. Verify original source raster in assets/source/
	foundSourceRaster := false
	for name := range entries {
		if strings.HasPrefix(name, "assets/source/") && strings.HasSuffix(name, ".png") {
			foundSourceRaster = true
			break
		}
	}
	if !foundSourceRaster {
		t.Errorf("expected source raster preserved under assets/source/")
	}

	// 10. Verify style.json specification v8 conformance
	styleEntry := entries["style.json"]
	if styleEntry != nil {
		rc, _ := styleEntry.Open()
		var styleDoc map[string]interface{}
		if err := json.NewDecoder(rc).Decode(&styleDoc); err != nil {
			t.Errorf("style.json failed to parse as JSON: %v", err)
		} else {
			if v, ok := styleDoc["version"].(float64); !ok || int(v) != 8 {
				t.Errorf("style.json expected version 8, got: %v", styleDoc["version"])
			}
			sources, _ := styleDoc["sources"].(map[string]interface{})
			if sources == nil || sources["osm"] == nil || sources["overlay"] == nil {
				t.Errorf("style.json sources missing 'osm' or 'overlay': %v", sources)
			}
			layers, _ := styleDoc["layers"].([]interface{})
			if len(layers) < 2 {
				t.Errorf("style.json expected at least 2 layers, got %d", len(layers))
			}
		}
		_ = rc.Close()
	}

	// 11. Verify viewer.html contains MapLibre GL JS and controls
	viewerEntry := entries["viewer.html"]
	if viewerEntry != nil {
		rc, _ := viewerEntry.Open()
		contentBytes, _ := io.ReadAll(rc)
		_ = rc.Close()
		content := string(contentBytes)

		mustHave := []string{
			"maplibre-gl.js",
			"maplibre-gl.css",
			"new maplibregl.Map",
			"overlay-tiles",
			"opacity-slider",
			"visibility-toggle",
			"reset-view-btn",
		}
		for _, kw := range mustHave {
			if !strings.Contains(content, kw) {
				t.Errorf("viewer.html missing required keyword: '%s'", kw)
			}
		}

		mustNotHave := []string{
			"leaflet.js",
			"L.map",
			"L.tileLayer",
			"google.maps",
			"ol.Map",
		}
		for _, mnh := range mustNotHave {
			if strings.HasPrefix(mnh, "L.") || strings.HasPrefix(mnh, "ol.") {
				if strings.Contains(content, mnh) {
					t.Errorf("viewer.html contains forbidden library reference: '%s'", mnh)
				}
			} else if strings.Contains(strings.ToLower(content), strings.ToLower(mnh)) {
				t.Errorf("viewer.html contains forbidden library reference: '%s'", mnh)
			}
		}
	}

	// 12. Verify tiles follow {z}/{x}/{y}.png pattern
	if len(tileFiles) == 0 {
		t.Errorf("expected at least one raster tile in export package")
	}
	for _, tf := range tileFiles {
		parts := strings.Split(tf, "/")
		if len(parts) != 3 {
			t.Errorf("tile '%s' does not match {z}/{x}/{y}.png hierarchy", tf)
		}
		if !strings.HasSuffix(parts[2], ".png") {
			t.Errorf("tile '%s' does not have .png extension", tf)
		}
	}

	// 13. Verify mapstudio.json portable project document
	msEntry := entries["mapstudio.json"]
	if msEntry != nil {
		rc, _ := msEntry.Open()
		var msDoc map[string]interface{}
		if err := json.NewDecoder(rc).Decode(&msDoc); err != nil {
			t.Errorf("mapstudio.json failed to parse: %v", err)
		} else {
			if msDoc["format"] != "mapstudio-folder" {
				t.Errorf("expected mapstudio.json format 'mapstudio-folder', got: %v", msDoc["format"])
			}
			cps, _ := msDoc["control_points"].([]interface{})
			if len(cps) != 4 {
				t.Errorf("expected 4 control points in mapstudio.json, got: %d", len(cps))
			}
			ips, _ := msDoc["icon_points"].([]interface{})
			if len(ips) != 1 {
				t.Errorf("expected 1 icon point in mapstudio.json, got: %d", len(ips))
			}
		}
		_ = rc.Close()
	}

	// 14. Verify Roundtrip Import into Fresh Session
	freshSess := repo.CreateSession()
	var reimportBuf bytes.Buffer
	rmw := multipart.NewWriter(&reimportBuf)
	rPart, _ := rmw.CreateFormFile("file", "exported_map.zip")
	_, _ = rPart.Write(zipBytes)
	_ = rmw.Close()

	reimportReq, _ := http.NewRequest("POST", server.URL+"/upload", &reimportBuf)
	reimportReq.Header.Set("Content-Type", rmw.FormDataContentType())
	reimportReq.Header.Set("X-Session-ID", freshSess.ID)
	reimportResp, err := client.Do(reimportReq)
	if err != nil || reimportResp.StatusCode != http.StatusCreated {
		t.Fatalf("reimporting exported map package failed: %v, status: %d", err, reimportResp.StatusCode)
	}
	_ = reimportResp.Body.Close()

	// Check that control points were restored in fresh session
	dss, _ := repo.ListDatasets(freshSess.ID)
	if len(dss) == 0 {
		t.Fatalf("expected dataset restored in fresh session")
	}
	restoredDs := dss[0]
	restoredCPs, _ := repo.ListControlPoints(freshSess.ID, restoredDs.ID)
	if len(restoredCPs) != 4 {
		t.Errorf("expected 4 control points restored from mapstudio.json, got: %d", len(restoredCPs))
	}
}

// TestLegacyZipImport_BackwardCompatibility verifies that older exported packages
// (which may contain leaflet.html) are successfully imported without error.
func TestLegacyZipImport_BackwardCompatibility(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer storage.SafeRemoveAll(tempDir)

	// Build synthetic legacy archive containing leaflet.html alongside a valid raster
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	// Add leaflet.html
	lw, _ := zw.Create("leaflet.html")
	_, _ = lw.Write([]byte("<html><script src='leaflet.js'></script></html>"))

	// Add metadata.json
	mw, _ := zw.Create("metadata.json")
	meta := map[string]interface{}{
		"name":    "Legacy Project",
		"version": "1.0.0",
		"bounds":  []float64{-122.5, 37.7, -122.4, 37.8},
	}
	metaBytes, _ := json.Marshal(meta)
	_, _ = mw.Write(metaBytes)

	// Add raster
	rw, _ := zw.Create("floorplan.png")
	gridBytes := createTestPNG(256, 256)
	_, _ = rw.Write(gridBytes)
	_ = zw.Close()

	// Upload legacy zip
	sess := repo.CreateSession()
	var reqBuf bytes.Buffer
	mpw := multipart.NewWriter(&reqBuf)
	p, _ := mpw.CreateFormFile("file", "legacy_map.zip")
	_, _ = p.Write(zipBuf.Bytes())
	_ = mpw.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", &reqBuf)
	req.Header.Set("Content-Type", mpw.FormDataContentType())
	req.Header.Set("X-Session-ID", sess.ID)

	resp, err := server.Client().Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("uploading legacy zip failed: %v (status %d)", err, resp.StatusCode)
	}
	defer resp.Body.Close()

	datasetsList, _ := repo.ListDatasets(sess.ID)
	if len(datasetsList) != 1 {
		t.Fatalf("expected 1 dataset imported from legacy zip, got: %d", len(datasetsList))
	}
	if datasetsList[0].GeoreferenceStatus != "completed" {
		t.Errorf("expected georeference completed from legacy metadata bounds, got: %s", datasetsList[0].GeoreferenceStatus)
	}
}
