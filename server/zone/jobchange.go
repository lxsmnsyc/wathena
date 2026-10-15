package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Job changes and resets (proposal 0002, review decision 21), ported from
// rAthena's pc_jobchange, pc_resetlvl, pc_resetskill and pc_resetstate,
// and from the rules of the rebirth NPC (npc/jobs/valkyrie.txt).

// Map job ID bits that job changes read, as rAthena's e_mapid.
const (
	jcFirstMask  = 0xff
	jcSecondMask = 0xfff
	jcSecond1    = 0x100
	jcSecond2    = 0x200
	jcThird      = 0x1000
	jcUpper      = 0x100000
	jcBaby       = 0x200000
	jcFourth     = 0x10000
	jcSuperNov   = jcSecond1 // MAPID_SUPER_NOVICE: JOBL_2_1 | MAPID_NOVICE
	jcSummoner   = 0xa
)

// Jobs that job changes know of.
const (
	jobNovice     = 0
	jobNoviceHigh = 4001
)

// transcendentStatusPoints is transcendent_status_points of
// conf/battle/player.conf: the status points a transcendent job adds.
const transcendentStatusPoints = 52

// noviceHighStatusPoints are the status points pc_resetlvl gives a new
// High Novice ("not 88").
const noviceHighStatusPoints = 100

// NextJobs is jobs.next_jobs: the jobs whose record lists a job in
// changes_from, in job ID order. Core's records hold rAthena's rules: a
// Novice becomes a first class or a Super Novice, a High Novice a
// transcendent first class, and a first class one of its two second
// classes. Jobs that differ by sex, such as Bard and Dancer, are both in
// the list.
func NextJobs(t *status.Tables, job int) []int {
	if t == nil {
		return nil
	}
	return t.NextJobs(job)
}

// changeJob is player:change_job, with pc_jobchange's rules. Without
// force, the job's record must list the player's job in changes_from,
// with the levels it needs, and a transcendent player stays in the line
// it had before its rebirth, as the transcendent job NPCs check. It
// returns false and changes nothing when the change is not allowed.
func (z *Zone) changeJob(p *Player, job int, force bool) bool {
	if z.Status == nil || job < 0 {
		return false
	}
	cur, ok := z.Status.MapID(p.char.Job)
	if !ok {
		return false
	}
	target, ok := z.Status.MapID(job)
	if !ok {
		return false
	}
	if !z.Status.IsMod(job) {
		// pc_mapid2jobid with the player's sex: a male who changes into
		// a Dancer becomes a Bard. A mod job's map job ID names a core
		// job, so it keeps its own ID.
		if job, ok = z.Status.JobOf(target, p.char.Male); !ok {
			return false
		}
		if target == cur && !z.Status.IsMod(p.char.Job) {
			return false
		}
	}
	if job == p.char.Job {
		return false
	}
	if !force {
		c, ok := z.Status.ChangeFrom(job, p.char.Job)
		if !ok || !z.rebirthLine(p, target) {
			return false
		}
		if z.Progress != nil && (p.progress.BaseLevel < c.BaseLevel || p.progress.JobLevel < c.JobLevel) {
			return false
		}
	}
	z.jobChange(p, job, target, cur)
	return true
}

// rebirthLine reports whether a transcendent player may change into a
// job: the transcendent first class of its line, then its transcendent
// second class, as the transcendent job NPCs read ADVJOB.
func (z *Zone) rebirthLine(p *Player, target uint64) bool {
	if target&jcUpper == 0 || p.record.RebirthJob == 0 {
		return true
	}
	line, ok := z.Status.MapID(p.record.RebirthJob)
	if !ok {
		return true
	}
	if target&(jcSecond1|jcSecond2) == 0 {
		return target&jcFirstMask == line&jcFirstMask
	}
	return target&jcSecondMask == line&jcSecondMask
}

