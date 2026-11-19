package gis

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
	svc *services.GISService
}

func NewHandler(svc *services.GISService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) GetService() *services.GISService {
	return h.svc
}

func writeGISError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrMapAccessDenied):
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil, nil)
	case errors.Is(err, models.ErrLayerLocked):
		models.WriteError(w, http.StatusLocked, "LAYER_LOCKED", err.Error(), nil, nil)
	case errors.Is(err, models.ErrLayerNotFound),
		errors.Is(err, models.ErrLayerGroupNotFound),
		errors.Is(err, models.ErrFeatureNotFound),
		errors.Is(err, models.ErrFieldNotFound),
		errors.Is(err, models.ErrMapNotFound),
		errors.Is(err, models.ErrSpatialQueryNotFound):
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil, nil)
	case errors.Is(err, models.ErrDuplicateFieldName),
		errors.Is(err, models.ErrDatasetInUse):
		models.WriteError(w, http.StatusConflict, "CONFLICT", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidGeometry):
		models.WriteError(w, http.StatusBadRequest, "INVALID_GEOMETRY", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidFieldName):
		models.WriteError(w, http.StatusBadRequest, "INVALID_FIELD_NAME", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidStyleConfig):
		models.WriteError(w, http.StatusBadRequest, "INVALID_STYLE_CONFIG", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidCRS):
		models.WriteError(w, http.StatusBadRequest, "INVALID_CRS", err.Error(), nil, nil)
	case errors.Is(err, models.ErrInvalidSpatialQuery):
		models.WriteError(w, http.StatusBadRequest, "INVALID_SPATIAL_QUERY", err.Error(), nil, nil)
	default:
		models.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil, nil)
	}
}

func userIDFromUser(u *models.User) string {
	if u == nil {
		return ""
	}
	return u.ID
}

// ============================================================================
// MODULE A — LAYER MANAGER HANDLERS
// ============================================================================

// CreateLayer handles POST /maps/:mapId/layers
func (h *Handler) CreateLayer(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	var req models.CreateLayerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	req.MapID = mapID
	layer, err := h.svc.CreateLayer(r.Context(), userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, layer)
}

// GetLayers handles GET /maps/:mapId/layers
func (h *Handler) GetLayers(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	layers, groups, err := h.svc.GetLayers(r.Context(), mapID, userIDFromUser(user))
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"map_id":       mapID,
		"layers":       layers,
		"layer_groups": groups,
		"total":        len(layers),
	})
}

// GetLayer handles GET /layers/:layerId
func (h *Handler) GetLayer(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	layer, err := h.svc.GetLayer(r.Context(), layerID, userIDFromUser(user))
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, layer)
}

// UpdateLayer handles PATCH /layers/:layerId
func (h *Handler) UpdateLayer(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var req models.UpdateLayerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	layer, err := h.svc.UpdateLayer(r.Context(), layerID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, layer)
}

// DeleteLayer handles DELETE /layers/:layerId
func (h *Handler) DeleteLayer(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	if err := h.svc.DeleteLayer(r.Context(), layerID, userIDFromUser(user)); err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"deleted":  true,
		"layer_id": layerID,
	})
}

// ReorderLayers handles PUT /maps/:mapId/layers/order
func (h *Handler) ReorderLayers(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	var req models.ReorderLayersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	layers, err := h.svc.ReorderLayers(r.Context(), mapID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"map_id": mapID,
		"layers": layers,
	})
}

// UpdateLayerVisibility handles PATCH /layers/:layerId/visibility
func (h *Handler) UpdateLayerVisibility(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var body struct {
		IsVisible *bool `json:"is_visible"`
		Visible   *bool `json:"visible"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	vis := true
	if body.IsVisible != nil {
		vis = *body.IsVisible
	} else if body.Visible != nil {
		vis = *body.Visible
	}
	layer, err := h.svc.UpdateLayerVisibility(r.Context(), layerID, userIDFromUser(user), vis)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, layer)
}

// UpdateLayerOpacity handles PATCH /layers/:layerId/opacity
func (h *Handler) UpdateLayerOpacity(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var body struct {
		Opacity float64 `json:"opacity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	layer, err := h.svc.UpdateLayerOpacity(r.Context(), layerID, userIDFromUser(user), body.Opacity)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, layer)
}

// DuplicateLayer handles POST /layers/:layerId/duplicate
func (h *Handler) DuplicateLayer(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	dup, err := h.svc.DuplicateLayer(r.Context(), layerID, userIDFromUser(user))
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, dup)
}

