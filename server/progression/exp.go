package progression

// Character is a player's progression, as the parts of rAthena's
// mmo_charstatus that exp changes.
type Character struct {
	Job                       int
	BaseLevel, JobLevel       int
	BaseExp, JobExp           int64
	StatusPoints, SkillPoints int
}

// NewCharacter is a new character of a job at base and job level 1, with
// the status points of level 1 and no skill points.
func (t *Tables) NewCharacter(job int) Character {
	return Character{Job: job, BaseLevel: 1, JobLevel: 1, StatusPoints: t.StatusPoints(1)}
}

// MaxBaseLevel is pc_maxbaselv. It is 0 for a job without exp tables.
func (t *Tables) MaxBaseLevel(job int) int {
	if j := t.jobs[job]; j != nil {
		return j.maxBase
	}
	return 0
}

// MaxJobLevel is pc_maxjoblv.
func (t *Tables) MaxJobLevel(job int) int {
	if j := t.jobs[job]; j != nil {
		return j.maxJob
	}
	return 0
}

// BaseExpAt is JobDatabase::get_baseExp: the exp to go from a base level
// to the next.
func (t *Tables) BaseExpAt(job, level int) int64 {
	if j := t.jobs[job]; j != nil && level >= 1 && level <= MaxLevel {
		return j.baseExp[level-1]
	}
	return 0
}

// JobExpAt is JobDatabase::get_jobExp.
func (t *Tables) JobExpAt(job, level int) int64 {
	if j := t.jobs[job]; j != nil && level >= 1 && level <= MaxLevel {
		return j.jobExp[level-1]
	}
	return 0
}

// IsMaxBaseLevel is pc_is_maxbaselv.
func (t *Tables) IsMaxBaseLevel(c *Character) bool { return c.BaseLevel >= t.MaxBaseLevel(c.Job) }

// IsMaxJobLevel is pc_is_maxjoblv.
func (t *Tables) IsMaxJobLevel(c *Character) bool { return c.JobLevel >= t.MaxJobLevel(c.Job) }

// NextBaseExp is pc_nextbaseexp: the exp the current base level needs.
// At the maximum level it is MaxLevelBaseExp, the most exp a player there
// may hold.
func (t *Tables) NextBaseExp(c *Character) int64 {
	if c.BaseLevel == 0 {
		return 0
	}
	if t.IsMaxBaseLevel(c) {
		return MaxLevelBaseExp
	}
	return t.BaseExpAt(c.Job, c.BaseLevel)
}

// NextJobExp is pc_nextjobexp.
func (t *Tables) NextJobExp(c *Character) int64 {
	if c.JobLevel == 0 {
		return 0
	}
	if t.IsMaxJobLevel(c) {
		return MaxLevelJobExp
	}
	return t.JobExpAt(c.Job, c.JobLevel)
}

// StatusPoints is PlayerStatPointDatabase::get_table_point: the total
// status points at a base level.
func (t *Tables) StatusPoints(level int) int {
	if level < 1 || level >= len(t.statPoints) {
		return 0
	}
	return t.statPoints[level]
}

// StatusPointGain is pc_gets_status_point: the status points for going
// from a base level to the next.
func (t *Tables) StatusPointGain(level int) int {
	return max(t.StatusPoints(level+1)-t.StatusPoints(level), 0)
}

// Gain is what GainExp did.
type Gain struct {
	// BaseExp and JobExp are the exp given, after the caps.
	BaseExp, JobExp int64
	// BaseLevels and JobLevels are the levels gained.
	BaseLevels, JobLevels int
}

// GainExp is pc_gainexp for exp from a monster, with no exp bonuses, no
// guild tax, and not on a PvP map. It adds the exp and then levels up as
// pc_checkbaselevelup and pc_checkjoblevelup.
func (t *Tables) GainExp(c *Character, base, job int64) Gain {
	maxBase, maxJob := t.IsMaxBaseLevel(c), t.IsMaxJobLevel(c)
	// pc_calcexp adds no bonus without equipment, skills or VIP.
	nextB, nextJ := t.NextBaseExp(c), t.NextJobExp(c)
	if maxBase {
		if c.BaseExp >= MaxLevelBaseExp {
			base = 0
		} else if base >= MaxLevelBaseExp-c.BaseExp {
			base = MaxLevelBaseExp - c.BaseExp
		}
	}
	if maxJob {
		if c.JobExp >= MaxLevelJobExp {
			job = 0
		} else if job >= MaxLevelJobExp-c.JobExp {
			job = MaxLevelJobExp - c.JobExp
		}
	}
	if t.MaxExpGainRate != 0 && (base != 0 || job != 0) {
		rate := float64(t.MaxExpGainRate) / 1000
		// rAthena compares in float and truncates to 32 bits.
		if nextB > 0 && float64(float32(base)/float32(nextB)) > rate {
			base = int64(uint32(rate * float64(nextB)))
		}
		if nextJ > 0 && float64(float32(job)/float32(nextJ)) > rate {
			job = int64(uint32(rate * float64(nextJ)))
		}
	}
	g := Gain{BaseExp: base, JobExp: job}
	if base != 0 {
		c.BaseExp = addCap(c.BaseExp, base, MaxExp)
		g.BaseLevels = t.checkBaseLevelUp(c)
	}
	if job != 0 {
		c.JobExp = addCap(c.JobExp, job, MaxExp)
		g.JobLevels = t.checkJobLevelUp(c)
	}
	return g
}

