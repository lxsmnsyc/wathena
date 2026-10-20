package convert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Script commands that map onto API that exists: equipment reads,
// strings and arrays, positions, and effects. Each follows its
// BUILDIN_FUNC in rAthena's src/map/script.cpp, and R's helpers in
// rathena.lua keep what Lua does not do the same way.

// equipCall converts the equipment reads of the attached player, by
// rAthena's EQI_ slot numbers: getequipid, getequipname,
// getequiprefinerycnt, getequipisequiped, getequipweaponlv,
// getequiparmorlv and getequipcardid. ok is false for other names.
func (b *body) equipCall(name string, args []Expr) (v val, ok bool, err error) {
	// The value without an item in the slot, as each command pushes it.
	fields := map[string]struct {
		field string
		none  string
		t     valType
	}{
		"getequipid":          {"item", "-1", tInt},
		"getequiprefinerycnt": {"refine", "0", tInt},
		"getequipweaponlv":    {"weapon_level", "0", tInt},
		"getequiparmorlv":     {"armor_level", "0", tInt},
	}
	switch name {
	case "getequipid", "getequiprefinerycnt", "getequipweaponlv", "getequiparmorlv", "getequipname", "getequipisequiped":
	case "getequipcardid":
		// getequipcardid(<slot>, <card slot>): card[n] for n from 0 to
		// 3, or 0. wAthena's copies list their cards from the first
		// slot without gaps, and have no forged or signed marks in
		// card[0], so the nth card is card[n].
		if len(args) != 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return val{}, true, err
		}
		return val{code: fmt.Sprintf("R.equip_card(player, %s, %s)", vals[0].asInt(), vals[1].asInt()), t: tInt}, true, nil
	default:
		return val{}, false, nil
	}
	if len(args) != 1 {
		// No slot reads the item whose script runs (EQI_COMPOUND_ON),
		// which only item scripts have. A second argument is another
		// character.
		return val{}, true, unsupported(name, "without a slot, or for another character")
	}
	slot, err := b.expr(args[0])
	if err != nil {
		return val{}, true, err
	}
	if slot.static != nil && *slot.static == -1 {
		return val{}, true, unsupported(name, "the item whose script runs (EQI_COMPOUND_ON)")
	}
	switch name {
	case "getequipname":
		// The item's name, item_data's ename, or "".
		return val{code: fmt.Sprintf("R.equip_name(player, %s)", slot.asInt()), t: tStr}, true, nil
	case "getequipisequiped":
		return val{code: fmt.Sprintf("(R.equipped(player, %s) ~= nil)", slot.asInt()), t: tBool}, true, nil
	}
	f := fields[name]
	return val{code: fmt.Sprintf("R.equip_field(player, %s, %q, %s)", slot.asInt(), f.field, f.none), t: f.t}, true, nil
}

// isequipped converts isequipped(<item>{, <item>...}): 1 when the
// attached player wears every item, else 0 (buildin_isequipped). It
// converts for items that are not cards, by fixed IDs, at most five:
//   - A card is matched once per combo through setitem_hash, a state of
//     the player's bonuses that the API does not have.
//   - From the sixth argument on, rAthena passes the argument's index to
//     pc_is_same_equip_index where it means a slot, so it skips some
//     slots.
//   - An ID of 0 ends the list, and IDs below 0 are skipped. Without an
//     ID above 0 it returns -1.
func (b *body) isequipped(args []Expr) (val, error) {
	const what = "isequipped"
	if len(args) > 5 {
		return val{}, unsupported(what, "more than five items, where rAthena's check skips slots by the argument's place")
	}
	var ids []string
	for _, a := range args {
		v, err := b.expr(a)
		if err != nil {
			return val{}, err
		}
		if v.static == nil {
			return val{}, unsupported(what, "a computed item, which may be a card")
		}
		id := *v.static
		if id == 0 {
			break
		}
		if id < 0 {
			continue
		}
		if b.n.c.itemTypes[uint32(id)] == "Card" { // #nosec G115 -- checked above 0.
			return val{}, unsupported(what, "a card, which rAthena matches once per combo (setitem_hash)")
		}
		it, err := b.item(a)
		if err != nil {
			return val{}, err
		}
		ids = append(ids, it)
	}
	if len(ids) == 0 {
		return intVal(-1), nil
	}
	return val{code: "R.isequipped(player, " + strings.Join(ids, ", ") + ")", t: tInt}, nil
}

