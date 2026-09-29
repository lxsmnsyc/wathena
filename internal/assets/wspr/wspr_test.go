package wspr

import (
	"bytes"
	"reflect"
	"testing"
)

func sample(t testing.TB) *Sprite {
	t.Helper()
	srcs := []Source{
		{Width: 3, Height: 2, Pix: []byte{1, 2, 3, 4, 5, 6}},
		{Width: 0, Height: 0},
		{Width: 1, Height: 4, Pix: []byte{7, 8, 9, 10}},
	}
	pages, images, err := Pack(Indexed, srcs)
	if err != nil {
		t.Fatal(err)
	}
	var pal [256][4]byte
	pal[1] = [4]byte{255, 0, 0, 255}
	return &Sprite{
		Pages:   pages,
		Palette: &pal,
		Images:  images,
		Actions: []Action{{IntervalMs: 100, Frames: []Frame{{
			Event:   0,
			Anchors: []Point{{-3, 5}},
			Layers:  []Layer{{Image: 2, X: -1, Y: 2, Mirror: true, Color: [4]byte{255, 255, 255, 128}, ScaleX: 1, ScaleY: 1.5, Rotation: 90}},
		}}}},
		Events: []string{"sound/effect/hit"},
	}
}

func TestRoundTrip(t *testing.T) {
	s := sample(t)
	b, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Errorf("decoded sprite differs:\n got %+v\nwant %+v", got, s)
	}
}

func TestPackKeepsPixels(t *testing.T) {
	s := sample(t)
	im := s.Images[2]
	p := s.Pages[im.Page]
	var col []byte
	for y := range int(im.Height) {
		col = append(col, p.Pix[(int(im.Y)+y)*int(p.Width)+int(im.X)])
	}
	if !bytes.Equal(col, []byte{7, 8, 9, 10}) {
		t.Errorf("packed pixels %v", col)
	}
}

func TestValidateRejects(t *testing.T) {
	for name, mutate := range map[string]func(*Sprite){
		"missing image": func(s *Sprite) { s.Actions[0].Frames[0].Layers[0].Image = 9 },
		"missing event": func(s *Sprite) { s.Actions[0].Frames[0].Event = 3 },
		"no palette":    func(s *Sprite) { s.Palette = nil },
		"rotation":      func(s *Sprite) { s.Actions[0].Frames[0].Layers[0].Rotation = 360 },
		"outside page":  func(s *Sprite) { s.Images[0].X = 2047 },
	} {
		s := sample(t)
		mutate(s)
		if _, err := s.Encode(); err == nil {
			t.Errorf("%s: Encode accepted it", name)
		}
	}
}

func FuzzDecode(f *testing.F) {
	s := sample(f)
	b, err := s.Encode()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := Decode(data)
		if err != nil {
			return
		}
		// Anything that decodes must encode to the same bytes.
		out, err := s.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, data) {
			t.Fatal("re-encoded bytes differ")
		}
	})
}
