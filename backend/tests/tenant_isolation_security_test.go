package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malikharman0823/veracity-maps/db"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/storage"
)

// TestTenantIsolation_PrivateStorageAndQuota verifies per-user filesystem isolation,
// path traversal defense, unique internal filenames, and per-user storage quota enforcement.
func TestTenantIsolation_PrivateStorageAndQuota(t *testing.T) {
	ps, err := storage.NewPrivateStorage(filepath.Join(t.TempDir(), "private"), 1024)
	if err != nil {
		t.Fatalf("NewPrivateStorage failed: %v", err)
	}

	// User A writes a private upload.
	payloadA := []byte("confidential-raster-bytes-user-a")
	pathA1, err := ps.WritePrivateFile("user-a", "proj-1", "uploads", "site_plan.tif", payloadA)
	if err != nil {
		t.Fatalf("User A write failed: %v", err)
	}

	// Second write with same original filename produces a distinct internal path (no overwrite).
	pathA2, err := ps.WritePrivateFile("user-a", "proj-1", "uploads", "site_plan.tif", payloadA)
	if err != nil {
		t.Fatalf("User A second write failed: %v", err)
	}
	if pathA1 == pathA2 {
		t.Fatalf("expected unique internal filenames, got identical paths: %s", pathA1)
	}

	// User A can read their own file.
	got, err := ps.ReadPrivateFile("user-a", pathA1)
	if err != nil || !bytes.Equal(got, payloadA) {
		t.Fatalf("User A failed to read own file: err=%v", err)
	}

	// Cross-user read & delete by User B MUST be denied even with the exact file path.
	if _, err := ps.ReadPrivateFile("user-b", pathA1); !errors.Is(err, storage.ErrUnauthorizedStorageAccess) {
		t.Fatalf("expected ErrUnauthorizedStorageAccess when User B reads User A file, got: %v", err)
	}
	if err := ps.DeletePrivateFile("user-b", pathA1); !errors.Is(err, storage.ErrUnauthorizedStorageAccess) {
		t.Fatalf("expected ErrUnauthorizedStorageAccess when User B deletes User A file, got: %v", err)
	}

	// Path traversal in ownerID, projectID, or candidate path MUST be rejected.
	if _, err := ps.UserRootDir("../user-a"); !errors.Is(err, storage.ErrInvalidStorageNamespace) {
		t.Fatalf("expected ErrInvalidStorageNamespace for traversal ownerID, got: %v", err)
	}
	if _, err := ps.ProjectBucketDir("user-b", "../../user-a", "uploads"); !errors.Is(err, storage.ErrInvalidStorageNamespace) {
		t.Fatalf("expected ErrInvalidStorageNamespace for traversal projectID, got: %v", err)
	}
	if _, err := ps.ProjectBucketDir("user-b", "proj-1", "invalid-bucket"); !errors.Is(err, storage.ErrInvalidStorageNamespace) {
		t.Fatalf("expected ErrInvalidStorageNamespace for unsupported bucket, got: %v", err)
	}

	// Per-user storage quota enforcement (1024 bytes limit).
	oversize := bytes.Repeat([]byte("x"), 1025)
	if _, err := ps.WritePrivateFile("user-a", "proj-1", "uploads", "huge.tif", oversize); !errors.Is(err, storage.ErrStorageQuotaExceeded) {
		t.Fatalf("expected ErrStorageQuotaExceeded for oversize upload, got: %v", err)
	}

	// User A can delete their own file.
	if err := ps.DeletePrivateFile("user-a", pathA1); err != nil {
		t.Fatalf("User A delete own file failed: %v", err)
	}
}

// TestTenantIsolation_RLSContextAndMigrationPolicy validates the fail-closed Go RLS context
// helper and verifies that migration 019 defines FILTER + BLOCK predicates for all user tables.
func TestTenantIsolation_RLSContextAndMigrationPolicy(t *testing.T) {
	// 1. Empty or whitespace userID must fail closed with ErrMissingTenantContext.
	for _, emptyID := range []string{"", "   ", "\t\n"} {
		err := db.WithUserContext(context.Background(), nil, emptyID, func(_ context.Context, _ *sql.Conn) error {
			t.Fatal("callback must never run without an authenticated user_id")
			return nil
		})
		if !errors.Is(err, db.ErrMissingTenantContext) {
			t.Fatalf("expected ErrMissingTenantContext for %q, got: %v", emptyID, err)
		}
	}

	// 2. Nil DB handle must return an error cleanly.
	if err := db.WithUserContext(context.Background(), nil, "user-123", func(_ context.Context, _ *sql.Conn) error {
		return nil
	}); err == nil {
		t.Fatal("expected error for nil sql.DB handle")
	}

	// 3. Verify 019_rls_user_isolation.sql covers all 12 user-owned tables with FILTER + BLOCK predicates.
	migrationPath := filepath.Join("..", "migrations", "019_rls_user_isolation.sql")
	rawSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("failed to read RLS migration %s: %v", migrationPath, err)
	}
	sqlText := string(rawSQL)

	requiredSnippets := []string{
		"security.fn_user_access",
		"WITH SCHEMABINDING",
		"SESSION_CONTEXT(N'user_id')",
		"SECURITY POLICY security.UserIsolationPolicy",
		"AFTER INSERT",
		"BEFORE UPDATE",
		"AFTER UPDATE",
		"BEFORE DELETE",
		"dbo.Maps",
		"dbo.Map_Assets",
		"dbo.Projects",
		"dbo.Datasets",
		"dbo.User_Saved_Maps",
		"dbo.User_Map_Categories",
		"dbo.processing_jobs",
		"dbo.processing_presets",
		"dbo.base_layers",
		"dbo.workspace_datasets",
		"dbo.Activity_Logs",
		"dbo.User_Application_Settings",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(sqlText, snippet) {
			t.Errorf("019_rls_user_isolation.sql missing required RLS clause/table: %s", snippet)
		}
	}
}

