package services

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

// ZoomConfiguration controls initial, min, and max camera zoom levels and preset in exported Viewscape
type ZoomConfiguration struct {
	InitialZoom *int   `json:"initial_zoom,omitempty"`
	MinZoom     *int   `json:"min_zoom,omitempty"`
	MaxZoom     *int   `json:"max_zoom,omitempty"`
	Preset      string `json:"preset,omitempty"`
}

// SelectiveExportOptions controls what supporting assets and metadata are included
type SelectiveExportOptions struct {
	IncludeBuildingConfig   bool               `json:"include_building_configuration"`
	IncludeControlPoints    bool               `json:"include_control_points"`
	IncludeIcons            bool               `json:"include_icons"`
	IncludeLayerStyles      bool               `json:"include_layer_styles"`
	IncludeOriginalRasters  bool               `json:"include_original_rasters"`
	IncludeProcessedRasters bool               `json:"include_processed_rasters"`
	IncludeBaseMapSettings  bool               `json:"include_base_map_settings"`
	IncludeProjectMetadata  bool               `json:"include_project_metadata"`
	ZoomConfig              *ZoomConfiguration `json:"zoom_config,omitempty"`
}

// DefaultSelectiveExportOptions returns sensible defaults for selective export
func DefaultSelectiveExportOptions() *SelectiveExportOptions {
	return &SelectiveExportOptions{
		IncludeBuildingConfig:   true,
		IncludeControlPoints:    true,
		IncludeIcons:            true,
		IncludeLayerStyles:      true,
		IncludeOriginalRasters:  true,
		IncludeProcessedRasters: true,
		IncludeBaseMapSettings:  false,
		IncludeProjectMetadata:  true,
		ZoomConfig:              nil,
	}
}

// SelectiveExportParams specifies the requested scope and options
type SelectiveExportParams struct {
	Format      string
	ProjectName string
	BuildingIDs []string
	FloorIDs    []string
	LayerIDs    []string
	Options     *SelectiveExportOptions
}

// ExportManifest defines the top-level manifest.json schema
type ExportManifest struct {
	FormatVersion     string                  `json:"format_version"`
	FormatName        string                  `json:"format_name"`
	Generator         string                  `json:"generator"`
	ExportTimestamp   string                  `json:"export_timestamp"`
	ProjectName       string                  `json:"project_name"`
	PrimaryDatasetID  string                  `json:"primary_dataset_id"`
	SelectedBuildings []string                `json:"selected_buildings"`
	SelectedFloors    []string                `json:"selected_floors"`
	SelectedLayers    []string                `json:"selected_layers"`
	IncludedOptions   *SelectiveExportOptions `json:"included_options"`
	Resources         []string                `json:"resources"`
}

