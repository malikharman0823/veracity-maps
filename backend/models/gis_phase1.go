package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrLayerNotFound        = errors.New("layer not found")
	ErrLayerLocked          = errors.New("layer is locked and cannot be modified")
	ErrLayerGroupNotFound   = errors.New("layer group not found")
	ErrFeatureNotFound      = errors.New("feature not found")
	ErrInvalidGeometry      = errors.New("geometry is invalid")
	ErrFieldNotFound        = errors.New("attribute field not found")
	ErrDuplicateFieldName   = errors.New("attribute field with this name already exists")
	ErrInvalidFieldName     = errors.New("invalid attribute field name")
	ErrInvalidStyleConfig   = errors.New("invalid layer style configuration")
	ErrInvalidCRS           = errors.New("unsupported or invalid coordinate reference system")
	ErrDatasetInUse         = errors.New("dataset is referenced by active map layers and cannot be deleted")
	ErrSpatialQueryNotFound = errors.New("spatial query not found")
	ErrInvalidSpatialQuery  = errors.New("invalid spatial query configuration")
)

var validFieldNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)

// ValidateFieldName ensures attribute field names are safe identifiers (prevents SQL injection).
func ValidateFieldName(name string) bool {
	return validFieldNamePattern.MatchString(strings.TrimSpace(name))
}

// ============================================================================
// MODULE A — LAYER MANAGER MODELS & REQUESTS
// ============================================================================

type DatasetLayerInfo struct {
	DatasetLayerID string                   `json:"dataset_layer_id"`
	DatasetID      string                   `json:"dataset_id"`
	LayerName      string                   `json:"layer_name"`
	GeometryType   string                   `json:"geometry_type"`
	CRS            string                   `json:"crs"`
	FeatureCount   int                      `json:"feature_count"`
	Bounds         []float64                `json:"bounds,omitempty"` // [west, south, east, north]
	Fields         []map[string]interface{} `json:"fields,omitempty"`
	CreatedAt      time.Time                `json:"created_at"`
}

