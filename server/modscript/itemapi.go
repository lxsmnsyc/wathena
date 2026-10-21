package modscript

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/item"
)

// The item and equipment calls of review decision 58, area B9 of
// proposal 0002b: getitem2, getnameditem, rentitem, countitem2 and
// rentalcountitem through player:give_item's and player:count_item's
// options, makeitem and makeitem2, getiteminfo and getitemslots, the
// refine reads of refine.yml, successrefitem, downrefitem,
// failedrefitem and delequip, getbrokenid, repair and repairall,
// consumeitem and mergeitem.

// ItemSpec is what a copy that a script makes holds besides its item,
// as getitem2's and rentitem's arguments.
type ItemSpec struct {
	Refine int
	Cards  []uint32
	// Broken is attribute 1. Unidentified is identify 0.
	Broken, Unidentified bool
	// SignedBy is the player handle the copy is named for, as
	// getnameditem, or 0.
	SignedBy int64
	// RentSeconds above 0 makes rentals that expire, as rentitem.
	RentSeconds int64
}

// Plain reports whether the spec holds nothing past the item.
func (s *ItemSpec) Plain() bool {
	return s.Refine == 0 && len(s.Cards) == 0 && !s.Broken && !s.Unidentified && s.SignedBy == 0 && s.RentSeconds == 0
}

// CountSpec is what player:count_item's options match, as countitem2
// and rentalcountitem.
type CountSpec struct {
	// Refine and Cards, when set, count only identified copies that are
	// not broken, with that refine and exactly those cards. nil is any.
	Refine *int
	Cards  []uint32
	// Rentals counts only rentals, as rentalcountitem. Without it, only
	// copies that are not rentals count, as countitem.
	Rentals bool
}

// ItemRef names an item for the refine reads: an item ID, or an
// equipped copy, whose refine and rental the reads use.
type ItemRef struct {
	Item     uint32
	Equipped int64
}

// ItemInfo is items.get's record, getiteminfo's values.
type ItemInfo struct {
	ID                                                  uint32
	Name, AegisName, Type, Gender                       string
	Buy, Sell, Weight, Attack, MagicAttack, Defense     int
	Range, Slots, View, MinLevel, MaxLevel, Subtype     int
	WeaponLevel, ArmorLevel                             int
	Locations                                           uint32
	Refineable, Stackable, Identified, CanTrade, IsCard bool
}

// RefineKinds are the cost kinds of items.refine_cost: rAthena's
// REFINE_COST_NORMAL, REFINE_COST_HD and REFINE_COST_ENRICHED.
var RefineKinds = []string{"normal", "hd", "enriched"}

// ItemActionHost is the host of the calls of area B9.
type ItemActionHost interface {
	// GiveItemSpec is getitem2, getnameditem and rentitem: amount copies
	// with the spec, one at a time for an item that does not stack. It
	// returns false at the first that does not fit.
	GiveItemSpec(player int64, item uint32, amount int, spec ItemSpec, roll GiveRoll, bound item.Bound) (bool, error)
	// CountItemSpec is countitem2 and rentalcountitem.
	CountItemSpec(player int64, item uint32, spec CountSpec) (int, error)
	// DropItem is makeitem and makeitem2: the items on a free cell next
	// to x, y of a map.
	DropItem(mapName string, x, y int, item uint32, amount int, spec ItemSpec) error
	// ItemInfo is getiteminfo. ok is false for an unknown item.
	ItemInfo(item uint32) (ItemInfo, bool)
	// CanRefine is getequipisenableref.
	CanRefine(ref ItemRef) (bool, error)
	// RefineChance is getequippercentrefinery: the chance in percent to
	// refine from refine to refine+1. refine is -1 for an equipped
	// copy's own.
	RefineChance(ref ItemRef, refine int, enriched bool) (int, error)
	// RefineCost is getequiprefinecost: the zeny and the material's item
	// ID. ok is false when refine.yml has no such cost.
	RefineCost(ref ItemRef, refine int, kind string) (zeny, material int64, ok bool, err error)
	// EquippedRefine is successrefitem (amount above 0) and downrefitem
	// (below 0), with their effects. It returns the new refine.
	EquippedRefine(equipped int64, amount int) (int, error)
	// EquippedDestroy is delequip, and failedrefitem's deletion: the
	// copy comes off and is gone.
	// With refineFailure it is failedrefitem: the copy's refine goes to
	// 0, and the wearer shows the refine failure effect.
	EquippedDestroy(equipped int64, refineFailure bool) error
	// BrokenItems is getbrokenid: the item IDs of the broken copies, in
	// inventory order.
	BrokenItems(player int64) ([]int64, error)
	// RepairItem is repair: the n-th broken copy, from 1. It returns
	// false when there is none.
	RepairItem(player int64, n int) (bool, error)
	// RepairAll is repairall. It returns how many it repaired.
	RepairAll(player int64) (int, error)
	// RunItemScript is consumeitem: the item's script runs for the
	// player, and nothing is spent.
	RunItemScript(player int64, item uint32) error
	// MergeItems is mergeitem: it joins the stacks of one item that may
	// stack. It returns how many stacks it joined.
	MergeItems(player int64) (int, error)
}

