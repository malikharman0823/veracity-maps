package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type queuedProcessingTask struct {
	jobID     string
	userID    string
	sessionID string
	runner    func(ctx context.Context, job *models.ProcessingJob) (map[string]interface{}, error)
	doneCh    chan struct{}
}

// ProcessingService implements the centralized Phase 2 GIS Processing Engine and Worker Pool.
type ProcessingService struct {
	procRepo    repository.ProcessingRepo
	gisService  *GISService
	gisRepo     repository.GISRepo
	mapRepo     repository.MapRepo
	sessionRepo *repository.SessionRepository
	tempDir     string

	mu            sync.Mutex
	activeCancels map[string]context.CancelFunc
	queue         chan *queuedProcessingTask
}

func NewProcessingService(
	procRepo repository.ProcessingRepo,
	gisService *GISService,
	mapRepo repository.MapRepo,
	sessionRepo *repository.SessionRepository,
	tempDir string,
) *ProcessingService {
	if procRepo == nil {
		procRepo = repository.NewMemoryProcessingRepository()
	}
	if tempDir == "" {
		tempDir = filepath.Join(os.TempDir(), "veracity-processing")
	}
	s := &ProcessingService{
		procRepo:      procRepo,
		gisService:    gisService,
		gisRepo:       gisService.GetGISRepo(),
		mapRepo:       mapRepo,
		sessionRepo:   sessionRepo,
		tempDir:       tempDir,
		activeCancels: make(map[string]context.CancelFunc),
		queue:         make(chan *queuedProcessingTask, 128),
	}
	// Launch 4 controlled background worker goroutines
	for i := 0; i < 4; i++ {
		go s.workerLoop()
	}
	return s
}

func (s *ProcessingService) workerLoop() {
	for task := range s.queue {
		s.executeTask(task)
	}
}

func (s *ProcessingService) executeTask(task *queuedProcessingTask) {
	defer close(task.doneCh)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.mu.Lock()
	// Check if already cancelled while queued
	jobNow, err := s.procRepo.GetJob(ctx, task.jobID)
	if err != nil || jobNow.Status == models.JobStatusCancelled {
		s.mu.Unlock()
		cancel()
		return
	}
	s.activeCancels[task.jobID] = cancel
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.activeCancels, task.jobID)
		s.mu.Unlock()
		cancel()
	}()

	_ = s.UpdateProcessingProgress(ctx, task.jobID, models.JobStatusValidating, "Validating input layers and CRS", 15)
	_ = s.procRepo.AppendJobLog(ctx, task.jobID, "Validation", "INFO", fmt.Sprintf("Starting validation for algorithm '%s'", jobNow.AlgorithmName))

	if ctx.Err() != nil {
		_, _ = s.CancelProcessingJob(context.Background(), task.jobID, task.userID)
		return
	}

	_ = s.UpdateProcessingProgress(ctx, task.jobID, models.JobStatusRunning, "Executing GIS algorithm", 35)
	_ = s.procRepo.AppendJobLog(ctx, task.jobID, "Execution", "INFO", "Executing spatial operation on input features/rasters")

	outputCfg, runErr := task.runner(ctx, jobNow)
	if ctx.Err() == context.Canceled {
		_ = s.procRepo.UpdateJobStatus(context.Background(), task.jobID, models.JobStatusCancelled, "Cancelled", jobNow.Progress, "JOB_CANCELLED", "Job was cancelled by user", nil)
		_ = s.procRepo.AppendJobLog(context.Background(), task.jobID, "Cancelled", "WARN", "Processing job cancelled")
		return
	}
	if runErr != nil {
		_ = s.FailProcessingJob(context.Background(), task.jobID, "PROCESSING_FAILED", runErr.Error())
		return
	}

	_ = s.CompleteProcessingJob(context.Background(), task.jobID, outputCfg)
}

// ============================================================================
// COMMON PROCESSING ENGINE LIFECYCLE METHODS
// ============================================================================

func (s *ProcessingService) GetProcessingAlgorithms() []models.ProcessingAlgorithm {
	return gis.GetProcessingAlgorithms()
}

func (s *ProcessingService) GetAlgorithmDetails(algorithmID string) (*models.ProcessingAlgorithm, error) {
	return gis.GetAlgorithmDetails(algorithmID)
}

func (s *ProcessingService) ValidateProcessingRequest(req models.ProcessingJobRequest) (*models.ProcessingAlgorithm, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	alg, err := gis.GetAlgorithmDetails(req.AlgorithmName)
	if err != nil {
		return nil, models.NewValidationError("UNKNOWN_ALGORITHM", err.Error(), "algorithm_name")
	}
	for _, param := range alg.Parameters {
		if param.Required {
			val, exists := req.Parameters[param.Name]
			if !exists || val == nil || fmt.Sprintf("%v", val) == "" {
				// Allow fallback if InputLayerIDs or InputDatasetIDs was provided
				if param.Type == "layer" && len(req.InputLayerIDs) > 0 {
					continue
				}
				if param.Type == "dataset" && len(req.InputDatasetIDs) > 0 {
					continue
				}
				return nil, models.NewValidationError("MISSING_PARAMETER", fmt.Sprintf("parameter '%s' is required for algorithm '%s'", param.Name, alg.Name), param.Name)
			}
		}
	}
	return alg, nil
}

func (s *ProcessingService) CreateProcessingJob(ctx context.Context, userID, mapID, projectID, algName string, inputCfg map[string]interface{}, inputs []models.ProcessingJobInput) (*models.ProcessingJob, error) {
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}
	job := &models.ProcessingJob{
		ProjectID:     projectID,
		MapID:         mapID,
		UserID:        userID,
		AlgorithmName: algName,
		Status:        models.JobStatusQueued,
		Progress:      5,
		Stage:         "Queued",
		InputConfig:   inputCfg,
		Inputs:        inputs,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.procRepo.CreateJob(ctx, job); err != nil {
		return nil, err
	}
	_ = s.procRepo.AppendJobLog(ctx, job.JobID, "Queued", "INFO", fmt.Sprintf("Job %s created and queued for '%s'", job.JobID, algName))
	return job, nil
}

func (s *ProcessingService) QueueProcessingJob(
	ctx context.Context,
	job *models.ProcessingJob,
	userID, sessionID string,
	waitBrief bool,
	runner func(ctx context.Context, job *models.ProcessingJob) (map[string]interface{}, error),
) (*models.ProcessingJob, error) {
	task := &queuedProcessingTask{
		jobID:     job.JobID,
		userID:    userID,
		sessionID: sessionID,
		runner:    runner,
		doneCh:    make(chan struct{}),
	}
	s.queue <- task

	if waitBrief {
		select {
		case <-task.doneCh:
		case <-time.After(2 * time.Second):
		}
	}
	return s.procRepo.GetJob(ctx, job.JobID)
}

func (s *ProcessingService) UpdateProcessingProgress(ctx context.Context, jobID, status, stage string, progress int) error {
	return s.procRepo.UpdateJobStatus(ctx, jobID, status, stage, progress, "", "", nil)
}

