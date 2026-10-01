package convert

import (
	"fmt"
	"strings"
)

// unsupportedError marks something the converter cannot convert. Feature
// is the short name the coverage report counts, such as "sc_start".
type unsupportedError struct {
	Feature string
	Detail  string
}

func (e *unsupportedError) Error() string {
	if e.Detail == "" {
		return e.Feature
	}
	return e.Feature + ": " + e.Detail
}

func unsupported(feature, format string, args ...any) error {
	return &unsupportedError{Feature: feature, Detail: fmt.Sprintf(format, args...)}
}

// scope is where converted code runs, which decides what it may read.
type scope int

const (
	// scopeEquip is an equip script's setup and its computed functions.
	// Values come from the equipped copy and its wearer. Random numbers
	// are not allowed, since a computed function must give the same
	// result for the same state.
	scopeEquip scope = iota
	// scopeUse is a use handler, which runs once per use.
	scopeUse
)

// slots maps rAthena's EQI_ constants to the slot names of
// player:get_equipped.
var slots = map[string]string{
	"EQI_HEAD_TOP": "head_top", "EQI_ARMOR": "armor", "EQI_HAND_L": "left_hand", "EQI_HAND_R": "right_hand",
	"EQI_GARMENT": "garment", "EQI_SHOES": "shoes", "EQI_ACC_L": "left_accessory", "EQI_ACC_R": "right_accessory",
	"EQI_HEAD_MID": "head_middle", "EQI_HEAD_LOW": "head_low",
	"EQI_COSTUME_HEAD_LOW": "costume_head_low", "EQI_COSTUME_HEAD_MID": "costume_head_middle",
	"EQI_COSTUME_HEAD_TOP": "costume_head_top", "EQI_COSTUME_GARMENT": "costume_garment", "EQI_AMMO": "ammo",
	"EQI_SHADOW_ARMOR": "shadow_armor", "EQI_SHADOW_WEAPON": "shadow_weapon", "EQI_SHADOW_SHIELD": "shadow_shield",
	"EQI_SHADOW_SHOES": "shadow_shoes", "EQI_SHADOW_ACC_R": "shadow_right_accessory", "EQI_SHADOW_ACC_L": "shadow_left_accessory",
}

// paramReads are the parameters (rAthena's SP_ values read by name) that
// converted scripts can read, as player fields.
var paramReads = map[string]string{
	"baselevel": "player.base_level",
	"joblevel":  "player.job_level",
	"class":     "player.job",
}

// statReads are the stats readparam can read, as player fields. The six
// trait stats read 0 until trait stats can be raised.
var statReads = map[string]string{
	"bstr": "base_str", "bagi": "base_agi", "bvit": "base_vit", "bint": "base_int", "bdex": "base_dex", "bluk": "base_luk",
	"bpow": "base_pow", "bsta": "base_sta", "bwis": "base_wis", "bspl": "base_spl", "bcon": "base_con", "bcrt": "base_crt",
}

