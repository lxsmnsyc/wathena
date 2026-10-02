package statuschange

// The parts of the Bard, Dancer, Clown and Gypsy statuses that
// status_change_start and status_change_timer hold: the values of the
// songs and dances, SC_DANCING's SP cost, and Marionette Control's stat
// transfer. What needs the zone (units on cells, other units) goes
// through Performer.

// Performer is an Env that knows the map's units, which the performance
// statuses read. An Env without it has no song units and no partners.
type Performer interface {
	// SongUnit is the song statuses' part of status_change_timer in
	// pre-renewal: whether a unit of the status's song on the unit's cell
	// still affects it, and that unit's limit.
	SongUnit(u Unit, typ int) (limit int64, ok bool)
	// UnitByID is map_id2bl: the unit of a Source ID, or nil.
	UnitByID(id int) Unit
	// Near is check_distance_bl: whether two units are within cells.
	Near(a, b Unit, cells int) bool
	// RawStats are a player's own stats (sd->status.str and the rest),
	// without job bonuses and equipment.
	RawStats(u Unit) [6]int
}

// Skills and values that the performance statuses read.
const (
	skillMusicalLesson = 315 // BA_MUSICALLESSON
	skillMoonlit       = 395 // CG_MOONLIT
	skillHermode       = 488 // CG_HERMODE
	efstMoon           = 123 // EFST_MOON
	// ringNibelungenMax is RINGNBL_MAX: SC_NIBELUNGEN's effects are 1 to
	// 11.
	ringNibelungenMax = 12
	// maxParameter is battle_config.max_parameter.
	maxParameter = 99
)

// Skills whose SC_DANCING costs SP every few seconds, by skill ID, as
// status_change_timer's case SC_DANCING in pre-renewal.
var danceSPInterval = map[int]int{
	307: 3, 309: 3, 310: 3, 313: 3, 317: 3, 320: 3, 325: 3, // BD_RICHMANKIM to DC_UGLYDANCE
	306: 4, 308: 4, 311: 4, 329: 4, // BD_LULLABY, BD_ETERNALCHAOS, BD_ROKISWEIL, DC_FORTUNEKISS
	488: 5, 312: 5, 319: 5, 327: 5, 321: 5, 330: 5, // CG_HERMODE to DC_SERVICEFORYOU
	322: 6,  // BA_APPLEIDUN
	328: 10, // DC_DONTFORGETME
}

// performanceSetup is the part of status_change_start that sets up the
// performance statuses. It reports whether the status may start.
func (v *setup) performanceSetup(typ int) bool {
	val := &v.val
	renewal := v.en.DB.renewal
	switch typ {
	case SCComa:
		// HP falls to 1, and SP to 1 unless val2 is set. rAthena returns
		// before the status starts, so it never stays.
		var sp int64
		if val[1] == 0 {
			sp = max(v.target.SP()-1, 0)
		}
		v.en.env.Damage(v.target, max(v.target.HP()-1, 0), sp, true, false)
		return false
	case SCDancing:
		// val1 is the skill ID with the level (val3) in its high bits,
		// val2 the unit group, val3 the seconds left, val4 the partner.
		if val[0] == skillMoonlit {
			v.en.env.Icon(v.target, efstMoon, true, v.tick, 0, 0, 0)
		}
		val[0] |= val[2] << 16
		val[2] = int(v.tick / 1000)
		v.tickTime = 1000
	case SCLonging:
		// Pre-renewal's ASPD penalty.
		if !renewal {
			val[1] = 500 - 100*val[0]
		}
	case SCMarionette:
		// Half of each of the caster's own stats, one byte each.
		var stats [6]int
		if v.target.Kind() == KindPlayer {
			if p, ok := v.en.env.(Performer); ok {
				stats = p.RawStats(v.target)
			}
		} else {
			b := v.target.Base()
			stats = [6]int{b.Str, b.Agi, b.Vit, b.Int, b.Dex, b.Luk}
		}
		val[2], val[3] = packStats(stats[0]/2, stats[1]/2, stats[2]/2), packStats(stats[3]/2, stats[4]/2, stats[5]/2)
	case SCMarionette2:
		// The caster's SC_MARIONETTE (val1 is the caster), capped so the
		// target's stats do not pass max_parameter.
		p, ok := v.en.env.(Performer)
		if !ok {
			return false
		}
		caster := p.UnitByID(val[0])
		if caster == nil {
			return false
		}
		pe := caster.Changes().Get(SCMarionette)
		if pe == nil {
			return false
		}
		st := v.st
		give := func(packed, shift, have int) int { return min((packed>>shift)&0xFF, maxParameter-have) }
		val[2] = packStats(give(pe.Val3, 16, st.Str), give(pe.Val3, 8, st.Agi), give(pe.Val3, 0, st.Vit))
		val[3] = packStats(give(pe.Val4, 16, st.Int), give(pe.Val4, 8, st.Dex), give(pe.Val4, 0, st.Luk))
	case SCEnsemblefatigue:
		if renewal {
			// Speed and ASPD penalty.
			val[1] = 30
		}
	case SCRichmankim:
		if renewal {
			// Exp bonus.
			val[1] = 10 + 10*val[0]
		}
	case SCDrumbattle:
		if renewal {
			val[1] = 15 + val[0]*5 // ATK
			val[2] = val[0] * 15   // DEF
		}
	case SCNibelungen:
		if renewal {
			// One of e_nibelungen_status, at random.
			val[1] = v.en.env.RandN(ringNibelungenMax)
		}
	case SCSiegfried:
		if renewal {
			val[1] = val[0] * 3 // Element resistance.
			val[2] = val[0] * 5 // Status resistance.
		}
	case SCWhistle:
		if renewal {
			val[1] = 18 + 2*val[0]           // FLEE.
			val[2] = ((val[0] + 1) / 2) * 10 // Perfect dodge.
		} else {
			v.songInfinite()
		}
	case SCAssncros:
		if renewal {
			// ASPD.
			if val[0] < 10 {
				val[1] = val[0]*2 - 1
			} else {
				val[1] = 20
			}
		} else {
			v.songInfinite()
		}
	case SCPoembragi:
		if renewal {
			val[1] = 2 * val[0] // Cast time reduction.
			val[2] = 3 * val[0] // After-cast delay reduction.
		} else {
			v.songInfinite()
		}
	case SCAppleidun:
		if renewal {
			if val[0] < 10 {
				val[1] = 9 + val[0] // Max HP rate.
			} else {
				val[1] = 20
			}
			val[2] = 2 * val[0] // Potion recovery rate.
		} else {
			// The HP rate: 5 + 2 per level + a tenth of the caster's VIT
			// + half of its Musical Lesson.
			vit := 0
			if v.src != nil {
				vit = v.src.Status().Vit
			}
			val[1] = (5 + 2*val[0]) + vit/10
			if v.src != nil && v.src.Kind() == KindPlayer {
				val[1] += v.src.Skill(skillMusicalLesson) / 2
			}
			v.songInfinite()
		}
	case SCHumming:
		if renewal {
			val[1] = 4 * val[0] // HIT.
		} else {
			v.songInfinite()
		}
	case SCDontforgetme:
		if renewal {
			val[1] = 1 + 30*val[0] // ASPD decrease.
			val[2] = 5 + 2*val[0]  // Speed decrease.
		} else {
			v.songInfinite()
		}
	case SCFortune:
		if renewal {
			val[1] = val[0] * 10 // Critical.
		} else {
			v.songInfinite()
		}
	case SCService4u:
		if renewal {
			if val[0] < 10 {
				val[1] = 9 + val[0] // Max SP rate.
			} else {
				val[1] = 20
			}
			val[2] = 5 + val[0] // SP cost reduction.
		} else {
			v.songInfinite()
		}
	}
	return true
}

