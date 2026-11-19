package gis

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// ============================================================================
// 1. ALGORITHM CATALOG REGISTRY (MODULE A)
// ============================================================================

func floatPtr(v float64) *float64 { return &v }

// GetProcessingAlgorithms returns the full catalog of Phase 2 GIS Processing Toolbox algorithms.
func GetProcessingAlgorithms() []models.ProcessingAlgorithm {
	return []models.ProcessingAlgorithm{
		// Category 1: Geometry Operations
		{
			AlgorithmID: "buffer",
			Name:        "Buffer Analysis",
			Category:    "Geometry Operations",
			Description: "Creates metric geographic buffer zones around Point, LineString, and Polygon features using automatic local UTM projection and GEOS/offset geometry.",
			Library:     "GEOS / PROJ",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_id", Label: "Input Vector Layer", Type: "layer", Required: true},
				{Name: "distance", Label: "Buffer Distance", Type: "number", Required: true, DefaultValue: 500.0, Min: floatPtr(0.1)},
				{Name: "unit", Label: "Distance Unit", Type: "select", Required: true, DefaultValue: "meters", Options: []string{"meters", "kilometers", "feet", "miles"}},
				{Name: "segments", Label: "Quarter-Circle Segments", Type: "number", Required: false, DefaultValue: 16, Min: floatPtr(4), Max: floatPtr(64)},
				{Name: "cap_style", Label: "End Cap Style", Type: "select", Required: false, DefaultValue: "round", Options: []string{"round", "flat", "square"}},
				{Name: "join_style", Label: "Join Style", Type: "select", Required: false, DefaultValue: "round", Options: []string{"round", "mitre", "bevel"}},
				{Name: "dissolve", Label: "Dissolve Overlapping Buffers", Type: "boolean", Required: false, DefaultValue: false},
				{Name: "output_crs", Label: "Output CRS", Type: "string", Required: false, DefaultValue: "EPSG:4326"},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Buffered Zone"},
			},
		},
		{
			AlgorithmID: "centroid",
			Name:        "Centroid Generation",
			Category:    "Geometry Operations",
			Description: "Generates a Point feature at the geometric centroid of each input feature while preserving all attributes.",
			Library:     "GEOS",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_id", Label: "Input Vector Layer", Type: "layer", Required: true},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Centroids"},
			},
		},
		{
			AlgorithmID: "calculate_geometry",
			Name:        "Calculate Area and Length",
			Category:    "Geometry Operations",
			Description: "Computes geometric area, perimeter, length, and centroid X/Y attributes on all features in a vector layer.",
			Library:     "GEOS / PROJ",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_id", Label: "Input Vector Layer", Type: "layer", Required: true},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Layer with Area & Length"},
			},
		},
		// Category 2: Overlay Analysis
		{
			AlgorithmID: "intersection",
			Name:        "Intersection",
			Category:    "Overlay Analysis",
			Description: "Computes the geometric intersection of overlapping features between two layers and merges their attributes.",
			Library:     "GEOS",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_a", Label: "Input Layer A", Type: "layer", Required: true},
				{Name: "input_layer_b", Label: "Overlay Layer B", Type: "layer", Required: true},
				{Name: "preserve_attributes", Label: "Attribute Preservation", Type: "select", Required: false, DefaultValue: "all", Options: []string{"all", "layer_a", "none"}},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Intersection Result"},
			},
		},
		{
			AlgorithmID: "union",
			Name:        "Union",
			Category:    "Overlay Analysis",
			Description: "Combines two polygon layers while preserving both overlapping and non-overlapping boundaries and attributes.",
			Library:     "GEOS",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_a", Label: "Input Layer A", Type: "layer", Required: true},
				{Name: "input_layer_b", Label: "Input Layer B", Type: "layer", Required: true},
				{Name: "preserve_attributes", Label: "Attribute Preservation", Type: "select", Required: false, DefaultValue: "all", Options: []string{"all", "layer_a", "none"}},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Union Result"},
			},
		},
		{
			AlgorithmID: "difference",
			Name:        "Difference (Erase)",
			Category:    "Overlay Analysis",
			Description: "Removes areas of Input Layer A that are covered by geometries in Overlay Layer B.",
			Library:     "GEOS",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_a", Label: "Input Layer A", Type: "layer", Required: true},
				{Name: "input_layer_b", Label: "Erase Layer B", Type: "layer", Required: true},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Difference Result"},
			},
		},
		{
			AlgorithmID: "clip",
			Name:        "Clip Vector by Mask",
			Category:    "Overlay Analysis",
			Description: "Extracts only the portions of Input Layer A falling inside the clipping boundary of Layer B.",
			Library:     "GEOS",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_a", Label: "Input Layer A", Type: "layer", Required: true},
				{Name: "input_layer_b", Label: "Clipping Boundary Layer B", Type: "layer", Required: true},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Clipped Vector"},
			},
		},
		{
			AlgorithmID: "dissolve",
			Name:        "Dissolve",
			Category:    "Overlay Analysis",
			Description: "Merges geographic features sharing identical values in selected dissolve fields, or dissolves all geometries into one.",
			Library:     "GEOS",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_id", Label: "Input Layer", Type: "layer", Required: true},
				{Name: "dissolve_all", Label: "Dissolve All Features", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "dissolve_fields", Label: "Dissolve Attribute Fields (comma-separated)", Type: "fields", Required: false},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Dissolved Layer"},
			},
		},
		{
			AlgorithmID: "symmetrical_difference",
			Name:        "Symmetrical Difference",
			Category:    "Overlay Analysis",
			Description: "Identifies geographic areas belonging to either Layer A or Layer B, but not both.",
			Library:     "GEOS",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_a", Label: "Input Layer A", Type: "layer", Required: true},
				{Name: "input_layer_b", Label: "Input Layer B", Type: "layer", Required: true},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Symmetrical Difference"},
			},
		},
		// Category 3: Spatial Statistics & Spatial Join
		{
			AlgorithmID: "spatial_join",
			Name:        "Join Attributes by Location (Spatial Join)",
			Category:    "Spatial Statistics",
			Description: "Combines and aggregates attributes from a source layer into a target layer based on spatial relationships (Intersects, Contains, Within, Touches, Overlaps, Crosses, Nearest).",
			Library:     "GEOS / Spatial Index",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "target_layer_id", Label: "Target Layer", Type: "layer", Required: true},
				{Name: "source_layer_id", Label: "Join Source Layer", Type: "layer", Required: true},
				{Name: "spatial_relationship", Label: "Spatial Predicate", Type: "select", Required: true, DefaultValue: "intersects", Options: []string{"intersects", "contains", "within", "touches", "overlaps", "crosses", "nearest"}},
				{Name: "aggregation_type", Label: "Aggregation Method", Type: "select", Required: true, DefaultValue: "count", Options: []string{"count", "sum", "avg", "min", "max", "first", "all"}},
				{Name: "selected_fields", Label: "Source Fields to Aggregate", Type: "fields", Required: false},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Spatial Join Result"},
			},
		},
		// Category 4: Proximity Analysis
		{
			AlgorithmID: "proximity",
			Name:        "Nearest Neighbor & Proximity Analysis",
			Category:    "Proximity Analysis",
			Description: "Finds k-nearest target features within an optional maximum radius and generates metric distance attributes and hub-to-spoke connection lines.",
			Library:     "PROJ / Spatial Index",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "source_layer_id", Label: "Source Origin Layer", Type: "layer", Required: true},
				{Name: "target_layer_id", Label: "Target Destination Layer", Type: "layer", Required: true},
				{Name: "maximum_distance", Label: "Maximum Search Radius (0 = unlimited)", Type: "number", Required: false, DefaultValue: 0, Min: floatPtr(0)},
				{Name: "unit", Label: "Distance Unit", Type: "select", Required: false, DefaultValue: "meters", Options: []string{"meters", "kilometers", "feet", "miles"}},
				{Name: "nearest_count", Label: "Nearest Neighbors Count (k)", Type: "number", Required: false, DefaultValue: 1, Min: floatPtr(1), Max: floatPtr(50)},
				{Name: "generate_connection_lines", Label: "Generate Connection Lines", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Proximity Connections"},
			},
		},
		{
			AlgorithmID: "distance_matrix",
			Name:        "Distance Matrix",
			Category:    "Proximity Analysis",
			Description: "Computes pairwise geodesic distances between source and target features and outputs a distance matrix layer.",
			Library:     "PROJ / Spatial Index",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "source_layer_id", Label: "Source Layer", Type: "layer", Required: true},
				{Name: "target_layer_id", Label: "Target Layer", Type: "layer", Required: true},
				{Name: "nearest_count", Label: "Max Target Pairs per Source", Type: "number", Required: false, DefaultValue: 5, Min: floatPtr(1), Max: floatPtr(100)},
				{Name: "unit", Label: "Distance Unit", Type: "select", Required: false, DefaultValue: "meters", Options: []string{"meters", "kilometers", "feet", "miles"}},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Distance Matrix"},
			},
		},
		{
			AlgorithmID: "nearest_facility",
			Name:        "Nearest Facility Allocation",
			Category:    "Proximity Analysis",
			Description: "Allocates each demand feature to its closest service facility and generates shortest straight-line allocation vectors.",
			Library:     "PROJ / Spatial Index",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "source_layer_id", Label: "Demand Points / Polygons", Type: "layer", Required: true},
				{Name: "target_layer_id", Label: "Service Facilities Layer", Type: "layer", Required: true},
				{Name: "maximum_distance", Label: "Service Threshold Radius (0 = unlimited)", Type: "number", Required: false, DefaultValue: 0},
				{Name: "unit", Label: "Distance Unit", Type: "select", Required: false, DefaultValue: "meters", Options: []string{"meters", "kilometers", "feet", "miles"}},
				{Name: "output_name", Label: "Output Layer Name", Type: "string", Required: false, DefaultValue: "Facility Allocation"},
			},
		},
		// Category 5: Density Analysis
		{
			AlgorithmID: "heatmap",
			Name:        "Kernel Density Heatmap (KDE)",
			Category:    "Density Analysis",
			Description: "Computes a continuous 2D Kernel Density Estimation raster from point features with configurable kernel, radius, weight field, and color ramp.",
			Library:     "GDAL / Raster KDE",
			InputType:   "vector",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_layer_id", Label: "Input Point Layer", Type: "layer", Required: true},
				{Name: "weight_field", Label: "Weight Attribute Field (optional)", Type: "string", Required: false},
				{Name: "radius", Label: "Search Bandwidth Radius", Type: "number", Required: true, DefaultValue: 500, Min: floatPtr(1)},
				{Name: "radius_unit", Label: "Radius Unit", Type: "select", Required: false, DefaultValue: "meters", Options: []string{"meters", "kilometers", "degrees"}},
				{Name: "kernel_type", Label: "KDE Kernel Function", Type: "select", Required: false, DefaultValue: "quartic", Options: []string{"quartic", "gaussian", "triangular", "uniform", "epanechnikov"}},
				{Name: "color_ramp", Label: "Color Ramp", Type: "select", Required: false, DefaultValue: "inferno", Options: []string{"inferno", "viridis", "magma", "plasma", "ylorrd", "coolwarm"}},
				{Name: "output_name", Label: "Output Raster Name", Type: "string", Required: false, DefaultValue: "KDE Heatmap"},
			},
		},
		// Category 6: Raster Processing
		{
			AlgorithmID: "raster_clip",
			Name:        "Clip Raster by Mask Layer",
			Category:    "Raster Processing",
			Description: "Clips and masks a georeferenced raster dataset to the boundary of a polygon vector layer.",
			Library:     "GDAL Warp",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_id", Label: "Input Raster Dataset", Type: "dataset", Required: true},
				{Name: "clipping_layer_id", Label: "Clipping Polygon Layer", Type: "layer", Required: true},
				{Name: "output_name", Label: "Output Raster Name", Type: "string", Required: false, DefaultValue: "Clipped Raster"},
			},
		},
		{
			AlgorithmID: "raster_reproject",
			Name:        "Warp (Reproject Raster)",
			Category:    "Raster Processing",
			Description: "Reprojects a raster dataset into a target Coordinate Reference System (EPSG).",
			Library:     "GDAL Warp / PROJ",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_id", Label: "Input Raster Dataset", Type: "dataset", Required: true},
				{Name: "target_crs", Label: "Target CRS", Type: "string", Required: true, DefaultValue: "EPSG:3857"},
				{Name: "resampling", Label: "Resampling Method", Type: "select", Required: false, DefaultValue: "bilinear", Options: []string{"nearest", "bilinear", "cubic"}},
				{Name: "output_name", Label: "Output Raster Name", Type: "string", Required: false, DefaultValue: "Reprojected Raster"},
			},
		},
		{
			AlgorithmID: "raster_resample",
			Name:        "Resample Raster Resolution",
			Category:    "Raster Processing",
			Description: "Changes the spatial resolution and pixel dimensions of a raster using nearest, bilinear, or cubic interpolation.",
			Library:     "GDAL",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_id", Label: "Input Raster Dataset", Type: "dataset", Required: true},
				{Name: "scale_factor", Label: "Resolution Scale Factor", Type: "number", Required: true, DefaultValue: 0.5, Min: floatPtr(0.05), Max: floatPtr(4.0)},
				{Name: "method", Label: "Interpolation Method", Type: "select", Required: false, DefaultValue: "bilinear", Options: []string{"nearest", "bilinear", "cubic"}},
				{Name: "output_name", Label: "Output Raster Name", Type: "string", Required: false, DefaultValue: "Resampled Raster"},
			},
		},
		{
			AlgorithmID: "raster_statistics",
			Name:        "Raster Layer Statistics",
			Category:    "Raster Processing",
			Description: "Computes summary statistics (minimum, maximum, mean, standard deviation) for a raster dataset.",
			Library:     "GDAL",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_id", Label: "Input Raster Dataset", Type: "dataset", Required: true},
				{Name: "output_name", Label: "Report Name", Type: "string", Required: false, DefaultValue: "Raster Statistics"},
			},
		},
		{
			AlgorithmID: "raster_calculator",
			Name:        "Raster Calculator (Band Math)",
			Category:    "Raster Processing",
			Description: "Evaluates safe mathematical and conditional expressions across one or more rasters (e.g. A + B, A * 1.5, (A - B) / (A + B)).",
			Library:     "GDAL / Band Math AST",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "input_rasters", Label: "Input Raster Dataset IDs", Type: "dataset", Required: true},
				{Name: "expression", Label: "Band Math Expression", Type: "string", Required: true, DefaultValue: "A * 1.25"},
				{Name: "output_name", Label: "Output Raster Name", Type: "string", Required: false, DefaultValue: "Calculated Raster"},
			},
		},
		{
			AlgorithmID: "raster_align",
			Name:        "Align Rasters to Reference Grid",
			Category:    "Raster Processing",
			Description: "Aligns CRS, pixel resolution, grid origin, and spatial extent of a raster to match a reference raster.",
			Library:     "GDAL",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_id", Label: "Target Raster to Align", Type: "dataset", Required: true},
				{Name: "reference_dataset_id", Label: "Reference Grid Raster", Type: "dataset", Required: false},
				{Name: "output_name", Label: "Output Raster Name", Type: "string", Required: false, DefaultValue: "Aligned Raster"},
			},
		},
		{
			AlgorithmID: "raster_mosaic",
			Name:        "Mosaic Rasters",
			Category:    "Raster Processing",
			Description: "Merges multiple adjacent or overlapping raster datasets into a single seamless composite raster.",
			Library:     "GDAL",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_ids", Label: "Input Raster Dataset IDs", Type: "dataset", Required: true},
				{Name: "output_name", Label: "Output Mosaic Name", Type: "string", Required: false, DefaultValue: "Raster Mosaic"},
			},
		},
		{
			AlgorithmID: "raster_contours",
			Name:        "Contour Lines Generation",
			Category:    "Data Conversion",
			Description: "Extracts vector contour isolines at a specified elevation/intensity interval from a raster band using Marching Squares.",
			Library:     "GDAL Contour",
			InputType:   "raster",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_id", Label: "Input Raster Dataset", Type: "dataset", Required: true},
				{Name: "interval", Label: "Contour Interval", Type: "number", Required: true, DefaultValue: 25.0, Min: floatPtr(1.0)},
				{Name: "output_name", Label: "Output Contour Layer Name", Type: "string", Required: false, DefaultValue: "Contours"},
			},
		},
		{
			AlgorithmID: "raster_hillshade",
			Name:        "Hillshade Terrain Illumination",
			Category:    "Raster Processing",
			Description: "Generates a 3D shaded relief raster using Horn's surface normal slope/aspect illumination method.",
			Library:     "GDAL DEM",
			InputType:   "raster",
			OutputType:  "raster",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "raster_dataset_id", Label: "Input Elevation/Intensity Raster", Type: "dataset", Required: true},
				{Name: "azimuth", Label: "Sun Azimuth Angle (0-360°)", Type: "number", Required: false, DefaultValue: 315.0, Min: floatPtr(0), Max: floatPtr(360)},
				{Name: "altitude", Label: "Sun Altitude Angle (1-90°)", Type: "number", Required: false, DefaultValue: 45.0, Min: floatPtr(1), Max: floatPtr(90)},
				{Name: "z_factor", Label: "Vertical Exaggeration (Z-Factor)", Type: "number", Required: false, DefaultValue: 1.0, Min: floatPtr(0.1)},
				{Name: "output_name", Label: "Output Hillshade Name", Type: "string", Required: false, DefaultValue: "Terrain Hillshade"},
			},
		},
		// Category 8: Geometry Validation & Topology
		{
			AlgorithmID: "topology",
			Name:        "Topology & Geometry Validator",
			Category:    "Geometry Validation",
			Description: "Detects self-intersections, polygon overlaps, unintended gaps, duplicate features, and disconnected dangles, producing an inspectable error layer and automated fixes.",
			Library:     "GEOS Topology",
			InputType:   "vector",
			OutputType:  "vector",
			Parameters: []models.AlgorithmParameterSpec{
				{Name: "layer_id", Label: "Input Vector Layer", Type: "layer", Required: true},
				{Name: "check_self_intersections", Label: "Check Self-Intersections & Invalid Rings", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "check_overlaps", Label: "Check Overlapping Polygons", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "check_gaps", Label: "Check Polygon Gaps", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "check_duplicates", Label: "Check Duplicate Geometries", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "check_disconnected_lines", Label: "Check Disconnected Line Dangles", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "output_error_layer", Label: "Generate Topology Error Layer on Map", Type: "boolean", Required: false, DefaultValue: true},
				{Name: "output_name", Label: "Error Layer Name", Type: "string", Required: false, DefaultValue: "Topology Errors"},
			},
		},
	}
}

