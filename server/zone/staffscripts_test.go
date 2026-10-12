package zone

import (
	"context"
	"testing"

	"github.com/lxsmnsyc/wathena/server/staff"
)

// TestStaffRoleOfPlayers checks the zone side of player:get_staff_role
// (decision 0028): a role counts only for a login with a second factor,
// the owners come from their account's name, and a change from the
// admin API applies at once.
func TestStaffRoleOfPlayers(t *testing.T) {
	z := saveZone()
	reg := staff.NewRegistry(staff.MustDefaultRoles(), []string{"Boss"})
	reg.Load(map[int64]string{3: "moderator", 4: "moderator"})
	z.Staff = reg
	join := func(id, account int64, name string, login staff.Login) *Player {
		c := savedCharacter()
		c.ID, c.AccountID, c.Name, c.Staff = id, account, name, login
		s := newSession(&saves{})
		s.staff = c.Staff
		p := newPlayer(s, c)
		s.zone, s.player = z, p
		z.join(p)
		return p
	}
	mod := join(7, 3, "Mod", staff.Login{AccountName: "ModAccount", TwoFactor: true})
	noTwo := join(8, 4, "Lazy", staff.Login{AccountName: "LazyAccount"})
	boss := join(9, 5, "Big", staff.Login{AccountName: "boss", TwoFactor: true})
	player := join(10, 6, "Plain", staff.Login{AccountName: "Plain", TwoFactor: true})
	h := scriptHost{z}
	for _, c := range []struct {
		p          *Player
		role       string
		mute, kick bool
	}{
		{mod, "moderator", true, true},
		{noTwo, "", false, false},
		{boss, staff.Owner, true, true},
		{player, "", false, false},
	} {
		role, err := h.StaffRole(int64(c.p.charID))
		mute, _ := h.HasStaffPermission(int64(c.p.charID), "moderation.mute")
		ban, _ := h.HasStaffPermission(int64(c.p.charID), "moderation.ban")
		if err != nil || role != c.role || mute != c.mute || ban != (c.role == staff.Owner) {
			t.Errorf("%s: role %q mute %v ban %v err %v, want %q", c.p.Name, role, mute, ban, err, c.role)
		}
	}
	_ = reg.StaffChanged(context.Background(), 3, "")
	_ = reg.StaffChanged(context.Background(), 6, "game_master")
	if role, _ := h.StaffRole(int64(mod.charID)); role != "" {
		t.Errorf("a removed role still counts: %q", role)
	}
	if role, _ := h.StaffRole(int64(player.charID)); role != "game_master" {
		t.Errorf("a given role does not count: %q", role)
	}
	z.Staff = nil
	if role, _ := h.StaffRole(int64(boss.charID)); role != "" {
		t.Errorf("a zone without a registry gives %q", role)
	}
}

// TestJoinKeepsStaffLogin checks that JoinCharacter keeps the login's
// staff standing on the session, which moves with the player.
func TestJoinKeepsStaffLogin(t *testing.T) {
	z := saveZone()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	c := savedCharacter()
	c.Staff = staff.Login{AccountName: "Staffer", TwoFactor: true}
	s, err := z.JoinCharacter(ctx, c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.staff != c.Staff {
		t.Errorf("the session has %+v", s.staff)
	}
	s.Leave()
}
