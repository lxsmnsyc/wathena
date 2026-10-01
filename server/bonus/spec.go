// Package bonus holds the item bonus names of proposal 0002a: what
// arguments and options each name takes, and the checks that
// bonuses.add runs. The Lua runtime, the static bonus data of items, and
// the script converter use the same table, so they accept the same
// bonuses.
package bonus

// Percent says how a name takes the percent option (0002a, rule 4).
type Percent int

const (
	// Flat names refuse { percent = true }.
	Flat Percent = iota
	// Both names take a flat amount, or a percent with percent = true.
	Both
	// PercentOnly names require percent = true.
	PercentOnly
	// Scaled names always scale a result, such as damage. Their amount
	// is a percent, and they take no percent option.
	Scaled
)

// Arg is the kind of an argument before the amount.
type Arg int

const (
	ArgStatus Arg = iota + 1
	ArgSkill
	ArgLevel
	ArgElement
)

// Spec is one bonus name.
type Spec struct {
	Name string
	// Subject are the arguments before the amount: what the bonus
	// produces or applies to.
	Subject []Arg
	// Amount is set when the name takes an amount. OptionalAmount
	// names work with or without one.
	Amount, OptionalAmount bool
	// Extra names the numbers after the amount, such as the interval of
	// hp_gain_over_time.
	Extra   []string
	Percent Percent
	// Options lists the options the name accepts, besides percent,
	// stacking and priority. Required lists those it needs.
	Options, Required []string
	// Proc names need the chance option.
	Proc bool
	// OneValue names keep one value: their stacking is "last" or
	// "priority".
	OneValue bool
}

// Flag reports whether a name takes no amount and no chance.
func (s *Spec) Flag() bool { return !s.Amount && !s.OptionalAmount && !s.Proc }

func stat(name string) *Spec { return &Spec{Name: name, Amount: true} }

func both(name string, opts ...string) *Spec {
	return &Spec{Name: name, Amount: true, Percent: Both, Options: opts}
}

func scaled(name string, opts ...string) *Spec {
	return &Spec{Name: name, Amount: true, Percent: Scaled, Options: opts}
}

func flag(name string) *Spec { return &Spec{Name: name} }

func proc(name string, opts ...string) *Spec {
	return &Spec{Name: name, Proc: true, Options: append([]string{"chance"}, opts...)}
}

