package convert

import (
	"fmt"
	"strings"
)

// useItem converts a usable item: its script becomes a "use" handler.
// An item with any part that does not convert gets no Lua, so the server
// refuses it before it spends the item.
func (c *itemConv) useItem(it *ItemScript) {
	out := c.item
	b, err := Parse(it.Script)
	if err != nil {
		c.fail("parse error", err)
		out.Status = None
		return
	}
	t := c.newImperative(2)
	t.block(b.Stmts)
	total, converted := 0, 0
	for _, ok := range out.Features {
		total++
		if ok {
			converted++
		}
	}
	switch {
	case !t.failed:
		out.Status = Full
	case converted > 0:
		out.Status = Partial
	default:
		out.Status = None
	}
	if t.failed {
		return
	}
	f := c.file
	f.need("events")
	f.line("return function()")
	f.indent++
	f.line(`events.on("use", function(player)`)
	f.indent++
	c.writeHelpers()
	f.indent--
	for _, l := range t.body() {
		f.body.WriteString(l + "\n")
	}
	f.line("end)")
	f.indent--
	f.line("end")
	out.Lua = f.String(header(it))
}

// bonusFeature is the report name of a bonus command, such as
// "bonus2 bAddRace".
func (c *itemConv) bonusFeature(cmd string, args []Expr) string {
	if len(args) > 0 {
		if n, err := c.bonusName(args[0]); err == nil {
			return cmd + " " + c.conv.bonusCase(n)
		}
	}
	return cmd
}

// equipItem converts an equipment item or card.
func (c *itemConv) equipItem(it *ItemScript) {
	out := c.item
	var setup []string
	var data []string
	static := true
	failed := false
	if it.Script != "" {
		effects, err := c.equipScript(it.Script)
		if err != nil {
			feature := "script"
			if ue, ok := err.(*unsupportedError); ok {
				feature = ue.Feature
			}
			c.fail(feature, err)
			failed = true
		}
		for _, e := range effects {
			cmd := strings.ToLower(e.cmd.Name)
			feature := cmd
			if strings.HasPrefix(cmd, "bonus") {
				feature = c.bonusFeature(cmd, e.args)
			}
			lines, entries, err := c.equipEffect(e)
			if err != nil {
				if ue, ok := err.(*unsupportedError); ok && strings.HasPrefix(ue.Feature, "conditional") {
					feature += " (conditional)"
				}
				c.fail(feature, err)
				failed = true
				continue
			}
			c.ok(feature)
			setup = append(setup, lines...)
			if entries == nil {
				static = false
			} else {
				data = append(data, entries...)
			}
		}
	}
	if it.EquipScript != "" {
		static = false
		lines, ok := c.onceScript(it.EquipScript, 1, false)
		failed = failed || !ok
		if ok {
			setup = append(setup, lines...)
		}
	}
	if it.UnequipScript != "" {
		static = false
		c.unequipping = true
		lines, ok := c.onceScript(it.UnequipScript, 2, true)
		c.unequipping = false
		failed = failed || !ok
		if ok && len(lines) > 0 && !onlyComments(lines) {
			c.file.need("events")
			setup = append(setup, `  events.on("unequip", function()`)
			setup = append(setup, lines...)
			setup = append(setup, "  end)")
		} else if ok {
			for _, l := range lines {
				setup = append(setup, "  "+strings.TrimSpace(l))
			}
		}
	}
	converted := 0
	for _, ok := range out.Features {
		if ok {
			converted++
		}
	}
	switch {
	case !failed:
		out.Status = Full
	case converted > 0:
		out.Status = Partial
	default:
		out.Status = None
	}
	if static && !failed {
		out.Bonuses = data
		return
	}
	if converted == 0 {
		return
	}
	for _, u := range out.Unsupported {
		setup = append(setup, "  -- Not converted: "+u)
	}
	c.writeEquip(it, setup)
}

func onlyComments(lines []string) bool {
	for _, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), "--") {
			return false
		}
	}
	return true
}

// writeEquip writes an equip entry script around the setup lines.
func (c *itemConv) writeEquip(it *ItemScript, setup []string) {
	f := c.file
	body := strings.Join(setup, "\n")
	usesPlayer := strings.Contains(body, "player") || len(c.helpers) > 0
	usesEquipped := strings.Contains(body, "equipped.") || usesPlayer
	f.line("return function()")
	f.indent++
	if usesEquipped {
		f.need("context")
		f.line("local equipped = context:get_instance()")
	}
	if usesPlayer {
		f.line("local player = equipped.wearer")
	}
	c.writeHelpers()
	f.indent--
	for _, l := range setup {
		f.body.WriteString(l + "\n")
	}
	f.line("end")
	c.item.Lua = f.String(header(it))
}

