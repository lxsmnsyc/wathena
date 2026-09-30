package mapimport

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
)

// RO map files use units of 1/5 cell, with X east, Y down, and Z north,
// and object positions relative to the map's center. WMAP uses cells with
// X east, Y up, and Z south, from the map's south-west corner. The
// conversion is a scale by 1/5, a shift by the center, and the half turn
// R = diag(1, -1, -1) that models use too.

// unitsPerCell is RO's world units per cell.
const unitsPerCell = 5

// Names maps RO file references to asset names.
type Names struct {
	// Texture maps a path relative to data\texture.
	Texture func(string) string
	// Model maps a path relative to data\model.
	Model func(string) string
	// Sound maps a path relative to data\wav. An empty result leaves the
	// sound out.
	Sound func(string) string
}

// cellFlags maps GAT cell types to WMAP flags. Types not listed block.
var cellFlags = map[uint32]uint8{
	0: wmap.Walkable | wmap.Shootable,
	2: wmap.Walkable | wmap.Shootable,
	3: wmap.Walkable | wmap.Shootable | wmap.Water,
	4: wmap.Walkable | wmap.Shootable,
	5: wmap.Shootable,
	6: wmap.Walkable | wmap.Shootable,
	// Some tools write types with the top bit set.
	0x80000000: wmap.Walkable | wmap.Shootable,
	0x80000002: wmap.Walkable | wmap.Shootable,
	0x80000003: wmap.Walkable | wmap.Shootable | wmap.Water,
	0x80000005: wmap.Walkable | wmap.Shootable,
	0x80000006: wmap.Walkable | wmap.Shootable,
}

// up converts an RO height, Y down in RO units, to cells, up positive.
func up(h float32) float32 { return -h / unitsPerCell }

// defined reports whether no value is NaN or infinite. Objects, sounds,
// and effects only need this: game files use huge values on purpose, such
// as an effect delay of 1e8.
func defined(v ...float32) bool {
	for _, f := range v {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return false
		}
	}
	return true
}

// finite reports whether values are usable as geometry.
func finite(v ...float32) bool {
	for _, f := range v {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) || math.Abs(float64(f)) > 1e7 {
			return false
		}
	}
	return true
}

// EmitterEffect is the RSW effect ID of particle emitters. The nth such
// effect in a map uses the nth entry of the map's emitter table.
const EmitterEffect = 974

// Emitter is a particle emitter from the client's effect tool files, in
// RO units: 1/5 cell, Y down.
type Emitter struct {
	Texture           string
	SrcMode, DestMode int
	Color             [4]float32
	Dir1, Dir2        [3]float32
	Gravity           [3]float32
	Radius            [3]float32
	Life, Rate, Size  [2]float32
	Speed             float32
	MaxCount          int
	ZEnable           bool
	Eternity          bool
	BillboardOff      bool
	Scale             [2]float32
	RotateAngle       [3]float32
}

// Extras are map data from outside the map's own files.
type Extras struct {
	// Emitters are the map's particle emitters by index.
	Emitters map[int]Emitter
	// Music is the background music's asset name, or empty.
	Music string
}

