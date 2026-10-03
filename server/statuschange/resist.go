package statuschange

import "github.com/lxsmnsyc/wathena/server/rules"

// Settings from rAthena's conf/battle files that resistance reads.
const (
	// pcSCDefRate and mobSCDefRate are pc_status_def_rate and
	// mob_status_def_rate, in percent.
	pcSCDefRate  = 100
	mobSCDefRate = 100
	// pcMaxSCDef and mobMaxSCDef are pc_max_status_def and
	// mob_max_status_def, in percent.
	pcMaxSCDef  = 100
	mobMaxSCDef = 100
)

// Resistance is what status_get_sc_def computes before its roll: the
// final rate and the duration, and how the duration was found.
type Resistance struct {
	// Rate is the chance, per 10,000, after resistance and min_rate.
	Rate int
	// Duration is the duration after resistance, or 0 when the status is
	// resisted for sure, as by immunity.
	Duration int64
	// Immune is set when the target cannot get the status at all.
	Immune bool
	// Plain is set for a status with no resistance: rAthena rolls the
	// rate given and keeps the duration (at least 1).
	Plain bool
}

// Resist is status_get_sc_def up to its roll: the resistance of the
// target to a status from a source, for a rate per 10,000 and a duration
// in milliseconds. flag holds the SCSTART_ bits that change it.
//
// A source of nil is rAthena's src == nullptr, which skips every check.
// status_isimmune, the immunity of Golden Thief Bug's bonus to magic
// statuses, needs the skill data of step 3 and is not ported.
func Resist(db *DB, src, target Unit, typ, rate int, tick int64, flag StartFlag) Resistance {
	return ResistRules(rules.Of(db.renewal), db, src, target, typ, rate, tick, flag)
}

