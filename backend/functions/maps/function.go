package maps

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/repository"
	"github.com/malikharman0823/veracity-maps/services"
)

type Handler struct {
	mapRepo              repository.MapRepo
	configurationService *services.MapConfigurationService
	mailer               services.Mailer
	appPublicURL         string
	storageDir           string
}

func (h *Handler) SetConfigurationService(service *services.MapConfigurationService) {
	h.configurationService = service
}

func NewHandler(mapRepo repository.MapRepo, mailer services.Mailer, appPublicURL string) *Handler {
	return &Handler{
		mapRepo:      mapRepo,
		mailer:       mailer,
		appPublicURL: strings.TrimRight(appPublicURL, "/"),
		storageDir:   "storage",
	}
}

func (h *Handler) GetMapRepo() repository.MapRepo {
	return h.mapRepo
}

func generateSecureToken(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("tok_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// ListMaps handles GET /api/maps
// Returns maps owned by user and maps shared with user.
func (h *Handler) ListMaps(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	myMaps, err := h.mapRepo.ListMyMaps(r.Context(), user.ID)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LIST_MAPS_ERROR", err.Error(), nil, nil)
		return
	}
	if myMaps == nil {
		myMaps = []*models.Map{}
	}

	sharedWithMe, err := h.mapRepo.ListSharedWithMe(r.Context(), user.ID)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LIST_SHARED_MAPS_ERROR", err.Error(), nil, nil)
		return
	}
	if sharedWithMe == nil {
		sharedWithMe = []*models.Map{}
	}

	// For backwards compatibility and convenience, maps combines myMaps + sharedWithMe
	combined := append([]*models.Map{}, myMaps...)
	combined = append(combined, sharedWithMe...)
	savedMaps := append([]*models.Map{}, myMaps...)
	for _, sharedMap := range sharedWithMe {
		if sharedMap.SavedByCurrentUser {
			savedMaps = append(savedMaps, sharedMap)
		}
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"my_maps":        myMaps,
		"shared_with_me": sharedWithMe,
		"saved_maps":     savedMaps,
		"maps":           combined,
		"total":          len(combined),
	})
}

// CreateMap handles POST /api/maps
func (h *Handler) CreateMap(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		models.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body", nil, nil)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		models.WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Map name is required", nil, nil)
		return
	}

	m, err := h.mapRepo.CreateMap(r.Context(), user.ID, name, req.Description)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "CREATE_MAP_ERROR", err.Error(), nil, nil)
		return
	}
	m.OwnerUserName = user.Name
	m.OwnerUserEmail = user.Email

	models.WriteJSON(w, http.StatusCreated, m)
}

