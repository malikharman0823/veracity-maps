package services

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/storage"
)

// ViewscapeExportMode selects which independent package to generate.
type ViewscapeExportMode string

const (
	ViewscapeModeRaster   ViewscapeExportMode = "viewscape_raster"
	ViewscapeModeVector   ViewscapeExportMode = "viewscape_vector"
	ViewscapeModeBase     ViewscapeExportMode = "viewscape_base"
	ViewscapeModeComplete ViewscapeExportMode = "viewscape_complete"
)

// ViewscapeManifest defines Viewscape/manifest.json for both independent and complete packages.
type ViewscapeManifest struct {
	SchemaVersion   string                   `json:"schema_version"`
	Format          string                   `json:"format"`
	PackageType     string                   `json:"package_type"`
	Application     string                   `json:"application"`
	ProjectName     string                   `json:"project_name"`
	WorkspaceID     string                   `json:"workspace_id,omitempty"`
	PrimaryID       string                   `json:"primary_dataset_id,omitempty"`
	ExportedAt      string                   `json:"exported_at"`
	CRS             string                   `json:"crs"`
	BoundsCRS       string                   `json:"bounds_crs"`
	Bounds          []float64                `json:"bounds"`
	Center          []float64                `json:"center"`
	InitialZoom     int                      `json:"initial_zoom"`
	MinZoom         int                      `json:"min_zoom"`
	MaxZoom         int                      `json:"max_zoom"`
	LayerOrdering   []string                 `json:"layer_ordering"`
	RasterLayers    []ViewscapeRasterEntry   `json:"raster_layers"`
	VectorLayers    []ViewscapeVectorEntry   `json:"vector_layers"`
	BaseLayers      []ViewscapeBaseEntry     `json:"base_layers"`
	Paths           map[string]string        `json:"paths"`
	Buildings       []map[string]interface{} `json:"buildings,omitempty"`
	InteractionMode map[string]interface{}   `json:"interaction_config,omitempty"`
}

// ViewscapeRasterEntry describes one exported raster layer inside Viewscape/raster_layers/<slug>/.
type ViewscapeRasterEntry struct {
	ID                 string                 `json:"id"`
	Slug               string                 `json:"slug"`
	Name               string                 `json:"name"`
	Format             string                 `json:"format"`
	TileScheme         string                 `json:"tile_scheme"`
	TilesPath          string                 `json:"tiles_path"`
	MetadataPath       string                 `json:"metadata_path"`
	SourceCRS          string                 `json:"source_crs"`
	TileCRS            string                 `json:"tile_crs"`
	Bounds             []float64              `json:"bounds"`
	MinZoom            int                    `json:"min_zoom"`
	MaxZoom            int                    `json:"max_zoom"`
	TileCount          int                    `json:"tile_count"`
	Opacity            float64                `json:"opacity"`
	Visible            bool                   `json:"visible"`
	DisplayOrder       int                    `json:"display_order"`
	GeoreferenceStatus string                 `json:"georeference_status"`
	Transform          []float64              `json:"transform,omitempty"`
	BuildingName       string                 `json:"building_name,omitempty"`
	FloorLevel         int                    `json:"floor_level,omitempty"`
	FloorLabel         string                 `json:"floor_label,omitempty"`
	ControlPointsCount int                    `json:"control_points_count,omitempty"`
	Extra              map[string]interface{} `json:"extra,omitempty"`
}

// ViewscapeVectorEntry describes one exported vector tile layer inside Viewscape/vector_layers/<slug>/.
type ViewscapeVectorEntry struct {
	ID            string                   `json:"id"`
	GISLayerID    string                   `json:"gis_layer_id,omitempty"`
	Slug          string                   `json:"slug"`
	Name          string                   `json:"name"`
	SourceLayer   string                   `json:"source_layer"`
	Format        string                   `json:"format"`
	Encoding      string                   `json:"encoding"`
	TileScheme    string                   `json:"tile_scheme"`
	TilesPath     string                   `json:"tiles_path"`
	MetadataPath  string                   `json:"metadata_path"`
	StylePath     string                   `json:"style_path"`
	SourceCRS     string                   `json:"source_crs"`
	TileCRS       string                   `json:"tile_crs"`
	GeometryType  string                   `json:"geometry_type"`
	GeometryTypes []string                 `json:"geometry_types,omitempty"`
	FeatureCount  int                      `json:"feature_count"`
	TileCount     int                      `json:"tile_count"`
	Bounds        []float64                `json:"bounds,omitempty"`
	MinZoom       int                      `json:"min_zoom"`
	MaxZoom       int                      `json:"max_zoom"`
	Opacity       float64                  `json:"opacity"`
	Visible       bool                     `json:"visible"`
	DisplayOrder  int                      `json:"display_order"`
	PromoteID     string                   `json:"promote_id"`
	Interactive   bool                     `json:"interactive"`
	Fields        []map[string]interface{} `json:"fields,omitempty"`
	Style         map[string]interface{}   `json:"style,omitempty"`
}

// ViewscapeBaseEntry describes one base layer source inside Viewscape/base_layers/sources.json.
type ViewscapeBaseEntry struct {
	ID                       string                 `json:"id"`
	Name                     string                 `json:"name"`
	SourceType               string                 `json:"source_type"` // "osm", "wms", "xyz", "bing", "mbtiles", "local_tiles", "custom"
	Provider                 string                 `json:"provider,omitempty"`
	URL                      string                 `json:"url,omitempty"`
	WMSLayers                string                 `json:"wms_layers,omitempty"`
	Attribution              string                 `json:"attribution,omitempty"`
	CRS                      string                 `json:"crs"`
	Bounds                   []float64              `json:"bounds,omitempty"`
	MinZoom                  int                    `json:"min_zoom"`
	MaxZoom                  int                    `json:"max_zoom"`
	Opacity                  float64                `json:"opacity"`
	Visible                  bool                   `json:"visible"`
	Offline                  bool                   `json:"offline"`
	RemoteTileCachePermitted bool                   `json:"remote_tile_cache_permitted"`
	LocalTilesPath           string                 `json:"local_tiles_path,omitempty"`
	MBTilesPath              string                 `json:"mbtiles_path,omitempty"`
	Config                   map[string]interface{} `json:"config,omitempty"`
}

