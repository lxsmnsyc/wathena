package convert

import (
	"strings"
)

// Equip scripts are run symbolically. rAthena reruns an equip script on
// every status recalculation, so a condition on the refine level or the
// base level decides which bonuses exist at that moment. The converter
// walks the script once:
//   - Variables hold expressions instead of values.
//   - A condition on static values picks its branch. A condition on game
//     values runs both branches, each under its guard, and variables
//     that differ become Ternary.
//   - Loops with static bounds are unrolled.
//   - Each bonus is recorded with the guard it ran under.
// The result is a list of bonuses whose amounts are expressions. A guard
// becomes the bonus's when option, and a game value in an amount becomes
// a computed bonus. Both rerun when a value they read changes (proposal
// 0002, "Item scripts").

// maxLoop is the most iterations an unrolled loop may take.
const maxLoop = 1000

// effect is one thing an equip script does under a guard.
type effect struct {
	guard Expr // nil when it always runs
	cmd   *Command
	args  []Expr // resolved arguments
}

type equipRun struct {
	c       *itemConv
	effects []effect
	steps   int
}

// env holds .@ variables.
type env map[string]Expr

func (e env) clone() env {
	out := make(env, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

// and joins two guards. nil is true.
func and(a, b Expr) Expr {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	return &Binary{Op: "&&", X: a, Y: b}
}

func not(a Expr) Expr {
	if v, ok := static(a); ok {
		return &Num{Value: b2i(v == 0)}
	}
	return &Unary{Op: "!", X: a}
}

// flow is how a statement left: normally, or by break, continue or end.
// Each holds the condition under which it happened: nil for never, and
// a Num 1 for always.
type flow struct {
	brk, cont, end Expr
}

var always = &Num{Value: 1}

func or(a, b Expr) Expr {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	if v, ok := static(a); ok && v != 0 {
		return a
	}
	if v, ok := static(b); ok && v != 0 {
		return b
	}
	return &Binary{Op: "||", X: a, Y: b}
}

func isAlways(e Expr) bool {
	v, ok := static(e)
	return ok && v != 0
}

func (r *equipRun) vars(en env) func(*Var) (Expr, error) {
	return func(v *Var) (Expr, error) {
		if x, ok := en[v.Name]; ok {
			return x, nil
		}
		if strings.HasSuffix(v.Name, "$") {
			return &Str{}, nil
		}
		return &Num{Value: 0}, nil
	}
}

func (r *equipRun) resolve(e Expr, en env) (Expr, error) {
	return r.c.resolve(e, scopeEquip, r.vars(en))
}

// block runs statements. It stops at a break, continue or end that
// always happens. One that happens under a condition makes the rest run
// under its negation.
func (r *equipRun) block(stmts []Stmt, en env, guard Expr) (flow, error) {
	var out flow
	for _, s := range stmts {
		f, err := r.stmt(s, en, guard)
		if err != nil {
			return out, err
		}
		out.brk, out.cont, out.end = or(out.brk, f.brk), or(out.cont, f.cont), or(out.end, f.end)
		stop := or(or(f.brk, f.cont), f.end)
		if stop == nil {
			continue
		}
		if isAlways(stop) {
			return out, nil
		}
		guard = and(guard, not(stop))
	}
	return out, nil
}

func (r *equipRun) stmt(s Stmt, en env, guard Expr) (flow, error) {
	r.steps++
	if r.steps > 100000 {
		return flow{}, unsupported("script too long to unroll", "")
	}
	switch s := s.(type) {
	case *Block:
		return r.block(s.Stmts, en, guard)
	case *Assign:
		return flow{}, r.assign(s.Target, s.Op, s.Value, en)
	case *IncDec:
		op := map[string]string{"++": "+=", "--": "-="}[s.Op]
		return flow{}, r.assign(s.Target, op, &Num{Value: 1}, en)
	case *Command:
		return r.command(s, en, guard)
	case *If:
		cond, err := r.resolve(s.Cond, en)
		if err != nil {
			return flow{}, err
		}
		if v, ok := static(cond); ok {
			if v != 0 {
				return r.stmt(s.Then, en, guard)
			}
			if s.Else != nil {
				return r.stmt(s.Else, en, guard)
			}
			return flow{}, nil
		}
		thenEnv := en.clone()
		ft, err := r.stmt(s.Then, thenEnv, and(guard, cond))
		if err != nil {
			return flow{}, err
		}
		elseEnv := en.clone()
		var fe flow
		if s.Else != nil {
			if fe, err = r.stmt(s.Else, elseEnv, and(guard, not(cond))); err != nil {
				return flow{}, err
			}
		}
		merge(en, cond, thenEnv, elseEnv)
		return flow{
			brk:  or(scoped(cond, ft.brk), scoped(not(cond), fe.brk)),
			cont: or(scoped(cond, ft.cont), scoped(not(cond), fe.cont)),
			end:  or(scoped(cond, ft.end), scoped(not(cond), fe.end)),
		}, nil
	case *Switch:
		return r.switchStmt(s, en, guard)
	case *For:
		if s.Init != nil {
			if _, err := r.stmt(s.Init, en, guard); err != nil {
				return flow{}, err
			}
		}
		return r.loop(s.Cond, s.Body, s.Post, false, en, guard)
	case *While:
		return r.loop(s.Cond, s.Body, nil, s.Do, en, guard)
	case *Jump:
		switch s.Kind {
		case "break":
			return flow{brk: always}, nil
		case "continue":
			return flow{cont: always}, nil
		}
		return flow{end: always}, nil
	case *Return:
		return flow{end: always}, nil
	case *Unsupported:
		return flow{}, unsupported("labels and goto", "%s", s.Text)
	}
	return flow{}, unsupported("statement", "%T", s)
}

// scoped is a flow condition inside a branch, seen from outside it.
func scoped(cond, f Expr) Expr {
	if f == nil {
		return nil
	}
	if isAlways(f) {
		return cond
	}
	return and(cond, f)
}

// merge joins the variables of two branches after a condition.
func merge(en env, cond Expr, a, b env) {
	for k := range a {
		if _, ok := b[k]; !ok {
			b[k] = zero(k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			a[k] = zero(k)
		}
	}
	for k, av := range a {
		bv := b[k]
		if same(av, bv) {
			en[k] = av
		} else {
			en[k] = &Ternary{Cond: cond, Then: av, Else: bv}
		}
	}
}

func zero(name string) Expr {
	if strings.HasSuffix(name, "$") {
		return &Str{}
	}
	return &Num{Value: 0}
}

// same reports whether two converted expressions are the same node or
// the same static value.
func same(a, b Expr) bool {
	if a == b {
		return true
	}
	av, aok := static(a)
	bv, bok := static(b)
	if aok && bok {
		return av == bv
	}
	as, aok := a.(*Str)
	bs, bok := b.(*Str)
	return aok && bok && as.Value == bs.Value
}

func (r *equipRun) assign(target *Var, op string, value Expr, en env) error {
	if !strings.HasPrefix(target.Name, ".@") || target.Index != nil {
		return unsupported("variables", "%s keeps state outside the script", target.Name)
	}
	v, err := r.resolve(value, en)
	if err != nil {
		return err
	}
	if op != "=" {
		old, _ := r.vars(en)(target)
		v, err = r.c.binary(strings.TrimSuffix(op, "="), old, v)
		if err != nil {
			return err
		}
	}
	en[target.Name] = v
	return nil
}

// loop unrolls a loop whose condition is static at every iteration.
func (r *equipRun) loop(cond Expr, body, post Stmt, do bool, en env, guard Expr) (flow, error) {
	var out flow
	for i := 0; ; i++ {
		if i > maxLoop {
			return out, unsupported("loop", "more than %d iterations", maxLoop)
		}
		if !do || i > 0 {
			if cond != nil {
				c, err := r.resolve(cond, en)
				if err != nil {
					return out, err
				}
				v, ok := static(c)
				if !ok {
					return out, unsupported("loop", "a condition on game values")
				}
				if v == 0 {
					return out, nil
				}
			}
		}
		f, err := r.stmt(body, en, guard)
		if err != nil {
			return out, err
		}
		out.end = or(out.end, f.end)
		if f.brk != nil && !isAlways(f.brk) || f.cont != nil && !isAlways(f.cont) || f.end != nil && !isAlways(f.end) {
			return out, unsupported("loop", "break, continue or end under a condition on game values")
		}
		if f.brk != nil || f.end != nil {
			return out, nil
		}
		if post != nil {
			if _, err := r.stmt(post, en, guard); err != nil {
				return out, err
			}
		}
	}
}

// switchStmt picks the case of a static value. A switch on a game value
// becomes one guard per case, when every case ends with a break.
func (r *equipRun) switchStmt(s *Switch, en env, guard Expr) (flow, error) {
	v, err := r.resolve(s.Value, en)
	if err != nil {
		return flow{}, err
	}
	if val, ok := static(v); ok {
		start := -1
		for i, cs := range s.Cases {
			for _, cv := range cs.Values {
				x, err := r.resolve(cv, en)
				if err != nil {
					return flow{}, err
				}
				if n, ok := static(x); ok && n == val && start < 0 {
					start = i
				}
			}
		}
		if start < 0 {
			for i, cs := range s.Cases {
				if cs.Default {
					start = i
				}
			}
		}
		if start < 0 {
			return flow{}, nil
		}
		var body []Stmt
		for _, cs := range s.Cases[start:] {
			body = append(body, cs.Body...)
		}
		f, err := r.block(body, en, guard)
		if err != nil {
			return flow{}, err
		}
		// A break leaves the switch only.
		return flow{cont: f.cont, end: f.end}, nil
	}
	var rest Expr = always
	var out flow
	var branches []env
	var conds []Expr
	for _, cs := range s.Cases {
		body := cs.Body
		if n := len(body); n > 0 {
			if j, ok := body[n-1].(*Jump); ok && j.Kind == "break" {
				body = body[:n-1]
			} else if cs != s.Cases[len(s.Cases)-1] {
				return flow{}, unsupported("switch", "a case on a game value that falls through")
			}
		}
		var cond Expr
		if cs.Default {
			cond = rest
		} else {
			for _, cv := range cs.Values {
				x, err := r.resolve(cv, en)
				if err != nil {
					return flow{}, err
				}
				eq, err := r.c.binary("==", v, x)
				if err != nil {
					return flow{}, err
				}
				cond = or(cond, eq)
			}
			cond = and(rest, cond)
		}
		be := en.clone()
		f, err := r.block(body, be, and(guard, cond))
		if err != nil {
			return flow{}, err
		}
		if f.brk != nil || f.cont != nil {
			return flow{}, unsupported("switch", "break or continue inside a case on a game value")
		}
		out.end = or(out.end, scoped(cond, f.end))
		branches = append(branches, be)
		conds = append(conds, cond)
		if !cs.Default {
			rest = and(rest, not(cond))
		}
	}
	// Variables take the value of the case that ran, or keep theirs.
	for i := len(branches) - 1; i >= 0; i-- {
		merged := en.clone()
		merge(merged, conds[i], branches[i], en.clone())
		for k, x := range merged {
			en[k] = x
		}
	}
	return out, nil
}

func (r *equipRun) command(s *Command, en env, guard Expr) (flow, error) {
	name := strings.ToLower(s.Name)
	if name == "end" || name == "close" {
		return flow{end: always}, nil
	}
	var args []Expr
	for i, a := range s.Args {
		// Scripts in strings, such as autobonus's, are kept as they are.
		if strings.HasPrefix(name, "autobonus") && (i == 0 || i == len(s.Args)-1) {
			if str, ok := a.(*Str); ok {
				args = append(args, str)
				continue
			}
		}
		x, err := r.resolve(a, en)
		if err != nil {
			return flow{}, err
		}
		args = append(args, x)
	}
	r.effects = append(r.effects, effect{guard: guard, cmd: s, args: args})
	return flow{}, nil
}

// equipScript runs an equip script symbolically.
func (c *itemConv) equipScript(src string) ([]effect, error) {
	return c.equipScriptEnv(src, env{})
}

// equipScriptEnv runs a script symbolically with variables already set.
func (c *itemConv) equipScriptEnv(src string, en env) ([]effect, error) {
	b, err := Parse(src)
	if err != nil {
		return nil, &unsupportedError{Feature: "parse error", Detail: err.Error()}
	}
	r := &equipRun{c: c}
	if _, err := r.block(b.Stmts, en.clone(), nil); err != nil {
		return nil, err
	}
	return r.effects, nil
}
