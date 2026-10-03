package statuschange

// Statuses of the monster self skills (NPC_DEFENDER, NPC_STONESKIN,
// NPC_ANTIMAGIC, NPC_MAGICMIRROR, NPC_INVINCIBLE, NPC_MILLENNIUMSHIELD
// and NPC_DAMAGE_HEAL) that setup.go reads, which ids.go does not name.
const (
	scArmor            = 104 // SC_ARMOR
	scArmorchange      = 252 // SC_ARMORCHANGE
	scMagicmirror      = 254 // SC_MAGICMIRROR
	scMillenniumshield = 314 // SC_MILLENNIUMSHIELD
	scDamageHeal       = 916 // SC_DAMAGE_HEAL

	// skillNpcAntimagic is NPC_ANTIMAGIC, which SC_ARMORCHANGE's val2
	// names.
	skillNpcAntimagic = 676

	// npcBfLong is BF_LONG, the range that SC_ARMOR keeps.
	npcBfLong = 0x0040
)
