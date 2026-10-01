package convert

import (
	"fmt"
	"strings"
)

// Expressions and commands of NPC scripts.

// expr converts an expression.
func (b *body) expr(e Expr) (val, error) {
	switch e := e.(type) {
	case *Num:
		return intVal(e.Value), nil
	case *Str:
		return strVal(e.Value), nil
	case *Var:
		return b.readVar(e.Name, e.Index)
	case *Ident:
		return b.ident(e.Name)
	case *Call:
		return b.call(e)
	case *Unary:
		return b.unary(e)
	case *Binary:
		return b.binary(e)
	case *Ternary:
		c, err := b.expr(e.Cond)
		if err != nil {
			return val{}, err
		}
		if c.static != nil {
			if *c.static != 0 {
				return b.expr(e.Then)
			}
			return b.expr(e.Else)
		}
		x, err := b.expr(e.Then)
		if err != nil {
			return val{}, err
		}
		y, err := b.expr(e.Else)
		if err != nil {
			return val{}, err
		}
		t := x.t
		if x.t != y.t {
			t = tAny
		}
		if x.t == tInt || x.t == tStr {
			return val{code: fmt.Sprintf("(%s and %s or %s)", c.asBool(), x.code, y.code), t: t}, nil
		}
		return val{code: fmt.Sprintf("(function() if %s then return %s end return %s end)()", c.asBool(), x.code, y.code), t: t}, nil
	case *Assign, *IncDec:
		return val{}, unsupported("assignment in an expression", "")
	}
	return val{}, unsupported("expression", "%T", e)
}

// ident converts a bare name: a constant, a parameter, or a character
// variable.
func (b *body) ident(name string) (val, error) {
	c := b.n.c
	if v, param, ok := c.consts.Lookup(name); ok {
		if param {
			return b.param(name)
		}
		lower := strings.ToLower(name)
		switch {
		case lower == "vip_script":
			// VIP accounts do not exist.
			return intVal(0), nil
		case strings.HasPrefix(lower, "job_"):
			if r, ok := c.modules["jobs"].Names[v]; ok {
				b.n.file.need("jobs")
				return val{code: "jobs." + r, t: tInt, static: &v}, nil
			}
		}
		return intVal(v), nil
	}
	if _, ok := c.consts.Skill(name); ok {
		return strVal(name), nil
	}
	return b.readVar(name, nil)
}

// param reads a parameter of the player, as rAthena's readparam.
func (b *body) param(name string) (val, error) {
	f := b.n.file
	switch strings.ToLower(name) {
	case "zeny":
		return val{code: "player.zeny", t: tInt}, nil
	case "baselevel":
		return val{code: "player.base_level", t: tInt}, nil
	case "joblevel":
		return val{code: "player.job_level", t: tInt}, nil
	case "class":
		return val{code: "player.job", t: tInt}, nil
	case "baseclass":
		f.need("jobs")
		return val{code: "jobs.first_job(player.job)", t: tInt}, nil
	case "basejob":
		f.need("jobs")
		return val{code: "jobs.base_job(player.job)", t: tInt}, nil
	case "sex":
		return val{code: `(player.sex == "male" and 1 or 0)`, t: tInt}, nil
	case "hp":
		return val{code: "player.hp", t: tInt}, nil
	case "maxhp":
		return val{code: "player.max_hp", t: tInt}, nil
	case "sp":
		return val{code: "player.sp", t: tInt}, nil
	case "maxsp":
		return val{code: "player.max_sp", t: tInt}, nil
	}
	return val{}, unsupported("parameter "+name, "")
}

