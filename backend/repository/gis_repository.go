package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// GISRepo defines the persistent storage interface for Phase 1 GIS modules A-G.
type GISRepo interface {
	// Module A — Layer Manager
	CreateLayer(ctx context.Context, req models.CreateLayerRequest) (*models.MapLayer, error)
	GetLayers(ctx context.Context, mapID string) ([]*models.MapLayer, error)
	GetLayer(ctx context.Context, layerID string) (*models.MapLayer, error)
	UpdateLayer(ctx context.Context, layerID string, req models.UpdateLayerRequest) (*models.MapLayer, error)
	DeleteLayer(ctx context.Context, layerID string) error
	ReorderLayers(ctx context.Context, mapID string, layerIDs []string) error
	DuplicateLayer(ctx context.Context, layerID, userID string) (*models.MapLayer, error)
	CreateLayerGroup(ctx context.Context, req models.CreateLayerGroupRequest) (*models.LayerGroup, error)
	GetLayerGroups(ctx context.Context, mapID string) ([]*models.LayerGroup, error)
	GetLayerGroup(ctx context.Context, groupID string) (*models.LayerGroup, error)
	UpdateLayerGroup(ctx context.Context, groupID string, req models.UpdateLayerGroupRequest) (*models.LayerGroup, error)
	DeleteLayerGroup(ctx context.Context, groupID string) error
	CountLayersByDatasetID(ctx context.Context, datasetID string) (int, error)

	// Module B — Vector Editing
	CreateFeature(ctx context.Context, userID string, req models.CreateFeatureRequest, wkt string, srid int, bbox []float64) (*models.VectorFeature, error)
	CreateFeatureBatch(ctx context.Context, userID string, items []models.BatchFeatureItem) ([]*models.VectorFeature, error)
	GetFeatures(ctx context.Context, layerID string, bbox []float64) ([]*models.VectorFeature, error)
	GetFeature(ctx context.Context, featureID string) (*models.VectorFeature, error)
	UpdateFeature(ctx context.Context, featureID, userID string, geom *models.GeoJSONGeometry, wkt string, srid int, bbox []float64, props map[string]interface{}) (*models.VectorFeature, error)
	DeleteFeature(ctx context.Context, featureID, userID string) error
	GetFeatureHistory(ctx context.Context, featureID string) ([]*models.FeatureHistoryRecord, error)

	// Module C — Attribute Table
	GetLayerFields(ctx context.Context, layerID string) ([]*models.LayerField, error)
	AddLayerField(ctx context.Context, layerID string, req models.AddFieldRequest) (*models.LayerField, error)
	DeleteLayerField(ctx context.Context, layerID, fieldID string) error
	UpdateFeatureAttributes(ctx context.Context, featureID, userID string, values map[string]interface{}) (*models.VectorFeature, error)

	// Module D — Symbology & Styling
	UpsertLayerStyle(ctx context.Context, layerID, styleType string, styleConfig, mapLibreStyle map[string]interface{}) (*models.LayerStyle, error)
	GetLayerStyle(ctx context.Context, layerID string) (*models.LayerStyle, error)
	DeleteLayerStyle(ctx context.Context, layerID string) error

	// Module F — Dataset Layers
	SaveDatasetLayers(ctx context.Context, datasetID string, layers []models.DatasetLayerInfo) error
	GetDatasetLayers(ctx context.Context, datasetID string) ([]models.DatasetLayerInfo, error)

	// Module G — Spatial Queries
	SaveSpatialQuery(ctx context.Context, userID string, req models.SpatialQueryRequest) (*models.SpatialQuery, error)
	GetSavedQueries(ctx context.Context, mapID, layerID, userID string) ([]*models.SpatialQuery, error)
	DeleteSpatialQuery(ctx context.Context, queryID, userID string) error
}

func generateGISID(prefix, seed string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", prefix, seed, time.Now().UnixNano()))
	return fmt.Sprintf("%s_%x", prefix, sum)[:20]
}

// ============================================================================
// 1. IN-MEMORY GIS REPOSITORY
// ============================================================================

type MemoryGISRepository struct {
	mu            sync.RWMutex
	layers        map[string]*models.MapLayer
	groups        map[string]*models.LayerGroup
	fields        map[string][]*models.LayerField
	features      map[string]*models.VectorFeature
	history       map[string][]*models.FeatureHistoryRecord
	styles        map[string]*models.LayerStyle
	datasetLayers map[string][]models.DatasetLayerInfo
	queries       map[string]*models.SpatialQuery
}

func NewMemoryGISRepository() *MemoryGISRepository {
	return &MemoryGISRepository{
		layers:        make(map[string]*models.MapLayer),
		groups:        make(map[string]*models.LayerGroup),
		fields:        make(map[string][]*models.LayerField),
		features:      make(map[string]*models.VectorFeature),
		history:       make(map[string][]*models.FeatureHistoryRecord),
		styles:        make(map[string]*models.LayerStyle),
		datasetLayers: make(map[string][]models.DatasetLayerInfo),
		queries:       make(map[string]*models.SpatialQuery),
	}
}

func (r *MemoryGISRepository) CreateLayer(ctx context.Context, req models.CreateLayerRequest) (*models.MapLayer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	layerID := generateGISID("lyr", req.MapID+req.LayerName)

	order := 0
	for _, l := range r.layers {
		if l.MapID == req.MapID && l.DisplayOrder >= order {
			order = l.DisplayOrder + 1
		}
	}
	if req.DisplayOrder != nil {
		order = *req.DisplayOrder
	}

	vis := true
	if req.IsVisible != nil {
		vis = *req.IsVisible
	}
	locked := false
	if req.IsLocked != nil {
		locked = *req.IsLocked
	}
	opacity := 1.0
	if req.Opacity != nil {
		opacity = *req.Opacity
	}
	minZ := 0.0
	if req.MinZoom != nil {
		minZ = *req.MinZoom
	}
	maxZ := 24.0
	if req.MaxZoom != nil {
		maxZ = *req.MaxZoom
	}
	crs := req.CRS
	if crs == "" {
		crs = "EPSG:4326"
	}

	layer := &models.MapLayer{
		LayerID:        layerID,
		MapID:          req.MapID,
		DatasetID:      req.DatasetID,
		DatasetLayerID: req.DatasetLayerID,
		LayerName:      req.LayerName,
		LayerType:      req.LayerType,
		GeometryType:   req.GeometryType,
		ParentGroupID:  req.ParentGroupID,
		DisplayOrder:   order,
		IsVisible:      vis,
		IsLocked:       locked,
		Opacity:        opacity,
		MinZoom:        minZ,
		MaxZoom:        maxZ,
		CRS:            crs,
		Bounds:         append([]float64{}, req.Bounds...),
		Metadata:       req.Metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	r.layers[layerID] = layer
	cp := *layer
	return &cp, nil
}

func (r *MemoryGISRepository) GetLayers(ctx context.Context, mapID string) ([]*models.MapLayer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.MapLayer
	for _, l := range r.layers {
		if l.MapID == mapID {
			cp := *l
			cnt := 0
			for _, f := range r.features {
				if f.LayerID == l.LayerID {
					cnt++
				}
			}
			cp.FeatureCount = cnt
			if st, ok := r.styles[l.LayerID]; ok {
				stCopy := *st
				cp.Style = &stCopy
			}
			list = append(list, &cp)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].DisplayOrder == list[j].DisplayOrder {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].DisplayOrder < list[j].DisplayOrder
	})
	return list, nil
}

func (r *MemoryGISRepository) GetLayer(ctx context.Context, layerID string) (*models.MapLayer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	l, ok := r.layers[layerID]
	if !ok {
		return nil, models.ErrLayerNotFound
	}
	cp := *l
	cnt := 0
	for _, f := range r.features {
		if f.LayerID == l.LayerID {
			cnt++
		}
	}
	cp.FeatureCount = cnt
	if st, ok := r.styles[l.LayerID]; ok {
		stCopy := *st
		cp.Style = &stCopy
	}
	return &cp, nil
}

