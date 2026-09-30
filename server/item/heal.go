package item

// Skill IDs pc_itemheal reads, from rAthena's skill_db.yml.
const (
	SkillIncHPRecovery  = 4   // SM_RECOVERY
	SkillIncSPRecovery  = 9   // MG_SRECOVERY
	SkillLearningPotion = 227 // AM_LEARNINGPOTION
)

// Roll runs the heal script's arguments: an amount that is a range draws
// rand(min, max), as BUILDIN_FUNC(rand) with rnd_value. The script reads
// HP before SP, so HP draws first. A fixed amount draws nothing.
func (h *Heal) Roll(r Rand) (hp, sp int) {
	roll := func(a [2]int) int {
		if a[0] == a[1] {
			return a[0]
		}
		return a[0] + r.IntN(a[1]-a[0]+1)
	}
	hp = roll(h.HP)
	sp = roll(h.SP)
	return hp, sp
}

// ItemHeal is pc_itemheal for a player with no equipment, status changes,
// or ranked potions. Each point of VIT adds 2% to HP heals, and each
// point of INT adds 2% to SP heals. Increase HP Recovery adds 10% per
// level to HP, Increase SP Recovery 10% per level to SP, and Learning
// Potion 5% per level to both. The result is what status_heal gets,
// before it caps the heal at the missing HP and SP.
func ItemHeal(hp, sp, vit, int_ int, skills map[int]int) (int, int) {
	if hp != 0 {
		bonus := 100 + vit*2 + skills[SkillIncHPRecovery]*10 + skills[SkillLearningPotion]*5
		// The overflow check keeps the heal when the bonus shrinks it.
		if tmp := hp * bonus / 100; bonus != 100 && tmp > hp {
			hp = tmp
		}
	}
	if sp != 0 {
		bonus := 100 + int_*2 + skills[SkillIncSPRecovery]*10 + skills[SkillLearningPotion]*5
		if tmp := sp * bonus / 100; bonus != 100 && tmp > sp {
			sp = tmp
		}
	}
	return hp, sp
}

// PercentHeal is pc_percentheal for a heal, through status_percent_heal
// and status_percent_change: a percent of max HP and max SP, from 0 to
// 100. A rate that heals less than 1 heals 1.
func PercentHeal(hpRate, spRate, maxHP, maxSP int) (int, int) {
	part := func(rate, maxValue int) int {
		rate = min(rate, 100)
		if rate <= 0 {
			return 0
		}
		v := maxValue
		if rate < 100 {
			v = applyRate(maxValue, rate)
		}
		return max(v, 1)
	}
	return part(hpRate, maxHP), part(spRate, maxSP)
}

// applyRate is rAthena's apply_rate macro.
func applyRate(v, rate int) int {
	switch {
	case rate == 100:
		return v
	case v > 100000:
		return v / 100 * rate
	}
	return v * rate / 100
}

// Cap is status_heal's limit: a heal fills at most the missing HP or SP.
// A negative heal, which rAthena turns into damage, heals nothing here.
func Cap(heal, current, maxValue int64) int64 {
	if heal <= 0 {
		return 0
	}
	return min(heal, max(maxValue-current, 0))
}
