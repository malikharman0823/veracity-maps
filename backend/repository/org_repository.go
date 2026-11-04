package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
	"github.com/malikharman0823/veracity-maps/storage"
)

var (
	ErrOrgNotFound            = errors.New("organization not found")
	ErrOrgForbidden           = errors.New("insufficient permissions for this organization")
	ErrOrgMemberAlreadyExists = errors.New("user is already a member or has a pending invitation")
	ErrOrgMemberNotFound      = errors.New("organization member not found")
	ErrOrgInvitationNotFound  = errors.New("invitation not found or already processed")
	ErrCannotRemoveOrgOwner   = errors.New("cannot remove organization owner")
	ErrCannotModifyOrgOwner   = errors.New("cannot alter role of organization owner")
	ErrStorageQuotaExceeded   = errors.New("organization storage quota exceeded")
)

type OrgRepository struct {
	mu         sync.RWMutex
	orgsDir    string
	orgs       map[string]*models.Organization         // Key: org ID -> Org
	members    map[string]map[string]*models.OrgMember // Key: org ID -> member ID -> OrgMember
	activities map[string][]*models.OrgActivity        // Key: org ID -> Activity list
}

func NewOrgRepository(storageDir string) (*OrgRepository, error) {
	orgsDir := filepath.Join(storageDir, "organizations")
	if err := os.MkdirAll(orgsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create organizations storage directory: %w", err)
	}

	repo := &OrgRepository{
		orgsDir:    orgsDir,
		orgs:       make(map[string]*models.Organization),
		members:    make(map[string]map[string]*models.OrgMember),
		activities: make(map[string][]*models.OrgActivity),
	}

	if err := repo.load(); err != nil {
		return nil, fmt.Errorf("failed to load organizations: %w", err)
	}

	return repo, nil
}

func (r *OrgRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.orgsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		orgID := entry.Name()
		orgFile := filepath.Join(r.orgsDir, orgID, "org.json")
		if _, err := os.Stat(orgFile); err == nil {
			data, err := os.ReadFile(orgFile)
			if err == nil {
				var o models.Organization
				if err := json.Unmarshal(data, &o); err == nil && o.ID != "" {
					r.orgs[o.ID] = &o
					r.members[o.ID] = make(map[string]*models.OrgMember)
					r.activities[o.ID] = make([]*models.OrgActivity, 0)

					// Members
					membersFile := filepath.Join(r.orgsDir, orgID, "members.json")
					if mData, mErr := os.ReadFile(membersFile); mErr == nil && len(mData) > 0 {
						var mList []*models.OrgMember
						if jErr := json.Unmarshal(mData, &mList); jErr == nil {
							for _, m := range mList {
								r.members[o.ID][m.ID] = m
							}
						}
					}

					// Activities
					actFile := filepath.Join(r.orgsDir, orgID, "activity.json")
					if aData, aErr := os.ReadFile(actFile); aErr == nil && len(aData) > 0 {
						var aList []*models.OrgActivity
						if jErr := json.Unmarshal(aData, &aList); jErr == nil {
							r.activities[o.ID] = aList
						}
					}
				}
			}
		}
	}
	return nil
}

