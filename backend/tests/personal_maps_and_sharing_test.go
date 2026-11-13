package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/maps"
	"github.com/malikharman0823/veracity-maps/functions/profile"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func setupPersonalMapTestServer(t *testing.T) (*httptest.Server, *repository.MemoryMapRepository, repository.AuthRepo, *fakeMailer) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "personal-maps-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	cfg := &config.Config{
		SessionStorageDir:  filepath.Join(tempDir, "sessions"),
		UploadDir:          filepath.Join(tempDir, "uploads"),
		ProcessedDir:       filepath.Join(tempDir, "processed"),
		MaxUploadSizeBytes: 50 * 1024 * 1024,
		Environment:        "test",
	}

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("storage error: %v", err)
	}

	repo := repository.NewSessionRepository()
	repo.SetStorageDir(cfg.SessionStorageDir)

	authRepo, err := repository.NewAuthRepository(filepath.Join(tempDir, "auth"))
	if err != nil {
		t.Fatalf("auth repo error: %v", err)
	}
	otpRepo := repository.NewMemoryOTPRepository()
	profileRepo := repository.NewMemoryProfileRepository(authRepo)
	mapRepo := repository.NewMemoryMapRepository()

	mw := api.NewMiddleware(repo, authRepo)
	mockMailer := &fakeMailer{}

	dsService := services.NewDatasetService(cfg, repo, sm)
	geoService := services.NewGeoreferenceService(repo, sm)
	expService := services.NewExportService(repo, sm)

	uploadH := upload.NewHandler(dsService, repo, sm)
	uploadH.SetMapRepo(mapRepo)

	datasetsH := datasets.NewHandler(dsService, repo)
	georefH := georeference.NewHandler(geoService, repo)
	exportH := export.NewHandler(expService, repo)
	authH := auth.NewHandlerWithOTP(authRepo, otpRepo, profileRepo, mockMailer, "http://localhost:5173")
	projectsH := projects.NewHandler(nil, repo, authRepo, mockMailer, "http://localhost:5173")
	profileH := profile.NewHandler(profileRepo, mockMailer, tempDir, "http://localhost:5173")
	mapsH := maps.NewHandler(mapRepo, mockMailer, "http://localhost:5173")
	mapsH.SetConfigurationService(services.NewMapConfigurationService(mapRepo, cfg.UploadDir, cfg.SessionStorageDir))

	router := api.NewRouter(repo, authRepo, nil, mw, uploadH, datasetsH, georefH, exportH, authH, projectsH, profileH)
	router.SetMapsHandler(mapsH)

	server := httptest.NewServer(router.BuildHandler())
	t.Cleanup(func() { server.Close() })

	return server, mapRepo, authRepo, mockMailer
}

