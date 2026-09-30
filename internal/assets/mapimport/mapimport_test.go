package mapimport

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
)

type buf struct{ bytes.Buffer }

func (b *buf) w(v ...any) {
	for _, x := range v {
		_ = binary.Write(b, binary.LittleEndian, x)
	}
}

func (b *buf) name(s string, n int) {
	f := make([]byte, n)
	copy(f, s)
	b.Write(f)
}

// buildGAT writes a 2 × 1 GAT: a walkable cell, and a cliff with one
// undefined corner.
func buildGAT() []byte {
	var b buf
	b.WriteString("GRAT")
	b.Write([]byte{1, 2})
	b.w(uint32(2), uint32(1))
	b.w([4]float32{0, -5, -10, -15}, uint32(0))
	b.w([4]float32{float32(math.NaN()), 5, 10, 15}, uint32(5))
	return b.Bytes()
}

// buildGND writes a 1 × 1 GND 1.8 with two textures of the same name, one
// tile, and one lightmap.
func buildGND() []byte {
	var b buf
	b.WriteString("GRGN")
	b.Write([]byte{1, 8})
	b.w(uint32(1), uint32(1), float32(10))
	b.w(uint32(2), uint32(80))
	b.name(`grass.bmp`, 80)
	b.name(`grass.bmp`, 80)
	b.w(int32(1), int32(8), int32(8), int32(1))
	light := make([]byte, 256)
	for i := range 64 {
		light[i] = byte(i)
		light[64+3*i], light[64+3*i+1], light[64+3*i+2] = 10, 20, 30
	}
	b.Write(light)
	b.w(uint32(1))
	b.w([4]float32{0, 1, 0, 1}, [4]float32{0, 0, 1, 1}, uint16(1), uint16(0), [4]uint8{30, 20, 10, 255})
	b.w([4]float32{-5, -5, -10, -10}, int32(0), int32(-1), int32(7))
	b.w(float32(-10), int32(3), float32(5), float32(2), float32(50), int32(3), int32(1), int32(1))
	return b.Bytes()
}

// buildRSW writes an RSW 2.6 with one model, one sound, and one effect.
func buildRSW() []byte {
	var b buf
	b.WriteString("GRSW")
	b.Write([]byte{2, 6})
	b.w(uint32(200)) // build number, which adds a byte to models
	b.WriteByte(0)
	b.name("", 40)
	b.name("t.gnd", 40)
	b.name("t.gat", 40)
	b.name("", 40)
	b.w(int32(45), int32(30), [3]float32{1, 1, 1}, [3]float32{0.3, 0.3, 0.3}, float32(0.5))
	b.w([4]int32{})
	b.w(int32(4))
	b.w(int32(1))
	b.name("tree", 40)
	b.w(int32(0), float32(1), int32(0))
	b.WriteByte(0)
	b.name(`forest\tree.rsm`, 80)
	b.name("", 80)
	b.w([3]float32{10, -5, 20}, [3]float32{0, 90, 0}, [3]float32{5, 5, 5})
	b.w(int32(1))
	b.name("broken", 40)
	b.w(int32(0), float32(1), int32(0))
	b.WriteByte(0)
	b.name(`x.rsm`, 80)
	b.name("", 80)
	nan := float32(math.NaN())
	b.w([3]float32{nan, nan, nan}, [3]float32{}, [3]float32{1, 1, 1})
	b.w(int32(3))
	b.name("", 80)
	b.name(`bird.wav`, 80)
	b.w([3]float32{-10, -15, 5}, float32(0.8), int32(10), int32(10), float32(250), float32(0))
	b.w(int32(4))
	b.name("", 80)
	b.w([3]float32{0, 0, 0}, int32(47), float32(1e8), [4]float32{1, 2, 3, 4})
	return b.Bytes()
}

func convertTest(t *testing.T) (*wmap.Map, []byte) {
	t.Helper()
	gat, err := ParseGAT(buildGAT())
	if err != nil {
		t.Fatal(err)
	}
	gnd, err := ParseGND(buildGND())
	if err != nil {
		t.Fatal(err)
	}
	rsw, err := ParseRSW(buildRSW())
	if err != nil {
		t.Fatal(err)
	}
	m, atlas, err := Convert(gat, gnd, rsw, Names{
		Texture: func(p string) string { return "texture/" + p },
		Model:   func(p string) string { return "model/" + p },
		Sound:   func(p string) string { return "sound/" + p },
	}, "lightmap/t", Extras{})
	if err != nil {
		t.Fatal(err)
	}
	return m, atlas.Pix
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-5 }

func TestCells(t *testing.T) {
	m, _ := convertTest(t)
	c := m.Cells
	if c[0].Flags != wmap.Walkable|wmap.Shootable || c[0].Heights != [4]float32{0, 1, 2, 3} {
		t.Errorf("cell 0 %+v", c[0])
	}
	// The cliff blocks walking, and its undefined corner takes the average
	// of the others: -(5 + 10 + 15) / 3 / 5 = -2.
	if c[1].Flags != wmap.Shootable || c[1].Heights != [4]float32{-2, -1, -2, -3} {
		t.Errorf("cell 1 %+v", c[1])
	}
}

