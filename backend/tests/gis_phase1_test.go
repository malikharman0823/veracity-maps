package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

func doGISRequest(t *testing.T, client *http.Client, method, url, token, sessionID string, body interface{}) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("failed to create request %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if sessionID != "" {
		req.Header.Set("X-Session-ID", sessionID)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, url, err)
	}
	respBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("failed reading response body from %s %s: %v", method, url, err)
	}
	return resp, respBytes
}

func TestGISPhase1_AllModulesEndToEnd(t *testing.T) {
	server, _, _, _ := setupPersonalMapTestServer(t)
	client := &http.Client{Timeout: 10 * time.Second}

	// Register Owner (Alice) and Viewer/Outsider (Bob)
	aliceToken, _ := registerAndLoginUser(t, server.URL, "alice-gis@example.com", "Alice GIS")
	bobToken, _ := registerAndLoginUser(t, server.URL, "bob-gis@example.com", "Bob Viewer")

	// Start a session for dataset operations
	sessResp, sessBytes := doGISRequest(t, client, "POST", server.URL+"/api/sessions", aliceToken, "", nil)
	if sessResp.StatusCode != http.StatusCreated {
		t.Fatalf("create session failed: %d %s", sessResp.StatusCode, string(sessBytes))
	}
	var sessData struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(sessBytes, &sessData)
	sessionID := sessData.Data.SessionID

	// Create a Map owned by Alice
	mapResp, mapBytes := doGISRequest(t, client, "POST", server.URL+"/api/maps", aliceToken, sessionID, map[string]string{
		"name":        "Phase 1 GIS Master Map",
		"description": "QGIS-inspired foundational GIS workspace",
	})
	if mapResp.StatusCode != http.StatusCreated {
		t.Fatalf("create map failed: %d %s", mapResp.StatusCode, string(mapBytes))
	}
	var createdMap struct {
		Data *models.Map `json:"data"`
	}
	_ = json.Unmarshal(mapBytes, &createdMap)
	mapID := createdMap.Data.ID

	// ========================================================================
	// MODULE A — ADVANCED LAYER MANAGER
	// ========================================================================
	t.Run("ModuleA_LayerManager", func(t *testing.T) {
		// 1. Create parent group and nested subgroup
		grpResp, grpBytes := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layer-groups", aliceToken, sessionID, map[string]interface{}{
			"group_name": "Infrastructure",
		})
		if grpResp.StatusCode != http.StatusCreated {
			t.Fatalf("create group failed: %d %s", grpResp.StatusCode, string(grpBytes))
		}
		var grp1 struct {
			Data *models.LayerGroup `json:"data"`
		}
		_ = json.Unmarshal(grpBytes, &grp1)

		subGrpResp, subGrpBytes := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layer-groups", aliceToken, sessionID, map[string]interface{}{
			"group_name":      "Utilities Subgroup",
			"parent_group_id": grp1.Data.GroupID,
		})
		if subGrpResp.StatusCode != http.StatusCreated {
			t.Fatalf("create subgroup failed: %d %s", subGrpResp.StatusCode, string(subGrpBytes))
		}
		var grp2 struct {
			Data *models.LayerGroup `json:"data"`
		}
		_ = json.Unmarshal(subGrpBytes, &grp2)
		if grp2.Data.ParentGroupID != grp1.Data.GroupID {
			t.Fatalf("expected subgroup parent_group_id %s, got %s", grp1.Data.GroupID, grp2.Data.ParentGroupID)
		}

		// 2. Create vector, raster, and CAD layers
		l1Resp, l1Bytes := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"layer_name":      "Parcels Vector",
			"layer_type":      "vector",
			"geometry_type":   "Polygon",
			"parent_group_id": grp1.Data.GroupID,
			"opacity":         0.9,
			"min_zoom":        2,
			"max_zoom":        20,
		})
		if l1Resp.StatusCode != http.StatusCreated {
			t.Fatalf("create layer 1 failed: %d %s", l1Resp.StatusCode, string(l1Bytes))
		}
		var layer1 struct {
			Data *models.MapLayer `json:"data"`
		}
		_ = json.Unmarshal(l1Bytes, &layer1)

		l2Resp, l2Bytes := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"layer_name":      "Drone Ortho Raster",
			"layer_type":      "raster",
			"parent_group_id": grp2.Data.GroupID,
			"opacity":         1.0,
		})
		if l2Resp.StatusCode != http.StatusCreated {
			t.Fatalf("create layer 2 failed: %d %s", l2Resp.StatusCode, string(l2Bytes))
		}
		var layer2 struct {
			Data *models.MapLayer `json:"data"`
		}
		_ = json.Unmarshal(l2Bytes, &layer2)

		// 3. Reorder layers (put layer2 before layer1)
		roResp, roBytes := doGISRequest(t, client, "PUT", server.URL+"/api/maps/"+mapID+"/layers/order", aliceToken, sessionID, map[string]interface{}{
			"layer_ids": []string{layer2.Data.LayerID, layer1.Data.LayerID},
		})
		if roResp.StatusCode != http.StatusOK {
			t.Fatalf("reorder layers failed: %d %s", roResp.StatusCode, string(roBytes))
		}

		// 4. Toggle visibility & opacity
		visResp, _ := doGISRequest(t, client, "PATCH", server.URL+"/api/layers/"+layer1.Data.LayerID+"/visibility", aliceToken, sessionID, map[string]interface{}{
			"is_visible": false,
		})
		if visResp.StatusCode != http.StatusOK {
			t.Fatalf("visibility update failed: %d", visResp.StatusCode)
		}
		opResp, _ := doGISRequest(t, client, "PATCH", server.URL+"/api/layers/"+layer1.Data.LayerID+"/opacity", aliceToken, sessionID, map[string]interface{}{
			"opacity": 0.65,
		})
		if opResp.StatusCode != http.StatusOK {
			t.Fatalf("opacity update failed: %d", opResp.StatusCode)
		}

		// 5. Lock layer and verify locked protection
		lockResp, _ := doGISRequest(t, client, "PATCH", server.URL+"/api/layers/"+layer2.Data.LayerID, aliceToken, sessionID, map[string]interface{}{
			"is_locked": true,
		})
		if lockResp.StatusCode != http.StatusOK {
			t.Fatalf("lock layer failed: %d", lockResp.StatusCode)
		}
		blockedResp, _ := doGISRequest(t, client, "PATCH", server.URL+"/api/layers/"+layer2.Data.LayerID+"/opacity", aliceToken, sessionID, map[string]interface{}{
			"opacity": 0.4,
		})
		if blockedResp.StatusCode != http.StatusLocked {
			t.Fatalf("expected 423 Locked when modifying locked layer, got %d", blockedResp.StatusCode)
		}
		// Unlock layer2
		doGISRequest(t, client, "PATCH", server.URL+"/api/layers/"+layer2.Data.LayerID, aliceToken, sessionID, map[string]interface{}{
			"is_locked": false,
		})

		// 6. Duplicate layer & delete duplicate
		dupResp, dupBytes := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layer1.Data.LayerID+"/duplicate", aliceToken, sessionID, nil)
		if dupResp.StatusCode != http.StatusCreated {
			t.Fatalf("duplicate layer failed: %d %s", dupResp.StatusCode, string(dupBytes))
		}
		var dupLayer struct {
			Data *models.MapLayer `json:"data"`
		}
		_ = json.Unmarshal(dupBytes, &dupLayer)
		delResp, _ := doGISRequest(t, client, "DELETE", server.URL+"/api/layers/"+dupLayer.Data.LayerID, aliceToken, sessionID, nil)
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("delete duplicated layer failed: %d", delResp.StatusCode)
		}
	})

	// Create a dedicated Vector Layer for Modules B, C, D, E, G
	vecLayerResp, vecLayerBytes := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layers", aliceToken, sessionID, map[string]interface{}{
		"layer_name":    "Survey Parcels",
		"layer_type":    "vector",
		"geometry_type": "Polygon",
		"crs":           "EPSG:4326",
	})
	if vecLayerResp.StatusCode != http.StatusCreated {
		t.Fatalf("failed creating Survey Parcels layer: %d %s", vecLayerResp.StatusCode, string(vecLayerBytes))
	}
	var surveyLayer struct {
		Data *models.MapLayer `json:"data"`
	}
	_ = json.Unmarshal(vecLayerBytes, &surveyLayer)
	layerID := surveyLayer.Data.LayerID

	// ========================================================================
	// MODULE B — VECTOR EDITING
	// ========================================================================
	var polyFeatureID string
	t.Run("ModuleB_VectorEditing", func(t *testing.T) {
		// 1. Validate invalid geometry (unclosed ring & self-intersection)
		invResp, invBytes := doGISRequest(t, client, "POST", server.URL+"/api/features/validate", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type":        "Polygon",
				"coordinates": [][][]float64{{{77.20, 28.60}, {77.22, 28.60}, {77.22, 28.62}}}, // Not closed
			},
			"crs": "EPSG:4326",
		})
		if invResp.StatusCode != http.StatusOK {
			t.Fatalf("validate geometry endpoint failed: %d %s", invResp.StatusCode, string(invBytes))
		}
		var valOut struct {
			Data models.GeometryValidationResult `json:"data"`
		}
		_ = json.Unmarshal(invBytes, &valOut)
		if valOut.Data.IsValid {
			t.Fatalf("expected unclosed polygon to be marked invalid")
		}
		if valOut.Data.Repaired == nil {
			t.Fatalf("expected repaired closed polygon suggestion")
		}

		// Verify creating an invalid geometry feature is rejected with 400
		badFeatResp, _ := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type":        "LineString",
				"coordinates": [][]float64{{77.20, 28.60}}, // Only 1 vertex
			},
		})
		if badFeatResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for 1-vertex LineString, got %d", badFeatResp.StatusCode)
		}

		// 2. Create valid Point, LineString, and Polygon features
		ptResp, ptBytes := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{77.2090, 28.6139},
			},
			"properties": map[string]interface{}{
				"name":   "Benchmark A",
				"zone":   "commercial",
				"value":  120.5,
				"active": true,
			},
		})
		if ptResp.StatusCode != http.StatusCreated {
			t.Fatalf("create point failed: %d %s", ptResp.StatusCode, string(ptBytes))
		}

		lineResp, lineBytes := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type":        "LineString",
				"coordinates": [][]float64{{77.200, 28.600}, {77.210, 28.610}},
			},
			"properties": map[string]interface{}{
				"name":  "Access Road",
				"zone":  "transit",
				"value": 80.0,
			},
		})
		if lineResp.StatusCode != http.StatusCreated {
			t.Fatalf("create line failed: %d %s", lineResp.StatusCode, string(lineBytes))
		}

		polyResp, polyBytes := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/features", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type": "Polygon",
				"coordinates": [][][]float64{
					{{77.20, 28.60}, {77.22, 28.60}, {77.22, 28.62}, {77.20, 28.62}, {77.20, 28.60}},
				},
			},
			"properties": map[string]interface{}{
				"name":  "Plot 101",
				"zone":  "residential",
				"value": 250.0,
			},
		})
		if polyResp.StatusCode != http.StatusCreated {
			t.Fatalf("create polygon failed: %d %s", polyResp.StatusCode, string(polyBytes))
		}
		var polyFeat struct {
			Data *models.VectorFeature `json:"data"`
		}
		_ = json.Unmarshal(polyBytes, &polyFeat)
		polyFeatureID = polyFeat.Data.FeatureID

		// 3. Edit vertices & properties of Polygon
		updResp, updBytes := doGISRequest(t, client, "PATCH", server.URL+"/api/features/"+polyFeatureID, aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type": "Polygon",
				"coordinates": [][][]float64{
					{{77.20, 28.60}, {77.23, 28.60}, {77.23, 28.62}, {77.20, 28.62}, {77.20, 28.60}},
				},
			},
			"properties": map[string]interface{}{
				"value": 300.0,
			},
		})
		if updResp.StatusCode != http.StatusOK {
			t.Fatalf("update feature failed: %d %s", updResp.StatusCode, string(updBytes))
		}

		// 4. Verify Feature History (Undo/Redo audit trail)
		histResp, histBytes := doGISRequest(t, client, "GET", server.URL+"/api/features/"+polyFeatureID+"/history", aliceToken, sessionID, nil)
		if histResp.StatusCode != http.StatusOK {
			t.Fatalf("get feature history failed: %d %s", histResp.StatusCode, string(histBytes))
		}
		var histData struct {
			Data struct {
				History []*models.FeatureHistoryRecord `json:"history"`
			} `json:"data"`
		}
		_ = json.Unmarshal(histBytes, &histData)
		if len(histData.Data.History) < 2 {
			t.Fatalf("expected at least 2 history records (create + update), got %d", len(histData.Data.History))
		}

		// 5. Snap vertex to existing geometry
		snapResp, snapBytes := doGISRequest(t, client, "POST", server.URL+"/api/features/snap", aliceToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{77.20001, 28.60001},
			},
			"target_layer_id": layerID,
			"tolerance":       0.001,
		})
		if snapResp.StatusCode != http.StatusOK {
			t.Fatalf("snap geometry failed: %d %s", snapResp.StatusCode, string(snapBytes))
		}
		var snapData struct {
			Data struct {
				Snapped  bool                   `json:"snapped"`
				Geometry models.GeoJSONGeometry `json:"geometry"`
			} `json:"data"`
		}
		_ = json.Unmarshal(snapBytes, &snapData)
		if !snapData.Data.Snapped {
			t.Fatalf("expected point within tolerance to snap to target vertex")
		}

		// 6. Split Polygon with a LineString and Merge the resulting parts back
		splitResp, splitBytes := doGISRequest(t, client, "POST", server.URL+"/api/features/"+polyFeatureID+"/split", aliceToken, sessionID, map[string]interface{}{
			"split_geometry": map[string]interface{}{
				"type":        "LineString",
				"coordinates": [][]float64{{77.215, 28.59}, {77.215, 28.63}},
			},
		})
		if splitResp.StatusCode != http.StatusOK {
			t.Fatalf("split polygon failed: %d %s", splitResp.StatusCode, string(splitBytes))
		}
		var splitData struct {
			Data struct {
				Features []*models.VectorFeature `json:"features"`
			} `json:"data"`
		}
		_ = json.Unmarshal(splitBytes, &splitData)
		if len(splitData.Data.Features) != 2 {
			t.Fatalf("expected split to produce 2 polygons, got %d", len(splitData.Data.Features))
		}

		// Merge the 2 split polygons
		mergeResp, mergeBytes := doGISRequest(t, client, "POST", server.URL+"/api/features/merge", aliceToken, sessionID, map[string]interface{}{
			"feature_ids": []string{splitData.Data.Features[0].FeatureID, splitData.Data.Features[1].FeatureID},
			"output_name": "Plot 101 Merged",
		})
		if mergeResp.StatusCode != http.StatusOK {
			t.Fatalf("merge features failed: %d %s", mergeResp.StatusCode, string(mergeBytes))
		}
		var mergedFeat struct {
			Data *models.VectorFeature `json:"data"`
		}
		_ = json.Unmarshal(mergeBytes, &mergedFeat)
		polyFeatureID = mergedFeat.Data.FeatureID
	})

	// ========================================================================
	// MODULE C — ATTRIBUTE TABLE
	// ========================================================================
	t.Run("ModuleC_AttributeTable", func(t *testing.T) {
		// 1. Add attribute field
		fldResp, fldBytes := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/fields", aliceToken, sessionID, map[string]interface{}{
			"field_name":    "tax_Assessed",
			"field_type":    "float",
			"default_value": "0",
		})
		if fldResp.StatusCode != http.StatusCreated {
			t.Fatalf("add field failed: %d %s", fldResp.StatusCode, string(fldBytes))
		}
		var addedFld struct {
			Data *models.LayerField `json:"data"`
		}
		_ = json.Unmarshal(fldBytes, &addedFld)

		// 2. Reject invalid / SQL injection field name
		badFldResp, _ := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/fields", aliceToken, sessionID, map[string]interface{}{
			"field_name": "bad; DROP TABLE features;--",
			"field_type": "string",
		})
		if badFldResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid field name, got %d", badFldResp.StatusCode)
		}

		// 3. Calculate field values using safe expression evaluator (value * 2)
		calcResp, calcBytes := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/attributes/calculate", aliceToken, sessionID, map[string]interface{}{
			"target_field": "tax_Assessed",
			"expression":   "value * 2",
		})
		if calcResp.StatusCode != http.StatusOK {
			t.Fatalf("calculate field failed: %d %s", calcResp.StatusCode, string(calcBytes))
		}

		// 4. Filter, sort, and paginate attribute table
		filterResp, filterBytes := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/attributes/filter", aliceToken, sessionID, map[string]interface{}{
			"filters": []map[string]interface{}{
				{"field": "value", "operator": ">=", "value": 100},
			},
			"sort_by":    "value",
			"sort_order": "desc",
			"page":       1,
			"page_size":  10,
		})
		if filterResp.StatusCode != http.StatusOK {
			t.Fatalf("filter attributes failed: %d %s", filterResp.StatusCode, string(filterBytes))
		}
		var tableData struct {
			Data struct {
				Total int                     `json:"total"`
				Rows  []*models.VectorFeature `json:"rows"`
			} `json:"data"`
		}
		_ = json.Unmarshal(filterBytes, &tableData)
		if tableData.Data.Total < 2 {
			t.Fatalf("expected at least 2 filtered rows with value >= 100, got %d", tableData.Data.Total)
		}

		// 5. Export attributes as CSV
		expResp, expBytes := doGISRequest(t, client, "GET", server.URL+"/api/layers/"+layerID+"/attributes/export?format=csv", aliceToken, sessionID, nil)
		if expResp.StatusCode != http.StatusOK || !strings.Contains(string(expBytes), "feature_id") {
			t.Fatalf("export CSV failed: %d %s", expResp.StatusCode, string(expBytes))
		}

		// 6. Delete attribute field
		delFldResp, _ := doGISRequest(t, client, "DELETE", server.URL+"/api/layers/"+layerID+"/fields/"+addedFld.Data.FieldID, aliceToken, sessionID, nil)
		if delFldResp.StatusCode != http.StatusOK {
			t.Fatalf("delete field failed: %d", delFldResp.StatusCode)
		}
	})

	// ========================================================================
	// MODULE D — SYMBOLOGY & STYLING ENGINE
	// ========================================================================
	t.Run("ModuleD_SymbologyEngine", func(t *testing.T) {
		// 1. Single symbol style
		singleResp, singleBytes := doGISRequest(t, client, "PUT", server.URL+"/api/layers/"+layerID+"/style", aliceToken, sessionID, map[string]interface{}{
			"style_type": "single",
			"style_config": map[string]interface{}{
				"fill_color":   "#10b981",
				"stroke_color": "#047857",
				"stroke_width": 2.5,
				"opacity":      0.8,
				"label_field":  "name",
			},
		})
		if singleResp.StatusCode != http.StatusOK {
			t.Fatalf("save single style failed: %d %s", singleResp.StatusCode, string(singleBytes))
		}

		// 2. Categorized style
		catResp, catBytes := doGISRequest(t, client, "PUT", server.URL+"/api/layers/"+layerID+"/style", aliceToken, sessionID, map[string]interface{}{
			"style_type": "categorized",
			"style_config": map[string]interface{}{
				"field": "zone",
				"categories": []map[string]interface{}{
					{"value": "commercial", "color": "#f59e0b"},
					{"value": "residential", "color": "#3b82f6"},
					{"value": "transit", "color": "#8b5cf6"},
				},
			},
		})
		if catResp.StatusCode != http.StatusOK {
			t.Fatalf("save categorized style failed: %d %s", catResp.StatusCode, string(catBytes))
		}

		// 3. Graduated style & persistence reload check
		gradResp, gradBytes := doGISRequest(t, client, "PUT", server.URL+"/api/layers/"+layerID+"/style", aliceToken, sessionID, map[string]interface{}{
			"style_type": "graduated",
			"style_config": map[string]interface{}{
				"field":      "value",
				"base_color": "#dbeafe",
				"breaks": []map[string]interface{}{
					{"stop": 100, "color": "#60a5fa"},
					{"stop": 200, "color": "#1d4ed8"},
				},
				"label_field": "name",
			},
		})
		if gradResp.StatusCode != http.StatusOK {
			t.Fatalf("save graduated style failed: %d %s", gradResp.StatusCode, string(gradBytes))
		}

		getStResp, getStBytes := doGISRequest(t, client, "GET", server.URL+"/api/layers/"+layerID+"/style", aliceToken, sessionID, nil)
		if getStResp.StatusCode != http.StatusOK {
			t.Fatalf("get layer style failed: %d %s", getStResp.StatusCode, string(getStBytes))
		}
		var loadedStyle struct {
			Data *models.LayerStyle `json:"data"`
		}
		_ = json.Unmarshal(getStBytes, &loadedStyle)
		if loadedStyle.Data.StyleType != "graduated" || loadedStyle.Data.MapLibreStyle == nil {
			t.Fatalf("expected persisted graduated style with compiled maplibre_style, got %+v", loadedStyle.Data)
		}
	})

	// ========================================================================
	// MODULE E — CRS MANAGEMENT
	// ========================================================================
	t.Run("ModuleE_CRSManagement", func(t *testing.T) {
		// 1. Search EPSG codes
		crsResp, crsBytes := doGISRequest(t, client, "GET", server.URL+"/api/crs?q=3857", aliceToken, sessionID, nil)
		if crsResp.StatusCode != http.StatusOK {
			t.Fatalf("list CRS failed: %d %s", crsResp.StatusCode, string(crsBytes))
		}
		infoResp, infoBytes := doGISRequest(t, client, "GET", server.URL+"/api/crs/EPSG:32643", aliceToken, sessionID, nil)
		if infoResp.StatusCode != http.StatusOK || !strings.Contains(string(infoBytes), "32643") {
			t.Fatalf("get CRS info failed: %d %s", infoResp.StatusCode, string(infoBytes))
		}

		// 2. Reproject vector layer from EPSG:4326 to EPSG:3857 (preserves original layer)
		reprojResp, reprojBytes := doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+layerID+"/reproject", aliceToken, sessionID, map[string]interface{}{
			"layer_id":    layerID,
			"source_epsg": "EPSG:4326",
			"target_epsg": "EPSG:3857",
			"output_name": "Survey Parcels Web Mercator",
		})
		if reprojResp.StatusCode != http.StatusCreated {
			t.Fatalf("reproject vector layer failed: %d %s", reprojResp.StatusCode, string(reprojBytes))
		}
		var reprojOut struct {
			Data struct {
				TargetEPSG string           `json:"target_epsg"`
				Layer      *models.MapLayer `json:"layer"`
			} `json:"data"`
		}
		_ = json.Unmarshal(reprojBytes, &reprojOut)
		if reprojOut.Data.TargetEPSG != "EPSG:3857" || reprojOut.Data.Layer == nil {
			t.Fatalf("unexpected reprojected layer output: %+v", reprojOut.Data)
		}
	})

	// ========================================================================
	// MODULE F — DATASET BROWSER & DEPENDENCY CHECKS
	// ========================================================================
	t.Run("ModuleF_DatasetBrowserAndDependencyChecks", func(t *testing.T) {
		// 1. Upload a test PNG dataset
		pngBytes := createTestPNGBytes(120, 90)
		var buf bytes.Buffer
		// Use multipart upload
		boundary := "----VeracityTestBoundary999"
		fmt.Fprintf(&buf, "--%s\r\n", boundary)
		buf.WriteString(`Content-Disposition: form-data; name="file"; filename="site_plan.png"` + "\r\n")
		buf.WriteString("Content-Type: image/png\r\n\r\n")
		buf.Write(pngBytes)
		fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)

		upReq, _ := http.NewRequest("POST", server.URL+"/api/upload", &buf)
		upReq.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
		upReq.Header.Set("Authorization", "Bearer "+aliceToken)
		upReq.Header.Set("X-Session-ID", sessionID)
		upResp, err := client.Do(upReq)
		if err != nil || (upResp.StatusCode != http.StatusOK && upResp.StatusCode != http.StatusCreated) {
			t.Fatalf("upload dataset failed: %v, status: %d", err, upResp.StatusCode)
		}
		var upData struct {
			Data *models.Dataset `json:"data"`
		}
		_ = json.NewDecoder(upResp.Body).Decode(&upData)
		upResp.Body.Close()
		dsID := upData.Data.ID

		// 2. Inspect dataset & get metadata and internal layers
		inspResp, inspBytes := doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+dsID+"/inspect", aliceToken, sessionID, nil)
		if inspResp.StatusCode != http.StatusOK {
			t.Fatalf("inspect dataset failed: %d %s", inspResp.StatusCode, string(inspBytes))
		}
		metaResp, metaBytes := doGISRequest(t, client, "GET", server.URL+"/api/datasets/"+dsID+"/metadata", aliceToken, sessionID, nil)
		if metaResp.StatusCode != http.StatusOK {
			t.Fatalf("get dataset metadata failed: %d %s", metaResp.StatusCode, string(metaBytes))
		}
		dsLayersResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/datasets/"+dsID+"/dataset-layers", aliceToken, sessionID, nil)
		if dsLayersResp.StatusCode != http.StatusOK {
			t.Fatalf("get dataset layers failed: %d", dsLayersResp.StatusCode)
		}

		// 3. Rename dataset
		renResp, renBytes := doGISRequest(t, client, "PATCH", server.URL+"/api/datasets/"+dsID, aliceToken, sessionID, map[string]string{
			"name": "site_plan_renamed.png",
		})
		if renResp.StatusCode != http.StatusOK || !strings.Contains(string(renBytes), "site_plan_renamed.png") {
			t.Fatalf("rename dataset failed: %d %s", renResp.StatusCode, string(renBytes))
		}

		// 4. Set missing source CRS and reproject raster dataset
		setCrsResp, _ := doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+dsID+"/crs", aliceToken, sessionID, map[string]string{
			"crs": "EPSG:4326",
		})
		if setCrsResp.StatusCode != http.StatusOK {
			t.Fatalf("set dataset CRS failed: %d", setCrsResp.StatusCode)
		}
		reprojDsResp, reprojDsBytes := doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+dsID+"/reproject", aliceToken, sessionID, map[string]string{
			"source_epsg": "EPSG:4326",
			"target_epsg": "EPSG:3857",
			"output_name": "site_plan_3857.png",
		})
		if reprojDsResp.StatusCode != http.StatusCreated {
			t.Fatalf("reproject raster dataset failed: %d %s", reprojDsResp.StatusCode, string(reprojDsBytes))
		}

		// 5. Attach dataset to a map layer and verify dependency check blocks deletion
		refLayerResp, refLayerBytes := doGISRequest(t, client, "POST", server.URL+"/api/maps/"+mapID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"dataset_id": dsID,
			"layer_name": "Referenced Site Plan",
			"layer_type": "raster",
		})
		if refLayerResp.StatusCode != http.StatusCreated {
			t.Fatalf("create referencing layer failed: %d %s", refLayerResp.StatusCode, string(refLayerBytes))
		}
		var refLayer struct {
			Data *models.MapLayer `json:"data"`
		}
		_ = json.Unmarshal(refLayerBytes, &refLayer)

		blockedDelResp, _ := doGISRequest(t, client, "DELETE", server.URL+"/api/datasets/"+dsID, aliceToken, sessionID, nil)
		if blockedDelResp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 Conflict when deleting dataset referenced by active map layer, got %d", blockedDelResp.StatusCode)
		}

		// Remove the referencing layer, then verify dataset deletion succeeds
		doGISRequest(t, client, "DELETE", server.URL+"/api/layers/"+refLayer.Data.LayerID, aliceToken, sessionID, nil)
		okDelResp, _ := doGISRequest(t, client, "DELETE", server.URL+"/api/datasets/"+dsID, aliceToken, sessionID, nil)
		if okDelResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK when deleting unreferenced dataset, got %d", okDelResp.StatusCode)
		}
	})

	// ========================================================================
	// MODULE G — SPATIAL QUERY BUILDER & SECURITY
	// ========================================================================
	t.Run("ModuleG_SpatialQueryBuilderAndPermissions", func(t *testing.T) {
		// 1. Execute combined Attribute + Spatial Intersects query
		execResp, execBytes := doGISRequest(t, client, "POST", server.URL+"/api/spatial-queries/execute", aliceToken, sessionID, map[string]interface{}{
			"map_id":   mapID,
			"layer_id": layerID,
			"conditions": []map[string]interface{}{
				{"field": "value", "operator": ">=", "value": 100},
			},
			"spatial_operator": "intersects",
			"geometry": map[string]interface{}{
				"type": "Polygon",
				"coordinates": [][][]float64{
					{{77.19, 28.59}, {77.25, 28.59}, {77.25, 28.65}, {77.19, 28.65}, {77.19, 28.59}},
				},
			},
		})
		if execResp.StatusCode != http.StatusOK {
			t.Fatalf("execute spatial query failed: %d %s", execResp.StatusCode, string(execBytes))
		}
		var sqExec struct {
			Data struct {
				Total      int      `json:"total"`
				FeatureIDs []string `json:"feature_ids"`
			} `json:"data"`
		}
		_ = json.Unmarshal(execBytes, &sqExec)
		if sqExec.Data.Total < 2 {
			t.Fatalf("expected at least 2 matching features for spatial intersects + value>=100, got %d", sqExec.Data.Total)
		}

		// 2. Execute Distance query (within_distance 500 meters of point [77.2090, 28.6139])
		distResp, distBytes := doGISRequest(t, client, "POST", server.URL+"/api/spatial-queries/execute", aliceToken, sessionID, map[string]interface{}{
			"layer_id":         layerID,
			"spatial_operator": "within_distance",
			"distance":         500,
			"unit":             "meters",
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{77.2090, 28.6139},
			},
		})
		if distResp.StatusCode != http.StatusOK {
			t.Fatalf("distance query failed: %d %s", distResp.StatusCode, string(distBytes))
		}

		// 3. Save, list, and delete reusable spatial query
		saveQResp, saveQBytes := doGISRequest(t, client, "POST", server.URL+"/api/spatial-queries", aliceToken, sessionID, map[string]interface{}{
			"map_id":     mapID,
			"layer_id":   layerID,
			"query_name": "High Value Parcels",
			"conditions": []map[string]interface{}{
				{"field": "value", "operator": ">", "value": 200},
			},
		})
		if saveQResp.StatusCode != http.StatusCreated {
			t.Fatalf("save spatial query failed: %d %s", saveQResp.StatusCode, string(saveQBytes))
		}
		var savedQ struct {
			Data *models.SpatialQuery `json:"data"`
		}
		_ = json.Unmarshal(saveQBytes, &savedQ)

		listQResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/spatial-queries?map_id="+mapID, aliceToken, sessionID, nil)
		if listQResp.StatusCode != http.StatusOK {
			t.Fatalf("list spatial queries failed: %d", listQResp.StatusCode)
		}

		delQResp, _ := doGISRequest(t, client, "DELETE", server.URL+"/api/spatial-queries/"+savedQ.Data.QueryID, aliceToken, sessionID, nil)
		if delQResp.StatusCode != http.StatusOK {
			t.Fatalf("delete spatial query failed: %d", delQResp.StatusCode)
		}

		// 4. Security: Reject invalid field name in spatial query
		badFieldQResp, _ := doGISRequest(t, client, "POST", server.URL+"/api/spatial-queries/execute", aliceToken, sessionID, map[string]interface{}{
			"layer_id": layerID,
			"conditions": []map[string]interface{}{
				{"field": "nonexistent_field", "operator": "=", "value": "x"},
			},
		})
		if badFieldQResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-existent schema field, got %d", badFieldQResp.StatusCode)
		}

		// 5. Security: Unauthorized user (Bob) cannot access or modify Alice's private map layer
		unauthResp, _ := doGISRequest(t, client, "POST", server.URL+"/api/layers/"+layerID+"/features", bobToken, sessionID, map[string]interface{}{
			"geometry": map[string]interface{}{
				"type":        "Point",
				"coordinates": []float64{77.21, 28.61},
			},
		})
		if unauthResp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden when unauthorized user edits Alice's layer, got %d", unauthResp.StatusCode)
		}
	})
}
