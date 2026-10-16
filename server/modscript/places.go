package modscript

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// Named places of generated maps (proposal 0002, review decision 42):
//   - places.find(scope, tag) returns every place of a scope with a tag,
//     in the index's fixed order.
//   - places.nearest(player, tag) returns the place with the tag closest
//     to the player by the map graph, then by distance.
//   - A place handle is the place's index in the world's index, from 1.
//
// Both calls need game.read. A classic world has no places.

// placeHandle returns the place of a handle's ID, which is its index
// plus 1.
func (r *Runtime) placeHandle(c *script.Call) (places.Place, error) {
	id, ok := c.Arg(0).AsInt()
	idx := r.opts.Places
	if !ok || id < 1 || id > int64(idx.Len()) {
		return places.Place{}, errors.New("not a place: call the method with a colon, such as place:get_map()")
	}
	return idx.Place(int(id - 1)), nil
}

// tagArg reads a tag argument.
func tagArg(c *script.Call, i int, what string) (string, error) {
	tag, ok := c.Arg(i).AsString()
	if !ok || !gen.ValidTag(tag) {
		return "", fmt.Errorf("%s: the tag must be 1 to 64 lowercase letters, digits, _, -, . and :, such as blacksmith", what)
	}
	return tag, nil
}

// placesFind is places.find(scope, tag). It returns the handle IDs.
func (r *Runtime) placesFind(c *script.Call) error {
	const what = "places.find"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	text, _ := c.Arg(0).AsString()
	scope, err := places.ParseScope(text)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	tag, err := tagArg(c, 1, what)
	if err != nil {
		return err
	}
	found := r.opts.Places.Find(scope, tag)
	out := make([]script.Value, len(found))
	for i, p := range found {
		out[i] = script.Int(int64(p + 1))
	}
	c.Return(script.List(out...))
	return nil
}

// placesNearest is places.nearest(player, tag). It returns the handle
// ID, or nothing.
func (r *Runtime) placesNearest(c *script.Call) error {
	const what = "places.nearest"
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New(what + ": want a player")
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	tag, err := tagArg(c, 1, what)
	if err != nil {
		return err
	}
	var at [3]script.Value
	for i, f := range []string{"map", "x", "y"} {
		if at[i], err = r.host.PlayerField(id, f); err != nil {
			return err
		}
	}
	m, _ := at[0].AsString()
	x, _ := at[1].AsInt()
	y, _ := at[2].AsInt()
	if p, ok := r.opts.Places.Nearest(m, int(x), int(y), tag); ok {
		c.Return(script.Int(int64(p + 1)))
	}
	return nil
}

// placeGet is the methods of a place handle: Arg(0) is the handle ID,
// Arg(1) the method.
func (r *Runtime) placeGet(c *script.Call) error {
	p, err := r.placeHandle(c)
	if err != nil {
		return err
	}
	switch c.String(1) {
	case "map":
		c.Return(script.String(p.Map))
	case "position":
		c.Return(script.Int(int64(p.X)), script.Int(int64(p.Y)))
	case "tags":
		tags := make([]script.Value, len(p.Tags))
		for i, t := range p.Tags {
			tags[i] = script.String(t)
		}
		c.Return(script.List(tags...))
	case "area":
		c.Return(script.String(p.Area()))
	default:
		return fmt.Errorf("place: no method %q", c.String(1))
	}
	return nil
}
