// Package placeholder generates the placeholder pack from assets-v1,
// "Placeholders". The art is drawn by code, so it is original and CC0.
package placeholder

import (
	"image"
	"image/color"
	"math"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

// License is the license of every placeholder asset.
const License = "CC0-1.0"

// Sprite returns placeholder/sprite: a grey rounded shape with a darker
// outline and a shadow, 24 × 40 pixels, used for every missing sprite.
func Sprite() *wspr.Sprite {
	const w, h = 24, 40
	pix := make([]byte, w*h)
	for y := range h {
		for x := range w {
			// A capsule: a rectangle with round ends.
			dx := float64(x) - (w-1)/2.0
			var dy float64
			switch {
			case y < w/2:
				dy = float64(y) - (w/2 - 1)
			case y >= h-w/2:
				dy = float64(y) - (h - w/2)
			}
			d := dx*dx + dy*dy
			r := float64(w) / 2
			switch {
			case d <= (r-2)*(r-2):
				pix[y*w+x] = 1
			case d <= r*r:
				pix[y*w+x] = 2
			}
		}
	}
	shadow := make([]byte, 20*6)
	for y := range 6 {
		for x := range 20 {
			dx, dy := (float64(x)-9.5)/10, (float64(y)-2.5)/3
			if dx*dx+dy*dy <= 1 {
				shadow[y*20+x] = 3
			}
		}
	}
	pages, images, err := wspr.Pack(wspr.Indexed, []wspr.Source{
		{Width: w, Height: h, Pix: pix},
		{Width: 20, Height: 6, Pix: shadow},
	})
	if err != nil {
		panic(err) // The inputs are fixed, so this cannot fail.
	}
	var pal [256][4]byte
	pal[1] = [4]byte{160, 160, 168, 255}
	pal[2] = [4]byte{70, 70, 80, 255}
	pal[3] = [4]byte{0, 0, 0, 96}
	frame := wspr.Frame{Event: -1, Layers: []wspr.Layer{
		{Image: 1, X: 0, Y: -1, Color: [4]byte{255, 255, 255, 255}, ScaleX: 1, ScaleY: 1},
		{Image: 0, X: 0, Y: -h / 2, Color: [4]byte{255, 255, 255, 255}, ScaleX: 1, ScaleY: 1},
	}}
	// Every action and direction shows the same still frame. Characters
	// use 13 actions and monsters 5, so 13 × 8 covers both.
	actions := make([]wspr.Action, 13*8)
	for i := range actions {
		actions[i] = wspr.Action{IntervalMs: 150, Frames: []wspr.Frame{frame}}
	}
	return &wspr.Sprite{Pages: pages, Palette: &pal, Images: images, Actions: actions}
}

// Texture returns placeholder/texture: a 64 × 64 grey checkerboard of
// 8-pixel squares, used for every missing texture. It tiles seamlessly.
func Texture() *image.NRGBA {
	const size, square = 64, 8
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			v := uint8(0x70)
			if (x/square+y/square)%2 == 0 {
				v = 0x98
			}
			img.SetNRGBA(x, y, color.NRGBA{v, v, v, 255})
		}
	}
	return img
}

// mapSize is the placeholder map's width and height in cells.
const mapSize = 24

// hill is the placeholder map's height at a point in cells, a low hill in
// the middle so the ground's lighting shows.
func hill(x, y float64) float32 {
	dx, dy := x-mapSize/2, y-mapSize/2
	return float32(1.5 * math.Exp(-(dx*dx+dy*dy)/40))
}

// Map returns placeholder/map and its lightmap, placeholder/lightmap: 24 ×
// 24 walkable cells of the placeholder texture, with a low hill. The
// client can run with it and no other assets, and CI renders it.
func Map() (*wmap.Map, *image.NRGBA) {
	m := &wmap.Map{Width: mapSize, Height: mapSize}
	m.Cells = make([]wmap.Cell, mapSize*mapSize)
	for y := range mapSize {
		for x := range mapSize {
			c := &m.Cells[y*mapSize+x]
			c.Flags = wmap.Walkable | wmap.Shootable
			for k := range 4 {
				c.Heights[k] = hill(float64(x+k&1), float64(y+k>>1))
			}
		}
	}
	const n = mapSize / 2
	g := &m.Ground
	g.Width, g.Height = n, n
	g.Textures = []string{"placeholder/texture"}
	g.Tiles = []wmap.Tile{{
		U:     [4]float32{0, 1, 0, 1},
		V:     [4]float32{1, 1, 0, 0},
		Color: [4]uint8{255, 255, 255, 255},
	}}
	g.Surfaces = make([]wmap.Surface, n*n)
	for j := range n {
		for i := range n {
			s := &g.Surfaces[j*n+i]
			s.Top, s.North, s.East = 0, -1, -1
			for k := range 4 {
				s.Heights[k] = hill(float64(2*i+2*(k&1)), float64(2*j+2*(k>>1)))
			}
		}
	}
	g.Lightmap, g.LightmapCount, g.LightmapColumns = "placeholder/lightmap", 1, 1
	m.Light = wmap.Light{
		Longitude: 45, Latitude: 45,
		Diffuse:   [3]float32{1, 1, 1},
		Ambient:   [3]float32{0.3, 0.3, 0.3},
		Opacity:   1,
		Direction: [3]float32{-0.5, 0.7071, 0.5},
	}
	m.Deps = []string{"placeholder/lightmap", "placeholder/texture"}
	// One lightmap block with no baked light and no shadow.
	lm := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 3; i < len(lm.Pix); i += 4 {
		lm.Pix[i] = 255
	}
	return m, lm
}
