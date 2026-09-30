package wmap

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

func sample() *Map {
	m := &Map{Width: 2, Height: 1}
	m.Cells = []Cell{{Flags: Walkable | Shootable, Heights: [4]float32{0, 1, 2, 3}}, {Flags: 0, Heights: [4]float32{-1, -1, -1, -1}}}
	m.Ground = Ground{
		Width: 1, Height: 1,
		Textures:        []string{"texture/grass"},
		Tiles:           []Tile{{U: [4]float32{0, 1, 0, 1}, V: [4]float32{0, 0, 1, 1}, Texture: 0, Lightmap: 3, Color: [4]uint8{1, 2, 3, 4}}},
		Surfaces:        []Surface{{Heights: [4]float32{1, 2, 3, 4}, Top: 0, North: -1, East: 0}},
		Lightmap:        "lightmap/test",
		LightmapCount:   4,
		LightmapColumns: 2,
	}
	m.Light = Light{Longitude: 45, Latitude: 30, Diffuse: [3]float32{1, 0.5, 0.25}, Ambient: [3]float32{0.1, 0.2, 0.3}, Opacity: 0.7, Direction: [3]float32{0, 1, 0}}
	m.Water = &WaterInfo{
		Zone:       Zone{Level: -1, Type: 2, WaveHeight: 0.2, WaveSpeed: 2, WavePitch: 50, AnimSpeed: 3},
		SplitWidth: 1, SplitHeight: 1,
		Zones:    []Zone{{Level: -2, Type: 2}},
		Textures: []WaterTextures{{Type: 2}},
	}
	for i := range m.Water.Textures[0].Frames {
		m.Water.Textures[0].Frames[i] = "texture/water"
	}
	m.Objects = []Object{{Model: "model/tree", Matrix: [16]float32{0: 1, 5: 1, 10: 1, 12: 3, 15: 1}, AnimType: 1, AnimSpeed: 2, BlockType: 3}}
	m.Sounds = []Sound{{Name: "sound/bird", Pos: [3]float32{1, 2, -3}, Volume: 0.5, Width: 1, Height: 2, Range: 10, Cycle: 7}}
	m.Effects = []Effect{{ID: 47, Pos: [3]float32{4, 5, -6}, Delay: 1, Param: [4]float32{1, 2, 3, 4}}}
	m.Deps = []string{"lightmap/test", "model/tree", "sound/bird", "texture/grass", "texture/water"}
	return m
}

func TestRoundTrip(t *testing.T) {
	m := sample()
	b, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Errorf("round trip changed the map:\n%+v\n%+v", got, m)
	}
	// Readers skip sections they do not know.
	extra := append(append([]byte(nil), b...), "NEW!\x02\x00\x00\x00ab"...)
	if _, err := Decode(extra); err != nil {
		t.Errorf("unknown section: %v", err)
	}
}

func TestRejects(t *testing.T) {
	bad := sample()
	bad.Ground.Surfaces[0].East = 5
	if _, err := bad.Encode(); err == nil {
		t.Error("encoded a surface with a missing tile")
	}
	bad = sample()
	bad.Ground.Tiles[0].Texture = 1
	if _, err := bad.Encode(); err == nil {
		t.Error("encoded a tile with a missing texture")
	}
	b, _ := sample().Encode()
	for _, cut := range []int{0, 5, 12, len(b) - 1} {
		if _, err := Decode(b[:cut]); err == nil {
			t.Errorf("decoded %d bytes", cut)
		}
	}
	// A ground section with only its size must not allocate that size.
	header := []byte("WMAP\x01\x00\x01\x00\x01\x00")
	truncated := append(header, "GRND\x04\x00\x00\x00\xff\xff\xff\xff"...)
	if _, err := Decode(truncated); err == nil {
		t.Error("decoded a truncated ground section")
	}
	twice := append(append([]byte(nil), b...), b[10:10+8+2*17]...)
	if _, err := Decode(twice); err == nil {
		t.Error("decoded a repeated section")
	}
}

func FuzzDecode(f *testing.F) {
	b, _ := sample().Encode()
	f.Add(b)
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Decode(data)
		if err != nil {
			return
		}
		if _, err := m.Encode(); err != nil {
			t.Fatalf("decoded map does not encode: %v", err)
		}
	})
}

// TestClientFixture keeps the web client's decoder test in step with this
// encoder. The client reads sample.wmap and checks it against sample.json.
// Run with -update after changing the format.
func TestClientFixture(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "client", "src", "formats", "testdata")
	b, err := sample().Encode()
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.MarshalIndent(sample(), "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"sample.wmap": b, "sample.json": append(j, '\n')}
	for name, want := range files {
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
			t.Errorf("%s is out of date. Run go test ./internal/assets/wmap -update", p)
		}
	}
}