// ResistRules is Resist under a world's rules: status_resist picks
// status_get_sc_def's formulas, and ensemble_songs Siegfried's part.
func ResistRules(rl *rules.Rules, db *DB, src, target Unit, typ, rate int, tick int64, flag StartFlag) Resistance {
	if src == nil {
		if tick == 0 {
			tick = 1
		}
		return Resistance{Rate: 10000, Duration: tick, Plain: true}
	}
	renewal := rl.StatusResist
	st := target.Status()
	sst := src.Status()
	isPC := target.Kind() == KindPlayer
	sc := target.Changes()
	lv, srcLv := target.Level(), src.Level()

	var scDef, scDef2, tickDef2 int
	tickDef := -1
	levelAdv := 0
	if renewal {
		// pow() is cast to uint16 before the division.
		d := max(0, srcLv-lv)
		levelAdv = int(uint16(d*d)) / 5 * 100 // #nosec G115 -- rAthena casts to uint16.
	}
	switch typ {
	case SCPoison, SCDpoison:
		if renewal {
			scDef = st.Vit*100 - levelAdv
			tickDef2 = -2000
		} else {
			scDef = st.Vit * 100
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			if isPC {
				tickDef = st.Vit * 75
				tickDef2 = st.Luk * 100
			} else {
				tick /= 2
				tickDef = (st.Vit * 200) / 3
			}
		}
	case SCStun:
		if renewal {
			scDef = st.Vit*100 - levelAdv
			tickDef2 = -500
		} else {
			scDef = st.Vit * 100
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			tickDef2 = st.Luk * 10
		}
	case SCSilence:
		if renewal {
			scDef = st.Int*100 - levelAdv
			tickDef2 = -2000
		} else {
			scDef = st.Vit * 100
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			tickDef2 = st.Luk * 10
		}
	case SCBleeding:
		if renewal {
			scDef = st.Agi*100 - levelAdv
			tickDef2 = -12000
		} else {
			scDef = st.Vit * 100
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			tickDef2 = st.Luk * 10
		}
	case SCSleep:
		if renewal {
			scDef = st.Agi*100 - levelAdv
			tickDef2 = -2000
		} else {
			scDef = st.Int * 100
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			tickDef2 = st.Luk * 10
		}
	case SCStonewait:
		if renewal {
			scDef = st.MDef*100 - levelAdv
			tickDef2 = -3000
		} else {
			scDef = st.MDef * 100
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			tickDef = 0
		}
	case SCFreeze:
		if renewal {
			scDef = st.MDef*100 - levelAdv
			tickDef2 = -3000
		} else {
			scDef = st.MDef * 100
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			// The caster's LUK lengthens the duration.
			tickDef2 = sst.Luk * -10
		}
	case SCCurse:
		// Immune with 0 LUK.
		if st.Luk == 0 {
			return Resistance{Immune: true}
		}
		if renewal {
			scDef = st.Luk*100 - levelAdv
			tickDef2 = -2000
		} else {
			scDef = st.Luk * 100
			scDef2 = st.Luk*10 - srcLv*10
			tickDef = st.Vit * 100
			tickDef2 = st.Luk * 10
		}
	case SCBlind:
		if renewal {
			scDef = st.Int*100 - levelAdv
			tickDef2 = -2000
		} else {
			scDef = (st.Vit + st.Int) * 50
			scDef2 = st.Luk*10 + lv*10 - srcLv*10
			tickDef2 = st.Luk * 10
		}
	case SCConfusion:
		if renewal {
			scDef = st.Luk*100 - levelAdv
			tickDef2 = -2000
		} else {
			scDef = (st.Str + st.Int) * 50
			scDef2 = srcLv*10 - lv*10 - st.Luk*10
			tickDef2 = st.Luk * 10
		}
	case SCDecreaseagi:
		if isPC {
			tick /= 2
		}
		scDef2 = st.MDef * 100
	case SCJointbeat:
		tickDef2 = 1000 * ((st.Luk/2 + st.Agi/5) / 2)
	case SCDeepsleep:
		tickDef2 = target.Base().Int*25 + lv*50
	case SCNetherworld:
		tickDef2 = lv*20 + target.JobLevel()*100
	case SCMarshofabyss:
		tickDef2 = (st.Int + st.Luk) * 50
	case SCStasis:
		tickDef2 = (st.Vit + st.Dex) * 50
	case SCWhiteimprison:
		if src != target {
			scDef = st.Str*20 + lv*20 + st.Luk*10
			tickDef2 = -2000
		}
	case SCFear:
		scDef = st.Int*20 + lv*20 + st.Luk*10
		tickDef2 = -4000
	case SCBurning:
		scDef = st.Agi*20 + lv*20 + st.Luk*10
		tickDef2 = -2000
	case SCOblivioncurse:
		scDef = max(st.Int*80, 500)
		tickDef = 0
		tickDef2 = (st.Vit + st.Luk) * 500
	case SCFreezing:
		tickDef2 = (st.Vit + st.Dex) * 50
	case SCToxin, SCParalyse, SCVenombleed, SCMagicmushroom, SCDeathhurt, SCPyrexia, SCLeechesend:
		tickDef2 = (st.Vit + st.Luk) * 500
	case SCBite:
		scDef2 = st.Agi * 25
	case SCElectricshocker:
		tickDef2 = (st.Vit + st.Agi) * 70
	case SCCrystalize:
		tickDef2 = target.Base().Vit * 100
	case SCVacuumExtreme:
		tickDef2 = target.Base().Str * 50
	case SCKyougaku:
		tickDef2 = 30 * st.Int
	case SCParalysis:
		tickDef2 = (st.Vit + st.Luk) * 50
	case SCVoiceofsiren:
		tickDef2 = lv*100 + target.JobLevel()*200
	case SCBTrap:
		tickDef = target.Base().Str * 50
	case SCNorecoverState:
		tickDef2 = st.Luk * 100
	default:
		// A status without resistance, such as a buff: the rate rolls as
		// it is.
		if tick == 0 {
			tick = 1
		}
		return Resistance{Rate: rate, Duration: tick, Plain: true}
	}

	defRate, maxDef := mobSCDefRate, mobMaxSCDef
	if isPC {
		defRate, maxDef = pcSCDefRate, pcMaxSCDef
	}
	if defRate != 100 {
		scDef = scDef * defRate / 100
		scDef2 = scDef2 * defRate / 100
	}
	if renewal {
		scDef = capInt(scDef, 0, maxDef*100)
		scDef2 = capInt(scDef2, 0, maxDef*100)
	} else {
		scDef = min(scDef, maxDef*100)
		scDef2 = min(scDef2, maxDef*100)
	}
	if defRate != 100 {
		tickDef = tickDef * defRate / 100
		tickDef2 = tickDef2 * defRate / 100
	}

	if !sc.Empty() {
		switch {
		case sc.Has(SCScresist):
			scDef += sc.Val1(SCScresist) * 100
		case sc.Has(SCSiegfried) && (!rl.EnsembleSongs || typ == SCBlind || typ == SCStone || typ == SCFreeze || typ == SCStun || typ == SCCurse || typ == SCSleep || typ == SCSilence):
			scDef += sc.Val3(SCSiegfried) * 100
		case sc.Has(SCLeechesend) && sc.Val3(SCLeechesend) == 0:
			if typ == SCBlind || typ == SCStun {
				return Resistance{Immune: true}
			}
		case sc.Has(SCOblivioncurse) && sc.Val3(SCOblivioncurse) == 0:
			if typ == SCSilence || typ == SCCurse {
				return Resistance{Immune: true}
			}
		}
	}

	if tickDef == -1 {
		tickDef = scDef
	}

	if flag&NoRateDef == 0 {
		rate -= rate * scDef / 10000
		rate -= scDef2
		// Item resistance applies to the rate.
		if isPC {
			if r := target.ResEff(typ); r != 0 {
				rate -= rate * r / 10000
			}
		}
		// Aegis accuracy.
		if rate > 0 && rate%10 != 0 {
			rate += 10 - rate%10
		}
	}

	def := db.Get(typ)
	if def != nil {
		rate = max(rate, def.MinRate)
	}
	minDuration := int64(1)
	if def != nil {
		minDuration = def.MinDuration
	}
	res := Resistance{Rate: rate}
	if flag&NoTickDef != 0 {
		res.Duration = max(tick, minDuration)
		return res
	}
	tick -= tick * int64(tickDef) / 10000
	if renewal && isPC {
		// Renewal applies item resistance to the duration too.
		if r := target.ResEff(typ); r != 0 {
			tick -= tick * int64(r) / 10000
		}
	}
	tick -= int64(tickDef2)
	res.Duration = max(tick, minDuration)
	return res
}

// Chance is the chance of a resistance, per 10,000: what the roll
// "rnd() % 10000 < rate" passes, or 10000 when it always passes.
func (r Resistance) Chance() int {
	switch {
	case r.Immune:
		return 0
	case r.Rate >= 10000:
		return 10000
	case r.Rate <= 0:
		return 0
	}
	return r.Rate
}

func capInt(v, lo, hi int) int { return min(max(v, lo), hi) }
