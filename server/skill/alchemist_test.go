package skill

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
)

func init() {
	// rAthena's 50 rolls hit 18 times here. Its other nine levels on
	// Baphomet hit 98 times in 450 rolls (22%), and the hit rate does not
	// depend on the level. 18 of 50 is 2.8 standard deviations above it.
	rollGaps["renewal: CR_ACIDDEMONSTRATION lv 9 (job 4019, weapon 0) on 1039"] = "statistical outlier of the recording"
}

// TestPitcherRequirement checks skill_get_requirement's rule for
// AM_POTIONPITCHER, CR_SLIMPITCHER and CR_CULTIVATION: only the item of
// the level (skill_lv%11 - 1) is used.
func TestPitcherRequirement(t *testing.T) {
	db := loadDB(t, reference.PreRenewal)
	st := &battle.Status{Player: &battle.Player{}}
	c := &Caster{Status: st}
	for _, tc := range []struct {
		id, lv int
		want   [][2]int
	}{
		{IDPotionPitcher, 1, [][2]int{{501, 1}}},
		{IDPotionPitcher, 5, [][2]int{{505, 1}}},
		{IDSlimPitcher, 6, [][2]int{{546, 1}}},
		{IDSlimPitcher, 10, [][2]int{{547, 1}}},
		{IDCultivation, 2, [][2]int{{905, 1}}},
		{IDCallHomunculus, 1, [][2]int{{7142, 1}}},
	} {
		if got := db.Get(tc.id).RequirementFor(c, tc.lv, 1, 1).Items; !slices.Equal(got, tc.want) {
			t.Errorf("skill %d lv %d: items %v, want %v", tc.id, tc.lv, got, tc.want)
		}
	}
}

// TestAcidDemonstrationPlayer checks the halving against a player of
// battle_calc_misc_attack's CR_ACIDDEMONSTRATION (pre-renewal) and of
// SkillAcidDemonstration::calculateSkillRatio (renewal).
func TestAcidDemonstrationPlayer(t *testing.T) {
	src := &battle.Status{Int: 99}
	mob := &battle.Status{Vit: 40}
	pc := &battle.Status{Vit: 40, Player: &battle.Player{}}
	ctx := func(target *battle.Status, renewal bool) *battle.SkillContext {
		return &battle.SkillContext{Src: src, Target: target, Skill: &battle.Skill{ID: IDAcidDemonstration, Level: 5},
			Renewal: renewal}
	}
	// 7 * 40 * 99 * 99 / (10 * 139) = 1974.
	if got := (acidDemonstration{}).BaseDamage(ctx(mob, false), nil); got != 1974 {
		t.Errorf("monster: %d, want 1974", got)
	}
	if got := (acidDemonstration{}).BaseDamage(ctx(pc, false), nil); got != 987 {
		t.Errorf("player: %d, want 987", got)
	}
	ratio := 100
	(acidDemonstration{}).SkillRatio(ctx(pc, true), &ratio)
	// (100 - 100 + 1000 + 99 + 40) / 2.
	if ratio != 569 {
		t.Errorf("renewal ratio on a player: %d, want 569", ratio)
	}
}
