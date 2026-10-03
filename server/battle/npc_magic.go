package battle

// The cases of battle_calc_magic_attack for monster skills of
// server/skill/npc_magic.go whose base damage needs the unexported
// parts of this package.

// Monster skill IDs that magic.go reads.
const (
	skillGrandDarkness = 339 // NPC_GRANDDARKNESS
	skillNPCEarthquake = 653 // NPC_EARTHQUAKE
	// skillNPCDragonBreath takes no masteries
	// (battle_skill_stacks_masteries_vvs).
	skillNPCDragonBreath = 731 // NPC_DRAGONBREATH
	// npcEarthquakeFlag is NPC_EARTHQUAKE_FLAG: the mflag bit of the first
	// hit of an Earthquake, which battle_calc_damage reads for Dodge and
	// Kaupe. Neither status is ported.
	npcEarthquakeFlag = 0x800
)

// npcEarthquakeDamage is the NPC_EARTHQUAKE case of
// battle_calc_magic_attack. rAthena notes that it is only accurate for
// pre-renewal.
//   - A player's base is its STR times 2 plus a weapon attack in renewal,
//     and its base and weapon ATK in pre-renewal. Players do not cast it,
//     so the zone never reaches that part.
//   - Another unit's base is battle_calc_base_damage, with its ATK
//     percent in pre-renewal.
//
// Then 200% + 100% per level + 100% per 2 levels + 100% above level 4,
// split among the targets.
func npcEarthquakeDamage(ad *damage, src, target *Status, pre bool, r Rand) int64 {
	if ad.miscflag&npcEarthquakeFlag != 0 {
		ad.flag |= npcEarthquakeFlag
		ad.miscflag &^= npcEarthquakeFlag
	}
	lv := int64(ad.sk.Level)
	var dmg int64
	switch {
	case src.Player != nil && pre:
		dmg = int64(src.BAtk + src.RHW.Atk)
	case src.Player != nil:
		wd := calcWeaponAttack(src, target, ad.sk, nil, r)
		dmg = int64(src.Str*2) + wd.damage
	case pre:
		dmg = calcBaseDamagePreRenewal(src, handRight, target.Size, 0, r)
		dmg = dmg * atkPercentPreRenewal(src, ad.sk) / 100
	default:
		dmg = calcBaseDamage(src, &src.RHW, target.Size, 0, r)
	}
	ratio := 200 + 100*lv + 100*(lv/2)
	if lv > 4 {
		ratio += 100
	}
	dmg = dmg * ratio / 100
	if ad.nk.Has(NKSplashSplit) && ad.miscflag > 1 {
		dmg /= int64(ad.miscflag)
	}
	return dmg
}
