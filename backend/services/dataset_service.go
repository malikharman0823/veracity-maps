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
	"strconv"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

var (
	SupportedRasterExtensions = map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".tif":  "image/tiff",
		".tiff": "image/tiff",
		".webp": "image/webp",
		".bmp":  "image/bmp",
	}
)

type DatasetService struct {
	cfg     *config.Config
	repo    *repository.SessionRepository
	storage *storage.StorageManager
}

func NewDatasetService(cfg *config.Config, repo *repository.SessionRepository, sm *storage.StorageManager) *DatasetService {
	return &DatasetService{
		cfg:     cfg,
		repo:    repo,
		storage: sm,
	}
}

func (s *DatasetService) HandleUpload(sessionID string, fileHeader *multipart.FileHeader) (*models.Dataset, error) {
	if _, err := s.repo.GetSession(sessionID); err != nil {
		return nil, repository.ErrSessionNotFound
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".zip" && ext != ".pdf" && ext != ".dxf" && SupportedRasterExtensions[ext] == "" {
		return nil, fmt.Errorf("INVALID_FILE_TYPE: Unsupported file format '%s'. Supported formats are PNG, JPEG, PDF, DXF, and ZIP", ext)
	}

	if fileHeader.Size > s.cfg.MaxUploadSizeBytes {
		return nil, fmt.Errorf("FILE_TOO_LARGE: Upload size exceeds maximum limit of %d bytes", s.cfg.MaxUploadSizeBytes)
	}

	datasetID := repository.GenerateDatasetID()
	datasetDir := s.storage.GetDatasetDir(sessionID, datasetID)
	if err := os.MkdirAll(datasetDir, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: Failed to create dataset directory: %w", err)
	}

	srcFile, err := fileHeader.Open()
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("UPLOAD_FAILED: Failed to open uploaded file: %w", err)
	}
	defer srcFile.Close()

	destPath := filepath.Join(datasetDir, fileHeader.Filename)
	destFile, err := os.Create(destPath)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("STORAGE_ERROR: Failed to save upload: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(destFile, hasher)
	size, err := io.Copy(writer, srcFile)
	_ = destFile.Close()
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("UPLOAD_FAILED: Failed writing upload to disk: %w", err)
	}

	fileHash := hex.EncodeToString(hasher.Sum(nil))

	if ext == ".zip" {
		return s.handleZipDataset(sessionID, datasetID, datasetDir, destPath, fileHeader.Filename, size, fileHash)
	}
	if ext == ".dxf" {
		return s.ConvertCadDxfToDataset(sessionID, datasetID, datasetDir, destPath, fileHeader.Filename, size, fileHash)
	}
	return s.handleRawRasterDataset(sessionID, datasetID, datasetDir, destPath, fileHeader.Filename, size, fileHash, SupportedRasterExtensions[ext])
}

func (s *DatasetService) handleRawRasterDataset(sessionID, datasetID, datasetDir, destPath, filename string, size int64, fileHash, contentType string) (*models.Dataset, error) {
	meta, err := gis.InspectRasterFile(destPath)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("CORRUPTED_FILE: %w", err)
	}

	// Guard: Treat icon images (PNGs, JPEGs, etc.) as icons only, not as big background map rasters
	if isIconAsset(filename, meta.Width, meta.Height, meta.IsGeoreferenced) {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("ICON_ASSET_NOT_A_MAP: '%s' (%dx%d) is recognized as an icon marker symbol, not a map or floorplan raster. Icons should be used as map annotations/markers, not as background raster layers.", filename, meta.Width, meta.Height)
	}

	extractedMeta := map[string]interface{}{
		"driver":               meta.Driver,
		"width":                meta.Width,
		"height":               meta.Height,
		"bands":                meta.Bands,
		"dtype":                meta.DType,
		"crs":                  meta.CRS,
		"bounds":               meta.Bounds,
		"resolution":           meta.Resolution,
		"transform":            meta.Transform,
		"georeference_source":  meta.GeoreferenceSource,
		"color_interpretation": meta.ColorInterpretation,
		"format":               strings.ToLower(meta.Driver),
		"archive_filename":     filename,
	}

	fBase := strings.TrimSuffix(filename, filepath.Ext(filename))
	lvl, lbl := inferFloor(fBase, "")
	bldName := inferBuilding("", fBase, "")
	extractedMeta["floor_level"] = lvl
	extractedMeta["floor_label"] = lbl
	extractedMeta["building_name"] = bldName
	extractedMeta["is_building_floor"] = true

	georefStatus := "not_started"
	var georefPath string
	var mapLayer map[string]interface{}
	if meta.IsGeoreferenced {
		georefStatus = "completed"
		georefPath = destPath
		if meta.Bounds != nil {
			boundsArr, normErr := gis.NormalizeBounds(meta.Bounds)
			if normErr == nil && len(boundsArr) == 4 {
				west, south, east, north := boundsArr[0], boundsArr[1], boundsArr[2], boundsArr[3]
				mapLayer = map[string]interface{}{
					"id":         "lyr_" + datasetID,
					"name":       filename,
					"type":       "raster",
					"bounds":     meta.Bounds,
					"bounds_crs": "EPSG:4326",
					"coordinates": [][]float64{
						{west, north},
						{east, north},
						{east, south},
						{west, south},
					},
				}
			}
		}
	}

	dataset := &models.Dataset{
		ID:                 datasetID,
		SessionID:          sessionID,
		Filename:           filename,
		FileSize:           size,
		ContentType:        contentType,
		FileHash:           fileHash,
		StoragePath:        destPath,
		GeoreferencedPath:  georefPath,
		GeoreferenceStatus: georefStatus,
		TileStatus:         "not_started",
		ExtractedMetadata:  extractedMeta,
		MapLayer:           mapLayer,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, err
	}

	// Register building and floor
	buildings, _ := s.repo.ListBuildings(sessionID)
	var bld *models.Building
	for _, b := range buildings {
		if strings.EqualFold(b.Name, bldName) {
			bld = b
			break
		}
	}
	if bld == nil {
		bld, _ = s.repo.CreateBuilding(sessionID, &models.Building{
			Name:              bldName,
			DefaultFloorLevel: lvl,
		})
	}
	if bld != nil {
		_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
			Level:      lvl,
			Name:       lbl,
			DatasetIDs: []string{datasetID},
		})
	}

	// Propagate if georeferenced
	if dataset.GeoreferenceStatus == "completed" && dataset.MapLayer != nil {
		_ = s.PropagateBuildingGeoreference(sessionID, dataset)
	}
	return dataset, nil
}

