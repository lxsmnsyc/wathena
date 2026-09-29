package effect

import (
	"fmt"
	"math"
)

// This file bakes an STR effect into per-frame layer states, so the client
// does not need STR's keyframe rules. The rules follow roBrowser:
//
//   - At frame k, a layer uses the last type 0 key at or before k, the
//     source, and the last type 1 key at or before k.
//   - When that type 1 key comes right after the source in the file and
//     has the same frame, values morph: source + change × (k - frame).
//   - Otherwise the source's values hold, unless a type 1 key exists and
//     no later source does, which hides the layer.
//   - A layer is hidden before its first source, and after its last key
//     when it has no type 1 key.
//
// Morphs are linear in k, so a client that interpolates linearly between
// baked frames gets the same result as RO between whole frames.

// canvasCenter is the center of STR's 640 × 640 pixel canvas.
const canvasCenter = 320

// pixelsPerCell is STR pixels per cell.
const pixelsPerCell = 35

// Bake converts an STR effect. textureName maps a texture file name from
// the effect to an asset name.
func Bake(s *STR, textureName func(string) string) (*Effect, error) {
	e := &Effect{FPS: s.FPS, Frames: s.MaxKey + 1}
	if len(s.Layers)*e.Frames > MaxFrameRecords {
		return nil, fmt.Errorf("%w: %d layers of %d frames", ErrFormat, len(s.Layers), e.Frames)
	}
	index := map[string]uint16{}
	for _, l := range s.Layers {
		ids := make([]uint16, len(l.Textures))
		for i, t := range l.Textures {
			name := textureName(t)
			id, ok := index[name]
			if !ok {
				if len(e.Textures) >= NoTexture-1 {
					return nil, fmt.Errorf("%w: too many textures", ErrFormat)
				}
				id = uint16(len(e.Textures)) // #nosec G115 -- checked above.
				index[name] = id
				e.Textures = append(e.Textures, name)
			}
			ids[i] = id
		}
		frames := make([]Frame, e.Frames)
		for k := range frames {
			frames[k] = bakeFrame(l, ids, k)
		}
		e.Layers = append(e.Layers, frames)
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return e, nil
}

func bakeFrame(l Layer, textures []uint16, k int) Frame {
	if len(textures) == 0 {
		return Frame{}
	}
	from, to := -1, -1
	lastFrame, lastSource := int32(0), int32(0)
	for i, key := range l.Keys {
		if int(key.Frame) <= k {
			switch key.Type {
			case 0:
				from = i
			case 1:
				to = i
			}
		}
		lastFrame = max(lastFrame, key.Frame)
		if key.Type == 0 {
			lastSource = max(lastSource, key.Frame)
		}
	}
	if from < 0 || (to < 0 && int(lastFrame) < k) {
		return Frame{}
	}
	src := l.Keys[from]
	delta := float64(k) - float64(src.Frame)
	v := src
	if to != from+1 || l.Keys[to].Frame != src.Frame {
		if to >= 0 && lastSource <= src.Frame {
			return Frame{}
		}
	} else {
		d := l.Keys[to]
		mix := func(a, b float32) float32 { return float32(float64(a) + float64(float64(b)*delta)) }
		for i := range v.Color {
			v.Color[i] = mix(src.Color[i], d.Color[i])
		}
		for i := range v.XY {
			v.XY[i] = mix(src.XY[i], d.XY[i])
		}
		v.Angle = mix(src.Angle, d.Angle)
		v.Pos = [2]float32{mix(src.Pos[0], d.Pos[0]), mix(src.Pos[1], d.Pos[1])}
		n := float64(len(textures))
		switch d.AniType {
		case 1: // advance by the key's frame change
			v.AniFrame = mix(src.AniFrame, d.AniFrame)
		case 2: // advance by delay, stop at the last texture
			v.AniFrame = float32(math.Min(float64(mix(src.AniFrame, d.Delay)), n-1))
		case 3: // advance by delay, repeat
			v.AniFrame = float32(math.Mod(float64(mix(src.AniFrame, d.Delay)), n))
		case 4: // go back by delay, repeat
			v.AniFrame = float32(math.Mod(float64(src.AniFrame)-float64(float64(d.Delay)*delta), n))
		default:
			v.AniFrame = 0
		}
	}
	// RO picks the texture by truncating the frame number.
	a := float64(v.AniFrame)
	if math.IsNaN(a) {
		a = 0
	}
	tex := math.Trunc(a)
	if tex < 0 || tex >= float64(len(textures)) {
		return Frame{}
	}
	f := Frame{
		Visible:  true,
		Texture:  textures[int(tex)],
		SrcBlend: blendCode(v.SrcBlend),
		DstBlend: blendCode(v.DstBlend),
		Offset:   [2]float32{(v.Pos[0] - canvasCenter) / pixelsPerCell, -(v.Pos[1] - canvasCenter) / pixelsPerCell},
		Angle:    v.Angle,
	}
	for i, c := range v.Color {
		f.Color[i] = uint8(math.Max(0, math.Min(255, math.Round(float64(c)))))
	}
	// STR lists corner X values, then Y values, as top-left, top-right,
	// bottom-right, bottom-left, with Y down.
	for c := range 4 {
		f.Corners[c] = [2]float32{v.XY[c] / pixelsPerCell, -v.XY[c+4] / pixelsPerCell}
	}
	if !finiteFrame(f) {
		return Frame{}
	}
	return f
}

func blendCode(v uint32) uint8 {
	if v > 255 {
		return 0
	}
	return uint8(v)
}

func finiteFrame(f Frame) bool {
	vals := []float32{f.Offset[0], f.Offset[1], f.Angle}
	for _, c := range f.Corners {
		vals = append(vals, c[0], c[1])
	}
	for _, v := range vals {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 1e6 {
			return false
		}
	}
	return true
}
