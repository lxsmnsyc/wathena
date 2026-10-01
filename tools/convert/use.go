package convert

import (
	"fmt"
	"strings"
)

// imperative translates statements that run once, in order: use
// handlers, equip scripts that run on equip, unequip handlers, and the
// effect scripts of autobonus. Variables become Lua locals, and commands
// become calls on the player.
type imperative struct {
	c      *itemConv
	lines  []string
	indent int
	locals []string
	seen   map[string]bool
	failed bool
	// loops counts open loops, and conts the loops whose body uses
	// continue.
	loops int
	conts []bool
}

func (c *itemConv) newImperative(indent int) *imperative {
	return &imperative{c: c, indent: indent, seen: map[string]bool{}}
}

func (t *imperative) line(format string, args ...any) {
	t.lines = append(t.lines, strings.Repeat("  ", t.indent)+fmt.Sprintf(format, args...))
}

// body returns the lines, with the locals declared first.
func (t *imperative) body() []string {
	if len(t.locals) == 0 {
		return t.lines
	}
	zeros := make([]string, len(t.locals))
	for i, l := range t.locals {
		zeros[i] = "0"
		if strings.HasSuffix(l, "$") {
			zeros[i] = `""`
		}
	}
	names := make([]string, len(t.locals))
	for i, l := range t.locals {
		names[i] = t.c.local(l)
	}
	decl := strings.Repeat("  ", t.indent) + "local " + strings.Join(names, ", ") + " = " + strings.Join(zeros, ", ")
	return append([]string{decl}, t.lines...)
}

func (t *imperative) fail(feature string, err error) {
	t.failed = true
	t.c.fail(feature, err)
}

func (t *imperative) num(e Expr) (string, error) {
	r, err := t.c.resolve(e, scopeUse, nil)
	if err != nil {
		return "", err
	}
	return t.c.num(r)
}

func (t *imperative) cond(e Expr) (string, error) {
	r, err := t.c.resolve(e, scopeUse, nil)
	if err != nil {
		return "", err
	}
	return t.c.cond(r)
}

func (t *imperative) block(stmts []Stmt) {
	for _, s := range stmts {
		t.stmt(s)
	}
}

func (t *imperative) nested(s Stmt) {
	t.indent++
	if b, ok := s.(*Block); ok {
		t.block(b.Stmts)
	} else if s != nil {
		t.stmt(s)
	}
	t.indent--
}

func (t *imperative) declare(v *Var) error {
	if !strings.HasPrefix(v.Name, ".@") || v.Index != nil {
		return unsupported("variables", "%s keeps state outside the script", v.Name)
	}
	if !t.seen[v.Name] {
		t.seen[v.Name] = true
		t.locals = append(t.locals, v.Name)
	}
	return nil
}

func (t *imperative) stmt(s Stmt) {
	switch s := s.(type) {
	case *Block:
		t.block(s.Stmts)
	case *Assign:
		if err := t.declare(s.Target); err != nil {
			t.fail("variables", err)
			return
		}
		value := Expr(s.Value)
		if s.Op != "=" {
			value = &Binary{Op: strings.TrimSuffix(s.Op, "="), X: s.Target, Y: s.Value}
		}
		v, err := t.num(value)
		if err != nil {
			t.fail("expression", err)
			return
		}
		t.line("%s = %s", t.c.local(s.Target.Name), v)
	case *IncDec:
		if err := t.declare(s.Target); err != nil {
			t.fail("variables", err)
			return
		}
		op := map[string]string{"++": "+", "--": "-"}[s.Op]
		l := t.c.local(s.Target.Name)
		t.line("%s = %s %s 1", l, l, op)
	case *If:
		cond, err := t.cond(s.Cond)
		if err != nil {
			t.fail("condition", err)
			return
		}
		t.line("if %s then", cond)
		t.nested(s.Then)
		for s.Else != nil {
			if next, ok := s.Else.(*If); ok {
				cond, err := t.cond(next.Cond)
				if err != nil {
					t.fail("condition", err)
					return
				}
				t.line("elseif %s then", cond)
				t.nested(next.Then)
				s = next
				continue
			}
			t.line("else")
			t.nested(s.Else)
			break
		}
		t.line("end")
	case *Switch:
		t.switchStmt(s)
	case *For:
		if s.Init != nil {
			t.stmt(s.Init)
		}
		cond := "true"
		if s.Cond != nil {
			c, err := t.cond(s.Cond)
			if err != nil {
				t.fail("condition", err)
				return
			}
			cond = c
		}
		t.loop(cond, s.Body, s.Post, false)
	case *While:
		cond, err := t.cond(s.Cond)
		if err != nil {
			t.fail("condition", err)
			return
		}
		t.loop(cond, s.Body, nil, s.Do)
	case *Jump:
		switch s.Kind {
		case "break":
			if t.loops == 0 {
				t.fail("break", unsupported("break", "outside a loop"))
				return
			}
			t.line("break")
		case "continue":
			if t.loops == 0 {
				t.fail("continue", unsupported("continue", "outside a loop"))
				return
			}
			t.conts[len(t.conts)-1] = true
			t.line("goto continue")
		default:
			t.line("do return end")
		}
	case *Return:
		t.line("do return end")
	case *Command:
		t.command(s)
	case *Unsupported:
		t.fail("labels and goto", unsupported("labels and goto", "%s", s.Text))
	}
}

