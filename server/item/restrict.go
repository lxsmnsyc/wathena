package item

import (
	"fmt"
	"slices"
)

// Restrictions and bound copies (proposal 0002, review decision 29):
//   - An item's restrictions are rAthena's item_trade flags: where the
//     item may not go. data/items.yml lists them by name.
//   - itemdb_isrestricted checks the item and every card in it, so a
//     card with no_trade makes the item it is in untradeable too.
//   - A bound copy, as rAthena's item.bound, cannot be traded or
//     dropped, and goes only into storage its owner shares.
//   - rAthena's Override lets group levels ignore the flags. wAthena has
//     no group levels in the game yet, so no one overrides them.

// Restriction is one item_trade flag.
type Restriction uint8

// The restrictions, as bits. The values are the protocol's
// ItemRestriction bits.
const (
	NoDrop Restriction = 1 << iota
	NoTrade
	NoSellFlag
	NoCart
	NoStorage
	NoGuildStorage
	NoMail
)

// restrictionNames are the names in data/items.yml, in bit order.
var restrictionNames = []string{"no_drop", "no_trade", "no_sell", "no_cart", "no_storage", "no_guild_storage", "no_mail"}

// RestrictionNames lists the names data/items.yml takes.
func RestrictionNames() []string { return slices.Clone(restrictionNames) }

// ParseRestriction reads a restriction's name.
func ParseRestriction(name string) (Restriction, error) {
	i := slices.Index(restrictionNames, name)
	if i < 0 {
		return 0, fmt.Errorf("%q is not a restriction; the restrictions are %v", name, restrictionNames)
	}
	return Restriction(1 << i), nil
}

// RestrictionMask is the item's restrictions as bits. Unknown names,
// which checkItem refuses, are left out.
func (d *Def) RestrictionMask() Restriction {
	var m Restriction
	for _, n := range d.Restrictions {
		if r, err := ParseRestriction(n); err == nil {
			m |= r
		}
	}
	if d.NoDrop {
		m |= NoDrop
	}
	if d.NoSell {
		m |= NoSellFlag
	}
	return m
}

// Restricted is itemdb_isrestricted for one flag: the item of a slot or
// a card in it has the restriction.
func (t *Table) Restricted(s *Slot, r Restriction) bool {
	if t.Get(s.ID).RestrictionMask()&r != 0 {
		return true
	}
	for _, c := range s.Cards {
		if t.Get(c).RestrictionMask()&r != 0 {
			return true
		}
	}
	return false
}

// SyncRestrictions keeps no_drop and no_sell, which item data had before
// restrictions, in step with the restrictions list. fromList says which
// one the data set last: the list, or the two fields.
func (d *Def) SyncRestrictions(fromList bool) {
	if fromList {
		d.NoDrop = slices.Contains(d.Restrictions, "no_drop")
		d.NoSell = slices.Contains(d.Restrictions, "no_sell")
		return
	}
	set := func(name string, on bool) {
		i := slices.Index(d.Restrictions, name)
		switch {
		case on && i < 0:
			d.Restrictions = append(d.Restrictions, name)
		case !on && i >= 0:
			d.Restrictions = slices.Delete(slices.Clone(d.Restrictions), i, i+1)
		}
	}
	set("no_drop", d.NoDrop)
	set("no_sell", d.NoSell)
	d.SortRestrictions()
}

// SortRestrictions puts the restrictions in bit order, without repeats.
func (d *Def) SortRestrictions() {
	var out []string
	for _, n := range restrictionNames {
		if slices.Contains(d.Restrictions, n) {
			out = append(out, n)
		}
	}
	d.Restrictions = out
}

// Bound is rAthena's bound_type.
type Bound uint8

// The bound types, with rAthena's values.
const (
	BoundNone Bound = iota
	BoundAccount
	BoundGuild
	BoundParty
	BoundCharacter
)

var boundNames = []string{"", "account", "guild", "party", "character"}

// String is the bound's name in Lua and logs, or "" for none.
func (b Bound) String() string {
	if int(b) < len(boundNames) {
		return boundNames[b]
	}
	return ""
}

// ParseBound reads a bound's name: account, character, party or guild.
func ParseBound(s string) (Bound, bool) {
	i := slices.Index(boundNames, s)
	if i <= 0 {
		return BoundNone, false
	}
	return Bound(i), true // #nosec G115 -- below len(boundNames).
}

// Places a bound copy may go into, for CanHold.
const (
	PlaceStorage = iota
	PlaceGuildStorage
	PlaceCart
	PlaceTrade
	PlaceDrop
	PlaceSell
	PlaceMail
)

// BoundAllows is rAthena's bound check of each place, for a player
// without the bounded item permission:
//   - Storage and cart take account-bound copies (bound > BOUND_ACCOUNT
//     is refused).
//   - Guild storage takes guild-bound copies only (BOUND_ACCOUNT or
//     bound > BOUND_GUILD is refused).
//   - Trade takes guild-bound copies to a guildmate only. sameGuild says
//     whether the trade partner is in the owner's guild.
//   - Drop, sell and mail take no bound copy.
func BoundAllows(b Bound, place int, sameGuild bool) bool {
	if b == BoundNone {
		return true
	}
	switch place {
	case PlaceStorage, PlaceCart:
		return b <= BoundAccount
	case PlaceGuildStorage:
		return b == BoundGuild
	case PlaceTrade:
		return b == BoundGuild && sameGuild
	}
	return false
}
