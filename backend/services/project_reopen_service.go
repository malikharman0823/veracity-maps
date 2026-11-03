package services

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

// isProjectArchiveZip inspects a .zip file on disk and returns true if it contains project
// metadata (project.json, manifest.json, mapstudio.json, metadata.json, layers/*/layer.json)
// or any .mbtiles database, distinguishing exported project archives from raw image bundles.
func isProjectArchiveZip(zipPath string) (bool, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return false, fmt.Errorf("CORRUPT_ZIP: The ZIP archive could not be opened: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		clean := strings.TrimPrefix(filepath.ToSlash(f.Name), "./")
		lower := strings.ToLower(clean)
		base := strings.ToLower(filepath.Base(clean))

		if strings.HasSuffix(lower, ".mbtiles") {
			return true, nil
		}
		if lower == "project.json" || lower == "mapstudio.json" || lower == "style.json" || lower == "metadata/layer_config.json" {
			return true, nil
		}
		if lower == "manifest.json" {
			rc, rErr := f.Open()
			if rErr == nil {
				var mDoc map[string]interface{}
				decErr := json.NewDecoder(io.LimitReader(rc, 256*1024)).Decode(&mDoc)
				_ = rc.Close()
				if decErr == nil && mDoc != nil {
					if mDoc["format_version"] != nil || mDoc["project_mbtiles"] != nil || mDoc["format_name"] != nil || mDoc["primary_dataset_id"] != nil || mDoc["format"] != nil || mDoc["schema_version"] != nil || mDoc["datasets"] != nil {
						return true, nil
					}
				}
			}
		}
		if strings.HasPrefix(lower, "layers/") && base == "layer.json" {
			return true, nil
		}
	}
	return false, nil
}

// extractAndValidateZip safely extracts a .zip archive into extractDir while enforcing:
// - non-empty archive check
// - path traversal (ZIP-slip) rejection
// - duplicate entry rejection
// - 500 MB uncompressed size guard
func (s *DatasetService) extractAndValidateZip(zipPath, extractDir string) ([]string, error) {
	fi, err := os.Stat(zipPath)
	if err != nil || fi.Size() == 0 {
		return nil, fmt.Errorf("EMPTY_ARCHIVE: Uploaded ZIP archive is empty")
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("CORRUPT_ZIP: The ZIP archive is corrupted or unreadable: %w", err)
	}
	defer zr.Close()

	if len(zr.File) == 0 {
		return nil, fmt.Errorf("EMPTY_ARCHIVE: ZIP archive contains zero entries")
	}
	if len(zr.File) > 5000 {
		return nil, fmt.Errorf("INVALID_ZIP: ZIP archive exceeds maximum entry count (5000)")
	}

	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: Failed creating extraction directory: %w", err)
	}

	seenNormalized := make(map[string]bool)
	var extractedRelPaths []string
	var totalBytes int64

	for _, file := range zr.File {
		rawSlash := filepath.ToSlash(file.Name)
		if strings.Contains(rawSlash, "__MACOSX") || strings.HasPrefix(filepath.Base(rawSlash), "._") || strings.EqualFold(filepath.Base(rawSlash), ".ds_store") {
			continue
		}

		destFull, safeErr := s.storage.SafePath(extractDir, file.Name)
		if safeErr != nil {
			return nil, fmt.Errorf("ZIP_SLIP_ATTACK: Unsafe path '%s' rejected: %w", file.Name, safeErr)
		}

		relNorm, relErr := filepath.Rel(extractDir, destFull)
		if relErr != nil || strings.HasPrefix(relNorm, "..") {
			return nil, fmt.Errorf("ZIP_SLIP_ATTACK: Path '%s' escapes extraction root", file.Name)
		}
		relSlash := filepath.ToSlash(relNorm)

		if file.FileInfo().IsDir() {
			_ = os.MkdirAll(destFull, 0755)
			continue
		}

		normKey := strings.ToLower(relSlash)
		if seenNormalized[normKey] {
			return nil, fmt.Errorf("DUPLICATE_ARCHIVE_ENTRIES: Archive contains duplicate file entry '%s'", relSlash)
		}
		seenNormalized[normKey] = true

		if err := os.MkdirAll(filepath.Dir(destFull), 0755); err != nil {
			return nil, fmt.Errorf("STORAGE_ERROR: Failed creating folder for '%s': %w", relSlash, err)
		}

		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("CORRUPT_ZIP: Failed reading entry '%s': %w", relSlash, err)
		}

		outF, err := os.OpenFile(destFull, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("STORAGE_ERROR: Failed writing entry '%s': %w", relSlash, err)
		}

		written, copyErr := io.Copy(outF, io.LimitReader(rc, 300*1024*1024))
		_ = outF.Close()
		_ = rc.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("CORRUPT_ZIP: Failed decompressing entry '%s': %w", relSlash, copyErr)
		}

		totalBytes += written
		if totalBytes > 500*1024*1024 {
			return nil, fmt.Errorf("ZIP_BOMB: Extracted archive exceeds 500 MB safety limit")
		}
		extractedRelPaths = append(extractedRelPaths, relSlash)
	}

	if len(extractedRelPaths) == 0 {
		return nil, fmt.Errorf("EMPTY_ARCHIVE: ZIP archive contains no extractable files")
	}

	return extractedRelPaths, nil
}