// resolve turns an expression into its converted form: constants become
// Const, reads become Read, and static parts are folded. vars gives the
// value of a .@ variable in an equip script. In a use script vars is nil
// and variables stay.
func (c *itemConv) resolve(e Expr, sc scope, vars func(*Var) (Expr, error)) (Expr, error) {
	switch e := e.(type) {
	case *Num, *Str, *Const, *Read:
		return e, nil
	case *Var:
		if !strings.HasPrefix(e.Name, ".@") {
			return nil, unsupported("variables", "%s keeps state outside the script", e.Name)
		}
		if e.Index != nil {
			return nil, unsupported("arrays", "%s[...]", e.Name)
		}
		if vars != nil {
			return vars(e)
		}
		return e, nil
	case *Ident:
		if v, param, ok := c.conv.consts.Lookup(e.Name); ok {
			if !param {
				return &Const{Name: e.Name, Value: v}, nil
			}
			return c.param(e.Name)
		}
		if _, ok := c.conv.consts.Skill(e.Name); ok {
			return &Str{Value: e.Name}, nil
		}
		return nil, unsupported("variables", "%s is a character variable or an unknown constant", e.Name)
	case *Call:
		return c.call(e, sc, vars)
	case *Unary:
		x, err := c.resolve(e.X, sc, vars)
		if err != nil {
			return nil, err
		}
		if v, ok := static(x); ok {
			switch e.Op {
			case "-":
				return &Num{Value: -v}, nil
			case "!":
				return &Num{Value: b2i(v == 0)}, nil
			case "~":
				return &Num{Value: ^v}, nil
			}
		}
		return &Unary{Op: e.Op, X: x}, nil
	case *Binary:
		x, err := c.resolve(e.X, sc, vars)
		if err != nil {
			return nil, err
		}
		y, err := c.resolve(e.Y, sc, vars)
		if err != nil {
			return nil, err
		}
		return c.binary(e.Op, x, y)
	case *Ternary:
		cond, err := c.resolve(e.Cond, sc, vars)
		if err != nil {
			return nil, err
		}
		a, err := c.resolve(e.Then, sc, vars)
		if err != nil {
			return nil, err
		}
		b, err := c.resolve(e.Else, sc, vars)
		if err != nil {
			return nil, err
		}
		if v, ok := static(cond); ok {
			if v != 0 {
				return a, nil
			}
			return b, nil
		}
		return &Ternary{Cond: cond, Then: a, Else: b}, nil
	case *Assign, *IncDec:
		return nil, unsupported("assignment in an expression", "")
	}
	return nil, unsupported("expression", "%T", e)
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// binary folds a binary operation on static values, and recognizes job
// class tests on eaclass().
func (c *itemConv) binary(op string, x, y Expr) (Expr, error) {
	a, aok := static(x)
	b, bok := static(y)
	if aok && bok {
		v, err := fold(op, a, b)
		if err != nil {
			return nil, err
		}
		return &Num{Value: v}, nil
	}
	if r, ok := x.(*Read); ok && r.Lua == "player.job_class" && op == "&" {
		if k, ok := y.(*Const); ok {
			if t := jobClassTest(k.Name); t != "" {
				c.file.need("jobs")
				return &Read{Lua: t, Bool: true}, nil
			}
		}
	}
	// (eaclass() & EAJ_xxxMASK) == EAJ_JOB compares a job without its
	// level bits.
	if op == "==" || op == "!=" {
		if inner, ok := x.(*Binary); ok && inner.Op == "&" {
			if r, ok := inner.X.(*Read); ok && r.Lua == "player.job_class" {
				mask, mok := inner.Y.(*Const)
				job, jok := y.(*Const)
				if mok && jok {
					if t, err := c.jobMaskTest(mask.Name, job.Name, job.Value); err == nil {
						if op == "!=" {
							t = "not (" + t + ")"
						}
						return &Read{Lua: t, Bool: true}, nil
					} else {
						return nil, err
					}
				}
			}
		}
	}
	return &Binary{Op: op, X: x, Y: y}, nil
}

// jobClassTest is the test function for eaclass() & EAJL_x.
func jobClassTest(name string) string {
	switch strings.ToUpper(name) {
	case "EAJL_UPPER":
		return "jobs.is_transcendent(player.job)"
	case "EAJL_BABY":
		return "jobs.is_baby(player.job)"
	case "EAJL_THIRD":
		return "jobs.is_third(player.job)"
	case "EAJL_FOURTH":
		return "jobs.is_fourth(player.job)"
	}
	return ""
}

// jobMaskTest converts (eaclass() & mask) == EAJ_x.
func (c *itemConv) jobMaskTest(mask, job string, value int64) (string, error) {
	jobName := strings.TrimPrefix(strings.ToUpper(job), "EAJ_")
	readable, ok := c.conv.jobByName(jobName)
	if !ok {
		return "", unsupported("eaclass", "no job named %s", job)
	}
	c.file.need("jobs")
	switch strings.ToUpper(mask) {
	case "EAJ_BASEMASK":
		return "jobs.first_job(player.job) == jobs." + readable, nil
	case "EAJ_UPPERMASK":
		return "jobs.base_job(player.job) == jobs." + readable, nil
	case "EAJ_THIRDMASK", "EAJ_FOURTHMASK":
		return "jobs.base_job(player.job) == jobs." + readable, nil
	}
	return "", unsupported("eaclass", "mask %s", mask)
}

func fold(op string, a, b int64) (int64, error) {
	switch op {
	case "+":
		return a + b, nil
	case "-":
		return a - b, nil
	case "*":
		return a * b, nil
	case "/":
		if b == 0 {
			return 0, unsupported("division by zero", "")
		}
		return a / b, nil
	case "%":
		if b == 0 {
			return 0, unsupported("division by zero", "")
		}
		return a % b, nil
	case "<<":
		return a << uint(b&63), nil
	case ">>":
		return a >> uint(b&63), nil
	case "&":
		return a & b, nil
	case "|":
		return a | b, nil
	case "^":
		return a ^ b, nil
	case "==":
		return b2i(a == b), nil
	case "!=":
		return b2i(a != b), nil
	case "<":
		return b2i(a < b), nil
	case "<=":
		return b2i(a <= b), nil
	case ">":
		return b2i(a > b), nil
	case ">=":
		return b2i(a >= b), nil
	case "&&":
		return b2i(a != 0 && b != 0), nil
	case "||":
		return b2i(a != 0 || b != 0), nil
	}
	return 0, unsupported("operator", "%s", op)
}

// param converts a parameter such as BaseLevel.
func (c *itemConv) param(name string) (Expr, error) {
	n := strings.ToLower(name)
	if lua, ok := paramReads[n]; ok {
		return &Read{Lua: lua, NonNeg: true, Job: n == "class"}, nil
	}
	switch n {
	case "basejob":
		c.file.need("jobs")
		return &Read{Lua: "jobs.base_job(player.job)", NonNeg: true, Job: true}, nil
	case "baseclass":
		c.file.need("jobs")
		return &Read{Lua: "jobs.first_job(player.job)", NonNeg: true, Job: true}, nil
	}
	return nil, unsupported("parameter "+name, "")
}

// call converts a function call in an expression.
func (c *itemConv) call(e *Call, sc scope, vars func(*Var) (Expr, error)) (Expr, error) {
	name := strings.ToLower(e.Name)
	args := make([]Expr, 0, len(e.Args))
	if name != "callfunc" && name != "getskilllv" && name != "readparam" && name != "getequipid" &&
		name != "getequiprefinerycnt" && name != "getiteminfo" {
		for _, a := range e.Args {
			r, err := c.resolve(a, sc, vars)
			if err != nil {
				return nil, err
			}
			args = append(args, r)
		}
	}
	equipOnly := func() error {
		if c.item.Kind != KindEquip {
			return unsupported(name, "outside an equip script")
		}
		return nil
	}
	switch name {
	case "getrefine":
		if err := equipOnly(); err != nil {
			return nil, err
		}
		return &Read{Lua: "equipped.refine", NonNeg: true}, nil
	case "getenchantgrade":
		if err := equipOnly(); err != nil {
			return nil, err
		}
		return &Read{Lua: "equipped.enchant_grade", NonNeg: true, Grade: true}, nil
	case "getequipweaponlv", "getequiparmorlv":
		field := map[string]string{"getequipweaponlv": "weapon_level", "getequiparmorlv": "armor_level"}[name]
		if len(e.Args) == 0 || isConst(e.Args[0], "EQI_COMPOUND_ON") || isNum(e.Args[0], -1) {
			if err := equipOnly(); err != nil {
				return nil, err
			}
			return &Read{Lua: "equipped." + field, NonNeg: true}, nil
		}
		return nil, unsupported(name, "of another slot")
	case "getskilllv":
		if len(e.Args) != 1 {
			return nil, unsupported(name, "arguments")
		}
		sk, err := c.skillArg(e.Args[0])
		if err != nil {
			return nil, err
		}
		return &Read{Lua: "player:get_skill_level(" + sk + ")", NonNeg: true}, nil
	case "readparam":
		if len(e.Args) != 1 {
			return nil, unsupported(name, "of another character")
		}
		id, ok := e.Args[0].(*Ident)
		if !ok {
			return nil, unsupported(name, "with a computed parameter")
		}
		field, ok := statReads[strings.ToLower(id.Name)]
		if !ok {
			return nil, unsupported("readparam "+id.Name, "")
		}
		return &Read{Lua: "player." + field, NonNeg: true}, nil
	case "eaclass":
		if len(args) != 0 {
			return nil, unsupported(name, "of another job")
		}
		return &Read{Lua: "player.job_class", NonNeg: true}, nil
	case "getequipid":
		if len(e.Args) != 1 {
			return nil, unsupported(name, "arguments")
		}
		slot, err := c.slotArg(e.Args[0])
		if err != nil {
			return nil, err
		}
		if slot == "" {
			if err := equipOnly(); err != nil {
				return nil, err
			}
			return &Read{Lua: "equipped.item", NonNeg: true}, nil
		}
		return &Read{Lua: fmt.Sprintf("equipped_info(%q, \"item\")", slot)}, c.needHelper("equipped_info")
	case "getequiprefinerycnt":
		if len(e.Args) != 1 {
			return nil, unsupported(name, "arguments")
		}
		slot, err := c.slotArg(e.Args[0])
		if err != nil {
			return nil, err
		}
		if slot == "" {
			if err := equipOnly(); err != nil {
				return nil, err
			}
			return &Read{Lua: "equipped.refine", NonNeg: true}, nil
		}
		return &Read{Lua: fmt.Sprintf("equipped_info(%q, \"refine\")", slot), NonNeg: true}, c.needHelper("equipped_info")
	case "getiteminfo":
		// getiteminfo(getequipid(slot), info) reads the item in a slot.
		if len(e.Args) != 2 {
			return nil, unsupported(name, "arguments")
		}
		inner, ok := e.Args[0].(*Call)
		if !ok || !strings.EqualFold(inner.Name, "getequipid") || len(inner.Args) != 1 {
			return nil, unsupported(name, "of an item other than an equipped one")
		}
		slot, err := c.slotArg(inner.Args[0])
		if err != nil {
			return nil, err
		}
		info, err := c.resolve(e.Args[1], sc, vars)
		if err != nil {
			return nil, err
		}
		v, ok := static(info)
		if !ok {
			return nil, unsupported(name, "computed info")
		}
		field := map[int64]string{11: "weapon_type", 20: "weapon_type", 13: "weapon_level", 19: "armor_level"}[v]
		if field == "" {
			return nil, unsupported(name, "info %d", v)
		}
		r := &Read{NonNeg: true, Weapon: field == "weapon_type"}
		if slot == "" {
			if err := equipOnly(); err != nil {
				return nil, err
			}
			r.Lua = "equipped." + field
			return r, nil
		}
		r.Lua = fmt.Sprintf("equipped_info(%q, %q)", slot, field)
		return r, c.needHelper("equipped_info")
	case "min", "max":
		if len(args) == 0 {
			return nil, unsupported(name, "no arguments")
		}
		all := true
		best := int64(0)
		for i, a := range args {
			v, ok := static(a)
			if !ok {
				all = false
				break
			}
			if i == 0 || name == "min" && v < best || name == "max" && v > best {
				best = v
			}
		}
		if all {
			return &Num{Value: best}, nil
		}
		return &Call{Name: "math." + name, Args: args}, nil
	case "pow":
		if len(args) != 2 {
			return nil, unsupported(name, "arguments")
		}
		base, bok := static(args[0])
		exp, eok := static(args[1])
		if bok && eok {
			v := int64(1)
			for range exp {
				v *= base
			}
			return &Num{Value: v}, nil
		}
		if eok && exp >= 0 && exp <= 4 {
			if exp == 0 {
				return &Num{Value: 1}, nil
			}
			out := args[0]
			for range exp - 1 {
				out = &Binary{Op: "*", X: out, Y: args[0]}
			}
			return out, nil
		}
		return nil, unsupported(name, "with a computed exponent")
	case "rand":
		if sc != scopeUse {
			return nil, unsupported("rand", "in an equip script, where bonuses must not change between recalculations")
		}
		c.file.need("random")
		switch len(args) {
		case 1:
			if v, ok := static(args[0]); ok {
				return &Call{Name: "random.integer", Args: []Expr{&Num{Value: 0}, &Num{Value: v - 1}}}, nil
			}
			return &Call{Name: "random.integer", Args: []Expr{&Num{Value: 0}, &Binary{Op: "-", X: args[0], Y: &Num{Value: 1}}}}, nil
		case 2:
			return &Call{Name: "random.integer", Args: args}, nil
		}
		return nil, unsupported(name, "arguments")
	case "callfunc":
		return c.callfunc(e, sc, vars)
	}
	return nil, unsupported(name, "")
}

func isConst(e Expr, name string) bool {
	id, ok := e.(*Ident)
	return ok && strings.EqualFold(id.Name, name)
}

func isNum(e Expr, v int64) bool {
	n, ok := e.(*Num)
	return ok && n.Value == v
}

// callfunc converts calls to the global functions the converter knows.
// F_Rand picks one of its arguments, as getarg(rand(getargcount())).
func (c *itemConv) callfunc(e *Call, sc scope, vars func(*Var) (Expr, error)) (Expr, error) {
	if len(e.Args) == 0 {
		return nil, unsupported("callfunc", "no function")
	}
	fn, ok := e.Args[0].(*Str)
	if !ok {
		return nil, unsupported("callfunc", "computed function name")
	}
	if !strings.EqualFold(fn.Value, "F_Rand") || sc != scopeUse {
		return nil, unsupported("callfunc "+fn.Value, "")
	}
	var choices []Expr
	for _, a := range e.Args[1:] {
		r, err := c.resolve(a, sc, vars)
		if err != nil {
			return nil, err
		}
		choices = append(choices, r)
	}
	c.file.need("random")
	return &Call{Name: "F_Rand", Args: choices}, nil
}

// slotArg converts an EQI_ argument to a slot name.
func (c *itemConv) slotArg(e Expr) (string, error) {
	id, ok := e.(*Ident)
	if !ok {
		return "", unsupported("equipment slot", "computed")
	}
	if strings.EqualFold(id.Name, "EQI_COMPOUND_ON") {
		// The item the script belongs to.
		return "", nil
	}
	slot, ok := slots[strings.ToUpper(id.Name)]
	if !ok {
		return "", unsupported("equipment slot "+id.Name, "")
	}
	return slot, nil
}

// skillArg converts a skill given by name or ID to the local that holds
// its ID.
func (c *itemConv) skillArg(e Expr) (string, error) {
	switch a := e.(type) {
	case *Str:
		if _, ok := c.conv.consts.Skill(a.Value); !ok {
			return "", unsupported("unknown skill", "%s", a.Value)
		}
		return c.file.record("skills.id", c.conv.skillName(a.Value)), nil
	case *Num:
		name, ok := c.conv.consts.SkillName(a.Value)
		if !ok {
			return "", unsupported("unknown skill", "%d", a.Value)
		}
		return c.file.record("skills.id", name), nil
	case *Ident:
		if _, ok := c.conv.consts.Skill(a.Name); ok {
			return c.file.record("skills.id", c.conv.skillName(a.Name)), nil
		}
	}
	return "", unsupported("skill argument", "computed")
}

// needHelper adds a local helper function to the file. They read other
// equipment slots through player:get_equipped.
func (c *itemConv) needHelper(name string) error {
	c.helpers[name] = true
	return nil
}

// valueType is what a converted expression gives in Lua.
type valueType int

const (
	typeInt valueType = iota
	typeBool
	typeStr
)

// lua writes a converted expression as Lua. Conditions and numbers are
// told apart, since 0 is true in Lua: a number used as a condition is
// compared with 0, and a condition used as a number becomes 1 or 0.
func (c *itemConv) lua(e Expr) (string, valueType, error) {
	switch e := e.(type) {
	case *Num:
		return fmt.Sprint(e.Value), typeInt, nil
	case *Const:
		return fmt.Sprint(e.Value), typeInt, nil
	case *Str:
		return luaString(e.Value), typeStr, nil
	case *Read:
		if e.Bool {
			return e.Lua, typeBool, nil
		}
		return e.Lua, typeInt, nil
	case *Var:
		return c.local(e.Name), typeInt, nil
	case *Unary:
		switch e.Op {
		case "!":
			x, err := c.cond(e.X)
			if err != nil {
				return "", 0, err
			}
			return "not " + paren(x), typeBool, nil
		case "-", "~":
			x, err := c.num(e.X)
			if err != nil {
				return "", 0, err
			}
			return e.Op + paren(x), typeInt, nil
		}
	case *Binary:
		return c.luaBinary(e)
	case *Ternary:
		cond, err := c.cond(e.Cond)
		if err != nil {
			return "", 0, err
		}
		a, err := c.num(e.Then)
		if err != nil {
			return "", 0, err
		}
		b, err := c.num(e.Else)
		if err != nil {
			return "", 0, err
		}
		// Numbers are never false in Lua, so and/or picks correctly.
		return "(" + cond + " and " + a + " or " + b + ")", typeInt, nil
	case *Call:
		switch e.Name {
		case "F_Rand":
			parts := make([]string, len(e.Args))
			for i, a := range e.Args {
				s, err := c.num(a)
				if err != nil {
					return "", 0, err
				}
				parts[i] = s
			}
			return fmt.Sprintf("({ %s })[random.integer(1, %d)]", strings.Join(parts, ", "), len(parts)), typeInt, nil
		default:
			parts := make([]string, len(e.Args))
			for i, a := range e.Args {
				s, err := c.num(a)
				if err != nil {
					return "", 0, err
				}
				parts[i] = s
			}
			return e.Name + "(" + strings.Join(parts, ", ") + ")", typeInt, nil
		}
	}
	return "", 0, unsupported("expression", "%T", e)
}

// num writes an expression that must be a number.
func (c *itemConv) num(e Expr) (string, error) {
	s, t, err := c.lua(e)
	if err != nil {
		return "", err
	}
	switch t {
	case typeBool:
		return "(" + s + " and 1 or 0)", nil
	case typeStr:
		return "", unsupported("string arithmetic", "")
	}
	return s, nil
}

// cond writes an expression that must be a condition.
func (c *itemConv) cond(e Expr) (string, error) {
	s, t, err := c.lua(e)
	if err != nil {
		return "", err
	}
	if t == typeInt {
		return paren(s) + " ~= 0", nil
	}
	return s, nil
}

func paren(s string) string {
	if isAtom(s) {
		return s
	}
	return "(" + s + ")"
}

// isAtom reports whether Lua code needs no parentheses as an operand.
func isAtom(s string) bool {
	depth := 0
	for i, r := range s {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ' ':
			if depth == 0 {
				return false
			}
		case '-':
			if i == 0 {
				return false
			}
		}
	}
	return !strings.HasPrefix(s, "not ")
}

