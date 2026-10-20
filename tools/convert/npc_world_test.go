package convert

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// worldNPC is an NPC of the tests of review decision 58's commands.
type worldNPC struct {
	m, sprite, state, script string
}

// worldNPCs are written for the tests of review decision 58's commands:
// views, events between NPCs, NPC and temporary server variables, an
// NPC without a map, and the refused commands.
var worldNPCs = map[string]worldNPC{
	"Lever#w": {"prontera", "4_F_KAFRA1", "", `{
	mes "[Lever]";
	disablenpc "Door#w";
	enablenpc "Door#w";
	hideonnpc "Door#w";
	hideoffnpc;
	cloakonnpc "Door#w";
	cloakoffnpcself "Door#w";
	cloakonnpc "Door#w", getcharid(0);
	if (isnpccloaked("Door#w")) mes "cloaked";
	donpcevent "Door#w::OnOpen";
	.@ok = donpcevent("Door#w::OnNone");
	donpcevent "::OnAll";
	donpcevent strnpcinfo(3) + "::OnSelf";
	donpcevent "Door#" + .@x$ + "::OnOpen";
	donpcevent .@ev$;
	doevent "Door#w::OnTalk";
	cmdothernpc "Door#w", "On";
	set getvariableofnpc(.count, "Door#w"), getvariableofnpc(.count, "Door#w") + 1;
	$@round = $@round + 1;
	mes strnpcinfo(0) + "|" + strnpcinfo(1) + "|" + strnpcinfo(2) + "|" + strnpcinfo(3) + "|" + strnpcinfo(4);
	unloadnpc "Old Sign#w";
	close;
OnSelf:
	end;
OnAll:
	end;
}`},
	"Door#w":      {"prontera", "4_F_KAFRA1", "", `{ mes "Count " + .count; close; OnOpen: .count += 1; end; OnTalk: mes "Talk"; close; OnCommandOn: end; OnAll: end; }`},
	"Old Sign#w":  {"prontera", "4_F_KAFRA1", "", `{ mes "Old"; close; }`},
	"Ghost#w":     {"", "-1", "", `{ end; OnAll: $@ghost = 1; end; OnInit: .n = 1; end; }`},
	"Far#w":       {"geffen", "4_F_KAFRA1", "", `{ end; OnOpen: end; }`},
	"Doer#w":      {"prontera", "4_F_KAFRA1", "", `{ doevent "Far#w::OnOpen"; end; }`},
	"Hidden#w":    {"prontera", "-1", "hidden", `{ end; OnShow: hideoffnpc; end; }`},
	"Logout#w":    {"prontera", "4_F_KAFRA1", "", `{ end; OnPCLogoutEvent: end; }`},
	"GM#w":        {"prontera", "4_F_KAFRA1", "", `{ atcommand "@go 0"; end; }`},
	"Flag#w":      {"prontera", "4_F_KAFRA1", "", `{ setbattleflag "base_exp_rate", 200; end; }`},
	"Touch Mob#w": {"prontera", "4_F_KAFRA1", "", `{ end; OnTouchNPC: end; }`},
	"Caller#w": {"prontera", "4_F_KAFRA1", "", `{
	.@a = getcharid(3, "Bob");
	.@n$ = convertpcinfo(.@a, CPC_NAME);
	getpartymember getcharid(1), 1;
	.@c = $@partymembercid[0];
	setd "$@dyn_" + .@a, 2;
	end;
}`},
}

// worldResult converts the worldNPCs.
func worldResult(t *testing.T) *NPCResult {
	t.Helper()
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var lines []npcLine
	for _, name := range sortedKeys(worldNPCs) {
		w := worldNPCs[name]
		lines = append(lines, npcLine{Kind: "script", Name: name, Display: name, File: "test.txt", Line: 1, Map: w.m,
			X: 150, Y: 150, Sprite: w.sprite, State: w.state, XS: -1, YS: -1, Script: w.script})
	}
	data, err := json.Marshal(map[string]any{"mode": "renewal", "npcs": lines})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(data), Constants: bytes.NewReader(read("script_constants")),
		ClientTables: bytes.NewReader(read("client_tables"))})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestNPCWorldConvert checks the converted text of review decision 58's
