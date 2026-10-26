package textfind

// A small reader for the ONNX file format: just enough of the protobuf
// encoding to load the nodes and constants of an inference graph.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// node is one operation of the graph.
type node struct {
	op      string
	inputs  []string
	outputs []string
	attrs   map[string]attr
}

// attr is a node attribute.
type attr struct {
	i      int64
	f      float32
	s      string
	ints   []int64
	floats []float32
	t      *tensor
}

type graph struct {
	nodes   []node
	consts  map[string]*tensor
	inputs  []string
	outputs []string
}

// protobuf wire types.
const (
	wireVarint = 0
	wire64     = 1
	wireBytes  = 2
	wire32     = 5
)

type pbReader struct {
	b   []byte
	err error
}

// next returns the next field number and wire type, or false at the end.
func (r *pbReader) next() (int, int, bool) {
	if r.err != nil || len(r.b) == 0 {
		return 0, 0, false
	}
	k := r.varint()
	if r.err != nil {
		return 0, 0, false
	}
	return int(k >> 3), int(k & 7), true
}

func (r *pbReader) varint() uint64 {
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.err = errors.New("onnx: bad varint")
		r.b = nil
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *pbReader) bytes() []byte {
	n := r.varint()
	if r.err != nil {
		return nil
	}
	if n > uint64(len(r.b)) {
		r.err = errors.New("onnx: truncated field")
		r.b = nil
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *pbReader) fixed32() uint32 {
	if len(r.b) < 4 {
		r.err = errors.New("onnx: truncated fixed32")
		r.b = nil
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v
}

func (r *pbReader) skip(wire int) {
	switch wire {
	case wireVarint:
		r.varint()
	case wire64:
		if len(r.b) < 8 {
			r.err = errors.New("onnx: truncated fixed64")
			r.b = nil
			return
		}
		r.b = r.b[8:]
	case wireBytes:
		r.bytes()
	case wire32:
		r.fixed32()
	default:
		r.err = fmt.Errorf("onnx: unknown wire type %d", wire)
		r.b = nil
	}
}

// ints reads a repeated int64 field, packed or not.
func (r *pbReader) ints(wire int, dst []int64) []int64 {
	if wire == wireBytes {
		p := pbReader{b: r.bytes()}
		for len(p.b) > 0 && p.err == nil {
			dst = append(dst, int64(p.varint()))
		}
		if p.err != nil {
			r.err = p.err
		}
		return dst
	}
	return append(dst, int64(r.varint()))
}

// floats reads a repeated float field, packed or not.
func (r *pbReader) floats(wire int, dst []float32) []float32 {
	if wire == wireBytes {
		b := r.bytes()
		for i := 0; i+4 <= len(b); i += 4 {
			dst = append(dst, math.Float32frombits(binary.LittleEndian.Uint32(b[i:])))
		}
		return dst
	}
	return append(dst, math.Float32frombits(r.fixed32()))
}

// parseModel reads a ModelProto.
func parseModel(b []byte) (*graph, error) {
	r := pbReader{b: b}
	var g *graph
	for {
		f, w, ok := r.next()
		if !ok {
			break
		}
		if f == 7 && w == wireBytes { // graph
			var err error
			if g, err = parseGraph(r.bytes()); err != nil {
				return nil, err
			}
			continue
		}
		r.skip(w)
	}
	if r.err != nil {
		return nil, r.err
	}
	if g == nil {
		return nil, errors.New("onnx: no graph")
	}
	return g, nil
}

func parseGraph(b []byte) (*graph, error) {
	g := &graph{consts: map[string]*tensor{}}
	r := pbReader{b: b}
	for {
		f, w, ok := r.next()
		if !ok {
			break
		}
		switch {
		case f == 1 && w == wireBytes: // node
			n, err := parseNode(r.bytes())
			if err != nil {
				return nil, err
			}
			g.nodes = append(g.nodes, n)
		case f == 5 && w == wireBytes: // initializer
			t, name, err := parseTensor(r.bytes())
			if err != nil {
				return nil, err
			}
			g.consts[name] = t
		case (f == 11 || f == 12) && w == wireBytes: // input, output
			name := valueInfoName(r.bytes())
			if f == 11 {
				g.inputs = append(g.inputs, name)
			} else {
				g.outputs = append(g.outputs, name)
			}
		default:
			r.skip(w)
		}
	}
	return g, r.err
}

func valueInfoName(b []byte) string {
	r := pbReader{b: b}
	for {
		f, w, ok := r.next()
		if !ok {
			return ""
		}
		if f == 1 && w == wireBytes {
			return string(r.bytes())
		}
		r.skip(w)
	}
}

func parseNode(b []byte) (node, error) {
	n := node{attrs: map[string]attr{}}
	r := pbReader{b: b}
	for {
		f, w, ok := r.next()
		if !ok {
			break
		}
		switch {
		case f == 1 && w == wireBytes:
			n.inputs = append(n.inputs, string(r.bytes()))
		case f == 2 && w == wireBytes:
			n.outputs = append(n.outputs, string(r.bytes()))
		case f == 4 && w == wireBytes:
			n.op = string(r.bytes())
		case f == 5 && w == wireBytes:
			name, a, err := parseAttr(r.bytes())
			if err != nil {
				return n, err
			}
			n.attrs[name] = a
		default:
			r.skip(w)
		}
	}
	return n, r.err
}

func parseAttr(b []byte) (string, attr, error) {
	var a attr
	var name string
	r := pbReader{b: b}
	for {
		f, w, ok := r.next()
		if !ok {
			break
		}
		switch {
		case f == 1 && w == wireBytes:
			name = string(r.bytes())
		case f == 2 && w == wire32:
			a.f = math.Float32frombits(r.fixed32())
		case f == 3 && w == wireVarint:
			a.i = int64(r.varint())
		case f == 4 && w == wireBytes:
			a.s = string(r.bytes())
		case f == 5 && w == wireBytes:
			t, _, err := parseTensor(r.bytes())
			if err != nil {
				return name, a, err
			}
			a.t = t
		case f == 7:
			a.floats = r.floats(w, a.floats)
		case f == 8:
			a.ints = r.ints(w, a.ints)
		default:
			r.skip(w)
		}
	}
	return name, a, r.err
}

// ONNX tensor data types.
const (
	typeFloat = 1
	typeInt32 = 6
	typeInt64 = 7
)

// parseTensor reads a TensorProto. Float tensors keep their values in
// data; integer tensors, which hold shapes and sizes here, in ints.
func parseTensor(b []byte) (*tensor, string, error) {
	var dims []int64
	var typ int
	var raw []byte
	var floats []float32
	var ints []int64
	var name string
	r := pbReader{b: b}
	for {
		f, w, ok := r.next()
		if !ok {
			break
		}
		switch {
		case f == 1:
			dims = r.ints(w, dims)
		case f == 2 && w == wireVarint:
			typ = int(r.varint())
		case f == 4:
			floats = r.floats(w, floats)
		case (f == 5 || f == 7) && (w == wireBytes || w == wireVarint):
			ints = r.ints(w, ints)
		case f == 8 && w == wireBytes:
			name = string(r.bytes())
		case f == 9 && w == wireBytes:
			raw = r.bytes()
		default:
			r.skip(w)
		}
	}
	if r.err != nil {
		return nil, "", r.err
	}
	shape := make([]int, len(dims))
	size := 1
	for i, d := range dims {
		shape[i] = int(d)
		size *= int(d)
	}
	t := &tensor{shape: shape}
	switch typ {
	case typeFloat:
		if raw != nil {
			if len(raw) != 4*size {
				return nil, "", fmt.Errorf("onnx: tensor %s has %d bytes for %d values", name, len(raw), size)
			}
			floats = make([]float32, size)
			for i := range floats {
				floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
			}
		}
		t.data = floats
	case typeInt64:
		if raw != nil {
			ints = make([]int64, len(raw)/8)
			for i := range ints {
				ints[i] = int64(binary.LittleEndian.Uint64(raw[8*i:]))
			}
		}
		t.ints = ints
	case typeInt32:
		if raw != nil {
			ints = make([]int64, len(raw)/4)
			for i := range ints {
				ints[i] = int64(int32(binary.LittleEndian.Uint32(raw[4*i:])))
			}
		}
		t.ints = ints
	default:
		return nil, "", fmt.Errorf("onnx: tensor %s has unsupported type %d", name, typ)
	}
	return t, name, nil
}
