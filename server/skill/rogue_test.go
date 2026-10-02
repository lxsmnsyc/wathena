package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// rogueContext is the SkillContext of a player with a weapon type.
func rogueContext(id, lv, weapon int, renewal bool) *battle.SkillContext {
	src := &battle.Status{Player: &battle.Player{Weapon: weapon, RightWeapon: weapon}}
	return &battle.SkillContext{Src: src, Target: &battle.Status{}, Skill: &battle.Skill{ID: id, Level: lv},
		Renewal: renewal}
}

// TestBackStabBowPenalty checks SkillBackStab::calculateSkillRatio:
// 300% + 40% per level, half of the bonus with a bow
// (backstab_bow_penalty).
func TestBackStabBowPenalty(t *testing.T) {
	for _, c := range []struct {
		weapon, lv, want int
	}{
		{battle.WeaponDagger, 1, 340},
		{battle.WeaponDagger, 10, 700},
		{battle.WeaponBow, 1, 220},
		{battle.WeaponBow, 10, 400},
	} {
		ratio := 100
		backStab{}.SkillRatio(rogueContext(IDBackStab, c.lv, c.weapon, true), &ratio)
		if ratio != c.want {
			t.Errorf("weapon %d lv %d: ratio %d, want %d", c.weapon, c.lv, ratio, c.want)
		}
	}
}

// TestBackStabDaggerHitsTwice checks SkillBackStab::modifyDamageData: a
// dagger hits twice in renewal only, and other weapons once.
func TestBackStabDaggerHitsTwice(t *testing.T) {
	for _, c := range []struct {
		weapon  int
		renewal bool
		want    int
	}{
		{battle.WeaponDagger, true, 2},
		{battle.WeaponDagger, false, 1},
		{battle.Weapon1HSword, true, 1},
	} {
		d := battle.DamageData{Div: 1}
		backStab{}.ModifyDamageData(rogueContext(IDBackStab, 5, c.weapon, c.renewal), &d)
		if d.Div != c.want {
			t.Errorf("weapon %d renewal %v: %d hits, want %d", c.weapon, c.renewal, d.Div, c.want)
		}
	}
}

// TestBackStabHitRate checks SkillBackStab::modifyHitRate: renewal adds
// the level.
func TestBackStabHitRate(t *testing.T) {
	hit := 80
	backStab{}.HitRate(rogueContext(IDBackStab, 7, battle.WeaponDagger, true), &hit)
	if hit != 87 {
		t.Errorf("renewal hit %d, want 87", hit)
	}
	hit = 80
	backStab{}.HitRate(rogueContext(IDBackStab, 7, battle.WeaponDagger, false), &hit)
	if hit != 80 {
		t.Errorf("pre-renewal hit %d, want 80", hit)
	}
}

// TestRaidAndIntimidateRatios checks SkillSightlessMind and SkillSnatch's
// calculateSkillRatio.
func TestRaidAndIntimidateRatios(t *testing.T) {
	ratio := 100
	raid{}.SkillRatio(rogueContext(IDRaid, 5, battle.WeaponDagger, true), &ratio)
	if ratio != 800 {
		t.Errorf("renewal Sightless Mind 5: %d, want 800", ratio)
	}
	ratio = 100
	raid{}.SkillRatio(rogueContext(IDRaid, 5, battle.WeaponDagger, false), &ratio)
	if ratio != 300 {
		t.Errorf("pre-renewal Sightless Mind 5: %d, want 300", ratio)
	}
	ratio = 100
	intimidate{}.SkillRatio(rogueContext(IDIntimidate, 5, battle.WeaponDagger, true), &ratio)
	if ratio != 250 {
		t.Errorf("Snatch 5: %d, want 250", ratio)
	}
}

// TestPlagiarismCopyFlags checks that the copy list follows skill_db's
// CopyFlags: Back Stab and Bash copy, Steal and Plagiarism do not.
func TestPlagiarismCopyFlags(t *testing.T) {
	for id, want := range map[int]bool{IDBackStab: true, IDBash: true, IDSteal: false, IDPlagiarism: false} {
		if CopyableByPlagiarism(id) != want {
			t.Errorf("skill %d: copyable %v, want %v", id, !want, want)
		}
	}
}