// jobChange is pc_jobchange after its checks.
func (z *Zone) jobChange(p *Player, job int, target, previous uint64) {
	// The job level at the change from a first class to a second class,
	// which the skill tree reads (change_level_2nd).
	if target&(jcSecond1|jcSecond2) != 0 && previous&(jcSecond1|jcSecond2) == 0 && z.Progress != nil {
		p.record.JobChangeLevel = p.progress.JobLevel
	}
	// The skill Plagiarism copied goes with the old job.
	z.plagiarismReset(p)
	from := p.char.Job
	p.char.Job = job
	z.updateBodyLook(p)
	if z.Progress != nil {
		p.progress.Job = job
		p.progress.JobLevel = 1
		p.progress.JobExp = 0
		if maxBase := z.Progress.MaxBaseLevel(job); maxBase > 0 && p.progress.BaseLevel > maxBase {
			p.progress.BaseLevel = maxBase
			p.progress.BaseExp = 0
			z.resetStatsOf(p)
		}
		// Transcendent status points.
		switch {
		case target&jcUpper != 0 && previous&jcUpper == 0:
			p.progress.StatusPoints += transcendentStatusPoints
		case target&jcUpper == 0 && previous&jcUpper != 0:
			if p.progress.StatusPoints < transcendentStatusPoints {
				// The player used its bonus points: its stats reset.
				z.resetStatsOf(p)
			} else {
				p.progress.StatusPoints -= transcendentStatusPoints
			}
		}
	}
	z.unequipInvalid(p)
	// The companion, mount and cart go when the new job does not meet
	// their requires.
	z.dropCompanionsWithoutSkills(p, false, true)
	if p.entity.sc.Has(sc.SCSpirit) {
		z.scEnd(p.entity, sc.SCSpirit)
	}
	z.afterJobChange(p)
	p.pending = append(p.pending, z.jobNameEvent(job))
	z.scriptEventFor("job_change", modscript.JobChange(int64(p.charID), int64(from), int64(job)))
}

// jobNameEvent names a job for the client, which knows only the jobs of
// its data tables.
func (z *Zone) jobNameEvent(job int) *wathenav1.Event {
	return wathenav1.Event_builder{JobName: wathenav1.JobName_builder{
		Job: uint32(job), Name: z.Status.DisplayName(job), // #nosec G115 -- job IDs are positive.
	}.Build()}.Build()
}

// afterJobChange computes the stats again and sends what changed, as the
// end of pc_jobchange, pc_resetlvl, pc_resetskill and pc_resetstate.
func (z *Zone) afterJobChange(p *Player) {
	z.markStats(p)
	z.flushStats()
	z.updateWeight(p)
	if z.Progress != nil {
		p.pending = append(p.pending, z.progressStats(p, nil))
	}
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
	z.sendSkillList(p)
}

// unequipInvalid takes off every item the player can no longer wear, as
// the loops of pc_jobchange and pc_resetlvl over pc_isequip.
func (z *Zone) unequipInvalid(p *Player) {
	if z.Items == nil || p.inventory == nil {
		return
	}
	w := z.wearer(p)
	for slot := range p.inventory.Slots {
		s := &p.inventory.Slots[slot]
		if s.ID == 0 || s.Equip == 0 {
			continue
		}
		d, ok := z.Items.Lookup(s.ID)
		if !ok {
			continue
		}
		weapon := p.inventory.Worn(z.Items).Weapon
		if item.CanEquip(d, w, weapon) != item.EquipOK {
			z.unequipItem(p, slot)
		}
	}
}

// Skills that job changes and resets read.
const (
	skillRiding   = 63 // KN_RIDING
	skillPushCart = 39 // MC_PUSHCART
)

// setOptions is pc_setoption for the saved options: the stats change,
// viewers see the new options, the player sees the icons of its looks,
// and a rider shows its mounted job.
func (z *Zone) setOptions(p *Player, o int) {
	old := p.record.Options
	p.record.Options = o
	p.pending = append(p.pending, lookIconEvents(old, o)...)
	z.statsChanged(p)
	p.entity.optionChanged = true
	z.updateBodyLook(p)
	if hadCart(old) != hadCart(o) {
		z.cartChanged(p, hadCart(old))
	}
}

