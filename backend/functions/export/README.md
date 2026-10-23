# Package: `functions/export`

Located in [`backend/functions/export/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/export)

The `export` subpackage provides endpoints for compiling, packaging, and downloading georeferenced raster datasets and MTMAP project bundles.

---

## Files in Package

### 1. `function.go`
- **Purpose**: Implements `Handler` with endpoints for generating export artifacts and serving file downloads.
- **Key Methods**:
  - `Export(w, r, datasetID)`: Accepts GET or POST with format selection (`mtmap`, `vrt`, `png`, `jpg`, `geotiff`). Generates the package on disk, registers the export with the session repository, and returns download URL metadata (`GET/POST /datasets/{id}/export`).
  - `Download(w, r, exportID)`: Streams the generated export artifact to the client with `Content-Disposition: attachment; filename="..."` header (`GET /api/exports/{id}/download`).

### 2. `request.go`
- **Purpose**: Defines `ExportRequest` and `ExportOptions`:
  - `format`: Export format (`mtmap`, `vrt`, `png`, `jpeg`, `geotiff`).
  - `project_name`: Name for the exported project archive.
  - `building_ids`: Explicit list of building IDs to export.
  - `floor_ids`: Explicit list of floor IDs to export.
  - `layer_ids`: Explicit list of raster layer IDs to export.
  - `options`:
    - `include_building_configuration`: Include building and floor configuration.
    - `include_control_points`: Include shared control points and georeferencing.
    - `include_icons`: Include icons and map points.
    - `include_layer_styles`: Include layer styles, background removal, and opacity.
    - `include_original_rasters`: Include original uploaded raster images.
    - `include_processed_rasters`: Include processed transparent and transformed rasters.
    - `include_base_map_settings`: Include base map configurations.
    - `include_project_metadata`: Include project metadata (name, description, timestamp).

### 3. `response.go`
- **Purpose**: Defines `ExportResponse`:
  - `export_id`: Generated export tracking ID.
  - `filename`: Download filename.
  - `format`: Export format (`mtmap`, `vrt`, etc.).
  - `download_url`: Path to download endpoint.

---

## Endpoints

| Method | Path | Description | Success Code |
| :--- | :--- | :--- | :--- |
| `GET` / `POST` | `/datasets/{id}/export` | Generate selective or full export package | 200 OK |
| `GET` | `/api/exports/{id}/download` | Stream binary export download | 200 OK |

