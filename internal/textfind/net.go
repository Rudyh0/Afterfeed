package textfind

// A small runner for the convolutional networks used here: the operations
// of the text detection model, on one image at a time, on one core. It is
// plain Go so the image needs no machine-learning libraries.

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// tensor holds float values in data, or integer values (shapes, sizes) in
// ints. Images are in NCHW order.
type tensor struct {
	shape []int
	data  []float32
	ints  []int64
}

func (t *tensor) size() int {
	n := 1
	for _, d := range t.shape {
		n *= d
	}
	return n
}

// dims4 returns the shape as N, C, H, W.
func (t *tensor) dims4() (n, c, h, w int, err error) {
	if len(t.shape) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("want 4 dimensions, have %v", t.shape)
	}
	return t.shape[0], t.shape[1], t.shape[2], t.shape[3], nil
}

func newTensor(shape ...int) *tensor {
	t := &tensor{shape: shape}
	t.data = make([]float32, t.size())
	return t
}

// step is a node prepared for running.
type step struct {
	node
	// in are the constant inputs, or nil for values made while running.
	in []*tensor
	// conv holds the prepared weights of a Conv or ConvTranspose.
	conv *convParams
	// act is an activation applied to the output of a Conv.
	act string
}

// net is a prepared graph.
type net struct {
	steps  []step
	input  string
	output string
	// lastUse is the index of the last step that reads each value.
	lastUse map[string]int
}

// loadNet prepares a graph: constants are computed once, and batch
// normalisation, biases and ReLU are folded into the convolutions.
func loadNet(g *graph) (*net, error) {
	consts := map[string]*tensor{}
	for k, v := range g.consts {
		consts[k] = v
	}
	uses := map[string]int{}
	for _, n := range g.nodes {
		for _, in := range n.inputs {
			uses[in]++
		}
	}
	for _, o := range g.outputs {
		uses[o]++
	}
	// Values computed from constants only are constants too.
	var live []node
	for _, n := range g.nodes {
		if n.op == "Constant" {
			if a, ok := n.attrs["value"]; ok && a.t != nil {
				consts[n.outputs[0]] = a.t
				continue
			}
			return nil, errors.New("onnx: Constant without a tensor")
		}
		allConst := len(n.inputs) > 0
		for _, in := range n.inputs {
			if in != "" && consts[in] == nil {
				allConst = false
			}
		}
		if allConst && n.op != "Conv" && n.op != "ConvTranspose" {
			ins := make([]*tensor, len(n.inputs))
			for i, in := range n.inputs {
				ins[i] = consts[in]
			}
			out, err := runOp(&step{node: n, in: ins}, ins)
			if err != nil {
				return nil, fmt.Errorf("fold %s: %w", n.op, err)
			}
			consts[n.outputs[0]] = out
			continue
		}
		live = append(live, n)
	}

	live = fuseHardSwish(live, consts, uses)

	// Find the node that consumes a value, when there is exactly one.
	consumer := func(name string, from int) int {
		if uses[name] != 1 {
			return -1
		}
		for j := from; j < len(live); j++ {
			if slices.Contains(live[j].inputs, name) {
				return j
			}
		}
		return -1
	}

	var steps []step
	skip := map[int]bool{}
	for i, n := range live {
		if skip[i] {
			continue
		}
		s := step{node: n, in: make([]*tensor, len(n.inputs))}
		for k, in := range n.inputs {
			s.in[k] = consts[in]
		}
		if n.op == "Conv" || n.op == "ConvTranspose" {
			p, err := prepareConv(&s)
			if err != nil {
				return nil, err
			}
			s.conv = p
			out := n.outputs[0]
			// Batch normalisation, a per-channel bias and ReLU after the
			// convolution are applied by it.
			for {
				j := consumer(out, i+1)
				if j < 0 {
					break
				}
				next := live[j]
				if next.op == "BatchNormalization" && next.inputs[0] == out && s.act == "" {
					if err := p.foldBatchNorm(next, consts); err != nil {
						return nil, err
					}
				} else if next.op == "Add" && s.act == "" && p.foldBias(next, out, consts) {
				} else if next.op == "Relu" && s.act == "" {
					s.act = "relu"
				} else {
					break
				}
				skip[j] = true
				out = next.outputs[0]
			}
			s.outputs = []string{out}
		}
		steps = append(steps, s)
	}

	if len(g.inputs) == 0 || len(g.outputs) != 1 {
		return nil, errors.New("onnx: want one input and one output")
	}
	nt := &net{steps: steps, output: g.outputs[0], lastUse: map[string]int{}}
	for _, in := range g.inputs {
		if consts[in] == nil {
			nt.input = in
		}
	}
	for i, s := range steps {
		for k, in := range s.inputs {
			if s.in[k] == nil {
				nt.lastUse[in] = i
			}
		}
	}
	return nt, nil
}

