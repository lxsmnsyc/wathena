package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// TestSoulSiphonDamage checks Soul Siphon's case of
// battle_calc_magic_attack: twice the target's SP, which the recordings
// do not show, since monsters have 1 SP there.
func TestSoulSiphonDamage(t *testing.T) {
	a := &battle.SkillContext{Target: &battle.Status{SP: 150}, Skill: &battle.Skill{ID: IDSoulBurn, Level: 5}}
	if got := (soulSiphon{}).BaseDamage(a, nil); got != 300 {
		t.Errorf("damage %d, want 300", got)
	}
	for _, id := range []int{IDAdvancedBook, IDFreeCast, IDDragonology, IDSoulBurn, IDDoubleCasting} {
		if !Ported(id) {
			t.Errorf("skill %d is not registered", id)
		}
	}
	if Ported(IDAbracadabra) || Ported(IDCreateCon) {
		t.Error("Hocus-pocus and Create Elemental Converter wait for their data and window")
	}
}
