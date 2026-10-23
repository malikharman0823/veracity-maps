# Veracity Maps Update 1.2

Update 1.2 expands document ingestion and improves map usability. It adds selective multi-page PDF import, DXF rasterization, document-aware workspace grouping, dynamic selected-file previews, and reliable custom icon rendering in MapLibre GL.

## Highlights

### Multi-page PDF inspection and import

- PDF uploads are stored once in the active session and inspected before raster datasets are created.
- The import dialog displays generated page thumbnails, dimensions, rotation, and a live selection count.
- Users can select any combination of pages and choose shared or independent georeferencing.
- Selected pages become individual raster datasets and can be managed as building floors.
- The original PDF is preserved for selective project export without duplicate copies.

New API routes:

- `GET /documents/{document_id}/pdf-thumbnail/{page_number}`
- `POST /datasets/import-pdf-pages`

### CAD drawing conversion

- DXF uploads are validated and rasterized into transparent PNG datasets.
- Conversion preserves the drawing aspect ratio and records units, extents, layer names, and DXF version metadata.
- The original source drawing is retained for project export.
- Conversion uses the lightweight Python helpers in `backend/scripts/` with PyMuPDF for PDFs and `ezdxf` plus Matplotlib for DXF drawings.

### Document-aware workspace layers

- PDF pages and ZIP-derived rasters are dynamically grouped under their source document.
- Groups include page or raster counts, visibility controls, thumbnails, georeferencing status, transform actions, background removal, and detach actions.
- Standalone datasets remain standalone instead of being assigned to a hardcoded building.

### Dynamic file preview popup

- Selecting a file from either upload screen now opens one reusable preview dialog.
- Browser-renderable image types, including PNG and JPEG, use an object URL for an actual visual preview.
- ZIP archives and other non-image formats use the same component to show live filename, MIME type, and size metadata.
- Preview behavior is derived from the selected file rather than separate hardcoded dialogs for each extension.

### Map icon rendering repair

- Processed icons now return `ImageData`, a format supported by MapLibre GL `addImage()`.
- Failed requests are no longer cached as successful red-dot icons.
- Existing icon cleanup, resizing, and transparency handling remain active.
- Custom markers now render their selected catalog image on the map.

## Main implementation files

### Backend

- `backend/models/pdf.go`
- `backend/services/pdf_cad_service.go`
- `backend/scripts/pdf_tool.py`
- `backend/scripts/cad_tool.py`
- `backend/functions/upload/function.go`
- `backend/api/router.go`
- `backend/services/export_selective.go`
- `backend/tests/api_pdf_cad_test.go`

### Frontend

- `frontend/src/components/ImportPdfModal.jsx`
- `frontend/src/components/FilePreviewDialog.jsx`
- `frontend/src/components/WorkspaceLayerPanel.jsx`
- `frontend/src/lib/filePreview.js`
- `frontend/src/lib/filePreview.test.js`
- `frontend/src/lib/iconProcessing.js`
- `frontend/src/pages/HomePage.jsx`
- `frontend/src/pages/UploadPage.jsx`
- `frontend/src/pages/WorkspacePage.jsx`

## Verification

The update was verified on Windows with:

```powershell
# Complete repository runner
powershell -ExecutionPolicy Bypass -File test-all.ps1

# Frontend unit tests
Set-Location frontend
cmd.exe /c "npm.cmd test"

# Backend focused checks
Set-Location ..\backend
go test -v -count=1 ./tests/...
go vet ./...
gofmt -l .
```

Results:

- Frontend unit tests: 12 passed.
- Backend unit and integration tests: passed.
- Go static analysis and formatting: passed.
- Frontend production build: passed.
- Automated repository runner: completed with 0 errors.
- Browser validation: the reported red-dot marker rendered as its selected tank icon with no relevant console errors.

The production build continues to report the existing Rollup large-chunk advisory; it does not fail the build.
