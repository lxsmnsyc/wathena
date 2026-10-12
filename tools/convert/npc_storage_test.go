package convert

import (
	"strings"
	"testing"
)

// TestStorageCommandsConvert checks the storage, cart and bound item
// commands (proposal 0002, review decision 29). It checks the converted
// text. server/zone's kafra_test.go runs a converted Kafra.
func TestStorageCommandsConvert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ openstorage; end; }`, []string{"player:open_storage()"}},
		{`{ openstorage2 0; end; }`, []string{"player:open_storage()"}},
		{`{ openstorage2 0,STOR_MODE_ALL,getcharid(3); end; }`, []string{"player:open_storage()"}},
		{`{ .@r = openstorage2(0); end; }`, []string{"r = (player:open_storage() and 1 or 0)"}},
		{`{ .@n = storagecountitem(501); end; }`, []string{`n = (player:count_item(RED_POTION, { from = "storage" }) or -1)`}},
		{`{ .@n = storagecountitem("Red_Potion",getcharid(3)); end; }`, []string{`n = (player:count_item(RED_POTION, { from = "storage" }) or -1)`}},
		{`{ .@n = cartcountitem(501); end; }`, []string{`n = (player:count_item(RED_POTION, { from = "cart" }) or -1)`}},
		{`{ .@n = guildstoragecountitem(501); end; }`, []string{"n = (player.guild and player.guild:count_item(RED_POTION) or -1)"}},
		{`{ storagedelitem 501,2; end; }`, []string{`player:take_item(RED_POTION, 2, { from = "storage" })`}},
		{`{ cartdelitem 501,.@n; end; }`, []string{`R.take_item(player, RED_POTION, n, { from = "cart" })`}},
		{`{ cartdelitem 501,0; end; }`, []string{"-- cartdelitem with an amount of 0 does nothing in rAthena."}},
		{`{ getitembound 501,1,Bound_Account; end; }`, []string{`player:give_item(RED_POTION, 1, { bound = "account" })`}},
		{`{ getitembound 501,1,BOUND_GUILD; end; }`, []string{`player:give_item(RED_POTION, 1, { bound = "guild" })`}},
		{`{ getitembound 501,1,3; end; }`, []string{`player:give_item(RED_POTION, 1, { bound = "party" })`}},
		{`{ getitembound 501,.@n,Bound_Char,getcharid(3); end; }`, []string{`R.give_item(player, RED_POTION, n, { bound = "character" })`}},
		{`{ getitembound 501,1,0; end; }`, []string{"player:give_item(RED_POTION, 1)"}},
		{`{ getitembound2 501,1,1,0,0,0,0,0,0,Bound_Account; end; }`, []string{`player:give_item(RED_POTION, 1, { bound = "account" })`}},
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
		{`{ openstorage2 1; end; }`, "openstorage2: storage ID 1: rAthena's IDs come from inter_server.yml"},
		{`{ openstorage2 .@id; end; }`, "openstorage2: a computed storage ID"},
		{`{ openstorage2 0,STOR_MODE_GET; end; }`, "openstorage2: a mode other than STOR_MODE_ALL"},
		{`{ openstorage2 0,STOR_MODE_ALL,2000001; end; }`, "openstorage2: for another account"},
		{`{ .@n = storagecountitem(501,2000001); end; }`, "storagecountitem: for another account"},
		{`{ .@n = storagecountitem2(501,1,0,0,0,0,0,0); end; }`, "storagecountitem2: count_item and take_item match the item only"},
		{`{ .@n = cartcountitem2(501,1,0,0,0,0,0,0); end; }`, "cartcountitem2: count_item and take_item match the item only"},
		{`{ .@n = guildstoragecountitem2(501,1,0,0,0,0,0,0); end; }`, "guildstoragecountitem2: count_item and take_item match the item only"},
		{`{ storagedelitem2 501,1,1,0,0,0,0,0,0; end; }`, "storagedelitem2: count_item and take_item match the item only"},
		{`{ guildstoragedelitem 501,1; end; }`, "guildstoragedelitem: the guild API cannot take items out of the guild storage"},
		{`{ getitembound 501,1,5; end; }`, "getitembound: bound type 5, which rAthena rejects"},
		{`{ getitembound 501,1,.@b; end; }`, "getitembound: a computed bound type"},
		{`{ getitembound2 501,1,1,4,0,0,0,0,0,Bound_Account; end; }`, "getitembound2: an item that is not identified, or has a refine"},
		{`{ .@n = countbound(); end; }`, "countbound: the player API cannot list the bound items"},
		{`{ countbound Bound_Account; end; }`, "countbound: the player API cannot list the bound items"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}
