# Package: `repository`

Located in [`backend/repository/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/repository)

The `repository` package provides the thread-safe, in-memory data store for anonymous user sessions, datasets, control points, map projects, and layer configurations.

---

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
