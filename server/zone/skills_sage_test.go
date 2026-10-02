package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Hand tests of the Sage and Professor skills: the behavior that
// skill_status.json and skill_damage.json do not record. Each names the
// rAthena code it checks.

// Items the Sage's skills use.
const (
	itemYellowGemstone = 715
	itemCobweb         = 1025
)

// sageZone is skillZone with a Sage or Professor, the gemstones and
// cobwebs in the item table, and the rolls of forcedMinSource, so every
// chance succeeds.
func sageZone(t *testing.T, job int, skills map[int]int) (*Zone, *Player, *entity) {
	t.Helper()
	z, p, m := skillZone(t, job, skills)
	z.Items = item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: itemYellowGemstone, AegisName: "Yellow_Gemstone", Type: "Etc", Weight: 30, Stackable: true},
		{ID: itemRedGemstone, AegisName: "Red_Gemstone", Type: "Etc", Weight: 30, Stackable: true},
		{ID: itemBlueGemstone, AegisName: "Blue_Gemstone", Type: "Etc", Weight: 30, Stackable: true},
		{ID: itemCobweb, AegisName: "Cobweb", Type: "Etc", Weight: 10, Stackable: true},
	}})
	z.rand = rng.New(forcedMinSource{})
	dummy(m)
	return z, p, m
}

// TestVolcanoGivesStatusInside checks SkillVolcano and the onplace and
// onleft of UNT_VOLCANO (skill_unit_onplace, skill_unit_onleft): a unit
// in the field gets SC_VOLCANO with status_change_start's values, which
// status_calc_batk adds to ATK, and loses it when it walks out.
func TestVolcanoGivesStatusInside(t *testing.T) {
	z, p, _ := sageZone(t, 16, map[int]int{skill.IDVolcano: 5})
	e := p.entity
	before := e.status.BAtk
	give(t, z, p, itemBlueGemstone, 1)
	d := z.Skills.Get(skill.IDVolcano)
	z.castendPos(e, e.x, e.y, d, 5, 0)
	s := e.sc.Get(sc.SCVolcano)
	if s == nil {
		t.Fatal("no SC_VOLCANO on the caster's cell")
	}
	// Renewal: ATK 5 + 5 per level, and 20% more fire damage at level 5.
	if s.Val2 != 30 || s.Val3 != 20 {
		t.Errorf("SC_VOLCANO values %d %d, want 30 20", s.Val2, s.Val3)
	}
	if e.status.BAtk != before+30 {
		t.Errorf("BATK %d, want %d", e.status.BAtk, before+30)
	}
	if e.status.Changes == nil || e.status.Changes.VolcanoRate != 20 {
		t.Errorf("fire bonus %+v", e.status.Changes)
	}
	if z.countItem(p, itemBlueGemstone) != 0 {
		t.Error("Volcano kept its gemstone")
	}
	// The 7x7 field ends 3 cells away.
	e.x += 4
	z.skillUnitMove(e)
	if e.sc.Has(sc.SCVolcano) {
		t.Error("SC_VOLCANO stayed outside the field")
	}
	e.x -= 2
	z.skillUnitMove(e)
	if !e.sc.Has(sc.SCVolcano) {
		t.Error("no SC_VOLCANO back inside the field")
	}
}

