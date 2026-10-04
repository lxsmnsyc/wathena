package zone

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/server/worldevent"
)

// fakeSink is the event manager as a zone sees it, for tests.
type fakeSink struct {
	mu    sync.Mutex
	notes []worldevent.Note
	hint  []string
}

func (s *fakeSink) Note(n worldevent.Note) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = append(s.notes, n)
}

func (s *fakeSink) Hint(int64, string, int, int) []string { return s.hint }

func (s *fakeSink) Request(string, string, bool) (bool, error) { return true, nil }

func (s *fakeSink) Running() []worldevent.Notice {
	return []worldevent.Notice{{Name: "core:Field_Boss", DisplayName: "Field Boss", Type: "boss", Map: "map/test"}}
}

var testRun = worldevent.RunInfo{ID: 7, Event: "core:Field_Boss", Display: "Field Boss", Type: worldevent.TypeBoss,
	Scheduled: time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)}

func TestEventInvadersEnterAtTheEdgesAndMarch(t *testing.T) {
	z, p, _, _ := worldZone(t, "scripts/main.lua", `return function() end`)
	p.entity.x, p.entity.y = 20, 20
	placed, err := z.spawnEvent(worldevent.Spawn{RunInfo: testRun, Map: z.Map, Monster: 1002, Amount: 6, Role: worldevent.RoleInvader, Edge: true})
	if err != nil || len(placed) != 6 {
		t.Fatalf("placed %v %v", placed, err)
	}
	for _, c := range placed {
		// The map's border cells are walls, as rAthena's.
		if c.X > 1 && c.Y > 1 && c.X < 38 && c.Y < 38 {
			t.Errorf("an invader entered at %d,%d, away from the edges", c.X, c.Y)
		}
	}
	far := 0
	for e := range z.typedRuns[testRun.ID].monsters {
		if e.status.Mode&mob.ModeAggressive == 0 {
			t.Errorf("an invader is not aggressive: mode %#x", e.status.Mode)
		}
		far += chebyshev(e, p.entity)
	}
	// The core Poring's own status stays as it was.
	if z.Mods.Monsters[1002].Status.Mode&mob.ModeAggressive != 0 {
		t.Fatal("the template became aggressive")
	}
	for i := range 100 {
		z.stepAt(int64(i+1) * 100)
	}
	near := 0
	for e := range z.typedRuns[testRun.ID].monsters {
		near += chebyshev(e, p.entity)
	}
	if near >= far {
		t.Fatalf("the invaders did not come closer: %d cells, then %d", far, near)
	}
	if n := z.eventAlive(testRun.ID); n != 6 && !p.entity.dead {
		t.Fatalf("%d alive", n)
	}
	z.clearEvent(testRun.ID)
	if n := len(monstersOf(z, 1002)); n != 1 {
		t.Fatalf("%d Porings after the run was cleared; only the map's own is left", n)
	}
}

func TestEventBossRewardsParticipants(t *testing.T) {
	z, a, _, _ := worldZone(t, "scripts/main.lua", `return function() end`)
	z.Items = testItems()
	sink := &fakeSink{}
	z.WorldEvents = sink
	b := add(z, "b")
	c := add(z, "c")
	b.entity.x, b.entity.y = a.entity.x, a.entity.y+1
	b.maxWeight = 10000
	rw := &worldevent.Rewards{To: worldevent.ToParticipants, MinShare: 25, Zeny: 10,
		Items: []worldevent.RewardItem{{ID: 501, Amount: 2}}, BaseExp: 5}
	if _, err := z.spawnEvent(worldevent.Spawn{RunInfo: testRun, Map: z.Map, Monster: 1002, Amount: 1, Role: worldevent.RoleBoss,
		Unit: worldevent.UnitBoss, Rewards: rw}); err != nil {
		t.Fatal(err)
	}
	var boss *entity
	for e := range z.typedRuns[testRun.ID].monsters {
		boss = e
	}
	// a does 20 damage and b 10: both are at or above 25%. c did none.
	a.record.Zeny = character.MaxZeny - 4
	z.logDamage(a.entity, boss, 20)
	z.logDamage(b.entity, boss, 10)
	if got := z.participants(z.typedRuns[testRun.ID]); len(got) != 2 {
		t.Fatalf("%d participants", len(got))
	}
	boss.hp = 0
	z.die(boss, a.entity)
	if a.record.Zeny != character.MaxZeny || b.record.Zeny != 10 || c.record.Zeny != 0 {
		t.Fatalf("zeny %d %d %d", a.record.Zeny, b.record.Zeny, c.record.Zeny)
	}
	if n := countOf(b, 501); n != 2 {
		t.Fatalf("b has %d Red Potions", n)
	}
	// The zeny over MAX_ZENY goes by mail, and so do items that do not
	// fit: a can carry nothing.
	if len(z.mailDeliver) != 1 || z.mailDeliver[0].Zeny != 6 || z.mailDeliver[0].ToName != "a" || len(z.mailDeliver[0].Items()) != 1 {
		t.Fatalf("mails %+v", z.mailDeliver)
	}
	// The reward is logged as an event's, and saved with its unit.
	actions := map[transfer.Action]bool{}
	for _, l := range z.txLogs.Zeny {
		actions[l.Action] = l.From.Kind == transfer.KindEvent
	}
	if !actions[transfer.ActionEvent] {
		t.Fatalf("zeny logs %+v", z.txLogs.Zeny)
	}
	if u := z.txEventUnits; len(u) != 1 || u[0].Unit != worldevent.UnitBoss || u[0].Event != testRun.Event || !u[0].Scheduled.Equal(testRun.Scheduled) {
		t.Fatalf("units %+v", u)
	}
	if n := sink.notes; len(n) != 1 || n[0].Kind != worldevent.NoteDied || n[0].Killer != "a" || n[0].Unit != worldevent.UnitBoss {
		t.Fatalf("notes %+v", n)
	}
	// A second reward of the same unit gives nothing.
	z.eventReward(worldevent.RewardRun{RunInfo: testRun, Map: z.Map, Unit: worldevent.UnitBoss, Rewards: rw})
	if b.record.Zeny != 10 {
		t.Fatalf("the reward came twice: %d", b.record.Zeny)
	}
}

