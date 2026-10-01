package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// testTables are small exp tables: novices need 10, 20 and 30 base exp
// for levels 2 to 4, and 5 job exp per job level, up to job level 3.
func testTables() *progression.Tables {
	maxBase, maxJob := 4, 3
	d := &progression.Data{Exp: []progression.ExpGroup{
		{Jobs: []int{0}, MaxBaseLevel: &maxBase, BaseExp: []int64{10, 20, 30, 40}},
		{Jobs: []int{0}, MaxJobLevel: &maxJob, JobExp: []int64{5, 5, 5}},
	}}
	return progression.New(d, map[uint32]string{909: "Etc", 9001: "PetEgg"}, progression.DefaultConfig)
}

// lootZone makes a zone with exp tables and one player two cells from a
// monster with 30 HP that the player kills in three hits of 10.
func lootZone(t *testing.T, reward *progression.Mob) (*Zone, *Player, *entity) {
	t.Helper()
	mob := &battle.Status{MaxHP: 30, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	spawns := []Spawn{{Mob: 1002, Name: "Poring", Amount: 1, X: 20, Y: 20, Speed: 400, Status: mob, RespawnMs: 60000, Reward: reward}}
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), spawns)
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 100, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	p := add(z, "a")
	var m *entity
	for e := range z.entities.all() {
		if e.monster != nil {
			m = e
		}
	}
	p.entity.x, p.entity.y = m.x+2, m.y
	return z, p, m
}

// kill attacks the monster until it dies and returns the events the
// player saw.
func kill(t *testing.T, z *Zone, p *Player, m *entity) []*wathenav1.Event {
	t.Helper()
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: p.netIDs.id(m), repeat: true}})
	var seen []*wathenav1.Event
	for start, now := z.now, z.now+50; now <= start+5000 && z.entities.has(m); now += 50 {
		z.stepAt(now)
		seen = append(seen, events(p)...)
	}
	if z.entities.has(m) {
		t.Fatal("the monster did not die")
	}
	// One more tick, so the items that fell show.
	z.stepAt(z.now + 50)
	return append(seen, events(p)...)
}

func stats(ev []*wathenav1.Event) map[wathenav1.StatId]int64 {
	out := map[wathenav1.StatId]int64{}
	for _, e := range ev {
		for _, s := range e.GetStats().GetStats() {
			out[s.GetId()] = s.GetValue()
		}
	}
	return out
}

func TestJoinSendsProgress(t *testing.T) {
	z, p, _ := lootZone(t, nil)
	z.stepAt(50)
	got := stats(events(p))
	want := map[wathenav1.StatId]int64{
		wathenav1.StatId_STAT_ID_BASE_LEVEL: 1, wathenav1.StatId_STAT_ID_JOB_LEVEL: 1,
		wathenav1.StatId_STAT_ID_BASE_EXP: 0, wathenav1.StatId_STAT_ID_JOB_EXP: 0,
		wathenav1.StatId_STAT_ID_NEXT_BASE_EXP: 10, wathenav1.StatId_STAT_ID_NEXT_JOB_EXP: 5,
		wathenav1.StatId_STAT_ID_STATUS_POINTS: 48, wathenav1.StatId_STAT_ID_SKILL_POINTS: 0,
	}
	for id, v := range want {
		if g, ok := got[id]; !ok || g != v {
			t.Errorf("%v: %d (sent %v), want %d", id, g, ok, v)
		}
	}
}

func TestKillGivesExpAndLevels(t *testing.T) {
	z, p, m := lootZone(t, &progression.Mob{ID: 1002, Level: 1, MaxHP: 30, BaseExp: 15, JobExp: 25})
	b := add(z, "b")
	b.entity.x, b.entity.y = p.entity.x, p.entity.y+1
	z.stepAt(50)
	events(p)
	events(b)
	// The player loses some HP first, to see the level up heal it.
	p.entity.hp = 50
	ev := kill(t, z, p, m)

	c := p.progress
	// 15 base exp: level 2 with 5 left. 25 job exp: one level only, with
	// the 5 - 1 = 4 that may carry over.
	if c.BaseLevel != 2 || c.BaseExp != 5 || c.StatusPoints != 51 || c.JobLevel != 2 || c.JobExp != 4 || c.SkillPoints != 1 {
		t.Errorf("progress %+v", c)
	}
	got := stats(ev)
	for id, v := range map[wathenav1.StatId]int64{
		wathenav1.StatId_STAT_ID_BASE_LEVEL: 2, wathenav1.StatId_STAT_ID_BASE_EXP: 5, wathenav1.StatId_STAT_ID_NEXT_BASE_EXP: 20,
		wathenav1.StatId_STAT_ID_JOB_LEVEL: 2, wathenav1.StatId_STAT_ID_JOB_EXP: 4, wathenav1.StatId_STAT_ID_SKILL_POINTS: 1,
		wathenav1.StatId_STAT_ID_STATUS_POINTS: 51, wathenav1.StatId_STAT_ID_HP: 100,
	} {
		if got[id] != v {
			t.Errorf("%v: %d, want %d", id, got[id], v)
		}
	}
	if p.entity.hp != 100 {
		t.Errorf("HP %d after a base level up", p.entity.hp)
	}
	// Both players see both level up effects on the player.
	for _, viewer := range []struct {
		p  *Player
		ev []*wathenav1.Event
	}{{p, ev}, {b, events(b)}} {
		var effects []uint32
		for _, e := range viewer.ev {
			if ef := e.GetEffect(); ef != nil && ef.GetEntity() == viewer.p.netIDs.id(p.entity) {
				effects = append(effects, ef.GetEffectId())
			}
		}
		if len(effects) != 2 || effects[0] != effectBaseLevelUp || effects[1] != effectJobLevelUp {
			t.Errorf("%s sees effects %v", viewer.p.Name, effects)
		}
	}
	// b did not fight, so it gets nothing.
	if b.progress.BaseExp != 0 || b.progress.BaseLevel != 1 {
		t.Errorf("b %+v", b.progress)
	}
}

