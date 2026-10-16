package spawns

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Nodes of the secret branch (decision 0042): each map's spawns come
// from child(child(secret, "spawns"), <map ID>), and each region's field
// bosses from child(child(secret, "regions"), <town ID>). The secret
// branch is child(root, "secret") of the world's seed, which only
// `wactl world new` holds.

// SecretBranch returns the secret branch of a world's root node. Only a
// holder of the world's seed can derive it: the game server has the
// public branch alone (decision 0039).
func SecretBranch(root rng.Node) rng.Node { return root.Child("secret") }

// MapNode returns a map's secret spawn node.
func MapNode(secret rng.Node, mapID string) rng.Node { return secret.Child("spawns").Child(mapID) }

// RegionNode returns a region's secret node.
func RegionNode(secret rng.Node, town string) rng.Node { return secret.Child("regions").Child(town) }

// Boss levels (strategy section 8.11): a mini boss is at most
// MiniBossAbove levels above the band's highest level, and a field boss
// at or above the band's lowest level, best within FieldBossAbove levels
// above its highest.
const (
	MiniBossAbove  = 20
	FieldBossAbove = 30
)

// Boss draws a boss of a role for a map (strategy section 8.11): a mini
// boss from the band's lowest level to MiniBossAbove levels above its
// highest, and a field boss from its lowest level to FieldBossAbove
// above its highest, each by its weight in the map's biome plus 1.
// Without one in that window, it takes the nearest by level: the
// closest mini boss, or the field boss closest above the band's lowest
// level, else the highest. Ties go to the first by name.
func (p *Pool) Boss(s *rng.Rand, m Map, role Role) (Monster, bool) {
	lo, hi := m.Band[0], m.Band[1]+MiniBossAbove
	if role == FieldBoss {
		hi = m.Band[1] + FieldBossAbove
	}
	var cands []pick
	var all []Monster
	for _, mon := range p.list {
		if mon.Tags.Role != role {
			continue
		}
		all = append(all, mon)
		if mon.Level < lo || mon.Level > hi {
			continue
		}
		cands = append(cands, pick{m: mon, weight: uint32(mon.Tags.Weight(m.Biome) + 1)}) // #nosec G115 -- at most 1,001.
	}
	if len(cands) > 0 {
		ws := make([]uint32, len(cands))
		for i, c := range cands {
			ws[i] = c.weight
		}
		return cands[s.Weighted(ws)].m, true
	}
	if len(all) == 0 {
		return Monster{}, false
	}
	best := all[0]
	better := func(a, b Monster) bool {
		if role == FieldBoss {
			// Above the band's lowest level, the lowest; else the highest.
			switch aAbove, bAbove := a.Level >= lo, b.Level >= lo; {
			case aAbove != bAbove:
				return aAbove
			case aAbove:
				return a.Level < b.Level
			default:
				return a.Level > b.Level
			}
		}
		return distance(a.Level, lo, hi) < distance(b.Level, lo, hi)
	}
	for _, mon := range all[1:] {
		if better(mon, best) {
			best = mon
		}
	}
	return best, true
}

func distance(level, lo, hi int) int {
	switch {
	case level < lo:
		return lo - level
	case level > hi:
		return level - hi
	}
	return 0
}

// bossSpawn is one boss line.
func bossSpawn(s *rng.Rand, m Map, mon Monster, role Role) Spawn {
	sp := Spawn{Monster: mon.Name, Count: 1, RespawnMs: respawn(s, role), Role: role, Avoid: Clearance(m, mon, role)}
	if m.BossRoom != nil {
		sp.Area = Area{X: m.BossRoom.X, Y: m.BossRoom.Y, W: BossRoomReach, H: BossRoomReach}
	}
	return sp
}

// Planned is one map's spawn list.
type Planned struct {
	Map    string
	Spawns []Spawn
}

// Plan picks the spawns of every field and dungeon floor of a world,
// in the order of maps, with their bosses (strategy section 8.11, step
// 6, review decision 44):
//
//   - Each map's species come from Select with its node.
//   - A field next to a town gets a boss of st.TownFieldBossRole with a
//     chance of st.TownFieldBossChance percent, from its node's child
//     boss.
//   - Each region's other fields: 1 or 2 of them, from the region's node,
//     get a field boss, from each field's node's child field_boss.
//   - Each dungeon's last floor gets a field boss in its boss room.
//
// Bosses keep TownPortalClear cells from every warp to a town, and
// aggressive monsters ArrivalClear cells from every arrival.
func Plan(secret rng.Node, list []Map, p *Pool, st Resolved) []Planned {
	out := make([]Planned, len(list))
	for i, m := range list {
		node := MapNode(secret, m.ID)
		out[i] = Planned{Map: m.ID, Spawns: Select(node, m, p, st)}
		var boss *Spawn
		switch {
		case m.Kind == "field" && m.NextToTown:
			s := node.Child("boss").SecretStream()
			if s.IntN(100) < st.TownFieldBossChance {
				if mon, ok := p.Boss(s, m, st.TownFieldBossRole); ok {
					b := bossSpawn(s, m, mon, st.TownFieldBossRole)
					boss = &b
				}
			}
		case m.Kind != "field" && m.Floor > 0 && m.Floor == m.Floors && m.BossRoom != nil:
			s := node.Child("boss").SecretStream()
			if mon, ok := p.Boss(s, m, FieldBoss); ok {
				b := bossSpawn(s, m, mon, FieldBoss)
				boss = &b
			}
		}
		if boss != nil {
			out[i].Spawns = append(out[i].Spawns, *boss)
		}
	}
	// Field bosses by region.
	regions := map[string][]int{}
	for i, m := range list {
		if m.Kind == "field" && !m.NextToTown && m.Region != "" {
			regions[m.Region] = append(regions[m.Region], i)
		}
	}
	for _, r := range sortedRegions(regions) {
		fields := slices.Clone(regions[r])
		s := RegionNode(secret, r).Child("bosses").SecretStream()
		n := 1 + s.IntN(2)
		s.Shuffle(len(fields), func(a, b int) { fields[a], fields[b] = fields[b], fields[a] })
		for _, i := range fields[:min(n, len(fields))] {
			m := list[i]
			fs := MapNode(secret, m.ID).Child("field_boss").SecretStream()
			if mon, ok := p.Boss(fs, m, FieldBoss); ok {
				out[i].Spawns = append(out[i].Spawns, bossSpawn(fs, m, mon, FieldBoss))
			}
		}
	}
	for i := range out {
		SortSpawns(out[i].Spawns)
	}
	return out
}

// sortedRegions orders town IDs by number: t2 before t10.
func sortedRegions(m map[string][]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	num := func(id string) int {
		n, err := strconv.Atoi(strings.TrimPrefix(id, "t"))
		if err != nil {
			return -1
		}
		return n
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Or(cmp.Compare(num(a), num(b)), cmp.Compare(a, b)) })
	return out
}
