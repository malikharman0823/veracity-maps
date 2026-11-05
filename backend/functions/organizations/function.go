package organizations

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type Handler struct {
	orgRepo     *repository.OrgRepository
	projectRepo *repository.ProjectRepository
	authRepo    *repository.AuthRepository
}

func NewHandler(
	orgRepo *repository.OrgRepository,
	projectRepo *repository.ProjectRepository,
	authRepo *repository.AuthRepository,
) *Handler {
	return &Handler{
		orgRepo:     orgRepo,
		projectRepo: projectRepo,
		authRepo:    authRepo,
	}
}

// List handles GET /api/organizations (organizations where user is a member)
func (h *Handler) List(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	orgs := h.orgRepo.ListByUser(userID)
	if orgs == nil {
		orgs = make([]*models.Organization, 0)
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"organizations": orgs,
		"total":         len(orgs),
	})
}

// Create handles POST /api/organizations
func (h *Handler) Create(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.CreateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		models.WriteError(w, http.StatusBadRequest, "INVALID_NAME", "Organization name cannot be empty", nil, nil)
		return
	}

	org, err := h.orgRepo.Create(user, req.Name, req.Description)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "CREATION_FAILED", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusCreated, org)
}

// Get handles GET /api/organizations/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	org, role, err := h.orgRepo.GetByID(orgID, user.ID)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have access to this organization", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Organization not found", nil, nil)
		return
	}

	// Calculate overview metrics
	members, _ := h.orgRepo.ListMembers(orgID, user.ID)
	orgProjects := h.projectRepo.ListByOrg(orgID)

	overview := models.OrgOverviewData{
		Organization:      org,
		UserRole:          role,
		TotalMembers:      len(members),
		TotalProjects:     len(orgProjects),
		TotalDatasets:     0,
		StorageUsedBytes:  org.StorageUsedBytes,
		StorageQuotaBytes: org.StorageQuotaBytes,
	}

	for _, p := range orgProjects {
		overview.TotalDatasets += len(p.DatasetIDs)
	}

	models.WriteJSON(w, http.StatusOK, overview)
}

// Update handles PATCH /api/organizations/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.UpdateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	org, err := h.orgRepo.Update(orgID, user.ID, req.Name, req.Description, user)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only organization owners and admins can update settings", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Organization not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, org)
}

// Delete handles DELETE /api/organizations/{id} (Owner ONLY)
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	err := h.orgRepo.Delete(orgID, user.ID)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the organization owner can delete this organization", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Organization not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ListMembers handles GET /api/organizations/{id}/members
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	members, err := h.orgRepo.ListMembers(orgID, user.ID)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to view members of this organization", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Organization not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"members": members,
		"total":   len(members),
	})
}

// Invite handles POST /api/organizations/{id}/invite
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.InviteOrgMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") {
		models.WriteError(w, http.StatusBadRequest, "INVALID_EMAIL", "A valid email address is required", nil, nil)
		return
	}

	targetUser, err := h.authRepo.FindUserByEmail(cleanEmail)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "No registered user found with that email address", nil, nil)
		return
	}

	member, err := h.orgRepo.InviteMember(orgID, user, targetUser, req.Role)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions to invite members", nil, nil)
			return
		}
		if err == repository.ErrOrgMemberAlreadyExists {
			models.WriteError(w, http.StatusConflict, "MEMBER_EXISTS", "User already has active or pending membership in this organization", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "INVITE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusCreated, member)
}

// UpdateMemberRole handles PATCH /api/organizations/{id}/members/{member_id}
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request, orgID, memberID string, user *models.User) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.UpdateOrgMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	updated, err := h.orgRepo.UpdateMemberRole(orgID, user, memberID, req.Role)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions to change member roles", nil, nil)
			return
		}
		if err == repository.ErrCannotModifyOrgOwner {
			models.WriteError(w, http.StatusBadRequest, "CANNOT_MODIFY_OWNER", "Cannot change role of organization owner", nil, nil)
			return
		}
		if err == repository.ErrOrgMemberNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Member not found", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "UPDATE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, updated)
}

// RemoveMember handles DELETE /api/organizations/{id}/members/{member_id}
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request, orgID, memberID string, user *models.User) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	err := h.orgRepo.RemoveMember(orgID, user, memberID)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions to remove this member", nil, nil)
			return
		}
		if err == repository.ErrCannotRemoveOrgOwner {
			models.WriteError(w, http.StatusBadRequest, "CANNOT_REMOVE_OWNER", "Cannot remove organization owner", nil, nil)
			return
		}
		if err == repository.ErrOrgMemberNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Member not found", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "REMOVE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

// ListInvitations handles GET /api/organizations/invitations
func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	invites := h.orgRepo.ListPendingInvitations(user)
	if invites == nil {
		invites = make([]*models.PendingOrgInvitationView, 0)
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"invitations": invites,
		"total":       len(invites),
	})
}

// RespondInvitation handles POST /api/organizations/invitations/{id}/respond
func (h *Handler) RespondInvitation(w http.ResponseWriter, r *http.Request, invitationID string, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.RespondInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	err := h.orgRepo.RespondInvitation(invitationID, user, req.Accept)
	if err != nil {
		if err == repository.ErrOrgInvitationNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Invitation not found or expired", nil, nil)
			return
		}
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You are not authorized to respond to this invitation", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "INVITATION_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"accepted": req.Accept,
	})
}

// ListActivities handles GET /api/organizations/{id}/activities
func (h *Handler) ListActivities(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	activities, err := h.orgRepo.ListActivities(orgID, user.ID)
	if err != nil {
		if err == repository.ErrOrgForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only organization owners and admins can view activity history", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Organization not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"activities": activities,
		"total":      len(activities),
	})
}

// ListProjects handles GET /api/organizations/{id}/projects
func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	_, ok := h.orgRepo.GetUserRole(orgID, user.ID)
	if !ok {
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have access to this organization", nil, nil)
		return
	}

	projects := h.projectRepo.ListByOrg(orgID)
	if projects == nil {
		projects = make([]*models.UserProject, 0)
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"projects": projects,
		"total":    len(projects),
	})
}

// CreateProject handles POST /api/organizations/{id}/projects
func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	role, ok := h.orgRepo.GetUserRole(orgID, user.ID)
	if !ok || role == models.OrgRoleViewer {
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Viewers cannot create projects in this organization", nil, nil)
		return
	}

	var req models.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "Untitled Org Project"
	}

	p, err := h.projectRepo.Create(user.ID, req.Name, req.Description, user.Name, user.Email, orgID)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "PROJECT_CREATION_FAILED", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusCreated, p)
}

// TransferProject handles POST /api/organizations/{id}/transfer-project
func (h *Handler) TransferProject(w http.ResponseWriter, r *http.Request, orgID string, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	role, ok := h.orgRepo.GetUserRole(orgID, user.ID)
	if !ok || (role != models.OrgRoleOwner && role != models.OrgRoleAdmin) {
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only organization owners and admins can transfer projects into the organization", nil, nil)
		return
	}

	var req models.TransferProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	transferred, err := h.projectRepo.TransferToOrg(req.ProjectID, user.ID, orgID, user.Name)
	if err != nil {
		if err == repository.ErrForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You can only transfer projects that you personally own", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, transferred)
}
