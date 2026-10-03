package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Hand tests of the Hunter's skills: the traps, the falcon, the
// automatic Blitz Beat and Beast Strafing's combo. The zones are golden
// zones, so every roll is lowest and every chance succeeds.

const (
	itemBow    = 1701 // Bow
	itemArrow  = 1750 // Arrow
	mobLunatic = 1063
	mobHornet  = 1004
	mobPoring  = 1002
)

// hunterSkills are a Hunter's skills at their highest levels.
var hunterSkills = [][2]int{
	{skill.IDBasic, 9}, {skill.IDOwlEye, 10}, {skill.IDVultureEye, 10}, {skill.IDConcentration, 10},
	{skill.IDDoubleStrafe, 10}, {skill.IDArrowShower, 10}, {skill.IDSkidTrap, 5}, {skill.IDLandMine, 5},
	{skill.IDAnkleSnare, 5}, {skill.IDShockwave, 5}, {skill.IDSandman, 5}, {skill.IDFlasher, 5},
	{skill.IDFreezingTrap, 5}, {skill.IDBlastMine, 5}, {skill.IDClaymoreTrap, 5}, {skill.IDRemoveTrap, 1},
	{skill.IDTalkieBox, 1}, {skill.IDBeastBane, 10}, {skill.IDFalcon, 1}, {skill.IDSteelCrow, 10},
	{skill.IDBlitzBeat, 5}, {skill.IDDetecting, 4}, {skill.IDSpringTrap, 5}, {skill.IDBeastStrafe, 1},
	{skill.IDPhantasmic, 1},
}

// hunterZone is a golden zone of a mode with a level 99 Hunter (job
// level 50) holding a bow, arrows and Booby Traps, and the given
// monsters.
func hunterZone(t *testing.T, mode reference.Mode, job int, mobs ...uint32) (*Zone, *Player) {
	t.Helper()
	z := goldenZone(t, mode, mobs...)
	skills := hunterSkills
	if job == 4012 {
		skills = append(skills, [2]int{skill.IDTrueSight, 10}, [2]int{skill.IDFalconAssault, 5},
			[2]int{skill.IDSharpShooting, 5}, [2]int{skill.IDWindWalk, 10})
	}
	p := goldenPlayerIn(t, z, goldenPlayer{Job: job, Build: "ranged", Sex: 1, JobLevel: 50, Skills: skills}, itemBow, itemArrow)
	give(t, z, p, itemTrap, 50)
	p.sp = 10000
	for e := range z.entities.all() {
		if e.monster != nil {
			// The monsters never move and never die.
			e.monster.spawn.CanMove = false
			if e.status != nil {
				// The monster AI does not move a monster without can_move.
				e.status.Mode &^= 1
			}
			e.hp = 1 << 40
		}
	}
	return z, p
}

// trapGroup is the zone's only group of a skill.
func trapGroup(z *Zone, id int) *unitGroup {
	for _, g := range z.groups {
		if g.def.ID == id {
			return g
		}
	}
	return nil
}

// placeTrap casts a trap two cells east of the player and runs the cast.
func placeTrap(t *testing.T, z *Zone, p *Player, id, lv int) *unitGroup {
	t.Helper()
	x, y := p.entity.x+2, p.entity.y
	castAt(z, p, id, lv, x, y)
	run(z, p, z.now+50, z.now+2000)
	g := trapGroup(z, id)
	if g == nil {
		t.Fatalf("no trap of skill %d: failures %v", id, skillFailures(events(p)))
	}
	return g
}

// Falcon skills need the falcon (skill_check_condition_castbegin's
// ST_FALCON), and pc_setfalcon needs Falconry Mastery.
func TestFalconRequirement(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	m := monsterOf(z, mobLunatic)
	cast(z, p, skill.IDBlitzBeat, 5, m)
	ev := run(z, p, z.now+50, z.now+500)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("Blitz Beat without a falcon: failures %v", f)
	}
	z.setFalcon(p, true)
	if !isFalcon(p) || p.record.Options&companion.OptionFalcon == 0 {
		t.Fatal("no falcon after pc_setfalcon")
	}
	cast(z, p, skill.IDBlitzBeat, 5, m)
	ev = run(z, p, z.now+50, z.now+3000)
	if hits := skillDamage(ev, skill.IDBlitzBeat); len(hits) != 1 {
		t.Fatalf("Blitz Beat hits %d, failures %v", len(hits), skillFailures(ev))
	}
	z.setFalcon(p, false)
	if isFalcon(p) {
		t.Error("falcon kept")
	}
	// Without Falconry Mastery there is no falcon.
	delete(p.char.Skills, skill.IDFalcon)
	z.recalc(p)
	z.setFalcon(p, true)
	if isFalcon(p) {
		t.Error("falcon without Falconry Mastery")
	}
}

