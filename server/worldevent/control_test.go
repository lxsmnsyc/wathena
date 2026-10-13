package worldevent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/schedule"
)

// apply queues a change and runs Check at now, as the scheduler's
// goroutine does, and returns the change's result.
func apply(t *testing.T, s *Scheduler, c Change, now time.Time) error {
	t.Helper()
	done := s.Queue(c)
	s.Check(context.Background(), now)
	select {
	case err := <-done:
		return err
	default:
		t.Fatal("Check did not apply the change")
		return nil
	}
}

func statusOf(s *Scheduler, name string) Status {
	for _, st := range s.Status() {
		if st.Def.Name == name {
			return st
		}
	}
	return Status{}
}

// TestChangeTrigger: a new trigger applies at once, from the time of the
// change.
func TestChangeTrigger(t *testing.T) {
	run := &fakeRunner{}
	var changed []Def
	s := New([]Def{boss}, Options{Runner: run, Changed: func(on []Def) { changed = on }})
	if err := s.Load(context.Background(), at(19, 0)); err != nil {
		t.Fatal(err)
	}
	if st := statusOf(s, boss.Name); !st.On || !st.Next.Equal(at(20, 0)) {
		t.Fatalf("status at load: %+v", st)
	}
	d := boss
	d.Trigger.Spec.Hour = 21
	d.Duration = 30 * time.Minute
	if err := apply(t, s, Change{Event: boss.Name, Set: &Setting{Def: d, On: true}}, at(19, 1)); err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].Trigger.Spec.Hour != 21 {
		t.Fatalf("Changed got %+v", changed)
	}
	if st := statusOf(s, boss.Name); !st.Next.Equal(at(21, 0)) || st.Def.Duration != 30*time.Minute {
		t.Fatalf("status after the change: %+v", st)
	}
	s.Check(context.Background(), at(20, 0))
	if got := run.take(); got != "" {
		t.Fatalf("the old time ran: %q", got)
	}
	s.Check(context.Background(), at(21, 0))
	if got := run.take(); got != "start example.bosses:Night 21:00-21:30" {
		t.Fatalf("the new time: %q", got)
	}
	// A new duration leaves the run that lasts as it is.
	d.Duration = 2 * time.Hour
	if err := apply(t, s, Change{Event: boss.Name, Set: &Setting{Def: d, On: true}}, at(21, 10)); err != nil {
		t.Fatal(err)
	}
	if st := statusOf(s, boss.Name); st.Active == nil || !st.Active.Ends.Equal(at(21, 30)) {
		t.Fatalf("the run that lasts: %+v", st.Active)
	}
	s.Check(context.Background(), at(21, 30))
	if got := run.take(); got != "end example.bosses:Night" {
		t.Fatalf("at the run's end: %q", got)
	}
}

// TestTurnOffAndManualRuns: an event that is off starts no runs by its
// trigger. Staff may start and stop it.
func TestTurnOffAndManualRuns(t *testing.T) {
	run := &fakeRunner{}
	s := New([]Def{boss}, Options{Runner: run})
	if err := s.Load(context.Background(), at(19, 0)); err != nil {
		t.Fatal(err)
	}
	if err := apply(t, s, Change{Event: boss.Name, Set: &Setting{Def: boss, On: false}}, at(19, 0)); err != nil {
		t.Fatal(err)
	}
	if st := statusOf(s, boss.Name); st.On || !st.Next.IsZero() {
		t.Fatalf("status of an event that is off: %+v", st)
	}
	if len(s.Defs()) != 0 {
		t.Fatalf("Defs lists an event that is off: %v", s.Defs())
	}
	s.Check(context.Background(), at(20, 0))
	if got := run.take(); got != "" {
		t.Fatalf("an event that is off started: %q", got)
	}
	if err := apply(t, s, Change{Event: boss.Name, Start: true}, at(20, 5)); err != nil {
		t.Fatal(err)
	}
	if got := run.take(); got != "start example.bosses:Night 20:05-21:05" {
		t.Fatalf("staff start: %q", got)
	}
	if err := apply(t, s, Change{Event: boss.Name, Start: true}, at(20, 6)); !errors.Is(err, ErrRunning) {
		t.Fatalf("a second start: %v", err)
	}
	if err := apply(t, s, Change{Event: boss.Name, Stop: true}, at(20, 7)); err != nil {
		t.Fatal(err)
	}
	if got := run.take(); got != "end example.bosses:Night" {
		t.Fatalf("staff stop: %q", got)
	}
	if err := apply(t, s, Change{Event: boss.Name, Stop: true}, at(20, 8)); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("a second stop: %v", err)
	}
	if err := apply(t, s, Change{Event: "example.none:Nothing", Start: true}, at(20, 9)); !errors.Is(err, ErrNoEvent) {
		t.Fatalf("an event that does not exist: %v", err)
	}
	// Back on, it follows its trigger from now.
	if err := apply(t, s, Change{Event: boss.Name, Set: &Setting{Def: boss, On: true}}, at(20, 10)); err != nil {
		t.Fatal(err)
	}
	if st := statusOf(s, boss.Name); !st.Next.Equal(time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("next after turning it on: %v", st.Next)
	}
}

