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
	ErrProjectNotFound     = errors.New("project not found")
	ErrUnauthorized        = errors.New("unauthorized access to project")
	ErrForbidden           = errors.New("insufficient permissions for this action")
	ErrMemberAlreadyExists = errors.New("user is already a collaborator or has a pending invitation")
	ErrMemberNotFound      = errors.New("member not found")
	ErrInvitationNotFound  = errors.New("invitation not found or already processed")
	ErrCannotInviteOwner   = errors.New("cannot invite the project owner")
	ErrCannotModifyOwner   = errors.New("cannot alter or remove project owner")
)

type ProjectRepository struct {
	mu          sync.RWMutex
	projectsDir string
	projects    map[string]*models.UserProject              // Key: project ID
	members     map[string]map[string]*models.ProjectMember // Key: project ID -> member ID -> member
	activities  map[string][]*models.ProjectActivity        // Key: project ID -> activity list
}

func NewProjectRepository(storageDir string) (*ProjectRepository, error) {
	projDir := filepath.Join(storageDir, "projects")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create projects storage directory: %w", err)
	}

	repo := &ProjectRepository{
		projectsDir: projDir,
		projects:    make(map[string]*models.UserProject),
		members:     make(map[string]map[string]*models.ProjectMember),
		activities:  make(map[string][]*models.ProjectActivity),
	}

	if err := repo.load(); err != nil {
		return nil, fmt.Errorf("failed to load projects: %w", err)
	}

	return repo, nil
}

func (r *ProjectRepository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.projectsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projID := entry.Name()
		projFile := filepath.Join(r.projectsDir, projID, "project.json")
		if _, err := os.Stat(projFile); err == nil {
			data, err := os.ReadFile(projFile)
			if err == nil {
				var p models.UserProject
				if err := json.Unmarshal(data, &p); err == nil && p.ID != "" {
					r.projects[p.ID] = &p
					r.members[p.ID] = make(map[string]*models.ProjectMember)
					r.activities[p.ID] = make([]*models.ProjectActivity, 0)

					// Load members if members.json exists
					membersFile := filepath.Join(r.projectsDir, projID, "members.json")
					if mData, mErr := os.ReadFile(membersFile); mErr == nil && len(mData) > 0 {
						var mList []*models.ProjectMember
						if jErr := json.Unmarshal(mData, &mList); jErr == nil {
							for _, m := range mList {
								r.members[p.ID][m.ID] = m
							}
						}
					}

					// Load activities if activity.json exists
					actFile := filepath.Join(r.projectsDir, projID, "activity.json")
					if aData, aErr := os.ReadFile(actFile); aErr == nil && len(aData) > 0 {
						var aList []*models.ProjectActivity
						if jErr := json.Unmarshal(aData, &aList); jErr == nil {
							r.activities[p.ID] = aList
						}
					}
				}
			}
		}
	}
	return nil
}

