package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// npcMod is a mod whose NPC script is scripts/npc.lua, with every
// permission.
func npcMod(src string) *mod.Scripts {
	sc := &mod.Scripts{Mod: "example.test", Files: map[string]string{"scripts/npc.lua": src}, Grants: mod.Grants{}, Visible: []string{"example.test"}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	return sc
}

// npcZone is a zone with one scripted NPC at 20,20 and a player next to
// it. The player's first frame is already read.
func npcZone(t *testing.T, src string) (*Zone, *Player, *entity, *syncBuffer) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	logs := &syncBuffer{}
	ids := map[string]uint32{"Red_Potion": 501, "Jellopy": 909, "Knife": 1201, "Untradable": 7777}
	z.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(src)}, IDs: []string{"core", "example.test"}, ItemIDs: ids,
		Log: slog.New(slog.NewTextHandler(logs, nil)), Server: variables.NewServer(nil, nil), Shops: shop.NewRegistry(nil)}
	z.SetNPCs([]NPC{{Name: "example.test:Guide", DisplayName: "Guide", X: 20, Y: 20, Sprite: 112, Script: "example.test:scripts/npc.lua"}})
	t.Cleanup(z.stopScripts)
	p := add(z, "a")
	p.entity.x, p.entity.y = 21, 20
	z.stepAt(50)
	events(p)
	var n *entity
	for e := range z.entities.all() {
		if e.npc != nil {
			n = e
		}
	}
	if n.npc.script == nil {
		t.Fatalf("the NPC did not start: %s", logs)
	}
	return z, p, n, logs
}

// click talks to an NPC and returns the events of the next frame.
func click(z *Zone, p *Player, n *entity) []*wathenav1.Event {
	id, _ := p.netIDs.get(n)
	z.handle(command{player: p, npcClick: &id})
	z.stepAt(z.now + 50)
	return events(p)
}

// answer replies to the player's waiting step and returns the events of
// the next frame.
func answer(z *Zone, p *Player, r *wathenav1.NpcReply) []*wathenav1.Event {
	z.handle(command{player: p, npcReply: r})
	z.stepAt(z.now + 50)
	return events(p)
}

func reply(dialog uint32, build func(b *wathenav1.NpcReply_builder)) *wathenav1.NpcReply {
	b := wathenav1.NpcReply_builder{Dialog: dialog}
	build(&b)
	return b.Build()
}

// dialogs are the NPC dialog steps in events.
func dialogs(ev []*wathenav1.Event) []*wathenav1.NpcDialog {
	var out []*wathenav1.NpcDialog
	for _, e := range ev {
		if d := e.GetNpcDialog(); d != nil {
			out = append(out, d)
		}
	}
	return out
}

// lastDialog is the last dialog step that waits, or nil.
func lastDialog(ev []*wathenav1.Event) *wathenav1.NpcDialog {
	var last *wathenav1.NpcDialog
	for _, d := range dialogs(ev) {
		if d.WhichPrompt() != wathenav1.NpcDialog_Prompt_not_set_case {
			last = d
		}
	}
	return last
}

func lines(ev []*wathenav1.Event) string {
	var out []string
	for _, d := range dialogs(ev) {
		out = append(out, d.GetLines()...)
	}
	return strings.Join(out, "|")
}

const chooser = `
local events = require "wa.events"
return function()
  events.on("click", function(player, conversation)
    conversation:say("[Guide]", "Where to?")
    local n = conversation:choose("North", "", "South")
    conversation:say("You chose " .. n)
    conversation:wait_for_next()
    conversation:say("Bye.")
  end)
end`

