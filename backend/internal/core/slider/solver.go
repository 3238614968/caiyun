// Package slider matches cloud-drive puzzle images locally. It owns no network
// client or mutable solver state and can run concurrently for separate accounts.
package slider

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strings"
)

const (
	maxImageBytes  = 4 << 20
	maxImagePixels = 2 << 20
	maxSamples     = 1200
	method         = "内置多特征NCC"
)

type Request struct {
	Puzzle       string
	Picture      string
	PictureWidth int // The coordinate width declared by the challenge server.
	PuzzleTop    *int
}

type Result struct {
	Offset     int
	Confidence float64 // Correlation quality, not a calibrated success probability.
	PeakMargin float64
	Method     string
}

type raster struct {
	w, h       int
	gray       []float64
	mask       []bool
	gx, gy, gm []float64
}

type sample struct {
	x, y              int
	luma, gx, gy, mag float64
}

type template struct {
	points                           []sample
	lumaEnergy, vecEnergy, magEnergy float64
}

type candidate struct {
	x, y  int
	score float64
}

// Solve returns the original puzzle-canvas origin in server image coordinates.
// Transparent left padding remains part of that origin; it is never added to
// the answer. Image resizing changes coordinates only through PictureWidth.
func Solve(ctx context.Context, req Request) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.PictureWidth < 0 || req.PictureWidth > 4096 {
		return nil, fmt.Errorf("背景坐标宽度无效")
	}
	background, err := decodeRaster(ctx, req.Picture, false)
	if err != nil {
		return nil, fmt.Errorf("解码背景图: %w", err)
	}
	puzzle, err := decodeRaster(ctx, req.Puzzle, true)
	if err != nil {
		return nil, fmt.Errorf("解码滑块图: %w", err)
	}
	if puzzle.w > background.w || puzzle.h > background.h {
		return nil, fmt.Errorf("滑块尺寸大于背景图")
	}
	full, err := makeTemplate(puzzle, maxSamples)
	if err != nil {
		return nil, err
	}
	minY, maxY := 0, background.h-puzzle.h
	if req.PuzzleTop != nil && puzzle.h != background.h {
		top := *req.PuzzleTop
		if req.PictureWidth > 0 {
			top = int(math.Round(float64(top) * float64(background.w) / float64(req.PictureWidth)))
		}
		if top < 0 || top > maxY {
			return nil, fmt.Errorf("滑块垂直位置超出背景范围")
		}
		minY, maxY = top, top
	}
	maxX := background.w - puzzle.w
	searchTemplate := full
	if maxY > minY {
		// Search every origin with a smaller spatially distributed sample set.
		// This avoids skipping odd-pixel matches in a coarse coordinate grid.
		if int64(maxX+1)*int64(maxY-minY+1) > 500000 {
			return nil, fmt.Errorf("缺少垂直位置且搜索区域过大")
		}
		searchTemplate, err = makeTemplate(puzzle, 80)
		if err != nil {
			return nil, err
		}
	}
	peaks := make([]candidate, 0, 12)
	for y := minY; y <= maxY; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x <= maxX; x++ {
			if x&63 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			keepPeak(&peaks, candidate{x, y, matchScore(background, searchTemplate, x, y)})
		}
	}
	if maxY > minY {
		refined := make([]candidate, 0, 12)
		for _, peak := range peaks {
			for y := max(minY, peak.y-1); y <= min(maxY, peak.y+1); y++ {
				for x := max(0, peak.x-2); x <= min(maxX, peak.x+2); x++ {
					keepPeak(&refined, candidate{x, y, matchScore(background, full, x, y)})
				}
			}
		}
		peaks = refined
	}
	if len(peaks) == 0 || peaks[0].score < -0.5 {
		return nil, fmt.Errorf("图片缺少可匹配纹理")
	}
	best := peaks[0]
	margin := 2.0
	for _, peak := range peaks[1:] {
		if abs(peak.x-best.x) > 6 {
			margin = best.score - peak.score
			break
		}
	}
	// Nearly perfect matches can have a small absolute gap on smooth images,
	// while still having much less residual error than their runner-up. Equal
	// peaks remain ambiguous regardless of their apparent confidence.
	if margin < 0.025 && (margin <= 1e-6 || 1-best.score > 0.5*(1-best.score+margin)) {
		return nil, fmt.Errorf("滑块匹配存在歧义: score=%.4f peak_margin=%.4f", best.score, margin)
	}
	width := req.PictureWidth
	if width <= 0 {
		width = background.w
	}
	return &Result{
		Offset:     int(math.Round(float64(best.x) * float64(width) / float64(background.w))),
		Confidence: clamp((best.score+1)/2, 0, 1), PeakMargin: margin, Method: method,
	}, nil
}

