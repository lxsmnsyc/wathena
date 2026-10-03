package zone

import (
	"cmp"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
)

// Parties, friends and party booking (phase 5 step 4, proposal 0002
// review decision 27, decision 0020):
//   - The world's social registry holds every party and friend list. A
//     zone hands it its players at login, on a map change and at logout,
//     and runs the players' party and friend commands through it.
//   - The registry hands each player what changed through its session,
//     which this zone turns into protocol events and Lua events.
//   - HP and cells go to the members on the same map, which are in the
//     same zone: HP when it changes, as clif_party_hp, and cells once a
//     second, as party_send_xy_timer.

// partyUpdateInterval is party_update_interval: how often members on a
// map see each other's cells, in milliseconds.
const partyUpdateInterval = 1000

// socialIn is a notice from the registry for a player of this zone.
type socialIn struct {
	sess *Session
	n    social.Notice
}

// DeliverSocial hands a notice from the registry to the zone the
// session is in. It never blocks: a zone that is far behind drops it.
func (s *Session) DeliverSocial(n social.Notice) {
	z, _ := s.where()
	if z == nil {
		return
	}
	select {
	case z.socialIn <- socialIn{s, n}:
	default:
	}
}

// partyState is a player's party as this zone knows it, and the values
// its members last saw.
type partyState struct {
	// view is the player's party from the registry, or nil.
	view *social.PartyView
	// sentHP, sentMaxHP, sentX and sentY are the values the members on
	// the map last got. nextXY is when the cell goes out next.
	sentHP, sentMaxHP int64
	sentX, sentY      int
	nextXY            int64
	// level and job are the values the registry last got.
	level, job int
}

type partyCommand struct {
	create  *partyCreate
	invite  string
	reply   *partyReply
	leave   bool
	expel   string
	leader  string
	options *social.Options
}

type partyCreate struct {
	name string
	opts social.Options
}

type partyReply struct {
	party  int64
	accept bool
}

type friendCommand struct {
	add, remove string
	reply       *friendReply
}

type friendReply struct {
	name   string
	accept bool
}

type bookingCommand struct {
	register *social.Ad
	search   *bookingSearch
	delete   bool
}

type bookingSearch struct {
	level   int
	mapName string
	job     int
}

// PartyCreate makes a party with the player as its leader.
func (s *Session) PartyCreate(seq uint32, name string, itemEven, pickupShared bool) bool {
	return s.send(command{seq: seq, party: &partyCommand{create: &partyCreate{name, social.Options{ItemEven: itemEven, PickupShared: pickupShared}}}})
}

// PartyInvite invites a player by name.
func (s *Session) PartyInvite(seq uint32, name string) bool {
	return s.send(command{seq: seq, party: &partyCommand{invite: name}})
}

// PartyReply answers a party invite.
func (s *Session) PartyReply(seq uint32, party int64, accept bool) bool {
	return s.send(command{seq: seq, party: &partyCommand{reply: &partyReply{party, accept}}})
}

// PartyLeave leaves the player's party.
func (s *Session) PartyLeave(seq uint32) bool {
	return s.send(command{seq: seq, party: &partyCommand{leave: true}})
}

// PartyExpel removes a member by name.
func (s *Session) PartyExpel(seq uint32, name string) bool {
	return s.send(command{seq: seq, party: &partyCommand{expel: name}})
}

// PartySetLeader hands the lead to a member by name.
func (s *Session) PartySetLeader(seq uint32, name string) bool {
	return s.send(command{seq: seq, party: &partyCommand{leader: name}})
}

// PartyOptions changes the party's options.
func (s *Session) PartyOptions(seq uint32, expEven, itemEven, pickupShared bool) bool {
	return s.send(command{seq: seq, party: &partyCommand{options: &social.Options{ExpEven: expEven, ItemEven: itemEven, PickupShared: pickupShared}}})
}

// PartyChat says a line to the player's party.
func (s *Session) PartyChat(seq uint32, text string) bool {
	return s.send(command{seq: seq, chat: &chatCommand{kind: chatParty, text: text}})
}

// FriendAdd asks a player by name to be friends.
func (s *Session) FriendAdd(seq uint32, name string) bool {
	return s.send(command{seq: seq, friend: &friendCommand{add: name}})
}

