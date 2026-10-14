package gamedata

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Job records (proposal 0002, review decision 24): core's jobs are
// rAthena's job database converted to records, and the mods'
// data/jobs.yml changes them and adds jobs. The job tables, the exp
// tables, jobs.next_jobs and the skill trees of new jobs are built from
// the records. mods/README.md, "Jobs", lists the fields.

// JobRecord is one job record with its names resolved: rAthena's job_db
// entry with the record's own fields, and its exp tables.
type JobRecord struct {
	Data status.JobData
	// Exp is the job's exp tables. HasExp is false for a job that has
	// none, such as a mounted job's ID.
	Exp    progression.JobExp
	HasExp bool
	// Art is the body sprites of the record's look, by sex: m and f
	// (decision 0033). Empty keeps the look's own sprites.
	Art map[string]string
}

// lookJob reads a job's look field: a job name, or a table with the job
// it starts from and body sprites by sex, such as
// { job: knight, m: mod/example.jobs/sprite/body/Desperado/m }.
func lookJob(v any) (string, map[string]string, error) {
	switch l := v.(type) {
	case string:
		return l, nil, nil
	case map[string]any:
		job, _ := l["job"].(string)
		if _, ok := l["job"]; ok && job == "" {
			return "", nil, errors.New("look: job is a job's name")
		}
		rest := maps.Clone(l)
		delete(rest, "job")
		art, err := mod.ArtTable(rest, []string{"m", "f"}, "sprite")
		if err != nil {
			return "", nil, fmt.Errorf("look: %w", err)
		}
		return job, art, nil
	}
	return "", nil, errors.New("look is a job's name, or a table with job, m and f")
}

// clone copies a record, so a change does not reach the original.
func (r *JobRecord) clone() *JobRecord {
	c := *r
	d := &c.Data
	d.ASPDBase = slices.Clone(d.ASPDBase)
	d.BonusStats = slices.Clone(d.BonusStats)
	d.BaseHP = slices.Clone(d.BaseHP)
	d.BaseSP = slices.Clone(d.BaseSP)
	d.Info.ChangesFrom = slices.Clone(d.Info.ChangesFrom)
	d.Info.Inherits = slices.Clone(d.Info.Inherits)
	c.Exp.BaseExp = slices.Clone(c.Exp.BaseExp)
	c.Exp.JobExp = slices.Clone(c.Exp.JobExp)
	return &c
}

// Jobs are every job record: core's with the mods' overlays, and the
// mods' new jobs.
type Jobs struct {
	// Records are by job ID.
	Records map[int]*JobRecord
	// Overlay is the merge of the mods' data/jobs.yml.
	Overlay *mod.Overlay
	// names finds a job by any of its names: the names of wa.jobs, and
	// the mods' full names.
	names map[string]int
}

// ID finds a job by a name of wa.jobs, such as knight, or a mod job's
// full name.
func (j *Jobs) ID(name string) (int, bool) {
	id, ok := j.names[name]
	return id, ok
}

// StatusData returns the records as the job tables read them, in job ID
// order.
func (j *Jobs) StatusData() []status.JobData {
	out := make([]status.JobData, 0, len(j.Records))
	for _, id := range slices.Sorted(maps.Keys(j.Records)) {
		out = append(out, j.Records[id].Data)
	}
	return out
}

// ExpTables returns every job's exp tables, by job ID.
func (j *Jobs) ExpTables() map[int]progression.JobExp {
	out := map[int]progression.JobExp{}
	for id, r := range j.Records {
		if r.HasExp {
			out[id] = r.Exp
		}
	}
	return out
}

