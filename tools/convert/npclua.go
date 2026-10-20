package convert

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The translation of NPC scripts and functions into Lua.
//
// Values have one of three types: integers, strings, and booleans, which
// rAthena keeps as 0 and 1. A value read from getarg or a function has
// no known type, and is tested with R.truthy.
//
// rAthena's variables become:
//   - .@x: a Lua local of the function, declared at its top, so labels
//     and goto work.
//   - .x: a local of the NPC's setup function, which every click shares.
//     A .x that getvariableofnpc reads from another NPC is an NPC
//     variable of wa.variables instead, which every zone reads (review
//     decision 58).
//   - x, @x, #x, ##x, $x and $@x: variables of wa.variables, of the
//     character, the character until logout, the account, the server,
//     and the server until it stops. ## is an account variable too.
//   - Arrays are tables with keys from 0. x and x[0] are the same.
//   - The type is integer, or string for names ending in $.

type valType int

const (
	tInt valType = iota
	tStr
	tBool
	tAny
)

// val is a converted expression.
type val struct {
	code string
	t    valType
	// static is set for an integer known at conversion time.
	static *int64
	// str is set for a string known at conversion time.
	str *string
}

func intVal(n int64) val  { return val{code: strconv.FormatInt(n, 10), t: tInt, static: &n} }
func strVal(s string) val { return val{code: luaString(s), t: tStr, str: &s} }

// asInt is the value as an integer.
func (v val) asInt() string {
	switch v.t {
	case tBool:
		return "(" + v.code + " and 1 or 0)"
	case tAny:
		return "(" + v.code + " or 0)"
	}
	return v.code
}

// asBool is the value as a condition.
func (v val) asBool() string {
	switch v.t {
	case tBool:
		return v.code
	case tStr:
		return "(" + v.code + " ~= \"\")"
	case tAny:
		return "R.truthy(" + v.code + ")"
	}
	if v.static != nil {
		return strconv.FormatBool(*v.static != 0)
	}
	return "(" + v.code + " ~= 0)"
}

// asText is the value in a string.
func (v val) asText() string {
	if v.t == tBool {
		return v.asInt()
	}
	return v.code
}

// npcVar is a variable that lives outside a Lua function: a variable of
// wa.variables, or an NPC variable.
type npcVar struct {
	// scope is character, character_temporary, account, server or
	// server_temporary for wa.variables, npc for a local of the setup
	// function, and npc_shared for an NPC variable of wa.variables.
	scope string
	name  string
	str   bool
	array bool
	local string
	// of is the owner of an NPC variable when it is not the NPC itself:
	// sourceVar, the source of an NPC with duplicates, which share its
	// .x variables as rAthena's duplicates share script_code->local.
	of string
}

// sourceVar is the Lua local of the source NPC of a script that
// duplicates run.
const sourceVar = "NpcSource"

// localVar is a .@ variable.
type localVar struct {
	lua   string
	str   bool
	array bool
}

// npcConv converts one NPC script or one function.
type npcConv struct {
	c    *npcConverter
	line *npcLine
	file *luaFile
	// vars are the variables of wa.variables and the NPC variables the
	// file uses, by lower-case rAthena name.
	vars     map[string]*npcVar
	varOrder []string
	// arrays are the lower-case names used as arrays.
	arrays   map[string]bool
	features map[string]bool
	calls    map[string]bool
	// stubs are the parts of a function that did not convert. An NPC's
	// own script allows none.
	stubs      []string
	allowStubs bool
	isFunction bool
	names      map[string]bool
	counter    int
	// eventLabels are the script's labels that start with On, in lower
	// case. eventFns are the local functions of the labels that
	// schedules and monsters' deaths run, in the order of eventOrder.
	eventLabels map[string]bool
	eventFns    map[string]string
	eventOrder  []string
	// bodyCalls, when set, collects the functions that the part being
	// written calls, for checkTouchBody.
	bodyCalls map[string]bool
	// touch is set when the script has a touch handler: OnTouch or
	// OnTouch_. touchDialog is set when it shows a dialog.
	touch, touchDialog bool
	// all is the script's top level, where callsub finds its labels.
	// localFuncs are its local functions by lower-case name.
	all        []Stmt
	localFuncs map[string]*FuncDecl
	// refArgs are, for a global function, the arguments its callers pass
	// by reference (refArgsOf).
	refArgs map[int]bool
	// returns says whether the part that callsub runs from a label can
	// return, by lower-case label (subReturns).
	returns map[string]bool
	// timer is set when the script uses its NPC timer (npctimer.go), and
	// timerLabels are its OnTimer labels.
	timer       bool
	timerLabels []timerLabel
	// sourceShared is set when the script reads .x variables that its
	// duplicates share (sourceVar).
	sourceShared bool
	// npcIDs are the variables that only hold getnpcid's value, with the
	// NPC's code, once npcIDVars read them (npcpresent.go).
	npcIDs map[string]string
	// inEvent is set while an event label's body is written, which may
	// run without a player.
	inEvent bool
}