// CreateMapFromConfiguration handles POST /api/maps/from-configuration.
func (h *Handler) CreateMapFromConfiguration(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}
	if h.configurationService == nil {
		models.WriteError(w, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "Durable map persistence is unavailable", nil, nil)
		return
	}

	var req struct {
		Name             string          `json:"name"`
		Description      string          `json:"description"`
		PrimaryDatasetID string          `json:"primary_dataset_id"`
		Members          []string        `json:"members"`
		Configuration    json.RawMessage `json:"configuration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body", nil, nil)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.PrimaryDatasetID = strings.TrimSpace(req.PrimaryDatasetID)
	if req.Name == "" || req.PrimaryDatasetID == "" || len(req.Configuration) == 0 || !json.Valid(req.Configuration) {
		models.WriteError(w, http.StatusUnprocessableEntity, "INVALID_CONFIGURATION", "Name, primary dataset, and valid configuration are required", nil, nil)
		return
	}

	snapshot, err := json.Marshal(struct {
		Name             string          `json:"name"`
		PrimaryDatasetID string          `json:"primary_dataset_id"`
		Members          []string        `json:"members"`
		Configuration    json.RawMessage `json:"configuration"`
	}{
		Name:             req.Name,
		PrimaryDatasetID: req.PrimaryDatasetID,
		Members:          req.Members,
		Configuration:    req.Configuration,
	})
	if err != nil {
		models.WriteError(w, http.StatusUnprocessableEntity, "INVALID_CONFIGURATION", "Configuration could not be serialized", nil, nil)
		return
	}

	result, err := h.configurationService.CreatePersonalMapFromConfiguration(r.Context(), user, models.MapConfigurationCommit{
		Name:              req.Name,
		Description:       req.Description,
		PrimaryDatasetID:  req.PrimaryDatasetID,
		MemberAssetIDs:    req.Members,
		ConfigurationJSON: string(snapshot),
	})
	if err != nil {
		writeMapConfigurationError(w, err)
		return
	}
	models.WriteJSON(w, http.StatusCreated, result)
}

func writeMapConfigurationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrMapAccessDenied):
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to save this map configuration", nil, nil)
	case errors.Is(err, models.ErrMapNotFound):
		models.WriteError(w, http.StatusNotFound, "MAP_NOT_FOUND", "Map not found", nil, nil)
	case errors.Is(err, models.ErrMapAssetNotFound):
		models.WriteError(w, http.StatusNotFound, "ASSET_NOT_FOUND", "A referenced map asset was not found", nil, nil)
	case errors.Is(err, models.ErrOutdatedRevision):
		models.WriteError(w, http.StatusConflict, "REVISION_CONFLICT", "Configuration revision is outdated; reload before saving", nil, nil)
	case errors.Is(err, models.ErrMapLockHeld):
		models.WriteError(w, http.StatusConflict, "LOCK_HELD", "Map is locked by another editor", nil, nil)
	case errors.Is(err, models.ErrInvalidMapConfiguration):
		models.WriteError(w, http.StatusUnprocessableEntity, "INVALID_CONFIGURATION", "Map configuration is invalid", nil, nil)
	case errors.Is(err, models.ErrPersistenceUnavailable):
		models.WriteError(w, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "Durable map persistence is unavailable", nil, nil)
	default:
		models.WriteError(w, http.StatusInternalServerError, "SAVE_CONFIG_ERROR", "Failed to save map configuration", nil, nil)
	}
}

// GetMap handles GET /api/maps/{mapId}
func (h *Handler) GetMap(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	m, err := h.mapRepo.GetMap(r.Context(), mapID, user.ID)
	if err != nil {
		if err == models.ErrMapNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Map not found", nil, nil)
			return
		}
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have access to this map", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "GET_MAP_ERROR", err.Error(), nil, nil)
		return
	}

	// Attach latest configuration if available
	if m.CurrentRevisionID != "" {
		rev, err := h.mapRepo.GetMapRevision(r.Context(), mapID, m.CurrentRevisionID, user.ID)
		if err == nil && rev != nil {
			m.Configuration = rev.ConfigurationJSON
		}
	}
	if m.Configuration == "" {
		revs, err := h.mapRepo.ListMapRevisions(r.Context(), mapID, user.ID)
		if err == nil && len(revs) > 0 {
			m.Configuration = revs[0].ConfigurationJSON
			m.CurrentRevisionID = revs[0].ID
			m.RevisionNumber = revs[0].RevisionNumber
		}
	}

	// Attach assets for this map
	seenAssets := make(map[string]bool)
	var mapAssets []*models.MapAsset
	assets, err := h.mapRepo.ListUserAssets(r.Context(), m.OwnerUserID)
	if err == nil {
		for _, a := range assets {
			if a.MapID == mapID && !seenAssets[a.ID] {
				mapAssets = append(mapAssets, a)
				seenAssets[a.ID] = true
			}
		}
	}
	if m.Configuration != "" {
		var confDoc struct {
			PrimaryDatasetID string   `json:"primary_dataset_id"`
			Members          []string `json:"members"`
		}
		if json.Unmarshal([]byte(m.Configuration), &confDoc) == nil {
			candidateIDs := append([]string{confDoc.PrimaryDatasetID}, confDoc.Members...)
			for _, cid := range candidateIDs {
				cid = strings.TrimSpace(cid)
				if cid == "" || seenAssets[cid] {
					continue
				}
				if a, aErr := h.mapRepo.GetAsset(r.Context(), cid, m.OwnerUserID); aErr == nil && a != nil {
					mapAssets = append(mapAssets, a)
					seenAssets[a.ID] = true
				}
			}
		}
	}
	m.Assets = mapAssets
	m.AssetCount = len(mapAssets)
	if m.Assets == nil {
		m.Assets = []*models.MapAsset{}
	}

	models.WriteJSON(w, http.StatusOK, m)
}

// UpdateMap handles PUT /api/maps/{mapId}
func (h *Handler) UpdateMap(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body", nil, nil)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		models.WriteError(w, http.StatusBadRequest, "VALIDATION_FAILED", "Map name cannot be empty", nil, nil)
		return
	}

	updated, err := h.mapRepo.UpdateMap(r.Context(), mapID, user.ID, name, req.Description)
	if err != nil {
		if err == models.ErrMapNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Map not found", nil, nil)
			return
		}
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the map owner or editors can update this map", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "UPDATE_MAP_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, updated)
}

// DeleteMap handles DELETE /api/maps/{mapId} (Owner only)
func (h *Handler) DeleteMap(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	if err := h.mapRepo.DeleteMap(r.Context(), mapID, user.ID); err != nil {
		if err == models.ErrMapNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Map not found", nil, nil)
			return
		}
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the owner can delete this map", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "DELETE_MAP_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// SaveMapConfig handles POST /api/maps/{mapId}/save
func (h *Handler) SaveMapConfig(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	if h.configurationService == nil {
		models.WriteError(w, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "Durable map persistence is unavailable", nil, nil)
		return
	}

	var req struct {
		Configuration     interface{} `json:"configuration"`
		ConfigurationJSON string      `json:"configuration_json"`
		Description       string      `json:"description"`
		ExpectedRevision  int         `json:"expected_revision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		models.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body", nil, nil)
		return
	}

	configStr := req.ConfigurationJSON
	if configStr == "" && req.Configuration != nil {
		b, _ := json.Marshal(req.Configuration)
		configStr = string(b)
	}
	if configStr == "" {
		configStr = "{}"
	}
	if !json.Valid([]byte(configStr)) {
		models.WriteError(w, http.StatusUnprocessableEntity, "INVALID_CONFIGURATION", "Configuration must be valid JSON", nil, nil)
		return
	}
	var snapshot struct {
		Name             string   `json:"name"`
		PrimaryDatasetID string   `json:"primary_dataset_id"`
		Members          []string `json:"members"`
	}
	_ = json.Unmarshal([]byte(configStr), &snapshot)

	result, err := h.configurationService.SaveExistingMapConfiguration(r.Context(), user, mapID, models.MapConfigurationCommit{
		Name:              snapshot.Name,
		Description:       req.Description,
		PrimaryDatasetID:  snapshot.PrimaryDatasetID,
		MemberAssetIDs:    snapshot.Members,
		ConfigurationJSON: configStr,
		ExpectedRevision:  req.ExpectedRevision,
	})
	if err != nil {
		writeMapConfigurationError(w, err)
		return
	}

	models.WriteJSON(w, http.StatusOK, result)
}