func (r *MemoryGISRepository) UpdateLayer(ctx context.Context, layerID string, req models.UpdateLayerRequest) (*models.MapLayer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	l, ok := r.layers[layerID]
	if !ok {
		return nil, models.ErrLayerNotFound
	}
	if req.LayerName != nil {
		l.LayerName = strings.TrimSpace(*req.LayerName)
	}
	if req.ParentGroupID != nil {
		l.ParentGroupID = strings.TrimSpace(*req.ParentGroupID)
	}
	if req.DisplayOrder != nil {
		l.DisplayOrder = *req.DisplayOrder
	}
	if req.IsVisible != nil {
		l.IsVisible = *req.IsVisible
	}
	if req.IsLocked != nil {
		l.IsLocked = *req.IsLocked
	}
	if req.Opacity != nil {
		l.Opacity = *req.Opacity
	}
	if req.MinZoom != nil {
		l.MinZoom = *req.MinZoom
	}
	if req.MaxZoom != nil {
		l.MaxZoom = *req.MaxZoom
	}
	l.UpdatedAt = time.Now().UTC()

	cp := *l
	return &cp, nil
}

func (r *MemoryGISRepository) DeleteLayer(ctx context.Context, layerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.layers[layerID]; !ok {
		return models.ErrLayerNotFound
	}
	delete(r.layers, layerID)
	delete(r.fields, layerID)
	delete(r.styles, layerID)
	for fid, f := range r.features {
		if f.LayerID == layerID {
			delete(r.features, fid)
			delete(r.history, fid)
		}
	}
	return nil
}

func (r *MemoryGISRepository) ReorderLayers(ctx context.Context, mapID string, layerIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	for idx, lid := range layerIDs {
		if l, ok := r.layers[lid]; ok && l.MapID == mapID {
			l.DisplayOrder = idx
			l.UpdatedAt = now
		}
	}
	return nil
}

func (r *MemoryGISRepository) DuplicateLayer(ctx context.Context, layerID, userID string) (*models.MapLayer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	orig, ok := r.layers[layerID]
	if !ok {
		return nil, models.ErrLayerNotFound
	}
	now := time.Now().UTC()
	newID := generateGISID("lyr", orig.MapID+orig.LayerName+"copy")

	dup := *orig
	dup.LayerID = newID
	dup.LayerName = orig.LayerName + " (Copy)"
	dup.DisplayOrder = orig.DisplayOrder + 1
	dup.IsLocked = false
	dup.CreatedAt = now
	dup.UpdatedAt = now
	r.layers[newID] = &dup

	// Copy fields
	if flds, ok := r.fields[layerID]; ok {
		var copiedFields []*models.LayerField
		for _, f := range flds {
			fc := *f
			fc.FieldID = generateGISID("fld", newID+f.FieldName)
			fc.LayerID = newID
			copiedFields = append(copiedFields, &fc)
		}
		r.fields[newID] = copiedFields
	}

	// Copy style
	if st, ok := r.styles[layerID]; ok {
		sc := *st
		sc.StyleID = generateGISID("sty", newID)
		sc.LayerID = newID
		r.styles[newID] = &sc
	}

	// Copy features
	for _, feat := range r.features {
		if feat.LayerID == layerID {
			fc := *feat
			fc.FeatureID = generateGISID("feat", newID)
			fc.LayerID = newID
			fc.CreatedBy = userID
			fc.UpdatedBy = userID
			fc.CreatedAt = now
			fc.UpdatedAt = now
			propsCopy := make(map[string]interface{}, len(feat.Properties))
			for k, v := range feat.Properties {
				propsCopy[k] = v
			}
			fc.Properties = propsCopy
			r.features[fc.FeatureID] = &fc
		}
	}

	cp := dup
	return &cp, nil
}

func (r *MemoryGISRepository) CreateLayerGroup(ctx context.Context, req models.CreateLayerGroupRequest) (*models.LayerGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	gid := generateGISID("grp", req.MapID+req.GroupName)
	order := 0
	for _, g := range r.groups {
		if g.MapID == req.MapID && g.DisplayOrder >= order {
			order = g.DisplayOrder + 1
		}
	}
	if req.DisplayOrder != nil {
		order = *req.DisplayOrder
	}
	grp := &models.LayerGroup{
		GroupID:       gid,
		MapID:         req.MapID,
		ParentGroupID: req.ParentGroupID,
		GroupName:     req.GroupName,
		DisplayOrder:  order,
		IsExpanded:    true,
		IsVisible:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	r.groups[gid] = grp
	cp := *grp
	return &cp, nil
}

func (r *MemoryGISRepository) GetLayerGroups(ctx context.Context, mapID string) ([]*models.LayerGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.LayerGroup
	for _, g := range r.groups {
		if g.MapID == mapID {
			cp := *g
			list = append(list, &cp)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].DisplayOrder < list[j].DisplayOrder
	})
	return list, nil
}

func (r *MemoryGISRepository) GetLayerGroup(ctx context.Context, groupID string) (*models.LayerGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	g, ok := r.groups[groupID]
	if !ok {
		return nil, models.ErrLayerGroupNotFound
	}
	cp := *g
	return &cp, nil
}

func (r *MemoryGISRepository) UpdateLayerGroup(ctx context.Context, groupID string, req models.UpdateLayerGroupRequest) (*models.LayerGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	g, ok := r.groups[groupID]
	if !ok {
		return nil, models.ErrLayerGroupNotFound
	}
	if req.GroupName != nil && strings.TrimSpace(*req.GroupName) != "" {
		g.GroupName = strings.TrimSpace(*req.GroupName)
	}
	if req.ParentGroupID != nil {
		g.ParentGroupID = strings.TrimSpace(*req.ParentGroupID)
	}
	if req.DisplayOrder != nil {
		g.DisplayOrder = *req.DisplayOrder
	}
	if req.IsExpanded != nil {
		g.IsExpanded = *req.IsExpanded
	}
	if req.IsVisible != nil {
		g.IsVisible = *req.IsVisible
	}
	g.UpdatedAt = time.Now().UTC()
	cp := *g
	return &cp, nil
}

func (r *MemoryGISRepository) DeleteLayerGroup(ctx context.Context, groupID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.groups[groupID]; !ok {
		return models.ErrLayerGroupNotFound
	}
	// Ungroup child layers and subgroups safely
	for _, l := range r.layers {
		if l.ParentGroupID == groupID {
			l.ParentGroupID = ""
		}
	}
	for _, g := range r.groups {
		if g.ParentGroupID == groupID {
			g.ParentGroupID = ""
		}
	}
	delete(r.groups, groupID)
	return nil
}

func (r *MemoryGISRepository) CountLayersByDatasetID(ctx context.Context, datasetID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, l := range r.layers {
		if l.DatasetID == datasetID {
			count++
		}
	}
	return count, nil
}

