package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

type MSSQLOrgRepository struct {
	db       *sql.DB
	authRepo AuthRepo
}

func NewMSSQLOrgRepository(db *sql.DB, authRepo AuthRepo) *MSSQLOrgRepository {
	return &MSSQLOrgRepository{db: db, authRepo: authRepo}
}

func (r *MSSQLOrgRepository) CreateOrg(name, description, createdBy string) (*models.Organization, error) {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return nil, errors.New("organization name is required")
	}

	oid, err := models.GenerateRandomHex(16)
	if err != nil {
		return nil, err
	}
	orgID := "org_" + oid
	now := time.Now()

	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	insertOrg := `
		INSERT INTO Organizations (id, name, description, created_by, storage_used_bytes, storage_quota_bytes, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8);
	`
	_, err = tx.Exec(insertOrg, orgID, cleanName, description, createdBy, 0, 5368709120, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed inserting organization: %w", err)
	}

	user, err := r.authRepo.GetUserByID(createdBy)
	userName := "Owner"
	userEmail := ""
	if err == nil && user != nil {
		userName = user.Name
		userEmail = user.Email
	}

	memID, _ := models.GenerateRandomHex(16)
	insertMember := `
		INSERT INTO OrganizationMembers (id, org_id, user_id, user_email, user_name, role, invitation_status, invited_by, joined_at, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10, @p11);
	`
	_, err = tx.Exec(insertMember, "omem_"+memID, orgID, createdBy, userEmail, userName, string(models.OrgRoleOwner), string(models.StatusAccepted), createdBy, now, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed inserting organization owner: %w", err)
	}

	actID, _ := models.GenerateRandomHex(16)
	insertAct := `
		INSERT INTO OrganizationActivities (id, org_id, user_id, user_name, action, details, timestamp)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
	`
	_, _ = tx.Exec(insertAct, "oact_"+actID, orgID, createdBy, userName, "org_created", fmt.Sprintf("Organization '%s' created", cleanName), now)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed committing organization creation: %w", err)
	}

	return &models.Organization{
		ID:                orgID,
		Name:              cleanName,
		Description:       description,
		CreatedBy:         createdBy,
		StorageUsedBytes:  0,
		StorageQuotaBytes: 5368709120,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (r *MSSQLOrgRepository) GetOrg(orgID string) (*models.Organization, error) {
	query := `
		SELECT id, name, description, created_by, storage_used_bytes, storage_quota_bytes, created_at, updated_at
		FROM Organizations WHERE id = @p1;
	`
	var o models.Organization
	var desc sql.NullString
	err := r.db.QueryRow(query, orgID).Scan(
		&o.ID, &o.Name, &desc, &o.CreatedBy, &o.StorageUsedBytes, &o.StorageQuotaBytes, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrgNotFound
		}
		return nil, fmt.Errorf("failed querying org: %w", err)
	}
	if desc.Valid {
		o.Description = desc.String
	}
	return &o, nil
}

