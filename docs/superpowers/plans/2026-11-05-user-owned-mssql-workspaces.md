# User-Owned MSSQL Workspaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace session-owned persistent data with authenticated user-owned projects, maps, uploads, invitations, and restorable temporary workspaces backed by Microsoft SQL Server 2022.

**Architecture:** Bearer authentication resolves every protected request to a user. MSSQL is the sole persistent source of truth, while large files remain in owned filesystem paths and unsaved GIS work lives in user/workspace temporary directories. Existing GIS handlers are retained and migrated from session context to authenticated workspace context in stages.

**Tech Stack:** Go 1.27, `net/http`, `database/sql`, `github.com/microsoft/go-mssqldb`, `golang.org/x/crypto/bcrypt`, `github.com/resend/resend-go/v4`, Microsoft SQL Server 2022, React 19, React Router 7, Vite 7, Tailwind CSS 4, MapLibre GL JS 6, Axios.

## Global Constraints

- Preserve and build on the current uncommitted `1.3` changes; never reset, checkout, or overwrite them.
- Keep the architecture `User -> Projects -> Maps -> Uploads`.
- MSSQL is mandatory outside isolated tests; no JSON or in-memory persistent fallback is permitted.
- Large raster, TIFF, PDF, CAD, archive, MBTiles, and export files remain outside MSSQL.
- Use shared relational tables with ownership foreign keys; never create per-user SQL tables.
- Use `database/sql`, named parameters, contexts, and transactions for multi-record mutations.
- Store only hashes of auth, verification, and invitation tokens.
- Use bcrypt cost 12 and validate bcrypt's 72-byte password input limit.
- Use `storage.SafeRemoveAll` for Windows directory cleanup.
- Preserve MapLibre coordinate order `[west, south, east, north]` and all existing GIS behavior.
- Preserve ZIP-slip canonical-prefix checks.
- Do not redesign unrelated pages or introduce a global state manager.
- Invoke native npm commands through `cmd.exe /c` on Windows.
- Stage only the files named by the current task; inspect `git diff --cached` before every commit.
- A build or API test is not browser acceptance; report browser verification separately.

## Planned File Structure

### Backend files to create

- `backend/migrations/002_user_owned_persistence.sql`: additive user/profile/token/project/map/upload/workspace schema and removal of organization-era schema.
- `backend/models/profile.go`: public profile and profile mutation payloads.
- `backend/models/project_access.go`: typed roles, permissions, capabilities, invitation, map, upload, and activity models.
- `backend/models/workspace.go`: authenticated workspace and draft models.
- `backend/repository/mssql_profile_repository.go`: profile, password, email verification, and avatar metadata persistence.
- `backend/repository/mssql_invitation_repository.go`: invitation lifecycle and membership creation.
- `backend/repository/mssql_map_repository.go`: maps, uploads, and explicit configuration saves.
- `backend/repository/mssql_workspace_repository.go`: temporary workspace registry and draft JSON persistence.
- `backend/repository/workspace_repository.go`: in-memory GIS aggregate keyed by authenticated workspace ID, hydrated from owned temporary files.
- `backend/services/access_service.go`: centralized effective-permission checks.
- `backend/services/mailer.go`: mailer interface and safe message inputs.
- `backend/services/resend_mailer.go`: Resend SDK adapter.
- `backend/services/workspace_service.go`: draft creation, save promotion, discard, and cleanup orchestration.
- `backend/functions/profile/function.go`: profile HTTP handlers.
- `backend/functions/workspaces/function.go`: workspace HTTP handlers.
- `backend/functions/maps/function.go`: map/upload persistence HTTP handlers.
- `backend/tests/test_server_test.go`: reusable test server and fake mailer.
- `backend/tests/profile_test.go`: profile and security API coverage.
- `backend/tests/project_access_test.go`: capabilities and IDOR coverage.
- `backend/tests/invitations_user_id_test.go`: email/User-ID invitation coverage.
- `backend/tests/maps_uploads_test.go`: persistent ownership coverage.
- `backend/tests/workspace_lifecycle_test.go`: draft/save/discard/reopen coverage.
- `backend/scripts/backup-legacy.ps1`: bounded backup and manifest generation.
- `backend/scripts/verify-migration.ps1`: database/file relationship verification.

### Frontend files to create

- `frontend/src/pages/ProfilePage.jsx`: profile, security, and account actions.
- `frontend/src/lib/profileForm.js`: pure profile validation and display helpers.
- `frontend/src/lib/profileForm.test.js`: Node tests for profile helpers.
- `frontend/src/lib/workspaceDraft.js`: workspace ID storage and draft-state helpers.
- `frontend/src/lib/workspaceDraft.test.js`: Node tests for workspace helpers.
- `frontend/src/components/AccountMenu.jsx`: avatar menu linking profile/dashboard/logout.
- `frontend/src/components/InvitationInput.jsx`: email/User-ID resolve and confirmation UI.

### Existing files with focused modifications

- Backend: `main.go`, `go.mod`, `go.sum`, `config/config.go`, `db/mssql.go`, `api/middleware.go`, `api/router.go`, auth/project handlers and repositories, storage, GIS services, migration code, tests, READMEs, `.env.example`, and test runners.
- Frontend: `App.jsx`, `api.js`, `AuthContext.jsx`, `Sidebar.jsx`, `DashboardPage.jsx`, `ShareProjectModal.jsx`, workspace pages/components, READMEs, and `index.css` only where new UI tokens are needed.
- Delete after replacement tests pass: JSON auth/project repositories, session repository, `EndSessionDialog.jsx`, session helper/tests, and organization-era migration/runtime files.

---

### Task 1: Reconcile and Commit the Current Multi-User Baseline

**Files:**
- Modify: the currently dirty backend/frontend files reported by `git status`
- Create: `backend/tests/organization_routes_removed_test.go`
- Test: `backend/tests/organization_routes_removed_test.go`

**Interfaces:**
- Consumes: current uncommitted organization-removal and multi-user edits.
- Produces: a compiling, tested baseline with project-only collaboration and no organization route/UI references.

- [ ] **Step 1: Capture the current user-owned diff without modifying it**

Run:

```powershell
git status --short --branch
git diff --stat
git diff -- backend/api/router.go backend/main.go frontend/src/App.jsx frontend/src/api.js
```

