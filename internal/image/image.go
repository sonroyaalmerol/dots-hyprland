// Package image provides image analysis utilities for region detection,
// least-busy-region finding, and text color detection using pure Go.
package image

import (
	"fmt"
	stdimage "image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
)

// Region represents a detected rectangular region in an image.
type Region struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// RegionHyprctl represents a region in hyprctl-compatible format.
type RegionHyprctl struct {
	At   [2]int `json:"at"`
	Size [2]int `json:"size"`
}

// LeastBusyResult is the output of FindLeastBusyRegion.
type LeastBusyResult struct {
	CenterX       int     `json:"center_x"`
	CenterY       int     `json:"center_y"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Variance      float64 `json:"variance"`
	DominantColor string  `json:"dominant_color"`
}

// LargestRegionResult is the output of FindLargestRegion.
type LargestRegionResult struct {
	CenterX       int     `json:"center_x"`
	CenterY       int     `json:"center_y"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Variance      float64 `json:"variance"`
	DominantColor string  `json:"dominant_color"`
}

// TextColorResult is the output of DetectTextColor.
type TextColorResult struct {
	Background string `json:"background"`
	Text       string `json:"text"`
}

// rgbToHex converts R, G, B uint8 values to a hex color string.
func rgbToHex(r, g, b uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// IsImageFile returns true if the file extension looks like an image.
func IsImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".bmp", ".svg", ".gif", ".tiff", ".tif":
		return true
	}
	return false
}

// loadImage loads an image from a file path, supporting JPEG, PNG, WebP,
// and falling back to imaging.Decode for other formats.
func loadImage(path string) (stdimage.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening image: %w", err)
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		img, err := jpeg.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decoding JPEG: %w", err)
		}
		return img, nil
	case ".png":
		img, err := png.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decoding PNG: %w", err)
		}
		return img, nil
	case ".webp":
		img, err := webp.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decoding WebP: %w", err)
		}
		return img, nil
	default:
		// Try imaging.Decode which supports more formats
		f.Seek(0, io.SeekStart)
		img, err := imaging.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("decoding image %s: %w", path, err)
		}
		return img, nil
	}
}

// decodeImage decodes image bytes, trying JPEG, PNG, WebP, then imaging.Decode.
func decodeImage(data []byte) (stdimage.Image, error) {
	// Try JPEG
	if img, err := jpeg.Decode(strings.NewReader(string(data))); err == nil {
		return img, nil
	}
	// Try PNG
	if img, err := png.Decode(strings.NewReader(string(data))); err == nil {
		return img, nil
	}
	// Try WebP
	if img, err := webp.Decode(strings.NewReader(string(data))); err == nil {
		return img, nil
	}
	// Try imaging (BMP, TIFF, etc.)
	if img, err := imaging.Decode(strings.NewReader(string(data))); err == nil {
		return img, nil
	}
	return nil, fmt.Errorf("could not decode image data: unsupported format")
}

// toNRGBA converts any stdimage.Image to *stdimage.NRGBA for consistent pixel access.
func toNRGBA(img stdimage.Image) *stdimage.NRGBA {
	if nrgba, ok := img.(*stdimage.NRGBA); ok {
		return nrgba
	}
	return imaging.Clone(img)
}

// grayscaleAt returns the luminance of a pixel (0-255).
func grayscaleAt(img *stdimage.NRGBA, x, y int) uint8 {
	c := img.NRGBAAt(x, y)
	// ITU-R BT.601 luma
	return uint8((19595*uint32(c.R) + 38470*uint32(c.G) + 7471*uint32(c.B)) >> 16)
}

// centerCropNRGBA crops the center of an image to targetW x targetH.
func centerCropNRGBA(img *stdimage.NRGBA, targetW, targetH int) *stdimage.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= targetW && h <= targetH {
		return img
	}
	x0 := max(0, (w-targetW)/2)
	y0 := max(0, (h-targetH)/2)
	x1 := min(w, x0+targetW)
	y1 := min(h, y0+targetH)
	cropped := stdimage.NewNRGBA(stdimage.Rect(0, 0, x1-x0, y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			cropped.Set(x-x0, y-y0, img.NRGBAAt(x, y))
		}
	}
	return cropped
}

// cropImage crops an image to the specified rectangle (x, y, w, h).
func cropImage(img stdimage.Image, x, y, w, h int) stdimage.Image {
	bounds := img.Bounds()
	// Clamp crop rect to image bounds
	x0 := bounds.Min.X + x
	y0 := bounds.Min.Y + y
	x1 := x0 + w
	y1 := y0 + h
	if x0 < bounds.Min.X {
		x0 = bounds.Min.X
	}
	if y0 < bounds.Min.Y {
		y0 = bounds.Min.Y
	}
	if x1 > bounds.Max.X {
		x1 = bounds.Max.X
	}
	if y1 > bounds.Max.Y {
		y1 = bounds.Max.Y
	}
	// Use imaging.Crop for easy cropping
	return imaging.Crop(img, stdimage.Rect(x0, y0, x1, y1))
}