// specialeffect converts specialeffect <effect>{, AREA{, "<NPC>"}} on
// the NPC itself: npc:show_effect plays it for the players near. In a
// function it plays on the NPC that called it, as rAthena's st->oid.
// Other targets and other NPCs need more of the API.
func (b *body) specialeffect(args []Expr) error {
	const what = "specialeffect"
	if len(args) < 1 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	if len(args) >= 2 {
		t, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if t.static == nil || *t.static != sendAREA {
			return unsupported(what, "a send target other than AREA")
		}
	}
	if len(args) == 3 {
		s, ok := args[2].(*Str)
		if !ok || b.n.isFunction || s.Value != b.n.line.Name || b.n.c.duplicated[b.n.line.Name] {
			return unsupported(what, "on another NPC")
		}
	}
	e, err := b.expr(args[0])
	if err != nil {
		return err
	}
	name, ok := "", false
	if e.static != nil {
		name, ok = b.n.c.modules["effects"].Names[*e.static]
	}
	if !ok {
		// rAthena does nothing for an effect outside EF_NONE to EF_MAX.
		return unsupported(what, "an effect that is not a fixed EF_ effect")
	}
	b.line("npc:show_effect(%s.%s)", b.n.file.need("effects"), name)
	return nil
}

// sendAREA is rAthena's send_target AREA.
const sendAREA = 2

// stringCall converts the string and number functions. ok is false for
// other names.
func (b *body) stringCall(name string, args []Expr) (v val, ok bool, err error) {
	arity := map[string][2]int{
		"atoi": {1, 1}, "strtol": {2, 2}, "compare": {2, 2}, "charat": {2, 2}, "strtoupper": {1, 1},
		"strtolower": {1, 1}, "substr": {3, 3}, "insertchar": {3, 3}, "replacestr": {3, 5}, "countstr": {2, 3},
		"pow": {2, 2}, "distance": {4, 4}, "sprintf": {1, 128}, "delchar": {2, 2},
	}
	n, known := arity[name]
	if !known {
		return val{}, false, nil
	}
	if len(args) < n[0] || len(args) > n[1] {
		return val{}, true, unsupported(name, "arguments")
	}
	if name == "sprintf" {
		v, err := b.sprintf(args)
		return v, true, err
	}
	vals, err := b.args(args)
	if err != nil {
		return val{}, true, err
	}
	// The arguments that rAthena reads with script_getnum. A string there
	// goes through conv_num, which the converter does not do.
	numbers := map[string][]int{
		"strtol": {1}, "charat": {1}, "substr": {1, 2}, "insertchar": {2}, "replacestr": {3, 4}, "countstr": {2}, "delchar": {1},
		"pow": {0, 1}, "distance": {0, 1, 2, 3},
	}
	for _, i := range numbers[name] {
		if i < len(vals) && vals[i].t == tStr {
			return val{}, true, unsupported(name, "a string where a number goes")
		}
	}
	// rAthena's script_getstr turns a number into its digits.
	str := func(i int) string {
		if vals[i].t == tStr {
			return vals[i].code
		}
		return "tostring(" + vals[i].asInt() + ")"
	}
	num := func(i int) string { return vals[i].asInt() }
	switch name {
	case "atoi":
		return val{code: "R.atoi(" + str(0) + ")", t: tInt}, true, nil
	case "strtol":
		return val{code: "R.strtol(" + str(0) + ", " + num(1) + ")", t: tInt}, true, nil
	case "compare":
		return val{code: "R.compare(" + str(0) + ", " + str(1) + ")", t: tInt}, true, nil
	case "charat":
		return val{code: "R.charat(" + str(0) + ", " + num(1) + ")", t: tStr}, true, nil
	case "strtoupper":
		return val{code: "string.upper(" + str(0) + ")", t: tStr}, true, nil
	case "strtolower":
		return val{code: "string.lower(" + str(0) + ")", t: tStr}, true, nil
	case "substr":
		return val{code: "R.substr(" + str(0) + ", " + num(1) + ", " + num(2) + ")", t: tStr}, true, nil
	case "insertchar":
		return val{code: "R.insertchar(" + str(0) + ", " + str(1) + ", " + num(2) + ")", t: tStr}, true, nil
	case "delchar":
		return val{code: "R.delchar(" + str(0) + ", " + num(1) + ")", t: tStr}, true, nil
	case "replacestr", "countstr":
		// The case flag must be a number, and the count may not be 0.
		// rAthena ends the script for both.
		opts := []string{}
		first := 3
		if name == "countstr" {
			first = 2
		}
		for i := first; i < len(vals); i++ {
			if vals[i].t == tStr {
				return val{}, true, unsupported(name, "a case flag that is a string")
			}
			opts = append(opts, num(i))
		}
		code := "R." + name + "(" + str(0) + ", " + str(1)
		if name == "replacestr" {
			code += ", " + str(2)
		}
		for _, o := range opts {
			code += ", " + o
		}
		t := tStr
		if name == "countstr" {
			t = tInt
		}
		return val{code: code + ")", t: t}, true, nil
	case "pow":
		return val{code: "R.pow(" + num(0) + ", " + num(1) + ")", t: tInt}, true, nil
	case "distance":
		return val{code: fmt.Sprintf("R.distance(%s, %s, %s, %s)", num(0), num(1), num(2), num(3)), t: tInt}, true, nil
	}
	return val{}, false, nil
}