Expected: the current organization deletions and project-only edits remain visible; no file is reset.

- [ ] **Step 2: Add a failing route-removal regression test**

Create `backend/tests/organization_routes_removed_test.go`:

```go
package tests

import (
    "net/http"
    "testing"
)

func TestOrganizationRoutesAreRemoved(t *testing.T) {
    server, _, tempDir := createTestServer(t)
    defer server.Close()
    defer cleanupTestDir(t, tempDir)

    response, err := http.Get(server.URL + "/api/organizations")
    if err != nil {
        t.Fatalf("request organization route: %v", err)
    }
    defer response.Body.Close()
    if response.StatusCode != http.StatusNotFound {
        t.Fatalf("expected removed organization route to return 404, got %d", response.StatusCode)
    }
}
```

- [ ] **Step 3: Run focused tests and capture the current failures**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestOrganizationRoutesAreRemoved|TestMultiUserAuthenticationAndIsolation"
```

Expected: the new test or compile step fails if stale organization symbols or inconsistent test helpers remain.

- [ ] **Step 4: Finish the existing organization removal and repair test helpers**

Remove remaining organization imports, constructors, routes, API functions, dashboard tabs, and tests. Add this helper beside `createTestServer` and use it instead of direct `os.RemoveAll`:

```go
func cleanupTestDir(t *testing.T, path string) {
    t.Helper()
    if err := storage.SafeRemoveAll(path); err != nil {
        t.Fatalf("cleanup test directory %s: %v", path, err)
    }
}
```

Do not remove session infrastructure in this task; it remains the GIS compatibility boundary until Task 9.

- [ ] **Step 5: Run the baseline gate**

Run:

```powershell
Set-Location backend
Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go vet ./...
go test -count=1 ./tests/...
Set-Location ..\frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit the reconciled baseline**

Stage only the intended dirty baseline and the new regression test, inspect the staged diff, then commit:

```powershell
git add backend frontend/src frontend/tailwind.config.js
git diff --cached --check
git diff --cached --name-status
git commit -m "refactor: complete project-only collaboration baseline"
```

### Task 2: Make MSSQL Mandatory and Add Versioned Schema Migrations

**Files:**
- Create: `backend/migrations/002_user_owned_persistence.sql`
- Modify: `backend/config/config.go`
- Modify: `backend/db/mssql.go`
- Modify: `backend/main.go`
- Modify: `backend/repository/migration.go`
- Modify: `backend/tests/mssql_multi_user_test.go`
- Modify: `.env.example`

**Interfaces:**
- Produces: `config.Validate() error`, `db.InitDB(ctx context.Context, connStr string) (*sql.DB, error)`, and `db.RunMigrations(ctx context.Context, db *sql.DB, dir string) error`.
- Produces schema tables and columns defined by the approved design.

- [ ] **Step 1: Write failing configuration and migration tests**

Add these cases to `backend/tests/mssql_multi_user_test.go`:

```go
func TestProductionConfigRequiresMSSQL(t *testing.T) {
    cfg := &config.Config{Environment: "production"}
    if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "MSSQL_CONNECTION_STRING") {
        t.Fatalf("expected missing MSSQL connection error, got %v", err)
    }
}

func TestMigrationsAreIdempotent(t *testing.T) {
    database := openTestMSSQL(t)
    ctx := context.Background()
    if err := db.RunMigrations(ctx, database, "../migrations"); err != nil {
        t.Fatalf("first migration run: %v", err)
    }
    if err := db.RunMigrations(ctx, database, "../migrations"); err != nil {
        t.Fatalf("second migration run: %v", err)
    }
}
```

`openTestMSSQL` reads `MSSQL_TEST_CONNECTION_STRING` and calls `t.Skip` when it is absent; the final acceptance gate must provide it.

- [ ] **Step 2: Run the tests to verify failure**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestProductionConfigRequiresMSSQL|TestMigrationsAreIdempotent"
```

Expected: compile failure because `Config.Validate`, contextual DB initialization, and migration tracking do not exist.

- [ ] **Step 3: Add explicit configuration validation**

Extend `Config` with:

```go
Environment     string
MSSQLConnStr   string
ResendAPIKey   string
ResendFrom     string
AppPublicURL   string
WorkspaceTTL   time.Duration
```

Implement:

```go
func (c *Config) Validate() error {
    if strings.TrimSpace(c.MSSQLConnStr) == "" && c.Environment != "test" {
        return errors.New("MSSQL_CONNECTION_STRING is required outside tests")
    }
    if c.WorkspaceTTL <= 0 {
        return errors.New("WORKSPACE_TTL must be positive")
    }
    return nil
}
```

- [ ] **Step 4: Implement tracked migrations and fail-fast startup**

Change database startup to use contexts and `PingContext`. `RunMigrations` must create and query:

```sql
CREATE TABLE SchemaMigrations (
    version NVARCHAR(255) NOT NULL PRIMARY KEY,
    applied_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET()
);
```

For each sorted `.sql` filename not present in `SchemaMigrations`, execute the file in a transaction and insert its filename before commit. In `main.go`, return a fatal startup error when config validation, connection, ping, or migration fails. Remove the `MigrateJSONToMSSQL` runtime call.

- [ ] **Step 5: Add the additive user-owned schema migration**

Create `002_user_owned_persistence.sql` with the approved tables and these security-critical shapes:

```sql
ALTER TABLE Users ADD
    profile_picture_path NVARCHAR(1024) NULL,
    email_verified BIT NOT NULL CONSTRAINT DF_Users_EmailVerified DEFAULT 0,
    pending_email NVARCHAR(255) NULL,
    account_status NVARCHAR(32) NOT NULL CONSTRAINT DF_Users_Status DEFAULT 'active';

CREATE TABLE Maps (
    id NVARCHAR(64) NOT NULL PRIMARY KEY,
    project_id NVARCHAR(64) NOT NULL,
    map_name NVARCHAR(255) NOT NULL,
    map_configuration NVARCHAR(MAX) NOT NULL CONSTRAINT CK_Maps_ConfigJSON CHECK (ISJSON(map_configuration) = 1),
    created_by NVARCHAR(64) NOT NULL,
    created_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    updated_at DATETIMEOFFSET NOT NULL DEFAULT SYSDATETIMEOFFSET(),
    CONSTRAINT FK_Maps_Project FOREIGN KEY (project_id) REFERENCES Projects(id) ON DELETE CASCADE,
    CONSTRAINT FK_Maps_Creator FOREIGN KEY (created_by) REFERENCES Users(id)
);

