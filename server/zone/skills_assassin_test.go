package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Hand tests of the Assassin and Assassin Cross skills: what the
// recordings do not cover. The zones are golden zones, whose rolls are
// all lowest, so every chance succeeds.

// assassinIn adds a level 99 melee player of a job with skills and a
// weapon to a golden zone.
func assassinIn(t *testing.T, z *Zone, job int, weapon uint32, skills map[int]int) *Player {
	t.Helper()
	var learned [][2]int
	for id, lv := range skills {
		learned = append(learned, [2]int{id, lv})
	}
	return goldenPlayerIn(t, z, goldenPlayer{Job: job, Build: "melee", Sex: 1, JobLevel: 50, Skills: learned}, weapon, 0)
}

// assassinMobs are a golden zone's monsters, kept in place.
func assassinMobs(z *Zone) []*entity {
	var out []*entity
	for e := range z.entities.all() {
		if e.monster != nil {
			e.monster.spawn.CanMove = false
			out = append(out, e)
		}
	}
	return out
}

// sturdy gives a monster enough HP to live through the test's hits.
func sturdy(e *entity) {
	e.status.MaxHP = 1000000
	e.hp = 1000000
}

// wallAt makes a cell a wall.
func wallAt(z *Zone, x, y int) { z.grid.Cells[x+y*z.grid.Width] &^= path.Walkable }

// TestCloakingWallsAndEnds checks SkillCloaking with rAthena's
// skill_check_condition_castbegin (AS_CLOAKING), skill_check_cloaking,
// map_moveblock's check and battle_weapon_attack's end of Cloaking.
func TestCloakingWallsAndEnds(t *testing.T) {
	z := goldenZone(t, reference.Renewal, 1002)
	m := assassinMobs(z)[0]
	p := assassinIn(t, z, 12, 0, map[int]int{skill.IDCloaking: 10})
	me := p.entity

	// Below level 3, a wall must be next to the caster.
	cast(z, p, skill.IDCloaking, 2, me)
	ev := run(z, p, 50, 500)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL || me.sc.Has(sc.SCCloaking) {
		t.Fatalf("level 2 in the open: failures %v, cloaked %v", f, me.sc.Has(sc.SCCloaking))
	}
	wallAt(z, me.x+1, me.y)
	cast(z, p, skill.IDCloaking, 2, me)
	run(z, p, 550, 1000)
	if !me.sc.Has(sc.SCCloaking) || me.sc.Option&sc.OptionCloak == 0 {
		t.Fatal("not cloaked next to a wall")
	}
	if z.canTarget(m, me) {
		t.Error("the monster can target the cloaked player")
	}
	// Walking away from the wall ends level 2.
	z.handle(command{player: p, walkTo: &[2]int{me.x - 5, me.y}})
	run(z, p, 1050, 3000)
	if me.sc.Has(sc.SCCloaking) {
		t.Fatal("level 2 still cloaked away from walls")
	}

	// Level 10 needs no wall, and an attack ends it.
	cast(z, p, skill.IDCloaking, 10, me)
	run(z, p, 3050, 3500)
	if !me.sc.Has(sc.SCCloaking) {
		t.Fatal("level 10 did not cloak in the open")
	}
	me.x, me.y = m.x, m.y-1
	me.target, me.repeat = m, false
	run(z, p, 3550, 4000)
	if me.sc.Has(sc.SCCloaking) {
		t.Error("still cloaked after an attack")
	}
	if m.hp == int64(m.status.MaxHP) {
		t.Error("the attack did not hit")
	}
}

