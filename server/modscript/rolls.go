package modscript

import (
	"errors"
	"fmt"
	"math"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Rolled item values (proposal 0002, review decision 23): the roll
// option of player:give_item, the item_roll hook, and the rolls,
// cards and random_options of equipped handles with set_roll and reroll.

// Roll modes of player:give_item.
const (
	RollDefault = "default"
	RollRandom  = "random"
	RollSet     = "set"
)

// GiveRoll is the roll option of player:give_item: default values,
// random ones, or the values a script set (Values), which the zone
// clamps to each roll's range.
type GiveRoll struct {
	Mode   string
	Values map[string]int
}

// RollHost is the part of the Host that rolled values need. The zone
// implements it.
type RollHost interface {
	// GiveRolledItem is GiveItem with the roll option.
	GiveRolledItem(player int64, item uint32, amount int, rentSeconds int64, roll GiveRoll) (bool, error)
	// EquippedSetRoll sets one roll of an equipped copy, clamped to its
	// range. An unknown name is an error.
	EquippedSetRoll(id int64, name string, value int) error
	// EquippedReroll rolls again one roll of an equipped copy, or every
	// roll when name is "".
	EquippedReroll(id int64, name string) error
}

// RollSource is where a rolled copy comes from: kind "drop" with the
// dead monster's handle, "script" or "shop".
type RollSource struct {
	Kind    string
	Monster int64
}

// ItemRoll is the object of the item_roll hook: one roll of a new copy.
// Value is the server's roll, which the innermost next returns.
type ItemRoll struct {
	Item                     uint32
	Name                     string
	Min, Max, Default, Value int
	Source                   RollSource
}

// RunItemRoll runs the item_roll hooks and returns the value they give.
// Without hooks, or when the chain fails, it is x.Value. The zone clamps
// the result to the roll's range.
func (r *Runtime) RunItemRoll(x ItemRoll) int {
	src := []script.Field{script.KV("kind", script.String(x.Source.Kind))}
	if x.Source.Monster != 0 {
		src = append(src, script.KV("monster", script.Int(x.Source.Monster)))
	}
	fields := []script.Field{
		script.KV("item", script.Int(int64(x.Item))), script.KV("name", script.String(x.Name)),
		script.KV("min", script.Int(int64(x.Min))), script.KV("max", script.Int(int64(x.Max))),
		script.KV("default", script.Int(int64(x.Default))), script.KV("value", script.Int(int64(x.Value))),
		script.KV("source", script.NewTable(src...)),
	}
	v, ok := r.runHookNested("item_roll", fields)
	if !ok {
		return x.Value
	}
	n, ok := v.AsInt()
	if !ok {
		return x.Value
	}
	return int(min(max(n, math.MinInt32), math.MaxInt32))
}

// rollHost returns the host's RollHost.
func (r *Runtime) rollHost(what string) (RollHost, error) {
	h, ok := r.host.(RollHost)
	if !ok {
		return nil, fmt.Errorf("%s: rolled values do not exist here", what)
	}
	return h, nil
}

// giveRoll reads the roll option of player:give_item. A table of values
// needs items.modify_rolls, except in a test case's own code (review
// decision 23, "Tests").
func (r *Runtime) giveRoll(v script.Value) (GiveRoll, error) {
	const what = "player:give_item"
	if v.IsNil() {
		return GiveRoll{Mode: RollDefault}, nil
	}
	if s, ok := v.AsString(); ok {
		if s != RollDefault && s != RollRandom {
			return GiveRoll{}, fmt.Errorf("%s: roll is \"random\", \"default\" or a table of values, not %q", what, s)
		}
		return GiveRoll{Mode: s}, nil
	}
	t, ok := v.AsTable()
	if !ok {
		return GiveRoll{}, errors.New(what + ": roll is \"random\", \"default\" or a table of values, such as { boss_damage = 31 }")
	}
	if !r.opts.Testing || r.current != nil {
		if err := r.need(what+" with a table of rolls", "items.modify_rolls"); err != nil {
			return GiveRoll{}, err
		}
	}
	out := GiveRoll{Mode: RollSet, Values: map[string]int{}}
	for _, f := range t.Fields {
		k, ok1 := f.Key.AsString()
		n, ok2 := f.Value.AsInt()
		if !ok1 || !ok2 {
			return GiveRoll{}, errors.New(what + ": a table of rolls maps names to integers, such as { boss_damage = 31 }")
		}
		out.Values[k] = int(min(max(n, math.MinInt32), math.MaxInt32))
	}
	return out, nil
}

// equippedSetRoll is equipped:set_roll(name, value).
func (r *Runtime) equippedSetRoll(c *script.Call) error {
	const what = "equipped:set_roll"
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "items.modify_rolls"); err != nil {
		return err
	}
	h, err := r.rollHost(what)
	if err != nil {
		return err
	}
	name, ok1 := c.Arg(1).AsString()
	v, ok2 := c.Arg(2).AsInt()
	if !ok1 || !ok2 {
		return errors.New(what + ": want a roll name and an integer")
	}
	if err := h.EquippedSetRoll(c.Int(0), name, int(min(max(v, math.MinInt32), math.MaxInt32))); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// equippedReroll is equipped:reroll(name?).
func (r *Runtime) equippedReroll(c *script.Call) error {
	const what = "equipped:reroll"
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "items.modify_rolls"); err != nil {
		return err
	}
	h, err := r.rollHost(what)
	if err != nil {
		return err
	}
	name := ""
	if c.NArgs() > 1 && !c.Arg(1).IsNil() {
		s, ok := c.Arg(1).AsString()
		if !ok {
			return errors.New(what + ": the roll name must be a string")
		}
		name = s
	}
	if err := h.EquippedReroll(c.Int(0), name); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}
