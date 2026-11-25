package gis

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VectorTileLayerResult describes a single vector layer converted to local XYZ MVT/PBF tiles.
type VectorTileLayerResult struct {
	Success       bool                     `json:"success"`
	LayerSlug     string                   `json:"layer_slug"`
	LayerName     string                   `json:"layer_name"`
	SourceLayer   string                   `json:"source_layer"`
	SourceCRS     string                   `json:"source_crs"`
	TileCRS       string                   `json:"tile_crs"`
	GeometryType  string                   `json:"geometry_type"`
	GeometryTypes []string                 `json:"geometry_types,omitempty"`
	FeatureCount  int                      `json:"feature_count"`
	TileCount     int                      `json:"tile_count"`
	Bounds        []float64                `json:"bounds,omitempty"`
	Center        []float64                `json:"center,omitempty"`
	MinZoom       int                      `json:"min_zoom"`
	MaxZoom       int                      `json:"max_zoom"`
	Fields        []map[string]interface{} `json:"fields,omitempty"`
	LayerDir      string                   `json:"layer_dir"`
	TilesDir      string                   `json:"tiles_dir"`
	MetadataPath  string                   `json:"metadata_path"`
	StylePath     string                   `json:"style_path"`
	Error         string                   `json:"error,omitempty"`
}

// ConvertVectorTilesResult is the response returned by vector_tool.py convert_gpkg_tiles.
type ConvertVectorTilesResult struct {
	Success         bool                    `json:"success"`
	VectorTilesRoot string                  `json:"vector_tiles_root"`
	Layers          []VectorTileLayerResult `json:"layers"`
	Error           string                  `json:"error,omitempty"`
}

// ConvertDatasetToVectorTiles converts all or selected internal layers of a GeoPackage (or vector file)
// into local XYZ MVT/PBF vector tiles under vectorTilesRoot/<layer_slug>/{metadata.json,style.json,tiles/z/x/y.pbf}.
func ConvertDatasetToVectorTiles(srcPath, vectorTilesRoot string, layerNames []string, minZoom, maxZoom int) (*ConvertVectorTilesResult, error) {
	if minZoom < 0 {
		minZoom = 0
	}
	if maxZoom <= 0 || maxZoom > 14 {
		maxZoom = 14
	}
	if minZoom > maxZoom {
		minZoom = maxZoom
	}
	args := []string{
		"convert_gpkg_tiles",
		srcPath,
		vectorTilesRoot,
		strings.Join(layerNames, ","),
		strconv.Itoa(minZoom),
		strconv.Itoa(maxZoom),
	}
	var out ConvertVectorTilesResult
	if err := RunPythonJSON(5*time.Minute, "vector_tool.py", args, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		if out.Error == "" {
			out.Error = "vector tile conversion failed"
		}
		return &out, fmt.Errorf("VECTOR_TILE_CONVERSION_FAILED: %s", out.Error)
	}
	return &out, nil
}