// TestVenomDustPoisonsItsCross checks AS_VENOMDUST's unit: the layout of
// skill_init_unit_layout (a cross of 5 cells, each acting on the cells
// around it) and UNT_VENOMDUST in skill_unit_onplace_timer.
func TestVenomDustPoisonsItsCross(t *testing.T) {
	z := goldenZone(t, reference.Renewal, 1002, 1002)
	mobs := assassinMobs(z)
	near, far := mobs[0], mobs[1]
	near.x, near.y = 22, 22
	far.x, far.y = 23, 22
	p := assassinIn(t, z, 12, 0, map[int]int{skill.IDVenomDust: 10})
	give(t, z, p, itemRedGemstone, 1)
	castAt(z, p, skill.IDVenomDust, 10, 20, 22)
	run(z, p, 50, 1500)
	var g *unitGroup
	for _, x := range z.groups {
		g = x
	}
	if g == nil || len(g.units) != 5 {
		t.Fatalf("Venom Dust group %v", g)
	}
	if !near.sc.Has(sc.SCPoison) {
		t.Error("the monster next to the cross is not poisoned")
	}
	if far.sc.Has(sc.SCPoison) {
		t.Error("the monster two cells from the cross is poisoned")
	}
	if z.countItem(p, itemRedGemstone) != 0 {
		t.Error("the gem was not used")
	}
}

// TestVenomSplasherExplodes checks SkillVenomSplasher, SC_SPLASHER's
// countdown in status_change_timer and its skill_castend_damage_id in
// status_change_end, in both modes, with pre-renewal's 3/4 HP limit.
func TestVenomSplasherExplodes(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z := goldenZone(t, mode, 1023, 1002)
			mobs := assassinMobs(z)
			orc, poring := mobs[0], mobs[1]
			orc.x, orc.y = 20, 21
			poring.x, poring.y = 21, 22
			sturdy(orc)
			sturdy(poring)
			p := assassinIn(t, z, 12, 0, map[int]int{skill.IDVenomSplasher: 10, skill.IDPoisonReact: 10})
			give(t, z, p, itemRedGemstone, 2)
			now := int64(50)
			if mode == reference.PreRenewal {
				// Above 3/4 of its HP, the target cannot be marked.
				cast(z, p, skill.IDVenomSplasher, 10, orc)
				ev := run(z, p, now, now+2000)
				if f := skillFailures(ev); len(f) != 1 || orc.sc.Has(sc.SCSplasher) {
					t.Fatalf("full HP: failures %v", f)
				}
				// The failed cast still started the 12 second cooldown.
				now += 14000
				orc.hp = int64(orc.status.MaxHP) / 2
				z.stepAt(now)
			}
			orcHP, poringHP := orc.hp, poring.hp
			cast(z, p, skill.IDVenomSplasher, 10, orc)
			for limit := now + 3000; now < limit && !orc.sc.Has(sc.SCSplasher); now += 50 {
				z.stepAt(now)
			}
			if !orc.sc.Has(sc.SCSplasher) {
				t.Fatalf("no Venom Splasher on the target: %v", skillFailures(events(p)))
			}
			marked := now
			// Level 10 counts down 2 seconds after a first second.
			run(z, p, now, marked+2400)
			if orc.hp != orcHP || poring.hp != poringHP {
				t.Fatalf("hit before the countdown ended: orc %d -> %d", orcHP, orc.hp)
			}
			run(z, p, marked+2450, marked+2700)
			if orc.sc.Has(sc.SCSplasher) || orc.hp >= orcHP || poring.hp >= poringHP {
				t.Fatalf("no explosion: orc %d -> %d, poring %d -> %d", orcHP, orc.hp, poringHP, poring.hp)
			}
			if !poring.sc.Has(sc.SCPoison) {
				t.Error("the explosion did not poison the monster next to the target")
			}
			// Pre-renewal uses a gem on each cast, the failed one too, and
			// none on the explosion. Renewal uses none.
			want := 2
			if mode == reference.PreRenewal {
				want = 0
			}
			if n := z.countItem(p, itemRedGemstone); n != want {
				t.Errorf("%d gems left, want %d", n, want)
			}
		})
	}
}

