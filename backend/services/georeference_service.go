package services

import (
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
	return s.repo.ListControlPoints(sessionID, datasetID)
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

	dataset.GeoreferenceStatus = "completed"
	_ = s.repo.UpdateDataset(sessionID, dataset)

	// Update layers
	layers, _ := s.repo.ListLayers(sessionID, datasetID)
	if len(layers) > 0 {
		layers[0].Bounds = result.MapLibreBounds
		_ = s.repo.SetLayers(sessionID, datasetID, layers)
	}

	return result, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
