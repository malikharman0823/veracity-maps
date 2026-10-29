# Implementation Plan: Replace Leaflet with MapLibre GL JS and MapTiler-Style ZIP Export

## Objective
Completely eliminate all Leaflet dependencies, viewers, and references from Veracity Maps (Go version) and standardize map rendering and export packaging on MapLibre GL JS. Implement portable, self-contained MapTiler-style ZIP exports (`metadata.json`, `mapstudio.json`, `style.json`, `viewer.html`, `index.html`, `README.txt`, `assets/source/`, and `{z}/{x}/{y}.png` tiles) that run independently in local Viewscapes and static web servers without requiring the Veracity Maps backend.

---

## Current State
1. **Frontend**:
   - The React application (`frontend/src`) is already powered by MapLibre GL JS (`maplibre-gl` v6.12.0) across `WorkspaceMap.jsx`, `Map.jsx`, `ReferenceMap.jsx`, and `GeoreferencePage.jsx`.
   - `package.json` contains no Leaflet dependencies.
   - No Leaflet imports exist in `frontend/src`.
2. **Backend**:
   - `backend/services/export_service.go` currently exports legacy HTML viewers: `leaflet.html`, `openlayers.html`, `googlemaps.html`, and `index.html` (TileServer viewer).
   - Tile generation in `backend/gis/tiles.go` creates extensionless entries `{z}/{x}/{y}` instead of standard `{z}/{x}/{y}.png`.
   - `backend/tests/api_zip_structure_test.go` and `backend/tests/api_full_matrix_test.go` assert the presence of `leaflet.html`.
   - `backend/services/README.md` documents `leaflet.html`, `openlayers.html`, and `googlemaps.html`.

---

## Target State
1. **Zero Leaflet**: No Leaflet, OpenLayers, or Google Maps scripts, stylesheets, templates, or references anywhere in backend export services, templates, or tests.
2. **MapLibre-Powered Export Package**:
   - `viewer.html`: Standalone, interactive MapLibre GL JS map viewer with basemap, raster overlay, opacity slider, layer visibility toggle, zoom extents, and error handling.
   - `index.html`: Web-server-ready entry point serving the MapLibre viewer.
   - `style.json`: Valid MapLibre Style Specification (v8) defining the basemap source (`osm`), dataset raster overlay source (`overlay`), tile URL `./{z}/{x}/{y}.png`, min/max zoom, bounds, and paint opacity.
   - `metadata.json`: MapTiler-compatible tile metadata with normalized `[west, south, east, north]` bounds.
   - `mapstudio.json`: Portable project document matching Python MapTiler / MapStudio schema with relative source asset reference, control points, icon points, and layer settings.
   - `README.txt`: Documentation explaining local HTTP serving instructions, offline/online basemap notes, and directory layout.
   - `assets/source/<filename>`: Preserved original source raster image.
   - `{z}/{x}/{y}.png`: Raster tiles organized in standard XYZ directory hierarchy.
3. **TMS to XYZ Conversion**:
   - Clean, validated `TMSToXYZ` and `XYZToTMS` functions in `backend/gis/tiles.go`.
4. **Tested & Verified**:
   - Comprehensive unit and integration tests passing with 100% success.
   - Browser smoke test and verification of exported ZIP in an isolated local HTTP server.

---

## Acceptance Criteria
- [x] Zero Leaflet, OpenLayers, or Google Maps references remaining in code, templates, or tests.
- [x] Exported ZIP contains `viewer.html`, `index.html`, `style.json`, `mapstudio.json`, `metadata.json`, `README.txt`, `assets/source/`, and `{z}/{x}/{y}.png`.
- [x] `style.json` conforms to MapLibre style spec (version 8).
- [x] `viewer.html` renders MapLibre map with basemap and raster overlay.
- [x] Tile paths follow `{z}/{x}/{y}.png`.
- [x] TMS to XYZ tile row conversion mathematically verified.
- [x] Re-importing exported ZIP into fresh session succeeds with full metadata restoration.
- [x] All automated tests pass (`test-all.ps1`).
