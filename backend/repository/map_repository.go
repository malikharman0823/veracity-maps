package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// MapRepo defines the contract for personal map ownership, asset management, and map sharing.
type MapRepo interface {
	// Maps
	ListMyMaps(ctx context.Context, userID string) ([]*models.Map, error)
	ListSharedWithMe(ctx context.Context, userID string) ([]*models.Map, error)
	CreateMap(ctx context.Context, ownerUserID, name, description string) (*models.Map, error)
	CreateMapFromConfiguration(ctx context.Context, ownerUserID, authorName string, commit models.MapConfigurationCommit) (*models.MapConfigurationResult, error)
	GetMap(ctx context.Context, mapID, userID string) (*models.Map, error)
	UpdateMap(ctx context.Context, mapID, userID, name, description string) (*models.Map, error)
	DeleteMap(ctx context.Context, mapID, userID string) error

	// Assets / Uploaded Files
	RegisterAsset(ctx context.Context, asset *models.MapAsset) error
	ListUserAssets(ctx context.Context, userID string) ([]*models.MapAsset, error)
	GetAsset(ctx context.Context, assetID, userID string) (*models.MapAsset, error)
	DeleteAsset(ctx context.Context, assetID, userID string) error

	// Revisions & Configurations
	SaveMapConfig(ctx context.Context, mapID, userID, userName, configJSON, description string, expectedRevision int) (*models.MapRevision, error)
	SaveMapConfiguration(ctx context.Context, mapID, userID, authorName string, commit models.MapConfigurationCommit) (*models.MapConfigurationResult, error)
	ListMapRevisions(ctx context.Context, mapID, userID string) ([]*models.MapRevision, error)
	GetMapRevision(ctx context.Context, mapID, revisionID, userID string) (*models.MapRevision, error)
	RestoreMapRevision(ctx context.Context, mapID, revisionID, userID, userName string) (*models.MapRevision, error)

	// Locks
	AcquireLock(ctx context.Context, mapID, userID, userName string, ttl time.Duration) (*models.MapEditingLock, error)
	RenewLock(ctx context.Context, mapID, userID string, ttl time.Duration) (*models.MapEditingLock, error)
	ReleaseLock(ctx context.Context, mapID, userID string) error
	GetLock(ctx context.Context, mapID string) (*models.MapEditingLock, error)

	// Email-Based Sharing
	CreateShareInvitation(ctx context.Context, mapID, invitedBy, inviterName, email string, perm models.SharePermission, token string, expiresAt time.Time) (*models.MapShareInvitation, error)
	GetInvitationByToken(ctx context.Context, token string) (*models.MapShareInvitation, error)
	AcceptShareInvitation(ctx context.Context, token, userID, userEmail, userName string) (*models.MapShare, error)
	DeclineShareInvitation(ctx context.Context, token string) error
	ListMapShares(ctx context.Context, mapID, userID string) ([]*models.MapShare, []*models.MapShareInvitation, error)
	UpdateMapSharePermission(ctx context.Context, mapID, shareID, userID string, perm models.SharePermission) error
	RevokeMapShare(ctx context.Context, mapID, shareID, userID string) error

	// Secure Shareable Links
	CreateShareLink(ctx context.Context, mapID, createdBy string, perm models.SharePermission, isPublic bool, expiresAt *time.Time, token string) (*models.ShareLink, error)
	GetShareLinkByToken(ctx context.Context, token string) (*models.ShareLink, *models.Map, error)
	ListShareLinks(ctx context.Context, mapID, userID string) ([]*models.ShareLink, error)
	RevokeShareLink(ctx context.Context, mapID, linkID, userID string) error
}

func HashShareToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// MemoryMapRepository implements MapRepo in-memory with thread safety.
type MemoryMapRepository struct {
	mu          sync.RWMutex
	maps        map[string]*models.Map
	assets      map[string]*models.MapAsset
	shares      map[string]*models.MapShare
	invitations map[string]*models.MapShareInvitation
	links       map[string]*models.ShareLink
	revisions   map[string][]*models.MapRevision
	locks       map[string]*models.MapEditingLock
}

func NewMemoryMapRepository() *MemoryMapRepository {
	return &MemoryMapRepository{
		maps:        make(map[string]*models.Map),
		assets:      make(map[string]*models.MapAsset),
		shares:      make(map[string]*models.MapShare),
		invitations: make(map[string]*models.MapShareInvitation),
		links:       make(map[string]*models.ShareLink),
		revisions:   make(map[string][]*models.MapRevision),
		locks:       make(map[string]*models.MapEditingLock),
	}
}

func (r *MemoryMapRepository) ListMyMaps(ctx context.Context, userID string) ([]*models.Map, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Map
	for _, m := range r.maps {
		if m.OwnerUserID == userID {
			clone := *m
			clone.SavedByCurrentUser = true
			clone.Permission = models.PermissionEditor
			clone.SharingStatus = "private"
			// Check if shared
			for _, s := range r.shares {
				if s.MapID == m.ID {
					clone.SharingStatus = "shared"
					break
				}
			}
			// Check active lock
			if lock, ok := r.locks[m.ID]; ok && time.Now().Before(lock.ExpiresAt) {
				lockClone := *lock
				clone.Lock = &lockClone
			}
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (r *MemoryMapRepository) ListSharedWithMe(ctx context.Context, userID string) ([]*models.Map, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.Map
	for _, s := range r.shares {
		if s.UserID == userID {
			if m, ok := r.maps[s.MapID]; ok {
				clone := *m
				for _, revision := range r.revisions[m.ID] {
					if revision.CreatedBy == userID {
						clone.SavedByCurrentUser = true
						break
					}
				}
				clone.Permission = s.Permission
				clone.SharingStatus = "shared_with_me"
				clone.SharedBy = s.SharedBy
				clone.SharedByName = s.SharedByName
				if lock, ok := r.locks[m.ID]; ok && time.Now().Before(lock.ExpiresAt) {
					lockClone := *lock
					clone.Lock = &lockClone
				}
				result = append(result, &clone)
			}
		}
	}
	return result, nil
}

func (r *MemoryMapRepository) CreateMap(ctx context.Context, ownerUserID, name, description string) (*models.Map, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	mapID := fmt.Sprintf("map_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", ownerUserID, name, now.UnixNano())))[:20]

	m := &models.Map{
		ID:             mapID,
		OwnerUserID:    ownerUserID,
		Name:           name,
		Description:    description,
		RevisionNumber: 1,
		CreatedBy:      ownerUserID,
		CreatedAt:      now,
		UpdatedAt:      now,
		Permission:     models.PermissionEditor,
		SharingStatus:  "private",
	}
	r.maps[mapID] = m

	// Initial Revision
	revID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-1-%d", mapID, now.UnixNano())))[:20]
	rev := &models.MapRevision{
		ID:                  revID,
		MapID:               mapID,
		RevisionNumber:      1,
		ConfigurationJSON:   `{"layers":[],"control_points":[]}`,
		RevisionDescription: "Initial map creation",
		CreatedBy:           ownerUserID,
		CreatedAt:           now,
	}
	r.revisions[mapID] = []*models.MapRevision{rev}
	m.CurrentRevisionID = revID

	clone := *m
	return &clone, nil
}

func (r *MemoryMapRepository) CreateMapFromConfiguration(
	ctx context.Context,
	ownerUserID, authorName string,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ownerUserID == "" || commit.Name == "" || commit.ConfigurationJSON == "" {
		return nil, models.ErrInvalidMapConfiguration
	}
	for _, location := range commit.AssetLocations {
		asset, ok := r.assets[location.AssetID]
		if !ok {
			return nil, models.ErrMapAssetNotFound
		}
		if asset.OwnerUserID != ownerUserID || location.OwnerUserID != ownerUserID {
			return nil, models.ErrMapAccessDenied
		}
	}

	now := time.Now().UTC()
	mapID := commit.MapID
	if mapID == "" {
		mapID = fmt.Sprintf("map_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", ownerUserID, commit.Name, now.UnixNano())))[:20]
	}
	revisionID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-1-%d", mapID, now.UnixNano())))[:20]
	createdMap := &models.Map{
		ID:                mapID,
		OwnerUserID:       ownerUserID,
		Name:              commit.Name,
		Description:       commit.Description,
		CurrentRevisionID: revisionID,
		RevisionNumber:    1,
		CreatedBy:         ownerUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
		LastModifiedUser:  authorName,
		Permission:        models.PermissionEditor,
		SharingStatus:     "private",
	}
	createdRevision := &models.MapRevision{
		ID:                  revisionID,
		MapID:               mapID,
		RevisionNumber:      1,
		ConfigurationJSON:   commit.ConfigurationJSON,
		RevisionDescription: commit.Description,
		CreatedBy:           ownerUserID,
		CreatedByName:       authorName,
		CreatedAt:           now,
	}

	for _, location := range commit.AssetLocations {
		asset := r.assets[location.AssetID]
		asset.MapID = mapID
		asset.StoragePath = location.StoragePath
	}
	r.maps[mapID] = createdMap
	r.revisions[mapID] = []*models.MapRevision{createdRevision}

	mapClone := *createdMap
	revisionClone := *createdRevision
	return &models.MapConfigurationResult{Map: &mapClone, Revision: &revisionClone}, nil
}

