package datasets

import (
	"encoding/json"
	"errors"
	"fmt"
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
	reqExt := strings.ToLower(filepath.Ext(yRaw))
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

	datasetDir := h.datasetService.StorageManager().GetDatasetDir(sessionID, dataset.ID)
	if !fileExists(datasetDir) && dataset.StoragePath != "" {
		datasetDir = filepath.Dir(dataset.StoragePath)
	}
	xyzCacheDir := filepath.Join(datasetDir, "tiles_xyz")

	// 1. Direct loose XYZ tile check on disk (from Custom Local Tiles or tile cache)
	exts := []string{".png", ".jpg", ".jpeg", ".pbf", ".mvt"}
	if reqExt == ".pbf" || reqExt == ".mvt" || strings.Contains(r.Header.Get("Accept"), "mapbox-vector-tile") || strings.Contains(r.Header.Get("Accept"), "protobuf") {
		exts = []string{".pbf", ".mvt", ".png", ".jpg", ".jpeg"}
	} else if reqExt == ".jpg" || reqExt == ".jpeg" {
		exts = []string{".jpg", ".jpeg", ".png", ".pbf", ".mvt"}
	} else if reqExt == ".png" {
		exts = []string{".png", ".jpg", ".jpeg", ".pbf", ".mvt"}
	}

	findLooseTile := func(extList []string) (string, string) {
		for _, ext := range extList {
			direct := filepath.Join(xyzCacheDir, fmt.Sprintf("%d", z), fmt.Sprintf("%d", x), fmt.Sprintf("%d%s", y, ext))
			if fileExists(direct) {
				return direct, ext
			}
			// Search vector and raster subdirectories
			for _, sub := range []string{"vector", "raster"} {
				subDir := filepath.Join(xyzCacheDir, sub)
				if entries, err := os.ReadDir(subDir); err == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							p := filepath.Join(subDir, entry.Name(), fmt.Sprintf("%d", z), fmt.Sprintf("%d", x), fmt.Sprintf("%d%s", y, ext))
							if fileExists(p) {
								return p, ext
							}
						}
					}
				}
			}
		}
		return "", ""
	}

	if loosePath, ext := findLooseTile(exts); loosePath != "" {
		b, rErr := os.ReadFile(loosePath)
		if rErr == nil && len(b) > 0 {
			cType := "image/png"
			switch ext {
			case ".jpg", ".jpeg":
				cType = "image/jpeg"
			case ".pbf", ".mvt":
				cType = "application/vnd.mapbox-vector-tile"
				if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
					w.Header().Set("Content-Encoding", "gzip")
				}
			}
			w.Header().Set("Content-Type", cType)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			_, _ = w.Write(b)
			return
		}
	}

	if mbtilesPath == "" || !fileExists(mbtilesPath) {
		models.WriteError(w, http.StatusNotFound, "MBTILES_NOT_FOUND", "No MBTiles or local tile cache exists for this dataset", nil, nil)
		return
	}

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

func sanitizeTileSlug(raw string) string {
	clean := filepath.Base(strings.TrimSpace(raw))
	if clean == "" || clean == "." || clean == ".." || strings.Contains(clean, "/") || strings.Contains(clean, "\\") {
		return ""
	}
	return clean
}

// ServeVectorTile serves a local XYZ MVT/PBF tile from <datasetDir>/vector_tiles/<layerSlug>/tiles/{z}/{x}/{y}.pbf
// (or reopened Viewscape vector_tiles directory), setting Content-Encoding: gzip automatically when gzip-compressed.
func (h *Handler) ServeVectorTile(w http.ResponseWriter, r *http.Request, datasetID, layerSlug, zStr, xStr, yRaw string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}

	slug := sanitizeTileSlug(layerSlug)
	if slug == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_LAYER_SLUG", "Invalid vector tile layer slug", nil, nil)
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

	datasetDir := h.datasetService.StorageManager().GetDatasetDir(sessionID, dataset.ID)
	if !fileExists(datasetDir) && dataset.StoragePath != "" {
		datasetDir = filepath.Dir(dataset.StoragePath)
	}

	candidates := []string{
		filepath.Join(datasetDir, "vector_tiles", slug, "tiles", zStr, xStr, yClean+".pbf"),
		filepath.Join(datasetDir, "vector_tiles", slug, "tiles", zStr, xStr, yClean+".mvt"),
		filepath.Join(datasetDir, "vector_tiles", slug, zStr, xStr, yClean+".pbf"),
		filepath.Join(datasetDir, "tiles_xyz", "vector", slug, "tiles", zStr, xStr, yClean+".pbf"),
		filepath.Join(datasetDir, "tiles_xyz", "vector", slug, zStr, xStr, yClean+".pbf"),
	}
	if dataset.StoragePath != "" {
		storageDir := filepath.Dir(dataset.StoragePath)
		if storageDir != datasetDir {
			candidates = append(candidates,
				filepath.Join(storageDir, "vector_tiles", slug, "tiles", zStr, xStr, yClean+".pbf"),
				filepath.Join(storageDir, "vector_tiles", slug, zStr, xStr, yClean+".pbf"),
			)
		}
	}

	var tilePath string
	for _, cand := range candidates {
		if fileExists(cand) {
			tilePath = cand
			break
		}
	}
	if tilePath == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	b, rErr := os.ReadFile(tilePath)
	if rErr != nil || len(b) == 0 {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.mapbox-vector-tile")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// ServeVectorTileAsset serves metadata.json or style.json for a converted vector tile layer.
func (h *Handler) ServeVectorTileAsset(w http.ResponseWriter, r *http.Request, datasetID, layerSlug, assetName string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is supported", nil, nil)
		return
	}
	slug := sanitizeTileSlug(layerSlug)
	asset := sanitizeTileSlug(assetName)
	if slug == "" || (asset != "metadata.json" && asset != "style.json") {
		models.WriteError(w, http.StatusBadRequest, "INVALID_ASSET", "Only metadata.json or style.json is supported", nil, nil)
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

	datasetDir := h.datasetService.StorageManager().GetDatasetDir(sessionID, dataset.ID)
	if !fileExists(datasetDir) && dataset.StoragePath != "" {
		datasetDir = filepath.Dir(dataset.StoragePath)
	}
	candidates := []string{
		filepath.Join(datasetDir, "vector_tiles", slug, asset),
		filepath.Join(datasetDir, "tiles_xyz", "vector", slug, asset),
	}
	if dataset.StoragePath != "" {
		storageDir := filepath.Dir(dataset.StoragePath)
		if storageDir != datasetDir {
			candidates = append(candidates, filepath.Join(storageDir, "vector_tiles", slug, asset))
		}
	}
	for _, cand := range candidates {
		if fileExists(cand) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			http.ServeFile(w, r, cand)
			return
		}
	}
	models.WriteError(w, http.StatusNotFound, "ASSET_NOT_FOUND", "Vector tile asset not found", nil, nil)
}
