package models

import (
	"errors"
	"time"
)

var (
	ErrPublishedMapNotFound     = errors.New("published map not found")
	ErrPublishedMapRevoked      = errors.New("published map has been revoked")
	ErrPublishedMapAccessDenied = errors.New("access denied to published map")
	ErrInvalidPublishRequest    = errors.New("invalid publish request")
)

const (
	PublishModeCombined = "combined"
	PublishModeRaster   = "raster"
	PublishModeVector   = "vector"
)

// PublishedRasterLayer stores metadata and persistent tile directory for one published raster layer.
type PublishedRasterLayer struct {
	LayerID      string      `json:"layer_id"`
	DatasetID    string      `json:"dataset_id,omitempty"`
	Name         string      `json:"name"`
	Format       string      `json:"format"` // "png" or "jpg"
	TileDir      string      `json:"-"`      // internal disk directory (never exposed in public JSON)
	ImagePath    string      `json:"-"`      // internal source/processed raster path for dynamic z0-z22 rendering
	TileURL      string      `json:"tile_url"`
	MetadataURL  string      `json:"metadata_url"`
	SourceCRS    string      `json:"source_crs"`
	Bounds       []float64   `json:"bounds,omitempty"`
	Corners      [][]float64 `json:"corners,omitempty"` // optional 4 WGS84 [lng, lat] corners [TL, TR, BR, BL] for rotated affine rasters
	MinZoom      int         `json:"min_zoom"`
	MaxZoom      int         `json:"max_zoom"`
	Opacity      float64     `json:"opacity"`
	Visible      bool        `json:"visible"`
	DisplayOrder int         `json:"display_order"`
}

// PublishedVectorLayer stores metadata, style, and persistent MVT directory for one published vector layer.
type PublishedVectorLayer struct {
	LayerID       string                   `json:"layer_id"`
	DatasetID     string                   `json:"dataset_id,omitempty"`
	GISLayerID    string                   `json:"gis_layer_id,omitempty"`
	Name          string                   `json:"name"`
	SourceLayer   string                   `json:"source_layer"`
	Format        string                   `json:"format"` // "pbf"
	TileDir       string                   `json:"-"`      // internal disk directory (never exposed in public JSON)
	TileURL       string                   `json:"tile_url"`
	MetadataURL   string                   `json:"metadata_url"`
	SourceCRS     string                   `json:"source_crs"`
	GeometryType  string                   `json:"geometry_type"`
	GeometryTypes []string                 `json:"geometry_types,omitempty"`
	FeatureCount  int                      `json:"feature_count"`
	Bounds        []float64                `json:"bounds,omitempty"`
	MinZoom       int                      `json:"min_zoom"`
	MaxZoom       int                      `json:"max_zoom"`
	Opacity       float64                  `json:"opacity"`
	Visible       bool                     `json:"visible"`
	DisplayOrder  int                      `json:"display_order"`
	PromoteID     string                   `json:"promote_id"`
	Interactive   bool                     `json:"interactive"`
	Fields        []map[string]interface{} `json:"fields,omitempty"`
	Style         map[string]interface{}   `json:"style,omitempty"`
}

// PublishedLayerStackItem identifies a single raster or vector layer in exact top-to-bottom Photoshop stack order.
type PublishedLayerStackItem struct {
	Kind string `json:"kind"`           // "raster" or "vector"
	ID   string `json:"id,omitempty"`   // dataset_id, gis_layer_id, or published layer_id
	Name string `json:"name,omitempty"` // source_layer or filename fallback
}

// PublishedMap represents a published Combined, Raster-only, or Vector-only web tile map.
type PublishedMap struct {
	MapID             string                    `json:"map_id"`
	OwnerID           string                    `json:"owner_id"`
	ProjectID         string                    `json:"project_id,omitempty"`
	WorkspaceID       string                    `json:"workspace_id,omitempty"`
	Name              string                    `json:"name"`
	Mode              string                    `json:"mode"` // "combined", "raster", "vector"
	IsPublic          bool                      `json:"is_public"`
	AccessToken       string                    `json:"access_token,omitempty"`
	Revoked           bool                      `json:"revoked"`
	RasterLayerIDs    []string                  `json:"raster_layer_ids"`
	VectorLayerIDs    []string                  `json:"vector_layer_ids"`
	RasterLayers      []PublishedRasterLayer    `json:"raster_layers"`
	VectorLayers      []PublishedVectorLayer    `json:"vector_layers"`
	LayerOrder        []string                  `json:"layer_order"`
	LayerStack        []PublishedLayerStackItem `json:"layer_stack,omitempty"` // exact top-to-bottom interleaved order
	Bounds            []float64                 `json:"bounds,omitempty"`
	Center            []float64                 `json:"center,omitempty"`
	InitialZoom       int                       `json:"initial_zoom"`
	MinZoom           int                       `json:"min_zoom"`
	MaxZoom           int                       `json:"max_zoom"`
	InteractionConfig map[string]interface{}    `json:"interaction_config,omitempty"`
	MapURL            string                    `json:"map_url"`
	StyleURL          string                    `json:"style_url"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`
	RevokedAt         *time.Time                `json:"revoked_at,omitempty"`
}

// PublishMapRequest defines the payload for POST /api/publish.
type PublishMapRequest struct {
	WorkspaceID    string                    `json:"workspace_id,omitempty"`
	ProjectID      string                    `json:"project_id,omitempty"`
	DatasetID      string                    `json:"dataset_id,omitempty"`
	Name           string                    `json:"name,omitempty"`
	Mode           string                    `json:"mode"` // "combined", "raster", "vector"
	IsPublic       *bool                     `json:"is_public,omitempty"`
	RasterLayerIDs []string                  `json:"raster_layer_ids,omitempty"`
	VectorLayerIDs []string                  `json:"vector_layer_ids,omitempty"`
	LayerIDs       []string                  `json:"layer_ids,omitempty"`
	RasterFormat   string                    `json:"raster_format,omitempty"`
	MinZoom        *int                      `json:"min_zoom,omitempty"`
	MaxZoom        *int                      `json:"max_zoom,omitempty"`
	InitialZoom    *int                      `json:"initial_zoom,omitempty"`
	Center         []float64                 `json:"center,omitempty"`
	LayerOrder     []string                  `json:"layer_order,omitempty"` // group fallback order
	LayerStack     []PublishedLayerStackItem `json:"layer_stack,omitempty"` // exact top-to-bottom per-layer order
}
