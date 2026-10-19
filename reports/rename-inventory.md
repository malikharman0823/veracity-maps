# Veracity Maps — Rename & Migration Inventory

**Date:** 2026-10-19  
**Source Root:** `c:\Users\harma\OneDrive\Desktop\veracity-maps`  
**Target Repository:** `https://github.com/malikharman0823/veracity-maps.git`  
**Target Branch:** `chore/rename-to-veracity-maps`  
**Default Branch:** `main`  

---

## 1. Existing Project Structure

```text
veracity-maps/
├── backend/
│   ├── api/                     # HTTP router & middleware
│   ├── config/                  # Server & environment config
│   ├── functions/               # Handlers: health, upload, datasets, georeference, export
│   ├── gis/                     # Spatial inspect & least-squares transform
│   ├── models/                  # Data structures (Dataset, Layer, ControlPoint, Response)
│   ├── repository/              # In-memory thread-safe session repository
│   ├── scripts/                 # PowerShell test runner (test-all.ps1)
│   ├── services/                # Business services (dataset, georeference, export)
│   ├── storage/                 # Temporary session directory manager
│   ├── tests/                   # Integration, unit, and API matrix tests
│   ├── go.mod                   # Go module definition
│   └── main.go                  # Backend entry point
├── frontend/
│   ├── src/                     # React/Vite source code
│   │   ├── components/          # TopHeader, Sidebar, WorkspaceMap, LayerPanel, etc.
│   │   ├── pages/               # HomePage, UploadPage, DatasetPage, WorkspacePage, etc.
│   │   ├── api.js               # Centralized API client
│   │   └── App.jsx, main.jsx    # Client entry points
│   ├── package.json             # Frontend package manifest
│   └── index.html               # Frontend HTML root
├── reports/                     # Test, security, and migration reports
├── testdata/                    # Deterministic test fixtures & generator
├── test-all.ps1                 # Root convenience test runner
└── .gitignore                   # Workspace gitignore rules
```

---

## 2. Identified Old-Name References

| Category | File Location | Old Reference | Planned New Reference |
| :--- | :--- | :--- | :--- |
| **Go Module** | `backend/go.mod` | `module maptiler-go` | `module github.com/malikharman0823/veracity-maps` |
| **Go Imports** | All Go files (`backend/**/*.go`) | `"maptiler-go/..."` | `"github.com/malikharman0823/veracity-maps/..."` |
| **Backend Logs/Headers** | `backend/main.go`, `api/middleware.go` | `"MapTiler Go Backend"` | `"Veracity Maps Backend"` |
| **Frontend Package** | `frontend/package.json` | `"maptiler-clone-frontend"` | `"veracity-maps"` |
| **Frontend HTML Title** | `frontend/index.html` | `<title>MapTiler Clone</title>` | `<title>Veracity Maps</title>` |
| **Frontend UI Branding** | `frontend/src/components/TopHeader.jsx`, `Sidebar.jsx` | `"MapTiler Clone"`, `"MapTiler"` | `"Veracity Maps"` |
| **Test Output / Scripts** | `backend/scripts/test-all.ps1`, `test-all.ps1` | `"MapTiler MVP - ..."` | `"Veracity Maps - ..."` |
| **Test Fixture Generator** | `testdata/generate_testdata.go` | Directory naming `testdata/maptiler-mvp` | Update metadata & retain fixture backwards compatibility |

### Legitimate Third-Party / Preserved Usage (Not Renamed)
- **MapLibre GL**: `maplibre-gl` library imports and MapLibre GL bounds ordering `[west, south, east, north]` must NOT be renamed.
- **GDAL & PROJ**: GIS CLI tool names and VRT metadata strings (`GDAL_VRT`) must NOT be renamed.
- **MTMAP**: `project.mtmap` project archive format extension must be preserved for compatibility.
- **Historical Reference Repositories**: Mentions of `https://github.com/malikharman0823/map-tiler` in documentation as the behavioral reference are preserved for traceability.

---

## 3. Relevant Entry Points

- **Backend:** [`backend/main.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/main.go) (HTTP server on port 8000).
- **Frontend:** [`frontend/src/main.jsx`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/main.jsx) / [`frontend/index.html`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/index.html) (Vite server on port 5173).
- **Test Runner:** [`backend/scripts/test-all.ps1`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/scripts/test-all.ps1) and [`test-all.ps1`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/test-all.ps1).

---

## 4. Existing Test Commands

- **Backend Tests:** `go test -v -count=1 ./tests/...` (executed from `backend/`)
- **Backend Vet:** `go vet ./...` (executed from `backend/`)
- **Backend Format:** `gofmt -l .` (executed from `backend/`)
- **Frontend Build:** `npm.cmd run build` (executed from `frontend/`)
- **All-in-One Automated Suite:** `powershell -ExecutionPolicy Bypass -File test-all.ps1`

---

## 5. Remote Repository & Branch State

- **Target URL:** `https://github.com/malikharman0823/veracity-maps.git`
- **Remote Inspection (`git ls-remote`):** Confirmed existing, empty remote repository (0 branches, 0 tags, 0 commits).
- **GitHub Authentication:** Authenticated as `malikharman0823` via GitHub CLI (`gh auth status`).
- **Target Working Branch:** `chore/rename-to-veracity-maps`
- **Default Branch:** `main`

---

## 6. Compatibility & Migration Risks

1. **Go Module Import Path Changes:**
   - Renaming `module maptiler-go` to `github.com/malikharman0823/veracity-maps` requires updating all 25+ Go source files across `api/`, `config/`, `functions/`, `gis/`, `models/`, `repository/`, `services/`, and `tests/`.
   - *Mitigation:* Perform systematic update and verify with `go build ./...` and `go test -count=1 ./tests/...`.
2. **Git Initialization on Empty Remote:**
   - The remote is empty. Initializing git locally on branch `main`, creating initial commit or creating `chore/rename-to-veracity-maps`, and pushing safely to the remote without overwriting or force-pushing.
   - *Mitigation:* Check branch names before push, push feature branch `chore/rename-to-veracity-maps`, open pull request targeting `main`.
