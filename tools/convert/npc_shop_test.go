package convert

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// shopLines are the shop lines of the shop tests, one of each kind.
const shopLines = `[
{"kind": "shop", "name": "Zeny#t", "display": "Zeny#t", "file": "test.txt", "line": 1, "map": "prontera", "x": 150, "y": 150, "sprite": "4_F_KAFRA1",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 50}], "discount": true}},
{"kind": "itemshop", "name": "Item#t", "display": "Item#t", "file": "test.txt", "line": 2, "map": "prontera", "x": 151, "y": 150, "sprite": "4_F_KAFRA1",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 3}, {"item": "Red_Potion", "id": 501, "price": 4}, {"item": "Apple", "id": 512, "price": 0}],
  "discount": false, "currency": {"item": "Poring_Coin"}}},
{"kind": "pointshop", "name": "Point#t", "display": "Point#t", "file": "test.txt", "line": 3, "sprite": "-1",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 10}], "discount": true, "currency": {"variable": "#EVENTPOINTS"}}},
{"kind": "pointshop", "name": "Quest#t", "display": "Quest#t", "file": "test.txt", "line": 4, "sprite": "-1",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 2}], "discount": false, "currency": {"variable": "Quest_Points"}}},
{"kind": "pointshop", "name": "Cashpoint#t", "display": "Cashpoint#t", "file": "test.txt", "line": 5, "sprite": "-1",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 7}], "discount": false, "currency": {"variable": "#CASHPOINTS"}}},
{"kind": "pointshop", "name": "Temp#t", "display": "Temp#t", "file": "test.txt", "line": 6, "sprite": "-1",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 7}], "discount": false, "currency": {"variable": "@temp"}}},
{"kind": "cashshop", "name": "Cash#t", "display": "Cash#t", "file": "test.txt", "line": 7, "sprite": "-1",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 100}], "discount": false}},
{"kind": "marketshop", "name": "Market#t", "display": "Market#t", "file": "test.txt", "line": 8, "sprite": "FAKE_NPC",
 "shop": {"items": [{"item": "Red_Potion", "id": 501, "price": 40, "stock": 5}, {"item": "Apple", "id": 512, "price": 15, "stock": -1},
  {"item": "Jellopy", "id": 909, "price": 6, "stock": 0}], "discount": false}}
]`

// shopBarters are the barter shops of the shop tests.
const shopBarters = `{"barters": [
{"name": "Barter#t", "file": "barters.yml", "map": null, "x": 0, "y": 0, "dir": 0, "sprite": null, "items": [
 {"index": 0, "item": "Oridecon", "id": 984, "stock": 0, "zeny": 0, "refine": 0, "required": [{"index": 0, "item": "Oridecon_Stone", "id": 756, "amount": 5, "refine": -1}]},
 {"index": 1, "item": "Elunium", "id": 985, "stock": 10, "zeny": 500, "refine": 0, "required": [{"index": 0, "item": "Jellopy", "id": 909, "amount": 2, "refine": -1}, {"index": 1, "item": "Apple", "id": 512, "amount": 1, "refine": 0}]},
 {"index": 2, "item": "Elunium", "id": 985, "stock": 0, "zeny": 900, "refine": 0, "required": []},
 {"index": 3, "item": "Sword", "id": 1101, "stock": 0, "zeny": 0, "refine": 7, "required": [{"index": 0, "item": "Jellopy", "id": 909, "amount": 1, "refine": -1}]},
 {"index": 4, "item": "Knife", "id": 1201, "stock": 0, "zeny": 0, "refine": 0, "required": [{"index": 0, "item": "Sword", "id": 1101, "amount": 1, "refine": 9}]},
 {"index": 5, "item": "Apple", "id": 512, "stock": 0, "zeny": 0, "refine": 0, "required": [
  {"index": 0, "item": "Jellopy", "id": 909, "amount": 1, "refine": -1}, {"index": 1, "item": "Jellopy", "id": 909, "amount": 1, "refine": -1},
  {"index": 2, "item": "Jellopy", "id": 909, "amount": 1, "refine": -1}, {"index": 3, "item": "Jellopy", "id": 909, "amount": 1, "refine": -1},
  {"index": 4, "item": "Jellopy", "id": 909, "amount": 1, "refine": -1}, {"index": 5, "item": "Jellopy", "id": 909, "amount": 1, "refine": -1}]}]},
{"name": "Placed Barter#t", "file": "barters.yml", "map": "prontera", "x": 155, "y": 160, "dir": 4, "sprite": "4_F_KAFRA1", "items": [
 {"index": 0, "item": "Apple", "id": 512, "stock": 0, "zeny": 30, "refine": 0, "required": []}]},
{"name": "Refined Barter#t", "file": "barters.yml", "map": null, "x": 0, "y": 0, "dir": 0, "sprite": null, "items": [
 {"index": 0, "item": "Sword", "id": 1101, "stock": 0, "zeny": 0, "refine": 4, "required": []}]}
]}`

