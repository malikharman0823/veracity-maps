package gis

import (
	"archive/zip"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"time"
)

// BasemapTilePackageConfig contains limits and URLs for packaging basemap tiles
type BasemapTilePackageConfig struct {
	BaseURL     string // e.g. "https://tile.openstreetmap.org"
	Attribution string
	MaxTiles    int
	UserAgent   string
}

// DefaultBasemapConfig returns the standard OpenStreetMap tile packaging config
func DefaultBasemapConfig() BasemapTilePackageConfig {
	return BasemapTilePackageConfig{
		BaseURL:     "https://tile.openstreetmap.org",
		Attribution: "© OpenStreetMap contributors",
		MaxTiles:    64, // Keep offline export compact and respect OSM tile servers
		UserAgent:   "VeracityMaps/2.0 (Offline Export Packager)",
	}
}

// PackageBasemapTiles fetches or generates basemap raster tiles into basemap/{z}/{x}/{y}.png
// in the provided ZIP archive for the specified geographic bounds and zoom range.
// If network is unavailable or times out, it falls back to creating clean offline reference grid tiles.
func PackageBasemapTiles(archive *zip.Writer, bounds []float64, minZoom, maxZoom int) (int, error) {
	if len(bounds) < 4 {
		return 0, fmt.Errorf("insufficient bounds for basemap tiles")
	}

	cfg := DefaultBasemapConfig()
	west, south, east, north := bounds[0], bounds[1], bounds[2], bounds[3]
	if west > east {
		west, east = east, west
	}
	if south > north {
		south, north = north, south
	}

	// Clamp zoom range to reasonable basemap range
	if minZoom < 1 {
		minZoom = 1
	}
	if maxZoom > 19 {
		maxZoom = 19
	}
	if minZoom > maxZoom {
		minZoom = maxZoom
	}

	// First calculate total tile budget
	type tileCoord struct {
		z, x, y int
	}
	var tilesToFetch []tileCoord

	for z := minZoom; z <= maxZoom; z++ {
		minX, maxY := LatLngToTile(south, west, z)
		maxX, minY := LatLngToTile(north, east, z)
		if minX > maxX {
			minX, maxX = maxX, minX
		}
		if minY > maxY {
			minY, maxY = maxY, minY
		}

		for x := minX; x <= maxX; x++ {
			for y := minY; y <= maxY; y++ {
				tilesToFetch = append(tilesToFetch, tileCoord{z, x, y})
			}
		}
	}

	// If tile count exceeds MaxTiles, subsample to the lower zooms or center
	if len(tilesToFetch) > cfg.MaxTiles {
		var clamped []tileCoord
		for _, tc := range tilesToFetch {
			if tc.z <= minZoom+1 {
				clamped = append(clamped, tc)
				if len(clamped) >= cfg.MaxTiles {
					break
				}
			}
		}
		if len(clamped) > 0 {
			tilesToFetch = clamped
		} else {
			tilesToFetch = tilesToFetch[:cfg.MaxTiles]
		}
	}

	client := &http.Client{
		Timeout: 2500 * time.Millisecond,
	}

	packagedCount := 0
	for _, tc := range tilesToFetch {
		entryName := fmt.Sprintf("basemap/%d/%d/%d.png", tc.z, tc.x, tc.y)
		entryWriter, err := archive.Create(entryName)
		if err != nil {
			continue
		}

		tileData, err := fetchOnlineTile(client, cfg.BaseURL, cfg.UserAgent, tc.z, tc.x, tc.y)
		if err == nil && len(tileData) > 0 {
			_, _ = entryWriter.Write(tileData)
			packagedCount++
		} else {
			// Generate offline cartographic fallback tile
			offlineTile := generateOfflineFallbackTile(tc.z, tc.x, tc.y)
			if err := png.Encode(entryWriter, offlineTile); err == nil {
				packagedCount++
			}
		}
	}

	return packagedCount, nil
}

func fetchOnlineTile(client *http.Client, baseURL, userAgent string, z, x, y int) ([]byte, error) {
	url := fmt.Sprintf("%s/%d/%d/%d.png", baseURL, z, x, y)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tile fetch status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// generateOfflineFallbackTile generates a clean 256x256 land/grid tile when offline
func generateOfflineFallbackTile(z, x, y int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	bgColor := color.RGBA{R: 242, G: 239, B: 233, A: 255} // OpenStreetMap land color
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// Draw subtle coordinate grid border
	gridColor := color.RGBA{R: 220, G: 216, B: 206, A: 255}
	for i := 0; i < 256; i++ {
		img.Set(i, 0, gridColor)
		img.Set(i, 255, gridColor)
		img.Set(0, i, gridColor)
		img.Set(255, i, gridColor)
	}

	return img
}
