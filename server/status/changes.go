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

func capStat(v int) int { return capValue(v, 0, math.MaxUint16) }

// calcStr is status_calc_str.
func calcStr(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	if l.Has(sc.SCBlessing) {
		if b := l.Val2(sc.SCBlessing); b != 0 {
			v += b
		} else {
			v -= v / 2
		}
	}
	v -= l.Val1(sc.SCStomachache)
	return capStat(v)
}

// calcAgi is status_calc_agi.
func calcAgi(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	v += l.Val2(sc.SCIncreaseagi)
	v -= l.Val2(sc.SCDecreaseagi)
	v -= l.Val1(sc.SCStomachache)
	return capStat(v)
}

// calcVit is status_calc_vit.
func calcVit(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	v -= l.Val1(sc.SCStomachache)
	return capStat(v)
}

// calcInt is status_calc_int.
func calcInt(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	if l.Has(sc.SCBlessing) {
		if b := l.Val2(sc.SCBlessing); b != 0 {
			v += b
		} else {
			v -= v / 2
		}
	}
	v -= l.Val1(sc.SCStomachache)
	return capStat(v)
}

// calcDex is status_calc_dex.
func calcDex(l *sc.List, v int) int {
	if l.Empty() {
		return capStat(v)
	}
	v += l.Val1(sc.SCIncallstatus)
	if l.Has(sc.SCBlessing) {
		if b := l.Val2(sc.SCBlessing); b != 0 {
			v += b
		} else {
			v -= v / 2
		}
	}
	v -= l.Val1(sc.SCStomachache)
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
	v -= l.Val1(sc.SCStomachache)
	return capStat(v)
}

// calcCritical is status_calc_critical.
func calcCritical(l *sc.List, v int) int {
	if l.Empty() {
		return capValue(v, 10, math.MaxInt16)
	}
	if l.Has(sc.SCCloaking) {
		v += v
	}
	return capValue(v, 10, math.MaxInt16)
}

// calcHit is status_calc_hit.
func calcHit(l *sc.List, v int, renewal bool) int {
	if l.Empty() {
		return capValue(v, 1, math.MaxInt16)
	}
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
		if l.Has(sc.SCAdrenaline) {
			v += l.Val1(sc.SCAdrenaline)*3 + 5
		}
	}
	return capValue(v, 1, math.MaxInt16)
}

// calcFlee is status_calc_flee. The flee penalties of GvG and
// battleground maps wait for those maps.
func calcFlee(l *sc.List, v int) int {
	if l.Empty() {
		return capValue(v, 1, math.MaxInt16)
	}
	// Fixed values.
	v += l.Val1(sc.SCIncflee)
	v += l.Val2(sc.SCWindwalk)
	// Rates.
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
	return capValue(v, 10, math.MaxInt16)
}

