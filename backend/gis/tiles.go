package gis

import (
	"archive/zip"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
)

// TileRange holds tile coordinate bounds for a specific zoom level
type TileRange struct {
	Zoom int
	MinX int
	MaxX int
	MinY int
	MaxY int
}

// LatLngToTile converts lat/lng in degrees to tile coordinates at zoom level z
func LatLngToTile(lat, lng float64, zoom int) (int, int) {
	n := math.Pow(2.0, float64(zoom))
	x := int(math.Floor((lng + 180.0) / 360.0 * n))

	latRad := lat * math.Pi / 180.0
	y := int(math.Floor((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n))

	maxIndex := int(n) - 1
	if x < 0 {
		x = 0
	} else if x > maxIndex {
		x = maxIndex
	}
	if y < 0 {
		y = 0
	} else if y > maxIndex {
		y = maxIndex
	}
	return x, y
}

// TileToLatLngBounds returns the [west, south, east, north] bounding box for tile x, y at zoom level z
func TileToLatLngBounds(x, y, zoom int) [4]float64 {
	n := math.Pow(2.0, float64(zoom))
	west := float64(x)/n*360.0 - 180.0
	east := float64(x+1)/n*360.0 - 180.0

	northRad := math.Atan(math.Sinh(math.Pi * (1.0 - 2.0*float64(y)/n)))
	southRad := math.Atan(math.Sinh(math.Pi * (1.0 - 2.0*float64(y+1)/n)))

	north := northRad * 180.0 / math.Pi
	south := southRad * 180.0 / math.Pi

	return [4]float64{west, south, east, north}
}

// DetermineTileZooms calculates reasonable min and max zoom levels for bounds and image size
func DetermineTileZooms(bounds []float64, width, height int) (minZoom, maxZoom int) {
	if len(bounds) < 4 {
		return 15, 17
	}
	west, south, east, north := bounds[0], bounds[1], bounds[2], bounds[3]
	degWidth := math.Abs(east - west)
	degHeight := math.Abs(north - south)
	maxSpan := math.Max(degWidth, degHeight)

	if maxSpan <= 0 {
		return 16, 18
	}

	// Calculate base zoom where the span fits into 1-2 tiles (256-512px)
	// 360 / 2^z ≈ maxSpan
	targetZoom := int(math.Round(math.Log2(360.0 / maxSpan)))
	if targetZoom < 0 {
		targetZoom = 0
	}
	if targetZoom > 22 {
		targetZoom = 22
	}

	// For floorplans/architectural drawings, zoom is typically 16-20
	minZoom = targetZoom
	if minZoom < 14 {
		minZoom = 14
	}
	maxZoom = minZoom + 2
	if maxZoom > 21 {
		maxZoom = 21
	}
	if minZoom > maxZoom {
		minZoom = maxZoom - 1
	}
	return minZoom, maxZoom
}

// GenerateRasterTiles renders XYZ tiles from a georeferenced raster directly into a ZIP archive.
// Returns the list of zoom levels generated and any error.
func GenerateRasterTiles(archive *zip.Writer, imagePath string, bounds []float64) ([]int, error) {
	if len(bounds) < 4 {
		return nil, fmt.Errorf("insufficient bounds for tiling")
	}
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed opening image for tiling: %w", err)
	}
	defer f.Close()

	srcImg, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed decoding image for tiling: %w", err)
	}

	srcBounds := srcImg.Bounds()
	imgW := srcBounds.Dx()
	imgH := srcBounds.Dy()
	if imgW <= 0 || imgH <= 0 {
		return nil, fmt.Errorf("invalid image dimensions")
	}

	minZoom, maxZoom := DetermineTileZooms(bounds, imgW, imgH)
	west, south, east, north := bounds[0], bounds[1], bounds[2], bounds[3]

	// Ensure bounds are non-inverted
	if west > east {
		west, east = east, west
	}
	if south > north {
		south, north = north, south
	}

	var generatedZooms []int
	tileCount := 0
	const maxTotalTiles = 250 // Safeguard against memory or file explosions

	for z := minZoom; z <= maxZoom; z++ {
		minX, maxY := LatLngToTile(south, west, z)
		maxX, minY := LatLngToTile(north, east, z)

		if minX > maxX {
			minX, maxX = maxX, minX
		}
		if minY > maxY {
			minY, maxY = maxY, minY
		}

		// Check if tile count for this zoom is reasonable
		countForZoom := (maxX - minX + 1) * (maxY - minY + 1)
		if countForZoom > 64 || tileCount+countForZoom > maxTotalTiles {
			// If at least one zoom level was generated, stop here
			if len(generatedZooms) > 0 {
				break
			}
			// If not even one zoom level generated yet, clamp range to center 2x2
			midX := (minX + maxX) / 2
			midY := (minY + maxY) / 2
			minX, maxX = midX, midX
			minY, maxY = midY, midY
		}

		zoomHasTiles := false
		for x := minX; x <= maxX; x++ {
			for y := minY; y <= maxY; y++ {
				tileBbox := TileToLatLngBounds(x, y, z)
				tWest, tSouth, tEast, tNorth := tileBbox[0], tileBbox[1], tileBbox[2], tileBbox[3]

				// Calculate intersection between tile and dataset bounds
				interWest := math.Max(west, tWest)
				interEast := math.Min(east, tEast)
				interSouth := math.Max(south, tSouth)
				interNorth := math.Min(north, tNorth)

				if interWest >= interEast || interSouth >= interNorth {
					continue
				}

				// Map tile intersection to normalized [0, 1] tile pixel coordinates
				tilePixelX0 := int(math.Floor((interWest - tWest) / (tEast - tWest) * 256.0))
				tilePixelX1 := int(math.Ceil((interEast - tWest) / (tEast - tWest) * 256.0))
				tilePixelY0 := int(math.Floor((tNorth - interNorth) / (tNorth - tSouth) * 256.0))
				tilePixelY1 := int(math.Ceil((tNorth - interSouth) / (tNorth - tSouth) * 256.0))

				if tilePixelX0 < 0 {
					tilePixelX0 = 0
				}
				if tilePixelX1 > 256 {
					tilePixelX1 = 256
				}
				if tilePixelY0 < 0 {
					tilePixelY0 = 0
				}
				if tilePixelY1 > 256 {
					tilePixelY1 = 256
				}

				// Map dataset intersection to source image pixels
				srcPixelX0 := int(math.Floor((interWest - west) / (east - west) * float64(imgW)))
				srcPixelX1 := int(math.Ceil((interEast - west) / (east - west) * float64(imgW)))
				srcPixelY0 := int(math.Floor((north - interNorth) / (north - south) * float64(imgH)))
				srcPixelY1 := int(math.Ceil((north - interSouth) / (north - south) * float64(imgH)))

				if srcPixelX0 < 0 {
					srcPixelX0 = 0
				}
				if srcPixelX1 > imgW {
					srcPixelX1 = imgW
				}
				if srcPixelY0 < 0 {
					srcPixelY0 = 0
				}
				if srcPixelY1 > imgH {
					srcPixelY1 = imgH
				}

				if srcPixelX1 <= srcPixelX0 || srcPixelY1 <= srcPixelY0 || tilePixelX1 <= tilePixelX0 || tilePixelY1 <= tilePixelY0 {
					continue
				}

				// Render 256x256 RGBA tile
				tileImg := image.NewRGBA(image.Rect(0, 0, 256, 256))
				rescaleSubImage(tileImg, tilePixelX0, tilePixelY0, tilePixelX1, tilePixelY1,
					srcImg, srcPixelX0, srcPixelY0, srcPixelX1, srcPixelY1)

				// Create ZIP entry: "{z}/{x}/{y}.png"
				tileEntryName := fmt.Sprintf("%d/%d/%d.png", z, x, y)
				entryWriter, err := archive.Create(tileEntryName)
				if err != nil {
					continue
				}

				if err := png.Encode(entryWriter, tileImg); err == nil {
					tileCount++
					zoomHasTiles = true
				}
			}
		}

		if zoomHasTiles {
			generatedZooms = append(generatedZooms, z)
		}
	}

	return generatedZooms, nil
}

// rescaleSubImage maps a source sub-rectangle into a tile destination sub-rectangle using nearest-neighbor scaling
func rescaleSubImage(dst *image.RGBA, dstX0, dstY0, dstX1, dstY1 int,
	src image.Image, srcX0, srcY0, srcX1, srcY1 int) {
	dstW := dstX1 - dstX0
	dstH := dstY1 - dstY0
	srcW := srcX1 - srcX0
	srcH := srcY1 - srcY0

	if dstW <= 0 || dstH <= 0 || srcW <= 0 || srcH <= 0 {
		return
	}

	for dy := dstY0; dy < dstY1; dy++ {
		sy := srcY0 + int(float64(dy-dstY0)/float64(dstH)*float64(srcH))
		if sy >= srcY1 {
			sy = srcY1 - 1
		}
		for dx := dstX0; dx < dstX1; dx++ {
			sx := srcX0 + int(float64(dx-dstX0)/float64(dstW)*float64(srcW))
			if sx >= srcX1 {
				sx = srcX1 - 1
			}
			dst.Set(dx, dy, src.At(sx, sy))
		}
	}
	_ = draw.Draw // ensure draw package is kept
}
