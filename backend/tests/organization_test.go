package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func setupOrgTestServer(t *testing.T, tempDir string) (*httptest.Server, *repository.AuthRepository, *repository.ProjectRepository, *repository.OrgRepository) {
	cfg := &config.Config{
		Port:              "0",
		Host:              "127.0.0.1",
		SessionStorageDir: tempDir + "/sessions",
		UploadDir:         tempDir + "/uploads",
		ProcessedDir:      tempDir + "/processed",
	}

	sm, err := storage.NewStorageManager(cfg.SessionStorageDir, cfg.UploadDir, cfg.ProcessedDir)
	if err != nil {
		t.Fatalf("failed creating storage manager: %v", err)
	}

	sessionRepo := repository.NewSessionRepository()
	sessionRepo.SetStorageDir(cfg.SessionStorageDir)

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

	return server, authRepo, projRepo, orgRepo
}

func orgAuthReq(method, url, token string, bodyData interface{}) (*http.Request, error) {
	var bodyBuf *bytes.Buffer
	if bodyData != nil {
		data, err := json.Marshal(bodyData)
		if err != nil {
			return nil, err
		}
		bodyBuf = bytes.NewBuffer(data)
	} else {
		bodyBuf = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, url, bodyBuf)
	if err != nil {
		return nil, err
	}
	if bodyData != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func TestPhase3_OrganizationAndTeamManagement(t *testing.T) {
	tempDir := t.TempDir()
	server, _, _, _ := setupOrgTestServer(t, tempDir)
	defer server.Close()

	client := &http.Client{}

	// 1. Register 3 Users: Alice (Owner), Bob (Admin/Editor), Charlie (Viewer)
	aliceAuth := registerHelper(t, server.URL, "Alice Alpha", "alice@alpha.org", "PasswordAlice123!")
	bobAuth := registerHelper(t, server.URL, "Bob Beta", "bob@beta.org", "PasswordBob123!")
	charlieAuth := registerHelper(t, server.URL, "Charlie Gamma", "charlie@gamma.org", "PasswordCharlie123!")

	// 2. Alice creates Organization "Alpha GeoTech"
	createOrgReq, _ := orgAuthReq("POST", server.URL+"/api/organizations", aliceAuth.Token, models.CreateOrgRequest{
		Name:        "Alpha GeoTech",
		Description: "Primary GIS & Mapping Team",
	})
	res, err := client.Do(createOrgReq)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("failed creating organization: code %d, err: %v", res.StatusCode, err)
	}
	var wrapAlpha struct {
		Success bool                `json:"success"`
		Data    models.Organization `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapAlpha)
	res.Body.Close()
	alphaOrg := wrapAlpha.Data
	if alphaOrg.ID == "" {
		t.Fatalf("unexpected created org: %+v", alphaOrg)
	}

	// 3. Bob creates independent Organization "Beta Analytics"
	createBetaReq, _ := orgAuthReq("POST", server.URL+"/api/organizations", bobAuth.Token, models.CreateOrgRequest{
		Name:        "Beta Analytics",
		Description: "Spatial ML Team",
	})
	resBeta, err := client.Do(createBetaReq)
	if err != nil || resBeta.StatusCode != http.StatusCreated {
		t.Fatalf("failed creating beta org: code %d, err: %v", resBeta.StatusCode, err)
	}
	var wrapBeta struct {
		Success bool                `json:"success"`
		Data    models.Organization `json:"data"`
	}
	_ = json.NewDecoder(resBeta.Body).Decode(&wrapBeta)
	resBeta.Body.Close()
	betaOrg := wrapBeta.Data

	// 4. Alice invites Bob as Editor and Charlie as Viewer to Alpha GeoTech
	inviteBobReq, _ := orgAuthReq("POST", fmt.Sprintf("%s/api/organizations/%s/members", server.URL, alphaOrg.ID), aliceAuth.Token, models.InviteOrgMemberRequest{
		Email: "bob@beta.org",
		Role:  models.OrgRoleEditor,
	})
	res, err = client.Do(inviteBobReq)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("Alice failed inviting Bob: %d", res.StatusCode)
	}
	res.Body.Close()

	inviteCharlieReq, _ := orgAuthReq("POST", fmt.Sprintf("%s/api/organizations/%s/members", server.URL, alphaOrg.ID), aliceAuth.Token, models.InviteOrgMemberRequest{
		Email: "charlie@gamma.org",
		Role:  models.OrgRoleViewer,
	})
	res, err = client.Do(inviteCharlieReq)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("Alice failed inviting Charlie: %d", res.StatusCode)
	}
	res.Body.Close()

	// 5. Bob lists invitations, sees Alpha GeoTech, and accepts
	listBobInvites, _ := orgAuthReq("GET", server.URL+"/api/organizations/invitations", bobAuth.Token, nil)
	res, err = client.Do(listBobInvites)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("Bob failed fetching invitations: %d", res.StatusCode)
	}
	var wrapBobInvites struct {
		Success bool `json:"success"`
		Data    struct {
			Invitations []models.PendingOrgInvitationView `json:"invitations"`
			Total       int                               `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapBobInvites)
	res.Body.Close()
	bobInvites := wrapBobInvites.Data.Invitations
	if len(bobInvites) != 1 || bobInvites[0].OrgID != alphaOrg.ID {
		t.Fatalf("Bob invitations mismatch: %+v", bobInvites)
	}

	// Bob accepts invitation
	respondBobReq, _ := orgAuthReq("POST", fmt.Sprintf("%s/api/organizations/invitations/%s/respond", server.URL, bobInvites[0].InvitationID), bobAuth.Token, models.RespondInvitationRequest{
		Accept: true,
	})
	res, err = client.Do(respondBobReq)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("Bob failed accepting invitation: %d", res.StatusCode)
	}
	res.Body.Close()

	// Charlie accepts invitation
	listCharlieInvites, _ := orgAuthReq("GET", server.URL+"/api/organizations/invitations", charlieAuth.Token, nil)
	res, _ = client.Do(listCharlieInvites)
	var wrapCharlieInvites struct {
		Success bool `json:"success"`
		Data    struct {
			Invitations []models.PendingOrgInvitationView `json:"invitations"`
			Total       int                               `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapCharlieInvites)
	res.Body.Close()
	charlieInvites := wrapCharlieInvites.Data.Invitations
	respondCharlieReq, _ := orgAuthReq("POST", fmt.Sprintf("%s/api/organizations/invitations/%s/respond", server.URL, charlieInvites[0].InvitationID), charlieAuth.Token, models.RespondInvitationRequest{
		Accept: true,
	})
	res, _ = client.Do(respondCharlieReq)
	res.Body.Close()

	// 6. Cross-Organization Isolation: Charlie cannot access Beta Analytics
	charlieAccessBetaReq, _ := orgAuthReq("GET", fmt.Sprintf("%s/api/organizations/%s", server.URL, betaOrg.ID), charlieAuth.Token, nil)
	res, err = client.Do(charlieAccessBetaReq)
	if err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Charlie accessing Beta, got %d", res.StatusCode)
	}
	res.Body.Close()

	// 7. Organization Projects: Bob (as Editor) creates project in Alpha GeoTech
	createOrgProjReq, _ := orgAuthReq("POST", fmt.Sprintf("%s/api/organizations/%s/projects", server.URL, alphaOrg.ID), bobAuth.Token, models.CreateProjectRequest{
		Name:        "City Center 3D Survey",
		Description: "High resolution orthomosaic",
	})
	res, err = client.Do(createOrgProjReq)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("Bob failed creating project in Alpha GeoTech: %d", res.StatusCode)
	}
	var wrapCityProj struct {
		Success bool               `json:"success"`
		Data    models.UserProject `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapCityProj)
	res.Body.Close()
	cityProj := wrapCityProj.Data
	if cityProj.OrgID != alphaOrg.ID || cityProj.OwnerType != "organization" {
		t.Fatalf("project owner type or orgID mismatch: %+v", cityProj)
	}

	// 8. Charlie (Viewer) cannot modify or delete project
	charliePatchProj, _ := orgAuthReq("PATCH", fmt.Sprintf("%s/api/projects/%s", server.URL, cityProj.ID), charlieAuth.Token, map[string]string{
		"name": "Hacked Project Name",
	})
	res, err = client.Do(charliePatchProj)
	if err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for Charlie patching org project, got %d", res.StatusCode)
	}
	res.Body.Close()

	// 9. Bob cannot delete the organization (Admin/Editor cannot delete, only Owner can)
	bobDeleteOrgReq, _ := orgAuthReq("DELETE", fmt.Sprintf("%s/api/organizations/%s", server.URL, alphaOrg.ID), bobAuth.Token, nil)
	res, err = client.Do(bobDeleteOrgReq)
	if err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for Bob attempting to delete organization, got %d", res.StatusCode)
	}
	res.Body.Close()

	// 10. Project Transfer: Alice creates a personal project and transfers it to Alpha GeoTech
	createAlicePersonalProj, _ := orgAuthReq("POST", server.URL+"/api/projects", aliceAuth.Token, models.CreateProjectRequest{
		Name: "Alice Personal Drone Flight",
	})
	res, err = client.Do(createAlicePersonalProj)
	if err != nil || res.StatusCode != http.StatusCreated {
		t.Fatalf("Alice failed creating personal project: %d", res.StatusCode)
	}
	var wrapAlicePersonal struct {
		Success bool               `json:"success"`
		Data    models.UserProject `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapAlicePersonal)
	res.Body.Close()
	alicePersonal := wrapAlicePersonal.Data

	// Transfer Alice's project to Alpha GeoTech
	transferReq, _ := orgAuthReq("POST", fmt.Sprintf("%s/api/organizations/%s/transfer-project", server.URL, alphaOrg.ID), aliceAuth.Token, models.TransferProjectRequest{
		ProjectID: alicePersonal.ID,
	})
	res, err = client.Do(transferReq)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("Alice failed transferring project to org: %d", res.StatusCode)
	}
	var wrapTransferredProj struct {
		Success bool               `json:"success"`
		Data    models.UserProject `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapTransferredProj)
	res.Body.Close()
	transferredProj := wrapTransferredProj.Data
	if transferredProj.OrgID != alphaOrg.ID || transferredProj.OwnerType != "organization" {
		t.Fatalf("transferred project metadata invalid: %+v", transferredProj)
	}

	// 11. Role Update: Alice promotes Bob to Admin
	// Find Bob's member record in Alpha
	listMembersReq, _ := orgAuthReq("GET", fmt.Sprintf("%s/api/organizations/%s/members", server.URL, alphaOrg.ID), aliceAuth.Token, nil)
	res, err = client.Do(listMembersReq)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("failed listing org members: %d", res.StatusCode)
	}
	var wrapMembers struct {
		Success bool `json:"success"`
		Data    struct {
			Members []models.OrgMember `json:"members"`
			Total   int                `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapMembers)
	res.Body.Close()
	members := wrapMembers.Data.Members

	var bobMemberID string
	for _, m := range members {
		if m.UserID == bobAuth.User.ID {
			bobMemberID = m.ID
			break
		}
	}
	if bobMemberID == "" {
		t.Fatalf("Bob member ID not found in members list")
	}

	updateRoleReq, _ := orgAuthReq("PATCH", fmt.Sprintf("%s/api/organizations/%s/members/%s", server.URL, alphaOrg.ID, bobMemberID), aliceAuth.Token, models.UpdateOrgMemberRoleRequest{
		Role: models.OrgRoleAdmin,
	})
	res, err = client.Do(updateRoleReq)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("Alice failed promoting Bob to admin: %d", res.StatusCode)
	}
	res.Body.Close()

	// 12. Activity log inspection
	actReq, _ := orgAuthReq("GET", fmt.Sprintf("%s/api/organizations/%s/activities", server.URL, alphaOrg.ID), aliceAuth.Token, nil)
	res, err = client.Do(actReq)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("failed fetching activity logs: %d", res.StatusCode)
	}
	var wrapActs struct {
		Success bool `json:"success"`
		Data    struct {
			Activities []models.OrgActivity `json:"activities"`
			Total      int                  `json:"total"`
		} `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&wrapActs)
	res.Body.Close()
	acts := wrapActs.Data.Activities
	if len(acts) == 0 {
		t.Fatalf("expected recorded activities, got 0")
	}

	// 13. Owner deletes organization safely
	aliceDeleteOrgReq, _ := orgAuthReq("DELETE", fmt.Sprintf("%s/api/organizations/%s", server.URL, alphaOrg.ID), aliceAuth.Token, nil)
	res, err = client.Do(aliceDeleteOrgReq)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("Alice failed deleting organization: %d", res.StatusCode)
	}
	res.Body.Close()

	// Verify org is deleted
	verifyDelReq, _ := orgAuthReq("GET", fmt.Sprintf("%s/api/organizations/%s", server.URL, alphaOrg.ID), aliceAuth.Token, nil)
	res, err = client.Do(verifyDelReq)
	if err != nil || res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for deleted org, got %d", res.StatusCode)
	}
	res.Body.Close()
}
