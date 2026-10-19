package modscript

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Job changes and resets (proposal 0002, review decision 21):
// player:change_job, player:rebirth, jobs.next_jobs, player:reset_skills,
// player:reset_stats, player:reset_skill and player:reset_stat, and
// player:give_skill (review decision 57).

// JobHost is what the job calls need of the zone.
type JobHost interface {
	// NextJobs is jobs.next_jobs: the jobs a job can change into.
	NextJobs(job int) []int
	// ChangeJob is pc_jobchange. Without force, the job must be a next job
	// of the player's. It returns false when nothing changed.
	ChangeJob(player int64, job int, force bool) (bool, error)
	// Rebirth makes a second class at base 99 and job 50 a High Novice.
	Rebirth(player int64) (bool, error)
	// ResetSkills is resetskill, ResetStats is resetstatus.
	ResetSkills(player int64) error
	ResetStats(player int64) error
	// ResetSkill lowers one skill. It returns false and changes nothing
	// when other skills need it and withDependents is not set.
	ResetSkill(player int64, skill, toLevel int, withDependents bool) (bool, error)
	// ResetStat lowers one stat (0 STR to 5 LUK) and refunds its points.
	ResetStat(player int64, stat, to int) (bool, error)
	// GiveSkill is pc_skill: it gives a skill at a level, of a kind. It
	// returns false when the skill does not exist. The level is 0 to
	// MaxGivenSkillLevel.
	GiveSkill(player int64, skill, level int, kind SkillKind) (bool, error)
}

// SkillKind is the kind of player:give_skill, as rAthena's
// e_addskill_type.
type SkillKind int

const (
	// SkillPermanent is ADDSKILL_PERMANENT: the level is learned and
	// saved. Level 0 removes the skill.
	SkillPermanent SkillKind = iota
	// SkillTemporary is ADDSKILL_TEMP: the skill's level is the higher of
	// its level and the given one, until the player's skills are
	// recalculated.
	SkillTemporary
	// SkillTemporaryAdd is ADDSKILL_TEMP_ADDLEVEL: the given level adds
	// to the skill's level, until the player's skills are recalculated.
	SkillTemporaryAdd
)

// skillKinds are the kinds of player:give_skill by name.
var skillKinds = map[string]SkillKind{"permanent": SkillPermanent, "temporary": SkillTemporary, "temporary_add": SkillTemporaryAdd}

// MaxGivenSkillLevel is rAthena's MAX_SKILL_LEVEL: the highest level
// player:give_skill gives.
const MaxGivenSkillLevel = 13

// jobHost is the zone's JobHost.
func (r *Runtime) jobHost() (JobHost, error) {
	h, ok := r.host.(JobHost)
	if !ok {
		return nil, errors.New("job changes do not run here")
	}
	return h, nil
}

// maxJobID bounds job arguments.
const maxJobID = 1 << 20

func (r *Runtime) jobsNext(c *script.Call) error {
	job, ok := c.Arg(0).AsInt()
	if !ok || job < 0 || job > maxJobID {
		return errors.New("jobs.next_jobs: want a job, such as player.job")
	}
	h, err := r.jobHost()
	if err != nil {
		return err
	}
	var out []script.Value
	for _, j := range h.NextJobs(int(job)) {
		out = append(out, script.Int(int64(j)))
	}
	c.Return(script.List(out...))
	return nil
}

// playerChangeJob is player:change_job(job, { force }). The prelude
// passes force as the third argument.
func (r *Runtime) playerChangeJob(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:change_job"); err != nil {
		return err
	}
	job, ok := c.Arg(1).AsInt()
	if !ok || job < 0 || job > maxJobID {
		return errors.New("player:change_job: want a job from wa.jobs")
	}
	force := c.Arg(2).Truthy()
	if force {
		if err := r.need("player:change_job with force", "player.change_job"); err != nil {
			return err
		}
	}
	h, err := r.jobHost()
	if err != nil {
		return err
	}
	changed, err := h.ChangeJob(id, int(job), force)
	if err != nil {
		return err
	}
	c.Return(script.Bool(changed))
	return nil
}

