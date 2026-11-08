package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/malikharman0823/veracity-maps/api"
	"github.com/malikharman0823/veracity-maps/functions/auth"
	"github.com/malikharman0823/veracity-maps/functions/profile"
	"github.com/malikharman0823/veracity-maps/functions/projects"
	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type invitationMockMailer struct {
	sentInvs []services.InvitationEmail
}

func (m *invitationMockMailer) SendVerification(ctx context.Context, message services.VerificationEmail) error {
	return nil
}

func (m *invitationMockMailer) SendInvitation(ctx context.Context, message services.InvitationEmail) error {
	m.sentInvs = append(m.sentInvs, message)
	return nil
}

func (m *invitationMockMailer) SendOTP(ctx context.Context, message services.OTPEmail) error {
	return nil
}

func TestInvitationEmailAndProfileDashboard(t *testing.T) {
	tempDir := t.TempDir()
	authRepo, err := repository.NewAuthRepository(tempDir)
	if err != nil {
		t.Fatalf("failed to init authRepo: %v", err)
	}
	projRepo, err := repository.NewProjectRepository(tempDir)
	if err != nil {
		t.Fatalf("failed to init projRepo: %v", err)
	}
	sessionRepo := repository.NewSessionRepository()
	mailer := &invitationMockMailer{}
	appURL := "http://localhost:5173"

	// Register Owner & Collaborator
	owner, err := authRepo.Register(context.Background(), "Alice Owner", "alice@example.com", "Password123!")
	if err != nil {
		t.Fatalf("failed to register owner: %v", err)
	}
	collab, err := authRepo.Register(context.Background(), "Bob Collab", "bob@example.com", "Password123!")
	if err != nil {
		t.Fatalf("failed to register collab: %v", err)
	}

	ownerToken, _ := authRepo.CreateToken(context.Background(), owner.ID, time.Hour)
	collabToken, _ := authRepo.CreateToken(context.Background(), collab.ID, time.Hour)

	mw := api.NewMiddleware(sessionRepo, authRepo)
	authH := auth.NewHandler(authRepo)
	projH := projects.NewHandler(projRepo, sessionRepo, authRepo, mailer, appURL)
	profRepo := repository.NewMemoryProfileRepository(authRepo)
	profH := profile.NewHandler(profRepo, mailer, tempDir, appURL)
	profH.SetRepositories(sessionRepo, projRepo, authRepo)

	router := api.NewRouter(sessionRepo, authRepo, projRepo, mw, nil, nil, nil, nil, authH, projH, profH)
	server := httptest.NewServer(router.BuildHandler())
	defer server.Close()

	// 1. Create a project by Owner
	createProjBody, _ := json.Marshal(map[string]string{
		"name":        "Alpine Survey Map",
		"description": "Topographical survey dataset",
	})
	req, _ := http.NewRequest("POST", server.URL+"/api/projects", bytes.NewBuffer(createProjBody))
	req.Header.Set("Authorization", "Bearer "+ownerToken.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create project: %v, status: %d", err, resp.StatusCode)
	}
	var projResp struct {
		Success bool               `json:"success"`
		Data    models.UserProject `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&projResp)
	resp.Body.Close()
	createdProject := projResp.Data

	// 2. Share project with Bob -> triggers Invitation Email!
	shareBody, _ := json.Marshal(map[string]string{
		"email": "bob@example.com",
		"role":  "editor",
	})
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/projects/%s/share", server.URL, createdProject.ID), bytes.NewBuffer(shareBody))
	req.Header.Set("Authorization", "Bearer "+ownerToken.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated) {
		t.Fatalf("failed to share project: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Verify Invitation Email was sent
	if len(mailer.sentInvs) != 1 {
		t.Fatalf("expected 1 invitation email to be sent, got %d", len(mailer.sentInvs))
	}
	inv := mailer.sentInvs[0]
	if inv.To != "bob@example.com" {
		t.Errorf("expected recipient bob@example.com, got %s", inv.To)
	}
	if inv.ProjectName != "Alpine Survey Map" {
		t.Errorf("expected project Alpine Survey Map, got %s", inv.ProjectName)
	}
	if inv.Role != "editor" {
		t.Errorf("expected role editor, got %s", inv.Role)
	}
	if inv.InviterName != "Alice Owner" {
		t.Errorf("expected inviter Alice Owner, got %s", inv.InviterName)
	}

	// 3. Test Profile Dashboard for Alice
	req, _ = http.NewRequest("GET", server.URL+"/api/profile/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated) {
		t.Fatalf("failed to fetch owner profile dashboard: %v, status: %d", err, resp.StatusCode)
	}
	var aliceResp struct {
		Success bool                             `json:"success"`
		Data    profile.ProfileDashboardResponse `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&aliceResp)
	resp.Body.Close()
	aliceDash := aliceResp.Data

	if aliceDash.User == nil || aliceDash.User.FullName != "Alice Owner" {
		t.Errorf("unexpected user in dashboard: %+v", aliceDash.User)
	}
	if aliceDash.Stats.TotalProjects != 1 {
		t.Errorf("expected 1 project, got %d", aliceDash.Stats.TotalProjects)
	}

	// 4. Test Profile Dashboard for Bob (has 1 pending invitation)
	req, _ = http.NewRequest("GET", server.URL+"/api/profile/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+collabToken.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated) {
		t.Fatalf("failed to fetch collab profile dashboard: %v, status: %d", err, resp.StatusCode)
	}
	var bobResp struct {
		Success bool                             `json:"success"`
		Data    profile.ProfileDashboardResponse `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&bobResp)
	resp.Body.Close()
	bobDash := bobResp.Data

	if bobDash.Stats.PendingInvitations != 1 {
		t.Errorf("expected 1 pending invitation for Bob, got %d", bobDash.Stats.PendingInvitations)
	}
	if len(bobDash.PendingInvitations) != 1 || bobDash.PendingInvitations[0].ProjectName != "Alpine Survey Map" {
		t.Errorf("unexpected pending invitations for Bob: %+v", bobDash.PendingInvitations)
	}
}
