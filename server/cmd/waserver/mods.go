package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
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
// the config, and merges their data over the core data. base is the
// world's ruleset base, or "" when nothing fixes it. ids keeps the IDs
// of the records mods add.
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
	set, err := mod.Resolve(found, mod.Config{Ruleset: o.Ruleset, Base: base, Approve: o.Approve, Disable: o.Disable, Strict: o.Strict, Core: core.Files})
	if err != nil {
		return nil, err
	}
	for _, n := range set.Notes {
		log.Warn("mods: " + n)
	}
	var list []string
	for _, m := range set.Mods {
		if !m.Builtin {
			list = append(list, m.ID()+" "+m.Manifest.Version)
		}
	}
	log.Info("mods loaded", "ruleset", set.Ruleset.String(), "mods", strings.Join(list, ", "))

	assigned, err := mod.LoadIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]uint32, len(items.Items))
	for _, d := range items.Items {
		byName[d.AegisName] = d.ID
	}
	merged, err := set.Items(ctx, items, assigned, core.Names(byName))
	if err != nil {
		return nil, err
	}
	for _, a := range assigned.Assigned {
		log.Info("mods: new ID", "kind", a.Kind, "name", a.Name, "id", a.ID, "mod", a.Mod)
	}
	sprites, err := npcSprites(mode)
	if err != nil {
		return nil, err
	}
	placed, npcOverlay, err := set.NPCs(func(name string) (uint32, bool) {
		id, ok := sprites[strings.ToUpper(name)]
		return id, ok
	})
	if err != nil {
		return nil, err
	}
	for _, n := range append(merged.Overlay.Notes, npcOverlay.Notes...) {
		log.Warn("mods: " + n)
	}
	out := &mods{set: set, items: merged, npcs: map[string][]zone.NPC{}}
	for _, n := range placed {
		out.npcs[n.Map] = append(out.npcs[n.Map], zone.NPC{
			Name: n.Name, DisplayName: n.DisplayName, X: n.X, Y: n.Y, Direction: n.Direction, Sprite: n.Sprite,
		})
	}
	scripts, err := set.AllScripts()
	if err != nil {
		return nil, err
	}
	if err := checkItemScripts(merged.Data, scripts); err != nil {
		return nil, err
	}
	groups, err := item.NewGroups(core.Groups, func(name string) (uint32, bool) {
		id, ok := merged.IDs[name]
		return id, ok
	})
	if err != nil {
		return nil, err
	}
	var all []string
	for _, m := range set.Mods {
		all = append(all, m.ID())
	}
	out.zone = &zone.Mods{Scripts: scripts, IDs: all, ItemIDs: merged.IDs, Log: log,
		Constants: core.Constants, Groups: groups, MonsterIDs: core.Monsters}
	return out, nil
}

// checkItemScripts checks that every item's script exists, so a typo in
// a mod's script field fails at start and not at the first use.
func checkItemScripts(items *item.Data, scripts []*mod.Scripts) error {
	files := map[string]bool{}
	for _, sc := range scripts {
		for p := range sc.Files {
			files[sc.Mod+":"+p] = true
		}
	}
	var errs []error
	for _, d := range items.Items {
		if d.Script != "" && !files[d.Script] {
			errs = append(errs, fmt.Errorf("items %s: script %s does not exist", d.AegisName, d.Script))
		}
	}
	return errors.Join(errs...)
}

// npcSprites reads the NPC sprite IDs of the client tables that
// reference/rathena extracted, by upper-case name, such as 4_F_KAFRA1.
func npcSprites(mode reference.Mode) (map[string]uint32, error) {
	var d struct {
		NPCSprites []struct {
			ID     uint32 `json:"id"`
			Sprite string `json:"sprite"`
		} `json:"npc_sprites"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/client_tables.json", &d); err != nil {
		return nil, err
	}
	out := make(map[string]uint32, len(d.NPCSprites))
	for _, s := range d.NPCSprites {
		out[strings.ToUpper(s.Sprite)] = s.ID
	}
	return out, nil
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
