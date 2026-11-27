package tests

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func createTinyTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 60, G: 180, B: 120, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create png failed: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png failed: %v", err)
	}
}

func TestGeoreferenceSyncsGISRasterLayerAndCrossSessionState(t *testing.T) {
	tmpDir := t.TempDir()
	sm, err := storage.NewStorageManager(
		filepath.Join(tmpDir, "sessions"),
		filepath.Join(tmpDir, "uploads"),
		filepath.Join(tmpDir, "processed"),
	)
	if err != nil {
		t.Fatalf("storage manager init failed: %v", err)
	}

	sessionRepo := repository.NewSessionRepository()
	sessionRepo.SetStorageDir(filepath.Join(tmpDir, "sessions"))
	gisRepo := repository.NewMemoryGISRepository()
	gisService := services.NewGISService(gisRepo, nil, sessionRepo)
	georefService := services.NewGeoreferenceService(sessionRepo, sm)
	georefService.SetGISRepo(gisRepo)

	sessA := sessionRepo.CreateSession().ID
	sessB := sessionRepo.CreateSession().ID

	dsID := "ds_raster_sync_1"
	imgPath := filepath.Join(sm.GetDatasetDir(sessA, dsID), "images.png")
	createTinyTestPNG(t, imgPath, 400, 300)

	origDS := &models.Dataset{
		ID:          dsID,
		SessionID:   sessA,
		Filename:    "images.png",
		StoragePath: imgPath,
		ExtractedMetadata: map[string]interface{}{
			"width":  400,
			"height": 300,
		},
	}
	if err := sessionRepo.InsertDataset(sessA, origDS); err != nil {
		t.Fatalf("insert dataset in sessA failed: %v", err)
	}

	// Simulate cross-session clone created by resolveDatasetAndSession
	clonedDS := *origDS
	clonedDS.SessionID = sessB
	clonedDS.ExtractedMetadata = map[string]interface{}{
		"width":  400,
		"height": 300,
	}
	if err := sessionRepo.InsertDataset(sessB, &clonedDS); err != nil {
		t.Fatalf("insert dataset in sessB failed: %v", err)
	}

	// Register unreferenced raster layer in GISRepo (same as WorkspaceRegistryService.AddLayersToWorkspace)
	ctx := context.Background()
	wsID := "map_test_sync"
	createdLayer, err := gisService.CreateLayer(ctx, "user_1", models.CreateLayerRequest{
		MapID:        wsID,
		DatasetID:    dsID,
		LayerName:    "images",
		LayerType:    "raster",
		GeometryType: "Raster",
	})
	if err != nil {
		t.Fatalf("create raster layer failed: %v", err)
	}
	if len(createdLayer.Bounds) != 0 {
		t.Fatalf("expected unreferenced raster layer to start with empty bounds, got %v", createdLayer.Bounds)
	}

	// Add 3 GCPs in sessB around New Delhi
	gcps := []*models.ControlPoint{
		{DatasetID: dsID, ImageX: 0, ImageY: 0, Longitude: 77.2000, Latitude: 28.6200},
		{DatasetID: dsID, ImageX: 399, ImageY: 0, Longitude: 77.2200, Latitude: 28.6200},
		{DatasetID: dsID, ImageX: 0, ImageY: 299, Longitude: 77.2000, Latitude: 28.6000},
	}
	for _, cp := range gcps {
		if _, err := georefService.CreateControlPoint(sessB, dsID, cp); err != nil {
			t.Fatalf("create control point failed: %v", err)
		}
	}

	res, err := georefService.Georeference(sessB, dsID, "save", "affine")
	if err != nil {
		t.Fatalf("georeference failed: %v", err)
	}
	if len(res.MapLibreBounds) != 4 {
		t.Fatalf("expected 4-element MapLibreBounds, got %v", res.MapLibreBounds)
	}

	// 1. Verify GISRepo layer was permanently updated with bounds, CRS, and metadata
	storedLayer, err := gisRepo.GetLayer(ctx, createdLayer.LayerID)
	if err != nil {
		t.Fatalf("get stored layer failed: %v", err)
	}
	if len(storedLayer.Bounds) != 4 {
		t.Fatalf("expected stored GISLayer to have 4-element bounds after georef, got %v", storedLayer.Bounds)
	}
	if storedLayer.Metadata["is_georeferenced"] != true {
		t.Fatalf("expected stored GISLayer metadata is_georeferenced=true, got %v", storedLayer.Metadata)
	}
	if storedLayer.Metadata["coordinates"] == nil {
		t.Fatalf("expected stored GISLayer metadata coordinates to be populated")
	}

	// 2. Verify GISService.GetLayers returns georeferenced bounds & metadata
	layers, _, err := gisService.GetLayers(ctx, wsID, "user_1")
	if err != nil {
		t.Fatalf("GetLayers failed: %v", err)
	}
	if len(layers) != 1 || len(layers[0].Bounds) != 4 {
		t.Fatalf("expected GetLayers to return 1 georeferenced raster layer, got %+v", layers)
	}

	// 3. Verify original session copy (sessA) was synced across sessions
	origAfter, err := sessionRepo.GetDataset(sessA, dsID)
	if err != nil {
		t.Fatalf("GetDataset(sessA) failed: %v", err)
	}
	if origAfter.GeoreferenceStatus != "completed" || origAfter.MapLayer == nil {
		t.Fatalf("expected sessA dataset copy to be synced as completed, got status=%q mapLayer=%v", origAfter.GeoreferenceStatus, origAfter.MapLayer)
	}
}
