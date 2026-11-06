# Package: `repository`

Located in [`backend/repository/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/repository)

The `repository` package provides the thread-safe, in-memory data store for anonymous user sessions, datasets, control points, map projects, and layer configurations.

---

## User-Owned Projects

`ProjectRepository.Create(userID, name, description, ownerName, ownerEmail)` and
`ProjectRepo.CreateProject(userID, name, description)` create projects owned by an individual user.
Collaboration uses project members, invitations, and owner/editor/viewer permissions.
Organization repositories and project-transfer methods have been removed. Existing SQL schema
compatibility columns remain in place; session storage and lifecycle remain the GIS boundary.
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