func (b *body) unary(e *Unary) (val, error) {
	x, err := b.expr(e.X)
	if err != nil {
		return val{}, err
	}
	if x.static != nil {
		v := *x.static
		switch e.Op {
		case "-":
			return intVal(-v), nil
		case "!":
			return intVal(b2i(v == 0)), nil
		case "~":
			return intVal(^v), nil
		}
	}
	switch e.Op {
	case "-":
		return val{code: "(-" + x.asInt() + ")", t: tInt}, nil
	case "!":
		return val{code: "(not " + x.asBool() + ")", t: tBool}, nil
	case "~":
		return val{code: "(~" + x.asInt() + ")", t: tInt}, nil
	}
	return val{}, unsupported("operator "+e.Op, "")
}

func (b *body) binary(e *Binary) (val, error) {
	x, err := b.expr(e.X)
	if err != nil {
		return val{}, err
	}
	// && and || stop early when the left side is known, so a part for a
	// setting that is off need not convert.
	if x.static != nil && (e.Op == "&&" && *x.static == 0 || e.Op == "||" && *x.static != 0) {
		return intVal(b2i(e.Op == "||")), nil
	}
	y, err := b.expr(e.Y)
	if err != nil {
		return val{}, err
	}
	if x.static != nil && y.static != nil {
		v, err := fold(e.Op, *x.static, *y.static)
		if err != nil {
			return val{}, err
		}
		return intVal(v), nil
	}
	if x.str != nil && y.str != nil && e.Op == "+" {
		return strVal(*x.str + *y.str), nil
	}
	str := x.t == tStr || y.t == tStr
	switch e.Op {
	case "+":
		if str {
			return val{code: x.asText() + " .. " + y.asText(), t: tStr}, nil
		}
		return val{code: "(" + x.asInt() + " + " + y.asInt() + ")", t: tInt}, nil
	case "-", "*", "&", "|", "<<", ">>":
		return val{code: "(" + x.asInt() + " " + e.Op + " " + y.asInt() + ")", t: tInt}, nil
	case "^":
		return val{code: "(" + x.asInt() + " ~ " + y.asInt() + ")", t: tInt}, nil
	case "/":
		b.n.file.need("integers")
		return val{code: "integers.divide_truncated(" + x.asInt() + ", " + y.asInt() + ")", t: tInt}, nil
	case "%":
		b.n.file.need("integers")
		return val{code: "integers.remainder_truncated(" + x.asInt() + ", " + y.asInt() + ")", t: tInt}, nil
	case "==", "!=", "<", "<=", ">", ">=":
		op := e.Op
		if op == "!=" {
			op = "~="
		}
		l, r := x.asInt(), y.asInt()
		if str || x.t == tAny || y.t == tAny {
			l, r = x.asText(), y.asText()
		}
		if x.t == tAny && y.t == tInt {
			r = y.code
		}
		return val{code: "(" + l + " " + op + " " + r + ")", t: tBool}, nil
	case "&&":
		if x.static != nil {
			return val{code: y.asBool(), t: tBool}, nil
		}
		return val{code: "(" + x.asBool() + " and " + y.asBool() + ")", t: tBool}, nil
	case "||":
		if x.static != nil {
			return val{code: y.asBool(), t: tBool}, nil
		}
		return val{code: "(" + x.asBool() + " or " + y.asBool() + ")", t: tBool}, nil
	}
	return val{}, unsupported("operator "+e.Op, "")
}

// item writes an item argument: a name, an ID, or a computed ID.
func (b *body) item(e Expr) (string, error) {
	v, err := b.expr(e)
	if err != nil {
		return "", err
	}
	c := b.n.c
	if v.static != nil {
		name, ok := c.items[uint32(*v.static)] // #nosec G115 -- item IDs are 32 bits.
		if !ok {
			return "", unsupported("item", "%d is not an item", *v.static)
		}
		return b.n.file.record("items.id", name), nil
	}
	if v.str != nil {
		id, ok := c.itemIDs[strings.ToLower(*v.str)]
		if !ok {
			return "", unsupported("item", "%q is not an item", *v.str)
		}
		return b.n.file.record("items.id", c.items[id]), nil
	}
	if v.t == tStr {
		b.n.file.need("items")
		return "items.id(" + v.code + ")", nil
	}
	return v.asInt(), nil
}

