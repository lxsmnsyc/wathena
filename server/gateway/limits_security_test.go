package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

func TestCommandLimit(t *testing.T) {
	now := time.Unix(1000, 0)
	rates := DefaultRates()
	l := newCommandLimit(now, rates)
	walk := rates.Walk
	for i := range walk.Burst {
		if ok, _ := l.allow(now, kindWalk); !ok {
			t.Fatalf("walk %d of the burst refused", i)
		}
	}
	if ok, over := l.allow(now, kindWalk); ok || over {
		t.Fatalf("walk past the burst: ok %v, over %v", ok, over)
	}
	// Other kinds keep their own buckets.
	if ok, _ := l.allow(now, kindAction); !ok {
		t.Fatal("an attack was refused for the walks")
	}
	// One more walk every Every.
	now = now.Add(walk.Every)
	if ok, _ := l.allow(now, kindWalk); !ok {
		t.Fatal("no walk after a refill")
	}
	// A steady flow at the kind's rate never ends the connection.
	for range 1000 {
		now = now.Add(walk.Every)
		if ok, over := l.allow(now, kindWalk); !ok || over {
			t.Fatalf("a walk at the rate: ok %v, over %v", ok, over)
		}
	}
	// A flood does, after MaxStrikes dropped messages.
	dropped := 0
	for {
		ok, over := l.allow(now, kindWalk)
		if over {
			break
		}
		if !ok {
			dropped++
		}
		if dropped > 10*rates.MaxStrikes {
			t.Fatal("a flood never ended the connection")
		}
	}
	if dropped < rates.MaxStrikes {
		t.Errorf("ended after %d dropped messages, want at least %d", dropped, rates.MaxStrikes)
	}
}

func TestKindOf(t *testing.T) {
	for _, c := range []struct {
		m    *wathenav1.ClientMessage
		want commandKind
	}{
		{wathenav1.ClientMessage_builder{Ping: wathenav1.Ping_builder{}.Build()}.Build(), kindPing},
		{wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{WalkTo: wathenav1.WalkTo_builder{}.Build()}.Build()}.Build(), kindWalk},
		{wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{DropItem: wathenav1.DropItem_builder{}.Build()}.Build()}.Build(), kindDrop},
		{chat(wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL, "hi"), kindChat},
	} {
		if got := kindOf(c.m); got != c.want {
			t.Errorf("%v: kind %d, want %d", c.m, got, c.want)
		}
	}
}

// TestCommandFloodDisconnects checks that a client sending walks as fast
// as it can is disconnected for its rate, while another player on the
// map keeps playing.
func TestCommandFloodDisconnects(t *testing.T) {
	url := server(t)
	honest := dial(t, url)
	honest.hello("Honest", schema.Version)
	honest.next((*wathenav1.ServerMessage).HasWelcome)
	a := dial(t, url)
	a.hello("Flood", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	walk := wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		WalkTo: wathenav1.WalkTo_builder{X: 25, Y: 20}.Build(),
	}.Build()}.Build()
	for range DefaultRates().All.Burst + DefaultRates().MaxStrikes + 50 {
		a.seq++
		walk.SetSeq(a.seq)
		data, _ := proto.Marshal(walk)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := a.c.Write(ctx, websocket.MessageBinary, data)
		cancel()
		if err != nil {
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var last *wathenav1.ServerMessage
	for {
		_, data, err := a.c.Read(ctx)
		if err != nil {
			break
		}
		last = &wathenav1.ServerMessage{}
		_ = proto.Unmarshal(data, last)
	}
	if last.GetDisconnect().GetReason() != wathenav1.DisconnectReason_DISCONNECT_REASON_RATE_LIMITED {
		t.Errorf("last message %v, want a rate limit disconnect", last)
	}
	// The other player still walks.
	honest.send(wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		WalkTo: wathenav1.WalkTo_builder{X: 22, Y: 22}.Build(),
	}.Build()}.Build())
	honest.next(func(m *wathenav1.ServerMessage) bool { return m.GetFrame().GetAckSeq() >= honest.seq })
}
