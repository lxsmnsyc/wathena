package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"math"
	"testing"
)

// testNode describes a node for buildRSM.
type testNode struct {
	name, parent string
	mat3         [9]float32
	offset, pos  [3]float32
	angle        float32
	axis         [3]float32
	scale        [3]float32
	verts        [][3]float32
	faces        [][3]uint16
	twoSide      bool
	rotKeys      []RotKey
}

// buildRSM writes a version 1.4 model with one texture.
func buildRSM(shade int32, alpha uint8, nodes []testNode) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	name := func(s string) {
		f := make([]byte, 40)
		copy(f, s)
		b.Write(f)
	}
	b.WriteString("GRSM")
	b.Write([]byte{1, 4})
	w(int32(1000)) // animation length
	w(shade)
	b.WriteByte(alpha)
	b.Write(make([]byte, 16))
	w(int32(1))
	name(`test\wall.bmp`)
	name(nodes[0].name)
	w(int32(len(nodes)))
	for _, n := range nodes {
		name(n.name)
		name(n.parent)
		w(int32(1))
		w(int32(0)) // texture 0
		w(n.mat3)
		w(n.offset)
		w(n.pos)
		w(n.angle)
		w(n.axis)
		w(n.scale)
		w(int32(len(n.verts)))
		for _, v := range n.verts {
			w(v)
		}
		w(int32(3))
		for _, uv := range [][2]float32{{0, 0}, {1, 0}, {0, 1}} {
			w(uint32(0xffffffff))
			w(uv)
		}
		w(int32(len(n.faces)))
		for _, f := range n.faces {
			w(f)
			w([3]uint16{0, 1, 2})
			w(uint16(0))
			w(uint16(0))
			two := int32(0)
			if n.twoSide {
				two = 1
			}
			w(two)
			w(int32(0)) // smoothing group
		}
		w(int32(len(n.rotKeys)))
		for _, k := range n.rotKeys {
			w(k.Frame)
			w(k.Q)
		}
	}
	w(int32(0)) // model position keyframes
	w(int32(0)) // volume boxes
	return b.Bytes()
}

var ident3 = [9]float32{1, 0, 0, 0, 1, 0, 0, 0, 1}

// floor is a triangle in RO space, where Y points down. RO's face normal
// is (b - a) × (c - a), which here is -Y, so it faces up.
func floor() testNode {
	return testNode{
		name:  "floor",
		mat3:  ident3,
		scale: [3]float32{1, 1, 1},
		verts: [][3]float32{{0, 0, 0}, {10, 0, 0}, {0, 0, 10}},
		faces: [][3]uint16{{0, 1, 2}},
	}
}

func convert(t *testing.T, data []byte) (*RSM, *glbFile) {
	t.Helper()
	m, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Convert(m, func(p string) string { return "texture/" + p })
	if err != nil {
		t.Fatal(err)
	}
	g, err := readGLB(out)
	if err != nil {
		t.Fatal(err)
	}
	return m, g
}

func TestParse(t *testing.T) {
	n := floor()
	n.rotKeys = []RotKey{{Frame: 0, Q: [4]float32{0, 0, 0, 1}}}
	m, err := Parse(buildRSM(ShadeFlat, 128, []testNode{n}))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != 14 || m.Alpha != 128 || m.ShadeType != ShadeFlat || m.MainNode != "floor" {
		t.Errorf("header %+v", m)
	}
	if len(m.Textures) != 1 || m.Textures[0] != `test\wall.bmp` {
		t.Errorf("textures %q", m.Textures)
	}
	nd := m.Nodes[0]
	if len(nd.Vertices) != 3 || len(nd.Faces) != 1 || len(nd.UVs) != 3 || len(nd.RotKeys) != 1 || nd.UVs[1] != [2]float32{1, 0} {
		t.Errorf("node %+v", nd)
	}
	if len(m.tail) != 8 {
		t.Errorf("%d bytes after the nodes, want 8", len(m.tail))
	}
	for _, bad := range [][]byte{nil, []byte("GRSM\x03\x00"), buildRSM(0, 255, []testNode{n})[:100]} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
}

func TestConvert(t *testing.T) {
	_, g := convert(t, buildRSM(ShadeFlat, 255, []testNode{floor()}))
	// The root centers the model: x and z on the box center, y on the
	// highest RO point, which is the lowest in glTF.
	root := g.doc.Nodes[0]
	if *root.Translation != [3]float32{-5, 0, -5} {
		t.Errorf("root translation %v", *root.Translation)
	}
	prim := g.doc.Meshes[0].Primitives[0]
	pos, _ := g.floats(prim.Attributes["POSITION"])
	nrm, _ := g.floats(prim.Attributes["NORMAL"])
	idx, _ := g.indices(prim.Indices)
	// The triangle faces up in glTF: its stored normal and its winding.
	if nrm[1] < 0.99 {
		t.Errorf("normal %v, want up", nrm[:3])
	}
	p := func(i int) [3]float64 { return [3]float64{pos[3*idx[i]], pos[3*idx[i]+1], pos[3*idx[i]+2]} }
	if n, _ := normalize(cross(sub(p(1), p(0)), sub(p(2), p(0)))); n[1] < 0.99 {
		t.Errorf("winding gives normal %v, want up", n)
	}
	mat := g.doc.Materials[prim.Material]
	if mat.DoubleSided || mat.AlphaMode != "MASK" || g.doc.Images[0].URI != "asset:texture/test%5Cwall.bmp" {
		t.Errorf("material %+v, image %q", mat, g.doc.Images[0].URI)
	}
}

