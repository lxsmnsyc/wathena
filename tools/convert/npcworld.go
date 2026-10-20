package convert

import (
	"fmt"
	"regexp"
	"strings"
)

// The NPC-to-NPC commands of review decision 58 (proposal 0002b, areas
// B1 to B4), onto world.get_npc, the NPC handle's view and event
// methods, and NPC and temporary server variables:
//   - An NPC named by a fixed string is found by its record name when
//     the script runs. A name of the NPC itself, from strnpcinfo(0) or
//     (3), is the npc handle, which is the duplicate running the script.
//   - A command on an NPC that does not exist does nothing, as rAthena
//     only logs an error, except getvariableofnpc, which ends the script
//     as rAthena's does.
//   - Every event label of a script (a label that starts with On) is an
//     npc_event handler, as rAthena's ev_db lists them, so donpcevent
//     reaches OnInit and the clock and timer labels too. The labels
//     rAthena runs for the players of every map do not convert.
//   - atcommand and setbattleflag are refused by review decision 58.

// refusedCommands are the commands review decision 58 refuses, with the
// reason.
var refusedCommands = map[string]string{
	"atcommand":     "refused by review decision 58: GM commands are staff tools, not script commands",
	"setbattleflag": "refused by review decision 58: the ruleset is fixed while the server runs",
}

// playerLabels are rAthena's labels of one player's events, which run
// for every NPC with the label, on every map (npc_script_event).
var playerLabels = map[string]bool{
	"onpcloginevent": true, "onpclogoutevent": true, "onpcloadmapevent": true, "onpcbaselvupevent": true,
	"onpcjoblvupevent": true, "onpcdieevent": true, "onpckillevent": true, "onnpckillevent": true,
	"onpcidentifyevent": true, "onpcstatcalcevent": true,
}

// systemLabels are labels that rAthena runs on its own for systems
// wAthena does not have, or not yet as labels: they stay unsupported.
var systemLabels = map[string]bool{
	"oninterifinit": true, "oninterifinitonce": true, "onwhisperglobal": true, "onbuyitem": true, "onsellitem": true,
	"onagitinit": true, "onagitstart": true, "onagitend": true, "onagitinit2": true, "onagitstart2": true,
	"onagitend2": true, "onagitinit3": true, "onagitstart3": true, "onagitend3": true, "onguildbreak": true,
	"oninstanceinit": true, "oninstancedestroy": true, "onagiteliminate": true, "onagitbreak": true,
}

// labelError is the reason a label does not become an npc_event
// handler, or nil.
func labelError(label string) error {
	switch {
	case playerLabels[label]:
		return unsupported("event "+label, "rAthena runs it for the players of every map, and an NPC hears only the players of its own map (decision 0062)")
	case systemLabels[label]:
		return unsupported("event "+label, "")
	}
	return nil
}

// getvariableofnpcRef matches getvariableofnpc(.x, <name>) in a script's
// text, for the NPC variables other NPCs read (sharedVars).
var getvariableofnpcRef = regexp.MustCompile(`(?i)getvariableofnpc\s*\(\s*(\.[A-Za-z_][A-Za-z0-9_]*\$?)\s*(?:\[[^\]]*\])?\s*,\s*([^)]*)\)`)

// fixedName matches a string literal with nothing around it.
var fixedName = regexp.MustCompile(`^\s*"([^"]*)"\s*$`)

// sharedVars reads every script for the NPC variables getvariableofnpc
// reaches: by NPC name and lower-case variable, or for every NPC when the
// name is computed. Those variables become NPC variables of
// wa.variables in the NPC's own script, so every zone reads them.
func (c *npcConverter) sharedVars(lines []npcLine) {
	c.shared, c.sharedAny = map[string]bool{}, map[string]bool{}
	c.dups = map[string][]string{}
	for i := range lines {
		l := &lines[i]
		if l.DuplicateOf != "" {
			c.dups[l.DuplicateOf] = append(c.dups[l.DuplicateOf], l.Name)
		}
		for _, m := range getvariableofnpcRef.FindAllStringSubmatch(l.Script, -1) {
			v := strings.ToLower(m[1])
			if f := fixedName.FindStringSubmatch(m[2]); f != nil {
				c.shared[f[1]+"\x00"+v] = true
			} else {
				c.sharedAny[v] = true
			}
		}
	}
}

