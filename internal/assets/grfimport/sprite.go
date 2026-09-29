// Package grfimport converts files from a game client into the asset
// contract (docs/specs/assets-v1.md). Game files can be crafted, so every
// parser checks lengths before reading.
package grfimport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"

	"golang.org/x/text/encoding/korean"

	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

var ErrFormat = errors.New("grfimport: invalid file")

// Parsing limits. Real sprites are far below them.
const (
	maxImages  = 4096
	maxActions = 4096
	maxFrames  = 4096
	maxLayers  = 1024
	maxAnchors = 64
	maxEvents  = 4096
	// defaultIntervalMs is RO's frame time for act files older than 2.2.
	defaultIntervalMs = 150
)

type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b)-r.p {
		r.err = ErrFormat
		return nil
	}
	out := r.b[r.p : r.p+n]
	r.p += n
	return out
}

func (r *reader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *reader) u16() uint16 {
	if b := r.take(2); b != nil {
		return binary.LittleEndian.Uint16(b)
	}
	return 0
}

func (r *reader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *reader) i32() int32   { return int32(r.u32()) } // #nosec G115 -- reinterprets the bits.
func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }
func (r *reader) left() int    { return len(r.b) - r.p }

// spr is a parsed RO sprite image file.
type spr struct {
	indexed []wspr.Source
	rgba    []wspr.Source
	palette *[256][4]byte
}

// parseSpr reads an spr file. Versions 1.0 to 2.1 are supported.
// Version 2.1 compresses runs of index 0 as a 0 byte and a count.
func parseSpr(b []byte) (*spr, error) {
	r := &reader{b: b}
	if string(r.take(2)) != "SP" {
		return nil, fmt.Errorf("%w: not an spr file", ErrFormat)
	}
	minor, major := r.u8(), r.u8()
	ver := int(major)<<8 | int(minor)
	if ver < 0x100 || ver > 0x201 {
		return nil, fmt.Errorf("%w: spr version %d.%d", ErrFormat, major, minor)
	}
	indexedCount := int(r.u16())
	rgbaCount := 0
	if ver >= 0x200 {
		rgbaCount = int(r.u16())
	}
	if indexedCount+rgbaCount > maxImages {
		return nil, fmt.Errorf("%w: %d images", ErrFormat, indexedCount+rgbaCount)
	}
	s := &spr{}
	for range indexedCount {
		w, h := int(r.u16()), int(r.u16())
		if r.err != nil {
			break
		}
		if w >= wspr.MaxPageSize || h >= wspr.MaxPageSize {
			return nil, fmt.Errorf("%w: image is %d × %d", ErrFormat, w, h)
		}
		pix := make([]byte, 0, w*h)
		if ver >= 0x201 {
			data := r.take(int(r.u16()))
			for i := 0; i < len(data) && len(pix) < w*h; i++ {
				if data[i] != 0 {
					pix = append(pix, data[i])
					continue
				}
				if i+1 >= len(data) {
					pix = append(pix, 0)
					break
				}
				i++
				n := int(data[i])
				if n == 0 {
					n = 1
				}
				n = min(n, w*h-len(pix))
				pix = append(pix, make([]byte, n)...)
			}
			// Short runs at the end of an image are transparent.
			pix = append(pix, make([]byte, w*h-len(pix))...)
		} else {
			pix = append(pix, r.take(w*h)...)
		}
		s.indexed = append(s.indexed, wspr.Source{Width: w, Height: h, Pix: pix})
	}
	for range rgbaCount {
		w, h := int(r.u16()), int(r.u16())
		if w >= wspr.MaxPageSize || h >= wspr.MaxPageSize {
			return nil, fmt.Errorf("%w: image is %d × %d", ErrFormat, w, h)
		}
		raw := r.take(w * h * 4)
		if raw == nil {
			break
		}
		// RGBA frames are stored bottom row first, as A, B, G, R.
		pix := make([]byte, w*h*4)
		for y := range h {
			src := raw[(h-1-y)*w*4:]
			dst := pix[y*w*4:]
			for x := range w {
				a, bl, g, rd := src[x*4], src[x*4+1], src[x*4+2], src[x*4+3]
				dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = rd, g, bl, a
			}
		}
		s.rgba = append(s.rgba, wspr.Source{Width: w, Height: h, Pix: pix})
	}
	if r.err != nil {
		return nil, fmt.Errorf("%w: spr is truncated", ErrFormat)
	}
	if ver >= 0x101 && indexedCount > 0 {
		if r.left() < 1024 {
			return nil, fmt.Errorf("%w: spr has no palette", ErrFormat)
		}
		raw := r.b[len(r.b)-1024:]
		var pal [256][4]byte
		for i := range pal {
			pal[i] = [4]byte{raw[i*4], raw[i*4+1], raw[i*4+2], 255}
		}
		// Index 0 is the transparent color.
		pal[0][3] = 0
		s.palette = &pal
	}
	return s, nil
}

// act is a parsed RO animation file.
type act struct {
	actions []wspr.Action
	// types holds each layer's image kind, parallel to the layers, so the
	// importer can map act image numbers to WSPR images.
	types  [][][]int32
	events []string
}

