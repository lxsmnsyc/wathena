// Package wspr reads and writes WSPR sprites, as defined in
// docs/specs/assets-v1.md, "Sprite (WSPR)".
package wspr

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

const (
	magic   = "WSPR"
	version = 1
)

// Page kinds.
const (
	Indexed uint8 = 0
	RGBA    uint8 = 1
)

// Limits from assets-v1, "Limits" and "Sprite (WSPR)".
const (
	MaxPageSize  = 2048
	MaxPages     = 64
	MaxBytes     = 32 << 20
	maxCount     = 1 << 20
	maxEventName = 255
)

var ErrFormat = errors.New("wspr: invalid sprite")

// Sprite is one decoded sprite.
type Sprite struct {
	Pages   []Page
	Palette *[256][4]byte // nil when there are no indexed pages
	Images  []Image
	Actions []Action
	Events  []string
}

// Page holds packed images. Pix has one byte per pixel for Indexed pages
// and four for RGBA pages.
type Page struct {
	Kind          uint8
	Width, Height uint16
	Pix           []byte
}

// Image is one frame image inside a page.
type Image struct {
	Page          uint16
	X, Y          uint16
	Width, Height uint16
}

// Action is one animation: RO action number × 8 + direction.
type Action struct {
	IntervalMs uint16
	Frames     []Frame
}

// Frame is one animation frame.
type Frame struct {
	Event   int16 // index into Sprite.Events, or -1
	Anchors []Point
	Layers  []Layer
}

type Point struct{ X, Y int16 }

// Layer draws one image, placed relative to the frame's anchor point.
type Layer struct {
	Image          int32 // index into Sprite.Images, or -1
	X, Y           int16
	Mirror         bool
	Color          [4]byte
	ScaleX, ScaleY float32
	Rotation       uint16 // degrees, 0 to 359
}

// Encode writes a sprite. It checks the sprite first.
func (s *Sprite) Encode() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString(magic)
	w(uint16(version))
	w(uint16(len(s.Pages)))   // #nosec G115 -- checked by Validate.
	w(uint32(len(s.Images)))  // #nosec G115 -- checked by Validate.
	w(uint32(len(s.Actions))) // #nosec G115 -- checked by Validate.
	w(uint16(len(s.Events)))  // #nosec G115 -- checked by Validate.
	for _, p := range s.Pages {
		w(p.Kind)
		w(p.Width)
		w(p.Height)
		b.Write(p.Pix)
	}
	if s.Palette != nil {
		for _, c := range s.Palette {
			b.Write(c[:])
		}
	}
	for _, im := range s.Images {
		w(im)
	}
	for _, a := range s.Actions {
		w(a.IntervalMs)
		w(uint16(len(a.Frames))) // #nosec G115 -- checked by Validate.
		for _, f := range a.Frames {
			w(f.Event)
			w(uint8(len(f.Anchors))) // #nosec G115 -- checked by Validate.
			for _, p := range f.Anchors {
				w(p)
			}
			w(uint16(len(f.Layers))) // #nosec G115 -- checked by Validate.
			for _, l := range f.Layers {
				w(l.Image)
				w(l.X)
				w(l.Y)
				mirror := uint8(0)
				if l.Mirror {
					mirror = 1
				}
				w(mirror)
				b.Write(l.Color[:])
				w(l.ScaleX)
				w(l.ScaleY)
				w(l.Rotation)
			}
		}
	}
	for _, e := range s.Events {
		w(uint8(len(e))) // #nosec G115 -- checked by Validate.
		b.WriteString(e)
	}
	if b.Len() > MaxBytes {
		return nil, fmt.Errorf("%w: %d bytes is over the limit", ErrFormat, b.Len())
	}
	return b.Bytes(), nil
}

