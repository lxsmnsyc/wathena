package model

import (
	"fmt"
	"sort"
)

// This file converts RSM2 models, versions 2.2 and 2.3.
//
// RSM2 nodes store their 3 × 3 matrix and position in model space. A
// node's transform relative to its parent is
//
//	T(position) × M × S
//
// where, at each moment:
//
//   - position is the position keyframe, or else the parent's inverse
//     matrix times (position - parent position);
//   - M is the rotation keyframe, or else the parent's inverse matrix times
//     the node's matrix;
//   - S is the scale keyframe, or else none.
//
// Before a node's first keyframe, rotation starts from none, scale from 1,
// and position from the stored position. After the last keyframe the last
// value holds. Keyframe times are frames at the model's frame rate.
//
// Model space has X east, Y up, and Z north, which is left-handed, and
// the model is not centered. glTF's Y-up space has Z south, so everything
// is mirrored through Z by Q = diag(1, 1, -1). This follows BrowEdit3's
// reading of the format.

// mirrorZ is Q = diag(1, 1, -1).
var mirrorZ = scaling([3]float64{1, 1, -1})

func (c *converter) mat3(n *Node) mat4 {
	m := mat4{15: 1}
	for col := range 3 {
		for row := range 3 {
			m[col*4+row] = float64(n.Mat3[col*3+row])
		}
	}
	return m
}

// inverse3 inverts the linear part of a matrix. A singular matrix gives
// the identity, which is what a degenerate node can be drawn with.
func inverse3(a mat4) mat4 {
	det := a.det3()
	if det == 0 || !finite(det) {
		return identity()
	}
	var out mat4
	out[15] = 1
	// The adjugate, transposed, divided by the determinant.
	at := func(c, r int) float64 { return a[c*4+r] }
	for c := range 3 {
		for r := range 3 {
			c1, c2 := (r+1)%3, (r+2)%3
			r1, r2 := (c+1)%3, (c+2)%3
			out[c*4+r] = float64(float64(at(c1, r1)*at(c2, r2))-float64(at(c1, r2)*at(c2, r1))) / det
		}
	}
	return out
}

// mirrorQuatZ conjugates a rotation by Q: the axis mirrors through Z and
// the angle changes sign, which negates x and y.
func mirrorQuatZ(q [4]float64) [4]float32 {
	r := turnQuat(q) // x, -y, -z, w
	return [4]float32{-r[0], r[1], -r[2], r[3]}
}

// v2State is a node's transform at frame 0.
type v2State struct {
	pos   [3]float64
	m     mat4
	rot   [4]float64
	scale [3]float64
}

func (c *converter) v2Frame0(i, parent int) v2State {
	n := c.m.Nodes[i]
	st := v2State{rot: [4]float64{0, 0, 0, 1}, scale: [3]float64{1, 1, 1}}
	inv := identity()
	if parent >= 0 {
		inv = inverse3(c.mat3(c.m.Nodes[parent]))
	}
	switch {
	case len(n.PosKeys) > 0:
		st.pos = vec(n.Offset)
		if k := firstKey(n.PosKeys, func(k PosKey) int32 { return k.Frame }); k >= 0 {
			st.pos = vec(n.PosKeys[k].Pos)
		}
	case parent >= 0:
		st.pos = inv.point(sub(vec(n.Offset), vec(c.m.Nodes[parent].Offset)))
	default:
		st.pos = vec(n.Offset)
	}
	if len(n.RotKeys) > 0 {
		if k := firstKey(n.RotKeys, func(k RotKey) int32 { return k.Frame }); k >= 0 {
			q := n.RotKeys[k].Q
			st.rot = [4]float64{float64(q[0]), float64(q[1]), float64(q[2]), float64(q[3])}
		}
		st.m = quatMatrix(st.rot)
	} else {
		st.m = inv.mul(c.mat3(n))
	}
	if len(n.ScaleKey) > 0 {
		if k := firstKey(n.ScaleKey, func(k ScaleKey) int32 { return k.Frame }); k >= 0 {
			st.scale = vec(n.ScaleKey[k].Scale)
		}
	}
	return st
}

// firstKey returns the index of the keyframe at frame 0, or -1 when the
// first keyframe is later.
func firstKey[K any](keys []K, frame func(K) int32) int {
	best := -1
	for i, k := range keys {
		if frame(k) <= 0 && (best < 0 || frame(k) > frame(keys[best])) {
			best = i
		}
	}
	return best
}

// linear returns the linear part of a matrix, without translation.
func linear(a mat4) mat4 {
	a[12], a[13], a[14] = 0, 0, 0
	return a
}

