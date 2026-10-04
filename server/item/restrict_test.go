package item

import "testing"

func TestRestrictions(t *testing.T) {
	tb := New(&Data{Items: []Def{
		{ID: 1, Restrictions: []string{"no_trade", "no_storage"}},
		{ID: 2, Type: "Card", Restrictions: []string{"no_cart"}},
		{ID: 3, NoDrop: true},
		{ID: 4},
	}})
	if !tb.Restricted(&Slot{ID: 1}, NoTrade) || tb.Restricted(&Slot{ID: 1}, NoCart) {
		t.Error("item 1")
	}
	// itemdb_isrestricted checks the cards in an item.
	if !tb.Restricted(&Slot{ID: 4, Cards: []uint32{2}}, NoCart) || tb.Restricted(&Slot{ID: 4}, NoCart) {
		t.Error("a card's restriction")
	}
	if !tb.Restricted(&Slot{ID: 3}, NoDrop) {
		t.Error("no_drop from the old field")
	}
	if _, err := ParseRestriction("no_auction"); err == nil {
		t.Error("no_auction is not a restriction of wAthena")
	}
	d := Def{NoDrop: true, Restrictions: []string{"no_mail"}}
	d.SyncRestrictions(false)
	if len(d.Restrictions) != 2 || d.Restrictions[0] != "no_drop" || d.Restrictions[1] != "no_mail" {
		t.Errorf("sync from the fields: %v", d.Restrictions)
	}
	d = Def{NoDrop: true, Restrictions: []string{"no_sell"}}
	d.SyncRestrictions(true)
	if d.NoDrop || !d.NoSell {
		t.Errorf("sync from the list: %+v", d)
	}
}

func TestBound(t *testing.T) {
	for _, c := range []struct {
		b     Bound
		place int
		guild bool
		want  bool
	}{
		{BoundNone, PlaceTrade, false, true},
		{BoundAccount, PlaceStorage, false, true},
		{BoundAccount, PlaceCart, false, true},
		{BoundAccount, PlaceGuildStorage, false, false},
		{BoundAccount, PlaceTrade, true, false},
		{BoundCharacter, PlaceStorage, false, false},
		{BoundParty, PlaceCart, false, false},
		{BoundGuild, PlaceGuildStorage, false, true},
		{BoundGuild, PlaceStorage, false, false},
		{BoundGuild, PlaceTrade, true, true},
		{BoundGuild, PlaceTrade, false, false},
		{BoundAccount, PlaceDrop, false, false},
		{BoundAccount, PlaceSell, false, false},
	} {
		if got := BoundAllows(c.b, c.place, c.guild); got != c.want {
			t.Errorf("%v at %d (guild %v): %v", c.b, c.place, c.guild, got)
		}
	}
	// Bound and unbound copies do not stack.
	x, y := Extra{Bound: BoundAccount}, Extra{}
	if x.Equal(&y) || x.Clone().Bound != BoundAccount {
		t.Error("bound in Equal or Clone")
	}
	if b, ok := ParseBound("character"); !ok || b != BoundCharacter || b.String() != "character" {
		t.Error("ParseBound")
	}
	if _, ok := ParseBound(""); ok {
		t.Error("empty bound parsed")
	}
}
