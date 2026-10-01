package bonus

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Num is a number argument. Computed marks a function that the caller
// keeps, such as a Lua function that bonuses.add got. Its Value is 0.
type Num struct {
	Value    int64
	Computed bool
}

// Target is the against or from option: one key and its value.
type Target struct {
	Key   string
	Value int64
}

// Options are the options table of bonuses.add (0002a, "Options table").
// Set lists the options that were given.
type Options struct {
	Percent    bool
	Skill      int64
	Item       int64
	ItemGroup  int64
	Kind       string
	Range      string
	Attack     string
	Element    int64
	WeaponType int64
	Against    *Target
	From       *Target
	Chance     Num
	// On is the trigger of a proc: attack, hit_taken or skill_used.
	On string
	// When is set when the bonus has a condition: a function the caller
	// keeps, which switches the bonus on and off. Every bonus takes it.
	When        bool
	Target      string
	Duration    int64
	RandomLevel bool
	Stacking    string
	Priority    int64
	Set         map[string]bool
}

// Has reports whether an option was given.
func (o *Options) Has(name string) bool { return o.Set[name] }

// Bonus is one bonuses.add call: the name, the numbers in order, and the
// options.
type Bonus struct {
	Name    string
	Args    []Num
	Options Options
}

// OptionNames are every option, in the order of 0002a's table.
var OptionNames = []string{"percent", "skill", "item", "item_group", "kind", "range", "attack", "element",
	"weapon_type", "against", "from", "chance", "on", "target", "duration", "random_level", "stacking", "priority", "when"}

// TargetKeys are the keys of against and from.
var TargetKeys = []string{"race", "element", "size", "class", "monster_group", "monster", "job"}

// targetKeys are the keys each name accepts in against or from. Names
// not listed accept none. damage and damage_taken are checked by their
// combinations.
var targetKeys = map[string][]string{
	"crit":              {"race"},
	"ignore_def":        {"race", "element", "class"},
	"ignore_mdef":       {"race", "element", "class", "monster_group"},
	"ignore_res":        {"race"},
	"ignore_mres":       {"race"},
	"def_as_damage":     {"race", "element", "class"},
	"set_def":           {"race"},
	"set_mdef":          {"race"},
	"coma":              {"race", "class", "element"},
	"block_healing":     {"race"},
	"deal_max_hp":       {"race"},
	"deal_max_sp":       {"race"},
	"hp_per_hit":        {"race", "class"},
	"sp_per_hit":        {"race", "class"},
	"sp_per_kill":       {"race"},
	"experience":        {"race", "class"},
	"drop_rate":         {"race", "class"},
	"extra_drop":        {"race", "class", "monster"},
	"damage":            TargetKeys,
	"damage_taken":      TargetKeys,
	"cause_status":      nil,
	"gain_zeny_on_kill": nil,
}

// Check checks a bonus against its name's spec. It returns the first
// problem, in words a mod author can act on.
func Check(b *Bonus) error {
	s, ok := Lookup(b.Name)
	if !ok {
		return fmt.Errorf("bonuses.add: there is no bonus %q", b.Name)
	}
	if err := checkArgs(s, b); err != nil {
		return fmt.Errorf("bonuses.add(%q): %w", b.Name, err)
	}
	if err := checkOptions(s, b); err != nil {
		return fmt.Errorf("bonuses.add(%q): %w", b.Name, err)
	}
	return nil
}

// HasAmount reports whether a bonus was given an amount.
func HasAmount(s *Spec, b *Bonus) bool {
	return s.Amount || s.OptionalAmount && len(b.Args) > len(s.Subject)
}

func checkArgs(s *Spec, b *Bonus) error {
	want := len(s.Subject) + len(s.Extra)
	if s.Amount {
		want++
	}
	got := len(b.Args)
	if s.OptionalAmount && got == want+1 {
		want++
	}
	if got != want {
		return fmt.Errorf("wants %s, got %d numbers", argWords(s), got)
	}
	for i, kind := range s.Subject {
		a := b.Args[i]
		if a.Computed {
			// A level may be computed, as rAthena's autospell levels
			// that depend on the refine (proposal 0002, decision 12).
			if kind != ArgLevel {
				return fmt.Errorf("argument %d must be a value, not a function", i+1)
			}
			continue
		}
		switch kind {
		case ArgSkill:
			if a.Value <= 0 {
				return fmt.Errorf("argument %d must be a skill from skills.id", i+1)
			}
		case ArgLevel:
			if a.Value < 1 {
				return fmt.Errorf("argument %d is a skill level, from 1", i+1)
			}
		case ArgStatus, ArgElement:
			if a.Value < 0 {
				return fmt.Errorf("argument %d must be a constant from wa.statuses or wa.elements", i+1)
			}
		}
	}
	for i := len(s.Subject) + 1; i < len(b.Args); i++ {
		if b.Args[i].Computed {
			return fmt.Errorf("argument %d must be a value, not a function: only the amount can be computed", i+1)
		}
	}
	return nil
}

