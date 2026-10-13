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

func TestRecent(t *testing.T) {
	z := &Zone{Name: "map/test"}
	now := time.Now()
	if r := z.Recent(now); r.Ticks != 0 {
		t.Errorf("a zone that never ticked: %+v", r)
	}
	// A tick older than a minute is left out.
	z.window.record(now.Add(-2*time.Minute), time.Second)
	for i := range 100 {
		z.window.record(now.Add(-time.Duration(i)*100*time.Millisecond), time.Duration(i+1)*time.Millisecond)
	}
	r := z.Recent(now)
	if r.Ticks != 100 || r.Max != 100*time.Millisecond || r.P50 != 51*time.Millisecond || r.P99 != 100*time.Millisecond {
		t.Errorf("recent %+v", r)
	}
	if r.OverBudget != 90 {
		t.Errorf("over budget %d, want 90", r.OverBudget)
	}
	// The ring keeps the newest WindowTicks.
	for i := range WindowTicks + 10 {
		z.window.record(now.Add(time.Duration(i)*time.Millisecond), time.Millisecond)
	}
	if r := z.Recent(now.Add(2 * time.Second)); r.Ticks != WindowTicks || r.Max != time.Millisecond {
		t.Errorf("after the ring wrapped: %+v", r)
	}
	z.Asleep()
	if !z.Sleeping() {
		t.Error("not sleeping after Asleep")
	}
	z.Tick(time.Millisecond, 1)
	if z.Sleeping() {
		t.Error("sleeping after a tick")
	}
}
