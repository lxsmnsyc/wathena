package texture

import (
	"encoding/binary"
	"fmt"
	"image"
)

// TGA image types.
const (
	tgaColorMapped    = 1
	tgaTrueColor      = 2
	tgaGray           = 3
	tgaRLEColorMapped = 9
	tgaRLETrueColor   = 10
	tgaRLEGray        = 11
)

// decodeTGA reads uncompressed and run-length encoded TGA images with 8, 16,
// 24, or 32 bits per pixel, which covers the files in RO clients.
func decodeTGA(b []byte) (*image.NRGBA, error) {
	if len(b) < 18 {
		return nil, fmt.Errorf("%w: tga is truncated", ErrFormat)
	}
	idLen := int(b[0])
	mapType, imgType := b[1], b[2]
	mapFirst := int(binary.LittleEndian.Uint16(b[3:]))
	mapLen := int(binary.LittleEndian.Uint16(b[5:]))
	mapDepth := int(b[7])
	w := int(binary.LittleEndian.Uint16(b[12:]))
	h := int(binary.LittleEndian.Uint16(b[14:]))
	depth := int(b[16])
	desc := b[17]
	if err := checkSize(w, h); err != nil {
		return nil, err
	}
	p := 18 + idLen
	if p > len(b) {
		return nil, fmt.Errorf("%w: tga is truncated", ErrFormat)
	}

	// The color map comes before the pixels, even in images that do not use it.
	var cmap [][4]byte
	if mapType == 1 {
		n := mapLen * ((mapDepth + 7) / 8)
		if mapDepth != 15 && mapDepth != 16 && mapDepth != 24 && mapDepth != 32 || p+n > len(b) {
			return nil, fmt.Errorf("%w: tga color map", ErrFormat)
		}
		for i := range mapLen {
			cmap = append(cmap, tgaColor(b[p+i*((mapDepth+7)/8):], mapDepth, desc&0x0f != 0))
		}
		p += n
	}

	var bpp int
	switch imgType {
	case tgaTrueColor, tgaRLETrueColor:
		if depth != 15 && depth != 16 && depth != 24 && depth != 32 {
			return nil, fmt.Errorf("%w: tga depth %d", ErrFormat, depth)
		}
	case tgaGray, tgaRLEGray:
		if depth != 8 && depth != 16 {
			return nil, fmt.Errorf("%w: tga gray depth %d", ErrFormat, depth)
		}
	case tgaColorMapped, tgaRLEColorMapped:
		if depth != 8 || cmap == nil {
			return nil, fmt.Errorf("%w: tga color-mapped depth %d", ErrFormat, depth)
		}
	default:
		return nil, fmt.Errorf("%w: tga type %d", ErrFormat, imgType)
	}
	bpp = (depth + 7) / 8

	// Read w × h pixels into raw, expanding runs.
	raw := make([]byte, 0, w*h*bpp)
	data := b[p:]
	if imgType >= tgaRLEColorMapped {
		q := 0
		for len(raw) < w*h*bpp {
			if q >= len(data) {
				return nil, fmt.Errorf("%w: tga is truncated", ErrFormat)
			}
			hdr := data[q]
			q++
			n := int(hdr&0x7f) + 1
			n = min(n, (w*h*bpp-len(raw))/bpp)
			if hdr&0x80 != 0 {
				if q+bpp > len(data) {
					return nil, fmt.Errorf("%w: tga is truncated", ErrFormat)
				}
				for range n {
					raw = append(raw, data[q:q+bpp]...)
				}
				q += bpp
			} else {
				if q+n*bpp > len(data) {
					return nil, fmt.Errorf("%w: tga is truncated", ErrFormat)
				}
				raw = append(raw, data[q:q+n*bpp]...)
				q += n * bpp
			}
		}
	} else {
		if len(data) < w*h*bpp {
			return nil, fmt.Errorf("%w: tga is truncated", ErrFormat)
		}
		raw = append(raw, data[:w*h*bpp]...)
	}

	// Rows are stored bottom first unless bit 5 of the descriptor is set.
	// Bit 4 stores columns right to left.
	topDown := desc&0x20 != 0
	rightToLeft := desc&0x10 != 0
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		sy := y
		if !topDown {
			sy = h - 1 - y
		}
		for x := range w {
			sx := x
			if rightToLeft {
				sx = w - 1 - x
			}
			px := raw[(sy*w+sx)*bpp:]
			var c [4]byte
			switch imgType {
			case tgaGray, tgaRLEGray:
				c = [4]byte{px[0], px[0], px[0], 255}
				if depth == 16 {
					c[3] = px[1]
				}
			case tgaColorMapped, tgaRLEColorMapped:
				i := int(px[0]) - mapFirst
				if i >= 0 && i < len(cmap) {
					c = cmap[i]
				}
			default:
				c = tgaColor(px, depth, desc&0x0f != 0)
			}
			copy(img.Pix[y*img.Stride+x*4:], c[:])
		}
	}
	return img, nil
}

// tgaColor reads one BGR or BGRA color. 16-bit colors use their top bit as
// alpha only if the header says the image has alpha bits.
func tgaColor(px []byte, depth int, alpha bool) [4]byte {
	switch depth {
	case 15, 16:
		v := uint16(px[0]) | uint16(px[1])<<8
		r, g, b := byte(v>>10&31), byte(v>>5&31), byte(v&31)
		a := byte(255)
		if depth == 16 && alpha && v&0x8000 == 0 {
			a = 0
		}
		return [4]byte{r<<3 | r>>2, g<<3 | g>>2, b<<3 | b>>2, a}
	case 24:
		return [4]byte{px[2], px[1], px[0], 255}
	}
	return [4]byte{px[2], px[1], px[0], px[3]}
}
