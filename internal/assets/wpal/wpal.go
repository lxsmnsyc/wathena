// Package wpal reads and writes WPAL palettes, as defined in
// docs/specs/assets-v1.md, "Palette (WPAL)".
package wpal

import (
	"errors"
)

const (
	magic   = "WPAL"
	version = 1
	// Size is the length of an encoded palette.
	Size = 4 + 2 + 256*4
)

var ErrFormat = errors.New("wpal: invalid palette")

// Palette is 256 RGBA colors. Index 0 is transparent.
type Palette [256][4]byte

// Encode writes a palette. Index 0 is always stored as transparent.
func (p *Palette) Encode() []byte {
	b := make([]byte, 0, Size)
	b = append(b, magic...)
	b = append(b, version, 0)
	for i, c := range p {
		if i == 0 {
			c[3] = 0
		}
		b = append(b, c[:]...)
	}
	return b
}

// Decode reads a palette.
func Decode(b []byte) (*Palette, error) {
	if len(b) != Size || string(b[:4]) != magic || b[4] != version || b[5] != 0 {
		return nil, ErrFormat
	}
	var p Palette
	for i := range p {
		copy(p[i][:], b[6+i*4:])
	}
	if p[0][3] != 0 {
		return nil, ErrFormat
	}
	return &p, nil
}

// FromRO converts an RO .pal file: 256 colors of R, G, B, and an unused byte.
func FromRO(b []byte) (*Palette, error) {
	if len(b) != 1024 {
		return nil, ErrFormat
	}
	var p Palette
	for i := range p {
		p[i] = [4]byte{b[i*4], b[i*4+1], b[i*4+2], 255}
	}
	p[0][3] = 0
	return &p, nil
}
