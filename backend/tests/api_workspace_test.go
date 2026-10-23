package tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/malikharman0823/veracity-maps/repository"
)

func TestAPI_WorkspaceAndIcons(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Test GET /icons
	iconResp, err := client.Get(server.URL + "/icons")
	if err != nil {
		t.Fatalf("failed to GET /icons: %v", err)
	}
	defer iconResp.Body.Close()
	if iconResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /icons, got %d", iconResp.StatusCode)
	}
	var iconData struct {
		Icons []string `json:"icons"`
	}
	_ = json.NewDecoder(iconResp.Body).Decode(&iconData)
	if len(iconData.Icons) == 0 {
		t.Fatalf("expected icons list, got empty")
	}

	// 2. Create session
	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 3. Upload a dataset
	pngBytes := createTestPNG(200, 100)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "floorplan.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	upResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to upload dataset: %v", err)
	}
	var uploadData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(upResp.Body).Decode(&uploadData)
	upResp.Body.Close()
	datasetID := uploadData.Data.ID

	// 4. Test GET /datasets/{id}/workspace
	wsReq, _ := http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/workspace", nil)
	wsReq.Header.Set("X-Session-ID", sessionID)
	wsResp, err := client.Do(wsReq)
	if err != nil {
		t.Fatalf("failed to GET workspace: %v", err)
	}
	defer wsResp.Body.Close()
	if wsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /workspace, got %d", wsResp.StatusCode)
	}
	var wsData struct {
		Data struct {
			ID      string                   `json:"id"`
			Name    string                   `json:"name"`
			Members []map[string]interface{} `json:"members"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wsResp.Body).Decode(&wsData)
	if len(wsData.Data.Members) == 0 {
		t.Fatalf("expected at least 1 member in workspace manifest, got 0")
	}
	sublayers, ok := wsData.Data.Members[0]["sublayers"].([]interface{})
	if !ok || len(sublayers) == 0 {
		t.Fatalf("expected sublayers array in member, got %v", wsData.Data.Members[0]["sublayers"])
	}

	// 5. Test GET /datasets/{id}/workspace/records
	recReq, _ := http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/workspace/records?page=1&page_size=25", nil)
	recReq.Header.Set("X-Session-ID", sessionID)
	recResp, err := client.Do(recReq)
	if err != nil {
		t.Fatalf("failed to GET workspace records: %v", err)
	}
	defer recResp.Body.Close()
	if recResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /workspace/records, got %d", recResp.StatusCode)
	}
	var recData struct {
		Data struct {
			Records []map[string]interface{} `json:"records"`
			Total   int                      `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(recResp.Body).Decode(&recData)
	if recData.Data.Total == 0 {
		t.Fatalf("expected at least 1 record (the raster), got 0")
	}

	// 6. Test PATCH /datasets/{id}/workspace/state
	statePatch, _ := json.Marshal(map[string]interface{}{"active_view": "map"})
	stateReq, _ := http.NewRequest("PATCH", server.URL+"/datasets/"+datasetID+"/workspace/state", bytes.NewReader(statePatch))
	stateReq.Header.Set("Content-Type", "application/json")
	stateReq.Header.Set("X-Session-ID", sessionID)
	stateResp, err := client.Do(stateReq)
	if err != nil {
		t.Fatalf("failed to PATCH workspace state: %v", err)
	}
	stateResp.Body.Close()
	if stateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from PATCH workspace state, got %d", stateResp.StatusCode)
	}

	// 7. Test PATCH /datasets/{id}/workspace/members/{id}
	memberPatch, _ := json.Marshal(map[string]interface{}{"raster_opacity": 0.75})
	memReq, _ := http.NewRequest("PATCH", server.URL+"/datasets/"+datasetID+"/workspace/members/"+datasetID, bytes.NewReader(memberPatch))
	memReq.Header.Set("Content-Type", "application/json")
	memReq.Header.Set("X-Session-ID", sessionID)
	memResp, err := client.Do(memReq)
	if err != nil {
		t.Fatalf("failed to PATCH workspace member: %v", err)
	}
	memResp.Body.Close()
	if memResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from PATCH workspace member, got %d", memResp.StatusCode)
	}

	// 8. Test Session Hydration from disk
	newRepo := repository.NewSessionRepository()
	newRepo.SetStorageDir(tempDir + "/sessions")
	reloadedSess, err := newRepo.GetSession(sessionID)
	if err != nil || reloadedSess == nil {
		t.Fatalf("expected session to hydrate from disk, got err: %v", err)
	}
	reloadedDs, err := newRepo.GetDataset(sessionID, datasetID)
	if err != nil || reloadedDs == nil {
		t.Fatalf("expected dataset to hydrate from disk, got err: %v", err)
	}
	if reloadedDs.Filename != "floorplan.png" {
		t.Fatalf("expected reloaded dataset filename 'floorplan.png', got %s", reloadedDs.Filename)
	}

	// 9. Test Uploading second standalone dataset creates independent workspace
	body2 := &bytes.Buffer{}
	writer2 := multipart.NewWriter(body2)
	part2, _ := writer2.CreateFormFile("file", "standalone_parking.png")
	_, _ = part2.Write(createTestPNG(150, 150))
	_ = writer2.Close()

	upReq2, _ := http.NewRequest("POST", server.URL+"/upload", body2)
	upReq2.Header.Set("Content-Type", writer2.FormDataContentType())
	upReq2.Header.Set("X-Session-ID", sessionID)
	upResp2, err := client.Do(upReq2)
	if err != nil {
		t.Fatalf("failed uploading 2nd dataset: %v", err)
	}
	var up2Data struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(upResp2.Body).Decode(&up2Data)
	upResp2.Body.Close()
	dsID2 := up2Data.Data.ID

	// Check 2nd dataset workspace manifest: must only contain dsID2, NOT datasetID (clean isolated workspace!)
	wsReq2, _ := http.NewRequest("GET", server.URL+"/datasets/"+dsID2+"/workspace", nil)
	wsReq2.Header.Set("X-Session-ID", sessionID)
	wsResp2, err := client.Do(wsReq2)
	if err != nil {
		t.Fatalf("failed getting workspace for ds2: %v", err)
	}
	defer wsResp2.Body.Close()
	var wsDoc2 struct {
		Data struct {
			Members []map[string]interface{} `json:"members"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wsResp2.Body).Decode(&wsDoc2)
	if len(wsDoc2.Data.Members) != 1 {
		t.Fatalf("expected 2nd workspace to be isolated with 1 member, got %d: %+v", len(wsDoc2.Data.Members), wsDoc2.Data.Members)
	}
	if wsDoc2.Data.Members[0]["dataset_id"] != dsID2 {
		t.Fatalf("expected 2nd workspace member to be %s, got %v", dsID2, wsDoc2.Data.Members[0]["dataset_id"])
	}

	// Also check original datasetID workspace: must NOT contain dsID2 automatically
	wsReq1, _ := http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/workspace", nil)
	wsReq1.Header.Set("X-Session-ID", sessionID)
	wsResp1, err := client.Do(wsReq1)
	if err != nil {
		t.Fatalf("failed getting workspace for ds1: %v", err)
	}
	defer wsResp1.Body.Close()
	var wsDoc1 struct {
		Data struct {
			Members []map[string]interface{} `json:"members"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wsResp1.Body).Decode(&wsDoc1)
	for _, m := range wsDoc1.Data.Members {
		if m["dataset_id"] == dsID2 {
			t.Fatalf("original workspace should not automatically contain newly uploaded dsID2 without explicit add dataset")
		}
	}

	// 10. Test Saved Configurations API: POST, GET list, DELETE
	cfgPayload, _ := json.Marshal(map[string]interface{}{
		"name":               "My Saved Campus Map",
		"primary_dataset_id": datasetID,
		"members":            []string{datasetID},
	})
	cfgReq, _ := http.NewRequest("POST", server.URL+"/sessions/"+sessionID+"/configs", bytes.NewReader(cfgPayload))
	cfgReq.Header.Set("Content-Type", "application/json")
	cfgResp, err := client.Do(cfgReq)
	if err != nil {
		t.Fatalf("failed saving config: %v", err)
	}
	defer cfgResp.Body.Close()
	if cfgResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from saving config, got %d", cfgResp.StatusCode)
	}
	var cfgRes struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	_ = json.NewDecoder(cfgResp.Body).Decode(&cfgRes)
	createdCfg := cfgRes.Data
	if createdCfg.ID == "" || createdCfg.Name != "My Saved Campus Map" {
		t.Fatalf("unexpected saved config: %+v", createdCfg)
	}

	// List saved configs
	listCfgReq, _ := http.NewRequest("GET", server.URL+"/sessions/"+sessionID+"/configs", nil)
	listCfgResp, err := client.Do(listCfgReq)
	if err != nil {
		t.Fatalf("failed listing saved configs: %v", err)
	}
	defer listCfgResp.Body.Close()
	var listRes struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	_ = json.NewDecoder(listCfgResp.Body).Decode(&listRes)
	if len(listRes.Data) != 1 {
		t.Fatalf("expected 1 saved config, got %d", len(listRes.Data))
	}

	// Delete saved config
	delCfgReq, _ := http.NewRequest("DELETE", server.URL+"/sessions/"+sessionID+"/configs/"+createdCfg.ID, nil)
	delCfgResp, err := client.Do(delCfgReq)
	if err != nil {
		t.Fatalf("failed deleting saved config: %v", err)
	}
	delCfgResp.Body.Close()
	if delCfgResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from deleting saved config, got %d", delCfgResp.StatusCode)
	}
}
