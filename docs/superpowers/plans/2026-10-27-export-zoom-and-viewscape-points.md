# Export Zoom Levels and Viewscape Point Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide configurable zoom levels in the export modal, bundle custom icon assets and control point annotations in the export ZIP, render interactive MapLibre GL symbol/marker layers for icon points and GCPs with visibility toggles in the standalone Viewscape viewer, and guarantee roundtrip import/export compatibility.

**Architecture:** Extend `backend/functions/export/` and `backend/services/export_*.go` to process `ZoomConfiguration` parameters, bundle referenced icon assets into `assets/icons/`, and inject GeoJSON features for control points and icon points into the standalone MapLibre GL JS viewer (`viewer.html` and `index.html`) with separate toggleable layers and popups. Update `ExportProjectModal.jsx` to render a zoom-level configuration panel with suggested level guide presets and validation. Ensure `DatasetService.handleZipDataset` safely imports and preserves camera zoom settings and point annotations.

**Tech Stack:** Go 1.22+ standard library (`net/http`, `archive/zip`, `encoding/json`, `image/png`), React 19, MapLibre GL JS, Vite.

## Global Constraints

- Backend must remain zero external database (in-memory repository with file storage).
- Use MapLibre GL JS exclusively for exported web viewer; no Leaflet or external runtime services.
- MapLibre coordinate order must strictly remain `[longitude, latitude]` (`[west, south, east, north]`).
- All archive entries must use safe relative paths with ZIP-Slip defense (`strings.HasPrefix(cleanDest, baseDir)`).
- Windows file lock handling: all deletions must use `storage.SafeRemoveAll`.
- All tests must pass: `go test -v -count=1 ./tests/...` and `powershell -ExecutionPolicy Bypass -File test-all.ps1`.

## Review Focus

1. **Overzooming Beyond Raster Range:** User configures camera max zoom 22 while raster tiles only exist up to zoom 16. MapLibre must overzoom without requesting missing tiles as 404 spam or breaking the viewport.
2. **Missing Icon Asset Fallback:** Referenced icon file missing or corrupted in ZIP. Viewscape must fall back to a high-contrast canvas SVG pin without crashing MapLibre.
3. **Invalid Zoom Range Normalization:** Client passes `min_zoom > max_zoom` or `initial_zoom < min_zoom`. Backend must normalize safely rather than generating an unopenable MapLibre instance.
4. **Coordinate Integrity:** Control points raster X/Y vs geographic long/lat must never be swapped or confused. Markers must render at geographic coordinates and remain stationary relative to the ground.
5. **Roundtrip Re-export Camera & Point Preservation:** Re-opening an exported ZIP and immediately re-exporting must retain the initial zoom, zoom range, preset, GCPs, and icon points.

---

### Task 1: Backend Export Models & Zoom Configuration Request

**Files:**
- Modify: `backend/functions/export/request.go:1-22`
- Modify: `backend/functions/export/function.go:60-80`
- Modify: `backend/services/export_selective.go:18-51`
- Test: `backend/tests/export_zoom_points_test.go`

**Interfaces:**
- `ZoomConfiguration`: struct with `InitialZoom *int`, `MinZoom *int`, `MaxZoom *int`, `Preset string`.
- `SelectiveExportOptions`: include `ZoomConfig *ZoomConfiguration`.
- `ExportRequest`: include `ZoomConfig *ZoomConfiguration` directly or via `Options.ZoomConfig`.

- [ ] **Step 1: Write failing test in `backend/tests/export_zoom_points_test.go`**
  Verify that `ExportRequest` decodes zoom parameters (`initial_zoom`, `min_zoom`, `max_zoom`, `preset`) and validates range constraints.
- [ ] **Step 2: Run test to verify failure**
  Run: `go test -v -run TestExport_ZoomConfigurationDecoding ./tests/...`
  Expected: FAIL with unknown field or compilation error.
- [ ] **Step 3: Implement `ZoomConfiguration` in `backend/functions/export/request.go` & `export_selective.go`**
  Define `ZoomConfiguration` struct and attach to `ExportOptions`, `ExportRequest`, and `SelectiveExportOptions`.
- [ ] **Step 4: Run test to verify it passes**
  Run: `go test -v -run TestExport_ZoomConfigurationDecoding ./tests/...`
  Expected: PASS.
