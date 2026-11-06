package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/config"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/profile"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

func profileRequest(t *testing.T, handler http.Handler, method, path, token, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func profileHarness(t *testing.T) (http.Handler, *sql.DB, *repository.MSSQLAuthRepository, *models.User, string, *fakeMailer, string) {
	t.Helper()
	database := getTestMSSQLDB(t)
	auth := repository.NewMSSQLAuthRepository(database)
	user, err := auth.Register(context.Background(), "Ada Lovelace", fmt.Sprintf("profile_%d@example.com", time.Now().UnixNano()), "Current123!")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.Exec("DELETE FROM Users WHERE id = @id", sql.Named("id", user.ID)) })
	token, err := auth.CreateToken(context.Background(), user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mailer := &fakeMailer{}
	root := t.TempDir()
	ph := profile.NewHandler(repository.NewMSSQLProfileRepository(database), mailer, root, "https://maps.example.com")
	sessions := repository.NewSessionRepository()
	router := api.NewRouter(sessions, auth, nil, api.NewMiddleware(sessions, auth), nil, nil, nil, nil, authHandlerForProfile(auth, database, mailer), nil, ph)
	return router.BuildHandler(), database, auth, user, token.Token, mailer, root
}

func authHandlerForProfile(authRepo repository.AuthRepo, database *sql.DB, mailer *fakeMailer) *auth.Handler {
	return auth.NewHandlerWithVerification(authRepo, repository.NewMSSQLProfileRepository(database), mailer, "https://maps.example.com")
}

