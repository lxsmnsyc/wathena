package item

import (
	"fmt"
	"strings"
)

// Item groups, as rAthena's ItemGroupDatabase in itemdb.cpp: the boxes
// that getgroupitem opens. item_groups.json holds the groups as rAthena
// loads them.

// GroupData is the item_groups.json extract.
type GroupData struct {
	Mode   string  `json:"mode"`
	Groups []Group `json:"groups"`
}

// Group is one item group: its subgroups in subgroup order.
type Group struct {
	// ID is the group's IG_ constant.
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Subgroups []Subgroup `json:"subgroups"`
}

// Algorithms of a subgroup, as e_group_algorithm_type.
const (
	AlgorithmSharedPool = "SharedPool"
	AlgorithmRandom     = "Random"
	AlgorithmAll        = "All"
	AlgorithmDrop       = "Drop"
)

// Subgroup is one subgroup: how it picks, and its entries in index
// order.
type Subgroup struct {
	Subgroup  int          `json:"subgroup"`
	Algorithm string       `json:"algorithm"`
	Entries   []GroupEntry `json:"entries"`
}

// GroupEntry is one item of a subgroup.
type GroupEntry struct {
	Index int `json:"index"`
	// Item is the item's AegisName.
	Item   string `json:"item"`
	Rate   int    `json:"rate"`
	Amount int    `json:"amount"`
	// Duration makes the item a rental, in minutes.
	Duration int `json:"duration"`
	// Stacked is false when stackable items are given one at a time.
	// The extract leaves it out when true, rAthena's default.
	Stacked *bool `json:"stacked"`
	// Announced, UniqueID, Named, Bound, refine and grade limits, and
	// random options are read but not applied yet: items have no such
	// state.
	Announced         bool   `json:"announced"`
	UniqueID          bool   `json:"unique_id"`
	Named             bool   `json:"named"`
	Bound             string `json:"bound"`
	RefineMinimum     int    `json:"refine_minimum"`
	RefineMaximum     int    `json:"refine_maximum"`
	GradeMinimum      string `json:"grade_minimum"`
	GradeMaximum      string `json:"grade_maximum"`
	RandomOptionGroup string `json:"random_option_group"`
}

// Groups holds the item groups, and the item IDs of their entries.
type Groups struct {
	byID   map[int64]*Group
	byName map[string]*Group
	// items are the item IDs of each entry, by group ID, subgroup and
	// entry, resolved once.
	items map[int64][][]uint32
}

// NewGroups builds the groups. ids finds an item's ID by AegisName.
// Entries whose item is unknown are an error.
func NewGroups(d *GroupData, ids func(name string) (uint32, bool)) (*Groups, error) {
	g := &Groups{byID: map[int64]*Group{}, byName: map[string]*Group{}, items: map[int64][][]uint32{}}
	for i := range d.Groups {
		gr := &d.Groups[i]
		g.byID[gr.ID] = gr
		g.byName[strings.ToLower(gr.Name)] = gr
		subs := make([][]uint32, len(gr.Subgroups))
		for s, sub := range gr.Subgroups {
			for _, e := range sub.Entries {
				id, ok := ids(e.Item)
				if !ok {
					return nil, fmt.Errorf("item group %s: no item %s", gr.Name, e.Item)
				}
				subs[s] = append(subs[s], id)
			}
		}
		g.items[gr.ID] = subs
	}
	return g, nil
}

// ByName finds a group by its name in item_group_db, without case, as
// rAthena finds IG_ constants.
func (g *Groups) ByName(name string) (*Group, bool) {
	if g == nil {
		return nil, false
	}
	gr, ok := g.byName[strings.ToLower(name)]
	return gr, ok
}

// ByID finds a group by its ID.
func (g *Groups) ByID(id int64) (*Group, bool) {
	if g == nil {
		return nil, false
	}
	gr, ok := g.byID[id]
	return gr, ok
}

// Pool is the shared pool state of SharedPool subgroups: how many times
// each entry was given. rAthena keeps one for the whole server. wAthena
// keeps one per zone, so each zone gives every entry its rate's share.
type Pool struct {
	given map[poolKey]int
	total map[poolKey]int
}

