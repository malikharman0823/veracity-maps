package datasets

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

type BackgroundRemovalRequest struct {
	Action       string  `json:"action"` // "status", "preview", "apply", "reset"
	Color        string  `json:"color"`
	Threshold    float64 `json:"threshold"`
	EdgeSoftness float64 `json:"edge_softness"`
}

type BackgroundRemovalData struct {
	DatasetID       string  `json:"dataset_id"`
	Status          string  `json:"status"`
	Enabled         bool    `json:"enabled"`
	Color           string  `json:"color"`
	Threshold       float64 `json:"threshold"`
	EdgeSoftness    float64 `json:"edge_softness"`
	PreviewDataURL  string  `json:"preview_data_url,omitempty"`
	OriginalDataURL string  `json:"original_data_url,omitempty"`
	TileURLTemplate string  `json:"tile_url_template,omitempty"`
	Message         string  `json:"message"`
	UpdatedAt       string  `json:"updated_at,omitempty"`
}

type BackgroundRemovalResponse struct {
	Data BackgroundRemovalData `json:"data"`
}

// GetBackgroundRemoval handles GET /datasets/{id}/background-removal
func (h *Handler) GetBackgroundRemoval(w http.ResponseWriter, r *http.Request, datasetID string) {
	dataset, _, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil || dataset == nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	meta := dataset.ExtractedMetadata
	if meta == nil {
		meta = make(map[string]interface{})
	}

	bgMeta, _ := meta["background_removal"].(map[string]interface{})
	enabled := false
	colorStr := "#ffffff"
	threshold := 30.0
	edgeSoftness := 10.0
	updatedAt := ""

	if bgMeta != nil {
		if en, ok := bgMeta["enabled"].(bool); ok {
			enabled = en
		}
		if c, ok := bgMeta["color"].(string); ok && c != "" {
			colorStr = c
		}
		if th, ok := bgMeta["threshold"].(float64); ok {
			threshold = th
		}
		if es, ok := bgMeta["edge_softness"].(float64); ok {
			edgeSoftness = es
		}
		if u, ok := bgMeta["updated_at"].(string); ok {
			updatedAt = u
		}
	}

	status := "not_configured"
	msg := "Background removal is not active."
	if enabled {
		status = "completed"
		msg = "Background removal is currently active."
	}

	models.WriteJSON(w, http.StatusOK, BackgroundRemovalData{
		DatasetID:       datasetID,
		Status:          status,
		Enabled:         enabled,
		Color:           colorStr,
		Threshold:       threshold,
		EdgeSoftness:    edgeSoftness,
		TileURLTemplate: "/datasets/" + datasetID + "/tiles/{z}/{x}/{y}.png",
		Message:         msg,
		UpdatedAt:       updatedAt,
	})
}