func (s *ProcessingService) CompleteProcessingJob(ctx context.Context, jobID string, outCfg map[string]interface{}) error {
	summaryParts := []string{}
	if name, ok := outCfg["output_layer_name"].(string); ok && name != "" {
		summaryParts = append(summaryParts, fmt.Sprintf("Output Layer: %q", name))
	}
	if fc, ok := outCfg["feature_count"]; ok {
		summaryParts = append(summaryParts, fmt.Sprintf("Features: %v", fc))
	}
	if ta, ok := outCfg["total_area"]; ok {
		summaryParts = append(summaryParts, fmt.Sprintf("Total Area: %.6g", toFloatSafe(ta)))
	}
	if tl, ok := outCfg["total_length"]; ok {
		summaryParts = append(summaryParts, fmt.Sprintf("Total Length: %.6g", toFloatSafe(tl)))
	}
	if vc, ok := outCfg["valid_count"]; ok {
		summaryParts = append(summaryParts, fmt.Sprintf("Valid: %v", vc))
	}
	if ic, ok := outCfg["invalid_count"]; ok {
		summaryParts = append(summaryParts, fmt.Sprintf("Invalid: %v", ic))
	}
	msg := "Processing completed and output registered"
	if len(summaryParts) > 0 {
		msg = fmt.Sprintf("Processing completed (%s)", strings.Join(summaryParts, ", "))
	}
	_ = s.procRepo.AppendJobLog(ctx, jobID, "Completed", "INFO", msg)
	return s.procRepo.UpdateJobStatus(ctx, jobID, models.JobStatusCompleted, "Completed", 100, "", "", outCfg)
}

func toFloatSafe(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func (s *ProcessingService) FailProcessingJob(ctx context.Context, jobID, errCode, errMsg string) error {
	_ = s.procRepo.AppendJobLog(ctx, jobID, "Failed", "ERROR", errMsg)
	return s.procRepo.UpdateJobStatus(ctx, jobID, models.JobStatusFailed, "Failed", 100, errCode, errMsg, nil)
}

func (s *ProcessingService) CancelProcessingJob(ctx context.Context, jobID, userID string) (*models.ProcessingJob, error) {
	job, err := s.procRepo.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if userID != "" && job.UserID != "" && job.UserID != userID {
		return nil, models.ErrMapAccessDenied
	}
	s.mu.Lock()
	if cancel, ok := s.activeCancels[jobID]; ok {
		cancel()
	}
	s.mu.Unlock()

	_ = s.procRepo.UpdateJobStatus(ctx, jobID, models.JobStatusCancelled, "Cancelled", job.Progress, "JOB_CANCELLED", "Cancelled by user request", nil)
	_ = s.procRepo.AppendJobLog(ctx, jobID, "Cancelled", "WARN", "Processing job cancelled by user")
	return s.procRepo.GetJob(ctx, jobID)
}

func (s *ProcessingService) GetProcessingJob(ctx context.Context, jobID, userID string) (*models.ProcessingJob, error) {
	job, err := s.procRepo.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.MapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, job.MapID, userID, false); err != nil {
			return nil, err
		}
	} else if userID != "" && job.UserID != "" && job.UserID != userID {
		return nil, models.ErrMapAccessDenied
	}
	return job, nil
}

func (s *ProcessingService) ListProcessingJobs(ctx context.Context, userID, mapID string, limit int) ([]*models.ProcessingJob, error) {
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, false); err != nil {
			return nil, err
		}
	}
	return s.procRepo.ListJobs(ctx, userID, mapID, limit)
}

func (s *ProcessingService) GetProcessingLogs(ctx context.Context, jobID, userID string) ([]models.ProcessingJobLog, error) {
	if _, err := s.GetProcessingJob(ctx, jobID, userID); err != nil {
		return nil, err
	}
	return s.procRepo.GetJobLogs(ctx, jobID)
}

func (s *ProcessingService) GetProcessingResult(ctx context.Context, jobID, userID string) (map[string]interface{}, error) {
	job, err := s.GetProcessingJob(ctx, jobID, userID)
	if err != nil {
		return nil, err
	}
	outputs, _ := s.procRepo.GetJobOutputs(ctx, jobID)
	return map[string]interface{}{
		"job":           job,
		"outputs":       outputs,
		"output_config": job.OutputConfig,
	}, nil
}

// Helper: saves generated vector features into a newly registered MapLayer and records ProcessingJobOutput
func (s *ProcessingService) saveVectorOutputLayer(
	ctx context.Context,
	jobID, userID, mapID, outputName, geomType, crs string,
	items []gis.OverlayFeatureItem,
) (*models.MapLayer, error) {
	if outputName == "" {
		outputName = "Processed Vector Layer"
	}
	if crs == "" {
		crs = "EPSG:4326"
	}
	if geomType == "" && len(items) > 0 {
		geomType = items[0].Geometry.Type
	}
	if geomType == "" {
		geomType = "Polygon"
	}

	// Discover schema fields from features' properties
	var fields []models.AddFieldRequest
	seenFields := make(map[string]bool)
	for _, it := range items {
		for k, v := range it.Properties {
			if seenFields[k] || !models.ValidateFieldName(k) {
				continue
			}
			seenFields[k] = true
			ft := "string"
			switch v.(type) {
			case int, int64:
				ft = "integer"
			case float64, float32:
				ft = "float"
			case bool:
				ft = "boolean"
			}
			fields = append(fields, models.AddFieldRequest{
				FieldName:  k,
				FieldType:  ft,
				IsNullable: &[]bool{true}[0],
			})
		}
	}

	vis := true
	op := 0.85
	newLayer, err := s.gisRepo.CreateLayer(ctx, models.CreateLayerRequest{
		MapID:        mapID,
		LayerName:    outputName,
		LayerType:    "vector",
		GeometryType: geomType,
		IsVisible:    &vis,
		Opacity:      &op,
		CRS:          crs,
	})
	if err != nil {
		return nil, err
	}
	for _, fld := range fields {
		_, _ = s.gisRepo.AddLayerField(ctx, newLayer.LayerID, fld)
	}

	srid := gis.ParseSRID(crs)
	for _, item := range items {
		valRes, valErr := gis.ValidateGeoJSONGeometry(item.Geometry, crs)
		wkt := "POINT(0 0)"
		var bbox []float64
		if valErr == nil && valRes != nil {
			wkt = valRes.WKT
			bbox = valRes.BBox
		}
		_, _ = s.gisRepo.CreateFeature(ctx, userID, models.CreateFeatureRequest{
			LayerID:    newLayer.LayerID,
			Geometry:   item.Geometry,
			Properties: item.Properties,
			CRS:        crs,
		}, wkt, srid, bbox)
	}

	_ = s.procRepo.SaveJobOutput(ctx, models.ProcessingJobOutput{
		JobID:        jobID,
		LayerID:      newLayer.LayerID,
		OutputName:   newLayer.LayerName,
		OutputFormat: "geojson",
	})
	return newLayer, nil
}

