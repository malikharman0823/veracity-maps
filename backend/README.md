# Veracity Maps — Backend Overview

The **Veracity Maps Backend** is a modular, zero-dependency Go web service built using the standard Go `net/http` library. It exposes RESTful API endpoints for managing geospatial raster datasets, validating files, executing least-squares affine transformations, managing layers, placing customizable geographic icon markers, managing multi-floor building plans, and importing multi-dataset ZIP archives.

---

## 1. Directory Structure

| Package / Folder | Purpose |
| :--- | :--- |
| [`api/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/api/README.md) | Central HTTP routing, parameter extraction, and CORS/Recovery middleware. |
| [`config/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/config/README.md) | Application configuration and environment variable parsing. |
| [`functions/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/README.md) | Domain HTTP handlers organized by resource (`health`, `upload`, `datasets`, `georeference`, `export`). |
| [`gis/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/gis/README.md) | Spatial raster inspection, native TIFF dimension decoding, least-squares affine regression solver, and GDAL VRT/world file generators. |
| [`models/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/models/README.md) | Common data structures, domain entities (`Dataset`, `Layer`, `ControlPoint`, `IconPoint`, `Building`, `BuildingFloor`), and API responses. |
| [`repository/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/repository/README.md) | Thread-safe, in-memory session, dataset, icon points, and building/floor store (`SessionRepository`). |
| [`scripts/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/scripts/README.md) | Automated testing, build, and static analysis PowerShell scripts. |
| [`services/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/services/README.md) | Core business logic (`DatasetService` with archive inspection, `GeoreferenceService`, `ExportService`). |
| [`storage/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/storage/README.md) | Temporary per-session disk workspace manager and Windows file-lock safe deletion utilities. |
| [`tests/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/tests/README.md) | Comprehensive test suite (unit, GIS solver, repository, building/floor tests, multi-dataset zip tests, and API integration matrices). |

---

## 2. API Capabilities

- **Icon Points**:
  - `GET /icons`: Discovers and lists available icons from `manifest.json`.
  - `GET /icons/{id}`: Serves icon asset file (PNG/SVG/WebP/ICO/JPG) with caching headers.
  - `POST /datasets/{id}/icon-points`: Creates an icon point associated with dataset, building, and floor.
  - `GET /datasets/{id}/icon-points`: Lists icon points for a dataset.
  - `GET /icon-points/{id}`: Retrieves an icon point.
  - `PUT /icon-points/{id}`: Updates name, description, icon, scope, floor, or geographic coordinates.
  - `DELETE /icon-points/{id}`: Deletes an icon point.

- **Buildings & Floor Plans**:
  - `POST /buildings`: Creates a new building entity.
  - `GET /buildings`: Lists buildings in the current session.
  - `GET /buildings/{id}`: Retrieves a building with its floor definitions and assigned datasets.
  - `PUT /buildings/{id}`: Updates building details.
  - `DELETE /buildings/{id}`: Deletes a building without deleting the underlying datasets.
  - `POST /buildings/{id}/floors`: Adds a floor plan with level, label, and assigned datasets.
  - `PUT /buildings/{id}/floors/{floorId}`: Updates floor plan properties or sorting.
  - `DELETE /buildings/{id}/floors/{floorId}`: Removes a floor plan.

- **Multi-Dataset ZIP Inspection & Import**:
  - `POST /datasets/inspect-archive`: Inspects uploaded archive, classifies raster files, pairs matching sidecars (`.pgw`, `.jgw`, `.tfw`, `.wld`, `.prj`), infers buildings and floors, and returns a staged token and candidate list.
  - `POST /datasets/import-archive-selected`: Selectively imports chosen candidate datasets into the active session with building and floor assignments.

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
