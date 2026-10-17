package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"testing"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

var updateContent = flag.Bool("update-content", false, "rewrite testdata/golden-content.json")

// goldenContent is a golden world's content in short: its counts, where
// the guilds and the rebirth NPC stand, and the SHA-256 of its quests,
// places and objects as JSON.
type goldenContent struct {
	Seed        string   `json:"seed"`
	BoardQuests int      `json:"boardQuests"`
	GuildQuests int      `json:"guildQuests"`
	Places      int      `json:"places"`
	Guilds      []string `json:"guilds"`
	Rebirth     string   `json:"rebirth"`
	Checksum    string   `json:"checksum"`
}

func summarize(seed string, w *content.World) goldenContent {
	g := goldenContent{Seed: seed, Places: len(w.Places)}
	for _, q := range w.Quests {
		if q.Purpose == content.PurposeBoard {
			g.BoardQuests++
		} else {
			g.GuildQuests++
		}
	}
	for _, f := range content.Families {
		for _, p := range w.Places {
			if p.Tags[0] == content.GuildTag(f) {
				g.Guilds = append(g.Guilds, f+" "+p.Map)
			}
		}
	}
	for _, p := range w.Places {
		if p.Tags[0] == content.TagRebirth {
			g.Rebirth = p.Map
		}
	}
	b, _ := json.Marshal([]any{w.Quests, w.Places, w.Objects})
	sum := sha256.Sum256(b)
	g.Checksum = hex.EncodeToString(sum[:])
	return g
}

// TestGoldenContent makes two default worlds of core's records and
// compares their content with testdata/golden-content.json: the quest
// boards' and guilds' quests, their places, and the rebirth town
// (decision 0044). The same seed must give the same content.
// go test ./tools/wactl -run TestGoldenContent -update-content rewrites
// the file.
func TestGoldenContent(t *testing.T) {
	g, ok := gen.Latest()
	if !ok {
		t.Skip("a build without the procedural tag")
	}
	var got []goldenContent
	for _, seed := range []string{"golden-content-seed-0000000001", "golden-content-seed-0000000002"} {
		_, _, w, err := proceduralWorld("", "renewal", []byte(seed), g.Version)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, summarize(seed, w))
	}
	const file = "testdata/golden-content.json"
	if *updateContent {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var want []goldenContent
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	for _, c := range got {
		if c.BoardQuests != 5*content.BoardQuests || c.GuildQuests != len(content.Families)*len(content.Stages)*2 || len(c.Guilds) != len(content.Families) || c.Rebirth == "" {
			t.Errorf("%+v", c)
		}
	}
}
