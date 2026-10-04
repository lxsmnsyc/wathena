package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Skill behaviors: what each first class skill does when its cast ends,
// as the SkillImpl classes of rAthena's src/map/skills. Damage formulas
// are in server/skill and server/battle; this file holds the rest: who
// is hit, statuses, heals, units and items. Each type names the rAthena
// class it ports.

// The methods a skill behavior may have, as SkillImpl's.
type (
	// castendDamager is castendDamageId.
	castendDamager interface {
		castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int)
	}
	// castendNoDamager is castendNoDamageId.
	castendNoDamager interface {
		castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int)
	}
	// castendPoser is castendPos2.
	castendPoser interface {
		castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int)
	}
	// castBeginChecker holds the skill's own checks of
	// skill_check_condition_castbegin. It returns
	// SKILL_FAILURE_UNSPECIFIED when the skill may start.
	castBeginChecker interface {
		castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure
	}
	// castEndChecker holds the skill's own checks of
	// skill_check_condition_castend, which may use items. It returns
	// SKILL_FAILURE_UNSPECIFIED when the skill may go on.
	castEndChecker interface {
		castEnd(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure
	}
	// additionalEffecter is applyAdditionalEffects: what a hit that
	// connected does besides damage.
	additionalEffecter interface {
		additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int)
	}
	// counterFlagger is applyCounterAdditionalEffects: it changes the
	// attack type of a hit that the target's autospells read.
	counterFlagger interface {
		counterFlag(src, target *entity, flag int) int
	}
)

// Statuses and options that skills read, which server/statuschange has
// no constants for.
const (
	scViolentgale   = 102
	scCamouflage    = 348
	scNewmoon       = 713
	scBitescar      = 633
	optionChasewalk = 0x4000
)

// skillImpls are the skills with Go behavior, by ID.
var skillImpls = map[int]any{
	skill.IDFirstAid:  firstAid{},
	skill.IDTrickDead: statusSkill{toggle: true},

	skill.IDBash:        bash{},
	skill.IDProvoke:     provoke{},
	skill.IDMagnumBreak: magnumBreak{},
	skill.IDEndure:      statusSkill{},
	skill.IDAutoBerserk: autoBerserk{},

	skill.IDSight:          sightSkill{},
	skill.IDNapalmBeat:     splashSkill{},
	skill.IDSafetyWall:     unitSkillImpl{},
	skill.IDSoulStrike:     attackSkill{},
	skill.IDColdBolt:       attackSkill{},
	skill.IDFireBolt:       attackSkill{},
	skill.IDLightningBolt:  attackSkill{},
	skill.IDFrostDiver:     frostDiver{},
	skill.IDStoneCurse:     stoneCurse{},
	skill.IDFireBall:       fireBall{},
	skill.IDFireWall:       unitSkillImpl{},
	skill.IDThunderStorm:   unitSkillImpl{},
	skill.IDEnergyCoat:     statusSkill{},
	skill.IDRuwach:         sightSkill{},
	skill.IDPneuma:         unitSkillImpl{},
	skill.IDTeleport:       teleport{},
	skill.IDHeal:           heal{},
	skill.IDIncAgi:         statusSkill{},
	skill.IDDecAgi:         decreaseAgi{},
	skill.IDHolyWater:      holyWater{},
	skill.IDCrucis:         crucis{},
	skill.IDAngelus:        partyStatus{},
	skill.IDBlessing:       statusSkill{},
	skill.IDCure:           cure{},
	skill.IDHolyLight:      attackSkill{},
	skill.IDConcentration:  concentration{},
	skill.IDDoubleStrafe:   attackSkill{},
	skill.IDArrowShower:    arrowShower{},
	skill.IDChargeArrow:    attackSkill{},
	skill.IDMammonite:      attackSkill{},
	skill.IDCartRevolution: cartRevolution{},
	skill.IDLoud:           loud{},
	skill.IDSteal:          steal{},
	skill.IDHiding:         statusSkill{toggle: true},
	skill.IDEnvenom:        envenom{},
	skill.IDDetoxify:       detoxify{},
	skill.IDSandAttack:     sandAttack{},
	skill.IDBackSliding:    backSlide{},
	skill.IDPickStone:      pickStone{},
	skill.IDThrowStone:     throwStone{},

	// Skills that need windows the client does not have yet: Warp
	// Portal's and Teleport's destination menus, Item Appraisal's list,
	// Arrow Crafting's list, carts and the buying store skill, which
	// player:open_buying_store replaces.
	skill.IDWarp:        notPorted{},
	skill.IDIdentify:    notPorted{},
	skill.IDVending:     vendingSkill{},
	skill.IDMakingArrow: notPorted{},
	skill.IDChangeCart:  notPorted{},
	idCallBaby:          notPorted{},
	idBuyingStore:       notPorted{},
	idCartDecorate:      notPorted{},
}

