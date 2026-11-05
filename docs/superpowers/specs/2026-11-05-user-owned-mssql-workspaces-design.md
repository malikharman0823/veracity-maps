# User-Owned MSSQL Persistence and Temporary Workspaces

**Date:** 2026-11-05
**Status:** Approved design
**Repository branch:** `1.3`

## 1. Purpose

Replace Veracity Maps' session-owned persistent data with authenticated,
user-owned data stored in Microsoft SQL Server 2022. Preserve the existing GIS
editor and its ability to hold unsaved work by replacing anonymous application
sessions with authenticated temporary workspaces.

The final ownership hierarchy is:

```text
User
└── Project (owner and authorized members)
    └── Map
        └── Upload metadata → persistent filesystem object
```

The implementation must build on the current branch and its uncommitted
multi-user and organization-removal work. It must finish incomplete features
without reverting unrelated user changes or redesigning unrelated pages.

## 2. Scope and Constraints

### In scope

- A dedicated user profile page and avatar menu.
- MSSQL-backed authentication, profiles, projects, maps, uploads, activities,
  memberships, invitations, and saved map configurations.
- Temporary authenticated workspaces for unsaved GIS changes.
- Invitations by normalized email address or exact unique User ID.
- Resend email delivery for account verification, email changes, and project
  invitations.
- Backend authorization for every protected resource.
- Backup, verified migration, and removal of legacy JSON/session persistence.
- Removal of the End Session feature from the frontend and backend.
- Documentation, environment templates, migrations, and automated tests.

### Out of scope

- Reintroducing organizations or organization-owned projects.
- Storing large raster, PDF, CAD, export, or archive files in MSSQL.
- Creating separate tables or schemas per user.
- Replacing React, Vite, Tailwind CSS, MapLibre GL JS, Go `net/http`, or the
  existing GIS engine.
- Unrelated UI redesign or a new global state-management dependency.

## 3. Architecture

Authentication is the only identity boundary. Every protected request resolves
a validated bearer token to a `UserID`. A user ID supplied in a request body,
query string, path, or header is never accepted as proof of ownership.

The Go backend is divided into five focused boundaries:

1. **Auth/profile repository:** users, password hashes, token hashes, email
   verification, account status, and profile fields.
2. **Project repository:** ownership, memberships, invitations, roles,
   permission overrides, and activity records.
3. **Map repository:** project-owned maps and explicitly saved map
   configurations.
4. **Upload repository:** file metadata and ownership relationships. File bytes
   remain on disk.
5. **Temporary workspace service:** unsaved GIS state keyed by authenticated
   `UserID + WorkspaceID`.

MSSQL is mandatory outside isolated tests. Startup verifies the database
connection and schema version before serving HTTP traffic. The JSON repositories
may be read by migration tooling but are never runtime fallbacks.

## 4. Data Model

Numbered, idempotent migrations are recorded in a `SchemaMigrations` table.
Shared relational tables use foreign keys and ownership columns to isolate data.

### Users

- `UserID`
- `FullName`
- normalized `Email`
- `PasswordHash`
- `ProfilePicturePath`
- `EmailVerified`
- `PendingEmail`
- `AccountStatus`
- `CreatedAt`
- `UpdatedAt`

The current password format is replaced with bcrypt from
`golang.org/x/crypto/bcrypt` at cost 12. Registration and password-change
validation enforce bcrypt's input limit. Existing valid users are transparently
upgraded after a successful login. Password hashes are never serialized by
public API models.

### AuthTokens

- `TokenID`
- `UserID`
- `TokenHash`
- `ExpiresAt`
- `RevokedAt`
- `CreatedAt`

Only the raw bearer token returned to the client is usable. MSSQL stores a
SHA-256 lookup hash of the high-entropy random token.

### Projects

- `ProjectID`
- `OwnerUserID`
- `ProjectName`
- `Description`
- `CreatedAt`
- `UpdatedAt`

### ProjectMembers

- `MembershipID`
- `ProjectID`
- `UserID`
- `Role`
- `Permissions`
- `JoinedAt`
- `CreatedAt`
- `UpdatedAt`

Only accepted memberships are stored here. `Permissions` is a typed backend
structure serialized to `NVARCHAR(MAX)` with an `ISJSON` constraint. It contains
explicit overrides for map creation/editing/deletion, uploads, file deletion,
invitations, and member management.

### ProjectInvitations

- `InvitationID`
- `ProjectID`
- `InviterUserID`
- normalized `InviteeEmail` or `InviteeUserID`
- normalized `InviteeKey` for duplicate prevention
- `TokenHash`
- `InvitationStatus`
- `Role`
- `Permissions`
- `DeliveryStatus`
- `ExpiresAt`
- `CreatedAt`
- `UpdatedAt`

A filtered unique index prevents multiple pending invitations for the same
project and invitee. Invitations are separate from accepted memberships.

### Maps

- `MapID`
- `ProjectID`
- `MapName`
- `MapConfiguration`
- `CreatedBy`
- `CreatedAt`
- `UpdatedAt`

`MapConfiguration` uses `NVARCHAR(MAX)` with an `ISJSON` constraint. It changes
only after an explicit Save Configuration operation.