// CreateLayerGroup handles POST /maps/:mapId/layer-groups
func (h *Handler) CreateLayerGroup(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	var req models.CreateLayerGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	req.MapID = mapID
	grp, err := h.svc.CreateLayerGroup(r.Context(), userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, grp)
}

// UpdateLayerGroup handles PATCH /layer-groups/:groupId
func (h *Handler) UpdateLayerGroup(w http.ResponseWriter, r *http.Request, user *models.User, groupID string) {
	var req models.UpdateLayerGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	grp, err := h.svc.UpdateLayerGroup(r.Context(), groupID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, grp)
}

// DeleteLayerGroup handles DELETE /layer-groups/:groupId
func (h *Handler) DeleteLayerGroup(w http.ResponseWriter, r *http.Request, user *models.User, groupID string) {
	if err := h.svc.DeleteLayerGroup(r.Context(), groupID, userIDFromUser(user)); err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"deleted":  true,
		"group_id": groupID,
	})
}

// ============================================================================
// MODULE B — VECTOR EDITING HANDLERS
// ============================================================================

// CreateFeature handles POST /layers/:layerId/features
func (h *Handler) CreateFeature(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var req models.CreateFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	req.LayerID = layerID
	feat, err := h.svc.CreateFeature(r.Context(), userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, feat)
}

// GetFeatures handles GET /layers/:layerId/features
func (h *Handler) GetFeatures(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var bbox []float64
	if bboxStr := strings.TrimSpace(r.URL.Query().Get("bbox")); bboxStr != "" {
		parts := strings.Split(bboxStr, ",")
		if len(parts) == 4 {
			var parsed []float64
			for _, p := range parts {
				if v, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
					parsed = append(parsed, v)
				}
			}
			if len(parsed) == 4 {
				bbox = parsed
			}
		}
	}
	limit := 0
	if limStr := r.URL.Query().Get("limit"); limStr != "" {
		if l, err := strconv.Atoi(limStr); err == nil && l > 0 {
			limit = l
		}
	}

	feats, err := h.svc.GetFeatures(r.Context(), layerID, userIDFromUser(user), bbox, limit)
	if err != nil {
		writeGISError(w, err)
		return
	}

	var geojsonFeats []map[string]interface{}
	for _, f := range feats {
		geojsonFeats = append(geojsonFeats, f.ToGeoJSONFeature())
	}
	if geojsonFeats == nil {
		geojsonFeats = []map[string]interface{}{}
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"type":     "FeatureCollection",
		"layer_id": layerID,
		"total":    len(feats),
		"features": geojsonFeats,
		"items":    feats,
	})
}

// GetFeature handles GET /features/:featureId
func (h *Handler) GetFeature(w http.ResponseWriter, r *http.Request, user *models.User, featureID string) {
	feat, err := h.svc.GetFeature(r.Context(), featureID, userIDFromUser(user))
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, feat)
}

// UpdateFeature handles PATCH /features/:featureId
func (h *Handler) UpdateFeature(w http.ResponseWriter, r *http.Request, user *models.User, featureID string) {
	var req models.UpdateFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	feat, err := h.svc.UpdateFeature(r.Context(), featureID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, feat)
}

// DeleteFeature handles DELETE /features/:featureId
func (h *Handler) DeleteFeature(w http.ResponseWriter, r *http.Request, user *models.User, featureID string) {
	if err := h.svc.DeleteFeature(r.Context(), featureID, userIDFromUser(user)); err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"deleted":    true,
		"feature_id": featureID,
	})
}

