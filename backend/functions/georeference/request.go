package georeference

type CreateControlPointRequest struct {
	ImageX    float64 `json:"image_x"`
	ImageY    float64 `json:"image_y"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type GeoreferenceRequest struct {
	Action         string `json:"action"`
	Transformation string `json:"transformation"`
	OutputFormat   string `json:"output_format"`
	TargetCRS      string `json:"target_crs"`
}
