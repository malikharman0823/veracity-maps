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

	for _, ds := range datasets {
		ensureDatasetMapLayer(ds)
	}

	models.WriteJSON(w, http.StatusOK, DatasetListResponse{Datasets: datasets})
}

func (h *Handler) resolveDatasetAndSession(r *http.Request, w http.ResponseWriter, datasetID string) (*models.Dataset, string, error) {
	sessionID := models.GetSessionID(r)
	if sessionID != "" {
		dataset, err := h.datasetService.GetDataset(sessionID, datasetID)
		if err == nil && dataset != nil {
			return dataset, sessionID, nil
		}
	}

	dataset, foundSess, err := h.datasetService.FindDataset(datasetID)
	if err != nil || dataset == nil {
		return nil, "", repository.ErrDatasetNotFound
	}
	if foundSess != "" && w != nil {
		w.Header().Set("X-Session-ID", foundSess)
	}
	return dataset, foundSess, nil
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	dataset, _, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	ensureDatasetMapLayer(dataset)

	models.WriteJSON(w, http.StatusOK, dataset)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only DELETE method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
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

	dataset, _, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil || dataset == nil {
		// Image stream fallback if session ID was stale or missing
		var foundSess string
		dataset, foundSess, err = h.datasetService.FindDataset(datasetID)
		if err == nil && foundSess != "" && w != nil {
			w.Header().Set("X-Session-ID", foundSess)
		}
	}
	if err != nil || dataset == nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	imagePath := dataset.StoragePath
	transparentPNG := filepath.Join(filepath.Dir(dataset.StoragePath), "processed", "processed_transparent.png")
	useTransparent := false
	if bg, ok := dataset.ExtractedMetadata["background_removal"].(map[string]interface{}); ok {
		if enabled, _ := bg["enabled"].(bool); enabled {
			useTransparent = true
		}
	}
	if useTransparent && fileExists(transparentPNG) {
		imagePath = transparentPNG
	} else if dataset.ProcessedPath != "" && fileExists(dataset.ProcessedPath) {
		pExt := strings.ToLower(filepath.Ext(dataset.ProcessedPath))
		if pExt == ".png" || pExt == ".jpg" || pExt == ".jpeg" {
			imagePath = dataset.ProcessedPath
		}
	}

	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		models.WriteError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Source image file was not found", nil, nil)
		return
	}

	ext := strings.ToLower(filepath.Ext(imagePath))
	contentType := "image/png"
	if ext == ".jpg" || ext == ".jpeg" {
		contentType = "image/jpeg"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.ServeFile(w, r, imagePath)
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
