package modscript

import (
	"errors"
	"fmt"
	"time"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Typed world events in scripts (proposal 0002, review decision 31):
//   - context:get_event() returns the run of a typed event's script: a
//     handle with get_monsters, get_participants and finish.
//   - world_event_start and world_event_end are world events: every
//     zone's handlers run when a run starts or ends.
//   - world_events.start(name) and world_events.stop(name) ask the
//     scheduler to start or stop an event, with events.control.

// EventHost runs the calls of typed runs and world_events. A Host
// without it raises an error on them.
type EventHost interface {
	// EventMonsters returns the handles of a run's monsters that live on
	// the zone's map.
	EventMonsters(run int64) []int64
	// EventParticipants returns the IDs of the players on the zone's map
	// who did at least the run's min share of the damage to its
	// monsters.
	EventParticipants(run int64) []int64
	// FinishEvent ends a run with a result: "won" or "lost".
	FinishEvent(run int64, result string) error
	// RequestWorldEvent asks to start or stop an event by name, as mod.
	// It reports false when there is no such event, or it already runs
	// (start), or does not run (stop).
	RequestWorldEvent(name, mod string, start bool) (bool, error)
}

// WorldEvent is the object of world_event_start and world_event_end:
// { name, type, result }.
type WorldEvent struct {
	Name, Type, Result string
}

// FireWorldEvent runs the handlers of world_event_start or
// world_event_end.
func (r *Runtime) FireWorldEvent(name string, ev WorldEvent) {
	fields := []script.Field{script.KV("name", script.String(ev.Name)), script.KV("type", script.String(ev.Type))}
	if ev.Result != "" {
		fields = append(fields, script.KV("result", script.String(ev.Result)))
	}
	r.Fire(name, Arg{"world_event", script.NewTable(fields...)})
}

// StartEventRun starts one run of a world event, as StartEvent, for a
// typed event: context:get_event() returns the run.
func (r *Runtime) StartEventRun(name, entry string, run int64) (*Context, error) {
	setup, mod, err := r.setup(entry)
	if err != nil {
		return nil, err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindEvent, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}, run: run}
	c.Instance = script.NewTable(script.KV("name", script.String(name)), script.KV("map", script.String(r.host.Map())))
	r.events[c.ID] = c
	if err := r.runSetup(c, setup); err != nil {
		r.EndContext(c)
		return nil, fmt.Errorf("%s: setup: %w", entry, err)
	}
	return c, nil
}

func (r *Runtime) eventHost(what string) (EventHost, error) {
	h, ok := r.host.(EventHost)
	if !ok {
		return nil, errors.New(what + ": world events do not run here")
	}
	return h, nil
}

// contextEvent is context:get_event(): the run's ID, or nil outside a
// typed event's run.
func (r *Runtime) contextEvent(c *script.Call) error {
	ctx, err := r.needContext("context:get_event")
	if err != nil {
		return err
	}
	if ctx.run != 0 {
		c.Return(script.Int(ctx.run))
	}
	return nil
}

// eventRunCall runs a method of a run handle: Arg(0) is the method,
// Arg(1) the run.
func (r *Runtime) eventRunCall(c *script.Call) error {
	op, _ := c.Arg(0).AsString()
	run, ok := c.Arg(1).AsInt()
	if !ok || run <= 0 {
		return errors.New("not a run: call the methods of context:get_event()")
	}
	what := "run:" + op
	h, err := r.eventHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "get_monsters", "get_participants":
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		var ids []int64
		if op == "get_monsters" {
			ids = h.EventMonsters(run)
		} else {
			ids = h.EventParticipants(run)
		}
		list := make([]script.Value, len(ids))
		for i, id := range ids {
			list[i] = script.Int(id)
		}
		c.Return(script.List(list...))
		return nil
	case "finish":
		if err := r.mutating(what); err != nil {
			return err
		}
		result, _ := c.Arg(2).AsString()
		if result != "won" && result != "lost" {
			return errors.New(`run:finish: the result is "won" or "lost"`)
		}
		return h.FinishEvent(run, result)
	}
	return fmt.Errorf("run: there is no method %q", op)
}

// worldEventsRequest is world_events.start(name) and
// world_events.stop(name): Arg(1) is true for start.
func (r *Runtime) worldEventsRequest(c *script.Call) error {
	start := c.Arg(1).Truthy()
	what := "world_events.stop"
	if start {
		what = "world_events.start"
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "events.control"); err != nil {
		return err
	}
	name, ok := c.Arg(0).AsString()
	if !ok || name == "" || len(name) > 160 {
		return errors.New(what + ": want an event's name of data/events.yml, such as example.bosses:Night")
	}
	h, err := r.eventHost(what)
	if err != nil {
		return err
	}
	done, err := h.RequestWorldEvent(name, r.actor(), start)
	if err != nil {
		return err
	}
	c.Return(script.Bool(done))
	return nil
}

// RestartPlanned is the object of restart_planned (review decision 36):
// { at, reason } when staff plan a restart, and { cancelled = true }
// when they cancel it. at is the restart's time in Unix seconds.
type RestartPlanned struct {
	At        time.Time
	Reason    string
	Cancelled bool
}

// FireRestartPlanned runs the handlers of restart_planned.
func (r *Runtime) FireRestartPlanned(ev RestartPlanned) {
	var fields []script.Field
	if ev.Cancelled {
		fields = []script.Field{script.KV("cancelled", script.Bool(true))}
	} else {
		fields = []script.Field{script.KV("at", script.Int(ev.At.Unix())), script.KV("reason", script.String(ev.Reason))}
	}
	r.Fire("restart_planned", Arg{"restart", script.NewTable(fields...)})
}
