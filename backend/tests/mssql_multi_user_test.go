package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/db"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/datasets"
	"github.com/malikharman0823/veracity-maps/functions/export"
	"github.com/malikharman0823/veracity-maps/functions/georeference"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func openTestMSSQL(t *testing.T) *sql.DB {
	t.Helper()
	connStr := os.Getenv("MSSQL_TEST_CONNECTION_STRING")
	if connStr == "" {
		t.Skip("MSSQL_TEST_CONNECTION_STRING is not set; a dedicated SQL Server test database is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sqlDB, err := db.InitDB(ctx, connStr)
	if err != nil {
		t.Fatal("cannot connect to the configured MSSQL test database (connection details withheld)")
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func getTestMSSQLDB(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB := openTestMSSQL(t)
	migrationsDir := filepath.Join("..", "migrations")
	if err := db.RunMigrations(context.Background(), sqlDB, migrationsDir); err != nil {
		t.Fatalf("Failed running migrations on test MSSQL database: %v", err)
	}

	return sqlDB
}

func TestProductionConfigRequiresMSSQL(t *testing.T) {
	cfg := &config.Config{Environment: "production"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "MSSQL_CONNECTION_STRING") {
		t.Fatalf("expected missing MSSQL connection error, got %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name, environment, connection string
		ttl                           time.Duration
		wantError                     string
	}{
		{"development requires database", "development", " ", time.Hour, "MSSQL_CONNECTION_STRING"},
		{"isolated tests allow no database", "test", "", time.Hour, ""},
		{"zero TTL rejected", "test", "", 0, "WORKSPACE_TTL"},
		{"negative TTL rejected", "production", "configured", -time.Hour, "WORKSPACE_TTL"},
		{"production configured", "production", "configured", time.Hour, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Environment: tc.environment, MSSQLConnStr: tc.connection, WorkspaceTTL: tc.ttl,
				ResendAPIKey: "test-key", ResendFrom: "maps@example.com", AppPublicURL: "https://maps.example.com"}
			err := cfg.Validate()
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected %s validation error, got %v", tc.wantError, err)
			}
		})
	}
}

func TestLoadConfigWorkspaceTTL(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("WORKSPACE_TTL", "90m")
	if cfg := config.LoadConfig(); cfg.WorkspaceTTL != 90*time.Minute || cfg.Environment != "test" {
		t.Fatal("environment and workspace duration were not loaded")
	}
	t.Setenv("WORKSPACE_TTL", "invalid")
	if err := config.LoadConfig().Validate(); err == nil || !strings.Contains(err.Error(), "WORKSPACE_TTL") {
		t.Fatalf("expected malformed duration to fail validation, got %v", err)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	database := openTestMSSQL(t)
	ctx := context.Background()
	if err := db.RunMigrations(ctx, database, "../migrations"); err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	var before int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM SchemaMigrations").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(ctx, database, "../migrations"); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
	var after int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM SchemaMigrations").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before < 2 || after != before {
		t.Fatalf("migration ledger changed on rerun: %d -> %d", before, after)
	}
}

func TestDatabaseInitializationHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if database, err := db.InitDB(ctx, "server=127.0.0.1;database=unused;trusted_connection=yes"); err == nil {
		_ = database.Close()
		t.Fatal("cancelled initialization unexpectedly connected")
	}
	if _, err := db.InitDB(context.Background(), " "); err == nil {
		t.Fatal("empty connection string accepted")
	}
	if err := db.RunMigrations(context.Background(), nil, t.TempDir()); err == nil {
		t.Fatal("nil database accepted")
	}
}

func TestUserOwnedSchemaIsAdditive(t *testing.T) {
	database := getTestMSSQLDB(t)
	for _, column := range []struct{ table, name string }{
		{"Users", "profile_picture_path"}, {"Users", "email_verified"}, {"Users", "pending_email"}, {"Users", "account_status"},
		{"Projects", "owner_user_id"}, {"Projects", "org_id"}, {"Projects", "owner_type"},
		{"ProjectMembers", "permissions"}, {"ProjectMembers", "joined_at"},
		{"AuthTokens", "id"}, {"AuthTokens", "token_hash"}, {"AuthTokens", "revoked_at"},
		{"ProjectInvitations", "invitee_key"}, {"ProjectInvitations", "token_hash"}, {"ProjectInvitations", "delivery_status"},
		{"Maps", "map_configuration"}, {"Uploads", "storage_path"}, {"Activities", "description"},
		{"WorkspaceDrafts", "expires_at"}, {"UserVerificationTokens", "token_hash"},
	} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID(@p1) AND name = @p2", column.table, column.name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("missing %s.%s", column.table, column.name)
		}
	}
	for _, table := range []string{"Organizations", "OrganizationMembers", "OrganizationInvitations", "OrganizationActivities"} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM sys.tables WHERE name = @p1", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("compatibility table %s removed", table)
		}
	}
}