// registerSkillImpls adds skill behaviors from another file, once each.
// The files of the second and transcendent classes call it in init.
func registerSkillImpls(impls map[int]any) {
	for id, impl := range impls {
		if _, dup := skillImpls[id]; dup {
			panic("zone: skill behavior registered twice")
		}
		skillImpls[id] = impl
	}
}

// Skill IDs of first class trees that server/skill does not name.
const (
	idCallBaby     = 410  // WE_CALLBABY
	idBuyingStore  = 2535 // ALL_BUYING_STORE
	idCartDecorate = 2544 // MC_CARTDECORATE
)

// notPorted is a skill whose behavior waits for a window or a system
// that does not exist yet. It fails before it starts, so it costs
// nothing.
type notPorted struct{}

func (notPorted) castBegin(*Zone, *entity, *skill.Def, int) wathenav1.SkillFailure {
	return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
}

// scStart is sc_start: a status from a skill, with the rate in percent.
func (z *Zone) scStart(src, target *entity, typ, rate, val1 int, duration int64) bool {
	return z.scStart4(src, target, typ, rate, val1, 0, 0, 0, duration)
}

// scStart4 is sc_start4.
func (z *Zone) scStart4(src, target *entity, typ, rate, v1, v2, v3, v4 int, duration int64) bool {
	en := z.statusEngine()
	if en == nil || typ < 0 {
		return false
	}
	su := z.unit(src)
	return en.Start(su, z.unit(target), sc.Start{Type: typ, Rate: rate * 100, Val1: v1, Val2: v2, Val3: v3, Val4: v4,
		Duration: duration, Reason: sc.ReasonSkill, Source: su.Ref()})
}

// scEnd is status_change_end.
func (z *Zone) scEnd(target *entity, typ int) bool {
	en := z.statusEngine()
	if en == nil || target.sc.Empty() || !target.sc.Has(typ) {
		return false
	}
	return en.End(z.unit(target), typ, sc.ReasonRemoved)
}

// attackSkill is WeaponSkillImpl, and the single target magic and misc
// skills: one skill_attack of the skill's type.
type attackSkill struct{}

func (attackSkill) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(d.Type, src, src, target, d, lv, *flag)
}

// splashSkill is SkillImplRecursiveDamageSplash: the first target and
// the enemies around it, as Napalm Beat.
type splashSkill struct{}

func (splashSkill) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.recursiveSplash(src, target, d, lv, *flag)
}

// statusSkill is StatusSkillImpl: the skill's status on the target for
// Duration1. A toggle ends the status when it is on.
type statusSkill struct{ toggle bool }

