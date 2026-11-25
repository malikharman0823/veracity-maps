package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

// publishedMapRecord is the on-disk JSON persistence wrapper (includes internal TileDir paths).
type publishedMapRecord struct {
	Map              models.PublishedMap `json:"map"`
	RasterTileDirs   map[string]string   `json:"raster_tile_dirs"`
	RasterImagePaths map[string]string   `json:"raster_image_paths,omitempty"`
	VectorTileDirs   map[string]string   `json:"vector_tile_dirs"`
}

// PublishService manages Combined, Raster-only, and Vector-only Web Tile publishing,
// reusing existing cached raster tiles and local GeoPackage MVT vector tiles without duplicating tile storage.
type PublishService struct {
	mu          sync.RWMutex
	records     map[string]*models.PublishedMap
	storePath   string
	sessionRepo *repository.SessionRepository
	storage     *storage.StorageManager
	exportSvc   *ExportService
	gisSvc      *GISService
	mapRepo     repository.MapRepo
	tileLocks   sync.Map
}

func NewPublishService(
	storeDir string,
	sessionRepo *repository.SessionRepository,
	sm *storage.StorageManager,
	exportSvc *ExportService,
	gisSvc *GISService,
	mapRepo repository.MapRepo,
) *PublishService {
	if strings.TrimSpace(storeDir) == "" {
		storeDir = "storage"
	}
	_ = os.MkdirAll(storeDir, 0755)
	svc := &PublishService{
		records:     make(map[string]*models.PublishedMap),
		storePath:   filepath.Join(storeDir, "published_maps.json"),
		sessionRepo: sessionRepo,
		storage:     sm,
		exportSvc:   exportSvc,
		gisSvc:      gisSvc,
		mapRepo:     mapRepo,
	}
	svc.loadFromDisk()
	return svc
}

func (s *PublishService) SetGISService(gs *GISService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gisSvc = gs
}

func (s *PublishService) SetMapRepo(mr repository.MapRepo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mapRepo = mr
}

func newPublishID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *PublishService) loadFromDisk() {
	raw, err := os.ReadFile(s.storePath)
	if err != nil || len(raw) == 0 {
		return
	}
	var list []publishedMapRecord
	if err := json.Unmarshal(raw, &list); err != nil {
		return
	}
	for _, rec := range list {
		pm := rec.Map
		for i := range pm.RasterLayers {
			if dir, ok := rec.RasterTileDirs[pm.RasterLayers[i].LayerID]; ok {
				pm.RasterLayers[i].TileDir = dir
			}
			if imgP, ok := rec.RasterImagePaths[pm.RasterLayers[i].LayerID]; ok {
				pm.RasterLayers[i].ImagePath = imgP
			}
		}
		for i := range pm.VectorLayers {
			if dir, ok := rec.VectorTileDirs[pm.VectorLayers[i].LayerID]; ok {
				pm.VectorLayers[i].TileDir = dir
			}
		}
		cp := pm
		s.records[cp.MapID] = &cp
	}
}

func (s *PublishService) persistToDiskLocked() {
	list := make([]publishedMapRecord, 0, len(s.records))
	for _, pm := range s.records {
		rDirs := make(map[string]string, len(pm.RasterLayers))
		rImgs := make(map[string]string, len(pm.RasterLayers))
		for _, rl := range pm.RasterLayers {
			rDirs[rl.LayerID] = rl.TileDir
			if rl.ImagePath != "" {
				rImgs[rl.LayerID] = rl.ImagePath
			}
		}
		vDirs := make(map[string]string, len(pm.VectorLayers))
		for _, vl := range pm.VectorLayers {
			vDirs[vl.LayerID] = vl.TileDir
		}
		list = append(list, publishedMapRecord{
			Map:              *pm,
			RasterTileDirs:   rDirs,
			RasterImagePaths: rImgs,
			VectorTileDirs:   vDirs,
		})
	}
	if b, err := json.MarshalIndent(list, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(s.storePath), 0755)
		_ = os.WriteFile(s.storePath, b, 0644)
	}
}