// FriendReply answers a friend request.
func (s *Session) FriendReply(seq uint32, name string, accept bool) bool {
	return s.send(command{seq: seq, friend: &friendCommand{reply: &friendReply{name, accept}}})
}

// FriendRemove takes a friend off the list.
func (s *Session) FriendRemove(seq uint32, name string) bool {
	return s.send(command{seq: seq, friend: &friendCommand{remove: name}})
}

// BookingRegister places the player's party booking ad.
func (s *Session) BookingRegister(seq uint32, ad social.Ad) bool {
	return s.send(command{seq: seq, booking: &bookingCommand{register: &ad}})
}

// BookingSearch searches party booking ads.
func (s *Session) BookingSearch(seq uint32, level int, mapName string, job int) bool {
	return s.send(command{seq: seq, booking: &bookingCommand{search: &bookingSearch{level, mapName, job}}})
}

// BookingDelete removes the player's party booking ad.
func (s *Session) BookingDelete(seq uint32) bool {
	return s.send(command{seq: seq, booking: &bookingCommand{delete: true}})
}

// rules returns the world's rules, or the core ruleset of the zone's
// base.
func (z *Zone) worldRules() *rules.Rules {
	if z.Mods != nil && z.Mods.Rules != nil {
		return z.Mods.Rules
	}
	return rules.Of(z.renewal())
}

// socialHub is the world's social registry, or one of the zone's own,
// made on first use.
func (z *Zone) socialHub() *social.Registry {
	if z.Social == nil {
		r := z.worldRules()
		z.Social = social.New(rules.LimitsOrDefault(r), rules.PartyOrDefault(r))
		hub := z.chatHub()
		z.Social.Blocks = hub.BlocksName
		z.Social.Clock = z.wallClock
	}
	return z.Social
}

// limits are the world's limits.
func (z *Zone) limits() rules.Limits { return rules.LimitsOrDefault(z.worldRules()) }

// socialOnline adds a player that entered the zone to the registry: at
// login it goes online, after a warp it moves.
func (z *Zone) socialOnline(p *Player, h *handoff, friends []social.Friend) {
	if p.sess == nil {
		return
	}
	reg := z.socialHub()
	e := p.entity
	maxHP := int64(0)
	if e.status != nil {
		maxHP = int64(e.status.MaxHP)
	}
	p.partyState.level, p.partyState.job = p.progress.BaseLevel, p.char.Job
	if h != nil && h.soc != nil {
		p.soc = h.soc
		reg.Move(p.soc, p.sess, z.Map, e.x, e.y)
		reg.Update(p.soc, e.hp, maxHP, e.x, e.y, p.progress.BaseLevel, p.char.Job)
		return
	}
	p.soc = reg.Online(social.Login{Name: p.Name, CharID: p.record.ID, AccountID: p.record.AccountID, Inbox: p.sess,
		Map: z.Map, X: e.x, Y: e.y, HP: e.hp, MaxHP: maxHP, Level: p.progress.BaseLevel, Job: p.char.Job, Friends: friends})
}

// socialOffline takes a player that logged out out of the registry.
func (z *Zone) socialOffline(p *Player) {
	if p.soc != nil && p.sess != nil {
		z.socialHub().Offline(p.soc, p.sess)
		p.soc = nil
	}
}

// partyCommand runs a party command.
func (z *Zone) partyCommand(p *Player, c *partyCommand) {
	if p.soc == nil {
		return
	}
	reg := z.socialHub()
	switch {
	case c.create != nil:
		name := c.create.name
		if err := z.chatHub().Filter.CheckName(name); err != nil || !social.ValidPartyName(name) {
			p.pending = append(p.pending, partyResult(social.PartyBadName, name))
			return
		}
		reg.Create(p.soc, name, c.create.opts)
	case c.invite != "":
		reg.Invite(p.soc, c.invite)
	case c.reply != nil:
		reg.Reply(p.soc, c.reply.party, c.reply.accept)
	case c.leave:
		reg.Leave(p.soc)
	case c.expel != "":
		reg.Expel(p.soc, c.expel)
	case c.leader != "":
		if _, ch := reg.SetLeader(p.soc, c.leader); ch != nil {
			z.partyChanged(ch)
		}
	case c.options != nil:
		if _, ch := reg.SetOptions(p.soc, *c.options); ch != nil {
			z.partyChanged(ch)
		}
	}
}

