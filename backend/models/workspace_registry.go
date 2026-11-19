package models

import (
	"errors"
	"time"
)

var (
	ErrWorkspaceLayerNotFound  = errors.New("workspace layer not found")
	ErrWorkspaceRecordNotFound = errors.New("workspace record not found")
	ErrWorkspaceRasterNotFound = errors.New("workspace raster not found")
	ErrInvalidWorkspaceRequest = errors.New("invalid workspace layer/record request")
)

const (
	RecordTypeAll          = "all"
	RecordTypeVector       = "vector"
	RecordTypeRaster       = "raster"
	RecordTypeControlPoint = "control_point"
	RecordTypeUserIcon     = "user_icon"
)

// WorkspaceDataset tracks an uploaded source file associated with a workspace without duplicating it.
type WorkspaceDataset struct {
	ID               string    `json:"id"`
	WorkspaceID      string    `json:"workspace_id"`
	UserID           string    `json:"user_id"`
	SourceDatasetID  string    `json:"source_dataset_id"`
	OriginalFilename string    `json:"original_filename"`
	StoredFilePath   string    `json:"stored_file_path,omitempty"`
	Format           string    `json:"format"`
	FileSize         int64     `json:"file_size"`
	UploadStatus     string    `json:"upload_status"`
	CreatedAt        time.Time `json:"created_at"`
}

// WorkspaceLayer represents an independently manageable layer imported from a GeoPackage or other GIS dataset.
type WorkspaceLayer struct {
	ID                 string                   `json:"id"`
	WorkspaceDatasetID string                   `json:"workspace_dataset_id"`
	SourceDatasetID    string                   `json:"source_dataset_id,omitempty"`
	WorkspaceID        string                   `json:"workspace_id"`
	GISLayerID         string                   `json:"gis_layer_id,omitempty"`
	SourceLayerName    string                   `json:"source_layer_name"`
	DisplayName        string                   `json:"display_name"`
	GeometryType       string                   `json:"geometry_type"`
	SourceCRS          string                   `json:"source_crs"`
	FeatureCount       int                      `json:"feature_count"`
	ExtentMinX         *float64                 `json:"extent_min_x,omitempty"`
	ExtentMinY         *float64                 `json:"extent_min_y,omitempty"`
	ExtentMaxX         *float64                 `json:"extent_max_x,omitempty"`
	ExtentMaxY         *float64                 `json:"extent_max_y,omitempty"`
	Extent             []float64                `json:"extent,omitempty"`
	Fields             []map[string]interface{} `json:"fields,omitempty"`
	RenderingSource    map[string]interface{}   `json:"rendering_source,omitempty"`
	Visibility         bool                     `json:"visibility"`
	Opacity            float64                  `json:"opacity"`
	LayerOrder         int                      `json:"layer_order"`
	Status             string                   `json:"status"`
	CreatedAt          time.Time                `json:"created_at"`
	UpdatedAt          time.Time                `json:"updated_at"`
}