// ListRevisions handles GET /api/maps/{mapId}/revisions
func (h *Handler) ListRevisions(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	revs, err := h.mapRepo.ListMapRevisions(r.Context(), mapID, user.ID)
	if err != nil {
		if err == models.ErrMapNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Map not found", nil, nil)
			return
		}
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "REVISIONS_ERROR", err.Error(), nil, nil)
		return
	}
	if revs == nil {
		revs = []*models.MapRevision{}
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"revisions": revs,
		"total":     len(revs),
	})
}

// RestoreRevision handles POST /api/maps/{mapId}/revisions/{revisionId}/restore
func (h *Handler) RestoreRevision(w http.ResponseWriter, r *http.Request, user *models.User, mapID, revisionID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	rev, err := h.mapRepo.RestoreMapRevision(r.Context(), mapID, revisionID, user.ID, user.Name)
	if err != nil {
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only editors or owner can restore revisions", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "RESTORE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, rev)
}

// AcquireLock handles POST /api/maps/{mapId}/lock
func (h *Handler) AcquireLock(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	lock, err := h.mapRepo.AcquireLock(r.Context(), mapID, user.ID, user.Name, 60*time.Second)
	if err != nil {
		if err == models.ErrMapLockHeld {
			currentLock, _ := h.mapRepo.GetLock(r.Context(), mapID)
			models.WriteError(w, http.StatusConflict, "LOCK_HELD", "Map is currently being edited by another user", currentLock, nil)
			return
		}
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have access to this map", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "LOCK_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, lock)
}

