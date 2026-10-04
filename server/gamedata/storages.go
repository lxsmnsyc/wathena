package gamedata

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/storage"
)

// Extra storages (proposal 0002, review decision 29): records of
// data/storages.yml. A record has:
//   - display_name: the name players see, 1 to 24 characters. It is the
//     record's name without the mod prefix when left out.
//   - slots: how many slots it holds, 1 to the storage_slots ceiling.
//   - owner: account, which its characters share, or character.
//
// Records of mods get IDs from 10,000, as companions do. A core record
// needs an id below that. Core has none: the account's storage is not a
// record.

// modStorages merges the storage records.
func modStorages(ctx context.Context, set *mod.Set, ids *mod.IDs) ([]*storage.Record, *mod.Overlay, error) {
	o, err := set.Overlay(mod.StorageKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var out []*storage.Record
	var errs []error
	ceiling := rules.Ceiling("storage_slots")
	for _, r := range o.Records {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("%s: storages %s: %s", r.Where, r.Name, fmt.Sprintf(format, args...)))
		}
		for _, k := range slices.Sorted(maps.Keys(r.Fields)) {
			switch k {
			case "id", "display_name", "slots", "owner":
			default:
				bad("%q is not a field; the fields are display_name, slots and owner", k)
			}
		}
		var id uint32
		if r.Mod == mod.Core {
			n, ok := r.Fields["id"].(int)
			if !ok || n <= 0 || n >= int(mod.StorageIDs.Start) {
				bad("a core record needs an id from 1 to %d", mod.StorageIDs.Start-1)
				continue
			}
			id = uint32(n) // #nosec G115 -- checked above.
		} else {
			if _, ok := r.Fields["id"]; ok && r.SetBy["id"] == r.Mod {
				bad("id is given by the server; leave it out")
				continue
			}
			n, err := ids.ID(ctx, mod.StorageIDs, r.Name, r.Mod)
			if err != nil {
				return nil, nil, err
			}
			id = n
		}
		rec := &storage.Record{ID: id, Name: r.Name, Account: true}
		_, local, _ := strings.Cut(r.Name, ":")
		if local == "" {
			local = r.Name
		}
		rec.DisplayName = local
		if v, ok := r.Fields["display_name"]; ok {
			s, isText := v.(string)
			if !isText || s == "" || utf8.RuneCountInString(s) > 24 {
				bad("display_name is 1 to 24 characters")
				continue
			}
			rec.DisplayName = s
		}
		slots, ok := r.Fields["slots"].(int)
		if !ok || slots < 1 || slots > ceiling {
			bad("slots is a whole number from 1 to %d, the engine's ceiling of storage_slots", ceiling)
			continue
		}
		rec.Slots = slots
		switch r.Fields["owner"] {
		case "account", nil:
		case "character":
			rec.Account = false
		default:
			bad("owner is account or character")
			continue
		}
		out = append(out, rec)
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	slices.SortFunc(out, func(a, b *storage.Record) int { return int(a.ID) - int(b.ID) })
	return out, o, nil
}
