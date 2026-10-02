package zone

import (
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The skills of the Sage and the Professor, as the SkillImpl classes of
// rAthena's src/map/skills/mage. Free Cast, Advanced Book and Dragonology
// are passives that status_calc_pc and battle read.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDCastCancel:      castCancel{},
		skill.IDMagicRod:        magicRod{},
		skill.IDSpellBreaker:    spellBreaker{},
		skill.IDAutoSpell:       hindsight{},
		skill.IDFlameLauncher:   endow{},
		skill.IDFrostWeapon:     endow{},
		skill.IDLightningLoader: endow{},
		skill.IDSeismicWeapon:   endow{},
		skill.IDVolcano:         elementField{},
		skill.IDDeluge:          elementField{},
		skill.IDViolentGale:     elementField{},
		skill.IDLandProtector:   unitSkillImpl{},
		skill.IDDispell:         dispell{},
		skill.IDElementWater:    elementalChange{},
		skill.IDElementGround:   elementalChange{},
		skill.IDElementFire:     elementalChange{},
		skill.IDElementWind:     elementalChange{},

		skill.IDHPConversion:  indulge{},
		skill.IDSoulChange:    soulExhale{},
		skill.IDSoulBurn:      soulSiphon{},
		skill.IDMindBreaker:   mindBreaker{},
		skill.IDMemorize:      statusSkill{},
		skill.IDFogWall:       blindingMist{},
		skill.IDSpiderWeb:     unitSkillImpl{},
		skill.IDDoubleCasting: statusSkill{},

		// Hocus-pocus picks a skill from abra_db, which the extracts do
		// not have, and lets the player cast it through the item skill
		// window. Create Elemental Converter needs the produce window.
		skill.IDAbracadabra: notPorted{},
		skill.IDCreateCon:   notPorted{},
	})
}

// Skills that the Sage's code reads.
const (
	idWaterBall  = 86 // WZ_WATERBALL
	idFirePillar = 80 // WZ_FIREPILLAR
	// untDummySkill is UNT_DUMMYSKILL: a unit the client does not show.
	untDummySkill = 0x86
)

// spSkillCost is skill_get_sp: the SP of a skill at a level in
// skill_db, without the caster's changes.
func spSkillCost(d *skill.Def, lv int) int64 { return int64(d.Requires.SP.At(lv)) }

// zap is status_zap: passive HP and SP damage, which stops no cast.
func (z *Zone) zap(e *entity, hp, sp int64) int64 {
	return scEnv{z}.Damage(z.unit(e), hp, sp, true, false)
}

// healSP is status_heal for SP.
func (z *Zone) healSP(e *entity, sp int64) {
	if sp > 0 {
		scEnv{z}.Heal(z.unit(e), 0, sp)
	}
}

// spOf is a unit's SP. Monsters have no SP of their own yet, so theirs is
// their max SP.
func (z *Zone) spOf(e *entity) int64 { return z.unit(e).SP() }

// takeSP is status_damage for SP: up to sp SP leaves the unit. It returns
// how much left, as status_damage does. Monsters keep theirs, since they
// have no SP pool yet.
func (z *Zone) takeSP(e *entity, sp int64) int64 {
	sp = min(sp, z.spOf(e))
	if sp <= 0 {
		return 0
	}
	z.zap(e, 0, sp)
	return sp
}

// percentSP is the SP part of status_percent_change: a positive rate is
// a percent of the current SP, a negative one of the max SP, and at
// least 1.
func (z *Zone) percentSP(e *entity, rate int) int64 {
	var sp int64
	switch {
	case rate > 99:
		sp = z.spOf(e)
	case rate > 0:
		sp = z.spOf(e) * int64(rate) / 100
	case rate < -99:
		sp = int64(z.unit(e).Status().MaxSP)
	case rate < 0:
		sp = int64(z.unit(e).Status().MaxSP) * int64(-rate) / 100
	}
	if rate != 0 && sp == 0 {
		sp = 1
	}
	return sp
}

// setSP is status_set_sp: the unit's SP becomes a value, at most its max.
func (z *Zone) setSP(e *entity, sp int64) {
	p := e.player
	if p == nil || e.dead {
		return
	}
	sp = min(max(sp, 0), int64(e.status.MaxSP))
	if sp > p.sp {
		z.healSP(e, sp-p.sp)
	} else if sp < p.sp {
		z.zap(e, 0, p.sp-sp)
	}
}

