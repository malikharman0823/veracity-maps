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

### 4. `workspace.go`
- **Purpose**: Implements workspace project state and record manifest endpoints (`GET/PATCH /datasets/{id}/workspace`, `GET /datasets/{id}/workspace/records`, `GET /icons`, `GET /icons/{name}`).
- **Key Methods**:
  - `GetWorkspace(w, r, datasetID)`: Returns the current map project, camera position, and enriched member datasets with synthesized `map_layer`.
  - `UpdateWorkspaceState(w, r, datasetID)`: Patches active view or camera position.
  - `UpdateMember(w, r, datasetID, memberID)`: Patches individual member visibility or opacity.
  - `ListIcons(w, r)`: Lists available map icons.
  - `ServeIcon(w, r, name)`: Streams built-in SVG icons.

### 5. `transparency.go`
- **Purpose**: Background color removal, live preview generation, and reset capabilities (`GET/POST /datasets/{id}/background-removal`).
- **Key Methods**:
  - `GetBackgroundRemoval(w, r, datasetID)`: Returns background removal status and configuration.
  - `HandleBackgroundRemoval(w, r, datasetID)`: Executes `preview`, `apply`, `reset`, or `status` actions.

### 6. `transform.go`
- **Purpose**: Interactive raster transformation, affine fitting, history management, and ground control point synchronization (`GET/POST /datasets/{id}/transform`).
- **Key Methods**:
  - `GetRasterTransform(w, r, datasetID)`: Returns current transformation corners, bounds, dimensions, and undo/redo capabilities.
  - `HandleRasterTransform(w, r, datasetID)`: Executes `preview`, `apply`, `undo`, `redo`, `reset`, or `status` actions.

---

## Endpoints

| Method | Path | Description | Success Code |
| :--- | :--- | :--- | :--- |
| `GET` | `/datasets` | Enumerate session datasets | 200 OK |
| `GET` | `/datasets/{id}` | Inspect dataset metadata | 200 OK |
| `DELETE` | `/datasets/{id}` | Delete dataset & cleanup files | 200 OK |
| `GET` | `/datasets/{id}/source-image` | Raw binary image stream (serves processed transparent PNG if active) | 200 OK |
| `GET` | `/datasets/{id}/layers` | Enumerate map layers | 200 OK |
| `PATCH` | `/datasets/{id}/layers/{layer_id}`| Update layer visibility/opacity | 200 OK |
| `GET` | `/datasets/{id}/background-removal`| Inspect background transparency status | 200 OK |
| `POST` | `/datasets/{id}/background-removal`| Preview, apply, or reset background removal | 200 OK |
| `GET` | `/datasets/{id}/transform` | Inspect raster transformation footprint & undo state | 200 OK |
| `POST` | `/datasets/{id}/transform` | Preview, apply, undo, redo, or reset raster transformation | 200 OK |
| `GET` | `/datasets/{id}/workspace` | Workspace project manifest | 200 OK |
| `PATCH` | `/datasets/{id}/workspace/state` | Update project active view / camera | 200 OK |
| `GET` | `/datasets/{id}/workspace/records`| Control and icon points records | 200 OK |
| `GET` | `/icons` | Available SVG icons list | 200 OK |
| `GET` | `/icons/{name}` | Stream SVG icon | 200 OK |

