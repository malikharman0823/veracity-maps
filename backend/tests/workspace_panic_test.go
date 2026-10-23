package tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
)

func TestWorkspaceWithMissingDatasetMemberRegression(t *testing.T) {
	server, repo, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Create session
	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	// 2. Upload primary dataset
	pngBytes := createTestPNG(150, 150)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "primary_floor.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", server.URL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)
	upResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to upload: %v", err)
	}
	var upResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(upResp.Body).Decode(&upResult)
	upResp.Body.Close()

	if upResult.Data.ID == "" {
		t.Fatalf("expected dataset id from upload")
	}

	datasetID := upResult.Data.ID

	// 3. Add a building that references both the primary dataset and a missing/deleted dataset
	_, _ = repo.CreateBuilding(sessionID, &models.Building{
		ID:        "bld_test",
		SessionID: sessionID,
		Name:      "Test Building",
		Floors: []*models.BuildingFloor{
			{
				ID:         "flr_1",
				BuildingID: "bld_test",
				Level:      0,
				Name:       "Ground Floor",
				DatasetIDs: []string{datasetID, "ds_nonexistent_ghost_floor"},
			},
		},
	})

	// 4. Request workspace for primary dataset - should return 200 without panicking
	wsReq, _ := http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/workspace", nil)
	wsReq.Header.Set("X-Session-ID", sessionID)
	wsResp, err := client.Do(wsReq)
	if err != nil {
		t.Fatalf("failed to get workspace: %v", err)
	}
	defer wsResp.Body.Close()

	if wsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from workspace, got %d", wsResp.StatusCode)
	}
}
