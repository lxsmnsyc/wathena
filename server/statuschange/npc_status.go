package statuschange

// Statuses of the monster skills that cause statuses, which ids.go has
// no constants for.
const (
	scNPCHallucinationWalk = 758 // SC_NPC_HALLUCINATIONWALK
	scAllStatDown          = 914 // SC_ALL_STAT_DOWN
)

// npcAllStatDownMax is skill_get_max(NPC_ALL_STAT_DOWN), from renewal's
// skill_db.yml. SC_ALL_STAT_DOWN's setup reads it.
const npcAllStatDownMax = 5