// shopResult converts the shop lines and barters of the shop tests.
func shopResult(t *testing.T) *NPCResult {
	t.Helper()
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var lines []json.RawMessage
	if err := json.Unmarshal([]byte(shopLines), &lines); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"mode": "renewal", "npcs": lines})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(data), Constants: bytes.NewReader(read("script_constants")),
		ClientTables: bytes.NewReader(read("client_tables")), Shops: strings.NewReader(shopBarters)})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestShopKindsConvert checks the record of each shop kind and of barter
// shops, and the currencies of pointshop variables.
func TestShopKindsConvert(t *testing.T) {
	res := shopResult(t)
	by := map[string]*NPC{}
	for _, n := range res.NPCs {
		by[n.Source] = n
	}
	for _, c := range []struct {
		name        string
		status      Status
		placed      bool
		record      string
		unsupported string
	}{
		{"Zeny#t", Full, true, `- {name: "Zeny#t", map: prontera, x: 150, y: 150, direction: north, sprite: 4_F_KAFRA1, display_name: Zeny, shop: [{item: Red_Potion, price: 50}]}`, ""},
		{"Item#t", Partial, true, `shop: {buys_from_players: false, discount: false, items: [{item: Red_Potion, cost: {items: [{item: Poring_Coin, amount: 3}]}}, {item: Apple, cost: {zeny: 0}}]}}`,
			"itemshop: Red_Potion is in the shop twice, and a shop table names each item once"},
		{"Point#t", Full, false, `- {name: "Point#t", shop: {buys_from_players: false, discount: true, items: [{item: Red_Potion, cost: {currency: {eventpoints: 10}}}]}}`, ""},
		{"Quest#t", Full, false, `items: [{item: Red_Potion, cost: {currency: {quest_points: 2}}}]`, ""},
		{"Cashpoint#t", Full, false, `items: [{item: Red_Potion, cost: {currency: {cash: 7}}}]`, ""},
		{"Temp#t", None, false, "", "pointshop: @temp is a temporary variable, which a currency cannot be"},
		{"Cash#t", Full, false, `- {name: "Cash#t", shop: {buys_from_players: false, discount: false, items: [{item: Red_Potion, cost: {currency: {cash: 100}}}]}}`, ""},
		{"Market#t", Partial, false, `- {name: "Market#t", shop: {buys_from_players: false, discount: false, items: [{item: Red_Potion, price: 40, stock: 5}, {item: Apple, price: 15}]}}`,
			"marketshop: Jellopy starts with no stock, and a stock in data/npcs.yml is 1 or more"},
		{"Barter#t", Partial, false, `- {name: "Barter#t", shop: {buys_from_players: false, discount: false, items: [{item: Oridecon, cost: {items: [{item: Oridecon_Stone, amount: 5}]}}, {item: Elunium, cost: {zeny: 500, items: [{item: Jellopy, amount: 2}, {item: Apple, amount: 1}]}, stock: 10}]}}`,
			"barter: Elunium (index 2) is sold a second time, and a shop table names each item once; " +
				"barter: Sword (index 3) is sold refined to +7, and a shop item has no refine; " +
				"barter: Knife (index 4) takes Sword refined to +9 or more, and a cost cannot ask for a refine; " +
				"barter: Apple (index 5) takes 6 kinds of items, and a cost takes at most 5"},
		{"Placed Barter#t", Full, true, `- {name: "Placed Barter#t", map: prontera, x: 155, y: 160, direction: south, sprite: 4_F_KAFRA1, display_name: Placed Barter, shop: {buys_from_players: false, discount: false, items: [{item: Apple, cost: {zeny: 30}}]}}`, ""},
		{"Refined Barter#t", None, false, "", "barter: Sword (index 0) is sold refined to +4, and a shop item has no refine; barter: 0 items convert, and a shop has 1 to 200"},
	} {
		n := by[c.name]
		if n == nil {
			t.Errorf("%s: missing", c.name)
			continue
		}
		if n.Status != c.status || n.Placed != c.placed {
			t.Errorf("%s: status %s, placed %v; want %s, %v", c.name, n.Status, n.Placed, c.status, c.placed)
		}
		if c.record != "" && !strings.Contains(n.record, c.record) {
			t.Errorf("%s: record\n%s\nwant it to have\n%s", c.name, n.record, c.record)
		}
		if got := strings.Join(n.Unsupported, "; "); got != c.unsupported {
			t.Errorf("%s: unsupported %q, want %q", c.name, got, c.unsupported)
		}
	}
	files := res.Files()
	want := "- name: eventpoints\n  id: 3\n  display_name: EVENTPOINTS\n  owner: account\n  max: 2147483647\n" +
		"- name: quest_points\n  id: 4\n  display_name: Quest_Points\n  owner: character\n  max: 2147483647\n"
	if cur := files["data/currencies.yml"]; !strings.Contains(cur, "- name: cash\n") || !strings.HasSuffix(cur, want) {
		t.Errorf("data/currencies.yml:\n%s\nwant it to end with\n%s", cur, want)
	}
	npcs := files["data/npcs.yml"]
	for _, name := range []string{"Zeny#t", "Barter#t", "Placed Barter#t", "Market#t"} {
		if !strings.Contains(npcs, by[name].record+"\n") {
			t.Errorf("data/npcs.yml lacks %s", name)
		}
	}
	if strings.Contains(npcs, "Refined Barter") || strings.Contains(npcs, "Temp#t") {
		t.Errorf("data/npcs.yml has a shop that did not convert:\n%s", npcs)
	}
}

