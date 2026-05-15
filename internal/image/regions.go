package image

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/disintegration/imaging"
)

// FindRegionsParams controls region detection parameters.
type FindRegionsParams struct {
	ImagePath    string
	MinWidth     int
	MinHeight    int
	MaxWidth     *int
	MaxHeight    *int
	Quality      bool
	K            int
	MinSize      int
	Sigma        float64
	ResizeFactor float64
	Hyprctl      bool
	Single       bool
	DebugOutput  string
}

// DefaultFindRegionsParams returns params with sensible defaults.
func DefaultFindRegionsParams(imagePath string) FindRegionsParams {
	return FindRegionsParams{
		ImagePath:    imagePath,
		MinWidth:     200,
		MinHeight:    100,
		K:            3000,
		MinSize:      50,
		Sigma:        0.6,
		ResizeFactor: 0.1,
	}
}

// iou computes intersection over union for two regions.
func iou(a, b Region) float64 {
	xA := math.Max(float64(a.X), float64(b.X))
	yA := math.Max(float64(a.Y), float64(b.Y))
	xB := math.Min(float64(a.X+a.Width), float64(b.X+b.Width))
	yB := math.Min(float64(a.Y+a.Height), float64(b.Y+b.Height))
	interW := math.Max(0, xB-xA)
	interH := math.Max(0, yB-yA)
	interArea := interW * interH
	aArea := float64(a.Width * a.Height)
	bArea := float64(b.Width * b.Height)
	denom := aArea + bArea - interArea
	if denom <= 0 {
		return 0
	}
	return interArea / denom
}

// nms applies non-maximum suppression to remove overlapping regions.
func nms(regions []Region, threshold float64) []Region {
	sort.Slice(regions, func(i, j int) bool {
		aArea := regions[i].Width * regions[i].Height
		bArea := regions[j].Width * regions[j].Height
		return aArea > bArea
	})
	var keep []Region
	for len(regions) > 0 {
		current := regions[0]
		regions = regions[1:]
		keep = append(keep, current)
		var filtered []Region
		for _, r := range regions {
			if iou(current, r) < threshold {
				filtered = append(filtered, r)
			}
		}
		regions = filtered
	}
	return keep
}

// FindRegions detects regions of interest in an image using pure Go
// edge detection + connected component labeling.
func FindRegions(params FindRegionsParams) ([]Region, error) {
	srcImg, err := loadImage(params.ImagePath)
	if err != nil {
		return nil, fmt.Errorf("could not load image: %w", err)
	}

	origBounds := srcImg.Bounds()
	origW, origH := origBounds.Dx(), origBounds.Dy()

	// Resize for faster processing
	var processImg image.Image
	if params.ResizeFactor > 0 && params.ResizeFactor != 1.0 {
		newW := int(float64(origW) * params.ResizeFactor)
		newH := int(float64(origH) * params.ResizeFactor)
		if newW < 1 {
			newW = 1
		}
		if newH < 1 {
			newH = 1
		}
		processImg = imaging.Resize(srcImg, newW, newH, imaging.Lanczos)
	} else {
		processImg = srcImg
	}

	nrgba := toNRGBA(processImg)
	w, h := nrgba.Bounds().Dx(), nrgba.Bounds().Dy()

	// Convert to grayscale
	gray := make([][]float64, h)
	for y := range h {
		gray[y] = make([]float64, w)
		for x := range w {
			gray[y][x] = float64(grayscaleAt(nrgba, x, y))
		}
	}

	// Gaussian blur
	kernelSize := max(params.K/500, 1)
	if kernelSize%2 == 0 {
		kernelSize++
	}
	blurred := gaussianBlur2D(gray, w, h, kernelSize, params.Sigma)

	// Sobel edge detection
	edges := sobelEdges(blurred, w, h)

	// Threshold edges (Canny-like: use high threshold)
	edgeThreshold := 30.0
	binary := make([][]bool, h)
	for y := range h {
		binary[y] = make([]bool, w)
		for x := range w {
			binary[y][x] = edges[y][x] > edgeThreshold
		}
	}

	// Dilate to close gaps
	dilated := dilate2D(binary, w, h, 3)

	// Connected component labeling (flood fill)
	labels := connectedComponents(dilated, w, h)

	// Extract bounding boxes from components
	scale := 1.0 / params.ResizeFactor
	if params.ResizeFactor == 0 || params.ResizeFactor == 1.0 {
		scale = 1.0
	}

	compBounds := make(map[int]struct{ x1, y1, x2, y2 int })
	for y := range h {
		for x := range w {
			label := labels[y][x]
			if label == 0 {
				continue
			}
			b, ok := compBounds[label]
			if !ok {
				b = struct{ x1, y1, x2, y2 int }{x, y, x, y}
			} else {
				if x < b.x1 {
					b.x1 = x
				}
				if y < b.y1 {
					b.y1 = y
				}
				if x > b.x2 {
					b.x2 = x
				}
				if y > b.y2 {
					b.y2 = y
				}
			}
			compBounds[label] = b
		}
	}

	var regions []Region
	for _, b := range compBounds {
		rw := int(float64(b.x2-b.x1+1) * scale)
		rh := int(float64(b.y2-b.y1+1) * scale)
		rx := int(float64(b.x1) * scale)
		ry := int(float64(b.y1) * scale)

		// Filter out full-image regions
		if rw == origW && rh == origH && rx == 0 && ry == 0 {
			continue
		}
		// Filter small regions
		if rw < params.MinWidth || rh < params.MinHeight {
			continue
		}
		if params.MaxWidth != nil && rw >= *params.MaxWidth {
			continue
		}
		if params.MaxHeight != nil && rh >= *params.MaxHeight {
			continue
		}
		regions = append(regions, Region{X: rx, Y: ry, Width: rw, Height: rh})
	}

	// Apply NMS to remove overlapping regions
	regions = nms(regions, 0.7)

	// Single mode: keep only the largest region
	if params.Single && len(regions) > 0 {
		largest := regions[0]
		for _, r := range regions[1:] {
			if r.Width*r.Height > largest.Width*largest.Height {
				largest = r
			}
		}
		regions = []Region{largest}
	}

	// Draw debug output if requested
	if params.DebugOutput != "" {
		drawDebugRegions(params.ImagePath, regions, params.DebugOutput)
	}

	return regions, nil
}

