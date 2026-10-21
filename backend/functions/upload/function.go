package upload

import (
	"mime/multipart"
	"net/http"
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

func (h *Handler) UploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		// If no session passed, create an automatic session
		sess := h.repo.CreateSession()
		sessionID = sess.ID
		w.Header().Set("X-Session-ID", sessionID)
	}

	// Max 500 MB memory buffer
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART_REQUEST", "Failed to parse multipart form data", err.Error(), nil)
		return
	}

	var header *multipart.FileHeader
	file, hFile, err := r.FormFile("file")
	if err == nil {
		header = hFile
		_ = file.Close()
	} else if r.MultipartForm != nil {
		if files := r.MultipartForm.File["files"]; len(files) > 0 {
			header = files[0]
		}
	}

	if header == nil {
		field := "file"
		models.WriteError(w, http.StatusBadRequest, "MISSING_FILE", "Upload request must include a 'file' or 'files' field", "No file found in request payload", &field)
		return
	}

	dataset, err := h.datasetService.HandleUpload(sessionID, header)
	if err != nil {
		statusCode := http.StatusBadRequest
		errCode := "UPLOAD_FAILED"
		msg := err.Error()

		if strings.Contains(msg, ":") {
			parts := strings.SplitN(msg, ":", 2)
			errCode = strings.TrimSpace(parts[0])
			msg = strings.TrimSpace(parts[1])
		}
		if errCode == "FILE_TOO_LARGE" {
			statusCode = http.StatusRequestEntityTooLarge
		}
		models.WriteError(w, statusCode, errCode, msg, nil, nil)
		return
	}

	w.Header().Set("X-Session-ID", sessionID)
	models.WriteJSON(w, http.StatusCreated, UploadData{
		ID:                 dataset.ID,
		Filename:           dataset.Filename,
		FileSize:           dataset.FileSize,
		ContentType:        dataset.ContentType,
		FileHash:           dataset.FileHash,
		StoragePath:        dataset.StoragePath,
		Metadata:           dataset.ExtractedMetadata,
		Existing:           false,
		IsGeoreferenced:    dataset.GeoreferenceStatus == "completed",
		GeoreferenceStatus: dataset.GeoreferenceStatus,
	})
}
