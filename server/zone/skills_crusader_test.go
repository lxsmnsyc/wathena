package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Hand tests of the Crusader and Paladin skills: the blocks, reflects,
// areas, attacks and failures that the recordings do not cover.

// guardShield is the Guard (2101), a shield of both modes.
const guardShield = 2101

// crusaderSkills are every Crusader and Paladin skill at its top level,
// with the Swordman and Knight skills they learn.
var crusaderSkills = [][2]int{
	{skill.IDBasic, 9}, {skill.IDSwordMastery, 10}, {skill.IDBash, 10}, {skill.IDSpearMastery, 10},
	{skill.IDRiding, 1}, {skill.IDCavalierMastery, 5}, {skill.IDFaith, 10}, {skill.IDAutoGuard, 10},
	{skill.IDSmite, 5}, {skill.IDShieldBoomerang, 5}, {skill.IDReflectShield, 10}, {skill.IDHolyCross, 10},
	{skill.IDGrandCross, 10}, {skill.IDDevotion, 5}, {skill.IDProvidence, 5}, {skill.IDDefender, 5},
	{skill.IDSpearQuicken, 10}, {skill.IDShrink, 1}, {skill.IDPressure, 5}, {skill.IDSacrifice, 5},
	{skill.IDGospel, 10}, {skill.IDShieldChain, 5},
}

// crusaderZone is a golden zone of a mode with a level 99 Paladin at
// (20, 20), and Golems with a lot of HP that cannot move. Rolls are
// forced to their lowest value. With shield set, the Paladin wears a
// Guard.
func crusaderZone(t *testing.T, mode reference.Mode, shield bool, golems int) (*Zone, *Player, []*entity) {
	t.Helper()
	mobs := make([]uint32, golems)
	for i := range mobs {
		mobs[i] = 1040
	}
	z := goldenZone(t, mode, mobs...)
	p := goldenPlayerIn(t, z, goldenPlayer{Job: 4015, Build: "melee", Sex: 1, JobLevel: 50, Skills: crusaderSkills}, 0, 0)
	if shield {
		equip(z, p, give(t, z, p, guardShield, 1))
		z.flushStats()
		events(p)
	}
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
		// They do not strike back, so only the skills deal damage.
		st.Mode &^= modeCanAttack
		out = append(out, e)
	}
	return z, p, out
}

// TestShieldRequirement checks ST_SHIELD in
// skill_check_condition_castbegin and pc_checkallowskill: Guard fails
// without a shield, starts with one, and ends when the shield comes
// off.
func TestShieldRequirement(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.Renewal, false, 0)
	cast(z, p, skill.IDAutoGuard, 10, nil)
	ev := run(z, p, 100, 300)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("without a shield: failures %v", f)
	}
	if p.entity.sc.Has(sc.SCAutoguard) {
		t.Fatal("Guard started without a shield")
	}
	slot := give(t, z, p, guardShield, 1)
	equip(z, p, slot)
	cast(z, p, skill.IDAutoGuard, 10, nil)
	run(z, p, 400, 600)
	if e := p.entity.sc.Get(sc.SCAutoguard); e == nil || e.Val2 != 30 {
		t.Fatalf("with a shield: Guard %+v", e)
	}
	unequip(z, p, slot)
	if p.entity.sc.Has(sc.SCAutoguard) {
		t.Error("Guard stayed after the shield came off")
	}
}

// TestSpearQuickenNeedsSpear checks pc_checkallowskill's weapon part:
// Spear Quicken ends when the spear comes off.
func TestSpearQuickenNeedsSpear(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.Renewal, false, 0)
	slot := give(t, z, p, javelin, 1)
	equip(z, p, slot)
	cast(z, p, skill.IDSpearQuicken, 10, nil)
	run(z, p, 100, 300)
	if !p.entity.sc.Has(sc.SCSpearquicken) {
		t.Fatal("Spear Quicken did not start with a spear")
	}
	unequip(z, p, slot)
	if p.entity.sc.Has(sc.SCSpearquicken) {
		t.Error("Spear Quicken stayed after the spear came off")
	}
}

// TestAutoGuardBlocks checks SC_AUTOGUARD in battle_status_block_damage:
// a weapon hit misses by its chance and the guard cannot walk for a
// moment. With Shrink in pre-renewal, the attacker is pushed 2 cells.
func TestAutoGuardBlocks(t *testing.T) {
	z, p, m := crusaderZone(t, reference.PreRenewal, true, 1)
	src := p.entity
	m[0].x, m[0].y = 20, 21
	z.scStart(src, src, sc.SCAutoguard, 100, 10, 300000)
	noLuckyDodge(src)
	hp := src.hp
	z.strike(m[0], src)
	if src.hp != hp {
		t.Errorf("Guard: HP %d -> %d", hp, src.hp)
	}
	if src.canMove <= z.now {
		t.Error("Guard set no walk delay")
	}
	z.scStart(src, src, sc.SCShrink, 100, 1, 300000)
	noLuckyDodge(src)
	z.strike(m[0], src)
	if m[0].y != 23 {
		t.Errorf("Shrink: attacker at (%d, %d), want (20, 23)", m[0].x, m[0].y)
	}
	// Magic passes Guard.
	fb := z.Skills.Get(skill.IDFireBolt)
	if z.skillAttack(battle.TypeMagic, m[0], m[0], src, fb, 1, 0) <= 0 {
		t.Error("Guard blocked a magic hit")
	}
}

