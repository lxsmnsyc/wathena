package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// mods is what the loaded mods bring to the world.
type mods struct {
	set   *mod.Set
	items *mod.Items
	// npcs are the NPC placements, by map asset name.
	npcs map[string][]zone.NPC
	// zone is what every zone gets.
	zone *zone.Mods
}

// loadMods finds the installed mods, checks them against each other and
// the config, and merges their data over the core data
// (server/gamedata). base is the world's ruleset base, or "" when
// nothing fixes it. ids keeps the IDs of the records mods add.
//
// core is the mode's item scripts, converted from rAthena when the
// server starts (docs/decisions/0007-converted-item-scripts.md). They
// are core's files.
func loadMods(ctx context.Context, o modOptions, base string, mode reference.Mode, items *item.Data, ids mod.IDStore, core *convert.Core, log *slog.Logger) (*mods, error) {
	var found []*mod.Mod
	if o.Dir != "" {
		var err error
		if found, err = mod.Discover(o.Dir); err != nil {
			return nil, err
		}
	}
	cfg := mod.Config{Ruleset: o.Ruleset, Base: base, Approve: o.Approve, Disable: o.Disable, Strict: o.Strict}
	m, err := gamedata.LoadMods(ctx, found, cfg, mode, items, ids, core, log)
	if err != nil {
		return nil, err
	}
	return &mods{set: m.Set, items: m.Items, npcs: m.NPCs, zone: m.Zone}, nil
}

// reloadHandler reloads one mod's scripts in every zone, for
// development: POST /debug/wathena/mods/reload?mod=<id>. Data files and
// the manifest are not read again.
func reloadHandler(set *mod.Set, w *world.World, log *slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		id := r.FormValue("mod")
		m := set.Mod(id)
		if m == nil || m.Builtin {
			http.Error(rw, "no loaded mod has that ID", http.StatusNotFound)
			return
		}
		sc, err := set.Scripts(id)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		began := time.Now()
		err = w.ReloadMod(ctx, sc)
		log.Info("mod reloaded", "mod", id, "took", time.Since(began).Round(time.Millisecond), "err", err)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(rw, "reloaded %s in %d zones\n", m.ID(), len(w.Zones())) // #nosec G705 -- plain text, and the loaded mod's ID, which matches the mod ID pattern.
	}
}
