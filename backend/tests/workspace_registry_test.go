package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

func generateArbitraryTestGeoPackage(t *testing.T) string {
	t.Helper()
	tmpPath := filepath.Join(t.TempDir(), "zurich_sample.gpkg")
	pyExe := filepath.Join("..", ".venv", "Scripts", "python.exe")
	if _, err := os.Stat(pyExe); err != nil {
		pyExe = "python"
	}
	script := fmt.Sprintf(`
import geopandas as gpd
from shapely.geometry import Point, LineString, Polygon

out = r"%s"
districts = gpd.GeoDataFrame(
    [{"district_code": "D1", "title": "Altstadt", "pop": 5800},
     {"district_code": "D2", "title": "Enge", "pop": 9400}],
    geometry=[
        Polygon([(8.53, 47.36), (8.55, 47.36), (8.55, 47.38), (8.53, 47.38), (8.53, 47.36)]),
        Polygon([(8.51, 47.35), (8.53, 47.35), (8.53, 47.37), (8.51, 47.37), (8.51, 47.35)]),
    ],
    crs="EPSG:4326",
)
districts.to_file(out, layer="city_districts", driver="GPKG")

lines = gpd.GeoDataFrame(
    [{"route_no": "T4", "operator": "VBZ"}],
    geometry=[LineString([(8.52, 47.36), (8.54, 47.37), (8.55, 47.38)])],
    crs="EPSG:4326",
)
lines.to_file(out, layer="transit_lines", driver="GPKG")

hubs = gpd.GeoDataFrame(
    [{"hub_name": "Limmatquai Charger", "ports": 8},
     {"hub_name": "Bahnhofplatz Hub", "ports": 12},
     {"hub_name": "Bellevue Node", "ports": 6}],
    geometry=[Point(8.542, 47.371), Point(8.540, 47.377), Point(8.545, 47.367)],
    crs="EPSG:4326",
)
hubs.to_file(out, layer="charging_hubs", driver="GPKG")
`, tmpPath)

	cmd := exec.Command(pyExe, "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed generating test GeoPackage: %v (%s)", err, string(out))
	}
	return tmpPath
}

