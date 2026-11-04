package projects

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type Handler struct {
	projectRepo *repository.ProjectRepository
	sessionRepo *repository.SessionRepository
}

func NewHandler(projectRepo *repository.ProjectRepository, sessionRepo *repository.SessionRepository) *Handler {
	return &Handler{
		projectRepo: projectRepo,
		sessionRepo: sessionRepo,
	}
}

// List handles GET /api/projects
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

// Create handles POST /api/projects
func (h *Handler) Create(w http.ResponseWriter, r *http.Request, userID string) {
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

	p, err := h.projectRepo.Create(userID, req.Name, req.Description)
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

	p, err := h.projectRepo.GetByID(projectID, userID)
	if err != nil {
		if err == repository.ErrUnauthorized {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to access this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, p)
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

	p, err := h.projectRepo.Update(projectID, userID, req.Name, req.Description)
	if err != nil {
		if err == repository.ErrUnauthorized {
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
		if err == repository.ErrUnauthorized {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to delete this project", nil, nil)
			return
		}
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Project not found", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
