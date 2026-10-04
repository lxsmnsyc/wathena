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
// one. Typed events (review decision 31) place their monsters and NPCs
// on the maps they pick. EventField is the worldevent.Field of a world.

// EventField is the worldevent.Field of a world.
type EventField struct{ w *World }

// Events returns the world's event field.
func (w *World) Events() EventField { return EventField{w} }

// eventWait is the longest the event manager waits on a zone.
const eventWait = 10 * time.Second

// EventZone returns the zone that runs an event's script or condition.
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

// CheckEvent checks at start that the maps an event names run: its
// map, an invasion's town, and a boss's or treasure hunt's maps. A hint
// NPC's map that does not run is skipped, with a warning when the run
// starts.
func (w *World) CheckEvent(d worldevent.Def) error {
	if _, err := w.EventZone(d); err != nil {
		return err
	}
	var maps []string
	switch {
	case d.Invasion != nil:
		maps = []string{d.Invasion.Town}
	case d.Boss != nil:
		maps = d.Boss.Maps
	case d.Treasure != nil:
		maps = d.Treasure.Maps
	}
	for _, m := range maps {
		if w.zones[m] == nil {
			return fmt.Errorf("the map %s does not run in this world", worldevent.MapID(m))
		}
	}
	return nil
}

func (f EventField) zone(mapName string) (*zone.Zone, error) {
	z := f.w.zones[mapName]
	if z == nil {
		return nil, fmt.Errorf("the map %s does not run in this world", worldevent.MapID(mapName))
	}
	return z, nil
}

func waitCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, eventWait)
}

// Has reports whether a map runs.
func (f EventField) Has(mapName string) bool { return f.w.zones[mapName] != nil }

// StartMap is the world's start map.
func (f EventField) StartMap() string { return f.w.start.Map }

// StartScript runs an event's script in the zone of a map.
func (f EventField) StartScript(ctx context.Context, mapName string, s worldevent.ScriptRun) error {
	z, err := f.zone(mapName)
	if err != nil {
		return err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.StartEvent(ctx, zone.EventRun{ID: s.ID, Name: s.Event, Entry: s.Entry, Typed: s.Type != worldevent.TypeScript})
}

// EndScript ends a run's script context.
func (f EventField) EndScript(ctx context.Context, mapName string, run int64) error {
	z, err := f.zone(mapName)
	if err != nil {
		return err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.EndEvent(ctx, run)
}

// Condition runs a condition trigger's function in the zone of a map.
func (f EventField) Condition(ctx context.Context, mapName, mod, module, fn string) (bool, error) {
	z, err := f.zone(mapName)
	if err != nil {
		return false, err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.EventCondition(ctx, mod, module, fn)
}

// Spawn places a typed run's monsters on a map.
func (f EventField) Spawn(ctx context.Context, s worldevent.Spawn) ([]worldevent.Placed, error) {
	z, err := f.zone(s.Map)
	if err != nil {
		return nil, err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.SpawnEvent(ctx, s)
}

// Alive counts a run's monsters that live on a map.
func (f EventField) Alive(ctx context.Context, mapName string, run int64) (int, error) {
	z, err := f.zone(mapName)
	if err != nil {
		return 0, err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.EventAlive(ctx, run)
}

// Hints places a treasure hunt's hint NPCs on a map.
func (f EventField) Hints(ctx context.Context, mapName string, run int64, hints []worldevent.Hint) error {
	z, err := f.zone(mapName)
	if err != nil {
		return err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.EventHints(ctx, run, hints)
}

// Reward gives a run's rewards on a map.
func (f EventField) Reward(ctx context.Context, r worldevent.RewardRun) error {
	z, err := f.zone(r.Map)
	if err != nil {
		return err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.EventReward(ctx, r)
}

// Clear removes a run's monsters and NPCs from a map.
func (f EventField) Clear(ctx context.Context, mapName string, run int64) error {
	z, err := f.zone(mapName)
	if err != nil {
		return err
	}
	ctx, cancel := waitCtx(ctx)
	defer cancel()
	return z.ClearEvent(ctx, run)
}

// Announce shows a line to every player of the world.
func (f EventField) Announce(text string) { f.w.Announce(text, 0) }

// Notice tells every zone that a run started or ended.
func (f EventField) Notice(n worldevent.Notice) {
	for _, z := range f.w.zones {
		z.WorldEventNotice(n)
	}
}