CREATE INDEX IX_Maps_ProjectID ON Maps(project_id);
```

Also add compatibility columns for hashed `AuthTokens`, and create `ProjectInvitations`, `Uploads`, `Activities`, `WorkspaceDrafts`, and `UserVerificationTokens`; add and backfill `owner_user_id` on projects; and add JSON permissions to members. Do not drop or rename columns/tables still used by current repositories in this task. Organization foreign keys/tables are removed in Task 12 only after their consumers are migrated and verification passes.

Because existing `AuthTokens.token` values are raw secrets, add nullable `token_hash`, `revoked_at`, and token-ID compatibility fields without copying raw values. Task 3 switches the repository, invalidates existing raw-token rows, and makes hashed-token fields authoritative in the same release sequence.

- [ ] **Step 6: Run the database gate**

Run with a dedicated SQL Server database:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestProductionConfigRequiresMSSQL|TestMigrationsAreIdempotent"
go vet ./...
```

Expected: both tests pass and migrations are repeatable.

- [ ] **Step 7: Commit**

```powershell
git add .env.example backend/config backend/db backend/main.go backend/migrations backend/repository/migration.go backend/tests/mssql_multi_user_test.go
git diff --cached --check
git commit -m "feat: require MSSQL and track schema migrations"
```

### Task 3: Harden Passwords and Authentication Tokens

**Files:**
- Create: `backend/migrations/003_hashed_auth_tokens.sql`
- Modify: `backend/models/user.go`
- Modify: `backend/repository/interfaces.go`
- Modify: `backend/repository/mssql_auth_repository.go`
- Modify: `backend/functions/auth/function.go`
- Modify: `backend/api/middleware.go`
- Modify: `backend/main.go`
- Modify: `backend/tests/multi_user_auth_test.go`
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

**Interfaces:**
- Produces: `models.HashPassword(password string) (string, error)`, `models.VerifyPassword(password, encoded string) (bool, error)`, and `models.HashToken(raw string) string`.
- Produces contextual `AuthRepo` methods and raw-token-once behavior.

- [ ] **Step 1: Write failing password and token tests**

Add:

```go
func TestPasswordHashUsesBcryptAndRejectsOversizeInput(t *testing.T) {
    hash, err := models.HashPassword("Correct Horse Battery Staple")
    if err != nil {
        t.Fatalf("hash password: %v", err)
    }
    if !strings.HasPrefix(hash, "$2") {
        t.Fatalf("expected bcrypt hash, got %q", hash)
    }
    if _, err := models.HashPassword(strings.Repeat("x", 73)); err == nil {
        t.Fatal("expected bcrypt input limit error")
    }
}

func TestAuthTokenDatabaseValueIsHashed(t *testing.T) {
    raw := "vmap_example-token"
    if models.HashToken(raw) == raw || len(models.HashToken(raw)) != 64 {
        t.Fatal("token must be stored as a SHA-256 hex lookup hash")
    }
}
```

- [ ] **Step 2: Run the tests to verify failure**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestPasswordHashUsesBcryptAndRejectsOversizeInput|TestAuthTokenDatabaseValueIsHashed"
```

Expected: compile failure due to the old signatures.

- [ ] **Step 3: Implement bcrypt and token hashing**

Use:

```go
const BcryptCost = 12

func HashPassword(password string) (string, error) {
    if len([]byte(password)) > 72 {
        return "", errors.New("password must be at most 72 bytes")
    }
    hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
    return string(hash), err
}

func VerifyPassword(password, encoded string) (bool, error) {
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
```

- [ ] **Step 4: Change auth persistence to contexts and hashes**

Create `003_hashed_auth_tokens.sql` to replace the compatibility `AuthTokens` table with `id` as primary key and required unique `token_hash`, preserving user/expiry/created fields and adding `revoked_at`. Intentionally invalidate existing raw-token rows rather than copying raw secrets. Define `AuthRepo` with `context.Context` as the first argument for every database operation. `CreateToken` inserts only `models.HashToken(raw)`. `ValidateToken` and `RevokeToken` hash the presented raw token before querying. Add `revoked_at IS NULL` to validation and use `sql.Named` parameters.

Keep legacy password verification only long enough to rehash the password to bcrypt after a successful login; never create a new legacy hash.

- [ ] **Step 5: Update handlers, middleware, and fake repositories**

Pass `r.Context()` through auth handlers and middleware. Public responses use `User.Public()` and must not contain `password_hash`, `salt`, `token_hash`, or `revoked_at`. Update production startup to construct `NewMSSQLAuthRepository(sqlDB)` as the `AuthRepo`; the filesystem `AuthRepository` remains available only to isolated tests until Task 12 removes it.

- [ ] **Step 6: Run focused and full auth tests**

Run:

```powershell
Set-Location backend
Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go test -count=1 ./tests/... -run "Password|Token|Authentication|Logout"
go test -count=1 ./tests/...
```

Expected: all commands pass.

- [ ] **Step 7: Commit**

```powershell
git add backend/go.mod backend/go.sum backend/main.go backend/migrations/003_hashed_auth_tokens.sql backend/models/user.go backend/repository/interfaces.go backend/repository/auth_repository.go backend/repository/mssql_auth_repository.go backend/repository/project_repository.go backend/repository/mssql_project_repository.go backend/functions/auth backend/functions/projects/function.go backend/api/middleware.go backend/tests/multi_user_auth_test.go backend/tests/mssql_multi_user_test.go backend/README.md backend/api/README.md backend/models/README.md backend/repository/README.md backend/tests/README.md
git diff --cached --check
git commit -m "feat: hash credentials and bearer tokens securely"
```

### Task 4: Add Profile, Password, Avatar, and Resend Verification APIs

**Files:**
- Create: `backend/models/profile.go`
- Create: `backend/repository/mssql_profile_repository.go`
- Create: `backend/services/mailer.go`
- Create: `backend/services/resend_mailer.go`
- Create: `backend/functions/profile/function.go`
- Create: `backend/tests/profile_test.go`
- Modify: `backend/config/config.go`
- Modify: `backend/repository/interfaces.go`
- Modify: `backend/api/router.go`
- Modify: `backend/main.go`
- Modify: `backend/functions/auth/function.go`
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

**Interfaces:**
- Produces `ProfileRepo` methods `Get`, `UpdateName`, `SetAvatar`, `ChangePassword`, `CreateEmailVerification`, and `ConsumeEmailVerification`.
- Produces `services.Mailer` with verification and invitation methods.
- Produces profile routes from the approved design.

- [ ] **Step 1: Add a fake mailer and failing profile tests**

Define in `backend/tests/test_server_test.go`:

```go
type fakeMailer struct {
    verificationRecipients []string
    invitationRecipients   []string
}

func (m *fakeMailer) SendVerification(_ context.Context, message services.VerificationEmail) error {
    m.verificationRecipients = append(m.verificationRecipients, message.To)
    return nil
}

func (m *fakeMailer) SendInvitation(_ context.Context, message services.InvitationEmail) error {
    m.invitationRecipients = append(m.invitationRecipients, message.To)
    return nil
}
```

Add API tests that assert `GET /api/profile` returns ID, name, email, verification status, creation date, and account status; that PATCH changes the name; that password change rejects a wrong current password and revokes other tokens; and that email change records one fake-mail delivery without exposing the token.

- [ ] **Step 2: Run tests to verify 404/failure**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestProfile|TestPasswordChange|TestEmailChange"
```

Expected: profile routes return 404 or required types are undefined.

- [ ] **Step 3: Define safe profile models**

Use:

```go
type UserPublicProfile struct {
    ID             string    `json:"id"`
    FullName       string    `json:"full_name"`
    Email          string    `json:"email"`
    ProfilePicture string    `json:"profile_picture,omitempty"`
    EmailVerified  bool      `json:"email_verified"`
    AccountStatus  string    `json:"account_status"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}

