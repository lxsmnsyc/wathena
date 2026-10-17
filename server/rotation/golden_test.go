//go:build procedural

package rotation_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/fields"
	fieldscore "github.com/lxsmnsyc/wathena/server/fields/core"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/layout"
	layoutcore "github.com/lxsmnsyc/wathena/server/layout/core"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/spawns"
	spawncore "github.com/lxsmnsyc/wathena/server/spawns/core"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

var update = flag.Bool("update", false, "rewrite testdata/golden-rotation.json")

// records reads one of core's record files.
func records(t testing.TB, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

var (
	poolOnce sync.Once
	pool     *spawns.Pool
	settings spawns.Resolved
	poolErr  error
)

// corePool is core's renewal monsters with core's spawn tags and themes.
func corePool(t testing.TB) (*spawns.Pool, spawns.Resolved) {
	t.Helper()
	poolOnce.Do(func() {
		b, err := reference.ReadFile("extracts/renewal/script_constants.json")
		if err != nil {
			poolErr = err
			return
		}
		consts, err := constants.Read(bytes.NewReader(b))
		if err != nil {
			poolErr = err
			return
		}
		monsters, err := gamedata.LoadMonsters(reference.Renewal, nil, nil)
		if err != nil {
			poolErr = err
			return
		}
		ct, err := spawns.LoadCore()
		if err != nil {
			poolErr = err
			return
		}
		tb, _ := spawncore.Files.ReadFile(spawncore.ThemesFile)
		var recs []map[string]any
		if poolErr = yaml.Unmarshal(tb, &recs); poolErr != nil {
			return
		}
		var themes []spawns.Theme
		for _, r := range recs {
			name, _ := r["name"].(string)
			delete(r, "name")
			th, err := spawns.ParseTheme(name, r, nil, nil)
			if err != nil {
				poolErr = err
				return
			}
			themes = append(themes, th)
		}
		pool = spawns.NewPool(gamedata.SpawnMonsters(monsters, consts), themes)
		settings, poolErr = spawns.Settings{}.Resolve(ct.Densities)
	})
	if poolErr != nil {
		t.Fatal(poolErr)
	}
	return pool, settings
}

// world is a new world of core's records: its layout, branches and
// rotating dungeon.
type world struct {
	g              gen.Generator
	base           *layout.Layout
	public, secret rng.Node
	rotation       layout.Rotation
}

func coreWorld(t testing.TB, secret string, cfg layout.Config) world {
	t.Helper()
	g, _ := gen.Latest()
	root, err := rng.NewRoot([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	public := gen.PublicBranch(root)
	in := layout.Inputs{Generator: g, Public: public}
	b, _ := fieldscore.Files.ReadFile("data/town_templates.yml")
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		tp, err := fields.ParseTemplate(r)
		if err != nil {
			t.Fatal(err)
		}
		in.Towns = append(in.Towns, layout.TownRecord{Name: name, Template: tp})
	}
	b, _ = fieldscore.Files.ReadFile("data/dungeon_styles.yml")
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		rot, err := fields.ParseStyleRotation(r)
		if err != nil {
			t.Fatal(err)
		}
		st, err := fields.ParseStyle(r)
		if err != nil {
			t.Fatal(err)
		}
		in.Styles = append(in.Styles, layout.StyleRecord{Name: name, Style: st, Rotation: rot})
	}
	b, _ = layoutcore.Files.ReadFile("data/naming_styles.yml")
	var styles []layout.NamingStyle
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		s, err := layout.ParseNamingStyle(name, r)
		if err != nil {
			t.Fatal(err)
		}
		styles = append(styles, s)
	}
	in.Naming = layout.NewNaming(styles)
	words, _ := chat.CoreRecords(chat.FilterFile)
	var list []chat.Word
	for _, w := range words {
		cw, err := chat.BuildWord(w.Name, w.Fields)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, cw)
	}
	in.Blocked = layout.Blocked(chat.NewFilter(list))
	l, err := layout.Build(cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := layout.PickRotation(l, in.Styles)
	if !ok {
		t.Fatal("no rotating dungeon")
	}
	return world{g: g, base: l, public: public, secret: spawns.SecretBranch(root), rotation: r}
}

// goldenState is one state of a golden world, and what it gives.
type goldenState struct {
	Dates   map[string]string `json:"dates"`
	Seasons map[string]int    `json:"seasons"`
	// Layout is the checksum of the layout that runs. Maps are each
	// rotating floor's and changed field's node, biome, grid checksum
	// and WMAP hash, and Spawns their spawn lists' hashes, by map ID.
	Layout string            `json:"layout"`
	Maps   map[string]string `json:"maps"`
	Spawns map[string]string `json:"spawns"`
}

type goldenWorld struct {
	Secret   string        `json:"secret"`
	Dungeon  string        `json:"dungeon"`
	Rotation string        `json:"rotation"`
	Region   string        `json:"region"`
	States   []goldenState `json:"states"`
}

var goldenSecrets = []string{"golden-rotation-1", "golden-rotation-2"}

