package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Hand tests of the Knight and Lord Knight skills: the areas, pushes,
// counters and statuses that the recordings do not cover.

// javelin is a one-handed spear of both modes.
const javelin = 1401

// knightSkills are every Knight and Lord Knight skill at its top level,
// with the Swordman skills.
var knightSkills = [][2]int{
	{skill.IDBasic, 9}, {skill.IDSwordMastery, 10}, {skill.IDTwoHandMastery, 10}, {skill.IDBash, 10},
	{skill.IDSpearMastery, 10}, {skill.IDPierce, 10}, {skill.IDBrandishSpear, 10}, {skill.IDSpearStab, 10},
	{skill.IDSpearBoomerang, 5}, {skill.IDTwoHandQuicken, 10}, {skill.IDAutoCounter, 5}, {skill.IDBowlingBash, 10},
	{skill.IDRiding, 1}, {skill.IDCavalierMastery, 5}, {skill.IDChargeAttack, 1}, {skill.IDAuraBlade, 5},
	{skill.IDParrying, 10}, {skill.IDLKConcentration, 5}, {skill.IDTensionRelax, 1}, {skill.IDBerserk, 1},
	{skill.IDSpiralPierce, 5}, {skill.IDHeadCrush, 5}, {skill.IDJointBeat, 10},
}

// knightZone is a golden zone of a mode with a level 99 Lord Knight at
// (20, 20) holding a weapon, and Golems with a lot of HP. Rolls are
// forced to their lowest value.
func knightZone(t *testing.T, mode reference.Mode, weapon uint32, golems int) (*Zone, *Player, []*entity) {
	t.Helper()
	mobs := make([]uint32, golems)
	for i := range mobs {
		mobs[i] = 1040
	}
	z := goldenZone(t, mode, mobs...)
	p := goldenPlayerIn(t, z, goldenPlayer{Job: 4008, Build: "melee", Sex: 1, JobLevel: 50, Skills: knightSkills}, weapon, 0)
	var out []*entity
	for e := range z.entities.all() {
		if e.monster == nil {
			continue
		}
		st := *e.status
		st.MaxHP = 1000000
		e.status = &st
		e.monster.spawn.Status = &st
		e.hp = 1000000
		e.monster.spawn.CanMove = false
		out = append(out, e)
	}
	return z, p, out
}

// hit reports whether a monster lost HP.
func wasHit(e *entity) bool { return e.hp < 1000000 }

// TestBrandishSpearArea checks SkillBrandishSpear's map_foreachindir in
// pre-renewal: the 5 rows in front of the caster, 2 cells to each side,
// toward the target, and not behind the caster or past the rows.
func TestBrandishSpearArea(t *testing.T) {
	z, p, m := knightZone(t, reference.PreRenewal, javelin, 5)
	src := p.entity
	place := func(e *entity, x, y int) { e.x, e.y = x, y }
	place(m[0], 20, 22) // the target, two rows north
	place(m[1], 22, 24) // the farthest row, at the side's edge
	place(m[2], 20, 25) // past the rows
	place(m[3], 23, 22) // past the side
	place(m[4], 20, 18) // behind the caster
	d := z.Skills.Get(skill.IDBrandishSpear)
	z.castendNoDamage(src, m[0], d, 10, 0)
	for i, want := range []bool{true, true, false, false, false} {
		if wasHit(m[i]) != want {
			t.Errorf("monster %d at (%d, %d): hit %v, want %v", i, m[i].x, m[i].y, wasHit(m[i]), want)
		}
	}
	// The nearer rows deal more at level 10 (flag 0 against flag 3).
	if dm0, dm1 := 1000000-m[0].hp, 1000000-m[1].hp; dm0 <= dm1 {
		t.Errorf("near row %d, far row %d", dm0, dm1)
	}
}

