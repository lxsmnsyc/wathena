// Package placeholder generates the placeholder pack from assets-v1,
// "Placeholders". The art is drawn by code, so it is original and CC0.
package placeholder

import (
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
