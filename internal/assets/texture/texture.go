// Package texture converts images into assets-v1 textures: lossless WebP
// with straight alpha, as defined in docs/specs/assets-v1.md.
package texture

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/webp"
)

// MaxSize is the largest width or height of a texture.
const MaxSize = 4096

var ErrFormat = errors.New("texture: invalid image")

// Format is an input image format.
type Format int

const (
	BMP Format = iota
	TGA
	PNG
	JPEG
	// WEBP is accepted from packs (assets-v1, "Pack format for original
	// art"). The GRF importer never sees it.
	WEBP
)

// FormatOf returns the format for a file extension, such as ".bmp".
func FormatOf(ext string) (Format, bool) {
	switch ext {
	case ".bmp":
		return BMP, true
	case ".tga":
		return TGA, true
	case ".png":
		return PNG, true
	case ".jpg", ".jpeg":
		return JPEG, true
	case ".webp":
		return WEBP, true
	}
	return 0, false
}

// Decode reads an image. It checks the size in the header before decoding,
// so a crafted header cannot make it allocate a huge image.
func Decode(f Format, data []byte) (*image.NRGBA, error) {
	var img image.Image
	var err error
	switch f {
	case TGA:
		return decodeTGA(data)
	case BMP:
		return decodeBMP(data)
	case PNG:
		if err := checkConfig(png.DecodeConfig(bytes.NewReader(data))); err != nil {
			return nil, err
		}
		img, err = png.Decode(bytes.NewReader(data))
	case JPEG:
		if err := checkConfig(jpeg.DecodeConfig(bytes.NewReader(data))); err != nil {
			return nil, err
		}
		img, err = jpeg.Decode(bytes.NewReader(data))
	case WEBP:
		if err := checkConfig(webp.DecodeConfig(bytes.NewReader(data))); err != nil {
			return nil, err
		}
		img, err = webp.Decode(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("%w: unknown format", ErrFormat)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n, nil
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	return out, nil
}

func checkConfig(c image.Config, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFormat, err)
	}
	return checkSize(c.Width, c.Height)
}

func checkSize(w, h int) error {
	if w <= 0 || h <= 0 || w > MaxSize || h > MaxSize {
		return fmt.Errorf("%w: image is %d × %d", ErrFormat, w, h)
	}
	return nil
}

// KeyMagenta makes magenta pixels transparent. RO draws them as holes in
// BMP textures. Like the game, it accepts colors close to magenta, since
// many files were saved with slight color errors.
func KeyMagenta(img *image.NRGBA) {
	for i := 0; i < len(img.Pix); i += 4 {
		p := img.Pix[i : i+4 : i+4]
		if p[0] > 230 && p[1] < 20 && p[2] > 230 {
			p[3] = 0
		}
	}
}

// Bleed gives fully transparent pixels the average color of their opaque
// neighbors, and black elsewhere. The client filters textures when it
// scales them, which mixes in the color of transparent pixels. Without
// this, keyed textures get magenta edges.
func Bleed(img *image.NRGBA) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	src := make([]byte, len(img.Pix))
	copy(src, img.Pix)
	for y := range h {
		for x := range w {
			i := y*img.Stride + x*4
			if src[i+3] != 0 {
				continue
			}
			var r, g, b, n int
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					j := ny*img.Stride + nx*4
					if src[j+3] == 0 {
						continue
					}
					r, g, b, n = r+int(src[j]), g+int(src[j+1]), b+int(src[j+2]), n+1
				}
			}
			if n == 0 {
				img.Pix[i], img.Pix[i+1], img.Pix[i+2] = 0, 0, 0
				continue
			}
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = byte(r/n), byte(g/n), byte(b/n) // #nosec G115 -- averages of bytes.
		}
	}
}

// Encode writes an image as lossless WebP.
func Encode(img *image.NRGBA) ([]byte, error) {
	if err := checkSize(img.Rect.Dx(), img.Rect.Dy()); err != nil {
		return nil, err
	}
	return encodeWebP(img, Level)
}

// Convert decodes an image, keys magenta in BMP files, and encodes it as
// lossless WebP.
func Convert(f Format, data []byte) ([]byte, error) {
	img, err := Decode(f, data)
	if err != nil {
		return nil, err
	}
	if f == BMP {
		KeyMagenta(img)
	}
	Bleed(img)
	return Encode(img)
}