func (s *DatasetService) handleZipDataset(sessionID, datasetID, datasetDir, zipPath, filename string, size int64, fileHash string) (*models.Dataset, error) {
	extractDir := filepath.Join(datasetDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("STORAGE_ERROR: Failed creating extraction folder: %w", err)
	}

	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("CORRUPT_ZIP: The ZIP archive could not be opened: %w", err)
	}
	defer zipReader.Close()

	if len(zipReader.File) > 1000 {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("INVALID_ZIP: ZIP contains too many entries (max 1000)")
	}

	var primaryRaster string
	var allRasters []string
	rasterRelPaths := make(map[string]string)
	var totalExtracted int64
	var lowestZoomTile string
	lowestZoom := 999

	for _, file := range zipReader.File {
		cleanEntry := filepath.ToSlash(file.Name)
		if strings.Contains(cleanEntry, "__MACOSX") || strings.HasPrefix(filepath.Base(cleanEntry), "._") || strings.EqualFold(filepath.Base(cleanEntry), ".ds_store") {
			continue
		}

		cleanName, err := s.storage.SafePath(extractDir, file.Name)
		if err != nil {
			_ = storage.SafeRemoveAll(datasetDir)
			return nil, fmt.Errorf("ZIP_SLIP_ATTACK: Unsafe entry '%s' in zip: %w", file.Name, err)
		}

		if file.FileInfo().IsDir() {
			_ = os.MkdirAll(cleanName, 0755)
			continue
		}

		_ = os.MkdirAll(filepath.Dir(cleanName), 0755)
		rc, err := file.Open()
		if err != nil {
			_ = storage.SafeRemoveAll(datasetDir)
			return nil, fmt.Errorf("CORRUPT_ZIP: Failed reading entry '%s': %w", file.Name, err)
		}

		outFile, err := os.OpenFile(cleanName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = rc.Close()
			_ = storage.SafeRemoveAll(datasetDir)
			return nil, fmt.Errorf("STORAGE_ERROR: Failed creating extracted file: %w", err)
		}

		written, err := io.Copy(outFile, io.LimitReader(rc, 200*1024*1024))
		_ = outFile.Close()
		_ = rc.Close()
		if err != nil {
			_ = storage.SafeRemoveAll(datasetDir)
			return nil, fmt.Errorf("STORAGE_ERROR: Error extracting file '%s': %w", file.Name, err)
		}

		totalExtracted += written
		if totalExtracted > 500*1024*1024 {
			_ = storage.SafeRemoveAll(datasetDir)
			return nil, fmt.Errorf("ZIP_BOMB: Extracted content exceeds 500MB safety threshold")
		}

		entryExt := strings.ToLower(filepath.Ext(file.Name))
		// Ignore any assets stored in icons/ folder or tile pyramids so they are never imported as floorplan rasters
		isIconDir := strings.HasPrefix(cleanEntry, "icons/") || strings.Contains(cleanEntry, "/icons/")
		isXYZTile := isTilePath(cleanEntry)
		if isXYZTile && (SupportedRasterExtensions[entryExt] != "" || entryExt == "") {
			parts := strings.Split(cleanEntry, "/")
			if len(parts) >= 3 {
				zPart := parts[len(parts)-3]
				if z, err := strconv.Atoi(zPart); err == nil {
					if z < lowestZoom {
						lowestZoom = z
						lowestZoomTile = cleanName
					}
				}
			}
		}
		if SupportedRasterExtensions[entryExt] != "" && !isIconDir && !isXYZTile {
			// Check if this image is an icon marker rather than a map floorplan
			if !isIconAsset(filepath.Base(cleanName), 0, 0, false) {
				if primaryRaster == "" {
					primaryRaster = cleanName
				}
				allRasters = append(allRasters, cleanName)
				rasterRelPaths[cleanName] = cleanEntry
			}
		}
	}

	// Prioritize mapstudio.json source_asset if present
	mapstudioCandidatePath := filepath.Join(extractDir, "mapstudio.json")
	if fileExists(mapstudioCandidatePath) {
		if data, err := os.ReadFile(mapstudioCandidatePath); err == nil {
			var doc struct {
				Dataset struct {
					SourceAsset string `json:"source_asset"`
					Filename    string `json:"filename"`
				} `json:"dataset"`
			}
			if json.Unmarshal(data, &doc) == nil {
				targetAsset := doc.Dataset.SourceAsset
				if targetAsset == "" && doc.Dataset.Filename != "" {
					targetAsset = doc.Dataset.Filename
				}
				if targetAsset != "" {
					for _, rPath := range allRasters {
						rRel := rasterRelPaths[rPath]
						if strings.EqualFold(filepath.Base(rPath), filepath.Base(targetAsset)) ||
							strings.EqualFold(rRel, filepath.ToSlash(targetAsset)) ||
							strings.EqualFold(filepath.Base(rRel), filepath.Base(targetAsset)) {
							primaryRaster = rPath
							break
						}
					}
				}
			}
		}
	}

	// If archive is a pure tile pyramid (e.g. exported Viewscape tile package), use the lowest-zoom tile as primary raster
	if primaryRaster == "" && lowestZoomTile != "" {
		primaryRaster = lowestZoomTile
		allRasters = append(allRasters, lowestZoomTile)
		rasterRelPaths[lowestZoomTile] = "tile_overview.png"
	}

	if primaryRaster == "" {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("NO_SUPPORTED_RASTER: The ZIP archive does not contain a supported PNG, JPEG, or TIFF image")
	}

	type manifestInfo struct {
		BuildingName string
		FloorLevel   int
		FloorName    string
	}
	manifestMap := make(map[string]*manifestInfo)
	manifestPath := filepath.Join(extractDir, "manifest.json")
	if fileExists(manifestPath) {
		if data, err := os.ReadFile(manifestPath); err == nil {
			var doc struct {
				Buildings []struct {
					ID         string `json:"id"`
					BuildingID string `json:"building_id"`
					Name       string `json:"name"`
					Floors     []struct {
						ID            string `json:"id"`
						FloorID       string `json:"floor_id"`
						Name          string `json:"name"`
						Level         int    `json:"level"`
						Raster        string `json:"raster"`
						Dataset       string `json:"dataset"`
						Path          string `json:"path"`
						File          string `json:"file"`
						Georeferenced bool   `json:"georeferenced"`
					} `json:"floors"`
				} `json:"buildings"`
			}
			if err := json.Unmarshal(data, &doc); err == nil {
				for _, b := range doc.Buildings {
					bName := strings.TrimSpace(b.Name)
					if bName == "" {
						bName = strings.TrimSpace(b.BuildingID)
					}
					if bName == "" {
						bName = b.ID
					}
					for _, fl := range b.Floors {
						rPath := fl.Raster
						if rPath == "" {
							rPath = fl.Dataset
						}
						if rPath == "" {
							rPath = fl.Path
						}
						if rPath == "" {
							rPath = fl.File
						}
						fName := strings.TrimSpace(fl.Name)
						if fName == "" {
							if fl.Level == 0 {
								fName = "Ground Floor"
							} else if fl.Level < 0 {
								fName = fmt.Sprintf("Basement %d", -fl.Level)
							} else {
								fName = fmt.Sprintf("Floor %d", fl.Level)
							}
						}
						cleanR := filepath.ToSlash(rPath)
						info := &manifestInfo{
							BuildingName: bName,
							FloorLevel:   fl.Level,
							FloorName:    fName,
						}
						manifestMap[cleanR] = info
						manifestMap[filepath.Base(cleanR)] = info
						manifestMap[strings.ToLower(cleanR)] = info
						manifestMap[strings.ToLower(filepath.Base(cleanR))] = info
					}
				}
			}
		}
	}

	meta, err := gis.InspectRasterFile(primaryRaster)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("CORRUPTED_FILE: Failed inspecting raster inside zip: %w", err)
	}

	relPrimary := rasterRelPaths[primaryRaster]
	if relPrimary == "" {
		relPrimary = filepath.Base(primaryRaster)
	}
	relDir := filepath.ToSlash(filepath.Dir(relPrimary))
	pBase := strings.TrimSuffix(filepath.Base(primaryRaster), filepath.Ext(primaryRaster))
	var pBld string
	var pLvl int
	var pLbl string
	if mInfo, ok := manifestMap[relPrimary]; ok {
		pBld = mInfo.BuildingName
		pLvl = mInfo.FloorLevel
		pLbl = mInfo.FloorName
	} else if mInfo, ok := manifestMap[filepath.Base(primaryRaster)]; ok {
		pBld = mInfo.BuildingName
		pLvl = mInfo.FloorLevel
		pLbl = mInfo.FloorName
	} else if mInfo, ok := manifestMap[strings.ToLower(relPrimary)]; ok {
		pBld = mInfo.BuildingName
		pLvl = mInfo.FloorLevel
		pLbl = mInfo.FloorName
	} else if mInfo, ok := manifestMap[strings.ToLower(filepath.Base(primaryRaster))]; ok {
		pBld = mInfo.BuildingName
		pLvl = mInfo.FloorLevel
		pLbl = mInfo.FloorName
	} else {
		pBld = inferBuilding(relDir, pBase, filename)
		pLvl, pLbl = inferFloor(pBase, relDir)
	}

	extractedMeta := map[string]interface{}{
		"driver":               meta.Driver,
		"width":                meta.Width,
		"height":               meta.Height,
		"bands":                meta.Bands,
		"dtype":                meta.DType,
		"crs":                  meta.CRS,
		"bounds":               meta.Bounds,
		"resolution":           meta.Resolution,
		"transform":            meta.Transform,
		"georeference_source":  meta.GeoreferenceSource,
		"color_interpretation": meta.ColorInterpretation,
		"format":               strings.ToLower(meta.Driver),
		"archive_filename":     filename,
		"archive_entry":        relPrimary,
		"building_name":        pBld,
		"is_building_floor":    true,
		"floor_level":          pLvl,
		"floor_label":          pLbl,
	}

	georefStatus := "not_started"
	var georefPath string
	var pMapLayer map[string]interface{}
	if meta.IsGeoreferenced && meta.Bounds != nil {
		georefStatus = "completed"
		georefPath = primaryRaster
		boundsArr, normErr := gis.NormalizeBounds(meta.Bounds)
		if normErr == nil && len(boundsArr) == 4 {
			west, south, east, north := boundsArr[0], boundsArr[1], boundsArr[2], boundsArr[3]
			pMapLayer = map[string]interface{}{
				"id":         "lyr_" + datasetID,
				"name":       filepath.Base(primaryRaster),
				"type":       "raster",
				"bounds":     meta.Bounds,
				"bounds_crs": "EPSG:4326",
				"coordinates": [][]float64{
					{west, north},
					{east, north},
					{east, south},
					{west, south},
				},
			}
		}
	}

	originalPrimaryRaster := primaryRaster
	// Ensure primary raster is saved in its own datasetDir folder
	primaryBase := filepath.Base(primaryRaster)
	if filepath.Ext(primaryBase) == "" {
		if strings.EqualFold(meta.Driver, "PNG") {
			primaryBase += ".png"
		} else if strings.EqualFold(meta.Driver, "JPEG") {
			primaryBase += ".jpg"
		} else if strings.EqualFold(meta.Driver, "TIFF") {
			primaryBase += ".tif"
		} else {
			primaryBase += ".png"
		}
	}
	primaryTarget := filepath.Join(datasetDir, primaryBase)
	if data, readErr := os.ReadFile(primaryRaster); readErr == nil {
		_ = os.WriteFile(primaryTarget, data, 0644)
		primaryRaster = primaryTarget
	}

	dataset := &models.Dataset{
		ID:                 datasetID,
		SessionID:          sessionID,
		Filename:           filepath.Base(primaryRaster),
		FileSize:           size,
		ContentType:        SupportedRasterExtensions[strings.ToLower(filepath.Ext(primaryRaster))],
		FileHash:           fileHash,
		StoragePath:        primaryRaster,
		GeoreferencedPath:  georefPath,
		GeoreferenceStatus: georefStatus,
		TileStatus:         "not_started",
		ExtractedMetadata:  extractedMeta,
		MapLayer:           pMapLayer,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Restore exported package metadata, control points, icon points, and project configuration if present
	parseZoomInt := func(val interface{}) int {
		if val == nil {
			return -1
		}
		switch v := val.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return i
			}
		}
		return -1
	}

	importedInitZoom := -1
	importedMinZoom := -1
	importedMaxZoom := -1
	importedZoomPreset := ""

	mapstudioPath := filepath.Join(extractDir, "mapstudio.json")
	if fileExists(mapstudioPath) {
		if data, err := os.ReadFile(mapstudioPath); err == nil {
			var doc struct {
				Project struct {
					Name          string                 `json:"name"`
					Configuration map[string]interface{} `json:"configuration"`
				} `json:"project"`
				Dataset struct {
					Filename           string                 `json:"filename"`
					GeoreferenceStatus string                 `json:"georeference_status"`
					ExtractedMetadata  map[string]interface{} `json:"extracted_metadata"`
					MapLayer           map[string]interface{} `json:"map_layer"`
				} `json:"dataset"`
				ControlPoints []*models.ControlPoint `json:"control_points"`
				IconPoints    []*models.IconPoint    `json:"icon_points"`
				InitialZoom   interface{}            `json:"initial_zoom"`
				MinZoom       interface{}            `json:"minzoom"`
				MaxZoom       interface{}            `json:"maxzoom"`
				ZoomPreset    string                 `json:"zoom_preset"`
			}
			if err := json.Unmarshal(data, &doc); err == nil {
				if doc.Dataset.ExtractedMetadata != nil {
					for k, v := range doc.Dataset.ExtractedMetadata {
						dataset.ExtractedMetadata[k] = v
					}
				}
				if doc.Dataset.MapLayer != nil {
					dataset.MapLayer = doc.Dataset.MapLayer
				}
				if doc.Dataset.GeoreferenceStatus != "" {
					dataset.GeoreferenceStatus = doc.Dataset.GeoreferenceStatus
				}

				if z := parseZoomInt(doc.InitialZoom); z >= 0 {
					importedInitZoom = z
				}
				if z := parseZoomInt(doc.MinZoom); z >= 0 {
					importedMinZoom = z
				}
				if z := parseZoomInt(doc.MaxZoom); z >= 0 {
					importedMaxZoom = z
				}
				if doc.ZoomPreset != "" {
					importedZoomPreset = doc.ZoomPreset
				}

				if len(doc.ControlPoints) > 0 {
					for _, p := range doc.ControlPoints {
						p.DatasetID = datasetID
					}
					_ = s.repo.SetControlPoints(sessionID, datasetID, doc.ControlPoints)
					dataset.GeoreferenceStatus = "completed"
					if dataset.GeoreferencedPath == "" {
						dataset.GeoreferencedPath = primaryRaster
					}

					if dataset.ExtractedMetadata["transform"] == nil && len(doc.ControlPoints) >= 2 && meta.Width > 0 && meta.Height > 0 {
						if res, err := gis.CalculateGeoreference(doc.ControlPoints, meta.Width, meta.Height, ""); err == nil && res != nil {
							dataset.ExtractedMetadata["bounds"] = res.Bounds
							dataset.ExtractedMetadata["transform"] = res.Transform
							dataset.ExtractedMetadata["crs"] = "EPSG:4326"
							dataset.ExtractedMetadata["georeference_source"] = "control_points"

							a, b, c := res.Transform[0], -res.Transform[1], res.Transform[2]
							d, e, f := res.Transform[3], -res.Transform[4], res.Transform[5]
							project := func(ix, iy float64) []float64 {
								return []float64{
									a*ix + b*(-iy) + c,
									d*ix + e*(-iy) + f,
								}
							}
							wF := float64(meta.Width)
							hF := float64(meta.Height)
							coordinates := [][]float64{
								project(0, 0),
								project(wF, 0),
								project(wF, hF),
								project(0, hF),
							}
							dataset.MapLayer = map[string]interface{}{
								"id":          "lyr_" + datasetID,
								"name":        dataset.Filename,
								"type":        "raster",
								"bounds":      res.Bounds,
								"bounds_crs":  "EPSG:4326",
								"coordinates": coordinates,
							}
							baseExt := strings.ToLower(filepath.Ext(primaryRaster))
							worldExt := ".wld"
							switch baseExt {
							case ".png":
								worldExt = ".pgw"
							case ".jpg", ".jpeg":
								worldExt = ".jgw"
							}
							worldPath := filepath.Join(filepath.Dir(primaryRaster), strings.TrimSuffix(filepath.Base(primaryRaster), baseExt)+worldExt)
							_ = gis.WriteWorldFile(worldPath, res.Transform)
						}
					}
				}

				if len(doc.IconPoints) > 0 {
					for _, ip := range doc.IconPoints {
						if ip.DatasetID == "" {
							ip.DatasetID = datasetID
						}
						ip.SessionID = sessionID
						if ip.Name == "" {
							if ip.Description != "" {
								ip.Name = ip.Description
							} else if ip.IconID != "" {
								ip.Name = ip.IconID
							}
						}
						_, _ = s.repo.CreateIconPoint(sessionID, ip)
					}
				}
			}
		}
	}

	// Check metadata.json for bounds, zoom config, control points, and icon points fallback
	metaJSONPath := filepath.Join(extractDir, "metadata.json")
	if fileExists(metaJSONPath) {
		if data, err := os.ReadFile(metaJSONPath); err == nil {
			var mDoc struct {
				Name          string                 `json:"name"`
				Bounds        []float64              `json:"bounds"`
				ControlPoints []*models.ControlPoint `json:"control_points"`
				IconPoints    []*models.IconPoint    `json:"icon_points"`
				InitialZoom   interface{}            `json:"initial_zoom"`
				MinZoom       interface{}            `json:"minzoom"`
				MaxZoom       interface{}            `json:"maxzoom"`
				ZoomPreset    string                 `json:"zoom_preset"`
			}
			if err := json.Unmarshal(data, &mDoc); err == nil {
				if z := parseZoomInt(mDoc.InitialZoom); z >= 0 {
					importedInitZoom = z
				}
				if z := parseZoomInt(mDoc.MinZoom); z >= 0 {
					importedMinZoom = z
				}
				if z := parseZoomInt(mDoc.MaxZoom); z >= 0 {
					importedMaxZoom = z
				}
				if mDoc.ZoomPreset != "" {
					importedZoomPreset = mDoc.ZoomPreset
				}

				if len(mDoc.Bounds) == 4 {
					west, south, east, north := mDoc.Bounds[0], mDoc.Bounds[1], mDoc.Bounds[2], mDoc.Bounds[3]
					if east > west && north > south {
						if dataset.ExtractedMetadata["bounds"] == nil {
							dataset.ExtractedMetadata["bounds"] = map[string]interface{}{
								"min_x": west,
								"min_y": south,
								"max_x": east,
								"max_y": north,
							}
						}
						if dataset.MapLayer == nil {
							dataset.MapLayer = map[string]interface{}{
								"id":         "lyr_" + datasetID,
								"name":       dataset.Filename,
								"type":       "raster",
								"bounds":     dataset.ExtractedMetadata["bounds"],
								"bounds_crs": "EPSG:4326",
								"coordinates": [][]float64{
									{west, north},
									{east, north},
									{east, south},
									{west, south},
								},
							}
						}
						dataset.GeoreferenceStatus = "completed"
					}
				}
				if len(mDoc.ControlPoints) > 0 {
					existingPts, _ := s.repo.ListControlPoints(sessionID, datasetID)
					if len(existingPts) == 0 {
						for _, p := range mDoc.ControlPoints {
							p.DatasetID = datasetID
						}
						_ = s.repo.SetControlPoints(sessionID, datasetID, mDoc.ControlPoints)
						dataset.GeoreferenceStatus = "completed"
						if dataset.GeoreferencedPath == "" {
							dataset.GeoreferencedPath = primaryRaster
						}
					}
				}
				if len(mDoc.IconPoints) > 0 {
					existingIPs, _ := s.repo.ListIconPoints(sessionID, datasetID, "")
					if len(existingIPs) == 0 {
						for _, ip := range mDoc.IconPoints {
							if ip.DatasetID == "" {
								ip.DatasetID = datasetID
							}
							ip.SessionID = sessionID
							if ip.Name == "" {
								if ip.Description != "" {
									ip.Name = ip.Description
								} else if ip.IconID != "" {
									ip.Name = ip.IconID
								}
							}
							_, _ = s.repo.CreateIconPoint(sessionID, ip)
						}
					}
				}
			}
		}
	}

	// Check icons/icon_points.json fallback
	iconsJSONPath := filepath.Join(extractDir, "icons", "icon_points.json")
	if fileExists(iconsJSONPath) {
		if data, err := os.ReadFile(iconsJSONPath); err == nil {
			var iconPts []*models.IconPoint
			if err := json.Unmarshal(data, &iconPts); err == nil && len(iconPts) > 0 {
				existingIPs, _ := s.repo.ListIconPoints(sessionID, datasetID, "")
				if len(existingIPs) == 0 {
					for _, ip := range iconPts {
						if ip.DatasetID == "" {
							ip.DatasetID = datasetID
						}
						ip.SessionID = sessionID
						if ip.Name == "" {
							if ip.Description != "" {
								ip.Name = ip.Description
							} else if ip.IconID != "" {
								ip.Name = ip.IconID
							}
						}
						_, _ = s.repo.CreateIconPoint(sessionID, ip)
					}
				}
			}
		}
	}

	// Check project.json (veracity-project v1 or version 2)
	projectJSONPath := filepath.Join(extractDir, "project.json")
	if fileExists(projectJSONPath) {
		if data, err := os.ReadFile(projectJSONPath); err == nil {
			var pDoc struct {
				Format        string      `json:"format"`
				SchemaVersion int         `json:"schema_version"`
				Name          string      `json:"name"`
				InitialZoom   interface{} `json:"initial_zoom"`
				MinZoom       interface{} `json:"minzoom"`
				MaxZoom       interface{} `json:"maxzoom"`
				ZoomPreset    string      `json:"zoom_preset"`
				Map           struct {
					Center []float64   `json:"center"`
					Zoom   interface{} `json:"zoom"`
					MinZ   interface{} `json:"min_zoom"`
					MaxZ   interface{} `json:"max_zoom"`
					Preset string      `json:"preset"`
				} `json:"map"`
			}
			if err := json.Unmarshal(data, &pDoc); err == nil {
				if z := parseZoomInt(pDoc.InitialZoom); z >= 0 {
					importedInitZoom = z
				} else if z := parseZoomInt(pDoc.Map.Zoom); z >= 0 {
					importedInitZoom = z
				}
				if z := parseZoomInt(pDoc.MinZoom); z >= 0 {
					importedMinZoom = z
				} else if z := parseZoomInt(pDoc.Map.MinZ); z >= 0 {
					importedMinZoom = z
				}
				if z := parseZoomInt(pDoc.MaxZoom); z >= 0 {
					importedMaxZoom = z
				} else if z := parseZoomInt(pDoc.Map.MaxZ); z >= 0 {
					importedMaxZoom = z
				}
				if pDoc.ZoomPreset != "" {
					importedZoomPreset = pDoc.ZoomPreset
				} else if pDoc.Map.Preset != "" {
					importedZoomPreset = pDoc.Map.Preset
				}
			}
		}
	}

	// Check layers/control_points.geojson
	cpGeoJSONPath := filepath.Join(extractDir, "layers", "control_points.geojson")
	if fileExists(cpGeoJSONPath) {
		if data, err := os.ReadFile(cpGeoJSONPath); err == nil {
			var fc struct {
				Features []struct {
					Geometry struct {
						Coordinates []float64 `json:"coordinates"`
					} `json:"geometry"`
					Properties struct {
						ID        string  `json:"id"`
						ImageX    float64 `json:"image_x"`
						ImageY    float64 `json:"image_y"`
						Longitude float64 `json:"longitude"`
						Latitude  float64 `json:"latitude"`
					} `json:"properties"`
				} `json:"features"`
			}
			if err := json.Unmarshal(data, &fc); err == nil && len(fc.Features) > 0 {
				var parsedCPs []*models.ControlPoint
				for _, feat := range fc.Features {
					lng := feat.Properties.Longitude
					lat := feat.Properties.Latitude
					if len(feat.Geometry.Coordinates) >= 2 {
						lng = feat.Geometry.Coordinates[0]
						lat = feat.Geometry.Coordinates[1]
					}
					id := feat.Properties.ID
					if id == "" {
						id = repository.GeneratePointID()
					}
					parsedCPs = append(parsedCPs, &models.ControlPoint{
						ID:        id,
						DatasetID: datasetID,
						ImageX:    feat.Properties.ImageX,
						ImageY:    feat.Properties.ImageY,
						Longitude: lng,
						Latitude:  lat,
					})
				}
				if len(parsedCPs) > 0 {
					existingPts, _ := s.repo.ListControlPoints(sessionID, datasetID)
					if len(existingPts) == 0 {
						_ = s.repo.SetControlPoints(sessionID, datasetID, parsedCPs)
						dataset.GeoreferenceStatus = "completed"
						if dataset.GeoreferencedPath == "" {
							dataset.GeoreferencedPath = primaryRaster
						}
					}
				}
			}
		}
	}

	// Check layers/landmarks.geojson
	landmarksGeoJSONPath := filepath.Join(extractDir, "layers", "landmarks.geojson")
	if fileExists(landmarksGeoJSONPath) {
		if data, err := os.ReadFile(landmarksGeoJSONPath); err == nil {
			var fc struct {
				Features []struct {
					Geometry struct {
						Coordinates []float64 `json:"coordinates"`
					} `json:"geometry"`
					Properties map[string]interface{} `json:"properties"`
				} `json:"features"`
			}
			if err := json.Unmarshal(data, &fc); err == nil && len(fc.Features) > 0 {
				existingIPs, _ := s.repo.ListIconPoints(sessionID, datasetID, "")
				if len(existingIPs) == 0 {
					for _, feat := range fc.Features {
						props := feat.Properties
						lng := 0.0
						lat := 0.0
						if len(feat.Geometry.Coordinates) >= 2 {
							lng = feat.Geometry.Coordinates[0]
							lat = feat.Geometry.Coordinates[1]
						}
						id, _ := props["id"].(string)
						if id == "" {
							id = repository.GenerateIconPointID()
						}
						name, _ := props["name"].(string)
						iconID, _ := props["icon_id"].(string)
						if iconID == "" {
							iconID, _ = props["icon_key"].(string)
						}
						desc, _ := props["description"].(string)
						scope, _ := props["scope"].(string)
						if scope == "" {
							scope = "global"
						}
						flLvl := 0
						if lvlVal, ok := props["floor_level"].(float64); ok {
							flLvl = int(lvlVal)
						}
						flLbl, _ := props["floor_label"].(string)
						ip := &models.IconPoint{
							ID:          id,
							DatasetID:   datasetID,
							SessionID:   sessionID,
							Name:        name,
							IconID:      iconID,
							Description: desc,
							Scope:       scope,
							FloorLevel:  flLvl,
							FloorLabel:  flLbl,
							Longitude:   lng,
							Latitude:    lat,
							Properties:  props,
						}
						if ip.Name == "" {
							ip.Name = ip.Description
						}
						if ip.Name == "" {
							ip.Name = ip.IconID
						}
						_, _ = s.repo.CreateIconPoint(sessionID, ip)
					}
				}
			}
		}
	}

	// Persist extracted zoom configuration into dataset metadata
	if importedInitZoom >= 0 {
		dataset.ExtractedMetadata["initial_zoom"] = importedInitZoom
	}
	if importedMinZoom >= 0 {
		dataset.ExtractedMetadata["minzoom"] = importedMinZoom
	}
	if importedMaxZoom >= 0 {
		dataset.ExtractedMetadata["maxzoom"] = importedMaxZoom
	}
	if importedZoomPreset != "" {
		dataset.ExtractedMetadata["zoom_preset"] = importedZoomPreset
	}

	if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, err
	}

	allCreatedDatasets := []*models.Dataset{dataset}

	// 3. Extract and register all additional rasters in archive (e.g. multi-floor building plans)
	for _, extraPath := range allRasters {
		if extraPath == originalPrimaryRaster || extraPath == primaryRaster {
			continue
		}
		extraID := repository.GenerateDatasetID()
		extraDir := s.storage.GetDatasetDir(sessionID, extraID)
		_ = os.MkdirAll(extraDir, 0755)

		extraBase := filepath.Base(extraPath)
		targetRasterPath := filepath.Join(extraDir, extraBase)
		if data, readErr := os.ReadFile(extraPath); readErr == nil {
			_ = os.WriteFile(targetRasterPath, data, 0644)
		} else {
			targetRasterPath = extraPath
		}

		// Copy any matching sidecars
		srcDir := filepath.Dir(extraPath)
		extraBaseNoExt := strings.TrimSuffix(extraBase, filepath.Ext(extraBase))
		for _, sExt := range []string{".pgw", ".jgw", ".wld", ".tfw", ".prj"} {
			if scData, scErr := os.ReadFile(filepath.Join(srcDir, extraBaseNoExt+sExt)); scErr == nil {
				_ = os.WriteFile(filepath.Join(extraDir, extraBaseNoExt+sExt), scData, 0644)
			}
		}

		extraMeta, err := gis.InspectRasterFile(targetRasterPath)
		if err != nil {
			continue
		}

		extraRel := rasterRelPaths[extraPath]
		if extraRel == "" {
			extraRel = extraBase
		}
		extraRelDir := filepath.ToSlash(filepath.Dir(extraRel))
		var eBld string
		var eLvl int
		var eLbl string
		if mInfo, ok := manifestMap[extraRel]; ok {
			eBld = mInfo.BuildingName
			eLvl = mInfo.FloorLevel
			eLbl = mInfo.FloorName
		} else if mInfo, ok := manifestMap[extraBase]; ok {
			eBld = mInfo.BuildingName
			eLvl = mInfo.FloorLevel
			eLbl = mInfo.FloorName
		} else if mInfo, ok := manifestMap[strings.ToLower(extraRel)]; ok {
			eBld = mInfo.BuildingName
			eLvl = mInfo.FloorLevel
			eLbl = mInfo.FloorName
		} else if mInfo, ok := manifestMap[strings.ToLower(extraBase)]; ok {
			eBld = mInfo.BuildingName
			eLvl = mInfo.FloorLevel
			eLbl = mInfo.FloorName
		} else {
			eBld = inferBuilding(extraRelDir, extraBaseNoExt, filename)
			eLvl, eLbl = inferFloor(extraBaseNoExt, extraRelDir)
		}

		eGeorefStatus := "not_started"
		var eGeorefPath string
		var extraLayer map[string]interface{}
		if extraMeta.IsGeoreferenced && extraMeta.Bounds != nil {
			eGeorefStatus = "completed"
			eGeorefPath = targetRasterPath
			boundsArr, normErr := gis.NormalizeBounds(extraMeta.Bounds)
			if normErr == nil && len(boundsArr) == 4 {
				west, south, east, north := boundsArr[0], boundsArr[1], boundsArr[2], boundsArr[3]
				extraLayer = map[string]interface{}{
					"id":         "lyr_" + extraID,
					"name":       extraBase,
					"type":       "raster",
					"bounds":     extraMeta.Bounds,
					"bounds_crs": "EPSG:4326",
					"coordinates": [][]float64{
						{west, north},
						{east, north},
						{east, south},
						{west, south},
					},
				}
			}
		}

		extraExtracted := map[string]interface{}{
			"driver":               extraMeta.Driver,
			"width":                extraMeta.Width,
			"height":               extraMeta.Height,
			"bands":                extraMeta.Bands,
			"dtype":                extraMeta.DType,
			"crs":                  extraMeta.CRS,
			"bounds":               extraMeta.Bounds,
			"resolution":           extraMeta.Resolution,
			"transform":            extraMeta.Transform,
			"georeference_source":  extraMeta.GeoreferenceSource,
			"color_interpretation": extraMeta.ColorInterpretation,
			"format":               strings.ToLower(extraMeta.Driver),
			"archive_filename":     filename,
			"archive_entry":        extraRel,
			"building_name":        eBld,
			"is_building_floor":    true,
			"floor_level":          eLvl,
			"floor_label":          eLbl,
		}

		extraFi, _ := os.Stat(targetRasterPath)
		extraSize := int64(0)
		if extraFi != nil {
			extraSize = extraFi.Size()
		}

		extraDs := &models.Dataset{
			ID:                 extraID,
			SessionID:          sessionID,
			Filename:           extraBase,
			FileSize:           extraSize,
			ContentType:        SupportedRasterExtensions[strings.ToLower(filepath.Ext(targetRasterPath))],
			StoragePath:        targetRasterPath,
			GeoreferencedPath:  eGeorefPath,
			GeoreferenceStatus: eGeorefStatus,
			TileStatus:         "not_started",
			ExtractedMetadata:  extraExtracted,
			MapLayer:           extraLayer,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}
		_ = s.repo.InsertDataset(sessionID, extraDs)
		allCreatedDatasets = append(allCreatedDatasets, extraDs)
	}

	// 4. Group all created floors by building and register Building & Floor models
	buildingFloorsMap := make(map[string][]*models.Dataset)
	for _, ds := range allCreatedDatasets {
		bName, _ := ds.ExtractedMetadata["building_name"].(string)
		if bName == "" {
			bName = "Building 1"
		}
		buildingFloorsMap[bName] = append(buildingFloorsMap[bName], ds)
	}

	for bName, floorDatasets := range buildingFloorsMap {
		buildings, _ := s.repo.ListBuildings(sessionID)
		var bld *models.Building
		for _, b := range buildings {
			if strings.EqualFold(b.Name, bName) {
				bld = b
				break
			}
		}
		if bld == nil {
			defLvl := 0
			if len(floorDatasets) > 0 {
				if l, ok := floorDatasets[0].ExtractedMetadata["floor_level"].(int); ok {
					defLvl = l
				}
			}
			bld, _ = s.repo.CreateBuilding(sessionID, &models.Building{
				Name:              bName,
				DefaultFloorLevel: defLvl,
			})
		}
		if bld != nil {
			for _, ds := range floorDatasets {
				lvl := 0
				if l, ok := ds.ExtractedMetadata["floor_level"].(int); ok {
					lvl = l
				}
				lbl, _ := ds.ExtractedMetadata["floor_label"].(string)
				if lbl == "" {
					lbl = ds.Filename
				}
				exists := false
				for _, fl := range bld.Floors {
					if fl.Level == lvl {
						fl.DatasetIDs = append(fl.DatasetIDs, ds.ID)
						exists = true
						break
					}
				}
				if !exists {
					_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
						Level:      lvl,
						Name:       lbl,
						DatasetIDs: []string{ds.ID},
					})
				}
			}
		}
	}

	// 5. If ANY floor has a georeference, propagate it to all other floors in that building
	for _, ds := range allCreatedDatasets {
		if ds.GeoreferenceStatus == "completed" && ds.ExtractedMetadata["bounds"] != nil {
			_ = s.PropagateBuildingGeoreference(sessionID, ds)
		}
	}

	// Reload datasets to reflect any propagated bounds
	var reloadedDatasets []*models.Dataset
	for _, ds := range allCreatedDatasets {
		if updated, err := s.repo.GetDataset(sessionID, ds.ID); err == nil && updated != nil {
			reloadedDatasets = append(reloadedDatasets, updated)
			if updated.ID == dataset.ID {
				dataset = updated
			}
		} else {
			reloadedDatasets = append(reloadedDatasets, ds)
		}
	}

	// 6. Build MapProject members list so workspace immediately contains all floors
	var membersList []map[string]interface{}
	for idx, ds := range reloadedDatasets {
		lvl, _ := ds.ExtractedMetadata["floor_level"].(int)
		lbl, _ := ds.ExtractedMetadata["floor_label"].(string)
		bName, _ := ds.ExtractedMetadata["building_name"].(string)
		membersList = append(membersList, map[string]interface{}{
			"dataset_id":             ds.ID,
			"name":                   ds.Filename,
			"floor_label":            lbl,
			"floor_level":            lvl,
			"building_name":          bName,
			"is_building_floor":      true,
			"visible":                true,
			"raster_visible":         true,
			"raster_opacity":         1.0,
			"control_points_visible": true,
			"icon_points_visible":    true,
			"sort_order":             idx,
			"bounds":                 ds.ExtractedMetadata["bounds"],
			"map_layer":              ds.MapLayer,
		})
	}

	for _, ds := range reloadedDatasets {
		_ = s.repo.SaveMapProject(sessionID, &models.MapProject{
			ID:        "proj_" + ds.ID,
			DatasetID: ds.ID,
			Name:      strings.TrimSuffix(filename, filepath.Ext(filename)),
			Configuration: map[string]interface{}{
				"active_view": "map",
				"members":     membersList,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}

	// 7. Expose imported_datasets list so frontend gets all floor datasets
	var importedList []map[string]interface{}
	for _, ds := range reloadedDatasets {
		importedList = append(importedList, map[string]interface{}{
			"id":                  ds.ID,
			"name":                ds.Filename,
			"is_georeferenced":    ds.GeoreferenceStatus == "completed",
			"georeference_status": ds.GeoreferenceStatus,
		})
	}
	dataset.ExtractedMetadata["imported_datasets"] = importedList
	_ = s.repo.UpdateDataset(sessionID, dataset)

	// 8. Restore selective layer settings, background removal, and icon points if exported project package
	for _, ds := range reloadedDatasets {
		// Check for layer.json in layers/{ds.ID} or layers/{origID}
		var layerJSONPaths []string
		_ = filepath.Walk(extractDir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && filepath.Base(p) == "layer.json" {
				layerJSONPaths = append(layerJSONPaths, p)
			}
			return nil
		})
		for _, ljp := range layerJSONPaths {
			if data, err := os.ReadFile(ljp); err == nil {
				var lDoc struct {
					ID                string                 `json:"id"`
					Filename          string                 `json:"filename"`
					MapLayer          map[string]interface{} `json:"map_layer"`
					BackgroundRemoval map[string]interface{} `json:"background_removal"`
					RasterTransform   map[string]interface{} `json:"raster_transform"`
					RasterCorners     interface{}            `json:"raster_corners"`
					RasterOpacity     interface{}            `json:"raster_opacity"`
					ControlPoints     []*models.ControlPoint `json:"control_points"`
				}
				if json.Unmarshal(data, &lDoc) == nil {
					if lDoc.ID == ds.ID || strings.EqualFold(lDoc.Filename, ds.Filename) {
						if lDoc.MapLayer != nil && ds.MapLayer == nil {
							ds.MapLayer = lDoc.MapLayer
						}
						if lDoc.BackgroundRemoval != nil {
							ds.ExtractedMetadata["background_removal"] = lDoc.BackgroundRemoval
							// If transparent png exists in layer dir or processed/
							tPath := filepath.Join(filepath.Dir(ljp), "processed_transparent.png")
							if fileExists(tPath) {
								destProcDir := filepath.Join(filepath.Dir(ds.StoragePath), "processed")
								_ = os.MkdirAll(destProcDir, 0755)
								if tBytes, rErr := os.ReadFile(tPath); rErr == nil {
									_ = os.WriteFile(filepath.Join(destProcDir, "processed_transparent.png"), tBytes, 0644)
								}
							}
						}
						if lDoc.RasterTransform != nil {
							ds.ExtractedMetadata["raster_transform"] = lDoc.RasterTransform
						}
						if lDoc.RasterCorners != nil {
							ds.ExtractedMetadata["raster_corners"] = lDoc.RasterCorners
						}
						if lDoc.RasterOpacity != nil {
							ds.ExtractedMetadata["raster_opacity"] = lDoc.RasterOpacity
						}
						if len(lDoc.ControlPoints) > 0 {
							for _, p := range lDoc.ControlPoints {
								p.DatasetID = ds.ID
							}
							_ = s.repo.SetControlPoints(sessionID, ds.ID, lDoc.ControlPoints)
						}
						_ = s.repo.UpdateDataset(sessionID, ds)
						break
					}
				}
			}
		}
	}

	// Restore icon points if present in icons/icon_points.json or metadata.json
	var iconPoints []*models.IconPoint
	iconPointsPath := filepath.Join(extractDir, "icons", "icon_points.json")
	if fileExists(iconPointsPath) {
		if data, err := os.ReadFile(iconPointsPath); err == nil {
			_ = json.Unmarshal(data, &iconPoints)
		}
	}
	if len(iconPoints) == 0 && fileExists(metaJSONPath) {
		if data, err := os.ReadFile(metaJSONPath); err == nil {
			var mDoc struct {
				IconPoints []*models.IconPoint `json:"icon_points"`
			}
			if json.Unmarshal(data, &mDoc) == nil && len(mDoc.IconPoints) > 0 {
				iconPoints = mDoc.IconPoints
			}
		}
	}
	if len(iconPoints) > 0 {
		for _, ip := range iconPoints {
			// Associate with the new primary dataset ID if it was tied to the previous export dataset
			if ip.DatasetID == "" || len(allCreatedDatasets) == 1 {
				ip.DatasetID = dataset.ID
			} else {
				// Check if it matches any floor dataset
				matched := false
				for _, ds := range allCreatedDatasets {
					if ds.ID == ip.DatasetID {
						matched = true
						break
					}
				}
				if !matched {
					ip.DatasetID = dataset.ID
				}
			}
			if ip.BuildingID == "" {
				if bld, ok := dataset.ExtractedMetadata["building_name"].(string); ok && bld != "" {
					ip.BuildingID = bld
				}
			}
			if ip.FloorLevel == 0 {
				if fl, ok := dataset.ExtractedMetadata["floor_level"].(int); ok {
					ip.FloorLevel = fl
				}
			}
			_, _ = s.repo.CreateIconPoint(sessionID, ip)
		}
	}

	return dataset, nil
}

func (s *DatasetService) ListDatasets(sessionID string) ([]*models.Dataset, error) {
	return s.repo.ListDatasets(sessionID)
}

func (s *DatasetService) GetDataset(sessionID, datasetID string) (*models.Dataset, error) {
	if sessionID != "" {
		return s.repo.GetDataset(sessionID, datasetID)
	}
	ds, _, err := s.repo.FindDataset(datasetID)
	return ds, err
}

func (s *DatasetService) FindDataset(datasetID string) (*models.Dataset, string, error) {
	return s.repo.FindDataset(datasetID)
}

func (s *DatasetService) DeleteDataset(sessionID, datasetID string) error {
	dataset, err := s.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		return err
	}

	datasetDir := filepath.Dir(dataset.StoragePath)
	if strings.Contains(datasetDir, datasetID) {
		_ = storage.SafeRemoveAll(datasetDir)
	}

	return s.repo.DeleteDataset(sessionID, datasetID)
}