// loop writes a while loop. A body with continue gets a label before the
// step.
func (t *imperative) loop(cond string, body, post Stmt, do bool) {
	t.loops++
	t.conts = append(t.conts, false)
	start := len(t.lines)
	if do {
		t.line("repeat")
	} else {
		t.line("while %s do", cond)
	}
	t.nested(body)
	if t.conts[len(t.conts)-1] {
		t.indent++
		t.line("::continue::")
		t.indent--
	}
	if post != nil {
		t.nested(post)
	}
	if do {
		t.line("until not (%s)", cond)
	} else {
		t.line("end")
	}
	_ = start
	t.loops--
	t.conts = t.conts[:len(t.conts)-1]
}

// switchStmt writes a switch as if and elseif, when every case ends with
// a break.
func (t *imperative) switchStmt(s *Switch) {
	v, err := t.num(s.Value)
	if err != nil {
		t.fail("switch", err)
		return
	}
	t.c.switches++
	name := fmt.Sprintf("value_%d", t.c.switches)
	t.line("local %s = %s", name, v)
	first := true
	for i, cs := range s.Cases {
		body := cs.Body
		if n := len(body); n > 0 {
			if j, ok := body[n-1].(*Jump); ok && j.Kind == "break" {
				body = body[:n-1]
			} else if i != len(s.Cases)-1 {
				t.fail("switch", unsupported("switch", "a case that falls through"))
				return
			}
		}
		if cs.Default {
			if first {
				t.line("do")
			} else {
				t.line("else")
			}
		} else {
			var conds []string
			for _, cv := range cs.Values {
				x, err := t.num(cv)
				if err != nil {
					t.fail("switch", err)
					return
				}
				conds = append(conds, name+" == "+x)
			}
			kw := "elseif"
			if first {
				kw = "if"
			}
			t.line("%s %s then", kw, strings.Join(conds, " or "))
		}
		first = false
		t.nested(&Block{Stmts: body})
	}
	if !first {
		t.line("end")
	}
}

// command writes one command.
func (t *imperative) command(s *Command) {
	name := strings.ToLower(s.Name)
	feature := name
	err := t.commandErr(s)
	if err != nil {
		if ue, ok := err.(*unsupportedError); ok && strings.HasPrefix(ue.Feature, name+" ") {
			feature = ue.Feature
		}
		t.fail(feature, err)
		return
	}
	t.c.ok(feature)
}

// heal writes the table of itemheal or percentheal: { hp = ..., sp = ... }.
// An amount that is rand(min, max) becomes { min, max }, which the
// server draws as rand does, HP before SP.
func (t *imperative) heal(args []Expr) (string, error) {
	var parts []string
	for i, a := range args {
		key := []string{"hp", "sp"}[i]
		if r, ok := a.(*Call); ok && strings.EqualFold(r.Name, "rand") {
			var lo, hi int64
			var ok1, ok2 bool
			switch len(r.Args) {
			case 1:
				var n int64
				n, ok1 = t.staticNum(r.Args[0])
				lo, hi, ok2 = 0, n-1, true
			case 2:
				lo, ok1 = t.staticNum(r.Args[0])
				hi, ok2 = t.staticNum(r.Args[1])
				if lo > hi {
					lo, hi = hi, lo
				}
			}
			if ok1 && ok2 {
				parts = append(parts, fmt.Sprintf("%s = { %d, %d }", key, lo, hi))
				continue
			}
		}
		v, err := t.num(a)
		if err != nil {
			return "", err
		}
		if v == "0" {
			continue
		}
		parts = append(parts, key+" = "+v)
	}
	return "{ " + strings.Join(parts, ", ") + " }", nil
}

func (t *imperative) staticNum(e Expr) (int64, bool) {
	r, err := t.c.resolve(e, scopeUse, nil)
	if err != nil {
		return 0, false
	}
	return static(r)
}