// Helper: registers a newly generated raster file as a SessionDataset and optional MapLayer
func (s *ProcessingService) saveRasterOutputDatasetAndLayer(
	ctx context.Context,
	jobID, sessionID, mapID, outputName, outPath, crs string,
	bounds []float64, width, height int,
) (*models.Dataset, *models.MapLayer, error) {
	if outputName == "" {
		outputName = "Processed Raster"
	}
	if crs == "" {
		crs = "EPSG:4326"
	}
	if len(bounds) < 4 {
		bounds = []float64{77.18, 28.58, 77.26, 28.65}
	}
	dsID := fmt.Sprintf("ds_proc_%s", jobID[4:])
	now := time.Now().UTC()

	ds := &models.Dataset{
		ID:                 dsID,
		SessionID:          sessionID,
		Filename:           outputName,
		ContentType:        "image/png",
		StoragePath:        outPath,
		GeoreferencedPath:  outPath,
		ProcessedPath:      outPath,
		GeoreferenceStatus: "completed",
		ExtractedMetadata: map[string]interface{}{
			"crs":    crs,
			"width":  width,
			"height": height,
			"bands":  4,
			"bounds": map[string]float64{
				"min_x": bounds[0],
				"min_y": bounds[1],
				"max_x": bounds[2],
				"max_y": bounds[3],
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if s.sessionRepo != nil && sessionID != "" {
		_ = s.sessionRepo.InsertDataset(sessionID, ds)
	}

	var layer *models.MapLayer
	if mapID != "" {
		vis := true
		op := 0.9
		layer, _ = s.gisRepo.CreateLayer(ctx, models.CreateLayerRequest{
			MapID:     mapID,
			DatasetID: ds.ID,
			LayerName: outputName,
			LayerType: "raster",
			IsVisible: &vis,
			Opacity:   &op,
			CRS:       crs,
			Bounds:    bounds,
		})
	}

	layerID := ""
	if layer != nil {
		layerID = layer.LayerID
	}
	_ = s.procRepo.SaveJobOutput(ctx, models.ProcessingJobOutput{
		JobID:        jobID,
		DatasetID:    ds.ID,
		LayerID:      layerID,
		OutputName:   outputName,
		StoragePath:  outPath,
		OutputFormat: "geotiff",
	})
	return ds, layer, nil
}

// ============================================================================
// MODULE B — BUFFER ANALYSIS
// ============================================================================

func (s *ProcessingService) SubmitBufferAnalysis(ctx context.Context, userID, sessionID string, req models.BufferRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	inLayer, err := s.gisService.checkLayerAccess(ctx, req.InputLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = inLayer.MapID
	}
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}

	cfg := map[string]interface{}{
		"input_layer_id": req.InputLayerID,
		"distance":       req.Distance,
		"unit":           req.Unit,
		"segments":       req.Segments,
		"cap_style":      req.CapStyle,
		"join_style":     req.JoinStyle,
		"dissolve":       req.Dissolve,
		"output_name":    req.OutputName,
		"output_crs":     req.OutputCRS,
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "buffer", cfg, []models.ProcessingJobInput{
		{LayerID: req.InputLayerID, InputRole: "input_layer"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		feats, err := s.gisRepo.GetFeatures(wCtx, req.InputLayerID, nil)
		if err != nil {
			return nil, err
		}
		if len(feats) == 0 {
			return nil, fmt.Errorf("input layer '%s' has no features to buffer", inLayer.LayerName)
		}

		distMeters := gis.ConvertDistanceToMeters(req.Distance, req.Unit)
		srcCRS := inLayer.CRS
		if srcCRS == "" {
			srcCRS = "EPSG:4326"
		}
		outCRS := req.OutputCRS
		if outCRS == "" {
			outCRS = srcCRS
		}

		_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Buffer", "INFO",
			fmt.Sprintf("Buffering %d features by %.2f %s (%.2f m) with cap=%s, join=%s", len(feats), req.Distance, req.Unit, distMeters, req.CapStyle, req.JoinStyle))

		var bufferedItems []gis.OverlayFeatureItem
		var bufferedGeoms []models.GeoJSONGeometry
		for _, f := range feats {
			bufGeom, err := gis.GenerateBufferGeometry(f.Geometry, srcCRS, distMeters, req.Segments, req.CapStyle, req.JoinStyle, outCRS)
			if err != nil {
				return nil, err
			}
			props := make(map[string]interface{})
			for k, v := range f.Properties {
				props[k] = v
			}
			props["buffer_distance"] = req.Distance
			props["buffer_unit"] = req.Unit
			bufferedItems = append(bufferedItems, gis.OverlayFeatureItem{Geometry: *bufGeom, Properties: props})
			bufferedGeoms = append(bufferedGeoms, *bufGeom)
		}

		if req.Dissolve && len(bufferedGeoms) > 1 {
			_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Dissolve", "INFO", "Dissolving overlapping buffer geometries")
			merged, err := gis.MergeGeometries(bufferedGeoms)
			if err == nil && merged != nil {
				bufferedItems = []gis.OverlayFeatureItem{
					{
						Geometry: *merged,
						Properties: map[string]interface{}{
							"buffer_distance": req.Distance,
							"buffer_unit":     req.Unit,
							"dissolved_count": len(bufferedGeoms),
						},
					},
				}
			}
		}

		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("%s Buffer (%g %s)", inLayer.LayerName, req.Distance, req.Unit)
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outName, "Polygon", outCRS, bufferedItems)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(bufferedItems),
			"crs":               outCRS,
		}, nil
	})
}

// ============================================================================
// MODULE C — OVERLAY & DISSOLVE ANALYSIS
// ============================================================================

func (s *ProcessingService) SubmitOverlayAnalysis(ctx context.Context, userID, sessionID, operation string, req models.OverlayRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	layerA, err := s.gisService.checkLayerAccess(ctx, req.InputLayerA, userID, false, false)
	if err != nil {
		return nil, err
	}
	layerB, err := s.gisService.checkLayerAccess(ctx, req.InputLayerB, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = layerA.MapID
	}
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}

	op := strings.ToLower(strings.TrimSpace(operation))
	if op == "" {
		op = strings.ToLower(strings.TrimSpace(req.Operation))
	}
	if op == "" {
		op = "intersection"
	}

	cfg := map[string]interface{}{
		"operation":           op,
		"input_layer_a":       req.InputLayerA,
		"input_layer_b":       req.InputLayerB,
		"preserve_attributes": req.PreserveAttributes,
		"output_name":         req.OutputName,
		"output_crs":          req.OutputCRS,
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", op, cfg, []models.ProcessingJobInput{
		{LayerID: req.InputLayerA, InputRole: "layer_a"},
		{LayerID: req.InputLayerB, InputRole: "layer_b"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		featsA, err := s.gisRepo.GetFeatures(wCtx, req.InputLayerA, nil)
		if err != nil {
			return nil, err
		}
		featsB, err := s.gisRepo.GetFeatures(wCtx, req.InputLayerB, nil)
		if err != nil {
			return nil, err
		}

		// Ensure CRS compatibility if layerB differs from layerA
		crsA := layerA.CRS
		if crsA == "" {
			crsA = "EPSG:4326"
		}
		crsB := layerB.CRS
		if crsB == "" {
			crsB = "EPSG:4326"
		}
		if !strings.EqualFold(crsA, crsB) {
			for _, fb := range featsB {
				if reproj, err := gis.TransformGeometryCRS(fb.Geometry, crsB, crsA); err == nil && reproj != nil {
					fb.Geometry = *reproj
				}
			}
		}

		_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Overlay", "INFO",
			fmt.Sprintf("Running %s between %s (%d features) and %s (%d features)", op, layerA.LayerName, len(featsA), layerB.LayerName, len(featsB)))

		items, err := gis.ExecuteOverlayOperation(op, featsA, featsB, req.PreserveAttributes)
		if err != nil {
			return nil, err
		}

		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("%s %s %s", layerA.LayerName, strings.Title(strings.ReplaceAll(op, "_", " ")), layerB.LayerName)
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outName, layerA.GeometryType, req.OutputCRS, items)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(items),
			"operation":         op,
		}, nil
	})
}