// run computes the output for one input.
func (nt *net) run(x *tensor) (*tensor, error) {
	vals := map[string]*tensor{nt.input: x}
	for i := range nt.steps {
		s := &nt.steps[i]
		ins := make([]*tensor, len(s.inputs))
		for k, in := range s.inputs {
			if s.in[k] != nil {
				ins[k] = s.in[k]
				continue
			}
			if in == "" {
				continue
			}
			v := vals[in]
			if v == nil {
				return nil, fmt.Errorf("%s: missing input %s", s.op, in)
			}
			ins[k] = v
		}
		out, err := runOp(s, ins)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.op, err)
		}
		vals[s.outputs[0]] = out
		// Free what is no longer needed.
		for _, in := range s.inputs {
			if last, ok := nt.lastUse[in]; ok && last == i && in != nt.output {
				delete(vals, in)
			}
		}
	}
	out := vals[nt.output]
	if out == nil {
		return nil, errors.New("no output")
	}
	return out, nil
}

func runOp(s *step, in []*tensor) (*tensor, error) {
	switch s.op {
	case "Conv":
		return s.conv.conv(in[0], s.act)
	case "ConvTranspose":
		return s.conv.convTranspose(in[0], s.act)
	case "BatchNormalization":
		return batchNorm(in, s.attrs)
	case "Add", "Mul", "Div", "Sub":
		return elementwise(s.op, in[0], in[1])
	case "Relu":
		return unary(in[0], func(v float32) float32 { return max(v, 0) }), nil
	case "HardSwish":
		return hardSwish(in[0]), nil
	case "Sigmoid":
		return unary(in[0], func(v float32) float32 { return float32(1 / (1 + math.Exp(-float64(v)))) }), nil
	case "HardSigmoid":
		alpha, beta := float32(0.2), float32(0.5)
		if a, ok := s.attrs["alpha"]; ok {
			alpha = a.f
		}
		if a, ok := s.attrs["beta"]; ok {
			beta = a.f
		}
		return unary(in[0], func(v float32) float32 { return min(max(alpha*v+beta, 0), 1) }), nil
	case "Clip":
		lo, hi := float32(math.Inf(-1)), float32(math.Inf(1))
		if len(in) > 1 && in[1] != nil {
			lo = in[1].data[0]
		}
		if len(in) > 2 && in[2] != nil {
			hi = in[2].data[0]
		}
		return unary(in[0], func(v float32) float32 { return min(max(v, lo), hi) }), nil
	case "Reshape":
		return reshape(in[0], in[1])
	case "GlobalAveragePool":
		return globalAveragePool(in[0])
	case "Resize":
		return resizeNearest(in, s.attrs)
	case "Concat":
		return concat(in, s.attrs)
	}
	return nil, fmt.Errorf("unsupported operation %s", s.op)
}

func unary(x *tensor, f func(float32) float32) *tensor {
	out := &tensor{shape: x.shape, data: make([]float32, len(x.data))}
	for i, v := range x.data {
		out.data[i] = f(v)
	}
	return out
}

// elementwise applies an operation with the broadcasting used by
// these models: equal shapes, a single value, or one value per channel.
func elementwise(op string, a, b *tensor) (*tensor, error) {
	big, small, swapped := a, b, false
	if b.size() > a.size() {
		big, small, swapped = b, a, true
	}
	out := &tensor{shape: big.shape, data: make([]float32, len(big.data))}
	switch {
	case len(small.data) == len(big.data):
		vecOp(op, swapped, out.data, big.data, small.data)
	case len(small.data) == 1:
		scalarOp(op, swapped, out.data, big.data, small.data[0])
	default:
		// small is [N?, C, 1, 1]: one value per image and channel.
		if len(big.shape) != 4 {
			return nil, fmt.Errorf("cannot broadcast %v with %v", a.shape, b.shape)
		}
		n, c, h, w := big.shape[0], big.shape[1], big.shape[2], big.shape[3]
		per := len(small.data)
		if per != c && per != n*c {
			return nil, fmt.Errorf("cannot broadcast %v with %v", a.shape, b.shape)
		}
		plane := h * w
		for i := 0; i < n*c; i++ {
			scalarOp(op, swapped, out.data[i*plane:(i+1)*plane], big.data[i*plane:(i+1)*plane], small.data[i%per])
		}
	}
	return out, nil
}

