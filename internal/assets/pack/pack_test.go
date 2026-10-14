package pack

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

// indexedPNG draws a w × h image of palette indexes: index 1 inside a
// border of index 2, with index 0 around it.
func indexedPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{
		color.NRGBA{255, 0, 255, 255}, color.NRGBA{240, 200, 80, 255}, color.NRGBA{90, 50, 20, 255},
	})
	for y := range h {
		for x := range w {
			switch {
			case x == 0 || y == 0 || x == w-1 || y == h-1:
			case x == 1 || y == 1 || x == w-2 || y == h-2:
				img.SetColorIndex(x, y, 2)
			default:
				img.SetColorIndex(x, y, 1)
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// rgbaPNG draws a w × h gradient with a transparent corner.
func rgbaPNG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), 128, 255}) // #nosec G115 -- below 256.
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// wav writes 0.05 seconds of a 16-bit mono square wave at 22,050 Hz.
func wav() []byte {
	const rate, n = 22050, 1102
	var b bytes.Buffer
	w := func(v ...any) {
		for _, x := range v {
			_ = binary.Write(&b, binary.LittleEndian, x)
		}
	}
	b.WriteString("RIFF")
	w(uint32(36 + n*2))
	b.WriteString("WAVEfmt ")
	w(uint32(16), uint16(1), uint16(1), uint32(rate), uint32(rate*2), uint16(2), uint16(16))
	b.WriteString("data")
	w(uint32(n * 2))
	for i := range n {
		v := int16(8000)
		if i/50%2 == 1 {
			v = -8000
		}
		w(v)
	}
	return b.Bytes()
}

