package placeholder

import (
	"image"
	"image/color"
	"math"
)

// GroundMaterials are the materials of the placeholder ground textures,
// placeholder/ground/<material>.
var GroundMaterials = []string{"grass", "dirt", "sand", "rock", "snow", "mud", "water"}

// groundTones are each material's dark and light colors.
var groundTones = map[string][2]color.NRGBA{
	"grass": {{0x3e, 0x7a, 0x2c, 255}, {0x6a, 0xa8, 0x44, 255}},
	"dirt":  {{0x6e, 0x4a, 0x2a, 255}, {0x96, 0x6c, 0x44, 255}},
	"sand":  {{0xd2, 0xc0, 0x84, 255}, {0xec, 0xde, 0xaa, 255}},
	"rock":  {{0x5c, 0x5e, 0x62, 255}, {0x86, 0x88, 0x8c, 255}},
	"snow":  {{0xd8, 0xe0, 0xea, 255}, {0xf6, 0xf8, 0xfc, 255}},
	"mud":   {{0x3a, 0x2a, 0x1c, 255}, {0x56, 0x40, 0x2c, 255}},
	"water": {{0x24, 0x5a, 0x9c, 255}, {0x46, 0x86, 0xc8, 255}},
}

// Ground returns placeholder/ground/<material>: a 64 × 64 soft noise of
// two tones of the material's color. Every wave repeats a whole number
// of times across the tile, so it tiles seamlessly. It returns nil for a
// material that is not in GroundMaterials.
func Ground(material string) *image.NRGBA {
	tones, ok := groundTones[material]
	if !ok {
		return nil
	}
	const size = 64
	// Each wave is a direction in whole cycles per tile, a weight, and a
	// phase. Water uses long waves along x, the others a rough mix.
	type wave struct{ fx, fy, w, phase float64 }
	waves := []wave{{3, 1, 0.22, 0}, {-2, 5, 0.16, 1}, {7, 4, 0.1, 2}, {1, -9, 0.06, 3}}
	if material == "water" {
		waves = []wave{{1, 4, 0.28, 0}, {-1, 6, 0.12, 1}, {2, 9, 0.06, 2}}
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			t := 0.5
			for _, w := range waves {
				t += w.w * math.Sin(2*math.Pi*(w.fx*float64(x)+w.fy*float64(y))/size+w.phase)
			}
			// A small fixed grain per pixel.
			h := uint32(x*73856093) ^ uint32(y*19349663) // #nosec G115 -- x and y are below 64.
			h = (h ^ h>>13) * 0x5bd1e995
			t += (float64(h>>24)/255 - 0.5) * 0.12
			t = min(max(t, 0), 1)
			lerp := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + t*(float64(b)-float64(a)))) }
			img.SetNRGBA(x, y, color.NRGBA{
				lerp(tones[0].R, tones[1].R),
				lerp(tones[0].G, tones[1].G),
				lerp(tones[0].B, tones[1].B),
				255,
			})
		}
	}
	return img
}
