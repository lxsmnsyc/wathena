package status

// Statuses of the monster self skills that change stats and attacks,
// which server/statuschange does not name.
const (
	scArmor       = 104 // SC_ARMOR, from NPC_DEFENDER
	scArmorchange = 252 // SC_ARMORCHANGE, from NPC_STONESKIN and NPC_ANTIMAGIC
	scPowerup     = 951 // SC_POWERUP, from NPC_POWERUP
	scAgiup       = 952 // SC_AGIUP, from NPC_AGIUP
)
