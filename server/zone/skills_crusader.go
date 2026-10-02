package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The Crusader and Paladin skills, as the SkillImpl classes of rAthena's
// src/map/skills/swordman, with the parts of battle.cpp, skill.cpp and
// pc.cpp that their statuses run: Guard and Shrink
// (battle_status_block_damage), Shield Reflect
// (battle_calc_return_damage), Martyr's Reckoning on normal attacks
// (battle_weapon_attack), and the end of statuses that need a shield or
// a spear (pc_checkallowskill). The damage formulas are in
// server/skill/crusader.go and server/battle; the statuses' stats are in
// server/status and server/statuschange.
//
// Parties do not exist yet:
//   - Sacrifice (Devotion) is a party skill. Its cast fails at
//     skill_castend_id_check, as rAthena's does for a player without a
//     party, so SC_DEVOTION never starts.
//   - Battle Chant's support effects reach party members other than
//     the caster. Without a party they reach no one, as in rAthena.
//
// Faith, Riding, Cavalier Mastery and Spear Mastery are passive: their
// effects are in server/status and server/battle.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDAutoGuard:       statusSkill{toggle: true},
		skill.IDSmite:           smite{},
		skill.IDShieldBoomerang: attackSkill{},
		skill.IDReflectShield:   shieldReflect{},
		skill.IDHolyCross:       holyCross{},
		skill.IDGrandCross:      grandCross{},
		skill.IDDevotion:        devotion{},
		skill.IDProvidence:      resistantSouls{},
		skill.IDDefender:        statusSkill{toggle: true},
		skill.IDSpearQuicken:    statusSkill{},
		skill.IDShrink:          statusSkill{toggle: true},
		skill.IDPressure:        gloriaDomini{},
		skill.IDSacrifice:       statusSkill{},
		skill.IDGospel:          battleChant{},
		skill.IDShieldChain:     attackSkill{},
	})
}

// Statuses, settings and map job IDs that the Crusader skills read.
const (
	scDarkcrow = 558 // SC_DARKCROW
	// mapidCrusader is MAPID_CRUSADER and mapidSecondMask
	// MAPID_SECONDMASK.
	mapidCrusader   = 0x201
	mapidSecondMask = 0xfff
	// devotionLevelDifference is devotion_level_difference of
	// conf/battle/skill.conf.
	devotionLevelDifference = 10
	// idMaxPainAtk is NPC_MAXPAIN_ATK, whose damage Shield Reflect does
	// not reflect.
	idMaxPainAtk = 703
)

// crusaderClass reports whether a player is a Crusader or a Paladin:
// (class_ & MAPID_SECONDMASK) == MAPID_CRUSADER.
func (z *Zone) crusaderClass(e *entity) bool {
	p := e.player
	if p == nil || z.Status == nil {
		return false
	}
	m, ok := z.Status.MapID(p.char.Job)
	return ok && m&mapidSecondMask == mapidCrusader
}

// smite is SkillSmite: a weapon hit that pushes the target, and may stun
// it.
type smite struct{ attackSkill }

func (smite) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCStun, 15+lv*5, lv, int64(d.Duration2.At(lv)))
}

// holyCross is SkillHolyCross: a holy weapon hit that may blind.
type holyCross struct{ attackSkill }

func (holyCross) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCBlind, 3*lv, lv, int64(d.Duration2.At(lv)))
}

// shieldReflect is SkillShieldReflect: StatusSkillImpl, which fails on a
// target under SC_DARKCROW.
type shieldReflect struct{}

func (shieldReflect) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.sc.Has(scDarkcrow) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	statusSkill{}.castendNoDamage(z, src, target, d, lv, flag)
}

// grandCross is SkillGrandCross: a cross of holy cells around the
// caster. Each cell hits 3 times in all (Duration1 / interval,
// gx_allhit off), then ends. In pre-renewal the cells hit the caster too
// (TargetSelf), for half the damage. Blind may strike demons and the
// undead.
type grandCross struct{}

func (grandCross) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	// Flag 1 keeps the ammunition.
	*flag |= 1
	z.unitSetting(src, d, lv, x, y)
}

func (grandCross) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if target.player == nil && isUndeadOrDemon(target) {
		z.scStart(src, target, sc.SCBlind, 100, lv, int64(d.Duration2.At(lv)))
	}
}

