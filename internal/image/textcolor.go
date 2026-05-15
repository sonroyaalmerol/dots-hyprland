package image

import (
	"encoding/json"
	"fmt"
	stdimage "image"
	"io"
	"math"
	"math/rand"
	"slices"
	"sort"
)

// DetectTextColorFromPath analyzes an image file and returns the dominant
// background and text colors. It reads corner pixels for background,
// then finds the 95th-percentile-distant pixels for text color.
func DetectTextColorFromPath(imagePath string) (*TextColorResult, error) {
	img, err := loadImage(imagePath)
	if err != nil {
		return nil, fmt.Errorf("could not load image: %w", err)
	}
	return detectTextColor(img)
}

// DetectTextColorFromPathCropped loads an image, crops it, and detects text colors.
func DetectTextColorFromPathCropped(imagePath string, x, y, w, h int) (*TextColorResult, error) {
	img, err := loadImage(imagePath)
	if err != nil {
		return nil, fmt.Errorf("could not load image: %w", err)
	}
	cropped := cropImage(img, x, y, w, h)
	return detectTextColor(cropped)
}

// DetectTextColorFromReader analyzes image data from a reader (e.g. stdin)
// and returns the dominant background and text colors.
func DetectTextColorFromReader(r io.Reader) (*TextColorResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading image data: %w", err)
	}
	return DetectTextColorFromBytes(data)
}

// DetectTextColorFromBytes analyzes image bytes and returns colors.
func DetectTextColorFromBytes(data []byte) (*TextColorResult, error) {
	img, err := decodeImage(data)
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}
	return detectTextColor(img)
}

// detectTextColor is the core algorithm, ported from text_color.py.
// It samples corner pixels for background color, then finds the 95th-percentile
// distant pixels for text color.
func detectTextColor(img stdimage.Image) (*TextColorResult, error) {
	nrgba := toNRGBA(img)
	w, h := nrgba.Bounds().Dx(), nrgba.Bounds().Dy()

	if w == 0 || h == 0 {
		return nil, fmt.Errorf("image has zero dimensions")
	}

	// 1. Sample corner pixels for background color
	type rgb struct{ r, g, b uint8 }
	corners := []rgb{
		pixelRGB(nrgba, 0, 0),
		pixelRGB(nrgba, w-1, 0),
		pixelRGB(nrgba, 0, h-1),
		pixelRGB(nrgba, w-1, h-1),
	}

	// 2. Median of corner pixels for background
	bgR := median4(corners[0].r, corners[1].r, corners[2].r, corners[3].r)
	bgG := median4(corners[0].g, corners[1].g, corners[2].g, corners[3].g)
	bgB := median4(corners[0].b, corners[1].b, corners[2].b, corners[3].b)

	// 3. Compute distances from background for all pixels (downsample large images)
	totalPixels := w * h
	sampleRate := 1
	if totalPixels > 500000 {
		sampleRate = max(totalPixels/500000, 1)
	}

	type pixelDist struct {
		r, g, b uint8
		dist    float64
	}
	var pixelData []pixelDist
	rng := rand.New(rand.NewSource(42))

	for y := range h {
		for x := range w {
			// Deterministic sampling for large images
			if sampleRate > 1 && (x+y*w)%sampleRate != 0 {
				// Also include some random pixels for better coverage
				if rng.Intn(sampleRate) != 0 {
					continue
				}
			}
			c := nrgba.NRGBAAt(x, y)
			dr := float64(c.R) - float64(bgR)
			dg := float64(c.G) - float64(bgG)
			db := float64(c.B) - float64(bgB)
			dist := math.Sqrt(dr*dr + dg*dg + db*db)
			pixelData = append(pixelData, pixelDist{c.R, c.G, c.B, dist})
		}
	}

	// 4. Compute 95th percentile threshold
	dists := make([]float64, len(pixelData))
	for i, p := range pixelData {
		dists[i] = p.dist
	}
	threshold := percentile(dists, 95.0)

	// 5. Collect text pixels (above threshold)
	var textR, textG, textB []uint8
	for _, p := range pixelData {
		if p.dist >= threshold {
			textR = append(textR, p.r)
			textG = append(textG, p.g)
			textB = append(textB, p.b)
		}
	}

	var textHex string
	if len(textR) == 0 {
		textHex = "#ffffff"
	} else {
		textHex = rgbToHex(medianSlice(textR), medianSlice(textG), medianSlice(textB))
	}

	return &TextColorResult{
		Background: rgbToHex(bgR, bgG, bgB),
		Text:       textHex,
	}, nil
}

// pixelRGB returns the RGB values at position (x, y).
func pixelRGB(img *stdimage.NRGBA, x, y int) struct{ r, g, b uint8 } {
	c := img.NRGBAAt(x, y)
	return struct{ r, g, b uint8 }{c.R, c.G, c.B}
}

// median4 computes the median of 4 uint8 values.
func median4(a, b, c, d uint8) uint8 {
	vals := []uint8{a, b, c, d}
	slices.Sort(vals)
	return (vals[1] + vals[2]) / 2
}

// medianSlice computes the median of a slice of uint8 values.
func medianSlice(vals []uint8) uint8 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sorted := make([]uint8, n)
	copy(sorted, vals)
	slices.Sort(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// percentile computes the given percentile from a sorted slice of float64 values.
func percentile(values []float64, p float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	rank := (p / 100.0) * float64(n-1)
	lower := int(math.Floor(rank))
	upper := lower + 1
	if upper >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}

// DetectTextColorJSON runs DetectTextColorFromPath and returns JSON.
func DetectTextColorJSON(imagePath string) (string, error) {
	result, err := DetectTextColorFromPath(imagePath)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
