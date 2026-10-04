package botclient

import (
	"context"
	"maps"
	"slices"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Parties, friends and party booking (phase 5 step 4): what a bot knows
// of them from the server's events, and the commands it sends.

// WorldLimits are the world's limits from Welcome.
type WorldLimits struct {
	PartyMembers, Friends, BlockList int
	// InventorySlots, StorageSlots and CartSlots are the slots of a new
	// character's inventory, an account's storage and a cart.
	InventorySlots, StorageSlots, CartSlots int
	// Ranges are every limit's base and max, by name.
	Ranges map[string]LimitRange
}

// LimitRange is a limit's base and max.
type LimitRange struct {
	Base, Max int
}

// limitsOf reads Welcome's limits.
func limitsOf(l *wathenav1.Limits) WorldLimits {
	w := WorldLimits{PartyMembers: int(l.GetPartyMembers()), Friends: int(l.GetFriends()), BlockList: int(l.GetBlockList()),
		InventorySlots: int(l.GetInventorySlots()), StorageSlots: int(l.GetStorageSlots()), CartSlots: int(l.GetCartSlots()),
		Ranges: map[string]LimitRange{}}
	for _, r := range l.GetRanges() {
		w.Ranges[r.GetName()] = LimitRange{Base: int(r.GetBase()), Max: int(r.GetMax())}
	}
	return w
}

// PartyMember is one member of the bot's party, as the server last
// told the bot.
type PartyMember struct {
	Name           string
	Leader, Online bool
	Map            string
	Job, BaseLevel int
	HP, MaxHP      int64
	X, Y           int
}

// Party is the bot's party: the last PartyInfo, with the
// PartyMemberUpdate events since merged in.
type Party struct {
	ID   uint64
	Name string
	// ExpEven, ItemEven and PickupShared are the party's options.
	ExpEven, ItemEven, PickupShared bool
	// Members are in slot order.
	Members []PartyMember
}

// Member returns the member with a name, and false when there is none.
func (p *Party) Member(name string) (PartyMember, bool) {
	if p == nil {
		return PartyMember{}, false
	}
	for _, m := range p.Members {
		if strings.EqualFold(m.Name, name) {
			return m, true
		}
	}
	return PartyMember{}, false
}

// Friend is one name on the bot's friend list.
type Friend struct {
	Name   string
	Online bool
}

// socialState is what a bot knows of its party and friends. The caller
// of apply holds the bot's mu.
type socialState struct {
	limits WorldLimits
	party  *Party
	// invite is the last party invite the bot has not answered.
	invite *wathenav1.PartyInvite
	// friends is the friend list, and requests the names that asked to
	// be friends and got no answer yet.
	friends  []Friend
	requests []string
}

func memberOf(m *wathenav1.PartyMember) PartyMember {
	return PartyMember{Name: m.GetName(), Leader: m.GetLeader(), Online: m.GetOnline(), Map: m.GetMap(),
		Job: int(m.GetJob()), BaseLevel: int(m.GetBaseLevel()), HP: m.GetHp(), MaxHP: m.GetMaxHp(), X: int(m.GetX()), Y: int(m.GetY())}
}

func (s *socialState) apply(e *wathenav1.Event) {
	switch {
	case e.HasPartyInfo():
		info := e.GetPartyInfo()
		if info.GetId() == 0 {
			s.party = nil
			return
		}
		o := info.GetOptions()
		p := &Party{ID: info.GetId(), Name: info.GetName(), ExpEven: o.GetExpEven(), ItemEven: o.GetItemEven(), PickupShared: o.GetPickupShared()}
		for _, m := range info.GetMembers() {
			p.Members = append(p.Members, memberOf(m))
		}
		s.party = p
		s.invite = nil
	case e.HasPartyMember():
		m := memberOf(e.GetPartyMember().GetMember())
		if s.party == nil {
			return
		}
		for i := range s.party.Members {
			if strings.EqualFold(s.party.Members[i].Name, m.Name) {
				s.party.Members[i] = m
			}
		}
	case e.HasPartyInvite():
		s.invite = e.GetPartyInvite()
	case e.HasFriendList():
		s.friends = s.friends[:0]
		for _, f := range e.GetFriendList().GetFriends() {
			s.friends = append(s.friends, Friend{Name: f.GetName(), Online: f.GetOnline()})
			s.requests = slices.DeleteFunc(s.requests, func(n string) bool { return strings.EqualFold(n, f.GetName()) })
		}
	case e.HasFriendStatus():
		st := e.GetFriendStatus()
		for i := range s.friends {
			if strings.EqualFold(s.friends[i].Name, st.GetName()) {
				s.friends[i].Online = st.GetOnline()
			}
		}
	case e.HasFriendRequest():
		from := e.GetFriendRequest().GetFrom()
		if !slices.ContainsFunc(s.requests, func(n string) bool { return strings.EqualFold(n, from) }) {
			s.requests = append(s.requests, from)
		}
	}
}

// clone copies the state for Game.
func (s *socialState) clone() (*Party, *wathenav1.PartyInvite, []Friend, []string) {
	var p *Party
	if s.party != nil {
		c := *s.party
		c.Members = slices.Clone(s.party.Members)
		p = &c
	}
	return p, s.invite, slices.Clone(s.friends), slices.Clone(s.requests)
}

// WorldLimits returns the world's limits from Welcome.
func (b *Bot) WorldLimits() WorldLimits {
	b.mu.Lock()
	defer b.mu.Unlock()
	l := b.social.limits
	l.Ranges = maps.Clone(l.Ranges)
	return l
}

func (b *Bot) party(ctx context.Context, c wathenav1.PartyCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Party: c.Build()})
}