// counterFlag is SkillGrandCross::applyCounterAdditionalEffects: a hit on
// the caster counts as physical for the autospells it triggers.
func (grandCross) counterFlag(src, target *entity, flag int) int {
	if src == target {
		flag |= battle.FlagWeapon
		flag &^= battle.FlagMagic
	}
	return flag
}

// grandCrossLayout is the cross of Grand Cross, from
// skill_init_unit_layout: 29 cells, 4 cells out on each axis.
var grandCrossLayout = layoutOf(
	[]int{0, 0, -1, 0, 1, -2, -1, 0, 1, 2, -4, -3, -2, -1, 0, 1, 2, 3, 4, -2, -1, 0, 1, 2, -1, 0, 1, 0, 0},
	[]int{-4, -3, -2, -2, -2, -1, -1, -1, -1, -1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 2, 2, 2, 3, 4})

// grandCrossCast is Grand Cross's part of skill_castend_id for a player:
// while the cells hit, the caster cannot change its shield
// (SC_STRIPSHIELD with val2 1, which keeps the shield on) or move
// (SC_ANKLE). A Divest Shield that lasts longer stays, and then neither
// starts.
func (z *Zone) grandCrossCast(src *entity, d *skill.Def, lv int) {
	if src.player == nil {
		return
	}
	ms := int64(d.Duration1.At(lv))
	if e := src.sc.Get(sc.SCStripshield); e != nil && e.Timer() >= 0 && e.Timer() > z.now+ms {
		return
	}
	z.scStart4(src, src, sc.SCStripshield, 100, 0, 1, 0, 0, ms)
	z.scStart(src, src, sc.SCAnkle, 100, 0, ms)
}

// grandCrossOnPlace is CR_GRANDCROSS in skill_unit_onplace_timer: the
// cell counts the hit, then hits. A cell that used its hits ends.
func (z *Zone) grandCrossOnPlace(unit, t *entity) {
	u := unit.unit
	g := u.group
	u.val1--
	z.skillAttack(g.def.Type, g.src, unit, t, g.def, g.lv, 0)
	if u.val1 <= 0 && z.entities.has(unit) {
		z.deleteUnit(unit)
	}
}

// devotion is SkillSacrifice (CR_DEVOTION). It needs a party: the cast
// fails at skill_castend_id_check (PartyOnly), as rAthena's does for a
// player without one, so this never runs. Its own checks are ported; a
// cast that passed them would need the devotion links of a party, which
// do not exist, so it fails there.
type devotion struct{}

func (devotion) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, _ int, _ *int) {
	if src.player == nil {
		return
	}
	// Only players can be devoted.
	if target.player == nil {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	diff := z.unit(src).Level() - z.unit(target).Level()
	if diff < 0 {
		diff = -diff
	}
	if diff > devotionLevelDifference || target.sc.Has(sc.SCDevotion) || z.crusaderClass(target) ||
		target.sc.Has(scHellpower) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	// sd->devotion holds the party members a Crusader protects. Without
	// parties no slot can hold one.
	z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
}

// resistantSouls is SkillResistantSouls (CR_PROVIDENCE): the status on
// the target, which fails on another Crusader.
type resistantSouls struct{}

func (resistantSouls) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if src.player != nil && target.player != nil && z.crusaderClass(target) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// gloriaDomini is SkillGloriaDomini (PA_PRESSURE): renewal's magic hit,
// or pre-renewal's fixed damage that also takes SP.
type gloriaDomini struct{}

func (gloriaDomini) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	typ := d.Type
	if z.renewal() {
		typ = battle.TypeMagic
	}
	z.skillAttack(typ, src, src, target, d, lv, *flag)
}

// additionalEffect is SkillGloriaDomini::applyAdditionalEffects in
// pre-renewal: the target loses 15% + 5% per level of its SP
// (status_percent_damage). rAthena also sets BF_NORMAL and BF_WEAPON on
// its copy of the attack type, which does not reach the autospells, so
// that part changes nothing.
func (gloriaDomini) additionalEffect(z *Zone, src, target *entity, _ *skill.Def, lv int) {
	if z.renewal() {
		return
	}
	p := target.player
	if p == nil {
		// Other units have no SP to lose here.
		return
	}
	sp := p.sp * int64(15+5*lv) / 100
	if sp == 0 {
		sp = 1
	}
	scEnv{z}.Damage(z.unit(target), 0, sp, true, false)
}