// constant writes a constant of a module, such as emotions.thanks.
func (b *body) constant(e Expr, module string) (string, error) {
	v, err := b.expr(e)
	if err != nil {
		return "", err
	}
	if v.static == nil {
		return v.asInt(), nil
	}
	if name, ok := b.n.c.modules[module].Names[*v.static]; ok {
		b.n.file.need(module)
		return module + "." + name, nil
	}
	return fmt.Sprint(*v.static), nil
}

// args converts arguments.
func (b *body) args(list []Expr) ([]val, error) {
	out := make([]val, 0, len(list))
	for _, a := range list {
		v, err := b.expr(a)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// texts writes arguments as strings, for say and choose.
func texts(vals []val) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = v.asText()
	}
	return strings.Join(parts, ", ")
}

// menuOptions writes the options of select: "A:B" splits at once, and a
// computed text splits when it runs.
func (b *body) menuOptions(list []Expr) (string, int, error) {
	var parts []string
	count := 0
	for _, a := range list {
		v, err := b.expr(a)
		if err != nil {
			return "", 0, err
		}
		if v.str != nil {
			for _, o := range strings.Split(*v.str, ":") {
				parts = append(parts, luaString(o))
				count++
			}
			continue
		}
		parts = append(parts, "R.menu("+v.asText()+")")
		count = -1
	}
	return strings.Join(parts, ", "), count, nil
}

// callfunc writes a call to an rAthena function: the converted module,
// loaded on first use, called with the player, the conversation, the
// NPC and the arguments.
func (b *body) callfunc(args []Expr) (val, error) {
	if len(args) == 0 {
		return val{}, unsupported("callfunc", "no function")
	}
	name, ok := args[0].(*Str)
	if !ok {
		return val{}, unsupported("callfunc", "a computed function name")
	}
	key := strings.ToLower(name.Value)
	if ignoredFunctions[key] {
		b.n.ok("callfunc")
		return intVal(0), nil
	}
	if what, ok := unavailableFunctions[key]; ok {
		b.n.ok("callfunc")
		return val{code: fmt.Sprintf("R.not_available(conversation, %s)", luaString(what)), t: tInt}, nil
	}
	f, err := b.n.c.function(key)
	if err != nil {
		return val{}, err
	}
	b.n.calls[key] = true
	vals, err := b.args(args[1:])
	if err != nil {
		return val{}, err
	}
	parts := []string{"player", "conversation", "npc"}
	for _, v := range vals {
		parts = append(parts, v.code)
	}
	b.n.ok("callfunc")
	return val{code: fmt.Sprintf("require(%s)(%s)", luaString(moduleName(f.Path)), strings.Join(parts, ", ")), t: tAny}, nil
}

// moduleName is the require name of a core file.
func moduleName(path string) string {
	return "core." + strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(path, "scripts/"), ".lua"), "/", ".")
}

// callsub writes a call to a label as a local function. The function
// runs from the label to the end of the script's top level.
func (b *body) callsub(args []Expr) (val, error) {
	if len(args) == 0 {
		return val{}, unsupported("callsub", "no label")
	}
	id, ok := args[0].(*Ident)
	if !ok {
		return val{}, unsupported("callsub", "a computed label")
	}
	label := strings.ToLower(id.Name)
	fn, ok := b.subs[label]
	if !ok {
		fn = "sub_" + luaName(label)
		b.subs[label] = fn
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return val{}, err
	}
	parts := []string{"player", "conversation", "npc"}
	for _, v := range vals {
		parts = append(parts, v.code)
	}
	b.n.ok("callsub")
	return val{code: fmt.Sprintf("%s(%s)", fn, strings.Join(parts, ", ")), t: tAny}, nil
}