// statusCharge is status_charge: the unit pays HP and SP, and fails
// without enough. The last HP cannot be paid.
func (z *Zone) statusCharge(e *entity, hp, sp int64) bool {
	if hp > 0 && hp >= e.hp {
		return false
	}
	if sp > z.spOf(e) {
		return false
	}
	z.zap(e, hp, sp)
	return true
}

// castCancel is SkillCastCancel: the caster's own cast stops, and Cast
// Cancel takes part of the stopped skill's SP: 90% at level 1, 20% less
// per level.
type castCancel struct{}

// castBegin is SA_CASTCANCEL's check of
// skill_check_condition_castbegin: there must be a cast to stop.
func (castCancel) castBegin(_ *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	if src.cast == nil {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (castCancel) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	p := src.player
	if p == nil || p.castOld == nil {
		return
	}
	old := p.castOld
	p.castOld = nil
	// unit_skillcastcancel with type 1.
	src.cast = old
	z.castCancel(src, false)
	sp := spSkillCost(old.def, old.lv) * int64(90-(lv-1)*20) / 100
	if sp > 0 {
		z.zap(src, 0, sp)
	}
}

// castCancelBegin is unit_skilluse_id2 for Cast Cancel during a cast. The
// cast it stops waits for Cast Cancel's effect. When Cast Cancel fails,
// that cast goes on, as rAthena's timer of it stays.
func (z *Zone) castCancelBegin(src *entity, old, cc *casting, castMs int) bool {
	p := src.player
	if p != nil {
		p.castOld = old
	}
	src.cast = cc
	ok := z.castBegin(src, castMs)
	if p != nil && p.castOld != nil && src.cast == nil {
		src.cast, p.castOld = p.castOld, nil
	}
	return ok
}

// magicRod is SkillMagicRod.
type magicRod struct{}

func (magicRod) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if z.renewal() {
		z.skillEffect(src, src, d, lv, int64(lv), true)
	}
	z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
}

// magicRodAbsorb is Magic Rod's part of skill_attack: a magic skill cast
// at a unit under Magic Rod misses, and the unit gains a part of the
// skill's SP.
func (z *Zone) magicRodAbsorb(target *entity, d *skill.Def, lv int, res *battle.Result) {
	e := target.sc.Get(sc.SCMagicrod)
	sp := spSkillCost(d, lv)
	if !z.renewal() {
		if rod := z.Skills.Get(skill.IDMagicRod); rod != nil {
			z.skillEffect(target, target, rod, e.Val1, int64(e.Val1), true)
		}
	}
	res.Damage, res.Left = 0, 0
	res.Kind = battle.Miss
	sp = sp * int64(e.Val2) / 100
	if d.ID == idWaterBall && lv > 1 {
		// The SP of a single water ball.
		sp /= int64((lv | 1) * (lv | 1))
	}
	z.healSP(target, sp)
}

// spellBreaker is SkillSpellBreaker.
type spellBreaker struct{}

func (spellBreaker) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if target.player != nil && target.sc.Has(sc.SCMagicrod) {
		// Magic Rod takes 20% of the caster's max SP instead.
		sp := z.takeSP(src, z.percentSP(src, -20))
		z.healSP(target, sp)
		return
	}
	c := target.cast
	if c == nil {
		// Nothing to cancel.
		return
	}
	st := target.status
	if st == nil {
		return
	}
	var hp int64
	if st.Mode&modeStatusImmune != 0 {
		// 10% chance against status immune units.
		if z.rand.IntN(100) < 90 {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return
		}
		if !z.renewal() && lv >= 5 && target.player == nil {
			hp = int64(st.MaxHP / 50)
		}
	} else if lv >= 5 && target.player == nil {
		// Level 5 takes 2% of max HP, from players only on PvP maps,
		// which do not exist yet.
		hp = int64(st.MaxHP / 50)
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.castCancel(target, false)
	sp := spSkillCost(c.def, c.lv)
	z.zap(target, 0, sp)
	// The caster gets back a part of the SP.
	z.healSP(src, sp*int64(25*(lv-1))/100)
	// Damage that would kill is not dealt.
	if hp > 0 && hp < target.hp {
		z.zap(target, hp, 0)
		z.hits = append(z.hits, hit{source: src, target: target, result: battle.Result{Kind: battle.Hit, Damage: hp, Hits: 1},
			hpPermille: target.hpPermille()})
		scEnv{z}.Heal(z.unit(src), hp/2, 0)
	}
}

