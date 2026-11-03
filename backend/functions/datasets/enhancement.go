package datasets

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

func splitServiceError(err error, defaultCode string) (string, string) {
	if err == nil {
		return defaultCode, ""
	}
	msg := err.Error()
	if strings.Contains(msg, ":") {
		parts := strings.SplitN(msg, ":", 2)
		code := strings.TrimSpace(parts[0])
		if code != "" && !strings.Contains(code, " ") {
			return code, strings.TrimSpace(parts[1])
		}
	}
	return defaultCode, msg
}

func (h *Handler) GetInspection(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	res, err := h.datasetService.GetUniversalInspection(sessionID, datasetID)
	if err != nil {
		code, msg := splitServiceError(err, "INSPECTION_FAILED")
		models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) HandleEnhancement(w http.ResponseWriter, r *http.Request, datasetID string) {
	dataset, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil || dataset == nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	if r.Method == http.MethodGet {
		var enh interface{}
		if dataset.ExtractedMetadata != nil {
			enh = dataset.ExtractedMetadata["enhancement"]
		}
		if enh == nil {
			enh = map[string]interface{}{
				"enabled":        false,
				"active_variant": "original",
			}
		}
		models.WriteJSON(w, http.StatusOK, enh)
		return
	}

	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET and POST methods are supported", nil, nil)
		return
	}

	var opts models.EnhancementOptions
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&opts)
	}

	state, err := h.datasetService.EnhanceDataset(sessionID, datasetID, opts)
	if err != nil {
		code, msg := splitServiceError(err, "ENHANCEMENT_FAILED")
		models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, state)
}

