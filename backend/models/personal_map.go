package models

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrMapNotFound             = errors.New("map not found")
	ErrMapAccessDenied         = errors.New("access denied for this map")
	ErrMapLockHeld             = errors.New("map is currently locked for editing by another user")
	ErrMapLockExpiredOrMissing = errors.New("editing lock has expired or is not held by you")
	ErrInvalidSharePermission  = errors.New("invalid sharing permission; must be 'viewer', 'editor', or 'download'")
	ErrShareLinkExpired        = errors.New("this share link has expired")
	ErrShareLinkNotFound       = errors.New("share link not found or has been revoked")
	ErrInvitationNotFound      = errors.New("invitation not found or already consumed")
	ErrInvitationEmailMismatch = errors.New("invitation email does not match authenticated user")
	ErrOutdatedRevision        = errors.New("configuration revision is outdated; please reload the latest revision")
	ErrInvalidMapConfiguration = errors.New("invalid map configuration")
	ErrMapAssetNotFound        = errors.New("map asset not found")
	ErrPersistenceUnavailable  = errors.New("durable map persistence is unavailable")
)

type SharePermission string

const (
	PermissionViewer   SharePermission = "viewer"
	PermissionEditor   SharePermission = "editor"
	PermissionDownload SharePermission = "download"
)

func CleanSharePermission(perm string) SharePermission {
	p := strings.ToLower(strings.TrimSpace(perm))
	switch p {
	case string(PermissionEditor):
		return PermissionEditor
	case string(PermissionDownload):
		return PermissionDownload
	default:
		return PermissionViewer
	}
}

// Map represents an individual map owned by a specific user.
type Map struct {
	ID                 string          `json:"id"`
	OwnerUserID        string          `json:"owner_user_id"`
	OwnerUserName      string          `json:"owner_user_name,omitempty"`
	OwnerUserEmail     string          `json:"owner_user_email,omitempty"`
	SharedBy           string          `json:"shared_by,omitempty"`
	SharedByName       string          `json:"shared_by_name,omitempty"`
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	CurrentRevisionID  string          `json:"current_revision_id,omitempty"`
	RevisionNumber     int             `json:"revision_number"`
	CreatedBy          string          `json:"created_by"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	LastModifiedUser   string          `json:"last_modified_user,omitempty"`
	SavedByCurrentUser bool            `json:"saved_by_current_user,omitempty"`
	AssetCount         int             `json:"asset_count,omitempty"`
	ThumbnailURL       string          `json:"thumbnail_url,omitempty"`
	Permission         SharePermission `json:"permission,omitempty"`     // For recipient: owner, editor, viewer, download
	SharingStatus      string          `json:"sharing_status,omitempty"` // "private", "shared", "shared_with_me"
	Lock               *MapEditingLock `json:"lock,omitempty"`
	Configuration      string          `json:"configuration,omitempty"`
	Assets             []*MapAsset     `json:"assets,omitempty"`
}

// MapAsset represents an uploaded file associated with a map or the user's personal storage.
type MapAsset struct {
	ID               string    `json:"id"`
	MapID            string    `json:"map_id,omitempty"`
	MapName          string    `json:"map_name,omitempty"`
	OwnerUserID      string    `json:"owner_user_id"`
	OwnerUserName    string    `json:"owner_user_name,omitempty"`
	OriginalFilename string    `json:"original_filename"`
	StoragePath      string    `json:"storage_path,omitempty"`
	FileType         string    `json:"file_type"`
	FileSize         int64     `json:"file_size"`
	MimeType         string    `json:"mime_type,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// MapShare represents an active sharing grant to a registered user.
type MapShare struct {
	ID           string          `json:"id"`
	MapID        string          `json:"map_id"`
	MapName      string          `json:"map_name,omitempty"`
	UserID       string          `json:"user_id"`
	UserName     string          `json:"user_name,omitempty"`
	UserEmail    string          `json:"user_email,omitempty"`
	Permission   SharePermission `json:"permission"`
	SharedBy     string          `json:"shared_by"`
	SharedByName string          `json:"shared_by_name,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// MapShareInvitation represents a pending email invitation dispatched via Gmail SMTP.
type MapShareInvitation struct {
	ID          string          `json:"id"`
	MapID       string          `json:"map_id"`
	MapName     string          `json:"map_name,omitempty"`
	Email       string          `json:"email"`
	Permission  SharePermission `json:"permission"`
	TokenHash   string          `json:"-"`
	Token       string          `json:"token,omitempty"`
	Status      string          `json:"status"` // 'pending', 'accepted', 'declined', 'revoked'
	InvitedBy   string          `json:"invited_by"`
	InviterName string          `json:"inviter_name,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
}

// ShareLink represents a cryptographically secure shareable link for a map.
type ShareLink struct {
	ID         string          `json:"id"`
	MapID      string          `json:"map_id"`
	TokenHash  string          `json:"-"`
	Token      string          `json:"token,omitempty"`
	Permission SharePermission `json:"permission"`
	IsPublic   bool            `json:"is_public"`
	ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
}

// MapRevision represents an immutable versioned snapshot of a map configuration.
type MapRevision struct {
	ID                  string    `json:"id"`
	MapID               string    `json:"map_id"`
	RevisionNumber      int       `json:"revision_number"`
	ConfigurationJSON   string    `json:"configuration_json"`
	RevisionDescription string    `json:"revision_description,omitempty"`
	CreatedBy           string    `json:"created_by"`
	CreatedByName       string    `json:"created_by_name,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// MapAssetLocation is the verified durable location committed for one map asset.
type MapAssetLocation struct {
	AssetID     string
	OwnerUserID string
	StoragePath string
}

// MapConfigurationCommit contains the caller-independent values for an atomic map revision commit.
type MapConfigurationCommit struct {
	MapID             string
	Name              string
	Description       string
	PrimaryDatasetID  string
	MemberAssetIDs    []string
	ConfigurationJSON string
	ExpectedRevision  int
	AssetLocations    []MapAssetLocation
}

// MapConfigurationResult returns both records changed by a durable configuration save.
type MapConfigurationResult struct {
	Map      *Map         `json:"map"`
	Revision *MapRevision `json:"revision"`
}

// MapEditingLock represents a single-editor lock for collaborative safety.
type MapEditingLock struct {
	MapID     string    `json:"map_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	LockedAt  time.Time `json:"locked_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ActivityLog logs user actions on maps.
type ActivityLog struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	MapID     string    `json:"map_id,omitempty"`
	Action    string    `json:"action"`
	Details   string    `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
