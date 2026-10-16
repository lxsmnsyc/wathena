package placeholder

import (
	"fmt"
	"math"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/assets/model"
)

// Props are the props with a placeholder model, placeholder/prop/<prop>:
// the field props, then the dungeon and town props (decision 0040).
var Props = []string{
	"tree", "pine", "bush", "rock", "cactus", "palm", "deadtree", "reed",
	"stalagmite", "stairs_up", "stairs_down", "altar", "pillar", "coffin", "bones", "brazier", "crate",
	"fountain", "lamp", "tower", "gate", "well",
}

// rgb returns the glTF base color for an sRGB color.
func rgb(r, g, b uint8) [4]float32 {
	lin := func(v uint8) float32 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return float32(c / 12.92)
		}
		return float32(math.Pow((c+0.055)/1.055, 2.4))
	}
	return [4]float32{lin(r), lin(g), lin(b), 1}
}

var (
	bark      = rgb(0x6e, 0x4a, 0x2c)
	leaves    = rgb(0x4a, 0x8c, 0x34)
	needles   = rgb(0x2a, 0x5e, 0x30)
	shrub     = rgb(0x52, 0x94, 0x3c)
	stone     = rgb(0x84, 0x84, 0x88)
	cactus    = rgb(0x4c, 0x8a, 0x44)
	palmBark  = rgb(0x8c, 0x6c, 0x48)
	frond     = rgb(0x5a, 0xa0, 0x3c)
	deadBark  = rgb(0x6a, 0x5e, 0x50)
	reedGreen = rgb(0xa8, 0xb4, 0x5c)
	masonry   = rgb(0x9a, 0x94, 0x8a)
	darkStone = rgb(0x5a, 0x56, 0x52)
	wood      = rgb(0x7a, 0x55, 0x33)
	darkWood  = rgb(0x4a, 0x34, 0x24)
	bone      = rgb(0xe8, 0xe0, 0xcc)
	flame     = rgb(0xff, 0x9a, 0x2a)
	glow      = rgb(0xff, 0xe0, 0x80)
	water     = rgb(0x46, 0x86, 0xc8)
	plaster   = rgb(0xdc, 0xd0, 0xb4)
	roofTile  = rgb(0x9c, 0x3c, 0x2c)
	void      = rgb(0x18, 0x14, 0x12)
)

// column returns a box from (x - r, y0, z - r) to (x + r, y1, z + r).
func column(x, z, r, y0, y1 float32, c [4]float32) model.Part {
	return model.Box([3]float32{x - r, y0, z - r}, [3]float32{x + r, y1, z + r}, c)
}

// lean returns a square column of half width r from (x0, y0, z0) to (x1,
// y1, z1), so it can lean.
func lean(x0, y0, z0, x1, y1, z1, r0, r1 float32, c [4]float32) model.Part {
	sq := func(x, y, z, r float32) [][3]float32 {
		return [][3]float32{{x - r, y, z - r}, {x + r, y, z - r}, {x + r, y, z + r}, {x - r, y, z + r}}
	}
	return model.Loft(sq(x0, y0, z0, r0), sq(x1, y1, z1, r1), c)
}

// Model returns placeholder/model: a neutral grey 1 × 1 × 1 box, used for
// every missing model.
func Model() ([]byte, error) {
	return model.Simple([]model.Part{column(0, 0, 0.5, 0, 1, rgb(0x98, 0x98, 0x9c))})
}

