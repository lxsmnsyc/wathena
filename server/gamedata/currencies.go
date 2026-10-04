package gamedata

import (
	"context"
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// Currencies (proposal 0002, review decision 30): core's
// data/currencies.yml, which the core mod carries, with the mods'
// records over them. A record has display_name, owner and max. Records
// of mods get IDs from 10,000, as storages do. A core record needs an id
// below that.

// modCurrencies merges the currency records.
func modCurrencies(ctx context.Context, set *mod.Set, ids *mod.IDs) (*currency.Table, *mod.Overlay, error) {
	o, err := set.Overlay(mod.CurrencyKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var recs []*currency.Record
	var errs []error
	used := map[uint32]string{}
	for _, r := range o.Records {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("%s: currencies %s: %s", r.Where, r.Name, fmt.Sprintf(format, args...)))
		}
		var id uint32
		if r.Mod == mod.Core {
			n, ok := r.Fields["id"].(int)
			if !ok || n <= 0 || n >= int(mod.CurrencyIDs.Start) {
				bad("a core record needs an id from 1 to %d", mod.CurrencyIDs.Start-1)
				continue
			}
			id = uint32(n) // #nosec G115 -- checked above.
		} else {
			if _, ok := r.Fields["id"]; ok && r.SetBy["id"] == r.Mod {
				bad("id is given by the server; leave it out")
				continue
			}
			n, err := ids.ID(ctx, mod.CurrencyIDs, r.Name, r.Mod)
			if err != nil {
				return nil, nil, err
			}
			id = n
		}
		if prev, ok := used[id]; ok {
			bad("id %d is also %s", id, prev)
			continue
		}
		used[id] = r.Name
		rec, err := currency.Build(r.Name, r.Mod, id, r.Fields)
		if err != nil {
			bad("%v", err)
			continue
		}
		recs = append(recs, rec)
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	return currency.NewTable(recs), o, nil
}
