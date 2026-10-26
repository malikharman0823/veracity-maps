# Implementation Plan: PDF Multi-Page Import & CAD Import

## Objective
Extend Veracity Maps to support PDF multi-page document inspection/selective raster import and CAD (.dxf) vector-to-raster conversion, maintaining 100% backward compatibility with PNG/JPEG/ZIP, multi-floor building management, shared georeferencing, and project ZIP exports.

---

## 1. Environment & Capability Verification
- **PDF Engine**: Verified PyMuPDF (`pymupdf`) and `pypdf` on Python 3.14. Fast C-based rendering to high-res PNG, page dimension extraction, encryption detection, and rotation handling.
- **CAD Engine**: Verified `ezdxf` with `matplotlib` addon. Parses DXF (ASCII & binary), computes bounding box extents, detects layers/linework, and renders crisp raster PNGs on white/dark background.
- **Bridge Architecture**: Isolated Go service calls Python sub-process scripts (`backend/scripts/pdf_tool.py` and `backend/scripts/cad_tool.py`) with strict timeout (30s), JSON stdout/stderr protocol, path traversal validation, and temp directory containment.

---

## 2. Architecture & Data Flow

```
PDF/CAD Upload (/upload)
    │
    ├─► If .pdf:
    │     1. Store original PDF in session storage: temp/sessions/<sid>/raw_docs/<doc_id>.pdf
    │     2. Run pdf_tool.py inspect -> returns page_count, encrypted, pages[{page_num, width, height, rotation, has_geo}]
    │     3. Render lightweight thumbnails for all pages into thumbnails/
    │     4. Return { type: "pdf_pending", document_id, filename, pages: [...] }
    │     5. Frontend displays <ImportPdfModal> (Thumbnails grid, checkboxes, "Georeference together" vs "Georeference separately")
    │     6. User submits POST /datasets/import-pdf-pages with { document_id, selected_pages, georeferencing_mode, building_id, floor_id }
    │     7. Backend calls pdf_tool.py render-pages for selected pages -> generates high-res PNGs
    │     8. Registers each selected page as a distinct Dataset in session
    │     9. If "shared": sets shared building coordinate system & maps page coordinate origins
    │    10. Appends layers to workspace manifest & returns created dataset list
    │
    └─► If .dxf:
          1. Store original DXF in session storage: temp/sessions/<sid>/raw_docs/<cad_id>.dxf
          2. Run cad_tool.py convert -> generates high-res PNG with preserved linework and aspect ratio
          3. Extracts drawing extents, units, layer count, bounds
          4. Registers converted PNG as Dataset with attached CAD metadata
          5. Proceeds to existing Georeference / Workspace flow seamlessly
```

---

## 3. Data Model Enhancements (`models/dataset.go`)

In `Dataset.ExtractedMetadata`:
- `source_document_id`: ID of parent document (PDF or CAD).
- `source_document_type`: `"pdf"` | `"cad_dxf"`.
- `source_filename`: Original filename.
- `pdf_page_number`: 1-based page number.
- `pdf_total_pages`: Total pages in source PDF.
- `georeferencing_mode`: `"shared"` | `"independent"`.
- `shared_reference_id`: Master dataset ID for shared coordinate mapping.
- `cad_format`: `"DXF"`.
- `cad_units`: Extracted units (e.g., meters, mm, inches).
- `cad_extents`: `[minX, minY, maxX, maxY]`.
- `cad_layers`: List of layer names.

---

## 4. API Endpoints

1. `POST /upload`
   - Accepts `.pdf` and `.dxf` in addition to existing raster/zip.
   - For `.pdf`: returns `HTTP 200` with inspection payload (`type: "pdf_pending"`).
   - For `.dxf`: converts directly to raster dataset and returns `HTTP 201 Created`.
2. `POST /datasets/inspect-pdf`
   - Can inspect an uploaded PDF directly or re-inspect by document ID.
3. `GET /documents/{doc_id}/pdf-thumbnail/{page_num}`
   - Serves generated thumbnail PNG for page inspection dialog.
4. `POST /datasets/import-pdf-pages`
   - Request:
     ```json
     {
       "document_id": "doc_...",
       "selected_pages": [1, 2, 4],
       "georeferencing_mode": "shared" | "independent",
       "building_id": "bld_...",
       "building_name": "Building North"
     }
     ```
   - Renders selected pages to PNG, registers datasets, configures shared georeferencing if selected, returns array of created datasets.

---

## 5. Frontend Dialogs & Integration

1. **`ImportPdfModal.jsx`**:
   - Matches existing UI aesthetic (Tailwind, Lucide icons, glassmorphism, responsive).
   - Thumbnail grid with selection checkboxes, Select All / Deselect All, selected counter.
   - Mode selection:
     - **Option A**: *Georeference together (one-time)* — Align selected pages using one shared building coordinate system.
     - **Option B**: *Georeference separately* — Align each selected page independently.
   - Building & Floor naming inputs.
   - Progress and error states.
2. **`UploadPage.jsx`**:
   - Detects PDF uploads, opens `ImportPdfModal`.
   - On completion, redirects directly to workspace with all selected pages loaded.
3. **`WorkspacePage.jsx`**:
   - Supports importing PDFs directly within an active workspace (e.g. adding floor plans to existing building).
   - MapLibre raster source/layer management properly renders each PDF page or CAD raster without canvas reset.

---

## 6. Export Integration (`export_selective.go`)

- In `SelectiveExportOptions`:
  - When `IncludeOriginalRasters` is true, also package the original `.pdf` and `.dxf` under `source_documents/`.
  - In `manifest.json`, record `source_document_id`, `pdf_page_number`, and `cad_metadata` for each layer.
  - Dedup: Source PDF is only written once in the ZIP, while individual page manifests reference it.

---

## 7. Testing & Verification

1. **Backend Integration Tests (`backend/tests/api_pdf_cad_test.go`)**:
   - Single-page and multi-page PDF generation and inspection.
   - Password-protected/corrupted PDF rejection.
   - Selective page import (subset vs all).
   - Shared vs independent georeferencing modes.
   - DXF file conversion to PNG and metadata extraction.
   - Export ZIP containing source PDF/CAD and generated rasters.
2. **Frontend Tests (`frontend/src/lib/pdfSelection.test.js`)**:
   - Selection state calculations, tri-state toggles, validation.
3. **Full Build Verification**:
   - `go test -v -count=1 ./tests/...`
   - `go vet ./...`
   - `gofmt -l .`
   - `cmd.exe /c "npm.cmd run build"`
