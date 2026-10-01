package convert

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/server/bonus"
)

// toBonus builds the bonus.Bonus of a call, with computed values marked,
// so bonus.Check validates the converter's output with the runtime's
// rules.
func (c *itemConv) toBonus(w *wcall) (*bonus.Bonus, error) {
	b := &bonus.Bonus{Name: w.name, Options: bonus.Options{Set: map[string]bool{}}}
	for _, a := range w.args {
		n, err := c.number(a.kind, a.v)
		if err != nil {
			return nil, err
		}
		b.Args = append(b.Args, n)
	}
	o := &b.Options
	for _, op := range w.opts {
		o.Set[op.key] = true
		switch {
		case op.flag:
			switch op.key {
			case "percent":
				o.Percent = true
			case "random_level":
				o.RandomLevel = true
			}
		case op.enum != "":
			switch op.key {
			case "kind":
				o.Kind = op.enum
			case "range":
				o.Range = op.enum
			case "attack":
				o.Attack = op.enum
			case "when":
				o.When = op.enum
			case "target":
				o.Target = op.enum
			case "stacking":
				o.Stacking = op.enum
			}
		case op.tkey != "":
			n, err := c.number(op.kind, op.v)
			if err != nil {
				return nil, err
			}
			if n.Computed {
				return nil, unsupported("computed "+op.key, "")
			}
			t := &bonus.Target{Key: op.tkey, Value: n.Value}
			if op.key == "against" {
				o.Against = t
			} else {
				o.From = t
			}
		default:
			n, err := c.number(op.kind, op.v)
			if err != nil {
				return nil, err
			}
			if n.Computed && op.key != "chance" {
				return nil, unsupported("computed "+op.key, "")
			}
			switch op.key {
			case "chance":
				o.Chance = n
			case "skill":
				o.Skill = n.Value
			case "item":
				o.Item = n.Value
			case "item_group":
				o.ItemGroup = n.Value
			case "element":
				o.Element = n.Value
			case "weapon_type":
				o.WeaponType = n.Value
			case "duration":
				o.Duration = n.Value
			}
		}
	}
	if err := bonus.Check(b); err != nil {
		return nil, unsupported("invalid bonus", "%v", err)
	}
	return b, nil
}

// number is the value of an argument for checking: static values with
// their number, game values as computed.
func (c *itemConv) number(k argKind, v Expr) (bonus.Num, error) {
	if n, ok := static(v); ok {
		return bonus.Num{Value: n}, nil
	}
	if s, ok := staticStr(v); ok {
		switch k {
		case kSkill:
			id, ok := c.conv.consts.Skill(s)
			if !ok {
				return bonus.Num{}, unsupported("unknown skill", "%s", s)
			}
			return bonus.Num{Value: id}, nil
		case kItem:
			id, ok := c.conv.itemID(s)
			if !ok {
				return bonus.Num{}, unsupported("unknown item", "%s", s)
			}
			return bonus.Num{Value: int64(id)}, nil
		}
		return bonus.Num{}, unsupported("string argument", "%q", s)
	}
	if k != kNum && k != kLevel {
		return bonus.Num{}, unsupported("computed argument", "a %s", kindWord(k))
	}
	return bonus.Num{Computed: true}, nil
}

func kindWord(k argKind) string {
	return map[argKind]string{kNum: "number", kRace: "race", kElement: "element", kSize: "size", kClass: "class",
		kMonsterGroup: "monster group", kWeaponType: "weapon type", kStatus: "status", kSkill: "skill", kLevel: "level",
		kItem: "item", kItemGroup: "item group", kMonster: "monster"}[k]
}

// ref writes a static value of a kind as Lua: a constant from a module, a
// record local, or a number.
func (c *itemConv) ref(k argKind, v Expr) (string, error) {
	switch k {
	case kNum, kLevel:
		n, ok := static(v)
		if !ok {
			return "", unsupported("computed "+kindWord(k), "")
		}
		return strconv.FormatInt(n, 10), nil
	case kSkill:
		return c.skillArg(v)
	case kItem:
		name, err := c.itemName(v)
		if err != nil {
			return "", err
		}
		return c.file.record("items.id", name), nil
	case kItemGroup:
		name, err := c.groupName(v)
		if err != nil {
			return "", err
		}
		return c.file.record("items.group", name), nil
	case kMonster:
		name, err := c.monsterName(v)
		if err != nil {
			return "", err
		}
		return c.file.record("monsters.id", name), nil
	}
	module := kindModule[k]
	name, err := c.constName(k, v)
	if err != nil {
		return "", err
	}
	return c.file.need(module) + "." + name, nil
}

