package store_test

import (
	"log/slog"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/store"
)

type nullInbox struct{}

func (nullInbox) DeliverSocial(social.Notice) {}

// TestPartiesAndFriendsSaved runs the social registry with the store's
// writer, and checks what loads after a restart: the parties with their
// slots, options and leader, and each character's friend list.
func TestPartiesAndFriendsSaved(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	ids := map[string]int64{}
	for _, n := range []string{"Ann", "Bob", "Cid"} {
		id, err := s.DevCharacter(ctx, newCharacter(n))
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = id
	}
	ss := store.NewSocialStore(game, slog.New(slog.DiscardHandler))
	r := social.New(rules.DefaultLimits(), rules.DefaultParty())
	r.Store = ss
	ps := map[string]*social.Player{}
	for n, id := range ids {
		ps[n] = r.Online(social.Login{Name: n, CharID: id, Inbox: nullInbox{}, Level: 10})
	}
	party, _ := r.Create(ps["Ann"], "Savers", social.Options{ItemEven: true})
	for _, n := range []string{"Bob", "Cid"} {
		r.Invite(ps["Ann"], n)
		r.Reply(ps[n], party, true)
	}
	r.Expel(ps["Ann"], "Bob")
	r.SetLeader(ps["Ann"], "Cid")
	r.SetOptions(ps["Cid"], social.Options{ExpEven: true, PickupShared: true})
	gone := r.ScriptCreate(ps["Bob"], "Gone")
	if err := r.Disband(gone); err != nil {
		t.Fatal(err)
	}
	r.FriendAdd(ps["Ann"], "Bob")
	r.FriendReply(ps["Bob"], "Ann", true)
	r.FriendAdd(ps["Ann"], "Cid")
	r.FriendReply(ps["Cid"], "Ann", true)
	r.FriendRemove(ps["Cid"], "Ann")
	if err := ss.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ss.Close(ctx); err != nil {
		t.Fatal(err)
	}
	parties, err := store.LoadParties(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if len(parties) != 1 {
		t.Fatalf("parties %+v", parties)
	}
	p := parties[0]
	var names []string
	for _, m := range p.Members {
		names = append(names, m.Name)
	}
	if p.ID != party || p.Name != "Savers" || p.Leader != "Cid" || p.Options != (social.Options{ExpEven: true, PickupShared: true}) ||
		!slices.Equal(names, []string{"Ann", "Cid"}) || p.Members[1].Slot != 2 {
		t.Errorf("party %+v", p)
	}
	for n, want := range map[string][]string{"Ann": {"Bob"}, "Bob": {"Ann"}, "Cid": nil} {
		c, _, err := s.Load(ctx, ids[n])
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, f := range c.Friends {
			got = append(got, f.Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s's friends %v, want %v", n, got, want)
		}
	}
}

// TestLongBlockListAndPartyLines saves a block list over rAthena's 20
// names, which the block_list limit allows up to 100, and a party chat
// line.
func TestLongBlockListAndPartyLines(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	id, err := s.DevCharacter(ctx, newCharacter("Blocker"))
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	next := c.Clone()
	for i := range 30 {
		next.Blocks = append(next.Blocks, "Name"+string(rune('A'+i%26))+string(rune('a'+i/26)))
	}
	sess.Save(next, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c, _, err = s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Blocks) != 30 {
		t.Errorf("%d blocks", len(c.Blocks))
	}
	l := store.NewChatLog(game, store.ChatLogOptions{})
	l.Line(chat.Line{ID: chat.NewLineID(c.CreatedAt), At: c.CreatedAt, Kind: chat.LineParty, Name: "Blocker", To: "Savers", Map: "map/prontera", Text: "hi"})
	if err := l.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
