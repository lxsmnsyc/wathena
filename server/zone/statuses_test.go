package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// testStatuses is renewal's status data with core's status scripts.
func testStatuses(t *testing.T) *sc.DB {
	t.Helper()
	core, err := testCore()
	if err != nil {
		t.Fatal(err)
	}
	db, _, err := core.LoadStatuses(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// statusZone makes a zone with job tables, renewal's statuses, core's
// item scripts, and a player two cells from a Poring.
func statusZone(t *testing.T) (*Zone, *Player, *entity) {
	t.Helper()
	mob := &battle.Status{MaxHP: 1000, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Level: 1, Vit: 1, Luk: 1, Int: 1,
		RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	spawns := []Spawn{{Mob: 1002, Name: "Poring", Amount: 1, X: 20, Y: 20, Speed: 400, Status: mob, RespawnMs: 60000}}
	z := New("map/test", open(40), rng.FromKey([32]byte{9}).Stream(), spawns)
	z.Items = item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 501, AegisName: "Red_Potion", Type: "Healing", Weight: 70, Stackable: true, Script: "core:scripts/items/Red_Potion.lua"},
		{ID: 506, AegisName: "Green_Potion", Type: "Healing", Weight: 70, Stackable: true, Script: "core:scripts/items/Green_Potion.lua"},
		{ID: 12041, AegisName: "Str_Dish01", Type: "Healing", Weight: 100, Stackable: true, Script: "core:scripts/items/Str_Dish01.lua"},
	}})
	z.Progress = testTables()
	z.Status, z.Start = jobTables(t), status.Character{Job: 0, Male: true, Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
	z.Statuses = testStatuses(t)
	coreMods(t, z)
	p := add(z, "a")
	p.maxWeight = maxWeight
	var m *entity
	for e := range z.entities.all() {
		if e.monster != nil {
			m = e
		}
	}
	p.entity.x, p.entity.y = m.x+2, m.y
	z.stepAt(50)
	events(p)
	return z, p, m
}

// statusIcons are the status icon events.
func statusIcons(ev []*wathenav1.Event) []*wathenav1.StatusIcon {
	var out []*wathenav1.StatusIcon
	for _, e := range ev {
		if s := e.GetStatusIcon(); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// startOn starts a status on an entity, as a script with no source does.
func startOn(z *Zone, e *entity, typ int, level int, ms int64) bool {
	u := z.unit(e)
	return z.statusEngine().Start(u, u, sc.Start{Type: typ, Rate: 10000, Val1: level, Duration: ms, Flag: sc.NoAvoid, Reason: sc.ReasonScript})
}

// timed gives a player timed bonuses from entries, as
// player:add_timed_bonuses does.
func timed(t *testing.T, z *Zone, p *Player, tb modscript.TimedBonuses, entries ...any) {
	t.Helper()
	for _, e := range entries {
		b, err := bonus.FromData(e, z.bonusNames())
		if err != nil {
			t.Fatal(err)
		}
		tb.Bonuses = append(tb.Bonuses, *b)
	}
	tb.Data = entries
	if err := z.addTimedBonuses(p, tb); err != nil {
		t.Fatal(err)
	}
}

// A STR dish starts SC_STRFOOD: STR rises, the icon shows with its time,
// and both go when it ends.
func TestFoodRaisesStr(t *testing.T) {
	z, p, _ := statusZone(t)
	z.addItem(p, 12041, 1)
	events(p)
	str := p.entity.status.Str
	use(z, p, 0)
	z.stepAt(z.now + 50)
	if !p.entity.sc.Has(sc.SCStrfood) {
		t.Fatal("the dish started no status")
	}
	if p.entity.status.Str != str+1 {
		t.Errorf("STR %d, want %d", p.entity.status.Str, str+1)
	}
	ev := events(p)
	icons := statusIcons(ev)
	icon := z.Statuses.Get(sc.SCStrfood).Icon
	if len(icons) != 1 || icons[0].GetIcon() != uint32(icon) || !icons[0].GetOn() || icons[0].GetRemainingMs() != 1200000 {
		t.Errorf("icons %v", icons)
	}
	if got := stats(ev); got[wathenav1.StatId_STAT_ID_STR] == 0 {
		t.Errorf("STR not sent: %v", got)
	}
	z.stepAt(z.now + 1200000)
	if p.entity.sc.Has(sc.SCStrfood) || p.entity.status.Str != str {
		t.Errorf("after the end: STR %d, active %v", p.entity.status.Str, p.entity.sc.Has(sc.SCStrfood))
	}
	if icons := statusIcons(events(p)); len(icons) != 1 || icons[0].GetOn() {
		t.Errorf("end icons %v", icons)
	}
}

// A Green Potion ends Poison.
func TestGreenPotionCuresPoison(t *testing.T) {
	z, p, _ := statusZone(t)
	z.addItem(p, 506, 1)
	if !startOn(z, p.entity, sc.SCPoison, 1, 60000) {
		t.Fatal("poison did not start")
	}
	use(z, p, 0)
	z.stepAt(z.now + 50)
	if p.entity.sc.Has(sc.SCPoison) {
		t.Error("the potion did not cure poison")
	}
}

// Poison takes HP every second, and shows to others as an option.
func TestPoisonTicksInZone(t *testing.T) {
	z, p, _ := statusZone(t)
	p.entity.hp = int64(p.entity.status.MaxHP)
	hp := p.entity.hp
	startOn(z, p.entity, sc.SCPoison, 1, 10000)
	z.stepAt(z.now + 50)
	var change *wathenav1.OptionChange
	for _, e := range events(p) {
		if c := e.GetOptionChange(); c != nil {
			change = c
		}
	}
	if change == nil || change.GetOptions().GetHealth()&1 == 0 {
		t.Errorf("option change %v", change)
	}
	for range 25 {
		z.stepAt(z.now + 50)
	}
	if p.entity.hp >= hp {
		t.Errorf("HP %d, was %d", p.entity.hp, hp)
	}
}

// A stunned player cannot walk or attack.
func TestStunStopsActions(t *testing.T) {
	z, p, m := statusZone(t)
	startOn(z, p.entity, sc.SCStun, 1, 3000)
	x, y := p.entity.x, p.entity.y
	z.handle(command{player: p, walkTo: &[2]int{x - 5, y}})
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: p.netIDs.id(m), repeat: true}})
	for range 20 {
		z.stepAt(z.now + 50)
	}
	if p.entity.x != x || p.entity.y != y || m.hp != int64(m.status.MaxHP) {
		t.Errorf("at %d,%d (was %d,%d), monster HP %d", p.entity.x, p.entity.y, x, y, m.hp)
	}
	z.stepAt(z.now + 3000)
	z.handle(command{player: p, walkTo: &[2]int{x - 5, y}})
	for range 20 {
		z.stepAt(z.now + 50)
	}
	if p.entity.x == x {
		t.Error("the player cannot walk after stun")
	}
}

