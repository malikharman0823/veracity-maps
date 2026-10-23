package upload

import (
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

type Handler struct {
	datasetService *services.DatasetService
	repo           *repository.SessionRepository
	storage        *storage.StorageManager
}

func NewHandler(ds *services.DatasetService, repo *repository.SessionRepository, sm *storage.StorageManager) *Handler {
	return &Handler{
		datasetService: ds,
		repo:           repo,
		storage:        sm,
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

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == ".pdf" {
		docID := repository.GenerateDatasetID()
		sessionDir := h.storage.GetSessionDir(sessionID)
		rawDocsDir := filepath.Join(sessionDir, "raw_docs")
		_ = os.MkdirAll(rawDocsDir, 0755)

		destPath := filepath.Join(rawDocsDir, docID+"_"+header.Filename)
		src, err := header.Open()
		if err != nil {
			models.WriteError(w, http.StatusBadRequest, "UPLOAD_FAILED", "Failed to open PDF: "+err.Error(), nil, nil)
			return
		}
		defer src.Close()

		dst, err := os.Create(destPath)
		if err != nil {
			models.WriteError(w, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to store PDF: "+err.Error(), nil, nil)
			return
		}
		size, err := io.Copy(dst, src)
		_ = dst.Close()
		if err != nil {
			models.WriteError(w, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to write PDF: "+err.Error(), nil, nil)
			return
		}

		inspectRes, err := h.datasetService.InspectPdfDocument(sessionID, docID, destPath, header.Filename, size)
		if err != nil {
			log.Printf("[UPLOAD ERROR] InspectPdfDocument failed: %v", err)
			statusCode := http.StatusBadRequest
			errCode := "PDF_INSPECTION_FAILED"
			msg := err.Error()
			if strings.Contains(msg, ":") {
				parts := strings.SplitN(msg, ":", 2)
				errCode = strings.TrimSpace(parts[0])
				msg = strings.TrimSpace(parts[1])
			}
			models.WriteError(w, statusCode, errCode, msg, nil, nil)
			return
		}

		w.Header().Set("X-Session-ID", sessionID)
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"type":        "pdf_pending",
			"document_id": docID,
			"filename":    header.Filename,
			"file_size":   size,
			"page_count":  inspectRes.PageCount,
			"pages":       inspectRes.Pages,
		})
		return
	}

	dataset, err := h.datasetService.HandleUpload(sessionID, header)
	if err != nil {
		log.Printf("[UPLOAD ERROR] HandleUpload failed for %s: %v", header.Filename, err)
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

func (h *Handler) ImportPdfPages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "No active session ID provided", nil, nil)
		return
	}

	var req models.ImportPdfPagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse JSON body", err.Error(), nil)
		return
	}

	res, err := h.datasetService.ImportPdfPages(sessionID, &req)
	if err != nil {
		statusCode := http.StatusBadRequest
		errCode := "PDF_IMPORT_FAILED"
		msg := err.Error()
		if strings.Contains(msg, ":") {
			parts := strings.SplitN(msg, ":", 2)
			errCode = strings.TrimSpace(parts[0])
			msg = strings.TrimSpace(parts[1])
		}
		models.WriteError(w, statusCode, errCode, msg, nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) GetPdfThumbnail(w http.ResponseWriter, r *http.Request, docID, pageStr string) {
	sessionID := models.GetSessionID(r)
	var thumbPath string

	if sessionID != "" {
		sessionDir := h.storage.GetSessionDir(sessionID)
		candidate := filepath.Join(sessionDir, "pdf_thumbnails", docID, "thumb_page_"+pageStr+".png")
		if _, err := os.Stat(candidate); err == nil {
			thumbPath = candidate
		}
	}

	// Dynamic fallback: If session header wasn't passed by browser <img> tag, look in sessions directory
	if thumbPath == "" {
		sessionsBase := h.storage.BaseSessionDir
		if entries, err := os.ReadDir(sessionsBase); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					candidate := filepath.Join(sessionsBase, entry.Name(), "pdf_thumbnails", docID, "thumb_page_"+pageStr+".png")
					if _, err := os.Stat(candidate); err == nil {
						thumbPath = candidate
						break
					}
				}
			}
		}
	}

	if thumbPath == "" {
		models.WriteError(w, http.StatusNotFound, "THUMBNAIL_NOT_FOUND", "PDF thumbnail not found", nil, nil)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, thumbPath)
}

func (h *Handler) InspectArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		sess := h.repo.CreateSession()
		sessionID = sess.ID
		w.Header().Set("X-Session-ID", sessionID)
	}

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
		models.WriteError(w, http.StatusBadRequest, "MISSING_FILE", "Upload request must include a 'file' or 'files' field", "No file found in payload", &field)
		return
	}

	result, err := h.datasetService.InspectArchive(sessionID, header)
	if err != nil {
		statusCode := http.StatusBadRequest
		errCode := "INSPECTION_FAILED"
		msg := err.Error()
		if strings.Contains(msg, ":") {
			parts := strings.SplitN(msg, ":", 2)
			errCode = strings.TrimSpace(parts[0])
			msg = strings.TrimSpace(parts[1])
		}
		models.WriteError(w, statusCode, errCode, msg, nil, nil)
		return
	}

	w.Header().Set("X-Session-ID", sessionID)
	models.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) ImportArchiveSelected(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is allowed", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "No active session ID provided", nil, nil)
		return
	}

	var req struct {
		ArchiveToken string                             `json:"archive_token"`
		Token        string                             `json:"token"`
		Selections   []*services.DatasetImportSelection `json:"selections"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse JSON body", err.Error(), nil)
		return
	}

	token := req.ArchiveToken
	if token == "" {
		token = req.Token
	}

	res, err := h.datasetService.ImportArchiveSelected(sessionID, token, req.Selections)
	if err != nil {
		log.Printf("ImportArchiveSelected error: %v (sessionID=%s, token=%s, selectionsCount=%d)", err, sessionID, token, len(req.Selections))
		models.WriteError(w, http.StatusBadRequest, "IMPORT_FAILED", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, res)
}
