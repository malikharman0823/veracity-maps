package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type GISService struct {
	gisRepo     repository.GISRepo
	mapRepo     repository.MapRepo
	sessionRepo *repository.SessionRepository
}

func NewGISService(gisRepo repository.GISRepo, mapRepo repository.MapRepo, sessionRepo *repository.SessionRepository) *GISService {
	return &GISService{
		gisRepo:     gisRepo,
		mapRepo:     mapRepo,
		sessionRepo: sessionRepo,
	}
}

func (s *GISService) GetGISRepo() repository.GISRepo {
	return s.gisRepo
}

// checkMapAccess verifies that userID has at least read (requireEdit=false) or edit (requireEdit=true) access to mapID.
func (s *GISService) checkMapAccess(ctx context.Context, mapID, userID string, requireEdit bool) (*models.Map, error) {
	if mapID == "" || s.mapRepo == nil {
		return nil, nil
	}
	m, err := s.mapRepo.GetMap(ctx, mapID, userID)
	if err != nil {
		if errors.Is(err, models.ErrMapNotFound) {
			// Standalone dataset workspace (e.g. ds_...) not backed by a dbo.Maps row — authenticated user allowed.
			return nil, nil
		}
		return nil, err
	}
	if m != nil && requireEdit {
		if m.OwnerUserID != userID && m.Permission != models.PermissionEditor {
			return nil, models.ErrMapAccessDenied
		}
	}
	return m, nil
}

// checkLayerAccess verifies user permissions on the map owning layerID, and checks layer lock if requireUnlocked=true.
func (s *GISService) checkLayerAccess(ctx context.Context, layerID, userID string, requireEdit, requireUnlocked bool) (*models.MapLayer, error) {
	layer, err := s.gisRepo.GetLayer(ctx, layerID)
	if err != nil {
		return nil, err
	}
	if layer.MapID != "" && userID != "" {
		if _, err := s.checkMapAccess(ctx, layer.MapID, userID, requireEdit); err != nil {
			return nil, err
		}
	}
	if requireUnlocked && layer.IsLocked {
		return nil, models.ErrLayerLocked
	}
	return layer, nil
}

// ============================================================================
// MODULE A — ADVANCED LAYER MANAGER
// ============================================================================

func (s *GISService) CreateLayer(ctx context.Context, userID string, req models.CreateLayerRequest) (*models.MapLayer, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.MapID != "" && userID != "" {
		if _, err := s.checkMapAccess(ctx, req.MapID, userID, true); err != nil {
			return nil, err
		}
	}
	return s.gisRepo.CreateLayer(ctx, req)
}

func (s *GISService) GetLayers(ctx context.Context, mapID, userID string) ([]*models.MapLayer, []*models.LayerGroup, error) {
	if mapID != "" && userID != "" {
		if _, err := s.checkMapAccess(ctx, mapID, userID, false); err != nil {
			return nil, nil, err
		}
	}
	layers, err := s.gisRepo.GetLayers(ctx, mapID)
	if err != nil {
		return nil, nil, err
	}
	groups, err := s.gisRepo.GetLayerGroups(ctx, mapID)
	if err != nil {
		return nil, nil, err
	}
	if layers == nil {
		layers = []*models.MapLayer{}
	}
	for _, lyr := range layers {
		s.enrichRasterLayerFromSession(lyr)
	}
	if groups == nil {
		groups = []*models.LayerGroup{}
	}
	return layers, groups, nil
}

func (s *GISService) GetLayer(ctx context.Context, layerID, userID string) (*models.MapLayer, error) {
	lyr, err := s.checkLayerAccess(ctx, layerID, userID, false, false)
	if err != nil {
		return nil, err
	}
	s.enrichRasterLayerFromSession(lyr)
	return lyr, nil
}

func (s *GISService) enrichRasterLayerFromSession(layer *models.MapLayer) {
	if layer == nil || s.sessionRepo == nil || !strings.EqualFold(layer.LayerType, "raster") || layer.DatasetID == "" {
		return
	}
	ds, sessID, err := s.sessionRepo.FindDataset(layer.DatasetID)
	if err != nil || ds == nil {
		return
	}

	var liveBounds []float64
	if ds.MapLayer != nil {
		if b, ok := extractMapLibreBoundsFromAny(ds.MapLayer["bounds"]); ok {
			liveBounds = b
		}
	}
	if len(liveBounds) != 4 && ds.ExtractedMetadata != nil {
		if b, ok := extractMapLibreBoundsFromAny(ds.ExtractedMetadata["bounds"]); ok {
			liveBounds = b
		}
	}
	if len(liveBounds) != 4 && sessID != "" {
		if dsLayers, lErr := s.sessionRepo.ListLayers(sessID, ds.ID); lErr == nil {
			for _, dl := range dsLayers {
				if dl != nil && len(dl.Bounds) == 4 && dl.Bounds[2] > dl.Bounds[0] && dl.Bounds[3] > dl.Bounds[1] {
					liveBounds = append([]float64{}, dl.Bounds...)
					break
				}
			}
		}
	}

	if len(liveBounds) == 4 {
		layer.Bounds = liveBounds
	}
	if ds.ExtractedMetadata != nil {
		if crs, ok := ds.ExtractedMetadata["crs"].(string); ok && strings.TrimSpace(crs) != "" {
			layer.CRS = strings.TrimSpace(crs)
		}
	}

	isGeo := strings.EqualFold(ds.GeoreferenceStatus, "completed") || len(layer.Bounds) == 4
	if isGeo || ds.GeoreferenceStatus != "" || (ds.MapLayer != nil && ds.MapLayer["coordinates"] != nil) {
		if layer.Metadata == nil {
			layer.Metadata = make(map[string]interface{})
		}
		if ds.GeoreferenceStatus != "" {
			layer.Metadata["georeference_status"] = ds.GeoreferenceStatus
		} else if isGeo {
			layer.Metadata["georeference_status"] = "completed"
		}
		if isGeo {
			layer.Metadata["is_georeferenced"] = true
		}
		if ds.MapLayer != nil && ds.MapLayer["coordinates"] != nil {
			layer.Metadata["coordinates"] = ds.MapLayer["coordinates"]
		}
	}
}

