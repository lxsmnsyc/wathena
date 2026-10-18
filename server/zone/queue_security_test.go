package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestOneSessionCannotFillTheSharedQueue checks that a flood from one
// session does not make another session's command fail. Session.send
// returns false when the zone's one queue is full, and the gateway
// disconnects whoever sent then ("Too many actions at once").
func TestOneSessionCannotFillTheSharedQueue(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), nil)
	attacker, honest := newSession(nil), newSession(nil)
	attacker.zone, attacker.player = z, add(z, "attacker")
	honest.zone, honest.player = z, add(z, "honest")

	// The zone is busy (its loop is not running), so the flood is not
	// drained, as when the sender outpaces the zone.
	sent := 0
	for attacker.WalkTo(uint32(sent), 5, 5) {
		sent++
		if sent > 10*cap(z.commands) {
			t.Fatal("the queue never filled")
		}
	}
	if !honest.Ack(1) {
		t.Errorf("after %d walk commands from one session, another session's command is refused, so the gateway disconnects an honest player", sent)
	}
}