// RegionsToHyprctl converts regions to hyprctl-compatible format.
func RegionsToHyprctl(regions []Region) []RegionHyprctl {
	result := make([]RegionHyprctl, len(regions))
	for i, r := range regions {
		result[i] = RegionHyprctl{
			At:   [2]int{r.X, r.Y},
			Size: [2]int{r.Width, r.Height},
		}
	}
	return result
}

// FindRegionsJSON runs FindRegions and returns JSON output.
func FindRegionsJSON(params FindRegionsParams) (string, error) {
	regions, err := FindRegions(params)
	if err != nil {
		return "", err
	}
	if params.Hyprctl {
		data, err := json.Marshal(RegionsToHyprctl(regions))
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	data, err := json.Marshal(regions)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// drawDebugRegions draws region rectangles on the image and saves it.
func drawDebugRegions(imagePath string, regions []Region, outputPath string) {
	srcImg, err := loadImage(imagePath)
	if err != nil {
		return
	}
	dst := toNRGBA(imaging.Clone(srcImg))
	for _, r := range regions {
		drawRectNRGBA(dst, r.X, r.Y, r.Width, r.Height, color.NRGBA{R: 255, G: 0, B: 0, A: 255}, 2)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return
	}
	defer f.Close()
	switch {
	case strings.HasSuffix(strings.ToLower(outputPath), ".png"):
		png.Encode(f, dst)
	default:
		jpeg.Encode(f, dst, &jpeg.Options{Quality: 90})
	}
}

// drawRectNRGBA draws a rectangle outline directly on an *image.NRGBA.
func drawRectNRGBA(img *image.NRGBA, x, y, w, h int, c color.NRGBA, thickness int) {
	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()
	for t := range thickness {
		// Top & bottom edges
		for dx := range w {
			for _, py := range []int{y + t, y + h - 1 - t} {
				px := x + dx
				if px >= 0 && px < imgW && py >= 0 && py < imgH {
					img.SetNRGBA(px, py, c)
				}
			}
		}
		// Left & right edges
		for dy := range h {
			for _, px := range []int{x + t, x + w - 1 - t} {
				py := y + dy
				if px >= 0 && px < imgW && py >= 0 && py < imgH {
					img.SetNRGBA(px, py, c)
				}
			}
		}
	}
}

// --- Pure Go image processing primitives ---

// gaussianBlur2D applies a Gaussian blur to a 2D grayscale buffer.
func gaussianBlur2D(data [][]float64, w, h, kernelSize int, sigma float64) [][]float64 {
	if kernelSize < 3 {
		kernelSize = 3
	}
	if sigma <= 0 {
		sigma = 0.6
	}

	// Generate 1D Gaussian kernel
	half := kernelSize / 2
	kernel := make([]float64, kernelSize)
	sum := 0.0
	for i := range kernel {
		x := float64(i - half)
		kernel[i] = math.Exp(-(x*x)/(2*sigma*sigma)) / (sigma * math.Sqrt(2*math.Pi))
		sum += kernel[i]
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	// Separable: horizontal pass then vertical pass
	temp := make([][]float64, h)
	for y := range h {
		temp[y] = make([]float64, w)
		for x := range w {
			v := 0.0
			for k := range kernel {
				sx := max(x+k-half, 0)
				if sx >= w {
					sx = w - 1
				}
				v += data[y][sx] * kernel[k]
			}
			temp[y][x] = v
		}
	}

	result := make([][]float64, h)
	for y := range h {
		result[y] = make([]float64, w)
		for x := range w {
			v := 0.0
			for k := range kernel {
				sy := max(y+k-half, 0)
				if sy >= h {
					sy = h - 1
				}
				v += temp[sy][x] * kernel[k]
			}
			result[y][x] = v
		}
	}
	return result
}

// sobelEdges computes edge magnitude using Sobel operators.
func sobelEdges(data [][]float64, w, h int) [][]float64 {
	result := make([][]float64, h)
	for y := range h {
		result[y] = make([]float64, w)
		for x := range w {
			// Skip borders
			if x == 0 || x == w-1 || y == 0 || y == h-1 {
				result[y][x] = 0
				continue
			}
			// Sobel X
			gx := -data[y-1][x-1] - 2*data[y][x-1] - data[y+1][x-1] +
				data[y-1][x+1] + 2*data[y][x+1] + data[y+1][x+1]
			// Sobel Y
			gy := -data[y-1][x-1] - 2*data[y-1][x] - data[y-1][x+1] +
				data[y+1][x-1] + 2*data[y+1][x] + data[y+1][x+1]
			result[y][x] = math.Sqrt(gx*gx + gy*gy)
		}
	}
	return result
}

// dilate2D applies morphological dilation to a binary image.
func dilate2D(data [][]bool, w, h, kernelSize int) [][]bool {
	half := kernelSize / 2
	result := make([][]bool, h)
	for y := range h {
		result[y] = make([]bool, w)
		for x := range w {
			// Check neighborhood
			found := false
			for dy := -half; dy <= half && !found; dy++ {
				for dx := -half; dx <= half && !found; dx++ {
					sy, sx := y+dy, x+dx
					if sy >= 0 && sy < h && sx >= 0 && sx < w && data[sy][sx] {
						found = true
					}
				}
			}
			result[y][x] = found
		}
	}
	return result
}

// connectedComponents labels connected regions using flood fill (4-connectivity).
func connectedComponents(binary [][]bool, w, h int) [][]int {
	labels := make([][]int, h)
	for y := range h {
		labels[y] = make([]int, w)
	}
	currentLabel := 0
	for y := range h {
		for x := range w {
			if binary[y][x] && labels[y][x] == 0 {
				currentLabel++
				floodFill4(binary, labels, w, h, x, y, currentLabel)
			}
		}
	}
	return labels
}

// floodFill4 performs 4-connected flood fill labeling.
func floodFill4(binary [][]bool, labels [][]int, w, h, startX, startY, label int) {
	stack := [][2]int{{startX, startY}}
	for len(stack) > 0 {
		x, y := stack[len(stack)-1][0], stack[len(stack)-1][1]
		stack = stack[:len(stack)-1]
		if x < 0 || x >= w || y < 0 || y >= h {
			continue
		}
		if labels[y][x] != 0 || !binary[y][x] {
			continue
		}
		labels[y][x] = label
		stack = append(stack,
			[2]int{x + 1, y},
			[2]int{x - 1, y},
			[2]int{x, y + 1},
			[2]int{x, y - 1},
		)
	}
}
