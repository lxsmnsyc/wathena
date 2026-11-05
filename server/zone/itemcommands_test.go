package zone

import (
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/item"
)

// The calls of decision 0072: jobs.name, player:count_equipped,
// take_item's and count_item's exact match, and a random option's id.

// TestCountEquippedAndOptionID checks player:count_equipped, as
// isequippedcnt: a worn copy counts its amount, a card counts per slot in
// worn copies, and a copy that is only carried does not count. It also
// checks the id of a random option. tools/convert's TestRunItemCommands
// checks jobs.name, which needs the constants.
func TestCountEquippedAndOptionID(t *testing.T) {
	z, p, logs := rollZone(t, `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(player)
    timers.after(1000, function()
      local shirt = player:get_equipped("armor")
      player:send_message(shirt.random_options[1].id .. " " .. player:count_equipped(2302) .. " " .. player:count_equipped(4001) ..
        " " .. player:count_equipped(2220))
    end)
  end)
end`)
	z.Mods.RandomOptions.Options["max_hp"].ID = 1
	if r := z.addCopy(p, rollShirt, 1, 0, nil, item.Extra{Options: []item.Option{{Name: "max_hp", Value: 100}}}, scriptOrigin); r != item.AddSuccess {
		t.Fatal(r)
	}
	if r := z.addCopy(p, rollCard, 1, 0, nil, item.Extra{}, scriptOrigin); r != item.AddSuccess {
		t.Fatal(r)
	}
	if r := z.addCopy(p, rollHat, 1, 0, nil, item.Extra{}, scriptOrigin); r != item.AddSuccess {
		t.Fatal(r)
	}
	shirt, card := slotOf(t, p, rollShirt), slotOf(t, p, rollCard)
	z.insertCard(p, card, shirt)
	equip(z, p, shirt)
	for now := z.now + 50; now <= 1500; now += 50 {
		z.stepAt(now)
	}
	got := messages(events(p))
	want := []string{"1 1 1 0"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q: %s", got, want, logs)
	}
}

// TestTakeItemExact checks take_item and count_item with refine, cards
// and signed_by_character, as delitem2 and countitem2: only identified,
// unbroken copies with that refine, those cards and that signer match,
// and too few take nothing.
func TestTakeItemExact(t *testing.T) {
	z, p, n, logs := npcZone(t, clickScript(`
    local knife = items.id("Knife")
    player:send_message(player:count_item(knife, { refine = 3 }) .. " " .. player:count_item(knife, { signed_by_character = 77 }) ..
      " " .. player:count_item(knife, { refine = 0 }))
    player:send_message(tostring(player:take_item(knife, 2, { refine = 3 })) .. " " .. tostring(player:take_item(knife, 1, { signed_by_character = 77 })) ..
      " " .. tostring(player:take_item(knife, 1, { refine = 3 })))
    player:send_message(player:count_item(knife, { refine = 3 }) .. " " .. player:count_item(knife, { signed_by_character = 77 }) .. " " .. player:count_item(knife))
    local ok, err = pcall(player.take_item, player, knife, 1, { refine = 3, from = "cart" })
    player:send_message(tostring(err))
`))
	p.maxWeight = 1 << 30
	for _, c := range []struct {
		refine int
		x      item.Extra
	}{{3, item.Extra{}}, {3, item.Extra{Broken: true}}, {0, item.Extra{SignedBy: 77}}, {0, item.Extra{}}} {
		if r := z.addCopy(p, 1201, 1, c.refine, nil, c.x, scriptOrigin); r != item.AddSuccess {
			t.Fatal(r)
		}
	}
	got := messages(click(z, p, n))
	want := []string{"1 1 1", "false true true", "0 0 2",
		"player:take_item: refine, cards and signed_by_character take from the inventory only, without from"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q: %s", got, want, logs)
	}
	for i := range want {
		// The error line has the call's place before the message.
		if !strings.HasSuffix(got[i], want[i]) {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
