package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

var dynamicLayerPalette = []string{
	"#3ecf8e",
	"#3b82f6",
	"#f59e0b",
	"#ec4899",
	"#8b5cf6",
	"#06b6d4",
	"#ef4444",
	"#84cc16",
	"#f97316",
	"#14b8a6",
}

// WorkspaceRegistryService orchestrates dynamic multi-layer GIS dataset inspection,
// selective layer import (GeoPackage, Shapefile, GeoJSON, KML, OSM, DXF, Rasters),
// centralized Workspace Records, feature querying, and raster management.
type WorkspaceRegistryService struct {
	repo        repository.WorkspaceRegistryRepo
	gisService  *GISService
	sessionRepo *repository.SessionRepository
	mapRepo     repository.MapRepo
}

func NewWorkspaceRegistryService(
	repo repository.WorkspaceRegistryRepo,
	gisService *GISService,
	sessionRepo *repository.SessionRepository,
	mapRepo repository.MapRepo,
) *WorkspaceRegistryService {
	if repo == nil {
		repo = repository.NewMemoryWorkspaceRegistryRepository()
	}
	return &WorkspaceRegistryService{
		repo:        repo,
		gisService:  gisService,
		sessionRepo: sessionRepo,
		mapRepo:     mapRepo,
	}
}

func (s *WorkspaceRegistryService) SetMapRepo(mapRepo repository.MapRepo) {
	s.mapRepo = mapRepo
}

func (s *WorkspaceRegistryService) SetGISService(gisService *GISService) {
	s.gisService = gisService
}

func newRegistryID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *WorkspaceRegistryService) checkWorkspaceAccess(ctx context.Context, workspaceID, userID string, requireWrite bool) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("%w: workspace_id is required", models.ErrInvalidWorkspaceRequest)
	}
	if s.mapRepo == nil || strings.HasPrefix(workspaceID, "ds_") {
		return nil
	}
	m, err := s.mapRepo.GetMap(ctx, workspaceID, userID)
	if err != nil {
		if errors.Is(err, models.ErrMapNotFound) {
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

func (s *WorkspaceRegistryService) findDataset(datasetID string) *models.Dataset {
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

func isVectorContainerExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".gpkg", ".shp", ".geojson", ".json", ".kml", ".osm", ".dxf", ".gml", ".fgb":
		return true
	}
	return false
}

func isRasterExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".tif", ".tiff", ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".mbtiles", ".pdf":
		return true
	}
	return false
}

// ============================================================================
// 1. DYNAMIC DATASET INSPECTION (GeoPackage, Shapefile, GeoJSON, Rasters, etc.)
// ============================================================================

type DatasetInspectionResponse struct {
	DatasetID       string                    `json:"dataset_id"`
	Filename        string                    `json:"filename"`
	Format          string                    `json:"format"`
	Driver          string                    `json:"driver"`
	FileSize        int64                     `json:"file_size"`
	IsMultiLayer    bool                      `json:"is_multi_layer"`
	IsGeoreferenced bool                      `json:"is_georeferenced"`
	CRS             string                    `json:"crs,omitempty"`
	Bounds          []float64                 `json:"bounds,omitempty"`
	Layers          []models.DatasetLayerInfo `json:"layers"`
	Metadata        map[string]interface{}    `json:"metadata,omitempty"`
}

func (s *WorkspaceRegistryService) InspectDataset(ctx context.Context, userID, datasetID string) (*DatasetInspectionResponse, error) {
	ds := s.findDataset(datasetID)
	if ds == nil {
		return nil, fmt.Errorf("%w: dataset %s not found", models.ErrWorkspaceLayerNotFound, datasetID)
	}
	ext := strings.ToLower(filepath.Ext(ds.Filename))
	format := strings.TrimPrefix(ext, ".")
	if format == "" {
		format = "unknown"
	}

	var layers []models.DatasetLayerInfo
	driver := strings.ToUpper(format)

	if isVectorContainerExt(ext) || ext == ".zip" {
		if ds.StoragePath != "" {
			inspected, drv, err := gis.InspectVectorDatasetFile(ds.StoragePath)
			if err != nil && ext == ".gpkg" {
				return nil, fmt.Errorf("scan GeoPackage: %w", err)
			}
			if err == nil && len(inspected) > 0 {
				layers = inspected
				if drv != "" {
					driver = drv
				}
			}
		}
		// Fallback to any layers already extracted during upload
		if len(layers) == 0 && ds.ExtractedMetadata != nil {
			if raw, ok := ds.ExtractedMetadata["layers"]; ok {
				if b, err := json.Marshal(raw); err == nil {
					_ = json.Unmarshal(b, &layers)
				}
			}
		}
	}

	// Raster dataset inspection fallback (GeoTIFF, TIFF, PNG/JPG, MBTiles, PDF)
	if len(layers) == 0 && (isRasterExt(ext) || ds.MapLayer != nil) {
		crs := extractCRSFromDataset(ds)
		var bounds []float64
		if ds.MapLayer != nil {
			if b, ok := parseBoundsInterface(ds.MapLayer["bounds"]); ok {
				bounds = b
			}
		}
		if len(bounds) != 4 && ds.ExtractedMetadata != nil {
			if b, ok := parseBoundsInterface(ds.ExtractedMetadata["bounds"]); ok {
				bounds = b
			}
		}
		layers = []models.DatasetLayerInfo{
			{
				DatasetLayerID: fmt.Sprintf("dsl_raster_%s", ds.ID),
				DatasetID:      ds.ID,
				LayerName:      strings.TrimSuffix(ds.Filename, filepath.Ext(ds.Filename)),
				GeometryType:   "Raster",
				CRS:            crs,
				FeatureCount:   1,
				Bounds:         bounds,
				Fields:         []map[string]interface{}{},
				CreatedAt:      time.Now().UTC(),
			},
		}
	}

	// Attach DatasetID and cache in GISRepo if available
	for i := range layers {
		layers[i].DatasetID = ds.ID
	}
	if s.gisService != nil && s.gisService.GetGISRepo() != nil && len(layers) > 0 {
		_ = s.gisService.GetGISRepo().SaveDatasetLayers(ctx, ds.ID, layers)
	}

	var overallBounds []float64
	overallCRS := extractCRSFromDataset(ds)
	for _, l := range layers {
		if overallCRS == "" && l.CRS != "" {
			overallCRS = l.CRS
		}
		if len(l.Bounds) == 4 {
			if len(overallBounds) != 4 {
				overallBounds = append([]float64(nil), l.Bounds...)
			} else {
				overallBounds[0] = math.Min(overallBounds[0], l.Bounds[0])
				overallBounds[1] = math.Min(overallBounds[1], l.Bounds[1])
				overallBounds[2] = math.Max(overallBounds[2], l.Bounds[2])
				overallBounds[3] = math.Max(overallBounds[3], l.Bounds[3])
			}
		}
	}

	isGeo := strings.EqualFold(ds.GeoreferenceStatus, "completed") || (overallCRS != "" && len(overallBounds) == 4)

	return &DatasetInspectionResponse{
		DatasetID:       ds.ID,
		Filename:        ds.Filename,
		Format:          format,
		Driver:          driver,
		FileSize:        ds.FileSize,
		IsMultiLayer:    len(layers) > 1,
		IsGeoreferenced: isGeo,
		CRS:             overallCRS,
		Bounds:          overallBounds,
		Layers:          layers,
		Metadata:        ds.ExtractedMetadata,
	}, nil
}

// ============================================================================
// 2. SELECTIVE MULTI-LAYER IMPORT & INDEPENDENT LAYER MANAGEMENT
// ============================================================================

func defaultStyleForGeometry(geomType, color string) map[string]interface{} {
	gt := strings.ToLower(geomType)
	if strings.Contains(gt, "point") {
		return map[string]interface{}{
			"mode":          "single",
			"fill_color":    color,
			"stroke_color":  "#ffffff",
			"stroke_width":  2.0,
			"point_radius":  7.0,
			"opacity":       0.95,
			"label_enabled": false,
		}
	}
	if strings.Contains(gt, "line") {
		return map[string]interface{}{
			"mode":         "single",
			"stroke_color": color,
			"stroke_width": 2.5,
			"opacity":      0.95,
		}
	}
	return map[string]interface{}{
		"mode":         "single",
		"fill_color":   color,
		"fill_opacity": 0.28,
		"stroke_color": color,
		"stroke_width": 1.8,
		"opacity":      0.9,
	}
}