func (c *npcConverter) newNPCConv(l *npcLine) *npcConv {
	return &npcConv{c: c, line: l, file: newLuaFile(), vars: map[string]*npcVar{}, arrays: map[string]bool{},
		features: map[string]bool{}, calls: map[string]bool{}, names: map[string]bool{},
		eventLabels: map[string]bool{}, eventFns: map[string]string{}, localFuncs: map[string]*FuncDecl{},
		returns: map[string]bool{}}
}

// prepare reads what the conversion of a script's statements needs
// first: its top level, its local functions, and its arrays.
func (n *npcConv) prepare(stmts []Stmt) {
	n.all = stmts
	walkStmts(stmts, func(s Stmt) {
		if f, ok := s.(*FuncDecl); ok && f.Body != nil {
			n.localFuncs[strings.ToLower(f.Name)] = f
		}
	})
	n.scanArrays(stmts)
}

func (n *npcConv) functions() []string {
	direct := make([]string, 0, len(n.calls))
	for k := range n.calls {
		direct = append(direct, k)
	}
	return n.c.functionsOf(direct)
}

func (n *npcConv) ok(feature string) {
	if _, seen := n.features[feature]; !seen {
		n.features[feature] = true
	}
}

// fail records a feature that did not convert.
func (n *npcConv) fail(feature string) { n.features[feature] = false }

// fresh returns a Lua name that nothing in the file uses yet.
func (n *npcConv) fresh(base string) string {
	n.counter++
	return fmt.Sprintf("%s_%d", base, n.counter)
}

// reserved are names converted code uses for itself.
var reserved = map[string]bool{
	"player": true, "conversation": true, "npc": true, "args": true, "R": true, "events": true, "context": true,
	"variables": true, "items": true, "skills": true, "random": true, "integers": true, "emotions": true,
	"effects": true, "jobs": true, "text": true, "maps": true, "world": true, "require": true, "self": true,
	"string": true, "table": true, "math": true, "select": true, "type": true, "tostring": true, "tonumber": true,
	"pairs": true, "ipairs": true, "error": true, "pcall": true, "schedules": true, "spawning": true, "monsters": true,
	"channels": true, "parties": true, "guilds": true, "rules": true,
}

// luaName turns an rAthena name into a Lua name: lower case, letters,
// digits and _, not a keyword or a reserved name.
func luaName(name string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(name) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' {
			b.WriteRune(c)
		}
	}
	s := b.String()
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		s = "v" + s
	}
	if luaKeywords[s] || reserved[s] {
		s += "_"
	}
	return s
}

// varName splits a variable's name into its prefix, its bare name, and
// whether it is a string.
func varName(name string) (prefix, bare string, str bool) {
	str = strings.HasSuffix(name, "$")
	name = strings.TrimSuffix(name, "$")
	for _, p := range []string{".@", "$@", "##", ".", "@", "$", "#", "'"} {
		if strings.HasPrefix(name, p) {
			return p, name[len(p):], str
		}
	}
	return "", name, str
}