func extractMapLibreBoundsFromAny(raw interface{}) ([]float64, bool) {
	if raw == nil {
		return nil, false
	}
	toFloat := func(v interface{}) (float64, bool) {
		switch n := v.(type) {
		case float64:
			return n, true
		case float32:
			return float64(n), true
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		default:
			return 0, false
		}
	}
	switch v := raw.(type) {
	case []float64:
		if len(v) == 4 && v[2] > v[0] && v[3] > v[1] {
			return append([]float64{}, v...), true
		}
	case []interface{}:
		if len(v) == 4 {
			out := make([]float64, 4)
			for i := 0; i < 4; i++ {
				f, ok := toFloat(v[i])
				if !ok {
					return nil, false
				}
				out[i] = f
			}
			if out[2] > out[0] && out[3] > out[1] {
				return out, true
			}
		}
	case map[string]float64:
		w, s, e, n := v["min_x"], v["min_y"], v["max_x"], v["max_y"]
		if w == 0 && s == 0 && e == 0 && n == 0 {
			w, s, e, n = v["west"], v["south"], v["east"], v["north"]
		}
		if e > w && n > s {
			return []float64{w, s, e, n}, true
		}
	case map[string]interface{}:
		w, _ := toFloat(v["min_x"])
		s, _ := toFloat(v["min_y"])
		e, _ := toFloat(v["max_x"])
		n, _ := toFloat(v["max_y"])
		if w == 0 && s == 0 && e == 0 && n == 0 {
			w, _ = toFloat(v["west"])
			s, _ = toFloat(v["south"])
			e, _ = toFloat(v["east"])
			n, _ = toFloat(v["north"])
		}
		if e > w && n > s {
			return []float64{w, s, e, n}, true
		}
	}
	return nil, false
}

func (s *GISService) UpdateLayer(ctx context.Context, layerID, userID string, req models.UpdateLayerRequest) (*models.MapLayer, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	layer, err := s.checkLayerAccess(ctx, layerID, userID, true, false)
	if err != nil {
		return nil, err
	}
	// If layer is currently locked and caller is not unlocking it, block other modifications
	if layer.IsLocked && (req.IsLocked == nil || *req.IsLocked) {
		if req.LayerName != nil || req.ParentGroupID != nil || req.Opacity != nil || req.MinZoom != nil || req.MaxZoom != nil {
			return nil, models.ErrLayerLocked
		}
	}
	return s.gisRepo.UpdateLayer(ctx, layerID, req)
}

func (s *GISService) UpdateLayerVisibility(ctx context.Context, layerID, userID string, visible bool) (*models.MapLayer, error) {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, false); err != nil {
		return nil, err
	}
	return s.gisRepo.UpdateLayer(ctx, layerID, models.UpdateLayerRequest{IsVisible: &visible})
}

func (s *GISService) UpdateLayerOpacity(ctx context.Context, layerID, userID string, opacity float64) (*models.MapLayer, error) {
	if opacity < 0 || opacity > 1 {
		return nil, fmt.Errorf("opacity must be between 0 and 1")
	}
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, true); err != nil {
		return nil, err
	}
	return s.gisRepo.UpdateLayer(ctx, layerID, models.UpdateLayerRequest{Opacity: &opacity})
}

func (s *GISService) DeleteLayer(ctx context.Context, layerID, userID string) error {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, true); err != nil {
		return err
	}
	return s.gisRepo.DeleteLayer(ctx, layerID)
}

func (s *GISService) ReorderLayers(ctx context.Context, mapID, userID string, req models.ReorderLayersRequest) ([]*models.MapLayer, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if mapID != "" && userID != "" {
		if _, err := s.checkMapAccess(ctx, mapID, userID, true); err != nil {
			return nil, err
		}
	}
	if err := s.gisRepo.ReorderLayers(ctx, mapID, req.LayerIDs); err != nil {
		return nil, err
	}
	layers, _ := s.gisRepo.GetLayers(ctx, mapID)
	return layers, nil
}

func (s *GISService) DuplicateLayer(ctx context.Context, layerID, userID string) (*models.MapLayer, error) {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, false); err != nil {
		return nil, err
	}
	return s.gisRepo.DuplicateLayer(ctx, layerID, userID)
}

func (s *GISService) CreateLayerGroup(ctx context.Context, userID string, req models.CreateLayerGroupRequest) (*models.LayerGroup, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.MapID != "" && userID != "" {
		if _, err := s.checkMapAccess(ctx, req.MapID, userID, true); err != nil {
			return nil, err
		}
	}
	return s.gisRepo.CreateLayerGroup(ctx, req)
}

func (s *GISService) UpdateLayerGroup(ctx context.Context, groupID, userID string, req models.UpdateLayerGroupRequest) (*models.LayerGroup, error) {
	grp, err := s.gisRepo.GetLayerGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if grp.MapID != "" && userID != "" {
		if _, err := s.checkMapAccess(ctx, grp.MapID, userID, true); err != nil {
			return nil, err
		}
	}
	return s.gisRepo.UpdateLayerGroup(ctx, groupID, req)
}

func (s *GISService) DeleteLayerGroup(ctx context.Context, groupID, userID string) error {
	grp, err := s.gisRepo.GetLayerGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if grp.MapID != "" && userID != "" {
		if _, err := s.checkMapAccess(ctx, grp.MapID, userID, true); err != nil {
			return err
		}
	}
	return s.gisRepo.DeleteLayerGroup(ctx, groupID)
}

// ============================================================================
// MODULE B — VECTOR EDITING
// ============================================================================

func (s *GISService) ValidateGeometry(geom models.GeoJSONGeometry, crs string) (*models.GeometryValidationResult, error) {
	return gis.ValidateGeoJSONGeometry(geom, crs)
}

