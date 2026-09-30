package account

import (
	"net/netip"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestBucket(t *testing.T) {
	l := newLimiter(DefaultLimits)
	ip := netip.MustParseAddr("203.0.113.7")
	for i := range DefaultLimits.Login.Burst {
		if ok, _ := l.allow(actLogin, ip, t0); !ok {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	ok, wait := l.allow(actLogin, ip, t0)
	if ok || wait != DefaultLimits.Login.Every {
		t.Fatalf("over the burst: %v, wait %v", ok, wait)
	}
	// Another action has its own bucket, and another address too.
	if ok, _ := l.allow(actSignup, ip, t0); !ok {
		t.Error("signup shares the login bucket")
	}
	if ok, _ := l.allow(actLogin, netip.MustParseAddr("203.0.113.8"), t0); !ok {
		t.Error("another address shares the bucket")
	}
	// One token comes back after Every.
	if ok, _ := l.allow(actLogin, ip, t0.Add(DefaultLimits.Login.Every)); !ok {
		t.Error("no token after Every")
	}
	if ok, _ := l.allow(actLogin, ip, t0.Add(DefaultLimits.Login.Every)); ok {
		t.Error("two tokens after Every")
	}
}

func TestIPv6GroupsBy64(t *testing.T) {
	l := newLimiter(Limits{Login: Bucket{Burst: 1, Every: time.Minute}})
	if ok, _ := l.allow(actLogin, netip.MustParseAddr("2001:db8:1:2::1"), t0); !ok {
		t.Fatal("first refused")
	}
	if ok, _ := l.allow(actLogin, netip.MustParseAddr("2001:db8:1:2:ffff::9"), t0); ok {
		t.Error("another address in the same /64 has its own bucket")
	}
	if ok, _ := l.allow(actLogin, netip.MustParseAddr("2001:db8:1:3::1"), t0); !ok {
		t.Error("another /64 shares the bucket")
	}
	// IPv4-mapped IPv6 is the IPv4 address.
	if addrKey(netip.MustParseAddr("::ffff:203.0.113.7")) != addrKey(netip.MustParseAddr("203.0.113.7")) {
		t.Error("mapped address differs")
	}
}

func TestBackoffAndCaptcha(t *testing.T) {
	l := newLimiter(DefaultLimits)
	ip := netip.MustParseAddr("198.51.100.1")
	now := t0
	var waits []time.Duration
	for i := 1; i <= 8; i++ {
		wait, captcha := l.check("Alice", ip, now)
		if wait > 0 {
			t.Fatalf("attempt %d must wait %v", i, wait)
		}
		if want := i > DefaultLimits.AccountCaptcha; captcha != want {
			t.Errorf("attempt %d: captcha %v", i, captcha)
		}
		l.fail("alice", ip, now) // names match in any case
		wait, _ = l.check("ALICE", ip, now)
		waits = append(waits, wait)
		now = now.Add(wait)
	}
	want := []time.Duration{0, 0, 0, 0, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for i := range want {
		if waits[i] != want[i] {
			t.Errorf("after %d failures: wait %v, want %v", i+1, waits[i], want[i])
		}
	}
	// Another name from the same address has no wait yet.
	if wait, _ := l.check("bob", ip, now); wait != 0 {
		t.Errorf("another name waits %v", wait)
	}
	// Success resets the name.
	l.succeed("Alice")
	if wait, captcha := l.check("alice", ip, now); wait != 0 || captcha {
		t.Errorf("after success: %v %v", wait, captcha)
	}
}

func TestBackoffCap(t *testing.T) {
	l := newLimiter(DefaultLimits)
	if got := l.backoff(100); got != DefaultLimits.BackoffMax {
		t.Errorf("backoff(100) = %v", got)
	}
}

func TestIPCaptchaAndForget(t *testing.T) {
	l := newLimiter(DefaultLimits)
	ip := netip.MustParseAddr("198.51.100.2")
	for i := range DefaultLimits.IPCaptcha {
		// A different name each time, as a spraying attacker does.
		l.fail("name"+string(rune('a'+i)), ip, t0)
	}
	if !l.captchaFor(ip, t0) {
		t.Error("no captcha after IPCaptcha failures from one address")
	}
	if _, captcha := l.check("fresh", ip, t0); !captcha {
		t.Error("a fresh name from that address needs no captcha")
	}
	later := t0.Add(DefaultLimits.Forget + time.Second)
	if l.captchaFor(ip, later) {
		t.Error("the address is not forgotten")
	}
	l.sweep(later)
	if len(l.addrs) != 0 || len(l.names) != 0 {
		t.Errorf("sweep left %d addresses and %d names", len(l.addrs), len(l.names))
	}
}

func TestFullTablesRefuse(t *testing.T) {
	lim := DefaultLimits
	lim.MaxEntries = 2
	l := newLimiter(lim)
	ip := netip.MustParseAddr("192.0.2.1")
	l.fail("aaaa", ip, t0)
	l.fail("bbbb", ip, t0)
	// A third name cannot be counted, so it is refused, not let through
	// without limits.
	if wait, _ := l.check("cccc", ip, t0); wait == 0 {
		t.Error("a name that cannot be counted is allowed")
	}
	if wait, _ := l.check("aaaa", ip, t0); wait != 0 {
		t.Error("a counted name is refused")
	}
	if ok, _ := l.allow(actLogin, netip.MustParseAddr("192.0.2.2"), t0); !ok {
		t.Error("bucket table refused its first entries")
	}
	_, _ = l.allow(actLogin, netip.MustParseAddr("192.0.2.3"), t0)
	if ok, _ := l.allow(actLogin, netip.MustParseAddr("192.0.2.4"), t0); ok {
		t.Error("a full bucket table took another address")
	}
}
