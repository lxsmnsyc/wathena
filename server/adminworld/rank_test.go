package adminworld_test

import (
	"errors"
	"testing"

	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// TestSanctionRank: a sanction, and the lift of one, is refused for an
// owner unless the actor is an owner, and for a staff account whose role
// the actor's role does not cover, as role changes are (review decision
// 51). A game master cannot ban an admin, which was the gap.
func TestSanctionRank(t *testing.T) {
	e := setup(t)
	cfg := e.cfg
	cfg.Staff = staff.NewRegistry(staff.MustDefaultRoles(), []string{"Bossy"})
	aw := adminworld.New(cfg)

	adminChar := e.character("Admiral")
	gmChar := e.character("Gamer")
	peerChar := e.character("Peer")
	modChar := e.character("Moddo")
	ownerChar := e.character("Bossy")
	plainChar := e.character("Pleb")
	adminAcc, gmAcc, peerAcc, modAcc, ownerAcc, plainAcc := e.accountOf(adminChar), e.accountOf(gmChar), e.accountOf(peerChar),
		e.accountOf(modChar), e.accountOf(ownerChar), e.accountOf(plainChar)
	e.exec(`INSERT INTO staff_roles (account_id, role) VALUES ($1, 'admin'), ($2, 'game_master'), ($3, 'game_master'), ($4, 'moderator')`,
		adminAcc, gmAcc, peerAcc, modAcc)

	gm := admin.Actor{AccountID: gmAcc, AccountName: "Gamer", Role: "game_master"}
	mod := admin.Actor{AccountID: modAcc, AccountName: "Moddo", Role: "moderator"}
	adm := admin.Actor{AccountID: adminAcc, AccountName: "Admiral", Role: "admin"}
	owner := admin.Actor{AccountID: ownerAcc, AccountName: "Bossy", Role: staff.Owner}

	ban := func(actor admin.Actor, account int64) error {
		_, err := aw.Sanction(e.ctx, admin.SanctionOrder{Actor: actor, Kind: "ban", Account: account, Minutes: 60, Reason: "test"})
		return err
	}
	mute := func(actor admin.Actor, char int64) error {
		_, err := aw.Sanction(e.ctx, admin.SanctionOrder{Actor: actor, Kind: "mute", Character: char, Minutes: 5, Reason: "test"})
		return err
	}

	// A game master tries to ban an admin and an owner.
	if err := ban(gm, adminAcc); code(err) != "forbidden" {
		t.Errorf("a game master banned an admin: %v", err)
	}
	if err := ban(gm, ownerAcc); code(err) != "forbidden" {
		t.Errorf("a game master banned an owner: %v", err)
	}
	if err := ban(adm, ownerAcc); code(err) != "forbidden" {
		t.Errorf("an admin banned an owner: %v", err)
	}
	// The world's owner by account ID is an owner too, though
	// staff.owners does not name it (review decision 54).
	founderAcc := e.accountOf(e.character("Founder"))
	cfg.Staff.SetOwnerAccount(founderAcc)
	if err := ban(adm, founderAcc); code(err) != "forbidden" {
		t.Errorf("an admin banned the owner account: %v", err)
	}
	// A moderator cannot mute, jail or kick a game master or an admin.
	for _, kind := range []string{"mute", "jail", "kick"} {
		for _, target := range []int64{gmChar, adminChar} {
			minutes := 5
			if kind == "kick" {
				minutes = 0
			}
			_, err := aw.Sanction(e.ctx, admin.SanctionOrder{Actor: mod, Kind: kind, Character: target, Minutes: minutes, Reason: "test"})
			if code(err) != "forbidden" {
				t.Errorf("a moderator's %s of %d: %v", kind, target, err)
			}
		}
	}
	var bans int
	if err := e.admin.QueryRow(e.ctx, "SELECT count(*) FROM sanctions WHERE kind = 'ban'").Scan(&bans); err != nil || bans != 0 {
		t.Errorf("%d bans after refused ones, %v", bans, err)
	}

	// Roles that the actor's role covers can be sanctioned: a player, a
	// moderator, and another game master.
	if err := ban(gm, plainAcc); err != nil {
		t.Errorf("a game master's ban of a player: %v", err)
	}
	if err := mute(gm, modChar); err != nil {
		t.Errorf("a game master's mute of a moderator: %v", err)
	}
	if err := mute(gm, peerChar); err != nil {
		t.Errorf("a game master's mute of a game master: %v", err)
	}
	// An owner may sanction an admin. A game master cannot lift it, an
	// admin can.
	s, err := aw.Sanction(e.ctx, admin.SanctionOrder{Actor: owner, Kind: "ban", Account: adminAcc, Minutes: 60, Reason: "rogue"})
	if err != nil {
		t.Fatalf("an owner's ban of an admin: %v", err)
	}
	if _, err := aw.LiftSanction(e.ctx, admin.Lift{Actor: gm, ID: s.GetId(), Reason: "friend"}); code(err) != "forbidden" {
		t.Errorf("a game master lifted an admin's ban: %v", err)
	}
	other := e.character("Otheradmin")
	otherAcc := e.accountOf(other)
	e.exec(`INSERT INTO staff_roles (account_id, role) VALUES ($1, 'admin')`, otherAcc)
	if _, err := aw.LiftSanction(e.ctx, admin.Lift{Actor: admin.Actor{AccountID: otherAcc, AccountName: "Otheradmin", Role: "admin"},
		ID: s.GetId(), Reason: "reviewed"}); err != nil {
		t.Errorf("an admin's lift of an admin's ban: %v", err)
	}

	// The game's @mute takes the same rule.
	sess := e.login(adminChar)
	defer sess.Leave()
	o := zone.StaffOrder{Kind: "mute", Target: "Admiral", Minutes: 5, Reason: "spam"}
	o.Actor.AccountID, o.Actor.AccountName, o.Actor.Role = mod.AccountID, mod.AccountName, mod.Role
	var ce *zone.CommandError
	if _, err := aw.Command(e.ctx, o); !errors.As(err, &ce) || ce.Text != "That account's staff role has permissions your role does not have." {
		t.Errorf("a moderator's @mute of an admin: %v", err)
	}
}