// hindsight is SkillHindsight. A player's Hindsight opens a window to
// pick the spell (clif_autospell), which the client does not have, so it
// fails before it starts. A monster's picks the spell by its level.
type hindsight struct{ notPorted }

func (hindsight) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if src.player != nil {
		// clif_autospell: the player picks the spell in the window.
		return
	}
	spell, maxLv := 0, 1
	switch {
	case lv >= 10:
		spell, maxLv = skill.IDFrostDiver, lv-9
	case lv >= 8:
		spell, maxLv = skill.IDFireBall, lv-7
	case lv >= 5:
		spell, maxLv = skill.IDSoulStrike, lv-4
	case lv >= 2:
		bolts := [3]int{skill.IDColdBolt, skill.IDFireBolt, skill.IDLightningBolt}
		spell, maxLv = bolts[z.rand.IntN(3)], lv-1
	case lv > 0:
		spell, maxLv = skill.IDNapalmBeat, 3
	}
	if spell > 0 {
		z.scStart4(src, src, sc.SCAutospell, 100, lv, spell, maxLv, 0, int64(d.Duration1.At(lv)))
	}
}

// autoSpellStatus is SC_AUTOSPELL's part of battle_weapon_attack: a
// normal attack may cast the status's spell on the target, at a lower
// level half the time, for two thirds of its SP. The Sage Spirit's full
// chance needs Soul Linker skills, which are not ported.
func (z *Zone) autoSpellStatus(src, target *entity) {
	e := src.sc.Get(sc.SCAutospell)
	if e == nil || z.Skills == nil || z.rand.IntN(100) >= e.Val4 {
		return
	}
	d := z.Skills.Get(e.Val2)
	if d == nil {
		return
	}
	lv := e.Val3
	i := z.rand.IntN(100)
	if lv > 1 {
		if i >= 50 {
			lv /= 2
		} else if i >= 15 {
			lv--
		}
	}
	sp := spSkillCost(d, lv) * 2 / 3
	if src.player != nil && !z.statusCharge(src, 0, sp) {
		return
	}
	switch d.CastType() {
	case skill.CastGround:
		z.castendPos(src, target.x, target.y, d, lv, 0)
	case skill.CastNoDamage:
		z.castendNoDamage(src, target, d, lv, 0)
	default:
		z.castendDamage(src, target, d, lv, 0)
	}
	z.autoSpellDelay(src, d, lv)
}

// endow is SkillEndowBlaze, SkillEndowTsunami, SkillEndowTornado and
// SkillEndowQuake: the element on the target's weapon. A player with no
// weapon cannot take it. In pre-renewal it may fail, and a failure takes
// the target's weapon off.
type endow struct{}

func (endow) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if p := target.player; p != nil && target.status != nil && target.status.Player != nil &&
		target.status.Player.Weapon == battle.WeaponFist {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		z.skillEffect(src, target, d, lv, int64(lv), false)
		return
	}
	if z.renewal() {
		ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
		z.skillEffect(src, target, d, lv, int64(lv), ok)
		return
	}
	// 100% at levels 4 and 5, which last longer at 5.
	ok := z.scStart(src, target, d.Status, 60+lv*10, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
	if ok {
		return
	}
	if p := target.player; p != nil && z.Items != nil && p.inventory != nil {
		// The weapon comes off instead of breaking.
		if slot := p.inventory.Worn(z.Items).Index[item.EqiHandR]; slot >= 0 {
			if def, found := z.Items.Lookup(p.inventory.Slots[slot].ID); found && def.IsWeapon() {
				z.unequipItem(p, slot)
			}
		}
	}
	z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
}

// elementField is SkillVolcano, SkillDeluge and SkillWhirlwind: a field
// that gives a status to the units in it. A field cast while the caster's
// last one still lasts keeps its gemstone and takes over the time left.
type elementField struct{}

func (elementField) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	if g := z.elementField(src); g != nil && isElementField(g.def.ID) {
		if g.limit-(z.now-g.start) > 0 {
			z.unitSetting(src, d, lv, x, y)
			*flag |= skillNoConsumeReq
			return
		}
		// Disable it.
		g.limit = 0
	}
	z.unitSetting(src, d, lv, x, y)
}

