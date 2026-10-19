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
- **Purpose**: Defines `ExportRequest`: `{ format: string }`.

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
| `GET` / `POST` | `/datasets/{id}/export` | Generate export package | 200 OK |
| `GET` | `/api/exports/{id}/download` | Stream binary export download | 200 OK |
