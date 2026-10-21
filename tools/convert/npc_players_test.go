package convert

import (
	"strings"
	"testing"
)

// TestPlayersConvert checks the converted text of review decision 59's
// player commands: the labels of one player's events, which become
// handlers of the player events of every map, attachrid and detachrid,
// and the commands that act on another character, which act on its
// handle on any map.
func TestPlayersConvert(t *testing.T) {
	res := worldResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ end; OnPCLoginEvent: getitem 501,1; end; OnPCLogoutEvent: end; OnPCBaseLvUpEvent: end; OnPCJobLvUpEvent: end; }`, []string{
			`events.on("login", function(player) event_onpcloginevent(player, nil) end)`,
			`events.on("logout", function(player) event_onpclogoutevent(player, nil) end)`,
			`events.on("base_level_up", function(player) event_onpcbaselvupevent(player, nil) end)`,
			`events.on("job_level_up", function(player) event_onpcjoblvupevent(player, nil) end)`,
			`{ name = "OnPCLoginEvent" }`,
		}},
		{`{ end; OnPCDieEvent: end; OnPCLoadMapEvent: end; }`, []string{
			`events.on("death", function(death) event_onpcdieevent(death.victim, nil) end, { kind = "player" })`,
			`events.on("map_enter", function(player) if maps.get_flag(player.map, "loadevent") then event_onpcloadmapevent(player, nil) end end)`,
		}},
		{`{ attachrid(getcharid(3,$@groom$)); getitem 2634,1; detachrid; end; }`, []string{
			`player = R.attachrid(player, R.player_field(maps.find_player(`, "player:give_item(", "player = nil",
		}},
		{`{ getitem 501,2,.@aid; delitem 501,1,getcharid(3); percentheal 50,50,.@cid; warpchar "prontera",150,150,.@cid; end; }`, []string{
			"local player = R.by_account(", "if player ~= nil then", "player:give_item(", "player:take_item(",
			"local player = R.by_char(", "player:heal_percent { hp = 50, sp = 50 }", `player:warp("prontera", 150, 150)`,
		}},
		{`{ sc_start SC_BLESSING,1000,10,10000,0,.@gid; specialeffect2 EF_HEAL,AREA,"Bob"; end; }`, []string{
			"local player = R.by_gid(", "player:start_status(", `local player = R.by_name("Bob")`,
		}},
		{`{ message "Bob","Hi"; dispbottom "Yo",0xFF0000,150001; setquest 1100,150001; end; }`, []string{
			`local player = R.by_name("Bob")`, `player:send_message("Hi")`, "local player = R.by_char(150001)",
			`player:send_message("Yo", { color = 16711680 })`, "-- setquest 1100: this mode's quest_db has no such quest",
		}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(lua, w) {
				t.Errorf("%s: no %q in\n%s", c.script, w, lua)
			}
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ if (attachrid(.@a)) end; end; }`, "attachrid"},
		{`{ attachrid .@a,0; end; }`, "attachrid: without force"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		if _, err := n.npcScript(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}
