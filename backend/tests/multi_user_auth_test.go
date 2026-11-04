package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/organizations"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func setupMultiUserTestServer(t *testing.T, tempDir string) (*httptest.Server, *repository.AuthRepository, *repository.ProjectRepository) {
	cfg := &config.Config{
		Port:               "0",
		Host:               "127.0.0.1",
		SessionStorageDir:  tempDir + "/sessions",
		UploadDir:          tempDir + "/uploads",
		ProcessedDir:       tempDir + "/processed",
		MaxUploadSizeBytes: 50 * 1024 * 1024,
	}

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("failed creating storage manager: %v", err)
	}

	sessionRepo := repository.NewSessionRepository()
	authRepo, err := repository.NewAuthRepository(tempDir)
	if err != nil {
		t.Fatalf("failed creating auth repo: %v", err)
	}
	projRepo, err := repository.NewProjectRepository(tempDir)
	if err != nil {
		t.Fatalf("failed creating project repo: %v", err)
	}
	orgRepo, err := repository.NewOrgRepository(tempDir)
	if err != nil {
		t.Fatalf("failed creating org repo: %v", err)
	}

	mw := api.NewMiddleware(sessionRepo, authRepo)

	dsService := services.NewDatasetService(cfg, sessionRepo, sm)
	geoService := services.NewGeoreferenceService(sessionRepo, sm)
	expService := services.NewExportService(sessionRepo, sm)

	uHandler := upload.NewHandler(dsService, sessionRepo, sm)
	dHandler := datasets.NewHandler(dsService, sessionRepo)
	gHandler := georeference.NewHandler(geoService, sessionRepo)
	eHandler := export.NewHandler(expService, sessionRepo)
	aHandler := auth.NewHandler(authRepo)
	pHandler := projects.NewHandler(projRepo, sessionRepo, authRepo)
	oHandler := organizations.NewHandler(orgRepo, projRepo, authRepo)

	router := api.NewRouter(sessionRepo, authRepo, projRepo, orgRepo, mw, uHandler, dHandler, gHandler, eHandler, aHandler, pHandler, oHandler)
	server := httptest.NewServer(router.BuildHandler())

	return server, authRepo, projRepo
}

