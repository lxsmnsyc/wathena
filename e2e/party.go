package e2e

import (
	"context"
	"fmt"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Party and friend steps (phase 5 step 4).

// Skills of the party scenarios.
const (
	// SkillDivineProtection is AL_DP, which Angelus needs at level 3.
	SkillDivineProtection uint32 = 22
	// SkillAngelus is AL_ANGELUS, a party buff.
	SkillAngelus uint32 = 33
	// JobAcolyte is rAthena's Acolyte.
	JobAcolyte = 4
)

// CreateParty makes a party with the player as its leader, and returns
// the PartyInfo that follows PARTY_OUTCOME_CREATED.
func (p *Player) CreateParty(name string, itemEven, pickupShared bool) *wathenav1.PartyInfo {
	p.t.Helper()
	p.send("create a party", func(ctx context.Context) error { return p.PartyCreate(ctx, name, itemEven, pickupShared) })
	if r := p.PartyResult(); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_CREATED || r.GetName() != name {
		p.t.Fatalf("%s: creating party %s: %v", p.Name, name, r)
	}
	return p.PartyInfo(func(info *wathenav1.PartyInfo) bool { return info.GetName() == name })
}

// PartyResult waits for the next PartyResult.
func (p *Player) PartyResult() *wathenav1.PartyResult {
	p.t.Helper()
	return p.Expect("a party result", (*wathenav1.Event).HasPartyResult).GetPartyResult()
}

// PartyInfo waits for a PartyInfo that ok accepts.
func (p *Player) PartyInfo(ok func(*wathenav1.PartyInfo) bool) *wathenav1.PartyInfo {
	p.t.Helper()
	return p.Expect("the party", func(e *wathenav1.Event) bool { return e.HasPartyInfo() && ok(e.GetPartyInfo()) }).GetPartyInfo()
}

// Invite invites a player by name and returns the outcome.
func (p *Player) Invite(name string) *wathenav1.PartyResult {
	p.t.Helper()
	p.send("invite "+name, func(ctx context.Context) error { return p.PartyInvite(ctx, name) })
	return p.PartyResult()
}

// Invited waits for a party invite from a leader.
func (p *Player) Invited(from string) *wathenav1.PartyInvite {
	p.t.Helper()
	return p.Expect("an invite from "+from, func(e *wathenav1.Event) bool {
		return e.HasPartyInvite() && e.GetPartyInvite().GetFrom() == from
	}).GetPartyInvite()
}

// Accept accepts a party invite and returns the party the player joined.
func (p *Player) Accept(inv *wathenav1.PartyInvite) *wathenav1.PartyInfo {
	p.t.Helper()
	p.send("accept the invite", func(ctx context.Context) error { return p.PartyReply(ctx, inv.GetPartyId(), true) })
	return p.PartyInfo(func(info *wathenav1.PartyInfo) bool { return info.GetId() == inv.GetPartyId() })
}

// MakeParty makes a party led by leader, and invites the members, who
// accept. It returns when every player's party lists them all.
func MakeParty(name string, itemEven, pickupShared bool, leader *Player, members ...*Player) uint64 {
	leader.t.Helper()
	id := leader.CreateParty(name, itemEven, pickupShared).GetId()
	for _, m := range members {
		if r := leader.Invite(m.Name); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_INVITE_SENT {
			leader.t.Fatalf("inviting %s: %v", m.Name, r)
		}
		m.Accept(m.Invited(leader.Name))
		if r := leader.PartyResult(); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_ACCEPTED || r.GetName() != m.Name {
			leader.t.Fatalf("%s joining: the leader got %v", m.Name, r)
		}
	}
	all := append([]*Player{leader}, members...)
	for _, p := range all {
		p.Until(fmt.Sprintf("%d members in %s", len(all), name), func(_ View, g Game) bool {
			return g.Party != nil && g.Party.ID == id && len(g.Party.Members) == len(all)
		})
	}
	return id
}

// SetPartyOptions changes the party's options, and waits for the
// PartyInfo that shows them.
func (p *Player) SetPartyOptions(expEven, itemEven, pickupShared bool) *wathenav1.PartyInfo {
	p.t.Helper()
	p.send("party options", func(ctx context.Context) error { return p.PartyOptions(ctx, expEven, itemEven, pickupShared) })
	return p.Expect("the new options", func(e *wathenav1.Event) bool {
		switch r := e.GetPartyResult(); r.GetOutcome() {
		case wathenav1.PartyOutcome_PARTY_OUTCOME_NOT_LEADER, wathenav1.PartyOutcome_PARTY_OUTCOME_NO_PARTY,
			wathenav1.PartyOutcome_PARTY_OUTCOME_EXP_SHARE_REFUSED, wathenav1.PartyOutcome_PARTY_OUTCOME_EXP_SHARE_OFF:
			p.t.Fatalf("%s: changing the options: %v", p.Name, r)
		}
		o := e.GetPartyInfo().GetOptions()
		return e.HasPartyInfo() && o.GetExpEven() == expEven && o.GetItemEven() == itemEven && o.GetPickupShared() == pickupShared
	}).GetPartyInfo()
}

// SayToParty says a line to the party and waits until the player hears
// it back.
func (p *Player) SayToParty(text string) *wathenav1.ChatMessage {
	p.t.Helper()
	p.send("party chat", func(ctx context.Context) error { return p.PartyChat(ctx, text) })
	return p.HearsParty(p.Name, text)
}

// HearsParty waits for a party line from a member.
func (p *Player) HearsParty(from, text string) *wathenav1.ChatMessage {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("%s to say %q to the party", from, text), func(e *wathenav1.Event) bool {
		if r := e.GetChatRefused(); r != nil {
			p.t.Fatalf("%s: a line was refused: %s", p.Name, r.GetReason())
		}
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_PARTY && c.GetFromName() == from && c.GetText() == text
	}).GetChat()
}