// skill_additional_effect: a normal attack with a bow and a falcon may
// cast Blitz Beat at a level of (job level + 9) / 10, at most the
// learned level.
func TestAutoBlitzBeat(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	m := monsterOf(z, mobLunatic)
	z.strike(p.entity, m)
	if len(z.hits) != 1 {
		t.Fatalf("%d hits without a falcon, want the normal attack only", len(z.hits))
	}
	z.hits = nil
	z.setFalcon(p, true)
	p.progress.JobLevel = 30
	z.strike(p.entity, m)
	var blitz []hit
	for _, h := range z.hits {
		if h.skill != nil && h.skill.ID == skill.IDBlitzBeat {
			blitz = append(blitz, h)
		}
	}
	if len(blitz) != 1 {
		t.Fatalf("%d automatic Blitz Beats, want 1", len(blitz))
	}
	// Level 3 of 5: 3 hits.
	if got := blitz[0].result.Hits; got != 3 {
		t.Errorf("automatic Blitz Beat hits %d, want 3 (level 3)", got)
	}
}

// Land Mine (UNT_LANDMINE): an enemy that comes within a cell sets it
// off; it hits that enemy, the trap is used for 1.5 seconds, and the
// cast took a Booby Trap.
func TestLandMine(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	m := monsterOf(z, mobLunatic)
	m.x, m.y = 30, 30
	traps := z.countItem(p, itemTrap)
	g := placeTrap(t, z, p, skill.IDLandMine, 5)
	if z.countItem(p, itemTrap) != traps-1 {
		t.Errorf("Booby Traps %d, want %d", z.countItem(p, itemTrap), traps-1)
	}
	trap := g.units[0]
	m.x, m.y = trap.x+1, trap.y
	ev := run(z, p, z.now+50, z.now+300)
	if hits := skillDamage(ev, skill.IDLandMine); len(hits) != 1 {
		t.Fatalf("Land Mine hits %d", len(hits))
	}
	if !trapUsed(g) {
		t.Error("the trap was not used")
	}
	run(z, p, z.now+50, z.now+1600)
	if trapGroup(z, skill.IDLandMine) != nil {
		t.Error("the used trap stayed")
	}
}

// Traps may not be set next to a unit (UF_NOFOOTSET) or another trap
// (UF_NOREITERATION and skill_check_unit_range_sub).
func TestTrapPlacement(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	m := monsterOf(z, mobLunatic)
	m.x, m.y = 30, 30
	castAt(z, p, skill.IDLandMine, 1, p.entity.x+1, p.entity.y)
	run(z, p, z.now+50, z.now+2000)
	if trapGroup(z, skill.IDLandMine) != nil {
		t.Fatal("a trap next to the caster")
	}
	placeTrap(t, z, p, skill.IDLandMine, 1)
	castAt(z, p, skill.IDSandman, 1, p.entity.x+3, p.entity.y)
	run(z, p, z.now+50, z.now+2000)
	if trapGroup(z, skill.IDSandman) != nil {
		t.Error("a trap next to another trap")
	}
}

// Ankle Snare (UNT_ANKLESNARE): the enemy is held on the trap's cell for
// Duration2 * (1 - AGI / 200), at least 30 * (caster level + 100), and
// the trap lasts as long. Remove Trap then gives no item back.
func TestAnkleSnare(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p := hunterZone(t, mode, 11, mobHornet)
			m := monsterOf(z, mobHornet)
			m.x, m.y = 30, 30
			g := placeTrap(t, z, p, skill.IDAnkleSnare, 5)
			trap := g.units[0]
			m.x, m.y = trap.x+1, trap.y+1
			run(z, p, z.now+50, z.now+100)
			s := m.sc.Get(sc.SCAnkle)
			if s == nil {
				t.Fatal("no SC_ANKLE")
			}
			if m.x != trap.x || m.y != trap.y {
				t.Errorf("held at %d,%d, want the trap's cell %d,%d", m.x, m.y, trap.x, trap.y)
			}
			agi := int64(m.status.Agi)
			want := max(20000*agi/-200+20000, int64(30*(99+100)))
			if got := s.Remaining(z.now); got > want || got < want-200 {
				t.Errorf("held %d ms, want %d", got, want)
			}
			if g.trap.caught != m || g.interval != -1 {
				t.Error("the trap does not hold the monster")
			}
			traps := z.countItem(p, itemTrap)
			p.entity.x, p.entity.y = trap.x-2, trap.y
			cast(z, p, skill.IDRemoveTrap, 1, trap)
			run(z, p, z.now+50, z.now+500)
			if trapGroup(z, skill.IDAnkleSnare) != nil {
				t.Fatal("Remove Trap left the trap")
			}
			if z.countItem(p, itemTrap) != traps {
				t.Error("a holding Ankle Snare gave its item back")
			}
			if m.sc.Has(sc.SCAnkle) {
				t.Error("the monster is still held after the trap went")
			}
		})
	}
}

