package worldevent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/lxsmnsyc/wathena/server/schedule"
)

// Staff control of the scheduler (proposal 0002 review decision 36,
// decision 0031):
//   - A Change sets an event's definition and whether it is on, or
//     starts or stops a run. Changes wait in a queue, and Check applies
//     them first, on the scheduler's goroutine. Do waits for that.
//   - A new trigger plans the event's next time from now. A run that
//     lasts keeps its end. A new duration applies to the next run.
//   - An event that is off starts no runs by its trigger. A run that
//     lasts goes on until its end. Staff may still start an event that
//     is off.
//   - Status is a copy of each event's state, made after each Check, so
//     any goroutine may read it.

// Change is a staff change of one event.
type Change struct {
	Event string
	// Set replaces the event's definition and whether it is on. Its
	// name must be Event.
	Set *Setting
	// Start starts a run now, and Stop ends the run that lasts.
	Start, Stop bool
}

// Errors of a Change.
var (
	ErrNoEvent    = errors.New("there is no such event")
	ErrRunning    = errors.New("the event has a run that lasts")
	ErrNotRunning = errors.New("the event has no run that lasts")
)

// Status is an event's state.
type Status struct {
	Def Def
	On  bool
	// Next is the next time of a clock trigger, or the next hour's draw
	// of a random trigger. It is zero for a condition trigger and for an
	// event that is off.
	Next time.Time
	// Active is the run that lasts, or nil.
	Active *Run
	// Failed is the error of the last run that did not start, and
	// FailedAt when.
	Failed   string
	FailedAt time.Time
}

type queued struct {
	c    Change
	done chan error
}

// Queue queues a change. Check applies it, and sends its result on the
// channel.
func (s *Scheduler) Queue(c Change) <-chan error {
	done := make(chan error, 1)
	s.mu.Lock()
	s.queue = append(s.queue, queued{c: c, done: done})
	s.mu.Unlock()
	return done
}

// Do queues a change and waits until Check applied it.
func (s *Scheduler) Do(ctx context.Context, c Change) error {
	done := s.Queue(c)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status returns each event's state after the last Check, by name.
func (s *Scheduler) Status() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Status(nil), s.status...)
}

// applyQueue applies the queued changes at now.
func (s *Scheduler) applyQueue(ctx context.Context, now time.Time) {
	s.mu.Lock()
	list := s.queue
	s.queue = nil
	s.mu.Unlock()
	if len(list) == 0 {
		return
	}
	changed := false
	errs := make([]error, len(list))
	for i, q := range list {
		errs[i] = s.apply(ctx, q.c, now)
		if errs[i] == nil && q.c.Set != nil {
			changed = true
		}
	}
	if changed && s.opts.Changed != nil {
		s.opts.Changed(s.onDefs())
	}
	// Status shows the changes before their callers hear back.
	s.publish()
	for i, q := range list {
		q.done <- errs[i]
	}
}

// onDefs are the events that are on.
func (s *Scheduler) onDefs() []Def {
	var out []Def
	for _, d := range s.defs {
		if s.on[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

func (s *Scheduler) apply(ctx context.Context, c Change, now time.Time) error {
	i := -1
	for j, d := range s.defs {
		if d.Name == c.Event {
			i = j
		}
	}
	st := s.state[c.Event]
	if i < 0 || st == nil {
		return ErrNoEvent
	}
	d := s.defs[i]
	switch {
	case c.Set != nil:
		if c.Set.Def.Name != c.Event {
			return fmt.Errorf("the change names %s, not %s", c.Set.Def.Name, c.Event)
		}
		s.defs[i] = c.Set.Def
		s.on[c.Event] = c.Set.On
		st.late, st.resume, st.held, st.polled = nil, nil, time.Time{}, time.Time{}
		st.next = s.firstTime(c.Set.Def, now)
		s.opts.Log.Info("world event changed by staff", "event", c.Event, "on", c.Set.On, "trigger", c.Set.Def.Trigger.Describe(),
			"minutes", int(c.Set.Def.Duration.Minutes()))
	case c.Start:
		if st.active != nil {
			return ErrRunning
		}
		at := now.Truncate(time.Second)
		return s.start(ctx, d, st, now, at, at.Add(d.Duration))
	case c.Stop:
		if st.active == nil {
			return ErrNotRunning
		}
		r := *st.active
		st.active = nil
		var err error
		if ctrl, ok := s.opts.Runner.(Controller); ok {
			err = ctrl.Stop(ctx, d, r)
		} else {
			err = s.opts.Runner.End(ctx, d, r)
		}
		if err != nil {
			s.opts.Log.Error("world event failed to stop", "event", d.Name, "err", err)
			return err
		}
		s.opts.Log.Info("world event stopped by staff", "event", d.Name)
	}
	return nil
}

// firstTime is the next time of an event's trigger after now.
func (s *Scheduler) firstTime(d Def, now time.Time) time.Time {
	switch d.Trigger.Kind {
	case Clock:
		return schedule.Next(d.Trigger.Spec, now, s.opts.Location)
	case Random:
		return now.Truncate(time.Hour).Add(time.Hour)
	}
	return time.Time{}
}

// publish copies each event's state for Status.
func (s *Scheduler) publish() {
	out := make([]Status, 0, len(s.defs))
	for _, d := range s.defs {
		st := s.state[d.Name]
		if st == nil {
			continue
		}
		e := Status{Def: d, On: s.on[d.Name], Failed: st.failed, FailedAt: st.failedAt}
		if e.On && d.Trigger.Kind != Condition {
			e.Next = st.next
		}
		if st.active != nil {
			r := *st.active
			e.Active = &r
		}
		out = append(out, e)
	}
	s.mu.Lock()
	s.status = out
	s.mu.Unlock()
}

// Value is the trigger in the form of data/events.yml and mods.events,
// such as {"daily": "20:00"}, for the admin screens. ParseTrigger reads
// it back.
func (t Trigger) Value() map[string]any {
	clock := fmt.Sprintf("%02d:%02d", t.Spec.Hour, t.Spec.Minute)
	switch t.Kind {
	case Random:
		return map[string]any{"random": map[string]any{"chance_per_hour": t.Chance}}
	case Condition:
		return map[string]any{"condition": map[string]any{"module": t.Module, "function": t.Function,
			"lasting_minutes": int(t.Lasting.Minutes())}}
	}
	switch t.Spec.Kind {
	case schedule.Weekly:
		return map[string]any{"weekly": map[string]any{"day": schedule.Weekdays[t.Spec.Weekday], "time": clock}}
	case schedule.Monthly:
		return map[string]any{"monthly": map[string]any{"day": t.Spec.Day, "time": clock}}
	case schedule.Hourly:
		return map[string]any{"hourly": map[string]any{"minute": t.Spec.Minute}}
	}
	return map[string]any{"daily": clock}
}

// ParseJSONTrigger reads a trigger that came as JSON, whose numbers are
// float64: whole numbers become ints, as YAML gives them.
func ParseJSONTrigger(v any) (Trigger, error) { return ParseTrigger(wholeNumbers(v)) }

func wholeNumbers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = wholeNumbers(e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = wholeNumbers(e)
		}
		return out
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
			return int(x)
		}
	}
	return v
}

// CheckMinutes checks a duration in minutes: 1 to MaxDuration.
func CheckMinutes(n int) error {
	_, err := minutes(n)
	return err
}
