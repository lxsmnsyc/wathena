package content

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// candidate is a monster a hunt or collect quest may name, with its
// weight: the monsters its spawn lines keep alive.
type candidate struct {
	monster string
	weight  int
}

// line is a field's spawn line with the field's warps from a town.
type line struct {
	spawns.Spawn
	warps int
}

// fieldLines returns the spawn lines of every field a town reaches, with
// their warps from it, in the spawn lists' order. Bosses are left out.
func (g *generator) fieldLines(town string) []line {
	var out []line
	for _, p := range g.w.Spawns {
		m := g.in.Layout.Map(p.Map)
		if m == nil || m.Kind != gen.KindField {
			continue
		}
		d := g.atlas.Distance(town, p.Map)
		if d < 0 {
			continue
		}
		for _, s := range p.Spawns {
			if s.Role.Boss() {
				continue
			}
			if mi, ok := g.monster(s.Monster); !ok || mi.Boss {
				continue
			}
			out = append(out, line{Spawn: s, warps: d})
		}
	}
	return out
}

// monster finds a monster's data.
func (g *generator) monster(name string) (MonsterInfo, bool) {
	if g.in.Data.Monster == nil {
		return MonsterInfo{}, false
	}
	return g.in.Data.Monster(name)
}

// item finds an item's data.
func (g *generator) item(name string) (ItemInfo, bool) {
	if g.in.Data.Item == nil {
		return ItemInfo{}, false
	}
	return g.in.Data.Item(name)
}

// merge sums the lines' counts by monster, in the order of first sight.
func merge(lines []line, keep func(line) bool) []candidate {
	var out []candidate
	for _, l := range lines {
		if !keep(l) {
			continue
		}
		i := slices.IndexFunc(out, func(c candidate) bool { return c.monster == l.Monster })
		if i < 0 {
			out = append(out, candidate{monster: l.Monster})
			i = len(out) - 1
		}
		out[i].weight += l.Count
	}
	return out
}

// boardPool returns the monsters of a town's nearby fields in roles:
// those within NearbyWarps warps, or further up to MaxNearbyWarps when
// none qualifies.
func (g *generator) boardPool(lines []line, roles []spawns.Role, ok func(string) bool) []candidate {
	for reach := NearbyWarps; reach <= MaxNearbyWarps; reach++ {
		c := merge(lines, func(l line) bool { return l.warps <= reach && slices.Contains(roles, l.Role) && ok(l.Monster) })
		if len(c) > 0 {
			return c
		}
	}
	return nil
}

// guildWindows are how far a guild test's monsters may be from the
// stage's level, tried in order. The last takes any level.
var guildWindows = []int{5, 10, 20, 40, 1000}

// guildPool returns the monsters of a guild test: in roles, within the
// first window of the stage's level that has any, and of those the
// fields fewest warps from the guild's town, or one more.
func (g *generator) guildPool(lines []line, roles []spawns.Role, level int, ok func(string) bool) []candidate {
	for _, w := range guildWindows {
		var fit []line
		for _, l := range lines {
			mi, found := g.monster(l.Monster)
			if !found || !slices.Contains(roles, l.Role) || abs(mi.Level-level) > w || !ok(l.Monster) {
				continue
			}
			fit = append(fit, l)
		}
		if len(fit) == 0 {
			continue
		}
		least := fit[0].warps
		for _, l := range fit {
			least = min(least, l.warps)
		}
		return merge(fit, func(l line) bool { return l.warps <= least+1 })
	}
	return nil
}

// pick draws a candidate by weight.
func pick(s *rng.Rand, list []candidate) candidate {
	w := make([]uint32, len(list))
	for i, c := range list {
		w[i] = uint32(min(max(c.weight, 1), 1<<20)) // #nosec G115 -- clamped above.
	}
	return list[s.Weighted(w)]
}

// collectDrops returns a monster's drops a collect quest may ask for,
// as candidates weighted by their rate.
func (g *generator) collectDrops(monster string) []candidate {
	mi, ok := g.monster(monster)
	if !ok {
		return nil
	}
	var out []candidate
	for _, d := range mi.Drops {
		it, found := g.item(d.Item)
		if !found || !it.Collectable || d.Rate <= 0 || slices.ContainsFunc(out, func(c candidate) bool { return c.monster == d.Item }) {
			continue
		}
		out = append(out, candidate{monster: d.Item, weight: d.Rate})
	}
	return out
}

// templatesFor returns the templates of a use and kinds, in record
// order.
func (g *generator) templatesFor(use string, kinds ...string) []Template {
	var out []Template
	for _, t := range g.w.Templates {
		if t.Has(use) && (len(kinds) == 0 || slices.Contains(kinds, t.Kind)) {
			out = append(out, t)
		}
	}
	return out
}