// Skid Trap (UNT_SKIDTRAP): the enemy slides away from the caster's cell,
// and in renewal stops (SC_STOP).
func TestSkidTrap(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	m := monsterOf(z, mobLunatic)
	m.x, m.y = 35, 35
	g := placeTrap(t, z, p, skill.IDSkidTrap, 5)
	trap := g.units[0]
	m.x, m.y = trap.x+1, trap.y
	run(z, p, z.now+50, z.now+100)
	// The caster stood 2 cells west: the monster slides 10 cells east.
	if m.x != trap.x+11 || m.y != trap.y {
		t.Errorf("monster at %d,%d, want %d,%d", m.x, m.y, trap.x+11, trap.y)
	}
	if !m.sc.Has(sc.SCStop) {
		t.Error("no SC_STOP in renewal")
	}
	if !trapUsed(g) {
		t.Error("the trap was not used")
	}
}

// Sandman (UNT_SANDMAN) puts the enemies around the unit that set it off
// to sleep; Flasher blinds that unit; Freezing Trap hits and freezes.
func TestStatusTraps(t *testing.T) {
	cases := []struct {
		id, status int
		damage     bool
	}{
		{skill.IDSandman, sc.SCSleep, false},
		{skill.IDFlasher, sc.SCBlind, false},
		{skill.IDFreezingTrap, sc.SCFreeze, true},
	}
	for _, c := range cases {
		z, p := hunterZone(t, reference.Renewal, 11, mobLunatic, mobLunatic)
		m := monsterOf(z, mobLunatic)
		var other *entity
		for e := range z.entities.all() {
			if e.monster != nil && e != m {
				other = e
			}
		}
		m.x, m.y = 35, 35
		other.x, other.y = 35, 30
		g := placeTrap(t, z, p, c.id, 5)
		trap := g.units[0]
		m.x, m.y = trap.x+1, trap.y
		other.x, other.y = trap.x+2, trap.y+1
		ev := run(z, p, z.now+50, z.now+1500)
		if !m.sc.Has(c.status) {
			t.Errorf("skill %d: no status %d on the unit that set it off", c.id, c.status)
		}
		if c.id == skill.IDSandman && !other.sc.Has(sc.SCSleep) {
			t.Error("Sandman did not reach 2 cells from the unit that set it off")
		}
		if c.id == skill.IDFlasher && other.sc.Has(sc.SCBlind) {
			t.Error("Flasher reached another unit")
		}
		if got := len(skillDamage(ev, c.id)) > 0; got != c.damage {
			t.Errorf("skill %d: damage %v, want %v", c.id, got, c.damage)
		}
	}
}

// Shockwave Trap (SkillShockwaveTrap) takes 15% per level + 5% of the
// target's maximum SP.
func TestShockwaveDrainsSP(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11)
	q := add(z, "target")
	q.entity.x, q.entity.y = p.entity.x+3, p.entity.y
	z.recalc(q)
	q.sp = int64(q.entity.status.MaxSP)
	d := z.Skills.Get(skill.IDShockwave)
	shockwaveTrap{}.additionalEffect(z, p.entity, q.entity, d, 5)
	want := int64(q.entity.status.MaxSP) - max(int64(q.entity.status.MaxSP)*80/100, 1)
	if q.sp != max(want, 0) {
		t.Errorf("SP %d, want %d", q.sp, want)
	}
}

