package zone

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// crowd joins n players to a 300 × 300 zone, spread over a square of
// side cells around the center, and runs one tick so each sees the
// others. The zone also has still monsters anywhere on the map, which
// stand for a field's monsters and the items that kills leave.
func crowd(tb testing.TB, n, side, still int) (*Zone, []*Player) {
	tb.Helper()
	var spawns []Spawn
	if still > 0 {
		spawns = []Spawn{{Mob: 1078, Name: "Red Plant", Amount: still, Speed: 2000}}
	}
	z := New("map/test", open(300), rng.FromKey([32]byte{5}).Stream(), spawns)
	r := rand.New(rand.NewPCG(1, 2))
	players := make([]*Player, n)
	for i := range players {
		p := add(z, fmt.Sprintf("p%d", i))
		p.entity.x = 150 - side/2 + r.IntN(side)
		p.entity.y = 150 - side/2 + r.IntN(side)
		players[i] = p
	}
	z.stepAt(50)
	for _, p := range players {
		events(p)
	}
	return z, players
}

// loadTick runs one tick of the town load test in strategy section 3.8:
// each player starts a walk every 3 seconds on average, and the zone
// hears 2 chat lines a second. It drains every player's messages, as the
// gateway does.
func loadTick(z *Zone, players []*Player, r *rand.Rand, now int64) {
	for _, p := range players {
		if r.IntN(60) == 0 {
			e := p.entity
			z.handle(command{player: p, walkTo: &[2]int{e.x + r.IntN(17) - 8, e.y + r.IntN(17) - 8}})
		}
	}
	if r.IntN(10) == 0 {
		line := "a chat line of forty characters, roughly"
		z.handle(command{player: players[r.IntN(len(players))], chat: &line})
	}
	z.stepAt(now)
	for _, p := range players {
		for len(p.Out) > 0 {
			<-p.Out
		}
	}
}

// BenchmarkLoadTick is one tick with walking and chatting players: a
// crowd where everyone sees everyone (side 20), a town where players
// spread over 120 × 120 cells, and a field where they spread over the
// whole map with 5,000 still entities, as after many kills.
func BenchmarkLoadTick(b *testing.B) {
	for _, c := range []struct {
		name           string
		n, side, still int
	}{{"crowd/500", 500, 20, 0}, {"town/500", 500, 120, 0}, {"field/500", 500, 280, 5000}, {"crowd/100", 100, 20, 0}} {
		b.Run(c.name, func(b *testing.B) {
			z, players := crowd(b, c.n, c.side, c.still)
			r := rand.New(rand.NewPCG(3, 4))
			now := int64(50)
			b.ReportAllocs()
			for b.Loop() {
				now += 50
				loadTick(z, players, r, now)
			}
		})
	}
}