type LayerGroup struct {
	GroupID       string    `json:"group_id"`
	MapID         string    `json:"map_id"`
	ParentGroupID string    `json:"parent_group_id,omitempty"`
	GroupName     string    `json:"group_name"`
	DisplayOrder  int       `json:"display_order"`
	IsExpanded    bool      `json:"is_expanded"`
	IsVisible     bool      `json:"is_visible"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MapLayer struct {
	LayerID        string                 `json:"layer_id"`
	MapID          string                 `json:"map_id"`
	DatasetID      string                 `json:"dataset_id,omitempty"`
	DatasetLayerID string                 `json:"dataset_layer_id,omitempty"`
	LayerName      string                 `json:"layer_name"`
	LayerType      string                 `json:"layer_type"` // "raster", "vector", "cad"
	GeometryType   string                 `json:"geometry_type,omitempty"`
	ParentGroupID  string                 `json:"parent_group_id,omitempty"`
	DisplayOrder   int                    `json:"display_order"`
	IsVisible      bool                   `json:"is_visible"`
	IsLocked       bool                   `json:"is_locked"`
	Opacity        float64                `json:"opacity"`
	MinZoom        float64                `json:"min_zoom"`
	MaxZoom        float64                `json:"max_zoom"`
	CRS            string                 `json:"crs,omitempty"`
	Bounds         []float64              `json:"bounds,omitempty"` // [west, south, east, north]
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	FeatureCount   int                    `json:"feature_count"`
	Style          *LayerStyle            `json:"style,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type CreateLayerRequest struct {
	MapID          string                 `json:"map_id"`
	DatasetID      string                 `json:"dataset_id,omitempty"`
	DatasetLayerID string                 `json:"dataset_layer_id,omitempty"`
	LayerName      string                 `json:"layer_name"`
	LayerType      string                 `json:"layer_type"` // "raster", "vector", "cad"
	GeometryType   string                 `json:"geometry_type,omitempty"`
	ParentGroupID  string                 `json:"parent_group_id,omitempty"`
	DisplayOrder   *int                   `json:"display_order,omitempty"`
	IsVisible      *bool                  `json:"is_visible,omitempty"`
	IsLocked       *bool                  `json:"is_locked,omitempty"`
	Opacity        *float64               `json:"opacity,omitempty"`
	MinZoom        *float64               `json:"min_zoom,omitempty"`
	MaxZoom        *float64               `json:"max_zoom,omitempty"`
	CRS            string                 `json:"crs,omitempty"`
	Bounds         []float64              `json:"bounds,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

func (r *CreateLayerRequest) Validate() error {
	r.LayerName = strings.TrimSpace(r.LayerName)
	if r.LayerName == "" {
		return errors.New("layer_name is required")
	}
	r.LayerType = strings.ToLower(strings.TrimSpace(r.LayerType))
	if r.LayerType == "" {
		r.LayerType = "vector"
	}
	if r.LayerType != "raster" && r.LayerType != "vector" && r.LayerType != "cad" {
		return errors.New("layer_type must be 'raster', 'vector', or 'cad'")
	}
	if r.Opacity != nil && (*r.Opacity < 0 || *r.Opacity > 1) {
		return errors.New("opacity must be between 0 and 1")
	}
	if r.MinZoom != nil && (*r.MinZoom < 0 || *r.MinZoom > 24) {
		return errors.New("min_zoom must be between 0 and 24")
	}
	if r.MaxZoom != nil && (*r.MaxZoom < 0 || *r.MaxZoom > 24) {
		return errors.New("max_zoom must be between 0 and 24")
	}
	if r.MinZoom != nil && r.MaxZoom != nil && *r.MinZoom > *r.MaxZoom {
		return errors.New("min_zoom cannot exceed max_zoom")
	}
	return nil
}

type UpdateLayerRequest struct {
	LayerName     *string  `json:"layer_name,omitempty"`
	ParentGroupID *string  `json:"parent_group_id,omitempty"`
	DisplayOrder  *int     `json:"display_order,omitempty"`
	IsVisible     *bool    `json:"is_visible,omitempty"`
	IsLocked      *bool    `json:"is_locked,omitempty"`
	Opacity       *float64 `json:"opacity,omitempty"`
	MinZoom       *float64 `json:"min_zoom,omitempty"`
	MaxZoom       *float64 `json:"max_zoom,omitempty"`
}

func (r *UpdateLayerRequest) Validate() error {
	if r.LayerName != nil && strings.TrimSpace(*r.LayerName) == "" {
		return errors.New("layer_name cannot be empty")
	}
	if r.Opacity != nil && (*r.Opacity < 0 || *r.Opacity > 1) {
		return errors.New("opacity must be between 0 and 1")
	}
	if r.MinZoom != nil && (*r.MinZoom < 0 || *r.MinZoom > 24) {
		return errors.New("min_zoom must be between 0 and 24")
	}
	if r.MaxZoom != nil && (*r.MaxZoom < 0 || *r.MaxZoom > 24) {
		return errors.New("max_zoom must be between 0 and 24")
	}
	if r.MinZoom != nil && r.MaxZoom != nil && *r.MinZoom > *r.MaxZoom {
		return errors.New("min_zoom cannot exceed max_zoom")
	}
	return nil
}

type ReorderLayersRequest struct {
	LayerIDs []string `json:"layer_ids"`
}

func (r *ReorderLayersRequest) Validate() error {
	if len(r.LayerIDs) == 0 {
		return errors.New("layer_ids must not be empty")
	}
	return nil
}

type CreateLayerGroupRequest struct {
	MapID         string `json:"map_id"`
	GroupName     string `json:"group_name"`
	ParentGroupID string `json:"parent_group_id,omitempty"`
	DisplayOrder  *int   `json:"display_order,omitempty"`
}

func (r *CreateLayerGroupRequest) Validate() error {
	r.GroupName = strings.TrimSpace(r.GroupName)
	if r.GroupName == "" {
		return errors.New("group_name is required")
	}
	return nil
}

type UpdateLayerGroupRequest struct {
	GroupName     *string `json:"group_name,omitempty"`
	ParentGroupID *string `json:"parent_group_id,omitempty"`
	DisplayOrder  *int    `json:"display_order,omitempty"`
	IsExpanded    *bool   `json:"is_expanded,omitempty"`
	IsVisible     *bool   `json:"is_visible,omitempty"`
}

// ============================================================================
// MODULE B — VECTOR EDITING MODELS & REQUESTS
// ============================================================================

type GeoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type VectorFeature struct {
	FeatureID    string                 `json:"feature_id"`
	LayerID      string                 `json:"layer_id"`
	Geometry     GeoJSONGeometry        `json:"geometry"`
	GeometryWKT  string                 `json:"geometry_wkt,omitempty"`
	GeometryType string                 `json:"geometry_type"`
	SRID         int                    `json:"srid"`
	BBox         []float64              `json:"bbox,omitempty"` // [minX, minY, maxX, maxY]
	Properties   map[string]interface{} `json:"properties"`
	CreatedBy    string                 `json:"created_by,omitempty"`
	UpdatedBy    string                 `json:"updated_by,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// ToGeoJSONFeature converts VectorFeature to a standard GeoJSON Feature object.
func (f *VectorFeature) ToGeoJSONFeature() map[string]interface{} {
	props := make(map[string]interface{}, len(f.Properties)+4)
	for k, v := range f.Properties {
		props[k] = v
	}
	props["feature_id"] = f.FeatureID
	props["layer_id"] = f.LayerID
	props["created_by"] = f.CreatedBy
	props["updated_by"] = f.UpdatedBy
	return map[string]interface{}{
		"type":       "Feature",
		"id":         f.FeatureID,
		"geometry":   f.Geometry,
		"bbox":       f.BBox,
		"properties": props,
	}
}

type FeatureHistoryRecord struct {
	HistoryID string                 `json:"history_id"`
	FeatureID string                 `json:"feature_id"`
	LayerID   string                 `json:"layer_id"`
	UserID    string                 `json:"user_id"`
	Operation string                 `json:"operation"`
	OldData   map[string]interface{} `json:"old_data,omitempty"`
	NewData   map[string]interface{} `json:"new_data,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

type CreateFeatureRequest struct {
	LayerID      string                 `json:"layer_id"`
	Geometry     GeoJSONGeometry        `json:"geometry"`
	GeometryType string                 `json:"geometry_type,omitempty"`
	CRS          string                 `json:"crs,omitempty"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
}

type BatchFeatureItem struct {
	Request CreateFeatureRequest `json:"request"`
	WKT     string               `json:"wkt"`
	SRID    int                  `json:"srid"`
	BBox    []float64            `json:"bbox"`
}

func (r *CreateFeatureRequest) Validate() error {
	if strings.TrimSpace(r.Geometry.Type) == "" || len(r.Geometry.Coordinates) == 0 {
		return errors.New("valid GeoJSON geometry with type and coordinates is required")
	}
	return nil
}

type UpdateFeatureRequest struct {
	Geometry   *GeoJSONGeometry       `json:"geometry,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

func (r *UpdateFeatureRequest) Validate() error {
	if r.Geometry == nil && r.Properties == nil {
		return errors.New("at least one of geometry or properties must be provided")
	}
	if r.Geometry != nil && (strings.TrimSpace(r.Geometry.Type) == "" || len(r.Geometry.Coordinates) == 0) {
		return errors.New("valid GeoJSON geometry is required")
	}
	return nil
}

type MergeFeaturesRequest struct {
	LayerID       string   `json:"layer_id,omitempty"`
	FeatureIDs    []string `json:"feature_ids"`
	OutputName    string   `json:"output_name,omitempty"`
	KeepOriginals bool     `json:"keep_originals,omitempty"`
}

func (r *MergeFeaturesRequest) Validate() error {
	if len(r.FeatureIDs) < 2 {
		return errors.New("at least 2 feature_ids are required to merge")
	}
	return nil
}

type SplitFeatureRequest struct {
	FeatureID     string          `json:"feature_id"`
	SplitGeometry GeoJSONGeometry `json:"split_geometry"`
}

func (r *SplitFeatureRequest) Validate() error {
	if strings.TrimSpace(r.SplitGeometry.Type) == "" || len(r.SplitGeometry.Coordinates) == 0 {
		return errors.New("split_geometry (LineString) is required")
	}
	return nil
}

type SnapGeometryRequest struct {
	Geometry      GeoJSONGeometry `json:"geometry"`
	TargetLayerID string          `json:"target_layer_id"`
	Tolerance     float64         `json:"tolerance"`
	Unit          string          `json:"unit,omitempty"` // "degrees", "meters"
}

func (r *SnapGeometryRequest) Validate() error {
	if strings.TrimSpace(r.Geometry.Type) == "" || len(r.Geometry.Coordinates) == 0 {
		return errors.New("geometry is required")
	}
	if strings.TrimSpace(r.TargetLayerID) == "" {
		return errors.New("target_layer_id is required")
	}
	if r.Tolerance <= 0 {
		r.Tolerance = 0.0005
	}
	return nil
}

type ValidateGeometryRequest struct {
	Geometry GeoJSONGeometry `json:"geometry"`
	CRS      string          `json:"crs,omitempty"`
}

type GeometryValidationResult struct {
	IsValid      bool             `json:"is_valid"`
	GeometryType string           `json:"geometry_type"`
	Reason       string           `json:"reason,omitempty"`
	WKT          string           `json:"wkt,omitempty"`
	BBox         []float64        `json:"bbox,omitempty"`
	VertexCount  int              `json:"vertex_count"`
	Area         float64          `json:"area,omitempty"`
	Length       float64          `json:"length,omitempty"`
	Repaired     *GeoJSONGeometry `json:"repaired_geometry,omitempty"`
}

// ============================================================================
// MODULE C — ATTRIBUTE TABLE MODELS & REQUESTS
// ============================================================================

type LayerField struct {
	FieldID      string    `json:"field_id"`
	LayerID      string    `json:"layer_id"`
	FieldName    string    `json:"field_name"`
	FieldType    string    `json:"field_type"` // "string", "integer", "float", "boolean", "date"
	IsNullable   bool      `json:"is_nullable"`
	DefaultValue string    `json:"default_value,omitempty"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
}

type AddFieldRequest struct {
	FieldName    string `json:"field_name"`
	FieldType    string `json:"field_type"`
	IsNullable   *bool  `json:"is_nullable,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
}

func (r *AddFieldRequest) Validate() error {
	r.FieldName = strings.TrimSpace(r.FieldName)
	if !ValidateFieldName(r.FieldName) {
		return fmt.Errorf("%w: must start with a letter or underscore and contain only alphanumeric characters or underscores", ErrInvalidFieldName)
	}
	r.FieldType = strings.ToLower(strings.TrimSpace(r.FieldType))
	switch r.FieldType {
	case "string", "text", "varchar", "nvarchar":
		r.FieldType = "string"
	case "int", "integer", "bigint":
		r.FieldType = "integer"
	case "float", "double", "number", "numeric", "decimal":
		r.FieldType = "float"
	case "bool", "boolean":
		r.FieldType = "boolean"
	case "date", "datetime", "timestamp":
		r.FieldType = "date"
	default:
		return errors.New("field_type must be 'string', 'integer', 'float', 'boolean', or 'date'")
	}
	return nil
}

type UpdateAttributeRequest struct {
	Values map[string]interface{} `json:"values"`
}

func (r *UpdateAttributeRequest) Validate() error {
	if len(r.Values) == 0 {
		return errors.New("values cannot be empty")
	}
	for k := range r.Values {
		if !ValidateFieldName(k) {
			return fmt.Errorf("%w: %s", ErrInvalidFieldName, k)
		}
	}
	return nil
}

type AttributeFilterCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // "=", "!=", ">", "<", ">=", "<=", "LIKE", "IN", "IS NULL", "IS NOT NULL"
	Value    interface{} `json:"value,omitempty"`
}

type FilterAttributeRequest struct {
	Filters   []AttributeFilterCondition `json:"filters,omitempty"`
	Search    string                     `json:"search,omitempty"`
	Page      int                        `json:"page,omitempty"`
	PageSize  int                        `json:"page_size,omitempty"`
	SortBy    string                     `json:"sort_by,omitempty"`
	SortOrder string                     `json:"sort_order,omitempty"` // "asc", "desc"
}

func (r *FilterAttributeRequest) Normalize() {
	if r.Page < 1 {
		r.Page = 1
	}
	if r.PageSize < 1 {
		r.PageSize = 50
	}
	if r.PageSize > 500 {
		r.PageSize = 500
	}
	r.SortOrder = strings.ToLower(strings.TrimSpace(r.SortOrder))
	if r.SortOrder != "desc" {
		r.SortOrder = "asc"
	}
}

type CalculateFieldRequest struct {
	TargetField string   `json:"target_field"`
	Expression  string   `json:"expression"`
	FeatureIDs  []string `json:"feature_ids,omitempty"` // Optional subset of features
}

func (r *CalculateFieldRequest) Validate() error {
	r.TargetField = strings.TrimSpace(r.TargetField)
	if !ValidateFieldName(r.TargetField) {
		return fmt.Errorf("%w: %s", ErrInvalidFieldName, r.TargetField)
	}
	r.Expression = strings.TrimSpace(r.Expression)
	if r.Expression == "" {
		return errors.New("expression is required")
	}
	if len(r.Expression) > 512 {
		return errors.New("expression exceeds maximum allowed length (512 chars)")
	}
	return nil
}

// ============================================================================
// MODULE D — SYMBOLOGY & STYLING ENGINE MODELS & REQUESTS
// ============================================================================

type LayerStyle struct {
	StyleID       string                 `json:"style_id"`
	LayerID       string                 `json:"layer_id"`
	StyleType     string                 `json:"style_type"` // "single", "categorized", "graduated", "rule_based", "heatmap"
	StyleConfig   map[string]interface{} `json:"style_config"`
	MapLibreStyle map[string]interface{} `json:"maplibre_style,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

type UpdateStyleRequest struct {
	StyleType   string                 `json:"style_type"`
	StyleConfig map[string]interface{} `json:"style_config"`
}

func (r *UpdateStyleRequest) Validate() error {
	r.StyleType = strings.ToLower(strings.TrimSpace(r.StyleType))
	switch r.StyleType {
	case "single", "categorized", "graduated", "rule_based", "heatmap":
	default:
		return fmt.Errorf("%w: style_type must be 'single', 'categorized', 'graduated', 'rule_based', or 'heatmap'", ErrInvalidStyleConfig)
	}
	if r.StyleConfig == nil {
		return fmt.Errorf("%w: style_config is required", ErrInvalidStyleConfig)
	}
	return nil
}

// ============================================================================
// MODULE E — CRS MANAGEMENT MODELS & REQUESTS
// ============================================================================

type CRSDefinition struct {
	EPSGCode    string    `json:"epsg_code"` // e.g., "EPSG:4326"
	SRID        int       `json:"srid"`      // e.g., 4326
	Name        string    `json:"name"`
	Type        string    `json:"type"` // "geographic" or "projected"
	Units       string    `json:"units"`
	AreaOfUse   string    `json:"area_of_use"`
	BoundsWGS84 []float64 `json:"bounds_wgs84,omitempty"`
	Proj4       string    `json:"proj4,omitempty"`
	WKT         string    `json:"wkt,omitempty"`
}

type ReprojectDatasetRequest struct {
	DatasetID  string `json:"dataset_id"`
	SourceEPSG string `json:"source_epsg,omitempty"`
	TargetEPSG string `json:"target_epsg"`
	OutputName string `json:"output_name,omitempty"`
}

func (r *ReprojectDatasetRequest) Validate() error {
	r.TargetEPSG = strings.ToUpper(strings.TrimSpace(r.TargetEPSG))
	if r.TargetEPSG == "" {
		return errors.New("target_epsg is required (e.g. EPSG:4326 or EPSG:3857)")
	}
	if !strings.HasPrefix(r.TargetEPSG, "EPSG:") {
		r.TargetEPSG = "EPSG:" + r.TargetEPSG
	}
	if r.SourceEPSG != "" {
		r.SourceEPSG = strings.ToUpper(strings.TrimSpace(r.SourceEPSG))
		if !strings.HasPrefix(r.SourceEPSG, "EPSG:") {
			r.SourceEPSG = "EPSG:" + r.SourceEPSG
		}
	}
	return nil
}

// ============================================================================
// MODULE G — SPATIAL QUERY BUILDER MODELS & REQUESTS
// ============================================================================

type SpatialQuery struct {
	QueryID     string              `json:"query_id"`
	MapID       string              `json:"map_id,omitempty"`
	LayerID     string              `json:"layer_id"`
	QueryName   string              `json:"query_name"`
	QueryConfig SpatialQueryRequest `json:"query_config"`
	CreatedBy   string              `json:"created_by"`
	CreatedAt   time.Time           `json:"created_at"`
}

type SpatialQueryRequest struct {
	MapID           string                     `json:"map_id,omitempty"`
	LayerID         string                     `json:"layer_id"`
	TargetLayerID   string                     `json:"target_layer_id,omitempty"`
	QueryName       string                     `json:"query_name,omitempty"`
	CombineOperator string                     `json:"combine_operator,omitempty"` // "AND", "OR"
	Conditions      []AttributeFilterCondition `json:"conditions,omitempty"`
	SpatialOperator string                     `json:"spatial_operator,omitempty"` // "intersects", "contains", "within", "touches", "overlaps", "crosses", "nearest", "within_distance", "beyond_distance"
	Geometry        *GeoJSONGeometry           `json:"geometry,omitempty"`
	Distance        float64                    `json:"distance,omitempty"`
	Unit            string                     `json:"unit,omitempty"` // "meters", "kilometers", "degrees", "feet"
	Page            int                        `json:"page,omitempty"`
	PageSize        int                        `json:"page_size,omitempty"`
}

func (r *SpatialQueryRequest) Validate() error {
	if strings.TrimSpace(r.LayerID) == "" {
		return fmt.Errorf("%w: layer_id is required", ErrInvalidSpatialQuery)
	}
	r.CombineOperator = strings.ToUpper(strings.TrimSpace(r.CombineOperator))
	if r.CombineOperator == "" {
		r.CombineOperator = "AND"
	}
	if r.CombineOperator != "AND" && r.CombineOperator != "OR" {
		return fmt.Errorf("%w: combine_operator must be 'AND' or 'OR'", ErrInvalidSpatialQuery)
	}

	validAttrOps := map[string]bool{
		"=": true, "!=": true, ">": true, "<": true, ">=": true, "<=": true,
		"LIKE": true, "IN": true, "IS NULL": true, "IS NOT NULL": true,
	}
	for _, cond := range r.Conditions {
		if !ValidateFieldName(cond.Field) {
			return fmt.Errorf("%w: invalid attribute field name '%s'", ErrInvalidSpatialQuery, cond.Field)
		}
		op := strings.ToUpper(strings.TrimSpace(cond.Operator))
		if !validAttrOps[op] {
			return fmt.Errorf("%w: unsupported attribute operator '%s'", ErrInvalidSpatialQuery, cond.Operator)
		}
	}

	if r.SpatialOperator != "" {
		op := strings.ToLower(strings.TrimSpace(r.SpatialOperator))
		validSpatialOps := map[string]bool{
			"intersects":      true,
			"contains":        true,
			"within":          true,
			"touches":         true,
			"overlaps":        true,
			"crosses":         true,
			"nearest":         true,
			"within_distance": true,
			"beyond_distance": true,
		}
		if !validSpatialOps[op] {
			return fmt.Errorf("%w: unsupported spatial operator '%s'", ErrInvalidSpatialQuery, r.SpatialOperator)
		}
		r.SpatialOperator = op
		if r.Geometry == nil && strings.TrimSpace(r.TargetLayerID) == "" {
			return fmt.Errorf("%w: spatial query requires either 'geometry' or 'target_layer_id'", ErrInvalidSpatialQuery)
		}
		if (op == "within_distance" || op == "beyond_distance") && r.Distance <= 0 {
			return fmt.Errorf("%w: distance must be positive for '%s'", ErrInvalidSpatialQuery, op)
		}
	}

	if r.Page < 1 {
		r.Page = 1
	}
	if r.PageSize < 1 {
		r.PageSize = 100
	}
	if r.PageSize > 1000 {
		r.PageSize = 1000
	}
	return nil
}
