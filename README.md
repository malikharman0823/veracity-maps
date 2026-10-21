# Veracity Maps

**Veracity Maps** is a high-performance, lightweight geospatial raster management and visual georeferencing web platform. Built with a pure-Go backend (`net/http`) and a responsive modern React / Vite / TailwindCSS / MapLibre GL frontend, it provides an end-to-end workflow for uploading unreferenced aerial and historical maps, interactively placing ground control points (GCPs), computing mathematical least-squares georeferencing transformations, placing geographic icon markers, managing multi-floor building plans, and importing multi-dataset ZIP archives.

---

## 1. System Architecture

```mermaid
graph TD
    subgraph Browser ["Frontend (React 19 + Vite + MapLibre GL)"]
        UI[User Interface & Canvas]
        GCP[Visual Control Point Placer]
        Viewer[MapLibre GL Map Viewport]
        Icons[Icon Picker & Point Placement Modal]
        Floors[Multi-Floor Elevator Switcher]
        ZipReview[ZIP Import Review Modal]
        Client[Axios Client (Session Scoped)]
    end

    subgraph Server ["Backend (Go 1.22+ Standard net/http)"]
        Router[HTTP Router & Route Dispatcher]
        Repo[(In-Memory Session Repository)]
        Storage[(Per-Session Temporary Disk Storage)]
        GIS[GIS Least-Squares Transform Engine]
        Exporter[MTMAP & VRT Export Engine]
        ZipInspector[Multi-Dataset Archive Inspector]
    end

    UI -->|Multipart Upload| Router
    GCP -->|Coordinates & GCPs| Router
    Icons -->|Custom Icon Points CRUD| Router
    Floors -->|Building & Floor Plans| Router
    ZipReview -->|Inspect & Selective Import| Router
    Viewer -->|Raw Raster & Dynamic Icons| Router
    Router --> Client
    Router --> Repo
    Router --> Storage
    Router --> GIS
    Router --> Exporter
    Router --> ZipInspector
```

### Technology Stack
- **Backend**: Go (`go1.22+`), zero external dependencies, standard library (`net/http`, `image`, `archive/zip`, `crypto/rand`, `math`).
- **Frontend**: React 19, Vite 7, TailwindCSS 4, MapLibre GL JS 6, Lucide Icons.
- **Persistence**: Zero external database. State is managed via an in-memory session repository (`SessionRepository`) with atomic read/write locks, combined with isolated temporary disk workspaces (`temp/sessions/<session_id>/`).
- **GIS Engine**: Pure-Go implementation of 2-point similarity and 3-to-10-point affine least-squares regression, native TIFF header decoder, world file parsing/generation (`.pgw`, `.jgw`, `.wld`, `.tfw`), GDAL VRT XML generation, and optional CLI integration with `gdal_translate`/`gdalwarp` if present in PATH.

---

## 2. Core Features

### Feature A — Custom Geographic Icon Points
- **Icon Asset Library**: Portable icon catalog located in `frontend/public/icons/` with a deterministic `manifest.json`.
- **Supported Formats**: PNG, SVG, WebP, ICO, JPG with transparent backgrounds fully preserved.
- **Interactive Icon Picker**: Search by name or category, category filtering chips, recent icons memory, and transparent background checkerboard preview.
- **Point Placement Workflow**:
  1. Select an icon from the picker.
  2. Click **Place Point** to enter placement mode (crosshair map cursor).
  3. Click any point on the map canvas to capture geographic coordinates.
  4. Fill in point name, description, scope (`floor`, `building`, `dataset`), and floor level in the point details modal.
  5. Save, move, or delete points with live updates.
- **MapLibre GL Dynamic Rendering**: Uses MapLibre's `styleimagemissing` hook to lazily fetch and register icons on demand. Points render above raster overlays with smooth hover tooltips and label halos.

### Feature B — Multiple Floor Plans per Building
- **Building & Floor Models**: Buildings represent physical structures (`Building`) containing multiple ordered levels (`BuildingFloor`), each mapped to independent raster datasets.
- **Independent Georeferencing**: Each floor maintains its own coordinate transformation, raster resolution, bounding box, and ground control points without interfering with other floors.
- **Elevator Floor Switcher**: Floating interactive widget on the map allowing instant switching between levels (e.g. `B2`, `B1`, `G`, `Floor 1`, `Floor 2`).
- **Point Scopes**:
  - `floor`: Visible strictly on the designated floor level.
  - `building`: Persists and displays across all floor levels of the building.
  - `dataset`: Associated strictly with a specific raster dataset.

