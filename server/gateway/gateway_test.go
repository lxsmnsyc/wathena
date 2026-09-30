package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
)

func server(t *testing.T) string {
	t.Helper()
	g := &path.Grid{Width: 40, Height: 40, Cells: make([]uint8, 1600)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	s := httptest.NewServer(&Gateway{Zone: z, Sessions: DevSessions{}})
	t.Cleanup(s.Close)
	return "ws" + strings.TrimPrefix(s.URL, "http")
}

type client struct {
	t   *testing.T
	c   *websocket.Conn
	seq uint32
}

func dial(t *testing.T, url string) *client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return &client{t: t, c: c}
}

func (c *client) send(m *wathenav1.ClientMessage) {
	c.t.Helper()
	c.seq++
	m.SetSeq(c.seq)
	data, _ := proto.Marshal(m)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.c.Write(ctx, websocket.MessageBinary, data); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) hello(name string, version uint32) {
	c.send(wathenav1.ClientMessage_builder{Hello: wathenav1.Hello_builder{
		ProtocolVersion: version, SessionToken: []byte(name),
	}.Build()}.Build())
}

// next reads messages until one matches, or fails after a timeout.
func (c *client) next(match func(*wathenav1.ServerMessage) bool) *wathenav1.ServerMessage {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := c.c.Read(ctx)
		if err != nil {
			c.t.Fatalf("read: %v", err)
		}
		m := &wathenav1.ServerMessage{}
		if err := proto.Unmarshal(data, m); err != nil {
			c.t.Fatal(err)
		}
		if match(m) {
			return m
		}
	}
}

func hasEvent(pick func(*wathenav1.Event) bool) func(*wathenav1.ServerMessage) bool {
	return func(m *wathenav1.ServerMessage) bool {
		for _, e := range m.GetFrame().GetEvents() {
			if pick(e) {
				return true
			}
		}
		return false
	}
}

func TestPlayersSeeEachOtherWalk(t *testing.T) {
	url := server(t)
	a := dial(t, url)
	a.hello("Alice", schema.Version)
	w := a.next((*wathenav1.ServerMessage).HasWelcome).GetWelcome()
	if w.GetMap() != "map/test" || w.GetNetId() == 0 {
		t.Fatalf("welcome %v", w)
	}
	self := w.GetNetId()
	a.next(hasEvent(func(e *wathenav1.Event) bool {
		return e.GetAppear().GetId() == self && e.GetAppear().GetName() == "Alice"
	}))

	b := dial(t, url)
	b.hello("Bob", schema.Version)
	b.next((*wathenav1.ServerMessage).HasWelcome)
	a.next(hasEvent(func(e *wathenav1.Event) bool { return e.GetAppear().GetName() == "Bob" }))

	// Bob walks. Alice sees the walk, and her frames acknowledge nothing
	// of Bob's.
	b.send(wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		WalkTo: wathenav1.WalkTo_builder{X: 25, Y: 20}.Build(),
	}.Build()}.Build())
	m := a.next(hasEvent(func(e *wathenav1.Event) bool { return e.HasMove() }))
	if mv := m.GetFrame().GetEvents()[0].GetMove(); len(mv.GetSteps()) == 0 || mv.GetWalkMs() != zone.WalkSpeed {
		t.Errorf("move %v", mv)
	}
	// Bob's own frame acknowledges his command.
	b.next(func(m *wathenav1.ServerMessage) bool { return m.GetFrame().GetAckSeq() == 2 })

	// Ping gets a pong with the nonce.
	a.send(wathenav1.ClientMessage_builder{Ping: wathenav1.Ping_builder{Nonce: 7}.Build()}.Build())
	if p := a.next((*wathenav1.ServerMessage).HasPong); p.GetPong().GetNonce() != 7 {
		t.Errorf("pong %v", p)
	}

	// Bob leaves, and Alice sees him go.
	_ = b.c.Close(websocket.StatusNormalClosure, "")
	a.next(hasEvent(func(e *wathenav1.Event) bool {
		return e.GetVanish().GetReason() == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT
	}))
}

func TestRejects(t *testing.T) {
	url := server(t)
	for name, tc := range map[string]struct {
		send   func(c *client)
		reason wathenav1.DisconnectReason
	}{
		"old version": {func(c *client) { c.hello("Alice", 0) }, wathenav1.DisconnectReason_DISCONNECT_REASON_VERSION_UNSUPPORTED},
		"bad name":    {func(c *client) { c.hello(" ", schema.Version) }, wathenav1.DisconnectReason_DISCONNECT_REASON_SESSION_INVALID},
		"no hello": {func(c *client) {
			c.send(wathenav1.ClientMessage_builder{Ping: wathenav1.Ping_builder{}.Build()}.Build())
		}, wathenav1.DisconnectReason_DISCONNECT_REASON_BAD_MESSAGE},
		"garbage": {func(c *client) {
			_ = c.c.Write(context.Background(), websocket.MessageBinary, []byte{0xff, 0xff, 0xff})
		}, wathenav1.DisconnectReason_DISCONNECT_REASON_BAD_MESSAGE},
		"too large": {func(c *client) {
			_ = c.c.Write(context.Background(), websocket.MessageBinary, make([]byte, schema.MaxClientMessageBytes+1))
		}, wathenav1.DisconnectReason_DISCONNECT_REASON_UNSPECIFIED},
	} {
		t.Run(name, func(t *testing.T) {
			c := dial(t, url)
			tc.send(c)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var last *wathenav1.ServerMessage
			for {
				_, data, err := c.c.Read(ctx)
				if err != nil {
					break
				}
				last = &wathenav1.ServerMessage{}
				_ = proto.Unmarshal(data, last)
			}
			if tc.reason != wathenav1.DisconnectReason_DISCONNECT_REASON_UNSPECIFIED && last.GetDisconnect().GetReason() != tc.reason {
				t.Errorf("got %v, want %v", last, tc.reason)
			}
		})
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{"Alice": true, "포링": true, "": false, " a": false, "a\n": false, strings.Repeat("x", 25): false} {
		if ValidName(name) != want {
			t.Errorf("%q", name)
		}
	}
}
