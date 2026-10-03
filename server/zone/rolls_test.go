package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Items with rolls for these tests, made from equipItems:
//   - the hat (2220) rolls luck from 1 to 5, its LUK bonus;
//   - the shirt (2302) rolls might from 1 to 9 and its Lua script adds it
//     to STR;
//   - the Poring Card (4001) rolls a hidden dodge from 1 to 9, its
//     Perfect Dodge bonus.
const (
	rollHat   = 2220
	rollShirt = 2302
	rollCard  = 4001
)

func staticBonus(t *testing.T, entry ...any) *bonus.Bonus {
	t.Helper()
	b, err := bonus.FromData(entry, &bonus.Tables{})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rollItems(t *testing.T) *item.Table {
	t.Helper()
	tb := equipItems(t)
	hat, _ := tb.Lookup(rollHat)
	hat.Rolls = map[string]*item.RollDef{"luck": {Min: 1, Max: 5, Default: 3, Distribution: item.Uniform, Label: "LUK"}}
	hat.Static = []*bonus.Bonus{staticBonus(t, "luk", map[string]any{"roll": "luck"})}
	shirt, _ := tb.Lookup(rollShirt)
	shirt.Rolls = map[string]*item.RollDef{"might": {Min: 1, Max: 9, Default: 2, Distribution: item.Uniform}}
	shirt.Script = "example.test:scripts/items/shirt.lua"
	card, _ := tb.Lookup(rollCard)
	card.Rolls = map[string]*item.RollDef{"dodge": {Min: 1, Max: 9, Default: 1, Hidden: true}}
	card.Static = []*bonus.Bonus{staticBonus(t, "perfect_dodge", map[string]any{"roll": "dodge"})}
	return tb
}

// rollOptions has one option, max_hp, and a group that always gives it
// at 100.
func rollOptions(t *testing.T) *mod.RandomOptions {
	t.Helper()
	return &mod.RandomOptions{
		Options: map[string]*item.RandomOption{"max_hp": {Name: "max_hp", DisplayName: "MaxHP",
			Static: []*bonus.Bonus{staticBonus(t, "max_hp", map[string]any{"roll": "value"})}}},
		Groups: map[string]*item.OptionGroup{"TEST": {Name: "TEST", Slots: [][]item.OptionEntry{{{Option: "max_hp", Min: 100, Max: 100, Chance: 10000}}}}},
	}
}

const shirtScript = `
local bonuses = require "wa.bonuses"
local context = require "wa.context"
return function()
  bonuses.add("str", context:get_instance().rolls.might)
end`

// rollZone is a zone with the rolled items, the item_rolls flag on, and a
// mod with src as its entry script and every permission rolls need. The
// player is a level 50 Novice.
func rollZone(t *testing.T, src string) (*Zone, *Player, *syncBuffer) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{7}).Stream(), nil)
	z.Items = rollItems(t)
	z.Progress = testTables()
	z.Status = jobTables(t)
	z.Start = status.Character{Male: true, Stats: status.Stats{Str: 50, Agi: 30, Vit: 30, Int: 1, Dex: 30, Luk: 1}}
	logs := &syncBuffer{}
	sc := testMod(src)
	sc.Files["scripts/items/shirt.lua"] = shirtScript
	for _, g := range []string{"items.modify_rolls", "rules.override", "player.give_items", "game.read", "event.listen"} {
		sc.Grants[g] = true
	}
	z.Mods = &Mods{Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.NewTextHandler(logs, nil)),
		RandomOptions: rollOptions(t), ItemRolls: true}
	z.startScripts()
	t.Cleanup(z.stopScripts)
	p := add(z, "a")
	p.progress.BaseLevel = 50
	z.recalc(p)
	z.runScripts()
	z.stepAt(50)
	events(p)
	return z, p, logs
}

func slotOf(t *testing.T, p *Player, id uint32) int {
	t.Helper()
	for i, s := range p.inventory.Slots {
		if s.ID == id {
			return i
		}
	}
	t.Fatalf("item %d not found", id)
	return -1
}