func (s *WorkspaceRegistryService) AddDatasetLayers(ctx context.Context, userID, workspaceID string, req models.AddDatasetLayersRequest) ([]*models.WorkspaceLayer, *models.WorkspaceDataset, error) {
	if workspaceID == "" {
		workspaceID = req.WorkspaceID
	}
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, nil, err
	}
	dsID := strings.TrimSpace(req.DatasetID)
	if dsID == "" {
		return nil, nil, fmt.Errorf("%w: dataset_id is required", models.ErrInvalidWorkspaceRequest)
	}
	ds := s.findDataset(dsID)
	if ds == nil {
		return nil, nil, fmt.Errorf("%w: dataset %s not found", models.ErrWorkspaceLayerNotFound, dsID)
	}

	inspection, err := s.InspectDataset(ctx, userID, dsID)
	if err != nil {
		return nil, nil, err
	}
	if len(inspection.Layers) == 0 {
		return nil, nil, fmt.Errorf("%w: no importable layers detected in dataset", models.ErrInvalidWorkspaceRequest)
	}

	// Ensure WorkspaceDataset record exists (without duplicating source file)
	wsDS, err := s.repo.GetWorkspaceDatasetBySource(ctx, workspaceID, ds.ID)
	if err != nil || wsDS == nil {
		wsDS = &models.WorkspaceDataset{
			ID:               newRegistryID("wds"),
			WorkspaceID:      workspaceID,
			UserID:           userID,
			SourceDatasetID:  ds.ID,
			OriginalFilename: ds.Filename,
			StoredFilePath:   ds.StoragePath,
			Format:           inspection.Format,
			FileSize:         ds.FileSize,
			UploadStatus:     "ready",
			CreatedAt:        time.Now().UTC(),
		}
		if createErr := s.repo.CreateWorkspaceDataset(ctx, wsDS); createErr != nil {
			return nil, nil, createErr
		}
	}

	// Filter to selected layer names (if provided); otherwise import all detected layers
	selectedSet := make(map[string]bool)
	for _, n := range req.SelectedLayerNames {
		trimmed := strings.TrimSpace(n)
		if trimmed != "" {
			selectedSet[trimmed] = true
		}
	}

	// Use a detached timeout context for multi-layer persistence so a transient client disconnect cannot truncate a GeoPackage import.
	importCtx, cancelImport := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancelImport()

	existingLayers, _ := s.repo.ListWorkspaceLayers(importCtx, workspaceID)
	existingByName := make(map[string]*models.WorkspaceLayer)
	for _, el := range existingLayers {
		if el.SourceDatasetID == ds.ID {
			existingByName[el.SourceLayerName] = el
		}
	}
	// Automatically convert GeoPackage (and other vector container) layers to local XYZ MVT/PBF vector tiles,
	// reusing any vector_tiles already generated at upload time. Run concurrently with feature extraction so
	// large GeoPackages (25k+ features) never stall HTTP requests.
	vectorTileResultsByLayer := make(map[string]gis.VectorTileLayerResult)
	if ds.ExtractedMetadata != nil {
		if existingVT, ok := ds.ExtractedMetadata["vector_tiles"].(map[string]interface{}); ok {
			for _, rawItem := range existingVT {
				if b, mErr := json.Marshal(rawItem); mErr == nil {
					var lres gis.VectorTileLayerResult
					if json.Unmarshal(b, &lres) == nil && lres.LayerSlug != "" && lres.TileCount > 0 {
						vectorTileResultsByLayer[lres.LayerName] = lres
						vectorTileResultsByLayer[lres.LayerSlug] = lres
					}
				}
			}
		}
	}
	extLower := strings.ToLower(filepath.Ext(ds.Filename))
	var vtDoneCh chan *gis.ConvertVectorTilesResult
	var vtRoot string
	totalVecFeatures := 0
	if ds.StoragePath != "" && (extLower == ".gpkg" || extLower == ".shp" || extLower == ".geojson" || extLower == ".kml" || extLower == ".gml" || extLower == ".fgb") {
		var vecLayerNames []string
		for _, info := range inspection.Layers {
			if len(selectedSet) > 0 && !selectedSet[info.LayerName] {
				continue
			}
			if !strings.EqualFold(info.GeometryType, "Raster") && !strings.EqualFold(info.GeometryType, "Table") {
				totalVecFeatures += info.FeatureCount
				if _, already := vectorTileResultsByLayer[info.LayerName]; !already {
					vecLayerNames = append(vecLayerNames, info.LayerName)
				}
			}
		}
		if len(vecLayerNames) > 0 {
			vtRoot = filepath.Join(filepath.Dir(ds.StoragePath), "vector_tiles")
			vtDoneCh = make(chan *gis.ConvertVectorTilesResult, 1)
			go func(srcPath, root string, names []string) {
				vtRes, vtErr := gis.ConvertDatasetToVectorTiles(srcPath, root, names, 0, 14)
				if vtErr == nil && vtRes != nil {
					vtDoneCh <- vtRes
				} else {
					vtDoneCh <- nil
				}
			}(ds.StoragePath, vtRoot, vecLayerNames)
			// For normal datasets (<= 5000 features), wait immediately so metadata is attached inline
			if totalVecFeatures <= 5000 {
				if vtRes := <-vtDoneCh; vtRes != nil {
					applyConvertedVectorTilesToDataset(ds, vtRoot, vtRes, vectorTileResultsByLayer)
				}
				vtDoneCh = nil
			}
		}
	}

	nextOrder := len(existingLayers)
	added := make([]*models.WorkspaceLayer, 0)

	for idx, info := range inspection.Layers {
		if len(selectedSet) > 0 && !selectedSet[info.LayerName] {
			continue
		}

		if existingLyr, exists := existingByName[info.LayerName]; exists && existingLyr.FeatureCount >= info.FeatureCount && info.FeatureCount > 0 {
			added = append(added, existingLyr)
			continue
		}

		color := dynamicLayerPalette[(nextOrder+idx)%len(dynamicLayerPalette)]
		isRaster := strings.EqualFold(info.GeometryType, "Raster")
		isTable := strings.EqualFold(info.GeometryType, "Table")
		gisLayerID := ""
		featureCount := info.FeatureCount
		layerBounds := info.Bounds

		if isRaster {
			// Register as raster layer in GIS layer manager & workspace_rasters
			if s.gisService != nil {
				createdLyr, cErr := s.gisService.CreateLayer(ctx, userID, models.CreateLayerRequest{
					MapID:        workspaceID,
					DatasetID:    ds.ID,
					LayerName:    info.LayerName,
					LayerType:    "raster",
					GeometryType: "Raster",
					CRS:          info.CRS,
				})
				if cErr == nil && createdLyr != nil {
					gisLayerID = createdLyr.LayerID
				}
			}

			wl := &models.WorkspaceLayer{
				ID:                 newRegistryID("wl"),
				WorkspaceDatasetID: wsDS.ID,
				SourceDatasetID:    ds.ID,
				WorkspaceID:        workspaceID,
				GISLayerID:         gisLayerID,
				SourceLayerName:    info.LayerName,
				DisplayName:        info.LayerName,
				GeometryType:       "Raster",
				SourceCRS:          info.CRS,
				FeatureCount:       1,
				Extent:             layerBounds,
				Fields:             info.Fields,
				RenderingSource: map[string]interface{}{
					"type":              "raster",
					"source_dataset_id": ds.ID,
					"session_id":        ds.SessionID,
					"image_url":         fmt.Sprintf("/datasets/%s/source-image", ds.ID),
				},
				Visibility: true,
				Opacity:    1.0,
				LayerOrder: nextOrder,
				Status:     "active",
			}
			setWorkspaceLayerExtentPointers(wl, layerBounds)
			if err := s.repo.CreateWorkspaceLayer(ctx, wl); err != nil {
				return nil, nil, err
			}

			// Register raster metadata in workspace_rasters
			w, h, bands, rx, ry := extractRasterDims(ds)
			rst := &models.WorkspaceRaster{
				ID:               newRegistryID("wrst"),
				WorkspaceID:      workspaceID,
				WorkspaceLayerID: wl.ID,
				DatasetID:        ds.ID,
				DisplayName:      info.LayerName,
				LayerName:        info.LayerName,
				RasterPath:       ds.StoragePath,
				Width:            w,
				Height:           h,
				BandCount:        bands,
				SourceCRS:        info.CRS,
				ResolutionX:      rx,
				ResolutionY:      ry,
				Bounds:           layerBounds,
				Visibility:       true,
				Opacity:          1.0,
				Status:           rasterStatusString(ds),
			}
			_ = s.repo.UpsertWorkspaceRaster(ctx, rst)
			added = append(added, wl)
			nextOrder++
			continue
		}

		// Vector or attribute table layer: extract features dynamically via GDAL/OGR
		var extracted *gis.ExtractedLayerResult
		if ds.StoragePath != "" {
			extRes, extErr := gis.ExtractDatasetLayerFeatures(ds.StoragePath, info.LayerName, 25000, 0)
			if extErr != nil {
				return nil, nil, fmt.Errorf("extract layer %s: %w", info.LayerName, extErr)
			}
			// Read through EOF, including points beyond the first extraction page.
			if extRes != nil {
				pageSize := len(extRes.Features)
				for pageSize == 25000 {
					if err := importCtx.Err(); err != nil {
						return nil, nil, err
					}
					page, err := gis.ExtractDatasetLayerFeatures(ds.StoragePath, info.LayerName, 25000, len(extRes.Features))
					if err != nil {
						return nil, nil, fmt.Errorf("extract layer %s: %w", info.LayerName, err)
					}
					pageSize = len(page.Features)
					extRes.Features = append(extRes.Features, page.Features...)
					if len(page.Bounds) == 4 {
						if len(extRes.Bounds) != 4 {
							extRes.Bounds = page.Bounds
						} else {
							extRes.Bounds[0] = math.Min(extRes.Bounds[0], page.Bounds[0])
							extRes.Bounds[1] = math.Min(extRes.Bounds[1], page.Bounds[1])
							extRes.Bounds[2] = math.Max(extRes.Bounds[2], page.Bounds[2])
							extRes.Bounds[3] = math.Max(extRes.Bounds[3], page.Bounds[3])
						}
					}
				}
			}
			if extRes != nil {
				extracted = extRes
				if len(extRes.Bounds) == 4 {
					layerBounds = extRes.Bounds
				}
				if extRes.TotalFeatures > 0 {
					featureCount = extRes.TotalFeatures
				}
				// If a non-spatial attribute table yielded synthesized Point geometries (e.g. lat/lon columns), promote it
				if isTable && strings.Contains(strings.ToLower(extRes.GeometryType), "point") {
					isTable = false
					info.GeometryType = extRes.GeometryType
					if info.CRS == "" {
						info.CRS = "EPSG:4326"
					}
				}
			}
		}

		var vtMeta map[string]interface{}
		if vtInfo, hasVT := vectorTileResultsByLayer[info.LayerName]; hasVT {
			vtMeta = map[string]interface{}{
				"type":              "vector",
				"format":            "pbf",
				"tile_scheme":       "xyz",
				"layer_slug":        vtInfo.LayerSlug,
				"layer_name":        vtInfo.LayerName,
				"source_layer":      vtInfo.SourceLayer,
				"dataset_id":        ds.ID,
				"source_crs":        vtInfo.SourceCRS,
				"tile_crs":          vtInfo.TileCRS,
				"geometry_type":     vtInfo.GeometryType,
				"geometry_types":    vtInfo.GeometryTypes,
				"feature_count":     vtInfo.FeatureCount,
				"tile_count":        vtInfo.TileCount,
				"bounds":            vtInfo.Bounds,
				"center":            vtInfo.Center,
				"min_zoom":          vtInfo.MinZoom,
				"max_zoom":          vtInfo.MaxZoom,
				"fields":            vtInfo.Fields,
				"layer_dir":         vtInfo.LayerDir,
				"tiles_dir":         vtInfo.TilesDir,
				"metadata_path":     vtInfo.MetadataPath,
				"style_path":        vtInfo.StylePath,
				"tile_url_template": fmt.Sprintf("/datasets/%s/vector-tiles/%s/{z}/{x}/{y}.pbf", ds.ID, vtInfo.LayerSlug),
				"metadata_url":      fmt.Sprintf("/datasets/%s/vector-tiles/%s/metadata.json", ds.ID, vtInfo.LayerSlug),
				"style_url":         fmt.Sprintf("/datasets/%s/vector-tiles/%s/style.json", ds.ID, vtInfo.LayerSlug),
				"promote_id":        "feature_id",
				"interactive":       true,
			}
			if len(vtInfo.Bounds) == 4 {
				layerBounds = vtInfo.Bounds
			}
			if vtInfo.FeatureCount > 0 {
				featureCount = vtInfo.FeatureCount
			}
		}

		if s.gisService != nil && !isTable {
			layerMeta := map[string]interface{}{
				"source_dataset_id": ds.ID,
				"source_layer_name": info.LayerName,
				"source_crs":        firstNonEmptyStr(info.CRS, "EPSG:4326"),
			}
			if vtMeta != nil {
				layerMeta["vector_tiles"] = vtMeta
			}
			createdLyr, cErr := s.gisService.CreateLayer(importCtx, userID, models.CreateLayerRequest{
				MapID:        workspaceID,
				DatasetID:    ds.ID,
				LayerName:    info.LayerName,
				LayerType:    "vector",
				GeometryType: info.GeometryType,
				CRS:          firstNonEmptyStr(info.CRS, "EPSG:4326"),
				Bounds:       layerBounds,
				Metadata:     layerMeta,
			})
			if cErr != nil {
				return nil, nil, cErr
			}
			if createdLyr != nil {
				gisLayerID = createdLyr.LayerID
				// Apply distinct default styling per layer
				_, _ = s.gisService.UpdateLayerStyle(importCtx, gisLayerID, userID, models.UpdateStyleRequest{
					StyleType:   "single",
					StyleConfig: defaultStyleForGeometry(info.GeometryType, color),
				})
				gisRepo := s.gisService.GetGISRepo()
				if gisRepo != nil {
					// Register discovered attribute fields
					for _, fld := range info.Fields {
						fname, _ := fld["name"].(string)
						ftype, _ := fld["type"].(string)
						if fname != "" && models.ValidateFieldName(fname) {
							_, _ = gisRepo.AddLayerField(importCtx, gisLayerID, models.AddFieldRequest{
								FieldName: fname,
								FieldType: normalizeGISFieldType(ftype),
							})
						}
					}
					// Insert extracted features into GIS feature store in batch (reprojected to EPSG:4326 for MapLibre)
					if extracted != nil && len(extracted.Features) > 0 {
						batchItems := make([]models.BatchFeatureItem, 0, len(extracted.Features))
						for _, f := range extracted.Features {
							if f.Geometry == nil {
								continue
							}
							props := make(map[string]interface{})
							for k, v := range f.Properties {
								props[k] = v
							}
							props["_source_feature_id"] = f.FeatureID
							props["_source_layer"] = info.LayerName
							props["_source_dataset_id"] = ds.ID
							valRes, _ := gis.ValidateGeoJSONGeometry(*f.Geometry, "EPSG:4326")
							wkt := ""
							var bbox []float64
							if valRes != nil {
								wkt = valRes.WKT
								bbox = valRes.BBox
							}
							batchItems = append(batchItems, models.BatchFeatureItem{
								Request: models.CreateFeatureRequest{
									LayerID:      gisLayerID,
									Geometry:     *f.Geometry,
									GeometryType: f.Geometry.Type,
									CRS:          "EPSG:4326",
									Properties:   props,
								},
								WKT:  wkt,
								SRID: 4326,
								BBox: bbox,
							})
						}
						if len(batchItems) > 0 {
							created, err := gisRepo.CreateFeatureBatch(importCtx, userID, batchItems)
							if err != nil || len(created) != len(batchItems) {
								_ = gisRepo.DeleteLayer(importCtx, gisLayerID)
								if err != nil {
									return nil, nil, fmt.Errorf("import layer %s: %w", info.LayerName, err)
								}
								return nil, nil, fmt.Errorf("import layer %s: stored %d of %d features", info.LayerName, len(created), len(batchItems))
							}
						}
					}
				}
			}
		}

		renderingSrc := map[string]interface{}{
			"type":              "vector",
			"source_dataset_id": ds.ID,
			"gis_layer_id":      gisLayerID,
			"color":             color,
		}
		if vtMeta != nil {
			renderingSrc["vector_tiles"] = vtMeta
			renderingSrc["tile_url_template"] = vtMeta["tile_url_template"]
			renderingSrc["source_layer"] = vtMeta["source_layer"]
			renderingSrc["layer_slug"] = vtMeta["layer_slug"]
		}

		wl := &models.WorkspaceLayer{
			ID:                 newRegistryID("wl"),
			WorkspaceDatasetID: wsDS.ID,
			SourceDatasetID:    ds.ID,
			WorkspaceID:        workspaceID,
			GISLayerID:         gisLayerID,
			SourceLayerName:    info.LayerName,
			DisplayName:        info.LayerName,
			GeometryType:       info.GeometryType,
			SourceCRS:          info.CRS,
			FeatureCount:       featureCount,
			Extent:             layerBounds,
			Fields:             info.Fields,
			RenderingSource:    renderingSrc,
			Visibility:         true,
			Opacity:            1.0,
			LayerOrder:         nextOrder,
			Status:             "active",
		}
		setWorkspaceLayerExtentPointers(wl, layerBounds)
		if err := s.repo.CreateWorkspaceLayer(importCtx, wl); err != nil {
			return nil, nil, err
		}
		added = append(added, wl)
		nextOrder++
	}

	// If large-dataset vector tile conversion ran concurrently, attach results if already ready (or finish in background)
	if vtDoneCh != nil {
		attachVT := func(vtRes *gis.ConvertVectorTilesResult) {
			if vtRes == nil {
				return
			}
			byName := make(map[string]gis.VectorTileLayerResult)
			applyConvertedVectorTilesToDataset(ds, vtRoot, vtRes, byName)
			for _, wl := range added {
				if lres, ok := byName[wl.SourceLayerName]; ok {
					vtMeta := buildVectorTileLayerMeta(ds.ID, lres)
					if wl.RenderingSource == nil {
						wl.RenderingSource = make(map[string]interface{})
					}
					wl.RenderingSource["vector_tiles"] = vtMeta
					wl.RenderingSource["tile_url_template"] = vtMeta["tile_url_template"]
					wl.RenderingSource["source_layer"] = vtMeta["source_layer"]
					wl.RenderingSource["layer_slug"] = vtMeta["layer_slug"]
					_ = s.repo.UpdateWorkspaceLayer(context.Background(), wl)
					if wl.GISLayerID != "" && s.gisService != nil && s.gisService.GetGISRepo() != nil {
						if gl, gErr := s.gisService.GetGISRepo().GetLayer(context.Background(), wl.GISLayerID); gErr == nil && gl != nil {
							if gl.Metadata == nil {
								gl.Metadata = make(map[string]interface{})
							}
							gl.Metadata["vector_tiles"] = vtMeta
						}
					}
				}
			}
		}
		select {
		case vtRes := <-vtDoneCh:
			attachVT(vtRes)
		default:
			go func() {
				attachVT(<-vtDoneCh)
			}()
		}
	}

	return added, wsDS, nil
}

