package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/shared/path"
)

// TestBasicAttackNeedsClearShot checks that a ranged basic attack does not
// pass through cells that block shots (rAthena battle_check_range).
func TestBasicAttackNeedsClearShot(t *testing.T) {
	z, p, m := fighters(t, 1000, 0)
	z.PlayerStatus.RHW.Range = 9
	p.entity.status.RHW.Range = 9
	p.entity.x, p.entity.y = m.x+6, m.y
	// A wall cell between them blocks shots.
	z.grid.Cells[(m.x+3)+m.y*z.grid.Width] &^= path.Walkable | path.Shootable
	if clear, _ := path.Shoot(z.grid, p.entity.x, p.entity.y, m.x, m.y); clear {
		t.Fatal("setup: the shot is not blocked")
	}
	hp := m.hp
	id := p.netIDs.id(m)
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: id, repeat: false}})
	for now := int64(100); now <= 2000; now += 50 {
		z.stepAt(now)
	}
	if m.hp != hp {
		t.Fatalf("monster lost %d HP through a blocked shot", hp-m.hp)
	}
}
