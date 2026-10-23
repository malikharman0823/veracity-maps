package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
)

// generateTestPdf generates a simple 2-page test PDF using python
func generateTestPdf(t *testing.T, outPath string) {
	cmd := exec.Command("python", "-c", fmt.Sprintf(`
import pymupdf
doc = pymupdf.open()
for i in range(2):
    page = doc.new_page(width=595, height=842) # A4
    page.draw_rect([50, 50, 545, 792], color=(0, 0, 1), fill=(0.9, 0.9, 0.9))
    page.insert_text((72, 100), f"Veracity Maps Test Page {i+1}", fontsize=20, color=(0, 0, 0))
doc.save(r"%s")
doc.close()
`, outPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to generate test pdf: %v, out: %s", err, string(out))
	}
}

// generateTestDxf generates a simple DXF drawing using python ezdxf
func generateTestDxf(t *testing.T, outPath string) {
	cmd := exec.Command("python", "-c", fmt.Sprintf(`
import ezdxf
doc = ezdxf.new("R2010")
msp = doc.modelspace()
msp.add_lwpolyline([(0, 0), (100, 0), (100, 80), (0, 80), (0, 0)], close=True)
msp.add_text("ROOM 101", height=5).set_placement((20, 40))
doc.saveas(r"%s")
`, outPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to generate test dxf: %v, out: %s", err, string(out))
	}
}

func helperCreateSession(t *testing.T, serverURL string) string {
	resp, err := http.Post(serverURL+"/sessions", "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("session creation failed: %v, status: %d", err, resp.StatusCode)
	}
	defer resp.Body.Close()
	var sessResult struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&sessResult)
	if sessResult.Data.SessionID == "" {
		t.Fatalf("expected non-empty session ID")
	}
	return sessResult.Data.SessionID
}

func helperAddControlPoint(t *testing.T, serverURL, sessionID, datasetID string, x, y, lon, lat float64) {
	ptBytes, _ := json.Marshal(map[string]float64{
		"image_x":   x,
		"image_y":   y,
		"longitude": lon,
		"latitude":  lat,
	})
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/datasets/%s/control-points", serverURL, datasetID), bytes.NewReader(ptBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed adding control point: %v", err)
	}
	_ = resp.Body.Close()
}

func helperCalcGeoreference(t *testing.T, serverURL, sessionID, datasetID string) {
	geoReqBytes, _ := json.Marshal(map[string]string{"action": "calculate", "transformation": "affine"})
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/datasets/%s/georeference", serverURL, datasetID), bytes.NewReader(geoReqBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("georeference calculation failed: %v", err)
	}
	_ = resp.Body.Close()
}