### Uploads

- `UploadID`
- `UserID`
- `ProjectID`
- optional `MapID`
- `OriginalFilename`
- `StoragePath`
- `FileType`
- `FileSize`
- `CreatedAt`

MSSQL stores metadata only. Persistent files use stable user/project/map paths.

### Activities

- `ActivityID`
- `UserID`
- `ProjectID`
- `Action`
- safe `Description`
- `CreatedAt`

### WorkspaceDrafts

- `WorkspaceID`
- `UserID`
- optional `ProjectID`
- optional `MapID`
- `DraftConfiguration`
- `StoragePath`
- `CreatedAt`
- `UpdatedAt`
- `ExpiresAt`

This table is a registry for temporary work, not persistent map truth.

### UserVerificationTokens

- `VerificationID`
- `UserID`
- `Purpose`
- optional normalized `RequestedEmail`
- `TokenHash`
- `ExpiresAt`
- `ConsumedAt`
- `CreatedAt`

Purposes include initial account verification and verified email change.

## 5. Temporary Workspace Lifecycle

Application sessions are replaced with authenticated workspaces.

1. Opening or importing into the editor creates a random `WorkspaceID` owned by
   the authenticated user.
2. Draft metadata and configuration are stored in `WorkspaceDrafts`.
3. Large temporary GIS files live under
   `temp/workspaces/<user_id>/<workspace_id>/`.
4. The frontend debounces meaningful changes into the draft. Draft writes never
   update the last saved `Maps.MapConfiguration`.
5. Refreshing or reopening restores the most recent owned draft when available.
6. Multiple workspaces can coexist for a user.
7. Explicit discard removes the draft and files with `storage.SafeRemoveAll`.
8. Logout does not destroy drafts.
9. A configurable cleanup worker removes drafts inactive for 30 days.

Saving a configuration performs the following operation:

1. Validate workspace ownership and project permission.
2. Validate referenced datasets and staged files.
3. Begin an MSSQL transaction.
4. Promote staged files into persistent project/map storage.
5. Insert or update the map, upload metadata, and activity record.
6. Commit the transaction.
7. Remove the temporary workspace only after commit.

If validation, file promotion, or the database transaction fails, compensating
cleanup restores or preserves the draft. A failed save never overwrites the last
saved configuration.

The final application contains no End Session UI, `endSession()` call,
`X-Session-ID`, `/sessions` route, session ownership check, or session-expiry
worker. Windows-safe directory cleanup remains required for workspace cleanup.

## 6. API and Authorization

All protected routes require `Authorization: Bearer <token>`. The frontend API
client attaches the token and, for draft GIS operations, `X-Workspace-ID`.

### Profile and authentication

- `GET /api/profile`
- `PATCH /api/profile`
- `POST /api/profile/avatar`
- `POST /api/profile/password`
- `POST /api/profile/email-change`
- `POST /api/auth/verify-email`
- `POST /api/auth/logout`

Changing a password verifies the current password and revokes all other tokens.
Changing an email creates a pending value and activates it only after the Resend
verification token is consumed.

### Projects, maps, and uploads

- `/api/projects`
- `/api/projects/{projectId}`
- `/api/projects/{projectId}/maps`
- `/api/projects/{projectId}/maps/{mapId}`
- `/api/projects/{projectId}/uploads`

Existing GIS routes remain where practical, but they resolve a validated
workspace rather than an application session.

### Workspaces

- `POST /api/workspaces`
- `GET /api/workspaces`
- `GET /api/workspaces/{workspaceId}`
- `PATCH /api/workspaces/{workspaceId}`
- `DELETE /api/workspaces/{workspaceId}`
- `POST /api/workspaces/{workspaceId}/save`

### Invitations and identity lookup

- `POST /api/users/resolve`
- `GET /api/projects/{projectId}/invitations`
- `POST /api/projects/{projectId}/invitations`
- `DELETE /api/projects/{projectId}/invitations/{invitationId}`
- `POST /api/invitations/{invitationId}/accept`
- `POST /api/invitations/{invitationId}/reject`

Exact identity lookup returns only display name, avatar, User ID, and a masked
email where appropriate. It does not support enumeration or partial search.

### Authorization policy

Every protected project, map, upload, invitation, member, and activity operation
uses one centralized project-access service.

- **Owner:** full control; only the owner can delete the project or manage
  ownership.
- **Editor:** create and edit maps and upload files when the effective
  permissions allow it.
- **Viewer:** read-only access to authorized resources.
- Per-member overrides narrow or extend non-owner capabilities without creating
  additional roles.

Changing an ID in an API request must never grant access. Missing access returns
stable `UNAUTHORIZED` or `FORBIDDEN` responses without revealing private
resource details.

## 7. Profile and Dashboard UX

A protected `/profile` route is available from the account/avatar menu.

The page includes:

- Avatar upload and preview.
- Full name editing.
- Email and verification state.
- User ID display and copy action.
- Account creation date and account status.
- Current/new password form.
- Verified email-change flow.
- Save and logout actions.

Avatar uploads are size and MIME validated. File bytes remain outside MSSQL and
the database stores only the owned path.