// Multi-dataset ZIP structures & methods
type ArchiveCandidate struct {
	EntryPath           string   `json:"entry_path"`
	Name                string   `json:"name"`
	Format              string   `json:"format"`
	FileSize            int64    `json:"file_size"`
	Width               int      `json:"width,omitempty"`
	Height              int      `json:"height,omitempty"`
	IsGeoreferenced     bool     `json:"is_georeferenced"`
	Sidecars            []string `json:"sidecars,omitempty"`
	SuggestedBuilding   string   `json:"suggested_building,omitempty"`
	SuggestedFloor      int      `json:"suggested_floor"`
	SuggestedFloorLabel string   `json:"suggested_floor_label,omitempty"`
	Status              string   `json:"status"` // "ready", "warning", "unsupported"
	Warnings            []string `json:"warnings,omitempty"`
}

type ArchiveInspectionResult struct {
	ArchiveToken string              `json:"archive_token"`
	ArchiveName  string              `json:"archive_name"`
	TotalEntries int                 `json:"total_entries"`
	TotalRasters int                 `json:"total_rasters"`
	Datasets     []*ArchiveCandidate `json:"datasets"`
}

type DatasetImportSelection struct {
	EntryPath    string `json:"entry_path"`
	Name         string `json:"name"`
	BuildingName string `json:"building_name,omitempty"`
	FloorLevel   int    `json:"floor_level"`
	FloorLabel   string `json:"floor_label,omitempty"`
}