// TestAutoGuardShrinkRenewal checks renewal's Shrink: a guarded hit tries
// to stun the attacker for skill_get_time2 of the attacking skill. A
// normal attack has none, so the stun lasts only the 500 ms that
// renewal's status_get_sc_def adds to a stun; Smite's lasts its
// Duration2 more.
func TestAutoGuardShrinkRenewal(t *testing.T) {
	z, p, m := crusaderZone(t, reference.Renewal, true, 1)
	src := p.entity
	m[0].x, m[0].y = 20, 21
	z.scStart(src, src, sc.SCAutoguard, 100, 10, 300000)
	z.scStart(src, src, sc.SCShrink, 100, 1, 300000)
	noLuckyDodge(src)
	z.strike(m[0], src)
	if e := m[0].sc.Get(sc.SCStun); e != nil && e.Timer()-z.now > 500 || m[0].y != 21 {
		t.Errorf("normal attack: stun %+v, at (%d, %d)", e, m[0].x, m[0].y)
	}
	z.scEnd(m[0], sc.SCStun)
	smite := z.Skills.Get(skill.IDSmite)
	z.skillAttack(battle.TypeWeapon, m[0], m[0], src, smite, 5, 0)
	if e := m[0].sc.Get(sc.SCStun); e == nil || e.Timer()-z.now <= 500 {
		t.Errorf("Smite on a guard with Shrink: stun %+v", e)
	}
}

// noLuckyDodge takes away a player's lucky dodge, which the lowest
// rolls would always grant.
func noLuckyDodge(e *entity) {
	st := *e.status
	st.Flee2 = 0
	e.status = &st
}

// TestReflectShield checks battle_calc_return_damage: a melee hit on a
// unit under Shield Reflect returns val2 percent to the attacker; a
// ranged one returns nothing.
func TestReflectShield(t *testing.T) {
	for _, mode := range reference.Modes {
		z, p, m := crusaderZone(t, mode, true, 1)
		src := p.entity
		m[0].x, m[0].y = 20, 21
		z.scStart(src, src, sc.SCReflectshield, 100, 10, 300000)
		noLuckyDodge(src)
		hp := src.hp
		z.strike(m[0], src)
		taken := hp - src.hp
		if taken <= 0 {
			t.Fatalf("%s: the hit dealt nothing", mode)
		}
		want := taken * 40 / 100
		if got := 1000000 - m[0].hp; got != want {
			t.Errorf("%s: reflected %d, want %d of %d", mode, got, want, taken)
		}
		// A ranged attacker gets nothing back.
		st := *m[0].status
		st.RHW.Range = 9
		m[0].status = &st
		before := m[0].hp
		z.strike(m[0], src)
		if m[0].hp != before {
			t.Errorf("%s: a ranged hit was reflected", mode)
		}
	}
}

