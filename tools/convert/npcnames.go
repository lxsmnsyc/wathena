package convert

import (
	"fmt"
	"regexp"
	"strings"
)

// Names and forms rAthena's parser takes that the converter used to
// refuse: variables by computed names (review decision 60, F3), labels
// inside blocks, and sscanf.

// visitExpr calls fn for an expression and the expressions inside it.
func visitExpr(e Expr, fn func(Expr)) {
	if e == nil {
		return
	}
	fn(e)
	switch e := e.(type) {
	case *Var:
		visitExpr(e.Index, fn)
	case *Call:
		for _, a := range e.Args {
			visitExpr(a, fn)
		}
	case *Unary:
		visitExpr(e.X, fn)
	case *Binary:
		visitExpr(e.X, fn)
		visitExpr(e.Y, fn)
	case *Ternary:
		visitExpr(e.Cond, fn)
		visitExpr(e.Then, fn)
		visitExpr(e.Else, fn)
	case *Assign:
		visitExpr(e.Target, fn)
		visitExpr(e.Value, fn)
	case *IncDec:
		visitExpr(e.Target, fn)
	}
}

// walkExprs calls fn for each expression of the statements, nested
// ones too.
func walkExprs(list []Stmt, fn func(Expr)) {
	walkStmts(list, func(s Stmt) {
		switch s := s.(type) {
		case *If:
			visitExpr(s.Cond, fn)
		case *Switch:
			visitExpr(s.Value, fn)
			for _, cs := range s.Cases {
				for _, v := range cs.Values {
					visitExpr(v, fn)
				}
			}
		case *For:
			visitExpr(s.Cond, fn)
			walkExprs([]Stmt{s.Init, s.Post}, fn)
		case *While:
			visitExpr(s.Cond, fn)
		case *Command:
			for _, a := range s.Args {
				visitExpr(a, fn)
			}
		case *Assign:
			visitExpr(s.Target, fn)
			visitExpr(s.Value, fn)
		case *IncDec:
			visitExpr(s.Target, fn)
		case *SetRef:
			visitExpr(s.Ref, fn)
			visitExpr(s.Value, fn)
		case *Return:
			visitExpr(s.Value, fn)
		}
	})
}

// nameScope is where a computed getd or setd name lives, as far as its
// fixed start says.
type nameScope int

const (
	// nameFixed is a name whose fixed start makes its scope: a character,
	// account or server variable.
	nameFixed nameScope = iota
	// nameLocal is a .@ variable.
	nameLocal
	// nameDot is a . variable of the NPC.
	nameDot
	// nameAny is a name whose start is not fixed, or does not fix the
	// scope, such as getd(.@var$) and getd("." + .@name$). It may name a
	// .@ or . variable too.
	nameAny
)

var namePrefix = regexp.MustCompile(`^(\.@|\.|\$@|\$|##|#|@|')?`)

// scopeOfName reads the first part of a computed name.
func scopeOfName(e Expr) nameScope {
	for {
		bin, ok := e.(*Binary)
		if !ok || bin.Op != "+" {
			break
		}
		e = bin.X
	}
	first, ok := e.(*Str)
	if !ok {
		return nameAny
	}
	prefix := namePrefix.FindString(first.Value)
	rest := first.Value[len(prefix):]
	if rest == "" || !variableRef.MatchString(prefix+rest[:1]) {
		return nameAny
	}
	switch prefix {
	case ".@":
		return nameLocal
	case ".":
		return nameDot
	}
	return nameFixed
}

// computedNames reads which scopes the computed getd and setd names of
// some statements may reach. A function whose names may reach .@
// variables keeps them in one table, and a script whose names may reach
// . variables makes them NPC variables (review decision 60, F3).
func computedNames(stmts []Stmt) (locals, dots bool) {
	note := func(name Expr) {
		if _, fixed := asVar(&Call{Name: "getd", Args: []Expr{name}}); fixed {
			return
		}
		switch scopeOfName(name) {
		case nameLocal:
			locals = true
		case nameDot:
			dots = true
		case nameAny:
			locals, dots = true, true
		}
	}
	walkStmts(stmts, func(s Stmt) {
		switch s := s.(type) {
		case *Command:
			if strings.EqualFold(s.Name, "setd") && len(s.Args) > 0 {
				note(s.Args[0])
			}
		case *SetRef:
			if strings.EqualFold(s.Ref.Name, "getd") && len(s.Ref.Args) == 1 {
				note(s.Ref.Args[0])
			}
		}
	})
	walkExprs(stmts, func(e Expr) {
		if c, ok := e.(*Call); ok && strings.EqualFold(c.Name, "getd") && len(c.Args) == 1 {
			note(c.Args[0])
		}
	})
	return locals, dots
}

