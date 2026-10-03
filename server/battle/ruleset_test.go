package battle

import (
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/rules/rulestest"
)

// withElements returns the core rules of a base with element_table set.
func withElements(base, elements bool) *rules.Rules {
	rl := rules.Base(base)
	rl.ElementTable = elements
	return rl
}

// TestElementTableFlag checks both values of the element_table flag on
// both bases:
//   - ElementFix gives every cell of the flag's mode's elements.json, on
//     either base.
//   - A magic attack picks the table of the flag, not of its base: a
//     water bolt on a holy level 4 target deals damage with renewal's
//     table (100%) and none with pre-renewal's (0%).
func TestElementTableFlag(t *testing.T) {
	for _, mode := range reference.Modes {
		var ref reference.Elements
		if err := reference.Load(mode, "elements", &ref); err != nil {
			t.Fatal(err)
		}
		flag := mode == reference.Renewal
		for _, base := range []bool{true, false} {
			rl := withElements(base, flag)
			cells := 0
			for _, lv := range ref.Levels {
				for atk, row := range lv.Table {
					for def, pct := range row {
						// flag 1 keeps negative percents, as attribute_recover.
						want := int64(1000)
						if flag {
							want -= 1000 * (100 - int64(pct)) / 100
						} else {
							want = 1000 * int64(pct) / 100
						}
						if got := ElementFix(rl, 1000, atk, def, lv.Level, 1); got != want {
							t.Errorf("base renewal %v, element_table %s: level %d, %d on %d: %d, want %d", base, mode, lv.Level, atk, def, got, want)
						}
						cells++
					}
				}
			}
			if cells != maxElementLevel*elementCount*elementCount {
				t.Errorf("%s: %d cells", mode, cells)
			}
		}
	}
	src := &Status{Level: 99, Int: 99, Dex: 99, Luk: 1, MAtkMin: 500, MAtkMax: 500, Hit: 500, EleLv: 1}
	holy := &Status{Level: 1, MaxHP: 100000, DefEle: Holy, EleLv: 4, Size: SizeMedium}
	for _, base := range []bool{true, false} {
		for _, elements := range []bool{true, false} {
			sk := &Skill{ID: 19, Level: 1, Hits: 1, Element: Water, Range: 9, Rules: withElements(base, elements)}
			got := SkillAttackRules(TypeMagic, src, holy, sk, forcedMin{})
			if hit := got.Damage > 0; hit != elements {
				t.Errorf("base renewal %v, element_table renewal %v: water on holy 4 dealt %d", base, elements, got.Damage)
			}
		}
	}
}

// TestMixedRulesetsBattle runs the battle recordings that apply to each
// sampled mixed ruleset (rulestest.Mixes), through AttackRules with the
// mix:
//   - mob_damage.json of the base, when element_table is the base's.
//   - pc_damage.json of the base, when element_table and size_table are
//     the base's.
//
// The other flags of the mix must not change normal attacks.
func TestMixedRulesetsBattle(t *testing.T) {
	ran := map[string]int{}
	pcs := map[reference.Mode]*pcFixture{}
	for i, rl := range rulestest.Mixes() {
		mode := reference.PreRenewal
		if rl.Renewal {
			mode = reference.Renewal
		}
		if rl.ElementTable != rl.Renewal {
			continue
		}
		attack := func(a, d *Status, r Rand) Result { return AttackRules(a, d, rl, r) }
		t.Run(fmt.Sprintf("%s mix %d mob_damage", mode, i), func(t *testing.T) {
			checkMobDamageForcedMin(t, mode, attack)
		})
		ran["mob_damage"]++
		if rl.SizeTable != rl.Renewal {
			continue
		}
		if pcs[mode] == nil {
			pcs[mode] = loadModePCFixture(t, mode)
		}
		f := *pcs[mode]
		f.attack = attack
		t.Run(fmt.Sprintf("%s mix %d pc_damage", mode, i), func(t *testing.T) {
			checkPCDamageForcedMin(t, &f)
		})
		ran["pc_damage"]++
	}
	t.Logf("recordings run with mixed rulesets: %v", ran)
	if ran["mob_damage"] == 0 || ran["pc_damage"] == 0 {
		t.Errorf("no mix ran a recording: %v", ran)
	}
}
