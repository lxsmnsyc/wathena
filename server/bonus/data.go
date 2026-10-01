package bonus

import (
	"fmt"
	"math"
)

// Static bonuses are item data: each entry is the bonus name followed by
// its arguments, as in bonuses.add, with constants and records written
// by name. The Poring Card is
//
//	bonuses: [[luk, 2], [perfect_dodge, 1]]
//
// and an Abysmal Knight Card is
//
//	bonuses: [[damage, 25, {kind: physical, against: {class: boss}}]]
//
// It is the same shape as the entries of player:add_timed_bonuses
// (0002a, "bonus_script"), so one reader serves both.

// Names finds the values of the names that item data writes.
type Names interface {
	// Constant finds a constant of a module, such as races demi_human.
	Constant(module, name string) (int64, bool)
	Skill(name string) (int64, bool)
	Item(name string) (int64, bool)
	ItemGroup(name string) (int64, bool)
	Monster(name string) (int64, bool)
}

// targetModules are the constant modules of the keys of against and from.
var targetModules = map[string]string{
	"race": "races", "element": "elements", "size": "sizes", "class": "monster_classes",
	"monster_group": "monster_groups", "job": "jobs",
}

// FromData reads one data entry and checks it.
func FromData(entry any, names Names) (*Bonus, error) {
	list, ok := entry.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("a bonus is a list: the name, then its arguments")
	}
	name, ok := list[0].(string)
	if !ok {
		return nil, fmt.Errorf("a bonus starts with its name")
	}
	s, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("there is no bonus %q", name)
	}
	b := &Bonus{Name: name, Options: Options{Set: map[string]bool{}}}
	args := list[1:]
	if n := len(args); n > 0 {
		if m, ok := args[n-1].(map[string]any); ok {
			if err := readOptions(&b.Options, m, names); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			args = args[:n-1]
		}
	}
	for i, a := range args {
		var v int64
		var err error
		if i < len(s.Subject) {
			switch s.Subject[i] {
			case ArgStatus:
				v, err = readName(a, func(n string) (int64, bool) { return names.Constant("statuses", n) }, "status")
			case ArgElement:
				v, err = readName(a, func(n string) (int64, bool) { return names.Constant("elements", n) }, "element")
			case ArgSkill:
				v, err = readName(a, names.Skill, "skill")
			default:
				v, err = integer(a)
			}
		} else {
			v, err = integer(a)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: argument %d: %w", name, i+1, err)
		}
		b.Args = append(b.Args, Num{Value: v})
	}
	if err := Check(b); err != nil {
		return nil, err
	}
	return b, nil
}

func integer(v any) (int64, error) {
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case uint64:
		if n > math.MaxInt64 {
			return 0, fmt.Errorf("%d is too large", n)
		}
		return int64(n), nil
	case float64:
		if n == math.Trunc(n) && math.Abs(n) < 1<<53 {
			return int64(n), nil
		}
	}
	return 0, fmt.Errorf("want a whole number, got %v", v)
}

// readName reads a constant or record written by name. A number is taken
// as it is.
func readName(v any, find func(string) (int64, bool), what string) (int64, error) {
	if s, ok := v.(string); ok {
		n, ok := find(s)
		if !ok {
			return 0, fmt.Errorf("there is no %s %q", what, s)
		}
		return n, nil
	}
	return integer(v)
}

func readOptions(o *Options, m map[string]any, names Names) error {
	str := func(key string, v any) (string, error) {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("%s must be a word, such as \"physical\"", key)
		}
		return s, nil
	}
	for key, v := range m {
		o.Set[key] = true
		var err error
		switch key {
		case "percent", "random_level":
			b, ok := v.(bool)
			if !ok || !b {
				return fmt.Errorf("%s must be true", key)
			}
			if key == "percent" {
				o.Percent = true
			} else {
				o.RandomLevel = true
			}
		case "kind":
			o.Kind, err = str(key, v)
		case "range":
			o.Range, err = str(key, v)
		case "attack":
			o.Attack, err = str(key, v)
		case "on":
			o.On, err = str(key, v)
		case "when":
			return fmt.Errorf("when is a function, so it only works in Lua: a static bonus always applies")
		case "target":
			o.Target, err = str(key, v)
		case "stacking":
			o.Stacking, err = str(key, v)
		case "skill":
			o.Skill, err = readName(v, names.Skill, "skill")
		case "item":
			o.Item, err = readName(v, names.Item, "item")
		case "item_group":
			o.ItemGroup, err = readName(v, names.ItemGroup, "item group")
		case "element":
			o.Element, err = readName(v, func(n string) (int64, bool) { return names.Constant("elements", n) }, "element")
		case "weapon_type":
			o.WeaponType, err = readName(v, func(n string) (int64, bool) { return names.Constant("weapon_types", n) }, "weapon type")
		case "chance":
			var n int64
			n, err = integer(v)
			o.Chance = Num{Value: n}
		case "duration":
			o.Duration, err = integer(v)
		case "priority":
			o.Priority, err = integer(v)
		case "against", "from":
			var t *Target
			t, err = readTarget(key, v, names)
			if key == "against" {
				o.Against = t
			} else {
				o.From = t
			}
		default:
			// Check names the unknown option.
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func readTarget(key string, v any, names Names) (*Target, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return nil, fmt.Errorf("%s is a table with one key, such as {race: demi_human}", key)
	}
	for k, val := range m {
		var n int64
		var err error
		if k == "monster" {
			n, err = readName(val, names.Monster, "monster")
		} else if module, ok := targetModules[k]; ok {
			n, err = readName(val, func(s string) (int64, bool) { return names.Constant(module, s) }, k)
		} else {
			return &Target{Key: k}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		return &Target{Key: k, Value: n}, nil
	}
	return nil, nil
}

// Tables is Names over plain maps, by exact name.
type Tables struct {
	// Constants are the constant modules: module, then name.
	Constants                       map[string]map[string]int64
	Skills, Items, Groups, Monsters map[string]int64
}

// Constant finds a constant of a module.
func (t *Tables) Constant(module, name string) (int64, bool) {
	v, ok := t.Constants[module][name]
	return v, ok
}

// Skill finds a skill by its skill_db name.
func (t *Tables) Skill(name string) (int64, bool) { v, ok := t.Skills[name]; return v, ok }

// Item finds an item by AegisName.
func (t *Tables) Item(name string) (int64, bool) { v, ok := t.Items[name]; return v, ok }

// ItemGroup finds an item group by name.
func (t *Tables) ItemGroup(name string) (int64, bool) { v, ok := t.Groups[name]; return v, ok }

// Monster finds a monster by AegisName.
func (t *Tables) Monster(name string) (int64, bool) { v, ok := t.Monsters[name]; return v, ok }
