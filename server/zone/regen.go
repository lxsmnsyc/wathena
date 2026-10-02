package zone

import (
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/statuschange"
)

// Regen: players heal HP and SP over time, as rAthena's
// status_natural_heal, which a timer runs every NATURAL_HEAL_INTERVAL
// for every unit. Heals from regen show no number, as status_heal with
// flag 1.

const (
	// naturalHealInterval is NATURAL_HEAL_INTERVAL.
	naturalHealInterval = 500
	// Default intervals from conf/battle/player.conf: natural_healhp_interval,
	// natural_healsp_interval, and natural_heal_skill_interval.
	healHPInterval    = 6000
	healSPInterval    = 8000
	healSkillInterval = 10000
)

// Regen flags, as rAthena's e_regen.
const (
	regenHP = 1 << iota
	regenSP
	regenSkillHP
	regenSkillSP
)

// regenState is a player's regen timing, as regen_data's tick and
// regen_data_sub's tick.
type regenState struct {
	// hp and sp are when HP and SP last healed or were blocked.
	hp, sp int64
	// skillHP and skillSP add up time toward the next skill heal.
	skillHP, skillSP int64
	// bonus adds up time for each regen bonus of the player.
	bonus []int64
}

// naturalHeal is status_natural_heal_timer: every naturalHealInterval, it
// runs status_natural_heal for each player.
func (z *Zone) naturalHeal() {
	if z.now < z.nextHeal {
		return
	}
	diff := z.now - z.prevHeal
	z.prevHeal = z.now
	z.nextHeal += naturalHealInterval
	if z.nextHeal <= z.now {
		z.nextHeal = z.now + naturalHealInterval
	}
	for e := range z.entities.all() {
		if p := e.player; p != nil && e.status != nil {
			z.naturalHealOne(p, diff)
		}
	}
}

// naturalHealOne is status_natural_heal for a standing player with no
// status changes:
//
//   - Nothing heals for the dead, or for an overweight player.
//   - A walking player heals SP only.
//   - HP heals every natural_healhp_interval and SP every
//     natural_healsp_interval, less half of NATURAL_HEAL_INTERVAL, since
//     the timer is not exact. The wait starts again while regen is
//     blocked or the value is full.
//   - Skill regen adds up time and heals every
//     natural_heal_skill_interval while the value is not full.
func (z *Zone) naturalHealOne(p *Player, diff int64) {
	e := p.entity
	r := p.regen
	flag := regenHP | regenSP
	if r.SkillHP > 0 {
		flag |= regenSkillHP
	}
	if r.SkillSP > 0 {
		flag |= regenSkillSP
	}
	// Only skill regen stops at max HP and SP.
	if e.hp >= int64(e.status.MaxHP) {
		flag &^= regenSkillHP
	}
	if p.sp >= int64(e.status.MaxSP) {
		flag &^= regenSkillSP
	}
	// status_calc_regen_rate: statuses such as Poison stop regen.
	if hp, sp := e.sc.Regen(); !hp || !sp {
		if !hp {
			flag &^= regenHP | regenSkillHP
		}
		if !sp {
			flag &^= regenSP | regenSkillSP
		}
	}
	// status_calc_regen_rate: Tension Relax triples HP regen, or lets an
	// overweight player heal at the normal rate. SC_REGENERATION raises
	// the rates, or with val4 removes natural regen.
	rateHP, rateSP := 100, 100
	overweight := p.overweight != item.WeightNormal
	if e.sc.Has(statuschange.SCTensionrelax) {
		if overweight {
			overweight = false
		} else {
			rateHP += 200
		}
	}
	if e.sc.Has(statuschange.SCMagnificat) {
		rateSP += 100
	}
	// Renewal's Ring of Nibelungen: its HP or SP regen effect.
	switch e.sc.Val2(statuschange.SCNibelungen) {
	case ringHPRegen:
		rateHP += 100
	case ringSPRegen:
		rateSP += 100
	}
	if r := e.sc.Get(statuschange.SCRegeneration); r != nil {
		if r.Val4 == 0 {
			rateHP += r.Val2 * 100
			rateSP += r.Val3 * 100
		} else {
			flag &^= r.Val4 & (regenHP | regenSP)
		}
	}
	if z.monkNoSPRegen(p) {
		flag &^= regenSP | regenSkillSP
	}
	if e.dead || (e.sc != nil && e.sc.Option&(statuschange.OptionHide|statuschange.OptionCloak|optionChaseWalk) != 0) {
		flag = 0
	}
	// pc_bleeding and pc_regen: HP and SP lost and gained over time.
	if len(p.bonuses.Regen) > 0 {
		z.regenBonuses(p, diff)
	}
	if flag != 0 && overweight {
		flag = 0
	}
	if len(e.path) > 0 {
		flag &^= regenSkillHP | regenSkillSP | regenHP
	}
	spBefore := p.sp
	s := &p.regenTicks
	if flag&regenHP != 0 {
		rate := int64(float64(healHPInterval)/(float64(rateHP)/100)) - naturalHealInterval/2
		if s.hp+rate <= z.now {
			s.hp = z.now
			if e.hp >= int64(e.status.MaxHP) {
				flag &^= regenHP | regenSkillHP
			} else if h, _ := z.statusHeal(p, int64(r.HP), 0); h < int64(r.HP) {
				flag &^= regenSkillHP
			}
		}
	} else {
		s.hp = z.now
	}
	if flag&regenSP != 0 {
		rate := int64(float64(healSPInterval)/(float64(rateSP)/100)) - naturalHealInterval/2
		if z.furySPRegen(p) {
			// Fury doubles the tick.
			rate = 2*(rate+naturalHealInterval/2) - naturalHealInterval/2
		}
		if s.sp+rate <= z.now {
			s.sp = z.now
			if p.sp >= int64(e.status.MaxSP) {
				flag &^= regenSP | regenSkillSP
			} else if _, h := z.statusHeal(p, 0, int64(r.SP)); h < int64(r.SP) {
				flag &^= regenSkillSP
			}
		}
	} else {
		s.sp = z.now
	}
	if flag&regenSkillHP != 0 {
		s.skillHP += diff
		for s.skillHP >= healSkillInterval {
			s.skillHP -= healSkillInterval
			if h, _ := z.statusHeal(p, int64(r.SkillHP), 0); h < int64(r.SkillHP) {
				break
			}
		}
	}
	if flag&regenSkillSP != 0 {
		s.skillSP += diff
		for s.skillSP >= healSkillInterval {
			s.skillSP -= healSkillInterval
			if _, h := z.statusHeal(p, 0, int64(r.SkillSP)); h < int64(r.SkillSP) {
				break
			}
		}
	}
	if p.sp != spBefore {
		if st := z.battleStats(p); st != nil {
			p.pending = append(p.pending, st)
		}
	}
}

