# Veracity Maps — Backend Overview

The **Veracity Maps Backend** is a modular, zero-dependency Go web service built using the standard Go `net/http` library. It exposes RESTful API endpoints for managing geospatial raster datasets, validating files, executing least-squares affine transformations, managing layers, and generating GIS packages.

---

## 1. Directory Structure

| Package / Folder | Purpose |
| :--- | :--- |
| [`api/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/api/README.md) | Central HTTP routing, parameter extraction, and CORS/Recovery middleware. |
| [`config/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/config/README.md) | Application configuration and environment variable parsing. |
| [`functions/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/README.md) | Domain HTTP handlers organized by resource (`health`, `upload`, `datasets`, `georeference`, `export`). |
| [`gis/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/gis/README.md) | Spatial raster inspection, least-squares affine regression solver, and GDAL VRT/world file generators. |
| [`models/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/models/README.md) | Common data structures, domain entities (`Dataset`, `Layer`, `ControlPoint`), and API responses. |
| [`repository/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/repository/README.md) | Thread-safe, in-memory session and dataset store (`SessionRepository`). |
| [`scripts/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/scripts/README.md) | Automated testing, build, and static analysis PowerShell scripts. |
| [`services/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/services/README.md) | Core business logic (`DatasetService`, `GeoreferenceService`, `ExportService`). |
| [`storage/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/storage/README.md) | Temporary per-session disk workspace manager and Windows file-lock safe deletion utilities. |
| [`tests/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/tests/README.md) | Comprehensive test suite (unit, GIS solver, repository, and full API integration matrices). |

---

## 2. Server Entry Point: `main.go`

- **Purpose**: Initializes application configuration, storage manager, in-memory repository, services, handlers, and the HTTP server with graceful shutdown and background session expiration cleanup.
- **Key Logic**:
  - `storage.NewStorageManager(cfg.SessionStorageDir)`: Ensures the root temporary workspace directory exists.
  - `repository.NewSessionRepository()`: Instantiates the thread-safe session map.
  - `api.NewRouter(...)`: Assembles handlers and middleware.
  - Background Goroutine: Runs every 10 minutes to clean up idle/expired sessions and prune disk directories using `storage.SafeRemoveAll`.
  - HTTP Server: Binds to `cfg.Host:cfg.Port` with 10-minute read/write timeouts to support large GIS uploads and exports.

---

## 3. Running & Verifying Backend

From the `backend/` directory:
```powershell
# Format check
gofmt -l .

# Static analysis
go vet ./...

# Run complete test suite
go test -v -count=1 ./tests/...

# Build binary
go build -o veracity-maps-server.exe main.go
```
