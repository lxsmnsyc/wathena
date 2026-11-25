package zone

import (
	"github.com/lxsmnsyc/wathena/server/item"
	"slices"
	"testing"
)

// The calls of decision 0078: player:transform's options, player:set_font,
// and the position of an equipped copy.

// TestTransformStatusAndActive checks player:transform with the status
// option, as transform's SC_MTF_ types: both statuses start for the same
// time, the active option starts SC_ACTIVE_MONSTER_TRANSFORM, and the
// transform's end ends the status that came with it.
func TestTransformStatusAndActive(t *testing.T) {
	z, p, n, logs := npcZone(t, `
local events = require "wa.events"
return function()
  local step = 0
  events.on("click", function(player)
    step = step + 1
    if step == 1 then
      player:send_message(tostring(player:transform(1002, 1000, { status = 581, values = { 20, 5 } })))
      player:send_message(tostring(player:has_status(566)) .. " " ..
        tostring(player:has_status(638)) .. " " .. tostring(player:has_status(581)))
      player:set_font(2)
    else
      player:send_message(tostring(player:transform(1002, 1000, { active = true })))
      player:send_message(tostring(player:has_status(566)) .. " " ..
        tostring(player:has_status(638)))
    end
  end)
end`)
	z.Statuses = testStatuses(t)
	z.Mods.Monsters = map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring"}}
	got := messages(click(z, p, n))
	if want := []string{"true", "true false true"}; !slices.Equal(got[:min(len(got), 2)], want) {
		t.Fatalf("got %q, want %q; logs %s", got, want, logs)
	}
	if p.font != 2 {
		t.Errorf("font %d, want 2", p.font)
	}
	for end, now := z.now+1500, z.now+50; now <= end; now += 50 {
		z.stepAt(now)
	}
	if p.transform != 0 {
		t.Errorf("the transform did not end: %d", p.transform)
	}
	if p.entity.sc.Has(scMonsterTransform) || p.entity.sc.Has(581) {
		t.Errorf("a status stayed after the transform ended")
	}
	got = messages(click(z, p, n))
	if want := []string{"true", "false true"}; !slices.Equal(got[:min(len(got), 2)], want) {
		t.Errorf("active: got %q, want %q; logs %s", got, want, logs)
	}
}

// TestEquippedPosition checks equipped.position, as getitempos: the EQP_
// bit of the slot the copy sits in.
func TestEquippedPosition(t *testing.T) {
	z, p, logs := rollZone(t, `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(player)
    timers.after(1000, function()
      player:send_message(tostring(player:get_equipped("armor").position))
    end)
  end)
end`)
	if r := z.addCopy(p, rollShirt, 1, 0, nil, item.Extra{}, scriptOrigin); r != item.AddSuccess {
		t.Fatal(r)
	}
	equip(z, p, slotOf(t, p, rollShirt))
	for now := z.now + 50; now <= 1500; now += 50 {
		z.stepAt(now)
	}
	// EQP_ARMOR is 16.
	if got, want := messages(events(p)), []string{"16"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q: %s", got, want, logs)
	}
}
