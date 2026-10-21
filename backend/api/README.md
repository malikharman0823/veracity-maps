# Package: `api`

Located in [`backend/api/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/api)

The `api` package provides the central HTTP request multiplexer, routing logic, path parameter parsing, and middleware chain for the Veracity Maps Go backend.

---

## Files in Package

### 1. `router.go`
- **Purpose**: Defines the `Router` struct and dispatches incoming HTTP requests to their appropriate functional handlers based on path and method.
- **Key Structs**:
  - `Router`: Holds references to `SessionRepository`, `Middleware`, and functional handlers (`uploadH`, `datasetsH`, `georefH`, `exportH`).
- **Core Functions**:
  - `NewRouter(...) *Router`: Constructs a new router with dependency injection.
  - `BuildHandler() http.Handler`: Wraps the core routing dispatcher with `Session`, `Recovery`, and `CORS` middleware.
  - `route(w http.ResponseWriter, r *http.Request)`: Implements URL path matching:
    - `/health` & `/api/health`: Dispatches to `health.HealthHandler`.
    - `/sessions`: Anonymous session creation (`POST`).
    - `/sessions/current`: Retrieves active session metadata (`GET`).
    - `/sessions/{id}/map-config`: Gets or updates viewport zoom and center (`GET`, `PUT`).
    - `/upload`, `/datasets/import-zip`: Dispatches multipart upload to `uploadH.UploadFile`.
    - `/datasets`: Enumerates active datasets for the caller's session (`GET`).
    - `/datasets/{id}`: Retrieves or deletes a dataset (`GET`, `DELETE`).
    - `/datasets/{id}/source-image`: Streams raw raster image bytes (`GET`).
    - `/datasets/{id}/control-points`: Lists or adds control points (`GET`, `POST`).
    - `/control-points/{id}`: Updates or removes a control point (`PUT`, `DELETE`).
    - `/datasets/{id}/georeference`: Computes or inspects affine georeferencing (`GET`, `POST`).
    - `/datasets/{id}/layers`: Retrieves dataset layers (`GET`).
    - `/datasets/{id}/layers/{layer_id}`: Updates layer opacity or visibility (`PATCH`).
    - `/datasets/{id}/export`: Generates GIS export bundle (`GET`).
    - `/api/exports/{id}/download`: Streams downloadable ZIP/VRT artifact (`GET`).
    - `/datasets/{id}/workspace`: Retrieves the workspace manifest (`GET`).
    - `/datasets/{id}/workspace/members`: Attaches a multi-layer member (`POST`).
    - `/datasets/{id}/workspace/members/{member_id}`: Patches or detaches a member (`PATCH`, `DELETE`).
    - `/datasets/{id}/workspace/state`: Updates workspace view/camera configuration (`PATCH`).
    - `/datasets/{id}/workspace/records`: Returns searchable/paginated database records (`GET`).
    - `/icons`: Lists available map icons (`GET`).
    - `/icons/{name}`: Streams icon image asset (`GET`).
- **Error Handling**: Non-matching routes return HTTP 404 `NOT_FOUND` via `models.WriteError`.

### 2. `middleware.go`
- **Purpose**: Provides cross-cutting concerns: CORS header injection, panic recovery, and session token resolution.
- **Key Structs**:
  - `Middleware`: Holds a reference to the `SessionRepository`.
- **Core Functions**:
  - `CORS(next http.Handler) http.Handler`: Injects `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Methods`, and `Access-Control-Allow-Headers` (including `X-Session-ID`). Intercepts preflight `OPTIONS` requests with HTTP 204.
  - `Recovery(next http.Handler) http.Handler`: Captures runtime panics, logs stack traces, and returns HTTP 500 `INTERNAL_ERROR`.
  - `Session(next http.Handler) http.Handler`: Checks for `X-Session-ID` header or `session_id` query parameter; if absent or expired, creates a fresh session automatically and sets the response header.

---

## Testing & Dependencies

- **Dependencies**: Depends on standard `net/http`, [`models`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/models/README.md), [`repository`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/repository/README.md), and functional handlers in [`functions/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/README.md).
- **Tests**: Covered comprehensively by [`backend/tests/api_full_matrix_test.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/tests/README.md) and `api_integration_test.go`.