type ImportDatasetResult struct {
	EntryPath string          `json:"entry_path"`
	DatasetID string          `json:"dataset_id,omitempty"`
	Name      string          `json:"name"`
	Status    string          `json:"status"` // "imported", "failed", "skipped"
	Error     string          `json:"error,omitempty"`
	Dataset   *models.Dataset `json:"dataset,omitempty"`
}

type ImportArchiveResponse struct {
	ImportedCount int                    `json:"imported_count"`
	FailedCount   int                    `json:"failed_count"`
	Results       []*ImportDatasetResult `json:"results"`
}

func (s *DatasetService) InspectArchive(sessionID string, header *multipart.FileHeader) (*ArchiveInspectionResult, error) {
	file, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("UPLOAD_FAILED: Unable to read uploaded file: %w", err)
	}
	defer file.Close()

	if header.Size > 500*1024*1024 {
		return nil, fmt.Errorf("FILE_TOO_LARGE: Uploaded archive exceeds maximum 500MB limit")
	}

	importID := repository.GenerateSessionID()
	importsDir := filepath.Join(s.storage.GetSessionDir(sessionID), "imports", importID)
	if err := os.MkdirAll(importsDir, 0755); err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: Failed creating import directory: %w", err)
	}

	archivePath := filepath.Join(importsDir, "archive.zip")
	out, err := os.Create(archivePath)
	if err != nil {
		return nil, fmt.Errorf("STORAGE_ERROR: Failed saving archive: %w", err)
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		return nil, fmt.Errorf("STORAGE_ERROR: Failed writing archive: %w", err)
	}
	out.Close()

	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("CORRUPT_ZIP: The ZIP archive could not be opened: %w", err)
	}
	defer zipReader.Close()

	if len(zipReader.File) > 1000 {
		return nil, fmt.Errorf("INVALID_ZIP: Archive exceeds limit of 1000 files")
	}

	// Classify files
	allEntryNames := make(map[string]*zip.File)
	var rasterFiles []*zip.File

	for _, f := range zipReader.File {
		cleanName := filepath.ToSlash(f.Name)
		if strings.Contains(cleanName, "..") || strings.HasPrefix(cleanName, "/") || strings.HasPrefix(cleanName, "\\") {
			return nil, fmt.Errorf("ZIP_SLIP_ATTACK: Unsafe path detected in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		allEntryNames[cleanName] = f
		ext := strings.ToLower(filepath.Ext(cleanName))
		isIconDir := strings.HasPrefix(cleanName, "icons/") || strings.Contains(cleanName, "/icons/")
		isXYZTile := isTilePath(cleanName)
		if SupportedRasterExtensions[ext] != "" && !isIconDir && !isXYZTile {
			if !isIconAsset(filepath.Base(cleanName), 0, 0, false) {
				rasterFiles = append(rasterFiles, f)
			}
		}
	}

	if len(rasterFiles) == 0 {
		return nil, fmt.Errorf("NO_SUPPORTED_RASTER: The ZIP archive does not contain any supported image files (PNG, JPG, TIFF, WebP)")
	}

	tempInspectDir := filepath.Join(importsDir, "staging")
	_ = os.MkdirAll(tempInspectDir, 0755)

	type manifestFloorInfo struct {
		BuildingName string
		FloorLevel   int
		FloorName    string
		DisplayName  string
	}
	manifestMap := make(map[string]*manifestFloorInfo)

	var manifestFile *zip.File
	for name, f := range allEntryNames {
		if strings.EqualFold(filepath.Base(name), "manifest.json") {
			manifestFile = f
			break
		}
	}

	if manifestFile != nil {
		if mRc, err := manifestFile.Open(); err == nil {
			var doc struct {
				Buildings []struct {
					ID         string `json:"id"`
					BuildingID string `json:"building_id"`
					Name       string `json:"name"`
					Floors     []struct {
						ID            string `json:"id"`
						FloorID       string `json:"floor_id"`
						Name          string `json:"name"`
						Level         int    `json:"level"`
						Raster        string `json:"raster"`
						Dataset       string `json:"dataset"`
						Path          string `json:"path"`
						File          string `json:"file"`
						Georeferenced bool   `json:"georeferenced"`
					} `json:"floors"`
				} `json:"buildings"`
			}
			if data, readErr := io.ReadAll(io.LimitReader(mRc, 5*1024*1024)); readErr == nil {
				if json.Unmarshal(data, &doc) == nil {
					for _, bld := range doc.Buildings {
						bldName := strings.TrimSpace(bld.Name)
						if bldName == "" {
							bldName = strings.TrimSpace(bld.BuildingID)
						}
						if bldName == "" {
							bldName = bld.ID
						}
						for _, fl := range bld.Floors {
							rPath := fl.Raster
							if rPath == "" {
								rPath = fl.Dataset
							}
							if rPath == "" {
								rPath = fl.Path
							}
							if rPath == "" {
								rPath = fl.File
							}
							cleanRaster := filepath.ToSlash(rPath)
							flName := strings.TrimSpace(fl.Name)
							if flName == "" {
								if fl.Level == 0 {
									flName = "Ground Floor"
								} else if fl.Level < 0 {
									flName = fmt.Sprintf("Basement %d", -fl.Level)
								} else {
									flName = fmt.Sprintf("Floor %d", fl.Level)
								}
							}
							dispName := fmt.Sprintf("%s %s", bldName, flName)
							info := &manifestFloorInfo{
								BuildingName: bldName,
								FloorLevel:   fl.Level,
								FloorName:    flName,
								DisplayName:  dispName,
							}
							manifestMap[cleanRaster] = info
							manifestMap[filepath.Base(cleanRaster)] = info
							manifestMap[strings.ToLower(cleanRaster)] = info
							manifestMap[strings.ToLower(filepath.Base(cleanRaster))] = info
						}
					}
				}
			}
			mRc.Close()
		}
	}

	var candidates []*ArchiveCandidate
	archiveBase := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))

	for _, rf := range rasterFiles {
		entryClean := filepath.ToSlash(rf.Name)
		dir := filepath.ToSlash(filepath.Dir(entryClean))
		base := strings.TrimSuffix(filepath.Base(entryClean), filepath.Ext(entryClean))
		ext := strings.ToLower(filepath.Ext(entryClean))

		// Check matching sidecars
		var matchingSidecars []string
		sidecarExts := []string{".pgw", ".jgw", ".wld", ".tfw", ".prj"}
		for _, sExt := range sidecarExts {
			sidecarCandidate := base + sExt
			if dir != "." && dir != "" {
				sidecarCandidate = dir + "/" + base + sExt
			}
			if _, exists := allEntryNames[sidecarCandidate]; exists {
				matchingSidecars = append(matchingSidecars, filepath.Base(sidecarCandidate))
			}
		}

		// Extract raster to staging for header inspection
		stagePath := filepath.Join(tempInspectDir, filepath.Base(entryClean))
		rc, err := rf.Open()
		var width, height int
		isGeoref := false
		if err == nil {
			destF, createErr := os.Create(stagePath)
			if createErr == nil {
				_, _ = io.Copy(destF, io.LimitReader(rc, 20*1024*1024))
				destF.Close()

				// Also extract sidecars to inspect georeferencing
				for _, scName := range matchingSidecars {
					scFull := scName
					if dir != "." && dir != "" {
						scFull = dir + "/" + scName
					}
					if scFile, ok := allEntryNames[scFull]; ok {
						if scRc, err := scFile.Open(); err == nil {
							scDest, err := os.Create(filepath.Join(tempInspectDir, scName))
							if err == nil {
								_, _ = io.Copy(scDest, scRc)
								scDest.Close()
							}
							scRc.Close()
						}
					}
				}

				if meta, err := gis.InspectRasterFile(stagePath); err == nil && meta != nil {
					width = meta.Width
					height = meta.Height
					isGeoref = meta.IsGeoreferenced
				}
			}
			rc.Close()
		}

		var suggestedBld string
		var suggestedFloor int
		var suggestedFloorLabel string
		var cleanDisplayName string

		if info, ok := manifestMap[entryClean]; ok {
			suggestedBld = info.BuildingName
			suggestedFloor = info.FloorLevel
			suggestedFloorLabel = info.FloorName
			cleanDisplayName = info.DisplayName
		} else if info, ok := manifestMap[filepath.Base(entryClean)]; ok {
			suggestedBld = info.BuildingName
			suggestedFloor = info.FloorLevel
			suggestedFloorLabel = info.FloorName
			cleanDisplayName = info.DisplayName
		} else if info, ok := manifestMap[strings.ToLower(entryClean)]; ok {
			suggestedBld = info.BuildingName
			suggestedFloor = info.FloorLevel
			suggestedFloorLabel = info.FloorName
			cleanDisplayName = info.DisplayName
		} else if info, ok := manifestMap[strings.ToLower(filepath.Base(entryClean))]; ok {
			suggestedBld = info.BuildingName
			suggestedFloor = info.FloorLevel
			suggestedFloorLabel = info.FloorName
			cleanDisplayName = info.DisplayName
		} else {
			suggestedBld = inferBuilding(dir, base, archiveBase)
			suggestedFloor, suggestedFloorLabel = inferFloor(base, dir)
			cleanDisplayName = strings.Title(strings.ReplaceAll(strings.ReplaceAll(base, "-", " "), "_", " "))
		}

		status := "ready"
		var warnings []string
		if !isGeoref {
			warnings = append(warnings, "No georeferencing metadata found. Layer will inherit building footprint or require manual georeferencing.")
		}

		candidates = append(candidates, &ArchiveCandidate{
			EntryPath:           entryClean,
			Name:                cleanDisplayName,
			Format:              strings.TrimPrefix(ext, "."),
			FileSize:            rf.FileInfo().Size(),
			Width:               width,
			Height:              height,
			IsGeoreferenced:     isGeoref,
			Sidecars:            matchingSidecars,
			SuggestedBuilding:   suggestedBld,
			SuggestedFloor:      suggestedFloor,
			SuggestedFloorLabel: suggestedFloorLabel,
			Status:              status,
			Warnings:            warnings,
		})
	}

	return &ArchiveInspectionResult{
		ArchiveToken: importID,
		ArchiveName:  header.Filename,
		TotalEntries: len(zipReader.File),
		TotalRasters: len(candidates),
		Datasets:     candidates,
	}, nil
}