func (r *Runtime) itemActionHost(what string) (ItemActionHost, error) {
	h, ok := r.host.(ItemActionHost)
	if !ok {
		return nil, fmt.Errorf("%s: this zone has no item actions", what)
	}
	return h, nil
}

// MaxRefineArg bounds refine arguments: MAX_REFINE is 20 in renewal.
const MaxRefineArg = 20

// readCards reads a list of card item IDs, at most 4, as getitem2's.
func readCards(what string, v script.Value) ([]uint32, error) {
	if v.IsNil() {
		return nil, nil
	}
	t, ok := v.AsTable()
	if !ok {
		return nil, errors.New(what + ": cards is a list of card item IDs")
	}
	var out []uint32
	for i := int64(1); ; i++ {
		e := t.Index(i)
		if e.IsNil() {
			break
		}
		n, ok := e.AsInt()
		if !ok || n <= 0 || n > 1<<32-1 || i > 4 {
			return nil, errors.New(what + ": cards is a list of at most 4 card item IDs")
		}
		out = append(out, uint32(n)) // #nosec G115 -- checked above.
	}
	if len(t.Fields) != len(out) {
		return nil, errors.New(what + ": cards is a list of card item IDs")
	}
	return out, nil
}

// readItemSpec reads the copy options of give_item and drop_item: a
// table of refine, cards, broken, identified and signed_by, which the
// prelude passed with signed_by as a player ID.
func readItemSpec(what string, v script.Value) (ItemSpec, error) {
	var s ItemSpec
	if v.IsNil() {
		return s, nil
	}
	t, ok := v.AsTable()
	if !ok {
		return s, errors.New(what + ": the options must be a table")
	}
	if r := t.Get("refine"); !r.IsNil() {
		n, ok := r.AsInt()
		if !ok || n < 0 || n > MaxRefineArg {
			return s, fmt.Errorf("%s: refine is 0 to %d", what, MaxRefineArg)
		}
		s.Refine = int(n)
	}
	var err error
	if s.Cards, err = readCards(what, t.Get("cards")); err != nil {
		return s, err
	}
	s.Broken = t.Get("broken").Truthy()
	if id := t.Get("identified"); !id.IsNil() {
		b, ok := id.AsBool()
		if !ok {
			return s, errors.New(what + ": identified is true or false")
		}
		s.Unidentified = !b
	}
	if p := t.Get("signed_by"); !p.IsNil() {
		if s.SignedBy, ok = p.AsInt(); !ok {
			return s, errors.New(what + ": signed_by is a player handle")
		}
	}
	return s, nil
}

// playerGiveItemSpec is player:give_item with the options of area B9 or
// rent_seconds: Arg(7) holds the copy options.
func (r *Runtime) playerGiveItemSpec(c *script.Call, id int64, itemID uint32, amount int, rent int64, roll GiveRoll, where string, bound item.Bound) error {
	const what = "player:give_item"
	spec, err := readItemSpec(what, c.Arg(7))
	if err != nil {
		return err
	}
	spec.RentSeconds = rent
	if where != "" {
		return errors.New(what + ": refine, cards, broken, identified, signed_by and rent_seconds go only to the inventory, as getitem2 and rentitem")
	}
	if rent > 0 && bound != item.BoundNone {
		return errors.New(what + ": a rental cannot be bound, as rentitem gives it unbound")
	}
	h, ok := r.host.(ItemActionHost)
	if !ok {
		// A host without the item actions gives plain items.
		if !c.Arg(7).IsNil() || roll.Mode != RollDefault {
			return errors.New(what + ": this zone has no item actions")
		}
		given, err := r.host.GiveItem(id, itemID, amount, rent)
		if err != nil {
			return err
		}
		c.Return(script.Bool(given))
		return nil
	}
	given, err := h.GiveItemSpec(id, itemID, amount, spec, roll, bound)
	if err != nil {
		return wrap(what, err)
	}
	c.Return(script.Bool(given))
	return nil
}

