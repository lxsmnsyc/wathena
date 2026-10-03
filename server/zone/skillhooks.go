package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The parts of skills that statuses and bonuses change: the statuses
// that block or reduce a hit (battle_status_block_damage and Energy
// Coat), and the procs of item bonuses (skill_additional_effect and
// skill_counter_additional_effect).

// defender is the target's battle status for one attack, with its HP. A
// target with a status that blocks or reduces hits gets a copy whose
// Changes run them.
func (z *Zone) defender(t *entity) *battle.Status { return z.defenderFor(nil, t, 0) }

// defenderFor is defender for an attack from src, or from no known unit
// when src is nil, with a skill level, 0 for a normal attack. Guard's
// Shrink reads both.
func (z *Zone) defenderFor(src, t *entity, lv int) *battle.Status {
	st := t.status
	l := t.sc
	if st == nil || l.Empty() || !l.Has(sc.SCSafetywall) && !l.Has(sc.SCPneuma) && !l.Has(sc.SCEnergycoat) &&
		!l.Has(sc.SCKyrie) && !l.Has(sc.SCAeterna) && !l.Has(sc.SCParrying) && !l.Has(sc.SCRaid) &&
		!l.Has(sc.SCRejectsword) && !l.Has(sc.SCSpiderweb) && !l.Has(sc.SCAutoguard) &&
		!l.Has(scMillenniumshield) && !l.Has(scDamageHeal) {
		return withHP(st, t.hp)
	}
	cp := *st
	cp.HP = t.hp
	var ch battle.Changes
	if st.Changes != nil {
		ch = *st.Changes
	}
	ch.Block = func(flag, skillID int, dmg int64) bool { return z.blockDamage(src, t, flag, skillID, lv, dmg) }
	if l.Has(sc.SCEnergycoat) {
		ch.EnergyCoat = func() int { return z.energyCoat(t) }
	}
	if l.Has(sc.SCKyrie) {
		ch.Kyrie = func(flag, skillID int, dmg int64) int64 { return z.kyrieBlock(t, flag, skillID, dmg) }
	}
	if l.Has(sc.SCAeterna) {
		ch.Aeterna = func(flag, skillID int) bool { return z.lexAeterna(t, flag, skillID) }
	}
	// Sightless Mind and Reject Sword change no stats, so they are read
	// when the hit comes.
	ch.Raid = l.Has(sc.SCRaid)
	ch.RejectSword = l.Val2(sc.SCRejectsword)
	if l.Has(sc.SCSpiderweb) {
		ch.SpiderWeb = func() bool { return z.spiderWebBurn(t) }
	}
	cp.Changes = &ch
	return &cp
}

// withHP is a status with the unit's HP, which some skills read, such as
// Sonic Blow and Turn Undead. A unit at full HP keeps its status.
func withHP(st *battle.Status, hp int64) *battle.Status {
	if st == nil || hp <= 0 || hp >= int64(st.MaxHP) {
		return st
	}
	cp := *st
	cp.HP = hp
	return &cp
}

// blockDamage is battle_status_block_damage for Safety Wall, Pneuma,
// Guard and Parrying. Safety Wall blocks melee hits that are not magic,
// and uses one of its hits; in renewal its durability also takes the
// damage. Pneuma blocks ranged hits that are not magic. Guard and
// Parrying block weapon hits by their chance. src is the attacker, or
// nil, and lv the attack's skill level.
func (z *Zone) blockDamage(src, t *entity, flag, skillID, lv int, dmg int64) bool {
	l := t.sc
	if e := l.Get(sc.SCSafetywall); e != nil && flag&(battle.FlagShort|battle.FlagMagic) == battle.FlagShort {
		if g := z.groups[int64(e.Val3)]; g != nil {
			if e.Val2 == skill.IDSafetyWall {
				g.val2--
				switch {
				case g.val2 <= 0:
					z.deleteGroup(g)
				case z.renewal():
					if g.val3-dmg > 0 {
						g.val3 -= dmg
					} else {
						z.deleteGroup(g)
					}
				}
			}
			return true
		}
		z.scEnd(t, sc.SCSafetywall)
	}
	if l.Has(sc.SCPneuma) && flag&(battle.FlagMagic|battle.FlagLong) == battle.FlagLong {
		return true
	}
	if z.millenniumShield(src, t, dmg) {
		return true
	}
	if z.autoGuard(src, t, flag, skillID, lv) {
		return true
	}
	// Wand of Hermode blocks magic.
	if l.Has(scHermode) && flag&battle.FlagMagic != 0 {
		return true
	}
	if e := l.Get(sc.SCParrying); e != nil && flag&battle.FlagWeapon != 0 && skillID != idCartTermination &&
		z.rand.IntN(100) < e.Val2 {
		if z.Skills != nil {
			if d := z.Skills.Get(skill.IDParrying); d != nil {
				z.skillEffect(t, t, d, e.Val1, int64(e.Val1), true)
			}
		}
		// unit_set_attackdelay with DELAY_EVENT_PARRY: a player's attack
		// and act delays start again.
		if t.player != nil && t.status != nil {
			t.nextAttack = max(t.nextAttack, z.now+int64(t.status.ADelay))
			t.canAct = max(t.canAct, z.now+int64(t.status.AMotion)+maxASPDDelay)
		}
		return true
	}
	return z.damageHeal(t, flag, dmg)
}

