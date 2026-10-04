package store_test

import (
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// TestGuildsSaved runs the social registry's guilds with the store's
// writer on the commit lane, and checks what loads after a restart: the
// guild with its members, positions, skills, notice, relations,
// expulsions, emblem and storage, and that a guild that broke up is gone
// with its storage.
func TestGuildsSaved(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	ids := map[string]int64{}
	for _, n := range []string{"Ann", "Bob", "Cid", "Dee"} {
		id, err := s.DevCharacter(ctx, newCharacter(n))
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = id
	}
	ss := store.NewSocialStore(game, slog.New(slog.DiscardHandler))
	ss.Lane = s.Lane
	r := social.New(rules.DefaultLimits(), rules.DefaultParty())
	r.Store = ss
	ps := map[string]*social.Player{}
	for n, id := range ids {
		ps[n] = r.Online(social.Login{Name: n, CharID: id, Inbox: nullInbox{}, Level: 10, Map: "map/prontera"})
	}
	guild, _ := r.GuildCreate(ps["Ann"], "Savers")
	for _, n := range []string{"Bob", "Cid"} {
		r.GuildInvite(ps["Ann"], n)
		r.GuildReply(ps[n], guild, true)
	}
	r.GuildSetPositions(ps["Ann"], []social.PositionChange{{Index: 1, Position: social.Position{Name: "Officer", Invite: true, ExpTax: 20}}})
	r.GuildSetMemberPosition(ps["Ann"], "Bob", 1)
	r.GuildSetNotice(ps["Ann"], social.GuildNotice{Title: "Hello", Text: "Be kind"})
	r.PayExp(ps["Bob"], 150000)
	r.GuildSkillUp(ps["Ann"], social.SkillExtension)
	r.GuildExpel(ps["Ann"], "Cid", "rude")
	other, _ := r.GuildCreate(ps["Dee"], "Others")
	r.GuildEnemy(ps["Ann"], "Dee")
	if _, _, err := r.SetEmblem(guild, []byte("png")); err != nil {
		t.Fatal(err)
	}
	r.GuildSetMemberPosition(ps["Ann"], "Bob", 0)
	gone := r.ScriptGuildCreate(ps["Cid"], "Gone")

	// A storage commit for each guild.
	serial, goneSerial := item.NewSerial(), item.NewSerial()
	s.CommitWith(nil, []store.StorageWrite{
		{Guild: guild, Cur: []character.Item{{Slot: 3, ID: 909, Amount: 7, Serial: serial}}},
		{Guild: gone, Cur: []character.Item{{Slot: 0, ID: 909, Amount: 1, Serial: goneSerial}}},
	}, transfer.Logs{Items: []transfer.ItemLog{{At: time.Now(), Action: transfer.ActionGive, From: transfer.End(transfer.KindScript),
		To: transfer.Owner{Kind: transfer.KindGuildStorage, ID: gone}, Serial: goneSerial, ItemID: 909, Amount: 1}}})
	if _, err := r.DisbandGuild(gone); err != nil {
		t.Fatal(err)
	}
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ss.Close(ctx); err != nil {
		t.Fatal(err)
	}
	guilds, _, err := store.LoadGuilds(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if len(guilds) != 2 {
		t.Fatalf("guilds %+v", guilds)
	}
	g := guilds[0]
	var names []string
	for _, m := range g.Members {
		names = append(names, m.Name)
	}
	if g.ID != guild || g.Name != "Savers" || !slices.Equal(names, []string{"Bob", "Ann"}) || g.Members[0].Slot != 0 || g.Members[0].Position != 0 ||
		g.Members[1].Position != 1 || g.Members[0].Exp != 150000 {
		t.Errorf("members %+v", g.Members)
	}
	if g.Level != 2 || g.Exp != 50000 || g.SkillPoints != 0 || g.Skills[social.SkillExtension] != 1 || g.Positions[1].Name != "Officer" ||
		g.Positions[1].ExpTax != 20 || !g.Positions[1].Invite || g.Notice.Text != "Be kind" || g.LastMasterChange.IsZero() {
		t.Errorf("guild %+v", g)
	}
	if len(g.Expulsions) != 1 || g.Expulsions[0].Name != "Cid" || g.Expulsions[0].Reason != "rude" {
		t.Errorf("expulsions %+v", g.Expulsions)
	}
	if len(g.Relations) != 1 || g.Relations[0].Guild != other || !g.Relations[0].Enemy || g.Relations[0].Name != "Others" {
		t.Errorf("relations %+v", g.Relations)
	}
	if g.Emblem.Version != 1 || string(g.Emblem.PNG) != "png" {
		t.Errorf("emblem %+v", g.Emblem)
	}
	if len(g.Storage) != 1 || g.Storage[0].Serial != serial || g.Storage[0].Amount != 7 || g.Storage[0].Slot != 3 {
		t.Errorf("storage %+v", g.Storage)
	}
	var left int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM guild_storage WHERE guild_id = $1`, gone).Scan(&left); err != nil || left != 0 {
		t.Errorf("the broken guild's storage: %d, %v", left, err)
	}
	// A new registry loads them, and gives the next guild a new ID.
	r2 := social.New(rules.DefaultLimits(), rules.DefaultParty())
	r2.LoadGuilds(guilds)
	floor, err := store.GuildIDFloor(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	r2.SetGuildIDFloor(floor)
	if !r2.Enemies(guild, other) || r2.GuildRuns(gone) {
		t.Error("loaded relations")
	}
	p := r2.Online(social.Login{Name: "Zed", Inbox: nullInbox{}})
	if id, _ := r2.GuildCreate(p, "Next"); id <= gone {
		t.Errorf("new guild ID %d after %d", id, gone)
	}
}
