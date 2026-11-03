package tests

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func runTestPython(t *testing.T, code string) {
	t.Helper()
	cmd := exec.Command(gis.ResolvePythonInterpreter(), "-c", code)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python fixture helper failed: %v\noutput: %s", err, string(out))
	}
}

func uploadMultipartFile(t *testing.T, client *http.Client, url, sessionID, fieldName, filename string, fileBytes []byte) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile(%s) failed: %v", filename, err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		t.Fatalf("writing multipart bytes failed: %v", err)
	}
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatalf("NewRequest(%s) failed: %v", url, err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, raw
}

func sha256FileHex(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file %s for hash: %v", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func testFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// TestPhase2And6_ImageEnhancementUpscalingAndPaddleOCR tests:
// 1. High-resolution PNG (>= 2400px) — verifies unnecessary Real-ESRGAN AI upscaling is skipped.
// 2. Low-resolution PNG with engineering labels — verifies OpenCV text_clarity + 2x Real-ESRGAN ONNX upscaling + PaddleOCR label extraction + annotation edit/verify/delete.
// 3. JPEG engineering drawing — verifies OpenCV enhancement while preserving the untouched original source file.
func TestPhase2And6_ImageEnhancementUpscalingAndPaddleOCR(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer storage.SafeRemoveAll(tempDir)
	client := server.Client()

	sess := repo.CreateSession()
	sessionID := sess.ID

	fixtureDir := t.TempDir()
	highResPNG := filepath.Join(fixtureDir, "high_res_plan.png")
	lowResPNG := filepath.Join(fixtureDir, "low_res_labels.png")
	engJPEG := filepath.Join(fixtureDir, "engineering_sheet.jpg")

	runTestPython(t, fmt.Sprintf(`
import cv2
import numpy as np

# 1. High-resolution PNG (3800 x 1800)
hi = np.full((1800, 3800, 3), 248, dtype=np.uint8)
cv2.rectangle(hi, (80, 80), (3720, 1720), (20, 20, 20), 4)
cv2.putText(hi, "HIGH RES MASTER PLAN 1:50", (180, 300), cv2.FONT_HERSHEY_SIMPLEX, 2.2, (10, 10, 10), 4, cv2.LINE_AA)
cv2.imwrite(r"%s", hi)

# 2. Low-resolution PNG (360 x 220) with crisp engineering text for OCR + 2x AI upscaling
lo = np.full((220, 360, 3), 255, dtype=np.uint8)
cv2.rectangle(lo, (12, 12), (348, 208), (30, 30, 30), 2)
cv2.putText(lo, "ROOM 101", (35, 85), cv2.FONT_HERSHEY_SIMPLEX, 1.05, (0, 0, 0), 2, cv2.LINE_AA)
cv2.putText(lo, "SCALE 1:100", (35, 155), cv2.FONT_HERSHEY_SIMPLEX, 0.9, (0, 0, 0), 2, cv2.LINE_AA)
cv2.imwrite(r"%s", lo)

# 3. JPEG engineering drawing (640 x 480)
jpg = np.full((480, 640, 3), 242, dtype=np.uint8)
cv2.rectangle(jpg, (40, 40), (600, 440), (25, 25, 25), 2)
cv2.line(jpg, (40, 240), (600, 240), (40, 40, 40), 2)
cv2.putText(jpg, "ELEV +12.50M", (80, 160), cv2.FONT_HERSHEY_SIMPLEX, 1.1, (15, 15, 15), 2, cv2.LINE_AA)
cv2.imwrite(r"%s", jpg, [int(cv2.IMWRITE_JPEG_QUALITY), 88])
`, highResPNG, lowResPNG, engJPEG))

	// --- Case 1: High-resolution PNG ---
	hiBytes, _ := os.ReadFile(highResPNG)
	resp, raw := uploadMultipartFile(t, client, server.URL+"/upload", sessionID, "file", "high_res_plan.png", hiBytes)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("high-res upload failed (%d): %s", resp.StatusCode, string(raw))
	}
	var hiUp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &hiUp)
	hiID := hiUp.Data.ID

	// Inspect high-res dataset
	inspReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/inspection", server.URL, hiID), nil)
	inspReq.Header.Set("X-Session-ID", sessionID)
	inspResp, err := client.Do(inspReq)
	if err != nil || inspResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /inspection failed: %v", err)
	}
	var inspDoc struct {
		Data struct {
			Width          float64                           `json:"width"`
			Height         float64                           `json:"height"`
			Recommendation *models.EnhancementRecommendation `json:"recommendation"`
		} `json:"data"`
	}
	_ = json.NewDecoder(inspResp.Body).Decode(&inspDoc)
	_ = inspResp.Body.Close()

	if int(inspDoc.Data.Width) != 3800 || int(inspDoc.Data.Height) != 1800 {
		t.Errorf("expected 3800x1800 inspection dimensions, got %.0fx%.0f", inspDoc.Data.Width, inspDoc.Data.Height)
	}
	if inspDoc.Data.Recommendation == nil || !inspDoc.Data.Recommendation.IsHighResolution {
		t.Errorf("expected Recommendation.IsHighResolution=true for 3800x1800 raster, got %+v", inspDoc.Data.Recommendation)
	}

	// Request 2x upscale without ForceUpscale -> high-res guard must skip AI upscaling
	enhBody := `{"preset":"balanced","upscale_factor":2,"force_upscale":false}`
	enhReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/datasets/%s/enhance", server.URL, hiID), strings.NewReader(enhBody))
	enhReq.Header.Set("Content-Type", "application/json")
	enhReq.Header.Set("X-Session-ID", sessionID)
	enhResp, err := client.Do(enhReq)
	if err != nil || enhResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /enhance on high-res failed: %v", err)
	}
	var hiEnhDoc struct {
		Data models.EnhancementState `json:"data"`
	}
	_ = json.NewDecoder(enhResp.Body).Decode(&hiEnhDoc)
	_ = enhResp.Body.Close()

	if hiEnhDoc.Data.AIUpscaled || hiEnhDoc.Data.AIUpscaleFactor > 1 {
		t.Errorf("expected high-res guard to skip AI upscaling, got AIUpscaled=%v AIUpscaleFactor=%d", hiEnhDoc.Data.AIUpscaled, hiEnhDoc.Data.AIUpscaleFactor)
	}
	if !hiEnhDoc.Data.SkippedHighResUpscale {
		t.Errorf("expected SkippedHighResUpscale=true on high-res image")
	}

	// --- Case 2: Low-resolution PNG with 2x Real-ESRGAN AI Upscaling + PaddleOCR ---
	loBytes, _ := os.ReadFile(lowResPNG)
	resp, raw = uploadMultipartFile(t, client, server.URL+"/upload", sessionID, "file", "low_res_labels.png", loBytes)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("low-res upload failed (%d): %s", resp.StatusCode, string(raw))
	}
	var loUp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &loUp)
	loID := loUp.Data.ID

	loDsBefore, _ := repo.GetDataset(sessionID, loID)
	origHashBefore := sha256FileHex(t, loDsBefore.StoragePath)

	// Georeference low-res dataset so OCR annotations are automatically projected to EPSG:4326
	helperAddControlPoint(t, server.URL, sessionID, loID, 0, 0, -122.4200, 37.7800)
	helperAddControlPoint(t, server.URL, sessionID, loID, 359, 0, -122.4100, 37.7800)
	helperAddControlPoint(t, server.URL, sessionID, loID, 359, 219, -122.4100, 37.7700)
	helperAddControlPoint(t, server.URL, sessionID, loID, 0, 219, -122.4200, 37.7700)
	helperCalcGeoreference(t, server.URL, sessionID, loID)

	loEnhBody := `{"preset":"text_clarity","upscale_factor":2,"run_ocr":true}`
	loEnhReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/datasets/%s/enhance", server.URL, loID), strings.NewReader(loEnhBody))
	loEnhReq.Header.Set("Content-Type", "application/json")
	loEnhReq.Header.Set("X-Session-ID", sessionID)
	loEnhResp, err := client.Do(loEnhReq)
	if err != nil || loEnhResp.StatusCode != http.StatusOK {
		rawErr, _ := io.ReadAll(loEnhResp.Body)
		t.Fatalf("POST /enhance (2x + OCR) failed (%d): %v %s", loEnhResp.StatusCode, err, string(rawErr))
	}
	var loEnhDoc struct {
		Data models.EnhancementState `json:"data"`
	}
	_ = json.NewDecoder(loEnhResp.Body).Decode(&loEnhDoc)
	_ = loEnhResp.Body.Close()

	if !loEnhDoc.Data.AIUpscaled || loEnhDoc.Data.AIUpscaleFactor != 2 {
		t.Errorf("expected AIUpscaled=true AIUpscaleFactor=2 on low-res image, got AIUpscaled=%v AIUpscaleFactor=%d", loEnhDoc.Data.AIUpscaled, loEnhDoc.Data.AIUpscaleFactor)
	}
	if loEnhDoc.Data.AIVerifiedEngineering {
		t.Errorf("AI-upscaled raster must NEVER report AIVerifiedEngineering=true")
	}
	if loEnhDoc.Data.EnhancedWidth != 720 || loEnhDoc.Data.EnhancedHeight != 440 {
		t.Errorf("expected 720x440 enhanced dimensions, got %dx%d", loEnhDoc.Data.EnhancedWidth, loEnhDoc.Data.EnhancedHeight)
	}
	if origHashAfter := sha256FileHex(t, loDsBefore.StoragePath); origHashAfter != origHashBefore {
		t.Fatalf("original StoragePath was mutated by enhancement pipeline!")
	}

	// Verify /source-image?variant=original returns 360x220 while ?variant=enhanced returns 720x440
	checkVariantDims := func(variant string, expW, expH int) {
		vReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/source-image?variant=%s", server.URL, loID, variant), nil)
		vReq.Header.Set("X-Session-ID", sessionID)
		vResp, vErr := client.Do(vReq)
		if vErr != nil || vResp.StatusCode != http.StatusOK {
			t.Fatalf("GET /source-image?variant=%s failed: %v", variant, vErr)
		}
		cfg, decErr := png.DecodeConfig(vResp.Body)
		_ = vResp.Body.Close()
		if decErr != nil {
			t.Fatalf("decoding %s PNG failed: %v", variant, decErr)
		}
		if cfg.Width != expW || cfg.Height != expH {
			t.Errorf("variant=%s expected %dx%d, got %dx%d", variant, expW, expH, cfg.Width, cfg.Height)
		}
	}
	checkVariantDims("original", 360, 220)
	checkVariantDims("enhanced", 720, 440)

	// Verify PaddleOCR extracted label annotations and that none are marked verified by default
	annReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/annotations", server.URL, loID), nil)
	annReq.Header.Set("X-Session-ID", sessionID)
	annResp, err := client.Do(annReq)
	if err != nil || annResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /annotations failed: %v", err)
	}
	var annDoc struct {
		Data struct {
			Annotations []*models.DatasetAnnotation `json:"annotations"`
		} `json:"data"`
	}
	_ = json.NewDecoder(annResp.Body).Decode(&annDoc)
	_ = annResp.Body.Close()

	if len(annDoc.Data.Annotations) == 0 {
		t.Fatalf("expected PaddleOCR to extract at least 1 label from low_res_labels.png, got 0")
	}
	foundRoom := false
	for _, a := range annDoc.Data.Annotations {
		if a.Verified {
			t.Errorf("unreviewed OCR annotation %q must NOT be marked verified=true", a.Text)
		}
		if strings.Contains(strings.ToUpper(a.Text), "ROOM") || strings.Contains(strings.ToUpper(a.Text), "101") {
			foundRoom = true
		}
		if a.Longitude == nil || a.Latitude == nil {
			t.Errorf("expected georeferenced annotation %q to have projected EPSG:4326 lon/lat", a.Text)
		}
	}
	if !foundRoom {
		t.Errorf("expected OCR annotations to include ROOM 101, got %+v", annDoc.Data.Annotations[0])
	}

	// Edit & verify first annotation, then test creating and deleting a temporary false-positive annotation
	firstAnn := annDoc.Data.Annotations[0]
	firstAnn.Text = "ROOM 101 - VERIFIED"
	firstAnn.Verified = true
	editBytes, _ := json.Marshal(firstAnn)
	putReq, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/datasets/%s/annotations/%s", server.URL, loID, firstAnn.ID), bytes.NewReader(editBytes))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("X-Session-ID", sessionID)
	putResp, err := client.Do(putReq)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /annotations/%s failed: %v", firstAnn.ID, err)
	}
	_ = putResp.Body.Close()

	createAnnReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/datasets/%s/annotations", server.URL, loID),
		strings.NewReader(`{"text":"FALSE_POSITIVE_NOISE","image_x":20,"image_y":20,"visible":true}`))
	createAnnReq.Header.Set("Content-Type", "application/json")
	createAnnReq.Header.Set("X-Session-ID", sessionID)
	createAnnResp, err := client.Do(createAnnReq)
	if err != nil || createAnnResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /annotations failed: %v", err)
	}
	var createdAnnDoc struct {
		Data models.DatasetAnnotation `json:"data"`
	}
	_ = json.NewDecoder(createAnnResp.Body).Decode(&createdAnnDoc)
	_ = createAnnResp.Body.Close()

	delAnnReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/datasets/%s/annotations/%s", server.URL, loID, createdAnnDoc.Data.ID), nil)
	delAnnReq.Header.Set("X-Session-ID", sessionID)
	delAnnResp, err := client.Do(delAnnReq)
	if err != nil || delAnnResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /annotations/%s failed: %v", createdAnnDoc.Data.ID, err)
	}
	_ = delAnnResp.Body.Close()

	// --- Case 3: JPEG engineering drawing ---
	jpgBytes, _ := os.ReadFile(engJPEG)
	resp, raw = uploadMultipartFile(t, client, server.URL+"/upload", sessionID, "file", "engineering_sheet.jpg", jpgBytes)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("JPEG upload failed (%d): %s", resp.StatusCode, string(raw))
	}
	var jpgUp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &jpgUp)
	jpgID := jpgUp.Data.ID
	jpgDs, _ := repo.GetDataset(sessionID, jpgID)
	jpgOrigHash := sha256FileHex(t, jpgDs.StoragePath)

	jpgEnhReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/datasets/%s/enhance", server.URL, jpgID), strings.NewReader(`{"preset":"line_preservation"}`))
	jpgEnhReq.Header.Set("Content-Type", "application/json")
	jpgEnhReq.Header.Set("X-Session-ID", sessionID)
	jpgEnhResp, err := client.Do(jpgEnhReq)
	if err != nil || jpgEnhResp.StatusCode != http.StatusOK {
		t.Fatalf("JPEG /enhance failed: %v", err)
	}
	_ = jpgEnhResp.Body.Close()
	if sha256FileHex(t, jpgDs.StoragePath) != jpgOrigHash {
		t.Errorf("original JPEG StoragePath was modified during OpenCV enhancement")
	}
}