// keepLocalsInTable makes the body keep its .@ variables in one table
// when a computed name in its statements may reach them.
func (b *body) keepLocalsInTable(stmts []Stmt) {
	if locals, _ := computedNames(stmts); locals {
		b.table = b.n.fresh("locals")
	}
}

// capture runs a conversion step and returns the lines it wrote, taken
// back out of the body, without their indent.
func (b *body) capture(step func() error) ([]string, error) {
	mark, indent := len(b.lines), b.indent
	b.indent = 0
	err := step()
	var out []string
	for _, l := range b.lines[mark:] {
		// A comment in the middle of the line would hide what follows it.
		if !strings.HasPrefix(l, "--") {
			out = append(out, l)
		}
	}
	b.lines, b.indent = b.lines[:mark], indent
	return out, err
}

// sscanf converts sscanf(<text>, <format>, <variables>...) (buildin_sscanf,
// script.cpp 17555). It reads the format %d, %s, %% and literal text, one
// conversion into each variable, and returns the count, or -1 when a
// conversion or a literal does not match, with the variables before it
// set. The format must be fixed text, and each %d must go into a number
// variable and each %s into a string variable. The result is the local
// that holds the count.
func (b *body) sscanf(args []Expr) (string, error) {
	const what = "sscanf"
	if len(args) < 2 {
		return "", unsupported(what, "arguments")
	}
	format, err := b.expr(args[1])
	if err != nil {
		return "", err
	}
	if format.str == nil {
		return "", unsupported(what, "a computed format")
	}
	var kinds []byte
	for i := 0; i < len(*format.str); i++ {
		if (*format.str)[i] != '%' {
			continue
		}
		i++
		switch {
		case i < len(*format.str) && (*format.str)[i] == '%':
		case i < len(*format.str) && ((*format.str)[i] == 'd' || (*format.str)[i] == 's'):
			kinds = append(kinds, (*format.str)[i])
		default:
			return "", unsupported(what, "a conversion other than %%d and %%s")
		}
	}
	targets := args[2:]
	if len(targets) != len(kinds) {
		return "", unsupported(what, "a different number of conversions and variables")
	}
	vars := make([]*Var, len(targets))
	for i, t := range targets {
		v, ok := asVar(t)
		if !ok {
			return "", unsupported(what, "a target that is not a variable")
		}
		if _, _, str := varName(v.Name); str != (kinds[i] == 's') {
			return "", unsupported(what, "a conversion that does not match the type of %s", v.Name)
		}
		vars[i] = v
	}
	text, err := b.expr(args[0])
	if err != nil {
		return "", err
	}
	code := text.code
	if text.t != tStr {
		code = "R.text(" + text.asInt() + ")"
	}
	count, got := b.hide("scan"), b.hide("scanned")
	b.line("%s, %s = R.sscanf(%s, %s)", count, got, code, format.code)
	for i, v := range vars {
		t := tInt
		if kinds[i] == 's' {
			t = tStr
		}
		b.line("if %s[%d] ~= nil then", got, i+1)
		b.indent++
		err := b.writeVar(v, val{code: fmt.Sprintf("%s[%d]", got, i+1), t: t})
		b.indent--
		if err != nil {
			return "", err
		}
		b.line("end")
	}
	return count, nil
}

// Labels inside blocks. Lua's goto sees a label from its own block and
// the blocks inside it, and rAthena jumps to any label. enterLabels
// rewrites a jump from outside the block (proposal 0002 step 40) so
// that it sets a flag and jumps to a new label before the top-level
// statement that holds the label. The
// statements on the way to the label then run only when the flag is
// clear, their conditions are skipped while it is set, and the label
// clears it:
//
//	if (a) {          if (flag || a) {
//	  x;                if (!flag) x;
//	  L: y;     =>      L: flag = 0; y;
//	}                 }
//
// It enters blocks, if branches and switch cases. A label in a loop, or
// behind another label that a jump reaches, stays as it was.
func enterLabels(stmts []Stmt) []Stmt {
	skip := map[string]bool{}
	for {
		name, list := nextDeepLabel(stmts, skip)
		if name == "" {
			return stmts
		}
		skip[name] = true
		if out, ok := enterLabel(stmts, name, list); ok {
			stmts = out
		}
	}
}