// RenewLock handles POST /api/maps/{mapId}/lock/renew
func (h *Handler) RenewLock(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	lock, err := h.mapRepo.RenewLock(r.Context(), mapID, user.ID, 60*time.Second)
	if err != nil {
		if err == models.ErrMapLockExpiredOrMissing {
			models.WriteError(w, http.StatusConflict, "LOCK_LOST", "Lock has expired or was acquired by another user", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "LOCK_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, lock)
}

// ReleaseLock handles DELETE /api/maps/{mapId}/lock
func (h *Handler) ReleaseLock(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	if err := h.mapRepo.ReleaseLock(r.Context(), mapID, user.ID); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LOCK_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"released": true})
}

// GetLock handles GET /api/maps/{mapId}/lock
func (h *Handler) GetLock(w http.ResponseWriter, r *http.Request, mapID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	lock, err := h.mapRepo.GetLock(r.Context(), mapID)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LOCK_ERROR", err.Error(), nil, nil)
		return
	}
	if lock == nil {
		models.WriteJSON(w, http.StatusOK, map[string]interface{}{"active": false})
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"active": true,
		"lock":   lock,
	})
}

// ListUserFiles handles GET /api/user/files (Personal Uploaded Files)
func (h *Handler) ListUserFiles(w http.ResponseWriter, r *http.Request, user *models.User) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	assets, err := h.mapRepo.ListUserAssets(r.Context(), user.ID)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LIST_FILES_ERROR", err.Error(), nil, nil)
		return
	}
	if assets == nil {
		assets = []*models.MapAsset{}
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"files": assets,
		"total": len(assets),
	})
}

