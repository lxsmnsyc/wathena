package convert

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// TestItemCommandsConvert checks the converted text of the item and
// equipment commands of decision 0072, and the reasons of the forms that
// do not convert.
func TestItemCommandsConvert(t *testing.T) {
	res := questResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ mes "You are a " + jobname(Class) + "."; end; }`, []string{"jobs.name(player.job)"}},
		{`{ delitem2 1201,1,1,7,0,4001,0,0,0; delitem2 1201,.@n,1,0,0,0,0,0,0; delitem2 1201,0,1,0,0,0,0,0,0; end; }`, []string{
			"if not player:take_item(KNIFE, 1, { refine = 7, cards = { 4001 } }) then R.stop() end",
			"if not R.take_item(player, KNIFE, n, { refine = 0, cards = {  } }) then R.stop() end",
			"-- delitem2 with an amount of 0 does nothing in rAthena.",
		}},
		{`{ delitem2 7049,1,1,0,0,254,0,.@a,.@b; end; }`, []string{"player:take_item(STONE, 1, R.match(0, 254, 0, a, b))"}},
		{`{ delitem2 7049,1,1,0,0,254,0,5,1; if (countitem2(7049,1,0,0,254,0,5,1)) end; end; }`, []string{
			"player:take_item(STONE, 1, { refine = 0, signed_by_character = 65541 })", "player:count_item(STONE, { refine = 0, signed_by_character = 65541 })",
		}},
		{`{ delitem2 7049,1,1,0,0,254,0,0x15,0x10001; end; }`, []string{"signed_by_character = 65557"}},
		{`{ delitem2 7049,1,1,0,0,254,3,5,1; end; }`, []string{"!delitem2: a named item's mark with a card 2 other than 0"}},
		{`{ delitem2 1201,1,0,0,0,0,0,0,0; end; }`, []string{"!delitem2: unidentified or broken copies"}},
		{`{ delitem2 1201,1,1,0,0,255,0,0,0; end; }`, []string{"!delitem2: a forged, named or pet item's mark in card 1"}},
		{`{ if (isequippedcnt(2941) || countitem(2941)) end; mes "" + isequippedcnt(.@id) + isequippedcnt(2941, 4001); end; }`, []string{
			"player:count_equipped(GUNSLINGER_MANUAL)", "R.count_equipped(player, id)", "R.count_equipped(player, GUNSLINGER_MANUAL, PORING_CARD)",
		}},
		{`{ if (getequiprandomoption(.@loc, .@i, ROA_ID) > 0) end; mes "" + getequiprandomoption(EQI_HAND_R, 0, ROA_PARAM); end; }`, []string{
			`R.equip_option(player, loc, i, "id")`, `R.equip_option(player, 9, 0, "param")`,
		}},
		{`{ mes "" + getequiprandomoption(EQI_HAND_R, 0, .@t); end; }`, []string{"!getequiprandomoption: a computed ROA_ type"}},
		{`{ mes mesitemlink(909) + mesitemlink(909, false) + mesitemlink(581, false, "Mushrooms") + mesitemlink(.@id[1], false, .@n$[1]); end; }`, []string{
			"text.item_link(JELLOPY)", `R.item_link(MUSHROOM, "Mushrooms")`, `R.item_link((id[1] or 0), (n[1] or ""))`,
		}},
		{`{ mes mesitemlink(909, .@b++); end; }`, []string{"R.first(text.item_link(JELLOPY), "}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		for _, want := range c.want {
			if reason, ok := strings.CutPrefix(want, "!"); ok {
				if err == nil || !strings.Contains(err.Error(), reason) {
					t.Errorf("%s: got %v, want %q", c.script, err, reason)
				}
				continue
			}
			if err != nil || !strings.Contains(lua, want) {
				t.Errorf("%s: %v, want %q in\n%s", c.script, err, want, lua)
			}
		}
	}
}

// TestRunItemCommands runs converted jobname, mesitemlink and
// getequiprandomoption. The test host wears nothing.
func TestRunItemCommands(t *testing.T) {
	got := namesRun(t, map[string]string{
		"Items#t": `{
	dispbottom jobname(Job_Swordman) + "|" + jobname(Job_Knight2) + "|" + jobname(9999);
	dispbottom mesitemlink(909) + mesitemlink(909, false, "Jelly <x>");
	dispbottom "" + getequiprandomoption(EQI_HAND_R, 0, ROA_ID) + getequiprandomoption(EQI_HAND_R, 5, ROA_ID);
	end;
}`,
	})
	want := "Swordsman|Knight|Unknown Job|" +
		"<ITEMLINK>Item 909<INFO>909</INFO></ITEMLINK><ITEMLINK>Jelly <x><INFO>909</INFO></ITEMLINK>|" +
		"-1-1"
	if got["Items#t"] != want {
		t.Errorf("got %q, want %q", got["Items#t"], want)
	}
}

// TestRunCommandHelpers runs the rathena.lua helpers of decision 0072 on
// stand-in handles: R.match, R.count_equipped, R.equip_option on a worn
// copy with options, R.party_members_into, R.callshop and
// R.item_link.
func TestRunCommandHelpers(t *testing.T) {
	const src = `