// checkBaseLevelUp is pc_checkbaselevelup. It returns the levels gained.
// Without multi_level_up, the exp left after a level up is at most the
// exp of the level just finished, minus 1.
func (t *Tables) checkBaseLevelUp(c *Character) int {
	next := t.NextBaseExp(c)
	if next == 0 || c.BaseExp < next || t.IsMaxBaseLevel(c) {
		return 0
	}
	levels := 0
	for {
		c.BaseExp -= next
		if (!t.MultiLevelUp || t.MultiLevelUpBase > 0 && c.BaseLevel >= t.MultiLevelUpBase) && c.BaseExp > next-1 {
			c.BaseExp = next - 1
		}
		c.StatusPoints += t.StatusPointGain(c.BaseLevel)
		c.BaseLevel++
		levels++
		if t.IsMaxBaseLevel(c) {
			c.BaseExp = min(c.BaseExp, MaxLevelBaseExp)
			break
		}
		if next = t.NextBaseExp(c); next <= 0 || c.BaseExp < next {
			break
		}
	}
	return levels
}

// checkJobLevelUp is pc_checkjoblevelup. Each job level gives one skill
// point.
func (t *Tables) checkJobLevelUp(c *Character) int {
	next := t.NextJobExp(c)
	if next == 0 || c.JobExp < next || t.IsMaxJobLevel(c) {
		return 0
	}
	levels := 0
	for {
		c.JobExp -= next
		if (!t.MultiLevelUp || t.MultiLevelUpJob > 0 && c.JobLevel >= t.MultiLevelUpJob) && c.JobExp > next-1 {
			c.JobExp = next - 1
		}
		c.JobLevel++
		c.SkillPoints++
		levels++
		if t.IsMaxJobLevel(c) {
			c.JobExp = min(c.JobExp, MaxLevelJobExp)
			break
		}
		if next = t.NextJobExp(c); next <= 0 || c.JobExp < next {
			break
		}
	}
	return levels
}

// MobExp scales a monster's exp from mob_db.yml by a rate in percent, as
// MobDatabase does when loading BaseExp and JobExp.
func MobExp(exp int64, rate int) int64 {
	return capFloat(float64(exp)*float64(rate)/100, 0)
}

// addCap is util::safe_addition_cap for non-negative values.
func addCap(a, b, limit int64) int64 {
	if b > limit-a {
		return limit
	}
	return a + b
}

// capFloat is cap_value(v, lo, MAX_EXP) on a double, then the cast to
// t_exp, which truncates.
func capFloat(v float64, lo int64) int64 {
	switch {
	case v >= float64(MaxExp):
		return MaxExp
	case v <= float64(lo):
		return lo
	}
	return int64(v)
}

// applyRate is rAthena's apply_rate on integers: val * rate / 100, dividing
// first above 100000 to avoid overflow.
func applyRate(val int64, rate int) int64 {
	switch {
	case rate == 100:
		return val
	case val > 100000:
		return val / 100 * int64(rate)
	}
	return val * int64(rate) / 100
}

// JobExp is a job's exp tables, as a job record of data/jobs.yml holds
// them: BaseExp[i] is the exp to go from base level i+1 to i+2, and
// JobExp the same for job levels.
type JobExp struct {
	MaxBaseLevel, MaxJobLevel int
	BaseExp, JobExp           []int64
}

// ExpOf returns a job's exp tables, as JobExp. ok is false for a job
// without them.
func (t *Tables) ExpOf(job int) (JobExp, bool) {
	j := t.jobs[job]
	if j == nil {
		return JobExp{}, false
	}
	e := JobExp{MaxBaseLevel: j.maxBase, MaxJobLevel: j.maxJob}
	e.BaseExp = trimZeros(j.baseExp[:])
	e.JobExp = trimZeros(j.jobExp[:])
	return e, true
}

// trimZeros returns a copy of a table without its trailing zeros.
func trimZeros(table []int64) []int64 {
	n := len(table)
	for n > 0 && table[n-1] == 0 {
		n--
	}
	return append([]int64(nil), table[:n]...)
}

// WithJobs returns tables whose exp tables are the given ones, by job
// ID: every job, core's included. The other tables stay shared.
func (t *Tables) WithJobs(jobs map[int]JobExp) *Tables {
	c := *t
	c.jobs = make(map[int]*job, len(jobs))
	for id, e := range jobs {
		j := &job{maxBase: levelCap(e.MaxBaseLevel), maxJob: levelCap(e.MaxJobLevel)}
		for i, exp := range e.BaseExp {
			if level := i + 1; level <= j.maxBase && level <= MaxLevel {
				j.baseExp[i] = exp
			}
		}
		for i, exp := range e.JobExp {
			if level := i + 1; level <= j.maxJob && level <= MaxLevel {
				j.jobExp[i] = exp
			}
		}
		c.jobs[id] = j
	}
	return &c
}