func (r *ProjectRepository) saveProjectLocked(p *models.UserProject) error {
	dir := filepath.Join(r.projectsDir, p.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, "project.json")
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

func (r *ProjectRepository) saveMembersLocked(projectID string) error {
	dir := filepath.Join(r.projectsDir, projectID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var mList []*models.ProjectMember
	for _, m := range r.members[projectID] {
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

func (r *ProjectRepository) saveActivitiesLocked(projectID string) error {
	dir := filepath.Join(r.projectsDir, projectID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, "activity.json")
	data, err := json.MarshalIndent(r.activities[projectID], "", "  ")
	if err != nil {
		return err
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

func (r *ProjectRepository) logActivityLocked(projectID, userID, userName, action, details string) {
	aidHex, _ := models.GenerateRandomHex(8)
	act := &models.ProjectActivity{
		ID:        "act_" + aidHex,
		ProjectID: projectID,
		UserID:    userID,
		UserName:  userName,
		Action:    action,
		Details:   details,
		Timestamp: time.Now(),
	}

	if r.activities[projectID] == nil {
		r.activities[projectID] = make([]*models.ProjectActivity, 0)
	}
	r.activities[projectID] = append(r.activities[projectID], act)
	_ = r.saveActivitiesLocked(projectID)
}

// Create creates and persists a new project owned by userID.
func (r *ProjectRepository) Create(userID, name, description, ownerName, ownerEmail string) (*models.UserProject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pidHex, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	projectID := "proj_" + pidHex
	now := time.Now()

	p := &models.UserProject{
		ID:          projectID,
		UserID:      userID,
		Name:        name,
		Description: description,
		DatasetIDs:  make([]string, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := r.saveProjectLocked(p); err != nil {
		return nil, fmt.Errorf("failed to save project: %w", err)
	}

	r.projects[projectID] = p
	r.members[projectID] = make(map[string]*models.ProjectMember)
	r.activities[projectID] = make([]*models.ProjectActivity, 0)

	// Add owner as accepted owner member
	memIDHex, _ := models.GenerateRandomHex(16)
	ownerMember := &models.ProjectMember{
		ID:               "mem_" + memIDHex,
		ProjectID:        projectID,
		UserID:           userID,
		UserEmail:        ownerEmail,
		UserName:         ownerName,
		Role:             models.RoleOwner,
		InvitationStatus: models.StatusAccepted,
		InvitedBy:        userID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.members[projectID][ownerMember.ID] = ownerMember
	_ = r.saveMembersLocked(projectID)

	r.logActivityLocked(projectID, userID, ownerName, "project_created", "Project created")

	return p, nil
}

// ListByUser returns all projects owned by userID, sorted by UpdatedAt desc.
func (r *ProjectRepository) ListByUser(userID string) []*models.UserProject {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.UserProject
	for _, p := range r.projects {
		if p.UserID == userID {
			result = append(result, p)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	return result
}

// ListSharedWithUser returns projects shared with the user (accepted membership, role != owner).
func (r *ProjectRepository) ListSharedWithUser(userID string, authRepo *AuthRepository) []*models.SharedProjectView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var views []*models.SharedProjectView

	for projID, members := range r.members {
		p, exists := r.projects[projID]
		if !exists || p.UserID == userID {
			continue // Owned by user or missing
		}

		for _, m := range members {
			if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
				ownerName := "Project Owner"
				ownerEmail := ""
				if authRepo != nil {
					if owner, err := authRepo.GetUserByID(p.UserID); err == nil {
						ownerName = owner.Name
						ownerEmail = owner.Email
					}
				}

				views = append(views, &models.SharedProjectView{
					Project:    p,
					OwnerName:  ownerName,
					OwnerEmail: ownerEmail,
					Role:       m.Role,
					SharedDate: m.UpdatedAt,
				})
				break
			}
		}
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].SharedDate.After(views[j].SharedDate)
	})

	return views
}

// ListPendingInvitations returns pending invitations for a specific user ID or email.
func (r *ProjectRepository) ListPendingInvitations(userID, email string, authRepo *AuthRepository) []*models.PendingInvitationView {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	var list []*models.PendingInvitationView

	for projID, members := range r.members {
		p, exists := r.projects[projID]
		if !exists {
			continue
		}

		for _, m := range members {
			if (m.UserID == userID || strings.EqualFold(m.UserEmail, cleanEmail)) && m.InvitationStatus == models.StatusPending {
				inviterName := "Project Owner"
				if authRepo != nil {
					if inviter, err := authRepo.GetUserByID(m.InvitedBy); err == nil {
						inviterName = inviter.Name
					}
				}

				list = append(list, &models.PendingInvitationView{
					InvitationID: m.ID,
					ProjectID:    projID,
					ProjectName:  p.Name,
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

// GetUserRole returns the role and access state of a user in a project.
func (r *ProjectRepository) GetUserRole(projectID, userID string) (models.ProjectRole, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.projects[projectID]
	if !exists {
		return "", false
	}

	// Owner check
	if p.UserID == userID {
		return models.RoleOwner, true
	}

	// Collaborator check
	members, exists := r.members[projectID]
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

// GetByID checks if the user has access (as owner, editor, or viewer) and returns project.
func (r *ProjectRepository) GetByID(projectID, userID string) (*models.UserProject, models.ProjectRole, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, "", ErrProjectNotFound
	}

	// Owner
	if p.UserID == userID {
		return p, models.RoleOwner, nil
	}

	// Member
	if members, ok := r.members[projectID]; ok {
		for _, m := range members {
			if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
				return p, m.Role, nil
			}
		}
	}

	return nil, "", ErrUnauthorized
}

// Update updates name and description (Owner or Editor).
func (r *ProjectRepository) Update(projectID, userID, name, description string, authRepo *AuthRepository) (*models.UserProject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, ErrProjectNotFound
	}

	isOwner := p.UserID == userID
	isEditor := false
	userName := "User"

	if members, ok := r.members[projectID]; ok {
		for _, m := range members {
			if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
				userName = m.UserName
				if m.Role == models.RoleEditor {
					isEditor = true
				}
			}
		}
	}

	if !isOwner && !isEditor {
		return nil, ErrForbidden
	}

	if isOwner && authRepo != nil {
		if owner, err := authRepo.GetUserByID(userID); err == nil {
			userName = owner.Name
		}
	}

	if name != "" {
		p.Name = name
	}
	p.Description = description
	p.UpdatedAt = time.Now()

	if err := r.saveProjectLocked(p); err != nil {
		return nil, err
	}

	r.logActivityLocked(projectID, userID, userName, "project_modified", fmt.Sprintf("Updated project info: %s", p.Name))

	return p, nil
}

// Delete removes project metadata and files (Owner ONLY).
func (r *ProjectRepository) Delete(projectID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return ErrProjectNotFound
	}
	if p.UserID != userID {
		return ErrForbidden
	}

	delete(r.projects, projectID)
	delete(r.members, projectID)
	delete(r.activities, projectID)

	dir := filepath.Join(r.projectsDir, projectID)
	_ = storage.SafeRemoveAll(dir)

	return nil
}

// InviteMember invites a target user to a project with role (Editor or Viewer). Only Owner can invite.
func (r *ProjectRepository) InviteMember(projectID, ownerID string, targetUser *models.User, role models.ProjectRole, inviterName string) (*models.ProjectMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, ErrProjectNotFound
	}
	if p.UserID != ownerID {
		return nil, ErrForbidden
	}

	if targetUser.ID == p.UserID {
		return nil, ErrCannotInviteOwner
	}

	if role != models.RoleEditor && role != models.RoleViewer {
		return nil, errors.New("invalid role: must be editor or viewer")
	}

	if r.members[projectID] == nil {
		r.members[projectID] = make(map[string]*models.ProjectMember)
	}

	// Check if already active or pending
	cleanEmail := strings.ToLower(strings.TrimSpace(targetUser.Email))
	for _, m := range r.members[projectID] {
		if (m.UserID == targetUser.ID || strings.EqualFold(m.UserEmail, cleanEmail)) &&
			(m.InvitationStatus == models.StatusAccepted || m.InvitationStatus == models.StatusPending) {
			return nil, ErrMemberAlreadyExists
		}
	}

	memIDHex, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	member := &models.ProjectMember{
		ID:               "mem_" + memIDHex,
		ProjectID:        projectID,
		UserID:           targetUser.ID,
		UserEmail:        cleanEmail,
		UserName:         targetUser.Name,
		Role:             role,
		InvitationStatus: models.StatusPending,
		InvitedBy:        ownerID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	r.members[projectID][member.ID] = member
	if err := r.saveMembersLocked(projectID); err != nil {
		delete(r.members[projectID], member.ID)
		return nil, err
	}

	r.logActivityLocked(projectID, ownerID, inviterName, "invitation_sent", fmt.Sprintf("Invited %s (%s) as %s", targetUser.Name, targetUser.Email, role))

	return member, nil
}

// ListMembers returns all collaborators and pending invitations for a project.
func (r *ProjectRepository) ListMembers(projectID, userID string) ([]*models.ProjectMember, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, ErrProjectNotFound
	}

	// Must be at least member or owner
	hasAccess := p.UserID == userID
	if !hasAccess {
		if members, ok := r.members[projectID]; ok {
			for _, m := range members {
				if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
					hasAccess = true
					break
				}
			}
		}
	}
	if !hasAccess {
		return nil, ErrUnauthorized
	}

	var list []*models.ProjectMember
	if members, ok := r.members[projectID]; ok {
		for _, m := range members {
			list = append(list, m)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		// Owner first, then accepted, then pending
		if list[i].Role == models.RoleOwner {
			return true
		}
		if list[j].Role == models.RoleOwner {
			return false
		}
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	return list, nil
}

// UpdateMemberRole updates the role of a collaborator (Owner ONLY).
func (r *ProjectRepository) UpdateMemberRole(projectID, ownerID, memberID string, newRole models.ProjectRole, ownerName string) (*models.ProjectMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, ErrProjectNotFound
	}
	if p.UserID != ownerID {
		return nil, ErrForbidden
	}

	if newRole != models.RoleEditor && newRole != models.RoleViewer {
		return nil, errors.New("invalid role: must be editor or viewer")
	}

	members, ok := r.members[projectID]
	if !ok {
		return nil, ErrMemberNotFound
	}

	m, ok := members[memberID]
	if !ok {
		return nil, ErrMemberNotFound
	}

	if m.Role == models.RoleOwner || m.UserID == p.UserID {
		return nil, ErrCannotModifyOwner
	}

	oldRole := m.Role
	m.Role = newRole
	m.UpdatedAt = time.Now()

	if err := r.saveMembersLocked(projectID); err != nil {
		m.Role = oldRole
		return nil, err
	}

	r.logActivityLocked(projectID, ownerID, ownerName, "permission_changed", fmt.Sprintf("Changed %s's role to %s", m.UserName, newRole))

	return m, nil
}

// RemoveMember removes a collaborator or cancels an invitation (Owner ONLY).
func (r *ProjectRepository) RemoveMember(projectID, ownerID, memberID string, ownerName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return ErrProjectNotFound
	}
	if p.UserID != ownerID {
		return ErrForbidden
	}

	members, ok := r.members[projectID]
	if !ok {
		return ErrMemberNotFound
	}

	m, ok := members[memberID]
	if !ok {
		return ErrMemberNotFound
	}

	if m.Role == models.RoleOwner || m.UserID == p.UserID {
		return ErrCannotModifyOwner
	}

	removedName := m.UserName
	delete(members, memberID)
	_ = r.saveMembersLocked(projectID)

	r.logActivityLocked(projectID, ownerID, ownerName, "collaborator_removed", fmt.Sprintf("Removed %s from project", removedName))

	return nil
}

// RespondInvitation accepts or rejects a pending invitation by the recipient.
func (r *ProjectRepository) RespondInvitation(invitationID, userID, userName string, accept bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var targetMember *models.ProjectMember
	var targetProjectID string

	for projID, members := range r.members {
		if m, ok := members[invitationID]; ok {
			targetMember = m
			targetProjectID = projID
			break
		}
	}

	if targetMember == nil || targetMember.InvitationStatus != models.StatusPending {
		return ErrInvitationNotFound
	}

	if targetMember.UserID != userID {
		return ErrForbidden
	}

	now := time.Now()
	targetMember.UpdatedAt = now

	if accept {
		targetMember.InvitationStatus = models.StatusAccepted
		_ = r.saveMembersLocked(targetProjectID)
		r.logActivityLocked(targetProjectID, userID, userName, "invitation_accepted", fmt.Sprintf("%s accepted the invitation as %s", userName, targetMember.Role))
	} else {
		targetMember.InvitationStatus = models.StatusRejected
		delete(r.members[targetProjectID], targetMember.ID)
		_ = r.saveMembersLocked(targetProjectID)
		r.logActivityLocked(targetProjectID, userID, userName, "invitation_rejected", fmt.Sprintf("%s declined the invitation", userName))
	}

	return nil
}

// ListActivities returns recent activity records for a project (Owner or Collaborator).
func (r *ProjectRepository) ListActivities(projectID, userID string) ([]*models.ProjectActivity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.projects[projectID]
	if !exists {
		return nil, ErrProjectNotFound
	}

	hasAccess := p.UserID == userID
	if !hasAccess {
		if members, ok := r.members[projectID]; ok {
			for _, m := range members {
				if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
					hasAccess = true
					break
				}
			}
		}
	}
	if !hasAccess {
		return nil, ErrUnauthorized
	}

	list := r.activities[projectID]
	if list == nil {
		list = make([]*models.ProjectActivity, 0)
	}

	// Sort newest first
	result := make([]*models.ProjectActivity, len(list))
	copy(result, list)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result, nil
}

// AddDataset associates a dataset ID with a project (Owner or Editor).
func (r *ProjectRepository) AddDataset(projectID, userID, datasetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.projects[projectID]
	if !exists {
		return ErrProjectNotFound
	}

	role, ok := r.getUserRoleLocked(projectID, userID)
	if !ok || (role != models.RoleOwner && role != models.RoleEditor) {
		return ErrForbidden
	}

	for _, id := range p.DatasetIDs {
		if id == datasetID {
			return nil
		}
	}

	p.DatasetIDs = append(p.DatasetIDs, datasetID)
	if p.PrimaryDatasetID == "" {
		p.PrimaryDatasetID = datasetID
	}
	p.UpdatedAt = time.Now()

	return r.saveProjectLocked(p)
}

func (r *ProjectRepository) getUserRoleLocked(projectID, userID string) (models.ProjectRole, bool) {
	p, exists := r.projects[projectID]
	if !exists {
		return "", false
	}
	if p.UserID == userID {
		return models.RoleOwner, true
	}
	if members, ok := r.members[projectID]; ok {
		for _, m := range members {
			if m.UserID == userID && m.InvitationStatus == models.StatusAccepted {
				return m.Role, true
			}
		}
	}
	return "", false
}

// GetProjectDir returns absolute storage directory for a project.
func (r *ProjectRepository) GetProjectDir(projectID string) string {
	return filepath.Join(r.projectsDir, projectID)
}