// writeHelpers writes the local helper functions the code uses.
func (c *itemConv) writeHelpers() {
	f := c.file
	if c.helpers["equipped_info"] {
		// rAthena reads -1 for the item, and 0 for the rest, of an
		// empty slot.
		f.line("local function equipped_info(slot, field)")
		f.line("  local copy = player:get_equipped(slot)")
		f.line("  if copy == nil then")
		f.line("    return field == \"item\" and -1 or 0")
		f.line("  end")
		f.line("  return copy[field]")
		f.line("end")
	}
}

// onceScript converts a script that runs once: on equip, or on unequip.
// It returns the lines and whether everything converted.
func (c *itemConv) onceScript(src string, indent int, unequip bool) ([]string, bool) {
	b, err := Parse(src)
	if err != nil {
		c.fail("parse error", err)
		return nil, false
	}
	t := c.newImperative(indent)
	t.block(b.Stmts)
	return t.body(), !t.failed
}

// equipEffect converts one effect of an equip script. It returns the
// setup lines, and the item data entries when the effect is static
// bonuses that always apply.
func (c *itemConv) equipEffect(e effect) ([]string, []string, error) {
	cmd := strings.ToLower(e.cmd.Name)
	switch cmd {
	case "bonus", "bonus2", "bonus3", "bonus4", "bonus5":
		n := 1
		if cmd != "bonus" {
			n = int(cmd[5] - '0')
		}
		calls, err := c.mapBonus(n, e.args)
		if err != nil {
			return nil, nil, err
		}
		return c.bonusLines(calls, e.guard)
	case "skill":
		if len(e.args) < 2 || len(e.args) > 3 {
			return nil, nil, unsupported("skill", "arguments")
		}
		if len(e.args) == 3 {
			if v, ok := static(e.args[2]); !ok || v != 1 {
				return nil, nil, unsupported("skill", "with a flag other than 1")
			}
		}
		return c.bonusLines([]*wcall{call("skill", of(kSkill, e.args[0]), of(kLevel, e.args[1]))}, e.guard)
	case "autobonus", "autobonus2", "autobonus3":
		// rAthena has the autobonus while the condition holds. The
		// handler checks the condition when it triggers, which is the
		// same.
		lines, err := c.autobonus(cmd, e.args, e.guard)
		return lines, nil, err
	case "hateffect", "specialeffect2":
		if e.guard != nil {
			return nil, nil, unsupported("conditional "+cmd, "")
		}
		t := c.newImperative(1)
		if err := t.commandErr(&Command{Name: cmd, Args: e.cmd.Args}); err != nil {
			return nil, nil, err
		}
		return t.body(), nil, nil
	case "sc_start", "sc_start2", "sc_start4", "sc_end":
		return nil, nil, unsupported("status effects", "%s", cmd)
	}
	return nil, nil, unsupported(cmd, "")
}

// bonusLines writes bonus calls. Static calls with no guard also give
// data entries.
func (c *itemConv) bonusLines(calls []*wcall, guard Expr) ([]string, []string, error) {
	var lines, entries []string
	allStatic := guard == nil
	for _, w := range calls {
		l, err := c.luaBonus(w, guard)
		if err != nil {
			return nil, nil, err
		}
		for _, part := range strings.Split(l, "\n") {
			lines = append(lines, "  "+part)
		}
		if allStatic && w.isStatic() {
			d, err := c.dataBonus(w)
			if err != nil {
				return nil, nil, err
			}
			entries = append(entries, d)
		} else {
			allStatic = false
		}
	}
	if !allStatic {
		entries = nil
	}
	return lines, entries, nil
}