func TestGround(t *testing.T) {
	m, atlas := convertTest(t)
	g := m.Ground
	if len(g.Textures) != 1 || g.Textures[0] != "texture/grass.bmp" {
		t.Errorf("textures %q", g.Textures)
	}
	tl := g.Tiles[0]
	if tl.Texture != 0 || tl.Color != [4]uint8{10, 20, 30, 255} {
		t.Errorf("tile %+v", tl)
	}
	s := g.Surfaces[0]
	// The east wall points at a tile that does not exist.
	if s.Heights != [4]float32{1, 1, 2, 2} || s.Top != 0 || s.North != -1 || s.East != -1 {
		t.Errorf("surface %+v", s)
	}
	if g.Lightmap != "lightmap/t" || g.LightmapCount != 1 || g.LightmapColumns != 1 {
		t.Errorf("lightmap %q %d %d", g.Lightmap, g.LightmapCount, g.LightmapColumns)
	}
	// Texel (3, 2) has shadow byte 2 × 8 + 3 = 19.
	p := (2*8 + 3) * 4
	if atlas[p] != 10 || atlas[p+1] != 20 || atlas[p+2] != 30 || atlas[p+3] != 19 {
		t.Errorf("atlas texel %v", atlas[p:p+4])
	}
}

func TestWorld(t *testing.T) {
	m, _ := convertTest(t)
	// Latitude 30° tilts the light 30° from straight up. Longitude 45°
	// turns the tilt toward the south-west.
	d := m.Light.Direction
	if m.Light.Latitude != 30 || m.Light.Opacity != 0.5 || !near(d[1], float32(math.Sqrt(3)/2)) || !near(d[0], -0.25*float32(math.Sqrt2)) || !near(d[2], 0.25*float32(math.Sqrt2)) {
		t.Errorf("light %+v", m.Light)
	}
	w := m.Water
	if w == nil || w.Level != 2 || w.WaveHeight != 1 || w.Type != 3 || len(w.Zones) != 0 {
		t.Fatalf("water %+v", w)
	}
	if len(w.Textures) != 1 || w.Textures[0].Frames[7] != `texture/워터\water307.jpg` {
		t.Errorf("water textures %+v", w.Textures)
	}
	// The broken object is left out.
	if len(m.Objects) != 1 {
		t.Fatalf("%d objects", len(m.Objects))
	}
	o := m.Objects[0]
	// The map center is (1, 1) in cells, from the 1 × 1 ground. Position
	// (10, -5, 20) / 5 is (2, -1, 4): x = 3, up = 1, north = 5, so z = -5.
	if o.Model != `model/forest\tree.rsm` || !near(o.Matrix[12], 3) || !near(o.Matrix[13], 1) || !near(o.Matrix[14], -5) {
		t.Errorf("object %q translation %v", o.Model, o.Matrix[12:15])
	}
	// A quarter turn around RO's Y turns east (+X) to south: +Z in map space.
	if !near(o.Matrix[0], 0) || !near(o.Matrix[2], 1) {
		t.Errorf("object x axis %v", o.Matrix[0:3])
	}
	s := m.Sounds[0]
	if s.Name != "sound/bird.wav" || !near(s.Pos[0], -1) || !near(s.Pos[1], 3) || !near(s.Pos[2], -2) || s.Range != 50 || s.Cycle != 7 {
		t.Errorf("sound %+v", s)
	}
	if e := m.Effects[0]; e.ID != 47 || e.Delay != 1e8 {
		t.Errorf("effect %+v", e)
	}
	if _, err := m.Encode(); err != nil {
		t.Fatal(err)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(uint8(0), buildGAT())
	f.Add(uint8(1), buildGND())
	f.Add(uint8(2), buildRSW())
	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		switch kind % 3 {
		case 0:
			_, _ = ParseGAT(data)
		case 1:
			_, _ = ParseGND(data)
		default:
			_, _ = ParseRSW(data)
		}
	})
}

func TestUnnamedSound(t *testing.T) {
	gat, _ := ParseGAT(buildGAT())
	gnd, _ := ParseGND(buildGND())
	rsw, _ := ParseRSW(buildRSW())
	m, _, err := Convert(gat, gnd, rsw, Names{
		Texture: func(p string) string { return "texture/" + p },
		Model:   func(p string) string { return "model/" + p },
		Sound:   func(p string) string { return "" },
	}, "lightmap/t", Extras{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sounds) != 0 {
		t.Errorf("%d sounds, want the unnamed one left out", len(m.Sounds))
	}
}

func TestConvertEmitter(t *testing.T) {
	names := Names{Texture: func(p string) string { return "texture/" + p }}
	e := Emitter{
		Texture: `effect\smoke1.bmp`, SrcMode: 9, DestMode: 2, Color: [4]float32{5, 15, 20, 300},
		Dir1: [3]float32{-3, -5, -3}, Dir2: [3]float32{5, 0, 5}, Gravity: [3]float32{0.7, -2, 0.7},
		Radius: [3]float32{-5, 0, 10}, Life: [2]float32{4, 4}, Rate: [2]float32{3, 4},
		Size: [2]float32{15, 20}, MaxCount: 5000, ZEnable: true, Scale: [2]float32{1, 1},
	}
	w, ok := convertEmitter(e, [3]float32{1, 2, -3}, names)
	if !ok {
		t.Fatal("left out")
	}
	// Y and Z flip, units become cells, and radii become sizes.
	if w.Dir1 != [3]float32{-0.6, 1, 0.6} || w.Gravity != [3]float32{0.14, 0.4, -0.14} || w.Radius != [3]float32{1, 0, 2} {
		t.Errorf("vectors %v %v %v", w.Dir1, w.Gravity, w.Radius)
	}
	if w.Size != [2]float32{3, 4} || w.Color != [4]uint8{5, 15, 20, 255} || w.MaxCount != 1000 || w.Flags != wmap.DepthTest {
		t.Errorf("emitter %+v", w)
	}
	if w.Texture != `texture/effect\smoke1.bmp` || w.SrcBlend != 9 || w.DstBlend != 2 {
		t.Errorf("texture %q, blend %d %d", w.Texture, w.SrcBlend, w.DstBlend)
	}
	e.Texture = ""
	if _, ok := convertEmitter(e, [3]float32{}, names); ok {
		t.Error("kept an emitter without a texture")
	}
}