func TestMultiUserAuthenticationAndIsolation(t *testing.T) {
	tempDir := t.TempDir()
	server, _, _ := setupMultiUserTestServer(t, tempDir)
	defer server.Close()

	// 1. Register User A
	userABody, _ := json.Marshal(map[string]string{
		"name":     "User Alpha",
		"email":    "alpha@veracity.maps",
		"password": "Password123!",
	})
	resA, err := http.Post(server.URL+"/api/auth/register", "application/json", bytes.NewBuffer(userABody))
	if err != nil {
		t.Fatalf("User A registration request failed: %v", err)
	}
	if resA.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for User A, got %d", resA.StatusCode)
	}

	var resWrapperA struct {
		Success bool                `json:"success"`
		Data    models.AuthResponse `json:"data"`
	}
	if err := json.NewDecoder(resA.Body).Decode(&resWrapperA); err != nil {
		t.Fatalf("Failed to decode User A response: %v", err)
	}
	resA.Body.Close()
	authA := resWrapperA.Data

	if authA.Token == "" || authA.User.Email != "alpha@veracity.maps" {
		t.Fatalf("Invalid response for User A: %+v", authA)
	}

	// 2. Register User B
	userBBody, _ := json.Marshal(map[string]string{
		"name":     "User Beta",
		"email":    "beta@veracity.maps",
		"password": "Password456!",
	})
	resB, err := http.Post(server.URL+"/api/auth/register", "application/json", bytes.NewBuffer(userBBody))
	if err != nil {
		t.Fatalf("User B registration request failed: %v", err)
	}
	if resB.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for User B, got %d", resB.StatusCode)
	}

	var resWrapperB struct {
		Success bool                `json:"success"`
		Data    models.AuthResponse `json:"data"`
	}
	if err := json.NewDecoder(resB.Body).Decode(&resWrapperB); err != nil {
		t.Fatalf("Failed to decode User B response: %v", err)
	}
	resB.Body.Close()
	authB := resWrapperB.Data

	// 3. User A creates a private project
	projABody, _ := json.Marshal(map[string]string{
		"name":        "Alpha Private Survey",
		"description": "Confidential survey map",
	})
	reqCreateProjA, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects", bytes.NewBuffer(projABody))
	reqCreateProjA.Header.Set("Authorization", "Bearer "+authA.Token)
	reqCreateProjA.Header.Set("Content-Type", "application/json")
	resCreateA, err := http.DefaultClient.Do(reqCreateProjA)
	if err != nil {
		t.Fatalf("User A create project request failed: %v", err)
	}
	if resCreateA.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for project A, got %d", resCreateA.StatusCode)
	}

	var wrapProjA struct {
		Success bool               `json:"success"`
		Data    models.UserProject `json:"data"`
	}
	if err := json.NewDecoder(resCreateA.Body).Decode(&wrapProjA); err != nil {
		t.Fatalf("Failed to decode Project A: %v", err)
	}
	resCreateA.Body.Close()
	projectA := wrapProjA.Data

	// 4. Isolation Check: User B lists projects -> Should NOT see User A's project
	reqListB, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects", nil)
	reqListB.Header.Set("Authorization", "Bearer "+authB.Token)
	resListB, err := http.DefaultClient.Do(reqListB)
	if err != nil {
		t.Fatalf("User B list projects request failed: %v", err)
	}
	if resListB.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for list projects, got %d", resListB.StatusCode)
	}

	var wrapListB struct {
		Success bool `json:"success"`
		Data    struct {
			Projects []*models.UserProject `json:"projects"`
			Total    int                   `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resListB.Body).Decode(&wrapListB); err != nil {
		t.Fatalf("Failed to decode list projects for User B: %v", err)
	}
	resListB.Body.Close()
	listBData := wrapListB.Data

	if listBData.Total != 0 || len(listBData.Projects) != 0 {
		t.Fatalf("User B shouldn't see any projects, got: %d", listBData.Total)
	}

	// 5. IDOR Attack Check: User B directly attempts to GET User A's project -> Must return 403 Forbidden
	reqGetAByB, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/"+projectA.ID, nil)
	reqGetAByB.Header.Set("Authorization", "Bearer "+authB.Token)
	resGetAByB, err := http.DefaultClient.Do(reqGetAByB)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resGetAByB.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for IDOR attempt, got %d", resGetAByB.StatusCode)
	}
	resGetAByB.Body.Close()

	// 6. IDOR Delete Attack Check: User B attempts to DELETE User A's project -> Must return 403 Forbidden
	reqDelAByB, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/projects/"+projectA.ID, nil)
	reqDelAByB.Header.Set("Authorization", "Bearer "+authB.Token)
	resDelAByB, err := http.DefaultClient.Do(reqDelAByB)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resDelAByB.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for IDOR delete attempt, got %d", resDelAByB.StatusCode)
	}
	resDelAByB.Body.Close()

	// 7. Restart Persistence Check: Simulate backend restart with the same storage directory
	server.Close()

	serverRestarted, _, _ := setupMultiUserTestServer(t, tempDir)
	defer serverRestarted.Close()

	// User A logs back in with email and password
	loginABody, _ := json.Marshal(map[string]string{
		"email":    "alpha@veracity.maps",
		"password": "Password123!",
	})
	resLoginA, err := http.Post(serverRestarted.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginABody))
	if err != nil {
		t.Fatalf("User A login after restart failed: %v", err)
	}
	if resLoginA.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for login after restart, got %d", resLoginA.StatusCode)
	}

	var wrapLoginA struct {
		Success bool                `json:"success"`
		Data    models.AuthResponse `json:"data"`
	}
	_ = json.NewDecoder(resLoginA.Body).Decode(&wrapLoginA)
	resLoginA.Body.Close()
	authARestart := wrapLoginA.Data

	// User A gets project list -> Project A MUST persist!
	reqListARestart, _ := http.NewRequest(http.MethodGet, serverRestarted.URL+"/api/projects", nil)
	reqListARestart.Header.Set("Authorization", "Bearer "+authARestart.Token)
	resListARestart, err := http.DefaultClient.Do(reqListARestart)
	if err != nil {
		t.Fatalf("User A list after restart failed: %v", err)
	}

	var wrapListA struct {
		Success bool `json:"success"`
		Data    struct {
			Projects []*models.UserProject `json:"projects"`
			Total    int                   `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resListARestart.Body).Decode(&wrapListA)
	resListARestart.Body.Close()
	listARestart := wrapListA.Data

	if listARestart.Total != 1 || listARestart.Projects[0].Name != "Alpha Private Survey" {
		t.Fatalf("Expected 1 persisted project 'Alpha Private Survey', got %d", listARestart.Total)
	}
}