// scanArrays marks the variables a script uses as arrays: with an index,
// or in an array command.
func (n *npcConv) scanArrays(stmts []Stmt) {
	var expr func(e Expr)
	mark := func(e Expr) {
		if v, ok := asVar(e); ok {
			n.arrays[strings.ToLower(v.Name)] = true
		}
	}
	expr = func(e Expr) {
		switch e := e.(type) {
		case *Var:
			if e.Index != nil {
				mark(e)
				expr(e.Index)
			}
		case *Call:
			switch strings.ToLower(e.Name) {
			case "getarraysize", "implode", "getelementofarray":
				if len(e.Args) > 0 {
					mark(e.Args[0])
				}
			case "getd":
				if v, ok := staticGetd(e); ok && v.Index != nil {
					mark(v)
				}
			}
			n.markRefArgs(e.Name, e.Args, mark)
			for _, a := range e.Args {
				expr(a)
			}
		case *Unary:
			expr(e.X)
		case *Binary:
			expr(e.X)
			expr(e.Y)
		case *Ternary:
			expr(e.Cond)
			expr(e.Then)
			expr(e.Else)
		case *Assign:
			expr(e.Target)
			expr(e.Value)
		}
	}
	var stmt func(s Stmt)
	stmt = func(s Stmt) {
		switch s := s.(type) {
		case *Block:
			for _, x := range s.Stmts {
				stmt(x)
			}
		case *If:
			expr(s.Cond)
			stmt(s.Then)
			if s.Else != nil {
				stmt(s.Else)
			}
		case *Switch:
			expr(s.Value)
			for _, x := range s.Pre {
				stmt(x)
			}
			for _, cs := range s.Cases {
				for _, x := range cs.Body {
					stmt(x)
				}
			}
		case *For:
			if s.Init != nil {
				stmt(s.Init)
			}
			if s.Cond != nil {
				expr(s.Cond)
			}
			if s.Post != nil {
				stmt(s.Post)
			}
			stmt(s.Body)
		case *While:
			expr(s.Cond)
			stmt(s.Body)
		case *Command:
			switch strings.ToLower(s.Name) {
			case "setarray", "deletearray", "cleararray", "copyarray", "explode":
				if len(s.Args) > 0 {
					mark(s.Args[0])
				}
			case "setd":
				if len(s.Args) > 0 {
					if v, ok := staticGetd(&Call{Name: "getd", Args: s.Args[:1]}); ok && v.Index != nil {
						mark(v)
					}
				}
			}
			n.markRefArgs(s.Name, s.Args, mark)
			for _, a := range s.Args {
				expr(a)
			}
		case *SetRef:
			expr(s.Ref)
			expr(s.Value)
		case *Assign:
			expr(s.Target)
			expr(s.Value)
		case *IncDec:
			expr(s.Target)
		case *Return:
			if s.Value != nil {
				expr(s.Value)
			}
		case *FuncDecl:
			if s.Body != nil {
				stmt(s.Body)
			}
		}
	}
	for _, s := range stmts {
		stmt(s)
	}
}

// body is one Lua function being written: the click handler, the setup
// function, a function module, or a local function.
type body struct {
	n      *npcConv
	lines  []string
	indent int
	locals map[string]*localVar
	order  []string
	// main is set for the click handler and the setup function, where
	// end and close return.
	main bool
	// hasArgs is set where getarg reads the function's arguments.
	hasArgs bool
	// labels are the labels of the function's top level. deepLabels
	// are the labels inside blocks, with the path of the block.
	labels     map[string]bool
	deepLabels map[string][]int
	gotos      []gotoRef
	breaks     []*breakable
	// path is the Lua blocks the statement being written is in, by
	// number, from the outermost. blocks counts them.
	path   []int
	blocks int
	// subs are the callsub targets, written as local functions, and funcs
	// the local functions of the script that it calls. Sub bodies share
	// them.
	subs, funcs map[string]string
	// refArgs are the arguments of a global function that its callers
	// pass by reference (npcConv.refArgs). Only its own body has them.
	refArgs map[int]bool
	// prev is the statement before the one being written in its list,
	// or nil.
	prev Stmt
	// hidden are locals the converter adds, such as the result of an
	// input, declared at the top with the others.
	hidden []string
	// rebirth is set after jobchange Job_Novice_High, which converts to
	// player:rebirth, so the resetlvl(1) after it converts to nothing.
	rebirth bool
	// partyNames, partyCIDs, partyAIDs and partyCount are the locals
	// that getpartymember sets, for reads of $@partymembername$,
	// $@partymembercid, $@partymemberaid and $@partymembercount.
	partyNames, partyCIDs, partyAIDs, partyCount string
	// guildNames and guildCount are the locals that getguildmember sets,
	// for reads of $@guildmembername$ and $@guildmembercount.
	guildNames, guildCount string
}

// gotoRef is a jump to a label, with the path of the block it is in.
type gotoRef struct {
	label string
	path  []int
}

// jumpTo records a jump to a label from the current block.
func (b *body) jumpTo(label string) {
	b.gotos = append(b.gotos, gotoRef{label: strings.ToLower(label), path: slices.Clone(b.path)})
}

// enter starts a Lua block, and the function it returns ends it.
func (b *body) enter() func() {
	b.blocks++
	b.path = append(b.path, b.blocks)
	return func() { b.path = b.path[:len(b.path)-1] }
}

// breakable is a loop or a switch, which break leaves.
type breakable struct {
	loop bool
	// end is the label after a switch. cont is the label before a
	// loop's step, used when its body has continue.
	end, cont string
	usesCont  bool
}

func (n *npcConv) newBody(indent int) *body {
	return &body{n: n, indent: indent, locals: map[string]*localVar{}, labels: map[string]bool{}, deepLabels: map[string][]int{},
		subs: map[string]string{}, funcs: map[string]string{}}
}