// Prop returns placeholder/prop/<prop>, a small untextured model sized in
// cells. It returns an error for a prop that is not in Props.
func Prop(prop string) ([]byte, error) {
	var parts []model.Part
	switch prop {
	case "tree":
		parts = []model.Part{
			column(0, 0, 0.15, 0, 1.2, bark),
			model.Cone([3]float32{0, 0.9, 0}, 0.9, 1.3, 8, leaves),
			model.Cone([3]float32{0, 1.7, 0}, 0.7, 1.3, 8, leaves),
		}
	case "pine":
		parts = []model.Part{
			column(0, 0, 0.12, 0, 1, bark),
			model.Cone([3]float32{0, 0.7, 0}, 0.65, 1.5, 7, needles),
			model.Cone([3]float32{0, 1.6, 0}, 0.5, 1.4, 7, needles),
			model.Cone([3]float32{0, 2.5, 0}, 0.35, 1.5, 7, needles),
		}
	case "bush":
		parts = []model.Part{
			model.Box([3]float32{-0.4, 0, -0.35}, [3]float32{0.35, 0.45, 0.4}, shrub),
			model.Box([3]float32{-0.1, 0.1, -0.45}, [3]float32{0.45, 0.6, 0.15}, shrub),
			model.Box([3]float32{-0.45, 0.15, -0.05}, [3]float32{0.05, 0.55, 0.45}, shrub),
		}
	case "rock":
		parts = []model.Part{model.Loft(
			[][3]float32{{-0.5, 0, -0.35}, {0.1, 0, -0.5}, {0.55, 0, -0.1}, {0.4, 0, 0.4}, {-0.2, 0, 0.5}, {-0.55, 0, 0.15}},
			[][3]float32{{-0.3, 0.45, -0.2}, {0.05, 0.55, -0.3}, {0.3, 0.5, -0.05}, {0.2, 0.4, 0.25}, {-0.1, 0.35, 0.3}, {-0.35, 0.4, 0.05}},
			stone,
		)}
	case "cactus":
		parts = []model.Part{
			column(0, 0, 0.2, 0, 2, cactus),
			model.Box([3]float32{-0.55, 0.7, -0.1}, [3]float32{-0.15, 0.9, 0.1}, cactus),
			column(-0.5, 0, 0.12, 0.7, 1.5, cactus),
			model.Box([3]float32{0.15, 1, -0.1}, [3]float32{0.55, 1.2, 0.1}, cactus),
			column(0.5, 0, 0.12, 1, 1.7, cactus),
		}
	case "palm":
		parts = []model.Part{lean(0, 0, 0, 0.25, 2.6, 0, 0.12, 0.08, palmBark)}
		// Each frond is a flat blade that widens and droops away from the
		// top of the trunk.
		blade := func(x, y, z, dx, dz, w float32) [][3]float32 {
			const t = 0.03
			return [][3]float32{{x - w*dz, y + t, z + w*dx}, {x + w*dz, y + t, z - w*dx}, {x + w*dz, y - t, z - w*dx}, {x - w*dz, y - t, z + w*dx}}
		}
		for i := range 6 {
			a := 2 * math.Pi * float64(i) / 6
			dx, dz := float32(math.Cos(a)), float32(math.Sin(a))
			parts = append(parts, model.Loft(
				blade(0.25+0.05*dx, 2.62, 0.05*dz, dx, dz, 0.06),
				blade(0.25+1.1*dx, 2.3, 1.1*dz, dx, dz, 0.22),
				frond,
			))
		}
	case "deadtree":
		parts = []model.Part{
			lean(0, 0, 0, 0.1, 2.2, 0, 0.16, 0.06, deadBark),
			lean(0.05, 1.1, 0, 0.7, 1.8, 0.1, 0.07, 0.03, deadBark),
			lean(0.05, 1.5, 0, -0.45, 2, -0.2, 0.06, 0.03, deadBark),
		}
	case "reed":
		for _, b := range [][4]float32{{0, 0, 0.1, 1.4}, {0.2, 0.1, -0.05, 1.1}, {-0.15, 0.15, 0.08, 1.25}, {0.1, -0.2, 0.12, 1}, {-0.2, -0.1, -0.1, 1.15}} {
			x, z, tilt, h := b[0], b[1], b[2], b[3]
			parts = append(parts, lean(x, 0, z, x+tilt, h, z+tilt/2, 0.025, 0.004, reedGreen))
		}
	default:
		var ok bool
		if parts, ok = dungeonProp(prop); !ok {
			if parts, ok = townProp(prop); !ok {
				return nil, fmt.Errorf("placeholder: no prop %q", prop)
			}
		}
	}
	return model.Simple(parts)
}

// box is a box from (x0, y0, z0) to (x1, y1, z1).
func box(x0, y0, z0, x1, y1, z1 float32, c [4]float32) model.Part {
	return model.Box([3]float32{x0, y0, z0}, [3]float32{x1, y1, z1}, c)
}

// dungeonProp returns the parts of a dungeon prop.
func dungeonProp(prop string) ([]model.Part, bool) {
	switch prop {
	case "stalagmite":
		return []model.Part{model.Cone([3]float32{0, 0, 0}, 0.45, 1.7, 7, stone), model.Cone([3]float32{0.3, 0, 0.2}, 0.2, 0.8, 6, stone)}, true
	case "stairs_up":
		// Three steps that rise toward the north, -Z.
		return []model.Part{
			box(-0.8, 0, -0.1, 0.8, 0.2, 0.7, masonry),
			box(-0.8, 0, -0.4, 0.8, 0.4, -0.1, masonry),
			box(-0.8, 0, -0.7, 0.8, 0.6, -0.4, masonry),
		}, true
	case "stairs_down":
		// A dark opening with a low rim.
		return []model.Part{
			box(-0.8, 0, -0.8, 0.8, 0.02, 0.8, void),
			box(-0.9, 0, -0.9, 0.9, 0.3, -0.75, masonry),
			box(-0.9, 0, 0.75, 0.9, 0.3, 0.9, masonry),
			box(-0.9, 0, -0.75, -0.75, 0.3, 0.75, masonry),
			box(0.75, 0, -0.75, 0.9, 0.3, 0.75, masonry),
		}, true
	case "altar":
		return []model.Part{box(-0.7, 0, -0.4, 0.7, 0.8, 0.4, darkStone), box(-0.8, 0.8, -0.5, 0.8, 0.95, 0.5, masonry)}, true
	case "pillar":
		return []model.Part{box(-0.45, 0, -0.45, 0.45, 0.3, 0.45, masonry), column(0, 0, 0.3, 0.3, 3, masonry), box(-0.45, 3, -0.45, 0.45, 3.2, 0.45, masonry)}, true
	case "coffin":
		return []model.Part{box(-0.35, 0, -0.8, 0.35, 0.45, 0.8, darkWood), box(-0.4, 0.45, -0.85, 0.4, 0.55, 0.85, wood)}, true
	case "bones":
		return []model.Part{
			box(-0.4, 0, -0.05, 0.4, 0.08, 0.05, bone),
			box(-0.1, 0, -0.35, 0, 0.08, 0.3, bone),
			box(0.15, 0, 0.1, 0.4, 0.32, 0.35, bone),
		}, true
	case "brazier":
		return []model.Part{
			column(0, 0, 0.06, 0, 0.9, darkStone),
			box(-0.35, 0.9, -0.35, 0.35, 1.15, 0.35, darkStone),
			model.Cone([3]float32{0, 1.15, 0}, 0.25, 0.4, 6, flame),
		}, true
	case "crate":
		return []model.Part{box(-0.45, 0, -0.45, 0.45, 0.9, 0.45, wood), box(-0.48, 0.4, -0.48, 0.48, 0.5, 0.48, darkWood)}, true
	}
	return nil, false
}

