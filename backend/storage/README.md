# Package: `storage`

Located in [`backend/storage/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/storage)

The `storage` package manages the temporary filesystem storage for session workspaces, dataset uploads, generated raster products, and robust deletion on Windows environments.

---

## File: `temporary.go`

### Purpose
Provides directory path resolution, path containment verification (preventing path traversal), and Windows file-lock resilient directory cleanup.

### Key Structs & Functions
- **`type StorageManager struct`**:
  - `BaseSessionDir`: Root path for sessions (`temp/sessions/`).
  - `UploadDir`: Global uploads staging directory.
  - `ProcessedDir`: Processed products directory.
- **`func NewStorageManager(sessionDir, uploadDir, processedDir string) (*StorageManager, error)`**:
  - Initializes storage paths and creates base directories with `0755` permissions.
- **`func (sm *StorageManager) GetSessionDir(sessionID string) string`**:
  - Returns `<BaseSessionDir>/<sessionID>`.
- **`func (sm *StorageManager) GetDatasetDir(sessionID, datasetID string) string`**:
  - Returns `<BaseSessionDir>/<sessionID>/datasets/<datasetID>`.
- **`func (sm *StorageManager) SafePath(baseDir, relativePath string) (string, error)`**:
  - Normalizes target paths using `filepath.Clean` and verifies that the target path begins with `cleanBase + separator`. Rejects path traversal attempts.
- **`func SafeRemoveAll(path string) error`**:
  - Recursively removes directory trees on Windows.
  - Clears `os.Chmod(p, 0666)` (removing read-only attributes that cause Windows permission denied errors).
  - Implements a retry loop (up to 5 attempts with 100ms backoff) to handle transient Windows antivirus or OS file descriptor locks.
