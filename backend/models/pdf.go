package models

type PdfPageInfo struct {
	PageNumber        int                 `json:"page_number"`
	Width             int                 `json:"width"`
	Height            int                 `json:"height"`
	WidthPt           float64             `json:"width_pt,omitempty"`
	HeightPt          float64             `json:"height_pt,omitempty"`
	Rotation          int                 `json:"rotation"`
	HasGeoreference   bool                `json:"has_georeference"`
	ThumbnailFilename string              `json:"thumbnail_filename"`
	ProcessingPath    string              `json:"processing_path,omitempty"` // "vector" | "raster" | "hybrid"
	VectorPathCount   int                 `json:"vector_path_count"`
	TextSpanCount     int                 `json:"text_span_count"`
	ImageCount        int                 `json:"image_count"`
	Fonts             []string            `json:"fonts,omitempty"`
	EstimatedDPI      int                 `json:"estimated_dpi,omitempty"`
	RecommendedDPI    int                 `json:"recommended_dpi,omitempty"`
	OcrRecommended    bool                `json:"ocr_recommended"`
	Content           *ContentComposition `json:"content,omitempty"`
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
	DPI                int    `json:"dpi,omitempty"`
	EnhancePreset      string `json:"enhance_preset,omitempty"`
	ExtractVectors     *bool  `json:"extract_vectors,omitempty"`
	RunOCR             bool   `json:"run_ocr,omitempty"`
}

type ImportPdfPagesResponse struct {
	Success            bool       `json:"success"`
	DocumentID         string     `json:"document_id"`
	GeoreferencingMode string     `json:"georeferencing_mode"`
	BuildingID         string     `json:"building_id,omitempty"`
	Datasets           []*Dataset `json:"datasets"`
}
