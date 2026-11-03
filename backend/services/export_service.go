package services

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"html"
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

// StorageManager exposes the underlying StorageManager for direct export file lookup.
func (s *ExportService) StorageManager() *storage.StorageManager {
	return s.storage
}

func safeStem(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" {
		base = name
	}
	reg := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	clean := reg.ReplaceAllString(base, "_")
	clean = strings.Trim(clean, "._-")
	if len(clean) > 80 {
		clean = clean[:80]
	}
	if clean == "" {
		return "map_project"
	}
	return clean
}

// resolveExportRasterPath selects the active renderable raster for export (enhanced, transparent, display PNG, or original)
// without modifying dataset.StoragePath.
func (s *ExportService) resolveExportRasterPath(dataset *models.Dataset) string {
	if dataset == nil {
		return ""
	}
	datasetDir := resolveDatasetDirectory(s.storage, dataset)
	processedDir := filepath.Join(datasetDir, "processed")

	transparentPNG := filepath.Join(processedDir, "processed_transparent.png")
	enhancedPNG := filepath.Join(processedDir, "enhanced.png")
	displayPNG := filepath.Join(processedDir, "display.png")
	mbtilesPreview := filepath.Join(processedDir, "mbtiles_preview.png")

	useTransparent := false
	useEnhanced := false
	if dataset.ExtractedMetadata != nil {
		if bg, ok := dataset.ExtractedMetadata["background_removal"].(map[string]interface{}); ok {
			if enabled, _ := bg["enabled"].(bool); enabled {
				useTransparent = true
			}
		}
		if enhMap, ok := dataset.ExtractedMetadata["enhancement"].(map[string]interface{}); ok {
			en, _ := enhMap["enabled"].(bool)
			av, _ := enhMap["active_variant"].(string)
			if en && av != "original" {
				useEnhanced = true
			}
		} else if enhState, ok := dataset.ExtractedMetadata["enhancement"].(*models.EnhancementState); ok && enhState != nil {
			if enhState.Enabled && enhState.ActiveVariant != "original" {
				useEnhanced = true
			}
		}
	}

	if useTransparent && fileExists(transparentPNG) {
		return transparentPNG
	}
	if useEnhanced && fileExists(enhancedPNG) {
		return enhancedPNG
	}

	storageExt := strings.ToLower(filepath.Ext(dataset.StoragePath))
	if storageExt == ".tif" || storageExt == ".tiff" {
		if !fileExists(displayPNG) && fileExists(dataset.StoragePath) {
			_ = gis.ConvertRasterToDisplayPNG(dataset.StoragePath, displayPNG)
		}
		if fileExists(displayPNG) {
			return displayPNG
		}
	}
	if storageExt == ".mbtiles" && fileExists(mbtilesPreview) {
		return mbtilesPreview
	}
	if fileExists(dataset.StoragePath) {
		return dataset.StoragePath
	}
	if dataset.ProcessedPath != "" && fileExists(dataset.ProcessedPath) {
		return dataset.ProcessedPath
	}
	return dataset.StoragePath
}

// buildLayerMBTilesInput builds a gis.MBTilesLayerInput for a georeferenced dataset,
// scaling the affine transform automatically when an AI-upscaled enhanced raster is active,
// and baking in control points and icon points so they display visually in Viewscape and MBTiles viewers.
func (s *ExportService) buildLayerMBTilesInput(sessionID string, dataset *models.Dataset, opacity float64) (*gis.MBTilesLayerInput, error) {
	if dataset == nil {
		return nil, fmt.Errorf("nil dataset")
	}
	bounds := extractDatasetBounds(dataset)
	if len(bounds) != 4 {
		return nil, fmt.Errorf("GEOREFERENCING_REQUIRED: Dataset '%s' must be georeferenced to generate geographic MBTiles", dataset.Filename)
	}

	imgPath := s.resolveExportRasterPath(dataset)
	if imgPath == "" || !fileExists(imgPath) {
		return nil, fmt.Errorf("SOURCE_MISSING: Renderable raster not found for '%s'", dataset.Filename)
	}

	// Burn control points and icon points directly onto a composite copy so that
	// Viewscape, MapLibre, MBTiles, and all external GIS viewers display the marked annotations!
	if s.repo != nil && sessionID != "" {
		cps, _ := s.repo.ListControlPoints(sessionID, dataset.ID)
		ips, _ := s.repo.ListIconPoints(sessionID, dataset.ID, "")
		if len(cps) > 0 || len(ips) > 0 {
			if comp, err := createCompositeRasterWithMarkers(imgPath, cps, ips, bounds, sessionID); err == nil && comp != "" && fileExists(comp) {
				imgPath = comp
			}
		}
	}

	origW := int(getDim(dataset.ExtractedMetadata["width"]))
	origH := int(getDim(dataset.ExtractedMetadata["height"]))
	transform, _ := extractAffineTransform(dataset.ExtractedMetadata)

	if opacity <= 0 || opacity > 1 {
		opacity = 1.0
	}

	return &gis.MBTilesLayerInput{
		ImagePath:      imgPath,
		Bounds:         bounds,
		Transform:      transform,
		OriginalWidth:  origW,
		OriginalHeight: origH,
		Opacity:        opacity,
		Resampling:     recommendedResamplingForDataset(dataset),
	}, nil
}

