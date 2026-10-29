package export

import "github.com/malikharman0823/veracity-maps/services"

type ExportOptions struct {
	IncludeBuildingConfig  bool                        `json:"include_building_configuration"`
	IncludeControlPoints   bool                        `json:"include_control_points"`
	IncludeIcons           bool                        `json:"include_icons"`
	IncludeLayerStyles     bool                        `json:"include_layer_styles"`
	IncludeOriginalRaster  bool                        `json:"include_original_rasters"`
	IncludeProcessedRaster bool                        `json:"include_processed_rasters"`
	IncludeBaseMapSettings bool                        `json:"include_base_map_settings"`
	IncludeProjectMetadata bool                        `json:"include_project_metadata"`
	ZoomConfig             *services.ZoomConfiguration `json:"zoom_config,omitempty"`
}

type ExportRequest struct {
	Format      string                      `json:"format"`
	ProjectName string                      `json:"project_name,omitempty"`
	BuildingIDs []string                    `json:"building_ids,omitempty"`
	FloorIDs    []string                    `json:"floor_ids,omitempty"`
	LayerIDs    []string                    `json:"layer_ids,omitempty"`
	Options     *ExportOptions              `json:"options,omitempty"`
	ZoomConfig  *services.ZoomConfiguration `json:"zoom_config,omitempty"`
}