// partyChanged queues the party_change event of a change this zone made.
func (z *Zone) partyChanged(ch *social.Change) {
	z.scriptEvents = append(z.scriptEvents, scriptEvent{party: &modscript.PartyEvent{Kind: "party_change", Party: ch.Party,
		Leader: z.playerNamed(ch.Leader), Options: partyOptions(ch.Options)}})
}

// playerNamed is the script ID of a player in this zone, or 0.
func (z *Zone) playerNamed(name string) int64 {
	k := chat.Key(name)
	for _, p := range z.players {
		if chat.Key(p.Name) == k && !p.gone() {
			return int64(p.charID)
		}
	}
	return 0
}

func partyOptions(o social.Options) modscript.PartyOptions {
	return modscript.PartyOptions{ExpEven: o.ExpEven, ItemEven: o.ItemEven, PickupShared: o.PickupShared}
}

// friendCommand runs a friend command.
func (z *Zone) friendCommand(p *Player, c *friendCommand) {
	if p.soc == nil {
		return
	}
	reg := z.socialHub()
	switch {
	case c.add != "":
		reg.FriendAdd(p.soc, c.add)
	case c.reply != nil:
		reg.FriendReply(p.soc, c.reply.name, c.reply.accept)
	case c.remove != "":
		reg.FriendRemove(p.soc, c.remove)
	}
}

// bookingCommand runs a party booking command.
func (z *Zone) bookingCommand(p *Player, c *bookingCommand) {
	if p.soc == nil {
		return
	}
	reg := z.socialHub()
	switch {
	case c.register != nil:
		reg.BookingRegister(p.soc, *c.register)
	case c.search != nil:
		reg.BookingSearch(p.soc, c.search.level, c.search.mapName, c.search.job)
	case c.delete:
		reg.BookingDelete(p.soc)
	}
}

// receiveSocial shows a notice from the registry to its player.
func (z *Zone) receiveSocial(in socialIn) {
	_, p := in.sess.where()
	if p == nil || p.gone() || p.entity == nil || !z.entities.has(p.entity) || p.sess != in.sess {
		return
	}
	n := in.n
	switch n.Kind {
	case social.NoticeParty:
		p.partyState.view = n.Party
		// The members on the map see this player's values again.
		p.partyState.sentHP, p.partyState.sentMaxHP, p.partyState.nextXY = -1, -1, 0
		p.pending = append(p.pending, partyInfo(n.Party))
	case social.NoticeMember:
		if v := p.partyState.view; v != nil {
			for i := range v.Members {
				if chat.Key(v.Members[i].Name) == chat.Key(n.Member.Name) {
					v.Members[i] = n.Member
				}
			}
		}
		p.pending = append(p.pending, partyMemberUpdate(n.Member))
	case social.NoticeInvite:
		p.pending = append(p.pending, wathenav1.Event_builder{PartyInvite: wathenav1.PartyInvite_builder{
			PartyId: uint64(n.Invite.Party), PartyName: n.Invite.PartyName, From: n.Invite.From, // #nosec G115 -- party IDs are positive.
		}.Build()}.Build())
	case social.NoticePartyResult:
		p.pending = append(p.pending, partyResult(n.PartyOutcome, n.Name))
	case social.NoticeChat:
		if p.blocking(n.Line.Name) {
			return
		}
		p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
			Channel: wathenav1.ChatChannel_CHAT_CHANNEL_PARTY, FromName: n.Line.Name, Text: n.Line.Text, LineId: n.Line.ID[:],
		}.Build()}.Build())
		p.see(n.Line)
	case social.NoticeFriends:
		list := make([]*wathenav1.Friend, 0, len(n.Friends))
		for _, f := range n.Friends {
			list = append(list, wathenav1.Friend_builder{Name: f.Name, Online: f.Online}.Build())
		}
		p.pending = append(p.pending, wathenav1.Event_builder{FriendList: wathenav1.FriendList_builder{Friends: list}.Build()}.Build())
	case social.NoticeFriendStatus:
		p.pending = append(p.pending, wathenav1.Event_builder{FriendStatus: wathenav1.FriendStatus_builder{Name: n.Name, Online: n.Online}.Build()}.Build())
	case social.NoticeFriendRequest:
		p.pending = append(p.pending, wathenav1.Event_builder{FriendRequest: wathenav1.FriendRequest_builder{From: n.Name}.Build()}.Build())
	case social.NoticeFriendResult:
		p.pending = append(p.pending, wathenav1.Event_builder{FriendResult: wathenav1.FriendResult_builder{
			Outcome: wathenav1.FriendOutcome(n.FriendOutcome), Name: n.Name, // #nosec G115 -- the outcomes match the protocol's.
		}.Build()}.Build())
	case social.NoticeEvent:
		ev := &modscript.PartyEvent{Kind: "party_join", Party: n.Event.Party, Player: int64(p.charID), Reason: n.Event.Reason}
		if n.Event.Kind == social.EventLeave {
			ev.Kind = "party_leave"
		}
		z.scriptEvents = append(z.scriptEvents, scriptEvent{party: ev})
	case social.NoticeItemShared:
		p.pending = append(p.pending, wathenav1.Event_builder{PartyItemShared: wathenav1.PartyItemShared_builder{
			Name: n.Shared.Name, ItemId: n.Shared.Item, Amount: uint32(n.Shared.Amount), // #nosec G115 -- a stack of at most 30000.
		}.Build()}.Build())
	case social.NoticeBooking:
		p.pending = append(p.pending, bookingList(n.Booking))
	}
}