// TestElementFieldTakesOverTheOld checks SkillDeluge::castendPos2 and
// skill_unitsetting for the element fields: a field cast while the
// caster's last one lasts replaces it, keeps the gemstone, and keeps the
// time the old one had left.
func TestElementFieldTakesOverTheOld(t *testing.T) {
	z, p, _ := sageZone(t, 16, map[int]int{skill.IDVolcano: 5, skill.IDDeluge: 5})
	e := p.entity
	give(t, z, p, itemBlueGemstone, 2)
	z.castendPos(e, e.x, e.y, z.Skills.Get(skill.IDVolcano), 5, 0)
	z.now += 10000
	z.castendPos(e, e.x, e.y, z.Skills.Get(skill.IDDeluge), 5, 0)
	if z.countItem(p, itemBlueGemstone) != 1 {
		t.Errorf("%d gemstones left, want 1", z.countItem(p, itemBlueGemstone))
	}
	if len(e.groups) != 1 || e.groups[0].def.ID != skill.IDDeluge {
		t.Fatalf("groups %v", e.groups)
	}
	if g := e.groups[0]; g.limit != 300000-10000 {
		t.Errorf("Deluge lasts %d, want the 290000 Volcano had left", g.limit)
	}
	if e.sc.Has(sc.SCVolcano) || !e.sc.Has(sc.SCDeluge) {
		t.Errorf("statuses: volcano %v deluge %v", e.sc.Has(sc.SCVolcano), e.sc.Has(sc.SCDeluge))
	}
	// status_get_hpbonus: Deluge's max HP percent, 15 at level 5.
	if v := e.sc.Val2(sc.SCDeluge); v != 15 {
		t.Errorf("Deluge max HP %d%%, want 15", v)
	}
}

// TestViolentGaleFlee checks SC_VIOLENTGALE in status_calc_flee: 3 FLEE
// per level in renewal.
func TestViolentGaleFlee(t *testing.T) {
	z, p, _ := sageZone(t, 16, map[int]int{skill.IDViolentGale: 5})
	e := p.entity
	before := e.status.Flee
	give(t, z, p, itemBlueGemstone, 1)
	z.castendPos(e, e.x, e.y, z.Skills.Get(skill.IDViolentGale), 5, 0)
	if e.status.Flee != before+15 {
		t.Errorf("FLEE %d, want %d", e.status.Flee, before+15)
	}
}

// TestFieldBonusInAttrFix checks battle_attr_fix with SC_VOLCANO on the
// attacker: renewal adds val3 to the element's percent, here 20% more
// fire damage.
func TestFieldBonusInAttrFix(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDFireBolt: 1})
	e := p.entity
	z.recalcMonster(m)
	bolt := z.Skills.Get(skill.IDFireBolt)
	base := z.calcSkillAttack(battle.TypeMagic, e, e, m, bolt, 1, 0, nil).Damage
	st := *e.status
	st.Changes = &battle.Changes{AtkPercent: 100, VolcanoRate: 20}
	e.status = &st
	got := z.calcSkillAttack(battle.TypeMagic, e, e, m, bolt, 1, 0, nil).Damage
	if got != base+base*20/100 {
		t.Errorf("Fire Bolt %d in Volcano, want %d", got, base+base*20/100)
	}
	cold := z.calcSkillAttack(battle.TypeMagic, e, e, m, z.Skills.Get(skill.IDColdBolt), 1, 0, nil).Damage
	if cold != base {
		t.Errorf("Cold Bolt %d in Volcano, want %d", cold, base)
	}
}

// TestLandProtectorRemovesAndBlocks checks skill_cell_overlap for
// SA_LANDPROTECTOR: it removes the units under it, and new units cannot
// be placed on it.
func TestLandProtectorRemovesAndBlocks(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDLandProtector: 1})
	e := p.entity
	sw := z.Skills.Get(skill.IDSafetyWall)
	if z.unitSetting(m, sw, 1, e.x+1, e.y) == nil {
		t.Fatal("no Safety Wall")
	}
	give(t, z, p, itemBlueGemstone, 1)
	give(t, z, p, itemYellowGemstone, 1)
	z.castendPos(e, e.x, e.y, z.Skills.Get(skill.IDLandProtector), 1, 0)
	for _, g := range z.groups {
		if g.def.ID == skill.IDSafetyWall {
			t.Fatal("Safety Wall stayed under Magnetic Earth")
		}
	}
	if !z.landProtected(e.x+1, e.y) || !z.landProtected(e.x-3, e.y-3) || z.landProtected(e.x+4, e.y) {
		t.Error("Magnetic Earth's 7x7 cells are wrong")
	}
	if z.unitSetting(m, sw, 1, e.x+1, e.y) != nil {
		t.Error("Safety Wall was placed on Magnetic Earth")
	}
	if z.unitSetting(m, z.Skills.Get(skill.IDVolcano), 1, e.x, e.y) != nil {
		t.Error("Volcano was placed on Magnetic Earth")
	}
	// Teleport fails on it (status_check_skilluse).
	if z.statusCheckSkillUse(e, nil, z.Skills.Get(skill.IDTeleport), 0) {
		t.Error("Teleport works on Magnetic Earth")
	}
}

