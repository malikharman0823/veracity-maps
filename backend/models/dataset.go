package models

import (
	"time"
)

type Dataset struct {
	ID                 string                 `json:"id"`
	SessionID          string                 `json:"session_id"`
	Filename           string                 `json:"filename"`
	FileSize           int64                  `json:"file_size"`
	ContentType        string                 `json:"content_type"`
	FileHash           string                 `json:"file_hash"`
	StoragePath        string                 `json:"storage_path"`
	GeoreferencedPath  string                 `json:"georeferenced_path,omitempty"`
	ProcessedPath      string                 `json:"processed_path,omitempty"`
	GeoreferenceStatus string                 `json:"georeference_status"`
	TileStatus         string                 `json:"tile_status"`
	TilePath           string                 `json:"tile_path,omitempty"`
	ExtractedMetadata  map[string]interface{} `json:"extracted_metadata"`
	MapLayer           map[string]interface{} `json:"map_layer,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

type MapProject struct {
	ID            string                 `json:"id"`
	DatasetID     string                 `json:"dataset_id"`
	Name          string                 `json:"name"`
	Configuration map[string]interface{} `json:"configuration"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}