type ChangePasswordRequest struct {
    CurrentPassword string `json:"current_password"`
    NewPassword     string `json:"new_password"`
}
```

Move the existing `UserPublicProfile` definition from `models/user.go` into `models/profile.go`; do not leave two definitions in package `models`.

- [ ] **Step 4: Implement the mailer boundary and Resend adapter**

Define:

```go
type Mailer interface {
    SendVerification(context.Context, VerificationEmail) error
    SendInvitation(context.Context, InvitationEmail) error
}
```

`NewResendMailer` constructs `resend.NewCustomClient` with an HTTP client timeout, and each method calls `Emails.SendWithContext`. Inputs contain recipient and already-built link; the adapter never logs the link or API key.

- [ ] **Step 5: Implement profile persistence and handlers**

Use named queries and transactions. Avatar uploads accept PNG, JPEG, or WebP up to 5 MiB, write beneath `storage/users/<user_id>/profile/`, then update the database path. Email-change tokens expire after 30 minutes and are consumed once. Password change verifies the current password and revokes all other tokens in the same transaction.

- [ ] **Step 6: Wire routes and startup dependencies**

Add authenticated profile routes, public token-consumption route, and authenticated `POST /api/auth/resend-verification` to `api/router.go`. Registration creates an initial-verification token and attempts Resend delivery without deleting the account on delivery failure; responses expose only safe verification/delivery status. Resend rotates outstanding initial tokens and enforces a deterministic recent-send rate limit. In `main.go`, instantiate the Resend mailer from config. Outside tests, fail startup if any Resend variable is empty because verification is part of the required account flow.

- [ ] **Step 7: Run tests and static analysis**

Run:

```powershell
Set-Location backend
Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go test -count=1 ./tests/... -run "TestProfile|TestPasswordChange|TestEmailChange"
go vet ./...
```

Expected: all commands pass and fake mail is used.

- [ ] **Step 8: Commit**

```powershell
git add backend/go.mod backend/go.sum backend/config backend/models backend/repository backend/services backend/functions/profile backend/api backend/main.go backend/tests
git diff --cached --check
git commit -m "feat: add secure profile and email verification APIs"
```

### Task 5: Build the Profile Page and Account Menu

**Files:**
- Create: `frontend/src/lib/profileForm.js`
- Create: `frontend/src/lib/profileForm.test.js`
- Create: `frontend/src/pages/ProfilePage.jsx`
- Create: `frontend/src/components/AccountMenu.jsx`
- Modify: `frontend/src/api.js`
- Modify: `frontend/src/App.jsx`
- Modify: `frontend/src/context/AuthContext.jsx`
- Modify: `frontend/src/components/Sidebar.jsx`
- Modify: `frontend/src/pages/README.md`

**Interfaces:**
- Consumes profile APIs from Task 4.
- Produces `refreshUser()`, profile API client functions, `/profile`, and reusable account menu.

- [ ] **Step 1: Write failing pure-helper tests**

Create:

```js
import test from "node:test"
import assert from "node:assert/strict"
import { validateProfileName, canSubmitPasswordChange, profileInitials } from "./profileForm.js"

test("profile helpers validate account forms", () => {
  assert.equal(validateProfileName("  Ada Lovelace  "), "Ada Lovelace")
  assert.equal(profileInitials("Ada Lovelace"), "AL")
  assert.equal(canSubmitPasswordChange("old-password", "new-password-123", "new-password-123"), true)
  assert.equal(canSubmitPasswordChange("old-password", "different-one", "mismatch"), false)
})
```

- [ ] **Step 2: Run the test to verify module-not-found failure**

Run:

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
```

Expected: failure because `profileForm.js` does not exist.

- [ ] **Step 3: Implement pure helpers and API functions**

Implement the three helpers without React dependencies. Add API functions `getProfile`, `updateProfile`, `uploadProfileAvatar`, `changePassword`, `requestEmailChange`, and `verifyEmail`. `uploadProfileAvatar` sends `FormData` and does not set a manual multipart boundary.

- [ ] **Step 4: Add `refreshUser` to auth context**

Implement:

```jsx
const refreshUser = async () => {
  const profile = await getCurrentUser()
  setUser(profile)
  return profile
}
```

Expose it from `AuthContext` so successful profile edits update the avatar menu immediately.

- [ ] **Step 5: Build the protected profile route**