// GetFeatureHistory handles GET /features/:featureId/history
func (h *Handler) GetFeatureHistory(w http.ResponseWriter, r *http.Request, user *models.User, featureID string) {
	records, err := h.svc.GetFeatureHistory(r.Context(), featureID, userIDFromUser(user))
	if err != nil {
		writeGISError(w, err)
		return
	}
	if records == nil {
		records = []*models.FeatureHistoryRecord{}
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"feature_id": featureID,
		"history":    records,
	})
}

// SplitFeature handles POST /features/:featureId/split
func (h *Handler) SplitFeature(w http.ResponseWriter, r *http.Request, user *models.User, featureID string) {
	var req models.SplitFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	req.FeatureID = featureID
	parts, err := h.svc.SplitFeature(r.Context(), featureID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"original_feature_id": featureID,
		"features":            parts,
		"count":               len(parts),
	})
}

// MergeFeatures handles POST /features/merge
func (h *Handler) MergeFeatures(w http.ResponseWriter, r *http.Request, user *models.User) {
	var req models.MergeFeaturesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	merged, err := h.svc.MergeFeatures(r.Context(), userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, merged)
}

// ValidateGeometry handles POST /features/validate
func (h *Handler) ValidateGeometry(w http.ResponseWriter, r *http.Request) {
	var req models.ValidateGeometryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	res, _ := h.svc.ValidateGeometry(req.Geometry, req.CRS)
	models.WriteJSON(w, http.StatusOK, res)
}

// SnapGeometry handles POST /features/snap
func (h *Handler) SnapGeometry(w http.ResponseWriter, r *http.Request, user *models.User) {
	var req models.SnapGeometryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	snappedGeom, snapped, err := h.svc.SnapGeometry(r.Context(), userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"snapped":  snapped,
		"geometry": snappedGeom,
	})
}

// ============================================================================
// MODULE C — ATTRIBUTE TABLE HANDLERS
// ============================================================================