func argWords(s *Spec) string {
	var parts []string
	for _, a := range s.Subject {
		parts = append(parts, map[Arg]string{ArgStatus: "a status", ArgSkill: "a skill", ArgLevel: "a level", ArgElement: "an element"}[a])
	}
	switch {
	case s.Amount:
		parts = append(parts, "an amount")
	case s.OptionalAmount:
		parts = append(parts, "an optional amount")
	}
	for _, e := range s.Extra {
		parts = append(parts, "the "+e)
	}
	if len(parts) == 0 {
		return "no numbers"
	}
	return strings.Join(parts, ", then ")
}

var enumValues = map[string][]string{
	"kind":     {"physical", "magic", "misc"},
	"range":    {"melee", "ranged"},
	"attack":   {"normal", "skill"},
	"on":       {"attack", "hit_taken", "skill_used"},
	"target":   {"self", "enemy"},
	"stacking": {"sum", "highest", "lowest", "last", "priority"},
}

func checkOptions(s *Spec, b *Bonus) error {
	o := &b.Options
	var names []string
	for n := range o.Set {
		names = append(names, n)
	}
	sort.Strings(names)
	amount := HasAmount(s, b)
	for _, n := range names {
		switch n {
		case "percent":
			if s.Percent != Both && s.Percent != PercentOnly || !amount {
				return fmt.Errorf("takes no percent option")
			}
		case "stacking":
			if !amount && !s.Proc && !s.OneValue {
				return fmt.Errorf("takes no stacking option: it has no amount or chance")
			}
		case "when":
			// A condition: every bonus takes one.
		case "priority":
			if o.Stacking != "priority" {
				return fmt.Errorf("priority needs stacking = \"priority\"")
			}
		default:
			if !slices.Contains(s.Options, n) {
				if !slices.Contains(OptionNames, n) {
					return fmt.Errorf("there is no option %q", n)
				}
				return fmt.Errorf("takes no %s option", n)
			}
		}
		if vals, ok := enumValues[n]; ok {
			v := map[string]string{"kind": o.Kind, "range": o.Range, "attack": o.Attack, "on": o.On, "target": o.Target, "stacking": o.Stacking}[n]
			if !slices.Contains(vals, v) {
				return fmt.Errorf("%s must be %s, not %q", n, quoteList(vals), v)
			}
		}
	}
	for _, n := range s.Required {
		if !o.Has(n) {
			return fmt.Errorf("needs the %s option", n)
		}
	}
	if s.Proc && !o.Has("chance") {
		return fmt.Errorf("needs the chance option: a whole number per 10,000")
	}
	if o.Has("stacking") {
		oneValue := o.Stacking == "last" || o.Stacking == "priority"
		if oneValue != s.OneValue {
			if s.OneValue {
				return fmt.Errorf("keeps one value: stacking must be \"last\" or \"priority\"")
			}
			return fmt.Errorf("stacking %q is only for the one-value bonuses", o.Stacking)
		}
	}
	if o.Has("range") && o.Kind != "physical" {
		return fmt.Errorf("range needs kind = \"physical\"")
	}
	if o.On == "skill_used" && !o.Has("skill") {
		return fmt.Errorf("on = \"skill_used\" needs the skill option")
	}
	if err := checkPercent(s, b, amount); err != nil {
		return err
	}
	for _, t := range []struct {
		name string
		t    *Target
	}{{"against", o.Against}, {"from", o.From}} {
		if t.t == nil {
			continue
		}
		if !slices.Contains(TargetKeys, t.t.Key) {
			return fmt.Errorf("%s has no key %q; it takes one of %s", t.name, t.t.Key, strings.Join(TargetKeys, ", "))
		}
		if keys := targetKeys[s.Name]; !slices.Contains(keys, t.t.Key) {
			if len(keys) == 0 {
				return fmt.Errorf("takes no %s option", t.name)
			}
			return fmt.Errorf("%s takes %s, not %s", t.name, strings.Join(keys, " or "), t.t.Key)
		}
	}
	switch s.Name {
	case "damage", "damage_taken":
		return checkDamage(s.Name, o)
	case "extra_drop":
		if o.Has("item") == o.Has("item_group") {
			return fmt.Errorf("needs exactly one of item and item_group")
		}
	}
	return nil
}

