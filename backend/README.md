# Veracity Maps — Backend Overview

The **Veracity Maps Backend** is a modular, zero-dependency Go web service built using the standard Go `net/http` library. It exposes RESTful API endpoints for managing geospatial raster datasets, validating files, executing least-squares affine transformations, managing layers, placing customizable geographic icon markers, managing multi-floor building plans, and importing multi-dataset ZIP archives.

---

Account profiles persist in MSSQL through `MSSQLProfileRepository`; production
startup also requires `RESEND_API_KEY`, `RESEND_FROM_EMAIL`, and `APP_PUBLIC_URL`
for verification mail. See [profile handlers](functions/profile/README.md) for
account/password/avatar/email API contracts. GIS sessions retain their existing
temporary storage lifecycle.

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

- **Passwordless OTP Authentication**:
  - `POST /api/auth/otp/send`: Generates and sends a single-use 6-digit numeric OTP via Resend email for login or registration, protected by a 60-second cooldown rate limit.
  - `POST /api/auth/otp/verify`: Validates the 6-digit OTP code against SHA-256 hash storage (max 5 attempts, 10-minute expiry), marks it consumed, and issues a session token.

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
  - `POST /buildings/{id}/propagate`: Propagates any georeferenced floor footprint, world files, and affine transform to all sibling floors in the building.

- **Multi-Dataset ZIP Inspection & Import**:
  - `POST /datasets/inspect-archive`: Inspects uploaded archive, classifies raster files, pairs matching sidecars (`.pgw`, `.jgw`, `.tfw`, `.wld`, `.prj`), infers buildings and floors, and returns a staged token and candidate list.
  - `POST /datasets/import-archive-selected`: Selectively imports chosen candidate datasets into the active session with building and floor assignments.

---

## 3. Running & Verifying Backend

Startup validates configuration, connects and pings MSSQL with a context deadline,
and applies migrations before creating the HTTP server. Connection or migration
failure terminates startup; there is no startup JSON import or connection-failure
fallback. Set `MSSQL_CONNECTION_STRING` securely; `.env.example` describes the
configuration (the executable reads process environment variables).

`db.InitDB(ctx, connStr)` opens the connection pool and calls `PingContext`.
`db.RunMigrations(ctx, database, dir)` reads sorted `.sql` files, skips filenames
already in `dbo.SchemaMigrations`, and executes each new file and its ledger insert
in one transaction. Standalone `GO` lines separate batches within that transaction.
Run the executable from `backend/` so the `migrations/` directory is available.

Migration 002 adds user profile, personal ownership, permission JSON, map, upload,
activity, draft and verification-token structures. Existing organization tables
and project compatibility columns remain during the additive rollout. Migration
003 deliberately invalidates all existing authentication tokens and replaces
their table with SHA-256 token hashes and revocation timestamps; users must log
in again. Passwords use bcrypt cost 12, with legacy salted hashes upgraded after
successful login. Production startup uses `MSSQLAuthRepository` for authoritative
user and token reads and writes. Filesystem auth remains for isolated tests and
legacy tooling; project persistence awaits its own cutover. Invitation token
dependencies await their separate cutover.

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