// TestOffAtLoad: an event that is off at the start does not start late,
// though its window is open.
func TestOffAtLoad(t *testing.T) {
	run := &fakeRunner{}
	s := NewFrom([]Setting{{Def: boss, On: false}}, Options{Runner: run})
	if err := s.Load(context.Background(), at(20, 30)); err != nil {
		t.Fatal(err)
	}
	s.Check(context.Background(), at(20, 30))
	if got := run.take(); got != "" {
		t.Fatalf("an event that is off started late: %q", got)
	}
	if len(s.Next()) != 0 {
		t.Fatalf("Next lists an event that is off: %v", s.Next())
	}
}

// TestStartFailure: a run that does not start is kept as the event's
// last failure.
func TestStartFailure(t *testing.T) {
	s := New([]Def{boss}, Options{Runner: &failingRunner{}})
	if err := s.Load(context.Background(), at(19, 0)); err != nil {
		t.Fatal(err)
	}
	if err := apply(t, s, Change{Event: boss.Name, Start: true}, at(19, 1)); err == nil {
		t.Fatal("a failed start returned no error")
	}
	if st := statusOf(s, boss.Name); st.Failed != "no map runs" || !st.FailedAt.Equal(at(19, 1)) || st.Active != nil {
		t.Fatalf("status %+v", st)
	}
}

type failingRunner struct{ fakeRunner }

func (*failingRunner) Start(context.Context, Def, Run) error { return errors.New("no map runs") }

// TestTriggerValue: Value gives the config's form, which ParseTrigger and
// ParseJSONTrigger read back.
func TestTriggerValue(t *testing.T) {
	for _, tr := range []Trigger{
		{Kind: Clock, Spec: schedule.Spec{Kind: schedule.Daily, Hour: 20, Minute: 5}},
		{Kind: Clock, Spec: schedule.Spec{Kind: schedule.Weekly, Weekday: time.Saturday, Hour: 8}},
		{Kind: Clock, Spec: schedule.Spec{Kind: schedule.Monthly, Day: 31, Hour: 23, Minute: 59}},
		{Kind: Clock, Spec: schedule.Spec{Kind: schedule.Hourly, Minute: 30}},
		{Kind: Random, Chance: 250},
		{Kind: Condition, Module: "example.bosses.conditions", Function: "town_is_quiet", Lasting: 10 * time.Minute},
	} {
		v := tr.Value()
		got, err := ParseTrigger(v)
		if err != nil || !reflect.DeepEqual(got, tr) {
			t.Errorf("%v: read back %+v, %v", v, got, err)
		}
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var j any
		if err := json.Unmarshal(b, &j); err != nil {
			t.Fatal(err)
		}
		if got, err := ParseJSONTrigger(j); err != nil || !reflect.DeepEqual(got, tr) {
			t.Errorf("%s: from JSON %+v, %v", b, got, err)
		}
	}
	if _, err := ParseJSONTrigger(map[string]any{"hourly": map[string]any{"minute": 1.5}}); err == nil {
		t.Error("a minute of 1.5 was read")
	}
}

// TestSettingsKeepOff: Settings keeps the events that are off, which
// Apply leaves out.
func TestSettingsKeepOff(t *testing.T) {
	core := Def{Name: "core:Field_Boss", Mod: mod.Core, Duration: time.Hour, Trigger: boss.Trigger}
	other := Def{Name: "example.a:Off", Mod: "example.a", Duration: time.Hour, Trigger: boss.Trigger}
	got, err := Settings([]Def{boss, core, other}, map[string]Override{"example.a:Off": {Disabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].On || got[1].On || got[2].On {
		t.Fatalf("settings %+v", got)
	}
	defs, err := Apply([]Def{boss, core, other}, map[string]Override{"example.a:Off": {Disabled: true}})
	if err != nil || len(defs) != 1 || defs[0].Name != boss.Name {
		t.Fatalf("apply %v %v", defs, err)
	}
}
