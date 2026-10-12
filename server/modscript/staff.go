package modscript

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/staff"
)

// Staff roles (proposal 0002, review decision 33, decision 0028):
//   - player:get_staff_role() returns the player's role, or nil.
//   - player:has_staff_permission(name) returns whether the role has a
//     permission of staff.Catalog.
//   - Both need game.read. A role counts only when the player logged in
//     with a second factor, as on the admin screens.

// StaffHost is what the staff calls need from the zone.
type StaffHost interface {
	// StaffRole returns the role that counts for a player's login, or "".
	StaffRole(player int64) (string, error)
	// HasStaffPermission reports whether that role has a permission of
	// staff.Catalog.
	HasStaffPermission(player int64, permission string) (bool, error)
}

// playerStaff is player:get_staff_role and player:has_staff_permission.
// Arguments: the player, the call ("role" or "has"), and the
// permission's name for "has".
func (r *Runtime) playerStaff(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	op := c.String(1)
	what := "player:get_staff_role"
	if op == "has" {
		what = "player:has_staff_permission"
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, ok := r.host.(StaffHost)
	if !ok {
		return errors.New(what + ": staff roles do not exist here")
	}
	switch op {
	case "role":
		role, err := h.StaffRole(id)
		if err != nil {
			return err
		}
		if role == "" {
			c.Return(script.Nil)
			return nil
		}
		c.Return(script.String(role))
	case "has":
		name := c.String(2)
		if !staff.Known(name) {
			return fmt.Errorf("%s: there is no staff permission %q; the permissions are %s", what, name, strings.Join(staff.Names(), ", "))
		}
		b, err := h.HasStaffPermission(id, name)
		if err != nil {
			return err
		}
		c.Return(script.Bool(b))
	default:
		return fmt.Errorf("player: no staff call %q", op)
	}
	return nil
}
