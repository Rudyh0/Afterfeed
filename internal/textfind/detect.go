// Package textfind finds lines of text in photos, such as signs, captions,
// screenshots and number plates, with the PP-OCRv3 text detection model.
// Reading the text in the lines is left to Tesseract.
package textfind

import (
	_ "embed"
	"errors"
	"fmt"
	"image"
	"math"
	"sort"
	"sync"
)

//go:embed model/ppocr_v3_det.onnx
var detModel []byte

// Detector finds text lines. It is safe for concurrent use.
type Detector struct {
	net *net
	// MaxSide is the longest side of the image the model looks at.
	MaxSide int
}

var (
	defaultOnce sync.Once
	defaultDet  *Detector
	defaultErr  error
)

// Default returns the detector with the built-in model, loaded once.
func Default() (*Detector, error) {
	defaultOnce.Do(func() {
		g, err := parseModel(detModel)
		if err != nil {
			defaultErr = err
			return
		}
		nt, err := loadNet(g)
		if err != nil {
			defaultErr = err
			return
		}
		defaultDet = &Detector{net: nt, MaxSide: 960}
	})
	return defaultDet, defaultErr
}

// Line is a found text line: four corners in reading order (top left,
// top right, bottom right, bottom left) in pixels of the image.
type Line struct {
	Corners [4][2]float64
	Score   float64
}

// Settings of the DB post-processing, as in PaddleOCR.
const (
	pixelThreshold = 0.3
	boxThreshold   = 0.6
	unclipRatio    = 1.6
	minSide        = 3
	maxLines       = 400
)

// Find returns the text lines in an image, top to bottom.
func (d *Detector) Find(img *RGB) (lines []Line, err error) {
	// An unexpected image must not stop the server.
	defer func() {
		if r := recover(); r != nil {
			lines, err = nil, fmt.Errorf("textfind: %v", r)
		}
	}()
	scale := 1.0
	if m := max(img.W, img.H); m > d.MaxSide {
		scale = float64(d.MaxSide) / float64(m)
	}
	w := max(32, int(math.Round(float64(img.W)*scale/32))*32)
	h := max(32, int(math.Round(float64(img.H)*scale/32))*32)
	x := img.normalized(w, h)
	out, err := d.net.run(x)
	if err != nil {
		return nil, err
	}
	if len(out.shape) != 4 || out.shape[2] != h || out.shape[3] != w {
		return nil, errShape
	}
	lines = boxes(out.data, w, h)
	sx, sy := float64(img.W)/float64(w), float64(img.H)/float64(h)
	for i := range lines {
		for k := range lines[i].Corners {
			lines[i].Corners[k][0] = clamp(lines[i].Corners[k][0]*sx, 0, float64(img.W))
			lines[i].Corners[k][1] = clamp(lines[i].Corners[k][1]*sy, 0, float64(img.H))
		}
	}
	sort.SliceStable(lines, func(i, j int) bool {
		ci, cj := lines[i].center(), lines[j].center()
		// Lines on about the same height are read left to right.
		if math.Abs(ci[1]-cj[1]) < 0.5*min(lines[i].height(), lines[j].height()) {
			return ci[0] < cj[0]
		}
		return ci[1] < cj[1]
	})
	return lines, nil
}

var errShape = errors.New("textfind: unexpected output shape")

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func (l Line) center() [2]float64 {
	var c [2]float64
	for _, p := range l.Corners {
		c[0] += p[0] / 4
		c[1] += p[1] / 4
	}
	return c
}

func (l Line) width() float64  { return dist(l.Corners[0], l.Corners[1]) }
func (l Line) height() float64 { return dist(l.Corners[0], l.Corners[3]) }

func dist(a, b [2]float64) float64 { return math.Hypot(a[0]-b[0], a[1]-b[1]) }

