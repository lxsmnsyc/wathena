package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

// testNode2 describes a node for buildRSM2.
type testNode2 struct {
	name, parent string
	mat3         [9]float32
	pos          [3]float32
	verts        [][3]float32
	faces        [][3]uint16
	rotKeys      []RotKey
	posKeys      []PosKey
	scaleKeys    []ScaleKey
}

// buildRSM2 writes a version 2.3 model at 30 frames per second.
func buildRSM2(nodes []testNode2) []byte {
	var b bytes.Buffer
	w := func(v ...any) {
		for _, x := range v {
			_ = binary.Write(&b, binary.LittleEndian, x)
		}
	}
	str := func(s string) {
		w(int32(len(s)))
		b.WriteString(s)
	}
	b.WriteString("GRSM")
	b.Write([]byte{2, 3})
	w(int32(60)) // animation length in frames
	w(int32(ShadeFlat))
	b.WriteByte(255)
	w(float32(30))
	w(int32(1))
	str(nodes[0].name)
	w(int32(len(nodes)))
	for _, n := range nodes {
		str(n.name)
		str(n.parent)
		w(int32(1))
		str(`test\wall.bmp`)
		w(n.mat3)
		w(n.pos)
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
			w(int32(24))
			w(f)
			w([3]uint16{0, 1, 2})
			w(uint16(0), uint16(0), int32(0), int32(0))
		}
		w(int32(len(n.scaleKeys)))
		for _, k := range n.scaleKeys {
			w(k.Frame, k.Scale, float32(0))
		}
		w(int32(len(n.rotKeys)))
		for _, k := range n.rotKeys {
			w(k.Frame, k.Q)
		}
		w(int32(len(n.posKeys)))
		for _, k := range n.posKeys {
			w(k.Frame, k.Pos, int32(0))
		}
		w(int32(0)) // texture animations
	}
	w(int32(0)) // volume boxes
	return b.Bytes()
}

// floor2 is a triangle in RSM2 space, where Y points up and Z north.
// (b - a) × (c - a) is +Y, so it faces up.
func floor2() testNode2 {
	return testNode2{
		name:  "base",
		mat3:  ident3,
		verts: [][3]float32{{0, 0, 0}, {0, 0, 10}, {10, 0, 0}},
		faces: [][3]uint16{{0, 1, 2}},
	}
}

func TestInverse3(t *testing.T) {
	a := mat4{2, 1, 0, 0, 0, 3, 1, 0, 1, 0, 4, 0, 0, 0, 0, 1}
	p := a.mul(inverse3(a))
	for i := range 16 {
		want := 0.0
		if i%5 == 0 {
			want = 1
		}
		if math.Abs(p[i]-want) > 1e-12 {
			t.Fatalf("a × inverse = %v", p)
		}
	}
}

