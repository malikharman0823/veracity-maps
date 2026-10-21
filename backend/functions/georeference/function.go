package georeference

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	geoService *services.GeoreferenceService
	repo       *repository.SessionRepository
}

func NewHandler(gs *services.GeoreferenceService, repo *repository.SessionRepository) *Handler {
	return &Handler{
		geoService: gs,
		repo:       repo,
	}
}

func (h *Handler) ListControlPoints(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	points, err := h.geoService.ListControlPoints(sessionID, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}
	if points == nil {
		points = []*models.ControlPoint{}
	}

	models.WriteJSON(w, http.StatusOK, ControlPointListResponse{Points: points})
}

func (h *Handler) CreateControlPoint(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	var req CreateControlPointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "Failed to parse JSON body", err.Error(), nil)
		return
	}

	cp := &models.ControlPoint{
		DatasetID: datasetID,
		ImageX:    req.ImageX,
		ImageY:    req.ImageY,
		Longitude: req.Longitude,
		Latitude:  req.Latitude,
	}

	created, err := h.geoService.CreateControlPoint(sessionID, datasetID, cp)
	if err != nil {
		models.WriteError(w, http.StatusBadRequest, "CREATE_POINT_FAILED", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateControlPoint(w http.ResponseWriter, r *http.Request, pointID string) {
	if r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only PUT method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	var req CreateControlPointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "Failed to parse JSON body", err.Error(), nil)
		return
	}

	cp := &models.ControlPoint{
		ID:        pointID,
		ImageX:    req.ImageX,
		ImageY:    req.ImageY,
		Longitude: req.Longitude,
		Latitude:  req.Latitude,
	}

	updated, err := h.geoService.UpdateControlPoint(sessionID, pointID, cp)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "CONTROL_POINT_NOT_FOUND", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteControlPoint(w http.ResponseWriter, r *http.Request, pointID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	deleted, err := h.geoService.DeleteControlPoint(sessionID, pointID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "CONTROL_POINT_NOT_FOUND", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, deleted)
}

func (h *Handler) Georeference(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method == http.MethodGet {
		h.GetConfig(w, r, datasetID)
		return
	}

	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST or GET method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	var req GeoreferenceRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	result, err := h.geoService.Georeference(sessionID, datasetID, req.Action, req.Transformation)
	if err != nil {
		errCode := "GEOREFERENCE_FAILED"
		msg := err.Error()
		if strings.Contains(msg, ":") {
			parts := strings.SplitN(msg, ":", 2)
			errCode = strings.TrimSpace(parts[0])
			msg = strings.TrimSpace(parts[1])
		}
		models.WriteError(w, http.StatusBadRequest, errCode, msg, nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, FromTransformResult(result))
}

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request, datasetID string) {
	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	dataset, err := h.repo.GetDataset(sessionID, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	points, _ := h.repo.ListControlPoints(sessionID, datasetID)
	pointCount := len(points)
	isReusable := dataset.GeoreferenceStatus == "completed" && dataset.ExtractedMetadata["transform"] != nil

	config := map[string]interface{}{
		"dataset_id":          dataset.ID,
		"georeference_status": dataset.GeoreferenceStatus,
		"status":              dataset.GeoreferenceStatus,
		"reusable":            isReusable,
		"control_point_count": pointCount,
		"crs":                 dataset.ExtractedMetadata["crs"],
		"target_crs":          dataset.ExtractedMetadata["crs"],
		"transform":           dataset.ExtractedMetadata["transform"],
		"bounds":              dataset.ExtractedMetadata["bounds"],
		"source_width":        dataset.ExtractedMetadata["width"],
		"source_height":       dataset.ExtractedMetadata["height"],
		"output_format":       dataset.ExtractedMetadata["format"],
	}

	models.WriteJSON(w, http.StatusOK, GeoreferenceConfigResponse{Configuration: config})
}
