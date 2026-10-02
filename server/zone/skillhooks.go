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

// defender is the target's battle status for one attack. A target with a
// status that blocks or reduces hits gets a copy whose Changes run them.
func (z *Zone) defender(t *entity) *battle.Status {
	st := t.status
	l := t.sc
	if st == nil || l.Empty() || !l.Has(sc.SCSafetywall) && !l.Has(sc.SCPneuma) && !l.Has(sc.SCEnergycoat) {
		return st
	}
	cp := *st
	var ch battle.Changes
	if st.Changes != nil {
		ch = *st.Changes
	}
	ch.Block = func(flag, _ int, dmg int64) bool { return z.blockDamage(t, flag, dmg) }
	if l.Has(sc.SCEnergycoat) {
		ch.EnergyCoat = func() int { return z.energyCoat(t) }
	}
	cp.Changes = &ch
	return &cp
}

// blockDamage is battle_status_block_damage for Safety Wall and Pneuma.
// Safety Wall blocks melee hits that are not magic, and uses one of its
// hits; in renewal its durability also takes the damage. Pneuma blocks
// ranged hits that are not magic.
func (z *Zone) blockDamage(t *entity, flag int, dmg int64) bool {
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
	return false
}

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
	for _, a := range p.bonuses.AddEff {
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
