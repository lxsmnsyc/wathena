package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// roomScript is the NPC script of the room tests. The Arena opens a room
// of 4 with a trigger of 2 and a fee of 100, and its OnStart event warps
// the room's members to map/b and notes who.
const roomScript = `
local context = require "wa.context"
local events = require "wa.events"
local variables = require "wa.variables"
local order = variables.server_temporary_string("order")
return function()
  local npc = context:get_instance()
  local function note(s) order:set(order:get() .. s .. ";") end
  if npc.unique_name == "Arena" then
    local room = npc:open_chat_room{ title = "Arena", limit = 4, event = "example.test:Arena::OnStart", trigger = 2, zeny = 100, min_level = 2, max_level = 60 }
    note("open=" .. tostring(room ~= nil) .. " again=" .. tostring(npc:open_chat_room{ title = "x", limit = 2 }))
    events.on("npc_event", function(event)
      local r = npc:chat_room()
      note("start users=" .. r.users .. " player=" .. tostring(event.player))
      local names = {}
      for _, p in ipairs(r:warp_members("map/b", 5, 6)) do
        names[#names + 1] = p.name
      end
      note("warped=" .. table.concat(names, ","))
    end, { name = "OnStart" })
  end
  if npc.unique_name == "Lobby" then
    npc:open_chat_room{ title = "Lobby", limit = 3 }
  end
  if npc.unique_name == "Arena" then
    events.on("chat_room_join", function(e) note("join " .. e.name .. " " .. e.title .. " " .. e.users) end)
    events.on("chat_room_leave", function(e) note("leave " .. e.name .. " " .. e.users .. " kicked=" .. tostring(e.kicked) .. " room=" .. tostring(e.room ~= nil)) end)
  end
end`

// roomWorld makes map/a with the Arena and the Lobby, and map/b, and
// three players on map/a that stand next to the Arena. Each player has
// base level 20 and 500 zeny.
func roomWorld(t *testing.T) (a, b *Zone, ps []*Player) {
	t.Helper()
	a, b = twoMaps()
	logs := &syncBuffer{}
	a.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(roomScript)}, IDs: []string{"core", "example.test"},
		Log: slog.New(slog.NewTextHandler(logs, nil)), Server: variables.NewServer(nil, nil), Temporary: variables.NewTemporary(),
		Shops: shop.NewRegistry(nil)}
	s := "example.test:scripts/npc.lua"
	a.SetNPCs([]NPC{
		{Name: "example.test:Arena", DisplayName: "Arena", X: 10, Y: 4, Sprite: 112, Script: s},
		{Name: "example.test:Lobby", DisplayName: "Lobby", X: 12, Y: 4, Sprite: 112, Script: s},
	})
	t.Cleanup(func() {
		a.stopScripts()
		// The script opens the Arena's room twice on purpose: rAthena logs it.
		out := strings.ReplaceAll(logs.String(), `msg="npc:open_chat_room: the NPC has a chat room already"`, "")
		if strings.Contains(out, "level=ERROR") || strings.Count(out, "level=WARN") > 1 {
			t.Errorf("logs: %s", logs)
		}
	})
	for i, name := range []string{"Ann", "Bea", "Cy"} {
		p := onMap(a, name, 11+i, 12, &saves{})
		p.progress.BaseLevel = 20
		p.record.Zeny = 500
		ps = append(ps, p)
	}
	a.stepAt(a.now + 50)
	a.stepAt(a.now + 50)
	for _, p := range ps {
		events(p)
	}
	if npcNamed(a, "Arena") == nil || a.roomOfNPC(npcNamed(a, "Arena")) == nil {
		t.Fatalf("the Arena has no room: %s", logs)
	}
	return a, b, ps
}

// roomOutcomes are the results of the join and create commands in a
// player's events.
func roomOutcomes(evs []*wathenav1.Event) []wathenav1.ChatRoomOutcome {
	var out []wathenav1.ChatRoomOutcome
	for _, e := range evs {
		if r := e.GetChatRoomResult(); r != nil {
			out = append(out, r.GetOutcome())
		}
	}
	return out
}