// calcDef is status_calc_def. cap is DEFTYPE's range.
func calcDef(l *sc.List, v int, renewal, player bool, capDef func(int) int) int {
	if l.Empty() {
		return capDef(v)
	}
	if renewal && l.Has(sc.SCAssumptio) {
		v += l.Val1(sc.SCAssumptio) * 50
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
	return capValue(v, lo, math.MaxInt16)
}

// calcMdef is status_calc_mdef.
func calcMdef(l *sc.List, v int, capDef func(int) int) int {
	if l.Empty() {
		return capDef(v)
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
	return capValue(v, lo, math.MaxInt16)
}

// speedInput is what status_calc_speed reads of a player besides its
// statuses: the item speed bonuses and the skills.
type speedInput struct {
	player     bool
	itemSpeed  int
	assassinTF int
	tunnel     int
}

// calcSpeed is status_calc_speed, without mounts, carts and the speed
// changes of casting, which wait for them.
func calcSpeed(l *sc.List, speed int, in speedInput) int {
	rate := 100
	// GetMoveHasteValue2: mounts.
	val := 0
	rate -= val
	// GetMoveSlowValue.
	if in.player && l.Has(sc.SCHiding) && in.tunnel > 0 {
		val = 120 - 6*in.tunnel
	} else {
		val = 0
		if l.Has(sc.SCDecreaseagi) {
			val = max(val, 25)
		}
		if l.Has(sc.SCCurse) {
			val = max(val, 300)
		}
		if l.Has(sc.SCWedding) {
			val = max(val, 100)
		}
		if e := l.Get(sc.SCCloaking); e != nil && e.Val4&1 == 0 {
			if e.Val1 < 3 {
				val = max(val, 300)
			} else {
				val = max(val, 30-3*e.Val1)
			}
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
	// GetSpeed.
	if l.Has(sc.SCParalyse) && l.Val3(sc.SCParalyse) == 1 {
		speed += speed * 50 / 100
	}
	if rate != 100 {
		speed = speed * rate / 100
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
		if bonus < 7 && l.Has(sc.SCAdrenaline) {
			bonus = 7
		} else if bonus < 6 && l.Has(sc.SCAdrenaline2) {
			bonus = 6
		}
		for _, t := range []int{sc.SCAspdpotion3, sc.SCAspdpotion2, sc.SCAspdpotion1, sc.SCAspdpotion0} {
			if l.Has(t) {
				bonus += l.Val1(t)
				break
			}
		}
		return bonus
	}
	if l.Has(sc.SCParalyse) && l.Val3(sc.SCParalyse) == 1 {
		bonus -= 10
	}
	bonus += l.Val1(sc.SCIncreaseagi)
	if player && skill != nil {
		bonus += skill(skillMusicalLesson)
		bonus += skill(skillPlagiarism)
	}
	return bonus
}

// calcFixAspd is status_calc_fix_aspd.
func calcFixAspd(l *sc.List, aspd int) int {
	return capValue(aspd, 1, minASPD)
}

// calcAspdRate is status_calc_aspd_rate, which pre-renewal players and
// other units read.
func calcAspdRate(l *sc.List, rate int) int {
	if l.Empty() {
		return capValue(rate, 0, math.MaxInt16)
	}
	most := 0
	if l.Has(sc.SCAdrenaline2) && most < l.Val3(sc.SCAdrenaline2) {
		most = l.Val3(sc.SCAdrenaline2)
	}
	if l.Has(sc.SCAdrenaline) && most < l.Val3(sc.SCAdrenaline) {
		most = l.Val3(sc.SCAdrenaline)
	}
	rate -= most
	for _, t := range []int{sc.SCAspdpotion3, sc.SCAspdpotion2, sc.SCAspdpotion1, sc.SCAspdpotion0} {
		if l.Has(t) {
			rate -= l.Val2(t)
			break
		}
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
	return l.Val1(sc.SCIncmsprate)
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
	}
	if renewal {
		if b.WAtk == 0 {
			s.WAtk = 0
			s.WAtk2 = capStat(b.WAtk2)
		} else {
			s.WAtk = capStat(b.WAtk)
		}
	} else {
		s.RHW.Atk = capStat(b.RHW.Atk)
		s.RHW.Atk2 = capStat(b.RHW.Atk2)
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
		s.Flee = calcFlee(l, b.Flee)
	} else {
		d := s.Agi - b.Agi
		if renewal {
			d += s.Luk/5 - b.Luk/5
		}
		s.Flee = calcFlee(l, b.Flee+d)
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
	s.MDef = calcMdef(l, b.MDef, capDef)
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
			s.Cri = calcCritical(l, b.Cri)
		} else if renewal {
			s.Cri = calcCritical(l, b.Cri+3*(s.Luk-b.Luk))
		} else {
			s.Cri = calcCritical(l, b.Cri+(s.Luk-b.Luk)*10/3)
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
		s.MAtkMin = capValue(mn, 0, math.MaxUint16)
		s.MAtkMax = capValue(mx, 0, math.MaxUint16)
	} else {
		mn := s.Int + (s.Int/7)*(s.Int/7)
		mx := s.Int + (s.Int/5)*(s.Int/5)
		if l.Has(sc.SCRecognizedspell) {
			mn = max(mn, mx)
		}
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
	c := &battle.Changes{Opt1: l.Opt1, Sleep: l.Has(sc.SCSleep), AtkPercent: 100, DefRate: l.Val1(sc.SCDefRate)}
	// battle_get_atkpercent.
	if l.Has(sc.SCCurse) {
		c.AtkPercent -= 25
	}
	c.AtkPercent += l.Val2(sc.SCConcentration)
	c.AtkPercent += l.Val1(sc.SCIncatkrate)
	return c
}
