package botclient

import (
	"context"
	"testing"
)

// TestPartyAndFriends: the bot keeps the limits from Welcome, its party
// from PartyInfo, an open invite, and its friend list.
func TestPartyAndFriends(t *testing.T) {
	s := serve(t, nil)
	a := dial(t, s, "Alice")
	b := dial(t, s, "Bob")
	ctx := context.Background()
	if l := a.WorldLimits(); l != (WorldLimits{PartyMembers: 12, Friends: 40, BlockList: 20}) {
		t.Errorf("limits %+v", l)
	}

	if err := a.PartyCreate(ctx, "Crew", true, false); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Alice's party", func() bool { return a.Game().Party != nil })
	if p := a.Game().Party; p.Name != "Crew" || !p.ItemEven || p.PickupShared || p.ExpEven || len(p.Members) != 1 || !p.Members[0].Leader {
		t.Fatalf("party %+v", p)
	}
	if err := a.PartyInvite(ctx, "Bob"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Bob's invite", func() bool { return b.Game().Invite != nil })
	inv := b.Game().Invite
	if inv.GetFrom() != "Alice" || inv.GetPartyName() != "Crew" {
		t.Fatalf("invite %v", inv)
	}
	if err := b.PartyReply(ctx, inv.GetPartyId(), true); err != nil {
		t.Fatal(err)
	}
	eventually(t, "two members", func() bool {
		p := b.Game().Party
		return p != nil && len(p.Members) == 2 && a.Game().Party != nil && len(a.Game().Party.Members) == 2
	})
	if m, ok := a.Game().Party.Member("bob"); !ok || !m.Online || m.Leader || m.Map != "map/test" {
		t.Errorf("Bob in Alice's party: %+v", m)
	}
	if b.Game().Invite != nil {
		t.Error("the invite is still open after the answer")
	}

	if err := a.FriendAdd(ctx, "Bob"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Bob's friend request", func() bool { return len(b.Game().FriendRequests) == 1 })
	if err := b.FriendReply(ctx, "Alice", true); err != nil {
		t.Fatal(err)
	}
	eventually(t, "both friend lists", func() bool {
		fa, fb := a.Game().Friends, b.Game().Friends
		return len(fa) == 1 && fa[0] == (Friend{"Bob", true}) && len(fb) == 1 && fb[0] == (Friend{"Alice", true})
	})

	if err := a.PartyLeave(ctx); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the party is gone", func() bool { return a.Game().Party == nil && b.Game().Party == nil })
}