func (s statusSkill) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if s.toggle && target.sc.Has(d.Status) {
		ok := z.scEnd(target, d.Status)
		z.skillEffect(src, target, d, lv, int64(lv), ok)
		return
	}
	switch d.ID {
	case skill.IDIncAgi, skill.IDBlessing:
		// A player under Change Undead takes 1 damage instead.
		if target.player != nil && target.sc.Has(sc.SCChangeundead) {
			z.skillEffect(src, target, d, lv, int64(lv), true)
			if target.hp > 1 {
				z.skillDamage(src, src, target, d, lv, battle.Result{Kind: battle.Hit, Damage: 1, Hits: 1,
					Flag: battle.FlagMisc}, 0)
			}
			return
		}
	}
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// firstAid is SkillFirstAid: 5 HP.
type firstAid struct{}

func (firstAid) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, 5, 5, true)
	z.healHP(target, 5)
}

// bash is SkillBash: a weapon hit that may stun with Fatal Blow.
type bash struct{ attackSkill }

func (bash) additionalEffect(z *Zone, src, target *entity, _ *skill.Def, lv int) {
	p := src.player
	if p == nil || lv <= 5 || p.skillLevel(skill.IDFatalBlow) <= 0 {
		return
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	// The rate is per 10,000: it calls status_change_start directly.
	rate := (lv - 5) * z.unit(src).Level() * 10
	d := z.Skills.Get(skill.IDBash)
	su := z.unit(src)
	en.Start(su, z.unit(target), sc.Start{Type: sc.SCStun, Rate: rate, Val1: lv, Duration: int64(d.Duration2.At(lv)),
		Reason: sc.ReasonSkill, Source: su.Ref()})
}

// provoke is SkillProvoke.
type provoke struct{}

func (provoke) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	st := target.status
	if st == nil || st.Mode&modeStatusImmune != 0 || battle.IsUndead(st.Race, st.DefEle) {
		return
	}
	rate := 70 + 3*lv + z.unit(src).Level() - z.unit(target).Level()
	if !z.scStart(src, target, sc.SCProvoke, rate, lv, int64(d.Duration1.At(lv))) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.castCancel(target, true)
	if target.monster != nil && !src.dead {
		// The provoked monster turns on the caster (provoke_flag and
		// mob_target).
		target.monster.provoke = src
		z.mobTarget(target, src)
	}
}

// magnumBreak is SkillMagnum: a hit on every enemy within its splash in
// line of sight, then the fire bonus.
type magnumBreak struct{}

func (magnumBreak) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, flag *int) {
	z.area = areaTemp{}
	for _, t := range z.areaTargets(src, src.x, src.y, d.SplashFor(lv), true) {
		f := *flag | 1 | sdAnimation
		if src.player != nil {
			// For players the damage depends on the distance.
			f |= chebyshev(src, t)
		}
		z.skillAttack(d.Type, src, src, t, d, lv, f)
	}
	z.skillEffect(src, src, d, lv, int64(lv), true)
	if z.skillRenewal(skill.IDMagnumBreak) {
		z.scStart4(src, src, sc.SCSubWeaponPro, 100, eleFire, 20, d.ID, 0, int64(d.Duration2.At(lv)))
	} else {
		z.scStart4(src, src, sc.SCWatkElement, 100, eleFire, 20, 0, 0, int64(d.Duration2.At(lv)))
	}
}

// autoBerserk is SkillAutoBerserk: a toggle without the Toggleable flag.
type autoBerserk struct{}

func (autoBerserk) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	var ok bool
	if target.sc.Has(sc.SCAutoberserk) {
		ok = z.scEnd(target, sc.SCAutoberserk)
	} else {
		ok = z.scStart(src, target, sc.SCAutoberserk, 100, lv, 60000)
	}
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// sightSkill is SkillSight and SkillRuwach: sc_start2 with the skill ID
// as val2, which the status reads for its splash.
type sightSkill struct{}

func (sightSkill) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 100, lv, d.ID, 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// frostDiver is SkillFrostDiver: a magic hit that may freeze.
type frostDiver struct{ attackSkill }