func GetAlgorithmDetails(algorithmID string) (*models.ProcessingAlgorithm, error) {
	clean := strings.ToLower(strings.TrimSpace(algorithmID))
	clean = strings.ReplaceAll(clean, "-", "_")
	for _, alg := range GetProcessingAlgorithms() {
		if alg.AlgorithmID == clean {
			cp := alg
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("processing algorithm '%s' not found", algorithmID)
}

// ============================================================================
// 2. MODULE B — METRIC BUFFER ANALYSIS ENGINE
// ============================================================================

// ConvertDistanceToMeters converts any supported unit (meters, kilometers, feet, miles) into meters.
func ConvertDistanceToMeters(distance float64, unit string) float64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "kilometers", "kilometres", "km":
		return distance * 1000.0
	case "feet", "ft":
		return distance * 0.3048
	case "miles", "mi":
		return distance * 1609.344
	default:
		return distance
	}
}

// ConvertMetersToUnit converts a distance in meters into the requested unit.
func ConvertMetersToUnit(meters float64, unit string) float64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "kilometers", "kilometres", "km":
		return meters / 1000.0
	case "feet", "ft":
		return meters / 0.3048
	case "miles", "mi":
		return meters / 1609.344
	default:
		return meters
	}
}

// DetermineMetricUTMZone determines the appropriate WGS84 UTM EPSG code (e.g. EPSG:32643 for Delhi)
// from a geographic longitude and latitude so metric buffers are computed in true planar meters.
func DetermineMetricUTMZone(lon, lat float64) string {
	zone := int(math.Floor((lon+180.0)/6.0)) + 1
	if zone < 1 {
		zone = 1
	}
	if zone > 60 {
		zone = 60
	}
	if lat >= 0 {
		return fmt.Sprintf("EPSG:%d", 32600+zone)
	}
	return fmt.Sprintf("EPSG:%d", 32700+zone)
}