func (s *DatasetService) ImportArchiveSelected(sessionID string, token string, selections []*DatasetImportSelection) (*ImportArchiveResponse, error) {
	if token == "" || len(selections) == 0 {
		return nil, fmt.Errorf("INVALID_REQUEST: Missing archive token or selections")
	}

	archivePath := filepath.Join(s.storage.GetSessionDir(sessionID), "imports", token, "archive.zip")
	if _, statErr := os.Stat(archivePath); statErr != nil {
		sessionsRoot := filepath.Dir(s.storage.GetSessionDir(sessionID))
		if matches, _ := filepath.Glob(filepath.Join(sessionsRoot, "*", "imports", token, "archive.zip")); len(matches) > 0 {
			archivePath = matches[0]
		}
	}
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("ARCHIVE_NOT_FOUND: Unable to open staged archive with token '%s': %w", token, err)
	}
	defer zipReader.Close()

	entriesMap := make(map[string]*zip.File)
	for _, f := range zipReader.File {
		clean := filepath.ToSlash(f.Name)
		entriesMap[clean] = f
	}

	var results []*ImportDatasetResult
	importedCount := 0
	failedCount := 0

	for _, sel := range selections {
		entryClean := filepath.ToSlash(sel.EntryPath)
		zipFile, exists := entriesMap[entryClean]
		if !exists {
			failedCount++
			results = append(results, &ImportDatasetResult{
				EntryPath: sel.EntryPath,
				Name:      sel.Name,
				Status:    "failed",
				Error:     "File entry not found in archive",
			})
			continue
		}

		datasetID := repository.GenerateDatasetID()
		datasetDir := s.storage.GetDatasetDir(sessionID, datasetID)
		if err := os.MkdirAll(datasetDir, 0755); err != nil {
			failedCount++
			results = append(results, &ImportDatasetResult{
				EntryPath: sel.EntryPath,
				Name:      sel.Name,
				Status:    "failed",
				Error:     fmt.Sprintf("Failed creating directory: %v", err),
			})
			continue
		}

		baseName := filepath.Base(entryClean)
		destRaster := filepath.Join(datasetDir, baseName)

		rc, err := zipFile.Open()
		if err != nil {
			_ = storage.SafeRemoveAll(datasetDir)
			failedCount++
			results = append(results, &ImportDatasetResult{
				EntryPath: sel.EntryPath,
				Name:      sel.Name,
				Status:    "failed",
				Error:     fmt.Sprintf("Failed opening archive entry: %v", err),
			})
			continue
		}

		destF, err := os.Create(destRaster)
		if err != nil {
			rc.Close()
			_ = storage.SafeRemoveAll(datasetDir)
			failedCount++
			results = append(results, &ImportDatasetResult{
				EntryPath: sel.EntryPath,
				Name:      sel.Name,
				Status:    "failed",
				Error:     fmt.Sprintf("Failed writing extracted file: %v", err),
			})
			continue
		}

		_, copyErr := io.Copy(destF, io.LimitReader(rc, 200*1024*1024))
		destF.Close()
		rc.Close()
		if copyErr != nil {
			_ = storage.SafeRemoveAll(datasetDir)
			failedCount++
			results = append(results, &ImportDatasetResult{
				EntryPath: sel.EntryPath,
				Name:      sel.Name,
				Status:    "failed",
				Error:     fmt.Sprintf("Failed extracting raster content: %v", copyErr),
			})
			continue
		}

		// Also extract any matching sidecars in the same directory
		dir := filepath.ToSlash(filepath.Dir(entryClean))
		baseWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		sidecarExts := []string{".pgw", ".jgw", ".wld", ".tfw", ".prj"}
		for _, sExt := range sidecarExts {
			sidecarEntry := baseWithoutExt + sExt
			if dir != "." && dir != "" {
				sidecarEntry = dir + "/" + baseWithoutExt + sExt
			}
			if scFile, ok := entriesMap[sidecarEntry]; ok {
				if scRc, err := scFile.Open(); err == nil {
					scDestPath := filepath.Join(datasetDir, filepath.Base(sidecarEntry))
					if scDestF, err := os.Create(scDestPath); err == nil {
						_, _ = io.Copy(scDestF, scRc)
						scDestF.Close()
					}
					scRc.Close()
				}
			}
		}

		meta, err := gis.InspectRasterFile(destRaster)
		if err != nil {
			_ = storage.SafeRemoveAll(datasetDir)
			failedCount++
			results = append(results, &ImportDatasetResult{
				EntryPath: sel.EntryPath,
				Name:      sel.Name,
				Status:    "failed",
				Error:     fmt.Sprintf("Raster inspection failed: %v", err),
			})
			continue
		}

		georefStatus := "not_started"
		var georefPath string
		if meta.IsGeoreferenced {
			georefStatus = "completed"
			georefPath = destRaster
		}

		extractedMeta := map[string]interface{}{
			"driver":               meta.Driver,
			"width":                meta.Width,
			"height":               meta.Height,
			"bands":                meta.Bands,
			"dtype":                meta.DType,
			"crs":                  meta.CRS,
			"bounds":               meta.Bounds,
			"resolution":           meta.Resolution,
			"transform":            meta.Transform,
			"georeference_source":  meta.GeoreferenceSource,
			"color_interpretation": meta.ColorInterpretation,
			"format":               strings.ToLower(meta.Driver),
			"archive_entry":        sel.EntryPath,
		}

		var mapLayer map[string]interface{}
		if georefStatus == "completed" && meta.Bounds != nil {
			boundsArr, normErr := gis.NormalizeBounds(meta.Bounds)
			if normErr == nil && len(boundsArr) == 4 {
				west, south, east, north := boundsArr[0], boundsArr[1], boundsArr[2], boundsArr[3]
				mapLayer = map[string]interface{}{
					"id":         "lyr_" + datasetID,
					"name":       sel.Name,
					"type":       "raster",
					"bounds":     meta.Bounds,
					"bounds_crs": "EPSG:4326",
					"coordinates": [][]float64{
						{west, north},
						{east, north},
						{east, south},
						{west, south},
					},
				}
			}
		}

		if sel.BuildingName != "" {
			extractedMeta["building_name"] = sel.BuildingName
			extractedMeta["is_building_floor"] = true
			extractedMeta["floor_level"] = sel.FloorLevel
			floorLbl := sel.FloorLabel
			if floorLbl == "" {
				floorLbl = fmt.Sprintf("Floor %d", sel.FloorLevel)
			}
			extractedMeta["floor_label"] = floorLbl
		}

		datasetName := sel.Name
		if datasetName == "" {
			datasetName = baseName
		}

		dataset := &models.Dataset{
			ID:                 datasetID,
			SessionID:          sessionID,
			Filename:           datasetName,
			FileSize:           zipFile.FileInfo().Size(),
			ContentType:        SupportedRasterExtensions[strings.ToLower(filepath.Ext(destRaster))],
			StoragePath:        destRaster,
			GeoreferencedPath:  georefPath,
			GeoreferenceStatus: georefStatus,
			TileStatus:         "not_started",
			ExtractedMetadata:  extractedMeta,
			MapLayer:           mapLayer,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
			_ = storage.SafeRemoveAll(datasetDir)
			failedCount++
			results = append(results, &ImportDatasetResult{
				EntryPath: sel.EntryPath,
				Name:      sel.Name,
				Status:    "failed",
				Error:     fmt.Sprintf("Failed inserting dataset record: %v", err),
			})
			continue
		}

		// Register Building & Floor if designated
		if sel.BuildingName != "" {
			bName := sel.BuildingName
			buildings, _ := s.repo.ListBuildings(sessionID)
			var bld *models.Building
			for _, b := range buildings {
				if strings.EqualFold(b.Name, bName) {
					bld = b
					break
				}
			}
			if bld == nil {
				bld, _ = s.repo.CreateBuilding(sessionID, &models.Building{
					Name:              bName,
					DefaultFloorLevel: sel.FloorLevel,
				})
			}
			if bld != nil {
				_, _ = s.repo.AddFloor(sessionID, bld.ID, &models.BuildingFloor{
					Level:      sel.FloorLevel,
					Name:       sel.FloorLabel,
					DatasetIDs: []string{datasetID},
				})
			}
		}

		importedCount++
		results = append(results, &ImportDatasetResult{
			EntryPath: sel.EntryPath,
			DatasetID: datasetID,
			Name:      datasetName,
			Status:    "imported",
			Dataset:   dataset,
		})
	}

	// Propagate building footprint/georeferencing to sibling floors that lack it
	if buildings, err := s.repo.ListBuildings(sessionID); err == nil {
		for _, bld := range buildings {
			var refDataset *models.Dataset
			for _, fl := range bld.Floors {
				for _, dID := range fl.DatasetIDs {
					if ds, err := s.repo.GetDataset(sessionID, dID); err == nil && ds != nil {
						if ds.GeoreferenceStatus == "completed" && ds.MapLayer != nil {
							refDataset = ds
							break
						}
					}
				}
				if refDataset != nil {
					break
				}
			}

			if refDataset != nil {
				_ = s.PropagateBuildingGeoreference(sessionID, refDataset)
			}
		}
	}

	// Build or update MapProject so the workspace immediately contains all imported datasets
	if len(results) > 0 {
		var firstID string
		var membersList []map[string]interface{}
		for idx, res := range results {
			if res.DatasetID != "" {
				if firstID == "" {
					firstID = res.DatasetID
				}
				ds, _ := s.repo.GetDataset(sessionID, res.DatasetID)
				if ds != nil {
					lvl, _ := ds.ExtractedMetadata["floor_level"].(int)
					lbl, _ := ds.ExtractedMetadata["floor_label"].(string)
					bName, _ := ds.ExtractedMetadata["building_name"].(string)
					membersList = append(membersList, map[string]interface{}{
						"dataset_id":             ds.ID,
						"name":                   ds.Filename,
						"floor_label":            lbl,
						"floor_level":            lvl,
						"building_name":          bName,
						"is_building_floor":      true,
						"visible":                true,
						"raster_visible":         true,
						"raster_opacity":         1.0,
						"control_points_visible": true,
						"icon_points_visible":    true,
						"sort_order":             idx,
						"bounds":                 ds.ExtractedMetadata["bounds"],
						"map_layer":              ds.MapLayer,
					})
				}
			}
		}
		if firstID != "" {
			_ = s.repo.SaveMapProject(sessionID, &models.MapProject{
				ID:        "proj_" + firstID,
				DatasetID: firstID,
				Name:      "Workspace",
				Configuration: map[string]interface{}{
					"active_view": "map",
					"members":     membersList,
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			})
		}
	}

	// Update response results with any mutated dataset records after inheritance
	for _, res := range results {
		if res.DatasetID != "" {
			if updatedDs, err := s.repo.GetDataset(sessionID, res.DatasetID); err == nil && updatedDs != nil {
				res.Dataset = updatedDs
				res.Status = "imported"
			}
		}
	}

	return &ImportArchiveResponse{
		ImportedCount: importedCount,
		FailedCount:   failedCount,
		Results:       results,
	}, nil
}

