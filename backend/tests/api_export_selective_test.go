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
	"github.com/malikharman0823/veracity-maps/services"
)

func TestSelectiveExport_HierarchyAndOptions(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)
	client := server.Client()

	sess := repo.CreateSession()
	sessionID := sess.ID

	// Upload two datasets
	uploadDS := func(filename string) string {
		pngBytes := createTestPNG(200, 150)
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

	ds1ID := uploadDS("floor_1.png")
	ds2ID := uploadDS("floor_2.png")

	// Create building with two floors
	bld, err := repo.CreateBuilding(sessionID, &models.Building{
		Name: "Tower Alpha",
		Floors: []*models.BuildingFloor{
			{ID: "flr_1", Level: 1, Name: "Level 1", DatasetIDs: []string{ds1ID}},
			{ID: "flr_2", Level: 2, Name: "Level 2", DatasetIDs: []string{ds2ID}},
		},
	})
	if err != nil {
		t.Fatalf("failed creating building: %v", err)
	}

	// Add icon point to ds1
	_, err = repo.CreateIconPoint(sessionID, &models.IconPoint{
		ID:         "ip_test_1",
		DatasetID:  ds1ID,
		BuildingID: bld.ID,
		Name:       "Fire Extinguisher",
		IconID:     "001-camera",
		Longitude:  -122.4,
		Latitude:   37.7,
	})
	if err != nil {
		t.Fatalf("failed creating icon point: %v", err)
	}

	// 1. Test POST selective export selecting only floor 1 (ds1ID)
	exportReqBody, _ := json.Marshal(export.ExportRequest{
		Format:      "mtmap",
		ProjectName: "Selective Test Project",
		LayerIDs:    []string{ds1ID},
		Options: &export.ExportOptions{
			IncludeBuildingConfig:  true,
			IncludeControlPoints:   true,
			IncludeIcons:           true,
			IncludeLayerStyles:     true,
			IncludeOriginalRaster:  true,
			IncludeProcessedRaster: true,
			IncludeProjectMetadata: true,
		},
	})

	req, _ := http.NewRequest("POST", server.URL+"/datasets/"+ds1ID+"/export", bytes.NewReader(exportReqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/zip")
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("export request failed: %v, status: %d", err, resp.StatusCode)
	}
	defer resp.Body.Close()

	zipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading export zip bytes: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("failed reading export as zip archive: %v", err)
	}

	entries := make(map[string]*zip.File)
	for _, f := range zipReader.File {
		entries[f.Name] = f
	}

	// Verify required entries
	if entries["manifest.json"] == nil {
		t.Errorf("manifest.json missing from export zip")
	}
	if entries["project.json"] == nil {
		t.Errorf("project.json missing from export zip")
	}
	if entries["README.txt"] == nil {
		t.Errorf("README.txt missing from export zip")
	}
	if entries["layers/"+ds1ID+"/layer.json"] == nil {
		t.Errorf("layer.json for ds1 missing")
	}
	if entries["layers/"+ds2ID+"/layer.json"] != nil {
		t.Errorf("unselected ds2 layer.json should NOT be present in selective export")
	}
	if entries["icons/icon_points.json"] == nil {
		t.Errorf("icons/icon_points.json should be present")
	}

	// Inspect manifest.json
	manifestFile, _ := entries["manifest.json"].Open()
	var manifest services.ExportManifest
	_ = json.NewDecoder(manifestFile).Decode(&manifest)
	_ = manifestFile.Close()

	if manifest.FormatVersion != "2.0.0" {
		t.Errorf("expected format_version 2.0.0, got %s", manifest.FormatVersion)
	}
	if len(manifest.SelectedLayers) != 1 || manifest.SelectedLayers[0] != ds1ID {
		t.Errorf("expected SelectedLayers to be [%s], got %v", ds1ID, manifest.SelectedLayers)
	}

	// 2. Test Roundtrip Import with Reopen /map-projects/import
	importBody := new(bytes.Buffer)
	mpWriter := multipart.NewWriter(importBody)
	part, _ := mpWriter.CreateFormFile("file", "reopened_export.zip")
	_, _ = part.Write(zipBytes)
	_ = mpWriter.Close()

	importReq, _ := http.NewRequest("POST", server.URL+"/map-projects/import", importBody)
	importReq.Header.Set("Content-Type", mpWriter.FormDataContentType())
	importReq.Header.Set("X-Session-ID", sessionID)
	importResp, err := client.Do(importReq)
	if err != nil || importResp.StatusCode != http.StatusCreated {
		t.Fatalf("reopening exported zip failed: %v, status: %d", err, importResp.StatusCode)
	}
	_ = importResp.Body.Close()

	// 3. Test validation error when invalid raster options provided
	emptyReqBody, _ := json.Marshal(export.ExportRequest{
		Format:   "mtmap",
		LayerIDs: []string{},
		Options: &export.ExportOptions{
			IncludeOriginalRaster:  false,
			IncludeProcessedRaster: false,
		},
	})
	emptyReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+ds1ID+"/export", bytes.NewReader(emptyReqBody))
	emptyReq.Header.Set("Content-Type", "application/json")
	emptyReq.Header.Set("X-Session-ID", sessionID)
	emptyResp, err := client.Do(emptyReq)
	if err != nil {
		t.Fatalf("empty export request error: %v", err)
	}
	defer emptyResp.Body.Close()
	if emptyResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid options/layers, got %d", emptyResp.StatusCode)
	}
}