// TestGrandCross checks SkillGrandCross and the unit's
// skill_unit_onplace_timer: the cross hits the enemies on its cells,
// each cell at most 3 times, and not those off it. In pre-renewal it
// hits its caster too (TargetSelf); in renewal it does not. The caster
// cannot move or change its shield while it lasts.
func TestGrandCross(t *testing.T) {
	for _, mode := range reference.Modes {
		z, p, m := crusaderZone(t, mode, true, 2)
		src := p.entity
		m[0].x, m[0].y = 20, 22 // on the cross
		m[1].x, m[1].y = 22, 22 // off it
		cast(z, p, skill.IDGrandCross, 10, nil)
		ev := run(z, p, 100, 6000)
		if !wasHit(m[0]) || wasHit(m[1]) {
			t.Errorf("%s: on the cross hit %v, off it hit %v", mode, wasHit(m[0]), wasHit(m[1]))
		}
		self := 0
		for _, d := range skillDamage(ev, skill.IDGrandCross) {
			if d.GetTarget() == p.netID(src) && d.GetAmount() > 0 {
				self++
			}
		}
		if mode == reference.PreRenewal && self == 0 {
			t.Errorf("%s: the cross did not hit its caster", mode)
		}
		if mode == reference.Renewal && self != 0 {
			t.Errorf("%s: the cross hit its caster %d times", mode, self)
		}
		if len(z.groups) != 0 {
			t.Errorf("%s: %d groups left", mode, len(z.groups))
		}
	}
	// The hits per cell, and the caster's statuses during the cross.
	z, p, m := crusaderZone(t, reference.Renewal, true, 1)
	m[0].x, m[0].y = 20, 22
	d := z.Skills.Get(skill.IDGrandCross)
	z.grandCrossCast(p.entity, d, 10)
	if !p.entity.sc.Has(sc.SCAnkle) || !p.entity.sc.Has(sc.SCStripshield) {
		t.Error("the caster is free to move during Grand Cross")
	}
	g := z.unitSetting(p.entity, d, 10, 20, 20)
	if g == nil || len(g.units) != 29 {
		t.Fatalf("Grand Cross cells: %v", g)
	}
	var cell *entity
	for _, u := range g.units {
		if u.x == 20 && u.y == 22 {
			cell = u
		}
	}
	if cell == nil || cell.unit.val1 != 3 {
		t.Fatalf("cell at (20, 22): %+v", cell)
	}
	hits := 0
	for range 5 {
		before := m[0].hp
		z.grandCrossOnPlace(cell, m[0])
		if m[0].hp < before {
			hits++
		}
		if !z.entities.has(cell) {
			break
		}
	}
	if hits != 3 || z.entities.has(cell) {
		t.Errorf("a cell hit %d times, still there %v", hits, z.entities.has(cell))
	}
}

// TestSacrificeHits checks SC_SACRIFICE in battle_weapon_attack: the next
// 5 normal attacks are Martyr's Reckoning hits that cost the attacker 9%
// of its max HP each, then the status ends.
func TestSacrificeHits(t *testing.T) {
	z, p, m := crusaderZone(t, reference.Renewal, false, 1)
	src := p.entity
	m[0].x, m[0].y = 20, 21
	d := z.Skills.Get(skill.IDSacrifice)
	z.castendNoDamage(src, src, d, 5, 0)
	e := src.sc.Get(sc.SCSacrifice)
	if e == nil || e.Val2 != 5 {
		t.Fatalf("SC_SACRIFICE %+v", e)
	}
	maxHP := int64(src.status.MaxHP)
	for i := range 5 {
		hp, mhp := src.hp, m[0].hp
		z.hits = nil
		z.strike(src, m[0])
		if src.hp != hp-maxHP*9/100 {
			t.Errorf("attack %d: HP %d -> %d, want a loss of %d", i+1, hp, src.hp, maxHP*9/100)
		}
		if m[0].hp >= mhp || len(z.hits) == 0 || z.hits[0].skill == nil || z.hits[0].skill.ID != skill.IDSacrifice {
			t.Errorf("attack %d: no Martyr's Reckoning hit", i+1)
		}
	}
	if src.sc.Has(sc.SCSacrifice) {
		t.Error("SC_SACRIFICE did not end after 5 attacks")
	}
	hp := src.hp
	z.hits = nil
	z.strike(src, m[0])
	if src.hp != hp || len(z.hits) != 1 || z.hits[0].skill != nil {
		t.Error("the sixth attack was not a normal attack")
	}
}

// TestGospel checks SkillBattleChant and UNT_GOSPEL: the caster gets
// SC_GOSPEL (BCT_SELF) and cannot move or use other skills; an enemy on
// the cross gets an offensive effect each 10 seconds (the lowest rolls:
// damage of 3000 less DEF); the caster gets none. Casting it again ends
// it at no cost, and the cross goes with it.
func TestGospel(t *testing.T) {
	z, p, m := crusaderZone(t, reference.Renewal, false, 1)
	src := p.entity
	m[0].x, m[0].y = 21, 21
	cast(z, p, skill.IDGospel, 10, nil)
	run(z, p, 100, 300)
	e := src.sc.Get(sc.SCGospel)
	if e == nil || e.Val4 != sc.BCTSelf || e.Val3 == 0 {
		t.Fatalf("SC_GOSPEL %+v", e)
	}
	if !src.cant().Move {
		t.Error("the caster can move")
	}
	if !wasHit(m[0]) {
		t.Error("the enemy on the cross took no damage")
	}
	if got := 1000000 - m[0].hp; got != int64(3000-m[0].status.Def-m[0].status.Def2) {
		t.Errorf("damage %d", got)
	}
	hp := src.hp
	run(z, p, 350, 600)
	if src.hp != hp {
		t.Error("the caster was hit by its own Battle Chant")
	}
	// Another skill fails while it lasts.
	if z.statusCheckSkillUse(src, src, z.Skills.Get(skill.IDAutoGuard), 0) {
		t.Error("Guard is usable during Battle Chant")
	}
	sp := p.sp
	cast(z, p, skill.IDGospel, 10, nil)
	run(z, p, 700, 900)
	if src.sc.Has(sc.SCGospel) || len(z.groups) != 0 {
		t.Errorf("recast: status %v, %d groups", src.sc.Has(sc.SCGospel), len(z.groups))
	}
	if p.sp != sp {
		t.Errorf("ending Battle Chant cost SP %d -> %d", sp, p.sp)
	}
}

