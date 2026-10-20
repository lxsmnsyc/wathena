package convert

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// TestParseRAthenaForms checks forms that rAthena's parser takes:
//   - set on a reference that is not a variable;
//   - any one character after an assignment, as parse_line skips it
//     without checking (script.cpp 1525): a comma in a for header, a
//     colon, and a stray ).
func TestParseRAthenaForms(t *testing.T) {
	for _, src := range []string{
		`{ set getelementofarray(getarg(0), .@i), 1; }`,
		`{ set getd("$x"), 2; }`,
		`{ for (.@i = 0, .@i < 3; .@i++) mes "a"; }`,
		`{ .@type = getarg(0): if (.@type == 3) end; }`,
		`{ .@total = (.@a <= 1) ? .@a : (.@b)); end; }`,
	} {
		b, err := ParseNPC(src)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if strings.HasPrefix(src, "{ set g") {
			if _, ok := b.Stmts[0].(*SetRef); !ok {
				t.Errorf("%s: %T, want a SetRef", src, b.Stmts[0])
			}
		}
	}
	// Item scripts still read set on a reference as an error.
	if _, err := Parse(`set getarg(0), 1;`); err == nil {
		t.Error("an item script read set getarg(0)")
	}
	// The comma ends the for header's first part: the condition follows.
	b, err := ParseNPC(`{ for (.@i = 0, .@i < 3; .@i++) mes "a"; }`)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := b.Stmts[0].(*For); !ok || f.Cond == nil || f.Init == nil || f.Post == nil {
		t.Errorf("for %+v", b.Stmts[0])
	}
}

// builtinFunctions are functions for the builtin tests: one that writes
// an argument's array, and one called by name.
var builtinFunctions = map[string]string{
	"F_TestRef":  `{ set getelementofarray(getarg(0), 1), 5; set getelementofarray(getarg(0), 2), getarg(1); return; }`,
	"F_TestName": `{ return getarg(0) + 1; }`,
	// F_TestRead reads its first argument as a value too, so it is not
	// passed by reference, and its set does not convert.
	"F_TestRead": `{ .@v = getarg(0); set getelementofarray(getarg(0), 1), 5; return; }`,
}

// builtinResult converts NPC lines written for the tests, with
// builtinFunctions and the mode's item groups.
func builtinResult(t *testing.T, scripts map[string]string) *NPCResult {
	t.Helper()
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var lines []npcLine
	for _, name := range sortedKeys(builtinFunctions) {
		lines = append(lines, npcLine{Kind: "function", Name: name, File: "test.txt", Line: 1, Script: builtinFunctions[name]})
	}
	for _, name := range sortedKeys(scripts) {
		lines = append(lines, npcLine{Kind: "script", Name: name, Display: name, File: "test.txt", Line: 1, Map: "prontera",
			X: 150, Y: 150, Sprite: "4_F_KAFRA1", XS: -1, YS: -1, Script: scripts[name]})
	}
	data, err := json.Marshal(map[string]any{"mode": "renewal", "npcs": lines})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(data), Constants: bytes.NewReader(read("script_constants")),
		ClientTables: bytes.NewReader(read("client_tables")), Groups: bytes.NewReader(read("item_groups"))})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestBuiltinsConvert checks the converted text of the commands that
