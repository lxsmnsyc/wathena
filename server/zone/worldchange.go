package zone

import (
	"context"
	"slices"
	"sync"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Removing or replacing a map (strategy section 8.10, proposal 0002
// review decision 47, decision 0045). A rotating dungeon's old floors
// go, and a changed region's fields come back with new cells:
//
//   - Evacuate moves the players of a map that goes to a town's save
//     spot. A player who arrives later, such as one who took a warp a
//     moment before, goes on there too.
//   - Relocations hold where each removed map's save and memo points
//     go. Relocate moves the points of the zone's players, and a player
//     who joins later, from a login or a warp, gets them moved as it
//     joins. The store moves those of offline characters.
//   - ReplaceWarps changes a zone's warps while it runs, so the portals
//     to a map change in the same step.
//   - WorldChanged runs the dungeon_rotated and region_changed handlers.

// Relocation is where the points of a removed map go: a town's map and
// its save spot.
type Relocation struct {
	Map  string
	X, Y int
}

// Relocations are where the points of removed maps go, by map ID. Every
// zone of a world shares one. Its methods are safe for concurrent use.
type Relocations struct {
	mu sync.RWMutex
	to map[string]Relocation
}

// Add records where a map's points go.
func (r *Relocations) Add(mapID string, to Relocation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.to == nil {
		r.to = map[string]Relocation{}
	}
	r.to[mapID] = to
}

// Lookup returns where a map's points go. ok is false for a map that
// was not removed.
func (r *Relocations) Lookup(mapID string) (Relocation, bool) {
	if r == nil {
		return Relocation{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	to, ok := r.to[mapID]
	return to, ok
}

// Moves returns a character's points moved as the relocations say: its
// save point, and its memo points, the newest first with one per map, as
// /memo keeps them. changed is false when no point moved.
func (r *Relocations) Moves(info character.Info, memos []character.Memo) (out character.Info, kept []character.Memo, changed bool) {
	out = info
	if to, ok := r.Lookup(info.SaveMap); ok {
		out.SaveMap, out.SaveX, out.SaveY, changed = to.Map, to.X, to.Y, true
	}
	for _, m := range memos {
		if to, ok := r.Lookup(m.Map); ok {
			m, changed = character.Memo{Map: to.Map, X: to.X, Y: to.Y}, true
		}
		if !slices.ContainsFunc(kept, func(k character.Memo) bool { return k.Map == m.Map }) {
			kept = append(kept, m)
		}
	}
	if !changed {
		return info, memos, false
	}
	return out, kept, true
}

// relocate moves a player's points as relocations say.
func (z *Zone) relocate(p *Player, r *Relocations) {
	if r == nil {
		return
	}
	if info, memos, ok := r.Moves(p.record, p.memos); ok {
		p.record, p.memos = info, memos
	}
}

// Relocate moves the points of the zone's players as r says, on the
// zone's goroutine. Their next saves write them. A replaced map, whose
// ID runs again, moves its points this once; a removed map's go into
// the zone's Moved, which also moves them at each later join.
func (z *Zone) Relocate(ctx context.Context, r *Relocations) error {
	return z.do(ctx, func() error {
		for _, p := range z.players {
			z.relocate(p, r)
		}
		return nil
	})
}

// evacuation is where a removed map's players go.
type evacuation struct {
	to   string
	x, y int
}

// Evacuate moves every player of the zone to a cell of another map,
// such as a town's save spot, as a warp does, on the zone's goroutine.
// From now on, a player who arrives goes on there too. It returns how
// many players it moved.
func (z *Zone) Evacuate(ctx context.Context, to string, x, y int) (int, error) {
	moved := 0
	err := z.do(ctx, func() error {
		z.evac = &evacuation{to: to, x: x, y: y}
		target := z.evacTarget()
		if target == nil {
			return nil
		}
		for _, p := range z.players {
			if p.sess == nil || p.gone() {
				continue
			}
			z.closeTalk(p)
			if z.depart(p, target, x, y) {
				moved++
			}
		}
		return nil
	})
	return moved, err
}

// evacTarget is the zone an evacuated zone's players go to, or nil.
func (z *Zone) evacTarget() *Zone {
	if z.evac == nil || z.World == nil {
		return nil
	}
	to := z.World.Zone(z.evac.to)
	if to == z {
		return nil
	}
	return to
}

// forward sends a player who arrives at an evacuated zone on to its
// target, without joining it. It returns false when the zone is not
// evacuated.
func (z *Zone) forward(h *handoff) bool {
	to := z.evacTarget()
	if to == nil {
		return false
	}
	h.char.Map, h.char.X, h.char.Y = to.Map, z.evac.x, z.evac.y
	to.incoming.Add(1)
	go func() {
		defer to.incoming.Done()
		select {
		case to.arrivals <- h:
		case <-to.stopped:
			h.finish(endLeave)
		}
	}()
	return true
}

// ReplaceWarps replaces the zone's warps while it runs, on its
// goroutine, as SetWarps does before Run. Players see the visible ones
// come and go with their next view update.
func (z *Zone) ReplaceWarps(ctx context.Context, warps []Warp) error {
	return z.do(ctx, func() error {
		z.SetWarps(warps)
		return nil
	})
}

// WorldChanged runs the dungeon_rotated or region_changed handlers of
// the zone's scripts (review decision 47). Any goroutine may call it. It
// never waits: when the zone is far behind, the event is dropped.
func (z *Zone) WorldChanged(ev modscript.WorldChange) {
	select {
	case z.changes <- ev:
	default:
	}
}

func (z *Zone) worldChanged(ev modscript.WorldChange) {
	if z.scripts != nil {
		z.scripts.FireWorldChange(ev)
	}
}

// Players counts the zone's players, on its goroutine.
func (z *Zone) Players(ctx context.Context) (int, error) {
	n := 0
	err := z.do(ctx, func() error {
		n = len(z.players)
		return nil
	})
	return n, err
}
