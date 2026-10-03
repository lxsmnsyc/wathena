package bonus

import (
	"strings"
)

// Describe names a bonus in words players read, for the lines of an
// item's description: "Physical damage against bosses" and the unit of
// its amount, "%" or "". constName gives the name of a constant of a
// module, such as races and 7 for demi_human, or "" when it has none.
//
// Labels follow the bonus name, the kind and range options, the element,
// and the against or from target. Names without a label of their own
// show as words, such as "Splash range".
func Describe(b *Bonus, constName func(module string, v int64) string) (label, unit string) {
	s, ok := Lookup(b.Name)
	if !ok {
		return words(b.Name), ""
	}
	o := &b.Options
	name := labels[b.Name]
	if name == "" {
		name = words(b.Name)
	}
	if len(s.Subject) > 0 && len(b.Args) > 0 {
		switch s.Subject[0] {
		case ArgElement:
			if n := constName("elements", b.Args[0].Value); n != "" {
				name += ": " + title(n)
			}
		case ArgStatus:
			if n := constName("statuses", b.Args[0].Value); n != "" {
				name += ": " + title(n)
			}
		}
	}
	var pre []string
	if o.Range != "" {
		pre = append(pre, title(o.Range))
	}
	if o.Has("element") {
		if n := constName("elements", o.Element); n != "" {
			pre = append(pre, title(n))
		}
	}
	if o.Kind != "" && (b.Name == "damage" || b.Name == "damage_taken" || b.Name == "crit") {
		pre = append(pre, o.Kind)
	}
	if len(pre) > 0 {
		name = strings.Join(pre, " ") + " " + lowerFirst(name)
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	if t := o.Against; t != nil {
		name += " against " + target(t, constName)
	}
	if t := o.From; t != nil {
		name += " from " + target(t, constName)
	}
	if s.Percent == Scaled || s.Percent == PercentOnly || s.Percent == Both && o.Percent {
		unit = "%"
	}
	return name, unit
}

// labels are the names players know for bonuses whose words differ from
// their name.
var labels = map[string]string{
	"str": "STR", "agi": "AGI", "vit": "VIT", "int": "INT", "dex": "DEX", "luk": "LUK",
	"pow": "POW", "sta": "STA", "wis": "WIS", "spl": "SPL", "con": "CON", "crt": "CRT",
	"patk": "P.ATK", "smatk": "S.MATK", "res": "RES", "mres": "MRES", "hplus": "H.PLUS", "crate": "C.RATE",
	"all_stats": "All stats", "all_trait_stats": "All trait stats",
	"max_hp": "MaxHP", "max_sp": "MaxSP", "max_ap": "MaxAP",
	"natural_hp_recovery": "HP recovery", "natural_sp_recovery": "SP recovery",
	"atk": "ATK", "weapon_atk": "Weapon ATK", "matk": "MATK", "weapon_matk": "Weapon MATK",
	"def": "DEF", "status_def": "Soft DEF", "mdef": "MDEF", "status_mdef": "Soft MDEF",
	"hit": "HIT", "flee": "FLEE", "perfect_dodge": "Perfect Dodge", "crit": "CRIT", "aspd": "ASPD",
	"variable_cast_time": "Variable cast time", "fixed_cast_time": "Fixed cast time",
	"after_cast_delay": "After cast delay", "sp_cost": "SP cost",
	"damage": "Damage", "damage_taken": "Damage taken", "crit_damage": "Critical damage",
	"ignore_def": "DEF ignored", "ignore_mdef": "MDEF ignored", "ignore_res": "RES ignored", "ignore_mres": "MRES ignored",
	"attack_element": "Attack element", "defense_element": "Armor element",
	"ignore_size_penalty": "No size penalty", "healing_done": "Healing done", "healing_received": "Healing received",
	"experience": "Experience", "drop_rate": "Drop rate",
	"unbreakable_weapon": "Indestructible weapon", "unbreakable_armor": "Indestructible armor",
	"move_speed": "Movement speed",
}

// target names the against or from option, such as "bosses" or "Angel".
func target(t *Target, constName func(module string, v int64) string) string {
	if t.Key == "class" {
		switch constName("monster_classes", t.Value) {
		case "boss":
			return "bosses"
		case "normal":
			return "normal monsters"
		case "all":
			return "all monsters"
		}
	}
	if module, ok := targetModules[t.Key]; ok {
		if n := constName(module, t.Value); n != "" {
			if n == "all" {
				return "all " + map[string]string{"race": "races", "element": "elements", "size": "sizes"}[t.Key]
			}
			out := title(n)
			switch t.Key {
			case "element":
				out += " element"
			case "size":
				out += " size"
			}
			return out
		}
	}
	return words(t.Key)
}

// words turns a name into words: splash_range is "Splash range".
func words(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// title capitalizes each word of a constant name: demi_human is
// "Demi Human".
func title(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func lowerFirst(s string) string {
	if s == "" || len(s) > 1 && strings.ToUpper(s[:2]) == s[:2] {
		// Abbreviations such as ATK keep their case.
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
