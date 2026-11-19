package baselayers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	svc *services.BaseLayerService
}

func NewHandler(svc *services.BaseLayerService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetService() *services.BaseLayerService {
	return h.svc
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrBaseLayerNotFound):
		models.WriteError(w, http.StatusNotFound, "BASE_LAYER_NOT_FOUND", err.Error(), nil, nil)
	case errors.Is(err, models.ErrBaseLayerAccessDenied), errors.Is(err, models.ErrMapAccessDenied):
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil, nil)
	case errors.Is(err, models.ErrBaseLayerNotGeoref):
		models.WriteError(w, http.StatusUnprocessableEntity, "BASE_LAYER_NOT_GEOREFERENCED", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidMapViewType):
		models.WriteError(w, http.StatusBadRequest, "INVALID_MAP_VIEW", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidBaseLayerRequest):
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil, nil)
	default:
		models.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Base layer operation failed", err.Error(), nil)
	}
}

// ListOrCreateBaseLayers handles GET /api/base-layers and POST /api/base-layers.
func (h *Handler) ListOrCreateBaseLayers(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		items, worldMap, err := h.svc.ListBaseLayers(r.Context(), userID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"default_base_layer": worldMap,
			"base_layers":        items,
			"count":              len(items),
		})
	case http.MethodPost:
		var req models.CreateBaseLayerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request payload", err.Error(), nil)
			return
		}
		bl, err := h.svc.RegisterBaseLayer(r.Context(), userID, req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"base_layer": bl,
		})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}

// BaseLayerItem handles GET/PUT/DELETE /api/base-layers/:id.
func (h *Handler) BaseLayerItem(w http.ResponseWriter, r *http.Request, userID, id string) {
	switch r.Method {
	case http.MethodGet:
		bl, err := h.svc.GetBaseLayer(r.Context(), userID, id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"base_layer": bl,
		})
	case http.MethodPut, http.MethodPatch:
		var req models.UpdateBaseLayerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request payload", err.Error(), nil)
			return
		}
		bl, err := h.svc.UpdateBaseLayer(r.Context(), userID, id, req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"base_layer": bl,
		})
	case http.MethodDelete:
		if err := h.svc.DeleteBaseLayer(r.Context(), userID, id); err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"deleted": true,
			"id":      id,
		})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}

// BaseLayerPreview handles GET /api/base-layers/:id/preview.
func (h *Handler) BaseLayerPreview(w http.ResponseWriter, r *http.Request, userID, id string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	preview, err := h.svc.GetBaseLayerPreview(r.Context(), userID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"preview": preview,
	})
}

// ValidateBaseLayer handles POST /api/base-layers/:id/validate.
func (h *Handler) ValidateBaseLayer(w http.ResponseWriter, r *http.Request, userID, id string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	res, err := h.svc.ValidateBaseLayer(r.Context(), userID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"validation": res,
	})
}

// WorkspaceBaseLayer handles GET/PUT /api/workspaces/:id/base-layer.
func (h *Handler) WorkspaceBaseLayer(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	switch r.Method {
	case http.MethodGet:
		wbl, err := h.svc.GetWorkspaceBaseLayer(r.Context(), userID, workspaceID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"workspace_base_layer": wbl,
		})
	case http.MethodPut:
		var req models.SelectWorkspaceBaseLayerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request payload", err.Error(), nil)
			return
		}
		wbl, err := h.svc.SelectWorkspaceBaseLayer(r.Context(), userID, workspaceID, req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"workspace_base_layer": wbl,
		})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}

// WorkspaceMapView handles GET/PUT /api/workspaces/:id/map-view.
func (h *Handler) WorkspaceMapView(w http.ResponseWriter, r *http.Request, userID, workspaceID string) {
	switch r.Method {
	case http.MethodGet:
		prefs, err := h.svc.GetWorkspaceMapView(r.Context(), userID, workspaceID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"map_view": prefs,
		})
	case http.MethodPut:
		var req models.UpdateMapViewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request payload", err.Error(), nil)
			return
		}
		prefs, err := h.svc.UpdateWorkspaceMapView(r.Context(), userID, workspaceID, req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"map_view": prefs,
		})
	default:
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
	}
}

// ListMapViews handles GET /api/map-views.
func (h *Handler) ListMapViews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	views := h.svc.ListMapViews()
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"map_views": views,
		"count":     len(views),
	})
}

// GetMapView handles GET /api/map-views/:type.
func (h *Handler) GetMapView(w http.ResponseWriter, r *http.Request, viewType string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	view, err := h.svc.GetMapView(viewType)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"map_view": view,
	})
}