func TestConversationChooseAndEnd(t *testing.T) {
	z, p, n, logs := npcZone(t, chooser)
	ev := click(z, p, n)
	if got := lines(ev); got != "[Guide]|Where to?" {
		t.Fatalf("lines %q, logs %s", got, logs)
	}
	menu := lastDialog(ev)
	if menu == nil || strings.Join(menu.GetMenu().GetOptions(), ",") != "North,,South" {
		t.Fatalf("menu %v", menu)
	}
	// A wrong dialog ID, an option out of range, a hidden option and a
	// next are dropped.
	for _, r := range []*wathenav1.NpcReply{
		reply(menu.GetDialog()+1, func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(1)) }),
		reply(menu.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(4)) }),
		reply(menu.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(2)) }),
		reply(menu.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(0)) }),
		reply(menu.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Next = new(true) }),
	} {
		if ev := answer(z, p, r); len(dialogs(ev)) != 0 {
			t.Fatalf("an answer that does not fit went through: %v", ev)
		}
	}
	ev = answer(z, p, reply(menu.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(3)) }))
	next := lastDialog(ev)
	if got := lines(ev); got != "You chose 3" || next == nil || !next.HasNext() {
		t.Fatalf("after the choice: %q %v", got, next)
	}
	ev = answer(z, p, reply(next.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Next = new(true) }))
	// The script ended with a line on show: the Close button stays, and
	// the player still talks until it presses it.
	closer := lastDialog(ev)
	if got := lines(ev); got != "Bye." || closer == nil || !closer.HasClose() || p.talk == nil {
		t.Fatalf("at the end: %q %v talk %v", got, closer, p.talk)
	}
	answer(z, p, reply(closer.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	if p.talk != nil {
		t.Fatal("the conversation did not end")
	}
	if z.scripts.Conversations() != 0 {
		t.Errorf("%d conversations left", z.scripts.Conversations())
	}
}

func TestConversationInputs(t *testing.T) {
	z, p, n, logs := npcZone(t, `
local events = require "wa.events"
return function()
  events.on("click", function(player, conversation)
    local amount = conversation:ask_number(1, 10)
    local name = conversation:ask_text(5)
    conversation:say(amount .. " " .. name)
  end)
end`)
	ev := click(z, p, n)
	num := lastDialog(ev)
	if num == nil || !num.HasInputNumber() || num.GetNumberMin() != 1 || num.GetNumberMax() != 10 {
		t.Fatalf("number prompt %v, logs %s", num, logs)
	}
	for _, v := range []int64{0, 11} {
		if ev := answer(z, p, reply(num.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.InputNumber = new(v) })); len(dialogs(ev)) != 0 {
			t.Fatalf("%d out of range went through", v)
		}
	}
	ev = answer(z, p, reply(num.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.InputNumber = new(int64(7)) }))
	text := lastDialog(ev)
	if text == nil || !text.HasInputText() || text.GetTextMax() != 5 {
		t.Fatalf("text prompt %v", text)
	}
	if ev := answer(z, p, reply(text.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.InputText = new("abcdef") })); len(dialogs(ev)) != 0 {
		t.Fatal("text over the length went through")
	}
	ev = answer(z, p, reply(text.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.InputText = new("héllo") }))
	if got := lines(ev); got != "7 héllo" {
		t.Fatalf("got %q", got)
	}
}