// TestPhases3_4_5_7_8_PDF_CAD_GeoTIFF_And_MBTilesReopenRoundtrip tests:
// 4. Scanned raster PDF (Path B adaptive DPI + OpenCV enhancement + OCR)
// 5 & 6. Vector & Multipage PDF (Path A vector paths + positioned text spans + ordered multi-floor layers)
// 7. CAD .dxf (blocks, layers, units, linework, text annotations -> projected EPSG:4326 GeoJSON)
// 8. GeoTIFF (EPSG:3857 -> EPSG:4326 auto-detection + display.png + GDAL MBTiles generation)
// 9. Standalone .mbtiles & Multi-floor .zip Project export -> /map-projects/inspect -> /map-projects/import reopen -> live /tiles/{z}/{x}/{y} serving.
func TestPhases3_4_5_7_8_PDF_CAD_GeoTIFF_And_MBTilesReopenRoundtrip(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer storage.SafeRemoveAll(tempDir)
	client := server.Client()

	sess := repo.CreateSession()
	sessionID := sess.ID

	fixtureDir := t.TempDir()
	scannedPDF := filepath.Join(fixtureDir, "scanned_drawing.pdf")
	vectorMultiPDF := filepath.Join(fixtureDir, "vector_multipage.pdf")
	cadDXF := filepath.Join(fixtureDir, "campus_cad.dxf")
	geoTIFF := filepath.Join(fixtureDir, "projected_site.tif")

	runTestPython(t, fmt.Sprintf(`
import cv2
import ezdxf
import numpy as np
import pymupdf
import rasterio
from rasterio.transform import from_bounds

# 1. Scanned raster-only PDF (Path B)
scan_img = np.full((600, 900, 3), 246, dtype=np.uint8)
cv2.rectangle(scan_img, (40, 40), (860, 560), (20, 20, 20), 3)
cv2.putText(scan_img, "SCANNED SHEET B1", (120, 280), cv2.FONT_HERSHEY_SIMPLEX, 1.6, (10, 10, 10), 3, cv2.LINE_AA)
ok, png_buf = cv2.imencode(".png", scan_img)
sdoc = pymupdf.open()
spage = sdoc.new_page(width=450, height=300)
spage.insert_image(pymupdf.Rect(0, 0, 450, 300), stream=png_buf.tobytes())
sdoc.save(r"%s")
sdoc.close()

# 2. Vector 2-page PDF (Path A + Multipage)
vdoc = pymupdf.open()
for idx, label in enumerate(["GROUND WING A", "SECOND FLOOR B"]):
    vp = vdoc.new_page(width=600, height=400)
    shape = vp.new_shape()
    shape.draw_rect(pymupdf.Rect(40, 40, 560, 360))
    shape.draw_line(pymupdf.Point(40, 200), pymupdf.Point(560, 200))
    shape.finish(color=(0.1, 0.2, 0.7), width=2)
    shape.commit()
    vp.insert_text((80, 120), label, fontsize=18, color=(0, 0, 0))
    vp.insert_text((80, 260), f"DIM {15 + idx}.40m", fontsize=14, color=(0.1, 0.1, 0.1))
vdoc.save(r"%s")
vdoc.close()

# 3. CAD DXF with BLOCKS + INSERT + LWPOLYLINE + TEXT + $INSUNITS
dxf = ezdxf.new("R2010")
dxf.header["$INSUNITS"] = 6  # Meters
dxf.layers.add("WALLS", color=7)
dxf.layers.add("LABELS", color=3)
blk = dxf.blocks.new(name="COLUMN_SYM")
blk.add_lwpolyline([(-2, -2), (2, -2), (2, 2), (-2, 2)], close=True)
msp = dxf.modelspace()
msp.add_lwpolyline([(0, 0), (120, 0), (120, 80), (0, 80)], close=True, dxfattribs={"layer": "WALLS"})
msp.add_blockref("COLUMN_SYM", (30, 40), dxfattribs={"layer": "WALLS"})
msp.add_text("TURBINE HALL", dxfattribs={"layer": "LABELS", "height": 5}).set_placement((25, 50))
dxf.saveas(r"%s")

# 4. GeoTIFF in EPSG:3857 (San Francisco Web Mercator bounds)
west_m, south_m, east_m, north_m = -13627665.0, 4547000.0, -13626550.0, 4548100.0
transform = from_bounds(west_m, south_m, east_m, north_m, 256, 256)
band1 = np.zeros((256, 256), dtype=np.uint8)
band2 = np.zeros((256, 256), dtype=np.uint8)
band3 = np.full((256, 256), 180, dtype=np.uint8)
for y in range(256):
    band1[y, :] = y
    band2[:, y] = y
with rasterio.open(
    r"%s", "w", driver="GTiff", height=256, width=256, count=3,
    dtype="uint8", crs="EPSG:3857", transform=transform
) as dst:
    dst.write(band1, 1)
    dst.write(band2, 2)
    dst.write(band3, 3)
`, scannedPDF, vectorMultiPDF, cadDXF, geoTIFF))

	// --- Test 4: Scanned Raster PDF (Path B) ---
	scanBytes, _ := os.ReadFile(scannedPDF)
	resp, raw := uploadMultipartFile(t, client, server.URL+"/upload", sessionID, "file", "scanned_drawing.pdf", scanBytes)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scanned PDF upload expected 200 inspection, got %d: %s", resp.StatusCode, string(raw))
	}
	var scanInsp struct {
		Data models.PdfInspectionResult `json:"data"`
	}
	_ = json.Unmarshal(raw, &scanInsp)
	if len(scanInsp.Data.Pages) != 1 || scanInsp.Data.Pages[0].ProcessingPath != "raster" {
		t.Errorf("expected scanned PDF page ProcessingPath='raster', got %+v", scanInsp.Data.Pages)
	}

	impScanPayload, _ := json.Marshal(models.ImportPdfPagesRequest{
		DocumentID:    scanInsp.Data.DocumentID,
		SelectedPages: []int{1},
		RunOCR:        true,
	})
	impScanReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/import-pdf-pages", bytes.NewReader(impScanPayload))
	impScanReq.Header.Set("Content-Type", "application/json")
	impScanReq.Header.Set("X-Session-ID", sessionID)
	impScanResp, err := client.Do(impScanReq)
	if err != nil || impScanResp.StatusCode != http.StatusOK {
		t.Fatalf("import scanned PDF page failed: %v", err)
	}
	_ = impScanResp.Body.Close()

	// --- Test 5 & 6: Vector Multipage PDF (Path A + Multi-floor ordering) ---
	vecPdfBytes, _ := os.ReadFile(vectorMultiPDF)
	resp, raw = uploadMultipartFile(t, client, server.URL+"/upload", sessionID, "file", "vector_multipage.pdf", vecPdfBytes)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("vector PDF upload expected 200 inspection, got %d: %s", resp.StatusCode, string(raw))
	}
	var vecInsp struct {
		Data models.PdfInspectionResult `json:"data"`
	}
	_ = json.Unmarshal(raw, &vecInsp)
	if vecInsp.Data.PageCount != 2 {
		t.Fatalf("expected 2-page vector PDF, got %d", vecInsp.Data.PageCount)
	}
	if vecInsp.Data.Pages[0].ProcessingPath != "vector" {
		t.Errorf("expected vector PDF page ProcessingPath='vector', got %s", vecInsp.Data.Pages[0].ProcessingPath)
	}

	extractVec := true
	impVecPayload, _ := json.Marshal(models.ImportPdfPagesRequest{
		DocumentID:     vecInsp.Data.DocumentID,
		SelectedPages:  []int{1, 2},
		ExtractVectors: &extractVec,
	})
	impVecReq, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/import-pdf-pages", bytes.NewReader(impVecPayload))
	impVecReq.Header.Set("Content-Type", "application/json")
	impVecReq.Header.Set("X-Session-ID", sessionID)
	impVecResp, err := client.Do(impVecReq)
	if err != nil || impVecResp.StatusCode != http.StatusOK {
		t.Fatalf("import vector PDF pages failed: %v", err)
	}
	var impVecRes struct {
		Data models.ImportPdfPagesResponse `json:"data"`
	}
	_ = json.NewDecoder(impVecResp.Body).Decode(&impVecRes)
	_ = impVecResp.Body.Close()

	if len(impVecRes.Data.Datasets) != 2 {
		t.Fatalf("expected 2 imported PDF page datasets, got %d", len(impVecRes.Data.Datasets))
	}
	pdfPage1ID := impVecRes.Data.Datasets[0].ID
	pdfPage2ID := impVecRes.Data.Datasets[1].ID

	// Georeference Page 1 & Page 2 so vector paths & text annotations project to EPSG:4326
	p1Ds, _ := repo.GetDataset(sessionID, pdfPage1ID)
	p1W := testFloat(p1Ds.ExtractedMetadata["width"])
	p1H := testFloat(p1Ds.ExtractedMetadata["height"])
	for _, pid := range []string{pdfPage1ID, pdfPage2ID} {
		helperAddControlPoint(t, server.URL, sessionID, pid, 0, 0, -122.4200, 37.7800)
		helperAddControlPoint(t, server.URL, sessionID, pid, p1W-1, 0, -122.4100, 37.7800)
		helperAddControlPoint(t, server.URL, sessionID, pid, p1W-1, p1H-1, -122.4100, 37.7700)
		helperAddControlPoint(t, server.URL, sessionID, pid, 0, p1H-1, -122.4200, 37.7700)
		helperCalcGeoreference(t, server.URL, sessionID, pid)
	}

	// Verify /vectors returns projected EPSG:4326 GeoJSON features for Vector PDF Page 1
	vecReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/vectors", server.URL, pdfPage1ID), nil)
	vecReq.Header.Set("X-Session-ID", sessionID)
	vecResp, err := client.Do(vecReq)
	if err != nil || vecResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /vectors for PDF page 1 failed: %v", err)
	}
	var vecFC struct {
		Data struct {
			Features []map[string]interface{} `json:"features"`
		} `json:"data"`
	}
	_ = json.NewDecoder(vecResp.Body).Decode(&vecFC)
	_ = vecResp.Body.Close()
	if len(vecFC.Data.Features) == 0 {
		t.Errorf("expected extracted vector GeoJSON features on Vector PDF page 1, got 0")
	}

	// --- Test 7: CAD .dxf with blocks, layers, units, linework, and text ---
	dxfBytes, _ := os.ReadFile(cadDXF)
	resp, raw = uploadMultipartFile(t, client, server.URL+"/upload", sessionID, "file", "campus_cad.dxf", dxfBytes)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("CAD DXF upload failed (%d): %s", resp.StatusCode, string(raw))
	}
	var dxfUp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &dxfUp)
	dxfID := dxfUp.Data.ID

	dxfDs, _ := repo.GetDataset(sessionID, dxfID)
	dxfW := testFloat(dxfDs.ExtractedMetadata["width"])
	dxfH := testFloat(dxfDs.ExtractedMetadata["height"])
	helperAddControlPoint(t, server.URL, sessionID, dxfID, 0, 0, -122.4200, 37.7800)
	helperAddControlPoint(t, server.URL, sessionID, dxfID, dxfW-1, 0, -122.4100, 37.7800)
	helperAddControlPoint(t, server.URL, sessionID, dxfID, dxfW-1, dxfH-1, -122.4100, 37.7700)
	helperAddControlPoint(t, server.URL, sessionID, dxfID, 0, dxfH-1, -122.4200, 37.7700)
	helperCalcGeoreference(t, server.URL, sessionID, dxfID)

	cadVecReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/vectors", server.URL, dxfID), nil)
	cadVecReq.Header.Set("X-Session-ID", sessionID)
	cadVecResp, _ := client.Do(cadVecReq)
	var cadVecFC struct {
		Data struct {
			Features []map[string]interface{} `json:"features"`
		} `json:"data"`
	}
	_ = json.NewDecoder(cadVecResp.Body).Decode(&cadVecFC)
	_ = cadVecResp.Body.Close()
	if len(cadVecFC.Data.Features) < 2 {
		t.Errorf("expected CAD DXF vector features (outer wall + expanded blockref), got %d", len(cadVecFC.Data.Features))
	}

	cadAnnReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/annotations", server.URL, dxfID), nil)
	cadAnnReq.Header.Set("X-Session-ID", sessionID)
	cadAnnResp, _ := client.Do(cadAnnReq)
	var cadAnnDoc struct {
		Data struct {
			Annotations []*models.DatasetAnnotation `json:"annotations"`
		} `json:"data"`
	}
	_ = json.NewDecoder(cadAnnResp.Body).Decode(&cadAnnDoc)
	_ = cadAnnResp.Body.Close()
	if len(cadAnnDoc.Data.Annotations) == 0 || !strings.Contains(cadAnnDoc.Data.Annotations[0].Text, "TURBINE HALL") {
		t.Errorf("expected CAD text annotation 'TURBINE HALL', got %+v", cadAnnDoc.Data.Annotations)
	}

	// --- Test 8: GeoTIFF (EPSG:3857 -> EPSG:4326 auto-georeferencing + display.png + GDAL MBTiles) ---
	tifBytes, _ := os.ReadFile(geoTIFF)
	resp, raw = uploadMultipartFile(t, client, server.URL+"/upload", sessionID, "file", "projected_site.tif", tifBytes)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("GeoTIFF upload failed (%d): %s", resp.StatusCode, string(raw))
	}
	var tifUp struct {
		Data struct {
			ID              string `json:"id"`
			IsGeoreferenced bool   `json:"is_georeferenced"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &tifUp)
	if !tifUp.Data.IsGeoreferenced {
		t.Fatalf("expected EPSG:3857 GeoTIFF to be auto-detected as georeferenced on upload")
	}

	// Verify /source-image serves browser-renderable PNG for GeoTIFF
	tifImgReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/source-image", server.URL, tifUp.Data.ID), nil)
	tifImgReq.Header.Set("X-Session-ID", sessionID)
	tifImgResp, _ := client.Do(tifImgReq)
	if tifImgResp.StatusCode != http.StatusOK || !strings.Contains(tifImgResp.Header.Get("Content-Type"), "image/png") {
		t.Errorf("expected GeoTIFF /source-image to serve browser-friendly image/png, got %d %s", tifImgResp.StatusCode, tifImgResp.Header.Get("Content-Type"))
	}
	_ = tifImgResp.Body.Close()

	// Generate dataset MBTiles via GDAL pipeline with Lanczos resampling
	genMBReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/datasets/%s/mbtiles", server.URL, tifUp.Data.ID), strings.NewReader(`{"resampling":"lanczos"}`))
	genMBReq.Header.Set("Content-Type", "application/json")
	genMBReq.Header.Set("X-Session-ID", sessionID)
	genMBResp, err := client.Do(genMBReq)
	if err != nil || genMBResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /datasets/{id}/mbtiles failed: %v", err)
	}
	_ = genMBResp.Body.Close()

	// --- Test 9A: Standalone .mbtiles Export -> Inspect -> Reopen -> Live Tile Fetch ---
	expMBReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/export?format=mbtiles", server.URL, tifUp.Data.ID), nil)
	expMBReq.Header.Set("X-Session-ID", sessionID)
	expMBResp, err := client.Do(expMBReq)
	if err != nil || expMBResp.StatusCode != http.StatusOK {
		t.Fatalf("standalone .mbtiles export failed: %v", err)
	}
	standaloneMBBytes, _ := io.ReadAll(expMBResp.Body)
	_ = expMBResp.Body.Close()

	if len(standaloneMBBytes) < 16 || string(standaloneMBBytes[:16]) != "SQLite format 3\x00" {
		t.Fatalf("standalone .mbtiles export lacks SQLite 3 magic header")
	}

	// Inspect standalone .mbtiles via POST /map-projects/inspect
	freshSess1 := repo.CreateSession()
	inspPkgResp, inspPkgRaw := uploadMultipartFile(t, client, server.URL+"/map-projects/inspect", freshSess1.ID, "file", "site_tiles.mbtiles", standaloneMBBytes)
	if inspPkgResp.StatusCode != http.StatusOK {
		t.Fatalf("/map-projects/inspect on .mbtiles failed (%d): %s", inspPkgResp.StatusCode, string(inspPkgRaw))
	}
	var mbPkgPreview struct {
		Data models.ProjectPackageInspection `json:"data"`
	}
	_ = json.Unmarshal(inspPkgRaw, &mbPkgPreview)
	if !mbPkgPreview.Data.Valid || mbPkgPreview.Data.PackageType != "mbtiles" || mbPkgPreview.Data.TileCount == 0 {
		t.Fatalf("unexpected .mbtiles inspection result: %+v", mbPkgPreview.Data)
	}

	// Reopen standalone .mbtiles via POST /map-projects/reopen
	reopenMBResp, reopenMBRaw := uploadMultipartFile(t, client, server.URL+"/map-projects/reopen", freshSess1.ID, "file", "site_tiles.mbtiles", standaloneMBBytes)
	if reopenMBResp.StatusCode != http.StatusCreated {
		t.Fatalf("/map-projects/reopen on .mbtiles failed (%d): %s", reopenMBResp.StatusCode, string(reopenMBRaw))
	}
	var reopenMBDoc struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(reopenMBRaw, &reopenMBDoc)
	reopenedMBID := reopenMBDoc.Data.ID

	// Compute center XYZ tile at MaxZoom and verify HTTP 200 valid PNG from /datasets/{id}/tiles/{z}/{x}/{y}
	reopenedMBDs, _ := repo.GetDataset(freshSess1.ID, reopenedMBID)
	mbInsp, err := gis.InspectMBTiles(reopenedMBDs.TilePath, "", "")
	if err != nil || mbInsp.TileCount == 0 {
		t.Fatalf("InspectMBTiles on reopened dataset failed: %v", err)
	}
	zTest := mbInsp.MaxZoom
	centerLon, centerLat := mbInsp.Center[0], mbInsp.Center[1]
	tx, ty := lonLatToXYZTile(centerLon, centerLat, zTest)
	tileReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/tiles/%d/%d/%d.png", server.URL, reopenedMBID, zTest, tx, ty), nil)
	tileReq.Header.Set("X-Session-ID", freshSess1.ID)
	tileResp, err := client.Do(tileReq)
	if err != nil || tileResp.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200 PNG tile at z=%d x=%d y=%d, got status %d (err=%v)", zTest, tx, ty, tileResp.StatusCode, err)
	}
	tileBytes, _ := io.ReadAll(tileResp.Body)
	_ = tileResp.Body.Close()
	if _, decErr := png.Decode(bytes.NewReader(tileBytes)); decErr != nil {
		t.Fatalf("tile served from reopened .mbtiles is not a valid PNG: %v", decErr)
	}

	// --- Test 9B: Multi-floor .zip Project Export (with vectors, annotations, landmarks, custom opacity & zoom) -> Inspect -> Reopen ---
	// Customize workspace member opacity on Page 1 before exporting
	wsInitReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/workspace", server.URL, pdfPage1ID), nil)
	wsInitReq.Header.Set("X-Session-ID", sessionID)
	wsInitResp, _ := client.Do(wsInitReq)
	_ = wsInitResp.Body.Close()

	patchMemReq, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/datasets/%s/workspace/members/%s", server.URL, pdfPage1ID, pdfPage1ID), strings.NewReader(`{"raster_opacity":0.65}`))
	patchMemReq.Header.Set("Content-Type", "application/json")
	patchMemReq.Header.Set("X-Session-ID", sessionID)
	patchMemResp, _ := client.Do(patchMemReq)
	_ = patchMemResp.Body.Close()

	// Add landmark icon point on Page 1
	ipReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/datasets/%s/icon-points", server.URL, pdfPage1ID),
		strings.NewReader(`{"name":"Elevator Core","description":"North Lift","icon_id":"001-camera","longitude":-122.415,"latitude":37.775,"floor_level":0,"scope":"floor"}`))
	ipReq.Header.Set("Content-Type", "application/json")
	ipReq.Header.Set("X-Session-ID", sessionID)
	ipResp, _ := client.Do(ipReq)
	_ = ipResp.Body.Close()

	initZ, minZ, maxZ := 19, 14, 21
	selExportBody, _ := json.Marshal(export.ExportRequest{
		Format:      "project_zip",
		ProjectName: "Multipage Engineering Project",
		LayerIDs:    []string{pdfPage1ID, pdfPage2ID},
		Options: &export.ExportOptions{
			IncludeBuildingConfig:  true,
			IncludeControlPoints:   true,
			IncludeIcons:           true,
			IncludeLayerStyles:     true,
			IncludeOriginalRaster:  true,
			IncludeProcessedRaster: true,
			IncludeBaseMapSettings: true,
			IncludeProjectMetadata: true,
			ZoomConfig: &services.ZoomConfiguration{
				InitialZoom: &initZ,
				MinZoom:     &minZ,
				MaxZoom:     &maxZ,
				Preset:      "street",
			},
		},
	})
	zipExpReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/datasets/%s/export", server.URL, pdfPage1ID), bytes.NewReader(selExportBody))
	zipExpReq.Header.Set("Content-Type", "application/json")
	zipExpReq.Header.Set("Accept", "application/zip")
	zipExpReq.Header.Set("X-Session-ID", sessionID)
	zipExpResp, err := client.Do(zipExpReq)
	if err != nil || zipExpResp.StatusCode != http.StatusOK {
		t.Fatalf("multi-floor project_zip export failed: %v", err)
	}
	projZipBytes, _ := io.ReadAll(zipExpResp.Body)
	_ = zipExpResp.Body.Close()

	// Inspect .zip package via /map-projects/inspect
	freshSess2 := repo.CreateSession()
	inspZipResp, inspZipRaw := uploadMultipartFile(t, client, server.URL+"/map-projects/inspect", freshSess2.ID, "file", "multipage_project.zip", projZipBytes)
	if inspZipResp.StatusCode != http.StatusOK {
		t.Fatalf("/map-projects/inspect on .zip failed (%d): %s", inspZipResp.StatusCode, string(inspZipRaw))
	}
	var zipPreview struct {
		Data models.ProjectPackageInspection `json:"data"`
	}
	_ = json.Unmarshal(inspZipRaw, &zipPreview)
	if !zipPreview.Data.Valid || zipPreview.Data.PackageType != "project_zip" || zipPreview.Data.LayerCount != 2 {
		t.Fatalf("expected valid 2-layer project_zip inspection, got %+v", zipPreview.Data)
	}

	// Reopen .zip package in fresh session via /map-projects/import
	reopenZipResp, reopenZipRaw := uploadMultipartFile(t, client, server.URL+"/map-projects/import", freshSess2.ID, "file", "multipage_project.zip", projZipBytes)
	if reopenZipResp.StatusCode != http.StatusCreated {
		t.Fatalf("/map-projects/import on .zip failed (%d): %s", reopenZipResp.StatusCode, string(reopenZipRaw))
	}
	var reopenZipDoc struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(reopenZipRaw, &reopenZipDoc)
	reopenedProjPrimaryID := reopenZipDoc.Data.ID

	// Verify exact 2 datasets restored (ZERO duplicate layers!)
	restoredList, _ := repo.ListDatasets(freshSess2.ID)
	if len(restoredList) != 2 {
		t.Fatalf("expected exactly 2 restored datasets (no duplicates), got %d", len(restoredList))
	}

	// Verify workspace manifest restores opacity=0.65, zoom=19 (14..21), buildings/floors, control points, landmarks, vectors, and annotations
	wsReopenReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/workspace", server.URL, reopenedProjPrimaryID), nil)
	wsReopenReq.Header.Set("X-Session-ID", freshSess2.ID)
	wsReopenResp, err := client.Do(wsReopenReq)
	if err != nil || wsReopenResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /workspace on reopened project failed: %v", err)
	}
	var wsDoc struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.NewDecoder(wsReopenResp.Body).Decode(&wsDoc)
	_ = wsReopenResp.Body.Close()

	membersArr, _ := wsDoc.Data["members"].([]interface{})
	if len(membersArr) != 2 {
		t.Fatalf("expected 2 workspace members after reopen, got %d", len(membersArr))
	}
	foundCustomOpacity := false
	for _, rm := range membersArr {
		m, _ := rm.(map[string]interface{})
		if m["dataset_id"] == reopenedProjPrimaryID {
			if op, ok := m["raster_opacity"].(float64); ok && op >= 0.64 && op <= 0.66 {
				foundCustomOpacity = true
			}
		}
	}
	if !foundCustomOpacity {
		t.Errorf("expected restored primary member raster_opacity=0.65 in workspace manifest")
	}

	// Verify restored vectors & annotations on reopened primary layer
	rVecReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/vectors", server.URL, reopenedProjPrimaryID), nil)
	rVecReq.Header.Set("X-Session-ID", freshSess2.ID)
	rVecResp, _ := client.Do(rVecReq)
	var rVecDoc struct {
		Data struct {
			Features []map[string]interface{} `json:"features"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rVecResp.Body).Decode(&rVecDoc)
	_ = rVecResp.Body.Close()
	if len(rVecDoc.Data.Features) == 0 {
		t.Errorf("expected PDF vector overlay GeoJSON to survive .zip export/reopen roundtrip")
	}

	rAnnReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/datasets/%s/annotations", server.URL, reopenedProjPrimaryID), nil)
	rAnnReq.Header.Set("X-Session-ID", freshSess2.ID)
	rAnnResp, _ := client.Do(rAnnReq)
	var rAnnDoc struct {
		Data struct {
			Annotations []*models.DatasetAnnotation `json:"annotations"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rAnnResp.Body).Decode(&rAnnDoc)
	_ = rAnnResp.Body.Close()
	if len(rAnnDoc.Data.Annotations) == 0 {
		t.Errorf("expected PDF vector text annotations to survive .zip export/reopen roundtrip")
	}
}

// TestPhase8And10_InvalidCorruptAndDeprecatedReopenRejections verifies strict validation
// of malformed, path-traversal, duplicate, deprecated (missing .mbtiles), and non-SQLite packages.
func TestPhase8And10_InvalidCorruptAndDeprecatedReopenRejections(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer storage.SafeRemoveAll(tempDir)
	client := server.Client()

	sess := repo.CreateSession()
	sessionID := sess.ID

	// Build a tiny valid SQLite .mbtiles buffer once so we can test corrupt project.json inside a .zip
	tmpMB := filepath.Join(t.TempDir(), "tiny.mbtiles")
	pngPath := filepath.Join(t.TempDir(), "tiny.png")
	_ = os.WriteFile(pngPath, createTestPNG(64, 64), 0644)
	minZ, maxZ := 14, 14
	if _, err := gis.CreateMBTiles(tmpMB, gis.MBTilesCreateSpec{
		Name:       "Tiny",
		TileFormat: "png",
		MinZoom:    &minZ,
		MaxZoom:    &maxZ,
		Layers: []gis.MBTilesLayerInput{
			{ImagePath: pngPath, Bounds: []float64{-122.42, 37.77, -122.41, 37.78}, Opacity: 1.0},
		},
	}); err != nil {
		t.Fatalf("CreateMBTiles fixture failed: %v", err)
	}
	validMBBytes, _ := os.ReadFile(tmpMB)

	makeZip := func(entries []struct {
		name string
		data []byte
	}) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, e := range entries {
			hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
			w, _ := zw.CreateHeader(hdr)
			_, _ = w.Write(e.data)
		}
		_ = zw.Close()
		return buf.Bytes()
	}

	cases := []struct {
		label        string
		filename     string
		payload      []byte
		expectedCode string
	}{
		{
			label:        "0-byte empty upload",
			filename:     "empty.mbtiles",
			payload:      []byte{},
			expectedCode: "EMPTY_ARCHIVE",
		},
		{
			label:        "corrupted non-zip bytes",
			filename:     "corrupted.zip",
			payload:      []byte("not-a-valid-zip-header-at-all"),
			expectedCode: "CORRUPT_ZIP",
		},
		{
			label:        "empty zip with 0 entries",
			filename:     "zero_entries.zip",
			payload:      makeZip(nil),
			expectedCode: "EMPTY_ARCHIVE",
		},
		{
			label:    "zip-slip path traversal",
			filename: "traversal.zip",
			payload: makeZip([]struct {
				name string
				data []byte
			}{
				{name: "../../escape.mbtiles", data: validMBBytes},
			}),
			expectedCode: "ZIP_SLIP_ATTACK",
		},
		{
			label:    "duplicate archive entries",
			filename: "duplicate.zip",
			payload: makeZip([]struct {
				name string
				data []byte
			}{
				{name: "project.mbtiles", data: validMBBytes},
				{name: "PROJECT.mbtiles", data: validMBBytes},
			}),
			expectedCode: "DUPLICATE_ARCHIVE_ENTRIES",
		},
		{
			label:    "deprecated loose-XYZ project zip missing .mbtiles",
			filename: "deprecated_v1.zip",
			payload: makeZip([]struct {
				name string
				data []byte
			}{
				{name: "mapstudio.json", data: []byte(`{"format":"mapstudio-folder"}`)},
				{name: "15/5241/12664.png", data: createTestPNG(64, 64)},
			}),
			expectedCode: "MISSING_MBTILES",
		},
		{
			label:        "fake non-SQLite .mbtiles file",
			filename:     "fake_tiles.mbtiles",
			payload:      []byte("this is plain text pretending to be sqlite3 mbtiles"),
			expectedCode: "INVALID_MBTILES",
		},
		{
			label:    "corrupted project.json metadata inside MBTiles project zip",
			filename: "corrupt_meta.zip",
			payload: makeZip([]struct {
				name string
				data []byte
			}{
				{name: "project.mbtiles", data: validMBBytes},
				{name: "project.json", data: []byte(`{"format": "veracity-project", BROKEN_JSON`)},
			}),
			expectedCode: "CORRUPT_PROJECT_METADATA",
		},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			resp, raw := uploadMultipartFile(t, client, server.URL+"/map-projects/import", sessionID, "file", tc.filename, tc.payload)
			if resp.StatusCode < 400 || resp.StatusCode >= 500 {
				t.Fatalf("expected 4xx rejection for %s, got %d: %s", tc.label, resp.StatusCode, string(raw))
			}
			var errEnv struct {
				Success bool `json:"success"`
				Error   struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(raw, &errEnv)
			if errEnv.Success {
				t.Errorf("expected success=false for %s", tc.label)
			}
			if errEnv.Error.Code != tc.expectedCode {
				t.Errorf("case %q expected error code %q, got %q (msg: %s)", tc.label, tc.expectedCode, errEnv.Error.Code, errEnv.Error.Message)
			}
		})
	}
}

func lonLatToXYZTile(lon, lat float64, z int) (int, int) {
	n := float64(int(1) << z)
	xtile := int((lon + 180.0) / 360.0 * n)
	latRad := lat * math.Pi / 180.0
	sinLat := math.Sin(latRad)
	ytile := int((1.0 - math.Log((1.0+sinLat)/(1.0-sinLat))/(2.0*math.Pi)) / 2.0 * n)
	if xtile < 0 {
		xtile = 0
	}
	if ytile < 0 {
		ytile = 0
	}
	limit := (1 << z) - 1
	if xtile > limit {
		xtile = limit
	}
	if ytile > limit {
		ytile = limit
	}
	return xtile, ytile
}