func (r *MemoryMapRepository) GetMap(ctx context.Context, mapID, userID string) (*models.Map, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}

	clone := *m
	if m.OwnerUserID == userID {
		clone.Permission = models.PermissionEditor
	} else {
		// Check share
		found := false
		for _, s := range r.shares {
			if s.MapID == mapID && s.UserID == userID {
				clone.Permission = s.Permission
				found = true
				break
			}
		}
		if !found {
			return nil, models.ErrMapAccessDenied
		}
	}

	if lock, ok := r.locks[mapID]; ok && time.Now().Before(lock.ExpiresAt) {
		lockClone := *lock
		clone.Lock = &lockClone
	}

	return &clone, nil
}

func (r *MemoryMapRepository) UpdateMap(ctx context.Context, mapID, userID, name, description string) (*models.Map, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		// Editors can update name/description if given editor permission
		canEdit := false
		for _, s := range r.shares {
			if s.MapID == mapID && s.UserID == userID && s.Permission == models.PermissionEditor {
				canEdit = true
				break
			}
		}
		if !canEdit {
			return nil, models.ErrMapAccessDenied
		}
	}

	m.Name = name
	m.Description = description
	m.UpdatedAt = time.Now().UTC()
	clone := *m
	return &clone, nil
}

func (r *MemoryMapRepository) DeleteMap(ctx context.Context, mapID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}

	delete(r.maps, mapID)
	delete(r.revisions, mapID)
	delete(r.locks, mapID)

	// Clean shares and links
	for k, s := range r.shares {
		if s.MapID == mapID {
			delete(r.shares, k)
		}
	}
	for k, inv := range r.invitations {
		if inv.MapID == mapID {
			delete(r.invitations, k)
		}
	}
	for k, l := range r.links {
		if l.MapID == mapID {
			delete(r.links, k)
		}
	}
	return nil
}

// Asset Management
func (r *MemoryMapRepository) RegisterAsset(ctx context.Context, asset *models.MapAsset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = time.Now().UTC()
	}
	r.assets[asset.ID] = asset
	return nil
}

func (r *MemoryMapRepository) ListUserAssets(ctx context.Context, userID string) ([]*models.MapAsset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.MapAsset
	for _, a := range r.assets {
		if a.OwnerUserID == userID {
			clone := *a
			if a.MapID != "" {
				if m, ok := r.maps[a.MapID]; ok {
					clone.MapName = m.Name
				}
			}
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (r *MemoryMapRepository) GetAsset(ctx context.Context, assetID, userID string) (*models.MapAsset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.assets[assetID]
	if !ok {
		return nil, errors.New("asset not found")
	}
	if a.OwnerUserID != userID {
		// Check if asset belongs to a map owned by or shared with this user
		if a.MapID != "" {
			allowed := false
			if m, mOk := r.maps[a.MapID]; mOk && m.OwnerUserID == userID {
				allowed = true
			}
			if !allowed {
				for _, s := range r.shares {
					if s.MapID == a.MapID && s.UserID == userID {
						allowed = true
						break
					}
				}
			}
			if !allowed {
				return nil, models.ErrMapAccessDenied
			}
		} else {
			return nil, models.ErrMapAccessDenied
		}
	}

	clone := *a
	return &clone, nil
}

func (r *MemoryMapRepository) DeleteAsset(ctx context.Context, assetID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.assets[assetID]
	if !ok {
		return errors.New("asset not found")
	}
	if a.OwnerUserID != userID {
		return errors.New("access denied: only asset owner can delete")
	}

	delete(r.assets, assetID)
	return nil
}

// Revisions & Configurations
func (r *MemoryMapRepository) SaveMapConfig(ctx context.Context, mapID, userID, userName, configJSON, description string, expectedRevision int) (*models.MapRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}

	// Verify permission
	if m.OwnerUserID != userID {
		canEdit := false
		for _, s := range r.shares {
			if s.MapID == mapID && s.UserID == userID && s.Permission == models.PermissionEditor {
				canEdit = true
				break
			}
		}
		if !canEdit {
			return nil, models.ErrMapAccessDenied
		}
	}

	// Lock check: if lock held by someone else, reject
	if lock, hasLock := r.locks[mapID]; hasLock && time.Now().Before(lock.ExpiresAt) {
		if lock.UserID != userID {
			return nil, models.ErrMapLockHeld
		}
	}

	// Optimistic concurrency check
	if expectedRevision > 0 && m.RevisionNumber != expectedRevision {
		return nil, models.ErrOutdatedRevision
	}

	now := time.Now().UTC()
	nextRevNum := m.RevisionNumber + 1
	revID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%d-%d", mapID, nextRevNum, now.UnixNano())))[:20]

	rev := &models.MapRevision{
		ID:                  revID,
		MapID:               mapID,
		RevisionNumber:      nextRevNum,
		ConfigurationJSON:   configJSON,
		RevisionDescription: description,
		CreatedBy:           userID,
		CreatedByName:       userName,
		CreatedAt:           now,
	}

	r.revisions[mapID] = append(r.revisions[mapID], rev)
	m.RevisionNumber = nextRevNum
	m.CurrentRevisionID = revID
	m.UpdatedAt = now
	m.LastModifiedUser = userName

	return rev, nil
}

func (r *MemoryMapRepository) SaveMapConfiguration(
	ctx context.Context,
	mapID, userID, authorName string,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		canEdit := false
		for _, share := range r.shares {
			if share.MapID == mapID && share.UserID == userID && share.Permission == models.PermissionEditor {
				canEdit = true
				break
			}
		}
		if !canEdit {
			return nil, models.ErrMapAccessDenied
		}
	}
	if lock, hasLock := r.locks[mapID]; hasLock && time.Now().Before(lock.ExpiresAt) && lock.UserID != userID {
		return nil, models.ErrMapLockHeld
	}
	if commit.ExpectedRevision > 0 && m.RevisionNumber != commit.ExpectedRevision {
		return nil, models.ErrOutdatedRevision
	}
	if commit.ConfigurationJSON == "" {
		return nil, models.ErrInvalidMapConfiguration
	}
	for _, location := range commit.AssetLocations {
		asset, exists := r.assets[location.AssetID]
		if !exists {
			return nil, models.ErrMapAssetNotFound
		}
		if asset.OwnerUserID != m.OwnerUserID && asset.OwnerUserID != userID {
			return nil, models.ErrMapAccessDenied
		}
	}

	now := time.Now().UTC()
	nextRevision := m.RevisionNumber + 1
	revisionID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%d-%d", mapID, nextRevision, now.UnixNano())))[:20]
	revision := &models.MapRevision{
		ID:                  revisionID,
		MapID:               mapID,
		RevisionNumber:      nextRevision,
		ConfigurationJSON:   commit.ConfigurationJSON,
		RevisionDescription: commit.Description,
		CreatedBy:           userID,
		CreatedByName:       authorName,
		CreatedAt:           now,
	}
	for _, location := range commit.AssetLocations {
		r.assets[location.AssetID].MapID = mapID
		r.assets[location.AssetID].StoragePath = location.StoragePath
	}
	r.revisions[mapID] = append(r.revisions[mapID], revision)
	m.RevisionNumber = nextRevision
	m.CurrentRevisionID = revisionID
	m.UpdatedAt = now
	m.LastModifiedUser = authorName

	mapClone := *m
	revisionClone := *revision
	return &models.MapConfigurationResult{Map: &mapClone, Revision: &revisionClone}, nil
}

func (r *MemoryMapRepository) ListMapRevisions(ctx context.Context, mapID, userID string) ([]*models.MapRevision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		canView := false
		for _, s := range r.shares {
			if s.MapID == mapID && s.UserID == userID {
				canView = true
				break
			}
		}
		if !canView {
			return nil, models.ErrMapAccessDenied
		}
	}

	return r.revisions[mapID], nil
}