func isFloorFolderToken(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	prefixes := []string{"floor", "flr", "fl_", "fl-", "level", "lvl", "storey", "story", "basement", "b1", "b2", "b3", "b4", "ground", "cellar", "roof", "terrace", "first", "second", "third"}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) || strings.EqualFold(lower, p) {
			return true
		}
	}
	if len(lower) == 2 && (lower[1] == 'f' || lower[1] == 'l') && lower[0] >= '0' && lower[0] <= '9' {
		return true
	}
	for _, num := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		if strings.HasSuffix(lower, num) && (strings.Contains(lower, "floor") || strings.Contains(lower, "flr") || strings.Contains(lower, "lvl") || strings.Contains(lower, "level") || strings.Contains(lower, "basement")) {
			return true
		}
	}
	return false
}

func inferFloor(name, folder string) (int, string) {
	combined := strings.ToLower(filepath.ToSlash(name + " " + folder))
	cleanBase := strings.ToLower(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))

	// Check Basements (negative floors)
	if strings.Contains(combined, "basement 4") || strings.Contains(combined, "b4") || (strings.HasSuffix(cleanBase, "_-4") || strings.HasSuffix(cleanBase, "-4")) && strings.Contains(cleanBase, "basement") {
		return -4, "Basement 4"
	}
	if strings.Contains(combined, "basement 3") || strings.Contains(combined, "b3") || (strings.HasSuffix(cleanBase, "_-3") || strings.HasSuffix(cleanBase, "-3")) && strings.Contains(cleanBase, "basement") {
		return -3, "Basement 3"
	}
	if strings.Contains(combined, "basement 2") || strings.Contains(combined, "b2") || strings.Contains(combined, "sub-basement") || (strings.HasSuffix(cleanBase, "_-2") || strings.HasSuffix(cleanBase, "-2")) && strings.Contains(cleanBase, "basement") {
		return -2, "Basement 2"
	}
	if strings.Contains(combined, "basement 1") || strings.Contains(combined, "basement") || strings.Contains(combined, "b1") || strings.Contains(combined, "cellar") || strings.Contains(combined, "floor_b") || strings.Contains(combined, "floor-b") || strings.HasSuffix(cleanBase, "_-1") || (strings.HasSuffix(cleanBase, "-1") && strings.Contains(cleanBase, "basement")) {
		return -1, "Basement 1"
	}

	// Check Roof / Terrace
	if strings.Contains(combined, "roof") || strings.Contains(combined, "terrace") {
		return 99, "Roof / Terrace"
	}

	// Check Ground
	if strings.Contains(combined, "ground") || strings.Contains(combined, "floor_g") || strings.Contains(combined, "floor-g") || strings.Contains(combined, "floor g") || strings.Contains(combined, "floor_0") || strings.Contains(combined, "floor 0") || strings.Contains(combined, "level 0") || strings.Contains(combined, "lvl 0") || strings.Contains(combined, "g-floor") || strings.Contains(combined, "g_floor") || strings.Contains(combined, "0f") || strings.HasSuffix(cleanBase, "_0") || strings.HasSuffix(cleanBase, "-0") {
		return 0, "Ground Floor"
	}

	// Suffix and word checks for upper floors
	if strings.Contains(combined, "first") || strings.Contains(combined, "1st") || strings.HasSuffix(cleanBase, "_1") || (strings.HasSuffix(cleanBase, "-1") && !strings.Contains(cleanBase, "basement")) {
		return 1, "First Floor"
	}
	if strings.Contains(combined, "second") || strings.Contains(combined, "2nd") || strings.HasSuffix(cleanBase, "_2") || strings.HasSuffix(cleanBase, "-2") {
		return 2, "Second Floor"
	}
	if strings.Contains(combined, "third") || strings.Contains(combined, "3rd") || strings.HasSuffix(cleanBase, "_3") || strings.HasSuffix(cleanBase, "-3") {
		return 3, "Third Floor"
	}
	if strings.Contains(combined, "fourth") || strings.Contains(combined, "4th") || strings.HasSuffix(cleanBase, "_4") || strings.HasSuffix(cleanBase, "-4") {
		return 4, "Fourth Floor"
	}
	if strings.Contains(combined, "fifth") || strings.Contains(combined, "5th") || strings.HasSuffix(cleanBase, "_5") || strings.HasSuffix(cleanBase, "-5") {
		return 5, "Fifth Floor"
	}

	// Numbered floors
	for lvl := 15; lvl >= 1; lvl-- {
		pats := []string{
			fmt.Sprintf("floor %d", lvl),
			fmt.Sprintf("floor_%d", lvl),
			fmt.Sprintf("floor-%d", lvl),
			fmt.Sprintf("floor%d", lvl),
			fmt.Sprintf("level %d", lvl),
			fmt.Sprintf("level_%d", lvl),
			fmt.Sprintf("level-%d", lvl),
			fmt.Sprintf("level%d", lvl),
			fmt.Sprintf("lvl %d", lvl),
			fmt.Sprintf("lvl_%d", lvl),
			fmt.Sprintf("lvl-%d", lvl),
			fmt.Sprintf("flr %d", lvl),
			fmt.Sprintf("flr_%d", lvl),
			fmt.Sprintf("fl_%d", lvl),
			fmt.Sprintf("%df", lvl),
		}
		for _, pat := range pats {
			if strings.Contains(combined, pat) {
				return lvl, fmt.Sprintf("Floor %d", lvl)
			}
		}
	}

	return 0, "Ground Floor"
}