// nextDeepLabel finds a label inside a block that a jump from outside
// the block reaches, and the statements of the block.
func nextDeepLabel(stmts []Stmt, skip map[string]bool) (string, []Stmt) {
	var gotos []*Goto
	walkStmts(stmts, func(s Stmt) {
		if g, ok := s.(*Goto); ok {
			gotos = append(gotos, g)
		}
	})
	for _, top := range stmts {
		if _, ok := top.(*Label); ok {
			continue
		}
		var name string
		var list []Stmt
		walkStmts([]Stmt{top}, func(s Stmt) {
			l, ok := s.(*Label)
			if !ok || name != "" || skip[strings.ToLower(l.Name)] {
				return
			}
			own := labelList([]Stmt{top}, l.Name)
			for _, g := range gotos {
				if strings.EqualFold(g.Label, l.Name) && !hasGoto(own, g) {
					name, list = strings.ToLower(l.Name), own
					return
				}
			}
		})
		if name != "" {
			return name, list
		}
	}
	return "", nil
}

// labelList is the statements of the block that holds a label.
func labelList(list []Stmt, name string) []Stmt {
	var found []Stmt
	var look func(list []Stmt)
	look = func(list []Stmt) {
		for _, s := range list {
			if found != nil {
				return
			}
			switch s := s.(type) {
			case *Label:
				if strings.EqualFold(s.Name, name) {
					found = list
					return
				}
			case *Block:
				look(s.Stmts)
			case *If:
				look([]Stmt{s.Then})
				if s.Else != nil {
					look([]Stmt{s.Else})
				}
			case *Switch:
				look(s.Pre)
				for _, cs := range s.Cases {
					look(cs.Body)
				}
			case *For:
				look([]Stmt{s.Body})
			case *While:
				look([]Stmt{s.Body})
			}
		}
	}
	look(list)
	return found
}

// hasGoto reports whether a goto statement is in the statements.
func hasGoto(list []Stmt, g *Goto) bool {
	found := false
	walkStmts(list, func(s Stmt) { found = found || s == Stmt(g) })
	return found
}

// holdsLabel reports whether a statement has the label inside it.
func holdsLabel(s Stmt, name string) bool {
	found := false
	walkStmts([]Stmt{s}, func(s Stmt) {
		if l, ok := s.(*Label); ok && strings.EqualFold(l.Name, name) {
			found = true
		}
	})
	return found
}

// enterLabel rewrites the jumps to one label, which own is the block of.
func enterLabel(stmts []Stmt, name string, own []Stmt) ([]Stmt, bool) {
	flag := &Var{Name: ".@wa_goto_" + name}
	enter := "wa_enter_" + name
	jump := func(g *Goto) Stmt {
		return &Block{Stmts: []Stmt{&Assign{Target: flag, Op: "=", Value: &Num{Value: 1}}, &Goto{Label: enter}}}
	}
	stmts = mapGotos(stmts, func(g *Goto) Stmt {
		if strings.EqualFold(g.Label, name) && !hasGoto(own, g) {
			return jump(g)
		}
		return g
	})
	for i, s := range stmts {
		if !holdsLabel(s, name) {
			continue
		}
		list, ok := enterList(stmts[i:i+1], name, flag)
		if !ok {
			return nil, false
		}
		out := append([]Stmt{}, stmts[:i]...)
		out = append(out, &Label{Name: enter})
		out = append(out, list...)
		return append(out, stmts[i+1:]...), true
	}
	return nil, false
}

// mapGotos rebuilds statements with each goto replaced by fn's result.
// A local function keeps its own jumps.
func mapGotos(list []Stmt, fn func(*Goto) Stmt) []Stmt {
	out := make([]Stmt, len(list))
	for i, s := range list {
		out[i] = mapGoto(s, fn)
	}
	return out
}

func mapGoto(s Stmt, fn func(*Goto) Stmt) Stmt {
	switch s := s.(type) {
	case *Goto:
		return fn(s)
	case *Block:
		return &Block{Stmts: mapGotos(s.Stmts, fn)}
	case *If:
		c := *s
		c.Then = mapGoto(s.Then, fn)
		if s.Else != nil {
			c.Else = mapGoto(s.Else, fn)
		}
		return &c
	case *Switch:
		c := *s
		c.Pre = mapGotos(s.Pre, fn)
		c.Cases = make([]*Case, len(s.Cases))
		for i, cs := range s.Cases {
			k := *cs
			k.Body = mapGotos(cs.Body, fn)
			c.Cases[i] = &k
		}
		return &c
	case *For:
		c := *s
		c.Body = mapGoto(s.Body, fn)
		return &c
	case *While:
		c := *s
		c.Body = mapGoto(s.Body, fn)
		return &c
	}
	return s
}

