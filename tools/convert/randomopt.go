package convert

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/server/bonus"
)

// Random options (proposal 0002, review decision 23): rAthena's
// item_randomopt_db.yml and item_randomopt_group.yml, read from the
// random_options.json extract, become core's data/random_options.yml and
// data/random_option_groups.yml.
//
//   - Each option's script converts to bonus entries, as a status script
//     does (status.go). getrandomoptinfo(ROA_VALUE) becomes { roll: value }
//     and ROA_PARAM { roll: param }. A division or a minus around it
//     becomes divide or multiply: -1.
//   - An option's readable name comes from its first bonus: the kind, the
//     bonus name, percent, the range, the element and the target, such as
//     physical_damage_against_boss for CLASS_DAMAGE_BOSS_USER. An option
//     whose bonuses do not name it alone keeps rAthena's name in lower
//     case.
//   - Its display_name is bonus.Describe of the first bonus.
//   - Groups keep rAthena's names, and their entries name options by
//     their readable names.

// RandomOptionData is the random_options.json extract.
type RandomOptionData struct {
	Mode    string `json:"mode"`
	Options []struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Script string `json:"script"`
	} `json:"options"`
	Groups []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Slots []struct {
			Slot    int              `json:"slot"`
			Options []randomOptEntry `json:"options"`
		} `json:"slots"`
		MaxRandom int              `json:"max_random"`
		Random    []randomOptEntry `json:"random"`
	} `json:"groups"`
	Drops []RandomOptionDrop `json:"drops"`
}

type randomOptEntry struct {
	Option string `json:"option"`
	Min    int    `json:"min"`
	Max    int    `json:"max"`
	Param  int    `json:"param"`
	Chance int    `json:"chance"`
}

// RandomOptionDrop is a monster drop with a random option group.
type RandomOptionDrop struct {
	Monster uint32 `json:"monster"`
	MVP     bool   `json:"mvp"`
	Index   int    `json:"index"`
	Item    uint32 `json:"item"`
	Group   string `json:"group"`
}

// RandomOptionResult is the converted options and groups of one mode.
type RandomOptionResult struct {
	// Options are the converted options in rAthena ID order. Failed are
	// those that do not convert, with why.
	Options []ConvertedOption
	Failed  []FailedOption
	// Names maps rAthena's option names to readable names.
	Names map[string]string
	// Groups is the YAML of data/random_option_groups.yml.
	groups string
}

// ConvertedOption is one option of data/random_options.yml.
type ConvertedOption struct {
	ID          int
	RAthena     string
	Name        string
	DisplayName string
	// Bonus are the YAML entries of its bonus list.
	Bonus []string
}

// FailedOption is an option whose script does not convert.
type FailedOption struct {
	RAthena string
	Err     error
}

// optExpr is getrandomoptinfo(ROA_VALUE) or (ROA_PARAM) in an option's
// script: name is value or param.
type optExpr struct{ name string }

func (*optExpr) expr() {}