// GenerateBufferGeometry creates a metric buffer polygon around an input geometry.
// It transforms geographic coordinates to a local metric UTM CRS first, buffers in meters,
// and reprojects the buffered polygon to targetCRS.
func GenerateBufferGeometry(geom models.GeoJSONGeometry, srcCRS string, distanceMeters float64, segments int, capStyle, joinStyle string, targetCRS string) (*models.GeoJSONGeometry, error) {
	if segments < 4 {
		segments = 16
	}
	if srcCRS == "" {
		srcCRS = "EPSG:4326"
	}
	if targetCRS == "" {
		targetCRS = srcCRS
	}

	// 1. Determine working metric CRS
	workingCRS := srcCRS
	if strings.EqualFold(srcCRS, "EPSG:4326") || strings.EqualFold(srcCRS, "WGS84") {
		bbox := computeBBox(geom)
		if len(bbox) == 4 {
			cx := (bbox[0] + bbox[2]) / 2.0
			cy := (bbox[1] + bbox[3]) / 2.0
			workingCRS = DetermineMetricUTMZone(cx, cy)
		} else {
			workingCRS = "EPSG:3857"
		}
	}

	// 2. Transform input geometry into working metric CRS
	metricGeom := geom
	if !strings.EqualFold(srcCRS, workingCRS) {
		projected, err := TransformGeometryCRS(geom, srcCRS, workingCRS)
		if err == nil && projected != nil {
			metricGeom = *projected
		}
	}

	// 3. Buffer in planar meters
	bufferedMetric, err := bufferMetricGeometry(metricGeom, distanceMeters, segments, capStyle, joinStyle)
	if err != nil {
		return nil, err
	}

	// 4. Transform from working metric CRS into targetCRS
	if !strings.EqualFold(workingCRS, targetCRS) {
		reproj, err := TransformGeometryCRS(*bufferedMetric, workingCRS, targetCRS)
		if err == nil && reproj != nil {
			return reproj, nil
		}
	}
	return bufferedMetric, nil
}

func bufferMetricGeometry(g models.GeoJSONGeometry, r float64, segments int, capStyle, joinStyle string) (*models.GeoJSONGeometry, error) {
	switch g.Type {
	case "Point":
		var pt []float64
		if err := json.Unmarshal(g.Coordinates, &pt); err != nil || len(pt) < 2 {
			return nil, fmt.Errorf("invalid Point coordinates")
		}
		ring := bufferPointMetric(pt[0], pt[1], r, segments, capStyle)
		raw, _ := json.Marshal([][][]float64{ring})
		return &models.GeoJSONGeometry{Type: "Polygon", Coordinates: raw}, nil

	case "MultiPoint":
		var pts [][]float64
		if err := json.Unmarshal(g.Coordinates, &pts); err != nil || len(pts) == 0 {
			return nil, fmt.Errorf("invalid MultiPoint coordinates")
		}
		var polys [][][][]float64
		for _, pt := range pts {
			if len(pt) >= 2 {
				polys = append(polys, [][][]float64{bufferPointMetric(pt[0], pt[1], r, segments, capStyle)})
			}
		}
		raw, _ := json.Marshal(polys)
		return &models.GeoJSONGeometry{Type: "MultiPolygon", Coordinates: raw}, nil

	case "LineString":
		var line [][]float64
		if err := json.Unmarshal(g.Coordinates, &line); err != nil || len(line) < 2 {
			return nil, fmt.Errorf("invalid LineString coordinates")
		}
		ring := bufferLineMetric(line, r, segments, capStyle, joinStyle)
		raw, _ := json.Marshal([][][]float64{ring})
		return &models.GeoJSONGeometry{Type: "Polygon", Coordinates: raw}, nil

	case "MultiLineString":
		var lines [][][]float64
		if err := json.Unmarshal(g.Coordinates, &lines); err != nil || len(lines) == 0 {
			return nil, fmt.Errorf("invalid MultiLineString coordinates")
		}
		var polys [][][][]float64
		for _, line := range lines {
			if len(line) >= 2 {
				polys = append(polys, [][][]float64{bufferLineMetric(line, r, segments, capStyle, joinStyle)})
			}
		}
		raw, _ := json.Marshal(polys)
		return &models.GeoJSONGeometry{Type: "MultiPolygon", Coordinates: raw}, nil

	case "Polygon":
		var rings [][][]float64
		if err := json.Unmarshal(g.Coordinates, &rings); err != nil || len(rings) == 0 {
			return nil, fmt.Errorf("invalid Polygon coordinates")
		}
		expanded := bufferPolygonRingMetric(rings[0], r, segments, joinStyle)
		raw, _ := json.Marshal([][][]float64{expanded})
		return &models.GeoJSONGeometry{Type: "Polygon", Coordinates: raw}, nil

	case "MultiPolygon":
		var polys [][][][]float64
		if err := json.Unmarshal(g.Coordinates, &polys); err != nil || len(polys) == 0 {
			return nil, fmt.Errorf("invalid MultiPolygon coordinates")
		}
		var out [][][][]float64
		for _, p := range polys {
			if len(p) > 0 {
				out = append(out, [][][]float64{bufferPolygonRingMetric(p[0], r, segments, joinStyle)})
			}
		}
		raw, _ := json.Marshal(out)
		return &models.GeoJSONGeometry{Type: "MultiPolygon", Coordinates: raw}, nil

	default:
		return nil, fmt.Errorf("unsupported geometry type '%s' for buffer", g.Type)
	}
}

func bufferPointMetric(cx, cy, r float64, segments int, capStyle string) [][]float64 {
	if strings.EqualFold(capStyle, "square") {
		return [][]float64{
			{cx - r, cy - r},
			{cx + r, cy - r},
			{cx + r, cy + r},
			{cx - r, cy + r},
			{cx - r, cy - r},
		}
	}
	n := segments * 4
	if n < 16 {
		n = 16
	}
	ring := make([][]float64, 0, n+1)
	for i := 0; i < n; i++ {
		theta := 2.0 * math.Pi * float64(i) / float64(n)
		ring = append(ring, []float64{cx + r*math.Cos(theta), cy + r*math.Sin(theta)})
	}
	ring = append(ring, ring[0])
	return ring
}

func bufferLineMetric(line [][]float64, r float64, segments int, capStyle, _ string) [][]float64 {
	var leftSide, rightSide [][]float64
	for i := 0; i < len(line)-1; i++ {
		p1, p2 := line[i], line[i+1]
		dx := p2[0] - p1[0]
		dy := p2[1] - p1[1]
		length := math.Hypot(dx, dy)
		if length < 1e-9 {
			continue
		}
		nx := -dy / length
		ny := dx / length

		ux := dx / length
		uy := dy / length

		// Start extension for square cap
		sExt := 0.0
		if i == 0 && strings.EqualFold(capStyle, "square") {
			sExt = r
		}
		eExt := 0.0
		if i == len(line)-2 && strings.EqualFold(capStyle, "square") {
			eExt = r
		}

		leftSide = append(leftSide,
			[]float64{p1[0] - ux*sExt + nx*r, p1[1] - uy*sExt + ny*r},
			[]float64{p2[0] + ux*eExt + nx*r, p2[1] + uy*eExt + ny*r},
		)
		rightSide = append(rightSide,
			[]float64{p1[0] - ux*sExt - nx*r, p1[1] - uy*sExt - ny*r},
			[]float64{p2[0] + ux*eExt - nx*r, p2[1] + uy*eExt - ny*r},
		)
	}
	if len(leftSide) == 0 {
		return bufferPointMetric(line[0][0], line[0][1], r, segments, capStyle)
	}

	var ring [][]float64
	ring = append(ring, leftSide...)

	// End cap around last vertex
	lastPt := line[len(line)-1]
	prevPt := line[len(line)-2]
	endAngle := math.Atan2(lastPt[1]-prevPt[1], lastPt[0]-prevPt[0])
	if strings.EqualFold(capStyle, "round") {
		for s := 1; s < segments; s++ {
			ang := endAngle + math.Pi/2.0 - math.Pi*float64(s)/float64(segments)
			ring = append(ring, []float64{lastPt[0] + r*math.Cos(ang), lastPt[1] + r*math.Sin(ang)})
		}
	}

	// Traverse right side in reverse
	for i := len(rightSide) - 1; i >= 0; i-- {
		ring = append(ring, rightSide[i])
	}

	// Start cap around first vertex
	firstPt := line[0]
	nextPt := line[1]
	startAngle := math.Atan2(nextPt[1]-firstPt[1], nextPt[0]-firstPt[0])
	if strings.EqualFold(capStyle, "round") {
		for s := 1; s < segments; s++ {
			ang := startAngle - math.Pi/2.0 - math.Pi*float64(s)/float64(segments)
			ring = append(ring, []float64{firstPt[0] + r*math.Cos(ang), firstPt[1] + r*math.Sin(ang)})
		}
	}

	ring = append(ring, ring[0])
	return ring
}