// Blast Mine and Claymore Trap hit every enemy around the trap, and split
// their damage in renewal; Claymore Trap uses the traps around it.
func TestBlastMineAndClaymore(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic, mobLunatic)
	var mobs []*entity
	for e := range z.entities.all() {
		if e.monster != nil {
			mobs = append(mobs, e)
			e.x, e.y = 35, 30+len(mobs)*3
		}
	}
	g := placeTrap(t, z, p, skill.IDClaymoreTrap, 5)
	trap := g.units[0]
	p.entity.x += 3
	other := placeTrap(t, z, p, skill.IDLandMine, 1)
	if other.units[0].x != trap.x+3 {
		t.Fatalf("land mine at %d", other.units[0].x)
	}
	mobs[0].x, mobs[0].y = trap.x, trap.y+1
	mobs[1].x, mobs[1].y = trap.x-1, trap.y-2
	ev := run(z, p, z.now+50, z.now+300)
	if hits := skillDamage(ev, skill.IDClaymoreTrap); len(hits) != 2 {
		t.Fatalf("Claymore hits %d, want 2", len(hits))
	}
	if !trapUsed(g) {
		t.Error("Claymore not used")
	}
	// The Land Mine 3 cells away is outside the 2 cell splash.
	if trapUsed(other) {
		t.Error("Claymore used a trap outside its splash")
	}
}

// skill_unit_timer_sub: a trap whose time runs out without going off
// leaves its Booby Trap on its cell.
func TestTrapExpiresIntoItem(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11)
	g := placeTrap(t, z, p, skill.IDSandman, 5)
	trap := g.units[0]
	run(z, p, z.now+50, z.now+int64(g.limit)+200)
	if trapGroup(z, skill.IDSandman) != nil {
		t.Fatal("the trap stayed")
	}
	found := false
	for e := range z.entities.all() {
		if e.item != nil && e.item.id == itemTrap && abs(e.x-trap.x) <= 1 && abs(e.y-trap.y) <= 1 {
			found = true
		}
	}
	if !found {
		t.Error("no Booby Trap on the trap's cell")
	}
}

// Remove Trap (SkillRemoveTrap) gives one Booby Trap back for a trap
// that did not go off. Spring Trap (SkillSpringTrap) needs the falcon and
// uses a trap up.
func TestRemoveAndSpringTrap(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11)
	g := placeTrap(t, z, p, skill.IDFlasher, 5)
	traps := z.countItem(p, itemTrap)
	cast(z, p, skill.IDRemoveTrap, 1, g.units[0])
	run(z, p, z.now+50, z.now+500)
	if trapGroup(z, skill.IDFlasher) != nil {
		t.Fatal("Remove Trap left the trap")
	}
	if z.countItem(p, itemTrap) != traps+1 {
		t.Errorf("Booby Traps %d, want %d", z.countItem(p, itemTrap), traps+1)
	}
	g = placeTrap(t, z, p, skill.IDFlasher, 5)
	cast(z, p, skill.IDSpringTrap, 5, g.units[0])
	if f := skillFailures(run(z, p, z.now+50, z.now+500)); len(f) != 1 {
		t.Fatalf("Spring Trap without a falcon: failures %v", f)
	}
	z.setFalcon(p, true)
	cast(z, p, skill.IDSpringTrap, 5, g.units[0])
	run(z, p, z.now+50, z.now+500)
	if !trapUsed(g) {
		t.Fatal("Spring Trap did not set off the trap")
	}
	run(z, p, z.now+50, z.now+1600)
	if trapGroup(z, skill.IDFlasher) != nil {
		t.Error("the sprung trap stayed")
	}
}

// Renewal's traps are hidden from others (traps_setting 2), and Detect
// shows them (skill_reveal_trap_inarea) and ends hiding around its cell.
func TestDetectRevealsTraps(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11)
	q := add(z, "viewer")
	q.entity.x, q.entity.y = p.entity.x, p.entity.y+5
	z.recalc(q)
	events(q)
	g := placeTrap(t, z, p, skill.IDSandman, 5)
	sees := func() bool {
		for _, e := range events(q) {
			if a := e.GetAppear(); a != nil && a.GetKind() == wathenav1.EntityKind_ENTITY_KIND_SKILL_UNIT {
				return true
			}
		}
		_, ok := q.netIDs.get(g.units[0])
		return ok
	}
	z.stepAt(z.now + 50)
	if sees() {
		t.Fatal("another player sees a hidden trap")
	}
	if _, ok := p.netIDs.get(g.units[0]); !ok {
		t.Error("the caster does not see its trap")
	}
	// The other player hides next to the trap.
	z.statusEngine().Start(z.unit(q.entity), z.unit(q.entity), sc.Start{Type: sc.SCHiding, Rate: 10000, Val1: 10,
		Duration: 60000, Flag: sc.NoAvoid, Reason: sc.ReasonScript})
	if !q.entity.sc.Has(sc.SCHiding) {
		t.Fatal("no hiding")
	}
	z.setFalcon(p, true)
	tx, ty := g.units[0].x, g.units[0].y
	castAt(z, p, skill.IDDetecting, 4, tx, ty+2)
	run(z, p, z.now+50, z.now+500)
	if g.trap.hidden {
		t.Fatal("Detect did not show the trap")
	}
	if !sees() {
		t.Error("the other player does not see the shown trap")
	}
	if q.entity.sc.Has(sc.SCHiding) {
		t.Error("Detect did not end hiding")
	}
}