The dashboard displays owned projects, shared projects, project maps, uploaded
datasets, pending invitations, members, and recent activity. Draft and saved
states are visibly distinct. When a restorable draft exists, the editor offers
to continue it without silently replacing saved state.

The existing dark/light design system and GIS editor layout remain intact.
Organization-era UI and all End Session entry points are removed.

## 8. Invitations and Resend

The invitation control accepts either a normalized email address or an exact
`usr_...` ID and displays a minimal matched identity before confirmation when a
registered account exists.

Email invitations may target registered users or unregistered addresses. An
unregistered recipient can register with the invited normalized email and then
accept the still-valid invitation.

Invitation rules:

- Validate exact User IDs against `Users`.
- Reject invitations to the project owner.
- Reject existing memberships and duplicate pending invitations.
- Require the inviter's effective permission.
- Use one-time hashed tokens with an expiry.
- Allow owners to cancel pending invitations.
- Transactionally create membership and close the invitation on acceptance.
- Immediately revoke access when a member is removed.

Resend configuration uses:

- `RESEND_API_KEY`
- `RESEND_FROM_EMAIL`
- `APP_PUBLIC_URL`

The backend uses the official Resend Go SDK behind a narrow mailer interface.
Email delivery occurs after the invitation or verification record commits.
Delivery failures update a safe status and may be retried. API keys, raw tokens,
and verification URLs are never written to logs.

## 9. Error Handling and Security

- Use `database/sql`, `github.com/microsoft/go-mssqldb`, contexts, named
  parameters, and transactions for multi-record mutations.
- Database credentials and Resend credentials come only from environment
  variables.
- Production SQL connections require encryption unless an explicitly documented
  local-development setting disables it.
- Password, auth, invitation, and verification secrets never appear in frontend
  models, logs, activities, or error details.
- Stable API error codes include `UNAUTHORIZED`, `FORBIDDEN`,
  `WORKSPACE_NOT_FOUND`, `INVITATION_EXPIRED`, `DUPLICATE_INVITATION`,
  `EMAIL_DELIVERY_FAILED`, and `DATABASE_UNAVAILABLE`.
- File paths are generated server-side and constrained to canonical owned roots.
- Existing ZIP-slip checks, coordinate invariants, upload limits, and
  `storage.SafeRemoveAll` behavior remain mandatory.

## 10. Migration and Legacy Removal

Migration is additive-first and destructive only after verification.

1. Back up the existing MSSQL database and legacy storage directories.
2. Apply schema migrations.
3. Preserve existing IDs for valid users, projects, maps, and uploads.
4. Migrate only records with reliable ownership relationships.
5. Verify row counts, foreign keys, storage paths, representative project
   reopening, and saved GIS configuration restoration.
6. Record a migration report containing counts and failures but no secrets.
7. Remove organization-era columns/tables and legacy JSON/session persistence.
8. Keep the backup for manual recovery; the application never reads it at
   runtime.

Legacy data that cannot be attributed reliably is excluded from live MSSQL data
and removed from runtime storage after the verified backup is complete.

## 11. Delivery Phases and Gates

Each phase must pass its focused tests before the next phase begins.

1. **Foundation:** stabilize the current multi-user changes, add schema version
   tracking, require MSSQL, hash tokens, and remove runtime JSON fallback.
2. **Profile:** profile APIs/UI, avatar storage, password changes, Resend email
   verification, and account status.
3. **Persistent ownership:** projects, maps, upload metadata, files, activities,
   and centralized authorization.
4. **Workspaces:** authenticated drafts, restoration, save promotion, discard,
   and cleanup.
5. **Invitations:** email/User-ID resolution, Resend delivery, acceptance,
   cancellation, roles, and permission overrides.
6. **Legacy removal:** verified backup and migration, then organization/session
   code and data removal.
7. **Regression:** full automated suite and browser acceptance across profile,
   collaboration, persistence, and GIS features.

## 12. Testing and Completion Criteria

Backend coverage must include registration/login, profile changes, token hashing
and revocation, project ownership, map persistence, upload ownership, workspace
restoration, save failure recovery, invitation expiry/cancellation, permission
overrides, and ID-tampering attempts.

MSSQL integration tests use a dedicated test database. Resend behavior is tested
through an injected fake mailer so automated tests do not send real email.

Frontend coverage must include profile rendering and editing, User ID copying,
dashboard ownership sections, member management, invitation input, draft/saved
indicators, and removal of End Session controls.

Required validation:

```powershell
Set-Location backend
gofmt -l .
go vet ./...
go test -v -count=1 ./tests/...

Set-Location ..\frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"

Set-Location ..
powershell -ExecutionPolicy Bypass -File test-all.ps1
```

Browser acceptance must verify the complete User A/User B/User C collaboration
workflow, temporary draft restoration, explicit configuration saving, access
revocation, protected-resource ID tampering, and continued GIS operation. Build
and API checks alone do not count as browser acceptance.

Completion also requires updated root, backend, frontend, API, repository,
models, storage, and page documentation; numbered SQL migrations; a safe
`.env.example`; a modified-file list; and reported test results.
