package services

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/storage"
)

// ZoomConfiguration controls initial, min, and max camera zoom levels and preset in exported projects.
type ZoomConfiguration struct {
	InitialZoom *int   `json:"initial_zoom,omitempty"`
	MinZoom     *int   `json:"min_zoom,omitempty"`
	MaxZoom     *int   `json:"max_zoom,omitempty"`
	Preset      string `json:"preset,omitempty"`
}

// SelectiveExportOptions controls what supporting assets and metadata are included in a project .zip package.
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

// DefaultSelectiveExportOptions returns defaults for MBTiles-backed project .zip exports.
func DefaultSelectiveExportOptions() *SelectiveExportOptions {
	return &SelectiveExportOptions{
		IncludeBuildingConfig:   true,
		IncludeControlPoints:    true,
		IncludeIcons:            true,
		IncludeLayerStyles:      true,
		IncludeOriginalRasters:  true,
		IncludeProcessedRasters: true,
		IncludeBaseMapSettings:  true,
		IncludeProjectMetadata:  true,
		ZoomConfig:              nil,
	}
}

// SelectiveExportParams specifies the requested scope and options.
type SelectiveExportParams struct {
	Format      string
	ProjectName string
	BuildingIDs []string
	FloorIDs    []string
	LayerIDs    []string
	Options     *SelectiveExportOptions
}

// LayerAssetRefs tracks non-duplicated archive paths for each exported layer.
type LayerAssetRefs struct {
	MBTiles       string `json:"mbtiles,omitempty"`
	Original      string `json:"original,omitempty"`
	Enhanced      string `json:"enhanced,omitempty"`
	Processed     string `json:"processed,omitempty"`
	MainRaster    string `json:"main_raster,omitempty"`
	WorldFile     string `json:"world_file,omitempty"`
	Vectors       string `json:"vectors,omitempty"`
	SourceVectors string `json:"source_vectors,omitempty"`
	Annotations   string `json:"annotations,omitempty"`
}

// ExportManifest defines the top-level manifest.json schema (MapTiler-compatible v1.0 / v3.0.0 project package).
type ExportManifest struct {
	Format             string                   `json:"format"`
	SchemaVersion      string                   `json:"schema_version"`
	Application        string                   `json:"application"`
	ApplicationVersion string                   `json:"application_version"`
	FormatVersion      string                   `json:"format_version"`
	FormatName         string                   `json:"format_name"`
	Generator          string                   `json:"generator"`
	ExportTimestamp    string                   `json:"export_timestamp"`
	ProjectID          string                   `json:"project_id"`
	ProjectName        string                   `json:"project_name"`
	PrimaryDatasetID   string                   `json:"primary_dataset_id"`
	ProjectMBTiles     string                   `json:"project_mbtiles"`
	CustomLocalTiles   string                   `json:"custom_local_tiles,omitempty"`
	MainRaster         string                   `json:"main_raster,omitempty"`
	LayerMBTiles       map[string]string        `json:"layer_mbtiles,omitempty"`
	CRS                string                   `json:"crs,omitempty"`
	Bounds             []float64                `json:"bounds,omitempty"`
	Center             []float64                `json:"center,omitempty"`
	ZoomConfiguration  map[string]interface{}   `json:"zoom_configuration,omitempty"`
	Datasets           []map[string]interface{} `json:"datasets,omitempty"`
	Layers             []map[string]interface{} `json:"layers,omitempty"`
	Buildings          []map[string]interface{} `json:"buildings,omitempty"`
	Assets             map[string]string        `json:"assets,omitempty"`
	Project            map[string]interface{}   `json:"project,omitempty"`
	SelectedBuildings  []string                 `json:"selected_buildings"`
	SelectedFloors     []string                 `json:"selected_floors"`
	SelectedLayers     []string                 `json:"selected_layers"`
	IncludedOptions    *SelectiveExportOptions  `json:"included_options"`
	Resources          []string                 `json:"resources"`
}