// commands, the records of an NPC without a map, of a state and of no
// sprite, and the reasons of those that do not convert.
func TestNPCWorldConvert(t *testing.T) {
	res := worldResult(t)
	byName := map[string]*NPC{}
	for _, n := range res.NPCs {
		byName[n.Source] = n
	}
	for name, want := range map[string]string{
		"Doer#w":   "doevent: an NPC on another map than the player's, or without a map",
		"Logout#w": "event onpclogoutevent: rAthena runs it for the players of every map",
		"GM#w":     "atcommand: refused by review decision 58",
		"Flag#w":   "setbattleflag: refused by review decision 58",
	} {
		if got := strings.Join(byName[name].Unsupported, "; "); !strings.Contains(got, want) {
			t.Errorf("%s: unsupported %q, want %q", name, got, want)
		}
	}
	for name, n := range byName {
		if strings.HasPrefix(n.Source, "Doer") || n.Source == "Logout#w" || n.Source == "GM#w" || n.Source == "Flag#w" {
			continue
		}
		if !n.Placed || n.Status == None {
			t.Errorf("%s is not placed: %v", name, n.Unsupported)
		}
	}
	for name, want := range map[string][]string{
		"Lever#w": {
			`R.npc_view("Door#w", "hide")`, `R.npc_view("Door#w", "show")`, `R.npc_view("Door#w", "set_hidden", true)`,
			`R.npc_view(npc, "set_hidden", false)`, `R.npc_view("Door#w", "set_cloaked", true)`,
			`R.npc_cloak_for("Door#w", false, player)`, `R.npc_cloak_for("Door#w", true, player)`,
			`R.is_cloaked("Door#w", player)`, `R.fire("Door#w", "OnOpen")`, `ok = (R.fire("Door#w", "OnNone") and 1 or 0)`,
			`world.fire_npc_event("OnAll")`, `R.fire(npc, "OnSelf")`, `R.fire("Door#" .. x, "OnOpen")`, `R.donpcevent(ev)`,
			`R.doevent("Door#w", "OnTalk", player)`, `R.fire("Door#w", "OnCommand" .. "On")`,
			`NPC_COUNT:set(R.npc_of("Door#w"), (NPC_COUNT:get(R.npc_of("Door#w")) + 1))`, `SERVER_TEMP_ROUND:set((SERVER_TEMP_ROUND:get() + 1))`,
			`npc.full_name .. "|" .. npc.name .. "|" .. npc.hidden_name .. "|" .. npc.unique_name .. "|" .. npc.map`,
			`R.npc_view("Old Sign#w", "remove")`, `local NPC_COUNT = variables.npc_integer "count"`,
			`local SERVER_TEMP_ROUND = variables.server_temporary_integer "round"`,
			`events.on("npc_event", function(event) event_onself(event.player, event.conversation) end, { name = "OnSelf" })`,
		},
		"Door#w": {
			`NPC_COUNT:add(npc, 1)`, `"Count " .. NPC_COUNT:get(npc)`, `{ name = "OnCommandOn" }`, `{ name = "OnTalk" }`,
		},
		"Ghost#w":     {`SERVER_TEMP_GHOST:set(1)`, `{ name = "OnInit" }`, "  event_oninit()\nend"},
		"Touch Mob#w": {`events.on("monster_touch", function() event_ontouchnpc() end)`},
		"Caller#w": {
			`R.player_field(maps.find_player("Bob"), "account_id", 0)`, `R.player_field(maps.find_player_by_id(a), "name", "")`,
			`R.party_members(player.party, "character_id")`, `R.setd("server_temporary", "$@dyn_" .. tostring(a), player, 2)`,
		},
	} {
		lua := byName[name].Lua
		for _, w := range want {
			if !strings.Contains(lua, w) {
				t.Errorf("%s: no %q in\n%s", name, w, lua)
			}
		}
	}
	data := res.Files()["data/npcs.yml"]
	for _, w := range []string{
		`- {name: "Ghost#w", script: scripts/npcs/Ghost_w.lua, display_name: Ghost}`,
		`sprite: none, display_name: Hidden, script: scripts/npcs/Hidden_w.lua, state: hidden}`,
	} {
		if !strings.Contains(data, w) {
			t.Errorf("no %q in\n%s", w, data)
		}
	}
}