// drainSocial shows every notice that waits, for a zone without its
// goroutine.
func (z *Zone) drainSocial() {
	for {
		select {
		case in := <-z.socialIn:
			z.receiveSocial(in)
		default:
			return
		}
	}
}

func partyResult(o social.PartyOutcome, name string) *wathenav1.Event {
	return wathenav1.Event_builder{PartyResult: wathenav1.PartyResult_builder{
		Outcome: wathenav1.PartyOutcome(o), Name: name, // #nosec G115 -- the outcomes match the protocol's.
	}.Build()}.Build()
}

func partyMember(m social.MemberView) *wathenav1.PartyMember {
	return wathenav1.PartyMember_builder{
		Name: m.Name, Leader: m.Leader, Online: m.Online, Map: m.Map,
		Job: uint32(max(m.Job, 0)), BaseLevel: uint32(max(m.Level, 0)), // #nosec G115 -- small values.
		Hp: m.HP, MaxHp: m.MaxHP,
		X: uint32(max(m.X, 0)), Y: uint32(max(m.Y, 0)), // #nosec G115 -- map cells.
	}.Build()
}

func partyInfo(v *social.PartyView) *wathenav1.Event {
	b := wathenav1.PartyInfo_builder{}
	if v != nil {
		b.Id, b.Name = uint64(v.ID), v.Name // #nosec G115 -- party IDs are positive.
		b.Options = wathenav1.PartyOptions_builder{ExpEven: v.Options.ExpEven, ItemEven: v.Options.ItemEven, PickupShared: v.Options.PickupShared}.Build()
		for _, m := range v.Members {
			b.Members = append(b.Members, partyMember(m))
		}
	}
	return wathenav1.Event_builder{PartyInfo: b.Build()}.Build()
}

func partyMemberUpdate(m social.MemberView) *wathenav1.Event {
	return wathenav1.Event_builder{PartyMember: wathenav1.PartyMemberUpdate_builder{Member: partyMember(m)}.Build()}.Build()
}

func bookingEntry(e social.BookingEntry) *wathenav1.PartyBookingEntry {
	jobs := make([]uint32, 0, len(e.Ad.Jobs))
	for _, j := range e.Ad.Jobs {
		jobs = append(jobs, uint32(max(j, 0))) // #nosec G115 -- job IDs.
	}
	return wathenav1.PartyBookingEntry_builder{Name: e.Name, PlacedAtMs: e.At.UnixMilli(), Ad: wathenav1.PartyBookingAd_builder{
		Level: uint32(max(e.Ad.Level, 0)), Map: e.Ad.Map, Jobs: jobs, Note: e.Ad.Note, // #nosec G115 -- 1 to 999.
	}.Build()}.Build()
}