// TestShopCommandsConvert checks the shop, mail, store and currency
// commands of review decision 30. It checks the converted text.
func TestShopCommandsConvert(t *testing.T) {
	res := shopResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ callshop "Barter#t"; end; }`, []string{`player:open_shop("core:Barter#t")`}},
		{`{ callshop "Market#t",1; end; }`, []string{`player:open_shop("core:Market#t")`}},
		{`{ npcshopupdate "Market#t",501,0,9999; end; }`, []string{`world.get_shop("Market#t"):set_stock(RED_POTION, 9999)`}},
		{`{ npcshopupdate "Market#t",501,0; end; }`, []string{`world.get_shop("Market#t"):set_stock(RED_POTION, -1)`}},
		{`{ npcshopupdate "Market#t",501,0,.@n; end; }`, []string{`world.get_shop("Market#t"):set_stock(RED_POTION, n)`}},
		{`{ npcshopupdate "Zeny#t",501,0,5; end; }`, []string{"-- npcshopupdate with a price of 0 changes nothing in a shop."}},
		{`{ npcshopupdate "Zeny#t",501,70; end; }`, []string{`world.get_shop("Zeny#t"):add_item(RED_POTION, { cost = { zeny = 70 } })`}},
		{`{ npcshopupdate "Zeny#t","Red_Potion",-1; end; }`, []string{`world.get_shop("Zeny#t"):add_item(RED_POTION)`}},
		{`{ npcshopupdate "Market#t",501,70,5; end; }`, []string{` = world.get_shop("Market#t")`, `:add_item(RED_POTION, { cost = { zeny = 70 } })`, `:set_stock(RED_POTION, 5)`}},
		{`{ npcshopupdate "Item#t",501,5; end; }`, []string{`world.get_shop("Item#t"):add_item(RED_POTION, { cost = { items = { { item = PORING_COIN, amount = 5 } } } })`}},
		{`{ npcshopupdate "Point#t",501,8; end; }`, []string{`world.get_shop("Point#t"):add_item(RED_POTION, { cost = { currency = { eventpoints = 8 } } })`}},
		{`{ npcshopupdate "Cash#t",501,9; end; }`, []string{`world.get_shop("Cash#t"):add_item(RED_POTION, { cost = { currency = { cash = 9 } } })`}},
		{`{ npcshopadditem "Zeny#t",512,20; end; }`, []string{`world.get_shop("Zeny#t"):add_item(APPLE, { cost = { zeny = 20 } })`}},
		{`{ npcshopadditem "Zeny#t",512,-1,909,4; end; }`, []string{`:add_item(APPLE)`, `:add_item(JELLOPY, { cost = { zeny = 4 } })`}},
		{`{ npcshopadditem "Market#t",512,20,10,909,3,-1; end; }`, []string{`:add_item(APPLE, { cost = { zeny = 20 } })`, `:set_stock(APPLE, 10)`, `:add_item(JELLOPY, { cost = { zeny = 3 } })`}},
		{`{ npcshopadditem "Point#t",512,7; end; }`, []string{`world.get_shop("Point#t"):add_item(APPLE, { cost = { currency = { eventpoints = 7 } } })`}},
		{`{ npcshopdelitem "Zeny#t",501; end; }`, []string{`world.get_shop("Zeny#t"):remove_item(RED_POTION)`}},
		{`{ npcshopdelitem "Zeny#t",501,512; end; }`, []string{`:remove_item(RED_POTION)`, `:remove_item(APPLE)`}},
		{`{ openmail; end; }`, []string{"player:open_mail()"}},
		{`{ searchstores 10,SEARCHSTORE_EFFECT_NORMAL; end; }`, []string{`player:search_stores({ uses = 10, effect = "show" })`}},
		{`{ searchstores 1,SEARCHSTORE_EFFECT_REMOTE,"this"; end; }`, []string{`player:search_stores({ uses = 1, effect = "open" })`}},
		{`{ buyingstore 4; end; }`, []string{"player:open_buying_store({ slots = 4 })"}},
		{`{ mail getcharid(0),"Poring","Welcome","Hi!",5000; end; }`,
			[]string{`player:send_mail({ to = player.character_id, sender = "Poring", title = "Welcome", body = "Hi!", zeny = 5000 })`}},
		{`{ mail 150001,"Poring","Welcome","Hi!"; end; }`, []string{`player:send_mail({ to = 150001, sender = "Poring", title = "Welcome", body = "Hi!" })`}},
		{`{ setarray .@id,501,512; setarray .@n,2,1; mail getcharid(0),"P","T","B",0,.@id,.@n; end; }`, []string{"items = R.mail_items(id, n)"}},
		{`{ .@id = 501; .@n = 1; mail getcharid(0),"P","T","B",0,.@id,.@n; end; }`, []string{"items = R.mail_items({ [0] = id }, { [0] = n })"}},
		{`{ .@c = #CASHPOINTS; end; }`, []string{`c = player:get_currency("cash")`}},
		{`{ if (#KAFRAPOINTS > 10) end; end; }`, []string{`if (player:get_currency("kafra") > 10) then`}},
		{`{ #CASHPOINTS += 10; end; }`, []string{`player:give_currency("cash", 10)`}},
		{`{ #KAFRAPOINTS -= 5; end; }`, []string{`player:take_currency("kafra", 5)`}},
		{`{ set #CASHPOINTS, #CASHPOINTS - .@n; end; }`, []string{`R.add_currency(player, "cash", -n)`}},
		{`{ set #CASHPOINTS, #CASHPOINTS + .@n; end; }`, []string{`R.add_currency(player, "cash", n)`}},
		{`{ #CASHPOINTS = 100; end; }`, []string{`R.set_currency(player, "cash", 100)`}},
		{`{ #CASHPOINTS *= 2; end; }`, []string{`R.set_currency(player, "cash", (player:get_currency("cash") * 2))`}},
		{`{ #EVENTPOINTS++; end; }`, []string{`player:give_currency("eventpoints", 1)`}},
		{`{ Quest_Points -= 2; end; }`, []string{`player:take_currency("quest_points", 2)`}},
		{`{ #CASHPOINTS += 0; end; }`, []string{"-- #CASHPOINTS changes by 0."}},
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
		{`{ callshop "Nope"; end; }`, "callshop: Nope is not a shop"},
		{`{ callshop "Refined Barter#t"; end; }`, "callshop: the shop Refined Barter#t does not convert"},
		{`{ callshop .@s$; end; }`, "callshop: a computed shop"},
		{`{ npcshopupdate "Barter#t",984,0,5; end; }`, "npcshopupdate: Barter#t is a barter shop"},
		{`{ npcshopupdate "Cash#t",501,-1; end; }`, "npcshopupdate: a price of -1, which rAthena refuses outside shop and marketshop lines"},
		{`{ npcshopupdate "Zeny#t",512,70; end; }`, "npcshopupdate: a new price for an item that the shop's line does not sell"},
		{`{ npcshopupdate "Zeny#t",501,.@p; end; }`, "npcshopupdate: a computed price"},
		{`{ npcshopupdate "Market#t",501,0,-2; end; }`, "npcshopupdate: a stock below -1"},
		{`{ npcshopadditem "Zeny#t",501,20; end; }`, "npcshopadditem: Red_Potion is in the shop already, and rAthena lists it twice"},
		{`{ npcshopadditem "Zeny#t",.@i,20; end; }`, "npcshopadditem: a computed item"},
		{`{ npcshopadditem "Item#t",909,-1; end; }`, "npcshopadditem: a price of -1, which rAthena refuses outside shop and marketshop lines"},
		{`{ npcshopdelitem "Barter#t",984; end; }`, "npcshopdelitem: Barter#t is a barter shop"},
		{`{ npcshopitem "Zeny#t",501,10; end; }`, "npcshopitem: it replaces every item of a shop"},
		{`{ npcshopattach "Zeny#t"; end; }`, "npcshopattach: rAthena lets the script take over the shop's sales"},
		{`{ sellitem 501; end; }`, "sellitem: rAthena has no such command"},
		{`{ stopselling 501; end; }`, "stopselling: rAthena has no such command"},
		{`{ .@r = npcshopupdate("Zeny#t",501,0); end; }`, "npcshopupdate: its result, which converts only as a statement"},
		{`{ openmail getcharid(0); end; }`, "openmail: for another character"},
		{`{ searchstores 10,SEARCHSTORE_EFFECT_NORMAL,"all"; end; }`, "searchstores: a map"},
		{`{ searchstores 10,.@e; end; }`, "searchstores: an effect other than"},
		{`{ mail getcharid(0),"P","T","B",0,.@id,.@n,.@refine; end; }`, "mail: refine, bound, card and random option arrays"},
		{`{ mail getcharid(0),"P","T","B",0,.@id; end; }`, "mail: arguments"},
		{`{ mail getcharid(0),"P","T","B",0,.@id[1],.@n; end; }`, "mail: an array from an index other than 0"},
		{`{ input #CASHPOINTS; end; }`, "#CASHPOINTS: written by a command"},
		{`{ .@c = #CASHPOINTS[1]; end; }`, "#CASHPOINTS: read as an array"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}

// TestRunShopCommands runs a converted script with the shop, mail and
// currency calls against a test host.
func TestRunShopCommands(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	res := shopResult(t)
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "Run#t", Map: "prontera", Script: `{
	#CASHPOINTS += 10;
	if (#CASHPOINTS >= 10) #CASHPOINTS -= 3;
	.@n = 4;
	set #KAFRAPOINTS, #KAFRAPOINTS + .@n;
	set #KAFRAPOINTS, #KAFRAPOINTS - 10;
	#EVENTPOINTS = 6;
	#EVENTPOINTS = 2;
	npcshopupdate "Market#t",501,0,7;
	npcshopadditem "Zeny#t",512,20;
	npcshopdelitem "Zeny#t",501;
	setarray .@id,501,512;
	setarray .@amount,2,1;
	mail getcharid(0),"Poring","Welcome","Hi!",100,.@id,.@amount;
	openmail;
	searchstores 5,SEARCHSTORE_EFFECT_REMOTE;
	buyingstore 3;
	end;
}`})
	lua, err := n.npcScript()
	if err != nil {
		t.Fatal(err)
	}
	_, base, core := testRuntime(t, reference.Renewal)
	h := &npcHost{testHost: base, npcs: map[int64]*NPC{1: {Source: "Run#t", Map: "prontera"}}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{},
		market: marketHost{balances: map[string]int64{}, shops: map[string]map[uint32]int{
			"Market#t": {501: 5, 512: -1}, "Zeny#t": {501: -1},
		}}}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{"scripts/npcs/Run_t.lua": lua, RathenaLib: rathenaLua}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
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
	npc, err := rt.StartNPC(1, "core:scripts/npcs/Run_t.lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Click(npc, 1); err != nil {
		t.Fatal(err)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	m := h.market
	// kafra: 0 + 4 = 4, and taking 10 fails, as the player has too little.
	for name, want := range map[string]int64{"cash": 7, "kafra": 4, "eventpoints": 2} {
		if m.balances[name] != want {
			t.Errorf("%s: %d, want %d", name, m.balances[name], want)
		}
	}
	if got := m.shops["Market#t"][501]; got != 7 {
		t.Errorf("Market#t stock of Red_Potion: %d, want 7", got)
	}
	if _, ok := m.shops["Zeny#t"][501]; ok {
		t.Error("Zeny#t still sells Red_Potion")
	}
	if got, ok := m.shops["Zeny#t"][512]; !ok || got != -1 {
		t.Errorf("Zeny#t Apple: %d, %v; want -1, true", got, ok)
	}
	if len(m.mails) != 1 {
		t.Fatalf("mails %+v, want 1", m.mails)
	}
	mail := m.mails[0]
	if mail.ToID != 1 || mail.Sender != "Poring" || mail.Title != "Welcome" || mail.Body != "Hi!" || mail.Zeny != 100 ||
		len(mail.Items) != 2 || mail.Items[0].Item != 501 || mail.Items[0].Amount != 2 || mail.Items[1].Item != 512 || mail.Items[1].Amount != 1 {
		t.Errorf("mail %+v", mail)
	}
	if got := strings.Join(m.calls, "|"); got != "open_mail|search_stores 5 true|open_buying_store 3" {
		t.Errorf("calls %q", got)
	}
}
