package convert

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/reference"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Status scripts: the Script field of status.yml, such as
// "bonus bStr, getstatus(SC_INCSTR, 1);", runs in status_calc_pc_sub for
// each active status. They convert to bonus templates
// (statuschange.Template): the bonuses of proposal 0002a, with numbers
// that read the status's values. The scripts only add bonuses, compute
// them from getstatus and getskilllv, and skip them while another status
// is active (if (!getstatus(SC_BERSERK))).

// StatusScript is the conversion of one status's script.
type StatusScript struct {
	Status string
	// Bonuses are the templates. Err says what did not convert; a status
	// whose script fails keeps no bonuses.
	Bonuses []sc.Template
	Err     error
}

// tmplExpr is a number of a status script that is not static: it reads
// the status's values or a skill level. It carries its template through
// the bonus mapping.
type tmplExpr struct{ t *sc.Expr }

func (*tmplExpr) expr() {}

// StatusBonuses converts the scripts of a status database's core
// statuses, by status ID.
func (r *Result) StatusBonuses(db *sc.DB) []StatusScript {
	var out []StatusScript
	for _, d := range db.Core() {
		if d.Script == "" {
			continue
		}
		ic := &itemConv{conv: r.conv, file: newLuaFile(), helpers: map[string]bool{},
			item: &Item{Features: map[string]bool{}}}
		st := &statusConv{ic: ic, self: d.ID, vars: map[string]Expr{}}
		res := StatusScript{Status: d.Name}
		res.Bonuses, res.Err = st.script(d.Script)
		out = append(out, res)
	}
	return out
}

// ApplyStatusBonuses converts the status scripts and sets the bonuses of
// each status that converts. It returns the scripts that do not.
func (r *Result) ApplyStatusBonuses(db *sc.DB) ([]StatusScript, error) {
	var failed []StatusScript
	for _, s := range r.StatusBonuses(db) {
		if s.Err != nil {
			failed = append(failed, s)
			continue
		}
		d := db.ByName(s.Status)
		if d == nil {
			return nil, fmt.Errorf("no status %s", s.Status)
		}
		if err := db.SetBonuses(d.ID, s.Bonuses); err != nil {
			return nil, err
		}
	}
	sort.Slice(failed, func(i, j int) bool { return failed[i].Status < failed[j].Status })
	return failed, nil
}

type statusConv struct {
	ic   *itemConv
	self int
	vars map[string]Expr
}

func (s *statusConv) script(src string) ([]sc.Template, error) {
	b, err := Parse(src)
	if err != nil {
		return nil, err
	}
	return s.block(b.Stmts, -1)
}

func (s *statusConv) block(stmts []Stmt, unless int) ([]sc.Template, error) {
	var out []sc.Template
	for _, st := range stmts {
		t, err := s.stmt(st, unless)
		if err != nil {
			return nil, err
		}
		out = append(out, t...)
	}
	return out, nil
}

func (s *statusConv) stmt(st Stmt, unless int) ([]sc.Template, error) {
	switch st := st.(type) {
	case *Block:
		return s.block(st.Stmts, unless)
	case *Assign:
		if st.Op != "=" || st.Target.Index != nil || !strings.HasPrefix(st.Target.Name, ".@") {
			return nil, unsupported("variables", "%s", st.Target.Name)
		}
		v, err := s.expr(st.Value)
		if err != nil {
			return nil, err
		}
		s.vars[st.Target.Name] = v
		return nil, nil
	case *If:
		// Only "if (!getstatus(SC_X)) { ... }" converts.
		u, ok := st.Cond.(*Unary)
		if !ok || u.Op != "!" || st.Else != nil || unless >= 0 {
			return nil, unsupported("condition", "")
		}
		c, ok := u.X.(*Call)
		if !ok || !strings.EqualFold(c.Name, "getstatus") || len(c.Args) != 1 {
			return nil, unsupported("condition", "")
		}
		id, ok := s.constant(c.Args[0])
		if !ok {
			return nil, unsupported("condition", "unknown status")
		}
		return s.stmt(st.Then, int(id))
	case *Command:
		name := strings.ToLower(st.Name)
		if !strings.HasPrefix(name, "bonus") {
			return nil, unsupported(name, "")
		}
		n := 1
		if name != "bonus" {
			if len(name) != 6 || name[5] < '2' || name[5] > '5' {
				return nil, unsupported(name, "")
			}
			n = int(name[5] - '0')
		}
		if len(st.Args) == 0 {
			return nil, unsupported(name, "no bonus type")
		}
		args := []Expr{st.Args[0]}
		if id, ok := s.ident(st.Args[0]); ok {
			args[0] = id
		}
		for _, a := range st.Args[1:] {
			v, err := s.expr(a)
			if err != nil {
				return nil, err
			}
			args = append(args, v)
		}
		calls, err := s.ic.mapBonus(n, args)
		if err != nil {
			return nil, err
		}
		var out []sc.Template
		for _, w := range calls {
			t, err := s.template(w, unless)
			if err != nil {
				return nil, err
			}
			out = append(out, t)
		}
		return out, nil
	}
	return nil, unsupported("statement", "%T", st)
}

// ident resolves a constant, such as a bonus type or Ele_Water.
func (s *statusConv) ident(e Expr) (Expr, bool) {
	if id, ok := e.(*Ident); ok {
		if v, param, ok := s.ic.conv.consts.Lookup(id.Name); ok && !param {
			return &Const{Name: id.Name, Value: v}, true
		}
	}
	return nil, false
}

