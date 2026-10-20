package zone

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// npcWorldScript is the script of every NPC of these tests. Each NPC
// notes what it does in the temporary server variable order, with its
// name.
const npcWorldScript = `
local context = require "wa.context"
local events = require "wa.events"
local world = require "wa.world"
local maps = require "wa.maps"
local variables = require "wa.variables"
local hits = variables.npc_integer("hits")
local order = variables.server_temporary_string("order")
return function()
  local npc = context:get_instance()
  local function note(s) order:set(order:get() .. npc.unique_name .. ":" .. s .. ";") end
  if npc.unique_name == "Sender" then
    events.on("click", function(player, conversation)
      local r = world.get_npc("Receiver")
      note("fire=" .. tostring(r:fire_event("OnPing")))
      note("none=" .. tostring(r:fire_event("OnNothing")))
      note("count=" .. world.fire_npc_event("onall"))
      note("with=" .. tostring(r:fire_event("OnTalk", { player = player })))
      hits:add(r, 2)
      note("after")
    end)
  end
  if npc.unique_name == "Receiver" then
    events.on("map_enter", function(player) note("enter@" .. player.name) end)
    events.on("npc_event", function(event)
      note("talk@" .. event.player.name)
      event.conversation:say("talk")
      event.conversation:wait_for_next()
      note("talk done")
    end, { name = "OnTalk" })
  end
  if npc.unique_name == "Trap" then
    events.on("monster_touch", function(monster) note("mob " .. monster.name) end)
  end
  if npc.unique_name == "Ghost#g1" then
    events.on("npc_event", function(event)
      local p = maps.find_player(event.name == "onfind" and "a" or "nobody")
      note("ghost " .. (p and p.name or "nil") .. " " .. npc.map)
    end, { name = "OnFind" })
  end
  events.on("npc_event", function(event) note(event.name .. (event.player and ("@" .. event.player.name) or "")) end, { name = "OnPing" })
  events.on("npc_event", function(event) note(event.name .. "=" .. hits:get(npc)) end, { name = "OnAll" })
end`

// npcWorldZone is a zone with the Sender, Receiver#r1 and a floating
// Ghost, and the Trap with a touch area, all running npcWorldScript.
func npcWorldZone(t *testing.T, spawns ...Spawn) (*Zone, *syncBuffer) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), spawns)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	logs := &syncBuffer{}
	z.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(npcWorldScript)}, IDs: []string{"core", "example.test"},
		Log: slog.New(slog.NewTextHandler(logs, nil)), Server: variables.NewServer(nil, nil), Temporary: variables.NewTemporary()}
	s := "example.test:scripts/npc.lua"
	z.SetNPCs([]NPC{
		{Name: "example.test:Sender", DisplayName: "Sender", X: 20, Y: 20, Sprite: 112, Script: s},
		{Name: "example.test:Receiver", DisplayName: "Receiver", X: 22, Y: 20, Sprite: 112, Script: s},
		{Name: "example.test:Trap", DisplayName: "Trap", X: 30, Y: 30, Sprite: 112, Script: s, Touch: &TouchArea{XSpan: 1, YSpan: 1}},
		{Name: "example.test:Ghost#g1", DisplayName: "Ghost", Script: s, Floating: true},
		{Name: "example.test:Wall", DisplayName: "Wall", X: 24, Y: 20, Sprite: 112, Script: s, State: "disabled"},
	})
	t.Cleanup(func() {
		z.stopScripts()
		if strings.Contains(logs.String(), "level=WARN") || strings.Contains(logs.String(), "level=ERROR") {
			t.Errorf("logs: %s", logs)
		}
	})
	return z, logs
}

// order reads and clears what the NPCs noted.
func order(t *testing.T, z *Zone) string {
	t.Helper()
	k := variables.TempKey{Mod: "example.test", Name: "order"}
	s, _ := z.Mods.Temporary.Get(k)
	if err := z.Mods.Temporary.Set(k, ""); err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(strings.Trim(s, `"`), `&`, "&")
}

func str(v script.Value) string {
	s, _ := v.AsString()
	return s
}

func npcNamed(z *Zone, name string) *entity {
	for _, e := range z.npcHandles {
		if e.npc.Name == "example.test:"+name {
			return e
		}
	}
	return nil
}

func clickNPC(t *testing.T, z *Zone, p *Player, e *entity) {
	t.Helper()
	id, ok := p.netIDs.get(e)
	if !ok {
		t.Fatalf("the player does not see %s", e.npc.Name)
	}
	z.click(p, id)
}

