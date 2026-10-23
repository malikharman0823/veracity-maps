package export

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	exportService *services.ExportService
	repo          *repository.SessionRepository
}

func NewHandler(es *services.ExportService, repo *repository.SessionRepository) *Handler {
	return &Handler{
		exportService: es,
		repo:          repo,
	}
}

func (h *Handler) Export(w http.ResponseWriter, r *http.Request, datasetID string) {
	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	format := "mtmap"
	var req ExportRequest
	isSelective := false

	switch r.Method {
	case http.MethodPost:
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Format != "" {
			format = req.Format
		}
		if len(req.BuildingIDs) > 0 || len(req.FloorIDs) > 0 || len(req.LayerIDs) > 0 || req.Options != nil {
			isSelective = true
		}
	case http.MethodGet:
		if qFormat := r.URL.Query().Get("format"); qFormat != "" {
			format = qFormat
		}
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET or POST method is supported", nil, nil)
		return
	}

	var filePath, filename string
	var err error

	if isSelective {
		var selOpts *services.SelectiveExportOptions
		if req.Options != nil {
			selOpts = &services.SelectiveExportOptions{
				IncludeBuildingConfig:   req.Options.IncludeBuildingConfig,
				IncludeControlPoints:    req.Options.IncludeControlPoints,
				IncludeIcons:            req.Options.IncludeIcons,
				IncludeLayerStyles:      req.Options.IncludeLayerStyles,
				IncludeOriginalRasters:  req.Options.IncludeOriginalRaster,
				IncludeProcessedRasters: req.Options.IncludeProcessedRaster,
				IncludeBaseMapSettings:  req.Options.IncludeBaseMapSettings,
				IncludeProjectMetadata:  req.Options.IncludeProjectMetadata,
			}
		}
		filePath, filename, err = h.exportService.GenerateSelectiveExport(sessionID, datasetID, services.SelectiveExportParams{
			ProjectName: req.ProjectName,
			BuildingIDs: req.BuildingIDs,
			FloorIDs:    req.FloorIDs,
			LayerIDs:    req.LayerIDs,
			Options:     selOpts,
		})
	} else {
		filePath, filename, err = h.exportService.GenerateExport(sessionID, datasetID, format)
	}

	if err != nil {
		errCode := "EXPORT_FAILED"
		msg := err.Error()
		if strings.Contains(msg, ":") {
			parts := strings.SplitN(msg, ":", 2)
			errCode = strings.TrimSpace(parts[0])
			msg = strings.TrimSpace(parts[1])
		}
		models.WriteError(w, http.StatusBadRequest, errCode, msg, nil, nil)
		return
	}

	// If client requested via GET (frontend browser download), stream file directly
	if r.Method == http.MethodGet || r.Header.Get("Accept") == "application/zip" || strings.Contains(r.Header.Get("Accept"), "octet-stream") {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Type", "application/zip")
		http.ServeFile(w, r, filePath)
		return
	}

	// Otherwise, return JSON response with download url
	downloadURL := fmt.Sprintf("/api/exports/%s/download?session_id=%s&file=%s", datasetID, sessionID, filename)
	models.WriteJSON(w, http.StatusOK, ExportResponse{
		ExportID:    datasetID,
		DownloadURL: downloadURL,
		Format:      format,
		Filename:    filename,
	})
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request, exportID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	filename := r.URL.Query().Get("file")
	if filename == "" {
		filename = "export.zip"
	}

	filePath, _, err := h.exportService.GenerateExport(sessionID, exportID, "mtmap")
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "EXPORT_NOT_FOUND", "Export could not be generated", nil, nil)
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		models.WriteError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Export artifact was not found", nil, nil)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(filePath)))
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, filePath)
}