// TestBrandishSpearNeedsRiding checks ST_RIDING in
// skill_check_condition_castbegin and pc_setriding: Brandish Spear fails
// on foot and works on a Peco Peco, which needs Riding.
func TestBrandishSpearNeedsRiding(t *testing.T) {
	z, p, m := knightZone(t, reference.PreRenewal, javelin, 1)
	m[0].x, m[0].y = 20, 21
	cast(z, p, skill.IDBrandishSpear, 10, m[0])
	ev := run(z, p, 100, 200)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("on foot: failures %v", f)
	}
	speed := p.entity.status.Speed
	weight := p.maxWeight
	if !z.setRiding(p, true) || !p.riding() {
		t.Fatal("could not mount")
	}
	z.flushStats()
	if p.entity.status.Speed >= speed || p.maxWeight != weight+10000 {
		t.Errorf("riding: speed %d -> %d, max weight %d -> %d", speed, p.entity.status.Speed, weight, p.maxWeight)
	}
	cast(z, p, skill.IDBrandishSpear, 10, m[0])
	run(z, p, 250, 3000)
	if !wasHit(m[0]) {
		t.Error("Brandish Spear did not hit while riding")
	}
	// Without Riding, a player cannot mount.
	delete(p.char.Skills, skill.IDRiding)
	z.setRiding(p, false)
	if z.setRiding(p, true) || p.riding() {
		t.Error("mounted without Riding")
	}
}

// TestRidingAspd checks status_calc_pc's riding ASPD penalty, which
// Cavalier Mastery takes away: in pre-renewal aspd_rate rises by 500 -
// 100 per level.
func TestRidingAspd(t *testing.T) {
	z, p, _ := knightZone(t, reference.PreRenewal, javelin, 0)
	walk := p.entity.status.AMotion
	z.setRiding(p, true)
	z.flushStats()
	if got := p.entity.status.AMotion; got != walk {
		t.Errorf("riding with Cavalier Mastery 5: amotion %d, walking %d", got, walk)
	}
	p.char.Skills[skill.IDCavalierMastery] = 0
	z.recalc(p)
	if got := p.entity.status.AMotion; got <= walk {
		t.Errorf("riding without Cavalier Mastery: amotion %d, walking %d", got, walk)
	}
}

// TestSpearStabLine checks SkillSpearStab: the target and the enemies on
// the cells between it and the caster are hit and pushed 6 cells away.
func TestSpearStabLine(t *testing.T) {
	z, p, m := knightZone(t, reference.Renewal, javelin, 3)
	m[0].x, m[0].y = 20, 24 // the target
	m[1].x, m[1].y = 20, 22 // between
	m[2].x, m[2].y = 22, 22 // off the line
	z.castendDamage(p.entity, m[0], z.Skills.Get(skill.IDSpearStab), 10, 0)
	if !wasHit(m[0]) || !wasHit(m[1]) || wasHit(m[2]) {
		t.Fatalf("hits: target %v, between %v, off %v", wasHit(m[0]), wasHit(m[1]), wasHit(m[2]))
	}
	if m[0].y != 30 || m[1].y != 28 {
		t.Errorf("pushed to y %d and %d, want 30 and 28", m[0].y, m[1].y)
	}
}

// TestSpearBoomerangRange checks skill_db's range per level, and a cast
// from 11 cells that hits without walking.
func TestSpearBoomerangRange(t *testing.T) {
	z, p, m := knightZone(t, reference.Renewal, javelin, 1)
	d := z.Skills.Get(skill.IDSpearBoomerang)
	for lv, want := range []int{3, 5, 7, 9, 11} {
		if got := d.RangeFor(p.entity.status, lv+1); got != want {
			t.Errorf("level %d: range %d, want %d", lv+1, got, want)
		}
	}
	m[0].x, m[0].y = 20, 31
	cast(z, p, skill.IDSpearBoomerang, 5, m[0])
	ev := run(z, p, 100, 1500)
	if len(skillDamage(ev, skill.IDSpearBoomerang)) != 1 || p.entity.y != 20 {
		t.Errorf("hits %d, caster at y %d", len(skillDamage(ev, skill.IDSpearBoomerang)), p.entity.y)
	}
}