// input writes rAthena's input into a variable, and returns the result
// code: 1 for a number over the maximum, 2 under the minimum, 0
// otherwise. Text input returns 0.
func (b *body) input(args []Expr) (string, error) {
	if len(args) == 0 || len(args) > 3 {
		return "", unsupported("input", "arguments")
	}
	target, ok := asVar(args[0])
	if !ok {
		return "", unsupported("input", "not into a variable")
	}
	_, _, str := varName(target.Name)
	tmp := b.hide("input")
	if str {
		max := "255"
		if len(args) == 3 {
			v, err := b.expr(args[2])
			if err != nil {
				return "", err
			}
			max = "math.min(255, " + v.asInt() + ")"
		}
		b.line("%s = conversation:ask_text(%s)", tmp, max)
		if err := b.writeVar(target, val{code: tmp, t: tStr}); err != nil {
			return "", err
		}
		return "0", nil
	}
	lo, hi := "0", "2147483647"
	if len(args) >= 2 {
		v, err := b.expr(args[1])
		if err != nil {
			return "", err
		}
		lo = v.asInt()
	}
	if len(args) == 3 {
		v, err := b.expr(args[2])
		if err != nil {
			return "", err
		}
		hi = v.asInt()
	}
	code := b.hide("input_code")
	b.line("%s, %s = R.input(conversation, %s, %s)", tmp, code, lo, hi)
	if err := b.writeVar(target, val{code: tmp, t: tInt}); err != nil {
		return "", err
	}
	return code, nil
}

