package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// Jobs of the level path: a Novice, a Swordman, a Knight, a High
// Novice and a Lord Knight.
const (
	jobNovice     = 0
	jobSwordman   = 1
	jobKnight     = 7
	jobHighNovice = 4001
	jobLordKnight = 4008
)

// TestLevelPath walks the level path of default new worlds, as `wactl
// world new` makes them (the phase 7 exit criteria, decision 0046): a
// world with at least 5 towns, 20 fields and 5 dungeons is playable
// from level 1 to the transcendent level cap.
//
//   - The world has 5 towns, 20 fields and 5 dungeons or more, and every
//     map is reachable from the home area.
//   - Each field is reachable from the home area through maps whose band
//     starts no higher than its own, so a player never has to cross a
//     harder map to reach the next one.
//   - Every level from 1 to the transcendent cap lies in the band of 3
//     fields or more whose spawns fit: their monsters, by count, have a
//     median level within 10 of the band, and the field is within the
//     economy tolerance of its band's targets.
//   - Each class family has a job guild in a town, with tests for the
//     first and the second job. Every guild stands in a town of a band
//     that starts at level 20 or lower, so a Novice of job level 10
//     reaches it (decision 0047). The rebirth place stands in a town of the
//     highest town band. A reborn High Novice climbs the same fields
//     from level 1 again, to the Lord Knight's cap of 99.
//
// A bot does not play from 1 to 99: the test proves the path's parts.
func TestLevelPath(t *testing.T) {
	testrace.SkipUnderRace(t, "it makes three worlds on one goroutine")
	g, ok := gen.Latest()
	if !ok {
		t.Skip("a build without the procedural tag")
	}
	for _, seed := range []string{"level-path-1", "level-path-2", "level-path-3"} {
		pw, err := proceduralWorld("", "renewal", []byte(seed), g.Version)
		if err != nil {
			t.Fatal(err)
		}
		levelPath(t, seed, pw)
	}
}