// Convert builds a map and its lightmap atlas. lightmapName is the asset
// name the map uses for the atlas.
func Convert(gat *GAT, gnd *GND, rsw *RSW, names Names, lightmapName string, extras Extras) (*wmap.Map, *image.NRGBA, error) {
	emitters := extras.Emitters
	m := &wmap.Map{Width: gat.Width, Height: gat.Height}
	deps := map[string]bool{}
	dep := func(n string) string {
		deps[n] = true
		return n
	}

	m.Cells = make([]wmap.Cell, len(gat.Cells))
	for i, c := range gat.Cells {
		m.Cells[i] = wmap.Cell{Flags: cellFlags[c.Type]}
		h := fillHeights(c.Heights)
		for k := range 4 {
			m.Cells[i].Heights[k] = up(h[k])
		}
	}

	// Ground textures. GND lists can repeat a name; tiles then use the
	// first.
	g := &m.Ground
	g.Width, g.Height = gnd.Width, gnd.Height
	remap := make([]uint16, len(gnd.Textures))
	seen := map[string]int{}
	for i, t := range gnd.Textures {
		name := names.Texture(t)
		j, ok := seen[name]
		if !ok {
			j = len(g.Textures)
			seen[name] = j
			g.Textures = append(g.Textures, dep(name))
		}
		remap[i] = uint16(j) // #nosec G115 -- at most maxTextures.
	}
	g.Tiles = make([]wmap.Tile, len(gnd.Tiles))
	for i, t := range gnd.Tiles {
		if !finite(t.U[:]...) || !finite(t.V[:]...) {
			return nil, nil, fmt.Errorf("%w: tile coordinates", ErrFormat)
		}
		tex := uint16(0)
		if int(t.Texture) < len(remap) {
			tex = remap[t.Texture]
		}
		light := uint32(t.Lightmap)
		if int(light) >= len(gnd.Lightmaps) {
			light = 0
		}
		g.Tiles[i] = wmap.Tile{U: t.U, V: t.V, Texture: tex, Lightmap: light, Color: [4]uint8{t.Color[2], t.Color[1], t.Color[0], t.Color[3]}}
	}
	if len(g.Textures) == 0 && len(g.Tiles) > 0 {
		// Tiles need a texture to point at. A missing name shows a
		// placeholder.
		g.Textures = append(g.Textures, dep(names.Texture("")))
	}
	tile := func(t int32) int32 {
		if t < 0 || int(t) >= len(g.Tiles) {
			return -1
		}
		return t
	}
	g.Surfaces = make([]wmap.Surface, len(gnd.Surfaces))
	for i, s := range gnd.Surfaces {
		if !finite(s.Heights[:]...) {
			return nil, nil, fmt.Errorf("%w: gnd heights", ErrFormat)
		}
		out := wmap.Surface{Top: tile(s.Up), North: tile(s.Front), East: tile(s.Right)}
		for k := range 4 {
			out.Heights[k] = up(s.Heights[k])
		}
		g.Surfaces[i] = out
	}

	// Lightmaps go into an atlas of 8 × 8 blocks: RGB is the light's
	// color, alpha is the shadow, 255 for none.
	count := max(len(gnd.Lightmaps), 1)
	cols := int(math.Ceil(math.Sqrt(float64(count))))
	rows := (count + cols - 1) / cols
	if cols*8 > 4096 || rows*8 > 4096 {
		return nil, nil, fmt.Errorf("%w: %d lightmaps do not fit a 4096 × 4096 atlas", ErrFormat, len(gnd.Lightmaps))
	}
	atlas := image.NewNRGBA(image.Rect(0, 0, cols*8, rows*8))
	for i, l := range gnd.Lightmaps {
		x0, y0 := i%cols*8, i/cols*8
		for k := range 64 {
			p := atlas.PixOffset(x0+k%8, y0+k/8)
			atlas.Pix[p], atlas.Pix[p+1], atlas.Pix[p+2], atlas.Pix[p+3] = l[64+3*k], l[64+3*k+1], l[64+3*k+2], l[k]
		}
	}
	g.Lightmap, g.LightmapCount, g.LightmapColumns = dep(lightmapName), len(gnd.Lightmaps), cols

	l := rsw.Light
	m.Light = wmap.Light{Longitude: l.Longitude, Latitude: l.Latitude, Diffuse: l.Diffuse, Ambient: l.Ambient, Opacity: l.Opacity, Direction: lightDirection(l)}

	// GND 1.8 and later hold the water. Older maps keep it in the RSW.
	water := gnd.Water
	if water == nil {
		water = rsw.Water
	}
	if water != nil {
		zone := func(level float32, typ int32, waveHeight, waveSpeed, wavePitch float32, anim int32) wmap.Zone {
			return wmap.Zone{Level: up(level), Type: typ, WaveHeight: waveHeight / unitsPerCell, WaveSpeed: waveSpeed, WavePitch: wavePitch, AnimSpeed: anim}
		}
		wa := &wmap.WaterInfo{Zone: zone(water.Level, water.Type, water.WaveHeight, water.WaveSpeed, water.WavePitch, water.AnimSpeed)}
		if len(water.Zones) > 0 {
			wa.SplitWidth, wa.SplitHeight = int(water.SplitWidth), int(water.SplitHeight)
			for _, z := range water.Zones {
				wa.Zones = append(wa.Zones, zone(z.Level, z.Type, z.WaveHeight, z.WaveSpeed, z.WavePitch, z.AnimSpeed))
			}
		}
		if !defined(wa.Level, wa.WaveHeight, wa.WaveSpeed, wa.WavePitch) {
			return nil, nil, fmt.Errorf("%w: water", ErrFormat)
		}
		types := []int32{wa.Type}
		for _, z := range wa.Zones {
			types = append(types, z.Type)
		}
		done := map[int32]bool{}
		for _, t := range types {
			if done[t] || t < 0 || t > 99 {
				continue
			}
			done[t] = true
			wt := wmap.WaterTextures{Type: t}
			for f := range wt.Frames {
				wt.Frames[f] = dep(names.Texture(fmt.Sprintf(`워터\water%d%02d.jpg`, t, f)))
			}
			wa.Textures = append(wa.Textures, wt)
		}
		m.Water = wa
	}

	// Objects and sounds are relative to the map's center, which is the
	// ground's width and height in tiles, since a tile is 2 cells.
	cx, cz := float64(gnd.Width), float64(gnd.Height)
	// An object, sound, or effect with undefined values is left out, since
	// the game cannot show it either.
	for _, o := range rsw.Models {
		if !defined(o.Pos[:]...) || !defined(o.Rot[:]...) || !defined(o.Scale[:]...) {
			continue
		}
		m.Objects = append(m.Objects, wmap.Object{
			Model:     dep(names.Model(o.File)),
			Matrix:    placement(o, cx, cz),
			AnimType:  o.AnimType,
			AnimSpeed: o.AnimSpeed,
			BlockType: o.BlockType,
		})
	}
	for _, s := range rsw.Sounds {
		name := names.Sound(s.File)
		if name == "" || !defined(s.Pos[:]...) || !defined(s.Volume, s.Range, s.Cycle) {
			continue
		}
		cycle := s.Cycle
		if cycle == 0 {
			// RO plays a sound without a cycle every 7 seconds.
			cycle = 7
		}
		m.Sounds = append(m.Sounds, wmap.Sound{
			Name:   dep(name),
			Pos:    mapPoint(s.Pos, cx, cz),
			Volume: s.Volume,
			Width:  float32(s.Width) / unitsPerCell,
			Height: float32(s.Height) / unitsPerCell,
			Range:  s.Range / unitsPerCell,
			Cycle:  cycle,
		})
	}
	emitterIndex := 0
	for _, e := range rsw.Effects {
		var em Emitter
		hasEmitter := false
		if e.ID == EmitterEffect {
			// Count every emitter effect, even ones left out below, so the
			// rest keep their table entries.
			em, hasEmitter = emitters[emitterIndex]
			emitterIndex++
		}
		if !defined(e.Pos[:]...) || !defined(e.Delay) || !defined(e.Param[:]...) {
			continue
		}
		if hasEmitter {
			if w, ok := convertEmitter(em, mapPoint(e.Pos, cx, cz), names); ok {
				dep(w.Texture)
				m.Emitters = append(m.Emitters, w)
			}
		}
		m.Effects = append(m.Effects, wmap.Effect{ID: e.ID, Pos: mapPoint(e.Pos, cx, cz), Delay: e.Delay, Param: e.Param})
	}

	if extras.Music != "" {
		m.Music = dep(extras.Music)
	}
	for n := range deps {
		m.Deps = append(m.Deps, n)
	}
	sort.Strings(m.Deps)
	if err := m.Validate(); err != nil {
		return nil, nil, err
	}
	return m, atlas, nil
}

