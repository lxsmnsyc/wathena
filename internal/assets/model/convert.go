package model

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
)

// This file converts version 1 RSM models to glTF.
//
// RO places a node with parent × T(pos) × R × S, where R comes from the
// first rotation keyframe or from an angle and axis. The node's vertices
// are then moved by T(offset) × M3. A model is centered on its bounding
// box and set on the ground: T(-center x, -max y, -center z).
//
// RO's model space has Y pointing down. The glTF file has Y pointing up,
// so everything is mirrored through Y by F = diag(1, -1, 1): vertices
// become F × v, and each node transform X becomes F × X × F, which is
// still a translation, rotation, and scale. The client multiplies map
// object placements by F the same way.
//
// Matrix math uses explicit float64 conversions between steps. Go may
// otherwise fuse multiply-adds on some platforms, which would change the
// output bytes.

// ErrUnsupported is returned for models this converter does not handle yet.
var ErrUnsupported = errors.New("model: unsupported rsm version")

// MaxBytes is the largest model the build accepts.
const MaxBytes = 16 << 20

// MaxPrimitiveVertices is the most vertices one primitive can have.
const MaxPrimitiveVertices = 1 << 16

// mat4 is a column-major 4 × 4 matrix, as in OpenGL.
type mat4 [16]float64

func identity() mat4 { return mat4{0: 1, 5: 1, 10: 1, 15: 1} }

func (a mat4) mul(b mat4) mat4 {
	var out mat4
	for c := range 4 {
		for r := range 4 {
			var s float64
			for k := range 4 {
				s = float64(s + float64(a[k*4+r]*b[c*4+k]))
			}
			out[c*4+r] = s
		}
	}
	return out
}

func translation(v [3]float64) mat4 {
	m := identity()
	m[12], m[13], m[14] = v[0], v[1], v[2]
	return m
}

func scaling(v [3]float64) mat4 { return mat4{0: v[0], 5: v[1], 10: v[2], 15: 1} }

// quatMatrix returns the rotation for a quaternion x, y, z, w, which is
// normalized first. A zero quaternion is no rotation.
func quatMatrix(q [4]float64) mat4 {
	n := math.Sqrt(float64(float64(q[0]*q[0])+float64(q[1]*q[1])) + float64(float64(q[2]*q[2])+float64(q[3]*q[3])))
	if n == 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return identity()
	}
	x, y, z, w := q[0]/n, q[1]/n, q[2]/n, q[3]/n
	return mat4{
		1 - float64(2*float64(float64(y*y)+float64(z*z))), float64(2 * float64(float64(x*y)+float64(z*w))), float64(2 * float64(float64(x*z)-float64(y*w))), 0,
		float64(2 * float64(float64(x*y)-float64(z*w))), 1 - float64(2*float64(float64(x*x)+float64(z*z))), float64(2 * float64(float64(y*z)+float64(x*w))), 0,
		float64(2 * float64(float64(x*z)+float64(y*w))), float64(2 * float64(float64(y*z)-float64(x*w))), 1 - float64(2*float64(float64(x*x)+float64(y*y))), 0,
		0, 0, 0, 1,
	}
}

// axisQuat returns the quaternion for a rotation by angle radians around
// axis. A zero axis is no rotation.
func axisQuat(angle float64, axis [3]float64) [4]float64 {
	n := math.Sqrt(float64(float64(axis[0]*axis[0])+float64(axis[1]*axis[1])) + float64(axis[2]*axis[2]))
	if n < 1e-6 || math.IsNaN(n) || math.IsInf(n, 0) || math.IsNaN(angle) || math.IsInf(angle, 0) {
		return [4]float64{0, 0, 0, 1}
	}
	s := math.Sin(angle / 2)
	return [4]float64{float64(axis[0] / n * s), float64(axis[1] / n * s), float64(axis[2] / n * s), math.Cos(angle / 2)}
}

func (a mat4) point(v [3]float64) [3]float64 {
	var out [3]float64
	for r := range 3 {
		out[r] = float64(float64(float64(a[r]*v[0])+float64(a[4+r]*v[1])) + float64(float64(a[8+r]*v[2])+a[12+r]))
	}
	return out
}

func (a mat4) det3() float64 {
	return float64(float64(a[0]*float64(float64(a[5]*a[10])-float64(a[9]*a[6])))-
		float64(a[4]*float64(float64(a[1]*a[10])-float64(a[9]*a[2])))) +
		float64(a[8]*float64(float64(a[1]*a[6])-float64(a[5]*a[2])))
}