// dataRef writes a static value of a kind for item data: numbers as they
// are, and constants and records by name.
func (c *itemConv) dataRef(k argKind, v Expr) (string, error) {
	switch k {
	case kNum, kLevel:
		n, ok := static(v)
		if !ok {
			return "", unsupported("computed "+kindWord(k), "")
		}
		return strconv.FormatInt(n, 10), nil
	case kSkill:
		switch a := v.(type) {
		case *Str:
			return yamlString(c.conv.skillName(a.Value)), nil
		case *Num:
			name, ok := c.conv.consts.SkillName(a.Value)
			if !ok {
				return "", unsupported("unknown skill", "%d", a.Value)
			}
			return yamlString(name), nil
		case *Const:
			name, ok := c.conv.consts.SkillName(a.Value)
			if !ok {
				return "", unsupported("unknown skill", "%d", a.Value)
			}
			return yamlString(name), nil
		}
		return "", unsupported("skill argument", "computed")
	case kItem:
		name, err := c.itemName(v)
		return yamlString(name), err
	case kItemGroup:
		name, err := c.groupName(v)
		return yamlString(name), err
	case kMonster:
		name, err := c.monsterName(v)
		return yamlString(name), err
	}
	name, err := c.constName(k, v)
	return yamlString(name), err
}

func (c *itemConv) constName(k argKind, v Expr) (string, error) {
	n, ok := static(v)
	if !ok {
		return "", unsupported("computed "+kindWord(k), "")
	}
	module := c.conv.modules[kindModule[k]]
	name, ok := module.Names[n]
	if !ok {
		return "", unsupported("unknown "+kindWord(k), "%d", n)
	}
	return name, nil
}

func (c *itemConv) itemName(v Expr) (string, error) {
	switch a := v.(type) {
	case *Str:
		if name, ok := c.conv.itemByName[strings.ToLower(a.Value)]; ok {
			return name, nil
		}
		return "", unsupported("unknown item", "%s", a.Value)
	}
	n, ok := static(v)
	if !ok {
		return "", unsupported("computed item", "")
	}
	name, ok := c.conv.items[uint32(n)] // #nosec G115 -- item IDs are small; an unknown one fails the lookup.
	if !ok {
		return "", unsupported("unknown item", "%d", n)
	}
	return name, nil
}

func (c *itemConv) groupName(v Expr) (string, error) {
	n, ok := static(v)
	if !ok {
		return "", unsupported("computed item group", "")
	}
	name, ok := c.conv.groups[n]
	if !ok {
		return "", unsupported("unknown item group", "%d", n)
	}
	return name, nil
}

func (c *itemConv) monsterName(v Expr) (string, error) {
	n, ok := static(v)
	if !ok {
		return "", unsupported("computed monster", "")
	}
	name, ok := c.conv.monsters[n]
	if !ok {
		return "", unsupported("unknown monster", "%d", n)
	}
	return name, nil
}

// isStatic reports whether a call has only static values.
func (w *wcall) isStatic() bool {
	for _, a := range w.args {
		if !isStaticValue(a.v) {
			return false
		}
	}
	for _, o := range w.opts {
		if o.v != nil && !isStaticValue(o.v) {
			return false
		}
	}
	return true
}

func isStaticValue(e Expr) bool {
	switch e.(type) {
	case *Num, *Const, *Str:
		return true
	}
	return false
}

