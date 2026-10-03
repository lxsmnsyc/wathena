package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Hand tests of the Monk and Champion skills, for what the recordings do
// not cover: spirit spheres and their timers, the combo chain, Blade
// Stop, Snap, Absorb Spirit Spheres, Zen and Asura Strike.

const (
	jobMonk     = 15
	jobChampion = 4016
)

// monkZone is skillZone with a Champion that learned every Monk and
// Champion skill, next to a Poring with a lot of HP that does not dodge.
func monkZone(t *testing.T) (*Zone, *Player, *entity) {
	t.Helper()
	z, p, m := skillZone(t, jobChampion, map[int]int{
		skill.IDBasic: 9, skill.IDIronHand: 10, skill.IDSpiritsRecovery: 5, skill.IDCallSpirits: 5,
		skill.IDAbsorbSpirits: 1, skill.IDTripleAttack: 10, skill.IDBodyRelocation: 1, skill.IDDodge: 10,
		skill.IDInvestigate: 5, skill.IDFingerOffensive: 5, skill.IDSteelBody: 5, skill.IDBladeStop: 5,
		skill.IDExplosionSpirits: 5, skill.IDExtremityFist: 5, skill.IDChainCombo: 5, skill.IDComboFinish: 5,
		skill.IDPalmStrike: 5, skill.IDTigerFist: 5, skill.IDChainCrush: 10, skill.IDSoulCollect: 1,
	})
	m.status = &battle.Status{MaxHP: 1000000, Level: 10, Vit: 1, Luk: 1, Int: 1, Agi: 1, Flee: 1, ADelay: 2000,
		AMotion: 500, DMotion: 300, RHW: battle.Weapon{Atk: 1, Atk2: 1, Range: 1}}
	m.hp = 1000000
	p.entity.x, p.entity.y = m.x+1, m.y
	return z, p, m
}

// giveSpheres gives a player spheres that last ten minutes.
func giveSpheres(z *Zone, p *Player, n int) {
	for range n {
		z.addSpiritBall(p, 600000, maxSpiritBall)
	}
}

// TestSpiritSphereTimers checks pc_addspiritball and pc_spiritball_timer:
// at the maximum the oldest sphere goes first, and each sphere ends with
// its own timer.
func TestSpiritSphereTimers(t *testing.T) {
	z, p, _ := monkZone(t)
	z.now = 1000
	z.addSpiritBall(p, 5000, 2)
	z.addSpiritBall(p, 3000, 2)
	if got := p.spiritTimers; len(got) != 2 || got[0] != 4000 || got[1] != 6000 {
		t.Fatalf("timers %v, want [4000 6000]", got)
	}
	// A third at the maximum of 2 removes the oldest timer, the first.
	z.addSpiritBall(p, 10000, 2)
	if got := p.spiritTimers; len(got) != 2 || got[0] != 6000 || got[1] != 11000 {
		t.Fatalf("timers %v, want [6000 11000]", got)
	}
	run(z, p, 5000, 6000)
	if p.spiritBalls() != 1 {
		t.Fatalf("%d spheres after the first timer, want 1", p.spiritBalls())
	}
	if got := p.entity.status.Player.SpiritBall; got != 1 {
		t.Errorf("battle status has %d spheres, want 1", got)
	}
	run(z, p, 6050, 11000)
	if p.spiritBalls() != 0 {
		t.Errorf("%d spheres after every timer, want 0", p.spiritBalls())
	}
}

// TestCallSpiritsAndZen checks SkillSummoningSpiritSphere (one sphere up
// to the level, and the castbegin check at the level) and SkillZen.
func TestCallSpiritsAndZen(t *testing.T) {
	z, p, _ := monkZone(t)
	now := int64(100)
	for i := 1; i <= 2; i++ {
		cast(z, p, skill.IDCallSpirits, 2, nil)
		run(z, p, now, now+3000)
		now += 3050
		if p.spiritBalls() != i {
			t.Fatalf("%d spheres after %d casts, want %d", p.spiritBalls(), i, i)
		}
	}
	sp := p.sp
	cast(z, p, skill.IDCallSpirits, 2, nil)
	ev := run(z, p, now, now+3000)
	now += 3050
	if p.spiritBalls() != 2 || p.sp < sp || len(skillFailures(ev)) != 1 {
		t.Fatalf("a cast at the level's maximum: %d spheres, SP %d of %d, failures %v", p.spiritBalls(), p.sp, sp,
			skillFailures(ev))
	}
	// Spheres last skill_get_time.
	if d := z.Skills.Get(skill.IDCallSpirits); p.spiritTimers[0] > z.now+int64(d.Duration1.At(2)) {
		t.Errorf("timer %d is past %d", p.spiritTimers[0], z.now+int64(d.Duration1.At(2)))
	}
	cast(z, p, skill.IDSoulCollect, 1, nil)
	run(z, p, now, now+3000)
	if p.spiritBalls() != 5 {
		t.Errorf("%d spheres after Zen, want 5", p.spiritBalls())
	}
}