func vec(v [3]float32) [3]float64 { return [3]float64{float64(v[0]), float64(v[1]), float64(v[2])} }

func sub(a, b [3]float64) [3]float64 {
	return [3]float64{float64(a[0] - b[0]), float64(a[1] - b[1]), float64(a[2] - b[2])}
}

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{
		float64(float64(a[1]*b[2]) - float64(a[2]*b[1])),
		float64(float64(a[2]*b[0]) - float64(a[0]*b[2])),
		float64(float64(a[0]*b[1]) - float64(a[1]*b[0])),
	}
}

func normalize(v [3]float64) ([3]float64, bool) {
	n := math.Sqrt(float64(float64(v[0]*v[0])+float64(v[1]*v[1])) + float64(v[2]*v[2]))
	if n == 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return [3]float64{0, 1, 0}, false
	}
	return [3]float64{v[0] / n, v[1] / n, v[2] / n}, true
}

// flip is F = diag(1, -1, 1), the mirror between RO's Y-down model space
// and glTF's Y-up space.
var flip = scaling([3]float64{1, -1, 1})

func finite(v ...float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 1e7 {
			return false
		}
	}
	return true
}

// TextureURI returns the glTF image URI for a texture name.
func TextureURI(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "asset:" + strings.Join(parts, "/")
}

// Convert converts a version 1 model to glTF binary. textureName maps a
// texture path from the model, relative to data\texture, to an asset name.
func Convert(m *RSM, textureName func(string) string) ([]byte, error) {
	if m.Version >= 20 {
		return nil, ErrUnsupported
	}
	c := &converter{m: m, textureName: textureName}
	return c.run()
}

type converter struct {
	m           *RSM
	textureName func(string) string
	b           builder
	// world holds each placed node's matrix in RO space at frame 0.
	world map[int]mat4
	// gltfNode maps an RSM node to its glTF node.
	gltfNode  map[int]int
	materials map[[2]int]int
	textures  map[int]int
}

// nodeMatrix is T(pos) × R × S for a node at frame 0.
func nodeMatrix(n *Node) (mat4, [4]float64) {
	q := axisQuat(float64(n.RotAngle), vec(n.RotAxis))
	if len(n.RotKeys) > 0 {
		k := n.RotKeys[0].Q
		q = [4]float64{float64(k[0]), float64(k[1]), float64(k[2]), float64(k[3])}
	}
	return translation(vec(n.Pos)).mul(quatMatrix(q)).mul(scaling(vec(n.Scale))), q
}

// meshMatrix is T(offset) × M3. A model with one node skips the offset.
func (c *converter) meshMatrix(n *Node) mat4 {
	m3 := mat4{15: 1}
	for col := range 3 {
		for row := range 3 {
			m3[col*4+row] = float64(n.Mat3[col*3+row])
		}
	}
	if len(c.m.Nodes) == 1 {
		return m3
	}
	return translation(vec(n.Offset)).mul(m3)
}

