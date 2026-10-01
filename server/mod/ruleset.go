package mod

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Ruleset bases (strategy section 3.6). A base cannot be mixed.
const (
	Renewal    = "renewal"
	PreRenewal = "pre_renewal"
)

// The built-in ruleset mods, one per base. Each sets every flag to its
// base's value.
const (
	CoreRenewal    = "core.ruleset.renewal"
	CorePreRenewal = "core.ruleset.pre_renewal"
)

// flagValues lists each ruleset flag and its values, from strategy
// section 3.6. skill_rebalance.<skill> flags are checked by prefix.
var flagValues = map[string][]string{
	"cast_time":          {Renewal, PreRenewal},
	"skill_cooldown":     {"on", "off"},
	"aspd":               {Renewal, PreRenewal},
	"stat_point_cost":    {Renewal, PreRenewal},
	"level_penalty_exp":  {"on", "off"},
	"level_penalty_drop": {"on", "off"},
	"level_damage_mod":   {"on", "off"},
	"element_table":      {Renewal, PreRenewal},
	"size_table":         {Renewal, PreRenewal},
	"status_resist":      {Renewal, PreRenewal},
	"heal_formula":       {Renewal, PreRenewal},
	"boss_protection":    {Renewal, PreRenewal},
	"ensemble_songs":     {Renewal, PreRenewal},
	"homunculus":         {Renewal, PreRenewal},
	"busy_restrictions":  {"on", "off"},
	"fourth_job_traits":  {"on", "off"},
	"item_rolls":         {"on", "off"},
}

const skillRebalance = "skill_rebalance."

// coreFlags returns the flags of a core ruleset: every mode flag set to
// the base, and the on/off flags on for renewal. item_rolls is off for
// both, which matches rAthena (strategy section 5.4).
func coreFlags(base string) map[string]string {
	f := map[string]string{}
	for name, values := range flagValues {
		switch {
		case values[0] == Renewal:
			f[name] = base
		case name == "item_rolls":
			f[name] = "off"
		case base == Renewal:
			f[name] = "on"
		default:
			f[name] = "off"
		}
	}
	return f
}

// Ruleset is a world's rules: a base and every flag's value.
type Ruleset struct {
	// Mod is the ruleset mod that made it, such as core.ruleset.renewal.
	Mod  string
	Base string
	// Flags holds every flag's value. skill_rebalance flags are only
	// present when a ruleset mod set them.
	Flags map[string]string
}

// Flag returns a flag's value.
func (r *Ruleset) Flag(name string) string { return r.Flags[name] }

// String formats the ruleset for logs.
func (r *Ruleset) String() string {
	var changed []string
	core := coreFlags(r.Base)
	for _, k := range slices.Sorted(maps.Keys(r.Flags)) {
		if core[k] != r.Flags[k] {
			changed = append(changed, k+"="+r.Flags[k])
		}
	}
	s := r.Mod + " (base " + r.Base
	if len(changed) > 0 {
		s += ", " + strings.Join(changed, ", ")
	}
	return s + ")"
}

// Mode returns the rAthena mode whose data the ruleset's base uses:
// renewal or pre-renewal, as waserver's --mode and the world row name
// them.
func (r *Ruleset) Mode() string { return BaseMode(r.Base) }

// BaseMode turns a ruleset base into the rAthena mode name.
func BaseMode(base string) string {
	if base == PreRenewal {
		return "pre-renewal"
	}
	return base
}

// ModeBase turns an rAthena mode name into a ruleset base.
func ModeBase(mode string) (string, bool) {
	switch mode {
	case "renewal":
		return Renewal, true
	case "pre-renewal":
		return PreRenewal, true
	}
	return "", false
}

// checkFlag checks one flag name and value.
func checkFlag(name, value string) error {
	values, ok := flagValues[name]
	if !ok && strings.HasPrefix(name, skillRebalance) && len(name) > len(skillRebalance) {
		values, ok = []string{Renewal, PreRenewal}, true
	}
	if !ok {
		return fmt.Errorf("%q is not a ruleset flag", name)
	}
	if !slices.Contains(values, value) {
		return fmt.Errorf("%s: %q is not one of %s", name, value, strings.Join(values, ", "))
	}
	return nil
}

// checkRuleset checks the dependencies between flags. Only
// fourth_job_traits has a hard one today.
func checkRuleset(r *Ruleset) error {
	if r.Flags["fourth_job_traits"] == "on" && r.Base != Renewal {
		return fmt.Errorf("fourth_job_traits: on needs the renewal base")
	}
	return nil
}

// supports reports whether a mod supports a ruleset, and why not.
func (m *Manifest) supports(r *Ruleset) (bool, string) {
	s := m.Supports.Rulesets
	if s == nil {
		return true, ""
	}
	if len(s.Base) > 0 && !slices.Contains(s.Base, r.Base) {
		return false, fmt.Sprintf("it supports the bases %s, and the world's base is %s", strings.Join(s.Base, ", "), r.Base)
	}
	for _, name := range slices.Sorted(maps.Keys(s.Flags)) {
		want := s.Flags[name]
		got := r.Flags[name]
		if got == "" {
			got = "unset"
		}
		if !slices.Contains(want, got) {
			return false, fmt.Sprintf("it needs %s to be %s, and the world's ruleset has %s", name, strings.Join(want, " or "), got)
		}
	}
	return true, ""
}
