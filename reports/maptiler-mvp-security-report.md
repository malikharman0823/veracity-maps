# MapTiler MVP — Authorized Vulnerability Assessment & Security Report

**Date:** 2026-10-19  
**Scope:** Local Go Backend (`maptiler-go/backend`) & React/Vite Frontend (`maptiler-go/frontend`)  
**Authorization:** Authorized local testing environment  
**Assessment Standard:** OWASP Top 10 API Security Risks & Geospatial Safe Processing Guidelines  

---

## 1. Security Assessment Summary

An authorized, non-destructive security assessment of the MapTiler Go MVP codebase was performed, targeting file uploads, archive extractions, directory traversal, session isolation, input sanitization, sub-process execution, and dependency supply chain vulnerabilities.

| Finding ID | Severity | Title | Status |
| :--- | :--- | :--- | :--- |
| **SEC-01** | High | Archive Extraction Path Traversal (Zip-Slip) | **Mitigated & Verified** |
| **SEC-02** | High | Cross-Session Access Control & Data Isolation | **Verified Isolated** |
| **SEC-03** | High | Path Traversal via Dataset / Session ID | **Mitigated & Verified** |
| **SEC-04** | Medium | Control Point Boundary Injection | **Fixed & Regression Tested** |
| **SEC-05** | Medium | Extension Spoofing & Malformed Image Injection | **Mitigated & Verified** |
| **SEC-06** | Informational | DevDependency Advisory (`braces` in `shadcn` CLI) | **Documented (Non-Reachable)** |

---

## 2. Detailed Vulnerability & Risk Findings

### SEC-01: Archive Extraction Path Traversal (Zip-Slip)
- **Severity:** High (CVSS 7.5)
- **Affected Component:** [`backend/functions/upload/function.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/functions/upload/function.go)
- **Preconditions & Impact:** An attacker uploads a crafted ZIP containing entries with relative traversal sequences (e.g., `../../outside.txt` or absolute Windows drive letters `C:\Windows\temp`). If improperly extracted, arbitrary files on the host filesystem could be overwritten.
- **Safe Reproduction Steps:**
  1. Generate fixture `zip_slip_malicious.zip` with entry `../outside.txt`.
  2. Send multipart `POST /upload` with `X-Session-ID: sess_test`.
- **Evidence:** Backend checks `strings.HasPrefix(targetPath, cleanDestDir)` for every zip entry. The malicious entry triggers an immediate error:
  ```json
  {"success":false,"error":{"code":"ZIP_SLIP_ATTEMPT","message":"Archive entry traverses outside destination directory: ../outside.txt"}}
  ```
- **Mitigation:** Implemented filepath sanitization using `filepath.Clean` and strict directory prefix boundary verification prior to opening file descriptors.
- **Regression Test:** [`TestAPI_ZipSlipRejection`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/tests/api_full_matrix_test.go#L370-L388) (PASS).
- **Retest Result:** Confirmed blocked and verified.

---

### SEC-02: Cross-Session Access Control & Data Isolation
- **Severity:** High (CVSS 7.1)
- **Affected Component:** [`backend/api/router.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/api/router.go), [`backend/repository/session_repository.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/repository/session_repository.go)
- **Preconditions & Impact:** In an anonymous multi-tenant environment without central databases, Session A could potentially inspect, modify, or delete datasets uploaded by Session B if dataset IDs are guessable or not scoped to session tokens.
- **Safe Reproduction Steps:**
  1. Session 1 (`sess_A`) creates a dataset `ds_target`.
  2. Session 2 (`sess_B`) attempts `GET /datasets`, `GET /datasets/ds_target`, and `DELETE /datasets/ds_target` with header `X-Session-ID: sess_B`.
- **Evidence:**
  - `GET /datasets` returns 0 datasets for Session 2.
  - `GET /datasets/ds_target` returns HTTP 404 `DATASET_NOT_FOUND`.
  - `DELETE /datasets/ds_target` returns HTTP 404 `DATASET_NOT_FOUND`.
- **Mitigation:** In-memory session repository enforces two-tier lookup: `r.sessions[sessionID].Datasets[datasetID]`. Unauthenticated or cross-session attempts cannot resolve another session's dataset pointers.
- **Regression Test:** [`TestAPI_FullMatrixCoverage` (Section 4)](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/tests/api_full_matrix_test.go#L235-L255) (PASS).
- **Retest Result:** Verified strict isolation.

---

### SEC-03: Path Traversal via Dataset / Session ID
- **Severity:** High (CVSS 7.5)
- **Affected Component:** [`backend/storage/temporary.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/storage/temporary.go)
- **Preconditions & Impact:** A client passes path traversal tokens in URL parameters such as `/datasets/..%2f..%2f..%2fwindows%2fsystem32/source-image` to read arbitrary host files.
- **Safe Reproduction Steps:**
  1. Send `GET /datasets/..%2f..%2fetc%2fhosts/source-image` with valid session ID.
