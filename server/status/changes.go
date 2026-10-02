package status

import (
	"math"

	"github.com/lxsmnsyc/wathena/server/battle"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The status_calc_* functions of status.cpp that status changes modify,
// for the statuses that wAthena ports: the common ailments and the
// statuses items start. Statuses that only skills start come with their
// skills. Each function keeps rAthena's order, since later modifiers
// read the value earlier ones made.

// Elements, as rAthena's e_element.
const (
	eleWater  = 1
	eleEarth  = 2
	eleWind   = 4
	elePoison = 5
	eleGhost  = 8
	eleUndead = 9
)

// Settings and skills that status changes read.
const (
	// maxASPDNoPC is MAX_ASPD_NOPC, the shortest attack delay of other
	// units than players, and amotionDividerNoPC its divider.
	maxASPDNoPC        = 100
	amotionDividerNoPC = 1
	// skillPlagiarism is RG_PLAGIARISM, which renewal's ASPD rate from
	// statuses reads, and skillTunnelDrive RG_TUNNELDRIVE, which the
	// speed of a hidden player reads.
	skillPlagiarism  = 225
	skillTunnelDrive = 213
)

// SC_NIBELUNGEN's effects in renewal, as e_nibelungen_status. Its val2
// is one of them. In pre-renewal val2 is an ATK value of 75 or more, so
// it never matches.
const (
	ringAspdRate = 1 // RINGNBL_ASPDRATE
	ringAtkRate  = 2 // RINGNBL_ATKRATE
	ringMatkRate = 3 // RINGNBL_MATKRATE
	ringHPRate   = 4 // RINGNBL_HPRATE
	ringSPRate   = 5 // RINGNBL_SPRATE
	ringAllStat  = 6 // RINGNBL_ALLSTAT
	ringHit      = 7 // RINGNBL_HIT
	ringFlee     = 8 // RINGNBL_FLEE
)

func capStat(v int) int { return capValue(v, 0, math.MaxUint16) }

// calcStr is status_calc_str.
func calcStr(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	v += l.Val1(sc.SCChasewalk2)
	if l.Has(sc.SCLoud) {
		v += 4
	}
	if l.Has(sc.SCTruesight) {
		v += 5
	}
	if l.Has(sc.SCBlessing) {
		if b := l.Val2(sc.SCBlessing); b != 0 {
			v += b
		} else {
			v -= v / 2
		}
	}
	v -= (l.Val3(sc.SCMarionette) >> 16) & 0xFF
	v += (l.Val3(sc.SCMarionette2) >> 16) & 0xFF
	v -= l.Val1(sc.SCStomachache)
	if l.Val2(sc.SCNibelungen) == ringAllStat {
		v += 15
	}
	return capStat(v)
}

// calcAgi is status_calc_agi.
func calcAgi(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	if e := l.Get(sc.SCConcentrate); e != nil && !l.Has(sc.SCQuagmire) {
		// The percent leaves out the AGI of cards (val3).
		v += (v - e.Val3) * e.Val2 / 100
	}
	v += l.Val1(sc.SCIncallstatus)
	if l.Has(sc.SCTruesight) {
		v += 5
	}
	v += l.Val2(sc.SCIncreaseagi)
	v -= l.Val2(sc.SCDecreaseagi)
	v -= l.Val2(sc.SCQuagmire)
	v -= (l.Val3(sc.SCMarionette) >> 8) & 0xFF
	v += (l.Val3(sc.SCMarionette2) >> 8) & 0xFF
	v -= l.Val1(sc.SCStomachache)
	if l.Val2(sc.SCNibelungen) == ringAllStat {
		v += 15
	}
	return capStat(v)
}

// calcVit is status_calc_vit. Divest Armor lowers the VIT of other
// units than players: a player's SC_STRIPARMOR has val2 0.
func calcVit(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	if l.Has(sc.SCTruesight) {
		v += 5
	}
	v -= (l.Val3(sc.SCMarionette)) & 0xFF
	v += (l.Val3(sc.SCMarionette2)) & 0xFF
	v -= l.Val1(sc.SCStomachache)
	if l.Val2(sc.SCNibelungen) == ringAllStat {
		v += 15
	}
	if e := l.Get(sc.SCStriparmor); e != nil {
		v -= v * e.Val2 / 100
	}
	return capStat(v)
}

// calcInt is status_calc_int.
func calcInt(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	if l.Has(sc.SCTruesight) {
		v += 5
	}
	if l.Has(sc.SCBlessing) {
		if b := l.Val2(sc.SCBlessing); b != 0 {
			v += b
		} else {
			v -= v / 2
		}
	}
	v -= (l.Val4(sc.SCMarionette) >> 16) & 0xFF
	v += (l.Val4(sc.SCMarionette2) >> 16) & 0xFF
	v -= l.Val1(sc.SCStomachache)
	if l.Val2(sc.SCNibelungen) == ringAllStat {
		v += 15
	}
	// Divest Helm lowers the INT of other units than players: a
	// player's SC_STRIPHELM has val2 0.
	if e := l.Get(sc.SCStriphelm); e != nil {
		v -= v * e.Val2 / 100
	}
	return capStat(v)
}

// calcDex is status_calc_dex.
func calcDex(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	if e := l.Get(sc.SCConcentrate); e != nil && !l.Has(sc.SCQuagmire) {
		// The percent leaves out the DEX of cards (val4).
		v += (v - e.Val4) * e.Val2 / 100
	}
	v += l.Val1(sc.SCIncallstatus)
	v -= l.Val2(sc.SCQuagmire)
	if l.Has(sc.SCTruesight) {
		v += 5
	}
	if l.Has(sc.SCBlessing) {
		if b := l.Val2(sc.SCBlessing); b != 0 {
			v += b
		} else {
			v -= v / 2
		}
	}
	v -= (l.Val4(sc.SCMarionette) >> 8) & 0xFF
	v += (l.Val4(sc.SCMarionette2) >> 8) & 0xFF
	v -= l.Val1(sc.SCStomachache)
	if l.Val2(sc.SCNibelungen) == ringAllStat {
		v += 15
	}
	return capStat(v)
}

// calcLuk is status_calc_luk. Curse takes LUK to 0.
func calcLuk(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	if l.Has(sc.SCCurse) {
		return 0
	}
	v += l.Val1(sc.SCIncallstatus)
	if l.Has(sc.SCGloria) {
		v += 30
	}
	if l.Has(sc.SCTruesight) {
		v += 5
	}
	v -= l.Val4(sc.SCMarionette) & 0xFF
	v += l.Val4(sc.SCMarionette2) & 0xFF
	v -= l.Val1(sc.SCStomachache)
	if l.Val2(sc.SCNibelungen) == ringAllStat {
		v += 15
	}
	return capStat(v)
}

// calcWatk is status_calc_watk: weapon ATK from statuses, capped to an
// unsigned short. lv4 is set when pre-renewal's Ring of Nibelungen
// applies: always for other units than players, and for a player whose
// hand holds a level 4 weapon.
func calcWatk(l *sc.List, v int, renewal, lv4 bool) int {
	if l.Empty() {
		return capStat(v)
	}
	if !renewal {
		v += l.Val2(sc.SCDrumbattle)
	}
	v += l.Val2(sc.SCImpositio)
	if !renewal && lv4 {
		v += l.Val2(sc.SCNibelungen)
	}
	return capStat(v)
}

// calcPseudobuffMatk is status_calc_pseudobuff_matk: flat MATK from
// skills. Renewal's Impositio Manus and Volcano add their val2. NV_TRANSCENDENCE's
// part is not ported.
func calcPseudobuffMatk(l *sc.List, v int, renewal bool) int {
	if l.Empty() {
		return capStat(v)
	}
	if renewal {
		v += l.Val2(sc.SCImpositio)
		v += l.Val2(sc.SCVolcano)
	}
	return capStat(v)
}

// calcBatk is status_calc_batk. rAthena does not cap the result.
func calcBatk(l *sc.List, v int, renewal bool) int {
	if l.Empty() {
		return v
	}
	v += l.Val2(sc.SCVolcano)
	if renewal && l.Has(sc.SCLoud) {
		v += 30
	}
	if l.Val2(sc.SCNibelungen) == ringAtkRate {
		v += v * 20 / 100
	}
	return v
}

// calcCritical is status_calc_critical.
func calcCritical(l *sc.List, v int, renewal bool) int {
	if l.Empty() {
		return capValue(v, 10, math.MaxInt16)
	}
	if l.Has(sc.SCExplosionspirits) {
		v += l.Val2(sc.SCExplosionspirits)
	}
	v += l.Val2(sc.SCTruesight)
	v += l.Val2(sc.SCFortune)
	if l.Has(sc.SCCloaking) {
		v += v
	}
	if renewal && l.Has(sc.SCSpearquicken) {
		v += 3 * l.Val1(sc.SCSpearquicken) * 10
	}
	if renewal && l.Has(sc.SCTwohandquicken) {
		v += (2 + l.Val1(sc.SCTwohandquicken)) * 10
	}
	return capValue(v, 10, math.MaxInt16)
}

// calcHit is status_calc_hit.
func calcHit(l *sc.List, v int, renewal bool) int {
	if l.Empty() {
		return capValue(v, 1, math.MaxInt16)
	}
	v += l.Val3(sc.SCTruesight)
	v += l.Val1(sc.SCInchit)
	v += l.Val2(sc.SCHumming)
	v += l.Val3(sc.SCConcentration)
	if l.Has(sc.SCInchitrate) {
		v += v * l.Val1(sc.SCInchitrate) / 100
	}
	if l.Has(sc.SCBlind) {
		v -= v * 25 / 100
	}
	if l.Has(sc.SCFear) {
		v -= v * 20 / 100
	}
	if renewal {
		if l.Has(sc.SCBlessing) {
			v += l.Val1(sc.SCBlessing) * 2
		}
		if l.Has(sc.SCTwohandquicken) {
			v += l.Val1(sc.SCTwohandquicken) * 2
		}
		if l.Has(sc.SCAdrenaline) {
			v += l.Val1(sc.SCAdrenaline)*3 + 5
		}
		if l.Val2(sc.SCNibelungen) == ringHit {
			v += 50
		}
	}
	return capValue(v, 1, math.MaxInt16)
}

// calcFlee is status_calc_flee. The flee penalties of GvG and
// battleground maps wait for those maps.
func calcFlee(l *sc.List, v int, renewal bool) int {
	if l.Empty() {
		return capValue(v, 1, math.MaxInt16)
	}
	// Fixed values.
	v += l.Val1(sc.SCIncflee)
	v += l.Val2(sc.SCWhistle)
	v += l.Val2(sc.SCWindwalk)
	v += l.Val3(sc.SCCloseconfine)
	v += l.Val2(sc.SCViolentgale)
	if renewal {
		v += 2 * l.Val1(sc.SCSpearquicken)
	}
	if l.Val2(sc.SCNibelungen) == ringFlee {
		v += 50
	}
	// Rates.
	if l.Has(sc.SCIncfleerate) {
		v += v * l.Val1(sc.SCIncfleerate) / 100
	}
	if l.Has(sc.SCBerserk) {
		v -= v * 50 / 100
	}
	if l.Has(sc.SCSpiderweb) {
		v -= v * 50 / 100
	}
	if l.Has(sc.SCBlind) {
		v -= v * 25 / 100
	}
	if l.Has(sc.SCFear) {
		v -= v * 20 / 100
	}
	if l.Has(sc.SCParalyse) && l.Val3(sc.SCParalyse) == 1 {
		v -= v * 10 / 100
	}
	return capValue(v, 1, math.MaxInt16)
}

// calcFlee2 is status_calc_flee2.
func calcFlee2(l *sc.List, v int) int {
	v += l.Val3(sc.SCWhistle)
	return capValue(v, 10, math.MaxInt16)
}

// calcDef is status_calc_def. cap is DEFTYPE's range.
func calcDef(l *sc.List, v int, renewal, player bool, capDef func(int) int) int {
	if l.Empty() {
		return capDef(v)
	}
	if l.Has(sc.SCBerserk) {
		return 0
	}
	// Steel Body gives 90 DEF only in pre-renewal.
	if !renewal && l.Has(sc.SCSteelbody) {
		return 90
	}
	if renewal && l.Has(sc.SCEternalchaos) {
		return 0
	}
	v += l.Val3(sc.SCDrumbattle)
	if renewal && l.Has(sc.SCAssumptio) {
		v += l.Val1(sc.SCAssumptio) * 50
	}
	if l.Has(sc.SCIncdefrate) {
		v += v * l.Val1(sc.SCIncdefrate) / 100
	}
	if l.Has(sc.SCStone) {
		v /= 2
	}
	if l.Has(sc.SCFreeze) {
		v /= 2
	}
	if (l.Has(sc.SCPoison) || l.Has(sc.SCDpoison)) && !player {
		v = v * 75 / 100
	}
	if l.Has(sc.SCSignumcrucis) {
		v -= v * l.Val2(sc.SCSignumcrucis) / 100
	}
	if l.Has(sc.SCConcentration) {
		v -= v * l.Val4(sc.SCConcentration) / 100
	}
	if l.Has(sc.SCProvoke) && !player {
		// Provoke does not lower the hard DEF of players.
		v -= v * l.Val3(sc.SCProvoke) / 100
	}
	if l.Has(sc.SCStripshield) && !player {
		// Divest Shield lowers only the DEF of other units than players.
		v -= v * l.Val2(sc.SCStripshield) / 100
	}
	return capDef(v)
}

// calcDef2 is status_calc_def2. vit is the unit's VIT, which renewal's
// Angelus reads.
func calcDef2(l *sc.List, v int, renewal bool, vit int) int {
	lo := 1
	if renewal {
		lo = math.MinInt16
	}
	if l.Empty() {
		return capValue(v, lo, math.MaxInt16)
	}
	if l.Has(sc.SCBerserk) || l.Has(sc.SCEternalchaos) {
		return 0
	}
	if l.Has(sc.SCAngelus) {
		if renewal {
			// Angelus raises the VIT part of DEF.
			v += vit / 2 * l.Val2(sc.SCAngelus) / 100
		} else {
			v += v * l.Val2(sc.SCAngelus) / 100
		}
	}
	if !renewal && l.Has(sc.SCConcentration) {
		v -= v * l.Val4(sc.SCConcentration) / 100
	}
	if l.Has(sc.SCPoison) || l.Has(sc.SCDpoison) {
		v = v * 75 / 100
	}
	if l.Has(sc.SCProvoke) {
		v -= v * l.Val3(sc.SCProvoke) / 100
	}
	if e := l.Get(sc.SCJointbeat); e != nil {
		// A broken shoulder takes 50% and a broken waist 25%.
		shoulder, waist := 0, 0
		if e.Val2&sc.BreakShoulder != 0 {
			shoulder = 50
		}
		if e.Val2&sc.BreakWaist != 0 {
			waist = 25
		}
		v -= v*shoulder/100 + v*waist/100
	}
	return capValue(v, lo, math.MaxInt16)
}

// calcMdef is status_calc_mdef.
func calcMdef(l *sc.List, v int, renewal bool, capDef func(int) int) int {
	if l.Empty() {
		return capDef(v)
	}
	if l.Has(sc.SCBerserk) {
		return 0
	}
	// Steel Body gives 90 MDEF only in pre-renewal.
	if !renewal && l.Has(sc.SCSteelbody) {
		return 90
	}
	if e := l.Get(sc.SCEndure); e != nil && e.Val3 == 0 {
		// Eddga's card gives 1 MDEF.
		if e.Val4 == 0 {
			v += e.Val1
		} else {
			v++
		}
	}
	if l.Has(sc.SCStone) {
		v += 25 * v / 100
	}
	if l.Has(sc.SCFreeze) {
		v += 25 * v / 100
	}
	return capDef(v)
}

// calcMdef2 is status_calc_mdef2.
func calcMdef2(l *sc.List, v int, renewal bool) int {
	lo := 1
	if renewal {
		lo = math.MinInt16
	}
	if l.Empty() {
		return capValue(v, lo, math.MaxInt16)
	}
	if l.Has(sc.SCBerserk) {
		return 0
	}
	if l.Has(sc.SCMindbreaker) {
		v -= v * l.Val3(sc.SCMindbreaker) / 100
	}
	return capValue(v, lo, math.MaxInt16)
}

// speedInput is what status_calc_speed reads of a player besides its
// statuses: the item speed bonuses and the skills.
type speedInput struct {
	player     bool
	itemSpeed  int
	assassinTF int
	tunnel     int
	// riding is pc_isriding: the player rides a Peco Peco.
	riding bool
	// freeCast is the level of Free Cast of a player who casts, or 0.
	freeCast int
	// cart is pc_iscarton, and pushCart the level of MC_PUSHCART.
	cart     bool
	pushCart int
	// danceLesson is the level of Musical Lesson for a male player and
	// of Dancing Lesson for a female one, which lessens SC_DANCING's
	// slowdown.
	danceLesson int
}

// calcSpeed is status_calc_speed, without the mounts other than the Peco
// Peco and the speed changes of casting, which wait for them.
func calcSpeed(l *sc.List, speed int, in speedInput) int {
	rate := 100
	if in.freeCast > 0 {
		// A player with Free Cast walks while casting, slower.
		rate = 175 - 5*in.freeCast
		return calcSpeedEnd(l, speed, rate, in)
	}
	// GetMoveHasteValue2: mounts.
	val := 0
	if in.player && in.riding {
		val = 25
	}
	rate -= val
	// GetMoveSlowValue.
	if in.player && l.Has(sc.SCHiding) && in.tunnel > 0 {
		val = 120 - 6*in.tunnel
	} else if e := l.Get(sc.SCChasewalk); in.player && e != nil && e.Val3 < 0 {
		val = e.Val3
	} else {
		val = 0
		// Longing for Freedom and renewal's Ensemble Fatigue replace the
		// slowdown of performing.
		switch {
		case l.Has(sc.SCEnsemblefatigue):
			val = max(val, l.Val2(sc.SCEnsemblefatigue))
		case l.Has(sc.SCLonging):
			val = max(val, 50-10*l.Val1(sc.SCLonging))
		case in.player && l.Has(sc.SCDancing):
			val = max(val, 500-40*in.danceLesson)
		}
		if l.Has(sc.SCDecreaseagi) {
			val = max(val, 25)
		}
		if l.Has(sc.SCQuagmire) {
			val = max(val, 50)
		}
		if l.Has(sc.SCDontforgetme) {
			val = max(val, l.Val3(sc.SCDontforgetme))
		}
		if l.Has(sc.SCCurse) {
			val = max(val, 300)
		}
		if l.Has(sc.SCChasewalk) {
			val = max(val, l.Val3(sc.SCChasewalk))
		}
		if l.Has(sc.SCWedding) {
			val = max(val, 100)
		}
		if e := l.Get(sc.SCJointbeat); e != nil && e.Val2&(sc.BreakAnkle|sc.BreakKnee) != 0 {
			slow := 0
			if e.Val2&sc.BreakAnkle != 0 {
				slow += 50
			}
			if e.Val2&sc.BreakKnee != 0 {
				slow += 30
			}
			val = max(val, slow)
		}
		if e := l.Get(sc.SCCloaking); e != nil && e.Val4&1 == 0 {
			if e.Val1 < 3 {
				val = max(val, 300)
			} else {
				val = max(val, 30-3*e.Val1)
			}
		}
		if l.Has(sc.SCGospel) && l.Val4(sc.SCGospel) == sc.BCTEnemy {
			val = max(val, 75)
		}
		if l.Has(sc.SCSlowdown) {
			val = max(val, l.Val1(sc.SCSlowdown))
		}
		if in.player && in.itemSpeed > 0 {
			// A slowdown from items.
			val = max(val, in.itemSpeed)
		}
	}
	rate += val
	val = 0
	// GetMoveHasteValue1.
	if l.Has(sc.SCIncreaseagi) {
		val = max(val, 25)
	}
	if l.Has(sc.SCWindwalk) {
		val = max(val, 2*l.Val1(sc.SCWindwalk))
	}
	if l.Has(sc.SCCartboost) {
		val = max(val, 20)
	}
	if in.assassinTF > 0 {
		val = max(val, in.assassinTF)
	}
	if e := l.Get(sc.SCCloaking); e != nil && e.Val4&1 == 1 {
		if e.Val1 >= 10 {
			val = max(val, 25)
		} else {
			val = max(val, 3*e.Val1-3)
		}
	}
	if l.Has(sc.SCBerserk) {
		val = max(val, 25)
	}
	if l.Has(sc.SCParalyse) && l.Val3(sc.SCParalyse) == 0 {
		val = max(val, 50)
	}
	if l.Has(sc.SCHovering) {
		val = max(val, 10)
	}
	if in.player && in.itemSpeed < 0 {
		// A speedup from items.
		val = max(val, -in.itemSpeed)
	}
	rate -= val
	rate = max(rate, 40)
	return calcSpeedEnd(l, speed, rate, in)
}

// calcSpeedEnd is the GetSpeed part of status_calc_speed: the rate and
// the statuses that change the speed itself.
func calcSpeedEnd(l *sc.List, speed, rate int, in speedInput) int {
	// A cart slows its pusher by 50% less 5% per level of Pushcart.
	if in.player && in.cart {
		speed += speed * (50 - 5*in.pushCart) / 100
	}
	if l.Has(sc.SCParalyse) && l.Val3(sc.SCParalyse) == 1 {
		speed += speed * 50 / 100
	}
	if rate != 100 {
		speed = speed * rate / 100
	}
	if l.Has(sc.SCSteelbody) {
		speed = 200
	}
	// Defending Aura walks at speed 200 or slower.
	if l.Has(sc.SCDefender) {
		speed = max(speed, 200)
	}
	return capValue(speed, minWalkSpeed, maxWalkSpeedCap)
}

// calcAspdBonus is renewal's status_calc_aspd: the fixed bonus that
// status_base_amotion_pc reads, or the rate bonus. The rate bonus adds
// Musical Lesson and Plagiarism only while some status is active, as
// rAthena reads them after its empty check.
func calcAspdBonus(l *sc.List, fixed, player bool, skill func(int) int) int {
	if l.Empty() {
		return 0
	}
	bonus := 0
	if fixed {
		// Quagmire stops the quickening statuses.
		if !l.Has(sc.SCQuagmire) {
			if bonus < 7 && (l.Has(sc.SCTwohandquicken) || l.Has(sc.SCOnehand) || l.Has(sc.SCAdrenaline) ||
				l.Has(sc.SCSpearquicken)) {
				bonus = 7
			} else if bonus < 6 && l.Has(sc.SCAdrenaline2) {
				bonus = 6
			}
		}
		if e := l.Get(sc.SCAssncros); e != nil && bonus < e.Val2 {
			bonus += e.Val2
		}
		if bonus < 15 && l.Has(sc.SCBerserk) {
			bonus = 15
		}
		for _, t := range []int{sc.SCAspdpotion3, sc.SCAspdpotion2, sc.SCAspdpotion1, sc.SCAspdpotion0} {
			if l.Has(t) {
				bonus += l.Val1(t)
				break
			}
		}
		return bonus
	}
	bonus -= l.Val4(sc.SCDefender) / 10
	if l.Has(sc.SCGospel) && l.Val4(sc.SCGospel) == sc.BCTEnemy {
		bonus -= 75
	}
	if player && l.Has(sc.SCDontforgetme) {
		bonus -= l.Val2(sc.SCDontforgetme) / 10
	}
	bonus -= l.Val2(sc.SCEnsemblefatigue)
	if e := l.Get(sc.SCJointbeat); e != nil {
		if e.Val2&sc.BreakWrist != 0 {
			bonus -= 25
		}
		if e.Val2&sc.BreakKnee != 0 {
			bonus -= 10
		}
	}
	if l.Has(sc.SCParalyse) && l.Val3(sc.SCParalyse) == 1 {
		bonus -= 10
	}
	if l.Has(sc.SCSteelbody) {
		bonus -= 25
	}
	bonus += l.Val1(sc.SCIncreaseagi)
	if l.Val2(sc.SCNibelungen) == ringAspdRate {
		bonus += 20
	}
	if player && skill != nil {
		bonus += skill(skillMusicalLesson)
		bonus += skill(skillPlagiarism)
	}
	return bonus
}

// aspdRangedWeapon reports whether a weapon type is a bow or a gun, which
// Impressive Riff does not quicken.
func aspdRangedWeapon(weapon int) bool {
	switch weapon {
	case weaponBow, weaponRevolver, weaponRifle, weaponGatling, weaponShotgun, weaponGrenade:
		return true
	}
	return false
}

// calcFixAspd is status_calc_fix_aspd.
func calcFixAspd(l *sc.List, aspd int) int {
	return capValue(aspd, 1, minASPD)
}

// calcAspdRate is status_calc_aspd_rate, which pre-renewal players and
// other units read, for a unit that is not a player.
func calcAspdRate(l *sc.List, rate int) int { return calcAspdRateOf(l, rate, -1) }

// calcAspdRateOf is status_calc_aspd_rate. weapon is a player's weapon
// type (sd->status.weapon), or -1 for other units.
func calcAspdRateOf(l *sc.List, rate, weapon int) int {
	if l.Empty() {
		return capValue(rate, 0, math.MaxInt16)
	}
	most := 0
	if l.Has(sc.SCTwohandquicken) && most < l.Val2(sc.SCTwohandquicken) {
		most = l.Val2(sc.SCTwohandquicken)
	}
	if l.Has(sc.SCOnehand) && most < l.Val2(sc.SCOnehand) {
		most = l.Val2(sc.SCOnehand)
	}
	if l.Has(sc.SCAdrenaline2) && most < l.Val3(sc.SCAdrenaline2) {
		most = l.Val3(sc.SCAdrenaline2)
	}
	if l.Has(sc.SCAdrenaline) && most < l.Val3(sc.SCAdrenaline) {
		most = l.Val3(sc.SCAdrenaline)
	}
	if l.Has(sc.SCSpearquicken) && most < l.Val2(sc.SCSpearquicken) {
		most = l.Val2(sc.SCSpearquicken)
	}
	// A Bard's song does not quicken bows and guns.
	if e := l.Get(sc.SCAssncros); e != nil && most < e.Val2 && !aspdRangedWeapon(weapon) {
		most = e.Val2
	}
	rate -= most
	if l.Has(sc.SCBerserk) {
		rate -= 300
	}
	for _, t := range []int{sc.SCAspdpotion3, sc.SCAspdpotion2, sc.SCAspdpotion1, sc.SCAspdpotion0} {
		if l.Has(t) {
			rate -= l.Val2(t)
			break
		}
	}
	rate += l.Val4(sc.SCDefender)
	if l.Has(sc.SCGospel) && l.Val4(sc.SCGospel) == sc.BCTEnemy {
		rate += 250
	}
	if weapon >= 0 {
		rate += l.Val2(sc.SCDontforgetme)
	}
	rate += l.Val2(sc.SCEnsemblefatigue)
	rate += l.Val2(sc.SCLonging)
	// Pre-renewal's Gravitation Field slows attacks.
	rate += l.Val2(sc.SCGravitation)
	if e := l.Get(sc.SCJointbeat); e != nil {
		if e.Val2&sc.BreakWrist != 0 {
			rate += 250
		}
		if e.Val2&sc.BreakKnee != 0 {
			rate += 100
		}
	}
	if l.Has(sc.SCSteelbody) {
		rate += 250
	}
	if l.Has(sc.SCParalyse) && l.Val3(sc.SCParalyse) == 1 {
		rate += 100
	}
	return capValue(rate, 0, math.MaxInt16)
}

// calcElement is status_calc_element: the defense element.
func calcElement(l *sc.List, ele int) int {
	switch {
	case l.Empty():
		return capValue(ele, 0, math.MaxUint8)
	case l.Has(sc.SCFreeze):
		return eleWater
	case l.Has(sc.SCStone):
		return eleEarth
	case l.Has(sc.SCBenedictio):
		return eleHoly
	case l.Has(sc.SCChangeundead):
		return eleUndead
	case l.Has(sc.SCElementalchange):
		return l.Val2(sc.SCElementalchange)
	}
	return capValue(ele, 0, math.MaxUint8)
}

// calcElementLv is status_calc_element_lv.
func calcElementLv(l *sc.List, lv int) int {
	switch {
	case l.Empty():
		return capValue(lv, 1, 4)
	case l.Has(sc.SCFreeze), l.Has(sc.SCStone), l.Has(sc.SCBenedictio), l.Has(sc.SCChangeundead):
		return 1
	case l.Has(sc.SCElementalchange):
		return l.Val1(sc.SCElementalchange)
	}
	return capValue(lv, 1, 4)
}

// calcAttackElement is status_calc_attack_element.
func calcAttackElement(l *sc.List, ele int) int {
	switch {
	case l.Empty():
		return capValue(ele, 0, math.MaxUint8)
	case l.Has(sc.SCWaterweapon):
		return eleWater
	case l.Has(sc.SCEarthweapon):
		return eleEarth
	case l.Has(sc.SCFireweapon):
		return eleFire
	case l.Has(sc.SCWindweapon):
		return eleWind
	case l.Has(sc.SCEncpoison):
		return elePoison
	case l.Has(sc.SCAspersio):
		return eleHoly
	case l.Has(sc.SCShadowweapon):
		return eleDark
	case l.Has(sc.SCGhostweapon):
		return eleGhost
	}
	return capValue(ele, 0, math.MaxUint8)
}

// hpBonusSC is the status part of status_get_hpbonus with
// STATUS_BONUS_FIX.
func hpBonusSC(l *sc.List, renewal bool) int {
	if l.Empty() {
		return 0
	}
	b := l.Val3(sc.SCPromoteHealthReserch)
	if l.Has(sc.SCMarionette) {
		b -= 1000
	}
	if renewal {
		b += l.Val1(sc.SCAngelus) * 50
	}
	return b
}

// hpRateSC is status_get_hpbonus with STATUS_BONUS_RATE.
func hpRateSC(l *sc.List) int {
	b := 0
	if !l.Empty() {
		b += l.Val1(sc.SCIncmhprate)
		b += l.Val2(sc.SCAppleidun)
		if l.Has(sc.SCBerserk) {
			b += 200
		}
		b += l.Val2(sc.SCDeluge)
		if l.Val2(sc.SCNibelungen) == ringHPRate {
			b += 30
		}
		if l.Has(sc.SCVenombleed) && l.Val3(sc.SCVenombleed) == 1 {
			b -= 15
		}
	}
	return max(b, -100)
}

// spRateSC is the status part of status_get_spbonus with
// STATUS_BONUS_RATE.
func spRateSC(l *sc.List) int {
	if l.Empty() {
		return 0
	}
	return l.Val1(sc.SCIncmsprate) + l.Val2(sc.SCService4u)
}

// spItemSC is status_get_spbonus_item.
func spItemSC(l *sc.List) int {
	if l.Empty() {
		return 0
	}
	return max(l.Val3(sc.SCEnergyDrinkReserch), -100)
}

// calcMaxHPNoPC is status_calc_maxhp for other units than players.
func calcMaxHPNoPC(l *sc.List, maxHP int, renewal bool) int {
	v := int64(maxHP) + int64(hpBonusSC(l, renewal))
	if rate := 100 + hpRateSC(l); rate != 100 {
		v = v * int64(rate) / 100
	}
	return int(capValue64(v, 1, math.MaxUint32))
}

// calcMaxSPNoPC is status_calc_maxsp for other units than players.
func calcMaxSPNoPC(l *sc.List, maxSP int) int {
	v := int64(maxSP)
	if rate := 100 + spRateSC(l); rate != 100 {
		v = v * int64(rate) / 100
	}
	return int(capValue64(v, 1, math.MaxUint32))
}

func capValue64(v, lo, hi int64) int64 { return min(max(v, lo), hi) }

// capDefType caps DEF or MDEF to DEFTYPE_MIN and DEFTYPE_MAX, as
// pc.capDef.
func capDefType(v int, renewal bool) int {
	if renewal {
		return capValue(v, math.MinInt16, math.MaxInt16)
	}
	return int(int8(capValue(v, 0, math.MaxUint8))) // #nosec G115 -- defType is a signed char in pre-renewal.
}

// CalcMonster is status_calc_bl_main for a monster: its battle status
// from its base status and its statuses. A monster without statuses keeps
// its base status. renewal picks the formulas.
func CalcMonster(base *battle.Status, l *sc.List, renewal bool) battle.Status {
	s := *base
	if l.Empty() {
		return s
	}
	b := base
	s.Str = calcStr(l, b.Str)
	s.Agi = calcAgi(l, b.Agi)
	s.Vit = calcVit(l, b.Vit)
	s.Int = calcInt(l, b.Int)
	s.Dex = calcDex(l, b.Dex)
	s.Luk = calcLuk(l, b.Luk)
	if b.BAtk != 0 {
		s.BAtk = mobBaseAtk(&s, renewal) + b.BAtk - mobBaseAtk(b, renewal)
		s.BAtk = calcBatk(l, s.BAtk, renewal)
	}
	if renewal {
		if b.WAtk == 0 {
			s.WAtk = 0
			s.WAtk2 = calcWatkMob(l, b.WAtk2, renewal)
		} else {
			s.WAtk = calcWatkMob(l, b.WAtk, renewal)
		}
	} else {
		s.RHW.Atk = calcWatkMob(l, b.RHW.Atk, renewal)
		s.RHW.Atk2 = calcWatkMob(l, b.RHW.Atk2, renewal)
		if b.LHW.Atk != 0 {
			s.LHW.Atk = calcWatkMob(l, b.LHW.Atk, renewal)
			s.LHW.Atk2 = calcWatkMob(l, b.LHW.Atk2, renewal)
		}
	}
	if s.Dex == b.Dex && (!renewal || s.Luk == b.Luk) {
		s.Hit = calcHit(l, b.Hit, renewal)
	} else {
		d := s.Dex - b.Dex
		if renewal {
			d += s.Luk/3 - b.Luk/3
		}
		s.Hit = calcHit(l, b.Hit+d, renewal)
	}
	if s.Agi == b.Agi && (!renewal || s.Luk == b.Luk) {
		s.Flee = calcFlee(l, b.Flee, renewal)
	} else {
		d := s.Agi - b.Agi
		if renewal {
			d += s.Luk/5 - b.Luk/5
		}
		s.Flee = calcFlee(l, b.Flee+d, renewal)
	}
	capDef := func(v int) int { return capDefType(v, renewal) }
	s.Def = calcDef(l, b.Def, renewal, false, capDef)
	if s.Vit == b.Vit && (!renewal || s.Agi == b.Agi) {
		s.Def2 = calcDef2(l, b.Def2, renewal, s.Vit)
	} else if renewal {
		s.Def2 = calcDef2(l, b.Def2+int((float32(s.Vit)/2-float32(b.Vit)/2)+(float32(s.Agi)/5-float32(b.Agi)/5)), renewal, s.Vit)
	} else {
		s.Def2 = calcDef2(l, b.Def2+s.Vit-b.Vit, renewal, s.Vit)
	}
	s.MDef = calcMdef(l, b.MDef, renewal, capDef)
	if s.Int == b.Int && s.Vit == b.Vit && (!renewal || s.Dex == b.Dex) {
		s.MDef2 = calcMdef2(l, b.MDef2, renewal)
	} else if renewal {
		s.MDef2 = calcMdef2(l, b.MDef2+s.Int-b.Int+int((float32(s.Dex)/5-float32(b.Dex)/5)+(float32(s.Vit)/5-float32(b.Vit)/5)), renewal)
	} else {
		s.MDef2 = calcMdef2(l, b.MDef2+s.Int-b.Int+(s.Vit-b.Vit)/2, renewal)
	}
	s.Speed = calcSpeed(l, b.Speed, speedInput{})
	if b.Cri != 0 {
		if s.Luk == b.Luk {
			s.Cri = calcCritical(l, b.Cri, renewal)
		} else if renewal {
			s.Cri = calcCritical(l, b.Cri+3*(s.Luk-b.Luk), renewal)
		} else {
			s.Cri = calcCritical(l, b.Cri+(s.Luk-b.Luk)*10/3, renewal)
		}
	}
	if b.Flee2 != 0 {
		s.Flee2 = calcFlee2(l, b.Flee2+s.Luk-b.Luk)
	}
	s.RHW.Ele = calcAttackElement(l, b.RHW.Ele)
	s.LHW.Ele = calcAttackElement(l, b.LHW.Ele)
	s.DefEle = calcElement(l, b.DefEle)
	s.EleLv = calcElementLv(l, b.EleLv)
	s.MaxHP = calcMaxHPNoPC(l, b.MaxHP, renewal)
	s.MaxSP = calcMaxSPNoPC(l, b.MaxSP)
	if renewal {
		lv := b.Level
		mn := s.Int + lv + s.RHW.Matk*70/100
		mx := s.Int + lv + s.RHW.Matk*130/100
		if l.Has(sc.SCRecognizedspell) {
			mn = max(mn, mx)
		}
		// Renewal applies Mystical Amplification before the pseudo buffs.
		mn, mx = magicPowerMatk(l, mn, mx, true)
		mn, mx = calcPseudobuffMatk(l, mn, renewal), calcPseudobuffMatk(l, mx, renewal)
		s.MAtkMin = capValue(mn, 0, math.MaxUint16)
		s.MAtkMax = capValue(mx, 0, math.MaxUint16)
	} else {
		mn := s.Int + (s.Int/7)*(s.Int/7)
		mx := s.Int + (s.Int/5)*(s.Int/5)
		if l.Has(sc.SCRecognizedspell) {
			mn = max(mn, mx)
		}
		// Pre-renewal applies the pseudo buffs before Mystical
		// Amplification.
		mn, mx = calcPseudobuffMatk(l, mn, renewal), calcPseudobuffMatk(l, mx, renewal)
		mn, mx = magicPowerMatk(l, mn, mx, false)
		s.MAtkMin = capValue(mn, 0, math.MaxUint16)
		s.MAtkMax = capValue(mx, 0, math.MaxUint16)
	}
	// ASPD of other units.
	rate := calcAspdRate(l, 1000)
	amotion := b.AMotion * rate / 1000
	amotion = calcFixAspd(l, amotion)
	s.AMotion = capValue(amotion, maxASPDNoPC/amotionDividerNoPC, minASPD/amotionDividerNoPC)
	s.ADelay = capValue(b.ADelay*min(rate, 1000)/1000, amotionDividerNoPC*s.AMotion, minASPD)
	s.Changes = BattleChanges(l)
	return s
}

// calcWatkMob is status_calc_watk for a monster: Volcano adds to a
// monster's weapon ATK only, then the statuses of calcWatk.
func calcWatkMob(l *sc.List, v int, renewal bool) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val2(sc.SCVolcano)
	return calcWatk(l, v, renewal, true)
}

