// Package metrics measures the server against the budgets in strategy
// section 3.8: tick work per zone, garbage collector pauses, memory,
// CPU, and bytes per connection. Zones and connections record into it
// with atomic counters, so recording never blocks them. Handler serves
// the numbers as JSON on a debug address, and Snapshot.Text prints them.
package metrics

import (
	"math/bits"
	"sync/atomic"
	"time"
)

// Histogram buckets: one for everything under 1024 ns, then subBuckets
// per power of two up to 2^maxExp ns (about 69 seconds). A bucket's
// width is 1/8 of its lower bound, so a quantile is off by at most
// 12.5%.
const (
	subBits    = 3
	subBuckets = 1 << subBits
	minExp     = 10
	maxExp     = 36
	nBuckets   = 1 + (maxExp-minExp)*subBuckets
)

// Histogram counts durations. Any number of goroutines may record and
// read at once.
type Histogram struct {
	counts [nBuckets]atomic.Uint64
	n      atomic.Uint64
	sum    atomic.Int64
	max    atomic.Int64
}

func bucket(ns int64) int {
	if ns < 1<<minExp {
		return 0
	}
	e := bits.Len64(uint64(ns)) - 1
	if e >= maxExp {
		return nBuckets - 1
	}
	sub := int(ns>>(e-subBits)) & (subBuckets - 1)
	return 1 + (e-minExp)*subBuckets + sub
}

// upper is the largest duration a bucket holds, in nanoseconds.
func upper(i int) int64 {
	if i == 0 {
		return 1<<minExp - 1
	}
	e := (i-1)/subBuckets + minExp
	sub := int64((i - 1) % subBuckets)
	return (subBuckets+sub+1)<<(e-subBits) - 1
}

// Record adds one duration.
func (h *Histogram) Record(d time.Duration) {
	ns := int64(d)
	if ns < 0 {
		ns = 0
	}
	h.counts[bucket(ns)].Add(1)
	h.n.Add(1)
	h.sum.Add(ns)
	for {
		m := h.max.Load()
		if ns <= m || h.max.CompareAndSwap(m, ns) {
			break
		}
	}
}

// Reset clears the histogram. Durations recorded during a Reset may be
// partly kept.
func (h *Histogram) Reset() {
	for i := range h.counts {
		h.counts[i].Store(0)
	}
	h.n.Store(0)
	h.sum.Store(0)
	h.max.Store(0)
}

// Summary is a histogram's count, mean, quantiles and maximum.
type Summary struct {
	Count uint64        `json:"count"`
	Mean  time.Duration `json:"mean_ns"`
	P50   time.Duration `json:"p50_ns"`
	P99   time.Duration `json:"p99_ns"`
	P999  time.Duration `json:"p999_ns"`
	Max   time.Duration `json:"max_ns"`
}

// Summary returns the histogram's summary. A quantile is the upper bound
// of its bucket, but never more than the maximum.
func (h *Histogram) Summary() Summary {
	var counts [nBuckets]uint64
	var n uint64
	for i := range counts {
		counts[i] = h.counts[i].Load()
		n += counts[i]
	}
	s := Summary{Count: n, Max: time.Duration(h.max.Load())}
	if n == 0 {
		return s
	}
	s.Mean = time.Duration(h.sum.Load() / int64(n)) // #nosec G115 -- a count of ticks.
	q := func(f float64) time.Duration {
		rank := uint64(f * float64(n))
		if rank >= n {
			rank = n - 1
		}
		var seen uint64
		for i, c := range counts {
			seen += c
			if seen > rank {
				return min(time.Duration(upper(i)), s.Max)
			}
		}
		return s.Max
	}
	s.P50, s.P99, s.P999 = q(0.5), q(0.99), q(0.999)
	return s
}