// HandleBackgroundRemoval handles POST /datasets/{id}/background-removal
func (h *Handler) HandleBackgroundRemoval(w http.ResponseWriter, r *http.Request, datasetID string) {
	dataset, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil || dataset == nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	var req BackgroundRemovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse JSON body", nil, nil)
		return
	}

	if req.Color == "" {
		req.Color = "#ffffff"
	}
	if req.Threshold <= 0 {
		req.Threshold = 30.0
	}
	if req.EdgeSoftness <= 0 {
		req.EdgeSoftness = 10.0
	}

	targetColor, err := gis.ParseHexColor(req.Color)
	if err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_COLOR", err.Error(), nil, nil)
		return
	}

	// 1. Status action
	if req.Action == "status" {
		h.GetBackgroundRemoval(w, r, datasetID)
		return
	}

	datasetDir := filepath.Dir(dataset.StoragePath)
	processedDir := filepath.Join(datasetDir, "processed")
	_ = os.MkdirAll(processedDir, 0755)
	processedPNG := filepath.Join(processedDir, "processed_transparent.png")
	originalBackup := filepath.Join(processedDir, "original_before_bg.png")

	// Active source path for preview: original image before any background removal
	sourcePath := dataset.StoragePath
	if fileExists(originalBackup) {
		sourcePath = originalBackup
	}

	// 2. Preview action
	if req.Action == "preview" {
		origURL, prevURL, err := gis.GenerateRasterPreview(sourcePath, targetColor, req.Threshold, req.EdgeSoftness, 600)
		if err != nil {
			models.WriteError(w, http.StatusInternalServerError, "PREVIEW_FAILED", err.Error(), nil, nil)
			return
		}

		models.WriteJSON(w, http.StatusOK, BackgroundRemovalData{
			DatasetID:       datasetID,
			Status:          "preview",
			Enabled:         false,
			Color:           req.Color,
			Threshold:       req.Threshold,
			EdgeSoftness:    req.EdgeSoftness,
			PreviewDataURL:  prevURL,
			OriginalDataURL: origURL,
			Message:         "Preview generated successfully.",
		})
		return
	}

	// 3. Reset action
	if req.Action == "reset" {
		// Restore original file if backup exists
		if fileExists(originalBackup) {
			_ = copyFile(originalBackup, dataset.StoragePath)
			_ = os.Remove(originalBackup)
		}
		if fileExists(processedPNG) {
			_ = os.Remove(processedPNG)
		}

		if dataset.ExtractedMetadata == nil {
			dataset.ExtractedMetadata = make(map[string]interface{})
		}
		nowStr := time.Now().UTC().Format(time.RFC3339)
		dataset.ExtractedMetadata["background_removal"] = map[string]interface{}{
			"enabled":       false,
			"color":         req.Color,
			"threshold":     req.Threshold,
			"edge_softness": req.EdgeSoftness,
			"updated_at":    nowStr,
		}
		dataset.ProcessedPath = ""
		_ = h.repo.UpdateDataset(sessionID, dataset)

		models.WriteJSON(w, http.StatusOK, BackgroundRemovalData{
			DatasetID:       datasetID,
			Status:          "reset",
			Enabled:         false,
			Color:           req.Color,
			Threshold:       req.Threshold,
			EdgeSoftness:    req.EdgeSoftness,
			TileURLTemplate: "/datasets/" + datasetID + "/tiles/{z}/{x}/{y}.png",
			Message:         "Background removal reset; original raster restored.",
			UpdatedAt:       nowStr,
		})
		return
	}

	// 4. Apply action
	if req.Action == "apply" {
		// Create backup of original before first application
		if !fileExists(originalBackup) && fileExists(dataset.StoragePath) {
			_ = copyFile(dataset.StoragePath, originalBackup)
		}

		// Input file is either existing processed raster or original storage path
		inputPath := dataset.StoragePath
		if fileExists(originalBackup) {
			inputPath = originalBackup
		}

		if err := gis.ProcessRasterBackgroundRemoval(inputPath, processedPNG, targetColor, req.Threshold, req.EdgeSoftness); err != nil {
			models.WriteError(w, http.StatusInternalServerError, "PROCESSING_FAILED", err.Error(), nil, nil)
			return
		}

		nowStr := time.Now().UTC().Format(time.RFC3339)
		if dataset.ExtractedMetadata == nil {
			dataset.ExtractedMetadata = make(map[string]interface{})
		}
		dataset.ExtractedMetadata["background_removal"] = map[string]interface{}{
			"enabled":       true,
			"color":         req.Color,
			"threshold":     req.Threshold,
			"edge_softness": req.EdgeSoftness,
			"updated_at":    nowStr,
		}
		dataset.ProcessedPath = processedPNG
		ensureDatasetMapLayer(dataset)
		_ = h.repo.UpdateDataset(sessionID, dataset)

		models.WriteJSON(w, http.StatusOK, BackgroundRemovalData{
			DatasetID:       datasetID,
			Status:          "completed",
			Enabled:         true,
			Color:           req.Color,
			Threshold:       req.Threshold,
			EdgeSoftness:    req.EdgeSoftness,
			TileURLTemplate: "/datasets/" + datasetID + "/tiles/{z}/{x}/{y}.png",
			Message:         "Background removal applied successfully.",
			UpdatedAt:       nowStr,
		})
		return
	}

	models.WriteError(w, http.StatusBadRequest, "INVALID_ACTION", "Unsupported action: "+req.Action, nil, nil)
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