func (s *ProcessingService) SubmitDissolveAnalysis(ctx context.Context, userID, sessionID string, req models.DissolveRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	inLayer, err := s.gisService.checkLayerAccess(ctx, req.InputLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = inLayer.MapID
	}
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}

	cfg := map[string]interface{}{
		"input_layer_id":  req.InputLayerID,
		"dissolve_fields": req.DissolveFields,
		"dissolve_all":    req.DissolveAll,
		"output_name":     req.OutputName,
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "dissolve", cfg, []models.ProcessingJobInput{
		{LayerID: req.InputLayerID, InputRole: "input_layer"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		feats, err := s.gisRepo.GetFeatures(wCtx, req.InputLayerID, nil)
		if err != nil {
			return nil, err
		}
		items, err := gis.ExecuteDissolveFeatures(feats, req.DissolveFields, req.DissolveAll)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("%s Dissolved", inLayer.LayerName)
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outName, inLayer.GeometryType, req.OutputCRS, items)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(items),
		}, nil
	})
}

// ============================================================================
// MODULE D — SPATIAL JOIN
// ============================================================================

func (s *ProcessingService) SubmitSpatialJoin(ctx context.Context, userID, sessionID string, req models.SpatialJoinRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	targetLayer, err := s.gisService.checkLayerAccess(ctx, req.TargetLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	sourceLayer, err := s.gisService.checkLayerAccess(ctx, req.SourceLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = targetLayer.MapID
	}
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}

	cfg := map[string]interface{}{
		"target_layer_id":      req.TargetLayerID,
		"source_layer_id":      req.SourceLayerID,
		"spatial_relationship": req.SpatialRelationship,
		"aggregation_type":     req.AggregationType,
		"selected_fields":      req.SelectedFields,
		"output_name":          req.OutputName,
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "spatial_join", cfg, []models.ProcessingJobInput{
		{LayerID: req.TargetLayerID, InputRole: "target_layer"},
		{LayerID: req.SourceLayerID, InputRole: "source_layer"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		targetFeats, err := s.gisRepo.GetFeatures(wCtx, req.TargetLayerID, nil)
		if err != nil {
			return nil, err
		}
		sourceFeats, err := s.gisRepo.GetFeatures(wCtx, req.SourceLayerID, nil)
		if err != nil {
			return nil, err
		}
		_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "SpatialJoin", "INFO",
			fmt.Sprintf("Joining %s (%d features) onto %s (%d features) via %s (%s)", sourceLayer.LayerName, len(sourceFeats), targetLayer.LayerName, len(targetFeats), req.SpatialRelationship, req.AggregationType))

		items, err := gis.ExecuteSpatialJoin(targetFeats, sourceFeats, req.SpatialRelationship, req.AggregationType, req.SelectedFields)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("%s Joined with %s", targetLayer.LayerName, sourceLayer.LayerName)
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outName, targetLayer.GeometryType, req.OutputCRS, items)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(items),
		}, nil
	})
}

// ============================================================================
// MODULE E — PROXIMITY ANALYSIS
// ============================================================================

func (s *ProcessingService) SubmitProximityAnalysis(ctx context.Context, userID, sessionID string, req models.ProximityRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	srcLayer, err := s.gisService.checkLayerAccess(ctx, req.SourceLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	dstLayer, err := s.gisService.checkLayerAccess(ctx, req.TargetLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = srcLayer.MapID
	}
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}
	algName := req.Mode
	if algName == "" {
		algName = "proximity"
	}

	cfg := map[string]interface{}{
		"source_layer_id":           req.SourceLayerID,
		"target_layer_id":           req.TargetLayerID,
		"maximum_distance":          req.MaximumDistance,
		"unit":                      req.Unit,
		"nearest_count":             req.NearestCount,
		"generate_connection_lines": req.GenerateConnectionLines,
		"output_name":               req.OutputName,
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", algName, cfg, []models.ProcessingJobInput{
		{LayerID: req.SourceLayerID, InputRole: "source_layer"},
		{LayerID: req.TargetLayerID, InputRole: "target_layer"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		srcFeats, err := s.gisRepo.GetFeatures(wCtx, req.SourceLayerID, nil)
		if err != nil {
			return nil, err
		}
		dstFeats, err := s.gisRepo.GetFeatures(wCtx, req.TargetLayerID, nil)
		if err != nil {
			return nil, err
		}
		items, err := gis.ExecuteProximityAnalysis(srcFeats, dstFeats, req)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("Proximity %s to %s", srcLayer.LayerName, dstLayer.LayerName)
		}
		geomType := srcLayer.GeometryType
		if req.GenerateConnectionLines || req.Mode == "distance_matrix" || req.Mode == "nearest_facility" {
			geomType = "LineString"
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outName, geomType, req.OutputCRS, items)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(items),
		}, nil
	})
}

// ============================================================================
// MODULE F — HEATMAP GENERATION (KDE)
// ============================================================================

func (s *ProcessingService) SubmitHeatmapGeneration(ctx context.Context, userID, sessionID string, req models.HeatmapRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	inLayer, err := s.gisService.checkLayerAccess(ctx, req.InputLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = inLayer.MapID
	}
	if mapID != "" && userID != "" {
		if _, err := s.gisService.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}

	cfg := map[string]interface{}{
		"input_layer_id": req.InputLayerID,
		"weight_field":   req.WeightField,
		"radius":         req.Radius,
		"radius_unit":    req.RadiusUnit,
		"pixel_size":     req.PixelSize,
		"kernel_type":    req.KernelType,
		"color_ramp":     req.ColorRamp,
		"output_name":    req.OutputName,
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "heatmap", cfg, []models.ProcessingJobInput{
		{LayerID: req.InputLayerID, InputRole: "input_layer"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		feats, err := s.gisRepo.GetFeatures(wCtx, req.InputLayerID, nil)
		if err != nil {
			return nil, err
		}
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "heatmap_kde.png")
		bounds, w, h, err := gis.GenerateKDEHeatmapFile(feats, req, outPath)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("%s KDE Heatmap", inLayer.LayerName)
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, mapID, outName, outPath, req.OutputCRS, bounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"output_name":       outName,
			"bounds":            bounds,
			"width":             w,
			"height":            h,
		}, nil
	})
}

// ============================================================================
// MODULE G — RASTER PROCESSING TOOLBOX
// ============================================================================

func (s *ProcessingService) resolveRasterPathAndBounds(datasetID string) (string, []float64, string) {
	if s.sessionRepo != nil {
		if ds, _, err := s.sessionRepo.FindDataset(datasetID); err == nil && ds != nil {
			p := ds.ProcessedPath
			if p == "" {
				p = ds.StoragePath
			}
			bounds := []float64{77.18, 28.58, 77.26, 28.65}
			crs := "EPSG:4326"
			if ds.ExtractedMetadata != nil {
				if cStr, ok := ds.ExtractedMetadata["crs"].(string); ok && cStr != "" {
					crs = cStr
				}
				if bMap, ok := ds.ExtractedMetadata["bounds"].(map[string]float64); ok {
					bounds = []float64{bMap["min_x"], bMap["min_y"], bMap["max_x"], bMap["max_y"]}
				}
			}
			return p, bounds, crs
		}
	}
	return "", []float64{77.18, 28.58, 77.26, 28.65}, "EPSG:4326"
}

