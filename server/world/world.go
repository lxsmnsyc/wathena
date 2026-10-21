// Package world runs several maps in one process: one zone goroutine per
// map (strategy section 4.1, item 1). Zones share no state. A player who
// warps to another map is handed to that map's zone as a message, and
// the world only finds zones by map name.
package world

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// World is a set of running zones. Add and Remove change its zones
// while it runs, when a rotating dungeon rotates or a region changes
// (decision 0045). Its methods are safe for concurrent use.
type World struct {
	mu      sync.RWMutex
	zones   map[string]*zone.Zone
	start   *zone.Zone
	stopped chan struct{}
	// ctx and wg run the zones once Run started; running is set then.
	// stops end each running zone alone, by zone. retired are the zones
	// that Remove took out, which Run still waits for.
	ctx     context.Context
	wg      sync.WaitGroup
	running bool
	stops   map[*zone.Zone]context.CancelFunc
	retired []*zone.Zone
	// npcs lists every zone's NPCs, which scripts reach across zones
	// (review decision 58), and players every online player (review
	// decision 59).
	npcs    *zone.NPCDirectory
	players *zone.PlayerDirectory
}

// New makes a world of zones. start is the asset name of the map where
// new characters start, and where characters whose map does not run
// enter. Each zone's World is set to the new world, so its warps reach
// the others.
func New(start string, zones ...*zone.Zone) (*World, error) {
	w := &World{zones: map[string]*zone.Zone{}, stopped: make(chan struct{}), stops: map[*zone.Zone]context.CancelFunc{},
		npcs: zone.NewNPCDirectory(), players: zone.NewPlayerDirectory()}
	for _, z := range zones {
		if w.zones[z.Map] != nil {
			return nil, fmt.Errorf("world: map %s is listed twice", z.Map)
		}
		w.zones[z.Map] = z
		z.World = w
	}
	// The start map's NPCs come first, so a name two zones have finds
	// the start map's, then the others by map name.
	if s := w.zones[start]; s != nil {
		w.npcs.AddZone(s)
	}
	for _, z := range w.sorted() {
		if z.Map != start {
			w.npcs.AddZone(z)
		}
	}
	w.start = w.zones[start]
	if w.start == nil {
		return nil, fmt.Errorf("world: the start map %s does not run", start)
	}
	// Zones without a chat hub or social registry share the start zone's.
	for _, z := range zones {
		z.ShareHubs(w.start)
	}
	return w, nil
}

// NPCs is the world's directory of NPCs.
func (w *World) NPCs() *zone.NPCDirectory { return w.npcs }

// Players is the world's directory of online players.
func (w *World) Players() *zone.PlayerDirectory { return w.players }

// sorted is the zones by map name. The caller holds w.mu or runs before
// the world does.
func (w *World) sorted() []*zone.Zone {
	out := slices.Collect(maps.Values(w.zones))
	slices.SortFunc(out, func(a, b *zone.Zone) int { return strings.Compare(a.Map, b.Map) })
	return out
}

// Zone returns the zone that runs a map, by asset name, or nil.
func (w *World) Zone(name string) *zone.Zone {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.zones[name]
}

// Start is the zone where new characters start.
func (w *World) Start() *zone.Zone { return w.start }

// Zones lists the zones, by map name.
func (w *World) Zones() []*zone.Zone {
	w.mu.RLock()
	out := make([]*zone.Zone, 0, len(w.zones))
	for _, z := range w.zones {
		out = append(out, z)
	}
	w.mu.RUnlock()
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
	w.mu.Lock()
	w.ctx, w.running = ctx, true
	for _, z := range w.zones {
		w.goZone(z)
	}
	w.mu.Unlock()
	<-ctx.Done()
	w.wg.Wait()
	// No zone runs now, so no player can start a new move.
	w.mu.RLock()
	all := slices.Concat(slices.Collect(maps.Values(w.zones)), w.retired)
	w.mu.RUnlock()
	for _, z := range all {
		z.WaitIncoming()
	}
}

// goZone runs a zone on its own context. The caller holds w.mu, and Run
// has started.
func (w *World) goZone(z *zone.Zone) {
	zctx, stop := context.WithCancel(w.ctx)
	w.stops[z] = stop
	w.wg.Go(func() {
		defer stop()
		z.Run(zctx)
	})
}

