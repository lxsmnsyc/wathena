package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// savingMaps are map/a and map/b with an NPC on map/a whose click saves
// the player's point on map/b at 30,31, and answers whether it could.
func savingMaps(t *testing.T) (a, b *Zone, n *entity) {
	t.Helper()
	a, b = twoMaps()
	a.SaveMap = "map/a"
	b.SaveMap = "map/a"
	src := `
local events = require "wa.events"
return function()
  events.on("click", function(player, conversation)
    conversation:say(tostring(player:set_save_point("b", 30, 31)), tostring(player:set_save_point("nowhere", 1, 1)))
  end)
end`
	a.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(src)}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.DiscardHandler),
		Server: variables.NewServer(nil, nil)}
	a.SetNPCs([]NPC{{Name: "example.test:Kafra", DisplayName: "Kafra", X: 10, Y: 10, Sprite: 112, Script: "example.test:scripts/npc.lua"}})
	t.Cleanup(a.stopScripts)
	for e := range a.entities.all() {
		if e.npc != nil {
			n = e
		}
	}
	return a, b, n
}

func TestSavePointAndRespawn(t *testing.T) {
	a, b, n := savingMaps(t)
	s := &saves{}
	p := onMap(a, "Saver", 11, 10, s)
	a.stepAt(50)
	events(p)
	ev := click(a, p, n)
	if got := lines(ev); got != "true|false" {
		t.Fatalf("set_save_point: %q", got)
	}
	if p.record.SaveMap != "map/b" || p.record.SaveX != 30 || p.record.SaveY != 31 {
		t.Fatalf("save point %s %d,%d", p.record.SaveMap, p.record.SaveX, p.record.SaveY)
	}
	answer(a, p, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	// A dead character is saved at its save point (pc_makesavestatus).
	p.entity.dead, p.entity.diedAt = true, a.now
	if c := a.snapshot(p); c.Map != "map/b" || c.X != 30 || c.Y != 31 {
		t.Fatalf("a dead character is saved at %s %d,%d", c.Map, c.X, c.Y)
	}
	// pc_respawn takes the player to its save point, on map/b.
	a.stepAt(a.now + reviveMs + 50)
	q := arrive(t, b)
	if q.entity.x != 30 || q.entity.y != 31 || q.entity.dead || q.entity.hp != int64(q.entity.status.MaxHP) {
		t.Fatalf("respawned at %d,%d dead %v hp %d", q.entity.x, q.entity.y, q.entity.dead, q.entity.hp)
	}
}

func TestButterflyWingGoesToSavePoint(t *testing.T) {
	a, b, _ := savingMaps(t)
	p := onMap(a, "Flyer", 11, 10, &saves{})
	h := scriptHost{a}
	// Without a save point, the start map's start cell.
	if ok, err := h.Warp(int64(p.charID), "save_point", "", 0, 0); !ok || err != nil {
		t.Fatal(ok, err)
	}
	x, y := a.startCell()
	if w := p.warpTo; w == nil || w.to != a || w.x != x || w.y != y {
		t.Fatalf("warp %+v", p.warpTo)
	}
	p.record.SaveMap, p.record.SaveX, p.record.SaveY = "map/b", 30, 31
	if ok, err := h.Warp(int64(p.charID), "save_point", "", 0, 0); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if w := p.warpTo; w == nil || w.to != b || w.x != 30 || w.y != 31 {
		t.Fatalf("warp %+v", p.warpTo)
	}
}

func TestAnnouncements(t *testing.T) {
	a, b := twoMaps()
	pa := onMap(a, "A", 5, 5, &saves{})
	pb := onMap(b, "B", 5, 5, &saves{})
	h := scriptHost{a}
	if err := h.Announce("", "to everyone"); err != nil {
		t.Fatal(err)
	}
	if err := h.Announce("b", "to map b"); err != nil {
		t.Fatal(err)
	}
	if err := h.Announce("nowhere", "lost"); err == nil {
		t.Error("an announcement to a map that does not run went through")
	}
	for _, z := range []*Zone{a, b} {
		for len(z.announces) > 0 {
			z.announce(<-z.announces)
		}
		z.stepAt(z.now + 50)
	}
	got := func(p *Player) []string {
		var out []string
		for _, e := range events(p) {
			if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT {
				out = append(out, c.GetText())
			}
		}
		return out
	}
	if g := got(pa); len(g) != 1 || g[0] != "to everyone" {
		t.Errorf("map a heard %q", g)
	}
	if g := got(pb); len(g) != 2 || g[0] != "to everyone" || g[1] != "to map b" {
		t.Errorf("map b heard %q", g)
	}
}

func TestNPCShowsAndSays(t *testing.T) {
	z, p, n, logs := npcZone(t, `
local events = require "wa.events"
local items = require "wa.items"
local text = require "wa.text"
return function()
  local npc = require("wa.context"):get_instance()
  events.on("click", function(player, conversation)
    conversation:show_image("kafra_01", "right")
    npc:show_emotion(15) -- emotions.thanks
    npc:say_aloud("Hello, " .. player.name)
    player:mark_minimap(1, 30, 31, 0xFF00FF)
    player:unmark_minimap(2)
    conversation:say(npc.name .. " at " .. npc.map .. " " .. npc.x .. "," .. npc.y, items.name(items.id "Red_Potion"), text.item_link(items.id "Red_Potion"))
  end)
end`)
	b := add(z, "b")
	b.entity.x, b.entity.y = 19, 20
	z.stepAt(z.now + 50)
	events(b)
	z.Items = shopItems()
	ev := click(z, p, n)
	var img *wathenav1.NpcImage
	var emo *wathenav1.Emotion
	var marks []*wathenav1.MinimapMark
	for _, e := range ev {
		if e.GetNpcImage() != nil {
			img = e.GetNpcImage()
		}
		if e.GetEmotion() != nil {
			emo = e.GetEmotion()
		}
		if e.GetMinimapMark() != nil {
			marks = append(marks, e.GetMinimapMark())
		}
	}
	id, _ := p.netIDs.get(n)
	if img == nil || img.GetName() != "kafra_01" || img.GetPosition() != wathenav1.ImagePosition_IMAGE_POSITION_RIGHT {
		t.Errorf("image %v, logs %s", img, logs)
	}
	if emo == nil || emo.GetEntity() != id || emo.GetEmotion() != 15 {
		t.Errorf("emotion %v", emo)
	}
	if len(marks) != 2 || marks[0].GetX() != 30 || marks[0].GetColor() != 0xFF00FF || !marks[1].GetRemove() {
		t.Errorf("minimap marks %v", marks)
	}
	if got := lines(ev); got != "Guide at test 20,20|Red_Potion|<ITEMLINK>Red_Potion<INFO>501</INFO></ITEMLINK>" {
		t.Errorf("lines %q", got)
	}
	// Players near the NPC hear it, as npctalk.
	heard := false
	for _, e := range events(b) {
		if c := e.GetChat(); c != nil && c.GetFromName() == "Guide" && c.GetText() == "Hello, a" {
			heard = true
		}
	}
	if !heard {
		t.Error("the other player did not hear the NPC")
	}
}

// TestAnnouncePermissionAndTakeZero checks the review of proposal 0002,
// decision 14: world.announce needs the permission world.announce, which
// the owner approves, maps.announce needs none, and take_item with an
// amount of 0 raises an error.
func TestAnnouncePermissionAndTakeZero(t *testing.T) {
	z, p, n, _ := npcZone(t, `
local events = require "wa.events"
local items = require "wa.items"
local maps = require "wa.maps"
local world = require "wa.world"
return function()
  events.on("click", function(player, conversation)
    local w, werr = pcall(world.announce, "to everyone")
    local m, merr = pcall(maps.announce, "test", "to map test")
    local took, terr = pcall(player.take_item, player, items.id "Red_Potion", 0)
    conversation:say(tostring(w), tostring(m), tostring(took), tostring(werr), tostring(merr), tostring(terr))
  end)
end`)
	delete(z.Mods.Scripts[0].Grants, "world.announce")
	ev := click(z, p, n)
	got := strings.Split(lines(ev), "|")
	if len(got) != 6 || got[0] != "false" || got[1] != "true" || got[2] != "false" {
		t.Fatalf("results %q", got)
	}
	if !strings.Contains(got[3], "needs the permission world.announce") {
		t.Errorf("world.announce error: %s", got[3])
	}
	if !strings.Contains(got[5], "an amount from 1 to 30000") {
		t.Errorf("take_item error: %s", got[5])
	}
	answer(z, p, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	for len(z.announces) > 0 {
		z.announce(<-z.announces)
	}
	z.stepAt(z.now + 50)
	var heard []string
	for _, e := range events(p) {
		if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT {
			heard = append(heard, c.GetText())
		}
	}
	if len(heard) != 1 || heard[0] != "to map test" {
		t.Errorf("heard %q", heard)
	}
}