func TestRolledStaticBonus(t *testing.T) {
	z, p, _ := rollZone(t, "return function() end")
	luk := p.entity.status.Luk
	if r := z.addCopy(p, rollHat, 1, 0, nil, item.Extra{Rolls: item.RollsOf(map[string]int{"luck": 5})}, scriptOrigin); r != item.AddSuccess {
		t.Fatal(r)
	}
	slot := slotOf(t, p, rollHat)
	equip(z, p, slot)
	z.stepAt(100)
	if got := p.entity.status.Luk; got != luk+5 {
		t.Errorf("LUK %d with a hat of luck 5, want %d", got, luk+5)
	}
	// A copy without rolls reads the defaults.
	unequip(z, p, slot)
	p.inventory.Slots[slot].Rolls = nil
	equip(z, p, slot)
	z.stepAt(150)
	if got := p.entity.status.Luk; got != luk+3 {
		t.Errorf("LUK %d with a hat of no rolls, want the default 3: %d", got, luk+3)
	}
}

func TestDropRolls(t *testing.T) {
	z, _, _ := rollZone(t, "return function() end")
	d := z.Items.Get(rollHat)
	// Flag on: values spread over the range. Off: always the default.
	seen := map[int]int{}
	for range 2000 {
		x := z.dropCopy(rollHat, "", nil).extra
		v, _ := x.Rolls.Get("luck")
		if v < 1 || v > 5 {
			t.Fatalf("luck %d", v)
		}
		seen[v]++
	}
	if len(seen) != 5 {
		t.Errorf("values %v, want each of 1 to 5", seen)
	}
	z.Mods.ItemRolls = false
	for range 200 {
		if v, _ := z.dropCopy(rollHat, "", nil).extra.Rolls.Get("luck"); v != d.Rolls["luck"].Default {
			t.Fatalf("luck %d with item_rolls off", v)
		}
	}
	// Random options roll either way.
	x := z.dropCopy(rollHat, "TEST", nil).extra
	if len(x.Options) != 1 || x.Options[0] != (item.Option{Name: "max_hp", Value: 100}) {
		t.Errorf("options %v", x.Options)
	}
	// A rolled copy with a hidden roll starts unidentified.
	z.Mods.ItemRolls = true
	if x := z.dropCopy(rollCard, "", nil).extra; !x.Unidentified {
		t.Error("a dropped card with a hidden roll is identified")
	}
}

func TestItemRollHook(t *testing.T) {
	z, p, logs := rollZone(t, `
local hooks = require "wa.hooks"
local events = require "wa.events"
return function()
  hooks.on("item_roll", function(roll, next)
    local v = next()
    if roll.source.kind == "drop" then return 99 end
    if roll.source.kind == "script" and roll.name == "luck" then return roll.min end
    return v
  end)
  events.on("login", function(player)
    player:give_item(2220, 1, { roll = "random" })
  end)
end`)
	// The hook's 99 is clamped to the roll's 5.
	if v, _ := z.dropCopy(rollHat, "", nil).extra.Rolls.Get("luck"); v != 5 {
		t.Errorf("hooked drop luck %d, want 5: %s", v, logs)
	}
	// A script's give runs the hooks too, inside the script's call.
	b := add(z, "b")
	z.runScripts()
	z.stepAt(z.now + 50)
	if v, _ := b.inventory.Slots[slotOf(t, b, rollHat)].Rolls.Get("luck"); v != 1 {
		t.Errorf("given luck %d, want the hook's 1: %s", v, logs)
	}
	_ = p
}

