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
