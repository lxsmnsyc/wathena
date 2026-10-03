package item

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/server/bonus"
)

// Random options: rAthena's item_randomopt_db.yml and
// item_randomopt_group.yml, as core's data/random_options.yml and
// data/random_option_groups.yml. A group gives a copy up to MaxOptions
// options, as s_random_opt_group::apply. They roll whatever the
// item_rolls flag says, as rAthena rolls them (proposal 0002, review
// decision 23).

// MaxOptions is MAX_ITEM_RDM_OPT, the most random options a copy has.
const MaxOptions = 5

// RandomOption is one option of data/random_options.yml.
type RandomOption struct {
	// Name is the option's readable name, such as max_hp. ID is
	// rAthena's option ID, which only core's options have.
	Name string `json:"name"`
	ID   int    `json:"id"`
	// DisplayName is what players see, such as "MaxHP".
	DisplayName string `json:"display_name"`
	// Bonus is the list of bonus entries the option gives, as static
	// bonuses, where { roll: value } is the option's value and
	// { roll: param } its param.
	Bonus []any `json:"bonus"`
	// Static are Bonus read and checked. The mod loader sets them.
	Static []*bonus.Bonus `json:"-"`
	// Unit is "%" or "", and Flag is set for an option without an
	// amount, such as an attack element. The mod loader sets them.
	Unit string `json:"-"`
	Flag bool   `json:"-"`
}

// OptionEntry is one option a group can give, with its range, param and
// chance per 10,000.
type OptionEntry struct {
	Option string `json:"option"`
	Min    int    `json:"min"`
	Max    int    `json:"max"`
	Param  int    `json:"param"`
	Chance int    `json:"chance"`
}

// OptionGroup is one group of data/random_option_groups.yml.
type OptionGroup struct {
	Name string `json:"name"`
	// Slots are the options each option index can get, from the first.
	// A copy always gets an option for each slot.
	Slots [][]OptionEntry `json:"slots"`
	// MaxRandom is how many indexes, from the first, may get an option of
	// Random when their slot gave none.
	MaxRandom int           `json:"max_random"`
	Random    []OptionEntry `json:"random"`
}

// Check checks a group's data. options finds an option by name.
func (g *OptionGroup) Check(options func(string) bool) error {
	if len(g.Slots) > MaxOptions {
		return fmt.Errorf("a group has at most %d slots", MaxOptions)
	}
	if g.MaxRandom < 0 || g.MaxRandom > MaxOptions {
		return fmt.Errorf("max_random is 0 to %d", MaxOptions)
	}
	if g.MaxRandom > 0 && len(g.Random) == 0 {
		return errors.New("max_random needs random options")
	}
	check := func(where string, e OptionEntry) error {
		if !options(e.Option) {
			return fmt.Errorf("%s: there is no random option %q", where, e.Option)
		}
		if e.Min > e.Max {
			return fmt.Errorf("%s: min %d is above max %d", where, e.Min, e.Max)
		}
		if e.Min < -maxRollValue || e.Max > maxRollValue || e.Param < -128 || e.Param > 127 {
			return fmt.Errorf("%s: values are %d to %d, and param -128 to 127", where, -maxRollValue, maxRollValue)
		}
		if e.Chance < 0 || e.Chance > 10000 {
			return fmt.Errorf("%s: chance is 0 to 10000", where)
		}
		return nil
	}
	for i, slot := range g.Slots {
		if len(slot) == 0 {
			return fmt.Errorf("slot %d has no options", i+1)
		}
		for j, e := range slot {
			if err := check(fmt.Sprintf("slot %d option %d", i+1, j+1), e); err != nil {
				return err
			}
		}
	}
	for j, e := range g.Random {
		if err := check(fmt.Sprintf("random option %d", j+1), e); err != nil {
			return err
		}
	}
	return nil
}

// Apply is s_random_opt_group::apply: the options a new copy gets.
//
//   - Each slot tries up to three times its number of options: it picks
//     one at random and keeps it when its chance passes. When none
//     passes, it keeps one picked at random without its chance.
//   - Then each index below MaxRandom that has no option picks one of
//     Random, kept when its chance passes.
//   - Options close up over the indexes without one, since the client
//     cannot show gaps.
//
// Values are drawn from each option's range, as rnd_value.
func (g *OptionGroup) Apply(r Rand) []Option {
	var opts [MaxOptions]Option
	var set [MaxOptions]bool
	pick := func(list []OptionEntry) *OptionEntry { return &list[r.IntN(len(list))] }
	chance := func(e *OptionEntry) bool { return 1+r.IntN(10000) <= e.Chance }
	apply := func(i int, e *OptionEntry) {
		opts[i] = Option{Name: e.Option, Value: e.Min + r.IntN(e.Max-e.Min+1), Param: e.Param}
		set[i] = true
	}
	for i := range min(len(g.Slots), len(set)) {
		slot := g.Slots[i]
		if len(slot) == 0 {
			break
		}
		for range len(slot) * 3 {
			if e := pick(slot); chance(e) {
				apply(i, e)
				break
			}
		}
		if !set[i] {
			apply(i, pick(slot))
		}
	}
	if len(g.Random) > 0 {
		for i := range min(g.MaxRandom, MaxOptions) {
			if set[i] {
				continue
			}
			if e := pick(g.Random); chance(e) {
				apply(i, e)
			}
		}
	}
	var out []Option
	for i := range opts {
		if set[i] {
			out = append(out, opts[i])
		}
	}
	return out
}
