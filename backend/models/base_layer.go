package models

import (
	"errors"
	"time"
)

var (
	ErrBaseLayerNotFound       = errors.New("base layer not found")
	ErrBaseLayerAccessDenied   = errors.New("base layer access denied")
	ErrBaseLayerNotGeoref      = errors.New("dataset must be georeferenced with a valid CRS before registering as a base layer")
	ErrInvalidMapViewType      = errors.New("unsupported map view type")
	ErrInvalidBaseLayerRequest = errors.New("invalid base layer request")
)

const (
	BaseLayerTypeWorldMap = "world_map"
	BaseLayerTypeCustom   = "custom"

	BaseLayerStatusReady       = "ready"
	BaseLayerStatusNeedsGeoref = "needs_georeferencing"
	BaseLayerStatusUnavailable = "unavailable"
	BaseLayerStatusError       = "error"

	MapViewWorld     = "world"
	MapViewSatellite = "satellite"
	MapViewStreets   = "streets"
	MapViewTerrain   = "terrain"
	MapViewLight     = "light"
	MapViewDark      = "dark"
	MapViewCustom    = "custom"
)

// BaseLayer represents a registered custom base layer or the system Default World Map.
type BaseLayer struct {
	ID              string                 `json:"id"`
	UserID          string                 `json:"user_id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	DatasetID       string                 `json:"dataset_id,omitempty"`
	DatasetName     string                 `json:"dataset_name,omitempty"`
	SourceType      string                 `json:"source_type"`
	SourceCRS       string                 `json:"source_crs"`
	ExtentMinX      *float64               `json:"extent_min_x,omitempty"`
	ExtentMinY      *float64               `json:"extent_min_y,omitempty"`
	ExtentMaxX      *float64               `json:"extent_max_x,omitempty"`
	ExtentMaxY      *float64               `json:"extent_max_y,omitempty"`
	Extent          []float64              `json:"extent,omitempty"`
	PreviewPath     string                 `json:"preview_path,omitempty"`
	RenderingSource map[string]interface{} `json:"rendering_source,omitempty"`
	Status          string                 `json:"status"`
	IsDefault       bool                   `json:"is_default"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// WorkspaceBaseLayer stores the selected base layer for a workspace/map.
type WorkspaceBaseLayer struct {
	ID                  string     `json:"id"`
	WorkspaceID         string     `json:"workspace_id"`
	BaseLayerID         string     `json:"base_layer_id,omitempty"`
	BaseLayerType       string     `json:"base_layer_type"`
	SelectedView        string     `json:"selected_view"`
	Opacity             float64    `json:"opacity"`
	Visibility          bool       `json:"visibility"`
	BaseLayer           *BaseLayer `json:"base_layer,omitempty"`
	PreviousUnavailable bool       `json:"previous_unavailable,omitempty"`
	UnavailableWarning  string     `json:"unavailable_warning,omitempty"`
	HasSavedSelection   bool       `json:"has_saved_selection"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// MapViewPreferences stores per-workspace background map view and filter settings.
type MapViewPreferences struct {
	ID                 string                 `json:"id"`
	WorkspaceID        string                 `json:"workspace_id"`
	ViewType           string                 `json:"view_type"`
	StyleConfiguration map[string]interface{} `json:"style_configuration,omitempty"`
	ShowLabels         bool                   `json:"show_labels"`
	ShowRoads          bool                   `json:"show_roads"`
	ShowPlaces         bool                   `json:"show_places"`
	BaseLayerOpacity   float64                `json:"base_layer_opacity"`
	BaseLayerVisible   bool                   `json:"base_layer_visible"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// MapViewCapabilities describes which provider-dependent filters a map view supports.
type MapViewCapabilities struct {
	SupportsLabels  bool `json:"supports_labels"`
	SupportsRoads   bool `json:"supports_roads"`
	SupportsPlaces  bool `json:"supports_places"`
	SupportsOpacity bool `json:"supports_opacity"`
}

// MapViewDefinition describes a configured, legally usable map view style.
type MapViewDefinition struct {
	Type           string              `json:"type"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	TileURL        string              `json:"tile_url,omitempty"`
	LabelsTileURL  string              `json:"labels_tile_url,omitempty"`
	RoadsTileURL   string              `json:"roads_tile_url,omitempty"`
	PlacesTileURL  string              `json:"places_tile_url,omitempty"`
	Attribution    string              `json:"attribution,omitempty"`
	MinZoom        int                 `json:"min_zoom"`
	MaxZoom        int                 `json:"max_zoom"`
	Available      bool                `json:"available"`
	Capabilities   MapViewCapabilities `json:"capabilities"`
	PreviewPalette []string            `json:"preview_palette,omitempty"`
}

// BaseLayerValidationResult is returned by POST /api/base-layers/:id/validate.
type BaseLayerValidationResult struct {
	BaseLayerID     string    `json:"base_layer_id"`
	IsValid         bool      `json:"is_valid"`
	IsGeoreferenced bool      `json:"is_georeferenced"`
	SourceCRS       string    `json:"source_crs"`
	Extent          []float64 `json:"extent,omitempty"`
	RenderMode      string    `json:"render_mode,omitempty"`
	Status          string    `json:"status"`
	Issues          []string  `json:"issues,omitempty"`
}

// BaseLayerPreviewResponse is returned by GET /api/base-layers/:id/preview.
type BaseLayerPreviewResponse struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	SourceType      string                 `json:"source_type"`
	SourceCRS       string                 `json:"source_crs"`
	Extent          []float64              `json:"extent,omitempty"`
	PreviewURL      string                 `json:"preview_url,omitempty"`
	RenderingSource map[string]interface{} `json:"rendering_source,omitempty"`
	Status          string                 `json:"status"`
	IsDefault       bool                   `json:"is_default"`
}

// Request models (Section 8 of specification)

type CreateBaseLayerRequest struct {
	DatasetID   string `json:"dataset_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SourceType  string `json:"source_type"`
}

type UpdateBaseLayerRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SelectWorkspaceBaseLayerRequest struct {
	BaseLayerType string   `json:"base_layer_type"`
	BaseLayerID   string   `json:"base_layer_id"`
	SelectedView  string   `json:"selected_view,omitempty"`
	Opacity       *float64 `json:"opacity,omitempty"`
	Visibility    *bool    `json:"visibility,omitempty"`
}

type UpdateMapViewRequest struct {
	ViewType           string                 `json:"view_type"`
	ShowLabels         *bool                  `json:"show_labels,omitempty"`
	ShowRoads          *bool                  `json:"show_roads,omitempty"`
	ShowPlaces         *bool                  `json:"show_places,omitempty"`
	Opacity            *float64               `json:"opacity,omitempty"`
	BaseLayerVisible   *bool                  `json:"base_layer_visible,omitempty"`
	StyleConfiguration map[string]interface{} `json:"style_configuration,omitempty"`
}
