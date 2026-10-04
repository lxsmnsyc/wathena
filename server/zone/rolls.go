package zone

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Rolled item values (strategy section 5.4, proposal 0002 review
// decision 23):
//   - A new copy of an item with rolls gets a value for each roll. Drops
//     and scripts that ask for "random" roll them from the zone's secret
//     random stream, when the item_rolls flag is on. Otherwise each roll
//     is its default, as rAthena.
//   - The item_roll hooks get each value and may change it. The result is
//     clamped to the roll's range.
//   - Drops with a random option group roll random options, as rAthena's
//     mob_setdropitem_option, whatever the flag says.
//   - A copy with hidden rolls that was rolled starts unidentified. The
//     client gets its hidden values only once it is identified.

// rollSource is where a new copy comes from, for the item_roll hook.
type rollSource struct {
	kind    string
	monster *entity
}

// Sources of rolled copies.
const (
	sourceDrop   = "drop"
	sourceScript = "script"
	sourceShop   = "shop"
)

// newRolls gives a new copy of an item its rolls: random ones when random
// is set and the item_rolls flag is on, defaults otherwise, each through
// the item_roll hooks.
func (z *Zone) newRolls(d *item.Def, random bool, src rollSource) item.Rolls {
	if len(d.Rolls) == 0 {
		return nil
	}
	random = random && z.Mods != nil && z.Mods.ItemRolls
	var hooks *modscript.Runtime
	if r := z.scripts; r != nil && r.HasHooks("item_roll") {
		hooks = r
	}
	m := make(map[string]int, len(d.Rolls))
	// Names in order, so the random stream is drawn the same way each
	// time.
	for _, name := range d.RollNames() {
		def := d.Rolls[name]
		v := def.Default
		if random {
			v = def.Random(z.rand)
		}
		if hooks != nil {
			lo, hi := def.Bounds()
			x := modscript.ItemRoll{Item: d.ID, Name: name, Min: lo, Max: hi, Default: def.Default, Value: v,
				Source: modscript.RollSource{Kind: src.kind}}
			if src.monster != nil && src.monster.monster != nil {
				x.Source.Monster = z.monsterHandle(src.monster)
			}
			v = hooks.RunItemRoll(x)
		}
		m[name] = def.Clamp(v)
	}
	return item.RollsOf(m)
}

// newExtra is the extra of a new copy: its rolls, and the random options
// of group when it is not "". A rolled copy with hidden rolls starts
// unidentified.
func (z *Zone) newExtra(d *item.Def, random bool, src rollSource, group string) item.Extra {
	x := item.Extra{Rolls: z.newRolls(d, random, src)}
	x.Unidentified = random && d.HasHidden()
	if group != "" && z.Mods != nil {
		if g := z.Mods.RandomOptions.Group(group); g != nil {
			x.Options = g.Apply(z.rand)
		}
	}
	return x
}

// fixExtra checks a loaded copy's rolls and options against the item
// data, as strategy section 5.4 asks: values outside a roll's range are
// clamped, and every change is logged. Options the world no longer has
// are dropped.
func (z *Zone) fixExtra(c string, slot int, d *item.Def, x *item.Extra, cards []uint32) {
	var notes []string
	var n []string
	x.Rolls, n = d.FixRolls(x.Rolls)
	notes = append(notes, n...)
	for i := range x.CardRolls {
		if i >= len(cards) {
			notes = append(notes, "card rolls past the last card dropped")
			x.CardRolls = x.CardRolls[:i]
			break
		}
		cd := z.Items.Get(cards[i])
		x.CardRolls[i], n = cd.FixRolls(x.CardRolls[i])
		for _, s := range n {
			notes = append(notes, fmt.Sprintf("card %d: %s", cards[i], s))
		}
	}
	if len(x.Options) > 0 && z.Mods != nil {
		kept := x.Options[:0]
		for _, o := range x.Options {
			if z.Mods.RandomOptions.Option(o.Name) == nil {
				notes = append(notes, fmt.Sprintf("random option %s dropped: the world has no such option", o.Name))
				continue
			}
			kept = append(kept, o)
		}
		x.Options = kept
		if len(x.Options) == 0 {
			x.Options = nil
		}
	}
	if len(notes) > 0 && z.Mods != nil && z.Mods.Log != nil {
		z.Mods.Log.Warn("item rolls fixed on load", "character", c, "slot", slot, "item", d.AegisName, "changes", notes)
	}
}

// addCopy is pc_additem for a copy with a refine, cards and an extra.
// A copy whose item has rolls but which has none gets the defaults. The
// copy comes from o, for the logs. A new stack or instance gets its own
// serial. Items picked up from the ground get a new one too, since
// ground items are not saved: the log line names the serial of the
// stack the item was dropped from.
func (z *Zone) addCopy(p *Player, id uint32, amount, refine int, cards []uint32, x item.Extra, o origin) item.AddResult {
	d := z.Items.Get(id)
	if len(x.Rolls) == 0 && len(d.Rolls) > 0 {
		x.Rolls = d.Defaults()
	}
	tx := transfer.New(o.action).Create(o.from, item.Slot{ID: id, Refine: refine, Cards: cards, Extra: x}, amount, z.box(p), o.source)
	var e *transfer.Error
	if err := z.apply(p, tx, 0); errors.As(err, &e) {
		return e.AddResult()
	}
	return item.AddSuccess
}

// rollMessages are a slot's rolls, card rolls and options as the client
// shows them. Hidden rolls of an unidentified copy go without their
// value.
func (z *Zone) rollMessages(d *item.Def, s *item.Slot) ([]*wathenav1.ItemRoll, []*wathenav1.CardRolls, []*wathenav1.ItemOption) {
	rolls := rollMessages(d, s.Rolls, s.Unidentified)
	var cards []*wathenav1.CardRolls
	if len(s.CardRolls) > 0 {
		for i, id := range s.Cards {
			cards = append(cards, wathenav1.CardRolls_builder{Rolls: rollMessages(z.Items.Get(id), s.CardRoll(i), false)}.Build())
		}
	}
	var opts []*wathenav1.ItemOption
	for _, o := range s.Options {
		m := wathenav1.ItemOption_builder{Name: o.Name, Label: o.Name, Value: int32(o.Value)} // #nosec G115 -- option values fit in int32.
		if z.Mods != nil {
			if od := z.Mods.RandomOptions.Option(o.Name); od != nil {
				m.Label, m.Unit, m.Flag = od.DisplayName, od.Unit, od.Flag
			}
		}
		opts = append(opts, m.Build())
	}
	return rolls, cards, opts
}

func rollMessages(d *item.Def, r item.Rolls, unidentified bool) []*wathenav1.ItemRoll {
	var out []*wathenav1.ItemRoll
	for _, x := range r {
		def := d.Rolls[x.Name]
		if def == nil {
			continue
		}
		lo, hi := def.Bounds()
		m := wathenav1.ItemRoll_builder{Name: x.Name, Label: def.Label, Unit: def.Unit,
			Min: int32(lo), Max: int32(hi), Known: true, Value: int32(x.Value)} // #nosec G115 -- roll values fit in int32 (item.RollDef.Check).
		if def.Hidden && unidentified {
			m.Known, m.Value = false, 0
		}
		out = append(out, m.Build())
	}
	return out
}

// sendRolls tells a player a slot's rolls changed.
func (z *Zone) sendRolls(p *Player, slot int) {
	s := &p.inventory.Slots[slot]
	rolls, cards, opts := z.rollMessages(z.Items.Get(s.ID), s)
	p.pending = append(p.pending, wathenav1.Event_builder{ItemRollsChange: wathenav1.ItemRollsChange_builder{
		Slot: uint32(slot), Rolls: rolls, CardRolls: cards, Options: opts, // #nosec G115 -- below MaxInventory.
	}.Build()}.Build())
}

// GiveRolledItem is player:give_item with the roll option.
func (h scriptHost) GiveRolledItem(id int64, itemID uint32, amount int, rentSeconds int64, roll modscript.GiveRoll) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	ok, err := h.z.giveRolled(p, itemID, amount, roll, item.BoundNone)
	if err != nil {
		return false, fmt.Errorf("player:give_item: %w", err)
	}
	return ok, nil
}