// TestPoisonReactCounterAndBlock checks SC_POISONREACT in
// battle_calc_weapon_final_atk_modifiers (the Envenom counter and its
// count), battle_weapon_attack (the block of a poison attack) and
// battle_calc_attack_skill_ratio (the boosted attack that poisons and
// ends the status).
func TestPoisonReactCounterAndBlock(t *testing.T) {
	z := goldenZone(t, reference.Renewal, 1002)
	m := assassinMobs(z)[0]
	m.x, m.y = 20, 21
	p := assassinIn(t, z, 12, 0, map[int]int{skill.IDPoisonReact: 10, skill.IDEnvenom: 10})
	me := p.entity
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDPoisonReact), 10, 0)
	if e := me.sc.Get(sc.SCPoisonreact); e == nil || e.Val2 != 5 {
		t.Fatalf("Poison React %+v", e)
	}
	// A normal attack that deals damage meets the counter. The player
	// cannot dodge it by luck.
	sturdy(m)
	me.status.Flee2 = 0
	m.target, m.repeat = me, false
	hp := m.hp
	ev := run(z, p, 50, 100)
	if len(skillDamage(ev, skill.IDEnvenom)) != 1 || m.hp >= hp {
		t.Fatalf("no Envenom counter: %v", ev)
	}
	if me.sc.Get(sc.SCPoisonreact).Val2 != 4 {
		t.Errorf("counters left %d, want 4", me.sc.Get(sc.SCPoisonreact).Val2)
	}

	// A poison attack is blocked and turns the status to its boost.
	m.status.RHW.Ele = elePoison
	m.target, m.repeat = me, false
	m.nextAttack = 0
	me.target = nil
	before := me.hp
	normal := goldenAttack(z, me, m).Damage
	z.strike(m, me)
	if me.hp != before {
		t.Errorf("HP %d -> %d through the block", before, me.hp)
	}
	if e := me.sc.Get(sc.SCPoisonreact); e == nil || e.Val4 != 1 {
		t.Fatalf("not in boost mode: %+v", e)
	}
	if me.target != m {
		t.Error("the player does not strike back")
	}
	boosted := goldenAttack(z, me, m).Damage
	if boosted <= normal {
		t.Errorf("boosted attack %d, normal %d", boosted, normal)
	}
	z.strike(me, m)
	if me.sc.Has(sc.SCPoisonreact) {
		t.Error("Poison React did not end after the boosted attack")
	}
	run(z, p, 150, 1000)
	if !m.sc.Has(sc.SCPoison) {
		t.Error("the boosted attack did not poison")
	}
}

// TestEDPOnNormalAttacks checks SC_EDP on normal attacks in both modes:
// battle_attack_sc_bonus in renewal, battle_calc_element_damage in
// pre-renewal, the deadly poison of skill_additional_effect, and
// pc_unequipitem's switch_remove_edp.
func TestEDPOnNormalAttacks(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z := goldenZone(t, mode, 1040)
			golem := assassinMobs(z)[0]
			const jur = 1250
			p := assassinIn(t, z, 4013, jur, map[int]int{skill.IDEnchantDeadly: 5, skill.IDKatarMastery: 10})
			me := p.entity
			before := goldenAttack(z, me, golem)
			z.castendNoDamage(me, me, z.Skills.Get(skill.IDEnchantDeadly), 5, 0)
			z.flushStats()
			if !me.sc.Has(sc.SCEdp) {
				t.Fatal("no EDP")
			}
			after := goldenAttack(z, me, golem)
			if after.Damage <= before.Damage {
				t.Errorf("damage %d with EDP, %d without", after.Damage, before.Damage)
			}
			golem.x, golem.y = me.x, me.y+1
			z.strike(me, golem)
			if !golem.sc.Has(sc.SCDpoison) {
				t.Error("the attack did not deadly poison")
			}
			slot := -1
			for i, s := range p.inventory.Slots {
				if s.ID == jur {
					slot = i
				}
			}
			unequip(z, p, slot)
			if got, want := me.sc.Has(sc.SCEdp), mode == reference.PreRenewal; got != want {
				t.Errorf("EDP after taking off the weapon: %v, want %v", got, want)
			}
		})
	}
}

