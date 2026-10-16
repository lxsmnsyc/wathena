// Package spawns picks the monsters of a procedural world's fields and
// dungeon floors (strategy section 8.11, proposal 0002, review decision
// 44, decision 0042):
//
//   - Monsters have spawn tags: biome weights and a role from rAthena's
//     spawn files (core/data/spawn_tags.yml, which tools/spawntags
//     writes), and the spawn field of monster records, which core and
//     mods may set.
//   - Spawn themes are records of data/spawn_themes.yml.
//   - Select picks one map's spawns, and Plan every map of a world with
//     its bosses, from the secret branch of the world's seed tree. Only
//     `wactl world new` holds that branch. The database keeps the
//     result, and the game server spawns from it.
//
// Every step uses integer math, sorted slices and the secret streams of
// shared/rng, so a seed gives the same spawns on every platform.
package spawns

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Role is a monster's part in a map's spawns (strategy section 8.11).
type Role string

// Roles, in the order spawn lists sort by.
const (
	Common    Role = "common"
	Uncommon  Role = "uncommon"
	Plant     Role = "plant"
	Rare      Role = "rare"
	MiniBoss  Role = "mini_boss"
	FieldBoss Role = "field_boss"
)

// Roles lists every role in order.
var Roles = []Role{Common, Uncommon, Plant, Rare, MiniBoss, FieldBoss}

// ParseRole returns the role of a name.
func ParseRole(s string) (Role, bool) {
	r := Role(s)
	return r, slices.Contains(Roles, r)
}

// Boss reports whether the role is a boss's: mini_boss or field_boss.
func (r Role) Boss() bool { return r == MiniBoss || r == FieldBoss }

func (r Role) rank() int { return slices.Index(Roles, r) }

// Limits of tags, themes and spawns.
const (
	// MaxWeight is the highest biome weight.
	MaxWeight = 1000
	// MaxBiomes and MaxThemes are the most biomes and themes a monster's
	// tags name.
	MaxBiomes = 32
	MaxThemes = 16
	// MaxCount is the most monsters of one spawn.
	MaxCount = 1000
	// MaxSpawns is the most spawns of one map.
	MaxSpawns = 64
	// MinRespawnMs and MaxRespawnMs bound a spawn's respawn time, as
	// spawning.monsters does.
	MinRespawnMs = 1000
	MaxRespawnMs = 0xfffffff
)

// Tags are a monster's spawn tags (strategy section 8.11): the biomes it
// lives in with their weights, its role, and the themes that raise it.
type Tags struct {
	// Biomes are weights from 1 to MaxWeight, by biome name.
	Biomes map[string]int
	Role   Role
	Themes []string
}

// Weight returns the monster's weight in a biome, 0 when it does not
// live there.
func (t *Tags) Weight(biome string) int {
	if t == nil {
		return 0
	}
	return t.Biomes[biome]
}

// ValidName reports whether a biome or theme name is 1 to 64 lowercase
// letters, digits and _.
func ValidName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// ParseTags reads a monster record's spawn field: { biomes, role,
// themes }. base, when not nil, holds the tags the record changes:
// each field given replaces base's. A role left out stays empty, for
// the caller's default.
func ParseTags(v any, base *Tags) (*Tags, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("spawn must be a table: { biomes = { grassland: 10 }, role: common, themes: [cute] }")
	}
	out := &Tags{}
	if base != nil {
		out.Role, out.Themes = base.Role, slices.Clone(base.Themes)
		out.Biomes = map[string]int{}
		for k, w := range base.Biomes {
			out.Biomes[k] = w
		}
	}
	var errs []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		switch k {
		case "biomes":
			bm, ok := m[k].(map[string]any)
			if !ok {
				errs = append(errs, "spawn.biomes must map biome names to weights, such as { grassland: 10 }")
				continue
			}
			if len(bm) > MaxBiomes {
				errs = append(errs, fmt.Sprintf("spawn.biomes has more than %d biomes", MaxBiomes))
			}
			out.Biomes = map[string]int{}
			for _, b := range slices.Sorted(maps.Keys(bm)) {
				w, ok := bm[b].(int)
				switch {
				case !ValidName(b):
					errs = append(errs, fmt.Sprintf("spawn.biomes: %q is not a biome name: 1 to 64 lowercase letters, digits and _", b))
				case !ok || w < 1 || w > MaxWeight:
					errs = append(errs, fmt.Sprintf("spawn.biomes: %s's weight must be a whole number from 1 to %d", b, MaxWeight))
				default:
					out.Biomes[b] = w
				}
			}
		case "role":
			s, _ := m[k].(string)
			r, ok := ParseRole(s)
			if !ok {
				errs = append(errs, fmt.Sprintf("spawn.role: %q is not a role; the roles are %s", s, roleList()))
				continue
			}
			out.Role = r
		case "themes":
			list, ok := m[k].([]any)
			if !ok {
				errs = append(errs, "spawn.themes must be a list of theme names, such as [cute]")
				continue
			}
			if len(list) > MaxThemes {
				errs = append(errs, fmt.Sprintf("spawn.themes has more than %d themes", MaxThemes))
			}
			out.Themes = nil
			for _, e := range list {
				s, _ := e.(string)
				if !ValidName(s) {
					errs = append(errs, fmt.Sprintf("spawn.themes: %q is not a theme name", s))
					continue
				}
				if !slices.Contains(out.Themes, s) {
					out.Themes = append(out.Themes, s)
				}
			}
		default:
			errs = append(errs, fmt.Sprintf("spawn: %q is not a field; spawn has biomes, role and themes", k))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return out, nil
}

func roleList() string {
	names := make([]string, len(Roles))
	for i, r := range Roles {
		names[i] = string(r)
	}
	return strings.Join(names, ", ")
}

// Monster is what spawns read of a monster: its data and its tags.
type Monster struct {
	// Name is the AegisName, such as PORING, or a mod monster's full
	// name. The database keeps spawns by it.
	Name  string
	Level int
	// Race, Element and Size are the names of wa.races, wa.elements and
	// wa.sizes, such as brute, water and small.
	Race, Element, Size string
	// Boss is set for mob_db's boss class and for MVPs. Such a monster
	// is never common or uncommon.
	Boss bool
	// Aggressive is the aggressive mode: the monster attacks on sight.
	Aggressive bool
	// Tags are the spawn tags, or nil for a monster that never spawns
	// on generated maps.
	Tags *Tags
}

// CheckTags returns why a monster's tags break the rules, or nil: boss
// monsters are never common or uncommon (review decision 44).
func (m Monster) CheckTags() error {
	if m.Tags != nil && m.Boss && (m.Tags.Role == Common || m.Tags.Role == Uncommon) {
		return fmt.Errorf("spawn.role: %s is a boss monster, and a boss is never %s; use rare, mini_boss or field_boss", m.Name, m.Tags.Role)
	}
	return nil
}