func (s *ProcessingService) SubmitRasterClip(ctx context.Context, userID, sessionID string, req models.RasterClipRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	clipLayer, err := s.gisService.checkLayerAccess(ctx, req.ClippingLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = clipLayer.MapID
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "raster_clip", map[string]interface{}{
		"raster_dataset_id": req.RasterDatasetID,
		"clipping_layer_id": req.ClippingLayerID,
		"output_name":       req.OutputName,
	}, []models.ProcessingJobInput{
		{DatasetID: req.RasterDatasetID, InputRole: "raster"},
		{LayerID: req.ClippingLayerID, InputRole: "clipping_mask"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		clipFeats, _ := s.gisRepo.GetFeatures(wCtx, req.ClippingLayerID, nil)
		srcPath, srcBounds, srcCRS := s.resolveRasterPathAndBounds(req.RasterDatasetID)
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "raster_clipped.png")
		bounds, w, h, err := gis.ClipRasterWithPolygons(srcPath, srcBounds, clipFeats, outPath)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = "Clipped Raster"
		}
		outCRS := req.OutputCRS
		if outCRS == "" {
			outCRS = srcCRS
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, mapID, outName, outPath, outCRS, bounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"output_name":       outName,
			"bounds":            bounds,
			"width":             w,
			"height":            h,
		}, nil
	})
}

func (s *ProcessingService) SubmitRasterReproject(ctx context.Context, userID, sessionID string, req models.RasterReprojectRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	job, err := s.CreateProcessingJob(ctx, userID, req.MapID, "", "raster_reproject", map[string]interface{}{
		"raster_dataset_id": req.RasterDatasetID,
		"target_crs":        req.TargetCRS,
		"output_name":       req.OutputName,
	}, []models.ProcessingJobInput{{DatasetID: req.RasterDatasetID, InputRole: "raster"}})
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		srcPath, srcBounds, srcCRS := s.resolveRasterPathAndBounds(req.RasterDatasetID)
		if req.SourceCRS != "" {
			srcCRS = req.SourceCRS
		}
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "raster_reprojected.png")
		w, h, err := gis.ResampleRasterImage(srcPath, 1.0, 0, 0, outPath)
		if err != nil {
			return nil, err
		}
		x1, y1, _ := gis.TransformCoordPair(srcBounds[0], srcBounds[1], srcCRS, req.TargetCRS)
		x2, y2, _ := gis.TransformCoordPair(srcBounds[2], srcBounds[3], srcCRS, req.TargetCRS)
		newBounds := []float64{x1, y1, x2, y2}
		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("Raster (%s)", req.TargetCRS)
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, req.MapID, outName, outPath, req.TargetCRS, newBounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"crs":               req.TargetCRS,
			"bounds":            newBounds,
		}, nil
	})
}

func (s *ProcessingService) SubmitRasterResample(ctx context.Context, userID, sessionID string, req models.RasterResampleRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	job, err := s.CreateProcessingJob(ctx, userID, req.MapID, "", "raster_resample", map[string]interface{}{
		"raster_dataset_id": req.RasterDatasetID,
		"scale_factor":      req.ScaleFactor,
		"method":            req.Method,
	}, []models.ProcessingJobInput{{DatasetID: req.RasterDatasetID, InputRole: "raster"}})
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		srcPath, srcBounds, srcCRS := s.resolveRasterPathAndBounds(req.RasterDatasetID)
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "raster_resampled.png")
		w, h, err := gis.ResampleRasterImage(srcPath, req.ScaleFactor, req.TargetWidth, req.TargetHeight, outPath)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = "Resampled Raster"
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, req.MapID, outName, outPath, srcCRS, srcBounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"width":             w,
			"height":            h,
		}, nil
	})
}

func (s *ProcessingService) SubmitRasterCalculator(ctx context.Context, userID, sessionID string, req models.RasterCalculatorRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var inputs []models.ProcessingJobInput
	for _, id := range req.InputRasters {
		inputs = append(inputs, models.ProcessingJobInput{DatasetID: id, InputRole: "band_input"})
	}
	job, err := s.CreateProcessingJob(ctx, userID, req.MapID, "", "raster_calculator", map[string]interface{}{
		"input_rasters": req.InputRasters,
		"expression":    req.Expression,
		"output_name":   req.OutputName,
	}, inputs)
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		var paths []string
		var bounds []float64
		crs := "EPSG:4326"
		for _, id := range req.InputRasters {
			p, b, c := s.resolveRasterPathAndBounds(id)
			paths = append(paths, p)
			if len(bounds) < 4 {
				bounds = b
				crs = c
			}
		}
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "raster_calculated.png")
		w, h, stats, err := gis.ExecuteRasterBandCalculator(paths, req.Expression, outPath)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("Raster Calc (%s)", req.Expression)
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, req.MapID, outName, outPath, crs, bounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"statistics":        stats,
			"width":             w,
			"height":            h,
		}, nil
	})
}

func (s *ProcessingService) SubmitRasterAlign(ctx context.Context, userID, sessionID string, req models.RasterAlignRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	targetID := req.RasterDatasetID
	if targetID == "" && len(req.RasterIDs) > 0 {
		targetID = req.RasterIDs[0]
	}
	job, err := s.CreateProcessingJob(ctx, userID, req.MapID, "", "raster_align", map[string]interface{}{
		"raster_dataset_id":    targetID,
		"reference_dataset_id": req.ReferenceID,
		"output_name":          req.OutputName,
	}, []models.ProcessingJobInput{{DatasetID: targetID, InputRole: "raster"}})
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		srcPath, srcBounds, srcCRS := s.resolveRasterPathAndBounds(targetID)
		if req.ReferenceID != "" {
			_, refBounds, refCRS := s.resolveRasterPathAndBounds(req.ReferenceID)
			srcBounds = refBounds
			srcCRS = refCRS
		}
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "raster_aligned.png")
		w, h, err := gis.ResampleRasterImage(srcPath, 1.0, 128, 128, outPath)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = "Aligned Raster"
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, req.MapID, outName, outPath, srcCRS, srcBounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"bounds":            srcBounds,
			"crs":               srcCRS,
		}, nil
	})
}

func (s *ProcessingService) SubmitRasterMosaic(ctx context.Context, userID, sessionID string, req models.RasterMosaicRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var inputs []models.ProcessingJobInput
	for _, id := range req.RasterDatasetIDs {
		inputs = append(inputs, models.ProcessingJobInput{DatasetID: id, InputRole: "mosaic_tile"})
	}
	job, err := s.CreateProcessingJob(ctx, userID, req.MapID, "", "raster_mosaic", map[string]interface{}{
		"raster_dataset_ids": req.RasterDatasetIDs,
		"output_name":        req.OutputName,
	}, inputs)
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		var paths []string
		unionBounds := []float64{180, 90, -180, -90}
		crs := req.TargetCRS
		for _, id := range req.RasterDatasetIDs {
			p, b, c := s.resolveRasterPathAndBounds(id)
			paths = append(paths, p)
			if crs == "" {
				crs = c
			}
			if len(b) == 4 {
				if b[0] < unionBounds[0] {
					unionBounds[0] = b[0]
				}
				if b[1] < unionBounds[1] {
					unionBounds[1] = b[1]
				}
				if b[2] > unionBounds[2] {
					unionBounds[2] = b[2]
				}
				if b[3] > unionBounds[3] {
					unionBounds[3] = b[3]
				}
			}
		}
		if unionBounds[0] > unionBounds[2] {
			unionBounds = []float64{77.18, 28.58, 77.28, 28.68}
		}
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "raster_mosaic.png")
		w, h, _, err := gis.ExecuteRasterBandCalculator(paths, "(A + B) / 2", outPath)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = "Raster Mosaic"
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, req.MapID, outName, outPath, crs, unionBounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"bounds":            unionBounds,
			"width":             w,
			"height":            h,
		}, nil
	})
}

