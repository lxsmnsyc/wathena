package effect

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the web client's test files")

// TestClientFixture keeps the web client's decoder test in step with this
// encoder. The client reads sample.weff and checks it against
// sample-weff.json. Run with -update after changing the format.
func TestClientFixture(t *testing.T) {
	shown := func(tex uint16, x float32) Frame {
		return Frame{
			Visible: true, Texture: tex, SrcBlend: 5, DstBlend: 2,
			Color:   [4]uint8{255, 128, 64, 200},
			Offset:  [2]float32{x, 0.5},
			Angle:   30,
			Corners: [4][2]float32{{-1, 1}, {1, 1}, {1, -1}, {-1, -1}},
		}
	}
	e := &Effect{
		FPS: 60, Frames: 3,
		Textures: []string{"texture/effect/fire", "texture/effect/불꽃"},
		Layers: [][]Frame{
			{shown(0, 0), shown(1, 0.25), {}},
			{{}, shown(0, -1), shown(1, -2)},
		},
	}
	b, err := e.Encode()
	if err != nil {
		t.Fatal(err)
	}
	j, err := json.MarshalIndent(e, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("..", "..", "..", "client", "src", "formats", "testdata")
	for name, want := range map[string][]byte{"sample.weff": b, "sample-weff.json": append(j, '\n')} {
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
			t.Errorf("%s is out of date. Run go test ./internal/assets/effect -update", p)
		}
	}
}