func TestConversationCloseGoesOn(t *testing.T) {
	z, p, n, logs := npcZone(t, `
local events = require "wa.events"
return function()
  events.on("click", function(player, conversation)
    conversation:say("Wait.")
    conversation:close()
    player:send_message("after close")
    local ok, err = pcall(conversation.say, conversation, "more")
    player:send_message(tostring(ok) .. " " .. tostring(err):match("the conversation .*"))
  end)
end`)
	ev := click(z, p, n)
	closer := lastDialog(ev)
	if closer == nil || !closer.HasClose() {
		t.Fatalf("close prompt %v, logs %s", closer, logs)
	}
	ev = answer(z, p, reply(closer.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	if got := strings.Join(systemLines(ev), "|"); got != "after close|false the conversation was closed" {
		t.Fatalf("got %q, logs %s", got, logs)
	}
	if p.talk != nil {
		t.Error("the player still talks")
	}
}

const waiter = `
local events = require "wa.events"
return function()
  local npc = require("wa.context"):get_instance()
  events.on("click", function(player, conversation)
    conversation:say("Hold on.")
    local ok, err = pcall(conversation.show_progress, conversation, 3)
    npc:say_aloud("progress " .. tostring(ok) .. " " .. tostring(err))
  end)
end`

func TestWalkDuringConversation(t *testing.T) {
	z, p, n, logs := npcZone(t, chooser)
	click(z, p, n)
	x, y := p.entity.x, p.entity.y
	// pc_cant_act: a walk does nothing while the player talks.
	z.handle(command{player: p, walkTo: &[2]int{5, 5}})
	z.stepAt(z.now + 1000)
	if p.entity.x != x || p.entity.y != y || p.talk == nil {
		t.Fatalf("the player walked to %d,%d, talk %v, logs %s", p.entity.x, p.entity.y, p.talk, logs)
	}
	// A progress bar ends when the player walks, and so does the
	// conversation.
	z2, p2, n2, logs2 := npcZone(t, waiter)
	ev := click(z2, p2, n2)
	var progress *wathenav1.NpcProgress
	for _, e := range ev {
		if e.GetNpcProgress() != nil {
			progress = e.GetNpcProgress()
		}
	}
	if progress == nil || progress.GetDurationMs() != 3000 {
		t.Fatalf("progress %v, logs %s", progress, logs2)
	}
	z2.handle(command{player: p2, walkTo: &[2]int{5, 5}})
	z2.stepAt(z2.now + 50)
	ev = events(p2)
	if p2.talk != nil || !hasChat(ev, "progress false the conversation was closed") {
		t.Fatalf("talk %v, events %v, logs %s", p2.talk, ev, logs2)
	}
	if d := lastDialog(ev); d == nil || !d.HasEnd() {
		t.Errorf("the window did not close: %v", d)
	}
}

func TestProgressBarEnds(t *testing.T) {
	z, p, n, logs := npcZone(t, waiter)
	click(z, p, n)
	z.stepAt(z.now + 2900)
	if p.talk == nil {
		t.Fatal("the progress bar ended early")
	}
	z.stepAt(z.now + 200)
	if ev := events(p); !hasChat(ev, "progress true nil") {
		t.Fatalf("events %v, logs %s", ev, logs)
	}
}

// hasChat reports whether a local chat line is in events.
func hasChat(ev []*wathenav1.Event, text string) bool {
	for _, e := range ev {
		if c := e.GetChat(); c != nil && c.GetText() == text {
			return true
		}
	}
	return false
}

func TestConversationEndsWhenThePlayerLeaves(t *testing.T) {
	z, p, n, logs := npcZone(t, `
local events = require "wa.events"
return function()
  local npc = require("wa.context"):get_instance()
  events.on("click", function(player, conversation)
    local ok, err = pcall(conversation.wait_for_next, conversation)
    local ok2, err2 = pcall(function() return player.name end)
    npc:say_aloud(tostring(ok) .. " " .. tostring(err) .. " / " .. tostring(ok2) .. " " .. tostring(err2):match("the player.*"))
  end)
end`)
	b := add(z, "b")
	b.entity.x, b.entity.y = 19, 20
	z.stepAt(z.now + 50)
	events(b)
	click(z, p, n)
	z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	z.stepAt(z.now + 50)
	if ev := events(b); !hasChat(ev, "false the conversation was closed / false the player has left this map") {
		t.Fatalf("events %v, logs %s", ev, logs)
	}
	if strings.Contains(logs.String(), "failed") {
		t.Errorf("a closed conversation was logged as a failure: %s", logs)
	}
	if z.scripts.Conversations() != 0 {
		t.Errorf("%d conversations left", z.scripts.Conversations())
	}
}

func TestConversationTimesOut(t *testing.T) {
	z, p, n, _ := npcZone(t, chooser)
	click(z, p, n)
	z.stepAt(z.now + timeoutMenu - 100)
	if p.talk == nil {
		t.Fatal("the menu timed out early")
	}
	z.stepAt(z.now + 200)
	ev := events(p)
	if d := lastDialog(ev); p.talk != nil || d == nil || !d.HasEnd() {
		t.Fatalf("talk %v, last step %v", p.talk, d)
	}
}

func TestClickRules(t *testing.T) {
	z, p, n, _ := npcZone(t, chooser)
	// Out of reach: AREA_SIZE + 1 cells is the most. The player still
	// knows the NPC, as when it moved away during this tick.
	id, _ := p.netIDs.get(n)
	p.entity.x = n.x + npcClickRange + 1
	z.handle(command{player: p, npcClick: &id})
	if p.talk != nil {
		t.Fatal("a click from too far started a conversation")
	}
	p.entity.x = n.x + ViewRange
	if ev := click(z, p, n); len(dialogs(ev)) == 0 || p.talk == nil {
		t.Fatal("a click in reach did nothing")
	}
	// A second click while talking does nothing.
	first := p.talk
	if ev := click(z, p, n); len(dialogs(ev)) != 0 || p.talk != first {
		t.Fatal("a click while talking started another conversation")
	}
	// An answer from out of reach is dropped, as npc_scriptcont drops it.
	d := first.dialog
	p.entity.x = n.x + npcClickRange + 1
	z.handle(command{player: p, npcReply: reply(d, func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(1)) })})
	if p.talk != first || first.dialog != d {
		t.Fatal("an answer from too far went through")
	}
}