// boxes turns the probability map into text lines: connected areas above
// the threshold, as rotated rectangles, grown by the unclip distance.
func boxes(prob []float32, w, h int) []Line {
	label := make([]int32, w*h)
	var lines []Line
	var queue []int
	next := int32(0)
	for start, p := range prob {
		if p <= pixelThreshold || label[start] != 0 {
			continue
		}
		next++
		// Flood fill the area, keeping the leftmost and rightmost pixel of
		// every row: the convex hull only needs those.
		queue = append(queue[:0], start)
		label[start] = next
		rows := map[int][2]int{}
		for q := 0; q < len(queue); q++ {
			i := queue[q]
			x, y := i%w, i/w
			if r, ok := rows[y]; ok {
				rows[y] = [2]int{min(r[0], x), max(r[1], x)}
			} else {
				rows[y] = [2]int{x, x}
			}
			for dy := -1; dy <= 1; dy++ {
				ny := y + dy
				if ny < 0 || ny >= h {
					continue
				}
				for dx := -1; dx <= 1; dx++ {
					nx := x + dx
					if nx < 0 || nx >= w || (dx == 0 && dy == 0) {
						continue
					}
					j := ny*w + nx
					if label[j] == 0 && prob[j] > pixelThreshold {
						label[j] = next
						queue = append(queue, j)
					}
				}
			}
		}
		if len(queue) < 4 {
			continue
		}
		pts := make([][2]float64, 0, 4*len(rows))
		for y, r := range rows {
			fy := float64(y)
			pts = append(pts, [2]float64{float64(r[0]), fy}, [2]float64{float64(r[0]), fy + 1},
				[2]float64{float64(r[1] + 1), fy}, [2]float64{float64(r[1] + 1), fy + 1})
		}
		rect := minAreaRect(convexHull(pts))
		if math.Min(rect.w, rect.h) < minSide {
			continue
		}
		score := rect.meanInside(prob, w, h)
		if score < boxThreshold {
			continue
		}
		// Grow by area·ratio/perimeter on every side, like pyclipper's
		// offset in PaddleOCR.
		dd := rect.w * rect.h * unclipRatio / (2 * (rect.w + rect.h))
		rect.w += 2 * dd
		rect.h += 2 * dd
		if math.Min(rect.w, rect.h) < minSide+2 {
			continue
		}
		lines = append(lines, Line{Corners: rect.readingCorners(), Score: score})
		if len(lines) >= maxLines {
			break
		}
	}
	return lines
}