// mountedJobs are the classes status_set_viewdata shows for a job that
// rides a Peco Peco.
var mountedJobs = map[int]int{7: 13, 14: 21, 4008: 4014, 4015: 4022, 4030: 4036, 4037: 4044}

// viewJob is the class a player shows, as status_set_viewdata: the
// mounted class while it rides.
func viewJob(job, options int) int {
	if options&companion.OptionRiding != 0 {
		if m, ok := mountedJobs[job]; ok {
			return m
		}
	}
	return job
}

// updateBodyLook sets the job a player shows, and marks the look for
// viewers when it changed (clif_changelook with LOOK_BASE). A job shows
// the sprites of its look.
func (z *Zone) updateBodyLook(p *Player) {
	j := uint32(viewJob(z.lookJob(p.char.Job), bodyOptions(p))) // #nosec G115 -- job IDs are positive.
	if p.Look.Job != j {
		p.Look.Job = j
		p.entity.lookChanged = true
	}
	if c, m := companionLookIDs(p); p.Look.Companion != c || p.Look.Mount != m {
		p.Look.Companion, p.Look.Mount = c, m
		p.entity.lookChanged = true
	}
}

// bodyOptions are the option bits that pick a player's body: its
// options, without the mount's bits when the mount's record names art.
// The client then draws the body on foot over the mount's art (decision
// 0036).
func bodyOptions(p *Player) int {
	if p.mount != nil && p.mount.Art != "" {
		return p.record.Options &^ companion.OptionLooks
	}
	return p.record.Options
}

// companionLookIDs are the record IDs of a player's companion and mount
// that looks send, or 0, so the client draws the art of a record that
// has it (decision 0036).
func companionLookIDs(p *Player) (companionID, mountID uint32) {
	if p.companion != nil {
		companionID = uint32(max(p.companion.ID, 0)) // #nosec G115 -- record IDs are small.
	}
	if p.mount != nil {
		mountID = uint32(max(p.mount.ID, 0)) // #nosec G115 -- record IDs are small.
	}
	return companionID, mountID
}

// lookJob is the job whose sprites a job shows. A mod's job shows its
// own ID, which the client data maps to its art and look.
func (z *Zone) lookJob(job int) int {
	if z.Status == nil || z.Mods != nil && z.Mods.OwnLookJobs[job] {
		return job
	}
	return z.Status.LookJob(job)
}

// rebirth is player:rebirth: a second class at base level 99 and job
// level 50 or more becomes a High Novice at base and job level 1, as the
// rebirth NPC's jobchange Job_Novice_High and resetlvl(1). Its second
// class picks its transcendent line. It returns false and changes
// nothing for any other player.
func (z *Zone) rebirth(p *Player) bool {
	if z.Status == nil || z.Progress == nil {
		return false
	}
	cur, ok := z.Status.MapID(p.char.Job)
	if !ok || cur&(jcUpper|jcBaby|jcThird|jcFourth) != 0 || cur&(jcSecond1|jcSecond2) == 0 || cur&jcSecondMask == jcSuperNov {
		return false
	}
	if p.progress.BaseLevel < 99 || p.progress.JobLevel < 50 {
		return false
	}
	second := p.char.Job
	if !z.Status.IsMod(second) {
		if second, ok = z.Status.JobOf(cur, p.char.Male); !ok {
			return false
		}
	}
	target, ok := z.Status.MapID(jobNoviceHigh)
	if !ok {
		return false
	}
	p.record.RebirthJob = second
	z.jobChange(p, jobNoviceHigh, target, cur)
	z.resetLevels(p)
	return true
}

// resetLevels is pc_resetlvl with type 1: skills reset, levels and exp
// go back to 1 and 0, every stat to 1, the options end, and a High Novice
// gets 100 status points with First Aid and Trick Dead.
func (z *Zone) resetLevels(p *Player) {
	z.resetSkillsOf(p)
	p.progress.SkillPoints = 0
	p.progress.BaseLevel, p.progress.JobLevel = 1, 1
	p.progress.BaseExp, p.progress.JobExp = 0, 0
	if p.record.Options != 0 {
		z.setOptions(p, 0)
	}
	p.char.Stats = status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}
	if p.char.Job == jobNoviceHigh {
		p.progress.StatusPoints = noviceHighStatusPoints
		// The platinum skills.
		if p.char.Skills == nil {
			p.char.Skills = map[int]int{}
		}
		p.char.Skills[skill.IDFirstAid] = 1
		p.char.Skills[skill.IDTrickDead] = 1
	}
	z.unequipInvalid(p)
	z.afterJobChange(p)
}