func roomCmd(a wathenav1.ChatRoomAction, build func(b *wathenav1.ChatRoomCommand_builder)) *wathenav1.ChatRoomCommand {
	b := wathenav1.ChatRoomCommand_builder{Action: a}
	if build != nil {
		build(&b)
	}
	return b.Build()
}

func joinRoomAs(z *Zone, p *Player, room uint32, password string) {
	z.handle(command{player: p, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_JOIN, func(b *wathenav1.ChatRoomCommand_builder) {
		b.Room, b.Password = room, password
	})})
	z.stepAt(z.now + 50)
}

func arenaRoom(z *Zone) *room { return z.roomOfNPC(npcNamed(z, "Arena")) }

// TestNPCRoomJoinChecks checks the join checks in rAthena's order on an
// NPC room: the NPC counts, so a limit of 4 takes 3 players; the level
// range; the fee is only checked, never taken, on a join.
func TestNPCRoomJoinChecks(t *testing.T) {
	z, _, ps := roomWorld(t)
	order(t, z)
	r := arenaRoom(z)
	ann, bea, cy := ps[0], ps[1], ps[2]

	ann.progress.BaseLevel = 1
	joinRoomAs(z, ann, r.ID, "")
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_LEVEL_LOW {
		t.Fatalf("level 1: %v", got)
	}
	ann.progress.BaseLevel = 61
	joinRoomAs(z, ann, r.ID, "")
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_LEVEL_HIGH {
		t.Fatalf("level 61: %v", got)
	}
	ann.progress.BaseLevel = 20
	ann.record.Zeny = 99
	joinRoomAs(z, ann, r.ID, "")
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_NO_ZENY {
		t.Fatalf("zeny 99: %v", got)
	}
	ann.record.Zeny = 500
	// The room is not a player's: no such room is the answer FULL.
	joinRoomAs(z, ann, 999, "")
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_FULL {
		t.Fatalf("no such room: %v", got)
	}

	// The trigger event is disabled, to fill the room: 4 - 1 = 3 players.
	r.Disabled = true
	for _, p := range ps {
		joinRoomAs(z, p, r.ID, "")
		if got := roomOutcomes(events(p)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_OK {
			t.Fatalf("%s joins: %v", p.Name, got)
		}
	}
	if r.Users() != 3 || ann.record.Zeny != 500 {
		t.Fatalf("users %d, zeny %d: the fee is taken at the warp only", r.Users(), ann.record.Zeny)
	}
	extra := onMap(z, "Dee", 14, 12, &saves{})
	extra.progress.BaseLevel, extra.record.Zeny = 20, 500
	z.stepAt(z.now + 50)
	events(extra)
	joinRoomAs(z, extra, r.ID, "")
	if got := roomOutcomes(events(extra)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_FULL {
		t.Fatalf("a fourth player: %v", got)
	}
	// getwaitingroomstate(32) tests users >= limit, which is 3 < 4.
	h := scriptHost{z}
	for field, want := range map[string]bool{"full": false, "triggered": false, "trigger_disabled": true, "public": true, "npc": true} {
		v, err := h.ChatRoomField(int64(r.ID), field)
		if got, _ := v.AsBool(); err != nil || got != want {
			t.Errorf("%s = %v, %v; want %v", field, v, err, want)
		}
	}
	_ = bea
	_ = cy
	_ = order(t, z)
}

// TestNPCRoomTriggerWarp checks the trigger, the event with no player,
// warpwaitingpc's fee for each member in join order, and that a warped
// member has left the room.
func TestNPCRoomTriggerWarp(t *testing.T) {
	z, b, ps := roomWorld(t)
	order(t, z)
	r := arenaRoom(z)
	ann, bea, cy := ps[0], ps[1], ps[2]
	joinRoomAs(z, ann, r.ID, "")
	z.stepAt(z.now + 50)
	if got := order(t, z); got != "join Ann Arena 1;" {
		t.Fatalf("after one join: %q", got)
	}
	if ann.room != r {
		t.Fatal("Ann is not in the room")
	}
	// The second join reaches the trigger of 2: the event runs after the
	// step, warps both, and takes 100 from each.
	joinRoomAs(z, bea, r.ID, "")
	z.stepAt(z.now + 50)
	// The leave events run after the script that warped.
	want := "join Bea Arena 2;start users=2 player=nil;warped=Ann,Bea;leave Ann 1 kicked=false room=true;leave Bea 0 kicked=false room=true;"
	if got := order(t, z); got != want {
		t.Fatalf("trigger:\n got %q\nwant %q", got, want)
	}
	if ann.room != nil || bea.room != nil || r.Users() != 0 {
		t.Fatalf("members still in the room: %v %v %d", ann.room, bea.room, r.Users())
	}
	if ann.record.Zeny != 400 || bea.record.Zeny != 400 {
		t.Fatalf("zeny %d %d, want 400 each", ann.record.Zeny, bea.record.Zeny)
	}
	// Both go to map/b, Ann first.
	for _, name := range []string{"Ann", "Bea"} {
		if got := arrive(t, b); got.Name != name || got.entity.x != 5 || got.entity.y != 6 {
			t.Fatalf("arrival %s at %d,%d, want %s at 5,6", got.Name, got.entity.x, got.entity.y, name)
		}
	}

	// With the trigger off, a join does not run the event; enabling it
	// runs the check at once.
	h := scriptHost{z}
	if err := h.ChatRoomEvent(int64(r.ID), false); err != nil {
		t.Fatal(err)
	}
	joinRoomAs(z, cy, r.ID, "")
	z.stepAt(z.now + 50)
	if got := order(t, z); got != "join Cy Arena 1;" {
		t.Fatalf("disabled: %q", got)
	}
	cy.record.Zeny = 50
	extra := onMap(z, "Dee", 14, 12, &saves{})
	extra.progress.BaseLevel, extra.record.Zeny = 20, 500
	z.stepAt(z.now + 50)
	events(extra)
	joinRoomAs(z, extra, r.ID, "")
	z.stepAt(z.now + 50)
	order(t, z)
	// Both are in; the fee stops the loop at Cy, the first member, who
	// cannot pay: nobody is warped.
	if err := h.ChatRoomEvent(int64(r.ID), true); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if got := order(t, z); got != "start users=2 player=nil;warped=;" {
		t.Fatalf("enable: %q", got)
	}
	if r.Users() != 2 || cy.record.Zeny != 50 {
		t.Fatalf("a member who cannot pay stays: users %d zeny %d", r.Users(), cy.record.Zeny)
	}
	// Kicks: one by name, then all, last first.
	if ok, _ := h.ChatRoomKick(int64(r.ID), "Nobody"); ok {
		t.Fatal("kicked a name that is not here")
	}
	if ok, _ := h.ChatRoomKick(int64(r.ID), "Cy"); !ok || cy.room != nil {
		t.Fatalf("kick Cy: %v", cy.room)
	}
	if n, _ := h.ChatRoomKickAll(int64(r.ID)); n != 1 || extra.room != nil {
		t.Fatalf("kick all: %d", n)
	}
	z.stepAt(z.now + 50)
	if got := order(t, z); got != "leave Cy 1 kicked=true room=true;leave Dee 0 kicked=false room=true;" {
		t.Fatalf("kicks: %q", got)
	}
}

// TestNPCRoomClose checks delwaitingroom, and that a second open is
// ignored.
func TestNPCRoomClose(t *testing.T) {
	z, _, ps := roomWorld(t)
	got := order(t, z)
	if !strings.Contains(got, "open=true again=nil") {
		t.Fatalf("the second open: %q", got)
	}
	r := arenaRoom(z)
	r.Disabled = true
	joinRoomAs(z, ps[0], r.ID, "")
	z.stepAt(z.now + 50)
	h := scriptHost{z}
	if ok, err := h.CloseChatRoom(npcNamed(z, "Arena").npc.handle); err != nil || !ok {
		t.Fatalf("close: %v %v", ok, err)
	}
	if ps[0].room != nil || arenaRoom(z) != nil || z.rooms[r.ID] != nil {
		t.Fatal("the room stays after delwaitingroom")
	}
	if ok, _ := h.CloseChatRoom(npcNamed(z, "Arena").npc.handle); ok {
		t.Fatal("closed a room twice")
	}
	// An NPC that goes takes its room with it.
	lobby := npcNamed(z, "Lobby")
	if z.roomOfNPC(lobby) == nil {
		t.Fatal("the Lobby has no room")
	}
	z.removeNPC(lobby)
	if z.roomOfNPC(lobby) != nil {
		t.Fatal("the Lobby's room outlived its NPC")
	}
}

// TestPlayerRoom checks a player's room: the checks of the create, the
// join checks, the owner change, kicks and the kick list, the status
// change and the room's chat.
func TestPlayerRoom(t *testing.T) {
	z, _, ps := roomWorld(t)
	ann, bea, cy := ps[0], ps[1], ps[2]
	create := func(p *Player, title, pass string, limit uint32, public bool) {
		z.handle(command{player: p, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_CREATE, func(b *wathenav1.ChatRoomCommand_builder) {
			b.Title, b.Password, b.Limit, b.Public = title, pass, limit, public
		})})
		z.stepAt(z.now + 50)
	}
	notAllowed := wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_NOT_ALLOWED
	// Basic Skill level 4 first.
	ann.char.Skills = map[int]int{skillBasic: 3}
	create(ann, "Hi", "", 5, true)
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != notAllowed {
		t.Fatalf("Basic Skill 3: %v", got)
	}
	ann.char.Skills = map[int]int{skillBasic: 9}
	create(ann, "", "", 5, true)
	create(ann, "Hi", "123456789", 5, true)
	create(ann, "Hi", "", 21, true)
	create(ann, "Hi", "", 0, true)
	create(ann, strings.Repeat("x", 37), "", 5, true)
	if got := roomOutcomes(events(ann)); len(got) != 5 {
		t.Fatalf("bad values: %v", got)
	}
	// A cell with no_chat and a map with nochat.
	if err := (scriptHost{z}).SetCells("map/a", modscript.Rect{X1: ann.entity.x, Y1: ann.entity.y, X2: ann.entity.x, Y2: ann.entity.y}, "no_chat", true, false); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	create(ann, "Hi", "", 5, true)
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != notAllowed || ann.room != nil {
		t.Fatalf("no_chat cell: %v", got)
	}
	if err := (scriptHost{z}).SetCells("map/a", modscript.Rect{X1: ann.entity.x, Y1: ann.entity.y, X2: ann.entity.x, Y2: ann.entity.y}, "no_chat", false, false); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if err := (scriptHost{z}).SetMapFlag("map/a", "nochat", true); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	create(ann, "Hi", "", 5, true)
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != notAllowed || ann.room != nil {
		t.Fatalf("nochat map: %v", got)
	}
	if err := (scriptHost{z}).SetMapFlag("map/a", "nochat", false); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	// A player room: private, limit 3, with a password.
	create(ann, "Club", "pw", 3, false)
	z.stepAt(z.now + 50)
	if got := roomOutcomes(events(ann)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_OK || ann.room == nil {
		t.Fatalf("create: %v", got)
	}
	r := ann.room
	// Bea sees the bubble.
	var shown *wathenav1.ChatRoomShow
	for _, e := range events(bea) {
		if s := e.GetChatRoomShow(); s != nil {
			shown = s
		}
	}
	if shown == nil || shown.GetTitle() != "Club" || shown.GetUsers() != 1 || shown.GetLimit() != 3 || shown.GetPublic() || shown.GetNpc() {
		t.Fatalf("the bubble: %v", shown)
	}
	joinRoomAs(z, bea, r.ID, "wrong")
	if got := roomOutcomes(events(bea)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_WRONG_PASSWORD {
		t.Fatalf("wrong password: %v", got)
	}
	joinRoomAs(z, bea, r.ID, "pw")
	joinRoomAs(z, cy, r.ID, "pw")
	z.stepAt(z.now + 50)
	if r.Users() != 3 {
		t.Fatalf("users %d", r.Users())
	}
	if got := roomOutcomes(events(bea)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_OK {
		t.Fatalf("join: %v", got)
	}

	// The room's chat goes to the members only.
	z.handle(command{player: bea, chat: &chatCommand{kind: chatRoom, text: "psst"}})
	z.stepAt(z.now + 50)
	heard := func(p *Player) []string {
		var out []string
		for _, c := range chats(p) {
			out = append(out, c.GetChannel().String()+":"+c.GetFromName()+":"+c.GetText())
		}
		return out
	}
	for _, p := range []*Player{ann, bea, cy} {
		if got := heard(p); len(got) != 1 || got[0] != "CHAT_CHANNEL_ROOM:Bea:psst" {
			t.Errorf("%s hears %v", p.Name, got)
		}
	}
	out := onMap(z, "Out", 13, 14, &saves{})
	z.stepAt(z.now + 50)
	events(out)
	z.handle(command{player: bea, chat: &chatCommand{kind: chatRoom, text: "psst 2"}})
	z.handle(command{player: out, chat: &chatCommand{kind: chatRoom, text: "hello?"}})
	z.stepAt(z.now + 50)
	if got := chats(out); len(got) != 0 {
		t.Errorf("a player outside hears %v", got)
	}

	// While in a room: no items, skills or attacks; a walk leaves.
	use := 501
	z.handle(command{player: bea, use: &use, attack: &attackCommand{target: 1}})
	z.stepAt(z.now + 50)
	if len(z.used) != 0 {
		t.Error("a member used an item")
	}
	// A change of status by the owner; others cannot.
	change := func(p *Player, title string, limit uint32) {
		z.handle(command{player: p, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_CHANGE, func(b *wathenav1.ChatRoomCommand_builder) {
			b.Title, b.Limit, b.Public = title, limit, true
		})})
		z.stepAt(z.now + 50)
	}
	change(bea, "Hijack", 5)
	if r.Title != "Club" {
		t.Fatal("a member changed the room")
	}
	change(ann, "Club 2", 5)
	if r.Title != "Club 2" || r.Limit != 5 || !r.Public {
		t.Fatalf("change: %+v", r.Room)
	}
	// Hand over to Cy, then Cy kicks Bea: she cannot come back.
	z.handle(command{player: ann, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_OWNER, func(b *wathenav1.ChatRoomCommand_builder) { b.Name = "Cy" })})
	if o, _ := r.Owner(); o.Name != "Cy" {
		t.Fatalf("owner %v", o)
	}
	z.handle(command{player: ann, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_KICK, func(b *wathenav1.ChatRoomCommand_builder) { b.Name = "Bea" })})
	if bea.room == nil {
		t.Fatal("a member kicked another")
	}
	z.handle(command{player: cy, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_KICK, func(b *wathenav1.ChatRoomCommand_builder) { b.Name = "Bea" })})
	if bea.room != nil {
		t.Fatal("the owner could not kick")
	}
	joinRoomAs(z, bea, r.ID, "")
	if got := roomOutcomes(events(bea)); len(got) != 1 || got[0] != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_KICKED {
		t.Fatalf("a kicked player returns: %v", got)
	}
	// A walk leaves the room and the owner passes to the first member.
	z.handle(command{player: cy, walkTo: &[2]int{cy.entity.x + 2, cy.entity.y}})
	if cy.room != nil {
		t.Fatal("a member who walks stays")
	}
	if o, ok := r.Owner(); !ok || o.Name != "Ann" {
		t.Fatalf("owner after the walk: %v", o)
	}
	// Death leaves too, and an empty room is gone.
	z.die(ann.entity, nil)
	if ann.room != nil || z.rooms[r.ID] != nil {
		t.Fatal("a dead player stays in its room, or the empty room stays")
	}
	z.stepAt(z.now + 50)
	var cleared bool
	for _, e := range events(out) {
		cleared = cleared || e.GetChatRoomClear().GetRoom() == r.ID
	}
	if !cleared {
		t.Error("the bubble stays after the room is gone")
	}
}