func (s *GISService) CreateFeature(ctx context.Context, userID string, req models.CreateFeatureRequest) (*models.VectorFeature, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	layer, err := s.checkLayerAccess(ctx, req.LayerID, userID, true, true)
	if err != nil {
		return nil, err
	}
	crs := req.CRS
	if crs == "" {
		crs = layer.CRS
	}
	valRes, err := gis.ValidateGeoJSONGeometry(req.Geometry, crs)
	if err != nil || !valRes.IsValid {
		return nil, fmt.Errorf("%w: %s", models.ErrInvalidGeometry, valRes.Reason)
	}

	// Auto-register any new property keys into layer_fields if not already present
	existingFields, _ := s.gisRepo.GetLayerFields(ctx, req.LayerID)
	fieldMap := make(map[string]bool)
	for _, f := range existingFields {
		fieldMap[strings.ToLower(f.FieldName)] = true
	}
	for k, v := range req.Properties {
		if models.ValidateFieldName(k) && !fieldMap[strings.ToLower(k)] {
			fType := inferFieldType(v)
			_, _ = s.gisRepo.AddLayerField(ctx, req.LayerID, models.AddFieldRequest{
				FieldName: k,
				FieldType: fType,
			})
			fieldMap[strings.ToLower(k)] = true
		}
	}

	srid := gis.ParseSRID(crs)
	return s.gisRepo.CreateFeature(ctx, userID, req, valRes.WKT, srid, valRes.BBox)
}

func inferFieldType(v interface{}) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case int, int64:
		return "integer"
	case float64, float32:
		return "float"
	default:
		return "string"
	}
}

func (s *GISService) GetFeatures(ctx context.Context, layerID, userID string, bbox []float64, limit int) ([]*models.VectorFeature, error) {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, false, false); err != nil {
		return nil, err
	}
	feats, err := s.gisRepo.GetFeatures(ctx, layerID, bbox)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(feats) > limit {
		feats = feats[:limit]
	}
	if feats == nil {
		feats = []*models.VectorFeature{}
	}
	return feats, nil
}

func (s *GISService) GetFeature(ctx context.Context, featureID, userID string) (*models.VectorFeature, error) {
	feat, err := s.gisRepo.GetFeature(ctx, featureID)
	if err != nil {
		return nil, err
	}
	if _, err := s.checkLayerAccess(ctx, feat.LayerID, userID, false, false); err != nil {
		return nil, err
	}
	return feat, nil
}

func (s *GISService) UpdateFeature(ctx context.Context, featureID, userID string, req models.UpdateFeatureRequest) (*models.VectorFeature, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	feat, err := s.gisRepo.GetFeature(ctx, featureID)
	if err != nil {
		return nil, err
	}
	layer, err := s.checkLayerAccess(ctx, feat.LayerID, userID, true, true)
	if err != nil {
		return nil, err
	}

	var wkt string
	var srid int
	var bbox []float64
	if req.Geometry != nil {
		valRes, err := gis.ValidateGeoJSONGeometry(*req.Geometry, layer.CRS)
		if err != nil || !valRes.IsValid {
			return nil, fmt.Errorf("%w: %s", models.ErrInvalidGeometry, valRes.Reason)
		}
		wkt = valRes.WKT
		srid = gis.ParseSRID(layer.CRS)
		bbox = valRes.BBox
	}
	return s.gisRepo.UpdateFeature(ctx, featureID, userID, req.Geometry, wkt, srid, bbox, req.Properties)
}

func (s *GISService) DeleteFeature(ctx context.Context, featureID, userID string) error {
	feat, err := s.gisRepo.GetFeature(ctx, featureID)
	if err != nil {
		return err
	}
	if _, err := s.checkLayerAccess(ctx, feat.LayerID, userID, true, true); err != nil {
		return err
	}
	return s.gisRepo.DeleteFeature(ctx, featureID, userID)
}

func (s *GISService) GetFeatureHistory(ctx context.Context, featureID, userID string) ([]*models.FeatureHistoryRecord, error) {
	feat, err := s.gisRepo.GetFeature(ctx, featureID)
	if err != nil {
		return nil, err
	}
	if _, err := s.checkLayerAccess(ctx, feat.LayerID, userID, false, false); err != nil {
		return nil, err
	}
	return s.gisRepo.GetFeatureHistory(ctx, featureID)
}

func (s *GISService) SplitFeature(ctx context.Context, featureID, userID string, req models.SplitFeatureRequest) ([]*models.VectorFeature, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	orig, err := s.gisRepo.GetFeature(ctx, featureID)
	if err != nil {
		return nil, err
	}
	layer, err := s.checkLayerAccess(ctx, orig.LayerID, userID, true, true)
	if err != nil {
		return nil, err
	}

	parts, err := gis.SplitPolygonByLine(orig.Geometry, req.SplitGeometry)
	if err != nil {
		return nil, err
	}

	var created []*models.VectorFeature
	for idx, partGeom := range parts {
		props := make(map[string]interface{}, len(orig.Properties)+1)
		for k, v := range orig.Properties {
			props[k] = v
		}
		if name, ok := props["name"].(string); ok && name != "" {
			props["name"] = fmt.Sprintf("%s (Part %d)", name, idx+1)
		}
		nf, err := s.CreateFeature(ctx, userID, models.CreateFeatureRequest{
			LayerID:      orig.LayerID,
			Geometry:     partGeom,
			GeometryType: partGeom.Type,
			CRS:          layer.CRS,
			Properties:   props,
		})
		if err != nil {
			return nil, err
		}
		created = append(created, nf)
	}

	_ = s.gisRepo.DeleteFeature(ctx, featureID, userID)
	return created, nil
}

func (s *GISService) MergeFeatures(ctx context.Context, userID string, req models.MergeFeaturesRequest) (*models.VectorFeature, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var geoms []models.GeoJSONGeometry
	var firstFeat *models.VectorFeature
	mergedProps := make(map[string]interface{})

	for _, fid := range req.FeatureIDs {
		f, err := s.gisRepo.GetFeature(ctx, fid)
		if err != nil {
			return nil, err
		}
		if firstFeat == nil {
			firstFeat = f
			for k, v := range f.Properties {
				mergedProps[k] = v
			}
		} else if f.LayerID != firstFeat.LayerID {
			return nil, fmt.Errorf("all features to merge must belong to the same layer")
		}
		geoms = append(geoms, f.Geometry)
	}

	layer, err := s.checkLayerAccess(ctx, firstFeat.LayerID, userID, true, true)
	if err != nil {
		return nil, err
	}

	mergedGeom, err := gis.MergeGeometries(geoms)
	if err != nil {
		return nil, err
	}
	if req.OutputName != "" {
		mergedProps["name"] = req.OutputName
	}

	created, err := s.CreateFeature(ctx, userID, models.CreateFeatureRequest{
		LayerID:      firstFeat.LayerID,
		Geometry:     *mergedGeom,
		GeometryType: mergedGeom.Type,
		CRS:          layer.CRS,
		Properties:   mergedProps,
	})
	if err != nil {
		return nil, err
	}

	if !req.KeepOriginals {
		for _, fid := range req.FeatureIDs {
			_ = s.gisRepo.DeleteFeature(ctx, fid, userID)
		}
	}
	return created, nil
}