// optionChaseWalk is OPTION_CHASEWALK, which stops regen as hiding does.
const optionChaseWalk = 0x4000

// regenBonuses is pc_bleeding and pc_regen: the hp_loss_over_time,
// sp_loss_over_time, hp_gain_over_time and sp_gain_over_time bonuses.
// Each counts its own time, which starts again at each stat calculation,
// as rAthena rebuilds the lists. Losses never kill.
func (z *Zone) regenBonuses(p *Player, diff int64) {
	e := p.entity
	regen := p.bonuses.Regen
	if len(p.regenTicks.bonus) != len(regen) {
		p.regenTicks.bonus = make([]int64, len(regen))
	}
	var lossHP, lossSP, gainHP, gainSP int64
	dead := e.dead
	for i, r := range regen {
		loss := r.Kind == "hp_loss_over_time" || r.Kind == "sp_loss_over_time"
		if loss && dead {
			continue
		}
		t := &p.regenTicks.bonus[i]
		*t += diff
		for *t >= int64(r.Rate) {
			*t -= int64(r.Rate)
			v := int64(r.Value)
			switch r.Kind {
			case "hp_loss_over_time":
				lossHP += v
			case "sp_loss_over_time":
				lossSP += v
			case "hp_gain_over_time":
				if r.Percent {
					v = int64(e.status.MaxHP) * v / 100
				}
				gainHP += v
			case "sp_gain_over_time":
				if r.Percent {
					v = int64(e.status.MaxSP) * v / 100
				}
				gainSP += v
			}
		}
		if r.Kind == "hp_loss_over_time" && lossHP >= e.hp {
			lossHP = e.hp - 1
		}
	}
	if lossHP > 0 || lossSP > 0 {
		scEnv{z}.Damage(z.unit(e), lossHP, lossSP, true, false)
	}
	if gainHP > 0 || gainSP > 0 {
		_, sp := z.statusHeal(p, gainHP, gainSP)
		if sp > 0 {
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		}
	}
}