// GenerateSelectiveExport creates a structured, versioned project ZIP containing selected buildings, floors, and layers.
func (s *ExportService) GenerateSelectiveExport(sessionID, primaryDatasetID string, params SelectiveExportParams) (string, string, error) {
	primaryDs, err := s.repo.GetDataset(sessionID, primaryDatasetID)
	if err != nil {
		return "", "", fmt.Errorf("DATASET_NOT_FOUND: Primary dataset not found: %w", err)
	}

	exportDir := filepath.Join(s.storage.GetDatasetDir(sessionID, primaryDatasetID), "exports")
	_ = os.MkdirAll(exportDir, 0755)

	if params.Format == "base_layer" || params.Format == "basemap" {
		return s.exportBaseLayerFile(primaryDs, exportDir)
	}
	if params.Format == "mbtiles" {
		return s.exportMBTilesFile(primaryDs, exportDir)
	}

	opts := params.Options
	if opts == nil {
		opts = DefaultSelectiveExportOptions()
	}

	// Validate options constraints:
	// If neither original nor processed raster is selected, we cannot export valid raster layers
	if !opts.IncludeOriginalRasters && !opts.IncludeProcessedRasters {
		return "", "", fmt.Errorf("INVALID_OPTIONS: At least one raster image type (original or processed) must be included")
	}

	// 1. Resolve all buildings, floors, and datasets in the session
	allBuildings, _ := s.repo.ListBuildings(sessionID)
	allDatasets, _ := s.repo.ListDatasets(sessionID)

	datasetMap := make(map[string]*models.Dataset)
	for _, ds := range allDatasets {
		datasetMap[ds.ID] = ds
	}

	// Map dataset ID to Building and Floor
	type DatasetLocation struct {
		Building *models.Building
		Floor    *models.BuildingFloor
	}
	locationByDsID := make(map[string]DatasetLocation)
	for _, b := range allBuildings {
		for _, fl := range b.Floors {
			for _, dID := range fl.DatasetIDs {
				locationByDsID[dID] = DatasetLocation{Building: b, Floor: fl}
			}
		}
	}

	// Explicit selections
	selectedBuildingSet := make(map[string]bool)
	for _, bID := range params.BuildingIDs {
		if bID != "" {
			selectedBuildingSet[bID] = true
		}
	}

	selectedFloorSet := make(map[string]bool)
	for _, fID := range params.FloorIDs {
		if fID != "" {
			selectedFloorSet[fID] = true
		}
	}

	selectedLayerSet := make(map[string]bool)
	for _, lID := range params.LayerIDs {
		if lID != "" {
			selectedLayerSet[lID] = true
		}
	}

	// Hierarchy expansion:
	// Only expand from buildings/floors down to layers if NO explicit LayerIDs were specified.
	// If the user explicitly provided LayerIDs, we strictly export only those selected layers.
	if len(selectedLayerSet) == 0 {
		// If building is selected, select all its floors and layers
		for _, b := range allBuildings {
			if selectedBuildingSet[b.ID] {
				for _, fl := range b.Floors {
					selectedFloorSet[fl.ID] = true
					for _, dID := range fl.DatasetIDs {
						selectedLayerSet[dID] = true
					}
				}
			}
		}

		// If floor is selected, select all its layers
		for _, b := range allBuildings {
			for _, fl := range b.Floors {
				if selectedFloorSet[fl.ID] {
					for _, dID := range fl.DatasetIDs {
						selectedLayerSet[dID] = true
					}
				}
			}
		}
	}

	// If no explicit layers/floors/buildings selected, default to the primary dataset
	if len(selectedLayerSet) == 0 && len(selectedFloorSet) == 0 && len(selectedBuildingSet) == 0 {
		selectedLayerSet[primaryDatasetID] = true
	}

	// Verify that each selected layer actually exists
	var resolvedLayerIDs []string
	for lID := range selectedLayerSet {
		if ds, exists := datasetMap[lID]; exists && ds != nil {
			resolvedLayerIDs = append(resolvedLayerIDs, lID)
		}
	}
	sort.Strings(resolvedLayerIDs)

	if len(resolvedLayerIDs) == 0 {
		return "", "", fmt.Errorf("NO_LAYERS_SELECTED: At least one valid raster layer must be selected for export")
	}

	// Back-propagate selected layers to floors and buildings
	for _, lID := range resolvedLayerIDs {
		if loc, ok := locationByDsID[lID]; ok {
			if loc.Building != nil {
				selectedBuildingSet[loc.Building.ID] = true
			}
			if loc.Floor != nil {
				selectedFloorSet[loc.Floor.ID] = true
			}
		}
	}

	// Prepare export output path
	projectName := params.ProjectName
	if projectName == "" {
		if proj, _ := s.repo.GetMapProject(sessionID, primaryDatasetID); proj != nil && proj.Name != "" {
			projectName = proj.Name
		} else {
			projectName = primaryDs.Filename
		}
	}
	stem := safeStem(projectName)
	exportDir = filepath.Join(s.storage.GetDatasetDir(sessionID, primaryDatasetID), "exports")
	_ = os.MkdirAll(exportDir, 0755)
	zipPath := filepath.Join(exportDir, stem+".zip")

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating export archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	var archiveResources []string
	addFileToZip := func(archivePath string, r io.Reader) error {
		w, err := archive.Create(archivePath)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, r)
		if err == nil {
			archiveResources = append(archiveResources, archivePath)
		}
		return err
	}

	addJSONToZip := func(archivePath string, v interface{}) error {
		bytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		w, err := archive.Create(archivePath)
		if err != nil {
			return err
		}
		_, err = w.Write(bytes)
		if err == nil {
			archiveResources = append(archiveResources, archivePath)
		}
		return err
	}

	// Map of dataset ID to relative paths in archive
	type LayerAssetRefs struct {
		Original  string `json:"original,omitempty"`
		Processed string `json:"processed,omitempty"`
		WorldFile string `json:"world_file,omitempty"`
	}
	layerAssetRefs := make(map[string]LayerAssetRefs)

	// Collect icons to export
	var allIconPoints []*models.IconPoint
	if opts.IncludeIcons {
		for _, lID := range resolvedLayerIDs {
			pts, _ := s.repo.ListIconPoints(sessionID, lID, "")
			allIconPoints = append(allIconPoints, pts...)
		}
		// Also building-scoped icon points for selected buildings
		for bID := range selectedBuildingSet {
			pts, _ := s.repo.ListIconPoints(sessionID, "", bID)
			for _, p := range pts {
				already := false
				for _, existing := range allIconPoints {
					if existing.ID == p.ID {
						already = true
						break
					}
				}
				if !already {
					allIconPoints = append(allIconPoints, p)
				}
			}
		}
	}

	// Copy icons and icon assets
	iconAssetsCopied := make(map[string]string)
	if opts.IncludeIcons && len(allIconPoints) > 0 {
		iconAssetsCopied = bundleReferencedIcons(archive, allIconPoints, sessionID)
		for _, p := range iconAssetsCopied {
			archiveResources = append(archiveResources, p)
		}
		_ = addJSONToZip("icons/icon_points.json", allIconPoints)
	}

	// 2. Export each selected layer raster and sidecars
	for _, lID := range resolvedLayerIDs {
		ds := datasetMap[lID]
		if ds == nil {
			continue
		}

		var refs LayerAssetRefs
		layerDir := "layers/" + lID

		// Original raster
		if opts.IncludeOriginalRasters && ds.StoragePath != "" && fileExists(ds.StoragePath) {
			origName := filepath.Base(ds.StoragePath)
			origRel := layerDir + "/original_" + origName
			rasterRel := "rasters/" + lID + "_" + origName
			if f, err := os.Open(ds.StoragePath); err == nil {
				_ = addFileToZip(origRel, f)
				_ = f.Close()
				refs.Original = origRel
			}
			if rf, err := os.Open(ds.StoragePath); err == nil {
				_ = addFileToZip(rasterRel, rf)
				_ = rf.Close()
			}
		}

		// Processed transparent raster
		transparentPNG := filepath.Join(filepath.Dir(ds.StoragePath), "processed", "processed_transparent.png")
		if opts.IncludeProcessedRasters && fileExists(transparentPNG) {
			procRel := layerDir + "/processed_transparent.png"
			procRasterRel := "rasters/" + lID + "_processed_transparent.png"
			if f, err := os.Open(transparentPNG); err == nil {
				_ = addFileToZip(procRel, f)
				_ = f.Close()
				refs.Processed = procRel
			}
			if rf, err := os.Open(transparentPNG); err == nil {
				_ = addFileToZip(procRasterRel, rf)
				_ = rf.Close()
			}
		}

		// If processed is requested but not transparent, check if ProcessedPath exists
		if opts.IncludeProcessedRasters && refs.Processed == "" && ds.ProcessedPath != "" && fileExists(ds.ProcessedPath) {
			pBase := filepath.Base(ds.ProcessedPath)
			procRel := layerDir + "/processed_" + pBase
			procRasterRel := "rasters/" + lID + "_processed_" + pBase
			if f, err := os.Open(ds.ProcessedPath); err == nil {
				_ = addFileToZip(procRel, f)
				_ = f.Close()
				refs.Processed = procRel
			}
			if rf, err := os.Open(ds.ProcessedPath); err == nil {
				_ = addFileToZip(procRasterRel, rf)
				_ = rf.Close()
			}
		}

		// Source document export (e.g. PDF or CAD original)
		if opts.IncludeOriginalRasters && ds.ExtractedMetadata != nil {
			if origSourcePath, ok := ds.ExtractedMetadata["original_source_path"].(string); ok && origSourcePath != "" && fileExists(origSourcePath) {
				sourceDocName := filepath.Base(origSourcePath)
				docID, _ := ds.ExtractedMetadata["document_id"].(string)
				if docID == "" {
					docID = "doc"
				}
				sourceRel := "source_documents/" + docID + "_" + sourceDocName
				alreadyCopied := false
				for _, r := range archiveResources {
					if r == sourceRel {
						alreadyCopied = true
						break
					}
				}
				if !alreadyCopied {
					if sf, err := os.Open(origSourcePath); err == nil {
						_ = addFileToZip(sourceRel, sf)
						_ = sf.Close()
					}
				}
			}
		}

		// World file sidecar
		baseExt := strings.ToLower(filepath.Ext(ds.Filename))
		worldExt := ".wld"
		switch baseExt {
		case ".png":
			worldExt = ".pgw"
		case ".jpg", ".jpeg":
			worldExt = ".jgw"
		}
		srcWorldFile := filepath.Join(filepath.Dir(ds.StoragePath), strings.TrimSuffix(filepath.Base(ds.Filename), baseExt)+worldExt)
		if fileExists(srcWorldFile) {
			wldRel := layerDir + "/" + filepath.Base(srcWorldFile)
			if wf, err := os.Open(srcWorldFile); err == nil {
				_ = addFileToZip(wldRel, wf)
				_ = wf.Close()
				refs.WorldFile = wldRel
			}
		}

		layerAssetRefs[lID] = refs

		// Write individual layer.json
		layerDoc := map[string]interface{}{
			"id":                  ds.ID,
			"filename":            ds.Filename,
			"file_size":           ds.FileSize,
			"content_type":        ds.ContentType,
			"georeference_status": ds.GeoreferenceStatus,
			"assets":              refs,
		}
		if opts.IncludeLayerStyles {
			layerDoc["map_layer"] = ds.MapLayer
			if bg, ok := ds.ExtractedMetadata["background_removal"].(map[string]interface{}); ok {
				layerDoc["background_removal"] = bg
			}
			if tr, ok := ds.ExtractedMetadata["raster_transform"].(map[string]interface{}); ok {
				layerDoc["raster_transform"] = tr
			}
			if crn, ok := ds.ExtractedMetadata["raster_corners"]; ok {
				layerDoc["raster_corners"] = crn
			}
			if op, ok := ds.ExtractedMetadata["raster_opacity"]; ok {
				layerDoc["raster_opacity"] = op
			}
		}
		if opts.IncludeControlPoints {
			pts, _ := s.repo.ListControlPoints(sessionID, ds.ID)
			layerDoc["control_points"] = pts
		}
		layerDoc["extracted_metadata"] = ds.ExtractedMetadata

		_ = addJSONToZip(layerDir+"/layer.json", layerDoc)
	}

	// 3. Export Buildings and Floors
	var exportedBuildings []*models.Building
	if opts.IncludeBuildingConfig {
		for _, b := range allBuildings {
			if !selectedBuildingSet[b.ID] {
				continue
			}
			// Clone building to filter floors and datasets
			bCopy := *b
			var filteredFloors []*models.BuildingFloor
			for _, fl := range b.Floors {
				if !selectedFloorSet[fl.ID] {
					continue
				}
				flCopy := *fl
				var filteredDsIDs []string
				for _, dID := range fl.DatasetIDs {
					if selectedLayerSet[dID] {
						filteredDsIDs = append(filteredDsIDs, dID)
					}
				}
				flCopy.DatasetIDs = filteredDsIDs
				filteredFloors = append(filteredFloors, &flCopy)
			}
			bCopy.Floors = filteredFloors

			if !opts.IncludeControlPoints {
				bCopy.ControlPoints = nil
			}

			bDir := "buildings/" + b.ID
			_ = addJSONToZip(bDir+"/building.json", bCopy)
			exportedBuildings = append(exportedBuildings, &bCopy)
		}
	}

	// 4. Project Configuration & Backward Compatible mapstudio.json
	primaryProj, _ := s.repo.GetMapProject(sessionID, primaryDatasetID)
	var projConfig map[string]interface{}
	if primaryProj != nil {
		projConfig = primaryProj.Configuration
	} else {
		projConfig = make(map[string]interface{})
	}

	// Filter members in projConfig
	if membersRaw, ok := projConfig["members"].([]interface{}); ok {
		var filteredMembers []interface{}
		for _, m := range membersRaw {
			if mObj, ok := m.(map[string]interface{}); ok {
				mID, _ := mObj["dataset_id"].(string)
				if selectedLayerSet[mID] {
					filteredMembers = append(filteredMembers, mObj)
				}
			}
		}
		projConfig["members"] = filteredMembers
	}

	primaryBounds := extractDatasetBounds(primaryDs)

	// Backward-compatible mapstudio.json document
	var pts []*models.ControlPoint
	if opts.IncludeControlPoints {
		pts, _ = s.repo.ListControlPoints(sessionID, primaryDatasetID)
	}
	primaryRefs := layerAssetRefs[primaryDatasetID]
	sourceAssetRel := primaryRefs.Original
	if sourceAssetRel == "" {
		sourceAssetRel = primaryRefs.Processed
	}

	mapStudioDoc := map[string]interface{}{
		"format":  "veracity-maps-project",
		"version": 2,
		"project": map[string]interface{}{
			"name":          projectName,
			"configuration": projConfig,
		},
		"dataset": map[string]interface{}{
			"filename":            primaryDs.Filename,
			"content_type":        primaryDs.ContentType,
			"source_asset":        sourceAssetRel,
			"georeference_status": primaryDs.GeoreferenceStatus,
			"extracted_metadata":  primaryDs.ExtractedMetadata,
			"map_layer":           primaryDs.MapLayer,
		},
		"control_points": pts,
		"icon_points":    allIconPoints,
		"selected_scope": map[string]interface{}{
			"building_ids": sortedKeys(selectedBuildingSet),
			"floor_ids":    sortedKeys(selectedFloorSet),
			"layer_ids":    resolvedLayerIDs,
		},
	}
	_ = addJSONToZip("mapstudio.json", mapStudioDoc)

	// Render XYZ tiles if georeferenced
	var generatedZooms []int
	tileSourceImage := primaryDs.StoragePath
	transparentPNGCheck := filepath.Join(filepath.Dir(primaryDs.StoragePath), "processed", "processed_transparent.png")
	if fileExists(transparentPNGCheck) {
		tileSourceImage = transparentPNGCheck
	}

	// Render composite raster with markers burned onto a copy of the background raster
	compositePath, _ := createCompositeRasterWithMarkers(tileSourceImage, pts, allIconPoints, primaryBounds, sessionID)
	tileSourceImage = compositePath

	if len(primaryBounds) == 4 && (primaryDs.GeoreferenceStatus == "georeferenced" || fileExists(tileSourceImage)) {
		zooms, tileErr := gis.GenerateRasterTiles(archive, tileSourceImage, primaryBounds)
		if tileErr == nil && len(zooms) > 0 {
			generatedZooms = zooms
			for _, z := range zooms {
				archiveResources = append(archiveResources, fmt.Sprintf("%d/", z))
			}
		}
	}

	minZoom, maxZoom := 0, 0
	if len(generatedZooms) > 0 {
		minZoom = generatedZooms[0]
		maxZoom = generatedZooms[len(generatedZooms)-1]
	} else if len(primaryBounds) == 4 {
		minZoom, maxZoom = 15, 17
	}

	// metadata.json for GIS standard viewers
	// Package permitted offline basemap tiles into basemap/{z}/{x}/{y}.png
	hasBasemap := false
	if len(primaryBounds) == 4 {
		bmMinZ := minZoom
		bmMaxZ := maxZoom
		if len(generatedZooms) == 0 {
			bmMinZ, bmMaxZ = 14, 18
		}
		packagedBM, err := gis.PackageBasemapTiles(archive, primaryBounds, bmMinZ, bmMaxZ)
		if err == nil && packagedBM > 0 {
			hasBasemap = true
		}
	}

	// Calculate center and camera zoom settings
	var centerCoords []float64
	if len(primaryBounds) == 4 {
		centerCoords = []float64{(primaryBounds[0] + primaryBounds[2]) / 2.0, (primaryBounds[1] + primaryBounds[3]) / 2.0}
	} else {
		centerCoords = []float64{0.0, 0.0}
	}

	initZoom, camMinZoom, camMaxZoom, zoomPreset := resolveCameraZoomSettings(opts.ZoomConfig, minZoom, maxZoom, primaryBounds)

	metaDoc := map[string]interface{}{
		"name":              projectName,
		"version":           "2.0.0",
		"description":       "Exported from Veracity Maps",
		"attribution":       "Veracity Maps & OpenStreetMap contributors",
		"type":              "overlay",
		"format":            "png",
		"bounds":            primaryBounds,
		"center":            centerCoords,
		"initial_zoom":      initZoom,
		"profile":           "mercator",
		"minzoom":           camMinZoom,
		"maxzoom":           camMaxZoom,
		"raster_minzoom":    minZoom,
		"raster_maxzoom":    maxZoom,
		"zoom_preset":       zoomPreset,
		"tile_url_template": "./{z}/{x}/{y}.png",
		"available_zooms":   generatedZooms,
		"crs":               "EPSG:3857",
		"control_points":    pts,
		"icon_points":       allIconPoints,
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
			"bounds":            primaryBounds,
			"has_tiles":         len(generatedZooms) > 0,
		},
		"offline_ready": true,
		"export_time":   time.Now().UTC().Format(time.RFC3339),
	}
	_ = addJSONToZip("metadata.json", metaDoc)

	// Build GeoJSON landmarks FeatureCollection
	landmarkFeatures := make([]map[string]interface{}, 0)
	if opts.IncludeIcons {
		for _, ip := range allIconPoints {
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
				name = "Landmark"
			}
			props := map[string]interface{}{
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
			}
			if len(ip.Properties) > 0 {
				for k, v := range ip.Properties {
					if _, exists := props[k]; !exists {
						props[k] = v
					}
				}
			}
			landmarkFeatures = append(landmarkFeatures, map[string]interface{}{
				"type": "Feature",
				"geometry": map[string]interface{}{
					"type":        "Point",
					"coordinates": []float64{ip.Longitude, ip.Latitude},
				},
				"properties": props,
			})
		}
	}
	landmarksDoc := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": landmarkFeatures,
	}
	_ = addJSONToZip("layers/landmarks.geojson", landmarksDoc)

	// Build GeoJSON control points FeatureCollection
	cpFeatures := make([]map[string]interface{}, 0)
	if opts.IncludeControlPoints {
		for _, cp := range pts {
			cpFeatures = append(cpFeatures, map[string]interface{}{
				"type": "Feature",
				"geometry": map[string]interface{}{
					"type":        "Point",
					"coordinates": []float64{cp.Longitude, cp.Latitude},
				},
				"properties": map[string]interface{}{
					"id":         cp.ID,
					"name":       cp.ID,
					"dataset_id": cp.DatasetID,
					"image_x":    cp.ImageX,
					"image_y":    cp.ImageY,
					"longitude":  cp.Longitude,
					"latitude":   cp.Latitude,
				},
			})
		}
	}
	controlPointsDoc := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": cpFeatures,
	}
	_ = addJSONToZip("layers/control_points.geojson", controlPointsDoc)

	// metadata/layer_config.json
	layerConfigDoc := map[string]interface{}{
		"project_id":         primaryDatasetID,
		"primary_dataset_id": primaryDatasetID,
		"buildings":          exportedBuildings,
		"layers":             resolvedLayerIDs,
		"layer_asset_refs":   layerAssetRefs,
		"configuration":      projConfig,
	}
	_ = addJSONToZip("metadata/layer_config.json", layerConfigDoc)

	// metadata/georeferencing.json
	georeferencingDoc := map[string]interface{}{
		"crs":                 "EPSG:3857",
		"geographic_crs":      "EPSG:4326",
		"bounds":              primaryBounds,
		"center":              centerCoords,
		"control_point_count": len(pts),
		"control_points":      pts,
		"extracted_metadata":  primaryDs.ExtractedMetadata,
		"georeference_status": primaryDs.GeoreferenceStatus,
	}
	_ = addJSONToZip("metadata/georeferencing.json", georeferencingDoc)

	// project.json: Conforms to veracity-project v1 manifest while retaining backward-compatible keys
	projectDoc := map[string]interface{}{
		"format":         "veracity-project",
		"schema_version": 1,
		"name":           projectName,
		"version":        "2.0.0",
		"project": map[string]interface{}{
			"id":   primaryDatasetID,
			"name": projectName,
		},
		"map": map[string]interface{}{
			"center":         centerCoords,
			"zoom":           initZoom,
			"min_zoom":       camMinZoom,
			"max_zoom":       camMaxZoom,
			"raster_minzoom": minZoom,
			"raster_maxzoom": maxZoom,
			"preset":         zoomPreset,
			"bounds":         primaryBounds,
			"crs":            "EPSG:3857",
		},
		"resources": map[string]interface{}{
			"style":          "style.json",
			"base_map":       "./basemap/{z}/{x}/{y}.png",
			"landmarks":      "layers/landmarks.geojson",
			"control_points": "layers/control_points.geojson",
			"layer_config":   "metadata/layer_config.json",
			"georeferencing": "metadata/georeferencing.json",
		},
		"primary_dataset_id": primaryDatasetID,
		"configuration":      projConfig,
		"buildings":          exportedBuildings,
		"layers":             resolvedLayerIDs,
		"initial_zoom":       initZoom,
		"minzoom":            camMinZoom,
		"maxzoom":            camMaxZoom,
		"zoom_preset":        zoomPreset,
		"export_timestamp":   time.Now().UTC().Format(time.RFC3339),
	}
	_ = addJSONToZip("project.json", projectDoc)

	// MapLibre Style Specification (v8)
	styleBytes, _ := generateMapLibreStyleJSON(projectName, primaryBounds, minZoom, maxZoom, 1.0)
	sw, _ := archive.Create("style.json")
	_, _ = sw.Write(styleBytes)
	archiveResources = append(archiveResources, "style.json")

	// HTML Viewers (MapLibre viewer.html & index.html)
	viewers := generateViewers(ViewscapeViewerParams{
		Title:            projectName,
		Bounds:           primaryBounds,
		RasterMinZoom:    minZoom,
		RasterMaxZoom:    maxZoom,
		InitialZoom:      initZoom,
		MinZoom:          camMinZoom,
		MaxZoom:          camMaxZoom,
		Preset:           zoomPreset,
		Opacity:          1.0,
		ControlPoints:    pts,
		IconPoints:       allIconPoints,
		BundledIconPaths: iconAssetsCopied,
	})
	for name, htmlContent := range viewers {
		vw, _ := archive.Create(name)
		_, _ = vw.Write([]byte(htmlContent))
		archiveResources = append(archiveResources, name)
	}

	// README.txt
	readmeContent := fmt.Sprintf(`================================================================================
Veracity Maps — Complete Project Archive (.zip)
================================================================================
Project:        %s
Export Format:  veracity-project (Schema Version 1)
Timestamp:      %s
Layers:         %d
Buildings:      %d

Package Directory Structure:
----------------------------
• project.json                  Root manifest (veracity-project v1) with center, zoom, bounds, CRS, and resource map.
• style.json                    MapLibre Style Specification v8 for offline layer and tile styling.
• viewer.html / index.html      Self-contained MapLibre offline Viewscape viewers.
• layers/                       Exported vector overlays and per-layer configurations:
    - landmarks.geojson         FeatureCollection of landmark icon points with coordinates, icon keys, and properties.
    - control_points.geojson    FeatureCollection of ground control points with geographic and image pixel coordinates.
    - {layer_id}/layer.json     Detailed layer styles, transformations, and sidecar definitions.
• rasters/                      Source and processed raster floorplans for offline viewing and editing.
• tiles/ or ./{z}/{x}/{y}.png   Offline raster overlay tile pyramid.
• basemap/                      Offline base world map tiles (when available).
• assets/icons/                 Bundled custom icon assets (PNG/SVG) referenced by landmarks.
• metadata/                     Supplementary reconstruction metadata:
    - layer_config.json         Hierarchical building, floor, opacity, and layer ordering configurations.
    - georeferencing.json       CRS, transformation parameters, bounds, and control point associations.

How to Reopen / Reconstruct:
----------------------------
1. In Veracity Maps: Click "Reopen Map (.zip)" on the Workspace top bar and select this ZIP archive.
   All layers, buildings, floors, control points, icons, and georeferencing are restored automatically.
2. In Offline Viewscape: Open "viewer.html" or "index.html" in any modern browser without internet access.
`, projectName, time.Now().UTC().Format(time.RFC3339), len(resolvedLayerIDs), len(exportedBuildings))
	rw, _ := archive.Create("README.txt")
	_, _ = rw.Write([]byte(readmeContent))
	archiveResources = append(archiveResources, "README.txt")

	// Final top-level manifest.json
	manifest := ExportManifest{
		FormatVersion:     "2.0.0",
		FormatName:        "veracity-maps-export",
		Generator:         "Veracity Maps Export Engine",
		ExportTimestamp:   time.Now().UTC().Format(time.RFC3339),
		ProjectName:       projectName,
		PrimaryDatasetID:  primaryDatasetID,
		SelectedBuildings: sortedKeys(selectedBuildingSet),
		SelectedFloors:    sortedKeys(selectedFloorSet),
		SelectedLayers:    resolvedLayerIDs,
		IncludedOptions:   opts,
		Resources:         archiveResources,
	}
	_ = addJSONToZip("manifest.json", manifest)

	return zipPath, stem + ".zip", nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