func (frostDiver) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if !z.scStart(src, target, sc.SCFreeze, min(lv*3+35, lv+60), lv, int64(d.Duration2.At(lv))) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
}

// stoneCurse is SkillStoneCurse.
type stoneCurse struct{}

func (stoneCurse) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	st := target.status
	if st == nil {
		return
	}
	if st.Mode&modeStatusImmune != 0 {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	su := z.unit(src)
	ok := en.Start(su, z.unit(target), sc.Start{Type: d.Status, Rate: (lv*4 + 20) * 100, Val1: lv, Val2: int(su.Ref().ID),
		Duration: int64(d.Duration2.At(lv)), Delay: int64(d.Duration1.At(lv)), Reason: sc.ReasonSkill, Source: su.Ref()})
	if ok || src.player == nil {
		z.skillEffect(src, target, d, lv, int64(lv), true)
		return
	}
	z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	if lv > 5 {
		// Levels 6 to 10 keep the gem when the roll fails.
		*flag |= skillNoConsumeReq
	}
}

// fireBall is SkillFireBall: a splash whose edge takes less, for players.
type fireBall struct{ splashSkill }

func (fireBall) splashDamage(z *Zone, src, target *entity, d *skill.Def, lv, flag int) {
	if src.player != nil && z.area.main != nil {
		flag |= chebyshev(z.area.main, target)
	}
	z.skillAttack(d.Type, src, src, target, d, lv, flag)
}

// unitSkillImpl is the ground skills that place a unit: Safety Wall,
// Fire Wall, Thunder Storm and Pneuma. flag 1 keeps ammunition.
type unitSkillImpl struct{}

func (unitSkillImpl) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	*flag |= 1
	z.unitSetting(src, d, lv, x, y)
}

// teleport is SkillTeleport. There is no warp menu: a player warps to a
// random cell, as with a Fly Wing, at both levels.
type teleport struct{}

func (teleport) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	x, y, ok := z.randomCell()
	if !ok {
		return
	}
	if p := target.player; p != nil {
		z.skillEffect(src, target, d, lv, int64(lv), true)
		p.warpTo = &scriptWarp{to: z, x: x, y: y}
		z.finishWarp(p)
		return
	}
	// unit_warp for a monster.
	z.moveTo(target, x, y)
}

// heal is SkillHeal.
type heal struct{}

func (heal) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(battle.TypeMagic, src, src, target, d, lv, *flag)
}

func (heal) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	srcSt, tSt := src.status, target.status
	if srcSt == nil || tSt == nil {
		return
	}
	amount := skill.CalcHeal(srcSt, tSt, d.ID, lv, true, z.rules().HealFormula, z.rand)
	z.skillEffect(src, target, d, lv, amount, true)
	z.healHP(target, amount)
}

// decreaseAgi is SkillDecreaseAgi.
type decreaseAgi struct{}

