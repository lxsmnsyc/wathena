package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
)

// battle_calc_misc_attack: in pre-renewal an automatic Blitz Beat (mflag
// above 1, the number of targets) splits its damage among its targets;
// a cast Blitz Beat (mflag 0) and renewal's do not.
func TestAutoBlitzBeatSplit(t *testing.T) {
	testrace.SkipUnderRace(t, "it loads the recorded damage data and runs on one goroutine")
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			f := loadDamageFixture(t, mode)
			var c *skillCase
			for i := range f.cases {
				if f.cases[i].SkillName == "HT_BLITZBEAT" && f.cases[i].Level == 5 && f.cases[i].Target == 1002 {
					c = &f.cases[i]
					break
				}
			}
			if c == nil {
				t.Fatal("no Blitz Beat case")
			}
			def := f.db.Get(IDBlitzBeat)
			src, err := f.attacker(t, c, def)
			if err != nil {
				t.Fatal(err)
			}
			target := f.mobs[c.Target]
			one := attack(mode, def.Type, src, target, def.Battle(src, 5, 1, 1), forcedMin{})
			three := attack(mode, def.Type, src, target, def.Battle(src, 5, 3, 1), forcedMin{})
			if mode == reference.Renewal {
				if three.Damage != one.Damage {
					t.Errorf("renewal: %d for 3 targets, %d for 1", three.Damage, one.Damage)
				}
				return
			}
			if three.Damage >= one.Damage || three.Damage < one.Damage/3-5 || three.Damage > one.Damage/3+5 {
				t.Errorf("pre-renewal: %d for 3 targets, %d for 1", three.Damage, one.Damage)
			}
		})
	}
}
