package character

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/item"
)

// TestClone checks that a clone is equal and shares nothing that the
// original's changes reach.
func TestClone(t *testing.T) {
	c := &Character{
		Info:         Info{Name: "Alice", Zeny: 5},
		Items:        []Item{{Slot: 0, ID: 909, Amount: 3, Cards: []uint32{4001}, Extra: item.Extra{Options: []item.Option{{}}}}},
		Variables:    map[Variable]string{{Name: "v"}: "1"},
		Statuses:     []Status{{Name: "s"}},
		TimedBonuses: []TimedBonus{{Key: "k", EndsOn: []string{"logout"}}},
		Skills:       map[int]int{1: 9},
		Blocks:       []string{"Bob"},
		Sanctions:    []Sanction{{Kind: SanctionMute}},
	}
	d := c.Clone()
	if !d.Equal(c) {
		t.Fatalf("clone %+v differs from %+v", d, c)
	}
	c.Items[0].Amount = 1
	c.Items[0].Cards[0] = 0
	c.Items[0].Options[0] = item.Option{Name: "x"}
	c.Variables[Variable{Name: "v"}] = "2"
	c.Statuses[0].Name = "t"
	c.TimedBonuses[0].EndsOn[0] = "never"
	c.Skills[1] = 1
	c.Blocks[0] = "Carol"
	c.Sanctions[0].Kind = SanctionJail
	if d.Items[0].Amount != 3 || d.Items[0].Cards[0] != 4001 || d.Items[0].Options[0] != (item.Option{}) ||
		d.Variables[Variable{Name: "v"}] != "1" || d.Statuses[0].Name != "s" || d.TimedBonuses[0].EndsOn[0] != "logout" ||
		d.Skills[1] != 9 || d.Blocks[0] != "Bob" || d.Sanctions[0].Kind != SanctionMute {
		t.Errorf("a change to the original reached the clone: %+v", d)
	}
}
