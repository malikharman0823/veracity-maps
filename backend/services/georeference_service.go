package services

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

type GeoreferenceService struct {
	repo    *repository.SessionRepository
	storage *storage.StorageManager
}

func NewGeoreferenceService(repo *repository.SessionRepository, sm *storage.StorageManager) *GeoreferenceService {
	return &GeoreferenceService{
		repo:    repo,
		storage: sm,
	}
}

func (s *GeoreferenceService) ListControlPoints(sessionID, datasetID string) ([]*models.ControlPoint, error) {
	cps, err := s.repo.ListControlPoints(sessionID, datasetID)
	if err == nil && len(cps) > 0 {
		return cps, nil
	}

	ds, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil || ds == nil {
		return cps, err
	}

	datasetDir := filepath.Dir(ds.StoragePath)
	candidates := []string{
		filepath.Join(datasetDir, "control_points.json"),
		filepath.Join(datasetDir, "extracted", "control_points.json"),
		filepath.Join(datasetDir, "..", "control_points.json"),
		filepath.Join(datasetDir, "..", "extracted", "control_points.json"),
	}
	for _, c := range candidates {
		if data, err := os.ReadFile(c); err == nil {
			var points []*models.ControlPoint
			if err := json.Unmarshal(data, &points); err == nil && len(points) > 0 {
				for _, p := range points {
					p.DatasetID = datasetID
				}
				_ = s.repo.SetControlPoints(sessionID, datasetID, points)
				return points, nil
			}
		}
	}

	studioCandidates := []string{
		filepath.Join(datasetDir, "mapstudio.json"),
		filepath.Join(datasetDir, "extracted", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "extracted", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "..", "mapstudio.json"),
		filepath.Join(datasetDir, "..", "..", "extracted", "mapstudio.json"),
	}
	for _, c := range studioCandidates {
		if data, err := os.ReadFile(c); err == nil {
			var doc struct {
				ControlPoints []*models.ControlPoint `json:"control_points"`
			}
			if err := json.Unmarshal(data, &doc); err == nil && len(doc.ControlPoints) > 0 {
				for _, p := range doc.ControlPoints {
					p.DatasetID = datasetID
				}
				_ = s.repo.SetControlPoints(sessionID, datasetID, doc.ControlPoints)
				return doc.ControlPoints, nil
			}
		}
	}

	return cps, nil
}

func getDim(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	default:
		return 0
	}
}

func (s *GeoreferenceService) CreateControlPoint(sessionID, datasetID string, cp *models.ControlPoint) (*models.ControlPoint, error) {
	if math.IsNaN(cp.ImageX) || math.IsNaN(cp.ImageY) || math.IsNaN(cp.Longitude) || math.IsNaN(cp.Latitude) {
		return nil, fmt.Errorf("INVALID_COORDINATES: Coordinates must be finite numbers")
	}
	if cp.Longitude < -180 || cp.Longitude > 180 || cp.Latitude < -90 || cp.Latitude > 90 {
		return nil, fmt.Errorf("INVALID_COORDINATES: Longitude must be [-180, 180] and latitude [-90, 90]")
	}
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}
	if w := getDim(dataset.ExtractedMetadata["width"]); w > 0 {
		h := getDim(dataset.ExtractedMetadata["height"])
		if cp.ImageX < 0 || cp.ImageX >= w || cp.ImageY < 0 || cp.ImageY >= h {
			return nil, fmt.Errorf("INVALID_CONTROL_POINT: Image coordinates must fall within raster dimensions (%.0fx%.0f)", w, h)
		}
	}

	if err := s.repo.InsertControlPoint(sessionID, datasetID, cp); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *GeoreferenceService) UpdateControlPoint(sessionID, pointID string, cp *models.ControlPoint) (*models.ControlPoint, error) {
	if math.IsNaN(cp.ImageX) || math.IsNaN(cp.ImageY) || math.IsNaN(cp.Longitude) || math.IsNaN(cp.Latitude) {
		return nil, fmt.Errorf("INVALID_COORDINATES: Coordinates must be finite numbers")
	}
	if cp.Longitude < -180 || cp.Longitude > 180 || cp.Latitude < -90 || cp.Latitude > 90 {
		return nil, fmt.Errorf("INVALID_COORDINATES: Longitude must be [-180, 180] and latitude [-90, 90]")
	}
	if err := s.repo.UpdateControlPoint(sessionID, pointID, cp); err != nil {
		return nil, err
	}
	return cp, nil
}

func (s *GeoreferenceService) DeleteControlPoint(sessionID, pointID string) (*models.ControlPoint, error) {
	return s.repo.DeleteControlPoint(sessionID, pointID)
}