func (r *MSSQLOrgRepository) UpdateOrg(orgID, name, description string) (*models.Organization, error) {
	o, err := r.GetOrg(orgID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(name) != "" {
		o.Name = strings.TrimSpace(name)
	}
	if description != "" {
		o.Description = strings.TrimSpace(description)
	}
	o.UpdatedAt = time.Now()

	query := `
		UPDATE Organizations
		SET name = @p1, description = @p2, updated_at = @p3
		WHERE id = @p4;
	`
	_, err = r.db.Exec(query, o.Name, o.Description, o.UpdatedAt, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed updating organization: %w", err)
	}

	return o, nil
}

func (r *MSSQLOrgRepository) DeleteOrg(orgID string) error {
	res, err := r.db.Exec("DELETE FROM Organizations WHERE id = @p1", orgID)
	if err != nil {
		return fmt.Errorf("failed deleting organization: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrOrgNotFound
	}
	return nil
}

func (r *MSSQLOrgRepository) ListUserOrgs(userID string) ([]*models.Organization, error) {
	query := `
		SELECT o.id, o.name, o.description, o.created_by, o.storage_used_bytes, o.storage_quota_bytes, o.created_at, o.updated_at
		FROM OrganizationMembers om
		INNER JOIN Organizations o ON om.org_id = o.id
		WHERE om.user_id = @p1 AND om.invitation_status = 'accepted'
		ORDER BY o.updated_at DESC;
	`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed listing user organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*models.Organization
	for rows.Next() {
		var o models.Organization
		var desc sql.NullString
		if err := rows.Scan(&o.ID, &o.Name, &desc, &o.CreatedBy, &o.StorageUsedBytes, &o.StorageQuotaBytes, &o.CreatedAt, &o.UpdatedAt); err == nil {
			if desc.Valid {
				o.Description = desc.String
			}
			orgs = append(orgs, &o)
		}
	}
	return orgs, nil
}

func (r *MSSQLOrgRepository) CheckOrgPermission(orgID, userID string, requiredRole models.OrgRole) (*models.OrgMember, error) {
	query := `
		SELECT id, org_id, user_id, user_email, user_name, role, invitation_status, invited_by, joined_at, created_at, updated_at
		FROM OrganizationMembers
		WHERE org_id = @p1 AND user_id = @p2 AND invitation_status = 'accepted';
	`
	var m models.OrgMember
	var roleStr, statusStr string
	var joinedAt sql.NullTime
	err := r.db.QueryRow(query, orgID, userID).Scan(
		&m.ID, &m.OrgID, &m.UserID, &m.UserEmail, &m.UserName, &roleStr, &statusStr, &m.InvitedBy, &joinedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrgForbidden
		}
		return nil, fmt.Errorf("failed checking org permission: %w", err)
	}

	m.Role = models.OrgRole(roleStr)
	m.InvitationStatus = models.InvitationStatus(statusStr)
	if joinedAt.Valid {
		m.JoinedAt = joinedAt.Time
	}

	roleWeights := map[models.OrgRole]int{
		models.OrgRoleViewer: 1,
		models.OrgRoleEditor: 2,
		models.OrgRoleAdmin:  3,
		models.OrgRoleOwner:  4,
	}

	if roleWeights[m.Role] < roleWeights[requiredRole] {
		return nil, ErrOrgForbidden
	}

	return &m, nil
}

func (r *MSSQLOrgRepository) InviteMember(orgID, email, invitedBy string, role models.OrgRole) (*models.OrgMember, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))

	inviterUser, _ := r.authRepo.GetUserByID(invitedBy)
	inviterName := "Organization Admin"
	if inviterUser != nil {
		inviterName = inviterUser.Name
	}

	targetUser, err := r.authRepo.FindUserByEmail(cleanEmail)
	if err != nil || targetUser == nil {
		return nil, fmt.Errorf("user with email %s is not registered", cleanEmail)
	}

	var count int
	_ = r.db.QueryRow("SELECT COUNT(*) FROM OrganizationMembers WHERE org_id = @p1 AND user_id = @p2", orgID, targetUser.ID).Scan(&count)
	if count > 0 {
		return nil, ErrOrgMemberAlreadyExists
	}

	memID, _ := models.GenerateRandomHex(16)
	memberID := "omem_" + memID
	now := time.Now()

	query := `
		INSERT INTO OrganizationMembers (id, org_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, @p10);
	`
	_, err = r.db.Exec(query, memberID, orgID, targetUser.ID, cleanEmail, targetUser.Name, string(role), string(models.StatusPending), invitedBy, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed inviting member: %w", err)
	}

	r.AddActivity(orgID, invitedBy, inviterName, "member_invited", fmt.Sprintf("Invited %s as %s", cleanEmail, role))

	return &models.OrgMember{
		ID:               memberID,
		OrgID:            orgID,
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

func (r *MSSQLOrgRepository) ListMembers(orgID string) ([]*models.OrgMember, error) {
	query := `
		SELECT id, org_id, user_id, user_email, user_name, role, invitation_status, invited_by, joined_at, created_at, updated_at
		FROM OrganizationMembers
		WHERE org_id = @p1
		ORDER BY created_at ASC;
	`
	rows, err := r.db.Query(query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed listing organization members: %w", err)
	}
	defer rows.Close()

	var members []*models.OrgMember
	for rows.Next() {
		var m models.OrgMember
		var roleStr, statusStr string
		var joinedAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.UserEmail, &m.UserName, &roleStr, &statusStr, &m.InvitedBy, &joinedAt, &m.CreatedAt, &m.UpdatedAt); err == nil {
			m.Role = models.OrgRole(roleStr)
			m.InvitationStatus = models.InvitationStatus(statusStr)
			if joinedAt.Valid {
				m.JoinedAt = joinedAt.Time
			}
			members = append(members, &m)
		}
	}
	return members, nil
}

func (r *MSSQLOrgRepository) UpdateMemberRole(orgID, memberID string, newRole models.OrgRole) (*models.OrgMember, error) {
	var m models.OrgMember
	var roleStr, statusStr string
	querySelect := `
		SELECT id, org_id, user_id, user_email, user_name, role, invitation_status, invited_by, created_at, updated_at
		FROM OrganizationMembers WHERE org_id = @p1 AND id = @p2;
	`
	err := r.db.QueryRow(querySelect, orgID, memberID).Scan(
		&m.ID, &m.OrgID, &m.UserID, &m.UserEmail, &m.UserName, &roleStr, &statusStr, &m.InvitedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, ErrOrgMemberNotFound
	}

	if roleStr == string(models.OrgRoleOwner) {
		return nil, ErrCannotModifyOrgOwner
	}

	now := time.Now()
	_, err = r.db.Exec("UPDATE OrganizationMembers SET role = @p1, updated_at = @p2 WHERE id = @p3", string(newRole), now, memberID)
	if err != nil {
		return nil, fmt.Errorf("failed updating organization member role: %w", err)
	}

	m.Role = newRole
	m.UpdatedAt = now
	return &m, nil
}