// TakeShared picks up a ground item for a party with even item share,
// which may give it to another member. It waits until the item leaves
// the ground.
func (p *Player) TakeShared(it botclient.Entity) {
	p.t.Helper()
	p.send("pick up", func(ctx context.Context) error { return p.Bot.PickUp(ctx, it.ID) })
	p.Expect(fmt.Sprintf("item %d to be picked up", it.ItemID), func(e *wathenav1.Event) bool {
		if f := e.GetPickUpFailed(); f != nil && f.GetItem() == it.ID {
			p.t.Fatalf("%s: picking up item %d failed: %s", p.Name, it.ItemID, f.GetReason())
		}
		v := e.GetVanish()
		return v != nil && v.GetId() == it.ID && v.GetReason() == wathenav1.VanishReason_VANISH_REASON_PICKED_UP
	})
}

// PickUpRefused tries to pick up a ground item and returns why the
// server refused.
func (p *Player) PickUpRefused(it botclient.Entity) wathenav1.PickUpFailure {
	p.t.Helper()
	p.send("pick up", func(ctx context.Context) error { return p.Bot.PickUp(ctx, it.ID) })
	return p.Expect(fmt.Sprintf("picking up item %d to fail", it.ItemID), func(e *wathenav1.Event) bool {
		if a := e.GetInventoryAdd(); a != nil && a.GetItemId() == it.ItemID {
			p.t.Fatalf("%s: picked up item %d", p.Name, it.ItemID)
		}
		f := e.GetPickUpFailed()
		return f != nil && f.GetItem() == it.ID
	}).GetPickUpFailed().GetReason()
}

// CastOnSelf casts a skill with no damage on the player itself, and
// returns its SkillEffect.
func (p *Player) CastOnSelf(skill, level uint32) *wathenav1.SkillEffect {
	p.t.Helper()
	self := p.Self()
	p.send("cast", func(ctx context.Context) error { return p.UseSkill(ctx, skill, level, self) })
	return p.Expect(fmt.Sprintf("skill %d to take effect", skill), func(e *wathenav1.Event) bool {
		if f := e.GetSkillFailed(); f != nil && f.GetSkillId() == skill {
			p.t.Fatalf("%s: skill %d failed: %s", p.Name, skill, f.GetReason())
		}
		s := e.GetSkillEffect()
		return s != nil && s.GetSkillId() == skill && s.GetSource() == self
	}).GetSkillEffect()
}

// AddFriend asks a player by name to be friends and returns the
// outcome.
func (p *Player) AddFriend(name string) *wathenav1.FriendResult {
	p.t.Helper()
	p.send("add "+name, func(ctx context.Context) error { return p.FriendAdd(ctx, name) })
	return p.FriendResult()
}

// FriendResult waits for the next FriendResult.
func (p *Player) FriendResult() *wathenav1.FriendResult {
	p.t.Helper()
	return p.Expect("a friend result", (*wathenav1.Event).HasFriendResult).GetFriendResult()
}

// AcceptFriend waits for a friend request and accepts it. It returns
// the outcome the player got.
func (p *Player) AcceptFriend(from string) *wathenav1.FriendResult {
	p.t.Helper()
	p.Expect("a friend request from "+from, func(e *wathenav1.Event) bool {
		return e.HasFriendRequest() && e.GetFriendRequest().GetFrom() == from
	})
	p.send("accept "+from, func(ctx context.Context) error { return p.FriendReply(ctx, from, true) })
	return p.FriendResult()
}

// FriendOnline waits until a friend is on the list with an online
// state.
func (p *Player) FriendOnline(name string, online bool) {
	p.t.Helper()
	p.Until(fmt.Sprintf("%s is on the friend list, online %v", name, online), func(_ View, g Game) bool {
		for _, f := range g.Friends {
			if strings.EqualFold(f.Name, name) {
				return f.Online == online
			}
		}
		return false
	})
}

// FriendStatus waits for a FriendStatus about a friend.
func (p *Player) FriendStatus(name string, online bool) {
	p.t.Helper()
	p.Expect(fmt.Sprintf("%s to be online %v", name, online), func(e *wathenav1.Event) bool {
		s := e.GetFriendStatus()
		return s != nil && s.GetName() == name && s.GetOnline() == online
	})
}
