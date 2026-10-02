package battle

// The parts of the Assassin and Assassin Cross skills and statuses that
// battle.cpp computes: Poison React's damage boost, Enchant Deadly
// Poison, and the special cases of Sonic Blow, Venom Splasher and Soul
// Breaker in the functions they belong to.

// Skill IDs of the Assassin and Assassin Cross that battle reads, from
// rAthena's skill_db.yml.
const (
	skillSonicBlow         = 136  // AS_SONICBLOW
	skillGrimtooth         = 137  // AS_GRIMTOOTH
	skillPoisonReact       = 139  // AS_POISONREACT
	skillVenomSplasher     = 141  // AS_SPLASHER
	skillSonicAcceleration = 1003 // AS_SONICACCEL
	skillVenomKnife        = 1004 // AS_VENOMKNIFE
	skillSoulBreaker       = 379  // ASC_BREAKER
	skillSandAttack        = 149  // TF_SPRINKLESAND
)

// poisonReactRatio is Poison React's part of
// battle_calc_attack_skill_ratio: in its damage boost mode, a normal
// attack (or Auto Counter) gains 30% per level. A player's level is the
// one it learned. The zone starts the poison and ends the status.
func poisonReactRatio(wd *damage, src *Status) int {
	const autoCounter = 61 // KN_AUTOCOUNTER
	c := src.Changes
	if c == nil || c.PoisonReact <= 0 || wd.sk != nil && wd.skillID() != autoCounter {
		return 0
	}
	if src.Player != nil {
		return 30 * src.skill(skillPoisonReact)
	}
	return 30 * c.PoisonReact
}

// edpBonus is renewal's SC_EDP part of battle_attack_sc_bonus: weapon and
// equipment ATK times 250% plus 30% per level. Sand Attack, Venom
// Splasher, Meteor Assault, Grimtooth and Venom Knife take no bonus.
func edpBonus(wd *damage, src *Status) {
	c := src.Changes
	if c == nil || c.EDP <= 0 {
		return
	}
	switch wd.skillID() {
	case skillSandAttack, skillVenomSplasher, skillMeteorAssault, skillGrimtooth, skillVenomKnife:
		return
	}
	rate := int64(250 + c.EDP*30)
	wd.atkRate2(src, &wd.weaponAtk, &wd.weaponAtk2, rate, rate)
	wd.atkRate2(src, &wd.equipAtk, &wd.equipAtk2, rate, rate)
}