// ConvertRandomOptions converts the random options of a mode's extract.
// r supplies the constants and the bonus mapping of the item converter.
func (r *Result) ConvertRandomOptions(in io.Reader) (*RandomOptionResult, error) {
	var d RandomOptionData
	if err := json.NewDecoder(in).Decode(&d); err != nil {
		return nil, fmt.Errorf("random_options.json: %w", err)
	}
	out := &RandomOptionResult{Names: map[string]string{}}
	type pending struct{ opt ConvertedOption }
	var all []pending
	byName := map[string][]int{}
	for _, o := range d.Options {
		ic := &itemConv{conv: r.conv, file: newLuaFile(), helpers: map[string]bool{}, item: &Item{Features: map[string]bool{}}}
		calls, err := optionCalls(ic, o.Script)
		if err == nil && len(calls) == 0 {
			err = unsupported("script", "no bonus")
		}
		var entries []string
		var desc string
		for i, w := range calls {
			if err != nil {
				break
			}
			var e string
			var b *bonus.Bonus
			e, b, err = optionEntry(ic, w)
			entries = append(entries, e)
			if i == 0 && b != nil {
				desc, _ = bonus.Describe(b, r.conv.constName)
			}
		}
		if err != nil {
			out.Failed = append(out.Failed, FailedOption{RAthena: o.Name, Err: err})
			continue
		}
		name := strings.ToLower(o.Name)
		if n, ok := multiNames[o.Name]; ok {
			name, desc = n[0], n[1]
		} else if len(calls) == 1 {
			name = optionName(ic, calls[0])
		}
		p := pending{opt: ConvertedOption{ID: o.ID, RAthena: o.Name, Name: name, DisplayName: desc, Bonus: entries}}
		byName[name] = append(byName[name], len(all))
		all = append(all, p)
	}
	// A name two options share falls back to rAthena's names.
	for _, idx := range byName {
		if len(idx) > 1 {
			for _, i := range idx {
				all[i].opt.Name = strings.ToLower(all[i].opt.RAthena)
			}
		}
	}
	for _, p := range all {
		out.Options = append(out.Options, p.opt)
		out.Names[p.opt.RAthena] = p.opt.Name
	}
	// Groups.
	var g strings.Builder
	g.WriteString("# Random option groups converted from rAthena's item_randomopt_group.yml by the server.\n")
	g.WriteString("# Generated; do not edit. See mods/README.md, \"Random options\".\n")
	entry := func(e randomOptEntry) (string, error) {
		n, ok := out.Names[e.Option]
		if !ok {
			return "", fmt.Errorf("option %s does not convert", e.Option)
		}
		parts := []string{"option: " + yamlString(n)}
		if e.Min != 0 || e.Max != 0 {
			parts = append(parts, "min: "+strconv.Itoa(e.Min), "max: "+strconv.Itoa(e.Max))
		}
		if e.Param != 0 {
			parts = append(parts, "param: "+strconv.Itoa(e.Param))
		}
		parts = append(parts, "chance: "+strconv.Itoa(e.Chance))
		return "{" + strings.Join(parts, ", ") + "}", nil
	}
	for _, grp := range d.Groups {
		var lines []string
		ok := true
		var slots []string
		for _, s := range grp.Slots {
			var es []string
			for _, e := range s.Options {
				x, err := entry(e)
				if err != nil {
					ok = false
					break
				}
				es = append(es, x)
			}
			slots = append(slots, "    - ["+strings.Join(es, ", ")+"]")
		}
		var random []string
		for _, e := range grp.Random {
			x, err := entry(e)
			if err != nil {
				ok = false
				break
			}
			random = append(random, "    - "+x)
		}
		if !ok {
			// A group with an option that does not convert is left out:
			// drops with it roll no options.
			continue
		}
		lines = append(lines, "- name: "+yamlString(grp.Name))
		if len(slots) > 0 {
			lines = append(lines, "  slots:")
			lines = append(lines, slots...)
		}
		if grp.MaxRandom > 0 && len(random) > 0 {
			lines = append(lines, "  max_random: "+strconv.Itoa(grp.MaxRandom), "  random:")
			lines = append(lines, random...)
		}
		g.WriteString(strings.Join(lines, "\n") + "\n")
	}
	out.groups = g.String()
	return out, nil
}

// multiNames name the options that give several bonuses, with their
// display names.
var multiNames = map[string][2]string{
	"ATTR_TOLERACE_ALLBUTNOTHING": {"all_but_neutral_damage_taken", "Damage taken from every element but Neutral"},
}

// Files are data/random_options.yml and data/random_option_groups.yml.
func (o *RandomOptionResult) Files() map[string]string {
	var b strings.Builder
	b.WriteString("# Random options converted from rAthena's item_randomopt_db.yml by the server.\n")
	b.WriteString("# Generated; do not edit. See mods/README.md, \"Random options\".\n")
	for _, opt := range o.Options {
		fmt.Fprintf(&b, "- {name: %s, id: %d, display_name: %s, bonus: [%s]}\n", yamlString(opt.Name), opt.ID,
			yamlString(opt.DisplayName), strings.Join(opt.Bonus, ", "))
	}
	return map[string]string{"data/random_options.yml": b.String(), "data/random_option_groups.yml": o.groups}
}