func levelPath(t *testing.T, seed string, pw *madeWorld) {
	t.Helper()
	l := pw.layout
	towns, fields, dungeons := 0, 0, map[string]bool{}
	for _, m := range l.Maps {
		switch m.Kind {
		case gen.KindTown:
			towns++
		case gen.KindField:
			fields++
		case gen.KindDungeon:
			dungeons[m.Dungeon] = true
		}
	}
	if towns < 5 || fields < 20 || len(dungeons) < 5 {
		t.Errorf("%s: %d towns, %d fields and %d dungeons", seed, towns, fields, len(dungeons))
	}
	// Reachability, and the easiest way to each map: the highest band
	// start a walk from home must cross, which the search keeps as low
	// as it can.
	next := map[string][]string{}
	for _, k := range l.Links {
		next[k.A.Map] = append(next[k.A.Map], k.B.Map)
		next[k.B.Map] = append(next[k.B.Map], k.A.Map)
	}
	start := func(id string) int {
		m := l.Map(id)
		if m == nil || m.Kind == gen.KindTown || m.Kind == layout.KindHome {
			// Towns and the home map have no monsters to cross.
			return 0
		}
		return m.Levels[0]
	}
	hardest := map[string]int{l.Home: 0}
	for changed := true; changed; {
		changed = false
		for from, h := range hardest {
			for _, to := range next[from] {
				x := max(h, start(to))
				if old, ok := hardest[to]; !ok || x < old {
					hardest[to] = x
					changed = true
				}
			}
		}
	}
	for _, m := range l.Maps {
		h, ok := hardest[m.ID]
		switch {
		case !ok:
			t.Errorf("%s: %s is not reachable from the home area", seed, m.ID)
		case m.Kind == gen.KindField && h > m.Levels[0]:
			t.Errorf("%s: the way to %s %v crosses a map from level %d", seed, m.ID, m.Levels, h)
		}
	}
	// Fields whose spawns fit their band.
	lists := map[string]int{}
	for i, p := range pw.spawns {
		lists[p.Map] = i
	}
	tol := pw.econ.Config.Tolerance.Generator
	var fitting [][2]int
	for _, m := range pw.maps {
		if m.Kind != gen.KindField {
			continue
		}
		list := pw.spawns[lists[m.ID]].Spawns
		var levels []int
		for _, s := range list {
			mon, ok := pw.econ.Monster(s.Monster)
			if s.Role.Boss() || !ok {
				continue
			}
			for range s.Count {
				levels = append(levels, mon.Reward.Level)
			}
		}
		slices.Sort(levels)
		if len(levels) == 0 {
			t.Errorf("%s: %s has no monsters", seed, m.ID)
			continue
		}
		median := levels[(len(levels)-1)/2]
		level := economy.Middle(m.Band)
		r, err := pw.econ.Simulate(level, gamedata.EconomyField(m, list))
		if err != nil {
			t.Fatal(err)
		}
		problems := economy.Check(r, pw.targets.At(level), tol)
		if median < m.Band[0]-10 || median > m.Band[1]+10 || len(problems) > 0 {
			t.Errorf("%s: %s %v has monsters of median level %d: %v", seed, m.ID, m.Band, median, problems)
			continue
		}
		fitting = append(fitting, m.Band)
	}
	maxLevel := pw.econ.Tables.MaxBaseLevel(jobLordKnight)
	for _, job := range []int{jobNovice, jobSwordman, jobKnight, jobHighNovice} {
		if c := pw.econ.Tables.MaxBaseLevel(job); c > maxLevel {
			t.Errorf("job %d reaches level %d, past the Lord Knight's %d", job, c, maxLevel)
		}
	}
	if maxLevel != 99 {
		t.Errorf("the transcendent level cap is %d, not 99", maxLevel)
	}
	for level := 1; level <= maxLevel; level++ {
		n := 0
		for _, b := range fitting {
			if b[0] <= level && level <= b[1] {
				n++
			}
		}
		if n == 0 {
			t.Errorf("%s: no fitting field for level %d", seed, level)
		}
	}
	for _, band := range economy.Bands() {
		n := 0
		for _, b := range fitting {
			if b[0] <= band[1] && band[0] <= b[1] {
				n++
			}
		}
		if n < 3 {
			t.Errorf("%s: %d fitting fields in the band %v", seed, n, band)
		}
	}
	// Job guilds and the rebirth place.
	top := 0
	for _, m := range l.Maps {
		if m.Kind == gen.KindTown {
			top = max(top, m.Levels[0])
		}
	}
	stages := map[string]int{}
	for _, q := range pw.content.Quests {
		if q.Purpose == content.PurposeGuild {
			stages[q.Guild+" "+q.Stage]++
		}
	}
	for _, f := range content.Families {
		at := ""
		for _, p := range pw.content.Places {
			if slices.Contains(p.Tags, content.GuildTag(f)) {
				at = p.Map
			}
		}
		m := l.Map(at)
		switch {
		case m == nil || m.Kind != gen.KindTown:
			t.Errorf("%s: no %s guild in a town", seed, f)
		case m.Levels[0] > content.GuildLevel:
			// Every family: a Novice of job level 10 reaches its guild
			// without crossing harder fields (decision 0047).
			t.Errorf("%s: the %s guild is in %s %v", seed, f, at, m.Levels)
		}
		for _, st := range content.Stages {
			if stages[f+" "+st] == 0 {
				t.Errorf("%s: the %s guild has no %s test", seed, f, st)
			}
		}
	}
	rebirth := ""
	for _, p := range pw.content.Places {
		if slices.Contains(p.Tags, content.TagRebirth) {
			rebirth = p.Map
		}
	}
	if m := l.Map(rebirth); m == nil || m.Kind != gen.KindTown || m.Levels[0] < top {
		t.Errorf("%s: the rebirth place is in %q, not a town of the highest town band from %d", seed, rebirth, top)
	}
	t.Logf("%s: %d towns, %d fields, %d dungeons; %d fitting fields; guilds in %s; rebirth in %s", seed, towns, fields, len(dungeons),
		len(fitting), fmt.Sprint(guildTowns(pw.content)), rebirth)
}

func guildTowns(w *content.World) []string {
	var out []string
	for _, p := range w.Places {
		for _, tag := range p.Tags {
			if len(tag) > 6 && tag[:6] == "guild:" {
				out = append(out, tag[6:]+"@"+p.Map)
			}
		}
	}
	return out
}

// TestSimEconomy: `wactl sim economy` prints every field of a new world
// with its rates, each passing, the floors with --floors, and the
// targets (decision 0046).
func TestSimEconomy(t *testing.T) {
	g, ok := gen.Latest()
	if !ok {
		t.Skip("a build without the procedural tag")
	}
	pw, err := proceduralWorld("", "renewal", []byte("sim-economy-1"), g.Version)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := printEconomy(&b, pw, true); err != nil {
		t.Fatalf("%v:\n%s", err, b.String())
	}
	out := b.String()
	fields := 0
	for _, m := range pw.layout.Maps {
		if m.Kind == gen.KindField {
			fields++
			if !strings.Contains(out, m.ID) {
				t.Errorf("%s is missing:\n%s", m.ID, out)
			}
		}
	}
	if strings.Count(out, " pass") != fields || !strings.Contains(out, "floor") || !strings.Contains(out, "levels 91-99") {
		t.Errorf("the table:\n%s", out)
	}
	if err := simEconomy([]string{"--seed", "x", "extra"}); err == nil {
		t.Error("an extra argument was taken")
	}
}
