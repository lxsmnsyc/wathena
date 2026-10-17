package economy_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/economy/core"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
)

var update = flag.Bool("update", false, "rewrite core/data/economy_targets.yml")

var (
	worldOnce sync.Once
	world     *economy.World
	monsters  map[uint32]gamedata.Monster
	worldErr  error
)

// coreWorld is the simulator on core's renewal data.
func coreWorld(t testing.TB) *economy.World {
	t.Helper()
	worldOnce.Do(func() {
		cfg, err := economy.LoadConfig()
		if err != nil {
			worldErr = err
			return
		}
		d, err := gamedata.LoadItemData(reference.Renewal)
		if err != nil {
			worldErr = err
			return
		}
		tables, prog, err := gamedata.LoadProgression(reference.Renewal)
		if err != nil {
			worldErr = err
			return
		}
		jobs, _, err := gamedata.StartCharacter(reference.Renewal, "all_1")
		if err != nil {
			worldErr = err
			return
		}
		if monsters, err = gamedata.LoadMonsters(reference.Renewal, tables, prog); err != nil {
			worldErr = err
			return
		}
		if worldErr = gamedata.ExtractSkills(reference.Renewal, monsters); worldErr != nil {
			return
		}
		world = gamedata.EconomyWorld(cfg, item.New(d), jobs, tables, nil, monsters)
	})
	if worldErr != nil {
		t.Fatal(worldErr)
	}
	return world
}

// classicName finds a monster's name and level by ID.
func classicName(id uint32) (string, int, bool) {
	m, ok := monsters[id]
	if !ok || m.Def == nil {
		return "", 0, false
	}
	return m.AegisName, m.Def.Level, true
}