// optionCalls converts an option's script to bonuses.add calls.
func optionCalls(ic *itemConv, src string) ([]*wcall, error) {
	b, err := Parse(src)
	if err != nil {
		return nil, err
	}
	vars := map[string]Expr{}
	var out []*wcall
	var stmts func([]Stmt) error
	var stmt func(Stmt) error
	stmt = func(st Stmt) error {
		switch st := st.(type) {
		case *Block:
			return stmts(st.Stmts)
		case *Command:
			name := strings.ToLower(st.Name)
			n := 1
			if name != "bonus" {
				if len(name) != 6 || !strings.HasPrefix(name, "bonus") || name[5] < '2' || name[5] > '5' {
					return unsupported(name, "")
				}
				n = int(name[5] - '0')
			}
			var args []Expr
			for i, a := range st.Args {
				v, err := optValue(ic, a, vars)
				if err != nil {
					return err
				}
				if i == 0 {
					if id, ok := a.(*Ident); ok {
						if c, _, ok := ic.conv.consts.Lookup(id.Name); ok {
							v = &Const{Name: id.Name, Value: c}
						}
					}
				}
				args = append(args, v)
			}
			calls, err := ic.mapBonus(n, args)
			if err != nil {
				return err
			}
			out = append(out, calls...)
			return nil
		case *For:
			// for (.@i = A; .@i < B; ++.@i) body, unrolled.
			init, ok := st.Init.(*Assign)
			cond, ok2 := st.Cond.(*Binary)
			if !ok || !ok2 || init.Op != "=" || cond.Op != "<" {
				return unsupported("for", "")
			}
			from, err := optValue(ic, init.Value, vars)
			if err != nil {
				return err
			}
			to, err := optValue(ic, cond.Y, vars)
			if err != nil {
				return err
			}
			lo, ok3 := static(from)
			hi, ok4 := static(to)
			if !ok3 || !ok4 || hi-lo > 64 {
				return unsupported("for", "bounds")
			}
			for i := lo; i < hi; i++ {
				vars[init.Target.Name] = &Num{Value: i}
				if err := stmt(st.Body); err != nil {
					return err
				}
			}
			delete(vars, init.Target.Name)
			return nil
		}
		return unsupported("statement", "%T", st)
	}
	stmts = func(list []Stmt) error {
		for _, s := range list {
			if err := stmt(s); err != nil {
				return err
			}
		}
		return nil
	}
	if err := stmts(b.Stmts); err != nil {
		return nil, err
	}
	return out, nil
}

// optValue resolves an argument of an option's script.
func optValue(ic *itemConv, e Expr, vars map[string]Expr) (Expr, error) {
	switch e := e.(type) {
	case *Num, *Str:
		return e, nil
	case *Ident:
		if v, param, ok := ic.conv.consts.Lookup(e.Name); ok && !param {
			return &Const{Name: e.Name, Value: v}, nil
		}
		return nil, unsupported("variables", "%s", e.Name)
	case *Var:
		if v, ok := vars[e.Name]; ok && e.Index == nil {
			return v, nil
		}
		return nil, unsupported("variables", "%s", e.Name)
	case *Call:
		if strings.EqualFold(e.Name, "getrandomoptinfo") && len(e.Args) == 1 {
			if id, ok := e.Args[0].(*Ident); ok {
				switch strings.ToUpper(id.Name) {
				case "ROA_VALUE":
					return &optExpr{"value"}, nil
				case "ROA_PARAM":
					return &optExpr{"param"}, nil
				}
			}
		}
		return nil, unsupported(e.Name, "")
	case *Unary:
		x, err := optValue(ic, e.X, vars)
		if err != nil {
			return nil, err
		}
		if e.Op != "-" {
			return nil, unsupported("expression", "%s", e.Op)
		}
		return ic.neg(x), nil
	case *Binary:
		x, err := optValue(ic, e.X, vars)
		if err != nil {
			return nil, err
		}
		y, err := optValue(ic, e.Y, vars)
		if err != nil {
			return nil, err
		}
		if e.Op != "*" && e.Op != "/" && e.Op != "+" && e.Op != "-" {
			return nil, unsupported("expression", "%s", e.Op)
		}
		return ic.binary(e.Op, x, y)
	}
	return nil, unsupported("expression", "%T", e)
}

// rollOf reads a number that is the option's value or param, scaled: a
// minus, a multiplication or a division by a constant.
func rollOf(e Expr) (name string, mul, div int64, ok bool) {
	switch e := e.(type) {
	case *optExpr:
		return e.name, 1, 0, true
	case *Unary:
		if e.Op == "-" {
			n, m, d, ok := rollOf(e.X)
			return n, -m, d, ok
		}
	case *Binary:
		switch e.Op {
		case "-":
			if z, isStatic := static(e.X); isStatic && z == 0 {
				n, m, d, ok := rollOf(e.Y)
				return n, -m, d, ok
			}
		case "*":
			if k, isStatic := static(e.Y); isStatic {
				n, m, d, ok := rollOf(e.X)
				return n, m * k, d, ok && d == 0
			}
			if k, isStatic := static(e.X); isStatic {
				n, m, d, ok := rollOf(e.Y)
				return n, m * k, d, ok && d == 0
			}
		case "/":
			if k, isStatic := static(e.Y); isStatic && k != 0 {
				n, m, d, ok := rollOf(e.X)
				return n, m, k, ok && d == 0
			}
		}
	}
	return "", 0, 0, false
}