func TestConvertV2(t *testing.T) {
	base := floor2()
	base.pos = [3]float32{1, 2, 3}
	arm := floor2()
	arm.name, arm.parent = "arm", "base"
	// The arm sits 5 east of the base, in model space, and turns around Y
	// from frame 15.
	arm.pos = [3]float32{6, 2, 3}
	s := float32(math.Sqrt(0.5))
	arm.rotKeys = []RotKey{{Frame: 15, Q: [4]float32{0, s, 0, s}}, {Frame: 30, Q: [4]float32{0, 1, 0, 0}}}
	arm.posKeys = []PosKey{{Frame: 0, Pos: [3]float32{5, 0, 1}}, {Frame: 30, Pos: [3]float32{5, 0, 2}}}
	m, err := Parse(buildRSM2([]testNode2{base, arm}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Convert(m, func(p string) string { return "texture/x" })
	if err != nil {
		t.Fatal(err)
	}
	g, err := readGLB(out)
	if err != nil {
		t.Fatal(err)
	}
	// Nodes: root, the base's frame, the base's mesh, and the arm. The base
	// keeps a node because the arm moves under it.
	if len(g.doc.Nodes) != 4 {
		t.Fatalf("%d nodes: %+v", len(g.doc.Nodes), g.doc.Nodes)
	}
	// The root is not centered. Z mirrors.
	frame := g.doc.Nodes[1].Matrix
	if g.doc.Nodes[0].Translation != nil || frame == nil || [3]float32(frame[12:15]) != [3]float32{1, 2, -3} {
		t.Errorf("root %+v, base frame %v", g.doc.Nodes[0], frame)
	}
	if g.doc.Nodes[2].Mesh == nil || g.doc.Nodes[2].Matrix != nil {
		t.Errorf("base mesh node %+v", g.doc.Nodes[2])
	}
	arm2 := g.doc.Nodes[3]
	// Frame 0 has the first position keyframe, and no rotation yet.
	if *arm2.Translation != [3]float32{5, 0, -1} || *arm2.Rotation != [4]float32{0, 0, 0, 1} {
		t.Errorf("arm %v %v", *arm2.Translation, *arm2.Rotation)
	}
	a := g.doc.Animations[0]
	if len(a.Channels) != 2 || a.Extras["loopSeconds"] != 2.0 {
		t.Fatalf("animation %+v", a)
	}
	for _, ch := range a.Channels {
		times, _ := g.floats(a.Samplers[ch.Sampler].Input)
		values, _ := g.floats(a.Samplers[ch.Sampler].Output)
		switch ch.Target.Path {
		case "rotation":
			// A key at frame 0 with no rotation starts the animation.
			if len(times) != 3 || times[0] != 0 || times[1] != 0.5 || values[3] != 1 {
				t.Errorf("rotation times %v values %v", times, values)
			}
			// A turn around Y mirrors to a turn the other way.
			if math.Abs(values[5]+float64(s)) > 1e-6 {
				t.Errorf("mirrored rotation %v", values[4:8])
			}
		case "translation":
			if len(times) != 2 || times[1] != 1 || values[5] != -2 {
				t.Errorf("translation times %v values %v", times, values)
			}
		}
	}
	// The face turns with Q and keeps facing up.
	prim := g.doc.Meshes[0].Primitives[0]
	pos, _ := g.floats(prim.Attributes["POSITION"])
	idx, _ := g.indices(prim.Indices)
	p := func(i int) [3]float64 { return [3]float64{pos[3*idx[i]], pos[3*idx[i]+1], pos[3*idx[i]+2]} }
	if n, _ := normalize(cross(sub(p(1), p(0)), sub(p(2), p(0)))); n[1] < 0.99 {
		t.Errorf("winding normal %v, want up", n)
	}
	sum := sha256.Sum256(out)
	if got := hex.EncodeToString(sum[:]); got != stableHashV2 {
		t.Errorf("output hash %s, want %s", got, stableHashV2)
	}
}

const stableHashV2 = "d82bd0df3e52f117f4ae7b0e42a62d88c569d08b393a5893d1dd876a3c3eba57"

func TestFlattenV2(t *testing.T) {
	// A model that does not move bakes every node into its vertices. The
	// base's matrix shears, which a glTF node matrix cannot hold.
	base := floor2()
	base.mat3 = [9]float32{1, 0, 0, 0.5, 1, 0, 0, 0, 1}
	child := floor2()
	child.name, child.parent = "child", "base"
	child.pos = [3]float32{0, 0, 5}
	m, err := Parse(buildRSM2([]testNode2{base, child}))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Convert(m, func(p string) string { return "texture/x" })
	if err != nil {
		t.Fatal(err)
	}
	g, err := readGLB(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range g.doc.Nodes {
		if n.Matrix != nil || n.Translation != nil || n.Rotation != nil {
			t.Errorf("node %q has a transform", n.Name)
		}
	}
	// The child's vertex (10, 0, 0) moves to (10, 0, 5), then mirrors to
	// (10, 0, -5).
	pos, _ := g.floats(g.doc.Meshes[1].Primitives[0].Attributes["POSITION"])
	found := false
	for i := 0; i < len(pos); i += 3 {
		found = found || (pos[i] == 10 && pos[i+1] == 0 && pos[i+2] == -5)
	}
	if !found {
		t.Errorf("child positions %v", pos)
	}
}

func TestStaticChildV2(t *testing.T) {
	// A static child's world transform is its own position and matrix in
	// model space, whatever its parent's matrix is.
	base := floor2()
	base.mat3 = [9]float32{0, 0, -1, 0, 1, 0, 1, 0, 0}
	base.pos = [3]float32{4, 0, 0}
	child := floor2()
	child.name, child.parent = "child", "base"
	child.pos = [3]float32{0, 1, 7}
	m, err := Parse(buildRSM2([]testNode2{base, child}))
	if err != nil {
		t.Fatal(err)
	}
	c := &converter{m: m}
	st := c.v2Frame0(1, 0)
	w := translation(vec(base.pos)).mul(c.mat3(m.Nodes[0])).mul(translation(st.pos)).mul(st.m)
	got := w.point([3]float64{0, 0, 0})
	if math.Abs(got[0]) > 1e-9 || math.Abs(got[1]-1) > 1e-9 || math.Abs(got[2]-7) > 1e-9 {
		t.Errorf("child origin at %v, want (0, 1, 7)", got)
	}
}
