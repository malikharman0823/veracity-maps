package gis

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseHexColor parses a hex color string like "#ffffff", "ffffff", or "#fff" into RGB components.
func ParseHexColor(hexStr string) (color.RGBA, error) {
	cleaned := strings.TrimSpace(hexStr)
	cleaned = strings.TrimPrefix(cleaned, "#")

	if len(cleaned) == 3 {
		var expanded strings.Builder
		for _, c := range cleaned {
			expanded.WriteRune(c)
			expanded.WriteRune(c)
		}
		cleaned = expanded.String()
	}

	if len(cleaned) != 6 {
		return color.RGBA{}, fmt.Errorf("INVALID_HEX_COLOR: Invalid hex color string: %s", hexStr)
	}

	val, err := strconv.ParseUint(cleaned, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("INVALID_HEX_COLOR: Invalid hex digits in: %s", hexStr)
	}

	return color.RGBA{
		R: uint8((val >> 16) & 0xFF),
		G: uint8((val >> 8) & 0xFF),
		B: uint8(val & 0xFF),
		A: 255,
	}, nil
}

// RemoveBackground processes an image and generates an RGBA image with the target color removed.
// It uses Euclidean color distance, antialiased edge softness, and color defringing.
func RemoveBackground(src image.Image, targetColor color.RGBA, threshold float64, edgeSoftness float64) *image.NRGBA {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	dst := image.NewNRGBA(image.Rect(0, 0, width, height))

	tr := float64(targetColor.R)
	tg := float64(targetColor.G)
	tb := float64(targetColor.B)

	thresh := math.Max(0.0, threshold)
	softness := math.Max(1.0, edgeSoftness)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := src.At(bounds.Min.X+x, bounds.Min.Y+y)
			r, g, b, a := c.RGBA()
			// RGBA() returns alpha-premultiplied values in [0, 65535]
			r8 := float64(r >> 8)
			g8 := float64(g >> 8)
			b8 := float64(b >> 8)
			origA8 := float64(a >> 8)

			// Un-premultiply if alpha > 0
			if origA8 > 0 && origA8 < 255 {
				r8 = math.Min(255.0, (r8*255.0)/origA8)
				g8 = math.Min(255.0, (g8*255.0)/origA8)
				b8 = math.Min(255.0, (b8*255.0)/origA8)
			}

			// Color distance in RGB
			diffR := r8 - tr
			diffG := g8 - tg
			diffB := b8 - tb
			dist := math.Sqrt((diffR*diffR + diffG*diffG + diffB*diffB) / 3.0)

			var newAlpha float64
			if dist <= thresh {
				newAlpha = 0.0
			} else if dist >= (thresh + softness) {
				newAlpha = 255.0
			} else {
				newAlpha = 255.0 * (dist - thresh) / softness
			}

			// Combine with existing alpha channel
			finalAlpha := math.Min(newAlpha, origA8)

			// Defringe semi-transparent edge pixels: remove background color halo
			outR := r8
			outG := g8
			outB := b8

			if finalAlpha > 0 && finalAlpha < 255 && dist > thresh && dist < (thresh+softness) {
				normA := math.Max(finalAlpha/255.0, 0.05)
				outR = math.Min(255.0, math.Max(0.0, (r8-(1.0-normA)*tr)/normA))
				outG = math.Min(255.0, math.Max(0.0, (g8-(1.0-normA)*tg)/normA))
				outB = math.Min(255.0, math.Max(0.0, (b8-(1.0-normA)*tb)/normA))
			}

			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Round(outR)),
				G: uint8(math.Round(outG)),
				B: uint8(math.Round(outB)),
				A: uint8(math.Round(finalAlpha)),
			})
		}
	}

	return dst
}

// GenerateRasterPreview creates base64 data URLs for the original image and background-removed preview.
func GenerateRasterPreview(filePath string, targetColor color.RGBA, threshold float64, edgeSoftness float64, maxDimension int) (string, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", fmt.Errorf("SOURCE_FILE_NOT_FOUND: %w", err)
	}
	defer file.Close()

	src, _, err := image.Decode(file)
	if err != nil {
		return "", "", fmt.Errorf("INVALID_IMAGE: Failed to decode image: %w", err)
	}

	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Scale down if larger than maxDimension
	scale := 1.0
	longest := math.Max(float64(w), float64(h))
	if longest > float64(maxDimension) && maxDimension > 0 {
		scale = float64(maxDimension) / longest
	}

	scaledW := int(math.Max(1.0, math.Round(float64(w)*scale)))
	scaledH := int(math.Max(1.0, math.Round(float64(h)*scale)))

	scaledSrc := image.NewNRGBA(image.Rect(0, 0, scaledW, scaledH))
	// Bilinear or nearest sampling for preview thumbnail
	for y := 0; y < scaledH; y++ {
		srcY := bounds.Min.Y + int(float64(y)/scale)
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < scaledW; x++ {
			srcX := bounds.Min.X + int(float64(x)/scale)
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			scaledSrc.Set(x, y, src.At(srcX, srcY))
		}
	}

	// 1. Original thumbnail as PNG data URL
	var origBuf bytes.Buffer
	if err := png.Encode(&origBuf, scaledSrc); err != nil {
		return "", "", fmt.Errorf("ENCODE_ERROR: %w", err)
	}
	originalDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(origBuf.Bytes())

	// 2. Processed thumbnail
	processed := RemoveBackground(scaledSrc, targetColor, threshold, edgeSoftness)
	var prevBuf bytes.Buffer
	if err := png.Encode(&prevBuf, processed); err != nil {
		return "", "", fmt.Errorf("ENCODE_ERROR: %w", err)
	}
	previewDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(prevBuf.Bytes())

	return originalDataURL, previewDataURL, nil
}

// ProcessRasterBackgroundRemoval removes the background color from a full raster image and writes an RGBA PNG.
func ProcessRasterBackgroundRemoval(inputPath, outputPath string, targetColor color.RGBA, threshold, edgeSoftness float64) error {
	file, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("SOURCE_FILE_NOT_FOUND: %w", err)
	}
	defer file.Close()

	src, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("INVALID_IMAGE: Failed to decode image: %w", err)
	}

	processed := RemoveBackground(src, targetColor, threshold, edgeSoftness)

	_ = os.MkdirAll(filepath.Dir(outputPath), 0755)
	tmpPath := outputPath + ".tmp"
	defer os.Remove(tmpPath)

	outF, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("STORAGE_ERROR: Failed to create output file: %w", err)
	}

	encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := encoder.Encode(outF, processed); err != nil {
		outF.Close()
		return fmt.Errorf("ENCODE_ERROR: Failed to encode PNG: %w", err)
	}
	outF.Close()

	_ = os.Remove(outputPath)
	if err := os.Rename(tmpPath, outputPath); err != nil {
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return fmt.Errorf("STORAGE_ERROR: Failed to read temp file: %w", err)
		}
		if writeErr := os.WriteFile(outputPath, data, 0644); writeErr != nil {
			return fmt.Errorf("STORAGE_ERROR: Failed to write output file: %w", writeErr)
		}
	}
	return nil
}

// ImageDimensions returns the pixel width and height of an image file.
func ImageDimensions(filePath string) (int, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