// call converts a function call in an expression.
func (b *body) call(e *Call) (val, error) {
	name := strings.ToLower(e.Name)
	args := e.Args
	f := b.n.file
	done := func(v val, err error) (val, error) {
		if err == nil {
			b.n.ok(name)
		}
		return v, err
	}
	switch name {
	case "select", "prompt":
		opts, _, err := b.menuOptions(args)
		if err != nil {
			return val{}, err
		}
		return done(val{code: "conversation:choose(" + opts + ")", t: tInt}, nil)
	case "rand":
		f.need("random")
		vals, err := b.args(args)
		if err != nil {
			return val{}, err
		}
		switch len(vals) {
		case 1:
			return done(val{code: "random.integer(0, " + vals[0].asInt() + " - 1)", t: tInt}, nil)
		case 2:
			return done(val{code: "random.integer(" + vals[0].asInt() + ", " + vals[1].asInt() + ")", t: tInt}, nil)
		}
		return val{}, unsupported("rand", "arguments")
	case "callfunc":
		return b.callfunc(args)
	case "callsub":
		return b.callsub(args)
	case "getarg":
		if !b.hasArgs || len(args) == 0 || len(args) > 2 {
			return val{}, unsupported("getarg", "outside a function")
		}
		i, err := b.expr(args[0])
		if err != nil {
			return val{}, err
		}
		if len(args) == 2 {
			d, err := b.expr(args[1])
			if err != nil {
				return val{}, err
			}
			return done(val{code: "R.arg(args, " + i.asInt() + ", " + d.code + ")", t: tAny}, nil)
		}
		return done(val{code: "R.arg(args, " + i.asInt() + ")", t: tAny}, nil)
	case "getargcount":
		if !b.hasArgs {
			return val{}, unsupported("getargcount", "outside a function")
		}
		return done(val{code: "args.n", t: tInt}, nil)
	case "countitem":
		if len(args) != 1 {
			return val{}, unsupported(name, "arguments")
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, err
		}
		return done(val{code: "player:count_item(" + it + ")", t: tInt}, nil)
	case "checkweight":
		if len(args) != 2 {
			return val{}, unsupported(name, "more than one item")
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, err
		}
		n, err := b.expr(args[1])
		if err != nil {
			return val{}, err
		}
		return done(val{code: "player:can_carry(" + it + ", " + n.asInt() + ")", t: tBool}, nil)
	case "strcharinfo":
		if len(args) == 1 {
			if v, err := b.expr(args[0]); err == nil && v.static != nil {
				switch *v.static {
				case 0:
					return done(val{code: "player.name", t: tStr}, nil)
				case 3:
					return done(val{code: "player.map", t: tStr}, nil)
				}
			}
		}
		return val{}, unsupported("strcharinfo", "other than the name and the map")
	case "strnpcinfo":
		if len(args) == 1 {
			if v, err := b.expr(args[0]); err == nil && v.static != nil {
				switch *v.static {
				case 0, 1:
					return done(val{code: "npc.name", t: tStr}, nil)
				case 4:
					return done(val{code: "npc.map", t: tStr}, nil)
				}
			}
		}
		return val{}, unsupported("strnpcinfo", "other than the name and the map")
	case "getcharid":
		if len(args) == 1 {
			if v, err := b.expr(args[0]); err == nil && v.static != nil {
				switch *v.static {
				case 0:
					return done(val{code: "player.character_id", t: tInt}, nil)
				case 3:
					return done(val{code: "player.account_id", t: tInt}, nil)
				}
			}
		}
		return val{}, unsupported("getcharid", "parties, guilds and other IDs")
	case "getitemname":
		if len(args) != 1 {
			return val{}, unsupported(name, "arguments")
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, err
		}
		f.need("items")
		return done(val{code: "items.name(" + it + ")", t: tStr}, nil)
	case "mesitemlink":
		if len(args) != 1 {
			return val{}, unsupported(name, "arguments")
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, err
		}
		f.need("text")
		return done(val{code: "text.item_link(" + it + ")", t: tStr}, nil)
	case "getarraysize", "implode":
		if len(args) == 0 {
			return val{}, unsupported(name, "arguments")
		}
		target, ok := asVar(args[0])
		if !ok {
			return val{}, unsupported(name, "not of a variable")
		}
		sep := `""`
		if name == "implode" && len(args) == 2 {
			v, err := b.expr(args[1])
			if err != nil {
				return val{}, err
			}
			sep = v.asText()
		}
		if p, _, _ := varName(target.Name); p == ".@" {
			l := b.local(target.Name)
			if name == "implode" {
				return done(val{code: "R.implode(" + l.lua + ", " + sep + ")", t: tStr}, nil)
			}
			return done(val{code: "R.size(" + l.lua + ")", t: tInt}, nil)
		}
		v, err := b.variable(target.Name)
		if err != nil {
			return val{}, err
		}
		if v.scope == "npc" {
			if name == "implode" {
				return done(val{code: "R.implode(" + v.local + ", " + sep + ")", t: tStr}, nil)
			}
			return done(val{code: "R.size(" + v.local + ")", t: tInt}, nil)
		}
		f.need("variables")
		if name == "implode" {
			return done(val{code: fmt.Sprintf("R.aimplode(%s, %s, %s)", v.local, v.owner(), sep), t: tStr}, nil)
		}
		return done(val{code: fmt.Sprintf("R.asize(%s, %s)", v.local, v.owner()), t: tInt}, nil)
	case "checkre":
		// The converter runs per mode, as rAthena's RENEWAL defines.
		return done(intVal(b2i(b.n.c.mode == "renewal")), nil)
	case "input":
		code, err := b.input(args)
		if err != nil {
			return val{}, err
		}
		return done(val{code: code, t: tInt}, nil)
	case "getskilllv":
		if len(args) != 1 {
			return val{}, unsupported(name, "arguments")
		}
		v, err := b.expr(args[0])
		if err != nil {
			return val{}, err
		}
		skill := ""
		switch {
		case v.str != nil:
			skill = *v.str
		case v.static != nil:
			if s, ok := b.n.c.consts.SkillName(*v.static); ok {
				skill = s
			}
		}
		if skill == "" {
			return val{}, unsupported(name, "a computed skill")
		}
		return done(val{code: "player:get_skill_level(" + f.record("skills.id", skill) + ")", t: tInt}, nil)
	case "playerattached":
		return done(intVal(1), nil)
	case "min", "max":
		vals, err := b.args(args)
		if err != nil || len(vals) == 0 {
			return val{}, unsupported(name, "arguments")
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = v.asInt()
		}
		return done(val{code: "math." + name + "(" + strings.Join(parts, ", ") + ")", t: tInt}, nil)
	case "getstrlen":
		if len(args) != 1 {
			return val{}, unsupported(name, "arguments")
		}
		v, err := b.expr(args[0])
		if err != nil {
			return val{}, err
		}
		return done(val{code: "#" + v.asText(), t: tInt}, nil)
	case "guildopenstorage":
		return done(val{code: `R.not_available(conversation, "Guild storage")`, t: tInt}, nil)
	}
	return val{}, unsupported(name, "")
}

