package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	sessionRepo *repository.SessionRepository
	projectRepo *repository.ProjectRepository
	authRepo    repository.AuthRepo
}

func NewHandler(repo repository.ProfileRepo, mailer services.Mailer, storageRoot, publicURL string) *Handler {
	return &Handler{repo: repo, mailer: mailer, storageRoot: storageRoot, publicURL: strings.TrimRight(publicURL, "/")}
}

func (h *Handler) SetRepositories(sessionRepo *repository.SessionRepository, projectRepo *repository.ProjectRepository, authRepo repository.AuthRepo) {
	h.sessionRepo = sessionRepo
	h.projectRepo = projectRepo
	h.authRepo = authRepo
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

type ProfileDashboardStats struct {
	TotalProjects      int    `json:"total_projects"`
	SharedProjects     int    `json:"shared_projects"`
	TotalFiles         int    `json:"total_files"`
	GeoreferencedFiles int    `json:"georeferenced_files"`
	PendingInvitations int    `json:"pending_invitations"`
	TotalSessions      int    `json:"total_sessions"`
	TotalMemory        string `json:"total_memory"`
}

type UserFileView struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"session_id"`
	Filename           string    `json:"filename"`
	FileSize           int64     `json:"file_size"`
	MemorySize         string    `json:"memory_size"`
	RAMEstimate        string    `json:"ram_estimate,omitempty"`
	ContentType        string    `json:"content_type"`
	GeoreferenceStatus string    `json:"georeference_status"`
	TileStatus         string    `json:"tile_status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Width              int       `json:"width,omitempty"`
	Height             int       `json:"height,omitempty"`
	CRS                string    `json:"crs,omitempty"`
	Format             string    `json:"format,omitempty"`
	ProjectID          string    `json:"project_id,omitempty"`
	ProjectName        string    `json:"project_name,omitempty"`
	DownloadURL        string    `json:"download_url,omitempty"`
}

type ProfileDashboardResponse struct {
	User               *models.UserPublicProfile       `json:"user"`
	Stats              ProfileDashboardStats           `json:"stats"`
	Projects           []*models.UserProject           `json:"projects"`
	SharedProjects     []*models.SharedProjectView     `json:"shared_projects"`
	Files              []UserFileView                  `json:"files"`
	PendingInvitations []*models.PendingInvitationView `json:"pending_invitations"`
	Sessions           []*repository.SessionSummary    `json:"sessions,omitempty"`
}

func (h *Handler) getUserProfile(r *http.Request, userID string) (*models.UserPublicProfile, error) {
	if h.repo != nil {
		p, err := h.repo.Get(r.Context(), userID)
		if err == nil && p != nil {
			return p, nil
		}
	}
	if h.authRepo != nil {
		u, err := h.authRepo.GetUserByID(r.Context(), userID)
		if err == nil && u != nil {
			return &models.UserPublicProfile{
				ID:            u.ID,
				FullName:      u.Name,
				Email:         u.Email,
				EmailVerified: true,
				AccountStatus: "active",
				CreatedAt:     u.CreatedAt,
				UpdatedAt:     u.UpdatedAt,
			}, nil
		}
	}
	return nil, repository.ErrUserNotFound
}

func formatMemoryBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	userProf, err := h.getUserProfile(r, userID)
	if err != nil {
		writeFailure(w, err)
		return
	}

	var ownedProjects []*models.UserProject
	var sharedProjects []*models.SharedProjectView
	var pendingInvs []*models.PendingInvitationView

	if h.projectRepo != nil {
		ownedProjects = h.projectRepo.ListByUser(userID)
		sharedProjects = h.projectRepo.ListSharedWithUser(userID, h.authRepo)
		pendingInvs = h.projectRepo.ListPendingInvitations(userID, userProf.Email, h.authRepo)
	}
	if ownedProjects == nil {
		ownedProjects = make([]*models.UserProject, 0)
	}
	if sharedProjects == nil {
		sharedProjects = make([]*models.SharedProjectView, 0)
	}
	if pendingInvs == nil {
		pendingInvs = make([]*models.PendingInvitationView, 0)
	}

	files := h.collectUserFiles(userID, ownedProjects, sharedProjects)
	georefCount := 0
	var totalMemoryBytes int64
	for _, f := range files {
		totalMemoryBytes += f.FileSize
		if f.GeoreferenceStatus == "completed" {
			georefCount++
		}
	}

	var sessionsList []*repository.SessionSummary
	if h.sessionRepo != nil {
		sessionsList = h.sessionRepo.ListSessions()
	}
	if sessionsList == nil {
		sessionsList = make([]*repository.SessionSummary, 0)
	}

	stats := ProfileDashboardStats{
		TotalProjects:      len(ownedProjects),
		SharedProjects:     len(sharedProjects),
		TotalFiles:         len(files),
		GeoreferencedFiles: georefCount,
		PendingInvitations: len(pendingInvs),
		TotalSessions:      len(sessionsList),
		TotalMemory:        formatMemoryBytes(totalMemoryBytes),
	}

	resp := ProfileDashboardResponse{
		User:               userProf,
		Stats:              stats,
		Projects:           ownedProjects,
		SharedProjects:     sharedProjects,
		Files:              files,
		PendingInvitations: pendingInvs,
		Sessions:           sessionsList,
	}

	models.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) collectUserFiles(userID string, ownedProjects []*models.UserProject, sharedProjects []*models.SharedProjectView) []UserFileView {
	if h.sessionRepo == nil {
		return make([]UserFileView, 0)
	}

	projMap := make(map[string]*models.UserProject)
	for _, p := range ownedProjects {
		if p.PrimaryDatasetID != "" {
			projMap[p.PrimaryDatasetID] = p
		}
		for _, dsID := range p.DatasetIDs {
			projMap[dsID] = p
		}
	}
	for _, sp := range sharedProjects {
		if sp.Project != nil {
			if sp.Project.PrimaryDatasetID != "" {
				projMap[sp.Project.PrimaryDatasetID] = sp.Project
			}
			for _, dsID := range sp.Project.DatasetIDs {
				projMap[dsID] = sp.Project
			}
		}
	}

	allDatasets := h.sessionRepo.ListAllDatasets()
	files := make([]UserFileView, 0)

	for _, ds := range allDatasets {
		if ds == nil {
			continue
		}
		proj, inProj := projMap[ds.ID]
		ownerID, _ := ds.ExtractedMetadata["user_id"].(string)
		isOwner := ownerID == userID

		// Include if associated with project, explicitly tagged with userID,
		// or if system has no projects created yet (convenient local dev mapping)
		if inProj || isOwner || (len(ownedProjects) == 0 && len(sharedProjects) == 0) {
			view := UserFileView{
				ID:                 ds.ID,
				SessionID:          ds.SessionID,
				Filename:           ds.Filename,
				FileSize:           ds.FileSize,
				MemorySize:         formatMemoryBytes(ds.FileSize),
				ContentType:        ds.ContentType,
				GeoreferenceStatus: ds.GeoreferenceStatus,
				TileStatus:         ds.TileStatus,
				CreatedAt:          ds.CreatedAt,
				UpdatedAt:          ds.UpdatedAt,
				DownloadURL:        "/sessions/" + ds.SessionID + "/datasets/" + ds.ID + "/raw",
			}
			if proj != nil {
				view.ProjectID = proj.ID
				view.ProjectName = proj.Name
			}
			if ds.ExtractedMetadata != nil {
				if w, ok := ds.ExtractedMetadata["width"].(float64); ok {
					view.Width = int(w)
				} else if w, ok := ds.ExtractedMetadata["width"].(int); ok {
					view.Width = w
				}
				if hVal, ok := ds.ExtractedMetadata["height"].(float64); ok {
					view.Height = int(hVal)
				} else if hVal, ok := ds.ExtractedMetadata["height"].(int); ok {
					view.Height = hVal
				}
				if crs, ok := ds.ExtractedMetadata["crs"].(string); ok {
					view.CRS = crs
				}
				if fmt, ok := ds.ExtractedMetadata["format"].(string); ok {
					view.Format = strings.ToUpper(fmt)
				}
			}
			if view.Width > 0 && view.Height > 0 {
				view.RAMEstimate = formatMemoryBytes(int64(view.Width) * int64(view.Height) * 4)
			}
			if view.Format == "" {
				ext := strings.TrimPrefix(filepath.Ext(ds.Filename), ".")
				view.Format = strings.ToUpper(ext)
			}
			files = append(files, view)
		}
	}
	return files
}

func (h *Handler) Files(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	var ownedProjects []*models.UserProject
	var sharedProjects []*models.SharedProjectView
	if h.projectRepo != nil {
		ownedProjects = h.projectRepo.ListByUser(userID)
		sharedProjects = h.projectRepo.ListSharedWithUser(userID, h.authRepo)
	}
	files := h.collectUserFiles(userID, ownedProjects, sharedProjects)
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"files": files,
		"count": len(files),
	})
}

func (h *Handler) FileItem(w http.ResponseWriter, r *http.Request, userID, fileID string) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if fileID == "" {
		invalid(w, "File ID is required")
		return
	}
	if h.sessionRepo == nil {
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "File not found", nil, nil)
		return
	}

	allDatasets := h.sessionRepo.ListAllDatasets()
	var targetDS *models.Dataset
	for _, ds := range allDatasets {
		if ds.ID == fileID {
			targetDS = ds
			break
		}
	}
	if targetDS == nil {
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "File not found", nil, nil)
		return
	}

	_ = h.sessionRepo.DeleteDataset(targetDS.SessionID, targetDS.ID)
	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": true,
		"id":      fileID,
	})
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