func TestMigrationsRollbackFailedFile(t *testing.T) {
	database := getTestMSSQLDB(t)
	dir := t.TempDir()
	version := fmt.Sprintf("999_test_rollback_%d.sql", time.Now().UnixNano())
	table := fmt.Sprintf("MigrationRollback%d", time.Now().UnixNano())
	script := "CREATE TABLE " + table + " (id INT NOT NULL);\r\ngo -- batch boundary\r\nTHROW 50000, 'intentional migration failure', 1;"
	if err := os.WriteFile(filepath.Join(dir, version), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(context.Background(), database, dir); err == nil {
		t.Fatal("expected migration failure")
	}
	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM SchemaMigrations WHERE version = @p1", version).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration was recorded")
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM sys.tables WHERE name = @p1", table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration left its table behind")
	}
	if err := os.WriteFile(filepath.Join(dir, version), []byte("CREATE TABLE "+table+" (id INT NOT NULL);\nGO\nINSERT INTO "+table+" VALUES (1);"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec("DROP TABLE IF EXISTS " + table)
		_, _ = database.Exec("DELETE FROM SchemaMigrations WHERE version = @p1", version)
	})
	if err := db.RunMigrations(context.Background(), database, dir); err != nil {
		t.Fatalf("retry corrected migration: %v", err)
	}
	if err := db.RunMigrations(context.Background(), database, dir); err != nil {
		t.Fatalf("rerun corrected migration: %v", err)
	}
	if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration ran more than once: %d rows", count)
	}
}

func setupMSSQLServer(t *testing.T, sqlDB *sql.DB) (*httptest.Server, *repository.AuthRepository, *repository.ProjectRepository) {
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

	middleware := api.NewMiddleware(sessionRepo, authRepo)
	datasetService := services.NewDatasetService(cfg, sessionRepo, sm)
	georefService := services.NewGeoreferenceService(sessionRepo, sm)
	exportService := services.NewExportService(sessionRepo, sm)

	uploadHandler := upload.NewHandler(datasetService, sessionRepo, sm)
	datasetsHandler := datasets.NewHandler(datasetService, sessionRepo)
	georefHandler := georeference.NewHandler(georefService, sessionRepo)
	exportHandler := export.NewHandler(exportService, sessionRepo)
	authHandler := auth.NewHandler(authRepo)
	projectsHandler := projects.NewHandler(projRepo, sessionRepo, authRepo, nil, "")

	router := api.NewRouter(
		sessionRepo,
		authRepo,
		projRepo,
		middleware,
		uploadHandler,
		datasetsHandler,
		georefHandler,
		exportHandler,
		authHandler,
		projectsHandler,
	)

	server := httptest.NewServer(router.BuildHandler())
	return server, authRepo, projRepo
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

	server, _, _ := setupMSSQLServer(t, sqlDB)
	defer server.Close()

	client := server.Client()

	// 1. User A & User B Register
	ts := time.Now().UnixNano()
	emailA := fmt.Sprintf("usera_%s_%d@veracity.com", strings.ToLower(t.Name()), ts)
	emailB := fmt.Sprintf("userb_%s_%d@veracity.com", strings.ToLower(t.Name()), ts)

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

	// 4. User A shares project with User B as Editor directly
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

	// User B accepts project invitation via unified /api/invitations/:id/accept
	reqAcceptProj, _ := http.NewRequest("POST", server.URL+"/api/invitations/"+projMemB.ID+"/accept", nil)
	reqAcceptProj.Header.Set("Authorization", "Bearer "+authB.Token)
	respAcceptProj, err := client.Do(reqAcceptProj)
	if err != nil || respAcceptProj.StatusCode != http.StatusOK {
		t.Fatalf("User B accept project invitation failed: status %d", respAcceptProj.StatusCode)
	}

	// Verify accepted membership in MSSQL
	var statusStr string
	_ = sqlDB.QueryRow("SELECT invitation_status FROM ProjectMembers WHERE id = @p1", projMemB.ID).Scan(&statusStr)
	if statusStr != string(models.StatusAccepted) {
		t.Fatalf("Expected accepted status in MSSQL ProjectMembers, got %s", statusStr)
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