func (r *MemoryMapRepository) GetMapRevision(ctx context.Context, mapID, revisionID, userID string) (*models.MapRevision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	revs := r.revisions[mapID]
	for _, rev := range revs {
		if rev.ID == revisionID {
			clone := *rev
			return &clone, nil
		}
	}
	return nil, errors.New("revision not found")
}

func (r *MemoryMapRepository) RestoreMapRevision(ctx context.Context, mapID, revisionID, userID, userName string) (*models.MapRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		return nil, models.ErrMapAccessDenied
	}

	var targetRev *models.MapRevision
	for _, rev := range r.revisions[mapID] {
		if rev.ID == revisionID {
			targetRev = rev
			break
		}
	}
	if targetRev == nil {
		return nil, errors.New("target revision not found")
	}

	now := time.Now().UTC()
	nextRevNum := m.RevisionNumber + 1
	newRevID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%d-%d", mapID, nextRevNum, now.UnixNano())))[:20]

	restoredRev := &models.MapRevision{
		ID:                  newRevID,
		MapID:               mapID,
		RevisionNumber:      nextRevNum,
		ConfigurationJSON:   targetRev.ConfigurationJSON,
		RevisionDescription: fmt.Sprintf("Restored from revision %d", targetRev.RevisionNumber),
		CreatedBy:           userID,
		CreatedByName:       userName,
		CreatedAt:           now,
	}

	r.revisions[mapID] = append(r.revisions[mapID], restoredRev)
	m.RevisionNumber = nextRevNum
	m.CurrentRevisionID = newRevID
	m.UpdatedAt = now
	m.LastModifiedUser = userName

	return restoredRev, nil
}

// Single-Editor Locks
func (r *MemoryMapRepository) AcquireLock(ctx context.Context, mapID, userID, userName string, ttl time.Duration) (*models.MapEditingLock, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	existing, ok := r.locks[mapID]
	if ok && now.Before(existing.ExpiresAt) && existing.UserID != userID {
		return existing, models.ErrMapLockHeld
	}

	lock := &models.MapEditingLock{
		MapID:     mapID,
		UserID:    userID,
		UserName:  userName,
		LockedAt:  now,
		ExpiresAt: now.Add(ttl),
	}
	r.locks[mapID] = lock
	return lock, nil
}

func (r *MemoryMapRepository) RenewLock(ctx context.Context, mapID, userID string, ttl time.Duration) (*models.MapEditingLock, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	existing, ok := r.locks[mapID]
	if ok && existing.UserID != userID && existing.ExpiresAt.After(now) {
		return nil, models.ErrMapLockHeld
	}

	userName := userID
	if ok && existing.UserName != "" {
		userName = existing.UserName
	}

	lock := &models.MapEditingLock{
		MapID:     mapID,
		UserID:    userID,
		UserName:  userName,
		LockedAt:  now,
		ExpiresAt: now.Add(ttl),
	}
	r.locks[mapID] = lock
	return lock, nil
}

func (r *MemoryMapRepository) ReleaseLock(ctx context.Context, mapID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.locks[mapID]
	if ok && existing.UserID == userID {
		delete(r.locks, mapID)
	}
	return nil
}

func (r *MemoryMapRepository) GetLock(ctx context.Context, mapID string) (*models.MapEditingLock, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lock, ok := r.locks[mapID]
	if !ok || time.Now().After(lock.ExpiresAt) {
		return nil, nil
	}
	clone := *lock
	return &clone, nil
}

// Email-Based Map Sharing
func (r *MemoryMapRepository) CreateShareInvitation(ctx context.Context, mapID, invitedBy, inviterName, email string, perm models.SharePermission, token string, expiresAt time.Time) (*models.MapShareInvitation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != invitedBy {
		return nil, models.ErrMapAccessDenied
	}

	sharesCount := 0
	for _, s := range r.shares {
		if s.MapID == mapID {
			sharesCount++
		}
	}
	invCount := 0
	for _, i := range r.invitations {
		if i.MapID == mapID && i.Status == "pending" && time.Now().Before(i.ExpiresAt) {
			invCount++
		}
	}
	if 1+sharesCount+invCount >= 10 {
		return nil, errors.New("map member cap reached (maximum 10 members)")
	}

	invID := fmt.Sprintf("inv_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", mapID, email, time.Now().UnixNano())))[:20]
	tokenHash := HashShareToken(token)

	inv := &models.MapShareInvitation{
		ID:          invID,
		MapID:       mapID,
		MapName:     m.Name,
		Email:       email,
		Permission:  perm,
		TokenHash:   tokenHash,
		Token:       token,
		Status:      "pending",
		InvitedBy:   invitedBy,
		InviterName: inviterName,
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   expiresAt,
	}
	r.invitations[tokenHash] = inv
	return inv, nil
}

func (r *MemoryMapRepository) GetInvitationByToken(ctx context.Context, token string) (*models.MapShareInvitation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tokenHash := HashShareToken(token)
	inv, ok := r.invitations[tokenHash]
	if !ok || inv.Status != "pending" || time.Now().After(inv.ExpiresAt) {
		return nil, models.ErrInvitationNotFound
	}
	clone := *inv
	return &clone, nil
}

func (r *MemoryMapRepository) AcceptShareInvitation(ctx context.Context, token, userID, userEmail, userName string) (*models.MapShare, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tokenHash := HashShareToken(token)
	inv, ok := r.invitations[tokenHash]
	if !ok {
		return nil, models.ErrInvitationNotFound
	}

	if !strings.EqualFold(strings.TrimSpace(inv.Email), strings.TrimSpace(userEmail)) {
		return nil, models.ErrInvitationEmailMismatch
	}

	// Idempotency check: if invitation was already accepted, return existing share
	if inv.Status == "accepted" {
		for _, s := range r.shares {
			if s.MapID == inv.MapID && s.UserID == userID {
				return s, nil
			}
		}
	}

	if inv.Status != "pending" || time.Now().After(inv.ExpiresAt) {
		return nil, models.ErrInvitationNotFound
	}

	shareID := fmt.Sprintf("share_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", inv.MapID, userID, time.Now().UnixNano())))[:20]
	now := time.Now().UTC()

	share := &models.MapShare{
		ID:           shareID,
		MapID:        inv.MapID,
		MapName:      inv.MapName,
		UserID:       userID,
		UserName:     userName,
		UserEmail:    userEmail,
		Permission:   inv.Permission,
		SharedBy:     inv.InvitedBy,
		SharedByName: inv.InviterName,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	r.shares[shareID] = share
	inv.Status = "accepted"

	return share, nil
}

func (r *MemoryMapRepository) DeclineShareInvitation(ctx context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tokenHash := HashShareToken(token)
	inv, ok := r.invitations[tokenHash]
	if !ok || inv.Status != "pending" {
		return models.ErrInvitationNotFound
	}
	inv.Status = "declined"
	return nil
}

func (r *MemoryMapRepository) ListMapShares(ctx context.Context, mapID, userID string) ([]*models.MapShare, []*models.MapShareInvitation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		return nil, nil, models.ErrMapAccessDenied
	}

	var activeShares []*models.MapShare
	for _, s := range r.shares {
		if s.MapID == mapID {
			clone := *s
			activeShares = append(activeShares, &clone)
		}
	}

	var pendingInvs []*models.MapShareInvitation
	for _, inv := range r.invitations {
		if inv.MapID == mapID && inv.Status == "pending" && time.Now().Before(inv.ExpiresAt) {
			clone := *inv
			pendingInvs = append(pendingInvs, &clone)
		}
	}

	return activeShares, pendingInvs, nil
}

func (r *MemoryMapRepository) UpdateMapSharePermission(ctx context.Context, mapID, shareID, userID string, perm models.SharePermission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}

	s, ok := r.shares[shareID]
	if !ok || s.MapID != mapID {
		return errors.New("share grant not found")
	}

	s.Permission = perm
	s.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryMapRepository) RevokeMapShare(ctx context.Context, mapID, shareID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}

	delete(r.shares, shareID)
	return nil
}