// map onto API that exists, and the reasons of the forms that do not
// convert. A want that starts with "!" is the reason.
func TestBuiltinsConvert(t *testing.T) {
	res := builtinResult(t, nil)
	for _, c := range []struct{ script, want string }{
		// NPC timers.
		{`{ end; OnInit: initnpctimer; end; OnTimer3000: end; OnTimer1000: stopnpctimer; end; }`,
			"NpcTimer = R.npc_timer({ { 1000, event_ontimer1000 }, { 3000, event_ontimer3000 } })"},
		{`{ end; OnInit: initnpctimer; end; }`, "NpcTimer:init()"},
		{`{ initnpctimer "X#t"; stopnpctimer "X#t",1; end; }`, "NpcTimer:stop()"},
		{`{ setnpctimer 0; startnpctimer; end; }`, "NpcTimer:set(0)\n      NpcTimer:start()"},
		{`{ mes "" + getnpctimer(1) + getnpctimer(2); end; OnTimer5: end; }`, `"" .. NpcTimer:due() .. 1`},
		{`{ initnpctimer 1; end; }`, "!initnpctimer: a timer attached to the player"},
		{`{ initnpctimer "Other#t"; end; }`, "!initnpctimer: the timer of another NPC (Other#t)"},
		{`{ startnpctimer; end; }`, "NpcTimer:start()"},
		{`{ mes "" + getnpctimer(0); end; }`, "NpcTimer:tick()"},
		// Equipment reads.
		{`{ if (isequipped(2234,5016)) mes "a"; end; }`, "R.isequipped(player, TIARA, BOY_S_CAP)"},
		{`{ mes "" + isequipped(0,2234); end; }`, `"" .. -1`},
		{`{ if (isequipped(4001)) end; }`, "!isequipped: a card"},
		{`{ if (isequipped(.@i)) end; }`, "!isequipped: a computed item"},
		{`{ if (isequipped(1,2,3,4,5,6)) end; }`, "!isequipped: more than five items"},
		{`{ mes "" + getequipid(EQI_HEAD_TOP); end; }`, `R.equip_field(player, 6, "item", -1)`},
		{`{ mes "" + getequiprefinerycnt(.@s); end; }`, `R.equip_field(player, s, "refine", 0)`},
		{`{ mes getequipname(EQI_ARMOR); end; }`, "R.equip_name(player, 7)"},
		{`{ if (getequipisequiped(EQI_ARMOR)) end; }`, "(R.equipped(player, 7) ~= nil)"},
		{`{ mes "" + getequipcardid(EQI_HAND_R,0); end; }`, "R.equip_card(player, 9, 0)"},
		{`{ mes "" + getequipweaponlv(EQI_HAND_R) + getequiparmorlv(EQI_ARMOR); end; }`, `"weapon_level", 0) .. R.equip_field(player, 7, "armor_level", 0)`},
		{`{ mes "" + getequipid(); end; }`, "!getequipid: without a slot"},
		// Effects on the NPC itself.
		{`{ specialeffect EF_HEAL; specialeffect EF_HEAL,AREA,"X#t"; end; }`, "npc:show_effect(effects."},
		{`{ specialeffect EF_HEAL,SELF; end; }`, `if player ~= nil then npc:show_effect(effects.heal, { to = player }) end`},
		{`{ specialeffect EF_HEAL,AREA,"Other#t"; end; }`, `R.npc_view("Other#t", "show_effect", effects.heal)`},
		{`{ specialeffect EF_HEAL,ALL_CLIENT; end; }`, "!specialeffect: a send target other than AREA, ALL_SAMEMAP and SELF"},
		// Strings and numbers.
		{`{ .@n = atoi(.@s$) + atoi(.@i); end; }`, "R.atoi(s) + R.atoi(tostring(i))"},
		{`{ .@n = compare(.@s$, "a"); end; }`, `R.compare(s, "a")`},
		{`{ .@s$ = strtoupper(.@s$) + strtolower("A") + charat("ab", 1); end; }`, `string.upper(s) .. string.lower("A") .. R.charat("ab", 1)`},
		{`{ .@s$ = replacestr(.@s$, "a", "b", 0, 2); end; }`, `R.replacestr(s, "a", "b", 0, 2)`},
		{`{ .@s$ = sprintf("%d-%5s|%%", .@i, .@s$); end; }`, `string.format("%d-%5s|%%", i, s)`},
		{`{ .@s$ = sprintf("%x", .@i); end; }`, "!sprintf: a conversion other than %d, %i and %s"},
		{`{ .@s$ = sprintf(.@f$, .@i); end; }`, "R.sprintf(f, i)"},
		{`{ .@s$ = delchar(.@s$, 2); end; }`, "R.delchar(s, 2)"},
		{`{ .@s$ = sprintf("%s", .@i); end; }`, "!sprintf: %s of a value that is not a string"},
		{`{ setarray .@a, 1, 2; .@n = inarray(.@a, 2); end; }`, "R.inarray(a, 2, 0)"},
		{`{ copyarray .@b[1], .@a[0], 2; end; }`, "R.copyarray(b, 1, a, 0, 2, 0)"},
		{`{ explode .@p$, .@s$, ","; end; }`, `R.explode(p, 0, s, ",")`},
		{`{ explode .@p, .@s$, ","; end; }`, "!explode: into an array that is not of strings"},
		// Positions and warps.
		{`{ getmapxy(.@m$, .@x, .@y); end; }`, "R.mapxy(player)"},
		{`{ .@r = getmapxy(.@m$, .@x, .@y, BL_NPC); end; }`, "R.mapxy(npc)"},
		{`{ getmapxy(.@m$, .@x, .@y, BL_PC, "Bob"); end; }`, "!getmapxy: of another unit"},
		{`{ warp .@m$, 10, 20; end; }`, "R.warp(player, m, 10, 20)"},
		{`{ warp "Save", 0, 0; end; }`, "player:warp_to_save_point()"},
		// Variables by computed names.
		{`{ setd "$foo_" + .@i, 3; mes "" + getd("$foo_" + .@i); end; }`, `R.setd("server", "$foo_" .. tostring(i), player, 3)`},
		{`{ mes getd("#bar_" + .@i + "$"); end; }`, `R.getd("account", "#bar_" .. tostring(i) .. "$", player)`},
		{`{ setd "$foo", 3; set getd("$foo"), 4; end; }`, "SERVER_FOO:set(4)"},
		{`{ mes "" + getd(".@x" + .@i); end; }`, "!getd: a computed name of a .@ or . variable"},
		{`{ setd "$@x" + .@i, 1; end; }`, `R.setd("server_temporary", "$@x" .. tostring(i), player, 1)`},
		{`{ setd "Zen" + .@i, 1; end; }`, "!setd: a name that may be"},
		// Functions called by name, local functions and callsub.
		{`{ F_TestName 1; mes "" + F_TestName(2); end; }`, `require("core.functions.F_TestName")(player, conversation, npc, 2)`},
		{`{ mes "" + Twice(2); end; function Twice { return getarg(0) * 2; } }`, "func_twice(player, conversation, npc, 2)"},
		{`{ function Cell; Cell 1; end; function Cell { mes "c"; return; } }`, "func_cell(player, conversation, npc, 1)"},
		{`{ mes "a"; close; S_Sub: .@v = getarg(0); return; OnInit: callsub S_Sub, 1; end; }`, "sub_s_sub(player, conversation, npc, 1)"},
		{`{ mes "a"; close; S_Sub: mes "" + getarg(0); return; OnInit: callsub S_Sub, 1; end; }`, "!event oninit: it waits or shows a dialog, and OnInit runs without a player"},
		{`{ switch (select("A:B")) { case 1: callsub S_X, 1; case 2: if (.@a) close; else end; } S_X: mes "" + getarg(0); end; }`, "sub_s_x(player, conversation, npc, 1)"},
		{`{ switch (select("A:B:C")) { case 1: callsub S_X, 1; case 2: close; } S_X: mes "" + getarg(0); end; }`, "!getarg: outside a function"},
		{`{ callsub S_X, 1; S_X: mes "" + getarg(0); return; }`, "!getarg: outside a function"},
		// Labels inside blocks.
		{`{ .@a = 3; if (.@a) { L_In: .@a--; if (.@a) goto L_In; } end; }`, "::l_in::"},
		{`{ goto L_In; if (.@a) { L_In: end; } end; }`, "!labels: l_in: a label inside a block, which a jump from outside the block reaches"},
		// Reference arguments.
		{`{ callfunc "F_TestRef", .@arr, 7; mes "" + .@arr[1]; end; }`, `require("core.functions.F_TestRef")(player, conversation, npc, arr, 7)`},
		{`{ F_TestRef .@other, 1; end; }`, `(player, conversation, npc, other, 1)`},
		{`{ callfunc "F_TestRef", 3; end; }`, "!callfunc: a reference argument that is not an array of the script"},
		{`{ callfunc "F_TestRead", .@x; end; }`, `require("core.functions.F_TestRead")(player, conversation, npc, x)`},
		{`{ setarray .@s, 280, 283; if (getskilllv(.@s[1]) == 0) end; }`, "R.skill_level(player, (s[1] or 0))"},
		{`{ if (getskilllv(.@s$) == 0) end; }`, "!getskilllv: a computed skill that is not a skill ID"},
		// Others.
		{`{ getgroupitem(IG_BlueBox); end; }`, "player:give_item_from_group(BLUEBOX)"},
		{`{ mes mesitemlink(501, true); end; }`, "text.item_link(RED_POTION)"},
		{`{ mes mesitemlink(501, false); end; }`, "!mesitemlink: without brackets"},
		{`{ debugmes "x"; end; }`, "-- debugmes"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if reason, ok := strings.CutPrefix(c.want, "!"); ok {
			if err == nil || !strings.Contains(err.Error(), reason) {
				t.Errorf("%s: got %v, want %q", c.script, err, reason)
			}
			continue
		}
		if err != nil || !strings.Contains(lua, c.want) {
			t.Errorf("%s: %v, want %q in\n%s", c.script, err, c.want, lua)
		}
	}
	if f := res.Functions["f_testread"]; f == nil || !strings.HasPrefix(strings.Join(f.Stubs, "; "), "set: on an element of an argument") {
		t.Errorf("F_TestRead: %+v", f)
	}
	f := res.Functions["f_testref"]
	if f == nil || len(f.Stubs) > 0 || !strings.Contains(f.Lua, "R.arg(args, 0)[1] = 5") || !strings.Contains(f.Lua, "R.arg(args, 0)[2] = R.arg(args, 1)") {
		t.Errorf("F_TestRef: %+v", f)
	}
}

// builtinHost is npcHost with the cards of equipped copies.
type builtinHost struct {
	*eventHost
	cards map[int64][]uint32
}

func (h *builtinHost) EquippedField(id int64, field string) (script.Value, error) {
	if field != "cards" {
		return h.testHost.EquippedField(id, field)
	}
	var list []script.Value
	for _, c := range h.cards[id] {
		list = append(list, script.NewTable(script.KV("item", script.Int(int64(c))), script.KV("rolls", script.NewTable())))
	}
	return script.List(list...), nil
}

// builtinRuntime starts a runtime with the converted files of res.
func builtinRuntime(t *testing.T, res *NPCResult, h modscript.Host, logs *logRecorder) *modscript.Runtime {
	t.Helper()
	_, base, core := testRuntime(t, reference.Renewal)
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	for p, content := range res.Files() {
		if strings.HasPrefix(p, "scripts/") {
			sc.Files[p] = content
		}
	}
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: base.jobs,
		Log: slog.New(logs), Server: variables.NewServer(nil, nil), Location: time.UTC,
		// The server clock runs with the zone clock, as a zone that has
		// players.
		Clock: func() time.Time { return testClock.Add(time.Duration(h.Now()) * time.Millisecond) }},
		[]*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	return rt
}

