package models

import (
	"time"
)

type ControlPoint struct {
	ID        string    `json:"id"`
	DatasetID string    `json:"dataset_id"`
	ImageX    float64   `json:"image_x"`
	ImageY    float64   `json:"image_y"`
	Longitude float64   `json:"longitude"`
	Latitude  float64   `json:"latitude"`
	CreatedAt time.Time `json:"created_at"`
}