// TestRunNPCWorld runs the converted Lever, Door, Ghost and Old Sign in
// one runtime: the Lever's click changes the Door's view, sends it
// events, which the test runs after the click, and changes its NPC
// variable, which the Door's own label adds to.
func TestRunNPCWorld(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	res := worldResult(t)
	_, base, core := testRuntime(t, reference.Renewal)
	h := &npcHost{testHost: base, npcs: map[int64]*NPC{}, steps: map[int64]modscript.Step{}, ended: map[int64]bool{},
		vars: map[modscript.VariableKey]string{}, byName: map[string]int64{}}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	for p, content := range res.Files() {
		if strings.HasPrefix(p, "scripts/") {
			sc.Files[p] = content
		}
	}
	logs := &logRecorder{}
	temp := variables.NewTemporary()
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: base.jobs,
		Log: slog.New(logs), Server: variables.NewServer(nil, nil), Temporary: temp}, []*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	instances := map[int64]*modscript.NPC{}
	ids := map[string]int64{}
	for i, n := range res.NPCs {
		if !n.Placed || n.Script == "" {
			continue
		}
		id := int64(i + 1)
		h.npcs[id], h.byName[n.Name], ids[n.Source] = n, id, id
		npc, err := rt.StartNPC(id, "core:"+n.Script)
		if err != nil {
			t.Fatal(err)
		}
		instances[id] = npc
	}
	conv, err := rt.Click(instances[ids["Lever#w"]], 1)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if conv == nil || conv.Done {
			break
		}
		rt.ResumeConversation(conv, script.Nil)
	}
	// The events run after the click, in the order sent.
	var ran []string
	for _, ev := range h.fired {
		targets := []int64{ev.npc}
		if ev.npc == 0 {
			targets = npcIDs(instances)
		}
		for _, id := range targets {
			c, ok, _ := rt.FireNPCEvent(instances[id], ev.label, ev.player)
			if ok {
				ran = append(ran, h.npcs[id].Source+"::"+ev.label)
			}
			if c != nil && !c.Done {
				rt.CloseConversation(c)
			}
		}
	}
	want := "Door#w::onopen|Door#w::onall|Ghost#w::onall|Lever#w::onall|Lever#w::onself|Door#w::ontalk|Door#w::oncommandon"
	if got := strings.Join(ran, "|"); got != want {
		t.Errorf("events: %s, want %s", got, want)
	}
	door := ids["Door#w"]
	wantControls := []string{
		idOp(door, "hide false 0"), idOp(door, "show false 0"), idOp(door, "set_hidden true 0"), idOp(ids["Lever#w"], "set_hidden false 0"),
		idOp(door, "set_cloaked true 0"), idOp(door, "set_cloaked false 1"), idOp(door, "set_cloaked true 1"), idOp(ids["Old Sign#w"], "remove false 0"),
	}
	if strings.Join(h.controls, "|") != strings.Join(wantControls, "|") {
		t.Errorf("controls %q, want %q", h.controls, wantControls)
	}
	get := func(k variables.TempKey) string {
		v, _ := temp.Get(k)
		return v
	}
	if got := get(variables.TempKey{Mod: mod.Core, NPC: "core:Door#w", Name: "count"}); got != "2" {
		t.Errorf("the Door's count: %q, want 2", got)
	}
	if got := get(variables.TempKey{Mod: mod.Core, Name: "round"}); got != "1" {
		t.Errorf("$@round: %q, want 1", got)
	}
	if got := get(variables.TempKey{Mod: mod.Core, Name: "ghost"}); got != "1" {
		t.Errorf("$@ghost: %q, want 1", got)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
}

func idOp(id int64, op string) string { return strconv.FormatInt(id, 10) + " " + op }
