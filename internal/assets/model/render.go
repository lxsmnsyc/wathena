package model

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
)

// This file draws a GLB file from this package, for previews and tests. It
// is a small software renderer, not the client's. It supports what the
// converter writes: node transforms, indexed triangles with positions,
// normals, and texture coordinates, textures, alpha masks, and back-face
// culling for single-sided materials.

// Texture looks up an image by asset name. It returns nil when the
// texture is missing, which draws grey.
type Texture func(name string) *image.NRGBA

type glbFile struct {
	doc gltfDoc
	bin []byte
}

func readGLB(b []byte) (*glbFile, error) {
	if len(b) < 20 || string(b[:4]) != "glTF" {
		return nil, fmt.Errorf("model: not a glb file")
	}
	n := int(binary.LittleEndian.Uint32(b[12:]))
	if string(b[16:20]) != "JSON" || 20+n > len(b) {
		return nil, fmt.Errorf("model: glb json chunk")
	}
	g := &glbFile{}
	if err := json.Unmarshal(b[20:20+n], &g.doc); err != nil {
		return nil, err
	}
	rest := b[20+n:]
	if len(rest) >= 8 && string(rest[4:8]) == "BIN\x00" {
		m := int(binary.LittleEndian.Uint32(rest))
		if 8+m <= len(rest) {
			g.bin = rest[8 : 8+m]
		}
	}
	return g, nil
}

// floats reads a float accessor.
func (g *glbFile) floats(i int) ([]float64, error) {
	a := g.doc.Accessors[i]
	v := g.doc.BufferViews[a.BufferView]
	comps := map[string]int{"SCALAR": 1, "VEC2": 2, "VEC3": 3, "VEC4": 4}[a.Type]
	if a.ComponentType != glFloat || v.ByteOffset+a.Count*comps*4 > len(g.bin) {
		return nil, fmt.Errorf("model: accessor %d", i)
	}
	out := make([]float64, a.Count*comps)
	for k := range out {
		out[k] = float64(math.Float32frombits(binary.LittleEndian.Uint32(g.bin[v.ByteOffset+4*k:])))
	}
	return out, nil
}

func (g *glbFile) indices(i int) ([]int, error) {
	a := g.doc.Accessors[i]
	v := g.doc.BufferViews[a.BufferView]
	size := 2
	if a.ComponentType == glUnsignedInt {
		size = 4
	}
	if v.ByteOffset+a.Count*size > len(g.bin) {
		return nil, fmt.Errorf("model: accessor %d", i)
	}
	out := make([]int, a.Count)
	for k := range out {
		if size == 2 {
			out[k] = int(binary.LittleEndian.Uint16(g.bin[v.ByteOffset+2*k:]))
		} else {
			out[k] = int(binary.LittleEndian.Uint32(g.bin[v.ByteOffset+4*k:]))
		}
	}
	return out, nil
}

func trs(n gltfNode) mat4 {
	m := identity()
	if n.Translation != nil {
		m = m.mul(translation([3]float64{float64(n.Translation[0]), float64(n.Translation[1]), float64(n.Translation[2])}))
	}
	if n.Rotation != nil {
		r := n.Rotation
		m = m.mul(quatMatrix([4]float64{float64(r[0]), float64(r[1]), float64(r[2]), float64(r[3])}))
	}
	if n.Scale != nil {
		m = m.mul(scaling([3]float64{float64(n.Scale[0]), float64(n.Scale[1]), float64(n.Scale[2])}))
	}
	return m
}

type rtri struct {
	p        [3][3]float64
	n        [3][3]float64
	uv       [3][2]float64
	tex      *image.NRGBA
	cull     bool
	mirrored bool
}