// vecOp computes dst = x op y, or y op x when swapped.
func vecOp(op string, swapped bool, dst, x, y []float32) {
	y = y[:len(x)]
	dst = dst[:len(x)]
	switch {
	case op == "Add":
		for i, v := range x {
			dst[i] = v + y[i]
		}
	case op == "Mul":
		for i, v := range x {
			dst[i] = v * y[i]
		}
	case op == "Sub" && !swapped, op == "Div" && !swapped:
		if op == "Sub" {
			for i, v := range x {
				dst[i] = v - y[i]
			}
		} else {
			for i, v := range x {
				dst[i] = v / y[i]
			}
		}
	case op == "Sub":
		for i, v := range x {
			dst[i] = y[i] - v
		}
	case op == "Div":
		for i, v := range x {
			dst[i] = y[i] / v
		}
	}
}

// scalarOp computes dst = x op y, or y op x when swapped, for one value y.
func scalarOp(op string, swapped bool, dst, x []float32, y float32) {
	dst = dst[:len(x)]
	switch {
	case op == "Add":
		for i, v := range x {
			dst[i] = v + y
		}
	case op == "Mul":
		for i, v := range x {
			dst[i] = v * y
		}
	case op == "Sub" && !swapped:
		for i, v := range x {
			dst[i] = v - y
		}
	case op == "Div" && !swapped:
		r := 1 / y
		for i, v := range x {
			dst[i] = v * r
		}
	case op == "Sub":
		for i, v := range x {
			dst[i] = y - v
		}
	case op == "Div":
		for i, v := range x {
			dst[i] = y / v
		}
	}
}

func batchNorm(in []*tensor, attrs map[string]attr) (*tensor, error) {
	eps := float32(1e-5)
	if a, ok := attrs["epsilon"]; ok {
		eps = a.f
	}
	x := in[0]
	_, c, h, w, err := x.dims4()
	if err != nil {
		return nil, err
	}
	scale, bias, mean, vr := in[1].data, in[2].data, in[3].data, in[4].data
	out := &tensor{shape: x.shape, data: make([]float32, len(x.data))}
	plane := h * w
	for i := 0; i < len(x.data)/plane; i++ {
		ch := i % c
		k := scale[ch] / float32(math.Sqrt(float64(vr[ch]+eps)))
		b := bias[ch] - mean[ch]*k
		src := x.data[i*plane : (i+1)*plane]
		dst := out.data[i*plane : (i+1)*plane]
		for j, v := range src {
			dst[j] = v*k + b
		}
	}
	return out, nil
}

func reshape(x, shape *tensor) (*tensor, error) {
	if shape == nil || shape.ints == nil {
		return nil, errors.New("reshape needs a constant shape")
	}
	out := make([]int, len(shape.ints))
	known, unknown := 1, -1
	for i, d := range shape.ints {
		switch {
		case d == 0 && i < len(x.shape):
			out[i] = x.shape[i]
		case d == -1:
			unknown = i
			continue
		default:
			out[i] = int(d)
		}
		known *= out[i]
	}
	total := x.size()
	if unknown >= 0 {
		if known == 0 {
			return nil, errors.New("reshape: bad shape")
		}
		out[unknown] = total / known
	}
	r := &tensor{shape: out, data: x.data, ints: x.ints}
	if r.size() != total {
		return nil, fmt.Errorf("reshape %v to %v", x.shape, out)
	}
	return r, nil
}

func globalAveragePool(x *tensor) (*tensor, error) {
	n, c, h, w, err := x.dims4()
	if err != nil {
		return nil, err
	}
	out := newTensor(n, c, 1, 1)
	plane := h * w
	for i := 0; i < n*c; i++ {
		var sum float64
		for _, v := range x.data[i*plane : (i+1)*plane] {
			sum += float64(v)
		}
		out.data[i] = float32(sum / float64(plane))
	}
	return out, nil
}