// Validate checks counts, sizes, and references.
func (s *Sprite) Validate() error {
	if len(s.Pages) > MaxPages || len(s.Images) > maxCount || len(s.Actions) > maxCount || len(s.Events) > math.MaxUint16 {
		return fmt.Errorf("%w: too many parts", ErrFormat)
	}
	hasIndexed := false
	for i, p := range s.Pages {
		if p.Width > MaxPageSize || p.Height > MaxPageSize {
			return fmt.Errorf("%w: page %d is larger than %d", ErrFormat, i, MaxPageSize)
		}
		bpp := 1
		switch p.Kind {
		case Indexed:
			hasIndexed = true
		case RGBA:
			bpp = 4
		default:
			return fmt.Errorf("%w: page %d has kind %d", ErrFormat, i, p.Kind)
		}
		if len(p.Pix) != int(p.Width)*int(p.Height)*bpp {
			return fmt.Errorf("%w: page %d has %d bytes", ErrFormat, i, len(p.Pix))
		}
	}
	if hasIndexed != (s.Palette != nil) {
		return fmt.Errorf("%w: a palette is needed exactly when there are indexed pages", ErrFormat)
	}
	for i, im := range s.Images {
		if int(im.Page) >= len(s.Pages) && (im.Width > 0 || im.Height > 0) {
			return fmt.Errorf("%w: image %d is on a missing page", ErrFormat, i)
		}
		if im.Width > 0 && im.Height > 0 {
			p := s.Pages[im.Page]
			if int(im.X)+int(im.Width) > int(p.Width) || int(im.Y)+int(im.Height) > int(p.Height) {
				return fmt.Errorf("%w: image %d is outside its page", ErrFormat, i)
			}
		}
	}
	for ai, a := range s.Actions {
		if len(a.Frames) > math.MaxUint16 {
			return fmt.Errorf("%w: action %d has too many frames", ErrFormat, ai)
		}
		for _, f := range a.Frames {
			if len(f.Anchors) > math.MaxUint8 || len(f.Layers) > math.MaxUint16 {
				return fmt.Errorf("%w: action %d has a frame with too many parts", ErrFormat, ai)
			}
			if f.Event < -1 || int(f.Event) >= len(s.Events) {
				return fmt.Errorf("%w: action %d refers to a missing event", ErrFormat, ai)
			}
			for _, l := range f.Layers {
				if l.Image < -1 || int(l.Image) >= len(s.Images) {
					return fmt.Errorf("%w: action %d refers to a missing image", ErrFormat, ai)
				}
				if l.Rotation >= 360 {
					return fmt.Errorf("%w: rotation %d", ErrFormat, l.Rotation)
				}
			}
		}
	}
	for _, e := range s.Events {
		if len(e) > maxEventName {
			return fmt.Errorf("%w: event name too long", ErrFormat)
		}
	}
	return nil
}

// reader reads little-endian values and records the first error.
type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || r.p+n > len(r.b) {
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

func (r *reader) i16() int16 { return int16(r.u16()) } // #nosec G115 -- reinterprets the bits.
func (r *reader) i32() int32 { return int32(r.u32()) } // #nosec G115 -- reinterprets the bits.
func (r *reader) f32() float32 {
	return math.Float32frombits(r.u32())
}

// Decode reads a sprite and checks it. Counts are checked against the
// remaining bytes before anything is allocated.
func Decode(data []byte) (*Sprite, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("%w: too large", ErrFormat)
	}
	r := &reader{b: data}
	if string(r.take(4)) != magic || r.u16() != version {
		return nil, fmt.Errorf("%w: bad header", ErrFormat)
	}
	pageCount := int(r.u16())
	imageCount := int(r.u32())
	actionCount := int(r.u32())
	eventCount := int(r.u16())
	remaining := func() int { return len(r.b) - r.p }
	// Each image takes 10 bytes and each action at least 4.
	if pageCount > MaxPages || imageCount*10 > remaining() || actionCount*4 > remaining() {
		return nil, fmt.Errorf("%w: counts do not fit the data", ErrFormat)
	}
	s := &Sprite{}
	hasIndexed := false
	for range pageCount {
		p := Page{Kind: r.u8(), Width: r.u16(), Height: r.u16()}
		bpp := 1
		if p.Kind == RGBA {
			bpp = 4
		} else {
			hasIndexed = true
		}
		p.Pix = r.take(int(p.Width) * int(p.Height) * bpp)
		s.Pages = append(s.Pages, p)
	}
	if hasIndexed {
		var pal [256][4]byte
		raw := r.take(1024)
		for i := range pal {
			if raw != nil {
				copy(pal[i][:], raw[i*4:])
			}
		}
		s.Palette = &pal
	}
	s.Images = make([]Image, imageCount)
	for i := range s.Images {
		s.Images[i] = Image{Page: r.u16(), X: r.u16(), Y: r.u16(), Width: r.u16(), Height: r.u16()}
	}
	s.Actions = make([]Action, 0, actionCount)
	for range actionCount {
		a := Action{IntervalMs: r.u16()}
		frames := int(r.u16())
		if frames*5 > remaining() {
			return nil, fmt.Errorf("%w: frame count does not fit the data", ErrFormat)
		}
		a.Frames = make([]Frame, 0, frames)
		for range frames {
			f := Frame{Event: r.i16()}
			for range int(r.u8()) {
				f.Anchors = append(f.Anchors, Point{r.i16(), r.i16()})
			}
			layers := int(r.u16())
			if layers*23 > remaining() {
				return nil, fmt.Errorf("%w: layer count does not fit the data", ErrFormat)
			}
			f.Layers = make([]Layer, 0, layers)
			for range layers {
				l := Layer{Image: r.i32(), X: r.i16(), Y: r.i16()}
				switch r.u8() {
				case 0:
				case 1:
					l.Mirror = true
				default:
					return nil, fmt.Errorf("%w: mirror flag", ErrFormat)
				}
				copy(l.Color[:], r.take(4))
				l.ScaleX, l.ScaleY, l.Rotation = r.f32(), r.f32(), r.u16()
				f.Layers = append(f.Layers, l)
			}
			a.Frames = append(a.Frames, f)
		}
		s.Actions = append(s.Actions, a)
	}
	for range eventCount {
		s.Events = append(s.Events, string(r.take(int(r.u8()))))
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.p != len(data) {
		return nil, fmt.Errorf("%w: %d extra bytes", ErrFormat, len(data)-r.p)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}