// collect returns the triangles of a GLB file placed by parent.
func collect(glb []byte, parent mat4, textures Texture) ([]rtri, error) {
	g, err := readGLB(glb)
	if err != nil {
		return nil, err
	}
	var tris []rtri
	var walk func(n int, parent mat4, depth int) error
	walk = func(n int, parent mat4, depth int) error {
		if n < 0 || n >= len(g.doc.Nodes) || depth > 64 {
			return fmt.Errorf("model: bad node %d", n)
		}
		node := g.doc.Nodes[n]
		m := parent.mul(trs(node))
		if node.Mesh != nil {
			mirrored := m.det3() < 0
			for _, p := range g.doc.Meshes[*node.Mesh].Primitives {
				pos, err := g.floats(p.Attributes["POSITION"])
				if err != nil {
					return err
				}
				nrm, err := g.floats(p.Attributes["NORMAL"])
				if err != nil {
					return err
				}
				uv, err := g.floats(p.Attributes["TEXCOORD_0"])
				if err != nil {
					return err
				}
				idx, err := g.indices(p.Indices)
				if err != nil {
					return err
				}
				mat := g.doc.Materials[p.Material]
				var tex *image.NRGBA
				if bt := mat.PBR.BaseColorTexture; bt != nil && textures != nil {
					uri := g.doc.Images[g.doc.Textures[bt.Index].Source].URI
					tex = textures(strings.TrimPrefix(unescapeURI(uri), "asset:"))
				}
				origin := m.point([3]float64{})
				for k := 0; k+2 < len(idx); k += 3 {
					t := rtri{tex: tex, cull: !mat.DoubleSided, mirrored: mirrored}
					for c := range 3 {
						v := idx[k+c]
						t.p[c] = m.point([3]float64{pos[3*v], pos[3*v+1], pos[3*v+2]})
						t.n[c], _ = normalize(sub(m.point([3]float64{nrm[3*v], nrm[3*v+1], nrm[3*v+2]}), origin))
						t.uv[c] = [2]float64{uv[2*v], uv[2*v+1]}
					}
					tris = append(tris, t)
				}
			}
		}
		for _, ch := range node.Children {
			if err := walk(ch, m, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, n := range g.doc.Scenes[g.doc.Scene].Nodes {
		if err := walk(n, parent, 0); err != nil {
			return nil, err
		}
	}
	return tris, nil
}

// Render draws a GLB file from a fixed camera above and to the south-east,
// fitted to size × size pixels.
func Render(glb []byte, size int, textures Texture) (*image.NRGBA, error) {
	tris, err := collect(glb, identity(), textures)
	if err != nil {
		return nil, err
	}
	// The camera looks down from the south-east: turn 45° around Y, then
	// tilt 35° down. Screen X is right, screen Y is down, depth is away.
	cy, sy := math.Cos(math.Pi/4), math.Sin(math.Pi/4)
	cx, sx := math.Cos(35*math.Pi/180), math.Sin(35*math.Pi/180)
	view := func(p [3]float64) [3]float64 {
		x := p[0]*cy - p[2]*sy
		z := p[0]*sy + p[2]*cy
		y := p[1]*cx - z*sx
		d := p[1]*sx + z*cx
		return [3]float64{x, -y, -d}
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	rasterize(img, tris, view, true)
	return img, nil
}

// Instance is a model placed by a column-major matrix.
type Instance struct {
	GLB    []byte
	Matrix [16]float32
}

// RenderTopDown draws models straight down onto img, over what it already
// holds, such as a ground image. One pixel is 1 / pixelsPerUnit units,
// with north (-Z) up and X = 0, Z = 0 at the bottom-left corner.
func RenderTopDown(img *image.NRGBA, instances []Instance, pixelsPerUnit float64, textures Texture) error {
	var tris []rtri
	for _, in := range instances {
		var m mat4
		for i, v := range in.Matrix {
			m[i] = float64(v)
		}
		t, err := collect(in.GLB, m, textures)
		if err != nil {
			return err
		}
		tris = append(tris, t...)
	}
	h := float64(img.Rect.Dy())
	// Screen Y grows toward the south, +Z. Depth is -Y: higher is nearer.
	view := func(p [3]float64) [3]float64 {
		return [3]float64{p[0] * pixelsPerUnit, h + p[2]*pixelsPerUnit, -p[1]}
	}
	rasterize(img, tris, view, false)
	return nil
}

func unescapeURI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			var v byte
			if _, err := fmt.Sscanf(s[i+1:i+3], "%02x", &v); err == nil {
				b.WriteByte(v)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// rasterize draws triangles through view, which maps a point to screen
// X, screen Y, and depth. With fit, the view is scaled to fill img.
func rasterize(img *image.NRGBA, tris []rtri, view func([3]float64) [3]float64, fit bool) {
	size := img.Rect.Dx()
	vt := make([][3][3]float64, len(tris))
	lo := [2]float64{math.Inf(1), math.Inf(1)}
	hi := [2]float64{math.Inf(-1), math.Inf(-1)}
	for i, t := range tris {
		for c := range 3 {
			vt[i][c] = view(t.p[c])
			lo[0], lo[1] = min(lo[0], vt[i][c][0]), min(lo[1], vt[i][c][1])
			hi[0], hi[1] = max(hi[0], vt[i][c][0]), max(hi[1], vt[i][c][1])
		}
	}
	scale, ox, oy := 1.0, 0.0, 0.0
	if fit {
		span := max(hi[0]-lo[0], hi[1]-lo[1])
		if span <= 0 || math.IsInf(span, 0) {
			return
		}
		scale = float64(size) * 0.9 / span
		ox = float64(size)/2 - (lo[0]+hi[0])/2*scale
		oy = float64(img.Rect.Dy())/2 - (lo[1]+hi[1])/2*scale
	}
	w, hgt := img.Rect.Dx(), img.Rect.Dy()
	depth := make([]float64, w*hgt)
	for i := range depth {
		depth[i] = math.Inf(1)
	}
	light, _ := normalize([3]float64{-0.4, 1, 0.6})
	for i, t := range tris {
		var sp [3][3]float64
		for c := range 3 {
			sp[c] = [3]float64{vt[i][c][0]*scale + ox, vt[i][c][1]*scale + oy, vt[i][c][2]}
		}
		area := (sp[1][0]-sp[0][0])*(sp[2][1]-sp[0][1]) - (sp[2][0]-sp[0][0])*(sp[1][1]-sp[0][1])
		if area == 0 {
			continue
		}
		// With screen Y down, a front face (counter-clockwise in glTF) has
		// a negative area. A mirrored transform swaps that.
		front := area < 0
		if t.mirrored {
			front = !front
		}
		if t.cull && !front {
			continue
		}
		x0 := max(0, int(math.Floor(min(sp[0][0], sp[1][0], sp[2][0]))))
		x1 := min(w-1, int(math.Ceil(max(sp[0][0], sp[1][0], sp[2][0]))))
		y0 := max(0, int(math.Floor(min(sp[0][1], sp[1][1], sp[2][1]))))
		y1 := min(hgt-1, int(math.Ceil(max(sp[0][1], sp[1][1], sp[2][1]))))
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				px, py := float64(x)+0.5, float64(y)+0.5
				w0 := ((sp[1][0]-px)*(sp[2][1]-py) - (sp[2][0]-px)*(sp[1][1]-py)) / area
				w1 := ((sp[2][0]-px)*(sp[0][1]-py) - (sp[0][0]-px)*(sp[2][1]-py)) / area
				w2 := 1 - w0 - w1
				if w0 < 0 || w1 < 0 || w2 < 0 {
					continue
				}
				d := w0*sp[0][2] + w1*sp[1][2] + w2*sp[2][2]
				if d >= depth[y*w+x] {
					continue
				}
				col := [4]float64{160, 160, 160, 255}
				if t.tex != nil {
					u := w0*t.uv[0][0] + w1*t.uv[1][0] + w2*t.uv[2][0]
					v := w0*t.uv[0][1] + w1*t.uv[1][1] + w2*t.uv[2][1]
					b := t.tex.Rect
					tx := b.Min.X + int(math.Floor((u-math.Floor(u))*float64(b.Dx())))
					ty := b.Min.Y + int(math.Floor((v-math.Floor(v))*float64(b.Dy())))
					c := t.tex.NRGBAAt(tx, ty)
					col = [4]float64{float64(c.R), float64(c.G), float64(c.B), float64(c.A)}
				}
				if col[3] < 128 {
					continue
				}
				var n [3]float64
				for c := range 3 {
					n[c] = w0*t.n[0][c] + w1*t.n[1][c] + w2*t.n[2][c]
				}
				n, _ = normalize(n)
				if !front {
					n = [3]float64{-n[0], -n[1], -n[2]}
				}
				l := 0.45 + 0.55*max(0, n[0]*light[0]+n[1]*light[1]+n[2]*light[2])
				depth[y*w+x] = d
				img.SetNRGBA(x, y, color.NRGBA{uint8(min(255, col[0]*l)), uint8(min(255, col[1]*l)), uint8(min(255, col[2]*l)), 255})
			}
		}
	}
}