func (s *GISService) SnapGeometry(ctx context.Context, userID string, req models.SnapGeometryRequest) (*models.GeoJSONGeometry, bool, error) {
	if err := req.Validate(); err != nil {
		return nil, false, err
	}
	if _, err := s.checkLayerAccess(ctx, req.TargetLayerID, userID, false, false); err != nil {
		return nil, false, err
	}
	targetFeats, err := s.gisRepo.GetFeatures(ctx, req.TargetLayerID, nil)
	if err != nil {
		return nil, false, err
	}
	var targetGeoms []models.GeoJSONGeometry
	for _, tf := range targetFeats {
		targetGeoms = append(targetGeoms, tf.Geometry)
	}
	return gis.SnapGeometryToTargets(req.Geometry, targetGeoms, req.Tolerance)
}

// ============================================================================
// MODULE C — ATTRIBUTE TABLE
// ============================================================================

func (s *GISService) GetAttributeTable(ctx context.Context, layerID, userID string, req models.FilterAttributeRequest) (map[string]interface{}, error) {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, false, false); err != nil {
		return nil, err
	}
	req.Normalize()

	fields, err := s.gisRepo.GetLayerFields(ctx, layerID)
	if err != nil {
		return nil, err
	}
	if fields == nil {
		fields = []*models.LayerField{}
	}

	// Validate filter and sort field names against SQL injection
	for _, cond := range req.Filters {
		if !models.ValidateFieldName(cond.Field) {
			return nil, fmt.Errorf("%w: %s", models.ErrInvalidFieldName, cond.Field)
		}
	}
	if req.SortBy != "" && !models.ValidateFieldName(req.SortBy) {
		return nil, fmt.Errorf("%w: %s", models.ErrInvalidFieldName, req.SortBy)
	}

	allFeats, err := s.gisRepo.GetFeatures(ctx, layerID, nil)
	if err != nil {
		return nil, err
	}

	filtered := gis.SortAndFilterFeatures(allFeats, req.Filters, "AND", req.Search, req.SortBy, req.SortOrder)
	total := len(filtered)

	start := (req.Page - 1) * req.PageSize
	if start > total {
		start = total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}
	pageRows := filtered[start:end]
	if pageRows == nil {
		pageRows = []*models.VectorFeature{}
	}

	return map[string]interface{}{
		"layer_id":    layerID,
		"fields":      fields,
		"rows":        pageRows,
		"total":       total,
		"page":        req.Page,
		"page_size":   req.PageSize,
		"total_pages": (total + req.PageSize - 1) / req.PageSize,
	}, nil
}

func (s *GISService) FilterFeatures(ctx context.Context, layerID, userID string, req models.FilterAttributeRequest) (map[string]interface{}, error) {
	return s.GetAttributeTable(ctx, layerID, userID, req)
}

func (s *GISService) SortFeatures(ctx context.Context, layerID, userID string, req models.FilterAttributeRequest) (map[string]interface{}, error) {
	return s.GetAttributeTable(ctx, layerID, userID, req)
}

func (s *GISService) GetFeatureAttributes(ctx context.Context, featureID, userID string) (map[string]interface{}, error) {
	feat, err := s.GetFeature(ctx, featureID, userID)
	if err != nil {
		return nil, err
	}
	return feat.Properties, nil
}

func (s *GISService) UpdateFeatureAttributes(ctx context.Context, featureID, userID string, req models.UpdateAttributeRequest) (*models.VectorFeature, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	feat, err := s.gisRepo.GetFeature(ctx, featureID)
	if err != nil {
		return nil, err
	}
	if _, err := s.checkLayerAccess(ctx, feat.LayerID, userID, true, true); err != nil {
		return nil, err
	}
	return s.gisRepo.UpdateFeatureAttributes(ctx, featureID, userID, req.Values)
}

func (s *GISService) AddAttributeField(ctx context.Context, layerID, userID string, req models.AddFieldRequest) (*models.LayerField, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, true); err != nil {
		return nil, err
	}
	return s.gisRepo.AddLayerField(ctx, layerID, req)
}

func (s *GISService) DeleteAttributeField(ctx context.Context, layerID, fieldID, userID string) error {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, true); err != nil {
		return err
	}
	return s.gisRepo.DeleteLayerField(ctx, layerID, fieldID)
}

func (s *GISService) CalculateField(ctx context.Context, layerID, userID string, req models.CalculateFieldRequest) (int, error) {
	if err := req.Validate(); err != nil {
		return 0, err
	}
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, true); err != nil {
		return 0, err
	}

	// Ensure target field exists in schema; create it if missing
	fields, _ := s.gisRepo.GetLayerFields(ctx, layerID)
	foundField := false
	for _, f := range fields {
		if strings.EqualFold(f.FieldName, req.TargetField) {
			foundField = true
			break
		}
	}
	if !foundField {
		_, _ = s.gisRepo.AddLayerField(ctx, layerID, models.AddFieldRequest{
			FieldName: req.TargetField,
			FieldType: "string",
		})
	}

	feats, err := s.gisRepo.GetFeatures(ctx, layerID, nil)
	if err != nil {
		return 0, err
	}

	filterSet := make(map[string]bool)
	for _, id := range req.FeatureIDs {
		filterSet[id] = true
	}

	updatedCount := 0
	for _, feat := range feats {
		if len(filterSet) > 0 && !filterSet[feat.FeatureID] {
			continue
		}
		val, err := gis.EvaluateFieldExpression(req.Expression, feat)
		if err != nil {
			return updatedCount, err
		}
		_, err = s.gisRepo.UpdateFeatureAttributes(ctx, feat.FeatureID, userID, map[string]interface{}{
			req.TargetField: val,
		})
		if err == nil {
			updatedCount++
		}
	}
	return updatedCount, nil
}

