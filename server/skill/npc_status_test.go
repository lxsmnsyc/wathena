package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// TestNPCStatusAttackHitRate checks modifyHitRate of the status attacks,
// as SkillBlindAttack's: 20% more, rounded down.
func TestNPCStatusAttackHitRate(t *testing.T) {
	hit := 87
	(npcStatusAttack{}).HitRate(nil, &hit)
	if hit != 104 {
		t.Errorf("hit rate %d, want 104", hit)
	}
	for _, id := range []int{idNPCPoison, idNPCBlindAttack, idNPCSilenceAttack, idNPCStunAttack, idNPCPetrifyAttack,
		idNPCCurseAttack, idNPCSleepAttack, idNPCBleeding, idNPCBleeding2, idNPCWideSuck} {
		if !Ported(id) {
			t.Errorf("skill %d is not registered", id)
		}
	}
}

// TestNPCWideSuckDamage checks NPC_WIDESUCK's case of
// battle_calc_misc_attack: 15% of the target's max HP, which no
// recording shows, since skill_db gives the skill no unit.
func TestNPCWideSuckDamage(t *testing.T) {
	a := &battle.SkillContext{Target: &battle.Status{MaxHP: 12345}, Skill: &battle.Skill{ID: idNPCWideSuck, Level: 1}}
	if got := (npcWideSuck{}).BaseDamage(a, nil); got != 1851 {
		t.Errorf("damage %d, want 1851", got)
	}
}