// isElementField reports whether a skill is Volcano, Deluge or Violent
// Gale.
func isElementField(id int) bool {
	return id == skill.IDVolcano || id == skill.IDDeluge || id == skill.IDViolentGale
}

// clearsAsField reports whether skill_locate_element_field and
// skill_clear_group with flag 1 count a skill's group: the element
// fields and Magnetic Earth.
func clearsAsField(id int) bool { return isElementField(id) || id == skill.IDLandProtector }

// elementField is skill_locate_element_field: the caster's first element
// field or Magnetic Earth.
func (z *Zone) elementField(src *entity) *unitGroup {
	for _, g := range src.groups {
		if clearsAsField(g.def.ID) {
			return g
		}
	}
	return nil
}

// clearGroup is skill_clear_group with flag 1: the caster's element
// fields and Magnetic Earth end.
func (z *Zone) clearGroup(src *entity) {
	for _, g := range slices.Clone(src.groups) {
		if clearsAsField(g.def.ID) {
			z.deleteGroup(g)
		}
	}
}

// sortedGroups are the zone's unit groups in the order they were placed.
func (z *Zone) sortedGroups() []*unitGroup {
	gs := make([]*unitGroup, 0, len(z.groups))
	for _, g := range z.groups {
		gs = append(gs, g)
	}
	slices.SortFunc(gs, func(a, b *unitGroup) int { return int(a.id - b.id) })
	return gs
}

// landProtected is map_getcell with CELL_CHKLANDPROTECTOR: a cell within
// a Magnetic Earth unit's range.
func (z *Zone) landProtected(x, y int) bool {
	for _, g := range z.groups {
		if g.def.ID != skill.IDLandProtector {
			continue
		}
		for _, u := range g.units {
			if abs(u.x-x) <= u.unit.rng && abs(u.y-y) <= u.unit.rng {
				return true
			}
		}
	}
	return false
}

// unitBlocked is the start of skill_unit_onplace: a unit's effect does
// not reach a target that stands on Magnetic Earth when the unit is
// magic, nor a hidden one unless the skill targets hidden units.
func (z *Zone) unitBlocked(g *unitGroup, t *entity) bool {
	if g.def.Type == battle.TypeMagic && g.def.ID != skill.IDLandProtector && z.landProtected(t.x, t.y) {
		return true
	}
	return !t.sc.Empty() && t.sc.Option&sc.OptionHide != 0 && !g.def.Has("TargetHidden")
}

// fieldStatuses are the statuses of the fields that last while a unit
// stands in them, by the skill of the field.
var fieldStatuses = []struct{ skill, status int }{
	{skill.IDVolcano, sc.SCVolcano},
	{skill.IDDeluge, sc.SCDeluge},
	{skill.IDViolentGale, sc.SCViolentgale},
	{skill.IDFogWall, sc.SCFogwall},
}

// fieldStatusRefresh is skill_unit_onplace and skill_unit_onleft for
// Volcano, Deluge, Violent Gale and Blinding Mist: a unit in one of their
// fields gets its status, and loses it when it leaves.
func (z *Zone) fieldStatusRefresh(t *entity) {
	en := z.statusEngine()
	if en == nil || t.dead {
		return
	}
	var groups []*unitGroup
	for _, f := range fieldStatuses {
		var on *unitGroup
		if len(z.groups) > 0 && groups == nil {
			groups = z.sortedGroups()
		}
		for _, g := range groups {
			if g.def.ID == f.skill && groupCovers(g, t) && checkTarget(g.src, t, g.targetFlag) && !z.unitBlocked(g, t) {
				on = g
				break
			}
		}
		has := t.sc.Has(f.status)
		switch {
		case on == nil && has:
			z.fieldLeft(t, f.status)
		case on != nil && !has:
			z.fieldPlace(on, t, f.status)
		}
	}
}

// fieldPlace is skill_unit_onplace for a field: its status for as long as
// the group lasts. Blinding Mist also blinds the enemies in it.
func (z *Zone) fieldPlace(g *unitGroup, t *entity, typ int) {
	en := z.statusEngine()
	su := z.unit(g.src)
	s := sc.Start{Type: typ, Rate: 10000, Val1: g.lv, Duration: g.limit, Reason: sc.ReasonSkill, Source: su.Ref()}
	if typ != sc.SCFogwall {
		en.Start(su, z.unit(t), s)
		return
	}
	s.Val2, s.Val3, s.Val4 = g.val2, int(g.val3), int(g.id)
	en.Start(su, z.unit(t), s)
	if checkTarget(g.src, t, bctEnemy) {
		z.skillAdditionalEffect(g.src, t, g.def, g.lv, battle.Result{Kind: battle.Hit, Flag: battle.FlagMisc}, true)
	}
}