func (t *imperative) commandErr(s *Command) error {
	name := strings.ToLower(s.Name)
	args := s.Args
	c := t.c
	switch name {
	case "itemheal", "percentheal":
		if len(args) != 2 {
			return unsupported(name, "arguments")
		}
		tbl, err := t.heal(args)
		if err != nil {
			return err
		}
		method := map[string]string{"itemheal": "item_heal", "percentheal": "heal_percent"}[name]
		t.line("player:%s %s", method, tbl)
		return nil
	case "getitem", "rentitem":
		if len(args) != 2 {
			return unsupported(name, "with an account ID or other extra arguments")
		}
		item, err := t.itemArg(args[0])
		if err != nil {
			return err
		}
		amount, err := t.num(args[1])
		if err != nil {
			return err
		}
		if name == "rentitem" {
			t.line("player:give_item(%s, 1, { rent_seconds = %s })", item, amount)
		} else {
			t.line("player:give_item(%s, %s)", item, amount)
		}
		return nil
	case "getgroupitem":
		if len(args) < 1 || len(args) > 2 {
			return unsupported(name, "arguments")
		}
		// The identify argument is left out: wAthena's items have no
		// unidentified state yet, so every item is identified.
		if len(args) == 2 {
			if _, ok := t.staticNum(args[1]); !ok {
				return unsupported(name, "with a computed identify argument")
			}
		}
		g, err := c.resolve(args[0], scopeUse, nil)
		if err != nil {
			return err
		}
		group, err := c.ref(kItemGroup, g)
		if err != nil {
			return err
		}
		t.line("player:give_item_from_group(%s)", group)
		return nil
	case "specialeffect2":
		if len(args) != 1 {
			return unsupported(name, "on another character")
		}
		eff, err := t.constArg(args[0], "effects")
		if err != nil {
			return err
		}
		t.line("player:show_effect(%s)", eff)
		return nil
	case "warp":
		if len(args) != 3 {
			return unsupported(name, "arguments")
		}
		m, ok := args[0].(*Str)
		if !ok {
			return unsupported(name, "to a computed map")
		}
		switch m.Value {
		case "Random":
			t.line("player:warp_to_random_cell()")
			return nil
		case "SavePoint":
			t.line("player:warp_to_save_point()")
			return nil
		}
		x, err := t.num(args[1])
		if err != nil {
			return err
		}
		y, err := t.num(args[2])
		if err != nil {
			return err
		}
		t.line("player:warp(%s, %s, %s)", luaString(m.Value), x, y)
		return nil
	case "itemskill":
		if len(args) >= 2 {
			if sk, ok := args[0].(*Str); ok && strings.EqualFold(sk.Value, "AL_TELEPORT") {
				lv, ok := t.staticNum(args[1])
				// AL_TELEPORT from an item: level 1 is a random cell, and
				// level 3 the save point (skill_castend_nodamage_id).
				switch {
				case ok && lv == 1 && len(args) == 2:
					t.line("player:warp_to_random_cell()")
					return nil
				case ok && lv == 3 && len(args) == 2:
					t.line("player:warp_to_save_point()")
					return nil
				}
			}
		}
		return unsupported(name, "skills used by items")
	case "hateffect":
		if len(args) != 2 {
			return unsupported(name, "arguments")
		}
		on, ok := t.staticNum(args[1])
		if !ok {
			return unsupported(name, "computed state")
		}
		if on == 0 {
			if !t.c.unequipping {
				return unsupported(name, "turned off outside an unequip script")
			}
			// The effect belongs to the equip context, which ends on
			// unequip and removes it.
			t.line("-- hateffect off: the headgear effect ends with the equip context.")
			return nil
		}
		eff, err := t.constArg(args[0], "headgear_effects")
		if err != nil {
			return err
		}
		t.line("player:show_headgear_effect(%s)", eff)
		return nil
	case "sc_start", "sc_start2", "sc_start4", "sc_end", "sc_end_class":
		return unsupported("status effects", "%s", name)
	}
	return unsupported(name, "")
}

// itemArg writes an item given by ID, name, or a computed choice.
func (t *imperative) itemArg(e Expr) (string, error) {
	r, err := t.c.resolve(e, scopeUse, nil)
	if err != nil {
		return "", err
	}
	if call, ok := r.(*Call); ok && call.Name == "F_Rand" {
		var parts []string
		for _, a := range call.Args {
			s, err := t.c.ref(kItem, a)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return fmt.Sprintf("({ %s })[random.integer(1, %d)]", strings.Join(parts, ", "), len(parts)), nil
	}
	if isStaticValue(r) {
		return t.c.ref(kItem, r)
	}
	return "", unsupported("computed item", "")
}

// constArg writes a constant of a module, such as effects.heal.
func (t *imperative) constArg(e Expr, module string) (string, error) {
	r, err := t.c.resolve(e, scopeUse, nil)
	if err != nil {
		return "", err
	}
	v, ok := static(r)
	if !ok {
		return "", unsupported("computed constant", "")
	}
	name, ok := t.c.conv.modules[module].Names[v]
	if !ok {
		// IDs that rAthena has no constant for stay numbers.
		return fmt.Sprint(v), nil
	}
	return t.c.file.need(module) + "." + name, nil
}