func (decreaseAgi) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	st := src.status
	if st == nil {
		st = &npcStatus
	}
	rate := 50 + lv*3 + (z.unit(src).Level()+st.Int)/5
	ok := z.scStart(src, target, sc.SCDecreaseagi, rate, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// holyWater is SkillHolyWater: an Empty Bottle into Holy Water.
type holyWater struct{}

func (holyWater) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	if p := src.player; p != nil && p.inventory.Count(itemEmptyBottle) == 0 {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (holyWater) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	if !z.takeItems(p, itemEmptyBottle, 1) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	z.addItem(p, itemHolyWater, 1, skillOrigin(d.ID))
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// crucis is SkillCrucis: Signum Crucis on every enemy in its splash.
type crucis struct{}

func (crucis) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 != 0 {
		rate := 25 + lv*4 + z.unit(src).Level() - z.unit(target).Level()
		z.scStart(src, target, sc.SCSignumcrucis, rate, lv, int64(d.Duration1.At(lv)))
		return
	}
	for _, t := range z.areaTargets(src, src.x, src.y, d.SplashFor(lv), false) {
		f := *flag | 1
		crucis{}.castendNoDamage(z, src, t, d, lv, &f)
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// partySameMap is party_foreachsamemap: the members of a player's party
// on its map, in slot order. A range above 0 keeps the members in the
// square of that many cells around the player; 0 is the whole map. A
// unit that is not a player in a party has none.
func (z *Zone) partySameMap(e *entity, rng int) []*entity {
	if e == nil || e.player == nil || z.Social == nil {
		return nil
	}
	id := e.player.partyID()
	if id == 0 {
		return nil
	}
	var out []*entity
	for _, m := range z.partyHere(id) {
		t := m.entity
		if rng > 0 && (abs(t.x-e.x) > rng || abs(t.y-e.y) > rng) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// partyAreaSub is party_foreachsamemap with skill_area_sub: the skill
// runs on each member in range that battle_check_target passes with the
// flag. It returns how many it ran on.
func (z *Zone) partyAreaSub(src *entity, rng, flag int, fn func(t *entity, flag int)) int {
	n := 0
	for _, t := range z.partySameMap(src, rng) {
		if checkTarget(src, t, flag) {
			fn(t, flag)
			n++
		}
	}
	return n
}

// partyCastNoDamage is party_foreachsamemap with skill_area_sub and
// skill_castend_nodamage_id, with flag|BCT_PARTY|1, over the members in
// the skill's splash.
func (z *Zone) partyCastNoDamage(src *entity, d *skill.Def, lv, flag int) int {
	return z.partyAreaSub(src, d.SplashFor(lv), flag|bctParty|1, func(t *entity, f int) {
		z.castendNoDamage(src, t, d, lv, f)
	})
}

// inParty reports whether a unit is a player in a party
// (sd->status.party_id).
func inParty(e *entity) bool { return e.player != nil && e.player.partyID() != 0 }

// partyStatus is the party buffs, as SkillAngelus. A player in a party
// casts it on the members in its splash on the map. The caster alone
// gets the status otherwise.
type partyStatus struct{}

func (partyStatus) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 == 0 && inParty(src) {
		z.partyCastNoDamage(src, d, lv, *flag)
		return
	}
	z.skillEffect(target, target, d, lv, int64(lv), true)
	z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
}

// loud is SkillCrazyUproar: StatusSkillImpl. In renewal a player in a
// party casts it on the members in its splash on the map.
type loud struct{ statusSkill }

func (l loud) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if z.skillRenewal(d.ID) && *flag&1 == 0 && inParty(src) {
		z.partyCastNoDamage(src, d, lv, *flag)
		return
	}
	l.statusSkill.castendNoDamage(z, src, target, d, lv, flag)
}

// cure is SkillCure.
type cure struct{}

func (cure) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if target.status != nil && target.status.Mode&modeStatusImmune != 0 && target.monster != nil {
		z.skillEffect(src, target, d, lv, int64(lv), false)
		return
	}
	for _, typ := range []int{sc.SCSilence, sc.SCBlind, sc.SCConfusion, scBitescar} {
		z.scEnd(target, typ)
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// concentration is SkillConcentration: the status, then one sweep that
// ends hiding around the caster.
type concentration struct{}

func (concentration) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
	z.revealArea(src, d.SplashFor(lv))
}

// revealArea is status_change_timer_sub for Sight and Improve
// Concentration: every character in the square around a unit, allies
// too, stops hiding.
func (z *Zone) revealArea(center *entity, r int) {
	for _, t := range z.findEntities(&query{area: areaRadius, x: center.x, y: center.y, radius: r,
		kinds: queryPlayers | queryMonsters}) {
		for _, typ := range []int{sc.SCHiding, sc.SCCloaking, sc.SCCloakingexceed, scCamouflage, scNewmoon} {
			z.scEnd(t, typ)
		}
	}
}

// arrowShower is SkillArrowShower: a ground splash.
type arrowShower struct{}

func (arrowShower) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, _ *int) {
	z.scEnd(src, scCamouflage)
	z.groundSplash(src, d, lv, x, y)
}

// cartRevolution is SkillCartRevolution: a splash with a preamble.
type cartRevolution struct{}

func (cartRevolution) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.recursiveSplash(src, target, d, lv, *flag|sdPreamble)
}

// steal is SkillSteal and pc_steal_item.
type steal struct{}

func (steal) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if z.stealItem(src, target, lv) {
		z.skillEffect(src, target, d, lv, int64(lv), true)
		return
	}
	z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
}

