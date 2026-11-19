package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

// BaseLayerService manages custom base layers, workspace base layer selection,
// and multiple map views with per-workspace persistence.
type BaseLayerService struct {
	repo        repository.BaseLayerRepo
	sessionRepo *repository.SessionRepository
	mapRepo     repository.MapRepo
	cfg         *config.Config
}

func NewBaseLayerService(
	repo repository.BaseLayerRepo,
	sessionRepo *repository.SessionRepository,
	mapRepo repository.MapRepo,
	cfg *config.Config,
) *BaseLayerService {
	if repo == nil {
		repo = repository.NewMemoryBaseLayerRepository()
	}
	return &BaseLayerService{
		repo:        repo,
		sessionRepo: sessionRepo,
		mapRepo:     mapRepo,
		cfg:         cfg,
	}
}

func (s *BaseLayerService) SetMapRepo(mapRepo repository.MapRepo) {
	s.mapRepo = mapRepo
}

func newBaseLayerID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *BaseLayerService) defaultTileURL() string {
	if s.cfg != nil && strings.TrimSpace(s.cfg.BasemapTileURL) != "" {
		return s.cfg.BasemapTileURL
	}
	return "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
}

func (s *BaseLayerService) defaultAttribution() string {
	if s.cfg != nil && strings.TrimSpace(s.cfg.BasemapAttribution) != "" {
		return s.cfg.BasemapAttribution
	}
	return "© OpenStreetMap contributors"
}

// DefaultWorldMap returns the system-level Default World Map base layer.
func (s *BaseLayerService) DefaultWorldMap() *models.BaseLayer {
	minX, minY, maxX, maxY := -180.0, -85.051129, 180.0, 85.051129
	return &models.BaseLayer{
		ID:          models.BaseLayerTypeWorldMap,
		UserID:      "system",
		Name:        "World Map",
		Description: "Use the default global map.",
		SourceType:  models.BaseLayerTypeWorldMap,
		SourceCRS:   "EPSG:3857",
		ExtentMinX:  &minX,
		ExtentMinY:  &minY,
		ExtentMaxX:  &maxX,
		ExtentMaxY:  &maxY,
		Extent:      []float64{minX, minY, maxX, maxY},
		RenderingSource: map[string]interface{}{
			"mode":        "tiles",
			"tile_url":    s.defaultTileURL(),
			"attribution": s.defaultAttribution(),
			"max_zoom":    19,
		},
		Status:    models.BaseLayerStatusReady,
		IsDefault: true,
	}
}

