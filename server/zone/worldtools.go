package zone

import (
	"context"
	"errors"

	"github.com/lxsmnsyc/wathena/server/modscript"
)

// The zone's part of the admin API's world, events and mods tools
// (proposal 0002 review decision 36, decision 0031):
//   - Summary reads the zone's monsters and players for the world view.
//   - SetModOff turns a mod off or on in the zone (modscript,
//     switches.go). Mods that are off when the zone starts are in
//     ModsOff.
//   - ModErrors counts each mod's script errors in the zone. It lives as
//     long as the zone, so a new Lua state keeps the counts.
//   - RestartPlanned runs the restart_planned handlers of the zone's
//     scripts.

// OnlinePlayer is a player of the zone, for the world view.
type OnlinePlayer struct {
	CharacterID int64
	Name        string
	X, Y        int
	BaseLevel   int
	Job         int
}

// Summary is what the world view shows of a zone.
type Summary struct {
	// Monsters counts the living monsters.
	Monsters int
	Players  []OnlinePlayer
}

// Summary reads the zone's living monsters and its players, on the
// zone's goroutine.
func (z *Zone) Summary(ctx context.Context) (Summary, error) {
	var s Summary
	err := z.do(ctx, func() error {
		for e := range z.entities.all() {
			if e.monster != nil && !e.dead {
				s.Monsters++
			}
		}
		for _, p := range z.players {
			if p.entity == nil {
				continue
			}
			s.Players = append(s.Players, OnlinePlayer{CharacterID: p.record.ID, Name: p.Name, X: p.entity.x, Y: p.entity.y,
				BaseLevel: p.progress.BaseLevel, Job: p.char.Job})
		}
		return nil
	})
	return s, err
}

// SetModOff turns a mod off or on in the zone, on its goroutine. It
// returns the errors of the scripts that start when a mod is turned on.
func (z *Zone) SetModOff(ctx context.Context, mod string, off bool) error {
	return z.do(ctx, func() error { return z.setModOff(mod, off) })
}

func (z *Zone) setModOff(mod string, off bool) error {
	if off {
		if z.modsOff == nil {
			z.modsOff = map[string]bool{}
		}
		z.modsOff[mod] = true
	} else {
		delete(z.modsOff, mod)
	}
	if z.scripts == nil {
		return nil
	}
	return z.scripts.SetOff(mod, off)
}

// SetModsOff sets the mods that are off when the zone starts. Call it
// before Run.
func (z *Zone) SetModsOff(mods []string) {
	z.modsOff = map[string]bool{}
	for _, m := range mods {
		z.modsOff[m] = true
	}
}

// offList is the mods that are off, for a new Lua state.
func (z *Zone) offList() []string {
	out := make([]string, 0, len(z.modsOff))
	for m := range z.modsOff {
		out = append(out, m)
	}
	return out
}

// ModErrors returns each mod's script errors in the zone. Any goroutine
// may call it.
func (z *Zone) ModErrors() map[string]modscript.ModErrors { return z.modErrors.Snapshot() }

// RestartPlanned runs the restart_planned handlers of the zone's scripts
// (review decision 36). Any goroutine may call it. It never waits: when
// the zone is far behind, the event is dropped.
func (z *Zone) RestartPlanned(ev modscript.RestartPlanned) {
	select {
	case z.restarts <- ev:
	default:
	}
}

func (z *Zone) restartPlanned(ev modscript.RestartPlanned) {
	// A zone without a Lua state has no handlers to run.
	if z.scripts != nil {
		z.scripts.FireRestartPlanned(ev)
	}
}

// quietOff reports whether a script error only says that its mod is off
// in the zone, which the zone does not log.
func quietOff(err error) bool { return errors.Is(err, modscript.ErrModOff) }
