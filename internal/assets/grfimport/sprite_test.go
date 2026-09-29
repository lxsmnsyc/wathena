package grfimport

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
	if got := (*Tables)(nil).SpriteNames(`data\sprite\인간족\머리통\여\07_여.spr`); len(got) != 1 || got[0] != "sprite/head/human/f/7" {
		t.Errorf("head names %v", got)
	}
	tb := &Tables{headgear: map[string][]int{"남/남_고글": {1, 5}}}
	if got := tb.SpriteNames(`data\sprite\악세사리\남\남_고글.act`); len(got) != 2 || got[0] != "sprite/headgear/m/1" || got[1] != "sprite/headgear/m/5" {
		t.Errorf("headgear names %v", got)
	}
	if got, ok := PaletteName(`data\palette\도람족\머리\머리6_여_2.pal`); !ok || got != "palette/hair/doram/f/6/2" {
		t.Errorf("hair palette %q", got)
	}
	ct := &Tables{
		bodies:   map[string][]string{"검사": {"swordman", "swordman_h"}},
		races:    map[string]string{"검사": "인간족"},
		weapons:  map[string][]int{"_검": {2}, "_1120": {1120}},
		garments: map[string][]int{"천사날개": {1}, "c_wings_of_gabriel": {37, 38}},
	}
	for in, want := range map[string][]string{
		`data\sprite\인간족\몸통\남\검사_남.spr`:       {"sprite/body/swordman/m", "sprite/body/swordman_h/m"},
		`data\sprite\인간족\검사\검사_여_검.act`:       {"sprite/weapon/swordman/f/2", "sprite/weapon/swordman_h/f/2"},
		`data\sprite\인간족\검사\검사_남_1120.spr`:    {"sprite/weapon/swordman/m/1120", "sprite/weapon/swordman_h/m/1120"},
		`data\sprite\도람족\몸통\남\검사_남.spr`:       nil,
		`data\sprite\방패\검사\검사_남_가드.spr`:       {"sprite/shield/swordman/m/1", "sprite/shield/swordman_h/m/1"},
		`data\sprite\방패\검사\검사_여_28901_방패.act`: {"sprite/shield/swordman/f/28901", "sprite/shield/swordman_h/f/28901"},
		`data\sprite\방패\검사\검사_여_2189.act`:     nil,
		`data\sprite\로브\천사날개\남\검사_남.spr`:      {"sprite/garment/swordman/m/1", "sprite/garment/swordman_h/m/1"},
		`data\sprite\로브\C_Wings_of_Gabriel\여\검사_여.spr`: {
			"sprite/garment/swordman/f/37", "sprite/garment/swordman_h/f/37",
			"sprite/garment/swordman/f/38", "sprite/garment/swordman_h/f/38",
		},
		`data\sprite\로브\천사날개\천사날개.spr`: nil,
	} {
		if got := ct.SpriteNames(in); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("SpriteNames(%q) = %v, want %v", in, got, want)
		}
	}
	if got := ct.PaletteNames(`data\palette\몸\검사_여_3.pal`); fmt.Sprint(got) != "[palette/body/swordman/f/3 palette/body/swordman_h/f/3]" {
		t.Errorf("body palettes %v", got)
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
