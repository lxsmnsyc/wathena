package status

import (
	"fmt"
	"math"
)

// Job level flags and masks of rAthena's map job IDs (e_mapid), from
// map.hpp.
const (
	joblSecond1 = 0x100
	joblSecond2 = 0x200
	joblSecond  = joblSecond1 | joblSecond2
	joblThird   = 0x1000
	joblUpper   = 0x100000
	joblBaby    = 0x200000

	mapidFirstMask  = 0xff
	mapidSecondMask = 0xfff
	mapidThirdMask  = 0xffff
	mapidFourthMask = 0xfffff
)

// Map job IDs this package reads, from rAthena's e_mapid.
const (
	mapidNovice       = 0x0
	mapidThief        = 0x6
	mapidTaekwon      = 0x7
	mapidNinja        = 0x9
	mapidGunslinger   = 0x8
	mapidSummoner     = 0xa
	mapidSuperNovice  = joblSecond1 | mapidNovice
	mapidAssassin     = joblSecond1 | mapidThief
	mapidRebellion    = joblSecond1 | mapidGunslinger
	mapidKagerouOboro = joblSecond1 | mapidNinja
	mapidStarEmperor  = joblThird | joblSecond1 | mapidTaekwon

	// The 4th job ranges of pc_is_primary_fourth.
	mapidDragonKnight  = 0x10000 | joblThird | joblSecond1 | 0x1
	mapidShadowCross   = 0x10000 | joblThird | joblSecond1 | 0x6
	mapidImperialGuard = 0x10000 | joblThird | joblSecond2 | 0x1
	mapidAbyssChaser   = 0x10000 | joblThird | joblSecond2 | 0x6
)

// JobData is one job in job_stats.json: rAthena's job_db entry as the
// database files write it, merged in load order.
type JobData struct {
	ID           int   `json:"id"`
	MapID        int64 `json:"mapid"`
	MaxWeight    int   `json:"max_weight"`
	HPFactor     int   `json:"hp_factor"`
	HPIncrease   int   `json:"hp_increase"`
	SPFactor     int   `json:"sp_factor"`
	SPIncrease   int   `json:"sp_increase"`
	MaxBaseLevel int   `json:"max_base_level"`
	MaxJobLevel  int   `json:"max_job_level"`
	// ASPDBase is indexed by weapon type. Renewal has one more column for
	// shields and holds ASPD. Pre-renewal holds amotion.
	ASPDBase   []int `json:"aspd_base"`
	BonusStats []struct {
		Level int `json:"level"`
		Str   int `json:"str"`
		Agi   int `json:"agi"`
		Vit   int `json:"vit"`
		Int   int `json:"int"`
		Dex   int `json:"dex"`
		Luk   int `json:"luk"`
		Pow   int `json:"pow"`
		Sta   int `json:"sta"`
		Wis   int `json:"wis"`
		Spl   int `json:"spl"`
		Con   int `json:"con"`
		Crt   int `json:"crt"`
	} `json:"bonus_stats"`
	BaseHP []struct {
		Level int `json:"level"`
		HP    int `json:"hp"`
	} `json:"base_hp"`
	BaseSP []struct {
		Level int `json:"level"`
		SP    int `json:"sp"`
	} `json:"base_sp"`
}

// Data is job_stats.json, which reference/rathena extracted from
// rAthena's job database.
type Data struct {
	Mode string    `json:"mode"`
	Jobs []JobData `json:"jobs"`
	// MapIDJobs is pc_mapid2jobid: the job ID of a map job ID, by sex.
	MapIDJobs []struct {
		MapID  int64 `json:"mapid"`
		Male   int   `json:"male"`
		Female int   `json:"female"`
	} `json:"mapid_jobs"`
}

// traits are the six main stats and the six trait stats, in rAthena's
// PARAM_* order.
type traits [12]int

const (
	paramStr = iota
	paramAgi
	paramVit
	paramInt
	paramDex
	paramLuk
	paramPow
	paramSta
	paramWis
	paramSpl
	paramCon
	paramCrt
)

// job is one job after JobDatabase::loadingFinished.
type job struct {
	id           int
	mapID        uint64
	maxWeight    int
	maxBaseLevel int
	maxJobLevel  int
	aspdBase     []int
	// bonus is the total job bonus at each job level.
	bonus  []traits
	baseHP []uint32
	baseSP []uint32
}

