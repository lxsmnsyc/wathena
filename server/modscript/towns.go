package modscript

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// Towns and display names of the world layout (proposal 0002, review
// decision 43):
//   - towns.list() returns every town by number, and
//     towns.list({ visited_by = player }) the towns the player entered.
//   - towns.nearest(player) returns the town the fewest warps from the
//     player's map, and towns.get(id) a town by its ID, such as t7.
//   - A town handle is the town's number, and reads its fields with its
//     methods: get_id, get_name, get_map and get_band.
//   - player:has_visited(town), maps.get_display_name(map) and
//     place:get_band().
//
// Every call needs game.read. A classic world has no towns: list
// returns an empty list, nearest and get nil, and display names nil.

// VisitHost is the part of a Host that knows the maps a player entered.
// A host without it has players that entered no map.
type VisitHost interface {
	HasVisited(player int64, mapID string) (bool, error)
}

// townArg reads a town handle's number.
func (r *Runtime) townArg(c *script.Call, i int, what string) (int64, error) {
	n, ok := c.Arg(i).AsInt()
	if !ok || n < 1 {
		return 0, errors.New(what + ": want a town from wa.towns")
	}
	if _, found := r.opts.Atlas.Town(gen.TownID(n)); !found {
		return 0, errors.New(what + ": not a town of this world")
	}
	return n, nil
}

// townsList is towns.list(visited_by): the town numbers, or those the
// player entered when visited_by is a player.
func (r *Runtime) townsList(c *script.Call) error {
	const what = "towns.list"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	player, filter := c.Arg(0).AsInt()
	var out []script.Value
	for _, t := range r.opts.Atlas.Towns() {
		if filter {
			v, err := r.visited(player, t.ID)
			if err != nil {
				return err
			}
			if !v {
				continue
			}
		}
		out = append(out, script.Int(t.Town))
	}
	c.Return(script.List(out...))
	return nil
}

// visited reports whether a player entered a map.
func (r *Runtime) visited(player int64, mapID string) (bool, error) {
	vh, ok := r.host.(VisitHost)
	if !ok {
		return false, nil
	}
	return vh.HasVisited(player, mapID)
}

// townsNearest is towns.nearest(player): the nearest town's number, or
// nothing.
func (r *Runtime) townsNearest(c *script.Call) error {
	const what = "towns.nearest"
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New(what + ": want a player")
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	v, err := r.host.PlayerField(id, "map")
	if err != nil {
		return err
	}
	m, _ := v.AsString()
	if t, ok := r.opts.Atlas.NearestTown(m); ok {
		c.Return(script.Int(t.Town))
	}
	return nil
}

// townsGet is towns.get(id): the number of the town with the ID, such
// as t7, or nothing.
func (r *Runtime) townsGet(c *script.Call) error {
	const what = "towns.get"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	id, ok := c.Arg(0).AsString()
	n, found := gen.ParseTownMapID("gen:town:" + id)
	if !ok || !found {
		return fmt.Errorf("%s: a town's ID is t and a number, such as t7", what)
	}
	if _, ok := r.opts.Atlas.Town(id); ok {
		c.Return(script.Int(n))
	}
	return nil
}

// townGet is the methods of a town handle: Arg(0) is the town's number,
// Arg(1) the method.
func (r *Runtime) townGet(c *script.Call) error {
	n, err := r.townArg(c, 0, "town")
	if err != nil {
		return err
	}
	if err := r.need("town:get_"+c.String(1), "game.read"); err != nil {
		return err
	}
	t, _ := r.opts.Atlas.Town(gen.TownID(n))
	switch c.String(1) {
	case "id":
		c.Return(script.String(gen.TownID(n)))
	case "name":
		c.Return(script.String(t.Name))
	case "map":
		c.Return(script.String(t.ID))
	case "band":
		c.Return(script.Int(int64(t.Levels[0])), script.Int(int64(t.Levels[1])))
	default:
		return fmt.Errorf("town: no method %q", c.String(1))
	}
	return nil
}

// playerHasVisited is player:has_visited(town).
func (r *Runtime) playerHasVisited(c *script.Call) error {
	const what = "player:has_visited"
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New(what + ": call the method with a colon, such as player:has_visited(town)")
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	n, err := r.townArg(c, 1, what)
	if err != nil {
		return err
	}
	v, err := r.visited(id, gen.TownMapID(n))
	if err != nil {
		return err
	}
	c.Return(script.Bool(v))
	return nil
}

// mapsDisplayName is maps.get_display_name(map): a generated map's
// display name, or nothing.
func (r *Runtime) mapsDisplayName(c *script.Call) error {
	const what = "maps.get_display_name"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	m, ok := c.Arg(0).AsString()
	if !ok {
		return errors.New(what + ": the map must be a map ID, such as gen:town:t7")
	}
	if name, ok := r.opts.Atlas.Name(m); ok {
		c.Return(script.String(name))
	}
	return nil
}

// placeBand is place:get_band(): the band of the place's map.
func (r *Runtime) placeBand(c *script.Call) error {
	p, err := r.placeHandle(c)
	if err != nil {
		return err
	}
	if err := r.need("place:get_band", "game.read"); err != nil {
		return err
	}
	m, ok := r.opts.Atlas.Map(p.Map)
	if !ok {
		return errors.New("place:get_band: the place's map has no band: " + strconv.Quote(p.Map))
	}
	c.Return(script.Int(int64(m.Levels[0])), script.Int(int64(m.Levels[1])))
	return nil
}
