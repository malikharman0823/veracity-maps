package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func testConfig() *config.Config {
	return &config.Config{
		Environment:        "test",
		WorkspaceTTL:       24 * time.Hour,
		SessionIdleTimeout: 1 * time.Hour,
		SessionMaxLifetime: 24 * time.Hour,
	}
}

func setupOTPOfflineRouter(t *testing.T, mailer services.Mailer) (*api.Router, *fakeMailer) {
	t.Helper()
	tempDir := t.TempDir()

	authRepo, err := repository.NewAuthRepository(tempDir)
	if err != nil {
		t.Fatalf("failed creating auth repo: %v", err)
	}
	otpRepo := repository.NewMemoryOTPRepository()
	sessionRepo := repository.NewSessionRepository()
	sessionRepo.SetStorageDir(tempDir)
	projRepo, err := repository.NewProjectRepository(tempDir)
	if err != nil {
		t.Fatalf("failed creating proj repo: %v", err)
	}

	sm, err := storage.NewStorageManager(tempDir+"/sessions", tempDir+"/uploads", tempDir+"/processed")
	if err != nil {
		t.Fatalf("failed creating storage manager: %v", err)
	}

	cfg := testConfig()
	datasetService := services.NewDatasetService(cfg, sessionRepo, sm)
	georefService := services.NewGeoreferenceService(sessionRepo, sm)
	exportService := services.NewExportService(sessionRepo, sm)

	uploadH := upload.NewHandler(datasetService, sessionRepo, sm)
	datasetsH := datasets.NewHandler(datasetService, sessionRepo)
	georefH := georeference.NewHandler(georefService, sessionRepo)
	exportH := export.NewHandler(exportService, sessionRepo)

	var fm *fakeMailer
	if mailer == nil {
		fm = &fakeMailer{}
		mailer = fm
	}

	authH := auth.NewHandlerWithOTP(authRepo, otpRepo, nil, mailer, "http://localhost:5173")
	projectsH := projects.NewHandler(projRepo, sessionRepo, authRepo, mailer, "http://localhost:5173")

	mw := api.NewMiddleware(sessionRepo, authRepo)

	router := api.NewRouter(
		sessionRepo,
		authRepo,
		projRepo,
		mw,
		uploadH,
		datasetsH,
		georefH,
		exportH,
		authH,
		projectsH,
	)

	return router, fm
}

