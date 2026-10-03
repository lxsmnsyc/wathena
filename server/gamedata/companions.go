package gamedata

import (
	"context"
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// Companions and mounts (proposal 0002, review decision 25): core's
// data/companions.yml and data/mounts.yml, which the core mod carries,
// with the mods' records over them. mods/README.md, "Companions, mounts
// and carts", lists the fields.

// modCompanions merges the companion and mount records. New records of
// mods get IDs from ids. skills resolves the skills of requires, and
// names the names of bonuses.
func modCompanions(ctx context.Context, set *mod.Set, ids *mod.IDs, skills *skill.DB, names bonus.Names) (*companion.Set, []*mod.Overlay, error) {
	out := &companion.Set{}
	var overlays []*mod.Overlay
	resolve := companion.Names{
		Skill: func(name string) (int, bool) {
			if d := skills.ByName(name); d != nil {
				return d.ID, true
			}
			return 0, false
		},
		Bonus: names,
	}
	var errs []error
	for _, k := range []struct {
		kind companion.Kind
		mod  mod.Kind
		ids  mod.IDRange
	}{{companion.Companions, mod.CompanionKind, mod.CompanionIDs}, {companion.Mounts, mod.MountKind, mod.MountIDs}} {
		o, err := set.Overlay(k.mod, func(string) bool { return false })
		if err != nil {
			return nil, nil, err
		}
		overlays = append(overlays, o)
		used := map[int]string{}
		for _, r := range o.Records {
			id := 0
			if r.Mod == mod.Core {
				n, ok := r.Fields["id"].(int)
				if !ok || n <= 0 || n >= int(k.ids.Start) {
					errs = append(errs, fmt.Errorf("%s: %s %s: a core record needs an id from 1 to %d", r.Where, k.kind.Name, r.Name, k.ids.Start-1))
					continue
				}
				id = n
			} else {
				if _, ok := r.Fields["id"]; ok && r.SetBy["id"] == r.Mod {
					errs = append(errs, fmt.Errorf("%s: %s %s: id is given by the server; leave it out", r.Where, k.kind.Name, r.Name))
					continue
				}
				n, err := ids.ID(ctx, k.ids, r.Name, r.Mod)
				if err != nil {
					return nil, nil, err
				}
				id = int(n)
			}
			if prev, ok := used[id]; ok {
				errs = append(errs, fmt.Errorf("%s: %s %s: id %d is also %s", r.Where, k.kind.Name, r.Name, id, prev))
				continue
			}
			used[id] = r.Name
			rec, err := k.kind.Build(r.Name, r.Mod, id, r.Fields, resolve)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %s %s: %w", r.Where, k.kind.Name, r.Name, err))
				continue
			}
			out.Add(k.kind, rec)
		}
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	// A skill's requirements may name a companion.
	for _, d := range skills.All() {
		if c := d.Requires.Companion; c != "" && out.Companions.ByName(c) == nil {
			errs = append(errs, fmt.Errorf("skills %s: requirements: there is no companion %q; the companions are %v", d.Name, c, out.Companions.Names()))
		}
	}
	return out, overlays, errors.Join(errs...)
}