// GenerateSelectiveExport creates either a standalone SQLite .mbtiles archive or a structured
// MBTiles-backed .zip project package containing selected buildings, floors, layers, vectors, and metadata.
func (s *ExportService) GenerateSelectiveExport(sessionID, primaryDatasetID string, params SelectiveExportParams) (string, string, error) {
	primaryDs, err := s.repo.GetDataset(sessionID, primaryDatasetID)
	if err != nil || primaryDs == nil {
		return "", "", fmt.Errorf("DATASET_NOT_FOUND: Primary dataset not found: %w", err)
	}

	exportDir := filepath.Join(s.storage.GetDatasetDir(sessionID, primaryDatasetID), "exports")
	_ = os.MkdirAll(exportDir, 0755)

	normFormat := strings.ToLower(strings.TrimSpace(params.Format))
	if normFormat == "base_layer" || normFormat == "basemap" {
		return s.exportBaseLayerFile(primaryDs, exportDir)
	}

	opts := params.Options
	if opts == nil {
		opts = DefaultSelectiveExportOptions()
	}

	if normFormat != "mbtiles" && normFormat != "base_layer" && normFormat != "basemap" {
		if !opts.IncludeOriginalRasters && !opts.IncludeProcessedRasters {
			return "", "", fmt.Errorf("INVALID_EXPORT_OPTIONS: At least one raster option (processed MBTiles imagery or original rasters) must be enabled")
		}
	}

	// 1. Resolve all buildings, floors, and datasets in the session
	allBuildings, _ := s.repo.ListBuildings(sessionID)
	allDatasets, _ := s.repo.ListDatasets(sessionID)

	datasetMap := make(map[string]*models.Dataset)
	for _, ds := range allDatasets {
		datasetMap[ds.ID] = ds
	}
	if _, ok := datasetMap[primaryDatasetID]; !ok {
		datasetMap[primaryDatasetID] = primaryDs
	}

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

	primaryProj, _ := s.repo.GetMapProject(sessionID, primaryDatasetID)

	// Hierarchy expansion when no explicit LayerIDs were specified
	if len(selectedLayerSet) == 0 {
		if len(selectedBuildingSet) > 0 || len(selectedFloorSet) > 0 {
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
			// Default full-project export: include all members of the MapProject (plus primary dataset)
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
		}
	}

	// Preserve workspace member sort_order when available
	memberOrder := make(map[string]int)
	memberMetaByID := make(map[string]map[string]interface{})
	if primaryProj != nil && primaryProj.Configuration != nil {
		if rawMembers, ok := primaryProj.Configuration["members"].([]interface{}); ok {
			for idx, rm := range rawMembers {
				if m, ok := rm.(map[string]interface{}); ok {
					mID, _ := m["dataset_id"].(string)
					if mID != "" {
						so := idx
						if _, hasSO := m["sort_order"]; hasSO {
							so = int(getDim(m["sort_order"]))
						}
						memberOrder[mID] = so
						memberMetaByID[mID] = m
					}
				}
			}
		}
	}

	var resolvedLayerIDs []string
	for lID := range selectedLayerSet {
		if ds, exists := datasetMap[lID]; exists && ds != nil {
			resolvedLayerIDs = append(resolvedLayerIDs, lID)
		}
	}
	sort.Slice(resolvedLayerIDs, func(i, j int) bool {
		oi, hasI := memberOrder[resolvedLayerIDs[i]]
		oj, hasJ := memberOrder[resolvedLayerIDs[j]]
		if hasI && hasJ && oi != oj {
			return oi < oj
		}
		if hasI != hasJ {
			return hasI
		}
		return resolvedLayerIDs[i] < resolvedLayerIDs[j]
	})

	if len(resolvedLayerIDs) == 0 {
		return "", "", fmt.Errorf("NO_LAYERS_SELECTED: At least one valid layer must be selected for export")
	}

	// Standalone .mbtiles export path (supports single or multi-layer workspace selection)
	if normFormat == "mbtiles" {
		selectedDatasets := make([]*models.Dataset, 0, len(resolvedLayerIDs))
		for _, lID := range resolvedLayerIDs {
			if m, ok := memberMetaByID[lID]; ok {
				if vis, hasVis := m["visible"].(bool); hasVis && !vis {
					continue
				}
				if rVis, hasRVis := m["raster_visible"].(bool); hasRVis && !rVis {
					continue
				}
			}
			selectedDatasets = append(selectedDatasets, datasetMap[lID])
		}
		if len(selectedDatasets) == 0 {
			for _, lID := range resolvedLayerIDs {
				selectedDatasets = append(selectedDatasets, datasetMap[lID])
			}
		}
		return s.exportStandaloneMBTiles(sessionID, primaryDs, selectedDatasets, params.ProjectName, exportDir, opts)
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

	exportPrimaryID := primaryDatasetID
	if !selectedLayerSet[exportPrimaryID] {
		exportPrimaryID = resolvedLayerIDs[0]
		primaryDs = datasetMap[exportPrimaryID]
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
	zipPath := filepath.Join(exportDir, stem+".zip")

	stageDir := filepath.Join(exportDir, ".stage_"+stem)
	_ = storage.SafeRemoveAll(stageDir)
	_ = os.MkdirAll(stageDir, 0755)
	defer func() {
		_ = storage.SafeRemoveAll(stageDir)
	}()

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating export archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	var archiveResources []string
	seenZipEntries := make(map[string]bool)
	addFileToZip := func(archivePath string, r io.Reader) error {
		archivePath = filepath.ToSlash(archivePath)
		if seenZipEntries[archivePath] {
			return nil
		}
		w, err := archive.Create(archivePath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			return err
		}
		seenZipEntries[archivePath] = true
		archiveResources = append(archiveResources, archivePath)
		return nil
	}

	addDiskFileToZip := func(archivePath, diskPath string) error {
		f, err := os.Open(diskPath)
		if err != nil {
			return err
		}
		defer f.Close()
		return addFileToZip(archivePath, f)
	}

	addJSONToZip := func(archivePath string, v interface{}) error {
		archivePath = filepath.ToSlash(archivePath)
		if seenZipEntries[archivePath] {
			return nil
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		w, err := archive.Create(archivePath)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		seenZipEntries[archivePath] = true
		archiveResources = append(archiveResources, archivePath)
		return nil
	}

	layerAssetRefs := make(map[string]LayerAssetRefs)
	layerMBTilesMap := make(map[string]string)

	// Collect icons across selected layers & buildings
	var allIconPoints []*models.IconPoint
	if opts.IncludeIcons {
		seenIP := make(map[string]bool)
		for _, lID := range resolvedLayerIDs {
			pts, _ := s.repo.ListIconPoints(sessionID, lID, "")
			for _, p := range pts {
				if p != nil && !seenIP[p.ID] {
					seenIP[p.ID] = true
					allIconPoints = append(allIconPoints, p)
				}
			}
		}
		for bID := range selectedBuildingSet {
			pts, _ := s.repo.ListIconPoints(sessionID, "", bID)
			for _, p := range pts {
				if p != nil && !seenIP[p.ID] {
					seenIP[p.ID] = true
					allIconPoints = append(allIconPoints, p)
				}
			}
		}
	}

	sessionDir := s.storage.GetSessionDir(sessionID)
	iconAssetsCopied := make(map[string]string)
	if opts.IncludeIcons && len(allIconPoints) > 0 {
		iconAssetsCopied = bundleReferencedIcons(archive, allIconPoints, sessionDir)
		for _, p := range iconAssetsCopied {
			if !seenZipEntries[p] {
				seenZipEntries[p] = true
				archiveResources = append(archiveResources, p)
			}
		}
		_ = addJSONToZip("icons/icon_points.json", allIconPoints)
	}

	// Track combined bounds, zoom range, control points, and vector annotations
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

	var allControlPoints []*models.ControlPoint
	var allAnnotationFeatures []map[string]interface{}
	var compositeSpecs []gis.MBTilesLayerInput
	var singleVisibleMBTilesDiskPath string
	minTileZoom := 99
	maxTileZoom := 0

	// 2. Package each selected layer: generate SQLite .mbtiles + single non-duplicated layer assets
	for _, lID := range resolvedLayerIDs {
		ds := datasetMap[lID]
		if ds == nil {
			continue
		}

		ProjectDatasetVectorsAndAnnotations(s.storage, ds)
		dsDir := resolveDatasetDirectory(s.storage, ds)
		var refs LayerAssetRefs
		layerZipDir := "layers/" + lID

		layerOpacity := 1.0
		layerVisible := true
		if m, ok := memberMetaByID[lID]; ok {
			if op := getDim(m["raster_opacity"]); op > 0 && op <= 1.0 {
				layerOpacity = op
			}
			if vis, hasVis := m["visible"].(bool); hasVis && !vis {
				layerVisible = false
			}
			if rVis, hasRVis := m["raster_visible"].(bool); hasRVis && !rVis {
				layerVisible = false
			}
		}

		// 2a. Per-layer SQLite .mbtiles generation
		dsBounds := extractDatasetBounds(ds)
		if len(dsBounds) == 4 {
			expandUnionBounds(dsBounds)
		}

		storageExt := strings.ToLower(filepath.Ext(ds.StoragePath))
		mbtilesRel := "tiles/" + lID + ".mbtiles"
		stagedLayerMBTiles := filepath.Join(stageDir, lID+".mbtiles")

		if storageExt == ".mbtiles" && fileExists(ds.StoragePath) && gis.HasSQLiteHeader(ds.StoragePath) {
			if storage.CopyFile(ds.StoragePath, stagedLayerMBTiles) == nil {
				if addDiskFileToZip(mbtilesRel, stagedLayerMBTiles) == nil {
					refs.MBTiles = mbtilesRel
					layerMBTilesMap[lID] = mbtilesRel
					if insp, iErr := gis.InspectMBTiles(stagedLayerMBTiles, "", ""); iErr == nil && insp != nil {
						if insp.MinZoom < minTileZoom {
							minTileZoom = insp.MinZoom
						}
						if insp.MaxZoom > maxTileZoom {
							maxTileZoom = insp.MaxZoom
						}
						expandUnionBounds(insp.Bounds)
					}
					if layerVisible && singleVisibleMBTilesDiskPath == "" {
						singleVisibleMBTilesDiskPath = stagedLayerMBTiles
					}
				}
			}
		} else if len(dsBounds) == 4 {
			if spec, specErr := s.buildLayerMBTilesInput(sessionID, ds, 1.0); specErr == nil && spec != nil {
				if cRes, cErr := gis.CreateMBTiles(stagedLayerMBTiles, gis.MBTilesCreateSpec{
					Name:        ds.Filename,
					Description: "Veracity Maps Layer MBTiles 1.3",
					TileFormat:  "png",
					Layers:      []gis.MBTilesLayerInput{*spec},
				}); cErr == nil && cRes != nil && cRes.TileCount > 0 && fileExists(stagedLayerMBTiles) {
					if addDiskFileToZip(mbtilesRel, stagedLayerMBTiles) == nil {
						refs.MBTiles = mbtilesRel
						layerMBTilesMap[lID] = mbtilesRel
						if cRes.MinZoom < minTileZoom {
							minTileZoom = cRes.MinZoom
						}
						if cRes.MaxZoom > maxTileZoom {
							maxTileZoom = cRes.MaxZoom
						}
					}
				}
				if layerVisible {
					visSpec := *spec
					visSpec.Opacity = layerOpacity
					compositeSpecs = append(compositeSpecs, visSpec)
					if len(compositeSpecs) == 1 && layerOpacity == 1.0 && fileExists(stagedLayerMBTiles) {
						singleVisibleMBTilesDiskPath = stagedLayerMBTiles
					} else {
						singleVisibleMBTilesDiskPath = ""
					}
				}
			}
		}

		// 2b. Single non-duplicated original & enhanced/processed assets (never duplicated under rasters/)
		if opts.IncludeOriginalRasters && storageExt != ".mbtiles" && ds.StoragePath != "" && fileExists(ds.StoragePath) {
			origRel := layerZipDir + "/source" + storageExt
			if addDiskFileToZip(origRel, ds.StoragePath) == nil {
				refs.Original = origRel
			}
			rasterOrigRel := "rasters/original/" + safeStem(ds.Filename) + storageExt
			_ = addDiskFileToZip(rasterOrigRel, ds.StoragePath)
		}

		enhancedPNG := filepath.Join(dsDir, "processed", "enhanced.png")
		if opts.IncludeProcessedRasters && fileExists(enhancedPNG) {
			enhRel := layerZipDir + "/enhanced.png"
			if addDiskFileToZip(enhRel, enhancedPNG) == nil {
				refs.Enhanced = enhRel
			}
			_ = addDiskFileToZip("rasters/processed/"+lID+"_enhanced.png", enhancedPNG)
		}

		transparentPNG := filepath.Join(dsDir, "processed", "processed_transparent.png")
		if opts.IncludeProcessedRasters && fileExists(transparentPNG) {
			procRel := layerZipDir + "/processed_transparent.png"
			if addDiskFileToZip(procRel, transparentPNG) == nil {
				refs.Processed = procRel
			}
			_ = addDiskFileToZip("rasters/processed/"+lID+"_transparent.png", transparentPNG)
		} else if opts.IncludeProcessedRasters && storageExt == ".mbtiles" {
			previewPNG := filepath.Join(dsDir, "processed", "mbtiles_preview.png")
			if fileExists(previewPNG) {
				prevRel := layerZipDir + "/mbtiles_preview.png"
				if addDiskFileToZip(prevRel, previewPNG) == nil {
					refs.Processed = prevRel
				}
				_ = addDiskFileToZip("rasters/processed/"+lID+"_preview.png", previewPNG)
			}
		}

		// Main renderable raster for this layer (with AI upscaling and/or background transparency applied)
		layerMainRaster := s.resolveExportRasterPath(ds)
		if layerMainRaster != "" && fileExists(layerMainRaster) {
			lmExt := strings.ToLower(filepath.Ext(layerMainRaster))
			if lmExt == "" {
				lmExt = ".png"
			}
			lmRel := layerZipDir + "/main_raster" + lmExt
			if addDiskFileToZip(lmRel, layerMainRaster) == nil {
				refs.MainRaster = lmRel
			}
			if t, ok := extractAffineTransform(ds.ExtractedMetadata); ok && len(t) == 6 {
				origW := int(getDim(ds.ExtractedMetadata["width"]))
				origH := int(getDim(ds.ExtractedMetadata["height"]))
				actualW, actualH := origW, origH
				if meta, mErr := gis.InspectRasterFile(layerMainRaster); mErr == nil && meta != nil && meta.Width > 0 && meta.Height > 0 {
					actualW, actualH = meta.Width, meta.Height
				}
				scaledT := t
				if origW > 0 && origH > 0 && actualW > 0 && actualH > 0 {
					rx := float64(origW) / float64(actualW)
					ry := float64(origH) / float64(actualH)
					scaledT = []float64{
						t[0] * rx,
						t[1] * ry,
						t[2],
						t[3] * rx,
						t[4] * ry,
						t[5],
					}
				}
				worldContent := fmt.Sprintf("%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n",
					scaledT[0], scaledT[3], scaledT[1], scaledT[4], scaledT[2], scaledT[5],
				)
				_ = addFileToZip(layerZipDir+"/main_raster.pgw", strings.NewReader(worldContent))
				_ = addFileToZip(layerZipDir+"/main_raster.wld", strings.NewReader(worldContent))
				prjWKT := "GEOGCS[\"WGS 84\",DATUM[\"WGS_1984\",SPHEROID[\"WGS 84\",6378137,298.257223563]],PRIMEM[\"Greenwich\",0],UNIT[\"degree\",0.0174532925199433]]\n"
				_ = addFileToZip(layerZipDir+"/main_raster.prj", strings.NewReader(prjWKT))
			}
		}

		// Original CAD/PDF source document when requested
		if opts.IncludeOriginalRasters && ds.ExtractedMetadata != nil {
			if origSourcePath, ok := ds.ExtractedMetadata["original_source_path"].(string); ok && origSourcePath != "" && fileExists(origSourcePath) {
				sourceDocName := filepath.Base(origSourcePath)
				docID, _ := ds.ExtractedMetadata["source_document_id"].(string)
				if docID == "" {
					docID = "doc"
				}
				sourceRel := "source_documents/" + docID + "_" + sourceDocName
				_ = addDiskFileToZip(sourceRel, origSourcePath)
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
		case ".tif", ".tiff":
			worldExt = ".tfw"
		}
		srcWorldFile := filepath.Join(filepath.Dir(ds.StoragePath), strings.TrimSuffix(filepath.Base(ds.Filename), baseExt)+worldExt)
		if fileExists(srcWorldFile) {
			wldRel := layerZipDir + "/" + filepath.Base(srcWorldFile)
			if addDiskFileToZip(wldRel, srcWorldFile) == nil {
				refs.WorldFile = wldRel
			}
		}

		// PDF/CAD vectors & OCR/vector label annotations
		srcVectorsPath := filepath.Join(dsDir, "vector", "source_vectors.json")
		if fileExists(srcVectorsPath) {
			rel := layerZipDir + "/source_vectors.json"
			if addDiskFileToZip(rel, srcVectorsPath) == nil {
				refs.SourceVectors = rel
			}
		}
		geoVectorsPath := filepath.Join(dsDir, "vector", "vectors.geojson")
		if fileExists(geoVectorsPath) {
			rel := layerZipDir + "/vectors.geojson"
			if addDiskFileToZip(rel, geoVectorsPath) == nil {
				refs.Vectors = rel
			}
		}
		annJSONPath := filepath.Join(dsDir, "vector", "annotations.json")
		if fileExists(annJSONPath) {
			rel := layerZipDir + "/annotations.json"
			if addDiskFileToZip(rel, annJSONPath) == nil {
				refs.Annotations = rel
			}
		}
		annGeoPath := filepath.Join(dsDir, "vector", "annotations.geojson")
		if fileExists(annGeoPath) {
			if raw, rErr := os.ReadFile(annGeoPath); rErr == nil {
				var fc struct {
					Features []map[string]interface{} `json:"features"`
				}
				if json.Unmarshal(raw, &fc) == nil {
					allAnnotationFeatures = append(allAnnotationFeatures, fc.Features...)
				}
			}
		}

		layerAssetRefs[lID] = refs

		var layerCPs []*models.ControlPoint
		if opts.IncludeControlPoints {
			layerCPs, _ = s.repo.ListControlPoints(sessionID, ds.ID)
			allControlPoints = append(allControlPoints, layerCPs...)
		}

		layerDoc := map[string]interface{}{
			"id":                  ds.ID,
			"filename":            ds.Filename,
			"file_size":           ds.FileSize,
			"content_type":        ds.ContentType,
			"georeference_status": ds.GeoreferenceStatus,
			"tile_status":         "completed",
			"mbtiles_path":        refs.MBTiles,
			"assets":              refs,
			"extracted_metadata":  ds.ExtractedMetadata,
		}
		if refs.MBTiles == "" {
			layerDoc["tile_status"] = ds.TileStatus
		}
		if opts.IncludeLayerStyles {
			layerDoc["map_layer"] = ds.MapLayer
			if bg, ok := ds.ExtractedMetadata["background_removal"].(map[string]interface{}); ok {
				layerDoc["background_removal"] = bg
			}
			if enh, ok := ds.ExtractedMetadata["enhancement"]; ok && enh != nil {
				layerDoc["enhancement"] = enh
			}
			if tr, ok := ds.ExtractedMetadata["raster_transform"].(map[string]interface{}); ok {
				layerDoc["raster_transform"] = tr
			}
		}
		if opts.IncludeControlPoints {
			layerDoc["control_points"] = layerCPs
		}

		_ = addJSONToZip(layerZipDir+"/layer.json", layerDoc)
	}

	// 3. Build root project.mbtiles (mandatory SQLite 1.3 container for every project .zip)
	stagedRootMBTiles := filepath.Join(stageDir, "project.mbtiles")
	if singleVisibleMBTilesDiskPath != "" && len(compositeSpecs) <= 1 && fileExists(singleVisibleMBTilesDiskPath) {
		_ = storage.CopyFile(singleVisibleMBTilesDiskPath, stagedRootMBTiles)
	} else if len(compositeSpecs) > 0 {
		if cRes, cErr := gis.CreateMBTiles(stagedRootMBTiles, gis.MBTilesCreateSpec{
			Name:        projectName,
			Description: "Veracity Maps Project Composite MBTiles 1.3",
			TileFormat:  "png",
			Layers:      compositeSpecs,
		}); cErr == nil && cRes != nil {
			if cRes.MinZoom < minTileZoom {
				minTileZoom = cRes.MinZoom
			}
			if cRes.MaxZoom > maxTileZoom {
				maxTileZoom = cRes.MaxZoom
			}
		}
	}

	// Fallback if all visible layers were hidden or draft unreferenced: reuse first layer mbtiles or build draft preview
	if !fileExists(stagedRootMBTiles) {
		for _, lID := range resolvedLayerIDs {
			cand := filepath.Join(stageDir, lID+".mbtiles")
			if fileExists(cand) {
				_ = storage.CopyFile(cand, stagedRootMBTiles)
				break
			}
		}
	}
	if !fileExists(stagedRootMBTiles) {
		if dRes, dErr := s.buildUnreferencedDraftMBTiles(primaryDs, stagedRootMBTiles, projectName); dErr == nil && dRes != nil {
			minTileZoom = dRes.MinZoom
			maxTileZoom = dRes.MaxZoom
		}
	}
	if !fileExists(stagedRootMBTiles) || !gis.HasSQLiteHeader(stagedRootMBTiles) {
		return "", "", fmt.Errorf("MBTILES_GENERATION_FAILED: Failed generating valid SQLite project.mbtiles for project archive")
	}
	if err := addDiskFileToZip("project.mbtiles", stagedRootMBTiles); err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed writing project.mbtiles to archive: %w", err)
	}

	// Add tiles/map.mbtiles as declared in MapTiler project package structure
	_ = addDiskFileToZip("tiles/map.mbtiles", stagedRootMBTiles)

	// Unpack composite tiles into {z}/{x}/{y}.png to provide standard custom local tiles for Viewscape,
	// and mirror under tiles/{z}/{x}/{y}.png for MapTiler / standard directory tile structure.
	stageCustomTilesDir := filepath.Join(stageDir, "tiles_custom")
	_ = os.MkdirAll(stageCustomTilesDir, 0755)
	if _, inspErr := gis.InspectMBTiles(stagedRootMBTiles, "", stageCustomTilesDir); inspErr == nil {
		_ = filepath.Walk(stageCustomTilesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(stageCustomTilesDir, path)
			if relErr != nil {
				return nil
			}
			slashRel := filepath.ToSlash(rel)
			_ = addDiskFileToZip(slashRel, path)             // Root {z}/{x}/{y}.png for Viewscape Custom Local Tiles
			return addDiskFileToZip("tiles/"+slashRel, path) // tiles/{z}/{x}/{y}.png
		})
	}

	// Export root main renderable raster (with AI upscaling and/or background transparency) for Viewscapes
	mainRasterPath := s.resolveExportRasterPath(primaryDs)
	if mainRasterPath != "" && fileExists(mainRasterPath) {
		mainRasterExt := strings.ToLower(filepath.Ext(mainRasterPath))
		if mainRasterExt == "" {
			mainRasterExt = ".png"
		}
		_ = addDiskFileToZip("assets/main_raster"+mainRasterExt, mainRasterPath)

		if t, ok := extractAffineTransform(primaryDs.ExtractedMetadata); ok && len(t) == 6 {
			origW := int(getDim(primaryDs.ExtractedMetadata["width"]))
			origH := int(getDim(primaryDs.ExtractedMetadata["height"]))
			actualW, actualH := origW, origH
			if meta, mErr := gis.InspectRasterFile(mainRasterPath); mErr == nil && meta != nil && meta.Width > 0 && meta.Height > 0 {
				actualW, actualH = meta.Width, meta.Height
			}
			scaledT := t
			if origW > 0 && origH > 0 && actualW > 0 && actualH > 0 {
				rx := float64(origW) / float64(actualW)
				ry := float64(origH) / float64(actualH)
				scaledT = []float64{
					t[0] * rx,
					t[1] * ry,
					t[2],
					t[3] * rx,
					t[4] * ry,
					t[5],
				}
			}
			worldContent := fmt.Sprintf("%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n",
				scaledT[0], scaledT[3], scaledT[1], scaledT[4], scaledT[2], scaledT[5],
			)
			_ = addFileToZip("assets/main_raster.pgw", strings.NewReader(worldContent))
			_ = addFileToZip("assets/main_raster.wld", strings.NewReader(worldContent))
			prjWKT := "GEOGCS[\"WGS 84\",DATUM[\"WGS_1984\",SPHEROID[\"WGS 84\",6378137,298.257223563]],PRIMEM[\"Greenwich\",0],UNIT[\"degree\",0.0174532925199433]]\n"
			_ = addFileToZip("assets/main_raster.prj", strings.NewReader(prjWKT))
		}

		// Previews thumbnail
		previewSrc := mainRasterPath
		primaryDir := resolveDatasetDirectory(s.storage, primaryDs)
		if candDisp := filepath.Join(primaryDir, "processed", "display.png"); fileExists(candDisp) {
			previewSrc = candDisp
		} else if candPrev := filepath.Join(primaryDir, "processed", "mbtiles_preview.png"); fileExists(candPrev) {
			previewSrc = candPrev
		} else if candProc := filepath.Join(primaryDir, "processed", "processed_transparent.png"); fileExists(candProc) {
			previewSrc = candProc
		} else if candEnh := filepath.Join(primaryDir, "processed", "enhanced.png"); fileExists(candEnh) {
			previewSrc = candEnh
		}
		_ = addDiskFileToZip("previews/thumbnail.png", previewSrc)
	}

	if minTileZoom > maxTileZoom || minTileZoom == 99 {
		minTileZoom, maxTileZoom = 14, 18
	}

	// 4. Export Buildings and Floors
	var exportedBuildings []*models.Building
	if opts.IncludeBuildingConfig {
		for _, b := range allBuildings {
			if !selectedBuildingSet[b.ID] {
				continue
			}
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
			_ = addJSONToZip("buildings/"+b.ID+"/building.json", bCopy)
			exportedBuildings = append(exportedBuildings, &bCopy)
		}
	}

	// 5. Synchronize MapProject Configuration (preserving visibility, opacity, order, camera, base layer, floors)
	projConfig := make(map[string]interface{})
	if primaryProj != nil && primaryProj.Configuration != nil {
		for k, v := range primaryProj.Configuration {
			projConfig[k] = v
		}
	}

	var exportedMembers []map[string]interface{}
	for idx, lID := range resolvedLayerIDs {
		ds := datasetMap[lID]
		memCopy := make(map[string]interface{})
		if origMem, ok := memberMetaByID[lID]; ok {
			for k, v := range origMem {
				memCopy[k] = v
			}
		}
		memCopy["dataset_id"] = lID
		if memCopy["name"] == nil || memCopy["name"] == "" {
			memCopy["name"] = ds.Filename
		}
		if ds.ExtractedMetadata != nil {
			if memCopy["building_name"] == nil || memCopy["building_name"] == "" {
				memCopy["building_name"] = ds.ExtractedMetadata["building_name"]
			}
			if memCopy["floor_level"] == nil {
				memCopy["floor_level"] = ds.ExtractedMetadata["floor_level"]
			}
			if memCopy["floor_label"] == nil || memCopy["floor_label"] == "" {
				memCopy["floor_label"] = ds.ExtractedMetadata["floor_label"]
			}
			if memCopy["is_building_floor"] == nil {
				memCopy["is_building_floor"] = ds.ExtractedMetadata["is_building_floor"]
			}
		}
		if memCopy["visible"] == nil {
			memCopy["visible"] = true
		}
		if memCopy["raster_visible"] == nil {
			memCopy["raster_visible"] = true
		}
		if memCopy["raster_opacity"] == nil {
			memCopy["raster_opacity"] = 1.0
		}
		if memCopy["control_points_visible"] == nil {
			memCopy["control_points_visible"] = true
		}
		if memCopy["icon_points_visible"] == nil {
			memCopy["icon_points_visible"] = true
		}
		if memCopy["annotations_visible"] == nil {
			memCopy["annotations_visible"] = true
		}
		if memCopy["vectors_visible"] == nil {
			memCopy["vectors_visible"] = true
		}
		memCopy["sort_order"] = idx
		exportedMembers = append(exportedMembers, memCopy)
	}
	projConfig["members"] = exportedMembers

	primaryBounds := extractDatasetBounds(primaryDs)
	if len(primaryBounds) != 4 && len(unionBounds) == 4 {
		primaryBounds = unionBounds
	}

	var centerCoords []float64
	if len(unionBounds) == 4 {
		centerCoords = []float64{(unionBounds[0] + unionBounds[2]) / 2.0, (unionBounds[1] + unionBounds[3]) / 2.0}
	} else if len(primaryBounds) == 4 {
		centerCoords = []float64{(primaryBounds[0] + primaryBounds[2]) / 2.0, (primaryBounds[1] + primaryBounds[3]) / 2.0}
	} else {
		centerCoords = []float64{0.0, 0.0}
	}

	initZoom, camMinZoom, camMaxZoom, zoomPreset := resolveCameraZoomSettings(opts.ZoomConfig, minTileZoom, maxTileZoom, primaryBounds)
	// Preserve explicit camera center/zoom from saved workspace project if user didn't override ZoomConfig
	if opts.ZoomConfig == nil {
		if camMap, ok := projConfig["camera"].(map[string]interface{}); ok {
			if cz := int(math.Round(getDim(camMap["zoom"]))); cz > 0 {
				initZoom = cz
			}
			if cLng, hasLng := camMap["longitude"]; hasLng && camMap["latitude"] != nil {
				lngVal := getDim(cLng)
				latVal := getDim(camMap["latitude"])
				if lngVal != 0 || latVal != 0 {
					centerCoords = []float64{lngVal, latVal}
				}
			}
		}
	}
	projConfig["min_zoom"] = camMinZoom
	projConfig["max_zoom"] = camMaxZoom
	projConfig["initial_zoom"] = initZoom
	if zoomPreset != "" {
		projConfig["zoom_preset"] = zoomPreset
	}

	hasBasemap := false
	if opts.IncludeBaseMapSettings && len(primaryBounds) == 4 {
		packagedBM, bmErr := gis.PackageBasemapTiles(archive, primaryBounds, minTileZoom, maxTileZoom)
		if bmErr == nil && packagedBM > 0 {
			hasBasemap = true
		}
	}

	baseLayerVisible := true
	if bv, ok := projConfig["base_map_visible"].(bool); ok {
		baseLayerVisible = bv
	} else if blMap, ok := projConfig["base_layer"].(map[string]interface{}); ok {
		if vis, ok := blMap["visible"].(bool); ok {
			baseLayerVisible = vis
		}
	}
	baseLayerDoc := map[string]interface{}{
		"type":              "raster-xyz",
		"name":              "OpenStreetMap",
		"visible":           baseLayerVisible,
		"include_settings":  opts.IncludeBaseMapSettings,
		"offline_packaged":  hasBasemap,
		"tile_url_template": "./basemap/{z}/{x}/{y}.png",
		"online_fallback":   "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
		"attribution":       "© OpenStreetMap contributors",
	}
	projConfig["base_layer"] = baseLayerDoc

	// 6. Write GeoJSON vector overlays (landmarks, control points across ALL selected layers, annotations)
	landmarkFeatures := make([]map[string]interface{}, 0, len(allIconPoints))
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
				"dataset_id":  ip.DatasetID,
				"building_id": ip.BuildingID,
				"name":        name,
				"icon_key":    cleanKeyNorm,
				"icon_id":     ip.IconID,
				"icon_path":   ip.IconPath,
				"description": ip.Description,
				"scope":       ip.Scope,
				"floor_level": ip.FloorLevel,
				"floor_label": ip.FloorLabel,
				"longitude":   ip.Longitude,
				"latitude":    ip.Latitude,
			}
			for k, v := range ip.Properties {
				if _, exists := props[k]; !exists {
					props[k] = v
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
	_ = addJSONToZip("layers/landmarks.geojson", map[string]interface{}{
		"type":     "FeatureCollection",
		"features": landmarkFeatures,
	})

	cpFeatures := make([]map[string]interface{}, 0, len(allControlPoints))
	if opts.IncludeControlPoints {
		for _, cp := range allControlPoints {
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
	_ = addJSONToZip("layers/control_points.geojson", map[string]interface{}{
		"type":     "FeatureCollection",
		"features": cpFeatures,
	})

	if allAnnotationFeatures == nil {
		allAnnotationFeatures = []map[string]interface{}{}
	}
	_ = addJSONToZip("layers/annotations.geojson", map[string]interface{}{
		"type":     "FeatureCollection",
		"features": allAnnotationFeatures,
	})

	// 7. Write synchronized metadata, project.json (schema v2), style.json, viewers, README, and manifest.json (v3.0.0)
	_ = addJSONToZip("metadata/layer_config.json", map[string]interface{}{
		"project_id":         exportPrimaryID,
		"primary_dataset_id": exportPrimaryID,
		"project_mbtiles":    "project.mbtiles",
		"layer_mbtiles":      layerMBTilesMap,
		"buildings":          exportedBuildings,
		"layers":             resolvedLayerIDs,
		"layer_asset_refs":   layerAssetRefs,
		"configuration":      projConfig,
	})

	_ = addJSONToZip("metadata/georeferencing.json", map[string]interface{}{
		"crs":                 "EPSG:3857",
		"geographic_crs":      "EPSG:4326",
		"bounds":              unionBounds,
		"primary_bounds":      primaryBounds,
		"center":              centerCoords,
		"control_point_count": len(allControlPoints),
		"control_points":      allControlPoints,
		"extracted_metadata":  primaryDs.ExtractedMetadata,
		"georeference_status": primaryDs.GeoreferenceStatus,
	})

	metaDoc := map[string]interface{}{
		"name":                   projectName,
		"version":                "3.0.0",
		"description":            "Exported from Veracity Maps (MBTiles 1.3 Project Package)",
		"attribution":            "Veracity Maps & OpenStreetMap contributors",
		"type":                   "overlay",
		"format":                 "mbtiles",
		"tile_format":            "png",
		"scheme":                 "xyz",
		"tile_url_template":      "./{z}/{x}/{y}.png",
		"project_mbtiles":        "project.mbtiles",
		"custom_local_tiles":     "./{z}/{x}/{y}.png",
		"main_raster":            "assets/main_raster.png",
		"main_raster_world_file": "assets/main_raster.pgw",
		"bounds":                 unionBounds,
		"center":                 centerCoords,
		"initial_zoom":           initZoom,
		"minzoom":                camMinZoom,
		"maxzoom":                camMaxZoom,
		"raster_minzoom":         minTileZoom,
		"raster_maxzoom":         maxTileZoom,
		"zoom_preset":            zoomPreset,
		"crs":                    "EPSG:3857",
		"control_points":         allControlPoints,
		"icon_points":            allIconPoints,
		"basemap":                baseLayerDoc,
		"export_time":            time.Now().UTC().Format(time.RFC3339),
	}
	_ = addJSONToZip("metadata.json", metaDoc)

	projectDoc := map[string]interface{}{
		"format":             "veracity-project",
		"schema_version":     2,
		"name":               projectName,
		"version":            "3.0.0",
		"project_mbtiles":    "project.mbtiles",
		"custom_local_tiles": "./tiles/{z}/{x}/{y}.png",
		"main_raster":        "assets/main_raster.png",
		"layer_mbtiles":      layerMBTilesMap,
		"project": map[string]interface{}{
			"id":   exportPrimaryID,
			"name": projectName,
		},
		"map": map[string]interface{}{
			"center":         centerCoords,
			"zoom":           initZoom,
			"min_zoom":       camMinZoom,
			"max_zoom":       camMaxZoom,
			"raster_minzoom": minTileZoom,
			"raster_maxzoom": maxTileZoom,
			"preset":         zoomPreset,
			"bounds":         unionBounds,
			"crs":            "EPSG:3857",
		},
		"base_layer": baseLayerDoc,
		"resources": map[string]interface{}{
			"project_mbtiles":        "project.mbtiles",
			"custom_local_tiles":     "./tiles/{z}/{x}/{y}.png",
			"main_raster":            "assets/main_raster.png",
			"main_raster_world_file": "assets/main_raster.pgw",
			"style":                  "style.json",
			"base_map":               "./basemap/{z}/{x}/{y}.png",
			"landmarks":              "layers/landmarks.geojson",
			"control_points":         "layers/control_points.geojson",
			"annotations":            "layers/annotations.geojson",
			"layer_config":           "metadata/layer_config.json",
			"georeferencing":         "metadata/georeferencing.json",
		},
		"primary_dataset_id": exportPrimaryID,
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
	_ = addJSONToZip("project/project.json", projectDoc)
	_ = addJSONToZip("metadata/dataset.json", primaryDs)
	_ = addJSONToZip("metadata/layers.json", resolvedLayerIDs)
	_ = addJSONToZip("metadata/buildings.json", exportedBuildings)
	_ = addJSONToZip("metadata/landmarks.json", allIconPoints)

	styleBytes, _ := generateMapLibreStyleJSON(projectName, unionBounds, minTileZoom, maxTileZoom, 1.0)
	if sw, swErr := archive.Create("style.json"); swErr == nil {
		_, _ = sw.Write(styleBytes)
		seenZipEntries["style.json"] = true
		archiveResources = append(archiveResources, "style.json")
	}

	viewers := generateViewers(ViewscapeViewerParams{
		Title:            projectName,
		Bounds:           unionBounds,
		RasterMinZoom:    minTileZoom,
		RasterMaxZoom:    maxTileZoom,
		InitialZoom:      initZoom,
		MinZoom:          camMinZoom,
		MaxZoom:          camMaxZoom,
		Preset:           zoomPreset,
		Opacity:          1.0,
		ControlPoints:    allControlPoints,
		IconPoints:       allIconPoints,
		BundledIconPaths: iconAssetsCopied,
	})
	for name, htmlContent := range viewers {
		if vw, vwErr := archive.Create(name); vwErr == nil {
			_, _ = vw.Write([]byte(htmlContent))
			seenZipEntries[name] = true
			archiveResources = append(archiveResources, name)
		}
	}

	readmeContent := fmt.Sprintf(`================================================================================
Veracity Maps — Portable MBTiles Project Archive (.zip)
================================================================================
Project:        %s
Export Format:  veracity-project (Schema Version 2 / MBTiles 1.3)
Timestamp:      %s
Layers:         %d
Buildings:      %d

Package Structure:
------------------
- project.mbtiles               SQLite MBTiles 1.3 composite project tile database
- tiles/{z}/{x}/{y}.png         Standard custom local XYZ tiles for offline Viewscapes
- tiles/{layer_id}.mbtiles      Per-layer SQLite MBTiles 1.3 tile databases
- assets/main_raster.png        Final active renderable raster (with AI upscaling / transparency)
- assets/main_raster.pgw        Georeferencing world file aligned with main raster resolution
- project.json / manifest.json  Synchronized project configuration, zoom, base layer & hierarchy
- layers/{layer_id}/layer.json  Per-layer georeferencing, styling, control points, vectors & labels
- layers/landmarks.geojson      GeoJSON landmark icon points
- layers/control_points.geojson GeoJSON ground control points across all exported layers
- layers/annotations.geojson    GeoJSON vector & PaddleOCR label overlays
- assets/icons/                 Bundled custom landmark icon assets
- style.json                    MapLibre GL JS Style Specification v8 (with custom local tiles)
- viewer.html / index.html      Standalone portable viewer supporting custom local tiles and file:///

Running in Viewscapes:
----------------------
1. Custom Local Tiles: Load ./tiles/{z}/{x}/{y}.png directly into any Viewscape or GIS application.
2. MBTiles: Use project.mbtiles or per-layer tiles/{layer_id}.mbtiles in standard MBTiles viewers.
3. Offline Browser: Double-click viewer.html to immediately render the project offline.

Reopening in Veracity Maps:
---------------------------
Upload either this .zip archive or any standalone .mbtiles file directly in Veracity Maps
("Reopen Project") to restore all layers, multi-floor buildings, control points, landmarks,
vector overlays, zoom limits, and visibility states.
`, projectName, time.Now().UTC().Format(time.RFC3339), len(resolvedLayerIDs), len(exportedBuildings))
	if rw, rwErr := archive.Create("README.txt"); rwErr == nil {
		_, _ = rw.Write([]byte(readmeContent))
		seenZipEntries["README.txt"] = true
		archiveResources = append(archiveResources, "README.txt")
	}

	var manifestDatasets []map[string]interface{}
	var manifestLayers []map[string]interface{}
	for _, lID := range resolvedLayerIDs {
		ds := datasetMap[lID]
		if ds == nil {
			continue
		}
		manifestDatasets = append(manifestDatasets, map[string]interface{}{
			"id":                  ds.ID,
			"filename":            ds.Filename,
			"content_type":        ds.ContentType,
			"georeference_status": ds.GeoreferenceStatus,
			"bounds":              extractDatasetBounds(ds),
		})
		manifestLayers = append(manifestLayers, map[string]interface{}{
			"id":          lID,
			"name":        ds.Filename,
			"mbtiles":     layerMBTilesMap[lID],
			"main_raster": layerAssetRefs[lID].MainRaster,
			"assets":      layerAssetRefs[lID],
		})
	}

	manifest := ExportManifest{
		Format:             "veracity-maps-project",
		SchemaVersion:      "1.0",
		Application:        "Veracity Maps",
		ApplicationVersion: "1.3.0",
		FormatVersion:      "3.0.0",
		FormatName:         "veracity-maps-mbtiles-project",
		Generator:          "Veracity Maps Export Engine",
		ExportTimestamp:    time.Now().UTC().Format(time.RFC3339),
		ProjectID:          exportPrimaryID,
		ProjectName:        projectName,
		PrimaryDatasetID:   exportPrimaryID,
		ProjectMBTiles:     "project.mbtiles",
		CustomLocalTiles:   "./tiles/{z}/{x}/{y}.png",
		MainRaster:         "assets/main_raster.png",
		LayerMBTiles:       layerMBTilesMap,
		CRS:                "EPSG:3857",
		Bounds:             unionBounds,
		Center:             centerCoords,
		ZoomConfiguration: map[string]interface{}{
			"initial_zoom": initZoom,
			"min_zoom":     camMinZoom,
			"max_zoom":     camMaxZoom,
			"preset":       zoomPreset,
		},
		Datasets: manifestDatasets,
		Layers:   manifestLayers,
		Assets: map[string]string{
			"mbtiles":      "project.mbtiles",
			"map_mbtiles":  "tiles/map.mbtiles",
			"custom_tiles": "tiles/{z}/{x}/{y}.png",
			"main_raster":  "assets/main_raster.png",
			"thumbnail":    "previews/thumbnail.png",
			"style":        "style.json",
		},
		Project:           projectDoc,
		SelectedBuildings: sortedKeys(selectedBuildingSet),
		SelectedFloors:    sortedKeys(selectedFloorSet),
		SelectedLayers:    resolvedLayerIDs,
		IncludedOptions:   opts,
		Resources:         archiveResources,
	}
	_ = addJSONToZip("manifest.json", manifest)

	_ = archive.Close()
	_ = zipFile.Close()

	if _, vErr := ValidateExport(zipPath, "project_zip", exportPrimaryID); vErr != nil {
		return "", "", vErr
	}

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
