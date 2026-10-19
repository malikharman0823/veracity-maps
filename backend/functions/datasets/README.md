# Package: `functions/datasets`

Located in [`backend/functions/datasets/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/datasets)

The `datasets` subpackage implements handlers for dataset cataloging, individual dataset inspection, image streaming, deletion, and raster layer management.

---

## Files in Package

### 1. `function.go`
- **Purpose**: Defines `Handler` with endpoints covering dataset lifecycle and layer styling.
- **Key Methods**:
  - `List(w, r)`: Returns all datasets registered under the caller's active session (`GET /datasets`).
  - `Get(w, r, datasetID)`: Returns metadata for a specific dataset (`GET /datasets/{id}`). Returns 404 if not found or if attempting cross-session access.
  - `Delete(w, r, datasetID)`: Deletes dataset files and removes registration from session repository (`DELETE /datasets/{id}`).
  - `SourceImage(w, r, datasetID)`: Streams raw raster image bytes directly via `http.ServeFile` with appropriate MIME headers (`GET /datasets/{id}/source-image`).
  - `ListLayers(w, r, datasetID)`: Lists MapLibre-compatible layers for the dataset (`GET /datasets/{id}/layers`).
  - `UpdateLayer(w, r, datasetID, layerID)`: Updates layer visibility or opacity (`PATCH /datasets/{id}/layers/{layer_id}`).

### 2. `request.go`
- **Purpose**: Defines request bodies:
  - `CreateDatasetRequest`: `{ name: string, format: string }`
  - `UpdateLayerRequest`: `{ visible?: *bool, opacity?: *float64 }`

### 3. `response.go`
- **Purpose**: Defines response structures:
  - `CreateDatasetResponse`
  - `DatasetListResponse`: `{ datasets: []*models.Dataset }`
  - `DeleteDatasetResponse`: `{ id: string, deleted: bool }`
  - `LayersResponse`: `{ layers: []*models.Layer }`

---

## Endpoints

| Method | Path | Description | Success Code |
| :--- | :--- | :--- | :--- |
| `GET` | `/datasets` | Enumerate session datasets | 200 OK |
| `GET` | `/datasets/{id}` | Inspect dataset metadata | 200 OK |
| `DELETE` | `/datasets/{id}` | Delete dataset & cleanup files | 200 OK |
| `GET` | `/datasets/{id}/source-image` | Raw binary image stream | 200 OK |
| `GET` | `/datasets/{id}/layers` | Enumerate map layers | 200 OK |
| `PATCH` | `/datasets/{id}/layers/{layer_id}`| Update layer visibility/opacity | 200 OK |
