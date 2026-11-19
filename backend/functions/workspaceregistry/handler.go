package workspaceregistry

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	svc *services.WorkspaceRegistryService
}

func NewHandler(svc *services.WorkspaceRegistryService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetService() *services.WorkspaceRegistryService {
	return h.svc
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrWorkspaceLayerNotFound),
		errors.Is(err, models.ErrWorkspaceRecordNotFound),
		errors.Is(err, models.ErrWorkspaceRasterNotFound):
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil, nil)
	case errors.Is(err, models.ErrMapAccessDenied):
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidWorkspaceRequest):
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil, nil)
	default:
		models.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Workspace layer/record operation failed", err.Error(), nil)
	}
}

// InspectDatasetPost handles POST /api/datasets/inspect.
func (h *Handler) InspectDatasetPost(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	var req models.InspectDatasetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
		return
	}
	dsID := strings.TrimSpace(req.FileID)
	if dsID == "" {
		dsID = strings.TrimSpace(req.DatasetID)
	}
	if dsID == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "file_id or dataset_id is required", nil, nil)
		return
	}
	res, err := h.svc.InspectDataset(r.Context(), userID, dsID)
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// InspectDatasetLayers handles GET /api/datasets/:id/layers (or /metadata).
func (h *Handler) InspectDatasetLayers(w http.ResponseWriter, r *http.Request, userID, datasetID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	res, err := h.svc.InspectDataset(r.Context(), userID, datasetID)
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// WorkspaceLayers handles GET/POST /api/workspaces/:id/layers.
func (h *Handler) WorkspaceLayers(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	switch r.Method {
	case http.MethodGet:
		layers, err := h.svc.ListWorkspaceLayers(r.Context(), userID, workspaceID)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"workspace_id": workspaceID,
			"layers":       layers,
			"count":        len(layers),
		})
	case http.MethodPost:
		var req models.AddDatasetLayersRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
			return
		}
		layers, ds, err := h.svc.AddDatasetLayers(r.Context(), userID, workspaceID, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"workspace_dataset": ds,
			"layers":            layers,
			"count":             len(layers),
		})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}

// WorkspaceLayerItem handles GET/PATCH/DELETE /api/workspaces/:id/layers/:layerId.
func (h *Handler) WorkspaceLayerItem(w http.ResponseWriter, r *http.Request, userID, workspaceID, layerID string) {
	switch r.Method {
	case http.MethodGet:
		lyr, err := h.svc.GetWorkspaceLayer(r.Context(), userID, workspaceID, layerID)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"layer": lyr})
	case http.MethodPatch, http.MethodPut:
		var req models.UpdateWorkspaceLayerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
			return
		}
		lyr, err := h.svc.UpdateWorkspaceLayer(r.Context(), userID, workspaceID, layerID, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"layer": lyr})
	case http.MethodDelete:
		if err := h.svc.DeleteWorkspaceLayer(r.Context(), userID, workspaceID, layerID); err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "layer_id": layerID})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}

// WorkspaceLayersOrder handles PATCH /api/workspaces/:id/layers/order.
func (h *Handler) WorkspaceLayersOrder(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	var req models.ReorderWorkspaceLayersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
		return
	}
	layers, err := h.svc.ReorderWorkspaceLayers(r.Context(), userID, workspaceID, req.LayerIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{"layers": layers})
}

// WorkspaceLayersVisibility handles PATCH /api/workspaces/:id/layers/visibility.
func (h *Handler) WorkspaceLayersVisibility(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	var req models.BatchWorkspaceVisibilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
		return
	}
	layers, err := h.svc.BatchUpdateVisibility(r.Context(), userID, workspaceID, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{"layers": layers})
}

