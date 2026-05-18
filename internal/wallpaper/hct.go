package wallpaper

// HCT (Hue-Chroma-Tone) color space implementation ported from
// materialyoucolor Python library. Uses CAM16 for Hue/Chroma and L* for Tone.
// The inverse transform uses HctSolver's binary search algorithm for
// finding in-gamut colors, matching the Python implementation exactly.

import (
	"fmt"
	"math"
)

// HCT represents a color in the HCT color space.
type HCT struct {
	Hue    float64
	Chroma float64
	Tone   float64
}

// --- Viewing conditions (default D65) ---

var defaultVC *ViewingConditions

// ViewingConditions holds CAM16 viewing condition parameters.
type ViewingConditions struct {
	N      float64
	Aw     float64
	Nbb    float64
	Ncb    float64
	C      float64
	Nc     float64
	RgbD   [3]float64
	Fl     float64
	FlRoot float64
	Z      float64
}

func init() {
	defaultVC = makeDefaultVC()
}

func makeDefaultVC() *ViewingConditions {
	// D65 white point
	wp := [3]float64{95.047, 100.0, 108.883}
	rW := wp[0]*0.401288 + wp[1]*0.650173 + wp[2]*(-0.051461)
	gW := wp[0]*(-0.250268) + wp[1]*1.204414 + wp[2]*0.045854
	bW := wp[0]*(-0.002079) + wp[1]*0.048952 + wp[2]*0.953127

	f := 0.8 + 2.0/10.0
	c := lerpF64(0.59, 0.69, (f-0.9)*10.0)
	adaptingLum := (200.0 / math.Pi) * yFromLstar(50.0) / 100.0
	d := f * (1.0 - (1.0/3.6)*math.Exp((-adaptingLum-42.0)/92.0))
	d = math.Min(1.0, math.Max(0.0, d))

	rgbD := [3]float64{
		d*(100.0/rW) + 1.0 - d,
		d*(100.0/gW) + 1.0 - d,
		d*(100.0/bW) + 1.0 - d,
	}

	k := 1.0 / (5.0*adaptingLum + 1.0)
	k4 := k * k * k * k
	k4F := 1.0 - k4
	fl := k4*adaptingLum + 0.1*k4F*k4F*math.Cbrt(5.0*adaptingLum)

	n := yFromLstar(50.0) / wp[1]
	z := 1.48 + math.Sqrt(n)
	nbb := 0.725 / math.Pow(n, 0.2)
	ncb := nbb

	rAF := math.Pow(fl*rgbD[0]*rW/100.0, 0.42)
	gAF := math.Pow(fl*rgbD[1]*gW/100.0, 0.42)
	bAF := math.Pow(fl*rgbD[2]*bW/100.0, 0.42)
	rA := (400.0 * rAF) / (rAF + 27.13)
	gA := (400.0 * gAF) / (gAF + 27.13)
	bA := (400.0 * bAF) / (bAF + 27.13)
	aw := (2.0*rA + gA + 0.05*bA) * nbb

	return &ViewingConditions{
		N:      n,
		Aw:     aw,
		Nbb:    nbb,
		Ncb:    ncb,
		C:      c,
		Nc:     f,
		RgbD:   rgbD,
		Fl:     fl,
		FlRoot: math.Pow(fl, 0.25),
		Z:      z,
	}
}

// --- Color math utilities ---

func delinearized(v float64) float64 {
	n := v / 100.0
	if n <= 0.0031308 {
		return clampFloat(0, 255, math.Round(n*12.92*255))
	}
	return clampFloat(0, 255, math.Round((1.055*math.Pow(n, 1.0/2.4)-0.055)*255))
}

func delinearizedF(v float64) float64 {
	n := v / 100.0
	if n <= 0.0031308 {
		return n * 12.92
	}
	return 1.055*math.Pow(n, 1.0/2.4) - 0.055
}

func linearized8(v float64) float64 {
	n := v / 255.0
	if n <= 0.040449936 {
		return n / 12.92 * 100.0
	}
	return math.Pow((n+0.055)/1.055, 2.4) * 100.0
}

func yFromLstar(lstar float64) float64 {
	return 100.0 * labInvF((lstar+16.0)/116.0)
}

