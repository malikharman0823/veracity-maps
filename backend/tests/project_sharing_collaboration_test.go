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
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/functions/upload"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
	"github.com/malikharman0823/veracity-maps/storage"
)

func setupCollaborationTestServer(t *testing.T, tempDir string) (*httptest.Server, *repository.AuthRepository, *repository.ProjectRepository) {
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

func registerHelper(t *testing.T, serverURL, name, email, password string) models.AuthResponse {
	body, _ := json.Marshal(map[string]string{
		"name":     name,
		"email":    email,
		"password": password,
	})
	res, err := http.Post(serverURL+"/api/auth/register", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Registration failed for %s: %v", email, err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for %s, got %d", email, res.StatusCode)
	}

	var wrap struct {
		Success bool                `json:"success"`
		Data    models.AuthResponse `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&wrap); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	return wrap.Data
}

func TestPhase2_ProjectSharingAndCollaboration(t *testing.T) {
	tempDir := t.TempDir()
	server, _, _ := setupCollaborationTestServer(t, tempDir)
	defer server.Close()

	// 1. Register 3 Users: User A (Owner), User B (Editor), User C (Viewer)
	userA := registerHelper(t, server.URL, "User Alpha", "owner@veracity.maps", "Password123!")
	userB := registerHelper(t, server.URL, "User Beta", "editor@veracity.maps", "Password123!")
	userC := registerHelper(t, server.URL, "User Gamma", "viewer@veracity.maps", "Password123!")

	// 2. User A creates a project
	createProjBody, _ := json.Marshal(map[string]string{
		"name":        "Metropolitan Transit Expansion",
		"description": "Multi-station blueprint and boundary maps",
	})
	reqCreate, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects", bytes.NewBuffer(createProjBody))
	reqCreate.Header.Set("Authorization", "Bearer "+userA.Token)
	reqCreate.Header.Set("Content-Type", "application/json")
	resCreate, err := http.DefaultClient.Do(reqCreate)
	if err != nil {
		t.Fatalf("Create project failed: %v", err)
	}
	var wrapProj struct {
		Success bool               `json:"success"`
		Data    models.UserProject `json:"data"`
	}
	_ = json.NewDecoder(resCreate.Body).Decode(&wrapProj)
	resCreate.Body.Close()
	projectA := wrapProj.Data

	// 3. User A invites User B as Editor
	shareBBody, _ := json.Marshal(map[string]string{
		"email": "editor@veracity.maps",
		"role":  "editor",
	})
	reqShareB, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects/"+projectA.ID+"/share", bytes.NewBuffer(shareBBody))
	reqShareB.Header.Set("Authorization", "Bearer "+userA.Token)
	reqShareB.Header.Set("Content-Type", "application/json")
	resShareB, err := http.DefaultClient.Do(reqShareB)
	if err != nil {
		t.Fatalf("Share with B failed: %v", err)
	}
	if resShareB.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for invite B, got %d", resShareB.StatusCode)
	}
	resShareB.Body.Close()

	// 4. User A invites User C as Viewer
	shareCBody, _ := json.Marshal(map[string]string{
		"email": "viewer@veracity.maps",
		"role":  "viewer",
	})
	reqShareC, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects/"+projectA.ID+"/share", bytes.NewBuffer(shareCBody))
	reqShareC.Header.Set("Authorization", "Bearer "+userA.Token)
	reqShareC.Header.Set("Content-Type", "application/json")
	resShareC, err := http.DefaultClient.Do(reqShareC)
	if err != nil {
		t.Fatalf("Share with C failed: %v", err)
	}
	if resShareC.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for invite C, got %d", resShareC.StatusCode)
	}
	resShareC.Body.Close()

	// 5. Duplicate Invitation Check: User A tries to re-invite User B -> Expect 409 Conflict
	resShareDup, _ := http.DefaultClient.Do(reqShareB)
	if resShareDup.StatusCode != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict for duplicate invite, got %d", resShareDup.StatusCode)
	}
	resShareDup.Body.Close()

	// 6. User B checks pending invitations
	reqInvB, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/invitations", nil)
	reqInvB.Header.Set("Authorization", "Bearer "+userB.Token)
	resInvB, _ := http.DefaultClient.Do(reqInvB)
	var wrapInvB struct {
		Success bool `json:"success"`
		Data    struct {
			Invitations []*models.PendingInvitationView `json:"invitations"`
			Total       int                             `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resInvB.Body).Decode(&wrapInvB)
	resInvB.Body.Close()

	if wrapInvB.Data.Total != 1 || wrapInvB.Data.Invitations[0].Role != models.RoleEditor {
		t.Fatalf("Expected 1 pending editor invite for User B, got %d", wrapInvB.Data.Total)
	}
	inviteBID := wrapInvB.Data.Invitations[0].InvitationID

	// 7. User B accepts the invitation
	acceptBody, _ := json.Marshal(map[string]bool{"accept": true})
	reqAcceptB, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects/invitations/"+inviteBID+"/respond", bytes.NewBuffer(acceptBody))
	reqAcceptB.Header.Set("Authorization", "Bearer "+userB.Token)
	reqAcceptB.Header.Set("Content-Type", "application/json")
	resAcceptB, _ := http.DefaultClient.Do(reqAcceptB)
	if resAcceptB.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for accept, got %d", resAcceptB.StatusCode)
	}
	resAcceptB.Body.Close()

	// User C checks and accepts invitation
	reqInvC, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/invitations", nil)
	reqInvC.Header.Set("Authorization", "Bearer "+userC.Token)
	resInvC, _ := http.DefaultClient.Do(reqInvC)
	var wrapInvC struct {
		Success bool `json:"success"`
		Data    struct {
			Invitations []*models.PendingInvitationView `json:"invitations"`
			Total       int                             `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resInvC.Body).Decode(&wrapInvC)
	resInvC.Body.Close()
	inviteCID := wrapInvC.Data.Invitations[0].InvitationID

	reqAcceptC, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects/invitations/"+inviteCID+"/respond", bytes.NewBuffer(acceptBody))
	reqAcceptC.Header.Set("Authorization", "Bearer "+userC.Token)
	reqAcceptC.Header.Set("Content-Type", "application/json")
	resAcceptC, _ := http.DefaultClient.Do(reqAcceptC)
	resAcceptC.Body.Close()

	// 8. Shared Dashboard Check: User B lists shared projects -> Should see project with Role "editor"
	reqSharedB, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/shared", nil)
	reqSharedB.Header.Set("Authorization", "Bearer "+userB.Token)
	resSharedB, _ := http.DefaultClient.Do(reqSharedB)
	var wrapSharedB struct {
		Success bool `json:"success"`
		Data    struct {
			SharedProjects []*models.SharedProjectView `json:"shared_projects"`
			Total          int                         `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resSharedB.Body).Decode(&wrapSharedB)
	resSharedB.Body.Close()

	if wrapSharedB.Data.Total != 1 || wrapSharedB.Data.SharedProjects[0].Role != models.RoleEditor {
		t.Fatalf("User B should see 1 shared editor project, got %d", wrapSharedB.Data.Total)
	}

	// 9. Editor Permission Verification: User B modifies project metadata -> Allowed!
	updateBody, _ := json.Marshal(map[string]string{
		"name":        "Metropolitan Transit Expansion (Editor Updated)",
		"description": "Updated by User Beta",
	})
	reqUpdateB, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/projects/"+projectA.ID, bytes.NewBuffer(updateBody))
	reqUpdateB.Header.Set("Authorization", "Bearer "+userB.Token)
	reqUpdateB.Header.Set("Content-Type", "application/json")
	resUpdateB, _ := http.DefaultClient.Do(reqUpdateB)
	if resUpdateB.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for editor updating project, got %d", resUpdateB.StatusCode)
	}
	resUpdateB.Body.Close()

	// 10. Editor Security Restriction: User B attempts to DELETE project -> Must be blocked with 403 Forbidden!
	reqDelB, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/projects/"+projectA.ID, nil)
	reqDelB.Header.Set("Authorization", "Bearer "+userB.Token)
	resDelB, _ := http.DefaultClient.Do(reqDelB)
	if resDelB.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when editor attempts delete, got %d", resDelB.StatusCode)
	}
	resDelB.Body.Close()

	// 11. Editor Security Restriction: User B attempts to invite someone -> Must be blocked with 403 Forbidden!
	reqInviteByB, _ := http.NewRequest(http.MethodPost, server.URL+"/api/projects/"+projectA.ID+"/share", bytes.NewBuffer(shareCBody))
	reqInviteByB.Header.Set("Authorization", "Bearer "+userB.Token)
	reqInviteByB.Header.Set("Content-Type", "application/json")
	resInviteByB, _ := http.DefaultClient.Do(reqInviteByB)
	if resInviteByB.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when editor attempts to share, got %d", resInviteByB.StatusCode)
	}
	resInviteByB.Body.Close()

	// 12. Viewer Permission Verification: User C reads project -> Allowed with role "viewer"
	reqGetC, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/"+projectA.ID, nil)
	reqGetC.Header.Set("Authorization", "Bearer "+userC.Token)
	resGetC, _ := http.DefaultClient.Do(reqGetC)
	if resGetC.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for viewer getting project, got %d", resGetC.StatusCode)
	}
	var wrapGetC struct {
		Success bool `json:"success"`
		Data    struct {
			Project *models.UserProject `json:"project"`
			Role    models.ProjectRole  `json:"role"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resGetC.Body).Decode(&wrapGetC)
	resGetC.Body.Close()
	if wrapGetC.Data.Role != models.RoleViewer {
		t.Fatalf("Expected viewer role for User C, got %s", wrapGetC.Data.Role)
	}

	// 13. Viewer Security Restriction: User C attempts to update project -> Must be blocked with 403 Forbidden!
	reqUpdateC, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/projects/"+projectA.ID, bytes.NewBuffer(updateBody))
	reqUpdateC.Header.Set("Authorization", "Bearer "+userC.Token)
	reqUpdateC.Header.Set("Content-Type", "application/json")
	resUpdateC, _ := http.DefaultClient.Do(reqUpdateC)
	if resUpdateC.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden when viewer attempts update, got %d", resUpdateC.StatusCode)
	}
	resUpdateC.Body.Close()

	// 14. Collaborator Roster & Role Update: User A promotes User C to Editor
	reqMembersA, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/"+projectA.ID+"/members", nil)
	reqMembersA.Header.Set("Authorization", "Bearer "+userA.Token)
	resMembersA, _ := http.DefaultClient.Do(reqMembersA)
	var wrapMembersA struct {
		Success bool `json:"success"`
		Data    struct {
			Members []*models.ProjectMember `json:"members"`
			Total   int                     `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resMembersA.Body).Decode(&wrapMembersA)
	resMembersA.Body.Close()

	if wrapMembersA.Data.Total != 3 {
		t.Fatalf("Expected 3 members (Owner + 2 Collaborators), got %d", wrapMembersA.Data.Total)
	}

	var memberCID string
	for _, m := range wrapMembersA.Data.Members {
		if m.UserID == userC.User.ID {
			memberCID = m.ID
			break
		}
	}

	promoteBody, _ := json.Marshal(map[string]string{"role": "editor"})
	reqPromote, _ := http.NewRequest(http.MethodPatch, server.URL+"/api/projects/"+projectA.ID+"/members/"+memberCID, bytes.NewBuffer(promoteBody))
	reqPromote.Header.Set("Authorization", "Bearer "+userA.Token)
	reqPromote.Header.Set("Content-Type", "application/json")
	resPromote, _ := http.DefaultClient.Do(reqPromote)
	if resPromote.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for role promotion, got %d", resPromote.StatusCode)
	}
	resPromote.Body.Close()

	// 15. Activity Log Check: User A lists activity log
	reqActA, _ := http.NewRequest(http.MethodGet, server.URL+"/api/projects/"+projectA.ID+"/activities", nil)
	reqActA.Header.Set("Authorization", "Bearer "+userA.Token)
	resActA, _ := http.DefaultClient.Do(reqActA)
	var wrapActA struct {
		Success bool `json:"success"`
		Data    struct {
			Activities []*models.ProjectActivity `json:"activities"`
			Total      int                       `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resActA.Body).Decode(&wrapActA)
	resActA.Body.Close()

	if wrapActA.Data.Total < 4 {
		t.Fatalf("Expected at least 4 activities logged, got %d", wrapActA.Data.Total)
	}

	// 16. Revoke Collaborator: User A removes User C
	reqRemoveC, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/projects/"+projectA.ID+"/members/"+memberCID, nil)
	reqRemoveC.Header.Set("Authorization", "Bearer "+userA.Token)
	resRemoveC, _ := http.DefaultClient.Do(reqRemoveC)
	if resRemoveC.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for remove collaborator, got %d", resRemoveC.StatusCode)
	}
	resRemoveC.Body.Close()

	// User C attempts to get project after revocation -> Must return 403 Forbidden!
	resGetCAfterRevoke, _ := http.DefaultClient.Do(reqGetC)
	if resGetCAfterRevoke.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden after revocation, got %d", resGetCAfterRevoke.StatusCode)
	}
	resGetCAfterRevoke.Body.Close()
}