`ProfilePage` renders Account Information, Security, and Account Actions sections. It displays creation date, account status, verification status, and User ID. The copy action uses `navigator.clipboard.writeText(user.id)` and provides an accessible success message. Password inputs never populate from API data.

- [ ] **Step 6: Add the account menu and remove duplicate logout controls**

`AccountMenu` shows avatar/initials, name, email, links to `/profile` and `/dashboard`, and logout. Replace direct account blocks in `Sidebar` and dashboard header with this component without changing unrelated navigation.

- [ ] **Step 7: Run frontend tests and build**

Run:

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"
```

Expected: tests and production build pass.

- [ ] **Step 8: Commit**

```powershell
git add frontend/src/api.js frontend/src/App.jsx frontend/src/context frontend/src/lib/profileForm* frontend/src/pages/ProfilePage.jsx frontend/src/pages/README.md frontend/src/components/AccountMenu.jsx frontend/src/components/Sidebar.jsx
git diff --cached --check
git commit -m "feat: add user profile and account menu"
```

### Task 6: Centralize Project Authorization and Permission Overrides

**Files:**
- Create: `backend/models/project_access.go`
- Create: `backend/services/access_service.go`
- Create: `backend/tests/project_access_test.go`
- Modify: `backend/repository/interfaces.go`
- Modify: `backend/repository/mssql_project_repository.go`
- Modify: `backend/functions/projects/function.go`
- Modify: `backend/api/router.go`

**Interfaces:**
- Produces `services.Capability`, `services.ProjectAccess.Authorize(ctx, projectID, userID, capability) error`, and typed `ProjectPermissions`.
- Consumed by all later map, upload, invitation, and workspace handlers.

- [ ] **Step 1: Write failing permission and IDOR tests**

Cover owner, editor, viewer, editor override denial, viewer override grant for a non-destructive capability, removed member denial, and a user changing `projectId` in a map/upload request. Use table cases:

```go
tests := []struct {
    name       string
    role       models.ProjectRole
    permission models.ProjectPermissions
    capability services.Capability
    allowed    bool
}{
    {"owner deletes project", models.RoleOwner, models.ProjectPermissions{}, services.DeleteProject, true},
    {"viewer cannot edit map", models.RoleViewer, models.ProjectPermissions{}, services.EditMap, false},
    {"editor upload denied by override", models.RoleEditor, models.ProjectPermissions{UploadFiles: boolPointer(false)}, services.UploadFile, false},
}
```

- [ ] **Step 2: Run tests to verify missing access service**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestProjectAccess|TestProjectIDTampering"
```

Expected: compile failure for the new models/service.

- [ ] **Step 3: Define roles, permissions, and capabilities**

Use pointer booleans in `ProjectPermissions` so absence means role default:

```go
type ProjectPermissions struct {
    CreateMaps  *bool `json:"create_maps,omitempty"`
    EditMaps    *bool `json:"edit_maps,omitempty"`
    DeleteMaps  *bool `json:"delete_maps,omitempty"`
    UploadFiles *bool `json:"upload_files,omitempty"`
    DeleteFiles *bool `json:"delete_files,omitempty"`
    InviteUsers *bool `json:"invite_users,omitempty"`
    ManageUsers *bool `json:"manage_users,omitempty"`
}
```

Project deletion and owner management are owner-only and cannot be granted by overrides.

- [ ] **Step 4: Implement one authorization path**

`ProjectAccess.Authorize` loads the project and accepted membership, computes role defaults plus overrides, and returns `repository.ErrForbidden` when denied. Replace handler-local role-weight checks with this service.

- [ ] **Step 5: Pass request contexts and use named parameters**

Update project repository methods to accept `context.Context`, use `QueryContext`/`ExecContext` and `sql.Named`, and return no private project details on unauthorized access.

- [ ] **Step 6: Run project and collaboration tests**

Run:

```powershell
Set-Location backend
Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go test -count=1 ./tests/... -run "TestProjectAccess|TestProjectIDTampering|TestProjectSharing"
go test -count=1 ./tests/...
```

Expected: all commands pass.

- [ ] **Step 7: Commit**

```powershell
git add backend/models/project_access.go backend/services/access_service.go backend/repository backend/functions/projects backend/api backend/tests/project_access_test.go
git diff --cached --check
git commit -m "feat: centralize project permission enforcement"
```

### Task 7: Implement Email and User-ID Invitations

**Files:**
- Create: `backend/repository/mssql_invitation_repository.go`
- Create: `backend/tests/invitations_user_id_test.go`
- Modify: `backend/models/project_access.go`
- Modify: `backend/functions/projects/function.go`
- Modify: `backend/api/router.go`
- Modify: `backend/services/mailer.go`

**Interfaces:**
- Produces `InvitationRepo.ResolveIdentity`, `Create`, `List`, `Cancel`, `Accept`, and `Reject`.
- Consumes `ProjectAccess.Authorize` and `Mailer.SendInvitation`.

- [ ] **Step 1: Write failing invitation lifecycle tests**

Test registered email, unregistered email, exact User ID, duplicate pending invitation, owner invitation, existing member, expiry, cancellation, accept, reject, unauthorized inviter, minimal identity response, and fake Resend delivery.

The successful User-ID case must assert:

```go
if invitation.InviteeUserID != userC.ID || invitation.InviteeEmail != userC.Email {
    t.Fatalf("resolved invitation target incorrectly: %+v", invitation)
}
if invitation.TokenHash == "" || strings.Contains(invitation.TokenHash, "invite_") {
    t.Fatal("database model must contain only the invitation token hash")
}
```

