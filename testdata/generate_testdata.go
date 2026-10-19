package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	outDir := filepath.Join(".", "testdata", "maptiler-mvp")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		panic(err)
	}

	fmt.Printf("Generating test fixtures in: %s\n", outDir)

	// A. Ordinary PNG: 400x300 with colored grid, corners and center marker
	pngPath := filepath.Join(outDir, "ordinary_grid.png")
	generateGridImage(pngPath, "png", 400, 300)

	// B. Ordinary JPEG: 400x300
	jpgPath := filepath.Join(outDir, "ordinary_photo.jpg")
	generateGridImage(jpgPath, "jpeg", 400, 300)

	// C. Georeferenced PNG with .pgw and .prj
	geoPngPath := filepath.Join(outDir, "georeferenced_campus.png")
	generateGridImage(geoPngPath, "png", 400, 300)
	pgwPath := filepath.Join(outDir, "georeferenced_campus.pgw")
	// World file: A=0.0005, D=0.0, B=0.0, E=-0.0005, C=-73.985, F=40.750
	pgwContent := "0.0005000000\n0.0000000000\n0.0000000000\n-0.0005000000\n-73.9850000000\n40.7500000000\n"
	_ = os.WriteFile(pgwPath, []byte(pgwContent), 0644)
	prjPath := filepath.Join(outDir, "georeferenced_campus.prj")
	prjContent := `GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563]],PRIMEM["Greenwich",0],UNIT["degree",0.0174532925199433],AUTHORITY["EPSG","4326"]]`
	_ = os.WriteFile(prjPath, []byte(prjContent), 0644)

	// E. Valid ZIP archive containing georeferenced raster bundle
	zipPath := filepath.Join(outDir, "valid_bundle.zip")
	createValidZip(zipPath, geoPngPath, pgwPath, prjPath)

	// F. Multi-image ZIP
	multiZipPath := filepath.Join(outDir, "multi_images.zip")
	createMultiImageZip(multiZipPath)

	// G. Malicious Zip-Slip fixture
	zipSlipPath := filepath.Join(outDir, "zip_slip_malicious.zip")
	createZipSlipFixture(zipSlipPath)

	// H. Corrupted and invalid files
	_ = os.WriteFile(filepath.Join(outDir, "truncated.png"), []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "truncated.jpg"), []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "invalid_archive.zip"), []byte("PK\x03\x04corrupted-archive-stream-header"), 0644)
	_ = os.WriteFile(filepath.Join(outDir, "text_as_png.png"), []byte("This is plain text pretending to be an image file."), 0644)
	_ = os.WriteFile(filepath.Join(outDir, "zero_byte.png"), []byte{}, 0644)
	_ = os.WriteFile(filepath.Join(outDir, "unsupported.xyz"), []byte("Unsupported file format content"), 0644)

	// I. Control point fixture
	cpRasterPath := filepath.Join(outDir, "unreferenced_park.png")
	generateGridImage(cpRasterPath, "png", 400, 300)
	generateControlPointsManifest(filepath.Join(outDir, "control_points_fixture.json"))

	// Manifest JSON & README
	generateManifestJSON(outDir)
	generateReadme(outDir)

	fmt.Println("All test fixtures generated successfully!")
}

func generateGridImage(path, format string, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{240, 243, 246, 255}}, image.Point{}, draw.Src)

	// Grid lines
	gridColor := color.RGBA{200, 210, 220, 255}
	for x := 0; x < w; x += 40 {
		for y := 0; y < h; y++ {
			img.Set(x, y, gridColor)
		}
	}
	for y := 0; y < h; y += 40 {
		for x := 0; x < w; x++ {
			img.Set(x, y, gridColor)
		}
	}

	// Colored corner blocks (30x30)
	drawBlock(img, 10, 10, 40, 40, color.RGBA{230, 50, 50, 255})    // Top-left Red
	drawBlock(img, w-40, 10, w-10, 40, color.RGBA{50, 200, 50, 255}) // Top-right Green
	drawBlock(img, w-40, h-40, w-10, h-10, color.RGBA{50, 100, 230, 255}) // Bottom-right Blue
	drawBlock(img, 10, h-40, 40, h-10, color.RGBA{230, 200, 50, 255}) // Bottom-left Yellow

	// Central bullseye marker
	cx, cy := w/2, h/2
	drawBlock(img, cx-15, cy-15, cx+15, cy+15, color.RGBA{20, 20, 20, 255})
	drawBlock(img, cx-8, cy-8, cx+8, cy+8, color.RGBA{255, 255, 255, 255})

	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if format == "png" {
		_ = png.Encode(f, img)
	} else {
		_ = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	}
}

func drawBlock(img *image.RGBA, x0, y0, x1, y1 int, col color.Color) {
	for x := x0; x < x1; x++ {
		for y := y0; y < y1; y++ {
			img.Set(x, y, col)
		}
	}
}

func createValidZip(destZip string, files ...string) {
	f, err := os.Create(destZip)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		w, err := zw.Create("bundle/" + filepath.Base(path))
		if err != nil {
			panic(err)
		}
		_, _ = w.Write(data)
	}
}

