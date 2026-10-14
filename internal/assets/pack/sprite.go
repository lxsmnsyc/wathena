package pack

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

// Limits of a pack sprite, besides those of WSPR.
const (
	// MaxSpriteImage is the largest width or height of a sprite's PNG.
	MaxSpriteImage = 4096
	// MaxSpriteYAML is the largest frame file.
	MaxSpriteYAML = 1 << 20
	// MaxFrames is the most frames one sprite may cut from its image.
	MaxFrames = 4096
	// MaxActions is the most actions a sprite may list. Characters use
	// 13 and monsters 5.
	MaxActions = 64
	// MaxActionFrames is the most frames one action may show.
	MaxActionFrames = 256
)

// SpriteSpec is a sprite's frame file, <name>.yaml beside <name>.png
// (assets-v1, "Pack sprites"). Every field is optional.
type SpriteSpec struct {
	// Grid cuts the image into frames of [width, height], numbered left
	// to right, then top to bottom.
	Grid []int `yaml:"grid"`
	// Frames lists frames as [x, y, width, height] instead.
	Frames [][]int `yaml:"frames"`
	// Origin is the pixel of each frame that stands on the entity's
	// cell, [x, y]. The default is the middle of the bottom row.
	Origin []int `yaml:"origin"`
	// Anchors are points where other sprites attach, such as a head on a
	// body, as [x, y] from the origin, with y down. Every frame has them.
	Anchors [][]int `yaml:"anchors"`
	// Actions are in RO's order: action number × 8 + direction. Without
	// any, every action shows frame 0.
	Actions []ActionSpec `yaml:"actions"`
}

// ActionSpec is one action of a pack sprite.
type ActionSpec struct {
	// Frames are the frame numbers it shows, the same in every direction.
	Frames []int `yaml:"frames"`
	// Directions are 8 lists of frame numbers, one per direction from
	// south, turning clockwise, in place of Frames.
	Directions [][]int `yaml:"directions"`
	// IntervalMs is the time per frame. The default is 150.
	IntervalMs int `yaml:"interval_ms"`
	// Events maps a position in the action's frame list to an event: a
	// sound name, or a marker without "/", such as atk.
	Events map[int]string `yaml:"events"`
}

// ParseSpriteSpec reads a frame file. Unknown fields are errors.
func ParseSpriteSpec(data []byte) (*SpriteSpec, error) {
	if len(data) > MaxSpriteYAML {
		return nil, fmt.Errorf("the frame file is %d bytes, over the limit of %d", len(data), MaxSpriteYAML)
	}
	var s SpriteSpec
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// An empty frame file means the defaults.
	if err := dec.Decode(&s); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("frame file: %w", err)
	}
	return &s, nil
}

// rect is a frame in the image.
type rect struct{ x, y, w, h int }