// lightDirection returns the unit vector toward the light in map space.
// RO turns (0, 1, 0) by X at the latitude, then by Y at the longitude,
// and lights along the opposite of the result, in its Y-down space. The
// half turn R then brings it to map space.
func lightDirection(l Light) [3]float32 {
	lon := float64(l.Longitude) * math.Pi / 180
	lat := float64(l.Latitude) * math.Pi / 180
	sLat, cLat := math.Sin(lat), math.Cos(lat)
	return [3]float32{
		float32(-float64(sLat * math.Sin(lon))),
		float32(cLat),
		float32(float64(sLat * math.Cos(lon))),
	}
}

// fillHeights replaces unusable corner heights, which a few game files
// have on blocked cells, with the average of the usable ones, or 0.
func fillHeights(h [4]float32) [4]float32 {
	var sum float64
	n := 0
	for _, v := range h {
		if finite(v) {
			sum += float64(v)
			n++
		}
	}
	for k, v := range h {
		if !finite(v) {
			h[k] = 0
			if n > 0 {
				h[k] = float32(sum / float64(n))
			}
		}
	}
	return h
}

// mapPoint converts a position relative to the map's center to map space.
func mapPoint(p [3]float32, cx, cz float64) [3]float32 {
	return [3]float32{
		float32(float64(p[0])/unitsPerCell + cx),
		float32(-float64(p[1]) / unitsPerCell),
		float32(-(float64(p[2])/unitsPerCell + cz)),
	}
}

