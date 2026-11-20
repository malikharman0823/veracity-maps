package services

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/storage"
)

// TileConfigRasterLayer defines one raster layer in tile_config.json
type TileConfigRasterLayer struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Format     string    `json:"format"`
	TileURL    string    `json:"tile_url"`
	Bounds     []float64 `json:"bounds"`
	MinZoom    int       `json:"min_zoom"`
	MaxZoom    int       `json:"max_zoom"`
	Opacity    float64   `json:"opacity"`
	Visible    bool      `json:"visible"`
	BuildingID string    `json:"building_id,omitempty"`
	FloorLevel int       `json:"floor_level,omitempty"`
	FloorLabel string    `json:"floor_label,omitempty"`
}

// TileConfigVectorLayer defines one vector layer in tile_config.json
type TileConfigVectorLayer struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Format       string    `json:"format"`
	TileURL      string    `json:"tile_url"`
	GeoJSONURL   string    `json:"geojson_url,omitempty"`
	Bounds       []float64 `json:"bounds,omitempty"`
	MinZoom      int       `json:"min_zoom"`
	MaxZoom      int       `json:"max_zoom"`
	GeometryType string    `json:"geometry_type,omitempty"`
	FeatureCount int       `json:"feature_count,omitempty"`
	Visible      bool      `json:"visible"`
}

// TileConfigBuilding defines a building with floor associations
type TileConfigBuilding struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Floors []TileConfigFloor `json:"floors"`
}

// TileConfigFloor defines a floor with associated layer IDs
type TileConfigFloor struct {
	Level          int      `json:"level"`
	Name           string   `json:"name"`
	RasterLayerID  string   `json:"raster_layer_id,omitempty"`
	VectorLayerIDs []string `json:"vector_layer_ids,omitempty"`
}

// CustomLocalTilesConfig defines the master schema for tile_config.json
type CustomLocalTilesConfig struct {
	Name             string                  `json:"name"`
	Format           string                  `json:"format"`
	Version          string                  `json:"version"`
	Generator        string                  `json:"generator"`
	CreatedAt        string                  `json:"created_at"`
	Bounds           []float64               `json:"bounds"`
	Center           []float64               `json:"center"`
	InitialZoom      int                     `json:"initial_zoom"`
	MinZoom          int                     `json:"min_zoom"`
	MaxZoom          int                     `json:"max_zoom"`
	TileSize         int                     `json:"tile_size"`
	CRS              string                  `json:"crs"`
	BoundsCRS        string                  `json:"bounds_crs"`
	TileScheme       string                  `json:"tile_scheme"`
	RasterLayers     []TileConfigRasterLayer `json:"raster_layers"`
	VectorLayers     []TileConfigVectorLayer `json:"vector_layers"`
	GeoJSONResources map[string]string       `json:"geojson_resources"`
	Styles           map[string]string       `json:"styles"`
	Assets           map[string]string       `json:"assets"`
	Sources          map[string]string       `json:"sources"`
	Metadata         map[string]string       `json:"metadata"`
	Buildings        []TileConfigBuilding    `json:"buildings,omitempty"`
	Summary          map[string]interface{}  `json:"summary,omitempty"`
}

