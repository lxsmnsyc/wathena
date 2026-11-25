package modscript

import (
	"errors"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Navigation (proposal 0006, section 5; decision 0079):
// player:navigate_to(map, { x, y, monster, services, hide_window }) shows
// the player a route to a map on the client's navigation window, as
// rAthena's navigateto. It needs no permission. It returns false when no
// map of this world has the name, which is every classic name in a
// procedural world.

// NaviTarget is where a navigation points.
type NaviTarget struct {
	// Map is a map name such as prontera.
	Map  string
	X, Y int
	// Monster is a monster ID, or 0.
	Monster int
	// Services is a NAV_* value.
	Services   int
	HideWindow bool
}

// NaviHost is what navigate_to needs from the zone.
type NaviHost interface {
	// Navigate starts a navigation for a player. It returns false when the
	// map does not run.
	Navigate(player int64, to NaviTarget) (bool, error)
}

// playerNavigate is player:navigate_to.
func (r *Runtime) playerNavigate(c *script.Call) error {
	const what = "player:navigate_to"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, ok := r.host.(NaviHost)
	if !ok {
		return errors.New(what + ": navigation does not exist here")
	}
	name, ok := c.Arg(1).AsString()
	if !ok || name == "" || len(name) > 64 {
		return errors.New(what + ": want a map's name, such as \"prontera\"")
	}
	var v [4]int64
	for i := range v {
		v[i], _ = c.Arg(2 + i).AsInt()
	}
	if v[0] < 0 || v[0] >= 1<<16 || v[1] < 0 || v[1] >= 1<<16 || v[2] < 0 || v[2] >= 1<<16 || v[3] < 0 || v[3] > 255 {
		return errors.New(what + ": x and y are cells, monster is a monster ID and services is a value of wa.navigation, 0 to 255")
	}
	ok, err = h.Navigate(id, NaviTarget{Map: name, X: int(v[0]), Y: int(v[1]), Monster: int(v[2]), Services: int(v[3]), HideWindow: c.Bool(6)})
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}
