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

// SelectiveExportOptions controls what supporting assets and metadata are included
type SelectiveExportOptions struct {
	IncludeBuildingConfig   bool `json:"include_building_configuration"`
	IncludeControlPoints    bool `json:"include_control_points"`
	IncludeIcons            bool `json:"include_icons"`
	IncludeLayerStyles      bool `json:"include_layer_styles"`
	IncludeOriginalRasters  bool `json:"include_original_rasters"`
	IncludeProcessedRasters bool `json:"include_processed_rasters"`
	IncludeBaseMapSettings  bool `json:"include_base_map_settings"`
	IncludeProjectMetadata  bool `json:"include_project_metadata"`
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
	}
}

// SelectiveExportParams specifies the requested scope and options
type SelectiveExportParams struct {
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
	exportDir := filepath.Join(s.storage.GetDatasetDir(sessionID, primaryDatasetID), "exports")
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
	iconAssetsCopied := make(map[string]string) // icon_id -> archive path
	if opts.IncludeIcons && len(allIconPoints) > 0 {
		for _, pt := range allIconPoints {
			if pt.IconID != "" && iconAssetsCopied[pt.IconID] == "" {
				// Search for icon asset on disk
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
						archiveRel := "icons/assets/" + foundName
						_ = addFileToZip(archiveRel, f)
						_ = f.Close()
						iconAssetsCopied[pt.IconID] = archiveRel
					}
				}
			}
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
			if f, err := os.Open(ds.StoragePath); err == nil {
				_ = addFileToZip(origRel, f)
				_ = f.Close()
				refs.Original = origRel
			}
		}

		// Processed transparent raster
		transparentPNG := filepath.Join(filepath.Dir(ds.StoragePath), "processed", "processed_transparent.png")
		if opts.IncludeProcessedRasters && fileExists(transparentPNG) {
			procRel := layerDir + "/processed_transparent.png"
			if f, err := os.Open(transparentPNG); err == nil {
				_ = addFileToZip(procRel, f)
				_ = f.Close()
				refs.Processed = procRel
			}
		}

		// If processed is requested but not transparent, check if ProcessedPath exists
		if opts.IncludeProcessedRasters && refs.Processed == "" && ds.ProcessedPath != "" && fileExists(ds.ProcessedPath) {
			pBase := filepath.Base(ds.ProcessedPath)
			procRel := layerDir + "/processed_" + pBase
			if f, err := os.Open(ds.ProcessedPath); err == nil {
				_ = addFileToZip(procRel, f)
				_ = f.Close()
				refs.Processed = procRel
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

	var primaryBounds []float64
	if b, ok := primaryDs.ExtractedMetadata["bounds"].(map[string]float64); ok {
		primaryBounds, _ = gis.NormalizeBounds(b)
	}

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
		"selected_scope": map[string]interface{}{
			"building_ids": sortedKeys(selectedBuildingSet),
			"floor_ids":    sortedKeys(selectedFloorSet),
			"layer_ids":    resolvedLayerIDs,
		},
	}
	_ = addJSONToZip("mapstudio.json", mapStudioDoc)

	// metadata.json for GIS standard viewers
	metaDoc := map[string]interface{}{
		"name":        projectName,
		"version":     "2.0.0",
		"description": "Exported from Veracity Maps",
		"attribution": "Veracity Maps",
		"type":        "overlay",
		"format":      "png",
		"bounds":      primaryBounds,
		"profile":     "mercator",
	}
	_ = addJSONToZip("metadata.json", metaDoc)

	// project.json
	projectDoc := map[string]interface{}{
		"name":               projectName,
		"version":            "2.0.0",
		"primary_dataset_id": primaryDatasetID,
		"configuration":      projConfig,
		"buildings":          exportedBuildings,
		"layers":             resolvedLayerIDs,
	}
	_ = addJSONToZip("project.json", projectDoc)

	// HTML Viewers
	viewers := generateViewers(projectName, primaryBounds)
	for name, htmlContent := range viewers {
		vw, _ := archive.Create(name)
		_, _ = vw.Write([]byte(htmlContent))
		archiveResources = append(archiveResources, name)
	}

	// README.txt
	readmeContent := fmt.Sprintf(`Veracity Maps Export Archive
Project: %s
Timestamp: %s
Layers Included: %d
Buildings Included: %d

This package contains raster layers, spatial transformations, control points, 
building-floor configurations, and icons exported from Veracity Maps.
To reopen this project in Veracity Maps, use the "Reopen Map (.zip)" button.
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
