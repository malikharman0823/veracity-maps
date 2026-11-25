package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/baselayers"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	gisFn "github.com/malikharman0823/veracity-maps/functions/gis"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/publish"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/functions/workspaceregistry"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func TestCombinedRasterAndVectorWebTilePublishing(t *testing.T) {
	tmpRoot := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir:  filepath.Join(tmpRoot, "sessions"),
		UploadDir:          filepath.Join(tmpRoot, "uploads"),
		ProcessedDir:       filepath.Join(tmpRoot, "processed"),
		MaxUploadSizeBytes: 256 << 20,
	}
	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("storage manager: %v", err)
	}
	repo := repository.NewSessionRepository()
	repo.SetStorageDir(cfg.SessionStorageDir)
	authRepo, _ := repository.NewAuthRepository(filepath.Join(tmpRoot, "auth"))
	projRepo, _ := repository.NewProjectRepository(filepath.Join(tmpRoot, "proj"))
	gisRepo := repository.NewMemoryGISRepository()
	baseLayerRepo := repository.NewMemoryBaseLayerRepository()
	wsRegRepo := repository.NewMemoryWorkspaceRegistryRepository()

	gisSvc := services.NewGISService(gisRepo, nil, repo)
	baseLayerSvc := services.NewBaseLayerService(baseLayerRepo, repo, nil, cfg)
	wsRegSvc := services.NewWorkspaceRegistryService(wsRegRepo, gisSvc, repo, nil)
	dsSvc := services.NewDatasetService(cfg, repo, sm)
	dsSvc.SetGISService(gisSvc)
	exportSvc := services.NewExportService(repo, sm)
	exportSvc.SetGISService(gisSvc)
	exportSvc.SetBaseLayerService(baseLayerSvc)
	publishSvc := services.NewPublishService(filepath.Join(tmpRoot, "pubstore"), repo, sm, exportSvc, gisSvc, nil)
	georefSvc := services.NewGeoreferenceService(repo, sm)

	mw := api.NewMiddleware(repo, authRepo)
	router := api.NewRouter(
		repo, authRepo, projRepo, mw,
		upload.NewHandler(dsSvc, repo, sm),
		datasets.NewHandler(dsSvc, repo),
		georeference.NewHandler(georefSvc, repo),
		export.NewHandler(exportSvc, repo),
		auth.NewHandler(authRepo),
		projects.NewHandler(projRepo, repo, authRepo, nil, ""),
	)
	router.SetGISHandler(gisFn.NewHandler(gisSvc))
	router.SetBaseLayerHandler(baselayers.NewHandler(baseLayerSvc))
	router.SetWorkspaceRegistryHandler(workspaceregistry.NewHandler(wsRegSvc))
	router.SetPublishHandler(publish.NewHandler(publishSvc, "http://localhost:5173"))
	handler := router.BuildHandler()

	// Create two users (owner Alice + non-owner Bob) via test-login
	loginUser := func(email, name string) string {
		body, _ := json.Marshal(map[string]string{"email": email, "name": name})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/test-login", bytes.NewReader(body))
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("test-login %s failed: %d %s", email, rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp.Data.Token
	}
	aliceToken := loginUser("alice-pub@example.com", "Alice")
	bobToken := loginUser("bob-pub@example.com", "Bob")

	// Upload georeferenced PNG raster + multi-layer GeoPackage into Alice's session
	sess := repo.CreateSession()
	sessionID := sess.ID

	rasterSrc := filepath.Join(tmpRoot, "campus_aerial.png")
	createTestGeorefPNG(t, rasterSrc)
	rasterDs, err := dsSvc.HandleUpload(sessionID, makeMultipartFileHeader(t, "file", rasterSrc))
	if err != nil {
		t.Fatalf("upload raster: %v", err)
	}
	rasterDs.GeoreferenceStatus = "completed"
	rasterDs.ExtractedMetadata["bounds"] = map[string]interface{}{
		"min_x": -122.425,
		"min_y": 37.770,
		"max_x": -122.405,
		"max_y": 37.788,
	}
	rasterDs.ExtractedMetadata["crs"] = "EPSG:4326"
	_ = repo.UpdateDataset(sessionID, rasterDs)

	gpkgSrc := filepath.Join(tmpRoot, "campus_vectors.gpkg")
	createTestMultiLayerGPKG(t, gpkgSrc)
	gpkgDs, err := dsSvc.HandleUpload(sessionID, makeMultipartFileHeader(t, "file", gpkgSrc))
	if err != nil {
		t.Fatalf("upload gpkg: %v", err)
	}
	if _, _, err := wsRegSvc.AddDatasetLayers(context.Background(), "alice", rasterDs.ID, models.AddDatasetLayersRequest{
		DatasetID: gpkgDs.ID,
	}); err != nil {
		t.Fatalf("AddDatasetLayers: %v", err)
	}

	doPublish := func(token string, payload map[string]interface{}) (int, map[string]interface{}) {
		b, _ := json.Marshal(payload)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/publish", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Session-ID", sessionID)
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(rec, req)
		var parsed struct {
			Data map[string]interface{} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
		return rec.Code, parsed.Data
	}

	// =========================================================================
	// 1. Publish Combined Map (Raster + Vector, Public)
	// =========================================================================
	code, combData := doPublish(aliceToken, map[string]interface{}{
		"workspace_id": rasterDs.ID,
		"name":         "SF Combined Campus Map",
		"mode":         "combined",
		"is_public":    true,
		"min_zoom":     12,
		"max_zoom":     13,
		"initial_zoom": 13,
	})
	if code != http.StatusCreated {
		t.Fatalf("combined publish failed (%d): %+v", code, combData)
	}
	combID, _ := combData["map_id"].(string)
	if !strings.HasPrefix(combID, "pub_") {
		t.Fatalf("expected pub_ map_id, got %q", combID)
	}

	// Verify /maps/{map_id} loads WITHOUT workspace session or auth (public HTML + JSON)
	recHTML := httptest.NewRecorder()
	reqHTML := httptest.NewRequest(http.MethodGet, "/maps/"+combID, nil)
	reqHTML.Header.Set("Accept", "text/html")
	handler.ServeHTTP(recHTML, reqHTML)
	if recHTML.Code != http.StatusOK || !strings.Contains(recHTML.Body.String(), "maplibregl.Map") {
		t.Fatalf("expected HTML MapLibre viewer at /maps/%s, got %d", combID, recHTML.Code)
	}

	recJSON := httptest.NewRecorder()
	reqJSON := httptest.NewRequest(http.MethodGet, "/api/published-maps/"+combID, nil)
	handler.ServeHTTP(recJSON, reqJSON)
	if recJSON.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/published-maps/%s, got %d", combID, recJSON.Code)
	}

	// Verify /maps/{map_id}/style.json is valid MapLibre v8 style with BOTH raster and vector sources
	recStyle := httptest.NewRecorder()
	reqStyle := httptest.NewRequest(http.MethodGet, "/maps/"+combID+"/style.json", nil)
	handler.ServeHTTP(recStyle, reqStyle)
	if recStyle.Code != http.StatusOK {
		t.Fatalf("expected 200 for style.json, got %d (%s)", recStyle.Code, recStyle.Body.String())
	}
	var styleDoc map[string]interface{}
	if err := json.Unmarshal(recStyle.Body.Bytes(), &styleDoc); err != nil {
		t.Fatalf("invalid style.json: %v", err)
	}
	sources, _ := styleDoc["sources"].(map[string]interface{})
	hasRasterSrc := false
	hasVectorSrc := false
	for k, v := range sources {
		if m, ok := v.(map[string]interface{}); ok {
			if strings.HasPrefix(k, "pub-raster-") && m["type"] == "raster" {
				hasRasterSrc = true
			}
			if strings.HasPrefix(k, "pub-vector-") && m["type"] == "vector" {
				hasVectorSrc = true
			}
		}
	}
	if !hasRasterSrc || !hasVectorSrc {
		t.Fatalf("combined style.json missing raster (%v) or vector (%v) sources: %+v", hasRasterSrc, hasVectorSrc, sources)
	}

	// Verify published vector tile endpoint (/tiles/{map_id}/vector/hospitals/0/0/0.pbf) with ETag caching
	recVT := httptest.NewRecorder()
	reqVT := httptest.NewRequest(http.MethodGet, "/tiles/"+combID+"/vector/hospitals/0/0/0.pbf", nil)
	handler.ServeHTTP(recVT, reqVT)
	if recVT.Code != http.StatusOK {
		t.Fatalf("expected 200 for published vector tile, got %d (%s)", recVT.Code, recVT.Body.String())
	}
	if recVT.Header().Get("Content-Type") != "application/vnd.mapbox-vector-tile" {
		t.Fatalf("expected MVT content-type, got %q", recVT.Header().Get("Content-Type"))
	}
	if recVT.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip Content-Encoding, got %q", recVT.Header().Get("Content-Encoding"))
	}
	etag := recVT.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected ETag header for HTTP caching")
	}
	rec304 := httptest.NewRecorder()
	req304 := httptest.NewRequest(http.MethodGet, "/tiles/"+combID+"/vector/hospitals/0/0/0.pbf", nil)
	req304.Header.Set("If-None-Match", etag)
	handler.ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified on matching ETag, got %d", rec304.Code)
	}

	// Verify published raster tile serves actual visible PNG pixels (regression check for empty tile_cache bug)
	// SF bounds [-122.42, 37.77, -122.40, 37.79] intersect z=12, x=655, y=1583
	recRT := httptest.NewRecorder()
	reqRT := httptest.NewRequest(http.MethodGet, "/tiles/"+combID+"/raster/campus_aerial/12/655/1583.png", nil)
	handler.ServeHTTP(recRT, reqRT)
	if recRT.Code != http.StatusOK {
		t.Fatalf("expected 200 for published raster tile, got %d (%s)", recRT.Code, recRT.Body.String())
	}
	rtImg, rtErr := png.Decode(bytes.NewReader(recRT.Body.Bytes()))
	if rtErr != nil {
		t.Fatalf("failed to decode published raster PNG tile: %v", rtErr)
	}
	nonTransparentRaster := 0
	for py := rtImg.Bounds().Min.Y; py < rtImg.Bounds().Max.Y; py++ {
		for px := rtImg.Bounds().Min.X; px < rtImg.Bounds().Max.X; px++ {
			_, _, _, a := rtImg.At(px, py).RGBA()
			if a > 0 {
				nonTransparentRaster++
			}
		}
	}
	if nonTransparentRaster == 0 {
		t.Fatalf("published raster tile 12/655/1583.png was completely transparent (0 visible pixels)")
	}

	// Verify composite PNG tile (/tiles/{map_id}/composite/{z}/{x}/{y}.png) also renders raster pixels
	recComp := httptest.NewRecorder()
	reqComp := httptest.NewRequest(http.MethodGet, "/tiles/"+combID+"/composite/12/655/1583.png", nil)
	handler.ServeHTTP(recComp, reqComp)
	if recComp.Code != http.StatusOK {
		t.Fatalf("expected 200 for composite PNG tile, got %d (%s)", recComp.Code, recComp.Body.String())
	}
	compImg, compErr := png.Decode(bytes.NewReader(recComp.Body.Bytes()))
	if compErr != nil {
		t.Fatalf("failed to decode composite PNG tile: %v", compErr)
	}
	nonTransparentComp := 0
	for py := compImg.Bounds().Min.Y; py < compImg.Bounds().Max.Y; py++ {
		for px := compImg.Bounds().Min.X; px < compImg.Bounds().Max.X; px++ {
			_, _, _, a := compImg.At(px, py).RGBA()
			if a > 0 {
				nonTransparentComp++
			}
		}
	}
	if nonTransparentComp == 0 {
		t.Fatalf("composite tile 12/655/1583.png was completely transparent (0 visible pixels)")
	}

	// Verify individual layer TileJSON metadata (/tiles/{map_id}/vector/hospitals/metadata.json)
	recVMeta := httptest.NewRecorder()
	reqVMeta := httptest.NewRequest(http.MethodGet, "/tiles/"+combID+"/vector/hospitals/metadata.json", nil)
	handler.ServeHTTP(recVMeta, reqVMeta)
	if recVMeta.Code != http.StatusOK {
		t.Fatalf("expected 200 for vector metadata.json, got %d", recVMeta.Code)
	}
	var vMeta map[string]interface{}
	_ = json.Unmarshal(recVMeta.Body.Bytes(), &vMeta)
	if vMeta["tilejson"] != "3.0.0" || vMeta["source_layer"] != "hospitals" || vMeta["interactive"] != true {
		t.Fatalf("unexpected vector TileJSON metadata: %+v", vMeta)
	}

	// =========================================================================
	// 2. Independent Raster-Only Web Tile Publishing
	// =========================================================================
	codeR, rData := doPublish(aliceToken, map[string]interface{}{
		"workspace_id": rasterDs.ID,
		"name":         "SF Raster Only",
		"mode":         "raster",
		"is_public":    true,
		"min_zoom":     12,
		"max_zoom":     13,
	})
	if codeR != http.StatusCreated {
		t.Fatalf("raster-only publish failed (%d): %+v", codeR, rData)
	}
	rMapID, _ := rData["map_id"].(string)
	recRMeta := httptest.NewRecorder()
	reqRMeta := httptest.NewRequest(http.MethodGet, "/tiles/"+rMapID+"/raster/campus_aerial/metadata.json", nil)
	handler.ServeHTTP(recRMeta, reqRMeta)
	if recRMeta.Code != http.StatusOK {
		t.Fatalf("expected 200 for raster metadata.json, got %d (%s)", recRMeta.Code, recRMeta.Body.String())
	}
	// Vector tiles must NOT be exposed on a raster-only published map
	recNoVec := httptest.NewRecorder()
	reqNoVec := httptest.NewRequest(http.MethodGet, "/tiles/"+rMapID+"/vector/hospitals/0/0/0.pbf", nil)
	handler.ServeHTTP(recNoVec, reqNoVec)
	if recNoVec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for vector tile on raster-only map, got %d", recNoVec.Code)
	}

	// =========================================================================
	// 3. Independent Vector-Only Web Tile Publishing (Restricted)
	// =========================================================================
	codeV, vData := doPublish(aliceToken, map[string]interface{}{
		"workspace_id": rasterDs.ID,
		"name":         "SF Restricted Vectors",
		"mode":         "vector",
		"is_public":    false,
	})
	if codeV != http.StatusCreated {
		t.Fatalf("vector-only publish failed (%d): %+v", codeV, vData)
	}
	vMapID, _ := vData["map_id"].(string)
	vToken, _ := vData["access_token"].(string)
	if vToken == "" {
		t.Fatalf("expected access_token for restricted published map")
	}

	// Anonymous access without token MUST be denied (403)
	recDeny := httptest.NewRecorder()
	reqDeny := httptest.NewRequest(http.MethodGet, "/tiles/"+vMapID+"/vector/hospitals/0/0/0.pbf", nil)
	handler.ServeHTTP(recDeny, reqDeny)
	if recDeny.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for restricted tile without token, got %d", recDeny.Code)
	}

	// Access WITH valid token MUST succeed (200)
	recAllow := httptest.NewRecorder()
	reqAllow := httptest.NewRequest(http.MethodGet, "/tiles/"+vMapID+"/vector/hospitals/0/0/0.pbf?token="+vToken, nil)
	handler.ServeHTTP(recAllow, reqAllow)
	if recAllow.Code != http.StatusOK {
		t.Fatalf("expected 200 for restricted tile with valid token, got %d", recAllow.Code)
	}

	// =========================================================================
	// 4. Persistence Reload (works after workspace session deleted!)
	// =========================================================================
	repo.DeleteSession(sessionID)
	reloadedSvc := services.NewPublishService(filepath.Join(tmpRoot, "pubstore"), repo, sm, exportSvc, gisSvc, nil)
	reloadedPM, err := reloadedSvc.GetPublishedMap(combID, "", "")
	if err != nil || reloadedPM == nil {
		t.Fatalf("published map did not persist across reload without workspace session: %v", err)
	}

	// =========================================================================
	// 5. Ownership Enforcement & Revocation
	// =========================================================================
	// Non-owner Bob cannot revoke Alice's published map
	recBobRev := httptest.NewRecorder()
	reqBobRev := httptest.NewRequest(http.MethodDelete, "/api/publish/"+combID, nil)
	reqBobRev.Header.Set("Authorization", "Bearer "+bobToken)
	handler.ServeHTTP(recBobRev, reqBobRev)
	if recBobRev.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when non-owner revokes published map, got %d", recBobRev.Code)
	}

	// Owner Alice revokes the combined map
	recAliceRev := httptest.NewRecorder()
	reqAliceRev := httptest.NewRequest(http.MethodDelete, "/api/publish/"+combID, nil)
	reqAliceRev.Header.Set("Authorization", "Bearer "+aliceToken)
	handler.ServeHTTP(recAliceRev, reqAliceRev)
	if recAliceRev.Code != http.StatusOK {
		t.Fatalf("expected 200 when owner revokes published map, got %d", recAliceRev.Code)
	}

	// After revocation, style.json and tiles must return 404
	recAfterRev := httptest.NewRecorder()
	reqAfterRev := httptest.NewRequest(http.MethodGet, "/maps/"+combID+"/style.json", nil)
	handler.ServeHTTP(recAfterRev, reqAfterRev)
	if recAfterRev.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after revocation, got %d", recAfterRev.Code)
	}

	// Cleanup temp files
	_ = os.Remove(rasterSrc)
	_ = os.Remove(gpkgSrc)
}