func (r *MSSQLOrgRepository) RemoveMember(orgID, memberID string) error {
	var roleStr string
	err := r.db.QueryRow("SELECT role FROM OrganizationMembers WHERE org_id = @p1 AND id = @p2", orgID, memberID).Scan(&roleStr)
	if err != nil {
		return ErrOrgMemberNotFound
	}
	if roleStr == string(models.OrgRoleOwner) {
		return ErrCannotRemoveOrgOwner
	}

	_, err = r.db.Exec("DELETE FROM OrganizationMembers WHERE org_id = @p1 AND id = @p2", orgID, memberID)
	if err != nil {
		return fmt.Errorf("failed removing organization member: %w", err)
	}
	return nil
}

func (r *MSSQLOrgRepository) ListPendingInvitations(userEmail string) ([]*models.PendingOrgInvitationView, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(userEmail))
	query := `
		SELECT om.id, om.org_id, o.name, om.invited_by, u.name, om.role, om.created_at
		FROM OrganizationMembers om
		INNER JOIN Organizations o ON om.org_id = o.id
		LEFT JOIN Users u ON om.invited_by = u.id
		WHERE om.user_email = @p1 AND om.invitation_status = 'pending'
		ORDER BY om.created_at DESC;
	`
	rows, err := r.db.Query(query, cleanEmail)
	if err != nil {
		return nil, fmt.Errorf("failed querying pending organization invitations: %w", err)
	}
	defer rows.Close()

	var invitations []*models.PendingOrgInvitationView
	for rows.Next() {
		var inv models.PendingOrgInvitationView
		var roleStr string
		var inviterName sql.NullString
		if err := rows.Scan(&inv.InvitationID, &inv.OrgID, &inv.OrgName, &inv.InvitedBy, &inviterName, &roleStr, &inv.CreatedAt); err == nil {
			inv.Role = models.OrgRole(roleStr)
			if inviterName.Valid {
				inv.InviterName = inviterName.String
			} else {
				inv.InviterName = "Organization Owner"
			}
			invitations = append(invitations, &inv)
		}
	}
	return invitations, nil
}

func (r *MSSQLOrgRepository) RespondInvitation(invitationID, userID, userEmail string, accept bool) error {
	cleanEmail := strings.ToLower(strings.TrimSpace(userEmail))
	var orgID, currentStatus string
	query := `SELECT org_id, invitation_status FROM OrganizationMembers WHERE id = @p1 AND user_email = @p2;`
	err := r.db.QueryRow(query, invitationID, cleanEmail).Scan(&orgID, &currentStatus)
	if err != nil {
		return ErrOrgInvitationNotFound
	}

	if currentStatus != string(models.StatusPending) {
		return ErrOrgInvitationNotFound
	}

	now := time.Now()
	newStatus := models.StatusRejected
	if accept {
		newStatus = models.StatusAccepted
	}

	_, err = r.db.Exec("UPDATE OrganizationMembers SET invitation_status = @p1, user_id = @p2, joined_at = @p3, updated_at = @p4 WHERE id = @p5", string(newStatus), userID, now, now, invitationID)
	if err != nil {
		return fmt.Errorf("failed updating organization invitation: %w", err)
	}

	action := "invitation_rejected"
	details := fmt.Sprintf("User %s rejected invitation", userEmail)
	if accept {
		action = "invitation_accepted"
		details = fmt.Sprintf("User %s accepted invitation and joined organization", userEmail)
	}
	r.AddActivity(orgID, userID, userEmail, action, details)

	return nil
}

func (r *MSSQLOrgRepository) AddActivity(orgID, userID, userName, action, details string) {
	actID, _ := models.GenerateRandomHex(16)
	query := `
		INSERT INTO OrganizationActivities (id, org_id, user_id, user_name, action, details, timestamp)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7);
	`
	_, _ = r.db.Exec(query, "oact_"+actID, orgID, userID, userName, action, details, time.Now())
}

func (r *MSSQLOrgRepository) GetActivities(orgID string) ([]*models.OrgActivity, error) {
	query := `
		SELECT id, org_id, user_id, user_name, action, details, timestamp
		FROM OrganizationActivities
		WHERE org_id = @p1
		ORDER BY timestamp DESC;
	`
	rows, err := r.db.Query(query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed fetching organization activities: %w", err)
	}
	defer rows.Close()

	var list []*models.OrgActivity
	for rows.Next() {
		var a models.OrgActivity
		var details sql.NullString
		if err := rows.Scan(&a.ID, &a.OrgID, &a.UserID, &a.UserName, &a.Action, &details, &a.Timestamp); err == nil {
			if details.Valid {
				a.Details = details.String
			}
			list = append(list, &a)
		}
	}
	return list, nil
}

func (r *MSSQLOrgRepository) TransferProject(orgID, projectID string) error {
	now := time.Now()
	query := `
		UPDATE Projects
		SET org_id = @p1, owner_type = 'organization', updated_at = @p2
		WHERE id = @p3;
	`
	res, err := r.db.Exec(query, orgID, now, projectID)
	if err != nil {
		return fmt.Errorf("failed transferring project to organization: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrProjectNotFound
	}
	return nil
}