// fieldLeft is skill_unit_onleft for a field: the status ends. Blinding
// Mist's blindness ends at once for players, and 30 seconds later for
// other units.
func (z *Zone) fieldLeft(t *entity, typ int) {
	z.scEnd(t, typ)
	if typ != sc.SCFogwall || !t.sc.Has(sc.SCBlind) {
		return
	}
	if t.player != nil {
		z.scEnd(t, sc.SCBlind)
		return
	}
	if en := z.statusEngine(); en != nil {
		en.Retime(z.unit(t), sc.SCBlind, 30000)
	}
}

// indulge is SkillIndulge: 10% of max HP turns into SP for the target.
type indulge struct{}

// castBegin is PF_HPCONVERSION's check of
// skill_check_condition_castbegin: it does nothing at full SP, and says
// nothing.
func (indulge) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	if p := src.player; p != nil && src.status != nil && p.sp == int64(src.status.MaxSP) {
		return skillFailSilent
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (indulge) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	st := src.status
	if st == nil {
		st = &npcStatus
	}
	hp := int64(st.MaxHP / 10)
	sp := hp * int64(lv)
	if !z.statusCharge(src, hp, 0) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.healSP(target, sp)
}

// soulExhale is SkillSoulExhale: the caster and a player swap SP, or the
// caster takes 3% of its max SP from a monster, once per monster.
type soulExhale struct{}

func (soulExhale) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	st := src.status
	if st == nil {
		return
	}
	if m := target.monster; m != nil {
		if m.soulChanged {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return
		}
		m.soulChanged = true
		z.healSP(src, int64(st.MaxSP)*3/100)
		z.skillEffect(src, target, d, lv, int64(lv), true)
		return
	}
	sp1, sp2 := z.spOf(src), z.spOf(target)
	if z.renewal() {
		sp1 /= 2
		sp2 /= 2
		if target.sc.Has(sc.SCExtremityfist) {
			sp1 = z.spOf(target)
		}
	}
	if target.sc.Has(sc.SCNorecoverState) {
		sp1 = z.spOf(target)
	}
	z.setSP(src, sp2)
	z.setSP(target, sp1)
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// soulSiphon is SkillSoulSiphon: the target, or the caster on a failed
// roll, loses all its SP. Level 5 first deals magic damage of twice the
// SP.
type soulSiphon struct{}

func (soulSiphon) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	rate := 70
	if lv < 5 {
		rate = 30 + lv*10
	}
	if z.rand.IntN(100) >= rate {
		target = src
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if lv == 5 {
		z.skillAttack(battle.TypeMagic, src, src, target, d, lv, *flag)
	}
	z.takeSP(target, z.percentSP(target, 100))
}

// mindBreaker is SkillMindBreaker: a status that raises MATK and lowers
// MDEF. A target that already has it, a status immune target and the
// undead keep the caster's requirements.
type mindBreaker struct{}

func (mindBreaker) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	st := target.status
	if st == nil {
		return
	}
	if st.Mode&modeStatusImmune != 0 || battle.IsUndead(st.Race, st.DefEle) || target.sc.Has(d.Status) {
		*flag |= skillNoConsumeReq
		return
	}
	if !z.scStart(src, target, d.Status, 55+5*lv, lv, int64(d.Duration1.At(lv))) {
		z.skillEffect(src, target, d, lv, int64(lv), false)
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		*flag |= skillNoConsumeReq
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.castCancel(target, false)
	if target.monster != nil && !src.dead {
		// mob_target: the monster turns on the caster.
		target.target, target.repeat = src, true
	}
}

// dispell is SkillDispell: the target's statuses end, but those that
// Dispell cannot end.
type dispell struct{}

// dispellSongs are the songs that dispel_song lets Dispell end once their
// target left the song's area. dispel_song is off, so they stay.
var dispellSongs = []int{173, 174, sc.SCPoembragi, 176, 178, 179, 180, 181}

// dispelSong is dispel_song of conf/battle/skill.conf.
const dispelSong = false

