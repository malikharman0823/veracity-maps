package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("requested resource not found")
)

type ValidationError struct {
	Code    string
	Message string
	Field   string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func NewValidationError(code, msg, field string) error {
	return &ValidationError{Code: code, Message: msg, Field: field}
}

// ============================================================================
// VERACITY MAPS 1.4 — PHASE 2: GIS PROCESSING ENGINE MODELS
// ============================================================================

const (
	JobStatusQueued     = "queued"
	JobStatusValidating = "validating"
	JobStatusRunning    = "running"
	JobStatusCompleted  = "completed"
	JobStatusFailed     = "failed"
	JobStatusCancelled  = "cancelled"
)

// ProcessingAlgorithm describes a GIS Processing Toolbox algorithm and its parameter schema.
type ProcessingAlgorithm struct {
	AlgorithmID string                   `json:"algorithm_id"`
	Name        string                   `json:"name"`
	Category    string                   `json:"category"`
	Description string                   `json:"description"`
	Library     string                   `json:"library"` // "GEOS", "GDAL/OGR", "PROJ", "GDAL/Raster"
	InputType   string                   `json:"input_type"`
	OutputType  string                   `json:"output_type"`
	Parameters  []AlgorithmParameterSpec `json:"parameters"`
}

type AlgorithmParameterSpec struct {
	Name         string      `json:"name"`
	Label        string      `json:"label"`
	Type         string      `json:"type"` // "layer", "dataset", "number", "string", "boolean", "select", "fields"
	Required     bool        `json:"required"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Options      []string    `json:"options,omitempty"`
	Min          *float64    `json:"min,omitempty"`
	Max          *float64    `json:"max,omitempty"`
	Description  string      `json:"description,omitempty"`
}

// ProcessingJob represents an asynchronous or completed GIS processing task.
type ProcessingJob struct {
	JobID         string                 `json:"job_id"`
	ProjectID     string                 `json:"project_id,omitempty"`
	MapID         string                 `json:"map_id,omitempty"`
	UserID        string                 `json:"user_id"`
	AlgorithmName string                 `json:"algorithm_name"`
	Status        string                 `json:"status"`
	Progress      int                    `json:"progress"`
	Stage         string                 `json:"stage"`
	InputConfig   map[string]interface{} `json:"input_config"`
	OutputConfig  map[string]interface{} `json:"output_config,omitempty"`
	Inputs        []ProcessingJobInput   `json:"inputs,omitempty"`
	Outputs       []ProcessingJobOutput  `json:"outputs,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	StartedAt     *time.Time             `json:"started_at,omitempty"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
}

type ProcessingJobLog struct {
	LogID     int64     `json:"log_id"`
	JobID     string    `json:"job_id"`
	Stage     string    `json:"stage"`
	LogLevel  string    `json:"log_level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type ProcessingJobInput struct {
	InputID   int64  `json:"input_id,omitempty"`
	JobID     string `json:"job_id"`
	DatasetID string `json:"dataset_id,omitempty"`
	LayerID   string `json:"layer_id,omitempty"`
	InputRole string `json:"input_role"`
}

type ProcessingJobOutput struct {
	OutputID     int64     `json:"output_id,omitempty"`
	JobID        string    `json:"job_id"`
	DatasetID    string    `json:"dataset_id,omitempty"`
	LayerID      string    `json:"layer_id,omitempty"`
	OutputName   string    `json:"output_name"`
	StoragePath  string    `json:"storage_path,omitempty"`
	OutputFormat string    `json:"output_format"`
	CreatedAt    time.Time `json:"created_at"`
}

type TopologyRule struct {
	RuleID     string                 `json:"rule_id"`
	ProjectID  string                 `json:"project_id,omitempty"`
	MapID      string                 `json:"map_id,omitempty"`
	LayerID    string                 `json:"layer_id"`
	RuleName   string                 `json:"rule_name"`
	RuleType   string                 `json:"rule_type"`
	RuleConfig map[string]interface{} `json:"rule_config,omitempty"`
	IsEnabled  bool                   `json:"is_enabled"`
	CreatedAt  time.Time              `json:"created_at"`
}

type TopologyError struct {
	ErrorID      string           `json:"error_id"`
	JobID        string           `json:"job_id"`
	LayerID      string           `json:"layer_id"`
	FeatureID    string           `json:"feature_id,omitempty"`
	ErrorType    string           `json:"error_type"`
	ErrorMessage string           `json:"error_message"`
	Geometry     *GeoJSONGeometry `json:"geometry,omitempty"`
	GeomWKT      string           `json:"-"`
	Suggestion   string           `json:"suggestion,omitempty"`
	IsResolved   bool             `json:"is_resolved"`
	CreatedAt    time.Time        `json:"created_at"`
}

type ProcessingPreset struct {
	PresetID      string                 `json:"preset_id"`
	ProjectID     string                 `json:"project_id,omitempty"`
	MapID         string                 `json:"map_id,omitempty"`
	UserID        string                 `json:"user_id"`
	AlgorithmName string                 `json:"algorithm_name"`
	PresetName    string                 `json:"preset_name"`
	Parameters    map[string]interface{} `json:"parameters"`
	CreatedAt     time.Time              `json:"created_at"`
}

// ============================================================================
// REQUEST MODELS & VALIDATION
// ============================================================================

type ProcessingJobRequest struct {
	ProjectID       string                 `json:"project_id"`
	MapID           string                 `json:"map_id"`
	AlgorithmName   string                 `json:"algorithm_name"`
	Parameters      map[string]interface{} `json:"parameters"`
	InputDatasetIDs []string               `json:"input_dataset_ids"`
	InputLayerIDs   []string               `json:"input_layer_ids"`
	OutputName      string                 `json:"output_name"`
	Sync            bool                   `json:"sync,omitempty"`
}

func (r *ProcessingJobRequest) Validate() error {
	r.AlgorithmName = strings.ToLower(strings.TrimSpace(r.AlgorithmName))
	if r.AlgorithmName == "" {
		return NewValidationError("MISSING_ALGORITHM", "algorithm_name is required", "algorithm_name")
	}
	if r.Parameters == nil {
		r.Parameters = map[string]interface{}{}
	}
	return nil
}

// Module B — BufferRequest
type BufferRequest struct {
	MapID        string  `json:"map_id"`
	InputLayerID string  `json:"input_layer_id"`
	Distance     float64 `json:"distance"`
	Unit         string  `json:"unit"` // meters, kilometers, feet, miles
	Segments     int     `json:"segments"`
	CapStyle     string  `json:"cap_style"`  // round, flat, square
	JoinStyle    string  `json:"join_style"` // round, mitre, bevel
	Dissolve     bool    `json:"dissolve"`
	OutputName   string  `json:"output_name"`
	OutputCRS    string  `json:"output_crs"`
	Sync         bool    `json:"sync,omitempty"`
}

func (r *BufferRequest) Validate() error {
	if strings.TrimSpace(r.InputLayerID) == "" {
		return NewValidationError("MISSING_INPUT_LAYER", "input_layer_id is required", "input_layer_id")
	}
	if r.Distance <= 0 || r.Distance > 1e7 {
		return NewValidationError("INVALID_BUFFER_DISTANCE", "distance must be > 0 and <= 10,000,000", "distance")
	}
	unit := strings.ToLower(strings.TrimSpace(r.Unit))
	if unit == "" {
		unit = "meters"
	}
	switch unit {
	case "meters", "metres", "m", "kilometers", "kilometres", "km", "feet", "ft", "miles", "mi":
		r.Unit = unit
	default:
		return NewValidationError("INVALID_UNIT", fmt.Sprintf("unsupported distance unit '%s'", r.Unit), "unit")
	}
	if r.Segments <= 0 {
		r.Segments = 16
	}
	if r.Segments > 128 {
		r.Segments = 128
	}
	capStyle := strings.ToLower(strings.TrimSpace(r.CapStyle))
	if capStyle == "" {
		capStyle = "round"
	}
	switch capStyle {
	case "round", "flat", "square":
		r.CapStyle = capStyle
	default:
		return NewValidationError("INVALID_CAP_STYLE", "cap_style must be round, flat, or square", "cap_style")
	}
	joinStyle := strings.ToLower(strings.TrimSpace(r.JoinStyle))
	if joinStyle == "" {
		joinStyle = "round"
	}
	switch joinStyle {
	case "round", "mitre", "miter", "bevel":
		r.JoinStyle = joinStyle
	default:
		return NewValidationError("INVALID_JOIN_STYLE", "join_style must be round, mitre, or bevel", "join_style")
	}
	if strings.TrimSpace(r.OutputCRS) == "" {
		r.OutputCRS = "EPSG:4326"
	}
	return nil
}

// Module C — OverlayRequest & DissolveRequest
type OverlayRequest struct {
	MapID              string `json:"map_id"`
	Operation          string `json:"operation,omitempty"` // intersection, union, difference, clip, symmetrical_difference
	InputLayerA        string `json:"input_layer_a"`
	InputLayerB        string `json:"input_layer_b"`
	OutputName         string `json:"output_name"`
	OutputCRS          string `json:"output_crs"`
	PreserveAttributes string `json:"preserve_attributes"` // all, layer_a, none
	Sync               bool   `json:"sync,omitempty"`
}

func (r *OverlayRequest) Validate() error {
	if strings.TrimSpace(r.InputLayerA) == "" {
		return NewValidationError("MISSING_LAYER_A", "input_layer_a is required", "input_layer_a")
	}
	if strings.TrimSpace(r.InputLayerB) == "" {
		return NewValidationError("MISSING_LAYER_B", "input_layer_b is required", "input_layer_b")
	}
	if strings.TrimSpace(r.PreserveAttributes) == "" {
		r.PreserveAttributes = "all"
	}
	if strings.TrimSpace(r.OutputCRS) == "" {
		r.OutputCRS = "EPSG:4326"
	}
	return nil
}

type DissolveRequest struct {
	MapID          string   `json:"map_id"`
	InputLayerID   string   `json:"input_layer_id"`
	DissolveFields []string `json:"dissolve_fields"`
	DissolveAll    bool     `json:"dissolve_all"`
	OutputName     string   `json:"output_name"`
	OutputCRS      string   `json:"output_crs"`
	Sync           bool     `json:"sync,omitempty"`
}

func (r *DissolveRequest) Validate() error {
	if strings.TrimSpace(r.InputLayerID) == "" {
		return NewValidationError("MISSING_INPUT_LAYER", "input_layer_id is required", "input_layer_id")
	}
	for _, f := range r.DissolveFields {
		if !ValidateFieldName(f) {
			return NewValidationError("INVALID_FIELD_NAME", fmt.Sprintf("invalid dissolve field name '%s'", f), "dissolve_fields")
		}
	}
	if len(r.DissolveFields) == 0 {
		r.DissolveAll = true
	}
	if strings.TrimSpace(r.OutputCRS) == "" {
		r.OutputCRS = "EPSG:4326"
	}
	return nil
}

// Module D — SpatialJoinRequest
type SpatialJoinRequest struct {
	MapID               string   `json:"map_id"`
	TargetLayerID       string   `json:"target_layer_id"`
	SourceLayerID       string   `json:"source_layer_id"`
	SpatialRelationship string   `json:"spatial_relationship"` // intersects, contains, within, touches, overlaps, crosses, nearest
	AggregationType     string   `json:"aggregation_type"`     // count, sum, avg, min, max, first, all
	SelectedFields      []string `json:"selected_fields"`
	OutputName          string   `json:"output_name"`
	OutputCRS           string   `json:"output_crs"`
	Sync                bool     `json:"sync,omitempty"`
}

func (r *SpatialJoinRequest) Validate() error {
	if strings.TrimSpace(r.TargetLayerID) == "" {
		return NewValidationError("MISSING_TARGET_LAYER", "target_layer_id is required", "target_layer_id")
	}
	if strings.TrimSpace(r.SourceLayerID) == "" {
		return NewValidationError("MISSING_SOURCE_LAYER", "source_layer_id is required", "source_layer_id")
	}
	rel := strings.ToLower(strings.TrimSpace(r.SpatialRelationship))
	if rel == "" {
		rel = "intersects"
	}
	switch rel {
	case "intersects", "contains", "within", "touches", "overlaps", "crosses", "nearest":
		r.SpatialRelationship = rel
	default:
		return NewValidationError("INVALID_SPATIAL_RELATIONSHIP", fmt.Sprintf("unsupported relationship '%s'", r.SpatialRelationship), "spatial_relationship")
	}
	agg := strings.ToLower(strings.TrimSpace(r.AggregationType))
	if agg == "" {
		agg = "count"
	}
	switch agg {
	case "count", "sum", "average", "avg", "minimum", "min", "maximum", "max", "first", "multiple", "all":
		r.AggregationType = agg
	default:
		return NewValidationError("INVALID_AGGREGATION", fmt.Sprintf("unsupported aggregation_type '%s'", r.AggregationType), "aggregation_type")
	}
	if strings.TrimSpace(r.OutputCRS) == "" {
		r.OutputCRS = "EPSG:4326"
	}
	return nil
}

// Module E — ProximityRequest
type ProximityRequest struct {
	MapID                   string  `json:"map_id"`
	SourceLayerID           string  `json:"source_layer_id"`
	TargetLayerID           string  `json:"target_layer_id"`
	MaximumDistance         float64 `json:"maximum_distance"`
	Unit                    string  `json:"unit"`
	NearestCount            int     `json:"nearest_count"`
	GenerateConnectionLines bool    `json:"generate_connection_lines"`
	Mode                    string  `json:"mode,omitempty"` // proximity, distance_matrix, nearest_facility
	OutputName              string  `json:"output_name"`
	OutputCRS               string  `json:"output_crs"`
	Sync                    bool    `json:"sync,omitempty"`
}

func (r *ProximityRequest) Validate() error {
	if strings.TrimSpace(r.SourceLayerID) == "" {
		return NewValidationError("MISSING_SOURCE_LAYER", "source_layer_id is required", "source_layer_id")
	}
	if strings.TrimSpace(r.TargetLayerID) == "" {
		return NewValidationError("MISSING_TARGET_LAYER", "target_layer_id is required", "target_layer_id")
	}
	if r.MaximumDistance < 0 {
		return NewValidationError("INVALID_MAX_DISTANCE", "maximum_distance cannot be negative", "maximum_distance")
	}
	if r.NearestCount <= 0 {
		r.NearestCount = 1
	}
	if r.NearestCount > 100 {
		r.NearestCount = 100
	}
	if strings.TrimSpace(r.Unit) == "" {
		r.Unit = "meters"
	}
	if strings.TrimSpace(r.OutputCRS) == "" {
		r.OutputCRS = "EPSG:4326"
	}
	return nil
}

// Module F — HeatmapRequest
type HeatmapRequest struct {
	MapID        string  `json:"map_id"`
	InputLayerID string  `json:"input_layer_id"`
	WeightField  string  `json:"weight_field"`
	Radius       float64 `json:"radius"`
	RadiusUnit   string  `json:"radius_unit"`
	PixelSize    float64 `json:"pixel_size"`
	KernelType   string  `json:"kernel_type"` // quartic, gaussian, triangular, uniform, epanechnikov
	ColorRamp    string  `json:"color_ramp"`  // viridis, inferno, magma, plasma, ylorrd, coolwarm
	OutputName   string  `json:"output_name"`
	OutputCRS    string  `json:"output_crs"`
	Sync         bool    `json:"sync,omitempty"`
}

func (r *HeatmapRequest) Validate() error {
	if strings.TrimSpace(r.InputLayerID) == "" {
		return NewValidationError("MISSING_INPUT_LAYER", "input_layer_id is required", "input_layer_id")
	}
	if r.Radius <= 0 {
		r.Radius = 500
	}
	if strings.TrimSpace(r.RadiusUnit) == "" {
		r.RadiusUnit = "meters"
	}
	kernel := strings.ToLower(strings.TrimSpace(r.KernelType))
	if kernel == "" {
		kernel = "quartic"
	}
	switch kernel {
	case "quartic", "gaussian", "triangular", "uniform", "epanechnikov":
		r.KernelType = kernel
	default:
		return NewValidationError("INVALID_KERNEL", "kernel_type must be quartic, gaussian, triangular, uniform, or epanechnikov", "kernel_type")
	}
	if strings.TrimSpace(r.ColorRamp) == "" {
		r.ColorRamp = "inferno"
	}
	if strings.TrimSpace(r.OutputCRS) == "" {
		r.OutputCRS = "EPSG:4326"
	}
	return nil
}

// Module G — Raster Processing Requests
type RasterClipRequest struct {
	MapID           string `json:"map_id"`
	RasterDatasetID string `json:"raster_dataset_id"`
	ClippingLayerID string `json:"clipping_layer_id"`
	OutputName      string `json:"output_name"`
	OutputCRS       string `json:"output_crs"`
	Sync            bool   `json:"sync,omitempty"`
}

func (r *RasterClipRequest) Validate() error {
	if strings.TrimSpace(r.RasterDatasetID) == "" {
		return NewValidationError("MISSING_RASTER_DATASET", "raster_dataset_id is required", "raster_dataset_id")
	}
	if strings.TrimSpace(r.ClippingLayerID) == "" {
		return NewValidationError("MISSING_CLIPPING_LAYER", "clipping_layer_id is required", "clipping_layer_id")
	}
	return nil
}

type RasterReprojectRequest struct {
	MapID           string `json:"map_id"`
	RasterDatasetID string `json:"raster_dataset_id"`
	SourceCRS       string `json:"source_crs"`
	TargetCRS       string `json:"target_crs"`
	Resampling      string `json:"resampling"` // nearest, bilinear, cubic
	OutputName      string `json:"output_name"`
	Sync            bool   `json:"sync,omitempty"`
}

func (r *RasterReprojectRequest) Validate() error {
	if strings.TrimSpace(r.RasterDatasetID) == "" {
		return NewValidationError("MISSING_RASTER_DATASET", "raster_dataset_id is required", "raster_dataset_id")
	}
	if strings.TrimSpace(r.TargetCRS) == "" {
		return NewValidationError("MISSING_TARGET_CRS", "target_crs is required", "target_crs")
	}
	return nil
}

type RasterResampleRequest struct {
	MapID           string  `json:"map_id"`
	RasterDatasetID string  `json:"raster_dataset_id"`
	ScaleFactor     float64 `json:"scale_factor"`
	TargetWidth     int     `json:"target_width"`
	TargetHeight    int     `json:"target_height"`
	Method          string  `json:"method"` // nearest, bilinear, cubic
	OutputName      string  `json:"output_name"`
	Sync            bool    `json:"sync,omitempty"`
}

func (r *RasterResampleRequest) Validate() error {
	if strings.TrimSpace(r.RasterDatasetID) == "" {
		return NewValidationError("MISSING_RASTER_DATASET", "raster_dataset_id is required", "raster_dataset_id")
	}
	if r.ScaleFactor <= 0 && (r.TargetWidth <= 0 || r.TargetHeight <= 0) {
		r.ScaleFactor = 0.5
	}
	return nil
}

type RasterCalculatorRequest struct {
	MapID        string   `json:"map_id"`
	InputRasters []string `json:"input_rasters"`
	Expression   string   `json:"expression"` // e.g., "A + B", "A - B", "A * 2", "(A - B) / (A + B)"
	OutputName   string   `json:"output_name"`
	OutputFormat string   `json:"output_format"`
	OutputCRS    string   `json:"output_crs"`
	Sync         bool     `json:"sync,omitempty"`
}

func (r *RasterCalculatorRequest) Validate() error {
	if len(r.InputRasters) == 0 {
		return NewValidationError("MISSING_INPUT_RASTERS", "at least one raster is required in input_rasters", "input_rasters")
	}
	expr := strings.TrimSpace(r.Expression)
	if expr == "" {
		return NewValidationError("MISSING_EXPRESSION", "expression is required", "expression")
	}
	// Restrict expression characters to safe mathematical tokens, A-Z band identifiers, numbers, and comparison operators
	for _, ch := range expr {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') ||
			strings.ContainsRune(" +-*/()._<>!=?:,", ch)) {
			return NewValidationError("UNSAFE_EXPRESSION", fmt.Sprintf("disallowed character '%c' in raster calculator expression", ch), "expression")
		}
	}
	upper := strings.ToUpper(expr)
	for _, forbidden := range []string{"IMPORT", "EVAL", "EXEC", "OS", "SYS", "OPEN", "__"} {
		if strings.Contains(upper, forbidden) {
			return NewValidationError("UNSAFE_EXPRESSION", "forbidden token in raster expression", "expression")
		}
	}
	return nil
}

type RasterAlignRequest struct {
	MapID           string   `json:"map_id"`
	RasterDatasetID string   `json:"raster_dataset_id"`
	ReferenceID     string   `json:"reference_dataset_id"`
	RasterIDs       []string `json:"raster_dataset_ids,omitempty"`
	TargetCRS       string   `json:"target_crs"`
	OutputName      string   `json:"output_name"`
	Sync            bool     `json:"sync,omitempty"`
}

func (r *RasterAlignRequest) Validate() error {
	if strings.TrimSpace(r.RasterDatasetID) == "" && len(r.RasterIDs) == 0 {
		return NewValidationError("MISSING_RASTER_DATASET", "raster_dataset_id is required", "raster_dataset_id")
	}
	return nil
}

type RasterMosaicRequest struct {
	MapID            string   `json:"map_id"`
	RasterDatasetIDs []string `json:"raster_dataset_ids"`
	TargetCRS        string   `json:"target_crs"`
	Resolution       float64  `json:"resolution"`
	OutputName       string   `json:"output_name"`
	Sync             bool     `json:"sync,omitempty"`
}

func (r *RasterMosaicRequest) Validate() error {
	if len(r.RasterDatasetIDs) < 2 {
		return NewValidationError("INSUFFICIENT_RASTERS", "at least 2 raster_dataset_ids are required for mosaic", "raster_dataset_ids")
	}
	return nil
}

type RasterContoursRequest struct {
	MapID           string  `json:"map_id"`
	RasterDatasetID string  `json:"raster_dataset_id"`
	Interval        float64 `json:"interval"`
	Band            int     `json:"band"`
	OutputName      string  `json:"output_name"`
	Sync            bool    `json:"sync,omitempty"`
}

func (r *RasterContoursRequest) Validate() error {
	if strings.TrimSpace(r.RasterDatasetID) == "" {
		return NewValidationError("MISSING_RASTER_DATASET", "raster_dataset_id is required", "raster_dataset_id")
	}
	if r.Interval <= 0 {
		r.Interval = 25.0
	}
	if r.Band <= 0 {
		r.Band = 1
	}
	return nil
}

type RasterHillshadeRequest struct {
	MapID           string  `json:"map_id"`
	RasterDatasetID string  `json:"raster_dataset_id"`
	Azimuth         float64 `json:"azimuth"`
	Altitude        float64 `json:"altitude"`
	ZFactor         float64 `json:"z_factor"`
	OutputName      string  `json:"output_name"`
	Sync            bool    `json:"sync,omitempty"`
}

func (r *RasterHillshadeRequest) Validate() error {
	if strings.TrimSpace(r.RasterDatasetID) == "" {
		return NewValidationError("MISSING_RASTER_DATASET", "raster_dataset_id is required", "raster_dataset_id")
	}
	if r.Azimuth <= 0 || r.Azimuth > 360 {
		r.Azimuth = 315.0
	}
	if r.Altitude <= 0 || r.Altitude > 90 {
		r.Altitude = 45.0
	}
	if r.ZFactor <= 0 {
		r.ZFactor = 1.0
	}
	return nil
}

// Module H — TopologyValidationRequest
type TopologyValidationRequest struct {
	MapID                  string   `json:"map_id"`
	LayerID                string   `json:"layer_id"`
	BoundaryLayerID        string   `json:"boundary_layer_id,omitempty"`
	RuleIDs                []string `json:"rule_ids,omitempty"`
	CheckOverlaps          bool     `json:"check_overlaps"`
	CheckGaps              bool     `json:"check_gaps"`
	CheckSelfIntersections bool     `json:"check_self_intersections"`
	CheckDuplicates        bool     `json:"check_duplicates"`
	CheckDisconnectedLines bool     `json:"check_disconnected_lines"`
	CheckEmptyOrInvalid    bool     `json:"check_empty_or_invalid"`
	OutputErrorLayer       bool     `json:"output_error_layer"`
	OutputName             string   `json:"output_name,omitempty"`
	Sync                   bool     `json:"sync,omitempty"`
}

func (r *TopologyValidationRequest) Validate() error {
	if strings.TrimSpace(r.LayerID) == "" {
		return NewValidationError("MISSING_LAYER_ID", "layer_id is required", "layer_id")
	}
	if !r.CheckOverlaps && !r.CheckGaps && !r.CheckSelfIntersections && !r.CheckDuplicates && !r.CheckDisconnectedLines && !r.CheckEmptyOrInvalid {
		r.CheckOverlaps = true
		r.CheckGaps = true
		r.CheckSelfIntersections = true
		r.CheckDuplicates = true
		r.CheckDisconnectedLines = true
		r.CheckEmptyOrInvalid = true
	}
	return nil
}

type TopologyFixRequest struct {
	ErrorIDs []string `json:"error_ids,omitempty"`
	FixAll   bool     `json:"fix_all"`
}