// giveRolled gives items with the roll option of player:give_item, bound
// as bound. It returns false when they do not fit.
func (z *Zone) giveRolled(p *Player, itemID uint32, amount int, roll modscript.GiveRoll, bound item.Bound) (bool, error) {
	d := z.Items.Get(itemID)
	if roll.Mode == modscript.RollSet {
		for name := range roll.Values {
			if d.Rolls[name] == nil {
				return false, fmt.Errorf("item %s has no roll %q", d.AegisName, name)
			}
		}
	}
	step := amount
	if !d.Stackable {
		step = 1
	}
	for given := 0; given < amount; given += step {
		var x item.Extra
		switch roll.Mode {
		case modscript.RollSet:
			x.Rolls = z.newRolls(d, false, rollSource{kind: sourceScript})
			for i := range x.Rolls {
				if v, ok := roll.Values[x.Rolls[i].Name]; ok {
					x.Rolls[i].Value = d.Rolls[x.Rolls[i].Name].Clamp(v)
				}
			}
		default:
			x = z.newExtra(d, roll.Mode == modscript.RollRandom, rollSource{kind: sourceScript}, "")
		}
		x.Bound = bound
		if r := z.addCopy(p, itemID, step, 0, nil, x, scriptOrigin); r != item.AddSuccess {
			p.pending = append(p.pending, pickUpFailed(0, pickUpFailure(r)))
			return false, nil
		}
	}
	return true, nil
}