// TestRunBuiltins runs a converted NPC that reads strings, arrays,
// equipment, positions and variables by computed names, and calls
// functions by name and by reference, and checks each value against
// rAthena's.
func TestRunBuiltins(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	res := builtinResult(t, map[string]string{"Run#t": `{
	dispbottom "" + atoi("  -12abc");
	dispbottom "" + atoi("99999999999");
	dispbottom "" + strtol("0x1F", 16) + " " + strtol("077", 0) + " " + strtol("99999999999", 10);
	dispbottom "" + compare("Hello World", "WORLD") + compare("Hello", "xyz") + compare("a", "");
	dispbottom charat("abc", 1) + "[" + charat("abc", 3) + "]";
	dispbottom strtoupper("aBc") + strtolower("XyZ");
	dispbottom substr("abcdef", 1, 3) + "[" + substr("abc", 2, 5) + "]";
	dispbottom insertchar("abc", "X", -1) + " " + insertchar("abc", "XY", 1) + " " + insertchar("abc", "", 1);
	dispbottom replacestr("aAaA", "a", "b") + " " + replacestr("aAaA", "a", "b", 0, 3);
	dispbottom "" + countstr("abcabcab", "ab") + countstr("ABab", "ab", 0);
	dispbottom "" + pow(2, 10) + " " + pow(2, 40) + " " + distance(1, 1, 4, 3);
	dispbottom sprintf("%03d|%-3s|%%", 7, "x") + sprintf("%%d");
	.@f$ = "%s has %03d%%";
	dispbottom sprintf(.@f$, "Bob", 3) + "|" + delchar("abc", 1) + delchar("abc", 5);
	setarray .@a$, "x", "y", "z";
	dispbottom "" + inarray(.@a$, "z") + inarray(.@a$, "q");
	copyarray .@b$[1], .@a$[0], 3;
	dispbottom .@b$[1] + .@b$[3] + getarraysize(.@b$);
	explode .@c$, "1,2,,3", ",";
	dispbottom .@c$[3] + "/" + getarraysize(.@c$);
	setd "$dyn_" + 7, 42;
	dispbottom "" + getd("$dyn_" + 7) + getd("$dyn_" + 8);
	setd "$dyn_" + "t$", "hi";
	setd "$dyn_" + "arr[2]", 5;
	dispbottom getd("$dyn_" + "t$") + getd("$dyn_" + "arr[2]");
	.@r = getmapxy(.@m$, .@x, .@y);
	dispbottom .@r + " " + .@m$ + " " + .@x + " " + .@y;
	dispbottom "" + isequipped(2234) + isequipped(2234, 2301) + isequipped(5016);
	dispbottom "" + getequipid(EQI_HEAD_TOP) + " " + getequipid(EQI_HEAD_MID) + " " + getequipid(EQI_ARMOR) + " " + getequipid(99);
	dispbottom "" + getequiprefinerycnt(EQI_HEAD_TOP) + getequipisequiped(EQI_HEAD_TOP) + getequipisequiped(EQI_SHOES);
	dispbottom "" + getequipcardid(EQI_HEAD_TOP, 0) + " " + getequipcardid(EQI_HEAD_TOP, 1) + " " + getequipcardid(EQI_HEAD_TOP, 4);
	callfunc "F_TestRef", .@ref, 9;
	dispbottom "" + .@ref[1] + .@ref[2] + getarraysize(.@ref);
	dispbottom "" + F_TestName(41) + Twice(21);
	end;
	function Twice { return getarg(0) * 2; }
}`})
	n := res.NPCs[len(res.NPCs)-1]
	if n.Source != "Run#t" || n.Status != Full {
		t.Fatalf("%s: %s %v", n.Source, n.Status, n.Unsupported)
	}
	_, base, _ := testRuntime(t, reference.Renewal)
	// A Tiara with a Poring Card, worn on the head's top and middle.
	base.slots["head_top"], base.slots["head_middle"] = 100, 100
	base.equipped[100], base.refine[100] = 2234, 7
	h := &builtinHost{eventHost: &eventHost{npcHost: &npcHost{testHost: base, npcs: map[int64]*NPC{1: n}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}}, cards: map[int64][]uint32{100: {4001}}}
	logs := &logRecorder{}
	rt := builtinRuntime(t, res, h, logs)
	npc, err := rt.StartNPC(1, "core:"+n.Script)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Click(npc, 1); err != nil {
		t.Fatal(err)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	want := []string{
		"-12",
		"1215752191", // glibc's atoi is (int) strtol: the low 32 bits.
		"31 63 2147483647",
		"101",
		"b[]",
		"ABCxyz",
		"bcd[]",
		"abcX aXbc a",
		"bAbA bbbA",
		"32",
		"1024 -2147483648 3",
		"007|x  |%%%d",
		"Bob has 003%|acabc",
		"2-1",
		"xz4",
		"3/4",
		"420",
		"hi5",
		"0 prontera 1 1",
		"100",
		"2234 2234 -1 -1",
		"710",
		"4001 0 0",
		"593",
		"4242",
	}
	var got []string
	for _, s := range h.said {
		got = append(got, strings.TrimPrefix(s, "message: "))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestRunNPCTimer runs a converted NPC timer: its labels in order of
// tick, getnpctimer(1) inside a label, and a stop, a set and a start
// from the last label, which runs it again a second later, as rAthena's
// npc_timerevent does.
func TestRunNPCTimer(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	res := builtinResult(t, map[string]string{"Timer#t": `{
	end;
OnInit:
	initnpctimer;
	end;
OnTimer1000:
	announce "one " + getnpctimer(1) + getnpctimer(2), bc_all;
	end;
OnTimer3000:
	announce "three " + getnpctimer(1), bc_all;
	stopnpctimer;
	setnpctimer 2000;
	startnpctimer;
	end;
OnTimer2000:
	announce "two", bc_all;
	end;
}`})
	n := res.NPCs[len(res.NPCs)-1]
	if n.Status != Full {
		t.Fatalf("%s: %s %v", n.Source, n.Status, n.Unsupported)
	}
	_, base, _ := testRuntime(t, reference.Renewal)
	h := &eventHost{npcHost: &npcHost{testHost: base, npcs: map[int64]*NPC{1: n}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}}
	logs := &logRecorder{}
	rt := builtinRuntime(t, res, h, logs)
	if _, err := rt.StartNPC(1, "core:"+n.Script); err != nil {
		t.Fatal(err)
	}
	for _, now := range []int64{999, 1000, 2000, 2999, 3000, 3999, 4000} {
		h.now = now
		rt.Advance(now)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	if got, want := strings.Join(h.said, "|"), "one 13|two|three 0|three 0"; got != want {
		t.Errorf("said %q, want %q", got, want)
	}
}