// battleChant is SkillBattleChant (PA_GOSPEL): a 7x7 cross around the
// caster, who cannot move or use other skills while it lasts and pays HP
// and SP every 10 seconds. Casting it again ends it, at no cost. Every 10
// seconds each unit on it may get a random effect.
type battleChant struct{}

func (battleChant) castendPos(z *Zone, src *entity, _, _ int, d *skill.Def, lv int, flag *int) {
	e := src.sc.Get(sc.SCGospel)
	if e != nil && e.Val4 == sc.BCTSelf {
		z.scEnd(src, sc.SCGospel)
		*flag |= skillNoConsumeReq
		return
	}
	g := z.unitSetting(src, d, lv, src.x, src.y)
	if g == nil {
		return
	}
	if e != nil {
		// It was someone else's Battle Chant.
		z.scEnd(src, sc.SCGospel)
	}
	z.scStart4(src, src, sc.SCGospel, 100, lv, 0, int(g.id), sc.BCTSelf, int64(d.Duration1.At(lv)))
}

// gospelOnPlace is UNT_GOSPEL in skill_unit_onplace_timer. A unit other
// than the caster gets an effect at 50% + 5% per level:
//   - A party member gets one of 13 support effects. Parties do not
//     exist yet, so no one does, as in rAthena without a party.
//   - An enemy gets one of 10 offensive effects: damage, ailments for 30
//     minutes, level 10 Provoke, or 20 seconds of lower DEF, ATK, FLEE,
//     speed and ASPD.
func (z *Zone) gospelOnPlace(unit, t *entity) {
	g := unit.unit.group
	ss := g.src
	if z.rand.IntN(100) >= 50+g.lv*5 || ss == t {
		return
	}
	if checkTarget(ss, t, bctParty) {
		z.gospelSupport(g, t)
		return
	}
	if !checkTarget(ss, t, bctEnemy) {
		return
	}
	switch i := z.rand.IntN(10); i {
	case 0, 1:
		// 3000 to 7999 damage less DEF, or 1500 to 5499 unreduced.
		z.skillAttack(battle.TypeMisc, ss, unit, t, g.def, g.lv, i)
	case 2:
		z.scStart(ss, t, sc.SCCurse, 100, 1, 1800000)
	case 3:
		z.scStart(ss, t, sc.SCBlind, 100, 1, 1800000)
	case 4:
		z.scStart4(ss, t, sc.SCPoison, 100, 1, int(z.unit(ss).Ref().ID), 0, 0, 1800000)
	case 5:
		ok := z.scStart(ss, t, sc.SCProvoke, 100, 10, sc.InfiniteTick)
		if z.Skills != nil {
			z.skillEffect(t, t, z.Skills.Get(skill.IDProvoke), 10, 10, ok)
		}
	case 6:
		z.scStart(ss, t, sc.SCIncdefrate, 100, -100, 20000)
	case 7:
		z.scStart(ss, t, sc.SCIncatkrate, 100, -100, 20000)
	case 8:
		z.scStart(ss, t, sc.SCIncfleerate, 100, -100, 20000)
	case 9:
		z.scStart4(ss, t, sc.SCGospel, 100, 1, 0, 0, sc.BCTEnemy, 20000)
	}
}

// gospelSupport is the support effects of UNT_GOSPEL on a party member:
// one of 13 at random. Only party members other than the caster get
// them, and parties do not exist yet, so nothing calls it.
func (z *Zone) gospelSupport(g *unitGroup, t *entity) {
	ss := g.src
	ms := int64(g.def.Duration2.At(g.lv))
	switch z.rand.IntN(13) {
	case 0:
		// Heal 1000 to 9999 HP.
		heal := int64(z.rand.IntN(9000) + 1000)
		if z.Skills != nil {
			z.skillEffect(ss, t, z.Skills.Get(skill.IDHeal), int(heal), heal, true)
		}
		z.healHP(t, heal)
	case 1:
		// End every debuff.
		if en := z.statusEngine(); en != nil {
			en.ClearBuffs(z.unit(t), sc.RemoveDebuffs|sc.RemoveOnRefresh)
		}
	case 2:
		z.scStart(ss, t, sc.SCScresist, 100, 100, ms)
	case 3:
		z.scStart(ss, t, sc.SCIncmhprate, 100, 100, ms)
	case 4:
		z.scStart(ss, t, sc.SCIncmsprate, 100, 100, ms)
	case 5:
		z.scStart(ss, t, sc.SCIncallstatus, 100, 20, ms)
	case 6:
		z.scStart(ss, t, sc.SCBlessing, 100, 10, z.skillTime(skill.IDBlessing, 10))
	case 7:
		z.scStart(ss, t, sc.SCIncreaseagi, 100, 10, z.skillTime(skill.IDIncAgi, 10))
	case 8:
		z.scStart(ss, t, sc.SCAspersio, 100, 1, ms)
	case 9:
		z.scStart(ss, t, sc.SCBenedictio, 100, 1, ms)
	case 10:
		z.scStart(ss, t, sc.SCIncdefrate, 100, 25, 10000)
	case 11:
		z.scStart(ss, t, sc.SCIncatkrate, 100, 100, ms)
	case 12:
		z.scStart(ss, t, sc.SCInchit, 100, 50, ms)
		z.scStart(ss, t, sc.SCIncflee, 100, 50, ms)
	}
}

