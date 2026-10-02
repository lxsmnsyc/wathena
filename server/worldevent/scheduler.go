package worldevent

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/lxsmnsyc/wathena/server/schedule"
)

// Rules of the scheduler:
//   - Clock triggers match times in the server's time zone (package
//     schedule). The server stores every time in UTC.
//   - Each run is recorded in the Store before it starts, so a restart
//     never runs the same time twice.
//   - Runs whose whole window passed while the server was stopped are
//     skipped. A run whose window is still open when the server starts,
//     and that has not started before, starts at once and ends at its
//     usual time. A run cut short by a stop does not start again.
//   - An event has at most one run at a time. A time that comes while a
//     run lasts is skipped.
//   - A random trigger draws once at the start of each hour, from a
//     secret stream (strategy section 8.3).
//   - A condition runs once a minute, in the zone of the event's map.
//     The event starts once the condition has held for its lasting
//     minutes. After the run ends, it must hold that long again.

// Run is one run of an event.
type Run struct {
	// ID names the run in the zone, for this process.
	ID    int64
	Event string
	// Scheduled is the time the trigger gave. Started is when it
	// started, and Ends when it ends.
	Scheduled, Started, Ends time.Time
}

// Runner runs events in the zones.
type Runner interface {
	// Start starts a run in the zone of the event's map.
	Start(ctx context.Context, d Def, r Run) error
	// End ends a run.
	End(ctx context.Context, d Def, r Run) error
	// Condition runs a condition trigger's function in the zone of the
	// event's map.
	Condition(ctx context.Context, d Def) (bool, error)
}

// Store keeps the last run of each event, so runs survive a restart.
type Store interface {
	// EventRuns returns the last recorded run of each event.
	EventRuns(ctx context.Context) ([]Run, error)
	// RecordEventRun records a run before it starts.
	RecordEventRun(ctx context.Context, r Run) error
}

// Rand draws numbers in [0, n), from a secret stream.
type Rand interface {
	IntN(n int) int
}

// Options configure a Scheduler.
type Options struct {
	Runner Runner
	// Store keeps runs. Without one, runs live in memory.
	Store Store
	// Location is the server's time zone. It is UTC when nil.
	Location *time.Location
	// Rand draws random triggers. It must be a secret stream.
	Rand Rand
	Log  *slog.Logger
}

// Scheduler starts and ends the runs of world events. Check does the
// work at a time. Run calls it every second on the system clock. Only
// one goroutine calls Check.
type Scheduler struct {
	opts   Options
	defs   []Def
	state  map[string]*state
	nextID int64
}

type state struct {
	// last is the Scheduled time of the last recorded run.
	last time.Time
	// active is the run that lasts now, or nil.
	active *Run
	// next is the next time of a clock trigger, or the start of the next
	// hour for a random trigger.
	next time.Time
	// late is a run whose window was open at the start.
	late *Run
	// held is when a condition started to hold, and polled the minute
	// it last ran.
	held, polled time.Time
}

// New makes a scheduler for events. Call Load before Check.
func New(defs []Def, opts Options) *Scheduler {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	if opts.Location == nil {
		opts.Location = time.UTC
	}
	if opts.Store == nil {
		opts.Store = &MemoryStore{}
	}
	defs = slices.Clone(defs)
	slices.SortFunc(defs, func(a, b Def) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		}
		return 0
	})
	return &Scheduler{opts: opts, defs: defs, state: map[string]*state{}}
}

// Defs returns the events, sorted by name.
func (s *Scheduler) Defs() []Def { return slices.Clone(s.defs) }

// Load reads the recorded runs and plans each event from now.
func (s *Scheduler) Load(ctx context.Context, now time.Time) error {
	runs, err := s.opts.Store.EventRuns(ctx)
	if err != nil {
		return err
	}
	last := map[string]time.Time{}
	for _, r := range runs {
		last[r.Event] = r.Scheduled
	}
	for _, d := range s.defs {
		st := &state{last: last[d.Name]}
		s.state[d.Name] = st
		switch d.Trigger.Kind {
		case Clock:
			// The latest time at or before now whose window is still
			// open starts late, unless it ran.
			var latest time.Time
			for t := schedule.Next(d.Trigger.Spec, now.Add(-d.Duration), s.opts.Location); !t.After(now); t = schedule.Next(d.Trigger.Spec, t, s.opts.Location) {
				latest = t
			}
			if !latest.IsZero() && latest.After(st.last) && now.Before(latest.Add(d.Duration)) {
				st.late = &Run{Event: d.Name, Scheduled: latest, Ends: latest.Add(d.Duration)}
			}
			st.next = schedule.Next(d.Trigger.Spec, now, s.opts.Location)
		case Random:
			hour := now.Truncate(time.Hour)
			st.next = hour.Add(time.Hour)
			if now.Sub(hour) < time.Minute && hour.After(st.last) {
				st.next = hour
			}
		}
	}
	return nil
}