func decodeRaster(ctx context.Context, encoded string, puzzle bool) (*raster, error) {
	encoded = strings.TrimSpace(encoded)
	if strings.HasPrefix(encoded, "data:") {
		comma := strings.IndexByte(encoded, ',')
		if comma < 0 || !strings.HasSuffix(encoded[:comma], ";base64") {
			return nil, fmt.Errorf("图片 data URI 无效")
		}
		encoded = encoded[comma+1:]
	}
	if len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(maxImageBytes) {
		return nil, fmt.Errorf("图片为空或超过大小限制")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, fmt.Errorf("图片 base64 无效")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("图片格式无效: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 2048 || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return nil, fmt.Errorf("图片尺寸超过限制")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	r := &raster{w: bounds.Dx(), h: bounds.Dy(), gray: make([]float64, bounds.Dx()*bounds.Dy())}
	r.mask = make([]bool, len(r.gray))
	hasTransparency := false
	for y := 0; y < r.h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < r.w; x++ {
			gray, aa := grayAlphaAt(img, bounds.Min.X+x, bounds.Min.Y+y)
			idx := y*r.w + x
			if aa < 0xffff {
				hasTransparency = true
			}
			r.mask[idx] = aa >= 0xc000
			r.gray[idx] = gray
		}
	}
	if puzzle && !hasTransparency {
		r.removeBlackMatte()
	}
	r.gx, r.gy, r.gm = make([]float64, len(r.gray)), make([]float64, len(r.gray)), make([]float64, len(r.gray))
	// Apply the same small Gaussian to both images before differentiating. JPEG
	// quantization changes single-pixel edges much more than the actual texture;
	// smoothing suppresses those artifacts while keeping the cutout position.
	// Reuse the gradient buffer as scratch to avoid another image-sized allocation.
	for y := 0; y < r.h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < r.w; x++ {
			idx := y*r.w + x
			r.gx[idx] = (r.gray[y*r.w+max(0, x-1)] + 2*r.gray[idx] + r.gray[y*r.w+min(r.w-1, x+1)]) / 4
		}
	}
	for y := 0; y < r.h; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < r.w; x++ {
			idx := y*r.w + x
			r.gray[idx] = (r.gx[max(0, y-1)*r.w+x] + 2*r.gx[idx] + r.gx[min(r.h-1, y+1)*r.w+x]) / 4
		}
	}
	clear(r.gx)
	for y := 1; y < r.h-1; y++ {
		for x := 1; x < r.w-1; x++ {
			idx := y*r.w + x
			r.gx[idx] = (r.gray[idx+1] - r.gray[idx-1]) / 2
			r.gy[idx] = (r.gray[idx+r.w] - r.gray[idx-r.w]) / 2
			r.gm[idx] = math.Hypot(r.gx[idx], r.gy[idx])
		}
	}
	return r, nil
}

// Typed access avoids boxing a color interface for each pixel of PNG images.
// Premultiplied RGBA is unwrapped; NRGBA already contains straight colors.
func grayAlphaAt(img image.Image, x, y int) (float64, uint32) {
	switch src := img.(type) {
	case *image.Gray:
		return float64(src.GrayAt(x, y).Y), 0xffff
	case *image.NRGBA:
		c := src.NRGBAAt(x, y)
		return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B), uint32(c.A) * 257
	case *image.RGBA:
		c := src.RGBAAt(x, y)
		if c.A == 0 {
			return 0, 0
		}
		return (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) * 255 / float64(c.A), uint32(c.A) * 257
	case *image.YCbCr:
		c := src.YCbCrAt(x, y)
		r, g, b, _ := c.RGBA()
		return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257, 0xffff
	default:
		r, g, b, a := img.At(x, y).RGBA()
		if a == 0 {
			return 0, 0
		}
		return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) * 255 / float64(a), a
	}
}

// Flood only the near-black matte connected to the image border. Dark details
// inside the puzzle remain available, unlike the reference's gray>30 mask.
func (r *raster) removeBlackMatte() {
	corners := []int{0, r.w - 1, (r.h - 1) * r.w, len(r.gray) - 1}
	darkCorners := 0
	for _, idx := range corners {
		if r.gray[idx] <= 8 {
			darkCorners++
		}
	}
	if darkCorners < 3 {
		return
	}
	queue := make([]int, 0, len(r.gray)/2)
	add := func(idx int) {
		if r.mask[idx] && r.gray[idx] <= 8 {
			r.mask[idx] = false
			queue = append(queue, idx)
		}
	}
	for x := 0; x < r.w; x++ {
		add(x)
		add((r.h-1)*r.w + x)
	}
	for y := 0; y < r.h; y++ {
		add(y * r.w)
		add(y*r.w + r.w - 1)
	}
	for head := 0; head < len(queue); head++ {
		idx := queue[head]
		x, y := idx%r.w, idx/r.w
		if x > 0 {
			add(idx - 1)
		}
		if x+1 < r.w {
			add(idx + 1)
		}
		if y > 0 {
			add(idx - r.w)
		}
		if y+1 < r.h {
			add(idx + r.w)
		}
	}
}

