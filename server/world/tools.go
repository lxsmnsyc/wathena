package world

import (
	"context"
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/server/modscript"
)

// The world's part of the admin API's world, events and mods tools
// (decision 0031).

// AnnounceMap shows a line to every player of one map, by asset name.
// It reports false when the map does not run.
func (w *World) AnnounceMap(mapName, text string, color uint32) bool {
	z := w.zones[mapName]
	if z == nil {
		return false
	}
	z.Announce(text, color)
	return true
}

// RestartPlanned runs the restart_planned handlers of every zone. It
// never waits on a zone.
func (w *World) RestartPlanned(ev modscript.RestartPlanned) {
	for _, z := range w.zones {
		z.RestartPlanned(ev)
	}
}

// SetModOff turns a mod off or on in the zones of some maps, by asset
// name, or in every zone when maps is empty. It goes on after a zone's
// error, and returns them all.
func (w *World) SetModOff(ctx context.Context, mod string, maps []string, off bool) error {
	zones := w.Zones()
	if len(maps) > 0 {
		zones = zones[:0]
		for _, m := range maps {
			z := w.zones[m]
			if z == nil {
				return fmt.Errorf("the map %s does not run in this world", m)
			}
			zones = append(zones, z)
		}
	}
	var errs []error
	for _, z := range zones {
		if err := z.SetModOff(ctx, mod, off); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", z.Map, err))
		}
	}
	return errors.Join(errs...)
}
