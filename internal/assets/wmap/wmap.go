// Package wmap reads and writes WMAP maps, as defined in
// docs/specs/assets-v1.md, "Map (WMAP)".
//
// Map space is in cells: X points east, Y up, and -Z north. Cell (x, y)
// covers X from x to x + 1 and Z from -(y + 1) to -y, so server
// coordinates map directly to cells.
package wmap

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	magic   = "WMAP"
	version = 1
	// MaxCells is the largest width or height.
	MaxCells = 1024
	// MaxBytes is the largest encoded map.
	MaxBytes = 64 << 20
)

var ErrFormat = errors.New("wmap: invalid map")

// Cell flags.
const (
	Walkable  = 1
	Shootable = 2
	Water     = 4
)

// Map is a decoded map.
type Map struct {
	Width, Height int
	// Cells are row by row from y = 0, the south edge.
	Cells   []Cell
	Ground  Ground
	Light   Light
	Water   *WaterInfo
	Objects []Object
	Sounds  []Sound
	Effects []Effect
	// Emitters are particle emitters, from the client's effect tool files.
	Emitters []Emitter
	// Music is the background music's asset name, or empty.
	Music string
	// Deps lists every asset name the map uses.
	Deps []string
}

// Cell is one cell. Heights are for the corners (x, y), (x+1, y),
// (x, y+1), and (x+1, y+1), in cells, up positive.
type Cell struct {
	Flags   uint8
	Heights [4]float32
}

// Ground is the ground mesh as a grid of surfaces, each 2 × 2 cells.
type Ground struct {
	Width, Height int
	// Textures are asset names.
	Textures []string
	Tiles    []Tile
	Surfaces []Surface
	// Lightmap is the lightmap atlas's asset name. Block i of 8 × 8
	// texels starts at (i % Columns × 8, i / Columns × 8).
	Lightmap        string
	LightmapCount   int
	LightmapColumns int
}

// Tile is a textured quad. U and V are for the corners in the order of
// Surface.Heights. Color is R, G, B, A.
type Tile struct {
	U, V     [4]float32
	Texture  uint16
	Lightmap uint32
	Color    [4]uint8
}

// Surface is one ground square. Heights are for its corners in cells, up
// positive. Top, North, and East are tile indexes, or -1: the top, the
// wall on the north edge, and the wall on the east edge.
type Surface struct {
	Heights          [4]float32
	Top, North, East int32
}

// Light is the directional light. Angles are degrees. Direction is the
// unit vector toward the light in map space, computed from the angles.
type Light struct {
	Longitude, Latitude int32
	Diffuse, Ambient    [3]float32
	Opacity             float32
	Direction           [3]float32
}

// WaterInfo is the map's water. Heights are in cells, up positive.
type WaterInfo struct {
	Zone
	SplitWidth, SplitHeight int
	// Zones override Zone per part of the map, row by row, when the map is
	// split.
	Zones []Zone
	// Textures lists the 32 animation frames of each water type used.
	Textures []WaterTextures
}

// Zone describes water in one area.
type Zone struct {
	Level      float32
	Type       int32
	WaveHeight float32
	WaveSpeed  float32
	WavePitch  float32
	AnimSpeed  int32
}

// WaterTextures names a water type's frames.
type WaterTextures struct {
	Type   int32
	Frames [32]string
}

// Object is a placed model.
type Object struct {
	Model string
	// Matrix places the model in map space, column-major.
	Matrix    [16]float32
	AnimType  int32
	AnimSpeed float32
	BlockType int32
}

// Sound is a positioned ambient sound. Distances are in cells.
type Sound struct {
	Name          string
	Pos           [3]float32
	Volume        float32
	Width, Height float32
	Range         float32
	// Cycle is the seconds between plays.
	Cycle float32
}

// Effect is a positioned effect by RO effect ID.
type Effect struct {
	ID    int32
	Pos   [3]float32
	Delay float32
	Param [4]float32
}

// MaxParticles is the most particles one emitter can have alive.
const MaxParticles = 1000

// Emitter flags.
const (
	// DepthTest hides particles behind the ground and models.
	DepthTest = 1
	// Eternal particles never fade or die.
	Eternal = 2
	// Flat particles lie in the plane given by Rotate instead of facing
	// the camera.
	Flat = 4
)