// skillTime is skill_get_time: a skill's Duration1 at a level.
func (z *Zone) skillTime(id, lv int) int64 {
	if z.Skills == nil {
		return 0
	}
	d := z.Skills.Get(id)
	if d == nil {
		return 0
	}
	return int64(d.Duration1.At(lv))
}

// gospelCaster reports whether a unit casts a Battle Chant: its
// SC_GOSPEL has BCT_SELF in val4. It cannot move (NoMoveCond) or use
// other skills.
func (e *entity) gospelCaster() bool {
	g := e.sc.Get(sc.SCGospel)
	return g != nil && g.Val4 == sc.BCTSelf
}

// autoGuard is SC_AUTOGUARD's part of battle_status_block_damage: a
// weapon hit misses by the status's chance, unless the skill ignores
// Guard. The guarding unit shows Guard and cannot walk for a moment.
// With Shrink, pre-renewal pushes the attacker 2 cells by a chance of 5%
// per Guard level; renewal stuns it by 50%. It reports whether the hit
// missed. rAthena marks the hit ATK_MISS; the zone reads it as blocked,
// which shows the same miss.
//
// A devoted player's Guard shows on its Crusader instead. Devotion needs
// a party and never starts, so that part is not ported.
func (z *Zone) autoGuard(src, t *entity, flag, skillID, lv int) bool {
	e := t.sc.Get(sc.SCAutoguard)
	if e == nil || flag&battle.FlagWeapon == 0 || z.rand.IntN(100) >= e.Val2 {
		return false
	}
	var d *skill.Def
	if z.Skills != nil {
		d = z.Skills.Get(skillID)
		if skillID != 0 && d != nil && d.Has("IgnoreAutoGuard") {
			return false
		}
	}
	// The walk delay by level.
	delay := int64(100)
	switch {
	case e.Val1 <= 5:
		delay = 300
	case e.Val1 <= 9:
		delay = 200
	}
	if z.Skills != nil {
		z.skillEffect(t, t, z.Skills.Get(skill.IDAutoGuard), e.Val1, int64(e.Val1), true)
	}
	z.setWalkDelay(t, delay)
	if src == nil || !t.sc.Has(sc.SCShrink) {
		return true
	}
	if z.renewal() {
		// skill_get_time2 of the attacking skill: a normal attack gives 0.
		ms := int64(0)
		if skillID != 0 && d != nil {
			ms = int64(d.Duration2.At(lv))
		}
		z.scStart(t, src, sc.SCStun, 50, lv, ms)
		return true
	}
	if z.rand.IntN(100) < 5*e.Val1 {
		n := 0
		if z.Skills != nil {
			if sd := z.Skills.Get(skill.IDShrink); sd != nil {
				n = sd.Knockback.At(1)
			}
		}
		z.skillBlown(t, src, n, -1, false)
	}
	return true
}

