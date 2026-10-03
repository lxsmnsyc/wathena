package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// npcWeaponMob is the monster that uses the skills in these tests: the
// Golden Thief Bug, whose hits a level 99 Knight survives.
const npcWeaponMob = 1086

// npcWeaponZone is a parity zone in a mode with the monster at the
// center and a level 99 Knight next to it, east. Every roll is at its
// lowest, so every chance succeeds and a random pick takes the first.
func npcWeaponZone(t *testing.T, mode reference.Mode) (*Zone, *entity, *Player) {
	t.Helper()
	z, defs := parityZone(t, mode)
	cx, cy := parityCenter[0], parityCenter[1]
	p := parityPlayer(t, z, 7, "melee", 99, 50, cx+1, cy)
	m := parityMob(t, z, defs, npcWeaponMob, cx, cy, defs.status[npcWeaponMob].Mode, nil)
	z.now = 2000
	return z, m, p
}

// knight adds another level 99 Knight on a cell.
func knight(t *testing.T, z *Zone, x, y int) *Player {
	t.Helper()
	return parityPlayer(t, z, 7, "melee", 99, 50, x, y)
}

// lost is the HP a player lost.
func lost(p *Player) int64 { return int64(p.entity.status.MaxHP) - p.entity.hp }

// TestNpcCriticalSlash checks is_attack_critical: NPC_CRITICALSLASH is
// always critical, in both modes, even with no critical rate.
func TestNpcCriticalSlash(t *testing.T) {
	for _, mode := range reference.Modes {
		z, m, p := npcWeaponZone(t, mode)
		st := *m.status
		st.Cri = 0
		m.status = &st
		d := z.Skills.Get(idNpcCriticalSlash)
		if res := z.calcSkillAttack(d.Type, m, m, p.entity, d, 1, 0, nil); res.Kind != battle.Critical {
			t.Errorf("%s: kind %v, want critical", mode, res.Kind)
		}
	}
}

// TestNpcBloodDrainHeals checks SkillSuckingBlood: the monster heals the
// damage it dealt.
func TestNpcBloodDrainHeals(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	m.hp = 100
	z.castendDamage(m, p.entity, z.Skills.Get(idNpcBloodDrain), 1, 0)
	if got := lost(p); got <= 0 || m.hp != 100+got {
		t.Errorf("player lost %d, monster HP %d, want 100 + the damage", got, m.hp)
	}
}

// TestNpcEnergyDrainHeals checks SkillEnergyDrain: a magic hit that
// heals the monster by the damage.
func TestNpcEnergyDrainHeals(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	m.hp = 100
	z.castendDamage(m, p.entity, z.Skills.Get(idNpcEnergyDrain), 1, 0)
	if got := lost(p); got <= 0 || m.hp != 100+got {
		t.Errorf("player lost %d, monster HP %d, want 100 + the damage", got, m.hp)
	}
}

// TestNpcMentalBreakerSP checks SkillSpiritDestruction and
// status_percent_damage: level 3 takes 25% of the maximum SP, and level 5
// all of it.
func TestNpcMentalBreakerSP(t *testing.T) {
	for _, c := range []struct{ lv, pct int }{{3, 25}, {5, 100}} {
		z, m, p := npcWeaponZone(t, reference.Renewal)
		maxSP := int64(p.entity.status.MaxSP)
		z.castendDamage(m, p.entity, z.Skills.Get(idNpcMentalBreaker), c.lv, 0)
		want := maxSP - maxSP*int64(c.pct)/100
		if c.pct == 100 {
			want = 0
		}
		if p.sp != want {
			t.Errorf("level %d: SP %d, want %d", c.lv, p.sp, want)
		}
	}
}

// TestNpcCriticalWound checks SkillCriticalWounds and SC_CRITICALWOUND's
// setup (20% per level, levels 6 to 10 as 1 to 5, Duration2), and
// pc_itemheal's penalty.
func TestNpcCriticalWound(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	d := z.Skills.Get(idNpcCriticalWound)
	z.castendDamage(m, p.entity, d, 2, 0)
	e := p.entity.sc.Get(sc.SCCriticalwound)
	if e == nil {
		t.Fatal("no SC_CRITICALWOUND")
	}
	if e.Val2 != 40 || e.End != z.now+30000 {
		t.Errorf("val2 %d, ends in %d ms, want 40 and 30000", e.Val2, e.End-z.now)
	}
	if hp, sp := itemHealPenalty(p.entity, 100, 50); hp != 60 || sp != 30 {
		t.Errorf("item heal %d HP and %d SP, want 60 and 30", hp, sp)
	}
}

