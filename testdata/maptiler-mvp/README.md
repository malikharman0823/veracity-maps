# MapTiler MVP Deterministic Test Data

This folder contains deterministic GIS test datasets generated programmatically.

## Files
- ordinary_grid.png: 400x300 RGB PNG with grid, colored corner anchors, and center marker
- ordinary_photo.jpg: 400x300 JPEG image
- georeferenced_campus.png / .pgw / .prj: Georeferenced image bundle with known EPSG:4326 world file
- valid_bundle.zip: Clean ZIP archive of georeferenced_campus
- multi_images.zip: Multi-raster floorplan package
- zip_slip_malicious.zip: Security test fixture for path traversal attacks
- corrupted/invalid fixtures: truncated.png, truncated.jpg, text_as_png.png, zero_byte.png, unsupported.xyz
- control_points_fixture.json: Coordinates for similarity and affine transformation testing

## Regeneration
To regenerate all fixtures deterministically, run:
```powershell
cd c:\Users\harma\OneDrive\Desktop\veracity-maps\maptiler-go
go run testdata/generate_testdata.go
```