// TestSphereRequirement checks that skill_check_condition_castbegin
// fails without the spheres, and that skill_consume_requirement takes
// them.
func TestSphereRequirement(t *testing.T) {
	z, p, m := monkZone(t)
	cast(z, p, skill.IDInvestigate, 1, m)
	ev := run(z, p, 100, 2000)
	if len(skillDamage(ev, skill.IDInvestigate)) != 0 || len(skillFailures(ev)) != 1 {
		t.Fatalf("Investigate without spheres: %v", ev)
	}
	giveSpheres(z, p, 2)
	cast(z, p, skill.IDInvestigate, 1, m)
	ev = run(z, p, 2050, 4000)
	if len(skillDamage(ev, skill.IDInvestigate)) != 1 {
		t.Fatalf("Investigate with spheres: %v", ev)
	}
	if p.spiritBalls() != 1 {
		t.Errorf("%d spheres left, want 1", p.spiritBalls())
	}
}

// TestTripleAttackComboChain checks battle_weapon_attack's Raging
// Trifecta Blow, and skill_combo with the combo rules of
// skill_check_condition_castbegin and unit_skilluse_id2: each combo skill
// auto targets the attack target, uses its spheres and opens the window
// of the next one.
func TestTripleAttackComboChain(t *testing.T) {
	z, p, m := monkZone(t)
	z.rand = rng.New(forcedMinSource{})
	giveSpheres(z, p, 5)
	e := p.entity
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: p.netIDs.id(m), repeat: true}})
	ev := run(z, p, 100, 100)
	if len(skillDamage(ev, skill.IDTripleAttack)) != 1 {
		t.Fatalf("no Raging Trifecta Blow: %v", ev)
	}
	combo := e.sc.Get(sc.SCCombo)
	if combo == nil || combo.Val1 != skill.IDTripleAttack {
		t.Fatalf("combo %+v, want Raging Trifecta Blow's", combo)
	}
	// The window is the delay, plus 300 ms of combo_delay_rate.
	delay := int64(z.Skills.Get(skill.IDTripleAttack).Delay(z.caster(e), 10))
	if int64(combo.Val4) != max(delay, int64(e.status.AMotion))+300 {
		t.Errorf("combo time %d, delay %d", combo.Val4, delay)
	}
	if e.nextAttack < z.now+int64(combo.Val4) {
		t.Errorf("next attack at %d, before the window ends at %d", e.nextAttack, z.now+int64(combo.Val4))
	}

	chain := []struct {
		id, spheres, next int
	}{
		{skill.IDChainCombo, 5, skill.IDChainCombo},
		{skill.IDComboFinish, 4, skill.IDComboFinish},
		{skill.IDTigerFist, 3, skill.IDTigerFist},
		// Chain Crush Combo needs Asura Strike's Fury to go on.
		{skill.IDChainCrush, 2, 0},
	}
	for _, c := range chain {
		z.stepAt(e.canAct)
		events(p)
		cast(z, p, c.id, 5, nil)
		ev = run(z, p, z.now+50, z.now+50)
		if len(skillDamage(ev, c.id)) != 1 {
			t.Fatalf("skill %d: no hit on the attack target: %v", c.id, ev)
		}
		if p.spiritBalls() != c.spheres {
			t.Errorf("skill %d: %d spheres, want %d", c.id, p.spiritBalls(), c.spheres)
		}
		if got := e.sc.Val1(sc.SCCombo); got != c.next {
			t.Fatalf("skill %d: combo of %d, want %d", c.id, got, c.next)
		}
		if e.target != m {
			t.Fatalf("skill %d: the attack stopped", c.id)
		}
	}
}

// TestComboWindowEnds checks that a combo skill cannot follow once the
// window ran out, and fails without a message, as rAthena's castbegin
// check returns false without clif_skill_fail.
func TestComboWindowEnds(t *testing.T) {
	z, p, m := monkZone(t)
	z.rand = rng.New(forcedMinSource{})
	e := p.entity
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: p.netIDs.id(m), repeat: true}})
	run(z, p, 100, 100)
	combo := e.sc.Get(sc.SCCombo)
	if combo == nil {
		t.Fatal("no combo")
	}
	end := z.now + int64(combo.Val4)
	// Stop attacking, so no new Raging Trifecta Blow opens a window.
	e.target = nil
	run(z, p, z.now+50, end+50)
	if e.sc.Has(sc.SCCombo) {
		t.Fatal("the combo window did not end")
	}
	e.target = m
	cast(z, p, skill.IDChainCombo, 5, nil)
	ev := run(z, p, z.now+50, z.now+500)
	if len(skillDamage(ev, skill.IDChainCombo)) != 0 || len(skillFailures(ev)) != 0 {
		t.Errorf("Raging Quadruple Blow after the window: %v", ev)
	}
}