func (s *GISService) ExportAttributes(ctx context.Context, layerID, userID, format string, featureIDs []string) ([]byte, string, error) {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, false, false); err != nil {
		return nil, "", err
	}
	feats, err := s.gisRepo.GetFeatures(ctx, layerID, nil)
	if err != nil {
		return nil, "", err
	}
	if len(featureIDs) > 0 {
		sel := make(map[string]bool)
		for _, id := range featureIDs {
			sel[id] = true
		}
		var sub []*models.VectorFeature
		for _, f := range feats {
			if sel[f.FeatureID] {
				sub = append(sub, f)
			}
		}
		feats = sub
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "geojson" || format == "json" {
		var gjFeats []map[string]interface{}
		for _, f := range feats {
			gjFeats = append(gjFeats, f.ToGeoJSONFeature())
		}
		fc := map[string]interface{}{
			"type":     "FeatureCollection",
			"features": gjFeats,
		}
		b, err := json.MarshalIndent(fc, "", "  ")
		return b, "application/geo+json", err
	}

	// Default CSV export
	fields, _ := s.gisRepo.GetLayerFields(ctx, layerID)
	var colNames []string
	colSeen := make(map[string]bool)
	for _, f := range fields {
		colNames = append(colNames, f.FieldName)
		colSeen[f.FieldName] = true
	}
	for _, feat := range feats {
		for k := range feat.Properties {
			if !colSeen[k] {
				colNames = append(colNames, k)
				colSeen[k] = true
			}
		}
	}
	sort.Strings(colNames)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := append([]string{"feature_id", "geometry_type", "wkt"}, colNames...)
	_ = writer.Write(header)

	for _, feat := range feats {
		row := []string{feat.FeatureID, feat.GeometryType, feat.GeometryWKT}
		for _, col := range colNames {
			v := feat.Properties[col]
			if v == nil {
				row = append(row, "")
			} else {
				row = append(row, fmt.Sprintf("%v", v))
			}
		}
		_ = writer.Write(row)
	}
	writer.Flush()
	return buf.Bytes(), "text/csv", nil
}

// ============================================================================
// MODULE D — SYMBOLOGY & STYLING ENGINE
// ============================================================================

func (s *GISService) ValidateStyleRules(req models.UpdateStyleRequest) (map[string]interface{}, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return gis.ValidateAndCompileStyle(req.StyleType, req.StyleConfig)
}

func (s *GISService) GenerateStyleConfiguration(req models.UpdateStyleRequest) (map[string]interface{}, error) {
	return s.ValidateStyleRules(req)
}

func (s *GISService) CreateLayerStyle(ctx context.Context, layerID, userID string, req models.UpdateStyleRequest) (*models.LayerStyle, error) {
	return s.UpdateLayerStyle(ctx, layerID, userID, req)
}

func (s *GISService) UpdateLayerStyle(ctx context.Context, layerID, userID string, req models.UpdateStyleRequest) (*models.LayerStyle, error) {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, true); err != nil {
		return nil, err
	}
	compiled, err := s.ValidateStyleRules(req)
	if err != nil {
		return nil, err
	}
	return s.gisRepo.UpsertLayerStyle(ctx, layerID, req.StyleType, req.StyleConfig, compiled)
}

func (s *GISService) GetLayerStyle(ctx context.Context, layerID, userID string) (*models.LayerStyle, error) {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, false, false); err != nil {
		return nil, err
	}
	st, err := s.gisRepo.GetLayerStyle(ctx, layerID)
	if err != nil {
		return nil, err
	}
	if st.MapLibreStyle == nil {
		compiled, cErr := gis.ValidateAndCompileStyle(st.StyleType, st.StyleConfig)
		if cErr == nil {
			st.MapLibreStyle = compiled
		}
	}
	return st, nil
}

func (s *GISService) DeleteLayerStyle(ctx context.Context, layerID, userID string) error {
	if _, err := s.checkLayerAccess(ctx, layerID, userID, true, true); err != nil {
		return err
	}
	return s.gisRepo.DeleteLayerStyle(ctx, layerID)
}

// ============================================================================
// MODULE E — CRS MANAGEMENT
// ============================================================================

func (s *GISService) GetSupportedCRS(query string) []models.CRSDefinition {
	return gis.GetSupportedCRSList(query)
}

func (s *GISService) GetCRSInformation(epsgCode string) (*models.CRSDefinition, error) {
	return gis.LookupCRS(epsgCode)
}

func (s *GISService) ValidateCRS(epsgCode string) (*models.CRSDefinition, error) {
	return gis.LookupCRS(epsgCode)
}

func (s *GISService) InspectDatasetCRS(sessionID, datasetID string) (map[string]interface{}, error) {
	ds, err := s.findDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}
	crsVal := ""
	if ds.ExtractedMetadata != nil {
		if c, ok := ds.ExtractedMetadata["crs"].(string); ok {
			crsVal = c
		} else if cPtr, ok := ds.ExtractedMetadata["crs"].(*string); ok && cPtr != nil {
			crsVal = *cPtr
		}
	}
	missingCRS := strings.TrimSpace(crsVal) == ""
	var crsDef *models.CRSDefinition
	if !missingCRS {
		crsDef, _ = gis.LookupCRS(crsVal)
	}
	return map[string]interface{}{
		"dataset_id":     ds.ID,
		"filename":       ds.Filename,
		"crs":            crsVal,
		"missing_crs":    missingCRS,
		"crs_definition": crsDef,
	}, nil
}