func lstarFromARGB(argb uint32) float64 {
	r := float64((argb >> 16) & 0xFF)
	g := float64((argb >> 8) & 0xFF)
	b := float64(argb & 0xFF)
	rl := linearized8(r) / 100.0
	gl := linearized8(g) / 100.0
	bl := linearized8(b) / 100.0
	y := 0.2126*rl + 0.7152*gl + 0.0722*bl
	return 116.0*labF(y) - 16.0
}

func labF(t float64) float64 {
	e := 216.0 / 24389.0
	if t > e {
		return math.Cbrt(t)
	}
	return (24389.0/27.0*t + 16.0) / 116.0
}

func labInvF(ft float64) float64 {
	e := 216.0 / 24389.0
	ft3 := ft * ft * ft
	if ft3 > e {
		return ft3
	}
	return (116.0*ft - 16.0) / (24389.0 / 27.0)
}

func signum(x float64) float64 {
	if x < 0 {
		return -1.0
	}
	if x == 0 {
		return 0.0
	}
	return 1.0
}

func lerpF64(a, b, t float64) float64 {
	return (1.0-t)*a + t*b
}

func clampFloat(lo, hi, v float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func sanitizeDegreesDouble(d float64) float64 {
	d = math.Mod(d, 360.0)
	if d < 0 {
		d += 360.0
	}
	return d
}

func differenceDegrees(a, b float64) float64 {
	return 180.0 - math.Abs(math.Abs(a-b)-180.0)
}

func rotationDirection(from, to float64) float64 {
	diff := sanitizeDegreesDouble(to - from)
	if diff <= 180.0 {
		return 1.0
	}
	return -1.0
}

// --- Matrix multiply ---

func matMul(row [3]float64, m [3][3]float64) [3]float64 {
	return [3]float64{
		row[0]*m[0][0] + row[1]*m[0][1] + row[2]*m[0][2],
		row[0]*m[1][0] + row[1]*m[1][1] + row[2]*m[1][2],
		row[0]*m[2][0] + row[1]*m[2][1] + row[2]*m[2][2],
	}
}

// --- SRGB ↔ XYZ matrices ---

var srgbToXYZ = [3][3]float64{
	{0.41233895, 0.35762064, 0.18051042},
	{0.2126, 0.7152, 0.0722},
	{0.01932141, 0.11916382, 0.95034478},
}

var xyzToSRGB = [3][3]float64{
	{3.2413774792388685, -1.5376652402851851, -0.49885366846268053},
	{-0.9691452513005321, 1.8758853451067872, 0.04156585616912061},
	{0.05562093689691305, -0.20395524564742123, 1.0571799111220335},
}

// D65 white point
var whitePointD65 = [3]float64{95.047, 100.0, 108.883}

// --- HctSolver: core of HCT → RGB conversion ---

var scaledDiscountFromLinRGB = [3][3]float64{
	{0.001200833568784504, 0.002389694492170889, 0.0002795742885861124},
	{0.0005891086651375999, 0.0029785502573438758, 0.0003270666104008398},
	{0.00010146692491640572, 0.0005364214359186694, 0.0032979401770712076},
}

var linRGBFromScaledDiscount = [3][3]float64{
	{1373.2198709594231, -1100.4251190754821, -7.278681089101213},
	{-271.815969077903, 559.6580465940733, -32.46047482791194},
	{1.9622899599665666, -57.173814538844006, 308.7233197812385},
}

var yFromLinRGB = [3]float64{0.2126, 0.7152, 0.0722}

// criticalPlanes is the precomputed list from the Python source.
var criticalPlanes = [...]float64{
	0.015176349177441876, 0.045529047532325624, 0.07588174588720938,
	0.10623444424209313, 0.13658714259697685, 0.16693984095186062,
	0.19729253930674434, 0.2276452376616281, 0.2579979360165119,
	0.28835063437139563, 0.3188300904430532, 0.350925934958123,
	0.3848314933096426, 0.42057480301049466, 0.458183274052838,
	0.4976837250274023, 0.5391024159806381, 0.5824650784040898,
	0.6277969426914107, 0.6751227633498623, 0.7244668422128921,
	0.775853049866786, 0.829304845476233, 0.8848452951698498,
	0.942497089126609, 1.0022825574869039, 1.0642236851973577,
	1.1283421258858297, 1.1946592148522128, 1.2631959812511864,
	1.3339731595349034, 1.407011200216447, 1.4823302800086415,
	1.5599503113873272, 1.6398909516233677, 1.7221716113234105,
	1.8068114625156377, 1.8938294463134073, 1.9832442801866852,
	2.075074464868551, 2.1693382909216234, 2.2660538449872063,
	2.36523901573795, 2.4669114995532007, 2.5710888059345764,
	2.6777882626779785, 2.7870270208169257, 2.898822059350997,
	3.0131901897720907, 3.1301480604002863, 3.2497121605402226,
	3.3718988244681087, 3.4967242352587946, 3.624204428461639,
	3.754355295633311, 3.887192587735158, 4.022731918402185,
	4.160988767090289, 4.301978482107941, 4.445716283538092,
	4.592217266055746, 4.741496401646282, 4.893568542229298,
	5.048448422192488, 5.20615066083972, 5.3666897647573375,
	5.5300801301023865, 5.696336044816294, 5.865471690767354,
	6.037501145825082, 6.212438385869475, 6.390297286737924,
	6.571091626112461, 6.7548350853498045, 6.941541251256611,
	7.131223617812143, 7.323895587840543, 7.5195704746346665,
	7.7182615035334345, 7.919981813454504, 8.124744458384042,
	8.332562408825165, 8.543448553206703, 8.757415699253682,
	8.974476575321063, 9.194643831691977, 9.417930041841839,
	9.644347703669503, 9.873909240696694, 10.106627003236781,
	10.342513269534024, 10.58158024687427, 10.8238400726681,
	11.069304815507364, 11.317986476196008, 11.569896988756009,
	11.825048221409341, 12.083451977536606, 12.345119996613247,
	12.610063955123938, 12.878295467455942, 13.149826086772048,
	13.42466730586372, 13.702830557985108, 13.984327217668513,
	14.269168601521828, 14.55736596900856, 14.848930523210871,
	15.143873411576273, 15.44220572664832, 15.743938506781891,
	16.04908273684337, 16.35764934889634, 16.66964922287304,
	16.985093187232053, 17.30399201960269, 17.62635644741625,
	17.95219714852476, 18.281524751807332, 18.614349837764564,
	18.95068293910138, 19.290534541298456, 19.633915083172692,
	19.98083495742689, 20.331304511189067, 20.685334046541502,
	21.042933821039977, 21.404114048223256, 21.76888489811322,
	22.137256497705877, 22.50923893145328, 22.884842241736916,
	23.264076429332462, 23.6469514538663, 24.033477234264016,
	24.42366364919083, 24.817520537484558, 25.21505769858089,
	25.61628489293138, 26.021211842414342, 26.429848230738664,
	26.842203703840827, 27.258287870275353, 27.678110301598522,
	28.10168053274597, 28.529008062403893, 28.96010235337422,
	29.39497283293396, 29.83362889318845, 30.276079891419332,
	30.722335150426627, 31.172403958865512, 31.62629557157785,
	32.08401920991837, 32.54558406207592, 33.010999283389665,
	33.4802739966603, 33.953417292456834, 34.430438229418264,
	34.911345834551085, 35.39614910352207, 35.88485700094671,
	36.37747846067349, 36.87402238606382, 37.37449765026789,
	37.87891309649659, 38.38727753828926, 38.89959975977785,
	39.41588851594697, 39.93615253289054, 40.460400508064545,
	40.98864111053629, 41.520882981230194, 42.05713473317016,
	42.597404951718396, 43.141702194811224, 43.6900349931913,
	44.24241185063697, 44.798841244188324, 45.35933162437017,
	45.92389141541209, 46.49252901546552, 47.065252796817916,
	47.64207110610409, 48.22299226451468, 48.808024568002054,
	49.3971762874833, 49.9904556690408, 50.587870934119984,
	51.189430279724725, 51.79514187861014, 52.40501387947288,
	53.0190544071392, 53.637271562750364, 54.259673423945976,
	54.88626804504493, 55.517063457223934, 56.15206766869424,
	56.79128866487574, 57.43473440856916, 58.08241284012621,
	58.734331877617365, 59.39049941699807, 60.05092333227251,
	60.715611475655585, 61.38457167773311, 62.057811747619894,
	62.7353394731159, 63.417162620860914, 64.10328893648692,
	64.79372614476921, 65.48848194977529, 66.18756403501224,
	66.89098006357258, 67.59873767827808, 68.31084450182222,
	69.02730813691093, 69.74813616640164, 70.47333615344107,
	71.20291564160104, 71.93688215501312, 72.67524319850172,
	73.41800625771542, 74.16517879925733, 74.9167682708136,
	75.67278210128072, 76.43322770089146, 77.1981124613393,
	77.96744375590167, 78.74122893956174, 79.51947534912904,
	80.30219030335869, 81.08938110306934, 81.88105503125999,
	82.67721935322541, 83.4778813166706, 84.28304815182372,
	85.09272707154808, 85.90692527145302, 86.72564993000343,
	87.54890820862819, 88.3767072518277, 89.2090541872801,
	90.04595612594655, 90.88742016217518, 91.73345337380438,
	92.58406282226491, 93.43925555268066, 94.29903859396902,
	95.16341895893969, 96.03240364439274, 96.9059996312159,
	97.78421388448044, 98.6670533535366, 99.55452497210776,
}

func sanitizeRadians(angle float64) float64 {
	return math.Mod(angle+math.Pi*8, math.Pi*2)
}

func chromaticAdaptation(component float64) float64 {
	af := math.Pow(math.Abs(component), 0.42)
	return signum(component) * 400.0 * af / (af + 27.13)
}

func hueOf(linrgb [3]float64) float64 {
	scaledDiscount := matMul(linrgb, scaledDiscountFromLinRGB)
	rA := chromaticAdaptation(scaledDiscount[0])
	gA := chromaticAdaptation(scaledDiscount[1])
	bA := chromaticAdaptation(scaledDiscount[2])
	a := (11.0*rA + -12.0*gA + bA) / 11.0
	b := (rA + gA - 2.0*bA) / 9.0
	return math.Atan2(b, a)
}

func areInCyclicOrder(a, b, c float64) bool {
	deltaAB := sanitizeRadians(b - a)
	deltaAC := sanitizeRadians(c - a)
	return deltaAB < deltaAC
}

func intercept(source, mid, target float64) float64 {
	return (mid - source) / (target - source)
}

func lerpPoint(source [3]float64, t float64, target [3]float64) [3]float64 {
	return [3]float64{
		source[0] + (target[0]-source[0])*t,
		source[1] + (target[1]-source[1])*t,
		source[2] + (target[2]-source[2])*t,
	}
}

func setCoordinate(source [3]float64, coordinate float64, target [3]float64, axis int) [3]float64 {
	t := intercept(source[axis], coordinate, target[axis])
	return lerpPoint(source, t, target)
}

func isBounded(x float64) bool {
	return 0.0 <= x && x <= 100.0
}

func nthVertex(y, n float64) [3]float64 {
	kr := yFromLinRGB[0]
	kg := yFromLinRGB[1]
	kb := yFromLinRGB[2]
	coordA := 0.0
	if int(n)%4 > 1 {
		coordA = 100.0
	}
	coordB := 0.0
	if int(n)%2 == 0 {
		coordB = 100.0
	}

	if n < 4 {
		g := coordA
		b := coordB
		r := (y - g*kg - b*kb) / kr
		if isBounded(r) {
			return [3]float64{r, g, b}
		}
		return [3]float64{-1.0, -1.0, -1.0}
	} else if n < 8 {
		b := coordA
		r := coordB
		g := (y - r*kr - b*kb) / kg
		if isBounded(g) {
			return [3]float64{r, g, b}
		}
		return [3]float64{-1.0, -1.0, -1.0}
	} else {
		r := coordA
		g := coordB
		b := (y - r*kr - g*kg) / kb
		if isBounded(b) {
			return [3]float64{r, g, b}
		}
		return [3]float64{-1.0, -1.0, -1.0}
	}
}

func bisectToSegment(y, targetHue float64) ([3]float64, [3]float64) {
	var left, right [3]float64
	left = [3]float64{-1, -1, -1}
	right = [3]float64{-1, -1, -1}
	var leftHue, rightHue float64
	initialized := false
	uncut := true

	for n := range 12 {
		mid := nthVertex(y, float64(n))
		if mid[0] < 0 {
			continue
		}
		midHue := hueOf(mid)
		if !initialized {
			left = mid
			right = mid
			leftHue = midHue
			rightHue = midHue
			initialized = true
			continue
		}
		if uncut || areInCyclicOrder(leftHue, midHue, rightHue) {
			uncut = false
			if areInCyclicOrder(leftHue, targetHue, midHue) {
				right = mid
				rightHue = midHue
			} else {
				left = mid
				leftHue = midHue
			}
		}
	}
	return left, right
}

func criticalPlaneBelow(x float64) float64 {
	return math.Floor(x - 0.5)
}

func criticalPlaneAbove(x float64) float64 {
	return math.Ceil(x - 0.5)
}

func bisectToLimit(y, targetHue float64) [3]float64 {
	left, right := bisectToSegment(y, targetHue)
	leftHue := hueOf(left)

	for axis := range 3 {
		if left[axis] != right[axis] {
			var lPlane, rPlane float64
			if left[axis] < right[axis] {
				lPlane = criticalPlaneBelow(delinearizedF(left[axis]))
				rPlane = criticalPlaneAbove(delinearizedF(right[axis]))
			} else {
				lPlane = criticalPlaneAbove(delinearizedF(left[axis]))
				rPlane = criticalPlaneBelow(delinearizedF(right[axis]))
			}

			for range 8 {
				if math.Abs(rPlane-lPlane) <= 1 {
					break
				}
				mPlane := math.Floor((lPlane + rPlane) / 2.0)
				midPlaneCoord := criticalPlanes[int(mPlane)]
				mid := setCoordinate(left, midPlaneCoord, right, axis)
				midHue := hueOf(mid)
				if areInCyclicOrder(leftHue, targetHue, midHue) {
					right = mid
					rPlane = mPlane
				} else {
					left = mid
					leftHue = midHue
					lPlane = mPlane
				}
			}
		}
	}
	return [3]float64{(left[0] + right[0]) / 2, (left[1] + right[1]) / 2, (left[2] + right[2]) / 2}
}

func inverseChromaticAdaptation(adapted float64) float64 {
	adaptedAbs := math.Abs(adapted)
	base := math.Max(0, 27.13*adaptedAbs/(400.0-adaptedAbs))
	return signum(adapted) * math.Pow(base, 1.0/0.42)
}

func argbFromLinRGB(linrgb [3]float64) uint32 {
	r := delinearized(linrgb[0])
	g := delinearized(linrgb[1])
	b := delinearized(linrgb[2])
	return 0xFF000000 | (uint32(r)&0xFF)<<16 | (uint32(g)&0xFF)<<8 | uint32(b)&0xFF
}

func argbFromLstar(lstar float64) uint32 {
	y := yFromLstar(lstar)
	component := delinearized(y)
	return 0xFF000000 | (uint32(component)&0xFF)<<16 | (uint32(component)&0xFF)<<8 | uint32(component)&0xFF
}

func findResultByJ(hueRadians, chroma, y float64) uint32 {
	j := math.Sqrt(y) * 11.0
	vc := defaultVC
	tInnerCoeff := 1.0 / math.Pow(1.64-math.Pow(0.29, vc.N), 0.73)
	eHue := 0.25 * (math.Cos(hueRadians+2.0) + 3.8)
	p1 := eHue * (50000.0 / 13.0) * vc.Nc * vc.Ncb
	hSin := math.Sin(hueRadians)
	hCos := math.Cos(hueRadians)

	for iterationRound := range 5 {
		jNormalized := j / 100.0
		var alpha float64
		if chroma != 0.0 && j != 0.0 {
			alpha = chroma / math.Sqrt(jNormalized)
		}
		t := math.Pow(alpha*tInnerCoeff, 1.0/0.9)
		ac := vc.Aw * math.Pow(jNormalized, 1.0/vc.C/vc.Z)
		p2 := ac / vc.Nbb
		gamma := (23.0 * (p2 + 0.305) * t) / (23.0*p1 + 11*t*hCos + 108.0*t*hSin)
		a := gamma * hCos
		b := gamma * hSin
		rA := (460.0*p2 + 451.0*a + 288.0*b) / 1403.0
		gA := (460.0*p2 - 891.0*a - 261.0*b) / 1403.0
		bA := (460.0*p2 - 220.0*a - 6300.0*b) / 1403.0
		rCScaled := inverseChromaticAdaptation(rA)
		gCScaled := inverseChromaticAdaptation(gA)
		bCScaled := inverseChromaticAdaptation(bA)
		linrgb := matMul([3]float64{rCScaled, gCScaled, bCScaled}, linRGBFromScaledDiscount)

		if linrgb[0] < 0 || linrgb[1] < 0 || linrgb[2] < 0 {
			return 0
		}

		fnj := yFromLinRGB[0]*linrgb[0] + yFromLinRGB[1]*linrgb[1] + yFromLinRGB[2]*linrgb[2]
		if fnj <= 0 {
			return 0
		}

		if iterationRound == 4 || math.Abs(fnj-y) < 0.002 {
			if linrgb[0] > 100.01 || linrgb[1] > 100.01 || linrgb[2] > 100.01 {
				return 0
			}
			return argbFromLinRGB(linrgb)
		}
		j = j - (fnj-y)*j/(2*fnj)
	}
	return 0
}

func solveToInt(hueDegrees, chroma, lstar float64) uint32 {
	if chroma < 0.0001 || lstar < 0.0001 || lstar > 99.9999 {
		return argbFromLstar(lstar)
	}
	hueDegrees = sanitizeDegreesDouble(hueDegrees)
	hueRadians := hueDegrees / 180.0 * math.Pi
	y := yFromLstar(lstar)
	exactAnswer := findResultByJ(hueRadians, chroma, y)
	if exactAnswer != 0 {
		return exactAnswer
	}
	linrgb := bisectToLimit(y, hueRadians)
	return argbFromLinRGB(linrgb)
}

// --- CAM16 forward transform (for reading HCT from a color) ---

func cam16FromInt(argb uint32) (hue, chroma, jCam, qCam, mCam, sCam, jStar, aStar, bStar float64) {
	vc := defaultVC
	r := float64((argb >> 16) & 0xFF)
	g := float64((argb >> 8) & 0xFF)
	b := float64(argb & 0xFF)
	redL := linearized8(r)
	greenL := linearized8(g)
	blueL := linearized8(b)

	x := 0.41233895*redL + 0.35762064*greenL + 0.18051042*blueL
	y := 0.2126*redL + 0.7152*greenL + 0.0722*blueL
	z := 0.01932141*redL + 0.11916382*greenL + 0.95034478*blueL

	rC := 0.401288*x + 0.650173*y - 0.051461*z
	gC := -0.250268*x + 1.204414*y + 0.045854*z
	bC := -0.002079*x + 0.048952*y + 0.953127*z

	rD := vc.RgbD[0] * rC
	gD := vc.RgbD[1] * gC
	bD := vc.RgbD[2] * bC

	rAF := math.Pow(vc.Fl*math.Abs(rD)/100.0, 0.42)
	gAF := math.Pow(vc.Fl*math.Abs(gD)/100.0, 0.42)
	bAF := math.Pow(vc.Fl*math.Abs(bD)/100.0, 0.42)

	rA := signum(rD) * 400.0 * rAF / (rAF + 27.13)
	gA := signum(gD) * 400.0 * gAF / (gAF + 27.13)
	bAA := signum(bD) * 400.0 * bAF / (bAF + 27.13)

	aPrim := (11.0*rA + -12.0*gA + bAA) / 11.0
	bPrim := (rA + gA - 2.0*bAA) / 9.0
	u := (20.0*rA + 20.0*gA + 21.0*bAA) / 20.0
	p2 := (40.0*rA + 20.0*gA + bAA) / 20.0

	atan2Val := math.Atan2(bPrim, aPrim)
	atanDegrees := atan2Val * 180.0 / math.Pi
	var h float64
	if atanDegrees < 0 {
		h = atanDegrees + 360.0
	} else if atanDegrees >= 360 {
		h = atanDegrees - 360.0
	} else {
		h = atanDegrees
	}
	hueRadians := h * math.Pi / 180.0
	hue = h

	ac := p2 * vc.Nbb
	jCam = 100.0 * math.Pow(ac/vc.Aw, vc.C*vc.Z)
	qCam = (4.0 / vc.C) * math.Sqrt(jCam/100.0) * (vc.Aw + 4.0) * vc.FlRoot

	huePrime := h + 360.0
	if h >= 20.14 {
		huePrime = h
	}
	eHue := 0.25 * (math.Cos(huePrime*math.Pi/180.0+2.0) + 3.8)
	p1 := (50000.0 / 13.0) * eHue * vc.Nc * vc.Ncb
	t := (p1 * math.Sqrt(aPrim*aPrim+bPrim*bPrim)) / (u + 0.305)
	alpha := math.Pow(t, 0.9) * math.Pow(1.64-math.Pow(0.29, vc.N), 0.73)
	chroma = alpha * math.Sqrt(jCam/100.0)
	mCam = chroma * vc.FlRoot
	sCam = 50.0 * math.Sqrt((alpha*vc.C)/(vc.Aw+4.0))
	jStar = ((1.0 + 100.0*0.007) * jCam) / (1.0 + 0.007*jCam)
	mStar := (1.0 / 0.0228) * math.Log(1.0+0.0228*mCam)
	aStar = mStar * math.Cos(hueRadians)
	bStar = mStar * math.Sin(hueRadians)
	return
}

// --- Public API ---

// HCTFromARGB creates an HCT from an ARGB int (0xAARRGGBB).
func HCTFromARGB(argb uint32) HCT {
	hue, chroma, _, _, _, _, _, _, _ := cam16FromInt(argb)
	tone := lstarFromARGB(argb)
	return HCT{Hue: hue, Chroma: chroma, Tone: tone}
}

// ToARGB converts HCT back to ARGB int using the HctSolver.
func (hct HCT) ToARGB() uint32 {
	return solveToInt(hct.Hue, hct.Chroma, hct.Tone)
}

// HexToARGB converts a hex color string (#RRGGBB or RRGGBB) to ARGB int.
func HexToARGB(hex string) uint32 {
	h := hex
	if len(h) > 0 && h[0] == '#' {
		h = h[1:]
	}
	r := parseHex(h[0:2])
	g := parseHex(h[2:4])
	b := parseHex(h[4:6])
	return 0xFF000000 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

func parseHex(s string) uint8 {
	v := 0
	for _, c := range s {
		v *= 16
		if c >= '0' && c <= '9' {
			v += int(c - '0')
		} else if c >= 'a' && c <= 'f' {
			v += int(c-'a') + 10
		} else if c >= 'A' && c <= 'F' {
			v += int(c-'A') + 10
		}
	}
	return uint8(v)
}

// ARGBToHex converts ARGB int to #RRGGBB hex string.
func ARGBToHex(argb uint32) string {
	r := uint8((argb >> 16) & 0xFF)
	g := uint8((argb >> 8) & 0xFF)
	b := uint8(argb & 0xFF)
	return formatHex(r, g, b)
}

func formatHex(r, g, b uint8) string {
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

// Harmonize shifts hue of designColor towards sourceColor, bounded by threshold.
// Ported from materialyoucolor's Blend.harmonize with the Python script's custom params.
func Harmonize(designColor, sourceColor uint32, threshold, harmony float64) uint32 {
	fromHCT := HCTFromARGB(designColor)
	toHCT := HCTFromARGB(sourceColor)
	diffDeg := differenceDegrees(fromHCT.Hue, toHCT.Hue)
	rotationDeg := math.Min(diffDeg*harmony, threshold)
	outputHue := sanitizeDegreesDouble(fromHCT.Hue + rotationDeg*rotationDirection(fromHCT.Hue, toHCT.Hue))
	return solveToInt(outputHue, fromHCT.Chroma, fromHCT.Tone)
}

// BoostChromaTone increases chroma and/or tone of a color in HCT space.
func BoostChromaTone(argb uint32, chroma, tone float64) uint32 {
	hct := HCTFromARGB(argb)
	return solveToInt(hct.Hue, hct.Chroma*chroma, hct.Tone*tone)
}
