package wallpaper

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
)

// SchemeType represents a Material You color scheme variant.
type SchemeType string

const (
	SchemeContent    SchemeType = "scheme-content"
	SchemeExpressive SchemeType = "scheme-expressive"
	SchemeFidelity   SchemeType = "scheme-fidelity"
	SchemeFruitSalad SchemeType = "scheme-fruit-salad"
	SchemeMonochrome SchemeType = "scheme-monochrome"
	SchemeNeutral    SchemeType = "scheme-neutral"
	SchemeRainbow    SchemeType = "scheme-rainbow"
	SchemeTonalSpot  SchemeType = "scheme-tonal-spot"
	SchemeVibrant    SchemeType = "scheme-vibrant"
	SchemeAuto       SchemeType = "auto"
)

// ValidSchemeTypes returns all valid scheme type strings.
func ValidSchemeTypes() []string {
	return []string{
		string(SchemeContent), string(SchemeExpressive), string(SchemeFidelity),
		string(SchemeFruitSalad), string(SchemeMonochrome), string(SchemeNeutral),
		string(SchemeRainbow), string(SchemeTonalSpot), string(SchemeVibrant), string(SchemeAuto),
	}
}

// ParseSchemeType parses a scheme type string, returning TonalSpot as default.
func ParseSchemeType(s string) SchemeType {
	switch SchemeType(s) {
	case SchemeContent, SchemeExpressive, SchemeFidelity, SchemeFruitSalad,
		SchemeMonochrome, SchemeNeutral, SchemeRainbow, SchemeTonalSpot, SchemeVibrant,
		SchemeAuto:
		return SchemeType(s)
	default:
		return SchemeTonalSpot
	}
}

// ImageColorfulness calculates the Hasler-Süsstrunk colorfulness metric for an image.
func ImageColorfulness(imgPath string) (float64, error) {
	f, err := os.Open(imgPath)
	if err != nil {
		return 0, fmt.Errorf("open image: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("decode image: %w", err)
	}
	bounds := img.Bounds()

	var sumRG, sumYB, sumRG2, sumYB2 float64
	n := float64(bounds.Dx() * bounds.Dy())

	if n == 0 {
		return 0, nil
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Convert from alpha-premultiplied 16-bit to 0-1 range
			rf := float64(r>>8) / 255.0
			gf := float64(g>>8) / 255.0
			bf := float64(b>>8) / 255.0

			rg := math.Abs(rf - gf)
			yb := math.Abs(0.5*(rf+gf) - bf)

			sumRG += rg
			sumYB += yb
			sumRG2 += rg * rg
			sumYB2 += yb * yb
		}
	}

	meanRG := sumRG / n
	meanYB := sumYB / n
	stdRG := math.Sqrt(sumRG2/n - meanRG*meanRG)
	stdYB := math.Sqrt(sumYB2/n - meanYB*meanYB)

	colorfulness := math.Sqrt(stdRG*stdRG+stdYB*stdYB) + 0.3*math.Sqrt(meanRG*meanRG+meanYB*meanYB)

	return colorfulness, nil
}

// PickSchemeFromImage selects a Material You scheme based on image colorfulness.
// Low colorfulness → neutral, high → tonal-spot.
func PickSchemeFromImage(imgPath string) SchemeType {
	cf, err := ImageColorfulness(imgPath)
	if err != nil {
		return SchemeTonalSpot
	}
	if cf < 40 {
		return SchemeNeutral
	}
	return SchemeTonalSpot
}

// DetectSchemeFromImage auto-detects scheme type from an image.
// If detection fails, returns TonalSpot as default.
func DetectSchemeFromImage(imgPath string, requested SchemeType) SchemeType {
	if requested != SchemeAuto {
		return requested
	}
	return PickSchemeFromImage(imgPath)
}