// townProp returns the parts of a town prop.
func townProp(prop string) ([]model.Part, bool) {
	switch prop {
	case "fountain":
		return []model.Part{
			model.Loft(model.Circle([3]float32{0, 0, 0}, 1.8, 8), model.Circle([3]float32{0, 0.5, 0}, 1.8, 8), masonry),
			model.Loft(model.Circle([3]float32{0, 0.5, 0}, 1.6, 8), model.Circle([3]float32{0, 0.52, 0}, 1.6, 8), water),
			column(0, 0, 0.3, 0.5, 1.6, masonry),
			model.Cone([3]float32{0, 1.6, 0}, 0.5, 0.4, 8, water),
		}, true
	case "lamp":
		return []model.Part{column(0, 0, 0.08, 0, 2.4, darkStone), box(-0.2, 2.4, -0.2, 0.2, 2.75, 0.2, glow), box(-0.25, 2.75, -0.25, 0.25, 2.85, 0.25, darkStone)}, true
	case "tower":
		return []model.Part{box(-1, 0, -1, 1, 3.6, 1, masonry), box(-1.15, 3.6, -1.15, 1.15, 3.8, 1.15, darkStone), model.Cone([3]float32{0, 3.8, 0}, 1.1, 0.7, 4, roofTile)}, true
	case "gate":
		// An arch over a road 4 cells wide, across the X axis.
		return []model.Part{
			box(-2.3, 0, -0.4, -1.9, 3.2, 0.4, masonry),
			box(1.9, 0, -0.4, 2.3, 3.2, 0.4, masonry),
			box(-2.3, 3.2, -0.4, 2.3, 3.7, 0.4, darkStone),
		}, true
	case "well":
		return []model.Part{
			model.Loft(model.Circle([3]float32{0, 0, 0}, 0.6, 8), model.Circle([3]float32{0, 0.7, 0}, 0.6, 8), masonry),
			column(-0.55, 0, 0.05, 0.7, 1.6, wood),
			column(0.55, 0, 0.05, 0.7, 1.6, wood),
			box(-0.8, 1.6, -0.5, 0.8, 1.75, 0.5, roofTile),
		}, true
	}
	return nil, false
}

// BuildingSizes are the footprint sides of the placeholder buildings,
// placeholder/building/<w>x<d>: even numbers from 2 to 12 cells.
var BuildingSizes = []int{2, 4, 6, 8, 10, 12}

// Building returns placeholder/building/<w>x<d>: a house that fills a
// footprint of w × d cells with a margin, with a pitched roof and a door
// on its south side, +Z. It returns an error for a size that is not in
// BuildingSizes.
func Building(w, d int) ([]byte, error) {
	if !slices.Contains(BuildingSizes, w) || !slices.Contains(BuildingSizes, d) {
		return nil, fmt.Errorf("placeholder: no building of %d × %d", w, d)
	}
	hw, hd := float32(w)/2-0.15, float32(d)/2-0.15
	h := 2.2 + 0.1*float32(min(w, d))
	ridge := min(0.3*float32(min(w, d)), 2.5)
	parts := []model.Part{
		box(-hw, 0, -hd, hw, h, hd, plaster),
		box(-0.45, 0, hd, 0.45, 1.5, hd+0.06, darkWood),
	}
	// The roof rises from the walls' top to a ridge along X.
	eave := float32(0.25)
	parts = append(parts, model.Loft(
		[][3]float32{{-hw - eave, h, -hd - eave}, {hw + eave, h, -hd - eave}, {hw + eave, h, hd + eave}, {-hw - eave, h, hd + eave}},
		[][3]float32{{-hw - eave, h + ridge, -0.02}, {hw + eave, h + ridge, -0.02}, {hw + eave, h + ridge, 0.02}, {-hw - eave, h + ridge, 0.02}},
		roofTile,
	))
	return model.Simple(parts)
}
