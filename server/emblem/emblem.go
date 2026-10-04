// Package emblem checks and re-encodes guild emblems (strategy section
// 7.6, decision 0021).
//
// Rules:
//   - An upload is at most MaxUpload bytes, as rAthena's web emblem
//     controller's MAX_EMBLEM_SIZE.
//   - It is a BMP, as the game client sends, a GIF, as the web emblem
//     controller also takes, or a PNG. Only its first frame counts.
//   - It is 24 by 24 pixels, the size the client draws.
//   - Magenta, 255 0 255, is transparent, as the client draws emblems.
//   - The server keeps and serves only its own PNG of the decoded pixels.
//     The uploaded file is never stored or served.
package emblem

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	// The formats an upload may have.
	_ "image/gif"

	_ "golang.org/x/image/bmp"
)

// MaxUpload is the largest upload in bytes.
const MaxUpload = 50000

// Size is the width and height of an emblem in pixels.
const Size = 24

// Errors of Process.
var (
	ErrTooLarge = errors.New("the emblem is larger than 50,000 bytes")
	ErrFormat   = errors.New("the emblem must be a BMP, GIF or PNG image")
	ErrSize     = errors.New("the emblem must be 24 by 24 pixels")
)

// Process checks an uploaded emblem and returns the PNG the server keeps.
func Process(data []byte) ([]byte, error) {
	if len(data) > MaxUpload {
		return nil, ErrTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "bmp" && format != "gif" && format != "png" {
		return nil, ErrFormat
	}
	if cfg.Width != Size || cfg.Height != Size {
		return nil, ErrSize
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrFormat
	}
	b := src.Bounds()
	if b.Dx() != Size || b.Dy() != Size {
		return nil, ErrSize
	}
	out := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	for y := range Size {
		for x := range Size {
			c := out.NRGBAAt(x, y)
			if c.R == 255 && c.G == 0 && c.B == 255 {
				out.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