// stealItem is pc_steal_item.
func (z *Zone) stealItem(src, target *entity, lv int) bool {
	p, m := src.player, target.monster
	if p == nil || m == nil || m.stolen || target.status == nil || src.status == nil {
		return false
	}
	if b := target.body(); b != sc.Opt1None && b != sc.Opt1Burning {
		return false
	}
	reward := m.spawn.Reward
	if target.status.Mode&modeStatusImmune != 0 || reward == nil || m.spawn.owner != 0 {
		m.stolen = true
		return false
	}
	rate := (src.status.Dex-target.status.Dex)/2 + lv*6 + 4 + p.bonuses.AddStealRate
	if rate < 1 {
		return false
	}
	if z.skillRenewal(skill.IDSteal) && z.rand.IntN(100) >= rate {
		return false
	}
	for _, dr := range reward.Drops {
		if dr.Item == 0 || dr.StealProtected || z.Items == nil {
			continue
		}
		if _, ok := z.Items.Lookup(dr.Item); !ok {
			continue
		}
		chance := dr.Rate
		if !z.skillRenewal(skill.IDSteal) {
			chance = dr.Rate * rate / 100
		}
		if z.rand.IntN(10000) >= chance {
			continue
		}
		// The monster cannot be stolen from again, even when the item
		// does not fit.
		m.stolen = true
		return z.addItem(p, dr.Item, 1, origin{action: transfer.ActionLoot, from: monsterOwner(target)}) == 0
	}
	return false
}

// envenom is SkillEnvenom: a weapon hit that may poison.
type envenom struct{ attackSkill }

func (envenom) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if !z.scStart4(src, target, sc.SCPoison, 4*lv+10, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(lv))) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
}

// detoxify is SkillDetoxify.
type detoxify struct{}

func (detoxify) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.scEnd(target, sc.SCPoison)
	z.scEnd(target, sc.SCDpoison)
}

// sandAttack is SkillSandAttack: a weapon hit that may blind.
type sandAttack struct{ attackSkill }

func (sandAttack) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := 15
	if src.player != nil {
		rate = 20
	}
	z.scStart(src, target, sc.SCBlind, rate, lv, int64(d.Duration2.At(lv)))
}

// backSlide is SkillBackSlide: 5 cells back from the way the caster
// faces.
type backSlide struct{}

func (backSlide) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if target.endure < z.now {
		target.endure = z.now + 200
	}
	z.skillBlown(src, target, d.Knockback.At(lv), rDir(target), true)
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// pickStone is SkillFindStone: one Stone.
type pickStone struct{}

func (pickStone) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if p := src.player; p != nil {
		z.addItem(p, itemStone, 1, skillOrigin(d.ID))
	}
}

// throwStone is SkillStoneFling: a misc hit that may stun or blind.
type throwStone struct{ attackSkill }

func (throwStone) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if src.player != nil {
		if !z.scStart(src, target, sc.SCStun, 3, lv, int64(d.Duration1.At(lv))) {
			z.scStart(src, target, sc.SCBlind, 3, lv, int64(d.Duration2.At(lv)))
		}
		return
	}
	z.scStart(src, target, sc.SCStun, 5, lv, int64(d.Duration1.At(lv)))
}
