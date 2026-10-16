package gamedata

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Spawn data of procedural worlds (review decision 44, decision 0042):
// the spawn themes of data/spawn_themes.yml, core's and the mods', and
// what spawn selection reads of each monster.

// spawnThemes reads the spawn theme records, with the race and element
// names of the constants.
func spawnThemes(set *mod.Set, consts *constants.Data) ([]spawns.Theme, *mod.Overlay, error) {
	o, err := set.Overlay(mod.SpawnThemeKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var races, elements func(string) bool
	if consts != nil {
		mods := consts.Modules()
		has := func(module string) func(string) bool {
			return func(name string) bool {
				_, ok := mods[module].Values[name]
				return ok
			}
		}
		races, elements = has("races"), has("elements")
	}
	var out []spawns.Theme
	var errs []error
	for _, r := range o.Records {
		t, err := spawns.ParseTheme(r.Name, r.Fields, races, elements)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: spawn themes %s: %w", r.Where, r.Name, err))
			continue
		}
		out = append(out, t)
	}
	return out, o, errors.Join(errs...)
}

// SpawnMonsters returns what spawn selection reads of each monster with
// stats, by name: its level, race, element and size by their names in
// the constants, whether it is a boss or aggressive, and its spawn tags.
func SpawnMonsters(monsters map[uint32]Monster, consts *constants.Data) []spawns.Monster {
	var mods map[string]*constants.Module
	if consts != nil {
		mods = consts.Modules()
	}
	name := func(module string, v int) string {
		if m := mods[module]; m != nil {
			return m.Names[int64(v)]
		}
		return ""
	}
	out := make([]spawns.Monster, 0, len(monsters))
	for _, m := range monsters {
		d := m.Def
		if d == nil || m.Status == nil || m.AegisName == "" {
			continue
		}
		out = append(out, spawns.Monster{
			Name: m.AegisName, Level: d.Level, Race: name("races", d.Race), Element: name("elements", d.Element),
			Size: name("sizes", d.Size), Boss: bossMonster(d), Aggressive: m.Status.Mode&mob.ModeAggressive != 0,
			Tags: m.SpawnTags,
		})
	}
	slices.SortFunc(out, func(a, b spawns.Monster) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// SpawnIndex returns the monsters' IDs by name, their names by ID, and
// what spawns read of each by name, for the spawn_table hook.
func SpawnIndex(monsters map[uint32]Monster, list []spawns.Monster) zone.SpawnIndex {
	idx := zone.SpawnIndex{IDs: map[string]int64{}, Names: map[int64]string{}, Monsters: map[string]spawns.Monster{}}
	for id, m := range monsters {
		if m.AegisName != "" && m.Status != nil {
			idx.IDs[m.AegisName], idx.Names[int64(id)] = int64(id), m.AegisName
		}
	}
	for _, m := range list {
		idx.Monsters[m.Name] = m
	}
	return idx
}

// ZoneSpawns turns a map's stored spawn lines into the zone's spawns,
// with each monster's stats by name. It returns the names it could not
// find, such as monsters of a mod that no longer loads.
func ZoneSpawns(list []spawns.Spawn, monsters map[uint32]Monster) ([]zone.Spawn, []string) {
	byName := make(map[string]uint32, len(monsters))
	for id, m := range monsters {
		if m.AegisName != "" && m.Status != nil {
			byName[m.AegisName] = id
		}
	}
	var out []zone.Spawn
	var missing []string
	for _, s := range list {
		id, ok := byName[s.Monster]
		if !ok {
			missing = append(missing, s.Monster)
			continue
		}
		m := monsters[id]
		sp := m.Spawn(id, m.Name, s.Area.X, s.Area.Y)
		sp.Amount, sp.W, sp.H, sp.RespawnMs = s.Count, s.Area.W, s.Area.H, s.RespawnMs
		for _, a := range s.Avoid {
			sp.Avoid = append(sp.Avoid, zone.Clear{X: a.X, Y: a.Y, R: a.R})
		}
		out = append(out, sp)
	}
	return out, missing
}
