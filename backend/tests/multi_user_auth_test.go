package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
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

func TestPasswordHashUsesBcryptAndRejectsOversizeInput(t *testing.T) {
	hash, err := models.HashPassword("Correct Horse Battery Staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("expected bcrypt hash, got %q", hash)
	}
	if cost, err := bcrypt.Cost([]byte(hash)); err != nil || cost != 12 {
		t.Fatalf("bcrypt cost: %d, %v", cost, err)
	}
	for _, password := range []string{strings.Repeat("x", 73), strings.Repeat("é", 37)} {
		if _, err := models.HashPassword(password); err == nil {
			t.Fatal("expected bcrypt input limit error")
		}
	}
	if _, err := models.HashPassword(strings.Repeat("x", 72)); err != nil {
		t.Fatalf("72 bytes must be accepted: %v", err)
	}
	for _, tc := range []struct {
		password, encoded string
		want, wantErr     bool
	}{
		{"Correct Horse Battery Staple", hash, true, false},
		{"wrong password", hash, false, false},
		{"password", "broken hash", false, true},
	} {
		ok, err := models.VerifyPassword(tc.password, tc.encoded)
		if ok != tc.want || (err != nil) != tc.wantErr {
			t.Fatalf("verify: got %v, %v", ok, err)
		}
	}
}

func TestAuthTokenDatabaseValueIsHashed(t *testing.T) {
	raw := "vmap_example-token"
	if models.HashToken(raw) == raw || len(models.HashToken(raw)) != 64 {
		t.Fatal("token must be stored as a SHA-256 hex lookup hash")
	}
	if got := models.HashToken("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("unexpected SHA-256 hash: %s", got)
	}
}