// InspectProjectPackage validates a standalone .mbtiles file or .zip project package
// without modifying session state, returning project metadata and layer previews for the Reopen UI.
func (s *DatasetService) InspectProjectPackage(sessionID string, fileHeader *multipart.FileHeader) (*models.ProjectPackageInspection, error) {
	if fileHeader == nil {
		return nil, fmt.Errorf("MISSING_FILE: No project file provided")
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".mbtiles" && ext != ".zip" {
		return nil, fmt.Errorf("INVALID_FILE_TYPE: Unsupported reopen format '%s'. Please select an .mbtiles or .zip project file", ext)
	}
	if fileHeader.Size == 0 {
		return nil, fmt.Errorf("EMPTY_ARCHIVE: Uploaded file '%s' is empty (0 bytes)", fileHeader.Filename)
	}

	tempRoot := filepath.Join(s.storage.GetSessionDir(sessionID), "temp_pkg_inspect_"+repository.GenerateDatasetID())
	if err := os.MkdirAll(tempRoot, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: %w", err)
	}
	defer func() {
		_ = storage.SafeRemoveAll(tempRoot)
	}()

	stagedFile := filepath.Join(tempRoot, filepath.Base(fileHeader.Filename))
	src, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("UPLOAD_FAILED: %w", err)
	}
	dst, err := os.Create(stagedFile)
	if err != nil {
		_ = src.Close()
		return nil, fmt.Errorf("STORAGE_ERROR: %w", err)
	}
	written, copyErr := io.Copy(dst, src)
	_ = dst.Close()
	_ = src.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("UPLOAD_FAILED: %w", copyErr)
	}
	if written == 0 {
		return nil, fmt.Errorf("EMPTY_ARCHIVE: Uploaded file '%s' is empty", fileHeader.Filename)
	}

	if ext == ".mbtiles" {
		if !gis.HasSQLiteHeader(stagedFile) {
			return nil, fmt.Errorf("INVALID_MBTILES: '%s' is not a valid SQLite MBTiles database", fileHeader.Filename)
		}
		insp, err := gis.InspectMBTiles(stagedFile, "", "")
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(insp.Name)
		if name == "" {
			name = strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename))
		}
		return &models.ProjectPackageInspection{
			Valid:         true,
			PackageType:   "mbtiles",
			Filename:      fileHeader.Filename,
			FileSize:      written,
			ProjectName:   name,
			SchemaVersion: 2,
			Bounds:        insp.Bounds,
			Center:        insp.Center,
			InitialZoom:   insp.InitialZoom,
			MinZoom:       insp.MinZoom,
			MaxZoom:       insp.MaxZoom,
			TileFormat:    insp.TileFormat,
			TileScheme:    insp.Scheme,
			TileCount:     insp.TileCount,
			HasBaseLayer:  true,
			BuildingCount: 1,
			FloorCount:    1,
			LayerCount:    1,
			Layers: []*models.ProjectLayerPreview{
				{
					ID:         "mbtiles_primary",
					Name:       name,
					Type:       "mbtiles",
					TileFormat: insp.TileFormat,
					FloorLevel: 0,
					FloorLabel: "Ground Floor",
					Visible:    true,
					Opacity:    1.0,
					MinZoom:    insp.MinZoom,
					MaxZoom:    insp.MaxZoom,
					Bounds:     insp.Bounds,
					HasMBTiles: true,
				},
			},
		}, nil
	}

	// .zip project inspection
	extractDir := filepath.Join(tempRoot, "extracted")
	relFiles, err := s.extractAndValidateZip(stagedFile, extractDir)
	if err != nil {
		return nil, err
	}

	var mbtilesRelPaths []string
	for _, rf := range relFiles {
		if strings.HasSuffix(strings.ToLower(rf), ".mbtiles") {
			mbtilesRelPaths = append(mbtilesRelPaths, rf)
		}
	}
	if len(mbtilesRelPaths) == 0 {
		return nil, fmt.Errorf("MISSING_MBTILES: Project ZIP '%s' does not contain an .mbtiles tile database. Deprecated loose-tile archives are not supported", fileHeader.Filename)
	}

	primaryMBTilesRel := mbtilesRelPaths[0]
	for _, p := range mbtilesRelPaths {
		if strings.EqualFold(p, "project.mbtiles") || strings.EqualFold(p, "tiles/map.mbtiles") {
			primaryMBTilesRel = p
			break
		}
	}
	primaryMBTilesDisk := filepath.Join(extractDir, filepath.FromSlash(primaryMBTilesRel))
	if !gis.HasSQLiteHeader(primaryMBTilesDisk) {
		return nil, fmt.Errorf("INVALID_MBTILES: Archive entry '%s' is not a valid SQLite MBTiles database", primaryMBTilesRel)
	}
	rootMBInsp, err := gis.InspectMBTiles(primaryMBTilesDisk, "", "")
	if err != nil {
		return nil, err
	}

	var projectDoc map[string]interface{}
	projJSONPath := filepath.Join(extractDir, "project.json")
	if fileExists(projJSONPath) {
		raw, rErr := os.ReadFile(projJSONPath)
		if rErr != nil || json.Unmarshal(raw, &projectDoc) != nil {
			return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: project.json is malformed or unreadable")
		}
	}

	var manifestDoc map[string]interface{}
	manJSONPath := filepath.Join(extractDir, "manifest.json")
	if fileExists(manJSONPath) {
		raw, rErr := os.ReadFile(manJSONPath)
		if rErr != nil || json.Unmarshal(raw, &manifestDoc) != nil {
			return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: manifest.json is malformed or unreadable")
		}
	}

	projName := strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename))
	schemaVer := 2
	bounds := rootMBInsp.Bounds
	center := rootMBInsp.Center
	initZoom := rootMBInsp.InitialZoom
	minZ := rootMBInsp.MinZoom
	maxZ := rootMBInsp.MaxZoom
	hasBaseLayer := true

	memberCfgByID := make(map[string]map[string]interface{})
	var orderedIDs []string
	var buildingCount, floorCount int

	if projectDoc != nil {
		if n, ok := projectDoc["name"].(string); ok && n != "" {
			projName = n
		}
		if sv := int(getDim(projectDoc["schema_version"])); sv > 0 {
			schemaVer = sv
		}
		if mapObj, ok := projectDoc["map"].(map[string]interface{}); ok {
			if _, bArr := parseBoundsMap(mapObj["bounds"]); len(bArr) == 4 {
				bounds = bArr
			}
			if cArr, ok := mapObj["center"].([]interface{}); ok && len(cArr) >= 2 {
				center = []float64{getDim(cArr[0]), getDim(cArr[1])}
			}
			if z := getDim(mapObj["zoom"]); z > 0 {
				initZoom = z
			}
			if _, hasMin := mapObj["min_zoom"]; hasMin {
				minZ = int(getDim(mapObj["min_zoom"]))
			}
			if _, hasMax := mapObj["max_zoom"]; hasMax {
				maxZ = int(getDim(mapObj["max_zoom"]))
			}
		}
		if blds, ok := projectDoc["buildings"].([]interface{}); ok {
			buildingCount = len(blds)
			for _, bRaw := range blds {
				if bm, ok := bRaw.(map[string]interface{}); ok {
					if fls, ok := bm["floors"].([]interface{}); ok {
						floorCount += len(fls)
					}
				}
			}
		}
		if lArr, ok := projectDoc["layers"].([]interface{}); ok {
			for _, lid := range lArr {
				if s, ok := lid.(string); ok && s != "" {
					orderedIDs = append(orderedIDs, s)
				}
			}
		}
		if cfg, ok := projectDoc["configuration"].(map[string]interface{}); ok {
			if members, ok := cfg["members"].([]interface{}); ok {
				for _, rm := range members {
					if m, ok := rm.(map[string]interface{}); ok {
						if idStr, _ := m["dataset_id"].(string); idStr != "" {
							memberCfgByID[idStr] = m
						}
					}
				}
			}
		}
	}

	if len(orderedIDs) == 0 && manifestDoc != nil {
		if n, ok := manifestDoc["project_name"].(string); ok && n != "" {
			projName = n
		}
		if lArr, ok := manifestDoc["selected_layers"].([]interface{}); ok {
			for _, lid := range lArr {
				if s, ok := lid.(string); ok && s != "" {
					orderedIDs = append(orderedIDs, s)
				}
			}
		}
	}

	// Discover layers/*/layer.json if orderedIDs still empty
	if len(orderedIDs) == 0 {
		layersDir := filepath.Join(extractDir, "layers")
		if entries, eErr := os.ReadDir(layersDir); eErr == nil {
			for _, ent := range entries {
				if ent.IsDir() && fileExists(filepath.Join(layersDir, ent.Name(), "layer.json")) {
					orderedIDs = append(orderedIDs, ent.Name())
				}
			}
		}
	}

	var layerPreviews []*models.ProjectLayerPreview
	totalCPs := 0

	for _, lid := range orderedIDs {
		lJSONPath := filepath.Join(extractDir, "layers", lid, "layer.json")
		lp := &models.ProjectLayerPreview{
			ID:         lid,
			Name:       lid,
			Type:       "mbtiles",
			TileFormat: rootMBInsp.TileFormat,
			Visible:    true,
			Opacity:    1.0,
			MinZoom:    minZ,
			MaxZoom:    maxZ,
			Bounds:     bounds,
		}
		if mCfg, ok := memberCfgByID[lid]; ok {
			if n, _ := mCfg["name"].(string); n != "" {
				lp.Name = n
			}
			if vis, ok := mCfg["visible"].(bool); ok {
				lp.Visible = vis
			}
			if op := getDim(mCfg["raster_opacity"]); op > 0 {
				lp.Opacity = op
			}
			lp.FloorLevel = int(getDim(mCfg["floor_level"]))
			lp.FloorLabel, _ = mCfg["floor_label"].(string)
			lp.BuildingName, _ = mCfg["building_name"].(string)
		}
		if fileExists(lJSONPath) {
			if raw, rErr := os.ReadFile(lJSONPath); rErr == nil {
				var lDoc map[string]interface{}
				if json.Unmarshal(raw, &lDoc) == nil {
					if fn, _ := lDoc["filename"].(string); fn != "" && lp.Name == lid {
						lp.Name = fn
					}
					if mbRel, _ := lDoc["mbtiles_path"].(string); mbRel != "" {
						lp.HasMBTiles = fileExists(filepath.Join(extractDir, filepath.FromSlash(mbRel)))
					}
					if assets, ok := lDoc["assets"].(map[string]interface{}); ok {
						origRel, _ := assets["original"].(string)
						vecRel, _ := assets["vectors"].(string)
						srcVecRel, _ := assets["source_vectors"].(string)
						lp.HasSource = origRel != "" && fileExists(filepath.Join(extractDir, filepath.FromSlash(origRel)))
						lp.HasVectors = (vecRel != "" && fileExists(filepath.Join(extractDir, filepath.FromSlash(vecRel)))) ||
							(srcVecRel != "" && fileExists(filepath.Join(extractDir, filepath.FromSlash(srcVecRel))))
					}
					if cps, ok := lDoc["control_points"].([]interface{}); ok {
						lp.ControlPoints = len(cps)
						totalCPs += len(cps)
					}
					if em, ok := lDoc["extracted_metadata"].(map[string]interface{}); ok {
						if _, bArr := parseBoundsMap(em["bounds"]); len(bArr) == 4 {
							lp.Bounds = bArr
						}
						if lp.BuildingName == "" {
							lp.BuildingName, _ = em["building_name"].(string)
						}
						if lp.FloorLabel == "" {
							lp.FloorLabel, _ = em["floor_label"].(string)
						}
					}
				}
			}
		} else {
			lp.HasMBTiles = true
		}
		layerPreviews = append(layerPreviews, lp)
	}

	if len(layerPreviews) == 0 {
		for idx, mbRel := range mbtilesRelPaths {
			mbDisk := filepath.Join(extractDir, filepath.FromSlash(mbRel))
			mbI, iErr := gis.InspectMBTiles(mbDisk, "", "")
			if iErr != nil {
				return nil, iErr
			}
			layerPreviews = append(layerPreviews, &models.ProjectLayerPreview{
				ID:         fmt.Sprintf("mbtiles_%d", idx+1),
				Name:       filepath.Base(mbRel),
				Type:       "mbtiles",
				TileFormat: mbI.TileFormat,
				Visible:    true,
				Opacity:    1.0,
				MinZoom:    mbI.MinZoom,
				MaxZoom:    mbI.MaxZoom,
				Bounds:     mbI.Bounds,
				HasMBTiles: true,
			})
		}
	}

	landmarkCount := 0
	if raw, err := os.ReadFile(filepath.Join(extractDir, "icons", "icon_points.json")); err == nil {
		var arr []interface{}
		if json.Unmarshal(raw, &arr) == nil {
			landmarkCount = len(arr)
		}
	} else if raw, err := os.ReadFile(filepath.Join(extractDir, "layers", "landmarks.geojson")); err == nil {
		var fc struct {
			Features []interface{} `json:"features"`
		}
		if json.Unmarshal(raw, &fc) == nil {
			landmarkCount = len(fc.Features)
		}
	}

	if totalCPs == 0 {
		if raw, err := os.ReadFile(filepath.Join(extractDir, "layers", "control_points.geojson")); err == nil {
			var fc struct {
				Features []interface{} `json:"features"`
			}
			if json.Unmarshal(raw, &fc) == nil {
				totalCPs = len(fc.Features)
			}
		}
	}

	annCount := 0
	if raw, err := os.ReadFile(filepath.Join(extractDir, "layers", "annotations.geojson")); err == nil {
		var fc struct {
			Features []interface{} `json:"features"`
		}
		if json.Unmarshal(raw, &fc) == nil {
			annCount = len(fc.Features)
		}
	}

	if buildingCount == 0 {
		buildingCount = 1
	}
	if floorCount == 0 {
		floorCount = len(layerPreviews)
	}

	return &models.ProjectPackageInspection{
		Valid:             true,
		PackageType:       "project_zip",
		Filename:          fileHeader.Filename,
		FileSize:          written,
		ProjectName:       projName,
		SchemaVersion:     schemaVer,
		Bounds:            bounds,
		Center:            center,
		InitialZoom:       initZoom,
		MinZoom:           minZ,
		MaxZoom:           maxZ,
		TileFormat:        rootMBInsp.TileFormat,
		TileScheme:        rootMBInsp.Scheme,
		TileCount:         rootMBInsp.TileCount,
		HasBaseLayer:      hasBaseLayer,
		BuildingCount:     buildingCount,
		FloorCount:        floorCount,
		LayerCount:        len(layerPreviews),
		ControlPointCount: totalCPs,
		LandmarkCount:     landmarkCount,
		AnnotationCount:   annCount,
		Layers:            layerPreviews,
	}, nil
}

