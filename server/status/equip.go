package status

import (
	"fmt"
	"strings"

	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
)

// Equipment is what a player wears, for Calc: map_session_data's
// equip_index with the inventory data and item instances.
type Equipment struct {
	// Items holds the item in each equip index (item.Eqi*). An item in
	// two positions, such as a two-handed weapon, is the same pointer in
	// both, as pc_is_same_equip_index finds.
	Items [item.EqiMax]*Equipped
	// Worn holds the weapon types and the shield, from item.Inventory.Worn.
	Worn item.Worn
	// MapType is the map's type as item.NoEquip names it, such as
	// item.MapNormal. Scripts of items and cards whose NoEquip has it do
	// not run, as itemdb_isNoEquip.
	MapType uint32
}

// Equipped is one equipped item instance.
type Equipped struct {
	Def *item.Def
	// Position is where it is equipped (EQP_ bits), as item.equip.
	Position uint32
	Refine   int
	// Bonuses are the bonuses of the item's script, in the order the
	// script added them: its static bonuses, then those of its Lua
	// script whose condition holds, with computed values resolved.
	Bonuses []Bonus
	// Cards are the cards in its slots, in slot order.
	Cards []Card
	// Options are the bonuses of its random options, in option order.
	// They run after every card, as status_calc_pc_sub's loop over
	// random options.
	Options []Bonus
}

// Card is a card in an equipped item.
type Card struct {
	Def     *item.Def
	Bonuses []Bonus
}

// Bonus is one active bonus with its values: the name, the numbers in
// order, the chance of a proc, and the options of bonuses.add.
type Bonus struct {
	Name    string
	Args    []int64
	Chance  int64
	Options *bonus.Options
}

// FromBonus makes a Bonus from a checked bonus whose values are fixed,
// such as a static bonus of item data.
func FromBonus(b *bonus.Bonus) Bonus { return FromRolled(b, nil) }

// FromRolled makes a Bonus from a static bonus whose numbers may read a
// copy's rolls, as { roll: name }. roll may be nil for none.
func FromRolled(b *bonus.Bonus, roll func(name string) (int, bool)) Bonus {
	if roll == nil {
		roll = func(string) (int, bool) { return 0, false }
	}
	out := Bonus{Name: b.Name, Chance: b.Options.Chance.Value, Options: &b.Options}
	for _, a := range b.Args {
		out.Args = append(out.Args, a.Resolve(roll))
	}
	return out
}

// RefineData is refine_db.json: rAthena's refine.yml with the Normal cost
// of each refine level. Other cost types are left out.
type RefineData struct {
	Mode      string `json:"mode"`
	MaxRefine int    `json:"max_refine"`
	Groups    []struct {
		Group        string `json:"group"`
		Level        int    `json:"level"`
		RefineLevels []struct {
			Level       int `json:"level"`
			Bonus       int `json:"bonus"`
			RandomBonus int `json:"random_bonus"`
			// Normal is nil when the level has no Normal chance.
			Normal *RefineCost `json:"normal"`
		} `json:"refine_levels"`
	} `json:"groups"`
}

// RefineCost is s_refine_cost of the Normal cost type. Rate and
// BreakingRate are out of 10000. Price is in zeny. Material is the
// AegisName of the ore the refine takes.
type RefineCost struct {
	Rate         int    `json:"rate"`
	Price        int    `json:"price"`
	Material     string `json:"material"`
	BreakingRate int    `json:"breaking_rate"`
}

// refineInfo is s_refine_level_info: bonus and randombonus_max, in
// hundredths, and the Normal cost. normal is nil when there is none.
type refineInfo struct {
	bonus, randomBonus int
	normal             *RefineCost
}

// refineKey is a refine type and an item level.
type refineKey struct {
	group string
	level int
}

// refineTable is refine_db.
type refineTable struct {
	max    int
	levels map[refineKey]map[int]refineInfo
}

// SetRefine loads the refine table. Without it, refines add nothing.
func (t *Tables) SetRefine(d *RefineData) error {
	if (d.Mode == "renewal") != t.renewal {
		return fmt.Errorf("status: refine table of mode %q for %s tables", d.Mode, map[bool]string{true: "renewal", false: "pre-renewal"}[t.renewal])
	}
	r := &refineTable{max: d.MaxRefine, levels: map[refineKey]map[int]refineInfo{}}
	for _, g := range d.Groups {
		k := refineKey{strings.ToLower(g.Group), g.Level}
		m := r.levels[k]
		if m == nil {
			m = map[int]refineInfo{}
			r.levels[k] = m
		}
		for _, l := range g.RefineLevels {
			m[l.Level] = refineInfo{l.Bonus, l.RandomBonus, l.Normal}
		}
	}
	t.refine = r
	return nil
}

// MaxRefine is MAX_REFINE: 20 in renewal and 10 in pre-renewal.
func (t *Tables) MaxRefine() int {
	if t.refine != nil {
		return t.refine.max
	}
	if t.renewal {
		return 20
	}
	return 10
}

// RefineCost is the Normal cost of refining an item from refine to
// refine+1, as RefineDatabase::findLevelInfo finds it. weapon picks the
// Weapon refine type over Armor. level is the weapon level or armor level.
// The cost is in the refine level refine+1 of refine.yml. It returns false
// at or above MAX_REFINE, for a negative refine, or when the table has no
// Normal chance there.
func (t *Tables) RefineCost(weapon bool, level, refine int) (RefineCost, bool) {
	if t.refine == nil || refine < 0 || refine >= t.refine.max {
		return RefineCost{}, false
	}
	k := refineKey{"armor", level}
	if weapon {
		k.group = "weapon"
	}
	info, ok := t.refine.levels[k][refine+1]
	if !ok || info.normal == nil {
		return RefineCost{}, false
	}
	return *info.normal, true
}

