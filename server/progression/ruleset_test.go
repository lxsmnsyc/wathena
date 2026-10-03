package progression

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/rules/rulestest"
)

// penaltyCases are level_penalty.json's cases: renewal's, as rAthena's
// pre-renewal has no level penalty.
type penaltyCases struct {
	Cases []struct {
		PlayerLevel int    `json:"player_level"`
		Mob         uint32 `json:"mob"`
		MobLevel    int    `json:"mob_level"`
		Exp         int    `json:"exp"`
		Drop        int    `json:"drop"`
		MVPExp      int    `json:"mvp_exp"`
		MVPDrop     int    `json:"mvp_drop"`
	} `json:"cases"`
}

// checkPenalty runs level_penalty.json on tables whose
// level_penalty_exp and level_penalty_drop flags are exp and drop: a
// type that is on gives the recorded rate, and one that is off 100%.
func checkPenalty(t *testing.T, name string, tb *Tables, exp, drop bool) {
	t.Helper()
	mobs := loadMobs(t, reference.Renewal)
	var ref penaltyCases
	if err := reference.Load(reference.Renewal, "level_penalty", &ref); err != nil {
		t.Fatal(err)
	}
	pick := func(on bool, rate int) int {
		if on {
			return rate
		}
		return 100
	}
	for _, c := range ref.Cases {
		m := &Mob{ID: c.Mob, Level: c.MobLevel, Mode: mobs[c.Mob].Status.Mode}
		got := [4]int{tb.Penalty(PenaltyExp, c.PlayerLevel, m), tb.Penalty(PenaltyDrop, c.PlayerLevel, m),
			tb.Penalty(PenaltyMVPExp, c.PlayerLevel, m), tb.Penalty(PenaltyMVPDrop, c.PlayerLevel, m)}
		want := [4]int{pick(exp, c.Exp), pick(drop, c.Drop), pick(exp, c.MVPExp), pick(drop, c.MVPDrop)}
		if got != want {
			t.Errorf("%s: player %d, mob %d level %d: got %v, want %v", name, c.PlayerLevel, c.Mob, c.MobLevel, got, want)
			return
		}
	}
}

// TestLevelPenaltyFlags checks both values of level_penalty_exp and
// level_penalty_drop on both bases against level_penalty.json: on, a
// pre-renewal base takes renewal's table and gives the recorded rates;
// off, a renewal base gives 100% as pre-renewal does. Without renewal's
// data a pre-renewal base cannot turn them on.
func TestLevelPenaltyFlags(t *testing.T) {
	re, _ := load(t, reference.Renewal)
	for _, mode := range reference.Modes {
		tb := tables(t, mode)
		for _, exp := range []bool{true, false} {
			for _, drop := range []bool{true, false} {
				got, err := tb.WithLevelPenalty(exp, drop, re)
				if err != nil {
					t.Fatal(err)
				}
				checkPenalty(t, string(mode), got, exp, drop)
			}
		}
	}
	if _, err := tables(t, reference.PreRenewal).WithLevelPenalty(true, false, nil); err == nil {
		t.Error("a pre-renewal base turned the exp penalty on without renewal's table")
	}
	// The tables a flag changes are copies.
	tb := tables(t, reference.Renewal)
	if _, err := tb.WithLevelPenalty(false, false, nil); err != nil {
		t.Fatal(err)
	}
	if tb.Penalty(PenaltyExp, 1, &Mob{Level: 99}) == 100 {
		t.Error("WithLevelPenalty changed its receiver")
	}
}

// TestMixedRulesetsLevelPenalty runs level_penalty.json for each sampled
// mixed ruleset (rulestest.Mixes), with the rates each mix's flags pick.
func TestMixedRulesetsLevelPenalty(t *testing.T) {
	re, _ := load(t, reference.Renewal)
	for _, rl := range rulestest.Mixes() {
		mode := reference.PreRenewal
		if rl.Renewal {
			mode = reference.Renewal
		}
		tb, err := tables(t, mode).WithLevelPenalty(rl.LevelPenaltyExp, rl.LevelPenaltyDrop, re)
		if err != nil {
			t.Fatal(err)
		}
		checkPenalty(t, string(mode)+" mix", tb, rl.LevelPenaltyExp, rl.LevelPenaltyDrop)
	}
}
