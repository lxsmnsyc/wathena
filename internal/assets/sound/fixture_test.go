package sound

import (
	"bytes"
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the web client's test files")

// TestClientFixture writes sounds with known lengths for the web client's
// browser check, which decodes them and compares the lengths. A browser
// that ignores CodecDelay or DiscardPadding gets a longer sound. Run with
// -update after changing the encoder.
func TestClientFixture(t *testing.T) {
	type sample struct {
		File     string `json:"file"`
		Channels int    `json:"channels"`
		Frames   int    `json:"frames"`
	}
	var list []sample
	files := map[string][]byte{}
	for _, s := range []struct {
		name           string
		rate, channels int
		frames         int
	}{
		{"mono.webm", 48000, 1, 4801},
		{"stereo.webm", 48000, 2, 12345},
		{"resampled.webm", 22050, 1, 11025},
	} {
		p := &PCM{Rate: s.rate, Channels: s.channels, Samples: make([]float32, s.frames*s.channels)}
		for i := range p.Samples {
			p.Samples[i] = float32(0.5 * math.Sin(float64(i/s.channels)*2*math.Pi*440/float64(s.rate)))
		}
		b, err := Encode(p)
		if err != nil {
			t.Fatal(err)
		}
		files[s.name] = b
		// Output is at 48 kHz, with the source's length.
		list = append(list, sample{s.name, s.channels, int(math.Round(float64(s.frames) * 48000 / float64(s.rate)))})
	}
	j, err := json.MarshalIndent(list, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	files["sounds.json"] = append(j, '\n')
	dir := filepath.Join("..", "..", "..", "client", "src", "audio", "testdata")
	for name, want := range files {
		p := filepath.Join(dir, name)
		if *update {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
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
			t.Errorf("%s is out of date. Run go test ./internal/assets/sound -update", p)
		}
	}
}
