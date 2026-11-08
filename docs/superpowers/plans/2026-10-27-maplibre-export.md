00   - `mapstudio.json`: Portable project document matching Python MapTiler / MapStudio schema with relative source asset reference, control points, icon points, and layer settings.
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
