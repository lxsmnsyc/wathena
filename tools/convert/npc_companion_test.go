package convert

import (
	"strings"
	"testing"
)

// TestCompanionCommandsConvert checks setfalcon, setriding, setcart and
// their checks (proposal 0002, review decision 25).
func TestCompanionCommandsConvert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct{ script, want string }{
		{`{ setfalcon; end; }`, "player:set_companion(FALCON)"},
		{`{ setfalcon 0; end; }`, "player:set_companion(nil)"},
		{`{ setfalcon .@x; end; }`, "player:set_companion(R.truthy(x) and FALCON or nil)"},
		{`{ setriding; end; }`, "player:set_mount(PECO_PECO)"},
		{`{ setriding 0; end; }`, "player:set_mount(nil)"},
		{`{ setcart; end; }`, "player:set_cart(1)"},
		{`{ setcart 0; end; }`, "player:set_cart(nil)"},
		{`{ setcart .@x; end; }`, "player:set_cart(R.nonzero(x))"},
		{`{ if (checkfalcon()) end; end; }`, "player.companion ~= nil"},
		{`{ if (checkriding()) end; end; }`, "player.mount ~= nil"},
		{`{ if (checkcart()) end; end; }`, "player.cart ~= nil"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		if !strings.Contains(lua, c.want) || (strings.Contains(c.want, "FALCON") && !strings.Contains(lua, `local FALCON = companions.id "falcon"`)) ||
			(strings.Contains(c.want, "PECO") && !strings.Contains(lua, `local PECO_PECO = mounts.id "peco_peco"`)) {
			t.Errorf("%s: got\n%s\nwant %q", c.script, lua, c.want)
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ setfalcon 1, 150001; end; }`, "setfalcon: for another character"},
		{`{ if (checkcart(150001)) end; end; }`, "checkcart: for another character"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}