func weights(list []Template) []uint32 {
	w := make([]uint32, len(list))
	for i, t := range list {
		w[i] = uint32(t.Weight) // #nosec G115 -- 1 to MaxWeight.
	}
	return w
}

// filled is a quest that a template filled, before its ID.
type filled struct {
	t                   Template
	monster, item, to   string
	count               int
	monsterName, itemNm string
	toName              string
}

// fill fills a template for a town from a stream, or returns false when
// nothing fits: no monster in its roles near the town, no drop to
// collect, or no other town. used are the monsters, items and towns that
// the town's quests already name.
func (g *generator) fill(s *rng.Rand, t Template, town *layout.Map, lines []line, used map[string]bool, guildLevel int) (filled, bool) {
	f := filled{t: t}
	pool := func(ok func(string) bool) []candidate {
		if guildLevel > 0 {
			return g.guildPool(lines, t.Roles, guildLevel, ok)
		}
		return g.boardPool(lines, t.Roles, ok)
	}
	switch t.Kind {
	case KindHunt:
		c := pool(func(m string) bool { return !used["hunt:"+m] })
		if len(c) == 0 {
			return f, false
		}
		f.monster = pick(s, c).monster
		f.count = int(s.Range(int64(t.Count[0]), int64(t.Count[1])))
		used["hunt:"+f.monster] = true
	case KindCollect:
		c := pool(func(m string) bool {
			for _, d := range g.collectDrops(m) {
				if !used["item:"+d.monster] {
					return true
				}
			}
			return false
		})
		if len(c) == 0 {
			return f, false
		}
		f.monster = pick(s, c).monster
		var drops []candidate
		for _, d := range g.collectDrops(f.monster) {
			if !used["item:"+d.monster] {
				drops = append(drops, d)
			}
		}
		f.item = pick(s, drops).monster
		f.count = int(s.Range(int64(t.Count[0]), int64(t.Count[1])))
		used["item:"+f.item] = true
	case KindDelivery:
		var others []layout.Map
		for _, o := range g.atlas.Towns() {
			if o.ID != town.ID && g.atlas.Distance(town.ID, o.ID) > 0 && !used["to:"+t.Name+":"+o.ID] {
				others = append(others, o)
			}
		}
		if len(others) == 0 {
			return f, false
		}
		slices.SortStableFunc(others, func(a, b layout.Map) int {
			return cmp.Or(cmp.Compare(g.atlas.Distance(town.ID, a.ID), g.atlas.Distance(town.ID, b.ID)), cmp.Compare(a.Town, b.Town))
		})
		others = others[:min(len(others), 3)]
		to := others[s.IntN(len(others))]
		f.to, f.toName, f.item = to.ID, to.Name, t.Item
		used["to:"+t.Name+":"+to.ID] = true
	}
	if f.monster != "" {
		f.monsterName = f.monster
		if mi, ok := g.monster(f.monster); ok && mi.DisplayName != "" {
			f.monsterName = mi.DisplayName
		}
	}
	if f.item != "" {
		f.itemNm = f.item
		if it, ok := g.item(f.item); ok && it.DisplayName != "" {
			f.itemNm = it.DisplayName
		}
	}
	return f, true
}

// quest makes a quest of a filled template.
func (g *generator) quest(f filled, town *layout.Map) (Quest, error) {
	id, err := g.newID()
	if err != nil {
		return Quest{}, err
	}
	q := Quest{ID: id, Town: town.ID, Template: f.t.Name, Kind: f.t.Kind, Monster: f.monster, Item: f.item, Count: f.count, To: f.to,
		Title: title(f.t, f.count, f.monsterName, f.itemNm, f.toName)}
	if f.t.Kind == KindCollect {
		q.DropRate = f.t.DropRate
	}
	return q, nil
}