// buildUnreferencedDraftMBTiles creates a small preview SQLite .mbtiles container for draft layers
// that have not yet been georeferenced, marking veracity_unreferenced=true so reopening never invents coordinates.
func (s *ExportService) buildUnreferencedDraftMBTiles(dataset *models.Dataset, outputPath, title string) (*gis.MBTilesCreateResult, error) {
	imgPath := s.resolveExportRasterPath(dataset)
	if imgPath == "" || !fileExists(imgPath) {
		return nil, fmt.Errorf("SOURCE_MISSING: Renderable image missing for draft dataset '%s'", dataset.Filename)
	}
	w := int(getDim(dataset.ExtractedMetadata["width"]))
	h := int(getDim(dataset.ExtractedMetadata["height"]))
	if w <= 0 || h <= 0 {
		w, h = 1000, 1000
	}
	aspect := float64(w) / float64(h)
	halfW := 0.005 * aspect
	halfH := 0.005
	draftBounds := []float64{-halfW, -halfH, halfW, halfH}
	minZ := 15
	maxZ := 16

	return gis.CreateMBTiles(outputPath, gis.MBTilesCreateSpec{
		Name:        title,
		Description: "Veracity Maps Draft Preview MBTiles (Unreferenced)",
		TileFormat:  "png",
		MinZoom:     &minZ,
		MaxZoom:     &maxZ,
		ExtraMetadata: map[string]interface{}{
			"veracity_unreferenced": "true",
		},
		Layers: []gis.MBTilesLayerInput{
			{
				ImagePath:      imgPath,
				Bounds:         draftBounds,
				OriginalWidth:  w,
				OriginalHeight: h,
				Opacity:        1.0,
				Resampling:     "cubic",
			},
		},
	})
}

// GenerateExport creates an export artifact using the new MBTiles & MBTiles-backed Project ZIP architecture.
func (s *ExportService) GenerateExport(sessionID, datasetID, format string) (string, string, error) {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return "", "", err
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "project_zip"
	}

	exportDir := filepath.Join(s.storage.GetDatasetDir(sessionID, datasetID), "exports")
	_ = os.MkdirAll(exportDir, 0755)

	var outPath, outName string

	switch format {
	case "project_zip", "zip", "project", "complete_project", "mtmap":
		var zoomCfg *ZoomConfiguration
		if dataset.ExtractedMetadata != nil {
			var izPtr, minZPtr, maxZPtr *int
			if iz := int(getDim(dataset.ExtractedMetadata["initial_zoom"])); iz > 0 {
				izPtr = &iz
			}
			if _, hasMin := dataset.ExtractedMetadata["minzoom"]; hasMin {
				mz := int(getDim(dataset.ExtractedMetadata["minzoom"]))
				minZPtr = &mz
			}
			if _, hasMax := dataset.ExtractedMetadata["maxzoom"]; hasMax {
				mz := int(getDim(dataset.ExtractedMetadata["maxzoom"]))
				maxZPtr = &mz
			}
			preset, _ := dataset.ExtractedMetadata["zoom_preset"].(string)
			if izPtr != nil || minZPtr != nil || maxZPtr != nil || preset != "" {
				zoomCfg = &ZoomConfiguration{
					InitialZoom: izPtr,
					MinZoom:     minZPtr,
					MaxZoom:     maxZPtr,
					Preset:      preset,
				}
			}
		}
		opts := DefaultSelectiveExportOptions()
		opts.ZoomConfig = zoomCfg
		outPath, outName, err = s.GenerateSelectiveExport(sessionID, datasetID, SelectiveExportParams{
			Format:  "project_zip",
			Options: opts,
		})
	case "mbtiles":
		outPath, outName, err = s.exportStandaloneMBTiles(sessionID, dataset, nil, "", exportDir, nil)
	case "png", "jpeg", "jpg":
		outPath, outName, err = s.exportRasterWithWorldFile(dataset, exportDir)
	case "geotiff", "tif", "tiff":
		outPath, outName, err = s.exportGeoTIFF(dataset, exportDir)
	case "base_layer", "basemap":
		outPath, outName, err = s.exportBaseLayerFile(dataset, exportDir)
	default:
		return "", "", fmt.Errorf("UNSUPPORTED_FORMAT: Export format '%s' is not supported. Supported: project_zip, mbtiles, base_layer, geotiff, png, jpeg", format)
	}

	if err != nil {
		return "", "", err
	}

	if _, vErr := ValidateExport(outPath, format, datasetID); vErr != nil {
		return "", "", vErr
	}

	return outPath, outName, nil
}

