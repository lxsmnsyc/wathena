package zone

import (
	"context"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// zones is a Directory of zones by map name.
type zones map[string]*Zone

func (d zones) Zone(name string) *Zone { return d[name] }

func (d zones) Announce(text string, color uint32) {
	for _, z := range d {
		z.Announce(text, color)
	}
}

// twoMaps makes map/a and map/b, 40 × 40 cells each, with the item and
// exp tables of saveZone. A warp on map/a at 20,20 with a span of 1 leads
// to map/b at 5,6.
func twoMaps() (a, b *Zone) {
	a, b = saveZone(), saveZone()
	a.Map, b.Map = "map/a", "map/b"
	d := zones{"map/a": a, "map/b": b}
	a.World, b.World = d, d
	a.SetWarps([]Warp{{Name: "a_b", X: 20, Y: 20, XS: 1, YS: 1, To: "map/b", ToX: 5, ToY: 6}})
	return a, b
}

// onMap joins a saved character to a zone at a cell, with a session.
func onMap(z *Zone, name string, x, y int, s Saver) *Player {
	c := savedCharacter()
	c.Name, c.Map, c.X, c.Y = name, z.Map, x, y
	return addCharacter(z, c, s)
}

// arrive delivers the next player on its way to a zone.
func arrive(t *testing.T, z *Zone) *Player {
	t.Helper()
	select {
	case h := <-z.arrivals:
		z.arrive(h)
		return h.sess.player
	case <-time.After(5 * time.Second):
		t.Fatal("no player arrived")
		return nil
	}
}

func TestWalkOntoWarpChangesMap(t *testing.T) {
	a, b := twoMaps()
	s := &saves{}
	p := onMap(a, "Walker", 14, 20, s)
	a.stepAt(50)
	events(p)
	// The walk east crosses the warp's area, x 19 to 21.
	a.handle(command{player: p, seq: 4, walkTo: &[2]int{30, 20}})
	for now := int64(100); now < 100+4*WalkSpeed; now += 50 {
		a.stepAt(now)
		if p.entity.x >= 19 {
			t.Fatalf("the player reached x %d before the warp took it", p.entity.x)
		}
	}
	// The fifth step enters x 19, in the area. The walk ends there, and
	// the player leaves map/a.
	a.stepAt(100 + 5*WalkSpeed)
	if a.entities.has(p.entity) || len(a.players) != 0 {
		t.Fatal("the player is still on map/a")
	}
	if len(s.got) != 0 {
		t.Fatalf("%d saves before the player arrived", len(s.got))
	}
	q := arrive(t, b)
	if q == nil || len(b.players) != 1 || q.entity.x != 5 || q.entity.y != 6 {
		t.Fatalf("on map/b: %+v", q)
	}
	// The map change saves the character at its new place, once.
	if len(s.got) != 1 || s.final[0] || s.got[0].Map != "map/b" || s.got[0].X != 5 || s.got[0].Y != 6 {
		t.Fatalf("saves %+v, final %v", s.got, s.final)
	}
	// The connection gets what map/a sent first. The first frame on
	// map/b starts with MapChange, then the player sees itself with the
	// ID MapChange gave.
	events(q)
	b.stepAt(50)
	ev := events(q)
	mc := ev[0].GetMapChange()
	if mc == nil || mc.GetMap() != "map/b" || mc.GetX() != 5 || mc.GetY() != 6 {
		t.Fatalf("first event %v", ev[0])
	}
	if ap := ev[1].GetAppear(); ap == nil || ap.GetId() != mc.GetNetId() || ap.GetKind() != wathenav1.EntityKind_ENTITY_KIND_PLAYER {
		t.Errorf("second event %v", ev[1])
	}
	if q.ackSeq != 4 {
		t.Errorf("ack %d, want the last command of map/a", q.ackSeq)
	}
}

func TestHandoffKeepsCharacterOnce(t *testing.T) {
	a, b := twoMaps()
	s := &saves{}
	p := onMap(a, "Keeper", 20, 18, s)
	p.maxWeight = maxWeight
	a.addItem(p, 909, 7)
	a.gainExp(p, 20, 5)
	p.entity.hp = 77
	p.itemDelays[501] = a.now + 3000
	before := a.snapshot(p)

	// One step north enters the warp's area.
	a.handle(command{player: p, walkTo: &[2]int{20, 19}})
	a.stepAt(a.now + 200)
	q := arrive(t, b)
	after := b.snapshot(q)
	before.Map, before.X, before.Y = "map/b", 5, 6
	if !after.Equal(before) {
		t.Errorf("the character changed on the way:\n%+v\n%+v", after, before)
	}
	// Exactly one copy exists: map/a forgot the player.
	if len(a.players) != 0 || len(b.players) != 1 {
		t.Errorf("%d players on map/a, %d on map/b", len(a.players), len(b.players))
	}
	for e := range a.entities.all() {
		if e.player != nil {
			t.Error("map/a still has a player entity")
		}
	}
	if left := q.itemDelays[501] - b.now; left <= 0 || left > 3000 {
		t.Errorf("item delay %d ms left after the warp", left)
	}
	if z, pl := q.sess.where(); z != b || pl != q {
		t.Error("the session does not point at map/b")
	}
}

func TestCommandsGoToTheNewZone(t *testing.T) {
	a, b := twoMaps()
	p := onMap(a, "Router", 20, 18, &saves{})
	sess := p.sess
	a.handle(command{player: p, walkTo: &[2]int{20, 19}})
	a.stepAt(a.now + 200)
	// Between zones, commands are dropped without error.
	if !sess.WalkTo(9, 1, 1) {
		t.Error("a command between zones was refused")
	}
	q := arrive(t, b)
	if !sess.WalkTo(10, 8, 8) {
		t.Fatal("the command was refused")
	}
	select {
	case c := <-b.commands:
		if c.player != q || c.seq != 10 {
			t.Errorf("command %+v", c)
		}
	default:
		t.Fatal("the command did not reach map/b")
	}
	select {
	case c := <-a.commands:
		t.Errorf("map/a got %+v", c)
	default:
	}
}

func TestChatAndViewStayOnTheirMap(t *testing.T) {
	a, b := twoMaps()
	x := onMap(a, "Left", 10, 10, &saves{})
	y := onMap(b, "Right", 10, 10, &saves{})
	a.stepAt(50)
	b.stepAt(50)
	for _, p := range []*Player{x, y} {
		for _, e := range events(p) {
			if ap := e.GetAppear(); ap != nil && ap.GetKind() == wathenav1.EntityKind_ENTITY_KIND_PLAYER && ap.GetName() != p.Name {
				t.Errorf("%s sees %s on another map", p.Name, ap.GetName())
			}
		}
	}
	a.handle(command{player: x, chat: new("hello")})
	a.stepAt(100)
	b.stepAt(100)
	if len(chats(y)) != 0 {
		t.Error("chat crossed maps")
	}
	if len(chats(x)) != 1 {
		t.Error("the speaker did not hear itself")
	}
}

func TestWarpRules(t *testing.T) {
	a, b := twoMaps()
	// Overlapping areas: the warp loaded first wins.
	a.SetWarps([]Warp{
		{Name: "first", X: 20, Y: 20, XS: 2, YS: 2, To: "map/b", ToX: 5, ToY: 6},
		{Name: "second", X: 21, Y: 20, XS: 2, YS: 2, To: "map/b", ToX: 30, ToY: 30},
	})
	if w := a.warpAt(21, 21); w == nil || w.Name != "first" {
		t.Errorf("warp at 21,21 is %+v", w)
	}
	// Visible warps are NPCs with the warp sprite.
	p := onMap(a, "Seer", 15, 15, &saves{})
	a.stepAt(50)
	portals := 0
	for _, e := range events(p) {
		if ap := e.GetAppear(); ap != nil && ap.GetKind() == wathenav1.EntityKind_ENTITY_KIND_NPC {
			portals++
			if ap.GetLook().GetBody() != WarpSprite || ap.GetName() != "" {
				t.Errorf("portal %v", ap)
			}
		}
	}
	if portals != 2 {
		t.Errorf("sees %d portals", portals)
	}
	// A dead player does not warp.
	p.entity.dead = true
	if a.target(p, a.warpAt(20, 20)) != nil {
		t.Error("a dead player warps")
	}
	p.entity.dead = false
	// A warp to a map that does not run leaves the walk going, as
	// pc_setpos does without a map server.
	a.SetWarps([]Warp{{Name: "nowhere", X: 20, Y: 15, XS: 0, YS: 0, To: "map/none", ToX: 5, ToY: 5}})
	a.handle(command{player: p, walkTo: &[2]int{25, 15}})
	for now := int64(100); now <= 100+12*WalkSpeed; now += 50 {
		a.stepAt(now)
	}
	if !a.entities.has(p.entity) || p.entity.x != 25 {
		t.Errorf("the player is at %d, want 25 on map/a", p.entity.x)
	}
	_ = b
}

func TestArrivalCell(t *testing.T) {
	_, b := twoMaps()
	b.grid.Cells[6*b.grid.Width+5] = 0
	// An unwalkable cell inside the map is used, as pc_setpos does.
	if x, y := b.arrivalCell(5, 6); x != 5 || y != 6 {
		t.Errorf("5,6 became %d,%d", x, y)
	}
	b.SetWarps([]Warp{{X: 20, Y: 20, XS: 3, YS: 3, To: "map/a"}})
	for _, c := range [][2]int{{0, 0}, {-1, 5}, {40, 5}, {5, 400}} {
		for range 50 {
			x, y := b.arrivalCell(c[0], c[1])
			if x < 1 || y < 1 || x > 38 || y > 38 || b.grid.NoPass(x, y) || b.warpNear(x, y, 1) {
				t.Fatalf("%v became %d,%d", c, x, y)
			}
		}
	}
}

func TestRewarpLimit(t *testing.T) {
	z := saveZone()
	// Two warps on one map lead onto each other.
	z.SetWarps([]Warp{
		{Name: "there", X: 10, Y: 10, To: "map/test", ToX: 30, ToY: 30},
		{Name: "back", X: 30, Y: 30, To: "map/test", ToX: 10, ToY: 10},
	})
	p := onMap(z, "Looper", 10, 12, &saves{})
	z.stepAt(50)
	z.handle(command{player: p, walkTo: &[2]int{10, 10}})
	warps := 0
	last := [2]int{p.entity.x, p.entity.y}
	for now := int64(100); now < 10000; now += 50 {
		z.stepAt(now)
		if at := [2]int{p.entity.x, p.entity.y}; at != last && (at == [2]int{10, 10} || at == [2]int{30, 30}) && abs(at[0]-last[0]) > 1 {
			warps++
		}
		last = [2]int{p.entity.x, p.entity.y}
	}
	// count_rewarp allows the first warp and 10 more onto warp areas.
	if warps != 11 {
		t.Errorf("%d warps, want 11", warps)
	}
}

func TestLeaveBetweenZones(t *testing.T) {
	a, b := twoMaps()
	s := &saves{}
	p := onMap(a, "Quitter", 20, 18, s)
	sess := p.sess
	a.handle(command{player: p, walkTo: &[2]int{20, 19}})
	a.stepAt(a.now + 200)
	// The connection closes while the player is on its way.
	done := make(chan struct{})
	go func() {
		sess.Leave()
		close(done)
	}()
	for ended := false; !ended; {
		sess.mu.Lock()
		ended = sess.end != endNone
		sess.mu.Unlock()
	}
	arrive(t, b)
	<-done
	if len(b.players) != 0 {
		t.Error("the player joined map/b after leaving")
	}
	if len(s.got) != 1 || !s.final[0] || s.got[0].Map != "map/b" {
		t.Errorf("saves %+v, final %v, want one final save", s.got, s.final)
	}
	if _, ok := <-sess.Out; ok {
		for range sess.Out {
		}
	}
}

func TestArrivalAtStoppedZone(t *testing.T) {
	a, b := twoMaps()
	s := &saves{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.Run(ctx)
	p := onMap(a, "Late", 20, 18, s)
	sess := p.sess
	a.handle(command{player: p, walkTo: &[2]int{20, 19}})
	a.stepAt(a.now + 200)
	b.WaitIncoming()
	select {
	case <-sess.left:
	default:
		t.Fatal("the session did not end")
	}
	if len(s.got) != 1 || !s.final[0] {
		t.Errorf("saves %v, want one final save", s.final)
	}
	for range sess.Out {
	}
}

func TestSameMapWarpTeleports(t *testing.T) {
	z := saveZone()
	z.SetWarps([]Warp{{Name: "hop", X: 10, Y: 10, To: "map/test", ToX: 30, ToY: 31}})
	p := onMap(z, "Hopper", 10, 12, &saves{})
	o := onMap(z, "Watcher", 10, 14, &saves{})
	z.stepAt(50)
	events(p)
	events(o)
	z.handle(command{player: p, walkTo: &[2]int{10, 9}})
	for now := int64(100); now < 1000; now += 50 {
		z.stepAt(now)
	}
	if p.entity.x != 30 || p.entity.y != 31 || !z.entities.has(p.entity) {
		t.Fatalf("at %d,%d", p.entity.x, p.entity.y)
	}
	gone := false
	for _, e := range events(o) {
		if e.GetVanish().GetReason() == wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT {
			gone = true
		}
	}
	if !gone {
		t.Error("the watcher did not see the player go")
	}
	for _, e := range events(p) {
		if e.HasMapChange() {
			t.Error("a warp on the same map sent MapChange")
		}
	}
}

func TestWakeResetsClock(t *testing.T) {
	spawns := []Spawn{{Mob: 1002, Name: "Poring", Amount: 3, Speed: 400, CanMove: true}}
	z := New("map/test", open(40), rng.FromKey([32]byte{2}).Stream(), spawns)
	z.asleep = true
	for e := range z.entities.all() {
		e.monster.nextWalk = 5
	}
	z.start = time.Now().Add(-time.Minute)
	z.wake()
	if z.asleep || z.now < 60000 || z.prevHeal != z.now {
		t.Errorf("after wake: asleep %v, now %d, prevHeal %d", z.asleep, z.now, z.prevHeal)
	}
	for e := range z.entities.all() {
		if e.monster.nextWalk != -1 {
			t.Error("a monster kept its old wander time")
		}
	}
}