func TestAuthenticationResponsesAndLogout(t *testing.T) {
	server, _, _ := setupMultiUserTestServer(t, t.TempDir())
	defer server.Close()
	var token string
	for _, path := range []string{"register", "login", "me", "logout"} {
		method, body := http.MethodPost, `{"name":"Auth User","email":"auth@example.com","password":"Password123!"}`
		if path == "me" {
			method, body = http.MethodGet, ""
		}
		req, _ := http.NewRequest(method, server.URL+"/api/auth/"+path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"password_hash", "salt", "token_hash", "revoked_at"} {
			if bytes.Contains(data, []byte(`"`+secret+`"`)) {
				t.Fatalf("%s leaked %s", path, secret)
			}
		}
		want := http.StatusOK
		if path == "register" {
			want = http.StatusCreated
		}
		if resp.StatusCode != want {
			t.Fatalf("%s: want %d got %d: %s", path, want, resp.StatusCode, data)
		}
		if path == "register" || path == "login" {
			var envelope struct {
				Data models.AuthResponse `json:"data"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil {
				t.Fatal(err)
			}
			token = envelope.Data.Token
			if !strings.HasPrefix(token, "vmap_") {
				t.Fatal("missing raw token")
			}
		}
		if path == "logout" {
			req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/auth/me", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			check, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			check.Body.Close()
			if check.StatusCode != http.StatusUnauthorized {
				t.Fatalf("revoked token accepted: %d", check.StatusCode)
			}
		}
	}
}

func TestAuthenticationRejectsOversizePassword(t *testing.T) {
	repo, err := repository.NewAuthRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(models.RegisterRequest{Name: "Oversize", Email: "oversize@example.com", Password: strings.Repeat("x", 73)})
	recorder := httptest.NewRecorder()
	auth.NewHandler(repo).Register(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("oversize password: %d", recorder.Code)
	}
	if _, err := repo.FindUserByEmail(context.Background(), "oversize@example.com"); !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("oversize registration created a user: %v", err)
	}
}

func TestAuthenticationTokenCompatibilityStorePersistsHashes(t *testing.T) {
	dir, ctx := t.TempDir(), context.Background()
	repo, err := repository.NewAuthRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := repo.Register(ctx, "Token User", "token@example.com", "Password123!")
	if err != nil {
		t.Fatal(err)
	}
	token, err := repo.CreateToken(ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "users", "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(token.Token)) || !bytes.Contains(data, []byte(models.HashToken(token.Token))) {
		t.Fatal("compatibility store persisted a raw token")
	}
	restarted, err := repository.NewAuthRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ValidateToken(ctx, token.Token); err != nil {
		t.Fatalf("token not persisted: %v", err)
	}
	if err := restarted.RevokeToken(ctx, token.Token); err != nil {
		t.Fatal(err)
	}
	restarted, err = repository.NewAuthRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ValidateToken(ctx, token.Token); !errors.Is(err, repository.ErrInvalidToken) {
		t.Fatalf("revocation not persisted: %v", err)
	}
}

type logoutFailureRepository struct{ repository.AuthRepo }

func (logoutFailureRepository) RevokeToken(ctx context.Context, token string) error {
	return errors.New("test database unavailable")
}

func TestLogoutReportsRevocationFailure(t *testing.T) {
	response := httptest.NewRecorder()
	auth.NewHandler(logoutFailureRepository{}).Logout(response, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil), "token")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failed revocation reported success: %d", response.Code)
	}
}

func TestAuthTokenCutoverInvalidatesLegacyRows(t *testing.T) {
	database := getTestMSSQLDB(t)
	ctx := context.Background()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Reconstruct the pre-cutover auth table inside a transaction; rollback restores
	// the migrated test database, even when this test fails.
	_, err = tx.ExecContext(ctx, `DROP TABLE dbo.AuthTokens;
		CREATE TABLE dbo.AuthTokens (token NVARCHAR(128) NOT NULL PRIMARY KEY,
		user_id NVARCHAR(64) NOT NULL, expires_at DATETIMEOFFSET NOT NULL,
		created_at DATETIMEOFFSET NOT NULL, id NVARCHAR(64) NULL,
		token_hash CHAR(64) NULL, revoked_at DATETIMEOFFSET NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dbo.AuthTokens (token, user_id, expires_at, created_at)
		VALUES ('legacy-secret', 'legacy-user', SYSDATETIMEOFFSET(), SYSDATETIMEOFFSET());`); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join("..", "migrations", "003_hashed_auth_tokens.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, string(script)); err != nil {
		t.Fatal(err)
	}
	var rows, rawColumn, requiredHash, primaryID int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM dbo.AuthTokens").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('dbo.AuthTokens') AND name = 'token'").Scan(&rawColumn); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys.columns WHERE object_id = OBJECT_ID('dbo.AuthTokens') AND name = 'token_hash' AND is_nullable = 0").Scan(&requiredHash); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys.indexes i JOIN sys.index_columns ic ON ic.object_id=i.object_id AND ic.index_id=i.index_id JOIN sys.columns c ON c.object_id=ic.object_id AND c.column_id=ic.column_id WHERE i.object_id=OBJECT_ID('dbo.AuthTokens') AND i.is_primary_key=1 AND c.name='id'`).Scan(&primaryID); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || rawColumn != 0 || requiredHash != 1 || primaryID != 1 {
		t.Fatalf("cutover: rows=%d raw=%d required hash=%d primary id=%d", rows, rawColumn, requiredHash, primaryID)
	}
}

func TestMSSQLAuthenticationHashesTokensAndRehashesLegacyPassword(t *testing.T) {
	database := getTestMSSQLDB(t)
	repo := repository.NewMSSQLAuthRepository(database)
	ctx := context.Background()
	suffix, _ := models.GenerateRandomHex(8)
	user, err := repo.Register(ctx, "Secure User", suffix+"@example.com", "Password123!")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.Exec("DELETE FROM Users WHERE id = @id", sql.Named("id", user.ID)) })
	if user.Salt != "" || !strings.HasPrefix(user.PasswordHash, "$2") {
		t.Fatal("new password is not bcrypt")
	}
	for _, duration := range []time.Duration{time.Hour, -time.Hour} {
		token, err := repo.CreateToken(ctx, user.ID, duration)
		if err != nil {
			t.Fatal(err)
		}
		var stored string
		if err := database.QueryRow("SELECT token_hash FROM AuthTokens WHERE user_id = @id AND token_hash = @hash", sql.Named("id", user.ID), sql.Named("hash", models.HashToken(token.Token))).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored == token.Token || len(stored) != 64 {
			t.Fatal("stored bearer token is not hashed")
		}
		_, err = repo.ValidateToken(ctx, token.Token)
		if duration < 0 {
			if !errors.Is(err, repository.ErrInvalidToken) {
				t.Fatalf("expired token: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ValidateToken(ctx, stored); !errors.Is(err, repository.ErrInvalidToken) {
			t.Fatalf("hash accepted as raw token: %v", err)
		}
		if err := repo.RevokeToken(ctx, token.Token); err != nil {
			t.Fatal(err)
		}
		var revoked sql.NullTime
		if err := database.QueryRow("SELECT revoked_at FROM AuthTokens WHERE token_hash = @hash", sql.Named("hash", stored)).Scan(&revoked); err != nil || !revoked.Valid {
			t.Fatalf("revocation not persisted: %v", err)
		}
		if _, err := repo.ValidateToken(ctx, token.Token); !errors.Is(err, repository.ErrInvalidToken) {
			t.Fatalf("revoked token accepted: %v", err)
		}
	}
	// Independently generated fixture from the previous salted SHA-256 scheme.
	const legacyHash = "d5cd010d12c98482b66f2aff25435764da31fe652d4f0698f0ec9df58c0d97a6"
	if _, err := database.Exec("UPDATE Users SET password_hash = @hash, salt = 'legacy-salt' WHERE id = @id", sql.Named("hash", legacyHash), sql.Named("id", user.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Authenticate(ctx, user.Email, "wrong"); !errors.Is(err, repository.ErrInvalidPassword) {
		t.Fatalf("wrong legacy password: %v", err)
	}
	if _, err := repo.Authenticate(ctx, user.Email, "Password123!"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Salt != "" || !strings.HasPrefix(reloaded.PasswordHash, "$2") {
		t.Fatal("legacy password not upgraded")
	}
	if _, err := repo.Authenticate(ctx, user.Email, "Password123!"); err != nil {
		t.Fatalf("upgraded login: %v", err)
	}
}

func TestMSSQLAuthenticationHonorsCancelledContext(t *testing.T) {
	database := getTestMSSQLDB(t)
	repo := repository.NewMSSQLAuthRepository(database)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checks := []func() error{
		func() error { _, err := repo.Register(ctx, "Name", "cancel@example.com", "Password123!"); return err },
		func() error { _, err := repo.Authenticate(ctx, "cancel@example.com", "Password123!"); return err },
		func() error { _, err := repo.CreateToken(ctx, "usr_none", time.Hour); return err },
		func() error { _, err := repo.ValidateToken(ctx, "none"); return err },
		func() error { return repo.RevokeToken(ctx, "none") },
		func() error { _, err := repo.GetUserByID(ctx, "none"); return err },
		func() error { _, err := repo.FindUserByEmail(ctx, "none"); return err },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, context.Canceled) {
			t.Fatalf("operation %d: %v", i, err)
		}
	}
}

// Exercise the real executable so production cannot silently switch back to
// filesystem authentication while repository-level SQL tests remain green.
func TestProductionAuthenticationSurvivesEmptyStorageRestart(t *testing.T) {
	database := getTestMSSQLDB(t)
	root := t.TempDir()
	binary := filepath.Join(root, "veracity-auth-test.exe")
	build := exec.Command("go", "build", "-o", binary, "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build production server: %v: %s", err, output)
	}

	start := func() (string, func()) {
		t.Helper()
		runtimeDir := t.TempDir()
		migrationsDir := filepath.Join(runtimeDir, "migrations")
		if err := os.Mkdir(migrationsDir, 0755); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Join("..", "migrations"))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".sql") {
				continue
			}
			data, err := os.ReadFile(filepath.Join("..", "migrations", entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(migrationsDir, entry.Name()), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		_, port, _ := net.SplitHostPort(address)
		listener.Close()
		command := exec.Command(binary)
		command.Dir = runtimeDir
		startupLog := filepath.Join(runtimeDir, "startup.log")
		logFile, err := os.Create(startupLog)
		if err != nil {
			t.Fatal(err)
		}
		command.Stdout, command.Stderr = logFile, logFile
		command.Env = append(os.Environ(), "APP_ENV=production", "HOST=127.0.0.1", "PORT="+port,
			"WORKSPACE_TTL=24h", "MSSQL_CONNECTION_STRING="+os.Getenv("MSSQL_TEST_CONNECTION_STRING"),
			"SESSION_STORAGE_DIR="+filepath.Join(runtimeDir, "sessions"),
			"UPLOAD_DIR="+filepath.Join(runtimeDir, "uploads"),
			"PROCESSED_DIR="+filepath.Join(runtimeDir, "processed"))
		if err := command.Start(); err != nil {
			t.Fatalf("start production server: %v", err)
		}
		stopped := false
		stop := func() {
			if !stopped {
				stopped = true
				_ = command.Process.Kill()
				_ = command.Wait()
				_ = logFile.Close()
			}
		}
		t.Cleanup(stop)
		url := "http://" + address
		client := &http.Client{Timeout: time.Second}
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			response, err := client.Get(url + "/health")
			if err == nil {
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return url, stop
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		stop()
		logData, _ := os.ReadFile(startupLog)
		t.Fatalf("production server did not become healthy: %s", logData)
		return "", stop
	}

	url, stop := start()
	suffix, err := models.GenerateRandomHex(8)
	if err != nil {
		t.Fatal(err)
	}
	email := suffix + "@startup.example.com"
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(url+"/api/auth/register", "application/json", strings.NewReader(`{"name":"Startup Auth","email":"`+email+`","password":"Password123!"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("production registration: %d", response.StatusCode)
	}
	var envelope struct {
		Data models.AuthResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	user := envelope.Data
	t.Cleanup(func() { _, _ = database.Exec("DELETE FROM Users WHERE id = @id", sql.Named("id", user.User.ID)) })
	stop()
	url, _ = start()
	request, _ := http.NewRequest(http.MethodGet, url+"/api/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+user.Token)
	me, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("production auth lost bearer token after empty-storage restart: %d", me.StatusCode)
	}
	login, err := client.Post(url+"/api/auth/login", "application/json", strings.NewReader(`{"email":"`+email+`","password":"Password123!"}`))
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("production auth lost user after empty-storage restart: %d", login.StatusCode)
	}
}

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

	router := api.NewRouter(sessionRepo, authRepo, projRepo, mw, uHandler, dHandler, gHandler, eHandler, aHandler, pHandler)
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