func registerAndLoginUser(t *testing.T, serverURL, email, name string) (string, *models.User) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Send OTP (purpose: register)
	sendBody, _ := json.Marshal(map[string]string{
		"email":   email,
		"name":    name,
		"purpose": "register",
	})
	resp, err := client.Post(serverURL+"/api/auth/send-otp", "application/json", bytes.NewReader(sendBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to send OTP for %s: %v, status: %d", email, err, resp.StatusCode)
	}

	// 2. Verify OTP (test bypass "999999")
	verifyBody, _ := json.Marshal(map[string]string{
		"email":   email,
		"code":    "999999",
		"name":    name,
		"purpose": "register",
	})
	resp, err = client.Post(serverURL+"/api/auth/verify-otp", "application/json", bytes.NewReader(verifyBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to verify OTP for %s: %v, status: %d", email, err, resp.StatusCode)
	}
	defer resp.Body.Close()

	var res struct {
		Success bool `json:"success"`
		Data    struct {
			Token string       `json:"token"`
			User  *models.User `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode auth response: %v", err)
	}

	return res.Data.Token, res.Data.User
}

func loginUserWithOTP(t *testing.T, serverURL, email string) (string, *models.User) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	sendBody, _ := json.Marshal(map[string]string{"email": email, "purpose": "login"})
	response, err := client.Post(serverURL+"/api/auth/send-otp", "application/json", bytes.NewReader(sendBody))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("failed to send login OTP for %s: %v, status: %d", email, err, response.StatusCode)
	}
	response.Body.Close()

	verifyBody, _ := json.Marshal(map[string]string{"email": email, "code": "999999", "purpose": "login"})
	response, err = client.Post(serverURL+"/api/auth/verify-otp", "application/json", bytes.NewReader(verifyBody))
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("failed to verify login OTP for %s: %v, status: %d", email, err, response.StatusCode)
	}
	defer response.Body.Close()
	var result struct {
		Data struct {
			Token string       `json:"token"`
			User  *models.User `json:"user"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return result.Data.Token, result.Data.User
}

func createTestPNGBytes(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 50, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestCreateMapFromConfigurationReturnsRevisionOne(t *testing.T) {
	server, _, _, _ := setupPersonalMapTestServer(t)
	client := &http.Client{Timeout: 10 * time.Second}
	token, user := registerAndLoginUser(t, server.URL, "durable-owner@example.com", "Durable Owner")

	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	part, err := writer.CreateFormFile("file", "ground-floor.png")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := part.Write(createTestPNGBytes(100, 100)); err != nil {
		t.Fatalf("write upload body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	uploadReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/upload", &uploadBody)
	if err != nil {
		t.Fatalf("create upload request: %v", err)
	}
	uploadReq.Header.Set("Authorization", "Bearer "+token)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		t.Fatalf("upload request returned error: %v", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(uploadResp.Body)
		t.Fatalf("upload status = %d, want 201; body=%s", uploadResp.StatusCode, body)
	}
	sessionID := uploadResp.Header.Get("X-Session-ID")
	var uploadResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploadResult); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}

	requestBody, _ := json.Marshal(map[string]interface{}{
		"name":               "Ground Floor",
		"description":        "Saved from workspace",
		"primary_dataset_id": uploadResult.Data.ID,
		"members":            []string{uploadResult.Data.ID},
		"configuration": map[string]interface{}{
			"active_view": "map",
			"camera":      map[string]float64{"zoom": 18},
		},
	})
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/maps/from-configuration", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("create map request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("create map request returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("create map status = %d, want 201; body=%s", response.StatusCode, body)
	}
	var result struct {
		Data *models.MapConfigurationResult `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode create map response: %v", err)
	}
	if result.Data == nil || result.Data.Map == nil || result.Data.Revision == nil {
		t.Fatalf("incomplete durable result: %#v", result.Data)
	}
	if result.Data.Map.OwnerUserID != user.ID || result.Data.Map.RevisionNumber != 1 || result.Data.Revision.RevisionNumber != 1 {
		t.Fatalf("durable result = %#v, want owner %q at revision 1", result.Data, user.ID)
	}
	if result.Data.Map.ID == "" || result.Data.Revision.MapID != result.Data.Map.ID {
		t.Fatalf("map/revision IDs do not match: %#v", result.Data)
	}

	deleteSessionReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/sessions/"+sessionID, nil)
	deleteSessionReq.Header.Set("Authorization", "Bearer "+token)
	deleteSessionResp, err := client.Do(deleteSessionReq)
	if err != nil || deleteSessionResp.StatusCode != http.StatusOK {
		t.Fatalf("delete session failed: %v, status: %d", err, deleteSessionResp.StatusCode)
	}
	deleteSessionResp.Body.Close()

	logoutReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+token)
	logoutResp, err := client.Do(logoutReq)
	if err != nil || logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("logout failed: %v, status: %d", err, logoutResp.StatusCode)
	}
	logoutResp.Body.Close()

	staleTokenReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/maps/"+result.Data.Map.ID, nil)
	staleTokenReq.Header.Set("Authorization", "Bearer "+token)
	staleTokenResp, err := client.Do(staleTokenReq)
	if err != nil {
		t.Fatalf("stale-token request failed: %v", err)
	}
	staleTokenResp.Body.Close()
	if staleTokenResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want 401 after logout", staleTokenResp.StatusCode)
	}

	newToken, loggedInUser := loginUserWithOTP(t, server.URL, "durable-owner@example.com")
	if loggedInUser.ID != user.ID {
		t.Fatalf("logged-in user ID = %q, want %q", loggedInUser.ID, user.ID)
	}
	reopenReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/maps/"+result.Data.Map.ID, nil)
	reopenReq.Header.Set("Authorization", "Bearer "+newToken)
	reopenResp, err := client.Do(reopenReq)
	if err != nil || reopenResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(reopenResp.Body)
		t.Fatalf("reopen map failed after session deletion and login: %v, status: %d, body=%s", err, reopenResp.StatusCode, body)
	}
	defer reopenResp.Body.Close()
	var reopened struct {
		Data *models.Map `json:"data"`
	}
	if err := json.NewDecoder(reopenResp.Body).Decode(&reopened); err != nil {
		t.Fatalf("decode reopened map: %v", err)
	}
	if reopened.Data == nil || reopened.Data.ID != result.Data.Map.ID || reopened.Data.RevisionNumber != 1 {
		t.Fatalf("reopened map = %#v, want original map at revision 1", reopened.Data)
	}
}

func TestPersonalMap_EndToEndSharingAndEditingFlow(t *testing.T) {
	server, _, _, _ := setupPersonalMapTestServer(t)
	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Register User A (Alice, Owner) & User B (Bob, Collaborator)
	aliceToken, alice := registerAndLoginUser(t, server.URL, "alice@example.com", "Alice Architect")
	bobToken, bob := registerAndLoginUser(t, server.URL, "bob@example.com", "Bob Builder")

	// 2. Alice creates a personal map
	createMapBody, _ := json.Marshal(map[string]string{
		"name":        "Floorplan Alpha",
		"description": "Ground floor architectural layout",
	})
	req, _ := http.NewRequest("POST", server.URL+"/api/maps", bytes.NewReader(createMapBody))
	req.Header.Set("Authorization", "Bearer "+aliceToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create map: %v, status: %d", err, resp.StatusCode)
	}
	var createdMapRes struct {
		Data *models.Map `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&createdMapRes)
	resp.Body.Close()
	mapID := createdMapRes.Data.ID
	if mapID == "" || createdMapRes.Data.OwnerUserID != alice.ID {
		t.Fatalf("invalid created map: %+v", createdMapRes.Data)
	}

	// 3. Alice uploads a file associated with mapID
	pngBytes := createTestPNGBytes(100, 100)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "floorplan_v1.png")
	part.Write(pngBytes)
	writer.WriteField("map_id", mapID)
	writer.Close()

	uploadReq, _ := http.NewRequest("POST", server.URL+"/api/upload", &body)
	uploadReq.Header.Set("Authorization", "Bearer "+aliceToken)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := client.Do(uploadReq)
	if err != nil || uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %v, status: %d", err, uploadResp.StatusCode)
	}
	uploadResp.Body.Close()

	// 4. Verify Alice's personal files list contains the uploaded file
	filesReq, _ := http.NewRequest("GET", server.URL+"/api/user/files", nil)
	filesReq.Header.Set("Authorization", "Bearer "+aliceToken)
	filesResp, err := client.Do(filesReq)
	if err != nil || filesResp.StatusCode != http.StatusOK {
		t.Fatalf("list files failed: %v, status: %d", err, filesResp.StatusCode)
	}
	var filesRes struct {
		Data struct {
			Files []*models.MapAsset `json:"files"`
			Total int                `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(filesResp.Body).Decode(&filesRes)
	filesResp.Body.Close()
	if filesRes.Data.Total == 0 {
		t.Fatalf("expected at least 1 personal uploaded file, got %d", filesRes.Data.Total)
	}

	// 5. Alice acquires exclusive lock on map
	lockReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/lock", nil)
	lockReq.Header.Set("Authorization", "Bearer "+aliceToken)
	lockResp, err := client.Do(lockReq)
	if err != nil || lockResp.StatusCode != http.StatusOK {
		t.Fatalf("acquire lock failed: %v, status: %d", err, lockResp.StatusCode)
	}
	lockResp.Body.Close()

	// 6. Alice saves initial configuration (Revision 1)
	saveBody, _ := json.Marshal(map[string]interface{}{
		"configuration_json": `{"version":1,"layers":[{"id":"layer-1","opacity":1.0}]}`,
		"description":        "Initial baseline floorplan",
		"expected_revision":  0,
	})
	saveReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/save", bytes.NewReader(saveBody))
	saveReq.Header.Set("Authorization", "Bearer "+aliceToken)
	saveReq.Header.Set("Content-Type", "application/json")
	saveResp, err := client.Do(saveReq)
	if err != nil || saveResp.StatusCode != http.StatusOK {
		t.Fatalf("save config failed: %v, status: %d", err, saveResp.StatusCode)
	}
	saveResp.Body.Close()

	// Alice releases lock
	unlockReq, _ := http.NewRequest("DELETE", server.URL+"/api/maps/"+mapID+"/lock", nil)
	unlockReq.Header.Set("Authorization", "Bearer "+aliceToken)
	unlockResp, _ := client.Do(unlockReq)
	unlockResp.Body.Close()

	// 7. Alice invites Bob via email invitation with editor permission
	inviteBody, _ := json.Marshal(map[string]string{
		"email":      bob.Email,
		"permission": "editor",
	})
	inviteReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/invitations", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("Authorization", "Bearer "+aliceToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteResp, err := client.Do(inviteReq)
	if err != nil || inviteResp.StatusCode != http.StatusCreated {
		t.Fatalf("invite failed: %v, status: %d", err, inviteResp.StatusCode)
	}
	var inviteRes struct {
		Data *models.MapShareInvitation `json:"data"`
	}
	_ = json.NewDecoder(inviteResp.Body).Decode(&inviteRes)
	inviteResp.Body.Close()
	inviteToken := inviteRes.Data.Token
	if inviteToken == "" {
		t.Fatalf("expected non-empty invitation token")
	}

	// 7b. Inspect invitation metadata and verify wrong account (Alice) is blocked with 403 INVITATION_EMAIL_MISMATCH
	inspectReq, _ := http.NewRequest("GET", server.URL+"/api/map-invitations/"+inviteToken, nil)
	inspectReq.Header.Set("Authorization", "Bearer "+aliceToken)
	inspectResp, err := client.Do(inspectReq)
	if err != nil || inspectResp.StatusCode != http.StatusOK {
		t.Fatalf("inspect invitation failed: %v, status: %d", err, inspectResp.StatusCode)
	}
	var inspectRes struct {
		Data struct {
			Email        string `json:"email"`
			EmailMatches bool   `json:"email_matches"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	_ = json.NewDecoder(inspectResp.Body).Decode(&inspectRes)
	inspectResp.Body.Close()
	if inspectRes.Data.Email != bob.Email || inspectRes.Data.EmailMatches || inspectRes.Data.Status != "pending" {
		t.Fatalf("unexpected invitation inspect result for Alice: %+v", inspectRes.Data)
	}

	wrongAcceptReq, _ := http.NewRequest("POST", server.URL+"/api/map-invitations/"+inviteToken+"/accept", nil)
	wrongAcceptReq.Header.Set("Authorization", "Bearer "+aliceToken)
	wrongAcceptResp, err := client.Do(wrongAcceptReq)
	if err != nil || wrongAcceptResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when wrong user accepts invitation, got err=%v status=%d", err, wrongAcceptResp.StatusCode)
	}
	var wrongAcceptErr struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(wrongAcceptResp.Body).Decode(&wrongAcceptErr)
	wrongAcceptResp.Body.Close()
	if wrongAcceptErr.Error.Code != "INVITATION_EMAIL_MISMATCH" {
		t.Fatalf("expected INVITATION_EMAIL_MISMATCH error code, got %q", wrongAcceptErr.Error.Code)
	}

	// 8. Bob accepts the invitation
	acceptReq, _ := http.NewRequest("POST", server.URL+"/api/map-invitations/"+inviteToken+"/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer "+bobToken)
	acceptResp, err := client.Do(acceptReq)
	if err != nil || acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("accept invitation failed: %v, status: %d", err, acceptResp.StatusCode)
	}
	acceptResp.Body.Close()

	// 9. Bob lists maps: verifies map appears in shared_with_me with 'editor' permission
	bobMapsReq, _ := http.NewRequest("GET", server.URL+"/api/maps", nil)
	bobMapsReq.Header.Set("Authorization", "Bearer "+bobToken)
	bobMapsResp, err := client.Do(bobMapsReq)
	if err != nil || bobMapsResp.StatusCode != http.StatusOK {
		t.Fatalf("bob list maps failed: %v, status: %d", err, bobMapsResp.StatusCode)
	}
	var bobMapsRes struct {
		Data struct {
			MyMaps       []*models.Map `json:"my_maps"`
			SharedWithMe []*models.Map `json:"shared_with_me"`
			SavedMaps    []*models.Map `json:"saved_maps"`
		} `json:"data"`
	}
	bobMapsBody, err := io.ReadAll(bobMapsResp.Body)
	if err != nil {
		t.Fatalf("read Bob maps response: %v", err)
	}
	if err := json.Unmarshal(bobMapsBody, &bobMapsRes); err != nil {
		t.Fatalf("decode Bob maps response: %v", err)
	}
	bobMapsResp.Body.Close()

	if len(bobMapsRes.Data.SharedWithMe) == 0 {
		t.Fatalf("expected map in Bob's shared_with_me, got 0")
	}
	sharedMap := bobMapsRes.Data.SharedWithMe[0]
	if sharedMap.ID != mapID || sharedMap.Permission != models.PermissionEditor {
		t.Fatalf("unexpected shared map permission: %+v", sharedMap)
	}
	var bobMapsContract struct {
		Data struct {
			SharedWithMe []struct {
				SharedBy     string `json:"shared_by"`
				SharedByName string `json:"shared_by_name"`
			} `json:"shared_with_me"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bobMapsBody, &bobMapsContract); err != nil {
		t.Fatalf("decode Bob shared-map contract: %v", err)
	}
	if got := bobMapsContract.Data.SharedWithMe[0]; got.SharedBy != alice.ID || got.SharedByName != alice.Name {
		t.Fatalf("shared map inviter = %q (%q), want %q (%q)", got.SharedByName, got.SharedBy, alice.Name, alice.ID)
	}
	if len(bobMapsRes.Data.SavedMaps) != 0 {
		t.Fatalf("shared map appeared in Bob's saved maps before he saved it: %+v", bobMapsRes.Data.SavedMaps)
	}

	// 10. Bob acquires lock, saves Revision 2, releases lock
	bobLockReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/lock", nil)
	bobLockReq.Header.Set("Authorization", "Bearer "+bobToken)
	bobLockResp, _ := client.Do(bobLockReq)
	if bobLockResp.StatusCode != http.StatusOK {
		t.Fatalf("bob failed to acquire lock: status %d", bobLockResp.StatusCode)
	}
	bobLockResp.Body.Close()

	bobSaveBody, _ := json.Marshal(map[string]interface{}{
		"configuration_json": `{"version":2,"layers":[{"id":"layer-1","opacity":0.8},{"id":"layer-2","opacity":1.0}]}`,
		"description":        "Bob added secondary floor overlay",
		"expected_revision":  2,
	})
	bobSaveReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/save", bytes.NewReader(bobSaveBody))
	bobSaveReq.Header.Set("Authorization", "Bearer "+bobToken)
	bobSaveReq.Header.Set("Content-Type", "application/json")
	bobSaveResp, err := client.Do(bobSaveReq)
	if err != nil || bobSaveResp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(bobSaveResp.Body)
		t.Fatalf("bob save failed: %v, status: %d, body: %s", err, bobSaveResp.StatusCode, string(respBytes))
	}
	var bobSaveResult struct {
		Data *models.MapConfigurationResult `json:"data"`
	}
	if err := json.NewDecoder(bobSaveResp.Body).Decode(&bobSaveResult); err != nil {
		t.Fatalf("decode Bob save response: %v", err)
	}
	bobSaveResp.Body.Close()
	if bobSaveResult.Data == nil || bobSaveResult.Data.Map == nil || bobSaveResult.Data.Revision == nil {
		t.Fatalf("Bob save returned incomplete result: %#v", bobSaveResult.Data)
	}
	if bobSaveResult.Data.Map.ID != mapID || bobSaveResult.Data.Map.RevisionNumber != 3 {
		t.Fatalf("Bob save map = %#v, want same shared map at revision 3", bobSaveResult.Data.Map)
	}
	if bobSaveResult.Data.Revision.MapID != mapID || bobSaveResult.Data.Revision.CreatedBy != bob.ID {
		t.Fatalf("Bob revision = %#v, want same map authored by Bob", bobSaveResult.Data.Revision)
	}

	bobSavedMapsReq, _ := http.NewRequest("GET", server.URL+"/api/maps", nil)
	bobSavedMapsReq.Header.Set("Authorization", "Bearer "+bobToken)
	bobSavedMapsResp, err := client.Do(bobSavedMapsReq)
	if err != nil || bobSavedMapsResp.StatusCode != http.StatusOK {
		t.Fatalf("Bob list saved maps failed after save: %v, status: %d", err, bobSavedMapsResp.StatusCode)
	}
	var bobSavedMapsRes struct {
		Data struct {
			SavedMaps []*models.Map `json:"saved_maps"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bobSavedMapsResp.Body).Decode(&bobSavedMapsRes); err != nil {
		t.Fatalf("decode Bob saved maps response: %v", err)
	}
	bobSavedMapsResp.Body.Close()
	if len(bobSavedMapsRes.Data.SavedMaps) != 1 {
		t.Fatalf("Bob saved maps count = %d, want 1 after saving shared map", len(bobSavedMapsRes.Data.SavedMaps))
	}
	if savedMap := bobSavedMapsRes.Data.SavedMaps[0]; savedMap.ID != mapID || savedMap.OwnerUserID != alice.ID {
		t.Fatalf("Bob saved map = %+v, want Alice-owned shared map %q", savedMap, mapID)
	}

	// Bob releases lock
	bobUnlockReq, _ := http.NewRequest("DELETE", server.URL+"/api/maps/"+mapID+"/lock", nil)
	bobUnlockReq.Header.Set("Authorization", "Bearer "+bobToken)
	bobUnlockResp, _ := client.Do(bobUnlockReq)
	bobUnlockResp.Body.Close()

	// 11. Alice creates a public share link (editor access)
	linkBody, _ := json.Marshal(map[string]interface{}{
		"permission":      "editor",
		"is_public":       true,
		"expires_in_days": 30,
	})
	linkReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/share-links", bytes.NewReader(linkBody))
	linkReq.Header.Set("Authorization", "Bearer "+aliceToken)
	linkReq.Header.Set("Content-Type", "application/json")
	linkResp, err := client.Do(linkReq)
	if err != nil || linkResp.StatusCode != http.StatusCreated {
		t.Fatalf("create share link failed: %v, status: %d", err, linkResp.StatusCode)
	}
	var linkRes struct {
		Data struct {
			Token string            `json:"token"`
			Link  *models.ShareLink `json:"link"`
		} `json:"data"`
	}
	_ = json.NewDecoder(linkResp.Body).Decode(&linkRes)
	linkResp.Body.Close()
	shareToken := linkRes.Data.Token
	linkID := linkRes.Data.Link.ID

	// 12. Unauthenticated client accesses public share link
	publicReq, _ := http.NewRequest("GET", server.URL+"/api/share/"+shareToken, nil)
	publicResp, err := client.Do(publicReq)
	if err != nil || publicResp.StatusCode != http.StatusOK {
		t.Fatalf("public share link access failed: %v, status: %d", err, publicResp.StatusCode)
	}
	var publicRes struct {
		Data struct {
			Map        *models.Map `json:"map"`
			Permission string      `json:"permission"`
		} `json:"data"`
	}
	_ = json.NewDecoder(publicResp.Body).Decode(&publicRes)
	publicResp.Body.Close()
	if publicRes.Data.Map.ID != mapID || publicRes.Data.Map.RevisionNumber != 3 {
		t.Fatalf("unexpected public map details: %+v", publicRes.Data.Map)
	}

	// 13. Alice revokes share link
	revokeLinkReq, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/maps/%s/share-links/%s", server.URL, mapID, linkID), nil)
	revokeLinkReq.Header.Set("Authorization", "Bearer "+aliceToken)
	revokeLinkResp, _ := client.Do(revokeLinkReq)
	if revokeLinkResp.StatusCode != http.StatusOK {
		t.Fatalf("revoke share link failed: status %d", revokeLinkResp.StatusCode)
	}
	revokeLinkResp.Body.Close()

	// Access revoked link -> 404 Not Found
	publicRevokedReq, _ := http.NewRequest("GET", server.URL+"/api/share/"+shareToken, nil)
	publicRevokedResp, _ := client.Do(publicRevokedReq)
	if publicRevokedResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for revoked link, got %d", publicRevokedResp.StatusCode)
	}
	publicRevokedResp.Body.Close()

	// 13b. Verify 10-member cap: Alice invites 8 more members (bringing total to 10 with Alice + Bob)
	for i := 1; i <= 8; i++ {
		memberBody, _ := json.Marshal(map[string]string{
			"email": fmt.Sprintf("member%d@example.com", i),
		})
		mReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/invitations", bytes.NewReader(memberBody))
		mReq.Header.Set("Authorization", "Bearer "+aliceToken)
		mReq.Header.Set("Content-Type", "application/json")
		mResp, err := client.Do(mReq)
		if err != nil || mResp.StatusCode != http.StatusCreated {
			t.Fatalf("expected invitation %d to succeed, got status %d", i, mResp.StatusCode)
		}
		mResp.Body.Close()
	}

	// 11th member invitation must fail with 400 MEMBER_CAP_REACHED
	overflowBody, _ := json.Marshal(map[string]string{
		"email": "overflow@example.com",
	})
	overReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/invitations", bytes.NewReader(overflowBody))
	overReq.Header.Set("Authorization", "Bearer "+aliceToken)
	overReq.Header.Set("Content-Type", "application/json")
	overResp, err := client.Do(overReq)
	if err != nil || overResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 MEMBER_CAP_REACHED when inviting 11th member, got status %d", overResp.StatusCode)
	}
	overResp.Body.Close()

	// 14. Bob attempts to delete map -> 403 Forbidden (Only Owner can delete)
	bobDelReq, _ := http.NewRequest("DELETE", server.URL+"/api/maps/"+mapID, nil)
	bobDelReq.Header.Set("Authorization", "Bearer "+bobToken)
	bobDelResp, _ := client.Do(bobDelReq)
	if bobDelResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when collaborator attempts to delete map, got %d", bobDelResp.StatusCode)
	}
	bobDelResp.Body.Close()

	// 15. Alice successfully deletes map
	aliceDelReq, _ := http.NewRequest("DELETE", server.URL+"/api/maps/"+mapID, nil)
	aliceDelReq.Header.Set("Authorization", "Bearer "+aliceToken)
	aliceDelResp, _ := client.Do(aliceDelReq)
	if aliceDelResp.StatusCode != http.StatusOK {
		t.Fatalf("owner delete map failed: status %d", aliceDelResp.StatusCode)
	}
	aliceDelResp.Body.Close()

	// Map is gone -> 404
	getGoneReq, _ := http.NewRequest("GET", server.URL+"/api/maps/"+mapID, nil)
	getGoneReq.Header.Set("Authorization", "Bearer "+aliceToken)
	getGoneResp, _ := client.Do(getGoneReq)
	if getGoneResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted map, got %d", getGoneResp.StatusCode)
	}
	getGoneResp.Body.Close()
}