func (r *MemoryGISRepository) CreateFeature(ctx context.Context, userID string, req models.CreateFeatureRequest, wkt string, srid int, bbox []float64) (*models.VectorFeature, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	fid := generateGISID("feat", req.LayerID+wkt)
	props := make(map[string]interface{})
	for k, v := range req.Properties {
		props[k] = v
	}

	feat := &models.VectorFeature{
		FeatureID:    fid,
		LayerID:      req.LayerID,
		Geometry:     req.Geometry,
		GeometryWKT:  wkt,
		GeometryType: req.Geometry.Type,
		SRID:         srid,
		BBox:         append([]float64{}, bbox...),
		Properties:   props,
		CreatedBy:    userID,
		UpdatedBy:    userID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	r.features[fid] = feat

	r.recordHistoryLocked(fid, req.LayerID, userID, "create", nil, feat.ToGeoJSONFeature())
	cp := *feat
	return &cp, nil
}

func (r *MemoryGISRepository) CreateFeatureBatch(_ context.Context, userID string, items []models.BatchFeatureItem) ([]*models.VectorFeature, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	out := make([]*models.VectorFeature, 0, len(items))
	for _, item := range items {
		fid := generateGISID("feat", item.Request.LayerID+item.WKT)
		props := make(map[string]interface{})
		for k, v := range item.Request.Properties {
			props[k] = v
		}
		feat := &models.VectorFeature{
			FeatureID:    fid,
			LayerID:      item.Request.LayerID,
			Geometry:     item.Request.Geometry,
			GeometryWKT:  item.WKT,
			GeometryType: item.Request.Geometry.Type,
			SRID:         item.SRID,
			BBox:         append([]float64{}, item.BBox...),
			Properties:   props,
			CreatedBy:    userID,
			UpdatedBy:    userID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		r.features[fid] = feat
		r.recordHistoryLocked(fid, item.Request.LayerID, userID, "create", nil, feat.ToGeoJSONFeature())
		cp := *feat
		out = append(out, &cp)
	}
	return out, nil
}

func (r *MemoryGISRepository) GetFeatures(ctx context.Context, layerID string, bbox []float64) ([]*models.VectorFeature, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.VectorFeature
	for _, f := range r.features {
		if f.LayerID != layerID {
			continue
		}
		if len(bbox) == 4 && len(f.BBox) == 4 {
			if f.BBox[2] < bbox[0] || f.BBox[0] > bbox[2] || f.BBox[3] < bbox[1] || f.BBox[1] > bbox[3] {
				continue
			}
		}
		cp := *f
		propsCopy := make(map[string]interface{}, len(f.Properties))
		for k, v := range f.Properties {
			propsCopy[k] = v
		}
		cp.Properties = propsCopy
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})
	return list, nil
}

func (r *MemoryGISRepository) GetFeature(ctx context.Context, featureID string) (*models.VectorFeature, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	f, ok := r.features[featureID]
	if !ok {
		return nil, models.ErrFeatureNotFound
	}
	cp := *f
	propsCopy := make(map[string]interface{}, len(f.Properties))
	for k, v := range f.Properties {
		propsCopy[k] = v
	}
	cp.Properties = propsCopy
	return &cp, nil
}

func (r *MemoryGISRepository) UpdateFeature(ctx context.Context, featureID, userID string, geom *models.GeoJSONGeometry, wkt string, srid int, bbox []float64, props map[string]interface{}) (*models.VectorFeature, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.features[featureID]
	if !ok {
		return nil, models.ErrFeatureNotFound
	}
	oldSnap := f.ToGeoJSONFeature()

	if geom != nil {
		f.Geometry = *geom
		f.GeometryWKT = wkt
		f.GeometryType = geom.Type
		if srid > 0 {
			f.SRID = srid
		}
		f.BBox = append([]float64{}, bbox...)
	}
	if props != nil {
		if f.Properties == nil {
			f.Properties = make(map[string]interface{})
		}
		for k, v := range props {
			f.Properties[k] = v
		}
	}
	f.UpdatedBy = userID
	f.UpdatedAt = time.Now().UTC()

	r.recordHistoryLocked(featureID, f.LayerID, userID, "update", oldSnap, f.ToGeoJSONFeature())
	cp := *f
	return &cp, nil
}

func (r *MemoryGISRepository) DeleteFeature(ctx context.Context, featureID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.features[featureID]
	if !ok {
		return models.ErrFeatureNotFound
	}
	oldSnap := f.ToGeoJSONFeature()
	r.recordHistoryLocked(featureID, f.LayerID, userID, "delete", oldSnap, nil)
	delete(r.features, featureID)
	return nil
}

func (r *MemoryGISRepository) GetFeatureHistory(ctx context.Context, featureID string) ([]*models.FeatureHistoryRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	records := r.history[featureID]
	var out []*models.FeatureHistoryRecord
	for _, rec := range records {
		cp := *rec
		out = append(out, &cp)
	}
	return out, nil
}

func (r *MemoryGISRepository) recordHistoryLocked(featureID, layerID, userID, op string, oldData, newData map[string]interface{}) {
	rec := &models.FeatureHistoryRecord{
		HistoryID: generateGISID("hst", featureID+op),
		FeatureID: featureID,
		LayerID:   layerID,
		UserID:    userID,
		Operation: op,
		OldData:   oldData,
		NewData:   newData,
		CreatedAt: time.Now().UTC(),
	}
	r.history[featureID] = append([]*models.FeatureHistoryRecord{rec}, r.history[featureID]...)
}

func (r *MemoryGISRepository) GetLayerFields(ctx context.Context, layerID string) ([]*models.LayerField, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []*models.LayerField
	for _, f := range r.fields[layerID] {
		cp := *f
		out = append(out, &cp)
	}
	return out, nil
}

func (r *MemoryGISRepository) AddLayerField(ctx context.Context, layerID string, req models.AddFieldRequest) (*models.LayerField, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.fields[layerID] {
		if strings.EqualFold(existing.FieldName, req.FieldName) {
			return nil, models.ErrDuplicateFieldName
		}
	}
	nullable := true
	if req.IsNullable != nil {
		nullable = *req.IsNullable
	}
	fld := &models.LayerField{
		FieldID:      generateGISID("fld", layerID+req.FieldName),
		LayerID:      layerID,
		FieldName:    req.FieldName,
		FieldType:    req.FieldType,
		IsNullable:   nullable,
		DefaultValue: req.DefaultValue,
		DisplayOrder: len(r.fields[layerID]),
		CreatedAt:    time.Now().UTC(),
	}
	r.fields[layerID] = append(r.fields[layerID], fld)

	// Populate default value across existing features if provided
	for _, feat := range r.features {
		if feat.LayerID == layerID {
			if feat.Properties == nil {
				feat.Properties = make(map[string]interface{})
			}
			if _, exists := feat.Properties[req.FieldName]; !exists {
				if req.DefaultValue != "" {
					feat.Properties[req.FieldName] = req.DefaultValue
				} else {
					feat.Properties[req.FieldName] = nil
				}
			}
		}
	}

	cp := *fld
	return &cp, nil
}

func (r *MemoryGISRepository) DeleteLayerField(ctx context.Context, layerID, fieldID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	flds := r.fields[layerID]
	var targetName string
	var updated []*models.LayerField
	for _, f := range flds {
		if f.FieldID == fieldID || strings.EqualFold(f.FieldName, fieldID) {
			targetName = f.FieldName
			continue
		}
		updated = append(updated, f)
	}
	if targetName == "" {
		return models.ErrFieldNotFound
	}
	r.fields[layerID] = updated

	for _, feat := range r.features {
		if feat.LayerID == layerID && feat.Properties != nil {
			delete(feat.Properties, targetName)
		}
	}
	return nil
}

func (r *MemoryGISRepository) UpdateFeatureAttributes(ctx context.Context, featureID, userID string, values map[string]interface{}) (*models.VectorFeature, error) {
	return r.UpdateFeature(ctx, featureID, userID, nil, "", 0, nil, values)
}

func (r *MemoryGISRepository) UpsertLayerStyle(ctx context.Context, layerID, styleType string, styleConfig, mapLibreStyle map[string]interface{}) (*models.LayerStyle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	st, exists := r.styles[layerID]
	if !exists {
		st = &models.LayerStyle{
			StyleID: generateGISID("sty", layerID),
			LayerID: layerID,
		}
	}
	st.StyleType = styleType
	st.StyleConfig = styleConfig
	st.MapLibreStyle = mapLibreStyle
	st.UpdatedAt = now
	r.styles[layerID] = st

	cp := *st
	return &cp, nil
}

func (r *MemoryGISRepository) GetLayerStyle(ctx context.Context, layerID string) (*models.LayerStyle, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if st, ok := r.styles[layerID]; ok {
		cp := *st
		return &cp, nil
	}
	// Return default single symbol style if none customized yet
	return &models.LayerStyle{
		StyleID:   "default_" + layerID,
		LayerID:   layerID,
		StyleType: "single",
		StyleConfig: map[string]interface{}{
			"fill_color":   "#3ecf8e",
			"stroke_color": "#24b47e",
			"point_color":  "#3ecf8e",
			"stroke_width": 2.0,
			"point_radius": 6.0,
			"opacity":      0.85,
		},
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (r *MemoryGISRepository) DeleteLayerStyle(ctx context.Context, layerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.styles, layerID)
	return nil
}

func (r *MemoryGISRepository) SaveDatasetLayers(ctx context.Context, datasetID string, layers []models.DatasetLayerInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var stored []models.DatasetLayerInfo
	for _, l := range layers {
		cp := l
		cp.DatasetID = datasetID
		if cp.DatasetLayerID == "" {
			cp.DatasetLayerID = generateGISID("dsl", datasetID+l.LayerName)
		}
		stored = append(stored, cp)
	}
	r.datasetLayers[datasetID] = stored
	return nil
}

func (r *MemoryGISRepository) GetDatasetLayers(ctx context.Context, datasetID string) ([]models.DatasetLayerInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]models.DatasetLayerInfo{}, r.datasetLayers[datasetID]...), nil
}