// nonNeg reports whether an expression is never negative, so // and %
// give C's result.
func nonNeg(e Expr) bool {
	switch e := e.(type) {
	case *Num:
		return e.Value >= 0
	case *Const:
		return e.Value >= 0
	case *Read:
		return e.NonNeg || e.Bool
	case *Binary:
		switch e.Op {
		case "+", "*", "/", "%", "&", ">>":
			return nonNeg(e.X) && nonNeg(e.Y)
		case "==", "!=", "<", "<=", ">", ">=", "&&", "||":
			return true
		}
	case *Ternary:
		return nonNeg(e.Then) && nonNeg(e.Else)
	case *Call:
		if e.Name == "math.min" || e.Name == "math.max" {
			for _, a := range e.Args {
				if !nonNeg(a) {
					return false
				}
			}
			return true
		}
		if e.Name == "random.integer" {
			return nonNeg(e.Args[0])
		}
	}
	return false
}

func (c *itemConv) luaBinary(e *Binary) (string, valueType, error) {
	switch e.Op {
	case "&&", "||":
		x, err := c.cond(e.X)
		if err != nil {
			return "", 0, err
		}
		y, err := c.cond(e.Y)
		if err != nil {
			return "", 0, err
		}
		op := map[string]string{"&&": " and ", "||": " or "}[e.Op]
		return paren(x) + op + paren(y), typeBool, nil
	case "==", "!=", "<", "<=", ">", ">=":
		x, xt, err := c.lua(e.X)
		if err != nil {
			return "", 0, err
		}
		y, yt, err := c.lua(e.Y)
		if err != nil {
			return "", 0, err
		}
		if xt == typeBool {
			x = "(" + x + " and 1 or 0)"
		}
		if yt == typeBool {
			y = "(" + y + " and 1 or 0)"
		}
		x, xn := c.named(e.X, e.Y, x)
		y, yn := c.named(e.Y, e.X, y)
		if !xn {
			x = paren(x)
		}
		if !yn {
			y = paren(y)
		}
		op := e.Op
		if op == "!=" {
			op = "~="
		}
		return x + " " + op + " " + y, typeBool, nil
	}
	x, err := c.num(e.X)
	if err != nil {
		return "", 0, err
	}
	y, err := c.num(e.Y)
	if err != nil {
		return "", 0, err
	}
	switch e.Op {
	case "+", "-", "*", "&", "|", "<<":
		return paren(x) + " " + e.Op + " " + paren(y), typeInt, nil
	case "^":
		return paren(x) + " ~ " + paren(y), typeInt, nil
	case ">>":
		if !nonNeg(e.X) {
			return "", 0, unsupported("shift of a negative number", "")
		}
		return paren(x) + " >> " + paren(y), typeInt, nil
	case "/", "%":
		if nonNeg(e.X) && nonNeg(e.Y) {
			op := map[string]string{"/": "//", "%": "%"}[e.Op]
			return paren(x) + " " + op + " " + paren(y), typeInt, nil
		}
		c.file.need("integers")
		fn := map[string]string{"/": "integers.divide_truncated", "%": "integers.remainder_truncated"}[e.Op]
		return fn + "(" + x + ", " + y + ")", typeInt, nil
	}
	return "", 0, unsupported("operator", "%s", e.Op)
}

