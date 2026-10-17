package content

import (
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Generation layers (review decision 46): each mod's
// generation.on("map") functions run on every generated map at world
// creation, on the server only, and return places, objects and bosses.
// The layers cannot change terrain cells, which the client generates.

// Limits of one layer's result on one map.
const (
	MaxLayerPlaces  = 32
	MaxLayerObjects = 256
	MaxLayerBosses  = 8
	// MaxPlaceTags is the most tags of a place.
	MaxPlaceTags = 8
	// MaxTag is the longest tag.
	MaxTag = 64
	// SnapReach is how far a place or a boss moves to the nearest open
	// cell when its own cell is not open ground.
	SnapReach = 8
	// BossReach is the area of a boss with a cell: within BossReach cells
	// of it.
	BossReach = 3
	// MaxTurn is the highest turn of an object, in quarter turns.
	MaxTurn = 3
)

// Default respawn times of the layers' bosses, in milliseconds, inside
// the ranges of strategy section 8.11.
const (
	DefaultMiniBossRespawn  = 45 * 60 * 1000
	DefaultFieldBossRespawn = 90 * 60 * 1000
)

// LayerMap is what a layer reads of a map.
type LayerMap struct {
	// ID is the map ID, and Kind field, town or dungeon.
	ID, Kind string
	// Biome is a field's biome, or a town's or a floor's tileset.
	// Tileset is the tileset its props come from.
	Biome, Tileset string
	// Band is the level band: the lowest and the highest level.
	Band          [2]int
	Width, Height int
	// Places are the map's generated places.
	Places []gen.Place
}

// LayerPlace is a place a layer returns.
type LayerPlace struct {
	Tags []string
	X, Y int
}

// LayerObject is an object a layer returns: a prop of the map's tileset.
type LayerObject struct {
	Model      string
	X, Y, Turn int
}

// LayerBoss is a boss a layer returns: a monster by name, its role
// (mini_boss or field_boss), and a cell when HasCell, else anywhere.
// RespawnMs 0 is the role's default.
type LayerBoss struct {
	Monster   string
	Role      string
	X, Y      int
	HasCell   bool
	RespawnMs int
}

// LayerResult is what one mod's layers returned on a map.
type LayerResult struct {
	Mod     string
	Places  []LayerPlace
	Objects []LayerObject
	Bosses  []LayerBoss
}

// Layers runs the mods' generation layers.
type Layers interface {
	// Map runs every layer on a map, in the mods' load order, and
	// returns each mod's result. notes are layers that failed.
	Map(m LayerMap) (results []LayerResult, notes []string)
	// Templates are the quest templates that the layers added with
	// generation.quest_template, in the order they came.
	Templates() []Template
}

// LayerNode returns the node a mod's layer gets on a map: a child of the
// secret branch, so layers' draws are hidden from players.
func LayerNode(secret rng.Node, modID, mapID string) rng.Node {
	return secret.Child("generation").Child(modID).Child(mapID)
}

// ValidTag reports whether a place tag is 1 to MaxTag lowercase letters,
// digits, _, -, . and :, as wa.places takes them.
func ValidTag(s string) bool {
	if s == "" || len(s) > MaxTag {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != ':' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// layerMap is what layers read of a generated map.
func (g *generator) layerMap(id string) (LayerMap, bool) {
	m := g.in.Layout.Map(id)
	f := g.in.Maps[id]
	if m == nil || f == nil {
		return LayerMap{}, false
	}
	lm := LayerMap{ID: id, Kind: m.Kind, Band: m.Levels, Width: f.Width, Height: f.Height, Places: slices.Clone(f.Places)}
	switch m.Kind {
	case gen.KindField:
		if f.Biome >= 0 && f.Biome < len(g.in.Generator.Biomes) {
			lm.Biome = g.in.Generator.Biomes[f.Biome]
		}
		lm.Tileset = lm.Biome
	case gen.KindTown:
		p, err := gen.DecodeTown(m.Params)
		if err != nil {
			return LayerMap{}, false
		}
		lm.Biome, lm.Tileset = p.Template.Tileset, p.Template.Tileset
	case gen.KindDungeon:
		p, err := gen.DecodeDungeon(m.Params)
		if err != nil {
			return LayerMap{}, false
		}
		lm.Biome, lm.Tileset = p.Style.Tileset, p.Style.Tileset
	default:
		return LayerMap{}, false
	}
	return lm, true
}

// layers runs the layers on every generated map in the layout's order,
// and keeps what the rules allow.
func (g *generator) layers() error {
	if g.in.Layers == nil {
		return nil
	}
	for _, m := range g.in.Layout.Maps {
		lm, ok := g.layerMap(m.ID)
		if !ok {
			continue
		}
		results, notes := g.in.Layers.Map(lm)
		g.w.Notes = append(g.w.Notes, notes...)
		for _, r := range results {
			g.apply(lm, r)
		}
	}
	g.layerTemplates = g.in.Layers.Templates()
	return nil
}

// note records a layer's entry that the rules left out.
func (g *generator) note(m LayerMap, r LayerResult, format string, args ...any) {
	g.w.Notes = append(g.w.Notes, fmt.Sprintf("%s on %s: ", r.Mod, m.ID)+fmt.Sprintf(format, args...))
}

// snap returns the open cell nearest a cell, within SnapReach, by rings
// as addPlace searches.
func snap(f *gen.Field, x, y int) (int, int, bool) {
	for d := 0; d <= SnapReach; d++ {
		for dy := -d; dy <= d; dy++ {
			for dx := -d; dx <= d; dx++ {
				if max(abs(dx), abs(dy)) == d && walkable(f, x+dx, y+dy) {
					return x + dx, y + dy, true
				}
			}
		}
	}
	return 0, 0, false
}

// apply keeps one mod's result on a map:
//   - Places with valid tags move to the nearest open cell, within
//     SnapReach.
//   - Objects must name a prop of the map's tileset, on the map.
//   - Bosses must be a spawning monster as mini_boss or field_boss, on a
//     field or a dungeon floor. They join its spawn list, with the clear
//     cells of the spawn rules, while the list has room.
//
// What breaks a rule is left out with a note.
func (g *generator) apply(m LayerMap, r LayerResult) {
	f := g.in.Maps[m.ID]
	inside := func(x, y int) bool { return x >= 0 && y >= 0 && x < m.Width && y < m.Height }
	for i, p := range r.Places {
		if i >= MaxLayerPlaces {
			g.note(m, r, "more than %d places; the rest are left out", MaxLayerPlaces)
			break
		}
		if len(p.Tags) == 0 || len(p.Tags) > MaxPlaceTags || slices.ContainsFunc(p.Tags, func(t string) bool { return !ValidTag(t) }) {
			g.note(m, r, "place %d: tags must be 1 to %d tags of 1 to %d lowercase letters, digits, _, -, . and :", i+1, MaxPlaceTags, MaxTag)
			continue
		}
		x, y, ok := snap(f, p.X, p.Y)
		if !inside(p.X, p.Y) || !ok {
			g.note(m, r, "place %d at %d,%d: no open ground within %d cells", i+1, p.X, p.Y, SnapReach)
			continue
		}
		g.w.Places = append(g.w.Places, Place{Map: m.ID, X: x, Y: y, Tags: slices.Clone(p.Tags), Source: r.Mod})
		g.occupied[m.ID] = append(g.occupied[m.ID], [2]int{x, y})
	}
	ts, _ := g.in.Generator.Tileset(m.Kind, m.Tileset)
	for i, o := range r.Objects {
		if i >= MaxLayerObjects {
			g.note(m, r, "more than %d objects; the rest are left out", MaxLayerObjects)
			break
		}
		switch {
		case !slices.Contains(ts.Models, o.Model):
			g.note(m, r, "object %d: %q is not a prop of the tileset %s", i+1, o.Model, m.Tileset)
		case !inside(o.X, o.Y):
			g.note(m, r, "object %d: %d,%d is off the map", i+1, o.X, o.Y)
		case o.Turn < 0 || o.Turn > MaxTurn:
			g.note(m, r, "object %d: turn is 0 to %d", i+1, MaxTurn)
		default:
			g.w.Objects = append(g.w.Objects, Object{Map: m.ID, Model: o.Model, X: o.X, Y: o.Y, Turn: o.Turn, Source: r.Mod})
		}
	}
	if len(r.Bosses) > 0 {
		g.bosses(m, r)
	}
}

// bosses joins a layer's bosses to a map's spawn list.
func (g *generator) bosses(m LayerMap, r LayerResult) {
	if m.Kind != gen.KindField && m.Kind != gen.KindDungeon {
		g.note(m, r, "bosses spawn on fields and dungeon floors only")
		return
	}
	pi := slices.IndexFunc(g.w.Spawns, func(p spawns.Planned) bool { return p.Map == m.ID })
	si := slices.IndexFunc(g.in.SpawnMaps, func(s spawns.Map) bool { return s.ID == m.ID })
	if pi < 0 || si < 0 {
		g.note(m, r, "the map has no spawn list")
		return
	}
	f := g.in.Maps[m.ID]
	for i, b := range r.Bosses {
		if i >= MaxLayerBosses {
			g.note(m, r, "more than %d bosses; the rest are left out", MaxLayerBosses)
			break
		}
		mon, ok := g.in.SpawnMonsters[b.Monster]
		role, okRole := spawns.ParseRole(b.Role)
		switch {
		case !ok:
			g.note(m, r, "boss %d: %s does not spawn", i+1, b.Monster)
			continue
		case !okRole || !role.Boss():
			g.note(m, r, "boss %d: the role must be mini_boss or field_boss", i+1)
			continue
		case len(g.w.Spawns[pi].Spawns) >= spawns.MaxSpawns:
			g.note(m, r, "boss %d: the spawn list holds %d lines already", i+1, spawns.MaxSpawns)
			continue
		case b.RespawnMs != 0 && (b.RespawnMs < spawns.MinRespawnMs || b.RespawnMs > spawns.MaxRespawnMs):
			g.note(m, r, "boss %d: respawn is %d to %d milliseconds", i+1, spawns.MinRespawnMs, spawns.MaxRespawnMs)
			continue
		}
		s := spawns.Spawn{Monster: b.Monster, Count: 1, RespawnMs: b.RespawnMs, Role: role, Avoid: spawns.Clearance(g.in.SpawnMaps[si], mon, role)}
		if s.RespawnMs == 0 {
			s.RespawnMs = DefaultMiniBossRespawn
			if role == spawns.FieldBoss {
				s.RespawnMs = DefaultFieldBossRespawn
			}
		}
		if b.HasCell {
			x, y, ok := snap(f, b.X, b.Y)
			if b.X < 0 || b.Y < 0 || b.X >= m.Width || b.Y >= m.Height || !ok {
				g.note(m, r, "boss %d at %d,%d: no open ground within %d cells", i+1, b.X, b.Y, SnapReach)
				continue
			}
			s.Area = spawns.Area{X: x, Y: y, W: BossReach, H: BossReach}
		}
		g.w.Spawns[pi].Spawns = append(g.w.Spawns[pi].Spawns, s)
	}
}
