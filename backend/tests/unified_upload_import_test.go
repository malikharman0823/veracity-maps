package tests

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func makeValidPNGBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 62, G: 207, B: 142, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func makeValidJPGBytes(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 120, B: 220, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

func generateTestFixturesViaPython(t *testing.T, dir string) (gpkgPath, pdfMultiPath, pdfEncPath, dxfValidPath, dxfEmptyPath string) {
	t.Helper()
	gpkgPath = filepath.Join(dir, "multi_layer.gpkg")
	pdfMultiPath = filepath.Join(dir, "site_plans.pdf")
	pdfEncPath = filepath.Join(dir, "encrypted.pdf")
	dxfValidPath = filepath.Join(dir, "floorplan.dxf")
	dxfEmptyPath = filepath.Join(dir, "empty.dxf")

	script := `
import sys, os
import geopandas as gpd
from shapely.geometry import Point, Polygon
import pymupdf
import ezdxf

gpkg_path, pdf_multi, pdf_enc, dxf_valid, dxf_empty = sys.argv[1:6]

# 1. Multi-layer GeoPackage (2 valid WGS84 feature layers)
gdf_pts = gpd.GeoDataFrame(
    {"name": ["Station A", "Station B"]},
    geometry=[Point(77.20, 28.61), Point(77.22, 28.63)],
    crs="EPSG:4326",
)
gdf_poly = gpd.GeoDataFrame(
    {"zone": ["Central"]},
    geometry=[Polygon([(77.19, 28.60), (77.23, 28.60), (77.23, 28.64), (77.19, 28.64), (77.19, 28.60)])],
    crs="EPSG:4326",
)
gdf_pts.to_file(gpkg_path, layer="stations", driver="GPKG")
gdf_poly.to_file(gpkg_path, layer="zones", driver="GPKG")

# 2. 2-page PDF
doc = pymupdf.open()
for i in range(2):
    p = doc.new_page(width=400, height=300)
    p.insert_text((40, 80), f"Veracity Site Plan Page {i+1}")
doc.save(pdf_multi)
doc.close()

# 3. Encrypted PDF
edoc = pymupdf.open()
ep = edoc.new_page(width=300, height=200)
ep.insert_text((30, 50), "Confidential")
edoc.save(pdf_enc, encryption=pymupdf.PDF_ENCRYPT_AES_256, user_pw="secret123", owner_pw="owner123")
edoc.close()

# 4. Valid DXF
d = ezdxf.new("R2010")
msp = d.modelspace()
msp.add_line((0, 0), (100, 0))
msp.add_line((100, 0), (100, 80))
msp.add_circle((50, 40), radius=15)
d.saveas(dxf_valid)

# 5. Empty DXF (0 entities)
de = ezdxf.new("R2010")
de.saveas(dxf_empty)
`
	pyExe := gis.ResolvePythonInterpreter()
	cmd := exec.Command(pyExe, "-c", script, gpkgPath, pdfMultiPath, pdfEncPath, dxfValidPath, dxfEmptyPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed generating Python GIS test fixtures: %v\nOutput: %s", err, string(out))
	}
	return
}

func TestUnifiedUploadAndImport_AllFormatsValidationAndAtomicity(t *testing.T) {
	tempDir := t.TempDir()
	ps, err := storage.NewPrivateStorage(filepath.Join(tempDir, "private"), 0)
	if err != nil {
		t.Fatalf("NewPrivateStorage: %v", err)
	}
	limits := gis.DefaultUploadLimits()
	limits.MaxImageDimension = 512 // Small limit to verify oversized image rejection quickly
	svc := services.NewUnifiedImportService(ps, nil, limits)

	fixturesDir := filepath.Join(tempDir, "fixtures")
	_ = os.MkdirAll(fixturesDir, 0755)
	gpkgPath, pdfMultiPath, pdfEncPath, dxfValidPath, dxfEmptyPath := generateTestFixturesViaPython(t, fixturesDir)

	// --- 1. RASTER: Valid PNG & Valid JPG ---
	pngBytes := makeValidPNGBytes(64, 48)
	uplPNG, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "plan.png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("valid PNG upload failed: %v", err)
	}
	if uplPNG.DetectedFormat != gis.FormatPNG || !uplPNG.Inspection.RequiresGeoreference || uplPNG.Inspection.ResultLayerType != gis.LayerTypeRaster {
		t.Fatalf("unexpected PNG inspection: %+v", uplPNG.Inspection)
	}
	jobPNG, err := svc.StartImport("user-1", uplPNG.ID, services.StartImportRequest{ProjectID: "proj-1", Sync: true, IdempotencyKey: "png-1"})
	if err != nil || jobPNG.Status != "ready" || len(jobPNG.Layers) != 1 || jobPNG.Layers[0].LayerType != gis.LayerTypeRaster {
		t.Fatalf("PNG import failed: err=%v job=%+v", err, jobPNG)
	}

	// Idempotency check: repeated import with same key must NOT duplicate dataset/layers.
	jobPNGDup, err := svc.StartImport("user-1", uplPNG.ID, services.StartImportRequest{ProjectID: "proj-1", Sync: true, IdempotencyKey: "png-1"})
	if err != nil || jobPNGDup.ID != jobPNG.ID || jobPNGDup.DatasetID != jobPNG.DatasetID {
		t.Fatalf("idempotency failed: first=%s second=%s", jobPNG.ID, jobPNGDup.ID)
	}

	jpgBytes := makeValidJPGBytes(40, 30)
	uplJPG, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "aerial.jpg", bytes.NewReader(jpgBytes))
	if err != nil || uplJPG.DetectedFormat != gis.FormatJPG {
		t.Fatalf("valid JPG upload failed: err=%v", err)
	}

	// --- 2. RASTER: Corrupted PNG, Truncated JPG, Oversized Image, Unsupported & Spoofed ---
	corruptPNG := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("not-valid-png-chunks")...)
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "bad.png", bytes.NewReader(corruptPNG)); err == nil {
		t.Fatal("expected corrupted PNG to fail validation")
	}

	truncatedJPG := jpgBytes[:len(jpgBytes)/2]
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "trunc.jpg", bytes.NewReader(truncatedJPG)); err == nil {
		t.Fatal("expected truncated JPG to fail validation")
	}

	oversizedPNG := makeValidPNGBytes(600, 100) // exceeds test limit MaxImageDimension=512
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "huge.png", bytes.NewReader(oversizedPNG)); err == nil {
		t.Fatal("expected oversized PNG to fail dimension limit check")
	}

	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "legacy.shp", bytes.NewReader([]byte("shapefile"))); err == nil {
		t.Fatal("expected unsupported .shp format to be rejected")
	}

	// Spoofed extension: PNG magic bytes named .geojson
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryVector, "spoof.geojson", bytes.NewReader(pngBytes)); err == nil {
		t.Fatal("expected extension/signature mismatch to be rejected")
	}

	// --- 3. VECTOR: PNG/JPG Image Overlay (distinct from native vector geometry) ---
	uplOverlay, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryVector, "overlay.png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("vector PNG image_overlay upload failed: %v", err)
	}
	if uplOverlay.Inspection.ResultLayerType != gis.LayerTypeImageOverlay || !uplOverlay.Inspection.RequiresGeoreference {
		t.Fatalf("expected image_overlay with requires_georeferencing=true, got: %+v", uplOverlay.Inspection)
	}
	jobOverlay, err := svc.StartImport("user-1", uplOverlay.ID, services.StartImportRequest{ProjectID: "proj-1", ImportCategory: gis.ImportCategoryVector, Sync: true})
	if err != nil || jobOverlay.Status != "ready" || len(jobOverlay.Layers) != 1 || jobOverlay.Layers[0].LayerType != gis.LayerTypeImageOverlay {
		t.Fatalf("expected image_overlay layer registration, got err=%v job=%+v", err, jobOverlay)
	}

	// --- 4. VECTOR: GeoJSON (Valid, Invalid JSON, Empty FC, Unclosed Polygon, Projected Coords) ---
	validGeoJSON := `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"name":"HQ"},"geometry":{"type":"Point","coordinates":[77.21,28.61]}}]}`
	uplGJ, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryVector, "sites.geojson", strings.NewReader(validGeoJSON))
	if err != nil || uplGJ.Inspection.CRS != "EPSG:4326" || uplGJ.Inspection.RequiresGeoreference {
		t.Fatalf("valid GeoJSON failed: err=%v insp=%+v", err, uplGJ)
	}
	jobGJ, err := svc.StartImport("user-1", uplGJ.ID, services.StartImportRequest{ProjectID: "proj-1", Sync: true})
	if err != nil || jobGJ.Status != "ready" || len(jobGJ.Layers) != 1 || jobGJ.Layers[0].LayerType != gis.LayerTypeVector {
		t.Fatalf("GeoJSON import failed: err=%v job=%+v", err, jobGJ)
	}

	for name, bad := range map[string]string{
		"invalid_json":     `{"type":"FeatureCollection", broken`,
		"empty_fc":         `{"type":"FeatureCollection","features":[]}`,
		"unclosed_polygon": `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1]]]}}]}`,
		"projected_coords": `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{},"geometry":{"type":"Point","coordinates":[500000,3165000]}}]}`,
	} {
		if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryVector, name+".geojson", strings.NewReader(bad)); err == nil {
			t.Fatalf("expected GeoJSON case %q to fail validation", name)
		}
	}

	// --- 5. VECTOR: Multi-Layer GeoPackage & Corrupted GeoPackage ---
	gpkgBytes, _ := os.ReadFile(gpkgPath)
	uplGPKG, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryVector, "multi_layer.gpkg", bytes.NewReader(gpkgBytes))
	if err != nil {
		t.Fatalf("valid GPKG upload failed: %v", err)
	}
	if len(uplGPKG.Inspection.SubLayers) != 2 {
		t.Fatalf("expected 2 discovered GPKG layers, got %d", len(uplGPKG.Inspection.SubLayers))
	}
	jobGPKG, err := svc.StartImport("user-1", uplGPKG.ID, services.StartImportRequest{
		ProjectID:      "proj-1",
		SelectedLayers: []string{"stations", "zones"},
		Sync:           true,
	})
	if err != nil || jobGPKG.Status != "ready" || len(jobGPKG.Layers) != 2 {
		t.Fatalf("GPKG multi-layer import failed: err=%v job=%+v", err, jobGPKG)
	}

	corruptGPKG := append([]byte("SQLite format 3\x00"), bytes.Repeat([]byte{0xFF}, 256)...)
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryVector, "corrupt.gpkg", bytes.NewReader(corruptGPKG)); err == nil {
		t.Fatal("expected corrupted GPKG to fail validation")
	}

	// --- 6. PDF: Multipage Valid, Invalid Page Selection (Atomic), Encrypted, Corrupted ---
	pdfBytes, _ := os.ReadFile(pdfMultiPath)
	uplPDF, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "site_plans.pdf", bytes.NewReader(pdfBytes))
	if err != nil || uplPDF.Inspection.PageCount != 2 {
		t.Fatalf("valid 2-page PDF upload failed: err=%v insp=%+v", err, uplPDF)
	}
	jobPDF, err := svc.StartImport("user-1", uplPDF.ID, services.StartImportRequest{
		ProjectID:     "proj-1",
		SelectedPages: []int{1, 2},
		Sync:          true,
	})
	if err != nil || jobPDF.Status != "ready" || len(jobPDF.Layers) != 2 {
		t.Fatalf("2-page PDF import failed: err=%v job=%+v", err, jobPDF)
	}

	// Invalid page selection must fail atomically and create 0 datasets/layers.
	beforeCount := len(svc.ListDatasets("user-1", "proj-1"))
	jobPDFBad, err := svc.StartImport("user-1", uplPDF.ID, services.StartImportRequest{
		ProjectID:     "proj-1",
		SelectedPages: []int{1, 99},
		Sync:          true,
	})
	if err != nil || jobPDFBad.Status != "failed" || jobPDFBad.DatasetID != "" || len(jobPDFBad.Layers) != 0 {
		t.Fatalf("expected atomic failure for invalid PDF page selection, got err=%v job=%+v", err, jobPDFBad)
	}
	if afterCount := len(svc.ListDatasets("user-1", "proj-1")); afterCount != beforeCount {
		t.Fatalf("failed import must not create datasets: before=%d after=%d", beforeCount, afterCount)
	}

	encBytes, _ := os.ReadFile(pdfEncPath)
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "encrypted.pdf", bytes.NewReader(encBytes)); err == nil {
		t.Fatal("expected encrypted PDF to be rejected")
	}

	corruptPDF := []byte("%PDF-1.7\n%corrupted-stream-bytes")
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "broken.pdf", bytes.NewReader(corruptPDF)); err == nil {
		t.Fatal("expected corrupted PDF to be rejected")
	}

	// --- 7. CAD (DXF): Valid, Empty, Corrupted ---
	dxfBytes, _ := os.ReadFile(dxfValidPath)
	uplDXF, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "floorplan.dxf", bytes.NewReader(dxfBytes))
	if err != nil || uplDXF.Inspection.EntityCount < 3 {
		t.Fatalf("valid DXF upload failed: err=%v insp=%+v", err, uplDXF)
	}
	jobDXF, err := svc.StartImport("user-1", uplDXF.ID, services.StartImportRequest{ProjectID: "proj-1", Sync: true})
	if err != nil || jobDXF.Status != "ready" || len(jobDXF.Layers) != 1 || jobDXF.Layers[0].LayerType != gis.LayerTypeRaster {
		t.Fatalf("DXF rasterization import failed: err=%v job=%+v", err, jobDXF)
	}

	dxfEmptyBytes, _ := os.ReadFile(dxfEmptyPath)
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "empty.dxf", bytes.NewReader(dxfEmptyBytes)); err == nil {
		t.Fatal("expected empty DXF (0 entities) to be rejected")
	}

	corruptDXF := []byte("0\nSECTION\n2\nHEADER\nCORRUPTED_GARBAGE_NO_EOF")
	if _, err := svc.UploadAndInspect(t.Context(), "user-1", "proj-1", gis.ImportCategoryRaster, "bad.dxf", bytes.NewReader(corruptDXF)); err == nil {
		t.Fatal("expected corrupted DXF to be rejected")
	}

	// --- 8. Temp Staging Cleanup Verification ---
	userTempDir, _ := ps.UserTempDir("user-1")
	entries, _ := os.ReadDir(userTempDir)
	if len(entries) != 0 {
		t.Fatalf("expected 0 leftover temp staging files/dirs, found %d", len(entries))
	}

	// --- 9. Verify Migration 020 RLS Definitions ---
	mig20, err := os.ReadFile(filepath.Join("..", "migrations", "020_unified_uploads_imports.sql"))
	if err != nil {
		t.Fatalf("read migration 020: %v", err)
	}
	for _, want := range []string{"dbo.Uploads", "dbo.Unified_Layers", "security.UserIsolationPolicy", "FILTER PREDICATE", "BLOCK PREDICATE"} {
		if !strings.Contains(string(mig20), want) {
			t.Errorf("migration 020 missing %q", want)
		}
	}
}

