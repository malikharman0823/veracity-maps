package models

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type UserContextKeyType string

const UserContextKey UserContextKeyType = "user"

// GetUserFromContext retrieves the authenticated user from the context if present.
func GetUserFromContext(ctx context.Context) *User {
	if ctx == nil {
		return nil
	}
	if val, ok := ctx.Value(UserContextKey).(*User); ok {
		return val
	}
	return nil
}

// User represents a registered system user.
type User struct {
	ID                 string    `json:"id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	PasswordHash       string    `json:"password_hash,omitempty"`
	Salt               string    `json:"salt,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	ProfilePicturePath string    `json:"-"`
	EmailVerified      bool      `json:"-"`
	AccountStatus      string    `json:"-"`
}

// UserProject represents a personal project owned by a user.
type UserProject struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
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

// InvitationStatus defines the state of a collaboration invitation.
type InvitationStatus string

const (
	StatusPending  InvitationStatus = "pending"
	StatusAccepted InvitationStatus = "accepted"
	StatusRejected InvitationStatus = "rejected"
	StatusRevoked  InvitationStatus = "revoked"
)

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
	CreatedAt time.Time `json:"created_at"`
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
	ID        string     `json:"id"`
	Token     string     `json:"token,omitempty"`
	TokenHash string     `json:"token_hash,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	UserID    string     `json:"user_id"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (u *User) Public() UserPublicProfile {
	status := u.AccountStatus
	if status == "" {
		status = "active"
	}
	avatar := ""
	if u.ProfilePicturePath != "" {
		avatar = "/api/profile/avatar"
	}
	return UserPublicProfile{
		ID:             u.ID,
		Email:          u.Email,
		FullName:       u.Name,
		ProfilePicture: avatar,
		EmailVerified:  u.EmailVerified,
		AccountStatus:  status,
		CreatedAt:      u.CreatedAt,
		UpdatedAt:      u.UpdatedAt,
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
	Token                string            `json:"token"`
	User                 UserPublicProfile `json:"user"`
	VerificationDelivery string            `json:"verification_delivery,omitempty"`
	VerificationCode     string            `json:"verification_code,omitempty"`
}

// CreateProjectRequest payload.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateProjectRequest payload.
type UpdateProjectRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

const BcryptCost = 12

func HashPassword(password string) (string, error) {
	if len([]byte(password)) > 72 {
		return "", errors.New("password must be at most 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	return string(hash), err
}

func VerifyPassword(password, encoded string) (bool, error) {
	if len([]byte(password)) > 72 {
		return false, errors.New("password must be at most 72 bytes")
	}
	err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	return err == nil, err
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// VerifyLegacyPassword is only for upgrading an existing salted hash on login.
func VerifyLegacyPassword(password, salt, expectedHash string) bool {
	combined := []byte(salt + password)
	h := sha256.Sum256(combined)
	for i := 0; i < 100000; i++ {
		h = sha256.Sum256(h[:])
	}
	computed := hex.EncodeToString(h[:])
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
	if len([]byte(req.Password)) > 72 {
		return errors.New("password must be at most 72 bytes")
	}
	return nil
}

// SendOTPRequest payload to request an OTP code.
type SendOTPRequest struct {
	Email   string `json:"email"`
	Name    string `json:"name,omitempty"`
	Purpose string `json:"purpose"` // "login" | "register"
}

// VerifyOTPRequest payload to verify an OTP and authenticate.
type VerifyOTPRequest struct {
	Email   string `json:"email"`
	Code    string `json:"code"`
	Name    string `json:"name,omitempty"`
	Purpose string `json:"purpose"` // "login" | "register"
}

// SendOTPResponse payload returned after OTP is dispatched.
type SendOTPResponse struct {
	Success            bool   `json:"success"`
	Email              string `json:"email"`
	Purpose            string `json:"purpose"`
	ResendAfterSeconds int    `json:"resend_after_seconds"`
	Message            string `json:"message"`
}

// GenerateNumericOTP creates a cryptographically secure 6-digit numeric OTP code.
func GenerateNumericOTP() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	val := binary.BigEndian.Uint32(b) % 1000000
	return fmt.Sprintf("%06d", val), nil
}

// ValidateEmail checks basic email sanity.
func ValidateEmail(email string) bool {
	clean := strings.ToLower(strings.TrimSpace(email))
	return clean != "" && strings.Contains(clean, "@") && strings.Contains(clean, ".")
}
