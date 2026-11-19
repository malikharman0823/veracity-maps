package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

func uploadTestPNGDataset(t *testing.T, client *http.Client, baseURL, token, sessionID, filename string) string {
	t.Helper()
	pngBytes := createTestPNGBytes(200, 150)
	var buf bytes.Buffer
	boundary := "----VeracityBaseLayerBoundary777"
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", filename)
	buf.WriteString("Content-Type: image/png\r\n\r\n")
	buf.Write(pngBytes)
	fmt.Fprintf(&buf, "\r\n--%s--\r\n", boundary)

	req, _ := http.NewRequest("POST", baseURL+"/api/upload", &buf)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Session-ID", sessionID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected upload status: %d", resp.StatusCode)
	}
	var parsed struct {
		Data *models.Dataset `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || parsed.Data == nil {
		t.Fatalf("failed to parse upload response: %v", err)
	}
	return parsed.Data.ID
}

func TestBaseLayersAndMapViews_EndToEnd(t *testing.T) {
	server, _, _, _ := setupPersonalMapTestServer(t)
	client := &http.Client{Timeout: 15 * time.Second}

	aliceToken, _ := registerAndLoginUser(t, server.URL, "alice-baselayer@example.com", "Alice BaseLayer")
	bobToken, _ := registerAndLoginUser(t, server.URL, "bob-baselayer@example.com", "Bob BaseLayer")

	// Create session and workspace map for Alice
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
		"name":        "Delhi Urban Survey Workspace",
		"description": "Workspace for Delhi base layer and map view testing",
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
	workspaceID := mapData.Data.ID

	// 1. Built-in Map Views API (GET /api/map-views and GET /api/map-views/:type)
	t.Run("MapViewsCatalog", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "GET", server.URL+"/api/map-views", aliceToken, sessionID, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/map-views failed (%d): %s", resp.StatusCode, string(body))
		}
		var parsed struct {
			Data struct {
				MapViews []models.MapViewDefinition `json:"map_views"`
				Count    int                        `json:"count"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &parsed)
		if parsed.Data.Count != 7 {
			t.Fatalf("expected 7 configured map views, got %d", parsed.Data.Count)
		}

		// Verify satellite supports labels, roads, places
		satResp, satBody := doGISRequest(t, client, "GET", server.URL+"/api/map-views/satellite", aliceToken, sessionID, nil)
		if satResp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/map-views/satellite failed: %s", string(satBody))
		}
		var satParsed struct {
			Data struct {
				MapView models.MapViewDefinition `json:"map_view"`
			} `json:"data"`
		}
		_ = json.Unmarshal(satBody, &satParsed)
		if !satParsed.Data.MapView.Capabilities.SupportsLabels || !satParsed.Data.MapView.Capabilities.SupportsRoads {
			t.Fatalf("expected satellite view to expose labels and roads capabilities")
		}

		// Invalid map view returns 400
		badResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/map-views/nonexistent", aliceToken, sessionID, nil)
		if badResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid map view type, got %d", badResp.StatusCode)
		}
	})

	// 2. Unreferenced raster MUST be rejected (never silently assign EPSG:4326)
	rawDsID := uploadTestPNGDataset(t, client, server.URL, aliceToken, sessionID, "raw_unreferenced_plan.png")
	t.Run("RejectUnreferencedRasterBaseLayer", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/base-layers", aliceToken, sessionID, map[string]interface{}{
			"dataset_id":  rawDsID,
			"name":        "Unreferenced Plan",
			"description": "Should fail because it has no CRS or georeferencing",
		})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 for unreferenced raster base layer, got %d: %s", resp.StatusCode, string(body))
		}
	})

	// 3. Upload & Georeference a Delhi map dataset, then register as Custom Base Layer
	delhiDsID := uploadTestPNGDataset(t, client, server.URL, aliceToken, sessionID, "delhi_city_map.png")
	{
		// Add 4 control points covering New Delhi (77.10..77.30, 28.50..28.70)
		cps := []map[string]interface{}{
			{"image_x": 0.0, "image_y": 0.0, "longitude": 77.10, "latitude": 28.70, "label": "NW"},
			{"image_x": 199.0, "image_y": 0.0, "longitude": 77.30, "latitude": 28.70, "label": "NE"},
			{"image_x": 199.0, "image_y": 149.0, "longitude": 77.30, "latitude": 28.50, "label": "SE"},
			{"image_x": 0.0, "image_y": 149.0, "longitude": 77.10, "latitude": 28.50, "label": "SW"},
		}
		for _, cp := range cps {
			resp, body := doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+delhiDsID+"/control-points", aliceToken, sessionID, cp)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				t.Fatalf("add control point failed (%d): %s", resp.StatusCode, string(body))
			}
		}
		geoResp, geoBody := doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+delhiDsID+"/georeference", aliceToken, sessionID, map[string]interface{}{
			"action": "save",
		})
		if geoResp.StatusCode != http.StatusOK {
			t.Fatalf("georeference save failed (%d): %s", geoResp.StatusCode, string(geoBody))
		}
	}

	var delhiBaseLayerID string
	t.Run("RegisterInspectValidateAndUpdateCustomBaseLayer", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/base-layers", aliceToken, sessionID, map[string]interface{}{
			"dataset_id":  delhiDsID,
			"name":        "Delhi Base Map",
			"description": "Georeferenced Delhi municipal reference map",
			"source_type": "raster",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("register base layer failed (%d): %s", resp.StatusCode, string(body))
		}
		var parsed struct {
			Data struct {
				BaseLayer *models.BaseLayer `json:"base_layer"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &parsed)
		bl := parsed.Data.BaseLayer
		if bl == nil || bl.ID == "" {
			t.Fatalf("expected valid base layer ID")
		}
		delhiBaseLayerID = bl.ID
		if bl.Status != models.BaseLayerStatusReady || len(bl.Extent) != 4 {
			t.Fatalf("expected ready status with 4-element extent, got status=%s extent=%v", bl.Status, bl.Extent)
		}
		if bl.Extent[0] < 77.0 || bl.Extent[2] > 77.4 || bl.Extent[1] < 28.4 || bl.Extent[3] > 28.8 {
			t.Fatalf("unexpected Delhi extent: %v", bl.Extent)
		}

		// List base layers includes Delhi Base Map and default_base_layer
		listResp, listBody := doGISRequest(t, client, "GET", server.URL+"/api/base-layers", aliceToken, sessionID, nil)
		if listResp.StatusCode != http.StatusOK {
			t.Fatalf("list base layers failed: %s", string(listBody))
		}
		var listParsed struct {
			Data struct {
				DefaultBaseLayer *models.BaseLayer   `json:"default_base_layer"`
				BaseLayers       []*models.BaseLayer `json:"base_layers"`
			} `json:"data"`
		}
		_ = json.Unmarshal(listBody, &listParsed)
		if listParsed.Data.DefaultBaseLayer == nil || !listParsed.Data.DefaultBaseLayer.IsDefault {
			t.Fatalf("expected default World Map in list response")
		}
		if len(listParsed.Data.BaseLayers) != 1 || listParsed.Data.BaseLayers[0].ID != delhiBaseLayerID {
			t.Fatalf("expected 1 custom base layer, got %d", len(listParsed.Data.BaseLayers))
		}

		// Preview endpoint
		prevResp, prevBody := doGISRequest(t, client, "GET", server.URL+"/api/base-layers/"+delhiBaseLayerID+"/preview", aliceToken, sessionID, nil)
		if prevResp.StatusCode != http.StatusOK {
			t.Fatalf("preview failed: %s", string(prevBody))
		}

		// Validate endpoint
		valResp, valBody := doGISRequest(t, client, "POST", server.URL+"/api/base-layers/"+delhiBaseLayerID+"/validate", aliceToken, sessionID, nil)
		if valResp.StatusCode != http.StatusOK {
			t.Fatalf("validate failed: %s", string(valBody))
		}
		var valParsed struct {
			Data struct {
				Validation models.BaseLayerValidationResult `json:"validation"`
			} `json:"data"`
		}
		_ = json.Unmarshal(valBody, &valParsed)
		if !valParsed.Data.Validation.IsValid || !valParsed.Data.Validation.IsGeoreferenced {
			t.Fatalf("expected valid georeferenced base layer, got %+v", valParsed.Data.Validation)
		}

		// Update metadata (owner)
		updResp, updBody := doGISRequest(t, client, "PUT", server.URL+"/api/base-layers/"+delhiBaseLayerID, aliceToken, sessionID, map[string]interface{}{
			"name":        "Delhi NCT Base Map",
			"description": "Updated description",
		})
		if updResp.StatusCode != http.StatusOK {
			t.Fatalf("update base layer failed: %s", string(updBody))
		}

		// Bob cannot access or update Alice's custom base layer
		bobGet, _ := doGISRequest(t, client, "GET", server.URL+"/api/base-layers/"+delhiBaseLayerID, bobToken, "", nil)
		if bobGet.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 when Bob reads Alice's base layer, got %d", bobGet.StatusCode)
		}
	})

	// 4. Workspace Base Layer Selection & Persistence + Map View Preferences
	t.Run("WorkspaceBaseLayerAndMapViewPreferences", func(t *testing.T) {
		// Default before selection is World Map (has_saved_selection: false)
		defResp, defBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/base-layer", aliceToken, sessionID, nil)
		if defResp.StatusCode != http.StatusOK {
			t.Fatalf("get default workspace base layer failed: %s", string(defBody))
		}
		var defParsed struct {
			Data struct {
				WorkspaceBaseLayer models.WorkspaceBaseLayer `json:"workspace_base_layer"`
			} `json:"data"`
		}
		_ = json.Unmarshal(defBody, &defParsed)
		if defParsed.Data.WorkspaceBaseLayer.BaseLayerType != models.BaseLayerTypeWorldMap || defParsed.Data.WorkspaceBaseLayer.HasSavedSelection {
			t.Fatalf("expected unsaved default world_map initially, got %+v", defParsed.Data.WorkspaceBaseLayer)
		}

		// Select Delhi Base Map for workspace
		selResp, selBody := doGISRequest(t, client, "PUT", server.URL+"/api/workspaces/"+workspaceID+"/base-layer", aliceToken, sessionID, map[string]interface{}{
			"base_layer_type": "custom",
			"base_layer_id":   delhiBaseLayerID,
		})
		if selResp.StatusCode != http.StatusOK {
			t.Fatalf("select custom base layer failed (%d): %s", selResp.StatusCode, string(selBody))
		}

		// Re-fetch confirms persistence
		getResp, getBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/base-layer", aliceToken, sessionID, nil)
		if getResp.StatusCode != http.StatusOK {
			t.Fatalf("re-fetch workspace base layer failed: %s", string(getBody))
		}
		var getParsed struct {
			Data struct {
				WorkspaceBaseLayer models.WorkspaceBaseLayer `json:"workspace_base_layer"`
			} `json:"data"`
		}
		_ = json.Unmarshal(getBody, &getParsed)
		if getParsed.Data.WorkspaceBaseLayer.BaseLayerID != delhiBaseLayerID || !getParsed.Data.WorkspaceBaseLayer.HasSavedSelection {
			t.Fatalf("expected persisted Delhi base layer, got %+v", getParsed.Data.WorkspaceBaseLayer)
		}

		// Update Map View to Satellite with filters & opacity
		mvResp, mvBody := doGISRequest(t, client, "PUT", server.URL+"/api/workspaces/"+workspaceID+"/map-view", aliceToken, sessionID, map[string]interface{}{
			"view_type":   "satellite",
			"show_labels": false,
			"show_roads":  true,
			"show_places": false,
			"opacity":     0.75,
		})
		if mvResp.StatusCode != http.StatusOK {
			t.Fatalf("update map view failed (%d): %s", mvResp.StatusCode, string(mvBody))
		}

		// Reject arbitrary tile URL injection in style_configuration
		injResp, _ := doGISRequest(t, client, "PUT", server.URL+"/api/workspaces/"+workspaceID+"/map-view", aliceToken, sessionID, map[string]interface{}{
			"view_type": "streets",
			"style_configuration": map[string]interface{}{
				"tile_url": "https://evil.example.com/{z}/{x}/{y}.png",
			},
		})
		if injResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 when injecting tile_url into style_configuration, got %d", injResp.StatusCode)
		}

		// Reject out-of-range opacity
		badOp, _ := doGISRequest(t, client, "PUT", server.URL+"/api/workspaces/"+workspaceID+"/map-view", aliceToken, sessionID, map[string]interface{}{
			"view_type": "streets",
			"opacity":   1.5,
		})
		if badOp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for opacity > 1, got %d", badOp.StatusCode)
		}

		// Bob cannot change Alice's workspace base layer
		bobPut, _ := doGISRequest(t, client, "PUT", server.URL+"/api/workspaces/"+workspaceID+"/base-layer", bobToken, "", map[string]interface{}{
			"base_layer_type": "world_map",
		})
		if bobPut.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 when Bob updates Alice's workspace base layer, got %d", bobPut.StatusCode)
		}
	})

	// 5. Deleting base layer registration preserves underlying dataset AND triggers unavailable warning on workspace reopen
	t.Run("DeleteRegistrationPreservesDatasetAndWarnsWorkspace", func(t *testing.T) {
		delResp, delBody := doGISRequest(t, client, "DELETE", server.URL+"/api/base-layers/"+delhiBaseLayerID, aliceToken, sessionID, nil)
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("delete base layer failed (%d): %s", delResp.StatusCode, string(delBody))
		}

		// Underlying dataset must still exist!
		dsResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/datasets/"+delhiDsID, aliceToken, sessionID, nil)
		if dsResp.StatusCode != http.StatusOK {
			t.Fatalf("expected source dataset %s to remain intact after deleting base layer registration, got status %d", delhiDsID, dsResp.StatusCode)
		}

		// Reopening workspace reports previous_unavailable warning without silently erasing selection
		wsResp, wsBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/base-layer", aliceToken, sessionID, nil)
		if wsResp.StatusCode != http.StatusOK {
			t.Fatalf("get workspace base layer after deletion failed: %s", string(wsBody))
		}
		var wsParsed struct {
			Data struct {
				WorkspaceBaseLayer models.WorkspaceBaseLayer `json:"workspace_base_layer"`
			} `json:"data"`
		}
		_ = json.Unmarshal(wsBody, &wsParsed)
		if !wsParsed.Data.WorkspaceBaseLayer.PreviousUnavailable || wsParsed.Data.WorkspaceBaseLayer.UnavailableWarning == "" {
			t.Fatalf("expected previous_unavailable warning when saved base layer was deleted, got %+v", wsParsed.Data.WorkspaceBaseLayer)
		}
	})
}