// PublishMap publishes a workspace in "combined", "raster", or "vector" mode.
// Existing raster tile caches and local vector_tiles directories are reused without duplicating tile files.
func (s *PublishService) PublishMap(
	ctx context.Context,
	sessionID, ownerID string,
	req models.PublishMapRequest,
) (*models.PublishedMap, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = models.PublishModeCombined
	}
	if mode != models.PublishModeCombined && mode != models.PublishModeRaster && mode != models.PublishModeVector {
		return nil, fmt.Errorf("%w: mode must be 'combined', 'raster', or 'vector'", models.ErrInvalidPublishRequest)
	}

	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(req.DatasetID)
	}
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(req.ProjectID)
	}

	// Collect datasets in session (and fallback to all datasets when workspace references them)
	datasetMap := make(map[string]*models.Dataset)
	var allDatasets []*models.Dataset
	if s.sessionRepo != nil {
		if sessionID != "" {
			if dsList, err := s.sessionRepo.ListDatasets(sessionID); err == nil {
				for _, ds := range dsList {
					if ds != nil {
						datasetMap[ds.ID] = ds
						allDatasets = append(allDatasets, ds)
					}
				}
			}
		}
		if len(allDatasets) == 0 {
			for _, ds := range s.sessionRepo.ListAllDatasets() {
				if ds != nil {
					if _, exists := datasetMap[ds.ID]; !exists {
						datasetMap[ds.ID] = ds
						allDatasets = append(allDatasets, ds)
					}
				}
			}
		}
	}

	// Filter selection sets if specified
	rasterFilter := make(map[string]bool)
	for _, id := range req.RasterLayerIDs {
		if strings.TrimSpace(id) != "" {
			rasterFilter[strings.TrimSpace(id)] = true
		}
	}
	vectorFilter := make(map[string]bool)
	for _, id := range req.VectorLayerIDs {
		if strings.TrimSpace(id) != "" {
			vectorFilter[strings.TrimSpace(id)] = true
		}
	}
	genericFilter := make(map[string]bool)
	for _, id := range req.LayerIDs {
		if strings.TrimSpace(id) != "" {
			genericFilter[strings.TrimSpace(id)] = true
		}
	}

	isVectorDataset := func(ds *models.Dataset) bool {
		if ds == nil {
			return false
		}
		ext := strings.ToLower(filepath.Ext(ds.Filename))
		switch ext {
		case ".gpkg", ".shp", ".kml", ".kmz", ".geojson", ".osm", ".pbf", ".fgb", ".gml":
			return true
		}
		if ds.ExtractedMetadata != nil {
			if dt, _ := ds.ExtractedMetadata["document_type"].(string); strings.EqualFold(dt, "vector") {
				return true
			}
			if _, hasVT := ds.ExtractedMetadata["vector_tiles"]; hasVT {
				return true
			}
		}
		return false
	}

	// Collect GIS layers for workspace
	var gisLayers []*models.MapLayer
	if s.gisSvc != nil && s.gisSvc.GetGISRepo() != nil {
		if workspaceID != "" {
			if lyrs, err := s.gisSvc.GetGISRepo().GetLayers(ctx, workspaceID); err == nil {
				gisLayers = lyrs
			}
		}
		if len(gisLayers) == 0 && req.DatasetID != "" && req.DatasetID != workspaceID {
			if lyrs, err := s.gisSvc.GetGISRepo().GetLayers(ctx, req.DatasetID); err == nil {
				gisLayers = lyrs
			}
		}
	}
	sort.Slice(gisLayers, func(i, j int) bool {
		return gisLayers[i].DisplayOrder < gisLayers[j].DisplayOrder
	})

	// Member visibility/opacity/order from MapProject if available
	memberMetaByID := make(map[string]map[string]interface{})
	var primaryProj *models.MapProject
	if s.sessionRepo != nil && sessionID != "" && workspaceID != "" {
		primaryProj, _ = s.sessionRepo.GetMapProject(sessionID, workspaceID)
		if primaryProj != nil && primaryProj.Configuration != nil {
			if rawMembers, ok := primaryProj.Configuration["members"].([]interface{}); ok {
				for _, rm := range rawMembers {
					if m, ok := rm.(map[string]interface{}); ok {
						if mID, _ := m["dataset_id"].(string); mID != "" {
							memberMetaByID[mID] = m
						}
					}
				}
			}
		}
	}

	mapID := newPublishID("pub")
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	accessToken := ""
	if !isPublic {
		accessToken = newPublishID("tok")
	}

	minZoom := 0
	maxZoom := 18
	initZoom := 14
	if req.MinZoom != nil {
		minZoom = *req.MinZoom
	}
	if req.MaxZoom != nil {
		maxZoom = *req.MaxZoom
	}
	if req.InitialZoom != nil {
		initZoom = *req.InitialZoom
	}

	var unionBounds []float64
	expandBounds := func(b []float64) {
		if len(b) != 4 {
			return
		}
		for _, v := range b {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return
			}
		}
		if len(unionBounds) != 4 {
			unionBounds = []float64{b[0], b[1], b[2], b[3]}
			return
		}
		unionBounds[0] = math.Min(unionBounds[0], b[0])
		unionBounds[1] = math.Min(unionBounds[1], b[1])
		unionBounds[2] = math.Max(unionBounds[2], b[2])
		unionBounds[3] = math.Max(unionBounds[3], b[3])
	}

	publishedRasters := make([]models.PublishedRasterLayer, 0)
	publishedVectors := make([]models.PublishedVectorLayer, 0)
	rasterIDs := make([]string, 0)
	vectorIDs := make([]string, 0)

	// 1. Collect & bind Raster Layers (reusing persistent cached tile directory!)
	if mode == models.PublishModeCombined || mode == models.PublishModeRaster {
		usedSlugs := make(map[string]int)
		rIdx := 0
		for _, ds := range allDatasets {
			if ds == nil || isVectorDataset(ds) {
				continue
			}
			if len(rasterFilter) > 0 && !rasterFilter[ds.ID] {
				continue
			}
			if len(rasterFilter) == 0 && len(genericFilter) > 0 && !genericFilter[ds.ID] {
				continue
			}
			dsBounds := extractDatasetBounds(ds)
			if len(dsBounds) != 4 {
				continue
			}
			expandBounds(dsBounds)

			slug := safeStem(ds.Filename)
			usedSlugs[slug]++
			layerID := slug
			if usedSlugs[slug] > 1 {
				layerID = fmt.Sprintf("%s_%d", slug, usedSlugs[slug])
			}

			tileFmt := strings.ToLower(strings.TrimSpace(req.RasterFormat))
			if tileFmt != "jpg" && tileFmt != "jpeg" {
				tileFmt = "png"
			}

			// Reuse existing loose tiles_xyz or persistent tile_cache without regenerating!
			var tileDir string
			var renderPath string
			if s.exportSvc != nil {
				renderPath = s.exportSvc.resolveExportRasterPath(ds)
			}
			if renderPath == "" || !fileExists(renderPath) {
				renderPath = ds.StoragePath
			}
			dsDir := resolveDatasetDirectory(s.storage, ds)
			candXYZ := filepath.Join(dsDir, "tiles_xyz")
			if dirContainsExtension(candXYZ, "."+tileFmt) {
				tileDir = candXYZ
			} else if s.exportSvc != nil {
				rMinZ := minZoom
				rMaxZ := maxZoom
				if rMinZ < 10 && (dsBounds[2]-dsBounds[0]) < 1.0 {
					rMinZ, _ = gis.DetermineTileZooms(dsBounds, 512, 512)
					if rMinZ > rMaxZ {
						rMinZ = rMaxZ
					}
				}
				cacheKey := computeRasterContentHash(renderPath, dsBounds, rMinZ, rMaxZ, tileFmt, 85)
				cacheDir := s.exportSvc.getTileCacheDir(layerID, cacheKey)
				// Ensure tiles exist in persistent cacheDir (filepath.Base(tmpStage) MUST equal layerID!)
				if !fileExists(filepath.Join(cacheDir, "cache_manifest.json")) {
					tmpParent := filepath.Join(os.TempDir(), "veracity_pub_stage_"+cacheKey)
					tmpStage := filepath.Join(tmpParent, layerID)
					_ = os.MkdirAll(tmpStage, 0755)
					_, _, _ = s.exportSvc.getOrCreateCachedRasterTiles(ctx, renderPath, dsBounds, rMinZ, rMaxZ, tileFmt, 85, tmpStage)
					_ = storage.SafeRemoveAll(tmpParent)
				}
				tileDir = cacheDir
			}

			dsCorners := extractDatasetCorners(ds)

			opacity := 1.0
			visible := true
			order := rIdx
			if m, ok := memberMetaByID[ds.ID]; ok {
				if op := getDim(m["raster_opacity"]); op > 0 && op <= 1.0 {
					opacity = op
				}
				if vis, hasVis := m["visible"].(bool); hasVis {
					visible = vis
				}
				if _, hasSO := m["sort_order"]; hasSO {
					order = int(getDim(m["sort_order"]))
				}
			}

			srcCRS := "EPSG:4326"
			if ds.ExtractedMetadata != nil {
				if c, ok := ds.ExtractedMetadata["crs"].(string); ok && c != "" {
					srcCRS = c
				}
			}

			publishedRasters = append(publishedRasters, models.PublishedRasterLayer{
				LayerID:      layerID,
				DatasetID:    ds.ID,
				Name:         ds.Filename,
				Format:       tileFmt,
				TileDir:      tileDir,
				ImagePath:    renderPath,
				TileURL:      fmt.Sprintf("/tiles/%s/raster/%s/{z}/{x}/{y}.%s", mapID, layerID, tileFmt),
				MetadataURL:  fmt.Sprintf("/tiles/%s/raster/%s/metadata.json", mapID, layerID),
				SourceCRS:    srcCRS,
				Bounds:       dsBounds,
				Corners:      dsCorners,
				MinZoom:      minZoom,
				MaxZoom:      maxZoom,
				Opacity:      opacity,
				Visible:      visible,
				DisplayOrder: order,
			})
			rasterIDs = append(rasterIDs, layerID)
			rIdx++
		}
	}

	// 2. Collect & bind Vector Layers (reusing existing vector_tiles/<slug>/ without duplicating!)
	if mode == models.PublishModeCombined || mode == models.PublishModeVector {
		usedVecSlugs := make(map[string]int)
		seenKeys := make(map[string]bool)

		addVectorLayer := func(
			dsID, gisID, name, rawSlug, layerDir, srcCRS, geomType string,
			geomTypes []string, featCount int, bounds []float64,
			vMinZ, vMaxZ int, opacity float64, visible bool, order int,
			fields []map[string]interface{}, styleDoc map[string]interface{},
		) {
			slug := safeStem(rawSlug)
			if slug == "" {
				slug = safeStem(name)
			}
			usedVecSlugs[slug]++
			layerID := slug
			if usedVecSlugs[slug] > 1 {
				layerID = fmt.Sprintf("%s_%d", slug, usedVecSlugs[slug])
			}
			if len(bounds) == 4 {
				expandBounds(bounds)
			}
			if styleDoc == nil {
				if rawS, err := os.ReadFile(filepath.Join(layerDir, "style.json")); err == nil {
					_ = json.Unmarshal(rawS, &styleDoc)
				}
			}
			if srcCRS == "" {
				srcCRS = "EPSG:4326"
			}
			if vMaxZ <= 0 {
				vMaxZ = 14
			}
			publishedVectors = append(publishedVectors, models.PublishedVectorLayer{
				LayerID:       layerID,
				DatasetID:     dsID,
				GISLayerID:    gisID,
				Name:          name,
				SourceLayer:   slug,
				Format:        "pbf",
				TileDir:       layerDir,
				TileURL:       fmt.Sprintf("/tiles/%s/vector/%s/{z}/{x}/{y}.pbf", mapID, layerID),
				MetadataURL:   fmt.Sprintf("/tiles/%s/vector/%s/metadata.json", mapID, layerID),
				SourceCRS:     srcCRS,
				GeometryType:  firstNonEmptyBase(geomType, "Geometry"),
				GeometryTypes: geomTypes,
				FeatureCount:  featCount,
				Bounds:        bounds,
				MinZoom:       vMinZ,
				MaxZoom:       vMaxZ,
				Opacity:       opacity,
				Visible:       visible,
				DisplayOrder:  order,
				PromoteID:     "feature_id",
				Interactive:   true,
				Fields:        fields,
				Style:         styleDoc,
			})
			vectorIDs = append(vectorIDs, layerID)
		}

		// First: active GIS vector layers in workspace
		for idx, gl := range gisLayers {
			if gl == nil || (gl.LayerType != "" && gl.LayerType != "vector") {
				continue
			}
			if len(vectorFilter) > 0 && !vectorFilter[gl.LayerID] && !vectorFilter[gl.DatasetID] {
				continue
			}
			if len(vectorFilter) == 0 && len(genericFilter) > 0 && !genericFilter[gl.LayerID] && !genericFilter[gl.DatasetID] {
				continue
			}
			vtMap, _ := gl.Metadata["vector_tiles"].(map[string]interface{})
			if vtMap == nil {
				continue
			}
			layerDir, _ := vtMap["layer_dir"].(string)
			slug, _ := vtMap["layer_slug"].(string)
			if layerDir == "" || !dirExists(layerDir) {
				if ds := datasetMap[gl.DatasetID]; ds != nil && ds.StoragePath != "" && slug != "" {
					cand := filepath.Join(filepath.Dir(ds.StoragePath), "vector_tiles", slug)
					if dirExists(cand) {
						layerDir = cand
					}
				}
			}
			if layerDir == "" || !dirExists(layerDir) {
				continue
			}
			var styleMap map[string]interface{}
			if gl.Style != nil {
				if b, err := json.Marshal(gl.Style); err == nil {
					_ = json.Unmarshal(b, &styleMap)
				}
			}
			var fields []map[string]interface{}
			if rawF, ok := vtMap["fields"]; ok {
				if b, err := json.Marshal(rawF); err == nil {
					_ = json.Unmarshal(b, &fields)
				}
			}
			var gTypes []string
			if rawGT, ok := vtMap["geometry_types"]; ok {
				if b, err := json.Marshal(rawGT); err == nil {
					_ = json.Unmarshal(b, &gTypes)
				}
			}
			srcCRS, _ := vtMap["source_crs"].(string)
			if srcCRS == "" {
				srcCRS = gl.CRS
			}
			addVectorLayer(
				gl.DatasetID, gl.LayerID, gl.LayerName, slug, layerDir,
				srcCRS, gl.GeometryType, gTypes, gl.FeatureCount, gl.Bounds,
				int(getDim(vtMap["min_zoom"])), int(getDim(vtMap["max_zoom"])),
				gl.Opacity, gl.IsVisible, idx, fields, styleMap,
			)
			seenKeys[gl.DatasetID+"::"+slug] = true
			seenKeys[gl.DatasetID+"::"+gl.LayerName] = true
		}

		// Second: any vector datasets (e.g. GeoPackages) not already covered via GIS layers
		for _, ds := range allDatasets {
			if ds == nil || !isVectorDataset(ds) || ds.StoragePath == "" {
				continue
			}
			if len(vectorFilter) > 0 && !vectorFilter[ds.ID] {
				continue
			}
			if len(vectorFilter) == 0 && len(genericFilter) > 0 && !genericFilter[ds.ID] {
				continue
			}
			vtRoot := filepath.Join(filepath.Dir(ds.StoragePath), "vector_tiles")
			if !dirExists(vtRoot) && fileExists(ds.StoragePath) {
				_, _ = gis.ConvertDatasetToVectorTiles(ds.StoragePath, vtRoot, nil, 0, min(maxZoom, 14))
			}
			if entries, err := os.ReadDir(vtRoot); err == nil {
				for _, ent := range entries {
					if !ent.IsDir() {
						continue
					}
					slug := ent.Name()
					if seenKeys[ds.ID+"::"+slug] {
						continue
					}
					layerDir := filepath.Join(vtRoot, slug)
					var meta map[string]interface{}
					if raw, rErr := os.ReadFile(filepath.Join(layerDir, "metadata.json")); rErr == nil {
						_ = json.Unmarshal(raw, &meta)
					}
					displayName := slug
					srcCRS := "EPSG:4326"
					geomType := "Geometry"
					featCount := 0
					vMinZ, vMaxZ := 0, 14
					var bounds []float64
					var fields []map[string]interface{}
					var gTypes []string
					if meta != nil {
						if ln, _ := meta["layer_name"].(string); ln != "" {
							displayName = ln
						}
						if seenKeys[ds.ID+"::"+displayName] {
							continue
						}
						if sc, _ := meta["source_crs"].(string); sc != "" {
							srcCRS = sc
						}
						if gt, _ := meta["geometry_type"].(string); gt != "" {
							geomType = gt
						}
						featCount = int(getDim(meta["feature_count"]))
						vMinZ = int(getDim(meta["min_zoom"]))
						if mz := int(getDim(meta["max_zoom"])); mz > 0 {
							vMaxZ = mz
						}
						if bRaw, ok := meta["bounds"].([]interface{}); ok && len(bRaw) == 4 {
							bounds = []float64{getDim(bRaw[0]), getDim(bRaw[1]), getDim(bRaw[2]), getDim(bRaw[3])}
						}
						if rawF, ok := meta["fields"]; ok {
							if b, err := json.Marshal(rawF); err == nil {
								_ = json.Unmarshal(b, &fields)
							}
						}
						if rawGT, ok := meta["geometry_types"]; ok {
							if b, err := json.Marshal(rawGT); err == nil {
								_ = json.Unmarshal(b, &gTypes)
							}
						}
					}
					addVectorLayer(
						ds.ID, "", displayName, slug, layerDir,
						srcCRS, geomType, gTypes, featCount, bounds,
						vMinZ, vMaxZ, 1.0, true, len(publishedVectors), fields, nil,
					)
					seenKeys[ds.ID+"::"+slug] = true
				}
			}
		}
	}

	if mode == models.PublishModeRaster && len(publishedRasters) == 0 {
		return nil, fmt.Errorf("%w: no eligible georeferenced raster layers found to publish", models.ErrInvalidPublishRequest)
	}
	if mode == models.PublishModeVector && len(publishedVectors) == 0 {
		return nil, fmt.Errorf("%w: no eligible vector tile layers found to publish", models.ErrInvalidPublishRequest)
	}
	if mode == models.PublishModeCombined && len(publishedRasters) == 0 && len(publishedVectors) == 0 {
		return nil, fmt.Errorf("%w: no eligible raster or vector layers found to publish", models.ErrInvalidPublishRequest)
	}

	if len(unionBounds) != 4 {
		unionBounds = []float64{-180, -85, 180, 85}
	}
	center := req.Center
	if len(center) != 2 {
		center = []float64{
			(unionBounds[0] + unionBounds[2]) / 2.0,
			(unionBounds[1] + unionBounds[3]) / 2.0,
		}
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		if primaryProj != nil && primaryProj.Name != "" {
			name = primaryProj.Name
		} else {
			name = fmt.Sprintf("Published %s Map", strings.ToUpper(mode[:1])+mode[1:])
		}
	}

	now := time.Now().UTC()
	layerOrder := normalizePublishedLayerOrder(req.LayerOrder)
	resolvedStack := resolvePublishedLayerStack(req.LayerStack, publishedRasters, publishedVectors, layerOrder)

	pm := &models.PublishedMap{
		MapID:          mapID,
		OwnerID:        ownerID,
		ProjectID:      req.ProjectID,
		WorkspaceID:    workspaceID,
		Name:           name,
		Mode:           mode,
		IsPublic:       isPublic,
		AccessToken:    accessToken,
		Revoked:        false,
		RasterLayerIDs: rasterIDs,
		VectorLayerIDs: vectorIDs,
		RasterLayers:   publishedRasters,
		VectorLayers:   publishedVectors,
		LayerOrder:     layerOrder,
		LayerStack:     resolvedStack,
		Bounds:         unionBounds,
		Center:         center,
		InitialZoom:    initZoom,
		MinZoom:        minZoom,
		MaxZoom:        maxZoom,
		InteractionConfig: map[string]interface{}{
			"interactive_points": true,
			"hover_highlight":    true,
			"attribute_popups":   true,
			"query_method":       "queryRenderedFeatures",
			"promote_id":         "feature_id",
		},
		MapURL:    fmt.Sprintf("/maps/%s", mapID),
		StyleURL:  fmt.Sprintf("/maps/%s/style.json", mapID),
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	s.records[mapID] = pm
	s.persistToDiskLocked()
	s.mu.Unlock()

	return pm, nil
}

