# Multi-Floor Dataset Parsing, Raster Display & Map Traversal Overlay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix multi-floor dataset parsing from ZIP archives, enable automated georeferencing inheritance across sibling floors of a building, and provide an interactive floating Map Overlay widget for floor traversal.

**Architecture:** Pure Go backend (in-memory `SessionRepository` with disk hydration, regex/token-based multi-floor inference, and footprint propagation across building floors) paired with React/MapLibre GL on the frontend with a glassmorphic floor traversal elevator overlay.

**Tech Stack:** Go 1.23+, React 19, MapLibre GL JS v6, TailwindCSS 4, Vite 7.

## Global Constraints
- Pure Go backend + standard React (Zero external SQL database / ORM / Python dependencies).
- Bounding boxes strictly `[west, south, east, north]`.
- All archive extractions protected against ZIP-slip.

## Review Focus
- Archive entries like `Building_A/Floor_1/plan.png` must infer building as `"Building A"`, not `"Floor 1"`.
- Sibling floors lacking `.pgw` must inherit the building's georeferenced footprint instead of failing to display.
- Archive import review badge must display `"Imported"` in green when `res.status === "imported"`.
- Traversal overlay must cleanly toggle active floor raster layers and scope icon points by floor.
- Traversal buttons (Up/Down) must respect upper/lower floor bounds.

---

### Task 1: Multi-Floor Parsing, Building Inference & Bounds Inheritance

**Files:**
- Modify: `backend/services/dataset_service.go`
- Modify: `frontend/src/components/ZipImportReview.jsx`
- Test: `backend/tests/multi_dataset_zip_test.go`

- [ ] **Step 1: Write unit test for multi-level folder parsing and building inheritance**
  Add test asserting that `Building_A/Floor_1` infers building `"Building A"` and floor level `1`, and that sibling floors without sidecars inherit building bounds.

- [ ] **Step 2: Update `inferBuilding` and `inferFloor` in `backend/services/dataset_service.go`**
  Implement `isFloorFolderToken` to skip floor subdirectories when determining the enclosing building name.
  Expand `inferFloor` with regex and token matching for all floor numbering conventions (`Floor_G`, `Floor_B1`, `Floor_1`, `1F`, `2F`, `B2`, etc.).

- [ ] **Step 3: Implement footprint/bounds inheritance in `ImportArchiveSelected`**
  After extracting datasets, find any georeferenced floor in each building. Propagate its `MapLayer` coordinates and bounds to sibling floors lacking georeferencing, setting their `GeoreferenceStatus` to `completed` and marking `inherited_from`.

- [ ] **Step 4: Update `ZipImportReview.jsx` status badge**
  Check `res.status === "imported" || res.status === "success"` to display green `"Imported"` badge instead of `"Failed"`.

- [ ] **Step 5: Run tests and verify**
  Run: `go test -v -count=1 ./tests/...`

---

### Task 2: Workspace Project Multi-Floor Auto-Population

**Files:**
- Modify: `backend/functions/datasets/workspace.go`
- Test: `backend/tests/api_workspace_test.go`

- [ ] **Step 1: Update `getOrCreateProject` in `workspace.go`**
  When initializing a project for a dataset belonging to a building, query all sibling floors of that building from `repo.ListBuildings(sessionID)` and populate all of them into `project.Configuration["members"]`.

- [ ] **Step 2: Update `buildEnrichedManifest` in `workspace.go`**
  Ensure that when a manifest is generated, any building sibling floor datasets not yet in the project configuration are seamlessly merged into `enrichedMembers`.

- [ ] **Step 3: Run backend workspace tests**
  Run: `go test -v -run TestAPI_WorkspaceAndIcons ./tests/...`

---

### Task 3: Interactive Floor Traversal Overlay on Map

**Files:**
- Create: `frontend/src/components/FloorTraversalOverlay.jsx`
- Modify: `frontend/src/components/WorkspaceMap.jsx`
- Modify: `frontend/src/index.css`

- [ ] **Step 1: Build `FloorTraversalOverlay.jsx` component**
  Create a floating glassmorphic panel supporting:
  - Building switcher dropdown / tabs if multiple buildings exist in workspace.
  - Step Up (▲) and Step Down (▼) buttons for quick traversal.
  - Vertical elevator stack from top floor down to basements with badges (`2`, `1`, `G`, `B1`, etc.) and active state indicators.
  - Active floor info panel with quick raster opacity slider and visibility toggle.
  - Keyboard listener (Up/Down arrow navigation).

- [ ] **Step 2: Integrate `FloorTraversalOverlay` into `WorkspaceMap.jsx`**
  Replace legacy minimal elevator markup with `FloorTraversalOverlay`.
  Hook up `activeFloorLevel`, `onActiveFloorChange`, and opacity controls.

- [ ] **Step 3: Add CSS styling for `FloorTraversalOverlay` in `index.css`**
  Ensure sleek shadows, blurred backdrop, hover states, and smooth transitions.

- [ ] **Step 4: Verify frontend build**
  Run: `cmd.exe /c "npm.cmd run build"`

---

### Task 4: Full System Verification

- [ ] **Step 1: Run complete automated test runner**
  `powershell -ExecutionPolicy Bypass -File test-all.ps1`
- [ ] **Step 2: Restart backend server daemon**
  Kill existing backend process and restart `go run main.go`.
- [ ] **Step 3: Browser smoke test**
  Navigate in Chrome DevTools to the multi-floor workspace, test floor switching on the overlay, and verify raster updates and floor point scoping.
