package gamedata

import (
	"slices"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/status"
)

// EconomyWorld is the economy simulator's view of loaded game data
// (decision 0046): the items, the job and exp tables, the ruleset's
// flags, and the monsters by name.
func EconomyWorld(cfg *economy.Config, items *item.Table, jobs *status.Tables, tables *progression.Tables,
	rl *rules.Rules, monsters map[uint32]Monster) *economy.World {
	byName := make(map[string]economy.Monster, len(monsters))
	for _, m := range monsters {
		if m.AegisName == "" || m.Status == nil || m.Reward == nil {
			continue
		}
		byName[m.AegisName] = economy.Monster{Name: m.AegisName, Status: m.Status, Reward: m.Reward, Skills: m.Skills}
	}
	return &economy.World{Config: cfg, Items: items, Jobs: jobs, Tables: tables, Rules: rl,
		Monster: func(name string) (economy.Monster, bool) {
			m, ok := byName[name]
			return m, ok
		}}
}

// EconomyField is the hunt of a field's spawn list: its lines without
// bosses, which the model player leaves alone.
func EconomyField(m spawns.Map, list []spawns.Spawn) economy.Field {
	f := economy.Field{Walkable: m.Walkable}
	for _, s := range list {
		if !s.Role.Boss() {
			f.Lines = append(f.Lines, economy.Line{Monster: s.Monster, Count: s.Count, RespawnMs: s.RespawnMs})
		}
	}
	return f
}

// EconomyRater rates fields for the economy checks of spawn selection
// (decision 0046): the model player of the band's middle level hunts the
// field, and each of base exp and zeny per hour that is more than the
// generator tolerance from the band's target adds the percent beyond
// it. A median monster level more than FitLevels from the band adds 10
// for each level beyond, so the redraws keep fitting monsters. A field
// the simulator cannot rate passes.
func EconomyRater(w *economy.World, ts economy.Targets) spawns.Rater {
	tol := int64(w.Config.Tolerance.Generator)
	return func(m spawns.Map, list []spawns.Spawn) int {
		level := economy.Middle(m.Band)
		r, err := w.Simulate(level, EconomyField(m, list))
		if err != nil {
			return 0
		}
		t := ts.At(level)
		// The monsters must still fit the band: their median level, by
		// count, within FitLevels of it.
		miss := int64(10 * fitMiss(m, list, w))
		for _, x := range [][2]int64{{r.BaseExp, t.BaseExp}, {r.Zeny, t.Zeny}} {
			if economy.Within(x[0], x[1], int(tol)) {
				continue
			}
			d := economy.Ratio(x[0], x[1]) - 100
			if d < 0 {
				d = -d
			}
			miss += max(d-tol, 1)
		}
		return int(min(miss, 1<<30))
	}
}

// ExtractSkills gives the monsters without skills those of the mode's
// mob_skill_db extract, which core's data/monsters.yml converts, for
// tests and tools that load the monsters without the mods (decision
// 0046). Rows the converter cannot hold are left out.
func ExtractSkills(mode reference.Mode, monsters map[uint32]Monster) error {
	defs, err := mob.Load(mode)
	if err != nil {
		return err
	}
	for _, d := range defs.Monsters {
		m, ok := monsters[d.ID]
		if !ok || len(m.Skills) > 0 {
			continue
		}
		for _, r := range d.Skills {
			s, _, err := mob.FromRaw(r, d.Mode, func(int) bool { return false }, func(id uint32) bool { _, ok := monsters[id]; return ok },
				func(int) mob.Chat { return mob.Chat{} })
			if err == nil {
				m.Skills = append(m.Skills, s)
			}
		}
		monsters[d.ID] = m
	}
	return nil
}

// FitLevels is how far the median level of a field's monsters, by count
// and without bosses, may be from its band (the spawn constraints of
// strategy section 8.11, decision 0046).
const FitLevels = 10

// fitMiss is how many levels the median level of a list's monsters is
// beyond FitLevels of the map's band, 0 when it fits.
func fitMiss(m spawns.Map, list []spawns.Spawn, w *economy.World) int {
	var levels []int
	for _, s := range list {
		mon, ok := w.Monster(s.Monster)
		if s.Role.Boss() || !ok || mon.Reward == nil {
			continue
		}
		for range s.Count {
			levels = append(levels, mon.Reward.Level)
		}
	}
	if len(levels) == 0 {
		return 0
	}
	slices.Sort(levels)
	med := levels[(len(levels)-1)/2]
	return max(m.Band[0]-FitLevels-med, med-m.Band[1]-FitLevels, 0)
}