// DeleteUserFile handles DELETE /api/user/files/{fileId}
func (h *Handler) DeleteUserFile(w http.ResponseWriter, r *http.Request, user *models.User, fileID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	if err := h.mapRepo.DeleteAsset(r.Context(), fileID, user.ID); err != nil {
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the owner can delete this file", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "DELETE_FILE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// InviteToMap handles POST /api/maps/{mapId}/invitations
func (h *Handler) InviteToMap(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req struct {
		Email      string `json:"email"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body", nil, nil)
		return
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") {
		models.WriteError(w, http.StatusBadRequest, "INVALID_EMAIL", "Valid email address is required", nil, nil)
		return
	}
	if strings.EqualFold(cleanEmail, user.Email) {
		models.WriteError(w, http.StatusBadRequest, "CANNOT_INVITE_SELF", "You cannot invite yourself to your map", nil, nil)
		return
	}

	m, err := h.mapRepo.GetMap(r.Context(), mapID, user.ID)
	if err != nil {
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Map not found", nil, nil)
		return
	}
	if m.OwnerUserID != user.ID {
		models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the map owner can invite users", nil, nil)
		return
	}

	// Cap total members to 10 (owner + active shares + pending invitations)
	shares, invitations, err := h.mapRepo.ListMapShares(r.Context(), mapID, user.ID)
	if err == nil {
		totalMembers := 1 + len(shares) + len(invitations)
		if totalMembers >= 10 {
			models.WriteError(w, http.StatusBadRequest, "MEMBER_CAP_REACHED", "Total map members cannot exceed 10", nil, nil)
			return
		}
	}

	// All shared collaborators receive Editor privileges; only owner can delete the map
	perm := models.PermissionEditor
	token := generateSecureToken(32)
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)

	inv, err := h.mapRepo.CreateShareInvitation(r.Context(), mapID, user.ID, user.Name, cleanEmail, perm, token, expiresAt)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "INVITE_ERROR", err.Error(), nil, nil)
		return
	}

	// Send email via SMTP mailer if configured
	if h.mailer != nil {
		base := h.appPublicURL
		if base == "" {
			base = "http://localhost:5173"
		}
		inviteLink := fmt.Sprintf("%s/dashboard?accept_invite=%s", base, token)
		invMsg := services.InvitationEmail{
			To:          cleanEmail,
			Link:        inviteLink,
			ProjectName: m.Name,
			InviterName: user.Name,
			Role:        string(perm),
		}
		if sendErr := h.mailer.SendInvitation(r.Context(), invMsg); sendErr != nil {
			log.Printf("[MAP INVITATION WARNING] Failed to deliver email to %s: %v", cleanEmail, sendErr)
		} else {
			log.Printf("[MAP INVITATION SUCCESS] Dispatched invitation to %s for map %s", cleanEmail, m.Name)
		}
	}

	inv.Token = token // Return token so frontend/tests can display or use it
	models.WriteJSON(w, http.StatusCreated, inv)
}

// ListShares handles GET /api/maps/{mapId}/shares
func (h *Handler) ListShares(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	shares, invitations, err := h.mapRepo.ListMapShares(r.Context(), mapID, user.ID)
	if err != nil {
		if err == models.ErrMapNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Map not found", nil, nil)
			return
		}
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Access denied", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "SHARES_ERROR", err.Error(), nil, nil)
		return
	}
	if shares == nil {
		shares = []*models.MapShare{}
	}
	if invitations == nil {
		invitations = []*models.MapShareInvitation{}
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"shares":      shares,
		"invitations": invitations,
		"total":       len(shares) + len(invitations),
	})
}

// UpdateSharePermission handles PATCH /api/maps/{mapId}/shares/{shareId}
func (h *Handler) UpdateSharePermission(w http.ResponseWriter, r *http.Request, user *models.User, mapID, shareID string) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body", nil, nil)
		return
	}

	perm := models.CleanSharePermission(req.Permission)
	if err := h.mapRepo.UpdateMapSharePermission(r.Context(), mapID, shareID, user.ID, perm); err != nil {
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the owner can update share permissions", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "UPDATE_SHARE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// RevokeShare handles DELETE /api/maps/{mapId}/shares/{shareId}
func (h *Handler) RevokeShare(w http.ResponseWriter, r *http.Request, user *models.User, mapID, shareID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	if err := h.mapRepo.RevokeMapShare(r.Context(), mapID, shareID, user.ID); err != nil {
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the owner can revoke map access", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "REVOKE_SHARE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

// GetInvitation handles GET /api/map-invitations/{token}
func (h *Handler) GetInvitation(w http.ResponseWriter, r *http.Request, user *models.User, token string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	inv, err := h.mapRepo.GetInvitationByToken(r.Context(), token)
	if err != nil || inv == nil {
		models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Invitation not found or has expired", nil, nil)
		return
	}

	emailMatches := false
	currentEmail := ""
	if user != nil {
		currentEmail = user.Email
		emailMatches = strings.EqualFold(strings.TrimSpace(inv.Email), strings.TrimSpace(user.Email))
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":                 inv.ID,
		"map_id":             inv.MapID,
		"map_name":           inv.MapName,
		"email":              inv.Email,
		"permission":         inv.Permission,
		"status":             inv.Status,
		"inviter_name":       inv.InviterName,
		"expires_at":         inv.ExpiresAt,
		"email_matches":      emailMatches,
		"current_user_email": currentEmail,
	})
}

// AcceptInvitation handles POST /api/map-invitations/{token}/accept
func (h *Handler) AcceptInvitation(w http.ResponseWriter, r *http.Request, user *models.User, token string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	share, err := h.mapRepo.AcceptShareInvitation(r.Context(), token, user.ID, user.Email, user.Name)
	if err != nil {
		if errors.Is(err, models.ErrInvitationEmailMismatch) {
			invEmail := ""
			mapName := ""
			if inv, iErr := h.mapRepo.GetInvitationByToken(r.Context(), token); iErr == nil && inv != nil {
				invEmail = inv.Email
				mapName = inv.MapName
			}
			msg := fmt.Sprintf("This invitation was sent to %s, but you are currently signed in as %s. Please sign in as %s to accept this invitation.", invEmail, user.Email, invEmail)
			if invEmail == "" {
				msg = fmt.Sprintf("This invitation was not sent to %s. Please sign in with the invited email address.", user.Email)
			}
			models.WriteError(w, http.StatusForbidden, "INVITATION_EMAIL_MISMATCH", msg, map[string]interface{}{
				"invited_email": invEmail,
				"current_email": user.Email,
				"map_name":      mapName,
			}, nil)
			return
		}
		if err == models.ErrInvitationNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Invitation not found or has expired", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "ACCEPT_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, share)
}

// DeclineInvitation handles POST /api/map-invitations/{token}/decline
func (h *Handler) DeclineInvitation(w http.ResponseWriter, r *http.Request, user *models.User, token string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	if err := h.mapRepo.DeclineShareInvitation(r.Context(), token); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "DECLINE_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"declined": true})
}

// CreateShareLink handles POST /api/maps/{mapId}/share-links
func (h *Handler) CreateShareLink(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodPost {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	var req struct {
		Permission    string `json:"permission"`
		IsPublic      bool   `json:"is_public"`
		ExpiresInDays int    `json:"expires_in_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		models.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body", nil, nil)
		return
	}

	// All share links grant Editor privileges; only owner can delete the map
	perm := models.PermissionEditor
	token := generateSecureToken(32)

	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		exp := time.Now().UTC().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &exp
	}

	link, err := h.mapRepo.CreateShareLink(r.Context(), mapID, user.ID, perm, req.IsPublic, expiresAt, token)
	if err != nil {
		if err == models.ErrMapAccessDenied {
			models.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only the map owner can create share links", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "CREATE_LINK_ERROR", err.Error(), nil, nil)
		return
	}

	base := h.appPublicURL
	if base == "" {
		base = "http://localhost:5173"
	}
	linkURL := fmt.Sprintf("%s/share/%s", base, token)

	models.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"link":      link,
		"share_url": linkURL,
		"token":     token,
	})
}

