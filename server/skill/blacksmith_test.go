package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
)

// TestCartTerminationRatio checks SkillCartTermination::calculateSkillRatio
// for the cases the recordings (a player with a cart of 8000) do not
// show: a player with an empty cart keeps 100%, a full cart, and a unit
// that is not a player deals what a full cart deals.
func TestCartTerminationRatio(t *testing.T) {
	for _, c := range []struct {
		name   string
		player *battle.Player
		lv     int
		want   int
	}{
		{"empty cart", &battle.Player{}, 10, 100},
		// 80000 / 60 * 80000 / 80000.
		{"full cart lv 10", &battle.Player{CartWeight: MaxCartWeight}, 10, 1333},
		{"monster lv 10", nil, 10, 80000 / 60},
		{"monster lv 1", nil, 1, 80000 / 150},
	} {
		ratio := 100
		a := &battle.SkillContext{Src: &battle.Status{Player: c.player}, Target: &battle.Status{},
			Skill: &battle.Skill{Level: c.lv}}
		cartTermination{}.SkillRatio(a, &ratio)
		if ratio != c.want {
			t.Errorf("%s: ratio %d, want %d", c.name, ratio, c.want)
		}
	}
}

// TestUnfairTrickZeny checks the cost reductions of
// skill_get_requirement: Unfair Trick (BS_UNFAIRLYTRICK) lowers
// Mammonite's zeny cost by 20% in renewal and 10% in pre-renewal, and
// Cart Termination's by 20% in renewal only. The recordings' players do
// not learn the quest skill.
func TestUnfairTrickZeny(t *testing.T) {
	for _, mode := range reference.Modes {
		db := loadDB(t, mode)
		renewal := mode == reference.Renewal
		for _, c := range []struct {
			id, lv, base int
		}{{IDMammonite, 10, 1000}, {IDCartTermination, 10, 1500}} {
			want := c.base
			switch {
			case renewal:
				want = c.base * 80 / 100
			case c.id == IDMammonite:
				want = c.base * 90 / 100
			}
			st := &battle.Status{Player: &battle.Player{Skills: map[int]int{IDUnfairTrick: 1}}}
			got := db.Get(c.id).RequirementFor(&Caster{Status: st, Renewal: renewal}, c.lv, 1, 1).Zeny
			if got != want {
				t.Errorf("%s: skill %d zeny %d, want %d", mode, c.id, got, want)
			}
			st.Player.Skills = map[int]int{}
			if got := db.Get(c.id).RequirementFor(&Caster{Status: st, Renewal: renewal}, c.lv, 1, 1).Zeny; got != c.base {
				t.Errorf("%s: skill %d zeny %d without Unfair Trick, want %d", mode, c.id, got, c.base)
			}
		}
	}
}
