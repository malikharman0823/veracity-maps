package models

import "time"

// ContentComposition describes whether an uploaded input contains raster imagery,
// vector graphics, embedded text, or a hybrid combination.
type ContentComposition struct {
	HasRaster       bool   `json:"has_raster"`
	HasVector       bool   `json:"has_vector"`
	HasEmbeddedText bool   `json:"has_embedded_text"`
	Classification  string `json:"classification"` // "raster_imagery" | "engineering_drawing" | "scanned_document" | "vector_pdf" | "hybrid_pdf" | "cad_vector" | "categorical_raster" | "mbtiles"
}

// QualityMetrics captures non-destructive image quality measurements from OpenCV.
type QualityMetrics struct {
	BlurScore             float64 `json:"blur_score"`
	BlurStatus            string  `json:"blur_status"` // "sharp" | "moderate" | "blurry"
	ContrastScore         float64 `json:"contrast_score"`
	DynamicRange          float64 `json:"dynamic_range"`
	NoiseEstimate         float64 `json:"noise_estimate"`
	TextAnnotationDensity float64 `json:"text_annotation_density"`
	TextDensityLevel      string  `json:"text_density_level"` // "low" | "medium" | "high"
	IsGrayscaleLike       bool    `json:"is_grayscale_like"`
	RecommendedResampling string  `json:"recommended_resampling"` // "lanczos" | "cubic" | "bilinear" | "nearest"
}

// EnhancementRecommendation advises the user on appropriate enhancement settings.
type EnhancementRecommendation struct {
	RecommendedPreset  string `json:"recommended_preset"` // "none" | "balanced" | "text_clarity" | "line_preservation" | "noise_reduction" | "scanned_drawing"
	Reason             string `json:"reason"`
	UpscaleRecommended bool   `json:"upscale_recommended"`
	SuggestedUpscale   int    `json:"suggested_upscale"` // 1, 2, or 4
	OcrRecommended     bool   `json:"ocr_recommended"`
	IsHighResolution   bool   `json:"is_high_resolution"`
}

// EnhancementOptions defines configurable OpenCV + Real-ESRGAN + OCR parameters.
type EnhancementOptions struct {
	Action            string  `json:"action,omitempty"` // "apply" | "preview" | "reset" | "set_variant"
	Preset            string  `json:"preset"`           // "balanced" | "text_clarity" | "line_preservation" | "noise_reduction" | "scanned_drawing" | "custom"
	Denoise           *bool   `json:"denoise,omitempty"`
	DenoiseStrength   float64 `json:"denoise_strength,omitempty"`
	Contrast          *bool   `json:"contrast,omitempty"`
	CLAHE             *bool   `json:"clahe,omitempty"`
	ClaheClipLimit    float64 `json:"clahe_clip_limit,omitempty"`
	SharpenText       *bool   `json:"sharpen_text,omitempty"`
	SharpenAmount     float64 `json:"sharpen_amount,omitempty"`
	PreserveLines     *bool   `json:"preserve_lines,omitempty"`
	AdaptiveThreshold bool    `json:"adaptive_threshold,omitempty"`
	Grayscale         bool    `json:"grayscale,omitempty"`
	UpscaleFactor     int     `json:"upscale_factor,omitempty"` // 1 (default), 2, or 4
	ForceUpscale      bool    `json:"force_upscale,omitempty"`
	RunOCR            bool    `json:"run_ocr,omitempty"`
	Resampling        string  `json:"resampling,omitempty"`     // "lanczos" | "cubic" | "bilinear" | "nearest"
	ActiveVariant     string  `json:"active_variant,omitempty"` // "enhanced" | "original"
}

// EnhancementState records applied enhancement metadata on a Dataset without modifying StoragePath.
type EnhancementState struct {
	Enabled               bool               `json:"enabled"`
	ActiveVariant         string             `json:"active_variant"` // "enhanced" | "original"
	Preset                string             `json:"preset"`
	EnhancedPath          string             `json:"enhanced_path,omitempty"`
	OriginalWidth         int                `json:"original_width"`
	OriginalHeight        int                `json:"original_height"`
	EnhancedWidth         int                `json:"enhanced_width"`
	EnhancedHeight        int                `json:"enhanced_height"`
	ScaleRatio            float64            `json:"scale_ratio"`
	AIUpscaled            bool               `json:"ai_upscaled"`
	AIUpscaleFactor       int                `json:"ai_upscale_factor,omitempty"`
	AIUpscaleEngine       string             `json:"ai_upscale_engine,omitempty"`
	AIVerifiedEngineering bool               `json:"ai_verified_engineering"`
	AIDisclaimer          string             `json:"ai_disclaimer,omitempty"`
	SkippedHighResUpscale bool               `json:"skipped_high_res_upscale,omitempty"`
	BlurScoreBefore       float64            `json:"blur_score_before"`
	BlurScoreAfter        float64            `json:"blur_score_after"`
	OperationsApplied     []string           `json:"operations_applied"`
	Options               EnhancementOptions `json:"options"`
	UpdatedAt             string             `json:"updated_at"`
}