// ReopenProjectUpload handles strict project reopening from POST /map-projects/import or /map-projects/reopen,
// accepting ONLY standalone .mbtiles files or MBTiles-backed .zip project packages.
func (s *DatasetService) ReopenProjectUpload(sessionID string, fileHeader *multipart.FileHeader) (*models.Dataset, error) {
	if _, err := s.repo.GetSession(sessionID); err != nil {
		return nil, repository.ErrSessionNotFound
	}
	if fileHeader == nil {
		return nil, fmt.Errorf("MISSING_FILE: No file provided")
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".mbtiles" && ext != ".zip" {
		return nil, fmt.Errorf("INVALID_FILE_TYPE: Project reopen accepts only .mbtiles or .zip packages (got '%s')", ext)
	}
	if fileHeader.Size == 0 {
		return nil, fmt.Errorf("EMPTY_ARCHIVE: Uploaded file '%s' is empty", fileHeader.Filename)
	}
	if fileHeader.Size > s.cfg.MaxUploadSizeBytes {
		return nil, fmt.Errorf("FILE_TOO_LARGE: Upload size exceeds maximum limit of %d bytes", s.cfg.MaxUploadSizeBytes)
	}

	datasetID := repository.GenerateDatasetID()
	datasetDir := s.storage.GetDatasetDir(sessionID, datasetID)
	if err := os.MkdirAll(datasetDir, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: %w", err)
	}

	srcFile, err := fileHeader.Open()
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("UPLOAD_FAILED: %w", err)
	}
	defer srcFile.Close()

	destPath := filepath.Join(datasetDir, filepath.Base(fileHeader.Filename))
	destFile, err := os.Create(destPath)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("STORAGE_ERROR: %w", err)
	}

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(destFile, hasher), srcFile)
	_ = destFile.Close()
	if err != nil || size == 0 {
		_ = storage.SafeRemoveAll(datasetDir)
		if size == 0 {
			return nil, fmt.Errorf("EMPTY_ARCHIVE: Uploaded file is empty")
		}
		return nil, fmt.Errorf("UPLOAD_FAILED: %w", err)
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))

	switch ext {
	case ".mbtiles":
		return s.ReopenMBTilesFromPath(sessionID, datasetID, datasetDir, destPath, fileHeader.Filename, size, fileHash)
	case ".zip":
		return s.ReopenProjectZipFromPath(sessionID, datasetID, datasetDir, destPath, fileHeader.Filename, size, fileHash)
	default:
		return nil, fmt.Errorf("INVALID_FILE_TYPE: Unsupported reopen format '%s'", ext)
	}
}