// command writes one command statement.
func (b *body) command(s *Command) error {
	name := strings.ToLower(s.Name)
	err := b.commandErr(name, s.Args)
	if err != nil {
		return b.unsupported(err)
	}
	b.n.ok(name)
	return nil
}

func (b *body) commandErr(name string, args []Expr) error {
	f := b.n.file
	switch name {
	case "mes":
		vals, err := b.args(args)
		if err != nil {
			return err
		}
		if len(vals) == 0 {
			return unsupported(name, "no text")
		}
		b.line("conversation:say(%s)", texts(vals))
	case "next":
		b.line("conversation:wait_for_next()")
	case "close2":
		b.line("conversation:close()")
	case "close3":
		b.line("conversation:hide_image()")
		return b.jump("close")
	case "end":
		return b.jump("end")
	case "cutin":
		if len(args) != 2 {
			return unsupported(name, "arguments")
		}
		img, err := b.expr(args[0])
		if err != nil {
			return err
		}
		pos, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if pos.static == nil {
			return unsupported(name, "a computed position")
		}
		if *pos.static == 255 || img.str != nil && *img.str == "" {
			b.line("conversation:hide_image()")
			return nil
		}
		where := map[int64]string{0: "left", 1: "center", 2: "right", 3: "center", 4: "center"}[*pos.static]
		if where == "" {
			return unsupported(name, "position %d", *pos.static)
		}
		b.line("conversation:show_image(%s, %q)", img.asText(), where)
	case "emotion":
		if len(args) != 1 {
			return unsupported(name, "on a player or another NPC")
		}
		e, err := b.constant(args[0], "emotions")
		if err != nil {
			return err
		}
		b.line("npc:show_emotion(%s)", e)
	case "npctalk":
		if len(args) != 1 {
			return unsupported(name, "as another NPC")
		}
		v, err := b.expr(args[0])
		if err != nil {
			return err
		}
		b.line("npc:say_aloud(%s)", v.asText())
	case "select", "prompt":
		opts, _, err := b.menuOptions(args)
		if err != nil {
			return err
		}
		b.line("conversation:choose(%s)", opts)
	case "menu":
		return b.menu(args)
	case "input":
		_, err := b.input(args)
		return err
	case "sleep", "sleep2":
		if len(args) != 1 {
			return unsupported(name, "arguments")
		}
		v, err := b.expr(args[0])
		if err != nil {
			return err
		}
		b.line("conversation:wait(%s)", v.asInt())
	case "progressbar":
		if len(args) != 2 {
			return unsupported(name, "arguments")
		}
		v, err := b.expr(args[1])
		if err != nil {
			return err
		}
		b.line("conversation:show_progress(%s)", v.asInt())
	case "callfunc":
		v, err := b.callfunc(args)
		if err != nil {
			return err
		}
		if v.static == nil {
			b.line("%s", v.code)
		}
	case "callsub":
		v, err := b.callsub(args)
		if err != nil {
			return err
		}
		b.line("%s", v.code)
	case "warp":
		if len(args) != 3 {
			return unsupported(name, "arguments")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return err
		}
		if m.str != nil {
			switch *m.str {
			case "Random":
				b.line("player:warp_to_random_cell()")
				return nil
			case "SavePoint":
				b.line("player:warp_to_save_point()")
				return nil
			}
		} else {
			return unsupported(name, "to a computed map")
		}
		x, err := b.expr(args[1])
		if err != nil {
			return err
		}
		y, err := b.expr(args[2])
		if err != nil {
			return err
		}
		b.line("player:warp(%s, %s, %s)", m.code, x.asInt(), y.asInt())
	case "savepoint", "save":
		// The spread of savepoint's range arguments is left out: the
		// save point is the cell itself.
		if len(args) != 3 && len(args) != 5 {
			return unsupported(name, "for another character")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return err
		}
		x, err := b.expr(args[1])
		if err != nil {
			return err
		}
		y, err := b.expr(args[2])
		if err != nil {
			return err
		}
		b.line("player:set_save_point(%s, %s, %s)", m.asText(), x.asInt(), y.asInt())
	case "percentheal":
		if len(args) != 2 {
			return unsupported(name, "arguments")
		}
		hp, err := b.expr(args[0])
		if err != nil {
			return err
		}
		sp, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if hp.static != nil && *hp.static < 0 || sp.static != nil && *sp.static < 0 {
			return unsupported("percentheal", "a negative amount, which takes HP or SP")
		}
		var parts []string
		if hp.static == nil || *hp.static != 0 {
			parts = append(parts, "hp = "+hp.asInt())
		}
		if sp.static == nil || *sp.static != 0 {
			parts = append(parts, "sp = "+sp.asInt())
		}
		b.line("player:heal_percent { %s }", strings.Join(parts, ", "))
	case "getitem", "delitem":
		if len(args) != 2 {
			return unsupported(name, "for another character")
		}
		it, err := b.item(args[0])
		if err != nil {
			return err
		}
		n, err := b.expr(args[1])
		if err != nil {
			return err
		}
		method := map[string]string{"getitem": "give_item", "delitem": "take_item"}[name]
		b.line("player:%s(%s, %s)", method, it, n.asInt())
	case "specialeffect2":
		if len(args) != 1 {
			return unsupported(name, "on another character")
		}
		e, err := b.constant(args[0], "effects")
		if err != nil {
			return err
		}
		b.line("player:show_effect(%s)", e)
	case "dispbottom", "message":
		text := args
		if name == "message" {
			if len(args) != 2 {
				return unsupported(name, "arguments")
			}
			text = args[1:]
		}
		if len(text) != 1 {
			return unsupported(name, "with a color")
		}
		v, err := b.expr(text[0])
		if err != nil {
			return err
		}
		b.line("player:send_message(%s)", v.asText())
	case "viewpoint":
		if len(args) != 5 {
			return unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return err
		}
		if vals[0].static != nil && *vals[0].static == 2 {
			b.line("player:unmark_minimap(%s)", vals[3].asInt())
			return nil
		}
		b.line("player:mark_minimap(%s, %s, %s, %s)", vals[3].asInt(), vals[1].asInt(), vals[2].asInt(), vals[4].asInt())
	case "callshop":
		if len(args) < 1 {
			return unsupported(name, "arguments")
		}
		v, err := b.expr(args[0])
		if err != nil {
			return err
		}
		if v.str == nil {
			return unsupported(name, "a computed shop")
		}
		b.line("player:open_shop(%s)", luaString("core:"+*v.str))
	case "announce":
		if len(args) < 2 {
			return unsupported(name, "arguments")
		}
		v, err := b.expr(args[0])
		if err != nil {
			return err
		}
		flag, err := b.expr(args[1])
		if err != nil || flag.static == nil {
			return unsupported(name, "a computed target")
		}
		// bc_all is 0; the color flags only change the color.
		switch *flag.static &^ 0x30 {
		case 0:
			f.need("world")
			b.line("world.announce(%s)", v.asText())
		case 1:
			f.need("maps")
			b.line("maps.announce(npc.map, %s)", v.asText())
		default:
			return unsupported(name, "to an area or one player")
		}
	case "mapannounce":
		if len(args) < 2 {
			return unsupported(name, "arguments")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return err
		}
		v, err := b.expr(args[1])
		if err != nil {
			return err
		}
		f.need("maps")
		b.line("maps.announce(%s, %s)", m.asText(), v.asText())
	case "logmes":
		b.line("-- logmes: wAthena has no script log yet.")
	case "setarray", "deletearray":
		return b.arrayCommand(name, args)
	case "openstorage":
		b.line(`R.not_available(conversation, "Storage")`)
	case "setcart":
		b.line(`R.not_available(conversation, "Pushcart rental")`)
	default:
		return unsupported(name, "")
	}
	return nil
}