// jobDisplayName is the name players see for a core job, from its
// rAthena constant: JOB_LORD_KNIGHT is Lord Knight, JOB_SWORDMAN_HIGH is
// High Swordman, and the mounted JOB_KNIGHT2 is Knight, as the client's
// jobName.
func jobDisplayName(constant string) string {
	s := strings.TrimSuffix(strings.TrimPrefix(constant, "JOB_"), "2")
	words := strings.Split(strings.ToLower(s), "_")
	if len(words) > 1 && words[len(words)-1] == "high" {
		words = append([]string{"high"}, words[:len(words)-1]...)
	}
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// coreJobCache caches core's job records of each mode.
var coreJobCache sync.Map // reference.Mode -> *coreJobsOnce

type coreJobsOnce struct {
	once  sync.Once
	jobs  map[int]*JobRecord
	names map[string]int
	err   error
}

// CoreJobs converts a mode's job database into core's job records:
// job_stats.json with the exp tables of progression.json, the Inherit
// lists of skill_trees.json, the names of wa.jobs, and jobs.next_jobs
// from rAthena's job change rules. names maps every name of wa.jobs to
// its job. The result is shared: do not change it.
func CoreJobs(mode reference.Mode, consts *constants.Data) (map[int]*JobRecord, map[string]int, error) {
	v, _ := coreJobCache.LoadOrStore(mode, &coreJobsOnce{})
	c := v.(*coreJobsOnce)
	c.once.Do(func() { c.jobs, c.names, c.err = convertCoreJobs(mode, consts) })
	return c.jobs, c.names, c.err
}

func convertCoreJobs(mode reference.Mode, consts *constants.Data) (map[int]*JobRecord, map[string]int, error) {
	var d status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &d); err != nil {
		return nil, nil, err
	}
	exp, _, err := LoadProgression(mode)
	if err != nil {
		return nil, nil, err
	}
	_, trees, err := LoadSkillData(mode)
	if err != nil {
		return nil, nil, err
	}
	inherits := map[int][]int{}
	for _, t := range trees.Trees {
		inherits[t.Job] = slices.Clone(t.Inherit)
	}
	var module *constants.Module
	if consts != nil {
		module = consts.Modules()["jobs"]
	}
	names := map[string]int{}
	if module != nil {
		for n, v := range module.Values {
			names[n] = int(v)
		}
	}
	jobs := status.CoreJobData(&d)
	out := make(map[int]*JobRecord, len(jobs))
	for _, jd := range jobs {
		r := &JobRecord{Data: jd}
		info := &r.Data.Info
		if module != nil {
			if n, ok := module.Names[int64(jd.ID)]; ok {
				info.Name = n
				info.DisplayName = jobDisplayName(module.Origins[n])
			}
		}
		info.Inherits = inherits[jd.ID]
		if e, ok := exp.ExpOf(jd.ID); ok {
			r.Exp, r.HasExp = e, true
		}
		names[info.Name] = jd.ID
		out[jd.ID] = r
	}
	return out, names, nil
}

// Fields of a job record.
var jobFields = []string{
	"display_name", "look", "tier", "transcendent", "baby", "base_job", "changes_from", "inherits", "equips_as",
	"max_base_level", "max_job_level", "max_stats", "max_weight", "hp", "sp", "hp_table", "sp_table", "aspd",
	"bonus_stats", "exp_table", "base_exp", "job_exp",
}

// coreFixedJobFields are fields that set a core job's class or its skill
// tree, which come from rAthena: a mod changes them only on its own jobs.
var coreFixedJobFields = []string{"tier", "transcendent", "baby", "base_job", "inherits"}

// bonusStatNames are the stats of bonus_stats.
var bonusStatNames = []string{"str", "agi", "vit", "int", "dex", "luk", "pow", "sta", "wis", "spl", "con", "crt"}