func write(t testing.TB, dir string, files map[string][]byte) {
	t.Helper()
	for p, data := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const ghostYAML = `grid: [16, 24]
origin: [8, 22]
actions:
  - frames: [0, 1]
    interval_ms: 200
  - directions: [[0], [1], [0], [1], [0], [1], [0], [1]]
  - frames: [1, 0]
    events: {1: sound/effect/boo}
`

func TestPackSource(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	write(t, dir, map[string][]byte{
		"pack.yaml":                   []byte("name: lantern art\nlicense: CC0-1.0\n"),
		"sprite/mob/Ghost.png":        indexedPNG(t, 32, 24),
		"sprite/mob/Ghost.yaml":       []byte(ghostYAML),
		"sprite/npc/Keeper.png":       rgbaPNG(t, 20, 30),
		"icon/item/Paper_Lantern.png": rgbaPNG(t, 24, 24),
		"texture/wall.png":            rgbaPNG(t, 8, 8),
		"sound/effect/boo.wav":        wav(),
		"table/lanterns.json":         []byte(`{"a": 1}`),
		"sprite/stray.jpg":            []byte("not a sprite"),
		"notes.txt":                   []byte("hello"),
		"sound/effect/loud.ogg":       []byte("OggS"),
		".hidden/x.png":               []byte("skipped"),
	})
	src := &Source{Dir: dir, Prefix: "mod/example.lantern/", Formats: Both}
	rep, err := build.Run(build.Options{Out: out, Sources: []build.Source{src}, Brotli: 1, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if src.Info == nil || src.Info.License != "CC0-1.0" {
		t.Errorf("pack.yaml: %+v", src.Info)
	}
	list, err := build.List(out)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"mod/example.lantern/sprite/mob/Ghost":        "sprite",
		"mod/example.lantern/sprite/npc/Keeper":       "sprite",
		"mod/example.lantern/icon/item/Paper_Lantern": "texture",
		"mod/example.lantern/texture/wall":            "texture",
		"mod/example.lantern/sound/effect/boo":        "sound",
		"mod/example.lantern/table/lanterns":          "table",
	}
	for n, typ := range want {
		if list[n].Type != typ {
			t.Errorf("%s: type %q, want %q", n, list[n].Type, typ)
		}
	}
	if len(list) != len(want) {
		t.Errorf("%d names, want %d: %v", len(list), len(want), list)
	}
	rejected := strings.Join(rep.Rejected, "\n")
	for _, p := range []string{"sprite/stray.jpg", "notes.txt", "sound/effect/loud.ogg"} {
		if !strings.Contains(rejected, p) {
			t.Errorf("%s was not rejected:\n%s", p, rejected)
		}
	}
	if strings.Contains(rejected, "hidden") {
		t.Errorf("a hidden file was read:\n%s", rejected)
	}

	// The ghost has 2 frames from its grid, 3 actions of 8 directions,
	// and its event names the mod's own sound.
	data, _, err := build.ReadAsset(out, "mod/example.lantern/sprite/mob/Ghost")
	if err != nil {
		t.Fatal(err)
	}
	s, err := wspr.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Images) != 2 || len(s.Actions) != 24 || s.Palette == nil {
		t.Fatalf("ghost: %d images, %d actions, palette %v", len(s.Images), len(s.Actions), s.Palette != nil)
	}
	if s.Palette[0][3] != 0 || s.Palette[1] != [4]byte{240, 200, 80, 255} {
		t.Errorf("palette %v %v", s.Palette[0], s.Palette[1])
	}
	if s.Actions[0].IntervalMs != 200 || len(s.Actions[0].Frames) != 2 || s.Actions[8+1].Frames[0].Layers[0].Image != 1 {
		t.Errorf("actions %+v", s.Actions[:2])
	}
	if !slices.Equal(s.Events, []string{"mod/example.lantern/sound/effect/boo"}) || s.Actions[16].Frames[1].Event != 0 {
		t.Errorf("events %v", s.Events)
	}
	// The origin [8, 22] of a 16 × 24 frame puts the image's center 0
	// pixels across and 10 up from the cell.
	if l := s.Actions[0].Frames[0].Layers[0]; l.X != 0 || l.Y != -10 {
		t.Errorf("layer at %d, %d", l.X, l.Y)
	}

	// The build passes the checks of a mod's build, and fails them with
	// another mod's prefix or a small limit.
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	cr, err := build.CheckFS(root.FS(), nil, build.CheckOptions{Prefix: "mod/example.lantern/"})
	if err != nil || len(cr.Broken) > 0 {
		t.Fatalf("check: %v %v", err, cr.Broken)
	}
	cr, err = build.CheckFS(root.FS(), nil, build.CheckOptions{Prefix: "mod/other/"})
	if err != nil || len(cr.Broken) != len(want) {
		t.Errorf("another prefix: %v %v", err, cr.Broken)
	}
	cr, err = build.CheckFS(root.FS(), nil, build.CheckOptions{Prefix: "mod/example.lantern/", MaxBytes: 100})
	if err != nil || len(cr.Broken) != 1 || !strings.Contains(cr.Broken[0], "over the limit") {
		t.Errorf("limit: %v %v", err, cr.Broken)
	}

	// A second build reuses every asset.
	rep, err = build.Run(build.Options{Out: out, Sources: []build.Source{src}, Brotli: 1})
	if err != nil || rep.Cached != len(want) {
		t.Errorf("second build: %v, %d cached", err, rep.Cached)
	}
}

func TestCorePackNames(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	write(t, dir, map[string][]byte{
		"pack.yaml":         []byte("license: CC-BY-4.0\n"),
		"texture/Upper.png": rgbaPNG(t, 4, 4),
		"texture/lower.png": rgbaPNG(t, 4, 4),
	})
	src := &Source{Dir: dir, Formats: Authoring}
	rep, err := build.Run(build.Options{Out: out, Sources: []build.Source{src}, Brotli: 1})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Assets != 1 || len(rep.Rejected) != 1 || !strings.Contains(rep.Rejected[0], "lower case") {
		t.Errorf("%d assets, rejected %v", rep.Assets, rep.Rejected)
	}
}

func TestParseInfo(t *testing.T) {
	for _, bad := range []string{"name: x\n", "license: ''\n", "license: x\ncontract: assets-v2\n", "license: x\nauthor: y\n", "- x\n"} {
		if _, err := ParseInfo([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	in, err := ParseInfo([]byte("name: n\nlicense: CC0-1.0\ncontract: assets-v1\n"))
	if err != nil || in.Name != "n" {
		t.Errorf("%v %+v", err, in)
	}
	dir := t.TempDir()
	write(t, dir, map[string][]byte{"texture/a.png": rgbaPNG(t, 2, 2)})
	if _, err := (&Source{Dir: dir, Formats: Authoring}).Items(); err == nil || !strings.Contains(err.Error(), "license") {
		t.Errorf("a pack without pack.yaml: %v", err)
	}
}

func TestSpriteSpecErrors(t *testing.T) {
	img := indexedPNG(t, 32, 24)
	for _, bad := range []string{
		"grid: [0, 4]",
		"grid: [16, 24]\nframes: [[0, 0, 1, 1]]",
		"frames: [[30, 0, 16, 16]]",
		"grid: [16, 24]\nactions: [{frames: [2]}]",
		"grid: [16, 24]\nactions: [{frames: []}]",
		"grid: [16, 24]\nactions: [{directions: [[0]]}]",
		"grid: [16, 24]\nactions: [{frames: [0], interval_ms: -1}]",
		"grid: [16, 24]\nactions: [{frames: [0], events: {3: atk}}]",
		"grid: [16, 24]\nactions: [{frames: [0], events: {0: 'Bad/Name'}}]",
		"origin: [1]",
		"anchors: [[1, 2, 3]]",
		"colour: red",
	} {
		spec, err := ParseSpriteSpec([]byte(bad))
		if err == nil {
			_, err = ConvertSprite(img, spec, func(s string) string { return s })
		}
		if err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// No frame file: one still frame in every direction.
	s, err := ConvertSprite(rgbaPNG(t, 10, 12), nil, func(s string) string { return s })
	if err != nil || len(s.Actions) != 8 || s.Palette != nil || len(s.Pages) != 1 || s.Pages[0].Kind != wspr.RGBA {
		t.Errorf("still sprite: %v %+v", err, s)
	}
	spec, err := ParseSpriteSpec(nil)
	if err != nil || spec == nil {
		t.Errorf("empty frame file: %v", err)
	}
}

func FuzzSpriteSpec(f *testing.F) {
	f.Add([]byte(ghostYAML))
	f.Add([]byte("frames: [[0, 0, 4, 4], [4, 0, 4, 4]]\nanchors: [[0, -20]]\nactions: [{frames: [0, 1], events: {0: atk}}]"))
	img := indexedPNG(f, 32, 24)
	f.Fuzz(func(t *testing.T, data []byte) {
		spec, err := ParseSpriteSpec(data)
		if err != nil {
			return
		}
		s, err := ConvertSprite(img, spec, func(s string) string { return s })
		if err != nil {
			return
		}
		// Whatever the spec, an accepted sprite encodes and decodes.
		b, err := s.Encode()
		if err != nil {
			t.Fatalf("accepted sprite does not encode: %v", err)
		}
		if _, err := wspr.Decode(b); err != nil {
			t.Fatalf("accepted sprite does not decode: %v", err)
		}
	})
}

func FuzzSpriteImage(f *testing.F) {
	f.Add(indexedPNG(f, 8, 8))
	f.Add(rgbaPNG(f, 6, 3))
	spec := &SpriteSpec{Grid: []int{4, 4}, Actions: []ActionSpec{{Frames: []int{0}}}}
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := ConvertSprite(data, spec, func(s string) string { return s })
		if err != nil {
			return
		}
		if _, err := s.Encode(); err != nil {
			t.Fatalf("accepted sprite does not encode: %v", err)
		}
	})
}

func FuzzParseInfo(f *testing.F) {
	f.Add([]byte("name: n\nlicense: CC0-1.0\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		in, err := ParseInfo(data)
		if err == nil && strings.TrimSpace(in.License) == "" {
			t.Fatal("a pack without a license was accepted")
		}
	})
}
