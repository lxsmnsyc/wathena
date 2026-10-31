package convert

import (
	"strings"
	"testing"
)

// TestDecision60Convert checks the commands that review decision 60's
// calls convert (npc-api-events).
func TestDecision60Convert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ end; OnMyMobDead: .@id = killedrid; .@gid = killedgid; end; }`, []string{
			"event_onmymobdead = function(player, conversation, event)",
			"id = (event and event.monster_id or 0)", "gid = (event and event.monster_game_id or 0)",
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
				t.Errorf("%s: got\n%s\nwant %q", c.script, lua, w)
			}
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ .@id = killedrid; end; }`, "killedrid: outside the code of a monster's label"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}