func TestGiveItemRolls(t *testing.T) {
	z, p, logs := rollZone(t, `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    player:give_item(2220, 1, { roll = { luck = 4 } })
    player:give_item(2220, 1, { roll = { luck = 40 } })
    player:give_item(2220, 1)
    local ok, err = pcall(player.give_item, player, 2220, 1, { roll = { power = 1 } })
    player:send_message(tostring(err))
    ok, err = pcall(player.give_item, player, 2220, 1, { roll = "best" })
    player:send_message(tostring(err))
  end)
end`)
	var got []int
	for _, s := range p.inventory.Slots {
		if s.ID == rollHat {
			v, _ := s.Rolls.Get("luck")
			got = append(got, v)
		}
	}
	if len(got) != 3 || got[0] != 4 || got[1] != 5 || got[2] != 3 {
		t.Errorf("luck %v, want 4, 5 (clamped) and the default 3: %s", got, logs)
	}
	_ = z
}

func TestEquippedRollsAndReroll(t *testing.T) {
	z, p, logs := rollZone(t, `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(player)
    timers.after(1000, function()
      local shirt = player:get_equipped("armor")
      player:send_message("might " .. shirt.rolls.might .. " cards " .. #shirt.cards .. " dodge " .. shirt.cards[1].rolls.dodge ..
        " option " .. shirt.random_options[1].option .. " " .. shirt.random_options[1].value)
      local ok, err = pcall(function() shirt.rolls.might = 9 end)
      player:send_message(tostring(err))
      shirt:set_roll("might", 7)
      player:send_message("set " .. shirt.rolls.might)
      shirt:reroll()
    end)
  end)
end`)
	x := item.Extra{Rolls: item.RollsOf(map[string]int{"might": 4}), Options: []item.Option{{Name: "max_hp", Value: 100}}}
	if r := z.addCopy(p, rollShirt, 1, 0, nil, x, scriptOrigin); r != item.AddSuccess {
		t.Fatal(r)
	}
	if r := z.addCopy(p, rollCard, 1, 0, nil, item.Extra{Rolls: item.RollsOf(map[string]int{"dodge": 6})}, scriptOrigin); r != item.AddSuccess {
		t.Fatal(r)
	}
	shirt, card := slotOf(t, p, rollShirt), slotOf(t, p, rollCard)
	z.insertCard(p, card, shirt)
	str, maxHP, dodge := p.entity.status.Str, p.entity.status.MaxHP, p.entity.status.Flee2
	equip(z, p, shirt)
	z.stepAt(100)
	if got := p.entity.status.Str; got != str+4 {
		t.Errorf("STR %d, want %d from the shirt's might: %s", got, str+4, logs)
	}
	if got := p.entity.status.MaxHP; got != maxHP+100 {
		t.Errorf("MaxHP %d, want %d from the random option", got, maxHP+100)
	}
	if got := p.entity.status.Flee2; got != dodge+60 {
		t.Errorf("Perfect Dodge %d, want %d from the card's roll of 6", got, dodge+60)
	}
	events(p)
	z.stepAt(1100)
	ev := events(p)
	lines := strings.Join(systemLines(ev), "|")
	if !strings.Contains(lines, "might 4 cards 1 dodge 6 option max_hp 100") || !strings.Contains(lines, "read-only") || !strings.Contains(lines, "set 7") {
		t.Errorf("lines %q, logs: %s", lines, logs)
	}
	// The reroll after set_roll draws again; the shirt's script started
	// again with the new value.
	v, _ := p.inventory.Slots[shirt].Rolls.Get("might")
	if got := p.entity.status.Str; got != str+v {
		t.Errorf("STR %d after the reroll to %d, want %d", got, v, str+v)
	}
	if findEvent(ev, func(e *wathenav1.Event) bool { return e.GetItemRollsChange() != nil }) == nil {
		t.Error("no ItemRollsChange")
	}
}