// songInfinite is the pre-renewal songs' setup: the icon shows no end,
// and the timer runs at the duration.
func (v *setup) songInfinite() {
	v.tickTime = v.tick
	v.tick = InfiniteTick
}

// packStats packs three stats in one value, a byte each, as
// SC_MARIONETTE's val3 and val4.
func packStats(a, b, c int) int {
	return min(max(a, 0), 0xFF)<<16 | min(max(b, 0), 0xFF)<<8 | min(max(c, 0), 0xFF)
}

// performanceTimer is status_change_timer for the performance statuses.
// It reports whether the status goes on.
func (en *Engine) performanceTimer(u Unit, e *Entry, due int64) bool {
	switch e.Type {
	case SCDancing:
		// The SP each dance costs every few seconds.
		e.Val3--
		if e.Val3 <= 0 {
			return false
		}
		s, sp := 0, 1
		id := e.Val1 & 0xFFFF
		if !en.DB.renewal {
			s = danceSPInterval[id]
		}
		if id == skillMoonlit {
			// 4 SP per level, every 10 seconds.
			sp = 4 * (e.Val1 >> 16)
			s = 10
		}
		if s != 0 && e.Val3%s == 0 {
			if !en.DB.renewal && u.Changes().Has(SCLonging) {
				sp *= 3
			}
			if !en.charge(u, 0, int64(sp)) {
				return false
			}
			// With no SP left the dance ends.
			if u.Kind() == KindPlayer && u.SP() == 0 {
				return false
			}
		}
		en.setTimer(u, e, 1000+due)
		return true
	case SCMarionette, SCMarionette2:
		// It goes on while the partner is within 7 cells.
		if p, ok := en.env.(Performer); ok {
			if o := p.UnitByID(e.Val1); o != nil && p.Near(u, o, 7) {
				en.setTimer(u, e, 1000+due)
				return true
			}
		}
	case SCWhistle, SCAssncros, SCPoembragi, SCAppleidun, SCHumming, SCDontforgetme, SCFortune, SCService4u:
		// refresh_song is off: a song that runs out while its unit still
		// covers the unit lasts again.
		if en.DB.renewal || u.Dead() {
			return false
		}
		p, ok := en.env.(Performer)
		if !ok {
			return false
		}
		limit, ok := p.SongUnit(u, e.Type)
		if !ok {
			return false
		}
		e.Val4 = 0
		en.setTimer(u, e, limit+due)
		return true
	}
	return false
}

// Linger is skill_unit_onleft's part for the pre-renewal songs: a player
// who leaves a song keeps its status for a while, marked by val4 1. It
// reports whether the status was there to linger.
func (en *Engine) Linger(u Unit, typ int, ms int64) bool {
	e := u.Changes().Get(typ)
	if e == nil || e.Val4 != 0 {
		return false
	}
	e.Val4 = 1
	en.setTimer(u, e, en.env.Now()+ms)
	return true
}