// WorkspaceLayerFeatures handles GET /api/workspaces/:id/layers/:layerId/features.
func (h *Handler) WorkspaceLayerFeatures(w http.ResponseWriter, r *http.Request, userID, workspaceID, layerID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	q := r.URL.Query()
	var bbox []float64
	if raw := strings.TrimSpace(q.Get("bbox")); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) == 4 {
			arr := make([]float64, 4)
			ok := true
			for i, p := range parts {
				v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if err != nil {
					ok = false
					break
				}
				arr[i] = v
			}
			if ok {
				bbox = arr
			}
		}
	}
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	var fields []string
	if rawF := strings.TrimSpace(q.Get("fields")); rawF != "" {
		fields = strings.Split(rawF, ",")
	}
	res, err := h.svc.QueryLayerFeatures(
		r.Context(), userID, workspaceID, layerID, bbox,
		q.Get("search"), q.Get("attr_field"), q.Get("attr_value"),
		fields, page, pageSize,
	)
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// WorkspaceLayerFeatureItem handles GET /api/workspaces/:id/layers/:layerId/features/:featureId(/geometry).
func (h *Handler) WorkspaceLayerFeatureItem(w http.ResponseWriter, r *http.Request, userID, workspaceID, layerID, featureID string, geometryOnly bool) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	feat, err := h.svc.GetLayerFeature(r.Context(), userID, workspaceID, layerID, featureID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if geometryOnly {
		crs := "EPSG:4326"
		if feat.SRID > 0 {
			crs = "EPSG:" + strconv.Itoa(feat.SRID)
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"feature_id": feat.FeatureID,
			"geometry":   feat.Geometry,
			"crs":        crs,
		})
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{"feature": feat})
}

// WorkspaceRecords handles GET /api/workspaces/:id/records.
func (h *Handler) WorkspaceRecords(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	var cols []string
	if rawCols := strings.TrimSpace(q.Get("columns")); rawCols != "" {
		cols = strings.Split(rawCols, ",")
	}
	res, err := h.svc.ListWorkspaceRecords(r.Context(), userID, workspaceID, models.WorkspaceRecordsQuery{
		RecordType:    q.Get("record_type"),
		LayerID:       q.Get("layer_id"),
		Search:        q.Get("search"),
		Status:        q.Get("status"),
		Page:          page,
		PageSize:      pageSize,
		SortBy:        q.Get("sort_by"),
		SortDirection: q.Get("sort_direction"),
		Columns:       cols,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// WorkspaceRecordCounts handles GET /api/workspaces/:id/records/counts.
func (h *Handler) WorkspaceRecordCounts(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	counts, err := h.svc.GetWorkspaceRecordCounts(r.Context(), userID, workspaceID)
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{"counts": counts})
}

// WorkspaceRecordSchema handles GET /api/workspaces/:id/records/schema.
func (h *Handler) WorkspaceRecordSchema(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	q := r.URL.Query()
	cols, err := h.svc.GetWorkspaceRecordSchema(r.Context(), userID, workspaceID, q.Get("record_type"), q.Get("layer_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{"columns": cols})
}

// WorkspaceRecordItem handles GET/PATCH/DELETE /api/workspaces/:id/records/:recordId.
func (h *Handler) WorkspaceRecordItem(w http.ResponseWriter, r *http.Request, userID, workspaceID, recordID string) {
	switch r.Method {
	case http.MethodGet:
		rec, err := h.svc.GetWorkspaceRecord(r.Context(), userID, workspaceID, recordID)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"record": rec})
	case http.MethodPatch, http.MethodPut:
		var req models.UpdateWorkspaceRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
			return
		}
		rec, err := h.svc.UpdateWorkspaceRecord(r.Context(), userID, workspaceID, recordID, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"record": rec})
	case http.MethodDelete:
		if err := h.svc.DeleteWorkspaceRecord(r.Context(), userID, workspaceID, recordID); err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true, "record_id": recordID})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}

// WorkspaceRecordsExport handles POST /api/workspaces/:id/records/export.
func (h *Handler) WorkspaceRecordsExport(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	var req models.ExportWorkspaceRecordsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
		return
	}
	data, contentType, filename, err := h.svc.ExportWorkspaceRecords(r.Context(), userID, workspaceID, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// WorkspaceRasters handles GET /api/workspaces/:id/rasters.
func (h *Handler) WorkspaceRasters(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	rasters, err := h.svc.ListWorkspaceRasters(r.Context(), userID, workspaceID)
	if err != nil {
		writeErr(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"rasters": rasters,
		"count":   len(rasters),
	})
}

// WorkspaceRasterItem handles GET/PATCH /api/workspaces/:id/rasters/:rasterId(/metadata).
func (h *Handler) WorkspaceRasterItem(w http.ResponseWriter, r *http.Request, userID, workspaceID, rasterID string) {
	switch r.Method {
	case http.MethodGet:
		rst, err := h.svc.GetWorkspaceRaster(r.Context(), userID, workspaceID, rasterID)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"raster": rst})
	case http.MethodPatch, http.MethodPut:
		var req models.UpdateWorkspaceRasterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON payload", err.Error(), nil)
			return
		}
		rst, err := h.svc.UpdateWorkspaceRaster(r.Context(), userID, workspaceID, rasterID, req)
		if err != nil {
			writeErr(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"raster": rst})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}
