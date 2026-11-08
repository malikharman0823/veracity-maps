package repository

import (
	"context"
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// AuthRepo defines the interface for user authentication and token storage.
type AuthRepo interface {
	Register(ctx context.Context, name, email, password string) (*models.User, error)
	RegisterWithoutPassword(ctx context.Context, name, email string) (*models.User, error)
	Authenticate(ctx context.Context, email, password string) (*models.User, error)
	AuthenticateAndCreateToken(ctx context.Context, email, password string, duration time.Duration) (*models.User, *models.AuthToken, error)
	CreateToken(ctx context.Context, userID string, duration time.Duration) (*models.AuthToken, error)
	ValidateToken(ctx context.Context, tokenStr string) (*models.User, error)
	RevokeToken(ctx context.Context, tokenStr string) error
	GetUserByID(ctx context.Context, userID string) (*models.User, error)
	FindUserByEmail(ctx context.Context, email string) (*models.User, error)
}

// OTPRecord represents an active or consumed one-time login or registration token.
type OTPRecord struct {
	ID                string
	Email             string
	Name              string
	OTPHash           string
	Purpose           string
	Attempts          int
	MaxAttempts       int
	ExpiresAt         time.Time
	ResendAvailableAt time.Time
	ConsumedAt        *time.Time
	CreatedAt         time.Time
}

// OTPRepo handles generation, storage, and one-time verification of email OTPs.
type OTPRepo interface {
	CreateOTP(ctx context.Context, email, name, purpose, code string, expiresDuration, resendCooldown time.Duration) (*OTPRecord, error)
	VerifyAndConsumeOTP(ctx context.Context, email, code, purpose string) (*OTPRecord, error)
}

// ProfileRepo handles account mutations independently of authentication.
type ProfileRepo interface {
	Get(context.Context, string) (*models.UserPublicProfile, error)
	UpdateName(context.Context, string, string) (*models.UserPublicProfile, error)
	SetAvatar(context.Context, string, string) (*models.UserPublicProfile, error)
	AvatarPath(context.Context, string) (string, error)
	ChangePassword(context.Context, string, string, string, string) error
	CreateEmailVerification(context.Context, string, string, string) (string, error)
	ConsumeEmailVerification(context.Context, string) error
}

// ProjectRepo defines the interface for project management and sharing.
type ProjectRepo interface {
	CreateProject(userID, name, description string) (*models.UserProject, error)
	GetProject(projectID string) (*models.UserProject, error)
	UpdateProject(projectID, name, description string) (*models.UserProject, error)
	DeleteProject(projectID string) error
	ListUserProjects(userID string) ([]*models.UserProject, error)
	ListSharedProjects(userID string) ([]*models.SharedProjectView, error)
	CheckPermission(projectID, userID string, requiredRole models.ProjectRole) (*models.ProjectMember, error)
	ShareProject(projectID, targetEmail, invitedBy string, role models.ProjectRole) (*models.ProjectMember, error)
	ListMembers(projectID string) ([]*models.ProjectMember, error)
	UpdateMemberRole(projectID, memberID string, newRole models.ProjectRole) (*models.ProjectMember, error)
	RemoveMember(projectID, memberID string) error
	ListPendingInvitations(userEmail string) ([]*models.PendingInvitationView, error)
	RespondInvitation(invitationID, userID, userEmail string, accept bool) error
	AssociateDataset(projectID, datasetID string) error
	GetActivities(projectID string) ([]*models.ProjectActivity, error)
	AddActivity(projectID, userID, userName, action, details string)
}
