package datasets

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	datasetService *services.DatasetService
	repo           *repository.SessionRepository
}

func NewHandler(ds *services.DatasetService, repo *repository.SessionRepository) *Handler {
	return &Handler{
		datasetService: ds,
		repo:           repo,
	}
}

// CreateHandler creates a demonstration dataset response
func CreateHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateDatasetRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "Invalid request body", nil, nil)
		return
	}

	if req.Name == "" {
		field := "name"
		models.WriteError(w, http.StatusBadRequest, "REQUIRED_FIELD", "Dataset name is required", nil, &field)
		return
	}

	if req.Format != "png" && req.Format != "jpg" && req.Format != "jpeg" {
		field := "format"
		models.WriteError(w, http.StatusBadRequest, "INVALID_FORMAT", "Invalid dataset format", nil, &field)
		return
	}

	response := CreateDatasetResponse{
		DatasetID:     repository.GenerateDatasetID(),
		Name:          req.Name,
		Format:        req.Format,
		Georeferenced: false,
		Status:        "metadata_received",
	}

	models.WriteJSON(w, http.StatusCreated, response)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteJSON(w, http.StatusOK, DatasetListResponse{Datasets: []*models.Dataset{}})
		return
	}

	datasets, err := h.datasetService.ListDatasets(sessionID)
	if err != nil {
		models.WriteJSON(w, http.StatusOK, DatasetListResponse{Datasets: []*models.Dataset{}})
		return
	}
	if datasets == nil {
		datasets = []*models.Dataset{}
	}

	models.WriteJSON(w, http.StatusOK, DatasetListResponse{Datasets: datasets})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	dataset, err := h.datasetService.GetDataset(sessionID, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, dataset)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	if err := h.datasetService.DeleteDataset(sessionID, datasetID); err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, DeleteDatasetResponse{
		ID:      datasetID,
		Deleted: true,
	})
}

func (h *Handler) SourceImage(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	dataset, err := h.datasetService.GetDataset(sessionID, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	if _, err := os.Stat(dataset.StoragePath); os.IsNotExist(err) {
		models.WriteError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Source image file was not found", nil, nil)
		return
	}

	ext := strings.ToLower(filepath.Ext(dataset.Filename))
	contentType := "image/png"
	if ext == ".jpg" || ext == ".jpeg" {
		contentType = "image/jpeg"
	}

	w.Header().Set("Content-Type", contentType)
	http.ServeFile(w, r, dataset.StoragePath)
}

func (h *Handler) ListLayers(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	layers, err := h.repo.ListLayers(sessionID, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, LayersResponse{Layers: layers})
}

func (h *Handler) UpdateLayer(w http.ResponseWriter, r *http.Request, datasetID, layerID string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only PATCH or PUT method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	var req UpdateLayerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "Failed to parse JSON body", err.Error(), nil)
		return
	}

	updated, err := h.repo.UpdateLayer(sessionID, datasetID, layerID, req.Visible, req.Opacity)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "LAYER_NOT_FOUND", "Layer not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, updated)
}
