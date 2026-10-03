package status

import "fmt"

// Status points: raising STR to LUK, as rAthena's pc_statusup. A stat
// costs status points by its current value, with PC_STATUS_POINT_COST,
// and stops at the job's maximum from pc_maxparameter.

// Stat is one of the six stats a player raises, in rAthena's SP_STR to
// SP_LUK order.
type Stat int

// The stats a player raises.
const (
	Str Stat = iota
	Agi
	Vit
	Int
	Dex
	Luk
)

// The maximum of each raised stat by kind of job, from rAthena's
// conf/battle/player.conf.
const (
	maxParameter          = 99  // max_parameter
	maxTransParameter     = 99  // max_trans_parameter
	maxThirdParameter     = 130 // max_third_parameter
	maxThirdTransParam    = 130 // max_third_trans_parameter
	maxBabyParameter      = 80  // max_baby_parameter
	maxBabyThirdParameter = 117 // max_baby_third_parameter
	maxExtendedParameter  = 130 // max_extended_parameter
	maxSummonerParameter  = 130 // max_summoner_parameter
	maxFourthParameter    = 130 // max_fourth_parameter
)

// Map job IDs of pc_is_upper_expanded_second, from rAthena's e_mapid.
const (
	mapidSpiritHandler    = joblSecond1 | 0xa
	mapidNightWatch       = joblThird | joblSecond1 | 0x8
	mapidShinkiroShiranui = joblThird | joblSecond1 | 0x9
	mapidHyperNovice      = 0x10000 | joblThird | joblSecond1
	mapidSkyEmperor       = 0x10000 | joblThird | joblSecond1 | 0x7
	mapidSoulAscetic      = 0x10000 | joblThird | joblSecond2 | 0x7
)

// isUpperExpandedSecond is pc_is_upper_expanded_second.
func isUpperExpandedSecond(mapID uint64) bool {
	return mapID&mapidFourthMask == mapidHyperNovice || mapID&mapidFourthMask == mapidSkyEmperor ||
		mapID&mapidFourthMask == mapidSoulAscetic || mapID&mapidThirdMask == mapidNightWatch ||
		mapID&mapidThirdMask == mapidShinkiroShiranui || mapID&mapidSecondMask == mapidSpiritHandler
}

// isTraitJob is pc_is_trait_job.
func isTraitJob(mapID uint64) bool { return isPrimaryFourth(mapID) || isUpperExpandedSecond(mapID) }

// maxParam is the default max_param of STR to LUK that
// JobDatabase::loadingFinished gives a job. rAthena's job_stats.yml sets
// no MaxStats, so every job has the default.
func maxParam(mapID uint64) int {
	switch {
	case mapID&joblBaby != 0 && mapID&joblThird != 0:
		return maxBabyThirdParameter
	case mapID&joblBaby != 0:
		return maxBabyParameter
	case mapID&mapidFirstMask == mapidSummoner:
		return maxSummonerParameter
	case mapID&mapidSecondMask == mapidKagerouOboro || mapID&mapidSecondMask == mapidRebellion:
		return maxExtendedParameter
	case isTraitJob(mapID):
		return maxFourthParameter
	case mapID&joblThird != 0 && (mapID&joblUpper != 0 || isPrimaryFourth(mapID)):
		return maxThirdTransParam
	case mapID&joblThird != 0:
		return maxThirdParameter
	case mapID&joblUpper != 0:
		return maxTransParameter
	}
	return maxParameter
}

// statusPointCost is PC_STATUS_POINT_COST: the status points that raising
// a stat from low to low + 1 costs. Renewal is RENEWAL_STAT.
func statusPointCost(renewal bool, low int) int {
	if !renewal {
		return 1 + (low+9)/10
	}
	if low < 100 {
		return 2 + (low-1)/10
	}
	return 16 + 4*((low-100)/5)
}

// stat returns the raised value of a stat.
func (s *Stats) stat(st Stat) *int {
	switch st {
	case Str:
		return &s.Str
	case Agi:
		return &s.Agi
	case Vit:
		return &s.Vit
	case Int:
		return &s.Int
	case Dex:
		return &s.Dex
	case Luk:
		return &s.Luk
	}
	return nil
}

// MaxParameter is pc_maxparameter for STR to LUK: the highest value a
// character may raise a stat to. The job comes from pc_mapid2jobid, as
// for job bonuses.
func (t *Tables) MaxParameter(c *Character) (int, error) {
	j, err := t.job(c.Job)
	if err != nil {
		return 0, err
	}
	sj, ok := t.statsJob(j, c.Male)
	if !ok {
		return 0, fmt.Errorf("status: job %d has no map job ID", c.Job)
	}
	return sj.maxParam, nil
}

// NeedStatusPoint is pc_need_status_point: the status points to raise a
// stat by val. It is 0 when the stat is at its maximum, as official
// servers show. A negative val gives the points the last -val raises
// cost. A job without tables costs 0.
func (t *Tables) NeedStatusPoint(c *Character, st Stat, val int) int {
	v := c.Stats.stat(st)
	if val == 0 || v == nil {
		return 0
	}
	maxValue, err := t.MaxParameter(c)
	if err != nil {
		return 0
	}
	low := *v
	if low >= maxValue && val > 0 {
		return 0
	}
	high := low + val
	if val < 0 {
		low, high = high, low
	}
	sp := 0
	for ; low < high; low++ {
		sp += statusPointCost(t.renewal, low)
	}
	return sp
}

// maxParameterIncrease is pc_maxparameterincrease: how much the status
// points can raise a stat, up to its maximum.
func (t *Tables) maxParameterIncrease(c *Character, st Stat, points, maxValue int) int {
	base := *c.Stats.stat(st)
	final := base
	for final <= maxValue && points >= 0 {
		points -= statusPointCost(t.renewal, final)
		final++
	}
	final--
	return max(final-base, 0)
}

// StatusUp is pc_statusup: it raises a stat by up to increase, as far as
// the status points and the job's maximum allow, and takes the points
// from points. It returns false, changing nothing, when the stat cannot
// rise:
//   - The stat is not STR to LUK, or increase is not positive.
//   - The stat is at its maximum.
//   - The points do not pay for one more.
//
// The caller then computes the stats again, as status_calc_pc.
func (t *Tables) StatusUp(c *Character, points *int, st Stat, increase int) bool {
	v := c.Stats.stat(st)
	if v == nil || increase <= 0 {
		return false
	}
	maxValue, err := t.MaxParameter(c)
	if err != nil {
		return false
	}
	current := *v
	increase = min(max(increase, 0), t.maxParameterIncrease(c, st, *points, maxValue))
	if increase <= 0 || current+increase > maxValue {
		return false
	}
	need := t.NeedStatusPoint(c, st, increase)
	if need < 0 || need > *points {
		return false
	}
	*v = current + increase
	*points -= need
	return true
}
