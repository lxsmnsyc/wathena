package gateway

import (
	"testing"
	"time"

	protobuf "google.golang.org/protobuf/proto"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// TestPacerKeepsWithinLimits checks that messages sent when the pacer
// says are never dropped by the gateway's limit.
func TestPacerKeepsWithinLimits(t *testing.T) {
	rates := DefaultRates()
	p := NewPacer(rates)
	now := time.Unix(1000, 0)
	l := newCommandLimit(now, rates)
	social := wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		Guild: wathenav1.GuildCommand_builder{Invite: protobuf.String("Bob")}.Build()}.Build()}.Build()
	waited := time.Duration(0)
	for i := range 200 {
		w := p.Reserve(now, social)
		now = now.Add(w)
		waited += w
		if ok, _ := l.allow(now, kindSocial); !ok {
			t.Fatalf("message %d dropped after a wait of %v", i, w)
		}
	}
	// 200 social messages at 18 at once, then one every 220 ms, the
	// rate with its margin, take 40 s.
	if want := time.Duration(200-(rates.Social.Burst-2)) * (rates.Social.Every * 11 / 10); waited < want-time.Millisecond || waited > want+time.Millisecond {
		t.Errorf("waited %v, want %v", waited, want)
	}
	// Messages that reach the gateway closer together than they were
	// sent are still not dropped.
	p, l = NewPacer(rates), newCommandLimit(now, rates)
	for i := range 200 {
		now = now.Add(p.Reserve(now, social))
		arrive := now.Add(-time.Duration(i%3) * 15 * time.Millisecond)
		if ok, _ := l.allow(arrive, kindSocial); !ok {
			t.Fatalf("message %d dropped with jitter", i)
		}
	}
}