// resetSkills is player:reset_skills, rAthena's resetskill: every skill
// learned with points goes back to 0 and refunds its points. Quest
// skills stay, and so does a non-Novice's Basic Skill.
func (z *Zone) resetSkills(p *Player) {
	z.dropCompanionsWithoutSkills(p, true, false)
	points := z.resetSkillsOf(p)
	if z.Progress != nil {
		p.progress.SkillPoints += points
	}
	z.afterJobChange(p)
}

// resetSkillsOf is the loop of pc_resetskill: it lowers every skill it
// may to 0 and returns the points they cost.
func (z *Zone) resetSkillsOf(p *Player) int {
	points := 0
	for _, id := range sortedSkills(p.char.Skills) {
		if lv := p.char.Skills[id]; lv > 0 && z.resettable(p, id) {
			points += lv
			delete(p.char.Skills, id)
			z.passiveChanged(p, id)
		}
	}
	return points
}

// resettable reports whether pc_resetskill resets a skill: not wedding or
// Soul Linker spirit skills, not quest skills (quest_skill_reset is
// off), and not Basic Skill for jobs outside the Novice line.
func (z *Zone) resettable(p *Player, id int) bool {
	d := z.Skills.Get(id)
	if d == nil {
		return false
	}
	if d.Has("IsWedding") || d.Has("IsSpirit") || d.Has("IsQuest") {
		return false
	}
	if id == skill.IDBasic && !z.noviceLine(p) {
		return false
	}
	return true
}

// noviceLine reports whether the player's (class_ & MAPID_SECONDMASK) is
// MAPID_NOVICE: a Novice or a High Novice.
func (z *Zone) noviceLine(p *Player) bool {
	m, ok := z.Status.MapID(p.char.Job)
	return ok && m&jcSecondMask == jobNovice
}

// resetSkill is player:reset_skill: one skill goes down to a level and
// refunds its points. The skills whose tree entry needs a higher level
// of it go to 0 with it when withDependents is set; otherwise the call
// returns false and changes nothing. Quest skills are never lowered.
func (z *Zone) resetSkill(p *Player, id, toLevel int, withDependents bool) bool {
	lv := p.char.Skills[id]
	if toLevel < 0 || lv <= toLevel || !z.resettable(p, id) {
		return false
	}
	lower := map[int]int{id: toLevel}
	for changed := true; changed; {
		changed = false
		for _, other := range sortedSkills(p.char.Skills) {
			if _, done := lower[other]; done || p.char.Skills[other] <= 0 {
				continue
			}
			if !z.needsLowered(p, other, lower) {
				continue
			}
			if !withDependents {
				return false
			}
			if !z.resettable(p, other) {
				// A quest skill keeps its level.
				continue
			}
			lower[other] = 0
			changed = true
		}
	}
	points := 0
	for _, s := range sortedSkills(lower) {
		to := lower[s]
		points += p.char.Skills[s] - to
		if to == 0 {
			delete(p.char.Skills, s)
		} else {
			p.char.Skills[s] = to
		}
		z.passiveChanged(p, s)
	}
	if z.Progress != nil {
		p.progress.SkillPoints += points
	}
	z.dropCompanionsWithoutSkills(p, false, false)
	z.afterJobChange(p)
	return true
}

// needsLowered reports whether a learned skill's tree entry needs a level
// of a skill above the level it is lowered to.
func (z *Zone) needsLowered(p *Player, id int, lower map[int]int) bool {
	e := z.SkillTrees.Job(p.char.Job).Skill(id)
	if e == nil {
		return false
	}
	for _, r := range e.Requires {
		if to, ok := lower[r[0]]; ok && r[1] > to {
			return true
		}
	}
	return false
}

