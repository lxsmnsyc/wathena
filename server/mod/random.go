package mod

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
)

// Random options (proposal 0002, review decision 23): rAthena's
// item_randomopt_db.yml and item_randomopt_group.yml become core's
// data/random_options.yml and data/random_option_groups.yml. Mods add
// options and groups named with their ID, and change core's by name.

// RandomOptionKind is data/random_options.yml. A record's name is the
// option's readable name, such as max_hp.
var RandomOptionKind = Kind{Name: "random options", File: "data/random_options.yml", Fixed: []string{"id"}, CoreNames: true}

// OptionGroupKind is data/random_option_groups.yml. A record's name is the
// group's name, such as AS_WEAPON_1.
var OptionGroupKind = Kind{Name: "random option groups", File: "data/random_option_groups.yml", CoreNames: true}

// RandomOptions are the random options and groups with every mod's
// overlay applied.
type RandomOptions struct {
	Options map[string]*item.RandomOption
	Groups  map[string]*item.OptionGroup
	// OptionOverlay and GroupOverlay are the merges, with their notes.
	OptionOverlay, GroupOverlay *Overlay
}

// Option returns an option by name, or nil.
func (r *RandomOptions) Option(name string) *item.RandomOption {
	if r == nil {
		return nil
	}
	return r.Options[name]
}

// Group returns a group by name, or nil.
func (r *RandomOptions) Group(name string) *item.OptionGroup {
	if r == nil {
		return nil
	}
	return r.Groups[name]
}

// optionRolls are the roll names a random option's bonus reads.
var optionRolls = []string{"value", "param"}

// RandomOptions merges the random options and groups. names resolves the
// names in the options' bonuses.
func (s *Set) RandomOptions(names bonus.Names) (*RandomOptions, error) {
	none := func(string) bool { return false }
	oo, err := s.Overlay(RandomOptionKind, none)
	if err != nil {
		return nil, err
	}
	out := &RandomOptions{Options: map[string]*item.RandomOption{}, Groups: map[string]*item.OptionGroup{}, OptionOverlay: oo}
	var errs []error
	for _, r := range oo.Records {
		o := &item.RandomOption{Name: r.Name}
		if err := applyFields(o, r.Fields); err != nil {
			errs = append(errs, fmt.Errorf("%s: random options %s: %w", r.Where, r.Name, err))
			continue
		}
		o.Name = r.Name
		if len(o.DisplayName) > 255 {
			errs = append(errs, fmt.Errorf("%s: random options %s: display_name is at most 255 bytes", r.Where, r.Name))
		}
		if len(o.Bonus) == 0 {
			errs = append(errs, fmt.Errorf("%s: random options %s: bonus is a list of bonus entries, such as [[max_hp, {roll: value}]]", r.Where, r.Name))
		}
		for i, entry := range o.Bonus {
			b, err := bonus.FromData(entry, names)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: random options %s: bonus[%d]: %w", r.Where, r.Name, i, err))
				continue
			}
			for _, n := range b.Rolls() {
				if !slices.Contains(optionRolls, n) {
					errs = append(errs, fmt.Errorf("%s: random options %s: bonus[%d]: a random option reads { roll: value } or { roll: param }, not %q", r.Where, r.Name, i, n))
				}
			}
			o.Static = append(o.Static, b)
		}
		out.Options[r.Name] = o
	}
	go2, err := s.Overlay(OptionGroupKind, none)
	if err != nil {
		return nil, err
	}
	out.GroupOverlay = go2
	exists := func(n string) bool { return out.Options[n] != nil }
	for _, r := range go2.Records {
		g := &item.OptionGroup{Name: r.Name}
		if err := applyFields(g, r.Fields); err != nil {
			errs = append(errs, fmt.Errorf("%s: random option groups %s: %w", r.Where, r.Name, err))
			continue
		}
		g.Name = r.Name
		if err := g.Check(exists); err != nil {
			errs = append(errs, fmt.Errorf("%s: random option groups %s: %w", r.Where, r.Name, err))
			continue
		}
		out.Groups[r.Name] = g
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

// GroupNames are the groups' names, sorted.
func (r *RandomOptions) GroupNames() []string {
	if r == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(r.Groups))
}