func (s *GISService) SetDatasetCRS(sessionID, datasetID, crsCode string) (*models.Dataset, error) {
	def, err := gis.LookupCRS(crsCode)
	if err != nil {
		return nil, err
	}
	ds, err := s.findDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}
	if ds.ExtractedMetadata == nil {
		ds.ExtractedMetadata = make(map[string]interface{})
	}
	ds.ExtractedMetadata["crs"] = def.EPSGCode
	ds.ExtractedMetadata["native_crs"] = def.EPSGCode
	ds.UpdatedAt = time.Now().UTC()
	if s.sessionRepo != nil && ds.SessionID != "" {
		_ = s.sessionRepo.UpdateDataset(ds.SessionID, ds)
	}
	return ds, nil
}

// TransformVectorCRS transforms all features of a vector layer or dataset between source and target CRS.
func (s *GISService) TransformVectorCRS(ctx context.Context, layerID, userID, sourceEPSG, targetEPSG, outputName string) (*models.MapLayer, error) {
	origLayer, err := s.checkLayerAccess(ctx, layerID, userID, true, false)
	if err != nil {
		return nil, err
	}
	srcCRS := sourceEPSG
	if srcCRS == "" {
		srcCRS = origLayer.CRS
	}
	if srcCRS == "" {
		return nil, fmt.Errorf("%w: source CRS is missing; please specify source_epsg", models.ErrInvalidCRS)
	}
	srcDef, err := gis.LookupCRS(srcCRS)
	if err != nil {
		return nil, err
	}
	dstDef, err := gis.LookupCRS(targetEPSG)
	if err != nil {
		return nil, err
	}

	newName := strings.TrimSpace(outputName)
	if newName == "" {
		newName = fmt.Sprintf("%s (%s)", origLayer.LayerName, dstDef.EPSGCode)
	}

	// Preserve original layer and create a new processed vector layer
	newLayer, err := s.CreateLayer(ctx, userID, models.CreateLayerRequest{
		MapID:         origLayer.MapID,
		DatasetID:     origLayer.DatasetID,
		LayerName:     newName,
		LayerType:     "vector",
		GeometryType:  origLayer.GeometryType,
		ParentGroupID: origLayer.ParentGroupID,
		CRS:           dstDef.EPSGCode,
	})
	if err != nil {
		return nil, err
	}

	// Copy attribute fields
	if fields, fErr := s.gisRepo.GetLayerFields(ctx, layerID); fErr == nil {
		for _, fld := range fields {
			nullable := fld.IsNullable
			_, _ = s.gisRepo.AddLayerField(ctx, newLayer.LayerID, models.AddFieldRequest{
				FieldName:    fld.FieldName,
				FieldType:    fld.FieldType,
				IsNullable:   &nullable,
				DefaultValue: fld.DefaultValue,
			})
		}
	}

	// Transform each feature's geometry using PROJ / TransformGeometryCRS
	feats, err := s.gisRepo.GetFeatures(ctx, layerID, nil)
	if err != nil {
		return nil, err
	}
	for _, feat := range feats {
		txGeom, txErr := gis.TransformGeometryCRS(feat.Geometry, srcDef.EPSGCode, dstDef.EPSGCode)
		if txErr != nil {
			return nil, txErr
		}
		valRes, vErr := gis.ValidateGeoJSONGeometry(*txGeom, dstDef.EPSGCode)
		if vErr != nil {
			return nil, vErr
		}
		_, err := s.gisRepo.CreateFeature(ctx, userID, models.CreateFeatureRequest{
			LayerID:      newLayer.LayerID,
			Geometry:     *txGeom,
			GeometryType: txGeom.Type,
			CRS:          dstDef.EPSGCode,
			Properties:   feat.Properties,
		}, valRes.WKT, dstDef.SRID, valRes.BBox)
		if err != nil {
			return nil, err
		}
	}

	return s.gisRepo.GetLayer(ctx, newLayer.LayerID)
}