- [ ] **Step 2: Run focused tests to verify failure**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestInvitation|TestResolveIdentity"
```

Expected: routes or repository types are missing.

- [ ] **Step 3: Implement exact identity resolution**

Normalize email with `strings.ToLower(strings.TrimSpace(value))`. Accept User IDs only when they match `^usr_[0-9a-f]{32}$`. Return a safe match containing ID, display name, avatar URL, and masked email; do not implement prefix search.

- [ ] **Step 4: Implement invitation transactions**

Create invitation records separately from members. Generate 32 random bytes, return the raw token only to the mail-building service, and persist `models.HashToken(raw)`. Acceptance locks the invitation row, validates status/expiry/identity, inserts membership, closes the invitation, and writes activity in one transaction.

- [ ] **Step 5: Send and safely retry Resend messages**

Commit the invitation before calling Resend. On delivery failure update `delivery_status='failed'`, return `EMAIL_DELIVERY_FAILED`, and retain the pending invitation for owner retry. A retry rotates the raw token and stored hash before sending.

- [ ] **Step 6: Wire routes and run tests**

Run:

```powershell
Set-Location backend
Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go test -count=1 ./tests/... -run "TestInvitation|TestResolveIdentity"
go test -count=1 ./tests/... -run "TestProjectSharing"
```

Expected: all invitation and existing collaboration tests pass.

- [ ] **Step 7: Commit**

```powershell
git add backend/models/project_access.go backend/repository/mssql_invitation_repository.go backend/services/mailer.go backend/functions/projects backend/api backend/tests/invitations_user_id_test.go
git diff --cached --check
git commit -m "feat: support project invitations by email or user ID"
```

### Task 8: Persist Maps and Upload Ownership

**Files:**
- Create: `backend/repository/mssql_map_repository.go`
- Create: `backend/functions/maps/function.go`
- Create: `backend/tests/maps_uploads_test.go`
- Modify: `backend/models/project_access.go`
- Modify: `backend/storage/temporary.go`
- Modify: `backend/functions/upload/function.go`
- Modify: `backend/api/router.go`
- Modify: `backend/main.go`

**Interfaces:**
- Produces `MapRepo.CreateMap`, `GetMap`, `ListMaps`, `SaveConfiguration`, `DeleteMap`, `CreateUpload`, `ListUploads`, and `DeleteUpload`.
- Produces server-generated persistent paths rooted at user/project/map IDs.

- [ ] **Step 1: Write failing persistent ownership tests**

Test map creation/list/save/reload, upload metadata, restart persistence, member permission, and IDOR. Assert that saved JSON survives a repository restart and the stored upload path stays beneath the configured persistent root.

```go
if !strings.HasPrefix(cleanPath, cleanRoot+string(os.PathSeparator)) {
    t.Fatalf("upload path escaped persistent root: %s", cleanPath)
}
```

- [ ] **Step 2: Run tests to verify missing routes**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestMapPersistence|TestUploadOwnership|TestMapIDTampering"
```

Expected: 404 or missing repository types.

- [ ] **Step 3: Implement map repository transactions**

Map creation checks `CreateMap`; configuration saving checks `EditMap`; deletion checks `DeleteMap`. Parse incoming configuration with `json.Valid` before executing SQL. Use `OUTPUT INSERTED` or a follow-up query within the same transaction to return the persisted record.

- [ ] **Step 4: Implement owned persistent storage paths**

Add:

```go
func (sm *StorageManager) GetPersistentMapDir(userID, projectID, mapID string) (string, error)
func (sm *StorageManager) GetPersistentUploadPath(userID, projectID, mapID, uploadID, filename string) (string, error)
```

Every segment must match generated ID formats, filenames must use `filepath.Base`, and `SafePath` must enforce the canonical root prefix.

- [ ] **Step 5: Update upload handling**

Require authenticated user, project ID, optional map ID, and effective `UploadFile` permission. Stream to a staging file, insert metadata only after the file is complete, and delete the staged file on database failure.

- [ ] **Step 6: Wire routes and run tests**

Run:

