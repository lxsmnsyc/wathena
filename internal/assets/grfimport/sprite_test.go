package grfimport

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

// buildSpr writes a version 2.1 spr with the given indexed images and one
// 1 × 2 RGBA image.
func buildSpr(images []wspr.Source) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("SP")
	b.Write([]byte{1, 2}) // version 2.1
	w(uint16(len(images)))
	w(uint16(1))
	for _, im := range images {
		w(uint16(im.Width))
		w(uint16(im.Height))
		var rle []byte
		for i := 0; i < len(im.Pix); i++ {
			if im.Pix[i] != 0 {
				rle = append(rle, im.Pix[i])
				continue
			}
			n := 0
			for i < len(im.Pix) && im.Pix[i] == 0 && n < 255 {
				n++
				i++
			}
			i--
			rle = append(rle, 0, byte(n))
		}
		w(uint16(len(rle)))
		b.Write(rle)
	}
	// RGBA image: bottom row first, stored as A, B, G, R.
	w(uint16(1))
	w(uint16(2))
	b.Write([]byte{255, 3, 2, 1}) // bottom pixel: R=1 G=2 B=3
	b.Write([]byte{255, 6, 5, 4}) // top pixel: R=4 G=5 B=6
	pal := make([]byte, 1024)
	pal[4], pal[5], pal[6] = 200, 100, 50
	b.Write(pal)
	return b.Bytes()
}

// buildAct writes a version 2.5 act with one action of one frame.
func buildAct() []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("AC")
	b.Write([]byte{5, 2})
	w(uint16(1))
	b.Write(make([]byte, 10))
	w(uint32(1)) // frames
	b.Write(make([]byte, 32))
	w(uint32(2)) // layers
	// Indexed image 1, mirrored.
	w(int32(-5))
	w(int32(7))
	w(int32(1))
	w(uint32(1))
	b.Write([]byte{255, 255, 255, 200})
	w(float32(1))
	w(float32(2))
	w(int32(-90))
	w(int32(0))
	w(int32(0))
	w(int32(0))
	// RGBA image 0.
	w(int32(0))
	w(int32(0))
	w(int32(0))
	w(uint32(0))
	b.Write([]byte{255, 255, 255, 255})
	w(float32(1))
	w(float32(1))
	w(int32(0))
	w(int32(1))
	w(int32(0))
	w(int32(0))
	w(int32(0))  // event 0
	w(uint32(1)) // anchors
	w(int32(0))
	w(int32(3))
	w(int32(-4))
	w(int32(0))
	w(uint32(1)) // events
	ev := make([]byte, 40)
	copy(ev, "poring_attack.wav")
	b.Write(ev)
	w(float32(4)) // 4 × 25 ms
	return b.Bytes()
}

func TestConvertSprite(t *testing.T) {
	images := []wspr.Source{
		{Width: 3, Height: 2, Pix: []byte{0, 1, 0, 0, 0, 1}},
		{Width: 2, Height: 2, Pix: []byte{1, 1, 0, 1}},
	}
	s, err := ConvertSprite(buildSpr(images), buildAct(), EventName)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Images) != 3 {
		t.Fatalf("%d images, want 3", len(s.Images))
	}
	// Indexed pixels survive run-length decoding and packing.
	im := s.Images[1]
	p := s.Pages[im.Page]
	var got []byte
	for y := range int(im.Height) {
		off := (int(im.Y)+y)*int(p.Width) + int(im.X)
		got = append(got, p.Pix[off:off+int(im.Width)]...)
	}
	if !bytes.Equal(got, images[1].Pix) {
		t.Errorf("indexed pixels %v, want %v", got, images[1].Pix)
	}
	// RGBA pixels are flipped to top row first and reordered to R, G, B, A.
	rim := s.Images[2]
	rp := s.Pages[rim.Page]
	top := rp.Pix[(int(rim.Y)*int(rp.Width)+int(rim.X))*4:][:4]
	if !bytes.Equal(top, []byte{4, 5, 6, 255}) {
		t.Errorf("top RGBA pixel %v", top)
	}
	if s.Palette[1] != [4]byte{200, 100, 50, 255} || s.Palette[0][3] != 0 {
		t.Errorf("palette %v %v", s.Palette[0], s.Palette[1])
	}
	f := s.Actions[0].Frames[0]
	l := f.Layers[0]
	if l.Image != 1 || !l.Mirror || l.X != -5 || l.Y != 7 || l.ScaleY != 2 || l.Rotation != 270 || l.Color[3] != 200 {
		t.Errorf("layer 0 %+v", l)
	}
	if f.Layers[1].Image != 2 {
		t.Errorf("RGBA layer points to image %d, want 2", f.Layers[1].Image)
	}
	if s.Actions[0].IntervalMs != 100 {
		t.Errorf("interval %d, want 100", s.Actions[0].IntervalMs)
	}
	if len(f.Anchors) != 1 || f.Anchors[0] != (wspr.Point{X: 3, Y: -4}) {
		t.Errorf("anchors %v", f.Anchors)
	}
	if f.Event != 0 || s.Events[0] != "sound/poring_attack" {
		t.Errorf("event %d %v", f.Event, s.Events)
	}
	if _, err := s.Encode(); err != nil {
		t.Fatal(err)
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{
		`data\sprite\몬스터\PORING.spr`:     "sprite/mob/poring",
		`data\sprite\npc\4_F_KAFRA1.act`: "sprite/npc/4_f_kafra1",
	} {
		if got, ok := SpriteName(in); !ok || got != want {
			t.Errorf("SpriteName(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{`data\sprite\몬스터\sub\x.spr`, `data\texture\x.bmp`, `data\sprite\npc\..spr`} {
		if got, ok := SpriteName(in); ok {
			t.Errorf("SpriteName(%q) = %q, want no name", in, got)
		}
	}
	if got := EventName("atk"); got != "atk" {
		t.Errorf("EventName(atk) = %q", got)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(buildSpr([]wspr.Source{{Width: 2, Height: 1, Pix: []byte{0, 1}}}), buildAct())
	f.Fuzz(func(t *testing.T, sprData, actData []byte) {
		s, err := ConvertSprite(sprData, actData, EventName)
		if err != nil {
			return
		}
		// ConvertSprite validated the sprite, so only the size limit can fail.
		if b, err := s.Encode(); err == nil {
			if _, err := wspr.Decode(b); err != nil {
				t.Fatal(err)
			}
		}
	})
}
