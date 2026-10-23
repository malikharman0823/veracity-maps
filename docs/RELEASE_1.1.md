# Release 1.1 — Dynamic Selective Export & Multi-Floor Synchronization

Welcome to **Veracity Maps v1.1**. This release brings full selective multi-layer export capabilities, dynamic session-truthful project packaging, automated georeference propagation across building floors, non-destructive background transparency removal, in-plane raster transformation adjustments, and shared control points.

---

## 🚀 Key Highlights & New Capabilities

### 1. Dynamic Selective Map Project Export Modal
- **Session-Truthful Layer Listing**: The export modal is dynamically computed strictly from the active session manifest (`manifest.members`). Standalone floor plans (like single `floorplan.png` uploads) appear as clean individual layer cards without phantom fallback containers (eliminating dummy `"Building A"` or `"Building B"` placeholders).
- **Hierarchical Building & Multi-Floor Selection**: When real multi-floor buildings are defined, layers are structured by building and floor level with tri-state selection checkboxes (`all`, `some`, `none`), layer counts, and collapsible accordions.
- **Granular Export Options**:
  - Building configurations (floors, ordering, level tags).
  - Shared control points & georeferencing matrices.
  - Custom geographic icon markers.
  - Layer styles (opacity, transparency removal, rotation).
  - Original and processed transparent rasters.
  - Base map styles & view configurations.
  - Project metadata (`manifest.json` specification).
- **Live Visual Preview & Metadata Inspector**: Selecting any layer previews the transformed raster with its opacity, background removal status, dimensions, and building/floor affiliation.
- **Roundtrip Re-importing**: Exported project `.zip` archives can be re-opened through the **Reopen Project** workflow, restoring all transformations, rasters, and GCPs.

---

### 2. Multi-Floor Georeference & Control Point Propagation
- **Zero-Redundancy Building Georeferencing**: Georeferencing a single reference floor automatically propagates affine coordinates and bounds to all other floors in the same building structure.
- **Floor-Aware Control Point Isolation**: Solved overlapping point artifacts. Active floor points are clearly distinguished, while sibling floor control points are hidden from the active canvas to prevent clutter.
- **Shared Spatial Alignment**: Guarantees that multi-story architectural drawings stack with millimeter precision in MapLibre GL.

---

### 3. Integrated Raster Background Removal & Transformations
- **Pure-Go Transparency Engine**: Color-distance background removal (`POST /datasets/{id}/transparency`) with live threshold and tolerance sliders, rendering transparent PNGs with alpha channels directly on the fly.
- **Live In-Plane Transform Controls**: Rotate (`0° - 360°`), flip horizontally/vertically, and fine-tune offset parameters (`POST /datasets/{id}/transform`) with instant client-side preview and server-side coordinate matrix updating.

---

### 4. Robust Windows Filesystem & Error Recovery
- **File-Lock Resilient Storage Deletion**: Enhanced `storage.SafeRemoveAll` with exponential backoff and attribute resetting, preventing Windows EPERM / access-denied locks on active session directories.
- **Error Boundaries**: Wrapped map viewports in React Error Boundaries to prevent full-page crashes if WebGL contexts are lost or corrupted tiles are requested.

---

## 🛠️ Verification & Test Suite

All changes adhere to the Veracity Maps testing and quality standards:

```bash
# 1. Backend tests (100% green)
cd backend
go test -v -count=1 ./tests/...

# 2. Go static analysis & formatting
go vet ./...
gofmt -l .

# 3. Frontend unit tests (100% pass)
cd ../frontend
node --test src/lib/*.test.js

# 4. Frontend production bundle build
cmd.exe /c "npm.cmd run build"
```

---

## 📂 Summary of Modified & Added Components

- `frontend/src/lib/exportHierarchy.js`: Dynamic hierarchy extractor ensuring truthful layer and building grouping.
- `frontend/src/lib/exportHierarchy.test.js`: Unit tests verifying standalone and multi-floor grouping.
- `frontend/src/components/ExportProjectModal.jsx`: Modern selective export dialog with live preview and custom options.
- `backend/services/export_selective.go`: Server-side ZIP packager honoring selective building, floor, and layer filters.
- `backend/services/georeference_service.go`: Automatic building-wide affine transform propagation.
- `backend/tests/api_export_selective_test.go`: End-to-end integration test verifying export package contents and filtering.
