package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

type MSSQLProjectRepository struct {
	db       *sql.DB
	authRepo AuthRepo
}

func NewMSSQLProjectRepository(db *sql.DB, authRepo AuthRepo) *MSSQLProjectRepository {
	return &MSSQLProjectRepository{db: db, authRepo: authRepo}
}

func (r *MSSQLProjectRepository) CreateProject(userID, name, description string) (*models.UserProject, error) {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return nil, errors.New("project name is required")
	}

	pid, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	projID := "proj_" + pid

	now := time.Now()
	datasetIDsJSON := "[]"

	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed starting tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	insertProj := `
		INSERT INTO Projects (id, user_id, org_id, owner_type, name, description, primary_dataset_id, dataset_ids, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10);
	`
	_, err = tx.Exec(insertProj, projID, userID, nil, "personal", cleanName, description, nil, datasetIDsJSON, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed creating project in database: %w", err)
	}

	user, err := r.authRepo.GetUserByID(context.Background(), userID)
	userName := "Owner"
	userEmail := ""
	if err == nil && user != nil {
		userName = user.Name
		userEmail = user.Email
	}

	memID, _ := models.GenerateRandomHex(16)
	insertMember := `
		INSERT INTO ProjectMembers (id, project_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10);
	`
	_, err = tx.Exec(insertMember, "mem_"+memID, projID, userID, userEmail, userName, string(models.RoleOwner), string(models.StatusAccepted), userID, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed adding owner membership: %w", err)
	}

	actID, _ := models.GenerateRandomHex(16)
	insertAct := `
		INSERT INTO ProjectActivities (id, project_id, user_id, user_name, action, details, timestamp)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
	`
	_, _ = tx.Exec(insertAct, "act_"+actID, projID, userID, userName, "project_created", fmt.Sprintf("Project '%s' created", cleanName), now)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed committing project creation: %w", err)
	}

	return &models.UserProject{
		ID:          projID,
		UserID:      userID,
		Name:        cleanName,
		Description: description,
		DatasetIDs:  []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (r *MSSQLProjectRepository) GetProject(projectID string) (*models.UserProject, error) {
	query := `
		SELECT id, user_id, org_id, owner_type, name, description, primary_dataset_id, dataset_ids, created_at, updated_at
		FROM Projects WHERE id = @p1;
	`
	var p models.UserProject
	var orgID, ownerType, desc, primaryDS, datasetIDs sql.NullString
	err := r.db.QueryRow(query, projectID).Scan(
		&p.ID, &p.UserID, &orgID, &ownerType, &p.Name, &desc, &primaryDS, &datasetIDs, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed fetching project: %w", err)
	}

	if desc.Valid {
		p.Description = desc.String
	}
	if primaryDS.Valid {
		p.PrimaryDatasetID = primaryDS.String
	}
	if datasetIDs.Valid && datasetIDs.String != "" {
		_ = json.Unmarshal([]byte(datasetIDs.String), &p.DatasetIDs)
	}
	if p.DatasetIDs == nil {
		p.DatasetIDs = []string{}
	}
	return &p, nil
}

func (r *MSSQLProjectRepository) UpdateProject(projectID, name, description string) (*models.UserProject, error) {
	p, err := r.GetProject(projectID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(name) != "" {
		p.Name = strings.TrimSpace(name)
	}
	if description != "" {
		p.Description = strings.TrimSpace(description)
	}
	p.UpdatedAt = time.Now()

	query := `
		UPDATE Projects
		SET name = @p1, description = @p2, updated_at = @p3
		WHERE id = @p4;
	`
	_, err = r.db.Exec(query, p.Name, p.Description, p.UpdatedAt, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed updating project: %w", err)
	}

	return p, nil
}

func (r *MSSQLProjectRepository) DeleteProject(projectID string) error {
	res, err := r.db.Exec("DELETE FROM Projects WHERE id = @p1", projectID)
	if err != nil {
		return fmt.Errorf("failed deleting project: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrProjectNotFound
	}
	return nil
}

func (r *MSSQLProjectRepository) ListUserProjects(userID string) ([]*models.UserProject, error) {
	query := `
		SELECT id, user_id, org_id, owner_type, name, description, primary_dataset_id, dataset_ids, created_at, updated_at
		FROM Projects
		WHERE user_id = @p1
		ORDER BY updated_at DESC;
	`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed listing user projects: %w", err)
	}
	defer rows.Close()

	var projects []*models.UserProject
	for rows.Next() {
		var p models.UserProject
		var orgID, ownerType, desc, primaryDS, datasetIDs sql.NullString
		if err := rows.Scan(&p.ID, &p.UserID, &orgID, &ownerType, &p.Name, &desc, &primaryDS, &datasetIDs, &p.CreatedAt, &p.UpdatedAt); err == nil {
			if desc.Valid {
				p.Description = desc.String
			}
			if primaryDS.Valid {
				p.PrimaryDatasetID = primaryDS.String
			}
			if datasetIDs.Valid && datasetIDs.String != "" {
				_ = json.Unmarshal([]byte(datasetIDs.String), &p.DatasetIDs)
			}
			if p.DatasetIDs == nil {
				p.DatasetIDs = []string{}
			}
			projects = append(projects, &p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}
	return projects, nil
}

func (r *MSSQLProjectRepository) ListSharedProjects(userID string) ([]*models.SharedProjectView, error) {
	query := `
		SELECT p.id, p.user_id, p.org_id, p.owner_type, p.name, p.description, p.primary_dataset_id, p.dataset_ids, p.created_at, p.updated_at,
		       pm.role, pm.created_at, u.name, u.email
		FROM ProjectMembers pm
		INNER JOIN Projects p ON pm.project_id = p.id
		LEFT JOIN Users u ON p.user_id = u.id
		WHERE pm.user_id = @p1 AND pm.role != 'owner' AND pm.invitation_status = 'accepted'
		ORDER BY pm.updated_at DESC;
	`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed listing shared projects: %w", err)
	}
	defer rows.Close()

	var shared []*models.SharedProjectView
	for rows.Next() {
		var p models.UserProject
		var orgID, ownerType, desc, primaryDS, datasetIDs sql.NullString
		var roleStr string
		var sharedDate time.Time
		var ownerName, ownerEmail sql.NullString

		if err := rows.Scan(
			&p.ID, &p.UserID, &orgID, &ownerType, &p.Name, &desc, &primaryDS, &datasetIDs, &p.CreatedAt, &p.UpdatedAt,
			&roleStr, &sharedDate, &ownerName, &ownerEmail,
		); err == nil {
			if desc.Valid {
				p.Description = desc.String
			}
			if primaryDS.Valid {
				p.PrimaryDatasetID = primaryDS.String
			}
			if datasetIDs.Valid && datasetIDs.String != "" {
				_ = json.Unmarshal([]byte(datasetIDs.String), &p.DatasetIDs)
			}
			if p.DatasetIDs == nil {
				p.DatasetIDs = []string{}
			}

			oName := "Project Owner"
			if ownerName.Valid {
				oName = ownerName.String
			}
			oEmail := ""
			if ownerEmail.Valid {
				oEmail = ownerEmail.String
			}

			shared = append(shared, &models.SharedProjectView{
				Project:    &p,
				OwnerName:  oName,
				OwnerEmail: oEmail,
				Role:       models.ProjectRole(roleStr),
				SharedDate: sharedDate,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return shared, nil
}

func (r *MSSQLProjectRepository) CheckPermission(projectID, userID string, requiredRole models.ProjectRole) (*models.ProjectMember, error) {
	proj, err := r.GetProject(projectID)
	if err != nil {
		return nil, err
	}

	if proj.UserID == userID {
		return &models.ProjectMember{
			ProjectID: projectID,
			UserID:    userID,
			Role:      models.RoleOwner,
		}, nil
	}

	query := `
		SELECT id, project_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at
		FROM ProjectMembers
		WHERE project_id = @p1 AND user_id = @p2 AND invitation_status = 'accepted';
	`
	var m models.ProjectMember
	var roleStr, statusStr string
	err = r.db.QueryRow(query, projectID, userID).Scan(
		&m.ID, &m.ProjectID, &m.UserID, &m.UserEmail, &m.UserName, &roleStr, &statusStr, &m.InvitedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("failed checking permission: %w", err)
	}

	m.Role = models.ProjectRole(roleStr)
	m.InvitationStatus = models.InvitationStatus(statusStr)

	roleWeights := map[models.ProjectRole]int{
		models.RoleViewer: 1,
		models.RoleEditor: 2,
		models.RoleOwner:  3,
	}

	if roleWeights[m.Role] < roleWeights[requiredRole] {
		return nil, ErrForbidden
	}

	return &m, nil
}

func (r *MSSQLProjectRepository) ShareProject(projectID, targetEmail, invitedBy string, role models.ProjectRole) (*models.ProjectMember, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(targetEmail))
	proj, err := r.GetProject(projectID)
	if err != nil {
		return nil, err
	}

	inviterUser, _ := r.authRepo.GetUserByID(context.Background(), invitedBy)
	inviterName := "Project Collaborator"
	if inviterUser != nil {
		inviterName = inviterUser.Name
	}

	targetUser, err := r.authRepo.FindUserByEmail(context.Background(), cleanEmail)
	if err != nil || targetUser == nil {
		return nil, fmt.Errorf("user with email %s is not registered", cleanEmail)
	}

	if targetUser.ID == proj.UserID {
		return nil, ErrCannotInviteOwner
	}

	var existingCount int
	err = r.db.QueryRow("SELECT COUNT(*) FROM ProjectMembers WHERE project_id = @p1 AND user_id = @p2", projectID, targetUser.ID).Scan(&existingCount)
	if err == nil && existingCount > 0 {
		return nil, ErrMemberAlreadyExists
	}

	memID, _ := models.GenerateRandomHex(16)
	memberID := "mem_" + memID
	now := time.Now()

	query := `
		INSERT INTO ProjectMembers (id, project_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10);
	`
	_, err = r.db.Exec(query, memberID, projectID, targetUser.ID, cleanEmail, targetUser.Name, string(role), string(models.StatusPending), invitedBy, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed sharing project: %w", err)
	}

	r.AddActivity(projectID, invitedBy, inviterName, "invitation_sent", fmt.Sprintf("Invited %s as %s", cleanEmail, role))

	return &models.ProjectMember{
		ID:               memberID,
		ProjectID:        projectID,
		UserID:           targetUser.ID,
		UserEmail:        cleanEmail,
		UserName:         targetUser.Name,
		Role:             role,
		InvitationStatus: models.StatusPending,
		InvitedBy:        invitedBy,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (r *MSSQLProjectRepository) ListMembers(projectID string) ([]*models.ProjectMember, error) {
	query := `
		SELECT id, project_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at
		FROM ProjectMembers
		WHERE project_id = @p1
		ORDER BY created_at ASC;
	`
	rows, err := r.db.Query(query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed listing members: %w", err)
	}
	defer rows.Close()

	var members []*models.ProjectMember
	for rows.Next() {
		var m models.ProjectMember
		var roleStr, statusStr string
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.UserEmail, &m.UserName, &roleStr, &statusStr, &m.InvitedBy, &m.CreatedAt, &m.UpdatedAt); err == nil {
			m.Role = models.ProjectRole(roleStr)
			m.InvitationStatus = models.InvitationStatus(statusStr)
			members = append(members, &m)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}
	return members, nil
}

func (r *MSSQLProjectRepository) UpdateMemberRole(projectID, memberID string, newRole models.ProjectRole) (*models.ProjectMember, error) {
	var m models.ProjectMember
	var roleStr, statusStr string
	querySelect := `
		SELECT id, project_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at
		FROM ProjectMembers WHERE project_id = @p1 AND id = @p2;
	`
	err := r.db.QueryRow(querySelect, projectID, memberID).Scan(
		&m.ID, &m.ProjectID, &m.UserID, &m.UserEmail, &m.UserName, &roleStr, &statusStr, &m.InvitedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, ErrMemberNotFound
	}

	if roleStr == string(models.RoleOwner) {
		return nil, ErrCannotModifyOwner
	}

	now := time.Now()
	_, err = r.db.Exec("UPDATE ProjectMembers SET role = @p1, updated_at = @p2 WHERE id = @p3", string(newRole), now, memberID)
	if err != nil {
		return nil, fmt.Errorf("failed updating role: %w", err)
	}

	m.Role = newRole
	m.UpdatedAt = now
	return &m, nil
}

func (r *MSSQLProjectRepository) RemoveMember(projectID, memberID string) error {
	var roleStr string
	err := r.db.QueryRow("SELECT role FROM ProjectMembers WHERE project_id = @p1 AND id = @p2", projectID, memberID).Scan(&roleStr)
	if err != nil {
		return ErrMemberNotFound
	}
	if roleStr == string(models.RoleOwner) {
		return ErrCannotModifyOwner
	}

	_, err = r.db.Exec("DELETE FROM ProjectMembers WHERE project_id = @p1 AND id = @p2", projectID, memberID)
	if err != nil {
		return fmt.Errorf("failed removing member: %w", err)
	}
	return nil
}

func (r *MSSQLProjectRepository) ListPendingInvitations(userEmail string) ([]*models.PendingInvitationView, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(userEmail))
	query := `
		SELECT pm.id, pm.project_id, p.name, pm.invited_by, u.name, pm.role, pm.created_at
		FROM ProjectMembers pm
		INNER JOIN Projects p ON pm.project_id = p.id
		LEFT JOIN Users u ON pm.invited_by = u.id
		WHERE pm.user_email = @p1 AND pm.invitation_status = 'pending'
		ORDER BY pm.created_at DESC;
	`
	rows, err := r.db.Query(query, cleanEmail)
	if err != nil {
		return nil, fmt.Errorf("failed querying pending invitations: %w", err)
	}
	defer rows.Close()

	var invitations []*models.PendingInvitationView
	for rows.Next() {
		var inv models.PendingInvitationView
		var roleStr string
		var inviterName sql.NullString
		if err := rows.Scan(&inv.InvitationID, &inv.ProjectID, &inv.ProjectName, &inv.InvitedBy, &inviterName, &roleStr, &inv.CreatedAt); err == nil {
			inv.Role = models.ProjectRole(roleStr)
			if inviterName.Valid {
				inv.InviterName = inviterName.String
			} else {
				inv.InviterName = "Project Owner"
			}
			invitations = append(invitations, &inv)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return invitations, nil
}

func (r *MSSQLProjectRepository) RespondInvitation(invitationID, userID, userEmail string, accept bool) error {
	cleanEmail := strings.ToLower(strings.TrimSpace(userEmail))
	var projectID, currentStatus string
	query := `SELECT project_id, invitation_status FROM ProjectMembers WHERE id = @p1 AND user_email = @p2;`
	err := r.db.QueryRow(query, invitationID, cleanEmail).Scan(&projectID, &currentStatus)
	if err != nil {
		return ErrInvitationNotFound
	}

	if currentStatus != string(models.StatusPending) {
		return ErrInvitationNotFound
	}

	now := time.Now()
	newStatus := models.StatusRejected
	if accept {
		newStatus = models.StatusAccepted
	}

	_, err = r.db.Exec("UPDATE ProjectMembers SET invitation_status = @p1, user_id = @p2, updated_at = @p3 WHERE id = @p4", string(newStatus), userID, now, invitationID)
	if err != nil {
		return fmt.Errorf("failed updating invitation: %w", err)
	}

	action := "invitation_rejected"
	details := fmt.Sprintf("User %s rejected invitation", userEmail)
	if accept {
		action = "invitation_accepted"
		details = fmt.Sprintf("User %s accepted invitation and joined project", userEmail)
	}
	r.AddActivity(projectID, userID, userEmail, action, details)

	return nil
}

func (r *MSSQLProjectRepository) AssociateDataset(projectID, datasetID string) error {
	p, err := r.GetProject(projectID)
	if err != nil {
		return err
	}

	found := false
	for _, id := range p.DatasetIDs {
		if id == datasetID {
			found = true
			break
		}
	}
	if !found {
		p.DatasetIDs = append(p.DatasetIDs, datasetID)
	}
	if p.PrimaryDatasetID == "" {
		p.PrimaryDatasetID = datasetID
	}
	p.UpdatedAt = time.Now()

	dataBytes, _ := json.Marshal(p.DatasetIDs)
	query := `
		UPDATE Projects
		SET primary_dataset_id = @p1, dataset_ids = @p2, updated_at = @p3
		WHERE id = @p4;
	`
	_, err = r.db.Exec(query, p.PrimaryDatasetID, string(dataBytes), p.UpdatedAt, projectID)
	return err
}

func (r *MSSQLProjectRepository) GetActivities(projectID string) ([]*models.ProjectActivity, error) {
	query := `
		SELECT id, project_id, user_id, user_name, action, details, timestamp
		FROM ProjectActivities
		WHERE project_id = @p1
		ORDER BY timestamp DESC;
	`
	rows, err := r.db.Query(query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed fetching project activities: %w", err)
	}
	defer rows.Close()

	var list []*models.ProjectActivity
	for rows.Next() {
		var a models.ProjectActivity
		var details sql.NullString
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.UserID, &a.UserName, &a.Action, &details, &a.Timestamp); err == nil {
			if details.Valid {
				a.Details = details.String
			}
			list = append(list, &a)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}
	return list, nil
}

func (r *MSSQLProjectRepository) AddActivity(projectID, userID, userName, action, details string) {
	actID, _ := models.GenerateRandomHex(16)
	query := `
		INSERT INTO ProjectActivities (id, project_id, user_id, user_name, action, details, timestamp)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
	`
	_, _ = r.db.Exec(query, "act_"+actID, projectID, userID, userName, action, details, time.Now())
}
