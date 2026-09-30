package account

import (
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Limits set the rate limits of the account pages. The limits live in
// the memory of one process:
//   - A failed login does not write to the database, so a flood of
//     guesses cannot turn into a flood of writes.
//   - A restart forgets them. Only the server owner can restart the
//     server, so an attacker cannot use that to reset them.
//   - With several account service processes behind one address, each
//     keeps its own counts, so the limits multiply by the number of
//     processes. Moving them to shared storage is left for when the
//     account service runs as more than one process.
type Limits struct {
	// Each client address (an IPv4 address, or an IPv6 /64) has a
	// bucket per action: Burst attempts, refilled one every Every.
	Login, Signup, Ticket Bucket
	// After AccountCaptcha failed logins for one account name, or
	// IPCaptcha from one address, the login form asks for a captcha.
	AccountCaptcha, IPCaptcha int
	// From BackoffAfter failed logins for one account name, each further
	// login for it waits BackoffBase, doubled per failure after the
	// first, up to BackoffMax.
	BackoffAfter int
	BackoffBase  time.Duration
	BackoffMax   time.Duration
	// Forget is how long after the last failure the failure counts go
	// back to 0. A successful login resets its account name's count.
	Forget time.Duration
	// MaxEntries bounds each table of counts. When a table is full of
	// entries still in use, new addresses and names are refused until
	// old ones expire.
	MaxEntries int
}

// Bucket is a token bucket: Burst at most, one more every Every.
type Bucket struct {
	Burst int
	Every time.Duration
}

// DefaultLimits:
//   - Login: 10 attempts at once, then 1 every 6 seconds (10 a minute)
//     per address.
//   - Signup: 3 at once, then 1 every 10 minutes per address.
//   - Ticket: 20 at once, then 1 every 3 seconds per address.
//   - A captcha after 3 failures for a name or 10 from an address.
//   - From 5 failures for a name, a wait of 1 s, 2 s, 4 s and so on, up
//     to 15 minutes.
//   - Counts are forgotten an hour after the last failure.
var DefaultLimits = Limits{
	Login:          Bucket{Burst: 10, Every: 6 * time.Second},
	Signup:         Bucket{Burst: 3, Every: 10 * time.Minute},
	Ticket:         Bucket{Burst: 20, Every: 3 * time.Second},
	AccountCaptcha: 3,
	IPCaptcha:      10,
	BackoffAfter:   5,
	BackoffBase:    time.Second,
	BackoffMax:     15 * time.Minute,
	Forget:         time.Hour,
	MaxEntries:     100_000,
}

type action uint8

const (
	actLogin action = iota
	actSignup
	actTicket
)

type bucketKey struct {
	act  action
	addr netip.Prefix
}

type bucket struct {
	tokens float64
	at     time.Time
}

type failures struct {
	n    int
	last time.Time
}

// limiter keeps the counts for Limits.
type limiter struct {
	lim Limits

	mu      sync.Mutex
	buckets map[bucketKey]*bucket
	names   map[string]*failures
	addrs   map[netip.Prefix]*failures
}

func newLimiter(lim Limits) *limiter {
	return &limiter{
		lim:     lim,
		buckets: map[bucketKey]*bucket{},
		names:   map[string]*failures{},
		addrs:   map[netip.Prefix]*failures{},
	}
}

// addrKey groups IPv6 addresses by /64, the block one customer usually
// gets, so one client cannot spread over many addresses.
func addrKey(a netip.Addr) netip.Prefix {
	a = a.Unmap()
	bits := 32
	if a.Is6() {
		bits = 64
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return netip.Prefix{}
	}
	return p
}

func (l *limiter) bucketOf(act action) Bucket {
	switch act {
	case actSignup:
		return l.lim.Signup
	case actTicket:
		return l.lim.Ticket
	default:
		return l.lim.Login
	}
}

// allow takes one token from an address's bucket for an action. It
// returns how long to wait when the bucket is empty.
func (l *limiter) allow(act action, addr netip.Addr, now time.Time) (bool, time.Duration) {
	cfg := l.bucketOf(act)
	if cfg.Burst <= 0 || cfg.Every <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	k := bucketKey{act, addrKey(addr)}
	b := l.buckets[k]
	if b == nil {
		if l.full(func() int { return len(l.buckets) }, now) {
			return false, cfg.Every
		}
		b = &bucket{tokens: float64(cfg.Burst), at: now}
		l.buckets[k] = b
	}
	b.tokens = min(float64(cfg.Burst), b.tokens+float64(now.Sub(b.at))/float64(cfg.Every))
	b.at = now
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) * float64(cfg.Every))
	}
	b.tokens--
	return true, 0
}