// TestNpcBreakArmor checks SkillBreakArmor and SkillBreakShield: the hit
// breaks the part with 1.5% per level (skill_break_equip).
func TestNpcBreakArmor(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	armor := give(t, z, p, 2302, 1)
	shield := give(t, z, p, 2101, 1)
	equip(z, p, armor)
	equip(z, p, shield)
	z.castendDamage(m, p.entity, z.Skills.Get(idNpcArmorBrake), 1, 0)
	if !p.inventory.Slots[armor].Broken {
		t.Error("armor not broken")
	}
	if p.inventory.Slots[shield].Broken {
		t.Error("shield broken by Armor Break")
	}
	z.castendDamage(m, p.entity, z.Skills.Get(idNpcShieldBrake), 1, 0)
	if !p.inventory.Slots[shield].Broken {
		t.Error("shield not broken")
	}
}

// TestNpcWeaponBraker checks NPC_WEAPONBRAKER's StatusSkillImpl, the
// setup of SC_WEAPONBREAKER (val2 is 2% per level, per 10,000), and
// skill_additional_effect, where it breaks the target's weapon on a
// weapon hit.
func TestNpcWeaponBraker(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	sword := give(t, z, p, 1101, 1)
	equip(z, p, sword)
	z.castendNoDamage(m, m, z.Skills.Get(idNpcWeaponBraker), 3, 0)
	e := m.sc.Get(scWeaponBreaker)
	if e == nil || e.Val2 != 600 || e.End != z.now+30000 {
		t.Fatalf("SC_WEAPONBREAKER %+v, want val2 600 for 30 s", e)
	}
	z.castendDamage(m, p.entity, z.Skills.Get(idNpcFireAttack), 1, 0)
	if !p.inventory.Slots[sword].Broken {
		t.Error("weapon not broken")
	}
}

// TestNpcSplashAttack checks SkillSplashAttack: the target and every
// enemy within 3 cells of it take a hit.
func TestNpcSplashAttack(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	cx, cy := parityCenter[0], parityCenter[1]
	near := knight(t, z, cx+4, cy)
	far := knight(t, z, cx+5, cy)
	z.castendDamage(m, p.entity, z.Skills.Get(idNpcSplashAttack), 1, 0)
	if lost(p) <= 0 || lost(near) <= 0 {
		t.Errorf("target lost %d, near lost %d, want both hit", lost(p), lost(near))
	}
	if lost(far) != 0 {
		t.Errorf("far lost %d", lost(far))
	}
}

// TestNpcPulseStrike checks SkillPulseStrike: a self skill whose splash
// of 7 cells around the monster hits and pushes every enemy.
func TestNpcPulseStrike(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	cx, cy := parityCenter[0], parityCenter[1]
	edge := knight(t, z, cx, cy+7)
	out := knight(t, z, cx, cy+8)
	z.castendNoDamage(m, m, z.Skills.Get(idNpcPulseStrike), 1, 0)
	if lost(p) <= 0 || lost(edge) <= 0 || lost(out) != 0 {
		t.Errorf("lost %d, %d and %d, want the first two hit", lost(p), lost(edge), lost(out))
	}
	if p.entity.x != cx+8 {
		t.Errorf("pushed to x %d, want %d", p.entity.x, cx+8)
	}
}

// TestNpcPulseStrike2 checks SkillPulseStrike2 and skill_timerskill:
// three splashes around the monster, Duration1 (1 s) apart. The Knight
// cannot be pushed, so the knockback keeps it in the splash.
func TestNpcPulseStrike2(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	p.bonuses.NoKnockback = true
	z.castendNoDamage(m, m, z.Skills.Get(idNpcPulseStrike2), 1, 0)
	var hits []int64
	for _, at := range []int64{2000, 2999, 3000, 4000, 9000} {
		z.now = at
		z.advanceTimerSkills()
		hits = append(hits, lost(p))
	}
	one := hits[0]
	if one <= 0 || hits[1] != one || hits[2] != 2*one || hits[3] != 3*one || hits[4] != 3*one {
		t.Errorf("HP lost over time %v, want one splash each second, three in all", hits)
	}
}

// TestNpcDancingBlade checks SkillDancingBlade and skill_timerskill's
// NPC_DANCINGBLADE_ATK: five blades, the first after the attack motion
// and the rest 650 ms apart, each on a random enemy within 5 cells of
// the last target.
func TestNpcDancingBlade(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	cx, cy := parityCenter[0], parityCenter[1]
	other := knight(t, z, cx+3, cy)
	z.castendNoDamage(m, p.entity, z.Skills.Get(idNpcDancingBlade), 1, 0)
	start := z.now + int64(m.status.AMotion)
	z.now = start - 1
	z.advanceTimerSkills()
	if lost(p) != 0 {
		t.Fatal("hit before the attack motion")
	}
	hits := 0
	last := int64(0)
	for i := range 6 {
		z.now = start + int64(i)*650
		z.advanceTimerSkills()
		if total := lost(p) + lost(other); total > last {
			hits++
			last = total
		}
	}
	if hits != 5 {
		t.Errorf("%d blades, want 5", hits)
	}
	// With the lowest rolls, each later blade takes the first enemy in
	// the area: the first Knight.
	if lost(other) != 0 {
		t.Errorf("other Knight lost %d", lost(other))
	}
}

