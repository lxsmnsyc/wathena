package effect

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	magic   = "WEFF"
	version = 1
	// MaxBytes is the largest encoded effect.
	MaxBytes = 8 << 20
	// NoTexture marks a frame without a texture.
	NoTexture = 0xffff
	// frameSize is the size of one frame record.
	frameSize = 1 + 2 + 2 + 4 + 8 + 4 + 32
)

// Effect is a baked effect: for each layer, its state at every frame.
type Effect struct {
	FPS      int
	Frames   int
	Textures []string
	// Layers hold Frames states each.
	Layers [][]Frame
}

// Frame is a layer's state at one frame. Distances are in cells, on a
// plane facing the camera, with X right and Y up from the effect's
// anchor.
type Frame struct {
	Visible bool
	// Texture indexes Effect.Textures, or is NoTexture.
	Texture uint16
	// SrcBlend and DstBlend are Direct3D blend codes, 1 to 15.
	SrcBlend, DstBlend uint8
	Color              [4]uint8
	Offset             [2]float32
	// Angle is in degrees, clockwise.
	Angle float32
	// Corners are top-left, top-right, bottom-right, and bottom-left,
	// relative to Offset, before the rotation.
	Corners [4][2]float32
}

// Validate checks limits that Encode relies on.
func (e *Effect) Validate() error {
	if e.FPS < 1 || e.FPS > math.MaxUint16 || e.Frames < 1 || e.Frames > MaxFrames || len(e.Layers) > maxLayers || len(e.Layers)*e.Frames > MaxFrameRecords || len(e.Textures) >= NoTexture {
		return fmt.Errorf("%w: %d fps, %d frames, %d layers", ErrFormat, e.FPS, e.Frames, len(e.Layers))
	}
	for _, t := range e.Textures {
		if len(t) > 255 || !utf8.ValidString(t) {
			return fmt.Errorf("%w: texture name %q", ErrFormat, t)
		}
	}
	for _, l := range e.Layers {
		if len(l) != e.Frames {
			return fmt.Errorf("%w: layer has %d frames", ErrFormat, len(l))
		}
		for _, f := range l {
			if f.Visible && f.Texture != NoTexture && int(f.Texture) >= len(e.Textures) {
				return fmt.Errorf("%w: texture %d", ErrFormat, f.Texture)
			}
		}
	}
	return nil
}

// Encode writes an effect.
func (e *Effect) Encode() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	b := make([]byte, 0, 16+len(e.Layers)*e.Frames*frameSize)
	b = append(b, magic...)
	b = binary.LittleEndian.AppendUint16(b, version)
	b = binary.LittleEndian.AppendUint16(b, uint16(e.FPS))           // #nosec G115 -- checked by Validate.
	b = binary.LittleEndian.AppendUint32(b, uint32(e.Frames))        // #nosec G115 -- as above.
	b = binary.LittleEndian.AppendUint16(b, uint16(len(e.Textures))) // #nosec G115 -- as above.
	for _, t := range e.Textures {
		b = append(b, uint8(len(t))) // #nosec G115 -- as above.
		b = append(b, t...)
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(e.Layers))) // #nosec G115 -- as above.
	f32 := func(v float32) { b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v)) }
	for _, l := range e.Layers {
		for _, f := range l {
			if !f.Visible {
				// Hidden frames are all zeros, which compress well.
				b = append(b, make([]byte, frameSize)...)
				continue
			}
			b = append(b, 1)
			b = binary.LittleEndian.AppendUint16(b, f.Texture)
			b = append(b, f.SrcBlend, f.DstBlend)
			b = append(b, f.Color[:]...)
			f32(f.Offset[0])
			f32(f.Offset[1])
			f32(f.Angle)
			for _, c := range f.Corners {
				f32(c[0])
				f32(c[1])
			}
		}
	}
	if len(b) > MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrFormat, len(b))
	}
	return b, nil
}

// Decode reads an effect.
func Decode(b []byte) (*Effect, error) {
	if len(b) < 14 || string(b[:4]) != magic || binary.LittleEndian.Uint16(b[4:]) != version {
		return nil, fmt.Errorf("%w: header", ErrFormat)
	}
	e := &Effect{FPS: int(binary.LittleEndian.Uint16(b[6:])), Frames: int(binary.LittleEndian.Uint32(b[8:]))}
	if e.Frames < 1 || e.Frames > MaxFrames {
		return nil, fmt.Errorf("%w: %d frames", ErrFormat, e.Frames)
	}
	p := 14
	n := int(binary.LittleEndian.Uint16(b[12:]))
	for range n {
		if p >= len(b) || p+1+int(b[p]) > len(b) {
			return nil, fmt.Errorf("%w: texture names", ErrFormat)
		}
		e.Textures = append(e.Textures, string(b[p+1:p+1+int(b[p])]))
		p += 1 + int(b[p])
	}
	if p+2 > len(b) {
		return nil, fmt.Errorf("%w: truncated", ErrFormat)
	}
	layers := int(binary.LittleEndian.Uint16(b[p:]))
	p += 2
	if layers*e.Frames > MaxFrameRecords || layers*e.Frames*frameSize != len(b)-p {
		return nil, fmt.Errorf("%w: frame data is %d bytes", ErrFormat, len(b)-p)
	}
	f32 := func() float32 {
		v := math.Float32frombits(binary.LittleEndian.Uint32(b[p:]))
		p += 4
		return v
	}
	for range layers {
		l := make([]Frame, e.Frames)
		for i := range l {
			f := &l[i]
			f.Visible = b[p] == 1
			f.Texture = binary.LittleEndian.Uint16(b[p+1:])
			f.SrcBlend, f.DstBlend = b[p+3], b[p+4]
			copy(f.Color[:], b[p+5:p+9])
			p += 9
			f.Offset = [2]float32{f32(), f32()}
			f.Angle = f32()
			for k := range f.Corners {
				f.Corners[k] = [2]float32{f32(), f32()}
			}
			if !f.Visible {
				*f = Frame{}
			}
		}
		e.Layers = append(e.Layers, l)
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return e, nil
}
