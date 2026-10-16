package mapfolder

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// lightmap is the lightmap of map folders' WMAPs, as for generated maps.
const lightmap = "placeholder/lightmap"

func cells(v rng.Fixed) float32 { return float32(v) / float32(rng.One) }

// WMAP writes the map as WMAP (docs/specs/assets-v1.md, "Map (WMAP)"):
//
//   - CELL from cells.bin.
//   - GRND with one surface per 2 × 2 cells, at the outer corners of its
//     cells, and the material of its south-west cell. Blends are not
//     drawn yet.
//   - The placeholder lightmap, a light from the north-east, and water
//     at water_level when map.yaml sets it.
//   - OBJS from objects.yaml, standing on the ground at their point.
func (m *Map) WMAP() ([]byte, error) {
	out := wmap.Map{Width: m.Width, Height: m.Height, Music: m.Music}
	out.Cells = make([]wmap.Cell, m.Width*m.Height)
	for i, t := range m.Types {
		c := wmap.Cell{Flags: gen.Flags(t)}
		for k := range 4 {
			c.Heights[k] = cells(m.Heights[4*i+k])
		}
		out.Cells[i] = c
	}
	sw, sh := (m.Width+1)/2, (m.Height+1)/2
	g := &out.Ground
	g.Width, g.Height = sw, sh
	g.Lightmap, g.LightmapCount, g.LightmapColumns = lightmap, 1, 1
	tileOf := make([]int32, len(m.Materials))
	for i := range tileOf {
		tileOf[i] = -1
	}
	corner := func(x, y, k int) float32 {
		x, y = min(x, m.Width-1), min(y, m.Height-1)
		return cells(m.Heights[4*(y*m.Width+x)+k])
	}
	for j := range sh {
		for i := range sw {
			mat := m.Material[2*j*m.Width+2*i]
			if tileOf[mat] < 0 {
				tileOf[mat] = int32(len(g.Tiles))                 // #nosec G115 -- at most 255 tiles.
				g.Textures = append(g.Textures, m.Materials[mat]) // one texture per tile
				g.Tiles = append(g.Tiles, wmap.Tile{
					U: [4]float32{0, 1, 0, 1}, V: [4]float32{1, 1, 0, 0},
					Texture: uint16(len(g.Textures) - 1), Color: [4]uint8{255, 255, 255, 255}, // #nosec G115 -- at most 255 textures.
				})
			}
			x, y := 2*i, 2*j
			g.Surfaces = append(g.Surfaces, wmap.Surface{
				Heights: [4]float32{corner(x, y, 0), corner(x+1, y, 1), corner(x, y+1, 2), corner(x+1, y+1, 3)},
				Top:     tileOf[mat], North: -1, East: -1,
			})
		}
	}
	out.Light = wmap.Light{Longitude: 45, Latitude: 45, Diffuse: [3]float32{1, 1, 1}, Ambient: [3]float32{0.35, 0.35, 0.35},
		Opacity: 1, Direction: [3]float32{-0.5, float32(math.Sqrt2 / 2), 0.5}}
	if m.HasWater {
		out.Water = &wmap.WaterInfo{Zone: wmap.Zone{Level: cells(m.WaterLevel), WaveHeight: 0.125, WaveSpeed: 2, WavePitch: 50, AnimSpeed: 3}}
	}
	for _, o := range m.Objects {
		x, y := min(int(o.X), m.Width-1), min(int(o.Y), m.Height-1)
		h := float32(0)
		for k := range 4 {
			h += cells(m.Heights[4*(y*m.Width+x)+k]) / 4
		}
		r := o.Rotation * math.Pi / 180
		c, s, sc := float32(math.Cos(r)*o.Scale), float32(math.Sin(r)*o.Scale), float32(o.Scale)
		out.Objects = append(out.Objects, wmap.Object{Model: o.Model, AnimSpeed: 1, Matrix: [16]float32{
			c, 0, -s, 0,
			0, sc, 0, 0,
			s, 0, c, 0,
			float32(o.X), h, -float32(o.Y), 1,
		}})
	}
	deps := append(slices.Clone(g.Textures), lightmap)
	for _, o := range m.Objects {
		deps = append(deps, o.Model)
	}
	if m.Music != "" {
		deps = append(deps, m.Music)
	}
	slices.Sort(deps)
	out.Deps = slices.Compact(deps)
	return out.Encode()
}

// EncodeCells writes cells.bin for a grid's cells.
func EncodeCells(width, height int, types []uint8, heights []rng.Fixed) []byte {
	b := header(cellsMagic, width, height)
	for i, t := range types {
		b = append(b, t)
		for k := range 4 {
			b = binary.BigEndian.AppendUint32(b, uint32(heights[4*i+k])) // #nosec G115 -- two's complement.
		}
	}
	return b
}

// EncodeMaterials writes materials.bin: each cell's material, the
// material it blends toward, and the blend's weight.
func EncodeMaterials(width, height int, material, blend, weight []uint8) []byte {
	b := header(materialsMagic, width, height)
	for i := range material {
		b = append(b, material[i], blend[i], weight[i])
	}
	return b
}

func header(magic string, width, height int) []byte {
	b := append([]byte(magic), Format)
	b = binary.BigEndian.AppendUint16(b, uint16(width))     // #nosec G115 -- at most MaxSize.
	return binary.BigEndian.AppendUint16(b, uint16(height)) // #nosec G115 -- at most MaxSize.
}