func (c *converter) run() ([]byte, error) {
	m := c.m
	main := 0
	for i, n := range m.Nodes {
		if n.Name == m.MainNode {
			main = i
			break
		}
	}
	// Children are nodes that name the parent, as RO links them. A node
	// that names itself as its parent has no children.
	children := map[int][]int{}
	for i, n := range m.Nodes {
		if n.Name == n.Parent {
			continue
		}
		for j, ch := range m.Nodes {
			if j != i && ch.Parent == n.Name {
				children[i] = append(children[i], j)
			}
		}
	}

	// Place nodes from the main node down, and find the bounding box.
	c.world = map[int]mat4{}
	lo := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	parentOf := map[int]int{main: -1}
	order := []int{main}
	for i := 0; i < len(order); i++ {
		n := order[i]
		parent := identity()
		if p := parentOf[n]; p >= 0 {
			parent = c.world[p]
		}
		local, _ := nodeMatrix(m.Nodes[n])
		c.world[n] = parent.mul(local)
		full := c.world[n].mul(c.meshMatrix(m.Nodes[n]))
		for _, v := range m.Nodes[n].Vertices {
			p := full.point(vec(v))
			for k := range 3 {
				lo[k], hi[k] = min(lo[k], p[k]), max(hi[k], p[k])
			}
		}
		for _, ch := range children[n] {
			// A node reached twice, through a loop or a repeated name, is
			// placed once.
			if _, seen := parentOf[ch]; !seen {
				parentOf[ch] = n
				order = append(order, ch)
			}
		}
	}
	if math.IsInf(lo[0], 0) {
		lo, hi = [3]float64{}, [3]float64{}
	}
	if !finite(lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]) {
		return nil, fmt.Errorf("%w: vertices are out of range", ErrFormat)
	}
	center := [3]float64{float64(lo[0]+hi[0]) / 2, 0, float64(lo[2]+hi[2]) / 2}

	// The root node holds the centering, mirrored: F × T(-cx, -max y, -cz) × F.
	c.b.doc.Asset = gltfAsset{Version: "2.0", Generator: "wathena"}
	c.b.doc.Nodes = append(c.b.doc.Nodes, gltfNode{Name: "root", Translation: f32p3([3]float64{-center[0], hi[1], -center[2]})})
	c.gltfNode = map[int]int{}
	c.materials = map[[2]int]int{}
	c.textures = map[int]int{}
	// Nodes that RO does not reach from the main node are drawn without a
	// node transform, so they hang from the root with none.
	for i := range m.Nodes {
		if _, ok := parentOf[i]; !ok {
			parentOf[i] = -1
			c.world[i] = identity()
			order = append(order, i)
		}
	}
	for _, n := range order {
		if err := c.addNode(n, parentOf[n]); err != nil {
			return nil, err
		}
	}
	c.b.doc.Scenes = []gltfScene{{Nodes: []int{0}}}
	c.b.doc.Scene = 0
	c.addAnimation(order)
	out, err := c.b.glb()
	if err != nil {
		return nil, err
	}
	if len(out) > MaxBytes {
		return nil, fmt.Errorf("model: %d bytes is over the size limit", len(out))
	}
	return out, nil
}

func f32p3(v [3]float64) *[3]float32 {
	return &[3]float32{float32(v[0]), float32(v[1]), float32(v[2])}
}

// mirrorQuat conjugates a rotation by F: the axis mirrors through Y and
// the angle changes sign, which negates x and z.
func mirrorQuat(q [4]float64) [4]float32 {
	n := math.Sqrt(float64(float64(q[0]*q[0])+float64(q[1]*q[1])) + float64(float64(q[2]*q[2])+float64(q[3]*q[3])))
	if n == 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return [4]float32{0, 0, 0, 1}
	}
	return [4]float32{float32(-q[0] / n), float32(q[1] / n), float32(-q[2] / n), float32(q[3] / n)}
}

func (c *converter) addNode(i, parent int) error {
	n := c.m.Nodes[i]
	gn := gltfNode{Name: n.Name}
	if parent >= 0 || i == c.mainIndex() {
		_, q := nodeMatrix(n)
		pos, scale := vec(n.Pos), vec(n.Scale)
		if !finite(pos[0], pos[1], pos[2], scale[0], scale[1], scale[2]) {
			return fmt.Errorf("%w: node transform is out of range", ErrFormat)
		}
		gn.Translation = f32p3([3]float64{pos[0], -pos[1], pos[2]})
		r := mirrorQuat(q)
		gn.Rotation = &r
		gn.Scale = f32p3(scale)
	}
	mesh, err := c.addMesh(i)
	if err != nil {
		return err
	}
	if mesh >= 0 {
		gn.Mesh = &mesh
	}
	c.b.doc.Nodes = append(c.b.doc.Nodes, gn)
	idx := len(c.b.doc.Nodes) - 1
	c.gltfNode[i] = idx
	p := 0
	if parent >= 0 {
		p = c.gltfNode[parent]
	}
	c.b.doc.Nodes[p].Children = append(c.b.doc.Nodes[p].Children, idx)
	return nil
}

func (c *converter) mainIndex() int {
	for i, n := range c.m.Nodes {
		if n.Name == c.m.MainNode {
			return i
		}
	}
	return 0
}