// sharedVar reports whether an NPC's .x variable, in lower case, is one
// that getvariableofnpc reaches: of the NPC, of a duplicate that runs
// its script, or of any NPC.
func (c *npcConverter) sharedVar(npc, key string) bool {
	if c.sharedAny[key] || c.shared[npc+"\x00"+key] {
		return true
	}
	for _, d := range c.dups[npc] {
		if c.shared[d+"\x00"+key] {
			return true
		}
	}
	return false
}

// recordFor is the record name of an rAthena NPC name: the converted
// record's, or the name itself for an NPC the converter does not know.
func (c *npcConverter) recordFor(name string) string {
	if r, ok := c.recordOf[name]; ok {
		return r
	}
	return name
}

// npcRef converts an NPC name argument into Lua code that is an NPC
// handle, or a name that R's helpers look up. own is set when it is the
// NPC itself: strnpcinfo(0) or strnpcinfo(3).
func (b *body) npcRef(what string, e Expr) (code string, own bool, err error) {
	if c, ok := e.(*Call); ok && strings.EqualFold(c.Name, "strnpcinfo") && len(c.Args) == 1 {
		// In a function, npc is the calling NPC, as st->oid.
		if k, ok := c.Args[0].(*Num); ok && (k.Value == 0 || k.Value == 3) {
			return "npc", true, nil
		}
	}
	if s, ok := e.(*Str); ok {
		return luaString(b.n.c.recordFor(s.Value)), false, nil
	}
	v, err := b.expr(e)
	if err != nil {
		return "", false, err
	}
	if v.t != tStr && v.t != tAny {
		return "", false, unsupported(what, "an NPC name that is not a string")
	}
	return v.asText(), false, nil
}

// npcView converts enablenpc and its aliases (script.cpp 12339):
// enablenpc, disablenpc, hideonnpc, hideoffnpc, cloakonnpc, cloakoffnpc,
// cloakonnpcself and cloakoffnpcself, with an NPC name or none, and for
// the cloaks a character ID.
func (b *body) npcView(name string, args []Expr) error {
	if len(args) > 2 {
		return unsupported(name, "arguments")
	}
	target := "npc"
	if len(args) >= 1 {
		var err error
		if target, _, err = b.npcRef(name, args[0]); err != nil {
			return err
		}
	}
	call := map[string]string{
		"enablenpc": `"show"`, "disablenpc": `"hide"`, "hideonnpc": `"set_hidden", true`, "hideoffnpc": `"set_hidden", false`,
		"cloakonnpc": `"set_cloaked", true`, "cloakoffnpc": `"set_cloaked", false`,
		"cloakonnpcself": `"set_cloaked", true`, "cloakoffnpcself": `"set_cloaked", false`,
	}[name]
	switch {
	case len(args) == 2:
		if !strings.HasPrefix(name, "cloak") {
			return unsupported(name, "a character ID, which only the cloak commands take")
		}
		// cloakonnpc "<name>", getcharid(0): the attached player.
		if !b.attachedChar(args[1]) {
			return unsupported(name, "the cloak of another character")
		}
		b.line("R.npc_cloak_for(%s, %s, player)", target, strings.TrimPrefix(call, `"set_cloaked", `))
		return nil
	case strings.HasSuffix(name, "self"):
		// Without an attached player, the self forms do nothing
		// (script_rid2sd).
		b.line("R.npc_cloak_for(%s, %s, player)", target, strings.TrimPrefix(call, `"set_cloaked", `))
		return nil
	}
	b.line("R.npc_view(%s, %s)", target, call)
	return nil
}

// attachedChar reports whether an expression is the attached player's
// character ID: getcharid(0).
func (b *body) attachedChar(e Expr) bool {
	c, ok := e.(*Call)
	if !ok || !strings.EqualFold(c.Name, "getcharid") || len(c.Args) != 1 {
		return false
	}
	k, ok := c.Args[0].(*Num)
	return ok && k.Value == 0
}

