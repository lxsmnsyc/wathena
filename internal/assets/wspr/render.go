package wspr

import (
	"image"
	"image/color"
)

// RenderFrame draws one frame onto a size × size image, with the frame's
// origin at the center. It applies position, mirroring, and color, but not
// scale or rotation. Tools use it for previews and tests, not the client.
func (s *Sprite) RenderFrame(action, frame, size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	if action >= len(s.Actions) || frame >= len(s.Actions[action].Frames) {
		return img
	}
	for _, l := range s.Actions[action].Frames[frame].Layers {
		if l.Image < 0 {
			continue
		}
		im := s.Images[l.Image]
		if im.Width == 0 || im.Height == 0 {
			continue
		}
		p := s.Pages[im.Page]
		// A layer's position is the center of its image.
		x0 := size/2 + int(l.X) - int(im.Width)/2
		y0 := size/2 + int(l.Y) - int(im.Height)/2
		for y := range int(im.Height) {
			for x := range int(im.Width) {
				sx := x
				if l.Mirror {
					sx = int(im.Width) - 1 - x
				}
				var c color.NRGBA
				off := (int(im.Y)+y)*int(p.Width) + int(im.X) + sx
				if p.Kind == Indexed {
					i := p.Pix[off]
					if i == 0 {
						continue
					}
					pc := s.Palette[i]
					c = color.NRGBA{pc[0], pc[1], pc[2], pc[3]}
				} else {
					px := p.Pix[off*4:]
					c = color.NRGBA{px[0], px[1], px[2], px[3]}
					if c.A == 0 {
						continue
					}
				}
				c.R = mul8(c.R, l.Color[0])
				c.G = mul8(c.G, l.Color[1])
				c.B = mul8(c.B, l.Color[2])
				c.A = mul8(c.A, l.Color[3])
				img.SetNRGBA(x0+x, y0+y, c)
			}
		}
	}
	return img
}

// mul8 multiplies two 0 to 255 values as fractions of 255.
func mul8(a, b uint8) uint8 {
	return uint8(uint16(a) * uint16(b) / 255) // #nosec G115 -- at most 255.
}
