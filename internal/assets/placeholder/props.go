package placeholder

import (
	"fmt"
	"math"

	"github.com/lxsmnsyc/wathena/internal/assets/model"
)

// Props are the props with a placeholder model, placeholder/prop/<prop>.
var Props = []string{"tree", "pine", "bush", "rock", "cactus", "palm", "deadtree", "reed"}

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
		return nil, fmt.Errorf("placeholder: no prop %q", prop)
	}
	return model.Simple(parts)
}
