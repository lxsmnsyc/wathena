package wspr

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the web client's test files")

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

// TestClientFixture keeps the web client's decoder test in step with this
// encoder. The client reads sample.wspr and checks it against
// sample.json. Run with -update after changing the format.
func TestClientFixture(t *testing.T) {
	s := sample(t)
	// An RGBA page as well, with one image.
	s.Pages = append(s.Pages, Page{Kind: RGBA, Width: 2, Height: 1, Pix: []byte{1, 2, 3, 4, 5, 6, 7, 8}})
	s.Images = append(s.Images, Image{Page: uint16(len(s.Pages) - 1), Width: 2, Height: 1}) // #nosec G115 -- small.
	s.Actions = append(s.Actions, Action{IntervalMs: 75, Frames: []Frame{{Event: -1, Layers: []Layer{{Image: 3, Color: [4]byte{1, 2, 3, 4}, ScaleX: 2, ScaleY: 2}}}}})
	b, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("..", "..", "..", "client", "src", "formats", "testdata")
	for name, want := range map[string][]byte{"sample.wspr": b, "sample-wspr.json": append(j, '\n')} {
		p := filepath.Join(dir, name)
		if *update {
			if err := os.WriteFile(p, want, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(p) // #nosec G304 -- fixed test path.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date. Run go test ./internal/assets/wspr -update", p)
		}
	}
}
