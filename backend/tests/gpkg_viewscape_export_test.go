package tests

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/functions/workspaceregistry"
	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func createTestMultiLayerGPKG(t *testing.T, destPath string) {
	t.Helper()
	pyExe := gis.ResolvePythonInterpreter()
	script := `
import sys, geopandas as gpd
from shapely.geometry import Point, MultiPoint, LineString, Polygon

out_gpkg = sys.argv[1]

# 1. Point layer (EPSG:4326)
hospitals = gpd.GeoDataFrame(
    {
        "name": ["City General Hospital", "St. Mary Clinic", "Metro Trauma Center"],
        "beds": [250, 80, 420],
        "emergency": ["yes", "no", "yes"],
    },
    geometry=[
        Point(-122.4194, 37.7749),
        Point(-122.4094, 37.7849),
        Point(-122.4294, 37.7649),
    ],
    crs="EPSG:4326",
)
hospitals.to_file(out_gpkg, layer="hospitals", driver="GPKG")

# 2. MultiPoint layer in projected CRS (EPSG:3857) to verify CRS preservation & reprojection
police = gpd.GeoDataFrame(
    {
        "station_name": ["Central Precinct", "Harbor Precinct"],
        "officers": [120, 65],
    },
    geometry=[
        MultiPoint([Point(-122.4150, 37.7780), Point(-122.4160, 37.7790)]),
        MultiPoint([Point(-122.4020, 37.7910)]),
    ],
    crs="EPSG:4326",
).to_crs("EPSG:3857")
police.to_file(out_gpkg, layer="police_stations", driver="GPKG")

# 3. LineString layer (EPSG:4326)
routes = gpd.GeoDataFrame(
    {"route_name": ["Main Corridor", "Bay Loop"], "lanes": [4, 2]},
    geometry=[
        LineString([(-122.43, 37.76), (-122.41, 37.78), (-122.40, 37.79)]),
        LineString([(-122.42, 37.77), (-122.405, 37.775)]),
    ],
    crs="EPSG:4326",
)
routes.to_file(out_gpkg, layer="transit_routes", driver="GPKG")

# 4. Polygon layer (EPSG:4326)
zones = gpd.GeoDataFrame(
    {"zone_name": ["Downtown District"], "zoning": ["commercial"]},
    geometry=[
        Polygon([
            (-122.425, 37.770),
            (-122.405, 37.770),
            (-122.405, 37.788),
            (-122.425, 37.788),
            (-122.425, 37.770),
        ])
    ],
    crs="EPSG:4326",
)
zones.to_file(out_gpkg, layer="city_zones", driver="GPKG")
`
	cmd := exec.Command(pyExe, "-c", script, destPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed generating test GeoPackage: %v\n%s", err, string(out))
	}
}

func createTestGeorefPNG(t *testing.T, destPath string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.RGBA{R: uint8(40 + x), G: uint8(160), B: uint8(90 + y/2), A: 255})
		}
	}
	f, err := os.Create(destPath)
	if err != nil {
		t.Fatalf("create test png: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
}

func makeMultipartFileHeader(t *testing.T, fieldName, filePath string) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		t.Fatalf("copy file: %v", err)
	}
	_ = f.Close()
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	return req.MultipartForm.File[fieldName][0]
}