// menu writes rAthena's menu: a choice, then a jump to the option's
// label. "-" goes on after the menu.
func (b *body) menu(args []Expr) error {
	if len(args) < 2 || len(args)%2 != 0 {
		return unsupported("menu", "arguments")
	}
	var opts []string
	var labels []string
	for i := 0; i < len(args); i += 2 {
		v, err := b.expr(args[i])
		if err != nil {
			return err
		}
		label := strings.ToLower(args[i+1].(*Ident).Name)
		if v.str == nil {
			return unsupported("menu", "computed options")
		}
		// Options with ':' are several options that share the label.
		for _, o := range strings.Split(*v.str, ":") {
			opts = append(opts, luaString(o))
			labels = append(labels, label)
		}
	}
	choice := b.n.fresh("choice")
	b.line("do")
	b.indent++
	b.line("local %s = conversation:choose(%s)", choice, strings.Join(opts, ", "))
	first := true
	for i, l := range labels {
		if l == "-" {
			continue
		}
		kw := "elseif"
		if first {
			kw = "if"
		}
		first = false
		b.gotos = append(b.gotos, l)
		b.line("%s %s == %d then goto %s", kw, choice, i+1, luaName(l))
	}
	if !first {
		b.line("end")
	}
	b.indent--
	b.line("end")
	return nil
}

