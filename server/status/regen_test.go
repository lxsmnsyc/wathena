package status

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// There is no recording of regen amounts, so these values come from
// reading rAthena's status_calc_regen.
func TestRegenOf(t *testing.T) {
	for _, c := range []struct {
		name   string
		st     battle.Status
		skills map[int]int
		want   Regen
	}{
		// A new novice: 1 HP, as max HP / 200 is at least 1.
		{"novice", battle.Status{MaxHP: 40, MaxSP: 11, Vit: 1, Int: 1}, nil, Regen{HP: 1, SP: 1}},
		// VIT 30 and 500 max HP: 6 + 2. INT 99 and 300 max SP: 1 + 16 + 3.
		{"stats", battle.Status{MaxHP: 500, MaxSP: 300, Vit: 30, Int: 99}, nil, Regen{HP: 8, SP: 20}},
		// INT 130 adds (130 - 120) / 2 + 4.
		{"int 130", battle.Status{MaxHP: 100, MaxSP: 1000, Int: 130}, nil, Regen{HP: 1, SP: 1 + 21 + 10 + 9}},
		// Increase HP Recovery 10: 50 + 10 × 1000 / 500. Increase SP
		// Recovery 5: 15 + 5 × 500 / 500. Meditatio 10 adds 30% to SP.
		{"skills", battle.Status{MaxHP: 1000, MaxSP: 500, Vit: 10, Int: 12},
			map[int]int{skillIncHPRecovery: 10, skillIncSPRecovery: 5, skillMeditatio: 10},
			Regen{HP: 2 + 5, SP: (1 + 2 + 5) * 130 / 100, SkillHP: 70, SkillSP: 20}},
	} {
		if got := RegenOf(&c.st, c.skills); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}