// playerCountItemSpec is player:count_item with refine, cards or
// rentals: Arg(3) holds them.
func (r *Runtime) playerCountItemSpec(c *script.Call, id int64, itemID uint32) error {
	const what = "player:count_item"
	t, ok := c.Arg(3).AsTable()
	if !ok {
		return errors.New(what + ": the options must be a table")
	}
	var spec CountSpec
	if v := t.Get("refine"); !v.IsNil() {
		n, ok := v.AsInt()
		if !ok || n < 0 || n > MaxRefineArg {
			return fmt.Errorf("%s: refine is 0 to %d", what, MaxRefineArg)
		}
		refine := int(n)
		spec.Refine = &refine
	}
	cards, err := readCards(what, t.Get("cards"))
	if err != nil {
		return err
	}
	if cards != nil {
		spec.Cards = cards
	} else if v := t.Get("cards"); !v.IsNil() {
		spec.Cards = []uint32{}
	}
	if spec.Refine == nil && spec.Cards != nil {
		zero := 0
		spec.Refine = &zero
	}
	if spec.Refine != nil && spec.Cards == nil {
		spec.Cards = []uint32{}
	}
	spec.Rentals = t.Get("rentals").Truthy()
	h, err := r.itemActionHost(what)
	if err != nil {
		return err
	}
	n, err := h.CountItemSpec(id, itemID, spec)
	if err != nil {
		return wrap(what, err)
	}
	c.Return(script.Int(int64(n)))
	return nil
}

// itemRef reads the item argument of the refine reads: an item ID, or an
// equipped copy, which the prelude passes as -handle.
func itemRef(what string, v script.Value) (ItemRef, error) {
	n, ok := v.AsInt()
	if !ok || n == 0 || n > 1<<32-1 {
		return ItemRef{}, errors.New(what + ": want an item from items.id or an equipped item")
	}
	if n < 0 {
		return ItemRef{Equipped: -n}, nil
	}
	return ItemRef{Item: uint32(n)}, nil // #nosec G115 -- checked above.
}

// itemsCall is the functions of wa.items of area B9: Arg(0) is the
// function, then its arguments.
func (r *Runtime) itemsCall(c *script.Call) error {
	op := c.String(0)
	what := "items." + op
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.itemActionHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "get":
		n, ok := c.Arg(1).AsInt()
		if !ok || n <= 0 || n > 1<<32-1 {
			return errors.New(what + ": want an item from items.id")
		}
		info, ok := h.ItemInfo(uint32(n)) // #nosec G115 -- checked above.
		if !ok {
			c.Return(script.Nil)
			return nil
		}
		c.Return(itemInfoTable(info))
		return nil
	case "can_refine":
		ref, err := itemRef(what, c.Arg(1))
		if err != nil {
			return err
		}
		ok, err := h.CanRefine(ref)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Bool(ok))
		return nil
	case "refine_chance", "refine_cost":
		ref, err := itemRef(what, c.Arg(1))
		if err != nil {
			return err
		}
		refine := -1
		if v := c.Arg(2); !v.IsNil() {
			n, ok := v.AsInt()
			if !ok || n < 0 || n > MaxRefineArg {
				return fmt.Errorf("%s: the refine is 0 to %d", what, MaxRefineArg)
			}
			refine = int(n)
		} else if ref.Equipped == 0 {
			return errors.New(what + ": an item ID needs the refine it has")
		}
		if op == "refine_chance" {
			n, err := h.RefineChance(ref, refine, c.Arg(3).Truthy())
			if err != nil {
				return wrap(what, err)
			}
			c.Return(script.Int(int64(n)))
			return nil
		}
		kind := "normal"
		if v := c.Arg(3); !v.IsNil() {
			kind, _ = v.AsString()
			if !slices.Contains(RefineKinds, kind) {
				return fmt.Errorf("%s: the kind is one of %q", what, RefineKinds)
			}
		}
		zeny, material, ok, err := h.RefineCost(ref, refine, kind)
		if err != nil {
			return wrap(what, err)
		}
		if !ok {
			c.Return(script.Nil)
			return nil
		}
		c.Return(script.Int(zeny), script.Int(material))
		return nil
	}
	return fmt.Errorf("items has no function %q", op)
}

func itemInfoTable(i ItemInfo) script.Value {
	f := []script.Field{
		script.KV("id", script.Int(int64(i.ID))), script.KV("name", script.String(i.Name)),
		script.KV("aegis_name", script.String(i.AegisName)), script.KV("type", script.String(i.Type)),
		script.KV("buy", script.Int(int64(i.Buy))), script.KV("sell", script.Int(int64(i.Sell))),
		script.KV("weight", script.Int(int64(i.Weight))), script.KV("attack", script.Int(int64(i.Attack))),
		script.KV("magic_attack", script.Int(int64(i.MagicAttack))), script.KV("defense", script.Int(int64(i.Defense))),
		script.KV("range", script.Int(int64(i.Range))), script.KV("slots", script.Int(int64(i.Slots))),
		script.KV("view", script.Int(int64(i.View))), script.KV("min_level", script.Int(int64(i.MinLevel))),
		script.KV("max_level", script.Int(int64(i.MaxLevel))), script.KV("subtype", script.Int(int64(i.Subtype))),
		script.KV("weapon_level", script.Int(int64(i.WeaponLevel))), script.KV("armor_level", script.Int(int64(i.ArmorLevel))),
		script.KV("locations", script.Int(int64(i.Locations))), script.KV("refineable", script.Bool(i.Refineable)),
		script.KV("stackable", script.Bool(i.Stackable)),
	}
	if i.Gender != "" {
		f = append(f, script.KV("gender", script.String(i.Gender)))
	}
	return script.NewTable(f...)
}