// AnnotationStyle controls MapLibre symbol rendering for an extracted/OCR label.
type AnnotationStyle struct {
	TextColor string  `json:"text_color,omitempty"`
	HaloColor string  `json:"halo_color,omitempty"`
	TextSize  float64 `json:"text_size,omitempty"`
}

// DatasetAnnotation represents an OCR-detected or PDF/CAD-extracted vector text label.
// Stored separately from original raster pixels; rendered as sharp MapLibre symbol layers.
type DatasetAnnotation struct {
	ID           string           `json:"id"`
	DatasetID    string           `json:"dataset_id"`
	Text         string           `json:"text"`
	OriginalText string           `json:"original_text,omitempty"`
	Category     string           `json:"category"`   // "dimension" | "elevation_level" | "pipe_culvert" | "road_label" | "general"
	Source       string           `json:"source"`     // "paddleocr" | "pdf_vector" | "cad_vector" | "manual"
	Confidence   float64          `json:"confidence"` // 0.0 - 1.0
	ImageX       float64          `json:"image_x"`
	ImageY       float64          `json:"image_y"`
	BBoxPx       []float64        `json:"bbox_px,omitempty"` // [x0, y0, x1, y1] in original image pixel space
	Rotation     float64          `json:"rotation,omitempty"`
	FontName     string           `json:"font_name,omitempty"`
	FontSize     float64          `json:"font_size,omitempty"`
	CADLayer     string           `json:"cad_layer,omitempty"`
	Longitude    *float64         `json:"longitude,omitempty"`
	Latitude     *float64         `json:"latitude,omitempty"`
	HasGeo       bool             `json:"has_geo"`
	Verified     bool             `json:"verified"`
	Visible      bool             `json:"visible"`
	Style        *AnnotationStyle `json:"style,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// ProjectLayerPreview summarizes a single layer inside an inspected .mbtiles or .zip package.
type ProjectLayerPreview struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"` // "mbtiles" | "raster" | "vector"
	TileFormat    string    `json:"tile_format,omitempty"`
	BuildingName  string    `json:"building_name,omitempty"`
	FloorLevel    int       `json:"floor_level"`
	FloorLabel    string    `json:"floor_label,omitempty"`
	Visible       bool      `json:"visible"`
	Opacity       float64   `json:"opacity"`
	MinZoom       int       `json:"min_zoom"`
	MaxZoom       int       `json:"max_zoom"`
	Bounds        []float64 `json:"bounds,omitempty"`
	HasMBTiles    bool      `json:"has_mbtiles"`
	HasSource     bool      `json:"has_source"`
	HasVectors    bool      `json:"has_vectors"`
	ControlPoints int       `json:"control_points"`
}

// ProjectPackageInspection returns pre-import validation and metadata preview for .mbtiles and .zip reopening.
type ProjectPackageInspection struct {
	Valid             bool                   `json:"valid"`
	PackageType       string                 `json:"package_type"` // "mbtiles" | "project_zip"
	Filename          string                 `json:"filename"`
	FileSize          int64                  `json:"file_size"`
	ProjectName       string                 `json:"project_name"`
	SchemaVersion     int                    `json:"schema_version"`
	Bounds            []float64              `json:"bounds,omitempty"`
	Center            []float64              `json:"center,omitempty"`
	InitialZoom       float64                `json:"initial_zoom"`
	MinZoom           int                    `json:"min_zoom"`
	MaxZoom           int                    `json:"max_zoom"`
	TileFormat        string                 `json:"tile_format,omitempty"`
	TileScheme        string                 `json:"tile_scheme,omitempty"`
	TileCount         int                    `json:"tile_count"`
	HasBaseLayer      bool                   `json:"has_base_layer"`
	BuildingCount     int                    `json:"building_count"`
	FloorCount        int                    `json:"floor_count"`
	LayerCount        int                    `json:"layer_count"`
	ControlPointCount int                    `json:"control_point_count"`
	LandmarkCount     int                    `json:"landmark_count"`
	AnnotationCount   int                    `json:"annotation_count"`
	Layers            []*ProjectLayerPreview `json:"layers"`
	Warnings          []string               `json:"warnings,omitempty"`
}
