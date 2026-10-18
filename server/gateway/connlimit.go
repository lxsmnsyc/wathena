package gateway

import (
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/lxsmnsyc/wathena/server/clientaddr"
)

// Connection limits per client address (strategy section 7.4). They
// replace rAthena's ddos_* settings in conf/packet_athena.conf:
//   - An address may hold PerAddress open game connections.
//   - An address may open Hello.Burst connections at once, then one every
//     Hello.Every. Each connection sends one Hello, so this is the rate of
//     Hellos, and of guesses at session tokens.
//   - A connection over a limit is refused with HTTP 429 and a
//     Retry-After header, before the WebSocket starts.
//   - The address is the one the account service sees: through
//     Gateway.TrustedProxies, an IPv4 address alone and an IPv6 address
//     by its /64 (clientaddr.Key).
//   - Loopback addresses are not limited. They are the host's own tools,
//     such as the load test bots, or a proxy on the same host that
//     trusted_proxies does not name, whose players would otherwise share
//     one address.
//
// The numbers sit above what a household or an internet cafe needs: the
// account service gives a game ticket at the same rate, 20 at once then
// one every 3 seconds per address.

// ConnLimits are the connection limits per client address.
type ConnLimits struct {
	PerAddress int
	Hello      Rate
}

// DefaultConnLimits are the limits a Gateway uses when its ConnLimits
// are zero.
func DefaultConnLimits() ConnLimits {
	return ConnLimits{PerAddress: 32, Hello: Rate{Burst: 20, Every: 3 * time.Second}}
}

// maxConnEntries bounds the table of addresses. When it is full of
// addresses with open connections or recent Hellos, a new address is
// refused until a sweep frees room.
const maxConnEntries = 100_000

// connLimit counts the open connections and the Hello bucket of each
// address.
type connLimit struct {
	mu     sync.Mutex
	limits ConnLimits
	addrs  map[netip.Prefix]*connEntry
	swept  time.Time
}

type connEntry struct {
	open  int
	hello bucket
}

func newConnLimit(l ConnLimits) *connLimit {
	return &connLimit{limits: l, addrs: map[netip.Prefix]*connEntry{}}
}

// open reports whether addr may open a connection at now. When it may,
// the caller must call close once the connection ends. When it may not,
// wait is how long the client should wait.
func (c *connLimit) open(addr netip.Addr, now time.Time) (ok bool, wait time.Duration) {
	if addr.IsLoopback() {
		return true, 0
	}
	k := clientaddr.Key(addr)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweep(now)
	e := c.addrs[k]
	if e == nil {
		if len(c.addrs) >= maxConnEntries {
			return false, time.Minute
		}
		e = &connEntry{}
		c.addrs[k] = e
	}
	if e.open >= c.limits.PerAddress {
		return false, time.Minute
	}
	if !e.hello.take(now, c.limits.Hello) {
		return false, c.limits.Hello.Every
	}
	e.open++
	return true, 0
}

// close ends a connection that open allowed.
func (c *connLimit) close(addr netip.Addr) {
	if addr.IsLoopback() {
		return
	}
	k := clientaddr.Key(addr)
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.addrs[k]; e != nil && e.open > 0 {
		e.open--
	}
}

// sweep drops, at most once a minute, the addresses with no open
// connection whose Hello bucket is full again.
func (c *connLimit) sweep(now time.Time) {
	if now.Sub(c.swept) < time.Minute {
		return
	}
	c.swept = now
	for k, e := range c.addrs {
		if e.open == 0 && !e.hello.full.After(now) {
			delete(c.addrs, k)
		}
	}
}

// retryAfter is a Retry-After header value: whole seconds, at least 1.
func retryAfter(d time.Duration) string {
	return strconv.Itoa(max(1, int((d+time.Second-1)/time.Second)))
}
