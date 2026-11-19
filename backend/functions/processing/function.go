package processing

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	svc *services.ProcessingService
}

func NewHandler(svc *services.ProcessingService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetService() *services.ProcessingService {
	return h.svc
}

func userIDFromUser(u *models.User) string {
	if u == nil {
		return ""
	}
	return u.ID
}

func writeProcessingSuccess(w http.ResponseWriter, status int, msg string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": msg,
		"data":    data,
	})
}

func writeProcessingError(w http.ResponseWriter, err error) {
	var valErr *models.ValidationError
	if errors.As(err, &valErr) {
		var fPtr *string
		if valErr.Field != "" {
			fPtr = &valErr.Field
		}
		models.WriteError(w, http.StatusBadRequest, valErr.Code, valErr.Message, nil, fPtr)
		return
	}
	switch {
	case errors.Is(err, models.ErrMapAccessDenied):
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil, nil)
	case errors.Is(err, models.ErrNotFound),
		errors.Is(err, models.ErrLayerNotFound),
		errors.Is(err, models.ErrMapNotFound):
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil, nil)
	default:
		models.WriteError(w, http.StatusBadRequest, "PROCESSING_ERROR", err.Error(), nil, nil)
	}
}

// ============================================================================
// MODULE A — TOOLBOX & ALGORITHM DISCOVERY
// ============================================================================

// GetAlgorithms handles GET /processing/algorithms
func (h *Handler) GetAlgorithms(w http.ResponseWriter, r *http.Request) {
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	all := h.svc.GetProcessingAlgorithms()

	var filtered []models.ProcessingAlgorithm
	for _, alg := range all {
		if category != "" && !strings.EqualFold(alg.Category, category) {
			continue
		}
		if search != "" &&
			!strings.Contains(strings.ToLower(alg.Name), search) &&
			!strings.Contains(strings.ToLower(alg.Description), search) &&
			!strings.Contains(strings.ToLower(alg.AlgorithmID), search) {
			continue
		}
		filtered = append(filtered, alg)
	}
	if filtered == nil {
		filtered = []models.ProcessingAlgorithm{}
	}
	writeProcessingSuccess(w, http.StatusOK, "Processing algorithms retrieved", map[string]interface{}{
		"algorithms": filtered,
		"total":      len(filtered),
	})
}

// GetAlgorithmDetails handles GET /processing/algorithms/:algorithmId
func (h *Handler) GetAlgorithmDetails(w http.ResponseWriter, r *http.Request, algorithmID string) {
	alg, err := h.svc.GetAlgorithmDetails(algorithmID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "ALGORITHM_NOT_FOUND", err.Error(), nil, nil)
		return
	}
	writeProcessingSuccess(w, http.StatusOK, "Algorithm details retrieved", alg)
}

// ValidateProcessingRequest handles POST /processing/validate
func (h *Handler) ValidateProcessingRequest(w http.ResponseWriter, r *http.Request) {
	var req models.ProcessingJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	alg, err := h.svc.ValidateProcessingRequest(req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusOK, "Processing request parameters are valid", map[string]interface{}{
		"valid":     true,
		"algorithm": alg,
	})
}

