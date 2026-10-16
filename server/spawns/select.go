package spawns

import (
	"cmp"
	"slices"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Point is a cell.
type Point struct{ X, Y int }

// Map is what selection reads of one field or dungeon floor
// (strategy section 8.11): its band, biome, size and features, and the
// cells its constraints keep clear.
type Map struct {
	// ID is the map ID, such as gen:field:3:1. Kind is field or
	// dungeon.
	ID, Kind string
	// Biome is a field's biome, or a floor's tileset, such as crypt.
	Biome string
	// Band is the level band: the lowest and highest level.
	Band [2]int
	// Walkable counts the cells monsters may stand on.
	Walkable int
	// Features are the map's features, such as water, clearing, room
	// and boss_room, sorted.
	Features []string
	// FeaturePoints are cells near features, where plants grow: water
	// and clearings on fields, rooms on floors.
	FeaturePoints []Point
	// Arrivals are the cells where players arrive through a warp.
	Arrivals []Point
	// TownPortals are the warps that lead to a town.
	TownPortals []Point
	// BossRoom is a last floor's boss room, or nil.
	BossRoom *Point
	// NextToTown is set for a field one warp from a town.
	NextToTown bool
	// Region is the town whose region holds a field: the fewest warps
	// away, ties to the lower number, such as t2.
	Region string
	// Floor and Floors are a dungeon floor's.
	Floor, Floors int
}

// Area is where a spawn's monsters appear: within W and H cells of
// (X, Y), as rAthena's spawn lines, or anywhere when it is zero.
type Area struct{ X, Y, W, H int }

// Whole reports whether the area is the whole map.
func (a Area) Whole() bool { return a == Area{} }

// Avoid keeps a spawn's monsters off the cells within R cells of
// (X, Y), by the larger of the two axis distances.
type Avoid struct{ X, Y, R int }

// Spawn is one line of a map's spawn list.
type Spawn struct {
	// Monster is the monster's name: an AegisName or a mod's full name.
	Monster   string
	Count     int
	RespawnMs int
	Role      Role
	Area      Area
	Avoid     []Avoid
}

// Distances that keep monsters clear (review decision 44).
const (
	// ArrivalClear is how close to an arrival no aggressive monster
	// spawns.
	ArrivalClear = 15
	// TownPortalClear is how close to a warp to a town no boss spawns.
	TownPortalClear = 30
	// PlantReach is how far from its feature a plant spawns.
	PlantReach = 8
	// BossRoomReach is how far from the boss room's place a floor's
	// boss spawns.
	BossRoomReach = 3
)

// Species limits of strategy section 8.11.
const (
	MinSpecies = 5
	MaxSpecies = 8
	// MinCommon is the fewest common species, MaxRare the most rare
	// ones, and MaxElement the most species of one element.
	MinCommon  = 2
	MaxRare    = 1
	MaxElement = 3
	// MaxAggressiveLow is the most aggressive species in a band that
	// starts below AggressiveLevel.
	MaxAggressiveLow = 2
	AggressiveLevel  = 30
)

// tier is one widening of the candidates: the levels beyond the band on
// each side, and whether monsters of other biomes count.
type tier struct {
	margin int
	other  bool
}

// tiers widen the candidates until the draw meets the limits: first the
// map's biome within 10 levels of its band, then any biome, then
// further levels, the biome first at each step.
var tiers = []tier{
	{0, false}, {5, false}, {10, false}, {0, true}, {5, true}, {10, true},
	{15, false}, {20, false}, {15, true}, {20, true}, {30, false}, {30, true}, {50, false}, {50, true},
}

// Pool is the monsters selection draws from: those with tags, by name.
type Pool struct {
	list   []Monster
	themes []Theme
}

// NewPool makes a pool of the monsters with tags, and the themes.
// Monsters without tags, or whose tags break the rules, are left out.
func NewPool(monsters []Monster, themes []Theme) *Pool {
	p := &Pool{themes: slices.Clone(themes)}
	for _, m := range monsters {
		if m.Tags == nil || m.Tags.Role == "" || m.CheckTags() != nil || m.Level < 1 {
			continue
		}
		p.list = append(p.list, m)
	}
	slices.SortFunc(p.list, func(a, b Monster) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortStableFunc(p.themes, func(a, b Theme) int { return cmp.Compare(a.Name, b.Name) })
	return p
}

// Monster returns a monster of the pool by name.
func (p *Pool) Monster(name string) (Monster, bool) {
	i, ok := slices.BinarySearchFunc(p.list, name, func(m Monster, n string) int { return cmp.Compare(m.Name, n) })
	if !ok {
		return Monster{}, false
	}
	return p.list[i], true
}

// Monsters returns the pool's monsters by name.
func (p *Pool) Monsters() []Monster { return p.list }

// closeness is how well a level fits a window, in percent: 100 inside,
// then 4 less per level outside, at least 10.
func closeness(level, lo, hi int) int {
	d := 0
	switch {
	case level < lo:
		d = lo - level
	case level > hi:
		d = level - hi
	}
	return max(100-4*d, 10)
}

// pick is a candidate species with its weight.
type pick struct {
	m      Monster
	weight uint32
}

// Theme returns the theme a map draws, or nil: with a chance of
// ThemeChance in 3, one of the themes that apply to its biome, by name.
func (p *Pool) Theme(node rng.Node, m Map) *Theme {
	s := node.Child("theme").SecretStream()
	if s.IntN(3) >= ThemeChance {
		return nil
	}
	var fit []int
	for i, t := range p.themes {
		if t.Applies(m.Biome) {
			fit = append(fit, i)
		}
	}
	if len(fit) == 0 {
		return nil
	}
	t := p.themes[fit[s.IntN(len(fit))]]
	return &t
}

// candidates returns the species a map may draw within a margin: the
// common, uncommon, plant and rare monsters of its biome whose level is
// within margin of its band. With other set, monsters of any biome
// count, at a weight of 1.
func (p *Pool) candidates(m Map, margin int, other bool, theme *Theme) []pick {
	var out []pick
	for _, mon := range p.list {
		if mon.Tags.Role.Boss() {
			continue
		}
		if mon.Level < m.Band[0]-margin || mon.Level > m.Band[1]+margin {
			continue
		}
		w := mon.Tags.Weight(m.Biome)
		if w == 0 {
			if !other {
				continue
			}
			w = 1
		}
		weight := w * closeness(mon.Level, m.Band[0], m.Band[1])
		if theme != nil && theme.Matches(mon) {
			weight = weight * theme.Boost / 100
		}
		out = append(out, pick{m: mon, weight: uint32(max(weight, 1))}) // #nosec G115 -- at most 1,000 × 100 × 10.
	}
	return out
}

// species draws a map's species from candidates (strategy section 8.11,
// step 3): first MinCommon common ones, then the rest from every
// candidate, each by weight, leaving out a draw that breaks a limit.
// It returns nil when the candidates cannot meet the limits.
func species(s *rng.Rand, cands []pick, want int, band [2]int) []Monster {
	var out []Monster
	fits := func(m Monster) bool {
		elements, aggressive, rare := 0, 0, 0
		for _, o := range out {
			if o.Element == m.Element {
				elements++
			}
			if o.Aggressive {
				aggressive++
			}
			if o.Tags.Role == Rare {
				rare++
			}
		}
		switch {
		case elements >= MaxElement:
			return false
		case m.Aggressive && band[0] < AggressiveLevel && aggressive >= MaxAggressiveLow:
			return false
		case m.Tags.Role == Rare && rare >= MaxRare:
			return false
		}
		return true
	}
	draw := func(pool []pick, n int) []pick {
		for len(out) < n && len(pool) > 0 {
			ws := make([]uint32, len(pool))
			for i, c := range pool {
				ws[i] = c.weight
			}
			i := s.Weighted(ws)
			c := pool[i]
			pool = slices.Delete(pool, i, i+1)
			if fits(c.m) {
				out = append(out, c.m)
			}
		}
		return pool
	}
	var commons []pick
	for _, c := range cands {
		if c.m.Tags.Role == Common {
			commons = append(commons, c)
		}
	}
	draw(commons, MinCommon)
	if len(out) < MinCommon {
		return nil
	}
	var rest []pick
	for _, c := range cands {
		if !slices.ContainsFunc(out, func(o Monster) bool { return o.Name == c.m.Name }) {
			rest = append(rest, c)
		}
	}
	draw(rest, want)
	if len(out) < MinSpecies {
		return nil
	}
	return out
}

// Settings are the spawn settings of the config, `spawns` in
// waserver.yml (review decision 44).
type Settings struct {
	// TownFieldBossChance is the chance in percent that a field next to
	// a town gets a boss, 50 by default.
	TownFieldBossChance *int `yaml:"town_field_boss_chance"`
	// TownFieldBossRole is that boss's role: mini_boss by default, or
	// field_boss for stronger bosses.
	TownFieldBossRole string `yaml:"town_field_boss_role"`
	// Density is the monsters per 10,000 walkable cells of fields and
	// of dungeon floors. 0 takes rAthena's, from core's spawn tags.
	Density struct {
		Field   int `yaml:"field"`
		Dungeon int `yaml:"dungeon"`
	} `yaml:"density"`
}

// Defaults of Settings.
const (
	DefaultTownFieldBossChance = 50
	MaxDensity                 = 1000
)

// Resolved are settings with their defaults filled in.
type Resolved struct {
	TownFieldBossChance   int
	TownFieldBossRole     Role
	FieldDensity, Density int
}

// Resolve checks the settings and fills in the defaults. densities are
// rAthena's, from core's spawn tags.
func (s Settings) Resolve(densities Densities) (Resolved, error) {
	r := Resolved{TownFieldBossChance: DefaultTownFieldBossChance, TownFieldBossRole: MiniBoss,
		FieldDensity: densities.Field, Density: densities.Dungeon}
	if s.TownFieldBossChance != nil {
		if *s.TownFieldBossChance < 0 || *s.TownFieldBossChance > 100 {
			return Resolved{}, errSetting("spawns.town_field_boss_chance is 0 to 100, in percent")
		}
		r.TownFieldBossChance = *s.TownFieldBossChance
	}
	switch s.TownFieldBossRole {
	case "", string(MiniBoss):
	case string(FieldBoss):
		r.TownFieldBossRole = FieldBoss
	default:
		return Resolved{}, errSetting("spawns.town_field_boss_role is mini_boss or field_boss")
	}
	for _, d := range []struct {
		v  int
		to *int
		k  string
	}{{s.Density.Field, &r.FieldDensity, "field"}, {s.Density.Dungeon, &r.Density, "dungeon"}} {
		switch {
		case d.v < 0 || d.v > MaxDensity:
			return Resolved{}, errSetting("spawns.density." + d.k + " is 1 to 1000 monsters per 10,000 walkable cells, or 0 for rAthena's")
		case d.v > 0:
			*d.to = d.v
		}
	}
	return r, nil
}

type errSetting string

func (e errSetting) Error() string { return string(e) }

// Densities are rAthena's monsters per 10,000 walkable cells.
type Densities struct{ Field, Dungeon int }

// Select picks one map's spawns without bosses, from the map's secret
// node (strategy section 8.11, steps 1 to 5):
//
//  1. Candidates: the common, uncommon, plant and rare monsters whose
//     biome is the map's and whose level is within the band, widened by
//     tiers until the draw below meets the limits.
//  2. A theme, with a chance of 1 in 3, raises its monsters' weights.
//  3. 5 to 8 species by weight, with the limits of section 8.11.
//  4. Counts from the density and the walkable cells, shared by role.
//  5. Areas: plants near features, the rest anywhere, and aggressive
//     monsters away from arrivals.
//
// It returns nil when no monster fits, such as with an empty pool.
func Select(node rng.Node, m Map, p *Pool, st Resolved) []Spawn {
	theme := p.Theme(node, m)
	want := MinSpecies + node.Child("count").SecretStream().IntN(MaxSpecies-MinSpecies+1)
	var chosen []Monster
	for i, t := range tiers {
		s := node.Child("species", int64(i)).SecretStream()
		if chosen = species(s, p.candidates(m, t.margin, t.other, theme), want, m.Band); chosen != nil {
			break
		}
	}
	if chosen == nil {
		return nil
	}
	density := st.FieldDensity
	if m.Kind != "field" {
		density = st.Density
	}
	return counts(node, m, chosen, density)
}

// counts gives each species its count, respawn and area (steps 4 and
// 5).
func counts(node rng.Node, m Map, chosen []Monster, density int) []Spawn {
	s := node.Child("counts").SecretStream()
	// At least 2 monsters per species, so commons outnumber the rest.
	total := max(m.Walkable*density/10000, 2*len(chosen))
	byRole := map[Role][]Monster{}
	for _, c := range chosen {
		byRole[c.Tags.Role] = append(byRole[c.Tags.Role], c)
	}
	var out []Spawn
	used := 0
	add := func(mon Monster, n int) {
		out = append(out, Spawn{Monster: mon.Name, Count: n, Role: mon.Tags.Role})
		used += n
	}
	// Rares and plants take at most a quarter of the total, each at
	// least 1.
	var few []Monster
	var wants []int
	sum := 0
	for _, mon := range byRole[Rare] {
		few, wants = append(few, mon), append(wants, int(s.Range(1, 5)))
		sum += wants[len(wants)-1]
	}
	for _, mon := range byRole[Plant] {
		few, wants = append(few, mon), append(wants, int(s.Range(3, 8)))
		sum += wants[len(wants)-1]
	}
	budget := max(total/4, len(few))
	for i, mon := range few {
		n := wants[i]
		if sum > budget {
			n = max(n*budget/sum, 1)
		}
		add(mon, n)
	}
	rest := max(total-used, len(byRole[Common])+len(byRole[Uncommon]))
	common := rest
	if len(byRole[Uncommon]) > 0 {
		c, u := int(s.Range(60, 70)), int(s.Range(25, 35))
		common = max(rest*c/(c+u), len(byRole[Common]))
		common = min(common, rest-len(byRole[Uncommon]))
	}
	split := func(list []Monster, n int) {
		parts := make([]int, len(list))
		sum := 0
		for i := range list {
			parts[i] = int(s.Range(2, 5))
			sum += parts[i]
		}
		given := 0
		for i, mon := range list {
			k := max(n*parts[i]/sum, 1)
			if i == len(list)-1 {
				k = max(n-given, 1)
			}
			given += k
			add(mon, min(k, MaxCount))
		}
	}
	split(byRole[Common], common)
	split(byRole[Uncommon], rest-common)
	r := node.Child("respawn").SecretStream()
	place := node.Child("place").SecretStream()
	for i := range out {
		out[i].RespawnMs = respawn(r, out[i].Role)
		if out[i].Role == Plant && len(m.FeaturePoints) > 0 {
			pt := m.FeaturePoints[place.IntN(len(m.FeaturePoints))]
			out[i].Area = Area{X: pt.X, Y: pt.Y, W: PlantReach, H: PlantReach}
		}
		if mon, ok := find(chosen, out[i].Monster); ok {
			out[i].Avoid = Clearance(m, mon, out[i].Role)
		}
	}
	SortSpawns(out)
	return out
}

func find(list []Monster, name string) (Monster, bool) {
	i := slices.IndexFunc(list, func(m Monster) bool { return m.Name == name })
	if i < 0 {
		return Monster{}, false
	}
	return list[i], true
}

// respawn draws a role's respawn time (strategy section 8.11): common 5
// seconds, uncommon 5 to 60 seconds, plants 3 to 30 minutes, rare and
// mini bosses 30 to 60 minutes, field bosses 1 to 2 hours, in whole
// seconds.
func respawn(s *rng.Rand, r Role) int {
	sec := int64(5)
	switch r {
	case Uncommon:
		sec = s.Range(5, 60)
	case Plant:
		sec = s.Range(3*60, 30*60)
	case Rare, MiniBoss:
		sec = s.Range(30*60, 60*60)
	case FieldBoss:
		sec = s.Range(60*60, 120*60)
	}
	return int(sec) * 1000
}

// Clearance returns the cells a spawn keeps clear (review decision 44):
// an aggressive monster stays ArrivalClear cells from every arrival,
// and a boss TownPortalClear cells from every warp to a town.
func Clearance(m Map, mon Monster, role Role) []Avoid {
	var out []Avoid
	if mon.Aggressive {
		for _, a := range m.Arrivals {
			out = append(out, Avoid{X: a.X, Y: a.Y, R: ArrivalClear})
		}
	}
	if role.Boss() {
		for _, t := range m.TownPortals {
			out = append(out, Avoid{X: t.X, Y: t.Y, R: TownPortalClear})
		}
	}
	return out
}

// SortSpawns orders a spawn list: by role, then by count from the
// largest, then by monster name.
func SortSpawns(list []Spawn) {
	slices.SortStableFunc(list, func(a, b Spawn) int {
		return cmp.Or(cmp.Compare(a.Role.rank(), b.Role.rank()), cmp.Compare(b.Count, a.Count), cmp.Compare(a.Monster, b.Monster))
	})
}