// Secure Shareable Links
func (r *MemoryMapRepository) CreateShareLink(ctx context.Context, mapID, createdBy string, perm models.SharePermission, isPublic bool, expiresAt *time.Time, token string) (*models.ShareLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != createdBy {
		return nil, models.ErrMapAccessDenied
	}

	linkID := fmt.Sprintf("link_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%d", mapID, time.Now().UnixNano())))[:20]
	tokenHash := HashShareToken(token)

	link := &models.ShareLink{
		ID:         linkID,
		MapID:      mapID,
		TokenHash:  tokenHash,
		Token:      token,
		Permission: perm,
		IsPublic:   isPublic,
		ExpiresAt:  expiresAt,
		CreatedBy:  createdBy,
		CreatedAt:  time.Now().UTC(),
	}
	r.links[tokenHash] = link
	return link, nil
}

func (r *MemoryMapRepository) GetShareLinkByToken(ctx context.Context, token string) (*models.ShareLink, *models.Map, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tokenHash := HashShareToken(token)
	link, ok := r.links[tokenHash]
	if !ok {
		return nil, nil, models.ErrShareLinkNotFound
	}
	if link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt) {
		return nil, nil, models.ErrShareLinkExpired
	}

	m, ok := r.maps[link.MapID]
	if !ok {
		return nil, nil, models.ErrMapNotFound
	}

	linkClone := *link
	mapClone := *m
	mapClone.Permission = link.Permission
	return &linkClone, &mapClone, nil
}

