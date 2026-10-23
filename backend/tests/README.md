# Package: `tests`

Located in [`backend/tests/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/tests)

The `tests` package houses the automated test suites for the Veracity Maps Go backend, spanning unit tests, regression suites, concurrency testing, and end-to-end API matrix execution.

---

## Test Files in Package

### 1. `api_full_matrix_test.go`
- **Purpose**: Exhaustive integration matrix verifying all HTTP routes, input validation boundaries, negative test cases, and cross-session isolation.
- **Key Tests**:
  - `TestAPI_FullMatrixCoverage`:
    1. Tests `/health` and `/api/health`.
    2. Tests session creation, current session lookup, and map configuration GET/PUT.
    3. Tests uploads with test fixtures:
       - `ordinary_grid.png` (Valid PNG).
       - `ordinary_photo.jpg` (Valid JPEG).
       - `valid_bundle.zip` (ZIP bundle with sidecars).
       - `zip_slip_malicious.zip` (Negative: rejects path traversal).
       - `truncated.png` (Negative: rejects corrupt headers).
       - `text_as_png.png` (Negative: rejects spoofed ASCII text).
       - `zero_byte.png` (Negative: rejects empty files).
       - `unsupported.xyz` (Negative: rejects disallowed extension).
    4. Tests cross-session isolation: Session 2 cannot see or access Session 1's dataset ID (returns 404).
    5. Tests source image binary streaming.
    6. Tests control point matrix:
       - Rejects out-of-bounds pixel coordinates (`image_x: 99999.0`).
       - Adds 4 valid affine control points.
       - Updates control point.
    7. Tests least-squares affine georeferencing execution.
    8. Tests layer listing and layer visibility/opacity updates.
    9. Tests MTMAP export generation.
    10. Tests dataset deletion and session teardown.
  - `TestAPI_ZipUploadWithSidecars`: Ingests a ZIP bundle and verifies automatic extraction of `.pgw` transform and `.prj` CRS.
  - `TestAPI_ZipSlipRejection`: Verifies rejection of archives containing traversal entries like `../outside.txt`.

### 2. `api_integration_test.go`
- **Purpose**: Tests full user lifecycle from anonymous session initiation, through image upload, control point entry, georeferencing, layer inspection, export, and cleanup.
- **Key Test**: `TestAPI_CompleteWorkflow`.

### 3. `gis_transform_test.go`
- **Purpose**: Unit tests for the pure-Go least-squares mathematical solvers in `backend/gis/transform.go`.
- **Key Tests**:
  - `TestGISTransform_Similarity`: Verifies 2-point similarity transformation solver against known translation and scale.
  - `TestGISTransform_Affine`: Verifies 4-point affine transformation solver.
  - `TestGISTransform_CollinearRejection`: Verifies rejection of collinear control point sets.
  - `TestGISTransform_InsufficientPoints`: Verifies rejection when fewer than 2 points are provided.

### 4. `repository_test.go`
- **Purpose**: Unit tests for the thread-safe `SessionRepository`.
- **Key Test**: `TestSessionRepository_Lifecycle`: Validates session creation, dataset insertion, control point CRUD, concurrent access, and TTL-based background session expiration.

### 5. `multi_dataset_zip_test.go`
- **Purpose**: Tests multi-dataset archive inspection, staged review, sidecar pairing, and selective import.
- **Key Tests**:
  - `TestAPI_MultiDatasetZipInspectionAndImport`: Validates discovery of multiple raster files, companion world files/projections, building and floor suggestions, and selective import execution.
  - `TestAPI_MultiDatasetZipSlipDefense`: Validates security protections against ZIP-Slip archive entries.
  - `TestAPI_MultiFloorInheritanceAndWorkspaceMembers`: Validates workspace manifest enrichment with multi-floor dataset members.

### 6. `multi_floor_propagation_test.go`
- **Purpose**: Tests multi-floor building hierarchy parsing, spatial footprint propagation, and pseudo-building consolidation.
- **Key Tests**:
  - `TestAPI_MultiFloorUploadAllFloorsParsed`: Uploads an archive with multiple building floors (e.g. Building A Basement, Ground, First, Second; Building B Ground) and verifies all floors are cleanly parsed and registered.
  - `TestAPI_SingleGeoreferencePropagatesToEntireBuilding`: Verifies that georeferencing one floor in a building automatically propagates the spatial footprint, bounding box, scaled affine transform, and companion `.pgw` files across all sibling floors.
  - `TestAPI_ConsolidatePseudoBuildingsAndPropagateGeoreference`: Verifies that pseudo-buildings named from floor tokens ("Floor B1", "Floor G", "Floor 1") are consolidated into the parent building ("Building A") with all floors marked `completed` and inheriting georeferencing.

---

## Running the Test Suite

From the `backend/` directory:
```powershell
go test -v -count=1 ./tests/...
```
