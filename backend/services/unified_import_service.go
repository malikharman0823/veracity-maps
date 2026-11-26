package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/storage"
)

var (
	ErrUnifiedNotFound       = errors.New("resource not found")
	ErrUnifiedForbidden      = errors.New("access denied: resource belongs to another user")
	ErrTooManyConcurrentJobs = errors.New("too many concurrent import jobs for this user")
	ErrJobNotCancellable     = errors.New("job is already finished and cannot be cancelled")
)

// UnifiedUpload represents an immutable, validated original file stored in PrivateStorage.
type UnifiedUpload struct {
	ID             string                    `json:"id"`
	OwnerID        string                    `json:"owner_id"`
	ProjectID      string                    `json:"project_id"`
	OriginalName   string                    `json:"original_name"`
	StorageKey     string                    `json:"storage_key"`
	DetectedFormat gis.UploadFormat          `json:"detected_format"`
	ImportCategory gis.ImportCategory        `json:"import_category"`
	FileSize       int64                     `json:"file_size"`
	MimeType       string                    `json:"mime_type"`
	UploadStatus   string                    `json:"upload_status"` // uploaded | inspecting | ready | failed
	Inspection     *gis.FileInspectionResult `json:"inspection,omitempty"`
	CreatedAt      time.Time                 `json:"created_at"`
}