// autobonus converts autobonus, autobonus2 and autobonus3 to handlers of
// the "damage" and "skill_used" events (0002a, "Item events").
func (c *itemConv) autobonus(cmd string, args []Expr, guard Expr) ([]string, error) {
	if len(args) < 3 {
		return nil, unsupported(cmd, "arguments")
	}
	// A bonus script built with + from text and values, such as
	// "{ bonus bAspdRate,"+.@r+"; }", becomes text with a variable for
	// each value.
	src, values, err := c.scriptText(args[0])
	if err != nil {
		return nil, unsupported(cmd, "a computed bonus script")
	}
	rate, err := c.num(args[1])
	if err != nil {
		return nil, err
	}
	duration, err := c.num(args[2])
	if err != nil {
		return nil, err
	}
	var other *Str
	var test string
	switch cmd {
	case "autobonus", "autobonus2":
		flag := int64(0)
		if len(args) > 3 {
			v, ok := static(args[3])
			if !ok {
				return nil, unsupported(cmd, "a computed flag")
			}
			flag = v
		}
		if len(args) > 4 {
			var ok bool
			if other, ok = args[4].(*Str); !ok {
				return nil, unsupported(cmd, "a computed effect script")
			}
		}
		if len(args) > 5 {
			return nil, unsupported(cmd, "arguments")
		}
		side := "hit.wearer_is_source"
		if cmd == "autobonus2" {
			side = "not hit.wearer_is_source"
		}
		test = strings.Join(append([]string{side}, hitTests(flag)...), " and ")
	case "autobonus3":
		if len(args) < 4 || len(args) > 5 {
			return nil, unsupported(cmd, "arguments")
		}
		sk, err := c.skillArg(args[3])
		if err != nil {
			return nil, err
		}
		if len(args) > 4 {
			var ok bool
			if other, ok = args[4].(*Str); !ok {
				return nil, unsupported(cmd, "a computed effect script")
			}
		}
		test = "use.skill == " + sk
	}
	if guard != nil {
		g, err := c.cond(guard)
		if err != nil {
			return nil, err
		}
		test += " and " + paren(g)
	}
	// The bonus script runs symbolically, like an equip script.
	effects, err := c.equipScriptEnv(strings.Trim(strings.TrimSpace(src), "{}"), values)
	if err != nil {
		return nil, err
	}
	var inner []string
	for _, e := range effects {
		lines, _, err := c.equipEffect(e)
		if err != nil {
			return nil, err
		}
		for _, l := range lines {
			inner = append(inner, "        "+strings.TrimPrefix(l, "  "))
		}
	}
	var after []string
	if other != nil {
		b, err := Parse(strings.Trim(strings.TrimSpace(other.Value), "{}"))
		if err != nil {
			return nil, unsupported(cmd, "effect script: %v", err)
		}
		t := c.newImperative(3)
		if t.block(b.Stmts); t.failed {
			return nil, unsupported(cmd, "effect script")
		}
		after = t.body()
	}
	c.autobonuses++
	active := "active"
	if c.autobonuses > 1 {
		active = fmt.Sprintf("active_%d", c.autobonuses)
	}
	event, arg := "damage", "hit"
	if cmd == "autobonus3" {
		event, arg = "skill_used", "use"
	}
	c.file.need("events")
	c.file.need("random")
	c.file.need("timers")
	out := []string{
		"  local " + active,
		fmt.Sprintf("  events.on(%q, function(%s)", event, arg),
		fmt.Sprintf("    if %s and random.chance(%s, 1000) then", test, rate),
		fmt.Sprintf("      %s = timers.with_bonuses(%s, function()", active, duration),
	}
	out = append(out, inner...)
	out = append(out, fmt.Sprintf("      end, { restart = %s })", active))
	out = append(out, after...)
	out = append(out, "    end", "  end)")
	return out, nil
}

// scriptText flattens a script given as a string, or as strings and
// values joined with +. Each value becomes a variable, given in env.
func (c *itemConv) scriptText(e Expr) (string, env, error) {
	values := env{}
	var b strings.Builder
	var walk func(Expr) error
	walk = func(e Expr) error {
		switch e := e.(type) {
		case *Str:
			b.WriteString(e.Value)
			return nil
		case *Binary:
			if e.Op == "+" {
				if err := walk(e.X); err != nil {
					return err
				}
				return walk(e.Y)
			}
		}
		if v, ok := static(e); ok {
			fmt.Fprint(&b, v)
			return nil
		}
		if _, ok := e.(*Var); ok {
			return unsupported("computed script", "")
		}
		name := fmt.Sprintf(".@wa_value_%d", len(values)+1)
		values[name] = e
		b.WriteString("(" + name + ")")
		return nil
	}
	if err := walk(e); err != nil {
		return "", nil, err
	}
	return b.String(), values, nil
}

// hitTests are the checks on a hit for an autobonus flag, after
// pc_addautobonus's defaults.
func hitTests(flag int64) []string {
	if flag&(bfShort|bfLong) == 0 {
		flag |= bfShort | bfLong
	}
	if flag&(bfWeapon|bfMagic|bfMisc) == 0 {
		flag |= bfWeapon
	}
	if flag&(bfSkill|bfNormal) == 0 {
		if flag&(bfMagic|bfMisc) != 0 {
			flag |= bfSkill
		}
		if flag&bfWeapon != 0 {
			flag |= bfNormal | bfSkill
		}
	}
	var tests []string
	var kinds []string
	for _, k := range []struct {
		bit  int64
		name string
	}{{bfWeapon, "physical"}, {bfMagic, "magic"}, {bfMisc, "misc"}} {
		if flag&k.bit != 0 {
			kinds = append(kinds, fmt.Sprintf("hit.kind == %q", k.name))
		}
	}
	switch len(kinds) {
	case 1:
		tests = append(tests, kinds[0])
	case 2:
		tests = append(tests, "("+strings.Join(kinds, " or ")+")")
	}
	switch flag & (bfShort | bfLong) {
	case bfShort:
		tests = append(tests, `hit.range == "melee"`)
	case bfLong:
		tests = append(tests, `hit.range == "ranged"`)
	}
	switch flag & (bfNormal | bfSkill) {
	case bfNormal:
		tests = append(tests, "hit.skill == nil")
	case bfSkill:
		tests = append(tests, "hit.skill ~= nil")
	}
	return tests
}
