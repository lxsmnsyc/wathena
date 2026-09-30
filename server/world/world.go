// Package world runs several maps in one process: one zone goroutine per
// map (strategy section 4.1, item 1). Zones share no state. A player who
// warps to another map is handed to that map's zone as a message, and
// the world only finds zones by map name.
package world

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// World is a set of running zones.
type World struct {
	zones   map[string]*zone.Zone
	start   *zone.Zone
	stopped chan struct{}
}

// New makes a world of zones. start is the asset name of the map where
// new characters start, and where characters whose map does not run
// enter. Each zone's World is set to the new world, so its warps reach
// the others.
func New(start string, zones ...*zone.Zone) (*World, error) {
	w := &World{zones: map[string]*zone.Zone{}, stopped: make(chan struct{})}
	for _, z := range zones {
		if w.zones[z.Map] != nil {
			return nil, fmt.Errorf("world: map %s is listed twice", z.Map)
		}
		w.zones[z.Map] = z
		z.World = w
	}
	w.start = w.zones[start]
	if w.start == nil {
		return nil, fmt.Errorf("world: the start map %s does not run", start)
	}
	return w, nil
}

// Zone returns the zone that runs a map, by asset name, or nil.
func (w *World) Zone(name string) *zone.Zone { return w.zones[name] }

// Start is the zone where new characters start.
func (w *World) Start() *zone.Zone { return w.start }

// Zones lists the zones, by map name.
func (w *World) Zones() []*zone.Zone {
	out := make([]*zone.Zone, 0, len(w.zones))
	for _, z := range w.zones {
		out = append(out, z)
	}
	slices.SortFunc(out, func(a, b *zone.Zone) int {
		switch {
		case a.Map < b.Map:
			return -1
		case a.Map > b.Map:
			return 1
		}
		return 0
	})
	return out
}

// Run runs every zone until ctx ends. It returns once every zone has
// stopped and every player on its way between zones got its last save.
func (w *World) Run(ctx context.Context) {
	defer close(w.stopped)
	var wg sync.WaitGroup
	for _, z := range w.zones {
		wg.Go(func() { z.Run(ctx) })
	}
	wg.Wait()
	// No zone runs now, so no player can start a new move.
	for _, z := range w.zones {
		z.WaitIncoming()
	}
}

// Stopped closes when Run has returned.
func (w *World) Stopped() <-chan struct{} { return w.stopped }

// JoinCharacter adds a loaded character to the zone of its map. A
// character whose map does not run enters the start zone at its start
// cell.
func (w *World) JoinCharacter(ctx context.Context, c *character.Character, saver zone.Saver) (*zone.Session, error) {
	z := w.zones[c.Map]
	if z == nil {
		z = w.start
	}
	return z.JoinCharacter(ctx, c, saver)
}

// NewCharacter is a new character on the start map.
func (w *World) NewCharacter(name string, look zone.Look) character.Character {
	return w.start.NewCharacter(name, look)
}