// ConvertSprite makes a WSPR sprite from a PNG and its frame spec. spec
// may be nil for a still image: one frame, the whole image. eventName
// maps an event to the name stored, such as a pack's own sound name with
// its prefix.
func ConvertSprite(pngData []byte, spec *SpriteSpec, eventName func(string) string) (*wspr.Sprite, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("png: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxSpriteImage || cfg.Height > MaxSpriteImage {
		return nil, fmt.Errorf("the image is %d × %d; a sprite image is at most %d × %d", cfg.Width, cfg.Height, MaxSpriteImage, MaxSpriteImage)
	}
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("png: %w", err)
	}
	if spec == nil {
		spec = &SpriteSpec{}
	}
	b := img.Bounds()
	frames, err := spec.rects(b.Dx(), b.Dy())
	if err != nil {
		return nil, err
	}
	out := &wspr.Sprite{}
	srcs := make([]wspr.Source, len(frames))
	kind := wspr.RGBA
	if p, ok := img.(*image.Paletted); ok {
		// An indexed PNG keeps its palette, so dyes can swap it. Index 0
		// is transparent, as in RO.
		kind = wspr.Indexed
		var pal [256][4]byte
		// A PNG palette has at most 256 colors.
		for i := range min(len(p.Palette), len(pal)) {
			n, _ := color.NRGBAModel.Convert(p.Palette[i]).(color.NRGBA)
			pal[i] = [4]byte{n.R, n.G, n.B, n.A}
		}
		pal[0] = [4]byte{}
		out.Palette = &pal
		for i, r := range frames {
			pix := make([]byte, r.w*r.h)
			for y := range r.h {
				row := p.Pix[(r.y+y)*p.Stride+r.x : (r.y+y)*p.Stride+r.x+r.w]
				copy(pix[y*r.w:], row)
			}
			srcs[i] = wspr.Source{Width: r.w, Height: r.h, Pix: pix}
		}
	} else {
		for i, r := range frames {
			pix := make([]byte, r.w*r.h*4)
			for y := range r.h {
				for x := range r.w {
					c, _ := color.NRGBAModel.Convert(img.At(b.Min.X+r.x+x, b.Min.Y+r.y+y)).(color.NRGBA)
					o := (y*r.w + x) * 4
					pix[o], pix[o+1], pix[o+2], pix[o+3] = c.R, c.G, c.B, c.A
				}
			}
			srcs[i] = wspr.Source{Width: r.w, Height: r.h, Pix: pix}
		}
	}
	pages, images, err := wspr.Pack(kind, srcs)
	if err != nil {
		return nil, err
	}
	out.Pages, out.Images = pages, images
	origin := func(r rect) (int, int) { return r.w / 2, r.h - 1 }
	if len(spec.Origin) > 0 {
		if len(spec.Origin) != 2 {
			return nil, errors.New("origin is [x, y]")
		}
		origin = func(rect) (int, int) { return spec.Origin[0], spec.Origin[1] }
	}
	var anchors []wspr.Point
	for i, a := range spec.Anchors {
		if len(a) != 2 || !fits16(a[0]) || !fits16(a[1]) {
			return nil, fmt.Errorf("anchors[%d] is [x, y]", i)
		}
		anchors = append(anchors, wspr.Point{X: int16(a[0]), Y: int16(a[1])}) // #nosec G115 -- checked by fits16.
	}
	if len(anchors) > 255 {
		return nil, errors.New("a sprite has at most 255 anchors")
	}
	layer := func(f int) (wspr.Layer, error) {
		if f < 0 || f >= len(frames) {
			return wspr.Layer{}, fmt.Errorf("frame %d does not exist; the image has %d frames", f, len(frames))
		}
		r := frames[f]
		ox, oy := origin(r)
		// A layer's position is the image's center, from the origin.
		x2, y2 := r.w-2*ox, r.h-2*oy
		if !fits16(x2/2) || !fits16(y2/2) {
			return wspr.Layer{}, fmt.Errorf("frame %d: origin is too far away", f)
		}
		return wspr.Layer{Image: int32(f), X: int16(x2 / 2), Y: int16(y2 / 2), Color: [4]byte{255, 255, 255, 255}, ScaleX: 1, ScaleY: 1}, nil // #nosec G115 -- checked above.
	}
	events := map[string]int16{}
	actions := spec.Actions
	if len(actions) == 0 {
		actions = []ActionSpec{{Frames: []int{0}}}
	}
	if len(actions) > MaxActions {
		return nil, fmt.Errorf("a sprite has at most %d actions", MaxActions)
	}
	for ai, a := range actions {
		dirs := a.Directions
		switch {
		case len(dirs) == 0 && len(a.Frames) > 0:
			dirs = make([][]int, 8)
			for d := range dirs {
				dirs[d] = a.Frames
			}
		case len(dirs) == 8 && len(a.Frames) == 0:
		default:
			return nil, fmt.Errorf("actions[%d]: give frames, or directions with 8 lists", ai)
		}
		interval := a.IntervalMs
		if interval == 0 {
			interval = 150
		}
		if interval < 1 || interval > 60000 {
			return nil, fmt.Errorf("actions[%d]: interval_ms is 1 to 60000", ai)
		}
		for d, list := range dirs {
			if len(list) == 0 || len(list) > MaxActionFrames {
				return nil, fmt.Errorf("actions[%d]: direction %d has %d frames; an action has 1 to %d", ai, d, len(list), MaxActionFrames)
			}
			act := wspr.Action{IntervalMs: uint16(interval)} // #nosec G115 -- checked above.
			for pos, f := range list {
				l, err := layer(f)
				if err != nil {
					return nil, fmt.Errorf("actions[%d]: %w", ai, err)
				}
				fr := wspr.Frame{Event: -1, Anchors: anchors, Layers: []wspr.Layer{l}}
				if ev, ok := a.Events[pos]; ok {
					name := eventName(ev)
					if err := checkEvent(name); err != nil {
						return nil, fmt.Errorf("actions[%d]: events: %w", ai, err)
					}
					idx, ok := events[name]
					if !ok {
						idx = int16(len(out.Events)) // #nosec G115 -- at most one per frame of MaxActions × 8 × MaxActionFrames.
						events[name] = idx
						out.Events = append(out.Events, name)
					}
					fr.Event = idx
				}
				act.Frames = append(act.Frames, fr)
			}
			out.Actions = append(out.Actions, act)
		}
		for pos := range a.Events {
			if pos < 0 || pos >= len(dirs[0]) {
				return nil, fmt.Errorf("actions[%d]: events: position %d is not in the action's frames", ai, pos)
			}
		}
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

// rects returns the frames the spec cuts from a w × h image.
func (s *SpriteSpec) rects(w, h int) ([]rect, error) {
	var out []rect
	switch {
	case len(s.Grid) > 0 && len(s.Frames) > 0:
		return nil, errors.New("give grid or frames, not both")
	case len(s.Grid) > 0:
		if len(s.Grid) != 2 || s.Grid[0] <= 0 || s.Grid[1] <= 0 || s.Grid[0] >= wspr.MaxPageSize || s.Grid[1] >= wspr.MaxPageSize {
			return nil, fmt.Errorf("grid is [width, height], each 1 to %d", wspr.MaxPageSize-1)
		}
		cols, rows := w/s.Grid[0], h/s.Grid[1]
		if cols == 0 || rows == 0 {
			return nil, fmt.Errorf("the grid's cells are larger than the %d × %d image", w, h)
		}
		if cols*rows > MaxFrames {
			return nil, fmt.Errorf("the grid has %d frames; a sprite has at most %d", cols*rows, MaxFrames)
		}
		for y := range rows {
			for x := range cols {
				out = append(out, rect{x * s.Grid[0], y * s.Grid[1], s.Grid[0], s.Grid[1]})
			}
		}
	case len(s.Frames) > 0:
		if len(s.Frames) > MaxFrames {
			return nil, fmt.Errorf("a sprite has at most %d frames", MaxFrames)
		}
		for i, f := range s.Frames {
			if len(f) != 4 || f[0] < 0 || f[1] < 0 || f[2] <= 0 || f[3] <= 0 || f[2] >= wspr.MaxPageSize || f[3] >= wspr.MaxPageSize ||
				f[0] > w-f[2] || f[1] > h-f[3] {
				return nil, fmt.Errorf("frames[%d] is [x, y, width, height] inside the %d × %d image", i, w, h)
			}
			out = append(out, rect{f[0], f[1], f[2], f[3]})
		}
	default:
		if w >= wspr.MaxPageSize || h >= wspr.MaxPageSize {
			return nil, fmt.Errorf("a still sprite is less than %d pixels wide and high", wspr.MaxPageSize)
		}
		out = append(out, rect{0, 0, w, h})
	}
	return out, nil
}

// checkEvent checks a sprite event: an asset name, or a marker of at most
// 255 bytes without "/".
func checkEvent(name string) error {
	if strings.Contains(name, "/") {
		if assets.ValidName(name) != nil {
			return fmt.Errorf("%q is not an asset name", name)
		}
		return nil
	}
	if name == "" || len(name) > 255 {
		return fmt.Errorf("%q is not a marker", name)
	}
	return nil
}

func fits16(v int) bool { return v >= -32768 && v <= 32767 }
