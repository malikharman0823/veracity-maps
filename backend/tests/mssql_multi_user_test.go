package tests

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/db"
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

func getTestMSSQLDB(t *testing.T) *sql.DB {
	connStr := os.Getenv("MSSQL_CONNECTION_STRING")
	if connStr == "" {
		// Probe 1434 (default on this instance) then 1433
		candidates := []string{
			"server=127.0.0.1;port=1434;database=veracity_maps;trusted_connection=yes;encrypt=disable",
			"server=localhost;port=1434;database=veracity_maps;trusted_connection=yes;encrypt=disable",
			"server=localhost;database=veracity_maps;trusted_connection=yes;encrypt=disable",
		}
		for _, candidate := range candidates {
			if testDB, err := db.InitDB(candidate); err == nil {
				connStr = candidate
				_ = testDB.Close()
				break
			}
		}
	}
	if connStr == "" {
		connStr = "server=127.0.0.1;port=1434;database=veracity_maps;trusted_connection=yes;encrypt=disable"
	}

	sqlDB, err := db.InitDB(connStr)
	if err != nil {
		t.Skipf("Skipping MSSQL integration test: cannot connect to SQL Server (%v)", err)
		return nil
	}

	// Apply migrations
	migrationsDir := filepath.Join("..", "migrations")
	if err := db.RunMigrations(sqlDB, migrationsDir); err != nil {
		t.Fatalf("Failed running migrations on test MSSQL database: %v", err)
	}

	return sqlDB
}

func setupMSSQLServer(t *testing.T, sqlDB *sql.DB) (*httptest.Server, *repository.AuthRepository, *repository.ProjectRepository, *repository.OrgRepository) {
	tempDir := t.TempDir()
	cfg := &config.Config{
		SessionStorageDir:  filepath.Join(tempDir, "sessions"),
		UploadDir:          filepath.Join(tempDir, "uploads"),
		ProcessedDir:       filepath.Join(tempDir, "processed"),
		MaxUploadSizeBytes: 100 * 1024 * 1024,
	}

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	sessionRepo := repository.NewSessionRepository()
	sessionRepo.SetStorageDir(cfg.SessionStorageDir)

	authRepo, err := repository.NewAuthRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to initialize authRepo: %v", err)
	}
	authRepo.SetDB(sqlDB)

	projRepo, err := repository.NewProjectRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to initialize projRepo: %v", err)
	}
	projRepo.SetDB(sqlDB)

	orgRepo, err := repository.NewOrgRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to initialize orgRepo: %v", err)
	}
	orgRepo.SetDB(sqlDB)

	middleware := api.NewMiddleware(sessionRepo, authRepo)
	datasetService := services.NewDatasetService(cfg, sessionRepo, sm)
	georefService := services.NewGeoreferenceService(sessionRepo, sm)
	exportService := services.NewExportService(sessionRepo, sm)

	uploadHandler := upload.NewHandler(datasetService, sessionRepo, sm)
	datasetsHandler := datasets.NewHandler(datasetService, sessionRepo)
	georefHandler := georeference.NewHandler(georefService, sessionRepo)
	exportHandler := export.NewHandler(exportService, sessionRepo)
	authHandler := auth.NewHandler(authRepo)
	projectsHandler := projects.NewHandler(projRepo, sessionRepo, authRepo)
	orgHandler := organizations.NewHandler(orgRepo, projRepo, authRepo)

	router := api.NewRouter(
		sessionRepo,
		authRepo,
		projRepo,
		orgRepo,
		middleware,
		uploadHandler,
		datasetsHandler,
		georefHandler,
		exportHandler,
		authHandler,
		projectsHandler,
		orgHandler,
	)

	server := httptest.NewServer(router.BuildHandler())
	return server, authRepo, projRepo, orgRepo
}

func parseResponse(resp *http.Response, target interface{}) {
	var wrapper struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&wrapper)
	_ = json.Unmarshal(wrapper.Data, target)
}

