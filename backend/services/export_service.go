package services

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

type ExportService struct {
	repo    *repository.SessionRepository
	storage *storage.StorageManager
}

func NewExportService(repo *repository.SessionRepository, sm *storage.StorageManager) *ExportService {
	return &ExportService{
		repo:    repo,
		storage: sm,
	}
}

func safeStem(name string) string {
	reg := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	clean := reg.ReplaceAllString(name, "_")
	clean = strings.Trim(clean, "._-")
	if len(clean) > 80 {
		clean = clean[:80]
	}
	if clean == "" {
		return "map_project"
	}
	return clean
}

// GenerateExport creates an export archive or file
func (s *ExportService) GenerateExport(sessionID, datasetID, format string) (string, string, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return "", "", err
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "mtmap"
	}

	exportDir := filepath.Join(s.storage.GetDatasetDir(sessionID, datasetID), "exports")
	_ = os.MkdirAll(exportDir, 0755)

	switch format {
	case "mtmap", "project", "zip":
		return s.exportMTMAP(sessionID, dataset, exportDir)
	case "png", "jpeg", "jpg":
		return s.exportRasterWithWorldFile(dataset, exportDir)
	case "geotiff", "tif", "tiff":
		return s.exportGeoTIFF(dataset)
	case "mbtiles":
		return s.exportMBTiles(dataset)
	default:
		return "", "", fmt.Errorf("UNSUPPORTED_FORMAT: Export format '%s' is not supported. Supported: mtmap, png, jpeg, geotiff, mbtiles", format)
	}
}