// TestGospelNoParty checks UNT_GOSPEL's support side without a party: a
// player who is not the caster is no party member, so it gets no
// support effect, and no offensive one either.
func TestGospelNoParty(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.PreRenewal, false, 0)
	other := add(z, "other")
	other.entity.x, other.entity.y = 21, 20
	z.recalc(other)
	d := z.Skills.Get(skill.IDGospel)
	z.castendNoDamage(p.entity, p.entity, d, 10, 0)
	z.unitTimer()
	if !other.entity.sc.Empty() {
		t.Errorf("a player without a party got %d statuses", other.entity.sc.Len())
	}
}

// TestDevotionFails checks SkillSacrifice without a party: Devotion
// cannot target its caster (NoTargetSelf), and on another player the
// cast fails at skill_castend_id_check (PartyOnly), costing nothing but
// the cast.
func TestDevotionFails(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.Renewal, false, 0)
	other := add(z, "other")
	other.entity.x, other.entity.y = 21, 20
	z.recalc(other)
	if z.skillUseID(p.entity, p.entity, z.Skills.Get(skill.IDDevotion), 5) {
		t.Error("Devotion started on its caster")
	}
	cast(z, p, skill.IDDevotion, 5, other.entity)
	ev := run(z, p, 100, 3000)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Errorf("failures %v", f)
	}
	if other.entity.sc.Has(sc.SCDevotion) {
		t.Error("SC_DEVOTION started without a party")
	}
	// The skill's own checks, reached directly, fail too.
	z.castendNoDamage(p.entity, other.entity, z.Skills.Get(skill.IDDevotion), 5, 0)
	if other.entity.sc.Has(sc.SCDevotion) {
		t.Error("SkillSacrifice started SC_DEVOTION")
	}
}

// TestProvidence checks SkillResistantSouls: it fails on another
// Crusader and gives others 5% holy and demon resistance per level.
func TestProvidence(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.PreRenewal, false, 0)
	d := z.Skills.Get(skill.IDProvidence)
	crusader := add(z, "crusader")
	crusader.char.Job = 14
	z.recalc(crusader)
	z.castendNoDamage(p.entity, crusader.entity, d, 5, 0)
	if crusader.entity.sc.Has(sc.SCProvidence) {
		t.Error("Resistant Souls worked on a Crusader")
	}
	knight := add(z, "knight")
	knight.char.Job = 7
	z.recalc(knight)
	z.castendNoDamage(p.entity, knight.entity, d, 5, 0)
	z.flushStats()
	if !knight.entity.sc.Has(sc.SCProvidence) {
		t.Fatal("Resistant Souls did not start on a Knight")
	}
	pl := knight.entity.status.Player
	if pl.SubEle[battle.Holy] != 25 || pl.SubRace[battle.RaceDemon] != 25 {
		t.Errorf("resistances: holy %d, demon %d", pl.SubEle[battle.Holy], pl.SubRace[battle.RaceDemon])
	}
}

// TestPressureSP checks SkillGloriaDomini::applyAdditionalEffects in
// pre-renewal: the target loses 15% + 5% per level of its SP.
func TestPressureSP(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.PreRenewal, false, 0)
	other := add(z, "other")
	other.entity.x, other.entity.y = 21, 20
	z.recalc(other)
	other.sp = 1000
	other.entity.hp = 1000000
	d := z.Skills.Get(skill.IDPressure)
	z.castendDamage(p.entity, other.entity, d, 5, 0)
	if other.sp != 600 {
		t.Errorf("SP %d, want 600", other.sp)
	}
}

// TestSmiteAndHolyCross checks SkillSmite (stun and knockback) and
// SkillHolyCross (blind), with the lowest rolls.
func TestSmiteAndHolyCross(t *testing.T) {
	z, p, m := crusaderZone(t, reference.Renewal, true, 2)
	src := p.entity
	m[0].x, m[0].y = 20, 21
	m[1].x, m[1].y = 21, 20
	z.castendDamage(src, m[0], z.Skills.Get(skill.IDSmite), 5, 0)
	if !m[0].sc.Has(sc.SCStun) || m[0].y <= 21 {
		t.Errorf("Smite: stun %v, at (%d, %d)", m[0].sc.Has(sc.SCStun), m[0].x, m[0].y)
	}
	z.castendDamage(src, m[1], z.Skills.Get(skill.IDHolyCross), 10, 0)
	if !m[1].sc.Has(sc.SCBlind) {
		t.Error("Holy Cross did not blind")
	}
}
