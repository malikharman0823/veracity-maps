package services

import (
	"archive/zip"
	"encoding/json"
	"fmt"
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
	"strings"
	"time"

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

	// Render XYZ tiles if georeferenced
	var generatedZooms []int
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

	if len(boundsList) == 4 && (dataset.GeoreferenceStatus == "georeferenced" || fileExists(tileSourceImage)) {
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

	// Package permitted offline basemap tiles into basemap/{z}/{x}/{y}.png
	hasBasemap := false
	if len(boundsList) == 4 {
		basemapZooms := generatedZooms
		bmMinZ := minZoom
		bmMaxZ := maxZoom
		if len(basemapZooms) == 0 {
			bmMinZ, bmMaxZ = 14, 18
		}
		packagedBM, err := gis.PackageBasemapTiles(archive, boundsList, bmMinZ, bmMaxZ)
		if err == nil && packagedBM > 0 {
			hasBasemap = true
		}
	}

	// Calculate center and initial zoom
	var centerCoords []float64
	initZoom := minZoom
	if len(boundsList) == 4 {
		centerCoords = []float64{(boundsList[0] + boundsList[2]) / 2.0, (boundsList[1] + boundsList[3]) / 2.0}
	} else {
		centerCoords = []float64{0.0, 0.0}
	}
	if initZoom < 1 {
		initZoom = 15
	}

	metadataDoc := map[string]interface{}{
		"name":              projectName,
		"version":           "2.0.0",
		"description":       "Exported from Veracity Maps",
		"attribution":       "Veracity Maps & OpenStreetMap contributors",
		"type":              "overlay",
		"format":            "png",
		"bounds":            boundsList,
		"center":            centerCoords,
		"initial_zoom":      initZoom,
		"profile":           "mercator",
		"minzoom":           minZoom,
		"maxzoom":           maxZoom,
		"tile_url_template": "./{z}/{x}/{y}.png",
		"available_zooms":   generatedZooms,
		"crs":               "EPSG:3857",
		"basemap": map[string]interface{}{
			"type":              "raster-xyz",
			"name":              "OpenStreetMap",
			"tile_url_template": "./basemap/{z}/{x}/{y}.png",
			"online_fallback":   "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
			"attribution":       "© OpenStreetMap contributors",
			"offline_packaged":  hasBasemap,
		},
		"overlay": map[string]interface{}{
			"tile_url_template": "./{z}/{x}/{y}.png",
			"bounds":            boundsList,
			"has_tiles":         len(generatedZooms) > 0,
		},
		"user_layers": []map[string]interface{}{
			{
				"id":         dataset.ID,
				"name":       dataset.Filename,
				"type":       "raster",
				"tile_path":  "./{z}/{x}/{y}.png",
				"visibility": true,
				"opacity":    1.0,
			},
		},
		"offline_ready": true,
		"export_time":   time.Now().UTC().Format(time.RFC3339),
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

	// HTML Viewers and Local Launchers
	viewers := generateViewers(projectName, boundsList, minZoom, maxZoom, len(generatedZooms) > 0, hasBasemap)
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

	// Rendered composite map image with markers burned in
	if compositePath != "" && fileExists(compositePath) {
		if cf, err := os.Open(compositePath); err == nil {
			defer cf.Close()
			if cw, err := archive.Create("processed/composite_rendered_map.png"); err == nil {
				_, _ = io.Copy(cw, cf)
			}
		}
	}

	// Export Icon Points and Icon Assets into separate icons/ directory
	if len(iconPoints) > 0 {
		iconAssetsCopied := make(map[string]bool)
		for _, pt := range iconPoints {
			if pt.IconID != "" && !iconAssetsCopied[pt.IconID] {
				cleanIconID := filepath.Base(pt.IconID)
				ext := filepath.Ext(cleanIconID)
				var candidates []string
				if ext != "" {
					candidates = append(candidates, cleanIconID)
				} else {
					for _, e := range []string{".png", ".svg", ".webp", ".ico"} {
						candidates = append(candidates, cleanIconID+e)
					}
				}
				searchDirs := []string{
					filepath.Join("frontend", "public", "icons"),
					filepath.Join("..", "frontend", "public", "icons"),
					filepath.Join("..", "..", "frontend", "public", "icons"),
				}
				var foundPath string
				var foundName string
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
				if foundPath != "" {
					if f, err := os.Open(foundPath); err == nil {
						if iw, err := archive.Create("icons/assets/" + foundName); err == nil {
							_, _ = io.Copy(iw, f)
						}
						_ = f.Close()
						iconAssetsCopied[pt.IconID] = true
					}
				}
			}
		}
		if ipBytes, err := json.MarshalIndent(iconPoints, "", "  "); err == nil {
			if ipWriter, err := archive.Create("icons/icon_points.json"); err == nil {
				_, _ = ipWriter.Write(ipBytes)
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

func generateViewers(title string, bounds []float64, minZoom, maxZoom int, hasTiles bool, hasBasemap bool) map[string]string {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(bounds) == 4 {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}
	centerLng := (west + east) / 2.0
	centerLat := (south + north) / 2.0
	initZoom := minZoom
	if initZoom < 1 {
		initZoom = 15
	}

	tileUrlRel := "./{z}/{x}/{y}.png"
	if !hasTiles {
		tileUrlRel = ""
	}

	basemapUrlRel := "./basemap/{z}/{x}/{y}.png"
	if !hasBasemap {
		basemapUrlRel = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
	}

	// PRIMARY VIEWER: index.html (Interactive standalone map viewer with embedded config)
	indexHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>%s - Veracity Maps Standalone Viewer</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.css" crossorigin="" />
  <script src="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.js" crossorigin=""></script>
  <style>
    * { box-sizing: border-box; }
    html, body { height: 100%%; margin: 0; padding: 0; background: #0f172a; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; overflow: hidden; }
    #map { width: 100%%; height: 100%%; background: #1e293b; }
    
    .top-panel {
      position: absolute; top: 12px; left: 14px; z-index: 1000;
      background: rgba(15, 23, 42, 0.92); backdrop-filter: blur(8px);
      padding: 12px 18px; border-radius: 8px; border: 1px solid rgba(255,255,255,0.12);
      box-shadow: 0 8px 24px rgba(0,0,0,0.4); color: #f8fafc; max-width: 380px;
    }
    .top-panel h1 { margin: 0 0 4px 0; font-size: 16px; font-weight: 700; color: #38bdf8; display: flex; align-items: center; gap: 8px; }
    .top-panel .subtitle { font-size: 12px; color: #94a3b8; margin: 0 0 10px 0; }
    
    .controls { display: flex; flex-direction: column; gap: 8px; font-size: 12px; }
    .control-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
    .control-row label { color: #cbd5e1; cursor: pointer; display: flex; align-items: center; gap: 6px; }
    .control-row input[type="range"] { flex: 1; accent-color: #0284c7; }
    .btn-group { display: flex; gap: 6px; margin-top: 6px; }
    .btn {
      background: #0284c7; color: #ffffff; border: none; border-radius: 4px;
      padding: 5px 10px; font-size: 11px; font-weight: 600; cursor: pointer;
      text-decoration: none; display: inline-flex; align-items: center; gap: 4px;
    }
    .btn:hover { background: #0369a1; }
    .btn-secondary { background: #334155; color: #e2e8f0; }
    .btn-secondary:hover { background: #475569; }

    .nav-bar {
      position: absolute; top: 12px; right: 14px; z-index: 1000;
      background: rgba(15, 23, 42, 0.92); backdrop-filter: blur(8px);
      padding: 8px 14px; border-radius: 8px; border: 1px solid rgba(255,255,255,0.12);
      box-shadow: 0 8px 24px rgba(0,0,0,0.4); font-size: 12px; color: #94a3b8;
    }
    .nav-bar a { color: #38bdf8; text-decoration: none; margin: 0 6px; font-weight: 500; }
    .nav-bar a:hover { text-decoration: underline; }

    #error-toast {
      position: absolute; bottom: 20px; left: 50%%; transform: translateX(-50%%); z-index: 2000;
      background: #ef4444; color: #fff; padding: 10px 18px; border-radius: 6px;
      font-size: 13px; font-weight: 500; box-shadow: 0 4px 12px rgba(0,0,0,0.3); display: none;
    }
  </style>
</head>
<body>
  <div class="top-panel">
    <h1>
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>
      %s
    </h1>
    <div class="subtitle">Standalone Offline Map Export &bull; EPSG:3857</div>
    <div class="controls">
      <div class="control-row">
        <label><input type="checkbox" id="toggle-basemap" checked /> Base World Map</label>
        <span id="basemap-mode" style="color:#64748b; font-size:11px;">%s</span>
      </div>
      <div class="control-row">
        <label><input type="checkbox" id="toggle-overlay" checked /> Dataset Overlay</label>
        <input type="range" id="opacity-slider" min="0" max="1" step="0.05" value="1.0" />
      </div>
      <div class="btn-group">
        <button class="btn" id="btn-fit">Fit Dataset Bounds</button>
        <a class="btn btn-secondary" href="metadata.json" target="_blank">metadata.json</a>
      </div>
    </div>
  </div>

  <div class="nav-bar">
    <a href="leaflet.html">Leaflet</a> |
    <a href="openlayers.html">OpenLayers</a> |
    <a href="googlemaps.html">Google Maps</a>
  </div>

  <div id="error-toast"></div>
  <div id="map"></div>

  <script>
    // Embedded metadata configuration to guarantee offline file:/// compatibility
    var MAP_CONFIG = {
      title: "%s",
      bounds: [[%.6f, %.6f], [%.6f, %.6f]],
      center: [%.6f, %.6f],
      minZoom: %d,
      maxZoom: %d,
      hasTiles: %t,
      hasBasemap: %t,
      overlayTemplate: "%s",
      basemapTemplate: "%s",
      onlineBasemapUrl: "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
      attribution: "Veracity Maps & OpenStreetMap contributors"
    };

    function showError(msg) {
      var toast = document.getElementById("error-toast");
      toast.textContent = msg;
      toast.style.display = "block";
      setTimeout(function() { toast.style.display = "none"; }, 6000);
    }

    try {
      var map = L.map('map', {
        minZoom: Math.max(1, MAP_CONFIG.minZoom - 4),
        maxZoom: Math.min(22, MAP_CONFIG.maxZoom + 4),
        attributionControl: true
      });

      // Fit dataset bounds initially
      map.fitBounds(MAP_CONFIG.bounds, { padding: [60, 60] });

      // 1. BASEMAP LAYER (Bottom)
      var basemapUrl = MAP_CONFIG.hasBasemap ? MAP_CONFIG.basemapTemplate : MAP_CONFIG.onlineBasemapUrl;
      var basemapLayer = L.tileLayer(basemapUrl, {
        maxZoom: 22,
        attribution: '&copy; <a href="https://openstreetmap.org/copyright" target="_blank">OpenStreetMap</a> contributors'
      }).addTo(map);

      // Fallback from local basemap to online OSM if local tile 404s
      if (MAP_CONFIG.hasBasemap) {
        basemapLayer.on('tileerror', function(error, tile) {
          if (!this._hasFallenBack && navigator.onLine) {
            console.log("Local basemap tile missing, falling back to online OpenStreetMap");
            this.setUrl(MAP_CONFIG.onlineBasemapUrl);
            this._hasFallenBack = true;
          }
        });
      }

      // 2. DATASET OVERLAY LAYER (Top)
      var overlayLayer = null;
      if (MAP_CONFIG.hasTiles) {
        overlayLayer = L.tileLayer(MAP_CONFIG.overlayTemplate, {
          minZoom: MAP_CONFIG.minZoom,
          maxZoom: MAP_CONFIG.maxZoom,
          bounds: MAP_CONFIG.bounds,
          opacity: 1.0,
          tms: false,
          attribution: 'Veracity Maps'
        }).addTo(map);
      } else {
        overlayLayer = L.imageOverlay('assets/source/preview.png', MAP_CONFIG.bounds, { opacity: 1.0 }).addTo(map);
      }

      // Controls Binding
      document.getElementById('toggle-basemap').addEventListener('change', function(e) {
        if (e.target.checked) {
          map.addLayer(basemapLayer);
          basemapLayer.bringToBack();
        } else {
          map.removeLayer(basemapLayer);
        }
      });

      document.getElementById('toggle-overlay').addEventListener('change', function(e) {
        if (overlayLayer) {
          if (e.target.checked) map.addLayer(overlayLayer);
          else map.removeLayer(overlayLayer);
        }
      });

      document.getElementById('opacity-slider').addEventListener('input', function(e) {
        var op = parseFloat(e.target.value);
        if (overlayLayer) {
          overlayLayer.setOpacity(op);
        }
      });

      document.getElementById('btn-fit').addEventListener('click', function() {
        map.fitBounds(MAP_CONFIG.bounds, { padding: [60, 60] });
      });

    } catch (err) {
      console.error("Map initialization error:", err);
      showError("Map load error: " + err.message + ". If opening via file:///, ensure local scripts are permitted or use serve.bat.");
    }
  </script>
</body>
</html>`, title, title, func() string {
		if hasBasemap {
			return "Local Offline Tiles"
		}
		return "Online Fallback"
	}(), title, south, west, north, east, centerLat, centerLng, minZoom, maxZoom, hasTiles, hasBasemap, tileUrlRel, basemapUrlRel)

	// ALTERNATIVE VIEWER 1: leaflet.html
	leafletHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>%s - Leaflet Viewer</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.css" crossorigin="" />
  <script src="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.9.4/leaflet.js" crossorigin=""></script>
  <style>
    html, body { height: 100%%; margin: 0; padding: 0; }
    #map { width: 100%%; height: 100%%; }
    .nav-bar { position: absolute; top: 10px; right: 10px; z-index: 1000; background: rgba(255,255,255,0.95); padding: 8px 14px; border-radius: 6px; box-shadow: 0 2px 6px rgba(0,0,0,0.25); font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
    .nav-bar a { color: #1e40af; text-decoration: none; margin: 0 6px; font-weight: 500; }
    .nav-bar a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <div class="nav-bar">
    <a href="index.html">&larr; Main Viewer</a> |
    <a href="openlayers.html">OpenLayers</a> |
    <a href="googlemaps.html">Google Maps</a> |
    <a href="metadata.json" target="_blank">metadata.json</a>
  </div>
  <div id="map"></div>
  <script>
    var bounds = [[%.6f, %.6f], [%.6f, %.6f]];
    var map = L.map('map').fitBounds(bounds, { padding: [50, 50] });
    var basemapUrl = %t ? './basemap/{z}/{x}/{y}.png' : 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png';
    L.tileLayer(basemapUrl, {
      maxZoom: 22,
      attribution: '&copy; <a href="https://openstreetmap.org/copyright">OpenStreetMap</a> contributors'
    }).addTo(map);

    var hasTiles = %t;
    if (hasTiles) {
      L.tileLayer('%s', {
        minZoom: %d,
        maxZoom: %d,
        bounds: bounds,
        tms: false,
        attribution: 'Veracity Maps'
      }).addTo(map);
    } else {
      L.imageOverlay('assets/source/preview.png', bounds, { opacity: 0.85 }).addTo(map);
    }
  </script>
</body>
</html>`, title, south, west, north, east, hasBasemap, hasTiles, tileUrlRel, minZoom, maxZoom)

	// ALTERNATIVE VIEWER 2: openlayers.html
	openLayersHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>%s - OpenLayers Viewer</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/ol@v10.2.1/ol.css">
  <script src="https://cdn.jsdelivr.net/npm/ol@v10.2.1/dist/ol.js"></script>
  <style>
    html, body { height: 100%%; margin: 0; padding: 0; }
    #map { width: 100%%; height: 100%%; }
    .nav-bar { position: absolute; top: 10px; right: 10px; z-index: 1000; background: rgba(255,255,255,0.95); padding: 8px 14px; border-radius: 6px; box-shadow: 0 2px 6px rgba(0,0,0,0.25); font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
    .nav-bar a { color: #1e40af; text-decoration: none; margin: 0 6px; font-weight: 500; }
    .nav-bar a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <div class="nav-bar">
    <a href="index.html">&larr; Main Viewer</a> |
    <a href="leaflet.html">Leaflet</a> |
    <a href="googlemaps.html">Google Maps</a> |
    <a href="metadata.json" target="_blank">metadata.json</a>
  </div>
  <div id="map"></div>
  <script>
    var extent = ol.proj.transformExtent([%.6f, %.6f, %.6f, %.6f], 'EPSG:4326', 'EPSG:3857');
    var basemapSource = %t ? new ol.source.XYZ({ url: './basemap/{z}/{x}/{y}.png' }) : new ol.source.OSM();
    var osmLayer = new ol.layer.Tile({
      source: basemapSource
    });

    var layers = [osmLayer];
    var hasTiles = %t;
    if (hasTiles) {
      var customTileLayer = new ol.layer.Tile({
        source: new ol.source.XYZ({
          url: '%s',
          minZoom: %d,
          maxZoom: %d
        })
      });
      layers.push(customTileLayer);
    }

    var map = new ol.Map({
      target: 'map',
      layers: layers,
      view: new ol.View({
        center: ol.proj.fromLonLat([%.6f, %.6f]),
        zoom: %d
      })
    });
    map.getView().fit(extent, { padding: [50, 50, 50, 50], maxZoom: %d });
  </script>
</body>
</html>`, title, west, south, east, north, hasBasemap, hasTiles, tileUrlRel, minZoom, maxZoom, centerLng, centerLat, initZoom, maxZoom)

	// ALTERNATIVE VIEWER 3: googlemaps.html
	googleMapsHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>%s - Google Maps Viewer</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
    html, body { height: 100%%; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
    #map { width: 100%%; height: 100%%; }
    .nav-bar { position: absolute; top: 10px; right: 10px; z-index: 1000; background: rgba(255,255,255,0.95); padding: 8px 14px; border-radius: 6px; box-shadow: 0 2px 6px rgba(0,0,0,0.25); font-size: 13px; }
    .nav-bar a { color: #1e40af; text-decoration: none; margin: 0 6px; font-weight: 500; }
    .nav-bar a:hover { text-decoration: underline; }
    #key-banner { position: absolute; top: 12px; left: 50%%; transform: translateX(-50%%); z-index: 1000; background: #ffffff; padding: 12px 18px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); display: flex; align-items: center; gap: 8px; font-size: 13px; }
    #key-banner input { padding: 6px 10px; border: 1px solid #cbd5e1; border-radius: 4px; font-size: 13px; width: 280px; }
    #key-banner button { background: #2563eb; color: #fff; border: none; padding: 6px 12px; border-radius: 4px; cursor: pointer; font-weight: 500; }
    #key-banner button:hover { background: #1d4ed8; }
  </style>
</head>
<body>
  <div class="nav-bar">
    <a href="index.html">&larr; Main Viewer</a> |
    <a href="leaflet.html">Leaflet</a> |
    <a href="openlayers.html">OpenLayers</a> |
    <a href="metadata.json" target="_blank">metadata.json</a>
  </div>
  <div id="key-banner">
    <span>Google Maps API Key:</span>
    <input type="text" id="api-key-input" placeholder="Paste your Google Maps API key" />
    <button onclick="loadMap()">Load Map</button>
  </div>
  <div id="map"></div>

  <script>
    var boundsGeo = {
      north: %.6f,
      south: %.6f,
      east: %.6f,
      west: %.6f
    };
    var centerCoords = { lat: %.6f, lng: %.6f };
    var minZ = %d, maxZ = %d;
    var hasTiles = %t;

    function getParam(param) {
      var match = RegExp('[?&]' + param + '=([^&]*)').exec(window.location.search);
      return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
    }

    function initGoogleMap() {
      document.getElementById('key-banner').style.display = 'none';
      var map = new google.maps.Map(document.getElementById('map'), {
        center: centerCoords,
        zoom: %d,
        mapTypeId: 'roadmap'
      });

      var mapBounds = new google.maps.LatLngBounds(
        new google.maps.LatLng(boundsGeo.south, boundsGeo.west),
        new google.maps.LatLng(boundsGeo.north, boundsGeo.east)
      );
      map.fitBounds(mapBounds);

      if (hasTiles) {
        var tileOverlay = new google.maps.ImageMapType({
          getTileUrl: function(coord, zoom) {
            return zoom + '/' + coord.x + '/' + coord.y + '.png';
          },
          tileSize: new google.maps.Size(256, 256),
          maxZoom: maxZ,
          minZoom: minZ,
          name: 'VeracityTiles'
        });
        map.overlayMapTypes.push(tileOverlay);
      }
    }

    function loadMap() {
      var key = document.getElementById('api-key-input').value.trim() || getParam('key');
      if (!key) {
        alert('Please enter a valid Google Maps API Key or pass ?key=YOUR_KEY in the URL');
        return;
      }
      var script = document.createElement('script');
      script.src = 'https://maps.googleapis.com/maps/api/js?key=' + encodeURIComponent(key) + '&callback=initGoogleMap';
      script.async = true;
      script.defer = true;
      document.head.appendChild(script);
    }

    var preKey = getParam('key');
    if (preKey) {
      document.getElementById('api-key-input').value = preKey;
      loadMap();
    }
  </script>
</body>
</html>`, title, north, south, east, west, centerLat, centerLng, minZoom, maxZoom, hasTiles, initZoom)

	// Standalone local launcher scripts
	serveBat := `@echo off
echo Starting local static map server...
echo Serving current directory on http://127.0.0.1:8080
start http://127.0.0.1:8080/index.html
python -m http.server 8080 --bind 127.0.0.1
`
	servePy := `#!/usr/bin/env python3
import http.server
import socketserver
import webbrowser

PORT = 8080
Handler = http.server.SimpleHTTPRequestHandler

print(f"Serving Veracity Maps export on http://127.0.0.1:{PORT}")
webbrowser.open(f"http://127.0.0.1:{PORT}/index.html")

with socketserver.TCPServer(("127.0.0.1", PORT), Handler) as httpd:
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nServer stopped.")
`

	return map[string]string{
		"index.html":      indexHTML,
		"leaflet.html":    leafletHTML,
		"openlayers.html": openLayersHTML,
		"googlemaps.html": googleMapsHTML,
		"serve.bat":       serveBat,
		"serve.py":        servePy,
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

	// 1. Draw Control Points (green crosshairs with white ring)
	for _, cp := range controlPoints {
		px := int(math.Round(cp.ImageX))
		py := int(math.Round(cp.ImageY))
		if px < 0 || px >= imgW || py < 0 || py >= imgH {
			continue
		}

		// Outer halo ring
		drawRing(px, py, 11, 14, color.RGBA{255, 255, 255, 220})
		// Inner green ring
		drawRing(px, py, 8, 11, color.RGBA{22, 163, 74, 255})
		// Center green dot
		drawCircle(px, py, 4, color.RGBA{22, 163, 74, 255})
		// Center white dot
		drawCircle(px, py, 2, color.RGBA{255, 255, 255, 255})
	}

	// 2. Draw Icon Points
	// Map geo coordinates (lng, lat) to image pixel coordinates using bounds or control points
	var west, south, east, north float64
	hasBounds := len(bounds) == 4 && bounds[2] > bounds[0] && bounds[3] > bounds[1]
	if hasBounds {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}

	for _, ip := range iconPoints {
		var px, py int
		if hasBounds {
			lng := ip.Longitude
			lat := ip.Latitude
			if lng >= west && lng <= east && lat >= south && lat <= north {
				normX := (lng - west) / (east - west)
				normY := (north - lat) / (north - south)
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
		if ip.IconID != "" {
			cleanIconID := filepath.Base(ip.IconID)
			ext := filepath.Ext(cleanIconID)
			var candidates []string
			if ext != "" {
				candidates = append(candidates, cleanIconID)
			} else {
				for _, e := range []string{".png", ".svg", ".webp", ".ico", ".jpg"} {
					candidates = append(candidates, cleanIconID+e)
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
			for _, dir := range searchDirs {
				for _, cand := range candidates {
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
			// Red marker fallback
			drawRing(px, py, 10, 12, color.RGBA{255, 255, 255, 255})
			drawCircle(px, py, 10, color.RGBA{220, 38, 38, 240})
			drawCircle(px, py, 4, color.RGBA{255, 255, 255, 255})
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