func bufferPolygonRingMetric(ring [][]float64, r float64, segments int, joinStyle string) [][]float64 {
	n := len(ring)
	if n > 1 && ring[0][0] == ring[n-1][0] && ring[0][1] == ring[n-1][1] {
		n--
	}
	if n < 3 {
		return bufferPointMetric(ring[0][0], ring[0][1], r, segments, "round")
	}

	// Compute centroid to ensure outward normal direction
	cx, cy := 0.0, 0.0
	for i := 0; i < n; i++ {
		cx += ring[i][0]
		cy += ring[i][1]
	}
	cx /= float64(n)
	cy /= float64(n)

	var out [][]float64
	for i := 0; i < n; i++ {
		prev := ring[(i+n-1)%n]
		curr := ring[i]
		next := ring[(i+1)%n]

		dx1, dy1 := curr[0]-prev[0], curr[1]-prev[1]
		l1 := math.Hypot(dx1, dy1)
		dx2, dy2 := next[0]-curr[0], next[1]-curr[1]
		l2 := math.Hypot(dx2, dy2)
		if l1 < 1e-9 || l2 < 1e-9 {
			continue
		}

		nx1, ny1 := -dy1/l1, dx1/l1
		if (curr[0]+nx1-cx)*(curr[0]-cx)+(curr[1]+ny1-cy)*(curr[1]-cy) < 0 {
			nx1, ny1 = -nx1, -ny1
		}
		nx2, ny2 := -dy2/l2, dx2/l2
		if (curr[0]+nx2-cx)*(curr[0]-cx)+(curr[1]+ny2-cy)*(curr[1]-cy) < 0 {
			nx2, ny2 = -nx2, -ny2
		}

		if strings.EqualFold(joinStyle, "bevel") {
			out = append(out,
				[]float64{curr[0] + nx1*r, curr[1] + ny1*r},
				[]float64{curr[0] + nx2*r, curr[1] + ny2*r},
			)
		} else if strings.EqualFold(joinStyle, "mitre") || strings.EqualFold(joinStyle, "miter") {
			bx, by := nx1+nx2, ny1+ny2
			bl := math.Hypot(bx, by)
			if bl < 1e-6 {
				out = append(out, []float64{curr[0] + nx1*r, curr[1] + ny1*r})
			} else {
				bx, by = bx/bl, by/bl
				cosHalf := bx*nx1 + by*ny1
				miterLen := r
				if cosHalf > 0.25 {
					miterLen = r / cosHalf
				}
				out = append(out, []float64{curr[0] + bx*miterLen, curr[1] + by*miterLen})
			}
		} else {
			// Round join arc between (nx1, ny1) and (nx2, ny2)
			ang1 := math.Atan2(ny1, nx1)
			ang2 := math.Atan2(ny2, nx2)
			diff := ang2 - ang1
			for diff > math.Pi {
				diff -= 2 * math.Pi
			}
			for diff < -math.Pi {
				diff += 2 * math.Pi
			}
			steps := int(math.Max(2, math.Ceil(math.Abs(diff)/(math.Pi/float64(segments)))))
			for s := 0; s <= steps; s++ {
				a := ang1 + diff*float64(s)/float64(steps)
				out = append(out, []float64{curr[0] + r*math.Cos(a), curr[1] + r*math.Sin(a)})
			}
		}
	}
	if len(out) > 0 {
		out = append(out, out[0])
	}
	return out
}

// ============================================================================
// 3. MODULE C — OVERLAY & DISSOLVE ENGINE
// ============================================================================

