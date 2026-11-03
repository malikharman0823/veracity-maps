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

func exportContentType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".mbtiles"):
		return "application/octet-stream"
	case strings.HasSuffix(lower, ".tif"), strings.HasSuffix(lower, ".tiff"):
		return "image/tiff"
	default:
		return "application/zip"
	}
}

func (h *Handler) Export(w http.ResponseWriter, r *http.Request, datasetID string) {
	sessionID := models.GetSessionID(r)
	if sessionID == "" {
		models.WriteError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "Session is missing or expired", nil, nil)
		return
	}

	format := "project_zip"
	var req ExportRequest
	isSelective := false

	switch r.Method {
	case http.MethodPost:
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Format != "" {
			format = req.Format
		}
		if len(req.BuildingIDs) > 0 || len(req.FloorIDs) > 0 || len(req.LayerIDs) > 0 || req.Options != nil || req.ProjectName != "" || req.ZoomConfig != nil {
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
				ZoomConfig:              req.Options.ZoomConfig,
			}
		} else if req.ZoomConfig != nil {
			selOpts = services.DefaultSelectiveExportOptions()
			selOpts.ZoomConfig = req.ZoomConfig
		}
		if selOpts != nil && selOpts.ZoomConfig == nil && req.ZoomConfig != nil {
			selOpts.ZoomConfig = req.ZoomConfig
		}
		filePath, filename, err = h.exportService.GenerateSelectiveExport(sessionID, datasetID, services.SelectiveExportParams{
			Format:      format,
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

	accept := r.Header.Get("Accept")
	wantsBinaryStream := r.Method == http.MethodGet ||
		accept == "application/zip" ||
		strings.Contains(accept, "octet-stream") ||
		strings.Contains(accept, "application/x-sqlite3")

	if wantsBinaryStream {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Type", exportContentType(filename))
		http.ServeFile(w, r, filePath)
		return
	}

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

	requestedFile := filepath.Base(strings.TrimSpace(r.URL.Query().Get("file")))
	if requestedFile == "" || requestedFile == "." || requestedFile == "/" {
		requestedFile = "export.zip"
	}

	// 1. Serve the already-generated export artifact directly from <datasetDir>/exports/<requestedFile>
	var filePath string
	if sm := h.exportService.StorageManager(); sm != nil {
		candidate := filepath.Join(sm.GetDatasetDir(sessionID, exportID), "exports", requestedFile)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			filePath = candidate
		}
	}

	// 2. Fallback: generate matching format on demand (.mbtiles vs project_zip)
	if filePath == "" {
		fallbackFormat := "project_zip"
		if strings.HasSuffix(strings.ToLower(requestedFile), ".mbtiles") {
			fallbackFormat = "mbtiles"
		}
		var err error
		filePath, _, err = h.exportService.GenerateExport(sessionID, exportID, fallbackFormat)
		if err != nil {
			models.WriteError(w, http.StatusNotFound, "EXPORT_NOT_FOUND", "Export could not be generated: "+err.Error(), nil, nil)
			return
		}
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		models.WriteError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Export artifact was not found", nil, nil)
		return
	}

	serveName := filepath.Base(filePath)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, serveName))
	w.Header().Set("Content-Type", exportContentType(serveName))
	http.ServeFile(w, r, filePath)
}
