# Package: `services`

Located in [`backend/services/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/services)

The `services` package contains the core business logic of Veracity Maps, coordinating between storage managers, session repositories, GIS solvers, and archive packagers.

---

## Files in Package

### 1. `dataset_service.go`
- **Purpose**: Manages raster and archive uploads, disk streaming, SHA-256 integrity calculation, image decoding, ZIP-slip sanitization, and multi-floor building archive ingestion.
- **Key Methods**:
  - `HandleUpload(sessionID string, fileHeader *multipart.FileHeader) (*models.Dataset, error)`: Entry point for uploads.
  - `handleSingleRasterUpload`: Streams file to disk, verifies header via `gis.InspectRasterFile`, auto-creates default map layers, and registers dataset in the session repository.
  - `handleZipUpload`: Securely extracts ZIP files, enforces path containment against canonical directory boundaries, locates all raster floor plans (filtering out macOS resource forks `__MACOSX`, `.DS_Store`), automatically detects building identities and floor levels, creates `models.Building` and `models.BuildingFloor` structures, and attaches all floors to the workspace manifest.
  - `PropagateBuildingGeoreference(sessionID, refDataset)`: Propagates spatial footprint, affine transformations, scaled world files, and layer coordinates across all sibling floors in the same building entity.
  - `ListDatasets`, `GetDataset`, `DeleteDataset`: CRUD delegates interacting with the repository and storage managers.

### 2. `georeference_service.go`
- **Purpose**: Handles control point validation, coordinate range enforcement, raster boundary checks, georeferencing execution, and building-wide spatial propagation.
- **Key Methods**:
  - `CreateControlPoint(sessionID, datasetID, cp)`: Enforces latitude `[-90, 90]`, longitude `[-180, 180]`, finite number validation, and raster dimension bounds (`[0, width)` and `[0, height)`).
  - `UpdateControlPoint(sessionID, pointID, cp)`: Updates coordinates with matching validation.
  - `DeleteControlPoint(sessionID, pointID)`: Removes a control point.
  - `Georeference(sessionID, datasetID, action, transformation)`: Calls `gis.CalculateGeoreference`, generates companion world files (`.pgw`/`.jgw`), outputs a standard GDAL VRT XML, conditionally triggers `gdal_translate` if GDAL is installed, updates layer bounds to MapLibre GL coordinates, marks status as `"completed"`, and automatically propagates bounds and layers to all sibling floors in the building via `PropagateBuildingGeoreference`.
  - `PropagateBuildingGeoreference(sessionID, refDataset)`: Synchronizes building footprint, bounds, and scaled world files to sibling floors.
  - `ConsolidateSessionBuildings(repo, store, sessionID)`: Scans the session for pseudo-buildings created from floor tokens (e.g. "Floor 1", "Floor B1", "Floor G", "Floor 2"), merges their datasets and floors into their real parent building (e.g. "Building A" or "Building 1"), cleans up obsolete pseudo-building records, and automatically triggers building-wide georeference propagation so all sibling floors inherit the spatial footprint and become georeferenced.
  - `IsFloorName(name)`: Pure function classifying whether a name string represents a floor or level indicator rather than a structural building name.

### 3. `export_service.go`
- **Purpose**: Compiles dataset assets into downloadable archives.
- **Key Methods**:
  - `GenerateExport(sessionID, datasetID, format)`:
    - `"mtmap"`: Packages the source raster, georeferenced TIF/VRT, world file, projection, and a `manifest.json` into a compressed `.mtmap` ZIP bundle.
    - `"vrt"`: Returns the GDAL VRT XML descriptor.
    - `"png"` / `"jpg"`: Returns the processed raster image.
    - `"geotiff"`: Returns the georeferenced TIFF.
