package store_test

import (
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/server/castle"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/store"
)

// TestCastlesSaved writes castles through the registry and the
// SocialStore's queue, and checks what loads after a restart: every
// field, and that a guild break clears the owner, in memory and in the
// database (proposal 0004, section 14).
func TestCastlesSaved(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	ids := map[string]int64{}
	for _, n := range []string{"Ann", "Bob"} {
		id, err := s.DevCharacter(ctx, newCharacter(n))
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = id
	}
	ss := store.NewSocialStore(game, slog.New(slog.DiscardHandler))
	r := social.New(rules.DefaultLimits(), rules.DefaultParty())
	r.Store = ss
	ann := r.Online(social.Login{Name: "Ann", CharID: ids["Ann"], Inbox: nullInbox{}, Level: 10, Map: "map/prontera"})
	bob := r.Online(social.Login{Name: "Bob", CharID: ids["Bob"], Inbox: nullInbox{}, Level: 10, Map: "map/prontera"})
	keep, _ := r.GuildCreate(ann, "Keepers")
	gone, _ := r.GuildCreate(bob, "Leavers")

	recs := []*castle.Record{
		{ID: 15, Name: "Kriemhild", Map: "prtg_cas01", Edition: "fe", NPC: "Agit#prtg_cas01"},
		{ID: 16, Name: "Swanhild", Map: "prtg_cas02", Edition: "fe", NPC: "Agit#prtg_cas02"},
	}
	c := castle.New(recs, nil)
	c.Store = ss
	var broke []string
	c.GuildBreak = func(npc, label string) { broke = append(broke, npc+"::"+label) }
	r.Castles = c

	c.SetOwner("Kriemhild", keep, castle.SourceScript)
	economy, defense, invested, next, kafra := 42, 17, 2, 99, true
	if err := c.Set("Kriemhild", castle.Values{Economy: &economy, Defense: &defense, InvestedEconomy: &invested,
		InvestedDefense: &invested, NextTime: &next, PayTime: &next, CreateTime: &next, Kafra: &kafra}); err != nil {
		t.Fatal(err)
	}
	if err := c.SetGuardian("Kriemhild", 7, true); err != nil {
		t.Fatal(err)
	}
	c.SetOwner("Swanhild", gone, castle.SourceScript)
	if out, _ := r.GuildDisband(bob, "Leavers"); out != social.GuildDisbanded {
		t.Fatalf("disband: %v", out)
	}
	if len(broke) != 1 || broke[0] != "Agit#prtg_cas02::OnGuildBreak" {
		t.Errorf("OnGuildBreak: %v", broke)
	}
	if err := ss.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	// A restart: load the rows into a new registry.
	saved, err := store.LoadCastles(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	again := castle.New(recs, saved)
	want := castle.State{Owner: keep, Economy: 42, Defense: 17, InvestedEconomy: 2, InvestedDefense: 2, NextTime: 99,
		PayTime: 99, CreateTime: 99, Kafra: true, Guardians: 1 << 7}
	if got, _ := again.State("Kriemhild"); got != want {
		t.Errorf("Kriemhild after a restart: %+v, want %+v", got, want)
	}
	if got, _ := again.State("Swanhild"); got.Owner != 0 {
		t.Errorf("Swanhild kept the broken guild: %+v", got)
	}
	// The game role cannot delete castle rows.
	if _, err := game.Exec(ctx, "DELETE FROM castles"); !isCode(err, "42501") {
		t.Errorf("delete as the game role: %v", err)
	}
}