func TestMSSQL_CompleteMultiUserWorkflow(t *testing.T) {
	sqlDB := getTestMSSQLDB(t)
	if sqlDB == nil {
		return
	}
	defer sqlDB.Close()

	server, _, _, _ := setupMSSQLServer(t, sqlDB)
	defer server.Close()

	client := server.Client()

	// 1. User A & User B Register
	emailA := strings.ToLower("userA_" + t.Name() + "@veracity.com")
	emailB := strings.ToLower("userB_" + t.Name() + "@veracity.com")

	regPayloadA := `{"name":"User Alpha","email":"` + emailA + `","password":"password123"}`
	respA, err := client.Post(server.URL+"/api/auth/register", "application/json", strings.NewReader(regPayloadA))
	if err != nil || respA.StatusCode != http.StatusCreated {
		t.Fatalf("User A registration failed: status %d, err %v", respA.StatusCode, err)
	}
	var authA models.AuthResponse
	parseResponse(respA, &authA)

	regPayloadB := `{"name":"User Beta","email":"` + emailB + `","password":"password123"}`
	respB, err := client.Post(server.URL+"/api/auth/register", "application/json", strings.NewReader(regPayloadB))
	if err != nil || respB.StatusCode != http.StatusCreated {
		t.Fatalf("User B registration failed: status %d, err %v", respB.StatusCode, err)
	}
	var authB models.AuthResponse
	parseResponse(respB, &authB)

	// Verify persistence in MSSQL Users table
	var countA, countB int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM Users WHERE email = @p1", emailA).Scan(&countA)
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM Users WHERE email = @p1", emailB).Scan(&countB)
	if countA != 1 || countB != 1 {
		t.Fatalf("Expected User A and User B in MSSQL Users table, got %d and %d", countA, countB)
	}

	// 2. User A creates private project
	createProjPayload := `{"name":"Alpha Confidential Map","description":"Private GIS Project"}`
	reqProj, _ := http.NewRequest("POST", server.URL+"/api/projects", strings.NewReader(createProjPayload))
	reqProj.Header.Set("Authorization", "Bearer "+authA.Token)
	reqProj.Header.Set("Content-Type", "application/json")
	respProj, err := client.Do(reqProj)
	if err != nil || respProj.StatusCode != http.StatusCreated {
		t.Fatalf("User A create project failed: status %d", respProj.StatusCode)
	}
	var projA models.UserProject
	parseResponse(respProj, &projA)

	// Verify project in MSSQL Projects table
	var projCount int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM Projects WHERE id = @p1", projA.ID).Scan(&projCount)
	if projCount != 1 {
		t.Fatalf("Expected project in MSSQL Projects table, got count %d", projCount)
	}

	// 3. User B cannot access User A's private project (Isolation check)
	reqBGet, _ := http.NewRequest("GET", server.URL+"/api/projects/"+projA.ID, nil)
	reqBGet.Header.Set("Authorization", "Bearer "+authB.Token)
	respBGet, err := client.Do(reqBGet)
	if err != nil || (respBGet.StatusCode != http.StatusForbidden && respBGet.StatusCode != http.StatusNotFound) {
		t.Fatalf("Expected 403 Forbidden or 404 for User B, got %d", respBGet.StatusCode)
	}

	// 4. User A creates an Organization
	createOrgPayload := `{"name":"Alpha Mapping Corp","description":"Primary Geo Org"}`
	reqOrg, _ := http.NewRequest("POST", server.URL+"/api/organizations", strings.NewReader(createOrgPayload))
	reqOrg.Header.Set("Authorization", "Bearer "+authA.Token)
	reqOrg.Header.Set("Content-Type", "application/json")
	respOrg, err := client.Do(reqOrg)
	if err != nil || respOrg.StatusCode != http.StatusCreated {
		t.Fatalf("User A create organization failed: status %d", respOrg.StatusCode)
	}
	var orgA models.Organization
	parseResponse(respOrg, &orgA)

	// Verify org in MSSQL Organizations table
	var orgCount int
	_ = sqlDB.QueryRow("SELECT COUNT(*) FROM Organizations WHERE id = @p1", orgA.ID).Scan(&orgCount)
	if orgCount != 1 {
		t.Fatalf("Expected organization in MSSQL Organizations table, got count %d", orgCount)
	}

	// 5. User A invites User B to Organization
	inviteOrgPayload := `{"email":"` + emailB + `","role":"editor"}`
	reqInvOrg, _ := http.NewRequest("POST", server.URL+"/api/organizations/"+orgA.ID+"/members", strings.NewReader(inviteOrgPayload))
	reqInvOrg.Header.Set("Authorization", "Bearer "+authA.Token)
	reqInvOrg.Header.Set("Content-Type", "application/json")
	respInvOrg, err := client.Do(reqInvOrg)
	if err != nil || respInvOrg.StatusCode != http.StatusCreated {
		t.Fatalf("Invite User B to org failed: status %d", respInvOrg.StatusCode)
	}
	var orgMemB models.OrgMember
	parseResponse(respInvOrg, &orgMemB)

	// 6. User B responds to invitation via unified /api/invitations/:id/accept
	reqAccept, _ := http.NewRequest("POST", server.URL+"/api/invitations/"+orgMemB.ID+"/accept", nil)
	reqAccept.Header.Set("Authorization", "Bearer "+authB.Token)
	respAccept, err := client.Do(reqAccept)
	if err != nil || respAccept.StatusCode != http.StatusOK {
		t.Fatalf("Accept invitation via unified endpoint failed: status %d", respAccept.StatusCode)
	}

	// Verify accepted membership in MSSQL
	var statusStr string
	_ = sqlDB.QueryRow("SELECT invitation_status FROM OrganizationMembers WHERE id = @p1", orgMemB.ID).Scan(&statusStr)
	if statusStr != string(models.StatusAccepted) {
		t.Fatalf("Expected accepted status in MSSQL, got %s", statusStr)
	}

	// 7. User A shares project with User B as Editor
	sharePayload := `{"email":"` + emailB + `","role":"editor"}`
	reqShare, _ := http.NewRequest("POST", server.URL+"/api/projects/"+projA.ID+"/share", strings.NewReader(sharePayload))
	reqShare.Header.Set("Authorization", "Bearer "+authA.Token)
	reqShare.Header.Set("Content-Type", "application/json")
	respShare, err := client.Do(reqShare)
	if err != nil || respShare.StatusCode != http.StatusCreated {
		t.Fatalf("User A share project failed: status %d", respShare.StatusCode)
	}
	var projMemB models.ProjectMember
	parseResponse(respShare, &projMemB)

	// User B accepts project invitation
	reqAcceptProj, _ := http.NewRequest("POST", server.URL+"/api/invitations/"+projMemB.ID+"/accept", nil)
	reqAcceptProj.Header.Set("Authorization", "Bearer "+authB.Token)
	respAcceptProj, err := client.Do(reqAcceptProj)
	if err != nil || respAcceptProj.StatusCode != http.StatusOK {
		t.Fatalf("User B accept project invitation failed: status %d", respAcceptProj.StatusCode)
	}

	// User B can now read and update the project
	updatePayload := `{"name":"Alpha Shared Map - Updated by Beta"}`
	reqUpdate, _ := http.NewRequest("PATCH", server.URL+"/api/projects/"+projA.ID, strings.NewReader(updatePayload))
	reqUpdate.Header.Set("Authorization", "Bearer "+authB.Token)
	reqUpdate.Header.Set("Content-Type", "application/json")
	respUpdate, err := client.Do(reqUpdate)
	if err != nil || respUpdate.StatusCode != http.StatusOK {
		t.Fatalf("User B update as Editor failed: status %d", respUpdate.StatusCode)
	}

	// 8. User A demotes User B to Viewer
	demotePayload := `{"role":"viewer"}`
	reqDemote, _ := http.NewRequest("PATCH", server.URL+"/api/projects/"+projA.ID+"/members/"+projMemB.ID, strings.NewReader(demotePayload))
	reqDemote.Header.Set("Authorization", "Bearer "+authA.Token)
	reqDemote.Header.Set("Content-Type", "application/json")
	respDemote, err := client.Do(reqDemote)
	if err != nil || respDemote.StatusCode != http.StatusOK {
		t.Fatalf("Demote User B to viewer failed: status %d", respDemote.StatusCode)
	}

	// User B can no longer update as Viewer (Forbidden)
	reqUpdate2, _ := http.NewRequest("PATCH", server.URL+"/api/projects/"+projA.ID, strings.NewReader(updatePayload))
	reqUpdate2.Header.Set("Authorization", "Bearer "+authB.Token)
	reqUpdate2.Header.Set("Content-Type", "application/json")
	respUpdateViewer, err := client.Do(reqUpdate2)
	if err != nil || respUpdateViewer.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when Viewer attempts to modify project, got %d", respUpdateViewer.StatusCode)
	}

	// 9. User A removes User B from project
	reqRemove, _ := http.NewRequest("DELETE", server.URL+"/api/projects/"+projA.ID+"/members/"+projMemB.ID, nil)
	reqRemove.Header.Set("Authorization", "Bearer "+authA.Token)
	respRemove, err := client.Do(reqRemove)
	if err != nil || respRemove.StatusCode != http.StatusOK {
		t.Fatalf("Remove User B from project failed: status %d", respRemove.StatusCode)
	}

	// User B can no longer access the project
	reqBGet2, _ := http.NewRequest("GET", server.URL+"/api/projects/"+projA.ID, nil)
	reqBGet2.Header.Set("Authorization", "Bearer "+authB.Token)
	respBAfterRemove, err := client.Do(reqBGet2)
	if err != nil || (respBAfterRemove.StatusCode != http.StatusForbidden && respBAfterRemove.StatusCode != http.StatusNotFound) {
		t.Fatalf("Expected 403 or 404 after removal, got %d", respBAfterRemove.StatusCode)
	}
}
