package tests

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
)

func uploadTestImage(t *testing.T, client *http.Client, serverURL, sessionID, filename string, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	buf := &bytes.Buffer{}
	_ = png.Encode(buf, img)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write(buf.Bytes())
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, serverURL+"/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", sessionID)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	defer resp.Body.Close()

	var uploadResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResult)
	if uploadResult.Data.ID == "" {
		t.Fatalf("expected dataset ID from upload")
	}
	return uploadResult.Data.ID
}

func TestAPI_IconPointsCRUD(t *testing.T) {
	server, _, tempDir := createTestServer(t)
	defer server.Close()
	defer os.RemoveAll(tempDir)

	client := server.Client()

	// 1. Verify GET /icons returns rich catalog
	iconResp, err := client.Get(server.URL + "/icons")
	if err != nil {
		t.Fatalf("failed to GET /icons: %v", err)
	}
	defer iconResp.Body.Close()
	if iconResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from /icons, got %d", iconResp.StatusCode)
	}

	var iconDoc struct {
		Success    bool                     `json:"success"`
		Icons      []string                 `json:"icons"`
		Categories []string                 `json:"categories"`
		Catalog    []map[string]interface{} `json:"catalog"`
	}
	if err := json.NewDecoder(iconResp.Body).Decode(&iconDoc); err != nil {
		t.Fatalf("failed decoding /icons response: %v", err)
	}
	if len(iconDoc.Icons) == 0 {
		t.Fatalf("expected non-empty icon IDs list")
	}

	// 2. Test GET /icons/{id}
	testIconID := iconDoc.Icons[0]
	singleIconResp, err := client.Get(server.URL + "/icons/" + testIconID)
	if err != nil {
		t.Fatalf("failed to GET /icons/%s: %v", testIconID, err)
	}
	defer singleIconResp.Body.Close()
	if singleIconResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /icons/%s, got %d", testIconID, singleIconResp.StatusCode)
	}
	contentType := singleIconResp.Header.Get("Content-Type")
	if contentType == "" || !bytes.Contains([]byte(contentType), []byte("image")) {
		t.Fatalf("expected image Content-Type, got %s", contentType)
	}

	// 3. Create session and dataset
	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("failed creating session: %v", err)
	}
	sessionID := sessResp.Header.Get("X-Session-ID")
	sessResp.Body.Close()

	datasetID := uploadTestImage(t, client, server.URL, sessionID, "floorplan.png", 200, 200)

	// 4. Create IconPoint
	lng := -122.4194
	lat := 37.7749
	createBody, _ := json.Marshal(map[string]interface{}{
		"name":        "Security Desk",
		"description": "Main entrance security monitoring point",
		"icon_id":     testIconID,
		"longitude":   lng,
		"latitude":    lat,
		"scope":       "floor",
		"floor_level": 0,
		"floor_label": "Ground Floor",
	})

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/datasets/"+datasetID+"/icon-points", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", sessionID)

	ptResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed creating icon point: %v", err)
	}
	defer ptResp.Body.Close()
	if ptResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from icon-points, got %d", ptResp.StatusCode)
	}

	var createdDoc struct {
		Success bool              `json:"success"`
		Data    *models.IconPoint `json:"data"`
	}
	_ = json.NewDecoder(ptResp.Body).Decode(&createdDoc)
	if createdDoc.Data == nil || createdDoc.Data.ID == "" {
		t.Fatalf("expected created icon point with ID")
	}
	ptID := createdDoc.Data.ID
	if createdDoc.Data.Name != "Security Desk" || createdDoc.Data.IconID != testIconID {
		t.Fatalf("mismatched icon point properties: %+v", createdDoc.Data)
	}

	// 5. List IconPoints for dataset
	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/datasets/"+datasetID+"/icon-points", nil)
	listReq.Header.Set("X-Session-ID", sessionID)
	listResp, err := client.Do(listReq)
	if err != nil {
		t.Fatalf("failed listing icon points: %v", err)
	}
	defer listResp.Body.Close()
	var listDoc struct {
		Data []*models.IconPoint `json:"data"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&listDoc)
	if len(listDoc.Data) != 1 {
		t.Fatalf("expected 1 icon point, got %d", len(listDoc.Data))
	}

	// 6. Update IconPoint
	newDesc := "Updated surveillance desk"
	updateBody, _ := json.Marshal(map[string]interface{}{
		"description": newDesc,
		"scope":       "building",
	})
	updateReq, _ := http.NewRequest(http.MethodPut, server.URL+"/icon-points/"+ptID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-Session-ID", sessionID)
	updateResp, err := client.Do(updateReq)
	if err != nil {
		t.Fatalf("failed updating icon point: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK updating icon point, got %d", updateResp.StatusCode)
	}

	// 7. Check workspace records includes icon point
	recReq, _ := http.NewRequest(http.MethodGet, server.URL+"/datasets/"+datasetID+"/workspace/records?kind=icon_point", nil)
	recReq.Header.Set("X-Session-ID", sessionID)
	recResp, err := client.Do(recReq)
	if err != nil {
		t.Fatalf("failed getting workspace records: %v", err)
	}
	defer recResp.Body.Close()
	var recDoc struct {
		Data struct {
			Records []map[string]interface{} `json:"records"`
			Total   int                      `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(recResp.Body).Decode(&recDoc)
	if recDoc.Data.Total < 1 {
		t.Fatalf("expected at least 1 icon point record in workspace, got %d", recDoc.Data.Total)
	}

	// 8. Delete IconPoint
	delReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/icon-points/"+ptID, nil)
	delReq.Header.Set("X-Session-ID", sessionID)
	delResp, err := client.Do(delReq)
	if err != nil {
		t.Fatalf("failed deleting icon point: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK deleting icon point, got %d", delResp.StatusCode)
	}

	// 9. Verify get after delete returns 404
	getReq, _ := http.NewRequest(http.MethodGet, server.URL+"/icon-points/"+ptID, nil)
	getReq.Header.Set("X-Session-ID", sessionID)
	getResp, err := client.Do(getReq)
	if err != nil {
		t.Fatalf("failed checking deleted point: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted point, got %d", getResp.StatusCode)
	}
}
