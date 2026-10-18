package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"google.golang.org/protobuf/proto"
)

// TestRepeatedTradeRequestsDoNotBloatVictimFrame checks that one asker
// repeating a trade request cannot grow the victim's pending events
// without bound (a frame over schema.MaxMessageBytes ends the victim's
// connection in gateway.write).
func TestRepeatedTradeRequestsDoNotBloatVictimFrame(t *testing.T) {
	z, a, b, _ := storageZone(t, stack(0, 909, 10))
	id := netIDOf(a, b)
	for range 4000 {
		z.handle(command{player: a, seq: 1, trade: &TradeAction{Request: id}})
	}
	z.stepAt(z.now + 50)
	ev := events(b)
	frame := wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{Events: ev}.Build()}.Build()
	var requests int
	for _, e := range ev {
		if e.HasTradeRequest() {
			requests++
		}
	}
	if requests > 1 || proto.Size(frame) > 64<<10 {
		t.Fatalf("victim got %d trade requests in one frame of %d bytes", requests, proto.Size(frame))
	}
}

// TestTradeRequestsAreThrottled checks that one asker reaches at most one
// player per tradeAskEvery, and that the asked player sees the dialog
// again once it answered.
func TestTradeRequestsAreThrottled(t *testing.T) {
	z, a, b, _ := storageZone(t)
	c := add(z, "c")
	c.entity.x, c.entity.y = a.entity.x, a.entity.y+1
	z.stepAt(z.now + 50)
	events(a)
	events(c)
	z.handle(command{player: a, seq: 1, trade: &TradeAction{Request: netIDOf(a, b)}})
	z.handle(command{player: a, seq: 2, trade: &TradeAction{Request: netIDOf(a, c)}})
	z.stepAt(z.now + 50)
	if got := tradeOutcomes(events(a)); len(got) != 2 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT || got[1] != wathenav1.TradeOutcome_TRADE_OUTCOME_FAILED {
		t.Errorf("outcomes %v, want REQUEST_SENT then FAILED", got)
	}
	for _, e := range events(c) {
		if e.HasTradeRequest() {
			t.Error("a second player was asked within tradeAskEvery")
		}
	}
	events(b)
	z.handle(command{player: b, seq: 1, trade: &TradeAction{Respond: true, Accept: false}})
	z.now += tradeAskEvery
	z.handle(command{player: a, seq: 3, trade: &TradeAction{Request: netIDOf(a, b)}})
	z.stepAt(z.now + 50)
	n := 0
	for _, e := range events(b) {
		if e.HasTradeRequest() {
			n++
		}
	}
	if n != 1 {
		t.Errorf("asked again after a refusal: %d requests, want 1", n)
	}
}
