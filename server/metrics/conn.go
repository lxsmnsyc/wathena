package metrics

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Conns counts the bytes and messages of every connection. Bytes are
// WebSocket bytes: the payload and the frame header, with the mask key
// on client frames. TCP, IP and TLS headers are not counted.
type Conns struct {
	opened, closed atomic.Uint64

	mu   sync.Mutex
	live map[*Conn]struct{}
	// done adds up the connections that closed since the last reset.
	done connTotals
	// reset is when the last reset happened.
	reset time.Time
}

type connTotals struct {
	bytesIn, bytesOut, msgsIn, msgsOut uint64
}

// Conn is one connection's counts.
type Conn struct {
	bytesIn, bytesOut, msgsIn, msgsOut atomic.Uint64

	// Guarded by Conns.mu.
	start           time.Time
	base            connTotals
	lastIn, lastOut uint64
	peakIn, peakOut uint64
	sampled         bool
}

// wsHeader is the size of a WebSocket frame header for a payload of n
// bytes, without a mask key.
func wsHeader(n int) uint64 {
	switch {
	case n <= 125:
		return 2
	case n <= 0xFFFF:
		return 4
	}
	return 10
}

// Sent records a server message of n payload bytes. A nil Conn counts
// nothing.
func (c *Conn) Sent(n int) {
	if c == nil {
		return
	}
	c.bytesOut.Add(uint64(n) + wsHeader(n)) // #nosec G115 -- a message size.
	c.msgsOut.Add(1)
}

// Received records a client message of n payload bytes. Client frames
// carry a 4-byte mask key. A nil Conn counts nothing.
func (c *Conn) Received(n int) {
	if c == nil {
		return
	}
	c.bytesIn.Add(uint64(n) + wsHeader(n) + 4) // #nosec G115 -- a message size.
	c.msgsIn.Add(1)
}

func (c *Conn) totals() connTotals {
	return connTotals{c.bytesIn.Load(), c.bytesOut.Load(), c.msgsIn.Load(), c.msgsOut.Load()}
}

// Open starts counting a new connection. On a nil Conns, it returns
// nil.
func (cs *Conns) Open() *Conn {
	if cs == nil {
		return nil
	}
	cs.opened.Add(1)
	c := &Conn{}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.live == nil {
		cs.live = map[*Conn]struct{}{}
	}
	c.start = time.Now()
	cs.live[c] = struct{}{}
	return c
}

// Close stops counting a connection. Its counts stay in the totals.
func (cs *Conns) Close(c *Conn) {
	if cs == nil || c == nil {
		return
	}
	cs.closed.Add(1)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.live, c)
	t := c.totals()
	cs.done.bytesIn += t.bytesIn - c.base.bytesIn
	cs.done.bytesOut += t.bytesOut - c.base.bytesOut
	cs.done.msgsIn += t.msgsIn - c.base.msgsIn
	cs.done.msgsOut += t.msgsOut - c.base.msgsOut
}

// Run samples every connection once a second until ctx ends, for the
// peak rates.
func (cs *Conns) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cs.sample()
		}
	}
}

func (cs *Conns) sample() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for c := range cs.live {
		in, out := c.bytesIn.Load(), c.bytesOut.Load()
		// The first sample of a connection starts its peak window.
		if c.sampled {
			c.peakIn = max(c.peakIn, in-c.lastIn)
			c.peakOut = max(c.peakOut, out-c.lastOut)
		}
		c.lastIn, c.lastOut, c.sampled = in, out, true
	}
}

// Reset starts the counts again from now.
func (cs *Conns) Reset() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	now := time.Now()
	cs.reset = now
	cs.done = connTotals{}
	for c := range cs.live {
		c.base = c.totals()
		c.start = now
		c.peakIn, c.peakOut, c.sampled = 0, 0, false
	}
}

// Spread is how a value spreads over connections.
type Spread struct {
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
}

func spread(v []float64) Spread {
	if len(v) == 0 {
		return Spread{}
	}
	slices.Sort(v)
	var sum float64
	for _, x := range v {
		sum += x
	}
	at := func(q float64) float64 { return v[min(len(v)-1, int(q*float64(len(v))))] }
	return Spread{Mean: sum / float64(len(v)), P50: at(0.5), P99: at(0.99), Max: v[len(v)-1]}
}

// ConnSnapshot is the connections' counts.
type ConnSnapshot struct {
	Live   int    `json:"live"`
	Opened uint64 `json:"opened"`
	Closed uint64 `json:"closed"`
	// BytesIn and the other totals cover every connection since the
	// last reset, open or closed.
	BytesIn  uint64 `json:"bytes_in"`
	BytesOut uint64 `json:"bytes_out"`
	MsgsIn   uint64 `json:"msgs_in"`
	MsgsOut  uint64 `json:"msgs_out"`
	// InRate and OutRate are the open connections' average bytes per
	// second since they opened or since the last reset.
	InRate  Spread `json:"in_rate"`
	OutRate Spread `json:"out_rate"`
	// PeakIn and PeakOut are the open connections' most bytes in one
	// second.
	PeakIn  Spread `json:"peak_in"`
	PeakOut Spread `json:"peak_out"`
}

// Snapshot returns the connections' counts.
func (cs *Conns) Snapshot() ConnSnapshot {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	s := ConnSnapshot{
		Live: len(cs.live), Opened: cs.opened.Load(), Closed: cs.closed.Load(),
		BytesIn: cs.done.bytesIn, BytesOut: cs.done.bytesOut, MsgsIn: cs.done.msgsIn, MsgsOut: cs.done.msgsOut,
	}
	now := time.Now()
	var in, out, peakIn, peakOut []float64
	for c := range cs.live {
		t := c.totals()
		t.bytesIn -= c.base.bytesIn
		t.bytesOut -= c.base.bytesOut
		t.msgsIn -= c.base.msgsIn
		t.msgsOut -= c.base.msgsOut
		s.BytesIn += t.bytesIn
		s.BytesOut += t.bytesOut
		s.MsgsIn += t.msgsIn
		s.MsgsOut += t.msgsOut
		secs := now.Sub(c.start).Seconds()
		if secs < 1 {
			continue
		}
		in = append(in, float64(t.bytesIn)/secs)
		out = append(out, float64(t.bytesOut)/secs)
		peakIn = append(peakIn, float64(c.peakIn))
		peakOut = append(peakOut, float64(c.peakOut))
	}
	s.InRate, s.OutRate = spread(in), spread(out)
	s.PeakIn, s.PeakOut = spread(peakIn), spread(peakOut)
	return s
}
