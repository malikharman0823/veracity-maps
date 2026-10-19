package models

type Layer struct {
	ID        string    `json:"id"`
	DatasetID string    `json:"dataset_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // "raster" or "vector"
	Visible   bool      `json:"visible"`
	Opacity   float64   `json:"opacity"`
	Bounds    []float64 `json:"bounds,omitempty"` // [west, south, east, north]
}
