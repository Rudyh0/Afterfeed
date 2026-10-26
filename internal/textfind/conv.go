package textfind

import (
	"errors"
	"fmt"
	"math"
)

// convParams are the prepared weights and settings of a convolution.
type convParams struct {
	transpose bool
	// m output channels, cg input channels per group, kernel kh×kw.
	m, cg, kh, kw int
	group         int
	sh, sw        int
	pt, pl        int
	pb, pr        int
	// w is [m][cg·kh·kw] for a convolution and [c][m/group][kh][kw] for a
	// transposed one.
	w []float32
	b []float32
}

func prepareConv(s *step) (*convParams, error) {
	if len(s.in) < 2 || s.in[1] == nil {
		return nil, errors.New("conv: weights must be constant")
	}
	wt := s.in[1]
	if len(wt.shape) != 4 {
		return nil, fmt.Errorf("conv: weights of shape %v", wt.shape)
	}
	p := &convParams{transpose: s.op == "ConvTranspose", group: 1, sh: 1, sw: 1}
	if a, ok := s.attrs["group"]; ok && a.i > 0 {
		p.group = int(a.i)
	}
	if a, ok := s.attrs["strides"]; ok && len(a.ints) == 2 {
		p.sh, p.sw = int(a.ints[0]), int(a.ints[1])
	}
	if a, ok := s.attrs["pads"]; ok && len(a.ints) == 4 {
		p.pt, p.pl, p.pb, p.pr = int(a.ints[0]), int(a.ints[1]), int(a.ints[2]), int(a.ints[3])
	}
	if a, ok := s.attrs["dilations"]; ok {
		for _, d := range a.ints {
			if d != 1 {
				return nil, errors.New("conv: dilation is not supported")
			}
		}
	}
	if a, ok := s.attrs["auto_pad"]; ok && a.s != "" && a.s != "NOTSET" {
		return nil, fmt.Errorf("conv: auto_pad %s is not supported", a.s)
	}
	p.kh, p.kw = wt.shape[2], wt.shape[3]
	p.w = append([]float32(nil), wt.data...)
	if p.transpose {
		if p.group != 1 {
			return nil, errors.New("conv transpose: groups are not supported")
		}
		p.cg = wt.shape[0]
		p.m = wt.shape[1]
	} else {
		p.m = wt.shape[0]
		p.cg = wt.shape[1]
	}
	p.b = make([]float32, p.m)
	if len(s.in) > 2 && s.in[2] != nil {
		if len(s.in[2].data) != p.m {
			return nil, errors.New("conv: bias size")
		}
		copy(p.b, s.in[2].data)
	}
	return p, nil
}

// foldBatchNorm applies a following batch normalisation to the weights.
func (p *convParams) foldBatchNorm(bn node, consts map[string]*tensor) error {
	if len(bn.inputs) < 5 {
		return errors.New("batch norm inputs")
	}
	var in [4]*tensor
	for i := range in {
		in[i] = consts[bn.inputs[i+1]]
		if in[i] == nil || len(in[i].data) != p.m {
			return errors.New("batch norm: parameters must be constant")
		}
	}
	eps := float32(1e-5)
	if a, ok := bn.attrs["epsilon"]; ok {
		eps = a.f
	}
	scale, bias, mean, vr := in[0].data, in[1].data, in[2].data, in[3].data
	kk := p.kh * p.kw
	for m := 0; m < p.m; m++ {
		k := scale[m] / float32(math.Sqrt(float64(vr[m]+eps)))
		if p.transpose {
			// Weights are [c][m][kh][kw].
			for c := 0; c < p.cg; c++ {
				w := p.w[(c*p.m+m)*kk:][:kk]
				for i := range w {
					w[i] *= k
				}
			}
		} else {
			w := p.w[m*p.cg*kk:][:p.cg*kk]
			for i := range w {
				w[i] *= k
			}
		}
		p.b[m] = (p.b[m]-mean[m])*k + bias[m]
	}
	return nil
}

// foldBias adds a following per-channel constant to the bias. It reports
// whether add is such an addition.
func (p *convParams) foldBias(add node, out string, consts map[string]*tensor) bool {
	if len(add.inputs) != 2 {
		return false
	}
	var c *tensor
	switch out {
	case add.inputs[0]:
		c = consts[add.inputs[1]]
	case add.inputs[1]:
		c = consts[add.inputs[0]]
	}
	if c == nil || len(c.data) != p.m {
		return false
	}
	// Only one value per output channel: [m], [m,1,1] or [1,m,1,1].
	switch sh := c.shape; len(sh) {
	case 1:
	case 3:
		if sh[1] != 1 || sh[2] != 1 {
			return false
		}
	case 4:
		if sh[0] != 1 || sh[2] != 1 || sh[3] != 1 {
			return false
		}
	default:
		return false
	}
	for m := range p.b {
		p.b[m] += c.data[m]
	}
	return true
}