// parseAct reads an act file. Versions 1.0 to 2.5 are supported.
func parseAct(b []byte) (*act, error) {
	r := &reader{b: b}
	if string(r.take(2)) != "AC" {
		return nil, fmt.Errorf("%w: not an act file", ErrFormat)
	}
	minor, major := r.u8(), r.u8()
	ver := int(major)<<8 | int(minor)
	if ver < 0x100 || ver > 0x205 {
		return nil, fmt.Errorf("%w: act version %d.%d", ErrFormat, major, minor)
	}
	count := int(r.u16())
	r.take(10)
	if count > maxActions {
		return nil, fmt.Errorf("%w: %d actions", ErrFormat, count)
	}
	a := &act{}
	for range count {
		frames := int(r.u32())
		if frames > maxFrames || r.err != nil {
			return nil, fmt.Errorf("%w: frame count", ErrFormat)
		}
		action := wspr.Action{IntervalMs: defaultIntervalMs}
		var types [][]int32
		for range frames {
			r.take(32) // attack ranges, unused
			layers := int(r.u32())
			if layers > maxLayers || r.err != nil {
				return nil, fmt.Errorf("%w: layer count", ErrFormat)
			}
			f := wspr.Frame{Event: -1}
			var ft []int32
			for range layers {
				x, y := r.i32(), r.i32()
				l := wspr.Layer{Image: r.i32(), Color: [4]byte{255, 255, 255, 255}, ScaleX: 1, ScaleY: 1}
				l.Mirror = r.u32() != 0
				kind := int32(0)
				if ver >= 0x200 {
					copy(l.Color[:], r.take(4))
					l.ScaleX = r.f32()
					l.ScaleY = l.ScaleX
					if ver >= 0x204 {
						l.ScaleY = r.f32()
					}
					rot := r.i32() % 360
					if rot < 0 {
						rot += 360
					}
					l.Rotation = uint16(rot) // #nosec G115 -- 0 to 359.
					kind = r.i32()
					if ver >= 0x205 {
						r.take(8) // width and height, unused
					}
				}
				l.X, l.Y = clamp16(x), clamp16(y)
				f.Layers = append(f.Layers, l)
				ft = append(ft, kind)
			}
			if ver >= 0x200 {
				f.Event = clamp16(r.i32())
			}
			if ver >= 0x203 {
				anchors := int(r.u32())
				if anchors > maxAnchors || r.err != nil {
					return nil, fmt.Errorf("%w: anchor count", ErrFormat)
				}
				for range anchors {
					r.take(4)
					x, y := r.i32(), r.i32()
					r.take(4)
					f.Anchors = append(f.Anchors, wspr.Point{X: clamp16(x), Y: clamp16(y)})
				}
			}
			action.Frames = append(action.Frames, f)
			types = append(types, ft)
		}
		a.actions = append(a.actions, action)
		a.types = append(a.types, types)
	}
	if ver >= 0x201 {
		n := int(r.u32())
		if n > maxEvents {
			return nil, fmt.Errorf("%w: %d events", ErrFormat, n)
		}
		for range n {
			raw := r.take(40)
			if i := strings.IndexByte(string(raw), 0); i >= 0 {
				raw = raw[:i]
			}
			// Event names are CP949, like GRF paths.
			name, err := korean.EUCKR.NewDecoder().String(string(raw))
			if err != nil {
				name = strings.ToValidUTF8(string(raw), "")
			}
			a.events = append(a.events, name)
		}
	}
	if ver >= 0x202 {
		for i := range a.actions {
			// The stored value counts 25 ms ticks.
			ms := math.Round(float64(r.f32()) * 25)
			if ms >= 1 && ms <= math.MaxUint16 {
				a.actions[i].IntervalMs = uint16(ms)
			}
		}
	}
	if r.err != nil {
		return nil, fmt.Errorf("%w: act is truncated", ErrFormat)
	}
	return a, nil
}

func clamp16(v int32) int16 {
	return int16(max(math.MinInt16, min(math.MaxInt16, v))) // #nosec G115 -- clamped.
}

// ConvertSprite converts an spr and act pair into a WSPR sprite.
// eventName maps an act event, such as a sound file name, to the name
// stored in the sprite.
func ConvertSprite(sprData, actData []byte, eventName func(string) string) (*wspr.Sprite, error) {
	s, err := parseSpr(sprData)
	if err != nil {
		return nil, err
	}
	a, err := parseAct(actData)
	if err != nil {
		return nil, err
	}
	out := &wspr.Sprite{Palette: s.palette}
	var images []wspr.Image
	if len(s.indexed) > 0 {
		pages, ims, err := wspr.Pack(wspr.Indexed, s.indexed)
		if err != nil {
			return nil, err
		}
		out.Pages = append(out.Pages, pages...)
		images = append(images, ims...)
	}
	if len(s.rgba) > 0 {
		pages, ims, err := wspr.Pack(wspr.RGBA, s.rgba)
		if err != nil {
			return nil, err
		}
		offset := uint16(len(out.Pages)) // #nosec G115 -- at most MaxPages.
		for i := range ims {
			ims[i].Page += offset
		}
		out.Pages = append(out.Pages, pages...)
		images = append(images, ims...)
	}
	if out.Palette == nil && len(s.indexed) > 0 {
		return nil, fmt.Errorf("%w: indexed images without a palette", ErrFormat)
	}
	out.Images = images

	for i, action := range a.actions {
		for j := range action.Frames {
			f := &action.Frames[j]
			for k := range f.Layers {
				l := &f.Layers[k]
				idx := l.Image
				if idx < 0 {
					l.Image = -1
					continue
				}
				// Layers name an image by kind and number. WSPR numbers
				// indexed images first, then RGBA images.
				if a.types[i][j][k] == 1 {
					idx += int32(len(s.indexed)) // #nosec G115 -- at most maxImages.
				}
				if int(idx) >= len(images) {
					l.Image = -1
					continue
				}
				l.Image = idx
			}
			if f.Event < 0 || int(f.Event) >= len(a.events) {
				f.Event = -1
			}
		}
		out.Actions = append(out.Actions, action)
	}
	for _, e := range a.events {
		if eventName != nil {
			e = eventName(e)
		}
		if len(e) > 255 {
			e = strings.ToValidUTF8(e[:255], "")
		}
		out.Events = append(out.Events, e)
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}