// TestAutoCounter checks Auto Counter's part of battle_weapon_attack: an
// attack from the front misses, the status ends, and the caster counters
// with a critical KN_AUTOCOUNTER hit. An attack from behind lands.
func TestAutoCounter(t *testing.T) {
	z, p, m := knightZone(t, reference.Renewal, 0, 1)
	src := p.entity
	golem := m[0]
	golem.x, golem.y = 20, 21
	src.dir = 4 // north
	d := z.Skills.Get(skill.IDAutoCounter)
	z.castendNoDamage(src, src, d, 5, 0)
	if !src.sc.Has(sc.SCAutocounter) {
		t.Fatal("no SC_AUTOCOUNTER")
	}
	hp := src.hp
	z.strike(golem, src)
	if src.hp != hp {
		t.Errorf("the countered attack dealt %d", hp-src.hp)
	}
	if src.sc.Has(sc.SCAutocounter) {
		t.Error("SC_AUTOCOUNTER did not end")
	}
	if !wasHit(golem) {
		t.Fatal("no counter")
	}
	var counter *hit
	for i := range z.hits {
		if z.hits[i].skill != nil && z.hits[i].skill.ID == skill.IDAutoCounter {
			counter = &z.hits[i]
		}
	}
	if counter == nil || counter.result.Kind != battle.Critical {
		t.Errorf("counter %+v", counter)
	}
	// From behind, the attack lands.
	z.castendNoDamage(src, src, d, 5, 0)
	golem.x, golem.y = 20, 19
	z.strike(golem, src)
	if !src.sc.Has(sc.SCAutocounter) {
		t.Error("an attack from behind ended Auto Counter")
	}
}

// TestBowlingBashChain checks pre-renewal SkillBowlingBash: the target
// rolls one cell the way the caster faces, hits the enemy it meets,
// which rolls on, and is hit twice itself (the collision and the
// original hit).
func TestBowlingBashChain(t *testing.T) {
	z, p, m := knightZone(t, reference.PreRenewal, 0, 2)
	src := p.entity
	src.dir = 4 // north
	a, b := m[0], m[1]
	a.x, a.y = 20, 21
	b.x, b.y = 20, 23
	z.castendDamage(src, a, z.Skills.Get(skill.IDBowlingBash), 10, 0)
	hits := map[*entity]int{}
	for _, h := range z.hits {
		if h.skill != nil && h.skill.ID == skill.IDBowlingBash {
			hits[h.target]++
		}
	}
	if hits[a] != 2 || hits[b] != 2 {
		t.Errorf("hits: target %d, other %d", hits[a], hits[b])
	}
	if a.x != 20 || a.y != 22 {
		t.Errorf("target at (%d, %d), want (20, 22)", a.x, a.y)
	}
	// The forced roll gives direction 0, so the other rolls one cell
	// south, back onto the target, and stops there.
	if b.x != 20 || b.y != 22 {
		t.Errorf("other at (%d, %d), want (20, 22)", b.x, b.y)
	}
}

// TestChargeAttackMovesAndPushes checks SkillChargeAttack: the caster
// lands next to the target, and with a straight path pushes it as many
// cells as the distance in pre-renewal.
func TestChargeAttackMovesAndPushes(t *testing.T) {
	z, p, m := knightZone(t, reference.PreRenewal, 0, 1)
	src, golem := p.entity, m[0]
	golem.x, golem.y = 20, 27
	z.castendDamage(src, golem, z.Skills.Get(skill.IDChargeAttack), 1, 0)
	if src.x != 20 || src.y != 26 {
		t.Errorf("caster at (%d, %d), want (20, 26)", src.x, src.y)
	}
	if !wasHit(golem) || golem.y != 34 {
		t.Errorf("hit %v, target at y %d, want 34", wasHit(golem), golem.y)
	}
}

// TestParryingBlocks checks Parrying in battle_status_block_damage:
// weapon hits are blocked by its chance, which forced rolls always pass.
func TestParryingBlocks(t *testing.T) {
	z, p, m := knightZone(t, reference.Renewal, 0, 1)
	src, golem := p.entity, m[0]
	golem.x, golem.y = 20, 21
	z.castendNoDamage(src, src, z.Skills.Get(skill.IDParrying), 10, 0)
	if src.sc.Val2(sc.SCParrying) != 50 {
		t.Fatalf("block chance %d", src.sc.Val2(sc.SCParrying))
	}
	// Forced rolls would make every attack a lucky dodge.
	src.status.Flee2 = 0
	hp := src.hp
	z.strike(golem, src)
	if src.hp != hp {
		t.Errorf("Parrying let %d through", hp-src.hp)
	}
	if len(z.hits) == 0 || !z.hits[len(z.hits)-1].result.Blocked {
		t.Error("the hit was not blocked")
	}
}