// luaBonus writes a bonuses.add call. guard is the condition on game
// values it ran under, or nil. A bonus under a guard, or with a game
// value, becomes computed:
//   - The amount is a function. Under a guard it returns 0 when the
//     guard is false, which adds nothing to a sum.
//   - A proc without an amount gets a computed chance, 0 when the guard
//     is false, so it never triggers.
//   - Flags, one-value bonuses, and bonuses with highest or lowest
//     stacking cannot be turned off this way. They are not converted
//     under a guard.
func (c *itemConv) luaBonus(w *wcall, guard Expr) (string, error) {
	b, err := c.toBonus(w)
	if err != nil {
		return "", err
	}
	spec, _ := bonus.Lookup(w.name)
	stacking := b.Options.Stacking
	sum := stacking == "" || stacking == "sum"
	amountAt := -1
	if bonus.HasAmount(spec, b) {
		amountAt = len(spec.Subject)
	}
	var g, when string
	if guard != nil {
		if g, err = c.cond(guard); err != nil {
			return "", err
		}
		switch {
		case amountAt >= 0 && sum && !spec.OneValue:
		case amountAt < 0 && spec.Proc && sum && !spec.OneValue:
		default:
			// Flags, one-value bonuses, granted skills, and highest or
			// lowest stacking have no value that turns them off. They
			// use bonuses.add_when, which is provisional.
			when, g = g, ""
		}
	}
	computed := func(v Expr, withGuard bool) (string, error) {
		val, err := c.num(v)
		if err != nil {
			return "", err
		}
		if withGuard && g != "" {
			return fmt.Sprintf("function()\n  if %s then\n    return %s\n  end\n  return 0\nend", g, val), nil
		}
		return "function() return " + val + " end", nil
	}
	parts := []string{luaString(w.name)}
	for i, a := range w.args {
		if i == amountAt && (!isStaticValue(a.v) || g != "") {
			s, err := computed(a.v, true)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
			continue
		}
		if a.kind == kLevel && !isStaticValue(a.v) {
			s, err := computed(a.v, false)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
			continue
		}
		s, err := c.ref(a.kind, a.v)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	var opts []string
	for _, o := range w.opts {
		var s string
		switch {
		case o.flag:
			s = "true"
		case o.enum != "":
			s = luaString(o.enum)
		case o.tkey != "":
			r, err := c.ref(o.kind, o.v)
			if err != nil {
				return "", err
			}
			s = "{ " + o.tkey + " = " + r + " }"
		case o.key == "chance" && (!isStaticValue(o.v) || g != "" && amountAt < 0):
			s, err = computed(o.v, amountAt < 0)
			if err != nil {
				return "", err
			}
		default:
			s, err = c.ref(o.kind, o.v)
			if err != nil {
				return "", err
			}
		}
		opts = append(opts, o.key+" = "+s)
	}
	if len(opts) > 0 {
		parts = append(parts, "{ "+strings.Join(opts, ", ")+" }")
	}
	c.file.need("bonuses")
	if when != "" {
		return "bonuses.add_when(function() return " + when + " end, " + strings.Join(parts, ", ") + ")", nil
	}
	return "bonuses.add(" + strings.Join(parts, ", ") + ")", nil
}

// dataBonus writes a static call as an item data entry: a flow list of
// the name, the arguments, and an options map.
func (c *itemConv) dataBonus(w *wcall) (string, error) {
	if _, err := c.toBonus(w); err != nil {
		return "", err
	}
	parts := []string{w.name}
	for _, a := range w.args {
		s, err := c.dataRef(a.kind, a.v)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	var opts []string
	for _, o := range w.opts {
		var s string
		switch {
		case o.flag:
			s = "true"
		case o.enum != "":
			s = o.enum
		case o.tkey != "":
			r, err := c.dataRef(o.kind, o.v)
			if err != nil {
				return "", err
			}
			s = "{" + o.tkey + ": " + r + "}"
		default:
			r, err := c.dataRef(o.kind, o.v)
			if err != nil {
				return "", err
			}
			s = r
		}
		opts = append(opts, o.key+": "+s)
	}
	if len(opts) > 0 {
		parts = append(parts, "{"+strings.Join(opts, ", ")+"}")
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

// yamlString writes a string as a plain YAML scalar when that reads back
// as the same string, and quoted otherwise.
func yamlString(s string) string {
	plain := s != "" && !strings.ContainsAny(s, ":#,[]{}&*!|>'\"%@`") && strings.TrimSpace(s) == s
	if plain {
		switch strings.ToLower(s) {
		case "true", "false", "null", "~", "yes", "no", "on", "off", "y", "n":
			plain = false
		}
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			plain = false
		}
		if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "?") {
			plain = false
		}
	}
	if plain {
		return s
	}
	return strconv.Quote(s)
}
