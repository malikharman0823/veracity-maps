package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
)

func TestAPI_BuildingsAndFloors(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Create session
	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed creating session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 2. Create Building
	bldBody, _ := json.Marshal(map[string]interface{}{
		"name":                "Science Complex",
		"description":         "Main research and lab wing",
		"default_floor_level": 0,
	})

	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/buildings", bytes.NewReader(bldBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Session-ID", sessionID)

	bldResp, err := client.Do(createReq)
	if err != nil {
		t.Fatalf("failed creating building: %v", err)
	}
	defer bldResp.Body.Close()
	if bldResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from /buildings, got %d", bldResp.StatusCode)
	}

	var bldDoc struct {
		Data *models.Building `json:"data"`
	}
	_ = json.NewDecoder(bldResp.Body).Decode(&bldDoc)
	if bldDoc.Data == nil || bldDoc.Data.ID == "" {
		t.Fatalf("expected created building with ID")
	}
	buildingID := bldDoc.Data.ID

	// 3. Add Floors to building (Basement 1, Ground, First Floor)
	floorLevels := []struct {
		Level int
		Name  string
	}{
		{Level: -1, Name: "Basement 1"},
		{Level: 0, Name: "Ground Floor"},
		{Level: 1, Name: "First Floor"},
	}

	for _, fl := range floorLevels {
		flBody, _ := json.Marshal(map[string]interface{}{
			"level": fl.Level,
			"name":  fl.Name,
		})
		flReq, _ := http.NewRequest(http.MethodPost, server.URL+"/buildings/"+buildingID+"/floors", bytes.NewReader(flBody))
		flReq.Header.Set("Content-Type", "application/json")
		flReq.Header.Set("X-Session-ID", sessionID)
		flResp, err := client.Do(flReq)
		if err != nil {
			t.Fatalf("failed adding floor %d: %v", fl.Level, err)
		}
		flResp.Body.Close()
		if flResp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 for floor %d, got %d", fl.Level, flResp.StatusCode)
		}
	}

	// 4. Retrieve Building and verify floors
	getReq, _ := http.NewRequest(http.MethodGet, server.URL+"/buildings/"+buildingID, nil)
	getReq.Header.Set("X-Session-ID", sessionID)
	getResp, err := client.Do(getReq)
	if err != nil {
		t.Fatalf("failed getting building: %v", err)
	}
	defer getResp.Body.Close()
	var fetchedDoc struct {
		Data *models.Building `json:"data"`
	}
	_ = json.NewDecoder(getResp.Body).Decode(&fetchedDoc)
	if len(fetchedDoc.Data.Floors) != 3 {
		t.Fatalf("expected 3 floors on building, found %d", len(fetchedDoc.Data.Floors))
	}

	// 5. Update a floor
	floorID := fetchedDoc.Data.Floors[0].ID
	upBody, _ := json.Marshal(map[string]interface{}{
		"name": "Sub-Basement Utility",
	})
	upReq, _ := http.NewRequest(http.MethodPut, server.URL+"/buildings/"+buildingID+"/floors/"+floorID, bytes.NewReader(upBody))
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("X-Session-ID", sessionID)
	upResp, err := client.Do(upReq)
	if err != nil {
		t.Fatalf("failed updating floor: %v", err)
	}
	upResp.Body.Close()
	if upResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK updating floor, got %d", upResp.StatusCode)
	}

	// 6. Delete a floor
	delFlReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/buildings/"+buildingID+"/floors/"+floorID, nil)
	delFlReq.Header.Set("X-Session-ID", sessionID)
	delFlResp, err := client.Do(delFlReq)
	if err != nil {
		t.Fatalf("failed deleting floor: %v", err)
	}
	delFlResp.Body.Close()
	if delFlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK deleting floor, got %d", delFlResp.StatusCode)
	}

	// Verify floor count decreased to 2
	getReq2, _ := http.NewRequest(http.MethodGet, server.URL+"/buildings/"+buildingID, nil)
	getReq2.Header.Set("X-Session-ID", sessionID)
	getResp2, err := client.Do(getReq2)
	if err != nil {
		t.Fatalf("failed fetching building: %v", err)
	}
	defer getResp2.Body.Close()
	var fetchedDoc2 struct {
		Data *models.Building `json:"data"`
	}
	_ = json.NewDecoder(getResp2.Body).Decode(&fetchedDoc2)
	if len(fetchedDoc2.Data.Floors) != 2 {
		t.Fatalf("expected 2 floors after deletion, got %d", len(fetchedDoc2.Data.Floors))
	}
}