func (s *statusConv) constant(e Expr) (int64, bool) {
	if c, ok := s.ident(e); ok {
		return c.(*Const).Value, true
	}
	return 0, false
}

// expr turns a script number into a converter expression: a static
// number, or a tmplExpr.
func (s *statusConv) expr(e Expr) (Expr, error) {
	switch e := e.(type) {
	case *Num, *Str:
		return e, nil
	case *Ident:
		if c, ok := s.ident(e); ok {
			return c, nil
		}
		return nil, unsupported("variables", "%s", e.Name)
	case *Var:
		v, ok := s.vars[e.Name]
		if !ok || e.Index != nil {
			return nil, unsupported("variables", "%s", e.Name)
		}
		return v, nil
	case *Unary:
		x, err := s.expr(e.X)
		if err != nil {
			return nil, err
		}
		if e.Op != "-" {
			return nil, unsupported("expression", "%s", e.Op)
		}
		return s.ic.neg(x), nil
	case *Binary:
		x, err := s.expr(e.X)
		if err != nil {
			return nil, err
		}
		y, err := s.expr(e.Y)
		if err != nil {
			return nil, err
		}
		if !strings.Contains("+-*/", e.Op) || len(e.Op) != 1 {
			return nil, unsupported("expression", "%s", e.Op)
		}
		return s.ic.binary(e.Op, x, y)
	case *Call:
		switch strings.ToLower(e.Name) {
		case "getstatus":
			if len(e.Args) != 2 {
				return nil, unsupported("getstatus", "")
			}
			id, ok := s.constant(e.Args[0])
			n, ok2 := static(e.Args[1])
			if !ok || !ok2 || int(id) != s.self || n < 1 || n > 4 {
				return nil, unsupported("getstatus", "of another status")
			}
			return &tmplExpr{&sc.Expr{Op: "val", N: n}}, nil
		case "getskilllv":
			if len(e.Args) != 1 {
				return nil, unsupported("getskilllv", "")
			}
			str, ok := e.Args[0].(*Str)
			if !ok {
				return nil, unsupported("getskilllv", "")
			}
			id, ok := s.ic.conv.consts.Skill(str.Value)
			if !ok {
				return nil, unsupported("getskilllv", "unknown skill %s", str.Value)
			}
			return &tmplExpr{&sc.Expr{Op: "skill", N: id}}, nil
		case "max", "min":
			if len(e.Args) != 2 {
				return nil, unsupported(e.Name, "")
			}
			x, err := s.expr(e.Args[0])
			if err != nil {
				return nil, err
			}
			y, err := s.expr(e.Args[1])
			if err != nil {
				return nil, err
			}
			tx, err := toTemplate(x)
			if err != nil {
				return nil, err
			}
			ty, err := toTemplate(y)
			if err != nil {
				return nil, err
			}
			return &tmplExpr{&sc.Expr{Op: strings.ToLower(e.Name), X: tx, Y: ty}}, nil
		}
		return nil, unsupported(e.Name, "")
	}
	return nil, unsupported("expression", "%T", e)
}

// toTemplate turns a converter expression back into a template.
func toTemplate(e Expr) (*sc.Expr, error) {
	switch e := e.(type) {
	case *Num:
		return sc.Num(e.Value), nil
	case *Const:
		return sc.Num(e.Value), nil
	case *tmplExpr:
		return e.t, nil
	case *Unary:
		if e.Op == "-" {
			x, err := toTemplate(e.X)
			if err != nil {
				return nil, err
			}
			return &sc.Expr{Op: "neg", X: x}, nil
		}
	case *Binary:
		x, err := toTemplate(e.X)
		if err != nil {
			return nil, err
		}
		y, err := toTemplate(e.Y)
		if err != nil {
			return nil, err
		}
		if len(e.Op) == 1 && strings.Contains("+-*/", e.Op) {
			return &sc.Expr{Op: e.Op, X: x, Y: y}, nil
		}
	}
	return nil, unsupported("expression", "%T", e)
}

// template makes a template of one bonuses.add call, checking it as the
// item converter does. The check sees 1 for each number that reads the
// status: a template computes every number when stats are computed, so
// numbers that Lua keeps static, such as the interval of
// hp_gain_over_time, may read the status too.
func (s *statusConv) template(w *wcall, unless int) (sc.Template, error) {
	check := *w
	check.args = make([]warg, len(w.args))
	for i, a := range w.args {
		check.args[i] = a
		if !isStaticValue(a.v) {
			check.args[i].v = &Num{Value: 1}
		}
	}
	b, err := s.ic.toBonus(&check)
	if err != nil {
		return sc.Template{}, err
	}
	t := sc.Template{Name: w.name, Options: b.Options, Unless: unless}
	for i, a := range w.args {
		if isStaticValue(a.v) {
			t.Args = append(t.Args, sc.Num(b.Args[i].Value))
			continue
		}
		x, err := toTemplate(a.v)
		if err != nil {
			return sc.Template{}, err
		}
		t.Args = append(t.Args, x)
	}
	return t, nil
}

// LoadStatuses loads a mode's status database from status.json, with
// the bonuses of its status scripts converted. It returns the scripts
// that do not convert, whose statuses keep no bonuses.
func (c *Core) LoadStatuses(mode reference.Mode) (*sc.DB, []StatusScript, error) {
	var d sc.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/status.json", &d); err != nil {
		return nil, nil, err
	}
	db, err := sc.Load(&d)
	if err != nil {
		return nil, nil, err
	}
	failed, err := c.Result.ApplyStatusBonuses(db)
	if err != nil {
		return nil, nil, err
	}
	return db, failed, nil
}
