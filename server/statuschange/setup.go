package statuschange

import "github.com/lxsmnsyc/wathena/server/battle"

// Settings from rAthena's conf/battle files that setups read.
const (
	// comboDelayRate is combo_delay_rate: the percent of 300 ms that a
	// combo adds to its time.
	comboDelayRate = 100
	// pcCloakCheckType and monsterCloakCheckType are
	// player_cloak_check_type and monster_cloak_check_type.
	pcCloakCheckType      = 1
	monsterCloakCheckType = 4
)

// Splash areas of the skills that Sight and Ruwach take, from
// skill_db.yml.
var sightSplash = map[int]int{
	10:   3, // MG_SIGHT
	24:   2, // AL_RUWACH
	1006: 1, // WZ_SIGHTBLASTER
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
	case SCAutoberserk:
		// The timer checks every 100 ms whether to start or end the
		// Provoke of Auto Berserk.
		v.tickTime = 100
		v.tick = InfiniteTick
	case SCSignumcrucis:
		val[1] = 10 + 4*val[0]
		v.tick = InfiniteTick
	case SCMaximizepower:
		// A player drains 1 SP every val2 ms, the skill's duration or 10
		// seconds, and the status has no end. Other units keep it for 10
		// seconds.
		if isPC {
			if v.tick <= 0 {
				v.tick = 10000
			}
			val[1] = int(v.tick)
			v.tickTime = v.tick
			v.tick = InfiniteTick
		} else {
			v.tick = 10000
		}
	case SCTrickdead:
		// rAthena also gives the unit the dead look (dead_sit). The zone
		// shows it.
		v.tick = InfiniteTick
	case SCConcentrate:
		// AGI and DEX in percent. val3 and val4 are the AGI and DEX of
		// cards, which the percent leaves out. A player's stat
		// calculation sets them again (status_calc_pc), and one follows
		// the start, so they start at 0 here.
		val[1] = 2 + val[0]
		val[2], val[3] = 0, 0
	case SCProvoke:
		// val2 is the ATK increase and val3 the DEF decrease, in percent.
		// val4 is 1 for the Provoke of Auto Berserk.
		if v.src != nil && v.src.Kind() != KindPlayer && val[0] == 10 {
			val[1] = 0
			val[2] = 100
		} else {
			val[1] = 2 + 3*val[0]
			val[2] = 5 + 5*val[0]
		}
	case SCSubWeaponPro:
		// Enchant Deadly Poison lasts longer with Research New Poison.
		if isPC && val[2] == skillEDP {
			if lv := v.target.Skill(skillResearchNewPoison); lv > 0 {
				v.tick += 30000
				v.tick += int64(lv) * 15 * 1000
			}
		}
	case SCEdp:
		// val2 is the chance to poison, val3 the damage increase in
		// pre-renewal. Research New Poison makes it last longer.
		val[1] = (val[0]+1)/2 + 2
		if !en.DB.renewal {
			val[2] = 50 * (val[0] + 1)
		}
		if isPC {
			if lv := v.target.Skill(skillResearchNewPoison); lv > 0 {
				v.tick += 30000
				v.tick += int64(lv) * 15 * 1000
			}
		}
	case SCPoisonreact:
		// val2 is the number of counters, val3 the chance of a counter and
		// of the poison of the boosted attack, val4 the mode: 0 waits for
		// a poison attack, 1 boosts the next normal attack.
		val[1] = val[0] / 2
		val[2] = 50
		val[3] = 0
	case SCKyrie:
		if val[3] != 0 {
			// Praefatio.
			val[1] = st.MaxHP*(val[0]*2+10)/100 + val[3]*2
			val[2] = 6 + val[0]
		} else {
			val[1] = st.MaxHP * (val[0]*2 + 10) / 100
			val[2] = val[0]/2 + 5
		}
	case SCEncpoison:
		// The chance to poison, per 10,000.
		val[1] = 250 + 50*val[0]
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
	case SCSight, SCRuwach, SCSightblaster:
		val[2] = sightSplash[val[1]]
		val[1] = int(v.tick / 20)
		v.tickTime = 20
	case SCWindwalk:
		val[1] = (val[0] + 1) / 2
	case SCTruesight:
		// Critical and hit.
		val[1] = 10 * val[0]
		val[2] = 3 * val[0]
	case SCBlessing:
		if isPC || (!isUndead(st.Race, st.DefEle) && st.Race != raceDemon) {
			val[1] = val[0]
		} else {
			// Half the stats.
			val[1] = 0
		}
	case SCMaxoverthrust:
		// The ATK increase.
		val[1] = 20 * val[0]
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
	case SCOnehand, SCTwohandquicken:
		// The ASPD rate, more for a boss's skill above level 10.
		val[1] = 300
		if val[0] > 10 {
			val[1] += 20 * (val[0] - 10)
		}
	case SCTensionrelax:
		// rAthena also makes a player sit, which waits for sitting.
		// val2 is the SP cost and val3 the number of 10 second ticks;
		// the client sees no end.
		val[1] = 12
		v.tickTime = 10000
		val[2] = int(v.tick / v.tickTime)
		v.tick = InfiniteTick
	case SCParrying:
		// The block chance.
		val[1] = 20 + val[0]*3
	case SCJointbeat:
		if val[1]&BreakNeck != 0 {
			en.Start(v.src, v.target, Start{Type: SCBleeding, Rate: 10000, Val1: val[0], Val2: val[2],
				Duration: int64(def.Duration2(val[0])), Reason: ReasonSkill})
		}
	case SCBerserk:
		// Endure level 10 with no end comes with it. The heal waits for
		// the stat calculation. val2 holds the HP penalty, val3 the
		// number of ticks and val4 the tick interval.
		en.Start(v.src, v.target, Start{Type: SCEndure, Rate: 10000, Val1: 10, Val4: 1, Duration: v.tick,
			Reason: ReasonSkill})
		if val[3] == 0 {
			val[3] = def.Duration2(val[0])
		}
		if val[3] == 0 {
			val[3] = 10000
		}
		val[2] = int(v.tick / int64(val[3]))
		v.tickTime = int64(val[3])
	case SCRegeneration:
		// The HP and SP regen rates. A val4 blocks regen instead.
		if val[0] == 1 {
			val[1] = 2
		} else {
			val[1] = val[0]
		}
		val[2] = val[0]
	case SCAutoguard:
		// The block chance: 5% for each of the first two levels, then 1%
		// less every two levels, at least 1%. Devotion would pass it on
		// to the devoted, but Devotion needs a party and never starts.
		if v.flag&NoAvoid == 0 {
			val[1] = 0
			for i := 0; i < val[0]; i++ {
				t := 5 - i/2
				if t < 0 {
					t = 1
				}
				val[1] += t
			}
		}
	case SCReflectshield:
		// The percent of melee damage reflected. Devotion would pass it
		// on, as for SC_AUTOGUARD.
		val[1] = 10 + val[0]*3
	case SCProvidence:
		// The holy and demon resistance.
		val[1] = val[0] * 5
	case SCDefender:
		// val2 is the ranged damage reduction and val4 the ASPD penalty.
		// Devotion would pass it on, as for SC_AUTOGUARD.
		if v.flag&NoAvoid == 0 {
			val[1] = 5 + 15*val[0]
			val[2] = 0
			val[3] = 250 - 50*val[0]
		}
	case SCSpearquicken:
		// The ASPD rate, without renewal's ASPD (RENEWAL_ASPD).
		if !en.DB.renewal {
			val[1] = 200 + 10*val[0]
		}
	case SCSacrifice:
		// It lasts 5 hits.
		val[1] = 5
		v.tick = InfiniteTick
	case SCGospel:
		// The caster's own Battle Chant ticks every 10 seconds, and ends
		// its buffs and debuffs.
		if val[3] == BCTSelf {
			val[1] = int(v.tick / 10000)
			v.tickTime = 10000
			en.ClearBuffs(v.target, RemoveBuffs|RemoveDebuffs|RemoveChemProtect)
		}
	case SCAngelus:
		val[1] = 5 * val[0]
	case SCImpositio:
		// Weapon ATK, and in renewal MATK.
		val[1] = 5 * val[0]
	case SCMeltdown:
		// The chances to break the target's weapon and armor, per
		// 10,000.
		val[1] = 100 * val[0]
		val[2] = 70 * val[0]
	case SCMagicpower:
		// MATK percent. In pre-renewal it lasts one cast: val4 is 0
		// while it waits and 1 once a spell started.
		val[2] = 5 * val[0]
		if !en.DB.renewal {
			val[1] = 1
			val[3] = 0
		}
	case SCGravitation:
		// ASPD decrease, a pre-renewal status.
		if !en.DB.renewal {
			val[1] = 50 * val[0]
		}
	case SCQuagmire:
		// AGI and DEX decrease.
		if isPC {
			val[1] = 5 * val[0]
		} else {
			val[1] = 10 * val[0]
		}
	case SCCriticalwound:
		val[0] = 1 + (val[0]-1)%5
		val[1] = 20 * val[0]
	case SCSlowcast:
		val[1] = 20 * val[0]
	case SCExpboost, SCJexpboost, SCPeriodReceiveitem2nd, SCPeriodPlusexp2nd:
		if val[0] < 1 {
			return false
		}
	case SCSuffragium:
		// Cast time reduction in percent.
		if en.DB.renewal {
			val[1] = 5 + val[0]*5
		} else {
			val[1] = 15 * val[0]
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
	case SCExplosionspirits:
		// Critical bonus.
		val[1] = 75 + 25*val[0]
	case SCCombo:
		// val1 is the skill, val2 the target for auto targeting, and val3
		// is set when the combo must not delay attacks and walking (2:
		// only attacks). val4 is the combo time. The zone sets the
		// attack and walk delays from val4 (skill_combo), since the
		// engine has no unit data.
		if val[2] == 0 || val[2] == 2 {
			v.tick += 300 * comboDelayRate / 100
		}
		val[2] = 0
		val[3] = int(v.tick)
	case SCStripweapon, SCStripshield, SCStriparmor, SCStriphelm:
		// Other units than players lose ATK, DEF, VIT or INT, in percent.
		if !isPC {
			val[1] = stripRate[typ]
		}
	case SCChasewalk:
		// val2 is how often SP drains, val3 the speed change, and val4 the
		// SP cost. GvG and battleground maps, where the cost is five
		// times higher, do not exist yet.
		if v.tick > 0 {
			val[1] = int(v.tick)
		} else {
			val[1] = 10000
		}
		val[2] = 35 - 5*val[0]
		if sc := v.target.Changes(); sc.Has(SCSpirit) && sc.Val2(SCSpirit) == skillSLRogue {
			val[2] -= 40
		}
		val[3] = 10 + val[0]*2
	case SCRejectsword:
		// The chance to reflect, and the reflections left.
		val[1] = 15 * val[0]
		val[2] = 3
		v.tick = InfiniteTick
	case SCCloseconfine2:
		// The caster holds the target: it gets SC_CLOSECONFINE, or its
		// hold counts one more target and lasts longer.
		src2 := v.src
		if val[1] == 0 || src2 == nil {
			return false
		}
		if e2 := src2.Changes().Get(SCCloseconfine); e2 == nil {
			// The FLEE bonus.
			if en.DB.renewal {
				val[2] = 50
			} else {
				val[2] = 10
			}
			en.Start(src2, src2, Start{Type: SCCloseconfine, Rate: 10000, Val1: val[0], Val2: 1, Val3: val[2],
				Duration: v.tick + 1000, Reason: ReasonSkill, Source: src2.Ref()})
		} else {
			e2.Val2++
			due := en.env.Now() + v.tick + 1000
			e2.End = due
			en.setTimer(src2, e2, due)
		}
	case SCMagicrod:
		// SP gained, in percent of the skill's cost.
		val[1] = val[0] * 20
	case SCAutospell:
		// val1 is Hindsight's level, val2 the skill to cast and val3 its
		// highest level. val4 is the chance, in percent.
		if en.DB.renewal {
			val[3] = val[0] * 2
		} else {
			val[3] = 5 + val[0]*2
		}
	case SCVolcano, SCViolentgale, SCDeluge:
		// val3 is the damage bonus of the field's element.
		i := max((val[0]-1)%5, 0)
		val[2] = fieldEnchant[i]
		switch typ {
		case SCVolcano:
			// ATK, and MATK in renewal.
			if en.DB.renewal {
				val[1] = 5 + val[0]*5
			} else {
				val[1] = val[0] * 10
				if st.DefEle != eleFire {
					val[1] = 0
				}
			}
		case SCViolentgale:
			// FLEE.
			val[1] = val[0] * 3
			if !en.DB.renewal && st.DefEle != eleWind {
				val[1] = 0
			}
		case SCDeluge:
			// Max HP, in percent.
			val[1] = delugeHP[i]
			if !en.DB.renewal && st.DefEle != eleWater {
				val[1] = 0
			}
		}
	case SCMemorize:
		// Casts left.
		val[1] = 5
		v.tick = InfiniteTick
	case SCMindbreaker:
		// MATK increase and MDEF reduction, in percent.
		val[1] = 20 * val[0]
		val[2] = 12 * val[0]
	case SCDoublecast:
		// The chance of a second bolt, in percent.
		val[1] = 30 + 10*val[0]
	case SCDancing, SCLonging, SCMarionette, SCMarionette2, SCEnsemblefatigue, SCRichmankim, SCDrumbattle,
		SCNibelungen, SCSiegfried, SCWhistle, SCAssncros, SCPoembragi, SCAppleidun, SCHumming, SCDontforgetme,
		SCFortune, SCService4u, SCComa:
		// The Bard and Dancer statuses, and Tarot Card's coma, in
		// bard.go.
		return v.performanceSetup(typ)
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

// stripRate is the stat that a strip takes from other units than
// players, in percent: ATK, DEF, VIT and INT.
var stripRate = map[int]int{SCStripweapon: 25, SCStripshield: 15, SCStriparmor: 40, SCStriphelm: 40}

// skillSLRogue is SL_ROGUE, the Soul Link that speeds up Chase Walk.
const skillSLRogue = 456

// fieldEnchant and delugeHP are the tables of SC_VOLCANO, SC_VIOLENTGALE
// and SC_DELUGE in status_change_start, by level: the damage bonus of the
// field's element, and Deluge's max HP percent.
var (
	fieldEnchant = [5]int{10, 14, 17, 19, 20}
	delugeHP     = [5]int{5, 9, 12, 14, 15}
)

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