func inferBuilding(folder, filename, archiveName string) string {
	parts := strings.Split(filepath.ToSlash(folder), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part == "" || part == "." || strings.EqualFold(part, "source") || strings.EqualFold(part, "images") || strings.EqualFold(part, "rasters") {
			continue
		}
		if isFloorFolderToken(part) {
			continue
		}
		clean := strings.ReplaceAll(part, "-", " ")
		clean = strings.ReplaceAll(clean, "_", " ")
		if isFloorFolderToken(clean) {
			continue
		}
		return strings.Title(strings.ToLower(clean))
	}
	if filename != "" {
		base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		lower := strings.ToLower(base)
		floorIdx := -1
		for _, token := range []string{"_floor", "-floor", " floor", "_flr", "-flr", "_lvl", "-lvl", "_level", "-level", "_b1", "_b2", "_b3", "_b4", "_g"} {
			if idx := strings.Index(lower, token); idx > 0 {
				floorIdx = idx
				break
			}
		}
		if floorIdx > 0 {
			bldPart := strings.TrimSpace(base[:floorIdx])
			if len(bldPart) > 1 && !isFloorFolderToken(bldPart) {
				clean := strings.ReplaceAll(strings.ReplaceAll(bldPart, "-", " "), "_", " ")
				if !isFloorFolderToken(clean) {
					return strings.Title(strings.ToLower(clean))
				}
			}
		} else if !isFloorFolderToken(base) {
			clean := strings.ReplaceAll(strings.ReplaceAll(base, "-", " "), "_", " ")
			if !isFloorFolderToken(clean) && len(clean) > 0 {
				return strings.Title(strings.ToLower(clean))
			}
		}
	}
	if archiveName != "" {
		base := strings.TrimSuffix(archiveName, filepath.Ext(archiveName))
		clean := strings.ReplaceAll(base, "-", " ")
		clean = strings.ReplaceAll(clean, "_", " ")
		if !isFloorFolderToken(clean) {
			return strings.Title(strings.ToLower(clean))
		}
	}
	return "Building 1"
}

