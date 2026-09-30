package battle

// Battle settings that normal attacks read, with the values rAthena's
// conf/battle files set by default. The recorder used these defaults.
const (
	// minHitrate and maxHitrate bound the chance to hit, in percent
	// (min_hitrate and max_hitrate).
	minHitrate = 5
	maxHitrate = 100
	// weaponDefenseType is weapon_defense_type. When not 0, hard DEF is
	// added to soft DEF this many times instead of reducing damage by a
	// percent.
	weaponDefenseType = 0
	// monsterAttackAttrNone is true because attack_attr_none (14) includes
	// monsters: their normal attacks ignore the element table.
	monsterAttackAttrNone = true
	// attrRecover is attribute_recover. When false, negative element
	// percents count as 0.
	attrRecover = false
	// skillMinDamage is skill_min_damage. When its bit for weapon attacks
	// (1) is set, damage below the hit count is raised to the hit count.
	skillMinDamage = 0
)

// The settings below only apply to cases that Attack does not handle.
//   - agi_penalty_target and vit_penalty_target are players only, so the
//     flee and defense penalties for many attackers never apply to a
//     monster target.
//   - ksprotection, pk_mode and the GvG and battleground damage rates do
//     not apply on the recorder's map, prontera.
