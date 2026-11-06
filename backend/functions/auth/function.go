package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	authRepo  repository.AuthRepo
	profiles  repository.ProfileRepo
	mailer    services.Mailer
	publicURL string
}

// NewHandlerWithVerification is the production account-flow constructor.
func NewHandlerWithVerification(authRepo repository.AuthRepo, profiles repository.ProfileRepo, mailer services.Mailer, publicURL string) *Handler {
	return &Handler{authRepo: authRepo, profiles: profiles, mailer: mailer, publicURL: strings.TrimRight(publicURL, "/")}
}

func NewHandler(authRepo repository.AuthRepo) *Handler {
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

	user, err := h.authRepo.Register(r.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		if err == repository.ErrUserAlreadyExists {
			models.WriteError(w, http.StatusConflict, "USER_EXISTS", "A user with this email address already exists", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "REGISTRATION_ERROR", err.Error(), nil, nil)
		return
	}

	token, err := h.authRepo.CreateToken(r.Context(), user.ID, 30*24*time.Hour) // 30 days valid
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "TOKEN_ERROR", "Failed to generate session token", nil, nil)
		return
	}

	response := models.AuthResponse{
		Token: token.Token,
		User:  user.Public(),
	}
	if h.profiles != nil {
		response.VerificationDelivery = "sent"
		if err := h.sendInitialVerification(r.Context(), user.ID, user.Email); err != nil {
			response.VerificationDelivery = "failed"
			response.VerificationCode = "VERIFICATION_UNAVAILABLE"
			if errors.Is(err, services.ErrEmailDeliveryFailed) {
				response.VerificationCode = "EMAIL_DELIVERY_FAILED"
			}
		}
	}
	models.WriteJSON(w, http.StatusCreated, response)
}

func (h *Handler) sendInitialVerification(ctx context.Context, userID, email string) error {
	raw, err := h.profiles.CreateEmailVerification(ctx, userID, email, "initial_verification")
	if err != nil {
		return err
	}
	link := h.publicURL + "/verify-email?" + url.Values{"token": {raw}}.Encode()
	if err := h.mailer.SendVerification(ctx, services.VerificationEmail{To: email, Link: link}); err != nil {
		return services.ErrEmailDeliveryFailed
	}
	return nil
}

func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	if h.profiles == nil {
		models.WriteError(w, 503, "VERIFICATION_UNAVAILABLE", "Email verification is unavailable", nil, nil)
		return
	}
	err := h.sendInitialVerification(r.Context(), user.ID, user.Email)
	switch {
	case errors.Is(err, repository.ErrEmailAlreadyVerified):
		models.WriteJSON(w, 200, map[string]bool{"email_verified": true, "verification_sent": false})
	case errors.Is(err, repository.ErrVerificationRateLimited):
		w.Header().Set("Retry-After", "60")
		models.WriteError(w, 429, "VERIFICATION_RATE_LIMITED", "Wait one minute before requesting another verification email", nil, nil)
	case errors.Is(err, services.ErrEmailDeliveryFailed):
		models.WriteError(w, 502, "EMAIL_DELIVERY_FAILED", "Verification email could not be delivered; retry after one minute", nil, nil)
	case err != nil:
		models.WriteError(w, 500, "VERIFICATION_UNAVAILABLE", "Email verification is unavailable", nil, nil)
	default:
		models.WriteJSON(w, 202, map[string]bool{"verification_sent": true})
	}
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

	user, err := h.authRepo.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		models.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password", nil, nil)
		return
	}

	token, err := h.authRepo.CreateToken(r.Context(), user.ID, 30*24*time.Hour)
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
		if err := h.authRepo.RevokeToken(r.Context(), tokenStr); err != nil {
			models.WriteError(w, http.StatusInternalServerError, "LOGOUT_ERROR", "Failed to revoke session token", nil, nil)
			return
		}
	}
	models.WriteJSON(w, http.StatusOK, map[string]bool{"logged_out": true})
}

// Me returns the currently authenticated user's profile.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request, user *models.User) {
	models.WriteJSON(w, http.StatusOK, user.Public())
}