func countOf(p *Player, id uint32) int {
	n := 0
	for _, s := range p.inventory.Slots {
		if s.ID == id {
			n += s.Amount
		}
	}
	return n
}

func TestEventEveryoneAndKillerRewards(t *testing.T) {
	z, a, _, _ := worldZone(t, "scripts/main.lua", `return function() end`)
	z.Items = testItems()
	b := add(z, "b")
	run := testRun
	run.ID, run.Type = 8, worldevent.TypeInvasion
	if _, err := z.spawnEvent(worldevent.Spawn{RunInfo: run, Map: z.Map, Monster: 1002, Amount: 1, Role: worldevent.RoleInvader}); err != nil {
		t.Fatal(err)
	}
	for e := range z.typedRuns[run.ID].monsters {
		z.logDamage(b.entity, e, 5)
		e.hp = 0
		z.die(e, b.entity)
	}
	z.eventReward(worldevent.RewardRun{RunInfo: run, Map: z.Map, Unit: worldevent.UnitResult,
		Rewards: &worldevent.Rewards{To: worldevent.ToEveryone, Zeny: 3}})
	if a.record.Zeny != 3 || b.record.Zeny != 3 {
		t.Fatalf("everyone: %d %d", a.record.Zeny, b.record.Zeny)
	}
	run.ID = 9
	z.runOf(run, nil).lastKiller = "b"
	z.eventReward(worldevent.RewardRun{RunInfo: run, Map: z.Map, Unit: worldevent.UnitResult,
		Rewards: &worldevent.Rewards{To: worldevent.ToKiller, Zeny: 4}})
	if a.record.Zeny != 3 || b.record.Zeny != 7 {
		t.Fatalf("killer: %d %d", a.record.Zeny, b.record.Zeny)
	}
}

func TestEventHintNPC(t *testing.T) {
	z, p, _, _ := worldZone(t, "scripts/main.lua", `return function() end`)
	z.WorldEvents = &fakeSink{hint: []string{"1 chest is still hidden.", "One lies to the north of here, 5 cells away."}}
	run := testRun
	run.Type = worldevent.TypeTreasure
	z.runOf(run, nil)
	z.eventHints(run.ID, []worldevent.Hint{{Map: z.Map, X: p.entity.x + 1, Y: p.entity.y, DisplayName: "Guide", SpriteID: 50}})
	z.stepAt(100)
	events(p)
	var npc *entity
	for e := range z.entities.all() {
		if e.npc != nil && e.npc.hint == run.ID {
			npc = e
		}
	}
	if npc == nil {
		t.Fatal("no hint NPC")
	}
	id := p.netID(npc)
	z.handle(command{player: p, npcClick: &id})
	z.stepAt(200)
	var lines []string
	closeButton := false
	for _, ev := range events(p) {
		if d := ev.GetNpcDialog(); d != nil {
			lines = append(lines, d.GetLines()...)
			closeButton = closeButton || d.GetClose()
		}
	}
	if !slices.Contains(lines, "[Guide]") || !slices.Contains(lines, "One lies to the north of here, 5 cells away.") || !closeButton {
		t.Fatalf("dialog %q close %v", lines, closeButton)
	}
	z.handle(command{player: p, npcReply: wathenav1.NpcReply_builder{Dialog: p.talk.dialog, Close: new(true)}.Build()})
	if p.talk != nil {
		t.Fatal("the dialog did not close")
	}
	z.clearEvent(run.ID)
	if z.entities.has(npc) {
		t.Fatal("the NPC stayed after the run")
	}
}

func TestWorldEventNotices(t *testing.T) {
	z, p, _, logs := worldZone(t, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("world_event_end", function(ev)
    error("world_event_end " .. ev.name .. " " .. ev.type .. " " .. tostring(ev.result))
  end)
end`)
	z.WorldEvents = &fakeSink{}
	z.worldNotice(worldevent.Notice{Name: "core:Field_Boss", DisplayName: "Field Boss", Type: "boss", Started: false, Result: "won"})
	z.stepAt(100)
	var got *wathenav1.WorldEventNotice
	for _, ev := range events(p) {
		if n := ev.GetWorldEventNotice(); n != nil {
			got = n
		}
	}
	if got == nil || got.GetEvent().GetName() != "core:Field_Boss" || got.GetStarted() || got.GetResult() != "won" || got.GetEvent().GetType() != "boss" {
		t.Fatalf("notice %v", got)
	}
	// The handler ran, with the event's fields.
	if !strings.Contains(logs.String(), "world_event_end core:Field_Boss boss won") {
		t.Fatalf("logs: %s", logs)
	}
	// A player who logs in gets the runs that last.
	q := add(z, "late")
	z.stepAt(200)
	found := false
	for _, ev := range events(q) {
		if l := ev.GetWorldEventList(); l != nil && len(l.GetEvents()) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("no world event list at login")
	}
}