// reflectShield is the SC_REFLECTSHIELD part of battle_do_reflect and
// battle_calc_return_damage: a melee weapon or misc hit on a unit under
// Shield Reflect returns val2 percent of its damage to the attacker, at
// least 1 and at most the reflecting unit's max HP. Venom Bleed's val2
// on the attacker reduces it. rAthena deals it with the attack's motion,
// before the hit itself; the zone deals it at once, before the hit.
// HP drain bonuses on reflected damage are not ported.
func (z *Zone) reflectShield(src, target *entity, skillID int, res battle.Result) {
	if res.Damage <= 0 || src == target || target.status == nil || src.dead {
		return
	}
	e := target.sc.Get(sc.SCReflectshield)
	if e == nil || res.Flag&battle.FlagShort == 0 || res.Flag&(battle.FlagWeapon|battle.FlagMisc) == 0 {
		return
	}
	if skillImmune(target) || skillID == idCartTermination || skillID == idMaxPainAtk {
		return
	}
	rdamage := res.Damage * int64(e.Val2) / 100
	if rdamage > 0 {
		reduce := int64(0)
		if v := src.sc.Get(sc.SCVenombleed); v != nil && v.Val3 == 0 {
			reduce += int64(v.Val2)
		}
		rdamage -= rdamage * min(100, reduce) / 100
		rdamage = max(rdamage, 1)
	}
	if rdamage == 0 {
		return
	}
	rdamage = min(rdamage, int64(target.status.MaxHP))
	z.reflectedHit(target, src, rdamage)
}

// reflectedHit is battle_damage for damage that Shield Reflect returns:
// the attacker takes it as a hit from the reflecting unit, which may
// stop its cast or kill it. The reflecting player's autospells on
// normal attacks may trigger (skill_additional_effect with
// CR_REFLECTSHIELD, which runs no item procs).
func (z *Zone) reflectedHit(from, to *entity, damage int64) {
	if to.dead {
		return
	}
	z.logDamage(from, to, damage)
	to.hp -= damage
	to.hpChanged = true
	res := battle.Result{Kind: battle.Hit, Damage: damage, Hits: 1}
	z.hits = append(z.hits, hit{source: to, target: to, result: res, hpPermille: to.hpPermille()})
	z.damaged(to, damage, from.monster != nil)
	z.castCancel(to, true)
	if to.hp <= 0 {
		z.die(to, from)
		return
	}
	if from.player != nil && !from.dead {
		z.autoSpellAttack(from, to, battle.FlagWeapon|battle.FlagShort|battle.FlagNormal)
	}
}

// sacrificeAttack is SC_SACRIFICE's part of battle_weapon_attack: a
// normal attack becomes Martyr's Reckoning at the status's level, which
// counts down 5 attacks, and the attacker loses 9% of its max HP, which
// may kill it. It reports whether it took the attack.
func (z *Zone) sacrificeAttack(src, target *entity) bool {
	e := src.sc.Get(sc.SCSacrifice)
	if e == nil || z.Skills == nil || src.status == nil {
		return false
	}
	d := z.Skills.Get(skill.IDSacrifice)
	if d == nil {
		return false
	}
	lv := e.Val1
	e.Val2--
	if e.Val2 <= 0 {
		z.scEnd(src, sc.SCSacrifice)
	}
	// The damage comes first, as the HP loss can kill the attacker.
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, 0)
	scEnv{z}.Damage(z.unit(src), int64(src.status.MaxHP*9/100), 0, true, false)
	return true
}

// checkAllowSkill is pc_checkallowskill: after an equipment change, a
// status that needs a weapon type ends without one, as Spear Quicken
// without a spear, and a status that needs a shield ends without one,
// as Guard. Defending Aura needs a shield only on a Crusader or a
// Paladin. dancing_weaponswitch_fix is on, so SC_DANCING is checked
// too, against Encore's instruments and whips.
func (z *Zone) checkAllowSkill(p *Player) {
	e := p.entity
	if e.sc.Empty() || z.Statuses == nil {
		return
	}
	shield := z.Items != nil && p.inventory != nil && p.inventory.Worn(z.Items).Shield > 0
	for _, x := range e.sc.Sorted() {
		def := z.Statuses.Get(x.Type)
		if def == nil {
			continue
		}
		if def.Flags.Has(sc.FlagRequireWeapon) && z.Skills != nil {
			// skill_get_weapontype of the status's skill. A skill without
			// a weapon type fits no weapon.
			mask := 0
			if d := z.Skills.Get(def.DurationSkill); d != nil {
				mask = d.Requires.Weapon
			}
			if !checkWeaponType(e, mask) {
				z.scEnd(e, x.Type)
				continue
			}
		}
		if def.Flags.Has(sc.FlagRequireNoWeapon) && e.status != nil && e.status.Player != nil &&
			e.status.Player.Weapon != battle.WeaponFist {
			z.scEnd(e, x.Type)
			continue
		}
		if def.Flags.Has(sc.FlagRequireShield) {
			if x.Type == sc.SCDefender && !z.crusaderClass(e) {
				continue
			}
			if !shield {
				z.scEnd(e, x.Type)
			}
		}
	}
}