// ListMapViews returns all configured, legally usable map view styles.
func (s *BaseLayerService) ListMapViews() []models.MapViewDefinition {
	satURL := "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}"
	streetsURL := "https://basemaps.cartocdn.com/rastertiles/voyager_nolabels/{z}/{x}/{y}.png"
	terrainURL := "https://tile.opentopomap.org/{z}/{x}/{y}.png"
	lightURL := "https://basemaps.cartocdn.com/light_nolabels/{z}/{x}/{y}.png"
	darkURL := "https://basemaps.cartocdn.com/dark_nolabels/{z}/{x}/{y}.png"

	if s.cfg != nil {
		if strings.TrimSpace(s.cfg.SatelliteTileURL) != "" {
			satURL = s.cfg.SatelliteTileURL
		}
		if strings.TrimSpace(s.cfg.StreetsTileURL) != "" {
			streetsURL = s.cfg.StreetsTileURL
		}
		if strings.TrimSpace(s.cfg.TerrainTileURL) != "" {
			terrainURL = s.cfg.TerrainTileURL
		}
		if strings.TrimSpace(s.cfg.LightTileURL) != "" {
			lightURL = s.cfg.LightTileURL
		}
		if strings.TrimSpace(s.cfg.DarkTileURL) != "" {
			darkURL = s.cfg.DarkTileURL
		}
	}

	return []models.MapViewDefinition{
		{
			Type:           models.MapViewWorld,
			Name:           "World",
			Description:    "Default OpenStreetMap global geographic reference.",
			TileURL:        s.defaultTileURL(),
			Attribution:    s.defaultAttribution(),
			MinZoom:        0,
			MaxZoom:        19,
			Available:      true,
			Capabilities:   models.MapViewCapabilities{SupportsLabels: false, SupportsRoads: false, SupportsPlaces: false, SupportsOpacity: true},
			PreviewPalette: []string{"#aad3df", "#f2efe9", "#8ec37b", "#f8c967"},
		},
		{
			Type:           models.MapViewSatellite,
			Name:           "Satellite",
			Description:    "High-resolution satellite and aerial imagery with optional road and place overlays.",
			TileURL:        satURL,
			LabelsTileURL:  "https://services.arcgisonline.com/ArcGIS/rest/services/Reference/World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}",
			RoadsTileURL:   "https://services.arcgisonline.com/ArcGIS/rest/services/Reference/World_Transportation/MapServer/tile/{z}/{y}/{x}",
			PlacesTileURL:  "https://services.arcgisonline.com/ArcGIS/rest/services/Reference/World_Boundaries_and_Places/MapServer/tile/{z}/{y}/{x}",
			Attribution:    "Tiles © Esri — Esri, Maxar, Earthstar Geographics, and the GIS User Community",
			MinZoom:        0,
			MaxZoom:        19,
			Available:      strings.TrimSpace(satURL) != "",
			Capabilities:   models.MapViewCapabilities{SupportsLabels: true, SupportsRoads: true, SupportsPlaces: true, SupportsOpacity: true},
			PreviewPalette: []string{"#10211d", "#1d3b2f", "#2f5342", "#6c7a67"},
		},
		{
			Type:           models.MapViewStreets,
			Name:           "Streets",
			Description:    "Clean street map with road networks and toggleable street and place labels.",
			TileURL:        streetsURL,
			LabelsTileURL:  "https://basemaps.cartocdn.com/rastertiles/voyager_only_labels/{z}/{x}/{y}.png",
			PlacesTileURL:  "https://basemaps.cartocdn.com/rastertiles/voyager_only_labels/{z}/{x}/{y}.png",
			Attribution:    "© OpenStreetMap contributors © CARTO",
			MinZoom:        0,
			MaxZoom:        20,
			Available:      strings.TrimSpace(streetsURL) != "",
			Capabilities:   models.MapViewCapabilities{SupportsLabels: true, SupportsRoads: false, SupportsPlaces: true, SupportsOpacity: true},
			PreviewPalette: []string{"#cde5f5", "#f7f5f0", "#f8d47c", "#ffffff"},
		},
		{
			Type:           models.MapViewTerrain,
			Name:           "Terrain",
			Description:    "Topographic elevation contours and shaded relief terrain visualization.",
			TileURL:        terrainURL,
			Attribution:    "Map data: © OpenStreetMap contributors, SRTM | Map style: © OpenTopoMap (CC-BY-SA)",
			MinZoom:        0,
			MaxZoom:        17,
			Available:      strings.TrimSpace(terrainURL) != "",
			Capabilities:   models.MapViewCapabilities{SupportsLabels: false, SupportsRoads: false, SupportsPlaces: false, SupportsOpacity: true},
			PreviewPalette: []string{"#b5d69c", "#e4e0c8", "#c8b598", "#8fb8d6"},
		},
		{
			Type:           models.MapViewLight,
			Name:           "Light",
			Description:    "Minimal light canvas ideal for inspecting user GIS layers and overlays.",
			TileURL:        lightURL,
			LabelsTileURL:  "https://basemaps.cartocdn.com/light_only_labels/{z}/{x}/{y}.png",
			PlacesTileURL:  "https://basemaps.cartocdn.com/light_only_labels/{z}/{x}/{y}.png",
			Attribution:    "© OpenStreetMap contributors © CARTO",
			MinZoom:        0,
			MaxZoom:        20,
			Available:      strings.TrimSpace(lightURL) != "",
			Capabilities:   models.MapViewCapabilities{SupportsLabels: true, SupportsRoads: false, SupportsPlaces: true, SupportsOpacity: true},
			PreviewPalette: []string{"#e6e8eb", "#f5f6f8", "#ffffff", "#cfd4dc"},
		},
		{
			Type:           models.MapViewDark,
			Name:           "Dark",
			Description:    "Dark technical basemap designed for high-contrast spatial analysis.",
			TileURL:        darkURL,
			LabelsTileURL:  "https://basemaps.cartocdn.com/dark_only_labels/{z}/{x}/{y}.png",
			PlacesTileURL:  "https://basemaps.cartocdn.com/dark_only_labels/{z}/{x}/{y}.png",
			Attribution:    "© OpenStreetMap contributors © CARTO",
			MinZoom:        0,
			MaxZoom:        20,
			Available:      strings.TrimSpace(darkURL) != "",
			Capabilities:   models.MapViewCapabilities{SupportsLabels: true, SupportsRoads: false, SupportsPlaces: true, SupportsOpacity: true},
			PreviewPalette: []string{"#14171c", "#1f242d", "#2c3442", "#4b5563"},
		},
		{
			Type:           models.MapViewCustom,
			Name:           "Custom",
			Description:    "Displays the workspace's selected custom geographic base layer.",
			MinZoom:        0,
			MaxZoom:        22,
			Available:      true,
			Capabilities:   models.MapViewCapabilities{SupportsLabels: false, SupportsRoads: false, SupportsPlaces: false, SupportsOpacity: true},
			PreviewPalette: []string{"#11271f", "#1e4b3a", "#3ecf8e", "#24b47e"},
		},
	}
}

func (s *BaseLayerService) GetMapView(viewType string) (*models.MapViewDefinition, error) {
	vt := strings.ToLower(strings.TrimSpace(viewType))
	for _, v := range s.ListMapViews() {
		if v.Type == vt {
			cp := v
			return &cp, nil
		}
	}
	return nil, models.ErrInvalidMapViewType
}

func (s *BaseLayerService) findDataset(datasetID string) *models.Dataset {
	if s.sessionRepo == nil || strings.TrimSpace(datasetID) == "" {
		return nil
	}
	for _, ds := range s.sessionRepo.ListAllDatasets() {
		if ds != nil && ds.ID == datasetID {
			return ds
		}
	}
	return nil
}

