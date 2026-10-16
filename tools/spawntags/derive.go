package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/spawns"
)

// extract is the part of spawns.json the tool reads.
type extract struct {
	Hash string       `json:"rathena_git_hash"`
	Maps []extractMap `json:"maps"`
}

// extractMap is one map of spawns.json, with its spawn lines.
type extractMap struct {
	Map      string        `json:"map"`
	Spawns   []extractLine `json:"spawns"`
	Walkable int           `json:"walkable_cells"`
	Total    int           `json:"total_monsters"`
}

// extractLine is one spawn line of rAthena.
type extractLine struct {
	Mob       uint32 `json:"mob"`
	Amount    int    `json:"amount"`
	RespawnMs int    `json:"respawn_ms"`
	Boss      bool   `json:"boss"`
}

// Data is what the tool writes.
type Data struct {
	Hash string
	// Density is the median monsters per 10,000 walkable cells of the
	// field maps and of the dungeon maps.
	FieldDensity, DungeonDensity int
	// Maps counts the maps of each biome.
	Maps     map[string]int
	Monsters []Tagged
}

// Tagged is one monster's tags.
type Tagged struct {
	Name   string
	Role   spawns.Role
	Biomes map[string]int
}

// Family returns the biome of a map's family, or "".
func Family(name string) string {
	best, biome := "", ""
	for b, list := range Families {
		for _, f := range list {
			if strings.HasPrefix(name, f) && len(name)-len(f) <= maxRest && len(f) > len(best) {
				best, biome = f, b
			}
		}
	}
	return biome
}

// LineRole is the role of one spawn line, by strategy section 8.11's
// rules: a boss line is a field boss, 5 or fewer with a respawn of 30
// minutes or more rare, an immobile monster of AI type 06 a plant, 10
// or more with a respawn of 60 seconds or less common, and the rest
// uncommon.
func LineRole(amount, respawnMs int, boss bool, ai string) spawns.Role {
	switch {
	case boss:
		return spawns.FieldBoss
	case amount <= 5 && respawnMs >= 30*60*1000:
		return spawns.Rare
	case ai == "06":
		return spawns.Plant
	case amount >= 10 && respawnMs <= 60*1000:
		return spawns.Common
	}
	return spawns.Uncommon
}

// Load derives the tags from the renewal extract and monster data.
func Load() (*Data, error) {
	var x extract
	if err := reference.LoadFile("extracts/renewal/spawns.json", &x); err != nil {
		return nil, err
	}
	defs, err := mob.Load(reference.Renewal)
	if err != nil {
		return nil, err
	}
	byID := map[uint32]*mob.Def{}
	for i := range defs.Monsters {
		byID[defs.Monsters[i].ID] = &defs.Monsters[i]
	}
	return Derive(x, byID)
}

// Derive computes the tags of every monster that spawns on a map of a
// family.
func Derive(x extract, byID map[uint32]*mob.Def) (*Data, error) {
	d := &Data{Hash: x.Hash, Maps: map[string]int{}}
	// amounts[biome][monster] and the biome's total.
	amounts := map[string]map[uint32]int{}
	totals := map[string]int{}
	// roles[monster][role] sums the amounts of its lines by role.
	roles := map[uint32]map[spawns.Role]int{}
	var fieldDensity, dungeonDensity []int
	for _, m := range x.Maps {
		biome := Family(m.Map)
		if biome == "" {
			continue
		}
		d.Maps[biome]++
		if m.Walkable > 0 {
			density := (m.Total*10000 + m.Walkable/2) / m.Walkable
			if slices.Contains(FieldBiomes, biome) {
				fieldDensity = append(fieldDensity, density)
			} else {
				dungeonDensity = append(dungeonDensity, density)
			}
		}
		if amounts[biome] == nil {
			amounts[biome] = map[uint32]int{}
		}
		for _, s := range m.Spawns {
			def := byID[s.Mob]
			if def == nil || s.Amount <= 0 {
				continue
			}
			amounts[biome][s.Mob] += s.Amount
			totals[biome] += s.Amount
			if roles[s.Mob] == nil {
				roles[s.Mob] = map[spawns.Role]int{}
			}
			roles[s.Mob][LineRole(s.Amount, s.RespawnMs, s.Boss, def.AI)] += s.Amount
		}
	}
	if len(fieldDensity) == 0 || len(dungeonDensity) == 0 {
		return nil, fmt.Errorf("spawns.json has no map of a family")
	}
	d.FieldDensity, d.DungeonDensity = median(fieldDensity), median(dungeonDensity)
	for _, id := range slices.Sorted(maps.Keys(roles)) {
		def := byID[id]
		t := Tagged{Name: def.AegisName, Biomes: map[string]int{}}
		best := -1
		for _, r := range spawns.Roles {
			if n := roles[id][r]; n > best {
				best, t.Role = n, r
			}
		}
		// A boss never spawns as common or uncommon (review decision 44):
		// an MVP is a field boss, and another boss a mini boss.
		if def.Class == 1 || def.Mode&mob.ModeMvp != 0 {
			t.Role = spawns.MiniBoss
			if def.Mode&mob.ModeMvp != 0 {
				t.Role = spawns.FieldBoss
			}
		}
		for b, list := range amounts {
			if n := list[id]; n > 0 {
				t.Biomes[b] = max(1, min(spawns.MaxWeight, (n*1000+totals[b]/2)/totals[b]))
			}
		}
		d.Monsters = append(d.Monsters, t)
	}
	slices.SortFunc(d.Monsters, func(a, b Tagged) int { return cmp.Compare(a.Name, b.Name) })
	return d, nil
}

func median(v []int) int {
	slices.Sort(v)
	return v[len(v)/2]
}

// YAML writes the data as core's data/spawn_tags.yml.
func (d *Data) YAML() []byte {
	var b strings.Builder
	b.WriteString(`# Core's spawn tags (strategy section 8.11, proposal 0002, review
# decision 44, decision 0042). tools/spawntags writes this file from
# rAthena's spawn files, as reference/rathena/extracts/renewal/
# spawns.json holds them. Do not edit it: run go run ./tools/spawntags.
#
#   - families: the rAthena map families of each biome, and how many
#     maps they have.
#   - density: the median monsters per 10,000 walkable cells of rAthena's
#     field maps and dungeon maps of those families.
#   - monsters: each monster that spawns on them, with its role and its
#     weight per 1,000 of each biome's monsters. Monster records change
#     them with spawn.
`)
	fmt.Fprintf(&b, "rathena_git_hash: %s\n", d.Hash)
	fmt.Fprintf(&b, "density:\n  field: %d\n  dungeon: %d\n", d.FieldDensity, d.DungeonDensity)
	b.WriteString("families:\n")
	for _, biome := range slices.Sorted(maps.Keys(Families)) {
		list := slices.Sorted(slices.Values(Families[biome]))
		fmt.Fprintf(&b, "  %s: {maps: %d, families: [%s]}\n", biome, d.Maps[biome], strings.Join(list, ", "))
	}
	b.WriteString("monsters:\n")
	for _, m := range d.Monsters {
		var parts []string
		for _, biome := range slices.Sorted(maps.Keys(m.Biomes)) {
			parts = append(parts, fmt.Sprintf("%s: %d", biome, m.Biomes[biome]))
		}
		fmt.Fprintf(&b, "  - {name: %s, role: %s, biomes: {%s}}\n", m.Name, m.Role, strings.Join(parts, ", "))
	}
	return []byte(b.String())
}