// ReopenMBTilesFromPath validates a standalone SQLite 1.3 .mbtiles file, registers it as a Dataset,
// Layer, Building/Floor, and MapProject, and configures fast MapLibre tile serving.
func (s *DatasetService) ReopenMBTilesFromPath(sessionID, datasetID, datasetDir, mbtilesPath, filename string, size int64, fileHash string) (*models.Dataset, error) {
	if size <= 0 || !fileExists(mbtilesPath) {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("EMPTY_MBTILES: MBTiles file is empty or missing")
	}
	if !gis.HasSQLiteHeader(mbtilesPath) {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("INVALID_MBTILES: '%s' is not a valid SQLite 3 MBTiles database", filename)
	}

	processedDir := filepath.Join(datasetDir, "processed")
	_ = os.MkdirAll(processedDir, 0755)
	previewPath := filepath.Join(processedDir, "mbtiles_preview.png")
	xyzCacheDir := filepath.Join(datasetDir, "tiles_xyz")

	insp, err := gis.InspectMBTiles(mbtilesPath, previewPath, xyzCacheDir)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, err
	}

	displayName := strings.TrimSpace(insp.Name)
	if displayName == "" {
		displayName = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	west, south, east, north := insp.Bounds[0], insp.Bounds[1], insp.Bounds[2], insp.Bounds[3]
	boundsMap := map[string]interface{}{
		"min_x": west,
		"min_y": south,
		"max_x": east,
		"max_y": north,
	}
	coords4 := [][]float64{
		{west, north},
		{east, north},
		{east, south},
		{west, south},
	}

	previewW, previewH := 512, 512
	if fileExists(previewPath) {
		if pMeta, pErr := gis.InspectRasterFile(previewPath); pErr == nil && pMeta != nil && pMeta.Width > 0 {
			previewW = pMeta.Width
			previewH = pMeta.Height
		}
	}

	tileURLTemplate := fmt.Sprintf("/datasets/%s/tiles/{z}/{x}/{y}", datasetID)
	mapLayer := map[string]interface{}{
		"id":                "lyr_" + datasetID,
		"name":              displayName,
		"type":              "mbtiles",
		"tile_format":       insp.TileFormat,
		"scheme":            insp.Scheme,
		"tile_url_template": tileURLTemplate,
		"tile_min_zoom":     insp.MinZoom,
		"tile_max_zoom":     insp.MaxZoom,
		"bounds":            boundsMap,
		"bounds_crs":        "EPSG:4326",
		"coordinates":       coords4,
		"vector_layers":     insp.VectorLayers,
	}

	extractedMeta := map[string]interface{}{
		"driver":              "MBTiles",
		"format":              "mbtiles",
		"document_type":       "mbtiles",
		"width":               previewW,
		"height":              previewH,
		"crs":                 "EPSG:4326",
		"tile_crs":            "EPSG:3857",
		"bounds":              boundsMap,
		"center":              insp.Center,
		"initial_zoom":        int(insp.InitialZoom),
		"minzoom":             insp.MinZoom,
		"maxzoom":             insp.MaxZoom,
		"tile_min_zoom":       insp.MinZoom,
		"tile_max_zoom":       insp.MaxZoom,
		"tile_count":          insp.TileCount,
		"tile_format":         insp.TileFormat,
		"tile_scheme":         insp.Scheme,
		"available_zooms":     insp.AvailableZooms,
		"vector_layers":       insp.VectorLayers,
		"georeference_source": "mbtiles_metadata",
		"building_name":       displayName,
		"floor_level":         0,
		"floor_label":         "Ground Floor",
		"is_building_floor":   true,
		"content": &models.ContentComposition{
			HasRaster:       insp.TileFormat != "pbf",
			HasVector:       insp.TileFormat == "pbf",
			HasEmbeddedText: insp.TileFormat == "pbf",
			Classification:  "mbtiles",
		},
	}

	var procPath string
	if fileExists(previewPath) {
		procPath = previewPath
	}

	dataset := &models.Dataset{
		ID:                 datasetID,
		SessionID:          sessionID,
		Filename:           filename,
		FileSize:           size,
		ContentType:        "application/vnd.mapbox-vector-tile",
		FileHash:           fileHash,
		StoragePath:        mbtilesPath,
		ProcessedPath:      procPath,
		GeoreferencedPath:  mbtilesPath,
		TilePath:           mbtilesPath,
		GeoreferenceStatus: "completed",
		TileStatus:         "completed",
		ExtractedMetadata:  extractedMeta,
		MapLayer:           mapLayer,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	if insp.TileFormat != "pbf" {
		dataset.ContentType = "application/x-sqlite3"
	}

	if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, err
	}

	_ = s.repo.SetLayers(sessionID, datasetID, []*models.Layer{
		{
			ID:        "lyr_" + datasetID,
			DatasetID: datasetID,
			Name:      displayName,
			Type:      "mbtiles",
			Visible:   true,
			Opacity:   1.0,
			Bounds:    insp.Bounds,
		},
	})

	bld, _ := s.repo.CreateBuilding(sessionID, &models.Building{
		Name:              displayName,
		DefaultFloorLevel: 0,
		Bounds:            boundsMap,
	})
	if bld != nil {
		_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
			Level:      0,
			Name:       "Ground Floor",
			DatasetIDs: []string{datasetID},
		})
	}

	camMinZ := 0
	if insp.MinZoom > 4 {
		camMinZ = insp.MinZoom - 4
	}
	camMaxZ := insp.MaxZoom + 4
	if camMaxZ > 24 {
		camMaxZ = 24
	}
	if camMaxZ < 18 {
		camMaxZ = 18
	}

	proj := &models.MapProject{
		ID:        "proj_" + repository.GenerateDatasetID(),
		DatasetID: datasetID,
		Name:      displayName,
		Configuration: map[string]interface{}{
			"active_view":       "map",
			"active_dataset_id": datasetID,
			"min_zoom":          camMinZ,
			"max_zoom":          camMaxZ,
			"initial_zoom":      insp.InitialZoom,
			"camera": map[string]interface{}{
				"longitude": insp.Center[0],
				"latitude":  insp.Center[1],
				"center":    insp.Center,
				"zoom":      insp.InitialZoom,
				"bearing":   0.0,
				"pitch":     0.0,
			},
			"base_layer": map[string]interface{}{
				"type":    "raster-xyz",
				"name":    "OpenStreetMap",
				"visible": true,
			},
			"members": []interface{}{
				map[string]interface{}{
					"dataset_id":             datasetID,
					"name":                   displayName,
					"building_name":          displayName,
					"is_building_floor":      true,
					"floor_level":            0,
					"floor_label":            "Ground Floor",
					"visible":                true,
					"raster_visible":         true,
					"raster_opacity":         1.0,
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

	return dataset, nil
}

// ReopenProjectZipFromPath validates and restores an MBTiles-backed .zip project package,
// reconstructing every layer once (without duplication), its SQLite .mbtiles tile database,
// optional source/enhanced rasters, PDF/CAD vector overlays, OCR annotations, control points,
// custom icons & landmarks, multi-floor buildings, and exact camera/zoom/visibility settings.
func (s *DatasetService) ReopenProjectZipFromPath(sessionID, primaryDatasetID, primaryDatasetDir, zipPath, filename string, size int64, fileHash string) (*models.Dataset, error) {
	extractDir := filepath.Join(primaryDatasetDir, "extracted")
	relFiles, err := s.extractAndValidateZip(zipPath, extractDir)
	if err != nil {
		_ = storage.SafeRemoveAll(primaryDatasetDir)
		return nil, err
	}

	// 1. Verify that at least one valid SQLite .mbtiles file exists in the archive
	var mbtilesRelPaths []string
	for _, rf := range relFiles {
		if strings.HasSuffix(strings.ToLower(rf), ".mbtiles") {
			mbtilesRelPaths = append(mbtilesRelPaths, rf)
		}
	}
	if len(mbtilesRelPaths) == 0 {
		_ = storage.SafeRemoveAll(primaryDatasetDir)
		return nil, fmt.Errorf("MISSING_MBTILES: Project archive '%s' does not contain an .mbtiles database. Deprecated loose-tile exports are no longer supported", filename)
	}

	for _, mbRel := range mbtilesRelPaths {
		mbDisk := filepath.Join(extractDir, filepath.FromSlash(mbRel))
		if !gis.HasSQLiteHeader(mbDisk) {
			_ = storage.SafeRemoveAll(primaryDatasetDir)
			return nil, fmt.Errorf("INVALID_MBTILES: Archive entry '%s' is not a valid SQLite 3 MBTiles database", mbRel)
		}
	}

	rootMBTilesRel := mbtilesRelPaths[0]
	for _, p := range mbtilesRelPaths {
		if strings.EqualFold(p, "project.mbtiles") || strings.EqualFold(p, "tiles/map.mbtiles") {
			rootMBTilesRel = p
			break
		}
	}
	rootMBTilesDisk := filepath.Join(extractDir, filepath.FromSlash(rootMBTilesRel))
	rootInsp, err := gis.InspectMBTiles(rootMBTilesDisk, "", "")
	if err != nil {
		_ = storage.SafeRemoveAll(primaryDatasetDir)
		return nil, err
	}

	// 2. Parse project.json, manifest.json, and metadata/layer_config.json safely
	var projectDoc map[string]interface{}
	if pPath := filepath.Join(extractDir, "project.json"); fileExists(pPath) {
		raw, rErr := os.ReadFile(pPath)
		if rErr != nil || json.Unmarshal(raw, &projectDoc) != nil {
			_ = storage.SafeRemoveAll(primaryDatasetDir)
			return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: project.json is corrupted or invalid JSON")
		}
	}

	var manifestDoc map[string]interface{}
	if mPath := filepath.Join(extractDir, "manifest.json"); fileExists(mPath) {
		raw, rErr := os.ReadFile(mPath)
		if rErr != nil || json.Unmarshal(raw, &manifestDoc) != nil {
			_ = storage.SafeRemoveAll(primaryDatasetDir)
			return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: manifest.json is corrupted or invalid JSON")
		}
	}

	var layerConfigDoc map[string]interface{}
	if lcPath := filepath.Join(extractDir, "metadata", "layer_config.json"); fileExists(lcPath) {
		if raw, rErr := os.ReadFile(lcPath); rErr == nil {
			_ = json.Unmarshal(raw, &layerConfigDoc)
		}
	}

	// If the ZIP has NO project metadata JSONs at all and just wraps a standalone .mbtiles file, import it cleanly
	if projectDoc == nil && manifestDoc == nil && layerConfigDoc == nil && len(mbtilesRelPaths) == 1 && !fileExists(filepath.Join(extractDir, "layers")) {
		stagedMB := filepath.Join(primaryDatasetDir, filepath.Base(rootMBTilesRel))
		if err := storage.CopyFile(rootMBTilesDisk, stagedMB); err != nil {
			_ = storage.SafeRemoveAll(primaryDatasetDir)
			return nil, fmt.Errorf("STORAGE_ERROR: %w", err)
		}
		_ = storage.SafeRemoveAll(extractDir)
		fi, _ := os.Stat(stagedMB)
		mbSize := size
		if fi != nil {
			mbSize = fi.Size()
		}
		return s.ReopenMBTilesFromPath(sessionID, primaryDatasetID, primaryDatasetDir, stagedMB, filepath.Base(rootMBTilesRel), mbSize, fileHash)
	}

	projectName := strings.TrimSuffix(filename, filepath.Ext(filename))
	oldPrimaryID := ""
	var savedProjConfig map[string]interface{}
	var rawBuildings []interface{}
	var orderedOldLayerIDs []string
	layerMBTilesByOldID := make(map[string]string)

	if projectDoc != nil {
		if n, _ := projectDoc["name"].(string); n != "" {
			projectName = n
		}
		oldPrimaryID, _ = projectDoc["primary_dataset_id"].(string)
		if cfg, ok := projectDoc["configuration"].(map[string]interface{}); ok {
			savedProjConfig = cfg
		}
		if bArr, ok := projectDoc["buildings"].([]interface{}); ok {
			rawBuildings = bArr
		}
		if lArr, ok := projectDoc["layers"].([]interface{}); ok {
			for _, v := range lArr {
				if s, ok := v.(string); ok && s != "" {
					orderedOldLayerIDs = append(orderedOldLayerIDs, s)
				}
			}
		}
		if lmMap, ok := projectDoc["layer_mbtiles"].(map[string]interface{}); ok {
			for k, v := range lmMap {
				if sv, ok := v.(string); ok && sv != "" {
					layerMBTilesByOldID[k] = sv
				}
			}
		}
	}

	if manifestDoc != nil {
		if projectName == "" {
			projectName, _ = manifestDoc["project_name"].(string)
		}
		if oldPrimaryID == "" {
			oldPrimaryID, _ = manifestDoc["primary_dataset_id"].(string)
		}
		if len(orderedOldLayerIDs) == 0 {
			if lArr, ok := manifestDoc["selected_layers"].([]interface{}); ok {
				for _, v := range lArr {
					if s, ok := v.(string); ok && s != "" {
						orderedOldLayerIDs = append(orderedOldLayerIDs, s)
					}
				}
			}
		}
		if lmMap, ok := manifestDoc["layer_mbtiles"].(map[string]interface{}); ok {
			for k, v := range lmMap {
				if sv, ok := v.(string); ok && sv != "" {
					layerMBTilesByOldID[k] = sv
				}
			}
		}
	}

	if layerConfigDoc != nil {
		if savedProjConfig == nil {
			if cfg, ok := layerConfigDoc["configuration"].(map[string]interface{}); ok {
				savedProjConfig = cfg
			}
		}
		if len(rawBuildings) == 0 {
			if bArr, ok := layerConfigDoc["buildings"].([]interface{}); ok {
				rawBuildings = bArr
			}
		}
	}

	// Discover layers/*/layer.json folders if orderedOldLayerIDs was omitted
	if len(orderedOldLayerIDs) == 0 {
		layersRoot := filepath.Join(extractDir, "layers")
		if entries, eErr := os.ReadDir(layersRoot); eErr == nil {
			for _, ent := range entries {
				if ent.IsDir() && fileExists(filepath.Join(layersRoot, ent.Name(), "layer.json")) {
					orderedOldLayerIDs = append(orderedOldLayerIDs, ent.Name())
				}
			}
			sort.Strings(orderedOldLayerIDs)
		}
	}

	if len(orderedOldLayerIDs) == 0 {
		_ = storage.SafeRemoveAll(primaryDatasetDir)
		return nil, fmt.Errorf("INCOMPLETE_PROJECT: Project ZIP is missing layer definitions (project.json / layers/*/layer.json)")
	}

	// Ensure oldPrimaryID comes first so the returned Dataset is the true primary dataset
	if oldPrimaryID != "" {
		for idx, id := range orderedOldLayerIDs {
			if id == oldPrimaryID && idx > 0 {
				orderedOldLayerIDs = append([]string{oldPrimaryID}, append(orderedOldLayerIDs[:idx], orderedOldLayerIDs[idx+1:]...)...)
				break
			}
		}
	} else {
		oldPrimaryID = orderedOldLayerIDs[0]
	}

	// Track created resources for rollback if any required layer fails validation
	var createdDatasetIDs []string
	var createdDatasetDirs []string
	rollback := func() {
		for _, cid := range createdDatasetIDs {
			_ = s.repo.DeleteDataset(sessionID, cid)
		}
		for _, cdir := range createdDatasetDirs {
			_ = storage.SafeRemoveAll(cdir)
		}
		_ = storage.SafeRemoveAll(primaryDatasetDir)
	}

	// Pre-load global control points fallback (partitioned by old dataset_id)
	globalCPsByOldID := make(map[string][]*models.ControlPoint)
	if rawCPGeo, rErr := os.ReadFile(filepath.Join(extractDir, "layers", "control_points.geojson")); rErr == nil {
		var fc struct {
			Features []struct {
				Properties struct {
					ID        string  `json:"id"`
					DatasetID string  `json:"dataset_id"`
					ImageX    float64 `json:"image_x"`
					ImageY    float64 `json:"image_y"`
					Longitude float64 `json:"longitude"`
					Latitude  float64 `json:"latitude"`
				} `json:"properties"`
			} `json:"features"`
		}
		if json.Unmarshal(rawCPGeo, &fc) == nil {
			for _, f := range fc.Features {
				did := f.Properties.DatasetID
				if did == "" {
					did = oldPrimaryID
				}
				globalCPsByOldID[did] = append(globalCPsByOldID[did], &models.ControlPoint{
					ID:        f.Properties.ID,
					DatasetID: did,
					ImageX:    f.Properties.ImageX,
					ImageY:    f.Properties.ImageY,
					Longitude: f.Properties.Longitude,
					Latitude:  f.Properties.Latitude,
					CreatedAt: time.Now(),
				})
			}
		}
	}

	oldToNewDatasetID := make(map[string]string)
	var restoredDatasets []*models.Dataset

	// 3. Reconstruct each layer ONCE (eliminating the old 2x-4x duplication bug)
	for idx, oldID := range orderedOldLayerIDs {
		newID := primaryDatasetID
		targetDir := primaryDatasetDir
		if idx > 0 {
			newID = repository.GenerateDatasetID()
			targetDir = s.storage.GetDatasetDir(sessionID, newID)
			if mkErr := os.MkdirAll(targetDir, 0755); mkErr != nil {
				rollback()
				return nil, fmt.Errorf("STORAGE_ERROR: %w", mkErr)
			}
			createdDatasetDirs = append(createdDatasetDirs, targetDir)
		}
		oldToNewDatasetID[oldID] = newID

		layerJSONPath := filepath.Join(extractDir, "layers", oldID, "layer.json")
		var layerDoc map[string]interface{}
		if fileExists(layerJSONPath) {
			raw, rErr := os.ReadFile(layerJSONPath)
			if rErr != nil || json.Unmarshal(raw, &layerDoc) != nil {
				rollback()
				return nil, fmt.Errorf("CORRUPT_PROJECT_METADATA: layers/%s/layer.json is corrupted", oldID)
			}
		} else {
			layerDoc = map[string]interface{}{
				"id":                  oldID,
				"filename":            fmt.Sprintf("%s_layer_%d.mbtiles", safeStem(projectName), idx+1),
				"georeference_status": "completed",
			}
		}

		layerFilename, _ := layerDoc["filename"].(string)
		if layerFilename == "" {
			layerFilename = fmt.Sprintf("layer_%d.png", idx+1)
		}
		contentType, _ := layerDoc["content_type"].(string)
		if contentType == "" {
			contentType = "image/png"
		}
		georefStatus, _ := layerDoc["georeference_status"].(string)
		if georefStatus == "" {
			georefStatus = "completed"
		}

		extractedMeta, _ := layerDoc["extracted_metadata"].(map[string]interface{})
		if extractedMeta == nil {
			extractedMeta = make(map[string]interface{})
		}
		assetsMap, _ := layerDoc["assets"].(map[string]interface{})

		procDir := filepath.Join(targetDir, "processed")
		tilesDir := filepath.Join(targetDir, "tiles")
		vecDir := filepath.Join(targetDir, "vector")
		_ = os.MkdirAll(procDir, 0755)
		_ = os.MkdirAll(tilesDir, 0755)

		// 3a. Locate and copy layer's SQLite .mbtiles database
		mbRel, _ := layerDoc["mbtiles_path"].(string)
		if mbRel == "" && assetsMap != nil {
			mbRel, _ = assetsMap["mbtiles"].(string)
		}
		if mbRel == "" {
			mbRel = layerMBTilesByOldID[oldID]
		}
		if mbRel == "" {
			candidateTile := "tiles/" + oldID + ".mbtiles"
			if fileExists(filepath.Join(extractDir, filepath.FromSlash(candidateTile))) {
				mbRel = candidateTile
			} else if len(orderedOldLayerIDs) == 1 {
				mbRel = rootMBTilesRel
			}
		}

		var destMBTilesPath string
		var layerMBInsp *gis.MBTilesInspection
		previewPNG := filepath.Join(procDir, "mbtiles_preview.png")
		xyzCacheDir := filepath.Join(targetDir, "tiles_xyz")

		if mbRel != "" {
			srcMB := filepath.Join(extractDir, filepath.FromSlash(mbRel))
			if fileExists(srcMB) {
				if !gis.HasSQLiteHeader(srcMB) {
					rollback()
					return nil, fmt.Errorf("INVALID_MBTILES: Layer MBTiles '%s' is not a valid SQLite database", mbRel)
				}
				destMBTilesPath = filepath.Join(tilesDir, "layer.mbtiles")
				if cpErr := storage.CopyFile(srcMB, destMBTilesPath); cpErr != nil {
					rollback()
					return nil, fmt.Errorf("STORAGE_ERROR: %w", cpErr)
				}
				insp, iErr := gis.InspectMBTiles(destMBTilesPath, previewPNG, xyzCacheDir)
				if iErr != nil {
					rollback()
					return nil, iErr
				}
				layerMBInsp = insp
				if insp.RawMetadata["veracity_unreferenced"] == "true" && georefStatus != "completed" {
					georefStatus = "not_started"
				}
			}
		}

		// 3b. Restore optional original, enhanced, and transparent rasters + sidecars + vectors + annotations
		var restoredStoragePath string
		var restoredProcessedPath string

		if assetsMap != nil {
			if origRel, _ := assetsMap["original"].(string); origRel != "" {
				srcOrig := filepath.Join(extractDir, filepath.FromSlash(origRel))
				if fileExists(srcOrig) {
					origExt := filepath.Ext(srcOrig)
					destName := layerFilename
					if !strings.EqualFold(filepath.Ext(destName), origExt) && origExt != "" {
						destName = strings.TrimSuffix(destName, filepath.Ext(destName)) + origExt
					}
					destOrig := filepath.Join(targetDir, filepath.Base(destName))
					if storage.CopyFile(srcOrig, destOrig) == nil {
						restoredStoragePath = destOrig
						if strings.EqualFold(origExt, ".tif") || strings.EqualFold(origExt, ".tiff") {
							_ = gis.ConvertRasterToDisplayPNG(destOrig, filepath.Join(procDir, "display.png"))
						}
					}
				}
			}
			if enhRel, _ := assetsMap["enhanced"].(string); enhRel != "" {
				srcEnh := filepath.Join(extractDir, filepath.FromSlash(enhRel))
				destEnh := filepath.Join(procDir, "enhanced.png")
				if fileExists(srcEnh) && storage.CopyFile(srcEnh, destEnh) == nil {
					restoredProcessedPath = destEnh
					if enhObj, ok := extractedMeta["enhancement"].(map[string]interface{}); ok {
						enhObj["enhanced_path"] = destEnh
					}
				}
			}
			if procRel, _ := assetsMap["processed"].(string); procRel != "" {
				srcProc := filepath.Join(extractDir, filepath.FromSlash(procRel))
				if fileExists(srcProc) {
					destProc := filepath.Join(procDir, filepath.Base(procRel))
					if storage.CopyFile(srcProc, destProc) == nil {
						if restoredProcessedPath == "" || strings.EqualFold(filepath.Base(procRel), "processed_transparent.png") {
							restoredProcessedPath = destProc
						}
					}
				}
			}
			if mainRel, _ := assetsMap["main_raster"].(string); mainRel != "" {
				srcMain := filepath.Join(extractDir, filepath.FromSlash(mainRel))
				if fileExists(srcMain) {
					destMain := filepath.Join(procDir, "main_raster.png")
					if storage.CopyFile(srcMain, destMain) == nil {
						if restoredProcessedPath == "" {
							restoredProcessedPath = destMain
						}
					}
				}
			}
			if wldRel, _ := assetsMap["world_file"].(string); wldRel != "" {
				srcWld := filepath.Join(extractDir, filepath.FromSlash(wldRel))
				if fileExists(srcWld) {
					_ = storage.CopyFile(srcWld, filepath.Join(targetDir, filepath.Base(wldRel)))
				}
			}
			if svRel, _ := assetsMap["source_vectors"].(string); svRel != "" {
				srcSV := filepath.Join(extractDir, filepath.FromSlash(svRel))
				if fileExists(srcSV) {
					_ = os.MkdirAll(vecDir, 0755)
					_ = storage.CopyFile(srcSV, filepath.Join(vecDir, "source_vectors.json"))
				}
			}
			if vecRel, _ := assetsMap["vectors"].(string); vecRel != "" {
				srcVec := filepath.Join(extractDir, filepath.FromSlash(vecRel))
				if fileExists(srcVec) {
					_ = os.MkdirAll(vecDir, 0755)
					_ = storage.CopyFile(srcVec, filepath.Join(vecDir, "vectors.geojson"))
				}
			}
			if annRel, _ := assetsMap["annotations"].(string); annRel != "" {
				srcAnn := filepath.Join(extractDir, filepath.FromSlash(annRel))
				if fileExists(srcAnn) {
					_ = os.MkdirAll(vecDir, 0755)
					destAnn := filepath.Join(vecDir, "annotations.json")
					if rawAnn, rErr := os.ReadFile(srcAnn); rErr == nil {
						var anns []*models.DatasetAnnotation
						if json.Unmarshal(rawAnn, &anns) == nil {
							for _, a := range anns {
								if a != nil {
									a.DatasetID = newID
								}
							}
							if b, mErr := json.MarshalIndent(anns, "", "  "); mErr == nil {
								_ = os.WriteFile(destAnn, b, 0644)
							}
						} else {
							_ = storage.CopyFile(srcAnn, destAnn)
						}
					}
				}
			}
		}

		// Ensure StoragePath ALWAYS points to a real existing file on disk so file_missing is never triggered
		if restoredStoragePath == "" {
			if restoredProcessedPath != "" && fileExists(restoredProcessedPath) {
				restoredStoragePath = restoredProcessedPath
			} else if destMBTilesPath != "" && fileExists(destMBTilesPath) {
				restoredStoragePath = destMBTilesPath
				if fileExists(previewPNG) {
					restoredProcessedPath = previewPNG
				}
			} else if fileExists(previewPNG) {
				restoredStoragePath = previewPNG
			}
		}

		// Restore background_removal metadata and ensure processed/processed_transparent.png is restored
		if bgObj, ok := layerDoc["background_removal"].(map[string]interface{}); ok && bgObj != nil {
			if extractedMeta == nil {
				extractedMeta = make(map[string]interface{})
			}
			extractedMeta["background_removal"] = bgObj
		}
		if bgObj, ok := extractedMeta["background_removal"].(map[string]interface{}); ok && bgObj != nil {
			if enabled, _ := bgObj["enabled"].(bool); enabled {
				tPath := filepath.Join(procDir, "processed_transparent.png")
				if !fileExists(tPath) {
					// Fallback search across layer assets or extractDir
					for _, candidate := range []string{
						filepath.Join(extractDir, "layers", oldID, "processed_transparent.png"),
						filepath.Join(extractDir, "layers", oldID, "processed", "processed_transparent.png"),
						filepath.Join(extractDir, "layers", oldID, "main_raster.png"),
						filepath.Join(extractDir, "assets", "main_raster.png"),
						restoredProcessedPath,
					} {
						if candidate != "" && fileExists(candidate) && !strings.EqualFold(filepath.Ext(candidate), ".mbtiles") {
							_ = storage.CopyFile(candidate, tPath)
							break
						}
					}
				}
				if fileExists(tPath) {
					restoredProcessedPath = tPath
				}
			}
		}
		if restoredStoragePath == "" || !fileExists(restoredStoragePath) {
			rollback()
			return nil, fmt.Errorf("INCOMPLETE_PROJECT: Layer '%s' has neither a valid MBTiles archive nor a source raster", layerFilename)
		}

		fi, _ := os.Stat(restoredStoragePath)
		layerSize := int64(getDim(layerDoc["file_size"]))
		if fi != nil && fi.Size() > 0 {
			layerSize = fi.Size()
		}

		// Reconcile bounds & MapLayer with MBTiles metadata
		_, boundsArr := parseBoundsMap(extractedMeta["bounds"])
		if len(boundsArr) != 4 && layerMBInsp != nil && georefStatus == "completed" {
			boundsArr = layerMBInsp.Bounds
			extractedMeta["bounds"] = map[string]interface{}{
				"min_x": boundsArr[0],
				"min_y": boundsArr[1],
				"max_x": boundsArr[2],
				"max_y": boundsArr[3],
			}
		}

		var mapLayer map[string]interface{}
		if savedML, ok := layerDoc["map_layer"].(map[string]interface{}); ok && savedML != nil {
			mapLayer = make(map[string]interface{})
			for k, v := range savedML {
				mapLayer[k] = v
			}
		}
		if georefStatus == "completed" && len(boundsArr) == 4 {
			if mapLayer == nil {
				mapLayer = make(map[string]interface{})
			}
			mapLayer["id"] = "lyr_" + newID
			mapLayer["name"] = layerFilename
			mapLayer["bounds_crs"] = "EPSG:4326"
			if mapLayer["bounds"] == nil {
				mapLayer["bounds"] = map[string]interface{}{
					"min_x": boundsArr[0],
					"min_y": boundsArr[1],
					"max_x": boundsArr[2],
					"max_y": boundsArr[3],
				}
			}
			if mapLayer["coordinates"] == nil {
				w, sB, e, n := boundsArr[0], boundsArr[1], boundsArr[2], boundsArr[3]
				mapLayer["coordinates"] = [][]float64{
					{w, n}, {e, n}, {e, sB}, {w, sB},
				}
			}
			if destMBTilesPath != "" && layerMBInsp != nil {
				mapLayer["type"] = "mbtiles"
				mapLayer["tile_format"] = layerMBInsp.TileFormat
				mapLayer["scheme"] = layerMBInsp.Scheme
				mapLayer["tile_url_template"] = fmt.Sprintf("/datasets/%s/tiles/{z}/{x}/{y}", newID)
				mapLayer["tile_min_zoom"] = layerMBInsp.MinZoom
				mapLayer["tile_max_zoom"] = layerMBInsp.MaxZoom
				if len(layerMBInsp.VectorLayers) > 0 {
					mapLayer["vector_layers"] = layerMBInsp.VectorLayers
				}
				extractedMeta["tile_min_zoom"] = layerMBInsp.MinZoom
				extractedMeta["tile_max_zoom"] = layerMBInsp.MaxZoom
				extractedMeta["tile_count"] = layerMBInsp.TileCount
				extractedMeta["tile_format"] = layerMBInsp.TileFormat
				extractedMeta["tile_scheme"] = layerMBInsp.Scheme
			}
		}

		tileStatus := "not_generated"
		if destMBTilesPath != "" {
			tileStatus = "completed"
		}

		ds := &models.Dataset{
			ID:                 newID,
			SessionID:          sessionID,
			Filename:           layerFilename,
			FileSize:           layerSize,
			ContentType:        contentType,
			FileHash:           fileHash,
			StoragePath:        restoredStoragePath,
			ProcessedPath:      restoredProcessedPath,
			GeoreferencedPath:  restoredStoragePath,
			TilePath:           destMBTilesPath,
			GeoreferenceStatus: georefStatus,
			TileStatus:         tileStatus,
			ExtractedMetadata:  extractedMeta,
			MapLayer:           mapLayer,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		if err := s.repo.InsertDataset(sessionID, ds); err != nil {
			rollback()
			return nil, err
		}
		createdDatasetIDs = append(createdDatasetIDs, newID)
		restoredDatasets = append(restoredDatasets, ds)

		lyrType := "raster"
		if destMBTilesPath != "" {
			lyrType = "mbtiles"
		}
		_ = s.repo.SetLayers(sessionID, newID, []*models.Layer{
			{
				ID:        "lyr_" + newID,
				DatasetID: newID,
				Name:      layerFilename,
				Type:      lyrType,
				Visible:   true,
				Opacity:   1.0,
				Bounds:    boundsArr,
			},
		})

		// Restore control points for this layer
		var layerCPs []*models.ControlPoint
		if rawCPs, ok := layerDoc["control_points"]; ok && rawCPs != nil {
			if b, mErr := json.Marshal(rawCPs); mErr == nil {
				_ = json.Unmarshal(b, &layerCPs)
			}
		}
		if len(layerCPs) == 0 {
			layerCPs = globalCPsByOldID[oldID]
		}
		if len(layerCPs) > 0 {
			for _, cp := range layerCPs {
				if cp != nil {
					if cp.ID == "" {
						cp.ID = "cp_" + repository.GenerateDatasetID()
					}
					cp.DatasetID = newID
				}
			}
			_ = s.repo.SetControlPoints(sessionID, newID, layerCPs)
		}

		ProjectDatasetVectorsAndAnnotations(s.storage, ds)
	}

	// 4. Restore Buildings & Multi-Floor Hierarchy (remapping old dataset IDs -> new dataset IDs)
	oldToNewBuildingID := make(map[string]string)
	if len(rawBuildings) > 0 {
		for _, rb := range rawBuildings {
			bBytes, mErr := json.Marshal(rb)
			if mErr != nil {
				continue
			}
			var bModel models.Building
			if json.Unmarshal(bBytes, &bModel) != nil {
				continue
			}
			oldBldID := bModel.ID
			bModel.ID = ""
			bModel.SessionID = sessionID
			var remappedFloors []*models.BuildingFloor
			for _, fl := range bModel.Floors {
				if fl == nil {
					continue
				}
				var newDsIDs []string
				for _, oldDid := range fl.DatasetIDs {
					if mapped, ok := oldToNewDatasetID[oldDid]; ok {
						newDsIDs = append(newDsIDs, mapped)
					}
				}
				fl.DatasetIDs = newDsIDs
				remappedFloors = append(remappedFloors, fl)
			}
			bModel.Floors = remappedFloors
			for _, cp := range bModel.ControlPoints {
				if cp != nil {
					if mapped, ok := oldToNewDatasetID[cp.DatasetID]; ok {
						cp.DatasetID = mapped
					}
				}
			}
			createdBld, cErr := s.repo.CreateBuilding(sessionID, &bModel)
			if cErr == nil && createdBld != nil && oldBldID != "" {
				oldToNewBuildingID[oldBldID] = createdBld.ID
			}
		}
	} else {
		// Reconstruct building/floor groupings from restored dataset metadata
		bldByName := make(map[string]*models.Building)
		for _, ds := range restoredDatasets {
			bName, _ := ds.ExtractedMetadata["building_name"].(string)
			if bName == "" {
				bName = projectName
			}
			flLvl := int(getDim(ds.ExtractedMetadata["floor_level"]))
			flLbl, _ := ds.ExtractedMetadata["floor_label"].(string)
			if flLbl == "" {
				flLbl = fmt.Sprintf("Floor %d", flLvl)
			}
			bld := bldByName[strings.ToLower(bName)]
			if bld == nil {
				bld, _ = s.repo.CreateBuilding(sessionID, &models.Building{
					Name:              bName,
					DefaultFloorLevel: flLvl,
				})
				if bld != nil {
					bldByName[strings.ToLower(bName)] = bld
				}
			}
			if bld != nil {
				_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
					Level:      flLvl,
					Name:       flLbl,
					DatasetIDs: []string{ds.ID},
				})
			}
		}
	}

	// 5. Restore Bundled Custom Icon Files + Landmark IconPoints
	sessionIconsDir := filepath.Join(s.storage.GetSessionDir(sessionID), "icons")
	_ = os.MkdirAll(sessionIconsDir, 0755)
	copiedIconFiles := make(map[string]string)
	for _, iconSub := range []string{"assets/icons", "icons"} {
		srcIconDir := filepath.Join(extractDir, filepath.FromSlash(iconSub))
		if entries, eErr := os.ReadDir(srcIconDir); eErr == nil {
			for _, ent := range entries {
				if ent.IsDir() || strings.EqualFold(filepath.Ext(ent.Name()), ".json") {
					continue
				}
				srcFile := filepath.Join(srcIconDir, ent.Name())
				dstFile := filepath.Join(sessionIconsDir, ent.Name())
				if storage.CopyFile(srcFile, dstFile) == nil {
					copiedIconFiles[ent.Name()] = dstFile
					stemKey := strings.TrimSuffix(ent.Name(), filepath.Ext(ent.Name()))
					copiedIconFiles[stemKey] = dstFile
				}
			}
		}
	}

	var restoredIcons []*models.IconPoint
	if rawIPs, rErr := os.ReadFile(filepath.Join(extractDir, "icons", "icon_points.json")); rErr == nil {
		_ = json.Unmarshal(rawIPs, &restoredIcons)
	} else if rawGeo, gErr := os.ReadFile(filepath.Join(extractDir, "layers", "landmarks.geojson")); gErr == nil {
		var fc struct {
			Features []struct {
				Geometry struct {
					Coordinates []float64 `json:"coordinates"`
				} `json:"geometry"`
				Properties map[string]interface{} `json:"properties"`
			} `json:"features"`
		}
		if json.Unmarshal(rawGeo, &fc) == nil {
			for _, f := range fc.Features {
				if len(f.Geometry.Coordinates) < 2 {
					continue
				}
				p := f.Properties
				ip := &models.IconPoint{
					ID:          fmt.Sprintf("%v", p["id"]),
					DatasetID:   fmt.Sprintf("%v", p["dataset_id"]),
					BuildingID:  fmt.Sprintf("%v", p["building_id"]),
					Name:        fmt.Sprintf("%v", p["name"]),
					Description: fmt.Sprintf("%v", p["description"]),
					IconID:      fmt.Sprintf("%v", p["icon_id"]),
					Scope:       fmt.Sprintf("%v", p["scope"]),
					FloorLevel:  int(getDim(p["floor_level"])),
					FloorLabel:  fmt.Sprintf("%v", p["floor_label"]),
					Longitude:   f.Geometry.Coordinates[0],
					Latitude:    f.Geometry.Coordinates[1],
				}
				if ip.IconID == "" || ip.IconID == "<nil>" {
					ip.IconID = fmt.Sprintf("%v", p["icon_key"])
				}
				restoredIcons = append(restoredIcons, ip)
			}
		}
	}

	newPrimaryID := restoredDatasets[0].ID
	for _, ip := range restoredIcons {
		if ip == nil {
			continue
		}
		ip.ID = ""
		if mappedDs, ok := oldToNewDatasetID[ip.DatasetID]; ok {
			ip.DatasetID = mappedDs
		} else if ip.DatasetID != "" {
			ip.DatasetID = newPrimaryID
		}
		if mappedBld, ok := oldToNewBuildingID[ip.BuildingID]; ok {
			ip.BuildingID = mappedBld
		}
		if ip.IconPath != "" {
			if localCopy, ok := copiedIconFiles[filepath.Base(ip.IconPath)]; ok {
				ip.IconPath = localCopy
			}
		} else if localCopy, ok := copiedIconFiles[filepath.Base(ip.IconID)]; ok {
			ip.IconPath = localCopy
		}
		_, _ = s.repo.CreateIconPoint(sessionID, ip)
	}

	// 6. Restore complete MapProject configuration (camera, zoom range, base layer, member visibility/opacity/order)
	finalConfig := make(map[string]interface{})
	for k, v := range savedProjConfig {
		finalConfig[k] = v
	}

	var restoredMembers []interface{}
	remappedSeen := make(map[string]bool)
	if rawMembers, ok := finalConfig["members"].([]interface{}); ok {
		for idx, rm := range rawMembers {
			m, ok := rm.(map[string]interface{})
			if !ok {
				continue
			}
			oldDid, _ := m["dataset_id"].(string)
			newDid, exists := oldToNewDatasetID[oldDid]
			if !exists || remappedSeen[newDid] {
				continue
			}
			remappedSeen[newDid] = true
			mCopy := make(map[string]interface{})
			for k, v := range m {
				mCopy[k] = v
			}
			mCopy["dataset_id"] = newDid
			if mCopy["sort_order"] == nil {
				mCopy["sort_order"] = idx
			}
			restoredMembers = append(restoredMembers, mCopy)
		}
	}

	for idx, ds := range restoredDatasets {
		if remappedSeen[ds.ID] {
			continue
		}
		restoredMembers = append(restoredMembers, map[string]interface{}{
			"dataset_id":             ds.ID,
			"name":                   ds.Filename,
			"building_name":          ds.ExtractedMetadata["building_name"],
			"is_building_floor":      true,
			"floor_level":            ds.ExtractedMetadata["floor_level"],
			"floor_label":            ds.ExtractedMetadata["floor_label"],
			"visible":                idx == 0,
			"raster_visible":         idx == 0,
			"raster_opacity":         1.0,
			"control_points_visible": true,
			"icon_points_visible":    true,
			"annotations_visible":    true,
			"vectors_visible":        true,
			"sort_order":             len(restoredMembers),
		})
	}
	finalConfig["members"] = restoredMembers
	finalConfig["active_view"] = "map"
	if actOld, ok := finalConfig["active_dataset_id"].(string); ok {
		if actNew, exists := oldToNewDatasetID[actOld]; exists {
			finalConfig["active_dataset_id"] = actNew
		} else {
			finalConfig["active_dataset_id"] = newPrimaryID
		}
	} else {
		finalConfig["active_dataset_id"] = newPrimaryID
	}

	// Restore camera center & zoom limits from project.json["map"], metadata.json, or root MBTiles inspection
	camCenter := rootInsp.Center
	camZoom := rootInsp.InitialZoom
	camMinZ := rootInsp.MinZoom
	camMaxZ := rootInsp.MaxZoom
	zoomPreset := ""
	explicitMapZoom := false

	if metaPath := filepath.Join(extractDir, "metadata.json"); fileExists(metaPath) {
		if rawM, mErr := os.ReadFile(metaPath); mErr == nil {
			var metaDoc map[string]interface{}
			if json.Unmarshal(rawM, &metaDoc) == nil && metaDoc != nil {
				if iz := getDim(metaDoc["initial_zoom"]); iz > 0 {
					camZoom = iz
					explicitMapZoom = true
				}
				if _, hasMin := metaDoc["minzoom"]; hasMin {
					camMinZ = int(getDim(metaDoc["minzoom"]))
				}
				if _, hasMax := metaDoc["maxzoom"]; hasMax {
					camMaxZ = int(getDim(metaDoc["maxzoom"]))
				}
				if zp, _ := metaDoc["zoom_preset"].(string); zp != "" {
					zoomPreset = zp
				}
			}
		}
	}

	if projectDoc != nil {
		if mapObj, ok := projectDoc["map"].(map[string]interface{}); ok {
			if cArr, ok := mapObj["center"].([]interface{}); ok && len(cArr) >= 2 {
				camCenter = []float64{getDim(cArr[0]), getDim(cArr[1])}
			}
			if z := getDim(mapObj["zoom"]); z > 0 {
				camZoom = z
				explicitMapZoom = true
			}
			if _, hasMin := mapObj["min_zoom"]; hasMin {
				camMinZ = int(getDim(mapObj["min_zoom"]))
			}
			if _, hasMax := mapObj["max_zoom"]; hasMax {
				camMaxZ = int(getDim(mapObj["max_zoom"]))
			}
			if preset, _ := mapObj["preset"].(string); preset != "" {
				zoomPreset = preset
			}
		}
		if bl, ok := projectDoc["base_layer"].(map[string]interface{}); ok && finalConfig["base_layer"] == nil {
			finalConfig["base_layer"] = bl
		}
	}

	if zoomPreset != "" {
		finalConfig["zoom_preset"] = zoomPreset
	}
	finalConfig["min_zoom"] = camMinZ
	finalConfig["max_zoom"] = camMaxZ
	finalConfig["initial_zoom"] = int(camZoom + 0.5)
	if camMap, ok := finalConfig["camera"].(map[string]interface{}); ok && camMap != nil {
		if explicitMapZoom {
			camMap["zoom"] = camZoom
		}
	} else if len(camCenter) >= 2 {
		finalConfig["camera"] = map[string]interface{}{
			"longitude": camCenter[0],
			"latitude":  camCenter[1],
			"center":    camCenter,
			"zoom":      camZoom,
			"bearing":   0.0,
			"pitch":     0.0,
		}
	}

	// Propagate zoom configuration onto the restored primary dataset metadata as well
	if len(restoredDatasets) > 0 && restoredDatasets[0] != nil {
		if restoredDatasets[0].ExtractedMetadata == nil {
			restoredDatasets[0].ExtractedMetadata = make(map[string]interface{})
		}
		restoredDatasets[0].ExtractedMetadata["initial_zoom"] = int(camZoom + 0.5)
		restoredDatasets[0].ExtractedMetadata["minzoom"] = camMinZ
		restoredDatasets[0].ExtractedMetadata["maxzoom"] = camMaxZ
		if zoomPreset != "" {
			restoredDatasets[0].ExtractedMetadata["zoom_preset"] = zoomPreset
		}
		_ = s.repo.UpdateDataset(sessionID, restoredDatasets[0])
	}

	_ = s.repo.SaveMapProject(sessionID, &models.MapProject{
		ID:            "proj_" + repository.GenerateDatasetID(),
		DatasetID:     newPrimaryID,
		Name:          projectName,
		Configuration: finalConfig,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	})

	// Clean up temporary extracted archive tree after all assets are copied to their dataset dirs
	_ = storage.SafeRemoveAll(extractDir)
	_ = os.Remove(zipPath)

	return restoredDatasets[0], nil
}
