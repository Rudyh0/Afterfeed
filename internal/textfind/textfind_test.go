package textfind

import (
	"encoding/binary"
	"image"
	"image/color"
	_ "image/jpeg"
	"math"
	"os"
	"testing"
)

func TestFindCaptionLines(t *testing.T) {
	f, err := os.Open("../media/testdata/caption.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	rgb := NewRGB(img)
	lines, err := d.Find(rgb)
	if err != nil {
		t.Fatal(err)
	}
	// The photo has a caption bar at about y=950 and a sign at y=1530.
	if len(lines) != 2 {
		t.Fatalf("found %d lines, want 2: %+v", len(lines), lines)
	}
	for i, wantY := range []float64{950, 1530} {
		c := lines[i].center()
		if math.Abs(c[1]-wantY) > 40 {
			t.Errorf("line %d at y=%.0f, want about %.0f", i, c[1], wantY)
		}
		if lines[i].width() < 500 {
			t.Errorf("line %d is %.0f wide", i, lines[i].width())
		}
	}
	sheet := Sheet(rgb, lines)
	if sheet == nil || sheet.Rect.Dy() != 2*LineHeight+2*sheetMargin+sheetGap {
		t.Fatalf("sheet %v", sheet.Rect)
	}
	// Both lines end up dark on light, also the white caption on black.
	for i := range 2 {
		y0 := sheetMargin + i*(LineHeight+sheetGap)
		var sum, n float64
		for y := y0; y < y0+LineHeight; y++ {
			for x := sheetMargin; x < sheet.Rect.Dx()-sheetMargin; x++ {
				sum += float64(sheet.GrayAt(x, y).Y)
				n++
			}
		}
		if mean := sum / n; mean < 128 {
			t.Errorf("line %d is mostly dark (%.0f)", i, mean)
		}
	}
}

func TestNoTextInPlainImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(10, 10, color.RGBA{0, 0, 0, 255})
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	lines, err := d.Find(NewRGB(img))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 0 {
		t.Errorf("found %d lines in an empty image", len(lines))
	}
}

func TestMinAreaRect(t *testing.T) {
	// A 100×20 rectangle turned by 30 degrees around (200, 150).
	a := 30 * math.Pi / 180
	var pts [][2]float64
	for _, c := range [][2]float64{{-50, -10}, {50, -10}, {50, 10}, {-50, 10}, {0, 0}, {20, 5}} {
		pts = append(pts, [2]float64{200 + c[0]*math.Cos(a) - c[1]*math.Sin(a), 150 + c[0]*math.Sin(a) + c[1]*math.Cos(a)})
	}
	r := minAreaRect(convexHull(pts))
	w, h := math.Max(r.w, r.h), math.Min(r.w, r.h)
	if math.Abs(w-100) > 0.01 || math.Abs(h-20) > 0.01 || math.Abs(r.cx-200) > 0.01 || math.Abs(r.cy-150) > 0.01 {
		t.Fatalf("rect %+v", r)
	}
	c := r.readingCorners()
	// Top left is left of top right and above bottom left.
	if c[0][0] >= c[1][0] || c[0][1] >= c[3][1] {
		t.Errorf("corners not in reading order: %v", c)
	}
}

func TestModelLoads(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range d.net.steps {
		switch s.op {
		case "BatchNormalization", "Clip", "Constant":
			t.Errorf("%s left in the prepared graph", s.op)
		}
	}
}

// The runner gives the same output as onnxruntime, which made
// testdata/caption_64x256.f32 from the caption bar of the test photo.
func TestSameAsReference(t *testing.T) {
	d, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	x := captionInput(t)
	if os.Getenv("TEXTFIND_WRITE_INPUT") != "" {
		f, _ := os.Create(os.Getenv("TEXTFIND_WRITE_INPUT"))
		binary.Write(f, binary.LittleEndian, x.data)
		f.Close()
	}
	b, err := os.ReadFile("testdata/caption_64x256.f32")
	if err != nil {
		t.Fatal(err)
	}
	y, err := d.net.run(x)
	if err != nil {
		t.Fatal(err)
	}
	if len(y.data)*4 != len(b) {
		t.Fatalf("output has %d values, reference %d", len(y.data), len(b)/4)
	}
	var worst float64
	text := 0
	for i, v := range y.data {
		want := math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
		worst = math.Max(worst, math.Abs(float64(v-want)))
		if want > pixelThreshold {
			text++
		}
	}
	if text < 100 {
		t.Fatalf("the reference has only %d text pixels", text)
	}
	if worst > 1e-3 {
		t.Errorf("differs from the reference by up to %g", worst)
	}
}

// captionInput is the caption bar of the test photo, as model input.
func captionInput(t *testing.T) *tensor {
	f, err := os.Open("../media/testdata/caption.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	rgb := NewRGB(img)
	// Rows 880 to 1015 hold the caption bar.
	bar := &RGB{W: rgb.W, H: 135, Pix: rgb.Pix[3*880*rgb.W : 3*1015*rgb.W]}
	return bar.normalized(256, 64)
}
