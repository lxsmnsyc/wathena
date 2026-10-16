package placeholder

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"

	"golang.org/x/image/webp"

	"github.com/lxsmnsyc/wathena/internal/assets/model"
	"github.com/lxsmnsyc/wathena/internal/assets/texture"
	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

func TestSprite(t *testing.T) {
	b, err := Sprite().Encode()
	if err != nil {
		t.Fatal(err)
	}
	s, err := wspr.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	img := s.RenderFrame(0, 0, 64)
	if img.NRGBAAt(32, 12).A == 0 {
		t.Error("the shape is not drawn above the origin")
	}
	if img.NRGBAAt(32, 31).A == 0 {
		t.Error("the shadow is not drawn at the origin")
	}
	if p := os.Getenv("WATHENA_PREVIEW"); p != "" {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, s.RenderFrame(0, 0, 64)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTexture(t *testing.T) {
	img := Texture()
	if img.Rect.Dx() != 64 || img.NRGBAAt(0, 0) == img.NRGBAAt(8, 0) || img.NRGBAAt(0, 0) != img.NRGBAAt(63, 63) {
		t.Error("texture is not a tiling checkerboard")
	}
}

func TestMap(t *testing.T) {
	m, lm := Map()
	b, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := wmap.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	// The ground and the cells agree on heights, and the hill is in the
	// middle.
	center := got.Cells[12*mapSize+12].Heights[0]
	surface := got.Ground.Surfaces[6*mapSize/2+6].Heights[0]
	if center != surface || center < 1 || got.Cells[0].Heights[0] > 0.1 {
		t.Errorf("heights: center %v, surface %v, corner %v", center, surface, got.Cells[0].Heights[0])
	}
	if lm.Rect.Dx() != 8 || lm.NRGBAAt(3, 3).A != 255 {
		t.Error("lightmap is not one neutral block")
	}
}

func TestGround(t *testing.T) {
	means := map[string][3]float64{"placeholder/texture": mean(Texture())}
	for _, m := range GroundMaterials {
		img := Ground(m)
		b, err := texture.Encode(img)
		if err != nil {
			t.Fatal(err)
		}
		got, err := webp.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if r := got.Bounds(); r.Dx() != 64 || r.Dy() != 64 {
			t.Errorf("%s is %d × %d", m, r.Dx(), r.Dy())
		}
		// The tile has two tones, and its edges match as well as its
		// inside does, so it tiles seamlessly.
		var inner, edge float64
		for i := range 64 {
			edge += diff(img.NRGBAAt(63, i), img.NRGBAAt(0, i)) + diff(img.NRGBAAt(i, 63), img.NRGBAAt(i, 0))
			inner += diff(img.NRGBAAt(31, i), img.NRGBAAt(32, i)) + diff(img.NRGBAAt(i, 31), img.NRGBAAt(i, 32))
		}
		if edge > 2*inner+64 {
			t.Errorf("%s has a seam: edge %v, inside %v", m, edge, inner)
		}
		lo, hi := 255.0, 0.0
		for y := range 64 {
			for x := range 64 {
				c := img.NRGBAAt(x, y)
				v := float64(c.R) + float64(c.G) + float64(c.B)
				lo, hi = min(lo, v), max(hi, v)
			}
		}
		if hi-lo < 30 {
			t.Errorf("%s is flat", m)
		}
		// Every texture's mean color is far from every other's.
		mu := mean(img)
		for name, other := range means {
			d := math.Abs(mu[0]-other[0]) + math.Abs(mu[1]-other[1]) + math.Abs(mu[2]-other[2])
			if d < 40 {
				t.Errorf("%s looks like %s: %v and %v", m, name, mu, other)
			}
		}
		means[m] = mu
	}
	if Ground("lava") != nil {
		t.Error("an unknown material has a texture")
	}
}

func mean(img *image.NRGBA) [3]float64 {
	var s [3]float64
	n := float64(img.Rect.Dx() * img.Rect.Dy())
	for i := 0; i < len(img.Pix); i += 4 {
		for k := range 3 {
			s[k] += float64(img.Pix[i+k]) / n
		}
	}
	return s
}

func diff(a, b color.NRGBA) float64 {
	return math.Abs(float64(a.R)-float64(b.R)) + math.Abs(float64(a.G)-float64(b.G)) + math.Abs(float64(a.B)-float64(b.B))
}

// glbDoc is the part of a GLB file's JSON that the tests read.
type glbDoc struct {
	Images    []json.RawMessage `json:"images"`
	Accessors []struct {
		Min []float64 `json:"min"`
		Max []float64 `json:"max"`
	} `json:"accessors"`
	Meshes []struct {
		Primitives []struct {
			Attributes map[string]int `json:"attributes"`
		} `json:"primitives"`
	} `json:"meshes"`
}

// checkModel checks a placeholder model with the build's model check and
// returns its bounds.
func checkModel(t *testing.T, name string, glb []byte) (lo, hi [3]float64) {
	t.Helper()
	names, err := model.Inspect(glb)
	if err != nil || len(names) != 0 {
		t.Fatalf("%s: Inspect: %v, %v", name, names, err)
	}
	if !bytes.Contains(glb, []byte(`"baseColorFactor"`)) || bytes.Contains(glb, []byte(`"uri"`)) || bytes.Contains(glb, []byte("KHR_materials_unlit")) {
		t.Errorf("%s: materials are not plain colors", name)
	}
	n := binary.LittleEndian.Uint32(glb[12:])
	var d glbDoc
	if err := json.Unmarshal(glb[20:20+n], &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Images) != 0 {
		t.Errorf("%s has images", name)
	}
	lo, hi = [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}, [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, m := range d.Meshes {
		for _, p := range m.Primitives {
			for _, key := range []string{"POSITION", "NORMAL", "TEXCOORD_0"} {
				if _, ok := p.Attributes[key]; !ok {
					t.Fatalf("%s: a primitive has no %s", name, key)
				}
			}
			a := d.Accessors[p.Attributes["POSITION"]]
			if len(a.Min) != 3 || len(a.Max) != 3 {
				t.Fatalf("%s: positions have no bounds", name)
			}
			for k := range 3 {
				lo[k], hi[k] = min(lo[k], a.Min[k]), max(hi[k], a.Max[k])
			}
		}
	}
	// It stands on y = 0 and is centered on x and z.
	if math.Abs(lo[1]) > 1e-6 || math.Abs(lo[0]+hi[0]) > 1e-5 || math.Abs(lo[2]+hi[2]) > 1e-5 {
		t.Errorf("%s: bounds %v to %v", name, lo, hi)
	}
	if len(glb) > 16<<10 {
		t.Errorf("%s is %d bytes", name, len(glb))
	}
	return lo, hi
}

func TestModel(t *testing.T) {
	glb, err := Model()
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := checkModel(t, "placeholder/model", glb)
	if hi[1] != 1 || hi[0]-lo[0] != 1 || hi[2]-lo[2] != 1 {
		t.Errorf("placeholder/model is not 1 × 1 × 1: %v to %v", lo, hi)
	}
}

func TestProps(t *testing.T) {
	heights := map[string]float64{}
	for _, p := range Props {
		glb, err := Prop(p)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := Prop(p)
		if !bytes.Equal(glb, again) {
			t.Errorf("%s is not deterministic", p)
		}
		lo, hi := checkModel(t, p, glb)
		// The fountain fills 4 cells of a town's square, and the gate
		// spans a main road of 4 cells.
		wide := 3.0
		if p == "fountain" || p == "gate" {
			wide = 4.6
		}
		if hi[1] < 0.3 || hi[1] > 4.5 || hi[0]-lo[0] > wide || hi[2]-lo[2] > wide {
			t.Errorf("%s has size %v to %v", p, lo, hi)
		}
		heights[p] = hi[1]
	}
	if heights["tree"] < 2.5 || heights["pine"] <= heights["tree"] || heights["bush"] > 1 || heights["rock"] > 1 {
		t.Errorf("heights %v", heights)
	}
	if _, err := Prop("castle"); err == nil {
		t.Error("an unknown prop has a model")
	}
}

// TestBuildings: each placeholder building fits its footprint.
func TestBuildings(t *testing.T) {
	for _, w := range BuildingSizes {
		for _, d := range BuildingSizes {
			glb, err := Building(w, d)
			if err != nil {
				t.Fatal(err)
			}
			lo, hi := checkModel(t, "building", glb)
			if hi[0]-lo[0] > float64(w)+0.2 || hi[2]-lo[2] > float64(d)+0.2 || hi[0]-lo[0] < float64(w)-0.5 || hi[1] < 2 || hi[1] > 6.5 {
				t.Errorf("%d × %d: %v to %v", w, d, lo, hi)
			}
		}
	}
	if _, err := Building(3, 4); err == nil {
		t.Error("a 3 × 4 building has a model")
	}
}
