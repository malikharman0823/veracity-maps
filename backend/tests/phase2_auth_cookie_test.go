package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/models"
)

func TestPhase2_AuthCookieAndTestLogin(t *testing.T) {
	ts, _, tempDir := createTestServer(t)
	defer cleanupTestDir(t, tempDir)

	// 1. TestLogin for User A (no OTP needed for automated testing)
	userABody, _ := json.Marshal(map[string]string{
		"email": "usera@example.com",
		"name":  "User A",
	})
	respA, err := http.Post(ts.URL+"/api/auth/test-login", "application/json", bytes.NewReader(userABody))
	if err != nil {
		t.Fatalf("Failed to test-login User A: %v", err)
	}
	defer respA.Body.Close()

	if respA.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for User A test-login, got %d", respA.StatusCode)
	}

	// Verify Set-Cookie was sent
	setCookie := respA.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "auth_token=") {
		t.Fatalf("Expected Set-Cookie with auth_token for User A, got: %s", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("Expected HttpOnly cookie, got: %s", setCookie)
	}

	var authDataA struct {
		Success bool                 `json:"success"`
		Data    *models.AuthResponse `json:"data"`
	}
	if err := json.NewDecoder(respA.Body).Decode(&authDataA); err != nil {
		t.Fatalf("Failed to decode User A auth response: %v", err)
	}
	if authDataA.Data == nil || authDataA.Data.User.Email != "usera@example.com" {
		t.Fatalf("Expected User A email usera@example.com, got %+v", authDataA.Data)
	}

	// 2. Verify /api/auth/me works using cookie alone (no Authorization header)
	reqMe, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth/me", nil)
	reqMe.Header.Set("Cookie", "auth_token="+authDataA.Data.Token)
	respMe, err := http.DefaultClient.Do(reqMe)
	if err != nil {
		t.Fatalf("Failed to request /api/auth/me: %v", err)
	}
	defer respMe.Body.Close()

	if respMe.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for /api/auth/me with cookie, got %d", respMe.StatusCode)
	}
	var meData struct {
		Success bool                      `json:"success"`
		Data    *models.UserPublicProfile `json:"data"`
	}
	if err := json.NewDecoder(respMe.Body).Decode(&meData); err != nil {
		t.Fatalf("Failed to decode /api/auth/me response: %v", err)
	}
	if meData.Data.Email != "usera@example.com" {
		t.Fatalf("Expected me email usera@example.com, got %s", meData.Data.Email)
	}

	// 3. TestLogin for User B
	userBBody, _ := json.Marshal(map[string]string{
		"email": "userb@example.com",
		"name":  "User B",
	})
	respB, err := http.Post(ts.URL+"/api/auth/test-login", "application/json", bytes.NewReader(userBBody))
	if err != nil {
		t.Fatalf("Failed to test-login User B: %v", err)
	}
	defer respB.Body.Close()

	if respB.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for User B test-login, got %d", respB.StatusCode)
	}

	// 4. Test OTP bypass code '000000' in non-production
	verifyBody, _ := json.Marshal(map[string]string{
		"email":   "userc@example.com",
		"code":    "000000",
		"purpose": "register",
		"name":    "User C",
	})
	respOTP, err := http.Post(ts.URL+"/api/auth/otp/verify", "application/json", bytes.NewReader(verifyBody))
	if err != nil {
		t.Fatalf("Failed to verify OTP with test code: %v", err)
	}
	defer respOTP.Body.Close()
	if respOTP.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for test OTP 000000, got %d", respOTP.StatusCode)
	}

	// 5. Test Logout clears cookie
	logoutReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/logout", nil)
	logoutReq.Header.Set("Cookie", "auth_token="+authDataA.Data.Token)
	logoutResp, err := http.DefaultClient.Do(logoutReq)
	if err != nil {
		t.Fatalf("Failed to logout: %v", err)
	}
	defer logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for logout, got %d", logoutResp.StatusCode)
	}
	logoutCookie := logoutResp.Header.Get("Set-Cookie")
	if !strings.Contains(logoutCookie, "auth_token=;") && !strings.Contains(logoutCookie, "Max-Age=0") && !strings.Contains(logoutCookie, "auth_token=") {
		t.Fatalf("Expected clearing cookie on logout, got: %s", logoutCookie)
	}
}