func (p *convParams) conv(x *tensor, act string) (*tensor, error) {
	n, c, h, w, err := x.dims4()
	if err != nil {
		return nil, err
	}
	if c != p.cg*p.group {
		return nil, fmt.Errorf("conv: %d input channels, weights for %d", c, p.cg*p.group)
	}
	oh := (h+p.pt+p.pb-p.kh)/p.sh + 1
	ow := (w+p.pl+p.pr-p.kw)/p.sw + 1
	if oh <= 0 || ow <= 0 {
		return nil, errors.New("conv: input too small")
	}
	out := newTensor(n, p.m, oh, ow)
	plane := oh * ow
	for b := 0; b < n; b++ {
		xs := x.data[b*c*h*w : (b+1)*c*h*w]
		os := out.data[b*p.m*plane : (b+1)*p.m*plane]
		for m := 0; m < p.m; m++ {
			fill(os[m*plane:(m+1)*plane], p.b[m])
		}
		switch {
		case p.group == c && p.cg == 1 && p.m == c:
			p.depthwise(xs, os, h, w, oh, ow)
		case p.kh == 1 && p.kw == 1 && p.sh == 1 && p.sw == 1 && p.pt+p.pl+p.pb+p.pr == 0 && p.group == 1:
			gemm(p.m, plane, c, p.w, xs, plane, os, plane)
		default:
			p.im2colConv(xs, os, c, h, w, oh, ow)
		}
	}
	applyAct(out.data, act)
	return out, nil
}

func fill(s []float32, v float32) {
	for i := range s {
		s[i] = v
	}
}

// depthwise convolves every channel with its own kernel.
func (p *convParams) depthwise(xs, os []float32, h, w, oh, ow int) {
	k := p.kh * p.kw
	for ch := 0; ch < p.m; ch++ {
		src := xs[ch*h*w : (ch+1)*h*w]
		dst := os[ch*oh*ow : (ch+1)*oh*ow]
		wk := p.w[ch*k : (ch+1)*k]
		for oy := 0; oy < oh; oy++ {
			drow := dst[oy*ow : (oy+1)*ow]
			for ky := 0; ky < p.kh; ky++ {
				iy := oy*p.sh + ky - p.pt
				if iy < 0 || iy >= h {
					continue
				}
				srow := src[iy*w : (iy+1)*w]
				for kx := 0; kx < p.kw; kx++ {
					wv := wk[ky*p.kw+kx]
					lo, hi := validRange(ow, w, p.sw, kx-p.pl)
					if p.sw == 1 {
						off := kx - p.pl
						s := srow[lo+off : hi+off]
						d := drow[lo:hi]
						d = d[:len(s)]
						for i, v := range s {
							d[i] += wv * v
						}
					} else {
						for ox := lo; ox < hi; ox++ {
							drow[ox] += wv * srow[ox*p.sw+kx-p.pl]
						}
					}
				}
			}
		}
	}
}

// validRange returns the output columns [lo, hi) whose input column
// ox·stride+off lies within 0..w-1.
func validRange(ow, w, stride, off int) (int, int) {
	lo := 0
	if off < 0 {
		lo = (-off + stride - 1) / stride
	}
	hi := ow
	if last := (w - 1 - off); last < (ow-1)*stride {
		if last < 0 {
			return 0, 0
		}
		hi = last/stride + 1
	}
	if lo > hi {
		return 0, 0
	}
	return lo, hi
}

// im2colConv convolves a few output rows at a time: the input pixels under
// the kernel are copied into columns, then multiplied with the weights.
func (p *convParams) im2colConv(xs, os []float32, c, h, w, oh, ow int) {
	k := p.cg * p.kh * p.kw
	// Enough rows for a few hundred columns, within a buffer that stays in
	// the cache.
	rows := max(1, min(oh, 512/ow))
	col := make([]float32, k*rows*ow)
	mg := p.m / p.group
	for g := 0; g < p.group; g++ {
		for oy0 := 0; oy0 < oh; oy0 += rows {
			r := min(rows, oh-oy0)
			ncol := r * ow
			for ci := 0; ci < p.cg; ci++ {
				src := xs[(g*p.cg+ci)*h*w:]
				for ky := 0; ky < p.kh; ky++ {
					for kx := 0; kx < p.kw; kx++ {
						dst := col[((ci*p.kh+ky)*p.kw+kx)*ncol:][:ncol]
						lo, hi := validRange(ow, w, p.sw, kx-p.pl)
						for rr := 0; rr < r; rr++ {
							d := dst[rr*ow : (rr+1)*ow]
							iy := (oy0+rr)*p.sh + ky - p.pt
							if iy < 0 || iy >= h || lo >= hi {
								fill(d, 0)
								continue
							}
							srow := src[iy*w : (iy+1)*w]
							fill(d[:lo], 0)
							fill(d[hi:], 0)
							if p.sw == 1 {
								copy(d[lo:hi], srow[lo+kx-p.pl:])
							} else {
								for ox := lo; ox < hi; ox++ {
									d[ox] = srow[ox*p.sw+kx-p.pl]
								}
							}
						}
					}
				}
			}
			// out[m][oy0·ow : …] += W[m][k] · col[k][…]
			gemm(mg, ncol, k, p.w[g*mg*k:], col, ncol, os[g*mg*oh*ow+oy0*ow:], oh*ow)
		}
	}
}

