package status

import (
	"fmt"
	"maps"
	"slices"
)

// Job records (proposal 0002, review decision 24): what server/gamedata
// builds from core's and the mods' data/jobs.yml. Core's jobs keep
// rAthena's map job IDs. A mod job has its own ID from the mod range,
// and a map job ID made from its look, tier, transcendent and baby, so
// rAthena's rules that read the class treat it as that class.

// WithJobs returns tables with the jobs of a job list in place of the
// current ones: every job, core's included. The refine table, the status
// database and the ruleset settings stay. pc_mapid2jobid keeps core's
// jobs only, so a map job ID still names a core job.
func (t *Tables) WithJobs(jobs []JobData) *Tables {
	out := *t
	out.jobs = make(map[int]*job, len(jobs))
	out.byName = make(map[string]int, len(jobs))
	for i := range jobs {
		j := newJob(&jobs[i], t.renewal)
		out.jobs[j.id] = j
		if j.info.Name != "" {
			out.byName[j.info.Name] = j.id
		}
	}
	return &out
}

// Info returns a job's record fields.
func (t *Tables) Info(job int) (JobInfo, bool) {
	j := t.jobs[job]
	if j == nil {
		return JobInfo{}, false
	}
	return j.info, true
}

// JobID finds a job by its name: its name in wa.jobs, such as knight, or
// a mod job's full name.
func (t *Tables) JobID(name string) (int, bool) {
	id, ok := t.byName[name]
	return id, ok
}

// Jobs returns every job ID, in order.
func (t *Tables) Jobs() []int {
	return slices.Sorted(maps.Keys(t.jobs))
}

// IsMod reports whether a mod added the job.
func (t *Tables) IsMod(job int) bool {
	j := t.jobs[job]
	return j != nil && j.info.Mod != ""
}

// LookJob is the job whose sprites a job shows: its look, or itself.
func (t *Tables) LookJob(job int) int {
	if j := t.jobs[job]; j != nil && j.info.Name != "" {
		return j.info.Look
	}
	return job
}

// DisplayName is the name players see for a job, or "" for a job the
// tables lack.
func (t *Tables) DisplayName(job int) string {
	if j := t.jobs[job]; j != nil {
		return j.info.DisplayName
	}
	return ""
}

// EquipMapID is the map job ID that equipment checks read for a job:
// the one of its equips_as job.
func (t *Tables) EquipMapID(job int) (uint64, bool) {
	j := t.jobs[job]
	if j == nil {
		return 0, false
	}
	if e := t.jobs[j.info.EquipsAs]; e != nil && j.info.Name != "" {
		return e.mapID, true
	}
	return j.mapID, true
}

// BaseJobOf is rAthena's BaseJob of a job, from its record. ok is false
// for a job without a record.
func (t *Tables) BaseJobOf(job int) (int, bool) {
	j := t.jobs[job]
	if j == nil || j.info.Name == "" {
		return 0, false
	}
	return j.info.BaseJob, true
}

// NextJobs is jobs.next_jobs: the jobs whose changes_from lists a job,
// in job ID order.
func (t *Tables) NextJobs(job int) []int {
	var out []int
	for _, id := range t.Jobs() {
		for _, c := range t.jobs[id].info.ChangesFrom {
			if c.Job == job {
				out = append(out, id)
				break
			}
		}
	}
	return out
}

// ChangeFrom returns the entry of a job's changes_from for the job a
// player has.
func (t *Tables) ChangeFrom(to, from int) (JobChange, bool) {
	j := t.jobs[to]
	if j == nil {
		return JobChange{}, false
	}
	for _, c := range j.info.ChangesFrom {
		if c.Job == from {
			return c, true
		}
	}
	return JobChange{}, false
}

// statsJob is the job whose bonuses, weight and maximum stats a
// character uses: job_db.find(pc_mapid2jobid(class_, sex)) for core's
// jobs, such as a Knight for a mounted Knight, and the job itself for a
// mod job, whose map job ID names a core job.
func (t *Tables) statsJob(j *job, male bool) (*job, bool) {
	if j.info.Mod != "" {
		return j, true
	}
	id, ok := t.mapIDToJob(j.mapID, male)
	if !ok {
		return nil, false
	}
	sj := t.jobs[id]
	return sj, sj != nil
}

// Job tiers of data/jobs.yml.
const (
	TierNovice   = "novice"
	TierFirst    = "first"
	TierSecond   = "second"
	TierThird    = "third"
	TierFourth   = "fourth"
	TierExpanded = "expanded"
)

// JobTiers lists the tiers.
var JobTiers = []string{TierNovice, TierFirst, TierSecond, TierThird, TierFourth, TierExpanded}

// expandedLines are the first job lines of rAthena's expanded classes:
// Taekwon, Gunslinger, Ninja and Summoner.
var expandedLines = []uint64{mapidTaekwon, mapidGunslinger, mapidNinja, mapidSummoner}

