package statuschange_test

import (
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/rules/rulestest"
)

// TestStatusResistFlag checks both values of the status_resist flag on
// both bases: status_get_sc_def follows the flag, so each mode's
// status_effects.json matches in full with either base's status
// database. ensemble_songs takes the same mode, since Siegfried's part
// of the resistance follows it.
func TestStatusResistFlag(t *testing.T) {
	for _, mode := range reference.Modes {
		for _, base := range reference.Modes {
			rl := rules.Base(base == reference.Renewal)
			rl.StatusResist = mode == reference.Renewal
			rl.EnsembleSongs = rl.StatusResist
			r := checkStatusEffects(t, mode, loadDB(t, base), rl)
			if r.matched != r.cases {
				t.Errorf("%s recording on the %s base: %d of %d cases match", mode, base, r.matched, r.cases)
			}
		}
	}
	// The other value differs: the renewal recording does not match with
	// pre-renewal's resistance.
	rl := rules.Base(true)
	rl.StatusResist = false
	if r := checkStatusEffects(t, reference.Renewal, loadDB(t, reference.Renewal), rl, false); r.matched == r.cases {
		t.Error("pre-renewal's resistance matched every renewal case")
	}
}

// TestMixedRulesetsStatusEffects runs status_effects.json of the base for
// each sampled mixed ruleset (rulestest.Mixes) whose status_resist and
// ensemble_songs flags are the base's.
func TestMixedRulesetsStatusEffects(t *testing.T) {
	ran := 0
	for i, rl := range rulestest.Mixes() {
		if rl.StatusResist != rl.Renewal || rl.EnsembleSongs != rl.Renewal {
			continue
		}
		mode := reference.PreRenewal
		if rl.Renewal {
			mode = reference.Renewal
		}
		t.Run(fmt.Sprintf("%s mix %d", mode, i), func(t *testing.T) {
			if r := checkStatusEffects(t, mode, loadDB(t, mode), rl); r.matched != r.cases {
				t.Errorf("%d of %d cases match", r.matched, r.cases)
			}
		})
		ran++
	}
	if ran == 0 {
		t.Error("no mix ran status_effects.json")
	}
}