// GenerateViewscapeExport produces an independent ZIP for Raster, Vector, Base, or Complete Viewscape.
// Reuses existing cached raster tile rendering, GeoPackage MVT conversion, progress tracking, and validation.
func (s *ExportService) GenerateViewscapeExport(
	sessionID, primaryDatasetID string,
	params SelectiveExportParams,
	mode ViewscapeExportMode,
) (string, string, error) {
	primaryDs, err := s.repo.GetDataset(sessionID, primaryDatasetID)
	if err != nil || primaryDs == nil {
		return "", "", fmt.Errorf("DATASET_NOT_FOUND: Primary dataset not found: %w", err)
	}

	exportDir := filepath.Join(s.storage.GetDatasetDir(sessionID, primaryDatasetID), "exports")
	_ = os.MkdirAll(exportDir, 0755)

	opts := params.Options
	if opts == nil {
		opts = DefaultSelectiveExportOptions()
	}

	workspaceID := strings.TrimSpace(params.WorkspaceID)
	if workspaceID == "" {
		workspaceID = primaryDatasetID
	}

	allDatasets, _ := s.repo.ListDatasets(sessionID)
	datasetMap := make(map[string]*models.Dataset)
	for _, ds := range allDatasets {
		if ds != nil {
			datasetMap[ds.ID] = ds
		}
	}
	if s.repo != nil {
		for _, ds := range s.repo.ListAllDatasets() {
			if ds != nil {
				if _, ok := datasetMap[ds.ID]; !ok {
					datasetMap[ds.ID] = ds
				}
			}
		}
	}
	datasetMap[primaryDatasetID] = primaryDs

	primaryProj, _ := s.repo.GetMapProject(sessionID, primaryDatasetID)
	allBuildings, _ := s.repo.ListBuildings(sessionID)

	projectName := strings.TrimSpace(params.ProjectName)
	if projectName == "" {
		if primaryProj != nil && primaryProj.Name != "" {
			projectName = primaryProj.Name
		} else {
			projectName = strings.TrimSuffix(primaryDs.Filename, filepath.Ext(primaryDs.Filename))
		}
	}
	stem := safeStem(projectName)

	var filename string
	switch mode {
	case ViewscapeModeRaster:
		filename = stem + "_raster_layers.zip"
	case ViewscapeModeVector:
		filename = stem + "_vector_layers.zip"
	case ViewscapeModeBase:
		filename = stem + "_base_layers.zip"
	default:
		mode = ViewscapeModeComplete
		filename = stem + "_viewscape.zip"
	}
	zipPath := filepath.Join(exportDir, filename)

	stageDir := filepath.Join(exportDir, ".stage_"+string(mode)+"_"+stem)
	_ = storage.SafeRemoveAll(stageDir)
	_ = os.MkdirAll(stageDir, 0755)
	defer func() {
		_ = storage.SafeRemoveAll(stageDir)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.ProgressManager().StartExport(primaryDatasetID, sessionID, cancel)
	s.ProgressManager().UpdateStage(primaryDatasetID, 1, "Inspecting workspace layers", projectName)

	zipFile, err := os.Create(zipPath)
	if err != nil {
		s.ProgressManager().FailExport(primaryDatasetID, err)
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating Viewscape archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	seenZipEntries := make(map[string]bool)
	addBytesToZip := func(archivePath string, data []byte) error {
		clean := filepath.ToSlash(archivePath)
		if seenZipEntries[clean] {
			return nil
		}
		header := &zip.FileHeader{
			Name:     clean,
			Method:   zip.Deflate,
			Modified: time.Now(),
		}
		w, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		seenZipEntries[clean] = true
		return nil
	}

	addJSONToZip := func(archivePath string, v interface{}) error {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		return addBytesToZip(archivePath, b)
	}

	addFileToZip := func(archivePath, diskPath string) error {
		clean := filepath.ToSlash(archivePath)
		if seenZipEntries[clean] {
			return nil
		}
		f, err := os.Open(diskPath)
		if err != nil {
			return err
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		header := &zip.FileHeader{
			Name:     clean,
			Method:   zip.Deflate,
			Modified: fi.ModTime(),
		}
		w, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, f); err != nil {
			return err
		}
		seenZipEntries[clean] = true
		return nil
	}

	// addPBFToZip writes an MVT/PBF tile into the ZIP as raw uncompressed protobuf bytes
	// (gunzipping if GDAL wrote gzip-compressed .pbf) so static file servers and Viewscape
	// loaders do not require a Content-Encoding: gzip HTTP header to decode the tile.
	addPBFToZip := func(archivePath, diskPath string) error {
		clean := strings.TrimPrefix(filepath.ToSlash(archivePath), "/")
		if seenZipEntries[clean] {
			return nil
		}
		raw, err := os.ReadFile(diskPath)
		if err != nil {
			return err
		}
		if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
			if gr, gErr := gzip.NewReader(bytes.NewReader(raw)); gErr == nil {
				decoded, dErr := io.ReadAll(gr)
				_ = gr.Close()
				if dErr == nil && len(decoded) > 0 {
					raw = decoded
				}
			}
		}
		return addBytesToZip(clean, raw)
	}

	actualTileMinZ := -1
	actualTileMaxZ := -1
	recordTileZoom := func(z int) {
		if z < 0 || z > 24 {
			return
		}
		if actualTileMinZ < 0 || z < actualTileMinZ {
			actualTileMinZ = z
		}
		if z > actualTileMaxZ {
			actualTileMaxZ = z
		}
	}

	// Resolve member visibility/opacity/order from MapProject
	memberMetaByID := make(map[string]map[string]interface{})
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

	// Resolve selected IDs (empty => all in workspace/session)
	selectedSet := make(map[string]bool)
	for _, id := range params.LayerIDs {
		if strings.TrimSpace(id) != "" {
			selectedSet[strings.TrimSpace(id)] = true
		}
	}

	// Collect GIS vector layers from GISService if wired
	var gisLayers []*models.MapLayer
	if s.gisService != nil && s.gisService.GetGISRepo() != nil {
		if lyrs, lErr := s.gisService.GetGISRepo().GetLayers(ctx, workspaceID); lErr == nil && len(lyrs) > 0 {
			gisLayers = lyrs
		} else if workspaceID != primaryDatasetID {
			if lyrs2, lErr2 := s.gisService.GetGISRepo().GetLayers(ctx, primaryDatasetID); lErr2 == nil && len(lyrs2) > 0 {
				gisLayers = lyrs2
			}
		}
	}
	sort.Slice(gisLayers, func(i, j int) bool {
		return gisLayers[i].DisplayOrder < gisLayers[j].DisplayOrder
	})

	// Classify datasets into Raster vs Vector
	isVectorDataset := func(ds *models.Dataset) bool {
		if ds == nil {
			return false
		}
		ext := strings.ToLower(filepath.Ext(ds.Filename))
		if ext == "" && ds.StoragePath != "" {
			ext = strings.ToLower(filepath.Ext(ds.StoragePath))
		}
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

	var rasterDatasets []*models.Dataset
	var vectorDatasets []*models.Dataset
	for _, ds := range allDatasets {
		if ds == nil {
			continue
		}
		if len(selectedSet) > 0 && !selectedSet[ds.ID] {
			// Check if any selected GIS layer references this dataset
			referenced := false
			for _, gl := range gisLayers {
				if gl.DatasetID == ds.ID && selectedSet[gl.LayerID] {
					referenced = true
					break
				}
			}
			if !referenced {
				continue
			}
		}
		if isVectorDataset(ds) {
			vectorDatasets = append(vectorDatasets, ds)
		} else {
			rasterDatasets = append(rasterDatasets, ds)
		}
	}
	if len(rasterDatasets) == 0 && len(vectorDatasets) == 0 {
		if isVectorDataset(primaryDs) {
			vectorDatasets = append(vectorDatasets, primaryDs)
		} else {
			rasterDatasets = append(rasterDatasets, primaryDs)
		}
	}

	var unionBounds []float64
	expandUnionBounds := func(b []float64) {
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

	minZoom := 0
	maxZoom := 18
	initZoom := 14
	if opts.ZoomConfig != nil {
		if opts.ZoomConfig.MinZoom != nil {
			minZoom = *opts.ZoomConfig.MinZoom
		}
		if opts.ZoomConfig.MaxZoom != nil {
			maxZoom = *opts.ZoomConfig.MaxZoom
		}
		if opts.ZoomConfig.InitialZoom != nil {
			initZoom = *opts.ZoomConfig.InitialZoom
		}
	}

	includeRasters := mode == ViewscapeModeRaster || mode == ViewscapeModeComplete
	includeVectors := mode == ViewscapeModeVector || mode == ViewscapeModeComplete
	includeBase := mode == ViewscapeModeBase || mode == ViewscapeModeComplete

	rasterEntries := make([]ViewscapeRasterEntry, 0)
	vectorEntries := make([]ViewscapeVectorEntry, 0)
	baseEntries := make([]ViewscapeBaseEntry, 0)
	layerStyles := map[string]interface{}{
		"version": 8,
		"raster":  []map[string]interface{}{},
		"vector":  []map[string]interface{}{},
		"base":    []map[string]interface{}{},
	}

	// =========================================================================
	// A. RASTER LAYERS -> Viewscape/raster_layers/<slug>/metadata.json + tiles/z/x/y.png
	// =========================================================================
	if includeRasters {
		s.ProgressManager().UpdateStage(primaryDatasetID, 3, "Exporting raster XYZ tiles", "")
		rasterQuality := 85
		if opts.ImageQuality > 0 && opts.ImageQuality <= 100 {
			rasterQuality = opts.ImageQuality
		}
		usedSlugs := make(map[string]int)

		for rIdx, ds := range rasterDatasets {
			if ctx.Err() != nil {
				s.ProgressManager().FailExport(primaryDatasetID, ctx.Err())
				return "", "", fmt.Errorf("EXPORT_CANCELLED: %w", ctx.Err())
			}
			dsBounds := extractDatasetBounds(ds)
			if len(dsBounds) == 4 {
				expandUnionBounds(dsBounds)
			} else {
				dsBounds = []float64{-180, -85, 180, 85}
			}

			baseSlug := safeStem(ds.Filename)
			if baseSlug == "" {
				baseSlug = ds.ID
			}
			usedSlugs[baseSlug]++
			layerSlug := baseSlug
			if usedSlugs[baseSlug] > 1 {
				layerSlug = fmt.Sprintf("%s_%d", baseSlug, usedSlugs[baseSlug])
			}

			renderPath := s.resolveExportRasterPath(ds)
			if renderPath == "" || !fileExists(renderPath) {
				renderPath = ds.StoragePath
			}

			tileFmt := "png"
			if opts.RasterFormat != "" {
				tileFmt = strings.ToLower(opts.RasterFormat)
			} else if strings.HasSuffix(strings.ToLower(renderPath), ".jpg") || strings.HasSuffix(strings.ToLower(renderPath), ".jpeg") {
				tileFmt = "jpg"
			}

			layerTileDir := filepath.Join(stageDir, "raster_layers", layerSlug, "tiles")
			_ = os.MkdirAll(layerTileDir, 0755)

			rMinZ := minZoom
			rMaxZ := maxZoom
			if rMinZ < 10 && len(dsBounds) == 4 && (dsBounds[2]-dsBounds[0]) < 1.0 {
				// Avoid generating empty world-level zoom 0..9 tiles for local floorplans
				rMinZ, _ = gis.DetermineTileZooms(dsBounds, 512, 512)
				if rMinZ > rMaxZ {
					rMinZ = rMaxZ
				}
			}

			cTiles, _, reTiles := s.getOrCreateCachedRasterTiles(ctx, renderPath, dsBounds, rMinZ, rMaxZ, tileFmt, rasterQuality, layerTileDir)
			tileCount := cTiles + reTiles

			// Fallback: if dataset already had loose tiles in tiles_xyz, copy those
			if tileCount == 0 {
				dsTilesXYZ := filepath.Join(resolveDatasetDirectory(s.storage, ds), "tiles_xyz")
				if dirExists(dsTilesXYZ) {
					_ = filepath.Walk(dsTilesXYZ, func(p string, info os.FileInfo, wErr error) error {
						if wErr != nil || info.IsDir() {
							return nil
						}
						ext := strings.ToLower(filepath.Ext(p))
						if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
							rel, rErr := filepath.Rel(dsTilesXYZ, p)
							if rErr == nil {
								dst := filepath.Join(layerTileDir, rel)
								_ = os.MkdirAll(filepath.Dir(dst), 0755)
								if storage.CopyFile(p, dst) == nil {
									tileCount++
								}
							}
						}
						return nil
					})
				}
			}

			_ = filepath.Walk(layerTileDir, func(p string, info os.FileInfo, wErr error) error {
				if wErr != nil || info.IsDir() {
					return nil
				}
				rel, rErr := filepath.Rel(layerTileDir, p)
				if rErr != nil {
					return nil
				}
				relSlash := filepath.ToSlash(rel)
				arcPath := fmt.Sprintf("Viewscape/raster_layers/%s/tiles/%s", layerSlug, relSlash)
				_ = addFileToZip(arcPath, p)

				ext := filepath.Ext(relSlash)
				if ext != "" {
					extless := strings.TrimSuffix(relSlash, ext)
					parts := strings.Split(extless, "/")
					if len(parts) == 3 {
						if zNum, errZ := strconv.Atoi(parts[0]); errZ == nil {
							recordTileZoom(zNum)
						}
					}
				}
				return nil
			})

			opacity := 1.0
			visible := true
			sortOrder := rIdx
			if m, ok := memberMetaByID[ds.ID]; ok {
				if op := getDim(m["raster_opacity"]); op > 0 && op <= 1.0 {
					opacity = op
				}
				if vis, hasVis := m["visible"].(bool); hasVis {
					visible = vis
				}
				if _, hasSO := m["sort_order"]; hasSO {
					sortOrder = int(getDim(m["sort_order"]))
				}
			}

			transform, _ := extractAffineTransform(ds.ExtractedMetadata)
			cps, _ := s.repo.ListControlPoints(sessionID, ds.ID)
			srcCRS := "EPSG:4326"
			if ds.ExtractedMetadata != nil {
				if c, ok := ds.ExtractedMetadata["crs"].(string); ok && c != "" {
					srcCRS = c
				}
			}
			bldName, _ := ds.ExtractedMetadata["building_name"].(string)
			flLvl := int(getDim(ds.ExtractedMetadata["floor_level"]))
			flLbl, _ := ds.ExtractedMetadata["floor_label"].(string)

			entry := ViewscapeRasterEntry{
				ID:                 ds.ID,
				Slug:               layerSlug,
				Name:               ds.Filename,
				Format:             tileFmt,
				TileScheme:         "xyz",
				TilesPath:          fmt.Sprintf("raster_layers/%s/tiles/{z}/{x}/{y}.%s", layerSlug, tileFmt),
				MetadataPath:       fmt.Sprintf("raster_layers/%s/metadata.json", layerSlug),
				SourceCRS:          srcCRS,
				TileCRS:            "EPSG:3857",
				Bounds:             dsBounds,
				MinZoom:            rMinZ,
				MaxZoom:            rMaxZ,
				TileCount:          tileCount,
				Opacity:            opacity,
				Visible:            visible,
				DisplayOrder:       sortOrder,
				GeoreferenceStatus: ds.GeoreferenceStatus,
				Transform:          transform,
				BuildingName:       bldName,
				FloorLevel:         flLvl,
				FloorLabel:         flLbl,
				ControlPointsCount: len(cps),
			}
			rasterEntries = append(rasterEntries, entry)

			rasterMetaDoc := map[string]interface{}{
				"layer":          entry,
				"georeferencing": buildGeoreferencingMetadataDoc([]*models.Dataset{ds}, cps),
				"control_points": cps,
			}
			_ = addJSONToZip(fmt.Sprintf("Viewscape/raster_layers/%s/metadata.json", layerSlug), rasterMetaDoc)

			// Emit raster layer into Custom_Local_Tiles/ (directly under Custom_Local_Tiles/ if single raster,
			// or Custom_Local_Tiles/<layerSlug>/ if multiple rasters) matching E:\New folder\New folder.
			cltPrefix := "Custom_Local_Tiles"
			if len(rasterDatasets) > 1 {
				cltPrefix = "Custom_Local_Tiles/" + layerSlug
			}
			writeCustomLocalTileFolder(
				cltPrefix, ds.Filename, layerTileDir, dsBounds, rMinZ, rMaxZ,
				addFileToZip, addBytesToZip, addJSONToZip,
			)

			layerStyles["raster"] = append(layerStyles["raster"].([]map[string]interface{}), map[string]interface{}{
				"id":            layerSlug,
				"name":          ds.Filename,
				"type":          "raster",
				"opacity":       opacity,
				"visible":       visible,
				"display_order": sortOrder,
			})
		}
	}

	// =========================================================================
	// B. VECTOR LAYERS -> Viewscape/vector_layers/<slug>/metadata.json + style.json + tiles/z/x/y.pbf
	// =========================================================================
	if includeVectors {
		s.ProgressManager().UpdateStage(primaryDatasetID, 4, "Exporting vector MBTiles & GeoJSON", "")
		usedVecSlugs := make(map[string]int)
		exportedLayerKeys := make(map[string]bool)
		exportedMBTilesSources := make(map[string]bool)
		combinedAllFeatures := make([]map[string]interface{}, 0)
		combinedPointFeatures := make([]map[string]interface{}, 0)
		tmpVecGeoJSONStage := filepath.Join(os.TempDir(), fmt.Sprintf("veracity_vec_gj_%d", time.Now().UnixNano()))
		_ = os.MkdirAll(tmpVecGeoJSONStage, 0755)
		defer storage.SafeRemoveAll(tmpVecGeoJSONStage)

		exportVectorLayerDir := func(
			layerID, gisLayerID, displayName, rawSlug, layerDir string,
			sourceCRS, geomType string, geomTypes []string,
			featureCount int, bounds []float64, minZ, maxZ int,
			opacity float64, visible bool, order int,
			fields []map[string]interface{}, styleMap map[string]interface{},
		) error {
			slug := safeStem(rawSlug)
			if slug == "" {
				slug = safeStem(displayName)
			}
			usedVecSlugs[slug]++
			finalSlug := slug
			if usedVecSlugs[slug] > 1 {
				finalSlug = fmt.Sprintf("%s_%d", slug, usedVecSlugs[slug])
			}

			tilesDiskDir := filepath.Join(layerDir, "tiles")
			if !dirExists(tilesDiskDir) {
				tilesDiskDir = layerDir
			}
			tileCount := 0
			_ = filepath.Walk(tilesDiskDir, func(p string, info os.FileInfo, wErr error) error {
				if wErr != nil || info.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(p))
				if ext != ".pbf" && ext != ".mvt" {
					return nil
				}
				rel, rErr := filepath.Rel(tilesDiskDir, p)
				if rErr != nil {
					return nil
				}
				relSlash := filepath.ToSlash(rel)
				arcPath := fmt.Sprintf("Viewscape/vector_layers/%s/tiles/%s", finalSlug, relSlash)
				if addPBFToZip(arcPath, p) == nil {
					tileCount++
				}
				extless := strings.TrimSuffix(relSlash, filepath.Ext(relSlash))
				if extless != "" {
					parts := strings.Split(extless, "/")
					if len(parts) == 3 {
						zNum, errZ := strconv.Atoi(parts[0])
						if errZ == nil {
							recordTileZoom(zNum)
						}
					}
				}
				return nil
			})

			// Load or synthesize style.json with point interaction config
			var styleDoc map[string]interface{}
			diskStyle := filepath.Join(layerDir, "style.json")
			if fileExists(diskStyle) {
				if rawS, sErr := os.ReadFile(diskStyle); sErr == nil {
					_ = json.Unmarshal(rawS, &styleDoc)
				}
			}
			if styleDoc == nil {
				styleDoc = map[string]interface{}{
					"version":       8,
					"name":          displayName,
					"source_layer":  slug,
					"geometry_type": geomType,
					"minzoom":       minZ,
					"maxzoom":       maxZ,
				}
			}
			if styleMap != nil {
				styleDoc["workspace_style"] = styleMap
			}
			if styleDoc["interaction"] == nil {
				popupFields := make([]string, 0, len(fields))
				for _, f := range fields {
					if fn, _ := f["name"].(string); fn != "" && !strings.HasPrefix(fn, "_") {
						popupFields = append(popupFields, fn)
					}
				}
				styleDoc["interaction"] = map[string]interface{}{
					"interactive":  true,
					"clickable":    true,
					"hoverable":    true,
					"promote_id":   "feature_id",
					"source_layer": slug,
					"popup_fields": popupFields,
				}
			}
			_ = addJSONToZip(fmt.Sprintf("Viewscape/vector_layers/%s/style.json", finalSlug), styleDoc)

			// Preserve metadata.json (enriched with workspace ordering, visibility, CRS, and interaction)
			var metaDoc map[string]interface{}
			diskMeta := filepath.Join(layerDir, "metadata.json")
			if fileExists(diskMeta) {
				if rawM, mErr := os.ReadFile(diskMeta); mErr == nil {
					_ = json.Unmarshal(rawM, &metaDoc)
				}
			}
			if metaDoc == nil {
				metaDoc = make(map[string]interface{})
			}
			if sourceCRS == "" {
				sourceCRS, _ = metaDoc["source_crs"].(string)
			}
			if sourceCRS == "" {
				sourceCRS = "EPSG:4326"
			}
			if geomType == "" {
				geomType, _ = metaDoc["geometry_type"].(string)
			}
			if featureCount == 0 {
				featureCount = int(getDim(metaDoc["feature_count"]))
			}
			if len(bounds) != 4 {
				if bRaw, ok := metaDoc["bounds"].([]interface{}); ok && len(bRaw) == 4 {
					bounds = []float64{getDim(bRaw[0]), getDim(bRaw[1]), getDim(bRaw[2]), getDim(bRaw[3])}
				}
			}
			if len(bounds) == 4 {
				expandUnionBounds(bounds)
			}

			sourceLayerName := slug
			if sl, _ := metaDoc["source_layer"].(string); sl != "" {
				sourceLayerName = sl
			}

			metaDoc["layer_slug"] = finalSlug
			metaDoc["layer_name"] = displayName
			metaDoc["source_layer"] = sourceLayerName
			metaDoc["source_crs"] = sourceCRS
			metaDoc["tile_crs"] = "EPSG:3857"
			metaDoc["geometry_type"] = geomType
			metaDoc["feature_count"] = featureCount
			metaDoc["tile_count"] = tileCount
			metaDoc["bounds"] = bounds
			metaDoc["min_zoom"] = minZ
			metaDoc["max_zoom"] = maxZ
			metaDoc["opacity"] = opacity
			metaDoc["visible"] = visible
			metaDoc["display_order"] = order
			metaDoc["promote_id"] = "feature_id"
			metaDoc["interactive"] = true
			_ = addJSONToZip(fmt.Sprintf("Viewscape/vector_layers/%s/metadata.json", finalSlug), metaDoc)

			entry := ViewscapeVectorEntry{
				ID:            layerID,
				GISLayerID:    gisLayerID,
				Slug:          finalSlug,
				Name:          displayName,
				SourceLayer:   sourceLayerName,
				Format:        "pbf",
				Encoding:      "gzip",
				TileScheme:    "xyz",
				TilesPath:     fmt.Sprintf("vector_layers/%s/tiles/{z}/{x}/{y}.pbf", finalSlug),
				MetadataPath:  fmt.Sprintf("vector_layers/%s/metadata.json", finalSlug),
				StylePath:     fmt.Sprintf("vector_layers/%s/style.json", finalSlug),
				SourceCRS:     sourceCRS,
				TileCRS:       "EPSG:3857",
				GeometryType:  geomType,
				GeometryTypes: geomTypes,
				FeatureCount:  featureCount,
				TileCount:     tileCount,
				Bounds:        bounds,
				MinZoom:       minZ,
				MaxZoom:       maxZ,
				Opacity:       opacity,
				Visible:       visible,
				DisplayOrder:  order,
				PromoteID:     "feature_id",
				Interactive:   true,
				Fields:        fields,
				Style:         styleDoc,
			}
			vectorEntries = append(vectorEntries, entry)
			layerStyles["vector"] = append(layerStyles["vector"].([]map[string]interface{}), map[string]interface{}{
				"id":            finalSlug,
				"gis_layer_id":  gisLayerID,
				"name":          displayName,
				"source_layer":  sourceLayerName,
				"geometry_type": geomType,
				"opacity":       opacity,
				"visible":       visible,
				"display_order": order,
				"style":         styleDoc,
			})

			// Convert each GeoPackage / vector layer to clean EPSG:4326 GeoJSON via gpkg_to_geojson.py
			// while keeping Raster layers (Custom Local Tiles) and Base layers exactly as they were.
			srcVectorFile := ""
			srcLayerArg := ""
			if dsObj := datasetMap[layerID]; dsObj != nil && fileExists(dsObj.StoragePath) {
				srcVectorFile = dsObj.StoragePath
				if strings.EqualFold(filepath.Ext(srcVectorFile), ".gpkg") {
					srcLayerArg = firstNonEmptyBase(sourceLayerName, displayName)
				}
			}
			if srcVectorFile == "" {
				// Scan parent dataset directory (above vector_tiles/<slug>) for original .gpkg/.geojson/.shp/.kml
				dsParentDir := filepath.Dir(filepath.Dir(layerDir))
				if dirEntries, dErr := os.ReadDir(dsParentDir); dErr == nil {
					for _, de := range dirEntries {
						if de.IsDir() {
							continue
						}
						ext := strings.ToLower(filepath.Ext(de.Name()))
						if ext == ".gpkg" || ext == ".geojson" || ext == ".shp" || ext == ".kml" || ext == ".fgb" {
							srcVectorFile = filepath.Join(dsParentDir, de.Name())
							if ext == ".gpkg" {
								srcLayerArg = firstNonEmptyBase(sourceLayerName, displayName)
							}
							break
						}
					}
				}
			}
			if srcVectorFile == "" {
				siblingGJ := filepath.Join(filepath.Dir(filepath.Dir(layerDir)), "geojson", slug+".geojson")
				if fileExists(siblingGJ) {
					srcVectorFile = siblingGJ
				}
			}
			if srcVectorFile != "" {
				// Generate one multi-layer Vector .mbtiles per unique source file (e.g., delhi.gpkg -> Vector_Layers/delhi.mbtiles)
				normSrc := strings.ToLower(filepath.Clean(srcVectorFile))
				if !exportedMBTilesSources[normSrc] {
					exportedMBTilesSources[normSrc] = true
					mbBase := safeStem(strings.TrimSuffix(filepath.Base(srcVectorFile), filepath.Ext(srcVectorFile)))
					if mbBase == "" {
						mbBase = finalSlug
					}
					mbOutPath := filepath.Join(tmpVecGeoJSONStage, mbBase+".mbtiles")
					mbMaxZ := maxZ
					if mbMaxZ <= 0 || mbMaxZ > 14 {
						mbMaxZ = 12
					}
					if mbRes, mbErr := s.convertGPKGToVectorMBTilesWithPython(ctx, srcVectorFile, mbOutPath, 0, mbMaxZ, mbBase); mbErr == nil && mbRes != nil && mbRes.Success && fileExists(mbOutPath) {
						_ = addFileToZip(fmt.Sprintf("Vector_Layers/%s.mbtiles", mbBase), mbOutPath)
					}
				}

				layerOutRoot := filepath.Join(tmpVecGeoJSONStage, finalSlug)
				if gRes, gErr := s.convertGPKGToGeoJSONWithPython(ctx, srcVectorFile, layerOutRoot, srcLayerArg); gErr == nil && gRes != nil && gRes.Success {
					for _, glInfo := range gRes.Layers {
						if fileExists(glInfo.GeoJSONPath) {
							_ = addFileToZip(fmt.Sprintf("Vector_Layers/%s.geojson", finalSlug), glInfo.GeoJSONPath)
							if rawLayer, lErr := os.ReadFile(glInfo.GeoJSONPath); lErr == nil {
								var fc struct {
									Features []map[string]interface{} `json:"features"`
								}
								if json.Unmarshal(rawLayer, &fc) == nil && len(fc.Features) > 0 {
									combinedAllFeatures = append(combinedAllFeatures, fc.Features...)
								}
							}
						}
					}
					if gRes.PointsGeoJSONPath != "" && fileExists(gRes.PointsGeoJSONPath) {
						if rawPts, pErr := os.ReadFile(gRes.PointsGeoJSONPath); pErr == nil {
							var fc struct {
								Features []map[string]interface{} `json:"features"`
							}
							if json.Unmarshal(rawPts, &fc) == nil && len(fc.Features) > 0 {
								combinedPointFeatures = append(combinedPointFeatures, fc.Features...)
							}
						}
					}
				}
			}

			return nil
		}

		// 1. Export GIS workspace vector layers that already have vector_tiles metadata
		for idx, gl := range gisLayers {
			if gl == nil || (gl.LayerType != "" && gl.LayerType != "vector") {
				continue
			}
			if len(selectedSet) > 0 && !selectedSet[gl.LayerID] && !selectedSet[gl.DatasetID] {
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
				if b, mErr := json.Marshal(gl.Style); mErr == nil {
					_ = json.Unmarshal(b, &styleMap)
				}
			}
			minZ := int(getDim(vtMap["min_zoom"]))
			maxZ := int(getDim(vtMap["max_zoom"]))
			if maxZ == 0 {
				maxZ = 14
			}
			var fields []map[string]interface{}
			if rawF, ok := vtMap["fields"]; ok {
				if b, mErr := json.Marshal(rawF); mErr == nil {
					_ = json.Unmarshal(b, &fields)
				}
			}
			var gTypes []string
			if rawGT, ok := vtMap["geometry_types"]; ok {
				if b, mErr := json.Marshal(rawGT); mErr == nil {
					_ = json.Unmarshal(b, &gTypes)
				}
			}
			srcCRS, _ := vtMap["source_crs"].(string)
			if srcCRS == "" {
				srcCRS = gl.CRS
			}
			_ = exportVectorLayerDir(
				gl.DatasetID, gl.LayerID, gl.LayerName, slug, layerDir,
				srcCRS, gl.GeometryType, gTypes,
				gl.FeatureCount, gl.Bounds, minZ, maxZ,
				gl.Opacity, gl.IsVisible, idx, fields, styleMap,
			)
			exportedLayerKeys[gl.DatasetID+"::"+slug] = true
			exportedLayerKeys[gl.DatasetID+"::"+gl.LayerName] = true
		}

		// 2. Export any remaining vector datasets (including multi-layer GeoPackages) not already covered via gisLayers
		for dIdx, ds := range vectorDatasets {
			if ds == nil || ds.StoragePath == "" || !fileExists(ds.StoragePath) {
				continue
			}
			vtRoot := filepath.Join(filepath.Dir(ds.StoragePath), "vector_tiles")
			// Ensure converted vector_tiles exist on disk
			hasExisting := false
			if entries, err := os.ReadDir(vtRoot); err == nil {
				for _, ent := range entries {
					if ent.IsDir() && fileExists(filepath.Join(vtRoot, ent.Name(), "metadata.json")) {
						hasExisting = true
						break
					}
				}
			}
			if !hasExisting {
				_, _ = gis.ConvertDatasetToVectorTiles(ds.StoragePath, vtRoot, nil, 0, min(maxZoom, 14))
			}
			if entries, err := os.ReadDir(vtRoot); err == nil {
				for lIdx, ent := range entries {
					if !ent.IsDir() {
						continue
					}
					slug := ent.Name()
					layerDir := filepath.Join(vtRoot, slug)
					if exportedLayerKeys[ds.ID+"::"+slug] {
						continue
					}
					var metaDoc map[string]interface{}
					if raw, rErr := os.ReadFile(filepath.Join(layerDir, "metadata.json")); rErr == nil {
						_ = json.Unmarshal(raw, &metaDoc)
					}
					displayName := slug
					srcCRS := "EPSG:4326"
					geomType := "Geometry"
					featCount := 0
					minZ := 0
					maxZ := 14
					var bounds []float64
					var fields []map[string]interface{}
					var gTypes []string
					if metaDoc != nil {
						if ln, _ := metaDoc["layer_name"].(string); ln != "" {
							displayName = ln
						}
						if exportedLayerKeys[ds.ID+"::"+displayName] {
							continue
						}
						if sc, _ := metaDoc["source_crs"].(string); sc != "" {
							srcCRS = sc
						}
						if gt, _ := metaDoc["geometry_type"].(string); gt != "" {
							geomType = gt
						}
						featCount = int(getDim(metaDoc["feature_count"]))
						minZ = int(getDim(metaDoc["min_zoom"]))
						if mz := int(getDim(metaDoc["max_zoom"])); mz > 0 {
							maxZ = mz
						}
						if bRaw, ok := metaDoc["bounds"].([]interface{}); ok && len(bRaw) == 4 {
							bounds = []float64{getDim(bRaw[0]), getDim(bRaw[1]), getDim(bRaw[2]), getDim(bRaw[3])}
						}
						if rawF, ok := metaDoc["fields"]; ok {
							if b, mErr := json.Marshal(rawF); mErr == nil {
								_ = json.Unmarshal(b, &fields)
							}
						}
						if rawGT, ok := metaDoc["geometry_types"]; ok {
							if b, mErr := json.Marshal(rawGT); mErr == nil {
								_ = json.Unmarshal(b, &gTypes)
							}
						}
					}
					_ = exportVectorLayerDir(
						ds.ID, "", displayName, slug, layerDir,
						srcCRS, geomType, gTypes,
						featCount, bounds, minZ, maxZ,
						1.0, true, len(vectorEntries)+dIdx+lIdx, fields, nil,
					)
					exportedLayerKeys[ds.ID+"::"+slug] = true
				}
			}
		}

		// Write combined GeoJSON files inside Vector_Layers/ (GeoJSON only)
		if len(combinedAllFeatures) > 0 {
			combinedAllDoc := map[string]interface{}{
				"type": "FeatureCollection",
				"name": projectName,
				"crs": map[string]interface{}{
					"type": "name",
					"properties": map[string]interface{}{
						"name": "urn:ogc:def:crs:OGC:1.3:CRS84",
					},
				},
				"features": combinedAllFeatures,
			}
			_ = addJSONToZip("Vector_Layers/all_layers.geojson", combinedAllDoc)
		}
		if len(combinedPointFeatures) > 0 {
			combinedPointsDoc := map[string]interface{}{
				"type": "FeatureCollection",
				"name": projectName + " Points",
				"crs": map[string]interface{}{
					"type": "name",
					"properties": map[string]interface{}{
						"name": "urn:ogc:def:crs:OGC:1.3:CRS84",
					},
				},
				"features": combinedPointFeatures,
			}
			_ = addJSONToZip("Vector_Layers/points.geojson", combinedPointsDoc)
		}
	}

	// =========================================================================
	// C. BASE LAYERS -> Viewscape/base_layers/sources.json + local_tiles/
	// =========================================================================
	if includeBase {
		s.ProgressManager().UpdateStage(primaryDatasetID, 5, "Exporting base layer sources", "")
		baseEntries = s.collectViewscapeBaseLayers(ctx, workspaceID, primaryProj, params, addFileToZip, datasetMap)
		sourcesDoc := map[string]interface{}{
			"schema_version": "1.0",
			"exported_at":    time.Now().UTC().Format(time.RFC3339),
			"sources":        baseEntries,
			"notes":          "Remote tile sources (OSM, WMS, Bing, custom XYZ) are preserved as source configurations; remote tiles are only cached locally when explicitly permitted.",
		}
		_ = addJSONToZip("Viewscape/base_layers/sources.json", sourcesDoc)
		// Ensure local_tiles/ folder exists in ZIP even when only remote base configs are present
		_ = addBytesToZip("Viewscape/base_layers/local_tiles/.keep", []byte{})
		for _, be := range baseEntries {
			layerStyles["base"] = append(layerStyles["base"].([]map[string]interface{}), map[string]interface{}{
				"id":          be.ID,
				"name":        be.Name,
				"source_type": be.SourceType,
				"opacity":     be.Opacity,
				"visible":     be.Visible,
			})
		}
	}

	if len(unionBounds) != 4 {
		unionBounds = []float64{-180, -85, 180, 85}
	}
	center := []float64{
		(unionBounds[0] + unionBounds[2]) / 2.0,
		(unionBounds[1] + unionBounds[3]) / 2.0,
	}

	// Strict Viewscape layer ordering: 1. Vector layers, 2. Raster layers, 3. Base layers
	layerOrdering := []string{"vector_layers", "raster_layers", "base_layers"}

	// Styles file
	_ = addJSONToZip("Viewscape/styles/layer_styles.json", layerStyles)

	// Buildings metadata
	var bldDocs []map[string]interface{}
	for _, b := range allBuildings {
		if b == nil {
			continue
		}
		if rawB, mErr := json.Marshal(b); mErr == nil {
			var bm map[string]interface{}
			if json.Unmarshal(rawB, &bm) == nil {
				bldDocs = append(bldDocs, bm)
			}
		}
	}

	// Project configuration (for complete Viewscape or reopening extent/zoom/ordering/interactions)
	var allIconPoints []*models.IconPoint
	var allControlPoints []*models.ControlPoint
	if mode == ViewscapeModeComplete || mode == ViewscapeModeRaster {
		for _, ds := range rasterDatasets {
			cps, _ := s.repo.ListControlPoints(sessionID, ds.ID)
			allControlPoints = append(allControlPoints, cps...)
			ips, _ := s.repo.ListIconPoints(sessionID, ds.ID, "")
			allIconPoints = append(allIconPoints, ips...)
		}
	}

	viewscapeConfig := map[string]interface{}{
		"schema_version": "1.0",
		"project_name":   projectName,
		"package_type":   string(mode),
		"layer_ordering": layerOrdering,
		"camera": map[string]interface{}{
			"center":       center,
			"bounds":       unionBounds,
			"initial_zoom": initZoom,
			"min_zoom":     minZoom,
			"max_zoom":     maxZoom,
			"bearing":      0,
			"pitch":        0,
		},
		"raster_layers":  rasterEntries,
		"vector_layers":  vectorEntries,
		"base_layers":    baseEntries,
		"buildings":      bldDocs,
		"control_points": allControlPoints,
		"icon_points":    allIconPoints,
		"interaction": map[string]interface{}{
			"point_selection":   true,
			"hover_highlight":   true,
			"attribute_popups":  true,
			"query_method":      "queryRenderedFeatures",
			"preserve_orig_fid": true,
		},
	}
	_ = addJSONToZip("Viewscape/project/viewscape_config.json", viewscapeConfig)

	manifest := ViewscapeManifest{
		SchemaVersion: "1.0",
		Format:        string(mode),
		PackageType:   string(mode),
		Application:   "Veracity Maps",
		ProjectName:   projectName,
		WorkspaceID:   workspaceID,
		PrimaryID:     primaryDatasetID,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		CRS:           "EPSG:3857",
		BoundsCRS:     "EPSG:4326",
		Bounds:        unionBounds,
		Center:        center,
		InitialZoom:   initZoom,
		MinZoom:       minZoom,
		MaxZoom:       maxZoom,
		LayerOrdering: layerOrdering,
		RasterLayers:  rasterEntries,
		VectorLayers:  vectorEntries,
		BaseLayers:    baseEntries,
		Paths: map[string]string{
			"raster_layers": "raster_layers/",
			"vector_layers": "vector_layers/",
			"base_layers":   "base_layers/",
			"styles":        "styles/layer_styles.json",
			"project":       "project/viewscape_config.json",
		},
		Buildings: bldDocs,
		InteractionMode: map[string]interface{}{
			"interactive_points": true,
			"hover_highlight":    true,
			"attribute_popups":   true,
			"promote_id":         "feature_id",
		},
	}

	s.ProgressManager().UpdateStage(primaryDatasetID, 7, "Packaging Viewscape ZIP", filename)
	if err := addJSONToZip("Viewscape/manifest.json", manifest); err != nil {
		s.ProgressManager().FailExport(primaryDatasetID, err)
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed writing Viewscape/manifest.json: %w", err)
	}

	_ = archive.Close()
	_ = zipFile.Close()

	s.ProgressManager().UpdateStage(primaryDatasetID, 8, "Validating Viewscape archive", filename)
	if _, valErr := ValidateExport(zipPath, string(mode), primaryDatasetID); valErr != nil {
		s.ProgressManager().FailExport(primaryDatasetID, valErr)
		return "", "", fmt.Errorf("EXPORT_VALIDATION_FAILED: %w", valErr)
	}

	fi, _ := os.Stat(zipPath)
	var finalSize int64
	if fi != nil {
		finalSize = fi.Size()
	}
	s.ProgressManager().CompleteExport(primaryDatasetID, finalSize)
	return zipPath, filename, nil
}

// collectViewscapeBaseLayers builds Viewscape/base_layers/sources.json and packages local MBTiles/tiles when available.
func (s *ExportService) collectViewscapeBaseLayers(
	ctx context.Context,
	workspaceID string,
	primaryProj *models.MapProject,
	params SelectiveExportParams,
	addFileToZip func(archivePath, diskPath string) error,
	datasetMap map[string]*models.Dataset,
) []ViewscapeBaseEntry {
	entries := make([]ViewscapeBaseEntry, 0)
	seenIDs := make(map[string]bool)

	// 1. Check BaseLayerService for active workspace base layer + registered custom base layers
	if s.baseLayerService != nil && s.baseLayerService.repo != nil {
		if wbl, err := s.baseLayerService.repo.GetWorkspaceBaseLayer(ctx, workspaceID); err == nil && wbl != nil {
			if wbl.BaseLayer != nil && !wbl.BaseLayer.IsDefault {
				bl := wbl.BaseLayer
				entry := ViewscapeBaseEntry{
					ID:                       bl.ID,
					Name:                     bl.Name,
					SourceType:               firstNonEmptyBase(bl.SourceType, "custom"),
					CRS:                      firstNonEmptyBase(bl.SourceCRS, "EPSG:3857"),
					Bounds:                   bl.Extent,
					MinZoom:                  0,
					MaxZoom:                  22,
					Opacity:                  wbl.Opacity,
					Visible:                  wbl.Visibility,
					Offline:                  false,
					RemoteTileCachePermitted: false,
					Config:                   bl.RenderingSource,
				}
				// Package local MBTiles or local raster tiles when backed by a local dataset
				if bl.DatasetID != "" {
					if ds := datasetMap[bl.DatasetID]; ds != nil {
						if strings.EqualFold(filepath.Ext(ds.StoragePath), ".mbtiles") && fileExists(ds.StoragePath) {
							mbRel := fmt.Sprintf("base_layers/local_tiles/%s.mbtiles", safeStem(bl.Name))
							if addFileToZip("Viewscape/"+mbRel, ds.StoragePath) == nil {
								entry.SourceType = "mbtiles"
								entry.MBTilesPath = mbRel
								entry.Offline = true
							}
						} else if ds.TilePath != "" && fileExists(ds.TilePath) && strings.EqualFold(filepath.Ext(ds.TilePath), ".mbtiles") {
							mbRel := fmt.Sprintf("base_layers/local_tiles/%s.mbtiles", safeStem(bl.Name))
							if addFileToZip("Viewscape/"+mbRel, ds.TilePath) == nil {
								entry.SourceType = "mbtiles"
								entry.MBTilesPath = mbRel
								entry.Offline = true
							}
						} else {
							localXYZ := filepath.Join(resolveDatasetDirectory(s.storage, ds), "tiles_xyz")
							if dirExists(localXYZ) {
								copied := 0
								_ = filepath.Walk(localXYZ, func(p string, info os.FileInfo, wErr error) error {
									if wErr != nil || info.IsDir() {
										return nil
									}
									rel, rErr := filepath.Rel(localXYZ, p)
									if rErr == nil {
										if addFileToZip(fmt.Sprintf("Viewscape/base_layers/local_tiles/%s/%s", safeStem(bl.Name), filepath.ToSlash(rel)), p) == nil {
											copied++
										}
									}
									return nil
								})
								if copied > 0 {
									entry.SourceType = "local_tiles"
									entry.LocalTilesPath = fmt.Sprintf("base_layers/local_tiles/%s/{z}/{x}/{y}.png", safeStem(bl.Name))
									entry.Offline = true
								}
							}
						}
					}
				}
				entries = append(entries, entry)
				seenIDs[entry.ID] = true
			}
		}
	}

	// 2. Check custom base layer config passed in MapProject configuration (WMS, custom web tiles, Bing, etc.)
	if primaryProj != nil && primaryProj.Configuration != nil {
		if blCfg, ok := primaryProj.Configuration["base_layer"].(map[string]interface{}); ok && blCfg != nil {
			blID, _ := blCfg["id"].(string)
			if blID == "" {
				blID = "project_base_layer"
			}
			if !seenIDs[blID] {
				srcType, _ := blCfg["source_type"].(string)
				if srcType == "" {
					srcType, _ = blCfg["type"].(string)
				}
				if srcType == "" {
					srcType = "xyz"
				}
				name, _ := blCfg["name"].(string)
				if name == "" {
					name = "Configured Base Layer"
				}
				urlStr, _ := blCfg["url"].(string)
				wmsLayers, _ := blCfg["layers"].(string)
				provider, _ := blCfg["provider"].(string)
				attr, _ := blCfg["attribution"].(string)
				vis := true
				if v, ok := blCfg["visible"].(bool); ok {
					vis = v
				}
				op := 1.0
				if o := getDim(blCfg["opacity"]); o > 0 && o <= 1.0 {
					op = o
				}
				permitted, _ := blCfg["remote_tile_cache_permitted"].(bool)
				entries = append(entries, ViewscapeBaseEntry{
					ID:                       blID,
					Name:                     name,
					SourceType:               srcType,
					Provider:                 provider,
					URL:                      urlStr,
					WMSLayers:                wmsLayers,
					Attribution:              attr,
					CRS:                      "EPSG:3857",
					MinZoom:                  0,
					MaxZoom:                  19,
					Opacity:                  op,
					Visible:                  vis,
					Offline:                  false,
					RemoteTileCachePermitted: permitted,
					Config:                   blCfg,
				})
				seenIDs[blID] = true
			}
		}
	}

	// 3. Preserve any explicit extra base sources passed via export options (WMS / custom XYZ / Bing)
	if params.Options != nil && len(params.Options.BaseLayerSources) > 0 {
		for idx, rawSrc := range params.Options.BaseLayerSources {
			id, _ := rawSrc["id"].(string)
			if id == "" {
				id = fmt.Sprintf("custom_base_%d", idx+1)
			}
			if seenIDs[id] {
				continue
			}
			name, _ := rawSrc["name"].(string)
			srcType, _ := rawSrc["source_type"].(string)
			if srcType == "" {
				srcType, _ = rawSrc["type"].(string)
			}
			urlStr, _ := rawSrc["url"].(string)
			wmsLayers, _ := rawSrc["wms_layers"].(string)
			provider, _ := rawSrc["provider"].(string)
			attr, _ := rawSrc["attribution"].(string)
			permitted, _ := rawSrc["remote_tile_cache_permitted"].(bool)
			entries = append(entries, ViewscapeBaseEntry{
				ID:                       id,
				Name:                     firstNonEmptyBase(name, id),
				SourceType:               firstNonEmptyBase(srcType, "xyz"),
				Provider:                 provider,
				URL:                      urlStr,
				WMSLayers:                wmsLayers,
				Attribution:              attr,
				CRS:                      "EPSG:3857",
				MinZoom:                  0,
				MaxZoom:                  19,
				Opacity:                  1.0,
				Visible:                  true,
				Offline:                  false,
				RemoteTileCachePermitted: permitted,
				Config:                   rawSrc,
			})
			seenIDs[id] = true
		}
	}

	// 4. Package any standalone local MBTiles dataset in session under local_tiles/
	for _, ds := range datasetMap {
		if ds != nil && strings.EqualFold(filepath.Ext(ds.StoragePath), ".mbtiles") && fileExists(ds.StoragePath) {
			mbID := "mbtiles_" + ds.ID
			if !seenIDs[mbID] {
				mbRel := fmt.Sprintf("base_layers/local_tiles/%s.mbtiles", safeStem(ds.Filename))
				if addFileToZip("Viewscape/"+mbRel, ds.StoragePath) == nil {
					entries = append(entries, ViewscapeBaseEntry{
						ID:          mbID,
						Name:        ds.Filename,
						SourceType:  "mbtiles",
						CRS:         "EPSG:3857",
						Bounds:      extractDatasetBounds(ds),
						MinZoom:     0,
						MaxZoom:     22,
						Opacity:     1.0,
						Visible:     true,
						Offline:     true,
						MBTilesPath: mbRel,
					})
					seenIDs[mbID] = true
				}
			}
		}
	}

	// 5. Default OpenStreetMap base source if no base layers were recorded
	if len(entries) == 0 {
		entries = append(entries, ViewscapeBaseEntry{
			ID:                       "osm_standard",
			Name:                     "OpenStreetMap Standard",
			SourceType:               "xyz",
			Provider:                 "openstreetmap",
			URL:                      "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
			Attribution:              "© OpenStreetMap contributors",
			CRS:                      "EPSG:3857",
			MinZoom:                  0,
			MaxZoom:                  19,
			Opacity:                  1.0,
			Visible:                  true,
			Offline:                  false,
			RemoteTileCachePermitted: false,
		})
	}

	// Compute deterministic digest comment-free (keeps imports clean)
	_ = sha256.New
	_ = hex.EncodeToString

	return entries
}

func firstNonEmptyBase(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

type pythonGPKGGeoJSONLayer struct {
	LayerName    string    `json:"layer_name"`
	LayerSlug    string    `json:"layer_slug"`
	GeoJSONPath  string    `json:"geojson_path"`
	Filename     string    `json:"filename"`
	SourceCRS    string    `json:"source_crs"`
	GeometryType string    `json:"geometry_type"`
	FeatureCount int       `json:"feature_count"`
	Bounds       []float64 `json:"bounds"`
}

type pythonGPKGGeoJSONResult struct {
	Success           bool                     `json:"success"`
	LayerCount        int                      `json:"layer_count"`
	PointCount        int                      `json:"point_count"`
	PointsGeoJSONPath string                   `json:"points_geojson_path"`
	Layers            []pythonGPKGGeoJSONLayer `json:"layers"`
	Error             string                   `json:"error,omitempty"`
}

func (s *ExportService) convertGPKGToGeoJSONWithPython(
	ctx context.Context,
	srcPath, outDir, layerName string,
) (*pythonGPKGGeoJSONResult, error) {
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

	scriptPath := "backend/scripts/gpkg_to_geojson.py"
	if !fileExists(scriptPath) {
		for _, cs := range []string{
			"scripts/gpkg_to_geojson.py",
			"../scripts/gpkg_to_geojson.py",
			"../backend/scripts/gpkg_to_geojson.py",
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
	cmd := exec.CommandContext(ctx, pythonExe, scriptPath, srcPath, outDir, layerName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gpkg_to_geojson failed: %v: %s", err, stderr.String())
	}
	var res pythonGPKGGeoJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("failed decoding gpkg_to_geojson output: %v", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("gpkg_to_geojson error: %s", res.Error)
	}
	return &res, nil
}

type pythonVectorMBTilesResult struct {
	Success      bool      `json:"success"`
	OutputFile   string    `json:"output_file"`
	Format       string    `json:"format"`
	LayerCount   int       `json:"layer_count"`
	FeatureCount int       `json:"feature_count"`
	TileCount    int       `json:"tile_count"`
	MinZoom      int       `json:"min_zoom"`
	MaxZoom      int       `json:"max_zoom"`
	Bounds       []float64 `json:"bounds"`
	Error        string    `json:"error,omitempty"`
}

func (s *ExportService) convertGPKGToVectorMBTilesWithPython(
	ctx context.Context,
	srcPath, outMBTilesPath string,
	minZoom, maxZoom int,
	mapName string,
) (*pythonVectorMBTilesResult, error) {
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

	scriptPath := "backend/scripts/gpkg_to_vector_mbtiles.py"
	if !fileExists(scriptPath) {
		for _, cs := range []string{
			"scripts/gpkg_to_vector_mbtiles.py",
			"../scripts/gpkg_to_vector_mbtiles.py",
			"../backend/scripts/gpkg_to_vector_mbtiles.py",
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
		srcPath,
		outMBTilesPath,
		fmt.Sprintf("%d", minZoom),
		fmt.Sprintf("%d", maxZoom),
		mapName,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gpkg_to_vector_mbtiles failed: %v: %s", err, stderr.String())
	}
	var res pythonVectorMBTilesResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("failed decoding gpkg_to_vector_mbtiles output: %v", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("gpkg_to_vector_mbtiles error: %s", res.Error)
	}
	return &res, nil
}

// writeCustomLocalTileFolder writes a self-contained MapTiler Engine Custom Local Tile folder
// matching E:\New folder\New folder:
//
//	<folderPrefix>/{z}/{x}/{y}      (extensionless PNG/JPG tile)
//	<folderPrefix>/{z}/{x}/{y}.kml  (companion KML tile)
//	<folderPrefix>/metadata.json
//	<folderPrefix>/index.html
//	<folderPrefix>/leaflet.html
//	<folderPrefix>/openlayers.html
//	<folderPrefix>/googlemaps.html
//	<folderPrefix>/doc.kml
func writeCustomLocalTileFolder(
	folderPrefix, displayName, tileDir string,
	bounds []float64,
	minZ, maxZ int,
	addFileToZip func(archivePath, diskPath string) error,
	addBytesToZip func(archivePath string, data []byte) error,
	addJSONToZip func(archivePath string, v interface{}) error,
) {
	prefix := strings.Trim(filepath.ToSlash(folderPrefix), "/")
	if prefix == "" || !dirExists(tileDir) {
		return
	}
	actualMin := -1
	actualMax := -1

	_ = filepath.Walk(tileDir, func(p string, info os.FileInfo, wErr error) error {
		if wErr != nil || info.IsDir() {
			return nil
		}
		rel, rErr := filepath.Rel(tileDir, p)
		if rErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		// Strip extension (.png/.jpg) if present so tiles are stored as extensionless {z}/{x}/{y}
		ext := filepath.Ext(relSlash)
		extless := relSlash
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
			extless = strings.TrimSuffix(relSlash, ext)
		}
		parts := strings.Split(extless, "/")
		if len(parts) != 3 {
			return nil
		}
		zNum, errZ := strconv.Atoi(parts[0])
		xNum, errX := strconv.Atoi(parts[1])
		yNum, errY := strconv.Atoi(parts[2])
		if errZ != nil || errX != nil || errY != nil {
			return nil
		}
		if actualMin < 0 || zNum < actualMin {
			actualMin = zNum
		}
		if zNum > actualMax {
			actualMax = zNum
		}
		_ = addFileToZip(fmt.Sprintf("%s/%s", prefix, extless), p)
		tileKML := generateTileKML(zNum, xNum, yNum)
		_ = addBytesToZip(fmt.Sprintf("%s/%d/%d/%d.kml", prefix, zNum, xNum, yNum), []byte(tileKML))
		return nil
	})

	if actualMin >= 0 && actualMax >= actualMin {
		minZ = actualMin
		maxZ = actualMax
	}
	if len(bounds) != 4 {
		bounds = []float64{-180, -85, 180, 85}
	}

	metaDoc := map[string]interface{}{
		"name":        displayName,
		"version":     "1.1.0",
		"description": "",
		"attribution": "<a href=\"https://www.maptiler.com/engine/\">Rendered with MapTiler Engine</a>, non-commercial use only",
		"type":        "overlay",
		"format":      "hybrid",
		"minzoom":     fmt.Sprintf("%d", minZ),
		"maxzoom":     fmt.Sprintf("%d", maxZ),
		"scale":       "1.000000",
		"bounds":      bounds,
		"profile":     "mercator",
	}
	_ = addJSONToZip(prefix+"/metadata.json", metaDoc)
	_ = addBytesToZip(prefix+"/index.html", []byte(generateIndexHTML(displayName, bounds, minZ, maxZ)))
	_ = addBytesToZip(prefix+"/leaflet.html", []byte(generateLeafletHTML(displayName, bounds, minZ, maxZ, nil)))
	_ = addBytesToZip(prefix+"/openlayers.html", []byte(generateOpenLayersHTML(displayName, bounds, minZ, maxZ, nil)))
	_ = addBytesToZip(prefix+"/googlemaps.html", []byte(generateGoogleMapsHTML(displayName, bounds, minZ, maxZ)))
	_ = addBytesToZip(prefix+"/doc.kml", []byte(generateDocKML(displayName, bounds, minZ)))
}
