# Instructions & Guidelines for AI Coding Agents

Welcome to **Veracity Maps**. This document defines the universal operating rules, architectural invariants, debugging practices, and change-completion checklists that every AI coding agent must follow when interacting with this repository.

---

## 1. Core Principles & Mindset

1. **Be an Engineer, Not a Code Generator**: Understand requirements, inspect actual working code, create a targeted plan, test immediately, verify real outputs, and document changes.
2. **The Codebase is Ground Truth**: Documentation serves as a starting point. Always verify types, function signatures, and route registrations against the active source files.
3. **Simplicity First (Anti-Overengineering)**: Avoid adding external databases, heavy state managers, speculative tables, unnecessary layers, or third-party dependencies. Pure Go and standard React components are preferred.
4. **Zero Database Architecture**: The backend uses an in-memory thread-safe `SessionRepository` paired with ephemeral per-session temporary directories (`temp/sessions/<session_id>/`). Do NOT introduce SQLite, PostgreSQL, or ORM frameworks.

---

## 2. Invariants & Rules

### Spatial & GIS Invariants
- **MapLibre GL Coordinate Order**: All raster and layer bounding boxes must strictly conform to `[west, south, east, north]` (`[minX, minY, maxX, maxY]`). Never swap latitude and longitude.
- **Affine Transformations**: The standard affine parameter order is `[a, -b, c, d, -e, f]`, corresponding to standard world file conventions (`pixelXSize`, `rotationY`, `rotationX`, `pixelYSize`, `upperLeftX`, `upperLeftY`).
- **Control Point Boundaries**: Raster coordinates (`image_x`, `image_y`) must fall within `[0, width)` and `[0, height)`. Longitudes must fall within `[-180, 180]` and latitudes within `[-90, 90]`.

### Filesystem & Windows Invariants
- **Windows File Lock Recovery**: Windows holds mandatory locks on active file descriptors. Always use `storage.SafeRemoveAll` which resets read-only file attributes and implements exponential backoff retries when deleting session directories.
- **ZIP-Slip Defense**: All archive extractions must check that target output paths start with the canonical destination directory prefix (`strings.HasPrefix(targetPath, cleanDestDir)`).

### Platform & Script Invariants
- **PowerShell Native Tooling**: On Windows PowerShell, native CLI warnings (such as Rollup chunk notices on stderr) can trigger `NativeCommandError` under strict `$ErrorActionPreference = "Stop"`. Always invoke native package managers using `cmd.exe /c "npm.cmd ..."` with scoped error handling and `$LASTEXITCODE` checks.

---

## 3. Workflow for Implementing Changes

Whenever assigned a non-trivial feature, bug fix, or refactor:

1. **Read & Inspect**:
   - Read the root [`README.md`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/README.md) and the closest directory README.
   - Inspect existing types and tests in [`backend/tests/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/tests) and [`frontend/src/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src).
2. **Smallest Reliable Change**:
   - Implement only the logic required for the feature or fix.
   - Do not reformat unrelated files or alter working API endpoints without cause.
3. **Test Immediately**:
   - Run backend tests: `go test -v -count=1 ./tests/...`
   - Run static analysis: `go vet ./...` and `gofmt -l .`
   - Run frontend build: `cmd.exe /c "npm.cmd run build"`
4. **Synchronize Documentation**:
   - Update the relevant folder README files if handlers, props, types, or environment variables changed.

---

## 4. Change-Completion Checklist

Before marking any task as complete, verify every applicable item:

- [ ] **No Guessing**: Code matches actual types and routes in `backend/api/router.go` and `frontend/src/api.js`.
- [ ] **Formatting**: `gofmt -l .` returns 0 unformatted files.
- [ ] **Static Analysis**: `go vet ./...` passes with 0 errors.
- [ ] **Tests Pass**: `go test -v -count=1 ./tests/...` completes with 100% green tests.
- [ ] **Frontend Build**: `npm run build` succeeds without bundle or compile failures.
- [ ] **Automated Runner**: `powershell -ExecutionPolicy Bypass -File test-all.ps1` exits with code 0.
- [ ] **No Secrets**: No API tokens, credentials, or private filepaths committed to source or logged.
- [ ] **Documentation Updated**: Directory READMEs reflect all new or modified functions and API contracts.
