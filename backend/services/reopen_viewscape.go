package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

// findViewscapeManifestRel returns the relative archive path to Viewscape/manifest.json (or root manifest.json
// with format starting with "viewscape_") if present in relFiles.
func findViewscapeManifestRel(extractDir string, relFiles []string) string {
	for _, rf := range relFiles {
		lower := strings.ToLower(filepath.ToSlash(rf))
		if lower == "viewscape/manifest.json" {
			return rf
		}
	}
	for _, rf := range relFiles {
		lower := strings.ToLower(filepath.ToSlash(rf))
		if lower == "manifest.json" {
			raw, err := os.ReadFile(filepath.Join(extractDir, filepath.FromSlash(rf)))
			if err == nil {
				var probe struct {
					Format      string `json:"format"`
					PackageType string `json:"package_type"`
				}
				if json.Unmarshal(raw, &probe) == nil {
					if strings.HasPrefix(strings.ToLower(probe.Format), "viewscape") ||
						strings.HasPrefix(strings.ToLower(probe.PackageType), "viewscape") {
						return rf
					}
				}
			}
		}
	}
	return ""
}

// inspectViewscapePackage builds a ProjectPackageInspection preview for a Viewscape ZIP archive.
func inspectViewscapePackage(extractDir, manifestRel, filename string, written int64) (*models.ProjectPackageInspection, error) {
	raw, err := os.ReadFile(filepath.Join(extractDir, filepath.FromSlash(manifestRel)))
	if err != nil {
		return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: failed to read Viewscape manifest.json: %w", err)
	}
	var man ViewscapeManifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: invalid Viewscape manifest.json: %w", err)
	}
	projName := man.ProjectName
	if projName == "" {
		projName = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	previews := make([]*models.ProjectLayerPreview, 0, len(man.RasterLayers)+len(man.VectorLayers))
	totalTiles := 0
	for _, r := range man.RasterLayers {
		totalTiles += r.TileCount
		previews = append(previews, &models.ProjectLayerPreview{
			ID:           r.Slug,
			Name:         r.Name,
			Type:         "raster",
			TileFormat:   r.Format,
			FloorLevel:   r.FloorLevel,
			FloorLabel:   r.FloorLabel,
			BuildingName: r.BuildingName,
			Visible:      r.Visible,
			Opacity:      r.Opacity,
			MinZoom:      r.MinZoom,
			MaxZoom:      r.MaxZoom,
			Bounds:       r.Bounds,
			HasMBTiles:   false,
		})
	}
	for _, v := range man.VectorLayers {
		totalTiles += v.TileCount
		previews = append(previews, &models.ProjectLayerPreview{
			ID:         v.Slug,
			Name:       v.Name,
			Type:       "vector",
			TileFormat: "pbf",
			Visible:    v.Visible,
			Opacity:    v.Opacity,
			MinZoom:    v.MinZoom,
			MaxZoom:    v.MaxZoom,
			Bounds:     v.Bounds,
			HasVectors: true,
		})
	}

	return &models.ProjectPackageInspection{
		Valid:         true,
		PackageType:   man.PackageType,
		Filename:      filename,
		FileSize:      written,
		ProjectName:   projName,
		SchemaVersion: 1,
		Bounds:        man.Bounds,
		Center:        man.Center,
		InitialZoom:   float64(man.InitialZoom),
		MinZoom:       man.MinZoom,
		MaxZoom:       man.MaxZoom,
		TileFormat:    "hybrid",
		TileScheme:    "xyz",
		TileCount:     totalTiles,
		HasBaseLayer:  len(man.BaseLayers) > 0,
		BuildingCount: len(man.Buildings),
		LayerCount:    len(previews),
		Layers:        previews,
	}, nil
}