// mobBaseAtk is status_base_atk for a monster: renewal's STR plus level,
// or pre-renewal's formula.
func mobBaseAtk(st *battle.Status, renewal bool) int {
	if renewal {
		return capValue(st.Str+st.Level, 0, math.MaxUint16)
	}
	dstr := st.Str / 10
	return capValue(st.Str+dstr*dstr+st.Dex/5+st.Luk/5, 0, math.MaxUint16)
}

// BattleChanges are what attacks read of a unit's statuses
// (battle.Changes), or nil for a unit without statuses.
func BattleChanges(l *sc.List) *battle.Changes {
	if l.Empty() {
		return nil
	}
	c := &battle.Changes{Opt1: l.Opt1, Sleep: l.Has(sc.SCSleep), AtkPercent: 100, DefRate: l.Val1(sc.SCDefRate),
		Assumptio: l.Val1(sc.SCAssumptio), Basilica: l.Has(sc.SCBasilica)}
	// battle_get_atkpercent.
	if l.Has(sc.SCCurse) {
		c.AtkPercent -= 25
	}
	c.AtkPercent += l.Val2(sc.SCProvoke)
	// Divest Weapon lowers the ATK of other units than players: a
	// player's SC_STRIPWEAPON has val2 0.
	c.AtkPercent -= l.Val2(sc.SCStripweapon)
	c.AtkPercent += l.Val2(sc.SCConcentration)
	if l.Val2(sc.SCJointbeat)&sc.BreakWaist != 0 {
		c.AtkPercent -= 25
	}
	c.AtkPercent += 2 * l.Val1(sc.SCTruesight)
	c.AtkPercent += l.Val1(sc.SCIncatkrate)
	c.Berserk = l.Has(sc.SCBerserk)
	c.AuraBlade = l.Val1(sc.SCAurablade)
	c.JointBeat = l.Val2(sc.SCJointbeat)
	c.Defender = l.Val2(sc.SCDefender)
	// Magnum Break's fire bonus: SC_WATK_ELEMENT in pre-renewal and
	// SC_SUB_WEAPONPROPERTY in renewal.
	if e := l.Get(sc.SCWatkElement); e != nil {
		c.WatkElement, c.WatkElementRate = e.Val1, e.Val2
	}
	if e := l.Get(sc.SCSubWeaponPro); e != nil {
		c.SubWeaponElement, c.SubWeaponRate = e.Val1, e.Val2
	}
	// Enchant Deadly Poison, and Poison React in its damage boost mode.
	if e := l.Get(sc.SCEdp); e != nil {
		c.EDP, c.EDPRate = e.Val1, e.Val3
	}
	if e := l.Get(sc.SCPoisonreact); e != nil && e.Val4 == 1 {
		c.PoisonReact = e.Val1
	}
	c.SteelBody = l.Has(sc.SCSteelbody)
	c.BladeStop = l.Has(sc.SCBladestop)
	// The element bonus of the ground field the unit stands on, and
	// Blinding Mist.
	c.VolcanoRate = l.Val3(sc.SCVolcano)
	c.DelugeRate = l.Val3(sc.SCDeluge)
	c.ViolentGaleRate = l.Val3(sc.SCViolentgale)
	c.FogWall = l.Has(sc.SCFogwall)
	// Power Thrust and Maximum Power Thrust in
	// battle_calc_attack_skill_ratio, Maximize Power in
	// battle_calc_base_damage, and Weapon Perfection's size penalty.
	c.Overthrust = l.Val3(sc.SCOverthrust) + l.Val2(sc.SCMaxoverthrust)
	c.MaximizePower = l.Has(sc.SCMaximizepower)
	c.WeaponPerfection = l.Has(sc.SCWeaponperfection)
	c.DrumBattle, c.Nibelungen = l.Val2(sc.SCDrumbattle), l.Val2(sc.SCNibelungen)
	return c
}

// magicPowerMatk is SC_MAGICPOWER's part of status_calc_bl_main: MATK
// rises by val3 percent. In pre-renewal only while a spell runs (val4).
func magicPowerMatk(l *sc.List, mn, mx int, renewal bool) (int, int) {
	e := l.Get(sc.SCMagicpower)
	if e == nil || !renewal && e.Val4 == 0 {
		return mn, mx
	}
	return mn + mn*e.Val3/100, mx + mx*e.Val3/100
}
