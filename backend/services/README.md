# Package: `services`

Located in [`backend/services/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/services)

The `services` package contains the core business logic of Veracity Maps, coordinating between storage managers, session repositories, GIS solvers, and archive packagers.

---

## Files in Package

### 1. `dataset_service.go`
- **Purpose**: Manages raster and archive uploads, disk streaming, SHA-256 integrity calculation, image decoding, and ZIP-slip sanitization.
- **Key Methods**:
  - `HandleUpload(sessionID string, fileHeader *multipart.FileHeader) (*models.Dataset, error)`: Entry point for uploads.
  - `handleSingleRasterUpload`: Streams file to disk, verifies header via `gis.InspectRasterFile`, auto-creates default map layers, and registers dataset in the session repository.
  - `handleZipUpload`: Securely extracts ZIP files, enforces path containment against canonical directory boundaries, locates primary raster images and companion sidecars (`.pgw`, `.prj`), and records georeferenced status.
  - `ListDatasets`, `GetDataset`, `DeleteDataset`: CRUD delegates interacting with the repository and storage managers.

### 2. `georeference_service.go`
- **Purpose**: Handles control point validation, coordinate range enforcement, raster boundary checks, and georeferencing execution.
- **Key Methods**:
  - `CreateControlPoint(sessionID, datasetID, cp)`: Enforces latitude `[-90, 90]`, longitude `[-180, 180]`, finite number validation, and raster dimension bounds (`[0, width)` and `[0, height)`).
  - `UpdateControlPoint(sessionID, pointID, cp)`: Updates coordinates with matching validation.
  - `DeleteControlPoint(sessionID, pointID)`: Removes a control point.
  - `Georeference(sessionID, datasetID, action, transformation)`: Calls `gis.CalculateGeoreference`, generates companion world files (`.pgw`/`.jgw`), outputs a standard GDAL VRT XML, conditionally triggers `gdal_translate` if GDAL is installed, updates layer bounds to MapLibre GL coordinates, and marks status as `"completed"`.

### 3. `export_service.go`
- **Purpose**: Compiles dataset assets into downloadable archives.
- **Key Methods**:
  - `GenerateExport(sessionID, datasetID, format)`:
    - `"mtmap"`: Packages the source raster, georeferenced TIF/VRT, world file, projection, and a `manifest.json` into a compressed `.mtmap` ZIP bundle.
    - `"vrt"`: Returns the GDAL VRT XML descriptor.
    - `"png"` / `"jpg"`: Returns the processed raster image.
    - `"geotiff"`: Returns the georeferenced TIFF.