func TestUnifiedUploadAPI_HTTPAndCrossUserIsolation(t *testing.T) {
	server, _, _ := setupMultiUserTestServer(t, t.TempDir())
	defer server.Close()

	register := func(name, email string) models.AuthResponse {
		body, _ := json.Marshal(map[string]string{"name": name, "email": email, "password": "Password123!"})
		res, err := http.Post(server.URL+"/api/auth/register", "application/json", bytes.NewReader(body))
		if err != nil || res.StatusCode != http.StatusCreated {
			t.Fatalf("register %s failed: err=%v status=%v", email, err, res)
		}
		defer res.Body.Close()
		var wrap struct {
			Data models.AuthResponse `json:"data"`
		}
		_ = json.NewDecoder(res.Body).Decode(&wrap)
		return wrap.Data
	}

	userA := register("Alice Upload", "alice-upl@veracity.maps")
	userB := register("Bob Upload", "bob-upl@veracity.maps")

	// 1. Unauthenticated POST /api/uploads must return 401.
	resUnauth, _ := http.Post(server.URL+"/api/uploads", "application/json", nil)
	if resUnauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthenticated, got %d", resUnauth.StatusCode)
	}
	resUnauth.Body.Close()

	// 2. User A uploads a valid PNG via multipart POST /api/uploads.
	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	_ = mw.WriteField("project_id", "proj-alpha")
	_ = mw.WriteField("import_category", "raster")
	fw, _ := mw.CreateFormFile("file", "campus.png")
	_, _ = io.Copy(fw, bytes.NewReader(makeValidPNGBytes(32, 32)))
	_ = mw.Close()

	reqUpl, _ := http.NewRequest(http.MethodPost, server.URL+"/api/uploads", &bodyBuf)
	reqUpl.Header.Set("Authorization", "Bearer "+userA.Token)
	reqUpl.Header.Set("Content-Type", mw.FormDataContentType())
	resUpl, err := http.DefaultClient.Do(reqUpl)
	if err != nil || resUpl.StatusCode != http.StatusCreated {
		t.Fatalf("User A POST /api/uploads failed: err=%v status=%d", err, resUpl.StatusCode)
	}
	var uplWrap struct {
		Data services.UnifiedUpload `json:"data"`
	}
	_ = json.NewDecoder(resUpl.Body).Decode(&uplWrap)
	resUpl.Body.Close()
	uploadID := uplWrap.Data.ID

	// 3. User A starts atomic import via POST /api/imports/{uploadId}.
	impPayload := `{"project_id":"proj-alpha","sync":true,"idempotency_key":"http-1"}`
	reqImp, _ := http.NewRequest(http.MethodPost, server.URL+"/api/imports/"+uploadID, strings.NewReader(impPayload))
	reqImp.Header.Set("Authorization", "Bearer "+userA.Token)
	reqImp.Header.Set("Content-Type", "application/json")
	resImp, err := http.DefaultClient.Do(reqImp)
	if err != nil || resImp.StatusCode != http.StatusOK {
		t.Fatalf("User A POST /api/imports failed: err=%v status=%d", err, resImp.StatusCode)
	}
	var jobWrap struct {
		Data services.UnifiedImportJob `json:"data"`
	}
	_ = json.NewDecoder(resImp.Body).Decode(&jobWrap)
	resImp.Body.Close()
	jobID := jobWrap.Data.ID
	datasetID := jobWrap.Data.DatasetID
	if jobWrap.Data.Status != "ready" || datasetID == "" {
		t.Fatalf("expected ready import job, got %+v", jobWrap.Data)
	}

	// 4. Cross-User Isolation: User B MUST receive 403 Forbidden for User A's upload, import, job, and dataset.
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/uploads/" + uploadID},
		{http.MethodPost, "/api/imports/" + uploadID},
		{http.MethodGet, "/api/jobs/" + jobID},
		{http.MethodPost, "/api/jobs/" + jobID + "/cancel"},
		{http.MethodGet, "/api/unified-datasets/" + datasetID},
		{http.MethodDelete, "/api/unified-datasets/" + datasetID},
	} {
		req, _ := http.NewRequest(tc.method, server.URL+tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+userB.Token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("User B %s %s failed: %v", tc.method, tc.path, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for User B %s %s, got %d", tc.method, tc.path, res.StatusCode)
		}
	}

	// 5. User A can delete their own dataset cleanly.
	reqDelA, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/unified-datasets/"+datasetID, nil)
	reqDelA.Header.Set("Authorization", "Bearer "+userA.Token)
	resDelA, _ := http.DefaultClient.Do(reqDelA)
	resDelA.Body.Close()
	if resDelA.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when User A deletes own dataset, got %d", resDelA.StatusCode)
	}
}
