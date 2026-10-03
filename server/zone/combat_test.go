package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// fighters makes a zone with one player and one monster two cells apart.
func fighters(t *testing.T, mobHP int, mode uint32) (*Zone, *Player, *entity) {
	t.Helper()
	// Both sides always hit for 10. The bare-handed player deals twice its
	// status ATK.
	mob := &battle.Status{MaxHP: mobHP, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Mode: mode, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	spawns := []Spawn{{Mob: 1002, Name: "Poring", Amount: 1, X: 20, Y: 20, Speed: 400, ViewRange: 10, ChaseRange: 12, Status: mob, RespawnMs: 5000}}
	z := New("map/test", open(40), rng.FromKey([32]byte{2}).Stream(), spawns)
	z.PlayerStatus = battle.Status{MaxHP: 100, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	p := add(z, "a")
	var m *entity
	for e := range z.entities.all() {
		if e.monster != nil {
			m = e
		}
	}
	p.entity.x, p.entity.y = m.x+2, m.y
	z.stepAt(50)
	events(p)
	return z, p, m
}

func TestAttackUntilDeathAndRespawn(t *testing.T) {
	z, p, m := fighters(t, 30, 0)
	id := p.netIDs.id(m)
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: id, repeat: true}})

	// The player walks next to the monster, then hits it for 10 once per
	// attack delay until its 30 HP run out.
	var damage, died int
	for now := int64(100); now <= 5000; now += 50 {
		z.stepAt(now)
		for _, e := range events(p) {
			if d := e.GetDamage(); d != nil {
				damage++
				if d.GetSource() != p.netIDs.id(p.entity) && d.GetSource() != 1 {
					t.Errorf("damage from %d", d.GetSource())
				}
			}
			if e.GetVanish().GetReason() == wathenav1.VanishReason_VANISH_REASON_DIED {
				died++
			}
		}
	}
	if damage != 3 || died != 1 {
		t.Fatalf("%d hits, %d deaths", damage, died)
	}
	if z.entities.has(m) || p.entity.target != nil {
		t.Error("the dead monster is still in the zone or targeted")
	}
	// It comes back 5 seconds after dying.
	back := false
	for now := int64(5050); now <= 12000; now += 50 {
		z.stepAt(now)
		for e := range z.entities.all() {
			if e.monster != nil {
				back = true
			}
		}
	}
	if !back {
		t.Error("the monster did not respawn")
	}
}

func TestMonsterStrikesBackAndPlayerRevives(t *testing.T) {
	z, p, m := fighters(t, 1000, modeCanAttack|1)
	id := p.netIDs.id(m)
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: id, repeat: false}})
	for now := int64(100); now <= 2000; now += 50 {
		z.stepAt(now)
	}
	if m.target != p.entity {
		t.Fatal("the monster did not strike back")
	}
	// It keeps hitting for 10 until the player's 100 HP run out, then the
	// player returns to the start with full HP.
	sawHP := false
	for now := int64(2050); now <= 120000 && !p.entity.dead; now += 50 {
		z.stepAt(now)
		for _, e := range events(p) {
			if e.HasStats() {
				sawHP = true
			}
		}
	}
	if !p.entity.dead || !sawHP {
		t.Fatalf("dead %v, saw HP %v", p.entity.dead, sawHP)
	}
	if m.target != nil {
		t.Error("the monster still targets a dead player")
	}
	died := z.now
	for now := died + 50; now <= died+reviveMs+100; now += 50 {
		z.stepAt(now)
	}
	sx, sy := z.startCell()
	if p.entity.dead || p.entity.hp != 100 || p.entity.x != sx || p.entity.y != sy {
		t.Errorf("after reviving: dead %v, HP %d, at (%d, %d)", p.entity.dead, p.entity.hp, p.entity.x, p.entity.y)
	}
}

func TestFacingTo(t *testing.T) {
	for _, c := range [][3]int{{0, -1, 0}, {0, 1, 4}, {1, 0, 6}, {-1, 1, 3}} {
		if got := facingTo(c[0], c[1]); got != c[2] {
			t.Errorf("facingTo(%d, %d) = %d", c[0], c[1], got)
		}
	}
}
