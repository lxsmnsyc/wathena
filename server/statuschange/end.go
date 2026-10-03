package statuschange

// End is status_change_end called by code rather than by the status's
// own timer: the status ends for a reason. It returns whether the
// status was active.
func (en *Engine) End(u Unit, typ int, reason Reason) bool {
	return en.end(u, typ, false, reason)
}

// end is status_change_end. expired is set when the status's own timer
// ends it, as a call with its timer ID.
func (en *Engine) end(u Unit, typ int, expired bool, reason Reason) bool {
	sc := u.Changes()
	e := sc.Get(typ)
	if e == nil {
		return false
	}
	if !expired && typ == SCEndure && e.Val4 != 0 {
		// Infinite Endure does not end.
		return false
	}
	val1, val2, val3 := e.Val1, e.Val2, e.Val3
	delete(sc.entries, typ)
	e.clearTimer()
	def := en.DB.Get(typ)
	if def == nil {
		return true
	}
	if def.States != 0 || def.CondStates {
		sc.calcState(en.DB, en.ruleset().EnsembleSongs)
	}

	recalcAll := false
	switch typ {
	case SCConcentration:
		if c := sc.Get(SCEndure); c != nil && c.Val4 == 0 {
			en.End(u, SCEndure, ReasonReplaced)
		}
	case SCIntravision:
		// Required for overlapping.
		recalcAll = true
	case SCAutoberserk:
		// The Provoke of Auto Berserk ends with it.
		if p := sc.Get(SCProvoke); p != nil && p.Val4 == 1 {
			en.End(u, SCProvoke, ReasonReplaced)
		}
	case SCBerserk:
		// Frenzy leaves 100 HP, unless val2 was cleared (as by a
		// dispel). Its Endure ends, and regen stops for the skill's
		// duration (SC_REGENERATION with val4 RGN_HP|RGN_SP).
		if h, ok := en.env.(HPSetter); ok && u.HP() > 100 && val2 != 0 {
			h.SetHP(u, 100)
		}
		if c := sc.Get(SCEndure); c != nil && c.Val4 != 0 {
			c.Val4 = 0
			en.End(u, SCEndure, ReasonReplaced)
		}
		en.Start(u, u, Start{Type: SCRegeneration, Rate: 10000, Val1: 10, Val4: regenHP | regenSP,
			Duration: int64(def.Duration(val1)), Reason: ReasonSkill, Source: e.Source})
	case SCSplasher:
		// When its own timer ends it, the caster's skill (val2) hits at its
		// level (val1), once the status has ended. The caster is the
		// status's source, which rAthena keeps in val3.
		if c, ok := en.env.(SkillCaster); ok && expired && e.Source.Kind != KindNone {
			defer c.CastendDamage(e.Source, u, val2, val1, sdLevel)
		}
	case SCCloseconfine2:
		// The holder counts one target less, and lets go at none. val2 is
		// 0 when the holder let go first. The fallthrough of rAthena to
		// SC_CLOSECONFINE only looks for SC_CLOSECONFINE2 holders around
		// the target, which status_change_timer_sub does nothing for.
		if val2 != 0 {
			if src := en.env.UnitOf(e.Source); src != nil {
				if h := src.Changes().Get(SCCloseconfine); h != nil {
					h.Val1--
					if h.Val1 <= 0 {
						en.End(src, SCCloseconfine, ReasonRemoved)
					}
				}
			}
		}
	case SCCloseconfine:
		// The holder let go: every character near it that it held goes
		// free (status_change_timer_sub). The range adds the ranges of
		// Close Confine and Back Slide, which most use to get away.
		if val2 > 0 {
			r := 1 + closeConfineRange + backSlidingRange
			if u.Kind() != KindPlayer {
				// skill_get_range2 gives other units than players 9 for a
				// range of 0.
				r = 1 + closeConfineRange + 9
			}
			ref := u.Ref()
			for _, t := range en.env.AreaChars(u, r) {
				if t2 := t.Changes().Get(SCCloseconfine2); t2 != nil && t2.Val2 != 0 && t2.Source == ref {
					t2.Val2 = 0
					en.End(t, SCCloseconfine2, ReasonRemoved)
				}
			}
		}
	case SCStop:
		// NPC_STOP holds two units: the partner's SC_STOP ends too when its
		// val2 names this unit. rAthena finds the partner by val2, a block
		// ID. IDs here are per kind of unit, so the status's source names
		// the partner.
		if val2 != 0 {
			if t := en.env.UnitOf(e.Source); t != nil {
				if p := t.Changes().Get(SCStop); p != nil && p.Val2 != 0 && p.Source == u.Ref() {
					en.End(t, SCStop, ReasonRemoved)
				}
			}
		}
	}

	for _, t := range def.EndOnEnd {
		en.End(u, t, ReasonReplaced)
	}

	// The simulated Explosion Spirits effect only shows on monsters.
	disableOpt := typ == SCIncatkrate && u.Kind() != KindMonster
	if def.Opt1 != Opt1None {
		sc.Opt1 = Opt1None
	}
	sc.Opt2 &^= def.Opt2
	sc.Opt3 &^= def.Opt3
	sc.Option &^= def.Options

	// On Aegis the status packet goes first, then the option packet.
	if def.Icon >= 0 {
		en.env.Icon(u, def.Icon, false, 0, 0, 0, 0)
	}
	if def.Flags.Has(FlagNonPlayer) || (!disableOpt && (def.Flags.Has(FlagSendOption) || def.Flags.Has(FlagOnTouch) ||
		def.Flags.Has(FlagUnitMove) || def.Flags.Has(FlagSendLook))) {
		en.env.OptionChanged(u)
	}
	if def.Changes() || recalcAll {
		calc := def
		if recalcAll {
			calc = nil
		}
		en.env.Recalc(u, calc)
	}
	// The script event sees the status as it was.
	en.env.Ended(u, e, reason)

	// Stone sets in when its waiting time runs out.
	if typ == SCStonewait && expired {
		en.Start(u, u, Start{Type: SCStone, Rate: 10000, Val1: val1, Val2: val2, Duration: int64(val3), Flag: NoAvoid, Reason: e.Reason, Source: e.Source})
	}
	return true
}