// newJob builds a job's tables as JobDatabase::loadingFinished: missing
// HP and SP use calc_basehp and calc_basesp, and job bonuses become
// totals up to each job level.
func newJob(d *JobData, renewal bool) *job {
	j := &job{
		id:           d.ID,
		mapID:        uint64(d.MapID), // #nosec G115 -- -1 stays all bits set, as rAthena's uint64 cast.
		maxWeight:    d.MaxWeight,
		maxBaseLevel: d.MaxBaseLevel,
		maxJobLevel:  d.MaxJobLevel,
		aspdBase:     d.ASPDBase,
	}
	j.baseHP = make([]uint32, j.maxBaseLevel)
	for _, r := range d.BaseHP {
		if r.Level >= 1 && r.Level <= j.maxBaseLevel {
			j.baseHP[r.Level-1] = uint32(r.HP) // #nosec G115 -- HP tables are positive.
		}
	}
	j.baseSP = make([]uint32, j.maxBaseLevel)
	for _, r := range d.BaseSP {
		if r.Level >= 1 && r.Level <= j.maxBaseLevel {
			j.baseSP[r.Level-1] = uint32(r.SP) // #nosec G115 -- SP tables are positive.
		}
	}
	for lv := 1; lv <= j.maxBaseLevel; lv++ {
		if j.baseHP[lv-1] == 0 {
			j.baseHP[lv-1] = calcBaseHP(lv, d, j.mapID, renewal)
		}
		if j.baseSP[lv-1] == 0 {
			j.baseSP[lv-1] = calcBaseSP(lv, d, j.mapID)
		}
	}
	perLevel := make([]traits, j.maxJobLevel)
	for _, b := range d.BonusStats {
		if b.Level >= 1 && b.Level <= j.maxJobLevel {
			perLevel[b.Level-1] = traits{b.Str, b.Agi, b.Vit, b.Int, b.Dex, b.Luk, b.Pow, b.Sta, b.Wis, b.Spl, b.Con, b.Crt}
		}
	}
	j.bonus = make([]traits, j.maxJobLevel)
	var total traits
	for lv := range perLevel {
		for p := range total {
			total[p] += perLevel[lv][p]
		}
		j.bonus[lv] = total
	}
	return j
}

// calcBaseHP is JobDatabase::calc_basehp.
func calcBaseHP(level int, d *JobData, mapID uint64, renewal bool) uint32 {
	hp := 35.0
	hp += math.Floor(float64(level) * (float64(d.HPIncrease) / 100))
	for i := 2; i <= level; i++ {
		// The reference was recorded on arm64, where the compiler fuses
		// a multiply and an add in one expression. math.FMA does the same
		// on every platform, so results do not depend on where the
		// server runs. An x86 build of rAthena does not fuse them.
		hp += math.Floor(math.FMA(float64(d.HPFactor)/100, float64(i), 0.5))
	}
	if !renewal {
		if first := mapID & mapidFirstMask; (first == mapidNinja || first == mapidGunslinger) && level >= 10 {
			hp += 90
		}
	}
	switch {
	case mapID&mapidFirstMask == mapidSummoner:
		hp += math.Floor(hp/2 + 0.5)
	case mapID&mapidSecondMask == mapidSuperNovice:
		if level >= 99 {
			hp += 2000
		}
		if level >= 150 {
			hp += 2000
		}
	}
	return uint32(hp)
}

// calcBaseSP is JobDatabase::calc_basesp.
func calcBaseSP(level int, d *JobData, mapID uint64) uint32 {
	sp := 10.0
	sp += math.Floor(float64(level) * (float64(d.SPIncrease) / 100))
	for i := 2; i <= level; i++ {
		// Fused as in calcBaseHP.
		sp += math.Floor(math.FMA(float64(d.SPFactor)/100, float64(i), 0.5))
	}
	switch mapID & mapidFirstMask {
	case mapidNinja:
		if level >= 10 {
			sp -= 22
		} else {
			sp = 11 + 3*float64(level)
		}
	case mapidGunslinger:
		if level >= 10 {
			sp -= 18
		} else {
			sp = 9 + 3*float64(level)
		}
	case mapidSummoner:
		sp += math.Floor(sp/2 + 0.5)
	}
	return uint32(sp)
}

// isPrimaryFourth is pc_is_primary_fourth.
func isPrimaryFourth(mapID uint64) bool {
	m := mapID & mapidFourthMask
	return m >= mapidDragonKnight && m <= mapidShadowCross || m >= mapidImperialGuard && m <= mapidAbyssChaser
}

// job returns a job by ID.
func (t *Tables) job(id int) (*job, error) {
	j := t.jobs[id]
	if j == nil {
		return nil, fmt.Errorf("status: job %d is not in the job tables", id)
	}
	return j, nil
}

// mapIDToJob is pc_mapid2jobid.
func (t *Tables) mapIDToJob(mapID uint64, male bool) (int, bool) {
	s, ok := t.mapIDJobs[mapID]
	if !ok {
		return 0, false
	}
	if male {
		return s[1], true
	}
	return s[0], true
}