func TestOTPRegistrationAndLoginFlow(t *testing.T) {
	router, fm := setupOTPOfflineRouter(t, nil)
	handler := router.BuildHandler()

	email := "harman@example.com"
	name := "Harman Malik"

	// 1. Request Register OTP with empty name -> must fail 400
	{
		body, _ := json.Marshal(models.SendOTPRequest{
			Email:   email,
			Name:    "",
			Purpose: "register",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/send", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty name, got %d", w.Code)
		}
	}

	// 2. Request Register OTP with valid details -> succeeds 200
	{
		body, _ := json.Marshal(models.SendOTPRequest{
			Email:   email,
			Name:    name,
			Purpose: "register",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/send", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for valid register OTP request, got %d: %s", w.Code, w.Body.String())
		}
		if len(fm.otps) != 1 {
			t.Fatalf("expected 1 sent OTP email, got %d", len(fm.otps))
		}
		if fm.otps[0].To != email || len(fm.otps[0].Code) != 6 {
			t.Fatalf("invalid OTP email payload: %+v", fm.otps[0])
		}
	}

	sentCode := fm.otps[0].Code

	// 3. Request Register OTP again immediately -> must be 429 rate limited
	{
		body, _ := json.Marshal(models.SendOTPRequest{
			Email:   email,
			Name:    name,
			Purpose: "register",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/send", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 rate limited on immediate resend, got %d", w.Code)
		}
	}

	// 4. Verify OTP with wrong code -> must fail 400
	{
		body, _ := json.Marshal(models.VerifyOTPRequest{
			Email:   email,
			Code:    "123450",
			Name:    name,
			Purpose: "register",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/verify", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for wrong code, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 5. Verify OTP with correct code -> succeeds 200 and registers user
	var authToken string
	{
		body, _ := json.Marshal(models.VerifyOTPRequest{
			Email:   email,
			Code:    sentCode,
			Name:    name,
			Purpose: "register",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/verify", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on verify correct code, got %d: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Success bool                `json:"success"`
			Data    models.AuthResponse `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed decoding auth response: %v", err)
		}
		if resp.Data.Token == "" {
			t.Fatal("expected non-empty auth token in response")
		}
		if resp.Data.User.Email != email || resp.Data.User.FullName != name {
			t.Fatalf("unexpected user in response: %+v", resp.Data.User)
		}
		authToken = resp.Data.Token
	}

	// 6. Test authenticated endpoint with new token -> /api/auth/me returns 200
	{
		req := httptest.NewRequest("GET", "/api/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+authToken)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on /api/auth/me with OTP token, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 7. Request register OTP again for already registered user -> returns 409 Conflict
	{
		body, _ := json.Marshal(models.SendOTPRequest{
			Email:   email,
			Name:    name,
			Purpose: "register",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/send", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 conflict for existing registered user, got %d", w.Code)
		}
	}

	// 8. Request Login OTP for unknown user -> returns 404
	{
		body, _ := json.Marshal(models.SendOTPRequest{
			Email:   "nonexistent@example.com",
			Purpose: "login",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/send", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown user login, got %d", w.Code)
		}
	}

	// 9. Request Login OTP for existing user -> returns 200
	var loginCode string
	{
		body, _ := json.Marshal(models.SendOTPRequest{
			Email:   email,
			Purpose: "login",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/send", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for existing user login OTP, got %d: %s", w.Code, w.Body.String())
		}
		if len(fm.otps) != 2 {
			t.Fatalf("expected 2 sent OTP emails, got %d", len(fm.otps))
		}
		loginCode = fm.otps[1].Code
	}

	// 10. Verify Login OTP with correct code -> returns 200 and valid session token
	{
		body, _ := json.Marshal(models.VerifyOTPRequest{
			Email:   email,
			Code:    loginCode,
			Purpose: "login",
		})
		req := httptest.NewRequest("POST", "/api/auth/otp/verify", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 on login OTP verification, got %d: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Data models.AuthResponse `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Data.Token == "" || resp.Data.User.Email != email {
			t.Fatalf("invalid login auth response: %+v", resp)
		}
	}
}

func TestMemoryOTPRepositoryRateLimitingAndMaxAttempts(t *testing.T) {
	repo := repository.NewMemoryOTPRepository()
	ctx := context.Background()
	email := "test@example.com"
	purpose := "login"

	// Create OTP with 2 second cooldown
	_, err := repo.CreateOTP(ctx, email, "Test", purpose, "123456", 1*time.Minute, 2*time.Second)
	if err != nil {
		t.Fatalf("failed creating OTP: %v", err)
	}

	// Immediate resend -> should be rate limited
	_, err = repo.CreateOTP(ctx, email, "Test", purpose, "654321", 1*time.Minute, 2*time.Second)
	if err != repository.ErrOTPRateLimited {
		t.Fatalf("expected ErrOTPRateLimited, got %v", err)
	}

	// Try wrong code 5 times
	for i := 1; i <= 5; i++ {
		_, err = repo.VerifyAndConsumeOTP(ctx, email, "000000", purpose)
		if err != repository.ErrOTPInvalid {
			t.Fatalf("attempt %d: expected ErrOTPInvalid, got %v", i, err)
		}
	}

	// 6th attempt should exceed max attempts
	_, err = repo.VerifyAndConsumeOTP(ctx, email, "000000", purpose)
	if err != repository.ErrOTPMaxAttempts {
		t.Fatalf("expected ErrOTPMaxAttempts, got %v", err)
	}
}

func TestMSSQLOTPRepositoryFlow(t *testing.T) {
	connStr := os.Getenv("MSSQL_TEST_CONNECTION_STRING")
	if connStr == "" {
		t.Skip("MSSQL_TEST_CONNECTION_STRING is not configured for live SQL tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sqlDB, err := db.InitDB(ctx, connStr)
	if err != nil {
		t.Skip("local MSSQL not accessible for live integration test")
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skip("local MSSQL not responding to ping")
	}

	repo := repository.NewMSSQLOTPRepository(sqlDB)
	testEmail := "mssql_otp_test_" + time.Now().Format("20060102150405") + "@example.com"
	purpose := "register"
	code := "543210"

	// 1. Create OTP
	rec, err := repo.CreateOTP(ctx, testEmail, "MSSQL Tester", purpose, code, 5*time.Minute, 1*time.Second)
	if err != nil {
		t.Fatalf("failed creating MSSQL OTP: %v", err)
	}
	if rec.Email != testEmail || rec.Purpose != purpose {
		t.Fatalf("unexpected record returned: %+v", rec)
	}

	// 2. Verify with wrong code
	_, err = repo.VerifyAndConsumeOTP(ctx, testEmail, "999999", purpose)
	if err != repository.ErrOTPInvalid {
		t.Fatalf("expected ErrOTPInvalid, got %v", err)
	}

	// 3. Verify with correct code
	consumed, err := repo.VerifyAndConsumeOTP(ctx, testEmail, code, purpose)
	if err != nil {
		t.Fatalf("failed verifying correct code in MSSQL: %v", err)
	}
	if consumed.ConsumedAt == nil {
		t.Fatal("expected ConsumedAt timestamp to be set")
	}

	// 4. Try verifying again -> must fail because already consumed
	_, err = repo.VerifyAndConsumeOTP(ctx, testEmail, code, purpose)
	if err != repository.ErrOTPExpiredOrNotFound {
		t.Fatalf("expected ErrOTPExpiredOrNotFound for already consumed OTP, got %v", err)
	}
}