func (dispell) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if splash := d.SplashFor(lv); *flag&1 == 0 && splash >= 1 {
		// Every unit in the splash.
		for _, t := range z.findEntities(&query{area: areaRadius, x: target.x, y: target.y, radius: splash,
			kinds: queryPlayers | queryMonsters}) {
			f := *flag | 1
			dispell{}.castendNoDamage(z, src, t, d, lv, &f)
		}
		return
	}
	if src.player != nil && target.player != nil {
		// Outside PvP, which does not exist yet, it only works on party
		// members, which do not exist either.
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if z.rand.IntN(100) >= 50+10*lv {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	if target.sc.Has(scHermode) {
		// status_isimmune.
		return
	}
	if p := target.player; p != nil {
		z.endTimedBonuses(p, "dispell")
	} else if target.monster != nil {
		// mob_unlocktarget.
		target.target = nil
	}
	en := z.statusEngine()
	if en == nil || target.sc.Empty() {
		return
	}
	for _, e := range target.sc.Sorted() {
		typ := e.Type
		def := en.DB.Get(typ)
		if def == nil || def.Flags.Has(sc.FlagNoDispell) || target.sc.Get(typ) != e {
			continue
		}
		if slices.Contains(dispellSongs, typ) && (!dispelSong || e.Val4 == 0) {
			continue
		}
		if typ == sc.SCAssumptio && target.monster != nil {
			continue
		}
		if typ == sc.SCBerserk || typ == sc.SCSaturdaynightfever {
			// A dispelled Berserk takes no HP when it ends.
			e.Val2 = 0
		}
		en.End(z.unit(target), typ, sc.ReasonDispell)
	}
}

// scHermode is SC_HERMODE, which makes a unit immune to Dispell.

// elementalChange is SkillElementalChangeWater and its kin: a monster's
// element changes. Players can only change monsters, and not status
// immune ones.
type elementalChange struct{}

func (elementalChange) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if src.player != nil && (target.monster == nil || target.status == nil || target.status.Mode&modeStatusImmune != 0) {
		return
	}
	ok := z.scStart4(src, target, d.Status, 100, lv, d.Element.At(lv), 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// blindingMist is SkillBlindingMist: a field that blinds the enemies in
// it, unless Deluge washes the mist off them.
type blindingMist struct{ unitSkillImpl }

func (blindingMist) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if src != target && !target.sc.Has(sc.SCDeluge) {
		z.scStart(src, target, sc.SCBlind, 100, lv, int64(d.Duration2.At(lv)))
	}
}

// spiderWebOnPlace is skill_unit_onplace for Fiber Lock: the web holds
// the first enemy on its cell. A unit in more webs is held longer, and a
// unit in three webs cannot be held by a fourth. The web lasts as long as
// it holds.
func (z *Zone) spiderWebOnPlace(t *entity) {
	en := z.statusEngine()
	if en == nil || t.dead || len(z.groups) == 0 {
		return
	}
	for _, g := range z.sortedGroups() {
		if g.def.ID != skill.IDSpiderWeb || g.target == t || g.deleted || !groupCovers(g, t) ||
			!checkTarget(g.src, t, g.targetFlag) || z.unitBlocked(g, t) {
			continue
		}
		// The time in PvM: 8 seconds, then 16, then 8.
		sec := int64(g.def.Duration2.At(g.lv))
		su := z.unit(g.src)
		gid := int(g.id)
		if e := t.sc.Get(sc.SCSpiderweb); e != nil {
			if e.Val2 != 0 && e.Val3 != 0 && e.Val4 != 0 {
				// Already held by three webs.
				g.limit = z.now - g.start
				continue
			}
			if e.Val1 < 2 {
				sec *= int64(e.Val1 + 1)
			}
			switch {
			case e.Val2 == 0:
				e.Val2 = gid
			case e.Val3 == 0:
				e.Val3 = gid
			case e.Val4 == 0:
				e.Val4 = gid
			}
			if left := e.Timer() - z.now; e.Timer() >= 0 {
				en.Start(su, z.unit(t), sc.Start{Type: sc.SCSpiderweb, Rate: 10000, Val1: e.Val1 + 1, Val2: e.Val2,
					Val3: e.Val3, Val4: e.Val4, Duration: max(left, sec), Flag: sc.NoRateDef, Reason: sc.ReasonSkill,
					Source: su.Ref()})
			}
		} else if en.Start(su, z.unit(t), sc.Start{Type: sc.SCSpiderweb, Rate: 10000, Val1: 1, Val2: gid,
			Duration: sec, Flag: sc.NoRateDef, Reason: sc.ReasonSkill, Source: su.Ref()}) {
			if e := t.sc.Get(sc.SCSpiderweb); e != nil && e.Timer() >= 0 {
				sec = e.Timer() - z.now
			}
			// The web pulls the unit onto its cell and stops it.
			if len(t.path) > 0 {
				t.path = t.path[:0]
				t.stopped = true
			}
		} else {
			// It could not hold the unit.
			sec = 3000
		}
		g.target = t
		g.limit = z.now - g.start + sec
	}
}

// spiderWebGroupDeleted is Fiber Lock's part of skill_delunitgroup: the
// web leaves the status of the unit it held, which ends with its last
// web.
func (z *Zone) spiderWebGroupDeleted(g *unitGroup) {
	t := g.target
	if t == nil || !z.entities.has(t) {
		return
	}
	e := t.sc.Get(sc.SCSpiderweb)
	if e == nil {
		return
	}
	gid := int(g.id)
	switch gid {
	case e.Val2:
		e.Val2 = 0
	case e.Val3:
		e.Val3 = 0
	case e.Val4:
		e.Val4 = 0
	default:
		// status_change_end removed it already.
		return
	}
	if e.Val2 == 0 && e.Val3 == 0 && e.Val4 == 0 {
		z.scEnd(t, sc.SCSpiderweb)
	}
}

// spiderWebBurn is battle_attr_fix's fire on a unit Fiber Lock holds:
// status_change_end of SC_SPIDERWEB, which removes the web with the least
// time left first, and ends the status with its last web. It reports
// whether the hit deals double.
func (z *Zone) spiderWebBurn(t *entity) bool {
	e := t.sc.Get(sc.SCSpiderweb)
	if e == nil {
		return false
	}
	left := func(g *unitGroup) int64 { return g.limit - (z.now - g.start) }
	var g *unitGroup
	pos := 0
	if e.Val2 != 0 {
		if g = z.groups[int64(e.Val2)]; g == nil {
			e.Val2 = 0
		}
	}
	for i, v := range []*int{&e.Val3, &e.Val4} {
		if *v == 0 {
			continue
		}
		g2 := z.groups[int64(*v)]
		if g2 == nil {
			*v = 0
			continue
		}
		if g == nil || left(g) > left(g2) {
			g, pos = g2, i+1
		}
	}
	switch pos {
	case 0:
		e.Val2 = 0
	case 1:
		e.Val3 = 0
	case 2:
		e.Val4 = 0
	}
	if g != nil {
		z.deleteGroup(g)
	}
	if t.dead || e.Val2 == 0 && e.Val3 == 0 && e.Val4 == 0 {
		z.scEnd(t, sc.SCSpiderweb)
	}
	return true
}

// doubleCast is Double Casting's part of skill_attack: a bolt may strike
// again after the attack motion.
func (z *Zone) doubleCast(src, dsrc, target *entity, d *skill.Def, lv, flag int) {
	if flag&2 != 0 {
		return
	}
	switch d.ID {
	case skill.IDColdBolt, skill.IDFireBolt, skill.IDLightningBolt:
	default:
		return
	}
	e := src.sc.Get(sc.SCDoublecast)
	if e == nil || z.rand.IntN(100) >= e.Val2 {
		return
	}
	amotion := 0
	if src == dsrc && src.status != nil {
		amotion = src.status.AMotion
	}
	z.addTimerSkill(src, z.now+int64(amotion), target, 0, 0, d, lv, battle.TypeMagic, flag|2)
}

// freeCast is the Free Cast part of unit_can_move and unit_attack: a
// player with Free Cast may walk and attack while it casts, but not
// during a guild skill.
func freeCast(e *entity) bool {
	p := e.player
	return p != nil && p.skillLevel(skill.IDFreeCast) > 0 && (e.cast == nil || !e.cast.def.Has("IsGuild"))
}

// freeCastChanged is the status_calc_bl of speed and ASPD that a cast's
// start and end run for a player with Free Cast.
func (z *Zone) freeCastChanged(e *entity) {
	if p := e.player; p != nil && p.skillLevel(skill.IDFreeCast) > 0 && z.Status != nil {
		z.statsChanged(p)
	}
}
