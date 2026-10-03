package progression

import (
	"math"
	"strings"
)

// ItemType returns an item's type from item_db in lower case, such as
// "healing" or "card", and whether the item exists.
func (t *Tables) ItemType(id uint32) (string, bool) {
	typ, ok := t.items[id]
	return typ, ok
}

// itemRate is the drop rate setting for an item type, from the switch in
// mob_drop_ratio_adjust. Item types without their own setting, such as
// etc items, ammunition and pet eggs, use the common one.
func (t *Tables) itemRate(typ string) ItemRate {
	switch typ {
	case "healing":
		return t.Heal
	case "usable", "cash":
		return t.Use
	case "weapon", "armor", "petarmor":
		return t.Equip
	case "card":
		return t.Card
	}
	return t.Common
}

// AdjustDrops is mob_drop_ratio_adjust for a monster's normal drops: it
// applies the drop rate settings for each item type and drops unknown
// items and items whose rate is 0.
//
// It does not read race2, so treasure chests use the item type settings,
// which have the same defaults as item_rate_treasure. It has no
// mob_item_ratio.yml, which is empty by default.
func (t *Tables) AdjustDrops(boss BossType, drops []Drop) []Drop {
	var out []Drop
	for _, d := range drops {
		if d.Item == 0 || d.Rate == 0 {
			continue
		}
		typ, ok := t.items[d.Item]
		if !ok {
			continue
		}
		rate := d.Rate
		if t.DropRateIncrease && rate < 5000 {
			rate++
		}
		r := t.itemRate(typ)
		adjust := r.Rate
		switch boss {
		case BossMVP:
			adjust = r.MVP
		case BossMini:
			adjust = r.Boss
		}
		if adjust == 0 {
			continue
		}
		d.Rate = t.dropAdjust(rate, adjust, r.Min, r.Max)
		out = append(out, d)
	}
	return out
}

// dropAdjust is mob_drop_adjust: a linear or logarithmic rate change,
// then the bounds.
func (t *Tables) dropAdjust(base, adjust, lo, hi int) int {
	rate := float64(base)
	if t.LogarithmicDrops && adjust > 0 && adjust != 100 && base > 0 {
		rate = rate*math.Pow(5-math.Log10(rate), math.Log(float64(adjust)/100)/math.Log(5)) + 0.5
	} else {
		rate = rate * float64(adjust) / 100
	}
	return int(min(max(rate, float64(lo)), float64(hi)))
}

// Killer is what drop rates read about the unit that dealt the last hit.
type Killer struct {
	Player bool
	Luk    int
}

// DropRate is mob_getdroprate for a drop's adjusted rate, out of 10000.
// modifier is the renewal drop penalty in percent, 100 in pre-renewal.
// killer is nil when nothing killed the monster, such as a script.
func (t *Tables) DropRate(base, modifier int, killer *Killer) int {
	rate := base
	// mob_size_influence is off.
	if killer != nil {
		if t.DropsByLuk != 0 {
			rate += killer.Luk * t.DropsByLuk / 100
		}
		if t.DropsByLuk2 != 0 {
			rate += int(0.5 + float64(rate*killer.Luk*t.DropsByLuk2)/10000)
		}
		if killer.Player {
			// No PK mode, item bonuses, item boost or VIP: the bonus is
			// 100%.
			bonus := 100
			rate = int(0.5 + float64(rate*bonus)/100)
			// The cap does not lower rates that start above it.
			if rate > t.DropRateCap && base < t.DropRateCap {
				rate = t.DropRateCap
			}
		}
	}
	rate = int(applyRate(int64(rate), modifier))
	rate = min(rate, 10000)
	if t.DropRate0Item {
		return max(rate, 0)
	}
	return max(rate, 1)
}

// Dropped is an item a kill dropped.
type Dropped struct {
	Item uint32
	// RandomOptionGroup is the drop's group of random options, or "".
	RandomOptionGroup string
	// Egg is set for a pet egg, which rAthena puts in the first looter's
	// inventory instead of on the ground.
	Egg bool
}

// RollDrops is the normal drop part of mob_dead: one roll out of 10000
// for each drop, in the monster's order. hasFirst tells whether the kill
// has a first looter, who gets pet eggs.
//
// It leaves out drops from item scripts, Ore Discovery, map drops,
// autoloot and MVP drops.
func (t *Tables) RollDrops(m *Mob, modifier int, killer *Killer, hasFirst bool, r Rand) []Dropped {
	return t.RollDropsWith(m, modifier, killer, hasFirst, r, nil)
}

// Hooks change what a kill gives, for the hooks of mods (proposal 0002,
// "Hooks"). A nil Hooks, or a nil field, keeps rAthena's result.
type Hooks struct {
	// DropRate gets each drop's rate after mob_getdroprate, per 10,000,
	// and returns the rate to roll against. Results are kept to 0 to
	// 10,000.
	DropRate func(item uint32, rate int) int
	// Experience gets the exp one player gets for the kill, after the
	// shares and the level penalty, and returns the exp to give.
	// Results below 0 count as 0.
	Experience func(charID uint32, base, job int64) (int64, int64)
	// ExpBonus is added to mob_dead's common exp bonus of 100%, as
	// SC_RICHMANKIM adds its val2.
	ExpBonus int
}

// RollDropsWith is RollDrops with hooks.
func (t *Tables) RollDropsWith(m *Mob, modifier int, killer *Killer, hasFirst bool, r Rand, h *Hooks) []Dropped {
	var out []Dropped
	for _, d := range m.Drops {
		typ, ok := t.items[d.Item]
		if d.Item == 0 || !ok {
			continue
		}
		rate := t.DropRate(d.Rate, modifier, killer)
		if h != nil && h.DropRate != nil {
			rate = min(max(h.DropRate(d.Item, rate), 0), 10000)
		}
		if r.IntN(10000) >= rate {
			continue
		}
		out = append(out, Dropped{Item: d.Item, RandomOptionGroup: d.RandomOptionGroup, Egg: hasFirst && typ == "petegg"})
	}
	return out
}

// WithItems returns a copy of the tables that also knows the types of
// more items, such as the items of mods, so their drops roll. types
// maps item IDs to item_db types in any case. Items the tables know
// keep their type.
func (t *Tables) WithItems(types map[uint32]string) *Tables {
	c := *t
	c.items = make(map[uint32]string, len(t.items)+len(types))
	for id, typ := range t.items {
		c.items[id] = typ
	}
	for id, typ := range types {
		if _, ok := c.items[id]; !ok {
			c.items[id] = strings.ToLower(typ)
		}
	}
	return &c
}