func buildVectorTileLayerMeta(datasetID string, lres gis.VectorTileLayerResult) map[string]interface{} {
	return map[string]interface{}{
		"type":              "vector",
		"format":            "pbf",
		"tile_scheme":       "xyz",
		"layer_slug":        lres.LayerSlug,
		"layer_name":        lres.LayerName,
		"source_layer":      lres.SourceLayer,
		"dataset_id":        datasetID,
		"source_crs":        lres.SourceCRS,
		"tile_crs":          lres.TileCRS,
		"geometry_type":     lres.GeometryType,
		"geometry_types":    lres.GeometryTypes,
		"feature_count":     lres.FeatureCount,
		"tile_count":        lres.TileCount,
		"bounds":            lres.Bounds,
		"center":            lres.Center,
		"min_zoom":          lres.MinZoom,
		"max_zoom":          lres.MaxZoom,
		"fields":            lres.Fields,
		"layer_dir":         lres.LayerDir,
		"tiles_dir":         lres.TilesDir,
		"metadata_path":     lres.MetadataPath,
		"style_path":        lres.StylePath,
		"tile_url_template": fmt.Sprintf("/datasets/%s/vector-tiles/%s/{z}/{x}/{y}.pbf", datasetID, lres.LayerSlug),
		"metadata_url":      fmt.Sprintf("/datasets/%s/vector-tiles/%s/metadata.json", datasetID, lres.LayerSlug),
		"style_url":         fmt.Sprintf("/datasets/%s/vector-tiles/%s/style.json", datasetID, lres.LayerSlug),
		"promote_id":        "feature_id",
		"interactive":       true,
	}
}