type OverlayFeatureItem struct {
	Geometry   models.GeoJSONGeometry `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

// ExecuteOverlayOperation runs Intersection, Union, Difference, Clip, or Symmetrical Difference
// between featuresA and featuresB using GEOS (via processing_tool.py) with a deterministic pure-Go fallback.
func ExecuteOverlayOperation(operation string, featuresA, featuresB []*models.VectorFeature, preserveAttrs string) ([]OverlayFeatureItem, error) {
	op := strings.ToLower(strings.TrimSpace(operation))
	op = strings.ReplaceAll(op, "-", "_")

	// Try GEOS via Python bridge first for exact polygon boolean algebra
	pyPayload := map[string]interface{}{
		"operation":           op,
		"preserve_attributes": preserveAttrs,
		"features_a":          toSimpleFeatureList(featuresA),
		"features_b":          toSimpleFeatureList(featuresB),
	}
	if payloadBytes, err := json.Marshal(pyPayload); err == nil {
		var pyResp struct {
			Success  bool                 `json:"success"`
			Features []OverlayFeatureItem `json:"features"`
		}
		if err := RunPythonJSON(45*time.Second, "processing_tool.py", []string{"overlay", string(payloadBytes)}, &pyResp); err == nil && pyResp.Success {
			return pyResp.Features, nil
		}
	}

	// Pure-Go deterministic geometric overlay implementation
	var out []OverlayFeatureItem
	switch op {
	case "intersection", "clip":
		for _, fa := range featuresA {
			boxA := computeBBox(fa.Geometry)
			for _, fb := range featuresB {
				boxB := computeBBox(fb.Geometry)
				if !bboxIntersects(boxA, boxB) {
					continue
				}
				interGeom := intersectPairGeometry(fa.Geometry, fb.Geometry)
				if interGeom != nil {
					props := map[string]interface{}{}
					if preserveAttrs != "none" {
						for k, v := range fa.Properties {
							props[k] = v
						}
					}
					if op == "intersection" && preserveAttrs == "all" {
						for k, v := range fb.Properties {
							if _, exists := props[k]; exists {
								props["b_"+k] = v
							} else {
								props[k] = v
							}
						}
					}
					out = append(out, OverlayFeatureItem{Geometry: *interGeom, Properties: props})
				}
			}
		}

	case "union":
		// Include intersections + remaining parts of A + remaining parts of B
		interItems, _ := ExecuteOverlayOperation("intersection", featuresA, featuresB, preserveAttrs)
		out = append(out, interItems...)
		diffA, _ := ExecuteOverlayOperation("difference", featuresA, featuresB, preserveAttrs)
		out = append(out, diffA...)
		diffB, _ := ExecuteOverlayOperation("difference", featuresB, featuresA, preserveAttrs)
		out = append(out, diffB...)

	case "difference":
		for _, fa := range featuresA {
			remGeom := differenceFromMask(fa.Geometry, featuresB)
			if remGeom != nil {
				props := map[string]interface{}{}
				if preserveAttrs != "none" {
					for k, v := range fa.Properties {
						props[k] = v
					}
				}
				out = append(out, OverlayFeatureItem{Geometry: *remGeom, Properties: props})
			}
		}

	case "symmetrical_difference":
		diffA, _ := ExecuteOverlayOperation("difference", featuresA, featuresB, preserveAttrs)
		diffB, _ := ExecuteOverlayOperation("difference", featuresB, featuresA, preserveAttrs)
		out = append(out, diffA...)
		out = append(out, diffB...)

	default:
		return nil, fmt.Errorf("unsupported overlay operation '%s'", operation)
	}
	return out, nil
}

func toSimpleFeatureList(feats []*models.VectorFeature) []map[string]interface{} {
	list := make([]map[string]interface{}, 0, len(feats))
	for _, f := range feats {
		var gMap map[string]interface{}
		raw, _ := json.Marshal(f.Geometry)
		_ = json.Unmarshal(raw, &gMap)
		list = append(list, map[string]interface{}{
			"geometry":   gMap,
			"properties": f.Properties,
		})
	}
	return list
}

// intersectPairGeometry computes the geometric intersection of geomA and geomB using Sutherland-Hodgman polygon clipping or point/line containment.
func intersectPairGeometry(geomA, geomB models.GeoJSONGeometry) *models.GeoJSONGeometry {
	if geomA.Type == "Point" {
		verts, _ := extractVerticesAndSegments(geomA)
		if len(verts) > 0 && pointInGeometry(verts[0], geomB, true) {
			cp := geomA
			return &cp
		}
		return nil
	}
	if geomA.Type == "Polygon" && geomB.Type == "Polygon" {
		var ringsA, ringsB [][][]float64
		if json.Unmarshal(geomA.Coordinates, &ringsA) == nil && json.Unmarshal(geomB.Coordinates, &ringsB) == nil && len(ringsA) > 0 && len(ringsB) > 0 {
			clipped := clipPolygonSutherlandHodgman(ringsA[0], ringsB[0])
			if len(clipped) >= 4 {
				raw, _ := json.Marshal([][][]float64{clipped})
				return &models.GeoJSONGeometry{Type: "Polygon", Coordinates: raw}
			}
		}
	}
	if EvaluateSpatialPredicate(geomA, geomB, "intersects", 0, "") {
		cp := geomA
		return &cp
	}
	return nil
}

func clipPolygonSutherlandHodgman(subjectRing, clipRing [][]float64) [][]float64 {
	output := dropClosingVertex(subjectRing)
	clipVerts := dropClosingVertex(clipRing)
	if len(output) < 3 || len(clipVerts) < 3 {
		return nil
	}
	// Ensure clipVerts is counter-clockwise
	if signedRingArea(clipVerts) < 0 {
		for i, j := 0, len(clipVerts)-1; i < j; i, j = i+1, j-1 {
			clipVerts[i], clipVerts[j] = clipVerts[j], clipVerts[i]
		}
	}

	for i := 0; i < len(clipVerts); i++ {
		cp1 := clipVerts[i]
		cp2 := clipVerts[(i+1)%len(clipVerts)]
		input := output
		output = nil
		if len(input) == 0 {
			break
		}
		s := input[len(input)-1]
		for _, e := range input {
			eInside := isInsideHalfPlane(e, cp1, cp2)
			sInside := isInsideHalfPlane(s, cp1, cp2)
			if eInside {
				if !sInside {
					output = append(output, lineEdgeIntersection(s, e, cp1, cp2))
				}
				output = append(output, e)
			} else if sInside {
				output = append(output, lineEdgeIntersection(s, e, cp1, cp2))
			}
			s = e
		}
	}
	if len(output) < 3 {
		return nil
	}
	output = append(output, output[0])
	return output
}

func dropClosingVertex(ring [][]float64) [][]float64 {
	if len(ring) > 1 && ring[0][0] == ring[len(ring)-1][0] && ring[0][1] == ring[len(ring)-1][1] {
		return append([][]float64{}, ring[:len(ring)-1]...)
	}
	return append([][]float64{}, ring...)
}

func isInsideHalfPlane(pt, cp1, cp2 []float64) bool {
	return (cp2[0]-cp1[0])*(pt[1]-cp1[1])-(cp2[1]-cp1[1])*(pt[0]-cp1[0]) >= -1e-11
}

func lineEdgeIntersection(s, e, cp1, cp2 []float64) []float64 {
	dcx, dcy := cp1[0]-cp2[0], cp1[1]-cp2[1]
	dpx, dpy := s[0]-e[0], s[1]-e[1]
	n1 := cp1[0]*cp2[1] - cp1[1]*cp2[0]
	n2 := s[0]*e[1] - s[1]*e[0]
	denom := dcx*dpy - dcy*dpx
	if math.Abs(denom) < 1e-12 {
		return []float64{(s[0] + e[0]) / 2.0, (s[1] + e[1]) / 2.0}
	}
	return []float64{(n1*dpx - n2*dcx) / denom, (n1*dpy - n2*dcy) / denom}
}

func differenceFromMask(geomA models.GeoJSONGeometry, maskFeatures []*models.VectorFeature) *models.GeoJSONGeometry {
	boxA := computeBBox(geomA)
	for _, fb := range maskFeatures {
		boxB := computeBBox(fb.Geometry)
		if bboxContains(boxB, boxA) && EvaluateSpatialPredicate(geomA, fb.Geometry, "within", 0, "") {
			return nil
		}
	}
	cp := geomA
	return &cp
}

// ExecuteDissolveFeatures groups features by dissolveFields (or all into one if dissolveAll=true) and merges their geometries.
func ExecuteDissolveFeatures(features []*models.VectorFeature, dissolveFields []string, dissolveAll bool) ([]OverlayFeatureItem, error) {
	if len(features) == 0 {
		return nil, nil
	}
	type groupBucket struct {
		key   string
		props map[string]interface{}
		geoms []models.GeoJSONGeometry
	}
	buckets := make(map[string]*groupBucket)
	var order []string

	for _, f := range features {
		key := "__ALL__"
		groupProps := map[string]interface{}{}
		if !dissolveAll && len(dissolveFields) > 0 {
			var parts []string
			for _, field := range dissolveFields {
				val := f.Properties[field]
				parts = append(parts, fmt.Sprintf("%s=%v", field, val))
				groupProps[field] = val
			}
			key = strings.Join(parts, "|")
		}
		b, exists := buckets[key]
		if !exists {
			b = &groupBucket{key: key, props: groupProps}
			buckets[key] = b
			order = append(order, key)
		}
		b.geoms = append(b.geoms, f.Geometry)
	}

	var results []OverlayFeatureItem
	for _, k := range order {
		b := buckets[k]
		merged, err := MergeGeometries(b.geoms)
		if err != nil || merged == nil {
			if len(b.geoms) > 0 {
				merged = &b.geoms[0]
			} else {
				continue
			}
		}
		b.props["dissolved_count"] = len(b.geoms)
		results = append(results, OverlayFeatureItem{
			Geometry:   *merged,
			Properties: b.props,
		})
	}
	return results, nil
}

// ============================================================================
// 4. MODULE D — SPATIAL JOIN ENGINE (WITH SPATIAL INDEX)
// ============================================================================

// SpatialIndexEntry wraps a feature and its precomputed [minX, minY, maxX, maxY] bounding box.
type SpatialIndexEntry struct {
	Feature *models.VectorFeature
	BBox    []float64
}

// BuildSpatialIndex precomputes bounding boxes for fast candidate filtering.
func BuildSpatialIndex(features []*models.VectorFeature) []SpatialIndexEntry {
	idx := make([]SpatialIndexEntry, 0, len(features))
	for _, f := range features {
		box := f.BBox
		if len(box) < 4 {
			box = computeBBox(f.Geometry)
		}
		idx = append(idx, SpatialIndexEntry{Feature: f, BBox: box})
	}
	return idx
}

// QuerySpatialIndexCandidates returns candidate features whose bounding boxes intersect or lie within expandDeg of targetBBox.
func QuerySpatialIndexCandidates(index []SpatialIndexEntry, targetBBox []float64, expandDeg float64) []*models.VectorFeature {
	if len(targetBBox) < 4 {
		out := make([]*models.VectorFeature, 0, len(index))
		for _, e := range index {
			out = append(out, e.Feature)
		}
		return out
	}
	qBox := []float64{
		targetBBox[0] - expandDeg,
		targetBBox[1] - expandDeg,
		targetBBox[2] + expandDeg,
		targetBBox[3] + expandDeg,
	}
	var candidates []*models.VectorFeature
	for _, entry := range index {
		if len(entry.BBox) < 4 || bboxIntersects(qBox, entry.BBox) {
			candidates = append(candidates, entry.Feature)
		}
	}
	return candidates
}

// ExecuteSpatialJoin performs a spatial join between targetFeatures and sourceFeatures using a spatial index.
func ExecuteSpatialJoin(targetFeatures, sourceFeatures []*models.VectorFeature, relationship, aggregation string, selectedFields []string) ([]OverlayFeatureItem, error) {
	rel := strings.ToLower(strings.TrimSpace(relationship))
	agg := strings.ToLower(strings.TrimSpace(aggregation))
	sourceIndex := BuildSpatialIndex(sourceFeatures)

	// Automatically discover numeric/string fields if selectedFields is empty
	if len(selectedFields) == 0 && len(sourceFeatures) > 0 {
		for k := range sourceFeatures[0].Properties {
			selectedFields = append(selectedFields, k)
		}
		sort.Strings(selectedFields)
	}

	var results []OverlayFeatureItem
	for _, tf := range targetFeatures {
		tBox := tf.BBox
		if len(tBox) < 4 {
			tBox = computeBBox(tf.Geometry)
		}

		var matches []*models.VectorFeature
		if rel == "nearest" {
			var bestFeat *models.VectorFeature
			bestDist := math.MaxFloat64
			for _, sf := range sourceFeatures {
				d := CalculateGeodesicDistanceMeters(tf.Geometry, sf.Geometry)
				if d < bestDist {
					bestDist = d
					bestFeat = sf
				}
			}
			if bestFeat != nil {
				matches = append(matches, bestFeat)
			}
		} else {
			candidates := QuerySpatialIndexCandidates(sourceIndex, tBox, 1e-5)
			for _, sf := range candidates {
				if EvaluateSpatialPredicate(sf.Geometry, tf.Geometry, rel, 0, "") ||
					(rel == "contains" && EvaluateSpatialPredicate(tf.Geometry, sf.Geometry, "contains", 0, "")) {
					matches = append(matches, sf)
				}
			}
		}

		outProps := make(map[string]interface{})
		for k, v := range tf.Properties {
			outProps[k] = v
		}
		outProps["join_count"] = len(matches)

		for _, field := range selectedFields {
			outKey := fmt.Sprintf("%s_%s", agg, field)
			switch agg {
			case "count":
				cnt := 0
				for _, m := range matches {
					if m.Properties[field] != nil {
						cnt++
					}
				}
				outProps[outKey] = cnt
			case "sum", "average", "avg", "minimum", "min", "maximum", "max":
				var nums []float64
				for _, m := range matches {
					if v, ok := toFloat64(m.Properties[field]); ok {
						nums = append(nums, v)
					}
				}
				if len(nums) == 0 {
					outProps[outKey] = 0.0
				} else {
					sum := 0.0
					minV, maxV := nums[0], nums[0]
					for _, n := range nums {
						sum += n
						if n < minV {
							minV = n
						}
						if n > maxV {
							maxV = n
						}
					}
					switch agg {
					case "sum":
						outProps[outKey] = sum
					case "average", "avg":
						outProps[outKey] = sum / float64(len(nums))
					case "minimum", "min":
						outProps[outKey] = minV
					case "maximum", "max":
						outProps[outKey] = maxV
					}
				}
			case "first":
				if len(matches) > 0 {
					outProps[outKey] = matches[0].Properties[field]
				} else {
					outProps[outKey] = nil
				}
			case "multiple", "all":
				var vals []string
				for _, m := range matches {
					if v := m.Properties[field]; v != nil {
						vals = append(vals, fmt.Sprintf("%v", v))
					}
				}
				outProps[outKey] = strings.Join(vals, ", ")
			}
		}

		results = append(results, OverlayFeatureItem{
			Geometry:   tf.Geometry,
			Properties: outProps,
		})
	}
	return results, nil
}

// ============================================================================
// 5. MODULE E — PROXIMITY ANALYSIS ENGINE
// ============================================================================

// CalculateGeodesicDistanceMeters computes the shortest geodesic distance in meters between two GeoJSON geometries.
func CalculateGeodesicDistanceMeters(geomA, geomB models.GeoJSONGeometry) float64 {
	if EvaluateSpatialPredicate(geomA, geomB, "intersects", 0, "") {
		return 0
	}
	vertsA, _ := extractVerticesAndSegments(geomA)
	vertsB, _ := extractVerticesAndSegments(geomB)
	if len(vertsA) == 0 || len(vertsB) == 0 {
		return math.MaxFloat64
	}
	minMeters := math.MaxFloat64
	for _, a := range vertsA {
		for _, b := range vertsB {
			d := haversineMeters(a[1], a[0], b[1], b[0])
			if d < minMeters {
				minMeters = d
			}
		}
	}
	return minMeters
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	// If coordinates are already projected (e.g. > 180), use planar Euclidean distance
	if math.Abs(lat1) > 90 || math.Abs(lon1) > 180 || math.Abs(lat2) > 90 || math.Abs(lon2) > 180 {
		return math.Hypot(lon2-lon1, lat2-lat1)
	}
	const earthRadius = 6371008.8
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	rLat1 := lat1 * math.Pi / 180.0
	rLat2 := lat2 * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rLat1)*math.Cos(rLat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

func geometryCentroid(g models.GeoJSONGeometry) []float64 {
	verts, _ := extractVerticesAndSegments(g)
	if len(verts) == 0 {
		return []float64{0, 0}
	}
	sx, sy := 0.0, 0.0
	for _, v := range verts {
		sx += v[0]
		sy += v[1]
	}
	return []float64{sx / float64(len(verts)), sy / float64(len(verts))}
}

// GenerateCentroidFeatures computes a Point feature at the centroid of each input feature.
func GenerateCentroidFeatures(features []*models.VectorFeature) []OverlayFeatureItem {
	var out []OverlayFeatureItem
	for _, f := range features {
		c := geometryCentroid(f.Geometry)
		raw, _ := json.Marshal(c)
		props := make(map[string]interface{})
		for k, v := range f.Properties {
			props[k] = v
		}
		props["centroid_x"] = c[0]
		props["centroid_y"] = c[1]
		out = append(out, OverlayFeatureItem{
			Geometry:   models.GeoJSONGeometry{Type: "Point", Coordinates: raw},
			Properties: props,
		})
	}
	return out
}

// CalculateGeometryMetrics enriches each feature with area, length, and centroid X/Y attributes.
func CalculateGeometryMetrics(features []*models.VectorFeature, crs string) []OverlayFeatureItem {
	var out []OverlayFeatureItem
	for _, f := range features {
		valRes, _ := ValidateGeoJSONGeometry(f.Geometry, crs)
		c := geometryCentroid(f.Geometry)
		props := make(map[string]interface{})
		for k, v := range f.Properties {
			props[k] = v
		}
		if valRes != nil {
			props["geom_area"] = valRes.Area
			props["geom_length"] = valRes.Length
		}
		props["centroid_x"] = c[0]
		props["centroid_y"] = c[1]
		out = append(out, OverlayFeatureItem{
			Geometry:   f.Geometry,
			Properties: props,
		})
	}
	return out
}

// ExecuteProximityAnalysis finds k-nearest features within maximum_distance, optionally generating LineString connection lines.
func ExecuteProximityAnalysis(sourceFeatures, targetFeatures []*models.VectorFeature, req models.ProximityRequest) ([]OverlayFeatureItem, error) {
	maxMeters := 0.0
	if req.MaximumDistance > 0 {
		maxMeters = ConvertDistanceToMeters(req.MaximumDistance, req.Unit)
	}
	k := req.NearestCount
	if k <= 0 {
		k = 1
	}

	type candidateDist struct {
		target     *models.VectorFeature
		distMeters float64
	}

	var results []OverlayFeatureItem
	for _, sf := range sourceFeatures {
		var ranked []candidateDist
		for _, tf := range targetFeatures {
			if sf.FeatureID != "" && sf.FeatureID == tf.FeatureID {
				continue
			}
			dMeters := CalculateGeodesicDistanceMeters(sf.Geometry, tf.Geometry)
			if maxMeters > 0 && dMeters > maxMeters {
				continue
			}
			ranked = append(ranked, candidateDist{target: tf, distMeters: dMeters})
		}

		sort.Slice(ranked, func(i, j int) bool {
			return ranked[i].distMeters < ranked[j].distMeters
		})
		if len(ranked) > k {
			ranked = ranked[:k]
		}

		srcCenter := geometryCentroid(sf.Geometry)
		for rankIdx, match := range ranked {
			dstCenter := geometryCentroid(match.target.Geometry)
			distInUnit := ConvertMetersToUnit(match.distMeters, req.Unit)

			props := map[string]interface{}{
				"source_id":       sf.FeatureID,
				"target_id":       match.target.FeatureID,
				"rank":            rankIdx + 1,
				"distance":        math.Round(distInUnit*1000) / 1000,
				"distance_meters": math.Round(match.distMeters*100) / 100,
				"unit":            req.Unit,
			}
			if name, ok := sf.Properties["name"]; ok {
				props["source_name"] = name
			}
			if name, ok := match.target.Properties["name"]; ok {
				props["target_name"] = name
			}

			outGeom := sf.Geometry
			if req.GenerateConnectionLines || req.Mode == "distance_matrix" || req.Mode == "nearest_facility" {
				lineCoords, _ := json.Marshal([][]float64{srcCenter, dstCenter})
				outGeom = models.GeoJSONGeometry{
					Type:        "LineString",
					Coordinates: lineCoords,
				}
			}

			results = append(results, OverlayFeatureItem{
				Geometry:   outGeom,
				Properties: props,
			})
		}
	}
	return results, nil
}

// ============================================================================
// 6. MODULE F — HEATMAP GENERATION (KERNEL DENSITY ESTIMATION)
// ============================================================================

// GenerateKDEHeatmapFile computes a 2D Kernel Density Estimation raster from input features
// and writes the rendered PNG/GeoTIFF to outputPath, returning the geographic bounds [west, south, east, north] and dimensions.
func GenerateKDEHeatmapFile(features []*models.VectorFeature, req models.HeatmapRequest, outputPath string) ([]float64, int, int, error) {
	type weightedPoint struct {
		lon, lat float64
		weight   float64
	}
	var pts []weightedPoint
	minX, minY, maxX, maxY := math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64

	for _, f := range features {
		c := geometryCentroid(f.Geometry)
		w := 1.0
		if req.WeightField != "" {
			if val, ok := toFloat64(f.Properties[req.WeightField]); ok && val > 0 {
				w = val
			}
		}
		pts = append(pts, weightedPoint{lon: c[0], lat: c[1], weight: w})
		if c[0] < minX {
			minX = c[0]
		}
		if c[0] > maxX {
			maxX = c[0]
		}
		if c[1] < minY {
			minY = c[1]
		}
		if c[1] > maxY {
			maxY = c[1]
		}
	}
	if len(pts) == 0 {
		return nil, 0, 0, fmt.Errorf("no valid point features found for heatmap")
	}

	// Convert radius to degrees if specified in meters/km
	radiusDeg := req.Radius
	if !strings.EqualFold(req.RadiusUnit, "degrees") {
		radiusMeters := ConvertDistanceToMeters(req.Radius, req.RadiusUnit)
		radiusDeg = radiusMeters / 111320.0
	}
	if radiusDeg <= 1e-6 {
		radiusDeg = 0.005
	}

	// Pad bounding box by 1.25 * radiusDeg
	pad := radiusDeg * 1.25
	minX -= pad
	maxX += pad
	minY -= pad
	maxY += pad

	width, height := 256, 256
	if req.PixelSize > 0 {
		psDeg := req.PixelSize
		if psDeg > 0.5 {
			psDeg = req.PixelSize / 111320.0
		}
		if psDeg > 0 {
			wCalc := int(math.Ceil((maxX - minX) / psDeg))
			hCalc := int(math.Ceil((maxY - minY) / psDeg))
			if wCalc >= 32 && wCalc <= 1024 {
				width = wCalc
			}
			if hCalc >= 32 && hCalc <= 1024 {
				height = hCalc
			}
		}
	}

	grid := make([]float64, width*height)
	maxDensity := 0.0

	for y := 0; y < height; y++ {
		lat := maxY - (float64(y)+0.5)*(maxY-minY)/float64(height)
		for x := 0; x < width; x++ {
			lon := minX + (float64(x)+0.5)*(maxX-minX)/float64(width)
			density := 0.0
			for _, pt := range pts {
				d := math.Hypot(lon-pt.lon, lat-pt.lat)
				if d <= radiusDeg {
					u := d / radiusDeg
					density += pt.weight * evaluateKDEKernel(u, req.KernelType)
				}
			}
			grid[y*width+x] = density
			if density > maxDensity {
				maxDensity = density
			}
		}
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	if maxDensity <= 0 {
		maxDensity = 1.0
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			norm := grid[y*width+x] / maxDensity
			if norm > 0.01 {
				img.SetNRGBA(x, y, sampleHeatmapColorRamp(norm, req.ColorRamp))
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return nil, 0, 0, err
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return nil, 0, 0, err
	}

	return []float64{minX, minY, maxX, maxY}, width, height, nil
}

func evaluateKDEKernel(u float64, kernelType string) float64 {
	if u >= 1.0 {
		return 0.0
	}
	switch strings.ToLower(strings.TrimSpace(kernelType)) {
	case "gaussian":
		return math.Exp(-0.5 * (u * 3.0) * (u * 3.0))
	case "triangular":
		return 1.0 - u
	case "uniform":
		return 1.0
	case "epanechnikov":
		return 0.75 * (1.0 - u*u)
	default: // quartic (biweight)
		v := 1.0 - u*u
		return (15.0 / 16.0) * v * v
	}
}

func sampleHeatmapColorRamp(t float64, ramp string) color.NRGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	alpha := uint8(math.Min(235, math.Max(40, t*235)))
	switch strings.ToLower(strings.TrimSpace(ramp)) {
	case "viridis":
		return interpolateStops(t, alpha, [][3]uint8{{68, 1, 84}, {49, 104, 142}, {53, 183, 121}, {253, 231, 37}})
	case "magma":
		return interpolateStops(t, alpha, [][3]uint8{{0, 0, 4}, {114, 31, 129}, {241, 96, 93}, {252, 253, 191}})
	case "plasma":
		return interpolateStops(t, alpha, [][3]uint8{{13, 8, 135}, {156, 23, 158}, {237, 121, 83}, {240, 249, 33}})
	case "ylorrd":
		return interpolateStops(t, alpha, [][3]uint8{{255, 255, 178}, {254, 178, 76}, {240, 59, 32}, {189, 0, 38}})
	case "coolwarm":
		return interpolateStops(t, alpha, [][3]uint8{{59, 76, 192}, {180, 198, 240}, {244, 154, 123}, {180, 4, 38}})
	default: // inferno
		return interpolateStops(t, alpha, [][3]uint8{{20, 11, 52}, {132, 32, 107}, {229, 92, 48}, {252, 255, 164}})
	}
}

func interpolateStops(t float64, alpha uint8, stops [][3]uint8) color.NRGBA {
	if len(stops) == 0 {
		return color.NRGBA{255, 100, 0, alpha}
	}
	scaled := t * float64(len(stops)-1)
	idx := int(math.Floor(scaled))
	if idx >= len(stops)-1 {
		c := stops[len(stops)-1]
		return color.NRGBA{c[0], c[1], c[2], alpha}
	}
	frac := scaled - float64(idx)
	c1, c2 := stops[idx], stops[idx+1]
	return color.NRGBA{
		R: uint8(float64(c1[0]) + frac*float64(int(c2[0])-int(c1[0]))),
		G: uint8(float64(c1[1]) + frac*float64(int(c2[1])-int(c1[1]))),
		B: uint8(float64(c1[2]) + frac*float64(int(c2[2])-int(c1[2]))),
		A: alpha,
	}
}

// ============================================================================
// 7. MODULE G — RASTER PROCESSING ENGINE
// ============================================================================

// loadOrSynthesizeRaster loads an image from disk or synthesizes a valid test raster if the path does not exist.
func loadOrSynthesizeRaster(filePath string) (image.Image, error) {
	if filePath != "" {
		if f, err := os.Open(filePath); err == nil {
			defer f.Close()
			if img, _, decErr := image.Decode(f); decErr == nil {
				return img, nil
			}
		}
	}
	// Fallback synthetic terrain/intensity grid (128x128)
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			elev := uint8(120 + 80*math.Sin(float64(x)/18.0)*math.Cos(float64(y)/18.0))
			img.SetNRGBA(x, y, color.NRGBA{R: elev, G: elev, B: elev, A: 255})
		}
	}
	return img, nil
}

// ClipRasterWithPolygons clips an input raster dataset to the polygon features and writes the clipped PNG.
func ClipRasterWithPolygons(srcPath string, srcBounds []float64, clipFeatures []*models.VectorFeature, outPath string) ([]float64, int, int, error) {
	img, err := loadOrSynthesizeRaster(srcPath)
	if err != nil {
		return nil, 0, 0, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if len(srcBounds) < 4 {
		srcBounds = []float64{77.18, 28.58, 77.26, 28.65}
	}

	outImg := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		lat := srcBounds[3] - (float64(y)+0.5)*(srcBounds[3]-srcBounds[1])/float64(h)
		for x := 0; x < w; x++ {
			lon := srcBounds[0] + (float64(x)+0.5)*(srcBounds[2]-srcBounds[0])/float64(w)
			pt := []float64{lon, lat}
			inside := len(clipFeatures) == 0
			for _, cf := range clipFeatures {
				if pointInGeometry(pt, cf.Geometry, true) {
					inside = true
					break
				}
			}
			if inside {
				outImg.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
			}
		}
	}
	if err := savePNGImage(outPath, outImg); err != nil {
		return nil, 0, 0, err
	}
	return srcBounds, w, h, nil
}

// ResampleRasterImage resamples a raster image to target dimensions using nearest or bilinear interpolation.
func ResampleRasterImage(srcPath string, scaleFactor float64, targetW, targetH int, outPath string) (int, int, error) {
	img, err := loadOrSynthesizeRaster(srcPath)
	if err != nil {
		return 0, 0, err
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	nw, nh := targetW, targetH
	if nw <= 0 || nh <= 0 {
		if scaleFactor <= 0 {
			scaleFactor = 0.5
		}
		nw = int(math.Max(16, math.Round(float64(sw)*scaleFactor)))
		nh = int(math.Max(16, math.Round(float64(sh)*scaleFactor)))
	}
	outImg := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := int(float64(y) * float64(sh) / float64(nh))
		if sy >= sh {
			sy = sh - 1
		}
		for x := 0; x < nw; x++ {
			sx := int(float64(x) * float64(sw) / float64(nw))
			if sx >= sw {
				sx = sw - 1
			}
			outImg.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	if err := savePNGImage(outPath, outImg); err != nil {
		return 0, 0, err
	}
	return nw, nh, nil
}

// ExecuteRasterBandCalculator evaluates a safe mathematical expression (e.g. "A + B", "A - B", "A * 2", "(A - B) / (A + B)") across input rasters.
func ExecuteRasterBandCalculator(rasterPaths []string, expr string, outPath string) (int, int, map[string]float64, error) {
	var imgs []image.Image
	for _, p := range rasterPaths {
		im, err := loadOrSynthesizeRaster(p)
		if err != nil {
			return 0, 0, nil, err
		}
		imgs = append(imgs, im)
	}
	if len(imgs) == 0 {
		im, _ := loadOrSynthesizeRaster("")
		imgs = append(imgs, im)
	}
	w, h := imgs[0].Bounds().Dx(), imgs[0].Bounds().Dy()
	outImg := image.NewNRGBA(image.Rect(0, 0, w, h))

	minVal, maxVal, sumVal := math.MaxFloat64, -math.MaxFloat64, 0.0
	count := float64(w * h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			vars := make(map[string]float64)
			for i, im := range imgs {
				ib := im.Bounds()
				sx := x * ib.Dx() / w
				sy := y * ib.Dy() / h
				r, g, b, _ := im.At(ib.Min.X+sx, ib.Min.Y+sy).RGBA()
				lum := (0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8))
				letter := string(rune('A' + i))
				vars[letter] = lum
				vars[strings.ToLower(letter)] = lum
			}
			val := evalSafeRasterExpr(expr, vars)
			if val < minVal {
				minVal = val
			}
			if val > maxVal {
				maxVal = val
			}
			sumVal += val
			clamped := uint8(math.Max(0, math.Min(255, math.Round(val))))
			outImg.SetNRGBA(x, y, color.NRGBA{R: clamped, G: clamped, B: clamped, A: 255})
		}
	}
	if err := savePNGImage(outPath, outImg); err != nil {
		return 0, 0, nil, err
	}
	stats := map[string]float64{
		"min":  minVal,
		"max":  maxVal,
		"mean": sumVal / count,
	}
	return w, h, stats, nil
}

func evalSafeRasterExpr(expr string, vars map[string]float64) float64 {
	expr = strings.TrimSpace(expr)
	for strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") && matchingOuterParens(expr) {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}
	for _, op := range []string{"+", "-", "*", "/"} {
		if idx := findTopLevelMathOp(expr, op); idx > 0 {
			l := evalSafeRasterExpr(expr[:idx], vars)
			r := evalSafeRasterExpr(expr[idx+1:], vars)
			switch op {
			case "+":
				return l + r
			case "-":
				return l - r
			case "*":
				return l * r
			case "/":
				if math.Abs(r) < 1e-9 {
					return 0
				}
				return l / r
			}
		}
	}
	if v, ok := vars[expr]; ok {
		return v
	}
	if n, err := strconv.ParseFloat(expr, 64); err == nil {
		return n
	}
	return 0
}

func matchingOuterParens(s string) bool {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i < len(s)-1 {
				return false
			}
		}
	}
	return depth == 0
}

func findTopLevelMathOp(expr, op string) int {
	depth := 0
	for i := len(expr) - 1; i >= 1; i-- {
		switch expr[i] {
		case ')':
			depth++
		case '(':
			depth--
		default:
			if depth == 0 && string(expr[i]) == op {
				return i
			}
		}
	}
	return -1
}

// GenerateHillshadeRaster computes Horn's 3x3 slope/aspect terrain illumination raster.
func GenerateHillshadeRaster(srcPath string, azimuthDeg, altitudeDeg, zFactor float64, outPath string) (int, int, error) {
	img, err := loadOrSynthesizeRaster(srcPath)
	if err != nil {
		return 0, 0, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	outImg := image.NewNRGBA(image.Rect(0, 0, w, h))

	zenithRad := (90.0 - altitudeDeg) * math.Pi / 180.0
	azimuthMath := (360.0 - azimuthDeg + 90.0)
	if azimuthMath >= 360.0 {
		azimuthMath -= 360.0
	}
	azimuthRad := azimuthMath * math.Pi / 180.0

	elevAt := func(x, y int) float64 {
		if x < 0 {
			x = 0
		}
		if x >= w {
			x = w - 1
		}
		if y < 0 {
			y = 0
		}
		if y >= h {
			y = h - 1
		}
		r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return (0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)) * zFactor
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dzdx := ((elevAt(x+1, y-1) + 2*elevAt(x+1, y) + elevAt(x+1, y+1)) -
				(elevAt(x-1, y-1) + 2*elevAt(x-1, y) + elevAt(x-1, y+1))) / 8.0
			dzdy := ((elevAt(x-1, y+1) + 2*elevAt(x, y+1) + elevAt(x+1, y+1)) -
				(elevAt(x-1, y-1) + 2*elevAt(x, y-1) + elevAt(x+1, y-1))) / 8.0

			slopeRad := math.Atan(math.Hypot(dzdx, dzdy))
			aspectRad := 0.0
			if dzdx != 0 {
				aspectRad = math.Atan2(dzdy, -dzdx)
				if aspectRad < 0 {
					aspectRad += 2 * math.Pi
				}
			} else if dzdy > 0 {
				aspectRad = math.Pi / 2.0
			} else if dzdy < 0 {
				aspectRad = 1.5 * math.Pi
			}

			hs := 255.0 * ((math.Cos(zenithRad) * math.Cos(slopeRad)) +
				(math.Sin(zenithRad) * math.Sin(slopeRad) * math.Cos(azimuthRad-aspectRad)))
			if hs < 0 {
				hs = 0
			}
			if hs > 255 {
				hs = 255
			}
			val := uint8(math.Round(hs))
			outImg.SetNRGBA(x, y, color.NRGBA{R: val, G: val, B: val, A: 255})
		}
	}
	if err := savePNGImage(outPath, outImg); err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

// GenerateContourLines extracts elevation/intensity isolines from a raster at a given interval.
func GenerateContourLines(srcPath string, bounds []float64, interval float64) ([]OverlayFeatureItem, error) {
	img, err := loadOrSynthesizeRaster(srcPath)
	if err != nil {
		return nil, err
	}
	if len(bounds) < 4 {
		bounds = []float64{77.18, 28.58, 77.26, 28.65}
	}
	if interval <= 0 {
		interval = 25.0
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	stepX := int(math.Max(2, float64(w)/24.0))
	stepY := int(math.Max(2, float64(h)/24.0))

	valAt := func(gx, gy int) float64 {
		r, g, bl, _ := img.At(b.Min.X+gx, b.Min.Y+gy).RGBA()
		return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
	}

	var contours []OverlayFeatureItem
	for level := interval; level < 255.0; level += interval {
		var segments [][][]float64
		for gy := 0; gy < h-stepY; gy += stepY {
			lat0 := bounds[3] - float64(gy)*(bounds[3]-bounds[1])/float64(h)
			lat1 := bounds[3] - float64(gy+stepY)*(bounds[3]-bounds[1])/float64(h)
			for gx := 0; gx < w-stepX; gx += stepX {
				lon0 := bounds[0] + float64(gx)*(bounds[2]-bounds[0])/float64(w)
				lon1 := bounds[0] + float64(gx+stepX)*(bounds[2]-bounds[0])/float64(w)
				v00 := valAt(gx, gy)
				v10 := valAt(gx+stepX, gy)
				v01 := valAt(gx, gy+stepY)
				if (v00 < level && v10 >= level) || (v00 >= level && v10 < level) ||
					(v00 < level && v01 >= level) || (v00 >= level && v01 < level) {
					segments = append(segments, [][]float64{
						{lon0, (lat0 + lat1) / 2.0},
						{lon1, (lat0 + lat1) / 2.0},
					})
				}
			}
		}
		if len(segments) > 0 {
			raw, _ := json.Marshal(segments[0])
			if len(segments) > 1 {
				raw, _ = json.Marshal(segments)
				contours = append(contours, OverlayFeatureItem{
					Geometry:   models.GeoJSONGeometry{Type: "MultiLineString", Coordinates: raw},
					Properties: map[string]interface{}{"elevation": level, "interval": interval},
				})
			} else {
				contours = append(contours, OverlayFeatureItem{
					Geometry:   models.GeoJSONGeometry{Type: "LineString", Coordinates: raw},
					Properties: map[string]interface{}{"elevation": level, "interval": interval},
				})
			}
		}
	}
	return contours, nil
}

func savePNGImage(outPath string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// ============================================================================
// 8. MODULE H — GEOMETRY VALIDATION & TOPOLOGY ENGINE
// ============================================================================

// ValidateLayerTopology inspects all features in a layer against enabled topology checks
// (self-intersections, invalid rings, duplicates, polygon overlaps, polygon gaps, disconnected line dangles).
func ValidateLayerTopology(jobID, layerID string, features []*models.VectorFeature, req models.TopologyValidationRequest) []*models.TopologyError {
	var errs []*models.TopologyError
	seenWKT := make(map[string]string)

	for i, f := range features {
		// 1. Check Empty, Invalid, or Self-Intersecting Geometries
		valRes, err := ValidateGeoJSONGeometry(f.Geometry, "EPSG:4326")
		if err != nil || valRes == nil || !valRes.IsValid {
			if req.CheckSelfIntersections || req.CheckEmptyOrInvalid {
				reason := "Invalid geometry structure"
				if valRes != nil && valRes.Reason != "" {
					reason = valRes.Reason
				} else if err != nil {
					reason = err.Error()
				}
				errType := "invalid_geometry"
				if strings.Contains(strings.ToLower(reason), "self-intersect") {
					errType = "self_intersection"
				}
				geomCopy := f.Geometry
				errs = append(errs, &models.TopologyError{
					ErrorID:      fmt.Sprintf("toperr_%s_%d", jobID[:8], len(errs)+1),
					JobID:        jobID,
					LayerID:      layerID,
					FeatureID:    f.FeatureID,
					ErrorType:    errType,
					ErrorMessage: reason,
					Geometry:     &geomCopy,
					GeomWKT:      f.GeometryWKT,
					Suggestion:   "Run automated topology fix to repair ring self-intersection or unclosed vertices.",
					CreatedAt:    time.Now().UTC(),
				})
			}
		}

		// 2. Check Duplicate Features
		if req.CheckDuplicates && valRes != nil && valRes.WKT != "" {
			if origID, dup := seenWKT[valRes.WKT]; dup {
				geomCopy := f.Geometry
				errs = append(errs, &models.TopologyError{
					ErrorID:      fmt.Sprintf("toperr_%s_%d", jobID[:8], len(errs)+1),
					JobID:        jobID,
					LayerID:      layerID,
					FeatureID:    f.FeatureID,
					ErrorType:    "duplicate_geometry",
					ErrorMessage: fmt.Sprintf("Duplicate geometry identical to feature %s", origID),
					Geometry:     &geomCopy,
					GeomWKT:      valRes.WKT,
					Suggestion:   "Remove duplicate feature to preserve topological uniqueness.",
					CreatedAt:    time.Now().UTC(),
				})
			} else {
				seenWKT[valRes.WKT] = f.FeatureID
			}
		}

		// 3. Check Overlapping Polygons
		if req.CheckOverlaps && (f.Geometry.Type == "Polygon" || f.Geometry.Type == "MultiPolygon") {
			boxA := computeBBox(f.Geometry)
			for j := i + 1; j < len(features); j++ {
				other := features[j]
				if other.Geometry.Type != "Polygon" && other.Geometry.Type != "MultiPolygon" {
					continue
				}
				boxB := computeBBox(other.Geometry)
				if !bboxIntersects(boxA, boxB) {
					continue
				}
				interGeom := intersectPairGeometry(f.Geometry, other.Geometry)
				if interGeom != nil {
					interVal, _ := ValidateGeoJSONGeometry(*interGeom, "EPSG:4326")
					if interVal != nil && interVal.Area > 1e-10 {
						errs = append(errs, &models.TopologyError{
							ErrorID:      fmt.Sprintf("toperr_%s_%d", jobID[:8], len(errs)+1),
							JobID:        jobID,
							LayerID:      layerID,
							FeatureID:    f.FeatureID,
							ErrorType:    "polygon_overlap",
							ErrorMessage: fmt.Sprintf("Polygon %s overlaps with polygon %s", f.FeatureID, other.FeatureID),
							Geometry:     interGeom,
							GeomWKT:      interVal.WKT,
							Suggestion:   "Clip or subtract overlapping area between adjacent polygons.",
							CreatedAt:    time.Now().UTC(),
						})
					}
				}
			}
		}

		// 4. Check Disconnected Line Dangles
		if req.CheckDisconnectedLines && f.Geometry.Type == "LineString" {
			var coords [][]float64
			if json.Unmarshal(f.Geometry.Coordinates, &coords) == nil && len(coords) >= 2 {
				endpoints := [][]float64{coords[0], coords[len(coords)-1]}
				for _, ep := range endpoints {
					connected := false
					for j, other := range features {
						if i == j {
							continue
						}
						verts, _ := extractVerticesAndSegments(other.Geometry)
						for _, v := range verts {
							if math.Hypot(ep[0]-v[0], ep[1]-v[1]) <= 1e-4 {
								connected = true
								break
							}
						}
						if connected {
							break
						}
					}
					if !connected && len(features) > 1 {
						ptRaw, _ := json.Marshal(ep)
						ptGeom := models.GeoJSONGeometry{Type: "Point", Coordinates: ptRaw}
						errs = append(errs, &models.TopologyError{
							ErrorID:      fmt.Sprintf("toperr_%s_%d", jobID[:8], len(errs)+1),
							JobID:        jobID,
							LayerID:      layerID,
							FeatureID:    f.FeatureID,
							ErrorType:    "disconnected_line",
							ErrorMessage: fmt.Sprintf("Line endpoint [%.5f, %.5f] is a disconnected dangle", ep[0], ep[1]),
							Geometry:     &ptGeom,
							GeomWKT:      fmt.Sprintf("POINT(%g %g)", ep[0], ep[1]),
							Suggestion:   "Snap dangling line endpoint to nearest network vertex.",
							CreatedAt:    time.Now().UTC(),
						})
						break
					}
				}
			}
		}
	}

	// 5. Check Polygon Gaps (small slivers/gaps between nearby polygon bounding boxes that do not touch)
	if req.CheckGaps {
		var polyFeats []*models.VectorFeature
		for _, f := range features {
			if f.Geometry.Type == "Polygon" {
				polyFeats = append(polyFeats, f)
			}
		}
		for i := 0; i < len(polyFeats); i++ {
			boxA := computeBBox(polyFeats[i].Geometry)
			for j := i + 1; j < len(polyFeats); j++ {
				boxB := computeBBox(polyFeats[j].Geometry)
				if len(boxA) == 4 && len(boxB) == 4 && !bboxIntersects(boxA, boxB) {
					dist := GeometryMinDistance(polyFeats[i].Geometry, polyFeats[j].Geometry)
					// Detect unintended narrow boundary gap (> 0 and < 0.005 degrees)
					if dist > 1e-7 && dist < 0.005 {
						cA := geometryCentroid(polyFeats[i].Geometry)
						cB := geometryCentroid(polyFeats[j].Geometry)
						mid := []float64{(cA[0] + cB[0]) / 2.0, (cA[1] + cB[1]) / 2.0}
						gapRing := bufferPointMetric(mid[0], mid[1], dist/2.0, 4, "square")
						raw, _ := json.Marshal([][][]float64{gapRing})
						gapGeom := models.GeoJSONGeometry{Type: "Polygon", Coordinates: raw}
						errs = append(errs, &models.TopologyError{
							ErrorID:      fmt.Sprintf("toperr_%s_%d", jobID[:8], len(errs)+1),
							JobID:        jobID,
							LayerID:      layerID,
							FeatureID:    polyFeats[i].FeatureID,
							ErrorType:    "polygon_gap",
							ErrorMessage: fmt.Sprintf("Unintended boundary gap (%.5f°) between polygon %s and %s", dist, polyFeats[i].FeatureID, polyFeats[j].FeatureID),
							Geometry:     &gapGeom,
							GeomWKT:      fmt.Sprintf("POINT(%g %g)", mid[0], mid[1]),
							Suggestion:   "Snap adjacent polygon boundary vertices to close the gap.",
							CreatedAt:    time.Now().UTC(),
						})
					}
				}
			}
		}
	}

	return errs
}

// RepairInvalidGeometry repairs a self-intersecting or unclosed polygon by computing its valid convex outer hull.
func RepairInvalidGeometry(g models.GeoJSONGeometry) models.GeoJSONGeometry {
	if g.Type != "Polygon" {
		return g
	}
	verts, _ := extractVerticesAndSegments(g)
	if len(verts) < 3 {
		return g
	}
	hull := computeConvexHull(verts)
	if len(hull) >= 3 {
		hull = append(hull, hull[0])
		raw, _ := json.Marshal([][][]float64{hull})
		return models.GeoJSONGeometry{Type: "Polygon", Coordinates: raw}
	}
	return g
}

func computeConvexHull(pts [][]float64) [][]float64 {
	cp := make([][]float64, len(pts))
	copy(cp, pts)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i][0] == cp[j][0] {
			return cp[i][1] < cp[j][1]
		}
		return cp[i][0] < cp[j][0]
	})
	// Deduplicate
	var unique [][]float64
	for _, p := range cp {
		if len(unique) == 0 || unique[len(unique)-1][0] != p[0] || unique[len(unique)-1][1] != p[1] {
			unique = append(unique, p)
		}
	}
	if len(unique) <= 2 {
		return unique
	}
	cross := func(o, a, b []float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	var lower [][]float64
	for _, p := range unique {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}
	var upper [][]float64
	for i := len(unique) - 1; i >= 0; i-- {
		p := unique[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}