func (c *converter) runV2() ([]byte, error) {
	m := c.m
	// A node's parent is the first node with its parent's name.
	parentOf := make([]int, len(m.Nodes))
	for i, n := range m.Nodes {
		parentOf[i] = -1
		for j, p := range m.Nodes {
			if j != i && p.Name == n.Parent {
				parentOf[i] = j
				break
			}
		}
	}
	// Walk from the roots. Nodes in a loop hang from the root.
	var order []int
	placed := make([]bool, len(m.Nodes))
	for i := range m.Nodes {
		if parentOf[i] < 0 {
			order = append(order, i)
			placed[i] = true
		}
	}
	for k := 0; k < len(order); k++ {
		for i := range m.Nodes {
			if !placed[i] && parentOf[i] == order[k] {
				order = append(order, i)
				placed[i] = true
			}
		}
	}
	for i := range m.Nodes {
		if !placed[i] {
			parentOf[i] = -1
			order = append(order, i)
		}
	}

	c.b.doc.Asset = gltfAsset{Version: "2.0", Generator: "wathena"}
	c.b.doc.Nodes = append(c.b.doc.Nodes, gltfNode{Name: "root"})
	c.gltfNode = map[int]int{}
	c.materials = map[[2]int]int{}
	c.textures = map[int]int{}

	children := make([][]int, len(m.Nodes))
	var roots []int
	for _, i := range order {
		if p := parentOf[i]; p >= 0 {
			children[p] = append(children[p], i)
		} else {
			roots = append(roots, i)
		}
	}
	animated := func(n *Node) bool { return len(n.RotKeys)+len(n.PosKeys)+len(n.ScaleKey) > 0 }
	// animatedBelow reports whether a node or any node under it moves.
	below := make([]bool, len(m.Nodes))
	for k := len(order) - 1; k >= 0; k-- {
		i := order[k]
		below[i] = below[i] || animated(m.Nodes[i])
		if p := parentOf[i]; p >= 0 && below[i] {
			below[p] = true
		}
	}

	// Animation targets: the glTF node that holds each moving RSM node's
	// translation, rotation, and scale.
	type targets struct{ translation, rotation, scale int }
	targetOf := map[int]targets{}
	add := func(gn gltfNode, to int) int {
		c.b.doc.Nodes = append(c.b.doc.Nodes, gn)
		idx := len(c.b.doc.Nodes) - 1
		c.b.doc.Nodes[to].Children = append(c.b.doc.Nodes[to].Children, idx)
		return idx
	}
	matrixNode := func(name string, a mat4, to int) int {
		mm := mirrorZ.mul(a).mul(mirrorZ)
		var mf, id [16]float32
		for k, v := range mm {
			mf[k] = float32(v)
		}
		for k, v := range identity() {
			id[k] = float32(v)
		}
		if mf == id {
			return to
		}
		return add(gltfNode{Name: name, Matrix: &mf}, to)
	}
	checked := func(st v2State) error {
		if !finite(st.pos[0], st.pos[1], st.pos[2], st.scale[0], st.scale[1], st.scale[2]) {
			return fmt.Errorf("%w: node transform is out of range", ErrFormat)
		}
		for _, v := range st.m {
			if !finite(v) {
				return fmt.Errorf("%w: node matrix is out of range", ErrFormat)
			}
		}
		return nil
	}

	// place adds node i under the glTF node attach. bake is the static
	// transform from attach's frame to i's parent's frame, and world is
	// the parent's world matrix at frame 0.
	//
	// A node whose subtree does not move has its whole transform baked
	// into its vertices, so no glTF node needs its matrix. This matters
	// because an RSM2 matrix can shear, which glTF node matrices cannot.
	var place func(i, attach int, bake, world mat4) error
	place = func(i, attach int, bake, world mat4) error {
		n := c.m.Nodes[i]
		st := c.v2Frame0(i, parentOf[i])
		if err := checked(st); err != nil {
			return err
		}
		local := translation(st.pos).mul(st.m).mul(scaling(st.scale))
		nodeWorld := world.mul(local)
		global := mirrorZ.mul(linear(nodeWorld)).mul(mirrorZ)
		if !animated(n) {
			full := bake.mul(local)
			holder := attach
			if below[i] {
				// A moving node under this one needs this node's frame as
				// a glTF node.
				holder = matrixNode(n.Name, full, attach)
				full = identity()
			}
			mesh, err := c.addMesh(n, mirrorZ.mul(full), global)
			if err != nil {
				return err
			}
			if mesh >= 0 {
				holder = add(gltfNode{Name: n.Name, Mesh: &mesh}, holder)
			}
			c.gltfNode[i] = holder
			for _, ch := range children[i] {
				if err := place(ch, holder, full, nodeWorld); err != nil {
					return err
				}
			}
			return nil
		}
		// A moving node: its static prefix, if any, then T × R × S, or
		// T × M × S when only position or scale moves.
		parentNode := matrixNode(n.Name+"#frame", bake, attach)
		outer := gltfNode{Name: n.Name, Translation: f32p3(mirrorZ.point(st.pos))}
		hasRot := len(n.RotKeys) > 0
		if hasRot {
			r := mirrorQuatZ(st.rot)
			outer.Rotation = &r
			outer.Scale = f32p3(st.scale)
		}
		last := add(outer, parentNode)
		t := targets{translation: last, rotation: last, scale: last}
		if !hasRot {
			last = matrixNode(n.Name+"#matrix", st.m, last)
			if len(n.ScaleKey) > 0 {
				last = add(gltfNode{Name: n.Name + "#scale", Scale: f32p3(st.scale)}, last)
			}
			t.scale = last
		}
		targetOf[i] = t
		mesh, err := c.addMesh(n, mirrorZ, global)
		if err != nil {
			return err
		}
		if mesh >= 0 {
			c.b.doc.Nodes[last].Mesh = &mesh
		}
		c.gltfNode[i] = last
		for _, ch := range children[i] {
			if err := place(ch, last, identity(), nodeWorld); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range roots {
		if err := place(r, 0, identity(), identity()); err != nil {
			return nil, err
		}
	}

	// Animations. Times are frames at the model's frame rate.
	fps := float64(m.FrameRate)
	if fps <= 0 || !finite(fps) {
		fps = 30
	}
	var anim gltfAnimation
	channel := func(node int, path string, times []float32, values []float32, n int) {
		typ := map[int]string{3: "VEC3", 4: "VEC4"}[n]
		in := c.b.floats(times, 1, "SCALAR", 0, true)
		out := c.b.floats(values, n, typ, 0, false)
		anim.Samplers = append(anim.Samplers, gltfAnimSample{Input: in, Output: out, Interpolation: "LINEAR"})
		anim.Channels = append(anim.Channels, gltfChannel{Sampler: len(anim.Samplers) - 1, Target: gltfTarget{Node: node, Path: path}})
	}
	for _, i := range order {
		n := c.m.Nodes[i]
		t := targetOf[i]
		if len(n.PosKeys) > 0 {
			keys := make([]keyframe, 0, len(n.PosKeys)+1)
			for _, k := range n.PosKeys {
				p := mirrorZ.point(vec(k.Pos))
				keys = append(keys, keyframe{k.Frame, []float64{p[0], p[1], p[2]}})
			}
			p := mirrorZ.point(vec(n.Offset))
			times, values := sampleKeys(keys, []float64{p[0], p[1], p[2]}, fps)
			if times != nil {
				channel(t.translation, "translation", times, values, 3)
			}
		}
		if len(n.RotKeys) > 0 {
			keys := make([]keyframe, 0, len(n.RotKeys)+1)
			for _, k := range n.RotKeys {
				q := mirrorQuatZ([4]float64{float64(k.Q[0]), float64(k.Q[1]), float64(k.Q[2]), float64(k.Q[3])})
				keys = append(keys, keyframe{k.Frame, []float64{float64(q[0]), float64(q[1]), float64(q[2]), float64(q[3])}})
			}
			times, values := sampleKeys(keys, []float64{0, 0, 0, 1}, fps)
			if times != nil {
				channel(t.rotation, "rotation", times, values, 4)
			}
		}
		if len(n.ScaleKey) > 0 {
			keys := make([]keyframe, 0, len(n.ScaleKey)+1)
			for _, k := range n.ScaleKey {
				s := vec(k.Scale)
				keys = append(keys, keyframe{k.Frame, []float64{s[0], s[1], s[2]}})
			}
			times, values := sampleKeys(keys, []float64{1, 1, 1}, fps)
			if times != nil {
				channel(t.scale, "scale", times, values, 3)
			}
		}
	}
	if len(anim.Channels) > 0 {
		anim.Extras = map[string]any{"loopSeconds": float64(max(m.AnimLen, 0)) / fps}
		c.b.doc.Animations = append(c.b.doc.Animations, anim)
	}

	c.b.doc.Scenes = []gltfScene{{Nodes: []int{0}}}
	out, err := c.b.glb()
	if err != nil {
		return nil, err
	}
	if len(out) > MaxBytes {
		return nil, fmt.Errorf("model: %d bytes is over the size limit", len(out))
	}
	return out, nil
}

type keyframe struct {
	frame int32
	value []float64
}

// sampleKeys returns glTF animation times and values for keyframes. When
// the first keyframe is after frame 0, the animation starts from start at
// frame 0. Repeated and negative frames are dropped. It returns nil for
// an animation of one moment, which the node's static value covers.
func sampleKeys(keys []keyframe, start []float64, fps float64) ([]float32, []float32) {
	sort.SliceStable(keys, func(a, b int) bool { return keys[a].frame < keys[b].frame })
	var times, values []float32
	push := func(frame int32, v []float64) {
		times = append(times, float32(float64(frame)/fps))
		for _, x := range v {
			values = append(values, float32(x))
		}
	}
	for k, key := range keys {
		if key.frame < 0 || (k > 0 && key.frame == keys[k-1].frame) {
			continue
		}
		if len(times) == 0 && key.frame > 0 {
			push(0, start)
		}
		ok := true
		for _, x := range key.value {
			ok = ok && finite(x)
		}
		if !ok {
			continue
		}
		push(key.frame, key.value)
	}
	if len(times) < 2 {
		return nil, nil
	}
	return times, values
}
