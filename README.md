# Veracity Maps

**Veracity Maps** is a high-performance, lightweight geospatial raster management and visual georeferencing web platform. Built with a pure-Go backend (`net/http`) and a responsive modern React / Vite / TailwindCSS / MapLibre GL frontend, it provides an end-to-end workflow for uploading unreferenced aerial and historical maps, interactively placing ground control points (GCPs), computing mathematical least-squares georeferencing transformations, and exporting GIS-compatible raster bundles and project archives.

---

## 1. System Architecture

```mermaid
graph TD
    subgraph Browser ["Frontend (React 19 + Vite + MapLibre GL)"]
        UI[User Interface & Canvas]
        GCP[Visual Control Point Placer]
        Viewer[MapLibre GL Map Viewport]
        Client[Axios Client (Session Scoped)]
    end

    subgraph Server ["Backend (Go 1.22+ Standard net/http)"]
        Router[HTTP Router & CORS Middleware]
        Repo[(In-Memory Session Repository)]
        Storage[(Per-Session Temporary Disk Storage)]
        GIS[GIS Least-Squares Transform Engine]
        Exporter[MTMAP & VRT Export Engine]
    end

    UI -->|Multipart Upload| Router
    GCP -->|Coordinates & Points| Router
    Viewer -->|Raw Raster Overlay| Router
    Router --> Client
    Router --> Repo
    Router --> Storage
    Router --> GIS
    Router --> Exporter
```

### Technology Stack
- **Backend**: Go (`go1.22+`), zero external dependencies, standard library (`net/http`, `image`, `archive/zip`, `crypto/rand`, `math`).
- **Frontend**: React 19, Vite 7, TailwindCSS 4, MapLibre GL JS 6, Lucide Icons.
- **Persistence**: Zero external database. State is managed via an in-memory session repository (`SessionRepository`) with atomic read/write locks, combined with isolated temporary disk workspaces (`temp/sessions/<session_id>/`).
- **GIS Engine**: Pure-Go implementation of 2-point similarity and 3-to-10-point affine least-squares regression, world file parsing/generation (`.pgw`, `.jgw`, `.wld`), GDAL VRT XML generation, and optional CLI integration with `gdal_translate`/`gdalwarp` if present in PATH.

---

## 2. Implemented Features vs MVP Scope

| Feature Area | Implemented & Verified | Scope Notes |
| :--- | :--- | :--- |
| **Anonymous Sessions** | Yes | Ephemeral sessions with `X-Session-ID` isolation and background TTL expiry |
| **Raster Uploads** | Yes | Supported formats: PNG, JPEG, and ZIP bundles containing sidecars (`.pgw`, `.prj`) |
| **Raster Inspection** | Yes | Dimension extraction, world file parameter calculation, bounds detection |
| **Control Point Georeferencer** | Yes | Interactive image-to-world coordinate pair entry (2 to 10 points) |
| **Least-Squares Regression** | Yes | Pure-Go similarity and affine transformations with collinearity rejection |
| **Map Visualization** | Yes | MapLibre GL raster overlays with strict bounds `[west, south, east, north]` |
| **Layer Management** | Yes | Layer visibility toggle, dynamic opacity adjustments, and multi-floor sorting |
| **Export Engine** | Yes | MTMAP project archive packaging (`.mtmap`), GDAL VRT XML, and world files |
| **Security Mitigations** | Yes | ZIP-Slip path-traversal rejection, strict ID validation, cross-session isolation |

---

## 3. Repository Directory Structure