// guarded runs statements only while the flag is clear. A statement
// that holds a label cannot be guarded.
func guarded(list []Stmt, flag *Var) ([]Stmt, bool) {
	var out []Stmt
	for _, s := range list {
		switch s.(type) {
		case *Label, *FuncDecl:
			out = append(out, s)
			continue
		}
		hasLabel := false
		walkStmts([]Stmt{s}, func(s Stmt) {
			if _, ok := s.(*Label); ok {
				hasLabel = true
			}
		})
		if hasLabel {
			return nil, false
		}
		out = append(out, &If{Cond: flagClear(flag), Then: s})
	}
	return out, true
}

func flagSet(flag *Var) Expr   { return &Binary{Op: "!=", X: flag, Y: &Num{Value: 0}} }
func flagClear(flag *Var) Expr { return &Binary{Op: "==", X: flag, Y: &Num{Value: 0}} }

// enterList makes a block run from the label on when the flag is set.
func enterList(list []Stmt, name string, flag *Var) ([]Stmt, bool) {
	for i, s := range list {
		if l, ok := s.(*Label); ok && strings.EqualFold(l.Name, name) {
			out, ok := guarded(list[:i], flag)
			if !ok {
				return nil, false
			}
			out = append(out, l, &Assign{Target: flag, Op: "=", Value: &Num{Value: 0}})
			return append(out, list[i+1:]...), true
		}
		if !holdsLabel(s, name) {
			continue
		}
		inner, ok := enterStmt(s, name, flag)
		if !ok {
			return nil, false
		}
		out, ok := guarded(list[:i], flag)
		if !ok {
			return nil, false
		}
		out = append(out, inner)
		return append(out, list[i+1:]...), true
	}
	return nil, false
}

// enterStmt makes a statement run from the label inside it on when the
// flag is set.
func enterStmt(s Stmt, name string, flag *Var) (Stmt, bool) {
	switch s := s.(type) {
	case *Block:
		list, ok := enterList(s.Stmts, name, flag)
		return &Block{Stmts: list}, ok
	case *If:
		c := *s
		if holdsLabel(s.Then, name) {
			inner, ok := enterStmt(s.Then, name, flag)
			c.Cond, c.Then = &Binary{Op: "||", X: flagSet(flag), Y: s.Cond}, inner
			return &c, ok
		}
		inner, ok := enterStmt(s.Else, name, flag)
		c.Cond, c.Else = &Binary{Op: "&&", X: flagClear(flag), Y: s.Cond}, inner
		return &c, ok
	case *Switch:
		return enterSwitch(s, name, flag)
	}
	return nil, false
}

// enterSwitch skips the value of a switch while the flag is set and
// picks the case that holds the label.
func enterSwitch(s *Switch, name string, flag *Var) (Stmt, bool) {
	c := *s
	pre, ok := guarded(s.Pre, flag)
	if !ok {
		return nil, false
	}
	c.Pre = pre
	c.Cases = make([]*Case, len(s.Cases))
	var pick Expr
	for i, cs := range s.Cases {
		k := *cs
		c.Cases[i] = &k
		holds := false
		walkStmts(cs.Body, func(s Stmt) {
			if l, ok := s.(*Label); ok && strings.EqualFold(l.Name, name) {
				holds = true
			}
		})
		if !holds {
			continue
		}
		body, ok := enterList(cs.Body, name, flag)
		if !ok {
			return nil, false
		}
		k.Body = body
		if len(cs.Values) > 0 {
			pick = cs.Values[0]
			continue
		}
		// The default case: a value that no other case has.
		var next int64
		for _, o := range s.Cases {
			for _, v := range o.Values {
				n, ok := v.(*Num)
				if !ok {
					return nil, false
				}
				next = max(next, n.Value+1)
			}
		}
		pick = &Num{Value: next}
	}
	if pick == nil {
		return nil, false
	}
	c.Value = &Ternary{Cond: flagSet(flag), Then: pick, Else: s.Value}
	return &c, true
}