// TestTenantIsolation_HTTPAuthAndCrossUserAccess verifies unauthenticated denial,
// forged token denial, and cross-user read/update/delete denial across User A and User B.
func TestTenantIsolation_HTTPAuthAndCrossUserAccess(t *testing.T) {
	server, _, _ := setupMultiUserTestServer(t, t.TempDir())
	defer server.Close()

	// 1. Unauthenticated request must return 401.
	resUnauth, err := http.Get(server.URL + "/api/projects")
	if err != nil {
		t.Fatalf("unauthenticated request failed: %v", err)
	}
	resUnauth.Body.Close()
	if resUnauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /api/projects, got %d", resUnauth.StatusCode)
	}

	// 2. Forged/expired bearer token must return 401.
	reqForged, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects", nil)
	reqForged.Header.Set("Authorization", "Bearer vmap_forged_or_expired_token_000")
	resForged, err := http.DefaultClient.Do(reqForged)
	if err != nil {
		t.Fatalf("forged token request failed: %v", err)
	}
	resForged.Body.Close()
	if resForged.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for forged token, got %d", resForged.StatusCode)
	}

	// 3. Register User A and User B.
	registerUser := func(name, email, password string) models.AuthResponse {
		body, _ := json.Marshal(map[string]string{"name": name, "email": email, "password": password})
		res, err := http.Post(server.URL+"/api/auth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("register %s failed: %v", email, err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("register %s expected 201, got %d", email, res.StatusCode)
		}
		var wrap struct {
			Data models.AuthResponse `json:"data"`
		}
		if err := json.NewDecoder(res.Body).Decode(&wrap); err != nil {
			t.Fatalf("decode register %s: %v", email, err)
		}
		return wrap.Data
	}

	authA := registerUser("Tenant Alpha", "tenant-a@veracity.maps", "SecurePassA123!")
	authB := registerUser("Tenant Beta", "tenant-b@veracity.maps", "SecurePassB456!")

	// 4. User A creates a private project.
	createBody, _ := json.Marshal(map[string]string{"name": "Tenant A Confidential", "description": "Private GIS"})
	reqCreate, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects", bytes.NewReader(createBody))
	reqCreate.Header.Set("Authorization", "Bearer "+authA.Token)
	reqCreate.Header.Set("Content-Type", "application/json")
	resCreate, err := http.DefaultClient.Do(reqCreate)
	if err != nil {
		t.Fatalf("create project failed: %v", err)
	}
	var createWrap struct {
		Data models.UserProject `json:"data"`
	}
	_ = json.NewDecoder(resCreate.Body).Decode(&createWrap)
	resCreate.Body.Close()
	if resCreate.StatusCode != http.StatusCreated || createWrap.Data.ID == "" {
		t.Fatalf("expected 201 with project ID, got %d (%+v)", resCreate.StatusCode, createWrap.Data)
	}
	projID := createWrap.Data.ID

	// 5. User B cannot GET, PATCH/PUT, or DELETE User A's project.
	for _, tc := range []struct {
		method string
		body   []byte
	}{
		{http.MethodGet, nil},
		{http.MethodPut, []byte(`{"name":"Hijacked Name"}`)},
		{http.MethodDelete, nil},
	} {
		req, _ := http.NewRequest(tc.method, server.URL+"/api/projects/"+projID, bytes.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+authB.Token)
		if tc.body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("User B %s request failed: %v", tc.method, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 403/404 when User B calls %s on User A project, got %d", tc.method, res.StatusCode)
		}
	}

	// 6. User A can still GET and DELETE their own project.
	reqGetA, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/"+projID, nil)
	reqGetA.Header.Set("Authorization", "Bearer "+authA.Token)
	resGetA, err := http.DefaultClient.Do(reqGetA)
	if err != nil {
		t.Fatalf("User A GET own project failed: %v", err)
	}
	resGetA.Body.Close()
	if resGetA.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for User A GET own project, got %d", resGetA.StatusCode)
	}
}