// checkPercent applies rule 4, and the names whose percent depends on
// the skill option.
func checkPercent(s *Spec, b *Bonus, amount bool) error {
	o := &b.Options
	switch s.Name {
	case "sp_cost":
		if !o.Has("skill") && !o.Percent {
			return fmt.Errorf("for all skills, the amount is a percent: add percent = true")
		}
		return nil
	case "after_cast_delay":
		if !o.Has("skill") && !o.Percent {
			return fmt.Errorf("for all skills, the amount is a percent: add percent = true")
		}
		if o.Has("skill") && o.Percent {
			return fmt.Errorf("with a skill, the amount is milliseconds: remove percent")
		}
		return nil
	}
	if s.Percent == PercentOnly && amount && !o.Percent {
		return fmt.Errorf("the amount is a percent: add percent = true")
	}
	return nil
}

func quoteList(vals []string) string {
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(q, ", ")
}

// checkDamage checks the options of damage and damage_taken against the
// combinations that have an rAthena sum (0002a, "Groups").
func checkDamage(name string, o *Options) error {
	if c := DamageGroup(name, o); c == "" {
		return fmt.Errorf("this combination of options has no rAthena sum; see 0002a, \"Damage and damage taken\"")
	}
	return nil
}

// DamageGroup returns the rAthena sum that a damage or damage_taken
// bonus adds to, or "" when its options have none. A damage bonus with
// an against key and no kind adds to the physical and the magic sum,
// returned as two names joined by "+".
func DamageGroup(name string, o *Options) string {
	flags := o.Has("range") || o.Has("attack")
	only := func(names ...string) bool {
		for n := range o.Set {
			if n == "stacking" || n == "priority" || n == "when" {
				continue
			}
			if !slices.Contains(names, n) {
				return false
			}
		}
		return true
	}
	if name == "damage" {
		switch {
		case o.Against != nil:
			if o.Against.Key == "element" && only("kind", "range", "attack", "against") && (flags || o.Kind == "misc") {
				return "bAddEle (bonus3)"
			}
			if !only("kind", "against") {
				return ""
			}
			phys := map[string]string{"race": "bAddRace", "class": "bAddClass", "size": "bAddSize", "element": "bAddEle",
				"monster_group": "bAddRace2", "monster": "bAddDamageClass", "job": "bAddDamageClass"}
			magic := map[string]string{"race": "bMagicAddRace", "class": "bMagicAddClass", "size": "bMagicAddSize", "element": "bMagicAddEle",
				"monster_group": "bMagicAddRace2", "monster": "bAddMagicDamageClass", "job": "bAddMagicDamageClass"}
			switch o.Kind {
			case "physical":
				return phys[o.Against.Key]
			case "magic":
				return magic[o.Against.Key]
			case "":
				return phys[o.Against.Key] + "+" + magic[o.Against.Key]
			}
		case o.Has("skill"):
			if only("skill") {
				return "bSkillAtk"
			}
		case o.Has("weapon_type"):
			if only("kind", "weapon_type") && o.Kind == "physical" {
				return "bWeaponDamageRate"
			}
		case o.Has("element"):
			if only("kind", "element") && o.Kind == "magic" {
				return "bMagicAtkEle"
			}
		case o.Has("range"):
			if only("kind", "range") {
				return map[string]string{"melee": "bShortAtkRate", "ranged": "bLongAtkRate"}[o.Range]
			}
		}
		return ""
	}
	switch {
	case o.From != nil:
		switch o.From.Key {
		case "race":
			if only("from") {
				return "bSubRace"
			}
			// The list with trigger flags: a range or an attack type
			// tells it from a plain kind, which has no sum.
			if only("kind", "range", "attack", "from") && flags {
				return "bSubRace (bonus3)"
			}
		case "class":
			if only("from") {
				return "bSubClass"
			}
		case "size":
			switch {
			case only("from"):
				return "bSubSize"
			case only("kind", "from") && o.Kind == "physical":
				return "bWeaponSubSize"
			case only("kind", "from") && o.Kind == "magic":
				return "bMagicSubSize"
			}
		case "element":
			if only("kind", "from") {
				return map[string]string{"physical": "bSubDefEle", "magic": "bMagicSubDefEle"}[o.Kind]
			}
		case "monster_group":
			if only("from") {
				return "bSubRace2"
			}
		case "monster", "job":
			if only("kind", "from") {
				return map[string]string{"physical": "bAddDefMonster", "magic": "bAddMDefMonster"}[o.Kind]
			}
		}
	case o.Has("skill"):
		if only("skill") {
			return "bSubSkill"
		}
	case o.Has("element"):
		if only("element") {
			return "bSubEle"
		}
		if only("kind", "range", "attack", "element") && flags {
			return "bSubEle (bonus3)"
		}
	case o.Has("kind"):
		switch {
		case only("kind", "range") && o.Kind == "physical" && o.Has("range"):
			return map[string]string{"melee": "bNearAtkDef", "ranged": "bLongAtkDef"}[o.Range]
		case only("kind") && o.Kind == "magic":
			return "bMagicAtkDef"
		case only("kind") && o.Kind == "misc":
			return "bMiscAtkDef"
		}
	}
	return ""
}