// optionEntry writes one call as a data entry, with { roll: value } for
// the option's value, and returns the checked bonus.
func optionEntry(ic *itemConv, w *wcall) (string, *bonus.Bonus, error) {
	check := *w
	check.args = slices.Clone(w.args)
	rolls := map[int]string{}
	for i, a := range w.args {
		if isStaticValue(a.v) {
			continue
		}
		name, mul, div, ok := rollOf(a.v)
		if !ok || a.kind != kNum {
			return "", nil, unsupported("computed argument", "")
		}
		s := "{roll: " + name
		if mul != 1 {
			s += ", multiply: " + strconv.FormatInt(mul, 10)
		}
		if div != 0 {
			s += ", divide: " + strconv.FormatInt(div, 10)
		}
		rolls[i] = s + "}"
		check.args[i].v = &Num{Value: 1}
	}
	b, err := ic.toBonus(&check)
	if err != nil {
		return "", nil, err
	}
	data, err := ic.dataBonus(&check)
	if err != nil {
		return "", nil, err
	}
	if len(rolls) == 0 {
		return data, b, nil
	}
	// Put the rolls in place of the checked 1s: rebuild the list.
	parts := []string{w.name}
	for i, a := range w.args {
		if s, ok := rolls[i]; ok {
			parts = append(parts, s)
			continue
		}
		s, err := ic.dataRef(a.kind, a.v)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, s)
	}
	full := "[" + strings.Join(parts, ", ")
	// The options map is the last part of the checked entry.
	if i := strings.Index(data, ", {"); i >= 0 && strings.HasSuffix(data, "}]") && len(w.opts) > 0 {
		full += data[i : len(data)-1]
	}
	return full + "]", b, nil
}

// optionName names an option from its first bonus: the range, the
// element and the kind, the bonus name, percent, and the target, such as
// fire_damage_taken or physical_damage_against_undead_element.
func optionName(ic *itemConv, w *wcall) string {
	var parts []string
	optVal := func(key string) (wopt, bool) {
		for _, o := range w.opts {
			if o.key == key {
				return o, true
			}
		}
		return wopt{}, false
	}
	if o, ok := optVal("range"); ok && o.enum != "" {
		parts = append(parts, o.enum)
	}
	if o, ok := optVal("element"); ok {
		if s, err := ic.dataRef(o.kind, o.v); err == nil {
			parts = append(parts, s)
		}
	}
	if o, ok := optVal("kind"); ok && o.enum != "" {
		parts = append(parts, o.enum)
	}
	parts = append(parts, w.name)
	for _, a := range w.args {
		if a.kind != kNum && a.kind != kLevel {
			if s, err := ic.dataRef(a.kind, a.v); err == nil {
				parts = append(parts, strings.Trim(s, `"`))
			}
		}
	}
	if _, ok := optVal("percent"); ok {
		parts = append(parts, "percent")
	}
	for _, key := range []string{"against", "from"} {
		if o, ok := optVal(key); ok {
			if s, err := ic.dataRef(o.kind, o.v); err == nil {
				parts = append(parts, key, s)
				switch o.tkey {
				case "element", "size":
					parts = append(parts, o.tkey)
				}
			}
		}
	}
	return strings.ToLower(strings.Join(parts, "_"))
}

// constName is the readable name of a constant of a module, for
// bonus.Describe.
func (c *converter) constName(module string, v int64) string {
	if m := c.modules[module]; m != nil {
		return m.Names[v]
	}
	return ""
}

// Report is the coverage report of a mode's random options: each option
// with its readable name and display name, and those that do not
// convert.
func (o *RandomOptionResult) Report(mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Random options (%s)\n\n", mode)
	b.WriteString("Generated by `wactl convert random-options`; do not edit. rAthena's item_randomopt_db.yml becomes core's data/random_options.yml (mods/README.md, \"Random options\").\n\n")
	fmt.Fprintf(&b, "%d of %d options convert. Their groups are in data/random_option_groups.yml.\n", len(o.Options), len(o.Options)+len(o.Failed))
	if len(o.Failed) > 0 {
		b.WriteString("\n## Options that do not convert\n\n| rAthena | Why |\n|---|---|\n")
		for _, f := range o.Failed {
			fmt.Fprintf(&b, "| `%s` | %s |\n", f.RAthena, strings.ReplaceAll(f.Err.Error(), "|", "\\|"))
		}
	}
	if len(o.Options) > 0 {
		b.WriteString("\n## Options\n\n| ID | rAthena | Name | Shows as |\n|---:|---|---|---|\n")
		for _, opt := range o.Options {
			fmt.Fprintf(&b, "| %d | `%s` | `%s` | %s |\n", opt.ID, opt.RAthena, opt.Name, opt.DisplayName)
		}
	}
	return b.String()
}
