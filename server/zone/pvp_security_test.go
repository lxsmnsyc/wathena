package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// A player must not be able to auto-attack another player: players do
// not fight players outside PvP, and PvP is not ported.
func TestSecurityAttackCommandRefusesPlayers(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{7}).Stream(), nil)
	z.PlayerStatus = battle.Status{MaxHP: 100, Hit: 200, BAtk: 50, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	a, b := add(z, "a"), add(z, "b")
	b.entity.x, b.entity.y = a.entity.x+1, a.entity.y
	z.stepAt(50)
	events(a)
	events(b)
	hp := b.entity.hp
	z.handle(command{player: a, seq: 1, attack: &attackCommand{target: a.netIDs.id(b.entity), repeat: true}})
	if a.entity.target == b.entity {
		t.Error("attack command accepted another player as target")
	}
	swings := 0
	for now := int64(100); now <= 5000; now += 50 {
		z.stepAt(now)
		for _, ev := range events(a) {
			if ev.GetDamage() != nil {
				swings++
				t.Logf("damage event %v", ev.GetDamage())
			}
		}
	}
	if swings > 0 {
		t.Errorf("player dealt damage to another player %d times outside PvP", swings)
	}
	t.Logf("hp %d->%d target set %v", hp, b.entity.hp, a.entity.target != nil)
	if b.entity.hp != hp {
		t.Errorf("victim HP %d, want %d", b.entity.hp, hp)
	}
}
