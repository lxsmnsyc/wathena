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

// TestPlayerAttackRangeIsCircle checks that a player's basic attack
// range is a circle, as rAthena's check_distance_client_bl in
// unit_attack_timer_sub: a bow of range 9 reaches 9,4 cells away but not
// 9,9, where the player walks closer first.
func TestPlayerAttackRangeIsCircle(t *testing.T) {
	for _, c := range []struct {
		dx, dy  int
		inReach bool
	}{{9, 4, true}, {9, 9, false}} {
		z, p, m := fighters(t, 1000, 0)
		z.PlayerStatus.RHW.Range = 9
		p.entity.status.RHW.Range = 9
		p.entity.x, p.entity.y = m.x+c.dx, m.y+c.dy
		hp, x, y := m.hp, p.entity.x, p.entity.y
		z.handle(command{player: p, seq: 1, attack: &attackCommand{target: p.netIDs.id(m), repeat: true}})
		z.stepAt(100)
		moved := len(p.entity.path) > 0 || p.entity.x != x || p.entity.y != y
		if c.inReach && (moved || m.hp == hp) {
			t.Errorf("offset %d,%d: the player moved %v, the monster lost %d HP; want an attack in place", c.dx, c.dy, moved, hp-m.hp)
		}
		if !c.inReach && (!moved || m.hp != hp) {
			t.Errorf("offset %d,%d: the player moved %v, the monster lost %d HP; want a walk closer first", c.dx, c.dy, moved, hp-m.hp)
		}
	}
}