func (s *ProcessingService) SubmitRasterContours(ctx context.Context, userID, sessionID string, req models.RasterContoursRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	job, err := s.CreateProcessingJob(ctx, userID, req.MapID, "", "raster_contours", map[string]interface{}{
		"raster_dataset_id": req.RasterDatasetID,
		"interval":          req.Interval,
		"output_name":       req.OutputName,
	}, []models.ProcessingJobInput{{DatasetID: req.RasterDatasetID, InputRole: "raster"}})
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		srcPath, srcBounds, srcCRS := s.resolveRasterPathAndBounds(req.RasterDatasetID)
		contours, err := gis.GenerateContourLines(srcPath, srcBounds, req.Interval)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = fmt.Sprintf("Contours (%gm)", req.Interval)
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, req.MapID, outName, "LineString", srcCRS, contours)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(contours),
		}, nil
	})
}

func (s *ProcessingService) SubmitRasterHillshade(ctx context.Context, userID, sessionID string, req models.RasterHillshadeRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	job, err := s.CreateProcessingJob(ctx, userID, req.MapID, "", "raster_hillshade", map[string]interface{}{
		"raster_dataset_id": req.RasterDatasetID,
		"azimuth":           req.Azimuth,
		"altitude":          req.Altitude,
		"z_factor":          req.ZFactor,
		"output_name":       req.OutputName,
	}, []models.ProcessingJobInput{{DatasetID: req.RasterDatasetID, InputRole: "raster"}})
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		srcPath, srcBounds, srcCRS := s.resolveRasterPathAndBounds(req.RasterDatasetID)
		outPath := filepath.Join(s.tempDir, "jobs", j.JobID, "raster_hillshade.png")
		w, h, err := gis.GenerateHillshadeRaster(srcPath, req.Azimuth, req.Altitude, req.ZFactor, outPath)
		if err != nil {
			return nil, err
		}
		outName := req.OutputName
		if outName == "" {
			outName = "Terrain Hillshade"
		}
		ds, outLayer, err := s.saveRasterOutputDatasetAndLayer(wCtx, j.JobID, sessionID, req.MapID, outName, outPath, srcCRS, srcBounds, w, h)
		if err != nil {
			return nil, err
		}
		layerID := ""
		if outLayer != nil {
			layerID = outLayer.LayerID
		}
		return map[string]interface{}{
			"output_dataset_id": ds.ID,
			"output_layer_id":   layerID,
			"width":             w,
			"height":            h,
		}, nil
	})
}

// ============================================================================
// MODULE H — TOPOLOGY VALIDATION & AUTOMATED FIXES
// ============================================================================

func (s *ProcessingService) SubmitTopologyValidation(ctx context.Context, userID, sessionID string, req models.TopologyValidationRequest) (*models.ProcessingJob, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	inLayer, err := s.gisService.checkLayerAccess(ctx, req.LayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	mapID := req.MapID
	if mapID == "" {
		mapID = inLayer.MapID
	}
	cfg := map[string]interface{}{
		"layer_id":                 req.LayerID,
		"check_overlaps":           req.CheckOverlaps,
		"check_gaps":               req.CheckGaps,
		"check_self_intersections": req.CheckSelfIntersections,
		"check_duplicates":         req.CheckDuplicates,
		"check_disconnected_lines": req.CheckDisconnectedLines,
		"output_error_layer":       req.OutputErrorLayer,
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "topology", cfg, []models.ProcessingJobInput{
		{LayerID: req.LayerID, InputRole: "target_layer"},
	})
	if err != nil {
		return nil, err
	}

	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		feats, err := s.gisRepo.GetFeatures(wCtx, req.LayerID, nil)
		if err != nil {
			return nil, err
		}
		topErrors := gis.ValidateLayerTopology(j.JobID, req.LayerID, feats, req)
		_ = s.procRepo.SaveTopologyErrors(wCtx, j.JobID, topErrors)

		_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Topology", "INFO",
			fmt.Sprintf("Detected %d topological anomalies across %d features", len(topErrors), len(feats)))

		errorLayerID := ""
		if req.OutputErrorLayer && len(topErrors) > 0 {
			var errItems []gis.OverlayFeatureItem
			for _, te := range topErrors {
				if te.Geometry != nil {
					errItems = append(errItems, gis.OverlayFeatureItem{
						Geometry: *te.Geometry,
						Properties: map[string]interface{}{
							"error_id":   te.ErrorID,
							"error_type": te.ErrorType,
							"message":    te.ErrorMessage,
							"suggestion": te.Suggestion,
							"feature_id": te.FeatureID,
						},
					})
				}
			}
			outName := req.OutputName
			if outName == "" {
				outName = fmt.Sprintf("%s Topology Errors", inLayer.LayerName)
			}
			if errLayer, lErr := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outName, "Polygon", inLayer.CRS, errItems); lErr == nil && errLayer != nil {
				errorLayerID = errLayer.LayerID
			}
		}

		return map[string]interface{}{
			"error_count":    len(topErrors),
			"error_layer_id": errorLayerID,
			"layer_id":       req.LayerID,
			"valid":          len(topErrors) == 0,
		}, nil
	})
}

func (s *ProcessingService) GetTopologyErrors(ctx context.Context, jobID, userID string) ([]*models.TopologyError, error) {
	if _, err := s.GetProcessingJob(ctx, jobID, userID); err != nil {
		return nil, err
	}
	return s.procRepo.GetTopologyErrors(ctx, jobID)
}

func (s *ProcessingService) FixTopologyErrors(ctx context.Context, jobID, userID string, req models.TopologyFixRequest) (map[string]interface{}, error) {
	if _, err := s.GetProcessingJob(ctx, jobID, userID); err != nil {
		return nil, err
	}
	errs, err := s.procRepo.GetTopologyErrors(ctx, jobID)
	if err != nil {
		return nil, err
	}
	targetIDs := make(map[string]bool)
	for _, id := range req.ErrorIDs {
		targetIDs[id] = true
	}

	resolvedCount := 0
	for _, te := range errs {
		if te.IsResolved {
			continue
		}
		if !req.FixAll && len(targetIDs) > 0 && !targetIDs[te.ErrorID] {
			continue
		}
		switch te.ErrorType {
		case "duplicate_geometry":
			if te.FeatureID != "" {
				_ = s.gisRepo.DeleteFeature(ctx, te.FeatureID, userID)
			}
		case "self_intersection", "invalid_geometry":
			if te.FeatureID != "" {
				if feat, fErr := s.gisRepo.GetFeature(ctx, te.FeatureID); fErr == nil && feat != nil {
					repaired := gis.RepairInvalidGeometry(feat.Geometry)
					if valRes, vErr := gis.ValidateGeoJSONGeometry(repaired, "EPSG:4326"); vErr == nil && valRes != nil {
						_, _ = s.gisRepo.UpdateFeature(ctx, te.FeatureID, userID, &repaired, valRes.WKT, 4326, valRes.BBox, feat.Properties)
					}
				}
			}
		}
		_ = s.procRepo.MarkTopologyErrorResolved(ctx, te.ErrorID)
		resolvedCount++
	}
	return map[string]interface{}{
		"job_id":         jobID,
		"resolved_count": resolvedCount,
	}, nil
}

