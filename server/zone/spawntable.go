package zone

import (
	"errors"
	"log/slog"

	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// The spawn_table hook (proposal 0002, review decision 44, decision
// 0042): `wactl world new` runs it on each spawn list of a new
// procedural world, in a Lua state of its own, and the test worlds of
// wactl mod test in theirs.

// SpawnIndex finds monsters for the spawn_table hook: IDs by name,
// names by ID, and what spawns read of each by name.
type SpawnIndex struct {
	IDs      map[string]int64
	Names    map[int64]string
	Monsters map[string]spawns.Monster
}

// HasSpawnHooks reports whether a mod may add spawn_table hooks: a mod
// with spawns.modify and an entry script.
func HasSpawnHooks(mods *Mods) bool {
	if mods == nil {
		return false
	}
	for _, sc := range mods.Scripts {
		if sc.Grants.Has("spawns.modify") && len(sc.Entries) > 0 {
			return true
		}
	}
	return false
}

// SpawnHooks is a Lua state that runs the mods' entry scripts once, on
// no map, for the spawn_table hooks of a new world.
type SpawnHooks struct {
	z *Zone
}

// NewSpawnHooks makes the state and runs the mods' entry scripts. seed
// fixes its random stream.
func NewSpawnHooks(mods *Mods, seed [32]byte) (*SpawnHooks, error) {
	if mods == nil || mods.Testing {
		return nil, errors.New("zone: the spawn hooks need Mods without Testing")
	}
	z := New("", nil, rng.FromKey(seed).Stream(), nil)
	z.Mods = mods
	if z.itemScripts() == nil {
		return nil, errors.New("zone: the mods' Lua state could not be made; see the log")
	}
	return &SpawnHooks{z: z}, nil
}

// Run runs the hooks on one map's list.
func (h *SpawnHooks) Run(m spawns.Map, list []spawns.Spawn, idx SpawnIndex, log *slog.Logger) []spawns.Spawn {
	return RunSpawnTable(h.z.scripts, m, list, idx, log)
}

// Close ends the state.
func (h *SpawnHooks) Close() { h.z.stopScripts() }

// RunSpawnTable runs a Lua state's spawn_table hooks on a map's list and
// returns the list they give, or list when no hook ran. The server keeps
// its rules over the hooks' list: each line's clear cells follow its
// monster and role (spawns.Clearance), and a boss as common or uncommon
// is left out with a warning.
func RunSpawnTable(r *modscript.Runtime, m spawns.Map, list []spawns.Spawn, idx SpawnIndex, log *slog.Logger) []spawns.Spawn {
	if r == nil || !r.HasHooks("spawn_table") {
		return list
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	in := make([]modscript.SpawnEntry, 0, len(list))
	for _, s := range list {
		id, ok := idx.IDs[s.Monster]
		if !ok {
			continue
		}
		e := modscript.SpawnEntry{Monster: id, Count: s.Count, Respawn: s.RespawnMs, Role: string(s.Role)}
		if !s.Area.Whole() {
			e.Area = &modscript.SpawnArea{X: s.Area.X, Y: s.Area.Y, W: s.Area.W, H: s.Area.H}
		}
		in = append(in, e)
	}
	known := func(id int64) bool {
		name, ok := idx.Names[id]
		if !ok {
			return false
		}
		_, ok = idx.Monsters[name]
		return ok
	}
	sm := modscript.SpawnMap{ID: m.ID, Kind: m.Kind, Biome: m.Biome, Band: m.Band, Features: m.Features}
	out, ok := r.RunSpawnTable(sm, in, known)
	if !ok {
		return list
	}
	got := make([]spawns.Spawn, 0, len(out))
	for _, e := range out {
		name := idx.Names[e.Monster]
		mon := idx.Monsters[name]
		role, _ := spawns.ParseRole(e.Role)
		if mon.Boss && (role == spawns.Common || role == spawns.Uncommon) {
			log.Warn("a spawn_table hook made a boss "+e.Role+"; the line is left out", "map", m.ID, "monster", name)
			continue
		}
		s := spawns.Spawn{Monster: name, Count: e.Count, RespawnMs: e.Respawn, Role: role, Avoid: spawns.Clearance(m, mon, role)}
		if e.Area != nil {
			s.Area = spawns.Area{X: e.Area.X, Y: e.Area.Y, W: e.Area.W, H: e.Area.H}
		}
		got = append(got, s)
	}
	return got
}