// GenerateCustomLocalTilesExport packages raster layers, vector layers, GeoJSON metadata,
// custom icons, MapLibre styles, and CRS configurations into Custom_Local_Tiles.zip.
func (s *ExportService) GenerateCustomLocalTilesExport(sessionID, primaryDatasetID string, params SelectiveExportParams) (string, string, error) {
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

	allBuildings, _ := s.repo.ListBuildings(sessionID)
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
	if _, ok := datasetMap[primaryDatasetID]; !ok {
		datasetMap[primaryDatasetID] = primaryDs
	}

	primaryProj, _ := s.repo.GetMapProject(sessionID, primaryDatasetID)

	type dsLocation struct {
		Building *models.Building
		Floor    *models.BuildingFloor
	}
	locationByDsID := make(map[string]dsLocation)
	for _, b := range allBuildings {
		for _, fl := range b.Floors {
			for _, dID := range fl.DatasetIDs {
				locationByDsID[dID] = dsLocation{Building: b, Floor: fl}
			}
		}
	}

	selectedLayerSet := make(map[string]bool)
	selectedBuildingSet := make(map[string]bool)
	selectedFloorSet := make(map[string]bool)

	for _, bID := range params.BuildingIDs {
		if strings.TrimSpace(bID) != "" {
			selectedBuildingSet[bID] = true
		}
	}
	for _, fID := range params.FloorIDs {
		if strings.TrimSpace(fID) != "" {
			selectedFloorSet[fID] = true
		}
	}
	for _, lID := range params.LayerIDs {
		if strings.TrimSpace(lID) != "" {
			selectedLayerSet[lID] = true
		}
	}

	if len(selectedLayerSet) == 0 {
		if len(selectedBuildingSet) > 0 {
			for _, b := range allBuildings {
				if selectedBuildingSet[b.ID] {
					for _, fl := range b.Floors {
						for _, dID := range fl.DatasetIDs {
							selectedLayerSet[dID] = true
						}
					}
				}
			}
		} else if len(selectedFloorSet) > 0 {
			for _, b := range allBuildings {
				for _, fl := range b.Floors {
					if selectedFloorSet[fl.ID] {
						for _, dID := range fl.DatasetIDs {
							selectedLayerSet[dID] = true
						}
					}
				}
			}
		} else {
			selectedLayerSet[primaryDatasetID] = true
			if primaryProj != nil && primaryProj.Configuration != nil {
				if rawMembers, ok := primaryProj.Configuration["members"].([]interface{}); ok {
					for _, rm := range rawMembers {
						if m, ok := rm.(map[string]interface{}); ok {
							if mID, _ := m["dataset_id"].(string); mID != "" && datasetMap[mID] != nil {
								selectedLayerSet[mID] = true
							}
						}
					}
				}
			}
			for _, ds := range allDatasets {
				if ds != nil {
					selectedLayerSet[ds.ID] = true
				}
			}
		}
	}

	// Back-propagate selected layers to floors and buildings
	for lID := range selectedLayerSet {
		if loc, ok := locationByDsID[lID]; ok {
			if loc.Building != nil {
				selectedBuildingSet[loc.Building.ID] = true
			}
			if loc.Floor != nil {
				selectedFloorSet[loc.Floor.ID] = true
			}
		}
	}

	projectName := strings.TrimSpace(params.ProjectName)
	if projectName == "" {
		if primaryProj != nil && primaryProj.Name != "" {
			projectName = primaryProj.Name
		} else {
			projectName = strings.TrimSuffix(primaryDs.Filename, filepath.Ext(primaryDs.Filename))
		}
	}
	stem := safeStem(projectName)

	// Section 11 naming standard
	filename := "Custom_Local_Tiles.zip"
	zipPath := filepath.Join(exportDir, filename)

	stageDir := filepath.Join(exportDir, ".stage_custom_local_tiles_"+stem)
	_ = storage.SafeRemoveAll(stageDir)
	_ = os.MkdirAll(stageDir, 0755)
	defer func() {
		_ = storage.SafeRemoveAll(stageDir)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.ProgressManager().StartExport(primaryDatasetID, sessionID, cancel)

	startTime := time.Now()
	var originalDataSize int64
	var completedTilesCount int
	var emptyTilesSkippedCount int
	var cachedTilesReusedCount int

	s.ProgressManager().UpdateStage(primaryDatasetID, 1, "Inspecting selected layers", primaryDs.Filename)

	zipFile, err := os.Create(zipPath)
	if err != nil {
		s.ProgressManager().FailExport(primaryDatasetID, err)
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating Custom Local Tiles archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	tileChecksums := make(map[string]string)
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
		h := sha256.Sum256(data)
		tileChecksums[clean] = hex.EncodeToString(h[:])
		return nil
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

		hasher := sha256.New()
		multi := io.MultiWriter(w, hasher)
		if _, err := io.Copy(multi, f); err != nil {
			return err
		}
		seenZipEntries[clean] = true
		tileChecksums[clean] = hex.EncodeToString(hasher.Sum(nil))
		return nil
	}

	var unionBounds []float64
	expandUnionBounds := func(b []float64) {
		if len(b) != 4 {
			return
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

	// Classify selected layers into Raster vs Vector
	var rasterDatasets []*models.Dataset
	var vectorDatasets []*models.Dataset

	for lID := range selectedLayerSet {
		ds := datasetMap[lID]
		if ds == nil {
			for _, candidate := range datasetMap {
				if candidate != nil {
					if safeStem(candidate.Filename) == safeStem(lID) {
						ds = candidate
						break
					}
					if candidate.ExtractedMetadata != nil {
						if rawLyrs, ok := candidate.ExtractedMetadata["layers"].([]interface{}); ok {
							for _, rl := range rawLyrs {
								if m, ok := rl.(map[string]interface{}); ok {
									if m["layer_name"] == lID || m["id"] == lID || m["dataset_layer_id"] == lID {
										ds = candidate
										break
									}
								}
							}
						}
					}
				}
			}
		}
		if ds == nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(ds.Filename))
		if ext == "" && ds.StoragePath != "" {
			ext = strings.ToLower(filepath.Ext(ds.StoragePath))
		}

		isVector := false
		if ext == ".gpkg" || ext == ".shp" || ext == ".kml" || ext == ".kmz" || ext == ".geojson" || ext == ".osm" || ext == ".pbf" {
			isVector = true
		} else if ds.ExtractedMetadata != nil {
			if docType, ok := ds.ExtractedMetadata["document_type"].(string); ok && strings.EqualFold(docType, "vector") {
				isVector = true
			} else if comp, ok := ds.ExtractedMetadata["content"].(*models.ContentComposition); ok && comp != nil && comp.HasVector && !comp.HasRaster {
				isVector = true
			}
		}

		if isVector {
			vectorDatasets = append(vectorDatasets, ds)
		} else {
			rasterDatasets = append(rasterDatasets, ds)
		}

		dsBounds := extractDatasetBounds(ds)
		if len(dsBounds) == 4 {
			expandUnionBounds(dsBounds)
		}
	}

	if len(unionBounds) != 4 {
		unionBounds = []float64{-180.0, -85.0, 180.0, 85.0}
	}
	center := []float64{
		(unionBounds[0] + unionBounds[2]) / 2.0,
		(unionBounds[1] + unionBounds[3]) / 2.0,
	}

	minZoom := 12
	maxZoom := 22
	initZoom := 18
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

	// Stage 2: Calculate tile coverage
	s.ProgressManager().UpdateStage(primaryDatasetID, 2, "Calculating tile coverage", "")
	totalExpectedTiles := 0
	for _, ds := range rasterDatasets {
		dsBounds := extractDatasetBounds(ds)
		if len(dsBounds) != 4 {
			dsBounds = unionBounds
		}
		clampMinZ, clampMaxZ := gis.DetermineTileZooms(dsBounds, 512, 512)
		if minZoom > clampMinZ {
			clampMinZ = minZoom
		}
		if maxZoom < clampMaxZ {
			clampMaxZ = maxZoom
		}
		if clampMinZ > clampMaxZ {
			clampMinZ = clampMaxZ
		}
		for z := clampMinZ; z <= clampMaxZ; z++ {
			minX, maxY := gis.LatLngToTile(dsBounds[1], dsBounds[0], z)
			maxX, minY := gis.LatLngToTile(dsBounds[3], dsBounds[2], z)
			if minX > maxX {
				minX, maxX = maxX, minX
			}
			if minY > maxY {
				minY, maxY = maxY, minY
			}
			totalExpectedTiles += (maxX - minX + 1) * (maxY - minY + 1)
		}
	}
	for range vectorDatasets {
		totalExpectedTiles += (maxZoom - minZoom + 1) * 2
	}
	if totalExpectedTiles == 0 {
		totalExpectedTiles = 10
	}

	// 1. Process Raster Layers -> raster/<layer_dir>/{z}/{x}/{y}.png
	s.ProgressManager().UpdateStage(primaryDatasetID, 3, "Processing raster layers", "")
	rasterQuality := 85
	if opts.ImageQuality > 0 && opts.ImageQuality <= 100 {
		rasterQuality = opts.ImageQuality
	}
	var rasterConfigLayers []TileConfigRasterLayer
	for _, ds := range rasterDatasets {
		if ctx.Err() != nil {
			s.ProgressManager().FailExport(primaryDatasetID, ctx.Err())
			return "", "", fmt.Errorf("EXPORT_CANCELLED: %w", ctx.Err())
		}
		if ds.FileSize > 0 {
			originalDataSize += ds.FileSize
		} else if fi, statErr := os.Stat(ds.StoragePath); statErr == nil {
			originalDataSize += fi.Size()
		}

		dsBounds := extractDatasetBounds(ds)
		if len(dsBounds) != 4 {
			dsBounds = unionBounds
		}

		bldName := ""
		flLevel := 0
		flLabel := ""
		if loc, ok := locationByDsID[ds.ID]; ok {
			if loc.Building != nil {
				bldName = loc.Building.Name
			}
			if loc.Floor != nil {
				flLevel = loc.Floor.Level
				flLabel = loc.Floor.Name
			}
		}
		if bldName == "" && ds.ExtractedMetadata != nil {
			if bn, ok := ds.ExtractedMetadata["building_name"].(string); ok {
				bldName = bn
			}
			if fl, ok := ds.ExtractedMetadata["floor_level"].(float64); ok {
				flLevel = int(fl)
			}
			if fLab, ok := ds.ExtractedMetadata["floor_label"].(string); ok {
				flLabel = fLab
			}
		}

		layerSlug := safeStem(ds.Filename)
		if flLabel != "" {
			layerSlug = safeStem(flLabel)
		} else if bldName != "" {
			layerSlug = safeStem(fmt.Sprintf("%s_floor_%d", bldName, flLevel))
		}
		if layerSlug == "" {
			layerSlug = ds.ID
		}

		renderPath := s.resolveExportRasterPath(ds)
		if renderPath == "" || !fileExists(renderPath) {
			renderPath = ds.StoragePath
		}

		format := "png"
		if opts.RasterFormat != "" {
			format = strings.ToLower(opts.RasterFormat)
		} else if strings.HasSuffix(strings.ToLower(renderPath), ".jpg") || strings.HasSuffix(strings.ToLower(renderPath), ".jpeg") {
			format = "jpg"
		}

		// Render XYZ tiles for this raster with caching and worker pool
		layerTileDir := filepath.Join(stageDir, "raster", layerSlug)
		_ = os.MkdirAll(layerTileDir, 0755)

		cTiles, skTiles, reTiles := s.getOrCreateCachedRasterTiles(ctx, renderPath, dsBounds, minZoom, maxZoom, format, rasterQuality, layerTileDir)
		completedTilesCount += cTiles
		emptyTilesSkippedCount += skTiles
		cachedTilesReusedCount += reTiles
		s.ProgressManager().UpdateTileMetrics(primaryDatasetID, completedTilesCount+cachedTilesReusedCount, totalExpectedTiles, emptyTilesSkippedCount, cachedTilesReusedCount, layerSlug)

		// Walk layerTileDir and add tiles to zip under Custom_Local_Tiles/raster/<layerSlug>/{z}/{x}/{y}.ext
		_ = filepath.Walk(layerTileDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, rErr := filepath.Rel(layerTileDir, path)
			if rErr != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			archivePath := fmt.Sprintf("Custom_Local_Tiles/raster/%s/%s", layerSlug, relSlash)
			_ = addFileToZip(archivePath, path)
			_ = addFileToZip(fmt.Sprintf("raster/%s/%s", layerSlug, relSlash), path)
			return nil
		})

		tileURL := fmt.Sprintf("./raster/%s/{z}/{x}/{y}.%s", layerSlug, format)
		rasterConfigLayers = append(rasterConfigLayers, TileConfigRasterLayer{
			ID:         layerSlug,
			Name:       ds.Filename,
			Format:     format,
			TileURL:    tileURL,
			Bounds:     dsBounds,
			MinZoom:    minZoom,
			MaxZoom:    maxZoom,
			Opacity:    1.0,
			Visible:    true,
			BuildingID: bldName,
			FloorLevel: flLevel,
			FloorLabel: flLabel,
		})
	}

	// 2. Process Vector Layers -> vector/<layer_name>/{z}/{x}/{y}.pbf
	s.ProgressManager().UpdateStage(primaryDatasetID, 4, "Generating vector tiles", "")
	var vectorConfigLayers []TileConfigVectorLayer
	for _, ds := range vectorDatasets {
		if ctx.Err() != nil {
			s.ProgressManager().FailExport(primaryDatasetID, ctx.Err())
			return "", "", fmt.Errorf("EXPORT_CANCELLED: %w", ctx.Err())
		}
		if ds.FileSize > 0 {
			originalDataSize += ds.FileSize
		} else if fi, statErr := os.Stat(ds.StoragePath); statErr == nil {
			originalDataSize += fi.Size()
		}

		layerSlug := safeStem(ds.Filename)
		if layerSlug == "" {
			layerSlug = ds.ID
		}

		vectorTileDir := filepath.Join(stageDir, "vector", layerSlug)
		_ = os.MkdirAll(filepath.Dir(vectorTileDir), 0755)

		featCount := 0
		geomType := "Vector"
		vRes, reTiles, vErr := s.getOrCreateCachedVectorTiles(ctx, ds.StoragePath, vectorTileDir, layerSlug, minZoom, maxZoom)
		if vErr == nil && vRes != nil {
			featCount = vRes.FeatureCount
			if vRes.GeometryType != "" {
				geomType = vRes.GeometryType
			}
			completedTilesCount += vRes.TileCount
		}
		cachedTilesReusedCount += reTiles
		s.ProgressManager().UpdateTileMetrics(primaryDatasetID, completedTilesCount+cachedTilesReusedCount, totalExpectedTiles, emptyTilesSkippedCount, cachedTilesReusedCount, layerSlug)

		_ = filepath.Walk(vectorTileDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, rErr := filepath.Rel(vectorTileDir, path)
			if rErr != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			archivePath := fmt.Sprintf("Custom_Local_Tiles/vector/%s/%s", layerSlug, relSlash)
			_ = addFileToZip(archivePath, path)
			_ = addFileToZip(fmt.Sprintf("vector/%s/%s", layerSlug, relSlash), path)
			return nil
		})

		// Also bundle GeoJSON if generated by vector_tool.py
		geoJSONRel := fmt.Sprintf("./geojson/%s.geojson", layerSlug)
		stagedGeoJSON := filepath.Join(stageDir, "geojson", layerSlug+".geojson")
		if fileExists(stagedGeoJSON) {
			_ = addFileToZip(fmt.Sprintf("Custom_Local_Tiles/geojson/%s.geojson", layerSlug), stagedGeoJSON)
			_ = addFileToZip(fmt.Sprintf("geojson/%s.geojson", layerSlug), stagedGeoJSON)
		}

		tileURL := fmt.Sprintf("./vector/%s/{z}/{x}/{y}.pbf", layerSlug)
		vectorConfigLayers = append(vectorConfigLayers, TileConfigVectorLayer{
			ID:           layerSlug,
			Name:         ds.Filename,
			Format:       "pbf",
			TileURL:      tileURL,
			GeoJSONURL:   geoJSONRel,
			Bounds:       extractDatasetBounds(ds),
			MinZoom:      minZoom,
			MaxZoom:      maxZoom,
			GeometryType: geomType,
			FeatureCount: featCount,
			Visible:      true,
		})
	}

	// 3. Process GeoJSON metadata (control points, icon points, landmarks, areas)
	s.ProgressManager().UpdateStage(primaryDatasetID, 5, "Processing GeoJSON metadata", "")
	var allControlPoints []*models.ControlPoint
	var allIconPoints []*models.IconPoint

	seenCP := make(map[string]bool)
	for lID := range selectedLayerSet {
		cps, _ := s.repo.ListControlPoints(sessionID, lID)
		for _, cp := range cps {
			if cp != nil && !seenCP[cp.ID] {
				seenCP[cp.ID] = true
				allControlPoints = append(allControlPoints, cp)
			}
		}
		ips, _ := s.repo.ListIconPoints(sessionID, lID, "")
		for _, ip := range ips {
			if ip != nil {
				allIconPoints = append(allIconPoints, ip)
			}
		}
	}
	for bID := range selectedBuildingSet {
		ips, _ := s.repo.ListIconPoints(sessionID, "", bID)
		for _, ip := range ips {
			if ip != nil {
				allIconPoints = append(allIconPoints, ip)
			}
		}
	}

	sessionDir := s.storage.GetSessionDir(sessionID)
	iconAssets := make(map[string]string)
	if len(allIconPoints) > 0 {
		bundled := bundleReferencedIcons(archive, allIconPoints, sessionDir)
		for iconID, arcPath := range bundled {
			iconAssets[iconID] = arcPath
			if !strings.HasPrefix(arcPath, "Custom_Local_Tiles/") {
				diskSource := filepath.Join(sessionDir, "icons", filepath.Base(arcPath))
				if fileExists(diskSource) {
					_ = addFileToZip(fmt.Sprintf("Custom_Local_Tiles/assets/icons/%s", filepath.Base(arcPath)), diskSource)
				}
			}
		}
	}

	cpFC := buildControlPointsGeoJSON(allControlPoints)
	cpData, _ := json.MarshalIndent(cpFC, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/geojson/control_points.geojson", cpData)
	_ = addBytesToZip("geojson/control_points.geojson", cpData)

	ipFC := buildIconPointsGeoJSON(allIconPoints)
	ipData, _ := json.MarshalIndent(ipFC, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/geojson/icon_points.geojson", ipData)
	_ = addBytesToZip("geojson/icon_points.geojson", ipData)

	landmarksFC := buildLandmarksGeoJSON(allIconPoints)
	lmData, _ := json.MarshalIndent(landmarksFC, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/geojson/landmarks.geojson", lmData)
	_ = addBytesToZip("geojson/landmarks.geojson", lmData)

	areasFC := buildAreasGeoJSON(primaryDs, datasetMap, selectedLayerSet)
	arData, _ := json.MarshalIndent(areasFC, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/geojson/areas.geojson", arData)
	_ = addBytesToZip("geojson/areas.geojson", arData)

	// 4. Styles: styles/map_style.json (MapLibre Style Spec v8)
	styleDoc := buildMapLibreStyleDoc(projectName, unionBounds, center, initZoom, rasterConfigLayers, vectorConfigLayers)
	styleData, _ := json.MarshalIndent(styleDoc, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/styles/map_style.json", styleData)
	_ = addBytesToZip("styles/map_style.json", styleData)

	// 5. Sources: sources/external_sources.json
	extSourcesDoc := map[string]interface{}{
		"sources": []map[string]interface{}{
			{
				"id":                "osm_standard",
				"type":              "raster-xyz",
				"name":              "OpenStreetMap",
				"url":               "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
				"attribution":       "© OpenStreetMap contributors",
				"offline":           false,
				"requires_internet": true,
			},
		},
	}
	extData, _ := json.MarshalIndent(extSourcesDoc, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/sources/external_sources.json", extData)
	_ = addBytesToZip("sources/external_sources.json", extData)

	// 6. Metadata: metadata/crs.json, metadata/layer_metadata.json, metadata/georeferencing.json
	crsDoc := map[string]interface{}{
		"tile_crs":   "EPSG:3857",
		"bounds_crs": "EPSG:4326",
		"bounds":     unionBounds,
		"center":     center,
	}
	crsData, _ := json.MarshalIndent(crsDoc, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/metadata/crs.json", crsData)
	_ = addBytesToZip("metadata/crs.json", crsData)

	layerMetaDoc := map[string]interface{}{
		"project_name":   projectName,
		"primary_id":     primaryDatasetID,
		"raster_layers":  rasterConfigLayers,
		"vector_layers":  vectorConfigLayers,
		"total_rasters":  len(rasterConfigLayers),
		"total_vectors":  len(vectorConfigLayers),
		"control_points": len(allControlPoints),
		"icon_points":    len(allIconPoints),
	}
	layerMetaData, _ := json.MarshalIndent(layerMetaDoc, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/metadata/layer_metadata.json", layerMetaData)
	_ = addBytesToZip("metadata/layer_metadata.json", layerMetaData)

	georefDoc := buildGeoreferencingMetadataDoc(rasterDatasets, allControlPoints)
	georefData, _ := json.MarshalIndent(georefDoc, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/metadata/georeferencing.json", georefData)
	_ = addBytesToZip("metadata/georeferencing.json", georefData)

	// 7. Buildings and floor configuration
	var buildingsConfig []TileConfigBuilding
	for _, b := range allBuildings {
		if !selectedBuildingSet[b.ID] {
			continue
		}
		var floors []TileConfigFloor
		for _, fl := range b.Floors {
			rID := ""
			for _, dID := range fl.DatasetIDs {
				if selectedLayerSet[dID] {
					rID = dID
					break
				}
			}
			floors = append(floors, TileConfigFloor{
				Level:         fl.Level,
				Name:          fl.Name,
				RasterLayerID: rID,
			})
		}
		buildingsConfig = append(buildingsConfig, TileConfigBuilding{
			ID:     b.ID,
			Name:   b.Name,
			Floors: floors,
		})
	}

	// 8. Master tile_config.json
	configDoc := CustomLocalTilesConfig{
		Name:         projectName,
		Format:       "custom_local_tiles",
		Version:      "1.0.0",
		Generator:    "Veracity Maps Custom Local Tiles Generator",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Bounds:       unionBounds,
		Center:       center,
		InitialZoom:  initZoom,
		MinZoom:      minZoom,
		MaxZoom:      maxZoom,
		TileSize:     256,
		CRS:          "EPSG:3857",
		BoundsCRS:    "EPSG:4326",
		TileScheme:   "xyz",
		RasterLayers: rasterConfigLayers,
		VectorLayers: vectorConfigLayers,
		GeoJSONResources: map[string]string{
			"control_points": "./geojson/control_points.geojson",
			"icon_points":    "./geojson/icon_points.geojson",
			"landmarks":      "./geojson/landmarks.geojson",
			"areas":          "./geojson/areas.geojson",
		},
		Styles: map[string]string{
			"map_style": "./styles/map_style.json",
		},
		Assets: map[string]string{
			"icons": "./assets/icons",
		},
		Sources: map[string]string{
			"external_sources": "./sources/external_sources.json",
		},
		Metadata: map[string]string{
			"crs":            "./metadata/crs.json",
			"layer_metadata": "./metadata/layer_metadata.json",
			"georeferencing": "./metadata/georeferencing.json",
		},
		Buildings: buildingsConfig,
		Summary: map[string]interface{}{
			"selected_rasters":         len(rasterConfigLayers),
			"selected_vectors":         len(vectorConfigLayers),
			"control_points":           len(allControlPoints),
			"icon_points":              len(allIconPoints),
			"duration_seconds":         math.Round(time.Since(startTime).Seconds()*100) / 100,
			"original_data_size_bytes": originalDataSize,
			"total_tiles_generated":    completedTilesCount,
			"empty_tiles_skipped":      emptyTilesSkippedCount,
			"cached_tiles_reused":      cachedTilesReusedCount,
		},
	}
	for _, v := range vectorConfigLayers {
		if v.GeoJSONURL != "" {
			configDoc.GeoJSONResources[v.ID] = v.GeoJSONURL
		}
	}
	configData, _ := json.MarshalIndent(configDoc, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/tile_config.json", configData)
	_ = addBytesToZip("tile_config.json", configData)

	// Backwards compatibility manifest.json & project.json
	_ = addBytesToZip("Custom_Local_Tiles/manifest.json", configData)
	_ = addBytesToZip("manifest.json", configData)
	_ = addBytesToZip("project.json", configData)

	// Stage 6: Reusing cached tiles
	if cachedTilesReusedCount > 0 {
		s.ProgressManager().UpdateStage(primaryDatasetID, 6, "Reusing cached tiles", fmt.Sprintf("%d cached tiles reused", cachedTilesReusedCount))
	}

	// Stage 7: Packaging ZIP
	s.ProgressManager().UpdateStage(primaryDatasetID, 7, "Packaging ZIP", filename)

	// Generate and bundle tile_manifest.json (Section 7 standard)
	manifestDoc := map[string]interface{}{
		"manifest_version": "1.0.0",
		"name":             projectName,
		"format":           "custom_local_tiles",
		"created_at":       time.Now().UTC().Format(time.RFC3339),
		"bounds":           unionBounds,
		"center":           center,
		"zoom_range":       []int{minZoom, maxZoom},
		"performance": map[string]interface{}{
			"duration_seconds":         math.Round(time.Since(startTime).Seconds()*100) / 100,
			"original_data_size_bytes": originalDataSize,
			"total_tiles_generated":    completedTilesCount,
			"empty_tiles_skipped":      emptyTilesSkippedCount,
			"cached_tiles_reused":      cachedTilesReusedCount,
		},
		"tile_counts": map[string]interface{}{
			"raster": len(rasterConfigLayers),
			"vector": len(vectorConfigLayers),
			"total":  completedTilesCount + cachedTilesReusedCount,
		},
		"tile_checksums": tileChecksums,
	}
	manifestData, _ := json.MarshalIndent(manifestDoc, "", "  ")
	_ = addBytesToZip("Custom_Local_Tiles/tile_manifest.json", manifestData)
	_ = addBytesToZip("tile_manifest.json", manifestData)

	_ = archive.Close()
	_ = zipFile.Close()

	// Stage 8: Validating export
	s.ProgressManager().UpdateStage(primaryDatasetID, 8, "Validating export", filename)
	if _, valErr := ValidateExport(zipPath, "custom_local_tiles", primaryDatasetID); valErr != nil {
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

type tileCacheManifest struct {
	Hash       string   `json:"hash"`
	Format     string   `json:"format"`
	MinZoom    int      `json:"min_zoom"`
	MaxZoom    int      `json:"max_zoom"`
	TileCount  int      `json:"tile_count"`
	TilePaths  []string `json:"tile_paths"`
	GeoJSONRel string   `json:"geojson_rel,omitempty"`
}

func computeRasterContentHash(filePath string, bounds []float64, minZ, maxZ int, format string, quality int) string {
	fi, err := os.Stat(filePath)
	if err != nil {
		return ""
	}
	keyStr := fmt.Sprintf("%s:%d:%d:%.6f,%.6f,%.6f,%.6f:%d:%d:%s:%d",
		filepath.Base(filePath),
		fi.Size(),
		fi.ModTime().UnixNano(),
		bounds[0], bounds[1], bounds[2], bounds[3],
		minZ, maxZ, format, quality,
	)
	h := sha256.Sum256([]byte(keyStr))
	return hex.EncodeToString(h[:12])
}

func computeVectorContentHash(filePath, layerName string, minZ, maxZ int) string {
	fi, err := os.Stat(filePath)
	if err != nil {
		return ""
	}
	keyStr := fmt.Sprintf("%s:%s:%d:%d:%d:%d",
		filepath.Base(filePath),
		layerName,
		fi.Size(),
		fi.ModTime().UnixNano(),
		minZ, maxZ,
	)
	h := sha256.Sum256([]byte(keyStr))
	return hex.EncodeToString(h[:12])
}

func (s *ExportService) getTileCacheDir(layerSlug, contentHash string) string {
	base := ""
	if s.storage != nil {
		base = s.storage.ProcessedDir
	}
	if base == "" {
		base = filepath.Join("storage", "processed")
	}
	dir := filepath.Join(base, "tile_cache", layerSlug+"_"+contentHash)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func isTileEmptyOrTransparent(img *image.RGBA) bool {
	pix := img.Pix
	for i := 3; i < len(pix); i += 4 {
		if pix[i] > 0 {
			return false
		}
	}
	return true
}

func (s *ExportService) getOrCreateCachedRasterTiles(ctx context.Context, renderPath string, dsBounds []float64, minZ, maxZ int, format string, quality int, layerTileDir string) (int, int, int) {
	cacheKey := computeRasterContentHash(renderPath, dsBounds, minZ, maxZ, format, quality)
	cacheDir := s.getTileCacheDir(filepath.Base(layerTileDir), cacheKey)

	manifestPath := filepath.Join(cacheDir, "cache_manifest.json")
	if fileExists(manifestPath) {
		raw, mErr := os.ReadFile(manifestPath)
		if mErr == nil {
			var mDoc tileCacheManifest
			if json.Unmarshal(raw, &mDoc) == nil && mDoc.TileCount > 0 {
				valid := true
				for _, p := range mDoc.TilePaths {
					if !fileExists(filepath.Join(cacheDir, filepath.FromSlash(p))) {
						valid = false
						break
					}
				}
				if valid {
					for _, p := range mDoc.TilePaths {
						src := filepath.Join(cacheDir, filepath.FromSlash(p))
						dst := filepath.Join(layerTileDir, filepath.FromSlash(p))
						_ = os.MkdirAll(filepath.Dir(dst), 0755)
						_ = storage.CopyFile(src, dst)
					}
					return 0, 0, mDoc.TileCount
				}
			}
		}
	}

	completed, skipped := s.renderRasterXYZTiles(ctx, renderPath, dsBounds, minZ, maxZ, format, quality, layerTileDir)

	if completed > 0 {
		var cachedPaths []string
		_ = filepath.Walk(layerTileDir, func(p string, info os.FileInfo, wErr error) error {
			if wErr != nil || info.IsDir() {
				return nil
			}
			rel, rErr := filepath.Rel(layerTileDir, p)
			if rErr == nil {
				cleanRel := filepath.ToSlash(rel)
				cachedPaths = append(cachedPaths, cleanRel)
				dst := filepath.Join(cacheDir, filepath.FromSlash(rel))
				_ = os.MkdirAll(filepath.Dir(dst), 0755)
				_ = storage.CopyFile(p, dst)
			}
			return nil
		})
		mDoc := tileCacheManifest{
			Hash:      cacheKey,
			Format:    format,
			MinZoom:   minZ,
			MaxZoom:   maxZ,
			TileCount: completed,
			TilePaths: cachedPaths,
		}
		mData, _ := json.Marshal(mDoc)
		_ = os.WriteFile(manifestPath, mData, 0644)
	}

	return completed, skipped, 0
}

func (s *ExportService) getOrCreateCachedVectorTiles(ctx context.Context, srcPath, vectorTileDir, layerSlug string, minZ, maxZ int) (*pythonVectorTileResult, int, error) {
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	default:
	}

	if maxZ > 14 {
		maxZ = 14
	}
	if minZ > maxZ {
		minZ = maxZ
	}

	cacheKey := computeVectorContentHash(srcPath, layerSlug, minZ, maxZ)
	cacheDir := s.getTileCacheDir(layerSlug, cacheKey)

	manifestPath := filepath.Join(cacheDir, "cache_manifest.json")
	if fileExists(manifestPath) {
		raw, mErr := os.ReadFile(manifestPath)
		if mErr == nil {
			var mDoc tileCacheManifest
			if json.Unmarshal(raw, &mDoc) == nil && mDoc.TileCount > 0 {
				valid := true
				for _, p := range mDoc.TilePaths {
					if !fileExists(filepath.Join(cacheDir, filepath.FromSlash(p))) {
						valid = false
						break
					}
				}
				if valid {
					for _, p := range mDoc.TilePaths {
						src := filepath.Join(cacheDir, filepath.FromSlash(p))
						dst := filepath.Join(vectorTileDir, filepath.FromSlash(p))
						_ = os.MkdirAll(filepath.Dir(dst), 0755)
						_ = storage.CopyFile(src, dst)
					}
					if mDoc.GeoJSONRel != "" {
						srcGJ := filepath.Join(cacheDir, mDoc.GeoJSONRel)
						if fileExists(srcGJ) {
							dstGJ := filepath.Join(filepath.Dir(filepath.Dir(vectorTileDir)), "geojson", layerSlug+".geojson")
							_ = os.MkdirAll(filepath.Dir(dstGJ), 0755)
							_ = storage.CopyFile(srcGJ, dstGJ)
						}
					}
					return &pythonVectorTileResult{
						Success:      true,
						LayerName:    layerSlug,
						FeatureCount: mDoc.TileCount,
						TileCount:    mDoc.TileCount,
						MinZoom:      minZ,
						MaxZoom:      maxZ,
					}, mDoc.TileCount, nil
				}
			}
		}
	}

	vRes, vErr := s.generateVectorTilesWithPython(ctx, srcPath, vectorTileDir, layerSlug, minZ, maxZ)
	if vErr != nil {
		return nil, 0, vErr
	}

	if vRes != nil && vRes.TileCount > 0 {
		var cachedPaths []string
		_ = filepath.Walk(vectorTileDir, func(p string, info os.FileInfo, wErr error) error {
			if wErr != nil || info.IsDir() {
				return nil
			}
			rel, rErr := filepath.Rel(vectorTileDir, p)
			if rErr == nil {
				cleanRel := filepath.ToSlash(rel)
				cachedPaths = append(cachedPaths, cleanRel)
				dst := filepath.Join(cacheDir, filepath.FromSlash(rel))
				_ = os.MkdirAll(filepath.Dir(dst), 0755)
				_ = storage.CopyFile(p, dst)
			}
			return nil
		})
		gjRel := ""
		if vRes.GeoJSONFile != "" && fileExists(vRes.GeoJSONFile) {
			gjDst := filepath.Join(cacheDir, layerSlug+".geojson")
			_ = storage.CopyFile(vRes.GeoJSONFile, gjDst)
			gjRel = layerSlug + ".geojson"
		}
		mDoc := tileCacheManifest{
			Hash:       cacheKey,
			Format:     "pbf",
			MinZoom:    minZ,
			MaxZoom:    maxZ,
			TileCount:  vRes.TileCount,
			TilePaths:  cachedPaths,
			GeoJSONRel: gjRel,
		}
		mData, _ := json.Marshal(mDoc)
		_ = os.WriteFile(manifestPath, mData, 0644)
	}

	return vRes, 0, nil
}

// renderRasterXYZTiles generates 256x256 XYZ PNG or JPG tiles with a controlled worker pool,
// smart bounding box filtering, and transparent/empty tile detection.
func (s *ExportService) renderRasterXYZTiles(ctx context.Context, imagePath string, bounds []float64, minZoom, maxZoom int, format string, quality int, targetDir string) (int, int) {
	if len(bounds) < 4 || !fileExists(imagePath) {
		return 0, 0
	}

	f, err := os.Open(imagePath)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	srcImg, _, err := image.Decode(f)
	if err != nil {
		return 0, 0
	}

	srcBounds := srcImg.Bounds()
	imgW := srcBounds.Dx()
	imgH := srcBounds.Dy()
	if imgW <= 0 || imgH <= 0 {
		return 0, 0
	}

	west, south, east, north := bounds[0], bounds[1], bounds[2], bounds[3]
	if west > east {
		west, east = east, west
	}
	if south > north {
		south, north = north, south
	}

	clampMinZ, clampMaxZ := gis.DetermineTileZooms(bounds, imgW, imgH)
	if minZoom > clampMinZ {
		clampMinZ = minZoom
	}
	if maxZoom < clampMaxZ {
		clampMaxZ = maxZoom
	}
	if clampMinZ > clampMaxZ {
		clampMinZ = clampMaxZ
	}

	type tileJob struct {
		x, y, z  int
		tPixelX0 int
		tPixelY0 int
		tPixelX1 int
		tPixelY1 int
		sPixelX0 int
		sPixelY0 int
		sPixelX1 int
		sPixelY1 int
	}

	var jobs []tileJob
	for z := clampMinZ; z <= clampMaxZ; z++ {
		minX, maxY := gis.LatLngToTile(south, west, z)
		maxX, minY := gis.LatLngToTile(north, east, z)

		if minX > maxX {
			minX, maxX = maxX, minX
		}
		if minY > maxY {
			minY, maxY = maxY, minY
		}

		for x := minX; x <= maxX; x++ {
			for y := minY; y <= maxY; y++ {
				tileBbox := gis.TileToLatLngBounds(x, y, z)
				tWest, tSouth, tEast, tNorth := tileBbox[0], tileBbox[1], tileBbox[2], tileBbox[3]

				interWest := math.Max(west, tWest)
				interEast := math.Min(east, tEast)
				interSouth := math.Max(south, tSouth)
				interNorth := math.Min(north, tNorth)

				if interWest >= interEast || interSouth >= interNorth {
					continue
				}

				tilePixelX0 := int(math.Floor((interWest - tWest) / (tEast - tWest) * 256.0))
				tilePixelX1 := int(math.Ceil((interEast - tWest) / (tEast - tWest) * 256.0))
				tilePixelY0 := int(math.Floor((tNorth - interNorth) / (tNorth - tSouth) * 256.0))
				tilePixelY1 := int(math.Ceil((tNorth - interSouth) / (tNorth - tSouth) * 256.0))

				tilePixelX0 = clampInt(tilePixelX0, 0, 255)
				tilePixelX1 = clampInt(tilePixelX1, 1, 256)
				tilePixelY0 = clampInt(tilePixelY0, 0, 255)
				tilePixelY1 = clampInt(tilePixelY1, 1, 256)

				srcPixelX0 := int(math.Floor((interWest - west) / (east - west) * float64(imgW)))
				srcPixelX1 := int(math.Ceil((interEast - west) / (east - west) * float64(imgW)))
				srcPixelY0 := int(math.Floor((north - interNorth) / (north - south) * float64(imgH)))
				srcPixelY1 := int(math.Ceil((north - interSouth) / (north - south) * float64(imgH)))

				srcPixelX0 = clampInt(srcPixelX0, 0, imgW-1)
				srcPixelX1 = clampInt(srcPixelX1, 1, imgW)
				srcPixelY0 = clampInt(srcPixelY0, 0, imgH-1)
				srcPixelY1 = clampInt(srcPixelY1, 1, imgH)

				if srcPixelX1 <= srcPixelX0 || srcPixelY1 <= srcPixelY0 || tilePixelX1 <= tilePixelX0 || tilePixelY1 <= tilePixelY0 {
					continue
				}

				jobs = append(jobs, tileJob{
					x: x, y: y, z: z,
					tPixelX0: tilePixelX0, tPixelY0: tilePixelY0,
					tPixelX1: tilePixelX1, tPixelY1: tilePixelY1,
					sPixelX0: srcPixelX0, sPixelY0: srcPixelY0,
					sPixelX1: srcPixelX1, sPixelY1: srcPixelY1,
				})
			}
		}
	}

	if len(jobs) == 0 {
		return 0, 0
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > len(jobs) {
		numWorkers = len(jobs)
	}

	jobChan := make(chan tileJob, len(jobs))
	for _, j := range jobs {
		jobChan <- j
	}
	close(jobChan)

	var completedCount int64
	var emptySkippedCount int64
	var wg sync.WaitGroup

	if quality <= 0 || quality > 100 {
		quality = 85
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tileImg := image.NewRGBA(image.Rect(0, 0, 256, 256))

			for j := range jobChan {
				if ctx != nil && ctx.Err() != nil {
					return
				}

				for i := range tileImg.Pix {
					tileImg.Pix[i] = 0
				}

				rescaleSubImageFast(tileImg, j.tPixelX0, j.tPixelY0, j.tPixelX1, j.tPixelY1,
					srcImg, j.sPixelX0, j.sPixelY0, j.sPixelX1, j.sPixelY1)

				if isTileEmptyOrTransparent(tileImg) {
					atomic.AddInt64(&emptySkippedCount, 1)
					continue
				}

				tileOutDir := filepath.Join(targetDir, fmt.Sprintf("%d", j.z), fmt.Sprintf("%d", j.x))
				_ = os.MkdirAll(tileOutDir, 0755)
				tileOutPath := filepath.Join(tileOutDir, fmt.Sprintf("%d.%s", j.y, format))

				outFile, oErr := os.Create(tileOutPath)
				if oErr == nil {
					if format == "jpg" {
						_ = jpeg.Encode(outFile, tileImg, &jpeg.Options{Quality: quality})
					} else {
						_ = png.Encode(outFile, tileImg)
					}
					_ = outFile.Close()
					atomic.AddInt64(&completedCount, 1)
				}
			}
		}()
	}

	wg.Wait()
	return int(completedCount), int(emptySkippedCount)
}

type pythonVectorTileResult struct {
	Success      bool   `json:"success"`
	LayerName    string `json:"layer_name"`
	FeatureCount int    `json:"feature_count"`
	TileCount    int    `json:"tile_count"`
	GeometryType string `json:"geometry_type,omitempty"`
	GeoJSONFile  string `json:"geojson_file,omitempty"`
	MinZoom      int    `json:"min_zoom"`
	MaxZoom      int    `json:"max_zoom"`
	OutDir       string `json:"out_dir"`
	Error        string `json:"error,omitempty"`
}

func (s *ExportService) generateVectorTilesWithPython(ctx context.Context, srcPath, outDir, layerName string, minZoom, maxZoom int) (*pythonVectorTileResult, error) {
	pythonExe := "python"
	candidates := []string{
		"backend/.venv/Scripts/python.exe",
		".venv/Scripts/python.exe",
		"../backend/.venv/Scripts/python.exe",
	}
	for _, c := range candidates {
		if fileExists(c) {
			pythonExe = c
			break
		}
	}

	scriptPath := "backend/scripts/vector_tool.py"
	if !fileExists(scriptPath) {
		candidatesScript := []string{
			"scripts/vector_tool.py",
			"../backend/scripts/vector_tool.py",
		}
		for _, cs := range candidatesScript {
			if fileExists(cs) {
				scriptPath = cs
				break
			}
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, pythonExe, scriptPath, "generate_vector_tiles", srcPath, outDir, layerName, fmt.Sprintf("%d", minZoom), fmt.Sprintf("%d", maxZoom))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("vector tile execution failed: %v: %s", err, stderr.String())
	}

	var res pythonVectorTileResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("failed decoding vector tile output: %v", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("vector tile generator reported error: %s", res.Error)
	}
	return &res, nil
}

func clampInt(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func rescaleSubImageFast(dst *image.RGBA, dx0, dy0, dx1, dy1 int, src image.Image, sx0, sy0, sx1, sy1 int) {
	dw := dx1 - dx0
	dh := dy1 - dy0
	sw := sx1 - sx0
	sh := sy1 - sy0
	if dw <= 0 || dh <= 0 || sw <= 0 || sh <= 0 {
		return
	}

	for y := 0; y < dh; y++ {
		srcY := sy0 + (y*sh)/dh
		for x := 0; x < dw; x++ {
			srcX := sx0 + (x*sw)/dw
			dst.Set(dx0+x, dy0+y, src.At(srcX, srcY))
		}
	}
}

func buildControlPointsGeoJSON(cps []*models.ControlPoint) map[string]interface{} {
	features := make([]map[string]interface{}, 0, len(cps))
	for _, cp := range cps {
		if cp == nil {
			continue
		}
		f := map[string]interface{}{
			"type": "Feature",
			"id":   cp.ID,
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{cp.Longitude, cp.Latitude},
			},
			"properties": map[string]interface{}{
				"id":                    cp.ID,
				"point_id":              cp.ID,
				"image_x":               cp.ImageX,
				"image_y":               cp.ImageY,
				"longitude":             cp.Longitude,
				"latitude":              cp.Latitude,
				"crs":                   "EPSG:4326",
				"associated_raster":     cp.DatasetID,
				"dataset_id":            cp.DatasetID,
				"georeferencing_status": "completed",
			},
		}
		features = append(features, f)
	}
	return map[string]interface{}{
		"type":     "FeatureCollection",
		"features": features,
	}
}

func buildIconPointsGeoJSON(ips []*models.IconPoint) map[string]interface{} {
	features := make([]map[string]interface{}, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		props := map[string]interface{}{
			"id":                ip.ID,
			"point_id":          ip.ID,
			"name":              ip.Name,
			"description":       ip.Description,
			"category":          ip.Scope,
			"icon_id":           ip.IconID,
			"icon_asset":        fmt.Sprintf("./assets/icons/%s", filepath.Base(ip.IconPath)),
			"longitude":         ip.Longitude,
			"latitude":          ip.Latitude,
			"associated_raster": ip.DatasetID,
			"building_id":       ip.BuildingID,
			"floor_level":       ip.FloorLevel,
			"floor_label":       ip.FloorLabel,
		}
		if ip.Properties != nil {
			for k, v := range ip.Properties {
				props[k] = v
			}
		}
		f := map[string]interface{}{
			"type": "Feature",
			"id":   ip.ID,
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{ip.Longitude, ip.Latitude},
			},
			"properties": props,
		}
		features = append(features, f)
	}
	return map[string]interface{}{
		"type":     "FeatureCollection",
		"features": features,
	}
}

func buildLandmarksGeoJSON(ips []*models.IconPoint) map[string]interface{} {
	var lmFeatures []map[string]interface{}
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		isLandmark := strings.EqualFold(ip.Scope, "landmark") || strings.Contains(strings.ToLower(ip.Name), "landmark")
		if !isLandmark {
			continue
		}
		lmFeatures = append(lmFeatures, map[string]interface{}{
			"type": "Feature",
			"id":   ip.ID,
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{ip.Longitude, ip.Latitude},
			},
			"properties": map[string]interface{}{
				"id":          ip.ID,
				"name":        ip.Name,
				"description": ip.Description,
				"category":    "landmark",
			},
		})
	}
	return map[string]interface{}{
		"type":     "FeatureCollection",
		"features": lmFeatures,
	}
}

func buildAreasGeoJSON(primaryDs *models.Dataset, dsMap map[string]*models.Dataset, selectedSet map[string]bool) map[string]interface{} {
	var areaFeatures []map[string]interface{}

	extractAreasFromMeta := func(meta map[string]interface{}) {
		if meta == nil {
			return
		}
		if rawAreas, ok := meta["areas"].([]interface{}); ok {
			for _, ra := range rawAreas {
				if am, ok := ra.(map[string]interface{}); ok {
					areaFeatures = append(areaFeatures, am)
				}
			}
		}
		if rawDrawings, ok := meta["drawings"].([]interface{}); ok {
			for _, rd := range rawDrawings {
				if dm, ok := rd.(map[string]interface{}); ok {
					areaFeatures = append(areaFeatures, dm)
				}
			}
		}
	}

	if primaryDs != nil {
		extractAreasFromMeta(primaryDs.ExtractedMetadata)
	}
	for id := range selectedSet {
		if ds := dsMap[id]; ds != nil && ds != primaryDs {
			extractAreasFromMeta(ds.ExtractedMetadata)
		}
	}

	return map[string]interface{}{
		"type":     "FeatureCollection",
		"features": areaFeatures,
	}
}

func buildGeoreferencingMetadataDoc(rasters []*models.Dataset, cps []*models.ControlPoint) map[string]interface{} {
	type georefEntry struct {
		DatasetID string    `json:"dataset_id"`
		Filename  string    `json:"filename"`
		CRS       string    `json:"crs"`
		Bounds    []float64 `json:"bounds"`
		Transform []float64 `json:"transform,omitempty"`
		RMSE      float64   `json:"rmse,omitempty"`
		Points    int       `json:"control_points_count"`
	}

	var entries []georefEntry
	for _, ds := range rasters {
		if ds == nil {
			continue
		}
		tCount := 0
		for _, cp := range cps {
			if cp != nil && cp.DatasetID == ds.ID {
				tCount++
			}
		}
		entries = append(entries, georefEntry{
			DatasetID: ds.ID,
			Filename:  ds.Filename,
			CRS:       "EPSG:4326",
			Bounds:    extractDatasetBounds(ds),
			Points:    tCount,
		})
	}

	return map[string]interface{}{
		"georeferenced_layers": entries,
		"crs_reference":        "WGS 84 / Pseudo-Mercator",
		"units":                "degrees_wgs84",
	}
}

func buildMapLibreStyleDoc(name string, bounds, center []float64, initZoom int, rasters []TileConfigRasterLayer, vectors []TileConfigVectorLayer) map[string]interface{} {
	sources := make(map[string]interface{})
	layers := make([]map[string]interface{}, 0)

	// Basemap source
	sources["osm_basemap"] = map[string]interface{}{
		"type":        "raster",
		"tiles":       []string{"https://tile.openstreetmap.org/{z}/{x}/{y}.png"},
		"tileSize":    256,
		"attribution": "© OpenStreetMap contributors",
	}
	layers = append(layers, map[string]interface{}{
		"id":     "osm_basemap_layer",
		"type":   "raster",
		"source": "osm_basemap",
		"paint":  map[string]interface{}{"raster-opacity": 0.8},
	})

	// Raster layer sources and layers
	for _, r := range rasters {
		srcID := "raster_source_" + r.ID
		sources[srcID] = map[string]interface{}{
			"type":     "raster",
			"tiles":    []string{r.TileURL},
			"tileSize": 256,
			"bounds":   r.Bounds,
			"minzoom":  r.MinZoom,
			"maxzoom":  r.MaxZoom,
		}
		layers = append(layers, map[string]interface{}{
			"id":     "raster_layer_" + r.ID,
			"type":   "raster",
			"source": srcID,
			"paint":  map[string]interface{}{"raster-opacity": r.Opacity},
		})
	}

	// Vector layer sources and layers
	for _, v := range vectors {
		srcID := "vector_source_" + v.ID
		vecSrcMaxZ := v.MaxZoom
		if vecSrcMaxZ > 14 {
			vecSrcMaxZ = 14
		}
		sources[srcID] = map[string]interface{}{
			"type":    "vector",
			"tiles":   []string{v.TileURL},
			"minzoom": v.MinZoom,
			"maxzoom": vecSrcMaxZ,
		}
		if v.GeoJSONURL != "" {
			sources["vector_geojson_"+v.ID] = map[string]interface{}{
				"type": "geojson",
				"data": v.GeoJSONURL,
			}
		}
		layers = append(layers, map[string]interface{}{
			"id":           "vector_poly_" + v.ID,
			"type":         "fill",
			"source":       srcID,
			"source-layer": v.ID,
			"filter":       []interface{}{"==", "$type", "Polygon"},
			"paint": map[string]interface{}{
				"fill-color":   "#3b82f6",
				"fill-opacity": 0.4,
			},
		})
		layers = append(layers, map[string]interface{}{
			"id":           "vector_line_" + v.ID,
			"type":         "line",
			"source":       srcID,
			"source-layer": v.ID,
			"filter":       []interface{}{"==", "$type", "LineString"},
			"paint": map[string]interface{}{
				"line-color": "#2563eb",
				"line-width": 2,
			},
		})
		layers = append(layers, map[string]interface{}{
			"id":           "vector_pt_" + v.ID,
			"type":         "circle",
			"source":       srcID,
			"source-layer": v.ID,
			"filter":       []interface{}{"==", "$type", "Point"},
			"paint": map[string]interface{}{
				"circle-radius": 5,
				"circle-color":  "#ef4444",
			},
		})
	}

	// GeoJSON point and area sources
	sources["areas_geojson"] = map[string]interface{}{
		"type": "geojson",
		"data": "./geojson/areas.geojson",
	}
	layers = append(layers, map[string]interface{}{
		"id":     "areas_fill",
		"type":   "fill",
		"source": "areas_geojson",
		"paint": map[string]interface{}{
			"fill-color":   "#10b981",
			"fill-opacity": 0.3,
		},
	})
	layers = append(layers, map[string]interface{}{
		"id":     "areas_line",
		"type":   "line",
		"source": "areas_geojson",
		"paint": map[string]interface{}{
			"line-color": "#059669",
			"line-width": 2,
		},
	})

	sources["control_points_geojson"] = map[string]interface{}{
		"type": "geojson",
		"data": "./geojson/control_points.geojson",
	}
	layers = append(layers, map[string]interface{}{
		"id":     "control_points_circle",
		"type":   "circle",
		"source": "control_points_geojson",
		"paint": map[string]interface{}{
			"circle-radius":       6,
			"circle-color":        "#f59e0b",
			"circle-stroke-color": "#ffffff",
			"circle-stroke-width": 2,
		},
	})

	sources["icon_points_geojson"] = map[string]interface{}{
		"type": "geojson",
		"data": "./geojson/icon_points.geojson",
	}
	layers = append(layers, map[string]interface{}{
		"id":     "icon_points_circle",
		"type":   "circle",
		"source": "icon_points_geojson",
		"paint": map[string]interface{}{
			"circle-radius":       8,
			"circle-color":        "#6366f1",
			"circle-stroke-color": "#ffffff",
			"circle-stroke-width": 2,
		},
	})

	doc := map[string]interface{}{
		"version": 8,
		"name":    name,
		"center":  center,
		"zoom":    initZoom,
		"sources": sources,
		"layers":  layers,
	}
	if len(bounds) >= 4 {
		doc["metadata"] = map[string]interface{}{
			"bounds": bounds,
		}
	}

	return doc
}