// GetPublishedMap validates access permissions (public, owner, or valid restricted token) and revocation status.
func (s *PublishService) GetPublishedMap(mapID, requestUserID, providedToken string) (*models.PublishedMap, error) {
	s.mu.RLock()
	pm, ok := s.records[mapID]
	s.mu.RUnlock()
	if !ok || pm == nil {
		return nil, models.ErrPublishedMapNotFound
	}
	if pm.Revoked {
		return nil, models.ErrPublishedMapRevoked
	}
	if !pm.IsPublic {
		isOwner := requestUserID != "" && pm.OwnerID != "" && requestUserID == pm.OwnerID
		tokenOK := providedToken != "" && pm.AccessToken != "" && providedToken == pm.AccessToken
		if !isOwner && !tokenOK {
			return nil, models.ErrPublishedMapAccessDenied
		}
	}
	return pm, nil
}

// ListPublishedMaps returns all published maps owned by userID (optionally filtered by workspaceID).
func (s *PublishService) ListPublishedMaps(ownerID, workspaceID string) []*models.PublishedMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*models.PublishedMap, 0)
	for _, pm := range s.records {
		if pm == nil {
			continue
		}
		if ownerID != "" && pm.OwnerID != ownerID {
			continue
		}
		if workspaceID != "" && pm.WorkspaceID != workspaceID && pm.ProjectID != workspaceID {
			continue
		}
		out = append(out, pm)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// RevokePublishedMap revokes a published map (owner-only).
func (s *PublishService) RevokePublishedMap(mapID, ownerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pm, ok := s.records[mapID]
	if !ok || pm == nil {
		return models.ErrPublishedMapNotFound
	}
	if pm.OwnerID != "" && ownerID != "" && pm.OwnerID != ownerID {
		return models.ErrPublishedMapAccessDenied
	}
	now := time.Now().UTC()
	pm.Revoked = true
	pm.RevokedAt = &now
	pm.UpdatedAt = now
	s.persistToDiskLocked()
	return nil
}

// normalizePublishedLayerOrder ensures a valid top-to-bottom group stacking order
// ("vector" above "raster" by default, or "raster" above "vector" when requested).
func normalizePublishedLayerOrder(order []string) []string {
	for _, g := range order {
		switch strings.ToLower(strings.TrimSpace(g)) {
		case "raster":
			return []string{"raster", "vector", "base"}
		case "vector":
			return []string{"vector", "raster", "base"}
		}
	}
	return []string{"vector", "raster", "base"}
}

// resolvePublishedLayerStack maps the caller's top-to-bottom Photoshop-style layer_stack
// onto the concrete published raster and vector layer_ids, appending any unmatched layers
// in their default group order so nothing is ever lost.
func resolvePublishedLayerStack(
	requested []models.PublishedLayerStackItem,
	rasters []models.PublishedRasterLayer,
	vectors []models.PublishedVectorLayer,
	fallbackOrder []string,
) []models.PublishedLayerStackItem {
	out := make([]models.PublishedLayerStackItem, 0, len(rasters)+len(vectors))
	usedRaster := make(map[string]bool, len(rasters))
	usedVector := make(map[string]bool, len(vectors))

	for _, item := range requested {
		kind := strings.ToLower(strings.TrimSpace(item.Kind))
		id := strings.TrimSpace(item.ID)
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if kind == "raster" {
			for _, rl := range rasters {
				if usedRaster[rl.LayerID] {
					continue
				}
				if (id != "" && (rl.LayerID == id || rl.DatasetID == id)) ||
					(name != "" && (strings.ToLower(rl.Name) == name || strings.ToLower(rl.LayerID) == name)) {
					usedRaster[rl.LayerID] = true
					out = append(out, models.PublishedLayerStackItem{Kind: "raster", ID: rl.LayerID, Name: rl.Name})
					break
				}
			}
		} else if kind == "vector" {
			for _, vl := range vectors {
				if usedVector[vl.LayerID] {
					continue
				}
				if (id != "" && (vl.LayerID == id || vl.GISLayerID == id || vl.DatasetID == id)) ||
					(name != "" && (strings.ToLower(vl.SourceLayer) == name || strings.ToLower(vl.Name) == name || strings.ToLower(vl.LayerID) == name)) {
					usedVector[vl.LayerID] = true
					out = append(out, models.PublishedLayerStackItem{Kind: "vector", ID: vl.LayerID, Name: vl.Name})
					break
				}
			}
		}
	}

	appendRemainingRasters := func() {
		for _, rl := range rasters {
			if !usedRaster[rl.LayerID] {
				usedRaster[rl.LayerID] = true
				out = append(out, models.PublishedLayerStackItem{Kind: "raster", ID: rl.LayerID, Name: rl.Name})
			}
		}
	}
	appendRemainingVectors := func() {
		for _, vl := range vectors {
			if !usedVector[vl.LayerID] {
				usedVector[vl.LayerID] = true
				out = append(out, models.PublishedLayerStackItem{Kind: "vector", ID: vl.LayerID, Name: vl.Name})
			}
		}
	}

	if len(fallbackOrder) > 0 && fallbackOrder[0] == "raster" {
		appendRemainingRasters()
		appendRemainingVectors()
	} else {
		appendRemainingVectors()
		appendRemainingRasters()
	}
	return out
}

// BuildMapLibreStyle generates a MapLibre Style Spec v8 JSON document referencing the published
// base, raster, and vector tile sources with preserved ordering, styles, and interaction metadata.
func (s *PublishService) BuildMapLibreStyle(pm *models.PublishedMap, baseURL, tokenQuery string) map[string]interface{} {
	base := strings.TrimSuffix(baseURL, "/")
	suffix := ""
	if tokenQuery != "" {
		suffix = "?token=" + tokenQuery
	}

	sources := map[string]interface{}{
		"osm-base": map[string]interface{}{
			"type":        "raster",
			"tiles":       []string{"https://tile.openstreetmap.org/{z}/{x}/{y}.png"},
			"tileSize":    256,
			"attribution": "© OpenStreetMap contributors",
		},
	}

	layers := []map[string]interface{}{
		{
			"id":     "osm-base-layer",
			"type":   "raster",
			"source": "osm-base",
			"layout": map[string]interface{}{"visibility": "visible"},
			"paint":  map[string]interface{}{"raster-opacity": 1.0},
		},
	}

	// 1. Build Raster layer definitions
	sortedRasters := append([]models.PublishedRasterLayer(nil), pm.RasterLayers...)
	sort.Slice(sortedRasters, func(i, j int) bool {
		return sortedRasters[i].DisplayOrder < sortedRasters[j].DisplayOrder
	})
	rasterMapLayers := make([]map[string]interface{}, 0, len(sortedRasters))
	for _, rl := range sortedRasters {
		srcID := "pub-raster-" + rl.LayerID
		tileURL := base + rl.TileURL + suffix
		srcDef := map[string]interface{}{
			"type":     "raster",
			"tiles":    []string{tileURL},
			"tileSize": 256,
			"scheme":   "xyz",
			"minzoom":  rl.MinZoom,
			"maxzoom":  rl.MaxZoom,
		}
		if len(rl.Bounds) == 4 {
			srcDef["bounds"] = rl.Bounds
		}
		sources[srcID] = srcDef

		vis := "visible"
		if !rl.Visible {
			vis = "none"
		}
		op := rl.Opacity
		if op <= 0 {
			op = 1.0
		}
		rasterMapLayers = append(rasterMapLayers, map[string]interface{}{
			"id":     "pub-raster-lyr-" + rl.LayerID,
			"type":   "raster",
			"source": srcID,
			"layout": map[string]interface{}{"visibility": vis},
			"paint":  map[string]interface{}{"raster-opacity": op},
		})
	}

	// 2. Vector layers (top: Polygons -> Lines -> Points)
	sortedVectors := append([]models.PublishedVectorLayer(nil), pm.VectorLayers...)
	sort.Slice(sortedVectors, func(i, j int) bool {
		return sortedVectors[i].DisplayOrder < sortedVectors[j].DisplayOrder
	})

	var fillLayers, lineLayers, circleLayers []map[string]interface{}
	for idx, vl := range sortedVectors {
		srcID := "pub-vector-" + vl.LayerID
		tileURL := base + vl.TileURL + suffix
		srcDef := map[string]interface{}{
			"type":      "vector",
			"tiles":     []string{tileURL},
			"scheme":    "xyz",
			"minzoom":   vl.MinZoom,
			"maxzoom":   vl.MaxZoom,
			"promoteId": firstNonEmptyBase(vl.PromoteID, "feature_id"),
		}
		if len(vl.Bounds) == 4 {
			srcDef["bounds"] = vl.Bounds
		}
		sources[srcID] = srcDef

		vis := "visible"
		if !vl.Visible {
			vis = "none"
		}
		op := vl.Opacity
		if op <= 0 {
			op = 1.0
		}
		color := dynamicLayerPalette[idx%len(dynamicLayerPalette)]
		if vl.Style != nil {
			if c, _ := vl.Style["color"].(string); c != "" {
				color = c
			}
			if ml, ok := vl.Style["maplibre_style"].(map[string]interface{}); ok {
				if c, _ := ml["circle"].(map[string]interface{})["circle-color"].(string); c != "" {
					color = c
				} else if c, _ := ml["fill"].(map[string]interface{})["fill-color"].(string); c != "" {
					color = c
				} else if c, _ := ml["line"].(map[string]interface{})["line-color"].(string); c != "" {
					color = c
				}
			}
		}

		fillLayers = append(fillLayers, map[string]interface{}{
			"id":           "pub-vec-fill-" + vl.LayerID,
			"type":         "fill",
			"source":       srcID,
			"source-layer": vl.SourceLayer,
			"filter":       []interface{}{"in", []interface{}{"geometry-type"}, []interface{}{"literal", []string{"Polygon", "MultiPolygon"}}},
			"layout":       map[string]interface{}{"visibility": vis},
			"paint": map[string]interface{}{
				"fill-color":         color,
				"fill-opacity":       0.32 * op,
				"fill-outline-color": color,
			},
		})
		lineLayers = append(lineLayers, map[string]interface{}{
			"id":           "pub-vec-line-" + vl.LayerID,
			"type":         "line",
			"source":       srcID,
			"source-layer": vl.SourceLayer,
			"filter":       []interface{}{"in", []interface{}{"geometry-type"}, []interface{}{"literal", []string{"LineString", "MultiLineString", "Polygon", "MultiPolygon"}}},
			"layout":       map[string]interface{}{"visibility": vis},
			"paint": map[string]interface{}{
				"line-color":   color,
				"line-width":   2.2,
				"line-opacity": 0.92 * op,
			},
		})
		circleLayers = append(circleLayers, map[string]interface{}{
			"id":           "pub-vec-circle-" + vl.LayerID,
			"type":         "circle",
			"source":       srcID,
			"source-layer": vl.SourceLayer,
			"filter":       []interface{}{"in", []interface{}{"geometry-type"}, []interface{}{"literal", []string{"Point", "MultiPoint"}}},
			"layout":       map[string]interface{}{"visibility": vis},
			"paint": map[string]interface{}{
				"circle-color":        color,
				"circle-radius":       []interface{}{"interpolate", []interface{}{"linear"}, []interface{}{"zoom"}, 3, 4, 8, 5.5, 12, 7, 16, 9.5},
				"circle-stroke-color": "#ffffff",
				"circle-stroke-width": 2,
				"circle-opacity":      0.95 * op,
			},
		})
	}

	// Append raster and vector layers bottom-to-top.
	// If pm.LayerStack is present, honor exact per-layer interleaved Photoshop order!
	if len(pm.LayerStack) > 0 {
		rasterByID := make(map[string]map[string]interface{}, len(rasterMapLayers))
		for _, rl := range rasterMapLayers {
			if id, _ := rl["id"].(string); id != "" {
				rasterByID[strings.TrimPrefix(id, "pub-raster-lyr-")] = rl
			}
		}
		fillByID := make(map[string]map[string]interface{}, len(fillLayers))
		lineByID := make(map[string]map[string]interface{}, len(lineLayers))
		circleByID := make(map[string]map[string]interface{}, len(circleLayers))
		for _, fl := range fillLayers {
			if id, _ := fl["id"].(string); id != "" {
				fillByID[strings.TrimPrefix(id, "pub-vec-fill-")] = fl
			}
		}
		for _, ll := range lineLayers {
			if id, _ := ll["id"].(string); id != "" {
				lineByID[strings.TrimPrefix(id, "pub-vec-line-")] = ll
			}
		}
		for _, cl := range circleLayers {
			if id, _ := cl["id"].(string); id != "" {
				circleByID[strings.TrimPrefix(id, "pub-vec-circle-")] = cl
			}
		}
		// Reverse LayerStack (top-to-bottom -> bottom-to-top for MapLibre)
		for i := len(pm.LayerStack) - 1; i >= 0; i-- {
			item := pm.LayerStack[i]
			if item.Kind == "raster" {
				if lyr, ok := rasterByID[item.ID]; ok {
					layers = append(layers, lyr)
				}
			} else if item.Kind == "vector" {
				if fl, ok := fillByID[item.ID]; ok {
					layers = append(layers, fl)
				}
				if ll, ok := lineByID[item.ID]; ok {
					layers = append(layers, ll)
				}
				if cl, ok := circleByID[item.ID]; ok {
					layers = append(layers, cl)
				}
			}
		}
	} else {
		// Group-level fallback ordering
		effectiveOrder := normalizePublishedLayerOrder(pm.LayerOrder)
		if effectiveOrder[0] == "raster" {
			layers = append(layers, fillLayers...)
			layers = append(layers, lineLayers...)
			layers = append(layers, circleLayers...)
			layers = append(layers, rasterMapLayers...)
		} else {
			layers = append(layers, rasterMapLayers...)
			layers = append(layers, fillLayers...)
			layers = append(layers, lineLayers...)
			layers = append(layers, circleLayers...)
		}
	}

	return map[string]interface{}{
		"version": 8,
		"name":    pm.Name,
		"center":  pm.Center,
		"zoom":    pm.InitialZoom,
		"sources": sources,
		"layers":  layers,
		"metadata": map[string]interface{}{
			"veracity_map_id":      pm.MapID,
			"veracity_mode":        pm.Mode,
			"veracity_layer_order": pm.LayerOrder,
			"veracity_interaction": pm.InteractionConfig,
			"bounds":               pm.Bounds,
		},
	}
}

// BuildLayerTileJSON returns TileJSON 3.0 metadata for an individual published raster or vector layer.
func (s *PublishService) BuildLayerTileJSON(pm *models.PublishedMap, layerType, layerID, baseURL, tokenQuery string) (map[string]interface{}, error) {
	base := strings.TrimSuffix(baseURL, "/")
	suffix := ""
	if tokenQuery != "" {
		suffix = "?token=" + tokenQuery
	}

	switch strings.ToLower(layerType) {
	case "raster":
		for _, rl := range pm.RasterLayers {
			if rl.LayerID == layerID {
				return map[string]interface{}{
					"tilejson":    "3.0.0",
					"name":        rl.Name,
					"id":          rl.LayerID,
					"type":        "raster",
					"format":      rl.Format,
					"scheme":      "xyz",
					"crs":         "EPSG:3857",
					"source_crs":  rl.SourceCRS,
					"tiles":       []string{base + rl.TileURL + suffix},
					"bounds":      rl.Bounds,
					"center":      pm.Center,
					"minzoom":     rl.MinZoom,
					"maxzoom":     rl.MaxZoom,
					"opacity":     rl.Opacity,
					"visible":     rl.Visible,
					"attribution": "Veracity Maps Published Raster Layer",
				}, nil
			}
		}
		return nil, models.ErrPublishedMapNotFound

	case "vector":
		for _, vl := range pm.VectorLayers {
			if vl.LayerID == layerID {
				fieldMap := make(map[string]string, len(vl.Fields))
				for _, f := range vl.Fields {
					if fn, _ := f["name"].(string); fn != "" {
						ft, _ := f["type"].(string)
						if ft == "" {
							ft = "String"
						}
						fieldMap[fn] = ft
					}
				}
				return map[string]interface{}{
					"tilejson":      "3.0.0",
					"name":          vl.Name,
					"id":            vl.LayerID,
					"type":          "vector",
					"format":        "pbf",
					"encoding":      "gzip",
					"scheme":        "xyz",
					"crs":           "EPSG:3857",
					"source_crs":    vl.SourceCRS,
					"tiles":         []string{base + vl.TileURL + suffix},
					"bounds":        vl.Bounds,
					"center":        pm.Center,
					"minzoom":       vl.MinZoom,
					"maxzoom":       vl.MaxZoom,
					"geometry_type": vl.GeometryType,
					"feature_count": vl.FeatureCount,
					"source_layer":  vl.SourceLayer,
					"promote_id":    firstNonEmptyBase(vl.PromoteID, "feature_id"),
					"interactive":   vl.Interactive,
					"vector_layers": []map[string]interface{}{
						{
							"id":            vl.SourceLayer,
							"description":   vl.Name,
							"minzoom":       vl.MinZoom,
							"maxzoom":       vl.MaxZoom,
							"geometry_type": vl.GeometryType,
							"fields":        fieldMap,
						},
					},
					"attribution": "Veracity Maps Published Vector Layer",
				}, nil
			}
		}
		return nil, models.ErrPublishedMapNotFound
	default:
		return nil, fmt.Errorf("%w: unsupported layer type '%s'", models.ErrInvalidPublishRequest, layerType)
	}
}

// extractDatasetCorners derives the 4 WGS84 [lng, lat] corners [TL, TR, BR, BL]
// from a dataset's affine transform (or explicit corners metadata) so rotated
// georeferenced rasters render with exact orientation in published & composite tiles.
func extractDatasetCorners(ds *models.Dataset) [][]float64 {
	if ds == nil || ds.ExtractedMetadata == nil {
		return nil
	}
	// 1. Explicit corners in ExtractedMetadata["corners"]
	if rawCorners, ok := ds.ExtractedMetadata["corners"]; ok {
		if cList, ok2 := rawCorners.([][]float64); ok2 && len(cList) == 4 {
			return cList
		}
		if ifaceList, ok2 := rawCorners.([]interface{}); ok2 && len(ifaceList) == 4 {
			out := make([][]float64, 0, 4)
			for _, item := range ifaceList {
				if pair, ok3 := item.([]interface{}); ok3 && len(pair) == 2 {
					out = append(out, []float64{getDim(pair[0]), getDim(pair[1])})
				}
			}
			if len(out) == 4 {
				return out
			}
		}
	}
	// 2. Compute from 6-element affine transform [a, -b, c, d, -e, f] + width/height
	var t []float64
	if tf, ok := ds.ExtractedMetadata["transform"].([]float64); ok && len(tf) == 6 {
		t = tf
	} else if ti, ok := ds.ExtractedMetadata["transform"].([]interface{}); ok && len(ti) == 6 {
		t = make([]float64, 6)
		for i := 0; i < 6; i++ {
			t[i] = getDim(ti[i])
		}
	}
	if len(t) == 6 {
		w := getDim(ds.ExtractedMetadata["width"])
		h := getDim(ds.ExtractedMetadata["height"])
		if w > 0 && h > 0 {
			a, b, c := t[0], -t[1], t[2]
			d, e, f := t[3], -t[4], t[5]
			return [][]float64{
				{c, f},
				{a*w + c, d*w + f},
				{a*w + b*h + c, d*w + e*h + f},
				{b*h + c, e*h + f},
			}
		}
	}
	return nil
}

// ResolvePublishedTilePath locates the tile file on disk for a published raster or vector layer.
func (s *PublishService) ResolvePublishedTilePath(pm *models.PublishedMap, layerType, layerID, zStr, xStr, yClean string) (string, string, error) {
	switch strings.ToLower(layerType) {
	case "raster":
		for _, rl := range pm.RasterLayers {
			if rl.LayerID == layerID {
				candidateDirs := make([]string, 0, 2)
				if rl.TileDir != "" {
					candidateDirs = append(candidateDirs, rl.TileDir)
					// Backwards-compat fallback for previously published maps whose tiles were written under veracity_pub_stage_<base>_<hash>
					baseName := filepath.Base(rl.TileDir)
					if idx := strings.LastIndex(baseName, "_"); idx > 0 {
						hashPart := baseName[idx+1:]
						legacyDir := filepath.Join(filepath.Dir(rl.TileDir), "veracity_pub_stage_"+baseName+"_"+hashPart)
						if dirExists(legacyDir) {
							candidateDirs = append(candidateDirs, legacyDir)
						}
					}
				}
				for _, dir := range candidateDirs {
					for _, ext := range []string{"." + rl.Format, ".png", ".jpg", ".jpeg"} {
						cand := filepath.Join(dir, zStr, xStr, yClean+ext)
						if fileExists(cand) {
							return cand, ext, nil
						}
					}
				}
				return "", "", os.ErrNotExist
			}
		}
		return "", "", models.ErrPublishedMapNotFound

	case "vector":
		for _, vl := range pm.VectorLayers {
			if vl.LayerID == layerID {
				if vl.TileDir == "" {
					return "", "", os.ErrNotExist
				}
				candidates := []string{
					filepath.Join(vl.TileDir, "tiles", zStr, xStr, yClean+".pbf"),
					filepath.Join(vl.TileDir, "tiles", zStr, xStr, yClean+".mvt"),
					filepath.Join(vl.TileDir, zStr, xStr, yClean+".pbf"),
				}
				for _, cand := range candidates {
					if fileExists(cand) {
						return cand, filepath.Ext(cand), nil
					}
				}
				return "", "", os.ErrNotExist
			}
		}
		return "", "", models.ErrPublishedMapNotFound
	default:
		return "", "", models.ErrPublishedMapNotFound
	}
}

// CollectPublishedVectorTileFiles scans all published vector layers in pm and maps each
// relative "z/x/y.pbf" key to the list of on-disk .pbf tile paths that contribute to it.
func (s *PublishService) CollectPublishedVectorTileFiles(pm *models.PublishedMap) map[string][]string {
	out := make(map[string][]string)
	if pm == nil {
		return out
	}
	seenSourceLayers := make(map[string]bool)
	for _, vl := range pm.VectorLayers {
		if vl.TileDir == "" {
			continue
		}
		// Deduplicate identical source layers that came from repeated dataset uploads
		slKey := strings.ToLower(strings.TrimSpace(vl.SourceLayer))
		if slKey != "" && seenSourceLayers[slKey] {
			continue
		}
		if slKey != "" {
			seenSourceLayers[slKey] = true
		}

		tilesRoot := filepath.Join(vl.TileDir, "tiles")
		if !dirExists(tilesRoot) {
			tilesRoot = vl.TileDir
		}
		if !dirExists(tilesRoot) {
			continue
		}
		_ = filepath.Walk(tilesRoot, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext != ".pbf" && ext != ".mvt" {
				return nil
			}
			rel, rErr := filepath.Rel(tilesRoot, p)
			if rErr != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			if strings.HasSuffix(strings.ToLower(relSlash), ".mvt") {
				relSlash = relSlash[:len(relSlash)-4] + ".pbf"
			}
			out[relSlash] = append(out[relSlash], p)
			return nil
		})
	}
	return out
}