func (s *ProcessingService) SubmitCentroidGeneration(ctx context.Context, userID, sessionID, mapID, inputLayerID, outputName string) (*models.ProcessingJob, error) {
	inLayer, err := s.gisService.checkLayerAccess(ctx, inputLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	if mapID == "" {
		mapID = inLayer.MapID
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "centroid", map[string]interface{}{
		"input_layer_id": inputLayerID,
		"output_name":    outputName,
	}, []models.ProcessingJobInput{{LayerID: inputLayerID, InputRole: "input_layer"}})
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		feats, err := s.gisRepo.GetFeatures(wCtx, inputLayerID, nil)
		if err != nil {
			return nil, err
		}
		items := gis.GenerateCentroidFeatures(feats)
		for idx, item := range items {
			if idx >= 25 {
				_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Geometry", "INFO", fmt.Sprintf("... and %d more centroid features", len(items)-25))
				break
			}
			cx := toFloatSafe(item.Properties["centroid_x"])
			cy := toFloatSafe(item.Properties["centroid_y"])
			label := featureDisplayName(feats, idx, item.Properties)
			_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Geometry", "INFO", fmt.Sprintf("Feature %d (%s): Centroid [%.6f, %.6f]", idx+1, label, cx, cy))
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outputName, "Point", inLayer.CRS, items)
		if err != nil {
			return nil, err
		}
		_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Geometry", "INFO", fmt.Sprintf("Generated %d centroids -> Output Layer %q", len(items), outLayer.LayerName))
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(items),
		}, nil
	})
}