// Talkie Box (UNT_TALKIEBOX) goes off for any other player, never for its
// caster, and stays 5 seconds.
func TestTalkieBox(t *testing.T) {
	z, p := hunterZone(t, reference.PreRenewal, 11)
	g := placeTrap(t, z, p, skill.IDTalkieBox, 1)
	p.entity.x = g.units[0].x - 1
	run(z, p, z.now+50, z.now+1200)
	if trapUsed(g) {
		t.Fatal("the caster set off its Talkie Box")
	}
	q := add(z, "walker")
	q.entity.x, q.entity.y = g.units[0].x+1, g.units[0].y
	z.recalc(q)
	run(z, p, z.now+50, z.now+1200)
	if !trapUsed(g) || !g.trap.spoken {
		t.Fatal("another player did not set off the Talkie Box")
	}
}

// Beast Strafing (HT_POWER) needs Double Strafe's combo window (SC_COMBO
// of skill_combo), and hits only brutes, insects and Doram.
func TestBeastStrafingCombo(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic, mobPoring)
	brute, plant := monsterOf(z, mobLunatic), monsterOf(z, mobPoring)
	brute.x, brute.y = p.entity.x+3, p.entity.y
	plant.x, plant.y = p.entity.x, p.entity.y+3
	cast(z, p, skill.IDBeastStrafe, 1, brute)
	ev := run(z, p, z.now+50, z.now+500)
	if len(skillDamage(ev, skill.IDBeastStrafe)) != 0 || len(skillFailures(ev)) != 0 {
		t.Fatalf("Beast Strafing without the combo: %v", ev)
	}
	cast(z, p, skill.IDDoubleStrafe, 10, brute)
	run(z, p, z.now+50, z.now+600)
	c := p.entity.sc.Get(sc.SCCombo)
	if c == nil || c.Val1 != skill.IDDoubleStrafe {
		t.Fatal("no combo window after Double Strafe")
	}
	run(z, p, z.now+50, z.now+600)
	cast(z, p, skill.IDBeastStrafe, 1, brute)
	ev = run(z, p, z.now+50, z.now+500)
	if len(skillDamage(ev, skill.IDBeastStrafe)) != 1 {
		t.Fatalf("Beast Strafing hits %d, failures %v", len(skillDamage(ev, skill.IDBeastStrafe)), skillFailures(ev))
	}
	if p.entity.sc.Has(sc.SCCombo) {
		t.Error("the combo window stayed after Beast Strafing")
	}
	// On a plant it costs the cast and deals nothing.
	cast(z, p, skill.IDDoubleStrafe, 10, plant)
	run(z, p, z.now+50, z.now+1200)
	cast(z, p, skill.IDBeastStrafe, 1, plant)
	ev = run(z, p, z.now+50, z.now+500)
	if len(skillDamage(ev, skill.IDBeastStrafe)) != 0 {
		t.Error("Beast Strafing hit a plant")
	}
}

// Phantasmic Arrow pushes its target 3 cells.
func TestPhantasmicArrowKnockback(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	m := monsterOf(z, mobLunatic)
	m.x, m.y = p.entity.x+4, p.entity.y
	cast(z, p, skill.IDPhantasmic, 1, m)
	// The monster walks back at once: look right after the hit.
	ev := run(z, p, z.now+50, z.now+100)
	if len(skillDamage(ev, skill.IDPhantasmic)) != 1 {
		t.Fatalf("Phantasmic Arrow hits: failures %v", skillFailures(ev))
	}
	if m.x != p.entity.x+7 {
		t.Errorf("monster %d cells away, want 7", m.x-p.entity.x)
	}
}