// Next returns the next time of each clock or random event, by name,
// for the log.
func (s *Scheduler) Next() map[string]time.Time {
	out := map[string]time.Time{}
	for _, d := range s.defs {
		if st := s.state[d.Name]; st != nil && !st.next.IsZero() {
			out[d.Name] = st.next
		}
	}
	return out
}

// Active returns the runs that last now.
func (s *Scheduler) Active() []Run {
	var out []Run
	for _, d := range s.defs {
		if st := s.state[d.Name]; st != nil && st.active != nil {
			out = append(out, *st.active)
		}
	}
	return out
}

// Check ends the runs that are over and starts the runs that are due at
// now.
func (s *Scheduler) Check(ctx context.Context, now time.Time) {
	minute := schedule.MinuteStart(now)
	for _, d := range s.defs {
		st := s.state[d.Name]
		if st == nil {
			continue
		}
		if st.active != nil && !now.Before(st.active.Ends) {
			s.end(ctx, d, st)
		}
		switch d.Trigger.Kind {
		case Clock:
			if st.late != nil {
				r := st.late
				st.late = nil
				if now.Before(r.Ends) {
					s.start(ctx, d, st, now, r.Scheduled, r.Ends)
				}
			}
			if st.next.After(now) {
				continue
			}
			at := st.next
			st.next = schedule.Next(d.Trigger.Spec, now, s.opts.Location)
			switch {
			case !now.Before(at.Add(d.Duration)):
				s.opts.Log.Warn("world event skipped: its time passed while the server was busy", "event", d.Name, "at", at)
			case st.active != nil:
				s.opts.Log.Warn("world event skipped: its last run still lasts", "event", d.Name, "at", at)
			default:
				s.start(ctx, d, st, now, at, at.Add(d.Duration))
			}
		case Random:
			if st.next.After(now) {
				continue
			}
			at := st.next
			st.next = now.Truncate(time.Hour).Add(time.Hour)
			if now.Sub(at) >= time.Minute || st.active != nil || !at.After(st.last) {
				continue
			}
			if s.opts.Rand == nil || s.opts.Rand.IntN(10000) >= d.Trigger.Chance {
				continue
			}
			s.start(ctx, d, st, now, at, now.Add(d.Duration))
		case Condition:
			if !st.polled.Before(minute) {
				continue
			}
			st.polled = minute
			if st.active != nil {
				continue
			}
			held, err := s.opts.Runner.Condition(ctx, d)
			if err != nil {
				s.opts.Log.Warn("world event condition failed", "event", d.Name, "mod", d.Mod, "err", err)
				st.held = time.Time{}
				continue
			}
			if !held {
				st.held = time.Time{}
				continue
			}
			if st.held.IsZero() {
				st.held = minute
			}
			if now.Sub(st.held) >= d.Trigger.Lasting {
				st.held = time.Time{}
				s.start(ctx, d, st, now, minute, now.Add(d.Duration))
			}
		}
	}
}

// start records a run, then starts it. A run that cannot be recorded
// does not start, so a restart cannot run it again.
func (s *Scheduler) start(ctx context.Context, d Def, st *state, now, scheduled, ends time.Time) {
	s.nextID++
	r := Run{ID: s.nextID, Event: d.Name, Scheduled: scheduled, Started: now, Ends: ends}
	if err := s.opts.Store.RecordEventRun(ctx, r); err != nil {
		s.opts.Log.Error("world event not started: its run could not be recorded", "event", d.Name, "err", err)
		return
	}
	st.last = scheduled
	if err := s.opts.Runner.Start(ctx, d, r); err != nil {
		s.opts.Log.Error("world event failed to start", "event", d.Name, "mod", d.Mod, "err", err)
		return
	}
	st.active = &r
	s.opts.Log.Info("world event started", "event", d.Name, "scheduled", scheduled, "ends", ends)
}

func (s *Scheduler) end(ctx context.Context, d Def, st *state) {
	r := *st.active
	st.active = nil
	if err := s.opts.Runner.End(ctx, d, r); err != nil {
		s.opts.Log.Error("world event failed to end", "event", d.Name, "err", err)
		return
	}
	s.opts.Log.Info("world event ended", "event", d.Name)
}

// Run calls Check every second on the system clock until ctx ends.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		s.Check(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// MemoryStore keeps runs in memory, for a server without a database and
// for tests.
type MemoryStore struct {
	mu   sync.Mutex
	runs map[string]Run
}

// EventRuns returns the last run of each event.
func (m *MemoryStore) EventRuns(context.Context) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Run, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, r)
	}
	return out, nil
}

// RecordEventRun records a run.
func (m *MemoryStore) RecordEventRun(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runs == nil {
		m.runs = map[string]Run{}
	}
	m.runs[r.Event] = r
	return nil
}
