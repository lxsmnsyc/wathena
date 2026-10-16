package model

import (
	"math"
	"testing"
)

func TestSimple(t *testing.T) {
	grey := [4]float32{0.5, 0.5, 0.5, 1}
	green := [4]float32{0, 0.5, 0, 1}
	glb, err := Simple([]Part{
		Box([3]float32{2, 1, 2}, [3]float32{3, 2, 3}, grey),
		Cone([3]float32{2.5, 2, 2.5}, 1, 2, 6, green),
	})
	if err != nil {
		t.Fatal(err)
	}
	if names, err := Inspect(glb); err != nil || len(names) != 0 {
		t.Fatalf("Inspect: %v, %v", names, err)
	}
	g, err := readGLB(glb)
	if err != nil {
		t.Fatal(err)
	}
	d := &g.doc
	if len(d.Images) != 0 || len(d.Textures) != 0 || len(d.Materials) != 2 || len(d.Meshes) != 1 || len(d.Meshes[0].Primitives) != 2 {
		t.Fatalf("doc: %d images, %d textures, %d materials, %d meshes", len(d.Images), len(d.Textures), len(d.Materials), len(d.Meshes))
	}
	for i, m := range d.Materials {
		if m.PBR.BaseColorFactor == nil || m.PBR.BaseColorTexture != nil {
			t.Errorf("material %d is not a plain color", i)
		}
	}
	lo := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	hi := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for pi, p := range d.Meshes[0].Primitives {
		for _, key := range []string{"POSITION", "NORMAL", "TEXCOORD_0"} {
			if _, ok := p.Attributes[key]; !ok {
				t.Fatalf("primitive %d has no %s", pi, key)
			}
		}
		a := d.Accessors[p.Attributes["POSITION"]]
		if len(a.Min) != 3 || len(a.Max) != 3 {
			t.Fatalf("primitive %d positions have no bounds", pi)
		}
		for k := range 3 {
			lo[k], hi[k] = min(lo[k], a.Min[k]), max(hi[k], a.Max[k])
		}
		// Every normal points away from the part's center, and is a unit
		// vector.
		pos, _ := g.floats(p.Attributes["POSITION"])
		nrm, _ := g.floats(p.Attributes["NORMAL"])
		var c [3]float64
		for v := 0; v < len(pos); v += 3 {
			for k := range 3 {
				c[k] += pos[v+k] / float64(len(pos)/3)
			}
		}
		for v := 0; v < len(pos); v += 9 {
			var mid [3]float64
			for k := range 3 {
				mid[k] = (pos[v+k]+pos[v+3+k]+pos[v+6+k])/3 - c[k]
			}
			n := [3]float64{nrm[v], nrm[v+1], nrm[v+2]}
			if dot(n, mid) <= 0 || math.Abs(dot(n, n)-1) > 1e-5 {
				t.Fatalf("primitive %d triangle %d faces inward", pi, v/9)
			}
		}
	}
	// The model stands on y = 0 and is centered on x and z.
	if lo[1] != 0 || hi[1] != 3 || lo[0] != -hi[0] || lo[2] != -hi[2] || hi[0] != 1 {
		t.Errorf("bounds %v to %v", lo, hi)
	}
	// A box has 12 triangles, and a cone of 6 sides has 6 sides and 4
	// base triangles.
	if n := d.Accessors[d.Meshes[0].Primitives[0].Indices].Count; n != 36 {
		t.Errorf("box has %d indices", n)
	}
	if n := d.Accessors[d.Meshes[0].Primitives[1].Indices].Count; n != 30 {
		t.Errorf("cone has %d indices", n)
	}
	if _, err := Simple(nil); err == nil {
		t.Error("no parts is accepted")
	}
}