- [ ] **Step 5: Commit changes**
  `git add backend/functions/export backend/services/export_selective.go backend/tests/export_zoom_points_test.go`
  `git commit -m "feat(export): add ZoomConfiguration model to export requests"`

---

### Task 2: Backend ZIP Asset Bundling (Custom Icons & Control Points)

**Files:**
- Modify: `backend/services/export_service.go`
- Modify: `backend/services/export_selective.go`
- Test: `backend/tests/export_zoom_points_test.go`

**Interfaces:**
- `bundleReferencedIcons(archive *zip.Writer, iconPoints []*models.IconPoint, sessionID string) map[string]string`: Copies referenced icon image files into `assets/icons/<clean_name>` in the ZIP and returns a map of `icon_id -> "assets/icons/<clean_name>"`.
- Update `metadata.json`, `mapstudio.json`, `manifest.json`, and `project.json` to store `initial_zoom`, `minzoom`, `maxzoom`, `zoom_preset`, `control_points`, `icon_points`, and point visibility settings.

- [ ] **Step 1: Write failing test for icon bundling and metadata in `backend/tests/export_zoom_points_test.go`**
  Verify that when `IncludeIcons` is enabled and an icon point references an icon, the asset is bundled in `assets/icons/` and registered in `metadata.json` and `mapstudio.json`.
- [ ] **Step 2: Run test to verify failure**
  Run: `go test -v -run TestExport_IconAssetBundling ./tests/...`
  Expected: FAIL.
- [ ] **Step 3: Implement `bundleReferencedIcons` and update ZIP writers**
  Add icon bundling helper in `export_service.go` / `export_selective.go` that finds icons in `frontend/public/icons` or session folders, copies them safely into `assets/icons/` without ZIP traversal, and writes `metadata.json` and `mapstudio.json`.
- [ ] **Step 4: Run test to verify it passes**
  Run: `go test -v -run TestExport_IconAssetBundling ./tests/...`
  Expected: PASS.
- [ ] **Step 5: Commit changes**
  `git add backend/services/ backend/tests/`
  `git commit -m "feat(export): bundle custom icon assets and record point annotations in export package"`

---

### Task 3: MapLibre Viewscape Standalone Viewer (Camera, Layers, & Toggles)

**Files:**
- Modify: `backend/services/export_service.go:520-830`
- Modify: `backend/services/export_selective.go:640-660`
- Test: `backend/tests/export_zoom_points_test.go`

**Interfaces:**
- `generateMapLibreViewerHTML(params ViewscapeViewerParams) string`:
  - `Title string`
  - `Bounds []float64`
  - `Center []float64`
  - `InitialZoom int`
  - `MinZoom int`
  - `MaxZoom int`
  - `RasterMinZoom int`
  - `RasterMaxZoom int`
  - `Opacity float64`
  - `ZoomPreset string`
  - `ControlPoints []*models.ControlPoint`
  - `IconPoints []*models.IconPoint`
  - `IconAssetMap map[string]string`
  - `StyleJSON string`
- Interactive MapLibre Viewer Features:
  - Centers on saved center or dataset bounds center at `InitialZoom`.
  - Camera zoom restricted to `[MinZoom, MaxZoom]`.
  - Raster source restricted to `[RasterMinZoom, RasterMaxZoom]` with overzooming enabled.
  - Separate `control-points-layer` with green target badges, text labels, and popup on click.
  - Separate `icon-points-layer` with bundled icon images (plus dynamic canvas fallback for missing icons), text labels, and popup on click.
  - Floating panel checkboxes:
    - Overlay Layer Visible
    - Icon Points Visible
    - Control Points Visible
  - Opacity slider, Zoom indicator (`map.getZoom().toFixed(1)`), Fit Dataset button, Coordinates readout.

- [ ] **Step 1: Write failing test for Viewscape HTML generation in `backend/tests/export_zoom_points_test.go`**
  Verify generated `viewer.html` contains:
  - Initial zoom and camera limits.
  - Separate GeoJSON sources for icon points and control points.
  - Visibility toggles for icon and control points.
  - MapLibre style with valid layers.
- [ ] **Step 2: Run test to verify failure**
  Run: `go test -v -run TestExport_ViewscapeViewerGeneration ./tests/...`
  Expected: FAIL.
- [ ] **Step 3: Implement enhanced `generateMapLibreViewerHTML` and `generateViewers`**
  Generate self-contained MapLibre HTML viewer embedding GeoJSON data, icon registration, popups, and floating control panel.