```text
veracity-maps/
├── backend/                     # Go backend module (net/http API & GIS algorithms)
│   ├── api/                     # Router, route dispatcher, and CORS/recovery middleware
│   ├── config/                  # Server configuration and environment variable parsing
│   ├── functions/               # HTTP handlers (health, upload, datasets, georef, export)
│   ├── gis/                     # Spatial inspection, least-squares solver, VRT/world files
│   ├── models/                  # Shared data models (Dataset, Layer, ControlPoint, API)
│   ├── repository/              # Thread-safe in-memory session repository
│   ├── scripts/                 # PowerShell automated test and verification runners
│   ├── services/                # Business logic services (dataset, georef, export)
│   ├── storage/                 # Temporary session directory and Windows file lock manager
│   ├── tests/                   # Complete unit, integration, and API matrix test suites
│   ├── go.mod                   # Go module (github.com/malikharman0823/veracity-maps)
│   └── main.go                  # Backend application entry point
├── frontend/                    # React / Vite frontend application
│   ├── src/                     # React source components, pages, hooks, and API client
│   │   ├── components/          # TopHeader, Sidebar, WorkspaceMap, LayerPanel, dialogs
│   │   ├── hooks/               # Custom hooks (useWorkspace, use-mobile)
│   │   ├── lib/                 # Utility functions and navigation helpers
│   │   ├── pages/               # Views: HomePage, UploadPage, DatasetPage, WorkspacePage
│   │   ├── api.js               # Centralized Axios API client with session management
│   │   └── main.jsx, App.jsx    # Client router and mount entry points
│   ├── index.html               # Frontend HTML root
│   └── package.json             # NPM package manifest (veracity-maps)
├── reports/                     # Quality assurance, security, and test inventory reports
├── testdata/                    # Reusable, reproducible deterministic GIS test fixtures
├── test-all.ps1                 # Root convenience PowerShell test runner
└── .gitignore                   # Multi-environment git exclusion rules
```

---

## 4. Prerequisites & Environment Setup

### Required Tools
- **Go**: Version `1.22` or later (`go version`)
- **Node.js**: Version `18.0` or later (`node --version`)
- **npm**: Version `9.0` or later (`npm --version`)
- **Operating System**: Windows 10/11, Linux, or macOS

### Optional External GIS Tools
- **GDAL Command-Line Tools** (`gdalinfo`, `gdal_translate`, `gdalwarp`): If present in `PATH`, the backend automatically generates optimized GeoTIFF binaries. If absent, the application gracefully operates using native pure-Go raster processing and GDAL VRT XML descriptors.

---

## 5. Startup Instructions

### 1. Start the Go Backend
From the workspace root:
```powershell
Set-Location backend
go run main.go
```
The backend initializes on `http://localhost:8000`. You will see startup diagnostics verifying available storage paths and GDAL CLI status.

### 2. Start the React Frontend
In a separate terminal from the workspace root:
```powershell
Set-Location frontend
cmd.exe /c "npm.cmd run dev"
```
The development server starts on `http://localhost:5173`. Open your browser to begin interacting with the application.

---

## 6. Running Tests & Automated Checks

To run the complete automated test suite (including prerequisites, Go formatting, `go vet`, unit tests, full API integration matrix, and frontend production build verification):

```powershell
# From the workspace root:
powershell -ExecutionPolicy Bypass -File test-all.ps1
```

Or run individual targeted commands:
- **Backend Unit & Integration Tests:**
  ```powershell
  Set-Location backend
  go test -v -count=1 ./tests/...
  ```
- **Backend Static Analysis:**
  ```powershell
  Set-Location backend
  gofmt -l .
  go vet ./...
  ```
- **Frontend Production Build:**
  ```powershell
  Set-Location frontend
  cmd.exe /c "npm.cmd run build"
  ```

---

## 7. Security & File Safety

1. **ZIP-Slip Prevention:** Archive extractions enforce strict canonical destination prefix checks (`strings.HasPrefix(targetPath, cleanDestDir)`), discarding any directory traversal entries (such as `../outside.txt`).
2. **Input Validation:** Control point pixel inputs must lie strictly within the raster's measured dimensions (`[0, width)`, `[0, height)`), and geographic coordinates must lie within `[-180, 180]` longitude and `[-90, 90]` latitude.
3. **Session Isolation:** All temporary storage directories are partitioned under `backend/temp/sessions/<session_id>/`. Cross-session dataset lookups, modifications, and downloads are rejected with HTTP 404.
4. **Subprocess Sanitization:** All external commands (e.g. GDAL CLI tools) pass arguments as discrete string arrays without invoking shell command interpreters, preventing command injection attacks.

---

## 8. Guidance for AI Coding Agents

- **Source Code is Ground Truth:** When modifying functionality, inspect the actual source code and running tests rather than relying solely on high-level documentation.
- **Maintain Invariants:**
  - Always enforce MapLibre GL bounding box ordering: `[west, south, east, north]`.
  - On Windows, always invoke filesystem cleanups with lock retries and reset read-only attributes before deleting directories.
- **Zero Database Assumption:** Do not attempt to configure PostgreSQL, SQLite, or ORM migrations; session state must remain ephemeral and thread-safe in memory.
- **Keep Documentation Synchronized:** Update corresponding folder READMEs whenever adding or altering endpoints, data contracts, or GIS transformation algorithms.
