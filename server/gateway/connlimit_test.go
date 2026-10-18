package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func TestConnLimitPerAddress(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newConnLimit(ConnLimits{PerAddress: 2, Hello: Rate{Burst: 10, Every: time.Second}})
	a := netip.MustParseAddr("203.0.113.7")
	for i := range 2 {
		if ok, _ := l.open(a, now); !ok {
			t.Fatalf("connection %d refused", i)
		}
	}
	if ok, wait := l.open(a, now); ok || wait <= 0 {
		t.Fatalf("third connection: ok %v, wait %v", ok, wait)
	}
	// Another address has its own count.
	if ok, _ := l.open(netip.MustParseAddr("203.0.113.8"), now); !ok {
		t.Fatal("another address was refused")
	}
	// A closed connection frees its place.
	l.close(a)
	if ok, _ := l.open(a, now); !ok {
		t.Fatal("no place after a close")
	}
	// An IPv6 /64 counts as one address.
	b1 := netip.MustParseAddr("2001:db8:1:2::1")
	b2 := netip.MustParseAddr("2001:db8:1:2:ffff::9")
	if ok, _ := l.open(b1, now); !ok {
		t.Fatal("first IPv6 connection refused")
	}
	if ok, _ := l.open(b2, now); !ok {
		t.Fatal("second IPv6 connection refused")
	}
	if ok, _ := l.open(netip.MustParseAddr("2001:db8:1:2::77"), now); ok {
		t.Fatal("a third address of the same /64 was allowed")
	}
	// Loopback is not limited.
	for i := range 10 {
		if ok, _ := l.open(netip.MustParseAddr("127.0.0.1"), now); !ok {
			t.Fatalf("loopback connection %d refused", i)
		}
	}
}

func TestConnLimitHelloRate(t *testing.T) {
	now := time.Unix(1000, 0)
	hello := Rate{Burst: 3, Every: 2 * time.Second}
	l := newConnLimit(ConnLimits{PerAddress: 100, Hello: hello})
	a := netip.MustParseAddr("198.51.100.4")
	for i := range hello.Burst {
		if ok, _ := l.open(a, now); !ok {
			t.Fatalf("hello %d of the burst refused", i)
		}
		l.close(a)
	}
	if ok, wait := l.open(a, now); ok || wait != hello.Every {
		t.Fatalf("hello past the burst: ok %v, wait %v", ok, wait)
	}
	now = now.Add(hello.Every)
	if ok, _ := l.open(a, now); !ok {
		t.Fatal("no hello after a refill")
	}
	l.close(a)
	// A sweep drops an address once its bucket is full again, and keeps
	// one with open connections.
	b := netip.MustParseAddr("198.51.100.5")
	if ok, _ := l.open(b, now); !ok {
		t.Fatal("b refused")
	}
	now = now.Add(time.Hour)
	l.mu.Lock()
	l.sweep(now)
	_, keptA := l.addrs[netip.MustParsePrefix("198.51.100.4/32")]
	_, keptB := l.addrs[netip.MustParsePrefix("198.51.100.5/32")]
	l.mu.Unlock()
	if keptA || !keptB {
		t.Errorf("after a sweep: a kept %v, b kept %v", keptA, keptB)
	}
}

// TestConnectionsPerAddressThroughProxy checks the limits on real
// connections, with the client address from a trusted proxy's
// X-Forwarded-For, as cmd/waserver wires it.
func TestConnectionsPerAddressThroughProxy(t *testing.T) {
	g := &path.Grid{Width: 40, Height: 40, Cells: make([]uint8, 1600)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	s := httptest.NewServer(&Gateway{World: z, Sessions: DevSessions{},
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
		Conns:          ConnLimits{PerAddress: 2, Hello: Rate{Burst: 20, Every: time.Second}}})
	t.Cleanup(s.Close)
	url := "ws" + strings.TrimPrefix(s.URL, "http")
	from := func(addr string) (*client, int) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
			HTTPHeader: http.Header{"X-Forwarded-For": {addr}},
		})
		if err != nil {
			if resp == nil {
				t.Fatal(err)
			}
			return nil, resp.StatusCode
		}
		t.Cleanup(func() { _ = c.CloseNow() })
		return &client{t: t, c: c}, http.StatusSwitchingProtocols
	}
	a, _ := from("203.0.113.7")
	if a == nil {
		t.Fatal("first connection refused")
	}
	a.hello("First", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	b, _ := from("203.0.113.7")
	if b == nil {
		t.Fatal("second connection refused")
	}
	if c, code := from("203.0.113.7"); c != nil || code != http.StatusTooManyRequests {
		t.Fatalf("third connection: status %d, want 429", code)
	}
	// Another client behind the same proxy still connects.
	if c, _ := from("203.0.113.99"); c == nil {
		t.Fatal("another address was refused")
	}
	// Once a connection ends, its place is free.
	_ = b.c.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, code := from("203.0.113.7")
		if c != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no place after a close: status %d", code)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRetryAfter(t *testing.T) {
	for d, want := range map[time.Duration]string{0: "1", time.Millisecond: "1", 3 * time.Second: "3", 2500 * time.Millisecond: "3", time.Minute: "60"} {
		if got := retryAfter(d); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}