// UnifiedDataset represents a fully parsed, validated, and atomically committed dataset.
type UnifiedDataset struct {
	ID                   string                 `json:"id"`
	OwnerID              string                 `json:"owner_id"`
	ProjectID            string                 `json:"project_id"`
	UploadID             string                 `json:"upload_id"`
	Name                 string                 `json:"name"`
	SourceFormat         gis.UploadFormat       `json:"source_format"`
	DatasetType          gis.UnifiedLayerType   `json:"dataset_type"` // raster | vector | image_overlay
	CRS                  string                 `json:"crs,omitempty"`
	Extent               []float64              `json:"extent,omitempty"`
	RequiresGeoreference bool                   `json:"requires_georeferencing"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	ImportStatus         string                 `json:"import_status"` // ready
	LayerIDs             []string               `json:"layer_ids"`
	CreatedAt            time.Time              `json:"created_at"`
}

// UnifiedLayer represents a single workspace layer derived from a UnifiedDataset.
type UnifiedLayer struct {
	ID                   string               `json:"id"`
	OwnerID              string               `json:"owner_id"`
	ProjectID            string               `json:"project_id"`
	DatasetID            string               `json:"dataset_id"`
	Name                 string               `json:"name"`
	LayerType            gis.UnifiedLayerType `json:"layer_type"` // raster | vector | image_overlay
	SourceFormat         gis.UploadFormat     `json:"source_format"`
	StorageKey           string               `json:"storage_key"`
	CRS                  string               `json:"crs,omitempty"`
	Extent               []float64            `json:"extent,omitempty"`
	Width                int                  `json:"width,omitempty"`
	Height               int                  `json:"height,omitempty"`
	FeatureCount         int                  `json:"feature_count,omitempty"`
	RequiresGeoreference bool                 `json:"requires_georeferencing"`
	Visibility           bool                 `json:"visibility"`
	Opacity              float64              `json:"opacity"`
	ZIndex               int                  `json:"z_index"`
	Status               string               `json:"status"` // ready
	CreatedAt            time.Time            `json:"created_at"`
}

// UnifiedImportJob tracks the lifecycle of an atomic import operation.
type UnifiedImportJob struct {
	ID             string          `json:"id"`
	OwnerID        string          `json:"owner_id"`
	ProjectID      string          `json:"project_id"`
	UploadID       string          `json:"upload_id"`
	DatasetID      string          `json:"dataset_id,omitempty"`
	JobType        string          `json:"job_type"`
	Status         string          `json:"status"` // pending | validating | processing | ready | failed | cancelled
	Progress       int             `json:"progress"`
	ErrorStage     string          `json:"error_stage,omitempty"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	Dataset        *UnifiedDataset `json:"dataset,omitempty"`
	Layers         []*UnifiedLayer `json:"layers,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

// StartImportRequest configures an import from a previously uploaded file.
type StartImportRequest struct {
	ProjectID      string             `json:"project_id"`
	SessionID      string             `json:"session_id,omitempty"`
	ImportCategory gis.ImportCategory `json:"import_category,omitempty"`
	DatasetName    string             `json:"dataset_name,omitempty"`
	SelectedLayers []string           `json:"selected_layers,omitempty"` // GeoPackage layer names
	SelectedPages  []int              `json:"selected_pages,omitempty"`  // PDF 1-based page numbers
	DPI            int                `json:"dpi,omitempty"`
	IdempotencyKey string             `json:"idempotency_key,omitempty"`
	Sync           bool               `json:"sync,omitempty"`
}

// UnifiedImportService orchestrates the 8-stage atomic upload & import pipeline with strict tenant isolation.
type UnifiedImportService struct {
	mu               sync.RWMutex
	privateStorage   *storage.PrivateStorage
	sessionRepo      *repository.SessionRepository
	limits           gis.UploadLimits
	uploads          map[string]*UnifiedUpload
	datasets         map[string]*UnifiedDataset
	layers           map[string]*UnifiedLayer
	jobs             map[string]*UnifiedImportJob
	idempotencyIndex map[string]string // ownerID|uploadID|key -> jobID
	activeJobsByUser map[string]int
	jobCancels       map[string]context.CancelFunc
}

func NewUnifiedImportService(ps *storage.PrivateStorage, repo *repository.SessionRepository, limits gis.UploadLimits) *UnifiedImportService {
	if limits.MaxFileSizeBytes <= 0 {
		limits = gis.DefaultUploadLimits()
	}
	return &UnifiedImportService{
		privateStorage:   ps,
		sessionRepo:      repo,
		limits:           limits,
		uploads:          make(map[string]*UnifiedUpload),
		datasets:         make(map[string]*UnifiedDataset),
		layers:           make(map[string]*UnifiedLayer),
		jobs:             make(map[string]*UnifiedImportJob),
		idempotencyIndex: make(map[string]string),
		activeJobsByUser: make(map[string]int),
		jobCancels:       make(map[string]context.CancelFunc),
	}
}

func newPrefixedID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// UploadAndInspect executes Stage 1 (Upload) + Stage 2 (Inspection) + Stage 3/4 (Deep Parsing & Validation).
// Invalid, corrupted, oversized, or unsupported files are rejected immediately and never persisted.
func (s *UnifiedImportService) UploadAndInspect(
	ctx context.Context,
	ownerID string,
	projectID string,
	category gis.ImportCategory,
	originalFilename string,
	r io.Reader,
) (*UnifiedUpload, error) {
	if strings.TrimSpace(ownerID) == "" {
		return nil, ErrUnifiedForbidden
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = "default"
	}

	tempDir, err := s.privateStorage.UserTempDir(ownerID)
	if err != nil {
		return nil, err
	}
	uploadID := newPrefixedID("upl")
	safeExt := strings.ToLower(filepath.Ext(filepath.Base(originalFilename)))
	stagePath := filepath.Join(tempDir, uploadID+"_incoming"+safeExt)

	// Stream upload to temp staging file while enforcing MaxFileSizeBytes strictly.
	out, err := os.OpenFile(stagePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("create upload staging file: %w", err)
	}

	limited := io.LimitReader(r, s.limits.MaxFileSizeBytes+1)
	written, copyErr := io.Copy(out, limited)
	_ = out.Close()

	cleanupStage := func() {
		_ = os.Remove(stagePath)
	}

	if copyErr != nil {
		cleanupStage()
		return nil, &gis.ValidationError{Stage: "upload", Code: "UPLOAD_STREAM_ERROR", Message: "Failed reading uploaded file stream"}
	}
	if written == 0 {
		cleanupStage()
		return nil, &gis.ValidationError{Stage: "upload", Code: "EMPTY_FILE", Message: "Uploaded file is empty (0 bytes)"}
	}
	if written > s.limits.MaxFileSizeBytes {
		cleanupStage()
		return nil, &gis.ValidationError{
			Stage:   "upload",
			Code:    "FILE_TOO_LARGE",
			Message: fmt.Sprintf("File exceeds maximum allowed size (%d bytes)", s.limits.MaxFileSizeBytes),
		}
	}

	// Enforce per-user private storage quota before inspection/storage.
	if err := s.privateStorage.CheckQuota(ownerID, written); err != nil {
		cleanupStage()
		return nil, &gis.ValidationError{Stage: "upload", Code: "STORAGE_QUOTA_EXCEEDED", Message: err.Error()}
	}

	// Stages 2-4: Magic-byte detection, deep parsing, and format-specific validation.
	inspection, err := gis.InspectAndValidateFile(ctx, stagePath, originalFilename, category, s.limits)
	if err != nil {
		cleanupStage()
		return nil, err
	}

	// Persist immutable original file into owner's private uploads bucket.
	rawBytes, err := os.ReadFile(stagePath)
	cleanupStage()
	if err != nil {
		return nil, fmt.Errorf("read validated upload: %w", err)
	}

	storedPath, err := s.privateStorage.WritePrivateFile(ownerID, projectID, "uploads", originalFilename, rawBytes)
	if err != nil {
		return nil, err
	}

	rec := &UnifiedUpload{
		ID:             uploadID,
		OwnerID:        ownerID,
		ProjectID:      projectID,
		OriginalName:   filepath.Base(originalFilename),
		StorageKey:     storedPath,
		DetectedFormat: inspection.DetectedFormat,
		ImportCategory: inspection.ImportCategory,
		FileSize:       written,
		MimeType:       inspection.MimeType,
		UploadStatus:   "uploaded",
		Inspection:     inspection,
		CreatedAt:      time.Now().UTC(),
	}

	s.mu.Lock()
	s.uploads[uploadID] = rec
	s.mu.Unlock()

	return rec, nil
}

// ListUploads returns only uploads belonging to ownerID, newest first.
func (s *UnifiedImportService) ListUploads(ownerID string) []*UnifiedUpload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*UnifiedUpload, 0)
	for _, u := range s.uploads {
		if u.OwnerID == ownerID {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// GetUpload returns an upload only if it belongs to ownerID.
func (s *UnifiedImportService) GetUpload(ownerID, uploadID string) (*UnifiedUpload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.uploads[uploadID]
	if !ok {
		return nil, ErrUnifiedNotFound
	}
	if u.OwnerID != ownerID {
		return nil, ErrUnifiedForbidden
	}
	return u, nil
}

// StartImport launches an atomic import job (Stages 5-8) with idempotency and concurrency guards.
func (s *UnifiedImportService) StartImport(ownerID, uploadID string, req StartImportRequest) (*UnifiedImportJob, error) {
	upload, err := s.GetUpload(ownerID, uploadID)
	if err != nil {
		return nil, err
	}

	// Verify file still resides strictly inside owner's private storage.
	if _, err := s.privateStorage.VerifyOwnerPath(ownerID, upload.StorageKey); err != nil {
		return nil, ErrUnifiedForbidden
	}

	category := req.ImportCategory
	if category == "" {
		category = upload.ImportCategory
	}
	if !gis.IsFormatAllowedForCategory(upload.DetectedFormat, category) {
		return nil, &gis.ValidationError{
			Stage:   "validation",
			Code:    "CATEGORY_FORMAT_MISMATCH",
			Message: fmt.Sprintf("Format %q is not allowed in %s import mode", upload.DetectedFormat, category),
		}
	}

	idemKey := strings.TrimSpace(req.IdempotencyKey)
	s.mu.Lock()
	if idemKey != "" {
		composite := ownerID + "|" + uploadID + "|" + idemKey
		if existingJobID, exists := s.idempotencyIndex[composite]; exists {
			if existingJob, ok := s.jobs[existingJobID]; ok && existingJob.Status != "failed" && existingJob.Status != "cancelled" {
				s.mu.Unlock()
				return existingJob, nil
			}
		}
	}
	if s.limits.MaxConcurrentJobsPerUser > 0 && s.activeJobsByUser[ownerID] >= s.limits.MaxConcurrentJobsPerUser {
		s.mu.Unlock()
		return nil, ErrTooManyConcurrentJobs
	}

	jobID := newPrefixedID("job")
	projID := strings.TrimSpace(req.ProjectID)
	if projID == "" {
		projID = upload.ProjectID
	}

	job := &UnifiedImportJob{
		ID:             jobID,
		OwnerID:        ownerID,
		ProjectID:      projID,
		UploadID:       uploadID,
		JobType:        fmt.Sprintf("import_%s_%s", category, upload.DetectedFormat),
		Status:         "processing",
		Progress:       10,
		IdempotencyKey: idemKey,
		CreatedAt:      time.Now().UTC(),
	}

	jobCtx, cancel := context.WithTimeout(context.Background(), s.limits.ProcessingTimeout)
	s.jobs[jobID] = job
	s.activeJobsByUser[ownerID]++
	s.jobCancels[jobID] = cancel
	if idemKey != "" {
		s.idempotencyIndex[ownerID+"|"+uploadID+"|"+idemKey] = jobID
	}
	s.mu.Unlock()

	if req.Sync {
		s.runImportJob(jobCtx, job, upload, category, req)
		return s.copyJob(jobID), nil
	}

	go s.runImportJob(jobCtx, job, upload, category, req)
	return s.copyJob(jobID), nil
}

func (s *UnifiedImportService) copyJob(jobID string) *UnifiedImportJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j := s.jobs[jobID]
	if j == nil {
		return nil
	}
	cp := *j
	return &cp
}

func (s *UnifiedImportService) runImportJob(ctx context.Context, job *UnifiedImportJob, upload *UnifiedUpload, category gis.ImportCategory, req StartImportRequest) {
	tempRoot, err := s.privateStorage.UserTempDir(job.OwnerID)
	stagingDir := filepath.Join(tempRoot, job.ID)
	var committedFiles []string

	defer func() {
		// Always clean up staging temporary directory on exit (success, failure, or cancellation).
		if stagingDir != "" {
			_ = storage.SafeRemoveAll(stagingDir)
		}
		s.mu.Lock()
		if cancel, ok := s.jobCancels[job.ID]; ok {
			cancel()
			delete(s.jobCancels, job.ID)
		}
		if s.activeJobsByUser[job.OwnerID] > 0 {
			s.activeJobsByUser[job.OwnerID]--
		}
		s.mu.Unlock()
	}()

	failJob := func(stage, code, msg string) {
		for _, f := range committedFiles {
			_ = os.Remove(f)
		}
		now := time.Now().UTC()
		s.mu.Lock()
		if job.Status != "cancelled" {
			job.Status = "failed"
			job.ErrorStage = stage
			job.ErrorCode = code
			job.ErrorMessage = msg
			job.CompletedAt = &now
		}
		s.mu.Unlock()
	}

	cancelJob := func() {
		for _, f := range committedFiles {
			_ = os.Remove(f)
		}
		now := time.Now().UTC()
		s.mu.Lock()
		job.Status = "cancelled"
		job.ErrorStage = "cancelled"
		job.ErrorCode = "JOB_CANCELLED"
		job.ErrorMessage = "Import job was cancelled by user"
		job.CompletedAt = &now
		s.mu.Unlock()
	}

	if err != nil {
		failJob("staging", "STAGING_DIR_ERROR", err.Error())
		return
	}
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		failJob("staging", "STAGING_DIR_ERROR", err.Error())
		return
	}

	if ctx.Err() != nil {
		cancelJob()
		return
	}

	type stagedLayerSpec struct {
		Name                 string
		LayerType            gis.UnifiedLayerType
		SourceFormat         gis.UploadFormat
		StagedFilePath       string
		CRS                  string
		Extent               []float64
		Width                int
		Height               int
		FeatureCount         int
		RequiresGeoreference bool
	}

	var specs []stagedLayerSpec
	baseName := strings.TrimSpace(req.DatasetName)
	if baseName == "" {
		baseName = strings.TrimSuffix(upload.OriginalName, filepath.Ext(upload.OriginalName))
	}

	// Stage 5: Execute format-specific handler into isolated stagingDir.
	switch upload.DetectedFormat {
	case gis.FormatPNG, gis.FormatJPG:
		layerType := gis.LayerTypeRaster
		if category == gis.ImportCategoryVector {
			layerType = gis.LayerTypeImageOverlay
		}
		data, rErr := s.privateStorage.ReadPrivateFile(job.OwnerID, upload.StorageKey)
		if rErr != nil {
			failJob("processing", "READ_ORIGINAL_FAILED", rErr.Error())
			return
		}
		stagedPath := filepath.Join(stagingDir, "prepared"+filepath.Ext(upload.OriginalName))
		if wErr := os.WriteFile(stagedPath, data, 0600); wErr != nil {
			failJob("processing", "STAGE_WRITE_FAILED", wErr.Error())
			return
		}
		var w, h int
		if upload.Inspection != nil {
			w = upload.Inspection.Width
			h = upload.Inspection.Height
		}
		specs = append(specs, stagedLayerSpec{
			Name:                 baseName,
			LayerType:            layerType,
			SourceFormat:         upload.DetectedFormat,
			StagedFilePath:       stagedPath,
			Width:                w,
			Height:               h,
			RequiresGeoreference: true,
		})

	case gis.FormatGeoJSON:
		// Re-validate before processing to guarantee integrity.
		insp, vErr := gis.InspectAndValidateFile(ctx, upload.StorageKey, upload.OriginalName, gis.ImportCategoryVector, s.limits)
		if vErr != nil {
			if ve, ok := vErr.(*gis.ValidationError); ok {
				failJob(ve.Stage, ve.Code, ve.Message)
			} else {
				failJob("validation", "GEOJSON_INVALID", vErr.Error())
			}
			return
		}
		data, rErr := s.privateStorage.ReadPrivateFile(job.OwnerID, upload.StorageKey)
		if rErr != nil {
			failJob("processing", "READ_ORIGINAL_FAILED", rErr.Error())
			return
		}
		stagedPath := filepath.Join(stagingDir, "normalized.geojson")
		if wErr := os.WriteFile(stagedPath, data, 0600); wErr != nil {
			failJob("processing", "STAGE_WRITE_FAILED", wErr.Error())
			return
		}
		specs = append(specs, stagedLayerSpec{
			Name:                 baseName,
			LayerType:            gis.LayerTypeVector,
			SourceFormat:         gis.FormatGeoJSON,
			StagedFilePath:       stagedPath,
			CRS:                  insp.CRS,
			Extent:               insp.Extent,
			FeatureCount:         insp.FeatureCount,
			RequiresGeoreference: false,
		})

	case gis.FormatGPKG, gis.FormatPDF, gis.FormatDXF:
		action := "process-" + string(upload.DetectedFormat)
		payload := map[string]interface{}{
			"action":              action,
			"file_path":           upload.StorageKey,
			"staging_dir":         stagingDir,
			"selected_layers":     req.SelectedLayers,
			"selected_pages":      req.SelectedPages,
			"dpi":                 req.DPI,
			"max_pdf_pages":       s.limits.MaxPDFPages,
			"max_pdf_dim_pt":      s.limits.MaxPDFPageDimensionPt,
			"max_cad_entities":    s.limits.MaxCADEntities,
			"max_gpkg_features":   s.limits.MaxGeoPackageFeatures,
			"max_image_dimension": s.limits.MaxImageDimension,
		}
		res, pErr := gis.RunPythonUnifiedProcess(ctx, payload, s.limits.ProcessingTimeout)
		if ctx.Err() != nil {
			cancelJob()
			return
		}
		if pErr != nil {
			if ve, ok := pErr.(*gis.ValidationError); ok {
				failJob(ve.Stage, ve.Code, ve.Message)
			} else {
				failJob("processing", "PROCESSING_FAILED", pErr.Error())
			}
			return
		}
		rawOuts, _ := res["outputs"].([]interface{})
		if len(rawOuts) == 0 {
			failJob("processing", "NO_OUTPUTS_GENERATED", "Processing produced zero valid layers")
			return
		}
		for _, ro := range rawOuts {
			m, ok := ro.(map[string]interface{})
			if !ok {
				continue
			}
			outPath, _ := m["file_path"].(string)
			name, _ := m["name"].(string)
			if name == "" {
				name = baseName
			} else if upload.DetectedFormat == gis.FormatPDF {
				name = fmt.Sprintf("%s - %s", baseName, name)
			}
			lType := gis.UnifiedLayerType(fmt.Sprintf("%v", m["layer_type"]))
			crsStr, _ := m["crs"].(string)
			reqGeo, _ := m["requires_georeferencing"].(bool)
			var extent []float64
			if rawExt, ok := m["extent"].([]interface{}); ok && len(rawExt) == 4 {
				for _, v := range rawExt {
					if fv, ok := v.(float64); ok {
						extent = append(extent, fv)
					}
				}
			}
			w, _ := m["width"].(float64)
			h, _ := m["height"].(float64)
			fc, _ := m["feature_count"].(float64)

			specs = append(specs, stagedLayerSpec{
				Name:                 name,
				LayerType:            lType,
				SourceFormat:         upload.DetectedFormat,
				StagedFilePath:       outPath,
				CRS:                  crsStr,
				Extent:               extent,
				Width:                int(w),
				Height:               int(h),
				FeatureCount:         int(fc),
				RequiresGeoreference: reqGeo,
			})
		}
	default:
		failJob("validation", "UNSUPPORTED_FORMAT", "Unsupported upload format")
		return
	}

	// Stage 6 & 7: Verify EVERY staged output file exists and is non-empty (atomic guarantee).
	if len(specs) == 0 {
		failJob("validation", "NO_LAYERS_PRODUCED", "Import did not produce any valid layers")
		return
	}
	for _, sp := range specs {
		info, sErr := os.Stat(sp.StagedFilePath)
		if sErr != nil || info.IsDir() || info.Size() == 0 {
			failJob("validation", "OUTPUT_VALIDATION_FAILED", fmt.Sprintf("Staged output for layer %q is missing or empty", sp.Name))
			return
		}
	}

	// Final cancellation check BEFORE atomic registration.
	if ctx.Err() != nil {
		cancelJob()
		return
	}

	// Copy validated outputs into owner's private processed bucket.
	datasetID := newPrefixedID("ds")
	now := time.Now().UTC()
	var layers []*UnifiedLayer
	var layerIDs []string

	for idx, sp := range specs {
		bytesOut, rErr := os.ReadFile(sp.StagedFilePath)
		if rErr != nil {
			failJob("registration", "STAGED_READ_FAILED", rErr.Error())
			return
		}
		storedOut, wErr := s.privateStorage.WritePrivateFile(job.OwnerID, job.ProjectID, "processed", filepath.Base(sp.StagedFilePath), bytesOut)
		if wErr != nil {
			failJob("registration", "PRIVATE_STORAGE_WRITE_FAILED", wErr.Error())
			return
		}
		committedFiles = append(committedFiles, storedOut)

		lyrID := newPrefixedID("lyr")
		layerIDs = append(layerIDs, lyrID)
		layers = append(layers, &UnifiedLayer{
			ID:                   lyrID,
			OwnerID:              job.OwnerID,
			ProjectID:            job.ProjectID,
			DatasetID:            datasetID,
			Name:                 sp.Name,
			LayerType:            sp.LayerType,
			SourceFormat:         sp.SourceFormat,
			StorageKey:           storedOut,
			CRS:                  sp.CRS,
			Extent:               sp.Extent,
			Width:                sp.Width,
			Height:               sp.Height,
			FeatureCount:         sp.FeatureCount,
			RequiresGeoreference: sp.RequiresGeoreference,
			Visibility:           true,
			Opacity:              1.0,
			ZIndex:               idx,
			Status:               "ready",
			CreatedAt:            now,
		})
	}

	ds := &UnifiedDataset{
		ID:                   datasetID,
		OwnerID:              job.OwnerID,
		ProjectID:            job.ProjectID,
		UploadID:             upload.ID,
		Name:                 baseName,
		SourceFormat:         upload.DetectedFormat,
		DatasetType:          layers[0].LayerType,
		CRS:                  layers[0].CRS,
		Extent:               layers[0].Extent,
		RequiresGeoreference: layers[0].RequiresGeoreference,
		ImportStatus:         "ready",
		LayerIDs:             layerIDs,
		CreatedAt:            now,
	}

	// Stage 8: Atomic commit of Dataset + all Layers + Job status.
	s.mu.Lock()
	if job.Status == "cancelled" {
		s.mu.Unlock()
		cancelJob()
		return
	}
	s.datasets[datasetID] = ds
	for _, l := range layers {
		s.layers[l.ID] = l
	}
	job.DatasetID = datasetID
	job.Dataset = ds
	job.Layers = layers
	job.Status = "ready"
	job.Progress = 100
	job.CompletedAt = &now
	s.mu.Unlock()
}

// GetJob returns job status only if it belongs to ownerID.
func (s *UnifiedImportService) GetJob(ownerID, jobID string) (*UnifiedImportJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[jobID]
	if !ok {
		return nil, ErrUnifiedNotFound
	}
	if j.OwnerID != ownerID {
		return nil, ErrUnifiedForbidden
	}
	cp := *j
	return &cp, nil
}

// CancelJob cancels an in-flight job belonging to ownerID and guarantees no partial workspace records.
func (s *UnifiedImportService) CancelJob(ownerID, jobID string) (*UnifiedImportJob, error) {
	s.mu.Lock()
	j, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return nil, ErrUnifiedNotFound
	}
	if j.OwnerID != ownerID {
		s.mu.Unlock()
		return nil, ErrUnifiedForbidden
	}
	if j.Status == "ready" || j.Status == "failed" || j.Status == "cancelled" {
		s.mu.Unlock()
		return nil, ErrJobNotCancellable
	}
	now := time.Now().UTC()
	j.Status = "cancelled"
	j.ErrorStage = "cancelled"
	j.ErrorCode = "JOB_CANCELLED"
	j.ErrorMessage = "Import job cancelled"
	j.CompletedAt = &now
	cancel := s.jobCancels[jobID]
	cp := *j
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return &cp, nil
}

// ListDatasets returns all imported datasets belonging to ownerID (optionally filtered by projectID).
func (s *UnifiedImportService) ListDatasets(ownerID, projectID string) []*UnifiedDataset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*UnifiedDataset, 0)
	for _, ds := range s.datasets {
		if ds.OwnerID != ownerID {
			continue
		}
		if projectID != "" && ds.ProjectID != projectID {
			continue
		}
		out = append(out, ds)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// GetDataset returns a dataset and its layers only if owned by ownerID.
func (s *UnifiedImportService) GetDataset(ownerID, datasetID string) (*UnifiedDataset, []*UnifiedLayer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ds, ok := s.datasets[datasetID]
	if !ok {
		return nil, nil, ErrUnifiedNotFound
	}
	if ds.OwnerID != ownerID {
		return nil, nil, ErrUnifiedForbidden
	}
	var layers []*UnifiedLayer
	for _, lid := range ds.LayerIDs {
		if l, ok := s.layers[lid]; ok && l.OwnerID == ownerID {
			layers = append(layers, l)
		}
	}
	return ds, layers, nil
}

// DeleteDataset deletes a dataset, its derived layers, and their processed files (original upload untouched).
func (s *UnifiedImportService) DeleteDataset(ownerID, datasetID string) error {
	s.mu.Lock()
	ds, ok := s.datasets[datasetID]
	if !ok {
		s.mu.Unlock()
		return ErrUnifiedNotFound
	}
	if ds.OwnerID != ownerID {
		s.mu.Unlock()
		return ErrUnifiedForbidden
	}
	var filesToRemove []string
	for _, lid := range ds.LayerIDs {
		if l, ok := s.layers[lid]; ok {
			filesToRemove = append(filesToRemove, l.StorageKey)
			delete(s.layers, lid)
		}
	}
	delete(s.datasets, datasetID)
	s.mu.Unlock()

	for _, f := range filesToRemove {
		_ = s.privateStorage.DeletePrivateFile(ownerID, f)
	}
	return nil
}