func (b *body) line(format string, args ...any) {
	b.lines = append(b.lines, strings.Repeat("  ", b.indent)+fmt.Sprintf(format, args...))
}

// text returns the function's lines: the locals first, then the local
// functions of callsub, then the statements.
func (b *body) text(subs []string) ([]string, bool) {
	pad := strings.Repeat("  ", b.indent)
	var out []string
	if len(b.hidden) > 0 {
		out = append(out, pad+"local "+strings.Join(b.hidden, ", "))
	}
	if len(b.order) > 0 {
		names := make([]string, len(b.order))
		vals := make([]string, len(b.order))
		for i, k := range b.order {
			l := b.locals[k]
			names[i] = l.lua
			switch {
			case l.array:
				vals[i] = "{}"
			case l.str:
				vals[i] = `""`
			default:
				vals[i] = "0"
			}
		}
		// Lua allows 200 locals per function.
		if len(names) > 150 {
			return nil, false
		}
		out = append(out, pad+"local "+strings.Join(names, ", ")+" = "+strings.Join(vals, ", "))
	}
	out = append(out, subs...)
	return append(out, b.lines...), true
}

// unsupported records a part that does not convert. In a function it
// becomes a call that fails when a script reaches it. Elsewhere the
// conversion fails.
func (b *body) unsupported(err error) error {
	n := b.n
	feature := errFeature(err)
	n.fail(feature)
	if !n.allowStubs {
		return err
	}
	n.stubs = append(n.stubs, describeError(feature, err))
	b.line("R.unsupported(%s)", luaString(feature))
	return nil
}

// hide adds a local of the converter's own, declared at the top of the
// function, so a goto never jumps into its scope.
func (b *body) hide(base string) string {
	name := b.n.fresh(base)
	b.hidden = append(b.hidden, name)
	return name
}

// local returns the Lua local of a .@ variable, declared at the top of
// the function.
func (b *body) local(name string) *localVar {
	key := strings.ToLower(name)
	if l, ok := b.locals[key]; ok {
		return l
	}
	_, bare, str := varName(name)
	lua := luaName(bare)
	for {
		taken := false
		for _, l := range b.locals {
			if l.lua == lua {
				taken = true
			}
		}
		if !taken && !b.n.names[lua] {
			break
		}
		lua += "_"
	}
	l := &localVar{lua: lua, str: str, array: b.n.arrays[key]}
	b.locals[key] = l
	b.order = append(b.order, key)
	return l
}

// variable returns the declaration of a variable that lives outside the
// function.
func (b *body) variable(name string) (*npcVar, error) {
	n := b.n
	key := strings.ToLower(name)
	if v, ok := n.vars[key]; ok {
		return v, nil
	}
	prefix, bare, str := varName(name)
	scope := ""
	switch prefix {
	case "":
		scope = "character"
	case "@":
		scope = "character_temporary"
	case "#", "##":
		scope = "account"
	case "$":
		scope = "server"
	case ".":
		if n.isFunction {
			return nil, unsupported("variables", "%s is an NPC variable in a function", name)
		}
		scope = "npc"
		if n.c.sharedVar(n.line.Name, key) {
			scope = "npc_shared"
		}
		if n.c.duplicated[n.line.Name] {
			// npc_parse_duplicate copies the script, whose .x variables
			// every duplicate shares with the source (npc.cpp 4596).
			scope = "npc_shared"
			n.sourceShared = true
		}
	case "$@":
		scope = "server_temporary"
	case "'":
		return nil, unsupported("variables", "%s is an instance variable", name)
	}
	if bare == "" || !variableNameOK(bare) {
		return nil, unsupported("variables", "%s is not a valid name", name)
	}
	// rAthena keeps x and x$ apart, so string variables get a suffix.
	declared := strings.ToLower(bare)
	if str {
		declared += "_text"
	}
	v := &npcVar{scope: scope, name: declared, str: str, array: n.arrays[key]}
	if scope == "npc_shared" && n.c.duplicated[n.line.Name] {
		v.of = sourceVar
	}
	if scope == "npc" {
		v.local = "npc_" + luaName(bare)
	} else {
		v.local = map[string]string{"character": "", "character_temporary": "TEMP_", "account": "ACCOUNT_", "server": "SERVER_",
			"server_temporary": "SERVER_TEMP_", "npc_shared": "NPC_"}[scope] + constName(bare)
		if str {
			v.local += "_TEXT"
		}
	}
	for _, o := range n.vars {
		if o.local == v.local {
			v.local += "_2"
		}
	}
	n.vars[key] = v
	n.varOrder = append(n.varOrder, key)
	return v, nil
}

