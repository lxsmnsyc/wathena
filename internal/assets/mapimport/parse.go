// Package mapimport converts RO maps, the GAT, GND, and RSW files, into
// assets-v1 maps (WMAP). Game files can be crafted, so every parser checks
// lengths before reading.
package mapimport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"golang.org/x/text/encoding/korean"
)

var ErrFormat = errors.New("mapimport: invalid map file")

// Parsing limits. Real maps are far below them.
const (
	maxSize      = 2048
	maxTextures  = 4096
	maxLightmaps = 1 << 20
	maxTiles     = 1 << 22
	maxObjects   = 1 << 18
	maxZones     = 1 << 16
)

type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b)-r.p {
		if r.err == nil {
			r.err = fmt.Errorf("%w: file is truncated", ErrFormat)
		}
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

func (r *reader) u16() uint16 {
	if b := r.take(2); b != nil {
		return binary.LittleEndian.Uint16(b)
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

func (r *reader) vec3() [3]float32 { return [3]float32{r.f32(), r.f32(), r.f32()} }

// count reads a count and checks it against a limit and the bytes left,
// given the smallest size of one item.
func (r *reader) count(limit, itemSize int) int {
	n := r.i32()
	if r.err == nil && (n < 0 || int(n) > limit || int(n)*itemSize > len(r.b)-r.p) {
		r.err = fmt.Errorf("%w: count %d", ErrFormat, n)
	}
	if r.err != nil {
		return 0
	}
	return int(n)
}

// name reads a NUL-padded CP949 string of n bytes.
func (r *reader) name(n int) string {
	b := r.take(n)
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	s, err := korean.EUCKR.NewDecoder().String(string(b))
	if err != nil {
		return strings.ToValidUTF8(string(b), "")
	}
	return s
}

func (r *reader) version(magic string) int {
	if string(r.take(4)) != magic {
		r.err = fmt.Errorf("%w: not a %s file", ErrFormat, magic)
		return 0
	}
	return int(r.u8())*10 + int(r.u8())
}

// GAT is the cell grid: walkability and heights.
type GAT struct {
	Width, Height int
	Cells         []GATCell
}

// GATCell is one cell. Heights are RO units, Y down, for the corners
// (x, y), (x+1, y), (x, y+1), and (x+1, y+1).
type GATCell struct {
	Heights [4]float32
	Type    uint32
}

// ParseGAT reads a GAT file.
func ParseGAT(b []byte) (*GAT, error) {
	r := &reader{b: b}
	r.version("GRAT")
	g := &GAT{Width: int(r.u32()), Height: int(r.u32())}
	if r.err != nil {
		return nil, r.err
	}
	if g.Width < 1 || g.Height < 1 || g.Width > maxSize || g.Height > maxSize || g.Width*g.Height*20 > len(b)-r.p {
		return nil, fmt.Errorf("%w: gat is %d × %d", ErrFormat, g.Width, g.Height)
	}
	g.Cells = make([]GATCell, g.Width*g.Height)
	for i := range g.Cells {
		c := &g.Cells[i]
		for k := range 4 {
			c.Heights[k] = r.f32()
		}
		c.Type = r.u32()
	}
	return g, r.err
}

// GND is the ground: textured tiles on a grid of surfaces, each 2 × 2
// cells, with lightmaps.
type GND struct {
	Version       int
	Width, Height int
	Zoom          float32
	Textures      []string
	// Lightmaps are 8 × 8: 64 shadow bytes, then 64 RGB colors.
	Lightmaps [][256]byte
	Tiles     []Tile
	Surfaces  []Surface
	Water     *Water
}

// Tile is a textured quad. U and V are for the corners in the order of
// Surface.Heights. Color is B, G, R, A.
type Tile struct {
	U, V     [4]float32
	Texture  uint16
	Lightmap uint16
	Color    [4]uint8
}

// Surface is one ground square. Up, Front, and Right are tile indexes, or
// -1: the top, the wall toward y+1, and the wall toward x+1.
type Surface struct {
	Heights          [4]float32
	Up, Front, Right int32
}

// Water describes a map's water. Versions of RSW before 2.6 store it; GND
// 1.8 and later store it, with per-zone values from 1.9.
type Water struct {
	Level       float32
	Type        int32
	WaveHeight  float32
	WaveSpeed   float32
	WavePitch   float32
	AnimSpeed   int32
	SplitWidth  int32
	SplitHeight int32
	Zones       []WaterZone
}

// WaterZone is one part of a split water surface.
type WaterZone struct {
	Level      float32
	Type       int32
	WaveHeight float32
	WaveSpeed  float32
	WavePitch  float32
	AnimSpeed  int32
}

// ParseGND reads a GND file. Versions 1.5 to 1.9 are supported.
func ParseGND(b []byte) (*GND, error) {
	r := &reader{b: b}
	g := &GND{Version: r.version("GRGN")}
	if r.err != nil {
		return nil, r.err
	}
	if g.Version < 15 || g.Version > 19 {
		return nil, fmt.Errorf("%w: gnd version %d.%d", ErrFormat, g.Version/10, g.Version%10)
	}
	g.Width, g.Height = int(r.u32()), int(r.u32())
	g.Zoom = r.f32()
	if g.Width < 1 || g.Height < 1 || g.Width > maxSize || g.Height > maxSize {
		return nil, fmt.Errorf("%w: gnd is %d × %d", ErrFormat, g.Width, g.Height)
	}
	n := r.count(maxTextures, 1)
	size := r.count(1024, 0)
	if r.err == nil && n*size > len(b)-r.p {
		r.err = fmt.Errorf("%w: gnd textures", ErrFormat)
	}
	for range n {
		g.Textures = append(g.Textures, r.name(size))
	}
	lights := r.count(maxLightmaps, 256)
	px, py, cell := r.i32(), r.i32(), r.i32()
	if r.err == nil && (px != 8 || py != 8 || cell != 1) {
		return nil, fmt.Errorf("%w: lightmaps are %d × %d × %d", ErrFormat, px, py, cell)
	}
	g.Lightmaps = make([][256]byte, lights)
	for i := range g.Lightmaps {
		copy(g.Lightmaps[i][:], r.take(256))
	}
	tiles := r.count(maxTiles, 40)
	g.Tiles = make([]Tile, tiles)
	for i := range g.Tiles {
		t := &g.Tiles[i]
		for k := range 4 {
			t.U[k] = r.f32()
		}
		for k := range 4 {
			t.V[k] = r.f32()
		}
		t.Texture = r.u16()
		t.Lightmap = r.u16()
		copy(t.Color[:], r.take(4))
	}
	if r.err == nil && g.Width*g.Height*28 > len(b)-r.p {
		return nil, fmt.Errorf("%w: gnd surfaces are truncated", ErrFormat)
	}
	g.Surfaces = make([]Surface, g.Width*g.Height)
	for i := range g.Surfaces {
		s := &g.Surfaces[i]
		for k := range 4 {
			s.Heights[k] = r.f32()
		}
		s.Up, s.Front, s.Right = r.i32(), r.i32(), r.i32()
	}
	if g.Version >= 18 {
		w := &Water{Level: r.f32(), Type: r.i32(), WaveHeight: r.f32(), WaveSpeed: r.f32(), WavePitch: r.f32(), AnimSpeed: r.i32(), SplitWidth: r.i32(), SplitHeight: r.i32()}
		if g.Version >= 19 {
			if w.SplitWidth < 0 || w.SplitHeight < 0 || int64(w.SplitWidth)*int64(w.SplitHeight) > maxZones {
				return nil, fmt.Errorf("%w: %d × %d water zones", ErrFormat, w.SplitWidth, w.SplitHeight)
			}
			for range int(w.SplitWidth) * int(w.SplitHeight) {
				w.Zones = append(w.Zones, WaterZone{Level: r.f32(), Type: r.i32(), WaveHeight: r.f32(), WaveSpeed: r.f32(), WavePitch: r.f32(), AnimSpeed: r.i32()})
			}
		}
		g.Water = w
	}
	if r.err != nil {
		return nil, r.err
	}
	return g, nil
}

// RSW is a map's world file: lighting, water, and objects.
type RSW struct {
	Version int
	GND     string
	GAT     string
	Water   *Water
	Light   Light
	Models  []Object
	Sounds  []Sound
	Effects []Effect
}

// Light is the map's directional light. Angles are degrees.
type Light struct {
	Longitude, Latitude int32
	Diffuse, Ambient    [3]float32
	Opacity             float32
}

// Object is a placed model. Positions are RO units relative to the map's
// center. Rotations are degrees around Z, then X, then Y.
type Object struct {
	Name      string
	AnimType  int32
	AnimSpeed float32
	BlockType int32
	File      string
	Pos       [3]float32
	Rot       [3]float32
	Scale     [3]float32
}

// Sound is a positioned ambient sound.
type Sound struct {
	File   string
	Pos    [3]float32
	Volume float32
	Width  int32
	Height int32
	Range  float32
	Cycle  float32
}

// Effect is a positioned effect.
type Effect struct {
	Pos   [3]float32
	ID    int32
	Delay float32
	Param [4]float32
}

// ParseRSW reads an RSW file. Versions 1.2 to 2.7 are supported.
func ParseRSW(b []byte) (*RSW, error) {
	r := &reader{b: b}
	w := &RSW{Version: r.version("GRSW")}
	if r.err != nil {
		return nil, r.err
	}
	if w.Version < 12 || w.Version > 27 {
		return nil, fmt.Errorf("%w: rsw version %d.%d", ErrFormat, w.Version/10, w.Version%10)
	}
	build := uint32(0)
	if w.Version >= 25 {
		build = r.u32()
	}
	if w.Version >= 22 {
		r.u8()
	}
	r.name(40) // ini file
	w.GND = r.name(40)
	w.GAT = r.name(40)
	if w.Version >= 14 {
		r.name(40) // source file
	}
	if w.Version < 26 {
		wa := &Water{Type: 0, WaveHeight: 5, WaveSpeed: 2, WavePitch: 50, AnimSpeed: 3}
		if w.Version >= 13 {
			wa.Level = r.f32()
		}
		if w.Version >= 18 {
			wa.Type, wa.WaveHeight, wa.WaveSpeed, wa.WavePitch = r.i32(), r.f32(), r.f32(), r.f32()
		}
		if w.Version >= 19 {
			wa.AnimSpeed = r.i32()
		}
		w.Water = wa
	}
	w.Light = Light{Longitude: 45, Latitude: 45, Diffuse: [3]float32{1, 1, 1}, Opacity: 1}
	if w.Version >= 15 {
		w.Light.Longitude, w.Light.Latitude = r.i32(), r.i32()
		w.Light.Diffuse, w.Light.Ambient = r.vec3(), r.vec3()
		if w.Version >= 17 {
			w.Light.Opacity = r.f32()
		}
	}
	if w.Version >= 16 {
		r.take(16) // ground bounds, unused
	}
	if w.Version >= 27 {
		r.take(4 * r.count(maxObjects, 4))
	}
	for range r.count(maxObjects, 4) {
		switch r.i32() {
		case 1:
			o := Object{AnimSpeed: 1}
			if w.Version >= 13 {
				o.Name = r.name(40)
				o.AnimType, o.AnimSpeed, o.BlockType = r.i32(), r.f32(), r.i32()
			}
			if w.Version >= 26 && build >= 186 {
				r.u8()
			}
			if w.Version >= 27 {
				r.i32()
			}
			o.File = r.name(80)
			r.name(80) // node name
			o.Pos, o.Rot, o.Scale = r.vec3(), r.vec3(), r.vec3()
			w.Models = append(w.Models, o)
		case 2:
			// Lights only mattered when the lightmaps were baked.
			r.take(80 + 12 + 12 + 4)
		case 3:
			s := Sound{}
			r.name(80)
			s.File = r.name(80)
			s.Pos, s.Volume = r.vec3(), r.f32()
			s.Width, s.Height, s.Range = r.i32(), r.i32(), r.f32()
			if w.Version >= 20 {
				s.Cycle = r.f32()
			}
			w.Sounds = append(w.Sounds, s)
		case 4:
			e := Effect{}
			r.name(80)
			e.Pos, e.ID, e.Delay = r.vec3(), r.i32(), r.f32()
			for k := range 4 {
				e.Param[k] = r.f32()
			}
			w.Effects = append(w.Effects, e)
		default:
			return nil, fmt.Errorf("%w: rsw object type", ErrFormat)
		}
		if r.err != nil {
			return nil, r.err
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	// A quad tree follows in some versions. It is not used.
	return w, nil
}