// TestDispellEndsStatuses checks SkillDispell: the target's statuses end,
// but those with NoDispell, and a monster drops its target.
func TestDispellEndsStatuses(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDDispell: 5})
	z.recalcMonster(m)
	z.scStart(m, m, sc.SCBlessing, 100, 10, 60000)
	z.scStart4(m, m, sc.SCElementalchange, 100, 1, 3, 0, 0, 60000)
	m.target = p.entity
	if !m.sc.Has(sc.SCBlessing) || !m.sc.Has(sc.SCElementalchange) {
		t.Fatal("statuses did not start")
	}
	z.castendNoDamage(p.entity, m, z.Skills.Get(skill.IDDispell), 5, 0)
	if m.sc.Has(sc.SCBlessing) {
		t.Error("Blessing stayed")
	}
	if !m.sc.Has(sc.SCElementalchange) {
		t.Error("Elemental Change, which has NoDispell, ended")
	}
	if m.target != nil {
		t.Error("the monster kept its target (mob_unlocktarget)")
	}
}

// TestMagicRodAbsorbsMagic checks skill_attack with SC_MAGICROD: a
// magic skill cast at the unit misses, and the unit gains val2 percent of
// its SP.
func TestMagicRodAbsorbsMagic(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDMagicRod: 5})
	e := p.entity
	z.castendNoDamage(e, e, z.Skills.Get(skill.IDMagicRod), 5, 0)
	if e.sc.Val2(sc.SCMagicrod) != 100 {
		t.Fatalf("SC_MAGICROD val2 %d, want 100", e.sc.Val2(sc.SCMagicrod))
	}
	p.sp = 0
	hp := e.hp
	m.status.MAtkMin, m.status.MAtkMax = 500, 500
	bolt := z.Skills.Get(skill.IDFireBolt)
	if dmg := z.skillAttack(battle.TypeMagic, m, m, e, bolt, 5, 0); dmg != 0 {
		t.Errorf("Fire Bolt dealt %d through Magic Rod", dmg)
	}
	if e.hp != hp {
		t.Errorf("HP %d -> %d", hp, e.hp)
	}
	if want := int64(bolt.Requires.SP.At(5)); p.sp != want {
		t.Errorf("SP %d, want %d", p.sp, want)
	}
}

// TestSpellBreakerCancelsCast checks SkillSpellBreaker: the target's cast
// stops, it loses the skill's SP, the caster gets 25% per level above 1
// back, and level 5 takes 2% of a monster's max HP, half of which heals
// the caster.
func TestSpellBreakerCancelsCast(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDSpellBreaker: 5})
	e := p.entity
	bolt := z.Skills.Get(skill.IDFireBolt)
	m.cast = &casting{def: bolt, lv: 5, target: e, end: z.now + 5000}
	p.sp = 0
	e.hp = 10
	z.castendNoDamage(e, m, z.Skills.Get(skill.IDSpellBreaker), 5, 0)
	if m.cast != nil {
		t.Error("the monster's cast went on")
	}
	if want := int64(bolt.Requires.SP.At(5)) * 100 / 100; p.sp != want {
		t.Errorf("SP %d, want %d", p.sp, want)
	}
	if m.hp != 100000-2000 {
		t.Errorf("monster HP %d, want %d", m.hp, 100000-2000)
	}
	if e.hp != 10+1000 {
		t.Errorf("caster HP %d, want %d", e.hp, 1010)
	}
	// Nothing to cancel: nothing happens.
	sp := p.sp
	z.castendNoDamage(e, m, z.Skills.Get(skill.IDSpellBreaker), 5, 0)
	if p.sp != sp || m.hp != 100000-2000 {
		t.Error("Spell Breaker worked without a cast")
	}
}