// material returns the material for a model texture and sidedness.
func (c *converter) material(tex int, twoSide bool) int {
	key := [2]int{tex, 0}
	if twoSide {
		key[1] = 1
	}
	if id, ok := c.materials[key]; ok {
		return id
	}
	mat := gltfMaterial{
		PBR:         gltfPBR{MetallicFactor: 0, RoughnessFactor: 1},
		AlphaMode:   "MASK",
		AlphaCutoff: 0.5,
		DoubleSided: twoSide,
	}
	if c.m.Alpha < 255 {
		mat.AlphaMode, mat.AlphaCutoff = "BLEND", 0
		mat.PBR.BaseColorFactor = &[4]float32{1, 1, 1, float32(c.m.Alpha) / 255}
	}
	t, ok := c.textures[tex]
	if !ok {
		if len(c.b.doc.Samplers) == 0 {
			c.b.doc.Samplers = []gltfSampler{{MagFilter: glLinear, MinFilter: glLinearMipmap, WrapS: glRepeat, WrapT: glRepeat}}
		}
		c.b.doc.Images = append(c.b.doc.Images, gltfImage{URI: TextureURI(c.textureName(c.m.Textures[tex]))})
		c.b.doc.Textures = append(c.b.doc.Textures, gltfTexture{Sampler: 0, Source: len(c.b.doc.Images) - 1})
		t = len(c.b.doc.Textures) - 1
		c.textures[tex] = t
	}
	mat.PBR.BaseColorTexture = &gltfTexInfo{Index: t}
	c.b.doc.Materials = append(c.b.doc.Materials, mat)
	c.materials[key] = len(c.b.doc.Materials) - 1
	return len(c.b.doc.Materials) - 1
}

// globalLinear returns the node's glTF world matrix at frame 0:
// F × C × world × F. Only its linear part is used, so C is left out.
func (c *converter) globalLinear(i int) mat4 {
	return flip.mul(c.world[i]).mul(flip)
}

func (c *converter) addMesh(i int) (int, error) {
	n := c.m.Nodes[i]
	// Vertices are stored as F × T(offset) × M3 × v.
	toMesh := flip.mul(c.meshMatrix(n))
	pos := make([][3]float64, len(n.Vertices))
	for k, v := range n.Vertices {
		pos[k] = toMesh.point(vec(v))
		if !finite(pos[k][0], pos[k][1], pos[k][2]) {
			return -1, fmt.Errorf("%w: vertices are out of range", ErrFormat)
		}
	}
	// Triangles keep facing outward: when the stored transform mirrors,
	// two corners swap.
	swap := toMesh.det3() < 0

	type tri struct {
		v, uv  [3]int
		normal [3]float64
		prim   [2]int
		smooth int32
	}
	var tris []tri
	for _, f := range n.Faces {
		t := tri{smooth: f.Smooth}
		ok := int(f.Texture) < len(n.TextureIDs)
		for k := range 3 {
			t.v[k], t.uv[k] = int(f.Vert[k]), int(f.UV[k])
			ok = ok && t.v[k] < len(pos) && t.uv[k] < len(n.UVs)
		}
		if !ok {
			continue
		}
		tex := n.TextureIDs[f.Texture]
		if tex < 0 || tex >= len(c.m.Textures) {
			continue
		}
		if swap {
			t.v[1], t.v[2] = t.v[2], t.v[1]
			t.uv[1], t.uv[2] = t.uv[2], t.uv[1]
		}
		t.normal, _ = normalize(cross(sub(pos[t.v[1]], pos[t.v[0]]), sub(pos[t.v[2]], pos[t.v[0]])))
		t.prim = [2]int{tex, 0}
		if f.TwoSide {
			t.prim[1] = 1
		}
		tris = append(tris, t)
	}
	if len(tris) == 0 {
		return -1, nil
	}

	// Smooth shading averages face normals per vertex within a smoothing
	// group. Unshaded models point every normal up in the world.
	type groupVertex struct {
		group int32
		v     int
	}
	smooth := map[groupVertex][3]float64{}
	if c.m.ShadeType == ShadeSmooth {
		for _, t := range tris {
			for k := range 3 {
				key := groupVertex{t.smooth, t.v[k]}
				s := smooth[key]
				smooth[key] = [3]float64{float64(s[0] + t.normal[0]), float64(s[1] + t.normal[1]), float64(s[2] + t.normal[2])}
			}
		}
	}
	var up [3]float64
	if c.m.ShadeType != ShadeFlat && c.m.ShadeType != ShadeSmooth {
		// The normal whose transform is world up: the transposed linear
		// part of the node's world matrix, times up.
		g := c.globalLinear(i)
		up, _ = normalize([3]float64{g[1], g[5], g[9]})
	}

	// One primitive per texture and sidedness, in a stable order.
	byPrim := map[[2]int][]tri{}
	var keys [][2]int
	for _, t := range tris {
		if _, ok := byPrim[t.prim]; !ok {
			keys = append(keys, t.prim)
		}
		byPrim[t.prim] = append(byPrim[t.prim], t)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a][0] != keys[b][0] {
			return keys[a][0] < keys[b][0]
		}
		return keys[a][1] < keys[b][1]
	})
	var mesh gltfMesh
	for _, key := range keys {
		type vertexKey struct {
			v, uv  int
			normal [3]float32
		}
		index := map[vertexKey]uint32{}
		var positions, normals, uvs []float32
		var idx []uint32
		for _, t := range byPrim[key] {
			for k := range 3 {
				nrm := t.normal
				switch c.m.ShadeType {
				case ShadeSmooth:
					nrm, _ = normalize(smooth[groupVertex{t.smooth, t.v[k]}])
				case ShadeFlat:
				default:
					nrm = up
				}
				vk := vertexKey{t.v[k], t.uv[k], [3]float32{float32(nrm[0]), float32(nrm[1]), float32(nrm[2])}}
				id, ok := index[vk]
				if !ok {
					id = uint32(len(positions) / 3) // #nosec G115 -- checked below.
					index[vk] = id
					p := pos[t.v[k]]
					positions = append(positions, float32(p[0]), float32(p[1]), float32(p[2]))
					normals = append(normals, vk.normal[:]...)
					uv := n.UVs[t.uv[k]]
					u, v := float64(uv[0]), float64(uv[1])
					if !finite(u, v) {
						u, v = 0, 0
					}
					uvs = append(uvs, float32(u), float32(v))
				}
				idx = append(idx, id)
			}
		}
		if len(positions)/3 > MaxPrimitiveVertices {
			return -1, fmt.Errorf("model: node %q has %d vertices for one texture, over the limit", n.Name, len(positions)/3)
		}
		mesh.Primitives = append(mesh.Primitives, gltfPrimitive{
			Attributes: map[string]int{
				"POSITION":   c.b.floats(positions, 3, "VEC3", glArrayBuffer, true),
				"NORMAL":     c.b.floats(normals, 3, "VEC3", glArrayBuffer, false),
				"TEXCOORD_0": c.b.floats(uvs, 2, "VEC2", glArrayBuffer, false),
			},
			Indices:  c.b.indices(idx, len(positions)/3),
			Material: c.material(key[0], key[1] == 1),
		})
	}
	c.b.doc.Meshes = append(c.b.doc.Meshes, mesh)
	return len(c.b.doc.Meshes) - 1, nil
}

