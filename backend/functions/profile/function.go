package profile

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

const maxAvatarBytes = 5 * 1024 * 1024

type Handler struct {
	repo        repository.ProfileRepo
	mailer      services.Mailer
	storageRoot string
	publicURL   string
}

func NewHandler(repo repository.ProfileRepo, mailer services.Mailer, storageRoot, publicURL string) *Handler {
	return &Handler{repo: repo, mailer: mailer, storageRoot: storageRoot, publicURL: strings.TrimRight(publicURL, "/")}
}

func (h *Handler) Profile(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		p, err := h.repo.Get(r.Context(), userID)
		if err != nil {
			writeFailure(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, p)
	case http.MethodPatch:
		var req models.UpdateProfileRequest
		if !decode(w, r, &req) {
			return
		}
		req.FullName = strings.TrimSpace(req.FullName)
		if req.FullName == "" || len(utf16.Encode([]rune(req.FullName))) > 255 {
			invalid(w, "Full name must contain between 1 and 255 characters")
			return
		}
		p, err := h.repo.UpdateName(r.Context(), userID, req.FullName)
		if err != nil {
			writeFailure(w, err)
			return
		}
		models.WriteJSON(w, http.StatusOK, p)
	default:
		methodNotAllowed(w)
	}
}

func (h *Handler) Password(w http.ResponseWriter, r *http.Request, userID, currentToken string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req models.ChangePasswordRequest
	if !decode(w, r, &req) {
		return
	}
	if req.CurrentPassword == "" || len([]byte(req.CurrentPassword)) > 72 || len(req.NewPassword) < 6 || len([]byte(req.NewPassword)) > 72 {
		invalid(w, "Provide the current password and a new password of at least 6 characters; passwords must be at most 72 bytes")
		return
	}
	if err := h.repo.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword, currentToken); err != nil {
		writeFailure(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]bool{"password_changed": true})
}

func (h *Handler) EmailChange(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req models.ChangeEmailRequest
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email || len(req.Email) > 255 || !strings.Contains(req.Email, ".") {
		invalid(w, "A valid email address is required")
		return
	}
	raw, err := h.repo.CreateEmailVerification(r.Context(), userID, req.Email, "email_change")
	if err != nil {
		writeFailure(w, err)
		return
	}
	link := h.publicURL + "/verify-email?" + url.Values{"token": {raw}}.Encode()
	if err := h.mailer.SendVerification(r.Context(), services.VerificationEmail{To: req.Email, Link: link}); err != nil {
		models.WriteError(w, http.StatusBadGateway, "EMAIL_DELIVERY_FAILED", "Verification email could not be delivered; request another verification email", nil, nil)
		return
	}
	models.WriteJSON(w, http.StatusAccepted, map[string]bool{"verification_sent": true})
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req models.VerifyEmailRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.Token) != 64 {
		invalid(w, "Invalid or expired verification token")
		return
	}
	if err := h.repo.ConsumeEmailVerification(r.Context(), req.Token); err != nil {
		writeFailure(w, err)
		return
	}
	models.WriteJSON(w, http.StatusOK, map[string]bool{"email_verified": true})
}

func (h *Handler) Avatar(w http.ResponseWriter, r *http.Request, userID string) {
	if !filepath.IsLocal(userID) || strings.ContainsAny(userID, `/\:`) {
		invalid(w, "Invalid user ID")
		return
	}
	if r.Method == http.MethodGet {
		h.serveAvatar(w, r, userID)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+64*1024)
	if err := r.ParseMultipartForm(maxAvatarBytes + 64*1024); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			models.WriteError(w, 413, "AVATAR_TOO_LARGE", "Avatar must be at most 5 MiB", nil, nil)
		} else {
			invalid(w, "Invalid avatar upload")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("avatar")
	if err != nil {
		invalid(w, "An avatar file is required")
		return
	}
	defer file.Close()
	if header.Size > maxAvatarBytes {
		models.WriteError(w, 413, "AVATAR_TOO_LARGE", "Avatar must be at most 5 MiB", nil, nil)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		invalid(w, "Could not read avatar")
		return
	}
	if len(data) > maxAvatarBytes {
		models.WriteError(w, 413, "AVATAR_TOO_LARGE", "Avatar must be at most 5 MiB", nil, nil)
		return
	}
	var ext string
	switch http.DetectContentType(data) {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	default:
		invalid(w, "Avatar must be PNG, JPEG, or WebP")
		return
	}
	id, err := models.GenerateRandomHex(16)
	if err != nil {
		writeFailure(w, err)
		return
	}
	relative := filepath.Join("users", userID, "profile", id+ext)
	path := filepath.Join(h.storageRoot, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		writeFailure(w, err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		_ = os.Remove(path)
		writeFailure(w, err)
		return
	}
	oldPath, _ := h.repo.AvatarPath(r.Context(), userID)
	p, err := h.repo.SetAvatar(r.Context(), userID, filepath.ToSlash(relative))
	if err != nil {
		_ = os.Remove(path)
		writeFailure(w, err)
		return
	}
	if oldPath != "" {
		if old, ok := h.avatarDiskPath(userID, oldPath); ok {
			_ = os.Remove(old)
		}
	}
	models.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) avatarDiskPath(userID, relative string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	prefix := filepath.Join("users", userID, "profile") + string(filepath.Separator)
	if !filepath.IsLocal(clean) || !strings.HasPrefix(clean, prefix) {
		return "", false
	}
	return filepath.Join(h.storageRoot, clean), true
}

func (h *Handler) serveAvatar(w http.ResponseWriter, r *http.Request, userID string) {
	relative, err := h.repo.AvatarPath(r.Context(), userID)
	if err != nil {
		writeFailure(w, err)
		return
	}
	path, ok := h.avatarDiskPath(userID, relative)
	if !ok {
		models.WriteError(w, 404, "NOT_FOUND", "Avatar not found", nil, nil)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		invalid(w, "Invalid request body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		invalid(w, "Provide a single JSON object")
		return false
	}
	return true
}

func invalid(w http.ResponseWriter, message string) {
	models.WriteError(w, 400, "VALIDATION_FAILED", message, nil, nil)
}
func methodNotAllowed(w http.ResponseWriter) {
	models.WriteError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
}
func writeFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrVerificationRateLimited):
		w.Header().Set("Retry-After", "60")
		models.WriteError(w, 429, "VERIFICATION_RATE_LIMITED", "Wait one minute before requesting another verification email", nil, nil)
	case errors.Is(err, repository.ErrInvalidPassword):
		models.WriteError(w, 401, "INVALID_PASSWORD", "Current password is incorrect", nil, nil)
	case errors.Is(err, repository.ErrInvalidToken):
		models.WriteError(w, 400, "INVALID_TOKEN", "Invalid or expired token", nil, nil)
	case errors.Is(err, repository.ErrUserAlreadyExists):
		models.WriteError(w, 409, "EMAIL_EXISTS", "Email address is already in use", nil, nil)
	case errors.Is(err, repository.ErrUserNotFound):
		models.WriteError(w, 404, "NOT_FOUND", "Profile or avatar not found", nil, nil)
	default:
		models.WriteError(w, 500, "PROFILE_ERROR", "Could not update or retrieve profile", nil, nil)
	}
}