// resetStats is player:reset_stats, rAthena's resetstatus.
func (z *Zone) resetStats(p *Player) {
	z.resetStatsOf(p)
	z.afterJobChange(p)
}

// resetStatsOf is pc_resetstate: the status points of the base level,
// with the transcendent bonus, and every stat at 1.
func (z *Zone) resetStatsOf(p *Player) {
	if z.Progress == nil {
		return
	}
	if maxBase := z.Progress.MaxBaseLevel(p.char.Job); maxBase > 0 && p.progress.BaseLevel > maxBase {
		p.progress.BaseLevel = maxBase
	}
	points := z.Progress.StatusPoints(p.progress.BaseLevel)
	if m, ok := z.Status.MapID(p.char.Job); ok && m&jcUpper != 0 {
		points += transcendentStatusPoints
	}
	p.progress.StatusPoints = points
	p.char.Stats = status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}
}

// resetStat is player:reset_stat: one stat goes down to a value, 1 by
// default, and refunds what raising it cost, by rAthena's cost table
// (PC_STATUS_POINT_COST through pc_need_status_point). It returns false
// when the stat is not above the value.
func (z *Zone) resetStat(p *Player, st status.Stat, to int) bool {
	if z.Status == nil || z.Progress == nil || to < 1 {
		return false
	}
	cur := statValue(&p.char.Stats, st)
	if cur == nil || *cur <= to {
		return false
	}
	refund := z.Status.NeedStatusPoint(&p.char, st, -(*cur - to))
	*cur = to
	p.progress.StatusPoints += refund
	z.afterJobChange(p)
	return true
}

// statValue returns a raised stat of a character.
func statValue(s *status.Stats, st status.Stat) *int {
	switch st {
	case status.Str:
		return &s.Str
	case status.Agi:
		return &s.Agi
	case status.Vit:
		return &s.Vit
	case status.Int:
		return &s.Int
	case status.Dex:
		return &s.Dex
	case status.Luk:
		return &s.Luk
	}
	return nil
}

// NextJobs is jobs.next_jobs for scripts.
func (h scriptHost) NextJobs(job int) []int { return NextJobs(h.z.Status, job) }

// ChangeJob is player:change_job for scripts.
func (h scriptHost) ChangeJob(id int64, job int, force bool) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.changeJob(p, job, force), nil
}

// Rebirth is player:rebirth for scripts.
func (h scriptHost) Rebirth(id int64) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.rebirth(p), nil
}

// ResetSkills is player:reset_skills for scripts.
func (h scriptHost) ResetSkills(id int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	if h.z.skillsReady() {
		h.z.resetSkills(p)
	}
	return nil
}

// ResetStats is player:reset_stats for scripts.
func (h scriptHost) ResetStats(id int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	if h.z.Status != nil {
		h.z.resetStats(p)
	}
	return nil
}

// ResetSkill is player:reset_skill for scripts.
func (h scriptHost) ResetSkill(id int64, skill, toLevel int, withDependents bool) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	if !h.z.skillsReady() {
		return false, nil
	}
	return h.z.resetSkill(p, skill, toLevel, withDependents), nil
}

// ResetStat is player:reset_stat for scripts.
func (h scriptHost) ResetStat(id int64, stat, to int) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.resetStat(p, status.Stat(stat), to), nil
}

// novice is JOB_NOVICE.
const novice = 0

// checkJob loads a character whose job the world no longer has, such as
// the job of a removed mod, as a Novice, and logs a warning. Its job
// level drops to the Novice's highest, with no job exp past it. Its
// skills, stats and items stay.
func (z *Zone) checkJob(c *character.Character) {
	if z.Status == nil {
		return
	}
	if _, ok := z.Status.Info(c.Job); ok {
		return
	}
	z.warn("character loaded as a Novice: the world has no such job", "character", c.Name, "job", c.Job)
	c.Job = novice
	if top := z.Status.MaxJobLevel(novice); top > 0 && c.JobLevel >= top {
		c.JobLevel, c.JobExp = top, 0
	}
}
