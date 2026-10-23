# Multi-Floor Top Corner Overlay, Single Building Georeference Inheritance & Parsing Implementation Plan

## Objective
Enable seamless multi-floor building management:
1. Parse all building floor plans from uploaded archives/ZIPs and standalone uploads without errors.
2. Unify georeferencing across the entire building: when one floor is georeferenced, all sibling floors automatically inherit the same building footprint, bounds, and coordinates.
3. Provide an interactive floating glassmorphic Floor Traversal Overlay in the top corner of the map to switch between floors (with vertical elevator stack, Up/Down climbing buttons, keyboard arrow navigation, and raster opacity/visibility controls).
4. Eliminate display and parsing errors across backend and frontend map rendering.

## Current State
- `backend/services/dataset_service.go`:
  - `handleZipDataset` only extracts a single `primaryRaster`, dropping all other floors in multi-floor archives.
  - Archive entries like macOS `__MACOSX/._*.png` are mistakenly processed as rasters, causing raster inspection failures.
- `backend/services/georeference_service.go`:
  - `Georeference()` georeferences only the target dataset, without propagating the footprint/bounds to sibling floors in the same building.
- `backend/functions/datasets/transform.go`:
  - Visual raster transform adjustments do not propagate to sibling floors of the building.
- `backend/functions/datasets/workspace.go`:
  - `ensureDatasetMapLayer()` returns early if a sibling floor is not individually marked completed, even when another floor in the same building has full georeferencing.
- `frontend/src/components/WorkspaceMap.jsx`:
  - `isLayerVisible` evaluates `member.raster_visible`, which defaults to `undefined` (falsy) on uninitialized members, inadvertently hiding the raster.
  - `buildingFloors` filters out floors if `m.is_building_floor` or `m.building_name` is only present in `m.dataset.extracted_metadata`.
  - `map.addLayer` lacks exception guards when adding layers during rapid floor toggles.
- `frontend/src/pages/DatasetPage.jsx`:
  - Has no floor switching overlay when a dataset is part of a building with multiple floors.

## Target State
- Uploading a building ZIP automatically parses all floors, registers the building and building floors, and links them into the workspace.
- Georeferencing any floor in a building (or having one georeferenced floor in an archive) instantly propagates the spatial footprint, bounding box, world files, and `map_layer` coordinates to all sibling floors of the building.
- A glassmorphic Floor Traversal Overlay is positioned in the top corner of the map (with building switcher, Up/Down climb buttons, vertical floor stack, and raster controls) allowing instantaneous switching with zero errors.
- Display and parsing pipelines are rock-solid, passing all backend unit tests, static analysis (`go vet`), and frontend production build (`npm run build`).

## Files to Create / Modify
- Modify `backend/services/dataset_service.go`:
  - Filter out `__MACOSX`, `._*`, `.ds_store`.
  - Update `handleZipDataset` to extract and register ALL rasters in multi-dataset/multi-floor archives.
  - Infer building and floor levels/labels for all rasters and create `models.Building` / `models.BuildingFloor`.
  - Add helper `PropagateBuildingGeoreference` to propagate footprints to sibling floors.
  - Populate `imported_datasets` so frontend receives all floor records.
- Modify `backend/services/georeference_service.go`:
  - In `Georeference()`, call building propagation so georeferencing one floor georeferences the entire building.
- Modify `backend/functions/datasets/transform.go`:
  - In `applyCornersToDataset()`, propagate transformed coordinates and bounds to sibling floors in the building.
- Modify `backend/functions/datasets/workspace.go`:
  - In `ensureDatasetMapLayer()` and `buildEnrichedManifest()`, inherit bounds/layer from any georeferenced sibling floor in the building.
- Modify `frontend/src/components/FloorTraversalOverlay.jsx`:
  - Enhance building map resolution to inspect metadata and `buildings` list.
  - Position overlay cleanly in the top corner with smooth transition and accessibility.
- Modify `frontend/src/components/WorkspaceMap.jsx`:
  - Fix `isLayerVisible` with safe boolean checks (`member.raster_visible !== false`).
  - Safeguard `map.addLayer` and image source additions with error handlers.
  - Allow floor switching without errors.
- Modify `frontend/src/pages/DatasetPage.jsx`:
  - Add floor switcher overlay support when viewing a dataset with sibling building floors.
- Create / Update tests:
  - `backend/tests/multi_floor_propagation_test.go`: Test single georeference propagation to entire building and multi-floor zip parsing.

## Verification Checklist
- [ ] Backend tests pass: `go test -v -count=1 ./tests/...`
- [ ] Static analysis clean: `go vet ./...`
- [ ] Formatting clean: `gofmt -l .`
- [ ] Frontend builds without error: `npm run build`