// TestGrimtoothNeedsHidingAndSplashes checks AS_GRIMTOOTH's requirement
// of skill_db (Hiding), SkillGrimtooth's 3x3 splash around the target
// and its Quagmire on monsters.
func TestGrimtoothNeedsHidingAndSplashes(t *testing.T) {
	z := goldenZone(t, reference.Renewal, 1002, 1002, 1002)
	mobs := assassinMobs(z)
	target, next, away := mobs[0], mobs[1], mobs[2]
	target.x, target.y = 20, 23
	next.x, next.y = 21, 24
	away.x, away.y = 22, 25
	for _, m := range mobs {
		sturdy(m)
	}
	p := assassinIn(t, z, 12, 1250, map[int]int{skill.IDGrimtooth: 5, skill.IDHiding: 10})
	me := p.entity
	cast(z, p, skill.IDGrimtooth, 5, target)
	ev := run(z, p, 50, 500)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("Grimtooth without hiding: failures %v", f)
	}
	cast(z, p, skill.IDHiding, 10, me)
	run(z, p, 550, 1000)
	if !me.sc.Has(sc.SCHiding) {
		t.Fatal("not hidden")
	}
	hp := [3]int64{target.hp, next.hp, away.hp}
	cast(z, p, skill.IDGrimtooth, 5, target)
	ev = run(z, p, 1050, 1500)
	if len(skillDamage(ev, skill.IDGrimtooth)) != 2 {
		t.Fatalf("Grimtooth hits: %v", skillDamage(ev, skill.IDGrimtooth))
	}
	if target.hp >= hp[0] || next.hp >= hp[1] || away.hp != hp[2] {
		t.Errorf("HP %v -> %d %d %d", hp, target.hp, next.hp, away.hp)
	}
	if !target.sc.Has(sc.SCQuagmire) || !next.sc.Has(sc.SCQuagmire) {
		t.Error("no Quagmire on the monsters hit")
	}
	if !me.sc.Has(sc.SCHiding) {
		t.Error("Grimtooth ended Hiding")
	}
}

// TestCreateDeadlyPoison checks SkillCreateDeadlyPoison and
// skill_produce_mix for ASC_CDP: the materials of produce_db.txt make a
// Poison Bottle, and a player without them fails.
func TestCreateDeadlyPoison(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	p := assassinIn(t, z, 4013, 0, map[int]int{skill.IDCreateDeadly: 1})
	for _, id := range deadlyPoisonRecipe {
		give(t, z, p, id, 1)
	}
	cast(z, p, skill.IDCreateDeadly, 1, p.entity)
	run(z, p, 50, 6000)
	if z.countItem(p, itemPoisonBottle) != 1 {
		t.Fatal("no Poison Bottle")
	}
	for _, id := range deadlyPoisonRecipe {
		if z.countItem(p, id) != 0 {
			t.Errorf("material %d not used", id)
		}
	}
	cast(z, p, skill.IDCreateDeadly, 1, p.entity)
	ev := run(z, p, 6050, 7000)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_STUFF_INSUFFICIENT {
		t.Errorf("without materials: failures %v", f)
	}
}

// TestMeteorAssaultSplashesAroundCaster checks SkillMeteorAssault: a
// self skill whose splash (5x5) hits the enemies around the caster.
func TestMeteorAssaultSplashesAroundCaster(t *testing.T) {
	z := goldenZone(t, reference.Renewal, 1002, 1002)
	mobs := assassinMobs(z)
	in, out := mobs[0], mobs[1]
	in.x, in.y = 22, 22
	out.x, out.y = 23, 20
	sturdy(in)
	sturdy(out)
	p := assassinIn(t, z, 4013, 1250, map[int]int{skill.IDMeteorAssault: 10})
	hp := [2]int64{in.hp, out.hp}
	cast(z, p, skill.IDMeteorAssault, 10, p.entity)
	ev := run(z, p, 50, 1500)
	if len(skillDamage(ev, skill.IDMeteorAssault)) != 1 || in.hp >= hp[0] || out.hp != hp[1] {
		t.Fatalf("Meteor Assault hits %v, HP %v -> %d %d", skillDamage(ev, skill.IDMeteorAssault), hp, in.hp, out.hp)
	}
}