func goldenConfig() layout.Config {
	cfg := layout.DefaultConfig()
	cfg.Width, cfg.Height, cfg.Towns, cfg.Dungeons, cfg.FieldSize = 4, 3, 3, 3, 96
	return cfg
}

// outcome generates a state of a world: the layout, the maps that
// differ from the stored ones, and their spawns from the derived keys.
func outcome(t testing.TB, w world, st layout.State) goldenState {
	t.Helper()
	p, rs := corePool(t)
	run := w.base.Apply(w.public, len(w.g.Biomes), st)
	made, err := fields.GenerateLayout(w.g, w.public, run, nil)
	if err != nil {
		t.Fatal(err)
	}
	set := fields.NewSet(made, nil)
	warps, err := set.LayoutWarps(run)
	if err != nil {
		t.Fatal(err)
	}
	out := goldenState{Dates: st.Dates, Seasons: st.Seasons, Maps: map[string]string{}, Spawns: map[string]string{}}
	sum := run.Checksum()
	out.Layout = hex.EncodeToString(sum[:])
	regions := map[string]string{}
	for _, r := range layout.Regions(w.base) {
		for _, f := range r.Fields {
			regions[f] = r.Town
		}
	}
	groups := map[string][]string{}
	nodes := map[string]rng.Node{}
	for _, m := range run.Maps {
		var group string
		var node rng.Node
		switch {
		case m.Date != "":
			group, node = "rotation:"+m.Date, rotation.RotationSecret(rotation.RotationKey(w.secret, m.Dungeon), m.Date)
		case m.Season > 0:
			group, node = "season:"+regions[m.ID], rotation.SeasonSecret(rotation.SeasonKey(w.secret), regions[m.ID], m.Season)
		default:
			continue
		}
		f := set.Get(m.ID)
		key := layout.MapNode(w.public, m).Key()
		out.Maps[m.ID] = fmt.Sprintf("node %s biome %s grid %s wmap %s", hex.EncodeToString(key[:]), f.BiomeName,
			hex.EncodeToString(f.Checksum[:]), hex.EncodeToString(f.WMAPSum[:]))
		groups[group], nodes[group] = append(groups[group], m.ID), node
	}
	for g, ids := range groups {
		sm, err := fields.SpawnMapsOf(run, warps, set.Get, func(id string) bool { return slices.Contains(ids, id) })
		if err != nil {
			t.Fatal(err)
		}
		for _, pl := range spawns.Plan(nodes[g], sm, p, rs) {
			h := sha256.New()
			for _, s := range pl.Spawns {
				_, _ = fmt.Fprintf(h, "%s %d %d %s %v %v\n", s.Monster, s.Count, s.RespawnMs, s.Role, s.Area, s.Avoid)
			}
			out.Spawns[pl.Map] = hex.EncodeToString(h.Sum(nil))
		}
	}
	return out
}

// goldenWorlds computes the golden worlds: each in two rotations of its
// rotating dungeon, and with its first region in seasons 1 and 2.
func goldenWorlds(t testing.TB) []goldenWorld {
	var out []goldenWorld
	for _, secret := range goldenSecrets {
		w := coreWorld(t, secret, goldenConfig())
		region := layout.Regions(w.base)[0].Town
		gw := goldenWorld{Secret: secret, Dungeon: w.rotation.Dungeon, Rotation: w.rotation.Rule.String(), Region: region}
		for _, st := range []layout.State{
			{Dates: map[string]string{w.rotation.Dungeon: "2026-10-16"}},
			{Dates: map[string]string{w.rotation.Dungeon: "2026-10-23"}},
			{Dates: map[string]string{w.rotation.Dungeon: "2026-10-23"}, Seasons: map[string]int{region: 1}},
			{Dates: map[string]string{w.rotation.Dungeon: "2026-10-23"}, Seasons: map[string]int{region: 2}},
		} {
			gw.States = append(gw.States, outcome(t, w, st))
		}
		out = append(out, gw)
	}
	return out
}

