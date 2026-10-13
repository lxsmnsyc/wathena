package restart

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type recorder struct {
	said    []string
	notices []string
	due     []Plan
}

func newPlanner(c *fakeClock, r *recorder) *Planner {
	return New(Options{Clock: c.Now,
		Announce: func(text string) { r.said = append(r.said, text) },
		Notify: func(p *Plan) {
			if p == nil {
				r.notices = append(r.notices, "cancelled")
				return
			}
			r.notices = append(r.notices, "planned "+p.Reason)
		},
		Due: func(p Plan) { r.due = append(r.due, p) },
	})
}

var start = time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)

// TestPlanAnnouncesAndCloses: a plan 12 minutes ahead announces at 10,
// 5 and 1 minute and 30 seconds, closes logins in the last minute, and
// runs Due once at its time.
func TestPlanAnnouncesAndCloses(t *testing.T) {
	c := &fakeClock{now: start}
	r := &recorder{}
	p := newPlanner(c, r)
	pl, err := p.Plan(Plan{At: start.Add(12 * time.Minute), Reason: "maintenance", Message: "New maps!", By: "Admin"})
	if err != nil {
		t.Fatal(err)
	}
	if !pl.Planned.Equal(start) || strings.Join(r.notices, "|") != "planned maintenance" {
		t.Fatalf("plan %+v, notices %q", pl, r.notices)
	}
	step := func(d time.Duration) {
		c.now = c.now.Add(d)
		p.Check(c.now)
	}
	step(time.Minute)
	if len(r.said) != 0 {
		t.Fatalf("said %q at 11 minutes left", r.said)
	}
	step(time.Minute) // 10 minutes left
	step(4 * time.Minute)
	step(time.Minute) // 5 minutes left
	if _, closed := p.Closed(); closed {
		t.Fatal("logins closed at 5 minutes left")
	}
	step(4 * time.Minute) // 1 minute left
	text, closed := p.Closed()
	if !closed || !strings.Contains(text, "maintenance") {
		t.Fatalf("logins at 1 minute left: %q %v", text, closed)
	}
	step(30 * time.Second)
	want := []string{
		"The server restarts in 10 minutes. New maps!",
		"The server restarts in 5 minutes. New maps!",
		"The server restarts in 1 minute. New maps!",
		"The server restarts in 30 seconds. New maps!",
	}
	if strings.Join(r.said, "|") != strings.Join(want, "|") {
		t.Fatalf("said %q", r.said)
	}
	step(29 * time.Second)
	if len(r.due) != 0 {
		t.Fatal("Due ran early")
	}
	step(time.Second)
	step(time.Second)
	if len(r.due) != 1 || r.due[0].Reason != "maintenance" {
		t.Fatalf("due %+v", r.due)
	}
	if _, err := p.Cancel(); !errors.Is(err, ErrTooLate) {
		t.Fatalf("cancel after the time: %v", err)
	}
	if !strings.Contains(DisconnectText(r.due[0]), "restarting: maintenance") {
		t.Fatal(DisconnectText(r.due[0]))
	}
}

// TestClockJump: when the clock passes several marks at once, only the
// last is announced.
func TestClockJump(t *testing.T) {
	c := &fakeClock{now: start}
	r := &recorder{}
	p := newPlanner(c, r)
	if _, err := p.Plan(Plan{At: start.Add(30 * time.Minute), Reason: "update"}); err != nil {
		t.Fatal(err)
	}
	p.Check(c.now)
	c.now = start.Add(29*time.Minute + 10*time.Second)
	p.Check(c.now)
	if strings.Join(r.said, "|") != "The server restarts in 30 minutes. update|The server restarts in 1 minute. update" {
		t.Fatalf("said %q", r.said)
	}
}

// TestPlanRules: one plan at a time, from 1 minute to 7 days ahead, with
// a reason. Cancelling announces it and tells the world.
func TestPlanRules(t *testing.T) {
	c := &fakeClock{now: start}
	r := &recorder{}
	p := newPlanner(c, r)
	for _, pl := range []Plan{
		{At: start.Add(30 * time.Second), Reason: "soon"},
		{At: start.Add(8 * 24 * time.Hour), Reason: "late"},
		{At: start.Add(-time.Hour), Reason: "past"},
	} {
		if _, err := p.Plan(pl); !errors.Is(err, ErrNotTime) {
			t.Errorf("%s: %v", pl.Reason, err)
		}
	}
	if _, err := p.Plan(Plan{At: start.Add(time.Hour)}); !errors.Is(err, ErrNoReason) {
		t.Errorf("no reason: %v", err)
	}
	if _, err := p.Cancel(); !errors.Is(err, ErrNoPlan) {
		t.Errorf("cancel without a plan: %v", err)
	}
	if _, err := p.Plan(Plan{At: start.Add(time.Hour), Reason: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Plan(Plan{At: start.Add(2 * time.Hour), Reason: "second"}); !errors.Is(err, ErrPlanned) {
		t.Errorf("a second plan: %v", err)
	}
	pl, err := p.Cancel()
	if err != nil || pl.Reason != "first" {
		t.Fatalf("cancel: %+v %v", pl, err)
	}
	if _, ok := p.Current(); ok {
		t.Fatal("a plan is left after cancel")
	}
	if strings.Join(r.notices, "|") != "planned first|cancelled" || strings.Join(r.said, "|") != "The planned server restart is cancelled." {
		t.Fatalf("notices %q, said %q", r.notices, r.said)
	}
	// After a cancel, the clock passing the old time does nothing.
	c.now = start.Add(2 * time.Hour)
	p.Check(c.now)
	if len(r.due) != 0 {
		t.Fatal("a cancelled plan ran")
	}
}

func TestLeft(t *testing.T) {
	for d, want := range map[time.Duration]string{time.Minute: "1 minute", 15 * time.Minute: "15 minutes", 30 * time.Second: "30 seconds", time.Second: "1 second"} {
		if got := Left(d); got != want {
			t.Errorf("%v: %q", d, got)
		}
	}
}
