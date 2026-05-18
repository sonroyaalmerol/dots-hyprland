package image

import (
	"encoding/json"
	"fmt"
	"image"
	"math"

	"github.com/disintegration/imaging"
)

// LeastBusyRegionParams controls least-busy-region detection.
type LeastBusyRegionParams struct {
	ImagePath         string
	RegionWidth       int
	RegionHeight      int
	ScreenWidth       int
	ScreenHeight      int
	Stride            int
	ScreenMode        string // "fill" or "fit"
	HorizontalPadding int
	VerticalPadding   int
	Busiest           bool
	VisualOutput      bool
}

// DefaultLeastBusyRegionParams returns sensible defaults matching the Python script.
func DefaultLeastBusyRegionParams(imagePath string) LeastBusyRegionParams {
	return LeastBusyRegionParams{
		ImagePath:         imagePath,
		RegionWidth:       300,
		RegionHeight:      300,
		ScreenWidth:       1920,
		ScreenHeight:      1080,
		Stride:            10,
		ScreenMode:        "fill",
		HorizontalPadding: 200,
		VerticalPadding:   200,
	}
}

// findLeastBusyRegion finds the least (or most) busy region in an image
// using integral images for fast variance computation.
func findLeastBusyRegion(params LeastBusyRegionParams) (*LeastBusyResult, error) {
	srcImg, err := loadImage(params.ImagePath)
	if err != nil {
		return nil, fmt.Errorf("could not load image: %w", err)
	}

	origBounds := srcImg.Bounds()
	origW, origH := origBounds.Dx(), origBounds.Dy()

	// Convert to grayscale NRGBA
	var processed image.Image
	if params.ScreenWidth > 0 && params.ScreenHeight > 0 {
		scaleW := float64(params.ScreenWidth) / float64(origW)
		scaleH := float64(params.ScreenHeight) / float64(origH)
		var scale float64
		if params.ScreenMode == "fill" {
			scale = math.Max(scaleW, scaleH)
		} else {
			scale = math.Min(scaleW, scaleH)
		}
		newW := int(float64(origW) * scale)
		newH := int(float64(origH) * scale)
		resized := imaging.Resize(srcImg, newW, newH, imaging.Lanczos)
		processed = imaging.CropCenter(resized, params.ScreenWidth, params.ScreenHeight)
	} else {
		processed = srcImg
	}

	nrgba := toNRGBA(processed)
	w, h := nrgba.Bounds().Dx(), nrgba.Bounds().Dy()

	// Build grayscale buffer
	gray := make([][]float64, h)
	for y := range h {
		gray[y] = make([]float64, w)
		for x := range w {
			gray[y][x] = float64(grayscaleAt(nrgba, x, y))
		}
	}

	// Build integral images for fast variance computation
	stride := max(params.Stride, 1)
	rw := params.RegionWidth
	rh := params.RegionHeight

	// Adjust padding
	hp := params.HorizontalPadding
	vp := params.VerticalPadding
	if hp*2 >= w {
		hp = max(0, (w-1)/2)
	}
	if vp*2 >= h {
		vp = max(0, (h-1)/2)
	}

	// Clamp region size
	maxRW := w - 2*hp
	maxRH := h - 2*vp
	if maxRW <= 0 || maxRH <= 0 {
		return nil, fmt.Errorf("image too small for the specified padding")
	}
	rw = min(rw, maxRW)
	rh = min(rh, maxRH)

	// Integral and integral-of-squares
	integral := make([][]float64, h+1)
	integralSq := make([][]float64, h+1)
	for i := range integral {
		integral[i] = make([]float64, w+1)
		integralSq[i] = make([]float64, w+1)
	}

	for y := range h {
		for x := range w {
			v := gray[y][x]
			integral[y+1][x+1] = v + integral[y][x+1] + integral[y+1][x] - integral[y][x]
			integralSq[y+1][x+1] = v*v + integralSq[y][x+1] + integralSq[y+1][x] - integralSq[y][x]
		}
	}

	regionSum := func(ii [][]float64, x1, y1, x2, y2 int) float64 {
		s := ii[y2+1][x2+1]
		if x1 > 0 {
			s -= ii[y2+1][x1]
		}
		if y1 > 0 {
			s -= ii[y1][x2+1]
		}
		if x1 > 0 && y1 > 0 {
			s += ii[y1][x1]
		}
		return s
	}

	// Sliding window to find min/max variance
	var minVar, maxVar float64
	var minX, minY, maxX, maxY int
	minVar = -1
	maxVar = -1
	area := float64(rw * rh)

	xStart := hp
	yStart := vp
	xEnd := w - rw - hp + 1
	yEnd := h - rh - vp + 1
	if xEnd < xStart {
		xEnd = xStart
	}
	if yEnd < yStart {
		yEnd = yStart
	}

	for y := yStart; y <= yEnd; y += stride {
		for x := xStart; x <= xEnd; x += stride {
			s := regionSum(integral, x, y, x+rw-1, y+rh-1)
			s2 := regionSum(integralSq, x, y, x+rw-1, y+rh-1)
			mean := s / area
			variance := s2/area - mean*mean
			if minVar < 0 || variance < minVar {
				minVar = variance
				minX, minY = x, y
			}
			if maxVar < 0 || variance > maxVar {
				maxVar = variance
				maxX, maxY = x, y
			}
		}
	}

	var cx, cy int
	var v float64
	if params.Busiest {
		cx = maxX + rw/2
		cy = maxY + rh/2
		v = maxVar
	} else {
		cx = minX + rw/2
		cy = minY + rh/2
		v = minVar
	}

	// Scale back to original image coordinates
	scaleBackW := float64(origW) / float64(w)
	scaleBackH := float64(origH) / float64(h)
	cx = int(float64(cx) * scaleBackW)
	cy = int(float64(cy) * scaleBackH)

	// Get dominant color from the region
	dominantColor, err := getDominantColor(params.ImagePath, minX, minY, rw, rh,
		params.ScreenWidth, params.ScreenHeight, params.ScreenMode)
	if err != nil {
		dominantColor = "#000000"
	}

	return &LeastBusyResult{
		CenterX:       cx,
		CenterY:       cy,
		Width:         rw,
		Height:        rh,
		Variance:      v,
		DominantColor: dominantColor,
	}, nil
}

