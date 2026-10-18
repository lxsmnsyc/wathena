package account

import (
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// A flood of failed logins for made-up names fills the name table. A
// player whose name has no entry must still be able to log in, since
// the per-address bucket still limits the flood.
func TestFullNameTableDoesNotLockOutNewNames(t *testing.T) {
	lim := DefaultLimits
	lim.MaxEntries = 100
	l := newLimiter(lim)
	for i := 0; i < lim.MaxEntries; i++ {
		ip := netip.MustParseAddr(fmt.Sprintf("2001:db8:%x::1", i))
		l.fail(fmt.Sprintf("r4nd0m_%d", i), ip, t0)
	}
	victim := netip.MustParseAddr("192.0.2.77")
	wait, captcha := l.check("realplayer", victim, t0)
	if wait != 0 || captcha {
		t.Errorf("a player with no failures is refused: wait=%v captcha=%v", wait, captcha)
	}
	wait, captcha, release := l.reserve("realplayer", victim, t0)
	release()
	if wait != 0 || captcha {
		t.Errorf("a reserve for a player with no failures is refused: wait=%v captcha=%v", wait, captcha)
	}
	if len(l.names) > lim.MaxEntries {
		t.Errorf("the name table grew to %d entries", len(l.names))
	}
}

// Many attempts for one name that arrive together, from many addresses,
// get at most BackoffAfter through before any of them failed. Then one
// at a time goes after each backoff.
func TestConcurrentAttemptsAreReserved(t *testing.T) {
	l := newLimiter(DefaultLimits)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		passed   int
		releases []func()
	)
	for i := range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ip := netip.MustParseAddr(fmt.Sprintf("2001:db8:%x::1", i))
			wait, _, release := l.reserve("victim", ip, t0)
			if wait == 0 {
				mu.Lock()
				passed++
				releases = append(releases, release)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if passed != DefaultLimits.BackoffAfter {
		t.Fatalf("%d attempts went at once, want %d", passed, DefaultLimits.BackoffAfter)
	}
	// They all fail. The name now waits its backoff.
	for i, release := range releases {
		l.fail("victim", netip.MustParseAddr(fmt.Sprintf("2001:db8:%x::1", i)), t0)
		release()
	}
	if wait, _, _ := l.reserve("victim", netip.MustParseAddr("192.0.2.9"), t0); wait == 0 {
		t.Error("no wait after BackoffAfter failures")
	}
	// After the backoff, one attempt goes, and a second waits for it.
	later := t0.Add(time.Minute)
	wait, _, first := l.reserve("victim", netip.MustParseAddr("192.0.2.9"), later)
	if wait != 0 {
		t.Fatalf("the attempt after the backoff waits %v", wait)
	}
	if wait, _, _ := l.reserve("victim", netip.MustParseAddr("192.0.2.10"), later); wait == 0 {
		t.Error("a second attempt went while the first was checked")
	}
	first()
	// A release that ran twice does not free a second slot.
	first()
	if l.names["victim"].inflight != 0 {
		t.Errorf("in flight after the release: %d", l.names["victim"].inflight)
	}
}

// A sweep keeps an entry whose attempt is still being checked.
func TestSweepKeepsAttemptsInFlight(t *testing.T) {
	l := newLimiter(DefaultLimits)
	_, _, release := l.reserve("slow", netip.MustParseAddr("192.0.2.1"), t0)
	l.sweep(t0.Add(2 * DefaultLimits.Forget))
	if l.names["slow"] == nil {
		t.Fatal("the sweep removed an entry with an attempt in flight")
	}
	release()
	l.sweep(t0.Add(2 * DefaultLimits.Forget))
	if l.names["slow"] != nil {
		t.Error("the sweep kept an idle entry")
	}
}