func applyConvertedVectorTilesToDataset(ds *models.Dataset, vtRoot string, vtRes *gis.ConvertVectorTilesResult, byLayer map[string]gis.VectorTileLayerResult) {
	if ds == nil || vtRes == nil {
		return
	}
	if ds.ExtractedMetadata == nil {
		ds.ExtractedMetadata = make(map[string]interface{})
	}
	vtMetaMap := make(map[string]interface{})
	if existingVT, ok := ds.ExtractedMetadata["vector_tiles"].(map[string]interface{}); ok {
		for k, v := range existingVT {
			vtMetaMap[k] = v
		}
	}
	for _, lres := range vtRes.Layers {
		if byLayer != nil {
			byLayer[lres.LayerName] = lres
			byLayer[lres.LayerSlug] = lres
		}
		vtMetaMap[lres.LayerSlug] = buildVectorTileLayerMeta(ds.ID, lres)
	}
	ds.ExtractedMetadata["vector_tiles"] = vtMetaMap
	ds.ExtractedMetadata["vector_tiles_root"] = vtRoot
}

func setWorkspaceLayerExtentPointers(wl *models.WorkspaceLayer, bounds []float64) {
	if len(bounds) == 4 {
		v0, v1, v2, v3 := bounds[0], bounds[1], bounds[2], bounds[3]
		wl.ExtentMinX, wl.ExtentMinY, wl.ExtentMaxX, wl.ExtentMaxY = &v0, &v1, &v2, &v3
		wl.Extent = []float64{v0, v1, v2, v3}
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func normalizeGISFieldType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "integer", "int", "int64":
		return "integer"
	case "float", "double", "real", "number":
		return "float"
	case "boolean", "bool":
		return "boolean"
	case "date", "datetime":
		return "date"
	default:
		return "string"
	}
}

func extractRasterDims(ds *models.Dataset) (width, height, bands int, resX, resY *float64) {
	if ds == nil || ds.ExtractedMetadata == nil {
		return 0, 0, 0, nil, nil
	}
	if w, ok := asFloat64(ds.ExtractedMetadata["width"]); ok {
		width = int(w)
	}
	if h, ok := asFloat64(ds.ExtractedMetadata["height"]); ok {
		height = int(h)
	}
	if b, ok := asFloat64(ds.ExtractedMetadata["bands"]); ok {
		bands = int(b)
	}
	if res, ok := ds.ExtractedMetadata["resolution"].([]interface{}); ok && len(res) >= 2 {
		if rx, ok1 := asFloat64(res[0]); ok1 {
			resX = &rx
		}
		if ry, ok2 := asFloat64(res[1]); ok2 {
			resY = &ry
		}
	} else if resF, ok := ds.ExtractedMetadata["resolution"].([]float64); ok && len(resF) >= 2 {
		rx, ry := resF[0], resF[1]
		resX, resY = &rx, &ry
	}
	return width, height, bands, resX, resY
}

func rasterStatusString(ds *models.Dataset) string {
	if ds == nil {
		return "unknown"
	}
	if strings.EqualFold(ds.GeoreferenceStatus, "completed") {
		return "georeferenced"
	}
	return "unreferenced"
}

func (s *WorkspaceRegistryService) ListWorkspaceLayers(ctx context.Context, userID, workspaceID string) ([]*models.WorkspaceLayer, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	layers, err := s.repo.ListWorkspaceLayers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	// Sync live visibility/opacity/name from linked GIS layer if mutated via AdvancedLayerManager
	if s.gisService != nil && s.gisService.GetGISRepo() != nil {
		gisLayers, _ := s.gisService.GetGISRepo().GetLayers(ctx, workspaceID)
		byGISID := make(map[string]*models.MapLayer, len(gisLayers))
		for _, gl := range gisLayers {
			byGISID[gl.LayerID] = gl
		}
		for _, wl := range layers {
			if gl, ok := byGISID[wl.GISLayerID]; ok {
				wl.Visibility = gl.IsVisible
				wl.Opacity = gl.Opacity
			}
		}
	}
	return layers, nil
}

func (s *WorkspaceRegistryService) GetWorkspaceLayer(ctx context.Context, userID, workspaceID, layerID string) (*models.WorkspaceLayer, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	return s.repo.GetWorkspaceLayer(ctx, workspaceID, layerID)
}

func (s *WorkspaceRegistryService) UpdateWorkspaceLayer(ctx context.Context, userID, workspaceID, layerID string, req models.UpdateWorkspaceLayerRequest) (*models.WorkspaceLayer, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, err
	}
	wl, err := s.repo.GetWorkspaceLayer(ctx, workspaceID, layerID)
	if err != nil {
		return nil, err
	}
	if req.DisplayName != nil && strings.TrimSpace(*req.DisplayName) != "" {
		wl.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Visibility != nil {
		wl.Visibility = *req.Visibility
	}
	if req.Opacity != nil {
		if math.IsNaN(*req.Opacity) || *req.Opacity < 0 || *req.Opacity > 1 {
			return nil, fmt.Errorf("%w: opacity must be between 0 and 1", models.ErrInvalidWorkspaceRequest)
		}
		wl.Opacity = *req.Opacity
	}
	if req.LayerOrder != nil {
		wl.LayerOrder = *req.LayerOrder
	}
	if err := s.repo.UpdateWorkspaceLayer(ctx, wl); err != nil {
		return nil, err
	}
	// Sync changes to linked GIS layer
	if wl.GISLayerID != "" && s.gisService != nil {
		_, _ = s.gisService.UpdateLayer(ctx, wl.GISLayerID, userID, models.UpdateLayerRequest{
			LayerName: &wl.DisplayName,
			IsVisible: &wl.Visibility,
			Opacity:   &wl.Opacity,
		})
	}
	return wl, nil
}

// DeleteWorkspaceLayer removes one workspace layer without deleting sibling layers or the original source dataset.
func (s *WorkspaceRegistryService) DeleteWorkspaceLayer(ctx context.Context, userID, workspaceID, layerID string) error {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return err
	}
	wl, err := s.repo.GetWorkspaceLayer(ctx, workspaceID, layerID)
	if err != nil {
		return err
	}
	if wl.GISLayerID != "" && s.gisService != nil {
		_ = s.gisService.DeleteLayer(ctx, wl.GISLayerID, userID)
	}
	_ = s.repo.DeleteWorkspaceRasterByLayer(ctx, workspaceID, layerID)
	return s.repo.DeleteWorkspaceLayer(ctx, workspaceID, layerID)
}

