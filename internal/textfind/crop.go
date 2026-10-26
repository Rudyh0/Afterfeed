package textfind

import (
	"image"
	"math"
	"sort"
)

// LineHeight is the height of a line in the image made by Sheet.
const LineHeight = 64

const (
	sheetMargin = 24
	sheetGap    = 24
	sheetWidth  = 2400
)

// Sheet puts the found lines below each other on a white sheet, straightened,
// in grey with dark text, so a text recogniser can read them in one go.
func Sheet(img *RGB, lines []Line) (sheet *image.Gray) {
	defer func() {
		if recover() != nil {
			sheet = nil
		}
	}()
	strips := make([]*image.Gray, 0, len(lines))
	width := 0
	for _, l := range lines {
		s := strip(img, l)
		if s == nil {
			continue
		}
		strips = append(strips, s)
		width = max(width, s.Rect.Dx())
	}
	if len(strips) == 0 {
		return nil
	}
	height := sheetMargin
	for _, s := range strips {
		height += s.Rect.Dy() + sheetGap
	}
	sheet = image.NewGray(image.Rect(0, 0, width+2*sheetMargin, height+sheetMargin-sheetGap))
	for i := range sheet.Pix {
		sheet.Pix[i] = 255
	}
	y := sheetMargin
	for _, s := range strips {
		for r := 0; r < s.Rect.Dy(); r++ {
			copy(sheet.Pix[(y+r)*sheet.Stride+sheetMargin:], s.Pix[r*s.Stride:r*s.Stride+s.Rect.Dx()])
		}
		y += s.Rect.Dy() + sheetGap
	}
	return sheet
}

// strip cuts out one line, straightened to LineHeight pixels high.
func strip(img *RGB, l Line) *image.Gray {
	w, h := l.width(), l.height()
	if w < 2 || h < 2 {
		return nil
	}
	ow := int(math.Round(w * LineHeight / h))
	oh := LineHeight
	if ow > sheetWidth {
		// A very long line is made lower rather than cut off.
		oh = max(16, LineHeight*sheetWidth/ow)
		ow = sheetWidth
	}
	ow = max(ow, 8)
	tl, tr, bl := l.Corners[0], l.Corners[1], l.Corners[3]
	ux, uy := (tr[0]-tl[0])/float64(ow), (tr[1]-tl[1])/float64(ow)
	vx, vy := (bl[0]-tl[0])/float64(oh), (bl[1]-tl[1])/float64(oh)
	// Going down more than one source pixel per output pixel: average.
	step := max(1, int(math.Ceil(math.Hypot(vx, vy))))
	vals := make([]float64, ow*oh)
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			var sum float64
			for k := 0; k < step; k++ {
				fy := float64(y) + (float64(k)+0.5)/float64(step)
				fx := float64(x) + 0.5
				sx := tl[0] + fx*ux + fy*vx
				sy := tl[1] + fx*uy + fy*vy
				r, g, b := img.sample(sx, sy)
				sum += 0.299*r + 0.587*g + 0.114*b
			}
			vals[y*ow+x] = sum / float64(step)
		}
	}
	// Stretch the contrast between the darkest and lightest few percent,
	// and make the text dark when it is light on a dark background: the
	// edge of the strip is mostly background.
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	lo, hi := sorted[len(sorted)*2/100], sorted[len(sorted)*98/100]
	if hi-lo < 16 {
		return nil
	}
	var edge, all float64
	ne := 0
	for y := 0; y < oh; y++ {
		for x := 0; x < ow; x++ {
			v := vals[y*ow+x]
			all += v
			if y < 3 || y >= oh-3 || x < 3 || x >= ow-3 {
				edge += v
				ne++
			}
		}
	}
	invert := edge/float64(ne) < all/float64(len(vals))
	out := image.NewGray(image.Rect(0, 0, ow, oh))
	for i, v := range vals {
		n := (v - lo) / (hi - lo)
		n = math.Max(0, math.Min(1, n))
		if invert {
			n = 1 - n
		}
		out.Pix[i] = uint8(n*255 + 0.5)
	}
	return out
}