// arrayCommand writes setarray and deletearray.
func (b *body) arrayCommand(name string, args []Expr) error {
	if len(args) == 0 {
		return unsupported(name, "arguments")
	}
	target, ok := asVar(args[0])
	if !ok {
		return unsupported(name, "not of a variable")
	}
	start := "0"
	if target.Index != nil {
		v, err := b.expr(target.Index)
		if err != nil {
			return err
		}
		start = v.asInt()
	}
	_, _, str := varName(target.Name)
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	parts := make([]string, len(vals))
	for i, v := range vals {
		if str {
			parts[i] = v.asText()
		} else {
			parts[i] = v.asInt()
		}
	}
	rest := ""
	if len(parts) > 0 {
		rest = ", " + strings.Join(parts, ", ")
	}
	if p, _, _ := varName(target.Name); p == ".@" {
		l := b.local(target.Name)
		if name == "setarray" {
			b.line("R.setarray(%s, %s%s)", l.lua, start, rest)
		} else {
			b.line("R.deletearray(%s, %s%s)", l.lua, start, rest)
		}
		return nil
	}
	v, err := b.variable(target.Name)
	if err != nil {
		return err
	}
	if v.scope == "npc" {
		if name == "setarray" {
			b.line("R.setarray(%s, %s%s)", v.local, start, rest)
		} else {
			b.line("R.deletearray(%s, %s%s)", v.local, start, rest)
		}
		return nil
	}
	b.n.file.need("variables")
	if name == "setarray" {
		b.line("R.asetarray(%s, %s, %s%s)", v.local, v.owner(), start, rest)
	} else {
		b.line("R.adelete(%s, %s, %s%s)", v.local, v.owner(), start, rest)
	}
	return nil
}

// sortedKeys returns a map's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