// resizeNearest scales an image by whole factors, picking the nearest
// pixel ("asymmetric" coordinates rounded down), as the models here do.
func resizeNearest(in []*tensor, attrs map[string]attr) (*tensor, error) {
	if m, ok := attrs["mode"]; ok && m.s != "nearest" {
		return nil, fmt.Errorf("resize mode %s", m.s)
	}
	x := in[0]
	n, c, h, w, err := x.dims4()
	if err != nil {
		return nil, err
	}
	var oh, ow int
	switch {
	case len(in) > 2 && in[2] != nil && len(in[2].data) == 4:
		oh = int(math.Floor(float64(h) * float64(in[2].data[2])))
		ow = int(math.Floor(float64(w) * float64(in[2].data[3])))
	case len(in) > 3 && in[3] != nil && len(in[3].ints) == 4:
		oh, ow = int(in[3].ints[2]), int(in[3].ints[3])
	default:
		return nil, errors.New("resize needs constant scales or sizes")
	}
	out := newTensor(n, c, oh, ow)
	sy, sx := float64(h)/float64(oh), float64(w)/float64(ow)
	xs := make([]int, ow)
	for ox := range xs {
		xs[ox] = min(int(float64(ox)*sx), w-1)
	}
	for i := 0; i < n*c; i++ {
		src := x.data[i*h*w : (i+1)*h*w]
		dst := out.data[i*oh*ow : (i+1)*oh*ow]
		for oy := 0; oy < oh; oy++ {
			row := src[min(int(float64(oy)*sy), h-1)*w:]
			drow := dst[oy*ow : (oy+1)*ow]
			for ox, sxi := range xs {
				drow[ox] = row[sxi]
			}
		}
	}
	return out, nil
}

func concat(in []*tensor, attrs map[string]attr) (*tensor, error) {
	if a, ok := attrs["axis"]; ok && a.i != 1 {
		return nil, fmt.Errorf("concat on axis %d", a.i)
	}
	n, _, h, w, err := in[0].dims4()
	if err != nil {
		return nil, err
	}
	c := 0
	for _, t := range in {
		tn, tc, th, tw, err := t.dims4()
		if err != nil {
			return nil, err
		}
		if tn != n || th != h || tw != w {
			return nil, fmt.Errorf("concat %v with %v", in[0].shape, t.shape)
		}
		c += tc
	}
	out := newTensor(n, c, h, w)
	plane := h * w
	off := 0
	for b := 0; b < n; b++ {
		for _, t := range in {
			tc := t.shape[1]
			copy(out.data[off:off+tc*plane], t.data[b*tc*plane:(b+1)*tc*plane])
			off += tc * plane
		}
	}
	return out, nil
}

// fuseHardSwish replaces x·clip(x+3, 0, 6)/6, written out in four nodes,
// by one HardSwish node.
func fuseHardSwish(nodes []node, consts map[string]*tensor, uses map[string]int) []node {
	isConst := func(name string, v float32) bool {
		t := consts[name]
		return t != nil && len(t.data) == 1 && t.data[0] == v
	}
	byInput := map[string]int{}
	for i, n := range nodes {
		for _, in := range n.inputs {
			if _, ok := byInput[in]; !ok {
				byInput[in] = i
			}
		}
	}
	next := func(out string) (int, bool) {
		if uses[out] != 1 {
			return 0, false
		}
		j, ok := byInput[out]
		return j, ok
	}
	drop := map[int]bool{}
	for i, n := range nodes {
		if n.op != "Add" || len(n.inputs) != 2 || !isConst(n.inputs[1], 3) {
			continue
		}
		x := n.inputs[0]
		ci, ok := next(n.outputs[0])
		if !ok || nodes[ci].op != "Clip" || len(nodes[ci].inputs) != 3 ||
			!isConst(nodes[ci].inputs[1], 0) || !isConst(nodes[ci].inputs[2], 6) {
			continue
		}
		mi, ok := next(nodes[ci].outputs[0])
		if !ok || nodes[mi].op != "Mul" || !slices.Contains(nodes[mi].inputs, x) {
			continue
		}
		di, ok := next(nodes[mi].outputs[0])
		if !ok || nodes[di].op != "Div" || len(nodes[di].inputs) != 2 || !isConst(nodes[di].inputs[1], 6) {
			continue
		}
		nodes[i] = node{op: "HardSwish", inputs: []string{x}, outputs: nodes[di].outputs, attrs: map[string]attr{}}
		drop[ci], drop[mi], drop[di] = true, true, true
	}
	out := nodes[:0:0]
	for i, n := range nodes {
		if !drop[i] {
			out = append(out, n)
		}
	}
	return out
}

func hardSwish(x *tensor) *tensor {
	out := &tensor{shape: x.shape, data: make([]float32, len(x.data))}
	for i, v := range x.data {
		out.data[i] = v * min(max(v+3, 0), 6) * (1.0 / 6)
	}
	return out
}