// Statuses outlive a logout and come back on login with their time left.
// Decrease AGI has NoSave, so it ends at logout.
func TestStatusesSurviveRelog(t *testing.T) {
	z, _, _ := statusZone(t)
	s := &saves{}
	c := savedCharacter()
	c.Items = nil
	p := addCharacter(z, c, s)
	startOn(z, p.entity, sc.SCBlessing, 10, 60000)
	startOn(z, p.entity, sc.SCDecreaseagi, 1, 60000)
	timed(t, z, p, modscript.TimedBonuses{Key: "k", Milliseconds: 30000, EndsOn: []string{"death"}, Icon: -1}, []any{"str", int64(5)})
	z.stepAt(z.now + 1000)
	str := p.entity.status.Str
	z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	saved := s.got[len(s.got)-1]
	if len(saved.Statuses) != 1 || saved.Statuses[0].Name != "blessing" || saved.Statuses[0].Values[0] != 10 || saved.Statuses[0].Tick != 59000 {
		t.Fatalf("saved statuses %+v", saved.Statuses)
	}
	if len(saved.TimedBonuses) != 1 || saved.TimedBonuses[0].Remaining != 29000 || saved.TimedBonuses[0].Bonuses != `[["str",5]]` {
		t.Fatalf("saved timed bonuses %+v", saved.TimedBonuses)
	}

	z2, _, _ := statusZone(t)
	q := addCharacter(z2, saved, &saves{})
	z2.stepAt(z2.now + 50)
	x := q.entity.sc.Get(sc.SCBlessing)
	if x == nil || x.Reason != sc.ReasonLogin || x.Remaining(z2.now) > 59000 || x.Remaining(z2.now) < 58900 {
		t.Fatalf("restored %+v", x)
	}
	if q.entity.sc.Has(sc.SCDecreaseagi) {
		t.Error("Decrease AGI came back")
	}
	if q.entity.status.Str != str {
		t.Errorf("STR %d after login, %d before", q.entity.status.Str, str)
	}
	if got := z2.snapshot(q); len(got.Statuses) != 1 || len(got.TimedBonuses) != 1 {
		t.Errorf("snapshot %+v %+v", got.Statuses, got.TimedBonuses)
	}
}

// A cause_status bonus stuns the monster a hit lands on.
func TestCauseStatusOnHit(t *testing.T) {
	z, p, m := statusZone(t)
	timed(t, z, p, modscript.TimedBonuses{Key: "stun", Milliseconds: 600000, Icon: -1},
		[]any{"cause_status", "stun", map[string]any{"chance": int64(10000)}})
	z.stepAt(z.now + 50)
	if len(p.bonuses.AddEff) != 1 {
		t.Fatalf("procs %+v", p.bonuses.AddEff)
	}
	m.status.Mode = 0
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: p.netIDs.id(m), repeat: false}})
	stunned := false
	for range 60 {
		z.stepAt(z.now + 50)
		if m.sc.Has(sc.SCStun) {
			stunned = true
			break
		}
	}
	if !stunned {
		t.Errorf("monster HP %d of %d, not stunned", m.hp, m.status.MaxHP)
	}
}