// equippedSlot finds the slot and the card of an equipped handle. card is
// -1 for the item itself.
func (z *Zone) equippedSlot(id int64) (*Player, int, int, error) {
	ref, ok := z.equippedRefs[id]
	if !ok || ref.p.gone() {
		return nil, 0, 0, errNotEquipped
	}
	return ref.p, ref.slot, ref.card, nil
}

// rollsOf returns the rolls of an equipped copy's item or card, and the
// item data they belong to.
func (z *Zone) rollsOf(p *Player, slot, card int) (*item.Def, *item.Rolls) {
	s := &p.inventory.Slots[slot]
	if card < 0 {
		return z.Items.Get(s.ID), &s.Rolls
	}
	for len(s.CardRolls) <= card {
		s.CardRolls = append(s.CardRolls, nil)
	}
	return z.Items.Get(s.Cards[card]), &s.CardRolls[card]
}

// EquippedSetRoll is equipped:set_roll.
func (h scriptHost) EquippedSetRoll(id int64, name string, value int) error {
	z := h.z
	p, slot, card, err := z.equippedSlot(id)
	if err != nil {
		return err
	}
	d, rolls := z.rollsOf(p, slot, card)
	def := d.Rolls[name]
	if def == nil {
		return fmt.Errorf("item %s has no roll %q", d.AegisName, name)
	}
	if len(*rolls) == 0 {
		*rolls = d.Defaults()
	}
	*rolls = rolls.With(name, def.Clamp(value))
	z.rollsChanged(p, slot)
	return nil
}

// EquippedReroll is equipped:reroll. A reroll draws as a random give
// does, with the source "script".
func (h scriptHost) EquippedReroll(id int64, name string) error {
	z := h.z
	p, slot, card, err := z.equippedSlot(id)
	if err != nil {
		return err
	}
	d, rolls := z.rollsOf(p, slot, card)
	if name != "" && d.Rolls[name] == nil {
		return fmt.Errorf("item %s has no roll %q", d.AegisName, name)
	}
	if len(d.Rolls) == 0 {
		return fmt.Errorf("item %s has no rolls", d.AegisName)
	}
	fresh := z.newRolls(d, true, rollSource{kind: sourceScript})
	if name == "" {
		*rolls = fresh
	} else {
		if len(*rolls) == 0 {
			*rolls = d.Defaults()
		}
		v, _ := fresh.Get(name)
		*rolls = rolls.With(name, v)
	}
	z.rollsChanged(p, slot)
	return nil
}

// rollsChanged sends a slot's new rolls and, once the script's call
// returns, starts its equip contexts again, so their scripts read the
// new values. The handles stay the same.
func (z *Zone) rollsChanged(p *Player, slot int) {
	z.sendRolls(p, slot)
	z.markStats(p)
	z.later(func() {
		if p.gone() || p.worn[slot] == nil {
			return
		}
		z.restartSlot(p, slot)
		z.statsChanged(p)
	})
}

// equippedRolls is the rolls field of an equipped handle: the rolls of
// the item, or of the card for a card's handle.
func (z *Zone) equippedRolls(p *Player, slot, card int) script.Value {
	s := &p.inventory.Slots[slot]
	r := s.Rolls
	d := z.Items.Get(s.ID)
	if card >= 0 {
		r, d = s.CardRoll(card), z.Items.Get(s.Cards[card])
	}
	if len(r) == 0 {
		r = d.Defaults()
	}
	fields := make([]script.Field, 0, len(r))
	for _, x := range r {
		fields = append(fields, script.KV(x.Name, script.Int(int64(x.Value))))
	}
	return script.NewTable(fields...)
}

// equippedCards is the cards field: { item, rolls } for each card.
func (z *Zone) equippedCards(p *Player, slot int) script.Value {
	s := &p.inventory.Slots[slot]
	var list []script.Value
	for i, id := range s.Cards {
		list = append(list, script.NewTable(script.KV("item", script.Int(int64(id))),
			script.KV("rolls", z.equippedRolls(p, slot, i))))
	}
	return script.List(list...)
}

// equippedOptions is the random_options field.
func equippedOptions(s *item.Slot) script.Value {
	var list []script.Value
	for _, o := range s.Options {
		list = append(list, script.NewTable(script.KV("option", script.String(o.Name)),
			script.KV("value", script.Int(int64(o.Value))), script.KV("param", script.Int(int64(o.Param)))))
	}
	return script.List(list...)
}

var _ modscript.RollHost = scriptHost{}