// Add adds a zone that runs a new map, or a new zone for a map whose
// zone Remove took out, and runs it when the world runs. It shares the
// start zone's hubs.
func (w *World) Add(z *zone.Zone) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.zones[z.Map] != nil {
		return fmt.Errorf("world: map %s runs already", z.Map)
	}
	z.World = w
	z.ShareHubs(w.start)
	w.zones[z.Map] = z
	w.npcs.AddZone(z)
	if w.running {
		w.goZone(z)
	}
	return nil
}

// Swap puts a new zone in place of a map's zone in one step, so a warp
// to the map finds one or the other. It returns the old zone, which goes
// on running until Retire, or nil when the map did not run.
func (w *World) Swap(z *zone.Zone) *zone.Zone {
	w.mu.Lock()
	defer w.mu.Unlock()
	old := w.zones[z.Map]
	z.World = w
	z.ShareHubs(w.start)
	w.zones[z.Map] = z
	if old != nil {
		w.npcs.RemoveZone(old)
	}
	w.npcs.AddZone(z)
	if w.running {
		w.goZone(z)
	}
	if old != nil {
		w.retired = append(w.retired, old)
	}
	return old
}

// Remove takes a map's zone out of the world: warps and logins no longer
// find it. It goes on running until Retire. It returns the zone, or nil
// when the map did not run. The start map cannot be removed.
func (w *World) Remove(name string) *zone.Zone {
	w.mu.Lock()
	defer w.mu.Unlock()
	z := w.zones[name]
	if z == nil || z == w.start {
		return nil
	}
	delete(w.zones, name)
	w.npcs.RemoveZone(z)
	w.retired = append(w.retired, z)
	return z
}

// Retire stops a zone that Remove or Swap took out. It saves and drops
// any player still there, as a stop does.
func (w *World) Retire(z *zone.Zone) {
	w.mu.Lock()
	stop := w.stops[z]
	delete(w.stops, z)
	w.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// ReloadMod loads a mod's scripts again in every zone, from a new
// snapshot, for development. Each zone ends the mod's old context, which
// removes its handlers and timers, and runs the new scripts. It returns
// the errors of every zone, by map.
func (w *World) ReloadMod(ctx context.Context, sc *mod.Scripts) error {
	var errs []error
	for _, z := range w.Zones() {
		if err := z.ReloadMod(ctx, sc); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", z.Map, err))
		}
	}
	return errors.Join(errs...)
}

// Announce shows a line to every player of every zone, as rAthena's
// announce. It never waits on a zone.
func (w *World) Announce(text string, color uint32) {
	w.AnnounceFrom(text, color, "")
}

// AnnounceFrom is Announce for a mod's script: players see mod, the
// mod's ID, with the line (review decision 52).
func (w *World) AnnounceFrom(text string, color uint32, mod string) {
	for _, z := range w.Zones() {
		z.AnnounceFrom(text, color, mod)
	}
}

// Stopped closes when Run has returned.
func (w *World) Stopped() <-chan struct{} { return w.stopped }

// JoinCharacter adds a loaded character to the zone of its map. A
// character whose map does not run enters the start zone at its start
// cell, and one whose map was removed enters the map's town at its save
// spot (decision 0045).
func (w *World) JoinCharacter(ctx context.Context, c *character.Character, saver zone.Saver) (*zone.Session, error) {
	z := w.Zone(c.Map)
	if z == nil {
		if to, ok := w.start.Moved.Lookup(c.Map); ok {
			if tz := w.Zone(to.Map); tz != nil {
				c.Map, c.X, c.Y = to.Map, to.X, to.Y
				z = tz
			}
		}
	}
	if z == nil {
		z = w.start
	}
	return z.JoinCharacter(ctx, c, saver)
}

// NewCharacter is a new character on the start map.
func (w *World) NewCharacter(name string, look zone.Look) character.Character {
	return w.start.NewCharacter(name, look)
}

// EndAutotrade ends the autotrade of a character, if it has one, and
// waits until it left, so a new login of the character loads its last
// save. Every zone shares one list of autotraders.
func (w *World) EndAutotrade(ctx context.Context, id int64) {
	if w.start != nil && w.start.Autotraders != nil {
		w.start.Autotraders.End(ctx, id)
	}
}
