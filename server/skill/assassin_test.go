package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// TestAssassinLevelRatio checks RE_LVL_DMOD(100) in
// SkillSoulDestroyer::calculateSkillRatio and
// SkillMeteorAssault::calculateSkillRatio: above base level 99 the
// renewal ratio grows with the level, which the recordings (made at
// level 99) do not show. Pre-renewal has no such rule.
func TestAssassinLevelRatio(t *testing.T) {
	src := &battle.Status{Level: 150, Str: 100, Int: 50}
	for _, c := range []struct {
		name    string
		impl    battle.SkillImpl
		renewal bool
		want    int
	}{
		// (100 - 100 + 150*10 + 100 + 50) * 150 / 100.
		{"ASC_BREAKER renewal", soulBreaker{}, true, 1650 * 150 / 100},
		{"ASC_BREAKER pre-renewal", soulBreaker{}, false, 1000},
		// (100 + 100 + 120*10) * 150 / 100.
		{"ASC_METEORASSAULT renewal", meteorAssault{}, true, 1400 * 150 / 100},
		{"ASC_METEORASSAULT pre-renewal", meteorAssault{}, false, 440},
	} {
		ratio := 100
		a := &battle.SkillContext{Src: src, Target: &battle.Status{MaxHP: 100}, Skill: &battle.Skill{Level: 10}, Renewal: c.renewal}
		c.impl.SkillRatio(a, &ratio)
		if ratio != c.want {
			t.Errorf("%s: ratio %d, want %d", c.name, ratio, c.want)
		}
	}
}

// TestSonicBlowLowHP checks SkillSonicBlow::calculateSkillRatio in
// renewal: a target below half its HP takes half again.
func TestSonicBlowLowHP(t *testing.T) {
	a := &battle.SkillContext{Src: &battle.Status{}, Target: &battle.Status{MaxHP: 1000, HP: 499},
		Skill: &battle.Skill{Level: 10}, Renewal: true}
	ratio := 100
	sonicBlow{}.SkillRatio(a, &ratio)
	if want := 1200 + 600; ratio != want {
		t.Errorf("ratio %d, want %d", ratio, want)
	}
	a.Target.HP = 500
	ratio = 100
	sonicBlow{}.SkillRatio(a, &ratio)
	if ratio != 1200 {
		t.Errorf("ratio %d at half HP, want 1200", ratio)
	}
}