// GetAttributeTable handles GET /layers/:layerId/attributes
func (h *Handler) GetAttributeTable(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	req := models.FilterAttributeRequest{
		Search:    q.Get("search"),
		Page:      page,
		PageSize:  pageSize,
		SortBy:    q.Get("sort_by"),
		SortOrder: q.Get("sort_order"),
	}
	res, err := h.svc.GetAttributeTable(r.Context(), layerID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// FilterAttributes handles POST /layers/:layerId/attributes/filter
func (h *Handler) FilterAttributes(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var req models.FilterAttributeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	res, err := h.svc.FilterFeatures(r.Context(), layerID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// UpdateFeatureAttributes handles PATCH /features/:featureId/attributes
func (h *Handler) UpdateFeatureAttributes(w http.ResponseWriter, r *http.Request, user *models.User, featureID string) {
	var req models.UpdateAttributeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	feat, err := h.svc.UpdateFeatureAttributes(r.Context(), featureID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, feat)
}

// AddAttributeField handles POST /layers/:layerId/fields
func (h *Handler) AddAttributeField(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var req models.AddFieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	fld, err := h.svc.AddAttributeField(r.Context(), layerID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, fld)
}

// DeleteAttributeField handles DELETE /layers/:layerId/fields/:fieldId
func (h *Handler) DeleteAttributeField(w http.ResponseWriter, r *http.Request, user *models.User, layerID, fieldID string) {
	if err := h.svc.DeleteAttributeField(r.Context(), layerID, fieldID, userIDFromUser(user)); err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"deleted":  true,
		"layer_id": layerID,
		"field_id": fieldID,
	})
}

// CalculateField handles POST /layers/:layerId/attributes/calculate
func (h *Handler) CalculateField(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var req models.CalculateFieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	updatedCount, err := h.svc.CalculateField(r.Context(), layerID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"layer_id":      layerID,
		"target_field":  req.TargetField,
		"updated_count": updatedCount,
	})
}

// ExportAttributes handles GET or POST /layers/:layerId/attributes/export
func (h *Handler) ExportAttributes(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	format := r.URL.Query().Get("format")
	var featureIDs []string
	if r.Method == http.MethodPost {
		var body struct {
			Format     string   `json:"format"`
			FeatureIDs []string `json:"feature_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Format != "" {
			format = body.Format
		}
		featureIDs = body.FeatureIDs
	}
	if format == "" {
		format = "csv"
	}

	data, contentType, err := h.svc.ExportAttributes(r.Context(), layerID, userIDFromUser(user), format, featureIDs)
	if err != nil {
		writeGISError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ============================================================================
// MODULE D — SYMBOLOGY & STYLING HANDLERS
// ============================================================================

// GetLayerStyle handles GET /layers/:layerId/style
func (h *Handler) GetLayerStyle(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	st, err := h.svc.GetLayerStyle(r.Context(), layerID, userIDFromUser(user))
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, st)
}

// UpdateLayerStyle handles PUT /layers/:layerId/style
func (h *Handler) UpdateLayerStyle(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	var req models.UpdateStyleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	st, err := h.svc.UpdateLayerStyle(r.Context(), layerID, userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, st)
}

// DeleteLayerStyle handles DELETE /layers/:layerId/style
func (h *Handler) DeleteLayerStyle(w http.ResponseWriter, r *http.Request, user *models.User, layerID string) {
	if err := h.svc.DeleteLayerStyle(r.Context(), layerID, userIDFromUser(user)); err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"deleted":  true,
		"layer_id": layerID,
	})
}

// ValidateLayerStyle handles POST /layers/:layerId/style/validate
func (h *Handler) ValidateLayerStyle(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateStyleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	compiled, err := h.svc.ValidateStyleRules(req)
	if err != nil {
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"is_valid": false,
			"error":    err.Error(),
		})
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"is_valid":       true,
		"maplibre_style": compiled,
	})
}

// ============================================================================
// MODULE E — CRS MANAGEMENT HANDLERS
// ============================================================================

// ListSupportedCRS handles GET /crs
func (h *Handler) ListSupportedCRS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		q = r.URL.Query().Get("search")
	}
	list := h.svc.GetSupportedCRS(q)
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"crs_list": list,
		"total":    len(list),
	})
}

// GetCRSInfo handles GET /crs/:epsgCode
func (h *Handler) GetCRSInfo(w http.ResponseWriter, r *http.Request, epsgCode string) {
	def, err := h.svc.GetCRSInformation(epsgCode)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, def)
}

// ReprojectDataset handles POST /datasets/:datasetId/reproject
func (h *Handler) ReprojectDataset(w http.ResponseWriter, r *http.Request, user *models.User, sessionID, datasetID string) {
	var body struct {
		LayerID    string `json:"layer_id"`
		SourceEPSG string `json:"source_epsg"`
		TargetEPSG string `json:"target_epsg"`
		OutputName string `json:"output_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}

	// If layer_id is provided or datasetID is actually a vector layer ID, transform vector layer
	if body.LayerID != "" || strings.HasPrefix(datasetID, "lyr_") {
		targetLayerID := body.LayerID
		if targetLayerID == "" {
			targetLayerID = datasetID
		}
		newLayer, err := h.svc.TransformVectorCRS(r.Context(), targetLayerID, userIDFromUser(user), body.SourceEPSG, body.TargetEPSG, body.OutputName)
		if err != nil {
			writeGISError(w, err)
			return
		}
		models.WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"type":        "vector_layer",
			"source_epsg": body.SourceEPSG,
			"target_epsg": newLayer.CRS,
			"layer":       newLayer,
		})
		return
	}

	newDS, err := h.svc.TransformRasterCRS(r.Context(), sessionID, models.ReprojectDatasetRequest{
		DatasetID:  datasetID,
		SourceEPSG: body.SourceEPSG,
		TargetEPSG: body.TargetEPSG,
		OutputName: body.OutputName,
	})
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"type":        "raster_dataset",
		"source_epsg": newDS.ExtractedMetadata["source_crs"],
		"target_epsg": newDS.ExtractedMetadata["crs"],
		"dataset":     newDS,
	})
}

