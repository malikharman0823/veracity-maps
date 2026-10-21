package models

import "time"

// IconPoint represents a customizable geographic marker associated with an icon.
type IconPoint struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"session_id"`
	DatasetID   string                 `json:"dataset_id,omitempty"`
	BuildingID  string                 `json:"building_id,omitempty"`
	FloorLevel  int                    `json:"floor_level,omitempty"`
	FloorLabel  string                 `json:"floor_label,omitempty"`
	Scope       string                 `json:"scope,omitempty"` // "building", "floor", "dataset"
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	IconID      string                 `json:"icon_id"`
	IconPath    string                 `json:"icon_path,omitempty"`
	Longitude   float64                `json:"longitude"`
	Latitude    float64                `json:"latitude"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}
