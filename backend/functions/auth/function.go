package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
)

type Handler struct {
	authRepo *repository.AuthRepository
}

func NewHandler(authRepo *repository.AuthRepository) *Handler {
	return &Handler{authRepo: authRepo}
}

// Register creates a new user and logs them in.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	if err := models.ValidateRegisterRequest(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), nil, nil)
		return
	}

	user, err := h.authRepo.Register(req.Name, req.Email, req.Password)
	if err != nil {
		if err == repository.ErrUserAlreadyExists {
			models.WriteError(w, http.StatusConflict, "USER_EXISTS", "A user with this email address already exists", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "REGISTRATION_ERROR", err.Error(), nil, nil)
		return
	}

	token, err := h.authRepo.CreateToken(user.ID, 30*24*time.Hour) // 30 days valid
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "TOKEN_ERROR", "Failed to generate session token", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusCreated, models.AuthResponse{
		Token: token.Token,
		User:  user.Public(),
	})
}

// Login verifies credentials and returns a session token.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	user, err := h.authRepo.Authenticate(req.Email, req.Password)
	if err != nil {
		models.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password", nil, nil)
		return
	}

	token, err := h.authRepo.CreateToken(user.ID, 30*24*time.Hour)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "TOKEN_ERROR", "Failed to generate session token", nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, models.AuthResponse{
		Token: token.Token,
		User:  user.Public(),
	})
}

// Logout revokes the current bearer token.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request, tokenStr string) {
	if tokenStr != "" {
		h.authRepo.RevokeToken(tokenStr)
	}
	models.WriteJSON(w, http.StatusOK, map[string]bool{"logged_out": true})
}

// Me returns the currently authenticated user's profile.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request, user *models.User) {
	models.WriteJSON(w, http.StatusOK, user.Public())
}