// The ranges of RG_CLOSECONFINE and TF_BACKSLIDING in skill_db.yml,
// which the end of SC_CLOSECONFINE reads.
const (
	closeConfineRange = 1
	backSlidingRange  = 0
)

// ClearKind is the type of status_change_clear.
type ClearKind int

const (
	// ClearDeath is type 0: the unit died. Statuses with NoRemoveOnDead
	// stay.
	ClearDeath ClearKind = 0
	// ClearAll is type 1: the unit leaves, and every status ends.
	ClearAll ClearKind = 1
	// ClearKeepBuffs is type 3: statuses with NoClearbuff stay.
	ClearKeepBuffs ClearKind = 3
)

// Clear is status_change_clear: statuses end for a reason, by kind.
func (en *Engine) Clear(u Unit, kind ClearKind, reason Reason) {
	sc := u.Changes()
	if sc.Empty() {
		return
	}
	for _, e := range sc.Sorted() {
		if sc.Get(e.Type) != e {
			continue
		}
		def := en.DB.Get(e.Type)
		if def == nil {
			continue
		}
		if kind == ClearDeath && def.Flags.Has(FlagNoRemoveOnDead) {
			// Elemental Change stays on death only when it is Holy or
			// Dark.
			if e.Type != SCElementalchange || e.Val2 == eleHoly || e.Val2 == eleDark {
				continue
			}
		}
		if kind == ClearKeepBuffs && def.Flags.Has(FlagNoClearBuff) {
			continue
		}
		en.End(u, e.Type, reason)
		if kind == ClearAll && sc.Get(e.Type) != nil {
			// A status that kept itself, such as infinite Endure, goes
			// when the unit leaves.
			delete(sc.entries, e.Type)
		}
	}
	sc.Opt1, sc.Opt2, sc.Opt3 = 0, 0, 0
	if kind == ClearDeath {
		en.env.OptionChanged(u)
	}
}