// eventLabel splits an event name of donpcevent or doevent: "NPC::OnX",
// "::OnX", or strnpcinfo(0) or (3) joined with "::OnX". target is the
// NPC's code, "" for every NPC, and computed is set when the whole name
// is computed: its code is in target.
func (b *body) eventLabel(what string, e Expr) (target, label string, computed bool, err error) {
	switch e := e.(type) {
	case *Str:
		i := strings.LastIndex(e.Value, "::")
		if i < 0 {
			return "", "", false, unsupported(what, "%q is not an event label NPC::OnX", e.Value)
		}
		name, l := e.Value[:i], e.Value[i+2:]
		if !labelName.MatchString(l) {
			return "", "", false, unsupported(what, "%q is not an event label", l)
		}
		if name == "" {
			return "", l, false, nil
		}
		return luaString(b.n.c.recordFor(name)), l, false, nil
	case *Binary:
		if s, ok := e.Y.(*Str); ok && e.Op == "+" && strings.HasPrefix(s.Value, "::") && labelName.MatchString(s.Value[2:]) {
			code, _, err := b.npcRef(what, e.X)
			if err != nil {
				return "", "", false, err
			}
			return code, s.Value[2:], false, nil
		}
	}
	v, err := b.expr(e)
	if err != nil {
		return "", "", false, err
	}
	return v.asText(), "", true, nil
}