func (r *OrgRepository) saveOrgLocked(o *models.Organization) error {
	dir := filepath.Join(r.orgsDir, o.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, "org.json")
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

func (r *OrgRepository) saveMembersLocked(orgID string) error {
	dir := filepath.Join(r.orgsDir, orgID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var mList []*models.OrgMember
	for _, m := range r.members[orgID] {
		mList = append(mList, m)
	}

	filePath := filepath.Join(dir, "members.json")
	data, err := json.MarshalIndent(mList, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

func (r *OrgRepository) saveActivitiesLocked(orgID string) error {
	dir := filepath.Join(r.orgsDir, orgID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, "activity.json")
	data, err := json.MarshalIndent(r.activities[orgID], "", "  ")
	if err != nil {
		return err
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

func (r *OrgRepository) logActivityLocked(orgID, userID, userName, action, details string) {
	aidHex, _ := models.GenerateRandomHex(8)
	act := &models.OrgActivity{
		ID:        "oact_" + aidHex,
		OrgID:     orgID,
		UserID:    userID,
		UserName:  userName,
		Action:    action,
		Details:   details,
		Timestamp: time.Now(),
	}

	if r.activities[orgID] == nil {
		r.activities[orgID] = make([]*models.OrgActivity, 0)
	}
	r.activities[orgID] = append(r.activities[orgID], act)
	_ = r.saveActivitiesLocked(orgID)
}

// Create initializes a new organization with the user as Owner.
func (r *OrgRepository) Create(creator *models.User, name, description string) (*models.Organization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	oidHex, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	orgID := "org_" + oidHex
	now := time.Now()

	org := &models.Organization{
		ID:                orgID,
		Name:              strings.TrimSpace(name),
		Description:       strings.TrimSpace(description),
		CreatedBy:         creator.ID,
		StorageUsedBytes:  0,
		StorageQuotaBytes: 10 * 1024 * 1024 * 1024, // 10 GB default quota
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := r.saveOrgLocked(org); err != nil {
		return nil, err
	}

	r.orgs[orgID] = org
	r.members[orgID] = make(map[string]*models.OrgMember)
	r.activities[orgID] = make([]*models.OrgActivity, 0)

	// Add creator as OrgRoleOwner
	midHex, _ := models.GenerateRandomHex(16)
	ownerMember := &models.OrgMember{
		ID:               "omem_" + midHex,
		OrgID:            orgID,
		UserID:           creator.ID,
		UserEmail:        creator.Email,
		UserName:         creator.Name,
		Role:             models.OrgRoleOwner,
		InvitationStatus: models.StatusAccepted,
		InvitedBy:        creator.ID,
		JoinedAt:         now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	r.members[orgID][ownerMember.ID] = ownerMember
	_ = r.saveMembersLocked(orgID)

	r.logActivityLocked(orgID, creator.ID, creator.Name, "org_created", fmt.Sprintf("Created organization %s", org.Name))

	return org, nil
}

// ListByUser returns all organizations where the user has accepted membership.
func (r *OrgRepository) ListByUser(userID string) []*models.Organization {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.Organization
	for orgID, members := range r.members {
		org, exists := r.orgs[orgID]
		if !exists {
			continue
		}
		for _, m := range members {
			if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
				list = append(list, org)
				break
			}
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	return list
}

// GetUserRole returns the user's role in the organization if they are an active member.
func (r *OrgRepository) GetUserRole(orgID, userID string) (models.OrgRole, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	members, exists := r.members[orgID]
	if !exists {
		return "", false
	}

	for _, m := range members {
		if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
			return m.Role, true
		}
	}

	return "", false
}

// GetByID returns the organization if the requesting user is an active member.
func (r *OrgRepository) GetByID(orgID, userID string) (*models.Organization, models.OrgRole, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	org, exists := r.orgs[orgID]
	if !exists {
		return nil, "", ErrOrgNotFound
	}

	members, exists := r.members[orgID]
	if !exists {
		return nil, "", ErrOrgForbidden
	}

	for _, m := range members {
		if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
			return org, m.Role, nil
		}
	}

	return nil, "", ErrOrgForbidden
}

// Update updates name and description (Owner or Admin).
func (r *OrgRepository) Update(orgID, userID, name, description string, user *models.User) (*models.Organization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	org, exists := r.orgs[orgID]
	if !exists {
		return nil, ErrOrgNotFound
	}

	role, ok := r.getUserRoleLocked(orgID, userID)
	if !ok || (role != models.OrgRoleOwner && role != models.OrgRoleAdmin) {
		return nil, ErrOrgForbidden
	}

	if name != "" {
		org.Name = strings.TrimSpace(name)
	}
	org.Description = strings.TrimSpace(description)
	org.UpdatedAt = time.Now()

	if err := r.saveOrgLocked(org); err != nil {
		return nil, err
	}

	r.logActivityLocked(orgID, user.ID, user.Name, "org_settings_updated", fmt.Sprintf("Updated organization settings: %s", org.Name))

	return org, nil
}

// Delete removes an organization and all its assets (Owner ONLY).
func (r *OrgRepository) Delete(orgID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.orgs[orgID]
	if !exists {
		return ErrOrgNotFound
	}

	role, ok := r.getUserRoleLocked(orgID, userID)
	if !ok || role != models.OrgRoleOwner {
		return ErrOrgForbidden
	}

	delete(r.orgs, orgID)
	delete(r.members, orgID)
	delete(r.activities, orgID)

	dir := filepath.Join(r.orgsDir, orgID)
	_ = storage.SafeRemoveAll(dir)

	return nil
}

// InviteMember invites a user to the organization (Owner or Admin).
func (r *OrgRepository) InviteMember(orgID string, inviter *models.User, targetUser *models.User, role models.OrgRole) (*models.OrgMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.orgs[orgID]
	if !exists {
		return nil, ErrOrgNotFound
	}

	inviterRole, ok := r.getUserRoleLocked(orgID, inviter.ID)
	if !ok || (inviterRole != models.OrgRoleOwner && inviterRole != models.OrgRoleAdmin) {
		return nil, ErrOrgForbidden
	}

	// Admins cannot invite someone as Owner or Admin
	if inviterRole == models.OrgRoleAdmin && (role == models.OrgRoleOwner || role == models.OrgRoleAdmin) {
		return nil, ErrOrgForbidden
	}

	if role != models.OrgRoleAdmin && role != models.OrgRoleEditor && role != models.OrgRoleViewer {
		role = models.OrgRoleViewer
	}

	if r.members[orgID] == nil {
		r.members[orgID] = make(map[string]*models.OrgMember)
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(targetUser.Email))
	for _, m := range r.members[orgID] {
		if (m.UserID == targetUser.ID || strings.EqualFold(m.UserEmail, cleanEmail)) &&
			(m.InvitationStatus == models.StatusAccepted || m.InvitationStatus == models.StatusPending) {
			return nil, ErrOrgMemberAlreadyExists
		}
	}

	midHex, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	member := &models.OrgMember{
		ID:               "omem_" + midHex,
		OrgID:            orgID,
		UserID:           targetUser.ID,
		UserEmail:        cleanEmail,
		UserName:         targetUser.Name,
		Role:             role,
		InvitationStatus: models.StatusPending,
		InvitedBy:        inviter.ID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	r.members[orgID][member.ID] = member
	if err := r.saveMembersLocked(orgID); err != nil {
		delete(r.members[orgID], member.ID)
		return nil, err
	}

	r.logActivityLocked(orgID, inviter.ID, inviter.Name, "member_invited", fmt.Sprintf("Invited %s (%s) as %s", targetUser.Name, targetUser.Email, role))

	return member, nil
}

// ListMembers returns all active members and pending invitations for an organization.
func (r *OrgRepository) ListMembers(orgID, userID string) ([]*models.OrgMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.getUserRoleLocked(orgID, userID)
	if !ok {
		return nil, ErrOrgForbidden
	}

	var list []*models.OrgMember
	if members, exists := r.members[orgID]; exists {
		for _, m := range members {
			list = append(list, m)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Role == models.OrgRoleOwner {
			return true
		}
		if list[j].Role == models.OrgRoleOwner {
			return false
		}
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	return list, nil
}

// UpdateMemberRole updates a member's role (Owner or Admin).
func (r *OrgRepository) UpdateMemberRole(orgID string, operator *models.User, memberID string, newRole models.OrgRole) (*models.OrgMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	org, exists := r.orgs[orgID]
	if !exists {
		return nil, ErrOrgNotFound
	}

	operatorRole, ok := r.getUserRoleLocked(orgID, operator.ID)
	if !ok || (operatorRole != models.OrgRoleOwner && operatorRole != models.OrgRoleAdmin) {
		return nil, ErrOrgForbidden
	}

	members, ok := r.members[orgID]
	if !ok {
		return nil, ErrOrgMemberNotFound
	}

	m, ok := members[memberID]
	if !ok {
		return nil, ErrOrgMemberNotFound
	}

	if m.Role == models.OrgRoleOwner || m.UserID == org.CreatedBy {
		return nil, ErrCannotModifyOrgOwner
	}

	// Admins cannot promote to Admin or Owner, nor demote an Admin
	if operatorRole == models.OrgRoleAdmin {
		if m.Role == models.OrgRoleAdmin || newRole == models.OrgRoleAdmin || newRole == models.OrgRoleOwner {
			return nil, ErrOrgForbidden
		}
	}

	oldRole := m.Role
	m.Role = newRole
	m.UpdatedAt = time.Now()

	if err := r.saveMembersLocked(orgID); err != nil {
		m.Role = oldRole
		return nil, err
	}

	r.logActivityLocked(orgID, operator.ID, operator.Name, "role_changed", fmt.Sprintf("Changed %s's role to %s", m.UserName, newRole))

	return m, nil
}

// RemoveMember removes a member or cancels an invitation.
func (r *OrgRepository) RemoveMember(orgID string, operator *models.User, memberID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	org, exists := r.orgs[orgID]
	if !exists {
		return ErrOrgNotFound
	}

	operatorRole, ok := r.getUserRoleLocked(orgID, operator.ID)
	if !ok || (operatorRole != models.OrgRoleOwner && operatorRole != models.OrgRoleAdmin) {
		return ErrOrgForbidden
	}

	members, ok := r.members[orgID]
	if !ok {
		return ErrOrgMemberNotFound
	}

	m, ok := members[memberID]
	if !ok {
		return ErrOrgMemberNotFound
	}

	if m.Role == models.OrgRoleOwner || m.UserID == org.CreatedBy {
		return ErrCannotRemoveOrgOwner
	}

	// Admins cannot remove other Admins
	if operatorRole == models.OrgRoleAdmin && m.Role == models.OrgRoleAdmin {
		return ErrOrgForbidden
	}

	removedName := m.UserName
	delete(members, memberID)
	_ = r.saveMembersLocked(orgID)

	r.logActivityLocked(orgID, operator.ID, operator.Name, "member_removed", fmt.Sprintf("Removed %s from organization", removedName))

	return nil
}

// ListPendingInvitations returns pending org invitations for a given user.
func (r *OrgRepository) ListPendingInvitations(user *models.User) []*models.PendingOrgInvitationView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*models.PendingOrgInvitationView
	cleanEmail := strings.ToLower(strings.TrimSpace(user.Email))

	for orgID, members := range r.members {
		org, exists := r.orgs[orgID]
		if !exists {
			continue
		}

		for _, m := range members {
			if (m.UserID == user.ID || strings.EqualFold(m.UserEmail, cleanEmail)) && m.InvitationStatus == models.StatusPending {
				inviterName := "Organization Admin"
				if inv, ok := members[m.InvitedBy]; ok {
					inviterName = inv.UserName
				}

				list = append(list, &models.PendingOrgInvitationView{
					InvitationID: m.ID,
					OrgID:        orgID,
					OrgName:      org.Name,
					InvitedBy:    m.InvitedBy,
					InviterName:  inviterName,
					Role:         m.Role,
					CreatedAt:    m.CreatedAt,
				})
			}
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	return list
}

// RespondInvitation processes an invitation response (Accept / Decline).
func (r *OrgRepository) RespondInvitation(invitationID string, user *models.User, accept bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var targetMember *models.OrgMember
	var targetOrgID string

	for orgID, members := range r.members {
		if m, ok := members[invitationID]; ok {
			targetMember = m
			targetOrgID = orgID
			break
		}
	}

	if targetMember == nil || targetMember.InvitationStatus != models.StatusPending {
		return ErrOrgInvitationNotFound
	}

	cleanEmail := strings.ToLower(strings.TrimSpace(user.Email))
	if targetMember.UserID != user.ID && !strings.EqualFold(targetMember.UserEmail, cleanEmail) {
		return ErrOrgForbidden
	}

	now := time.Now()
	targetMember.UpdatedAt = now
	targetMember.UserID = user.ID
	targetMember.UserName = user.Name

	if accept {
		targetMember.InvitationStatus = models.StatusAccepted
		targetMember.JoinedAt = now
		_ = r.saveMembersLocked(targetOrgID)
		r.logActivityLocked(targetOrgID, user.ID, user.Name, "member_joined", fmt.Sprintf("%s joined the organization as %s", user.Name, targetMember.Role))
	} else {
		targetMember.InvitationStatus = models.StatusRejected
		delete(r.members[targetOrgID], targetMember.ID)
		_ = r.saveMembersLocked(targetOrgID)
		r.logActivityLocked(targetOrgID, user.ID, user.Name, "invitation_declined", fmt.Sprintf("%s declined the invitation", user.Name))
	}

	return nil
}

// ListActivities returns recent organizational activities.
func (r *OrgRepository) ListActivities(orgID, userID string) ([]*models.OrgActivity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, ok := r.getUserRoleLocked(orgID, userID)
	if !ok || (role != models.OrgRoleOwner && role != models.OrgRoleAdmin) {
		return nil, ErrOrgForbidden
	}

	list := r.activities[orgID]
	if list == nil {
		list = make([]*models.OrgActivity, 0)
	}

	result := make([]*models.OrgActivity, len(list))
	copy(result, list)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result, nil
}

func (r *OrgRepository) getUserRoleLocked(orgID, userID string) (models.OrgRole, bool) {
	members, exists := r.members[orgID]
	if !exists {
		return "", false
	}
	for _, m := range members {
		if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
			return m.Role, true
		}
	}
	return "", false
}
