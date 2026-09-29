// Package model converts RO's RSM models into assets-v1 models: glTF 2.0
// binary files, as defined in docs/specs/assets-v1.md, "Model".
package model

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"golang.org/x/text/encoding/korean"
)

var ErrFormat = errors.New("model: invalid rsm file")

// Parsing limits. Real models are far below them.
const (
	maxNodes     = 4096
	maxTextures  = 4096
	maxVertices  = 1 << 20
	maxFaces     = 1 << 20
	maxKeyframes = 1 << 16
	maxString    = 1024
)

// Shading types.
const (
	ShadeNone   = 0
	ShadeFlat   = 1
	ShadeSmooth = 2
)

// RSM is a parsed model.
type RSM struct {
	// Version is major × 10 + minor, such as 14 for 1.4.
	Version   int
	AnimLen   int32
	ShadeType int32
	// Alpha is the model's opacity, 0 to 255.
	Alpha uint8
	// FrameRate is the keyframe rate of version 2 models.
	FrameRate float32
	// Textures are paths relative to data\texture, as UTF-8.
	Textures []string
	MainNode string
	Nodes    []*Node
	// tail is the unused data after the nodes.
	tail []byte
}

// Node is one part of a model.
type Node struct {
	Name, Parent string
	// TextureIDs index RSM.Textures.
	TextureIDs []int
	Mat3       [9]float32
	Offset     [3]float32
	// Pos, RotAngle, RotAxis, and Scale exist in version 1 only.
	Pos      [3]float32
	RotAngle float32
	RotAxis  [3]float32
	Scale    [3]float32
	Vertices [][3]float32
	UVs      [][2]float32
	Faces    []Face
	RotKeys  []RotKey
	PosKeys  []PosKey
	ScaleKey []ScaleKey
}

// Face is a triangle.
type Face struct {
	Vert    [3]uint16
	UV      [3]uint16
	Texture uint16
	TwoSide bool
	Smooth  int32
}

// RotKey is a rotation keyframe, a quaternion stored as x, y, z, w.
type RotKey struct {
	Frame int32
	Q     [4]float32
}

// PosKey is a position keyframe.
type PosKey struct {
	Frame int32
	Pos   [3]float32
}

// ScaleKey is a scale keyframe.
type ScaleKey struct {
	Frame int32
	Scale [3]float32
}

type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b)-r.p {
		if r.err == nil {
			r.err = fmt.Errorf("%w: file is truncated", ErrFormat)
		}
		return nil
	}
	out := r.b[r.p : r.p+n]
	r.p += n
	return out
}

func (r *reader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) u16() uint16 {
	if b := r.take(2); b != nil {
		return binary.LittleEndian.Uint16(b)
	}
	return 0
}

func (r *reader) i32() int32 {
	if b := r.take(4); b != nil {
		return int32(binary.LittleEndian.Uint32(b)) // #nosec G115 -- reinterprets the bits.
	}
	return 0
}

func (r *reader) f32() float32 {
	if b := r.take(4); b != nil {
		return math.Float32frombits(binary.LittleEndian.Uint32(b))
	}
	return 0
}

// count reads a count and checks it against a limit and the bytes left,
// given the smallest size of one item.
func (r *reader) count(limit, itemSize int) int {
	n := r.i32()
	if r.err == nil && (n < 0 || int(n) > limit || int(n)*itemSize > len(r.b)-r.p) {
		r.err = fmt.Errorf("%w: count %d", ErrFormat, n)
	}
	if r.err != nil {
		return 0
	}
	return int(n)
}

// fixedString reads a NUL-padded CP949 string of n bytes.
func (r *reader) fixedString(n int) string {
	return decodeName(r.take(n))
}

// lenString reads a CP949 string with a 32-bit length.
func (r *reader) lenString() string {
	return decodeName(r.take(r.count(maxString, 1)))
}

func decodeName(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	s, err := korean.EUCKR.NewDecoder().String(string(b))
	if err != nil {
		return strings.ToValidUTF8(string(b), "")
	}
	return s
}

func (r *reader) vec3() [3]float32 { return [3]float32{r.f32(), r.f32(), r.f32()} }

