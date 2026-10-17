package zone

import (
	"slices"

	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Core's procedural NPCs (decision 0044) reach the world's quests, quest
// rewards, skill points and the refiner through wa.internal. These are
// the zone's side of those calls.

// WorldQuests lists the world's quests, for the quest boards and job
// guilds.
func (h scriptHost) WorldQuests() []modscript.CoreQuest {
	if h.z.Mods == nil {
		return nil
	}
	return slices.Clone(h.z.Mods.World)
}

// GiveExp gives a player a quest's exp, as rAthena's getexp: the guild
// tax, level ups and their effects follow as for a kill's exp.
func (h scriptHost) GiveExp(player, base, job int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if base > 0 || job > 0 {
		h.z.gainExp(p, base, job)
	}
	return nil
}

// SkillPoints is the player's unused skill points.
func (h scriptHost) SkillPoints(player int64) (int, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	return p.progress.SkillPoints, nil
}