// boards makes each town's quest board: its place near the square, and
// BoardQuests quests from the board templates, drawn by weight from the
// town's secret quest node. A template that nothing fits is drawn again,
// up to 8 tries for each quest.
func (g *generator) boards() error {
	list := g.templatesFor(UseBoard)
	for _, town := range g.towns() {
		x, y := g.anchor(town.ID, TagSquare)
		if _, ok := g.addPlace(town.ID, x, y, placeNear, placeFar, []string{TagQuestBoard}, SourceCore); !ok {
			return fmt.Errorf("%w: the town %s has no free cell for its quest board", ErrContent, town.ID)
		}
		if len(list) == 0 {
			continue
		}
		s := TownNode(g.in.Secret, town.Town).SecretStream()
		lines := g.fieldLines(town.ID)
		used := map[string]bool{}
		w := weights(list)
		made := 0
		for tries := 0; made < BoardQuests && tries < BoardQuests*8; tries++ {
			f, ok := g.fill(s, list[s.Weighted(w)], town, lines, used, 0)
			if !ok {
				continue
			}
			q, err := g.quest(f, town)
			if err != nil {
				return err
			}
			q.Purpose = PurposeBoard
			q.BaseExp, q.JobExp, q.Zeny = g.reward(f.t, town.Levels)
			g.w.Quests = append(g.w.Quests, q)
			made++
		}
	}
	return nil
}

// GuildLevel is the highest band start of a guild's town. A Novice
// reaches job level 10 at about base level 10 to 15, so every guild
// stands in a town of a band that starts at this level or lower.
const GuildLevel = 20

// GuildTowns returns the town of each family's guild, in family order
// (decision 0047):
//
//   - The hosts are the towns of a band that starts at GuildLevel or
//     lower, by their band's lowest level, then number, the first three
//     at most.
//   - A world without such a town has the lowest town as its one host.
//   - The hosts take the families in turn.
//
// So every guild stands in a low town that a Novice reaches. A guild
// gives its first and second job tests in one place, so the second job
// tests stand in the same low town.
func GuildTowns(towns []*layout.Map) []*layout.Map {
	if len(towns) == 0 {
		return nil
	}
	sorted := sortedTowns(towns)
	hosts := 0
	for hosts < len(sorted) && hosts < 3 && sorted[hosts].Levels[0] <= GuildLevel {
		hosts++
	}
	hosts = max(hosts, 1)
	out := make([]*layout.Map, len(Families))
	for i := range Families {
		out[i] = sorted[i%hosts]
	}
	return out
}

// guilds places each family's guild and makes its tests: for each stage,
// a hunt and a collect quest from the guild templates, drawn by weight
// from the guild's secret node, with monsters near the stage's level on
// the fields nearest the guild's town. Tests give no reward, as
// rAthena's.
func (g *generator) guilds() error {
	hosts := GuildTowns(g.towns())
	if len(hosts) == 0 {
		return nil
	}
	for i, family := range Families {
		town := hosts[i]
		x, y := g.anchor(town.ID, TagGuild)
		if _, ok := g.addPlace(town.ID, x, y, 1, placeFar, []string{GuildTag(family)}, SourceCore); !ok {
			return fmt.Errorf("%w: the town %s has no free cell for the %s guild", ErrContent, town.ID, family)
		}
		lines := g.fieldLines(town.ID)
		for _, stage := range Stages {
			s := GuildNode(g.in.Secret, family, stage).SecretStream()
			used := map[string]bool{}
			for step, kind := range []string{KindHunt, KindCollect} {
				list := g.templatesFor(UseGuild, kind)
				if len(list) == 0 {
					continue
				}
				f, ok := g.fill(s, list[s.Weighted(weights(list))], town, lines, used, stageLevel[stage])
				if !ok {
					continue
				}
				q, err := g.quest(f, town)
				if err != nil {
					return err
				}
				q.Purpose, q.Guild, q.Stage, q.Step = PurposeGuild, family, stage, step+1
				g.w.Quests = append(g.w.Quests, q)
			}
		}
	}
	return nil
}

// RebirthTown returns the town of the rebirth NPC (strategy section
// 11.4): of the towns of the highest band, by its lowest level, the
// largest map, ties to the lower number.
func RebirthTown(towns []*layout.Map, size func(id string) int) *layout.Map {
	var best *layout.Map
	for _, t := range towns {
		if best == nil {
			best = t
			continue
		}
		if c := cmp.Or(cmp.Compare(t.Levels[0], best.Levels[0]), cmp.Compare(size(t.ID), size(best.ID)), cmp.Compare(best.Town, t.Town)); c > 0 {
			best = t
		}
	}
	return best
}

// rebirth places the rebirth NPC near the square of RebirthTown.
func (g *generator) rebirth() {
	t := RebirthTown(g.towns(), func(id string) int {
		if f := g.in.Maps[id]; f != nil {
			return f.Width * f.Height
		}
		return 0
	})
	if t == nil {
		return
	}
	x, y := g.anchor(t.ID, TagSquare)
	if _, ok := g.addPlace(t.ID, x, y, placeNear, placeFar, []string{TagRebirth}, SourceCore); !ok {
		g.w.Notes = append(g.w.Notes, "the town "+t.ID+" has no free cell for the rebirth NPC")
	}
}