// full reports whether a table cannot take one more entry. When it is
// full, it sweeps expired entries first. l.mu must be held.
func (l *limiter) full(size func() int, now time.Time) bool {
	if l.lim.MaxEntries <= 0 || size() < l.lim.MaxEntries {
		return false
	}
	l.sweepLocked(now)
	return size() >= l.lim.MaxEntries
}

// nameKey is an account name as the database compares it.
func nameKey(name string) string { return strings.ToLower(name) }

// check returns, before a login attempt, how long the name must still
// wait and whether a captcha is needed. It applies to every name, taken
// or not, so its answers do not tell which accounts exist.
func (l *limiter) check(name string, addr netip.Addr, now time.Time) (wait time.Duration, captcha bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.names[nameKey(name)]; !ok && l.full(func() int { return len(l.names) }, now) {
		// The table is full, so a failure for this name could not be
		// counted. Refuse the attempt instead of allowing it unlimited.
		return time.Minute, true
	}
	f := l.live(l.names[nameKey(name)], now)
	a := l.live(l.addrs[addrKey(addr)], now)
	if l.lim.AccountCaptcha > 0 && f.n >= l.lim.AccountCaptcha || l.lim.IPCaptcha > 0 && a.n >= l.lim.IPCaptcha {
		captcha = true
	}
	if until := f.last.Add(l.backoff(f.n)); f.n >= l.lim.BackoffAfter && l.lim.BackoffAfter > 0 && until.After(now) {
		wait = until.Sub(now)
	}
	return wait, captcha
}

// captchaFor reports whether an address must solve a captcha, to show
// the field on the login form before the name is known.
func (l *limiter) captchaFor(addr netip.Addr, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.live(l.addrs[addrKey(addr)], now)
	return l.lim.IPCaptcha > 0 && a.n >= l.lim.IPCaptcha
}

// backoff is the wait after n failures.
func (l *limiter) backoff(n int) time.Duration {
	if l.lim.BackoffAfter <= 0 || n < l.lim.BackoffAfter {
		return 0
	}
	d := l.lim.BackoffBase
	for i := l.lim.BackoffAfter; i < n && d < l.lim.BackoffMax; i++ {
		d *= 2
	}
	return min(d, l.lim.BackoffMax)
}

// live returns the counts that still apply, or zero counts.
func (l *limiter) live(f *failures, now time.Time) failures {
	if f == nil || now.Sub(f.last) > l.lim.Forget {
		return failures{}
	}
	return *f
}

// fail records a failed login for a name and an address. It returns
// false when the tables are full.
func (l *limiter) fail(name string, addr netip.Addr, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	byName := bump(l, l.names, nameKey(name), now)
	byAddr := bump(l, l.addrs, addrKey(addr), now)
	return byName && byAddr
}

func bump[K comparable](l *limiter, m map[K]*failures, k K, now time.Time) bool {
	f := m[k]
	if f == nil {
		if l.full(func() int { return len(m) }, now) {
			return false
		}
		f = &failures{}
		m[k] = f
	}
	if now.Sub(f.last) > l.lim.Forget {
		f.n = 0
	}
	f.n++
	f.last = now
	return true
}

// failAddr records a failure for an address only, such as a passkey
// that did not work, whose account is not known.
func (l *limiter) failAddr(addr netip.Addr, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bump(l, l.addrs, addrKey(addr), now)
}

// succeed forgets a name's failures after a successful login.
func (l *limiter) succeed(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.names, nameKey(name))
}

// sweep removes entries that no longer limit anything.
func (l *limiter) sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(now)
}

func (l *limiter) sweepLocked(now time.Time) {
	for k, b := range l.buckets {
		cfg := l.bucketOf(k.act)
		// A bucket that has refilled is the same as no bucket.
		if now.Sub(b.at) >= time.Duration(cfg.Burst)*cfg.Every {
			delete(l.buckets, k)
		}
	}
	for k, f := range l.names {
		if now.Sub(f.last) > l.lim.Forget {
			delete(l.names, k)
		}
	}
	for k, f := range l.addrs {
		if now.Sub(f.last) > l.lim.Forget {
			delete(l.addrs, k)
		}
	}
}