// getDominantColor extracts the dominant color from a region of the image using k-means.
func getDominantColor(imagePath string, x, y, w, h, screenWidth, screenHeight int, screenMode string) (string, error) {
	srcImg, err := loadImage(imagePath)
	if err != nil {
		return "#000000", fmt.Errorf("could not load image: %w", err)
	}

	origBounds := srcImg.Bounds()
	origW, origH := origBounds.Dx(), origBounds.Dy()

	// Scale image same as the region computation
	var scaled image.Image
	if screenWidth > 0 && screenHeight > 0 {
		scaleW := float64(screenWidth) / float64(origW)
		scaleH := float64(screenHeight) / float64(origH)
		var scale float64
		if screenMode == "fill" {
			scale = math.Max(scaleW, scaleH)
		} else {
			scale = math.Min(scaleW, scaleH)
		}
		newW := int(float64(origW) * scale)
		newH := int(float64(origH) * scale)
		resized := imaging.Resize(srcImg, newW, newH, imaging.Lanczos)
		scaled = imaging.CropCenter(resized, screenWidth, screenHeight)
	} else {
		scaled = srcImg
	}

	nrgba := toNRGBA(scaled)

	// Clamp region bounds
	rx := max(0, x)
	ry := max(0, y)
	rw := max(1, min(w, nrgba.Bounds().Dx()-rx))
	rh := max(1, min(h, nrgba.Bounds().Dy()-ry))

	// Sample pixels from the region for k-means
	// Use stride to avoid sampling too many pixels
	stride := max(1, max(rw, rh)/50)
	var pixels []struct{ r, g, b float64 }
	for py := ry; py < ry+rh && py < nrgba.Bounds().Dy(); py += stride {
		for px := rx; px < rx+rw && px < nrgba.Bounds().Dx(); px += stride {
			c := nrgba.NRGBAAt(px, py)
			pixels = append(pixels, struct{ r, g, b float64 }{
				float64(c.R), float64(c.G), float64(c.B),
			})
		}
	}

	if len(pixels) == 0 {
		return "#000000", nil
	}

	// K-means with k=3
	centers := kmeansColors(pixels, 3, 10)

	// Find dominant cluster by counting nearest pixels
	counts := make([]int, len(centers))
	for _, p := range pixels {
		nearest := 0
		minDist := math.MaxFloat64
		for i, c := range centers {
			d := (p.r-c.r)*(p.r-c.r) + (p.g-c.g)*(p.g-c.g) + (p.b-c.b)*(p.b-c.b)
			if d < minDist {
				minDist = d
				nearest = i
			}
		}
		counts[nearest]++
	}

	dominant := 0
	maxCount := 0
	for i, c := range counts {
		if c > maxCount {
			maxCount = c
			dominant = i
		}
	}

	return rgbToHex(
		clampUint8(centers[dominant].r),
		clampUint8(centers[dominant].g),
		clampUint8(centers[dominant].b),
	), nil
}

// kmeansColors performs k-means clustering on color pixels.
func kmeansColors(pixels []struct{ r, g, b float64 }, k, iterations int) []struct{ r, g, b float64 } {
	n := len(pixels)
	if n == 0 || k <= 0 {
		return nil
	}
	if k > n {
		k = n
	}

	// Initialize centers from evenly spaced pixels
	centers := make([]struct{ r, g, b float64 }, k)
	for i := range k {
		idx := i * n / k
		centers[i] = pixels[idx]
	}

	labels := make([]int, n)

	for range iterations {
		// Assign each pixel to nearest center
		for i, p := range pixels {
			minDist := math.MaxFloat64
			for j, c := range centers {
				d := (p.r-c.r)*(p.r-c.r) + (p.g-c.g)*(p.g-c.g) + (p.b-c.b)*(p.b-c.b)
				if d < minDist {
					minDist = d
					labels[i] = j
				}
			}
		}

		// Update centers
		sums := make([]struct{ r, g, b float64 }, k)
		counts := make([]int, k)
		for i, p := range pixels {
			l := labels[i]
			sums[l].r += p.r
			sums[l].g += p.g
			sums[l].b += p.b
			counts[l]++
		}
		for j := range k {
			if counts[j] > 0 {
				centers[j].r = sums[j].r / float64(counts[j])
				centers[j].g = sums[j].g / float64(counts[j])
				centers[j].b = sums[j].b / float64(counts[j])
			}
		}
	}

	return centers
}

// clampUint8 clamps a float64 to [0, 255] and converts to uint8.
func clampUint8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// FindLeastBusyRegionJSON runs FindLeastBusyRegion and returns JSON output.
func FindLeastBusyRegionJSON(params LeastBusyRegionParams) (string, error) {
	result, err := findLeastBusyRegion(params)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