func (s *WorkspaceRegistryService) ReorderWorkspaceLayers(ctx context.Context, userID, workspaceID string, layerIDs []string) ([]*models.WorkspaceLayer, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, err
	}
	gisOrder := make([]string, 0, len(layerIDs))
	for idx, lid := range layerIDs {
		wl, err := s.repo.GetWorkspaceLayer(ctx, workspaceID, lid)
		if err != nil {
			continue
		}
		wl.LayerOrder = idx
		_ = s.repo.UpdateWorkspaceLayer(ctx, wl)
		if wl.GISLayerID != "" {
			gisOrder = append(gisOrder, wl.GISLayerID)
		}
	}
	if len(gisOrder) > 0 && s.gisService != nil {
		_, _ = s.gisService.ReorderLayers(ctx, workspaceID, userID, models.ReorderLayersRequest{
			LayerIDs: gisOrder,
		})
	}
	return s.repo.ListWorkspaceLayers(ctx, workspaceID)
}

func (s *WorkspaceRegistryService) BatchUpdateVisibility(ctx context.Context, userID, workspaceID string, req models.BatchWorkspaceVisibilityRequest) ([]*models.WorkspaceLayer, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, err
	}
	for _, lid := range req.LayerIDs {
		vis := req.Visibility
		_, _ = s.UpdateWorkspaceLayer(ctx, userID, workspaceID, lid, models.UpdateWorkspaceLayerRequest{
			Visibility: &vis,
		})
	}
	return s.repo.ListWorkspaceLayers(ctx, workspaceID)
}

// ============================================================================
// 3. FEATURE QUERYING (Filtering, BBox, Pagination, Attribute Selection)
// ============================================================================

func featureCenterAndBounds(geom models.GeoJSONGeometry) ([]float64, []float64) {
	val, _ := gis.ValidateGeoJSONGeometry(geom, "EPSG:4326")
	var bounds []float64
	if val != nil && len(val.BBox) == 4 {
		bounds = val.BBox
		center := []float64{(bounds[0] + bounds[2]) / 2.0, (bounds[1] + bounds[3]) / 2.0}
		return center, bounds
	}
	if geom.Type == "Point" {
		var pt []float64
		if json.Unmarshal(geom.Coordinates, &pt) == nil && len(pt) >= 2 {
			return []float64{pt[0], pt[1]}, []float64{pt[0], pt[1], pt[0], pt[1]}
		}
	}
	return nil, nil
}

func (s *WorkspaceRegistryService) resolveGISLayerID(ctx context.Context, workspaceID, layerID string) (string, *models.WorkspaceLayer) {
	if wl, err := s.repo.GetWorkspaceLayer(ctx, workspaceID, layerID); err == nil && wl != nil {
		if wl.GISLayerID != "" {
			return wl.GISLayerID, wl
		}
	}
	return layerID, nil
}