// placement returns an object's matrix in map space: R × M × R, where M
// is RO's T(position / 5 + center) × Rz × Rx × Ry × S(scale / 5).
func placement(o Object, cx, cz float64) [16]float32 {
	rad := func(d float32) float64 { return float64(d) * math.Pi / 180 }
	m := translate(float64(o.Pos[0])/unitsPerCell+cx, float64(o.Pos[1])/unitsPerCell, float64(o.Pos[2])/unitsPerCell+cz).
		mul(rotZ(rad(o.Rot[2]))).
		mul(rotX(rad(o.Rot[0]))).
		mul(rotY(rad(o.Rot[1]))).
		mul(scale(float64(o.Scale[0])/unitsPerCell, float64(o.Scale[1])/unitsPerCell, float64(o.Scale[2])/unitsPerCell))
	turn := scale(1, -1, -1)
	m = turn.mul(m).mul(turn)
	var out [16]float32
	for i, v := range m {
		out[i] = float32(v)
	}
	return out
}

// mat4 is a column-major 4 × 4 matrix. Products convert each step to
// float64, so Go cannot fuse multiply-adds, which would change results
// between platforms.
type mat4 [16]float64

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

func translate(x, y, z float64) mat4 { return mat4{0: 1, 5: 1, 10: 1, 12: x, 13: y, 14: z, 15: 1} }
func scale(x, y, z float64) mat4     { return mat4{0: x, 5: y, 10: z, 15: 1} }

func rotX(a float64) mat4 {
	c, s := math.Cos(a), math.Sin(a)
	return mat4{0: 1, 5: c, 6: s, 9: -s, 10: c, 15: 1}
}

func rotY(a float64) mat4 {
	c, s := math.Cos(a), math.Sin(a)
	return mat4{0: c, 2: -s, 5: 1, 8: s, 10: c, 15: 1}
}

func rotZ(a float64) mat4 {
	c, s := math.Cos(a), math.Sin(a)
	return mat4{0: c, 1: s, 4: -s, 5: c, 10: 1, 15: 1}
}

// convertEmitter converts an emitter to map space: cells, Y up, and north
// toward -Z. Emitters with undefined values or no texture are left out.
func convertEmitter(e Emitter, pos [3]float32, names Names) (wmap.Emitter, bool) {
	vals := []float32{e.Speed}
	for _, v := range [][]float32{e.Color[:], e.Dir1[:], e.Dir2[:], e.Gravity[:], e.Radius[:], e.Life[:], e.Rate[:], e.Size[:], e.Scale[:], e.RotateAngle[:]} {
		vals = append(vals, v...)
	}
	if !finite(vals...) || e.Texture == "" {
		return wmap.Emitter{}, false
	}
	// RO's Y points down and its Z north, the opposite of map space.
	vec := func(v [3]float32) [3]float32 {
		return [3]float32{v[0] / unitsPerCell, -v[1] / unitsPerCell, -v[2] / unitsPerCell}
	}
	abs := func(v [3]float32) [3]float32 {
		for i := range v {
			v[i] = float32(math.Abs(float64(v[i])))
		}
		return v
	}
	u8 := func(f float32) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(float64(f))))) }
	blend := func(mode int) uint8 {
		if mode < 1 || mode > 15 {
			return 2
		}
		return uint8(mode) // #nosec G115 -- 1 to 15.
	}
	w := wmap.Emitter{
		Pos:      pos,
		Texture:  names.Texture(e.Texture),
		SrcBlend: blend(e.SrcMode),
		DstBlend: blend(e.DestMode),
		Color:    [4]uint8{u8(e.Color[0]), u8(e.Color[1]), u8(e.Color[2]), u8(e.Color[3])},
		Dir1:     vec(e.Dir1),
		Dir2:     vec(e.Dir2),
		Gravity:  vec(e.Gravity),
		Radius:   abs(vec(e.Radius)),
		Life:     e.Life,
		Rate:     e.Rate,
		Size:     [2]float32{e.Size[0] / unitsPerCell, e.Size[1] / unitsPerCell},
		Speed:    e.Speed,
		MaxCount: uint16(min(max(e.MaxCount, 0), wmap.MaxParticles)), // #nosec G115 -- clamped.
		Scale:    e.Scale,
		Rotate:   e.RotateAngle,
	}
	if e.ZEnable {
		w.Flags |= wmap.DepthTest
	}
	if e.Eternity {
		w.Flags |= wmap.Eternal
	}
	if e.BillboardOff {
		w.Flags |= wmap.Flat
	}
	return w, true
}
