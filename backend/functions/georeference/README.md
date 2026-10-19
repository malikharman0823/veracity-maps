# Package: `functions/georeference`

Located in [`backend/functions/georeference/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/georeference)

The `georeference` subpackage exposes endpoints for ground control point management, spatial validation, and least-squares affine transformation computation.

---

## Files in Package

### 1. `function.go`
- **Purpose**: Defines `Handler` with endpoints for managing control points and triggering raster georeferencing.
- **Key Methods**:
  - `ListControlPoints(w, r, datasetID)`: Returns the list of control points associated with the dataset (`GET /datasets/{id}/control-points`).
  - `CreateControlPoint(w, r, datasetID)`: Validates coordinates against the raster dimensions and geographic limits before creating a new control point (`POST /datasets/{id}/control-points`).
  - `UpdateControlPoint(w, r, pointID)`: Updates coordinates for an existing control point (`PUT /control-points/{id}`).
  - `DeleteControlPoint(w, r, pointID)`: Removes a control point (`DELETE /control-points/{id}`).
  - `Georeference(w, r, datasetID)`: Dispatches to `GetConfig` on GET or executes least-squares calculation and world file/VRT generation on POST (`POST /datasets/{id}/georeference`).
  - `GetConfig(w, r, datasetID)`: Returns current georeference metadata, CRS, and bounds (`GET /datasets/{id}/georeference`).

### 2. `request.go`
- **Purpose**: Defines request payloads:
  - `CreateControlPointRequest`: `{ image_x: float64, image_y: float64, longitude: float64, latitude: float64 }`
  - `GeoreferenceRequest`: `{ action: string, transformation?: string }`

### 3. `response.go`
- **Purpose**: Defines response payloads:
  - `ControlPointListResponse`: `{ points: []*models.ControlPoint }`
  - `GeoreferenceResponse`: Transformation coefficients, parameters, and MapLibre GL bounds.
  - `GeoreferenceConfigResponse`: Stored transform configuration.

---

## Endpoints

| Method | Path | Description | Success Code |
| :--- | :--- | :--- | :--- |
| `GET` | `/datasets/{id}/control-points` | List control points | 200 OK |
| `POST` | `/datasets/{id}/control-points` | Create & validate control point | 201 Created |
| `PUT` | `/control-points/{id}` | Update control point | 200 OK |
| `DELETE` | `/control-points/{id}` | Delete control point | 200 OK |
| `GET` | `/datasets/{id}/georeference` | Inspect current transform config | 200 OK |
| `POST` | `/datasets/{id}/georeference` | Calculate least-squares transform | 200 OK |