// ReopenViewscapeFromPath restores an exported Viewscape ZIP (complete, raster-only, vector-only, or base-only)
// without requiring the original uploaded GeoPackage or temporary upload URLs.
func (s *DatasetService) ReopenViewscapeFromPath(
	sessionID, primaryDatasetID, primaryDatasetDir, extractDir, zipPath, filename string,
	size int64, fileHash, manifestRel string,
) (*models.Dataset, error) {
	manifestDisk := filepath.Join(extractDir, filepath.FromSlash(manifestRel))
	rawMan, err := os.ReadFile(manifestDisk)
	if err != nil {
		_ = storage.SafeRemoveAll(primaryDatasetDir)
		return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: failed reading Viewscape manifest.json: %w", err)
	}
	var man ViewscapeManifest
	if err := json.Unmarshal(rawMan, &man); err != nil {
		_ = storage.SafeRemoveAll(primaryDatasetDir)
		return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: invalid Viewscape manifest.json: %w", err)
	}

	vsRoot := filepath.Dir(manifestDisk)
	projName := strings.TrimSpace(man.ProjectName)
	if projName == "" {
		projName = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	// Read optional project/viewscape_config.json
	var vsConfig map[string]interface{}
	if cfgBytes, cErr := os.ReadFile(filepath.Join(vsRoot, "project", "viewscape_config.json")); cErr == nil {
		_ = json.Unmarshal(cfgBytes, &vsConfig)
	}

	// Read optional base_layers/sources.json
	var baseSources []ViewscapeBaseEntry
	if srcBytes, sErr := os.ReadFile(filepath.Join(vsRoot, "base_layers", "sources.json")); sErr == nil {
		var srcDoc struct {
			Sources []ViewscapeBaseEntry `json:"sources"`
		}
		if json.Unmarshal(srcBytes, &srcDoc) == nil && len(srcDoc.Sources) > 0 {
			baseSources = srcDoc.Sources
		}
	}
	if len(baseSources) == 0 {
		baseSources = man.BaseLayers
	}

	// 1. Restore Raster Layers -> <primaryDatasetDir>/tiles_xyz/raster/<slug>/{z}/{x}/{y}.ext
	xyzDir := filepath.Join(primaryDatasetDir, "tiles_xyz")
	_ = os.MkdirAll(xyzDir, 0755)
	var previewSampleTile string

	sortedRasters := append([]ViewscapeRasterEntry(nil), man.RasterLayers...)
	sort.Slice(sortedRasters, func(i, j int) bool {
		return sortedRasters[i].DisplayOrder < sortedRasters[j].DisplayOrder
	})

	for idx, rEntry := range sortedRasters {
		srcTiles := filepath.Join(vsRoot, "raster_layers", rEntry.Slug, "tiles")
		if !dirExists(srcTiles) {
			continue
		}
		_ = filepath.Walk(srcTiles, func(p string, info os.FileInfo, wErr error) error {
			if wErr != nil || info.IsDir() {
				return nil
			}
			rel, rErr := filepath.Rel(srcTiles, p)
			if rErr != nil {
				return nil
			}
			// Copy under tiles_xyz/raster/<slug>/
			dstNested := filepath.Join(xyzDir, "raster", rEntry.Slug, rel)
			_ = os.MkdirAll(filepath.Dir(dstNested), 0755)
			_ = storage.CopyFile(p, dstNested)

			// First raster also populates root tiles_xyz/{z}/{x}/{y}.ext for direct ServeTile
			if idx == 0 {
				dstRoot := filepath.Join(xyzDir, rel)
				_ = os.MkdirAll(filepath.Dir(dstRoot), 0755)
				_ = storage.CopyFile(p, dstRoot)
				if previewSampleTile == "" {
					previewSampleTile = p
				}
			}
			return nil
		})
	}

	// Copy preview tile for display/thumbnail
	processedDir := filepath.Join(primaryDatasetDir, "processed")
	_ = os.MkdirAll(processedDir, 0755)
	previewPath := filepath.Join(processedDir, "mbtiles_preview.png")
	if previewSampleTile != "" && fileExists(previewSampleTile) {
		_ = storage.CopyFile(previewSampleTile, previewPath)
		_ = storage.CopyFile(previewSampleTile, filepath.Join(processedDir, "display.png"))
	}

	// 2. Restore Vector Tile Layers -> <primaryDatasetDir>/vector_tiles/<slug>/{metadata.json,style.json,tiles/z/x/y.pbf}
	vtRoot := filepath.Join(primaryDatasetDir, "vector_tiles")
	_ = os.MkdirAll(vtRoot, 0755)
	restoredVectorMeta := make(map[string]interface{})

	sortedVectors := append([]ViewscapeVectorEntry(nil), man.VectorLayers...)
	sort.Slice(sortedVectors, func(i, j int) bool {
		return sortedVectors[i].DisplayOrder < sortedVectors[j].DisplayOrder
	})

	for idx, vEntry := range sortedVectors {
		srcVecDir := filepath.Join(vsRoot, "vector_layers", vEntry.Slug)
		if !dirExists(srcVecDir) {
			continue
		}
		dstVecDir := filepath.Join(vtRoot, vEntry.Slug)
		_ = os.MkdirAll(dstVecDir, 0755)

		copiedTiles := 0
		_ = filepath.Walk(srcVecDir, func(p string, info os.FileInfo, wErr error) error {
			if wErr != nil || info.IsDir() {
				return nil
			}
			rel, rErr := filepath.Rel(srcVecDir, p)
			if rErr != nil {
				return nil
			}
			dst := filepath.Join(dstVecDir, rel)
			_ = os.MkdirAll(filepath.Dir(dst), 0755)
			if storage.CopyFile(p, dst) == nil {
				ext := strings.ToLower(filepath.Ext(p))
				if ext == ".pbf" || ext == ".mvt" {
					copiedTiles++
				}
			}
			return nil
		})

		sourceLayer := vEntry.SourceLayer
		if sourceLayer == "" {
			sourceLayer = vEntry.Slug
		}
		promoteID := vEntry.PromoteID
		if promoteID == "" {
			promoteID = "feature_id"
		}
		tileCount := vEntry.TileCount
		if copiedTiles > 0 {
			tileCount = copiedTiles
		}

		vtMeta := map[string]interface{}{
			"type":              "vector",
			"format":            "pbf",
			"encoding":          "gzip",
			"tile_scheme":       "xyz",
			"layer_slug":        vEntry.Slug,
			"layer_name":        vEntry.Name,
			"source_layer":      sourceLayer,
			"dataset_id":        primaryDatasetID,
			"source_crs":        firstNonEmptyBase(vEntry.SourceCRS, "EPSG:4326"),
			"tile_crs":          "EPSG:3857",
			"geometry_type":     firstNonEmptyBase(vEntry.GeometryType, "Geometry"),
			"geometry_types":    vEntry.GeometryTypes,
			"feature_count":     vEntry.FeatureCount,
			"tile_count":        tileCount,
			"bounds":            vEntry.Bounds,
			"min_zoom":          vEntry.MinZoom,
			"max_zoom":          vEntry.MaxZoom,
			"fields":            vEntry.Fields,
			"layer_dir":         dstVecDir,
			"tiles_dir":         filepath.Join(dstVecDir, "tiles"),
			"metadata_path":     filepath.Join(dstVecDir, "metadata.json"),
			"style_path":        filepath.Join(dstVecDir, "style.json"),
			"tile_url_template": fmt.Sprintf("/datasets/%s/vector-tiles/%s/{z}/{x}/{y}.pbf", primaryDatasetID, vEntry.Slug),
			"metadata_url":      fmt.Sprintf("/datasets/%s/vector-tiles/%s/metadata.json", primaryDatasetID, vEntry.Slug),
			"style_url":         fmt.Sprintf("/datasets/%s/vector-tiles/%s/style.json", primaryDatasetID, vEntry.Slug),
			"promote_id":        promoteID,
			"interactive":       true,
		}
		restoredVectorMeta[vEntry.Slug] = vtMeta

		// Register restored vector tile layer in GISService so MapLibre renders it immediately with full interaction
		if s.gisService != nil {
			vis := vEntry.Visible
			op := vEntry.Opacity
			if op <= 0 {
				op = 1.0
			}
			order := vEntry.DisplayOrder
			if order == 0 {
				order = idx
			}
			minZ := float64(vEntry.MinZoom)
			maxZ := 24.0
			createdLyr, cErr := s.gisService.CreateLayer(context.Background(), "", models.CreateLayerRequest{
				MapID:        primaryDatasetID,
				DatasetID:    primaryDatasetID,
				LayerName:    firstNonEmptyBase(vEntry.Name, vEntry.Slug),
				LayerType:    "vector",
				GeometryType: firstNonEmptyBase(vEntry.GeometryType, "Geometry"),
				DisplayOrder: &order,
				IsVisible:    &vis,
				Opacity:      &op,
				MinZoom:      &minZ,
				MaxZoom:      &maxZ,
				CRS:          firstNonEmptyBase(vEntry.SourceCRS, "EPSG:4326"),
				Bounds:       vEntry.Bounds,
				Metadata: map[string]interface{}{
					"source_dataset_id":  primaryDatasetID,
					"source_layer_name":  vEntry.Name,
					"source_crs":         firstNonEmptyBase(vEntry.SourceCRS, "EPSG:4326"),
					"reopened_viewscape": true,
					"vector_tiles":       vtMeta,
				},
			})
			if cErr == nil && createdLyr != nil {
				// Restore feature count and style
				if s.gisService.GetGISRepo() != nil {
					createdLyr.FeatureCount = vEntry.FeatureCount
				}
				// Restore styling from style.json
				var styleDoc map[string]interface{}
				if rawS, sErr := os.ReadFile(filepath.Join(dstVecDir, "style.json")); sErr == nil {
					_ = json.Unmarshal(rawS, &styleDoc)
				}
				color := "#3ecf8e"
				if styleDoc != nil {
					if c, _ := styleDoc["color"].(string); c != "" {
						color = c
					}
					if wsStyle, ok := styleDoc["workspace_style"].(map[string]interface{}); ok {
						if sc, ok2 := wsStyle["style_config"].(map[string]interface{}); ok2 {
							_, _ = s.gisService.UpdateLayerStyle(context.Background(), createdLyr.LayerID, "", models.UpdateStyleRequest{
								StyleType:   "single",
								StyleConfig: sc,
							})
							continue
						}
					}
				}
				_, _ = s.gisService.UpdateLayerStyle(context.Background(), createdLyr.LayerID, "", models.UpdateStyleRequest{
					StyleType:   "single",
					StyleConfig: defaultStyleForGeometry(vEntry.GeometryType, color),
				})
			}
		}
	}

	// 3. Restore Base Layers (local_tiles + sources.json)
	srcLocalBase := filepath.Join(vsRoot, "base_layers", "local_tiles")
	dstLocalBase := filepath.Join(primaryDatasetDir, "base_layers", "local_tiles")
	if dirExists(srcLocalBase) {
		_ = os.MkdirAll(dstLocalBase, 0755)
		_ = filepath.Walk(srcLocalBase, func(p string, info os.FileInfo, wErr error) error {
			if wErr != nil || info.IsDir() || filepath.Base(p) == ".keep" {
				return nil
			}
			rel, rErr := filepath.Rel(srcLocalBase, p)
			if rErr == nil {
				dst := filepath.Join(dstLocalBase, rel)
				_ = os.MkdirAll(filepath.Dir(dst), 0755)
				_ = storage.CopyFile(p, dst)
			}
			return nil
		})
	}

	// 4. Restore Control Points & Icon Points from viewscape_config.json if present
	if vsConfig != nil {
		if rawCPs, ok := vsConfig["control_points"]; ok && rawCPs != nil {
			if b, mErr := json.Marshal(rawCPs); mErr == nil {
				var cps []*models.ControlPoint
				if json.Unmarshal(b, &cps) == nil && len(cps) > 0 {
					for _, cp := range cps {
						if cp != nil {
							cp.DatasetID = primaryDatasetID
						}
					}
					_ = s.repo.SetControlPoints(sessionID, primaryDatasetID, cps)
				}
			}
		}
		if rawIPs, ok := vsConfig["icon_points"]; ok && rawIPs != nil {
			if b, mErr := json.Marshal(rawIPs); mErr == nil {
				var ips []*models.IconPoint
				if json.Unmarshal(b, &ips) == nil {
					for _, ip := range ips {
						if ip != nil {
							ip.ID = ""
							ip.DatasetID = primaryDatasetID
							_, _ = s.repo.CreateIconPoint(sessionID, ip)
						}
					}
				}
			}
		}
	}

	// 5. Restore Bounds, Center, Zoom, and Register Primary Dataset + MapProject
	bounds := man.Bounds
	if len(bounds) != 4 {
		bounds = []float64{-180, -85, 180, 85}
	}
	center := man.Center
	if len(center) != 2 {
		center = []float64{(bounds[0] + bounds[2]) / 2.0, (bounds[1] + bounds[3]) / 2.0}
	}
	initZoom := man.InitialZoom
	if initZoom <= 0 {
		initZoom = 14
	}

	boundsMap := map[string]interface{}{
		"min_x": bounds[0],
		"min_y": bounds[1],
		"max_x": bounds[2],
		"max_y": bounds[3],
	}
	coords4 := [][]float64{
		{bounds[0], bounds[3]},
		{bounds[2], bounds[3]},
		{bounds[2], bounds[1]},
		{bounds[0], bounds[1]},
	}

	mapLayer := map[string]interface{}{
		"id":                "lyr_" + primaryDatasetID,
		"name":              projName,
		"type":              "custom_local_tiles",
		"tile_format":       "png",
		"scheme":            "xyz",
		"tile_url_template": fmt.Sprintf("/datasets/%s/tiles/{z}/{x}/{y}", primaryDatasetID),
		"tile_min_zoom":     man.MinZoom,
		"tile_max_zoom":     man.MaxZoom,
		"bounds":            boundsMap,
		"bounds_crs":        "EPSG:4326",
		"coordinates":       coords4,
	}

	extractedMeta := map[string]interface{}{
		"driver":             "Viewscape",
		"format":             man.Format,
		"document_type":      "viewscape",
		"crs":                "EPSG:4326",
		"tile_crs":           "EPSG:3857",
		"bounds":             boundsMap,
		"center":             center,
		"initial_zoom":       initZoom,
		"minzoom":            man.MinZoom,
		"maxzoom":            man.MaxZoom,
		"tile_min_zoom":      man.MinZoom,
		"tile_max_zoom":      man.MaxZoom,
		"tile_scheme":        "xyz",
		"layer_ordering":     man.LayerOrdering,
		"raster_layers":      sortedRasters,
		"vector_layers":      sortedVectors,
		"vector_tiles":       restoredVectorMeta,
		"vector_tiles_root":  vtRoot,
		"base_layers":        baseSources,
		"interaction_config": man.InteractionMode,
		"reopened_viewscape": true,
		"content": &models.ContentComposition{
			HasRaster: len(sortedRasters) > 0,
			HasVector: len(sortedVectors) > 0,
		},
	}

	dataset := &models.Dataset{
		ID:                 primaryDatasetID,
		SessionID:          sessionID,
		Filename:           filename,
		FileSize:           size,
		ContentType:        "application/zip",
		FileHash:           fileHash,
		StoragePath:        zipPath,
		ProcessedPath:      previewPath,
		GeoreferencedPath:  zipPath,
		TilePath:           xyzDir,
		GeoreferenceStatus: "completed",
		TileStatus:         "completed",
		ExtractedMetadata:  extractedMeta,
		MapLayer:           mapLayer,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
		_ = storage.SafeRemoveAll(primaryDatasetDir)
		return nil, err
	}

	_ = s.repo.SetLayers(sessionID, primaryDatasetID, []*models.Layer{
		{
			ID:        "lyr_" + primaryDatasetID,
			DatasetID: primaryDatasetID,
			Name:      projName,
			Type:      "viewscape",
			Visible:   true,
			Opacity:   1.0,
			Bounds:    bounds,
		},
	})

	primaryBaseCfg := map[string]interface{}{
		"type":    "xyz",
		"name":    "OpenStreetMap Standard",
		"visible": true,
		"opacity": 1.0,
	}
	if len(baseSources) > 0 {
		b0 := baseSources[0]
		primaryBaseCfg = map[string]interface{}{
			"id":                          b0.ID,
			"name":                        b0.Name,
			"source_type":                 b0.SourceType,
			"provider":                    b0.Provider,
			"url":                         b0.URL,
			"wms_layers":                  b0.WMSLayers,
			"attribution":                 b0.Attribution,
			"visible":                     b0.Visible,
			"opacity":                     b0.Opacity,
			"offline":                     b0.Offline,
			"mbtiles_path":                b0.MBTilesPath,
			"local_tiles_path":            b0.LocalTilesPath,
			"remote_tile_cache_permitted": b0.RemoteTileCachePermitted,
		}
	}

	rasterVisible := len(sortedRasters) > 0
	rasterOpacity := 1.0
	if len(sortedRasters) > 0 {
		rasterVisible = sortedRasters[0].Visible
		if sortedRasters[0].Opacity > 0 {
			rasterOpacity = sortedRasters[0].Opacity
		}
	}

	proj := &models.MapProject{
		ID:        "proj_" + repository.GenerateDatasetID(),
		DatasetID: primaryDatasetID,
		Name:      projName,
		Configuration: map[string]interface{}{
			"active_view":       "map",
			"active_dataset_id": primaryDatasetID,
			"min_zoom":          man.MinZoom,
			"max_zoom":          man.MaxZoom,
			"initial_zoom":      initZoom,
			"layer_ordering":    man.LayerOrdering,
			"camera": map[string]interface{}{
				"longitude": center[0],
				"latitude":  center[1],
				"center":    center,
				"zoom":      initZoom,
				"bounds":    bounds,
				"bearing":   0.0,
				"pitch":     0.0,
			},
			"base_layer":  primaryBaseCfg,
			"base_layers": baseSources,
			"interaction": man.InteractionMode,
			"members": []interface{}{
				map[string]interface{}{
					"dataset_id":             primaryDatasetID,
					"name":                   projName,
					"visible":                rasterVisible,
					"raster_visible":         rasterVisible,
					"raster_opacity":         rasterOpacity,
					"control_points_visible": true,
					"icon_points_visible":    true,
					"annotations_visible":    true,
					"vectors_visible":        true,
					"sort_order":             0,
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_ = s.repo.SaveMapProject(sessionID, proj)

	// Cleanup temporary extraction folder; all tiles & assets are now in primaryDatasetDir
	_ = storage.SafeRemoveAll(extractDir)
	return dataset, nil
}