// Types of ClearBuffs, as rAthena's e_status_change_clear_buffs_flags.
const (
	RemoveBuffs       = 0x01
	RemoveDebuffs     = 0x02
	RemoveOnRefresh   = 0x04
	RemoveChemProtect = 0x08
	RemoveOnLuxAnima  = 0x10
	RemoveOnHermode   = 0x20
)

// ClearBuffs is status_change_clear_buffs: the statuses that a type of
// removal ends. Timed bonuses end apart, in the zone.
func (en *Engine) ClearBuffs(u Unit, typ int) {
	sc := u.Changes()
	for _, e := range sc.Sorted() {
		def := en.DB.Get(e.Type)
		if def == nil || def.Flags.Has(FlagNoClearBuff) {
			continue
		}
		f := def.Flags
		end := false
		switch {
		case typ&RemoveOnLuxAnima != 0 && f.Has(FlagRemoveOnLuxAnima):
			end = true
		case typ&RemoveChemProtect != 0 && f.Has(FlagRemoveChemicalProtect):
			end = true
		case typ&RemoveOnRefresh != 0 && f.Has(FlagRemoveOnRefresh):
			end = true
		case typ&RemoveDebuffs != 0 && f.Has(FlagDebuff):
			end = true
		case typ&RemoveBuffs != 0 && !f.Has(FlagDebuff):
			end = true
		case typ&RemoveOnHermode != 0 && f.Has(FlagRemoveOnHermode):
			end = true
		}
		if e.Type == SCSaturdaynightfever || e.Type == SCBerserk {
			// Marked so they take no HP when they end.
			e.Val2 = 0
		}
		if end {
			reason := ReasonDispell
			if typ&RemoveOnRefresh != 0 {
				reason = ReasonRemoved
			}
			en.End(u, e.Type, reason)
		}
	}
}

// Damaged is the status part of status_damage for damage that is not
// passive: statuses that damage ends do, and Endure counts the hit.
// fromMonster is set when a monster dealt the damage. hp is the damage.
func (en *Engine) Damaged(u Unit, hp int64, fromMonster bool) {
	sc := u.Changes()
	if sc.Empty() {
		return
	}
	isPC := u.Kind() == KindPlayer
	for _, e := range sc.Sorted() {
		typ := e.Type
		// For units other than players, these end only on damage.
		if (typ == SCWinkcharm || typ == SCVoiceofsiren || typ == SCDeepsleep) && !isPC && hp == 0 {
			continue
		}
		def := en.DB.Get(typ)
		if sc.Get(typ) == nil || def == nil || !def.Flags.Has(FlagRemoveOnDamaged) {
			continue
		}
		// An opt1 status that damage breaks still counts for the Fail
		// lists of the same attack, for 10 ms.
		if sc.Opt1 > Opt1None && en.env.Now() >= sc.lastEffectAt {
			sc.lastEffect, sc.lastEffectAt = typ, en.env.Now()+10
		}
		en.End(u, typ, ReasonDamage)
	}
	if e := sc.Get(SCEndure); e != nil && e.Val4 == 0 && fromMonster {
		// Only monsters count down Endure, outside GvG and battlegrounds.
		e.Val2--
		if e.Val2 <= 0 {
			en.End(u, SCEndure, ReasonExpired)
		}
	}
}

// Retime is delete_timer and add_timer on an active status: it ends ms
// from now instead of when it would. It reports whether the status was
// active. Blinding Mist uses it on the blindness of a monster that walks
// out of the mist.
func (en *Engine) Retime(u Unit, typ int, ms int64) bool {
	e := u.Changes().Get(typ)
	if e == nil {
		return false
	}
	at := en.env.Now() + ms
	e.End = at
	en.setTimer(u, e, at)
	return true
}

// Dispell ends the statuses that Dispell ends: those without NoDispell.
// It is the base of SA_DISPELL and of scripts that end debuffs.
func (en *Engine) Dispell(u Unit, debuffsOnly bool) {
	sc := u.Changes()
	for _, e := range sc.Sorted() {
		def := en.DB.Get(e.Type)
		if def == nil || def.Flags.Has(FlagNoDispell) {
			continue
		}
		if debuffsOnly && !def.Flags.Has(FlagDebuff) {
			continue
		}
		en.End(u, e.Type, ReasonDispell)
	}
}

