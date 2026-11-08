package projects

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	projectRepo  *repository.ProjectRepository
	sessionRepo  *repository.SessionRepository
	authRepo     repository.AuthRepo
	mailer       services.Mailer
	appPublicURL string
}

func NewHandler(
	projectRepo *repository.ProjectRepository,
	sessionRepo *repository.SessionRepository,
	authRepo repository.AuthRepo,
	mailer services.Mailer,
	appPublicURL string,
) *Handler {
	return &Handler{
		projectRepo:  projectRepo,
		sessionRepo:  sessionRepo,
		authRepo:     authRepo,
		mailer:       mailer,
		appPublicURL: strings.TrimRight(appPublicURL, "/"),
	}
}

// List handles GET /api/projects (owned projects)
func (h *Handler) List(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	projs := h.projectRepo.ListByUser(userID)
	if projs == nil {
		projs = make([]*models.UserProject, 0)
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"projects": projs,
		"total":    len(projs),
	})
}

// ListShared handles GET /api/projects/shared (projects shared with user)
func (h *Handler) ListShared(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	views := h.projectRepo.ListSharedWithUser(userID, h.authRepo)
	if views == nil {
		views = make([]*models.SharedProjectView, 0)
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"shared_projects": views,
		"total":           len(views),
	})
}

// ListInvitations handles GET /api/projects/invitations
func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	invites := h.projectRepo.ListPendingInvitations(user.ID, user.Email, h.authRepo)
	if invites == nil {
		invites = make([]*models.PendingInvitationView, 0)
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"invitations": invites,
		"total":       len(invites),
	})
}

// RespondInvitation handles POST /api/projects/invitations/{id}/respond
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

	err := h.projectRepo.RespondInvitation(invitationID, user.ID, user.Name, req.Accept)
	if err != nil {
		if err == repository.ErrInvitationNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Invitation not found or expired", nil, nil)
			return
		}
		if err == repository.ErrForbidden {
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

// Create handles POST /api/projects
func (h *Handler) Create(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "Untitled Project"
	}

	p, err := h.projectRepo.Create(user.ID, req.Name, req.Description, user.Name, user.Email)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "PROJECT_CREATION_FAILED", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusCreated, p)
}

// Get handles GET /api/projects/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request, projectID, userID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	p, role, err := h.projectRepo.GetByID(projectID, userID)
	if err != nil {
		if err == repository.ErrUnauthorized {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to access this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"project": p,
		"role":    role,
	})
}

// Update handles PATCH /api/projects/{id} or PUT /api/projects/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request, projectID, userID string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	p, err := h.projectRepo.Update(projectID, userID, req.Name, req.Description, h.authRepo)
	if err != nil {
		if err == repository.ErrForbidden || err == repository.ErrUnauthorized {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to modify this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, p)
}

// Delete handles DELETE /api/projects/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request, projectID, userID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	err := h.projectRepo.Delete(projectID, userID)
	if err != nil {
		if err == repository.ErrForbidden || err == repository.ErrUnauthorized {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the project owner can delete this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// Share handles POST /api/projects/{id}/share (Owner invites collaborator)
func (h *Handler) Share(w http.ResponseWriter, r *http.Request, projectID string, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.ShareProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") {
		models.WriteError(w, http.StatusBadRequest, "INVALID_EMAIL", "A valid email address is required", nil, nil)
		return
	}

	targetUser, err := h.authRepo.FindUserByEmail(r.Context(), cleanEmail)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "No registered user found with that email address", nil, nil)
		return
	}

	if req.Role != models.RoleEditor && req.Role != models.RoleViewer {
		req.Role = models.RoleViewer
	}

	member, err := h.projectRepo.InviteMember(projectID, user.ID, targetUser, req.Role, user.Name)
	if err != nil {
		if err == repository.ErrForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the project owner can share this project", nil, nil)
			return
		}
		if err == repository.ErrCannotInviteOwner {
			models.WriteError(w, http.StatusBadRequest, "CANNOT_INVITE_OWNER", "Cannot invite the owner of the project", nil, nil)
			return
		}
		if err == repository.ErrMemberAlreadyExists {
			models.WriteError(w, http.StatusConflict, "COLLABORATOR_EXISTS", "User already has active or pending access to this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "INVITE_ERROR", err.Error(), nil, nil)
		return
	}

	// Dispatch invitation email via configured mailer
	if h.mailer != nil {
		projName := "Veracity Maps Project"
		if p, _, pErr := h.projectRepo.GetByID(projectID, user.ID); pErr == nil && p != nil && p.Name != "" {
			projName = p.Name
		}
		publicBase := h.appPublicURL
		if publicBase == "" {
			publicBase = "http://localhost:5173"
		}
		inviteLink := fmt.Sprintf("%s/dashboard?tab=invitations", strings.TrimRight(publicBase, "/"))
		invMsg := services.InvitationEmail{
			To:          cleanEmail,
			Link:        inviteLink,
			ProjectName: projName,
			InviterName: user.Name,
			Role:        string(req.Role),
		}
		if sendErr := h.mailer.SendInvitation(r.Context(), invMsg); sendErr != nil {
			log.Printf("[INVITATION ERROR] Failed to deliver project invitation email to %s: %v", cleanEmail, sendErr)
		} else {
			log.Printf("[INVITATION SUCCESS] Dispatched project invitation email to %s for project '%s'", cleanEmail, projName)
		}
	}

	models.WriteJSON(w, http.StatusCreated, member)
}

// ListMembers handles GET /api/projects/{id}/members
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request, projectID, userID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	members, err := h.projectRepo.ListMembers(projectID, userID)
	if err != nil {
		if err == repository.ErrUnauthorized {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to view members of this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"members": members,
		"total":   len(members),
	})
}

// UpdateMemberRole handles PATCH /api/projects/{id}/members/{member_id}
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request, projectID, memberID string, user *models.User) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	updated, err := h.projectRepo.UpdateMemberRole(projectID, user.ID, memberID, req.Role, user.Name)
	if err != nil {
		if err == repository.ErrForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the project owner can change permissions", nil, nil)
			return
		}
		if err == repository.ErrCannotModifyOwner {
			models.WriteError(w, http.StatusBadRequest, "CANNOT_MODIFY_OWNER", "Cannot change role of project owner", nil, nil)
			return
		}
		if err == repository.ErrMemberNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Collaborator not found", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "UPDATE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, updated)
}

// RemoveMember handles DELETE /api/projects/{id}/members/{member_id}
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request, projectID, memberID string, user *models.User) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	err := h.projectRepo.RemoveMember(projectID, user.ID, memberID, user.Name)
	if err != nil {
		if err == repository.ErrForbidden {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the project owner can remove collaborators", nil, nil)
			return
		}
		if err == repository.ErrCannotModifyOwner {
			models.WriteError(w, http.StatusBadRequest, "CANNOT_MODIFY_OWNER", "Cannot remove project owner", nil, nil)
			return
		}
		if err == repository.ErrMemberNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Collaborator not found", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "REMOVE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

// ListActivities handles GET /api/projects/{id}/activities
func (h *Handler) ListActivities(w http.ResponseWriter, r *http.Request, projectID, userID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	activities, err := h.projectRepo.ListActivities(projectID, userID)
	if err != nil {
		if err == repository.ErrUnauthorized {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to view activity for this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"activities": activities,
		"total":      len(activities),
	})
}