func detectSourceType(ds *models.Dataset, requested string) string {
	req := strings.ToLower(strings.TrimSpace(requested))
	switch req {
	case "geotiff", "tiff", "raster", "mbtiles", "geopackage", "vector", "geojson", "shapefile":
		return req
	}
	ext := strings.ToLower(filepath.Ext(ds.Filename))
	switch ext {
	case ".tif", ".tiff":
		return "geotiff"
	case ".mbtiles":
		return "mbtiles"
	case ".gpkg":
		return "geopackage"
	case ".geojson", ".json", ".shp", ".kml", ".dxf":
		return "vector"
	default:
		if ds.MapLayer != nil {
			if t, ok := ds.MapLayer["type"].(string); ok && t != "" {
				return strings.ToLower(t)
			}
		}
		return "raster"
	}
}

func extractCRSFromDataset(ds *models.Dataset) string {
	if ds == nil {
		return ""
	}
	if ds.ExtractedMetadata != nil {
		for _, k := range []string{"crs", "source_crs", "srid", "epsg"} {
			if v, ok := ds.ExtractedMetadata[k]; ok && v != nil {
				str := strings.TrimSpace(fmt.Sprintf("%v", v))
				if str != "" && str != "<nil>" && strings.ToLower(str) != "unknown" && strings.ToLower(str) != "none" {
					if _, err := fmt.Sscanf(str, "%d", new(int)); err == nil && !strings.Contains(strings.ToUpper(str), "EPSG:") {
						return "EPSG:" + str
					}
					return str
				}
			}
		}
		if georef, ok := ds.ExtractedMetadata["georeference"].(map[string]interface{}); ok {
			if tc, ok := georef["target_crs"].(string); ok && strings.TrimSpace(tc) != "" {
				return strings.TrimSpace(tc)
			}
		}
	}
	if ds.MapLayer != nil {
		for _, k := range []string{"bounds_crs", "crs", "source_crs"} {
			if v, ok := ds.MapLayer[k].(string); ok && strings.TrimSpace(v) != "" && strings.ToLower(v) != "unknown" {
				return strings.TrimSpace(v)
			}
		}
	}
	// If georeferencing completed through Veracity Maps pipeline, output is georeferenced in EPSG:4326 or target CRS
	if strings.EqualFold(ds.GeoreferenceStatus, "completed") {
		if ds.MapLayer != nil && ds.MapLayer["bounds"] != nil {
			return "EPSG:4326"
		}
	}
	return ""
}

func parseBoundsInterface(raw interface{}) ([]float64, bool) {
	if raw == nil {
		return nil, false
	}
	switch v := raw.(type) {
	case []float64:
		if len(v) == 4 && allFinite(v) && v[0] < v[2] && v[1] < v[3] {
			return []float64{v[0], v[1], v[2], v[3]}, true
		}
	case [4]float64:
		arr := []float64{v[0], v[1], v[2], v[3]}
		if allFinite(arr) && arr[0] < arr[2] && arr[1] < arr[3] {
			return arr, true
		}
	case []interface{}:
		if len(v) == 4 {
			arr := make([]float64, 4)
			for i, item := range v {
				f, ok := asFloat64(item)
				if !ok {
					return nil, false
				}
				arr[i] = f
			}
			if allFinite(arr) && arr[0] < arr[2] && arr[1] < arr[3] {
				return arr, true
			}
		}
	case map[string]interface{}:
		minX, ok1 := firstFloat(v, "min_longitude", "min_x", "west", "minX", "minLon")
		minY, ok2 := firstFloat(v, "min_latitude", "min_y", "south", "minY", "minLat")
		maxX, ok3 := firstFloat(v, "max_longitude", "max_x", "east", "maxX", "maxLon")
		maxY, ok4 := firstFloat(v, "max_latitude", "max_y", "north", "maxY", "maxLat")
		if ok1 && ok2 && ok3 && ok4 && minX < maxX && minY < maxY {
			return []float64{minX, minY, maxX, maxY}, true
		}
	default:
		// Handle typed Go structs or map[string]float64 via JSON normalization
		if b, err := json.Marshal(raw); err == nil && len(b) > 0 {
			var norm interface{}
			if json.Unmarshal(b, &norm) == nil {
				switch norm.(type) {
				case []interface{}, map[string]interface{}:
					return parseBoundsInterface(norm)
				}
			}
		}
	}
	return nil, false
}

func asFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case float32:
		f := float64(n)
		return f, !math.IsNaN(f) && !math.IsInf(f, 0)
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func firstFloat(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, k := range keys {
		if val, ok := m[k]; ok {
			if f, ok2 := asFloat64(val); ok2 {
				return f, true
			}
		}
	}
	return 0, false
}