// ExecuteProcessingJob handles POST /processing/execute
func (h *Handler) ExecuteProcessingJob(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.ProcessingJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.ExecuteGenericJob(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Processing job accepted", job)
}

// ============================================================================
// MODULE B — BUFFER ANALYSIS
// ============================================================================

// BufferAnalysis handles POST /analysis/buffer
func (h *Handler) BufferAnalysis(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.BufferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitBufferAnalysis(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Buffer analysis submitted", job)
}

// GetBufferJob handles GET /analysis/buffer/:jobId
func (h *Handler) GetBufferJob(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	h.GetJob(w, r, user, jobID)
}

// ============================================================================
// MODULE C — OVERLAY & DISSOLVE ANALYSIS
// ============================================================================

// OverlayAnalysis handles POST /analysis/intersection, /union, /difference, /clip, /symmetrical-difference
func (h *Handler) OverlayAnalysis(w http.ResponseWriter, r *http.Request, user *models.User, sessionID, operation string) {
	var req models.OverlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitOverlayAnalysis(r.Context(), userIDFromUser(user), sessionID, operation, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Overlay analysis submitted", job)
}

// DissolveAnalysis handles POST /analysis/dissolve
func (h *Handler) DissolveAnalysis(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.DissolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitDissolveAnalysis(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Dissolve analysis submitted", job)
}

// ============================================================================
// MODULE D — SPATIAL JOIN
// ============================================================================

// SpatialJoin handles POST /analysis/spatial-join
func (h *Handler) SpatialJoin(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.SpatialJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitSpatialJoin(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Spatial join submitted", job)
}

// ============================================================================
// MODULE E — PROXIMITY ANALYSIS
// ============================================================================

// ProximityAnalysis handles POST /analysis/proximity, /distance-matrix, /nearest-facility
func (h *Handler) ProximityAnalysis(w http.ResponseWriter, r *http.Request, user *models.User, sessionID, mode string) {
	var req models.ProximityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	req.Mode = mode
	job, err := h.svc.SubmitProximityAnalysis(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Proximity analysis submitted", job)
}

// ============================================================================
// MODULE F — HEATMAP GENERATION
// ============================================================================

// HeatmapGeneration handles POST /analysis/heatmap
func (h *Handler) HeatmapGeneration(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.HeatmapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitHeatmapGeneration(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Heatmap KDE job submitted", job)
}

// ============================================================================
// MODULE G — RASTER PROCESSING TOOLBOX
// ============================================================================

// RasterClip handles POST /raster/clip
func (h *Handler) RasterClip(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterClipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterClip(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Raster clip job submitted", job)
}

// RasterReproject handles POST /raster/reproject
func (h *Handler) RasterReproject(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterReprojectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterReproject(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Raster reproject job submitted", job)
}

// RasterResample handles POST /raster/resample
func (h *Handler) RasterResample(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterResampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterResample(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Raster resample job submitted", job)
}

// RasterCalculator handles POST /raster/calculator
func (h *Handler) RasterCalculator(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterCalculatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterCalculator(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Raster calculator job submitted", job)
}

// RasterAlign handles POST /raster/align
func (h *Handler) RasterAlign(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterAlignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterAlign(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Raster align job submitted", job)
}

// RasterMosaic handles POST /raster/mosaic
func (h *Handler) RasterMosaic(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterMosaicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterMosaic(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Raster mosaic job submitted", job)
}

// RasterContours handles POST /raster/contours
func (h *Handler) RasterContours(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterContoursRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterContours(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Contour generation job submitted", job)
}

// RasterHillshade handles POST /raster/hillshade
func (h *Handler) RasterHillshade(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.RasterHillshadeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitRasterHillshade(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Hillshade generation job submitted", job)
}

// ============================================================================
// MODULE H — TOPOLOGY VALIDATION & FIXES
// ============================================================================

// TopologyValidation handles POST /analysis/topology
func (h *Handler) TopologyValidation(w http.ResponseWriter, r *http.Request, user *models.User, sessionID string) {
	var req models.TopologyValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	job, err := h.svc.SubmitTopologyValidation(r.Context(), userIDFromUser(user), sessionID, req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusAccepted, "Topology validation job submitted", job)
}

// GetTopologyResults handles GET /analysis/topology/:jobId/results
func (h *Handler) GetTopologyResults(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	job, err := h.svc.GetProcessingJob(r.Context(), jobID, userIDFromUser(user))
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	errs, _ := h.svc.GetTopologyErrors(r.Context(), jobID, userIDFromUser(user))
	writeProcessingSuccess(w, http.StatusOK, "Topology validation results retrieved", map[string]interface{}{
		"job":         job,
		"errors":      errs,
		"error_count": len(errs),
	})
}

// GetTopologyErrors handles GET /analysis/topology/:jobId/errors
func (h *Handler) GetTopologyErrors(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	errs, err := h.svc.GetTopologyErrors(r.Context(), jobID, userIDFromUser(user))
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	if errs == nil {
		errs = []*models.TopologyError{}
	}
	writeProcessingSuccess(w, http.StatusOK, "Topology errors retrieved", map[string]interface{}{
		"job_id": jobID,
		"errors": errs,
		"total":  len(errs),
	})
}

// FixTopologyErrors handles POST /analysis/topology/:jobId/fix
func (h *Handler) FixTopologyErrors(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	var req models.TopologyFixRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	res, err := h.svc.FixTopologyErrors(r.Context(), jobID, userIDFromUser(user), req)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusOK, "Topology errors resolved", res)
}

// ============================================================================
// PROCESSING JOBS & PRESETS MANAGEMENT
// ============================================================================

// ListJobs handles GET /jobs
func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request, user *models.User) {
	mapID := strings.TrimSpace(r.URL.Query().Get("map_id"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs, err := h.svc.ListProcessingJobs(r.Context(), userIDFromUser(user), mapID, limit)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	if jobs == nil {
		jobs = []*models.ProcessingJob{}
	}
	writeProcessingSuccess(w, http.StatusOK, "Processing jobs listed", map[string]interface{}{
		"jobs":  jobs,
		"total": len(jobs),
	})
}

// GetJob handles GET /jobs/:jobId
func (h *Handler) GetJob(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	job, err := h.svc.GetProcessingJob(r.Context(), jobID, userIDFromUser(user))
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusOK, "Processing job retrieved", job)
}

// CancelJob handles POST /jobs/:jobId/cancel
func (h *Handler) CancelJob(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	job, err := h.svc.CancelProcessingJob(r.Context(), jobID, userIDFromUser(user))
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusOK, "Processing job cancelled", job)
}

// GetJobResult handles GET /jobs/:jobId/result
func (h *Handler) GetJobResult(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	res, err := h.svc.GetProcessingResult(r.Context(), jobID, userIDFromUser(user))
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusOK, "Processing result retrieved", res)
}

// GetJobLogs handles GET /jobs/:jobId/logs
func (h *Handler) GetJobLogs(w http.ResponseWriter, r *http.Request, user *models.User, jobID string) {
	logs, err := h.svc.GetProcessingLogs(r.Context(), jobID, userIDFromUser(user))
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	if logs == nil {
		logs = []models.ProcessingJobLog{}
	}
	writeProcessingSuccess(w, http.StatusOK, "Processing logs retrieved", map[string]interface{}{
		"job_id": jobID,
		"logs":   logs,
	})
}

// Presets
func (h *Handler) SavePreset(w http.ResponseWriter, r *http.Request, user *models.User) {
	var preset models.ProcessingPreset
	if err := json.NewDecoder(r.Body).Decode(&preset); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", nil, nil)
		return
	}
	saved, err := h.svc.SavePreset(r.Context(), userIDFromUser(user), &preset)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusCreated, "Preset saved", saved)
}

func (h *Handler) ListPresets(w http.ResponseWriter, r *http.Request, user *models.User) {
	alg := strings.TrimSpace(r.URL.Query().Get("algorithm_name"))
	presets, err := h.svc.ListPresets(r.Context(), userIDFromUser(user), alg)
	if err != nil {
		writeProcessingError(w, err)
		return
	}
	if presets == nil {
		presets = []*models.ProcessingPreset{}
	}
	writeProcessingSuccess(w, http.StatusOK, "Presets listed", map[string]interface{}{
		"presets": presets,
	})
}

func (h *Handler) DeletePreset(w http.ResponseWriter, r *http.Request, user *models.User, presetID string) {
	if err := h.svc.DeletePreset(r.Context(), presetID, userIDFromUser(user)); err != nil {
		writeProcessingError(w, err)
		return
	}
	writeProcessingSuccess(w, http.StatusOK, "Preset deleted", map[string]interface{}{"deleted": true})
}
