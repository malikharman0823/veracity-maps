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

	metadataDoc := map[string]interface{}{
		"name":        projectName,
		"version":     "1.1.0",
		"description": "",
		"attribution": "<a href=\"https://www.maptiler.com/engine/\">Rendered with MapTiler Engine</a>, non-commercial use only",
		"type":        "overlay",
		"format":      "hybrid",
		"minzoom":     strconv.Itoa(minZoom),
		"maxzoom":     strconv.Itoa(maxZoom),
		"scale":       "1.000000",
		"bounds":      boundsList,
		"profile":     "mercator",
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

	// HTML Viewers (index.html, leaflet.html, openlayers.html, googlemaps.html)
	viewers := generateViewers(projectName, boundsList, minZoom, maxZoom)
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

func generateViewers(title string, bounds []float64, minZoom, maxZoom int) map[string]string {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(bounds) == 4 {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}
	centerLng := (west + east) / 2.0
	centerLat := (south + north) / 2.0
	initZoom := minZoom
	if initZoom < 1 {
		initZoom = 7
	}

	boundsStr := fmt.Sprintf("%.8f,%.8f,%.8f,%.8f", west, south, east, north)

	// index.html: MapTiler tileviewer template calling tileserver(data)
	indexHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
  <head>
    <title>%s</title>
    <meta charset="utf-8"/>
    <link rel="stylesheet" href="https://cdn.maptiler.com/tileviewer/v1/index.css" type="text/css"/>
    <script src="https://cdn.maptiler.com/tileviewer/v1/index.js"></script>
  </head>
  <body>
    <script type="text/javascript">
      // var GOOGLEMAPS_KEY = 'YOUR-API-KEY';
      var data = {
        "name": "%s",
        "version": "1.0.0",
        "description": "",
        "type": "overlay",
        "format": "hybrid",
        "minzoom": "%d",
        "maxzoom": "%d",
        "bounds": "%s",
        "scale": "1.000000",
        "profile": "mercator"
      };
      tileserver(data);
    </script>
  </body>
</html>
`, title, title, minZoom, maxZoom, boundsStr)

	// leaflet.html: Leaflet 1.7.1 template matching MapTiler Engine export
	leafletHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
  <head>
    <title>%s</title>
    <meta http-equiv="imagetoolbar" content="no">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.7.1/leaflet.css">
    <link rel="stylesheet" href="https://cdn.rawgit.com/ardhi/Leaflet.MousePosition/master/src/L.Control.MousePosition.css">
    <script src="https://cdnjs.cloudflare.com/ajax/libs/leaflet/1.7.1/leaflet.js" type="text/javascript"></script>
    <script src="https://cdn.rawgit.com/ardhi/Leaflet.MousePosition/master/src/L.Control.MousePosition.js" type="text/javascript"></script>
    <style>
      body { margin:0; padding:0; width:100%%; height:100%%; background: #ffffff; }
      #map { position:absolute; top:0; bottom:0; width:100%%; z-index: 1; }
      #slider{ position: absolute; top: 10px; right: 10px; z-index: 5; }
#api-keys-info-panel {color: #946F00; background-color:#FFF7D6; border-radius:0.375rem; position:absolute; width:35rem;  top:3rem; left:50%%; margin-left:-20rem; display:flex; align-items: center; justify-content: center; gap: 1rem;  padding:0 1.5rem; box-sizing: 'content-box;'}
#api-keys-info-panel p {width:100%%; font-size:1rem; }
#api-keys-info-panel p a {color: #946F00; }
    </style>
  </head>
<body>
  <div id="map"></div>
<div id='api-keys-info-panel' >
  <svg width="2rem" height="2rem" viewBox="0 0 18 20" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M9 5.8675L14.6475 15.625H3.3525L9 5.8675ZM9 2.875L0.75 17.125H17.25L9 2.875Z" fill="#946F00"/>
    <path d="M9.75 13.375H8.25V14.875H9.75V13.375Z" fill="#946F00"/>
    <path d="M9.75 8.875H8.25V12.625H9.75V8.875Z" fill="#946F00"/>
  </svg>
  <p>Set your API key in the MapTiler Engine settings to display the map correctly.<br/>  Learn how in our <a target='_blank' href='https://docs.maptiler.com/guides/map-tiling-hosting/data-processing/output-preview/'>documentation</a>.
  </p>
  <input id="close-info-panel-btn" type="button" value="Close" />
</div>
  <input id="slider" type="range" min="0" max="1" step="0.1" value="1" oninput="layer.setOpacity(this.value)">
  <script type="text/javascript">
    var mapExtent = [%.8f, %.8f, %.8f, %.8f];
      var crs = L.CRS.Simple;
    var mapMinZoom = %d;
    var mapMaxZoom = %d;
    var bounds = new L.LatLngBounds(
      new L.LatLng(mapExtent[1], mapExtent[0]),
      new L.LatLng(mapExtent[3], mapExtent[2]));
    var map = L.map('map').fitBounds(bounds);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png').addTo(map);
    var layer;
    var options = {
      minZoom: mapMinZoom,
      maxZoom: mapMaxZoom,
      opacity: 1.0,
      attribution: '<a href="https://www.maptiler.com/engine/">Rendered with MapTiler Engine</a>, non-commercial use only',
      tms: false,
    };
    layer = L.tileLayer('{z}/{x}/{y}', options).addTo(map);
    map.setZoom(%d);
  </script>
<script type="text/javascript">
  var panelCloseBtn = document.getElementById('close-info-panel-btn');
  panelCloseBtn.addEventListener("click", () => {document.getElementById("api-keys-info-panel").innerHTML = '';});
</script>
</body>
</html>
`, title, west, south, east, north, minZoom, maxZoom, initZoom)

	// openlayers.html: OpenLayers 6.12.0 template matching MapTiler Engine export
	openLayersHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<title>%s</title>
<meta http-equiv="imagetoolbar" content="no"/>
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=0">
<meta name="apple-mobile-web-app-capable" content="yes">
<style type="text/css">
html, body { margin:0; padding: 0; height: 100%%; width: 100%%; }
body { width:100%%; height:100%%; background: #ffffff; }
#map { position: absolute; height: 100%%; width: 100%%; }
#api-keys-info-panel {color: #946F00; background-color:#FFF7D6; border-radius:0.375rem; position:absolute; width:35rem;  top:3rem; left:50%%; margin-left:-20rem; display:flex; align-items: center; justify-content: center; gap: 1rem;  padding:0 1.5rem; box-sizing: 'content-box;'}
#api-keys-info-panel p {width:100%%; font-size:1rem; }
#api-keys-info-panel p a {color: #946F00; }
#slider { position: absolute; top: 10px; right: 10px; }
</style>
<link rel="stylesheet" href="https://cdn.maptiler.com/ol/v6.12.0/ol.css" type="text/css">
<script src="https://cdnjs.cloudflare.com/polyfill/v2/polyfill.min.js?features=requestAnimationFrame,Element.prototype.classList,URL,fetch,Function.prototype.bind,es5&flags=always,gated&unknown=polyfill" type="text/javascript"></script>
<script src="https://cdn.maptiler.com/ol/v6.12.0/ol.js" type="text/javascript"></script>
</head>
<body>
<div id="map"></div>
<div id='api-keys-info-panel' >
  <svg width="2rem" height="2rem" viewBox="0 0 18 20" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M9 5.8675L14.6475 15.625H3.3525L9 5.8675ZM9 2.875L0.75 17.125H17.25L9 2.875Z" fill="#946F00"/>
    <path d="M9.75 13.375H8.25V14.875H9.75V13.375Z" fill="#946F00"/>
    <path d="M9.75 8.875H8.25V12.625H9.75V8.875Z" fill="#946F00"/>
  </svg>
  <p>Set your API key in the MapTiler Engine settings to display the map correctly.<br/>  Learn how in our <a target='_blank' href='https://docs.maptiler.com/guides/map-tiling-hosting/data-processing/output-preview/'>documentation</a>.
  </p>
  <input id="close-info-panel-btn" type="button" value="Close" />
</div>
<input id="slider" type="range" min="0" max="1" step="0.1" value="1" oninput="layer.setOpacity(parseFloat(this.value))">
<script type="text/javascript">
var mapExtent = ol.proj.transformExtent([%.6f, %.6f, %.6f, %.6f], 'EPSG:4326', 'EPSG:3857');
var mapMinZoom = %d;
var mapMaxZoom = %d;
var layer = new ol.layer.Tile({
  extent: mapExtent,
  source: new ol.source.XYZ({
    attributions: '<a href="https://www.maptiler.com/engine/">Rendered with MapTiler Engine</a>, non-commercial use only',
    url: "{z}/{x}/{y}",
    tilePixelRatio: 1.00000000,
    minZoom: mapMinZoom,
    maxZoom: mapMaxZoom,
  })
});
var mapLayer = new ol.layer.Tile({
  source: new ol.source.OSM(),
});

var map = new ol.Map({
  target: 'map',
  layers: [
    mapLayer,
    layer
  ],
  view: new ol.View({
    center: ol.proj.fromLonLat([%.6f, %.6f]),
    zoom: %d
  })
});
</script>
<script type="text/javascript">
  var panelCloseBtn = document.getElementById('close-info-panel-btn');
  panelCloseBtn.addEventListener("click", () => {document.getElementById("api-keys-info-panel").innerHTML = '';});
</script>
</body>
</html>
`, title, west, south, east, north, minZoom, maxZoom, centerLng, centerLat, initZoom)

	// googlemaps.html: Google Maps ImageMapType template matching MapTiler Engine export
	googleMapsHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<title>%s</title>
<meta name="viewport" content="initial-scale=1.0, user-scalable=no" />
<style type="text/css">
html { overflow: hidden; }
body { overflow: hidden; padding: 0; margin: 0;
width: 100%%; height: 100%%; font-family: Trebuchet MS, Trebuchet, Arial, sans-serif; }
#map { position: absolute; top: 10px; left: 10px; right: 10px; bottom: 15px; overflow: auto; }
#footer { position: absolute; bottom: 0px; left: 0px; width:100%%; height: 12px; overflow: hidden; }
@media screen and (max-width: 600px) {
  #map { top:0px; left:0px; width:100%%; height:100%%;}
}
body { background: #f4f4f4;}
#header { background: #fff; box-shadow: 0 1px 3px #CCC; border: 1px solid #ccc; }
#header h1 { padding:7px 10px; margin:0; font-size: 28px; }
#map { border: 1px solid #ccc; box-shadow: 0 1px 3px #CCC; background-color: #DEDCD7;}
#footer { text-align:center; font-size:9px; color:#606060; }
</style>
<!--[if lte IE 6]>
<style type="text/css">
#map {
    height:expression(document.body.clientHeight-35); /* 10+10+15=35 */
    width:expression(document.body.clientWidth-20); /* 10+10=20 */
}
</style>
<![endif]-->
<script type="text/javascript" src="https://maps.google.com/maps/api/js?sensor=false"></script>
<!-- Get your Google Maps API Key: https://developers.google.com/maps/documentation/javascript/tutorials/adding-a-google-map#introduction-->
<!-- <script type="text/javascript" src="https://maps.google.com/maps/api/js?key=YOUR_API_KEY&sensor=false"></script> -->
<script type="text/javascript">
var map;
var mapBounds = new google.maps.LatLngBounds(
    new google.maps.LatLng(%.6f, %.6f),
    new google.maps.LatLng(%.6f, %.6f));
var mapMinZoom = %d;
var mapMaxZoom = %d;
var maptiler = new google.maps.ImageMapType({
    getTileUrl: function(coord, zoom) { 
        var proj = map.getProjection();
        var z2 = Math.pow(2, zoom);
        var tileXSize = 256 / z2;
        var tileYSize = 256 / z2;
        var tileBounds = new google.maps.LatLngBounds(
            proj.fromPointToLatLng(new google.maps.Point(coord.x * tileXSize, (coord.y + 1) * tileYSize)),
            proj.fromPointToLatLng(new google.maps.Point((coord.x + 1) * tileXSize, coord.y * tileYSize))
        );
        var x = coord.x >= 0 ? coord.x : z2 + coord.x
        var y = coord.y;
        if (mapBounds.intersects(tileBounds) && (mapMinZoom <= zoom) && (zoom <= mapMaxZoom))
            return zoom + "/" + x + "/" + y + "";
        else
            return "https://www.maptiler.com/img/none.png";
    },
    tileSize: new google.maps.Size(256, 256),
    isPng: false,
    name: "Rendered with MapTiler Engine <https://www.maptiler.com/engine/>, non-commercial use only",
    alt: "Rendered with MapTiler Engine",

    opacity: 1.0
});
function init() {
    var opts = {
        tilt:0,
        streetViewControl: false,
        center: new google.maps.LatLng(%.6f, %.6f),
        zoom: %d
    };
    map = new google.maps.Map(document.getElementById("map"), opts);
    map.setMapTypeId('satellite');
    map.overlayMapTypes.insertAt(0, maptiler);
}
</script>
</head>
<body onload="init()">
<div id="footer"><a href="https://www.maptiler.com/engine/">Rendered with MapTiler Engine</a>, non-commercial use only</div>
<div id="map"></div>
</body>
</html>
`, title, south, west, north, east, minZoom, maxZoom, centerLat, centerLng, initZoom)

	return map[string]string{
		"index.html":      indexHTML,
		"leaflet.html":    leafletHTML,
		"openlayers.html": openLayersHTML,
		"googlemaps.html": googleMapsHTML,
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
