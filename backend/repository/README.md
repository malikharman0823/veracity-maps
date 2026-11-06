# Package: `repository`

`AuthRepo.AuthenticateAndCreateToken(ctx, email, password, duration)` verifies
credentials and issues a token as one operation. MSSQL holds `UPDLOCK, HOLDLOCK`
on the user row through legacy hash upgrade, token insertion, and transaction
commit. Password change takes the same lock before updating/revoking tokens, so
a login that commits first has its token revoked, while a password change that
commits first makes the old-password login fail. Both standalone and atomic
authentication guard legacy upgrades against the originally read hash and salt;
zero updated rows reject stale credentials. The filesystem test repository holds
its mutex across verification and token persistence.

Located in [`backend/repository/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/repository)

The `repository` package provides the thread-safe, in-memory data store for anonymous user sessions, datasets, control points, map projects, and layer configurations.

---

## User-Owned Projects

`ProjectRepository.Create(userID, name, description, ownerName, ownerEmail)` and
`ProjectRepo.CreateProject(userID, name, description)` create projects owned by an individual user.
Collaboration uses project members, invitations, and owner/editor/viewer permissions.
Organization repositories and project-transfer methods have been removed. Existing SQL schema
compatibility columns remain in place; session storage and lifecycle remain the GIS boundary.

## Profile persistence

`MSSQLProfileRepository` implements the context-aware `ProfileRepo` methods:
`Get`, `UpdateName`, `SetAvatar`, `AvatarPath`, `ChangePassword`,
`CreateEmailVerification`, and `ConsumeEmailVerification`. `AvatarPath` is
internal file metadata; public profiles contain an authenticated avatar URL.
SQL statements use named parameters. Password verification, bcrypt replacement,
and revocation of every other token commit in one transaction. The current token
must belong to the user and remain active.

Email requests persist `pending_email` and only the SHA-256 token hash, expiring
after 30 minutes. Initial registration uses `initial_verification` tokens;
profile changes use `email_change` tokens. A shared one-minute per-user window
is checked while holding the user update lock to serialize concurrent requests.
A request consumes preceding pending requests of its purpose. Consumption
locks the user before the token, updates the email/verified flag, clears the
pending email, and consumes the token in one transaction. The unique user email
constraint also prevents a conflicting activation after a request was issued.
Initial-token consumption only verifies the current address and preserves a
pending email change; it rejects stale addresses or already verified accounts.
The MSSQL auth repository reads profile metadata on login/token validation so
`/api/auth/me` reflects account edits immediately.
Migration 002 adds nullable `Projects.owner_user_id`, backfilled only for valid
personal owners (`owner_type='personal'`, no organization, and an existing user).
Current repositories continue using their compatibility fields until the later
repository cutover. `MigrateJSONToMSSQL` is deprecated offline tooling and is never
called at server startup.

`AuthRepo` accepts `context.Context` first on every operation. MSSQL authentication
uses contextual queries and named parameters. New passwords use bcrypt cost 12;
a successful legacy-password login persists a bcrypt replacement and clears its
salt before issuing a token. Migration 003 deliberately invalidates existing auth
tokens and replaces their table with an ID primary key, unique SHA-256 lookup
hash, expiry, creation time, and nullable revocation time. Token creation returns
the raw secret once; validation and revocation hash the presented secret, and
validation excludes expired or revoked rows. `RevokeToken` returns persistence
errors. The filesystem compatibility repository also persists only token hashes
and ignores old raw-token files; it remains for isolated tests and legacy tooling.
Production startup injects `MSSQLAuthRepository` through `AuthRepo` into middleware,
auth handlers, and the existing project consumers. Project persistence remains
on its compatibility repository until its separate cutover.

The existing context-free project repository methods use `context.Background()`
for their internal auth lookups until their separate repository cutover.

## File: `session_repository.go`

### Purpose
Implements zero-database in-memory state management using Go's `sync.RWMutex`.

### Key Structs
- **`type Session struct`**:
  - `ID string`: Session token (`sess_<32-hex>`).
  - `CreatedAt time.Time`: Creation timestamp.
  - `LastActiveAt time.Time`: Updated on every session interaction for idle timeout calculation.
  - `Datasets map[string]*models.Dataset`: Dataset lookup by ID.
  - `ControlPoints map[string][]*models.ControlPoint`: GCP lists mapped by dataset ID.
  - `Layers map[string][]*models.Layer`: Layer lists mapped by dataset ID.
  - `MapProjects map[string]*models.MapProject`: Multi-floor map projects.
  - `MapConfig map[string]interface{}`: Stored zoom and center coordinates.
- **`type SessionRepository struct`**:
  - `mu sync.RWMutex`: Reader/Writer lock guaranteeing race-free concurrent read and write access across HTTP goroutines.
  - `sessions map[string]*Session`: Master dictionary of active sessions.

### Public Methods
- **Session Lifecycle**:
  - `CreateSession() *Session`: Creates a new session with cryptographically random ID.
  - `GetSession(sessionID) (*Session, error)`: Looks up session and updates `LastActiveAt`.
  - `DeleteSession(sessionID)`: Removes session from the repository.
  - `CleanupExpiredSessions(idleTimeout, maxLifetime, onExpire)`: Evaluates all sessions against retention thresholds, invokes the `onExpire` hook for filesystem pruning, and deletes expired sessions.
- **Dataset Operations**:
  - `InsertDataset(sessionID, dataset)`: Registers a dataset under a session.
  - `GetDataset(sessionID, datasetID)`: Retrieves dataset scoped to the session.
  - `ListDatasets(sessionID)`: Returns all datasets for the session.
  - `UpdateDataset(sessionID, dataset)`: Updates dataset metadata.
  - `DeleteDataset(sessionID, datasetID)`: Removes dataset from the session.
- **Control Point Operations**:
  - `ListControlPoints(sessionID, datasetID)`
  - `InsertControlPoint(sessionID, datasetID, cp)`
  - `UpdateControlPoint(sessionID, pointID, updated)`
  - `DeleteControlPoint(sessionID, pointID)`
- **Layer Operations**:
  - `ListLayers(sessionID, datasetID)`
  - `SetLayers(sessionID, datasetID, layers)`
  - `UpdateLayer(sessionID, datasetID, layerID, visible, opacity)`

---

## Testing & Quality

- **Tests**:
  - [`backend/tests/repository_test.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/tests/README.md) validates session creation, concurrent writes, dataset registration, and expiration logic.