// labelName is rAthena's event label after the "::".
var labelName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,49}$`)

// donpcevent converts donpcevent "<NPC>::<label>" (script.cpp 11615):
// the label runs after the current step, with no player, on the NPC or
// on every NPC with the label for "::<label>". It returns 1 when an NPC
// has the label, as npc_event_do's count.
func (b *body) donpcevent(args []Expr) (val, error) {
	const what = "donpcevent"
	if len(args) != 1 {
		return val{}, unsupported(what, "arguments")
	}
	if s, ok := args[0].(*Str); ok && !strings.Contains(s.Value, "::") {
		// check_event only warns, and npc_event_do finds no event.
		return val{code: "false", t: tBool}, nil
	}
	target, label, computed, err := b.eventLabel(what, args[0])
	if err != nil {
		return val{}, err
	}
	switch {
	case computed:
		return val{code: fmt.Sprintf("R.donpcevent(%s)", target), t: tBool}, nil
	case target == "":
		b.n.file.need("world")
		return val{code: fmt.Sprintf("(world.fire_npc_event(%s) > 0)", luaString(label)), t: tBool}, nil
	}
	return val{code: fmt.Sprintf("R.fire(%s, %s)", target, luaString(label)), t: tBool}, nil
}

// doevent converts doevent "<NPC>::<label>" (script.cpp 11598): the
// label runs for the attached player, after the current step and after
// the player's conversation. The player must be on the NPC's map, so
// the NPC must be on the map of the NPC whose script runs.
func (b *body) doevent(args []Expr) error {
	const what = "doevent"
	if len(args) != 1 {
		return unsupported(what, "arguments")
	}
	s, ok := args[0].(*Str)
	if !ok {
		target, label, computed, err := b.eventLabel(what, args[0])
		if err != nil {
			return err
		}
		if computed || target != "npc" || b.n.isFunction {
			return unsupported(what, "a computed event label")
		}
		b.line("R.doevent(npc, %s, player)", luaString(label))
		return nil
	}
	i := strings.LastIndex(s.Value, "::")
	if i <= 0 {
		return unsupported(what, "%q is not an event label NPC::OnX", s.Value)
	}
	name, label := s.Value[:i], s.Value[i+2:]
	n := b.n
	if n.isFunction {
		return unsupported(what, "in a function, which may run on a map other than the NPC's")
	}
	m, known := n.c.npcMap[name]
	switch {
	case !known:
		return unsupported(what, "the NPC %s is not in the extract", name)
	case m == "" || m != n.line.Map:
		return unsupported(what, "an NPC on another map than the player's, or without a map")
	}
	b.line("R.doevent(%s, %s, player)", luaString(n.c.recordFor(name)), luaString(label))
	return nil
}

// cmdothernpc converts cmdothernpc "<NPC>","<command>" (script.cpp
// 11629): donpcevent of the NPC's OnCommand<command> label.
func (b *body) cmdothernpc(args []Expr) (val, error) {
	const what = "cmdothernpc"
	if len(args) != 2 {
		return val{}, unsupported(what, "arguments")
	}
	target, _, err := b.npcRef(what, args[0])
	if err != nil {
		return val{}, err
	}
	cmd, err := b.expr(args[1])
	if err != nil {
		return val{}, err
	}
	return val{code: fmt.Sprintf("R.fire(%s, \"OnCommand\" .. %s)", target, cmd.asText()), t: tBool}, nil
}

// isnpccloaked converts isnpccloaked{("<NPC>"{, <character ID>})}
// (script.cpp 26738).
func (b *body) isnpccloaked(args []Expr) (val, error) {
	const what = "isnpccloaked"
	target := "npc"
	if len(args) >= 1 {
		var err error
		if target, _, err = b.npcRef(what, args[0]); err != nil {
			return val{}, err
		}
	}
	switch {
	case len(args) > 2:
		return val{}, unsupported(what, "arguments")
	case len(args) == 2 && !b.attachedChar(args[1]):
		return val{}, unsupported(what, "of another character")
	}
	return val{code: fmt.Sprintf("R.is_cloaked(%s, player)", target), t: tBool}, nil
}

// unloadnpc converts unloadnpc "<NPC>" (script.cpp 25658). rAthena
// refuses to unload the NPC that runs the script, and unloads the
// duplicates of the NPC too.
func (b *body) unloadnpc(args []Expr) error {
	const what = "unloadnpc"
	if len(args) != 1 {
		return unsupported(what, "arguments")
	}
	s, ok := args[0].(*Str)
	switch {
	case !ok:
		return unsupported(what, "a computed NPC name")
	case s.Value == b.n.line.Name && !b.n.isFunction:
		// buildin_unloadnpc: "You cannot self-unload NPC".
		b.line("-- unloadnpc of the NPC itself does nothing, as in rAthena.")
		return nil
	case len(b.n.c.dups[s.Value]) > 0:
		return unsupported(what, "an NPC with duplicates, which rAthena unloads with it")
	}
	b.line("R.npc_view(%s, \"remove\")", luaString(b.n.c.recordFor(s.Value)))
	return nil
}

// npcVariableRef reads getvariableofnpc(<.x>, "<NPC>"): the variable and
// the NPC's code.
func (b *body) npcVariableRef(args []Expr) (*npcVar, *Var, string, error) {
	const what = "getvariableofnpc"
	if len(args) != 2 {
		return nil, nil, "", unsupported(what, "arguments")
	}
	v, ok := args[0].(*Var)
	if !ok {
		return nil, nil, "", unsupported(what, "not of a variable")
	}
	if p, _, _ := varName(v.Name); p != "." {
		// buildin_getvariableofnpc: "invalid scope (not npc variable)".
		return nil, nil, "", unsupported(what, "%s is not an NPC variable", v.Name)
	}
	target, own, err := b.npcRef(what, args[1])
	if err != nil {
		return nil, nil, "", err
	}
	if !own {
		target = "R.npc_of(" + target + ")"
	}
	nv, err := b.sharedVariable(v.Name)
	if err != nil {
		return nil, nil, "", err
	}
	return nv, v, target, nil
}

// sharedVariable is the NPC variable of wa.variables that holds an
// NPC's .x, for getvariableofnpc.
func (b *body) sharedVariable(name string) (*npcVar, error) {
	n := b.n
	key := "npcvar " + strings.ToLower(name)
	if v, ok := n.vars[strings.ToLower(name)]; ok && v.scope == "npc_shared" {
		return v, nil
	}
	if v, ok := n.vars[key]; ok {
		return v, nil
	}
	_, bare, str := varName(name)
	if !variableNameOK(bare) {
		return nil, unsupported("variables", "%s is not a valid name", name)
	}
	declared := strings.ToLower(bare)
	if str {
		declared += "_text"
	}
	v := &npcVar{scope: "npc_shared", name: declared, str: str, array: n.arrays[strings.ToLower(name)], local: "NPC_" + constName(bare)}
	if str {
		v.local += "_TEXT"
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

// getvariableofnpc reads another NPC's variable (script.cpp 20483).
func (b *body) getvariableofnpc(args []Expr) (val, error) {
	nv, v, target, err := b.npcVariableRef(args)
	if err != nil {
		return val{}, err
	}
	b.n.file.need("variables")
	t, def := tInt, "0"
	if nv.str {
		t, def = tStr, `""`
	}
	if nv.array || v.Index != nil {
		idx := "0"
		if v.Index != nil {
			iv, err := b.expr(v.Index)
			if err != nil {
				return val{}, err
			}
			idx = iv.asInt()
		}
		if !nv.array {
			return val{}, unsupported("getvariableofnpc", "an element of a variable that is not an array")
		}
		return val{code: fmt.Sprintf("R.aget(%s, %s, %s, %s)", nv.local, target, idx, def), t: t}, nil
	}
	return val{code: fmt.Sprintf("%s:get(%s)", nv.local, target), t: t}, nil
}

// setVariableOfNPC writes set getvariableofnpc(<.x>, "<NPC>"), <value>.
func (b *body) setVariableOfNPC(s *SetRef) error {
	nv, v, target, err := b.npcVariableRef(s.Ref.Args)
	if err != nil {
		return err
	}
	value, err := b.expr(s.Value)
	if err != nil {
		return err
	}
	code := value.asInt()
	if nv.str {
		code = value.asText()
		if value.t == tAny {
			code = "R.text(" + value.code + ")"
		}
	}
	b.n.file.need("variables")
	if nv.array || v.Index != nil {
		if !nv.array {
			return unsupported("getvariableofnpc", "an element of a variable that is not an array")
		}
		idx := "0"
		if v.Index != nil {
			iv, err := b.expr(v.Index)
			if err != nil {
				return err
			}
			idx = iv.asInt()
		}
		b.line("R.aset(%s, %s, %s, %s)", nv.local, target, idx, code)
		return nil
	}
	b.line("%s:set(%s, %s)", nv.local, target, code)
	return nil
}

// strnpcinfo reads strnpcinfo(<type>) of the NPC itself (script.cpp
// 9248): 0 the full name with its # part, 1 the name players see, 2 the
// part after #, 3 the unique name and 4 the map, "" for an NPC without
// one.
func (b *body) strnpcinfo(args []Expr) (val, error) {
	if len(args) == 1 {
		if v, err := b.expr(args[0]); err == nil && v.static != nil {
			if field, ok := map[int64]string{0: "full_name", 1: "name", 2: "hidden_name", 3: "unique_name", 4: "map"}[*v.static]; ok {
				return val{code: "npc." + field, t: tStr}, nil
			}
			return val{}, unsupported("strnpcinfo", "type %d, such as the NPC's file", *v.static)
		}
		return val{}, unsupported("strnpcinfo", "a computed type")
	}
	return val{}, unsupported("strnpcinfo", "of another NPC by its game ID")
}

// otherCharID converts getcharid(<type>, "<name>") of another
// character (script.cpp 8884): 0 the character ID and 3 the account ID
// of an online player of the map, or 0, as rAthena gives for a player
// who is not online.
func (b *body) otherCharID(args []Expr) (val, error) {
	t, err := b.expr(args[0])
	if err != nil {
		return val{}, err
	}
	field := ""
	if t.static != nil {
		field = map[int64]string{0: "character_id", 3: "account_id"}[*t.static]
	}
	if field == "" {
		return val{}, unsupported("getcharid", "of another character, other than its character and account IDs")
	}
	name, err := b.expr(args[1])
	if err != nil {
		return val{}, err
	}
	b.n.file.need("maps")
	return val{code: fmt.Sprintf("R.player_field(maps.find_player(%s), %s, 0)", name.asText(), luaString(field)), t: tInt}, nil
}

// convertpcinfo converts convertpcinfo(<name or ID>, <type>) (script.cpp
// 26690): the name (CPC_NAME), character ID (CPC_CHAR) or account ID
// (CPC_ACCOUNT) of an online player of the map, or "" and 0 for none.
func (b *body) convertpcinfo(args []Expr) (val, error) {
	const what = "convertpcinfo"
	if len(args) != 2 {
		return val{}, unsupported(what, "arguments")
	}
	who, err := b.expr(args[0])
	if err != nil {
		return val{}, err
	}
	t, err := b.expr(args[1])
	if err != nil {
		return val{}, err
	}
	if t.static == nil || *t.static < 0 || *t.static > 2 {
		return val{}, unsupported(what, "a computed or unknown type")
	}
	b.n.file.need("maps")
	find := "maps.find_player_by_id(" + who.asInt() + ")"
	switch who.t {
	case tStr:
		find = "maps.find_player(" + who.code + ")"
	case tAny:
		return val{}, unsupported(what, "a player that may be a name or an ID")
	}
	if *t.static == 0 {
		return val{code: fmt.Sprintf("R.player_field(%s, \"name\", \"\")", find), t: tStr}, nil
	}
	field := map[int64]string{1: "character_id", 2: "account_id"}[*t.static]
	return val{code: fmt.Sprintf("R.player_field(%s, %s, 0)", find, luaString(field)), t: tInt}, nil
}
