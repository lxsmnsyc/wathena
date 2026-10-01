package metrics

import (
	"testing"
	"time"
)

func TestOverBudget(t *testing.T) {
	z := &Zone{Name: "map/test"}
	calls := 0
	z.OnOverBudget = func(_ *Zone, recent []time.Duration) {
		calls++
		if len(recent) != OverBudgetStreak {
			t.Errorf("got %d recent ticks", len(recent))
		}
	}
	over := TickBudget + time.Millisecond
	// 9 ticks over, one under, then 10 over: one call.
	for range OverBudgetStreak - 1 {
		z.Tick(over, 1)
	}
	z.Tick(time.Millisecond, 1)
	for range OverBudgetStreak {
		z.Tick(over, 1)
	}
	if calls != 1 {
		t.Errorf("OnOverBudget ran %d times, want 1", calls)
	}
	// Work between ticks counts toward the next tick.
	z.Handled(TickMax)
	z.Tick(time.Millisecond, 1)
	s := z.Snapshot()
	if s.Ticks != 21 || s.OverBudget != 2*OverBudgetStreak || s.OverMax != 1 {
		t.Errorf("snapshot %+v", s)
	}
	if s.Work.Max != TickMax+time.Millisecond || s.Step.Max != over {
		t.Errorf("work max %v, step max %v", s.Work.Max, s.Step.Max)
	}
}