// SetDatasetCRS handles POST /datasets/:datasetId/crs
func (h *Handler) SetDatasetCRS(w http.ResponseWriter, r *http.Request, sessionID, datasetID string) {
	var body struct {
		CRS string `json:"crs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	ds, err := h.svc.SetDatasetCRS(sessionID, datasetID, body.CRS)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, ds)
}

// ============================================================================
// MODULE F — DATASET BROWSER HANDLERS
// ============================================================================

// InspectDataset handles POST /datasets/:datasetId/inspect
func (h *Handler) InspectDataset(w http.ResponseWriter, r *http.Request, sessionID, datasetID string) {
	res, err := h.svc.InspectDataset(r.Context(), sessionID, datasetID)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// GetDatasetMetadata handles GET /datasets/:datasetId/metadata
func (h *Handler) GetDatasetMetadata(w http.ResponseWriter, r *http.Request, sessionID, datasetID string) {
	res, err := h.svc.GetDatasetMetadata(r.Context(), sessionID, datasetID)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// GetDatasetLayers handles GET /datasets/:datasetId/layers (for GeoPackage/multi-layer preview)
func (h *Handler) GetDatasetLayers(w http.ResponseWriter, r *http.Request, sessionID, datasetID string) {
	layers, err := h.svc.GetDatasetLayers(r.Context(), sessionID, datasetID)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"dataset_id": datasetID,
		"layers":     layers,
		"total":      len(layers),
	})
}

// RenameDataset handles PATCH /datasets/:datasetId
func (h *Handler) RenameDataset(w http.ResponseWriter, r *http.Request, sessionID, datasetID string) {
	var body struct {
		Name     string `json:"name"`
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	newName := body.Name
	if newName == "" {
		newName = body.Filename
	}
	ds, err := h.svc.RenameDataset(sessionID, datasetID, newName)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, ds)
}

// ============================================================================
// MODULE G — SPATIAL QUERY BUILDER HANDLERS
// ============================================================================

// ValidateSpatialQuery handles POST /spatial-queries/validate
func (h *Handler) ValidateSpatialQuery(w http.ResponseWriter, r *http.Request, user *models.User) {
	var req models.SpatialQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	if err := h.svc.ValidateSpatialQuery(r.Context(), userIDFromUser(user), req); err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"is_valid": true,
	})
}

// ExecuteSpatialQuery handles POST /spatial-queries/execute
func (h *Handler) ExecuteSpatialQuery(w http.ResponseWriter, r *http.Request, user *models.User) {
	var req models.SpatialQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	res, err := h.svc.ExecuteSpatialQuery(r.Context(), userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, res)
}

// SaveSpatialQuery handles POST /spatial-queries
func (h *Handler) SaveSpatialQuery(w http.ResponseWriter, r *http.Request, user *models.User) {
	var req models.SpatialQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body", nil, nil)
		return
	}
	sq, err := h.svc.SaveSpatialQuery(r.Context(), userIDFromUser(user), req)
	if err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, sq)
}

// GetSavedQueries handles GET /spatial-queries
func (h *Handler) GetSavedQueries(w http.ResponseWriter, r *http.Request, user *models.User) {
	mapID := r.URL.Query().Get("map_id")
	layerID := r.URL.Query().Get("layer_id")
	list, err := h.svc.GetSavedQueries(r.Context(), mapID, layerID, userIDFromUser(user))
	if err != nil {
		writeGISError(w, err)
		return
	}
	if list == nil {
		list = []*models.SpatialQuery{}
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"queries": list,
		"total":   len(list),
	})
}

// DeleteSpatialQuery handles DELETE /spatial-queries/:queryId
func (h *Handler) DeleteSpatialQuery(w http.ResponseWriter, r *http.Request, user *models.User, queryID string) {
	if err := h.svc.DeleteSpatialQuery(r.Context(), queryID, userIDFromUser(user)); err != nil {
		writeGISError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"deleted":  true,
		"query_id": queryID,
	})
}