// convexHull returns the hull of the points, counter-clockwise (monotone
// chain).
func convexHull(pts [][2]float64) [][2]float64 {
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][0] != pts[j][0] {
			return pts[i][0] < pts[j][0]
		}
		return pts[i][1] < pts[j][1]
	})
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	hull := make([][2]float64, 0, 2*len(pts))
	for _, p := range pts {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	lower := len(hull) + 1
	for i := len(pts) - 2; i >= 0; i-- {
		p := pts[i]
		for len(hull) >= lower && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	return hull[:len(hull)-1]
}

// rotRect is a rectangle with sides w (along angle) and h.
type rotRect struct {
	cx, cy float64
	w, h   float64
	// ux, uy is the unit vector along the w side.
	ux, uy float64
}

// minAreaRect finds the smallest rectangle around a convex hull: one of
// its sides lies on a hull edge (rotating calipers).
func minAreaRect(hull [][2]float64) rotRect {
	best := rotRect{w: math.Inf(1), h: 1}
	n := len(hull)
	for i := 0; i < n; i++ {
		a, b := hull[i], hull[(i+1)%n]
		ex, ey := b[0]-a[0], b[1]-a[1]
		l := math.Hypot(ex, ey)
		if l == 0 {
			continue
		}
		ux, uy := ex/l, ey/l
		minU, maxU, minV, maxV := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, p := range hull {
			u := p[0]*ux + p[1]*uy
			v := -p[0]*uy + p[1]*ux
			minU, maxU = math.Min(minU, u), math.Max(maxU, u)
			minV, maxV = math.Min(minV, v), math.Max(maxV, v)
		}
		w, h := maxU-minU, maxV-minV
		if w*h < best.w*best.h {
			cu, cv := (minU+maxU)/2, (minV+maxV)/2
			best = rotRect{cx: cu*ux - cv*uy, cy: cu*uy + cv*ux, w: w, h: h, ux: ux, uy: uy}
		}
	}
	if math.IsInf(best.w, 1) {
		return rotRect{}
	}
	return best
}

// meanInside is the mean probability of the pixels inside the rectangle.
func (r rotRect) meanInside(prob []float32, w, h int) float64 {
	c := r.corners()
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range c {
		x0, y0 = math.Min(x0, p[0]), math.Min(y0, p[1])
		x1, y1 = math.Max(x1, p[0]), math.Max(y1, p[1])
	}
	var sum float64
	n := 0
	for y := max(0, int(y0)); y < min(h, int(math.Ceil(y1))); y++ {
		for x := max(0, int(x0)); x < min(w, int(math.Ceil(x1))); x++ {
			px, py := float64(x)+0.5-r.cx, float64(y)+0.5-r.cy
			u := px*r.ux + py*r.uy
			v := -px*r.uy + py*r.ux
			if math.Abs(u) <= r.w/2 && math.Abs(v) <= r.h/2 {
				sum += float64(prob[y*w+x])
				n++
			}
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func (r rotRect) corners() [4][2]float64 {
	hw, hh := r.w/2, r.h/2
	// v axis is (-uy, ux).
	var c [4][2]float64
	for i, s := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		u, v := s[0]*hw, s[1]*hh
		c[i] = [2]float64{r.cx + u*r.ux - v*r.uy, r.cy + u*r.uy + v*r.ux}
	}
	return c
}

// readingCorners returns the corners with the long side as the width, in
// reading order: top left, top right, bottom right, bottom left.
func (r rotRect) readingCorners() [4][2]float64 {
	if r.h > r.w {
		// Make the long side run along u.
		r.w, r.h = r.h, r.w
		r.ux, r.uy = -r.uy, r.ux
	}
	// Text runs left to right: u points right.
	if r.ux < 0 || (r.ux == 0 && r.uy > 0) {
		r.ux, r.uy = -r.ux, -r.uy
	}
	// With u pointing right, v = (-uy, ux) points down in image space.
	return r.corners()
}

// RGB is an image as 8-bit red, green and blue values.
type RGB struct {
	W, H int
	Pix  []uint8
}

// NewRGB copies an image.
func NewRGB(img image.Image) *RGB {
	b := img.Bounds()
	out := &RGB{W: b.Dx(), H: b.Dy(), Pix: make([]uint8, 3*b.Dx()*b.Dy())}
	switch src := img.(type) {
	case *image.NRGBA:
		for y := 0; y < out.H; y++ {
			row := src.Pix[(y)*src.Stride:]
			dst := out.Pix[3*y*out.W:]
			for x := 0; x < out.W; x++ {
				// Transparent areas are white, like paper.
				a := uint32(row[4*x+3])
				for k := 0; k < 3; k++ {
					dst[3*x+k] = uint8((uint32(row[4*x+k])*a + 255*(255-a)) / 255)
				}
			}
		}
	case *image.RGBA:
		for y := 0; y < out.H; y++ {
			row := src.Pix[y*src.Stride:]
			dst := out.Pix[3*y*out.W:]
			for x := 0; x < out.W; x++ {
				a := uint32(row[4*x+3])
				for k := 0; k < 3; k++ {
					dst[3*x+k] = uint8(min(255, uint32(row[4*x+k])+(255-a)))
				}
			}
		}
	case *image.Gray:
		for y := 0; y < out.H; y++ {
			row := src.Pix[y*src.Stride:]
			dst := out.Pix[3*y*out.W:]
			for x := 0; x < out.W; x++ {
				dst[3*x], dst[3*x+1], dst[3*x+2] = row[x], row[x], row[x]
			}
		}
	default:
		i := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := img.At(x, y).RGBA()
				w := 0xffff - a
				out.Pix[i] = uint8((r + w) >> 8)
				out.Pix[i+1] = uint8((g + w) >> 8)
				out.Pix[i+2] = uint8((bl + w) >> 8)
				i += 3
			}
		}
	}
	return out
}

// sample returns the colour at a point with bilinear interpolation.
func (m *RGB) sample(x, y float64) (r, g, b float64) {
	x = clamp(x-0.5, 0, float64(m.W-1))
	y = clamp(y-0.5, 0, float64(m.H-1))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, m.W-1), min(y0+1, m.H-1)
	fx, fy := x-float64(x0), y-float64(y0)
	var c [3]float64
	for k := 0; k < 3; k++ {
		p00 := float64(m.Pix[3*(y0*m.W+x0)+k])
		p10 := float64(m.Pix[3*(y0*m.W+x1)+k])
		p01 := float64(m.Pix[3*(y1*m.W+x0)+k])
		p11 := float64(m.Pix[3*(y1*m.W+x1)+k])
		c[k] = (p00*(1-fx)+p10*fx)*(1-fy) + (p01*(1-fx)+p11*fx)*fy
	}
	return c[0], c[1], c[2]
}

