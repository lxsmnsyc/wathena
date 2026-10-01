package zone

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// fullScanViews is viewEvents as it was before the block index: every
// entity in the zone is checked against the view. It walks the blocks
// in the same order as viewEvents, so new entities get the same IDs.
func fullScanViews(z *Zone) func(p *Player, events []*wathenav1.Event) []*wathenav1.Event {
	return func(p *Player, events []*wathenav1.Event) []*wathenav1.Event {
		me := p.entity
		for _, block := range z.blocks.blocks {
			for _, e := range block {
				visible := abs(e.x-me.x) <= ViewRange && abs(e.y-me.y) <= ViewRange
				id, known := p.netIDs.get(e)
				switch {
				case visible && (!known || e == me && p.fresh):
					id = p.netID(e)
					events = append(events, z.appear(id, e))
					if len(e.path) > 0 {
						events = append(events, z.move(id, e))
					}
				case visible && e.walkChanged:
					events = append(events, z.move(id, e))
				case visible && e.stopped:
					events = append(events, stop(id, e))
				case !visible && known:
					p.netIDs.del(e)
					events = append(events, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT))
				}
			}
		}
		return events
	}
}

// checkBlocks checks that every entity of the zone is in the block of
// its cell, once.
func checkBlocks(t *testing.T, z *Zone) {
	t.Helper()
	n := 0
	for i, block := range z.blocks.blocks {
		for _, e := range block {
			n++
			if !z.entities.has(e) || z.blocks.block(e.x/blockSize, e.y/blockSize) != i {
				t.Fatalf("entity at %d,%d is in block %d", e.x, e.y, i)
			}
		}
	}
	if n != z.entities.len() {
		t.Fatalf("the blocks hold %d entities, the zone %d", n, z.entities.len())
	}
}

// twinZone makes a zone with wandering monsters that fight back, and
// warps within the map.
func twinZone() *Zone {
	mob := &battle.Status{MaxHP: 40, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Mode: modeCanAttack, RHW: battle.Weapon{Atk: 30, Atk2: 30, Range: 1}}
	spawns := []Spawn{
		{Mob: 1002, Name: "Poring", Amount: 60, Speed: 200, CanMove: true, Status: mob, RespawnMs: 2000},
		{Mob: 1078, Name: "Red Plant", Amount: 40, Speed: 2000},
	}
	z := New("map/test", open(120), rng.FromKey([32]byte{7}).Stream(), spawns)
	z.PlayerStatus = battle.Status{MaxHP: 100, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	z.SetWarps([]Warp{{X: 60, Y: 60, XS: 1, YS: 1, To: "map/test", ToX: 10, ToY: 10}})
	return z
}

// frameText is a player's messages of one tick, with each frame's
// events sorted, since their order within a frame is not set. It counts
// the kinds of events in seen.
func frameText(p *Player, seen map[string]int) []string {
	var out []string
	for len(p.Out) > 0 {
		m := <-p.Out
		f := m.GetFrame()
		var ev []string
		for _, e := range f.GetEvents() {
			ev = append(ev, prototext.Format(e))
			switch {
			case e.HasVanish():
				seen[e.GetVanish().GetReason().String()]++
			case e.HasAppear():
				seen["appear"]++
			case e.HasMove():
				seen["move"]++
			case e.HasStop():
				seen["stop"]++
			case e.HasDamage():
				seen["damage"]++
			case e.HasChat():
				seen["chat"]++
			}
		}
		slices.Sort(ev)
		out = append(out, fmt.Sprint(f.GetTick(), f.GetAckSeq(), ev, m.GetDisconnect()))
	}
	return out
}

// TestViewsMatchFullScan runs two zones through the same random game,
// one with viewEvents and one with a scan of every entity, and checks
// that every player gets the same messages from both.
func TestViewsMatchFullScan(t *testing.T) {
	a, b := twinZone(), twinZone()
	b.viewsForTest = fullScanViews(b)
	r := rand.New(rand.NewPCG(8, 9))
	var pa, pb []*Player
	join := func(i int) {
		x, y := r.IntN(110)+5, r.IntN(110)+5
		for _, z := range []*Zone{a, b} {
			p := add(z, fmt.Sprintf("p%d", i))
			p.entity.x, p.entity.y = x, y
			if z == a {
				pa = append(pa, p)
			} else {
				pb = append(pb, p)
			}
		}
	}
	for i := range 40 {
		join(i)
	}
	next := 40
	line := "hello"
	seen := map[string]int{}
	for tick := range 1500 {
		now := int64(tick+1) * 50
		for i := range pa {
			c := command{seq: uint32(tick) + 1} // #nosec G115 -- a small count.
			switch n := r.IntN(100); {
			case n < 4:
				e := pa[i].entity
				c.walkTo = &[2]int{e.x + r.IntN(25) - 12, e.y + r.IntN(25) - 12}
			case n < 5:
				// Walk onto the warp, which moves the player within the
				// map.
				c.walkTo = &[2]int{60, 60}
			case n < 6:
				c.chat = &line
			case n < 8:
				// Attack a monster the player sees. Both zones gave it
				// the same ID, as the frames so far matched.
				for e, id := range pa[i].netIDs.all() {
					if e.monster != nil {
						c.attack = &attackCommand{target: id, repeat: true}
						break
					}
				}
			default:
				continue
			}
			for _, z := range []*Zone{a, b} {
				c.player = pa[i]
				if z == b {
					c.player = pb[i]
				}
				z.handle(c)
			}
		}
		// Now and then a player leaves and another joins.
		if r.IntN(50) == 0 && len(pa) > 1 {
			i := r.IntN(len(pa))
			a.leave(pa[i], wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
			b.leave(pb[i], wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
			pa = slices.Delete(pa, i, i+1)
			pb = slices.Delete(pb, i, i+1)
			join(next)
			next++
		}
		a.stepAt(now)
		b.stepAt(now)
		checkBlocks(t, a)
		for i := range pa {
			fa, fb := frameText(pa[i], seen), frameText(pb[i], map[string]int{})
			if !slices.Equal(fa, fb) {
				t.Fatalf("tick %d, %s:\nblocks:    %v\nfull scan: %v", tick, pa[i].Name, fa, fb)
			}
		}
	}
	// The game had walks, fights, deaths, revives, warps and leaves to
	// compare.
	t.Log(seen)
	for _, k := range []string{"appear", "move", "stop", "damage", "chat", "VANISH_REASON_OUT_OF_SIGHT", "VANISH_REASON_DIED", "VANISH_REASON_TELEPORTED", "VANISH_REASON_LOGGED_OUT"} {
		if seen[k] == 0 {
			t.Errorf("no %s event", k)
		}
	}
}
