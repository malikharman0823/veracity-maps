package services

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	case "mtmap", "project", "zip", "project_zip", "complete_project":
		return s.exportMTMAP(sessionID, dataset, exportDir)
	case "png", "jpeg", "jpg":
		return s.exportRasterWithWorldFile(dataset, exportDir)
	case "geotiff", "tif", "tiff":
		return s.exportGeoTIFF(dataset)
	case "mbtiles":
		return s.exportMBTilesFile(sessionID, dataset, exportDir)
	case "base_layer", "basemap":
		return s.exportBaseLayerFile(sessionID, dataset, exportDir)
	default:
		return "", "", fmt.Errorf("UNSUPPORTED_FORMAT: Export format '%s' is not supported. Supported: project_zip, mbtiles, base_layer, mtmap, png, jpeg, geotiff", format)
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

	// Extract bounds robustly
	boundsList := extractDatasetBounds(dataset)

	// Check if we need to render composite raster with markers
	tileSourceImage := dataset.StoragePath
	transparentPNGCheck := filepath.Join(filepath.Dir(dataset.StoragePath), "processed", "processed_transparent.png")
	if fileExists(transparentPNGCheck) {
		tileSourceImage = transparentPNGCheck
	}

	// Load icon points and control points
	iconPoints, _ := s.repo.ListIconPoints(sessionID, dataset.ID, "")

	// Render composite raster with markers burned onto a copy of the background raster
	compositePath, _ := createCompositeRasterWithMarkers(tileSourceImage, points, iconPoints, boundsList, sessionID)
	tileSourceImage = compositePath

	// Render XYZ tiles if georeferenced or raster exists
	var generatedZooms []int
	if len(boundsList) == 4 && (dataset.GeoreferenceStatus == "georeferenced" || dataset.GeoreferenceStatus == "completed" || fileExists(tileSourceImage)) {
		zooms, tileErr := gis.GenerateRasterTiles(archive, tileSourceImage, boundsList)
		if tileErr == nil && len(zooms) > 0 {
			generatedZooms = zooms
		}
	}

	minZoom, maxZoom := 0, 0
	if len(generatedZooms) > 0 {
		minZoom = generatedZooms[0]
		maxZoom = generatedZooms[len(generatedZooms)-1]
	} else if len(boundsList) == 4 {
		minZoom, maxZoom = 15, 17
	}

	// 1. Write original source raster under assets/source/ if available
	var sourceAssetRelPath string
	if dataset.StoragePath != "" && fileExists(dataset.StoragePath) {
		cleanStem := safeStem(strings.TrimSuffix(filepath.Base(dataset.Filename), filepath.Ext(dataset.Filename)))
		cleanExt := strings.ToLower(filepath.Ext(dataset.StoragePath))
		sourceAssetRelPath = "assets/source/" + cleanStem + cleanExt

		if srcFile, err := os.Open(dataset.StoragePath); err == nil {
			if assetWriter, err := archive.Create(sourceAssetRelPath); err == nil {
				_, _ = io.Copy(assetWriter, srcFile)
			}
			_ = srcFile.Close()
		}
	}

	// Bundle referenced icons into the ZIP archive
	bundledIcons := bundleReferencedIcons(archive, iconPoints, sessionID)

	// Resolve zoom configuration from metadata if present
	var zc *ZoomConfiguration
	if dataset.ExtractedMetadata != nil {
		var izPtr, minZPtr, maxZPtr *int
		if iz, ok := dataset.ExtractedMetadata["initial_zoom"].(int); ok && iz > 0 {
			izPtr = &iz
		} else if fz, ok := dataset.ExtractedMetadata["initial_zoom"].(float64); ok && fz > 0 {
			izInt := int(fz)
			izPtr = &izInt
		}
		if minZ, ok := dataset.ExtractedMetadata["minzoom"].(int); ok && minZ >= 0 {
			minZPtr = &minZ
		} else if minZ, ok := dataset.ExtractedMetadata["minzoom"].(float64); ok && minZ >= 0 {
			mInt := int(minZ)
			minZPtr = &mInt
		}
		if maxZ, ok := dataset.ExtractedMetadata["maxzoom"].(int); ok && maxZ >= 0 {
			maxZPtr = &maxZ
		} else if maxZ, ok := dataset.ExtractedMetadata["maxzoom"].(float64); ok && maxZ >= 0 {
			mInt := int(maxZ)
			maxZPtr = &mInt
		}
		preset, _ := dataset.ExtractedMetadata["zoom_preset"].(string)
		if izPtr != nil || minZPtr != nil || maxZPtr != nil || preset != "" {
			zc = &ZoomConfiguration{
				InitialZoom: izPtr,
				MinZoom:     minZPtr,
				MaxZoom:     maxZPtr,
				Preset:      preset,
			}
		}
	}
	initZoom, camMinZoom, camMaxZoom, zoomPreset := resolveCameraZoomSettings(zc, minZoom, maxZoom, boundsList)

	// 2. Write metadata.json
	metadataDoc := map[string]interface{}{
		"name":            projectName,
		"version":         "1.1.0",
		"description":     "",
		"attribution":     "<a href=\"https://www.maptiler.com/engine/\">Rendered with MapTiler Engine</a>",
		"type":            "overlay",
		"format":          "png",
		"minzoom":         strconv.Itoa(camMinZoom),
		"maxzoom":         strconv.Itoa(camMaxZoom),
		"raster_minzoom":  strconv.Itoa(minZoom),
		"raster_maxzoom":  strconv.Itoa(maxZoom),
		"initial_zoom":    initZoom,
		"zoom_preset":     zoomPreset,
		"scale":           "1.000000",
		"bounds":          boundsList,
		"profile":         "mercator",
		"available_zooms": generatedZooms,
	}
	if len(points) > 0 {
		metadataDoc["control_points"] = points
	}
	if len(iconPoints) > 0 {
		metadataDoc["icon_points"] = iconPoints
	}
	if projectConfig != nil {
		metadataDoc["project_configuration"] = projectConfig
	}

	metaBytes, _ := json.MarshalIndent(metadataDoc, "", "  ")
	metaWriter, _ := archive.Create("metadata.json")
	_, _ = metaWriter.Write(metaBytes)

	// 3. Write mapstudio.json
	mapStudioDoc := generateMapStudioJSON(dataset, project, points, iconPoints, sourceAssetRelPath)
	mapStudioDoc["initial_zoom"] = initZoom
	mapStudioDoc["minzoom"] = camMinZoom
	mapStudioDoc["maxzoom"] = camMaxZoom
	mapStudioDoc["zoom_preset"] = zoomPreset
	msBytes, _ := json.MarshalIndent(mapStudioDoc, "", "  ")
	msWriter, _ := archive.Create("mapstudio.json")
	_, _ = msWriter.Write(msBytes)

	// 4. Resolve saved opacity
	opacityVal := 1.0
	if projectConfig != nil {
		if opRaw, ok := projectConfig["opacity"]; ok {
			if opFloat, ok := opRaw.(float64); ok && opFloat >= 0 && opFloat <= 1 {
				opacityVal = opFloat
			}
		}
	}

	// 5. Write style.json (MapLibre GL JS Style Spec version 8)
	styleBytes, _ := generateMapLibreStyleJSON(projectName, boundsList, minZoom, maxZoom, opacityVal)
	styleWriter, _ := archive.Create("style.json")
	_, _ = styleWriter.Write(styleBytes)

	// 6. Write README.txt
	readmeContent := generateReadmeTxt(projectName, boundsList, minZoom, maxZoom, metadataDoc["attribution"].(string))
	readmeWriter, _ := archive.Create("README.txt")
	_, _ = readmeWriter.Write([]byte(readmeContent))

	// 7. Write MapLibre Viewers (viewer.html and index.html)
	viewers := generateViewers(ViewscapeViewerParams{
		Title:            projectName,
		Bounds:           boundsList,
		RasterMinZoom:    minZoom,
		RasterMaxZoom:    maxZoom,
		InitialZoom:      initZoom,
		MinZoom:          camMinZoom,
		MaxZoom:          camMaxZoom,
		Preset:           zoomPreset,
		Opacity:          opacityVal,
		ControlPoints:    points,
		IconPoints:       iconPoints,
		BundledIconPaths: bundledIcons,
	})
	for name, htmlContent := range viewers {
		vw, _ := archive.Create(name)
		_, _ = vw.Write([]byte(htmlContent))
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

func (s *ExportService) exportMBTilesFile(sessionID string, dataset *models.Dataset, exportDir string) (string, string, error) {
	// 1. If an actual .mbtiles file already exists for this dataset, export it directly
	if dataset.TilePath != "" && strings.HasSuffix(strings.ToLower(dataset.TilePath), ".mbtiles") && fileExists(dataset.TilePath) {
		return dataset.TilePath, filepath.Base(dataset.TilePath), nil
	}

	// 2. Otherwise generate a standards-compliant portable map tile package (.mbtiles.zip)
	boundsList := extractDatasetBounds(dataset)
	if len(boundsList) < 4 {
		return "", "", fmt.Errorf("GEOREFERENCING_REQUIRED: Dataset must be georeferenced to export map tiles")
	}

	stem := safeStem(dataset.Filename)
	zipPath := filepath.Join(exportDir, stem+".mbtiles.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating map tile archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	tileSourceImage := dataset.StoragePath
	transparentPNGCheck := filepath.Join(filepath.Dir(dataset.StoragePath), "processed", "processed_transparent.png")
	if fileExists(transparentPNGCheck) {
		tileSourceImage = transparentPNGCheck
	}

	zooms, tileErr := gis.GenerateRasterTiles(archive, tileSourceImage, boundsList)
	if tileErr != nil {
		return "", "", fmt.Errorf("TILE_GENERATION_FAILED: Failed generating raster tiles: %w", tileErr)
	}

	minZ, maxZ := 0, 0
	if len(zooms) > 0 {
		minZ = zooms[0]
		maxZ = zooms[len(zooms)-1]
	}

	center := []float64{(boundsList[0] + boundsList[2]) / 2.0, (boundsList[1] + boundsList[3]) / 2.0}
	metadataDoc := map[string]interface{}{
		"name":        dataset.Filename,
		"format":      "png",
		"type":        "overlay",
		"bounds":      boundsList,
		"center":      []interface{}{center[0], center[1], minZ},
		"minzoom":     minZ,
		"maxzoom":     maxZ,
		"attribution": "Veracity Maps",
		"description": "Portable Map Tile Package (XYZ standard pyramid)",
		"scheme":      "xyz",
		"limitation":  "MBTiles does not automatically preserve the complete project, editable control points, custom icon assets, or all original styling.",
	}
	metaBytes, _ := json.MarshalIndent(metadataDoc, "", "  ")
	mw, _ := archive.Create("metadata.json")
	_, _ = mw.Write(metaBytes)

	// Add README explaining tile layout
	readme := fmt.Sprintf(`Veracity Maps — Portable Map Tile Package
=========================================
Dataset:     %s
Bounds:      [%.6f, %.6f, %.6f, %.6f]
Zoom Range:  %d to %d
Format:      Standard XYZ raster tile pyramid ({z}/{x}/{y}.png)

Compatibility:
• Compatible with MapLibre GL, Leaflet, OpenLayers, and QGIS tile directory loaders.
• Notice: MBTiles does not automatically preserve the complete project, editable control points, custom icon assets, or all original styling.
`, dataset.Filename, boundsList[0], boundsList[1], boundsList[2], boundsList[3], minZ, maxZ)
	rw, _ := archive.Create("README.txt")
	_, _ = rw.Write([]byte(readme))

	return zipPath, stem + ".mbtiles.zip", nil
}

func (s *ExportService) exportBaseLayerFile(sessionID string, dataset *models.Dataset, exportDir string) (string, string, error) {
	boundsList := extractDatasetBounds(dataset)
	if len(boundsList) < 4 {
		// Fallback to world bounds if dataset has no spatial extent
		boundsList = []float64{-180.0, -85.0511, 180.0, 85.0511}
	}

	zipPath := filepath.Join(exportDir, "veracity-base-map.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating base map archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	// Package genuine base map tiles
	minZoom := 14
	maxZoom := 18
	if boundsList[0] == -180.0 && boundsList[2] == 180.0 {
		minZoom = 0
		maxZoom = 4
	}

	packagedCount, _ := gis.PackageBasemapTiles(archive, boundsList, minZoom, maxZoom)

	center := []float64{(boundsList[0] + boundsList[2]) / 2.0, (boundsList[1] + boundsList[3]) / 2.0}
	metadataDoc := map[string]interface{}{
		"name":           "veracity-base-map",
		"format":         "png",
		"type":           "baselayer",
		"bounds":         boundsList,
		"center":         []interface{}{center[0], center[1], minZoom},
		"minzoom":        minZoom,
		"maxzoom":        maxZoom,
		"attribution":    "© OpenStreetMap contributors",
		"tile_count":     packagedCount,
		"tile_template":  "./basemap/{z}/{x}/{y}.png",
		"reusable_scope": "Independent offline base layer for compatible Viewscape applications.",
	}
	metaBytes, _ := json.MarshalIndent(metadataDoc, "", "  ")
	mw, _ := archive.Create("metadata.json")
	_, _ = mw.Write(metaBytes)

	// Standalone style.json for MapLibre Viewscapes
	styleDoc := MapLibreStyle{
		Version: 8,
		Name:    "Veracity Offline Base Map",
		Sources: map[string]interface{}{
			"offline-basemap": map[string]interface{}{
				"type":        "raster",
				"tiles":       []string{"./basemap/{z}/{x}/{y}.png"},
				"tileSize":    256,
				"minzoom":     minZoom,
				"maxzoom":     maxZoom,
				"attribution": "© OpenStreetMap contributors",
			},
		},
		Layers: []map[string]interface{}{
			{
				"id":     "offline-basemap-layer",
				"type":   "raster",
				"source": "offline-basemap",
			},
		},
	}
	styleBytes, _ := json.MarshalIndent(styleDoc, "", "  ")
	sw, _ := archive.Create("style.json")
	_, _ = sw.Write(styleBytes)

	readme := fmt.Sprintf(`Veracity Maps — Reusable Offline Base Layer
============================================
Layer Name:  veracity-base-map
Type:        Tile-based raster base map
Bounds:      [%.6f, %.6f, %.6f, %.6f]
Zoom Range:  %d to %d
Attribution: © OpenStreetMap contributors

Usage in Offline Viewscape:
1. Extract or reference this package into your offline GIS viewer.
2. The standalone MapLibre style is available in "style.json".
3. Standalone tiles are located under "basemap/{z}/{x}/{y}.png".
`, boundsList[0], boundsList[1], boundsList[2], boundsList[3], minZoom, maxZoom)
	rw, _ := archive.Create("README.txt")
	_, _ = rw.Write([]byte(readme))

	return zipPath, "veracity-base-map.zip", nil
}

// MapLibreStyle defines a MapLibre GL JS Style Specification (version 8) document.
type MapLibreStyle struct {
	Version  int                      `json:"version"`
	Name     string                   `json:"name"`
	Metadata map[string]interface{}   `json:"metadata,omitempty"`
	Sources  map[string]interface{}   `json:"sources"`
	Layers   []map[string]interface{} `json:"layers"`
}

func extractDatasetBounds(dataset *models.Dataset) []float64 {
	if dataset == nil || dataset.ExtractedMetadata == nil {
		return nil
	}
	if b, ok := dataset.ExtractedMetadata["bounds"].(map[string]float64); ok {
		if norm, err := gis.NormalizeBounds(b); err == nil && len(norm) == 4 {
			return norm
		}
	}
	if bMap, ok := dataset.ExtractedMetadata["bounds"].(map[string]interface{}); ok {
		m := make(map[string]float64)
		for k, v := range bMap {
			if f, ok := v.(float64); ok {
				m[k] = f
			}
		}
		if norm, err := gis.NormalizeBounds(m); err == nil && len(norm) == 4 {
			return norm
		}
	}
	if bList, ok := dataset.ExtractedMetadata["bounds"].([]float64); ok && len(bList) == 4 {
		return bList
	}
	if bList, ok := dataset.ExtractedMetadata["bounds"].([]interface{}); ok && len(bList) == 4 {
		var res []float64
		for _, item := range bList {
			if f, ok := item.(float64); ok {
				res = append(res, f)
			}
		}
		if len(res) == 4 {
			return res
		}
	}
	if dataset.MapLayer != nil {
		if b, ok := dataset.MapLayer["bounds"].(map[string]float64); ok {
			if norm, err := gis.NormalizeBounds(b); err == nil && len(norm) == 4 {
				return norm
			}
		}
		if bMap, ok := dataset.MapLayer["bounds"].(map[string]interface{}); ok {
			m := make(map[string]float64)
			for k, v := range bMap {
				if f, ok := v.(float64); ok {
					m[k] = f
				}
			}
			if norm, err := gis.NormalizeBounds(m); err == nil && len(norm) == 4 {
				return norm
			}
		}
	}
	return nil
}

func generateMapLibreStyleJSON(title string, bounds []float64, minZoom, maxZoom int, opacity float64) ([]byte, error) {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(bounds) == 4 {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}
	if opacity <= 0 || opacity > 1 {
		opacity = 1.0
	}

	style := MapLibreStyle{
		Version: 8,
		Name:    title,
		Metadata: map[string]interface{}{
			"maputnik:renderer": "mlgljs",
		},
		Sources: map[string]interface{}{
			"osm": map[string]interface{}{
				"type":        "raster",
				"tiles":       []string{"https://tile.openstreetmap.org/{z}/{x}/{y}.png"},
				"tileSize":    256,
				"attribution": "&copy; <a href=\"https://www.openstreetmap.org/copyright\" target=\"_blank\">OpenStreetMap</a> contributors",
				"maxzoom":     19,
			},
			"overlay": map[string]interface{}{
				"type":        "raster",
				"tiles":       []string{"./{z}/{x}/{y}.png"},
				"tileSize":    256,
				"minzoom":     minZoom,
				"maxzoom":     maxZoom,
				"bounds":      []float64{west, south, east, north},
				"attribution": "<a href=\"https://www.maptiler.com/engine/\" target=\"_blank\">Rendered with MapTiler Engine</a>",
			},
		},
		Layers: []map[string]interface{}{
			{
				"id":      "osm-tiles",
				"type":    "raster",
				"source":  "osm",
				"minzoom": 0,
				"maxzoom": 19,
			},
			{
				"id":      "overlay-tiles",
				"type":    "raster",
				"source":  "overlay",
				"minzoom": minZoom,
				"maxzoom": maxZoom,
				"paint": map[string]interface{}{
					"raster-opacity": opacity,
				},
			},
		},
	}

	return json.MarshalIndent(style, "", "  ")
}

func generateMapStudioJSON(dataset *models.Dataset, project *models.MapProject, controlPoints []*models.ControlPoint, iconPoints []*models.IconPoint, sourceAssetRelPath string) map[string]interface{} {
	projectName := dataset.Filename
	var projectConfig map[string]interface{}
	if project != nil {
		projectName = project.Name
		projectConfig = project.Configuration
	}

	var formattedCPs []map[string]interface{}
	for _, p := range controlPoints {
		formattedCPs = append(formattedCPs, map[string]interface{}{
			"id":        p.ID,
			"name":      p.ID,
			"image_x":   p.ImageX,
			"image_y":   p.ImageY,
			"longitude": p.Longitude,
			"latitude":  p.Latitude,
		})
	}

	var formattedIPs []map[string]interface{}
	for _, p := range iconPoints {
		name := p.Name
		if name == "" {
			name = p.Description
		}
		formattedIPs = append(formattedIPs, map[string]interface{}{
			"id":          p.ID,
			"name":        name,
			"icon_id":     p.IconID,
			"icon_key":    p.IconID,
			"icon_path":   p.IconPath,
			"description": p.Description,
			"scope":       p.Scope,
			"floor_level": p.FloorLevel,
			"floor_label": p.FloorLabel,
			"longitude":   p.Longitude,
			"latitude":    p.Latitude,
			"properties":  p.Properties,
		})
	}

	var sourceAssetVal interface{}
	if sourceAssetRelPath != "" {
		sourceAssetVal = sourceAssetRelPath
	}

	return map[string]interface{}{
		"format":  "mapstudio-folder",
		"version": 1,
		"project": map[string]interface{}{
			"name":          projectName,
			"configuration": projectConfig,
		},
		"dataset": map[string]interface{}{
			"filename":            dataset.Filename,
			"content_type":        dataset.ContentType,
			"source_asset":        sourceAssetVal,
			"georeference_status": dataset.GeoreferenceStatus,
			"extracted_metadata":  dataset.ExtractedMetadata,
			"map_layer":           dataset.MapLayer,
		},
		"control_points": formattedCPs,
		"icon_points":    formattedIPs,
	}
}

func generateReadmeTxt(title string, bounds []float64, minZoom, maxZoom int, attribution string) string {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(bounds) == 4 {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}

	return fmt.Sprintf(`================================================================================
Veracity Maps - MapTiler-Compatible MapLibre Tile Package
================================================================================

Project:     %s
Format:      MapLibre GL JS v8 Raster Tile Archive
Zoom Range:  %d - %d
Bounds:      West: %.8f, South: %.8f, East: %.8f, North: %.8f
Projection:  EPSG:3857 (Tiles) / EPSG:4326 (WGS84 Coordinates)

Package Contents:
-----------------
- viewer.html     : Interactive MapLibre GL JS web viewer
- index.html      : Default root entry point for HTTP servers (identical to viewer.html)
- style.json      : MapLibre GL JS Style Specification (version 8) document
- metadata.json   : Standard MapTiler raster tile metadata
- mapstudio.json  : Veracity Maps / MapStudio portable project configuration
- assets/source/  : Original uncompressed source raster (when available)
- {z}/{x}/{y}.png : Standard XYZ raster tile pyramid

How to View Locally:
-------------------
Because web browsers restrict local XMLHttpRequest/Fetch (CORS) when opening 
files directly via file://, please serve this directory using a local HTTP server:

  Option 1 (Python):
    python -m http.server 8000
    Open: http://localhost:8000/viewer.html

  Option 2 (Node.js):
    npx serve .
    Open the printed local URL

  Option 3 (VS Code):
    Right click viewer.html -> "Open with Live Server"

Basemap & Offline Notice:
-------------------------
- Raster overlay tiles ({z}/{x}/{y}.png) are self-contained and run 100%%%% offline.
- The geographic basemap loads OpenStreetMap raster tiles (https://tile.openstreetmap.org/).
  Viewing the basemap requires an active internet connection.

Attribution:
------------
%s
Basemap: (C) OpenStreetMap contributors
`, title, minZoom, maxZoom, west, south, east, north, attribution)
}

type ViewscapeViewerParams struct {
	Title            string
	Bounds           []float64
	RasterMinZoom    int
	RasterMaxZoom    int
	InitialZoom      int
	MinZoom          int // Camera min zoom
	MaxZoom          int // Camera max zoom
	Preset           string
	Opacity          float64
	ControlPoints    []*models.ControlPoint
	IconPoints       []*models.IconPoint
	BundledIconPaths map[string]string // iconKey -> relative path in zip
}

var PresetToZoom = map[string]int{
	"house":          21,
	"building":       20,
	"street":         19,
	"neighborhood":   18,
	"local":          17,
	"district":       16,
	"city":           15,
	"large_city":     14,
	"regional":       13,
	"wide_regional":  12,
	"state":          11,
	"large_regional": 10,
	"country":        5,
	"world":          1,
}

var ZoomLevelDescriptions = map[int]string{
	21: "House level",
	20: "Building level",
	19: "Street level",
	18: "Neighborhood level",
	17: "Local area",
	16: "District level",
	15: "City level",
	14: "Large city area",
	13: "Regional level",
	12: "Wide regional view",
	11: "State-level overview",
	10: "Large regional overview",
	5:  "Country-level overview",
	1:  "World overview",
}

func resolveCameraZoomSettings(zc *ZoomConfiguration, rasterMinZoom, rasterMaxZoom int, _ []float64) (initZoom, minZoom, maxZoom int, preset string) {
	minZoom = 0
	if rasterMinZoom > 4 {
		minZoom = rasterMinZoom - 4
	}
	maxZoom = 22
	if rasterMaxZoom > 0 {
		maxZoom = rasterMaxZoom + 4
		if maxZoom > 24 {
			maxZoom = 24
		}
	}
	initZoom = 16
	if rasterMinZoom > 0 {
		initZoom = rasterMinZoom + 1
		if initZoom > rasterMaxZoom {
			initZoom = rasterMaxZoom
		}
	}

	if zc != nil {
		if zc.Preset != "" {
			preset = strings.ToLower(strings.TrimSpace(zc.Preset))
			if pz, ok := PresetToZoom[preset]; ok {
				initZoom = pz
			}
		}
		if zc.MinZoom != nil {
			minZoom = *zc.MinZoom
		}
		if zc.MaxZoom != nil {
			maxZoom = *zc.MaxZoom
		}
		if zc.InitialZoom != nil {
			initZoom = *zc.InitialZoom
		}
	}

	if minZoom < 0 {
		minZoom = 0
	}
	if minZoom > 24 {
		minZoom = 24
	}
	if maxZoom < 0 {
		maxZoom = 0
	}
	if maxZoom > 24 {
		maxZoom = 24
	}
	if minZoom > maxZoom {
		minZoom, maxZoom = maxZoom, minZoom
	}
	if initZoom < minZoom {
		initZoom = minZoom
	}
	if initZoom > maxZoom {
		initZoom = maxZoom
	}

	return initZoom, minZoom, maxZoom, preset
}

func bundleReferencedIcons(archive *zip.Writer, iconPoints []*models.IconPoint, sessionID string) map[string]string {
	bundled := make(map[string]string)
	if len(iconPoints) == 0 {
		return bundled
	}

	searchDirs := []string{
		filepath.Join("frontend", "public", "icons"),
		filepath.Join("..", "frontend", "public", "icons"),
		filepath.Join("..", "..", "frontend", "public", "icons"),
	}
	if sessionID != "" {
		searchDirs = append(searchDirs,
			filepath.Join("temp", "sessions", sessionID, "icons"),
			filepath.Join("temp", "sessions", sessionID, "uploads"),
			filepath.Join("temp", "sessions", sessionID),
		)
	}

	seenEntries := make(map[string]bool)

	for _, pt := range iconPoints {
		if pt.IconID == "" && pt.IconPath == "" {
			continue
		}
		rawKey := pt.IconID
		if rawKey == "" {
			rawKey = filepath.Base(pt.IconPath)
		}
		cleanKey := filepath.Base(rawKey)
		ext := filepath.Ext(cleanKey)
		var candidates []string
		if ext != "" {
			candidates = append(candidates, cleanKey)
		} else {
			for _, e := range []string{".png", ".svg", ".webp", ".ico", ".jpg"} {
				candidates = append(candidates, cleanKey+e)
			}
		}

		var foundPath string
		var foundName string
		if pt.IconPath != "" {
			if fi, err := os.Stat(pt.IconPath); err == nil && !fi.IsDir() {
				foundPath = pt.IconPath
				foundName = filepath.Base(pt.IconPath)
			}
		}

		if foundPath == "" {
			for _, dir := range searchDirs {
				for _, cand := range candidates {
					p := filepath.Join(dir, cand)
					if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
						foundPath = p
						foundName = cand
						break
					}
				}
				if foundPath != "" {
					break
				}
			}
		}

		cleanKeyNorm := strings.TrimSuffix(cleanKey, ext)
		if foundPath != "" {
			relPath := "assets/icons/" + foundName
			if !seenEntries[relPath] {
				if f, err := os.Open(foundPath); err == nil {
					if w, err := archive.Create(relPath); err == nil {
						_, _ = io.Copy(w, f)
						seenEntries[relPath] = true
					}
					_ = f.Close()
				}
			}
			bundled[cleanKey] = relPath
			bundled[cleanKeyNorm] = relPath
			bundled[rawKey] = relPath
		}
	}
	return bundled
}

func generateMapLibreViewerHTML(params ViewscapeViewerParams, styleJSON string) string {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(params.Bounds) == 4 {
		west, south, east, north = params.Bounds[0], params.Bounds[1], params.Bounds[2], params.Bounds[3]
	}
	opacity := params.Opacity
	if opacity <= 0 || opacity > 1 {
		opacity = 1.0
	}
	escapedTitle := html.EscapeString(params.Title)
	if escapedTitle == "" {
		escapedTitle = "Veracity Maps"
	}
	presetName := params.Preset
	if presetName == "" {
		if desc, ok := ZoomLevelDescriptions[params.InitialZoom]; ok {
			presetName = desc
		} else {
			presetName = "Custom"
		}
	}

	// Prepare Control Points GeoJSON
	cpFeatures := make([]map[string]interface{}, 0)
	for _, cp := range params.ControlPoints {
		cpFeatures = append(cpFeatures, map[string]interface{}{
			"type": "Feature",
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{cp.Longitude, cp.Latitude},
			},
			"properties": map[string]interface{}{
				"id":        cp.ID,
				"name":      cp.ID,
				"image_x":   cp.ImageX,
				"image_y":   cp.ImageY,
				"longitude": cp.Longitude,
				"latitude":  cp.Latitude,
			},
		})
	}
	cpDoc := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": cpFeatures,
	}
	cpJSON, _ := json.Marshal(cpDoc)

	// Prepare Icon Points GeoJSON
	ipFeatures := make([]map[string]interface{}, 0)
	for _, ip := range params.IconPoints {
		cleanKey := filepath.Base(ip.IconID)
		if cleanKey == "" {
			cleanKey = filepath.Base(ip.IconPath)
		}
		cleanKeyNorm := strings.TrimSuffix(cleanKey, filepath.Ext(cleanKey))
		name := ip.Name
		if name == "" {
			name = ip.Description
		}
		if name == "" {
			name = "Marker"
		}
		ipFeatures = append(ipFeatures, map[string]interface{}{
			"type": "Feature",
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{ip.Longitude, ip.Latitude},
			},
			"properties": map[string]interface{}{
				"id":          ip.ID,
				"name":        name,
				"icon_key":    cleanKeyNorm,
				"icon_id":     ip.IconID,
				"description": ip.Description,
				"scope":       ip.Scope,
				"floor_level": ip.FloorLevel,
				"floor_label": ip.FloorLabel,
				"longitude":   ip.Longitude,
				"latitude":    ip.Latitude,
			},
		})
	}
	ipDoc := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": ipFeatures,
	}
	ipJSON, _ := json.Marshal(ipDoc)

	bundledJSON, _ := json.Marshal(params.BundledIconPaths)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>%s - Viewscape Map Viewer</title>
  <link rel="stylesheet" href="https://unpkg.com/maplibre-gl@3.6.2/dist/maplibre-gl.css" />
  <script src="https://unpkg.com/maplibre-gl@3.6.2/dist/maplibre-gl.js"></script>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body, html { width: 100%%; height: 100%%; overflow: hidden; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: #0f172a; color: #f8fafc; }
    #map { position: absolute; top: 0; bottom: 0; width: 100%%; height: 100%%; }
    
    .floating-panel {
      position: absolute;
      top: 16px;
      right: 16px;
      width: 330px;
      max-width: calc(100vw - 32px);
      background: rgba(15, 23, 42, 0.90);
      backdrop-filter: blur(14px);
      -webkit-backdrop-filter: blur(14px);
      border: 1px solid rgba(255, 255, 255, 0.12);
      border-radius: 12px;
      padding: 16px;
      box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.5), 0 8px 10px -6px rgba(0, 0, 0, 0.4);
      z-index: 10;
    }
    .panel-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      border-bottom: 1px solid rgba(255, 255, 255, 0.08);
      padding-bottom: 12px;
      margin-bottom: 12px;
    }
    .panel-title {
      font-size: 14px;
      font-weight: 600;
      color: #f1f5f9;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 220px;
    }
    .status-badge {
      font-size: 10px;
      font-weight: 500;
      padding: 2px 8px;
      border-radius: 9999px;
      background: rgba(16, 185, 129, 0.2);
      color: #34d399;
      border: 1px solid rgba(16, 185, 129, 0.3);
      display: flex;
      align-items: center;
      gap: 4px;
    }
    .status-dot {
      width: 6px;
      height: 6px;
      border-radius: 50%%;
      background: #10b981;
    }
    .control-group {
      margin-bottom: 12px;
    }
    .control-label {
      display: flex;
      justify-content: space-between;
      font-size: 12px;
      color: #94a3b8;
      margin-bottom: 6px;
    }
    .control-value {
      font-weight: 600;
      color: #e2e8f0;
      font-variant-numeric: tabular-nums;
    }
    .slider {
      width: 100%%;
      height: 6px;
      border-radius: 3px;
      background: #334155;
      outline: none;
      -webkit-appearance: none;
      cursor: pointer;
    }
    .slider::-webkit-slider-thumb {
      -webkit-appearance: none;
      width: 16px;
      height: 16px;
      border-radius: 50%%;
      background: #38bdf8;
      cursor: pointer;
      box-shadow: 0 0 8px rgba(56, 189, 248, 0.6);
    }
    .checkbox-row {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 12px;
      color: #cbd5e1;
      cursor: pointer;
      user-select: none;
      margin-bottom: 6px;
    }
    .checkbox-row input {
      accent-color: #38bdf8;
      cursor: pointer;
      width: 14px;
      height: 14px;
    }
    .info-grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 8px;
      font-size: 11px;
      background: rgba(0, 0, 0, 0.25);
      border-radius: 6px;
      padding: 8px;
      margin-bottom: 12px;
      border: 1px solid rgba(255, 255, 255, 0.05);
    }
    .info-item {
      display: flex;
      flex-direction: column;
    }
    .info-label {
      color: #64748b;
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }
    .info-val {
      color: #cbd5e1;
      font-weight: 500;
      font-family: monospace;
    }
    .button-row {
      display: flex;
      gap: 8px;
    }
    .btn {
      flex: 1;
      background: #1e293b;
      color: #f1f5f9;
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 6px;
      padding: 6px 12px;
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
      transition: background 0.15s, border-color 0.15s;
    }
    .btn:hover {
      background: #334155;
      border-color: rgba(255, 255, 255, 0.2);
    }
    .coordinates-bar {
      margin-top: 10px;
      font-size: 10px;
      color: #64748b;
      font-family: monospace;
      text-align: center;
    }
    .maplibregl-popup-content {
      background: #1e293b;
      color: #f8fafc;
      border-radius: 8px;
      padding: 10px 14px;
      border: 1px solid rgba(255, 255, 255, 0.15);
      box-shadow: 0 10px 20px rgba(0,0,0,0.5);
    }
    .maplibregl-popup-anchor-top .maplibregl-popup-tip { border-bottom-color: #1e293b; }
    .maplibregl-popup-anchor-bottom .maplibregl-popup-tip { border-top-color: #1e293b; }
    .maplibregl-popup-anchor-left .maplibregl-popup-tip { border-right-color: #1e293b; }
    .maplibregl-popup-anchor-right .maplibregl-popup-tip { border-left-color: #1e293b; }
    .maplibregl-popup-close-button {
      color: #94a3b8;
      font-size: 16px;
      padding: 2px 6px;
    }
    .maplibregl-popup-close-button:hover { color: #f8fafc; }
  </style>
</head>
<body>
  <div id="map"></div>
  
  <div class="floating-panel">
    <div class="panel-header">
      <div class="panel-title" title="%s">%s</div>
      <div class="status-badge">
        <span class="status-dot"></span>
        <span>MapLibre Viewscape</span>
      </div>
    </div>
    
    <div class="control-group">
      <label class="checkbox-row">
        <input type="checkbox" id="visibility-toggle" checked />
        <span>Overlay Layer Visible</span>
      </label>
      <label class="checkbox-row">
        <input type="checkbox" id="control-points-toggle" checked />
        <span>Control Points Visible</span>
      </label>
      <label class="checkbox-row">
        <input type="checkbox" id="icon-points-toggle" checked />
        <span>Icon Points Visible</span>
      </label>
    </div>
    
    <div class="control-group">
      <div class="control-label">
        <span>Raster Opacity</span>
        <span class="control-value" id="opacity-val">%.0f%%%%</span>
      </div>
      <input type="range" class="slider" id="opacity-slider" min="0" max="1" step="0.05" value="%.2f" />
    </div>
    
    <div class="info-grid">
      <div class="info-item">
        <span class="info-label">Current Zoom</span>
        <span class="info-val" id="zoom-val">%d</span>
      </div>
      <div class="info-item">
        <span class="info-label">Camera Limits</span>
        <span class="info-val">%d - %d</span>
      </div>
      <div class="info-item">
        <span class="info-label">Tile Zooms</span>
        <span class="info-val">%d - %d</span>
      </div>
      <div class="info-item">
        <span class="info-label">Initial Preset</span>
        <span class="info-val">%s</span>
      </div>
    </div>
    
    <div class="button-row">
      <button class="btn" id="reset-view-btn">Fit Dataset</button>
    </div>
    
    <div class="coordinates-bar" id="coords-display">
      Hover map for coordinates
    </div>
  </div>

  <script>
    const west = %.8f, south = %.8f, east = %.8f, north = %.8f;
    const rasterMinZoom = %d, rasterMaxZoom = %d;
    const initialZoom = %d;
    const minZoom = %d, maxZoom = %d;
    const initialOpacity = %.2f;
    const bounds = [[west, south], [east, north]];
    const center = [(west + east) / 2.0, (south + north) / 2.0];

    const styleDoc = %s;
    const controlPointsGeoJSON = %s;
    const iconPointsGeoJSON = %s;
    const bundledIcons = %s;

    const map = new maplibregl.Map({
      container: 'map',
      style: styleDoc,
      center: center,
      zoom: initialZoom,
      minZoom: minZoom,
      maxZoom: maxZoom
    });

    map.addControl(new maplibregl.NavigationControl({ visualizePitch: true }), 'top-left');
    map.addControl(new maplibregl.ScaleControl({ unit: 'metric' }), 'bottom-left');

    // Missing image fallback: dynamically generate crisp circular canvas marker so icons never crash
    map.on('styleimagemissing', (e) => {
      const id = e.id;
      const size = 32;
      const canvas = document.createElement('canvas');
      canvas.width = size;
      canvas.height = size;
      const ctx = canvas.getContext('2d');
      
      // Outer colored pin (crisp red dot fallback matching specification)
      ctx.beginPath();
      ctx.arc(size / 2, size / 2, 11, 0, Math.PI * 2);
      ctx.fillStyle = '#dc2626';
      ctx.fill();
      ctx.lineWidth = 2.5;
      ctx.strokeStyle = '#ffffff';
      ctx.stroke();

      // Inner white center
      ctx.beginPath();
      ctx.arc(size / 2, size / 2, 4.5, 0, Math.PI * 2);
      ctx.fillStyle = '#ffffff';
      ctx.fill();

      const imgData = ctx.getImageData(0, 0, size, size);
      if (!map.hasImage(id)) {
        map.addImage(id, imgData, { pixelRatio: 2 });
      }
    });

    // Load bundled icon images
    if (bundledIcons && typeof bundledIcons === 'object') {
      for (const [key, path] of Object.entries(bundledIcons)) {
        map.loadImage(path, (error, image) => {
          if (!error && image) {
            if (!map.hasImage(key)) map.addImage(key, image);
            const dotIdx = key.lastIndexOf('.');
            if (dotIdx > 0) {
              const noExt = key.substring(0, dotIdx);
              if (!map.hasImage(noExt)) map.addImage(noExt, image);
            }
          }
        });
      }
    }

    map.on('load', () => {
      // 1. Add Control Points Source and Layers
      map.addSource('control-points-source', {
        type: 'geojson',
        data: controlPointsGeoJSON
      });

      map.addLayer({
        id: 'control-points-halo',
        type: 'circle',
        source: 'control-points-source',
        paint: {
          'circle-radius': 9,
          'circle-color': '#ffffff',
          'circle-opacity': 0.85
        }
      });

      map.addLayer({
        id: 'control-points-layer',
        type: 'circle',
        source: 'control-points-source',
        paint: {
          'circle-radius': 6,
          'circle-color': '#16a34a',
          'circle-stroke-width': 2,
          'circle-stroke-color': '#ffffff'
        }
      });

      map.addLayer({
        id: 'control-points-labels',
        type: 'symbol',
        source: 'control-points-source',
        layout: {
          'text-field': ['coalesce', ['get', 'name'], ['get', 'id']],
          'text-size': 11,
          'text-offset': [0, 1.3],
          'text-anchor': 'top'
        },
        paint: {
          'text-color': '#f8fafc',
          'text-halo-color': '#0f172a',
          'text-halo-width': 1.5
        }
      });

      // 2. Add Icon Points Source and Layer
      map.addSource('icon-points-source', {
        type: 'geojson',
        data: iconPointsGeoJSON
      });

      map.addLayer({
        id: 'icon-points-layer',
        type: 'symbol',
        source: 'icon-points-source',
        layout: {
          'icon-image': ['get', 'icon_key'],
          'icon-size': 0.8,
          'icon-allow-overlap': true,
          'text-field': ['get', 'name'],
          'text-size': 11,
          'text-offset': [0, 1.4],
          'text-anchor': 'top'
        },
        paint: {
          'text-color': '#f8fafc',
          'text-halo-color': '#0f172a',
          'text-halo-width': 1.5
        }
      });

      // 3. Interactive Popups
      map.on('click', 'control-points-layer', (e) => {
        if (!e.features || e.features.length === 0) return;
        const p = e.features[0].properties;
        const coords = e.features[0].geometry.coordinates;
        new maplibregl.Popup({ closeButton: true })
          .setLngLat(coords)
          .setHTML('<div style="font-size:12px;line-height:1.4;">' +
            '<div style="font-weight:bold;color:#16a34a;margin-bottom:4px;">Control Point: ' + (p.name || p.id) + '</div>' +
            '<div><strong>Coordinates:</strong> ' + Number(p.latitude).toFixed(6) + ', ' + Number(p.longitude).toFixed(6) + '</div>' +
            (p.image_x !== undefined ? '<div><strong>Raster Pixel:</strong> (' + Math.round(p.image_x) + ', ' + Math.round(p.image_y) + ')</div>' : '') +
          '</div>')
          .addTo(map);
      });

      map.on('click', 'icon-points-layer', (e) => {
        if (!e.features || e.features.length === 0) return;
        const p = e.features[0].properties;
        const coords = e.features[0].geometry.coordinates;
        new maplibregl.Popup({ closeButton: true })
          .setLngLat(coords)
          .setHTML('<div style="font-size:12px;line-height:1.4;">' +
            '<div style="font-weight:bold;color:#38bdf8;margin-bottom:4px;">' + (p.name || p.description || 'Icon Point') + '</div>' +
            (p.description && p.name !== p.description ? '<div style="margin-bottom:2px;">' + p.description + '</div>' : '') +
            (p.scope ? '<div><span style="color:#94a3b8;">Scope:</span> ' + p.scope + '</div>' : '') +
            (p.floor_label ? '<div><span style="color:#94a3b8;">Floor:</span> ' + p.floor_label + '</div>' : '') +
            '<div style="font-size:11px;color:#94a3b8;margin-top:4px;">' + Number(p.latitude).toFixed(6) + ', ' + Number(p.longitude).toFixed(6) + '</div>' +
          '</div>')
          .addTo(map);
      });

      map.on('mouseenter', 'control-points-layer', () => { map.getCanvas().style.cursor = 'pointer'; });
      map.on('mouseleave', 'control-points-layer', () => { map.getCanvas().style.cursor = ''; });
      map.on('mouseenter', 'icon-points-layer', () => { map.getCanvas().style.cursor = 'pointer'; });
      map.on('mouseleave', 'icon-points-layer', () => { map.getCanvas().style.cursor = ''; });
    });

    // Control visibility toggles
    const cpToggle = document.getElementById('control-points-toggle');
    if (cpToggle) {
      cpToggle.addEventListener('change', (e) => {
        const vis = e.target.checked ? 'visible' : 'none';
        if (map.getLayer('control-points-layer')) map.setLayoutProperty('control-points-layer', 'visibility', vis);
        if (map.getLayer('control-points-halo')) map.setLayoutProperty('control-points-halo', 'visibility', vis);
        if (map.getLayer('control-points-labels')) map.setLayoutProperty('control-points-labels', 'visibility', vis);
      });
    }

    const ipToggle = document.getElementById('icon-points-toggle');
    if (ipToggle) {
      ipToggle.addEventListener('change', (e) => {
        const vis = e.target.checked ? 'visible' : 'none';
        if (map.getLayer('icon-points-layer')) map.setLayoutProperty('icon-points-layer', 'visibility', vis);
      });
    }

    const opacitySlider = document.getElementById('opacity-slider');
    const opacityVal = document.getElementById('opacity-val');
    if (opacitySlider) {
      opacitySlider.addEventListener('input', (e) => {
        const val = parseFloat(e.target.value);
        if (opacityVal) opacityVal.textContent = Math.round(val * 100) + '%%%%';
        if (map.getLayer('overlay-tiles')) {
          map.setPaintProperty('overlay-tiles', 'raster-opacity', val);
        }
      });
    }

    const visToggle = document.getElementById('visibility-toggle');
    if (visToggle) {
      visToggle.addEventListener('change', (e) => {
        const vis = e.target.checked ? 'visible' : 'none';
        if (map.getLayer('overlay-tiles')) {
          map.setLayoutProperty('overlay-tiles', 'visibility', vis);
        }
      });
    }

    const resetBtn = document.getElementById('reset-view-btn');
    if (resetBtn) {
      resetBtn.addEventListener('click', () => {
        if (west < east && south < north) {
          map.fitBounds(bounds, { padding: 40 });
        } else {
          map.setCenter(center);
          map.setZoom(initialZoom);
        }
      });
    }

    const zoomDisplay = document.getElementById('zoom-val');
    map.on('zoom', () => {
      if (zoomDisplay) zoomDisplay.textContent = map.getZoom().toFixed(1);
    });

    const coordsDisplay = document.getElementById('coords-display');
    if (coordsDisplay) {
      map.on('mousemove', (e) => {
        coordsDisplay.textContent = 'Lng: ' + e.lngLat.lng.toFixed(5) + ' | Lat: ' + e.lngLat.lat.toFixed(5);
      });
    }

    map.on('error', (e) => {
      // Gracefully handle 404 or missing tile responses without breaking Viewscape
    });
  </script>
</body>
</html>
`,
		escapedTitle,
		escapedTitle, escapedTitle,
		opacity*100, opacity,
		params.InitialZoom, params.MinZoom, params.MaxZoom,
		params.RasterMinZoom, params.RasterMaxZoom,
		presetName,
		west, south, east, north,
		params.RasterMinZoom, params.RasterMaxZoom,
		params.InitialZoom,
		params.MinZoom, params.MaxZoom,
		opacity,
		styleJSON,
		string(cpJSON),
		string(ipJSON),
		string(bundledJSON),
	)
}

func generateViewers(params ViewscapeViewerParams) map[string]string {
	styleBytes, _ := generateMapLibreStyleJSON(params.Title, params.Bounds, params.RasterMinZoom, params.RasterMaxZoom, params.Opacity)
	viewerHTML := generateMapLibreViewerHTML(params, string(styleBytes))

	return map[string]string{
		"viewer.html": viewerHTML,
		"index.html":  viewerHTML,
	}
}

// createCompositeRasterWithMarkers burns control points and icon points directly onto a copy
// of the background raster so that the exported map/tiles display the marked annotations.
func createCompositeRasterWithMarkers(baseImagePath string, controlPoints []*models.ControlPoint, iconPoints []*models.IconPoint, bounds []float64, sessionID string) (string, error) {
	if len(controlPoints) == 0 && len(iconPoints) == 0 {
		return baseImagePath, nil
	}

	srcFile, err := os.Open(baseImagePath)
	if err != nil {
		return baseImagePath, err
	}
	defer srcFile.Close()

	srcImg, _, err := image.Decode(srcFile)
	if err != nil {
		return baseImagePath, err
	}

	boundsRect := srcImg.Bounds()
	imgW := boundsRect.Dx()
	imgH := boundsRect.Dy()
	if imgW <= 0 || imgH <= 0 {
		return baseImagePath, nil
	}

	// Create a writable RGBA canvas
	canvas := image.NewRGBA(boundsRect)
	draw.Draw(canvas, boundsRect, srcImg, boundsRect.Min, draw.Src)

	// Helper to draw filled circle
	drawCircle := func(cx, cy, radius int, col color.Color) {
		r2 := radius * radius
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx*dx+dy*dy <= r2 {
					canvas.Set(cx+dx, cy+dy, col)
				}
			}
		}
	}

	// Helper to draw stroked ring
	drawRing := func(cx, cy, innerR, outerR int, col color.Color) {
		in2 := innerR * innerR
		out2 := outerR * outerR
		for dy := -outerR; dy <= outerR; dy++ {
			for dx := -outerR; dx <= outerR; dx++ {
				d2 := dx*dx + dy*dy
				if d2 <= out2 && d2 >= in2 {
					canvas.Set(cx+dx, cy+dy, col)
				}
			}
		}
	}

	// Map geo coordinates (lng, lat) to image pixel coordinates using bounds or control points
	var west, south, east, north float64
	hasBounds := len(bounds) == 4 && bounds[2] > bounds[0] && bounds[3] > bounds[1]
	if hasBounds {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}

	// Dynamic marker radius according to image size
	cpR := 11
	dotR := 10
	if imgW > 1500 || imgH > 1500 {
		cpR = 18
		dotR = 16
	}
	if imgW > 3000 || imgH > 3000 {
		cpR = 26
		dotR = 24
	}

	// 1. Draw Control Points (green crosshairs with white ring)
	for _, cp := range controlPoints {
		px := int(math.Round(cp.ImageX))
		py := int(math.Round(cp.ImageY))
		if (px == 0 && py == 0) && hasBounds && (cp.Longitude != 0 || cp.Latitude != 0) {
			if cp.Longitude >= west && cp.Longitude <= east && cp.Latitude >= south && cp.Latitude <= north {
				normX := (cp.Longitude - west) / (east - west)
				normY := (north - cp.Latitude) / (north - south)
				px = int(math.Round(normX * float64(imgW)))
				py = int(math.Round(normY * float64(imgH)))
			}
		}
		if px < 0 || px >= imgW || py < 0 || py >= imgH {
			continue
		}

		// Outer halo ring
		drawRing(px, py, cpR, cpR+3, color.RGBA{255, 255, 255, 220})
		// Inner green ring
		drawRing(px, py, cpR-3, cpR, color.RGBA{22, 163, 74, 255})
		// Center green dot
		drawCircle(px, py, cpR/2, color.RGBA{22, 163, 74, 255})
		// Center white dot
		drawCircle(px, py, cpR/4, color.RGBA{255, 255, 255, 255})
	}

	// 2. Draw Icon Points
	for _, ip := range iconPoints {
		var px, py int
		if hasBounds {
			lng := ip.Longitude
			lat := ip.Latitude
			if lng >= west-0.05*(east-west) && lng <= east+0.05*(east-west) && lat >= south-0.05*(north-south) && lat <= north+0.05*(north-south) {
				normX := (lng - west) / (east - west)
				normY := (north - lat) / (north - south)
				if normX < 0 {
					normX = 0
				}
				if normX > 1 {
					normX = 1
				}
				if normY < 0 {
					normY = 0
				}
				if normY > 1 {
					normY = 1
				}
				px = int(math.Round(normX * float64(imgW)))
				py = int(math.Round(normY * float64(imgH)))
			} else {
				continue
			}
		} else {
			continue
		}

		if px < 0 || px >= imgW || py < 0 || py >= imgH {
			continue
		}

		// Try loading custom/known icon image asset
		var iconImg image.Image
		var candidates []string
		if ip.IconPath != "" {
			candidates = append(candidates, ip.IconPath, filepath.Base(ip.IconPath))
		}
		if ip.IconID != "" {
			cleanIconID := filepath.Base(ip.IconID)
			ext := filepath.Ext(cleanIconID)
			if ext != "" {
				candidates = append(candidates, cleanIconID)
			} else {
				for _, e := range []string{".png", ".svg", ".webp", ".ico", ".jpg"} {
					candidates = append(candidates, cleanIconID+e)
				}
			}
		}
		searchDirs := []string{
			filepath.Join("frontend", "public", "icons"),
			filepath.Join("..", "frontend", "public", "icons"),
			filepath.Join("..", "..", "frontend", "public", "icons"),
		}
		if sessionID != "" {
			sessionDir := filepath.Join("temp", "sessions", sessionID)
			_ = filepath.Walk(sessionDir, func(p string, info os.FileInfo, err error) error {
				if err == nil && info.IsDir() {
					cleanDir := filepath.ToSlash(p)
					if strings.HasSuffix(cleanDir, "/icons/assets") || strings.HasSuffix(cleanDir, "/icons") {
						searchDirs = append(searchDirs, p)
					}
				}
				return nil
			})
		}
		for _, cand := range candidates {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				if f, err := os.Open(cand); err == nil {
					dec, _, decErr := image.Decode(f)
					_ = f.Close()
					if decErr == nil {
						iconImg = dec
						break
					}
				}
			}
			for _, dir := range searchDirs {
				p := filepath.Join(dir, cand)
				if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
					if f, err := os.Open(p); err == nil {
						dec, _, decErr := image.Decode(f)
						_ = f.Close()
						if decErr == nil {
							iconImg = dec
							break
						}
					}
				}
			}
			if iconImg != nil {
				break
			}
		}

		if iconImg != nil {
			// Stamp icon centered at px, py (default size 32x32)
			iconBounds := iconImg.Bounds()
			targetSize := 32
			if imgW > 1500 || imgH > 1500 {
				targetSize = 48
			}
			halfSize := targetSize / 2
			dstX0 := px - halfSize
			dstY0 := py - halfSize
			dstX1 := px + halfSize
			dstY1 := py + halfSize

			// Draw white halo background badge for visibility
			drawCircle(px, py, halfSize+3, color.RGBA{255, 255, 255, 230})
			drawRing(px, py, halfSize+2, halfSize+4, color.RGBA{30, 41, 59, 200})

			// Scale & overlay icon onto canvas using nearest-neighbor
			srcW := iconBounds.Dx()
			srcH := iconBounds.Dy()
			if srcW > 0 && srcH > 0 {
				for dy := dstY0; dy < dstY1; dy++ {
					if dy < 0 || dy >= imgH {
						continue
					}
					sy := iconBounds.Min.Y + int(float64(dy-dstY0)/float64(targetSize)*float64(srcH))
					if sy >= iconBounds.Max.Y {
						sy = iconBounds.Max.Y - 1
					}
					for dx := dstX0; dx < dstX1; dx++ {
						if dx < 0 || dx >= imgW {
							continue
						}
						sx := iconBounds.Min.X + int(float64(dx-dstX0)/float64(targetSize)*float64(srcW))
						if sx >= iconBounds.Max.X {
							sx = iconBounds.Max.X - 1
						}
						c := iconImg.At(sx, sy)
						_, _, _, a := c.RGBA()
						if a > 0 {
							canvas.Set(dx, dy, c)
						}
					}
				}
			}
		} else {
			// Default red dot fallback (with white outline and white center dot)
			drawRing(px, py, dotR, dotR+3, color.RGBA{255, 255, 255, 255})
			drawCircle(px, py, dotR, color.RGBA{220, 38, 38, 245})
			drawCircle(px, py, dotR/3, color.RGBA{255, 255, 255, 255})
		}
	}

	// Save composite to temporary file in dataset's processed folder
	compDir := filepath.Join(filepath.Dir(baseImagePath), "processed")
	_ = os.MkdirAll(compDir, 0755)
	compositePath := filepath.Join(compDir, "composite_rendered_map.png")
	outF, err := os.Create(compositePath)
	if err != nil {
		return baseImagePath, err
	}
	defer outF.Close()

	if err := png.Encode(outF, canvas); err != nil {
		return baseImagePath, err
	}

	return compositePath, nil
}
