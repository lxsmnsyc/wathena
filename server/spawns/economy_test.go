//go:build procedural

package spawns_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
)

var (
	econOnce    sync.Once
	econSim     *economy.World
	econTargets economy.Targets
	econErr     error
)

// econWorld is the economy simulator on core's renewal data, with core's
// targets.
func econWorld(t testing.TB) (*economy.World, economy.Targets) {
	t.Helper()
	econOnce.Do(func() {
		cfg, err := economy.LoadConfig()
		if err != nil {
			econErr = err
			return
		}
		if econTargets, err = economy.LoadTargets(); err != nil {
			econErr = err
			return
		}
		d, err := gamedata.LoadItemData(reference.Renewal)
		if err != nil {
			econErr = err
			return
		}
		tables, prog, err := gamedata.LoadProgression(reference.Renewal)
		if err != nil {
			econErr = err
			return
		}
		jobs, _, err := gamedata.StartCharacter(reference.Renewal, "all_1")
		if err != nil {
			econErr = err
			return
		}
		mons, err := gamedata.LoadMonsters(reference.Renewal, tables, prog)
		if err != nil {
			econErr = err
			return
		}
		if econErr = gamedata.ExtractSkills(reference.Renewal, mons); econErr != nil {
			return
		}
		econSim = gamedata.EconomyWorld(cfg, item.New(d), jobs, tables, nil, mons)
	})
	if econErr != nil {
		t.Fatal(econErr)
	}
	return econSim, econTargets
}

// econPool is core's pool with the settings of a new world: the economy
// checks on, with core's simulator and targets.
func econPool(t testing.TB) (*spawns.Pool, spawns.Resolved) {
	t.Helper()
	p, st := corePool(t)
	w, ts := econWorld(t)
	st.Rate = gamedata.EconomyRater(w, ts)
	return p, st
}

// TestEconomyChecks: with the economy checks of a new world, every field
// of many default worlds is within the tolerance of its band's targets
// in the simulator, its monsters' median level is within FitLevels of
// its band, and every field and floor still meets the spawn rules of
// strategy section 8.11 (the phase 7 exit criteria, decision
// 0046). Without the checks, many fields miss.
func TestEconomyChecks(t *testing.T) {
	testrace.SkipUnderRace(t, "it plans 24 worlds on one goroutine")
	p, st := econPool(t)
	w, ts := econWorld(t)
	plain := st
	plain.Rate = nil
	cfg := layout.DefaultConfig()
	fields, missed := 0, 0
	byBand := map[[2]int][]int64{}
	for i := range 24 {
		name := fmt.Sprintf("economy-%d", i)
		cw := coreWorld(t, name, cfg)
		planned := spawns.Plan(cw.secret, cw.maps, p, st)
		checkWorld(t, name, cw, planned, p, st)
		before := spawns.Plan(cw.secret, cw.maps, p, plain)
		for j, pl := range planned {
			m := cw.maps[j]
			if m.Kind != "field" {
				continue
			}
			fields++
			level := economy.Middle(m.Band)
			target := ts.At(level)
			r, err := w.Simulate(level, gamedata.EconomyField(m, pl.Spawns))
			if err != nil {
				t.Fatal(err)
			}
			for _, problem := range economy.Check(r, target, w.Config.Tolerance.Generator) {
				t.Errorf("%s %s %v: %s", name, m.ID, m.Band, problem)
			}
			var levels []int
			for _, s := range pl.Spawns {
				if mon, ok := w.Monster(s.Monster); ok && !s.Role.Boss() {
					for range s.Count {
						levels = append(levels, mon.Reward.Level)
					}
				}
			}
			slices.Sort(levels)
			if med := levels[(len(levels)-1)/2]; med < m.Band[0]-gamedata.FitLevels || med > m.Band[1]+gamedata.FitLevels {
				t.Errorf("%s %s %v: the monsters' median level is %d", name, m.ID, m.Band, med)
			}
			byBand[m.Band] = append(byBand[m.Band], economy.Ratio(r.BaseExp, target.BaseExp), economy.Ratio(r.Zeny, target.Zeny))
			b, err := w.Simulate(level, gamedata.EconomyField(m, before[j].Spawns))
			if err != nil {
				t.Fatal(err)
			}
			if len(economy.Check(b, target, w.Config.Tolerance.Generator)) > 0 {
				missed++
			}
		}
	}
	bands := make([][2]int, 0, len(byBand))
	for k := range byBand {
		bands = append(bands, k)
	}
	slices.SortFunc(bands, func(a, b [2]int) int { return a[0] - b[0] })
	for _, band := range bands {
		v := byBand[band]
		lo, hi := v[0], v[0]
		for _, x := range v {
			lo, hi = min(lo, x), max(hi, x)
		}
		t.Logf("band %v: %d fields, base exp and zeny from %d%% to %d%% of the targets", band, len(v)/2, lo, hi)
	}
	t.Logf("%d fields pass; without the checks %d of them miss", fields, missed)
	if missed == 0 {
		t.Error("no field missed without the checks, so the test shows nothing")
	}
}

