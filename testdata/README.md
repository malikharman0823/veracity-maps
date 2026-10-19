# Directory: `testdata`

Located in [`testdata/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/testdata)

This directory contains deterministic, reproducible synthetic GIS raster test fixtures and automated generation tools.

---

## Contents

- **`generate_testdata.go`**: Pure-Go standalone program that programmatically creates all 18 test fixtures with known dimensions, affine matrices, and intentional defect states.
- **[`maptiler-mvp/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/testdata/maptiler-mvp/README.md)**: Generated fixture directory containing:
  - `ordinary_grid.png`, `ordinary_photo.jpg` (Valid rasters)
  - `georeferenced_campus.png` + `.pgw` + `.prj` (Valid sidecars)
  - `valid_bundle.zip`, `multi_images.zip` (Archives)
  - `zip_slip_malicious.zip` (Security path traversal fixture)
  - `truncated.png`, `truncated.jpg`, `invalid_archive.zip`, `text_as_png.png`, `zero_byte.png`, `unsupported.xyz` (Negative inputs)
  - `unreferenced_park.png` + `control_points_fixture.json` (Ground-truth georeferencing suite)
  - `manifest.json`: Machine-readable fixture catalog.

### Regenerating Fixtures
To regenerate all test fixtures from scratch:
```powershell
go run testdata/generate_testdata.go
```