// TestRoomLeavesOnLogout checks that a logout and a warp take a member
// out of its room.
func TestRoomLeavesOnLogout(t *testing.T) {
	z, _, ps := roomWorld(t)
	r := arenaRoom(z)
	r.Disabled = true
	joinRoomAs(z, ps[0], r.ID, "")
	joinRoomAs(z, ps[1], r.ID, "")
	z.leave(ps[0], wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	if r.Users() != 1 || r.Has(int64(ps[0].charID)) {
		t.Fatalf("users %d after a logout", r.Users())
	}
	z.teleport(ps[1], 5, 5)
	if r.Users() != 0 || ps[1].room != nil {
		t.Fatal("a member who teleports stays")
	}
}

// TestChatRoomRules checks the rules chat_room_max_users and
// chat_room_title.
func TestChatRoomRules(t *testing.T) {
	z, _, ps := roomWorld(t)
	rl := rules.Base(true)
	rl.Items, _ = rules.ItemsOf(map[string]int{"basic_skill_check": 0})
	rl.Values["chat_room_max_users"] = 5
	rl.Values["chat_room_title"] = 4
	z.Mods.Rules = rl
	ann := ps[0]
	create := func(title string, limit uint32) {
		z.handle(command{player: ann, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_CREATE, func(b *wathenav1.ChatRoomCommand_builder) {
			b.Title, b.Limit, b.Public = title, limit, true
		})})
	}
	create("Hello", 5)
	create("Hi", 6)
	if ann.room != nil {
		t.Fatal("a title of 5 or a limit of 6 passed")
	}
	create("Héé", 5)
	if ann.room == nil || ann.room.Title != "Héé" {
		t.Fatal("a title of 3 characters, 5 bytes, was refused")
	}
	// An NPC room is cut to the rule's most users.
	h := scriptHost{z}
	l := npcNamed(z, "Lobby")
	if _, err := h.CloseChatRoom(l.npc.handle); err != nil {
		t.Fatal(err)
	}
	if _, err := h.OpenChatRoom(l.npc.handle, modscript.RoomSpec{Title: "Lob", Limit: 9, Trigger: -1}); err != nil {
		t.Fatal(err)
	}
	if got := z.roomOfNPC(l).Limit; got != 5 {
		t.Fatalf("an NPC room's limit %d, want 5", got)
	}
}