func (r *MemoryGISRepository) SaveSpatialQuery(ctx context.Context, userID string, req models.SpatialQueryRequest) (*models.SpatialQuery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	qid := generateGISID("sq", userID+req.LayerID+req.QueryName)
	qName := strings.TrimSpace(req.QueryName)
	if qName == "" {
		qName = "Saved Spatial Query"
	}
	sq := &models.SpatialQuery{
		QueryID:     qid,
		MapID:       req.MapID,
		LayerID:     req.LayerID,
		QueryName:   qName,
		QueryConfig: req,
		CreatedBy:   userID,
		CreatedAt:   now,
	}
	r.queries[qid] = sq
	cp := *sq
	return &cp, nil
}

func (r *MemoryGISRepository) GetSavedQueries(ctx context.Context, mapID, layerID, userID string) ([]*models.SpatialQuery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.SpatialQuery
	for _, sq := range r.queries {
		if mapID != "" && sq.MapID != mapID {
			continue
		}
		if layerID != "" && sq.LayerID != layerID {
			continue
		}
		cp := *sq
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list, nil
}

func (r *MemoryGISRepository) DeleteSpatialQuery(ctx context.Context, queryID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.queries[queryID]; !ok {
		return models.ErrSpatialQueryNotFound
	}
	delete(r.queries, queryID)
	return nil
}

// ============================================================================
// 2. MICROSOFT SQL SERVER 2022 GIS REPOSITORY
// ============================================================================

type MSSQLGISRepository struct {
	db *sql.DB
}

func NewMSSQLGISRepository(db *sql.DB) *MSSQLGISRepository {
	return &MSSQLGISRepository{db: db}
}

func (r *MSSQLGISRepository) CreateLayer(ctx context.Context, req models.CreateLayerRequest) (*models.MapLayer, error) {
	now := time.Now().UTC()
	layerID := generateGISID("lyr", req.MapID+req.LayerName)

	var maxOrder sql.NullInt64
	_ = r.db.QueryRowContext(ctx, `SELECT MAX(display_order) FROM dbo.map_layers WHERE map_id = @p1;`, req.MapID).Scan(&maxOrder)
	order := 0
	if maxOrder.Valid {
		order = int(maxOrder.Int64) + 1
	}
	if req.DisplayOrder != nil {
		order = *req.DisplayOrder
	}

	vis := true
	if req.IsVisible != nil {
		vis = *req.IsVisible
	}
	locked := false
	if req.IsLocked != nil {
		locked = *req.IsLocked
	}
	opacity := 1.0
	if req.Opacity != nil {
		opacity = *req.Opacity
	}
	minZ := 0.0
	if req.MinZoom != nil {
		minZ = *req.MinZoom
	}
	maxZ := 24.0
	if req.MaxZoom != nil {
		maxZ = *req.MaxZoom
	}
	crs := req.CRS
	if crs == "" {
		crs = "EPSG:4326"
	}

	var boundsJSON, metaJSON string
	if len(req.Bounds) > 0 {
		b, _ := json.Marshal(req.Bounds)
		boundsJSON = string(b)
	}
	if len(req.Metadata) > 0 {
		b, _ := json.Marshal(req.Metadata)
		metaJSON = string(b)
	}

	query := `
		INSERT INTO dbo.map_layers (
			layer_id, map_id, dataset_id, dataset_layer_id, layer_name, layer_type,
			geometry_type, parent_group_id, display_order, is_visible, is_locked,
			opacity, min_zoom, max_zoom, crs, bounds_json, metadata_json, created_at, updated_at
		) VALUES (
			@p1, @p2, NULLIF(@p3, ''), NULLIF(@p4, ''), @p5, @p6,
			NULLIF(@p7, ''), NULLIF(@p8, ''), @p9, @p10, @p11,
			@p12, @p13, @p14, @p15, NULLIF(@p16, ''), NULLIF(@p17, ''), @p18, @p19
		);
	`
	_, err := r.db.ExecContext(ctx, query,
		layerID, req.MapID, req.DatasetID, req.DatasetLayerID, req.LayerName, req.LayerType,
		req.GeometryType, req.ParentGroupID, order, vis, locked,
		opacity, minZ, maxZ, crs, boundsJSON, metaJSON, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert map_layer: %w", err)
	}

	return r.GetLayer(ctx, layerID)
}

func (r *MSSQLGISRepository) GetLayers(ctx context.Context, mapID string) ([]*models.MapLayer, error) {
	query := `
		SELECT l.layer_id, l.map_id, ISNULL(l.dataset_id, ''), ISNULL(l.dataset_layer_id, ''),
		       l.layer_name, l.layer_type, ISNULL(l.geometry_type, ''), ISNULL(l.parent_group_id, ''),
		       l.display_order, l.is_visible, l.is_locked, l.opacity, l.min_zoom, l.max_zoom,
		       ISNULL(l.crs, 'EPSG:4326'), ISNULL(l.bounds_json, ''), ISNULL(l.metadata_json, ''),
		       (SELECT COUNT(1) FROM dbo.features f WHERE f.layer_id = l.layer_id) AS feature_count,
		       l.created_at, l.updated_at
		FROM dbo.map_layers l
		WHERE l.map_id = @p1
		ORDER BY l.display_order ASC, l.created_at ASC;
	`
	rows, err := r.db.QueryContext(ctx, query, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.MapLayer
	for rows.Next() {
		l, err := scanMapLayer(rows)
		if err != nil {
			return nil, err
		}
		if st, sErr := r.GetLayerStyle(ctx, l.LayerID); sErr == nil {
			l.Style = st
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

func (r *MSSQLGISRepository) GetLayer(ctx context.Context, layerID string) (*models.MapLayer, error) {
	query := `
		SELECT l.layer_id, l.map_id, ISNULL(l.dataset_id, ''), ISNULL(l.dataset_layer_id, ''),
		       l.layer_name, l.layer_type, ISNULL(l.geometry_type, ''), ISNULL(l.parent_group_id, ''),
		       l.display_order, l.is_visible, l.is_locked, l.opacity, l.min_zoom, l.max_zoom,
		       ISNULL(l.crs, 'EPSG:4326'), ISNULL(l.bounds_json, ''), ISNULL(l.metadata_json, ''),
		       (SELECT COUNT(1) FROM dbo.features f WHERE f.layer_id = l.layer_id) AS feature_count,
		       l.created_at, l.updated_at
		FROM dbo.map_layers l
		WHERE l.layer_id = @p1;
	`
	row := r.db.QueryRowContext(ctx, query, layerID)
	l, err := scanMapLayerRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrLayerNotFound
		}
		return nil, err
	}
	if st, sErr := r.GetLayerStyle(ctx, l.LayerID); sErr == nil {
		l.Style = st
	}
	return l, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanMapLayer(s rowScanner) (*models.MapLayer, error) {
	return scanMapLayerRow(s)
}

func scanMapLayerRow(s rowScanner) (*models.MapLayer, error) {
	var l models.MapLayer
	var boundsJSON, metaJSON string
	err := s.Scan(
		&l.LayerID, &l.MapID, &l.DatasetID, &l.DatasetLayerID,
		&l.LayerName, &l.LayerType, &l.GeometryType, &l.ParentGroupID,
		&l.DisplayOrder, &l.IsVisible, &l.IsLocked, &l.Opacity, &l.MinZoom, &l.MaxZoom,
		&l.CRS, &boundsJSON, &metaJSON, &l.FeatureCount, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if boundsJSON != "" {
		_ = json.Unmarshal([]byte(boundsJSON), &l.Bounds)
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &l.Metadata)
	}
	return &l, nil
}

func (r *MSSQLGISRepository) UpdateLayer(ctx context.Context, layerID string, req models.UpdateLayerRequest) (*models.MapLayer, error) {
	existing, err := r.GetLayer(ctx, layerID)
	if err != nil {
		return nil, err
	}
	if req.LayerName != nil {
		existing.LayerName = strings.TrimSpace(*req.LayerName)
	}
	if req.ParentGroupID != nil {
		existing.ParentGroupID = strings.TrimSpace(*req.ParentGroupID)
	}
	if req.DisplayOrder != nil {
		existing.DisplayOrder = *req.DisplayOrder
	}
	if req.IsVisible != nil {
		existing.IsVisible = *req.IsVisible
	}
	if req.IsLocked != nil {
		existing.IsLocked = *req.IsLocked
	}
	if req.Opacity != nil {
		existing.Opacity = *req.Opacity
	}
	if req.MinZoom != nil {
		existing.MinZoom = *req.MinZoom
	}
	if req.MaxZoom != nil {
		existing.MaxZoom = *req.MaxZoom
	}
	now := time.Now().UTC()
	query := `
		UPDATE dbo.map_layers
		SET layer_name = @p1,
		    parent_group_id = NULLIF(@p2, ''),
		    display_order = @p3,
		    is_visible = @p4,
		    is_locked = @p5,
		    opacity = @p6,
		    min_zoom = @p7,
		    max_zoom = @p8,
		    updated_at = @p9
		WHERE layer_id = @p10;
	`
	_, err = r.db.ExecContext(ctx, query,
		existing.LayerName, existing.ParentGroupID, existing.DisplayOrder,
		existing.IsVisible, existing.IsLocked, existing.Opacity,
		existing.MinZoom, existing.MaxZoom, now, layerID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetLayer(ctx, layerID)
}

func (r *MSSQLGISRepository) DeleteLayer(ctx context.Context, layerID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM dbo.map_layers WHERE layer_id = @p1;`, layerID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return models.ErrLayerNotFound
	}
	return nil
}

func (r *MSSQLGISRepository) ReorderLayers(ctx context.Context, mapID string, layerIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for idx, lid := range layerIDs {
		_, err := tx.ExecContext(ctx, `
			UPDATE dbo.map_layers
			SET display_order = @p1, updated_at = @p2
			WHERE layer_id = @p3 AND map_id = @p4;
		`, idx, now, lid, mapID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MSSQLGISRepository) DuplicateLayer(ctx context.Context, layerID, userID string) (*models.MapLayer, error) {
	orig, err := r.GetLayer(ctx, layerID)
	if err != nil {
		return nil, err
	}
	order := orig.DisplayOrder + 1
	vis := orig.IsVisible
	locked := false
	op := orig.Opacity
	minZ := orig.MinZoom
	maxZ := orig.MaxZoom
	dup, err := r.CreateLayer(ctx, models.CreateLayerRequest{
		MapID:          orig.MapID,
		DatasetID:      orig.DatasetID,
		DatasetLayerID: orig.DatasetLayerID,
		LayerName:      orig.LayerName + " (Copy)",
		LayerType:      orig.LayerType,
		GeometryType:   orig.GeometryType,
		ParentGroupID:  orig.ParentGroupID,
		DisplayOrder:   &order,
		IsVisible:      &vis,
		IsLocked:       &locked,
		Opacity:        &op,
		MinZoom:        &minZ,
		MaxZoom:        &maxZ,
		CRS:            orig.CRS,
		Bounds:         orig.Bounds,
		Metadata:       orig.Metadata,
	})
	if err != nil {
		return nil, err
	}

	// Copy fields
	if flds, fErr := r.GetLayerFields(ctx, layerID); fErr == nil {
		for _, f := range flds {
			nullable := f.IsNullable
			_, _ = r.AddLayerField(ctx, dup.LayerID, models.AddFieldRequest{
				FieldName:    f.FieldName,
				FieldType:    f.FieldType,
				IsNullable:   &nullable,
				DefaultValue: f.DefaultValue,
			})
		}
	}

	// Copy style
	if orig.Style != nil {
		_, _ = r.UpsertLayerStyle(ctx, dup.LayerID, orig.Style.StyleType, orig.Style.StyleConfig, orig.Style.MapLibreStyle)
	}

	// Copy features
	if feats, ftErr := r.GetFeatures(ctx, layerID, nil); ftErr == nil {
		for _, feat := range feats {
			_, _ = r.CreateFeature(ctx, userID, models.CreateFeatureRequest{
				LayerID:      dup.LayerID,
				Geometry:     feat.Geometry,
				GeometryType: feat.GeometryType,
				Properties:   feat.Properties,
			}, feat.GeometryWKT, feat.SRID, feat.BBox)
		}
	}

	return r.GetLayer(ctx, dup.LayerID)
}

func (r *MSSQLGISRepository) CreateLayerGroup(ctx context.Context, req models.CreateLayerGroupRequest) (*models.LayerGroup, error) {
	now := time.Now().UTC()
	gid := generateGISID("grp", req.MapID+req.GroupName)
	order := 0
	if req.DisplayOrder != nil {
		order = *req.DisplayOrder
	}
	query := `
		INSERT INTO dbo.layer_groups (group_id, map_id, parent_group_id, group_name, display_order, is_expanded, is_visible, created_at, updated_at)
		VALUES (@p1, @p2, NULLIF(@p3, ''), @p4, @p5, 1, 1, @p6, @p7);
	`
	_, err := r.db.ExecContext(ctx, query, gid, req.MapID, req.ParentGroupID, req.GroupName, order, now, now)
	if err != nil {
		return nil, err
	}
	return r.GetLayerGroup(ctx, gid)
}

func (r *MSSQLGISRepository) GetLayerGroups(ctx context.Context, mapID string) ([]*models.LayerGroup, error) {
	query := `
		SELECT group_id, map_id, ISNULL(parent_group_id, ''), group_name, display_order, is_expanded, is_visible, created_at, updated_at
		FROM dbo.layer_groups
		WHERE map_id = @p1
		ORDER BY display_order ASC, created_at ASC;
	`
	rows, err := r.db.QueryContext(ctx, query, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.LayerGroup
	for rows.Next() {
		var g models.LayerGroup
		if err := rows.Scan(&g.GroupID, &g.MapID, &g.ParentGroupID, &g.GroupName, &g.DisplayOrder, &g.IsExpanded, &g.IsVisible, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &g)
	}
	return list, rows.Err()
}

func (r *MSSQLGISRepository) GetLayerGroup(ctx context.Context, groupID string) (*models.LayerGroup, error) {
	query := `
		SELECT group_id, map_id, ISNULL(parent_group_id, ''), group_name, display_order, is_expanded, is_visible, created_at, updated_at
		FROM dbo.layer_groups
		WHERE group_id = @p1;
	`
	var g models.LayerGroup
	err := r.db.QueryRowContext(ctx, query, groupID).Scan(
		&g.GroupID, &g.MapID, &g.ParentGroupID, &g.GroupName, &g.DisplayOrder, &g.IsExpanded, &g.IsVisible, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrLayerGroupNotFound
		}
		return nil, err
	}
	return &g, nil
}

func (r *MSSQLGISRepository) UpdateLayerGroup(ctx context.Context, groupID string, req models.UpdateLayerGroupRequest) (*models.LayerGroup, error) {
	g, err := r.GetLayerGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if req.GroupName != nil && strings.TrimSpace(*req.GroupName) != "" {
		g.GroupName = strings.TrimSpace(*req.GroupName)
	}
	if req.ParentGroupID != nil {
		g.ParentGroupID = strings.TrimSpace(*req.ParentGroupID)
	}
	if req.DisplayOrder != nil {
		g.DisplayOrder = *req.DisplayOrder
	}
	if req.IsExpanded != nil {
		g.IsExpanded = *req.IsExpanded
	}
	if req.IsVisible != nil {
		g.IsVisible = *req.IsVisible
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `
		UPDATE dbo.layer_groups
		SET group_name = @p1, parent_group_id = NULLIF(@p2, ''), display_order = @p3, is_expanded = @p4, is_visible = @p5, updated_at = @p6
		WHERE group_id = @p7;
	`, g.GroupName, g.ParentGroupID, g.DisplayOrder, g.IsExpanded, g.IsVisible, now, groupID)
	if err != nil {
		return nil, err
	}
	return r.GetLayerGroup(ctx, groupID)
}

func (r *MSSQLGISRepository) DeleteLayerGroup(ctx context.Context, groupID string) error {
	_, _ = r.db.ExecContext(ctx, `UPDATE dbo.map_layers SET parent_group_id = NULL WHERE parent_group_id = @p1;`, groupID)
	_, _ = r.db.ExecContext(ctx, `UPDATE dbo.layer_groups SET parent_group_id = NULL WHERE parent_group_id = @p1;`, groupID)
	res, err := r.db.ExecContext(ctx, `DELETE FROM dbo.layer_groups WHERE group_id = @p1;`, groupID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return models.ErrLayerGroupNotFound
	}
	return nil
}

func (r *MSSQLGISRepository) CountLayersByDatasetID(ctx context.Context, datasetID string) (int, error) {
	var cnt int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM dbo.map_layers WHERE dataset_id = @p1;`, datasetID).Scan(&cnt)
	return cnt, err
}

func (r *MSSQLGISRepository) CreateFeature(ctx context.Context, userID string, req models.CreateFeatureRequest, wkt string, srid int, bbox []float64) (*models.VectorFeature, error) {
	now := time.Now().UTC()
	fid := generateGISID("feat", req.LayerID+wkt)
	if req.Properties == nil {
		req.Properties = make(map[string]interface{})
	}
	attrBytes, _ := json.Marshal(req.Properties)
	geomBytes, _ := json.Marshal(req.Geometry)
	// Store raw geojson inside a reserved internal property or reconstruct from coordinates
	var minX, minY, maxX, maxY float64
	if len(bbox) == 4 {
		minX, minY, maxX, maxY = bbox[0], bbox[1], bbox[2], bbox[3]
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	query := `
		INSERT INTO dbo.features (
			feature_id, layer_id, geometry, geometry_wkt, geometry_type, srid,
			bbox_minx, bbox_miny, bbox_maxx, bbox_maxy, attributes,
			created_by, updated_by, created_at, updated_at
		) VALUES (
			@p1, @p2, geometry::STGeomFromText(@p3, @p4).MakeValid(), @p5, @p6, @p4,
			@p7, @p8, @p9, @p10, @p11,
			@p12, @p12, @p13, @p13
		);
	`
	_, err = tx.ExecContext(ctx, query,
		fid, req.LayerID, wkt, srid, string(geomBytes), req.Geometry.Type,
		minX, minY, maxX, maxY, string(attrBytes),
		userID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed inserting feature: %w", err)
	}

	feat := &models.VectorFeature{
		FeatureID:    fid,
		LayerID:      req.LayerID,
		Geometry:     req.Geometry,
		GeometryWKT:  wkt,
		GeometryType: req.Geometry.Type,
		SRID:         srid,
		BBox:         bbox,
		Properties:   req.Properties,
		CreatedBy:    userID,
		UpdatedBy:    userID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	newJSON, _ := json.Marshal(feat.ToGeoJSONFeature())
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO dbo.feature_history (history_id, feature_id, layer_id, user_id, operation, old_data, new_data, created_at)
		VALUES (@p1, @p2, @p3, @p4, 'create', NULL, @p5, @p6);
	`, generateGISID("hst", fid), fid, req.LayerID, userID, string(newJSON), now)

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return feat, nil
}

func (r *MSSQLGISRepository) CreateFeatureBatch(ctx context.Context, userID string, items []models.BatchFeatureItem) ([]*models.VectorFeature, error) {
	if len(items) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	stmtFeat, err := tx.PrepareContext(ctx, `
		INSERT INTO dbo.features (
			feature_id, layer_id, geometry, geometry_wkt, geometry_type, srid,
			bbox_minx, bbox_miny, bbox_maxx, bbox_maxy, attributes,
			created_by, updated_by, created_at, updated_at
		) VALUES (
			@p1, @p2, geometry::STGeomFromText(@p3, @p4).MakeValid(), @p5, @p6, @p4,
			@p7, @p8, @p9, @p10, @p11,
			@p12, @p12, @p13, @p13
		);
	`)
	if err != nil {
		return nil, err
	}
	defer stmtFeat.Close()

	stmtHist, err := tx.PrepareContext(ctx, `
		INSERT INTO dbo.feature_history (history_id, feature_id, layer_id, user_id, operation, old_data, new_data, created_at)
		VALUES (@p1, @p2, @p3, @p4, 'create', NULL, @p5, @p6);
	`)
	if err != nil {
		return nil, err
	}
	defer stmtHist.Close()

	out := make([]*models.VectorFeature, 0, len(items))
	for _, item := range items {
		fid := generateGISID("feat", item.Request.LayerID+item.WKT)
		if item.Request.Properties == nil {
			item.Request.Properties = make(map[string]interface{})
		}
		attrBytes, _ := json.Marshal(item.Request.Properties)
		geomBytes, _ := json.Marshal(item.Request.Geometry)
		var minX, minY, maxX, maxY float64
		if len(item.BBox) == 4 {
			minX, minY, maxX, maxY = item.BBox[0], item.BBox[1], item.BBox[2], item.BBox[3]
		}
		_, execErr := stmtFeat.ExecContext(ctx,
			fid, item.Request.LayerID, item.WKT, item.SRID, string(geomBytes), item.Request.Geometry.Type,
			minX, minY, maxX, maxY, string(attrBytes),
			userID, now,
		)
		if execErr != nil {
			continue
		}
		feat := &models.VectorFeature{
			FeatureID:    fid,
			LayerID:      item.Request.LayerID,
			Geometry:     item.Request.Geometry,
			GeometryWKT:  item.WKT,
			GeometryType: item.Request.Geometry.Type,
			SRID:         item.SRID,
			BBox:         item.BBox,
			Properties:   item.Request.Properties,
			CreatedBy:    userID,
			UpdatedBy:    userID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		newJSON, _ := json.Marshal(feat.ToGeoJSONFeature())
		_, _ = stmtHist.ExecContext(ctx,
			generateGISID("hst", fid), fid, item.Request.LayerID, userID, string(newJSON), now,
		)
		out = append(out, feat)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *MSSQLGISRepository) GetFeatures(ctx context.Context, layerID string, bbox []float64) ([]*models.VectorFeature, error) {
	var rows *sql.Rows
	var err error
	if len(bbox) == 4 {
		query := `
			SELECT feature_id, layer_id, geometry_wkt, geometry_type, srid,
			       ISNULL(bbox_minx, 0), ISNULL(bbox_miny, 0), ISNULL(bbox_maxx, 0), ISNULL(bbox_maxy, 0),
			       attributes, ISNULL(created_by, ''), ISNULL(updated_by, ''), created_at, updated_at
			FROM dbo.features
			WHERE layer_id = @p1
			  AND bbox_maxx >= @p2 AND bbox_minx <= @p3
			  AND bbox_maxy >= @p4 AND bbox_miny <= @p5
			ORDER BY created_at ASC;
		`
		rows, err = r.db.QueryContext(ctx, query, layerID, bbox[0], bbox[2], bbox[1], bbox[3])
	} else {
		query := `
			SELECT feature_id, layer_id, geometry_wkt, geometry_type, srid,
			       ISNULL(bbox_minx, 0), ISNULL(bbox_miny, 0), ISNULL(bbox_maxx, 0), ISNULL(bbox_maxy, 0),
			       attributes, ISNULL(created_by, ''), ISNULL(updated_by, ''), created_at, updated_at
			FROM dbo.features
			WHERE layer_id = @p1
			ORDER BY created_at ASC;
		`
		rows, err = r.db.QueryContext(ctx, query, layerID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.VectorFeature
	for rows.Next() {
		f, err := scanVectorFeature(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, rows.Err()
}

func (r *MSSQLGISRepository) GetFeature(ctx context.Context, featureID string) (*models.VectorFeature, error) {
	query := `
		SELECT feature_id, layer_id, geometry_wkt, geometry_type, srid,
		       ISNULL(bbox_minx, 0), ISNULL(bbox_miny, 0), ISNULL(bbox_maxx, 0), ISNULL(bbox_maxy, 0),
		       attributes, ISNULL(created_by, ''), ISNULL(updated_by, ''), created_at, updated_at
		FROM dbo.features
		WHERE feature_id = @p1;
	`
	f, err := scanVectorFeature(r.db.QueryRowContext(ctx, query, featureID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrFeatureNotFound
		}
		return nil, err
	}
	return f, nil
}

func scanVectorFeature(s rowScanner) (*models.VectorFeature, error) {
	var f models.VectorFeature
	var geomPayload, attrJSON string
	var minX, minY, maxX, maxY float64
	err := s.Scan(
		&f.FeatureID, &f.LayerID, &geomPayload, &f.GeometryType, &f.SRID,
		&minX, &minY, &maxX, &maxY,
		&attrJSON, &f.CreatedBy, &f.UpdatedBy, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	f.BBox = []float64{minX, minY, maxX, maxY}
	if strings.HasPrefix(strings.TrimSpace(geomPayload), "{") {
		_ = json.Unmarshal([]byte(geomPayload), &f.Geometry)
	}
	f.Properties = make(map[string]interface{})
	if attrJSON != "" {
		_ = json.Unmarshal([]byte(attrJSON), &f.Properties)
	}
	return &f, nil
}

func (r *MSSQLGISRepository) UpdateFeature(ctx context.Context, featureID, userID string, geom *models.GeoJSONGeometry, wkt string, srid int, bbox []float64, props map[string]interface{}) (*models.VectorFeature, error) {
	existing, err := r.GetFeature(ctx, featureID)
	if err != nil {
		return nil, err
	}
	oldJSON, _ := json.Marshal(existing.ToGeoJSONFeature())

	if geom != nil {
		existing.Geometry = *geom
		existing.GeometryType = geom.Type
		if srid > 0 {
			existing.SRID = srid
		}
		if len(bbox) == 4 {
			existing.BBox = bbox
		}
	}
	if props != nil {
		if existing.Properties == nil {
			existing.Properties = make(map[string]interface{})
		}
		for k, v := range props {
			existing.Properties[k] = v
		}
	}
	now := time.Now().UTC()
	existing.UpdatedBy = userID
	existing.UpdatedAt = now

	geomBytes, _ := json.Marshal(existing.Geometry)
	attrBytes, _ := json.Marshal(existing.Properties)
	var minX, minY, maxX, maxY float64
	if len(existing.BBox) == 4 {
		minX, minY, maxX, maxY = existing.BBox[0], existing.BBox[1], existing.BBox[2], existing.BBox[3]
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if wkt != "" {
		_, err = tx.ExecContext(ctx, `
			UPDATE dbo.features
			SET geometry = geometry::STGeomFromText(@p1, @p2).MakeValid(),
			    geometry_wkt = @p3,
			    geometry_type = @p4,
			    srid = @p2,
			    bbox_minx = @p5, bbox_miny = @p6, bbox_maxx = @p7, bbox_maxy = @p8,
			    attributes = @p9,
			    updated_by = @p10,
			    updated_at = @p11
			WHERE feature_id = @p12;
		`, wkt, existing.SRID, string(geomBytes), existing.GeometryType, minX, minY, maxX, maxY, string(attrBytes), userID, now, featureID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE dbo.features
			SET attributes = @p1, updated_by = @p2, updated_at = @p3
			WHERE feature_id = @p4;
		`, string(attrBytes), userID, now, featureID)
	}
	if err != nil {
		return nil, err
	}

	newJSON, _ := json.Marshal(existing.ToGeoJSONFeature())
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO dbo.feature_history (history_id, feature_id, layer_id, user_id, operation, old_data, new_data, created_at)
		VALUES (@p1, @p2, @p3, @p4, 'update', @p5, @p6, @p7);
	`, generateGISID("hst", featureID), featureID, existing.LayerID, userID, string(oldJSON), string(newJSON), now)

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return existing, nil
}

func (r *MSSQLGISRepository) DeleteFeature(ctx context.Context, featureID, userID string) error {
	existing, err := r.GetFeature(ctx, featureID)
	if err != nil {
		return err
	}
	oldJSON, _ := json.Marshal(existing.ToGeoJSONFeature())
	now := time.Now().UTC()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.ExecContext(ctx, `
		INSERT INTO dbo.feature_history (history_id, feature_id, layer_id, user_id, operation, old_data, new_data, created_at)
		VALUES (@p1, @p2, @p3, @p4, 'delete', @p5, NULL, @p6);
	`, generateGISID("hst", featureID), featureID, existing.LayerID, userID, string(oldJSON), now)

	if _, err := tx.ExecContext(ctx, `DELETE FROM dbo.features WHERE feature_id = @p1;`, featureID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MSSQLGISRepository) GetFeatureHistory(ctx context.Context, featureID string) ([]*models.FeatureHistoryRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT history_id, feature_id, layer_id, ISNULL(user_id, ''), operation, ISNULL(old_data, ''), ISNULL(new_data, ''), created_at
		FROM dbo.feature_history
		WHERE feature_id = @p1
		ORDER BY created_at DESC;
	`, featureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.FeatureHistoryRecord
	for rows.Next() {
		var rec models.FeatureHistoryRecord
		var oldS, newS string
		if err := rows.Scan(&rec.HistoryID, &rec.FeatureID, &rec.LayerID, &rec.UserID, &rec.Operation, &oldS, &newS, &rec.CreatedAt); err != nil {
			return nil, err
		}
		if oldS != "" {
			_ = json.Unmarshal([]byte(oldS), &rec.OldData)
		}
		if newS != "" {
			_ = json.Unmarshal([]byte(newS), &rec.NewData)
		}
		list = append(list, &rec)
	}
	return list, rows.Err()
}

func (r *MSSQLGISRepository) GetLayerFields(ctx context.Context, layerID string) ([]*models.LayerField, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT field_id, layer_id, field_name, field_type, is_nullable, ISNULL(default_value, ''), display_order, created_at
		FROM dbo.layer_fields
		WHERE layer_id = @p1
		ORDER BY display_order ASC, created_at ASC;
	`, layerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.LayerField
	for rows.Next() {
		var f models.LayerField
		if err := rows.Scan(&f.FieldID, &f.LayerID, &f.FieldName, &f.FieldType, &f.IsNullable, &f.DefaultValue, &f.DisplayOrder, &f.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &f)
	}
	return list, rows.Err()
}

func (r *MSSQLGISRepository) AddLayerField(ctx context.Context, layerID string, req models.AddFieldRequest) (*models.LayerField, error) {
	now := time.Now().UTC()
	fid := generateGISID("fld", layerID+req.FieldName)
	nullable := true
	if req.IsNullable != nil {
		nullable = *req.IsNullable
	}

	var count int
	_ = r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM dbo.layer_fields WHERE layer_id = @p1 AND LOWER(field_name) = LOWER(@p2);`, layerID, req.FieldName).Scan(&count)
	if count > 0 {
		return nil, models.ErrDuplicateFieldName
	}

	var maxOrder sql.NullInt64
	_ = r.db.QueryRowContext(ctx, `SELECT MAX(display_order) FROM dbo.layer_fields WHERE layer_id = @p1;`, layerID).Scan(&maxOrder)
	order := 0
	if maxOrder.Valid {
		order = int(maxOrder.Int64) + 1
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.layer_fields (field_id, layer_id, field_name, field_type, is_nullable, default_value, display_order, created_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, NULLIF(@p6, ''), @p7, @p8);
	`, fid, layerID, req.FieldName, req.FieldType, nullable, req.DefaultValue, order, now)
	if err != nil {
		return nil, err
	}

	return &models.LayerField{
		FieldID:      fid,
		LayerID:      layerID,
		FieldName:    req.FieldName,
		FieldType:    req.FieldType,
		IsNullable:   nullable,
		DefaultValue: req.DefaultValue,
		DisplayOrder: order,
		CreatedAt:    now,
	}, nil
}

func (r *MSSQLGISRepository) DeleteLayerField(ctx context.Context, layerID, fieldID string) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM dbo.layer_fields WHERE layer_id = @p1 AND (field_id = @p2 OR field_name = @p2);
	`, layerID, fieldID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return models.ErrFieldNotFound
	}
	return nil
}

func (r *MSSQLGISRepository) UpdateFeatureAttributes(ctx context.Context, featureID, userID string, values map[string]interface{}) (*models.VectorFeature, error) {
	return r.UpdateFeature(ctx, featureID, userID, nil, "", 0, nil, values)
}

func (r *MSSQLGISRepository) UpsertLayerStyle(ctx context.Context, layerID, styleType string, styleConfig, mapLibreStyle map[string]interface{}) (*models.LayerStyle, error) {
	now := time.Now().UTC()
	sid := generateGISID("sty", layerID)
	cfgBytes, _ := json.Marshal(styleConfig)
	mlBytes, _ := json.Marshal(mapLibreStyle)

	query := `
		MERGE dbo.layer_styles AS target
		USING (SELECT @p1 AS layer_id) AS src
		ON target.layer_id = src.layer_id
		WHEN MATCHED THEN
			UPDATE SET style_type = @p2, style_config = @p3, maplibre_style = @p4, updated_at = @p5
		WHEN NOT MATCHED THEN
			INSERT (style_id, layer_id, style_type, style_config, maplibre_style, updated_at)
			VALUES (@p6, @p1, @p2, @p3, @p4, @p5);
	`
	_, err := r.db.ExecContext(ctx, query, layerID, styleType, string(cfgBytes), string(mlBytes), now, sid)
	if err != nil {
		return nil, err
	}
	return r.GetLayerStyle(ctx, layerID)
}

func (r *MSSQLGISRepository) GetLayerStyle(ctx context.Context, layerID string) (*models.LayerStyle, error) {
	var st models.LayerStyle
	var cfgJSON, mlJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT style_id, layer_id, style_type, style_config, ISNULL(maplibre_style, ''), updated_at
		FROM dbo.layer_styles
		WHERE layer_id = @p1;
	`, layerID).Scan(&st.StyleID, &st.LayerID, &st.StyleType, &cfgJSON, &mlJSON, &st.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return &models.LayerStyle{
				StyleID:   "default_" + layerID,
				LayerID:   layerID,
				StyleType: "single",
				StyleConfig: map[string]interface{}{
					"fill_color":   "#3ecf8e",
					"stroke_color": "#24b47e",
					"point_color":  "#3ecf8e",
					"stroke_width": 2.0,
					"point_radius": 6.0,
					"opacity":      0.85,
				},
				UpdatedAt: time.Now().UTC(),
			}, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(cfgJSON), &st.StyleConfig)
	if mlJSON != "" {
		_ = json.Unmarshal([]byte(mlJSON), &st.MapLibreStyle)
	}
	return &st, nil
}

func (r *MSSQLGISRepository) DeleteLayerStyle(ctx context.Context, layerID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dbo.layer_styles WHERE layer_id = @p1;`, layerID)
	return err
}

func (r *MSSQLGISRepository) SaveDatasetLayers(ctx context.Context, datasetID string, layers []models.DatasetLayerInfo) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, _ = tx.ExecContext(ctx, `DELETE FROM dbo.dataset_layers WHERE dataset_id = @p1;`, datasetID)
	now := time.Now().UTC()
	for _, l := range layers {
		dlid := l.DatasetLayerID
		if dlid == "" {
			dlid = generateGISID("dsl", datasetID+l.LayerName)
		}
		bBytes, _ := json.Marshal(l.Bounds)
		fBytes, _ := json.Marshal(l.Fields)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO dbo.dataset_layers (dataset_layer_id, dataset_id, layer_name, geometry_type, crs, feature_count, bounds_json, fields_json, created_at)
			VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9);
		`, dlid, datasetID, l.LayerName, l.GeometryType, l.CRS, l.FeatureCount, string(bBytes), string(fBytes), now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MSSQLGISRepository) GetDatasetLayers(ctx context.Context, datasetID string) ([]models.DatasetLayerInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT dataset_layer_id, dataset_id, layer_name, geometry_type, ISNULL(crs, ''), feature_count, ISNULL(bounds_json, ''), ISNULL(fields_json, ''), created_at
		FROM dbo.dataset_layers
		WHERE dataset_id = @p1
		ORDER BY created_at ASC;
	`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.DatasetLayerInfo
	for rows.Next() {
		var l models.DatasetLayerInfo
		var bJSON, fJSON string
		if err := rows.Scan(&l.DatasetLayerID, &l.DatasetID, &l.LayerName, &l.GeometryType, &l.CRS, &l.FeatureCount, &bJSON, &fJSON, &l.CreatedAt); err != nil {
			return nil, err
		}
		if bJSON != "" {
			_ = json.Unmarshal([]byte(bJSON), &l.Bounds)
		}
		if fJSON != "" {
			_ = json.Unmarshal([]byte(fJSON), &l.Fields)
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

func (r *MSSQLGISRepository) SaveSpatialQuery(ctx context.Context, userID string, req models.SpatialQueryRequest) (*models.SpatialQuery, error) {
	now := time.Now().UTC()
	qid := generateGISID("sq", userID+req.LayerID+req.QueryName)
	qName := strings.TrimSpace(req.QueryName)
	if qName == "" {
		qName = "Saved Spatial Query"
	}
	cfgBytes, _ := json.Marshal(req)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO dbo.spatial_queries (query_id, map_id, layer_id, query_name, query_config, created_by, created_at)
		VALUES (@p1, NULLIF(@p2, ''), @p3, @p4, @p5, @p6, @p7);
	`, qid, req.MapID, req.LayerID, qName, string(cfgBytes), userID, now)
	if err != nil {
		return nil, err
	}
	return &models.SpatialQuery{
		QueryID:     qid,
		MapID:       req.MapID,
		LayerID:     req.LayerID,
		QueryName:   qName,
		QueryConfig: req,
		CreatedBy:   userID,
		CreatedAt:   now,
	}, nil
}

func (r *MSSQLGISRepository) GetSavedQueries(ctx context.Context, mapID, layerID, userID string) ([]*models.SpatialQuery, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT query_id, ISNULL(map_id, ''), layer_id, query_name, query_config, created_by, created_at
		FROM dbo.spatial_queries
		WHERE (@p1 = '' OR map_id = @p1)
		  AND (@p2 = '' OR layer_id = @p2)
		ORDER BY created_at DESC;
	`, mapID, layerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.SpatialQuery
	for rows.Next() {
		var sq models.SpatialQuery
		var cfgJSON string
		if err := rows.Scan(&sq.QueryID, &sq.MapID, &sq.LayerID, &sq.QueryName, &cfgJSON, &sq.CreatedBy, &sq.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(cfgJSON), &sq.QueryConfig)
		list = append(list, &sq)
	}
	return list, rows.Err()
}

func (r *MSSQLGISRepository) DeleteSpatialQuery(ctx context.Context, queryID, userID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM dbo.spatial_queries WHERE query_id = @p1;`, queryID)
	if err != nil {
		return err
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return models.ErrSpatialQueryNotFound
	}
	return nil
}
