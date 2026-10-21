package georeference

import (
	"github.com/malikharman0823/veracity-maps/gis"
	"github.com/malikharman0823/veracity-maps/models"
)

type ControlPointListResponse struct {
	Points []*models.ControlPoint `json:"points"`
}

type GeoreferenceResponse struct {
	Transformation           string             `json:"transformation"`
	Parameters               map[string]float64 `json:"parameters"`
	TransformationParameters map[string]float64 `json:"transformation_parameters,omitempty"`
	Transform                []float64          `json:"transform"`
	TargetCRS                string             `json:"target_crs"`
	Bounds                   map[string]float64 `json:"bounds"`
	MapLibreBounds           []float64          `json:"maplibre_bounds"`
	ControlPointCount        int                `json:"control_point_count,omitempty"`
	Status                   string             `json:"status,omitempty"`
	IsGeoreferenced          bool               `json:"is_georeferenced"`
}

type GeoreferenceConfigResponse struct {
	Configuration map[string]interface{} `json:"configuration"`
}

func FromTransformResult(tr *gis.TransformResult) GeoreferenceResponse {
	return GeoreferenceResponse{
		Transformation:           tr.Transformation,
		Parameters:               tr.Parameters,
		TransformationParameters: tr.Parameters,
		Transform:                tr.Transform,
		TargetCRS:                tr.TargetCRS,
		Bounds:                   tr.Bounds,
		MapLibreBounds:           tr.MapLibreBounds,
		ControlPointCount:        tr.ControlPointCount,
		Status:                   "calculated",
		IsGeoreferenced:          true,
	}
}
