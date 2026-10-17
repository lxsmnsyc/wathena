package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
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
// (decision 0044). The same seed must give the same content. The worlds
// leave the economy checks of decision 0046 off, with spawns.economy:
// false, so the file still holds step 6b's content;
// TestGoldenEconomyContent holds a new world's.
// go test ./tools/wactl -run TestGoldenContent -update-content rewrites
// the files.
func TestGoldenContent(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "waserver.yml")
	if err := os.WriteFile(cfg, []byte("spawns:\n  economy: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goldenContentFile(t, cfg, "testdata/golden-content.json")
}

// TestGoldenEconomyContent is TestGoldenContent for new worlds, with the
// economy checks on, in testdata/golden-content-economy.json.
func TestGoldenEconomyContent(t *testing.T) {
	goldenContentFile(t, "", "testdata/golden-content-economy.json")
}

func goldenContentFile(t *testing.T, cfg, file string) {
	t.Helper()
	g, ok := gen.Latest()
	if !ok {
		t.Skip("a build without the procedural tag")
	}
	var got []goldenContent
	for _, seed := range []string{"golden-content-seed-0000000001", "golden-content-seed-0000000002"} {
		pw, err := proceduralWorld(cfg, "renewal", []byte(seed), g.Version)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, summarize(seed, pw.content))
	}
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
