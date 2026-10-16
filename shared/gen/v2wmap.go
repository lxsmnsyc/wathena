//go:build procedural

package gen

import (
	"encoding/binary"
	"math/bits"
	"slices"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// The WMAP encoding of a version 2 field (docs/specs/assets-v1.md, "Map
// (WMAP)"). WMAP stores f32 values, which this file builds from
// fixed-point values with integer math, so the server and the web client
// write the same bytes.

// f32bits returns the IEEE 754 single-precision bits of v / 2^frac,
// rounded to the nearest value, ties to even. The values here are far
// inside the normal range.
func f32bits(v int64, frac uint) uint32 {
	if v == 0 {
		return 0
	}
	var sign uint32
	u := uint64(v) // #nosec G115 -- the sign is handled below.
	if v < 0 {
		sign = 1 << 31
		u = uint64(-v) // #nosec G115 -- v is above the int64 minimum here.
	}
	n := bits.Len64(u)
	exp := n - 1 - int(frac) // #nosec G115 -- frac is small.
	var mant uint64
	if n > 24 {
		r := uint(n - 24) // #nosec G115 -- positive.
		mant = u >> r
		rem := u & (1<<r - 1)
		half := uint64(1) << (r - 1)
		if rem > half || (rem == half && mant&1 == 1) {
			mant++
			if mant == 1<<24 {
				mant >>= 1
				exp++
			}
		}
	} else {
		mant = u << uint(24-n) // #nosec G115 -- positive.
	}
	return sign | uint32(exp+127)<<23 | uint32(mant)&0x7fffff // #nosec G115 -- 8 and 23 bits.
}

// v2Turn holds cos and sin of each 16th of a turn, in Q16.16.
var v2Turn = [16][2]int64{
	{65536, 0}, {60547, 25080}, {46341, 46341}, {25080, 60547},
	{0, 65536}, {-25080, 60547}, {-46341, 46341}, {-60547, 25080},
	{-65536, 0}, {-60547, -25080}, {-46341, -46341}, {-25080, -60547},
	{0, -65536}, {25080, -60547}, {46341, -46341}, {60547, -25080},
}

// wmapWriter appends WMAP values.
type wmapWriter struct{ b []byte }

func (w *wmapWriter) u8(v uint8)    { w.b = append(w.b, v) }
func (w *wmapWriter) u16(v int)     { w.b = binary.LittleEndian.AppendUint16(w.b, uint16(v)) } // #nosec G115 -- sizes are at most 1024.
func (w *wmapWriter) u32(v int)     { w.b = binary.LittleEndian.AppendUint32(w.b, uint32(v)) } // #nosec G115 -- counts fit.
func (w *wmapWriter) i32(v int32)   { w.b = binary.LittleEndian.AppendUint32(w.b, uint32(v)) } // #nosec G115 -- two's complement.
func (w *wmapWriter) bits(v uint32) { w.b = binary.LittleEndian.AppendUint32(w.b, v) }

// fixed writes a Q16.16 value as f32.
func (w *wmapWriter) fixed(v rng.Fixed) { w.bits(f32bits(int64(v), 16)) }

func (w *wmapWriter) str(s string) {
	w.u8(uint8(len(s))) // #nosec G115 -- names are short.
	w.b = append(w.b, s...)
}

// section writes a tag, then the body's length, then the body, in
// place.
func (w *wmapWriter) section(tag string, body func(*wmapWriter)) {
	w.b = append(w.b, tag...)
	at := len(w.b)
	w.b = append(w.b, 0, 0, 0, 0)
	body(w)
	binary.LittleEndian.PutUint32(w.b[at:], uint32(len(w.b)-at-4)) // #nosec G115 -- maps are below 64 MiB.
}

// v2Texture and v2Model are the asset names a field uses.
func v2Texture(b *v2Biome, material string) string { return TextureName(b.Tileset, material) }

func v2Model(b *v2Biome, prop string) string { return ModelName(b.Tileset, prop) }

// v2Lightmap is the lightmap of every field: no baked light.
const v2Lightmap = "placeholder/lightmap"

// encode writes the field as WMAP:
//
//   - CELL: each cell's flags and corner heights.
//   - GRND: a surface of 2 × 2 cells takes the material of its cells'
//     highest rank. One tile per material covers a surface once.
//   - LMAP, LGHT: no baked light, and the biome's light.
//   - WATR: the water level, when any cell is below it.
//   - OBJS: the props, each turned and scaled on its cell.
//   - SNDS, EFCT: none. DEPS: every name, sorted.
func (m *v2Map) encode() []byte {
	b := m.b
	sw, sh := (m.w+1)/2, (m.h+1)/2
	// The map's size, so the bytes are written once without copies.
	size := 1024 + 17*m.w*m.h + 28*sw*sh + 64*matCount + 32*64 + 128*len(m.objects)
	w := &wmapWriter{b: make([]byte, 0, size)}
	w.b = append(w.b, "WMAP"...)
	w.u16(1)
	w.u16(m.w)
	w.u16(m.h)
	var deps []string
	w.section("CELL", func(w *wmapWriter) {
		for y := range m.h {
			for x := range m.w {
				w.u8(Flags(m.types[y*m.w+x]))
				v := y*m.vw + x
				w.fixed(m.vert[v])
				w.fixed(m.vert[v+1])
				w.fixed(m.vert[v+m.vw])
				w.fixed(m.vert[v+m.vw+1])
			}
		}
	})
	surface := make([]uint8, sw*sh)
	var used [matCount]bool
	for j := range sh {
		for i := range sw {
			best, mat := -1, 0
			for k := range 4 {
				x, y := 2*i+k&1, 2*j+k>>1
				if x >= m.w || y >= m.h {
					continue
				}
				c := y*m.w + x
				if r := int(m.rank[c]); r > best {
					best, mat = r, int(m.material[c])
				}
			}
			surface[j*sw+i] = uint8(mat) // #nosec G115 -- below matCount.
			used[mat] = true
		}
	}
	var tileOf [matCount]int
	var textures []string
	for mat, u := range used {
		if u {
			tileOf[mat] = len(textures)
			textures = append(textures, v2Texture(b, v2Materials[mat]))
		}
	}
	deps = append(deps, textures...)
	w.section("GRND", func(w *wmapWriter) {
		w.u16(sw)
		w.u16(sh)
		w.u16(len(textures))
		for _, t := range textures {
			w.str(t)
		}
		w.u32(len(textures))
		one := f32bits(1, 0)
		for t := range textures {
			// U, then V, in surface corner order.
			for _, u := range [8]uint32{0, one, 0, one, one, one, 0, 0} {
				w.bits(u)
			}
			w.u16(t)
			w.u32(0)
			w.b = append(w.b, 255, 255, 255, 255)
		}
		for j := range sh {
			for i := range sw {
				for k := range 4 {
					x := min(2*i+2*(k&1), m.w)
					y := min(2*j+2*(k>>1), m.h)
					w.fixed(m.vert[y*m.vw+x])
				}
				w.i32(int32(tileOf[surface[j*sw+i]])) // #nosec G115 -- a few tiles.
				w.i32(-1)
				w.i32(-1)
			}
		}
	})
	deps = append(deps, v2Lightmap)
	w.section("LMAP", func(w *wmapWriter) {
		w.str(v2Lightmap)
		w.u32(1)
		w.u16(1)
	})
	w.section("LGHT", func(w *wmapWriter) {
		w.i32(45)
		w.i32(45)
		for _, c := range b.Diffuse {
			w.fixed(c)
		}
		for _, c := range b.Ambient {
			w.fixed(c)
		}
		w.fixed(rng.One)
		// Toward the light at 45 degrees of longitude and latitude.
		w.fixed(-rng.One / 2)
		w.fixed(46341)
		w.fixed(rng.One / 2)
	})
	wet := false
	for _, h := range m.mean {
		if h < m.level {
			wet = true
			break
		}
	}
	if wet {
		water := v2Texture(b, "water")
		deps = append(deps, water)
		w.section("WATR", func(w *wmapWriter) {
			w.fixed(m.level)
			w.i32(0)
			w.fixed(rng.One / 8)
			w.fixed(2 * rng.One)
			w.fixed(50 * rng.One)
			w.i32(3)
			w.u16(0)
			w.u16(0)
			w.u8(1)
			w.i32(0)
			for range 32 {
				w.str(water)
			}
		})
	}
	w.section("OBJS", func(w *wmapWriter) {
		w.u32(len(m.objects))
		for _, o := range m.objects {
			name := v2Model(b, o.Prop)
			deps = append(deps, name)
			w.str(name)
			s := int64(o.Scale)
			cs := v2Turn[o.Turn]
			c, sn := s*cs[0], s*cs[1]
			zero := uint32(0)
			for _, v := range [16]uint32{
				f32bits(c, 32), zero, f32bits(-sn, 32), zero,
				zero, f32bits(s, 16), zero, zero,
				f32bits(sn, 32), zero, f32bits(c, 32), zero,
				f32bits(int64(2*o.X+1), 1), f32bits(int64(m.mean[o.Y*m.w+o.X]), 16), f32bits(-int64(2*o.Y+1), 1), f32bits(1, 0),
			} {
				w.bits(v)
			}
			w.i32(0)
			w.bits(f32bits(1, 0))
			w.i32(0)
		}
	})
	w.section("SNDS", func(w *wmapWriter) { w.u32(0) })
	w.section("EFCT", func(w *wmapWriter) { w.u32(0) })
	slices.Sort(deps)
	deps = slices.Compact(deps)
	w.section("DEPS", func(w *wmapWriter) {
		w.u32(len(deps))
		for _, d := range deps {
			w.str(d)
		}
	})
	return w.b
}