// exportStandaloneMBTiles generates a genuine SQLite 1.3 .mbtiles database file (never .mbtiles.zip).
func (s *ExportService) exportStandaloneMBTiles(sessionID string, primaryDs *models.Dataset, selectedLayers []*models.Dataset, customName, exportDir string, opts *SelectiveExportOptions) (string, string, error) {
	title := strings.TrimSpace(customName)
	if title == "" {
		if proj, _ := s.repo.GetMapProject(sessionID, primaryDs.ID); proj != nil && proj.Name != "" {
			title = proj.Name
		} else {
			title = strings.TrimSuffix(primaryDs.Filename, filepath.Ext(primaryDs.Filename))
		}
	}
	stem := safeStem(title)
	outPath := filepath.Join(exportDir, stem+".mbtiles")

	// Fast path: single standalone uploaded .mbtiles file
	if len(selectedLayers) <= 1 && strings.EqualFold(filepath.Ext(primaryDs.StoragePath), ".mbtiles") &&
		fileExists(primaryDs.StoragePath) && gis.HasSQLiteHeader(primaryDs.StoragePath) {
		if err := storage.CopyFile(primaryDs.StoragePath, outPath); err != nil {
			return "", "", fmt.Errorf("STORAGE_ERROR: Failed copying standalone MBTiles: %w", err)
		}
		if _, vErr := ValidateExport(outPath, "mbtiles", primaryDs.ID); vErr != nil {
			return "", "", vErr
		}
		return outPath, stem + ".mbtiles", nil
	}

	targets := selectedLayers
	if len(targets) == 0 {
		targets = []*models.Dataset{primaryDs}
	}

	// Read member opacities/visibilities from MapProject if available
	opacityByID := make(map[string]float64)
	if proj, _ := s.repo.GetMapProject(sessionID, primaryDs.ID); proj != nil && proj.Configuration != nil {
		if rawMembers, ok := proj.Configuration["members"].([]interface{}); ok {
			for _, rm := range rawMembers {
				if m, ok := rm.(map[string]interface{}); ok {
					mID, _ := m["dataset_id"].(string)
					if op := getDim(m["raster_opacity"]); op > 0 && op <= 1.0 {
						opacityByID[mID] = op
					}
				}
			}
		}
	}

	var specs []gis.MBTilesLayerInput
	for _, ds := range targets {
		if ds == nil {
			continue
		}
		// If one of the layers is already a standalone .mbtiles and is the sole target, copy directly
		if len(targets) == 1 && ds.TilePath != "" && fileExists(ds.TilePath) && gis.HasSQLiteHeader(ds.TilePath) &&
			strings.EqualFold(filepath.Ext(ds.StoragePath), ".mbtiles") {
			if err := storage.CopyFile(ds.TilePath, outPath); err != nil {
				return "", "", fmt.Errorf("STORAGE_ERROR: %w", err)
			}
			return outPath, stem + ".mbtiles", nil
		}

		op := opacityByID[ds.ID]
		if op <= 0 {
			op = 1.0
		}
		spec, err := s.buildLayerMBTilesInput(sessionID, ds, op)
		if err != nil {
			continue
		}
		specs = append(specs, *spec)
	}

	if len(specs) == 0 {
		return "", "", fmt.Errorf("GEOREFERENCING_REQUIRED: At least one selected dataset must be georeferenced to export standalone .mbtiles")
	}

	var minZ, maxZ, initZ *int
	if opts != nil && opts.ZoomConfig != nil {
		minZ = opts.ZoomConfig.MinZoom
		maxZ = opts.ZoomConfig.MaxZoom
		initZ = opts.ZoomConfig.InitialZoom
	} else if primaryDs.ExtractedMetadata != nil {
		if iz := int(getDim(primaryDs.ExtractedMetadata["initial_zoom"])); iz > 0 {
			initZ = &iz
		}
		if mz := int(getDim(primaryDs.ExtractedMetadata["minzoom"])); mz > 0 {
			minZ = &mz
		}
		if mz := int(getDim(primaryDs.ExtractedMetadata["maxzoom"])); mz > 0 {
			maxZ = &mz
		}
	}

	createSpec := gis.MBTilesCreateSpec{
		Name:        title,
		Description: "Exported from Veracity Maps (MBTiles 1.3)",
		TileFormat:  "png",
		MinZoom:     minZ,
		MaxZoom:     maxZ,
		InitialZoom: initZ,
		Layers:      specs,
	}
	if opts != nil && opts.IncludeBaseMapSettings {
		createSpec.ExtraMetadata = map[string]interface{}{
			"include_basemap": true,
		}
	}

	res, err := gis.CreateMBTiles(outPath, createSpec)
	if err != nil {
		return "", "", fmt.Errorf("MBTILES_EXPORT_FAILED: %w", err)
	}
	if res.TileCount <= 0 || !fileExists(outPath) || !gis.HasSQLiteHeader(outPath) {
		return "", "", fmt.Errorf("MBTILES_VALIDATION_FAILED: Generated MBTiles database failed validation")
	}

	if _, vErr := ValidateExport(outPath, "mbtiles", primaryDs.ID); vErr != nil {
		return "", "", vErr
	}

	return outPath, stem + ".mbtiles", nil
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

	imgToExport := s.resolveExportRasterPath(dataset)
	srcFile, err := os.Open(imgToExport)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Source image not found: %w", err)
	}
	defer srcFile.Close()

	baseName := filepath.Base(imgToExport)
	imgWriter, _ := archive.Create(baseName)
	_, _ = io.Copy(imgWriter, srcFile)

	if t, ok := extractAffineTransform(dataset.ExtractedMetadata); ok && len(t) == 6 {
		origW := int(getDim(dataset.ExtractedMetadata["width"]))
		origH := int(getDim(dataset.ExtractedMetadata["height"]))
		actualW, actualH := origW, origH
		if meta, err := gis.InspectRasterFile(imgToExport); err == nil && meta != nil && meta.Width > 0 && meta.Height > 0 {
			actualW, actualH = meta.Width, meta.Height
		}
		if origW > 0 && origH > 0 && actualW > 0 && actualH > 0 {
			rx := float64(origW) / float64(actualW)
			ry := float64(origH) / float64(actualH)
			t = []float64{
				t[0] * rx,
				t[1] * ry,
				t[2],
				t[3] * rx,
				t[4] * ry,
				t[5],
			}
		}

		baseExt := strings.ToLower(filepath.Ext(imgToExport))
		worldExt := ".wld"
		switch baseExt {
		case ".png":
			worldExt = ".pgw"
		case ".jpg", ".jpeg":
			worldExt = ".jgw"
		case ".tif", ".tiff":
			worldExt = ".tfw"
		}
		worldName := strings.TrimSuffix(baseName, baseExt) + worldExt
		content := fmt.Sprintf("%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n%.10f\n",
			t[0], t[3], t[1], t[4], t[2], t[5],
		)
		ww, _ := archive.Create(worldName)
		_, _ = ww.Write([]byte(content))

		prjContent := "GEOGCS[\"WGS 84\",DATUM[\"WGS_1984\",SPHEROID[\"WGS 84\",6378137,298.257223563]],PRIMEM[\"Greenwich\",0],UNIT[\"degree\",0.0174532925199433]]\n"
		prjName := strings.TrimSuffix(baseName, baseExt) + ".prj"
		if prjW, err := archive.Create(prjName); err == nil {
			_, _ = prjW.Write([]byte(prjContent))
		}

		// Also provide main_raster.png and main_raster.pgw for universal Viewscape compatibility
		if !strings.EqualFold(baseName, "main_raster.png") && !strings.EqualFold(baseName, "main_raster"+baseExt) {
			if mrW, err := archive.Create("main_raster" + baseExt); err == nil {
				if f, err := os.Open(imgToExport); err == nil {
					_, _ = io.Copy(mrW, f)
					_ = f.Close()
				}
			}
			if mrWld, err := archive.Create("main_raster" + worldExt); err == nil {
				_, _ = mrWld.Write([]byte(content))
			}
			if mrPrj, err := archive.Create("main_raster.prj"); err == nil {
				_, _ = mrPrj.Write([]byte(prjContent))
			}
		}
	}

	_ = archive.Close()
	_ = zipFile.Close()

	if _, vErr := ValidateExport(zipPath, "png", dataset.ID); vErr != nil {
		return "", "", vErr
	}

	return zipPath, stem + "_georeferenced.zip", nil
}

