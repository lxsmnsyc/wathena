package statuschange

import "github.com/lxsmnsyc/wathena/server/battle"

// Settings from rAthena's conf/battle files that setups read.
const (
	// pcCloakCheckType and monsterCloakCheckType are
	// player_cloak_check_type and monster_cloak_check_type.
	pcCloakCheckType      = 1
	monsterCloakCheckType = 4
)

// Splash areas of the skills that Sight and Ruwach take, from
// skill_db.yml.
var sightSplash = map[int]int{
	10: 3, // MG_SIGHT
	24: 2, // AL_RUWACH
}

// setup is the switch of status_change_start_post_delay that sets up a
// status's values when it starts, for the statuses this package ports.
type setup struct {
	en          *Engine
	src, target Unit
	st          *battle.Status
	val         [4]int
	tick        int64
	tickTime    int64
	flag        StartFlag
}

// run sets up the values. It returns false when the status fails.
func (v *setup) run(typ int, def *Def) bool {
	en, st := v.en, v.st
	val := &v.val
	isPC := v.target.Kind() == KindPlayer
	switch typ {
	// Permanent effects.
	case SCAeterna, SCModechange, SCWeight50, SCWeight90, SCBrokenweapon, SCBrokenarmor, SCReadystorm,
		SCReadydown, SCReadycounter, SCReadyturn, SCDodge, SCPushCart, SCSpritemable, SCClanInfo,
		SCDailysendmailcnt, SCSoulattack:
		v.tick = InfiniteTick
	case SCDecreaseagi, SCIncreaseagi:
		// AGI change.
		val[1] = 2 + val[0]
	case SCEndure:
		// Hit count. Devotion does not exist yet, so it is not passed on.
		val[1] = 7
		if val[3] != 0 {
			v.tick = InfiniteTick
		}
	case SCSignumcrucis:
		val[1] = 10 + 4*val[0]
		v.tick = InfiniteTick
	case SCKyrie:
		if val[3] != 0 {
			// Praefatio.
			val[1] = st.MaxHP*(val[0]*2+10)/100 + val[3]*2
			val[2] = 6 + val[0]
		} else {
			val[1] = st.MaxHP * (val[0]*2 + 10) / 100
			val[2] = val[0]/2 + 5
		}
	case SCElementalchange:
		// val1 is the element level, val2 the element, and val3 is 1 when
		// a script gives a fixed level.
		if val[1] == 0 {
			val[1] = en.env.RandN(eleAll)
		}
		if val[0] == 1 && val[2] == 0 {
			val[0] = 1 + en.env.RandN(4)
		} else if val[0] > 4 {
			val[0] = 4
		}
		val[2] = 0
	case SCAspdpotion0, SCAspdpotion1, SCAspdpotion2, SCAspdpotion3:
		val[1] = 50 * (2 + typ - SCAspdpotion0)
	case SCDpoison:
		// Lose 10% (15% for others than players) of max HP, but not below
		// a quarter.
		hp := v.target.HP()
		maxHP := int64(st.MaxHP)
		if hp > maxHP/4 {
			pct := int64(15)
			if isPC {
				pct = 10
			}
			diff := maxHP * pct / 100
			if hp-diff < maxHP/4 {
				diff = hp - maxHP/4
			}
			en.env.Damage(v.target, diff, 0, true, false)
		}
		v.tickTime = int64(scInterval(typ))
		val[3] = int(v.tick - v.tickTime)
	case SCStone, SCPoison, SCBleeding, SCBurning, SCKillingAura, SCWinkcharm, SCVoiceofsiren:
		v.tickTime = int64(scInterval(typ))
		val[3] = int(v.tick - v.tickTime)
	case SCToxin:
		if val[2] == 1 {
			v.tickTime = int64(scInterval(typ))
		} else {
			v.tickTime = 1000
		}
		val[3] = int(v.tick - v.tickTime)
	case SCDeathhurt:
		if val[2] != 1 {
			v.tickTime = int64(scInterval(typ))
			val[3] = int(v.tick - v.tickTime)
		}
	case SCLeechesend:
		if val[2] != 0 {
			v.tickTime = int64(scInterval(typ))
			val[3] = int(v.tick - v.tickTime)
		}
	case SCPyrexia:
		if val[2] == 1 {
			// The target is blind while it lasts.
			en.Start(v.src, v.target, Start{Type: SCBlind, Rate: 10000, Val1: val[0], Duration: v.tick,
				Flag: NoAvoid | NoTickDef | NoRateDef, Reason: ReasonSkill})
			v.tickTime = int64(scInterval(typ))
			val[3] = int(v.tick - v.tickTime)
		} else {
			val[1] = 15
		}
	case SCVenombleed:
		if val[2] == 0 {
			val[1] = 30
		}
	case SCMagicmushroom:
		if val[2] == 1 {
			v.tickTime = int64(scInterval(typ))
			val[3] = int(v.tick - v.tickTime)
		} else {
			val[1] = 10
		}
	case SCBossmapinfo:
		// The minimap of the map's boss needs boss tracking, which does
		// not exist yet. rAthena fails without a boss on the map.
		return false
	case SCHiding:
		val[1] = int(v.tick / 1000)
		v.tickTime = 1000
		val[2] = 0
		val[3] = val[0] + 3
	case SCCloaking:
		if isPC {
			// SP drain interval; the client sees no end.
			if v.tick > 0 {
				val[1] = int(v.tick)
			} else {
				val[1] = 10000
			}
			v.tickTime = int64(val[1])
			v.tick = InfiniteTick
		} else {
			val[0] = 10
			v.tick = 10000
		}
		val[2] = 0
		if isPC {
			val[3] |= pcCloakCheckType & 7
		} else {
			val[3] |= monsterCloakCheckType & 7
		}
	case SCSight, SCRuwach:
		val[2] = sightSplash[val[1]]
		val[1] = int(v.tick / 20)
		v.tickTime = 20
	case SCWindwalk:
		val[1] = (val[0] + 1) / 2
	case SCBlessing:
		if isPC || (!isUndead(st.Race, st.DefEle) && st.Race != raceDemon) {
			val[1] = val[0]
		} else {
			// Half the stats.
			val[1] = 0
		}
	case SCOverthrust, SCAdrenaline2, SCAdrenaline, SCWeaponperfection:
		switch typ {
		case SCOverthrust:
			switch {
			case val[1] != 0:
				val[2] = 5 * val[0]
			case !en.DB.renewal:
				val[2] = 5
			case val[0] > 4:
				val[2] = 15
			case val[0] > 2:
				val[2] = 10
			default:
				val[2] = 5
			}
		case SCAdrenaline2, SCAdrenaline:
			if val[1] != 0 {
				val[2] = 300
			} else {
				val[2] = 200
			}
		}
		if v.src != nil && v.src.Kind() == KindPlayer && v.src.Skill(skillHilt) > 0 {
			v.tick += v.tick / 10
		}
	case SCConcentration:
		if en.DB.renewal {
			val[1] = 5 + val[0]*2
			val[3] = 5 + val[0]*2
		} else {
			val[1] = 5 * val[0]
			val[3] = 5 * val[0]
		}
		val[2] = 10 * val[0]
		// Endure level 1 comes with it.
		en.Start(v.src, v.target, Start{Type: SCEndure, Rate: 10000, Val1: 1, Duration: v.tick, Reason: ReasonSkill})
	case SCAngelus:
		val[1] = 5 * val[0]
	case SCCriticalwound:
		val[0] = 1 + (val[0]-1)%5
		val[1] = 20 * val[0]
	case SCSlowcast:
		val[1] = 20 * val[0]
	case SCExpboost, SCJexpboost, SCPeriodReceiveitem2nd, SCPeriodPlusexp2nd:
		if val[0] < 1 {
			return false
		}
	case SCInchealrate:
		if val[0] < 1 {
			val[0] = 1
		}
	case SCFear:
		en.Start(v.src, v.target, Start{Type: SCAnkle, Rate: 10000, Duration: 2000, Reason: ReasonSkill})
	case SCRefresh:
		en.env.Heal(v.target, int64(st.MaxHP)*25/100, 0)
		en.ClearBuffs(v.target, RemoveOnRefresh)
	case SCWeaponblocking:
		val[1] = 10 + 2*val[0]
		val[3] = int(v.tick / 5000)
		v.tickTime = 5000
	case SCOblivioncurse:
		if val[2] != 0 {
			val[3] = int(v.tick / 3000)
			v.tickTime = 3000
		}
	case SCElectricshocker, SCCrystalize:
		val[3] = max(int(v.tick/1000), 1)
		v.tickTime = 1000
	case SCStomachache:
		// SP lost.
		val[1] = 8
		val[3] = int(v.tick / 10000)
		v.tickTime = 10000
	case SCPromoteHealthReserch:
		// val1: 1 used, 2 thrown. val2: small, medium or large. val3:
		// max HP.
		switch val[0] {
		case 1:
			val[2] = 1000*val[1] - 500 + v.target.Level()*10/3
		case 2:
			val[2] = 1000*val[1] - 500 + v.src.Level()*10/3
		}
		val[2] = max(val[2], 0)
	case SCEnergyDrinkReserch:
		// val3: max SP in percent.
		switch val[0] {
		case 1:
			val[2] = v.target.Level()/10 + 5*val[1] - 10
		case 2:
			val[2] = v.src.Level()/10 + 5*val[1] - 10
		}
		val[2] = max(val[2], 0)
	case SCTelekinesisIntense:
		val[1] = 10 * val[0]
		val[2] = 40 * val[0]
	case SCUnlimit:
		val[1] = 50 * val[0]
	case SCAncilla:
		val[0] = 15
		val[1] = 30
	default:
		if def.Mod != "" && def.TickMs > 0 && (v.tick == InfiniteTick || v.tick > def.TickMs) {
			// A mod status that ticks: its timer runs at the interval, and
			// its end comes from Entry.End, so its values stay the mod's.
			v.tickTime = def.TickMs
		}
		if !def.Changes() && def.DurationSkill == 0 && def.Icon < 0 && def.Opt1 == Opt1None && def.Opt2 == 0 &&
			def.States == 0 && def.Flags == 0 && len(def.EndOnStart) == 0 && len(def.EndReturn) == 0 &&
			len(def.Fail) == 0 && len(def.EndOnEnd) == 0 && def.Mod == "" {
			// A bare status with nothing to do.
			return false
		}
	}
	return true
}

// scInterval is status_get_sc_interval: how often a status's timer runs.
func scInterval(typ int) int {
	switch typ {
	case SCPoison, SCLeechesend, SCDpoison, SCDeathhurt, SCGradualGravity, SCKillingAura, SCBossmapinfo:
		return 1000
	case SCWinkcharm, SCVoiceofsiren:
		return 2250
	case SCBurning, SCPyrexia:
		return 3000
	case SCMagicmushroom:
		return 4000
	case SCStone:
		return 5000
	case SCBleeding, SCToxin:
		return 10000
	case SCHellsPlant:
		return 333
	case SCShieldspellHp:
		return 3000
	case SCShieldspellSp:
		return 5000
	case SCStarBurst:
		return 300
	}
	return 0
}