// Parse reads an RSM file. Versions 1.1 to 1.6, 2.2, and 2.3 are supported.
func Parse(b []byte) (*RSM, error) {
	r := &reader{b: b}
	magic := string(r.take(4))
	if magic != "GRSM" && magic != "GRSX" {
		return nil, fmt.Errorf("%w: not an rsm file", ErrFormat)
	}
	m := &RSM{Alpha: 255}
	m.Version = int(r.u8())*10 + int(r.u8())
	switch {
	case m.Version >= 11 && m.Version <= 16, m.Version == 22, m.Version == 23:
	default:
		return nil, fmt.Errorf("%w: version %d.%d", ErrFormat, m.Version/10, m.Version%10)
	}
	m.AnimLen = r.i32()
	m.ShadeType = r.i32()
	if m.Version >= 14 {
		m.Alpha = r.u8()
	}
	// Version 1 lists textures, then names the main node. Version 2.2
	// lists textures, then root node names. Version 2.3 lists only root
	// node names, and each node names its own textures.
	if m.Version >= 22 {
		m.FrameRate = r.f32()
	} else {
		r.take(16) // reserved
	}
	if m.Version <= 22 {
		size := 40
		if m.Version == 22 {
			size = 4
		}
		for range r.count(maxTextures, size) {
			if m.Version == 22 {
				m.Textures = append(m.Textures, r.lenString())
			} else {
				m.Textures = append(m.Textures, r.fixedString(40))
			}
		}
	}
	if m.Version >= 22 {
		for range r.count(maxNodes, 4) {
			name := r.lenString()
			if m.MainNode == "" {
				m.MainNode = name
			}
		}
	} else {
		m.MainNode = r.fixedString(40)
	}
	for range r.count(maxNodes, 1) {
		n := r.node(m)
		if r.err != nil {
			return nil, r.err
		}
		m.Nodes = append(m.Nodes, n)
	}
	if r.err != nil {
		return nil, r.err
	}
	if len(m.Nodes) == 0 {
		return nil, fmt.Errorf("%w: no nodes", ErrFormat)
	}
	// What follows, model position keyframes in version 1 and volume
	// boxes, is not used.
	m.tail = r.b[r.p:]
	return m, nil
}

func (r *reader) node(m *RSM) *Node {
	n := &Node{Scale: [3]float32{1, 1, 1}}
	v2 := m.Version >= 22
	if v2 {
		n.Name, n.Parent = r.lenString(), r.lenString()
	} else {
		n.Name, n.Parent = r.fixedString(40), r.fixedString(40)
	}
	for range r.count(maxTextures, 4) {
		if m.Version >= 23 {
			// Version 2.3 nodes name their textures.
			name := r.lenString()
			id := -1
			for i, t := range m.Textures {
				if t == name {
					id = i
				}
			}
			if id < 0 {
				id = len(m.Textures)
				m.Textures = append(m.Textures, name)
			}
			n.TextureIDs = append(n.TextureIDs, id)
			continue
		}
		n.TextureIDs = append(n.TextureIDs, int(r.i32()))
	}
	for i := range n.Mat3 {
		n.Mat3[i] = r.f32()
	}
	n.Offset = r.vec3()
	if !v2 {
		n.Pos = r.vec3()
		n.RotAngle = r.f32()
		n.RotAxis = r.vec3()
		n.Scale = r.vec3()
	}
	for range r.count(maxVertices, 12) {
		n.Vertices = append(n.Vertices, r.vec3())
	}
	uvSize := 8
	if m.Version >= 12 {
		uvSize = 12
	}
	for range r.count(maxVertices, uvSize) {
		if m.Version >= 12 {
			r.take(4) // vertex color, unused
		}
		n.UVs = append(n.UVs, [2]float32{r.f32(), r.f32()})
	}
	for range r.count(maxFaces, 20) {
		size := 24
		if v2 {
			size = int(r.i32())
		}
		start := r.p
		var f Face
		for i := range 3 {
			f.Vert[i] = r.u16()
		}
		for i := range 3 {
			f.UV[i] = r.u16()
		}
		f.Texture = r.u16()
		r.u16() // padding
		f.TwoSide = r.i32() != 0
		if m.Version >= 12 {
			f.Smooth = r.i32()
		}
		// Version 2 faces can have more smoothing groups, which are skipped.
		if v2 && size > r.p-start {
			r.take(size - (r.p - start))
		}
		n.Faces = append(n.Faces, f)
	}
	if m.Version >= 16 {
		for range r.count(maxKeyframes, 20) {
			k := ScaleKey{Frame: r.i32(), Scale: r.vec3()}
			r.f32() // unused
			n.ScaleKey = append(n.ScaleKey, k)
		}
	}
	for range r.count(maxKeyframes, 20) {
		n.RotKeys = append(n.RotKeys, RotKey{Frame: r.i32(), Q: [4]float32{r.f32(), r.f32(), r.f32(), r.f32()}})
	}
	if v2 {
		for range r.count(maxKeyframes, 20) {
			k := PosKey{Frame: r.i32(), Pos: r.vec3()}
			r.i32() // unused
			n.PosKeys = append(n.PosKeys, k)
		}
	}
	if m.Version >= 23 {
		// Texture animations are skipped: groups of texture, then
		// animations of type and keyframes of frame and value.
		for range r.count(maxKeyframes, 8) {
			r.i32()
			for range r.count(maxKeyframes, 8) {
				r.i32()
				r.take(8 * r.count(maxKeyframes, 8))
			}
		}
	}
	return n
}