// TestCastCancelStopsOwnCast checks SkillCastCancel and
// clif_parse_UseSkillToId: Cast Cancel works during a cast, stops it,
// and takes 90% of the stopped skill's SP at level 1.
func TestCastCancelStopsOwnCast(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDFireBolt: 10, skill.IDCastCancel: 1})
	cast(z, p, skill.IDFireBolt, 10, m)
	z.stepAt(100)
	if p.entity.cast == nil {
		t.Fatal("no Fire Bolt cast")
	}
	sp := p.sp
	cast(z, p, skill.IDCastCancel, 1, nil)
	z.stepAt(150)
	if p.entity.cast != nil {
		t.Fatal("the cast went on")
	}
	bolt := z.Skills.Get(skill.IDFireBolt)
	cost := int64(z.Skills.Get(skill.IDCastCancel).Requires.SP.At(1))
	want := sp - cost - int64(bolt.Requires.SP.At(10))*90/100
	if p.sp != want {
		t.Errorf("SP %d, want %d", p.sp, want)
	}
	// Without a cast, Cast Cancel fails at the start.
	sp = p.sp
	cast(z, p, skill.IDCastCancel, 1, nil)
	z.stepAt(5000)
	if p.sp != sp {
		t.Error("Cast Cancel without a cast cost SP")
	}
}

// TestFreeCastWalksWhileCasting checks unit_can_move and
// status_calc_speed with SA_FREECAST: a player with Free Cast walks
// during a cast, at 175% - 5% per level of its speed, and gets its speed
// back when the cast ends. Without Free Cast the walk is refused.
func TestFreeCastWalksWhileCasting(t *testing.T) {
	for _, lv := range []int{0, 10} {
		z, p, m := sageZone(t, 16, map[int]int{skill.IDFireBolt: 10, skill.IDFreeCast: lv})
		e := p.entity
		speed := e.speed
		e.x, e.y = m.x+5, m.y
		cast(z, p, skill.IDFireBolt, 10, m)
		z.stepAt(100)
		if e.cast == nil {
			t.Fatal("no cast")
		}
		z.handle(command{player: p, walkTo: &[2]int{e.x, e.y + 5}})
		walking := len(e.path) > 0
		if lv == 0 {
			if walking {
				t.Error("walked during a cast without Free Cast")
			}
			continue
		}
		if !walking {
			t.Fatal("no walk during the cast with Free Cast")
		}
		if want := speed * (175 - 5*lv) / 100; e.speed != want {
			t.Errorf("speed %d during the cast, want %d", e.speed, want)
		}
		z.castCancel(e, false)
		if e.speed != speed {
			t.Errorf("speed %d after the cast, want %d", e.speed, speed)
		}
	}
}

// TestDoubleCastingSecondBolt checks skill_attack with SC_DOUBLECAST: a
// bolt strikes again after the attack motion (skill_addtimerskill).
func TestDoubleCastingSecondBolt(t *testing.T) {
	z, p, m := sageZone(t, 4017, map[int]int{skill.IDFireBolt: 1, skill.IDDoubleCasting: 5})
	e := p.entity
	z.castendNoDamage(e, e, z.Skills.Get(skill.IDDoubleCasting), 5, 0)
	if e.sc.Val2(sc.SCDoublecast) != 80 {
		t.Fatalf("SC_DOUBLECAST val2 %d, want 80", e.sc.Val2(sc.SCDoublecast))
	}
	hp := m.hp
	z.castendDamage(e, m, z.Skills.Get(skill.IDFireBolt), 1, 0)
	first := hp - m.hp
	if first <= 0 || len(z.timerSkills) != 1 {
		t.Fatalf("damage %d, %d timer skills", first, len(z.timerSkills))
	}
	ev := run(z, p, z.now+50, z.now+int64(e.status.AMotion)+100)
	if hp-m.hp != 2*first {
		t.Errorf("damage %d after the second bolt, want %d", hp-m.hp, 2*first)
	}
	if len(skillDamage(ev, skill.IDFireBolt)) == 0 {
		t.Error("no second Fire Bolt hit shown")
	}
	// The second bolt does not roll again.
	if len(z.timerSkills) != 0 {
		t.Error("the second bolt cast a third")
	}
}

