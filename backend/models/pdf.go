package models

type PdfPageInfo struct {
	PageNumber        int    `json:"page_number"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	Rotation          int    `json:"rotation"`
	HasGeoreference   bool   `json:"has_georeference"`
	ThumbnailFilename string `json:"thumbnail_filename"`
}

type PdfInspectionResult struct {
	DocumentID  string         `json:"document_id"`
	Filename    string         `json:"filename"`
	FileSize    int64          `json:"file_size"`
	PageCount   int            `json:"page_count"`
	Pages       []*PdfPageInfo `json:"pages"`
	StoragePath string         `json:"storage_path,omitempty"`
}

type ImportPdfPagesRequest struct {
	DocumentID         string `json:"document_id"`
	SelectedPages      []int  `json:"selected_pages"`
	GeoreferencingMode string `json:"georeferencing_mode"` // "shared" | "independent"
	BuildingID         string `json:"building_id,omitempty"`
	BuildingName       string `json:"building_name,omitempty"`
	FloorID            string `json:"floor_id,omitempty"`
}

type ImportPdfPagesResponse struct {
	Success            bool       `json:"success"`
	DocumentID         string     `json:"document_id"`
	GeoreferencingMode string     `json:"georeferencing_mode"`
	BuildingID         string     `json:"building_id,omitempty"`
	Datasets           []*Dataset `json:"datasets"`
}
