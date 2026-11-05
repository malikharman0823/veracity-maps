package repository

import (
	"time"

	"github.com/malikharman0823/veracity-maps/models"
)

// AuthRepo defines the interface for user authentication and token storage.
type AuthRepo interface {
	Register(name, email, password string) (*models.User, error)
	Authenticate(email, password string) (*models.User, error)
	CreateToken(userID string, duration time.Duration) (*models.AuthToken, error)
	ValidateToken(tokenStr string) (*models.User, error)
	RevokeToken(tokenStr string)
	GetUserByID(userID string) (*models.User, error)
	FindUserByEmail(email string) (*models.User, error)
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