// printfSpec matches a conversion of sprintf's format that converts: %d
// or %i with the flags -, +, space and 0, or %s with -, each with a
// width and a precision of at most two digits, which Lua's string.format
// takes as C's sprintf does.
var printfSpec = regexp.MustCompile(`^%(?:([-+ 0]*)[0-9]{0,2}(?:\.[0-9]{1,2})?([di])|-*[0-9]{0,2}(?:\.[0-9]{1,2})?(s))`)

// sprintf converts sprintf(<format>, ...) for a fixed format whose
// conversions are %d, %i and %s, with arguments of the type each takes
// (buildin_sprintf). Without arguments rAthena returns the format as it
// is, %% too.
func (b *body) sprintf(args []Expr) (val, error) {
	const what = "sprintf"
	f, ok := args[0].(*Str)
	if !ok {
		// R.sprintf reads a computed format when it runs, with the same
		// conversions, and the types of the values it gets.
		vals, err := b.args(args)
		if err != nil {
			return val{}, err
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			parts[i] = v.code
			if v.t == tBool {
				parts[i] = v.asInt()
			}
		}
		if vals[0].t != tStr && vals[0].t != tAny {
			return val{}, unsupported(what, "a format that is a number")
		}
		return val{code: "R.sprintf(" + strings.Join(parts, ", ") + ")", t: tStr}, nil
	}
	if len(args) == 1 {
		return strVal(f.Value), nil
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return val{}, err
	}
	var out strings.Builder
	rest := f.Value
	used := 0
	for {
		i := strings.IndexByte(rest, '%')
		if i < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:i])
		rest = rest[i:]
		if strings.HasPrefix(rest, "%%") {
			out.WriteString("%%")
			rest = rest[2:]
			continue
		}
		m := printfSpec.FindStringSubmatch(rest)
		if m == nil {
			return val{}, unsupported(what, "a conversion other than %%d, %%i and %%s")
		}
		if used >= len(vals) {
			// rAthena returns "" and reports too few arguments.
			return val{}, unsupported(what, "fewer arguments than conversions")
		}
		v := vals[used]
		spec := m[0]
		if m[3] == "s" {
			if v.t != tStr {
				return val{}, unsupported(what, "%%s of a value that is not a string")
			}
		} else {
			if v.t == tStr || v.t == tAny {
				return val{}, unsupported(what, "%%d of a value that is not a number")
			}
			vals[used].code = v.asInt()
			spec = spec[:len(spec)-1] + "d"
		}
		out.WriteString(spec)
		rest = rest[len(m[0]):]
		used++
	}
	parts := []string{luaString(out.String())}
	for _, v := range vals {
		parts = append(parts, v.code)
	}
	return val{code: "string.format(" + strings.Join(parts, ", ") + ")", t: tStr}, nil
}