// TestTargets: the committed targets are what the simulator gives on
// rAthena's renewal classic fields. go test -run TestTargets
// ./server/economy -update rewrites them.
func TestTargets(t *testing.T) {
	w := coreWorld(t)
	fields, err := economy.ClassicFields(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	ts, derived, err := w.DeriveTargets(fields, classicName)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range derived {
		t.Logf("%-14s level %2d band %v: %d kills/h, base %d/h, job %d/h, zeny %d/h, supplies %d/h",
			d.Map, d.Level, d.Band, d.Rates.Kills, d.Rates.BaseExp, d.Rates.JobExp, d.Rates.Zeny, d.Rates.Supplies)
	}
	b := economy.TargetsFile(ts)
	file := filepath.Join("core", core.TargetsFile)
	if *update {
		if err := os.WriteFile(file, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, b) {
		t.Errorf("%s differs from the derived targets; run with -update:\n%s", file, b)
	}
	for _, x := range ts {
		t.Logf("band %v level %d: %d fields, base %d/h, job %d/h, zeny %d/h, supplies %d/h", x.Levels, x.Level, x.Fields, x.BaseExp, x.JobExp, x.Zeny, x.Supplies)
	}
}

// TestModel: the model player of every level from 1 to 99 spends its
// status points, wears its gear, and gets stronger with its level.
func TestModel(t *testing.T) {
	w := coreWorld(t)
	prevAtk := 0
	for level := 1; level <= 99; level++ {
		m, err := w.Model(level)
		if err != nil {
			t.Fatal(err)
		}
		if m.Level != level || m.Status.MaxHP < 1 || m.Heal < 1 || m.PotionPrice < 1 || len(m.Gear) == 0 {
			t.Fatalf("level %d: %+v", level, m.Player)
		}
		for i, pos := range m.Positions {
			if pos == 0 {
				t.Errorf("level %d: item %d is not worn", level, m.Gear[i])
			}
		}
		left := w.Tables.StatusPoints(level)
		st := m.Stats
		// The weights put STR first, then VIT, DEX and AGI.
		if st.Str < st.Vit || st.Vit < st.Dex || st.Dex < st.Agi || st.Int != 1 || st.Luk != 1 {
			t.Errorf("level %d: stats %+v", level, st)
		}
		if left < 48 {
			t.Errorf("level %d: %d status points", level, left)
		}
		atk := m.Status.BAtk + m.Status.RHW.Atk
		if atk < prevAtk && level != 11 && level != 50 {
			t.Errorf("level %d: ATK %d below the level before's %d", level, atk, prevAtk)
		}
		prevAtk = atk
	}
}

// TestSimulate: the simulator's rates follow its inputs.
func TestSimulate(t *testing.T) {
	w := coreWorld(t)
	f := economy.Field{Walkable: 40000, Lines: []economy.Line{
		{Monster: "PORING", Count: 100, RespawnMs: 5000},
		{Monster: "LUNATIC", Count: 60, RespawnMs: 5000},
	}}
	base, err := w.Simulate(5, f)
	if err != nil {
		t.Fatal(err)
	}
	if base.Kills < 100 || base.BaseExp < 1 || base.JobExp < 1 || base.Zeny < 1 || len(base.Lines) != 2 {
		t.Fatalf("%+v", base)
	}
	// A crowded field is faster to hunt.
	crowded := f
	crowded.Walkable = 4000
	if r, _ := w.Simulate(5, crowded); r.Kills <= base.Kills || r.BaseExp <= base.BaseExp {
		t.Errorf("a crowded field %+v, the field %+v", r, base)
	}
	// Unknown monsters are left out, and a field of them gives nothing.
	if r, _ := w.Simulate(5, economy.Field{Walkable: 100, Lines: []economy.Line{{Monster: "NOPE", Count: 5, RespawnMs: 5000}}}); r.Kills != 0 {
		t.Errorf("unknown monsters %+v", r)
	}
	// A line that comes back slowly is capped by its respawn.
	slow := economy.Field{Walkable: 40000, Lines: []economy.Line{
		{Monster: "PORING", Count: 100, RespawnMs: 5000},
		{Monster: "LUNATIC", Count: 2, RespawnMs: 3_600_000},
	}}
	r, _ := w.Simulate(5, slow)
	if r.Lines[1] > 2000 {
		t.Errorf("a line of 2 that returns hourly gives %d.%03d kills an hour", r.Lines[1]/1000, r.Lines[1]%1000)
	}
	// The same input gives the same result.
	if again, _ := w.Simulate(5, f); again.BaseExp != base.BaseExp || again.Zeny != base.Zeny {
		t.Errorf("%+v then %+v", base, again)
	}
}

// TestTargetsAt: targets between bands are on the line between them, and
// past the ends they are the ends'.
func TestTargetsAt(t *testing.T) {
	ts := economy.Targets{
		{Levels: [2]int{1, 10}, Level: 5, BaseExp: 100, Zeny: 10},
		{Levels: [2]int{11, 20}, Level: 15, BaseExp: 200, Zeny: 30},
	}
	for _, c := range []struct {
		level     int
		exp, zeny int64
	}{{1, 100, 10}, {5, 100, 10}, {10, 150, 20}, {13, 180, 26}, {15, 200, 30}, {99, 200, 30}} {
		if got := ts.At(c.level); got.BaseExp != c.exp || got.Zeny != c.zeny || got.Level != c.level {
			t.Errorf("level %d: %+v", c.level, got)
		}
	}
	if !economy.Within(130, 100, 30) || economy.Within(131, 100, 30) || !economy.Within(70, 100, 30) || economy.Within(69, 100, 30) {
		t.Error("Within")
	}
	if p := economy.Check(economy.Rates{BaseExp: 131, Zeny: 10}, ts[0], 30); len(p) != 1 {
		t.Errorf("Check %v", p)
	}
	loaded, err := economy.LoadTargets()
	if err != nil || len(loaded) != len(economy.Bands()) {
		t.Fatalf("%d targets: %v", len(loaded), err)
	}
	for i, x := range loaded {
		if x.Levels != economy.Bands()[i] || x.Fields < 3 {
			t.Errorf("target %d: %+v", i, x)
		}
	}
}