// TestElementalChangeOnMonsters checks SkillElementalChangeFire: a
// player changes a monster's element, but not a player's.
func TestElementalChangeOnMonsters(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDElementFire: 1})
	z.recalcMonster(m)
	d := z.Skills.Get(skill.IDElementFire)
	z.castendNoDamage(p.entity, m, d, 1, 0)
	if m.sc.Val2(sc.SCElementalchange) != battle.Fire || m.status.DefEle != battle.Fire {
		t.Errorf("monster element %d, status %d", m.status.DefEle, m.sc.Val2(sc.SCElementalchange))
	}
	z.castendNoDamage(p.entity, p.entity, d, 1, 0)
	if p.entity.sc.Has(sc.SCElementalchange) {
		t.Error("a player changed a player's element")
	}
}

// TestEndowFailsWithoutWeapon checks SkillEndowBlaze: a player without a
// weapon cannot take an endow.
func TestEndowFailsWithoutWeapon(t *testing.T) {
	z, p, _ := sageZone(t, 16, map[int]int{skill.IDFlameLauncher: 5})
	z.castendNoDamage(p.entity, p.entity, z.Skills.Get(skill.IDFlameLauncher), 5, 0)
	if p.entity.sc.Has(sc.SCFireweapon) {
		t.Error("an endow on bare fists")
	}
}

// TestBlindingMistBlindsEnemies checks SkillBlindingMist and UNT_FOGWALL:
// the 5x3 mist gives SC_FOGWALL to the units in it and blinds enemies.
func TestBlindingMistBlindsEnemies(t *testing.T) {
	z, p, m := sageZone(t, 4017, map[int]int{skill.IDFogWall: 1})
	z.recalcMonster(m)
	g := z.unitSetting(p.entity, z.Skills.Get(skill.IDFogWall), 1, m.x, m.y)
	if g == nil || len(g.units) != 15 {
		t.Fatalf("mist %v", g)
	}
	if !m.sc.Has(sc.SCFogwall) || !m.sc.Has(sc.SCBlind) {
		t.Errorf("monster in the mist: fog %v blind %v", m.sc.Has(sc.SCFogwall), m.sc.Has(sc.SCBlind))
	}
	if m.status.Changes == nil || !m.status.Changes.FogWall {
		t.Error("no FogWall in the monster's battle changes")
	}
	// Leaving the mist: the monster stays blind 30 more seconds.
	m.x += 3
	z.skillUnitMove(m)
	if m.sc.Has(sc.SCFogwall) {
		t.Error("SC_FOGWALL outside the mist")
	}
	if e := m.sc.Get(sc.SCBlind); e == nil || e.End != z.now+30000 {
		t.Errorf("blindness after the mist: %+v", e)
	}
}

