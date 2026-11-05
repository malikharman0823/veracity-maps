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
	CreateProject(userID, orgID, ownerType, name, description string) (*models.UserProject, error)
	GetProject(projectID string) (*models.UserProject, error)
	UpdateProject(projectID, name, description string) (*models.UserProject, error)
	DeleteProject(projectID string) error
	ListUserProjects(userID string) ([]*models.UserProject, error)
	ListSharedProjects(userID string) ([]*models.SharedProjectView, error)
	ListOrgProjects(orgID string) ([]*models.UserProject, error)
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

// OrgRepo defines the interface for organization workspaces and team management.
type OrgRepo interface {
	CreateOrg(name, description, createdBy string) (*models.Organization, error)
	GetOrg(orgID string) (*models.Organization, error)
	UpdateOrg(orgID, name, description string) (*models.Organization, error)
	DeleteOrg(orgID string) error
	ListUserOrgs(userID string) ([]*models.Organization, error)
	CheckOrgPermission(orgID, userID string, requiredRole models.OrgRole) (*models.OrgMember, error)
	InviteMember(orgID, email, invitedBy string, role models.OrgRole) (*models.OrgMember, error)
	ListMembers(orgID string) ([]*models.OrgMember, error)
	UpdateMemberRole(orgID, memberID string, newRole models.OrgRole) (*models.OrgMember, error)
	RemoveMember(orgID, memberID string) error
	ListPendingInvitations(userEmail string) ([]*models.PendingOrgInvitationView, error)
	RespondInvitation(invitationID, userID, userEmail string, accept bool) error
	AddActivity(orgID, userID, userName, action, details string)
	GetActivities(orgID string) ([]*models.OrgActivity, error)
	TransferProject(orgID, projectID string) error
}