// Emitter spawns textured particles. Distances are in cells and times in
// seconds. A particle starts at Pos plus a random offset within ±Radius,
// with a direction between Dir1 and Dir2 on each axis, and is at
// start + dir × Speed × t + Gravity × Speed × t² / 2 after t seconds.
type Emitter struct {
	Pos                [3]float32
	Texture            string
	SrcBlend, DstBlend uint8
	Color              [4]uint8
	Dir1, Dir2         [3]float32
	Gravity            [3]float32
	Radius             [3]float32
	// Life, Rate, and Size are ranges: seconds, particles per second, and
	// half the particle's width.
	Life, Rate, Size [2]float32
	Speed            float32
	MaxCount         uint16
	Flags            uint8
	Scale            [2]float32
	// Rotate is the plane of flat particles, in degrees about X, Y, and Z.
	Rotate [3]float32
}

type writer struct{ b []byte }

func (w *writer) u8(v uint8)    { w.b = append(w.b, v) }
func (w *writer) u16(v int)     { w.b = binary.LittleEndian.AppendUint16(w.b, uint16(v)) } // #nosec G115 -- checked by Validate.
func (w *writer) u32(v int)     { w.b = binary.LittleEndian.AppendUint32(w.b, uint32(v)) } // #nosec G115 -- checked by Validate.
func (w *writer) i32(v int32)   { w.b = binary.LittleEndian.AppendUint32(w.b, uint32(v)) } // #nosec G115 -- reinterprets the bits.
func (w *writer) f32(v float32) { w.b = binary.LittleEndian.AppendUint32(w.b, math.Float32bits(v)) }
func (w *writer) str(s string)  { w.u8(uint8(len(s))); w.b = append(w.b, s...) } // #nosec G115 -- checked by Validate.
func (w *writer) f32s(v ...float32) {
	for _, f := range v {
		w.f32(f)
	}
}

func (w *writer) section(tag string, body func(*writer)) {
	s := &writer{}
	body(s)
	w.b = append(w.b, tag...)
	w.u32(len(s.b))
	w.b = append(w.b, s.b...)
}

// Validate checks limits that Encode relies on.
func (m *Map) Validate() error {
	if m.Width < 1 || m.Height < 1 || m.Width > MaxCells || m.Height > MaxCells || len(m.Cells) != m.Width*m.Height {
		return fmt.Errorf("%w: %d × %d cells, %d records", ErrFormat, m.Width, m.Height, len(m.Cells))
	}
	g := &m.Ground
	if g.Width < 0 || g.Height < 0 || g.Width > MaxCells || g.Height > MaxCells || len(g.Surfaces) != g.Width*g.Height {
		return fmt.Errorf("%w: ground is %d × %d", ErrFormat, g.Width, g.Height)
	}
	if len(g.Textures) > math.MaxUint16 || g.LightmapColumns < 0 || g.LightmapColumns > math.MaxUint16 || g.LightmapCount < 0 {
		return fmt.Errorf("%w: ground limits", ErrFormat)
	}
	names := append([]string{g.Lightmap, m.Music}, g.Textures...)
	names = append(names, m.Deps...)
	for _, o := range m.Objects {
		names = append(names, o.Model)
	}
	for _, s := range m.Sounds {
		names = append(names, s.Name)
	}
	for _, e := range m.Emitters {
		names = append(names, e.Texture)
		if e.MaxCount > MaxParticles {
			return fmt.Errorf("%w: emitter with %d particles", ErrFormat, e.MaxCount)
		}
	}
	if m.Water != nil {
		if m.Water.SplitWidth < 0 || m.Water.SplitHeight < 0 || len(m.Water.Zones) != m.Water.SplitWidth*m.Water.SplitHeight || m.Water.SplitWidth > math.MaxUint16 || m.Water.SplitHeight > math.MaxUint16 || len(m.Water.Textures) > math.MaxUint8 {
			return fmt.Errorf("%w: water zones", ErrFormat)
		}
		for _, t := range m.Water.Textures {
			names = append(names, t.Frames[:]...)
		}
	}
	for _, n := range names {
		if len(n) > 255 || !utf8.ValidString(n) {
			return fmt.Errorf("%w: name %q", ErrFormat, n)
		}
	}
	for _, t := range g.Tiles {
		if int(t.Texture) >= len(g.Textures) {
			return fmt.Errorf("%w: tile texture %d", ErrFormat, t.Texture)
		}
	}
	for _, s := range g.Surfaces {
		for _, t := range [3]int32{s.Top, s.North, s.East} {
			if t < -1 || int(t) >= len(g.Tiles) {
				return fmt.Errorf("%w: surface tile %d", ErrFormat, t)
			}
		}
	}
	return nil
}