func TestPDFMultiPage_InspectionAndSelectiveImport(t *testing.T) {
	ts, _, tempDir := createTestServer(t)
	defer ts.Close()
	defer os.RemoveAll(tempDir)

	// 1. Create Session
	sessionID := helperCreateSession(t, ts.URL)

	// 2. Generate a 2-page test PDF
	pdfPath := filepath.Join(tempDir, "floorplan_multipage.pdf")
	generateTestPdf(t, pdfPath)
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("failed to read test pdf: %v", err)
	}

	// 3. Upload the PDF
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "floorplan_multipage.pdf")
	_, _ = part.Write(pdfBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("upload pdf request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK for pdf inspection upload, got %d: %s", resp.StatusCode, string(b))
	}

	var uploadRes struct {
		Success bool `json:"success"`
		Data    struct {
			Type       string                `json:"type"`
			DocumentID string                `json:"document_id"`
			Filename   string                `json:"filename"`
			FileSize   int64                 `json:"file_size"`
			PageCount  int                   `json:"page_count"`
			Pages      []*models.PdfPageInfo `json:"pages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploadRes); err != nil {
		t.Fatalf("failed decoding upload response: %v", err)
	}

	if uploadRes.Data.Type != "pdf_pending" {
		t.Fatalf("expected type 'pdf_pending', got '%s'", uploadRes.Data.Type)
	}
	if uploadRes.Data.PageCount != 2 {
		t.Fatalf("expected 2 pages, got %d", uploadRes.Data.PageCount)
	}
	if uploadRes.Data.DocumentID == "" {
		t.Fatalf("expected document_id to be populated")
	}

	docID := uploadRes.Data.DocumentID

	// 4. Test Thumbnail Endpoint for Page 1
	thumbReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/documents/%s/pdf-thumbnail/1", ts.URL, docID), nil)
	thumbReq.Header.Set("X-Session-ID", sessionID)
	thumbResp, err := ts.Client().Do(thumbReq)
	if err != nil {
		t.Fatalf("thumbnail request failed: %v", err)
	}
	defer thumbResp.Body.Close()
	if thumbResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for thumbnail, got %d", thumbResp.StatusCode)
	}
	if thumbResp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("expected thumbnail Content-Type 'image/png', got %s", thumbResp.Header.Get("Content-Type"))
	}

	// 5. Test Import PDF Pages (Selective: Page 1 and Page 2 in Shared Mode)
	importReqPayload := models.ImportPdfPagesRequest{
		DocumentID:         docID,
		SelectedPages:      []int{1, 2},
		GeoreferencingMode: "shared",
		BuildingName:       "Corporate HQ",
	}
	payloadBytes, _ := json.Marshal(importReqPayload)

	importReq, _ := http.NewRequest("POST", ts.URL+"/datasets/import-pdf-pages", bytes.NewReader(payloadBytes))
	importReq.Header.Set("Content-Type", "application/json")
	importReq.Header.Set("X-Session-ID", sessionID)

	importResp, err := ts.Client().Do(importReq)
	if err != nil {
		t.Fatalf("import-pdf-pages request failed: %v", err)
	}
	defer importResp.Body.Close()

	if importResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(importResp.Body)
		t.Fatalf("expected 200 OK for import-pdf-pages, got %d: %s", importResp.StatusCode, string(b))
	}

	var importRes struct {
		Success bool                          `json:"success"`
		Data    models.ImportPdfPagesResponse `json:"data"`
	}
	if err := json.NewDecoder(importResp.Body).Decode(&importRes); err != nil {
		t.Fatalf("failed decoding import response: %v", err)
	}

	if len(importRes.Data.Datasets) != 2 {
		t.Fatalf("expected 2 datasets created, got %d", len(importRes.Data.Datasets))
	}

	ds1 := importRes.Data.Datasets[0]
	ds2 := importRes.Data.Datasets[1]

	if ds1.ExtractedMetadata["document_type"] != "pdf" {
		t.Fatalf("expected document_type pdf, got %v", ds1.ExtractedMetadata["document_type"])
	}
	if ds1.ExtractedMetadata["georeferencing_mode"] != "shared" {
		t.Fatalf("expected shared georef mode, got %v", ds1.ExtractedMetadata["georeferencing_mode"])
	}

	// 6. Georeference Page 1 with Control Points to verify Shared Mode propagation
	helperAddControlPoint(t, ts.URL, sessionID, ds1.ID, 0, 0, -122.4194, 37.7749)
	helperAddControlPoint(t, ts.URL, sessionID, ds1.ID, 500, 0, -122.4184, 37.7749)
	helperAddControlPoint(t, ts.URL, sessionID, ds1.ID, 500, 700, -122.4184, 37.7739)
	helperAddControlPoint(t, ts.URL, sessionID, ds1.ID, 0, 700, -122.4194, 37.7739)

	helperCalcGeoreference(t, ts.URL, sessionID, ds1.ID)

	// Verify that Page 2 also inherited the transformation in shared mode
	getReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/datasets/%s", ts.URL, ds2.ID), nil)
	getReq.Header.Set("X-Session-ID", sessionID)
	getResp, err := ts.Client().Do(getReq)
	if err != nil {
		t.Fatalf("failed fetching ds2: %v", err)
	}
	defer getResp.Body.Close()

	var ds2Res struct {
		Success bool           `json:"success"`
		Data    models.Dataset `json:"data"`
	}
	_ = json.NewDecoder(getResp.Body).Decode(&ds2Res)

	if ds2Res.Data.GeoreferenceStatus != "completed" {
		t.Fatalf("expected ds2 to be georeferenced via shared building propagation, got %s", ds2Res.Data.GeoreferenceStatus)
	}
}

func TestCAD_DXFUploadAndConversion(t *testing.T) {
	ts, _, tempDir := createTestServer(t)
	defer ts.Close()
	defer os.RemoveAll(tempDir)

	// 1. Create Session
	sessionID := helperCreateSession(t, ts.URL)

	// 2. Generate a test DXF file
	dxfPath := filepath.Join(tempDir, "floorplan.dxf")
	generateTestDxf(t, dxfPath)
	dxfBytes, err := os.ReadFile(dxfPath)
	if err != nil {
		t.Fatalf("failed to read test dxf: %v", err)
	}

	// 3. Upload the DXF
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "floorplan.dxf")
	_, _ = part.Write(dxfBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("upload dxf request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200/201 for dxf upload, got %d: %s", resp.StatusCode, string(b))
	}

	var uploadRes struct {
		Success bool `json:"success"`
		Data    struct {
			ID       string                 `json:"id"`
			Filename string                 `json:"filename"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploadRes); err != nil {
		t.Fatalf("failed decoding upload response: %v", err)
	}

	meta := uploadRes.Data.Metadata
	if meta["document_type"] != "cad" {
		t.Fatalf("expected document_type cad, got %v", meta["document_type"])
	}
	if meta["cad_format"] != "dxf" {
		t.Fatalf("expected cad_format dxf, got %v", meta["cad_format"])
	}
	w, _ := meta["width"].(float64)
	h, _ := meta["height"].(float64)
	if w <= 0 || h <= 0 {
		t.Fatalf("expected valid raster dimensions, got %vx%v", meta["width"], meta["height"])
	}

	// 4. Verify raster image exists and is readable via source-image
	rasterReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/datasets/%s/source-image", ts.URL, uploadRes.Data.ID), nil)
	rasterReq.Header.Set("X-Session-ID", sessionID)
	rasterResp, err := ts.Client().Do(rasterReq)
	if err != nil {
		t.Fatalf("failed fetching raster: %v", err)
	}
	defer rasterResp.Body.Close()

	if rasterResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for raster, got %d", rasterResp.StatusCode)
	}
}