func items(ev []*wathenav1.Event) []*wathenav1.EntityAppear {
	var out []*wathenav1.EntityAppear
	for _, e := range ev {
		if a := e.GetAppear(); a.GetKind() == wathenav1.EntityKind_ENTITY_KIND_ITEM {
			out = append(out, a)
		}
	}
	return out
}

func TestDropsAppearAndExpire(t *testing.T) {
	z, p, m := lootZone(t, &progression.Mob{ID: 1002, Level: 1, MaxHP: 30, Drops: []progression.Drop{{Item: 909, Rate: 10000}, {Item: 909, Rate: 10000}, {Item: 9001, Rate: 10000}}})
	z.stepAt(50)
	events(p)
	mx, my := m.x, m.y
	ev := kill(t, z, p, m)

	// The pet egg goes to the inventory. The other two fall: the first
	// on the monster's cell, the second south-east or next to it.
	if p.inventory.Count(9001) != 1 {
		t.Errorf("inventory %v", p.inventory)
	}
	seen := items(ev)
	if len(seen) != 2 {
		t.Fatalf("saw %d items", len(seen))
	}
	for _, a := range seen {
		g := a.GetItem()
		if g.GetItemId() != 909 || g.GetAmount() != 1 || !g.GetDropped() || g.GetSubX()%3 != 0 || g.GetSubX() < 3 || g.GetSubX() > 12 {
			t.Errorf("item %v", a)
		}
	}
	// Frames list entities in no set order.
	if seen[1].GetX() == uint32(mx) && seen[1].GetY() == uint32(my) {
		seen[0], seen[1] = seen[1], seen[0]
	}
	if seen[0].GetX() != uint32(mx) || seen[0].GetY() != uint32(my) {
		t.Errorf("no item on the monster's cell (%d, %d): %v", mx, my, seen)
	}
	if dx, dy := int(seen[1].GetX())-mx, int(seen[1].GetY())-my; abs(dx) > 2 || abs(dy) > 2 || dx == 0 && dy == 0 {
		t.Errorf("second item at offset (%d, %d)", dx, dy)
	}

	// A player who comes later sees the items already on the ground.
	b := add(z, "b")
	b.entity.x, b.entity.y = mx, my+3
	z.stepAt(z.now + 50)
	for _, a := range items(events(b)) {
		if a.GetItem().GetDropped() {
			t.Error("b sees an old item fall")
		}
	}

	// After a minute they vanish.
	dropped := z.now - 100
	expired := 0
	for now := z.now + 50; now <= dropped+60000+100; now += 50 {
		z.stepAt(now)
		for _, e := range events(p) {
			if e.GetVanish().GetReason() == wathenav1.VanishReason_VANISH_REASON_EXPIRED {
				expired++
			}
		}
	}
	if expired != 2 {
		t.Errorf("%d items expired", expired)
	}
}

// dropOne kills the monster, which drops one item, with a second player
// two cells from it. It returns the item's entity.
func dropOne(t *testing.T) (*Zone, *Player, *Player, *entity) {
	t.Helper()
	z, p, m := lootZone(t, &progression.Mob{ID: 1002, Level: 1, MaxHP: 30, Drops: []progression.Drop{{Item: 909, Rate: 10000}}})
	b := add(z, "b")
	b.entity.x, b.entity.y = m.x-2, m.y
	z.stepAt(50)
	events(p)
	events(b)
	kill(t, z, p, m)
	events(b)
	var it *entity
	for e := range z.entities.all() {
		if e.item != nil {
			it = e
		}
	}
	if it == nil {
		t.Fatal("nothing dropped")
	}
	return z, p, b, it
}