func TestRollMessages(t *testing.T) {
	z, p, _ := rollZone(t, "return function() end")
	z.addCopy(p, rollCard, 1, 0, nil, item.Extra{Rolls: item.RollsOf(map[string]int{"dodge": 6}), Unidentified: true}, scriptOrigin)
	z.addCopy(p, rollHat, 1, 0, nil, item.Extra{Rolls: item.RollsOf(map[string]int{"luck": 2}), Options: []item.Option{{Name: "max_hp", Value: 100}}}, scriptOrigin)
	z.stepAt(z.now + 50)
	ev := events(p)
	var adds []*wathenav1.InventoryAdd
	for _, e := range ev {
		if a := e.GetInventoryAdd(); a != nil {
			adds = append(adds, a)
		}
	}
	if len(adds) != 2 {
		t.Fatalf("%d adds", len(adds))
	}
	// The hidden roll of an unidentified copy goes without its value.
	if r := adds[0].GetRolls(); len(r) != 1 || r[0].GetKnown() || r[0].GetValue() != 0 || r[0].GetMin() != 1 || r[0].GetMax() != 9 {
		t.Errorf("hidden roll %v", r)
	}
	if r := adds[1].GetRolls(); len(r) != 1 || !r[0].GetKnown() || r[0].GetValue() != 2 || r[0].GetLabel() != "LUK" {
		t.Errorf("roll %v", r)
	}
	if o := adds[1].GetOptions(); len(o) != 1 || o[0].GetLabel() != "MaxHP" || o[0].GetValue() != 100 {
		t.Errorf("options %v", o)
	}
	// The whole inventory carries them too.
	list := z.inventoryList(p).GetInventory().GetItems()
	if len(list) != 2 || len(list[1].GetRolls()) != 1 || len(list[1].GetOptions()) != 1 {
		t.Errorf("inventory %v", list)
	}
}

func TestRollsLoadAndSplit(t *testing.T) {
	z, p, logs := rollZone(t, "return function() end")
	// Saving keeps the rolls, and loading clamps and logs a value outside
	// the range.
	c := &character.Character{Info: character.Info{Name: "c", InventorySize: item.BaseSlots}, Items: []character.Item{
		{Slot: 0, ID: rollHat, Amount: 1, Extra: item.Extra{Rolls: item.RollsOf(map[string]int{"luck": 50, "old": 1})}},
		{Slot: 1, ID: rollCard, Amount: 5, Extra: item.Extra{Rolls: item.RollsOf(map[string]int{"dodge": 4})}},
	}}
	q := &Player{Name: "c", inventory: restoreInventory(z.Items, c)}
	z.fixRolls(q)
	if got := q.inventory.Slots[0].Rolls.Key(); got != "luck=5" {
		t.Errorf("loaded rolls %q", got)
	}
	if !strings.Contains(logs.String(), "item rolls fixed on load") {
		t.Errorf("no log: %s", logs)
	}
	saved := savedItems(q.inventory)
	if saved[1].Rolls.Key() != "dodge=4" {
		t.Errorf("saved %+v", saved)
	}
	// Dropping part of a stack, then picking it up, gives a new serial and
	// logs the stack it came from.
	z.addCopy(p, rollCard, 5, 0, nil, item.Extra{Rolls: item.RollsOf(map[string]int{"dodge": 4})}, scriptOrigin)
	slot := slotOf(t, p, rollCard)
	from := p.inventory.Slots[slot].Serial
	z.dropItem(p, slot, 2)
	var it *entity
	for e := range z.entities.all() {
		if e.item != nil && e.item.id == rollCard {
			it = e
		}
	}
	if it == nil || it.item.origin != from || it.item.extra.Rolls.Key() != "dodge=4" {
		t.Fatalf("ground item %+v", it)
	}
	// The same rolls join the stack again.
	z.take(p, it, 0)
	if p.inventory.Slots[slot].Amount != 5 {
		t.Errorf("amount %d after picking the split back up", p.inventory.Slots[slot].Amount)
	}
	// The pick-up's log line names the stack the part was dropped from.
	got := z.txLogs.Items
	if n := len(got); n < 2 || got[n-2].Action != transfer.ActionDrop || got[n-2].Serial != from ||
		got[n-1].Action != transfer.ActionPickUp || got[n-1].Source != from || got[n-1].Serial != from || got[n-1].Amount != 2 {
		t.Errorf("logs %+v", got)
	}
}