- **Evidence:** Storage manager normalizes all paths under `filepath.Clean(s.rootDir)` and verifies dataset ID formats (`ds_[a-f0-9]{32}`). Non-conforming or traversal IDs fail with 404.
- **Mitigation:** Strictly bounded temporary directory paths and deterministic alphanumeric ID generation.
- **Regression Test:** Tested in API integration suite.
- **Retest Result:** PASS.

---

### SEC-04: Control Point Boundary Injection
- **Severity:** Medium (CVSS 5.3)
- **Affected Component:** [`backend/services/georeference_service.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/services/georeference_service.go)
- **Preconditions & Impact:** Client sends invalid pixel coordinates (`image_x: 99999.0`) or invalid geographic coordinates (`lat: 250.0`) during `POST /datasets/{id}/control-points`. Previously, these were stored into session state and only failed during later transformation requests.
- **Safe Reproduction Steps:**
  1. Send `POST /datasets/{id}/control-points` with `{"image_x": 99999.0, "image_y": 10.0, "latitude": 40.7, "longitude": -74.0}`.
- **Evidence:** Server now rejects input at entry point:
  ```json
  {"success":false,"error":{"code":"INVALID_CONTROL_POINT","message":"Image coordinates must fall within raster dimensions (400x300)"}}
  ```
- **Mitigation:** Added immediate coordinate and raster dimension validation to `CreateControlPoint` and `UpdateControlPoint`.
- **Regression Test:** Verified in `api_full_matrix_test.go` line 273.
- **Retest Result:** Verified fixed.

---

### SEC-05: Extension Spoofing & Malformed Image Injection
- **Severity:** Medium (CVSS 4.3)
- **Affected Component:** [`backend/gis/inspect.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/gis/inspect.go), [`backend/functions/upload/function.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/functions/upload/function.go)
- **Preconditions & Impact:** User uploads non-image payloads (e.g., text scripts, HTML) disguised with `.png` or `.jpg` extensions.
- **Safe Reproduction Steps:**
  1. Upload `text_as_png.png` containing ASCII plain text.
  2. Upload `zero_byte.png` containing 0 bytes.
  3. Upload `truncated.png` with corrupted headers.
- **Evidence:**
  - `text_as_png.png` rejected with HTTP 400 (`IMAGE_DECODE_FAILED`).
  - `zero_byte.png` rejected with HTTP 400 (`EMPTY_FILE`).
  - `truncated.png` rejected with HTTP 400 (`IMAGE_DECODE_FAILED`).
- **Mitigation:** The upload pipeline does not trust extensions alone; it executes Go's `image.DecodeConfig` to validate binary headers and extract raster dimensions before registering the dataset.
- **Regression Test:** Full coverage in `TestAPI_FullMatrixCoverage` (PASS).
- **Retest Result:** Verified robust.

---

### SEC-06: Frontend DevDependency Advisory (`braces` in `shadcn` CLI)
- **Severity:** Informational (Scanner Warning / GHSA-vfj7-8cjw-p6xm)
- **Affected Component:** `frontend/node_modules/shadcn` (DevDependency)
- **Impact Analysis:**
  - `npm audit` flagged a high-severity advisory on `braces` (used by `micromatch` -> `fast-glob` -> `shadcn` CLI).
  - Reachability: `shadcn` is a development-time scaffolding tool for generating React UI components. It is not bundled or executed in the client-side production runtime bundle.
  - Production bundle inspection confirms `dist/assets/index-Cd442GcI.js` does not contain `braces` or Node.js CLI dependencies.
- **Mitigation Recommendation:** Upgrade `shadcn` when breaking changes are compatible with the current project layout. No immediate production exposure.

---

## 3. Sub-Process & Command Injection Review

All dynamic GIS tool invocations in [`backend/gis/inspect.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/gis/inspect.go) and [`backend/services/georeference_service.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/maptiler-go/backend/services/georeference_service.go) were inspected:
- **Command Structure:** Executed using `exec.Command(binaryPath, arg1, arg2, ...)` as discrete argument slices. No shell expansion (`cmd.exe /c` or `bash -c`) is used.
- **Argument Sanitization:** Filenames and paths are internally generated within the isolated session root. User-supplied parameters (action, transform type) are validated against hardcoded allowlists (`similarity`, `affine`, `auto`).
- **Result:** Immune to command injection attacks.