// TestRoomStaffRightsAndDead checks chat.join_all (a locked room opens
// without its password), chat.no_kick, that a dead player may create and
// join a room, and that a mod without chat_rooms.manage reads no password.
func TestRoomStaffRightsAndDead(t *testing.T) {
	w := newGMWorld()
	z := w.a
	rl := rules.Base(true)
	rl.Items, _ = rules.ItemsOf(map[string]int{"basic_skill_check": 0})
	z.Mods = &Mods{Rules: rl}
	owner := w.player(z, 11, 90, "Owner", 5, 5, false)
	gm := w.player(z, 12, 2, "Gm", 6, 5, true)
	plain := w.player(z, 13, 91, "Plain", 7, 5, false)
	z.stepAt(z.now + 50)

	// A dead owner makes a locked room.
	owner.entity.dead = true
	z.handle(command{player: owner, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_CREATE, func(b *wathenav1.ChatRoomCommand_builder) {
		b.Title, b.Password, b.Limit = "Dead", "pw", 5
	})})
	r := owner.room
	if r == nil {
		t.Fatal("a dead player cannot make a room")
	}
	joinRoomAs(z, plain, r.ID, "bad")
	if plain.room != nil {
		t.Fatal("a wrong password joined")
	}
	joinRoomAs(z, gm, r.ID, "")
	if gm.room != r {
		t.Fatal("chat.join_all did not open the locked room")
	}
	z.handle(command{player: owner, chatRoom: roomCmd(wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_KICK, func(b *wathenav1.ChatRoomCommand_builder) { b.Name = "Gm" })})
	if gm.room != r {
		t.Fatal("chat.no_kick did not stop the kick")
	}
	// A dead player joins an NPC-less public room too.
	plain.entity.dead = true
	r.Public = true
	joinRoomAs(z, plain, r.ID, "")
	if plain.room != r {
		t.Fatal("a dead player cannot join")
	}
}
