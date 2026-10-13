package metrics

import (
	"slices"
	"sync"
	"time"
)

// The zone's last minute of ticks, for the admin API's world view
// (decision 0031):
//   - Each tick's work is kept with its time in a ring of WindowTicks,
//     more than a minute of 50 ms ticks.
//   - The ring is made at the zone's first tick, so a zone that never
//     had a player costs nothing.
//   - Only the zone's goroutine writes. Any goroutine may read. A
//     mutex guards the ring, and the zone takes it once per tick.

// Window is how far back Recent looks.
const Window = time.Minute

// WindowTicks is the size of the ring: 64 seconds of 50 ms ticks.
const WindowTicks = 1280

type window struct {
	mu   sync.Mutex
	at   []int64
	work []time.Duration
	next int
}

func (w *window) record(at time.Time, work time.Duration) {
	w.mu.Lock()
	if w.at == nil {
		w.at, w.work = make([]int64, WindowTicks), make([]time.Duration, WindowTicks)
	}
	w.at[w.next], w.work[w.next] = at.UnixNano(), work
	w.next = (w.next + 1) % WindowTicks
	w.mu.Unlock()
}

// Recent is a zone's ticks of the last minute.
type Recent struct {
	Ticks int
	// P50, P99 and Max are of the ticks' work: the tick and what the
	// zone handled since the tick before.
	P50, P99, Max time.Duration
	// OverBudget counts the ticks whose work was over TickBudget.
	OverBudget int
}

// Recent summarizes the ticks of the minute before now.
func (z *Zone) Recent(now time.Time) Recent {
	from := now.Add(-Window).UnixNano()
	w := &z.window
	w.mu.Lock()
	var works []time.Duration
	for i, at := range w.at {
		if at > from && at <= now.UnixNano() {
			works = append(works, w.work[i])
		}
	}
	w.mu.Unlock()
	r := Recent{Ticks: len(works)}
	if len(works) == 0 {
		return r
	}
	slices.Sort(works)
	for _, d := range works {
		if d > TickBudget {
			r.OverBudget++
		}
	}
	q := func(f float64) time.Duration { return works[min(int(f*float64(len(works))), len(works)-1)] }
	r.P50, r.P99, r.Max = q(0.5), q(0.99), works[len(works)-1]
	return r
}

// Sleeping reports whether the zone stopped ticking because it has no
// players.
func (z *Zone) Sleeping() bool { return z.asleep.Load() }
