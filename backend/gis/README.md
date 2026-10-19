# Package: `gis`

Located in [`backend/gis/`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/gis)

The `gis` package implements spatial algorithms natively in pure Go, including raster metadata extraction, world file parsing and writing, GDAL VRT XML generation, ground control point spatial distribution analysis, and least-squares regression for similarity and affine transformations.

---

## Files in Package

### 1. `inspect.go`
- **Purpose**: Detects external GDAL tools and extracts metadata from raster files and companion sidecars.
- **Key Structs**:
  - `RasterMetadata`: Contains driver name, width, height, bands, data type, bounds, resolution, CRS, transform matrix, and georeference status.
  - `GDALTools`: Availability flags and detected paths for `gdalinfo`, `gdal_translate`, and `gdalwarp`.
- **Key Functions**:
  - `DetectGDAL() GDALTools`: Uses `exec.LookPath` to dynamically detect GDAL binaries.
  - `InspectRasterFile(filePath string) (*RasterMetadata, error)`: Decodes image headers via `image.DecodeConfig` (without loading the entire raster into memory) and searches for sidecar files (`.pgw`, `.jgw`, `.wld`, `.prj`).
  - `ParseWorldFile(path string) ([]float64, error)`: Parses 6-line standard world file coordinates into `[a, b, c, d, e, f]` matrix.
  - `CalculateBoundsFromTransform(transform []float64, width, height int) map[string]float64`: Computes `north`, `south`, `east`, `west` corners from the transformation coefficients.
  - `WriteWorldFile(path string, transform []float64) error`: Writes 6-line world files (`A, D, B, E, C, F` format).
  - `WriteGDALVrt(vrtPath, srcImagePath string, width, height int, transform []float64, crs string) error`: Generates standard GDAL VRT XML linking the raster to its affine georeferencing matrix.

### 2. `transform.go`
- **Purpose**: Computes mathematical georeferencing models using ordinary least-squares matrix inversion.
- **Key Structs**:
  - `TransformResult`: Contains transformation type (`"similarity"` or `"affine"`), parameter map, 6-element affine transform array, target CRS (`"EPSG:4326"`), corner bounds, and MapLibre GL bounds (`[west, south, east, north]`).
- **Core Algorithms & Logic**:
  - `CalculateGeoreference(points, width, height, requestedTransform)`:
    - Validates point counts: 2 points -> similarity; 3–10 points -> affine.
    - Validates coordinates: finite numbers, within raster bounds, valid lat/lon.
    - `validateDistribution(points, width, height)`: Enforces point spread (diagonal extent >= 10% of image diagonal) and collinearity checks (triangle cross-product area >= 1% of bounding box).
  - `solveSimilarity(points)`: Solves normal equations for 4 unknowns: scale, rotation, and translation offsets.
  - `solveAffine(points)`: Solves 6-parameter affine transformation using 3x3 matrix inversion via Cramer's rule / adjoint matrix.

---

## Testing & Quality

- **Tests**:
  - [`backend/tests/gis_transform_test.go`](file:///c:/Users/harma/OneDrive/Desktop/veracity-maps/backend/tests/README.md) validates similarity equations, affine regression, collinearity rejection, and insufficient point error states.