// TransformRasterCRS reprojects a raster dataset into targetEPSG while preserving the original dataset.
func (s *GISService) TransformRasterCRS(ctx context.Context, sessionID string, req models.ReprojectDatasetRequest) (*models.Dataset, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	dstDef, err := gis.LookupCRS(req.TargetEPSG)
	if err != nil {
		return nil, err
	}

	origDS, err := s.findDataset(sessionID, req.DatasetID)
	if err != nil {
		return nil, err
	}

	srcCRS := req.SourceEPSG
	if srcCRS == "" && origDS.ExtractedMetadata != nil {
		if c, ok := origDS.ExtractedMetadata["crs"].(string); ok && c != "" {
			srcCRS = c
		} else if cPtr, ok := origDS.ExtractedMetadata["crs"].(*string); ok && cPtr != nil && *cPtr != "" {
			srcCRS = *cPtr
		}
	}
	if srcCRS == "" {
		return nil, fmt.Errorf("%w: dataset has no detected CRS; confirm and provide source_epsg first", models.ErrInvalidCRS)
	}
	srcDef, err := gis.LookupCRS(srcCRS)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	newID := fmt.Sprintf("ds_reproj_%x", now.UnixNano())[:20]
	outName := strings.TrimSpace(req.OutputName)
	if outName == "" {
		ext := filepath.Ext(origDS.Filename)
		base := strings.TrimSuffix(origDS.Filename, ext)
		outName = fmt.Sprintf("%s_%d%s", base, dstDef.SRID, ext)
	}

	// Copy and transform bounds
	newMeta := make(map[string]interface{})
	for k, v := range origDS.ExtractedMetadata {
		newMeta[k] = v
	}
	newMeta["crs"] = dstDef.EPSGCode
	newMeta["native_crs"] = dstDef.EPSGCode
	newMeta["source_crs"] = srcDef.EPSGCode
	newMeta["reprojected_from"] = origDS.ID

	if bMap, ok := origDS.ExtractedMetadata["bounds"].(map[string]interface{}); ok {
		minX, _ := bMap["min_x"].(float64)
		minY, _ := bMap["min_y"].(float64)
		maxX, _ := bMap["max_x"].(float64)
		maxY, _ := bMap["max_y"].(float64)
		txMinX, txMinY, err1 := gis.TransformCoordPair(minX, minY, srcDef.EPSGCode, dstDef.EPSGCode)
		txMaxX, txMaxY, err2 := gis.TransformCoordPair(maxX, maxY, srcDef.EPSGCode, dstDef.EPSGCode)
		if err1 == nil && err2 == nil {
			newMeta["projected_bounds"] = map[string]float64{
				"min_x": txMinX, "min_y": txMinY, "max_x": txMaxX, "max_y": txMaxY,
			}
		}
	} else if bMapF, ok := origDS.ExtractedMetadata["bounds"].(map[string]float64); ok {
		txMinX, txMinY, err1 := gis.TransformCoordPair(bMapF["min_x"], bMapF["min_y"], srcDef.EPSGCode, dstDef.EPSGCode)
		txMaxX, txMaxY, err2 := gis.TransformCoordPair(bMapF["max_x"], bMapF["max_y"], srcDef.EPSGCode, dstDef.EPSGCode)
		if err1 == nil && err2 == nil {
			newMeta["projected_bounds"] = map[string]float64{
				"min_x": txMinX, "min_y": txMinY, "max_x": txMaxX, "max_y": txMaxY,
			}
		}
	}

	newDS := &models.Dataset{
		ID:                 newID,
		SessionID:          origDS.SessionID,
		Filename:           outName,
		FileSize:           origDS.FileSize,
		ContentType:        origDS.ContentType,
		FileHash:           origDS.FileHash,
		StoragePath:        origDS.StoragePath,
		GeoreferencedPath:  origDS.GeoreferencedPath,
		ProcessedPath:      origDS.ProcessedPath,
		GeoreferenceStatus: origDS.GeoreferenceStatus,
		TileStatus:         origDS.TileStatus,
		TilePath:           origDS.TilePath,
		ExtractedMetadata:  newMeta,
		MapLayer:           origDS.MapLayer,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if s.sessionRepo != nil && origDS.SessionID != "" {
		_ = s.sessionRepo.InsertDataset(origDS.SessionID, newDS)
	}
	return newDS, nil
}

// ============================================================================
// MODULE F — DATASET BROWSER
// ============================================================================

func (s *GISService) findDataset(sessionID, datasetID string) (*models.Dataset, error) {
	if s.sessionRepo == nil {
		return nil, fmt.Errorf("dataset not found")
	}
	if sessionID != "" {
		if ds, err := s.sessionRepo.GetDataset(sessionID, datasetID); err == nil && ds != nil {
			return ds, nil
		}
	}
	if ds, _, err := s.sessionRepo.FindDataset(datasetID); err == nil && ds != nil {
		return ds, nil
	}
	return nil, fmt.Errorf("dataset not found: %s", datasetID)
}

func (s *GISService) InspectDataset(ctx context.Context, sessionID, datasetID string) (map[string]interface{}, error) {
	ds, err := s.findDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(ds.Filename))
	var layers []models.DatasetLayerInfo

	switch ext {
	case ".gpkg", ".shp", ".geojson", ".json", ".dxf", ".kml":
		if ds.StoragePath != "" {
			if _, statErr := os.Stat(ds.StoragePath); statErr == nil {
				inspected, _, iErr := gis.InspectVectorDatasetFile(ds.StoragePath)
				if iErr == nil && len(inspected) > 0 {
					layers = inspected
				}
			}
		}
		if len(layers) == 0 {
			layers = []models.DatasetLayerInfo{
				{
					DatasetLayerID: fmt.Sprintf("dsl_1_%s", ds.ID),
					DatasetID:      ds.ID,
					LayerName:      strings.TrimSuffix(ds.Filename, ext),
					GeometryType:   "Geometry",
					CRS:            "EPSG:4326",
					FeatureCount:   0,
					CreatedAt:      ds.CreatedAt,
				},
			}
		}
	default:
		crsStr := ""
		if ds.ExtractedMetadata != nil {
			if c, ok := ds.ExtractedMetadata["crs"].(string); ok {
				crsStr = c
			} else if cPtr, ok := ds.ExtractedMetadata["crs"].(*string); ok && cPtr != nil {
				crsStr = *cPtr
			}
		}
		layers = []models.DatasetLayerInfo{
			{
				DatasetLayerID: fmt.Sprintf("dsl_1_%s", ds.ID),
				DatasetID:      ds.ID,
				LayerName:      strings.TrimSuffix(ds.Filename, ext),
				GeometryType:   "Raster",
				CRS:            crsStr,
				FeatureCount:   1,
				CreatedAt:      ds.CreatedAt,
			},
		}
	}

	_ = s.gisRepo.SaveDatasetLayers(ctx, datasetID, layers)

	return map[string]interface{}{
		"dataset_id": ds.ID,
		"filename":   ds.Filename,
		"file_size":  ds.FileSize,
		"metadata":   ds.ExtractedMetadata,
		"layers":     layers,
	}, nil
}

func (s *GISService) GetDatasetMetadata(ctx context.Context, sessionID, datasetID string) (map[string]interface{}, error) {
	ds, err := s.findDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}
	layers, _ := s.gisRepo.GetDatasetLayers(ctx, datasetID)
	if len(layers) == 0 {
		res, iErr := s.InspectDataset(ctx, sessionID, datasetID)
		if iErr == nil {
			if l, ok := res["layers"].([]models.DatasetLayerInfo); ok {
				layers = l
			}
		}
	}
	refCount, _ := s.gisRepo.CountLayersByDatasetID(ctx, datasetID)
	return map[string]interface{}{
		"id":                  ds.ID,
		"filename":            ds.Filename,
		"file_size":           ds.FileSize,
		"content_type":        ds.ContentType,
		"georeference_status": ds.GeoreferenceStatus,
		"extracted_metadata":  ds.ExtractedMetadata,
		"layers":              layers,
		"referenced_layers":   refCount,
		"created_at":          ds.CreatedAt,
		"updated_at":          ds.UpdatedAt,
	}, nil
}

func (s *GISService) GetDatasetLayers(ctx context.Context, sessionID, datasetID string) ([]models.DatasetLayerInfo, error) {
	layers, err := s.gisRepo.GetDatasetLayers(ctx, datasetID)
	if err == nil && len(layers) > 0 {
		return layers, nil
	}
	res, err := s.InspectDataset(ctx, sessionID, datasetID)
	if err != nil {
		return nil, err
	}
	if l, ok := res["layers"].([]models.DatasetLayerInfo); ok {
		return l, nil
	}
	return []models.DatasetLayerInfo{}, nil
}

