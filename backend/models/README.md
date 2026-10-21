# Package: `models`

Located in [`backend/models/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/models)

The `models` package defines all core domain entities, API response wrappers, and session extraction utilities shared across the backend service layers.

---

## Files in Package

### 1. `dataset.go`
- **`type Dataset struct`**:
  - `ID`: Unique dataset identifier (`ds_<32-hex>`).
  - `SessionID`: Associated session owner.
  - `Filename`: Name of uploaded image or bundle.
  - `FileSize`: Size in bytes.
  - `ContentType`: MIME type string.
  - `FileHash`: SHA-256 checksum for integrity verification.
  - `StoragePath`: Absolute local filesystem path to the stored raster.
  - `GeoreferencedPath`: Path to the georeferenced output (TIF or VRT).
  - `ProcessedPath`: Path to processed output.
  - `GeoreferenceStatus`: Current state (`"not_started"`, `"pending"`, `"completed"`).
  - `ExtractedMetadata`: Key-value map containing dimensions (`width`, `height`), `crs`, `transform`, `bounds`.
  - `MapLayer`: Synthesized MapLibre-compatible raster layer metadata (`id`, `name`, `type`, `bounds`, `coordinates`).
- **`type MapProject struct`**: Encapsulates project configurations for multi-layer or building-floor mappings.

### 2. `layer.go`
- **`type Layer struct`**:
  - `ID`: Layer identifier (`lyr_<32-hex>`).
  - `DatasetID`: Associated dataset.
  - `Name`: Display name in the workspace layer panel.
  - `Type`: `"raster"` or `"vector"`.
  - `Visible`: Boolean toggle for MapLibre layer visibility.
  - `Opacity`: Float value between `0.0` (transparent) and `1.0` (opaque).
  - `Bounds`: MapLibre GL bounding box: `[west, south, east, north]`.

### 3. `control_point.go`
- **`type ControlPoint struct`**:
  - `ID`: Control point identifier (`cp_<32-hex>`).
  - `DatasetID`: Target dataset.
  - `ImageX`, `ImageY`: Pixel coordinates on the source raster image.
  - `Longitude`, `Latitude`: Real-world geographic coordinates in WGS84 (EPSG:4326).
  - `CreatedAt`: Creation timestamp.

### 4. `response.go`
- **`type APIResponse struct`**: Standard unified API envelope `{ success: bool, data?: interface{}, error?: *APIError }`.
- **`type APIError struct`**: Detailed error payload `{ code: string, message: string, details?: interface{}, field?: *string }`.
- **`func WriteJSON(w http.ResponseWriter, statusCode int, data interface{})`**: Serializes successful JSON response with HTTP status code.
- **`func WriteError(w http.ResponseWriter, statusCode int, code string, message string, details interface{}, field *string)`**: Serializes structured error responses.
- **`func GetSessionID(r *http.Request) string`**: Extracts the active session ID from the `X-Session-ID` header or `session_id` query parameter.
