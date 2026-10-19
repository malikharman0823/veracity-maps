# Package: `functions`

Located in [`backend/functions/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions)

The `functions` package groups HTTP handlers by business resource. Each subpackage contains the request schemas, response models, and HTTP handler functions required to process API calls, enforce input validation, and interact with backend services.

---

## Subpackages

| Subpackage | Endpoints Handled | Description |
| :--- | :--- | :--- |
| [`health/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/health/README.md) | `GET /health`, `GET /api/health` | Service uptime, version, and dynamic GDAL status check. |
| [`upload/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/upload/README.md) | `POST /upload`, `POST /datasets/import-zip` | Multipart upload, ZIP extraction, sidecar discovery, and ZIP-slip defense. |
| [`datasets/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/datasets/README.md) | `GET /datasets`, `GET /datasets/{id}`, `DELETE /datasets/{id}`, `GET /datasets/{id}/source-image`, `GET /datasets/{id}/layers`, `PATCH /datasets/{id}/layers/{layer_id}` | Dataset listing, inspection, deletion, raw raster streaming, and layer styling. |
| [`georeference/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/georeference/README.md) | `GET /datasets/{id}/control-points`, `POST /datasets/{id}/control-points`, `PUT /control-points/{id}`, `DELETE /control-points/{id}`, `GET /datasets/{id}/georeference`, `POST /datasets/{id}/georeference` | Ground control point CRUD, boundary validation, and least-squares affine transformation execution. |
| [`export/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/functions/export/README.md) | `GET /datasets/{id}/export`, `GET /api/exports/{id}/download` | Export generation (MTMAP project packages, GDAL VRT, world files) and file streaming downloads. |

---

## Architectural Conventions

1. **Clean Route Layer**: Handlers do not directly execute filesystem mutations or math calculations; they parse requests, validate parameters, invoke the appropriate service in [`services/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/services/README.md), and return structured responses via `models.WriteJSON` or `models.WriteError`.
2. **Session Scoping**: All handlers resolve session context using `models.GetSessionID(r)` from the `X-Session-ID` header or query string.