func TestMirroredNode(t *testing.T) {
	// A mat3 that mirrors X flips the winding. The triangle must still
	// face up.
	n := floor()
	n.mat3 = [9]float32{-1, 0, 0, 0, 1, 0, 0, 0, 1}
	n.twoSide = true
	_, g := convert(t, buildRSM(ShadeSmooth, 255, []testNode{n}))
	prim := g.doc.Meshes[0].Primitives[0]
	pos, _ := g.floats(prim.Attributes["POSITION"])
	nrm, _ := g.floats(prim.Attributes["NORMAL"])
	idx, _ := g.indices(prim.Indices)
	p := func(i int) [3]float64 { return [3]float64{pos[3*idx[i]], pos[3*idx[i]+1], pos[3*idx[i]+2]} }
	if w, _ := normalize(cross(sub(p(1), p(0)), sub(p(2), p(0)))); w[1] < 0.99 || nrm[1] < 0.99 {
		t.Errorf("winding normal %v, stored normal %v, want up", w, nrm[:3])
	}
	if !g.doc.Materials[prim.Material].DoubleSided {
		t.Error("two-sided face is not double-sided")
	}
}

func TestHierarchyAndAnimation(t *testing.T) {
	parent := floor()
	parent.name = "base"
	parent.pos = [3]float32{0, -5, 0}
	child := floor()
	child.name, child.parent = "arm", "base"
	child.pos = [3]float32{1, 2, 3}
	// A quarter turn around Y, then a half turn.
	s := float32(math.Sqrt(0.5))
	child.rotKeys = []RotKey{{Frame: 0, Q: [4]float32{0, s, 0, s}}, {Frame: 500, Q: [4]float32{0, 1, 0, 0}}}
	_, g := convert(t, buildRSM(ShadeNone, 100, []testNode{parent, child}))
	if len(g.doc.Nodes) != 3 || len(g.doc.Nodes[1].Children) != 1 {
		t.Fatalf("nodes %+v", g.doc.Nodes)
	}
	arm := g.doc.Nodes[2]
	// Translations mirror through Y. Rotations around Y keep their
	// quaternion, since x and z are zero.
	if *arm.Translation != [3]float32{1, -2, 3} || math.Abs(float64(arm.Rotation[1]-s)) > 1e-6 {
		t.Errorf("arm %+v %v", *arm.Translation, *arm.Rotation)
	}
	if len(g.doc.Animations) != 1 {
		t.Fatalf("%d animations", len(g.doc.Animations))
	}
	a := g.doc.Animations[0]
	times, _ := g.floats(a.Samplers[0].Input)
	if len(times) != 2 || times[1] != 0.5 || a.Channels[0].Target.Node != 2 || a.Extras["loopSeconds"] != 1.0 {
		t.Errorf("animation %+v, times %v", a, times)
	}
	if mat := g.doc.Materials[0]; mat.AlphaMode != "BLEND" || mat.PBR.BaseColorFactor[3] != float32(100)/255 {
		t.Errorf("translucent material %+v", mat)
	}
}

func TestStableOutput(t *testing.T) {
	// The build stores models by content hash, so the same input must give
	// the same bytes on every platform. This model uses only keyframe
	// rotations, whose math is exact enough to pin.
	parent := floor()
	parent.mat3 = [9]float32{0.5, 0.25, 0, 0.1, 1.5, 0.2, 0, 0.3, 2}
	parent.offset = [3]float32{0.3, 0.7, -1.1}
	child := floor()
	child.name, child.parent = "arm", "floor"
	child.pos = [3]float32{1.5, -2.25, 3.125}
	child.rotKeys = []RotKey{{0, [4]float32{0.1, 0.7, 0.3, 0.64}}, {250, [4]float32{0.5, 0.5, 0.5, 0.5}}}
	m, err := Parse(buildRSM(ShadeSmooth, 255, []testNode{parent, child}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Convert(m, func(p string) string { return "texture/x" })
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(out)
	if got := hex.EncodeToString(sum[:]); got != stableHash {
		t.Errorf("output hash %s, want %s", got, stableHash)
	}
}

const stableHash = "ca95e04166e5bc022416f15ea7aa1353ec6229449b785b0708e27fccc5634f6b"

func TestRender(t *testing.T) {
	m, err := Parse(buildRSM(ShadeFlat, 255, []testNode{floor()}))
	if err != nil {
		t.Fatal(err)
	}
	glb, err := Convert(m, func(p string) string { return "texture/wall" })
	if err != nil {
		t.Fatal(err)
	}
	red := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := range 4 {
		red.SetNRGBA(i%2, i/2, color.NRGBA{255, 0, 0, 255})
	}
	img, err := Render(glb, 32, func(name string) *image.NRGBA {
		if name == "texture/wall" {
			return red
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The floor faces the camera, so back-face culling keeps it.
	lit := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 100 && img.Pix[i+1] == 0 {
			lit++
		}
	}
	if lit < 50 {
		t.Errorf("%d red pixels", lit)
	}
}

func FuzzConvert(f *testing.F) {
	f.Add(buildRSM(ShadeSmooth, 255, []testNode{floor()}))
	child := floor()
	child.name, child.parent = "b", "floor"
	child.rotKeys = []RotKey{{0, [4]float32{0, 0, 0, 1}}, {10, [4]float32{1, 0, 0, 0}}}
	f.Add(buildRSM(ShadeNone, 1, []testNode{floor(), child}))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Parse(data)
		if err != nil {
			return
		}
		out, err := Convert(m, func(p string) string { return "texture/x" })
		if err != nil {
			return
		}
		if _, err := Render(out, 8, nil); err != nil {
			t.Fatal(err)
		}
	})
}