// Encode writes a map.
func (m *Map) Encode() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	w := &writer{}
	w.b = append(w.b, magic...)
	w.u16(version)
	w.u16(m.Width)
	w.u16(m.Height)
	w.section("CELL", func(w *writer) {
		for _, c := range m.Cells {
			w.u8(c.Flags)
			w.f32s(c.Heights[:]...)
		}
	})
	g := &m.Ground
	w.section("GRND", func(w *writer) {
		w.u16(g.Width)
		w.u16(g.Height)
		w.u16(len(g.Textures))
		for _, t := range g.Textures {
			w.str(t)
		}
		w.u32(len(g.Tiles))
		for _, t := range g.Tiles {
			w.f32s(t.U[:]...)
			w.f32s(t.V[:]...)
			w.u16(int(t.Texture))
			w.u32(int(t.Lightmap))
			w.b = append(w.b, t.Color[:]...)
		}
		for _, s := range g.Surfaces {
			w.f32s(s.Heights[:]...)
			w.i32(s.Top)
			w.i32(s.North)
			w.i32(s.East)
		}
	})
	w.section("LMAP", func(w *writer) {
		w.str(g.Lightmap)
		w.u32(g.LightmapCount)
		w.u16(g.LightmapColumns)
	})
	w.section("LGHT", func(w *writer) {
		w.i32(m.Light.Longitude)
		w.i32(m.Light.Latitude)
		w.f32s(m.Light.Diffuse[:]...)
		w.f32s(m.Light.Ambient[:]...)
		w.f32(m.Light.Opacity)
		w.f32s(m.Light.Direction[:]...)
	})
	if wa := m.Water; wa != nil {
		w.section("WATR", func(w *writer) {
			zone := func(z Zone) {
				w.f32(z.Level)
				w.i32(z.Type)
				w.f32s(z.WaveHeight, z.WaveSpeed, z.WavePitch)
				w.i32(z.AnimSpeed)
			}
			zone(wa.Zone)
			w.u16(wa.SplitWidth)
			w.u16(wa.SplitHeight)
			for _, z := range wa.Zones {
				zone(z)
			}
			w.u8(uint8(len(wa.Textures))) // #nosec G115 -- checked by Validate.
			for _, t := range wa.Textures {
				w.i32(t.Type)
				for _, f := range t.Frames {
					w.str(f)
				}
			}
		})
	}
	w.section("OBJS", func(w *writer) {
		w.u32(len(m.Objects))
		for _, o := range m.Objects {
			w.str(o.Model)
			w.f32s(o.Matrix[:]...)
			w.i32(o.AnimType)
			w.f32(o.AnimSpeed)
			w.i32(o.BlockType)
		}
	})
	w.section("SNDS", func(w *writer) {
		w.u32(len(m.Sounds))
		for _, s := range m.Sounds {
			w.str(s.Name)
			w.f32s(s.Pos[:]...)
			w.f32s(s.Volume, s.Width, s.Height, s.Range, s.Cycle)
		}
	})
	w.section("EFCT", func(w *writer) {
		w.u32(len(m.Effects))
		for _, e := range m.Effects {
			w.i32(e.ID)
			w.f32s(e.Pos[:]...)
			w.f32(e.Delay)
			w.f32s(e.Param[:]...)
		}
	})
	if m.Music != "" {
		w.section("MUSC", func(w *writer) { w.str(m.Music) })
	}
	if len(m.Emitters) > 0 {
		w.section("EMIT", func(w *writer) {
			w.u32(len(m.Emitters))
			for _, e := range m.Emitters {
				w.f32s(e.Pos[:]...)
				w.str(e.Texture)
				w.u8(e.SrcBlend)
				w.u8(e.DstBlend)
				w.b = append(w.b, e.Color[:]...)
				w.f32s(e.Dir1[:]...)
				w.f32s(e.Dir2[:]...)
				w.f32s(e.Gravity[:]...)
				w.f32s(e.Radius[:]...)
				w.f32s(e.Life[:]...)
				w.f32s(e.Rate[:]...)
				w.f32s(e.Size[:]...)
				w.f32(e.Speed)
				w.u16(int(e.MaxCount))
				w.u8(e.Flags)
				w.f32s(e.Scale[:]...)
				w.f32s(e.Rotate[:]...)
			}
		})
	}
	w.section("DEPS", func(w *writer) {
		w.u32(len(m.Deps))
		for _, d := range m.Deps {
			w.str(d)
		}
	})
	if len(w.b) > MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrFormat, len(w.b))
	}
	return w.b, nil
}