type poolKey struct {
	group    int64
	subgroup int
	entry    int
}

// NewPool makes an empty pool state.
func NewPool() *Pool { return &Pool{given: map[poolKey]int{}, total: map[poolKey]int{}} }

// Grant is one item a group gives: its ID, the amount of one add, and how
// many adds.
type Grant struct {
	Item   uint32
	Amount int
	// Rental is the rental time in seconds, or 0.
	Rental int
	// Stacked is set when the amount goes in as one add.
	Stacked bool
	Entry   *GroupEntry
}

// Open is pc_get_itemgroup: it picks the items one use of a group gives,
// subgroup by subgroup:
//   - Random picks one entry, with a chance of rate over the total rate.
//   - SharedPool picks one entry from what is left of the pool, then
//     takes that entry's share out. An empty pool fills again.
//   - All gives every entry.
//   - Drop subgroups give nothing here, as in rAthena.
//
// Random draws follow the order of the subgroups and entries in the
// extract. rAthena's order follows its unordered maps, which depends on
// the C++ library, so the draws match rAthena's chances but not its
// sequence.
func (g *Groups) Open(gr *Group, r Rand, pool *Pool) []Grant {
	var out []Grant
	ids := g.items[gr.ID]
	for s := range gr.Subgroups {
		sub := &gr.Subgroups[s]
		switch sub.Algorithm {
		case AlgorithmAll:
			for e := range sub.Entries {
				out = append(out, grant(&sub.Entries[e], ids[s][e]))
			}
		case AlgorithmRandom:
			if e := pickRandom(sub, r); e >= 0 {
				out = append(out, grant(&sub.Entries[e], ids[s][e]))
			}
		case AlgorithmSharedPool:
			if e := pickShared(gr.ID, s, sub, r, pool); e >= 0 {
				out = append(out, grant(&sub.Entries[e], ids[s][e]))
			}
		}
	}
	return out
}

func grant(e *GroupEntry, id uint32) Grant {
	amount := max(e.Amount, 1)
	return Grant{Item: id, Amount: amount, Rental: e.Duration * 60, Stacked: e.Stacked == nil || *e.Stacked, Entry: e}
}

// pickRandom is GROUP_ALGORITHM_RANDOM: a position from 1 to the total
// rate, and the entry whose rate covers it.
func pickRandom(sub *Subgroup, r Rand) int {
	total := 0
	for _, e := range sub.Entries {
		total += e.Rate
	}
	if total <= 0 {
		return -1
	}
	pos := 1 + r.IntN(total)
	cur := 1
	for i, e := range sub.Entries {
		cur += e.Rate
		if cur > pos {
			return i
		}
	}
	return -1
}

// pickShared is GROUP_ALGORITHM_SHAREDPOOL.
func pickShared(group int64, s int, sub *Subgroup, r Rand, pool *Pool) int {
	total := 0
	for _, e := range sub.Entries {
		total += e.Rate
	}
	key := poolKey{group: group, subgroup: s, entry: -1}
	givenTotal := pool.total[key]
	if total-givenTotal <= 0 {
		return -1
	}
	pos := 1 + r.IntN(total-givenTotal)
	cur := 1
	for i, e := range sub.Entries {
		ek := poolKey{group: group, subgroup: s, entry: i}
		cur += e.Rate - pool.given[ek]
		if cur > pos {
			pool.given[ek]++
			pool.total[key]++
			if pool.total[key] >= total {
				// Every entry was given out: the pool fills again.
				pool.total[key] = 0
				for j := range sub.Entries {
					delete(pool.given, poolKey{group: group, subgroup: s, entry: j})
				}
			}
			return i
		}
	}
	return -1
}

// Contains is itemdb_group.item_exists: whether a group of a name lists
// an item in any subgroup.
func (g *Groups) Contains(name string, id uint32) bool {
	gr, ok := g.ByName(name)
	if !ok {
		return false
	}
	for _, sub := range g.items[gr.ID] {
		for _, it := range sub {
			if it == id {
				return true
			}
		}
	}
	return false
}