// normalized scales the image to w×h and normalises it as the model was
// trained: RGB, ImageNet mean and deviation, NCHW. When the image is made
// smaller, every target pixel is the mean of its source area, so thin
// letters do not break up.
func (m *RGB) normalized(w, h int) *tensor {
	t := newTensor(1, 3, h, w)
	mean := [3]float32{0.485, 0.456, 0.406}
	std := [3]float32{0.229, 0.224, 0.225}
	var scale, shift [3]float32
	for k := range scale {
		scale[k] = 1 / (255 * std[k])
		shift[k] = -mean[k] / std[k]
	}
	sx, sy := float64(m.W)/float64(w), float64(m.H)/float64(h)
	plane := w * h
	if sx <= 1 && sy <= 1 {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b := m.sample((float64(x)+0.5)*sx, (float64(y)+0.5)*sy)
				i := y*w + x
				t.data[i] = float32(r)*scale[0] + shift[0]
				t.data[plane+i] = float32(g)*scale[1] + shift[1]
				t.data[2*plane+i] = float32(b)*scale[2] + shift[2]
			}
		}
		return t
	}
	// Source columns of every target column.
	xa, xb := make([]int, w), make([]int, w)
	for x := 0; x < w; x++ {
		xa[x] = min(m.W-1, int(float64(x)*sx))
		xb[x] = max(xa[x]+1, min(m.W, int(math.Ceil(float64(x+1)*sx))))
	}
	cols := make([]uint32, 3*m.W)
	for y := 0; y < h; y++ {
		ya := min(m.H-1, int(float64(y)*sy))
		yb := max(ya+1, min(m.H, int(math.Ceil(float64(y+1)*sy))))
		clear(cols)
		for sy := ya; sy < yb; sy++ {
			row := m.Pix[3*sy*m.W : 3*(sy+1)*m.W]
			for i, v := range row {
				cols[i] += uint32(v)
			}
		}
		rows := float32(yb - ya)
		for x := 0; x < w; x++ {
			var c [3]uint32
			for sx := xa[x]; sx < xb[x]; sx++ {
				c[0] += cols[3*sx]
				c[1] += cols[3*sx+1]
				c[2] += cols[3*sx+2]
			}
			n := rows * float32(xb[x]-xa[x])
			i := y*w + x
			t.data[i] = float32(c[0])/n*scale[0] + shift[0]
			t.data[plane+i] = float32(c[1])/n*scale[1] + shift[1]
			t.data[2*plane+i] = float32(c[2])/n*scale[2] + shift[2]
		}
	}
	return t
}