// TestGoldenRotation: rotations for fixed dates and seasons of a region
// give the floors, fields and spawns of testdata/golden-rotation.json on
// this platform (decision 0045). scripts/check.sh gen runs it on arm64
// and on amd64 under Rosetta.
func TestGoldenRotation(t *testing.T) {
	file := filepath.Join("testdata", "golden-rotation.json")
	got := goldenWorlds(t)
	if *update {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
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
	var want []goldenWorld
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if !bytes.Equal(gb, wb) {
		for i := range min(len(got), len(want)) {
			for k := range min(len(got[i].States), len(want[i].States)) {
				g, w := got[i].States[k], want[i].States[k]
				if g.Layout != w.Layout {
					t.Errorf("%s state %d: layout %s, want %s", got[i].Secret, k, g.Layout, w.Layout)
				}
				for id, v := range w.Maps {
					if g.Maps[id] != v {
						t.Errorf("%s state %d %s:\n got %s\nwant %s", got[i].Secret, k, id, g.Maps[id], v)
					}
				}
				for id, v := range w.Spawns {
					if g.Spawns[id] != v {
						t.Errorf("%s state %d %s: spawns %s, want %s", got[i].Secret, k, id, g.Spawns[id], v)
					}
				}
			}
		}
		t.Fatal("the golden rotations differ")
	}
	t.Logf("%d golden worlds match on %s/%s", len(want), runtime.GOOS, runtime.GOARCH)
}

// TestRotationRules: in the golden worlds, each date gives other floors
// with other IDs and nomemo, linked to the same entrance, each season
// gives the region's fields one new biome and keeps their IDs and bands,
// and the public branch gives no spawns of the keys.
func TestRotationRules(t *testing.T) {
	w := coreWorld(t, goldenSecrets[0], goldenConfig())
	d := w.rotation.Dungeon
	a := w.base.Apply(w.public, len(w.g.Biomes), layout.State{Dates: map[string]string{d: "2026-10-16"}})
	b := w.base.Apply(w.public, len(w.g.Biomes), layout.State{Dates: map[string]string{d: "2026-10-23"}})
	floors := 0
	for i, m := range a.Maps {
		if m.Dungeon != d {
			if b.Maps[i].ID != m.ID {
				t.Errorf("%s changed to %s", m.ID, b.Maps[i].ID)
			}
			continue
		}
		floors++
		want := layout.FloorID(d, "2026-10-16", m.Floor)
		if m.ID != want || !m.NoMemo || m.Base != gen.DungeonMapID(d, m.Floor) || b.Maps[i].ID != layout.FloorID(d, "2026-10-23", m.Floor) {
			t.Errorf("floor %+v", m)
		}
		ka, kb := layout.MapNode(w.public, m).Key(), layout.MapNode(w.public, b.Maps[i]).Key()
		if ka == kb {
			t.Errorf("%s: two dates share a node", m.ID)
		}
		if id, date, f, ok := layout.ParseFloorID(m.ID); !ok || id != d || date != "2026-10-16" || f != m.Floor {
			t.Errorf("parse %s: %s %s %d %v", m.ID, id, date, f, ok)
		}
	}
	if floors == 0 {
		t.Fatal("no rotating floors")
	}
	entrance := func(l *layout.Layout) string {
		for _, k := range l.Links {
			if k.B.Tag == layout.TagEntrance && k.B.Map == l.Maps[slices.IndexFunc(l.Maps, func(m layout.Map) bool { return m.Dungeon == d && m.Floor == 1 })].ID {
				return k.A.String()
			}
		}
		return ""
	}
	if ea, eb := entrance(a), entrance(b); ea == "" || ea != eb {
		t.Errorf("entrances %q and %q", ea, eb)
	}
	r := layout.Regions(w.base)[0]
	for n := 1; n <= 3; n++ {
		s := w.base.Apply(w.public, len(w.g.Biomes), layout.State{Seasons: map[string]int{r.Town: n}})
		prev := w.base.Apply(w.public, len(w.g.Biomes), layout.State{Seasons: map[string]int{r.Town: n - 1}})
		biome := -1
		for i, m := range s.Maps {
			o := w.base.Maps[i]
			switch {
			case slices.Contains(r.Fields, m.ID):
				if m.Season != n || m.ID != o.ID || m.Levels != o.Levels {
					t.Errorf("season %d: %+v", n, m)
				}
				if biome >= 0 && m.Biome != biome {
					t.Errorf("season %d: two biomes in one region", n)
				}
				biome = m.Biome
			case m.Season != 0 || m.Biome != o.Biome:
				t.Errorf("season %d changed %s outside the region", n, m.ID)
			}
		}
		if n > 1 && slices.ContainsFunc(prev.Maps, func(m layout.Map) bool { return slices.Contains(r.Fields, m.ID) && m.Biome == biome }) {
			t.Errorf("season %d kept the biome of season %d", n, n-1)
		}
	}
	// The public branch cannot stand in for the keys.
	p, st := corePool(t)
	run := a
	made, err := fields.GenerateLayout(w.g, w.public, run, nil)
	if err != nil {
		t.Fatal(err)
	}
	set := fields.NewSet(made, nil)
	warps, _ := set.LayoutWarps(run)
	only := func(id string) bool { m := run.Map(id); return m != nil && m.Date != "" }
	sm, err := fields.SpawnMapsOf(run, warps, set.Get, only)
	if err != nil {
		t.Fatal(err)
	}
	key := rotation.RotationSecret(rotation.RotationKey(w.secret, d), "2026-10-16")
	real := spawns.Plan(key, sm, p, st)
	for _, fake := range []rng.Node{
		rotation.RotationSecret(rotation.RotationKey(w.public, d), "2026-10-16"),
		rotation.RotationSecret(rotation.RotationKey(w.secret, d), "2026-10-23"),
	} {
		if slices.EqualFunc(spawns.Plan(fake, sm, p, st), real, func(x, y spawns.Planned) bool { return slices.EqualFunc(x.Spawns, y.Spawns, sameSpawn) }) {
			t.Error("another key gave the same spawns")
		}
	}
}

func sameSpawn(a, b spawns.Spawn) bool {
	return a.Monster == b.Monster && a.Count == b.Count && a.RespawnMs == b.RespawnMs && a.Role == b.Role && a.Area == b.Area
}