func registerProfileAccount(t *testing.T, h http.Handler, database *sql.DB) models.AuthResponse {
	t.Helper()
	email := fmt.Sprintf("initial_%d@example.com", time.Now().UnixNano())
	res := profileRequest(t, h, "POST", "/api/auth/register", "", `{"name":"Initial User","email":"`+email+`","password":"Current123!"}`)
	if res.Code != 201 {
		t.Fatalf("register for initial verification: %d %s", res.Code, res.Body)
	}
	var envelope struct {
		Data models.AuthResponse `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec("DELETE FROM Users WHERE id = @id", sql.Named("id", envelope.Data.User.ID))
	})
	return envelope.Data
}

func TestInitialVerificationRegistrationAndResend(t *testing.T) {
	h, database, _, _, _, mailer, _ := profileHarness(t)
	registered := registerProfileAccount(t, h, database)
	if registered.VerificationDelivery != "sent" || registered.User.EmailVerified || len(mailer.verifications) != 1 || mailer.verificationRecipients[0] != registered.User.Email {
		t.Fatal("initial verification not delivered safely")
	}
	firstURL, _ := url.Parse(mailer.verifications[0].Link)
	first := firstURL.Query().Get("token")
	var purpose, hash string
	if err := database.QueryRow("SELECT purpose, token_hash FROM UserVerificationTokens WHERE user_id = @id", sql.Named("id", registered.User.ID)).Scan(&purpose, &hash); err != nil {
		t.Fatal(err)
	}
	if purpose != "initial_verification" || hash != models.HashToken(first) {
		t.Fatal("initial verification stored incorrectly")
	}
	res := profileRequest(t, h, "POST", "/api/auth/resend-verification", registered.Token, `{}`)
	if res.Code != 429 || len(mailer.verifications) != 1 {
		t.Fatalf("resend was not rate limited: %d", res.Code)
	}
	if bypass := profileRequest(t, h, "POST", "/api/profile/email-change", registered.Token, `{"email":"`+registered.User.Email+`"}`); bypass.Code != 429 || len(mailer.verifications) != 1 {
		t.Fatal("email-change route bypassed verification rate limit")
	}
	if _, err := database.Exec("UPDATE UserVerificationTokens SET created_at = @past WHERE user_id = @id", sql.Named("past", time.Now().Add(-2*time.Minute)), sql.Named("id", registered.User.ID)); err != nil {
		t.Fatal(err)
	}
	res = profileRequest(t, h, "POST", "/api/auth/resend-verification", registered.Token, `{}`)
	if res.Code != 202 || len(mailer.verifications) != 2 {
		t.Fatalf("resend after window: %d", res.Code)
	}
	secondURL, _ := url.Parse(mailer.verifications[1].Link)
	second := secondURL.Query().Get("token")
	if second == "" || second == first || strings.Contains(res.Body.String(), second) {
		t.Fatal("resend token was not rotated privately")
	}
	if res := profileRequest(t, h, "POST", "/api/auth/verify-email", "", `{"token":"`+first+`"}`); res.Code != 400 {
		t.Fatal("superseded initial token accepted")
	}
	if res := profileRequest(t, h, "POST", "/api/auth/verify-email", "", `{"token":"`+second+`"}`); res.Code != 200 {
		t.Fatalf("initial verification rejected: %d %s", res.Code, res.Body)
	}
	if res := profileRequest(t, h, "POST", "/api/auth/verify-email", "", `{"token":"`+second+`"}`); res.Code != 400 {
		t.Fatal("initial token reused")
	}
	res = profileRequest(t, h, "POST", "/api/auth/resend-verification", registered.Token, `{}`)
	if res.Code != 200 || len(mailer.verifications) != 2 || !strings.Contains(res.Body.String(), `"email_verified":true`) {
		t.Fatal("already verified account received mail")
	}
	if res := profileRequest(t, h, "POST", "/api/auth/resend-verification", "", `{}`); res.Code != 401 {
		t.Fatal("resend allowed without authentication")
	}
}

func TestInitialVerificationDeliveryFailureRetainsAccount(t *testing.T) {
	h, database, authRepo, _, _, mailer, _ := profileHarness(t)
	mailer.err = errors.New("provider private-link-and-key")
	registered := registerProfileAccount(t, h, database)
	if registered.VerificationDelivery != "failed" || registered.VerificationCode != "EMAIL_DELIVERY_FAILED" || registered.User.EmailVerified {
		t.Fatal("unsafe registration delivery status")
	}
	if _, err := authRepo.Authenticate(context.Background(), registered.User.Email, "Current123!"); err != nil {
		t.Fatal("mail failure deleted the account")
	}
	if _, err := authRepo.ValidateToken(context.Background(), registered.Token); err != nil {
		t.Fatal("mail failure invalidated the registration auth token")
	}
	if strings.Contains(registered.VerificationCode, "private-link-and-key") {
		t.Fatal("private provider failure leaked")
	}
}

func TestProfileReadAndUpdate(t *testing.T) {
	h, _, _, user, token, _, _ := profileHarness(t)
	res := profileRequest(t, h, "GET", "/api/profile", token, "")
	if res.Code != 200 {
		t.Fatalf("get profile: %d %s", res.Code, res.Body)
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	p := envelope.Data
	if p["id"] != user.ID || p["full_name"] != "Ada Lovelace" || p["name"] != "Ada Lovelace" || p["email"] != user.Email || p["email_verified"] != false || p["account_status"] != "active" || p["created_at"] == nil || p["updated_at"] == nil {
		t.Fatalf("incomplete profile: %v", p)
	}
	for _, secret := range []string{"password_hash", "salt", "token_hash", "pending_email", "storage/users"} {
		if strings.Contains(res.Body.String(), secret) {
			t.Fatalf("profile leaked %s", secret)
		}
	}
	res = profileRequest(t, h, "PATCH", "/api/profile", token, `{"full_name":"  Ada Byron  "}`)
	if res.Code != 200 {
		t.Fatalf("patch profile: %d %s", res.Code, res.Body)
	}
	res = profileRequest(t, h, "GET", "/api/profile", token, "")
	if !strings.Contains(res.Body.String(), `"full_name":"Ada Byron"`) || !strings.Contains(res.Body.String(), `"name":"Ada Byron"`) {
		t.Fatal("name update was not persisted")
	}
}

func TestPasswordChangeVerifiesCurrentAndRevokesOtherTokens(t *testing.T) {
	h, _, auth, user, token, _, _ := profileHarness(t)
	other, err := auth.CreateToken(context.Background(), user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	res := profileRequest(t, h, "POST", "/api/profile/password", token, `{"current_password":"wrong","new_password":"Replacement123!"}`)
	if res.Code != 401 {
		t.Fatalf("wrong password: %d", res.Code)
	}
	if _, err := auth.ValidateToken(context.Background(), other.Token); err != nil {
		t.Fatal("wrong password revoked another token")
	}
	res = profileRequest(t, h, "POST", "/api/profile/password", token, `{"current_password":"Current123!","new_password":"Replacement123!"}`)
	if res.Code != 200 {
		t.Fatalf("change password: %d %s", res.Code, res.Body)
	}
	if _, err := auth.ValidateToken(context.Background(), other.Token); !errors.Is(err, repository.ErrInvalidToken) {
		t.Fatalf("other token was not revoked: %v", err)
	}
	if _, err := auth.ValidateToken(context.Background(), token); err != nil {
		t.Fatal("current token revoked")
	}
	if _, err := auth.Authenticate(context.Background(), user.Email, "Current123!"); !errors.Is(err, repository.ErrInvalidPassword) {
		t.Fatal("old password still works")
	}
	if _, err := auth.Authenticate(context.Background(), user.Email, "Replacement123!"); err != nil {
		t.Fatal("new password rejected")
	}
}

func TestEmailChangeDeliversOnceAndConsumesOnce(t *testing.T) {
	h, database, auth, user, token, mailer, _ := profileHarness(t)
	email := fmt.Sprintf("changed_%d@example.com", time.Now().UnixNano())
	res := profileRequest(t, h, "POST", "/api/profile/email-change", token, `{"email":"`+email+`"}`)
	if res.Code != 202 {
		t.Fatalf("request email change: %d %s", res.Code, res.Body)
	}
	if len(mailer.verificationRecipients) != 1 || mailer.verificationRecipients[0] != email {
		t.Fatal("verification not delivered exactly once to requested email")
	}
	link, err := url.Parse(mailer.verifications[0].Link)
	if err != nil {
		t.Fatal(err)
	}
	raw := link.Query().Get("token")
	if raw == "" || strings.Contains(res.Body.String(), raw) || strings.Contains(res.Body.String(), "token") {
		t.Fatal("missing mail token or token leaked in response")
	}
	var stored string
	var expiry time.Time
	if err := database.QueryRow("SELECT token_hash, expires_at FROM UserVerificationTokens WHERE user_id = @id AND consumed_at IS NULL", sql.Named("id", user.ID)).Scan(&stored, &expiry); err != nil {
		t.Fatal(err)
	}
	if stored != models.HashToken(raw) || time.Until(expiry) > 30*time.Minute || time.Until(expiry) < 29*time.Minute {
		t.Fatal("verification hash or 30 minute expiry incorrect")
	}
	if current, err := auth.GetUserByID(context.Background(), user.ID); err != nil || current.Email != user.Email {
		t.Fatal("email changed before verification")
	}
	res = profileRequest(t, h, "POST", "/api/auth/verify-email", "", `{"token":"`+raw+`"}`)
	if res.Code != 200 {
		t.Fatalf("verify email: %d %s", res.Code, res.Body)
	}
	if current, err := auth.GetUserByID(context.Background(), user.ID); err != nil || current.Email != email {
		t.Fatal("verified email not activated")
	}
	res = profileRequest(t, h, "POST", "/api/auth/verify-email", "", `{"token":"`+raw+`"}`)
	if res.Code != 400 {
		t.Fatalf("replayed token accepted: %d", res.Code)
	}
	res = profileRequest(t, h, "GET", "/api/profile", token, "")
	if !strings.Contains(res.Body.String(), `"email_verified":true`) {
		t.Fatal("verification flag not persisted")
	}
	if current, err := auth.ValidateToken(context.Background(), token); err != nil || !current.Public().EmailVerified {
		t.Fatal("authenticated account data did not refresh the verified flag")
	}
}

func TestEmailChangeRejectsExpiredSupersededAndDuplicateEmail(t *testing.T) {
	h, database, auth, user, token, mailer, _ := profileHarness(t)
	request := func(email string) string {
		t.Helper()
		if _, err := database.Exec("UPDATE UserVerificationTokens SET created_at = @past WHERE user_id = @id", sql.Named("past", time.Now().Add(-2*time.Minute)), sql.Named("id", user.ID)); err != nil {
			t.Fatal(err)
		}
		res := profileRequest(t, h, "POST", "/api/profile/email-change", token, `{"email":"`+email+`"}`)
		if res.Code != 202 {
			t.Fatalf("email request: %d", res.Code)
		}
		u, _ := url.Parse(mailer.verifications[len(mailer.verifications)-1].Link)
		return u.Query().Get("token")
	}
	first := request("first_" + user.Email)
	second := request("second_" + user.Email)
	if res := profileRequest(t, h, "POST", "/api/auth/verify-email", "", `{"token":"`+first+`"}`); res.Code != 400 {
		t.Fatal("superseded token accepted")
	}
	if _, err := database.Exec("UPDATE UserVerificationTokens SET expires_at = @past WHERE token_hash = @hash", sql.Named("past", time.Now().Add(-time.Minute)), sql.Named("hash", models.HashToken(second))); err != nil {
		t.Fatal(err)
	}
	if res := profileRequest(t, h, "POST", "/api/auth/verify-email", "", `{"token":"`+second+`"}`); res.Code != 400 {
		t.Fatal("expired token accepted")
	}
	other, err := auth.Register(context.Background(), "Other", "other_"+user.Email, "Password123!")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.Exec("DELETE FROM Users WHERE id = @id", sql.Named("id", other.ID)) })
	if res := profileRequest(t, h, "POST", "/api/profile/email-change", token, `{"email":"`+other.Email+`"}`); res.Code != 409 {
		t.Fatalf("duplicate email: %d", res.Code)
	}
	mailer.err = errors.New("provider failure contains secret link")
	if _, err := database.Exec("UPDATE UserVerificationTokens SET created_at = @past WHERE user_id = @id", sql.Named("past", time.Now().Add(-2*time.Minute)), sql.Named("id", user.ID)); err != nil {
		t.Fatal(err)
	}
	if res := profileRequest(t, h, "POST", "/api/profile/email-change", token, `{"email":"failed_`+user.Email+`"}`); res.Code != 502 || strings.Contains(res.Body.String(), "secret link") {
		t.Fatalf("unsafe mail failure: %d", res.Code)
	}
}

func TestProfileValidationAndAuthentication(t *testing.T) {
	auth, err := repository.NewAuthRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.Register(context.Background(), "Offline", "offline@example.com", "Current123!")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.CreateToken(context.Background(), user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sessions := repository.NewSessionRepository()
	ph := profile.NewHandler(repository.NewMSSQLProfileRepository(nil), &fakeMailer{}, t.TempDir(), "https://maps.example.com")
	h := api.NewRouter(sessions, auth, nil, api.NewMiddleware(sessions, auth), nil, nil, nil, nil, nil, nil, ph).BuildHandler()
	for _, path := range []string{"/api/profile", "/api/profile/avatar", "/api/profile/password", "/api/profile/email-change"} {
		if res := profileRequest(t, h, "POST", path, "", "{}"); res.Code != 401 {
			t.Fatalf("unauthenticated %s: %d", path, res.Code)
		}
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"PATCH", "/api/profile", `{"full_name":"  "}`, 400},
		{"PATCH", "/api/profile", `{"full_name":"` + strings.Repeat("a", 256) + `"}`, 400},
		{"PATCH", "/api/profile", `{`, 400},
		{"POST", "/api/profile/password", `{"current_password":"x","new_password":"short"}`, 400},
		{"POST", "/api/profile/password", `{"current_password":"x","new_password":"` + strings.Repeat("a", 73) + `"}`, 400},
		{"POST", "/api/profile/password", `{"current_password":"` + strings.Repeat("a", 73) + `","new_password":"Replacement123!"}`, 400},
		{"POST", "/api/profile/email-change", `{"email":"bad@example.com extra"}`, 400},
		{"POST", "/api/auth/verify-email", `{"token":""}`, 400},
		{"DELETE", "/api/profile", `{}`, 405},
	} {
		if res := profileRequest(t, h, tc.method, tc.path, token.Token, tc.body); res.Code != tc.status {
			t.Errorf("%s %s expected %d got %d", tc.method, tc.path, tc.status, res.Code)
		}
	}
}

func TestProfileAvatarValidationAndPersistence(t *testing.T) {
	h, _, _, _, token, _, root := profileHarness(t)
	var valid bytes.Buffer
	if err := png.Encode(&valid, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		data   []byte
		status int
	}{
		{"avatar.png", valid.Bytes(), 200},
		{"fake.png", []byte("not an image"), 400},
		{"too-large.png", append(valid.Bytes(), make([]byte, 5*1024*1024)...), 413},
	} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, err := writer.CreateFormFile("avatar", tc.name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.Write(tc.data)
		_ = writer.Close()
		req := httptest.NewRequest("POST", "/api/profile/avatar", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.name, res.Code, res.Body)
		}
		if tc.status == 200 {
			var result struct {
				Data models.UserPublicProfile `json:"data"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Data.ProfilePicture != "/api/profile/avatar" {
				t.Fatalf("private path in avatar URL: %s", result.Data.ProfilePicture)
			}
			get := profileRequest(t, h, "GET", "/api/profile/avatar", token, "")
			if get.Code != 200 || !bytes.Equal(get.Body.Bytes(), valid.Bytes()) {
				t.Fatal("saved avatar not served")
			}
		}
	}
	var files int
	_ = filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			files++
		}
		return err
	})
	if files != 1 {
		t.Fatalf("rejected avatars left files: %d", files)
	}
}