// modJobs applies the mods' data/jobs.yml over core's job records. New
// jobs get IDs in the mod range from ids.
func modJobs(ctx context.Context, set *mod.Set, mode reference.Mode, ids *mod.IDs, consts *constants.Data) (*Jobs, error) {
	core, coreNames, err := CoreJobs(mode, consts)
	if err != nil {
		return nil, err
	}
	o, err := set.Jobs(func(name string) bool { _, ok := coreNames[name]; return ok && !strings.Contains(name, ":") })
	if err != nil {
		return nil, err
	}
	out := &Jobs{Records: maps.Clone(core), Overlay: o, names: maps.Clone(coreNames)}
	if len(o.Records) == 0 {
		return out, nil
	}
	// IDs first, so any record can name a new job.
	newIDs := map[string]int{}
	for _, r := range o.Records {
		if _, ok := coreNames[r.Name]; ok && !strings.Contains(r.Name, ":") {
			continue
		}
		id, err := ids.ID(ctx, mod.JobIDs, r.Name, r.Mod)
		if err != nil {
			return nil, err
		}
		newIDs[r.Name] = int(id)
		out.names[r.Name] = int(id)
	}
	var weapons *constants.Module
	if consts != nil {
		weapons = consts.Modules()["weapon_types"]
	}
	b := &jobBuilder{jobs: out, overlay: o, newIDs: newIDs, weapons: weapons, state: map[string]int{}}
	var errs []error
	for _, r := range o.Records {
		if err := b.build(r); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

// jobBuilder applies the job records in an order where a new job's look
// is built before it.
type jobBuilder struct {
	jobs    *Jobs
	overlay *mod.Overlay
	newIDs  map[string]int
	weapons *constants.Module
	// state is 1 while a record is built and 2 once it is done.
	state map[string]int
}

// build applies one record of the overlay.
func (b *jobBuilder) build(r *mod.Record) error {
	switch b.state[r.Name] {
	case 1:
		return fmt.Errorf("%s: jobs %s: its look leads back to itself", r.Where, r.Name)
	case 2:
		return nil
	}
	b.state[r.Name] = 1
	defer func() { b.state[r.Name] = 2 }()
	id, isNew := b.newIDs[r.Name]
	var rec *JobRecord
	if !isNew {
		id = b.jobs.names[r.Name]
		rec = b.jobs.Records[id].clone()
		for _, k := range coreFixedJobFields {
			if _, ok := r.Fields[k]; ok {
				return fmt.Errorf("%s: jobs %s: %s comes from rAthena for a core job; a mod sets it on its own jobs", r.Where, r.Name, k)
			}
		}
	} else {
		lookName, _, _ := lookJob(r.Fields["look"])
		lookID, ok := b.jobs.names[lookName]
		if !ok {
			return fmt.Errorf("%s: jobs %s: a new job needs look: the job whose sprites it shows and whose fields it starts from, such as knight", r.Where, r.Name)
		}
		if lr := b.overlay.Record(lookName); lr != nil {
			if err := b.build(lr); err != nil {
				return err
			}
		}
		look := b.jobs.Records[lookID]
		if look == nil {
			return fmt.Errorf("%s: jobs %s: look %s has no record", r.Where, r.Name, lookName)
		}
		rec = look.clone()
		rec.Data.ID = id
		rec.Data.Info.Name, rec.Data.Info.Mod = r.Name, r.Mod
		rec.Data.Info.Look = look.Data.Info.Look
		rec.Data.Info.DisplayName = mod.Local(r.Name)
	}
	if err := b.apply(rec, r, isNew); err != nil {
		return err
	}
	if isNew {
		lookName, _, _ := lookJob(r.Fields["look"])
		look := b.jobs.Records[b.jobs.names[lookName]]
		m := status.ModMapID(uint64(look.Data.MapID), rec.Data.Info.Tier, rec.Data.Info.Transcendent, rec.Data.Info.Baby) // #nosec G115 -- map job IDs are positive.
		rec.Data.MapID = int64(m)                                                                                         // #nosec G115 -- below 2^22.
	}
	b.jobs.Records[id] = rec
	return nil
}

// apply sets a record's fields on a job.
func (b *jobBuilder) apply(rec *JobRecord, r *mod.Record, isNew bool) error {
	f := r.Fields
	d, info := &rec.Data, &rec.Data.Info
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	for k := range f {
		if !slices.Contains(jobFields, k) {
			bad("%q is not a field; jobs have %s", k, strings.Join(jobFields, ", "))
		}
	}
	job := func(k string, v any) (int, bool) {
		s, _ := v.(string)
		id, ok := b.jobs.names[s]
		if !ok {
			bad("%s: %v is not a job: want a name of wa.jobs, such as knight, or a mod job's full name", k, v)
		}
		return id, ok
	}
	num := func(k string, lo, hi int, to *int) {
		v, ok := f[k]
		if !ok {
			return
		}
		n, ok := v.(int)
		if !ok || n < lo || n > hi {
			bad("%s must be a whole number from %d to %d", k, lo, hi)
			return
		}
		*to = n
	}
	flag := func(k string, to *bool) {
		v, ok := f[k]
		if !ok {
			return
		}
		x, ok := v.(bool)
		if !ok {
			bad("%s must be true or false", k)
			return
		}
		*to = x
	}
	if v, ok := f["display_name"]; ok {
		s, _ := v.(string)
		if s == "" || len(s) > 64 {
			bad("display_name must be 1 to 64 bytes")
		}
		info.DisplayName = s
	}
	if v, ok := f["look"]; ok {
		name, art, err := lookJob(v)
		switch {
		case err != nil:
			bad("%v", err)
		case !isNew && name != "":
			// A core job's look changes only its sprites.
			if id, ok := job("look", name); ok {
				info.Look = b.jobs.Records[id].Data.Info.Look
			}
		}
		if len(art) > 0 {
			rec.Art = art
		}
	}
	if v, ok := f["tier"]; ok {
		s, _ := v.(string)
		if !slices.Contains(status.JobTiers, s) {
			bad("tier must be one of %s", strings.Join(status.JobTiers, ", "))
		}
		info.Tier = s
	}
	flag("transcendent", &info.Transcendent)
	flag("baby", &info.Baby)
	if v, ok := f["base_job"]; ok {
		if id, ok := job("base_job", v); ok {
			info.BaseJob = id
		}
	}
	if v, ok := f["equips_as"]; ok {
		if id, ok := job("equips_as", v); ok {
			info.EquipsAs = id
		}
	}
	if v, ok := f["changes_from"]; ok {
		list, ok := v.([]any)
		if !ok {
			bad("changes_from must be a list of { job, base_level, job_level }")
		}
		info.ChangesFrom = nil
		for i, e := range list {
			m, ok := e.(map[string]any)
			if !ok {
				bad("changes_from[%d]: want { job: swordman, job_level: 40 }", i)
				continue
			}
			var c status.JobChange
			for k, x := range m {
				switch k {
				case "job":
					c.Job, _ = job(fmt.Sprintf("changes_from[%d].job", i), x)
				case "base_level", "job_level":
					n, ok := x.(int)
					if !ok || n < 0 || n > progression.MaxLevel {
						bad("changes_from[%d].%s must be a whole number from 0 to %d", i, k, progression.MaxLevel)
					}
					if k == "base_level" {
						c.BaseLevel = n
					} else {
						c.JobLevel = n
					}
				default:
					bad("changes_from[%d]: %q is not a field; an entry has job, base_level and job_level", i, k)
				}
			}
			if _, ok := m["job"]; !ok {
				bad("changes_from[%d] needs job", i)
			}
			info.ChangesFrom = append(info.ChangesFrom, c)
		}
	}
	if v, ok := f["inherits"]; ok {
		list, ok := v.([]any)
		if !ok {
			bad("inherits must be a list of jobs, such as [novice, swordman, knight]")
		}
		info.Inherits = nil
		for i, e := range list {
			if id, ok := job(fmt.Sprintf("inherits[%d]", i), e); ok {
				info.Inherits = append(info.Inherits, id)
			}
		}
	}
	num("max_base_level", 1, progression.MaxLevel, &d.MaxBaseLevel)
	num("max_job_level", 1, progression.MaxLevel, &d.MaxJobLevel)
	num("max_stats", 1, 1<<15-1, &info.MaxStats)
	num("max_weight", 0, 1<<31-1, &d.MaxWeight)
	growth := func(k string, factor, increase *int) {
		v, ok := f[k]
		if !ok {
			return
		}
		m, ok := v.(map[string]any)
		fa, ok1 := m["factor"].(int)
		in, ok2 := m["increase"].(int)
		if !ok || len(m) != 2 || !ok1 || !ok2 || fa < 0 || in < 0 || fa > 1<<20 || in > 1<<20 {
			bad("%s must be { factor, increase }, whole numbers from 0, as job_stats.yml's %sFactor and %sIncrease", k, strings.ToUpper(k), strings.ToUpper(k))
			return
		}
		*factor, *increase = fa, in
	}
	if _, ok := f["hp"]; ok {
		growth("hp", &d.HPFactor, &d.HPIncrease)
		d.BaseHP = nil
	}
	if _, ok := f["sp"]; ok {
		growth("sp", &d.SPFactor, &d.SPIncrease)
		d.BaseSP = nil
	}
	table := func(k string) []int {
		v := f[k]
		list, ok := v.([]any)
		var out []int
		for _, e := range list {
			n, isInt := e.(int)
			if !isInt || n < 1 || n > 1<<30 {
				ok = false
				break
			}
			out = append(out, n)
		}
		if !ok || len(out) < d.MaxBaseLevel {
			bad("%s must list one whole number from 1 per base level, %d of them for max_base_level %d", k, d.MaxBaseLevel, d.MaxBaseLevel)
			return nil
		}
		return out
	}
	if _, ok := f["hp_table"]; ok {
		if _, both := f["hp"]; both {
			bad("set hp or hp_table, not both")
		}
		d.BaseHP = d.BaseHP[:0]
		for i, n := range table("hp_table") {
			d.BaseHP = append(d.BaseHP, status.JobHP{Level: i + 1, HP: n})
		}
	}
	if _, ok := f["sp_table"]; ok {
		if _, both := f["sp"]; both {
			bad("set sp or sp_table, not both")
		}
		d.BaseSP = d.BaseSP[:0]
		for i, n := range table("sp_table") {
			d.BaseSP = append(d.BaseSP, status.JobSP{Level: i + 1, SP: n})
		}
	}
	if v, ok := f["aspd"]; ok {
		m, ok := v.(map[string]any)
		if !ok {
			bad("aspd must map weapon types to base ASPD, such as { fist: 40, dagger: 55 }")
		}
		for _, k := range slices.Sorted(maps.Keys(m)) {
			var w int64 = -1
			if b.weapons != nil {
				if x, ok := b.weapons.Values[k]; ok {
					w = x
				}
			}
			n, isInt := m[k].(int)
			switch {
			case w < 0 || int(w) >= len(d.ASPDBase):
				bad("aspd: %q is not a weapon type of wa.weapon_types that the job table has", k)
			case !isInt || n < 0 || n > 2000:
				bad("aspd: %s must be a whole number from 0 to 2000", k)
			default:
				d.ASPDBase[w] = n
			}
		}
	}
	if v, ok := f["bonus_stats"]; ok {
		list, ok := v.([]any)
		if !ok {
			bad("bonus_stats must be a list of { level, str, agi, ... }, the stats each job level adds")
		}
		d.BonusStats = d.BonusStats[:0]
		for i, e := range list {
			m, ok := e.(map[string]any)
			lv, ok2 := m["level"].(int)
			if !ok || !ok2 || lv < 1 || lv > progression.MaxLevel {
				bad("bonus_stats[%d]: want { level: 2, luk: 1 }, with a job level from 1", i)
				continue
			}
			var row [12]int
			for k, x := range m {
				if k == "level" {
					continue
				}
				s := slices.Index(bonusStatNames, k)
				n, isInt := x.(int)
				if s < 0 || !isInt || n < -1000 || n > 1000 {
					bad("bonus_stats[%d]: %s must be one of %s with a whole number", i, k, strings.Join(bonusStatNames, ", "))
					continue
				}
				row[s] = n
			}
			d.BonusStats = append(d.BonusStats, status.JobBonusStats{Level: lv, Str: row[0], Agi: row[1], Vit: row[2], Int: row[3], Dex: row[4], Luk: row[5],
				Pow: row[6], Sta: row[7], Wis: row[8], Spl: row[9], Con: row[10], Crt: row[11]})
		}
	}
	if v, ok := f["exp_table"]; ok {
		if id, ok := job("exp_table", v); ok {
			from := b.jobs.Records[id]
			if from == nil || !from.HasExp {
				bad("exp_table: %v has no exp tables", v)
			} else {
				rec.Exp.BaseExp, rec.Exp.JobExp = slices.Clone(from.Exp.BaseExp), slices.Clone(from.Exp.JobExp)
				rec.HasExp = true
			}
		}
		if _, both := f["base_exp"]; both {
			bad("set exp_table, or base_exp and job_exp, not both")
		}
	}
	expList := func(k string, to *[]int64) {
		v, ok := f[k]
		if !ok {
			return
		}
		list, ok := v.([]any)
		var out []int64
		for _, e := range list {
			n, isInt := e.(int)
			if !isInt || n < 0 {
				ok = false
				break
			}
			out = append(out, int64(n))
		}
		if !ok {
			bad("%s must be a list of whole numbers from 0: the exp from each level to the next", k)
			return
		}
		*to = out
		rec.HasExp = true
	}
	expList("base_exp", &rec.Exp.BaseExp)
	expList("job_exp", &rec.Exp.JobExp)
	rec.Exp.MaxBaseLevel, rec.Exp.MaxJobLevel = d.MaxBaseLevel, d.MaxJobLevel
	if rec.HasExp && (len(rec.Exp.BaseExp) < d.MaxBaseLevel-1 || len(rec.Exp.JobExp) < d.MaxJobLevel-1) {
		bad("the exp tables have %d base levels and %d job levels, and max_base_level %d and max_job_level %d need %d and %d",
			len(rec.Exp.BaseExp), len(rec.Exp.JobExp), d.MaxBaseLevel, d.MaxJobLevel, d.MaxBaseLevel-1, d.MaxJobLevel-1)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s: jobs %s: %s", r.Where, r.Name, strings.Join(errs, "; "))
	}
	return nil
}

// modInherits returns each new job's inherits, by job ID.
func (j *Jobs) modInherits() map[int][]int {
	out := map[int][]int{}
	for id, r := range j.Records {
		if r.Data.Info.Mod != "" {
			out[id] = r.Data.Info.Inherits
		}
	}
	return out
}

// buildModTrees gives each new job its skill tree: the trees of the jobs
// it inherits, in order, as SkillTreeDatabase's Inherit. A job that
// inherits a new job gets that job's tree after it is built.
func buildModTrees(trees *skill.Trees, inherits map[int][]int) error {
	done := map[int]bool{}
	var visit func(id int, path []int) error
	visit = func(id int, path []int) error {
		if done[id] {
			return nil
		}
		if slices.Contains(path, id) {
			return fmt.Errorf("job %d inherits itself", id)
		}
		for _, from := range inherits[id] {
			if _, isMod := inherits[from]; isMod {
				if err := visit(from, append(path, id)); err != nil {
					return err
				}
			}
			tree := trees.Job(from)
			if tree == nil {
				continue
			}
			for _, s := range tree.Skills {
				if trees.Job(id).Skill(s.Skill) == nil {
					c := *s
					c.Requires = slices.Clone(s.Requires)
					trees.Set(id, c)
				}
			}
		}
		done[id] = true
		return nil
	}
	for _, id := range slices.Sorted(maps.Keys(inherits)) {
		if err := visit(id, nil); err != nil {
			return err
		}
	}
	return nil
}
