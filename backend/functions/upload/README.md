# Package: `functions/upload`

Located in [`backend/functions/upload/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/upload)

The `upload` subpackage handles multipart file uploads, valid file type inspection, ZIP archive ingestion, and sidecar georeferencing discovery.

---

## Files in Package

### 1. `function.go`
- **Purpose**: Exposes `Handler.UploadFile` to process multipart form requests containing single image files (PNG, JPEG) or multi-file ZIP bundles.
- **Key Structs & Methods**:
  - `Handler`: Holds dependencies on `services.DatasetService` and `repository.SessionRepository`.
  - `NewHandler(ds *services.DatasetService, repo *repository.SessionRepository) *Handler`: Factory constructor.
  - `UploadFile(w http.ResponseWriter, r *http.Request)`:
    - Verifies HTTP `POST`.
    - Resolves or creates an anonymous session.
    - Parses multipart streams (buffer limit: 32 MB in RAM, remainder spilled to disk).
    - Extracts `file` header and delegates processing to `datasetService.HandleUpload(sessionID, header)`.
    - Formats and writes HTTP 201 JSON `UploadData` response.
- **Error Handling**: Missing `file` field returns HTTP 400 `MISSING_FILE`. Invalid multipart boundaries return HTTP 400 `INVALID_MULTIPART_REQUEST`. File processing errors (unsupported extensions, empty files, ZIP-slip attacks) return HTTP 400 with specific error codes.

### 2. `request.go`
- **Purpose**: Defines `UploadRequest` struct for session association.

### 3. `response.go`
- **Purpose**: Defines `UploadData` structure returned inside standard `models.APIResponse`:
  - `id`: Unique dataset identifier (`ds_<32-hex>`).
  - `filename`: Original uploaded filename.
  - `file_size`: Size in bytes.
  - `content_type`: Detected MIME type (`image/png`, `image/jpeg`, etc.).
  - `is_georeferenced`: Boolean indicating whether world files or CRS metadata were located.
  - `georeference_status`: Initial status (`not_started` or `completed`).
  - `metadata`: Dimensions, format, and spatial parameters.

---

## Endpoint Contract

- **`POST /upload`** & **`POST /datasets/import-zip`**:
  - Request: Multipart Form (`Content-Type: multipart/form-data`) with field `file=@raster.png`.
  - Headers: Optional `X-Session-ID: <session_id>`.
  - Success Response: HTTP 201 Created.