func (s *ExportService) exportGeoTIFF(dataset *models.Dataset, exportDir string) (string, string, error) {
	bounds := extractDatasetBounds(dataset)
	if len(bounds) != 4 {
		return "", "", fmt.Errorf("GEOREFERENCING_REQUIRED: Dataset must be georeferenced before exporting to GeoTIFF")
	}

	stem := safeStem(dataset.Filename)
	outTif := filepath.Join(exportDir, stem+".tif")

	imgPath := s.resolveExportRasterPath(dataset)
	transform, _ := extractAffineTransform(dataset.ExtractedMetadata)
	origW := int(getDim(dataset.ExtractedMetadata["width"]))
	origH := int(getDim(dataset.ExtractedMetadata["height"]))
	actualW, actualH := origW, origH
	if meta, err := gis.InspectRasterFile(imgPath); err == nil && meta != nil && meta.Width > 0 && meta.Height > 0 {
		actualW, actualH = meta.Width, meta.Height
	}
	if len(transform) == 6 && origW > 0 && origH > 0 && actualW > 0 && actualH > 0 {
		rx := float64(origW) / float64(actualW)
		ry := float64(origH) / float64(actualH)
		transform = []float64{
			transform[0] * rx,
			transform[1] * ry,
			transform[2],
			transform[3] * rx,
			transform[4] * ry,
			transform[5],
		}
	}

	if err := gis.OptimizeGeoTIFF(imgPath, outTif, bounds, transform, recommendedResamplingForDataset(dataset)); err == nil && fileExists(outTif) {
		if _, vErr := ValidateExport(outTif, "geotiff", dataset.ID); vErr != nil {
			return "", "", vErr
		}
		return outTif, stem + ".tif", nil
	}

	if dataset.GeoreferencedPath != "" && strings.HasSuffix(strings.ToLower(dataset.GeoreferencedPath), ".tif") && fileExists(dataset.GeoreferencedPath) {
		if _, vErr := ValidateExport(dataset.GeoreferencedPath, "geotiff", dataset.ID); vErr != nil {
			return "", "", vErr
		}
		return dataset.GeoreferencedPath, filepath.Base(dataset.GeoreferencedPath), nil
	}

	return "", "", fmt.Errorf("GEOTIFF_EXPORT_FAILED: Unable to generate optimized GeoTIFF")
}

