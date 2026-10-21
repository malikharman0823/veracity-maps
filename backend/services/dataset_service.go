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
	if ext != ".zip" && SupportedRasterExtensions[ext] == "" {
		return nil, fmt.Errorf("INVALID_FILE_TYPE: Unsupported file format '%s'. Supported formats are PNG, JPEG, and ZIP", ext)
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
	return s.handleRawRasterDataset(sessionID, datasetID, datasetDir, destPath, fileHeader.Filename, size, fileHash, SupportedRasterExtensions[ext])
}

func (s *DatasetService) handleRawRasterDataset(sessionID, datasetID, datasetDir, destPath, filename string, size int64, fileHash, contentType string) (*models.Dataset, error) {
	meta, err := gis.InspectRasterFile(destPath)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("CORRUPTED_FILE: %w", err)
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

	georefStatus := "not_started"
	var georefPath string
	if meta.IsGeoreferenced {
		georefStatus = "completed"
		georefPath = destPath
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
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, err
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
	var totalExtracted int64

	for _, file := range zipReader.File {
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
		if SupportedRasterExtensions[entryExt] != "" {
			if primaryRaster == "" {
				primaryRaster = cleanName
			}
			allRasters = append(allRasters, cleanName)
		}
	}

	if primaryRaster == "" {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("NO_SUPPORTED_RASTER: The ZIP archive does not contain a supported PNG, JPEG, or TIFF image")
	}

	meta, err := gis.InspectRasterFile(primaryRaster)
	if err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("CORRUPTED_FILE: Failed inspecting raster inside zip: %w", err)
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

	georefStatus := "not_started"
	var georefPath string
	if meta.IsGeoreferenced {
		georefStatus = "completed"
		georefPath = primaryRaster
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
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Restore exported package metadata, control points, and project configuration if present
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

				if len(doc.ControlPoints) > 0 {
					for _, p := range doc.ControlPoints {
						p.DatasetID = datasetID
					}
					_ = s.repo.SetControlPoints(sessionID, datasetID, doc.ControlPoints)
					dataset.GeoreferenceStatus = "completed"
					if dataset.GeoreferencedPath == "" {
						dataset.GeoreferencedPath = primaryRaster
					}

					// If transform/bounds not yet set, compute georeference from control points
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
							// Also write world file next to primary raster
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

				if doc.Project.Configuration != nil {
					// Remap all members to new datasetID
					if rawMembers, ok := doc.Project.Configuration["members"].([]interface{}); ok {
						for _, rm := range rawMembers {
							if m, ok := rm.(map[string]interface{}); ok {
								m["dataset_id"] = datasetID
								m["name"] = dataset.Filename
								if dataset.MapLayer != nil {
									m["map_layer"] = dataset.MapLayer
								}
								if dataset.ExtractedMetadata["bounds"] != nil {
									m["bounds"] = dataset.ExtractedMetadata["bounds"]
								}
							}
						}
					}
					_ = s.repo.SaveMapProject(sessionID, &models.MapProject{
						ID:            "proj_" + datasetID,
						DatasetID:     datasetID,
						Name:          doc.Project.Name,
						Configuration: doc.Project.Configuration,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					})
				}
			}
		}
	}

	// 2. Check metadata.json for bounds fallback
	metaJSONPath := filepath.Join(extractDir, "metadata.json")
	if fileExists(metaJSONPath) {
		if data, err := os.ReadFile(metaJSONPath); err == nil {
			var mDoc struct {
				Bounds []float64 `json:"bounds"`
			}
			if err := json.Unmarshal(data, &mDoc); err == nil && len(mDoc.Bounds) == 4 {
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
		}
	}

	if err := s.repo.InsertDataset(sessionID, dataset); err != nil {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, err
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
		if SupportedRasterExtensions[ext] != "" {
			rasterFiles = append(rasterFiles, f)
		}
	}

	if len(rasterFiles) == 0 {
		return nil, fmt.Errorf("NO_SUPPORTED_RASTER: The ZIP archive does not contain any supported image files (PNG, JPG, TIFF, WebP)")
	}

	tempInspectDir := filepath.Join(importsDir, "staging")
	_ = os.MkdirAll(tempInspectDir, 0755)

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

		suggestedBld := inferBuilding(dir, archiveBase)
		suggestedFloor, suggestedFloorLabel := inferFloor(base, dir)

		status := "ready"
		var warnings []string
		if !isGeoref {
			warnings = append(warnings, "No georeferencing metadata found. Layer will require manual georeferencing.")
		}

		cleanDisplayName := strings.Title(strings.ReplaceAll(strings.ReplaceAll(base, "-", " "), "_", " "))

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

	return &ImportArchiveResponse{
		ImportedCount: importedCount,
		FailedCount:   failedCount,
		Results:       results,
	}, nil
}

func inferFloor(name, folder string) (int, string) {
	combined := strings.ToLower(name + " " + folder)
	if strings.Contains(combined, "basement 2") || strings.Contains(combined, "b2") || strings.Contains(combined, "sub-basement") {
		return -2, "Basement 2"
	}
	if strings.Contains(combined, "basement 1") || strings.Contains(combined, "basement") || strings.Contains(combined, "b1") || strings.Contains(combined, "cellar") {
		return -1, "Basement 1"
	}
	if strings.Contains(combined, "ground") || strings.Contains(combined, "floor 0") || strings.Contains(combined, "level 0") || strings.Contains(combined, "g-floor") {
		return 0, "Ground Floor"
	}
	if strings.Contains(combined, "first floor") || strings.Contains(combined, "floor 1") || strings.Contains(combined, "level 1") || strings.Contains(combined, "1st floor") || strings.Contains(combined, "1f") {
		return 1, "First Floor"
	}
	if strings.Contains(combined, "second floor") || strings.Contains(combined, "floor 2") || strings.Contains(combined, "level 2") || strings.Contains(combined, "2nd floor") || strings.Contains(combined, "2f") {
		return 2, "Second Floor"
	}
	if strings.Contains(combined, "third floor") || strings.Contains(combined, "floor 3") || strings.Contains(combined, "level 3") || strings.Contains(combined, "3rd floor") || strings.Contains(combined, "3f") {
		return 3, "Third Floor"
	}
	if strings.Contains(combined, "roof") || strings.Contains(combined, "terrace") {
		return 4, "Roof / Terrace"
	}
	return 0, "Ground Floor"
}

func inferBuilding(folder, archiveName string) string {
	parts := strings.Split(filepath.ToSlash(folder), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "" && part != "." && !strings.EqualFold(part, "source") && !strings.EqualFold(part, "images") && !strings.EqualFold(part, "rasters") {
			clean := strings.ReplaceAll(part, "-", " ")
			clean = strings.ReplaceAll(clean, "_", " ")
			return strings.Title(strings.ToLower(clean))
		}
	}
	if archiveName != "" {
		base := strings.TrimSuffix(archiveName, filepath.Ext(archiveName))
		clean := strings.ReplaceAll(base, "-", " ")
		clean = strings.ReplaceAll(clean, "_", " ")
		return strings.Title(strings.ToLower(clean))
	}
	return "Building"
}
