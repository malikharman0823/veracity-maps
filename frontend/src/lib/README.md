# Directory: `frontend/src/lib`

Located in [`frontend/src/lib/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/frontend/src/lib)

`floorSelection.js` applies controlled workspace floor state and restores visibility for matching floor members without feeding the selection back through the map callback.

`sessionLoading.js` waits for active-session recovery before starting session-scoped dashboard requests, normalizes saved-configuration API payloads, and maps workspace members to the dataset-ID list required by the saved-config API.

This directory contains utility functions and pure helper logic supporting UI styling and navigation.

---

## Files in Directory

### 1. `utils.js`
- **Purpose**: Exports the canonical `cn(...inputs)` helper combining `clsx` and `tailwind-merge` for safe conditional class name composition.

### 2. `workspaceNavigation.js`
- **Purpose**: Encapsulates workspace URL route generation and view mode switching (`"map"` vs `"database"`).
- **Functions**:
  - `workspaceViewPath(requestedView, id)`: Generates the route path for switching between tabular dataset view and interactive map view.

### 3. `workspaceNavigation.test.js`
- **Purpose**: Unit tests verifying route generation for default, map, and database view modes. Run via `npm.cmd test`.

### 4. `coordinateParser.js`
- **Purpose**: Parses Google Maps coordinate inputs and computes direct-placement corner control points without manual map clicking.
- **Functions**:
  - `parseGoogleMapsCoordinates(input, options)`: Parses Google Maps decimal `Lat, Lng` (with unambiguous `Lng, Lat` auto-swap when `|first| > 90 && |second| <= 90`), DMS/DDM with cardinal `N/S/E/W`, labeled `Lat:/Lng:`, Google Maps URLs (prioritizing exact `!3d<lat>!4d<lng>` pin coordinates over `@<lat>,<lng>,<zoom>z` viewport centers), and full 8+-character Plus Codes (Open Location Code).
  - `metersPerDegreeAtLatitude(lat)`: Computes WGS84 ellipsoidal meters-per-degree (`latMeters`, `lonMeters`) at a given latitude.
  - `buildDirectCoordinateControlPoints(params)`: Generates 4 valid raster corner GCPs (`0 <= image_x < width`, `0 <= image_y < height`) from either `"center"` (`centerLat`, `centerLon`, `groundWidthMeters`, `rotationDeg` preserving exact image aspect ratio) or `"corners"` (Top-Left NW & Bottom-Right SE `Lat, Lng`).

### 5. `coordinateParser.test.js`
- **Purpose**: Unit tests covering Google Maps decimal, DMS, cardinal, URL pin vs. viewport, Plus Code parsing, and direct-placement corner GCP generation.