// idCartTermination is WS_CARTTERMINATION, which Parrying cannot block.
const idCartTermination = 485

// energyCoat is Energy Coat's part of battle_calc_damage: it costs 1% of
// max SP plus 0.5% per 20% of SP, ends when the SP cannot be paid, and
// returns the damage it takes away: 6% plus 6% per 20% of SP.
func (z *Zone) energyCoat(t *entity) int {
	st := t.status
	if st == nil || st.MaxSP <= 0 {
		return 0
	}
	sp := int64(st.MaxSP)
	if p := t.player; p != nil {
		sp = p.sp
	}
	per := int(100*sp/int64(st.MaxSP)) - 1
	per /= 20
	cost := int64((10 + 5*per) * st.MaxSP / 1000)
	if p := t.player; p != nil {
		if p.sp < cost {
			z.scEnd(t, sc.SCEnergycoat)
		} else if cost > 0 {
			p.sp -= cost
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		}
	}
	return 6 * (1 + per)
}

// additionalEffectFlags is skill_additional_effect's item procs for an
// attack with battle flags: the attacker's cause_status bonuses, and for
// a skill those on that skill (addeff_onskill).
func (z *Zone) additionalEffectFlags(src, target *entity, flag, skillID int) {
	p := src.player
	en := z.statusEngine()
	if p == nil || en == nil || target.dead {
		return
	}
	su, tu := z.unit(src), z.unit(target)
	long := flag&battle.FlagLong != 0
	effs := p.bonuses.AddEff
	if skillID == skill.IDSoulBreaker && !z.renewal() || skillID == skill.IDCartTermination || skillID == skill.IDBomb {
		// Cart Termination, Bomb, and pre-renewal's Soul Breaker trigger
		// no cause_status bonus. Of rAthena's list of such skills, they
		// are the ones ported.
		effs = nil
	}
	for _, a := range effs {
		rate := a.Rate
		if long {
			rate += a.ArrowRate
		}
		if rate == 0 || !addEffMatchesFlag(a.Flag, flag) {
			continue
		}
		s := sc.Start{Type: a.Status, Rate: rate, Val1: 7, Duration: int64(a.Duration), Delay: addEffDelay,
			Reason: sc.ReasonBonus, Source: su.Ref()}
		if a.Flag&status.ATFTarget != 0 {
			en.Start(su, tu, s)
		}
		if a.Flag&status.ATFSelf != 0 {
			en.Start(su, su, s)
		}
	}
	if skillID == 0 {
		return
	}
	for _, a := range p.bonuses.AddEffOnSkill {
		if a.Skill != skillID || a.Rate == 0 {
			continue
		}
		s := sc.Start{Type: a.Status, Rate: a.Rate, Val1: 7, Duration: int64(a.Duration), Delay: addEffDelay,
			Reason: sc.ReasonBonus, Source: su.Ref()}
		if a.Flag&status.ATFTarget != 0 {
			en.Start(su, tu, s)
		}
		if a.Flag&status.ATFSelf != 0 {
			en.Start(su, su, s)
		}
	}
}

