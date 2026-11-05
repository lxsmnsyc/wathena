package convert

import (
	"strings"
	"testing"
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
		{`{ delitem2 1201,1,1,7,0,4001,0,0,0; delitem2 1201,.@n,1,0,0,0,0,0,0; end; }`, []string{
			"player:take_item(KNIFE, 1, { refine = 7, cards = { 4001 } })", "R.take_item(player, KNIFE, n, { refine = 0, cards = {  } })",
		}},
		{`{ delitem2 7049,1,1,0,0,254,0,.@a,.@b; end; }`, []string{"player:take_item(STONE, 1, R.match(0, 254, 0, a, b))"}},
		{`{ delitem2 7049,1,1,0,0,254,0,5,1; if (countitem2(7049,1,0,0,254,0,5,1)) end; end; }`, []string{
			"player:take_item(STONE, 1, { refine = 0, signed_by_character = 65541 })", "player:count_item(STONE, { refine = 0, signed_by_character = 65541 })",
		}},
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
		"<ITEMLINK>Item 909<INFO>909</INFO></ITEMLINK><ITEMLINK>Jelly x<INFO>909</INFO></ITEMLINK>|" +
		"-1-1"
	if got["Items#t"] != want {
		t.Errorf("got %q, want %q", got["Items#t"], want)
	}
}