func TestReloadMidConversation(t *testing.T) {
	z, p, n, logs := npcZone(t, chooser)
	ev := click(z, p, n)
	menu := lastDialog(ev)
	sc := npcMod(strings.ReplaceAll(chooser, "Bye.", "Bye from the new version."))
	if err := z.scripts.Reload(sc); err != nil {
		t.Fatal(err)
	}
	ev = answer(z, p, reply(menu.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(1)) }))
	next := lastDialog(ev)
	ev = answer(z, p, reply(next.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Next = new(true) }))
	if got := lines(ev); got != "Bye." {
		t.Fatalf("the running conversation got %q, logs %s", got, logs)
	}
	answer(z, p, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	// A new click runs the new version.
	ev = click(z, p, n)
	menu = lastDialog(ev)
	ev = answer(z, p, reply(menu.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(1)) }))
	ev = answer(z, p, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Next = new(true) }))
	if got := lines(ev); got != "Bye from the new version." {
		t.Fatalf("a new conversation got %q", got)
	}
}

// TestHealPercentTakes checks review decision 56: player:heal_percent
// with a negative amount takes that percent of the maximum, as
// percentheal, and never takes HP below 1. A positive amount still
// heals, beside a negative one.
func TestHealPercentTakes(t *testing.T) {
	z, p, n, logs := npcZone(t, `
local events = require "wa.events"
return function()
  local step = 0
  events.on("click", function(player)
    step = step + 1
    if step == 1 then player:heal_percent { hp = -30, sp = -50 } end
    if step == 2 then player:heal_percent { hp = -100, sp = -100 } end
    if step == 3 then player:heal_percent { hp = 10, sp = -1 } end
  end)
end`)
	p.entity.hp, p.sp = 150, 50
	click(z, p, n)
	if p.entity.hp != 90 || p.sp != 25 {
		t.Fatalf("-30%% and -50%%: HP %d, SP %d, want 90 and 25: %s", p.entity.hp, p.sp, logs)
	}
	click(z, p, n)
	if p.entity.hp != 1 || p.sp != 0 || p.entity.dead {
		t.Fatalf("-100%%: HP %d, SP %d, dead %v, want 1 and 0", p.entity.hp, p.sp, p.entity.dead)
	}
	p.sp = 10
	click(z, p, n)
	// 10% of 200 HP heals 20. -1% of 50 SP is 0, which takes 1.
	if p.entity.hp != 21 || p.sp != 9 {
		t.Fatalf("10%% and -1%%: HP %d, SP %d, want 21 and 9", p.entity.hp, p.sp)
	}
}
