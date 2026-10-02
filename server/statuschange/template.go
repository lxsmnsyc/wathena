package statuschange

import (
	"fmt"

	"github.com/lxsmnsyc/wathena/server/bonus"
)

// Template is one bonus of a status script in status.yml, such as
// "bonus bStr, getstatus(SC_INCSTR, 1);". Its numbers may read the
// status's values, so they are expressions. The script converter
// (tools/convert) makes templates from the scripts.
type Template struct {
	// Name is the bonus name of proposal 0002a, and Args its numbers in
	// order.
	Name string
	Args []*Expr
	// Options are the options of bonuses.add, checked.
	Options bonus.Options
	// Unless is a status whose presence turns the bonus off, as a script
	// with "if (!getstatus(SC_BERSERK))", or -1.
	Unless int
}

// Expr is a number of a template.
type Expr struct {
	// Op is "num" for N, "val" for value N of the status (1 to 4),
	// "skill" for the level of skill N, "neg" for -X, or one of + - * /
	// max min with X and Y.
	Op   string
	N    int64
	X, Y *Expr
}

// Num is an expression for a number.
func Num(n int64) *Expr { return &Expr{Op: "num", N: n} }

// Eval computes an expression with the status's values, as rAthena's
// script engine does in int32 arithmetic. skill gives a skill's level.
func (e *Expr) Eval(vals [4]int, skill func(int) int) (int64, error) {
	switch e.Op {
	case "num":
		return e.N, nil
	case "val":
		if e.N < 1 || e.N > 4 {
			return 0, fmt.Errorf("value %d of a status", e.N)
		}
		return int64(vals[e.N-1]), nil
	case "skill":
		if skill == nil {
			return 0, nil
		}
		return int64(skill(int(e.N))), nil
	case "neg":
		x, err := e.X.Eval(vals, skill)
		return int64(int32(-x)), err // #nosec G115 -- the script engine wraps in int32.
	}
	x, err := e.X.Eval(vals, skill)
	if err != nil {
		return 0, err
	}
	y, err := e.Y.Eval(vals, skill)
	if err != nil {
		return 0, err
	}
	var r int64
	switch e.Op {
	case "+":
		r = x + y
	case "-":
		r = x - y
	case "*":
		r = x * y
	case "/":
		if y == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		r = x / y
	case "max":
		r = max(x, y)
	case "min":
		r = min(x, y)
	default:
		return 0, fmt.Errorf("unknown operation %q", e.Op)
	}
	return int64(int32(r)), nil // #nosec G115 -- the script engine wraps in int32.
}

// String writes an expression for reports and tests.
func (e *Expr) String() string {
	switch e.Op {
	case "num":
		return fmt.Sprint(e.N)
	case "val":
		return fmt.Sprintf("val%d", e.N)
	case "skill":
		return fmt.Sprintf("skill(%d)", e.N)
	case "neg":
		return "-" + e.X.String()
	case "max", "min":
		return fmt.Sprintf("%s(%s, %s)", e.Op, e.X, e.Y)
	}
	return fmt.Sprintf("(%s %s %s)", e.X, e.Op, e.Y)
}

// Bonus is a template's bonus with its numbers computed.
type Bonus struct {
	Name    string
	Args    []int64
	Options *bonus.Options
}

// ScriptBonuses runs the templates of every active status with a
// script, in status order, as status_calc_pc_sub runs status scripts.
// skill gives the player's skill levels.
func (l *List) ScriptBonuses(db *DB, skill func(int) int) []Bonus {
	var out []Bonus
	for _, e := range l.Sorted() {
		def := db.Get(e.Type)
		if def == nil {
			continue
		}
		vals := [4]int{e.Val1, e.Val2, e.Val3, e.Val4}
		for i := range def.Bonuses {
			t := &def.Bonuses[i]
			if t.Unless >= 0 && l.Has(t.Unless) {
				continue
			}
			b := Bonus{Name: t.Name, Options: &t.Options}
			ok := true
			for _, a := range t.Args {
				v, err := a.Eval(vals, skill)
				if err != nil {
					ok = false
					break
				}
				b.Args = append(b.Args, v)
			}
			if ok {
				out = append(out, b)
			}
		}
		out = append(out, e.Bonuses...)
	}
	return out
}
