package zone

// The staff calls of the Lua API (decision 0028): a player's staff
// role, as the world's staff registry has it, for a login with a second
// factor.

// staffRole returns the role that counts for a player's login, or "".
func (z *Zone) staffRole(p *Player) string {
	if z.Staff == nil || p.sess == nil {
		return ""
	}
	return z.Staff.RoleOfLogin(p.record.AccountID, p.sess.staff)
}

// StaffRole returns the role that counts for a player's login, or "".
func (h scriptHost) StaffRole(player int64) (string, error) {
	p, err := h.player(player)
	if err != nil {
		return "", err
	}
	return h.z.staffRole(p), nil
}

// HasStaffPermission reports whether a player's role has a permission.
func (h scriptHost) HasStaffPermission(player int64, permission string) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	role := h.z.staffRole(p)
	return role != "" && h.z.Staff.Roles().Has(role, permission), nil
}