func (r *MemoryMapRepository) ListShareLinks(ctx context.Context, mapID, userID string) ([]*models.ShareLink, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.maps[mapID]
	if !ok {
		return nil, models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		return nil, models.ErrMapAccessDenied
	}

	var result []*models.ShareLink
	for _, l := range r.links {
		if l.MapID == mapID {
			clone := *l
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (r *MemoryMapRepository) RevokeShareLink(ctx context.Context, mapID, linkID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.maps[mapID]
	if !ok {
		return models.ErrMapNotFound
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}

	for k, l := range r.links {
		if l.ID == linkID && l.MapID == mapID {
			delete(r.links, k)
			break
		}
	}
	return nil
}

// MSSQLMapRepository implements MapRepo backed by SQL Server 2022.
type MSSQLMapRepository struct {
	db *sql.DB
}

func NewMSSQLMapRepository(db *sql.DB) *MSSQLMapRepository {
	return &MSSQLMapRepository{db: db}
}

func (r *MSSQLMapRepository) ListMyMaps(ctx context.Context, userID string) ([]*models.Map, error) {
	query := `
		SELECT m.id, m.owner_user_id, u.name, u.email, m.name, ISNULL(m.description, ''),
		       ISNULL(m.current_revision_id, ''), ISNULL(m.revision_number, 1), m.created_at, m.updated_at,
		       (SELECT COUNT(1) FROM dbo.Map_Assets WHERE map_id = m.id) AS asset_count
		FROM dbo.Maps m
		LEFT JOIN dbo.Users u ON u.id = m.owner_user_id
		WHERE m.owner_user_id = @p1
		ORDER BY m.updated_at DESC;
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.Map
	for rows.Next() {
		var m models.Map
		if err := rows.Scan(
			&m.ID, &m.OwnerUserID, &m.OwnerUserName, &m.OwnerUserEmail, &m.Name, &m.Description,
			&m.CurrentRevisionID, &m.RevisionNumber, &m.CreatedAt, &m.UpdatedAt, &m.AssetCount,
		); err != nil {
			return nil, err
		}
		m.Permission = models.PermissionEditor
		m.SavedByCurrentUser = true
		m.SharingStatus = "private"
		list = append(list, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *MSSQLMapRepository) ListSharedWithMe(ctx context.Context, userID string) ([]*models.Map, error) {
	query := `
		SELECT m.id, m.owner_user_id, u.name, u.email, m.name, ISNULL(m.description, ''),
		       ISNULL(m.current_revision_id, ''), ISNULL(m.revision_number, 1), m.created_at, m.updated_at,
		       s.permission, (SELECT COUNT(1) FROM dbo.Map_Assets WHERE map_id = m.id) AS asset_count,
		       s.shared_by, ISNULL(inviter.name, ''),
		       CAST(CASE WHEN EXISTS (
		           SELECT 1 FROM dbo.Map_Revisions r
		           WHERE r.map_id = m.id AND r.created_by = @p1
		       ) THEN 1 ELSE 0 END AS BIT) AS saved_by_current_user
		FROM dbo.Map_Shares s
		INNER JOIN dbo.Maps m ON m.id = s.map_id
		LEFT JOIN dbo.Users u ON u.id = m.owner_user_id
		LEFT JOIN dbo.Users inviter ON inviter.id = s.shared_by
		WHERE s.user_id = @p1
		ORDER BY s.updated_at DESC;
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.Map
	for rows.Next() {
		var m models.Map
		var perm string
		if err := rows.Scan(
			&m.ID, &m.OwnerUserID, &m.OwnerUserName, &m.OwnerUserEmail, &m.Name, &m.Description,
			&m.CurrentRevisionID, &m.RevisionNumber, &m.CreatedAt, &m.UpdatedAt, &perm, &m.AssetCount,
			&m.SharedBy, &m.SharedByName,
			&m.SavedByCurrentUser,
		); err != nil {
			return nil, err
		}
		m.Permission = models.CleanSharePermission(perm)
		m.SharingStatus = "shared_with_me"
		list = append(list, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *MSSQLMapRepository) CreateMap(ctx context.Context, ownerUserID, name, description string) (*models.Map, error) {
	now := time.Now().UTC()
	mapID := fmt.Sprintf("map_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", ownerUserID, name, now.UnixNano())))[:20]

	query := `
		INSERT INTO dbo.Maps (id, owner_user_id, name, description, revision_number, created_by, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, 1, @p2, @p5, @p5);
	`
	_, err := r.db.ExecContext(ctx, query, mapID, ownerUserID, name, description, now)
	if err != nil {
		return nil, err
	}

	// Insert initial revision
	revID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-1-%d", mapID, now.UnixNano())))[:20]
	revQuery := `
		INSERT INTO dbo.Map_Revisions (id, map_id, revision_number, configuration_json, revision_description, created_by, created_at)
		VALUES (@p1, @p2, 1, @p3, 'Initial creation', @p4, @p5);
	`
	_, err = r.db.ExecContext(ctx, revQuery, revID, mapID, `{"layers":[],"control_points":[]}`, ownerUserID, now)
	if err != nil {
		return nil, err
	}

	return &models.Map{
		ID:                mapID,
		OwnerUserID:       ownerUserID,
		Name:              name,
		Description:       description,
		RevisionNumber:    1,
		CurrentRevisionID: revID,
		CreatedBy:         ownerUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
		Permission:        models.PermissionEditor,
		SharingStatus:     "private",
	}, nil
}

func (r *MSSQLMapRepository) CreateMapFromConfiguration(
	ctx context.Context,
	ownerUserID, authorName string,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	if r == nil || r.db == nil {
		return nil, models.ErrPersistenceUnavailable
	}
	if ownerUserID == "" || commit.MapID == "" || commit.Name == "" || commit.ConfigurationJSON == "" {
		return nil, models.ErrInvalidMapConfiguration
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin map configuration transaction: %w", err)
	}
	defer tx.Rollback()

	for _, location := range commit.AssetLocations {
		var storedOwner string
		err := tx.QueryRowContext(ctx, `
			SELECT owner_user_id
			FROM dbo.Map_Assets WITH (UPDLOCK, HOLDLOCK)
			WHERE id = @p1;
		`, location.AssetID).Scan(&storedOwner)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, models.ErrMapAssetNotFound
		}
		if err != nil {
			return nil, err
		}
		if storedOwner != ownerUserID || location.OwnerUserID != ownerUserID {
			return nil, models.ErrMapAccessDenied
		}
	}

	now := time.Now().UTC()
	revisionID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-1-%d", commit.MapID, now.UnixNano())))[:20]
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dbo.Maps (id, owner_user_id, name, description, revision_number, created_by, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, 1, @p2, @p5, @p5);
	`, commit.MapID, ownerUserID, commit.Name, commit.Description, now); err != nil {
		return nil, err
	}
	for _, location := range commit.AssetLocations {
		result, err := tx.ExecContext(ctx, `
			UPDATE dbo.Map_Assets
			SET map_id = @p1, storage_path = @p2
			WHERE id = @p3 AND owner_user_id = @p4;
		`, commit.MapID, location.StoragePath, location.AssetID, ownerUserID)
		if err != nil {
			return nil, err
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			if err != nil {
				return nil, err
			}
			return nil, models.ErrMapAssetNotFound
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dbo.Map_Revisions (id, map_id, revision_number, configuration_json, revision_description, created_by, created_at)
		VALUES (@p1, @p2, 1, @p3, @p4, @p5, @p6);
	`, revisionID, commit.MapID, commit.ConfigurationJSON, commit.Description, ownerUserID, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE dbo.Maps SET current_revision_id = @p1 WHERE id = @p2;
	`, revisionID, commit.MapID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	createdMap := &models.Map{
		ID:                commit.MapID,
		OwnerUserID:       ownerUserID,
		OwnerUserName:     authorName,
		Name:              commit.Name,
		Description:       commit.Description,
		CurrentRevisionID: revisionID,
		RevisionNumber:    1,
		CreatedBy:         ownerUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
		LastModifiedUser:  authorName,
		AssetCount:        len(commit.AssetLocations),
		Permission:        models.PermissionEditor,
		SharingStatus:     "private",
	}
	createdRevision := &models.MapRevision{
		ID:                  revisionID,
		MapID:               commit.MapID,
		RevisionNumber:      1,
		ConfigurationJSON:   commit.ConfigurationJSON,
		RevisionDescription: commit.Description,
		CreatedBy:           ownerUserID,
		CreatedByName:       authorName,
		CreatedAt:           now,
	}
	return &models.MapConfigurationResult{Map: createdMap, Revision: createdRevision}, nil
}

func (r *MSSQLMapRepository) GetMap(ctx context.Context, mapID, userID string) (*models.Map, error) {
	query := `
		SELECT m.id, m.owner_user_id, u.name, u.email, m.name, ISNULL(m.description, ''),
		       ISNULL(m.current_revision_id, ''), ISNULL(m.revision_number, 1), m.created_at, m.updated_at,
		       ISNULL(s.permission, '') AS share_perm
		FROM dbo.Maps m
		LEFT JOIN dbo.Users u ON u.id = m.owner_user_id
		LEFT JOIN dbo.Map_Shares s ON s.map_id = m.id AND s.user_id = @p2
		WHERE m.id = @p1;
	`
	var m models.Map
	var sharePerm string
	err := r.db.QueryRowContext(ctx, query, mapID, userID).Scan(
		&m.ID, &m.OwnerUserID, &m.OwnerUserName, &m.OwnerUserEmail, &m.Name, &m.Description,
		&m.CurrentRevisionID, &m.RevisionNumber, &m.CreatedAt, &m.UpdatedAt, &sharePerm,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, models.ErrMapNotFound
		}
		return nil, err
	}

	if m.OwnerUserID == userID {
		m.Permission = models.PermissionEditor
		m.SharingStatus = "private"
	} else if sharePerm != "" {
		m.Permission = models.CleanSharePermission(sharePerm)
		m.SharingStatus = "shared_with_me"
	} else {
		return nil, models.ErrMapAccessDenied
	}

	// Check lock
	lock, _ := r.GetLock(ctx, mapID)
	m.Lock = lock

	return &m, nil
}

func (r *MSSQLMapRepository) UpdateMap(ctx context.Context, mapID, userID, name, description string) (*models.Map, error) {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return nil, err
	}
	if m.Permission != models.PermissionEditor {
		return nil, models.ErrMapAccessDenied
	}

	now := time.Now().UTC()
	query := `UPDATE dbo.Maps SET name = @p1, description = @p2, updated_at = @p3 WHERE id = @p4;`
	_, err = r.db.ExecContext(ctx, query, name, description, now, mapID)
	if err != nil {
		return nil, err
	}
	m.Name = name
	m.Description = description
	m.UpdatedAt = now
	return m, nil
}

func (r *MSSQLMapRepository) DeleteMap(ctx context.Context, mapID, userID string) error {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return err
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}

	_, err = r.db.ExecContext(ctx, `DELETE FROM dbo.Maps WHERE id = @p1;`, mapID)
	return err
}

// Assets
func (r *MSSQLMapRepository) RegisterAsset(ctx context.Context, asset *models.MapAsset) error {
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = time.Now().UTC()
	}
	query := `
		INSERT INTO dbo.Map_Assets (id, map_id, owner_user_id, original_filename, storage_path, file_type, file_size, mime_type, created_at)
		VALUES (@p1, NULLIF(@p2, ''), @p3, @p4, @p5, @p6, @p7, @p8, @p9);
	`
	_, err := r.db.ExecContext(ctx, query, asset.ID, asset.MapID, asset.OwnerUserID, asset.OriginalFilename, asset.StoragePath, asset.FileType, asset.FileSize, asset.MimeType, asset.CreatedAt)
	return err
}

func (r *MSSQLMapRepository) ListUserAssets(ctx context.Context, userID string) ([]*models.MapAsset, error) {
	query := `
		SELECT a.id, ISNULL(a.map_id, ''), ISNULL(m.name, ''), a.owner_user_id, a.original_filename,
		       ISNULL(a.storage_path, ''), a.file_type, a.file_size, ISNULL(a.mime_type, ''), a.created_at
		FROM dbo.Map_Assets a
		LEFT JOIN dbo.Maps m ON m.id = a.map_id
		WHERE a.owner_user_id = @p1
		ORDER BY a.created_at DESC;
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.MapAsset
	for rows.Next() {
		var a models.MapAsset
		if err := rows.Scan(
			&a.ID, &a.MapID, &a.MapName, &a.OwnerUserID, &a.OriginalFilename,
			&a.StoragePath, &a.FileType, &a.FileSize, &a.MimeType, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *MSSQLMapRepository) GetAsset(ctx context.Context, assetID, userID string) (*models.MapAsset, error) {
	query := `
		SELECT a.id, ISNULL(a.map_id, ''), ISNULL(m.name, ''), a.owner_user_id, a.original_filename,
		       a.storage_path, a.file_type, a.file_size, ISNULL(a.mime_type, ''), a.created_at
		FROM dbo.Map_Assets a
		LEFT JOIN dbo.Maps m ON m.id = a.map_id
		WHERE a.id = @p1;
	`
	var a models.MapAsset
	err := r.db.QueryRowContext(ctx, query, assetID).Scan(
		&a.ID, &a.MapID, &a.MapName, &a.OwnerUserID, &a.OriginalFilename,
		&a.StoragePath, &a.FileType, &a.FileSize, &a.MimeType, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if a.OwnerUserID != userID {
		if a.MapID == "" {
			return nil, models.ErrMapAccessDenied
		}
		var hasAccess int
		accessErr := r.db.QueryRowContext(ctx, `
			SELECT CASE WHEN EXISTS (
				SELECT 1 FROM dbo.Maps m
				LEFT JOIN dbo.Map_Shares s ON s.map_id = m.id AND s.user_id = @p2
				WHERE m.id = @p1 AND (m.owner_user_id = @p2 OR s.user_id IS NOT NULL)
			) THEN 1 ELSE 0 END;
		`, a.MapID, userID).Scan(&hasAccess)
		if accessErr != nil || hasAccess == 0 {
			return nil, models.ErrMapAccessDenied
		}
	}
	return &a, nil
}

func (r *MSSQLMapRepository) DeleteAsset(ctx context.Context, assetID, userID string) error {
	a, err := r.GetAsset(ctx, assetID, userID)
	if err != nil {
		return err
	}
	if a.OwnerUserID != userID {
		return errors.New("access denied")
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM dbo.Map_Assets WHERE id = @p1;`, assetID)
	return err
}

// Revisions & Configurations
func (r *MSSQLMapRepository) SaveMapConfig(ctx context.Context, mapID, userID, userName, configJSON, description string, expectedRevision int) (*models.MapRevision, error) {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return nil, err
	}
	if m.Permission != models.PermissionEditor {
		return nil, models.ErrMapAccessDenied
	}

	// Verify Lock
	lock, _ := r.GetLock(ctx, mapID)
	if lock != nil && lock.UserID != userID {
		return nil, models.ErrMapLockHeld
	}

	// Concurrency Check
	if expectedRevision > 0 && m.RevisionNumber != expectedRevision {
		return nil, models.ErrOutdatedRevision
	}

	now := time.Now().UTC()
	nextRevNum := m.RevisionNumber + 1
	revID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%d-%d", mapID, nextRevNum, now.UnixNano())))[:20]

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	insertRev := `
		INSERT INTO dbo.Map_Revisions (id, map_id, revision_number, configuration_json, revision_description, created_by, created_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
	`
	if _, err := tx.ExecContext(ctx, insertRev, revID, mapID, nextRevNum, configJSON, description, userID, now); err != nil {
		return nil, err
	}

	updateMap := `
		UPDATE dbo.Maps
		SET revision_number = @p1, current_revision_id = @p2, updated_at = @p3
		WHERE id = @p4;
	`
	if _, err := tx.ExecContext(ctx, updateMap, nextRevNum, revID, now, mapID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &models.MapRevision{
		ID:                  revID,
		MapID:               mapID,
		RevisionNumber:      nextRevNum,
		ConfigurationJSON:   configJSON,
		RevisionDescription: description,
		CreatedBy:           userID,
		CreatedByName:       userName,
		CreatedAt:           now,
	}, nil
}

func (r *MSSQLMapRepository) SaveMapConfiguration(
	ctx context.Context,
	mapID, userID, authorName string,
	commit models.MapConfigurationCommit,
) (*models.MapConfigurationResult, error) {
	if r == nil || r.db == nil {
		return nil, models.ErrPersistenceUnavailable
	}
	if mapID == "" || userID == "" || commit.ConfigurationJSON == "" {
		return nil, models.ErrInvalidMapConfiguration
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin map configuration transaction: %w", err)
	}
	defer tx.Rollback()

	var current models.Map
	var sharePermission string
	err = tx.QueryRowContext(ctx, `
		SELECT m.owner_user_id, m.name, ISNULL(m.description, ''), ISNULL(m.current_revision_id, ''),
		       ISNULL(m.revision_number, 1), m.created_by, m.created_at, m.updated_at,
		       ISNULL(s.permission, '')
		FROM dbo.Maps AS m WITH (UPDLOCK, HOLDLOCK)
		LEFT JOIN dbo.Map_Shares AS s ON s.map_id = m.id AND s.user_id = @p2
		WHERE m.id = @p1;
	`, mapID, userID).Scan(
		&current.OwnerUserID, &current.Name, &current.Description, &current.CurrentRevisionID,
		&current.RevisionNumber, &current.CreatedBy, &current.CreatedAt, &current.UpdatedAt,
		&sharePermission,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrMapNotFound
	}
	if err != nil {
		return nil, err
	}
	if current.OwnerUserID != userID && models.CleanSharePermission(sharePermission) != models.PermissionEditor {
		return nil, models.ErrMapAccessDenied
	}

	var lockUserID string
	var lockExpiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, expires_at
		FROM dbo.Map_Editing_Locks WITH (UPDLOCK, HOLDLOCK)
		WHERE map_id = @p1;
	`, mapID).Scan(&lockUserID, &lockExpiresAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && time.Now().UTC().Before(lockExpiresAt) && lockUserID != userID {
		return nil, models.ErrMapLockHeld
	}
	if commit.ExpectedRevision > 0 && current.RevisionNumber != commit.ExpectedRevision {
		return nil, models.ErrOutdatedRevision
	}

	for _, location := range commit.AssetLocations {
		var storedOwner, storedMapID string
		err := tx.QueryRowContext(ctx, `
			SELECT owner_user_id, ISNULL(map_id, '')
			FROM dbo.Map_Assets WITH (UPDLOCK, HOLDLOCK)
			WHERE id = @p1;
		`, location.AssetID).Scan(&storedOwner, &storedMapID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, models.ErrMapAssetNotFound
		}
		if err != nil {
			return nil, err
		}
		if storedOwner != current.OwnerUserID && storedOwner != userID {
			return nil, models.ErrMapAccessDenied
		}
	}
	for _, location := range commit.AssetLocations {
		result, err := tx.ExecContext(ctx, `
			UPDATE dbo.Map_Assets SET map_id = @p1, storage_path = @p2
			WHERE id = @p3;
		`, mapID, location.StoragePath, location.AssetID)
		if err != nil {
			return nil, err
		}
		rows, err := result.RowsAffected()
		if err != nil || rows != 1 {
			if err != nil {
				return nil, err
			}
			return nil, models.ErrMapAssetNotFound
		}
	}

	now := time.Now().UTC()
	nextRevision := current.RevisionNumber + 1
	revisionID := fmt.Sprintf("rev_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%d-%d", mapID, nextRevision, now.UnixNano())))[:20]
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dbo.Map_Revisions (id, map_id, revision_number, configuration_json, revision_description, created_by, created_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
	`, revisionID, mapID, nextRevision, commit.ConfigurationJSON, commit.Description, userID, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE dbo.Maps
		SET revision_number = @p1, current_revision_id = @p2, updated_at = @p3
		WHERE id = @p4;
	`, nextRevision, revisionID, now, mapID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	current.ID = mapID
	current.CurrentRevisionID = revisionID
	current.RevisionNumber = nextRevision
	current.UpdatedAt = now
	current.LastModifiedUser = authorName
	current.Permission = models.PermissionEditor
	if current.OwnerUserID == userID {
		current.SharingStatus = "private"
	} else {
		current.SharingStatus = "shared_with_me"
	}
	revision := &models.MapRevision{
		ID:                  revisionID,
		MapID:               mapID,
		RevisionNumber:      nextRevision,
		ConfigurationJSON:   commit.ConfigurationJSON,
		RevisionDescription: commit.Description,
		CreatedBy:           userID,
		CreatedByName:       authorName,
		CreatedAt:           now,
	}
	return &models.MapConfigurationResult{Map: &current, Revision: revision}, nil
}

func (r *MSSQLMapRepository) ListMapRevisions(ctx context.Context, mapID, userID string) ([]*models.MapRevision, error) {
	if _, err := r.GetMap(ctx, mapID, userID); err != nil {
		return nil, err
	}

	query := `
		SELECT r.id, r.map_id, r.revision_number, r.configuration_json, ISNULL(r.revision_description, ''),
		       r.created_by, ISNULL(u.name, ''), r.created_at
		FROM dbo.Map_Revisions r
		LEFT JOIN dbo.Users u ON u.id = r.created_by
		WHERE r.map_id = @p1
		ORDER BY r.revision_number DESC;
	`
	rows, err := r.db.QueryContext(ctx, query, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.MapRevision
	for rows.Next() {
		var rev models.MapRevision
		if err := rows.Scan(
			&rev.ID, &rev.MapID, &rev.RevisionNumber, &rev.ConfigurationJSON,
			&rev.RevisionDescription, &rev.CreatedBy, &rev.CreatedByName, &rev.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, &rev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *MSSQLMapRepository) GetMapRevision(ctx context.Context, mapID, revisionID, userID string) (*models.MapRevision, error) {
	if _, err := r.GetMap(ctx, mapID, userID); err != nil {
		return nil, err
	}

	query := `
		SELECT r.id, r.map_id, r.revision_number, r.configuration_json, ISNULL(r.revision_description, ''),
		       r.created_by, ISNULL(u.name, ''), r.created_at
		FROM dbo.Map_Revisions r
		LEFT JOIN dbo.Users u ON u.id = r.created_by
		WHERE r.map_id = @p1 AND r.id = @p2;
	`
	var rev models.MapRevision
	err := r.db.QueryRowContext(ctx, query, mapID, revisionID).Scan(
		&rev.ID, &rev.MapID, &rev.RevisionNumber, &rev.ConfigurationJSON,
		&rev.RevisionDescription, &rev.CreatedBy, &rev.CreatedByName, &rev.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &rev, nil
}

func (r *MSSQLMapRepository) RestoreMapRevision(ctx context.Context, mapID, revisionID, userID, userName string) (*models.MapRevision, error) {
	targetRev, err := r.GetMapRevision(ctx, mapID, revisionID, userID)
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("Restored from revision %d", targetRev.RevisionNumber)
	return r.SaveMapConfig(ctx, mapID, userID, userName, targetRev.ConfigurationJSON, desc, 0)
}

// Locks
func (r *MSSQLMapRepository) AcquireLock(ctx context.Context, mapID, userID, userName string, ttl time.Duration) (*models.MapEditingLock, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	// Clean expired lock first
	_, _ = r.db.ExecContext(ctx, `DELETE FROM dbo.Map_Editing_Locks WHERE map_id = @p1 AND expires_at < @p2;`, mapID, now)

	query := `
		MERGE INTO dbo.Map_Editing_Locks AS target
		USING (SELECT @p1 AS map_id) AS source
		ON target.map_id = source.map_id
		WHEN MATCHED AND target.user_id = @p2 THEN
			UPDATE SET locked_at = @p3, expires_at = @p4
		WHEN NOT MATCHED THEN
			INSERT (map_id, user_id, user_name, locked_at, expires_at)
			VALUES (@p1, @p2, @p5, @p3, @p4);
	`
	res, err := r.db.ExecContext(ctx, query, mapID, userID, now, expiresAt, userName)
	if err != nil {
		// Lock held by someone else
		currentLock, _ := r.GetLock(ctx, mapID)
		if currentLock != nil && currentLock.UserID != userID {
			return currentLock, models.ErrMapLockHeld
		}
		return nil, err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		currentLock, _ := r.GetLock(ctx, mapID)
		return currentLock, models.ErrMapLockHeld
	}

	return &models.MapEditingLock{
		MapID:     mapID,
		UserID:    userID,
		UserName:  userName,
		LockedAt:  now,
		ExpiresAt: expiresAt,
	}, nil
}

func (r *MSSQLMapRepository) RenewLock(ctx context.Context, mapID, userID string, ttl time.Duration) (*models.MapEditingLock, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	// Check if another user holds an active lock
	var activeHolder string
	checkQuery := `SELECT user_name FROM dbo.Map_Editing_Locks WHERE map_id = @p1 AND user_id <> @p2 AND expires_at > @p3;`
	err := r.db.QueryRowContext(ctx, checkQuery, mapID, userID, now).Scan(&activeHolder)
	if err == nil && activeHolder != "" {
		return nil, models.ErrMapLockHeld
	}

	// Upsert / renew lock for this user
	upsertQuery := `
		MERGE INTO dbo.Map_Editing_Locks AS target
		USING (SELECT @p1 AS map_id) AS source
		ON (target.map_id = source.map_id)
		WHEN MATCHED AND (target.user_id = @p2 OR target.expires_at <= @p3) THEN
			UPDATE SET user_id = @p2, user_name = (SELECT name FROM dbo.Users WHERE id = @p2), locked_at = @p3, expires_at = @p4
		WHEN NOT MATCHED THEN
			INSERT (map_id, user_id, user_name, locked_at, expires_at)
			VALUES (@p1, @p2, (SELECT name FROM dbo.Users WHERE id = @p2), @p3, @p4);
	`
	_, err = r.db.ExecContext(ctx, upsertQuery, mapID, userID, now, expiresAt)
	if err != nil {
		return nil, err
	}

	return &models.MapEditingLock{
		MapID:     mapID,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}, nil
}

func (r *MSSQLMapRepository) ReleaseLock(ctx context.Context, mapID, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM dbo.Map_Editing_Locks WHERE map_id = @p1 AND user_id = @p2;`, mapID, userID)
	return err
}

func (r *MSSQLMapRepository) GetLock(ctx context.Context, mapID string) (*models.MapEditingLock, error) {
	now := time.Now().UTC()
	query := `
		SELECT map_id, user_id, user_name, locked_at, expires_at
		FROM dbo.Map_Editing_Locks
		WHERE map_id = @p1 AND expires_at > @p2;
	`
	var lock models.MapEditingLock
	err := r.db.QueryRowContext(ctx, query, mapID, now).Scan(&lock.MapID, &lock.UserID, &lock.UserName, &lock.LockedAt, &lock.ExpiresAt)
	if err != nil {
		return nil, nil
	}
	return &lock, nil
}

// Sharing (Gmail SMTP Invitations)
func (r *MSSQLMapRepository) CreateShareInvitation(ctx context.Context, mapID, invitedBy, inviterName, email string, perm models.SharePermission, token string, expiresAt time.Time) (*models.MapShareInvitation, error) {
	m, err := r.GetMap(ctx, mapID, invitedBy)
	if err != nil {
		return nil, err
	}
	if m.OwnerUserID != invitedBy {
		return nil, models.ErrMapAccessDenied
	}

	var currentMembers int
	countQuery := `
		SELECT (SELECT COUNT(1) FROM dbo.Map_Shares WHERE map_id = @p1) +
		       (SELECT COUNT(1) FROM dbo.Map_Share_Invitations WHERE map_id = @p1 AND status = 'pending' AND expires_at > GETUTCDATE());
	`
	if err := r.db.QueryRowContext(ctx, countQuery, mapID).Scan(&currentMembers); err == nil {
		if 1+currentMembers >= 10 {
			return nil, errors.New("map member cap reached (maximum 10 members)")
		}
	}

	tokenHash := HashShareToken(token)
	invID := fmt.Sprintf("inv_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", mapID, email, time.Now().UnixNano())))[:20]
	now := time.Now().UTC()

	query := `
		INSERT INTO dbo.Map_Share_Invitations (id, map_id, email, permission, token_hash, status, invited_by, created_at, expires_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, 'pending', @p6, @p7, @p8);
	`
	_, err = r.db.ExecContext(ctx, query, invID, mapID, email, string(perm), tokenHash, invitedBy, now, expiresAt)
	if err != nil {
		return nil, err
	}

	return &models.MapShareInvitation{
		ID:          invID,
		MapID:       mapID,
		MapName:     m.Name,
		Email:       email,
		Permission:  perm,
		Token:       token,
		Status:      "pending",
		InvitedBy:   invitedBy,
		InviterName: inviterName,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}, nil
}

func (r *MSSQLMapRepository) GetInvitationByToken(ctx context.Context, token string) (*models.MapShareInvitation, error) {
	tokenHash := HashShareToken(token)
	now := time.Now().UTC()
	query := `
		SELECT i.id, i.map_id, m.name, i.email, i.permission, i.status, i.invited_by, ISNULL(u.name, ''), i.created_at, i.expires_at
		FROM dbo.Map_Share_Invitations i
		INNER JOIN dbo.Maps m ON m.id = i.map_id
		LEFT JOIN dbo.Users u ON u.id = i.invited_by
		WHERE i.token_hash = @p1 AND i.status = 'pending' AND i.expires_at > @p2;
	`
	var inv models.MapShareInvitation
	var perm string
	err := r.db.QueryRowContext(ctx, query, tokenHash, now).Scan(
		&inv.ID, &inv.MapID, &inv.MapName, &inv.Email, &perm, &inv.Status, &inv.InvitedBy, &inv.InviterName, &inv.CreatedAt, &inv.ExpiresAt,
	)
	if err != nil {
		return nil, models.ErrInvitationNotFound
	}
	inv.Permission = models.CleanSharePermission(perm)
	inv.Token = token
	return &inv, nil
}

func (r *MSSQLMapRepository) AcceptShareInvitation(ctx context.Context, token, userID, userEmail, userName string) (*models.MapShare, error) {
	inv, err := r.GetInvitationByToken(ctx, token)
	if err != nil {
		// Idempotency check: if invitation was already accepted, find and return existing share
		tokenHash := HashShareToken(token)
		var existingMapID, invEmail, invStatus string
		checkQuery := `SELECT map_id, email, status FROM dbo.Map_Share_Invitations WHERE token_hash = @p1;`
		if checkErr := r.db.QueryRowContext(ctx, checkQuery, tokenHash).Scan(&existingMapID, &invEmail, &invStatus); checkErr == nil {
			if !strings.EqualFold(strings.TrimSpace(invEmail), strings.TrimSpace(userEmail)) {
				return nil, models.ErrInvitationEmailMismatch
			}
			if invStatus == "accepted" {
				shareQuery := `
					SELECT s.id, s.map_id, m.name, s.user_id, u.name, u.email, s.permission, s.shared_by,
					       ISNULL(inviter.name, ''), s.created_at, s.updated_at
					FROM dbo.Map_Shares s
					INNER JOIN dbo.Maps m ON m.id = s.map_id
					INNER JOIN dbo.Users u ON u.id = s.user_id
					LEFT JOIN dbo.Users inviter ON inviter.id = s.shared_by
					WHERE s.map_id = @p1 AND s.user_id = @p2;
				`
				var existingShare models.MapShare
				var perm string
				if sErr := r.db.QueryRowContext(ctx, shareQuery, existingMapID, userID).Scan(
					&existingShare.ID, &existingShare.MapID, &existingShare.MapName,
					&existingShare.UserID, &existingShare.UserName, &existingShare.UserEmail,
					&perm, &existingShare.SharedBy, &existingShare.SharedByName, &existingShare.CreatedAt, &existingShare.UpdatedAt,
				); sErr == nil {
					existingShare.Permission = models.CleanSharePermission(perm)
					return &existingShare, nil
				}
			}
		}
		return nil, err
	}

	if !strings.EqualFold(strings.TrimSpace(inv.Email), strings.TrimSpace(userEmail)) {
		return nil, models.ErrInvitationEmailMismatch
	}

	now := time.Now().UTC()
	shareID := fmt.Sprintf("share_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%s-%d", inv.MapID, userID, now.UnixNano())))[:20]

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	queryShare := `
		MERGE INTO dbo.Map_Shares AS target
		USING (SELECT @p1 AS map_id, @p2 AS user_id) AS source
		ON target.map_id = source.map_id AND target.user_id = source.user_id
		WHEN MATCHED THEN
			UPDATE SET permission = @p3, updated_at = @p4
		WHEN NOT MATCHED THEN
			INSERT (id, map_id, user_id, permission, shared_by, created_at, updated_at)
			VALUES (@p5, @p1, @p2, @p3, @p6, @p4, @p4);
	`
	if _, err := tx.ExecContext(ctx, queryShare, inv.MapID, userID, string(inv.Permission), now, shareID, inv.InvitedBy); err != nil {
		return nil, err
	}

	updateInv := `UPDATE dbo.Map_Share_Invitations SET status = 'accepted' WHERE token_hash = @p1;`
	if _, err := tx.ExecContext(ctx, updateInv, HashShareToken(token)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &models.MapShare{
		ID:           shareID,
		MapID:        inv.MapID,
		MapName:      inv.MapName,
		UserID:       userID,
		UserName:     userName,
		UserEmail:    userEmail,
		Permission:   inv.Permission,
		SharedBy:     inv.InvitedBy,
		SharedByName: inv.InviterName,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (r *MSSQLMapRepository) DeclineShareInvitation(ctx context.Context, token string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE dbo.Map_Share_Invitations SET status = 'declined' WHERE token_hash = @p1 AND status = 'pending';`, HashShareToken(token))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.ErrInvitationNotFound
	}
	return nil
}

func (r *MSSQLMapRepository) ListMapShares(ctx context.Context, mapID, userID string) ([]*models.MapShare, []*models.MapShareInvitation, error) {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return nil, nil, err
	}
	if m.OwnerUserID != userID {
		return nil, nil, models.ErrMapAccessDenied
	}

	queryShares := `
		SELECT s.id, s.map_id, s.user_id, u.name, u.email, s.permission, s.shared_by,
		       ISNULL(inviter.name, ''), s.created_at, s.updated_at
		FROM dbo.Map_Shares s
		INNER JOIN dbo.Users u ON u.id = s.user_id
		LEFT JOIN dbo.Users inviter ON inviter.id = s.shared_by
		WHERE s.map_id = @p1
		ORDER BY s.updated_at DESC;
	`
	rows, err := r.db.QueryContext(ctx, queryShares, mapID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var shares []*models.MapShare
	for rows.Next() {
		var s models.MapShare
		var perm string
		if err := rows.Scan(&s.ID, &s.MapID, &s.UserID, &s.UserName, &s.UserEmail, &perm, &s.SharedBy, &s.SharedByName, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, nil, err
		}
		s.Permission = models.CleanSharePermission(perm)
		shares = append(shares, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	queryInvs := `
		SELECT i.id, i.map_id, i.email, i.permission, i.status, i.invited_by, i.created_at, i.expires_at
		FROM dbo.Map_Share_Invitations i
		WHERE i.map_id = @p1 AND i.status = 'pending' AND i.expires_at > @p2
		ORDER BY i.created_at DESC;
	`
	invRows, err := r.db.QueryContext(ctx, queryInvs, mapID, now)
	if err != nil {
		return nil, nil, err
	}
	defer invRows.Close()

	var invs []*models.MapShareInvitation
	for invRows.Next() {
		var inv models.MapShareInvitation
		var perm string
		if err := invRows.Scan(&inv.ID, &inv.MapID, &inv.Email, &perm, &inv.Status, &inv.InvitedBy, &inv.CreatedAt, &inv.ExpiresAt); err != nil {
			return nil, nil, err
		}
		inv.Permission = models.CleanSharePermission(perm)
		invs = append(invs, &inv)
	}
	if err := invRows.Err(); err != nil {
		return nil, nil, err
	}

	return shares, invs, nil
}

func (r *MSSQLMapRepository) UpdateMapSharePermission(ctx context.Context, mapID, shareID, userID string, perm models.SharePermission) error {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return err
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `UPDATE dbo.Map_Shares SET permission = @p1, updated_at = @p2 WHERE id = @p3 AND map_id = @p4;`, string(perm), now, shareID, mapID)
	return err
}

func (r *MSSQLMapRepository) RevokeMapShare(ctx context.Context, mapID, shareID, userID string) error {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return err
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM dbo.Map_Shares WHERE id = @p1 AND map_id = @p2;`, shareID, mapID)
	return err
}

// Share Links
func (r *MSSQLMapRepository) CreateShareLink(ctx context.Context, mapID, createdBy string, perm models.SharePermission, isPublic bool, expiresAt *time.Time, token string) (*models.ShareLink, error) {
	m, err := r.GetMap(ctx, mapID, createdBy)
	if err != nil {
		return nil, err
	}
	if m.OwnerUserID != createdBy {
		return nil, models.ErrMapAccessDenied
	}

	linkID := fmt.Sprintf("link_%x", sha256.Sum256(fmt.Appendf(nil, "%s-%d", mapID, time.Now().UnixNano())))[:20]
	tokenHash := HashShareToken(token)
	now := time.Now().UTC()

	query := `
		INSERT INTO dbo.Share_Links (id, map_id, token_hash, permission, is_public, expires_at, created_by, created_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8);
	`
	_, err = r.db.ExecContext(ctx, query, linkID, mapID, tokenHash, string(perm), isPublic, expiresAt, createdBy, now)
	if err != nil {
		return nil, err
	}

	return &models.ShareLink{
		ID:         linkID,
		MapID:      mapID,
		Token:      token,
		Permission: perm,
		IsPublic:   isPublic,
		ExpiresAt:  expiresAt,
		CreatedBy:  createdBy,
		CreatedAt:  now,
	}, nil
}

func (r *MSSQLMapRepository) GetShareLinkByToken(ctx context.Context, token string) (*models.ShareLink, *models.Map, error) {
	tokenHash := HashShareToken(token)
	now := time.Now().UTC()

	query := `
		SELECT l.id, l.map_id, l.permission, l.is_public, l.expires_at, l.created_by, l.created_at,
		       m.owner_user_id, u.name, u.email, m.name, ISNULL(m.description, ''), ISNULL(m.current_revision_id, ''), ISNULL(m.revision_number, 1), m.created_at, m.updated_at
		FROM dbo.Share_Links l
		INNER JOIN dbo.Maps m ON m.id = l.map_id
		LEFT JOIN dbo.Users u ON u.id = m.owner_user_id
		WHERE l.token_hash = @p1 AND (l.expires_at IS NULL OR l.expires_at > @p2);
	`
	var link models.ShareLink
	var m models.Map
	var perm string
	err := r.db.QueryRowContext(ctx, query, tokenHash, now).Scan(
		&link.ID, &link.MapID, &perm, &link.IsPublic, &link.ExpiresAt, &link.CreatedBy, &link.CreatedAt,
		&m.OwnerUserID, &m.OwnerUserName, &m.OwnerUserEmail, &m.Name, &m.Description, &m.CurrentRevisionID, &m.RevisionNumber, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, nil, models.ErrShareLinkNotFound
	}
	link.Permission = models.CleanSharePermission(perm)
	m.ID = link.MapID
	m.Permission = link.Permission
	m.SharingStatus = "shared_link"
	return &link, &m, nil
}

func (r *MSSQLMapRepository) ListShareLinks(ctx context.Context, mapID, userID string) ([]*models.ShareLink, error) {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return nil, err
	}
	if m.OwnerUserID != userID {
		return nil, models.ErrMapAccessDenied
	}

	query := `
		SELECT id, map_id, permission, is_public, expires_at, created_by, created_at
		FROM dbo.Share_Links
		WHERE map_id = @p1
		ORDER BY created_at DESC;
	`
	rows, err := r.db.QueryContext(ctx, query, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.ShareLink
	for rows.Next() {
		var l models.ShareLink
		var perm string
		if err := rows.Scan(&l.ID, &l.MapID, &perm, &l.IsPublic, &l.ExpiresAt, &l.CreatedBy, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Permission = models.CleanSharePermission(perm)
		list = append(list, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *MSSQLMapRepository) RevokeShareLink(ctx context.Context, mapID, linkID, userID string) error {
	m, err := r.GetMap(ctx, mapID, userID)
	if err != nil {
		return err
	}
	if m.OwnerUserID != userID {
		return models.ErrMapAccessDenied
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM dbo.Share_Links WHERE id = @p1 AND map_id = @p2;`, linkID, mapID)
	return err
}
