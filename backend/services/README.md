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
- **Purpose**: Compiles dataset assets into downloadable archives following standard GIS web-tile structures.
- **Root-Level ZIP Structure**:
  ```text
  veracity-maps-export.zip
  │
  ├── {z}/
  │   └── {x}/
  │   └── {x}/
  │   └── {x}/
  │       └── {y}
  ├── googlemaps.html
  ├── index.html
  ├── leaflet.html
  ├── metadata.json
  └── openlayers.html
  ```
- **Root Files & Convention (Strict MapTiler & Viewscape Compatibility)**:
  - `{z}/{x}/{y}`: Extensionless XYZ map tiles (e.g. `7/90/53`) covering the georeferenced dataset bounds with markers and annotations composited directly into the raster tiles.
  - `index.html`: Exact MapTiler `tileviewer` template using `https://cdn.maptiler.com/tileviewer/v1/` calling `tileserver(data)`.
  - `leaflet.html`: Standalone viewer powered by Leaflet v1.7.1 using `L.tileLayer('{z}/{x}/{y}', options)`.
  - `openlayers.html`: Standalone OpenLayers v6.12.0 viewer configured with `url: "{z}/{x}/{y}"`.
  - `googlemaps.html`: Google Maps JS API viewer using `google.maps.ImageMapType` overlay returning `zoom + "/" + x + "/" + y`.
  - `metadata.json`: Exact MapTiler metadata format (`"format": "hybrid"`, `"version": "1.1.0"`, `"scale": "1.000000"`, `"profile": "mercator"`, `"bounds": [west, south, east, north]`), with preserved `control_points` and `icon_points` for roundtrip re-opening in Veracity Maps.
- **Key Methods**:
  - `GenerateExport(sessionID, datasetID, format)`: Exports standard `.zip` (`mtmap`) matching the exact MapTiler package containing numeric zoom folders, `metadata.json`, and the 4 HTML viewers (`index.html`, `leaflet.html`, `openlayers.html`, `googlemaps.html`).
  - `GenerateSelectiveExport(sessionID, primaryDatasetID, params)`: Exports selective project hierarchies with options for buildings, floors, raster types, and styling.