// TestNpcPhantomThrust checks SkillNpcPhantomThrust: the target is
// pulled next to the monster and hit, and skill_castend_id_check fails a
// unit that is not an enemy with USESKILL_FAIL_TOTARGET.
func TestNpcPhantomThrust(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	cx, cy := parityCenter[0], parityCenter[1]
	p.entity.x = cx + 5
	d := z.Skills.Get(idNpcPhantomThrust)
	z.castendDamage(m, p.entity, d, 1, 0)
	if p.entity.x != cx+1 || p.entity.y != cy {
		t.Errorf("target at %d,%d, want %d,%d", p.entity.x, p.entity.y, cx+1, cy)
	}
	if lost(p) <= 0 {
		t.Error("no hit")
	}
	if r := z.castEndIDCheck(m, m, d); r == 0 {
		t.Error("the monster itself passed the target check")
	}
}

// TestNpcDarkPiercing checks SkillDarkPiercing with
// skill_eightpath_algorithm: the enemies in a line toward the target,
// 2 cells to each side, take a hit.
func TestNpcDarkPiercing(t *testing.T) {
	z, m, p := npcWeaponZone(t, reference.Renewal)
	cx, cy := parityCenter[0], parityCenter[1]
	line := knight(t, z, cx+6, cy+2)
	off := knight(t, z, cx+2, cy+3)
	z.castendDamage(m, p.entity, z.Skills.Get(idNpcDarkPiercing), 1, 0)
	if lost(p) <= 0 || lost(line) <= 0 || lost(off) != 0 {
		t.Errorf("lost %d, %d and %d, want the first two hit", lost(p), lost(line), lost(off))
	}
}

// TestNpcRevenge checks SkillRevenge and battle_gettargeted: a slave
// turns on the unit that attacks its master, as if provoked.
func TestNpcRevenge(t *testing.T) {
	z, defs := parityZone(t, reference.Renewal)
	cx, cy := parityCenter[0], parityCenter[1]
	master := parityMob(t, z, defs, npcWeaponMob, cx, cy, defs.status[npcWeaponMob].Mode, nil)
	slave := parityMob(t, z, defs, npcWeaponMob, cx+3, cy, defs.status[npcWeaponMob].Mode|mob.ModeAggressive, nil)
	slave.monster.master = master
	p := knight(t, z, cx+1, cy)
	z.castendNoDamage(slave, slave, z.Skills.Get(idNpcRevenge), 1, 0)
	if slave.monster.target != nil {
		t.Fatal("a target with no attacker")
	}
	p.entity.target = master
	z.castendNoDamage(slave, slave, z.Skills.Get(idNpcRevenge), 1, 0)
	if slave.monster.target != p.entity || slave.monster.provoke != p.entity {
		t.Errorf("target %v, provoke %v, want the attacker", slave.monster.target, slave.monster.provoke)
	}
}

// TestNpcSkillsRegistered checks that every skill of the batch has a
// formula and a behavior.
func TestNpcSkillsRegistered(t *testing.T) {
	for _, id := range []int{idNpcGroundAttack, idNpcWaterAttack, idNpcFireAttack, idNpcWindAttack,
		idNpcDarknessAttack, idNpcPoisonAttack, idNpcHolyAttack, idNpcTelekinesisAttack, idNpcUndeadAttack,
		idNpcComboAttack, idNpcCriticalSlash, idNpcGuidedAttack, idNpcPiercingAtt, idNpcRangeAttack,
		idNpcRandomAttack, idNpcSplashAttack, idNpcBloodDrain, idNpcEnergyDrain, idNpcMentalBreaker,
		idNpcPulseStrike, idNpcCriticalWound, idNpcArmorBrake, idNpcHelmBrake, idNpcShieldBrake,
		idNpcPhantomThrust, idNpcDarkPiercing, idNpcDancingBladeAttack, idNpcPulseStrike2} {
		if !skill.Ported(id) {
			t.Errorf("skill %d has no formula", id)
		}
	}
	for _, id := range []int{idNpcWeaponBraker, idNpcDancingBlade, idNpcRevenge, idNpcDancingBladeAttack} {
		if skillImpls[id] == nil {
			t.Errorf("skill %d has no behavior", id)
		}
	}
}
