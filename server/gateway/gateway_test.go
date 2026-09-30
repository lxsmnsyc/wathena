package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func server(t *testing.T) string {
	t.Helper()
	return serverWith(t, func(*zone.Zone) {})
}

// serverWith runs a zone that setup prepares.
func serverWith(t *testing.T, setup func(*zone.Zone)) string {
	t.Helper()
	g := &path.Grid{Width: 40, Height: 40, Cells: make([]uint8, 1600)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
	setup(z)
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

	// Picking up an item that is not there fails.
	a.send(wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		PickUp: wathenav1.PickUp_builder{Item: 999}.Build(),
	}.Build()}.Build())
	f := a.next(hasEvent(func(e *wathenav1.Event) bool { return e.HasPickUpFailed() }))
	for _, e := range f.GetFrame().GetEvents() {
		if pf := e.GetPickUpFailed(); pf != nil && (pf.GetItem() != 999 || pf.GetReason() != wathenav1.PickUpFailure_PICK_UP_FAILURE_GONE) {
			t.Errorf("pick up failed %v", pf)
		}
	}

	// Bob leaves, and Alice sees him go.
	_ = b.c.Close(websocket.StatusNormalClosure, "")
	a.next(hasEvent(func(e *wathenav1.Event) bool {
		return e.GetVanish().GetReason() == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT
	}))
}

// TestRaiseStat checks that the gateway passes valid raises to the zone
// and drops the others.
func TestRaiseStat(t *testing.T) {
	var jobs status.Data
	if err := reference.LoadFile("extracts/renewal/job_stats.json", &jobs); err != nil {
		t.Fatal(err)
	}
	tables, err := status.New(&jobs)
	if err != nil {
		t.Fatal(err)
	}
	var prog progression.Data
	if err := reference.LoadFile("extracts/renewal/progression.json", &prog); err != nil {
		t.Fatal(err)
	}
	url := serverWith(t, func(z *zone.Zone) {
		z.Status = tables
		z.Start = status.Character{Job: 0, Male: true, Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
		z.Progress = progression.New(&prog, nil, progression.DefaultConfig)
	})
	a := dial(t, url)
	a.hello("Alice", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	raise := func(st wathenav1.BaseStat, amount uint32) {
		a.send(wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
			RaiseStat: wathenav1.RaiseStat_builder{Stat: st, Amount: amount}.Build(),
		}.Build()}.Build())
	}
	raise(wathenav1.BaseStat_BASE_STAT_UNSPECIFIED, 1)
	raise(wathenav1.BaseStat(7), 1)
	raise(wathenav1.BaseStat_BASE_STAT_STR, 0)
	raise(wathenav1.BaseStat_BASE_STAT_STR, MaxRaise+1)
	raise(wathenav1.BaseStat_BASE_STAT_STR, 2)
	m := a.next(hasEvent(func(e *wathenav1.Event) bool {
		for _, s := range e.GetStats().GetStats() {
			if s.GetId() == wathenav1.StatId_STAT_ID_STR && s.GetValue() != 1 {
				return true
			}
		}
		return false
	}))
	got := map[wathenav1.StatId]int64{}
	for _, e := range m.GetFrame().GetEvents() {
		for _, s := range e.GetStats().GetStats() {
			got[s.GetId()] = s.GetValue()
		}
	}
	if got[wathenav1.StatId_STAT_ID_STR] != 3 || got[wathenav1.StatId_STAT_ID_STATUS_POINTS] != 44 || m.GetFrame().GetAckSeq() != 6 {
		t.Errorf("STR %d, status points %d, ack %d; want 3, 44 and 6", got[wathenav1.StatId_STAT_ID_STR], got[wathenav1.StatId_STAT_ID_STATUS_POINTS], m.GetFrame().GetAckSeq())
	}
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

// TestItemCommands checks that the gateway passes item commands with a
// slot and amount in range to the zone, and drops the others.
func TestItemCommands(t *testing.T) {
	url := serverWith(t, func(z *zone.Zone) {
		z.StartItems = []zone.StartItem{{ID: 909, Amount: 3}}
	})
	a := dial(t, url)
	a.hello("Alice", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	inv := a.next(hasEvent((*wathenav1.Event).HasInventory))
	for _, e := range inv.GetFrame().GetEvents() {
		if l := e.GetInventory(); l != nil {
			if it := l.GetItems(); len(it) != 1 || it[0].GetSlot() != 0 || it[0].GetItemId() != 909 || it[0].GetAmount() != 3 || l.GetSize() != 100 {
				t.Errorf("inventory %v", l)
			}
		}
	}
	command := func(c *wathenav1.Command) {
		a.send(wathenav1.ClientMessage_builder{Command: c}.Build())
	}
	use := func(slot uint32) {
		command(wathenav1.Command_builder{UseItem: wathenav1.UseItem_builder{Slot: slot}.Build()}.Build())
	}
	drop := func(slot, amount uint32) {
		command(wathenav1.Command_builder{DropItem: wathenav1.DropItem_builder{Slot: slot, Amount: amount}.Build()}.Build())
	}
	use(200)
	drop(200, 1)
	drop(0, 30001)
	// Jellopy is an Etc item.
	use(0)
	m := a.next(hasEvent((*wathenav1.Event).HasUseItemFailed))
	if m.GetFrame().GetAckSeq() != 5 {
		t.Errorf("ack %d, want 5", m.GetFrame().GetAckSeq())
	}
	for _, e := range m.GetFrame().GetEvents() {
		if f := e.GetUseItemFailed(); f != nil && (f.GetSlot() != 0 || f.GetReason() != wathenav1.UseItemFailure_USE_ITEM_FAILURE_NOT_USABLE) {
			t.Errorf("use failed %v", f)
		}
	}
	drop(0, 2)
	m = a.next(hasEvent((*wathenav1.Event).HasInventoryRemove))
	for _, e := range m.GetFrame().GetEvents() {
		if r := e.GetInventoryRemove(); r != nil && (r.GetSlot() != 0 || r.GetAmount() != 2 || r.GetReason() != wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_DROPPED) {
			t.Errorf("remove %v", r)
		}
	}
}