func TestSharedMapFileAndGeoreferenceAdoptionForCollaborator(t *testing.T) {
	server, _, _, _ := setupPersonalMapTestServer(t)
	client := &http.Client{Timeout: 10 * time.Second}

	ownerToken, _ := registerAndLoginUser(t, server.URL, "harmanrox823@gmail.com", "Harman Owner")

	// 1. Owner creates a map
	createMapBody, _ := json.Marshal(map[string]string{
		"name":        "Shared Campus Map",
		"description": "Shared floorplan with georeferenced raster",
	})
	createReq, _ := http.NewRequest("POST", server.URL+"/api/maps", bytes.NewReader(createMapBody))
	createReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := client.Do(createReq)
	if err != nil || createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create map failed: %v, status: %d", err, createResp.StatusCode)
	}
	var createRes struct {
		Data *models.Map `json:"data"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&createRes)
	createResp.Body.Close()
	mapID := createRes.Data.ID

	// 2. Owner uploads a PNG dataset
	var uploadBody bytes.Buffer
	writer := multipart.NewWriter(&uploadBody)
	part, _ := writer.CreateFormFile("file", "shared-floor.png")
	_, _ = part.Write(createTestPNGBytes(100, 100))
	_ = writer.Close()

	uploadReq, _ := http.NewRequest("POST", server.URL+"/api/upload", &uploadBody)
	uploadReq.Header.Set("Authorization", "Bearer "+ownerToken)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := client.Do(uploadReq)
	if err != nil || uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("upload failed: %v, status: %d", err, uploadResp.StatusCode)
	}
	ownerSessionID := uploadResp.Header.Get("X-Session-ID")
	var uploadResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(uploadResp.Body).Decode(&uploadResult)
	uploadResp.Body.Close()
	datasetID := uploadResult.Data.ID

	// 3. Owner acquires lock and saves initial (pre-georeference) configuration (Revision 1)
	lockReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/lock", nil)
	lockReq.Header.Set("Authorization", "Bearer "+ownerToken)
	lockResp, _ := client.Do(lockReq)
	lockResp.Body.Close()

	save1Payload, _ := json.Marshal(map[string]interface{}{
		"name":               "Shared Campus Map",
		"primary_dataset_id": datasetID,
		"members":            []string{datasetID},
	})
	save1Body, _ := json.Marshal(map[string]interface{}{
		"configuration_json": string(save1Payload),
		"description":        "Initial upload",
		"expected_revision":  0,
	})
	save1Req, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/save", bytes.NewReader(save1Body))
	save1Req.Header.Set("Authorization", "Bearer "+ownerToken)
	save1Req.Header.Set("Content-Type", "application/json")
	save1Resp, err := client.Do(save1Req)
	if err != nil || save1Resp.StatusCode != http.StatusOK {
		t.Fatalf("save1 failed: %v, status: %d", err, save1Resp.StatusCode)
	}
	save1Resp.Body.Close()

	// 4. Owner adds 3 control points and runs georeference in ownerSessionID
	points := []map[string]interface{}{
		{"name": "NW", "image_x": 0, "image_y": 0, "longitude": 77.2000, "latitude": 28.6100},
		{"name": "NE", "image_x": 99, "image_y": 0, "longitude": 77.2100, "latitude": 28.6100},
		{"name": "SW", "image_x": 0, "image_y": 99, "longitude": 77.2000, "latitude": 28.6000},
	}
	for _, pt := range points {
		ptBody, _ := json.Marshal(pt)
		ptReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/control-points", bytes.NewReader(ptBody))
		ptReq.Header.Set("Authorization", "Bearer "+ownerToken)
		ptReq.Header.Set("X-Session-ID", ownerSessionID)
		ptReq.Header.Set("Content-Type", "application/json")
		ptResp, err := client.Do(ptReq)
		if err != nil || ptResp.StatusCode != http.StatusCreated {
			t.Fatalf("add control point failed: %v, status: %d", err, ptResp.StatusCode)
		}
		ptResp.Body.Close()
	}

	georefReq, _ := http.NewRequest("POST", server.URL+"/datasets/"+datasetID+"/georeference", bytes.NewReader([]byte(`{}`)))
	georefReq.Header.Set("Authorization", "Bearer "+ownerToken)
	georefReq.Header.Set("X-Session-ID", ownerSessionID)
	georefReq.Header.Set("Content-Type", "application/json")
	georefResp, err := client.Do(georefReq)
	if err != nil || georefResp.StatusCode != http.StatusOK {
		t.Fatalf("georeference failed: %v, status: %d", err, georefResp.StatusCode)
	}
	georefResp.Body.Close()

	// 5. Owner re-saves map configuration after georeferencing (Revision 2) so durable asset is updated
	save2Resp, err := client.Do(func() *http.Request {
		r, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/save", bytes.NewReader(save1Body))
		r.Header.Set("Authorization", "Bearer "+ownerToken)
		r.Header.Set("Content-Type", "application/json")
		return r
	}())
	if err != nil || save2Resp.StatusCode != http.StatusOK {
		t.Fatalf("save2 failed: %v, status: %d", err, save2Resp.StatusCode)
	}
	save2Resp.Body.Close()

	unlockReq, _ := http.NewRequest("DELETE", server.URL+"/api/maps/"+mapID+"/lock", nil)
	unlockReq.Header.Set("Authorization", "Bearer "+ownerToken)
	unlockResp, _ := client.Do(unlockReq)
	unlockResp.Body.Close()

	// 6. Owner invites malikharman666@gmail.com
	inviteBody, _ := json.Marshal(map[string]string{
		"email":      "malikharman666@gmail.com",
		"permission": "editor",
	})
	inviteReq, _ := http.NewRequest("POST", server.URL+"/api/maps/"+mapID+"/invitations", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteResp, err := client.Do(inviteReq)
	if err != nil || inviteResp.StatusCode != http.StatusCreated {
		t.Fatalf("invite failed: %v, status: %d", err, inviteResp.StatusCode)
	}
	var inviteRes struct {
		Data *models.MapShareInvitation `json:"data"`
	}
	_ = json.NewDecoder(inviteResp.Body).Decode(&inviteRes)
	inviteResp.Body.Close()

	// 7. Collaborator registers as malikharman666@gmail.com and accepts invitation
	collabToken, _ := registerAndLoginUser(t, server.URL, "malikharman666@gmail.com", "Malik Collaborator")
	acceptReq, _ := http.NewRequest("POST", server.URL+"/api/map-invitations/"+inviteRes.Data.Token+"/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer "+collabToken)
	acceptResp, err := client.Do(acceptReq)
	if err != nil || acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("collaborator accept failed: %v, status: %d", err, acceptResp.StatusCode)
	}
	acceptResp.Body.Close()

	// 8. Collaborator opens a fresh session and loads the shared map and its workspace dataset
	sessResp, err := client.Post(server.URL+"/sessions", "application/json", nil)
	if err != nil || sessResp.StatusCode != http.StatusCreated {
		t.Fatalf("collaborator create session failed: %v, status: %d", err, sessResp.StatusCode)
	}
	var sessData struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(sessResp.Body).Decode(&sessData)
	sessResp.Body.Close()
	collabSessionID := sessData.Data.ID

	wsReq, _ := http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/workspace", nil)
	wsReq.Header.Set("Authorization", "Bearer "+collabToken)
	wsReq.Header.Set("X-Session-ID", collabSessionID)
	wsResp, err := client.Do(wsReq)
	if err != nil || wsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(wsResp.Body)
		t.Fatalf("collaborator get workspace failed: %v, status: %d, body: %s", err, wsResp.StatusCode, string(body))
	}
	var wsData struct {
		Data struct {
			PrimaryDatasetID string `json:"primary_dataset_id"`
			Members          []struct {
				DatasetID       string `json:"dataset_id"`
				IsGeoreferenced bool   `json:"is_georeferenced"`
			} `json:"members"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wsResp.Body).Decode(&wsData)
	wsResp.Body.Close()
	if len(wsData.Data.Members) == 0 || !wsData.Data.Members[0].IsGeoreferenced {
		t.Fatalf("expected shared dataset to be georeferenced in collaborator workspace, got %+v", wsData.Data)
	}

	imgReq, _ := http.NewRequest("GET", server.URL+"/datasets/"+datasetID+"/source-image", nil)
	imgReq.Header.Set("Authorization", "Bearer "+collabToken)
	imgReq.Header.Set("X-Session-ID", collabSessionID)
	imgResp, err := client.Do(imgReq)
	if err != nil || imgResp.StatusCode != http.StatusOK {
		t.Fatalf("collaborator get source-image failed: %v, status: %d", err, imgResp.StatusCode)
	}
	imgResp.Body.Close()
}
