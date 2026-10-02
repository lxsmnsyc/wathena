package statuschange_test

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// testPlayer is a level 50 player with 1,000 HP and 100 SP.
func testPlayer() *unit {
	u := newUnit(sc.KindPlayer, battle.Status{Level: 50, MaxHP: 1000, MaxSP: 100, Vit: 1, Int: 1, Luk: 1, Agi: 1, Dex: 1, Str: 1})
	u.id = 1
	return u
}

func newEngine(t *testing.T, mode reference.Mode) (*sc.Engine, *env) {
	t.Helper()
	e := &env{}
	return sc.New(loadDB(t, mode), e), e
}

func TestStartAndExpire(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	if !en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 10, Duration: 60000, Flag: sc.NoAvoid, Reason: sc.ReasonItem}) {
		t.Fatal("blessing did not start")
	}
	x := p.list.Get(sc.SCBlessing)
	if x == nil || x.Val1 != 10 || x.Reason != sc.ReasonItem || x.Remaining(0) != 60000 {
		t.Fatalf("entry %+v", x)
	}
	if e.recalc == 0 || len(e.icons) != 1 || len(e.started) != 1 {
		t.Errorf("recalc %d, icons %v, started %v", e.recalc, e.icons, e.started)
	}
	e.now = 59999
	en.Advance(e.now)
	if !p.list.Has(sc.SCBlessing) {
		t.Fatal("blessing ended early")
	}
	e.now = 60000
	en.Advance(e.now)
	if p.list.Has(sc.SCBlessing) {
		t.Fatal("blessing did not end")
	}
	if !slices.Equal(e.ended, []string{"30:expired"}) || len(e.icons) != 2 {
		t.Errorf("ended %v, icons %v", e.ended, e.icons)
	}
	if en.Next() != -1 {
		t.Errorf("a timer is left at %d", en.Next())
	}
}

func TestHigherLevelStays(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 10, Duration: 60000, Flag: sc.NoAvoid})
	// A weaker blessing reports success and changes nothing.
	if !en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 5, Duration: 90000, Flag: sc.NoAvoid}) {
		t.Error("the weaker start failed")
	}
	if x := p.list.Get(sc.SCBlessing); x.Val1 != 10 || x.Remaining(0) != 60000 {
		t.Errorf("entry %+v", x)
	}
	// The same level starts again with the new duration.
	e.now = 1000
	en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 10, Duration: 90000, Flag: sc.NoAvoid})
	if x := p.list.Get(sc.SCBlessing); x.Remaining(e.now) != 90000 {
		t.Errorf("remaining %d", x.Remaining(e.now))
	}
	e.now = 61000
	en.Advance(e.now)
	if !p.list.Has(sc.SCBlessing) {
		t.Error("the old timer ended the new blessing")
	}
}

func TestPoisonTicks(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	p.hp = 1000
	if !en.Start(p, p, sc.Start{Type: sc.SCPoison, Rate: 10000, Val1: 1, Duration: 5000, Flag: sc.NoAvoid}) {
		t.Fatal("poison did not start")
	}
	if p.list.Opt2&0x1 == 0 { // OPT2_POISON
		t.Errorf("opt2 %#x", p.list.Opt2)
	}
	if hp, sp := p.list.Regen(); hp || sp {
		t.Error("poison lets HP regen run")
	}
	for e.now = 0; e.now <= 6000; e.now += 100 {
		en.Advance(e.now)
	}
	// status_change_timer: 2 + 3/200 of max HP every second, for a
	// player, the last one too.
	if want := []int64{17, 17, 17, 17, 17}; !slices.Equal(e.damage, want) {
		t.Errorf("damage %v, want %v", e.damage, want)
	}
	if p.list.Has(sc.SCPoison) || p.list.Opt2 != 0 {
		t.Errorf("poison still on: opt2 %#x", p.list.Opt2)
	}
	if len(e.ticked) != 5 {
		t.Errorf("ticks %v", e.ticked)
	}
}

func TestPoisonStopsAtQuarterHP(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	p.hp = 260
	en.Start(p, p, sc.Start{Type: sc.SCPoison, Rate: 10000, Val1: 1, Duration: 10000, Flag: sc.NoAvoid})
	for e.now = 0; e.now <= 10000; e.now += 100 {
		en.Advance(e.now)
	}
	if p.hp != 260-17 {
		t.Errorf("hp %d", p.hp)
	}
}

func TestStunBlocksActions(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000, Flag: sc.NoAvoid})
	c := p.list.Cant
	if !c.Move || !c.Attack || !c.Cast || p.list.Opt1 != sc.Opt1Stun {
		t.Errorf("cant %+v, opt1 %d", c, p.list.Opt1)
	}
	e.now = 3000
	en.Advance(e.now)
	if p.list.Cant.Move || p.list.Opt1 != sc.Opt1None {
		t.Errorf("after stun: cant %+v, opt1 %d", p.list.Cant, p.list.Opt1)
	}
}

func TestOpt1Replaces(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000, Flag: sc.NoAvoid})
	en.Start(p, p, sc.Start{Type: sc.SCSleep, Rate: 10000, Val1: 1, Duration: 3000, Flag: sc.NoAvoid})
	if p.list.Has(sc.SCStun) || !p.list.Has(sc.SCSleep) || p.list.Opt1 != sc.Opt1Sleep {
		t.Errorf("stun %v, sleep %v, opt1 %d", p.list.Has(sc.SCStun), p.list.Has(sc.SCSleep), p.list.Opt1)
	}
	if !slices.Equal(e.ended, []string{"2:replaced"}) {
		t.Errorf("ended %v", e.ended)
	}
}