// TestBerserk checks SC_BERSERK: full HP at three times the max, no SP,
// no skills, no healing from skills, 5% of max HP every 10 seconds, and
// 100 HP when it ends.
func TestBerserk(t *testing.T) {
	z, p, _ := knightZone(t, reference.Renewal, 0, 0)
	src := p.entity
	maxHP := src.status.MaxHP
	z.castendNoDamage(src, src, z.Skills.Get(skill.IDBerserk), 1, 0)
	z.flushStats()
	if !src.sc.Has(sc.SCBerserk) || src.status.MaxHP < maxHP*3 {
		t.Fatalf("max HP %d -> %d", maxHP, src.status.MaxHP)
	}
	if src.hp != int64(src.status.MaxHP) || p.sp != 0 {
		t.Errorf("HP %d of %d, SP %d", src.hp, src.status.MaxHP, p.sp)
	}
	if !src.cant().Cast {
		t.Error("skills are allowed")
	}
	before := src.hp
	z.healHP(src, 100)
	if src.hp != before {
		t.Error("a skill healed under Frenzy")
	}
	penalty := int64(5 * src.status.MaxHP / 100)
	start := z.now
	run(z, p, start+50, start+10050)
	if src.hp != before-penalty {
		t.Errorf("HP %d after 10 s, want %d", src.hp, before-penalty)
	}
	z.scEnd(src, sc.SCBerserk)
	z.flushStats()
	if src.hp != 100 || !src.sc.Has(sc.SCRegeneration) {
		t.Fatalf("after Frenzy: HP %d, regeneration blocked %v", src.hp, src.sc.Has(sc.SCRegeneration))
	}
	// SC_REGENERATION with val4 stops natural HP regen.
	now := z.now
	run(z, p, now+50, now+20000)
	if src.hp != 100 {
		t.Errorf("HP regenerated to %d after Frenzy", src.hp)
	}
}

// TestTensionRelaxRegen checks status_calc_regen_rate: Tension Relax
// triples natural HP regen, so HP heals every 2 seconds instead of 6.
func TestTensionRelaxRegen(t *testing.T) {
	gain := func(relax bool) int64 {
		z, p, _ := knightZone(t, reference.Renewal, 0, 0)
		src := p.entity
		if relax {
			z.castendNoDamage(src, src, z.Skills.Get(skill.IDTensionRelax), 1, 0)
		}
		src.hp = 1000
		now := z.now
		run(z, p, now+50, now+12000)
		return src.hp - 1000
	}
	walk, relax := gain(false), gain(true)
	if walk <= 0 || relax < 3*walk-walk/2 {
		t.Errorf("12 s of regen: %d standing, %d with Tension Relax", walk, relax)
	}
}

// TestJointBeatBreaks checks SkillVitalStrike: the forced roll breaks the
// ankle, which slows the target (status_calc_speed).
func TestJointBeatBreaks(t *testing.T) {
	z, p, m := knightZone(t, reference.Renewal, javelin, 1)
	golem := m[0]
	golem.x, golem.y = 20, 21
	speed := golem.status.Speed
	z.castendDamage(p.entity, golem, z.Skills.Get(skill.IDJointBeat), 10, 0)
	if golem.sc.Val2(sc.SCJointbeat) != sc.BreakAnkle {
		t.Fatalf("breaks %#x", golem.sc.Val2(sc.SCJointbeat))
	}
	if golem.status.Speed <= speed {
		t.Errorf("speed %d -> %d", speed, golem.status.Speed)
	}
}

// TestHeadCrushFailsOnBoss checks SkillTraumaticBlow::castendDamageId:
// no hit on a boss.
func TestHeadCrushFailsOnBoss(t *testing.T) {
	z, p, m := knightZone(t, reference.Renewal, 0, 1)
	golem := m[0]
	golem.x, golem.y = 20, 21
	golem.status.Class = battle.ClassBoss
	z.castendDamage(p.entity, golem, z.Skills.Get(skill.IDHeadCrush), 5, 0)
	if wasHit(golem) {
		t.Error("Head Crush hit a boss")
	}
	golem.status.Class = 0
	z.castendDamage(p.entity, golem, z.Skills.Get(skill.IDHeadCrush), 5, 0)
	if !wasHit(golem) || !golem.sc.Has(sc.SCBleeding) {
		t.Errorf("hit %v, bleeding %v", wasHit(golem), golem.sc.Has(sc.SCBleeding))
	}
}