func createMultiImageZip(destZip string) {
	f, err := os.Create(destZip)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for i := 1; i <= 3; i++ {
		var buf bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{uint8(i * 60), 120, 200, 255}}, image.Point{}, draw.Src)
		_ = png.Encode(&buf, img)

		w, _ := zw.Create(fmt.Sprintf("floor_%d.png", i))
		_, _ = w.Write(buf.Bytes())
	}
}

func createZipSlipFixture(destZip string) {
	f, err := os.Create(destZip)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	w, _ := zw.Create("../../traversal_target.txt")
	_, _ = w.Write([]byte("malicious payload outside root"))
}

func generateControlPointsManifest(path string) {
	manifest := map[string]interface{}{
		"source_image": "unreferenced_park.png",
		"dimensions":   map[string]int{"width": 400, "height": 300},
		"valid_affine_points": []map[string]interface{}{
			{"name": "TL", "image_x": 50.0, "image_y": 50.0, "longitude": -73.980, "latitude": 40.760},
			{"name": "TR", "image_x": 350.0, "image_y": 50.0, "longitude": -73.900, "latitude": 40.760},
			{"name": "BR", "image_x": 350.0, "image_y": 250.0, "longitude": -73.900, "latitude": 40.700},
			{"name": "BL", "image_x": 50.0, "image_y": 250.0, "longitude": -73.980, "latitude": 40.700},
		},
		"negative_cases": map[string]interface{}{
			"insufficient": []map[string]float64{
				{"image_x": 50.0, "image_y": 50.0, "longitude": -73.98, "latitude": 40.76},
			},
			"collinear": []map[string]float64{
				{"image_x": 50.0, "image_y": 50.0, "longitude": -73.98, "latitude": 40.76},
				{"image_x": 200.0, "image_y": 150.0, "longitude": -73.94, "latitude": 40.73},
				{"image_x": 350.0, "image_y": 250.0, "longitude": -73.90, "latitude": 40.70},
			},
			"out_of_bounds": []map[string]float64{
				{"image_x": 5000.0, "image_y": 50.0, "longitude": -73.98, "latitude": 40.76},
			},
			"invalid_latitude": []map[string]float64{
				{"image_x": 50.0, "image_y": 50.0, "longitude": -73.98, "latitude": 115.0},
			},
		},
	}
	bytes, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(path, bytes, 0644)
}

func generateManifestJSON(outDir string) {
	manifest := map[string]interface{}{
		"schema_version": "1.0.0",
		"fixtures": []map[string]interface{}{
			{
				"filename":              "ordinary_grid.png",
				"format":                "png",
				"width":                 400,
				"height":                300,
				"expected_georeferenced": false,
				"expected_validation":   "VALID",
				"intended_test_case":    "Ordinary PNG upload and display without georeferencing sidecars",
			},
			{
				"filename":              "ordinary_photo.jpg",
				"format":                "jpeg",
				"width":                 400,
				"height":                300,
				"expected_georeferenced": false,
				"expected_validation":   "VALID",
				"intended_test_case":    "Ordinary JPEG upload and decoding",
			},
			{
				"filename":              "georeferenced_campus.png",
				"format":                "png",
				"width":                 400,
				"height":                300,
				"expected_georeferenced": true,
				"expected_crs":          "EPSG:4326",
				"expected_bounds":       []float64{-73.985, 40.600, -73.785, 40.750},
				"expected_validation":   "VALID",
				"intended_test_case":    "Pre-georeferenced raster with world file (.pgw) and projection (.prj)",
			},
			{
				"filename":              "valid_bundle.zip",
				"format":                "zip",
				"expected_georeferenced": true,
				"expected_validation":   "VALID",
				"intended_test_case":    "ZIP bundle containing raster and sidecars",
			},
			{
				"filename":              "multi_images.zip",
				"format":                "zip",
				"expected_validation":   "VALID",
				"intended_test_case":    "ZIP containing multiple floor images",
			},
			{
				"filename":              "zip_slip_malicious.zip",
				"format":                "zip",
				"expected_validation":   "REJECT",
				"intended_test_case":    "Security check: path traversal in ZIP archive",
			},
			{
				"filename":              "truncated.png",
				"format":                "png",
				"expected_validation":   "REJECT",
				"intended_test_case":    "Corrupted PNG upload handling",
			},
			{
				"filename":              "text_as_png.png",
				"format":                "text/png",
				"expected_validation":   "REJECT",
				"intended_test_case":    "MIME spoofing rejection",
			},
			{
				"filename":              "unreferenced_park.png",
				"format":                "png",
				"width":                 400,
				"height":                300,
				"expected_georeferenced": false,
				"intended_test_case":    "Interactive control-point georeferencing source",
			},
		},
	}
	bytes, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "manifest.json"), bytes, 0644)
}

func generateReadme(outDir string) {
	readme := `# MapTiler MVP Deterministic Test Data

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
` + "```powershell\n" + `cd c:\Users\harma\OneDrive\Desktop\veracity-maps\maptiler-go
go run testdata/generate_testdata.go
` + "```\n"
	_ = os.WriteFile(filepath.Join(outDir, "README.md"), []byte(readme), 0644)
}