func TestDamageEndsSleep(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCSleep, Rate: 10000, Val1: 1, Duration: 30000, Flag: sc.NoAvoid})
	en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 1, Duration: 30000, Flag: sc.NoAvoid})
	en.Damaged(p, 10, true)
	if p.list.Has(sc.SCSleep) || !p.list.Has(sc.SCBlessing) {
		t.Errorf("sleep %v, blessing %v", p.list.Has(sc.SCSleep), p.list.Has(sc.SCBlessing))
	}
	if !slices.Equal(e.ended, []string{"3:damage"}) {
		t.Errorf("ended %v", e.ended)
	}
}

func TestDeathClears(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 1, Duration: 30000, Flag: sc.NoAvoid})
	en.Start(p, p, sc.Start{Type: sc.SCPoison, Rate: 10000, Val1: 1, Duration: 30000, Flag: sc.NoAvoid})
	en.Clear(p, sc.ClearDeath, sc.ReasonDeath)
	if !p.list.Empty() {
		t.Errorf("%d statuses left", p.list.Len())
	}
	if len(e.ended) != 2 || !slices.Contains(e.ended, "30:death") {
		t.Errorf("ended %v", e.ended)
	}
	// A dead unit takes no new status.
	p.hp = 0
	if en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 1, Duration: 30000, Flag: sc.NoAvoid}) {
		t.Error("a dead player got a status")
	}
}

func TestSaveAndLoad(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 10, Duration: 60000, Flag: sc.NoAvoid})
	en.Start(p, p, sc.Start{Type: sc.SCStrfood, Rate: 10000, Val1: 5, Duration: sc.InfiniteTick, Flag: sc.NoAvoid})
	en.Start(p, p, sc.Start{Type: sc.SCPoison, Rate: 10000, Val1: 1, Duration: 10000, Flag: sc.NoAvoid})
	e.now = 1500
	en.Advance(e.now)
	saved := en.Save(p)
	if len(saved) != 3 {
		t.Fatalf("saved %+v", saved)
	}
	byType := map[int]sc.Saved{}
	for _, s := range saved {
		byType[s.Type] = s
	}
	if s := byType[sc.SCBlessing]; s.Tick != 58500 || s.Val1 != 10 {
		t.Errorf("blessing %+v", s)
	}
	if s := byType[sc.SCStrfood]; s.Tick != sc.InfiniteTick {
		t.Errorf("food %+v", s)
	}
	// Poison saves the time to its next tick, and val4 the time after.
	if s := byType[sc.SCPoison]; s.Tick != 500 || s.Val4 != 8000 {
		t.Errorf("poison %+v", s)
	}

	en2, e2 := newEngine(t, reference.Renewal)
	q := testPlayer()
	q.hp = 1000
	for _, s := range saved {
		if !en2.Load(q, s, nil) {
			t.Errorf("load %+v failed", s)
		}
	}
	if x := q.list.Get(sc.SCBlessing); x == nil || x.Remaining(0) != 58500 || x.Reason != sc.ReasonLogin {
		t.Errorf("loaded blessing %+v", x)
	}
	if x := q.list.Get(sc.SCPoison); x == nil || x.Remaining(0) != 8500 {
		t.Errorf("loaded poison %+v", x)
	}
	e2.now = 500
	en2.Advance(e2.now)
	if len(e2.damage) != 1 {
		t.Errorf("loaded poison damage %v", e2.damage)
	}
}

func TestLogoutEndsNoSave(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCBlessing, Rate: 10000, Val1: 10, Duration: 60000, Flag: sc.NoAvoid})
	en.Start(p, p, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000, Flag: sc.NoAvoid})
	en.Logout(p)
	for _, x := range e.ended {
		if x != "2:logout" {
			t.Errorf("ended %v", e.ended)
		}
	}
	if !p.list.Has(sc.SCBlessing) {
		t.Error("logout ended a saved status")
	}
}

func TestDelayedStart(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	if !en.Start(p, p, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000, Flag: sc.NoAvoid, Delay: 100}) {
		t.Fatal("start failed")
	}
	if p.list.Has(sc.SCStun) {
		t.Fatal("started before the delay")
	}
	e.now = 100
	en.Advance(e.now)
	// Renewal takes the delay from the duration.
	if x := p.list.Get(sc.SCStun); x == nil || x.Remaining(e.now) != 2900 {
		t.Errorf("entry %+v", x)
	}
}

func TestBossImmunity(t *testing.T) {
	en, _ := newEngine(t, reference.Renewal)
	p := testPlayer()
	// MD_STATUSIMMUNE.
	boss := newUnit(sc.KindMonster, battle.Status{Level: 99, MaxHP: 100000, Mode: 0x4000000, Vit: 1})
	boss.id = 2
	if en.Start(p, boss, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000}) {
		t.Error("a status immune monster was stunned")
	}
	if !en.Start(p, boss, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000, Flag: sc.NoAvoid}) {
		t.Error("cannot_be_avoided did not pass immunity")
	}
}

func TestResistanceRolls(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	target := testPlayer()
	target.st.Vit = 99
	target.base.Vit = 99
	// The highest roll fails a chance below 100%.
	e.roll = 9999
	if en.Start(p, target, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000}) {
		t.Error("stun started through 99 VIT with the highest roll")
	}
	e.roll = 0
	if !en.Start(p, target, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000}) {
		t.Error("stun failed with the lowest roll")
	}
	// keep_duration keeps the full duration.
	en.End(target, sc.SCStun, sc.ReasonRemoved)
	en.Start(p, target, sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1, Duration: 3000, Flag: sc.NoTickDef})
	if x := target.list.Get(sc.SCStun); x == nil || x.Remaining(0) != 3000 {
		t.Errorf("keep_duration: %+v", x)
	}
}