func TestProfileConfigRequiresResendOutsideTests(t *testing.T) {
	for _, missing := range []string{"RESEND_API_KEY", "RESEND_FROM_EMAIL", "APP_PUBLIC_URL"} {
		cfg := &config.Config{Environment: "production", MSSQLConnStr: "configured", WorkspaceTTL: time.Hour, ResendAPIKey: "test-key", ResendFrom: "maps@example.com", AppPublicURL: "https://maps.example.com"}
		switch missing {
		case "RESEND_API_KEY":
			cfg.ResendAPIKey = " "
		case "RESEND_FROM_EMAIL":
			cfg.ResendFrom = ""
		case "APP_PUBLIC_URL":
			cfg.AppPublicURL = ""
		}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("expected %s validation error, got %v", missing, err)
		}
	}
}

func TestPasswordChangeRollsBackIfTokenRevocationFails(t *testing.T) {
	_, database, auth, user, token, _, _ := profileHarness(t)
	other, err := auth.CreateToken(context.Background(), user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	trigger := "ProfileRevokeFailure" + strings.TrimPrefix(user.ID, "usr_")
	_, err = database.Exec("CREATE TRIGGER " + trigger + " ON AuthTokens AFTER UPDATE AS BEGIN IF EXISTS (SELECT 1 FROM inserted WHERE user_id = '" + user.ID + "') THROW 50000, 'test revocation failure', 1; END")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.Exec("DROP TRIGGER IF EXISTS " + trigger) })
	err = repository.NewMSSQLProfileRepository(database).ChangePassword(context.Background(), user.ID, "Current123!", "Replacement123!", token)
	if err == nil {
		t.Fatal("expected token revocation failure")
	}
	if _, err := auth.Authenticate(context.Background(), user.Email, "Current123!"); err != nil {
		t.Fatal("password was changed despite transaction failure")
	}
	if _, err := auth.ValidateToken(context.Background(), other.Token); err != nil {
		t.Fatal("token was revoked despite transaction failure")
	}
}