```powershell
Set-Location backend
Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go test -count=1 ./tests/... -run "TestMapPersistence|TestUploadOwnership|TestMapIDTampering"
go test -count=1 ./tests/... -run "TestAPI_CompleteWorkflow|TestProjectAccess"
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```powershell
git add backend/models/project_access.go backend/repository/mssql_map_repository.go backend/storage backend/functions/maps backend/functions/upload backend/api backend/main.go backend/tests/maps_uploads_test.go
git diff --cached --check
git commit -m "feat: persist user-owned maps and uploads"
```

### Task 9: Replace Session Storage with Authenticated Workspaces

**Files:**
- Create: `backend/models/workspace.go`
- Create: `backend/repository/mssql_workspace_repository.go`
- Create: `backend/repository/workspace_repository.go`
- Create: `backend/services/workspace_service.go`
- Create: `backend/functions/workspaces/function.go`
- Create: `backend/tests/workspace_lifecycle_test.go`
- Modify: `backend/api/middleware.go`
- Modify: `backend/api/router.go`
- Modify: `backend/storage/temporary.go`
- Modify: all GIS handlers/services currently accepting session IDs
- Delete after tests migrate: `backend/repository/session_repository.go`

**Interfaces:**
- Produces persistent `WorkspaceRepo`, ephemeral `WorkspaceStateRepository`, and request-context `api.GetWorkspace`.
- Preserves existing dataset/control-point/layer/building/icon GIS method behavior under `WorkspaceID`.

- [ ] **Step 1: Write failing workspace lifecycle tests**

Cover create, ownership denial, multiple workspaces, draft update, simulated restart restoration, explicit save, failed-save draft preservation, discard, and TTL cleanup. The core ownership assertion is:

```go
request.Header.Set("Authorization", "Bearer "+userB.Token)
request.Header.Set("X-Workspace-ID", userAWorkspace.ID)
response := perform(request)
if response.Code != http.StatusForbidden {
    t.Fatalf("expected cross-user workspace denial, got %d", response.Code)
}
```

- [ ] **Step 2: Run tests to verify missing workspace APIs**

Run:

```powershell
Set-Location backend
go test -count=1 ./tests/... -run "TestWorkspace|TestDraft|TestWorkspaceSave"
```

Expected: missing types/routes.

- [ ] **Step 3: Define workspace models and repository**

Use:

```go
type Workspace struct {
    ID                 string          `json:"id"`
    UserID             string          `json:"-"`
    ProjectID          string          `json:"project_id,omitempty"`
    MapID              string          `json:"map_id,omitempty"`
    DraftConfiguration json.RawMessage `json:"draft_configuration"`
    StoragePath        string          `json:"-"`
    CreatedAt          time.Time       `json:"created_at"`
    UpdatedAt          time.Time       `json:"updated_at"`
    ExpiresAt          time.Time       `json:"expires_at"`
}
```

`WorkspaceRepo` stores ownership/draft metadata in MSSQL and its lookups always require both user ID and workspace ID. `WorkspaceStateRepository` owns the existing dataset, control-point, layer, icon, building, floor, and map-project operations keyed by workspace ID; it contains no authentication or persistent-project fallback.

- [ ] **Step 4: Add workspace middleware and storage roots**

Authenticated middleware resolves the bearer token first, then `X-Workspace-ID`, loads the owned workspace, and places it in request context. Temporary files live under `temp/workspaces/<user_id>/<workspace_id>/`; reject any derived path outside that root.

- [ ] **Step 5: Port the in-memory GIS aggregate**

Move the session aggregate into `workspace_repository.go`, rename it to workspace semantics, and add `UserID`, `ProjectID`, and `MapID`. Keep dataset, control point, layer, icon, building, floor, saved-draft, and map-project operations behaviorally identical. Replace `sessionID` parameters with `workspaceID` mechanically, hydrate only from the owned workspace directory, then remove global dataset lookup fallbacks that bypass ownership.

- [ ] **Step 6: Implement transactional save promotion**

`WorkspaceService.Save` authorizes project/map access, validates JSON and referenced datasets, begins a DB transaction, promotes staged files, upserts the map/uploads/activity, commits, then calls `SafeRemoveAll`. On any failure it rolls back and retains or restores the draft.

- [ ] **Step 7: Replace routes and cleanup worker**

Add `/api/workspaces` routes. Replace the session-expiry goroutine with workspace expiry based on `WorkspaceTTL`. Remove `/sessions` routes only after all migrated API tests use authentication and `X-Workspace-ID`.

- [ ] **Step 8: Run the backend GIS regression gate**

Run:

```powershell
Set-Location backend
Get-ChildItem -Recurse -Filter *.go | ForEach-Object { gofmt -w $_.FullName }
go test -count=1 ./tests/... -run "TestWorkspace|TestDraft|TestWorkspaceSave"
go test -count=1 ./tests/...
go vet ./...
```

Expected: every existing GIS test passes with workspace identity and no `/sessions` dependency.

- [ ] **Step 9: Commit**

```powershell
git add backend/models/workspace.go backend/repository backend/services backend/functions backend/api backend/storage backend/tests backend/main.go
git diff --cached --check
git commit -m "feat: replace sessions with authenticated workspaces"
```

### Task 10: Migrate the Frontend to Workspaces and Remove End Session

**Files:**
- Create: `frontend/src/lib/workspaceDraft.js`
- Create: `frontend/src/lib/workspaceDraft.test.js`
- Modify: `frontend/src/api.js`
- Modify: `frontend/src/App.jsx`
- Modify: workspace/dataset/upload pages and components that call session APIs
- Modify: `frontend/src/hooks/useWorkspace.js`
- Delete: `frontend/src/components/EndSessionDialog.jsx`
- Delete: `frontend/src/lib/sessionLoading.js`
- Delete: `frontend/src/lib/sessionLoading.test.js`

**Interfaces:**
- Consumes Task 9 workspace APIs.
- Produces `WORKSPACE_STORAGE_KEY`, `ensureActiveWorkspace`, draft restoration, and explicit save/discard flows.

- [ ] **Step 1: Write failing workspace helper tests**

Create:

```js
import test from "node:test"
import assert from "node:assert/strict"
import { chooseRestorableWorkspace, draftStatus } from "./workspaceDraft.js"

test("chooses the newest non-expired owned draft", () => {
  const drafts = [
    { id: "wrk_old", updated_at: "2026-11-01T00:00:00Z", expires_at: "2026-12-01T00:00:00Z" },
    { id: "wrk_new", updated_at: "2026-11-02T00:00:00Z", expires_at: "2026-12-02T00:00:00Z" },
  ]
  assert.equal(chooseRestorableWorkspace(drafts, new Date("2026-11-05T00:00:00Z")).id, "wrk_new")
  assert.equal(draftStatus(true, false), "Unsaved changes")
})
```

- [ ] **Step 2: Run tests to verify missing module**

Run:

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
```

Expected: module-not-found failure.

- [ ] **Step 3: Replace session client state**

Remove `SESSION_STORAGE_KEY`, `getActiveSessionId`, `ensureActiveSession`, `endSession`, session map-config functions, and `/sessions/.../configs` functions. Add `WORKSPACE_STORAGE_KEY`, workspace CRUD/save calls, and an interceptor that sends `X-Workspace-ID` only when a workspace is active.

- [ ] **Step 4: Remove startup session creation**

Delete the `App.jsx` effect that calls `ensureActiveSession`. Create a workspace only after authentication when the user starts/imports work or resumes a draft.

- [ ] **Step 5: Migrate pages and hooks**

Replace session-loading calls with `ensureActiveWorkspace({ projectId, mapId })`. Add a visible `Saved` or `Unsaved changes` indicator. Debounce draft configuration updates, cancel pending updates on unmount, and never call persistent map-save APIs without explicit user action.

- [ ] **Step 6: Remove End Session everywhere**

Delete the component, imports, buttons, dialogs, CSS selectors, and API calls. Add explicit `Discard draft` action with confirmation and keep logout separate from draft deletion.

- [ ] **Step 7: Run search, tests, and build**

Run:

```powershell
rg -n "End Session|EndSession|endSession|X-Session-ID|SESSION_STORAGE_KEY|/sessions" frontend/src
Set-Location frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"
```

Expected: `rg` returns no matches and tests/build pass.

- [ ] **Step 8: Commit**

```powershell
git add frontend/src
git diff --cached --check
git commit -m "feat: migrate editor state to user workspaces"
```

### Task 11: Complete Dashboard, Member, and Invitation UI

**Files:**
- Create: `frontend/src/components/InvitationInput.jsx`
- Modify: `frontend/src/components/ShareProjectModal.jsx`
- Modify: `frontend/src/pages/DashboardPage.jsx`
- Modify: `frontend/src/api.js`
- Modify: `frontend/src/pages/README.md`
- Modify: `frontend/src/components/README.md`

**Interfaces:**
- Consumes Tasks 6-8 APIs.
- Produces owned/shared project views, maps/uploads/activity/member views, and email/User-ID invitation workflow.

- [ ] **Step 1: Add pure invitation-input classification tests**

Add exported `classifyInviteTarget` beside the component and test:

