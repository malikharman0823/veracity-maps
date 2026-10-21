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
		if SupportedRasterExtensions[entryExt] != "" && primaryRaster == "" {
			primaryRaster = cleanName
		}
	}

	if primaryRaster == "" {
		_ = storage.SafeRemoveAll(datasetDir)
		return nil, fmt.Errorf("NO_SUPPORTED_RASTER: The ZIP archive does not contain a supported PNG or JPEG image")
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