func (s *ProcessingService) SubmitCalculateGeometry(ctx context.Context, userID, sessionID, mapID, inputLayerID, outputName string) (*models.ProcessingJob, error) {
	inLayer, err := s.gisService.checkLayerAccess(ctx, inputLayerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	if mapID == "" {
		mapID = inLayer.MapID
	}
	job, err := s.CreateProcessingJob(ctx, userID, mapID, "", "calculate_geometry", map[string]interface{}{
		"input_layer_id": inputLayerID,
		"output_name":    outputName,
	}, []models.ProcessingJobInput{{LayerID: inputLayerID, InputRole: "input_layer"}})
	if err != nil {
		return nil, err
	}
	return s.QueueProcessingJob(ctx, job, userID, sessionID, true, func(wCtx context.Context, j *models.ProcessingJob) (map[string]interface{}, error) {
		feats, err := s.gisRepo.GetFeatures(wCtx, inputLayerID, nil)
		if err != nil {
			return nil, err
		}
		items := gis.CalculateGeometryMetrics(feats, inLayer.CRS)
		totalArea := 0.0
		totalLength := 0.0
		for idx, item := range items {
			area := toFloatSafe(item.Properties["geom_area"])
			length := toFloatSafe(item.Properties["geom_length"])
			cx := toFloatSafe(item.Properties["centroid_x"])
			cy := toFloatSafe(item.Properties["centroid_y"])
			totalArea += area
			totalLength += length
			if idx < 25 {
				label := featureDisplayName(feats, idx, item.Properties)
				_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Geometry", "INFO", fmt.Sprintf("Feature %d (%s): Area=%.6g, Length/Perimeter=%.6g, Centroid=[%.6f, %.6f]", idx+1, label, area, length, cx, cy))
			} else if idx == 25 {
				_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Geometry", "INFO", fmt.Sprintf("... and %d more features", len(items)-25))
			}
		}
		outLayer, err := s.saveVectorOutputLayer(wCtx, j.JobID, userID, mapID, outputName, inLayer.GeometryType, inLayer.CRS, items)
		if err != nil {
			return nil, err
		}
		_ = s.procRepo.AppendJobLog(wCtx, j.JobID, "Geometry", "INFO", fmt.Sprintf("Summary (%d features): Total Area=%.6g, Total Length/Perimeter=%.6g -> Output Layer %q", len(items), totalArea, totalLength, outLayer.LayerName))
		return map[string]interface{}{
			"output_layer_id":   outLayer.LayerID,
			"output_layer_name": outLayer.LayerName,
			"feature_count":     len(items),
			"total_area":        totalArea,
			"total_length":      totalLength,
		}, nil
	})
}

func featureDisplayName(feats []*models.VectorFeature, idx int, props map[string]interface{}) string {
	for _, key := range []string{"name", "Name", "NAME", "ward_name", "title", "id"} {
		if v, ok := props[key]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	if idx >= 0 && idx < len(feats) && feats[idx] != nil && feats[idx].FeatureID != "" {
		return feats[idx].FeatureID
	}
	return fmt.Sprintf("#%d", idx+1)
}

// ============================================================================
// GENERIC TOOLBOX EXECUTION DISPATCHER (MODULE A)
// ============================================================================

func (s *ProcessingService) ExecuteGenericJob(ctx context.Context, userID, sessionID string, req models.ProcessingJobRequest) (*models.ProcessingJob, error) {
	if _, err := s.ValidateProcessingRequest(req); err != nil {
		return nil, err
	}
	alg := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(req.AlgorithmName)), "-", "_")
	rawParams, _ := json.Marshal(req.Parameters)

	switch alg {
	case "buffer":
		var bReq models.BufferRequest
		_ = json.Unmarshal(rawParams, &bReq)
		bReq.MapID = firstNonEmpty(bReq.MapID, req.MapID)
		bReq.OutputName = firstNonEmpty(bReq.OutputName, req.OutputName)
		if bReq.InputLayerID == "" && len(req.InputLayerIDs) > 0 {
			bReq.InputLayerID = req.InputLayerIDs[0]
		}
		return s.SubmitBufferAnalysis(ctx, userID, sessionID, bReq)

	case "centroid":
		layerID, _ := req.Parameters["input_layer_id"].(string)
		if layerID == "" && len(req.InputLayerIDs) > 0 {
			layerID = req.InputLayerIDs[0]
		}
		outName := firstNonEmpty(req.OutputName, fmt.Sprintf("%v", req.Parameters["output_name"]), "Centroids")
		return s.SubmitCentroidGeneration(ctx, userID, sessionID, req.MapID, layerID, outName)

	case "calculate_geometry":
		layerID, _ := req.Parameters["input_layer_id"].(string)
		if layerID == "" && len(req.InputLayerIDs) > 0 {
			layerID = req.InputLayerIDs[0]
		}
		outName := firstNonEmpty(req.OutputName, fmt.Sprintf("%v", req.Parameters["output_name"]), "Calculated Geometry")
		return s.SubmitCalculateGeometry(ctx, userID, sessionID, req.MapID, layerID, outName)

	case "raster_statistics":
		dsID, _ := req.Parameters["raster_dataset_id"].(string)
		if dsID == "" && len(req.InputDatasetIDs) > 0 {
			dsID = req.InputDatasetIDs[0]
		}
		return s.SubmitRasterCalculator(ctx, userID, sessionID, models.RasterCalculatorRequest{
			MapID:        req.MapID,
			InputRasters: []string{dsID},
			Expression:   "A * 1",
			OutputName:   firstNonEmpty(req.OutputName, "Raster Statistics"),
		})

	case "intersection", "union", "difference", "clip", "symmetrical_difference":
		var oReq models.OverlayRequest
		_ = json.Unmarshal(rawParams, &oReq)
		oReq.MapID = firstNonEmpty(oReq.MapID, req.MapID)
		oReq.OutputName = firstNonEmpty(oReq.OutputName, req.OutputName)
		if oReq.InputLayerA == "" && len(req.InputLayerIDs) > 0 {
			oReq.InputLayerA = req.InputLayerIDs[0]
		}
		if oReq.InputLayerB == "" && len(req.InputLayerIDs) > 1 {
			oReq.InputLayerB = req.InputLayerIDs[1]
		}
		return s.SubmitOverlayAnalysis(ctx, userID, sessionID, alg, oReq)

	case "dissolve":
		var dReq models.DissolveRequest
		_ = json.Unmarshal(rawParams, &dReq)
		dReq.MapID = firstNonEmpty(dReq.MapID, req.MapID)
		dReq.OutputName = firstNonEmpty(dReq.OutputName, req.OutputName)
		if dReq.InputLayerID == "" && len(req.InputLayerIDs) > 0 {
			dReq.InputLayerID = req.InputLayerIDs[0]
		}
		return s.SubmitDissolveAnalysis(ctx, userID, sessionID, dReq)

	case "spatial_join":
		var sjReq models.SpatialJoinRequest
		_ = json.Unmarshal(rawParams, &sjReq)
		sjReq.MapID = firstNonEmpty(sjReq.MapID, req.MapID)
		sjReq.OutputName = firstNonEmpty(sjReq.OutputName, req.OutputName)
		return s.SubmitSpatialJoin(ctx, userID, sessionID, sjReq)

	case "proximity", "distance_matrix", "nearest_facility":
		var pReq models.ProximityRequest
		_ = json.Unmarshal(rawParams, &pReq)
		pReq.MapID = firstNonEmpty(pReq.MapID, req.MapID)
		pReq.Mode = alg
		pReq.OutputName = firstNonEmpty(pReq.OutputName, req.OutputName)
		return s.SubmitProximityAnalysis(ctx, userID, sessionID, pReq)

	case "heatmap":
		var hReq models.HeatmapRequest
		_ = json.Unmarshal(rawParams, &hReq)
		hReq.MapID = firstNonEmpty(hReq.MapID, req.MapID)
		hReq.OutputName = firstNonEmpty(hReq.OutputName, req.OutputName)
		return s.SubmitHeatmapGeneration(ctx, userID, sessionID, hReq)

	case "raster_clip":
		var rcReq models.RasterClipRequest
		_ = json.Unmarshal(rawParams, &rcReq)
		rcReq.MapID = firstNonEmpty(rcReq.MapID, req.MapID)
		rcReq.OutputName = firstNonEmpty(rcReq.OutputName, req.OutputName)
		return s.SubmitRasterClip(ctx, userID, sessionID, rcReq)

	case "raster_reproject":
		var rrReq models.RasterReprojectRequest
		_ = json.Unmarshal(rawParams, &rrReq)
		rrReq.MapID = firstNonEmpty(rrReq.MapID, req.MapID)
		rrReq.OutputName = firstNonEmpty(rrReq.OutputName, req.OutputName)
		return s.SubmitRasterReproject(ctx, userID, sessionID, rrReq)

	case "raster_resample":
		var rsReq models.RasterResampleRequest
		_ = json.Unmarshal(rawParams, &rsReq)
		rsReq.MapID = firstNonEmpty(rsReq.MapID, req.MapID)
		rsReq.OutputName = firstNonEmpty(rsReq.OutputName, req.OutputName)
		return s.SubmitRasterResample(ctx, userID, sessionID, rsReq)

	case "raster_calculator":
		var calcReq models.RasterCalculatorRequest
		_ = json.Unmarshal(rawParams, &calcReq)
		calcReq.MapID = firstNonEmpty(calcReq.MapID, req.MapID)
		calcReq.OutputName = firstNonEmpty(calcReq.OutputName, req.OutputName)
		if len(calcReq.InputRasters) == 0 && len(req.InputDatasetIDs) > 0 {
			calcReq.InputRasters = req.InputDatasetIDs
		}
		return s.SubmitRasterCalculator(ctx, userID, sessionID, calcReq)

	case "raster_align":
		var alReq models.RasterAlignRequest
		_ = json.Unmarshal(rawParams, &alReq)
		alReq.MapID = firstNonEmpty(alReq.MapID, req.MapID)
		alReq.OutputName = firstNonEmpty(alReq.OutputName, req.OutputName)
		return s.SubmitRasterAlign(ctx, userID, sessionID, alReq)

	case "raster_mosaic":
		var mReq models.RasterMosaicRequest
		_ = json.Unmarshal(rawParams, &mReq)
		mReq.MapID = firstNonEmpty(mReq.MapID, req.MapID)
		mReq.OutputName = firstNonEmpty(mReq.OutputName, req.OutputName)
		if len(mReq.RasterDatasetIDs) == 0 && len(req.InputDatasetIDs) > 0 {
			mReq.RasterDatasetIDs = req.InputDatasetIDs
		}
		return s.SubmitRasterMosaic(ctx, userID, sessionID, mReq)

	case "raster_contours":
		var cReq models.RasterContoursRequest
		_ = json.Unmarshal(rawParams, &cReq)
		cReq.MapID = firstNonEmpty(cReq.MapID, req.MapID)
		cReq.OutputName = firstNonEmpty(cReq.OutputName, req.OutputName)
		return s.SubmitRasterContours(ctx, userID, sessionID, cReq)

	case "raster_hillshade":
		var hsReq models.RasterHillshadeRequest
		_ = json.Unmarshal(rawParams, &hsReq)
		hsReq.MapID = firstNonEmpty(hsReq.MapID, req.MapID)
		hsReq.OutputName = firstNonEmpty(hsReq.OutputName, req.OutputName)
		return s.SubmitRasterHillshade(ctx, userID, sessionID, hsReq)

	case "topology":
		var tReq models.TopologyValidationRequest
		_ = json.Unmarshal(rawParams, &tReq)
		tReq.MapID = firstNonEmpty(tReq.MapID, req.MapID)
		tReq.OutputName = firstNonEmpty(tReq.OutputName, req.OutputName)
		if tReq.LayerID == "" && len(req.InputLayerIDs) > 0 {
			tReq.LayerID = req.InputLayerIDs[0]
		}
		return s.SubmitTopologyValidation(ctx, userID, sessionID, tReq)

	default:
		return nil, models.NewValidationError("UNSUPPORTED_ALGORITHM", fmt.Sprintf("algorithm '%s' is not supported", req.AlgorithmName), "algorithm_name")
	}
}

// Presets
func (s *ProcessingService) SavePreset(ctx context.Context, userID string, preset *models.ProcessingPreset) (*models.ProcessingPreset, error) {
	if strings.TrimSpace(preset.AlgorithmName) == "" || strings.TrimSpace(preset.PresetName) == "" {
		return nil, models.NewValidationError("INVALID_PRESET", "algorithm_name and preset_name are required", "preset_name")
	}
	preset.UserID = userID
	if err := s.procRepo.SavePreset(ctx, preset); err != nil {
		return nil, err
	}
	return preset, nil
}

func (s *ProcessingService) ListPresets(ctx context.Context, userID, algName string) ([]*models.ProcessingPreset, error) {
	return s.procRepo.ListPresets(ctx, userID, algName)
}

func (s *ProcessingService) DeletePreset(ctx context.Context, presetID, userID string) error {
	return s.procRepo.DeletePreset(ctx, presetID, userID)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
