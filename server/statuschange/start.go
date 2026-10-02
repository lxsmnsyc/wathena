package statuschange

import (
	"container/heap"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// Monster modes and elements that starts read, as rAthena's e_mode and
// e_element.
const (
	modeStatusImmune = 0x4000000
	modeMVP          = 0x0080000
	eleWater         = 1
	eleEarth         = 2
	eleFire          = 3
	eleWind          = 4
	eleHoly          = 6
	eleDark          = 7
	eleUndead        = 9
	eleAll           = 10
	raceUndead       = 1
	raceDemon        = 6
)

// isUndead is battle_check_undead with undead_detect_type 0: the undead
// element.
func isUndead(race, ele int) bool { return ele == eleUndead }

// Start is status_change_start: resistance, then the status starts now
// or after its delay. It returns whether the status starts, or will
// start after the delay. src is the unit whose stats resistance reads,
// or nil for none.
func (en *Engine) Start(src, target Unit, s Start) bool {
	db := en.DB
	def := db.Get(s.Type)
	if def == nil {
		return false
	}
	sc := target.Changes()
	// Scripted statuses only work on players.
	if def.Script != "" && target.Kind() != KindPlayer {
		return false
	}
	if target.Kind() != KindNPC && target.Dead() && s.Type != SCNochat && s.Type != SCJailed {
		return false
	}
	if sc.Has(SCGravitycontrol) {
		return false
	}
	st := target.Status()
	if sc.Option&optionMadogear != 0 && s.Flag&NoAvoid != 0 && def.Flags.Has(FlagFailedMado) {
		return false
	}
	// Boss and MVP resistance.
	if st.Mode&modeStatusImmune != 0 && s.Flag&NoAvoid == 0 && def.Flags.Has(FlagBossResist) {
		return false
	}
	if st.Mode&modeMVP != 0 && s.Flag&NoAvoid == 0 && def.Flags.Has(FlagMVPResist) {
		return false
	}
	// EndReturn: ending any of these is all the start does.
	if len(def.EndReturn) > 0 {
		removed := false
		for _, t := range def.EndReturn {
			if sc.Has(t) {
				en.End(target, t, ReasonReplaced)
				removed = true
			}
		}
		if removed {
			return true
		}
	}
	for _, t := range def.Fail {
		if sc.Has(t) || (sc.lastEffect == t && en.env.Now() < sc.lastEffectAt) {
			return false
		}
	}
	duration := s.Duration
	if s.Flag&(NoAvoid|Loaded) == 0 {
		duration = en.scDef(src, target, &s, duration)
		if duration == 0 {
			return false
		}
	}
	if s.Flag&Loaded == 0 && duration != InfiniteTick {
		var srcRef *Source
		if s.Source.Kind != KindNone {
			srcRef = &s.Source
		}
		duration = en.env.Duration(target, srcRef, s.Type, s.Val1, duration)
		if duration == 0 {
			return false
		}
	}
	// The duration is an int32 from here.
	tick := int64(int32(duration)) // #nosec G115 -- rAthena keeps the duration in an int32.
	delay := s.Delay
	switch s.Type {
	case SCStone, SCStonewait, SCFreeze:
		if s.Flag&NoAvoid == 0 {
			// Undead are immune to Freeze and Stone. undead_detect_type
			// is 0, so the race check below never fails first.
			if isUndead(st.Race, st.DefEle) {
				return false
			}
		}
		if s.Type == SCStonewait {
			// The delay is the time until stone sets in.
			s.Val3 = int(max(1, tick-delay))
			tick = delay
			delay = 0
		}
	case SCBleeding:
		delay = 0
	case SCBurning:
		if st.DefEle == eleFire && st.EleLv == 2 {
			return false
		}
	case SCStripweapon, SCStripshield, SCStriparmor, SCStriphelm:
		// The strips need to take something off a player, or they fail. A
		// status loaded from a save takes nothing.
		if (s.Type == SCStripweapon || s.Type == SCStripshield) && s.Val2 == 1 {
			// The effect of Brandish Spear and Grand Cross: nothing comes
			// off.
			s.Val2 = 0
		} else if target.Kind() == KindPlayer && s.Flag&Loaded == 0 && !en.env.Strip(target, s.Type) {
			return false
		}
		if tick == 1 {
			// The shortest duration only strips, without the status.
			return true
		}
	}
	s.Duration = tick
	if delay <= 0 {
		return en.startPostDelay(src, target, &s)
	}
	if db.renewal {
		// Renewal takes the delay from the duration.
		s.Duration = max(1, tick-delay)
	}
	en.seq++
	start := s
	heap.Push(&en.timers, &timer{due: en.env.Now() + delay, seq: en.seq, unit: target, start: &start, src: src})
	return true
}

// scDef is status_get_sc_def with its roll and the status_chance hook:
// the duration, or 0 when the status is resisted.
func (en *Engine) scDef(src, target Unit, s *Start, tick int64) int64 {
	r := Resist(en.DB, src, target, s.Type, s.Rate, tick, s.Flag)
	if r.Immune || r.Duration == 0 {
		return 0
	}
	var srcRef *Source
	if s.Source.Kind != KindNone {
		srcRef = &s.Source
	}
	rate := en.env.Chance(target, srcRef, s.Type, s.Val1, r.Rate)
	if r.Plain {
		// A status without resistance always rolls.
		if en.env.RandN(10000) >= rate {
			return 0
		}
		return r.Duration
	}
	if rate < 10000 && (rate <= 0 || en.env.RandN(10000) >= rate) {
		return 0
	}
	return r.Duration
}

// optionMadogear is OPTION_MADOGEAR.
const optionMadogear = 0x8000000

// startPostDelay is status_change_start_post_delay: the checks that
// happen after the delay, the values each status sets up, and the start
// itself.
func (en *Engine) startPostDelay(src, target Unit, s *Start) bool {
	db := en.DB
	def := db.Get(s.Type)
	if def == nil {
		return false
	}
	sc := target.Changes()
	st := target.Status()
	typ := s.Type
	val1, val2, val3, val4 := s.Val1, s.Val2, s.Val3, s.Val4
	tick := s.Duration
	flag := s.Flag
	undead := isUndead(st.Race, st.DefEle)
	isPC := target.Kind() == KindPlayer

	// Immunities and failures.
	switch typ {
	case SCSignumcrucis:
		// Only demons and the undead element, never players.
		if (!undead && st.Race != raceDemon) || isPC {
			return false
		}
	case SCKyrie, SCTunaparty:
		if target.Kind() == KindMonster {
			return false
		}
	case SCAdrenaline, SCAdrenaline2:
		// Adrenaline Rush needs an axe or a mace.
		if isPC && !adrenalineWeapon(st, typ) {
			return false
		}
	case SCCloaking:
		// Without Cloaking level 3, a wall must be next to the player. The
		// learned level counts, not the level used.
		if isPC && target.Skill(skillCloaking) < 3 && !en.cloakWall(target) {
			return false
		}
	case SCStrfood, SCAgifood, SCVitfood, SCIntfood, SCDexfood, SCLukfood,
		SCFoodStrCash, SCFoodAgiCash, SCFoodVitCash, SCFoodIntCash, SCFoodDexCash, SCFoodLukCash:
		// A stronger food of the other kind keeps a weaker one out.
		if other := foodPair[typ]; sc.Has(other) && sc.Val1(other) > val1 {
			return false
		}
	case SCToxin, SCParalyse, SCVenombleed, SCMagicmushroom, SCDeathhurt, SCPyrexia, SCOblivioncurse, SCLeechesend:
		if val3 == 0 {
			// The caster's part shows no icon.
			flag |= NoIcon
		}
		for _, t := range []int{SCToxin, SCParalyse, SCVenombleed, SCMagicmushroom, SCDeathhurt, SCPyrexia, SCOblivioncurse, SCLeechesend} {
			if e := sc.Get(t); e != nil && e.Val3 == 1 {
				return false
			} else if e != nil && e.Val3 == 0 {
				en.End(target, t, ReasonReplaced)
			}
		}
	}

	// OPT1 statuses do not stack: a new one ends the others.
	if sc.Opt1 > Opt1None && def.Opt1 > Opt1None {
		for _, e := range sc.Sorted() {
			if d := db.Get(e.Type); d != nil && d.Opt1 > Opt1None {
				en.End(target, e.Type, ReasonReplaced)
			}
		}
	}
	for _, t := range def.EndOnStart {
		if e := sc.Get(t); e != nil {
			if t == SCBerserk || t == SCSaturdaynightfever {
				// Marks the status so its end takes no HP.
				e.Val2 = 0
			}
			en.End(target, t, ReasonReplaced)
		}
	}

	// Statuses that end others before they start.
	switch typ {
	case SCBlessing:
		if isPC {
			// Curse ends first; Stone only without Curse. Either way the
			// stats do not rise.
			if sc.Has(SCCurse) {
				en.End(target, SCCurse, ReasonRemoved)
				return true
			} else if sc.Has(SCStone) {
				en.End(target, SCStone, ReasonRemoved)
				return true
			}
		}
		if sc.Has(SCSpirit) && sc.Val2(SCSpirit) == skillSLHigh {
			en.End(target, SCSpirit, ReasonReplaced)
		}
	case SCIncreaseagi:
		if sc.Has(SCSpirit) && sc.Val2(SCSpirit) == skillSLHigh {
			en.End(target, SCSpirit, ReasonReplaced)
		}
	case SCDeluge:
		// Deluge washes off Blinding Mist's blindness.
		if sc.Has(SCFogwall) && sc.Has(SCBlind) {
			en.End(target, SCBlind, ReasonRemoved)
		}
	case SCImpositio:
		if e := sc.Get(SCImpositio); e != nil && e.Val1 > val1 {
			en.End(target, SCImpositio, ReasonReplaced)
		}
	case SCSilence:
		// Silence ends the caster's Battle Chant.
		if e := sc.Get(SCGospel); e != nil && e.Val4 == BCTSelf {
			en.End(target, SCGospel, ReasonReplaced)
		}
	case SCEndure:
		if isPC && target.AlwaysEndure() {
			// bNoWalkDelay already gives endure.
			return true
		}
	}

	// A status already active with a higher level stays.
	if e := sc.Get(typ); e != nil {
		switch typ {
		case SCAdrenaline, SCAdrenaline2, SCWeaponperfection, SCOverthrust:
			if e.Val2 > val2 {
				return false
			}
		case SCGospel:
			// Nothing replaces the caster's own Battle Chant.
			if e.Val4 == BCTSelf {
				return false
			}
			if e.Val1 > val1 {
				return true
			}
		case SCEndure:
			if e.Val4 != 0 && val4 == 0 {
				return true
			}
			if e.Val1 > val1 {
				return true
			}
		case SCJointbeat:
			// A broken neck takes no new breaks until it ends. Other
			// breaks add up.
			if e.Val2&BreakNeck != 0 {
				return false
			}
			val2 |= e.Val2
			if !def.Flags.Has(FlagOverlapIgnoreLevel) && e.Val1 > val1 {
				return true
			}
		default:
			if !def.Flags.Has(FlagOverlapIgnoreLevel) && e.Val1 > val1 {
				return true
			}
		}
	}

	tickTime := int64(0)
	if flag&Loaded == 0 {
		v := setup{en: en, src: src, target: target, st: st, val: [4]int{val1, val2, val3, val4}, tick: tick, flag: flag}
		if !v.run(typ, def) {
			return false
		}
		val1, val2, val3, val4 = v.val[0], v.val[1], v.val[2], v.val[3]
		tick, tickTime, flag = v.tick, v.tickTime, v.flag
	} else {
		// A status loaded from a save: the duration saved is the time to
		// its next timer, and val4 the time after it.
		switch typ {
		case SCStone, SCStonewait, SCPoison, SCDpoison, SCBleeding, SCBurning, SCToxin:
			tickTime = tick
			tick = tickTime + int64(max(val4, 0))
		case SCDeathhurt:
			if val3 != 1 {
				tickTime = tick
				tick = tickTime + int64(max(val4, 0))
			}
		case SCMagicmushroom, SCPyrexia, SCLeechesend:
			if val3 != 0 {
				tickTime = tick
				tick = tickTime + int64(max(val4, 0))
			}
		}
		if def.Mod != "" && def.TickMs > 0 && s.NextTick > 0 {
			// A mod status keeps its time to the next tick.
			tickTime = s.NextTick
		}
	}

	if def.Flags.Has(FlagStopAttacking) {
		en.env.StopAttacking(target)
	}
	if def.Flags.Has(FlagStopWalking) {
		en.env.StopWalking(target)
	}

	if def.Opt1 != Opt1None {
		sc.Opt1 = def.Opt1
	}
	sc.Opt2 |= def.Opt2
	sc.Opt3 |= def.Opt3
	sc.Option |= def.Options
	if def.Flags.Has(FlagSendOption) || def.Flags.Has(FlagOnTouch) || def.Flags.Has(FlagUnitMove) ||
		def.Flags.Has(FlagNonPlayer) || def.Flags.Has(FlagSendLook) {
		en.env.OptionChanged(target)
	}

	now := en.env.Now()
	// A status loaded with DisplayPC, or one with DisplayNPC, sends no
	// icon here.
	hidden := flag&Loaded != 0 && def.Flags.Has(FlagDisplayPC) || def.Flags.Has(FlagDisplayNPC)
	if flag&NoIcon == 0 && !hidden && def.Icon >= 0 {
		v1, v2, v3 := 1, 0, 0
		if def.Flags.Has(FlagSendVal1) {
			v1 = val1
		}
		if def.Flags.Has(FlagSendVal2) {
			v2 = val2
		}
		if def.Flags.Has(FlagSendVal3) {
			v3 = val3
		}
		en.env.Icon(target, def.Icon, true, tick, v1, v2, v3)
	}
	end := int64(InfiniteTick)
	if tick >= 0 {
		end = now + tick
	}
	if tickTime != 0 {
		// The timer runs at the interval; the icon showed the whole time.
		tick = tickTime
	}

	e := sc.entries[typ]
	isNew := e == nil
	if isNew {
		e = &Entry{Type: typ}
		sc.entries[typ] = e
	}
	e.Val1, e.Val2, e.Val3, e.Val4 = val1, val2, val3, val4
	e.End = end
	e.Source, e.Reason, e.HideIcon = s.Source, s.Reason, flag&NoIcon != 0
	e.Bonuses = s.Bonuses
	if tick >= 0 {
		en.setTimer(target, e, now+tick)
	} else {
		e.clearTimer()
	}
	if def.Changes() {
		en.env.Recalc(target, def)
	}
	if isNew && (def.States != 0 || def.CondStates) {
		sc.calcState(db)
	}

	// What runs once the status is in place.
	switch typ {
	case SCGvgStun, SCGvgStone, SCGvgFreez, SCGvgSleep, SCGvgCurse, SCGvgSilence, SCGvgBlind, SCGvgGiant, SCGvgGolem:
		if val1 != 0 || val2 != 0 {
			en.env.Damage(target, int64(val1), int64(val2), true, false)
		}
	case SCBerserk:
		// Frenzy heals to the new max HP and takes all SP, unless val2
		// is already set. val2 then holds the HP each tick takes: 5% of
		// max HP.
		if h, ok := en.env.(HPSetter); ok && e.Val2 == 0 {
			h.ForceHeal(target, int64(target.Status().MaxHP))
			h.SetSP(target, 0)
		}
		e.Val2 = 5 * target.Status().MaxHP / 100
	}
	en.env.Started(target, e)
	return true
}

// foodPair are the food statuses whose stronger value keeps the other
// out, as the food and cash food checks of
// status_change_start_post_delay.
var foodPair = map[int]int{
	SCStrfood: SCFoodStrCash, SCAgifood: SCFoodAgiCash, SCVitfood: SCFoodVitCash,
	SCIntfood: SCFoodIntCash, SCDexfood: SCFoodDexCash, SCLukfood: SCFoodLukCash,
	SCFoodStrCash: SCStrfood, SCFoodAgiCash: SCAgifood, SCFoodVitCash: SCVitfood,
	SCFoodIntCash: SCIntfood, SCFoodDexCash: SCDexfood, SCFoodLukCash: SCLukfood,
}

// Skills that starts read.
const (
	skillCloaking = 135 // AS_CLOAKING
	skillSLHigh   = 494 // SL_HIGH
	skillHilt     = 105 // BS_HILTBINDING
	// skillEDP is ASC_EDP and skillResearchNewPoison
	// GC_RESEARCHNEWPOISON, which SC_SUB_WEAPONPROPERTY reads.
	skillEDP               = 378
	skillResearchNewPoison = 2024
)

// Weapon masks of skill_get_weapontype for BS_ADRENALINE (one- and
// two-handed axes and maces) and BS_ADRENALINE2 (every weapon type from
// W_FIST to W_KATAR but bows), from skill_db.yml.
const (
	adrenalineWeapons  = 1<<6 | 1<<7 | 1<<8
	adrenaline2Weapons = (1<<17 - 1) &^ (1 << 11)
	weaponTypeMax      = 24 // MAX_WEAPON_TYPE
)

// adrenalineWeapon is pc_check_weapontype with the weapon types of
// Adrenaline Rush or Advanced Adrenaline Rush. A dual wield checks
// either hand.
func adrenalineWeapon(st *battle.Status, typ int) bool {
	p := st.Player
	if p == nil {
		return true
	}
	mask := adrenalineWeapons
	if typ == SCAdrenaline2 {
		mask = adrenaline2Weapons
	}
	worn := 1 << uint(p.Weapon&31) // #nosec G115 -- weapon types are below 32.
	if p.Weapon >= weaponTypeMax {
		worn |= 1<<uint(p.RightWeapon&31) | 1<<uint(p.LeftWeapon&31) // #nosec G115 -- as above.
	}
	return mask&worn != 0
}

// Preview is what a start would see before its roll, without changing
// anything: the rate and the duration after resistance. ok is false
// when the start would fail or stop before its roll, as for a boss's
// immunity, a status in its Fail list, or an EndReturn.
func (en *Engine) Preview(src, target Unit, s Start) (rate int, duration int64, ok bool) {
	def := en.DB.Get(s.Type)
	if def == nil {
		return 0, 0, false
	}
	sc := target.Changes()
	if def.Script != "" && target.Kind() != KindPlayer {
		return 0, 0, false
	}
	if target.Kind() != KindNPC && target.Dead() || sc.Has(SCGravitycontrol) {
		return 0, 0, false
	}
	st := target.Status()
	if s.Flag&NoAvoid == 0 && (st.Mode&modeStatusImmune != 0 && def.Flags.Has(FlagBossResist) ||
		st.Mode&modeMVP != 0 && def.Flags.Has(FlagMVPResist)) {
		return 0, 0, false
	}
	for _, t := range def.EndReturn {
		if sc.Has(t) {
			return 0, 0, false
		}
	}
	for _, t := range def.Fail {
		if sc.Has(t) {
			return 0, 0, false
		}
	}
	if s.Flag&(NoAvoid|Loaded) != 0 {
		return s.Rate, s.Duration, true
	}
	r := Resist(en.DB, src, target, s.Type, s.Rate, s.Duration, s.Flag)
	if r.Immune || r.Duration == 0 {
		return 0, 0, false
	}
	return r.Rate, r.Duration, true
}