// Specs lists the 127 bonus names and the skill bonus, in the order of
// 0002a's "Names" section.
var Specs = []*Spec{
	// Stats.
	stat("str"), stat("agi"), stat("vit"), stat("int"), stat("dex"), stat("luk"), stat("all_stats"),
	stat("pow"), stat("sta"), stat("wis"), stat("spl"), stat("con"), stat("crt"), stat("all_trait_stats"),
	both("patk"), both("smatk"), both("res"), both("mres"), both("hplus"), both("crate"),

	// HP, SP and AP.
	both("max_hp"), both("max_sp"), both("max_ap"),
	{Name: "natural_hp_recovery", Amount: true, Percent: PercentOnly},
	{Name: "natural_sp_recovery", Amount: true, Percent: PercentOnly},
	{Name: "hp_gain_over_time", Amount: true, Extra: []string{"interval"}, Percent: Both},
	{Name: "sp_gain_over_time", Amount: true, Extra: []string{"interval"}, Percent: Both},
	{Name: "hp_loss_over_time", Amount: true, Extra: []string{"interval"}},
	{Name: "sp_loss_over_time", Amount: true, Extra: []string{"interval"}},
	flag("no_hp_recovery"), flag("no_sp_recovery"),

	// ATK and DEF.
	both("atk", "weapon_type"), both("weapon_atk"), stat("refine_atk"), both("matk"), stat("hidden_matk"),
	{Name: "weapon_matk", Amount: true, Percent: PercentOnly},
	both("def"), both("status_def"), both("mdef"), both("status_mdef"), stat("attack_range"),

	// HIT, FLEE and CRIT.
	both("hit"), both("flee"), both("perfect_dodge"),
	both("crit", "kind", "range", "attack", "against"),
	scaled("crit_chance_taken"),
	stat("perfect_hit_chance"), stat("double_attack_chance"),

	// Speed, cast and delay.
	{Name: "move_speed", Amount: true, Percent: PercentOnly},
	both("aspd"),
	both("variable_cast_time", "skill"), both("fixed_cast_time", "skill"),
	both("after_cast_delay", "skill"),
	{Name: "cooldown", Amount: true, Options: []string{"skill"}, Required: []string{"skill"}},
	both("sp_cost", "skill"),
	flag("uninterruptible_cast"), flag("uninterruptible_cast_everywhere"), flag("always_endure"),

	// Damage.
	scaled("damage", "kind", "range", "attack", "element", "skill", "weapon_type", "against"),
	scaled("damage_taken", "kind", "range", "attack", "element", "skill", "from"),
	scaled("crit_damage"), scaled("non_crit_damage"), scaled("crit_damage_taken"), scaled("skill_ratio"),
	flag("ignore_size_penalty"),
	{Name: "immunity", Amount: true, Percent: Scaled, Options: []string{"kind"}, Required: []string{"kind"}},
	scaled("damage_taken_cap"),
	{Name: "damage_reflected", Amount: true, Percent: Scaled, Options: []string{"kind", "range"}, Required: []string{"kind", "range"}},
	scaled("reflected_damage_taken"),

	// Defense.
	{Name: "ignore_def", OptionalAmount: true, Percent: PercentOnly, Options: []string{"against"}, Required: []string{"against"}},
	{Name: "ignore_mdef", OptionalAmount: true, Percent: PercentOnly, Options: []string{"against"}, Required: []string{"against"}},
	{Name: "ignore_res", Amount: true, Percent: PercentOnly, Options: []string{"against"}, Required: []string{"against"}},
	{Name: "ignore_mres", Amount: true, Percent: PercentOnly, Options: []string{"against"}, Required: []string{"against"}},
	{Name: "def_as_damage", Options: []string{"against"}, Required: []string{"against"}},

	// Elements.
	{Name: "attack_element", Subject: []Arg{ArgElement}, OneValue: true},
	{Name: "defense_element", Subject: []Arg{ArgElement}, OneValue: true},

	// Procs.
	{Name: "cause_status", Subject: []Arg{ArgStatus}, Proc: true,
		Options: []string{"chance", "on", "target", "kind", "range", "attack", "skill", "duration"}},
	proc("coma", "attack", "against"),
	{Name: "block_healing", Proc: true, OneValue: true, Options: []string{"chance", "attack", "against", "duration"}},
	{Name: "cast", Subject: []Arg{ArgSkill, ArgLevel}, Proc: true,
		Options: []string{"chance", "on", "skill", "target", "random_level", "kind", "range", "attack"}},
	{Name: "drain_hp", Amount: true, Percent: Scaled, Proc: true, Options: []string{"chance"}},
	{Name: "drain_sp", Amount: true, Percent: Scaled, Proc: true, Options: []string{"chance"}},
	{Name: "remove_hp", Amount: true, Percent: Scaled, Proc: true, Options: []string{"chance", "kind", "range", "attack"}},
	{Name: "remove_sp", Amount: true, Percent: Scaled, Proc: true, Options: []string{"chance", "kind", "range", "attack"}},
	{Name: "deal_max_hp", Amount: true, Percent: Scaled, Proc: true, Options: []string{"chance", "kind", "against"}},
	{Name: "deal_max_sp", Amount: true, Percent: Scaled, Proc: true, Options: []string{"chance", "kind", "against"}},
	{Name: "set_def", Amount: true, Proc: true, OneValue: true, Options: []string{"chance", "against", "attack", "duration"}},
	{Name: "set_mdef", Amount: true, Proc: true, OneValue: true, Options: []string{"chance", "against", "attack", "duration"}},
	proc("break_weapon"), proc("break_armor"),
	{Name: "transform_monster", Proc: true, OneValue: true, Options: []string{"chance", "kind"}},
	proc("reflect_magic", "on"),
	{Name: "gain_zeny_on_kill", Amount: true, Proc: true, Options: []string{"chance"}},

	// Status.
	{Name: "status_resistance", Subject: []Arg{ArgStatus}, Amount: true},

	// Skills.
	{Name: "skill", Subject: []Arg{ArgSkill, ArgLevel}},
	{Name: "knockback", Amount: true, Options: []string{"skill"}, Required: []string{"skill"}},
	flag("no_gemstone_cost"), flag("no_magic_gear_fuel_cost"),

	// HP and SP from hits and kills.
	{Name: "hp_per_hit", Amount: true, Options: []string{"against"}},
	{Name: "sp_per_hit", Amount: true, Options: []string{"against"}},
	{Name: "hp_per_kill", Amount: true, Options: []string{"kind", "range"}, Required: []string{"kind"}},
	{Name: "sp_per_kill", Amount: true, Options: []string{"kind", "range", "against"}, Required: []string{"kind"}},

	// Drops and experience.
	scaled("experience", "from"), scaled("drop_rate", "from"),
	{Name: "extra_drop", Proc: true, Options: []string{"chance", "item", "item_group", "from"}},
	stat("steal_chance"),

	// Healing.
	scaled("healing_done", "skill"), scaled("healing_received", "skill"),
	scaled("item_hp_recovery", "item", "item_group"), scaled("item_sp_recovery", "item", "item_group"),
	flag("full_recovery_on_revive"),

	// Equipment.
	flag("unbreakable_weapon"), flag("unbreakable_armor"), flag("unbreakable_helm"), flag("unbreakable_shield"),
	flag("unbreakable_garment"), flag("unbreakable_shoes"), scaled("equipment_break_chance"),
	flag("unstrippable_weapon"), flag("unstrippable_armor"), flag("unstrippable_helm"), flag("unstrippable_shield"),

	// Other.
	stat("splash_range"), flag("no_knockback"), flag("see_hidden"), flag("hidden_from_detectors"), stat("max_weight"),
}

var byName = func() map[string]*Spec {
	m := make(map[string]*Spec, len(Specs))
	for _, s := range Specs {
		m[s.Name] = s
	}
	return m
}()

// Lookup returns a name's spec.
func Lookup(name string) (*Spec, bool) {
	s, ok := byName[name]
	return s, ok
}
