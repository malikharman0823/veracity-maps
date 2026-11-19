package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

func TestGISPhase2_ProcessingEngineAllModules(t *testing.T) {
	server, _, _, _ := setupPersonalMapTestServer(t)
	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Register Alice (Owner) and Bob (Unauthorized Outsider)
	aliceToken, _ := registerAndLoginUser(t, server.URL, "alice-gis2@example.com", "Alice GIS2")
	bobToken, _ := registerAndLoginUser(t, server.URL, "bob-gis2@example.com", "Bob GIS2")

	// 2. Create Session & Map for Alice
	sessResp, sessBody := doGISRequest(t, client, "POST", server.URL+"/api/sessions", aliceToken, "", nil)
	if sessResp.StatusCode != http.StatusOK && sessResp.StatusCode != http.StatusCreated {
		t.Fatalf("create session failed: %s", string(sessBody))
	}
	var sessData struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(sessBody, &sessData)
	sessionID := sessData.Data.SessionID

	mapResp, mapBody := doGISRequest(t, client, "POST", server.URL+"/api/maps", aliceToken, sessionID, map[string]interface{}{
		"name":        "Delhi Phase 2 GIS Processing Workspace",
		"description": "End-to-end test map for Phase 2 GIS Processing Toolbox",
	})
	if mapResp.StatusCode != http.StatusCreated && mapResp.StatusCode != http.StatusOK {
		t.Fatalf("create map failed: %s", string(mapBody))
	}
	var mapData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(mapBody, &mapData)
	mapID := mapData.Data.ID

	// Create Seed Vector Layers:
	// Layer 1: Hospitals (Points with "beds" attribute)
	var hospitalsLayerID string
	{
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"layer_name":    "Delhi Hospitals",
			"layer_type":    "vector",
			"geometry_type": "Point",
			"crs":           "EPSG:4326",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create hospitals layer failed: %s", string(body))
		}
		var lData struct {
			Data *models.MapLayer `json:"data"`
		}
		_ = json.Unmarshal(body, &lData)
		hospitalsLayerID = lData.Data.LayerID

		// Add 3 Hospital Points
		pts := []struct {
			lon, lat float64
			name     string
			beds     int
			district string
		}{
			{77.2090, 28.6139, "AIIMS Hospital", 450, "Central"},
			{77.2167, 28.6315, "Ram Manohar Lohia", 320, "Central"},
			{77.2500, 28.5672, "Holy Family Hospital", 280, "South"},
		}
		for _, pt := range pts {
			r, b := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+hospitalsLayerID+"/features", aliceToken, sessionID, map[string]interface{}{
				"geometry": map[string]interface{}{
					"type":        "Point",
					"coordinates": []float64{pt.lon, pt.lat},
				},
				"properties": map[string]interface{}{
					"name":     pt.name,
					"beds":     pt.beds,
					"district": pt.district,
				},
			})
			if r.StatusCode != http.StatusCreated {
				t.Fatalf("insert hospital point failed: %s", string(b))
			}
		}
	}

	// Layer 2: Municipal Wards (Polygons)
	var wardsLayerID string
	{
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"layer_name":    "Delhi Municipal Wards",
			"layer_type":    "vector",
			"geometry_type": "Polygon",
			"crs":           "EPSG:4326",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create wards layer failed: %s", string(body))
		}
		var lData struct {
			Data *models.MapLayer `json:"data"`
		}
		_ = json.Unmarshal(body, &lData)
		wardsLayerID = lData.Data.LayerID

		// Ward 1 (Central - contains AIIMS and RML)
		doGISRequest(t, client, "POST", server.URL+"/api/layers/"+wardsLayerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type": "Polygon",
				"coordinates": [][][]float64{
					{{77.195, 28.600}, {77.230, 28.600}, {77.230, 28.645}, {77.195, 28.645}, {77.195, 28.600}},
				},
			},
			"properties": map[string]interface{}{"ward_name": "Connaught Ward", "zone": "Central", "pop": 85000},
		})
		// Ward 2 (South - contains Holy Family)
		doGISRequest(t, client, "POST", server.URL+"/api/layers/"+wardsLayerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type": "Polygon",
				"coordinates": [][][]float64{
					{{77.235, 28.550}, {77.270, 28.550}, {77.270, 28.590}, {77.235, 28.590}, {77.235, 28.550}},
				},
			},
			"properties": map[string]interface{}{"ward_name": "Okhla Ward", "zone": "South", "pop": 92000},
		})
	}

	// Layer 3: Flood Risk Zone (Overlapping Polygon for Overlay Analysis)
	var floodLayerID string
	{
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"layer_name":    "Flood Risk Zone",
			"layer_type":    "vector",
			"geometry_type": "Polygon",
			"crs":           "EPSG:4326",
		})
		var lData struct {
			Data *models.MapLayer `json:"data"`
		}
		_ = json.Unmarshal(body, &lData)
		floodLayerID = lData.Data.LayerID
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create flood layer failed: %s", string(body))
		}

		// Overlaps part of Ward 1
		doGISRequest(t, client, "POST", server.URL+"/api/layers/"+floodLayerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type": "Polygon",
				"coordinates": [][][]float64{
					{{77.210, 28.610}, {77.245, 28.610}, {77.245, 28.650}, {77.210, 28.650}, {77.210, 28.610}},
				},
			},
			"properties": map[string]interface{}{"risk_level": "High", "return_period": 50},
		})
	}

	// ========================================================================
	// MODULE A — GIS PROCESSING TOOLBOX & PRESETS
	// ========================================================================
	t.Run("ModuleA_ProcessingToolboxAndPresets", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "GET", server.URL+"/api/v1/processing/algorithms", aliceToken, sessionID, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list algorithms failed: %s", string(body))
		}
		var algResp struct {
			Success bool `json:"success"`
			Data    struct {
				Algorithms []models.ProcessingAlgorithm `json:"algorithms"`
				Total      int                          `json:"total"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &algResp)
		if !algResp.Success || algResp.Data.Total < 18 {
			t.Fatalf("expected at least 18 algorithms in catalog, got %d", algResp.Data.Total)
		}

		// Get single algorithm details
		detResp, detBody := doGISRequest(t, client, "GET", server.URL+"/api/v1/processing/algorithms/buffer", aliceToken, sessionID, nil)
		if detResp.StatusCode != http.StatusOK {
			t.Fatalf("get algorithm details failed: %s", string(detBody))
		}

		// Validate request parameters
		valResp, valBody := doGISRequest(t, client, "POST", server.URL+"/api/v1/processing/validate", aliceToken, sessionID, map[string]interface{}{
			"algorithm_name": "buffer",
			"parameters": map[string]interface{}{
				"input_layer_id": hospitalsLayerID,
				"distance":       1000,
				"unit":           "meters",
			},
		})
		if valResp.StatusCode != http.StatusOK {
			t.Fatalf("validate request failed: %s", string(valBody))
		}

		// Save, list, and delete processing preset
		prResp, prBody := doGISRequest(t, client, "POST", server.URL+"/api/v1/processing/presets", aliceToken, sessionID, map[string]interface{}{
			"algorithm_name": "buffer",
			"preset_name":    "2km Hospital Catchment",
			"parameters": map[string]interface{}{
				"distance": 2.0,
				"unit":     "kilometers",
				"dissolve": true,
			},
		})
		if prResp.StatusCode != http.StatusCreated {
			t.Fatalf("save preset failed: %s", string(prBody))
		}
		var prData struct {
			Data *models.ProcessingPreset `json:"data"`
		}
		_ = json.Unmarshal(prBody, &prData)

		listPrResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/v1/processing/presets?algorithm_name=buffer", aliceToken, sessionID, nil)
		if listPrResp.StatusCode != http.StatusOK {
			t.Fatalf("list presets failed")
		}

		delPrResp, _ := doGISRequest(t, client, "DELETE", server.URL+"/api/v1/processing/presets/"+prData.Data.PresetID, aliceToken, sessionID, nil)
		if delPrResp.StatusCode != http.StatusOK {
			t.Fatalf("delete preset failed")
		}
	})

	// ========================================================================
	// MODULE B — BUFFER ANALYSIS (METRIC UTM BUFFER + DISSOLVE + CRS)
	// ========================================================================
	t.Run("ModuleB_BufferAnalysis", func(t *testing.T) {
		// 1. Undissolved 1.5 km buffer around hospitals
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/buffer", aliceToken, sessionID, map[string]interface{}{
			"map_id":         mapID,
			"input_layer_id": hospitalsLayerID,
			"distance":       1.5,
			"unit":           "kilometers",
			"segments":       16,
			"cap_style":      "round",
			"join_style":     "round",
			"dissolve":       false,
			"output_name":    "Hospitals 1.5km Buffer",
			"output_crs":     "EPSG:4326",
		})
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("buffer analysis failed (%d): %s", resp.StatusCode, string(body))
		}
		var jobResp struct {
			Data *models.ProcessingJob `json:"data"`
		}
		_ = json.Unmarshal(body, &jobResp)
		if jobResp.Data.Status != models.JobStatusCompleted {
			t.Fatalf("expected buffer job completed, got %s (%s)", jobResp.Data.Status, jobResp.Data.ErrorMessage)
		}
		outLayerID, _ := jobResp.Data.OutputConfig["output_layer_id"].(string)
		if outLayerID == "" {
			t.Fatalf("expected output_layer_id in job output_config")
		}

		// Verify 3 polygon features were generated
		fResp, fBody := doGISRequest(t, client, "GET", server.URL+"/api/layers/"+outLayerID+"/features", aliceToken, sessionID, nil)
		if fResp.StatusCode != http.StatusOK {
			t.Fatalf("get buffered features failed: %s", string(fBody))
		}
		var fc struct {
			Data struct {
				Features []interface{} `json:"features"`
			} `json:"data"`
		}
		_ = json.Unmarshal(fBody, &fc)
		if len(fc.Data.Features) != 3 {
			t.Fatalf("expected 3 buffered polygons, got %d", len(fc.Data.Features))
		}

		// 2. Dissolved buffer in EPSG:3857
		resp2, body2 := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/buffer", aliceToken, sessionID, map[string]interface{}{
			"map_id":         mapID,
			"input_layer_id": hospitalsLayerID,
			"distance":       2500,
			"unit":           "meters",
			"dissolve":       true,
			"output_name":    "Dissolved Hospital Buffer 3857",
			"output_crs":     "EPSG:3857",
		})
		if resp2.StatusCode != http.StatusAccepted {
			t.Fatalf("dissolved buffer failed: %s", string(body2))
		}

		// Check job logs & result endpoints
		logResp, logBody := doGISRequest(t, client, "GET", server.URL+"/api/v1/jobs/"+jobResp.Data.JobID+"/logs", aliceToken, sessionID, nil)
		if logResp.StatusCode != http.StatusOK {
			t.Fatalf("get job logs failed: %s", string(logBody))
		}
		resResp, resBody := doGISRequest(t, client, "GET", server.URL+"/api/v1/jobs/"+jobResp.Data.JobID+"/result", aliceToken, sessionID, nil)
		if resResp.StatusCode != http.StatusOK {
			t.Fatalf("get job result failed: %s", string(resBody))
		}
	})

	// ========================================================================
	// MODULE C — OVERLAY ANALYSIS (INTERSECTION, UNION, DIFFERENCE, CLIP, DISSOLVE, SYMMETRICAL DIFFERENCE)
	// ========================================================================
	t.Run("ModuleC_OverlayAnalysis", func(t *testing.T) {
		ops := []string{"intersection", "union", "difference", "clip", "symmetrical-difference"}
		for _, op := range ops {
			resp, body := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/"+op, aliceToken, sessionID, map[string]interface{}{
				"map_id":              mapID,
				"input_layer_a":       wardsLayerID,
				"input_layer_b":       floodLayerID,
				"preserve_attributes": "all",
				"output_name":         "Wards " + op + " Flood",
			})
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("overlay %s failed (%d): %s", op, resp.StatusCode, string(body))
			}
			var jobResp struct {
				Data *models.ProcessingJob `json:"data"`
			}
			_ = json.Unmarshal(body, &jobResp)
			if jobResp.Data.Status != models.JobStatusCompleted {
				t.Fatalf("overlay %s did not complete: %s", op, jobResp.Data.ErrorMessage)
			}
		}

		// Test Dissolve by attribute field ("district")
		dResp, dBody := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/dissolve", aliceToken, sessionID, map[string]interface{}{
			"map_id":          mapID,
			"input_layer_id":  hospitalsLayerID,
			"dissolve_fields": []string{"district"},
			"dissolve_all":    false,
			"output_name":     "Hospitals Dissolved by District",
		})
		if dResp.StatusCode != http.StatusAccepted {
			t.Fatalf("dissolve failed: %s", string(dBody))
		}
		var dJob struct {
			Data *models.ProcessingJob `json:"data"`
		}
		_ = json.Unmarshal(dBody, &dJob)
		if cnt, _ := dJob.Data.OutputConfig["feature_count"].(float64); int(cnt) != 2 {
			t.Fatalf("expected 2 dissolved district groups (Central, South), got %v", dJob.Data.OutputConfig["feature_count"])
		}
	})

	// ========================================================================
	// MODULE D — SPATIAL JOIN (COUNT & SUM AGGREGATIONS)
	// ========================================================================
	t.Run("ModuleD_SpatialJoin", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/spatial-join", aliceToken, sessionID, map[string]interface{}{
			"map_id":               mapID,
			"target_layer_id":      wardsLayerID,
			"source_layer_id":      hospitalsLayerID,
			"spatial_relationship": "contains",
			"aggregation_type":     "sum",
			"selected_fields":      []string{"beds"},
			"output_name":          "Wards with Hospital Beds",
		})
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("spatial join failed: %s", string(body))
		}
		var jobResp struct {
			Data *models.ProcessingJob `json:"data"`
		}
		_ = json.Unmarshal(body, &jobResp)
		outLayerID, _ := jobResp.Data.OutputConfig["output_layer_id"].(string)

		// Inspect joined attributes on the output layer
		fResp, fBody := doGISRequest(t, client, "GET", server.URL+"/api/layers/"+outLayerID+"/features", aliceToken, sessionID, nil)
		if fResp.StatusCode != http.StatusOK {
			t.Fatalf("get spatial join features failed: %s", string(fBody))
		}
		var fc struct {
			Data struct {
				Features []struct {
					Properties map[string]interface{} `json:"properties"`
				} `json:"features"`
			} `json:"data"`
		}
		_ = json.Unmarshal(fBody, &fc)
		if len(fc.Data.Features) != 2 {
			t.Fatalf("expected 2 target wards in spatial join output, got %d", len(fc.Data.Features))
		}
		// Central ward has 2 hospitals (450 + 320 = 770 beds)
		foundCentral := false
		for _, f := range fc.Data.Features {
			if f.Properties["ward_name"] == "Connaught Ward" {
				foundCentral = true
				if sumBeds, _ := f.Properties["sum_beds"].(float64); int(sumBeds) != 770 {
					t.Fatalf("expected Central Ward sum_beds=770, got %v", f.Properties["sum_beds"])
				}
			}
		}
		if !foundCentral {
			t.Fatalf("Connaught Ward not found in spatial join output")
		}
	})

	// ========================================================================
	// MODULE E — PROXIMITY ANALYSIS (NEAREST, DISTANCE MATRIX, NEAREST FACILITY)
	// ========================================================================
	t.Run("ModuleE_ProximityAnalysis", func(t *testing.T) {
		for _, endpoint := range []string{"proximity", "distance-matrix", "nearest-facility"} {
			resp, body := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/"+endpoint, aliceToken, sessionID, map[string]interface{}{
				"map_id":                    mapID,
				"source_layer_id":           wardsLayerID,
				"target_layer_id":           hospitalsLayerID,
				"maximum_distance":          25,
				"unit":                      "kilometers",
				"nearest_count":             2,
				"generate_connection_lines": true,
				"output_name":               "Proximity " + endpoint,
			})
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("proximity %s failed: %s", endpoint, string(body))
			}
			var jobResp struct {
				Data *models.ProcessingJob `json:"data"`
			}
			_ = json.Unmarshal(body, &jobResp)
			if jobResp.Data.Status != models.JobStatusCompleted {
				t.Fatalf("proximity %s failed: %s", endpoint, jobResp.Data.ErrorMessage)
			}
		}
	})

	// ========================================================================
	// MODULE F — HEATMAP GENERATION (KDE RASTER)
	// ========================================================================
	var generatedRasterDatasetID string
	t.Run("ModuleF_HeatmapGeneration", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/heatmap", aliceToken, sessionID, map[string]interface{}{
			"map_id":         mapID,
			"input_layer_id": hospitalsLayerID,
			"weight_field":   "beds",
			"radius":         1500,
			"radius_unit":    "meters",
			"kernel_type":    "quartic",
			"color_ramp":     "inferno",
			"output_name":    "Hospital Beds KDE Heatmap",
		})
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("heatmap KDE failed: %s", string(body))
		}
		var jobResp struct {
			Data *models.ProcessingJob `json:"data"`
		}
		_ = json.Unmarshal(body, &jobResp)
		if jobResp.Data.Status != models.JobStatusCompleted {
			t.Fatalf("heatmap job did not complete: %s", jobResp.Data.ErrorMessage)
		}
		generatedRasterDatasetID, _ = jobResp.Data.OutputConfig["output_dataset_id"].(string)
		if generatedRasterDatasetID == "" {
			t.Fatalf("expected output_dataset_id from heatmap generation")
		}
		if len(jobResp.Data.Outputs) > 0 && jobResp.Data.Outputs[0].StoragePath != "" {
			if _, statErr := os.Stat(jobResp.Data.Outputs[0].StoragePath); statErr != nil {
				t.Fatalf("expected generated heatmap PNG file to exist on disk: %v", statErr)
			}
		}
	})

	// ========================================================================
	// MODULE G — RASTER PROCESSING TOOLBOX (CLIP, REPROJECT, RESAMPLE, CALCULATOR, ALIGN, MOSAIC, CONTOURS, HILLSHADE)
	// ========================================================================
	t.Run("ModuleG_RasterProcessingToolbox", func(t *testing.T) {
		// 1. Clip Raster
		r1, b1 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/clip", aliceToken, sessionID, map[string]interface{}{
			"map_id":            mapID,
			"raster_dataset_id": generatedRasterDatasetID,
			"clipping_layer_id": wardsLayerID,
			"output_name":       "Clipped Heatmap",
		})
		if r1.StatusCode != http.StatusAccepted {
			t.Fatalf("raster clip failed: %s", string(b1))
		}

		// 2. Reproject Raster
		r2, b2 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/reproject", aliceToken, sessionID, map[string]interface{}{
			"map_id":            mapID,
			"raster_dataset_id": generatedRasterDatasetID,
			"target_crs":        "EPSG:3857",
			"output_name":       "Heatmap 3857",
		})
		if r2.StatusCode != http.StatusAccepted {
			t.Fatalf("raster reproject failed: %s", string(b2))
		}

		// 3. Resample Raster
		r3, b3 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/resample", aliceToken, sessionID, map[string]interface{}{
			"map_id":            mapID,
			"raster_dataset_id": generatedRasterDatasetID,
			"scale_factor":      0.5,
			"method":            "bilinear",
		})
		if r3.StatusCode != http.StatusAccepted {
			t.Fatalf("raster resample failed: %s", string(b3))
		}

		// 4. Raster Calculator (valid expression + unsafe expression rejection)
		r4, b4 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/calculator", aliceToken, sessionID, map[string]interface{}{
			"map_id":        mapID,
			"input_rasters": []string{generatedRasterDatasetID},
			"expression":    "(A * 1.5) + 10",
			"output_name":   "Scaled Intensity",
		})
		if r4.StatusCode != http.StatusAccepted {
			t.Fatalf("raster calculator failed: %s", string(b4))
		}
		badCalcResp, _ := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/calculator", aliceToken, sessionID, map[string]interface{}{
			"map_id":        mapID,
			"input_rasters": []string{generatedRasterDatasetID},
			"expression":    "__import__('os').system('calc')",
		})
		if badCalcResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for unsafe raster calculator expression, got %d", badCalcResp.StatusCode)
		}

		// 5. Align Rasters
		r5, b5 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/align", aliceToken, sessionID, map[string]interface{}{
			"map_id":               mapID,
			"raster_dataset_id":    generatedRasterDatasetID,
			"reference_dataset_id": generatedRasterDatasetID,
		})
		if r5.StatusCode != http.StatusAccepted {
			t.Fatalf("raster align failed: %s", string(b5))
		}

		// 6. Mosaic Rasters
		r6, b6 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/mosaic", aliceToken, sessionID, map[string]interface{}{
			"map_id":             mapID,
			"raster_dataset_ids": []string{generatedRasterDatasetID, generatedRasterDatasetID},
			"output_name":        "Seamless Mosaic",
		})
		if r6.StatusCode != http.StatusAccepted {
			t.Fatalf("raster mosaic failed: %s", string(b6))
		}

		// 7. Contours Generation
		r7, b7 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/contours", aliceToken, sessionID, map[string]interface{}{
			"map_id":            mapID,
			"raster_dataset_id": generatedRasterDatasetID,
			"interval":          40.0,
		})
		if r7.StatusCode != http.StatusAccepted {
			t.Fatalf("raster contours failed: %s", string(b7))
		}

		// 8. Hillshade Generation
		r8, b8 := doGISRequest(t, client, "POST", server.URL+"/api/v1/raster/hillshade", aliceToken, sessionID, map[string]interface{}{
			"map_id":            mapID,
			"raster_dataset_id": generatedRasterDatasetID,
			"azimuth":           315.0,
			"altitude":          45.0,
			"z_factor":          1.2,
		})
		if r8.StatusCode != http.StatusAccepted {
			t.Fatalf("raster hillshade failed: %s", string(b8))
		}
	})

	// ========================================================================
	// MODULE H — TOPOLOGY VALIDATION, ERROR LAYER, FIXES & PERMISSIONS
	// ========================================================================
	t.Run("ModuleH_TopologyValidationAndPermissions", func(t *testing.T) {
		// Add an overlapping polygon and a duplicate polygon to wardsLayerID to trigger topology errors
		doGISRequest(t, client, "POST", server.URL+"/api/layers/"+wardsLayerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type": "Polygon",
				"coordinates": [][][]float64{
					{{77.205, 28.610}, {77.240, 28.610}, {77.240, 28.650}, {77.205, 28.650}, {77.205, 28.610}},
				},
			},
			"properties": map[string]interface{}{"ward_name": "Overlapping Ward"},
		})

		topResp, topBody := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/topology", aliceToken, sessionID, map[string]interface{}{
			"map_id":                   mapID,
			"layer_id":                 wardsLayerID,
			"check_overlaps":           true,
			"check_gaps":               true,
			"check_self_intersections": true,
			"check_duplicates":         true,
			"output_error_layer":       true,
		})
		if topResp.StatusCode != http.StatusAccepted {
			t.Fatalf("topology validation failed: %s", string(topBody))
		}
		var topJob struct {
			Data *models.ProcessingJob `json:"data"`
		}
		_ = json.Unmarshal(topBody, &topJob)
		jobID := topJob.Data.JobID

		// Retrieve topology errors
		errResp, errBody := doGISRequest(t, client, "GET", server.URL+"/api/v1/analysis/topology/"+jobID+"/errors", aliceToken, sessionID, nil)
		if errResp.StatusCode != http.StatusOK {
			t.Fatalf("get topology errors failed: %s", string(errBody))
		}
		var errList struct {
			Data struct {
				Errors []*models.TopologyError `json:"errors"`
				Total  int                     `json:"total"`
			} `json:"data"`
		}
		_ = json.Unmarshal(errBody, &errList)
		if errList.Data.Total == 0 {
			t.Fatalf("expected at least 1 topology error for overlapping polygon")
		}

		// Fix topology errors
		fixResp, fixBody := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/topology/"+jobID+"/fix", aliceToken, sessionID, map[string]interface{}{
			"fix_all": true,
		})
		if fixResp.StatusCode != http.StatusOK {
			t.Fatalf("fix topology errors failed: %s", string(fixBody))
		}

		// Unauthorized Bob cannot execute processing on Alice's map
		bobResp, _ := doGISRequest(t, client, "POST", server.URL+"/api/v1/analysis/buffer", bobToken, sessionID, map[string]interface{}{
			"map_id":         mapID,
			"input_layer_id": hospitalsLayerID,
			"distance":       500,
			"unit":           "meters",
		})
		if bobResp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unauthorized Bob, got %d", bobResp.StatusCode)
		}

		// Core Basic QGIS Operations: Centroid Generation, Calculate Area & Length, Raster Statistics
		for _, coreAlg := range []struct {
			alg    string
			params map[string]interface{}
		}{
			{"centroid", map[string]interface{}{"input_layer_id": wardsLayerID, "output_name": "Ward Centroids"}},
			{"calculate_geometry", map[string]interface{}{"input_layer_id": wardsLayerID, "output_name": "Wards Area & Length"}},
			{"raster_statistics", map[string]interface{}{"raster_dataset_id": generatedRasterDatasetID, "output_name": "Heatmap Stats"}},
		} {
			cResp, cBody := doGISRequest(t, client, "POST", server.URL+"/api/v1/processing/execute", aliceToken, sessionID, map[string]interface{}{
				"map_id":         mapID,
				"algorithm_name": coreAlg.alg,
				"parameters":     coreAlg.params,
			})
			if cResp.StatusCode != http.StatusAccepted {
				t.Fatalf("core algorithm %s failed: %s", coreAlg.alg, string(cBody))
			}
		}
	})
}
