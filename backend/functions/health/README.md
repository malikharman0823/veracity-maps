# Package: `functions/health`

Located in [`backend/functions/health/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/health)

The `health` subpackage implements backend heartbeat and GIS dependency inspection endpoints.

---

## File: `function.go`

### Purpose
Handles health probes from monitoring services and frontend startup initialization.

### Public Types & Handlers
- **`type HealthResponse struct`**:
  - `Status string`: `"ok"` when running.
  - `Version string`: Backend version string (e.g. `"1.0.0"`).
  - `Uptime string`: Human-readable duration string since server start (e.g. `"4.837s"`).
  - `Timestamp time.Time`: Current server UTC timestamp.
  - `GIS gis.GDALTools`: Status of external GDAL CLI tools (`Available bool`, `VersionMessage string`).
- **`func HealthHandler(w http.ResponseWriter, r *http.Request)`**:
  - Validates `r.Method == http.MethodGet`.
  - Runs `gis.DetectGDAL()` to inspect system PATH for `gdalinfo`, `gdal_translate`, and `gdalwarp`.
  - Writes HTTP 200 JSON with `HealthResponse`.

---

## Endpoints

- **`GET /health`** & **`GET /api/health`**:
  - Response:
    ```json
    {
      "success": true,
      "data": {
        "status": "ok",
        "version": "1.0.0",
        "uptime": "12.4s",
        "timestamp": "2026-10-19T14:00:00Z",
        "gis": {
          "available": false,
          "version_message": "GDAL command line utilities (gdalinfo, gdal_translate, gdalwarp) were not found in PATH."
        }
      }
    }
    ```