// TestFiberLockHoldsAndBurns checks SkillFiberLock and UNT_SPIDERWEB: the
// web holds the target on its cell, halves its FLEE, and fire deals
// double and burns the web (battle_attr_fix).
func TestFiberLockHoldsAndBurns(t *testing.T) {
	z, p, m := sageZone(t, 4017, map[int]int{skill.IDSpiderWeb: 1})
	m.monster.spawn.Status.Flee = 100
	z.recalcMonster(m)
	flee := m.status.Flee
	give(t, z, p, itemCobweb, 1)
	p.entity.x = m.x + 3
	if !z.castEndID(p.entity, &casting{def: z.Skills.Get(skill.IDSpiderWeb), lv: 1, target: m}, false) {
		t.Fatal("Fiber Lock failed")
	}
	e := m.sc.Get(sc.SCSpiderweb)
	if e == nil {
		t.Fatal("no SC_SPIDERWEB")
	}
	if !m.cant().Move {
		t.Error("the web does not hold")
	}
	if m.status.Flee != flee-flee*50/100 {
		t.Errorf("FLEE %d, want half of %d", m.status.Flee, flee)
	}
	if z.countItem(p, itemCobweb) != 0 {
		t.Error("the cobweb was kept")
	}
	tgt := z.defender(m)
	if tgt.Changes == nil || tgt.Changes.SpiderWeb == nil || !tgt.Changes.SpiderWeb() {
		t.Fatal("fire does not burn the web")
	}
	if m.sc.Has(sc.SCSpiderweb) {
		t.Error("the web stayed after the fire")
	}
	for _, g := range z.groups {
		if g.def.ID == skill.IDSpiderWeb {
			t.Error("the web's unit stayed after the fire")
		}
	}
}

// TestHindsightCastsOnAttack checks SC_AUTOSPELL in battle_weapon_attack:
// a normal attack may cast the status's bolt on the target for two
// thirds of its SP.
func TestHindsightCastsOnAttack(t *testing.T) {
	z, p, m := sageZone(t, 16, map[int]int{skill.IDAutoSpell: 10, skill.IDFireBolt: 3})
	e := p.entity
	e.x = m.x + 1
	z.scStart4(e, e, sc.SCAutospell, 100, 10, skill.IDFireBolt, 3, 0, 60000)
	if e.sc.Val4(sc.SCAutospell) != 20 {
		t.Fatalf("chance %d, want 20", e.sc.Val4(sc.SCAutospell))
	}
	sp := p.sp
	hp := m.hp
	z.strike(e, m)
	if m.hp >= hp {
		t.Error("no damage")
	}
	// The forced roll keeps level 3.
	want := sp - int64(z.Skills.Get(skill.IDFireBolt).Requires.SP.At(3))*2/3
	if p.sp != want {
		t.Errorf("SP %d, want %d", p.sp, want)
	}
	// A player's Hindsight waits for its window.
	if r := (hindsight{}).castBegin(z, e, z.Skills.Get(skill.IDAutoSpell), 10); r == 0 {
		t.Error("Hindsight starts without its window")
	}
}

// TestSoulSiphonDrainsSP checks SkillSoulSiphon: the target, a player,
// loses all its SP. On a monster the cast fails (skill_castend_id_check).
func TestSoulSiphonDrainsSP(t *testing.T) {
	z, p, m := sageZone(t, 4017, map[int]int{skill.IDSoulBurn: 4})
	other := add(z, "b")
	other.entity.x, other.entity.y = p.entity.x+1, p.entity.y
	z.recalc(other)
	other.sp = int64(other.entity.status.MaxSP)
	z.castendDamage(p.entity, other.entity, z.Skills.Get(skill.IDSoulBurn), 4, 0)
	if other.sp != 0 {
		t.Errorf("SP %d after Soul Siphon", other.sp)
	}
	if r := z.castEndIDCheck(p.entity, m, z.Skills.Get(skill.IDSoulBurn)); r == 0 {
		t.Error("Soul Siphon works on a monster")
	}
}

// TestIndulgeTurnsHPIntoSP checks SkillIndulge: 10% of max HP becomes
// that times the level in SP.
func TestIndulgeTurnsHPIntoSP(t *testing.T) {
	z, p, _ := sageZone(t, 4017, map[int]int{skill.IDHPConversion: 5})
	e := p.entity
	p.sp = 0
	hp := e.hp
	z.castendNoDamage(e, e, z.Skills.Get(skill.IDHPConversion), 5, 0)
	cost := int64(e.status.MaxHP / 10)
	if e.hp != hp-cost {
		t.Errorf("HP %d, want %d", e.hp, hp-cost)
	}
	if p.sp != min(cost*5, int64(e.status.MaxSP)) {
		t.Errorf("SP %d, want %d", p.sp, min(cost*5, int64(e.status.MaxSP)))
	}
}
