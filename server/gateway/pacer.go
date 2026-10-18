package gateway

import (
	"sync"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Pacer keeps a client within a connection's rates, so the gateway
// drops none of its messages. Test bots that act faster than people,
// such as the duplication bots of the e2e tests, use one. Load test bots
// do not, so their runs show whether the rates refuse normal play.
type Pacer struct {
	mu    sync.Mutex
	rates Rates
	all   bucket
	kinds [kindCount]bucket
}

// NewPacer returns a Pacer for rates, DefaultRates when they are zero.
// It keeps a margin under each rate: two tokens fewer at once, and a
// tenth longer for each, since messages can reach the gateway closer
// together than they were sent.
func NewPacer(rates Rates) *Pacer {
	if rates == (Rates{}) {
		rates = DefaultRates()
	}
	for _, r := range []*Rate{&rates.All, &rates.Ping, &rates.Walk, &rates.Action, &rates.Drop, &rates.NPC, &rates.Storage, &rates.Social, &rates.Search} {
		r.Burst = max(1, r.Burst-2)
		r.Every += r.Every / 10
	}
	return &Pacer{rates: rates}
}

// Reserve takes the tokens that m needs and returns how long the client
// must wait, from now, before it sends m.
func (p *Pacer) Reserve(now time.Time, m *wathenav1.ClientMessage) time.Duration {
	k := kindOf(m)
	if k == kindChat || m.HasHello() {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	wait := reserve(&p.all, now, p.rates.All)
	if k != kindOther {
		wait = max(wait, reserve(&p.kinds[k], now, p.rates.of(k)))
	}
	return wait
}

// reserve takes a token of b, even one that is not back yet, and returns
// how long until it is.
func reserve(b *bucket, now time.Time, r Rate) time.Duration {
	next := b.full
	if next.Before(now) {
		next = now
	}
	next = next.Add(r.Every)
	b.full = next
	return max(0, next.Sub(now)-time.Duration(r.Burst)*r.Every)
}