func bookingList(v social.BookingView) *wathenav1.Event {
	b := wathenav1.PartyBookingList_builder{More: v.More, Refused: v.Refused, Registered: v.Own != nil}
	for _, e := range v.Entries {
		b.Entries = append(b.Entries, bookingEntry(e))
	}
	if v.Own != nil {
		b.Own = bookingEntry(*v.Own)
	}
	return wathenav1.Event_builder{PartyBookingList: b.Build()}.Build()
}

// memberView is what a player's party members see of it now.
func (z *Zone) memberView(p *Player) social.MemberView {
	e := p.entity
	m := social.MemberView{Name: p.Name, Online: true, Map: z.Map, Job: p.char.Job, Level: p.progress.BaseLevel, HP: e.hp, X: e.x, Y: e.y}
	if e.status != nil {
		m.MaxHP = int64(e.status.MaxHP)
	}
	if v := p.partyState.view; v != nil {
		for _, o := range v.Members {
			if chat.Key(o.Name) == chat.Key(p.Name) {
				m.Leader = o.Leader
			}
		}
	}
	return m
}

// partyID is the ID of the player's party as the zone knows it, or 0.
func (p *Player) partyID() int64 {
	if v := p.partyState.view; v != nil {
		return v.ID
	}
	return 0
}

// sameParty reports whether two players are in one party, as rAthena's
// status.party_id comparisons. A unit that is not a player has no party.
func sameParty(a, b *entity) bool {
	if a == nil || b == nil || a.player == nil || b.player == nil {
		return false
	}
	id := a.player.partyID()
	return id != 0 && id == b.player.partyID()
}

// partyUpdates sends the members on this map what changed of each
// player: HP at once, as clif_party_hp, and the cell once a second, as
// party_send_xy_timer. A level or job change goes to the registry,
// which checks the even exp share range.
func (z *Zone) partyUpdates() {
	if z.Social == nil {
		return
	}
	for _, p := range z.players {
		if p.gone() || p.soc == nil {
			continue
		}
		st := &p.partyState
		if st.level != p.progress.BaseLevel || st.job != p.char.Job {
			st.level, st.job = p.progress.BaseLevel, p.char.Job
			m := z.memberView(p)
			z.Social.Update(p.soc, m.HP, m.MaxHP, m.X, m.Y, m.Level, m.Job)
		}
		id := p.partyID()
		if id == 0 {
			continue
		}
		e := p.entity
		maxHP := int64(0)
		if e.status != nil {
			maxHP = int64(e.status.MaxHP)
		}
		hp := e.hp != st.sentHP || maxHP != st.sentMaxHP
		xy := z.now >= st.nextXY && (e.x != st.sentX || e.y != st.sentY || st.nextXY == 0)
		if !hp && !xy {
			continue
		}
		st.sentHP, st.sentMaxHP = e.hp, maxHP
		if xy {
			st.sentX, st.sentY, st.nextXY = e.x, e.y, z.now+partyUpdateInterval
		}
		ev := partyMemberUpdate(z.memberView(p))
		for _, q := range z.sortedPlayers() {
			if q != p && !q.gone() && q.partyID() == id {
				q.pending = append(q.pending, ev)
			}
		}
	}
}

// sortedPlayers are the zone's players by charID, so the order of what
// they get does not depend on map order.
func (z *Zone) sortedPlayers() []*Player {
	out := make([]*Player, 0, len(z.players))
	for _, p := range z.players {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b *Player) int { return cmp.Compare(a.charID, b.charID) })
	return out
}

// friendsOf is a loaded character's friend list for the registry.
func friendsOf(c *character.Character) []social.Friend {
	out := make([]social.Friend, 0, len(c.Friends))
	for _, f := range c.Friends {
		out = append(out, social.Friend{Name: f.Name, CharID: f.CharID})
	}
	return out
}

// ShareHubs gives z the chat hub and social registry of from, making
// them on first use, unless z has its own. Zones of one world must share
// them, since a player who warps keeps its chat and social state.
func (z *Zone) ShareHubs(from *Zone) {
	if z.Chat == nil {
		z.Chat = from.chatHub()
	}
	if z.Social == nil {
		z.Social = from.socialHub()
	}
}
