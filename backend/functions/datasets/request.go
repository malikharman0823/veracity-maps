package datasets

type CreateDatasetRequest struct {
	Name   string `json:"name"`
	Format string `json:"format"`
}

type UpdateLayerRequest struct {
	Visible *bool    `json:"visible,omitempty"`
	Opacity *float64 `json:"opacity,omitempty"`
}
