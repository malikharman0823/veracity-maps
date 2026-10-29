# Package: `services`

Located in [`backend/services/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/services)

The `services` package contains the core business logic of Veracity Maps, coordinating between storage managers, session repositories, GIS solvers, and archive packagers.

---

## Files in Package

### 1. `dataset_service.go`
- **Purpose**: Manages raster and archive uploads, disk streaming, SHA-256 integrity calculation, image decoding, ZIP-slip sanitization, and multi-floor building archive ingestion.
- **Key Methods**:
  - `HandleUpload(sessionID string, fileHeader *multipart.FileHeader) (*models.Dataset, error)`: Entry point for uploads.
  - `handleSingleRasterUpload`: Streams file to disk, verifies header via `gis.InspectRasterFile`, auto-creates default map layers, and registers dataset in the session repository.
  - `isIconAsset`: Guard preventing icon images (e.g. `001-camera.png`, `icon-*.png`, tiny pictorial PNGs/JPEGs) from being imported and georeferenced as giant background map rasters; directs them to be used as icon markers/annotations instead.
  - `handleZipUpload`: Securely extracts ZIP files, enforces path containment against canonical directory boundaries, locates all raster floor plans (filtering out macOS resource forks `__MACOSX`, `.DS_Store`), automatically detects building identities and floor levels, creates `models.Building` and `models.BuildingFloor` structures, and attaches all floors to the workspace manifest.
  - `PropagateBuildingGeoreference(sessionID, refDataset)`: Propagates spatial footprint, affine transformations, scaled world files, and layer coordinates across all sibling floors in the same building entity.
  - `ListDatasets`, `GetDataset`, `DeleteDataset`: CRUD delegates interacting with the repository and storage managers.

### 2. `georeference_service.go`
- **Purpose**: Handles control point validation, coordinate range enforcement, raster boundary checks, georeferencing execution, and building-wide spatial propagation.
- **Key Methods**:
  - `CreateControlPoint(sessionID, datasetID, cp)`: Enforces latitude `[-90, 90]`, longitude `[-180, 180]`, finite number validation, and raster dimension bounds (`[0, width)` and `[0, height)`).
  - `UpdateControlPoint(sessionID, pointID, cp)`: Updates coordinates with matching validation.
  - `DeleteControlPoint(sessionID, pointID)`: Removes a control point.
  - `Georeference(sessionID, datasetID, action, transformation)`: Calls `gis.CalculateGeoreference`, generates companion world files (`.pgw`/`.jgw`), outputs a standard GDAL VRT XML, conditionally triggers `gdal_translate` if GDAL is installed, updates layer bounds to MapLibre GL coordinates, marks status as `"completed"`, and automatically propagates bounds and layers to all sibling floors in the building via `PropagateBuildingGeoreference`.
  - `PropagateBuildingGeoreference(sessionID, refDataset)`: Synchronizes building footprint, bounds, and scaled world files to sibling floors.
  - `ConsolidateSessionBuildings(repo, store, sessionID)`: Scans the session for pseudo-buildings created from floor tokens (e.g. "Floor 1", "Floor B1", "Floor G", "Floor 2"), merges their datasets and floors into their real parent building (e.g. "Building A" or "Building 1"), cleans up obsolete pseudo-building records, and automatically triggers building-wide georeference propagation so all sibling floors inherit the spatial footprint and become georeferenced.
  - `IsFloorName(name)`: Pure function classifying whether a name string represents a floor or level indicator rather than a structural building name.

### 3. `export_service.go` & `export_selective.go`
- **Purpose**: Compiles dataset assets into portable, standalone downloadable archives following MapTiler and MapLibre GL JS packaging specifications.
- **Root-Level ZIP Structure**:
  ```text
  exported-map.zip
  │
  ├── 0/
  │   └── 0/
  │       └── 0.png
  ├── assets/
  │   └── source/
  │       └── original-raster.png
  ├── index.html
  ├── viewer.html
  ├── style.json
  ├── mapstudio.json
  ├── metadata.json
  └── README.txt
  ```
- **Root Files & Convention (Strict MapTiler & Viewscape Compatibility)**:
  - `{z}/{x}/{y}.png`: Web Mercator XYZ raster tiles covering the georeferenced dataset bounds with optional burn-in markers/annotations.
  - `assets/source/`: Contains the original uncompressed source raster image for roundtrip fidelity.
  - `viewer.html` & `index.html`: Fully self-contained MapLibre GL JS interactive web viewer with layer opacity slider, overlay visibility toggle, viewport reset to dataset bounds, and mouse hover coordinate tracker. Works out-of-the-box over any static HTTP server or live-server extension without backend dependencies.
  - `style.json`: Valid MapLibre GL JS Style Specification (v8) document declaring OSM raster basemap and the local `./{z}/{x}/{y}.png` overlay raster tile source and layer with bounds, minzoom, maxzoom, and initial opacity.
  - `mapstudio.json`: Portable MapStudio project configuration document storing georeferencing control points, icon points, dataset metadata, and layer opacity for lossless project restoration.
  - `metadata.json`: Standard MapTiler tile package metadata format (`format: "png"`, `version: "1.1.0"`, `profile: "mercator"`, `bounds: [west, south, east, north]`, `minzoom`, `maxzoom`), with preserved control points and icon points for roundtrip re-opening.
  - `README.txt`: Comprehensive instructions for local serving via Python `http.server`, Node.js `serve`, or VS Code Live Server, along with basemap network requirements and attribution.
- **Key Methods**:
  - `GenerateExport(sessionID, datasetID, format)`: Exports standard `.zip` (`mtmap`) matching the MapTiler MapLibre package containing numeric zoom folders, `assets/source/`, `metadata.json`, `mapstudio.json`, `style.json`, `viewer.html`, `index.html`, and `README.txt`.
  - `GenerateSelectiveExport(sessionID, primaryDatasetID, params)`: Exports selective project hierarchies with options for buildings, floors, raster types, and MapLibre style configuration. Supports both standard Map Studio package (`format: "mtmap"`) and Complete Project ZIP (`format: "project_zip"` / `"complete_project"`).
  - `createCompositeRasterWithMarkers`: Burns control points (green crosshairs with white ring) and icon points (custom icon image or default red dot with white outline and center) directly into the raster image before tile generation so that third-party tile viewers (such as Viewscape) display all annotations.
  - `gis.GenerateRasterTiles`: Renders Web Mercator XYZ tiles down to regional overview zoom levels (starting from zoom 7) with sub-pixel marker badges for low zooms, ensuring visibility when viewed in regional overviews like Viewscape's "Set Map Default Position" dialog.

- **Complete Project ZIP Package Structure (`format: "project_zip"`)**:
  ```text
  veracity-project.zip
  │
  ├── project.json                      # Versioned project manifest (schema v1, format: "veracity-project")
  ├── style.json                        # MapLibre Style Specification v8 with local raster sources
  ├── index.html & viewer.html          # Self-contained offline MapLibre GL JS viewer
  ├── README.txt                        # Offline reconstruction guide & format specification
  ├── layers/
  │   ├── landmarks.geojson             # GeoJSON FeatureCollection of custom landmark icon markers
  │   └── control_points.geojson        # GeoJSON FeatureCollection of georeferencing GCPs (image X/Y & lat/lng)
  ├── metadata/
  │   ├── layer_config.json             # Layer ordering, visibility, opacity, and floor assignments
  │   └── georeferencing.json           # CRS, transformation parameters, bounds, and GCP coordinates
  ├── rasters/                          # Preserved source and processed floorplan images per layer
  │   ├── {layer_id}_source.png
  │   └── {layer_id}_processed.png
  ├── assets/
  │   ├── icons/                        # Bundled custom SVG/PNG icon marker assets
  │   └── source/                       # Backward-compatible source image root
  ├── tiles/                            # Local tile pyramid mirror
  │   └── {z}/{x}/{y}.png
  ├── basemap/                          # Offline world basemap tiles (when available)
  └── {z}/{x}/{y}.png                   # Direct root tile pyramid for standard viewers
  ```

- **Project Manifest Schema (`project.json`)**:
  - `format`: `"veracity-project"`
  - `schema_version`: `1`
  - `project`: Project metadata (`id`, `name`, `timestamp`)
  - `map`: Viewport configuration (`center: [lng, lat]`, `zoom`, `min_zoom`, `max_zoom`, `preset`, `bounds`)
  - `resources`: Relative paths to bundled layers, styles, rasters, and metadata
  - `layers`: Preserved layer configuration with opacity, visibility, and source paths
  - `georeferencing`: Spatial footprint, affine parameters, control points, and CRS
  - Fully roundtrip re-importable via `/map-projects/import` (`handleZipDataset`).
- **Single MBTiles File (`format: "mbtiles"`)**:
  - Exports a portable map tile database package (`.mbtiles.zip` or existing `.mbtiles` file) containing the raster XYZ tile pyramid, `metadata.json` with tile bounds, zoom limits, and attribution.
  - Documents explicit limitation: MBTiles does not preserve editable control points, custom icon assets, or project configurations.

- **Base Layer Only (`format: "base_layer"` / `"basemap"`)**:
  - Dedicated export packaging standalone offline base map tiles (`veracity-base-map.zip`) without project-specific overlays, landmarks, or control points.
  - Contains `./basemap/{z}/{x}/{y}.png`, standalone MapLibre `style.json` (v8) with `offline-basemap` source and layer, `metadata.json`, and offline reconstruction documentation for reuse in other Viewscapes.
