package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	authRepo  repository.AuthRepo
	otpRepo   repository.OTPRepo
	profiles  repository.ProfileRepo
	mailer    services.Mailer
	publicURL string
}

// NewHandlerWithVerification is the production account-flow constructor.
func NewHandlerWithVerification(authRepo repository.AuthRepo, profiles repository.ProfileRepo, mailer services.Mailer, publicURL string) *Handler {
	return &Handler{
		authRepo:  authRepo,
		otpRepo:   repository.NewMemoryOTPRepository(),
		profiles:  profiles,
		mailer:    mailer,
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

func NewHandlerWithOTP(authRepo repository.AuthRepo, otpRepo repository.OTPRepo, profiles repository.ProfileRepo, mailer services.Mailer, publicURL string) *Handler {
	return &Handler{
		authRepo:  authRepo,
		otpRepo:   otpRepo,
		profiles:  profiles,
		mailer:    mailer,
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

func NewHandler(authRepo repository.AuthRepo) *Handler {
	return &Handler{authRepo: authRepo, otpRepo: repository.NewMemoryOTPRepository()}
}

func (h *Handler) SetOTPRepo(otpRepo repository.OTPRepo) {
	h.otpRepo = otpRepo
}

func (h *Handler) getOTPRepo() repository.OTPRepo {
	if h.otpRepo == nil {
		h.otpRepo = repository.NewMemoryOTPRepository()
	}
	return h.otpRepo
}

// SendOTP generates and dispatches a 6-digit numeric OTP to the requested email.
func (h *Handler) SendOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.SendOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !models.ValidateEmail(req.Email) {
		models.WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", "A valid email address is required", nil, nil)
		return
	}

	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	if purpose != "register" && purpose != "login" {
		purpose = "login"
	}
	req.Purpose = purpose

	if req.Purpose == "register" {
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			models.WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Full name is required for registration", nil, nil)
			return
		}
		existing, err := h.authRepo.FindUserByEmail(r.Context(), req.Email)
		if err == nil && existing != nil {
			models.WriteError(w, http.StatusConflict, "USER_EXISTS", "An account with this email address already exists. Please sign in.", nil, nil)
			return
		}
	} else if req.Purpose == "login" {
		existing, err := h.authRepo.FindUserByEmail(r.Context(), req.Email)
		if err != nil || existing == nil {
			models.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "No account found with this email address. Please register first.", nil, nil)
			return
		}
	}

	code, err := models.GenerateNumericOTP()
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "OTP_GENERATION_FAILED", "Failed to generate security code", nil, nil)
		return
	}

	_, err = h.getOTPRepo().CreateOTP(r.Context(), req.Email, req.Name, req.Purpose, code, 10*time.Minute, 60*time.Second)
	if err != nil {
		if errors.Is(err, repository.ErrOTPRateLimited) {
			w.Header().Set("Retry-After", "60")
			models.WriteError(w, http.StatusTooManyRequests, "OTP_RATE_LIMITED", "Please wait 60 seconds before requesting a new code", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "OTP_STORAGE_FAILED", "Failed to register verification code", nil, nil)
		return
	}

	// Log OTP in development console for fast developer testing
	log.Printf("[AUTH OTP] Verification code for %s (%s): %s", req.Email, req.Purpose, code)

	if h.mailer != nil {
		if err := h.mailer.SendOTP(r.Context(), services.OTPEmail{To: req.Email, Code: code, Purpose: req.Purpose}); err != nil {
			log.Printf("[MAILER WARNING] Provider delivery failed to %s: %v", req.Email, err)
			if os.Getenv("APP_ENV") != "production" {
				log.Printf("[DEV NOTICE] Provider delivery failed (e.g. Resend unverified recipient). In dev mode, use code '%s' to proceed.", code)
			} else {
				if errors.Is(err, services.ErrEmailDeliveryFailed) {
					models.WriteError(w, http.StatusBadGateway, "EMAIL_DELIVERY_FAILED", "Verification email could not be delivered; please retry shortly", nil, nil)
					return
				}
				models.WriteError(w, http.StatusInternalServerError, "EMAIL_DELIVERY_FAILED", "Failed to dispatch verification email", nil, nil)
				return
			}
		}
	}

	models.WriteJSON(w, http.StatusOK, models.SendOTPResponse{
		Success:            true,
		Email:              req.Email,
		Purpose:            req.Purpose,
		ResendAfterSeconds: 60,
		Message:            "Verification code sent to your email",
	})
}

// VerifyOTP checks the 6-digit code and authenticates the user.
func (h *Handler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req models.VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", nil, nil)
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	if !models.ValidateEmail(req.Email) || len(req.Code) != 6 {
		models.WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", "A valid email address and 6-digit code are required", nil, nil)
		return
	}

	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	if purpose != "register" && purpose != "login" {
		purpose = "login"
	}
	req.Purpose = purpose

	rec, err := h.getOTPRepo().VerifyAndConsumeOTP(r.Context(), req.Email, req.Code, req.Purpose)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrOTPExpiredOrNotFound):
			models.WriteError(w, http.StatusBadRequest, "OTP_EXPIRED", "Verification code has expired or was not found. Please request a new code.", nil, nil)
		case errors.Is(err, repository.ErrOTPMaxAttempts):
			models.WriteError(w, http.StatusTooManyRequests, "OTP_MAX_ATTEMPTS", "Maximum verification attempts exceeded. Please request a new code.", nil, nil)
		case errors.Is(err, repository.ErrOTPInvalid):
			models.WriteError(w, http.StatusBadRequest, "INVALID_OTP", "Invalid verification code. Please check your email and try again.", nil, nil)
		default:
			models.WriteError(w, http.StatusInternalServerError, "OTP_VERIFICATION_FAILED", err.Error(), nil, nil)
		}
		return
	}

	var user *models.User
	if req.Purpose == "register" {
		name := strings.TrimSpace(rec.Name)
		if name == "" {
			name = strings.TrimSpace(req.Name)
		}
		if name == "" {
			parts := strings.Split(req.Email, "@")
			name = parts[0]
		}
		u, err := h.authRepo.RegisterWithoutPassword(r.Context(), name, req.Email)
		if err != nil {
			if errors.Is(err, repository.ErrUserAlreadyExists) {
				u, err = h.authRepo.FindUserByEmail(r.Context(), req.Email)
				if err != nil {
					models.WriteError(w, http.StatusInternalServerError, "USER_FETCH_ERROR", "Failed to retrieve user record", nil, nil)
					return
				}
			} else {
				models.WriteError(w, http.StatusInternalServerError, "REGISTRATION_ERROR", err.Error(), nil, nil)
				return
			}
		}
		user = u
	} else {
		u, err := h.authRepo.FindUserByEmail(r.Context(), req.Email)
		if err != nil || u == nil {
			models.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "User account not found", nil, nil)
			return
		}
		user = u
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

	user, token, err := h.authRepo.AuthenticateAndCreateToken(r.Context(), req.Email, req.Password, 30*24*time.Hour)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidPassword) {
			models.WriteError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password", nil, nil)
		} else {
			models.WriteError(w, http.StatusInternalServerError, "AUTHENTICATION_ERROR", "Could not authenticate or issue a session token", nil, nil)
		}
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
