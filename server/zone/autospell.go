package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Autospells: skills that item bonuses cast (the "cast" bonus of
// proposal 0002a, rAthena's bAutoSpell, bAutoSpellWhenHit and
// bAutoSpellOnSkill). They cast at once, without a cast time, and cost
// no SP (sd->state.autocast).

// Masks of battle flags that an autospell's trigger must match in each
// part, as rAthena's BF_WEAPONMASK, BF_RANGEMASK and BF_SKILLMASK.
const (
	bfWeaponMask = battle.FlagWeapon | battle.FlagMagic | battle.FlagMisc
	bfRangeMask  = battle.FlagShort | battle.FlagLong
	bfSkillMask  = battle.FlagSkill | battle.FlagNormal
)

// autoSpellMatches checks an autospell's battle flags against a hit's.
func autoSpellMatches(want, got int) bool {
	m := want & got
	return m&bfWeaponMask != 0 && m&bfRangeMask != 0 && m&bfSkillMask != 0
}

// autoSpellBlocked is skill_isNotOk for an autocast: the skill's
// cooldown.
func (z *Zone) autoSpellBlocked(p *Player, id int) bool {
	until, ok := p.cooldowns[id]
	return ok && z.now < until
}

// autoCast casts an autospell's skill on a target or a cell at once, as
// the switch on skill_get_casttype that the autospell functions share.
// The SP is not paid; the HP and zeny are.
func (z *Zone) autoCast(src, target *entity, d *skill.Def, lv int) {
	p := src.player
	p.autocast = true
	z.consumeRequirement(src, d, lv, 1)
	switch d.CastType() {
	case skill.CastGround:
		z.castendPos(src, target.x, target.y, d, lv, 0)
	case skill.CastNoDamage:
		z.castendNoDamage(src, target, d, lv, 0)
	default:
		z.castendDamage(src, target, d, lv, 0)
	}
	p.autocast = false
}

// autoSpellDelay sets the act delay after an autospell, as the end of
// the autospell loops: the skill's after-cast delay, when it is later.
func (z *Zone) autoSpellDelay(src *entity, d *skill.Def, lv int) {
	delay := int64(d.Delay(z.caster(src), lv))
	if src.canAct < z.now+delay {
		src.canAct = z.now + delay
		if p := src.player; p != nil {
			p.pending = append(p.pending, cooldownEvent(0, delay))
		}
	}
}

// autoSpellLevel is the level an autospell casts at.
func (z *Zone) autoSpellLevel(a *status.AutoSpell) int {
	lv := max(a.Level, 1)
	if a.Flag&status.AutoSpellForceRandomLevel != 0 {
		lv = 1 + z.rand.IntN(lv)
	}
	return lv
}

// autoSpellAttack is the autospell part of skill_additional_effect: a
// hit that connected may cast the attacker's bAutoSpell skills, on the
// target or on the attacker.
func (z *Zone) autoSpellAttack(src, target *entity, flag int) {
	p := src.player
	if p == nil || z.Skills == nil || target.dead || len(p.bonuses.AutoSpell) == 0 {
		return
	}
	for i := range p.bonuses.AutoSpell {
		a := &p.bonuses.AutoSpell[i]
		if !autoSpellMatches(a.BattleFlag, flag) {
			continue
		}
		d := z.Skills.Get(a.Skill)
		if d == nil || z.autoSpellBlocked(p, a.Skill) {
			continue
		}
		lv := z.autoSpellLevel(a)
		rate := a.Rate
		if p.entity.status != nil && p.entity.status.Player != nil && p.entity.status.Player.ArrowAttack {
			rate /= 2
		}
		if z.rand.IntN(1000) >= rate {
			continue
		}
		tbl := src
		if a.Flag&status.AutoSpellForceTarget != 0 {
			tbl = target
		}
		if d.CastType() == skill.CastGround && !z.posMaxCount(src, d, lv, tbl.x, tbl.y, false) {
			continue
		}
		if !z.renewal() {
			z.toggleMagicPower(src, d)
		}
		z.autoCast(src, tbl, d, lv)
		z.autoSpellDelay(src, d, lv)
		if src.dead || target.dead {
			return
		}
	}
}

// autoSpellWhenHit is the autospell part of
// skill_counter_additional_effect: a player hit by an attack may cast its
// bAutoSpellWhenHit skills, on the attacker or on itself. Ranged weapon
// hits trigger them half as often.
func (z *Zone) autoSpellWhenHit(src, target *entity, flag int, skillNK battle.NK) {
	p := target.player
	if p == nil || z.Skills == nil || target.dead || len(p.bonuses.AutoSpellWhenHit) == 0 || skillNK.Has(battle.NKNoDamage) {
		return
	}
	for i := range p.bonuses.AutoSpellWhenHit {
		a := &p.bonuses.AutoSpellWhenHit[i]
		if !autoSpellMatches(a.BattleFlag, flag) {
			continue
		}
		d := z.Skills.Get(a.Skill)
		if d == nil {
			continue
		}
		lv := z.autoSpellLevel(a)
		rate := a.Rate
		if flag&(battle.FlagWeapon|battle.FlagLong) == battle.FlagWeapon|battle.FlagLong {
			rate /= 2
		}
		if z.autoSpellBlocked(p, a.Skill) || z.rand.IntN(1000) >= rate {
			continue
		}
		tbl := target
		if a.Flag&status.AutoSpellForceTarget != 0 {
			tbl = src
		}
		if d.CastType() == skill.CastGround && !z.posMaxCount(target, d, lv, tbl.x, tbl.y, false) {
			continue
		}
		z.autoCast(target, tbl, d, lv)
		z.autoSpellDelay(target, d, lv)
		if target.dead {
			return
		}
	}
}

// autoSpellOnSkill is the autospell part of skill_onskillusage: using a
// skill may cast the player's bAutoSpellOnSkill skills, on the skill's
// target, or on the player with AutoSpellForceTarget. A ground skill has
// no target, so only those on the player cast.
func (z *Zone) autoSpellOnSkill(src, target *entity, used *skill.Def, _ int) {
	p := src.player
	if p == nil || z.Skills == nil || len(p.bonuses.AutoSpellOnSkill) == 0 {
		return
	}
	for i := range p.bonuses.AutoSpellOnSkill {
		a := &p.bonuses.AutoSpellOnSkill[i]
		if a.TriggerSkill != used.ID || p.autoSpellLock[i] {
			continue
		}
		d := z.Skills.Get(a.Skill)
		if d == nil || z.autoSpellBlocked(p, a.Skill) {
			continue
		}
		tbl := target
		if a.Flag&status.AutoSpellForceTarget != 0 {
			tbl = src
		}
		if tbl == nil || z.rand.IntN(1000) >= a.Rate {
			continue
		}
		lv := max(a.Level, 1)
		if a.Flag&status.AutoSpellForceRandomLevel != 0 {
			lv = 1 + z.rand.IntN(lv)
		}
		// rAthena checks the trigger skill's units here, not the cast
		// skill's.
		if d.CastType() == skill.CastGround && !z.posMaxCount(src, used, lv, tbl.x, tbl.y, false) {
			continue
		}
		if p.autoSpellLock == nil {
			p.autoSpellLock = map[int]bool{}
		}
		p.autoSpellLock[i] = true
		z.autoCast(src, tbl, d, lv)
		delete(p.autoSpellLock, i)
		if src.dead {
			return
		}
	}
}