// named writes a constant compared with a job, a weapon type or an
// enchant grade by its name: jobs.knight, weapon_types.bow, or the
// number with the rAthena name in a comment. It reports whether it named
// the constant, so the caller adds no parentheses.
func (c *itemConv) named(self, other Expr, code string) (string, bool) {
	k, ok := self.(*Const)
	if !ok {
		return code, false
	}
	r, ok := other.(*Read)
	if !ok {
		return code, false
	}
	if r.Job {
		if name, ok := c.conv.modules["jobs"].Names[k.Value]; ok {
			c.file.need("jobs")
			return "jobs." + name, true
		}
	}
	if r.Grade {
		return fmt.Sprintf("%d --[[ %s ]]", k.Value, strings.ToUpper(k.Name)), true
	}
	if r.Weapon {
		if name, ok := c.conv.modules["weapon_types"].Names[k.Value]; ok {
			c.file.need("weapon_types")
			return "weapon_types." + name, true
		}
	}
	return code, false
}

// local is the Lua name of a .@ variable.
func (c *itemConv) local(name string) string {
	n := strings.TrimPrefix(name, ".@")
	n = strings.TrimSuffix(n, "$")
	n = strings.ToLower(n)
	if luaKeywords[n] || n == "player" || n == "equipped" || builtinLocals[n] {
		n += "_"
	}
	return n
}

// builtinLocals are names the generated code uses itself.
var builtinLocals = map[string]bool{
	"bonuses": true, "context": true, "events": true, "random": true, "timers": true, "integers": true,
	"items": true, "skills": true, "monsters": true, "jobs": true, "math": true, "hit": true, "use": true,
	"active": true, "equipped_info": true, "effects": true, "statuses": true,
}
