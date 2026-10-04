package zone

import (
	"maps"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/social"
)

// Skills: a player's learned skills, its skill list and skill points, as
// rAthena's pc_calc_skilltree, pc_skillup and clif_skillinfoblock. The
// casting is in cast.go, the hits in skillattack.go, ground skills in
// skillunits.go and each skill's behavior in skillimpls.go.

// unitSkill is what an entity has of rAthena's unit_data for skills.
type unitSkill struct {
	// cast is the skill being cast (ud->skilltimer), or nil.
	cast *casting
	// canAct is canact_tick: no skill until then. canMove is
	// canmove_tick: no walking until then. endure is endure_tick: hits do
	// not stop the unit until then.
	canAct, canMove, endure int64
	// tickset is skillunittick: when each ground skill group, by group ID,
	// or skill ID for UF_NOOVERLAP, may hit the unit again.
	tickset map[int64]int64
	// groups are the ground skills the unit placed (ud->skillunits).
	groups []*unitGroup
	// pending is a skill to cast once a walk brings the unit in range, as
	// rAthena's stepaction.
	pending *pendingSkill
	// sgCounter is status_change's sg_counter: the Storm Gust hits the
	// unit took in a row, in pre-renewal.
	sgCounter int
	// comet is status_change's comet_x and comet_y: the cell of the
	// unit's last NPC_COMET, or nil before its first.
	comet *[2]int
}

// skillLevel is pc_checkskill: the level of a skill the player learned,
// or the level an item grants when it is higher.
func (p *Player) skillLevel(id int) int {
	if social.IsGuildSkill(id) {
		return p.guildSkillLevel(id)
	}
	lv := p.char.Skills[id]
	if st := p.entity.status; st != nil && st.Player != nil {
		lv = max(lv, st.Player.Skills[id])
	}
	// The skill Plagiarism copied.
	if id == p.rogue.cloneID {
		lv = max(lv, p.rogue.cloneLv)
	}
	return lv
}

// learner is what the skill tree reads of a player.
func (z *Zone) learner(p *Player) *skill.Learner {
	l := &skill.Learner{Job: p.char.Job, Male: p.char.Male, BaseLevel: 1, JobLevel: 1, Learned: p.char.Skills,
		ChangeLevel2nd: p.record.JobChangeLevel}
	if z.Progress != nil {
		l.BaseLevel, l.JobLevel = p.progress.BaseLevel, p.progress.JobLevel
	}
	return l
}

// skillsReady reports whether the zone has skill data and job tables.
func (z *Zone) skillsReady() bool {
	return z.Skills != nil && z.SkillTrees != nil && z.Status != nil
}

// skillList is clif_skillinfoblock: the player's skill list, or nil
// without skill data.
func (z *Zone) skillList(p *Player) *wathenav1.Event {
	if !z.skillsReady() {
		return nil
	}
	entries, err := z.SkillTrees.List(z.Skills, z.Status, z.learner(p), z.SkillSettings)
	if err != nil {
		return nil
	}
	tree := z.SkillTrees.Job(p.char.Job)
	var infos []*wathenav1.SkillInfo
	for _, e := range entries {
		d := z.Skills.Get(e.Skill)
		if d == nil {
			continue
		}
		lv := max(e.Level, 1)
		info := wathenav1.SkillInfo_builder{
			SkillId:     uint32(d.ID),                                              // #nosec G115 -- skill IDs are positive.
			Level:       uint32(e.Level),                                           // #nosec G115 -- a small level.
			MaxLevel:    uint32(z.SkillTrees.MaxLevel(z.Skills, d.ID, p.char.Job)), // #nosec G115 -- as above.
			Name:        d.Name,
			DisplayName: d.DisplayName,
			Target:      skillTarget(d.Target),
			Sp:          uint32(max(d.Requires.SP.At(lv), 0)),            // #nosec G115 -- checked.
			Range:       uint32(max(d.RangeFor(p.entity.status, lv), 0)), // #nosec G115 -- checked.
			Upgradable:  e.Upgradable,
			Icon:        uint32(d.ID), // #nosec G115 -- as above.
		}
		if d.Icon != 0 {
			info.Icon = uint32(d.Icon) // #nosec G115 -- a skill ID.
		}
		if ts := tree.Skill(d.ID); ts != nil {
			for _, r := range ts.Requires {
				info.Requires = append(info.Requires, wathenav1.SkillRequirement_builder{
					SkillId: uint32(r[0]), Level: uint32(r[1]), // #nosec G115 -- skill IDs and levels.
				}.Build())
			}
		}
		infos = append(infos, info.Build())
	}
	if ci := z.copiedSkillInfo(p); ci != nil {
		infos = append(infos, ci)
	}
	return wathenav1.Event_builder{SkillList: wathenav1.SkillList_builder{Skills: infos}.Build()}.Build()
}

// skillTarget is a skill's target type in the protocol.
func skillTarget(t skill.Target) wathenav1.SkillTarget {
	switch t {
	case skill.TargetEnemy:
		return wathenav1.SkillTarget_SKILL_TARGET_ENEMY
	case skill.TargetGround:
		return wathenav1.SkillTarget_SKILL_TARGET_GROUND
	case skill.TargetSelf:
		return wathenav1.SkillTarget_SKILL_TARGET_SELF
	case skill.TargetSupport:
		return wathenav1.SkillTarget_SKILL_TARGET_SUPPORT
	case skill.TargetTrap:
		return wathenav1.SkillTarget_SKILL_TARGET_TRAP
	}
	return wathenav1.SkillTarget_SKILL_TARGET_PASSIVE
}

// sendSkillList queues the player's skill list for its next frame.
func (z *Zone) sendSkillList(p *Player) {
	if ev := z.skillList(p); ev != nil {
		p.pending = append(p.pending, ev)
	}
}

// learnSkill is pc_skillup: a skill point raises a skill of the
// player's list, below its maximum level in the job's tree. A passive
// skill changes the player's stats at once. The list goes out again. A
// skill-up that fails does nothing, as in rAthena.
func (z *Zone) learnSkill(p *Player, id int) {
	if !z.skillsReady() || z.Progress == nil {
		return
	}
	if !z.SkillTrees.SkillUp(z.Skills, z.Status, z.learner(p), z.SkillSettings, p.progress.SkillPoints, id) {
		return
	}
	before := z.progressStats(p, nil)
	if p.char.Skills == nil {
		p.char.Skills = map[int]int{}
	}
	p.char.Skills[id]++
	p.progress.SkillPoints--
	if d := z.Skills.Get(id); d != nil && d.Passive() {
		// status_calc_pc for a passive skill.
		z.markStats(p)
		z.flushStats()
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
	}
	z.passiveChanged(p, id)
	if s := z.progressStats(p, before); s != nil {
		p.pending = append(p.pending, s)
	}
	z.sendSkillList(p)
}

// learnedSkills is a copy of the skills a player learned, for saving.
func learnedSkills(p *Player) map[int]int {
	if len(p.char.Skills) == 0 {
		return nil
	}
	out := make(map[int]int, len(p.char.Skills))
	for id, lv := range p.char.Skills {
		if lv > 0 {
			out[id] = lv
		}
	}
	return out
}

// sortedSkills lists skill IDs in order, for code that must not depend
// on map order.
func sortedSkills(m map[int]int) []int { return slices.Sorted(maps.Keys(m)) }