func uploadBinaryFile(t *testing.T, client *http.Client, baseURL, token, sessionID, filePath, contentType string) string {
	t.Helper()
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	var buf bytes.Buffer
	boundary := "----VeracityGpkgBoundary555"
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	fmt.Fprintf(&buf, "Content-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\n", filepath.Base(filePath))
	fmt.Fprintf(&buf, "Content-Type: %s\r\n\r\n", contentType)
	buf.Write(raw)
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
		t.Fatalf("upload returned status %d", resp.StatusCode)
	}
	var parsed struct {
		Data *models.Dataset `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || parsed.Data == nil {
		t.Fatalf("decode upload response failed: %v", err)
	}
	return parsed.Data.ID
}

func TestWorkspaceRegistry_DynamicGeoPackageAndRecords(t *testing.T) {
	server, _, _, _ := setupPersonalMapTestServer(t)
	client := &http.Client{Timeout: 30 * time.Second}

	aliceToken, _ := registerAndLoginUser(t, server.URL, "alice-gpkg@example.com", "Alice GPKG")
	bobToken, _ := registerAndLoginUser(t, server.URL, "bob-gpkg@example.com", "Bob GPKG")

	// Session + Workspace
	_, sessBody := doGISRequest(t, client, "POST", server.URL+"/api/sessions", aliceToken, "", nil)
	var sessData struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(sessBody, &sessData)
	sessionID := sessData.Data.SessionID

	_, mapBody := doGISRequest(t, client, "POST", server.URL+"/api/maps", aliceToken, sessionID, map[string]interface{}{
		"name":        "Zurich Multimodal GIS Workspace",
		"description": "Arbitrary non-Delhi GeoPackage + Raster + Control Points + Icons test",
	})
	var mapData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(mapBody, &mapData)
	workspaceID := mapData.Data.ID

	// Upload arbitrary 3-layer GeoPackage
	gpkgPath := generateArbitraryTestGeoPackage(t)
	gpkgDsID := uploadBinaryFile(t, client, server.URL, aliceToken, sessionID, gpkgPath, "application/geopackage+sqlite3")

	// Also upload a raster PNG + add a control point + add a user icon
	rasterDsID := uploadTestPNGDataset(t, client, server.URL, aliceToken, sessionID, "zurich_aerial_plan.png")
	_, _ = doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+rasterDsID+"/control-points", aliceToken, sessionID, map[string]interface{}{
		"image_x": 10.0, "image_y": 20.0, "longitude": 8.54, "latitude": 47.37, "label": "Ref Corner A",
	})
	_, _ = doGISRequest(t, client, "POST", server.URL+"/api/datasets/"+rasterDsID+"/icon-points", aliceToken, sessionID, map[string]interface{}{
		"name": "Main Entrance", "icon_id": "cinema", "longitude": 8.541, "latitude": 47.372,
	})

	// 1. Dynamic Multi-Layer Inspection (POST /api/datasets/inspect)
	t.Run("DynamicGeoPackageInspection", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/datasets/inspect", aliceToken, sessionID, map[string]interface{}{
			"file_id":      gpkgDsID,
			"workspace_id": workspaceID,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("inspect dataset failed (%d): %s", resp.StatusCode, string(body))
		}
		var parsed struct {
			Data struct {
				IsMultiLayer bool                      `json:"is_multi_layer"`
				Layers       []models.DatasetLayerInfo `json:"layers"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &parsed)
		if !parsed.Data.IsMultiLayer || len(parsed.Data.Layers) != 3 {
			t.Fatalf("expected 3 dynamically discovered layers, got %d (multi=%v)", len(parsed.Data.Layers), parsed.Data.IsMultiLayer)
		}
	})

	// 2. Selective Import of 2 of 3 GeoPackage layers + Raster Layer
	var districtsLayerID, hubsLayerID string
	t.Run("SelectiveLayerImportAndIndependentManagement", func(t *testing.T) {
		resp, body := doGISRequest(t, client, "POST", server.URL+"/api/workspaces/"+workspaceID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"dataset_id":           gpkgDsID,
			"selected_layer_names": []string{"city_districts", "charging_hubs"},
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("selective layer import failed (%d): %s", resp.StatusCode, string(body))
		}
		var parsed struct {
			Data struct {
				Layers []*models.WorkspaceLayer `json:"layers"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &parsed)
		if len(parsed.Data.Layers) != 2 {
			t.Fatalf("expected 2 selectively imported layers, got %d", len(parsed.Data.Layers))
		}
		for _, l := range parsed.Data.Layers {
			if l.SourceLayerName == "city_districts" {
				districtsLayerID = l.ID
			} else if l.SourceLayerName == "charging_hubs" {
				hubsLayerID = l.ID
			}
		}
		if districtsLayerID == "" || hubsLayerID == "" {
			t.Fatalf("expected both city_districts and charging_hubs layer IDs")
		}

		// Import raster layer into same workspace
		rResp, rBody := doGISRequest(t, client, "POST", server.URL+"/api/workspaces/"+workspaceID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"dataset_id": rasterDsID,
		})
		if rResp.StatusCode != http.StatusCreated {
			t.Fatalf("raster layer import failed (%d): %s", rResp.StatusCode, string(rBody))
		}

		// Toggle visibility & rename charging_hubs independently
		newName := "EV Charging Hubs"
		visFalse := false
		updResp, updBody := doGISRequest(t, client, "PATCH", server.URL+"/api/workspaces/"+workspaceID+"/layers/"+hubsLayerID, aliceToken, sessionID, map[string]interface{}{
			"display_name": newName,
			"visibility":   visFalse,
		})
		if updResp.StatusCode != http.StatusOK {
			t.Fatalf("update workspace layer failed (%d): %s", updResp.StatusCode, string(updBody))
		}

		// Query features with pagination & attribute filter
		fResp, fBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/layers/"+hubsLayerID+"/features?attr_field=hub_name&attr_value=Bellevue&page=1&page_size=10", aliceToken, sessionID, nil)
		if fResp.StatusCode != http.StatusOK {
			t.Fatalf("query layer features failed (%d): %s", fResp.StatusCode, string(fBody))
		}
		var fParsed struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		_ = json.Unmarshal(fBody, &fParsed)
		if fParsed.Data.Total != 1 {
			t.Fatalf("expected 1 filtered charging hub matching 'Bellevue', got %d", fParsed.Data.Total)
		}
	})

	// 3. Centralized Workspace Records, Dynamic Columns, Edit & Export
	t.Run("WorkspaceRecordsCountsSchemaEditAndExport", func(t *testing.T) {
		cntResp, cntBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/records/counts", aliceToken, sessionID, nil)
		if cntResp.StatusCode != http.StatusOK {
			t.Fatalf("record counts failed (%d): %s", cntResp.StatusCode, string(cntBody))
		}
		var cntParsed struct {
			Data struct {
				Counts models.WorkspaceRecordCounts `json:"counts"`
			} `json:"data"`
		}
		_ = json.Unmarshal(cntBody, &cntParsed)
		// 2 districts + 3 hubs = 5 vectors, >=1 raster, >=1 control point, >=1 user icon
		if cntParsed.Data.Counts.Vectors != 5 || cntParsed.Data.Counts.Rasters < 1 || cntParsed.Data.Counts.ControlPoints < 1 || cntParsed.Data.Counts.UserIcons < 1 {
			t.Fatalf("unexpected record counts: %+v", cntParsed.Data.Counts)
		}

		// Dynamic schema must discover arbitrary GeoPackage fields like district_code, hub_name, ports
		schResp, schBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/records/schema?record_type=vector", aliceToken, sessionID, nil)
		if schResp.StatusCode != http.StatusOK {
			t.Fatalf("record schema failed: %s", string(schBody))
		}
		if !strings.Contains(string(schBody), `"attr:hub_name"`) || !strings.Contains(string(schBody), `"attr:district_code"`) {
			t.Fatalf("expected dynamic columns attr:hub_name and attr:district_code in schema: %s", string(schBody))
		}

		// Paginated query for vectors
		recResp, recBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/records?record_type=vector&search=Altstadt&page=1&page_size=10", aliceToken, sessionID, nil)
		if recResp.StatusCode != http.StatusOK {
			t.Fatalf("list records failed: %s", string(recBody))
		}
		var recParsed struct {
			Data struct {
				Total   int                       `json:"total"`
				Records []*models.WorkspaceRecord `json:"records"`
			} `json:"data"`
		}
		_ = json.Unmarshal(recBody, &recParsed)
		if recParsed.Data.Total != 1 || len(recParsed.Data.Records) != 1 {
			t.Fatalf("expected 1 Altstadt record, got total=%d", recParsed.Data.Total)
		}

		// Edit attribute inline
		recID := recParsed.Data.Records[0].ID
		edResp, edBody := doGISRequest(t, client, "PATCH", server.URL+"/api/workspaces/"+workspaceID+"/records/"+recID, aliceToken, sessionID, map[string]interface{}{
			"editable_attributes": map[string]interface{}{"pop": 6100},
		})
		if edResp.StatusCode != http.StatusOK {
			t.Fatalf("edit record failed (%d): %s", edResp.StatusCode, string(edBody))
		}

		// Export CSV
		expResp, expBody := doGISRequest(t, client, "POST", server.URL+"/api/workspaces/"+workspaceID+"/records/export", aliceToken, sessionID, map[string]interface{}{
			"format":      "csv",
			"record_type": "vector",
		})
		if expResp.StatusCode != http.StatusOK || !strings.Contains(string(expBody), "Altstadt") {
			t.Fatalf("export CSV failed (%d): %s", expResp.StatusCode, string(expBody))
		}

		// Bob cannot access Alice's workspace records
		bobResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/records", bobToken, "", nil)
		if bobResp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403 for unauthorized user, got %d", bobResp.StatusCode)
		}
	})

	// 4. Deleting one GeoPackage layer must NOT delete sibling layer or source dataset
	t.Run("DeleteSingleLayerPreservesSiblingAndSourceDataset", func(t *testing.T) {
		delResp, delBody := doGISRequest(t, client, "DELETE", server.URL+"/api/workspaces/"+workspaceID+"/layers/"+hubsLayerID, aliceToken, sessionID, nil)
		if delResp.StatusCode != http.StatusOK {
			t.Fatalf("delete layer failed (%d): %s", delResp.StatusCode, string(delBody))
		}

		// city_districts must still exist
		sibResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/layers/"+districtsLayerID, aliceToken, sessionID, nil)
		if sibResp.StatusCode != http.StatusOK {
			t.Fatalf("expected sibling GeoPackage layer to remain intact, got %d", sibResp.StatusCode)
		}

		// Original source dataset must still exist
		dsResp, _ := doGISRequest(t, client, "GET", server.URL+"/api/datasets/"+gpkgDsID, aliceToken, sessionID, nil)
		if dsResp.StatusCode != http.StatusOK {
			t.Fatalf("expected original GeoPackage dataset to remain intact, got %d", dsResp.StatusCode)
		}

		// Vector record count should now be 2 (only city_districts)
		cntResp, cntBody := doGISRequest(t, client, "GET", server.URL+"/api/workspaces/"+workspaceID+"/records/counts", aliceToken, sessionID, nil)
		if cntResp.StatusCode != http.StatusOK {
			t.Fatalf("counts after delete failed: %s", string(cntBody))
		}
		var cntParsed struct {
			Data struct {
				Counts models.WorkspaceRecordCounts `json:"counts"`
			} `json:"data"`
		}
		_ = json.Unmarshal(cntBody, &cntParsed)
		if cntParsed.Data.Counts.Vectors != 2 {
			t.Fatalf("expected 2 vector records after deleting charging_hubs, got %d", cntParsed.Data.Counts.Vectors)
		}
	})

	// 5. Complete GeoPackage Point Scan (3D PointZ + attribute table with latitude/longitude columns)
	t.Run("CompleteGpkgPointScan3DAndLatLonTable", func(t *testing.T) {
		ptGpkgPath := filepath.Join(t.TempDir(), "emergency_points_scan.gpkg")
		pyExe := filepath.Join("..", ".venv", "Scripts", "python.exe")
		if _, err := os.Stat(pyExe); err != nil {
			pyExe = "python"
		}
		script := fmt.Sprintf(`
import sqlite3
import geopandas as gpd
from shapely.geometry import Point

out = r"%s"
pts3d = gpd.GeoDataFrame(
    [{"name": "Central Hospital", "service_type": "Hospital"},
     {"name": "North Clinic", "service_type": "Hospital"}],
    geometry=[Point(77.21, 28.63, 215.5), Point(77.19, 28.65, 220.0)],
    crs="EPSG:4326",
)
pts3d.to_file(out, layer="hospitals_3d", driver="GPKG")

conn = sqlite3.connect(out)
cur = conn.cursor()
cur.execute("CREATE TABLE fire_stations_attr (id INTEGER PRIMARY KEY, name TEXT, latitude REAL, longitude REAL)")
cur.execute("INSERT INTO fire_stations_attr (name, latitude, longitude) VALUES ('Connaught Fire Station', 28.6315, 77.2167)")
cur.execute("INSERT INTO fire_stations_attr (name, latitude, longitude) VALUES ('Rohini Fire Station', 28.7041, 77.1025)")
cur.execute("INSERT INTO gpkg_contents (table_name, data_type, identifier, description, srs_id) VALUES ('fire_stations_attr', 'attributes', 'fire_stations_attr', 'Lat/Lon attribute table', 4326)")
conn.commit()
conn.close()
`, ptGpkgPath)
		cmd := exec.Command(pyExe, "-c", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed generating point scan GeoPackage: %v (%s)", err, string(out))
		}

		ptDsID := uploadBinaryFile(t, client, server.URL, aliceToken, sessionID, ptGpkgPath, "application/geopackage+sqlite3")
		impResp, impBody := doGISRequest(t, client, "POST", server.URL+"/api/workspaces/"+workspaceID+"/layers", aliceToken, sessionID, map[string]interface{}{
			"dataset_id": ptDsID,
		})
		if impResp.StatusCode != http.StatusCreated {
			t.Fatalf("import point scan gpkg failed (%d): %s", impResp.StatusCode, string(impBody))
		}
		var impParsed struct {
			Data struct {
				Layers []*models.WorkspaceLayer `json:"layers"`
			} `json:"data"`
		}
		_ = json.Unmarshal(impBody, &impParsed)
		if len(impParsed.Data.Layers) != 2 {
			t.Fatalf("expected 2 point layers discovered & imported, got %d", len(impParsed.Data.Layers))
		}
		for _, lyr := range impParsed.Data.Layers {
			if lyr.GeometryType != "Point" || lyr.GISLayerID == "" {
				t.Fatalf("expected layer %s to be imported as vector Point with GISLayerID, got geom=%s gisLayer=%s", lyr.SourceLayerName, lyr.GeometryType, lyr.GISLayerID)
			}
			fResp, fBody := doGISRequest(t, client, "GET", server.URL+"/api/layers/"+lyr.GISLayerID+"/features", aliceToken, sessionID, nil)
			if fResp.StatusCode != http.StatusOK {
				t.Fatalf("get maplibre features for %s failed (%d): %s", lyr.SourceLayerName, fResp.StatusCode, string(fBody))
			}
			var fParsed struct {
				Data struct {
					Total int `json:"total"`
				} `json:"data"`
			}
			_ = json.Unmarshal(fBody, &fParsed)
			if fParsed.Data.Total != 2 {
				t.Fatalf("expected 2 MapLibre point features in %s, got %d", lyr.SourceLayerName, fParsed.Data.Total)
			}
		}
	})
}