// TestNPCEventsOrder checks review decision 58's events between NPCs of
// one zone: they run after the step that sent them, in the order sent.
// donpcevent returns whether the NPC has the label, ::OnX reaches every
// NPC with it, and doevent runs in a conversation with the player.
func TestNPCEventsOrder(t *testing.T) {
	z, _ := npcWorldZone(t)
	a := joinAt(z, "a", 20, 21)
	z.stepAt(z.now + 50)
	if got := order(t, z); got != "Receiver:enter@a;" {
		t.Fatalf("map_enter: %q", got)
	}
	clickNPC(t, z, a, npcNamed(z, "Sender"))
	z.stepAt(z.now + 50)
	want := "Sender:fire=true;Sender:none=false;Sender:count=5;Sender:with=true;Sender:after;" +
		"Receiver:onping;Sender:onall=0;Receiver:onall=2;Trap:onall=0;Ghost#g1:onall=0;Wall:onall=0;Receiver:talk@a;"
	if got := order(t, z); got != want {
		t.Fatalf("order:\n got %q\nwant %q", got, want)
	}
	if a.talk == nil || a.talk.conv.Event != "ontalk" {
		t.Fatalf("doevent's conversation: %+v", a.talk)
	}
	// A second doevent while the player talks waits in its event queue,
	// and runs touchQueueDelay after the conversation ends.
	h := scriptHost{z}
	recv := npcNamed(z, "Receiver").npc.handle
	if ran, err := h.FireNPCEvent(recv, "ontalk", int64(a.charID)); err != nil || !ran {
		t.Fatalf("FireNPCEvent: %v %v", ran, err)
	}
	z.stepAt(z.now + 50)
	if len(a.touchQueue) != 1 {
		t.Fatalf("queue: %d", len(a.touchQueue))
	}
	ev := next(z, a, events(a))
	answer(z, a, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	if got := order(t, z); got != "Receiver:talk done;" {
		t.Fatalf("after next: %q", got)
	}
	z.stepAt(z.now + touchQueueDelay + 50)
	if got := order(t, z); got != "Receiver:talk@a;" {
		t.Fatalf("queued event: %q", got)
	}
}

// TestNPCViewState checks enablenpc and its aliases: a disabled NPC does
// not show, refuses clicks and player events, and still runs events
// without a player; a hidden one keeps its touch area; a cloak can be
// for one player.
func TestNPCViewState(t *testing.T) {
	z, _ := npcWorldZone(t)
	a := joinAt(z, "a", 20, 21)
	b := joinAt(z, "b", 21, 21)
	z.stepAt(z.now + 50)
	order(t, z)
	h := scriptHost{z}
	recv := npcNamed(z, "Receiver")
	wall := npcNamed(z, "Wall")
	if _, ok := a.netIDs.get(wall); ok {
		t.Fatal("a player sees an NPC that starts disabled")
	}
	if v, _ := h.NPCField(wall.npc.handle, "state"); str(v) != "disabled" {
		t.Fatalf("Wall's state: %v", v)
	}
	if _, ok := a.netIDs.get(recv); !ok {
		t.Fatal("a does not see the Receiver")
	}
	if err := h.ControlNPC(recv.npc.handle, modscript.NPCControl{Op: "hide"}); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if _, ok := a.netIDs.get(recv); ok {
		t.Fatal("a still sees the disabled Receiver")
	}
	if ran, _ := h.FireNPCEvent(recv.npc.handle, "ontalk", int64(a.charID)); ran {
		t.Fatal("doevent ran on a disabled NPC")
	}
	if ran, _ := h.FireNPCEvent(recv.npc.handle, "onping", 0); !ran {
		t.Fatal("donpcevent did not run on a disabled NPC")
	}
	z.stepAt(z.now + 50)
	c := joinAt(z, "c", 19, 21)
	z.stepAt(z.now + 50)
	if got := order(t, z); got != "Receiver:onping;" {
		t.Fatalf("a disabled NPC heard map_enter, or no donpcevent: %q", got)
	}
	if err := h.ControlNPC(recv.npc.handle, modscript.NPCControl{Op: "show"}); err != nil {
		t.Fatal(err)
	}
	// One player's cloak.
	if err := h.ControlNPC(recv.npc.handle, modscript.NPCControl{Op: "set_cloaked", On: true, Player: int64(b.charID)}); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	_, aSees := a.netIDs.get(recv)
	_, bSees := b.netIDs.get(recv)
	if !aSees || bSees {
		t.Fatalf("after a cloak for b: a sees %v, b sees %v", aSees, bSees)
	}
	for _, x := range []struct {
		p    int64
		want bool
	}{{0, false}, {int64(a.charID), false}, {int64(b.charID), true}} {
		if got, err := h.NPCCloaked(recv.npc.handle, x.p); err != nil || got != x.want {
			t.Errorf("is_cloaked(%d) = %v %v", x.p, got, err)
		}
	}
	// A cloak for everyone ends the single cloaks on the map
	// (npc_cloaked_sub): b's goes, and b is cloaked as the others.
	if err := h.ControlNPC(recv.npc.handle, modscript.NPCControl{Op: "set_cloaked", On: true}); err != nil {
		t.Fatal(err)
	}
	if err := h.ControlNPC(recv.npc.handle, modscript.NPCControl{Op: "set_cloaked", On: false, Player: int64(c.charID)}); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	_, aSees = a.netIDs.get(recv)
	_, cSees := c.netIDs.get(recv)
	if aSees || !cSees {
		t.Fatalf("after a cloak for all and an uncloak for c: a sees %v, c sees %v", aSees, cSees)
	}
	// Hidden: no view, no click, the touch area works.
	trap := npcNamed(z, "Trap")
	if err := h.ControlNPC(trap.npc.handle, modscript.NPCControl{Op: "set_hidden", On: true}); err != nil {
		t.Fatal(err)
	}
	if v, _ := h.NPCField(trap.npc.handle, "state"); str(v) != "hidden" {
		t.Fatalf("Trap's state: %v", v)
	}
	walkTo(t, z, a, 29, 30)
	if trap.npc.clickable() {
		t.Fatal("a hidden NPC takes clicks")
	}
	if len(a.touching) != 1 || a.touching[0] != trap {
		t.Fatalf("a hidden NPC's touch area did not work: %v", a.touching)
	}
}

// TestNPCRemoveAndFind checks unloadnpc, world.get_npc of the zone's own
// NPCs without a world, the names of strnpcinfo, and maps.find_player.
func TestNPCRemoveAndFind(t *testing.T) {
	z, _ := npcWorldZone(t)
	a := joinAt(z, "a", 20, 21)
	z.stepAt(z.now + 50)
	order(t, z)
	h := scriptHost{z}
	ghost, ok := h.FindNPC("Ghost#g1", "example.test")
	if !ok {
		t.Fatal("the floating NPC is not found")
	}
	for field, want := range map[string]string{"name": "Ghost", "full_name": "Ghost#g1", "hidden_name": "g1", "unique_name": "Ghost#g1", "map": "", "state": "shown"} {
		if v, err := h.NPCField(ghost, field); err != nil || str(v) != want {
			t.Errorf("%s = %v %v, want %s", field, v, err, want)
		}
	}
	if _, ok := h.FindNPC("example.test:Ghost#g1", "core"); !ok {
		t.Fatal("a full name is not found")
	}
	if _, ok := h.FindNPC("Ghost#g1", "core"); ok {
		t.Fatal("a plain name found another mod's NPC")
	}
	if _, ok := h.FindPlayer("a"); !ok {
		t.Fatal("find_player")
	}
	if got, ok := h.FindPlayerByID(a.record.AccountID); a.record.AccountID != 0 && (!ok || got != int64(a.charID)) {
		t.Fatal("find_player_by_id of the account")
	}
	if _, err := h.FireNPCEvent(ghost, "onfind", 0); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if got := order(t, z); got != "Ghost#g1:ghost a ;" {
		t.Fatalf("floating NPC: %q", got)
	}
	recv := npcNamed(z, "Receiver")
	if err := z.Mods.Temporary.Set(variables.TempKey{Mod: "example.test", NPC: "example.test:Receiver", Name: "hits"}, "5"); err != nil {
		t.Fatal(err)
	}
	if err := h.ControlNPC(recv.npc.handle, modscript.NPCControl{Op: "remove"}); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if _, ok := h.FindNPC("Receiver", "example.test"); ok {
		t.Fatal("a removed NPC is found")
	}
	if _, ok := a.netIDs.get(recv); ok || z.entities.has(recv) {
		t.Fatal("a removed NPC stays on the map")
	}
	if recv.npc.script != nil {
		t.Fatal("a removed NPC's context did not end")
	}
	if _, ok := z.Mods.Temporary.Get(variables.TempKey{Mod: "example.test", NPC: "example.test:Receiver", Name: "hits"}); ok {
		t.Fatal("a removed NPC's variables stayed")
	}
	if _, err := h.NPCField(recv.npc.handle, "name"); err == nil {
		t.Fatal("a removed NPC's handle reads")
	}
}

// TestMonsterTouch checks OnTouchNPC: a monster that walks into the
// Trap's area runs monster_touch once, until it leaves, and not while
// the Trap is cloaked.
func TestMonsterTouch(t *testing.T) {
	mob := &battle.Status{MaxHP: 30, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Mode: 0x81, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	poring := Spawn{Mob: 1002, Name: "Poring", Amount: 1, X: 34, Y: 30, Speed: 100, ViewRange: 10, ChaseRange: 12, Status: mob, RespawnMs: 60000}
	z, _ := npcWorldZone(t, poring)
	joinAt(z, "a", 5, 5)
	z.stepAt(z.now + 50)
	order(t, z)
	var m *entity
	for e := range z.entities.all() {
		if e.monster != nil {
			m = e
		}
	}
	// No random walk: the test walks it.
	m.monster.nextWalk = 1 << 60
	walk := func(x, y int) {
		m.x, m.y, m.path = x+1, y, nil
		z.walk(m, x, y)
		for range 40 {
			z.stepAt(z.now + 50)
		}
	}
	walk(31, 30)
	walk(30, 30)
	if got := order(t, z); got != "Trap:mob Poring;" {
		t.Fatalf("monster_touch: %q", got)
	}
	walk(34, 30)
	h := scriptHost{z}
	if err := h.ControlNPC(npcNamed(z, "Trap").npc.handle, modscript.NPCControl{Op: "set_cloaked", On: true}); err != nil {
		t.Fatal(err)
	}
	walk(31, 30)
	if got := order(t, z); got != "" {
		t.Fatalf("a cloaked NPC heard a monster: %q", got)
	}
}

// TestNPCControlPermission checks that another mod's NPC needs
// npcs.control for changes and events, and that ::OnX reaches only the
// mod's own NPCs without it (review decision 58).
func TestNPCControlPermission(t *testing.T) {
	z, logs := npcWorldZone(t)
	other := &mod.Scripts{Mod: "example.other", Files: map[string]string{"scripts/npc.lua": `
local context = require "wa.context"
local events = require "wa.events"
local world = require "wa.world"
local variables = require "wa.variables"
local result = variables.server_temporary_string("result")
return function()
  local npc = context:get_instance()
  events.on("npc_event", function() end, { name = "OnAll" })
  events.on("npc_event", function()
    local r = world.get_npc("example.test:Receiver")
    local out = {}
    for _, f in ipairs({ function() r:hide() end, function() r:fire_event("OnPing") end, function() r:set_cloaked(true) end }) do
      local ok, err = pcall(f)
      out[#out + 1] = ok and "ok" or (string.find(err, "npcs.control", 1, true) and "refused" or err)
    end
    out[#out + 1] = tostring(r:is_cloaked()) .. " " .. r.state
    out[#out + 1] = tostring(world.fire_npc_event("OnAll"))
    result:set(table.concat(out, " "))
  end, { name = "OnTry" })
end`}, Grants: mod.Grants{"game.read": true, "event.listen": true, "storage.mod": true}, Visible: []string{"example.other"}}
	z.Mods.Scripts = append(z.Mods.Scripts, other)
	z.Mods.IDs = append(z.Mods.IDs, "example.other")
	npcs := []NPC{}
	for _, e := range z.npcsInOrder() {
		npcs = append(npcs, e.npc.NPC)
	}
	npcs = append(npcs, NPC{Name: "example.other:Stranger", DisplayName: "Stranger", X: 10, Y: 10, Sprite: 112, Script: "example.other:scripts/npc.lua"})
	z.SetNPCs(npcs)
	joinAt(z, "a", 20, 21)
	z.stepAt(z.now + 50)
	order(t, z)
	h := scriptHost{z}
	id, ok := h.FindNPC("example.other:Stranger", "core")
	if !ok {
		t.Fatal("no Stranger")
	}
	if _, err := h.FireNPCEvent(id, "ontry", 0); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	got, _ := z.Mods.Temporary.Get(variables.TempKey{Mod: "example.other", Name: "result"})
	if got != `"refused refused refused false shown 1"` {
		t.Fatalf("result: %s; logs: %s", got, logs)
	}
	// The broadcast of the Stranger's mod reached no example.test NPC.
	if got := order(t, z); got != "" {
		t.Fatalf("another mod's broadcast reached: %q", got)
	}
}
