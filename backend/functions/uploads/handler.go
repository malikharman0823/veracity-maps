package uploads

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/services"
)

// Handler serves the unified, user-isolated Upload, Import, Job, and Dataset endpoints.
type Handler struct {
	svc *services.UnifiedImportService
}

func NewHandler(svc *services.UnifiedImportService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetService() *services.UnifiedImportService {
	return h.svc
}

func writeUnifiedError(w http.ResponseWriter, err error) {
	var ve *gis.ValidationError
	if errors.As(err, &ve) {
		status := http.StatusBadRequest
		if ve.Code == "FILE_TOO_LARGE" || ve.Code == "STORAGE_QUOTA_EXCEEDED" {
			status = http.StatusRequestEntityTooLarge
		}
		models.WriteError(w, status, ve.Code, ve.Message, map[string]interface{}{"stage": ve.Stage}, nil)
		return
	}
	switch {
	case errors.Is(err, services.ErrUnifiedForbidden):
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil, nil)
	case errors.Is(err, services.ErrUnifiedNotFound):
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil, nil)
	case errors.Is(err, services.ErrTooManyConcurrentJobs):
		models.WriteError(w, http.StatusTooManyRequests, "CONCURRENT_JOB_LIMIT", err.Error(), nil, nil)
	case errors.Is(err, services.ErrJobNotCancellable):
		models.WriteError(w, http.StatusConflict, "JOB_NOT_CANCELLABLE", err.Error(), nil, nil)
	default:
		models.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil, nil)
	}
}

// PostUpload handles POST /api/uploads (single file per request; multi-file queues invoke independently).
func (h *Handler) PostUpload(w http.ResponseWriter, r *http.Request, user *models.User) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Failed to parse multipart upload", err.Error(), nil)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		field := "file"
		models.WriteError(w, http.StatusBadRequest, "MISSING_FILE", "Upload must include a 'file' field", nil, &field)
		return
	}
	defer file.Close()

	projectID := strings.TrimSpace(r.FormValue("project_id"))
	category := gis.ImportCategory(strings.ToLower(strings.TrimSpace(r.FormValue("import_category"))))

	rec, err := h.svc.UploadAndInspect(r.Context(), user.ID, projectID, category, header.Filename, file)
	if err != nil {
		writeUnifiedError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, rec)
}

// ListUploads handles GET /api/uploads.
func (h *Handler) ListUploads(w http.ResponseWriter, _ *http.Request, user *models.User) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	uploads := h.svc.ListUploads(user.ID)
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"uploads": uploads,
		"total":   len(uploads),
	})
}

// GetUpload handles GET /api/uploads/{id}.
func (h *Handler) GetUpload(w http.ResponseWriter, _ *http.Request, user *models.User, uploadID string) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	rec, err := h.svc.GetUpload(user.ID, uploadID)
	if err != nil {
		writeUnifiedError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, rec)
}

// StartImport handles POST /api/imports/{uploadId}.
func (h *Handler) StartImport(w http.ResponseWriter, r *http.Request, user *models.User, uploadID string) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	var req services.StartImportRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	job, err := h.svc.StartImport(user.ID, uploadID, req)
	if err != nil {
		writeUnifiedError(w, err)
		return
	}
	status := http.StatusAccepted
	if req.Sync {
		if job.Status == "ready" {
			status = http.StatusOK
		} else if job.Status == "failed" {
			status = http.StatusUnprocessableEntity
		}
	}
	models.WriteJSON(w, status, job)
}

// GetJob handles GET /api/jobs/{id}.
func (h *Handler) GetJob(w http.ResponseWriter, _ *http.Request, user *models.User, jobID string) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	job, err := h.svc.GetJob(user.ID, jobID)
	if err != nil {
		writeUnifiedError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, job)
}

// CancelJob handles POST /api/jobs/{id}/cancel.
func (h *Handler) CancelJob(w http.ResponseWriter, _ *http.Request, user *models.User, jobID string) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	job, err := h.svc.CancelJob(user.ID, jobID)
	if err != nil {
		writeUnifiedError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, job)
}

// ListDatasets handles GET /api/unified-datasets.
func (h *Handler) ListDatasets(w http.ResponseWriter, r *http.Request, user *models.User) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	items := h.svc.ListDatasets(user.ID, projectID)
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"datasets": items,
		"total":    len(items),
	})
}

// GetDataset handles GET /api/unified-datasets/{id}.
func (h *Handler) GetDataset(w http.ResponseWriter, _ *http.Request, user *models.User, datasetID string) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	ds, layers, err := h.svc.GetDataset(user.ID, datasetID)
	if err != nil {
		writeUnifiedError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"dataset": ds,
		"layers":  layers,
	})
}

// DeleteDataset handles DELETE /api/unified-datasets/{id}.
func (h *Handler) DeleteDataset(w http.ResponseWriter, _ *http.Request, user *models.User, datasetID string) {
	if user == nil {
		models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", nil, nil)
		return
	}
	if err := h.svc.DeleteDataset(user.ID, datasetID); err != nil {
		writeUnifiedError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "id": datasetID})
}