- [ ] **Step 4: Run test to verify it passes**
  Run: `go test -v -run TestExport_ViewscapeViewerGeneration ./tests/...`
  Expected: PASS.
- [ ] **Step 5: Commit changes**
  `git add backend/services/ backend/tests/`
  `git commit -m "feat(export): generate standalone Viewscape viewer with point layers and zoom controls"`

---

### Task 4: Import & Roundtrip Compatibility

**Files:**
- Modify: `backend/services/dataset_service.go:550-650`
- Test: `backend/tests/export_zoom_points_test.go`

**Interfaces:**
- In `handleZipDataset`, parse `initial_zoom`, `minzoom` / `min_zoom`, `maxzoom` / `max_zoom`, and `zoom_preset` from `metadata.json` and `mapstudio.json`.
- Safely normalize zoom values and persist in `dataset.ExtractedMetadata`.
- Verify imported project can be re-exported with identical camera zoom settings and point annotations.

- [ ] **Step 1: Write failing test for roundtrip re-export in `backend/tests/export_zoom_points_test.go`**
  Upload sample dataset with control points and icon points, export with custom zoom (initial: 18, min: 14, max: 21, preset: "neighborhood"), re-import the ZIP, and verify metadata and point preservation.
- [ ] **Step 2: Run test to verify failure**
  Run: `go test -v -run TestExport_RoundtripZoomAndPoints ./tests/...`
  Expected: FAIL.
- [ ] **Step 3: Update `handleZipDataset` in `backend/services/dataset_service.go`**
  Extract zoom settings from `metadata.json` / `mapstudio.json` into `ExtractedMetadata` with normalization fallbacks.
- [ ] **Step 4: Run test to verify it passes**
  Run: `go test -v -run TestExport_RoundtripZoomAndPoints ./tests/...`
  Expected: PASS.
- [ ] **Step 5: Commit changes**
  `git add backend/services/dataset_service.go backend/tests/`
  `git commit -m "feat(import): restore zoom settings and point annotations on archive import"`

---

### Task 5: Frontend Export Modal Zoom Configuration UI

**Files:**
- Modify: `frontend/src/components/ExportProjectModal.jsx`
- Test: `frontend/src/components/ExportProjectModal.test.jsx` (or frontend build verification)

**Interfaces:**
- Add "Viewscape Zoom Configuration" section in `ExportProjectModal.jsx`:
  - Suggested zoom guide table / reference (Zoom levels 1 to 21 with labels: House level, Building level, Street level, Neighborhood level, Local area, District level, City level, Large city area, Regional level, Wide regional view, State-level overview, Large regional overview, Country-level overview, World overview).
  - Preset dropdown: `Auto`, `House (21)`, `Building (20)`, `Street (19)`, `Neighborhood (18)`, `Local area (17)`, `District (16)`, `City (15)`, `Large city area (14)`, `Regional (12)`, `State (10)`, `Custom`.
  - Initial Zoom slider & number input.
  - Min Zoom and Max Zoom number inputs.
  - Real-time validation badge: ensures `minZoom <= initialZoom <= maxZoom`.
  - Reset button restoring auto/sensible view.
  - Pass `zoom_config: { initial_zoom, min_zoom, max_zoom, preset }` inside `options` to `onExport(...)`.

- [ ] **Step 1: Update `ExportProjectModal.jsx` with zoom configuration controls**
- [ ] **Step 2: Verify frontend compilation and tests**
  Run: `cmd.exe /c "npm.cmd run build"` in `frontend/`.
  Expected: SUCCESS.
- [ ] **Step 3: Commit changes**
  `git add frontend/src/components/ExportProjectModal.jsx`
  `git commit -m "feat(frontend): add Viewscape Zoom Configuration controls to export modal"`

---

### Task 6: Full Repository Testing & Regression Verification

**Files:**
- All changed files
- Runner: `powershell -ExecutionPolicy Bypass -File test-all.ps1`

- [ ] **Step 1: Run complete Go test suite**
  `go test -v -count=1 ./tests/...`
- [ ] **Step 2: Run Go static analysis and formatting**
  `gofmt -l .` and `go vet ./...`
- [ ] **Step 3: Run Frontend Production Build**
  `cmd.exe /c "npm.cmd run build"`
- [ ] **Step 4: Run repository automated runner**
  `powershell -ExecutionPolicy Bypass -File test-all.ps1`
- [ ] **Step 5: Commit all remaining verified changes**
