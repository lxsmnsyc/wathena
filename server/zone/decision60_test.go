package zone

import (
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// decision60Script runs the calls of review decision 60 that
// npc-api-events builds on npc_event labels, and notes results in the
// variable seen.
const decision60Script = `
local events = require "wa.events"
local maps = require "wa.maps"
local spawning = require "wa.spawning"
local variables = require "wa.variables"
local seen = variables.server_temporary_string("seen")
local function note(s) seen:set(seen:get() .. s .. ";") end
local kept
return function()
  local function on(name, f) events.on("npc_event", f, { name = name }) end
  on("OnEmote", function()
    kept = spawning.monsters { monster = 1002, x = 22, y = 20 }[1]
    kept:show_emotion(9) -- ET_HNG
    note("bad " .. tostring(pcall(kept.show_emotion, kept, 300)))
  end)
  on("OnSpawnLabeled", function()
    kept = spawning.monsters { monster = 1002, x = 30, y = 30, label = "OnMyMobDead" }[1]
    note("gid " .. kept.game_id)
    kept:kill()
  end)
  on("OnMyMobDead", function(event)
    note("dead " .. tostring(event.monster_id) .. " " .. tostring(event.monster_game_id))
  end)
  on("OnPlain", function(event) note("plain " .. tostring(event.monster_id)) end)
  on("OnSelf", function() maps.find_player("a"):say_aloud("hush", { to = "self" }) end)
  on("OnUnEffect", function()
    local npc = require("wa.context"):get_instance()
    npc:remove_effect(10, { to = maps.find_player("a") })
    maps.find_player("a"):remove_effect(11, { to = "map" })
  end)
end`

func emotionsOf(evs []*wathenav1.Event) map[uint32]uint32 {
	out := map[uint32]uint32{}
	for _, e := range evs {
		if em := e.GetEmotion(); em != nil {
			out[em.GetEntity()] = em.GetEmotion()
		}
	}
	return out
}

func chatTexts(evs []*wathenav1.Event) []string {
	var out []string
	for _, e := range evs {
		if c := e.GetChat(); c != nil {
			out = append(out, c.GetText())
		}
	}
	return out
}

// TestMonsterEmotion checks monster:show_emotion (review decision 60,
// B9): the players who see the monster get the emotion.
func TestMonsterEmotion(t *testing.T) {
	z, p, n := followupZoneWith(t, decision60Script)
	fire(t, z, n, "OnEmote")
	z.stepAt(z.now + 50)
	var mob *entity
	for e := range z.entities.all() {
		if e.monster != nil {
			mob = e
		}
	}
	id, _ := p.netIDs.get(mob)
	if got := emotionsOf(events(p)); got[id] != 9 {
		t.Errorf("emotions %v, want hng (9) on the monster %d", got, id)
	}
	if got := seen(t, z); got != "bad false;" {
		t.Errorf("got %q", got)
	}
}

// TestSayAloudSelf checks player:say_aloud's to = "self" (review
// decision 60, B11): only the speaker hears it.
func TestSayAloudSelf(t *testing.T) {
	z, p, n := followupZoneWith(t, decision60Script)
	other := add(z, "b")
	other.entity.x, other.entity.y = 21, 21
	z.stepAt(z.now + 50)
	events(p)
	events(other)
	fire(t, z, n, "OnSelf")
	z.stepAt(z.now + 50)
	if got := chatTexts(events(p)); len(got) != 1 || got[0] != "hush" {
		t.Errorf("the speaker got %q", got)
	}
	if got := chatTexts(events(other)); len(got) != 0 {
		t.Errorf("another player heard %q", got)
	}
}

func effectRemovals(evs []*wathenav1.Event) []uint32 {
	var out []uint32
	for _, e := range evs {
		if r := e.GetEffectRemove(); r != nil {
			out = append(out, r.GetEffectId())
		}
	}
	return out
}

// TestRemoveEffectTo checks remove_effect's to (review decision 60, D6):
// a player alone, or every player of the map who sees the entity.
func TestRemoveEffectTo(t *testing.T) {
	z, p, n := followupZoneWith(t, decision60Script)
	other := add(z, "b")
	other.entity.x, other.entity.y = 21, 21
	z.stepAt(z.now + 50)
	events(p)
	events(other)
	fire(t, z, n, "OnUnEffect")
	z.stepAt(z.now + 50)
	if got := effectRemovals(events(p)); len(got) != 2 || got[0] != 10 || got[1] != 11 {
		t.Errorf("the player got %v", got)
	}
	if got := effectRemovals(events(other)); len(got) != 1 || got[0] != 11 {
		t.Errorf("another player got %v", got)
	}
}

// TestLabelMonster checks monster_id and monster_game_id on a monster
// label's event (review decision 60, B4), and nil on other events.
func TestLabelMonster(t *testing.T) {
	z, _, n := followupZoneWith(t, decision60Script)
	fire(t, z, n, "OnSpawnLabeled")
	z.stepAt(z.now + 50)
	z.runScripts()
	got := seen(t, z)
	gid := strings.TrimSuffix(strings.TrimPrefix(strings.SplitN(got, ";", 2)[0], "gid "), "")
	if want := "gid " + gid + ";dead 1002 " + gid + ";"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	fire(t, z, n, "OnPlain")
	if got := seen(t, z); got != "plain nil;" {
		t.Errorf("got %q", got)
	}
}