### Feature C — Multi-Dataset ZIP Archive Import
- **Staged Inspection Workflow**:
  1. **Upload**: User uploads any `.zip` archive via the upload page or workspace dialog.
  2. **Security & Validation**: Checks for ZIP-Slip path traversal (`..`), safe decompression limits, and uncompressed size ceilings.
  3. **Inspection**: Automatically discovers all nested raster images (`.png`, `.jpg`, `.tif`, `.webp`, `.bmp`) and auto-associates matching sidecars (`.pgw`, `.jgw`, `.tfw`, `.wld`, `.prj`).
  4. **Review Screen (`ZipImportReview`)**: Interactive table displaying discovered datasets, dimensions, georeferencing status, sidecars, and auto-suggested buildings and floors.
  5. **Selective Import**: Users can rename datasets, toggle individual items, assign buildings/floors, and import only desired layers without corrupting other datasets.

---

## 3. Importing the Icon Library (Windows)

A deterministic PowerShell import script is provided to discover, validate, deduplicate (via SHA-256), categorize, and copy icon assets from any local Windows folder into the portable repository location:

```powershell
# Run with default local Windows icon directory:
powershell -ExecutionPolicy Bypass -File scripts/import-icons.ps1

# Or specify a custom source directory:
powershell -ExecutionPolicy Bypass -File scripts/import-icons.ps1 -SourceDir "E:\Path\To\Icons"
```

The script:
1. Validates that the source directory exists.
2. Recursively traverses all subfolders to detect `.png`, `.svg`, `.webp`, `.ico`, and `.jpg` files.
3. Automatically computes SHA-256 hashes to skip duplicate icons.
4. Generates slugs, categorizes based on folder hierarchy, and extracts image dimensions.
5. Emits a deterministic, UTF-8 (no-BOM) `frontend/public/icons/manifest.json`.
6. Prints a summary report of imported, skipped, and duplicate assets.

---

## 4. Session Storage & Lifecycle Architecture

- **Session Directories**:
  ```text
  backend/temp/sessions/<session_id>/
  ├── datasets/
  │   └── <dataset_id>/
  │       ├── source.png
  │       ├── source.pgw
  │       └── metadata.json
  ├── buildings/
  │   └── <building_id>/
  │       └── floors.json
  └── imports/
      └── <import_id>/
          └── archive.zip
  ```
- **Windows File Lock Recovery**: Windows holds mandatory file descriptor locks on active files. Directory removals use `storage.SafeRemoveAll`, which resets read-only attributes and executes exponential backoff retries.
- **Session Cleanup**: A background worker runs every 10 minutes to prune idle sessions and wipe their disk storage cleanly.

---

## 5. Startup Instructions

### 1. Start the Go Backend
From the workspace root:
```powershell
Set-Location backend
go run main.go
```
The backend initializes on `http://localhost:8000`. Startup diagnostics display available storage paths and GDAL CLI detection status.

### 2. Start the React Frontend
In a separate terminal from the workspace root:
```powershell
Set-Location frontend
cmd.exe /c "npm.cmd run dev"
```
The development server starts on `http://localhost:5173`.

---

## 6. Running Tests & Automated Checks

To run the complete automated test suite (Go formatting, `go vet`, unit tests, building/floor tests, multi-dataset zip inspection/import tests, icon points tests, and frontend production build verification):

```powershell
# From the workspace root:
powershell -ExecutionPolicy Bypass -File test-all.ps1
```

Or run individual targeted commands:
- **Backend Unit & Integration Tests:**
  ```powershell
  Set-Location backend
  go test -v -count=1 ./tests/...
  ```
- **Backend Static Analysis:**
  ```powershell
  Set-Location backend
  gofmt -l .
  go vet ./...
  ```
- **Frontend Production Build:**
  ```powershell
  Set-Location frontend
  cmd.exe /c "npm.cmd run build"
  ```

---

## 7. Known Limitations & Notes

1. **In-Memory Volatility**: Data is scoped to the temporary session. Ending a session or allowing it to expire deletes all temporary raster files and points.
2. **GDAL Optional Dependency**: If `gdalinfo` or `gdalwarp` are absent from `PATH`, the application seamlessly falls back to pure-Go native raster inspection and affine matrix transformations.
3. **Archive Memory Limits**: Archive file uploads are capped at 500 MB and 1000 files per archive to ensure safety and prevent resource exhaustion.