```js
assert.deepEqual(classifyInviteTarget("USER@Example.COM "), { type: "email", value: "user@example.com" })
assert.deepEqual(classifyInviteTarget("usr_0123456789abcdef0123456789abcdef"), {
  type: "user_id",
  value: "usr_0123456789abcdef0123456789abcdef",
})
assert.equal(classifyInviteTarget("partial-name"), null)
```

- [ ] **Step 2: Run frontend tests to verify failure**

Run:

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
```

Expected: missing helper/component failure.

- [ ] **Step 3: Implement resolve-confirm-invite UI**

Use placeholder `Enter email address or User ID`. Resolve only exact valid inputs, display the safe matched identity, allow role and permission selection, and require a second confirmation before creating the invitation. Display Resend delivery failure with a retry action.

- [ ] **Step 4: Complete project dashboard sections**

Owned and shared projects must display owner, role, maps, uploads, members, and recent activity. Member actions are hidden or disabled based on effective permissions returned by the API. Removing a member refreshes project access immediately.

- [ ] **Step 5: Run tests and build**

Run:

```powershell
Set-Location frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"
```

Expected: tests and build pass.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src/api.js frontend/src/components/InvitationInput.jsx frontend/src/components/ShareProjectModal.jsx frontend/src/components/README.md frontend/src/pages/DashboardPage.jsx frontend/src/pages/README.md
git diff --cached --check
git commit -m "feat: complete project membership and invitation UI"
```

### Task 12: Back Up, Verify, Remove Legacy Persistence, and Complete Acceptance

**Files:**
- Create: `backend/scripts/backup-legacy.ps1`
- Create: `backend/scripts/verify-migration.ps1`
- Modify: `backend/scripts/test-all.ps1`
- Modify: `test-all.ps1`
- Modify: root/backend/frontend/API/repository/models/storage/pages/component READMEs
- Modify: `.env.example`
- Delete: legacy JSON auth/project migration/runtime files after verification

**Interfaces:**
- Consumes all prior tasks.
- Produces a verified migration report, no runtime legacy/session code, updated documentation, and complete test evidence.

- [ ] **Step 1: Write the backup script with bounded targets**

`backup-legacy.ps1` accepts explicit `-SourceRoot` and `-BackupRoot`, resolves both absolute paths, rejects equal/parent-root targets, copies only named legacy directories and an MSSQL backup file, and writes a SHA-256 manifest. It never deletes data.

- [ ] **Step 2: Write the verification script**

`verify-migration.ps1` accepts `-ConnectionString` and `-StorageRoot`, checks schema version, orphan foreign keys, project/map/upload counts, referenced file existence, pending invitation invariants, and absence of raw token-shaped values. It exits nonzero on any failure and writes a timestamped report without credentials.

- [ ] **Step 3: Run backup and migration verification before deletion**

Run with explicit local paths:

```powershell
powershell -ExecutionPolicy Bypass -File backend/scripts/backup-legacy.ps1 -SourceRoot backend/storage -BackupRoot backend/migration-backups/2026-11-05
powershell -ExecutionPolicy Bypass -File backend/scripts/verify-migration.ps1 -ConnectionString $env:MSSQL_TEST_CONNECTION_STRING -StorageRoot backend/storage
```

Expected: both scripts exit 0 and the manifest/report contain no secrets.

- [ ] **Step 4: Remove legacy runtime code and data only after verification**

First replace any test-server dependency on JSON repositories with `_test.go` fakes that implement the contextual `AuthRepo`, `ProjectRepo`, `WorkspaceRepo`, and `WorkspaceStateRepository` interfaces entirely inside `backend/tests/test_server_test.go`. Integration tests marked for MSSQL continue to use `MSSQL_TEST_CONNECTION_STRING`.

Then delete JSON auth/project repositories, JSON-to-MSSQL startup migration, session repository, session directories, and obsolete organization schema/runtime files. Preserve the verified backup. Re-run:

```powershell
rg -n "NewAuthRepository|NewProjectRepository|SessionRepository|X-Session-ID|/sessions|organizations" backend frontend/src
```

Expected: no runtime references; migration history or release documentation may mention removed terms only when clearly historical.

- [ ] **Step 5: Update documentation and environment templates**

Document required MSSQL and Resend variables, workspace paths/TTL, profile routes, project/map/upload APIs, invitation rules, migration/restore commands, and the removal of End Session. Use safe example values only.

- [ ] **Step 6: Run the complete automated gate**

Run:

```powershell
Set-Location backend
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -v -count=1 ./tests/...
Set-Location ..\frontend
cmd.exe /c "npm.cmd test"
cmd.exe /c "npm.cmd run build"
Set-Location ..
powershell -ExecutionPolicy Bypass -File test-all.ps1
```

Expected: `gofmt -l .` prints nothing and every command exits 0. `backend/scripts/test-all.ps1` must also run `go test -count=1 ./...` so package-local tests cannot be skipped.

- [ ] **Step 7: Perform browser acceptance**

With the real local backend, dedicated MSSQL database, and Resend test sender configured, verify:

1. User A registers, verifies email, opens profile, edits name/avatar, copies User ID, and changes password.
2. User A creates a project, multiple maps, and uploads datasets.
3. User A edits without saving, refreshes, and restores the draft.
4. Explicit save persists the configuration; reopening loads saved state.
5. User B accepts an email invitation; User C accepts a User-ID invitation.
6. Roles and overrides permit/deny the expected operations.
7. Changing project/map/upload/workspace IDs cannot expose another user's data.
8. Removing a member revokes access immediately.
9. No End Session control or `/sessions` request appears.
10. Georeferencing, control points, layers, icons, buildings/floors, imports, exports, PDF/CAD, MBTiles, and raster processing remain operational.

Record browser URL, API listener, console errors, failed network requests, and screenshots for any defect. Do not call this complete if browser automation is unavailable.

- [ ] **Step 8: Commit final cleanup and documentation**

```powershell
git add .env.example README.md backend frontend test-all.ps1
git diff --cached --check
git diff --cached --name-status
git commit -m "docs: finalize user-owned persistence migration"
```

- [ ] **Step 9: Report delivery evidence**

Provide the modified-file list, migration/backup locations, schema version, automated command results, browser acceptance results, and any environment-only setup still required. Do not report publication or deployment unless separately requested and verified.
