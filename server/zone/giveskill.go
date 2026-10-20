package zone

import (
	"slices"

	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/social"
)

// player:give_skill, as rAthena's skill command and pc_skill (review
// decision 57).

// giveSkill is pc_skill. A permanent level is learned and saved, outside
// the skill tree and the skill points, and level 0 removes the skill. The
// client gets the skill list again, as clif_addskill and clif_deleteskill,
// and a passive skill computes the stats again, as status_calc_pc.
//
// A temporary level sets the skill's level until the player's skills are
// recalculated (tempSkills). The client gets nothing and the stats are
// not computed again, as in rAthena. It returns false for a skill the
// skill data does not have, and for a guild skill.
func (z *Zone) giveSkill(p *Player, id, level int, kind modscript.SkillKind) bool {
	if z.Skills == nil || social.IsGuildSkill(id) {
		return false
	}
	d := z.Skills.Get(id)
	if d == nil {
		return false
	}
	switch kind {
	case modscript.SkillPermanent, modscript.SkillPermanentGrant:
		// The level replaces a temporary one, as the flag goes back to
		// SKILL_FLAG_PERMANENT, or SKILL_FLAG_PERM_GRANTED for
		// permanent_grant (pc.cpp 5435), which a skill reset keeps.
		delete(p.tempSkills, id)
		p.granted = slices.DeleteFunc(p.granted, func(g int) bool { return g == id })
		if kind == modscript.SkillPermanentGrant && level > 0 {
			p.granted = append(p.granted, id)
		}
		if level == 0 {
			delete(p.char.Skills, id)
		} else {
			if p.char.Skills == nil {
				p.char.Skills = map[int]int{}
			}
			p.char.Skills[id] = level
		}
		z.passiveChanged(p, id)
		if d.Passive() {
			z.markStats(p)
			z.flushStats()
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		}
		z.sendSkillList(p)
	case modscript.SkillTemporary:
		z.setTempSkill(p, id, max(p.skillLevel(id), level))
	case modscript.SkillTemporaryAdd:
		z.setTempSkill(p, id, min(p.skillLevel(id)+level, modscript.MaxGivenSkillLevel))
	default:
		return false
	}
	return true
}

// setTempSkill sets a skill's temporary level, which skillLevel reads.
func (z *Zone) setTempSkill(p *Player, id, level int) {
	if level <= 0 {
		return
	}
	if p.tempSkills == nil {
		p.tempSkills = map[int]int{}
	}
	p.tempSkills[id] = level
}

// GiveSkill is player:give_skill for scripts.
func (h scriptHost) GiveSkill(id int64, skill, level int, kind modscript.SkillKind) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.giveSkill(p, skill, level, kind), nil
}