// RefineCostOf is RefineCost for an item, as calculate_refine_info picks
// its refine type and level (status.cpp 465): a weapon by its weapon
// level, an armor by its armor level, and shadow gear at level 1. ok is
// false for an item that is not refined, or a cost the table lacks.
func (t *Tables) RefineCostOf(d *item.Def, refine int) (RefineCost, bool) {
	if t.refine == nil || !d.Refineable || refine < 0 || refine >= t.refine.max {
		return RefineCost{}, false
	}
	var k refineKey
	switch {
	case d.IsWeapon():
		k = refineKey{"weapon", max(d.WeaponLevel, 1)}
	case d.IsArmor():
		k = refineKey{"armor", max(d.ArmorLevel, 1)}
	case d.IsShadowGear():
		k = refineKey{"shadow_armor", 1}
		if uint32(d.Location) == item.EqpShadowWeapon {
			k.group = "shadow_weapon"
		}
	default:
		return RefineCost{}, false
	}
	info, ok := t.refine.levels[k][refine+1]
	if !ok || info.normal == nil {
		return RefineCost{}, false
	}
	return *info.normal, true
}

// currentRefine is RefineDatabase::findCurrentLevelInfo: the bonus of an
// item at its refine level, or nil for refine 0, an item that cannot be
// refined, or a level the table lacks.
func (t *Tables) currentRefine(d *item.Def, refine int) *refineInfo {
	if t.refine == nil || refine <= 0 || !d.Refineable {
		return nil
	}
	refine = min(refine, t.refine.max)
	var k refineKey
	switch {
	case d.IsWeapon():
		k = refineKey{"weapon", d.WeaponLevel}
	case d.IsArmor():
		k = refineKey{"armor", d.ArmorLevel}
	case d.IsShadowGear():
		k = refineKey{"shadow_armor", 1}
		if uint32(d.Location) == item.EqpShadowWeapon {
			k.group = "shadow_weapon"
		}
	default:
		return nil
	}
	info, ok := t.refine.levels[k][refine]
	if !ok {
		return nil
	}
	return &info
}

// BonusSource returns the active bonuses of an equipped item's script
// (card -1) or of one of its cards (the card's index), with computed
// values resolved. slot is the inventory slot.
type BonusSource func(slot, card int, d *item.Def) []Bonus

// Wear builds the Equipment of an inventory, as pc_setequipindex and
// pc_setinventorydata: each equip index points at its slot's item, and
// an item in two positions is one Equipped. Items and cards the table
// lacks are left out. src may be nil for no bonuses.
func Wear(inv *item.Inventory, t *item.Table, src BonusSource) *Equipment {
	return WearOptions(inv, t, src, nil)
}

// WearOptions is Wear with the random options of each copy: find returns
// an option's data. A nil find leaves options out.
func WearOptions(inv *item.Inventory, t *item.Table, src BonusSource, find func(name string) *item.RandomOption) *Equipment {
	e := &Equipment{Worn: inv.Worn(t), MapType: item.MapNormal}
	bySlot := map[int]*Equipped{}
	for i, slot := range e.Worn.Index {
		if slot < 0 {
			continue
		}
		eq, ok := bySlot[slot]
		if !ok {
			s := &inv.Slots[slot]
			d, known := t.Lookup(s.ID)
			if !known {
				continue
			}
			eq = &Equipped{Def: d, Position: s.Equip, Refine: s.Refine}
			if src != nil {
				eq.Bonuses = src(slot, -1, d)
			}
			for c, id := range s.Cards {
				cd, known := t.Lookup(id)
				if !known {
					continue
				}
				card := Card{Def: cd}
				if src != nil {
					card.Bonuses = src(slot, c, cd)
				}
				eq.Cards = append(eq.Cards, card)
			}
			if find != nil {
				eq.Options = OptionBonuses(s.Options, find)
			}
			bySlot[slot] = eq
		}
		e.Items[i] = eq
	}
	return e
}

// StaticBonuses are an item's static bonuses as Bonus values, with the
// copy's rolls. A roll the copy lacks reads its default.
func StaticBonuses(d *item.Def, rolls item.Rolls) []Bonus {
	out := make([]Bonus, 0, len(d.Static))
	roll := func(name string) (int, bool) {
		if v, ok := rolls.Get(name); ok {
			return v, true
		}
		if r := d.Rolls[name]; r != nil {
			return r.Default, true
		}
		return 0, false
	}
	for _, b := range d.Static {
		out = append(out, FromRolled(b, roll))
	}
	return out
}

// OptionBonuses are the bonuses of a copy's random options, in option
// order. find returns an option's data, or nil for one the world lacks,
// which gives nothing.
func OptionBonuses(opts []item.Option, find func(name string) *item.RandomOption) []Bonus {
	var out []Bonus
	for _, o := range opts {
		d := find(o.Name)
		if d == nil {
			continue
		}
		roll := func(name string) (int, bool) {
			switch name {
			case "value":
				return o.Value, true
			case "param":
				return o.Param, true
			}
			return 0, false
		}
		for _, b := range d.Static {
			out = append(out, FromRolled(b, roll))
		}
	}
	return out
}