// RemoveMap is the status part of unit_remove_map: the unit leaves its
// map, by a warp, a change of map, death or logout.
//   - Statuses with RemoveOnChangeMap end.
//   - Cloaking and Cloaking Exceed end on units other than players.
//   - The caster's own Battle Chant (SC_GOSPEL) ends.
//   - The Provoke of Auto Berserk ends, so it cannot be kept by leaving
//     the map.
func (en *Engine) RemoveMap(u Unit) {
	sc := u.Changes()
	if sc.Empty() {
		return
	}
	for _, e := range sc.Sorted() {
		if def := en.DB.Get(e.Type); def != nil && def.Flags.Has(FlagRemoveOnChangeMap) {
			en.End(u, e.Type, ReasonRemoved)
		}
	}
	if u.Kind() != KindPlayer {
		en.End(u, SCCloaking, ReasonRemoved)
		en.End(u, SCCloakingexceed, ReasonRemoved)
	}
	if g := sc.Get(SCGospel); g != nil && g.Val4 == BCTSelf {
		en.End(u, SCGospel, ReasonRemoved)
	}
	if p := sc.Get(SCProvoke); p != nil && p.Val4 == 1 {
		en.End(u, SCProvoke, ReasonRemoved)
	}
}

// Logout is map_quit's status part: statuses that rAthena does not save
// end, for the reason logout. debuff_on_logout is 0.
func (en *Engine) Logout(u Unit) {
	sc := u.Changes()
	for _, e := range sc.Sorted() {
		def := en.DB.Get(e.Type)
		if def == nil {
			continue
		}
		if def.Flags.Has(FlagNoSaveInfinite) && (e.Val4 > 0 || e.timer < 0) {
			en.End(u, e.Type, ReasonLogout)
			continue
		}
		if def.Flags.Has(FlagNoSave) {
			en.End(u, e.Type, ReasonLogout)
		}
	}
}

// Saved is one status to save: its type, its values, the time to its
// timer (as chrif_save_scdata) or InfiniteTick, and for a mod status
// the time to its end.
type Saved struct {
	Type                   int
	Val1, Val2, Val3, Val4 int
	Tick                   int64
	// End is the time to a mod status's end that ticks, or 0.
	End int64
}

// Save is chrif_save_scdata: the statuses to save, with the time to
// their timers.
func (en *Engine) Save(u Unit) []Saved {
	sc := u.Changes()
	now := en.env.Now()
	var out []Saved
	for _, e := range sc.Sorted() {
		def := en.DB.Get(e.Type)
		if def == nil {
			continue
		}
		s := Saved{Type: e.Type, Val1: e.Val1, Val2: e.Val2, Val3: e.Val3, Val4: e.Val4, Tick: InfiniteTick}
		if e.timer >= 0 {
			s.Tick = max(e.timer-now, 0)
		}
		if def.Mod != "" && def.TickMs > 0 {
			s.End = e.Remaining(now)
		}
		out = append(out, s)
	}
	return out
}

// Load is chrif_load_scdata for one saved status: it starts again with
// its values, without resistance or setup, for the reason login. A mod
// status gets its bonuses again from its definition.
func (en *Engine) Load(u Unit, s Saved, bonuses []Bonus) bool {
	st := Start{Type: s.Type, Rate: 10000, Val1: s.Val1, Val2: s.Val2, Val3: s.Val3, Val4: s.Val4,
		Duration: s.Tick, Flag: NoAvoid | NoTickDef | Loaded | NoRateDef, Reason: ReasonLogin, Bonuses: bonuses}
	if def := en.DB.Get(s.Type); def != nil && def.Mod != "" && def.TickMs > 0 && s.End != 0 {
		st.Duration, st.NextTick = s.End, s.Tick
	}
	return en.Start(nil, u, st)
}

// Regen flags that SC_REGENERATION's val4 removes, as rAthena's
// RGN_HP and RGN_SP.
const (
	regenHP = 0x1
	regenSP = 0x2
)