// getmapxy converts getmapxy(<map$>, <x>, <y>{, <type>}) for the
// attached player (BL_PC, the default) or the NPC itself (BL_NPC). It
// sets the variables and returns 0, or returns -1 and sets nothing
// without the unit. A name or a game ID of another unit needs lookups
// the API does not have.
func (b *body) getmapxy(args []Expr) (string, error) {
	const what = "getmapxy"
	if len(args) < 3 || len(args) > 5 {
		return "", unsupported(what, "arguments")
	}
	if len(args) == 5 {
		return "", unsupported(what, "of another unit by name or game ID")
	}
	unit := "player"
	if len(args) == 4 {
		t, err := b.expr(args[3])
		if err != nil {
			return "", err
		}
		pc, _, _ := b.n.c.consts.Lookup("BL_PC")
		npc, _, _ := b.n.c.consts.Lookup("BL_NPC")
		switch {
		case t.static != nil && *t.static == pc:
		case t.static != nil && *t.static == npc:
			unit = "npc"
		default:
			return "", unsupported(what, "of a unit other than the player or the NPC")
		}
	}
	var targets [3]*Var
	for i := range targets {
		v, ok := asVar(args[i])
		if ok {
			if _, _, str := varName(v.Name); str != (i == 0) {
				ok = false
			}
		}
		if !ok {
			return "", unsupported(what, "into values that are not a string and two number variables")
		}
		targets[i] = v
	}
	code := b.hide("mapxy")
	m, x, y := b.hide("mapxy_map"), b.hide("mapxy_x"), b.hide("mapxy_y")
	b.line("%s, %s, %s, %s = R.mapxy(%s)", code, m, x, y, unit)
	b.line("if %s == 0 then", code)
	err := func() error {
		b.indent++
		defer func() { b.indent-- }()
		for i, t := range []val{{code: m, t: tStr}, {code: x, t: tInt}, {code: y, t: tInt}} {
			if err := b.writeVar(targets[i], t); err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return "", err
	}
	b.line("end")
	return code, nil
}

// scriptArray is the Lua table of an array that inarray, copyarray and
// explode take: a .@ array, or the NPC's . array, with its reference.
func (b *body) scriptArray(what string, e Expr) (string, *Var, error) {
	v, ok := asVar(e)
	if ok {
		switch p, _, _ := varName(v.Name); p {
		case ".@":
			if l := b.local(v.Name); l.array {
				return l.lua, v, nil
			}
		case ".":
			if nv, err := b.variable(v.Name); err == nil && nv.scope == "npc" && nv.array {
				return nv.local, v, nil
			}
		}
	}
	return "", nil, unsupported(what, "an array that is not a .@ or . array of the script")
}

// index is the index of an array reference, 0 without one.
func (b *body) index(v *Var) (string, error) {
	if v.Index == nil {
		return "0", nil
	}
	i, err := b.expr(v.Index)
	if err != nil {
		return "", err
	}
	return i.asInt(), nil
}

// inarray converts inarray <array>, <value>: the first index from 0
// whose element equals the value, or -1 (buildin_inarray). A string
// array compares strings, and a number array numbers.
func (b *body) inarray(args []Expr) (val, error) {
	const what = "inarray"
	if len(args) != 2 {
		return val{}, unsupported(what, "arguments")
	}
	t, v, err := b.scriptArray(what, args[0])
	if err != nil {
		return val{}, err
	}
	x, err := b.expr(args[1])
	if err != nil {
		return val{}, err
	}
	_, _, str := varName(v.Name)
	value := x.asInt()
	switch {
	case str && x.t != tStr:
		value = "tostring(" + x.asInt() + ")"
	case str:
		value = x.code
	case x.t == tStr || x.t == tAny:
		// script_getnum64 turns a string into a number, which the
		// converter does not do.
		return val{}, unsupported(what, "a value that may be a string for an array of numbers")
	}
	def := "0"
	if str {
		def = `""`
	}
	return val{code: fmt.Sprintf("R.inarray(%s, %s, %s)", t, value, def), t: tInt}, nil
}

// copyarray converts copyarray <to>, <from>, <count>
// (buildin_copyarray). Both arrays must be of the same type.
func (b *body) copyarray(args []Expr) error {
	const what = "copyarray"
	if len(args) != 3 {
		return unsupported(what, "arguments")
	}
	to, tv, err := b.scriptArray(what, args[0])
	if err != nil {
		return err
	}
	from, fv, err := b.scriptArray(what, args[1])
	if err != nil {
		return err
	}
	_, _, s1 := varName(tv.Name)
	_, _, s2 := varName(fv.Name)
	if s1 != s2 {
		// rAthena ends the script on a type mismatch.
		return unsupported(what, "arrays of different types")
	}
	i1, err := b.index(tv)
	if err != nil {
		return err
	}
	i2, err := b.index(fv)
	if err != nil {
		return err
	}
	n, err := b.expr(args[2])
	if err != nil {
		return err
	}
	def := "0"
	if s1 {
		def = `""`
	}
	b.line("R.copyarray(%s, %s, %s, %s, %s, %s)", to, i1, from, i2, n.asInt(), def)
	return nil
}

// explode converts explode <array$>, <text>, <delimiter>: the text's
// parts between the delimiter's first character, into the array from
// its index (buildin_explode).
func (b *body) explode(args []Expr) error {
	const what = "explode"
	if len(args) != 3 {
		return unsupported(what, "arguments")
	}
	t, v, err := b.scriptArray(what, args[0])
	if err != nil {
		return err
	}
	if _, _, str := varName(v.Name); !str {
		// rAthena ends the script for an array that is not of strings.
		return unsupported(what, "into an array that is not of strings")
	}
	i, err := b.index(v)
	if err != nil {
		return err
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	text := func(v val) string {
		if v.t == tStr {
			return v.code
		}
		return "tostring(" + v.asInt() + ")"
	}
	b.line("R.explode(%s, %s, %s, %s)", t, i, text(vals[0]), text(vals[1]))
	return nil
}

// warpComputed converts warp to a map that is not fixed: R.warp checks
// "Random", "SavePoint" and "Save" when it runs, as buildin_warp does.
func (b *body) warpComputed(m val, args []Expr) error {
	x, err := b.expr(args[1])
	if err != nil {
		return err
	}
	y, err := b.expr(args[2])
	if err != nil {
		return err
	}
	b.line("R.warp(player, %s, %s, %s)", m.asText(), x.asInt(), y.asInt())
	return nil
}

// getgroupitem converts getgroupitem <group>{, <identify>}:
// player:give_item_from_group. wAthena's items have no unidentified
// state yet, so the identify flag changes nothing.
func (b *body) getgroupitem(args []Expr) error {
	const what = "getgroupitem"
	if len(args) < 1 || len(args) > 2 {
		return unsupported(what, "for another character")
	}
	if len(args) == 2 {
		if v, err := b.expr(args[1]); err != nil || v.static == nil {
			return unsupported(what, "a computed identify flag")
		}
	}
	g, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if g.static == nil {
		return unsupported(what, "a computed group")
	}
	name, ok := b.n.c.groups[*g.static]
	if !ok {
		return unsupported(what, "%d is not an item group", *g.static)
	}
	b.line("player:give_item_from_group(%s)", b.n.file.record("items.group", name))
	return nil
}

// getdName is the part of a getd or setd name that sscanf reads with
// "%99[^[][%11d]" (buildin_getd, script.cpp 18155): the name up to the
// first [, and the number after it, or 0.
func getdName(s string) (string, int64) {
	name, rest, found := strings.Cut(s, "[")
	if !found {
		return name, 0
	}
	rest = strings.TrimLeft(rest, " \t\n\v\f\r")
	end := 0
	if end < len(rest) && (rest[0] == '-' || rest[0] == '+') {
		end++
	}
	for end < len(rest) && end < 11 && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	n, err := strconv.ParseInt(rest[:end], 10, 32)
	if err != nil {
		return name, 0
	}
	return name, n
}

// variableRef matches a variable's name with its prefix and $ suffix.
var variableRef = regexp.MustCompile(`^(\.@|\.|\$@|\$|##|#|@|')?[A-Za-z_][A-Za-z0-9_]*\$?$`)

// staticGetd reads getd("<name>") of a fixed name: that variable, with
// its index.
func staticGetd(c *Call) (*Var, bool) {
	if !strings.EqualFold(c.Name, "getd") || len(c.Args) != 1 {
		return nil, false
	}
	s, ok := c.Args[0].(*Str)
	if !ok {
		return nil, false
	}
	name, i := getdName(s.Value)
	if !variableRef.MatchString(name) {
		return nil, false
	}
	v := &Var{Name: name}
	if i != 0 {
		v.Index = &Num{Value: i}
	}
	return v, true
}

// dynamicVar is a getd or setd name made when the script runs: the
// variables of its scope, the Lua code of the name, and the type of its
// value when the converter can tell it.
type dynamicVar struct {
	scope, name string
	t           valType
}

// dynamicName reads a computed getd or setd name. It converts when the
// text starts with a fixed string that fixes the scope: no prefix (a
// character variable), @, # or ##, or $. R.getd and R.setd read the
// rest of the name, its $ and its index when the script runs, as getd
// does. These do not convert:
//   - .@ and . variables, which are Lua locals with names fixed when the
//     script converts.
//   - $@ and ' variables, which need temporary server and instance
//     variables.
//   - A start that a currency's variable, a parameter, a constant or a
//     skill also starts with: getd would read those, not a variable.
func (b *body) dynamicName(what string, e Expr) (*dynamicVar, error) {
	var parts []Expr
	var flatten func(e Expr)
	flatten = func(e Expr) {
		if bin, ok := e.(*Binary); ok && bin.Op == "+" {
			flatten(bin.X)
			flatten(bin.Y)
			return
		}
		parts = append(parts, e)
	}
	flatten(e)
	first, ok := parts[0].(*Str)
	if !ok {
		return nil, unsupported(what, "a name that does not start with fixed text")
	}
	text := first.Value
	prefix := regexp.MustCompile(`^(\.@|\.|\$@|\$|##|#|@|')?`).FindString(text)
	rest := text[len(prefix):]
	if rest == "" || !variableRef.MatchString(rest[:1]) {
		return nil, unsupported(what, "a name whose fixed start does not fix its scope")
	}
	scope := map[string]string{"": "character", "@": "character_temporary", "#": "account", "##": "account", "$": "server"}[prefix]
	switch prefix {
	case ".@", ".":
		return nil, unsupported(what, "a computed name of a .@ or . variable, which are Lua locals")
	case "$@":
		return nil, unsupported("variables", "%s... is a temporary server variable", text)
	case "'":
		return nil, unsupported("variables", "%s... is an instance variable", text)
	}
	lower := strings.ToLower(text)
	c := b.n.c
	for name := range c.currencyVars {
		if strings.HasPrefix(name, lower) {
			return nil, unsupported(what, "a name that may be the currency %s", name)
		}
	}
	if prefix == "" {
		for _, names := range []map[string]int64{c.consts.Constants, c.consts.Parameters, c.consts.Skills} {
			for name := range names {
				if strings.HasPrefix(strings.ToLower(name), lower) {
					return nil, unsupported(what, "a name that may be %s, which is not a variable", name)
				}
			}
		}
	}
	vals, err := b.args(parts)
	if err != nil {
		return nil, err
	}
	code := make([]string, len(vals))
	for i, v := range vals {
		if v.t == tStr {
			code[i] = v.code
		} else {
			code[i] = "tostring(" + v.asInt() + ")"
		}
	}
	// The value is a string when the name ends with $: before the first
	// [, or at the end without one. The end is in the part i, or in a
	// computed part before it, whose type may tell its last character.
	end := len(parts) - 1
	tail, found := "", false
	for i, p := range parts {
		if s, ok := p.(*Str); ok && strings.Contains(s.Value, "[") {
			end, found = i, true
			tail, _, _ = strings.Cut(s.Value, "[")
			break
		}
	}
	if !found {
		if s, ok := parts[end].(*Str); ok {
			tail = s.Value
		}
	}
	t := tAny
	switch {
	case tail != "":
		t = tInt
		if strings.HasSuffix(tail, "$") {
			t = tStr
		}
	case found && end > 0:
		// [ starts the part: the part before it ends the name.
		if s, ok := parts[end-1].(*Str); ok {
			t = tInt
			if strings.HasSuffix(s.Value, "$") {
				t = tStr
			}
		} else if vals[end-1].t == tInt || vals[end-1].t == tBool {
			t = tInt
		}
	case !found && (vals[end].t == tInt || vals[end].t == tBool):
		// A number ends with a digit.
		t = tInt
	}
	return &dynamicVar{scope: scope, name: strings.Join(code, " .. "), t: t}, nil
}

// getd converts getd("<name>"): a variable by a name made when the
// script runs, or a fixed name.
func (b *body) getd(args []Expr) (val, error) {
	const what = "getd"
	if len(args) != 1 {
		return val{}, unsupported(what, "arguments")
	}
	if v, ok := asVar(&Call{Name: "getd", Args: args}); ok {
		return b.readVar(v.Name, v.Index)
	}
	d, err := b.dynamicName(what, args[0])
	if err != nil {
		return val{}, err
	}
	return val{code: fmt.Sprintf("R.getd(%q, %s, player)", d.scope, d.name), t: d.t}, nil
}

// setd converts setd "<name>", <value>; and set getd("<name>"), <value>;.
func (b *body) setd(what string, args []Expr) error {
	if len(args) != 2 {
		return unsupported(what, "arguments, such as a character ID")
	}
	value, err := b.expr(args[1])
	if err != nil {
		return err
	}
	if v, ok := asVar(&Call{Name: "getd", Args: args[:1]}); ok {
		if p, _, _ := varName(v.Name); p != ".@" && v.Index != nil {
			// A saved array's element; the variable must be an array.
			if nv, err := b.variable(v.Name); err == nil && !nv.array {
				return unsupported(what, "an element of %s, which the script does not use as an array", v.Name)
			}
		}
		if p, _, _ := varName(v.Name); p == ".@" && v.Index != nil && !b.local(v.Name).array {
			return unsupported(what, "an element of %s, which the script does not use as an array", v.Name)
		}
		return b.writeVar(v, value)
	}
	d, err := b.dynamicName(what, args[0])
	if err != nil {
		return err
	}
	code := value.code
	if value.t == tBool {
		code = value.asInt()
	}
	b.line("R.setd(%q, %s, player, %s)", d.scope, d.name, code)
	return nil
}