// convTranspose spreads every input pixel over the kernel area of the
// output, as in the upsampling of a detection head.
func (p *convParams) convTranspose(x *tensor, act string) (*tensor, error) {
	n, c, h, w, err := x.dims4()
	if err != nil {
		return nil, err
	}
	if c != p.cg {
		return nil, fmt.Errorf("conv transpose: %d input channels, weights for %d", c, p.cg)
	}
	oh := (h-1)*p.sh + p.kh - p.pt - p.pb
	ow := (w-1)*p.sw + p.kw - p.pl - p.pr
	out := newTensor(n, p.m, oh, ow)
	plane := oh * ow
	for b := 0; b < n; b++ {
		xs := x.data[b*c*h*w : (b+1)*c*h*w]
		os := out.data[b*p.m*plane : (b+1)*p.m*plane]
		for m := 0; m < p.m; m++ {
			dst := os[m*plane : (m+1)*plane]
			fill(dst, p.b[m])
			for ci := 0; ci < c; ci++ {
				src := xs[ci*h*w : (ci+1)*h*w]
				wk := p.w[(ci*p.m+m)*p.kh*p.kw:]
				for ky := 0; ky < p.kh; ky++ {
					for kx := 0; kx < p.kw; kx++ {
						wv := wk[ky*p.kw+kx]
						for iy := 0; iy < h; iy++ {
							oy := iy*p.sh + ky - p.pt
							if oy < 0 || oy >= oh {
								continue
							}
							srow := src[iy*w : (iy+1)*w]
							drow := dst[oy*ow : (oy+1)*ow]
							for ix, v := range srow {
								ox := ix*p.sw + kx - p.pl
								if ox >= 0 && ox < ow {
									drow[ox] += wv * v
								}
							}
						}
					}
				}
			}
		}
	}
	applyAct(out.data, act)
	return out, nil
}

func applyAct(data []float32, act string) {
	if act == "relu" {
		for i, v := range data {
			if v < 0 {
				data[i] = 0
			}
		}
	}
}

// gemm computes c[i][j] += Σ a[i][p]·b[p][j] for an m×k matrix a (rows of
// k values), a k×n matrix b with rows ldb apart and c with rows ldc apart.
// Four rows of c are updated together, over blocks of columns that keep
// their part of c in the cache.
func gemm(m, n, k int, a, b []float32, ldb int, c []float32, ldc int) {
	const nb = 256
	for j0 := 0; j0 < n; j0 += nb {
		j1 := min(j0+nb, n)
		width := j1 - j0
		i := 0
		for ; i+4 <= m; i += 4 {
			c0 := c[i*ldc+j0:][:width]
			c1 := c[(i+1)*ldc+j0:][:width]
			c2 := c[(i+2)*ldc+j0:][:width]
			c3 := c[(i+3)*ldc+j0:][:width]
			a0, a1, a2, a3 := a[i*k:][:k], a[(i+1)*k:][:k], a[(i+2)*k:][:k], a[(i+3)*k:][:k]
			for q := 0; q < k; q++ {
				w0, w1, w2, w3 := a0[q], a1[q], a2[q], a3[q]
				br := b[q*ldb+j0:][:width]
				for j, v := range br {
					c0[j] += w0 * v
					c1[j] += w1 * v
					c2[j] += w2 * v
					c3[j] += w3 * v
				}
			}
		}
		for ; i < m; i++ {
			ci := c[i*ldc+j0:][:width]
			ai := a[i*k:][:k]
			for q := 0; q < k; q++ {
				wv := ai[q]
				br := b[q*ldb+j0:][:width]
				for j, v := range br {
					ci[j] += wv * v
				}
			}
		}
	}
}