// ListShareLinks handles GET /api/maps/{mapId}/share-links
func (h *Handler) ListShareLinks(w http.ResponseWriter, r *http.Request, user *models.User, mapID string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	links, err := h.mapRepo.ListShareLinks(r.Context(), mapID, user.ID)
	if err != nil {
		models.WriteError(w, http.StatusInternalServerError, "LIST_LINKS_ERROR", err.Error(), nil, nil)
		return
	}
	if links == nil {
		links = []*models.ShareLink{}
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"links": links,
		"total": len(links),
	})
}

// RevokeShareLink handles DELETE /api/maps/{mapId}/share-links/{linkId}
func (h *Handler) RevokeShareLink(w http.ResponseWriter, r *http.Request, user *models.User, mapID, linkID string) {
	if r.Method != http.MethodDelete {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	if err := h.mapRepo.RevokeShareLink(r.Context(), mapID, linkID, user.ID); err != nil {
		models.WriteError(w, http.StatusInternalServerError, "REVOKE_LINK_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

// ResolveShareLink handles GET /api/share/{token} (Public or authenticated)
func (h *Handler) ResolveShareLink(w http.ResponseWriter, r *http.Request, token string) {
	if r.Method != http.MethodGet {
		models.WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil, nil)
		return
	}

	link, m, err := h.mapRepo.GetShareLinkByToken(r.Context(), token)
	if err != nil {
		if err == models.ErrShareLinkNotFound {
			models.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Share link not found or has been revoked", nil, nil)
			return
		}
		if err == models.ErrShareLinkExpired {
			models.WriteError(w, http.StatusGone, "LINK_EXPIRED", "This share link has expired", nil, nil)
			return
		}
		models.WriteError(w, http.StatusInternalServerError, "RESOLVE_LINK_ERROR", err.Error(), nil, nil)
		return
	}

	models.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"map":        m,
		"permission": link.Permission,
		"is_public":  link.IsPublic,
		"expires_at": link.ExpiresAt,
	})
}
