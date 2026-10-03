package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// TestNpcWeaponFormulas checks the calculateSkillRatio and modifyHitRate
// methods of the monster weapon attacks' SkillImpl classes, from a base
// ratio of 100 and a hit rate of 80.
func TestNpcWeaponFormulas(t *testing.T) {
	for _, c := range []struct {
		name       string
		id, lv     int
		ratio, hit int
	}{
		// SkillFireAttributeAttack and the other element attacks: 100% per
		// level after the first, and 20% more hit.
		{"NPC_FIREATTACK", idNpcFireAttack, 1, 100, 96},
		{"NPC_GROUNDATTACK", idNpcGroundAttack, 3, 300, 96},
		{"NPC_UNDEADATTACK", idNpcUndeadAttack, 10, 1000, 96},
		// SkillMultiStageAttack: 25% per level.
		{"NPC_COMBOATTACK", idNpcComboAttack, 4, 200, 80},
		// SkillPiercingAttack: 75%.
		{"NPC_PIERCINGATT", idNpcPiercingAtt, 5, 75, 80},
		// SkillRandomAttack, SkillSuckingBlood and SkillPulseStrike.
		{"NPC_RANDOMATTACK", idNpcRandomAttack, 2, 200, 80},
		{"NPC_BLOODDRAIN", idNpcBloodDrain, 1, 100, 80},
		{"NPC_PULSESTRIKE", idNpcPulseStrike, 5, 500, 80},
		// SkillEnergyDrain: 100% per level.
		{"NPC_ENERGYDRAIN", idNpcEnergyDrain, 1, 200, 80},
		// SkillSpiritDestruction: 20% more hit.
		{"NPC_MENTALBREAKER", idNpcMentalBreaker, 1, 100, 96},
		// SkillPulseStrike2: 100% more.
		{"NPC_PULSESTRIKE2", idNpcPulseStrike2, 1, 200, 80},
		// WeaponSkillImpl.
		{"NPC_CRITICALSLASH", idNpcCriticalSlash, 1, 100, 80},
	} {
		impl := registry[c.id]
		if impl == nil {
			t.Errorf("%s: not registered", c.name)
			continue
		}
		a := &battle.SkillContext{Skill: &battle.Skill{ID: c.id, Level: c.lv}}
		ratio, hit := 100, 80
		impl.SkillRatio(a, &ratio)
		impl.HitRate(a, &hit)
		if ratio != c.ratio || hit != c.hit {
			t.Errorf("%s level %d: ratio %d, hit %d, want %d and %d", c.name, c.lv, ratio, hit, c.ratio, c.hit)
		}
	}
}
