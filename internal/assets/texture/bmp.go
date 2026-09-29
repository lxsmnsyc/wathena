package texture

import (
	"encoding/binary"
	"fmt"
	"image"
	"math/bits"
)

// BMP compression values.
const (
	biRGB       = 0
	biBitfields = 3
)

// decodeBMP reads uncompressed BMP files with 1, 4, 8, 16, 24, or 32 bits
// per pixel. It is more lenient than golang.org/x/image/bmp in two ways
// that RO's files need:
//
//   - 16-bit images, stored as 5 bits per channel.
//   - Palette indexes past the end of the palette, which become black.
func decodeBMP(b []byte) (*image.NRGBA, error) {
	if len(b) < 26 || string(b[:2]) != "BM" {
		return nil, fmt.Errorf("%w: not a bmp file", ErrFormat)
	}
	off := int(binary.LittleEndian.Uint32(b[10:]))
	hdr := int(binary.LittleEndian.Uint32(b[14:]))
	var w, h, bpp, comp int
	switch {
	case hdr == 12:
		w = int(binary.LittleEndian.Uint16(b[18:]))
		h = int(int16(binary.LittleEndian.Uint16(b[20:]))) // #nosec G115 -- reinterprets the bits.
		bpp = int(binary.LittleEndian.Uint16(b[24:]))
	case hdr >= 40 && len(b) >= 14+hdr:
		w = int(int32(binary.LittleEndian.Uint32(b[18:]))) // #nosec G115 -- reinterprets the bits.
		h = int(int32(binary.LittleEndian.Uint32(b[22:]))) // #nosec G115 -- reinterprets the bits.
		bpp = int(binary.LittleEndian.Uint16(b[28:]))
		comp = int(binary.LittleEndian.Uint32(b[30:]))
	default:
		return nil, fmt.Errorf("%w: bmp header size %d", ErrFormat, hdr)
	}
	// A negative height means rows are stored top first.
	topDown := h < 0
	if topDown {
		h = -h
	}
	if err := checkSize(w, h); err != nil {
		return nil, err
	}

	// Channel masks for 16- and 32-bit images.
	masks := [4]uint32{0x7c00, 0x03e0, 0x001f, 0}
	if bpp == 32 {
		masks = [4]uint32{0xff0000, 0xff00, 0xff, 0}
	}
	switch comp {
	case biRGB:
	case biBitfields:
		if bpp != 16 && bpp != 32 {
			return nil, fmt.Errorf("%w: bmp bitfields with %d bits", ErrFormat, bpp)
		}
		// The masks follow a 40-byte header, or are part of a larger one.
		p := 14 + 40
		n := 3
		if hdr >= 56 {
			n = 4
		}
		if len(b) < p+4*n {
			return nil, fmt.Errorf("%w: bmp is truncated", ErrFormat)
		}
		for i := range n {
			masks[i] = binary.LittleEndian.Uint32(b[p+4*i:])
		}
	default:
		return nil, fmt.Errorf("%w: bmp compression %d", ErrFormat, comp)
	}

	var pal [][4]byte
	if bpp <= 8 {
		entry := 4
		if hdr == 12 {
			entry = 3
		}
		n := 1 << bpp
		if hdr >= 40 {
			if used := int(binary.LittleEndian.Uint32(b[46:])); used > 0 && used < n {
				n = used
			}
		}
		p := 14 + hdr
		for i := 0; i < n && p+entry*(i+1) <= len(b); i++ {
			c := b[p+entry*i:]
			pal = append(pal, [4]byte{c[2], c[1], c[0], 255})
		}
	} else if bpp != 16 && bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("%w: bmp with %d bits per pixel", ErrFormat, bpp)
	}

	stride := (w*bpp + 31) / 32 * 4
	if off < 14 || off > len(b) || len(b)-off < stride*h {
		return nil, fmt.Errorf("%w: bmp is truncated", ErrFormat)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		sy := h - 1 - y
		if topDown {
			sy = y
		}
		row := b[off+sy*stride : off+(sy+1)*stride]
		dst := img.Pix[y*img.Stride:]
		for x := range w {
			var c [4]byte
			switch bpp {
			case 1, 4, 8:
				bit := x * bpp
				i := int(row[bit/8]>>(8-bpp-bit%8)) & (1<<bpp - 1)
				if i < len(pal) {
					c = pal[i]
				} else {
					c = [4]byte{0, 0, 0, 255}
				}
			case 24:
				c = [4]byte{row[x*3+2], row[x*3+1], row[x*3], 255}
			case 16:
				c = maskColor(uint32(binary.LittleEndian.Uint16(row[x*2:])), masks)
			case 32:
				c = maskColor(binary.LittleEndian.Uint32(row[x*4:]), masks)
			}
			copy(dst[x*4:], c[:])
		}
	}
	return img, nil
}

// maskColor extracts channels with bit masks and scales them to 8 bits.
// Without an alpha mask the pixel is opaque.
func maskColor(v uint32, masks [4]uint32) [4]byte {
	c := [4]byte{0, 0, 0, 255}
	for i, m := range masks {
		if m == 0 {
			continue
		}
		c[i] = scaleTo8((v&m)>>bits.TrailingZeros32(m), bits.OnesCount32(m))
	}
	return c
}

// scaleTo8 scales an n-bit value to 0 to 255.
func scaleTo8(v uint32, n int) byte {
	if n >= 8 {
		return byte(v >> (n - 8)) // #nosec G115 -- 8 bits remain.
	}
	return byte(v * 255 / (1<<n - 1)) // #nosec G115 -- at most 255.
}
