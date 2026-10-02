package world

import (
	"context"
	"fmt"
	"time"

	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// The world runs world events in its zones (strategy section 8.8): the
// zone of an event's map, or the start map's zone for an event without
// one.

// EventRunner is the scheduler's worldevent.Runner for a world.
type EventRunner struct{ w *World }

// Events returns the world's event runner.
func (w *World) Events() EventRunner { return EventRunner{w} }

// eventWait is the longest the scheduler waits on a zone.
const eventWait = 10 * time.Second

// EventZone returns the zone that runs an event.
func (w *World) EventZone(d worldevent.Def) (*zone.Zone, error) {
	if d.Map == "" {
		return w.start, nil
	}
	z := w.zones[d.Map]
	if z == nil {
		return nil, fmt.Errorf("the map %s of %s does not run in this world", d.Map, d.Name)
	}
	return z, nil
}

// Start starts a run of an event in its zone.
func (e EventRunner) Start(ctx context.Context, d worldevent.Def, r worldevent.Run) error {
	z, err := e.w.EventZone(d)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, eventWait)
	defer cancel()
	return z.StartEvent(ctx, zone.EventRun{ID: r.ID, Name: d.Name, Entry: d.Script})
}

// End ends a run of an event.
func (e EventRunner) End(ctx context.Context, d worldevent.Def, r worldevent.Run) error {
	z, err := e.w.EventZone(d)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, eventWait)
	defer cancel()
	return z.EndEvent(ctx, r.ID)
}

// Condition runs an event's condition in its zone.
func (e EventRunner) Condition(ctx context.Context, d worldevent.Def) (bool, error) {
	z, err := e.w.EventZone(d)
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, eventWait)
	defer cancel()
	return z.EventCondition(ctx, d.Mod, d.Trigger.Module, d.Trigger.Function)
}
