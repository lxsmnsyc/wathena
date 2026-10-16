package modscript

import (
	"errors"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Spawns (proposal 0002, review decision 44): the spawn_table hook,
// which runs when `wactl world new` makes a procedural world, and in the
// test worlds of wactl mod test. Each hook gets the map, its spawn list
// and next: hooks.on("spawn_table", function(map, spawns, next) ... end).
// It needs spawns.modify.

// SpawnMap is the map object of the spawn_table hook.
type SpawnMap struct {
	// ID is the map ID, Kind field or dungeon, and Biome a field's biome
	// or a floor's tileset.
	ID, Kind, Biome string
	// Band is the level band: the lowest and highest level.
	Band [2]int
	// Features are the map's features, such as water and clearing.
	Features []string
}

// SpawnArea is a spawn's area: within W and H cells of X and Y.
type SpawnArea struct{ X, Y, W, H int }

// SpawnEntry is one spawn of the spawn_table hook. Monster is a monster
// ID, Respawn milliseconds, and Area nil for the whole map.
type SpawnEntry struct {
	Monster int64
	Count   int
	Respawn int
	Role    string
	Area    *SpawnArea
}

// MaxSpawnEntries is the most spawns a hook may return.
const MaxSpawnEntries = 64

// RunSpawnTable runs the spawn_table hooks on a map's spawn list and
// returns the list they give. known reports whether a monster ID may
// spawn. ok is false without hooks, or when the chain failed: the list
// stays as it was.
func (r *Runtime) RunSpawnTable(m SpawnMap, list []SpawnEntry, known func(int64) bool) ([]SpawnEntry, bool) {
	features := make([]script.Value, len(m.Features))
	for i, f := range m.Features {
		features[i] = script.String(f)
	}
	spawns := make([]script.Value, len(list))
	for i, e := range list {
		fields := []script.Field{
			script.KV("monster", script.Int(e.Monster)), script.KV("count", script.Int(int64(e.Count))),
			script.KV("respawn", script.Int(int64(e.Respawn))), script.KV("role", script.String(e.Role)),
		}
		if e.Area != nil {
			fields = append(fields, script.KV("area", script.NewTable(
				script.KV("x", script.Int(int64(e.Area.X))), script.KV("y", script.Int(int64(e.Area.Y))),
				script.KV("w", script.Int(int64(e.Area.W))), script.KV("h", script.Int(int64(e.Area.H))))))
		}
		spawns[i] = script.NewTable(fields...)
	}
	fields := []script.Field{
		script.KV("map", script.NewTable(
			script.KV("id", script.String(m.ID)), script.KV("kind", script.String(m.Kind)),
			script.KV("biome", script.String(m.Biome)),
			script.KV("band", script.List(script.Int(int64(m.Band[0])), script.Int(int64(m.Band[1])))),
			script.KV("features", script.List(features...)))),
		script.KV("spawns", script.List(spawns...)),
	}
	prev := r.spawnKnown
	r.spawnKnown = known
	vals, ok := r.runHooks("spawn_table", fields)
	r.spawnKnown = prev
	if !ok {
		return list, false
	}
	t, isTable := vals[0].AsTable()
	if !isTable {
		return list, false
	}
	out, err := readSpawnList(t)
	if err != nil {
		return list, false
	}
	return out, true
}

// readSpawnList reads the list the chain returned, which the prelude
// checked.
func readSpawnList(t *script.Table) ([]SpawnEntry, error) {
	var out []SpawnEntry
	for i := int64(1); ; i++ {
		v := t.Index(i)
		if v.IsNil() {
			break
		}
		e, ok := v.AsTable()
		if !ok {
			return nil, errors.New("a spawn is not a table")
		}
		num := func(k string) int {
			n, _ := e.Get(k).AsInt()
			return int(n)
		}
		mon, _ := e.Get("monster").AsInt()
		role, _ := e.Get("role").AsString()
		se := SpawnEntry{Monster: mon, Count: num("count"), Respawn: num("respawn"), Role: role}
		if a, ok := e.Get("area").AsTable(); ok {
			get := func(k string) int {
				n, _ := a.Get(k).AsInt()
				return int(n)
			}
			se.Area = &SpawnArea{X: get("x"), Y: get("y"), W: get("w"), H: get("h")}
		}
		out = append(out, se)
	}
	return out, nil
}

// spawnKnownCall is the prelude's check of a spawn's monster ID.
func (r *Runtime) spawnKnownCall(c *script.Call) error {
	id := c.Int(0)
	c.Return(script.Bool(r.spawnKnown != nil && r.spawnKnown(id)))
	return nil
}
