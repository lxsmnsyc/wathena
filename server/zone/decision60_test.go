package zone

import (
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
  on("OnSelf", function() maps.find_player("a"):say_aloud("hush", { to = "self" }) end)
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