// WorkspaceRecord is a unified row in the centralized Workspace Records data table.
type WorkspaceRecord struct {
	ID               string                 `json:"id"`
	WorkspaceID      string                 `json:"workspace_id"`
	WorkspaceLayerID string                 `json:"workspace_layer_id,omitempty"`
	LayerName        string                 `json:"layer_name,omitempty"`
	RecordType       string                 `json:"record_type"`
	SourceFeatureID  string                 `json:"source_feature_id,omitempty"`
	DatasetID        string                 `json:"dataset_id,omitempty"`
	DisplayName      string                 `json:"display_name"`
	GeometryType     string                 `json:"geometry_type,omitempty"`
	Coordinates      []float64              `json:"coordinates,omitempty"`
	Bounds           []float64              `json:"bounds,omitempty"`
	CRS              string                 `json:"crs,omitempty"`
	Visibility       bool                   `json:"visibility"`
	SourceReference  string                 `json:"source_reference,omitempty"`
	Status           string                 `json:"status"`
	Attributes       map[string]interface{} `json:"attributes,omitempty"`
	// Specialized metadata for Rasters, Control Points, and User Icons
	Width         int       `json:"width,omitempty"`
	Height        int       `json:"height,omitempty"`
	Resolution    string    `json:"resolution,omitempty"`
	ImageX        *float64  `json:"image_x,omitempty"`
	ImageY        *float64  `json:"image_y,omitempty"`
	GeographicX   *float64  `json:"geographic_x,omitempty"`
	GeographicY   *float64  `json:"geographic_y,omitempty"`
	PairingStatus string    `json:"pairing_status,omitempty"`
	IconType      string    `json:"icon_type,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// WorkspaceRaster tracks uploaded raster datasets and their spatial metadata in a workspace.
type WorkspaceRaster struct {
	ID               string    `json:"id"`
	WorkspaceID      string    `json:"workspace_id"`
	WorkspaceLayerID string    `json:"workspace_layer_id,omitempty"`
	DatasetID        string    `json:"dataset_id"`
	DisplayName      string    `json:"display_name"`
	LayerName        string    `json:"layer_name,omitempty"`
	RasterPath       string    `json:"raster_path,omitempty"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	BandCount        int       `json:"band_count"`
	SourceCRS        string    `json:"source_crs"`
	ResolutionX      *float64  `json:"resolution_x,omitempty"`
	ResolutionY      *float64  `json:"resolution_y,omitempty"`
	Bounds           []float64 `json:"bounds,omitempty"`
	Visibility       bool      `json:"visibility"`
	Opacity          float64   `json:"opacity"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// WorkspaceRecordColumn describes a dynamic column available in the Workspace Records table.
type WorkspaceRecordColumn struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Tooltip     string `json:"tooltip,omitempty"`
	DefaultShow bool   `json:"default_show"`
	Dynamic     bool   `json:"dynamic,omitempty"`
}

// WorkspaceRecordCounts holds tab totals for the Workspace Records interface.
type WorkspaceRecordCounts struct {
	All           int `json:"all"`
	Vectors       int `json:"vectors"`
	Rasters       int `json:"rasters"`
	ControlPoints int `json:"control_points"`
	UserIcons     int `json:"user_icons"`
}

// Request models (Section 8 of specification)

type InspectDatasetRequest struct {
	FileID      string `json:"file_id"`
	DatasetID   string `json:"dataset_id"`
	WorkspaceID string `json:"workspace_id"`
}

type AddDatasetLayersRequest struct {
	DatasetID          string   `json:"dataset_id"`
	WorkspaceID        string   `json:"workspace_id"`
	SelectedLayerNames []string `json:"selected_layer_names"`
}

type UpdateWorkspaceLayerRequest struct {
	DisplayName *string  `json:"display_name,omitempty"`
	Visibility  *bool    `json:"visibility,omitempty"`
	Opacity     *float64 `json:"opacity,omitempty"`
	LayerOrder  *int     `json:"layer_order,omitempty"`
}

type ReorderWorkspaceLayersRequest struct {
	LayerIDs []string `json:"layer_ids"`
}

type BatchWorkspaceVisibilityRequest struct {
	LayerIDs   []string `json:"layer_ids"`
	Visibility bool     `json:"visibility"`
}

type WorkspaceRecordsQuery struct {
	RecordType    string   `json:"record_type"`
	LayerID       string   `json:"layer_id"`
	Search        string   `json:"search"`
	Status        string   `json:"status"`
	Page          int      `json:"page"`
	PageSize      int      `json:"page_size"`
	SortBy        string   `json:"sort_by"`
	SortDirection string   `json:"sort_direction"`
	Columns       []string `json:"columns"`
}

type UpdateWorkspaceRecordRequest struct {
	DisplayName        *string                `json:"display_name,omitempty"`
	Visibility         *bool                  `json:"visibility,omitempty"`
	EditableAttributes map[string]interface{} `json:"editable_attributes,omitempty"`
}

type ExportWorkspaceRecordsRequest struct {
	Format     string   `json:"format"` // "csv" | "geojson"
	RecordType string   `json:"record_type"`
	LayerID    string   `json:"layer_id"`
	RecordIDs  []string `json:"record_ids"`
	Columns    []string `json:"columns"`
}

type UpdateWorkspaceRasterRequest struct {
	DisplayName *string  `json:"display_name,omitempty"`
	Visibility  *bool    `json:"visibility,omitempty"`
	Opacity     *float64 `json:"opacity,omitempty"`
}
