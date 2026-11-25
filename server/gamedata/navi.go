package gamedata

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/navi"
)

// Navigation (proposal 0006, section 5): the extra edges of
// data/navi.yml, which add airship, Kafra and scroll trips to the warps'
// graph. Core ships none. A record names a trip: from, to, x, y, to_x,
// to_y and service. A record that breaks a rule is an error.

// modNavi merges the navigation records into edges, in the order mods
// added them.
func modNavi(set *mod.Set) ([]navi.Edge, *mod.Overlay, error) {
	o, err := set.Overlay(mod.NaviKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var out []navi.Edge
	var errs []error
	for _, r := range o.Records {
		e, err := navi.FromRecord(r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: navi %s: %w", r.Where, r.Name, err))
			continue
		}
		out = append(out, e)
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	return out, o, nil
}
