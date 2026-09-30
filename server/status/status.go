// Package status computes a player's stats as rAthena's status.cpp does,
// so results match the recorded reference in
// reference/rathena/<mode>/pc_status.json.
//
// This is a port of rAthena's status.cpp and parts of pc.cpp (GPL-3.0).
// It covers a player with no equipment, no status changes, no mount and
// no cart, in both renewal and pre-renewal. Function comments name the
// rAthena functions they port. The settings are the defaults from
// rAthena's conf/battle files.
//
// What it does not port yet:
//   - Equipment, cards, item bonuses and status changes.
//   - Passive skills that need a weapon or shield, such as Snake Eye or
//     Mace Mastery's critical bonus, since the player is bare-handed.
//   - Summoners (pc_checkskill_summoner and Soul Attack's range), which
//     Calc refuses.
//   - Taekwon rankers, which need the fame list. Calc treats every
//     Taekwon as unranked, as rAthena does for a character not on it.
package status

import (
	"fmt"
	"math"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// Settings from rAthena's conf/battle files and headers.
const (
	// maxHPLv99, maxHPLv150 and maxHP cap max HP by base level
	// (max_hp_lv99, max_hp_lv150, max_hp).
	maxHPLv99  = 330000
	maxHPLv150 = 660000
	maxHP      = 1100000
	// maxSP is max_sp.
	maxSP = 1000000
	// maxASPD, maxThirdASPD, maxExtendedASPD and maxSummonerASPD are
	// max_aspd and its variants, after battle_adjust_conf turns them into
	// the shortest attack delay: (2000 - aspd * 10) * 2.
	maxASPD         = (amotionZeroASPD - 190*amotionInterval) * amotionDividerPC
	maxThirdASPD    = (amotionZeroASPD - 193*amotionInterval) * amotionDividerPC
	maxExtendedASPD = (amotionZeroASPD - 193*amotionInterval) * amotionDividerPC
	maxSummonerASPD = (amotionZeroASPD - 193*amotionInterval) * amotionDividerPC
	// maxWalkSpeed is max_walk_speed (300) after battle_adjust_conf:
	// 100 * DEFAULT_WALK_SPEED / 300.
	maxWalkSpeed = 100 * defaultWalkSpeed / 300

	defaultWalkSpeed = 150
	minWalkSpeed     = 20
	maxWalkSpeedCap  = 1000
	// minASPD is MIN_ASPD, the longest attack delay.
	minASPD          = 8000
	amotionDividerPC = 2
	amotionZeroASPD  = 2000
	amotionInterval  = 10

	// playerMode is the mode status_calc_pc_sub gives players: MD_MASK
	// without MD_ANGRY and MD_TARGETWEAK. The other excluded bits are
	// outside MD_MASK.
	playerMode = 0xffff &^ (0x800 | 0x4000)

	sizeSmall  = 0
	sizeMedium = 1
	// racePlayerHuman is RC_PLAYER_HUMAN.
	racePlayerHuman = 10

	// weaponTypes is MAX_WEAPON_TYPE. Renewal's ASPD table has a shield
	// column after it.
	weaponTypes = 24
	weaponFist  = 0
)

// Skill IDs this package reads, from rAthena's skill_db.yml.
const (
	skillIncCarry          = 36   // MC_INCCARRY
	skillOwlEye            = 43   // AC_OWL
	skillVultureEye        = 44   // AC_VULTURE
	skillMiss              = 49   // TF_MISS
	skillHiltBinding       = 105  // BS_HILTBINDING
	skillWeaponResearch    = 107  // BS_WEAPONRESEARCH
	skillTrust             = 248  // CR_TRUST
	skillDodge             = 265  // MO_DODGE
	skillDragonology       = 284  // SA_DRAGONOLOGY
	skillMusicalLesson     = 315  // BA_MUSICALLESSON
	skillDancingLesson     = 323  // DC_DANCINGLESSON
	skillMeditatio         = 363  // HP_MEDITATIO
	skillSoulDrain         = 364  // HW_SOULDRAIN
	skillDevil             = 441  // SG_DEVIL
	skillKaina             = 466  // SL_KAINA
	skillAllIncCarry       = 681  // ALL_INCCARRY
	skillResearchTrap      = 2248 // RA_RESEARCHTRAP
	skillLesson            = 2412 // WM_LESSON
	skillBreakthrough      = 5075 // NV_BREAKTHROUGH
	skillTranscendence     = 5077 // NV_TRANSCENDENCE
	skillShadowSense       = 5288 // SHC_SHADOW_SENSE
	skillGrenadeMastery    = 5402 // NW_GRENADE_MASTERY
	skillTalismanMastery   = 5416 // SOA_TALISMAN_MASTERY
	skillSoulMastery       = 5417 // SOA_SOUL_MASTERY
	skillMysticalCreatureM = 5433 // SH_MYSTICAL_CREATURE_MASTERY
	skillSelfStudyTactics  = 5449 // HN_SELFSTUDY_TATICS
	skillSelfStudySorcery  = 5450 // HN_SELFSTUDY_SOCERY
)

// Stats are the six stats a player raised with status points, as
// rAthena's mmo_charstatus str to luk. They do not include job bonuses.
type Stats struct {
	Str, Agi, Vit, Int, Dex, Luk int
}

// Character is what a player's stats come from.
type Character struct {
	// Job is the job ID, as rAthena's e_job.
	Job int
	// Male is the character's sex. It picks the job whose bonus stats and
	// weight apply to Bard and Dancer style jobs, as pc_mapid2jobid.
	Male                bool
	BaseLevel, JobLevel int
	Stats               Stats
	// Skills maps a learned skill ID to its level, as pc_checkskill
	// reads.
	Skills map[int]int
	// Deaths is die_counter. A Super Novice who never died gets +10 to
	// every stat.
	Deaths int
}

// Result is a player's stats.
type Result struct {
	// Base is base_status: stats before status changes.
	Base battle.Status
	// Battle is battle_status, what attacks use. Its Player is set, for a
	// bare-handed player with the character's skills.
	Battle battle.Status
	// MaxWeight is max_weight, in tenths of a weight unit.
	MaxWeight int
	// Window is what rAthena's status window shows.
	Window Window
}

// Window holds the two numbers rAthena's status window shows for ATK,
// MATK, DEF and MDEF, from the pc_leftside_* and pc_rightside_* macros
// that clif_updatestatus sends. In renewal the window shows them as
// "left + right". In pre-renewal it shows ATK, DEF and MDEF as
// "left + right" and MATK as the range "left ~ right".
type Window struct {
	ATK, ATK2   int
	MATK, MATK2 int
	DEF, DEF2   int
	MDEF, MDEF2 int
}

// Tables are the job tables of one rAthena mode.
type Tables struct {
	renewal   bool
	jobs      map[int]*job
	mapIDJobs map[uint64][2]int
}

// New builds the tables from job_stats.json.
func New(d *Data) (*Tables, error) {
	var renewal bool
	switch d.Mode {
	case "renewal":
		renewal = true
	case "pre-renewal":
	default:
		return nil, fmt.Errorf("status: unknown mode %q", d.Mode)
	}
	t := &Tables{renewal: renewal, jobs: map[int]*job{}, mapIDJobs: map[uint64][2]int{}}
	for i := range d.Jobs {
		t.jobs[d.Jobs[i].ID] = newJob(&d.Jobs[i], renewal)
	}
	for _, m := range d.MapIDJobs {
		t.mapIDJobs[uint64(m.MapID)] = [2]int{m.Female, m.Male} // #nosec G115 -- map job IDs are positive.
	}
	return t, nil
}

// Renewal reports whether the tables are renewal's.
func (t *Tables) Renewal() bool { return t.renewal }

// data is rAthena's status_data with the fields battle.Status lacks.
type data struct {
	battle.Status
	pow, sta, wis, spl, con, crt int
	patk, smatk, hplus, crate    int
	aspdRate                     int
}

// pc is the part of map_session_data this package reads.
type pc struct {
	t     *Tables
	c     *Character
	job   *job
	mapID uint64
	// statsJob is job_db.find(pc_mapid2jobid(class_, sex)), which gives
	// job bonuses and weight.
	statsJob *job
}

func (p *pc) skill(id int) int { return p.c.Skills[id] }

// Calc is status_calc_pc with SCO_FORCE, for a player with no equipment:
// status_calc_pc_sub fills the base status, then status_calc_bl_main
// makes the battle status with no status changes.
func (t *Tables) Calc(c Character) (Result, error) {
	j, err := t.job(c.Job)
	if err != nil {
		return Result{}, err
	}
	p := &pc{t: t, c: &c, job: j, mapID: j.mapID}
	if p.mapID&mapidFirstMask == mapidSummoner {
		return Result{}, fmt.Errorf("status: job %d is a summoner, which is not ported", c.Job)
	}
	statsID, ok := t.mapIDToJob(p.mapID, c.Male)
	if !ok {
		return Result{}, fmt.Errorf("status: job %d has no map job ID", c.Job)
	}
	if p.statsJob, err = t.job(statsID); err != nil {
		return Result{}, err
	}
	if c.BaseLevel < 1 || c.BaseLevel > len(j.baseHP) || c.JobLevel < 1 || c.JobLevel > len(p.statsJob.bonus) {
		return Result{}, fmt.Errorf("status: job %d cannot have base level %d and job level %d", c.Job, c.BaseLevel, c.JobLevel)
	}
	base := p.calcBase()
	bat := p.calcBattle(&base)
	res := Result{Base: base.Status, Battle: bat.Status, MaxWeight: p.maxWeight(), Window: p.window(&bat)}
	res.Base.Level, res.Battle.Level = c.BaseLevel, c.BaseLevel
	res.Battle.Player = &battle.Player{Skills: c.Skills}
	return res, nil
}

// calcBase is the part of status_calc_pc_sub that a player with no
// equipment and no status changes reaches.
func (p *pc) calcBase() data {
	var b data
	c := p.c
	b.Speed = defaultWalkSpeed
	b.Mode = playerMode
	b.Size = sizeMedium
	if p.mapID&joblBaby != 0 {
		b.Size = sizeSmall
	}
	b.aspdRate = 1000
	b.EleLv = 1
	b.Race = racePlayerHuman
	// rhw.range and lhw.range become at least 1.
	b.RHW.Range, b.LHW.Range = 1, 1

	// Job bonuses.
	bonus := p.statsJob.bonus[c.JobLevel-1]
	b.Str, b.Agi, b.Vit, b.Int, b.Dex, b.Luk = bonus[paramStr], bonus[paramAgi], bonus[paramVit], bonus[paramInt], bonus[paramDex], bonus[paramLuk]
	b.pow, b.sta, b.wis, b.spl, b.con, b.crt = bonus[paramPow], bonus[paramSta], bonus[paramWis], bonus[paramSpl], bonus[paramCon], bonus[paramCrt]

	// A Super Novice who never died, at job level 70 or a third job.
	if p.mapID&mapidSecondMask == mapidSuperNovice && (c.JobLevel >= 70 || p.mapID&joblThird != 0) && c.Deaths == 0 {
		b.Str += 10
		b.Agi += 10
		b.Vit += 10
		b.Int += 10
		b.Dex += 10
		b.Luk += 10
	}

	// Absolute modifiers from passive skills.
	if p.skill(skillHiltBinding) > 0 {
		b.Str++
	}
	if lv := p.skill(skillDragonology); lv > 0 {
		b.Int += (lv + 1) / 2
	}
	b.Dex += p.skill(skillOwlEye)
	b.Int += p.skill(skillResearchTrap)

	// Raised stats, capped to uint16.
	b.Str = capValue(b.Str+c.Stats.Str, 0, math.MaxUint16)
	b.Agi = capValue(b.Agi+c.Stats.Agi, 0, math.MaxUint16)
	b.Vit = capValue(b.Vit+c.Stats.Vit, 0, math.MaxUint16)
	b.Int = capValue(b.Int+c.Stats.Int, 0, math.MaxUint16)
	b.Dex = capValue(b.Dex+c.Stats.Dex, 0, math.MaxUint16)
	b.Luk = capValue(b.Luk+c.Stats.Luk, 0, math.MaxUint16)
	for _, v := range []*int{&b.pow, &b.sta, &b.wis, &b.spl, &b.con, &b.crt} {
		*v = capValue(*v, 0, math.MaxUint16)
	}

	b.con += p.skill(skillGrenadeMastery)
	b.spl += p.skill(skillSoulMastery)

	// Attack. Renewal's weapon ATK is 0 without a weapon.
	if !p.t.renewal && p.skill(skillHiltBinding) > 0 {
		b.BAtk += 4
	}

	b.MaxHP = p.capMaxHP(p.calcMaxHP(b.Vit))
	b.MaxSP = capValue(p.calcMaxSP(b.Int), 1, maxSP)

	p.calcMisc(&b)

	// Hit.
	if !p.t.renewal {
		b.Hit += p.skill(skillWeaponResearch) * 2
		b.Hit += p.skill(skillVultureEye)
	}

	// Flee.
	if lv := p.skill(skillMiss); lv > 0 {
		if p.mapID&joblSecond != 0 && p.mapID&mapidFirstMask == mapidThief {
			b.Flee += lv * 4
		} else {
			b.Flee += lv * 3
		}
	}
	b.Flee += p.skill(skillDodge) * 3 / 2
	b.Flee += p.skill(skillShadowSense) * 10

	// Critical.
	if p.t.renewal {
		b.Cri += p.skill(skillDancingLesson) * 10
	}

	// P.Atk and S.Matk.
	b.patk += p.skill(skillSelfStudyTactics)
	b.smatk += p.skill(skillSelfStudySorcery)
	b.smatk += p.skill(skillTalismanMastery)
	if lv := p.skill(skillMysticalCreatureM); lv > 0 {
		b.smatk += lv * 15 / 10
		b.patk += lv * 15 / 10
	}

	// ASPD.
	i := p.baseAmotion(&b)
	if p.t.renewal {
		// Renewal's base value is ASPD, not amotion.
		i = amotionZeroASPD - i*amotionInterval
	}
	b.AMotion = capValue(i, p.maxASPD()/amotionDividerPC, minASPD/amotionDividerPC)
	if !p.t.renewal {
		if lv := p.skill(skillDevil); lv > 0 && (p.mapID&mapidThirdMask == mapidStarEmperor || c.JobLevel >= p.job.maxJobLevel) {
			b.aspdRate -= 30 * lv
		}
	}
	b.ADelay = amotionDividerPC * b.AMotion

	b.DMotion = capValue(800-b.Agi*4, 400, 800)
	return b
}

// calcMisc is status_calc_misc for a player.
func (p *pc) calcMisc(b *data) {
	level := p.c.BaseLevel
	if p.t.renewal {
		b.Hit = capValue(b.Hit+level+b.Dex+b.Luk/3+175+2*b.con, 1, math.MaxInt16)
		b.Flee = capValue(b.Flee+level+b.Agi+b.Luk/5+100+2*b.con, 1, math.MaxInt16)
		b.Def2 = capValue(b.Def2+int((float32(level)+float32(b.Vit))/2+float32(b.Agi)/5), 0, math.MaxInt16)
		b.MDef2 = capValue(b.MDef2+int(float32(b.Int)+float32(level)/4+float32(b.Dex+b.Vit)/5), 0, math.MaxInt16)
		b.patk = capValue(b.patk+b.pow/3+b.con/5, 0, math.MaxInt16)
		b.smatk = capValue(b.smatk+b.spl/3+b.con/5, 0, math.MaxInt16)
		b.Res = capValue(b.Res+b.sta+b.sta/3*5, 0, math.MaxInt16)
		b.MRes = capValue(b.MRes+b.wis+b.wis/3*5, 0, math.MaxInt16)
		b.hplus = capValue(b.hplus+b.crt, 0, math.MaxInt16)
		b.crate = capValue(b.crate+b.crt/3, 0, math.MaxInt16)
		b.MAtkMin = p.baseMatk(b)
		b.MAtkMax = p.baseMatk(b)
	} else {
		b.MAtkMin = int(uint16(b.Int + (b.Int/7)*(b.Int/7))) // #nosec G115 -- status_base_matk_min returns uint16.
		b.MAtkMax = int(uint16(b.Int + (b.Int/5)*(b.Int/5))) // #nosec G115 -- status_base_matk_max returns uint16.
		b.Hit = capValue(b.Hit+level+b.Dex, 1, math.MaxInt16)
		b.Flee = capValue(b.Flee+level+b.Agi, 1, math.MaxInt16)
		b.Def2 = capValue(b.Def2+b.Vit, 0, math.MaxInt16)
		b.MDef2 = capValue(b.MDef2+b.Int+b.Vit/2, 0, math.MaxInt16)
	}
	// enable_critical and enable_perfect_flee include players.
	if p.t.renewal {
		b.Cri = capValue(b.Cri+level/10+10+b.Luk*3, 1, math.MaxInt16)
	} else {
		b.Cri = capValue(b.Cri+10+b.Luk*10/3, 1, math.MaxInt16)
	}
	b.Flee2 = capValue(b.Flee2+b.Luk+10, 0, math.MaxInt16)
	b.BAtk += p.baseAtk(b)
}

// baseMatk is status_base_matk_min and status_base_matk_max for a
// player in renewal. Both are the same.
func (p *pc) baseMatk(b *data) int {
	return int(uint16(b.Int + b.Int/2 + b.Dex/5 + b.Luk/3 + p.c.BaseLevel/4 + 5*b.spl)) // #nosec G115 -- rAthena returns uint16.
}

// baseAtk is status_base_atk for a bare-handed player.
func (p *pc) baseAtk(b *data) int {
	str, dex := b.Str, b.Dex
	var v int
	if p.t.renewal {
		v = (str*10+dex*10/5+b.Luk*10/3+p.c.BaseLevel*10/4)/10 + 5*b.pow
	} else {
		dstr := str / 10
		v = str + dstr*dstr + dex/5 + b.Luk/5
	}
	return capValue(v, 0, math.MaxUint16)
}

// baseAmotion is status_base_amotion_pc for a bare-handed player. In
// renewal it returns ASPD, and in pre-renewal amotion.
func (p *pc) baseAmotion(b *data) int {
	if p.t.renewal {
		aspd := p.job.aspdBase[weaponFist]
		temp := float32(b.Dex*b.Dex)/5 + float32(b.Agi*b.Agi)*0.5
		temp = float32(math.Sqrt(float64(temp)))*0.25 + 196
		val := 0
		if lv := p.skill(skillDevil); lv > 0 && (p.mapID&mapidThirdMask == mapidStarEmperor || p.c.JobLevel >= p.job.maxJobLevel) {
			val += 1 + lv
		}
		return int(temp+float32(val)*float32(b.Agi)/200) - min(aspd, 200)
	}
	amotion := p.job.aspdBase[weaponFist]
	amotion -= amotion * (4*b.Agi + b.Dex) / 1000
	return amotion
}

// maxASPD is pc_maxaspd.
func (p *pc) maxASPD() int {
	switch {
	case p.mapID&joblThird != 0:
		return maxThirdASPD
	case p.mapID&mapidSecondMask == mapidKagerouOboro || p.mapID&mapidSecondMask == mapidRebellion:
		return maxExtendedASPD
	case p.mapID&mapidFirstMask == mapidSummoner:
		return maxSummonerASPD
	}
	return maxASPD
}

// calcMaxHP is status_calc_maxhp_pc.
func (p *pc) calcMaxHP(vit int) int {
	dmax := float64(p.job.baseHP[max(p.c.BaseLevel, 1)-1])
	if vit > 0 {
		// Fused as the arm64 recording is. See calcBaseHP.
		dmax *= math.FMA(float64(vit), 0.01, 1.0)
	}
	if p.mapID&joblUpper != 0 || isPrimaryFourth(p.mapID) {
		dmax *= 1.25
	}
	dmax += float64(p.hpBonus())
	// status_get_hpbonus with STATUS_BONUS_RATE has only status changes.
	return max(int(uint32(max(dmax, 1))), 1)
}

// capMaxHP caps max HP by base level.
func (p *pc) capMaxHP(hp int) int {
	switch {
	case p.c.BaseLevel < 100:
		return capValue(hp, 1, maxHPLv99)
	case p.c.BaseLevel < 151:
		return capValue(hp, 1, maxHPLv150)
	}
	return capValue(hp, 1, maxHP)
}

// hpBonus is status_get_hpbonus with STATUS_BONUS_FIX, from skills.
func (p *pc) hpBonus() int {
	bonus := p.skill(skillTrust) * 200
	for _, id := range []int{skillBreakthrough, skillTranscendence} {
		if lv := p.skill(id); lv > 0 {
			bonus += 350 * lv
			if lv > 4 {
				bonus += 250
			}
		}
	}
	return bonus
}

// calcMaxSP is status_calc_maxsp_pc.
func (p *pc) calcMaxSP(intel int) int {
	dmax := float64(p.job.baseSP[max(p.c.BaseLevel, 1)-1])
	if intel > 0 {
		// Fused as in calcMaxHP.
		dmax *= math.FMA(float64(intel), 0.01, 1.0)
	}
	if p.mapID&joblUpper != 0 || isPrimaryFourth(p.mapID) {
		dmax *= 1.25
	}
	dmax += float64(p.spBonus())
	// "Aegis accuracy": the rate bonus is truncated before it is added.
	dmax += float64(int64(dmax * float64(p.spRate()) / 100))
	return max(int(uint32(max(dmax, 1))), 1)
}

// spBonus is status_get_spbonus with STATUS_BONUS_FIX, from skills.
func (p *pc) spBonus() int {
	bonus := p.skill(skillKaina) * 30
	if lv := p.skill(skillResearchTrap); lv > 0 {
		bonus += 200 + 20*lv
	}
	bonus += p.skill(skillLesson) * 30
	for _, id := range []int{skillBreakthrough, skillTranscendence} {
		if lv := p.skill(id); lv > 0 {
			bonus += 30 * lv
			if lv > 4 {
				bonus += 50
			}
		}
	}
	return bonus
}

// spRate is status_get_spbonus with STATUS_BONUS_RATE, from skills.
func (p *pc) spRate() int {
	bonus := p.skill(skillMeditatio) + 2*p.skill(skillSoulDrain)
	if p.t.renewal {
		lesson := skillDancingLesson
		if p.c.Male {
			lesson = skillMusicalLesson
		}
		bonus += p.skill(lesson)
	}
	return max(bonus, -100)
}

// maxWeight is status_calc_weight with CALCWT_MAXBONUS. It reads the
// raised STR, not the total.
func (p *pc) maxWeight() int {
	w := p.statsJob.maxWeight + p.c.Stats.Str*300
	w += 2000 * p.skill(skillIncCarry)
	w += 2000 * p.skill(skillAllIncCarry)
	return w
}

// calcBattle is status_calc_bl_main with every flag and no status
// changes. Most values stay as in the base status.
func (p *pc) calcBattle(b *data) data {
	s := *b

	// status_calc_speed. A player's status change list is never nil, so
	// the haste from Improve Dodge applies even with no status changes.
	rate := 100
	if p.mapID&mapidSecondMask == mapidAssassin && p.skill(skillMiss) > 0 {
		rate -= p.skill(skillMiss)
	}
	rate = max(rate, 40)
	speed := b.Speed
	if rate != 100 {
		speed = speed * rate / 100
	}
	s.Speed = capValue(speed, minWalkSpeed, maxWalkSpeedCap)
	if s.Speed < maxWalkSpeed {
		s.Speed = maxWalkSpeed
	}

	// ASPD.
	amotion := p.baseAmotion(&s)
	amotion = amotion * s.aspdRate / 1000
	if p.t.renewal {
		// The renewal rate modifier adds nothing without status changes.
		amotion = amotionZeroASPD - amotion*amotionInterval
	}
	// status_calc_fix_aspd.
	amotion = capValue(amotion, 1, minASPD)
	s.AMotion = capValue(amotion, p.maxASPD()/amotionDividerPC, minASPD/amotionDividerPC)
	s.ADelay = amotionDividerPC * s.AMotion
	return s
}

// window is pc_leftside_atk to pc_rightside_matk, with the clamp that
// clif_updatestatus applies to pre-renewal's SP_MDEF2. Equipment and
// status change MATK are 0 here.
func (p *pc) window(s *data) Window {
	if p.t.renewal {
		return Window{
			ATK: s.BAtk, ATK2: s.WAtk + s.WAtk2 + s.EAtk,
			MATK: p.baseMatk(s), MATK2: s.RHW.Matk + s.LHW.Matk,
			DEF: s.Def2, DEF2: s.Def,
			MDEF: s.MDef2, MDEF2: s.MDef,
		}
	}
	return Window{
		ATK: s.BAtk + s.RHW.Atk + s.LHW.Atk, ATK2: s.RHW.Atk2 + s.LHW.Atk2,
		MATK: s.MAtkMin, MATK2: s.MAtkMax,
		DEF: s.Def, DEF2: s.Def2,
		MDEF: s.MDef, MDEF2: max(s.MDef2-s.Vit/2, 0),
	}
}

// capValue is rAthena's cap_value.
func capValue(v, lo, hi int) int { return min(max(v, lo), hi) }