// counterEffectFlags is skill_counter_additional_effect for an attack
// with battle flags that landed on a player.
func (z *Zone) counterEffectFlags(src, target *entity, flag int) {
	p := target.player
	en := z.statusEngine()
	if p == nil || en == nil {
		return
	}
	su, tu := z.unit(src), z.unit(target)
	long := flag&battle.FlagLong != 0
	for _, a := range p.bonuses.AddEffWhenHit {
		rate := a.Rate
		if long {
			rate += a.ArrowRate
		}
		if rate == 0 || !addEffMatchesFlag(a.Flag, flag) {
			continue
		}
		s := sc.Start{Type: a.Status, Rate: rate, Val1: 7, Duration: int64(a.Duration), Delay: addEffDelay,
			Reason: sc.ReasonBonus, Source: tu.Ref()}
		if a.Flag&status.ATFTarget != 0 && src != target {
			en.Start(su, su, s)
		}
		if a.Flag&status.ATFSelf != 0 && !target.dead {
			en.Start(su, tu, s)
		}
	}
}

// addEffMatchesFlag checks an entry's ATF_ flags against an attack's
// battle flags: its kind (weapon, magic or misc) and its range.
func addEffMatchesFlag(atf, flag int) bool {
	const kinds = status.ATFWeapon | status.ATFMagic | status.ATFMisc
	if atf&kinds != kinds {
		switch {
		case flag&battle.FlagWeapon != 0 && atf&status.ATFWeapon == 0,
			flag&battle.FlagMagic != 0 && atf&status.ATFMagic == 0,
			flag&battle.FlagMisc != 0 && atf&status.ATFMisc == 0:
			return false
		}
	}
	long := flag&battle.FlagLong != 0
	if atf&(status.ATFLong|status.ATFShort) != status.ATFLong|status.ATFShort {
		if atf&status.ATFLong != 0 && !long || atf&status.ATFShort != 0 && long {
			return false
		}
	}
	return true
}

// revealTick is the timer of Sight and Ruwach (status_change_timer with
// status_change_timer_sub), run each tick instead of every 20 ms: every
// character in the square around the unit stops hiding. Ruwach also
// hits each enemy it reveals with a level 1 magic hit.
func (z *Zone) revealTick() {
	z.sightBlasterTick()
	var sight, ruwach []*entity
	for e := range z.entities.all() {
		if e.sc.Empty() || e.dead {
			continue
		}
		if e.sc.Has(sc.SCSight) {
			sight = append(sight, e)
		}
		if e.sc.Has(sc.SCRuwach) {
			ruwach = append(ruwach, e)
		}
	}
	for _, e := range sight {
		if z.entities.has(e) && e.sc.Has(sc.SCSight) {
			z.revealArea(e, e.sc.Val3(sc.SCSight))
		}
	}
	if len(ruwach) == 0 || z.Skills == nil {
		return
	}
	d := z.Skills.Get(skill.IDRuwach)
	for _, e := range ruwach {
		if !z.entities.has(e) || !e.sc.Has(sc.SCRuwach) {
			continue
		}
		r := e.sc.Val3(sc.SCRuwach)
		for _, t := range z.findEntities(&query{area: areaRadius, x: e.x, y: e.y, radius: r, kinds: queryPlayers | queryMonsters}) {
			l := t.sc
			if l.Empty() || !l.Has(sc.SCHiding) && !l.Has(sc.SCCloaking) && !l.Has(scCamouflage) && !l.Has(scNewmoon) &&
				!l.Has(sc.SCCloakingexceed) {
				continue
			}
			for _, typ := range []int{sc.SCHiding, sc.SCCloaking, scCamouflage, sc.SCCloakingexceed, scNewmoon} {
				z.scEnd(t, typ)
			}
			if d != nil && isEnemy(e, t) {
				z.skillAttack(battle.TypeMagic, e, e, t, d, 1, 0)
			}
		}
	}
}

// removeMap is the status part of unit_remove_map: the unit leaves its
// cell by a warp, a map change, logout or, for a monster, death.
func (z *Zone) removeMap(e *entity) {
	if en := z.statusEngine(); en != nil && !e.sc.Empty() {
		en.RemoveMap(z.unit(e))
	}
}
