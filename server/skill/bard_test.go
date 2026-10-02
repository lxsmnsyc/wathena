package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
)

// TestVulcanArrowLevelRatio checks RE_LVL_DMOD(100) in
// SkillVulcanArrow::calculateSkillRatio, which the recordings (made at
// level 99) do not show.
func TestVulcanArrowLevelRatio(t *testing.T) {
	a := &battle.SkillContext{Src: &battle.Status{Level: 150}, Target: &battle.Status{MaxHP: 100},
		Skill: &battle.Skill{Level: 10}, Renewal: true}
	ratio := 100
	vulcanArrow{}.SkillRatio(a, &ratio)
	if want := (100 + 400 + 1000) * 150 / 100; ratio != want {
		t.Errorf("ratio %d, want %d", ratio, want)
	}
}

// TestDissonanceJobLevel checks SkillUnchainedSerenade's renewal ratio,
// which a player's job level scales.
func TestDissonanceJobLevel(t *testing.T) {
	a := &battle.SkillContext{Src: &battle.Status{Player: &battle.Player{JobLevel: 30}}, Target: &battle.Status{},
		Skill: &battle.Skill{Level: 5}, Renewal: true}
	ratio := 100
	unchainedSerenade{}.SkillRatio(a, &ratio)
	if want := (100 + 10 + 250) * 30 / 10; ratio != want {
		t.Errorf("ratio %d, want %d", ratio, want)
	}
}

// TestEncoreRequirement checks skill_get_requirement after Encore: the
// dance Encore repeats costs half its SP, and other skills cost theirs.
func TestEncoreRequirement(t *testing.T) {
	db := loadDB(t, reference.PreRenewal)
	st := &battle.Status{MaxSP: 1000, Player: &battle.Player{Cast: battle.CastBonus{SPRate: 100}}}
	whistle := db.Get(IDWhistle)
	full := whistle.RequirementFor(&Caster{Status: st}, 10, 1, 1000).SP
	if got := whistle.RequirementFor(&Caster{Status: st, Encore: IDWhistle}, 10, 1, 1000).SP; got != full/2 {
		t.Errorf("Encore: %d SP, want %d", got, full/2)
	}
	humming := db.Get(IDHumming)
	if got := humming.RequirementFor(&Caster{Status: st, Encore: IDWhistle}, 10, 1, 1000).SP; got != humming.Requires.SP.At(10) {
		t.Errorf("Encore halved another dance: %d SP", got)
	}
}