func (s *ExportService) exportBaseLayerFile(dataset *models.Dataset, exportDir string) (string, string, error) {
	boundsList := extractDatasetBounds(dataset)
	if len(boundsList) < 4 {
		boundsList = []float64{-180.0, -85.0511, 180.0, 85.0511}
	}

	zipPath := filepath.Join(exportDir, "veracity-base-map.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", "", fmt.Errorf("STORAGE_ERROR: Failed creating base map archive: %w", err)
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	minZoom := 14
	maxZoom := 18
	if boundsList[0] == -180.0 && boundsList[2] == 180.0 {
		minZoom = 0
		maxZoom = 4
	}

	packagedCount, _ := gis.PackageBasemapTiles(archive, boundsList, minZoom, maxZoom)

	center := []float64{(boundsList[0] + boundsList[2]) / 2.0, (boundsList[1] + boundsList[3]) / 2.0}
	metadataDoc := map[string]interface{}{
		"name":           "veracity-base-map",
		"format":         "png",
		"type":           "baselayer",
		"bounds":         boundsList,
		"center":         []interface{}{center[0], center[1], minZoom},
		"minzoom":        minZoom,
		"maxzoom":        maxZoom,
		"attribution":    "© OpenStreetMap contributors",
		"tile_count":     packagedCount,
		"tile_template":  "./basemap/{z}/{x}/{y}.png",
		"reusable_scope": "Independent offline base layer for compatible Viewscape applications.",
	}
	metaBytes, _ := json.MarshalIndent(metadataDoc, "", "  ")
	mw, _ := archive.Create("metadata.json")
	_, _ = mw.Write(metaBytes)

	styleDoc := MapLibreStyle{
		Version: 8,
		Name:    "Veracity Offline Base Map",
		Sources: map[string]interface{}{
			"offline-basemap": map[string]interface{}{
				"type":        "raster",
				"tiles":       []string{"./basemap/{z}/{x}/{y}.png"},
				"tileSize":    256,
				"minzoom":     minZoom,
				"maxzoom":     maxZoom,
				"attribution": "© OpenStreetMap contributors",
			},
		},
		Layers: []map[string]interface{}{
			{
				"id":     "offline-basemap-layer",
				"type":   "raster",
				"source": "offline-basemap",
			},
		},
	}
	styleBytes, _ := json.MarshalIndent(styleDoc, "", "  ")
	sw, _ := archive.Create("style.json")
	_, _ = sw.Write(styleBytes)

	readme := fmt.Sprintf(`Veracity Maps — Reusable Offline Base Layer
============================================
Layer Name:  veracity-base-map
Type:        Tile-based raster base map
Bounds:      [%.6f, %.6f, %.6f, %.6f]
Zoom Range:  %d to %d
Attribution: © OpenStreetMap contributors
`, boundsList[0], boundsList[1], boundsList[2], boundsList[3], minZoom, maxZoom)
	rw, _ := archive.Create("README.txt")
	_, _ = rw.Write([]byte(readme))

	return zipPath, "veracity-base-map.zip", nil
}

// MapLibreStyle defines a MapLibre GL JS Style Specification (version 8) document.
type MapLibreStyle struct {
	Version  int                      `json:"version"`
	Name     string                   `json:"name"`
	Metadata map[string]interface{}   `json:"metadata,omitempty"`
	Sources  map[string]interface{}   `json:"sources"`
	Layers   []map[string]interface{} `json:"layers"`
}

func extractDatasetBounds(dataset *models.Dataset) []float64 {
	if dataset == nil {
		return nil
	}
	if dataset.ExtractedMetadata != nil {
		if b, ok := dataset.ExtractedMetadata["bounds"].(map[string]float64); ok {
			if norm, err := gis.NormalizeBounds(b); err == nil && len(norm) == 4 {
				return norm
			}
		}
		if bMap, ok := dataset.ExtractedMetadata["bounds"].(map[string]interface{}); ok {
			m := make(map[string]float64)
			for k, v := range bMap {
				m[k] = getDim(v)
			}
			if norm, err := gis.NormalizeBounds(m); err == nil && len(norm) == 4 {
				return norm
			}
		}
		if bList, ok := dataset.ExtractedMetadata["bounds"].([]float64); ok && len(bList) == 4 {
			return bList
		}
		if bList, ok := dataset.ExtractedMetadata["bounds"].([]interface{}); ok && len(bList) == 4 {
			res := make([]float64, 0, 4)
			for _, item := range bList {
				res = append(res, getDim(item))
			}
			if len(res) == 4 && res[2] > res[0] && res[3] > res[1] {
				return res
			}
		}
	}
	if dataset.MapLayer != nil {
		if bList, ok := dataset.MapLayer["bounds"].([]float64); ok && len(bList) == 4 {
			return bList
		}
		if bList, ok := dataset.MapLayer["bounds"].([]interface{}); ok && len(bList) == 4 {
			res := make([]float64, 0, 4)
			for _, item := range bList {
				res = append(res, getDim(item))
			}
			if len(res) == 4 && res[2] > res[0] && res[3] > res[1] {
				return res
			}
		}
		if b, ok := dataset.MapLayer["bounds"].(map[string]float64); ok {
			if norm, err := gis.NormalizeBounds(b); err == nil && len(norm) == 4 {
				return norm
			}
		}
		if bMap, ok := dataset.MapLayer["bounds"].(map[string]interface{}); ok {
			m := make(map[string]float64)
			for k, v := range bMap {
				m[k] = getDim(v)
			}
			if norm, err := gis.NormalizeBounds(m); err == nil && len(norm) == 4 {
				return norm
			}
		}
	}
	return nil
}

func generateMapLibreStyleJSON(title string, bounds []float64, minZoom, maxZoom int, opacity float64) ([]byte, error) {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(bounds) == 4 {
		west, south, east, north = bounds[0], bounds[1], bounds[2], bounds[3]
	}
	if opacity <= 0 || opacity > 1 {
		opacity = 1.0
	}

	style := MapLibreStyle{
		Version: 8,
		Name:    title,
		Metadata: map[string]interface{}{
			"maputnik:renderer":       "mlgljs",
			"veracity:tile_container": "project.mbtiles",
		},
		Sources: map[string]interface{}{
			"osm": map[string]interface{}{
				"type":        "raster",
				"tiles":       []string{"https://tile.openstreetmap.org/{z}/{x}/{y}.png"},
				"tileSize":    256,
				"attribution": "&copy; OpenStreetMap contributors",
				"maxzoom":     19,
			},
			"overlay": map[string]interface{}{
				"type":        "raster",
				"tiles":       []string{"./tiles/{z}/{x}/{y}.png"},
				"url":         "mbtiles://./project.mbtiles",
				"tileSize":    256,
				"minzoom":     minZoom,
				"maxzoom":     maxZoom,
				"bounds":      []float64{west, south, east, north},
				"attribution": "Veracity Maps MBTiles 1.3",
			},
			"landmarks": map[string]interface{}{
				"type": "geojson",
				"data": "./layers/landmarks.geojson",
			},
			"annotations": map[string]interface{}{
				"type": "geojson",
				"data": "./layers/annotations.geojson",
			},
		},
		Layers: []map[string]interface{}{
			{
				"id":      "osm-tiles",
				"type":    "raster",
				"source":  "osm",
				"minzoom": 0,
				"maxzoom": 19,
			},
			{
				"id":      "overlay-tiles",
				"type":    "raster",
				"source":  "overlay",
				"minzoom": minZoom,
				"maxzoom": maxZoom,
				"paint": map[string]interface{}{
					"raster-opacity": opacity,
				},
			},
			{
				"id":     "annotation-labels",
				"type":   "symbol",
				"source": "annotations",
				"layout": map[string]interface{}{
					"text-field": []interface{}{"get", "text"},
					"text-size":  12,
				},
				"paint": map[string]interface{}{
					"text-color":      "#0f172a",
					"text-halo-color": "#ffffff",
					"text-halo-width": 1.75,
				},
			},
		},
	}

	return json.MarshalIndent(style, "", "  ")
}

type ViewscapeViewerParams struct {
	Title            string
	Bounds           []float64
	RasterMinZoom    int
	RasterMaxZoom    int
	InitialZoom      int
	MinZoom          int
	MaxZoom          int
	Preset           string
	Opacity          float64
	ControlPoints    []*models.ControlPoint
	IconPoints       []*models.IconPoint
	BundledIconPaths map[string]string
}

var PresetToZoom = map[string]int{
	"house":          21,
	"building":       20,
	"street":         19,
	"neighborhood":   18,
	"local":          17,
	"district":       16,
	"city":           15,
	"large_city":     14,
	"regional":       13,
	"wide_regional":  12,
	"state":          11,
	"large_regional": 10,
	"country":        5,
	"world":          1,
}

var ZoomLevelDescriptions = map[int]string{
	21: "House level",
	20: "Building level",
	19: "Street level",
	18: "Neighborhood level",
	17: "Local area",
	16: "District level",
	15: "City level",
	14: "Large city area",
	13: "Regional level",
	12: "Wide regional view",
	11: "State-level overview",
	10: "Large regional overview",
	5:  "Country-level overview",
	1:  "World overview",
}

func resolveCameraZoomSettings(zc *ZoomConfiguration, rasterMinZoom, rasterMaxZoom int, _ []float64) (initZoom, minZoom, maxZoom int, preset string) {
	minZoom = 0
	if rasterMinZoom > 4 {
		minZoom = rasterMinZoom - 4
	}
	maxZoom = 22
	if rasterMaxZoom > 0 {
		maxZoom = rasterMaxZoom + 4
		if maxZoom > 24 {
			maxZoom = 24
		}
	}
	initZoom = 16
	if rasterMinZoom > 0 {
		initZoom = rasterMinZoom + 1
		if initZoom > rasterMaxZoom {
			initZoom = rasterMaxZoom
		}
	}

	if zc != nil {
		if zc.Preset != "" {
			preset = strings.ToLower(strings.TrimSpace(zc.Preset))
			if pz, ok := PresetToZoom[preset]; ok {
				initZoom = pz
			}
		}
		if zc.MinZoom != nil {
			minZoom = *zc.MinZoom
		}
		if zc.MaxZoom != nil {
			maxZoom = *zc.MaxZoom
		}
		if zc.InitialZoom != nil {
			initZoom = *zc.InitialZoom
		}
	}

	if minZoom < 0 {
		minZoom = 0
	}
	if minZoom > 24 {
		minZoom = 24
	}
	if maxZoom < 0 {
		maxZoom = 0
	}
	if maxZoom > 24 {
		maxZoom = 24
	}
	if minZoom > maxZoom {
		minZoom, maxZoom = maxZoom, minZoom
	}
	if initZoom < minZoom {
		initZoom = minZoom
	}
	if initZoom > maxZoom {
		initZoom = maxZoom
	}

	return initZoom, minZoom, maxZoom, preset
}

func bundleReferencedIcons(archive *zip.Writer, iconPoints []*models.IconPoint, sessionDir string) map[string]string {
	bundled := make(map[string]string)
	if len(iconPoints) == 0 {
		return bundled
	}

	searchDirs := []string{
		filepath.Join("frontend", "public", "icons"),
		filepath.Join("..", "frontend", "public", "icons"),
		filepath.Join("..", "..", "frontend", "public", "icons"),
	}
	if sessionDir != "" {
		searchDirs = append(searchDirs,
			filepath.Join(sessionDir, "icons"),
			filepath.Join(sessionDir, "uploads"),
			sessionDir,
		)
	}

	seenEntries := make(map[string]bool)

	for _, pt := range iconPoints {
		if pt.IconID == "" && pt.IconPath == "" {
			continue
		}
		rawKey := pt.IconID
		if rawKey == "" {
			rawKey = filepath.Base(pt.IconPath)
		}
		cleanKey := filepath.Base(rawKey)
		ext := filepath.Ext(cleanKey)
		var candidates []string
		if ext != "" {
			candidates = append(candidates, cleanKey)
		} else {
			for _, e := range []string{".png", ".svg", ".webp", ".ico", ".jpg"} {
				candidates = append(candidates, cleanKey+e)
			}
		}

		var foundPath string
		var foundName string
		if pt.IconPath != "" {
			if fi, err := os.Stat(pt.IconPath); err == nil && !fi.IsDir() {
				foundPath = pt.IconPath
				foundName = filepath.Base(pt.IconPath)
			}
		}

		if foundPath == "" {
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
		}

		cleanKeyNorm := strings.TrimSuffix(cleanKey, ext)
		if foundPath != "" {
			relPath := "assets/icons/" + foundName
			if !seenEntries[relPath] {
				if f, err := os.Open(foundPath); err == nil {
					if w, err := archive.Create(relPath); err == nil {
						_, _ = io.Copy(w, f)
						seenEntries[relPath] = true
					}
					_ = f.Close()
				}
			}
			bundled[cleanKey] = relPath
			bundled[cleanKeyNorm] = relPath
			bundled[rawKey] = relPath
		}
	}
	return bundled
}

func generateViewers(params ViewscapeViewerParams) map[string]string {
	west, south, east, north := -180.0, -85.0, 180.0, 85.0
	if len(params.Bounds) == 4 {
		west, south, east, north = params.Bounds[0], params.Bounds[1], params.Bounds[2], params.Bounds[3]
	}
	escapedTitle := html.EscapeString(params.Title)
	if escapedTitle == "" {
		escapedTitle = "Veracity Maps Project"
	}
	cpBytes, _ := json.Marshal(params.ControlPoints)
	ipBytes, _ := json.Marshal(params.IconPoints)

	viewerHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>%s - Veracity Maps MBTiles Project (MapLibre GL JS)</title>
  <link href="https://unpkg.com/maplibre-gl@4.7.1/dist/maplibre-gl.css" rel="stylesheet" />
  <script src="https://unpkg.com/maplibre-gl@4.7.1/dist/maplibre-gl.js"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/sql-wasm.js"></script>
  <style>
    html, body, #map { margin: 0; padding: 0; width: 100%%; height: 100%%; font-family: system-ui, sans-serif; }
    .panel { position: absolute; top: 12px; left: 12px; z-index: 10; background: rgba(15,23,42,0.94); color: #f8fafc; padding: 14px 16px; border-radius: 12px; max-width: 320px; font-size: 13px; box-shadow: 0 10px 25px rgba(0,0,0,0.3); }
    .panel h1 { margin: 0 0 6px 0; font-size: 15px; }
    .meta { color: #94a3b8; font-size: 12px; line-height: 1.5; margin-bottom: 10px; }
    .ctrl { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 6px; font-size: 12px; }
    .btn { margin-top: 10px; width: 100%%; padding: 6px 10px; border: 1px solid #334155; border-radius: 6px; background: #1e293b; color: #f8fafc; cursor: pointer; font-size: 12px; }
    .btn:hover { background: #334155; }
  </style>
</head>
<body>
  <div class="panel">
    <h1>%s</h1>
    <div class="meta">
      Engine: MapLibre GL JS | Container: <code>project.mbtiles</code><br/>
      Zoom: %d (range %d - %d) | Tiles: %d - %d
    </div>
    <label class="ctrl">
      <span>Overlay Visible</span>
      <input id="visibility-toggle" type="checkbox" checked />
    </label>
    <label class="ctrl">
      <span>Opacity</span>
      <input id="opacity-slider" type="range" min="0" max="1" step="0.05" value="1" />
    </label>
    <label class="ctrl">
      <span>Control Points Visible</span>
      <input id="control-points-toggle" type="checkbox" checked />
    </label>
    <label class="ctrl">
      <span>Icon Points Visible</span>
      <input id="icon-points-toggle" type="checkbox" checked />
    </label>
    <button id="reset-view-btn" class="btn" type="button">Reset View</button>
  </div>
  <div id="map"></div>
  <script>
    const bounds = [%.8f, %.8f, %.8f, %.8f];
    const center = [(bounds[0] + bounds[2]) / 2, (bounds[1] + bounds[3]) / 2];
    const initialZoom = %d;
    const controlPoints = %s || [];
    const iconPoints = %s || [];

    let mbtilesDbPromise = null;
    if (typeof initSqlJs === 'function' && window.location.protocol !== 'file:') {
      mbtilesDbPromise = initSqlJs({
        locateFile: f => 'https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/' + f
      }).then(async SQL => {
        const res = await fetch('./project.mbtiles');
        if (!res.ok) return null;
        const buf = await res.arrayBuffer();
        return new SQL.Database(new Uint8Array(buf));
      }).catch(() => null);

      maplibregl.addProtocol('mbtiles', async (params) => {
        const db = await mbtilesDbPromise;
        if (!db) return { data: new Uint8Array() };
        const m = params.url.match(/\/(\d+)\/(\d+)\/(\d+)$/);
        if (!m) return { data: new Uint8Array() };
        const z = Number(m[1]), x = Number(m[2]);
        const tmsY = ((1 << z) - 1) - Number(m[3]);
        const stmt = db.prepare('SELECT tile_data FROM tiles WHERE zoom_level = ? AND tile_column = ? AND tile_row = ? LIMIT 1');
        stmt.bind([z, x, tmsY]);
        let data = new Uint8Array();
        if (stmt.step()) {
          data = stmt.get()[0];
        }
        stmt.free();
        return { data: data.buffer ? data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength) : data };
      });
    }

    const map = new maplibregl.Map({
      container: 'map',
      center: center,
      zoom: %d,
      minZoom: %d,
      maxZoom: %d,
      style: {
        version: 8,
        sources: {
          osm: {
            type: 'raster',
            tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
            tileSize: 256,
            attribution: '&copy; OpenStreetMap contributors'
          }
        },
        layers: [{ id: 'osm-tiles', type: 'raster', source: 'osm' }]
      }
    });
    map.addControl(new maplibregl.NavigationControl(), 'top-right');

    map.on('load', () => {
      const tileSourceUrl = mbtilesDbPromise ? 'mbtiles://project/{z}/{x}/{y}' : './tiles/{z}/{x}/{y}.png';
      map.addSource('overlay', {
        type: 'raster',
        tiles: [tileSourceUrl],
        tileSize: 256,
        bounds: bounds,
        minzoom: %d,
        maxzoom: %d
      });
      map.addLayer({
        id: 'overlay-tiles',
        type: 'raster',
        source: 'overlay',
        paint: { 'raster-opacity': 1.0 }
      });

      if (window.location.protocol === 'file:') {
        map.addSource('overlay-image', {
          type: 'image',
          url: './assets/main_raster.png',
          coordinates: [
            [bounds[0], bounds[3]],
            [bounds[2], bounds[3]],
            [bounds[2], bounds[1]],
            [bounds[0], bounds[1]]
          ]
        });
        map.addLayer({
          id: 'overlay-image',
          type: 'raster',
          source: 'overlay-image',
          paint: { 'raster-opacity': 1.0 }
        });
      }

      if (controlPoints.length > 0) {
        map.addSource('control-points', {
          type: 'geojson',
          data: {
            type: 'FeatureCollection',
            features: controlPoints.map(cp => ({
              type: 'Feature',
              geometry: { type: 'Point', coordinates: [cp.longitude, cp.latitude] },
              properties: cp
            }))
          }
        });
        map.addLayer({
          id: 'control-points-layer',
          type: 'circle',
          source: 'control-points',
          paint: { 'circle-radius': 6, 'circle-color': '#16a34a', 'circle-stroke-width': 2, 'circle-stroke-color': '#ffffff' }
        });
        map.on('click', 'control-points-layer', (e) => {
          const p = e.features[0].properties;
          new maplibregl.Popup().setLngLat(e.lngLat).setHTML('<strong>GCP</strong><br/>Lon: ' + p.longitude + '<br/>Lat: ' + p.latitude).addTo(map);
        });
      }

      if (iconPoints.length > 0) {
        map.addSource('icon-points', {
          type: 'geojson',
          data: {
            type: 'FeatureCollection',
            features: iconPoints.map(ip => ({
              type: 'Feature',
              geometry: { type: 'Point', coordinates: [ip.longitude, ip.latitude] },
              properties: ip
            }))
          }
        });
        map.addLayer({
          id: 'icon-points-layer',
          type: 'circle',
          source: 'icon-points',
          paint: { 'circle-radius': 7, 'circle-color': '#2563eb', 'circle-stroke-width': 2, 'circle-stroke-color': '#ffffff' }
        });
        map.on('click', 'icon-points-layer', (e) => {
          const p = e.features[0].properties;
          new maplibregl.Popup().setLngLat(e.lngLat).setHTML('<strong>' + (p.name || p.icon_id || 'Landmark') + '</strong><br/>' + (p.description || '')).addTo(map);
        });
      }

      document.getElementById('visibility-toggle').addEventListener('change', (e) => {
        const v = e.target.checked ? 'visible' : 'none';
        if (map.getLayer('overlay-tiles')) {
          map.setLayoutProperty('overlay-tiles', 'visibility', v);
        }
        if (map.getLayer('overlay-image')) {
          map.setLayoutProperty('overlay-image', 'visibility', v);
        }
      });
      document.getElementById('opacity-slider').addEventListener('input', (e) => {
        const op = Number(e.target.value);
        if (map.getLayer('overlay-tiles')) {
          map.setPaintProperty('overlay-tiles', 'raster-opacity', op);
        }
        if (map.getLayer('overlay-image')) {
          map.setPaintProperty('overlay-image', 'raster-opacity', op);
        }
      });
      document.getElementById('control-points-toggle').addEventListener('change', (e) => {
        if (map.getLayer('control-points-layer')) {
          map.setLayoutProperty('control-points-layer', 'visibility', e.target.checked ? 'visible' : 'none');
        }
      });
      document.getElementById('icon-points-toggle').addEventListener('change', (e) => {
        if (map.getLayer('icon-points-layer')) {
          map.setLayoutProperty('icon-points-layer', 'visibility', e.target.checked ? 'visible' : 'none');
        }
      });
      document.getElementById('reset-view-btn').addEventListener('click', () => {
        if (bounds[0] < bounds[2] && bounds[1] < bounds[3]) {
          map.fitBounds([[bounds[0], bounds[1]], [bounds[2], bounds[3]]], { padding: 48, maxZoom: %d });
        } else {
          map.flyTo({ center: center, zoom: initialZoom });
        }
      });
    });
  </script>
</body>
</html>`,
		escapedTitle,
		escapedTitle,
		params.InitialZoom, params.MinZoom, params.MaxZoom,
		params.RasterMinZoom, params.RasterMaxZoom,
		west, south, east, north,
		params.InitialZoom,
		string(cpBytes),
		string(ipBytes),
		params.InitialZoom, params.MinZoom, params.MaxZoom,
		params.RasterMinZoom, params.RasterMaxZoom,
		params.MaxZoom,
	)

	return map[string]string{
		"viewer.html": viewerHTML,
		"index.html":  viewerHTML,
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

	// 1. Draw Control Points (distinct high-visibility green target ring with white crosshair dots)
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

			drawCircle(px, py, halfSize+3, color.RGBA{255, 255, 255, 230})
			drawRing(px, py, halfSize+2, halfSize+4, color.RGBA{30, 41, 59, 200})

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
