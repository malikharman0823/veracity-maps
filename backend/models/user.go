package models

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// User represents a registered system user.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"password_hash,omitempty"`
	Salt         string    `json:"salt,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserProject represents a project owned either personally or by an organization.
type UserProject struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	OrgID            string    `json:"org_id,omitempty"`
	OwnerType        string    `json:"owner_type,omitempty"` // "personal" | "organization"
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	PrimaryDatasetID string    `json:"primary_dataset_id,omitempty"`
	DatasetIDs       []string  `json:"dataset_ids,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ProjectRole defines the permission level of a user within a project.
type ProjectRole string

const (
	RoleOwner  ProjectRole = "owner"
	RoleEditor ProjectRole = "editor"
	RoleViewer ProjectRole = "viewer"
)

// OrgRole defines the permission level of a user within an organization.
type OrgRole string

const (
	OrgRoleOwner  OrgRole = "owner"
	OrgRoleAdmin  OrgRole = "admin"
	OrgRoleEditor OrgRole = "editor"
	OrgRoleViewer OrgRole = "viewer"
)

// InvitationStatus defines the state of a collaboration invitation.
type InvitationStatus string

const (
	StatusPending  InvitationStatus = "pending"
	StatusAccepted InvitationStatus = "accepted"
	StatusRejected InvitationStatus = "rejected"
	StatusRevoked  InvitationStatus = "revoked"
)

// Organization represents an organizational workspace.
type Organization struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description,omitempty"`
	CreatedBy         string    `json:"created_by"`
	StorageUsedBytes  int64     `json:"storage_used_bytes"`
	StorageQuotaBytes int64     `json:"storage_quota_bytes"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// OrgMember represents a user's membership and permission within an organization.
type OrgMember struct {
	ID               string           `json:"id"`
	OrgID            string           `json:"org_id"`
	UserID           string           `json:"user_id"`
	UserEmail        string           `json:"user_email"`
	UserName         string           `json:"user_name"`
	Role             OrgRole          `json:"role"`
	InvitationStatus InvitationStatus `json:"invitation_status"`
	InvitedBy        string           `json:"invited_by"`
	JoinedAt         time.Time        `json:"joined_at,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// OrgActivity logs organizational audit events.
type OrgActivity struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Action    string    `json:"action"` // org_created, member_invited, member_joined, role_changed, etc.
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

// PendingOrgInvitationView represents an invitation awaiting user response.
type PendingOrgInvitationView struct {
	InvitationID string    `json:"invitation_id"`
	OrgID        string    `json:"org_id"`
	OrgName      string    `json:"org_name"`
	InvitedBy    string    `json:"invited_by"`
	InviterName  string    `json:"inviter_name"`
	Role         OrgRole   `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// OrgOverviewData summarizes statistics for the organization dashboard.
type OrgOverviewData struct {
	Organization      *Organization `json:"organization"`
	UserRole          OrgRole       `json:"user_role"`
	TotalMembers      int           `json:"total_members"`
	TotalProjects     int           `json:"total_projects"`
	TotalDatasets     int           `json:"total_datasets"`
	StorageUsedBytes  int64         `json:"storage_used_bytes"`
	StorageQuotaBytes int64         `json:"storage_quota_bytes"`
}

// CreateOrgRequest payload.
type CreateOrgRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateOrgRequest payload.
type UpdateOrgRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// InviteOrgMemberRequest payload.
type InviteOrgMemberRequest struct {
	Email string  `json:"email"`
	Role  OrgRole `json:"role"`
}

// UpdateOrgMemberRoleRequest payload.
type UpdateOrgMemberRoleRequest struct {
	Role OrgRole `json:"role"`
}

// TransferProjectRequest payload.
type TransferProjectRequest struct {
	ProjectID string `json:"project_id"`
}

// ProjectMember represents a user's membership and permission within a project.
type ProjectMember struct {
	ID               string           `json:"id"`
	ProjectID        string           `json:"project_id"`
	UserID           string           `json:"user_id"`
	UserEmail        string           `json:"user_email"`
	UserName         string           `json:"user_name"`
	Role             ProjectRole      `json:"role"`
	InvitationStatus InvitationStatus `json:"invitation_status"`
	InvitedBy        string           `json:"invited_by"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// ProjectActivity logs events that occur on a project.
type ProjectActivity struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Action    string    `json:"action"` // e.g., project_created, invitation_sent, invitation_accepted, etc.
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

// SharedProjectView represents a project shared with a user along with membership metadata.
type SharedProjectView struct {
	Project    *UserProject `json:"project"`
	OwnerName  string       `json:"owner_name"`
	OwnerEmail string       `json:"owner_email"`
	Role       ProjectRole  `json:"role"`
	SharedDate time.Time    `json:"shared_date"`
}

// PendingInvitationView represents an invitation awaiting user response.
type PendingInvitationView struct {
	InvitationID string      `json:"invitation_id"`
	ProjectID    string      `json:"project_id"`
	ProjectName  string      `json:"project_name"`
	InvitedBy    string      `json:"invited_by"`
	InviterName  string      `json:"inviter_name"`
	Role         ProjectRole `json:"role"`
	CreatedAt    time.Time   `json:"created_at"`
}

// ShareProjectRequest payload to invite a collaborator.
type ShareProjectRequest struct {
	Email string      `json:"email"`
	Role  ProjectRole `json:"role"`
}

// UpdateMemberRoleRequest payload to change a collaborator's role.
type UpdateMemberRoleRequest struct {
	Role ProjectRole `json:"role"`
}

// RespondInvitationRequest payload to accept or reject an invitation.
type RespondInvitationRequest struct {
	Accept bool `json:"accept"`
}

// AuthToken represents a persistent or session-based bearer authentication token.
type AuthToken struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// UserPublicProfile is safe to serialize in public responses (without hashes or salts).
type UserPublicProfile struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u *User) Public() UserPublicProfile {
	return UserPublicProfile{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// RegisterRequest payload.
type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest payload.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse payload returned on successful register or login.
type AuthResponse struct {
	Token string            `json:"token"`
	User  UserPublicProfile `json:"user"`
}

// CreateProjectRequest payload.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	OrgID       string `json:"org_id,omitempty"`
}

// UpdateProjectRequest payload.
type UpdateProjectRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// Helper to hash password using SHA-256 with cryptographic salt and multiple stretching iterations.
func HashPassword(password, salt string) string {
	combined := []byte(salt + password)
	h := sha256.Sum256(combined)
	for i := 0; i < 100000; i++ {
		h = sha256.Sum256(h[:])
	}
	return hex.EncodeToString(h[:])
}

// VerifyPassword performs constant-time comparison of hashed password.
func VerifyPassword(password, salt, expectedHash string) bool {
	computed := HashPassword(password, salt)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(expectedHash)) == 1
}

// GenerateRandomHex creates a cryptographically secure random hex string.
func GenerateRandomHex(nBytes int) (string, error) {
	bytes := make([]byte, nBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// ValidateRegisterRequest checks sanity of registration payload.
func ValidateRegisterRequest(req *RegisterRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Name == "" {
		return errors.New("name cannot be empty")
	}
	if req.Email == "" || !strings.Contains(req.Email, "@") || !strings.Contains(req.Email, ".") {
		return errors.New("a valid email address is required")
	}
	if len(req.Password) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	return nil
}