func TestPickUp(t *testing.T) {
	z, p, b, it := dropOne(t)
	z.handle(command{player: p, seq: 2, pickUp: new(p.netIDs.id(it))})
	for start, now := z.now, z.now+50; now <= start+1000; now += 50 {
		z.stepAt(now)
		if !z.entities.has(it) {
			break
		}
	}
	if z.entities.has(it) || p.inventory.Count(909) != 1 {
		t.Fatalf("item still there: %v, inventory %v", z.entities.has(it), p.inventory)
	}
	// Both see the pick-up, then the item vanish. The picker gets it.
	for _, viewer := range []*Player{p, b} {
		var order []string
		for _, e := range events(viewer) {
			switch {
			case e.HasItemTaken():
				if e.GetItemTaken().GetSource() != viewer.netIDs.id(p.entity) {
					t.Errorf("%s: taken by %d", viewer.Name, e.GetItemTaken().GetSource())
				}
				order = append(order, "taken")
			case e.GetVanish().GetReason() == wathenav1.VanishReason_VANISH_REASON_PICKED_UP:
				order = append(order, "vanish")
			case e.HasInventoryAdd():
				if a := e.GetInventoryAdd(); a.GetItemId() != 909 || a.GetAmount() != 1 {
					t.Errorf("add %v", a)
				}
				order = append(order, "add")
			}
		}
		want := "taken vanish"
		if viewer == p {
			want += " add"
		}
		if got := join(order); got != want {
			t.Errorf("%s sees %q, want %q", viewer.Name, got, want)
		}
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += " "
		}
		out += v
	}
	return out
}

func failures(p *Player) []wathenav1.PickUpFailure {
	var out []wathenav1.PickUpFailure
	for _, e := range events(p) {
		if f := e.GetPickUpFailed(); f != nil {
			out = append(out, f.GetReason())
		}
	}
	return out
}

func TestPickUpNotYours(t *testing.T) {
	z, _, b, it := dropOne(t)
	// b did not fight, so it waits for all three looter times: 7 seconds.
	dropped := z.now
	z.handle(command{player: b, seq: 1, pickUp: new(b.netIDs.id(it))})
	for start, now := z.now, z.now+50; now <= start+1000; now += 50 {
		z.stepAt(now)
	}
	if f := failures(b); len(f) != 1 || f[0] != wathenav1.PickUpFailure_PICK_UP_FAILURE_NOT_YOURS {
		t.Fatalf("failures %v", f)
	}
	if !z.entities.has(it) || b.inventory.Count(909) != 0 {
		t.Fatal("b took the item")
	}
	for now := z.now + 50; now < dropped+7000; now += 50 {
		z.stepAt(now)
	}
	z.handle(command{player: b, seq: 2, pickUp: new(b.netIDs.id(it))})
	z.stepAt(dropped + 7050)
	if z.entities.has(it) || b.inventory.Count(909) != 1 {
		t.Errorf("b could not take the item after 7 seconds")
	}
}

func TestPickUpGone(t *testing.T) {
	z, p, b, it := dropOne(t)
	id := b.netIDs.id(it)
	// p takes the item first.
	z.handle(command{player: p, seq: 2, pickUp: new(p.netIDs.id(it))})
	z.stepAt(z.now + 50)
	if p.inventory.Count(909) != 1 {
		t.Fatal("p did not take the item")
	}
	// b asks in the same tick, before it sees the item vanish.
	z.handle(command{player: b, seq: 1, pickUp: new(id)})
	z.stepAt(z.now + 50)
	f := failures(b)
	if len(f) != 1 || f[0] != wathenav1.PickUpFailure_PICK_UP_FAILURE_GONE {
		t.Errorf("failures %v", f)
	}
	// An ID the player never saw is gone too.
	z.handle(command{player: b, seq: 2, pickUp: new(uint32(999))})
	z.stepAt(z.now + 50)
	if f := failures(b); len(f) != 1 || f[0] != wathenav1.PickUpFailure_PICK_UP_FAILURE_GONE {
		t.Errorf("unknown ID: %v", f)
	}
}

func TestDamageLog(t *testing.T) {
	z, p, m := lootZone(t, nil)
	z.stepAt(50)
	events(p)
	// A miss still puts the player in the log.
	z.logDamage(p.entity, m, 0)
	z.logDamage(m, p.entity, 7)
	l := m.monster.dmglog.Entries
	if len(l) != 1 || l[0].CharID != p.charID || l[0].Damage != 0 || l[0].Tanked != 7 {
		t.Errorf("%+v", l)
	}
}