func (s *ExportService) exportMTMAP(sessionID string, dataset *models.Dataset, exportDir string) (string, string, error) {
	project, _ := s.repo.GetMapProject(sessionID, dataset.ID)
	projectName := dataset.Filename
	var projectConfig map[string]interface{}
	if project != nil {
		projectName = project.Name
		projectConfig = project.Configuration
	}

	points, _ := s.repo.ListControlPoints(sessionID, dataset.ID)

	stem := safeStem(projectName)
	zipPath := filepath.Join(exportDir, stem+".zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating export archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	// Extract bounds
	var boundsList []float64
	if b, ok := dataset.ExtractedMetadata["bounds"].(map[string]float64); ok {
		boundsList, _ = gis.NormalizeBounds(b)
	}

	metadataDoc := map[string]interface{}{
		"name":        projectName,
		"version":     "1.1.0",
		"description": "Exported from MapTiler Go MVP",
		"attribution": `<a href="https://www.maptiler.com/engine/">Rendered with MapTiler</a>`,
		"type":        "overlay",
		"format":      "png",
		"bounds":      boundsList,
		"profile":     "mercator",
	}
	metaBytes, _ := json.MarshalIndent(metadataDoc, "", "  ")
	metaWriter, _ := archive.Create("metadata.json")
	_, _ = metaWriter.Write(metaBytes)

	// mapstudio.json document
	projectDoc := map[string]interface{}{
		"format":  "mapstudio-folder",
		"version": 1,
		"project": map[string]interface{}{
			"name":          projectName,
			"configuration": projectConfig,
		},
		"dataset": map[string]interface{}{
			"filename":            dataset.Filename,
			"content_type":        dataset.ContentType,
			"source_asset":        "assets/source/" + filepath.Base(dataset.StoragePath),
			"georeference_status": dataset.GeoreferenceStatus,
			"extracted_metadata":  dataset.ExtractedMetadata,
			"map_layer":           dataset.MapLayer,
		},
		"control_points": points,
	}
	projBytes, _ := json.MarshalIndent(projectDoc, "", "  ")
	projWriter, _ := archive.Create("mapstudio.json")
	_, _ = projWriter.Write(projBytes)

	// dataset.json document
	dsBytes, _ := json.MarshalIndent(dataset, "", "  ")
	dsWriter, _ := archive.Create("dataset.json")
	_, _ = dsWriter.Write(dsBytes)

	// HTML Viewers
	viewers := generateViewers(projectName, boundsList)
	for name, htmlContent := range viewers {
		vw, _ := archive.Create(name)
		_, _ = vw.Write([]byte(htmlContent))
	}

	// Source Asset
	srcFile, err := os.Open(dataset.StoragePath)
	if err == nil {
		defer srcFile.Close()
		assetWriter, err := archive.Create("assets/source/" + filepath.Base(dataset.StoragePath))
		if err == nil {
			_, _ = io.Copy(assetWriter, srcFile)
		}
	}

	// World file sidecar if present
	baseExt := strings.ToLower(filepath.Ext(dataset.Filename))
	worldExt := ".wld"
	switch baseExt {
	case ".png":
		worldExt = ".pgw"
	case ".jpg", ".jpeg":
		worldExt = ".jgw"
	}
	srcWorldFile := filepath.Join(filepath.Dir(dataset.StoragePath), strings.TrimSuffix(filepath.Base(dataset.Filename), baseExt)+worldExt)
	if fileExists(srcWorldFile) {
		if wf, err := os.Open(srcWorldFile); err == nil {
			defer wf.Close()
			if ww, err := archive.Create("assets/source/" + filepath.Base(srcWorldFile)); err == nil {
				_, _ = io.Copy(ww, wf)
			}
		}
	}

	// Processed transparent raster if present
	transparentPNG := filepath.Join(filepath.Dir(dataset.StoragePath), "processed", "processed_transparent.png")
	if fileExists(transparentPNG) {
		if tf, err := os.Open(transparentPNG); err == nil {
			defer tf.Close()
			if tw, err := archive.Create("processed/processed_transparent.png"); err == nil {
				_, _ = io.Copy(tw, tf)
			}
		}
	}

	return zipPath, stem + ".zip", nil
}

func (s *ExportService) exportRasterWithWorldFile(dataset *models.Dataset, exportDir string) (string, string, error) {
	stem := safeStem(dataset.Filename)
	zipPath := filepath.Join(exportDir, stem+"_georeferenced.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed to create export zip: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	// Add image
	srcFile, err := os.Open(dataset.StoragePath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Source image not found: %w", err)
	}
	defer srcFile.Close()

	imgWriter, _ := archive.Create(filepath.Base(dataset.StoragePath))
	_, _ = io.Copy(imgWriter, srcFile)

	// Add world file if georeferenced
	if transform, ok := dataset.ExtractedMetadata["transform"].([]interface{}); ok && len(transform) == 6 {
		var t []float64
		for _, v := range transform {
			if f, ok := v.(float64); ok {
				t = append(t, f)
			}
		}
		if len(t) == 6 {
			baseExt := strings.ToLower(filepath.Ext(dataset.Filename))
			worldExt := ".wld"
			switch baseExt {
			case ".png":
				worldExt = ".pgw"
			case ".jpg", ".jpeg":
				worldExt = ".jgw"
			}
			worldName := strings.TrimSuffix(filepath.Base(dataset.Filename), baseExt) + worldExt
			content := fmt.Sprintf("%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n",
				t[0], t[3], t[1], t[4], t[2], t[5],
			)
			ww, _ := archive.Create(worldName)
			_, _ = ww.Write([]byte(content))
		}
	}

	return zipPath, stem + "_georeferenced.zip", nil
}

func (s *ExportService) exportGeoTIFF(dataset *models.Dataset) (string, string, error) {
	if dataset.GeoreferencedPath != "" && strings.HasSuffix(strings.ToLower(dataset.GeoreferencedPath), ".tif") {
		if fileExists(dataset.GeoreferencedPath) {
			return dataset.GeoreferencedPath, filepath.Base(dataset.GeoreferencedPath), nil
		}
	}

	gdal := gis.DetectGDAL()
	if !gdal.Available {
		return "", "", fmt.Errorf("GIS_DEPENDENCY_MISSING: GeoTIFF generation requires GDAL CLI tools (gdal_translate) which are not installed or in PATH")
	}

	return "", "", fmt.Errorf("GEOREFERENCING_REQUIRED: Dataset must be georeferenced before exporting to GeoTIFF")
}

func (s *ExportService) exportMBTiles(dataset *models.Dataset) (string, string, error) {
	if dataset.TilePath != "" && strings.HasSuffix(strings.ToLower(dataset.TilePath), ".mbtiles") {
		if fileExists(dataset.TilePath) {
			return dataset.TilePath, filepath.Base(dataset.TilePath), nil
		}
	}
	return "", "", fmt.Errorf("GIS_DEPENDENCY_MISSING: MBTiles export requires pre-rendered tiles or MapTiler raster tiling tool")
}

func generateViewers(title string, bounds []float64) map[string]string {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(bounds) == 4 {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}

	leafletHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <title>%s</title>
  <meta charset="utf-8" />
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.7.1/leaflet.css" />
  <script src="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.7.1/leaflet.js"></script>
  <style>body { margin:0; padding:0; } #map { position:absolute; top:0; bottom:0; width:100%%; }</style>
</head>
<body>
  <div id="map"></div>
  <script>
    var bounds = [[%.6f, %.6f], [%.6f, %.6f]];
    var map = L.map('map').fitBounds(bounds);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: 'Map data &copy; OpenStreetMap'
    }).addTo(map);
  </script>
</body>
</html>`, title, south, west, north, east)

	indexHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body style="font-family:sans-serif; padding:40px;">
  <h1>%s</h1>
  <p>Available Map Viewers:</p>
  <ul>
    <li><a href="leaflet.html">Leaflet Viewer</a></li>
  </ul>
</body>
</html>`, title, title)

	return map[string]string{
		"index.html":   indexHTML,
		"leaflet.html": leafletHTML,
	}
}
