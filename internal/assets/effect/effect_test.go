package effect

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

// buildSTR writes an STR with one layer of two textures and the given keys.
func buildSTR(maxKey int, keys []Key) []byte {
	var b bytes.Buffer
	w := func(v ...any) {
		for _, x := range v {
			_ = binary.Write(&b, binary.LittleEndian, x)
		}
	}
	b.WriteString("STRM")
	w(uint32(0x94), uint32(60), uint32(maxKey), uint32(1))
	b.Write(make([]byte, 16))
	w(uint32(2))
	for _, name := range []string{"a.bmp", "b.bmp"} {
		f := make([]byte, 128)
		copy(f, name)
		b.Write(f)
	}
	w(uint32(len(keys)))
	for _, k := range keys {
		w(k.Frame, k.Type, k.Pos, k.UV, k.XY, k.AniFrame, k.AniType, k.Delay, k.Angle*1024/360, k.Color, k.SrcBlend, k.DstBlend, k.MTPreset)
	}
	return b.Bytes()
}

// square is a 70-pixel square, 2 cells wide, at the canvas center.
func square(frame int32, typ uint32) Key {
	return Key{
		Frame: frame, Type: typ,
		Pos:      [2]float32{320, 320},
		XY:       [8]float32{-35, 35, 35, -35, -35, -35, 35, 35},
		Color:    [4]float32{255, 128, 0, 255},
		SrcBlend: 5, DstBlend: 2,
	}
}

func bake(t *testing.T, maxKey int, keys []Key) *Effect {
	t.Helper()
	s, err := ParseSTR(buildSTR(maxKey, keys))
	if err != nil {
		t.Fatal(err)
	}
	e, err := Bake(s, func(p string) string { return "texture/" + p })
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestStatic(t *testing.T) {
	e := bake(t, 5, []Key{square(2, 0)})
	if e.Frames != 6 || e.FPS != 60 || !reflect.DeepEqual(e.Textures, []string{"texture/a.bmp", "texture/b.bmp"}) {
		t.Fatalf("effect %d frames, %d fps, %q", e.Frames, e.FPS, e.Textures)
	}
	l := e.Layers[0]
	// Hidden before the first key and after the last, since the layer has
	// no type 1 key.
	if l[1].Visible || !l[2].Visible || l[3].Visible {
		t.Errorf("visibility %v %v %v", l[1].Visible, l[2].Visible, l[3].Visible)
	}
	f := l[2]
	// Y turns from down to up: the top-left corner is (-1, 1) cells.
	if f.Corners[0] != [2]float32{-1, 1} || f.Corners[2] != [2]float32{1, -1} || f.Offset != [2]float32{0, 0} {
		t.Errorf("corners %v offset %v", f.Corners, f.Offset)
	}
	if f.Color != [4]uint8{255, 128, 0, 255} || f.SrcBlend != 5 || f.DstBlend != 2 || f.Texture != 0 {
		t.Errorf("frame %+v", f)
	}
}

func TestMorph(t *testing.T) {
	src := square(0, 0)
	src.AniFrame = 0
	change := Key{Frame: 0, Type: 1, Pos: [2]float32{35, -35}, Angle: 10, Color: [4]float32{0, 0, 0, -50}, AniType: 3, Delay: 0.5}
	e := bake(t, 4, []Key{src, change})
	f := e.Layers[0][3]
	// Three frames in: moved 3 cells right and 3 up, turned 30°, faded
	// by 150, and on texture 1 (1.5, truncated).
	if !f.Visible || f.Offset != [2]float32{3, 3} || f.Angle != 30 || f.Color[3] != 105 || f.Texture != 1 {
		t.Errorf("frame 3 %+v", f)
	}
	// Frame 4: 2.0 wraps to texture 0.
	if e.Layers[0][4].Texture != 0 {
		t.Errorf("frame 4 texture %d", e.Layers[0][4].Texture)
	}
}

func TestRoundTrip(t *testing.T) {
	e := bake(t, 3, []Key{square(0, 0), {Frame: 0, Type: 1, Pos: [2]float32{1, 1}, AniType: 1}})
	b, err := e.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, e) {
		t.Errorf("round trip changed the effect")
	}
	if _, err := Decode(b[:len(b)-1]); err == nil {
		t.Error("decoded a truncated effect")
	}
	if _, err := ParseSTR(buildSTR(1<<20, nil)); err == nil {
		t.Error("parsed an effect with too many frames")
	}
}

func FuzzEffect(f *testing.F) {
	f.Add(buildSTR(4, []Key{square(0, 0), {Frame: 0, Type: 1, AniType: 4, Delay: 1}}))
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := ParseSTR(data)
		if err != nil {
			if e, err := Decode(data); err == nil {
				if _, err := e.Encode(); err != nil {
					t.Fatal(err)
				}
			}
			return
		}
		e, err := Bake(s, func(p string) string { return "t" })
		if err != nil {
			return
		}
		b, err := e.Encode()
		if err != nil {
			return
		}
		if _, err := Decode(b); err != nil {
			t.Fatal(err)
		}
	})
}