func dirContainsExtension(root, ext string) bool {
	if !dirExists(root) {
		return false
	}
	found := false
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

var (
	transparent256Once sync.Once
	transparent256PNG  []byte
)

func getTransparent256PNG() []byte {
	transparent256Once.Do(func() {
		img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		transparent256PNG = buf.Bytes()
	})
	return transparent256PNG
}

func xyzTileToWGS84Bounds(z, x, y int) (west, south, east, north float64) {
	n := math.Exp2(float64(z))
	west = float64(x)/n*360.0 - 180.0
	east = float64(x+1)/n*360.0 - 180.0
	latN := math.Atan(math.Sinh(math.Pi * (1.0 - 2.0*float64(y)/n)))
	latS := math.Atan(math.Sinh(math.Pi * (1.0 - 2.0*float64(y+1)/n)))
	north = latN * 180.0 / math.Pi
	south = latS * 180.0 / math.Pi
	return west, south, east, north
}

func (s *PublishService) resolveCompositeSpecPath(pm *models.PublishedMap) (string, error) {
	cacheRoot := filepath.Join(filepath.Dir(s.storePath), "composite_cache", pm.MapID)
	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		return "", err
	}
	specPath := filepath.Join(cacheRoot, "spec.json")
	if fileExists(specPath) {
		return specPath, nil
	}

	rasterSpecs := make([]map[string]interface{}, 0, len(pm.RasterLayers))
	for _, rl := range pm.RasterLayers {
		imgPath := rl.ImagePath
		if imgPath != "" && !fileExists(imgPath) {
			imgPath = ""
		}
		// Fallback 1: look up active dataset by DatasetID to find source/processed raster image
		if imgPath == "" && rl.DatasetID != "" && s.sessionRepo != nil {
			for _, ds := range s.sessionRepo.ListAllDatasets() {
				if ds != nil && ds.ID == rl.DatasetID {
					if s.exportSvc != nil {
						if rp := s.exportSvc.resolveExportRasterPath(ds); rp != "" && fileExists(rp) {
							imgPath = rp
						}
					}
					if imgPath == "" && ds.StoragePath != "" && fileExists(ds.StoragePath) {
						imgPath = ds.StoragePath
					}
					if len(rl.Corners) == 0 {
						rl.Corners = extractDatasetCorners(ds)
					}
					break
				}
			}
		}
		// Fallback 2: check parent directory of TileDir
		if imgPath == "" && rl.TileDir != "" {
			parent := filepath.Dir(rl.TileDir)
			if entries, err := os.ReadDir(parent); err == nil {
				for _, ent := range entries {
					if ent.IsDir() {
						continue
					}
					ext := strings.ToLower(filepath.Ext(ent.Name()))
					if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".tif" || ext == ".tiff" {
						imgPath = filepath.Join(parent, ent.Name())
						break
					}
				}
			}
		}
		effectiveTileDir := rl.TileDir
		if effectiveTileDir != "" && !dirContainsExtension(effectiveTileDir, "."+rl.Format) && !dirContainsExtension(effectiveTileDir, ".png") {
			baseName := filepath.Base(effectiveTileDir)
			if idx := strings.LastIndex(baseName, "_"); idx > 0 {
				hashPart := baseName[idx+1:]
				legacyDir := filepath.Join(filepath.Dir(effectiveTileDir), "veracity_pub_stage_"+baseName+"_"+hashPart)
				if dirExists(legacyDir) {
					effectiveTileDir = legacyDir
				}
			}
		}
		rasterSpecs = append(rasterSpecs, map[string]interface{}{
			"layer_id":   rl.LayerID,
			"tile_dir":   effectiveTileDir,
			"image_path": imgPath,
			"bounds":     rl.Bounds,
			"corners":    rl.Corners,
			"opacity":    rl.Opacity,
			"visible":    rl.Visible,
		})
	}

	vectorSpecs := make([]map[string]interface{}, 0, len(pm.VectorLayers))
	for idx, vl := range pm.VectorLayers {
		srcFile := ""
		if vl.TileDir != "" {
			// vl.TileDir is .../datasets/<ds_id>/vector_tiles/<slug> -> parent of vector_tiles is dataset dir
			dsParent := filepath.Dir(filepath.Dir(vl.TileDir))
			if entries, err := os.ReadDir(dsParent); err == nil {
				for _, ent := range entries {
					if ent.IsDir() {
						continue
					}
					ext := strings.ToLower(filepath.Ext(ent.Name()))
					if ext == ".gpkg" || ext == ".geojson" || ext == ".shp" || ext == ".kml" || ext == ".fgb" {
						srcFile = filepath.Join(dsParent, ent.Name())
						break
					}
				}
			}
		}
		if srcFile == "" {
			continue
		}
		color := dynamicLayerPalette[idx%len(dynamicLayerPalette)]
		if vl.Style != nil {
			if c, _ := vl.Style["color"].(string); c != "" {
				color = c
			}
			if ml, ok := vl.Style["maplibre_style"].(map[string]interface{}); ok {
				if cm, ok2 := ml["circle"].(map[string]interface{}); ok2 {
					if c, _ := cm["circle-color"].(string); c != "" {
						color = c
					}
				} else if fm, ok2 := ml["fill"].(map[string]interface{}); ok2 {
					if c, _ := fm["fill-color"].(string); c != "" {
						color = c
					}
				}
			}
		}
		vectorSpecs = append(vectorSpecs, map[string]interface{}{
			"layer_id":     vl.LayerID,
			"source_file":  srcFile,
			"source_layer": firstNonEmptyBase(vl.SourceLayer, vl.Name),
			"color":        color,
			"opacity":      vl.Opacity,
			"visible":      vl.Visible,
		})
	}

	specDoc := map[string]interface{}{
		"map_id":        pm.MapID,
		"bounds":        pm.Bounds,
		"layer_order":   normalizePublishedLayerOrder(pm.LayerOrder),
		"layer_stack":   pm.LayerStack,
		"raster_layers": rasterSpecs,
		"vector_layers": vectorSpecs,
	}
	b, err := json.MarshalIndent(specDoc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(specPath, b, 0644); err != nil {
		return "", err
	}
	return specPath, nil
}

// RenderCompositePNGTile renders (or returns from persistent cache) a 256x256 RGBA PNG tile
// combining all published raster and vector layers (polygons, lines, and points) across z0-z18+.
// Out-of-bounds tiles immediately return a valid transparent 256x256 PNG (never 204/404).
func (s *PublishService) RenderCompositePNGTile(ctx context.Context, pm *models.PublishedMap, z, x, y int) ([]byte, error) {
	if pm == nil {
		return nil, models.ErrPublishedMapNotFound
	}
	// Fast bounds check: return transparent 256x256 PNG immediately when outside map bounds
	if len(pm.Bounds) == 4 {
		tw, ts, te, tn := xyzTileToWGS84Bounds(z, x, y)
		bw, bs, be, bn := pm.Bounds[0], pm.Bounds[1], pm.Bounds[2], pm.Bounds[3]
		// Slight padding for point markers near tile edge
		padLon := (te - tw) * 0.1
		padLat := (tn - ts) * 0.1
		if te+padLon < bw || tw-padLon > be || tn+padLat < bs || ts-padLat > bn {
			return getTransparent256PNG(), nil
		}
	}

	cachePath := filepath.Join(filepath.Dir(s.storePath), "composite_cache", pm.MapID, fmt.Sprintf("%d", z), fmt.Sprintf("%d", x), fmt.Sprintf("%d.png", y))
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, nil
	}

	// Singleflight lock per (mapID, z, x, y) so concurrent requests don't render twice
	lockKey := fmt.Sprintf("%s:%d:%d:%d", pm.MapID, z, x, y)
	actual, _ := s.tileLocks.LoadOrStore(lockKey, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, nil
	}

	specPath, err := s.resolveCompositeSpecPath(pm)
	if err != nil {
		return getTransparent256PNG(), nil
	}

	pythonExe := "python"
	for _, c := range []string{
		"backend/.venv/Scripts/python.exe",
		".venv/Scripts/python.exe",
		"../.venv/Scripts/python.exe",
		"../backend/.venv/Scripts/python.exe",
	} {
		if fileExists(c) {
			pythonExe = c
			break
		}
	}
	scriptPath := "backend/scripts/render_composite_tile.py"
	if !fileExists(scriptPath) {
		for _, cs := range []string{
			"scripts/render_composite_tile.py",
			"../scripts/render_composite_tile.py",
			"../backend/scripts/render_composite_tile.py",
		} {
			if fileExists(cs) {
				scriptPath = cs
				break
			}
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(
		ctx,
		pythonExe,
		scriptPath,
		specPath,
		fmt.Sprintf("%d", z),
		fmt.Sprintf("%d", x),
		fmt.Sprintf("%d", y),
		cachePath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("composite tile render failed: %v: %s", err, stderr.String())
	}

	data, err := os.ReadFile(cachePath)
	if err != nil || len(data) == 0 {
		return getTransparent256PNG(), nil
	}
	return data, nil
}

// RenderPublishedRasterTile dynamically renders and caches a single 256x256 PNG tile
// for a published raster layer at any zoom (z0-z22), preserving rotated 4-corner affine
// geometry when present. Returns (nil, nil) when (z, x, y) is outside the raster's bounds.
func (s *PublishService) RenderPublishedRasterTile(ctx context.Context, pm *models.PublishedMap, layerID string, z, x, y int) ([]byte, error) {
	if pm == nil {
		return nil, models.ErrPublishedMapNotFound
	}
	var target *models.PublishedRasterLayer
	for i := range pm.RasterLayers {
		if pm.RasterLayers[i].LayerID == layerID {
			target = &pm.RasterLayers[i]
			break
		}
	}
	if target == nil {
		return nil, models.ErrPublishedMapNotFound
	}
	// Fast bounds check
	if len(target.Bounds) == 4 {
		tw, ts, te, tn := xyzTileToWGS84Bounds(z, x, y)
		bw, bs, be, bn := target.Bounds[0], target.Bounds[1], target.Bounds[2], target.Bounds[3]
		if te < bw || tw > be || tn < bs || ts > bn {
			return nil, nil
		}
	}

	cacheRoot := filepath.Join(filepath.Dir(s.storePath), "composite_cache", pm.MapID, "raster_"+layerID)
	cachePath := filepath.Join(cacheRoot, fmt.Sprintf("%d", z), fmt.Sprintf("%d", x), fmt.Sprintf("%d.png", y))
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, nil
	}

	lockKey := fmt.Sprintf("raster:%s:%s:%d:%d:%d", pm.MapID, layerID, z, x, y)
	actual, _ := s.tileLocks.LoadOrStore(lockKey, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, nil
	}

	imgPath := target.ImagePath
	if imgPath != "" && !fileExists(imgPath) {
		imgPath = ""
	}
	if imgPath == "" && target.DatasetID != "" && s.sessionRepo != nil {
		for _, ds := range s.sessionRepo.ListAllDatasets() {
			if ds != nil && ds.ID == target.DatasetID {
				if s.exportSvc != nil {
					if rp := s.exportSvc.resolveExportRasterPath(ds); rp != "" && fileExists(rp) {
						imgPath = rp
					}
				}
				if imgPath == "" && ds.StoragePath != "" && fileExists(ds.StoragePath) {
					imgPath = ds.StoragePath
				}
				if len(target.Corners) == 0 {
					target.Corners = extractDatasetCorners(ds)
				}
				break
			}
		}
	}
	if imgPath == "" {
		return nil, os.ErrNotExist
	}

	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		return nil, err
	}
	specPath := filepath.Join(cacheRoot, "spec.json")
	specDoc := map[string]interface{}{
		"map_id": pm.MapID,
		"bounds": target.Bounds,
		"raster_layers": []map[string]interface{}{
			{
				"layer_id":   target.LayerID,
				"tile_dir":   "",
				"image_path": imgPath,
				"bounds":     target.Bounds,
				"corners":    target.Corners,
				"opacity":    1.0, // MapLibre applies paint.raster-opacity
				"visible":    true,
			},
		},
		"vector_layers": []interface{}{},
	}
	if b, err := json.MarshalIndent(specDoc, "", "  "); err == nil {
		_ = os.WriteFile(specPath, b, 0644)
	}

	pythonExe := "python"
	for _, c := range []string{
		"backend/.venv/Scripts/python.exe",
		".venv/Scripts/python.exe",
		"../.venv/Scripts/python.exe",
		"../backend/.venv/Scripts/python.exe",
	} {
		if fileExists(c) {
			pythonExe = c
			break
		}
	}
	scriptPath := "backend/scripts/render_composite_tile.py"
	if !fileExists(scriptPath) {
		for _, cs := range []string{
			"scripts/render_composite_tile.py",
			"../scripts/render_composite_tile.py",
			"../backend/scripts/render_composite_tile.py",
		} {
			if fileExists(cs) {
				scriptPath = cs
				break
			}
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(
		ctx,
		pythonExe,
		scriptPath,
		specPath,
		fmt.Sprintf("%d", z),
		fmt.Sprintf("%d", x),
		fmt.Sprintf("%d", y),
		cachePath,
	)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cachePath)
	if err != nil || len(data) == 0 {
		return nil, os.ErrNotExist
	}
	return data, nil
}