local events = require "wa.events"
local R = require "core.lib.rathena"
return function()
  events.on("click", function(player, conversation)
    local function say(s) player:send_message(s) end
    local m = R.match(3, 254, 0, 5, 1)
    say(m.refine .. " " .. m.signed_by_character .. " " .. tostring(m.cards))
    m = R.match(0, 4001, 0, 4002, 0)
    say(m.refine .. " " .. #m.cards .. " " .. m.cards[1] .. " " .. m.cards[2])
    say(tostring(pcall(R.match, 0, 254, 7, 5, 1)))
    local worn = { count_equipped = function(_, id) return id % 10 end,
      get_equipped = function(_, slot)
        if slot == "right_hand" then return { random_options = { { id = 3, value = 40, param = 1 } } } end
      end }
    say(R.count_equipped(worn, 2941, 0, 2941, -3, 4002) .. "")
    say(R.equip_option(worn, 9, 0, "id") .. " " .. R.equip_option(worn, 9, 0, "value") .. " " .. R.equip_option(worn, 9, 0, "param") ..
      " " .. R.equip_option(worn, 9, 1, "id") .. " " .. R.equip_option(worn, 9, 0, "") .. " " .. R.equip_option(worn, 9, 5, "id") ..
      " " .. R.equip_option(worn, 7, 0, "id"))
    local party = { get_member_list = function() return { { character_id = 5 }, { character_id = 7 } } end }
    local arr = { [0] = 1, [1] = 2, [2] = 3, [3] = 4 }
    local n
    arr, n = R.party_members_into(arr, party, "character_id")
    say(n .. " " .. arr[0] .. arr[1] .. arr[2] .. arr[3])
    arr, n = R.party_members_into(nil, nil, "character_id")
    say(n .. " " .. tostring(arr[0]))
    local opened = {}
    local shopper = { open_shop = function(_, rec) opened[#opened + 1] = rec end }
    local shops = { a = "core:A", b = false }
    R.callshop(shopper, "a", shops)
    R.callshop(shopper, "nope", shops)
    say(table.concat(opened, ",") .. " " .. tostring(pcall(R.callshop, shopper, "b", shops)))
    say(R.item_link(909, "Jelly <x>") .. R.first("x", 1))
  end)
end`
	_, base, core := testRuntime(t, reference.Renewal)
	h := &npcHost{testHost: base, npcs: map[int64]*NPC{1: {Source: "T#t", Map: "prontera"}}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{"scripts/npcs/T_t.lua": src, RathenaLib: rathenaLua}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	logs := &logRecorder{}
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: base.jobs,
		Log: slog.New(logs), Server: variables.NewServer(nil, nil), Clock: func() time.Time { return testClock }, Location: time.UTC},
		[]*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	npc, err := rt.StartNPC(1, "core:scripts/npcs/T_t.lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Click(npc, 1); err != nil {
		t.Fatal(err)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	var got []string
	for _, s := range h.said {
		got = append(got, strings.TrimPrefix(s, "message: "))
	}
	want := []string{"3 65541 nil", "0 2 4001 4002", "false", "3", "3 40 1 0 0 -1 -1", "2 5734", "0 nil",
		"core:A false", "<ITEMLINK>Jelly <x><INFO>909</INFO></ITEMLINK>x"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