type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b)-r.p {
		r.err = ErrFormat
		return nil
	}
	out := r.b[r.p : r.p+n]
	r.p += n
	return out
}

func (r *reader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) u16() int {
	if b := r.take(2); b != nil {
		return int(binary.LittleEndian.Uint16(b))
	}
	return 0
}

func (r *reader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *reader) i32() int32   { return int32(r.u32()) } // #nosec G115 -- reinterprets the bits.
func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }
func (r *reader) str() string  { return string(r.take(int(r.u8()))) }

// count reads a u32 count and checks it against the bytes left, given
// the smallest size of one item.
func (r *reader) count(itemSize int) int {
	n := r.u32()
	// n is at most 2^32 and itemSize is small, so the product fits in
	// int64.
	if r.err == nil && int64(n)*int64(itemSize) > int64(len(r.b)-r.p) {
		r.err = ErrFormat
	}
	if r.err != nil {
		return 0
	}
	return int(n)
}

func (r *reader) f32s(dst []float32) {
	for i := range dst {
		dst[i] = r.f32()
	}
}

// Decode reads a map. Unknown sections are skipped.
func Decode(b []byte) (*Map, error) {
	if len(b) > MaxBytes {
		return nil, ErrFormat
	}
	r := &reader{b: b}
	if string(r.take(4)) != magic || r.u16() != version {
		return nil, fmt.Errorf("%w: header", ErrFormat)
	}
	m := &Map{Width: r.u16(), Height: r.u16()}
	if r.err != nil || m.Width < 1 || m.Height < 1 || m.Width > MaxCells || m.Height > MaxCells {
		return nil, fmt.Errorf("%w: size", ErrFormat)
	}
	seen := map[string]bool{}
	for r.err == nil && r.p < len(b) {
		tag := string(r.take(4))
		body := r.take(int(r.u32()))
		if r.err != nil {
			break
		}
		if seen[tag] {
			return nil, fmt.Errorf("%w: repeated section %q", ErrFormat, tag)
		}
		seen[tag] = true
		s := &reader{b: body}
		switch tag {
		case "CELL":
			if len(body) != m.Width*m.Height*17 {
				return nil, fmt.Errorf("%w: cell section", ErrFormat)
			}
			m.Cells = make([]Cell, m.Width*m.Height)
			for i := range m.Cells {
				m.Cells[i].Flags = s.u8()
				s.f32s(m.Cells[i].Heights[:])
			}
		case "GRND":
			g := &m.Ground
			g.Width, g.Height = s.u16(), s.u16()
			for range s.u16() {
				g.Textures = append(g.Textures, s.str())
			}
			g.Tiles = make([]Tile, s.count(42))
			for i := range g.Tiles {
				t := &g.Tiles[i]
				s.f32s(t.U[:])
				s.f32s(t.V[:])
				t.Texture = uint16(s.u16()) // #nosec G115 -- read as 16 bits.
				t.Lightmap = s.u32()
				copy(t.Color[:], s.take(4))
			}
			// The surfaces fill the rest of the section, which bounds the
			// allocation below.
			if s.err != nil || g.Width > MaxCells || g.Height > MaxCells || g.Width*g.Height*28 != len(body)-s.p {
				return nil, fmt.Errorf("%w: ground section", ErrFormat)
			}
			g.Surfaces = make([]Surface, g.Width*g.Height)
			for i := range g.Surfaces {
				sf := &g.Surfaces[i]
				s.f32s(sf.Heights[:])
				sf.Top, sf.North, sf.East = s.i32(), s.i32(), s.i32()
			}
		case "LMAP":
			m.Ground.Lightmap = s.str()
			m.Ground.LightmapCount = int(s.u32())
			m.Ground.LightmapColumns = s.u16()
		case "LGHT":
			m.Light.Longitude, m.Light.Latitude = s.i32(), s.i32()
			s.f32s(m.Light.Diffuse[:])
			s.f32s(m.Light.Ambient[:])
			m.Light.Opacity = s.f32()
			s.f32s(m.Light.Direction[:])
		case "WATR":
			wa := &WaterInfo{}
			zone := func() Zone {
				return Zone{Level: s.f32(), Type: s.i32(), WaveHeight: s.f32(), WaveSpeed: s.f32(), WavePitch: s.f32(), AnimSpeed: s.i32()}
			}
			wa.Zone = zone()
			wa.SplitWidth, wa.SplitHeight = s.u16(), s.u16()
			if s.err != nil || wa.SplitWidth*wa.SplitHeight*24 > len(body)-s.p {
				return nil, fmt.Errorf("%w: water zones", ErrFormat)
			}
			for range wa.SplitWidth * wa.SplitHeight {
				wa.Zones = append(wa.Zones, zone())
			}
			for range s.u8() {
				t := WaterTextures{Type: s.i32()}
				for k := range t.Frames {
					t.Frames[k] = s.str()
				}
				wa.Textures = append(wa.Textures, t)
			}
			m.Water = wa
		case "OBJS":
			for range s.count(77) {
				o := Object{Model: s.str()}
				s.f32s(o.Matrix[:])
				o.AnimType, o.AnimSpeed, o.BlockType = s.i32(), s.f32(), s.i32()
				m.Objects = append(m.Objects, o)
			}
		case "SNDS":
			for range s.count(33) {
				o := Sound{Name: s.str()}
				s.f32s(o.Pos[:])
				o.Volume, o.Width, o.Height, o.Range, o.Cycle = s.f32(), s.f32(), s.f32(), s.f32(), s.f32()
				m.Sounds = append(m.Sounds, o)
			}
		case "EFCT":
			for range s.count(36) {
				e := Effect{ID: s.i32()}
				s.f32s(e.Pos[:])
				e.Delay = s.f32()
				s.f32s(e.Param[:])
				m.Effects = append(m.Effects, e)
			}
		case "MUSC":
			m.Music = s.str()
		case "EMIT":
			for range s.count(130) {
				e := Emitter{}
				s.f32s(e.Pos[:])
				e.Texture = s.str()
				e.SrcBlend, e.DstBlend = s.u8(), s.u8()
				copy(e.Color[:], s.take(4))
				s.f32s(e.Dir1[:])
				s.f32s(e.Dir2[:])
				s.f32s(e.Gravity[:])
				s.f32s(e.Radius[:])
				s.f32s(e.Life[:])
				s.f32s(e.Rate[:])
				s.f32s(e.Size[:])
				e.Speed = s.f32()
				e.MaxCount = uint16(s.u16()) // #nosec G115 -- read as 16 bits.
				e.Flags = s.u8()
				s.f32s(e.Scale[:])
				s.f32s(e.Rotate[:])
				m.Emitters = append(m.Emitters, e)
			}
		case "DEPS":
			for range s.count(1) {
				m.Deps = append(m.Deps, s.str())
			}
		}
		if s.err != nil {
			return nil, fmt.Errorf("%w: section %q", ErrFormat, tag)
		}
	}
	if r.err != nil {
		return nil, fmt.Errorf("%w: truncated", ErrFormat)
	}
	for _, tag := range []string{"CELL", "GRND", "LMAP", "LGHT", "OBJS", "SNDS", "EFCT", "DEPS"} {
		if !seen[tag] {
			return nil, fmt.Errorf("%w: no %s section", ErrFormat, tag)
		}
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}