// TestEconomyOff: with spawns.economy off, a world's plan is the plan
// without a Rate.
func TestEconomyOff(t *testing.T) {
	p, st := econPool(t)
	plain := st
	plain.Rate = nil
	off := st
	off.Economy = false
	cw := coreWorld(t, goldenCases[1].Secret, goldenCases[1].config())
	a, b := spawns.Plan(cw.secret, cw.maps, p, off), spawns.Plan(cw.secret, cw.maps, p, plain)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Error("the plan with the checks off differs from the plan without a Rate")
	}
	if c := spawns.Plan(cw.secret, cw.maps, p, st); fmt.Sprint(c) == fmt.Sprint(a) {
		t.Error("the checks changed no list of the world")
	}
}

// TestBalance: Balance moves monsters within a role, which keeps the
// roles' totals, and scales the density, at most to twice the first
// total.
func TestBalance(t *testing.T) {
	m := spawns.Map{ID: "gen:field:0:0", Kind: "field", Band: [2]int{1, 10}, Walkable: 10000}
	list := []spawns.Spawn{
		{Monster: "A", Count: 40, RespawnMs: 5000, Role: spawns.Common},
		{Monster: "B", Count: 40, RespawnMs: 5000, Role: spawns.Common},
		{Monster: "C", Count: 20, RespawnMs: 9000, Role: spawns.Uncommon},
	}
	total := func(l []spawns.Spawn) int {
		n := 0
		for _, s := range l {
			n += s.Count
		}
		return n
	}
	// A rating that wants 70 or more B, of 100 monsters.
	wantB := func(_ spawns.Map, l []spawns.Spawn) int {
		miss := 0
		if total(l) != 100 {
			miss = 100
		}
		for _, s := range l {
			if s.Monster == "B" {
				return miss + max(70-s.Count, 0)
			}
		}
		return 1000
	}
	got, miss := spawns.Balance(m, list, wantB)
	if miss != 0 {
		t.Fatalf("miss %d: %+v", miss, got)
	}
	sum := map[spawns.Role]int{}
	for _, s := range got {
		sum[s.Role] += s.Count
	}
	if sum[spawns.Common] != 80 || sum[spawns.Uncommon] != 20 {
		t.Errorf("role totals %v", sum)
	}
	if list[1].Count != 40 {
		t.Error("Balance changed its input")
	}
	// A rating that wants more monsters scales them all.
	wantMore := func(_ spawns.Map, l []spawns.Spawn) int { return max(150-total(l), 0) }
	if got, miss := spawns.Balance(m, list, wantMore); miss != 0 || total(got) < 150 {
		t.Errorf("miss %d: %+v", miss, got)
	}
	// Never past twice the first total.
	wantMany := func(_ spawns.Map, l []spawns.Spawn) int { return max(1000-total(l), 0) }
	if got, _ := spawns.Balance(m, list, wantMany); total(got) > 200 || total(got) <= 100 {
		t.Errorf("%d monsters after balancing toward 1,000 from 100", total(got))
	}
}

// TestGoldenEconomySpawns: a default world's spawn lists with the
// economy checks match testdata/golden-spawns-economy.json on this
// platform, so the checks pick the same lists on arm64 and amd64.
// scripts/check.sh gen runs it on both. -update rewrites the file.
func TestGoldenEconomySpawns(t *testing.T) {
	p, st := econPool(t)
	c := goldenCase{Secret: "golden-economy-1", Width: 6, Height: 5, Towns: 5, Dungeons: 6, FieldSize: 300}
	file := filepath.Join("testdata", "golden-spawns-economy.json")
	cw := coreWorld(t, c.Secret, c.config())
	got := outcome(c, spawns.Plan(cw.secret, cw.maps, p, st))
	if *update {
		b, err := json.MarshalIndent(got, "", "  ")
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
	var want goldenCase
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if got.Checksum != want.Checksum {
		t.Errorf("checksum %s, want %s", got.Checksum, want.Checksum)
		for id, lines := range want.Maps {
			if !slices.Equal(got.Maps[id], lines) {
				t.Errorf("%s:\n got %q\nwant %q", id, got.Maps[id], lines)
			}
		}
	}
	t.Logf("the golden economy world matches on %s/%s", runtime.GOOS, runtime.GOARCH)
}