// PropagateBuildingGeoreference delegates building-wide spatial propagation to the shared service helper.
func (s *DatasetService) PropagateBuildingGeoreference(sessionID string, refDataset *models.Dataset) error {
	return PropagateBuildingGeoreference(s.repo, s.storage, sessionID, refDataset)
}

// ConsolidateSessionBuildings delegates pseudo-building consolidation and footprint propagation.
func (s *DatasetService) ConsolidateSessionBuildings(sessionID string) error {
	return ConsolidateSessionBuildings(s.repo, s.storage, sessionID)
}

// StorageManager returns the active StorageManager.
func (s *DatasetService) StorageManager() *storage.StorageManager {
	return s.storage
}

// LocateOrRestoreDatasetImage attempts to locate the dataset's raster image on disk or re-extract it from an archive if displaced or missing.
func (s *DatasetService) LocateOrRestoreDatasetImage(dataset *models.Dataset) string {
	if dataset == nil {
		return ""
	}

	// 1. If StoragePath exists and is non-empty, verify on disk
	if dataset.StoragePath != "" && fileExists(dataset.StoragePath) {
		return dataset.StoragePath
	}

	// 2. Check ProcessedPath if it's an image
	if dataset.ProcessedPath != "" && fileExists(dataset.ProcessedPath) {
		ext := strings.ToLower(filepath.Ext(dataset.ProcessedPath))
		if SupportedRasterExtensions[ext] != "" && ext != ".vrt" {
			return dataset.ProcessedPath
		}
	}

	// 3. Check GeoreferencedPath if it's an image
	if dataset.GeoreferencedPath != "" && fileExists(dataset.GeoreferencedPath) {
		ext := strings.ToLower(filepath.Ext(dataset.GeoreferencedPath))
		if SupportedRasterExtensions[ext] != "" && ext != ".vrt" {
			return dataset.GeoreferencedPath
		}
	}

	// 4. Resolve session and dataset directories
	datasetDir := s.storage.GetDatasetDir(dataset.SessionID, dataset.ID)
	sessionDir := s.storage.GetSessionDir(dataset.SessionID)

	targetFilename := dataset.Filename
	if targetFilename == "" && dataset.StoragePath != "" {
		targetFilename = filepath.Base(dataset.StoragePath)
	}

	var archiveEntry string
	if dataset.ExtractedMetadata != nil {
		if ae, ok := dataset.ExtractedMetadata["archive_entry"].(string); ok && ae != "" {
			archiveEntry = ae
		}
	}

	// 5. Walk datasetDir recursively to see if file is located anywhere in the dataset directory
	if fileExists(datasetDir) {
		var foundPath string
		_ = filepath.Walk(datasetDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if SupportedRasterExtensions[ext] == "" || ext == ".vrt" {
				return nil
			}
			base := filepath.Base(path)
			if strings.EqualFold(base, targetFilename) ||
				(archiveEntry != "" && strings.EqualFold(base, filepath.Base(archiveEntry))) ||
				(dataset.StoragePath != "" && strings.EqualFold(base, filepath.Base(dataset.StoragePath))) {
				foundPath = path
				return filepath.SkipAll
			}
			return nil
		})
		if foundPath != "" && fileExists(foundPath) {
			dataset.StoragePath = foundPath
			_ = s.repo.UpdateDataset(dataset.SessionID, dataset)
			return foundPath
		}
	}

	// 6. Look for any zip files in datasetDir or sessionDir
	var zipCandidates []string
	if fileExists(datasetDir) {
		if matches, err := filepath.Glob(filepath.Join(datasetDir, "*.zip")); err == nil {
			zipCandidates = append(zipCandidates, matches...)
		}
	}
	if dataset.ExtractedMetadata != nil {
		if archFn, ok := dataset.ExtractedMetadata["archive_filename"].(string); ok && archFn != "" {
			cand := filepath.Join(datasetDir, archFn)
			if fileExists(cand) {
				zipCandidates = append(zipCandidates, cand)
			}
			candSess := filepath.Join(sessionDir, archFn)
			if fileExists(candSess) {
				zipCandidates = append(zipCandidates, candSess)
			}
		}
	}
	// Also check session-level imports or datasets for archives
	if fileExists(filepath.Join(sessionDir, "imports")) {
		_ = filepath.Walk(filepath.Join(sessionDir, "imports"), func(path string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".zip") {
				zipCandidates = append(zipCandidates, path)
			}
			return nil
		})
	}
	if fileExists(filepath.Join(sessionDir, "datasets")) {
		_ = filepath.Walk(filepath.Join(sessionDir, "datasets"), func(path string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".zip") {
				zipCandidates = append(zipCandidates, path)
			}
			return nil
		})
	}

	// Try extracting from candidate zips
	seenZips := make(map[string]bool)
	for _, zPath := range zipCandidates {
		if seenZips[zPath] || !fileExists(zPath) {
			continue
		}
		seenZips[zPath] = true

		r, err := zip.OpenReader(zPath)
		if err != nil {
			continue
		}

		var targetEntry *zip.File
		for _, f := range r.File {
			if f.FileInfo().IsDir() {
				continue
			}
			fName := filepath.ToSlash(f.Name)
			fBase := filepath.Base(f.Name)

			if archiveEntry != "" && (strings.EqualFold(fName, filepath.ToSlash(archiveEntry)) || strings.EqualFold(fBase, filepath.Base(archiveEntry))) {
				targetEntry = f
				break
			}
			if targetFilename != "" && strings.EqualFold(fBase, targetFilename) {
				targetEntry = f
				break
			}
			if dataset.StoragePath != "" && strings.EqualFold(fBase, filepath.Base(dataset.StoragePath)) {
				targetEntry = f
				break
			}
		}

		if targetEntry != nil {
			// Determine output destination
			destPath := dataset.StoragePath
			if destPath == "" || !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(sessionDir)) {
				destPath = filepath.Join(datasetDir, filepath.Base(targetEntry.Name))
			}

			_ = os.MkdirAll(filepath.Dir(destPath), 0755)
			rc, err := targetEntry.Open()
			if err == nil {
				outFile, createErr := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
				if createErr == nil {
					_, _ = io.Copy(outFile, rc)
					_ = outFile.Close()
				}
				_ = rc.Close()
			}

			// Also extract matching sidecar files (e.g. pgw, wld, prj) if present in zip
			entryBaseNoExt := strings.TrimSuffix(filepath.Base(targetEntry.Name), filepath.Ext(targetEntry.Name))
			for _, sidecarFile := range r.File {
				if sidecarFile.FileInfo().IsDir() {
					continue
				}
				scBase := filepath.Base(sidecarFile.Name)
				scBaseNoExt := strings.TrimSuffix(scBase, filepath.Ext(scBase))
				scExt := strings.ToLower(filepath.Ext(scBase))
				if strings.EqualFold(scBaseNoExt, entryBaseNoExt) && (scExt == ".pgw" || scExt == ".jgw" || scExt == ".prj" || scExt == ".wld" || scExt == ".tfw") {
					scDest := filepath.Join(filepath.Dir(destPath), scBase)
					if scRc, scErr := sidecarFile.Open(); scErr == nil {
						if scOut, scOutErr := os.OpenFile(scDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); scOutErr == nil {
							_, _ = io.Copy(scOut, scRc)
							_ = scOut.Close()
						}
						_ = scRc.Close()
					}
				}
			}

			_ = r.Close()

			if fileExists(destPath) {
				dataset.StoragePath = destPath
				_ = s.repo.UpdateDataset(dataset.SessionID, dataset)
				return destPath
			}
		}
		_ = r.Close()
	}

	// 7. Check sibling datasets in the same session
	datasetsParent := filepath.Join(sessionDir, "datasets")
	if fileExists(datasetsParent) {
		var siblingMatch string
		_ = filepath.Walk(datasetsParent, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if SupportedRasterExtensions[ext] == "" || ext == ".vrt" {
				return nil
			}
			base := filepath.Base(path)
			if strings.EqualFold(base, targetFilename) ||
				(archiveEntry != "" && strings.EqualFold(base, filepath.Base(archiveEntry))) {
				siblingMatch = path
				return filepath.SkipAll
			}
			return nil
		})
		if siblingMatch != "" && fileExists(siblingMatch) {
			destPath := filepath.Join(datasetDir, filepath.Base(siblingMatch))
			if data, rErr := os.ReadFile(siblingMatch); rErr == nil {
				_ = os.MkdirAll(datasetDir, 0755)
				if wErr := os.WriteFile(destPath, data, 0644); wErr == nil {
					dataset.StoragePath = destPath
					_ = s.repo.UpdateDataset(dataset.SessionID, dataset)
					return destPath
				}
			}
			// Fallback directly to the sibling match
			dataset.StoragePath = siblingMatch
			_ = s.repo.UpdateDataset(dataset.SessionID, dataset)
			return siblingMatch
		}
	}

	return ""
}

// isIconAsset determines whether an image file is an icon/symbol marker rather than a spatial map raster.
func isIconAsset(filename string, width, height int, isGeoreferenced bool) bool {
	if isGeoreferenced {
		return false
	}
	clean := strings.ToLower(filepath.Base(filename))
	cleanWithoutExt := strings.TrimSuffix(clean, filepath.Ext(clean))

	// Direct icon directory or icon prefix naming
	if strings.HasPrefix(clean, "icon-") || strings.HasPrefix(clean, "icon_") || strings.HasPrefix(clean, "ico_") {
		return true
	}

	// Known default icon names
	for _, def := range []string{"cinema", "fingerprint", "key", "locked-with-key", "locked", "person-police-officer", "security-lock", "shield", "unlocked", "video-call", "camera", "001-camera"} {
		if cleanWithoutExt == def || strings.Contains(cleanWithoutExt, def) {
			if width <= 256 && height <= 256 {
				return true
			}
		}
	}

	// Tiny pictorial images (standard icon dimensions <= 96x96 without spatial sidecars)
	if width > 0 && height > 0 && width <= 96 && height <= 96 {
		return true
	}

	return false
}

// isTilePath returns true if a zip path represents an XYZ map tile (e.g. 15/123/456.png).
func isTilePath(relPath string) bool {
	clean := filepath.ToSlash(relPath)
	parts := strings.Split(clean, "/")
	if len(parts) >= 3 {
		// Last part is y.ext, middle is x, previous is z
		yPart := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
		xPart := parts[len(parts)-2]
		zPart := parts[len(parts)-3]
		isDigits := func(s string) bool {
			if s == "" {
				return false
			}
			for _, r := range s {
				if r < '0' || r > '9' {
					return false
				}
			}
			return true
		}
		if isDigits(zPart) && isDigits(xPart) && isDigits(yPart) {
			return true
		}
	}
	return false
}