// equippedAction is equipped:refine and equipped:destroy: Arg(0) is the
// handle, Arg(1) the method, then its arguments.
func (r *Runtime) equippedAction(c *script.Call) error {
	id := c.Int(0)
	op := c.String(1)
	what := "equipped item:" + op
	if op == "refine" {
		what = "equipped item:add_refine"
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "player.give_items"); err != nil {
		return err
	}
	h, err := r.itemActionHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "refine":
		n, ok := c.Arg(2).AsInt()
		if !ok || n < -MaxRefineArg || n > MaxRefineArg || n == 0 {
			return fmt.Errorf("%s: the amount is -%d to %d, not 0", what, MaxRefineArg, MaxRefineArg)
		}
		v, err := h.EquippedRefine(id, int(n))
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Int(int64(v)))
		return nil
	case "destroy":
		failure := false
		err := walkOptions(what, c.Arg(2), []string{"refine_failure"}, func(_ string, v script.Value) error {
			b, ok := v.AsBool()
			if !ok {
				return fmt.Errorf("%s: refine_failure must be true or false", what)
			}
			failure = b
			return nil
		})
		if err != nil {
			return err
		}
		return wrap(what, h.EquippedDestroy(id, failure))
	}
	return fmt.Errorf("equipped item has no method %q", op)
}

// playerItemAction is the B9 methods of player handles: Arg(0) is the
// handle, Arg(1) the method, then its arguments.
func (r *Runtime) playerItemAction(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	op := c.String(1)
	what := "player:" + op
	if op == "get_broken_items" {
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
	} else if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.itemActionHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "get_broken_items":
		list, err := h.BrokenItems(id)
		if err != nil {
			return wrap(what, err)
		}
		out := make([]script.Value, len(list))
		for i, v := range list {
			out[i] = script.Int(v)
		}
		c.Return(script.List(out...))
		return nil
	case "repair_item":
		n, ok := c.Arg(2).AsInt()
		if !ok || n < 1 || n > 1<<16 {
			return errors.New(what + ": want the broken item's number from 1, as player:get_broken_items lists them")
		}
		done, err := h.RepairItem(id, int(n))
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Bool(done))
		return nil
	case "repair_all":
		n, err := h.RepairAll(id)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Int(int64(n)))
		return nil
	case "run_item_script":
		n, ok := c.Arg(2).AsInt()
		if !ok || n <= 0 || n > 1<<32-1 {
			return errors.New(what + ": want an item from items.id")
		}
		return wrap(what, h.RunItemScript(id, uint32(n))) // #nosec G115 -- checked above.
	case "merge_items":
		n, err := h.MergeItems(id)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Int(int64(n)))
		return nil
	}
	return fmt.Errorf("player has no method %q", op)
}

// mapsDropItem is maps.drop_item(map, x, y, item, amount, options).
func (r *Runtime) mapsDropItem(c *script.Call) error {
	const what = "maps.drop_item"
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "player.give_items"); err != nil {
		return err
	}
	m, ok := c.Arg(0).AsString()
	if !ok || m == "" || len(m) > 64 {
		return errors.New(what + ": want a map name, such as prontera")
	}
	x, y, err := checkCell(what, c.Arg(1), c.Arg(2))
	if err != nil {
		return err
	}
	itemID, ok := c.Arg(3).AsInt()
	amount, ok2 := c.Arg(4).AsInt()
	if !ok || itemID <= 0 || itemID > 1<<32-1 || !ok2 || amount < 1 || amount > 30000 {
		return errors.New(what + ": want an item ID from items.id and an amount from 1 to 30000")
	}
	spec, err := readItemSpec(what, c.Arg(5))
	if err != nil {
		return err
	}
	if spec.SignedBy != 0 {
		return errors.New(what + ": a dropped item cannot be signed")
	}
	h, err := r.itemActionHost(what)
	if err != nil {
		return err
	}
	return wrap(what, h.DropItem(m, x, y, uint32(itemID), int(amount), spec)) // #nosec G115 -- checked above.
}
