package worldevent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/schedule"
)

type fakeRunner struct {
	log  []string
	held bool
}

func (f *fakeRunner) Start(_ context.Context, d Def, r Run) error {
	f.log = append(f.log, "start "+d.Name+" "+r.Scheduled.Format("15:04")+"-"+r.Ends.Format("15:04"))
	return nil
}

func (f *fakeRunner) End(_ context.Context, d Def, _ Run) error {
	f.log = append(f.log, "end "+d.Name)
	return nil
}

func (f *fakeRunner) Condition(context.Context, Def) (bool, error) { return f.held, nil }

func (f *fakeRunner) take() string {
	s := strings.Join(f.log, "|")
	f.log = nil
	return s
}

type fixedRand int

func (r fixedRand) IntN(int) int { return int(r) }

var boss = Def{Name: "example.bosses:Night", Mod: "example.bosses", Duration: time.Hour,
	Trigger: Trigger{Kind: Clock, Spec: schedule.Spec{Kind: schedule.Daily, Hour: 20}}}

func at(hh, mm int) time.Time { return time.Date(2026, 10, 31, hh, mm, 0, 0, time.UTC) }

func newScheduler(t *testing.T, defs []Def, store Store, run *fakeRunner, now time.Time, rand Rand) *Scheduler {
	t.Helper()
	s := New(defs, Options{Runner: run, Store: store, Rand: rand})
	if err := s.Load(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestClockRunAndRestart(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	run := &fakeRunner{}
	s := newScheduler(t, []Def{boss}, store, run, at(19, 58), nil)
	s.Check(ctx, at(19, 59))
	if got := run.take(); got != "" {
		t.Fatalf("before the time: %q", got)
	}
	s.Check(ctx, at(20, 0))
	if got := run.take(); got != "start example.bosses:Night 20:00-21:00" {
		t.Fatalf("at the time: %q", got)
	}
	s.Check(ctx, at(20, 30))
	if got := run.take(); got != "" {
		t.Fatalf("during the run: %q", got)
	}
	// A restart during the run does not run it again.
	s2 := newScheduler(t, []Def{boss}, store, run, at(20, 30), nil)
	s2.Check(ctx, at(20, 30))
	if got := run.take(); got != "" {
		t.Fatalf("after a restart during the run: %q", got)
	}
	s.Check(ctx, at(21, 0))
	if got := run.take(); got != "end example.bosses:Night" {
		t.Fatalf("at the end: %q", got)
	}
}

func TestOpenWindowAndMissedRuns(t *testing.T) {
	ctx := context.Background()
	run := &fakeRunner{}
	// The server starts at 20:30 and has never run the event: its
	// window is open, so it starts late and ends at 21:00.
	s := newScheduler(t, []Def{boss}, &MemoryStore{}, run, at(20, 30), nil)
	s.Check(ctx, at(20, 30))
	if got := run.take(); got != "start example.bosses:Night 20:00-21:00" {
		t.Fatalf("open window: %q", got)
	}
	// Started at 21:05, the window has closed: the run is skipped.
	s = newScheduler(t, []Def{boss}, &MemoryStore{}, run, at(21, 5), nil)
	s.Check(ctx, at(21, 5))
	if got := run.take(); got != "" {
		t.Fatalf("closed window: %q", got)
	}
	if next := s.Next()[boss.Name]; !next.Equal(time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("next run at %v", next)
	}
}

func TestTimeZone(t *testing.T) {
	manila, err := time.LoadLocation("Asia/Manila")
	if err != nil {
		t.Fatal(err)
	}
	run := &fakeRunner{}
	s := New([]Def{boss}, Options{Runner: run, Location: manila})
	if err := s.Load(context.Background(), at(11, 0)); err != nil {
		t.Fatal(err)
	}
	// 20:00 in Manila is 12:00 UTC.
	s.Check(context.Background(), at(12, 0))
	if got := run.take(); !strings.HasPrefix(got, "start example.bosses:Night 12:00-13:00") {
		t.Fatalf("got %q", got)
	}
}

func TestRandomAndCondition(t *testing.T) {
	ctx := context.Background()
	run := &fakeRunner{}
	lucky := Def{Name: "m:Lucky", Duration: 10 * time.Minute, Trigger: Trigger{Kind: Random, Chance: 5000}}
	s := newScheduler(t, []Def{lucky}, &MemoryStore{}, run, at(9, 30), fixedRand(4999))
	s.Check(ctx, at(9, 59))
	s.Check(ctx, at(10, 0))
	if got := run.take(); got != "start m:Lucky 10:00-10:10" {
		t.Fatalf("random: %q", got)
	}
	s = newScheduler(t, []Def{lucky}, &MemoryStore{}, run, at(9, 30), fixedRand(5000))
	s.Check(ctx, at(10, 0))
	if got := run.take(); got != "" {
		t.Fatalf("random without luck: %q", got)
	}

	quiet := Def{Name: "m:Quiet", Duration: 5 * time.Minute, Trigger: Trigger{Kind: Condition, Lasting: 2 * time.Minute}}
	s = newScheduler(t, []Def{quiet}, &MemoryStore{}, run, at(9, 0), nil)
	run.held = true
	for m := 0; m < 2; m++ {
		s.Check(ctx, at(9, m))
	}
	if got := run.take(); got != "" {
		t.Fatalf("condition before it lasted: %q", got)
	}
	s.Check(ctx, at(9, 2))
	if got := run.take(); got != "start m:Quiet 09:02-09:07" {
		t.Fatalf("condition: %q", got)
	}
	s.Check(ctx, at(9, 7))
	if got := run.take(); got != "end m:Quiet" {
		t.Fatalf("condition end: %q", got)
	}
}

func TestParse(t *testing.T) {
	r := &mod.Record{Name: "example.halloween:Boss", Mod: "example.halloween", Fields: map[string]any{
		"trigger":          map[string]any{"weekly": map[string]any{"day": "saturday", "time": "20:00"}},
		"duration_minutes": 60,
		"script":           "scripts/events/boss.lua",
		"map":              "prt_fild08",
	}, SetBy: map[string]string{"script": "example.halloween"}}
	d, err := Parse(r)
	if err != nil {
		t.Fatal(err)
	}
	if d.Trigger.Spec.Weekday != time.Saturday || d.Duration != time.Hour || d.Script != "example.halloween:scripts/events/boss.lua" || d.Map != "map/prt_fild08" {
		t.Fatalf("%+v", d)
	}
	for _, bad := range []any{
		map[string]any{"daily": "25:00"},
		map[string]any{"weekly": map[string]any{"day": "caturday", "time": "20:00"}},
		map[string]any{"random": map[string]any{"chance_per_hour": 0}},
		map[string]any{"condition": map[string]any{"module": "m"}},
		map[string]any{"daily": "20:00", "hourly": map[string]any{"minute": 1}},
		"20:00",
	} {
		if _, err := ParseTrigger(bad); err == nil {
			t.Errorf("%v parsed", bad)
		}
	}
	defs, err := Apply([]Def{d}, map[string]Override{d.Name: {Trigger: map[string]any{"daily": "15:42"}, DurationMinutes: 3}})
	if err != nil || len(defs) != 1 || defs[0].Trigger.Spec.Kind != schedule.Daily || defs[0].Trigger.Spec.Minute != 42 || defs[0].Duration != 3*time.Minute {
		t.Fatalf("override: %+v %v", defs, err)
	}
	if defs, err := Apply([]Def{d}, map[string]Override{d.Name: {Disabled: true}}); err != nil || len(defs) != 0 {
		t.Fatalf("disabled: %+v %v", defs, err)
	}
	if _, err := Apply([]Def{d}, map[string]Override{"x:Missing": {Disabled: true}}); err == nil {
		t.Fatal("an override of a missing event passed")
	}
}