func (h *Handler) HandleOCR(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	var req struct {
		MinConfidence float64 `json:"min_confidence"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	anns, err := h.datasetService.RunDatasetOCR(sessionID, datasetID, req.MinConfidence)
	if err != nil {
		code, msg := splitServiceError(err, "OCR_FAILED")
		models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"dataset_id":  datasetID,
		"count":       len(anns),
		"annotations": anns,
	})
}

func (h *Handler) HandleAnnotations(w http.ResponseWriter, r *http.Request, datasetID string) {
	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	if r.Method == http.MethodGet {
		anns, err := h.datasetService.ListAnnotations(sessionID, datasetID)
		if err != nil {
			code, msg := splitServiceError(err, "ANNOTATIONS_READ_FAILED")
			models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"dataset_id":  datasetID,
			"count":       len(anns),
			"annotations": anns,
		})
		return
	}

	if r.Method == http.MethodPost {
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", err.Error(), nil)
			return
		}
		annID, _ := patch["id"].(string)
		saved, err := h.datasetService.UpsertAnnotation(sessionID, datasetID, annID, patch)
		if err != nil {
			code, msg := splitServiceError(err, "ANNOTATION_SAVE_FAILED")
			models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
			return
		}
		models.WriteJSON(w, http.StatusCreated, saved)
		return
	}

	models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET and POST methods are supported", nil, nil)
}

func (h *Handler) HandleAnnotationItem(w http.ResponseWriter, r *http.Request, datasetID, annotationID string) {
	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		var patch map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", err.Error(), nil)
			return
		}
		saved, err := h.datasetService.UpsertAnnotation(sessionID, datasetID, annotationID, patch)
		if err != nil {
			code, msg := splitServiceError(err, "ANNOTATION_UPDATE_FAILED")
			models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
			return
		}
		models.WriteJSON(w, http.StatusOK, saved)
		return
	}

	if r.Method == http.MethodDelete {
		if err := h.datasetService.DeleteAnnotation(sessionID, datasetID, annotationID); err != nil {
			code, msg := splitServiceError(err, "ANNOTATION_NOT_FOUND")
			status := http.StatusBadRequest
			if code == "NOT_FOUND" {
				status = http.StatusNotFound
			}
			models.WriteError(w, status, code, msg, nil, nil)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"id":      annotationID,
			"deleted": true,
		})
		return
	}

	models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only PUT, PATCH, and DELETE methods are supported", nil, nil)
}

func (h *Handler) GetVectors(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	fc, err := h.datasetService.GetDatasetVectorsGeoJSON(sessionID, datasetID)
	if err != nil {
		code, msg := splitServiceError(err, "VECTORS_READ_FAILED")
		models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, fc)
}

func (h *Handler) HandleGenerateTiles(w http.ResponseWriter, r *http.Request, datasetID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST method is supported", nil, nil)
		return
	}

	_, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	var req struct {
		Resampling string `json:"resampling"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	insp, mbtilesPath, err := h.datasetService.GenerateDatasetMBTiles(sessionID, datasetID, req.Resampling)
	if err != nil {
		code, msg := splitServiceError(err, "MBTILES_GENERATION_FAILED")
		models.WriteError(w, http.StatusBadRequest, code, msg, nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"dataset_id":        datasetID,
		"tile_status":       "completed",
		"mbtiles_path":      filepath.Base(mbtilesPath),
		"tile_url_template": "/datasets/" + datasetID + "/tiles/{z}/{x}/{y}",
		"inspection":        insp,
	})
}

func (h *Handler) ServeTile(w http.ResponseWriter, r *http.Request, datasetID, zStr, xStr, yRaw string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	dataset, sessionID, err := h.resolveDatasetAndSession(r, w, datasetID)
	if err != nil || dataset == nil {
		var foundSess string
		dataset, foundSess, err = h.datasetService.FindDataset(datasetID)
		if err == nil && foundSess != "" {
			sessionID = foundSess
			w.Header().Set("X-Session-ID", foundSess)
		}
	}
	if err != nil || dataset == nil {
		models.WriteError(w, http.StatusNotFound, "DATASET_NOT_FOUND", "Dataset not found", nil, nil)
		return
	}

	yClean := yRaw
	if dotIdx := strings.IndexByte(yClean, '.'); dotIdx >= 0 {
		yClean = yClean[:dotIdx]
	}

	z, errZ := strconv.Atoi(zStr)
	x, errX := strconv.Atoi(xStr)
	y, errY := strconv.Atoi(yClean)
	if errZ != nil || errX != nil || errY != nil || z < 0 || z > 25 || x < 0 || y < 0 {
		models.WriteError(w, http.StatusBadRequest, "INVALID_TILE_COORDS", "Invalid z/x/y tile coordinates", nil, nil)
		return
	}

	mbtilesPath := ""
	if dataset.TilePath != "" && fileExists(dataset.TilePath) {
		mbtilesPath = dataset.TilePath
	} else if strings.EqualFold(filepath.Ext(dataset.StoragePath), ".mbtiles") && fileExists(dataset.StoragePath) {
		mbtilesPath = dataset.StoragePath
	} else {
		// Also check exports/project.mbtiles or staged mbtiles if available
		exportDir := filepath.Join(h.datasetService.StorageManager().GetDatasetDir(sessionID, dataset.ID), "exports")
		cand1 := filepath.Join(exportDir, "project.mbtiles")
		if fileExists(cand1) && gis.HasSQLiteHeader(cand1) {
			mbtilesPath = cand1
		}
	}
	if mbtilesPath == "" && dataset.GeoreferenceStatus == "completed" {
		if _, genPath, genErr := h.datasetService.GenerateDatasetMBTiles(sessionID, dataset.ID, ""); genErr == nil && fileExists(genPath) {
			mbtilesPath = genPath
		}
	}

	if mbtilesPath == "" || !fileExists(mbtilesPath) {
		models.WriteError(w, http.StatusNotFound, "MBTILES_NOT_FOUND", "No MBTiles tile database exists for this dataset", nil, nil)
		return
	}

	datasetDir := h.datasetService.StorageManager().GetDatasetDir(sessionID, dataset.ID)
	if !fileExists(datasetDir) && dataset.StoragePath != "" {
		datasetDir = filepath.Dir(dataset.StoragePath)
	}
	xyzCacheDir := filepath.Join(datasetDir, "tiles_xyz")

	scheme := "tms"
	tileFormat := "png"
	if dataset.ExtractedMetadata != nil {
		if sc, ok := dataset.ExtractedMetadata["tile_scheme"].(string); ok && sc != "" {
			scheme = sc
		}
		if tf, ok := dataset.ExtractedMetadata["tile_format"].(string); ok && tf != "" {
			tileFormat = tf
		}
	}

	tileBytes, contentType, isGzipped, readErr := gis.ReadMBTilesTile(mbtilesPath, xyzCacheDir, scheme, tileFormat, z, x, y)
	if errors.Is(readErr, os.ErrNotExist) || len(tileBytes) == 0 {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if readErr != nil {
		models.WriteError(w, http.StatusInternalServerError, "TILE_READ_ERROR", readErr.Error(), nil, nil)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if isGzipped {
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(tileBytes)
}
