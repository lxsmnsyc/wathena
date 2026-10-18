package zone

import (
	"context"
	"runtime"
	"testing"
	"time"
	"weak"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// freed reports whether the garbage collector freed what w points to.
func freed(w weak.Pointer[Player]) bool {
	for range 3 {
		runtime.GC()
	}
	return w.Value() == nil
}

// TestLeftPlayersAreFreed: once players left and their zone sleeps, the
// zone keeps none of them in memory. The load tests of decision 0050
// found the zone's block index of the last tick, and each left player's
// IDs for what it saw, kept every player that ever left in memory.
func TestLeftPlayersAreFreed(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	join := func(name string) (*Session, weak.Pointer[Player]) {
		s, err := z.Join(ctx, name, Look{})
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			for range s.Out {
			}
		}()
		s.mu.Lock()
		defer s.mu.Unlock()
		return s, weak.Make(s.player)
	}
	a, wa := join("a")
	b, wb := join("b")
	// They see each other for a few ticks.
	time.Sleep(5 * Tick)
	a.Leave()
	b.Leave()
	// The zone handled the leaves and fell asleep once a control call
	// runs after them.
	done := make(chan struct{})
	z.control <- func() { close(done) }
	<-done
	a, b = nil, nil
	if !freed(wa) || !freed(wb) {
		t.Error("the zone keeps players that left")
	}
	runtime.KeepAlive(z)
}

// TestFighterThatLeftIsFreed: a player that attacked a monster and left
// is not kept by the last tick's hits or the monster's AI, while another
// player keeps the zone awake.
func TestFighterThatLeftIsFreed(t *testing.T) {
	z, p, m := fighters(t, 1_000_000, 0)
	other := add(z, "b")
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: p.netIDs.id(m), repeat: true}})
	now := int64(100)
	for ; now <= 3000; now += 50 {
		z.stepAt(now)
		events(p)
		events(other)
	}
	w := weak.Make(p)
	z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	p = nil
	for end := now + 500; now <= end; now += 50 {
		z.stepAt(now)
		events(other)
	}
	if !freed(w) {
		t.Error("the zone keeps a fighter that left")
	}
	runtime.KeepAlive(z)
}
