package models

import "time"

// BuildingFloor represents a specific floor within a building.
type BuildingFloor struct {
	ID           string   `json:"id"`
	BuildingID   string   `json:"building_id"`
	Level        int      `json:"level"`
	Name         string   `json:"name"`
	DatasetIDs   []string `json:"dataset_ids,omitempty"`
	DisplayOrder int      `json:"display_order"`
}

// Building represents a structural entity containing one or more floor plans.
type Building struct {
	ID                   string                 `json:"id"`
	SessionID            string                 `json:"session_id"`
	Name                 string                 `json:"name"`
	Description          string                 `json:"description,omitempty"`
	Floors               []*BuildingFloor       `json:"floors,omitempty"`
	DefaultFloorLevel    int                    `json:"default_floor_level"`
	ActiveFloorID        string                 `json:"active_floor_id,omitempty"`
	ControlPoints        []*ControlPoint        `json:"control_points,omitempty"`
	SharedTransform      []float64              `json:"shared_transform,omitempty"`
	SharedRasterGeometry map[string]interface{} `json:"shared_raster_geometry,omitempty"`
	Bounds               map[string]interface{} `json:"bounds,omitempty"`
	DisplayOrder         int                    `json:"display_order"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}
