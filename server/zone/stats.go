package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Stats: a player's battle status comes from its character through
// server/status, as rAthena's status_calc_pc, and is computed again when
// its base or job level changes or it raises a stat. The player gets its
// stats in the first frame and the ones that changed after each
// calculation.

// costIDs are the stats that carry each raise cost, in status.Stat order.
var costIDs = [...]wathenav1.StatId{
	wathenav1.StatId_STAT_ID_STR_COST,
	wathenav1.StatId_STAT_ID_AGI_COST,
	wathenav1.StatId_STAT_ID_VIT_COST,
	wathenav1.StatId_STAT_ID_INT_COST,
	wathenav1.StatId_STAT_ID_DEX_COST,
	wathenav1.StatId_STAT_ID_LUK_COST,
}

// raiseStat is pc_statusup for a player. The status points are the
// progression's, which hold the points left to spend, as rAthena's
// status_point. After a raise, the player's stats are computed again,
// and the next frame carries the changed stats, raise costs and status
// points, as clif_updatestatus does. A raise that fails sends nothing.
// Without job or exp tables, there are no status points to spend.
func (z *Zone) raiseStat(p *Player, st status.Stat, amount int) {
	if z.Status == nil || z.Progress == nil {
		return
	}
	before := z.progressStats(p, nil)
	if !z.Status.StatusUp(&p.char, &p.progress.StatusPoints, st, amount) {
		return
	}
	z.recalc(p)
	if s := z.progressStats(p, before); s != nil {
		p.pending = append(p.pending, s)
	}
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
}

// recalc computes a player's stats, as status_calc_pc. HP and SP above
// their new maximum drop to it, as status_calc_bl_main. Without job
// tables, the player keeps PlayerStatus.
func (z *Zone) recalc(p *Player) {
	e := p.entity
	if z.Status == nil {
		if e.status == nil {
			st := z.PlayerStatus
			e.status = &st
			p.sp = int64(st.MaxSP)
		}
		return
	}
	c := p.char
	c.BaseLevel, c.JobLevel = 1, 1
	if z.Progress != nil {
		c.BaseLevel, c.JobLevel = p.progress.BaseLevel, p.progress.JobLevel
	}
	res, err := z.Status.Calc(c)
	if err != nil {
		// Levels the job tables do not have. The player keeps its stats.
		if e.status == nil {
			st := z.PlayerStatus
			e.status = &st
		}
		return
	}
	oldMax := -1
	if e.status != nil {
		oldMax = e.status.MaxHP
	}
	st := res.Battle
	e.status = &st
	p.maxWeight = res.MaxWeight
	p.window = res.Window
	if st.Speed > 0 {
		e.speed = st.Speed
	}
	if st.MaxHP != oldMax {
		e.hpChanged = true
	}
	if e.hp > int64(st.MaxHP) {
		e.hp, e.hpChanged = int64(st.MaxHP), true
	}
	p.sp = min(p.sp, int64(st.MaxSP))
}

// battleStats is a StatUpdate of the player's stats other than HP, which
// combatEvents sends. It holds only the values that changed since the
// last one, and is nil when none did. The encoding is in
// docs/proposals/0001-protocol.md.
func (z *Zone) battleStats(p *Player) *wathenav1.Event {
	st, w := p.entity.status, p.window
	if z.Status == nil && st.MaxHP == 0 {
		// A zone with no player stats, as in tests, sends none.
		return nil
	}
	if z.Status == nil {
		// Without job tables, the window is the renewal one of the fixed
		// status.
		w = status.Window{ATK: st.BAtk, ATK2: st.WAtk + st.WAtk2 + st.EAtk, MATK: st.MAtkMin, DEF: st.Def2, DEF2: st.Def, MDEF: st.MDef2, MDEF2: st.MDef}
	}
	matk := w.MATK + w.MATK2
	if z.Status != nil && !z.Status.Renewal() {
		// Pre-renewal's window shows MATK as a range.
		matk = w.MATK2
	}
	all := []*wathenav1.Stat{
		stat(wathenav1.StatId_STAT_ID_STR, int64(st.Str)),
		stat(wathenav1.StatId_STAT_ID_AGI, int64(st.Agi)),
		stat(wathenav1.StatId_STAT_ID_VIT, int64(st.Vit)),
		stat(wathenav1.StatId_STAT_ID_INT, int64(st.Int)),
		stat(wathenav1.StatId_STAT_ID_DEX, int64(st.Dex)),
		stat(wathenav1.StatId_STAT_ID_LUK, int64(st.Luk)),
		stat(wathenav1.StatId_STAT_ID_ATK, int64(w.ATK+w.ATK2)),
		stat(wathenav1.StatId_STAT_ID_MATK, int64(matk)),
		stat(wathenav1.StatId_STAT_ID_DEF, int64(w.DEF+w.DEF2)),
		stat(wathenav1.StatId_STAT_ID_MDEF, int64(w.MDEF+w.MDEF2)),
		stat(wathenav1.StatId_STAT_ID_HIT, int64(st.Hit)),
		stat(wathenav1.StatId_STAT_ID_FLEE, int64(st.Flee)),
		// The client shows critical in whole points.
		stat(wathenav1.StatId_STAT_ID_CRITICAL, int64(st.Cri/10)),
		// ASPD as the window shows it: amotion 440 is ASPD 156.
		stat(wathenav1.StatId_STAT_ID_ASPD, int64((2000-st.AMotion)/10)),
		stat(wathenav1.StatId_STAT_ID_MAX_SP, int64(st.MaxSP)),
		stat(wathenav1.StatId_STAT_ID_SP, p.sp),
		stat(wathenav1.StatId_STAT_ID_MAX_WEIGHT, int64(p.maxWeight)),
	}
	if z.Status != nil {
		// The raise costs, as clif_updatestatus sends SP_USTR to SP_ULUK.
		for i, id := range costIDs {
			all = append(all, stat(id, int64(z.Status.NeedStatusPoint(&p.char, status.Stat(i), 1))))
		}
	}
	var changed []*wathenav1.Stat
	for _, s := range all {
		if old, ok := p.sent[s.GetId()]; !ok || old != s.GetValue() {
			changed = append(changed, s)
			p.sent[s.GetId()] = s.GetValue()
		}
	}
	if changed == nil {
		return nil
	}
	return wathenav1.Event_builder{Stats: wathenav1.StatUpdate_builder{Stats: changed}.Build()}.Build()
}