func TestGeoPackageVectorTilesAndIndependentViewscapeExportAndReopen(t *testing.T) {
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
	handler := router.BuildHandler()

	sess := repo.CreateSession()
	sessionID := sess.ID

	// 1. Create and upload a georeferenced raster PNG
	rasterSrc := filepath.Join(tmpRoot, "campus_floor1.png")
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

	// 2. Create and upload multi-layer GeoPackage (Point, MultiPoint in EPSG:3857, LineString, Polygon)
	gpkgSrc := filepath.Join(tmpRoot, "city_facilities.gpkg")
	createTestMultiLayerGPKG(t, gpkgSrc)
	gpkgDs, err := dsSvc.HandleUpload(sessionID, makeMultipartFileHeader(t, "file", gpkgSrc))
	if err != nil {
		t.Fatalf("upload gpkg: %v", err)
	}

	// Verify automatic GeoPackage-to-local-vector-tiles conversion on upload
	if gpkgDs.TileStatus != "completed" {
		t.Fatalf("expected gpkg TileStatus=completed, got %q", gpkgDs.TileStatus)
	}
	vtMap, ok := gpkgDs.ExtractedMetadata["vector_tiles"].(map[string]interface{})
	if !ok || len(vtMap) != 4 {
		t.Fatalf("expected 4 converted vector tile layers in metadata, got %d (%v)", len(vtMap), vtMap)
	}
	for _, expectedSlug := range []string{"hospitals", "police_stations", "transit_routes", "city_zones"} {
		entry, exists := vtMap[expectedSlug].(map[string]interface{})
		if !exists {
			t.Fatalf("missing vector_tiles entry for %s", expectedSlug)
		}
		layerDir, _ := entry["layer_dir"].(string)
		if _, err := os.Stat(filepath.Join(layerDir, "metadata.json")); err != nil {
			t.Fatalf("missing metadata.json for %s: %v", expectedSlug, err)
		}
		if _, err := os.Stat(filepath.Join(layerDir, "style.json")); err != nil {
			t.Fatalf("missing style.json for %s: %v", expectedSlug, err)
		}
		pbfCount := 0
		_ = filepath.Walk(filepath.Join(layerDir, "tiles"), func(p string, info os.FileInfo, wErr error) error {
			if wErr == nil && !info.IsDir() && strings.HasSuffix(p, ".pbf") {
				pbfCount++
			}
			return nil
		})
		if pbfCount == 0 {
			t.Fatalf("expected >=1 .pbf tile for %s under tiles/, got 0", expectedSlug)
		}
		if expectedSlug == "hospitals" || expectedSlug == "police_stations" {
			if _, err := os.Stat(filepath.Join(layerDir, "tiles", "0", "0", "0.pbf")); err != nil {
				t.Fatalf("missing tiles/0/0/0.pbf for point layer %s: %v", expectedSlug, err)
			}
		}
	}

	// Verify CRS preservation for police_stations (EPSG:3857) and feature counts
	policeMetaRaw, err := os.ReadFile(filepath.Join(filepath.Dir(gpkgDs.StoragePath), "vector_tiles", "police_stations", "metadata.json"))
	if err != nil {
		t.Fatalf("read police_stations metadata.json: %v", err)
	}
	var policeMeta map[string]interface{}
	_ = json.Unmarshal(policeMetaRaw, &policeMeta)
	if policeMeta["source_crs"] != "EPSG:3857" {
		t.Fatalf("expected police_stations source_crs=EPSG:3857, got %v", policeMeta["source_crs"])
	}
	if int(policeMeta["feature_count"].(float64)) != 2 {
		t.Fatalf("expected police_stations feature_count=2, got %v", policeMeta["feature_count"])
	}

	// 3. Register GeoPackage layers into workspace via WorkspaceRegistryService
	wsLayers, _, err := wsRegSvc.AddDatasetLayers(context.Background(), "user_test", rasterDs.ID, models.AddDatasetLayersRequest{
		DatasetID: gpkgDs.ID,
	})
	if err != nil {
		t.Fatalf("AddDatasetLayers: %v", err)
	}
	if len(wsLayers) != 4 {
		t.Fatalf("expected 4 workspace layers added, got %d", len(wsLayers))
	}

	// Verify HTTP serving of local vector tiles, metadata.json, style.json, and 204 for missing tile
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/datasets/"+gpkgDs.ID+"/vector-tiles/hospitals/0/0/0.pbf", nil)
	req.Header.Set("X-Session-ID", sessionID)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for vector tile 0/0/0.pbf, got %d (%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.mapbox-vector-tile" {
		t.Fatalf("expected MVT content-type, got %q", ct)
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("expected gzip Content-Encoding for compressed MVT, got %q", ce)
	}

	rec204 := httptest.NewRecorder()
	req204 := httptest.NewRequest(http.MethodGet, "/datasets/"+gpkgDs.ID+"/vector-tiles/hospitals/14/0/0.pbf", nil)
	req204.Header.Set("X-Session-ID", sessionID)
	handler.ServeHTTP(rec204, req204)
	if rec204.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for empty tile outside bounds, got %d", rec204.Code)
	}

	recMeta := httptest.NewRecorder()
	reqMeta := httptest.NewRequest(http.MethodGet, "/datasets/"+gpkgDs.ID+"/vector-tiles/hospitals/metadata.json", nil)
	reqMeta.Header.Set("X-Session-ID", sessionID)
	handler.ServeHTTP(recMeta, reqMeta)
	if recMeta.Code != http.StatusOK {
		t.Fatalf("expected 200 for vector tile metadata.json, got %d", recMeta.Code)
	}

	// Configure base layer sources (WMS + Bing + custom XYZ) for export
	optsWithBases := services.DefaultSelectiveExportOptions()
	minZ := 12
	maxZ := 13
	initZ := 13
	optsWithBases.ZoomConfig = &services.ZoomConfiguration{MinZoom: &minZ, MaxZoom: &maxZ, InitialZoom: &initZ}
	optsWithBases.BaseLayerSources = []map[string]interface{}{
		{
			"id":          "wms_ortho",
			"name":        "City Orthophoto WMS",
			"source_type": "wms",
			"url":         "https://example.org/wms?service=WMS&request=GetMap",
			"wms_layers":  "ortho_2025",
		},
		{
			"id":          "bing_aerial",
			"name":        "Bing Aerial",
			"source_type": "bing",
			"provider":    "bing",
			"url":         "https://ecn.t0.tiles.virtualearth.net/tiles/a{quadkey}.jpeg",
		},
	}

	inspectZipEntries := func(zipPath string) map[string]bool {
		zr, err := zip.OpenReader(zipPath)
		if err != nil {
			t.Fatalf("open zip %s: %v", zipPath, err)
		}
		defer zr.Close()
		out := make(map[string]bool, len(zr.File))
		for _, f := range zr.File {
			out[filepath.ToSlash(f.Name)] = true
		}
		return out
	}

	// 4A. Independent Raster Layers ZIP
	rasterZip, _, err := exportSvc.GenerateSelectiveExport(sessionID, rasterDs.ID, services.SelectiveExportParams{
		Format:      "viewscape_raster",
		ProjectName: "SF Campus",
		WorkspaceID: rasterDs.ID,
		Options:     optsWithBases,
	})
	if err != nil {
		t.Fatalf("viewscape_raster export failed: %v", err)
	}
	rEntries := inspectZipEntries(rasterZip)
	if !rEntries["Viewscape/manifest.json"] {
		t.Fatalf("raster ZIP missing Viewscape/manifest.json")
	}
	hasRasterMeta := false
	hasRasterTile := false
	for k := range rEntries {
		if strings.HasPrefix(k, "Viewscape/raster_layers/") && strings.HasSuffix(k, "/metadata.json") {
			hasRasterMeta = true
		}
		if strings.HasPrefix(k, "Viewscape/raster_layers/") && strings.HasSuffix(k, ".png") {
			hasRasterTile = true
		}
		if strings.HasPrefix(k, "Viewscape/vector_layers/") {
			t.Fatalf("independent raster ZIP should not contain vector_layers: %s", k)
		}
	}
	if !hasRasterMeta || !hasRasterTile {
		t.Fatalf("raster ZIP missing metadata (%v) or png tiles (%v)", hasRasterMeta, hasRasterTile)
	}

	// 4B. Independent Vector Layers ZIP
	vectorZip, _, err := exportSvc.GenerateSelectiveExport(sessionID, rasterDs.ID, services.SelectiveExportParams{
		Format:      "viewscape_vector",
		ProjectName: "SF Campus",
		WorkspaceID: rasterDs.ID,
		Options:     optsWithBases,
	})
	if err != nil {
		t.Fatalf("viewscape_vector export failed: %v", err)
	}
	vEntries := inspectZipEntries(vectorZip)
	if !vEntries["Viewscape/manifest.json"] {
		t.Fatalf("vector ZIP missing Viewscape/manifest.json")
	}
	for _, slug := range []string{"hospitals", "police_stations", "transit_routes", "city_zones"} {
		if !vEntries["Viewscape/vector_layers/"+slug+"/metadata.json"] {
			t.Fatalf("vector ZIP missing metadata.json for %s", slug)
		}
		if !vEntries["Viewscape/vector_layers/"+slug+"/style.json"] {
			t.Fatalf("vector ZIP missing style.json for %s", slug)
		}
		hasPBF := false
		prefix := "Viewscape/vector_layers/" + slug + "/tiles/"
		for k := range vEntries {
			if strings.HasPrefix(k, prefix) && strings.HasSuffix(k, ".pbf") {
				hasPBF = true
				break
			}
		}
		if !hasPBF {
			t.Fatalf("vector ZIP missing .pbf tiles for %s", slug)
		}
	}
	for k := range vEntries {
		if strings.HasPrefix(k, "Viewscape/raster_layers/") {
			t.Fatalf("independent vector ZIP should not contain raster_layers: %s", k)
		}
	}

	// Verify Vector_Layers/ contains GeoJSON files only (separate per-layer .geojson + all_layers.geojson + points.geojson)
	for _, vecSlug := range []string{"hospitals", "police_stations", "transit_routes", "city_zones"} {
		if !vEntries["Vector_Layers/"+vecSlug+".geojson"] {
			t.Fatalf("vector ZIP missing Vector_Layers/%s.geojson", vecSlug)
		}
	}
	if !vEntries["Vector_Layers/all_layers.geojson"] {
		t.Fatalf("vector ZIP missing Vector_Layers/all_layers.geojson")
	}
	if !vEntries["Vector_Layers/points.geojson"] {
		t.Fatalf("vector ZIP missing Vector_Layers/points.geojson")
	}
	// Verify Vector_Layers/ contains multi-layer Vector .mbtiles AND GeoJSON files
	hasVectorMBTiles := false
	for k := range vEntries {
		if strings.HasPrefix(k, "Vector_Layers/") && strings.HasSuffix(k, ".mbtiles") {
			hasVectorMBTiles = true
		}
		if strings.HasPrefix(k, "Vector_Layers/") && !strings.HasSuffix(k, ".geojson") && !strings.HasSuffix(k, ".mbtiles") {
			t.Fatalf("Vector_Layers/ should contain only .mbtiles and .geojson files, found: %s", k)
		}
	}
	if !hasVectorMBTiles {
		t.Fatalf("vector ZIP missing multi-layer Vector .mbtiles under Vector_Layers/")
	}
	zrPts, err := zip.OpenReader(vectorZip)
	if err != nil {
		t.Fatalf("open vectorZip for Vector_Layers/points.geojson: %v", err)
	}
	var ptsRaw []byte
	for _, zf := range zrPts.File {
		if zf.Name == "Vector_Layers/points.geojson" {
			rc, rErr := zf.Open()
			if rErr != nil {
				t.Fatalf("open Vector_Layers/points.geojson: %v", rErr)
			}
			ptsRaw, _ = io.ReadAll(rc)
			_ = rc.Close()
			break
		}
	}
	_ = zrPts.Close()
	var combinedPts struct {
		Type     string                   `json:"type"`
		Features []map[string]interface{} `json:"features"`
	}
	if err := json.Unmarshal(ptsRaw, &combinedPts); err != nil {
		t.Fatalf("invalid points.geojson: %v", err)
	}
	if combinedPts.Type != "FeatureCollection" || len(combinedPts.Features) != 5 {
		t.Fatalf("expected combined points.geojson FeatureCollection with 5 points (3 hospitals + 2 police_stations), got type=%q count=%d", combinedPts.Type, len(combinedPts.Features))
	}

	// 4C. Independent Base Layers ZIP
	baseZip, _, err := exportSvc.GenerateSelectiveExport(sessionID, rasterDs.ID, services.SelectiveExportParams{
		Format:      "viewscape_base",
		ProjectName: "SF Campus",
		WorkspaceID: rasterDs.ID,
		Options:     optsWithBases,
	})
	if err != nil {
		t.Fatalf("viewscape_base export failed: %v", err)
	}
	bEntries := inspectZipEntries(baseZip)
	if !bEntries["Viewscape/manifest.json"] || !bEntries["Viewscape/base_layers/sources.json"] {
		t.Fatalf("base ZIP missing manifest.json or base_layers/sources.json")
	}
	// Verify WMS and Bing configs preserved in sources.json
	zrBase, _ := zip.OpenReader(baseZip)
	for _, f := range zrBase.File {
		if filepath.ToSlash(f.Name) == "Viewscape/base_layers/sources.json" {
			rc, _ := f.Open()
			raw, _ := io.ReadAll(rc)
			_ = rc.Close()
			if !strings.Contains(string(raw), "wms_ortho") || !strings.Contains(string(raw), "bing_aerial") {
				t.Fatalf("base sources.json did not preserve WMS and Bing configs: %s", string(raw))
			}
		}
	}
	_ = zrBase.Close()

	// 4D. Complete Viewscape ZIP
	completeZip, _, err := exportSvc.GenerateSelectiveExport(sessionID, rasterDs.ID, services.SelectiveExportParams{
		Format:      "viewscape_complete",
		ProjectName: "SF Campus",
		WorkspaceID: rasterDs.ID,
		Options:     optsWithBases,
	})
	if err != nil {
		t.Fatalf("viewscape_complete export failed: %v", err)
	}
	cEntries := inspectZipEntries(completeZip)
	for _, required := range []string{
		"Viewscape/manifest.json",
		"Viewscape/styles/layer_styles.json",
		"Viewscape/project/viewscape_config.json",
		"Viewscape/base_layers/sources.json",
		"Viewscape/vector_layers/hospitals/metadata.json",
		"Viewscape/vector_layers/hospitals/style.json",
		"Viewscape/vector_layers/hospitals/tiles/0/0/0.pbf",
	} {
		if !cEntries[required] {
			t.Fatalf("complete Viewscape ZIP missing required entry %s", required)
		}
	}

	// 5. Reopen Complete Viewscape ZIP in a BRAND NEW session WITHOUT original .gpkg!
	_ = os.Remove(gpkgSrc)
	_ = os.Remove(gpkgDs.StoragePath)

	reopenSess := repo.CreateSession()
	reopenFH := makeMultipartFileHeader(t, "file", completeZip)

	insp, err := dsSvc.InspectProjectPackage(reopenSess.ID, reopenFH)
	if err != nil {
		t.Fatalf("InspectProjectPackage for Viewscape ZIP failed: %v", err)
	}
	if !insp.Valid || insp.PackageType != "viewscape_complete" {
		t.Fatalf("expected valid viewscape_complete inspection, got %+v", insp)
	}
	if insp.LayerCount < 5 {
		t.Fatalf("expected at least 5 layers (1 raster + 4 vector), got %d", insp.LayerCount)
	}

	restoredDs, err := dsSvc.ReopenProjectUpload(reopenSess.ID, makeMultipartFileHeader(t, "file", completeZip))
	if err != nil {
		t.Fatalf("ReopenProjectUpload for Viewscape ZIP failed: %v", err)
	}

	// Verify restored GIS vector tile layers exist with interactive point config & original CRS
	restoredGISLayers, err := gisRepo.GetLayers(context.Background(), restoredDs.ID)
	if err != nil || len(restoredGISLayers) != 4 {
		t.Fatalf("expected 4 restored GIS vector layers, got %d (err=%v)", len(restoredGISLayers), err)
	}
	foundHospitals := false
	for _, gl := range restoredGISLayers {
		vt, _ := gl.Metadata["vector_tiles"].(map[string]interface{})
		if vt == nil {
			t.Fatalf("restored layer %s missing metadata.vector_tiles", gl.LayerName)
		}
		if vt["interactive"] != true || vt["promote_id"] != "feature_id" {
			t.Fatalf("restored layer %s missing interactive point config: %+v", gl.LayerName, vt)
		}
		if gl.LayerName == "hospitals" {
			foundHospitals = true
		}
	}
	if !foundHospitals {
		t.Fatalf("restored layers missing hospitals point layer")
	}

	// Verify restored vector tiles are served over HTTP from the reopened dataset (without original .gpkg)
	recReopenVT := httptest.NewRecorder()
	reqReopenVT := httptest.NewRequest(http.MethodGet, "/datasets/"+restoredDs.ID+"/vector-tiles/hospitals/0/0/0.pbf", nil)
	reqReopenVT.Header.Set("X-Session-ID", reopenSess.ID)
	handler.ServeHTTP(recReopenVT, reqReopenVT)
	if recReopenVT.Code != http.StatusOK {
		t.Fatalf("expected 200 serving reopened hospitals vector tile, got %d", recReopenVT.Code)
	}
	if recReopenVT.Header().Get("Content-Type") != "application/vnd.mapbox-vector-tile" {
		t.Fatalf("expected MVT content-type on reopened tile, got %q", recReopenVT.Header().Get("Content-Type"))
	}

	// Verify restored MapProject preserves layer ordering, base_layers, camera, and interaction config
	restoredProj, err := repo.GetMapProject(reopenSess.ID, restoredDs.ID)
	if err != nil || restoredProj == nil {
		t.Fatalf("expected restored MapProject, got err=%v", err)
	}
	if restoredProj.Configuration["camera"] == nil || restoredProj.Configuration["base_layers"] == nil || restoredProj.Configuration["layer_ordering"] == nil {
		t.Fatalf("restored MapProject missing camera, base_layers, or layer_ordering: %+v", restoredProj.Configuration)
	}
}
