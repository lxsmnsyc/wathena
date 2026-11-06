package convert

import (
	"strings"
	"testing"
)

// TestComputedCallshopConvert checks callshop of a computed name
// (decision 0072): the shops the script names, and those whose names
// start with the name's fixed text, become a table that R.callshop reads
// when the script runs.
func TestComputedCallshopConvert(t *testing.T) {
	res := shopResult(t)
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: `{
	.@s$ = "Barter#t";
	if (.@x) .@s$ = "Refined Barter#t";
	setarray .@n$[0], "Zeny#t", "Nope";
	callshop .@s$;
	callshop(.@n$[1], 1);
	end;
}`})
	lua, err := n.npcScript()
	if err != nil {
		t.Fatal(err)
	}
	want := `{ ["Barter#t"] = "core:Barter#t", ["Refined Barter#t"] = false, ["Zeny#t"] = "core:Zeny#t" }`
	for _, w := range []string{"R.callshop(player, s, " + want + ")", "R.callshop(player, (n[1] or \"\"), " + want + ")"} {
		if !strings.Contains(lua, w) {
			t.Errorf("want %q in\n%s", w, lua)
		}
	}
	n = res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: `{ callshop "Ma" + .@s$; end; }`})
	if lua, err := n.npcScript(); err != nil || !strings.Contains(lua, `R.callshop(player, "Ma" .. s, { ["Market#t"] = "core:Market#t" })`) {
		t.Errorf("%v\n%s", err, lua)
	}
	n = res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: `{ callshop .@s$; end; }`})
	if _, err := n.npcScript(); err == nil || !strings.Contains(err.Error(), "callshop: a computed shop, and the script names no shop") {
		t.Errorf("got %v", err)
	}
}
