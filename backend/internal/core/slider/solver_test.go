package slider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"sync"
	"testing"
)

func referenceRequest(t testing.TB) Request {
	t.Helper()
	encode := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(data)
	}
	return Request{Picture: encode("testdata/reference-picture.png"), Puzzle: encode("testdata/reference-puzzle.png"), PictureWidth: 680}
}

func TestReferenceCloudPuzzle(t *testing.T) {
	result, err := Solve(context.Background(), referenceRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Offset != 392 || result.Confidence < 0.75 {
		t.Fatalf("reference result=%+v", result)
	}
	t.Logf("offset=%d confidence=%.4f peak_margin=%.4f", result.Offset, result.Confidence, result.PeakMargin)
}

func TestCapturedSuccessfulExchangePuzzle(t *testing.T) {
	load := func(name string) string {
		data, err := os.ReadFile("testdata/har-success-" + name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(data)
	}
	result, err := Solve(context.Background(), Request{Picture: load("picture"), Puzzle: load("puzzle"), PictureWidth: 680})
	if err != nil {
		t.Fatal(err)
	}
	// HAR entry 1798 actually succeeded with 546. Allow one pixel of matching
	// error without claiming knowledge of the server's full acceptance range.
	if abs(result.Offset-546) > 1 || result.Confidence < 0.75 {
		t.Fatalf("captured successful offset=546, solver=%+v", result)
	}
	t.Logf("offset=%d confidence=%.4f peak_margin=%.4f", result.Offset, result.Confidence, result.PeakMargin)
}

type fixtureOptions struct {
	offset, width, height           int
	dark, matte, cropped, duplicate bool
}

// The test controls the true cutout location before shading the background.
// The foreground shape, dark details and distractor are independent of solver
// sampling and scoring, so this checks matching behavior rather than internals.
func syntheticRequest(t testing.TB, opts fixtureOptions) Request {
	t.Helper()
	width, height := opts.width, opts.height
	if width == 0 {
		width = 680
	}
	if height == 0 {
		height = 400
	}
	bg := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := 105 + 37*math.Sin(float64(x)*0.13+float64(y)*0.09) + 26*math.Cos(float64(x)*0.071-float64(y)*0.12) + float64((x*31+y*17+x*y*7)%29)
			if opts.dark {
				v = v*0.1 + 3
			}
			bg.SetNRGBA(x, y, color.NRGBA{uint8(v), uint8(v), uint8(v), 255})
		}
	}
	const pieceWidth, pieceHeight = 96, 90
	top := height/2 - 45
	puzzleHeight, puzzleY := height, top
	if opts.cropped {
		puzzleHeight, puzzleY = pieceHeight, 0
	}
	piece := image.NewNRGBA(image.Rect(0, 0, pieceWidth, puzzleHeight))
	if opts.matte {
		for y := 0; y < puzzleHeight; y++ {
			for x := 0; x < pieceWidth; x++ {
				piece.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	inside := func(x, y int) bool {
		return x >= 9 && x < 86 && y >= 8 && y < 83 && !((x-48)*(x-48)+(y-8)*(y-8) < 13*13) && !((x-85)*(x-85)+(y-48)*(y-48) < 13*13)
	}
	for y := 0; y < pieceHeight; y++ {
		for x := 0; x < pieceWidth; x++ {
			if !inside(x, y) {
				continue
			}
			original := bg.NRGBAAt(opts.offset+x, top+y)
			piece.SetNRGBA(x, puzzleY+y, original)
			if opts.duplicate {
				// A second equally good cutout must cause abstention.
				bg.SetNRGBA(420+x, top+y, original)
			}
			shaded := original
			if !opts.dark {
				shaded.R = uint8(float64(original.R)*0.55 + 15)
				shaded.G = shaded.R
				shaded.B = shaded.R
			}
			bg.SetNRGBA(opts.offset+x, top+y, shaded)
			if opts.duplicate {
				bg.SetNRGBA(420+x, top+y, shaded)
			}
		}
	}
	encode := func(img image.Image) string {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(buf.Bytes())
	}
	return Request{Picture: encode(bg), Puzzle: encode(piece), PictureWidth: width}
}

func TestKnownCutoutsAndCoordinateGeometry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  fixtureOptions
		scale int
		uri   bool
	}{
		{"left-outside-reference-range", fixtureOptions{offset: 18}, 1, false},
		{"right-outside-reference-range", fixtureOptions{offset: 566}, 1, false},
		{"transparent-padding", fixtureOptions{offset: 327}, 1, false},
		{"opaque-black-matte", fixtureOptions{offset: 215, matte: true}, 1, false},
		{"dark-texture", fixtureOptions{offset: 282, dark: true}, 1, false},
		{"cropped-unknown-top", fixtureOptions{offset: 327, cropped: true}, 1, false},
		{"declared-coordinate-scale", fixtureOptions{offset: 137, width: 340, height: 200}, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := syntheticRequest(t, tc.opts)
			req.PictureWidth *= tc.scale
			if tc.uri {
				req.Picture = "data:image/png;base64," + req.Picture
				req.Puzzle = "data:image/png;base64," + req.Puzzle
			}
			result, err := Solve(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if result.Offset != tc.opts.offset*tc.scale || result.Confidence < 0.75 {
				t.Fatalf("result=%+v expected=%d", result, tc.opts.offset*tc.scale)
			}
		})
	}
}

func TestAmbiguousCutoutsAreRejected(t *testing.T) {
	req := syntheticRequest(t, fixtureOptions{offset: 180, duplicate: true})
	if result, err := Solve(context.Background(), req); err == nil {
		t.Fatalf("duplicate texture was accepted: %+v", result)
	}
}

func TestJPEGBackgroundAndSemiTransparentPuzzle(t *testing.T) {
	for _, tc := range []struct {
		name        string
		jpegQuality int
	}{
		{"jpeg-quality-50", 50}, {"jpeg-quality-82", 82}, {"jpeg-quality-95", 95}, {"rgba-alpha", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transparent := tc.jpegQuality == 0
			req := syntheticRequest(t, fixtureOptions{offset: 327})
			field := &req.Picture
			if transparent {
				field = &req.Puzzle
			}
			data, err := base64.StdEncoding.DecodeString(*field)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			var encoded bytes.Buffer
			if transparent {
				bounds := img.Bounds()
				out := image.NewNRGBA(bounds)
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					for x := bounds.Min.X; x < bounds.Max.X; x++ {
						c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
						if c.A > 0 {
							c.A = 200
						}
						out.SetNRGBA(x, y, c)
					}
				}
				err = png.Encode(&encoded, out)
			} else {
				err = jpeg.Encode(&encoded, img, &jpeg.Options{Quality: tc.jpegQuality})
			}
			if err != nil {
				t.Fatal(err)
			}
			*field = base64.StdEncoding.EncodeToString(encoded.Bytes())
			result, err := Solve(context.Background(), req)
			if err != nil || abs(result.Offset-327) > 1 || result.Confidence < 0.75 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestCroppedPuzzleUsesDeclaredVerticalPosition(t *testing.T) {
	req := syntheticRequest(t, fixtureOptions{offset: 137, width: 340, height: 200, cropped: true})
	top := 110 // 55 pixels in the decoded 340px image, 110 at server width 680.
	req.PictureWidth = 680
	req.PuzzleTop = &top
	result, err := Solve(context.Background(), req)
	if err != nil || result.Offset != 274 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestOversizedHeaderIsRejectedBeforeImageAllocation(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	binary.BigEndian.PutUint32(data[16:20], 4096)
	binary.BigEndian.PutUint32(data[20:24], 4096)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := decodeRaster(context.Background(), base64.StdEncoding.EncodeToString(data), false); err == nil {
		t.Fatal("oversized PNG header accepted")
	}
}

func TestInvalidInputsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Solve(ctx, Request{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	for _, req := range []Request{{}, {Picture: "not base64", Puzzle: "broken"}, {Picture: "data:image/png,invalid", Puzzle: "invalid"}} {
		if _, err := Solve(context.Background(), req); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	req := syntheticRequest(t, fixtureOptions{offset: 180, cropped: true})
	badTop := 500
	req.PuzzleTop = &badTop
	if _, err := Solve(context.Background(), req); err == nil {
		t.Fatal("invalid vertical position accepted")
	}
}

func TestConcurrentSolvesHaveIndependentResults(t *testing.T) {
	req := referenceRequest(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := Solve(context.Background(), req)
			if err != nil || result.Offset != 392 {
				t.Errorf("concurrent result=%+v err=%v", result, err)
			}
		}()
	}
	wg.Wait()
}

func BenchmarkBuiltinReferencePuzzle(b *testing.B) {
	req := referenceRequest(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Solve(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
}