// PartyCreate makes a party with the bot as its leader. Exp share
// starts as each.
func (b *Bot) PartyCreate(ctx context.Context, name string, itemEven, pickupShared bool) error {
	return b.party(ctx, wathenav1.PartyCommand_builder{Create: wathenav1.PartyCreate_builder{
		Name: name, Options: wathenav1.PartyOptions_builder{ItemEven: itemEven, PickupShared: pickupShared}.Build(),
	}.Build()})
}

// PartyInvite invites a player by name to the bot's party.
func (b *Bot) PartyInvite(ctx context.Context, name string) error {
	return b.party(ctx, wathenav1.PartyCommand_builder{Invite: &name})
}

// PartyReply answers a party invite. The bot forgets the invite.
func (b *Bot) PartyReply(ctx context.Context, party uint64, accept bool) error {
	b.mu.Lock()
	if b.social.invite.GetPartyId() == party {
		b.social.invite = nil
	}
	b.mu.Unlock()
	return b.party(ctx, wathenav1.PartyCommand_builder{Reply: wathenav1.PartyReply_builder{PartyId: party, Accept: accept}.Build()})
}

// PartyLeave leaves the bot's party.
func (b *Bot) PartyLeave(ctx context.Context) error {
	return b.party(ctx, wathenav1.PartyCommand_builder{Leave: new(true)})
}

// PartyExpel removes a member by name.
func (b *Bot) PartyExpel(ctx context.Context, name string) error {
	return b.party(ctx, wathenav1.PartyCommand_builder{Expel: &name})
}

// PartySetLeader makes a member the leader.
func (b *Bot) PartySetLeader(ctx context.Context, name string) error {
	return b.party(ctx, wathenav1.PartyCommand_builder{SetLeader: &name})
}

// PartyOptions changes the party's options.
func (b *Bot) PartyOptions(ctx context.Context, expEven, itemEven, pickupShared bool) error {
	return b.party(ctx, wathenav1.PartyCommand_builder{Options: wathenav1.PartyOptions_builder{
		ExpEven: expEven, ItemEven: itemEven, PickupShared: pickupShared,
	}.Build()})
}

// PartyChat says a line to the bot's party.
func (b *Bot) PartyChat(ctx context.Context, text string) error {
	return b.command(ctx, wathenav1.Command_builder{Chat: wathenav1.Chat_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_PARTY, Text: text,
	}.Build()})
}

func (b *Bot) friend(ctx context.Context, c wathenav1.FriendCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Friend: c.Build()})
}

// FriendAdd asks an online player by name to be friends.
func (b *Bot) FriendAdd(ctx context.Context, name string) error {
	return b.friend(ctx, wathenav1.FriendCommand_builder{Add: &name})
}

// FriendReply answers a friend request. The bot forgets the request.
func (b *Bot) FriendReply(ctx context.Context, name string, accept bool) error {
	b.mu.Lock()
	b.social.requests = slices.DeleteFunc(b.social.requests, func(n string) bool { return strings.EqualFold(n, name) })
	b.mu.Unlock()
	return b.friend(ctx, wathenav1.FriendCommand_builder{Reply: wathenav1.FriendReply_builder{Name: name, Accept: accept}.Build()})
}

// FriendRemove takes a friend off the list, on both sides.
func (b *Bot) FriendRemove(ctx context.Context, name string) error {
	return b.friend(ctx, wathenav1.FriendCommand_builder{Remove: &name})
}

func (b *Bot) booking(ctx context.Context, c wathenav1.PartyBookingCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{PartyBooking: c.Build()})
}

// BookingRegister places the bot's party booking ad.
func (b *Bot) BookingRegister(ctx context.Context, level uint32, mapName string, jobs []uint32, note string) error {
	return b.booking(ctx, wathenav1.PartyBookingCommand_builder{Register: wathenav1.PartyBookingAd_builder{
		Level: level, Map: mapName, Jobs: jobs, Note: note,
	}.Build()})
}

// BookingSearch searches party booking ads. 0 and "" match any.
func (b *Bot) BookingSearch(ctx context.Context, level uint32, mapName string, job uint32) error {
	return b.booking(ctx, wathenav1.PartyBookingCommand_builder{Search: wathenav1.PartyBookingSearch_builder{
		Level: level, Map: mapName, Job: job,
	}.Build()})
}

// BookingDelete removes the bot's own ad.
func (b *Bot) BookingDelete(ctx context.Context) error {
	return b.booking(ctx, wathenav1.PartyBookingCommand_builder{Delete: new(true)})
}
