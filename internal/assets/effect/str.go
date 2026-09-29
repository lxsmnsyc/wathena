// Package effect reads RO's STR effects and writes assets-v1 effects
// (WEFF), as defined in docs/specs/assets-v1.md, "Effect (WEFF)".
package effect

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"golang.org/x/text/encoding/korean"
)

var ErrFormat = errors.New("effect: invalid effect")

// Parsing limits. Real effects are far below them.
const (
	maxLayers   = 1024
	maxTextures = 1024
	maxKeys     = 1 << 16
	// MaxFrames is the longest effect, in frames.
	MaxFrames = 1 << 14
	// MaxFrameRecords is the most layers × frames an effect can have. The
	// largest kRO effect has 60 layers of 620 frames.
	MaxFrameRecords = 1 << 18
)

// STR is a parsed STR effect.
type STR struct {
	FPS    int
	MaxKey int
	Layers []Layer
}

// Layer is one textured quad with its keyframes.
type Layer struct {
	// Textures are file names in the effect's folder.
	Textures []string
	Keys     []Key
}

// Key is a keyframe. A type 0 key sets values at its frame. A type 1 key
// holds changes per frame, added to the type 0 key before it.
type Key struct {
	Frame    int32
	Type     uint32
	Pos      [2]float32
	UV       [8]float32
	XY       [8]float32
	AniFrame float32
	AniType  uint32
	Delay    float32
	// Angle is in degrees.
	Angle float32
	// Color is R, G, B, A from 0 to 255.
	Color    [4]float32
	SrcBlend uint32
	DstBlend uint32
	MTPreset uint32
}

type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b)-r.p {
		if r.err == nil {
			r.err = fmt.Errorf("%w: file is truncated", ErrFormat)
		}
		return nil
	}
	out := r.b[r.p : r.p+n]
	r.p += n
	return out
}

func (r *reader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }

func (r *reader) count(limit, itemSize int) int {
	n := r.u32()
	if r.err == nil && (n > uint32(limit) || int(n)*itemSize > len(r.b)-r.p) { // #nosec G115 -- limit is small.
		r.err = fmt.Errorf("%w: count %d", ErrFormat, n)
	}
	if r.err != nil {
		return 0
	}
	return int(n)
}

// keySize is the size of one keyframe in bytes.
const keySize = 4 + 4 + 8 + 32 + 32 + 4 + 4 + 4 + 4 + 16 + 12

// ParseSTR reads an STR file. Only version 0x94 exists.
func ParseSTR(b []byte) (*STR, error) {
	r := &reader{b: b}
	if string(r.take(4)) != "STRM" {
		return nil, fmt.Errorf("%w: not an str file", ErrFormat)
	}
	if v := r.u32(); v != 0x94 {
		return nil, fmt.Errorf("%w: str version %#x", ErrFormat, v)
	}
	s := &STR{FPS: int(r.u32()), MaxKey: int(r.u32())}
	layers := r.count(maxLayers, 8)
	r.take(16)
	if r.err != nil {
		return nil, r.err
	}
	if s.FPS < 1 || s.FPS > 1000 || s.MaxKey < 0 || s.MaxKey >= MaxFrames {
		return nil, fmt.Errorf("%w: %d fps, %d frames", ErrFormat, s.FPS, s.MaxKey)
	}
	for range layers {
		var l Layer
		for range r.count(maxTextures, 128) {
			l.Textures = append(l.Textures, decodeName(r.take(128)))
		}
		for range r.count(maxKeys, keySize) {
			k := Key{Frame: int32(r.u32()), Type: r.u32()} // #nosec G115 -- reinterprets the bits.
			k.Pos = [2]float32{r.f32(), r.f32()}
			for i := range k.UV {
				k.UV[i] = r.f32()
			}
			for i := range k.XY {
				k.XY[i] = r.f32()
			}
			k.AniFrame, k.AniType, k.Delay = r.f32(), r.u32(), r.f32()
			// Angles are stored in 1024ths of a turn.
			k.Angle = r.f32() * 360 / 1024
			for i := range k.Color {
				k.Color[i] = r.f32()
			}
			k.SrcBlend, k.DstBlend, k.MTPreset = r.u32(), r.u32(), r.u32()
			l.Keys = append(l.Keys, k)
		}
		if r.err != nil {
			return nil, r.err
		}
		s.Layers = append(s.Layers, l)
	}
	return s, nil
}

func decodeName(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	s, err := korean.EUCKR.NewDecoder().String(string(b))
	if err != nil {
		return strings.ToValidUTF8(string(b), "")
	}
	return s
}