// addAnimation adds the rotation keyframes of placed nodes as one
// animation. Version 1 keyframe times are in milliseconds.
func (c *converter) addAnimation(order []int) {
	var anim gltfAnimation
	for _, i := range order {
		keys := c.m.Nodes[i].RotKeys
		if len(keys) < 2 {
			continue
		}
		sorted := append([]RotKey(nil), keys...)
		sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].Frame < sorted[b].Frame })
		var times, values []float32
		for k, key := range sorted {
			// glTF needs increasing times, so repeated frames keep the first.
			if key.Frame < 0 || (k > 0 && key.Frame == sorted[k-1].Frame) {
				continue
			}
			q := mirrorQuat([4]float64{float64(key.Q[0]), float64(key.Q[1]), float64(key.Q[2]), float64(key.Q[3])})
			times = append(times, float32(key.Frame)/1000)
			values = append(values, q[:]...)
		}
		if len(times) < 2 {
			continue
		}
		in := c.b.floats(times, 1, "SCALAR", 0, true)
		out := c.b.floats(values, 4, "VEC4", 0, false)
		anim.Samplers = append(anim.Samplers, gltfAnimSample{Input: in, Output: out, Interpolation: "LINEAR"})
		anim.Channels = append(anim.Channels, gltfChannel{Sampler: len(anim.Samplers) - 1, Target: gltfTarget{Node: c.gltfNode[i], Path: "rotation"}})
	}
	if len(anim.Channels) == 0 {
		return
	}
	// The model loops over its whole length, which can be longer than the
	// last keyframe.
	anim.Extras = map[string]any{"loopSeconds": float64(max(c.m.AnimLen, 0)) / 1000}
	c.b.doc.Animations = append(c.b.doc.Animations, anim)
}
