//go:build procedural

package spawns_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

var update = flag.Bool("update", false, "rewrite testdata/golden-spawns.json")

// goldenCase is one golden world: a secret and a layout config, and its
// spawn lists, one line per spawn.
type goldenCase struct {
	Secret    string `json:"secret"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Towns     int    `json:"towns"`
	Dungeons  int    `json:"dungeons"`
	FieldSize int    `json:"field_size"`
	// The results.
	Checksum string              `json:"checksum"`
	Maps     map[string][]string `json:"maps"`
}

var goldenCases = []goldenCase{
	{Secret: "golden-spawns-1", Width: 6, Height: 5, Towns: 5, Dungeons: 6, FieldSize: 96},
	{Secret: "golden-spawns-2", Width: 3, Height: 3, Towns: 2, Dungeons: 2, FieldSize: 128},
}

func (c goldenCase) config() layout.Config {
	cfg := layout.DefaultConfig()
	cfg.Width, cfg.Height, cfg.Towns, cfg.Dungeons, cfg.FieldSize = c.Width, c.Height, c.Towns, c.Dungeons, c.FieldSize
	return cfg
}

// line writes a spawn as the golden file keeps it.
func line(s spawns.Spawn) string {
	var avoid []string
	for _, a := range s.Avoid {
		avoid = append(avoid, fmt.Sprintf("%d,%d,%d", a.X, a.Y, a.R))
	}
	return fmt.Sprintf("%s %s x%d %dms area=%d,%d,%d,%d avoid=[%s]", s.Role, s.Monster, s.Count, s.RespawnMs,
		s.Area.X, s.Area.Y, s.Area.W, s.Area.H, strings.Join(avoid, " "))
}

func outcome(c goldenCase, planned []spawns.Planned) goldenCase {
	c.Maps = map[string][]string{}
	h := sha256.New()
	for _, p := range planned {
		for _, s := range p.Spawns {
			l := line(s)
			c.Maps[p.Map] = append(c.Maps[p.Map], l)
			fmt.Fprintf(h, "%s %s\n", p.Map, l)
		}
	}
	c.Checksum = hex.EncodeToString(h.Sum(nil))
	return c
}

// TestGoldenSpawns: the golden worlds' spawn lists match
// testdata/golden-spawns.json on this platform. scripts/check.sh gen runs
// it on arm64 and on amd64. go test -tags procedural -run
// TestGoldenSpawns ./server/spawns -update rewrites the file.
func TestGoldenSpawns(t *testing.T) {
	p, st := corePool(t)
	file := filepath.Join("testdata", "golden-spawns.json")
	if *update {
		var out []goldenCase
		for _, c := range goldenCases {
			w := coreWorld(t, c.Secret, c.config())
			out = append(out, outcome(c, spawns.Plan(w.secret, w.maps, p, st)))
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
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
	var want []goldenCase
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(goldenCases) {
		t.Fatalf("%d golden worlds, want %d", len(want), len(goldenCases))
	}
	for _, c := range want {
		w := coreWorld(t, c.Secret, c.config())
		got := outcome(c, spawns.Plan(w.secret, w.maps, p, st))
		if got.Checksum == c.Checksum {
			continue
		}
		t.Errorf("%s: checksum %s, want %s", c.Secret, got.Checksum, c.Checksum)
		for _, id := range slices.Sorted(func(yield func(string) bool) {
			for k := range c.Maps {
				if !yield(k) {
					return
				}
			}
		}) {
			if !slices.Equal(got.Maps[id], c.Maps[id]) {
				t.Errorf("%s %s:\n got %q\nwant %q", c.Secret, id, got.Maps[id], c.Maps[id])
			}
		}
	}
	t.Logf("%d golden worlds match on %s/%s", len(want), runtime.GOOS, runtime.GOARCH)
}

// TestSpawnDeterministic: a world's spawns are the same each time, and
// a different secret gives different ones.
func TestSpawnDeterministic(t *testing.T) {
	p, st := corePool(t)
	c := goldenCases[1]
	w := coreWorld(t, c.Secret, c.config())
	a, b := spawns.Plan(w.secret, w.maps, p, st), spawns.Plan(w.secret, w.maps, p, st)
	if !reflect.DeepEqual(a, b) {
		t.Error("two plans differ")
	}
	other := spawns.Plan(spawns.SecretBranch(rng.FromKey([32]byte{1})), w.maps, p, st)
	if reflect.DeepEqual(a, other) {
		t.Error("two secrets gave one plan")
	}
}

// checkWorld checks every spawn rule on a world's plan, and returns the
// field bosses of each region.
func checkWorld(t *testing.T, name string, w world, planned []spawns.Planned, p *spawns.Pool, st spawns.Resolved) {
	t.Helper()
	if len(planned) != len(w.maps) {
		t.Fatalf("%s: %d lists for %d maps", name, len(planned), len(w.maps))
	}
	bosses := map[string]int{}
	eligible := map[string]int{}
	for i, pl := range planned {
		m := w.maps[i]
		if pl.Map != m.ID {
			t.Fatalf("%s: list %d is %s, the map %s", name, i, pl.Map, m.ID)
		}
		for _, problem := range spawns.Check(m, pl.Spawns, p.Monster) {
			t.Errorf("%s %s: %s", name, m.ID, problem)
		}
		if m.Kind == "field" && !m.NextToTown {
			eligible[m.Region]++
		}
		var common, uncommon, mini, field int
		for _, s := range pl.Spawns {
			mon, _ := p.Monster(s.Monster)
			switch s.Role {
			case spawns.Common:
				common += s.Count
			case spawns.Uncommon:
				uncommon += s.Count
			case spawns.MiniBoss:
				mini++
				if mon.Level > m.Band[1]+spawns.MiniBossAbove || mon.Level < m.Band[0] && hasBossIn(p, spawns.MiniBoss, m.Band[0], m.Band[1]+spawns.MiniBossAbove) {
					t.Errorf("%s %s: the mini boss %s is level %d in %v", name, m.ID, s.Monster, mon.Level, m.Band)
				}
			case spawns.FieldBoss:
				field++
				if mon.Level < m.Band[0] {
					t.Errorf("%s %s: the field boss %s is level %d, below %v", name, m.ID, s.Monster, mon.Level, m.Band)
				}
			case spawns.Plant:
				if len(m.FeaturePoints) > 0 && !slices.ContainsFunc(m.FeaturePoints, func(f spawns.Point) bool {
					return f.X == s.Area.X && f.Y == s.Area.Y && s.Area.W == spawns.PlantReach
				}) {
					t.Errorf("%s %s: the plant %s is not near a feature: %+v", name, m.ID, s.Monster, s.Area)
				}
			}
			if s.Role != spawns.Plant && !s.Role.Boss() && !s.Area.Whole() {
				t.Errorf("%s %s: %s has an area", name, m.ID, s.Monster)
			}
		}
		if common <= uncommon {
			t.Errorf("%s %s: %d common monsters and %d uncommon", name, m.ID, common, uncommon)
		}
		switch {
		case m.Kind == "field" && m.NextToTown:
			if want := st.TownFieldBossRole; mini+field > 1 || want == spawns.MiniBoss && field > 0 || want == spawns.FieldBoss && mini > 0 {
				t.Errorf("%s %s: next to a town with %d field bosses and %d mini bosses, the role %s", name, m.ID, field, mini, want)
			}
			if st.TownFieldBossChance == 100 && mini+field != 1 {
				t.Errorf("%s %s: no boss at a chance of 100", name, m.ID)
			}
		case m.Kind == "field":
			if mini > 0 || field > 1 {
				t.Errorf("%s %s: %d mini bosses and %d field bosses", name, m.ID, mini, field)
			}
			bosses[m.Region] += field
		case m.Floor == m.Floors && m.BossRoom != nil:
			i := slices.IndexFunc(pl.Spawns, func(s spawns.Spawn) bool { return s.Role == spawns.FieldBoss })
			if i < 0 || mini > 0 || field != 1 || pl.Spawns[i].Area != (spawns.Area{X: m.BossRoom.X, Y: m.BossRoom.Y, W: spawns.BossRoomReach, H: spawns.BossRoomReach}) {
				t.Errorf("%s %s: the last floor's bosses %+v", name, m.ID, pl.Spawns)
			}
		default:
			if mini+field > 0 {
				t.Errorf("%s %s: a floor with a boss", name, m.ID)
			}
		}
	}
	for r, n := range eligible {
		if got := bosses[r]; got < 1 || got > 2 || got > n {
			t.Errorf("%s: region %s has %d field bosses on %d fields", name, r, got, n)
		}
	}
}

func hasBossIn(p *spawns.Pool, role spawns.Role, lo, hi int) bool {
	return slices.ContainsFunc(p.Monsters(), func(m spawns.Monster) bool {
		return m.Tags.Role == role && m.Level >= lo && m.Level <= hi
	})
}

// TestSpawnRules: the default world of many seeds meets every rule of
// strategy section 8.11 and review decision 44 on every field and floor:
// species, roles, elements, aggressive limits, distances from arrivals
// and warps to towns, and the boss rules, also with the settings.
func TestSpawnRules(t *testing.T) {
	p, st := corePool(t)
	cfg := layout.DefaultConfig()
	cfg.FieldSize = 96
	for i := range 12 {
		w := coreWorld(t, fmt.Sprintf("rules-%d", i), cfg)
		s := st
		if i%3 == 1 {
			s.TownFieldBossChance, s.TownFieldBossRole = 100, spawns.FieldBoss
		}
		if i%3 == 2 {
			s.TownFieldBossChance = 100
		}
		checkWorld(t, fmt.Sprintf("rules-%d", i), w, spawns.Plan(w.secret, w.maps, p, s), p, s)
	}
}

// TestSpawnDensity: counts follow the density and the walkable cells.
func TestSpawnDensity(t *testing.T) {
	p, st := corePool(t)
	m := spawns.Map{ID: "gen:field:0:0", Kind: "field", Biome: "grassland", Band: [2]int{1, 10}, Walkable: 50000}
	node := rng.FromKey([32]byte{5})
	total := func(list []spawns.Spawn) int {
		n := 0
		for _, s := range list {
			n += s.Count
		}
		return n
	}
	base := total(spawns.Select(node, m, p, st))
	want := 50000 * st.FieldDensity / 10000
	if base < want-10 || base > want+10 {
		t.Errorf("%d monsters on 50,000 cells, want about %d", base, want)
	}
	st.FieldDensity *= 2
	if double := total(spawns.Select(node, m, p, st)); double < 2*want-20 {
		t.Errorf("%d monsters at double density, want about %d", double, 2*want)
	}
}

// TestPublicSeed: what the client gets reveals nothing of the spawns
// (the phase 7 exit criteria, decision 0046).
// The spawn lists come from the secret branch only: the same public
// branch, maps and layout with another secret give other lists, and
// every node a client can derive from the public branch, used in its
// place, gives lists that differ from the world's. The maps the server
// sends name no monster.
func TestPublicSeed(t *testing.T) {
	p, st := corePool(t)
	c := goldenCases[1]
	w := coreWorld(t, c.Secret, c.config())
	real := spawns.Plan(w.secret, w.maps, p, st)
	guesses := map[string]rng.Node{
		"the public branch":                w.public,
		"the public branch's secret child": spawns.SecretBranch(w.public),
		"the public key as a root":         spawns.SecretBranch(rng.FromKey(w.public.Key())),
		"the layout node":                  layout.LayoutNode(w.public),
		"a field's node":                   gen.FieldNode(w.public, 0, 0),
	}
	for name, guess := range guesses {
		got := spawns.Plan(guess, w.maps, p, st)
		same := 0
		for i := range got {
			if reflect.DeepEqual(got[i].Spawns, real[i].Spawns) {
				same++
			}
		}
		if same > len(got)/4 {
			t.Errorf("%s gives %d of %d lists of the world", name, same, len(got))
		}
	}
	// A new world's lists, with the economy checks that redraw and
	// balance fields (decision 0046), come from the secret branch too.
	ep, est := econPool(t)
	realEcon := spawns.Plan(w.secret, w.maps, ep, est)
	for name, guess := range guesses {
		got := spawns.Plan(guess, w.maps, ep, est)
		same := 0
		for i := range got {
			if reflect.DeepEqual(got[i].Spawns, realEcon[i].Spawns) {
				same++
			}
		}
		if same > len(got)/4 {
			t.Errorf("%s gives %d of %d lists of the world with the economy checks", name, same, len(got))
		}
	}
	// The keys the game role gets for the rotating dungeons' and the
	// seasons' spawns (decision 0045) are not any node a client derives.
	for name, guess := range guesses {
		if rotation.RotationKey(guess, "d1").Key() == rotation.RotationKey(w.secret, "d1").Key() ||
			rotation.SeasonKey(guess).Key() == rotation.SeasonKey(w.secret).Key() {
			t.Errorf("%s gives a rotation or season key of the world", name)
		}
	}
	// The spawns depend on the secret: other secrets with the same maps
	// give other lists.
	for i := range 4 {
		other := spawns.Plan(spawns.SecretBranch(rng.FromKey([32]byte{byte(i + 1)})), w.maps, p, st)
		if reflect.DeepEqual(other, real) {
			t.Errorf("secret %d gives the world's lists", i)
		}
	}
	// The map info and cells the server sends hold no monster name.
	names := map[string]bool{}
	for _, pl := range real {
		for _, s := range pl.Spawns {
			names[s.Monster] = true
		}
	}
	for _, f := range w.set.Fields() {
		b, err := json.Marshal(struct {
			Places any
			Levels [2]int
			Name   string
		}{f.Field.Places, f.Levels, f.DisplayName})
		if err != nil {
			t.Fatal(err)
		}
		for n := range names {
			if strings.Contains(string(b), n) || strings.Contains(string(f.Field.WMAP), n) {
				t.Errorf("%s's data names %s", f.ID, n)
			}
		}
	}
}

// FuzzSelect: any node, band, biome and size give a list that meets the
// rules, with a boss that keeps its distances.
func FuzzSelect(f *testing.F) {
	f.Add("seed", uint8(1), uint8(0), uint16(20000), false)
	f.Add("x", uint8(95), uint8(7), uint16(500), true)
	biomes := []string{"grassland", "forest", "desert", "snow", "swamp", "beach", "cave", "crypt", "tower", "ruins", "moon"}
	f.Fuzz(func(t *testing.T, secret string, lo, biome uint8, walkable uint16, next bool) {
		p, st := corePool(t)
		low := int(lo%90) + 1
		m := spawns.Map{ID: "gen:field:1:1", Kind: "field", Biome: biomes[int(biome)%len(biomes)], Band: [2]int{low, low + 9},
			Walkable: int(walkable), NextToTown: next, Region: "t1",
			Arrivals: []spawns.Point{{X: 3, Y: 40}, {X: 60, Y: 3}}, TownPortals: []spawns.Point{{X: 0, Y: 40}},
			FeaturePoints: []spawns.Point{{X: 30, Y: 30}}, Features: []string{"water"}}
		root, err := rng.NewRoot([]byte("fuzz:" + secret))
		if err != nil {
			t.Fatal(err)
		}
		st.TownFieldBossChance = 100
		planned := spawns.Plan(spawns.SecretBranch(root), []spawns.Map{m}, p, st)
		if len(planned) != 1 || len(planned[0].Spawns) == 0 {
			t.Fatalf("%+v", planned)
		}
		for _, problem := range spawns.Check(m, planned[0].Spawns, p.Monster) {
			t.Errorf("%v: %s", m, problem)
		}
	})
}