func (s *GeoreferenceService) Georeference(sessionID, datasetID, action, transformation string) (*gis.TransformResult, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}

	points, err := s.repo.ListControlPoints(sessionID, datasetID)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(dataset.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("INVALID_SOURCE_IMAGE: Source image could not be opened: %w", err)
	}
	defer file.Close()

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("INVALID_SOURCE_IMAGE: Failed reading image dimensions: %w", err)
	}

	result, err := gis.CalculateGeoreference(points, cfg.Width, cfg.Height, transformation)
	if err != nil {
		return nil, err
	}

	// Update dataset metadata
	dataset.ExtractedMetadata["transform"] = result.Transform
	dataset.ExtractedMetadata["bounds"] = result.Bounds
	crsStr := "EPSG:4326"
	dataset.ExtractedMetadata["crs"] = crsStr
	dataset.ExtractedMetadata["georeference_source"] = "control_points"

	// Create processed directory
	processedDir := filepath.Join(s.storage.GetDatasetDir(sessionID, datasetID), "processed")
	_ = os.MkdirAll(processedDir, 0755)

	// Write world file
	baseExt := strings.ToLower(filepath.Ext(dataset.Filename))
	worldExt := ".wld"
	switch baseExt {
	case ".png":
		worldExt = ".pgw"
	case ".jpg", ".jpeg":
		worldExt = ".jgw"
	}
	worldPath := filepath.Join(processedDir, "georeferenced"+worldExt)
	_ = gis.WriteWorldFile(worldPath, result.Transform)

	// Also write world file in the source directory next to source file for permanence
	sourceWorldPath := filepath.Join(filepath.Dir(dataset.StoragePath), strings.TrimSuffix(filepath.Base(dataset.Filename), baseExt)+worldExt)
	_ = gis.WriteWorldFile(sourceWorldPath, result.Transform)

	// Generate VRT
	vrtPath := filepath.Join(processedDir, "georeferenced.vrt")
	_ = gis.WriteGDALVrt(vrtPath, dataset.StoragePath, cfg.Width, cfg.Height, result.Transform, "EPSG:4326")

	// If GDAL is installed, run gdal_translate to produce GeoTIFF
	tifPath := filepath.Join(processedDir, "georeferenced.tif")
	gdal := gis.DetectGDAL()
	if gdal.Available {
		cmd := exec.Command(gdal.TranslatePath, "-of", "GTiff", "-co", "COMPRESS=DEFLATE", vrtPath, tifPath)
		if out, err := cmd.CombinedOutput(); err == nil && fileExists(tifPath) {
			dataset.GeoreferencedPath = tifPath
			dataset.ProcessedPath = tifPath
		} else {
			_ = out
			dataset.GeoreferencedPath = vrtPath
			dataset.ProcessedPath = vrtPath
		}
	} else {
		dataset.GeoreferencedPath = vrtPath
		dataset.ProcessedPath = vrtPath
	}

	// Compute corner coordinates: [top-left, top-right, bottom-right, bottom-left]
	a, b, c := result.Transform[0], -result.Transform[1], result.Transform[2]
	d, e, f := result.Transform[3], -result.Transform[4], result.Transform[5]
	project := func(ix, iy float64) []float64 {
		return []float64{
			a*ix + b*(-iy) + c,
			d*ix + e*(-iy) + f,
		}
	}
	wFloat := float64(cfg.Width)
	hFloat := float64(cfg.Height)
	coordinates := [][]float64{
		project(0, 0),
		project(wFloat, 0),
		project(wFloat, hFloat),
		project(0, hFloat),
	}

	dataset.MapLayer = map[string]interface{}{
		"id":          "lyr_" + datasetID,
		"name":        dataset.Filename,
		"type":        "raster",
		"bounds":      result.Bounds,
		"bounds_crs":  "EPSG:4326",
		"coordinates": coordinates,
	}

	dataset.GeoreferenceStatus = "completed"
	_ = s.repo.UpdateDataset(sessionID, dataset)

	// Update or initialize layers
	layers, _ := s.repo.ListLayers(sessionID, datasetID)
	if len(layers) > 0 {
		layers[0].Bounds = result.MapLibreBounds
		_ = s.repo.SetLayers(sessionID, datasetID, layers)
	} else {
		layers = []*models.Layer{
			{
				ID:        "lyr_" + datasetID,
				DatasetID: datasetID,
				Name:      dataset.Filename,
				Type:      "raster",
				Visible:   true,
				Opacity:   1.0,
				Bounds:    result.MapLibreBounds,
			},
		}
		_ = s.repo.SetLayers(sessionID, datasetID, layers)
	}

	// Center MapProject camera on georeferenced dataset if project exists
	if proj, err := s.repo.GetMapProject(sessionID, datasetID); err == nil && proj != nil && proj.Configuration != nil {
		if len(result.MapLibreBounds) == 4 {
			centerLng := (result.MapLibreBounds[0] + result.MapLibreBounds[2]) / 2.0
			centerLat := (result.MapLibreBounds[1] + result.MapLibreBounds[3]) / 2.0
			proj.Configuration["camera"] = map[string]interface{}{
				"longitude": centerLng,
				"latitude":  centerLat,
				"center":    []float64{centerLng, centerLat},
				"zoom":      15.0,
				"bearing":   0.0,
				"pitch":     0.0,
			}
			_ = s.repo.SaveMapProject(sessionID, proj)
		}
	}

	return result, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
