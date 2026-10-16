package model

import (
	"fmt"
	"math"
)

// This file writes simple untextured models from colored solids, for the
// placeholder pack.

// Part is one colored solid of a simple model. Each part becomes one
// primitive with flat normals and a material that has only a base color.
type Part struct {
	// Color is the glTF baseColorFactor: linear RGBA from 0 to 1.
	Color [4]float32
	// Triangles are the part's triangles, counterclockwise when seen from
	// outside.
	Triangles [][3][3]float32
}

// Loft returns the convex solid between two rings of points with the same
// count, with both ends closed. A ring may repeat one point to make a
// cone. Triangles are turned to face away from the solid's center, so the
// rings may run either way. Triangles with no area are left out.
func Loft(bottom, top [][3]float32, color [4]float32) Part {
	n := len(bottom)
	if n < 3 || len(top) != n {
		panic("model: Loft needs two rings of at least 3 points with the same count")
	}
	var center [3]float64
	for _, ring := range [][][3]float32{bottom, top} {
		for _, p := range ring {
			for k := range 3 {
				center[k] += float64(p[k]) / float64(2*n)
			}
		}
	}
	p := Part{Color: color}
	add := func(a, b, c [3]float32) {
		ab, ac := sub(f64(b), f64(a)), sub(f64(c), f64(a))
		nrm := cross(ab, ac)
		if dot(nrm, nrm) < 1e-12 {
			return
		}
		mid := [3]float64{(float64(a[0]+b[0]+c[0]))/3 - center[0], (float64(a[1]+b[1]+c[1]))/3 - center[1], (float64(a[2]+b[2]+c[2]))/3 - center[2]}
		if dot(nrm, mid) < 0 {
			b, c = c, b
		}
		p.Triangles = append(p.Triangles, [3][3]float32{a, b, c})
	}
	for i := range n {
		j := (i + 1) % n
		add(bottom[i], bottom[j], top[j])
		add(bottom[i], top[j], top[i])
	}
	for i := 1; i+1 < n; i++ {
		add(bottom[0], bottom[i], bottom[i+1])
		add(top[0], top[i], top[i+1])
	}
	return p
}

// Box returns an axis-aligned box from lo to hi.
func Box(lo, hi [3]float32, color [4]float32) Part {
	ring := func(y float32) [][3]float32 {
		return [][3]float32{{lo[0], y, lo[2]}, {hi[0], y, lo[2]}, {hi[0], y, hi[2]}, {lo[0], y, hi[2]}}
	}
	return Loft(ring(lo[1]), ring(hi[1]), color)
}

// Circle returns sides points on a horizontal circle around center.
func Circle(center [3]float32, radius float32, sides int) [][3]float32 {
	out := make([][3]float32, sides)
	for i := range out {
		a := 2 * math.Pi * float64(i) / float64(sides)
		out[i] = [3]float32{center[0] + radius*float32(math.Cos(a)), center[1], center[2] + radius*float32(math.Sin(a))}
	}
	return out
}

// Cone returns a cone with a base of sides points around base and its tip
// height above it.
func Cone(base [3]float32, radius, height float32, sides int, color [4]float32) Part {
	tip := make([][3]float32, sides)
	for i := range tip {
		tip[i] = [3]float32{base[0], base[1] + height, base[2]}
	}
	return Loft(Circle(base, radius, sides), tip, color)
}

// Simple writes an untextured GLB model in the assets-v1 profile from
// parts. It moves the parts so the model is centered on x and z and its
// lowest point is on y = 0. Texture coordinates are the x and z
// positions, since no texture uses them.
func Simple(parts []Part) ([]byte, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("model: no parts")
	}
	lo := [3]float32{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	hi := [3]float32{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for i, p := range parts {
		if len(p.Triangles) == 0 {
			return nil, fmt.Errorf("model: part %d has no triangles", i)
		}
		for _, t := range p.Triangles {
			for _, v := range t {
				for k := range 3 {
					lo[k], hi[k] = min(lo[k], v[k]), max(hi[k], v[k])
				}
			}
		}
	}
	shift := [3]float32{-(lo[0] + hi[0]) / 2, -lo[1], -(lo[2] + hi[2]) / 2}

	var b builder
	b.doc.Asset = gltfAsset{Version: "2.0", Generator: "wathena"}
	materials := map[[4]float32]int{}
	var mesh gltfMesh
	for i, p := range parts {
		if 3*len(p.Triangles) > MaxPrimitiveVertices {
			return nil, fmt.Errorf("model: part %d has too many vertices", i)
		}
		mat, ok := materials[p.Color]
		if !ok {
			c := p.Color
			b.doc.Materials = append(b.doc.Materials, gltfMaterial{PBR: gltfPBR{BaseColorFactor: &c, MetallicFactor: 0, RoughnessFactor: 1}})
			mat = len(b.doc.Materials) - 1
			materials[p.Color] = mat
		}
		positions := make([]float32, 0, 9*len(p.Triangles))
		normals := make([]float32, 0, 9*len(p.Triangles))
		uvs := make([]float32, 0, 6*len(p.Triangles))
		idx := make([]uint32, 0, 3*len(p.Triangles))
		for _, t := range p.Triangles {
			n, _ := normalize(cross(sub(f64(t[1]), f64(t[0])), sub(f64(t[2]), f64(t[0]))))
			for _, v := range t {
				idx = append(idx, uint32(len(positions)/3)) // #nosec G115 -- at most MaxPrimitiveVertices.
				positions = append(positions, v[0]+shift[0], v[1]+shift[1], v[2]+shift[2])
				normals = append(normals, float32(n[0]), float32(n[1]), float32(n[2]))
				uvs = append(uvs, v[0]+shift[0], v[2]+shift[2])
			}
		}
		mesh.Primitives = append(mesh.Primitives, gltfPrimitive{
			Attributes: map[string]int{
				"POSITION":   b.floats(positions, 3, "VEC3", glArrayBuffer, true),
				"NORMAL":     b.floats(normals, 3, "VEC3", glArrayBuffer, false),
				"TEXCOORD_0": b.floats(uvs, 2, "VEC2", glArrayBuffer, false),
			},
			Indices:  b.indices(idx, len(positions)/3),
			Material: mat,
		})
	}
	b.doc.Meshes = []gltfMesh{mesh}
	m := 0
	b.doc.Nodes = []gltfNode{{Name: "root", Mesh: &m}}
	b.doc.Scenes = []gltfScene{{Nodes: []int{0}}}
	out, err := b.glb()
	if err != nil {
		return nil, err
	}
	if len(out) > MaxBytes {
		return nil, fmt.Errorf("model: %d bytes is over the size limit", len(out))
	}
	return out, nil
}

func f64(v [3]float32) [3]float64 {
	return [3]float64{float64(v[0]), float64(v[1]), float64(v[2])}
}

func dot(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}