// TestAsuraStrike checks SkillAsuraStrike: it uses every sphere and all
// SP, starts SC_EXTREMITYFIST, which stops SP regen for a Monk, ends
// Fury, and moves the caster toward the target.
func TestAsuraStrike(t *testing.T) {
	z, p, m := monkZone(t)
	e := p.entity
	e.x, e.y = m.x+2, m.y
	giveSpheres(z, p, 5)
	z.scStart(e, e, sc.SCExplosionspirits, 100, 5, 180000)
	sp := p.sp
	// Renewal: SP needed is 1 and the ratio reads the current SP.
	if req := z.requirement(e, z.Skills.Get(skill.IDExtremityFist), 5); req.SpiritSphere != 5 || req.SP != 1 {
		t.Fatalf("requirement %+v", req)
	}
	cast(z, p, skill.IDExtremityFist, 5, m)
	ev := run(z, p, 100, 3000)
	hits := skillDamage(ev, skill.IDExtremityFist)
	if len(hits) != 1 || hits[0].GetAmount() <= 0 {
		t.Fatalf("Asura Strike hits %v", hits)
	}
	if p.sp != 0 || p.spiritBalls() != 0 {
		t.Errorf("SP %d (was %d) and %d spheres after Asura Strike, want none", p.sp, sp, p.spiritBalls())
	}
	if p.spiritBallOld != 5 {
		t.Errorf("spiritball_old %d, want 5", p.spiritBallOld)
	}
	if !e.sc.Has(sc.SCExtremityfist) || e.sc.Has(sc.SCExplosionspirits) {
		t.Errorf("statuses after Asura Strike: %v", e.sc.Sorted())
	}
	// The caster moves 3 cells west, toward the target and past it.
	if e.x != m.x+2-3 || e.y != m.y {
		t.Errorf("caster at %d,%d, want %d,%d", e.x, e.y, m.x-1, m.y)
	}
	if !z.monkNoSPRegen(p) {
		t.Error("SP regen goes on under SC_EXTREMITYFIST")
	}
}

// TestAsuraStrikeAfterRagingThrust checks unit_skilluse_id2's instant
// Asura Strike in a combo, and skill_get_requirement's renewal rule: it
// uses the spheres the player has.
func TestAsuraStrikeAfterRagingThrust(t *testing.T) {
	z, p, _ := monkZone(t)
	e := p.entity
	giveSpheres(z, p, 3)
	z.scStart4(e, e, sc.SCCombo, 100, skill.IDComboFinish, 0, 0, 0, 2000)
	d := z.Skills.Get(skill.IDExtremityFist)
	if req := z.requirement(e, d, 5); req.SpiritSphere != 3 {
		t.Errorf("spheres needed %d, want the 3 the player has", req.SpiritSphere)
	}
	if res := d.CastTimeFor(z.caster(e), 5, true, true); res.Milliseconds != 0 {
		t.Errorf("cast time %d in a combo, want 0", res.Milliseconds)
	}
}

// TestBladeStop checks battle_weapon_attack's catch of SC_BLADESTOP_WAIT,
// which locks both units in SC_BLADESTOP, and that Raging Quadruple Blow
// hits the caught unit and ends Blade Stop for both.
func TestBladeStop(t *testing.T) {
	z, p, m := monkZone(t)
	e := p.entity
	giveSpheres(z, p, 1)
	cast(z, p, skill.IDBladeStop, 5, nil)
	run(z, p, 100, 100)
	if !e.sc.Has(sc.SCBladestopWait) || p.spiritBalls() != 0 {
		t.Fatalf("no SC_BLADESTOP_WAIT, or the sphere was kept: %v", e.sc.Sorted())
	}
	// The monster attacks.
	m.target, m.repeat = e, true
	m.nextAttack = 0
	ev := run(z, p, 150, 150)
	if !e.sc.Has(sc.SCBladestop) || !m.sc.Has(sc.SCBladestop) {
		t.Fatalf("Blade Stop did not catch the attack: %v", ev)
	}
	if e.sc.Val1(sc.SCBladestop) != 5 || m.sc.Val1(sc.SCBladestop) != 5 {
		t.Errorf("levels %d and %d, want 5 and 5", e.sc.Val1(sc.SCBladestop), m.sc.Val1(sc.SCBladestop))
	}
	if !m.cant().Attack || !e.cant().Move {
		t.Error("the units can still act")
	}
	// Only the skills of the level are allowed: Steel Body is not.
	if z.statusCheckSkillUse(e, e, z.Skills.Get(skill.IDSteelBody), 0) {
		t.Error("Steel Body allowed in Blade Stop")
	}
	z.stepAt(max(e.canAct, 200))
	cast(z, p, skill.IDChainCombo, 5, nil)
	ev = run(z, p, z.now+50, z.now+50)
	if len(skillDamage(ev, skill.IDChainCombo)) != 1 {
		t.Fatalf("Raging Quadruple Blow did not hit the caught unit: %v", ev)
	}
	if e.sc.Has(sc.SCBladestop) || m.sc.Has(sc.SCBladestop) {
		t.Error("Blade Stop did not end for both units")
	}
}