func (r *Runtime) playerRebirth(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:rebirth"); err != nil {
		return err
	}
	h, err := r.jobHost()
	if err != nil {
		return err
	}
	ok, err := h.Rebirth(id)
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

// playerReset is player:reset_skills and player:reset_stats, by the
// second argument: "skills" or "stats".
func (r *Runtime) playerReset(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	what := "player:reset_" + c.String(1)
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.jobHost()
	if err != nil {
		return err
	}
	switch c.String(1) {
	case "skills":
		return h.ResetSkills(id)
	case "stats":
		return h.ResetStats(id)
	}
	return fmt.Errorf("no reset %q", c.String(1))
}

// playerResetSkill is player:reset_skill(skill, { to_level,
// with_dependents }). The prelude passes the options as arguments.
func (r *Runtime) playerResetSkill(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:reset_skill"); err != nil {
		return err
	}
	skill, ok := c.Arg(1).AsInt()
	if !ok || skill <= 0 || skill > maxJobID {
		return errors.New("player:reset_skill: want a skill from skills.id")
	}
	to := int64(0)
	if !c.Arg(2).IsNil() {
		if to, ok = c.Arg(2).AsInt(); !ok || to < 0 || to > 1000 {
			return errors.New("player:reset_skill: to_level must be an integer from 0")
		}
	}
	h, err := r.jobHost()
	if err != nil {
		return err
	}
	done, err := h.ResetSkill(id, int(skill), int(to), c.Arg(3).Truthy())
	if err != nil {
		return err
	}
	c.Return(script.Bool(done))
	return nil
}

// statNames are the stats of player:reset_stat, in rAthena's SP_STR to
// SP_LUK order.
var statNames = map[string]int{"str": 0, "agi": 1, "vit": 2, "int": 3, "dex": 4, "luk": 5}

// playerResetStat is player:reset_stat(stat, { to }).
func (r *Runtime) playerResetStat(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:reset_stat"); err != nil {
		return err
	}
	name, _ := c.Arg(1).AsString()
	stat, ok := statNames[name]
	if !ok {
		return errors.New(`player:reset_stat: want "str", "agi", "vit", "int", "dex" or "luk"`)
	}
	to := int64(1)
	if !c.Arg(2).IsNil() {
		if to, ok = c.Arg(2).AsInt(); !ok || to < 1 || to > 1000 {
			return errors.New("player:reset_stat: to must be an integer from 1")
		}
	}
	h, err := r.jobHost()
	if err != nil {
		return err
	}
	done, err := h.ResetStat(id, stat, int(to))
	if err != nil {
		return err
	}
	c.Return(script.Bool(done))
	return nil
}

// playerGiveSkill is player:give_skill(skill, level, kind), as rAthena's
// skill command (review decision 57).
func (r *Runtime) playerGiveSkill(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:give_skill"); err != nil {
		return err
	}
	if err := r.need("player:give_skill", "player.skills"); err != nil {
		return err
	}
	skill, ok := c.Arg(1).AsInt()
	if !ok || skill <= 0 || skill > maxJobID {
		return errors.New("player:give_skill: want a skill from skills.id")
	}
	level, ok := c.Arg(2).AsInt()
	if !ok || level < 0 {
		return errors.New("player:give_skill: want a level from 0")
	}
	name, _ := c.Arg(3).AsString()
	kind, ok := skillKinds[name]
	if !ok {
		return errors.New(`player:give_skill: want the kind "permanent", "temporary" or "temporary_add"`)
	}
	h, err := r.jobHost()
	if err != nil {
		return err
	}
	if level > MaxGivenSkillLevel {
		// pc_skill refuses it with an error.
		c.Return(script.Bool(false))
		return nil
	}
	given, err := h.GiveSkill(id, int(skill), int(level), kind)
	if err != nil {
		return err
	}
	c.Return(script.Bool(given))
	return nil
}
