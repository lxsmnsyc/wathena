package zone

import (
	"context"
	"errors"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
)

// lastDisconnect drains a player's messages until they close, and
// returns the reason of the last Disconnect.
func lastDisconnect(t *testing.T, out <-chan *wathenav1.ServerMessage) wathenav1.DisconnectReason {
	t.Helper()
	var reason wathenav1.DisconnectReason
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-out:
			if !ok {
				return reason
			}
			if m.HasDisconnect() {
				reason = m.GetDisconnect().GetReason()
			}
		case <-timeout:
			t.Fatal("the messages did not close")
		}
	}
}

func redPotions(c *character.Character) int {
	n := 0
	for _, it := range c.Items {
		if it.ID == 501 {
			n += it.Amount
		}
	}
	return n
}

// TestCommandPanicEndsOnlySender checks that a panic in a command
// handler ends the sender's session with its last saved copy, and that
// the zone and the other players go on.
func TestCommandPanicEndsOnlySender(t *testing.T) {
	z := saveZone()
	sa, sb := &saves{}, &saves{}
	cb := savedCharacter()
	cb.ID, cb.Name = 8, "Other"
	a, b := addCharacter(z, savedCharacter(), sa), addCharacter(z, cb, sb)
	z.stepAt(50)
	events(a)
	events(b)
	// The handler gives items, then fails before it ends.
	z.handleSafely(command{player: a, staff: func(z *Zone, p *Player) {
		z.addItem(p, 501, 10, scriptOrigin)
		panic("a bug")
	}})
	if z.failed {
		t.Fatal("the zone stopped for a command's panic")
	}
	if z.entities.has(a.entity) || !z.entities.has(b.entity) {
		t.Fatalf("a in the zone %v, b in the zone %v", z.entities.has(a.entity), z.entities.has(b.entity))
	}
	if r := lastDisconnect(t, a.Out); r != wathenav1.DisconnectReason_DISCONNECT_REASON_SERVER_ERROR {
		t.Errorf("disconnect reason %v", r)
	}
	if n := len(sa.got); n == 0 || !sa.final[n-1] || redPotions(sa.got[n-1]) != 3 {
		t.Fatalf("the last save of a is not its saved copy with 3 Red Potions: %d saves", n)
	}
	for i, c := range sa.got {
		if redPotions(c) != 3 {
			t.Errorf("save %d has %d Red Potions, want 3: the half-done give was saved", i, redPotions(c))
		}
	}
	if len(sb.final) > 0 && sb.final[len(sb.final)-1] {
		t.Error("b's session ended")
	}
	// The zone still runs b's commands.
	z.handle(command{player: b, seq: 9})
	if b.ackSeq != 9 {
		t.Error("b's command was not handled")
	}
}

// TestTransferPanicStopsZone checks that a panic inside a transfer,
// whose boxes the zone does not know, stops the zone: every player is
// disconnected with its last saved copy, and joins are refused.
func TestTransferPanicStopsZone(t *testing.T) {
	z := saveZone()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	sa, sb := &saves{}, &saves{}
	cb := savedCharacter()
	cb.ID, cb.Name = 8, "Other"
	a, err := z.JoinCharacter(ctx, savedCharacter(), sa)
	if err != nil {
		t.Fatal(err)
	}
	b, err := z.JoinCharacter(ctx, cb, sb)
	if err != nil {
		t.Fatal(err)
	}
	if !a.send(command{staff: func(z *Zone, p *Player) {
		z.addItem(p, 501, 10, scriptOrigin)
		panic("transfer: move 0 failed after its checks passed")
	}}) {
		t.Fatal("the command was refused")
	}
	select {
	case <-z.Stopped():
	case <-time.After(5 * time.Second):
		t.Fatal("the zone did not stop")
	}
	for _, s := range []*Session{a, b} {
		if r := lastDisconnect(t, s.Out); r != wathenav1.DisconnectReason_DISCONNECT_REASON_SERVER_ERROR {
			t.Errorf("disconnect reason %v", r)
		}
	}
	for name, sv := range map[string]*saves{"a": sa, "b": sb} {
		n := len(sv.got)
		if n == 0 || !sv.final[n-1] {
			t.Errorf("%s: no final save", name)
			continue
		}
		if redPotions(sv.got[n-1]) != 3 {
			t.Errorf("%s: final save has %d Red Potions, want 3", name, redPotions(sv.got[n-1]))
		}
	}
	c := savedCharacter()
	c.ID, c.Name = 9, "Late"
	if _, err := z.JoinCharacter(ctx, c, &saves{}); !errors.Is(err, ErrStopped) {
		t.Errorf("join after the stop: %v, want ErrStopped", err)
	}
}

// TestFramePanicEndsOnlyThatPlayer checks that a panic while the zone
// builds one player's frame ends that player's connection, and the
// tick goes on for the others.
func TestFramePanicEndsOnlyThatPlayer(t *testing.T) {
	z := saveZone()
	a, b := add(z, "a"), add(z, "b")
	z.viewsForTest = func(p *Player, ev []*wathenav1.Event) []*wathenav1.Event {
		if p == a {
			panic("a bug")
		}
		return z.viewEvents(p, ev)
	}
	z.stepAt(50)
	if z.failed {
		t.Fatal("the zone stopped for a frame's panic")
	}
	if r := lastDisconnect(t, a.Out); r != wathenav1.DisconnectReason_DISCONNECT_REASON_SERVER_ERROR {
		t.Errorf("disconnect reason %v", r)
	}
	if len(events(b)) == 0 {
		t.Error("b got no frame")
	}
}