// TestSnap checks SkillSnap: the caster moves to the cell, uses its
// sphere, and cannot use Asura Strike for 2 seconds. Under Fury it needs
// no sphere.
func TestSnap(t *testing.T) {
	z, p, _ := monkZone(t)
	e := p.entity
	giveSpheres(z, p, 1)
	x, y := e.x+5, e.y+3
	at := z.now
	castAt(z, p, skill.IDBodyRelocation, 1, x, y)
	run(z, p, 100, 100)
	if e.x != x || e.y != y {
		t.Fatalf("caster at %d,%d, want %d,%d", e.x, e.y, x, y)
	}
	if p.spiritBalls() != 0 {
		t.Errorf("%d spheres left, want 0", p.spiritBalls())
	}
	if until := p.cooldowns[skill.IDExtremityFist]; until != at+2000 {
		t.Errorf("Asura Strike blocked until %d, want %d", until, at+2000)
	}
	z.scStart(e, e, sc.SCExplosionspirits, 100, 5, 180000)
	if req := z.requirement(e, z.Skills.Get(skill.IDBodyRelocation), 1); req.SpiritSphere != 0 {
		t.Errorf("Snap under Fury needs %d spheres, want 0", req.SpiritSphere)
	}
}

// TestAbsorbSpiritSpheres checks SkillAbsorbSpiritSphere: 7 SP per sphere
// of the caster itself, and 2 SP per level of a monster at 20%, after
// which the monster attacks the caster.
func TestAbsorbSpiritSpheres(t *testing.T) {
	z, p, m := monkZone(t)
	e := p.entity
	giveSpheres(z, p, 3)
	p.sp = 10
	cast(z, p, skill.IDAbsorbSpirits, 1, nil)
	run(z, p, 100, 1000)
	if p.spiritBalls() != 0 || p.sp != 10-5+21 {
		t.Fatalf("%d spheres and %d SP, want 0 and %d", p.spiritBalls(), p.sp, 10-5+21)
	}
	z.rand = rng.New(forcedMinSource{})
	// mob_target: only a monster that can attack takes a target.
	m.status.Mode = mob.ModeCanAttack
	sp := p.sp
	z.stepAt(max(e.canAct, 1050))
	cast(z, p, skill.IDAbsorbSpirits, 1, m)
	run(z, p, z.now+50, z.now+1000)
	if want := sp - 5 + int64(2*m.status.Level); p.sp != want {
		t.Errorf("SP %d after absorbing from the monster, want %d", p.sp, want)
	}
	if m.monster.target != e {
		t.Error("the monster did not turn on the caster")
	}
}

// TestPalmStrikeLands checks SkillRagingPalmStrike: the hit comes a
// second after the cast, plus the attack motion, through
// skill_addtimerskill.
func TestPalmStrikeLands(t *testing.T) {
	z, p, m := monkZone(t)
	e := p.entity
	z.scStart(e, e, sc.SCExplosionspirits, 100, 5, 180000)
	cast(z, p, skill.IDPalmStrike, 5, m)
	if len(z.timerSkills) != 1 || z.timerSkills[0].src != e {
		t.Fatalf("%d timer skills, want 1 of the caster", len(z.timerSkills))
	}
	due := z.timerSkills[0].at
	if due != z.now+1000+int64(e.status.AMotion) {
		t.Errorf("due at %d, want %d", due, z.now+1000+int64(e.status.AMotion))
	}
	ev := run(z, p, 100, due-50)
	if hits := skillDamage(ev, skill.IDPalmStrike); len(hits) != 0 {
		t.Fatalf("the hit came early: %v", hits)
	}
	hp := m.hp
	ev = run(z, p, due, due+50)
	if hits := skillDamage(ev, skill.IDPalmStrike); len(hits) != 1 || m.hp >= hp {
		t.Errorf("no hit at the timer: %v", ev)
	}
}