// TierOf is a core job's tier, from its map job ID.
func TierOf(m uint64) string {
	m &^= joblUpper | joblBaby
	switch {
	case m == 0:
		return TierNovice
	case slices.Contains(expandedLines, m&mapidFirstMask) || m&mapidSecondMask == mapidSuperNovice:
		return TierExpanded
	case m&mapidFourth != 0:
		return TierFourth
	case m&joblThird != 0:
		return TierThird
	case m&joblSecond != 0:
		return TierSecond
	}
	return TierFirst
}

// mapidFourth is the 4th job bit of e_mapid.
const mapidFourth = 0x10000

// ModMapID is a mod job's map job ID: its look's class, changed to the
// job's tier, transcendent and baby.
func ModMapID(look uint64, tier string, transcendent, baby bool) uint64 {
	base := look &^ (joblUpper | joblBaby)
	second := base & joblSecond
	if second == 0 {
		second = joblSecond1
	}
	line := base & mapidFirstMask
	var m uint64
	switch tier {
	case TierOf(look), TierExpanded:
		m = base
	case TierNovice:
		m = mapidNovice
	case TierFirst:
		m = line
	case TierSecond:
		m = line | second
	case TierThird:
		m = line | second | joblThird
	case TierFourth:
		m = line | second | joblThird | mapidFourth
	}
	if transcendent {
		m |= joblUpper
	}
	if baby {
		m |= joblBaby
	}
	return m
}

// changeJobs are the jobs that rAthena's job change rules reach until
// third classes exist: the Novice, the first and second classes, the
// Super Novice, and their transcendent jobs. Expanded classes, baby jobs
// and mounted job IDs are left out.
var changeJobs = []int{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 15, 16, 17, 18, 19, 20, 23,
	4001, 4002, 4003, 4004, 4005, 4006, 4007, 4008, 4009, 4010, 4011, 4012, 4013,
	4015, 4016, 4017, 4018, 4019, 4020, 4021,
}

// nextMapID reports whether map job ID n is a next job of m in
// rAthena's job change rules: a Novice becomes a first class or the
// Super Novice, and a first class one of its two second classes, with
// the same transcendent bit.
func nextMapID(m, n uint64) bool {
	upper := m & joblUpper
	if m&joblBaby != 0 || n&joblUpper != upper || n&(joblBaby|joblThird|mapidFourth) != 0 {
		return false
	}
	base := m &^ joblUpper
	nb := n &^ joblUpper
	switch {
	case base == mapidNovice:
		if nb&joblSecond == 0 {
			f := nb & mapidFirstMask
			return f > 0 && f <= mapidThief
		}
		return nb == mapidSuperNovice && upper == 0
	case base&joblSecond == 0 && base&mapidFirstMask > 0 && base&mapidFirstMask <= mapidThief:
		return nb == base|joblSecond1 || nb == base|joblSecond2
	}
	return false
}

// CoreJobData returns rAthena's jobs with the record fields that follow
// from the job database alone: the look and equips_as of each job is
// itself, the tier, transcendent and baby come from its map job ID,
// base_job is rAthena's BaseJob, and changes_from holds rAthena's job
// change rules. The name is job_<ID> and the display name Job <ID>, until
// server/gamedata names them. Jobs that have a record keep it.
func CoreJobData(d *Data) []JobData {
	byMapID := map[uint64][2]int{}
	for _, m := range d.MapIDJobs {
		byMapID[uint64(m.MapID)] = [2]int{m.Female, m.Male} // #nosec G115 -- map job IDs are positive.
	}
	jobOf := func(m uint64, male bool) (int, bool) {
		s, ok := byMapID[m]
		if !ok {
			return 0, false
		}
		if male {
			return s[1], true
		}
		return s[0], true
	}
	out := slices.Clone(d.Jobs)
	mapIDs := map[int]uint64{}
	for i := range out {
		mapIDs[out[i].ID] = uint64(out[i].MapID) // #nosec G115 -- as above.
	}
	for i := range out {
		j := &out[i]
		if j.Info.Name != "" {
			continue
		}
		m := mapIDs[j.ID]
		info := JobInfo{Name: fmt.Sprintf("job_%d", j.ID), DisplayName: fmt.Sprintf("Job %d", j.ID), Look: j.ID, EquipsAs: j.ID,
			Tier: TierOf(m), Transcendent: m&joblUpper != 0, Baby: m&joblBaby != 0, BaseJob: j.ID}
		// rAthena's BaseJob: the job of the class without its third and
		// transcendent bits, of the job's own sex.
		male := true
		if k, ok := jobOf(m, true); ok && k != j.ID {
			male = false
		}
		if b, ok := jobOf(m&mapidSecondMask, male); ok {
			info.BaseJob = b
		}
		if slices.Contains(changeJobs, j.ID) {
			for _, from := range slices.Sorted(maps.Keys(mapIDs)) {
				if nextMapID(mapIDs[from], m) {
					info.ChangesFrom = append(info.ChangesFrom, JobChange{Job: from})
				}
			}
		}
		j.Info = info
	}
	return out
}