func allFinite(vals []float64) bool {
	for _, v := range vals {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func normalizeExtentToWGS84(bounds []float64, sourceCRS string) ([]float64, error) {
	if len(bounds) != 4 {
		return nil, fmt.Errorf("invalid bounds")
	}
	minX, minY, maxX, maxY := bounds[0], bounds[1], bounds[2], bounds[3]
	isGeo := minX >= -180 && maxX <= 180 && minY >= -90 && maxY <= 90
	srid := gis.ParseSRID(sourceCRS)
	if isGeo && (srid == 4326 || srid == 0) {
		return []float64{minX, minY, maxX, maxY}, nil
	}
	if srid != 0 && srid != 4326 {
		corners := [][2]float64{
			{minX, minY},
			{maxX, minY},
			{maxX, maxY},
			{minX, maxY},
		}
		outMinX, outMinY := math.Inf(1), math.Inf(1)
		outMaxX, outMaxY := math.Inf(-1), math.Inf(-1)
		for _, pt := range corners {
			tx, ty, err := gis.TransformCoordPair(pt[0], pt[1], sourceCRS, "EPSG:4326")
			if err != nil {
				return nil, err
			}
			if tx < outMinX {
				outMinX = tx
			}
			if tx > outMaxX {
				outMaxX = tx
			}
			if ty < outMinY {
				outMinY = ty
			}
			if ty > outMaxY {
				outMaxY = ty
			}
		}
		if outMinX >= -180 && outMaxX <= 180 && outMinY >= -90 && outMaxY <= 90 && outMinX < outMaxX && outMinY < outMaxY {
			return []float64{outMinX, outMinY, outMaxX, outMaxY}, nil
		}
	}
	if isGeo {
		return []float64{minX, minY, maxX, maxY}, nil
	}
	return nil, fmt.Errorf("unable to project extent from %s to EPSG:4326", sourceCRS)
}

func extractOverlayCoordinates(ds *models.Dataset, wgs84Extent []float64) [][]float64 {
	// Prefer exact affine 4-corner coordinates when available
	if ds != nil && ds.MapLayer != nil {
		if nativeCoords, ok := ds.MapLayer["coordinates"].([][]float64); ok && len(nativeCoords) == 4 {
			out := make([][]float64, 0, 4)
			for _, pair := range nativeCoords {
				if len(pair) >= 2 && allFinite(pair[:2]) {
					out = append(out, []float64{pair[0], pair[1]})
				}
			}
			if len(out) == 4 {
				return out
			}
		}
		if rawCoords, ok := ds.MapLayer["coordinates"].([]interface{}); ok && len(rawCoords) == 4 {
			out := make([][]float64, 0, 4)
			for _, item := range rawCoords {
				if pair, ok := item.([]interface{}); ok && len(pair) >= 2 {
					x, okX := asFloat64(pair[0])
					y, okY := asFloat64(pair[1])
					if okX && okY {
						out = append(out, []float64{x, y})
					}
				}
			}
			if len(out) == 4 {
				return out
			}
		}
	}
	if len(wgs84Extent) == 4 {
		w, s, e, n := wgs84Extent[0], wgs84Extent[1], wgs84Extent[2], wgs84Extent[3]
		return [][]float64{
			{w, n},
			{e, n},
			{e, s},
			{w, s},
		}
	}
	return nil
}

func inspectDatasetForBaseLayer(ds *models.Dataset, requestedSourceType string) (sourceType, sourceCRS string, wgs84Extent []float64, renderSource map[string]interface{}, previewPath string, issues []string) {
	sourceType = detectSourceType(ds, requestedSourceType)
	sourceCRS = extractCRSFromDataset(ds)

	var rawBounds []float64
	var ok bool
	if ds.MapLayer != nil {
		rawBounds, ok = parseBoundsInterface(ds.MapLayer["bounds"])
	}
	if !ok && ds.ExtractedMetadata != nil {
		if b, found := parseBoundsInterface(ds.ExtractedMetadata["wgs84_bounds"]); found {
			rawBounds, ok = b, true
		} else if b, found := parseBoundsInterface(ds.ExtractedMetadata["bounds"]); found {
			rawBounds, ok = b, true
		}
	}

	isGeorefFlag := strings.EqualFold(ds.GeoreferenceStatus, "completed")
	if ds.ExtractedMetadata != nil {
		if ig, found := ds.ExtractedMetadata["is_georeferenced"].(bool); found && ig {
			isGeorefFlag = true
		}
	}
	if sourceType == "mbtiles" || sourceType == "vector" || sourceType == "geopackage" {
		if len(rawBounds) == 4 && sourceCRS != "" {
			isGeorefFlag = true
		}
	}

	if sourceCRS == "" {
		issues = append(issues, "Dataset does not have a detected CRS. Georeference the dataset or assign its CRS before registering it as a base layer.")
	}
	if !ok || len(rawBounds) != 4 {
		issues = append(issues, "Dataset does not have a valid geographic extent/bounds.")
	}
	if !isGeorefFlag && (sourceType == "raster" || sourceType == "geotiff" || sourceType == "tiff") {
		issues = append(issues, "Raster dataset is not georeferenced. Complete georeferencing before registering it as a base layer.")
	}

	if ok && sourceCRS != "" {
		ext, err := normalizeExtentToWGS84(rawBounds, sourceCRS)
		if err != nil {
			issues = append(issues, fmt.Sprintf("Failed to validate/transform dataset extent from %s: %v", sourceCRS, err))
		} else {
			wgs84Extent = ext
		}
	}

	renderSource = map[string]interface{}{
		"dataset_id":   ds.ID,
		"dataset_name": ds.Filename,
		"session_id":   ds.SessionID,
		"source_type":  sourceType,
		"source_crs":   sourceCRS,
	}

	hasTileTemplate := false
	if ds.MapLayer != nil {
		if tmpl, ok := ds.MapLayer["tile_url_template"].(string); ok && strings.TrimSpace(tmpl) != "" {
			if strings.EqualFold(ds.TileStatus, "completed") || sourceType == "mbtiles" {
				hasTileTemplate = true
				renderSource["mode"] = "tiles"
				renderSource["tile_url_template"] = tmpl
				if zMin, ok := ds.MapLayer["tile_min_zoom"]; ok {
					renderSource["tile_min_zoom"] = zMin
				}
				if zMax, ok := ds.MapLayer["tile_max_zoom"]; ok {
					renderSource["tile_max_zoom"] = zMax
				}
				if ts, ok := ds.MapLayer["tile_size"]; ok {
					renderSource["tile_size"] = ts
				}
			}
		}
	}

	if !hasTileTemplate {
		if sourceType == "vector" || sourceType == "geopackage" || sourceType == "geojson" || sourceType == "shapefile" {
			renderSource["mode"] = "vector"
			renderSource["geojson_url"] = fmt.Sprintf("/datasets/%s/vectors", ds.ID)
		} else {
			renderSource["mode"] = "image"
			renderSource["image_url"] = fmt.Sprintf("/datasets/%s/source-image", ds.ID)
			if coords := extractOverlayCoordinates(ds, wgs84Extent); len(coords) == 4 {
				renderSource["coordinates"] = coords
			}
		}
	}

	if len(wgs84Extent) == 4 {
		renderSource["bounds"] = wgs84Extent
	}

	previewPath = fmt.Sprintf("/datasets/%s/source-image", ds.ID)
	if ds.SessionID != "" {
		previewPath = fmt.Sprintf("/datasets/%s/source-image?session_id=%s", ds.ID, ds.SessionID)
	}
	return sourceType, sourceCRS, wgs84Extent, renderSource, previewPath, issues
}

// RegisterBaseLayer registers an uploaded, georeferenced dataset as a reusable custom base layer.
func (s *BaseLayerService) RegisterBaseLayer(ctx context.Context, userID string, req models.CreateBaseLayerRequest) (*models.BaseLayer, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, models.ErrBaseLayerAccessDenied
	}
	datasetID := strings.TrimSpace(req.DatasetID)
	if datasetID == "" {
		return nil, fmt.Errorf("%w: dataset_id is required", models.ErrInvalidBaseLayerRequest)
	}
	ds := s.findDataset(datasetID)
	if ds == nil {
		return nil, fmt.Errorf("%w: dataset %s not found in active storage", models.ErrBaseLayerNotFound, datasetID)
	}

	sourceType, sourceCRS, wgs84Extent, renderSource, previewPath, issues := inspectDatasetForBaseLayer(ds, req.SourceType)
	if len(issues) > 0 {
		return nil, fmt.Errorf("%w: %s", models.ErrBaseLayerNotGeoref, strings.Join(issues, " "))
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSuffix(ds.Filename, filepath.Ext(ds.Filename))
		if name == "" {
			name = "Custom Base Layer"
		}
	}

	bl := &models.BaseLayer{
		ID:              newBaseLayerID("bl"),
		UserID:          userID,
		Name:            name,
		Description:     strings.TrimSpace(req.Description),
		DatasetID:       ds.ID,
		DatasetName:     ds.Filename,
		SourceType:      sourceType,
		SourceCRS:       sourceCRS,
		Extent:          wgs84Extent,
		PreviewPath:     previewPath,
		RenderingSource: renderSource,
		Status:          models.BaseLayerStatusReady,
		IsDefault:       false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if len(wgs84Extent) == 4 {
		v0, v1, v2, v3 := wgs84Extent[0], wgs84Extent[1], wgs84Extent[2], wgs84Extent[3]
		bl.ExtentMinX = &v0
		bl.ExtentMinY = &v1
		bl.ExtentMaxX = &v2
		bl.ExtentMaxY = &v3
	}

	if err := s.repo.CreateBaseLayer(ctx, bl); err != nil {
		return nil, err
	}
	return bl, nil
}

// ListBaseLayers returns the current user's custom base layers plus the system Default World Map.
func (s *BaseLayerService) ListBaseLayers(ctx context.Context, userID string) ([]*models.BaseLayer, *models.BaseLayer, error) {
	items, err := s.repo.ListUserBaseLayers(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	// Refresh dataset name / live status without failing the list
	for _, bl := range items {
		if bl.DatasetID != "" {
			if ds := s.findDataset(bl.DatasetID); ds != nil {
				if bl.DatasetName == "" {
					bl.DatasetName = ds.Filename
				}
			}
		}
	}
	return items, s.DefaultWorldMap(), nil
}

// GetBaseLayer returns details for a base layer (or the system default world_map).
func (s *BaseLayerService) GetBaseLayer(ctx context.Context, userID, id string) (*models.BaseLayer, error) {
	if id == models.BaseLayerTypeWorldMap || id == "default" {
		return s.DefaultWorldMap(), nil
	}
	bl, err := s.repo.GetBaseLayer(ctx, id)
	if err != nil {
		return nil, err
	}
	if bl.UserID != userID && !bl.IsDefault {
		return nil, models.ErrBaseLayerAccessDenied
	}
	return bl, nil
}

// GetBaseLayerPreview returns preview metadata and thumbnail URL for a base layer.
func (s *BaseLayerService) GetBaseLayerPreview(ctx context.Context, userID, id string) (*models.BaseLayerPreviewResponse, error) {
	bl, err := s.GetBaseLayer(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return &models.BaseLayerPreviewResponse{
		ID:              bl.ID,
		Name:            bl.Name,
		Description:     bl.Description,
		SourceType:      bl.SourceType,
		SourceCRS:       bl.SourceCRS,
		Extent:          bl.Extent,
		PreviewURL:      bl.PreviewPath,
		RenderingSource: bl.RenderingSource,
		Status:          bl.Status,
		IsDefault:       bl.IsDefault,
	}, nil
}

// UpdateBaseLayer updates name and description for a user-owned base layer.
func (s *BaseLayerService) UpdateBaseLayer(ctx context.Context, userID, id string, req models.UpdateBaseLayerRequest) (*models.BaseLayer, error) {
	if id == models.BaseLayerTypeWorldMap {
		return nil, fmt.Errorf("%w: default World Map cannot be modified", models.ErrInvalidBaseLayerRequest)
	}
	bl, err := s.repo.GetBaseLayer(ctx, id)
	if err != nil {
		return nil, err
	}
	if bl.UserID != userID {
		return nil, models.ErrBaseLayerAccessDenied
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		bl.Name = name
	}
	bl.Description = strings.TrimSpace(req.Description)
	if err := s.repo.UpdateBaseLayer(ctx, bl); err != nil {
		return nil, err
	}
	return bl, nil
}

// DeleteBaseLayer deletes only the base layer registration (never deletes shared source dataset).
func (s *BaseLayerService) DeleteBaseLayer(ctx context.Context, userID, id string) error {
	if id == models.BaseLayerTypeWorldMap {
		return fmt.Errorf("%w: default World Map cannot be deleted", models.ErrInvalidBaseLayerRequest)
	}
	bl, err := s.repo.GetBaseLayer(ctx, id)
	if err != nil {
		return err
	}
	if bl.UserID != userID {
		return models.ErrBaseLayerAccessDenied
	}
	return s.repo.DeleteBaseLayer(ctx, id)
}

// ValidateBaseLayer inspects the base layer's source dataset, CRS, extent, and rendering compatibility.
func (s *BaseLayerService) ValidateBaseLayer(ctx context.Context, userID, id string) (*models.BaseLayerValidationResult, error) {
	if id == models.BaseLayerTypeWorldMap {
		wm := s.DefaultWorldMap()
		return &models.BaseLayerValidationResult{
			BaseLayerID:     wm.ID,
			IsValid:         true,
			IsGeoreferenced: true,
			SourceCRS:       wm.SourceCRS,
			Extent:          wm.Extent,
			RenderMode:      "tiles",
			Status:          models.BaseLayerStatusReady,
		}, nil
	}
	bl, err := s.GetBaseLayer(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	res := &models.BaseLayerValidationResult{
		BaseLayerID: bl.ID,
		SourceCRS:   bl.SourceCRS,
		Extent:      bl.Extent,
		Status:      bl.Status,
	}
	if bl.RenderingSource != nil {
		if mode, ok := bl.RenderingSource["mode"].(string); ok {
			res.RenderMode = mode
		}
	}

	if bl.DatasetID != "" {
		ds := s.findDataset(bl.DatasetID)
		if ds != nil {
			sourceType, sourceCRS, wgs84Extent, renderSource, previewPath, issues := inspectDatasetForBaseLayer(ds, bl.SourceType)
			res.SourceCRS = sourceCRS
			res.Extent = wgs84Extent
			res.Issues = issues
			if mode, ok := renderSource["mode"].(string); ok {
				res.RenderMode = mode
			}
			if len(issues) == 0 {
				res.IsValid = true
				res.IsGeoreferenced = true
				res.Status = models.BaseLayerStatusReady
				bl.SourceType = sourceType
				bl.SourceCRS = sourceCRS
				bl.Extent = wgs84Extent
				bl.RenderingSource = renderSource
				bl.PreviewPath = previewPath
				bl.Status = models.BaseLayerStatusReady
				if len(wgs84Extent) == 4 {
					v0, v1, v2, v3 := wgs84Extent[0], wgs84Extent[1], wgs84Extent[2], wgs84Extent[3]
					bl.ExtentMinX, bl.ExtentMinY, bl.ExtentMaxX, bl.ExtentMaxY = &v0, &v1, &v2, &v3
				}
				_ = s.repo.UpdateBaseLayer(ctx, bl)
			} else {
				res.IsValid = false
				res.IsGeoreferenced = false
				res.Status = models.BaseLayerStatusNeedsGeoref
				bl.Status = models.BaseLayerStatusNeedsGeoref
				_ = s.repo.UpdateBaseLayer(ctx, bl)
			}
			return res, nil
		}
	}

	// If session dataset isn't loaded in memory right now, validate stored metadata
	if bl.SourceCRS == "" {
		res.Issues = append(res.Issues, "Missing source CRS")
	}
	if len(bl.Extent) != 4 {
		res.Issues = append(res.Issues, "Missing geographic extent")
	}
	if len(res.Issues) == 0 && bl.RenderingSource != nil {
		res.IsValid = true
		res.IsGeoreferenced = true
		res.Status = models.BaseLayerStatusReady
	} else {
		res.IsValid = false
		res.Status = models.BaseLayerStatusUnavailable
	}
	return res, nil
}

func (s *BaseLayerService) checkWorkspaceAccess(ctx context.Context, workspaceID, userID string, requireWrite bool) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("%w: workspace_id is required", models.ErrInvalidBaseLayerRequest)
	}
	if s.mapRepo == nil {
		return nil
	}
	m, err := s.mapRepo.GetMap(ctx, workspaceID, userID)
	if err != nil {
		if errors.Is(err, models.ErrMapNotFound) {
			// Standalone dataset workspace (not a saved Map record) — authenticated user allowed
			return nil
		}
		return err
	}
	if m != nil && requireWrite {
		if m.OwnerUserID != userID && m.Permission != models.PermissionEditor {
			return models.ErrMapAccessDenied
		}
	}
	return nil
}

// GetWorkspaceBaseLayer retrieves the currently selected base layer for a workspace.
// If a previously saved custom base layer is unavailable, returns a clear warning without silently overwriting.
func (s *BaseLayerService) GetWorkspaceBaseLayer(ctx context.Context, userID, workspaceID string) (*models.WorkspaceBaseLayer, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	wbl, err := s.repo.GetWorkspaceBaseLayer(ctx, workspaceID)
	if err != nil {
		if err == models.ErrBaseLayerNotFound {
			return &models.WorkspaceBaseLayer{
				WorkspaceID:       workspaceID,
				BaseLayerID:       models.BaseLayerTypeWorldMap,
				BaseLayerType:     models.BaseLayerTypeWorldMap,
				SelectedView:      models.MapViewWorld,
				Opacity:           1.0,
				Visibility:        true,
				BaseLayer:         s.DefaultWorldMap(),
				HasSavedSelection: false,
			}, nil
		}
		return nil, err
	}

	wbl.HasSavedSelection = true
	if wbl.BaseLayerType == models.BaseLayerTypeCustom && wbl.BaseLayerID != "" {
		bl, blErr := s.repo.GetBaseLayer(ctx, wbl.BaseLayerID)
		if blErr != nil || bl == nil {
			wbl.PreviousUnavailable = true
			wbl.UnavailableWarning = "The previously saved custom base layer is unavailable or was removed. Please select another custom base layer or switch to the Default World Map."
			wbl.BaseLayer = s.DefaultWorldMap()
		} else {
			wbl.BaseLayer = bl
		}
	} else {
		wbl.BaseLayerType = models.BaseLayerTypeWorldMap
		wbl.BaseLayerID = models.BaseLayerTypeWorldMap
		wbl.BaseLayer = s.DefaultWorldMap()
	}
	return wbl, nil
}

// SelectWorkspaceBaseLayer saves the selected base layer for a workspace.
func (s *BaseLayerService) SelectWorkspaceBaseLayer(ctx context.Context, userID, workspaceID string, req models.SelectWorkspaceBaseLayerRequest) (*models.WorkspaceBaseLayer, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, err
	}

	blType := strings.ToLower(strings.TrimSpace(req.BaseLayerType))
	if blType == "" {
		if req.BaseLayerID != "" && req.BaseLayerID != models.BaseLayerTypeWorldMap {
			blType = models.BaseLayerTypeCustom
		} else {
			blType = models.BaseLayerTypeWorldMap
		}
	}
	if blType != models.BaseLayerTypeWorldMap && blType != models.BaseLayerTypeCustom {
		return nil, fmt.Errorf("%w: base_layer_type must be 'world_map' or 'custom'", models.ErrInvalidBaseLayerRequest)
	}

	var resolvedLayer *models.BaseLayer
	baseLayerID := models.BaseLayerTypeWorldMap
	defaultView := models.MapViewWorld

	if blType == models.BaseLayerTypeCustom {
		if strings.TrimSpace(req.BaseLayerID) == "" {
			return nil, fmt.Errorf("%w: base_layer_id is required for custom base layer", models.ErrInvalidBaseLayerRequest)
		}
		bl, err := s.GetBaseLayer(ctx, userID, strings.TrimSpace(req.BaseLayerID))
		if err != nil {
			return nil, err
		}
		if bl.Status != models.BaseLayerStatusReady {
			return nil, fmt.Errorf("%w: custom base layer is not ready (%s)", models.ErrBaseLayerNotGeoref, bl.Status)
		}
		resolvedLayer = bl
		baseLayerID = bl.ID
		defaultView = models.MapViewCustom
	} else {
		resolvedLayer = s.DefaultWorldMap()
	}

	selectedView := strings.ToLower(strings.TrimSpace(req.SelectedView))
	if selectedView == "" {
		selectedView = defaultView
	}
	if _, err := s.GetMapView(selectedView); err != nil {
		return nil, err
	}

	opacity := 1.0
	if req.Opacity != nil {
		if math.IsNaN(*req.Opacity) || *req.Opacity < 0 || *req.Opacity > 1 {
			return nil, fmt.Errorf("%w: opacity must be between 0 and 1", models.ErrInvalidBaseLayerRequest)
		}
		opacity = *req.Opacity
	} else if existing, err := s.repo.GetWorkspaceBaseLayer(ctx, workspaceID); err == nil && existing != nil {
		opacity = existing.Opacity
	}

	visibility := true
	if req.Visibility != nil {
		visibility = *req.Visibility
	} else if existing, err := s.repo.GetWorkspaceBaseLayer(ctx, workspaceID); err == nil && existing != nil {
		visibility = existing.Visibility
	}

	wbl := &models.WorkspaceBaseLayer{
		ID:                newBaseLayerID("wbl"),
		WorkspaceID:       workspaceID,
		BaseLayerID:       baseLayerID,
		BaseLayerType:     blType,
		SelectedView:      selectedView,
		Opacity:           opacity,
		Visibility:        visibility,
		BaseLayer:         resolvedLayer,
		HasSavedSelection: true,
	}
	if err := s.repo.UpsertWorkspaceBaseLayer(ctx, wbl); err != nil {
		return nil, err
	}

	// Keep map view preferences in sync with the newly chosen base layer view
	prefs, _ := s.GetWorkspaceMapView(ctx, userID, workspaceID)
	if prefs == nil {
		prefs = &models.MapViewPreferences{
			ID:               newBaseLayerID("mvp"),
			WorkspaceID:      workspaceID,
			ShowLabels:       true,
			ShowRoads:        true,
			ShowPlaces:       true,
			BaseLayerOpacity: opacity,
			BaseLayerVisible: visibility,
		}
	}
	prefs.ViewType = selectedView
	prefs.BaseLayerOpacity = opacity
	prefs.BaseLayerVisible = visibility
	_ = s.repo.UpsertMapViewPreferences(ctx, prefs)

	wbl.BaseLayer = resolvedLayer
	return wbl, nil
}

// GetWorkspaceMapView retrieves saved map view & filter preferences for a workspace.
func (s *BaseLayerService) GetWorkspaceMapView(ctx context.Context, userID, workspaceID string) (*models.MapViewPreferences, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	prefs, err := s.repo.GetMapViewPreferences(ctx, workspaceID)
	if err != nil {
		if err == models.ErrBaseLayerNotFound {
			viewType := models.MapViewWorld
			opacity := 1.0
			visible := true
			if wbl, wErr := s.repo.GetWorkspaceBaseLayer(ctx, workspaceID); wErr == nil && wbl != nil {
				if wbl.SelectedView != "" {
					viewType = wbl.SelectedView
				}
				opacity = wbl.Opacity
				visible = wbl.Visibility
			}
			return &models.MapViewPreferences{
				WorkspaceID:      workspaceID,
				ViewType:         viewType,
				ShowLabels:       true,
				ShowRoads:        true,
				ShowPlaces:       true,
				BaseLayerOpacity: opacity,
				BaseLayerVisible: visible,
			}, nil
		}
		return nil, err
	}
	return prefs, nil
}

// UpdateWorkspaceMapView validates and saves map view & filter preferences for a workspace.
func (s *BaseLayerService) UpdateWorkspaceMapView(ctx context.Context, userID, workspaceID string, req models.UpdateMapViewRequest) (*models.MapViewPreferences, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, err
	}
	existing, _ := s.GetWorkspaceMapView(ctx, userID, workspaceID)
	if existing == nil {
		existing = &models.MapViewPreferences{
			ID:               newBaseLayerID("mvp"),
			WorkspaceID:      workspaceID,
			ViewType:         models.MapViewWorld,
			ShowLabels:       true,
			ShowRoads:        true,
			ShowPlaces:       true,
			BaseLayerOpacity: 1.0,
			BaseLayerVisible: true,
		}
	}
	if existing.ID == "" {
		existing.ID = newBaseLayerID("mvp")
	}

	if vt := strings.ToLower(strings.TrimSpace(req.ViewType)); vt != "" {
		if _, err := s.GetMapView(vt); err != nil {
			return nil, err
		}
		existing.ViewType = vt
	}
	if req.ShowLabels != nil {
		existing.ShowLabels = *req.ShowLabels
	}
	if req.ShowRoads != nil {
		existing.ShowRoads = *req.ShowRoads
	}
	if req.ShowPlaces != nil {
		existing.ShowPlaces = *req.ShowPlaces
	}
	if req.Opacity != nil {
		if math.IsNaN(*req.Opacity) || *req.Opacity < 0 || *req.Opacity > 1 {
			return nil, fmt.Errorf("%w: opacity must be between 0 and 1", models.ErrInvalidBaseLayerRequest)
		}
		existing.BaseLayerOpacity = *req.Opacity
	}
	if req.BaseLayerVisible != nil {
		existing.BaseLayerVisible = *req.BaseLayerVisible
	}
	// Sanitize style_configuration: disallow arbitrary external tile URLs or raw style injection
	if req.StyleConfiguration != nil {
		sanitized := make(map[string]interface{})
		for k, v := range req.StyleConfiguration {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "url") || strings.Contains(lk, "tiles") || strings.Contains(lk, "script") || strings.Contains(lk, "source") {
				return nil, fmt.Errorf("%w: custom tile URLs or raw style sources are not permitted in style_configuration", models.ErrInvalidBaseLayerRequest)
			}
			sanitized[k] = v
		}
		existing.StyleConfiguration = sanitized
	}

	if err := s.repo.UpsertMapViewPreferences(ctx, existing); err != nil {
		return nil, err
	}

	// Sync opacity/visibility/selected_view back to workspace_base_layers if present
	if wbl, err := s.repo.GetWorkspaceBaseLayer(ctx, workspaceID); err == nil && wbl != nil {
		wbl.SelectedView = existing.ViewType
		wbl.Opacity = existing.BaseLayerOpacity
		wbl.Visibility = existing.BaseLayerVisible
		_ = s.repo.UpsertWorkspaceBaseLayer(ctx, wbl)
	}

	return existing, nil
}