func (s *GISService) RenameDataset(sessionID, datasetID, newName string) (*models.Dataset, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return nil, fmt.Errorf("dataset name cannot be empty")
	}
	ds, err := s.findDataset(sessionID, datasetID)
	if err != nil {
		return nil, err
	}
	ds.Filename = newName
	ds.UpdatedAt = time.Now().UTC()
	if s.sessionRepo != nil && ds.SessionID != "" {
		_ = s.sessionRepo.UpdateDataset(ds.SessionID, ds)
	}
	return ds, nil
}

func (s *GISService) CheckDatasetDependencies(ctx context.Context, datasetID string) (int, error) {
	return s.gisRepo.CountLayersByDatasetID(ctx, datasetID)
}

// ============================================================================
// MODULE G — SPATIAL QUERY BUILDER
// ============================================================================

func (s *GISService) ValidateSpatialQuery(ctx context.Context, userID string, req models.SpatialQueryRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}
	if _, err := s.checkLayerAccess(ctx, req.LayerID, userID, false, false); err != nil {
		return err
	}
	// Verify all condition fields exist in the layer's field schema (or standard feature attributes)
	fields, _ := s.gisRepo.GetLayerFields(ctx, req.LayerID)
	if len(fields) > 0 {
		allowed := map[string]bool{
			"id": true, "feature_id": true, "name": true,
		}
		for _, f := range fields {
			allowed[strings.ToLower(f.FieldName)] = true
		}
		for _, cond := range req.Conditions {
			if !allowed[strings.ToLower(cond.Field)] {
				return fmt.Errorf("%w: field '%s' does not exist in layer schema", models.ErrInvalidFieldName, cond.Field)
			}
		}
	}
	if req.Geometry != nil {
		if val, err := gis.ValidateGeoJSONGeometry(*req.Geometry, "EPSG:4326"); err != nil || !val.IsValid {
			return fmt.Errorf("%w: invalid query geometry", models.ErrInvalidSpatialQuery)
		}
	}
	return nil
}

func (s *GISService) ExecuteSpatialQuery(ctx context.Context, userID string, req models.SpatialQueryRequest) (map[string]interface{}, error) {
	if err := s.ValidateSpatialQuery(ctx, userID, req); err != nil {
		return nil, err
	}

	candidates, err := s.gisRepo.GetFeatures(ctx, req.LayerID, nil)
	if err != nil {
		return nil, err
	}

	// Step 1: Attribute filtering
	matched := gis.SortAndFilterFeatures(candidates, req.Conditions, req.CombineOperator, "", "", "asc")

	// Step 2: Spatial filtering
	if req.SpatialOperator != "" {
		var queryGeoms []models.GeoJSONGeometry
		if req.Geometry != nil {
			queryGeoms = append(queryGeoms, *req.Geometry)
		} else if req.TargetLayerID != "" {
			if _, err := s.checkLayerAccess(ctx, req.TargetLayerID, userID, false, false); err != nil {
				return nil, err
			}
			targetFeats, _ := s.gisRepo.GetFeatures(ctx, req.TargetLayerID, nil)
			for _, tf := range targetFeats {
				queryGeoms = append(queryGeoms, tf.Geometry)
			}
		}

		if len(queryGeoms) > 0 {
			if req.SpatialOperator == "nearest" {
				type distPair struct {
					feat *models.VectorFeature
					dist float64
				}
				var pairs []distPair
				for _, cand := range matched {
					bestD := 1e18
					for _, qg := range queryGeoms {
						d := gis.GeometryMinDistance(cand.Geometry, qg)
						if d < bestD {
							bestD = d
						}
					}
					pairs = append(pairs, distPair{feat: cand, dist: bestD})
				}
				sort.Slice(pairs, func(i, j int) bool {
					return pairs[i].dist < pairs[j].dist
				})
				var nearest []*models.VectorFeature
				if len(pairs) > 0 {
					nearest = append(nearest, pairs[0].feat)
				}
				matched = nearest
			} else {
				var spatialMatched []*models.VectorFeature
				for _, cand := range matched {
					okAny := false
					for _, qg := range queryGeoms {
						if gis.EvaluateSpatialPredicate(cand.Geometry, qg, req.SpatialOperator, req.Distance, req.Unit) {
							okAny = true
							break
						}
					}
					if okAny {
						spatialMatched = append(spatialMatched, cand)
					}
				}
				matched = spatialMatched
			}
		} else {
			matched = []*models.VectorFeature{}
		}
	}

	total := len(matched)
	start := (req.Page - 1) * req.PageSize
	if start > total {
		start = total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}
	paged := matched[start:end]

	var geojsonFeatures []map[string]interface{}
	var featureIDs []string
	for _, f := range paged {
		geojsonFeatures = append(geojsonFeatures, f.ToGeoJSONFeature())
		featureIDs = append(featureIDs, f.FeatureID)
	}
	if geojsonFeatures == nil {
		geojsonFeatures = []map[string]interface{}{}
	}
	if featureIDs == nil {
		featureIDs = []string{}
	}

	return map[string]interface{}{
		"layer_id":    req.LayerID,
		"total":       total,
		"page":        req.Page,
		"page_size":   req.PageSize,
		"feature_ids": featureIDs,
		"features":    paged,
		"geojson": map[string]interface{}{
			"type":     "FeatureCollection",
			"features": geojsonFeatures,
		},
	}, nil
}

func (s *GISService) SaveSpatialQuery(ctx context.Context, userID string, req models.SpatialQueryRequest) (*models.SpatialQuery, error) {
	if err := s.ValidateSpatialQuery(ctx, userID, req); err != nil {
		return nil, err
	}
	return s.gisRepo.SaveSpatialQuery(ctx, userID, req)
}

func (s *GISService) GetSavedQueries(ctx context.Context, mapID, layerID, userID string) ([]*models.SpatialQuery, error) {
	return s.gisRepo.GetSavedQueries(ctx, mapID, layerID, userID)
}

func (s *GISService) DeleteSpatialQuery(ctx context.Context, queryID, userID string) error {
	return s.gisRepo.DeleteSpatialQuery(ctx, queryID, userID)
}