func makeTemplate(r *raster, limit int) (*template, error) {
	indices := make([]int, 0, min(r.w*r.h, 4096))
	for y := 2; y < r.h-2; y++ {
		for x := 2; x < r.w-2; x++ {
			idx := y*r.w + x
			// Erode the mask by two pixels so the white outline and transparent
			// silhouette cannot dominate the actual texture gradients.
			interior := true
			for dy := -2; dy <= 2 && interior; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if !r.mask[idx+dy*r.w+dx] {
						interior = false
						break
					}
				}
			}
			if interior {
				indices = append(indices, idx)
			}
		}
	}
	if len(indices) < 48 {
		return nil, fmt.Errorf("滑块有效区域过小")
	}
	points := make([]sample, min(limit, len(indices)))
	for i := range points {
		idx := indices[i*len(indices)/len(points)]
		points[i] = sample{idx % r.w, idx / r.w, r.gray[idx], r.gx[idx], r.gy[idx], r.gm[idx]}
	}
	t := &template{points: points}
	var meanLuma, meanX, meanY, meanMag float64
	for _, p := range points {
		meanLuma += p.luma
		meanX += p.gx
		meanY += p.gy
		meanMag += p.mag
	}
	n := float64(len(points))
	meanLuma /= n
	meanX /= n
	meanY /= n
	meanMag /= n
	for i := range t.points {
		p := &t.points[i]
		p.luma -= meanLuma
		p.gx -= meanX
		p.gy -= meanY
		p.mag -= meanMag
		t.lumaEnergy += p.luma * p.luma
		t.vecEnergy += p.gx*p.gx + p.gy*p.gy
		t.magEnergy += p.mag * p.mag
	}
	if t.lumaEnergy/n < 0.1 || t.vecEnergy/n < 0.02 {
		return nil, fmt.Errorf("滑块缺少可辨识纹理")
	}
	return t, nil
}

func matchScore(bg *raster, t *template, x, y int) float64 {
	var sumL, sumX, sumY, sumM, squareL, squareV, squareM, dotL, dotV, dotM float64
	for _, p := range t.points {
		idx := (y+p.y)*bg.w + x + p.x
		l, gx, gy, m := bg.gray[idx], bg.gx[idx], bg.gy[idx], bg.gm[idx]
		sumL += l
		sumX += gx
		sumY += gy
		sumM += m
		squareL += l * l
		squareV += gx*gx + gy*gy
		squareM += m * m
		dotL += l * p.luma
		dotV += gx*p.gx + gy*p.gy
		dotM += m * p.mag
	}
	n := float64(len(t.points))
	varL, varV, varM := squareL-sumL*sumL/n, squareV-(sumX*sumX+sumY*sumY)/n, squareM-sumM*sumM/n
	if varL <= 1e-8 || varV <= 1e-8 {
		return -1
	}
	// Signed gradients distinguish opposite edges; magnitude is robust to
	// small edge shifts, and luminance correlation tolerates brightness changes.
	value, weight := 0.2*dotL/math.Sqrt(varL*t.lumaEnergy)+0.55*dotV/math.Sqrt(varV*t.vecEnergy), 0.75
	if varM > 1e-8 && t.magEnergy > 1e-8 {
		value += 0.25 * dotM / math.Sqrt(varM*t.magEnergy)
		weight += 0.25
	}
	return clamp(value/weight, -1, 1)
}

func keepPeak(peaks *[]candidate, p candidate) {
	for i, old := range *peaks {
		if abs(old.x-p.x) <= 6 && abs(old.y-p.y) <= 2 {
			if p.score > old.score {
				(*peaks)[i] = p
				sortPeaks(*peaks)
			}
			return
		}
	}
	if len(*peaks) == 12 && p.score <= (*peaks)[11].score {
		return
	}
	*peaks = append(*peaks, p)
	sortPeaks(*peaks)
	if len(*peaks) > 12 {
		*peaks = (*peaks)[:12]
	}
}

func sortPeaks(peaks []candidate) {
	for i := 1; i < len(peaks); i++ {
		for j := i; j > 0 && peaks[j].score > peaks[j-1].score; j-- {
			peaks[j], peaks[j-1] = peaks[j-1], peaks[j]
		}
	}
}

func clamp(value, low, high float64) float64 { return math.Max(low, math.Min(high, value)) }
func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