func variableNameOK(s string) bool {
	if len(s) > 64 || s == "" || s[0] >= '0' && s[0] <= '9' {
		return false
	}
	for _, c := range s {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// declaration is the line that declares a variable at the top of the
// file.
func (v *npcVar) declaration() string {
	kind := "integer"
	switch {
	case v.array:
		kind = "table"
	case v.str:
		kind = "string"
	}
	scope := v.scope
	if scope == "npc_shared" {
		scope = "npc"
	}
	return fmt.Sprintf("local %s = variables.%s_%s %s", v.local, scope, kind, luaString(v.name))
}

// owner is the argument of a variable's methods: none for a server
// variable, the NPC for an NPC variable, and the player otherwise.
func (v *npcVar) owner() string {
	if v.of != "" {
		return v.of
	}
	switch v.scope {
	case "server", "server_temporary":
		return "nil"
	case "npc_shared":
		return "npc"
	}
	return "player"
}

// get, set and add are a variable's methods, with the owner first when
// it has one.
func (v *npcVar) get() string {
	if o := v.owner(); o != "nil" {
		return v.local + ":get(" + o + ")"
	}
	return v.local + ":get()"
}

func (v *npcVar) set(value string) string {
	if o := v.owner(); o != "nil" {
		return v.local + ":set(" + o + ", " + value + ")"
	}
	return v.local + ":set(" + value + ")"
}

func (v *npcVar) add(amount string) string {
	if o := v.owner(); o != "nil" {
		return v.local + ":add(" + o + ", " + amount + ")"
	}
	return v.local + ":add(" + amount + ")"
}

// readVar reads a variable or one element of an array.
func (b *body) readVar(name string, index Expr) (val, error) {
	if _, ok := b.npcIDVars()[strings.ToLower(name)]; ok && index == nil {
		return val{}, unsupported("getnpcid", "the game ID of an NPC, which only the unit commands take")
	}
	prefix, _, str := varName(name)
	t := tInt
	if str {
		t = tStr
	}
	def := "0"
	if str {
		def = `""`
	}
	idx := "0"
	if index != nil {
		iv, err := b.expr(index)
		if err != nil {
			return val{}, err
		}
		idx = iv.asInt()
	}
	if cur, ok := b.n.c.currencyVar(name); ok {
		return b.readCurrency(name, cur, index)
	}
	if prefix == "$@" {
		if v, ok, err := b.partyMemberVar(name, idx, false); ok {
			return v, err
		}
		if v, ok, err := b.guildMemberVar(name, idx, false); ok {
			return v, err
		}
	}
	if prefix == ".@" {
		l := b.local(name)
		if l.array {
			return val{code: fmt.Sprintf("(%s[%s] or %s)", l.lua, idx, def), t: t}, nil
		}
		return val{code: l.lua, t: t}, nil
	}
	v, err := b.variable(name)
	if err != nil {
		return val{}, err
	}
	if v.scope == "npc" {
		if v.array {
			return val{code: fmt.Sprintf("(%s[%s] or %s)", v.local, idx, def), t: t}, nil
		}
		return val{code: v.local, t: t}, nil
	}
	b.n.file.need("variables")
	if v.array {
		return val{code: fmt.Sprintf("R.aget(%s, %s, %s, %s)", v.local, v.owner(), idx, def), t: t}, nil
	}
	return val{code: v.get(), t: t}, nil
}

// writeVar writes a variable, or one element of an array.
func (b *body) writeVar(target *Var, value val) error {
	if _, ok := b.n.c.currencyVar(target.Name); ok {
		return unsupported(strings.ToUpper(target.Name), "written by a command, as only reads and assignments of a currency convert")
	}
	prefix, _, str := varName(target.Name)
	code := value.asInt()
	switch {
	case str && (value.t == tAny || value.t == tInt || value.t == tBool):
		// rAthena's set turns a number into its digits (conv_str).
		code = "R.text(" + value.asInt() + ")"
		if value.t == tAny {
			code = "R.text(" + value.code + ")"
		}
	case str:
		code = value.asText()
	case value.t == tStr:
		// A string into a number variable is conv_num: strtoll in base
		// 10, held to the int32 limits.
		code = "R.strtol(" + value.code + ", 10)"
	}
	idx := "0"
	if target.Index != nil {
		iv, err := b.expr(target.Index)
		if err != nil {
			return err
		}
		idx = iv.asInt()
	}
	if prefix == ".@" {
		l := b.local(target.Name)
		if l.array {
			b.line("%s[%s] = %s", l.lua, idx, code)
		} else {
			b.line("%s = %s", l.lua, code)
		}
		return nil
	}
	v, err := b.variable(target.Name)
	if err != nil {
		return err
	}
	if v.scope == "npc" {
		if v.array {
			b.line("%s[%s] = %s", v.local, idx, code)
		} else {
			b.line("%s = %s", v.local, code)
		}
		return nil
	}
	b.n.file.need("variables")
	if v.array {
		b.line("R.aset(%s, %s, %s, %s)", v.local, v.owner(), idx, code)
	} else {
		b.line("%s", v.set(code))
	}
	return nil
}

// asVar reports whether an expression names a variable, for commands
// that take one, such as input. getd of a fixed name is that variable.
func asVar(e Expr) (*Var, bool) {
	switch e := e.(type) {
	case *Var:
		return e, true
	case *Ident:
		return &Var{Name: e.Name}, true
	case *Call:
		return staticGetd(e)
	}
	return nil, false
}

// block writes statements.
func (b *body) block(stmts []Stmt) error {
	return b.stmts(stmts, false)
}

// stmts writes statements. Lines of mes in a row become one say.
func (b *body) stmts(list []Stmt, deep bool) error {
	for i := 0; i < len(list); i++ {
		if c, ok := list[i].(*Command); ok && strings.EqualFold(c.Name, "mes") && len(c.Args) > 0 {
			var all []val
			j := i
			for ; j < len(list); j++ {
				m, ok := list[j].(*Command)
				if !ok || !strings.EqualFold(m.Name, "mes") || len(m.Args) == 0 || len(all)+len(m.Args) > 32 {
					break
				}
				vals, err := b.args(m.Args)
				if err != nil {
					break
				}
				all = append(all, vals...)
			}
			if j > i {
				b.line("conversation:say(%s)", texts(all))
				b.n.ok("mes")
				i = j - 1
				continue
			}
		}
		b.prev = nil
		if i > 0 {
			b.prev = list[i-1]
		}
		if err := b.stmt(list[i], deep); err != nil {
			return err
		}
	}
	return nil
}

func (b *body) nested(s Stmt) error {
	b.indent++
	defer func() { b.indent-- }()
	defer b.enter()()
	if bl, ok := s.(*Block); ok {
		return b.nestedBlock(bl.Stmts)
	}
	if s == nil {
		return nil
	}
	b.prev = nil
	return b.stmt(s, true)
}

func (b *body) nestedBlock(stmts []Stmt) error {
	return b.stmts(stmts, true)
}

// stmt writes one statement. deep is set inside a block, where labels
// are not allowed.
func (b *body) stmt(s Stmt, deep bool) error {
	switch s := s.(type) {
	case *Block:
		b.line("do")
		if err := b.nested(s); err != nil {
			return err
		}
		b.line("end")
	case *Label:
		name := strings.ToLower(s.Name)
		if deep {
			// rAthena allows a label anywhere. In Lua only the block's own
			// jumps see it, which checkGotos checks.
			b.deepLabels[name] = slices.Clone(b.path)
		} else {
			b.labels[name] = true
		}
		b.line("::%s::", luaName(name))
	case *Goto:
		b.jumpTo(s.Label)
		b.line("goto %s", luaName(s.Label))
		b.n.ok("goto")
	case *FuncDecl:
		// rAthena jumps over a local function's body where it is written
		// (parse_syntax, script.cpp 1918). Calls run it (userCall).
		if s.Body != nil {
			b.n.ok("function")
		}
	case *SetRef:
		return b.setRef(s)
	case *Assign:
		return b.assign(s)
	case *IncDec:
		op := map[string]string{"++": "+=", "--": "-="}[s.Op]
		return b.assign(&Assign{Target: s.Target, Op: op, Value: &Num{Value: 1}})
	case *If:
		return b.ifStmt(s)
	case *Switch:
		return b.switchStmt(s)
	case *For:
		if s.Init != nil {
			if err := b.stmt(s.Init, deep); err != nil {
				return err
			}
		}
		cond := "true"
		if s.Cond != nil {
			c, err := b.expr(s.Cond)
			if err != nil {
				return b.unsupported(err)
			}
			cond = c.asBool()
		}
		return b.loop(cond, s.Body, s.Post, false)
	case *While:
		c, err := b.expr(s.Cond)
		if err != nil {
			return b.unsupported(err)
		}
		return b.loop(c.asBool(), s.Body, nil, s.Do)
	case *Jump:
		return b.jump(s.Kind)
	case *Return:
		if b.main {
			b.line("do return end")
			return nil
		}
		if s.Value == nil {
			b.line("do return end")
			return nil
		}
		v, err := b.expr(s.Value)
		if err != nil {
			return b.unsupported(err)
		}
		// rAthena functions return numbers: a condition returns 1 or 0.
		if v.t == tBool {
			v.code = v.asInt()
		}
		b.line("do return %s end", v.code)
	case *Command:
		return b.command(s)
	case *Unsupported:
		return b.unsupported(unsupported("script", "%s", s.Text))
	default:
		return b.unsupported(unsupported("statement", "%T", s))
	}
	return nil
}

// jump writes break, continue, end and close.
func (b *body) jump(kind string) error {
	switch kind {
	case "break", "continue":
		var target *breakable
		for i := len(b.breaks) - 1; i >= 0; i-- {
			if kind == "break" || b.breaks[i].loop {
				target = b.breaks[i]
				break
			}
		}
		if target == nil {
			return b.unsupported(unsupported(kind, "outside a loop or a switch"))
		}
		switch {
		case kind == "continue":
			target.usesCont = true
			b.line("goto %s", target.cont)
		case target.loop:
			b.line("break")
		default:
			b.line("goto %s", target.end)
		}
		return nil
	}
	// end and close end the script. In the click handler they return;
	// in a function they stop the whole script. close leaves the Close
	// button, which the runtime shows when the script ends with lines on
	// show.
	b.n.ok(kind)
	if b.main {
		b.line("do return end")
	} else {
		b.line("R.stop()")
	}
	return nil
}

func (b *body) assign(s *Assign) error {
	t := s.Target
	if _, ok := b.npcIDVars()[strings.ToLower(t.Name)]; ok && t.Index == nil {
		// The variable holds an NPC's game ID, which the unit commands
		// read as the NPC (npcpresent.go).
		b.line("-- %s holds the game ID of an NPC.", t.Name)
		return nil
	}
	if cur, ok := b.n.c.currencyVar(t.Name); ok {
		return b.assignCurrency(s, cur)
	}
	if p, _, _ := varName(t.Name); p == "" && t.Index == nil {
		if _, param, ok := b.n.c.consts.Lookup(t.Name); ok && param {
			return b.assignParam(s)
		}
	}
	// x += n on a saved integer is one add, so it cannot lose a change
	// made in between.
	if (s.Op == "+=" || s.Op == "-=") && t.Index == nil {
		if p, _, str := varName(t.Name); !str && p != ".@" {
			if v, err := b.variable(t.Name); err == nil && !v.array && v.scope != "npc" {
				d, err := b.expr(s.Value)
				if err != nil {
					return b.unsupported(err)
				}
				amount := d.asInt()
				if s.Op == "-=" {
					amount = "-" + amount
				}
				b.n.file.need("variables")
				b.line("%s", v.add(amount))
				b.n.ok("variables")
				return nil
			}
		}
	}
	value := Expr(s.Value)
	if s.Op != "=" {
		value = &Binary{Op: strings.TrimSuffix(s.Op, "="), X: t, Y: s.Value}
	}
	v, err := b.expr(value)
	if err != nil {
		return b.unsupported(err)
	}
	if err := b.writeVar(t, v); err != nil {
		return b.unsupported(err)
	}
	b.n.ok("variables")
	return nil
}

// assignParam writes a change to a parameter. Only Zeny changes, through
// pay_zeny and give_zeny: Zeny -= x, Zeny += x, and Zeny = Zeny - x.
func (b *body) assignParam(s *Assign) error {
	name := strings.ToLower(s.Target.Name)
	if name == "hp" && s.Target.Index == nil {
		// Hp = n is pc_setparam's SP_HP: player:set_hp (review decision
		// 58, area B7).
		value := Expr(s.Value)
		if s.Op != "=" {
			value = &Binary{Op: strings.TrimSuffix(s.Op, "="), X: &Ident{Name: s.Target.Name}, Y: s.Value}
		}
		v, err := b.expr(value)
		if err != nil {
			return b.unsupported(err)
		}
		b.line("player:set_hp(%s)", v.asInt())
		b.n.ok("Hp")
		return nil
	}
	if name != "zeny" {
		return b.unsupported(unsupported("parameter "+s.Target.Name, "changed"))
	}
	op, amount := s.Op, s.Value
	if op == "=" {
		bin, ok := s.Value.(*Binary)
		var id *Ident
		isZeny := false
		if ok {
			id, isZeny = bin.X.(*Ident)
		}
		if !ok || !isZeny || !strings.EqualFold(id.Name, "Zeny") || bin.Op != "+" && bin.Op != "-" {
			return b.unsupported(unsupported("parameter Zeny", "set to a value"))
		}
		op, amount = bin.Op+"=", bin.Y
	}
	v, err := b.expr(amount)
	if err != nil {
		return b.unsupported(err)
	}
	switch op {
	case "-=":
		b.line("player:pay_zeny(%s)", v.asInt())
	case "+=":
		b.line("player:give_zeny(%s)", v.asInt())
	default:
		return b.unsupported(unsupported("parameter Zeny", "changed with %s", op))
	}
	b.n.ok("Zeny")
	return nil
}

func (b *body) ifStmt(s *If) error {
	c, err := b.expr(s.Cond)
	if err != nil {
		return b.unsupported(err)
	}
	// A condition known now keeps only its branch, so a branch for
	// another mode or a server setting need not convert.
	if c.static != nil {
		if *c.static != 0 {
			return b.stmtInBlock(s.Then)
		}
		if s.Else != nil {
			return b.stmtInBlock(s.Else)
		}
		return nil
	}
	b.line("if %s then", c.asBool())
	if err := b.nested(s.Then); err != nil {
		return err
	}
	for s.Else != nil {
		if next, ok := s.Else.(*If); ok {
			c, err := b.expr(next.Cond)
			if err != nil {
				return b.unsupported(err)
			}
			if c.static != nil {
				b.line("else")
				if *c.static != 0 {
					if err := b.nested(next.Then); err != nil {
						return err
					}
				} else if next.Else != nil {
					if err := b.nested(next.Else); err != nil {
						return err
					}
				}
				break
			}
			b.line("elseif %s then", c.asBool())
			if err := b.nested(next.Then); err != nil {
				return err
			}
			s = next
			continue
		}
		b.line("else")
		if err := b.nested(s.Else); err != nil {
			return err
		}
		break
	}
	b.line("end")
	return nil
}

// stmtInBlock writes a branch that a static condition kept.
func (b *body) stmtInBlock(s Stmt) error {
	b.line("do")
	if err := b.nested(s); err != nil {
		return err
	}
	b.line("end")
	return nil
}

func (b *body) loop(cond string, body, post Stmt, do bool) error {
	br := &breakable{loop: true, cont: b.n.fresh("continue")}
	b.breaks = append(b.breaks, br)
	defer func() { b.breaks = b.breaks[:len(b.breaks)-1] }()
	if do {
		b.line("repeat")
	} else {
		b.line("while %s do", cond)
	}
	if err := b.nested(body); err != nil {
		return err
	}
	if br.usesCont {
		b.indent++
		b.line("::%s::", br.cont)
		b.indent--
	}
	if post != nil {
		if err := b.nested(post); err != nil {
			return err
		}
	}
	if do {
		b.line("until not %s", cond)
	} else {
		b.line("end")
	}
	return nil
}

// switchStmt writes a switch with C's rules: a case runs on into the
// next one until a break.
func (b *body) switchStmt(s *Switch) error {
	v, err := b.expr(s.Value)
	if err != nil {
		return b.unsupported(err)
	}
	name := b.n.fresh("value")
	end := b.n.fresh("switch_end")
	br := &breakable{end: end}
	b.breaks = append(b.breaks, br)
	defer func() { b.breaks = b.breaks[:len(b.breaks)-1] }()
	b.line("do")
	b.indent++
	defer b.enter()()
	b.line("local %s = %s", name, v.code)
	if len(s.Pre) > 0 {
		if err := b.block(s.Pre); err != nil {
			b.indent--
			return err
		}
	}
	labels := make([]string, len(s.Cases))
	var conds []string
	def := ""
	for i, cs := range s.Cases {
		labels[i] = b.n.fresh("case")
		if cs.Default {
			def = labels[i]
		}
		for _, cv := range cs.Values {
			x, err := b.expr(cv)
			if err != nil {
				b.indent--
				return b.unsupported(err)
			}
			conds = append(conds, fmt.Sprintf("%s == %s then goto %s", name, x.code, labels[i]))
		}
	}
	for i, c := range conds {
		kw := "elseif"
		if i == 0 {
			kw = "if"
		}
		b.line("%s %s", kw, c)
	}
	if len(conds) > 0 {
		b.line("end")
	}
	if def != "" {
		b.line("goto %s", def)
	} else {
		b.line("goto %s", end)
	}
	for i, cs := range s.Cases {
		b.line("::%s::", labels[i])
		if err := b.nestedBlockSameIndent(cs.Body); err != nil {
			return err
		}
	}
	b.line("::%s::", end)
	b.indent--
	b.line("end")
	return nil
}

func (b *body) nestedBlockSameIndent(stmts []Stmt) error {
	return b.stmts(stmts, true)
}