type LayerFeaturesPage struct {
	LayerID    string                  `json:"layer_id"`
	Total      int                     `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalPages int                     `json:"total_pages"`
	Features   []*models.VectorFeature `json:"features"`
}

func (s *WorkspaceRegistryService) QueryLayerFeatures(
	ctx context.Context,
	userID, workspaceID, layerID string,
	bbox []float64,
	search, attrField, attrValue string,
	selectedFields []string,
	page, pageSize int,
) (*LayerFeaturesPage, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	if s.gisService == nil || s.gisService.GetGISRepo() == nil {
		return &LayerFeaturesPage{LayerID: layerID, Page: 1, PageSize: 50, Features: []*models.VectorFeature{}}, nil
	}
	gisLayerID, _ := s.resolveGISLayerID(ctx, workspaceID, layerID)
	feats, err := s.gisService.GetGISRepo().GetFeatures(ctx, gisLayerID, bbox)
	if err != nil {
		return nil, err
	}

	// Safe attribute & search filtering
	filtered := make([]*models.VectorFeature, 0, len(feats))
	lq := strings.ToLower(strings.TrimSpace(search))
	af := strings.TrimSpace(attrField)
	av := strings.ToLower(strings.TrimSpace(attrValue))

	for _, f := range feats {
		if af != "" {
			val, exists := f.Properties[af]
			if !exists || !strings.Contains(strings.ToLower(fmt.Sprintf("%v", val)), av) {
				continue
			}
		}
		if lq != "" {
			matched := strings.Contains(strings.ToLower(f.FeatureID), lq)
			if !matched {
				for _, v := range f.Properties {
					if strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), lq) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, f)
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	total := len(filtered)
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	pageItems := filtered[start:end]
	if len(selectedFields) > 0 {
		allow := make(map[string]bool, len(selectedFields))
		for _, k := range selectedFields {
			allow[strings.TrimSpace(k)] = true
		}
		projected := make([]*models.VectorFeature, 0, len(pageItems))
		for _, f := range pageItems {
			cp := *f
			props := make(map[string]interface{})
			for k, v := range f.Properties {
				if allow[k] {
					props[k] = v
				}
			}
			cp.Properties = props
			projected = append(projected, &cp)
		}
		pageItems = projected
	}

	return &LayerFeaturesPage{
		LayerID:    layerID,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		Features:   pageItems,
	}, nil
}

func (s *WorkspaceRegistryService) GetLayerFeature(ctx context.Context, userID, workspaceID, layerID, featureID string) (*models.VectorFeature, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	if s.gisService == nil || s.gisService.GetGISRepo() == nil {
		return nil, models.ErrWorkspaceRecordNotFound
	}
	return s.gisService.GetGISRepo().GetFeature(ctx, featureID)
}

// ============================================================================
// 4. CENTRALIZED WORKSPACE RECORDS (All, Vectors, Rasters, Control Points, Icons)
// ============================================================================

func (s *WorkspaceRegistryService) collectWorkspaceDatasetIDs(ctx context.Context, workspaceID string) map[string]bool {
	dsIDs := make(map[string]bool)
	if workspaceID != "" {
		dsIDs[workspaceID] = true
	}
	if wsDatasets, err := s.repo.ListWorkspaceDatasets(ctx, workspaceID); err == nil {
		for _, wds := range wsDatasets {
			if wds.SourceDatasetID != "" {
				dsIDs[wds.SourceDatasetID] = true
			}
		}
	}
	if wsRasters, err := s.repo.ListWorkspaceRasters(ctx, workspaceID); err == nil {
		for _, rst := range wsRasters {
			if rst.DatasetID != "" {
				dsIDs[rst.DatasetID] = true
			}
		}
	}
	if s.gisService != nil && s.gisService.GetGISRepo() != nil {
		if layers, err := s.gisService.GetGISRepo().GetLayers(ctx, workspaceID); err == nil {
			for _, l := range layers {
				if l.DatasetID != "" {
					dsIDs[l.DatasetID] = true
				}
			}
		}
	}
	// Also include members from active session workspace project if workspaceID is a dataset/project ID
	if s.sessionRepo != nil {
		for _, ds := range s.sessionRepo.ListAllDatasets() {
			if ds == nil {
				continue
			}
			if ds.ID == workspaceID {
				dsIDs[ds.ID] = true
				if proj, err := s.sessionRepo.GetMapProject(ds.SessionID, ds.ID); err == nil && proj != nil && proj.Configuration != nil {
					if rawMembers, ok := proj.Configuration["members"].([]interface{}); ok {
						for _, m := range rawMembers {
							if mMap, ok := m.(map[string]interface{}); ok {
								if mid, ok := mMap["dataset_id"].(string); ok && mid != "" {
									dsIDs[mid] = true
								}
							} else if mid, ok := m.(string); ok && mid != "" {
								dsIDs[mid] = true
							}
						}
					}
				}
			}
		}
	}
	return dsIDs
}

func pickFeatureDisplayName(f *models.VectorFeature, fallback string) string {
	for _, key := range []string{"name", "Name", "NAME", "title", "label", "ward_name", "station_name", "ps_name", "id", "fid"} {
		if v, ok := f.Properties[key]; ok && v != nil {
			str := strings.TrimSpace(fmt.Sprintf("%v", v))
			if str != "" && str != "<nil>" {
				return str
			}
		}
	}
	for k, v := range f.Properties {
		if strings.HasPrefix(k, "_") || v == nil {
			continue
		}
		str := strings.TrimSpace(fmt.Sprintf("%v", v))
		if str != "" && str != "<nil>" && len(str) <= 80 {
			return str
		}
	}
	return fallback
}

func (s *WorkspaceRegistryService) aggregateAllWorkspaceRecords(ctx context.Context, workspaceID string) []*models.WorkspaceRecord {
	records := make([]*models.WorkspaceRecord, 0)
	wsLayers, _ := s.repo.ListWorkspaceLayers(ctx, workspaceID)
	wlByGISID := make(map[string]*models.WorkspaceLayer)
	for _, wl := range wsLayers {
		if wl.GISLayerID != "" {
			wlByGISID[wl.GISLayerID] = wl
		}
	}

	// A. Vector feature records across all active vector layers in this workspace
	if s.gisService != nil && s.gisService.GetGISRepo() != nil {
		gisLayers, _ := s.gisService.GetGISRepo().GetLayers(ctx, workspaceID)
		for _, gl := range gisLayers {
			if gl.LayerType != "vector" {
				continue
			}
			wl := wlByGISID[gl.LayerID]
			layerKey := gl.LayerID
			layerName := gl.LayerName
			layerCRS := gl.CRS
			if wl != nil {
				layerKey = wl.ID
				layerName = wl.DisplayName
				if wl.SourceCRS != "" {
					layerCRS = wl.SourceCRS
				}
			}
			feats, _ := s.gisService.GetGISRepo().GetFeatures(ctx, gl.LayerID, nil)
			for idx, f := range feats {
				center, bounds := featureCenterAndBounds(f.Geometry)
				attrs := make(map[string]interface{})
				for k, v := range f.Properties {
					if !strings.HasPrefix(k, "_") {
						attrs[k] = v
					}
				}
				srcFeatID := f.FeatureID
				if sfi, ok := f.Properties["_source_feature_id"].(string); ok && sfi != "" {
					srcFeatID = sfi
				}
				featCRS := ""
				if f.SRID > 0 {
					featCRS = fmt.Sprintf("EPSG:%d", f.SRID)
				}
				records = append(records, &models.WorkspaceRecord{
					ID:               "vec:" + f.FeatureID,
					WorkspaceID:      workspaceID,
					WorkspaceLayerID: layerKey,
					LayerName:        layerName,
					RecordType:       models.RecordTypeVector,
					SourceFeatureID:  srcFeatID,
					DatasetID:        gl.DatasetID,
					DisplayName:      pickFeatureDisplayName(f, fmt.Sprintf("%s #%d", layerName, idx+1)),
					GeometryType:     f.Geometry.Type,
					Coordinates:      center,
					Bounds:           bounds,
					CRS:              firstNonEmptyStr(layerCRS, featCRS, "EPSG:4326"),
					Visibility:       gl.IsVisible,
					SourceReference:  gl.LayerName,
					Status:           "active",
					Attributes:       attrs,
					CreatedAt:        f.CreatedAt,
					UpdatedAt:        f.UpdatedAt,
				})
			}
		}
	}

	// B. Raster records (from workspace_rasters + active session raster datasets linked to this workspace)
	seenRasterDS := make(map[string]bool)
	wsRasters, _ := s.repo.ListWorkspaceRasters(ctx, workspaceID)
	for _, rst := range wsRasters {
		seenRasterDS[rst.DatasetID] = true
		resStr := ""
		if rst.ResolutionX != nil && rst.ResolutionY != nil {
			resStr = fmt.Sprintf("%.4g × %.4g", *rst.ResolutionX, *rst.ResolutionY)
		}
		records = append(records, &models.WorkspaceRecord{
			ID:               "rst:" + rst.ID,
			WorkspaceID:      workspaceID,
			WorkspaceLayerID: rst.WorkspaceLayerID,
			LayerName:        firstNonEmptyStr(rst.LayerName, rst.DisplayName),
			RecordType:       models.RecordTypeRaster,
			DatasetID:        rst.DatasetID,
			DisplayName:      rst.DisplayName,
			GeometryType:     "Raster",
			Bounds:           rst.Bounds,
			CRS:              rst.SourceCRS,
			Visibility:       rst.Visibility,
			SourceReference:  rst.RasterPath,
			Status:           rst.Status,
			Width:            rst.Width,
			Height:           rst.Height,
			Resolution:       resStr,
			CreatedAt:        rst.CreatedAt,
			UpdatedAt:        rst.UpdatedAt,
		})
	}

	dsIDs := s.collectWorkspaceDatasetIDs(ctx, workspaceID)
	if s.sessionRepo != nil {
		for _, ds := range s.sessionRepo.ListAllDatasets() {
			if ds == nil || !dsIDs[ds.ID] {
				continue
			}
			ext := strings.ToLower(filepath.Ext(ds.Filename))
			if isRasterExt(ext) && !seenRasterDS[ds.ID] {
				seenRasterDS[ds.ID] = true
				w, h, _, rx, ry := extractRasterDims(ds)
				var bounds []float64
				if ds.MapLayer != nil {
					if b, ok := parseBoundsInterface(ds.MapLayer["bounds"]); ok {
						bounds = b
					}
				}
				if len(bounds) != 4 && ds.ExtractedMetadata != nil {
					if b, ok := parseBoundsInterface(ds.ExtractedMetadata["bounds"]); ok {
						bounds = b
					}
				}
				resStr := ""
				if rx != nil && ry != nil {
					resStr = fmt.Sprintf("%.4g × %.4g", *rx, *ry)
				}
				records = append(records, &models.WorkspaceRecord{
					ID:              "rst:" + ds.ID,
					WorkspaceID:     workspaceID,
					LayerName:       ds.Filename,
					RecordType:      models.RecordTypeRaster,
					DatasetID:       ds.ID,
					DisplayName:     ds.Filename,
					GeometryType:    "Raster",
					Bounds:          bounds,
					CRS:             extractCRSFromDataset(ds),
					Visibility:      true,
					SourceReference: ds.StoragePath,
					Status:          rasterStatusString(ds),
					Width:           w,
					Height:          h,
					Resolution:      resStr,
					CreatedAt:       ds.CreatedAt,
					UpdatedAt:       ds.UpdatedAt,
				})
			}

			// C. Control Points for datasets belonging to this workspace
			if cps, err := s.sessionRepo.ListControlPoints(ds.SessionID, ds.ID); err == nil {
				for idx, cp := range cps {
					imgX, imgY := cp.ImageX, cp.ImageY
					geoX, geoY := cp.Longitude, cp.Latitude
					pStatus := "paired"
					if geoX == 0 && geoY == 0 {
						pStatus = "unpaired"
					}
					label := fmt.Sprintf("Control Point #%d", idx+1)
					records = append(records, &models.WorkspaceRecord{
						ID:              "cp:" + cp.ID,
						WorkspaceID:     workspaceID,
						LayerName:       ds.Filename,
						RecordType:      models.RecordTypeControlPoint,
						SourceFeatureID: cp.ID,
						DatasetID:       ds.ID,
						DisplayName:     label,
						GeometryType:    "Point",
						Coordinates:     []float64{geoX, geoY},
						CRS:             "EPSG:4326",
						Visibility:      true,
						SourceReference: ds.Filename,
						Status:          pStatus,
						ImageX:          &imgX,
						ImageY:          &imgY,
						GeographicX:     &geoX,
						GeographicY:     &geoY,
						PairingStatus:   pStatus,
						CreatedAt:       cp.CreatedAt,
						UpdatedAt:       cp.CreatedAt,
					})
				}
			}

			// D. User-created Icon Points for datasets belonging to this workspace
			if icons, err := s.sessionRepo.ListIconPoints(ds.SessionID, ds.ID, ""); err == nil {
				for _, ic := range icons {
					gx, gy := ic.Longitude, ic.Latitude
					records = append(records, &models.WorkspaceRecord{
						ID:              "icon:" + ic.ID,
						WorkspaceID:     workspaceID,
						LayerName:       ds.Filename,
						RecordType:      models.RecordTypeUserIcon,
						SourceFeatureID: ic.ID,
						DatasetID:       ds.ID,
						DisplayName:     firstNonEmptyStr(ic.Name, ic.IconID, "Marker"),
						GeometryType:    "Point",
						Coordinates:     []float64{gx, gy},
						CRS:             "EPSG:4326",
						Visibility:      true,
						SourceReference: ds.Filename,
						Status:          "active",
						IconType:        ic.IconID,
						GeographicX:     &gx,
						GeographicY:     &gy,
						Attributes: map[string]interface{}{
							"description": ic.Description,
							"icon_id":     ic.IconID,
							"scope":       ic.Scope,
							"floor_level": ic.FloorLevel,
						},
						CreatedAt: ic.CreatedAt,
						UpdatedAt: ic.UpdatedAt,
					})
				}
			}
		}
	}

	return records
}

type WorkspaceRecordsPage struct {
	Total      int                          `json:"total"`
	Page       int                          `json:"page"`
	PageSize   int                          `json:"page_size"`
	TotalPages int                          `json:"total_pages"`
	Counts     models.WorkspaceRecordCounts `json:"counts"`
	Records    []*models.WorkspaceRecord    `json:"records"`
}

func (s *WorkspaceRegistryService) GetWorkspaceRecordCounts(ctx context.Context, userID, workspaceID string) (*models.WorkspaceRecordCounts, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	all := s.aggregateAllWorkspaceRecords(ctx, workspaceID)
	counts := &models.WorkspaceRecordCounts{All: len(all)}
	for _, r := range all {
		switch r.RecordType {
		case models.RecordTypeVector:
			counts.Vectors++
		case models.RecordTypeRaster:
			counts.Rasters++
		case models.RecordTypeControlPoint:
			counts.ControlPoints++
		case models.RecordTypeUserIcon:
			counts.UserIcons++
		}
	}
	return counts, nil
}

func (s *WorkspaceRegistryService) ListWorkspaceRecords(ctx context.Context, userID, workspaceID string, q models.WorkspaceRecordsQuery) (*WorkspaceRecordsPage, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	all := s.aggregateAllWorkspaceRecords(ctx, workspaceID)
	counts := models.WorkspaceRecordCounts{All: len(all)}
	for _, r := range all {
		switch r.RecordType {
		case models.RecordTypeVector:
			counts.Vectors++
		case models.RecordTypeRaster:
			counts.Rasters++
		case models.RecordTypeControlPoint:
			counts.ControlPoints++
		case models.RecordTypeUserIcon:
			counts.UserIcons++
		}
	}

	recType := strings.ToLower(strings.TrimSpace(q.RecordType))
	if recType == "" {
		recType = models.RecordTypeAll
	}
	layerFilter := strings.TrimSpace(q.LayerID)
	statusFilter := strings.ToLower(strings.TrimSpace(q.Status))
	search := strings.ToLower(strings.TrimSpace(q.Search))

	filtered := make([]*models.WorkspaceRecord, 0, len(all))
	for _, r := range all {
		if recType != models.RecordTypeAll && r.RecordType != recType {
			continue
		}
		if layerFilter != "" && r.WorkspaceLayerID != layerFilter && r.LayerName != layerFilter && r.DatasetID != layerFilter {
			continue
		}
		if statusFilter != "" && strings.ToLower(r.Status) != statusFilter {
			continue
		}
		if search != "" {
			hit := strings.Contains(strings.ToLower(r.DisplayName), search) ||
				strings.Contains(strings.ToLower(r.LayerName), search) ||
				strings.Contains(strings.ToLower(r.SourceFeatureID), search) ||
				strings.Contains(strings.ToLower(r.GeometryType), search) ||
				strings.Contains(strings.ToLower(r.CRS), search)
			if !hit && r.Attributes != nil {
				for _, v := range r.Attributes {
					if strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), search) {
						hit = true
						break
					}
				}
			}
			if !hit {
				continue
			}
		}
		filtered = append(filtered, r)
	}

	// Safe sorting
	sortBy := strings.TrimSpace(q.SortBy)
	desc := strings.EqualFold(q.SortDirection, "desc")
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		var cmp int
		switch sortBy {
		case "layer_name":
			cmp = strings.Compare(strings.ToLower(a.LayerName), strings.ToLower(b.LayerName))
		case "record_type":
			cmp = strings.Compare(a.RecordType, b.RecordType)
		case "geometry_type":
			cmp = strings.Compare(strings.ToLower(a.GeometryType), strings.ToLower(b.GeometryType))
		case "status":
			cmp = strings.Compare(strings.ToLower(a.Status), strings.ToLower(b.Status))
		case "crs":
			cmp = strings.Compare(strings.ToLower(a.CRS), strings.ToLower(b.CRS))
		case "updated_at":
			if a.UpdatedAt.Before(b.UpdatedAt) {
				cmp = -1
			} else if a.UpdatedAt.After(b.UpdatedAt) {
				cmp = 1
			}
		case "", "display_name":
			cmp = strings.Compare(strings.ToLower(a.DisplayName), strings.ToLower(b.DisplayName))
		default:
			va := fmt.Sprintf("%v", a.Attributes[sortBy])
			vb := fmt.Sprintf("%v", b.Attributes[sortBy])
			cmp = strings.Compare(strings.ToLower(va), strings.ToLower(vb))
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})

	page := q.Page
	if page <= 0 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 || pageSize > 250 {
		pageSize = 25
	}
	total := len(filtered)
	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return &WorkspaceRecordsPage{
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		Counts:     counts,
		Records:    filtered[start:end],
	}, nil
}

func (s *WorkspaceRegistryService) GetWorkspaceRecord(ctx context.Context, userID, workspaceID, recordID string) (*models.WorkspaceRecord, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	for _, r := range s.aggregateAllWorkspaceRecords(ctx, workspaceID) {
		if r.ID == recordID || r.SourceFeatureID == recordID {
			return r, nil
		}
	}
	return nil, models.ErrWorkspaceRecordNotFound
}

func (s *WorkspaceRegistryService) GetWorkspaceRecordSchema(ctx context.Context, userID, workspaceID, recordType, layerID string) ([]models.WorkspaceRecordColumn, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	rt := strings.ToLower(strings.TrimSpace(recordType))
	var cols []models.WorkspaceRecordColumn

	switch rt {
	case models.RecordTypeRaster:
		cols = []models.WorkspaceRecordColumn{
			{Key: "display_name", Label: "Name / Label", Type: "string", DefaultShow: true},
			{Key: "layer_name", Label: "Layer", Type: "string", DefaultShow: true},
			{Key: "visibility", Label: "Visibility", Type: "boolean", Tooltip: "Controls whether this layer appears on the map.", DefaultShow: true},
			{Key: "bounds", Label: "Coordinates / Bounds", Type: "bounds", Tooltip: "The geographic area covered by this layer.", DefaultShow: true},
			{Key: "source_reference", Label: "Local File Location", Type: "string", DefaultShow: false},
			{Key: "crs", Label: "CRS", Type: "string", Tooltip: "Coordinate Reference System — defines how locations are positioned on the map.", DefaultShow: true},
			{Key: "dimensions", Label: "Dimensions", Type: "string", DefaultShow: true},
			{Key: "resolution", Label: "Resolution", Type: "string", DefaultShow: true},
			{Key: "status", Label: "Status", Type: "string", DefaultShow: true},
		}
	case models.RecordTypeControlPoint:
		cols = []models.WorkspaceRecordColumn{
			{Key: "display_name", Label: "Name / Label", Type: "string", DefaultShow: true},
			{Key: "source_feature_id", Label: "Control Point ID", Type: "string", DefaultShow: false},
			{Key: "layer_name", Label: "Associated Raster / Layer", Type: "string", DefaultShow: true},
			{Key: "image_x", Label: "Image X", Type: "number", DefaultShow: true},
			{Key: "image_y", Label: "Image Y", Type: "number", DefaultShow: true},
			{Key: "geographic_x", Label: "Geographic X (Lng)", Type: "number", DefaultShow: true},
			{Key: "geographic_y", Label: "Geographic Y (Lat)", Type: "number", DefaultShow: true},
			{Key: "crs", Label: "CRS", Type: "string", Tooltip: "Coordinate Reference System — defines how locations are positioned on the map.", DefaultShow: true},
			{Key: "pairing_status", Label: "Pairing Status", Type: "string", DefaultShow: true},
			{Key: "visibility", Label: "Visibility", Type: "boolean", DefaultShow: true},
		}
	case models.RecordTypeUserIcon:
		cols = []models.WorkspaceRecordColumn{
			{Key: "display_name", Label: "Name / Label", Type: "string", DefaultShow: true},
			{Key: "icon_type", Label: "Icon Type", Type: "string", DefaultShow: true},
			{Key: "layer_name", Label: "Associated Layer", Type: "string", DefaultShow: true},
			{Key: "coordinates", Label: "Coordinates", Type: "coords", DefaultShow: true},
			{Key: "crs", Label: "CRS", Type: "string", Tooltip: "Coordinate Reference System — defines how locations are positioned on the map.", DefaultShow: true},
			{Key: "visibility", Label: "Visibility", Type: "boolean", DefaultShow: true},
			{Key: "status", Label: "Status", Type: "string", DefaultShow: true},
		}
	default:
		// All Records or Vectors — base columns + dynamically discovered attribute fields
		cols = []models.WorkspaceRecordColumn{
			{Key: "display_name", Label: "Name / Label", Type: "string", DefaultShow: true},
			{Key: "layer_name", Label: "Layer", Type: "string", DefaultShow: true},
			{Key: "record_type", Label: "Category", Type: "string", DefaultShow: rt == "" || rt == models.RecordTypeAll},
			{Key: "geometry_type", Label: "Geometry", Type: "string", DefaultShow: true},
			{Key: "coordinates", Label: "Coordinates / Extent", Type: "coords", Tooltip: "Geographic position or bounding box.", DefaultShow: true},
			{Key: "crs", Label: "CRS", Type: "string", Tooltip: "Coordinate Reference System — defines how locations are positioned on the map.", DefaultShow: true},
			{Key: "visibility", Label: "Visibility", Type: "boolean", Tooltip: "Controls whether this layer appears on the map.", DefaultShow: true},
			{Key: "status", Label: "Status", Type: "string", DefaultShow: true},
		}

		// Dynamically discover attribute fields from active layers (never hardcoded!)
		seenFields := make(map[string]bool)
		wsLayers, _ := s.repo.ListWorkspaceLayers(ctx, workspaceID)
		for _, wl := range wsLayers {
			if layerID != "" && wl.ID != layerID && wl.GISLayerID != layerID {
				continue
			}
			for _, f := range wl.Fields {
				fn, _ := f["name"].(string)
				ft, _ := f["type"].(string)
				if fn == "" || strings.HasPrefix(fn, "_") || seenFields[fn] {
					continue
				}
				seenFields[fn] = true
				cols = append(cols, models.WorkspaceRecordColumn{
					Key:         "attr:" + fn,
					Label:       fn,
					Type:        firstNonEmptyStr(ft, "string"),
					DefaultShow: len(seenFields) <= 4 && rt == models.RecordTypeVector,
					Dynamic:     true,
				})
			}
		}
	}

	return cols, nil
}

func (s *WorkspaceRegistryService) UpdateWorkspaceRecord(ctx context.Context, userID, workspaceID, recordID string, req models.UpdateWorkspaceRecordRequest) (*models.WorkspaceRecord, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, err
	}
	if strings.HasPrefix(recordID, "vec:") {
		fid := strings.TrimPrefix(recordID, "vec:")
		if s.gisService == nil {
			return nil, models.ErrWorkspaceRecordNotFound
		}
		feat, err := s.gisService.GetGISRepo().GetFeature(ctx, fid)
		if err != nil {
			return nil, models.ErrWorkspaceRecordNotFound
		}
		props := make(map[string]interface{})
		for k, v := range feat.Properties {
			props[k] = v
		}
		if req.DisplayName != nil {
			props["name"] = strings.TrimSpace(*req.DisplayName)
		}
		for k, v := range req.EditableAttributes {
			if !strings.HasPrefix(k, "_") {
				props[k] = v
			}
		}
		if _, err := s.gisService.UpdateFeature(ctx, fid, userID, models.UpdateFeatureRequest{Properties: props}); err != nil {
			return nil, err
		}
		return s.GetWorkspaceRecord(ctx, userID, workspaceID, recordID)
	}

	if strings.HasPrefix(recordID, "rst:") {
		rid := strings.TrimPrefix(recordID, "rst:")
		rst, err := s.UpdateWorkspaceRaster(ctx, userID, workspaceID, rid, models.UpdateWorkspaceRasterRequest{
			DisplayName: req.DisplayName,
			Visibility:  req.Visibility,
		})
		if err != nil {
			return nil, err
		}
		return s.GetWorkspaceRecord(ctx, userID, workspaceID, "rst:"+rst.ID)
	}

	return nil, fmt.Errorf("%w: record type is read-only or unsupported for direct update", models.ErrInvalidWorkspaceRequest)
}

func (s *WorkspaceRegistryService) DeleteWorkspaceRecord(ctx context.Context, userID, workspaceID, recordID string) error {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return err
	}
	if strings.HasPrefix(recordID, "vec:") {
		fid := strings.TrimPrefix(recordID, "vec:")
		if s.gisService == nil {
			return models.ErrWorkspaceRecordNotFound
		}
		return s.gisService.DeleteFeature(ctx, fid, userID)
	}
	if strings.HasPrefix(recordID, "cp:") && s.sessionRepo != nil {
		cpID := strings.TrimPrefix(recordID, "cp:")
		for _, ds := range s.sessionRepo.ListAllDatasets() {
			if ds == nil {
				continue
			}
			if _, err := s.sessionRepo.DeleteControlPoint(ds.SessionID, cpID); err == nil {
				return nil
			}
		}
		return models.ErrWorkspaceRecordNotFound
	}
	if strings.HasPrefix(recordID, "icon:") && s.sessionRepo != nil {
		iconID := strings.TrimPrefix(recordID, "icon:")
		for _, ds := range s.sessionRepo.ListAllDatasets() {
			if ds == nil {
				continue
			}
			if _, err := s.sessionRepo.DeleteIconPoint(ds.SessionID, iconID); err == nil {
				return nil
			}
		}
		return models.ErrWorkspaceRecordNotFound
	}
	return models.ErrWorkspaceRecordNotFound
}

func (s *WorkspaceRegistryService) ExportWorkspaceRecords(ctx context.Context, userID, workspaceID string, req models.ExportWorkspaceRecordsRequest) ([]byte, string, string, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, "", "", err
	}
	page, err := s.ListWorkspaceRecords(ctx, userID, workspaceID, models.WorkspaceRecordsQuery{
		RecordType: req.RecordType,
		LayerID:    req.LayerID,
		Page:       1,
		PageSize:   10000,
	})
	if err != nil {
		return nil, "", "", err
	}
	records := page.Records
	if len(req.RecordIDs) > 0 {
		allow := make(map[string]bool, len(req.RecordIDs))
		for _, id := range req.RecordIDs {
			allow[id] = true
		}
		filtered := make([]*models.WorkspaceRecord, 0, len(req.RecordIDs))
		for _, r := range records {
			if allow[r.ID] {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}

	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "geojson" {
		feats := make([]map[string]interface{}, 0, len(records))
		for _, r := range records {
			var geom interface{}
			if len(r.Coordinates) >= 2 {
				geom = map[string]interface{}{
					"type":        "Point",
					"coordinates": r.Coordinates[:2],
				}
			}
			props := map[string]interface{}{
				"id":            r.ID,
				"display_name":  r.DisplayName,
				"layer_name":    r.LayerName,
				"record_type":   r.RecordType,
				"geometry_type": r.GeometryType,
				"crs":           r.CRS,
				"status":        r.Status,
			}
			for k, v := range r.Attributes {
				props[k] = v
			}
			feats = append(feats, map[string]interface{}{
				"type":       "Feature",
				"id":         r.ID,
				"geometry":   geom,
				"properties": props,
			})
		}
		raw, _ := json.MarshalIndent(map[string]interface{}{
			"type":     "FeatureCollection",
			"features": feats,
		}, "", "  ")
		return raw, "application/geo+json", "workspace_records.geojson", nil
	}

	// Default CSV export
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	attrKeysMap := make(map[string]bool)
	for _, r := range records {
		for k := range r.Attributes {
			attrKeysMap[k] = true
		}
	}
	attrKeys := make([]string, 0, len(attrKeysMap))
	for k := range attrKeysMap {
		attrKeys = append(attrKeys, k)
	}
	sort.Strings(attrKeys)

	headers := append([]string{"id", "display_name", "record_type", "layer_name", "geometry_type", "crs", "visibility", "status", "coordinates"}, attrKeys...)
	_ = w.Write(headers)
	for _, r := range records {
		coordStr := ""
		if len(r.Coordinates) >= 2 {
			coordStr = fmt.Sprintf("%.6f, %.6f", r.Coordinates[0], r.Coordinates[1])
		} else if len(r.Bounds) == 4 {
			coordStr = fmt.Sprintf("[%.4f, %.4f, %.4f, %.4f]", r.Bounds[0], r.Bounds[1], r.Bounds[2], r.Bounds[3])
		}
		row := []string{
			r.ID, r.DisplayName, r.RecordType, r.LayerName, r.GeometryType, r.CRS,
			strconv.FormatBool(r.Visibility), r.Status, coordStr,
		}
		for _, k := range attrKeys {
			if v, ok := r.Attributes[k]; ok && v != nil {
				row = append(row, fmt.Sprintf("%v", v))
			} else {
				row = append(row, "")
			}
		}
		_ = w.Write(row)
	}
	w.Flush()
	return buf.Bytes(), "text/csv; charset=utf-8", "workspace_records.csv", nil
}

// ============================================================================
// 5. RASTER MANAGEMENT
// ============================================================================

func (s *WorkspaceRegistryService) ListWorkspaceRasters(ctx context.Context, userID, workspaceID string) ([]*models.WorkspaceRaster, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	return s.repo.ListWorkspaceRasters(ctx, workspaceID)
}

func (s *WorkspaceRegistryService) GetWorkspaceRaster(ctx context.Context, userID, workspaceID, rasterID string) (*models.WorkspaceRaster, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, false); err != nil {
		return nil, err
	}
	return s.repo.GetWorkspaceRaster(ctx, workspaceID, rasterID)
}

func (s *WorkspaceRegistryService) UpdateWorkspaceRaster(ctx context.Context, userID, workspaceID, rasterID string, req models.UpdateWorkspaceRasterRequest) (*models.WorkspaceRaster, error) {
	if err := s.checkWorkspaceAccess(ctx, workspaceID, userID, true); err != nil {
		return nil, err
	}
	rst, err := s.repo.GetWorkspaceRaster(ctx, workspaceID, rasterID)
	if err != nil {
		return nil, err
	}
	if req.DisplayName != nil && strings.TrimSpace(*req.DisplayName) != "" {
		rst.DisplayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.Visibility != nil {
		rst.Visibility = *req.Visibility
	}
	if req.Opacity != nil {
		if math.IsNaN(*req.Opacity) || *req.Opacity < 0 || *req.Opacity > 1 {
			return nil, fmt.Errorf("%w: opacity must be between 0 and 1", models.ErrInvalidWorkspaceRequest)
		}
		rst.Opacity = *req.Opacity
	}
	if err := s.repo.UpsertWorkspaceRaster(ctx, rst); err != nil {
		return nil, err
	}
	if rst.WorkspaceLayerID != "" {
		_, _ = s.UpdateWorkspaceLayer(ctx, userID, workspaceID, rst.WorkspaceLayerID, models.UpdateWorkspaceLayerRequest{
			DisplayName: req.DisplayName,
			Visibility:  req.Visibility,
			Opacity:     req.Opacity,
		})
	}
	return rst, nil
}
