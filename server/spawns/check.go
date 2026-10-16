package spawns

import (
	"fmt"
	"slices"
)

// Check returns the ways a map's spawn list breaks the rules of strategy
// section 8.11 and review decision 44, or nil. monster finds a
// monster's data by name. Select and Plan always pass it; a list that a
// spawn_table hook changed may not.
//
//   - 5 to 8 species besides bosses, at least 2 of them common, at most 1
//     rare, at most 3 of one element, and at most 2 aggressive ones in a
//     band that starts below level 30.
//   - No boss monster as common or uncommon.
//   - Counts of 1 to 1,000 and respawns of at least 1 second.
//   - An aggressive monster keeps 15 cells from every arrival, and a
//     boss 30 cells from every warp to a town.
//   - A mini boss only on a field next to a town, at most 20 levels
//     above the band, and a field boss elsewhere.
func Check(m Map, list []Spawn, monster func(string) (Monster, bool)) []string {
	var out []string
	bad := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	var species []Monster
	common, rare, aggressive := 0, 0, 0
	elements := map[string]int{}
	for _, s := range list {
		mon, ok := monster(s.Monster)
		if !ok {
			bad("%s is not a monster", s.Monster)
			continue
		}
		if s.Count < 1 || s.Count > MaxCount {
			bad("%s has a count of %d", s.Monster, s.Count)
		}
		if s.RespawnMs < MinRespawnMs || s.RespawnMs > MaxRespawnMs {
			bad("%s respawns after %d ms", s.Monster, s.RespawnMs)
		}
		if mon.Boss && (s.Role == Common || s.Role == Uncommon) {
			bad("%s is a boss as %s", s.Monster, s.Role)
		}
		if mon.Aggressive {
			for _, a := range m.Arrivals {
				if !covers(s.Avoid, a, ArrivalClear) {
					bad("%s is aggressive and may spawn within %d cells of the arrival %d,%d", s.Monster, ArrivalClear, a.X, a.Y)
				}
			}
		}
		if s.Role.Boss() {
			for _, t := range m.TownPortals {
				if !covers(s.Avoid, t, TownPortalClear) {
					bad("the boss %s may spawn within %d cells of the warp to a town at %d,%d", s.Monster, TownPortalClear, t.X, t.Y)
				}
			}
			switch {
			case s.Role == MiniBoss && (m.Kind != "field" || !m.NextToTown):
				bad("the mini boss %s is on a map not next to a town", s.Monster)
			case s.Role == MiniBoss && mon.Level > m.Band[1]+MiniBossAbove:
				bad("the mini boss %s is level %d, more than %d above the band %v", s.Monster, mon.Level, MiniBossAbove, m.Band)
			}
			continue
		}
		if slices.ContainsFunc(species, func(o Monster) bool { return o.Name == mon.Name }) {
			continue
		}
		species = append(species, mon)
		switch s.Role {
		case Common:
			common++
		case Rare:
			rare++
		}
		if mon.Aggressive {
			aggressive++
		}
		elements[mon.Element]++
	}
	if n := len(species); n < MinSpecies || n > MaxSpecies {
		bad("%d species, not %d to %d", n, MinSpecies, MaxSpecies)
	}
	if common < MinCommon {
		bad("%d common species, fewer than %d", common, MinCommon)
	}
	if rare > MaxRare {
		bad("%d rare species, more than %d", rare, MaxRare)
	}
	for e, n := range elements {
		if n > MaxElement {
			bad("%d species of the element %s, more than %d", n, e, MaxElement)
		}
	}
	if m.Band[0] < AggressiveLevel && aggressive > MaxAggressiveLow {
		bad("%d aggressive species in the band %v, more than %d", aggressive, m.Band, MaxAggressiveLow)
	}
	slices.Sort(out)
	return out
}

// covers reports whether an avoid list keeps r cells around a point.
func covers(avoid []Avoid, p Point, r int) bool {
	return slices.ContainsFunc(avoid, func(a Avoid) bool { return a.X == p.X && a.Y == p.Y && a.R >= r })
}
