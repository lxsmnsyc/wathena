package social

import (
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/server/chat"
)

// MaxPartyName is the longest party name, in characters, as NAME_LENGTH
// less its terminator.
const MaxPartyName = 24

// Options are a party's sharing options, as rAthena's party.exp and
// party.item.
type Options struct {
	// ExpEven is exp 1: even exp share.
	ExpEven bool
	// ItemEven is item & 2: items go to a member at random or in turn.
	ItemEven bool
	// PickupShared is item & 1: members may take the items their party's
	// top attackers may take.
	PickupShared bool
}

// Party is one party.
type Party struct {
	ID      int64
	Name    string
	Options Options
	// slots are the members, as party.member[MAX_PARTY]. A member that
	// left leaves a hole, which the next member to join takes.
	slots  []*Player
	leader *Player
	// itemc is the slot that round robin item share gave the last item,
	// as party_data.itemc.
	itemc int
	ended bool
}

// members are the members in slot order, without holes.
func (pt *Party) members() []*Player {
	out := make([]*Player, 0, len(pt.slots))
	for _, m := range pt.slots {
		if m != nil {
			out = append(out, m)
		}
	}
	return out
}

func (pt *Party) size() int {
	n := 0
	for _, m := range pt.slots {
		if m != nil {
			n++
		}
	}
	return n
}

func (pt *Party) view() *PartyView {
	v := &PartyView{ID: pt.ID, Name: pt.Name, Options: pt.Options}
	for _, m := range pt.members() {
		v.Members = append(v.Members, pt.memberView(m))
	}
	return v
}

func (pt *Party) memberView(m *Player) MemberView {
	return MemberView{Name: m.name, Leader: m == pt.leader, Online: m.online, Map: m.mapName, Job: m.job, Level: m.level,
		HP: m.hp, MaxHP: m.maxHP, X: m.x, Y: m.y}
}

// PartyRecord is a saved party, as the Store reads and writes it.
type PartyRecord struct {
	ID      int64
	Name    string
	Options Options
	// Leader is the leader's name.
	Leader  string
	Members []MemberRecord
}

// MemberRecord is a saved party member.
type MemberRecord struct {
	// Slot is the member's place in the party.
	Slot      int
	CharID    int64
	AccountID int64
	Name      string
	Level     int
	Job       int
	Map       string
}

// Load puts saved parties in the registry, at start. A party without
// members is dropped, as party_check_empty, and deleted from the Store.
func (r *Registry) Load(parties []PartyRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range parties {
		// IDs of dropped parties are not given again, since their rows may
		// still be in the database.
		r.nextID = max(r.nextID, rec.ID+1)
		pt := &Party{ID: rec.ID, Name: rec.Name, Options: rec.Options}
		for _, m := range rec.Members {
			p := r.player(m.Name, m.CharID)
			if p.party != nil {
				continue
			}
			p.accountID, p.level, p.job, p.mapName = m.AccountID, m.Level, m.Job, m.Map
			for len(pt.slots) <= m.Slot {
				pt.slots = append(pt.slots, nil)
			}
			if pt.slots[m.Slot] != nil {
				pt.slots = append(pt.slots, nil)
				m.Slot = len(pt.slots) - 1
			}
			pt.slots[m.Slot] = p
			p.party = pt
			if Key(m.Name) == Key(rec.Leader) {
				pt.leader = p
			}
		}
		if pt.size() == 0 {
			r.save(func(s Store) { s.DeleteParty(pt.ID) })
			continue
		}
		if pt.leader == nil {
			pt.leader = pt.members()[0]
		}
		r.parties[pt.ID] = pt
		r.names[Key(pt.Name)] = pt
	}
}

// record is a party's saved form.
func (pt *Party) record() PartyRecord {
	rec := PartyRecord{ID: pt.ID, Name: pt.Name, Options: pt.Options}
	if pt.leader != nil {
		rec.Leader = pt.leader.name
	}
	for i, m := range pt.slots {
		if m != nil {
			rec.Members = append(rec.Members, memberRecord(i, m))
		}
	}
	return rec
}

func memberRecord(slot int, m *Player) MemberRecord {
	return MemberRecord{Slot: slot, CharID: m.charID, AccountID: m.accountID, Name: m.name, Level: m.level, Job: m.job, Map: m.mapName}
}

// broadcast hands a notice to every online member. r.mu must be held.
func (pt *Party) broadcast(n Notice) {
	for _, m := range pt.slots {
		deliver(m, n)
	}
}

// changed sends every online member the party. r.mu must be held.
func (pt *Party) changed() {
	pt.broadcast(Notice{Kind: NoticeParty, Party: pt.view()})
}

// memberChanged sends the other online members one member's values.
// r.mu must be held.
func (r *Registry) memberChanged(pt *Party, p *Player) {
	n := Notice{Kind: NoticeMember, Member: pt.memberView(p)}
	for _, m := range pt.slots {
		if m != nil && m != p {
			deliver(m, n)
		}
	}
}

// result sends a player a party outcome. r.mu must be held.
func result(p *Player, o PartyOutcome, name string) {
	deliver(p, Notice{Kind: NoticePartyResult, PartyOutcome: o, Name: name})
}

// levelRange is the base level range of a party's online members, as
// int_party_check_lv. ok is false when fewer than two are online.
func (pt *Party) levelRange() (lo, hi int, ok bool) {
	count := 0
	for _, m := range pt.slots {
		if m == nil || !m.online {
			continue
		}
		if count == 0 || m.level < lo {
			lo = m.level
		}
		if count == 0 || m.level > hi {
			hi = m.level
		}
		count++
	}
	return lo, hi, count >= 2
}

// canShareExp is party_check_exp_share: even exp share needs the online
// members' base levels within party_share_level. rAthena's family
// exception needs marriages and adoptions, which do not exist yet.
func (r *Registry) canShareExp(pt *Party) bool {
	lo, hi, ok := pt.levelRange()
	return !ok || hi-lo <= r.rules.ShareLevel
}

// checkLevels turns even exp share off when the online members' levels
// are too far apart, as int_party_check_lv. by is the player whose
// change made the check. r.mu must be held.
func (r *Registry) checkLevels(pt *Party, by *Player) {
	if !pt.Options.ExpEven || r.canShareExp(pt) {
		return
	}
	pt.Options.ExpEven = false
	r.save(func(s Store) { s.SaveParty(pt.record()) })
	pt.broadcast(Notice{Kind: NoticePartyResult, PartyOutcome: PartyExpShareOff, Name: pt.Name})
	pt.changed()
	_ = by
}

// save runs a write on the Store, when there is one. r.mu must be held,
// so writes reach the Store in order.
func (r *Registry) save(f func(Store)) {
	if r.Store != nil {
		f(r.Store)
	}
}

// ValidPartyName reports whether a party name has 1 to MaxPartyName
// characters after trimming.
func ValidPartyName(name string) bool {
	name = trimName(name)
	return name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= MaxPartyName
}

// Create makes a party with a leader from the client, as party_create
// and the char server's mapif_parse_CreateParty. Exp share starts as
// each. The leader gets party_join with the reason "create" (review
// decision 32). It returns the party's ID, or 0 and why not.
func (r *Registry) Create(leader *Player, name string, opts Options) (int64, PartyOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, o := r.create(leader, name, opts)
	if id != 0 {
		deliver(leader, Notice{Kind: NoticeEvent, Event: Event{Kind: EventJoin, Party: id, Reason: ReasonCreate}})
	}
	return id, o
}

// create makes a party with a leader. r.mu must be held.
func (r *Registry) create(leader *Player, name string, opts Options) (int64, PartyOutcome) {
	name = trimName(name)
	switch {
	case !ValidPartyName(name):
		result(leader, PartyBadName, name)
		return 0, PartyBadName
	case leader.party != nil:
		result(leader, PartyAlreadyInParty, name)
		return 0, PartyAlreadyInParty
	case r.names[Key(name)] != nil:
		result(leader, PartyNameTaken, name)
		return 0, PartyNameTaken
	}
	opts.ExpEven = false
	pt := &Party{ID: r.nextID, Name: name, Options: opts, slots: []*Player{leader}, leader: leader}
	r.nextID++
	r.parties[pt.ID] = pt
	r.names[Key(name)] = pt
	leader.party = pt
	leader.invite = nil
	r.save(func(s Store) { s.CreateParty(pt.record()) })
	result(leader, PartyCreated, name)
	pt.changed()
	return pt.ID, PartyCreated
}

// join puts a player in the first free slot. r.mu must be held.
func (r *Registry) join(pt *Party, p *Player) {
	slot := slices.Index(pt.slots, nil)
	if slot < 0 {
		pt.slots = append(pt.slots, p)
		slot = len(pt.slots) - 1
	} else {
		pt.slots[slot] = p
	}
	p.party, p.invite = pt, nil
	r.save(func(s Store) { s.AddMember(pt.ID, memberRecord(slot, p)) })
}

// full reports whether a party holds the most members the limit allows.
func (r *Registry) full(pt *Party) bool { return pt.size() >= r.limits.PartyMembers }

// sameAccount reports whether a member of the party plays a character
// of p's account, as block_account_in_same_party. Characters that only
// live in memory have no account.
func (r *Registry) sameAccount(pt *Party, p *Player) bool {
	if !r.rules.BlockAccount || p.accountID == 0 {
		return false
	}
	return slices.ContainsFunc(pt.slots, func(m *Player) bool { return m != nil && m != p && m.accountID == p.accountID })
}

// Invite asks an online player to join from's party, as party_invite.
// Only the leader may invite. A player who blocks the leader refuses at
// once, without seeing the invite (decision 0018).
func (r *Registry) Invite(from *Player, name string) PartyOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := from.party
	if pt == nil {
		result(from, PartyNoParty, "")
		return PartyNoParty
	}
	if pt.leader != from {
		result(from, PartyNotLeader, "")
		return PartyNotLeader
	}
	to := r.online(name)
	o := PartyInviteSent
	switch {
	case to == nil:
		o, name = PartyNotOnline, ""
	case to == from || to.party == pt:
		o, name = PartyTargetInParty, to.name
	case r.blocks(to, from):
		o, name = PartyRefused, to.name
	case r.sameAccount(pt, to):
		o, name = PartySameAccount, to.name
	case r.full(pt):
		o, name = PartyFull, to.name
	case to.party != nil || to.invite != nil:
		o, name = PartyTargetInParty, to.name
	default:
		name = to.name
		to.invite = &invite{party: pt.ID, from: from.key}
		deliver(to, Notice{Kind: NoticeInvite, Invite: InviteView{Party: pt.ID, PartyName: pt.Name, From: from.name}})
	}
	result(from, o, name)
	return o
}

// Reply answers a player's open invite, as party_reply_invite and the
// char server's mapif_parse_PartyAddMember. An answer to an invite the
// player does not have is ignored.
func (r *Registry) Reply(p *Player, party int64, accept bool) PartyOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv := p.invite
	if inv == nil || inv.party != party || p.party != nil {
		p.invite = nil
		return 0
	}
	p.invite = nil
	from := r.players[inv.from]
	pt := r.parties[party]
	if !accept || pt == nil {
		if from != nil {
			result(from, PartyRefused, p.name)
		}
		return PartyRefused
	}
	if r.full(pt) || r.sameAccount(pt, p) {
		if from != nil {
			result(from, PartyFull, p.name)
		}
		return PartyFull
	}
	r.join(pt, p)
	if from != nil {
		result(from, PartyAccepted, p.name)
	}
	for _, m := range pt.slots {
		if m != nil && m != from && m != p {
			result(m, PartyJoined, p.name)
		}
	}
	deliver(p, Notice{Kind: NoticeEvent, Event: Event{Kind: EventJoin, Party: pt.ID, Reason: ReasonInvited}})
	pt.changed()
	r.checkLevels(pt, p)
	return PartyAccepted
}

// withdraw takes a member out of a party, as party_member_withdraw. The
// player gets no party and its own outcome. r.mu must be held.
func (r *Registry) withdraw(pt *Party, p *Player, own PartyOutcome, reason string) {
	i := slices.Index(pt.slots, p)
	if i < 0 {
		return
	}
	pt.slots[i] = nil
	p.party = nil
	r.save(func(s Store) { s.RemoveMember(pt.ID, p.charID, p.name) })
	result(p, own, pt.Name)
	deliver(p, Notice{Kind: NoticeParty})
	deliver(p, Notice{Kind: NoticeEvent, Event: Event{Kind: EventLeave, Party: pt.ID, Reason: reason}})
}

// breakUp ends a party: every member leaves it, as the char server does
// when the leader leaves and party_broken. leaver is the member whose
// leave broke it up, or nil. r.mu must be held.
func (r *Registry) breakUp(pt *Party, leaver *Player, leaverReason string) {
	for _, m := range pt.members() {
		if m == leaver {
			r.withdraw(pt, m, PartyLeft, leaverReason)
			continue
		}
		r.withdraw(pt, m, PartyDisbanded, ReasonDisbanded)
	}
	pt.ended = true
	delete(r.parties, pt.ID)
	if r.names[Key(pt.Name)] == pt {
		delete(r.names, Key(pt.Name))
	}
	r.ended[pt.ID] = pt
	r.endedIDs = append(r.endedIDs, pt.ID)
	if len(r.endedIDs) > maxEnded {
		delete(r.ended, r.endedIDs[0])
		r.endedIDs = r.endedIDs[1:]
	}
	r.save(func(s Store) { s.DeleteParty(pt.ID) })
}

// remove takes a member out, or breaks the party up when it is the
// leader. r.mu must be held.
func (r *Registry) remove(pt *Party, p *Player, own PartyOutcome, others PartyOutcome, reason string) {
	if p == pt.leader {
		r.breakUp(pt, p, reason)
		return
	}
	r.withdraw(pt, p, own, reason)
	for _, m := range pt.slots {
		if m != nil {
			result(m, others, p.name)
		}
	}
	pt.changed()
	r.recheckRange(pt)
}

// recheckRange is int_party_check_lv after a member left: the range
// shrinks, so nothing turns off. rAthena does not turn even share back
// on. It is here for the char server's call order.
func (r *Registry) recheckRange(pt *Party) { r.checkLevels(pt, nil) }

// Leave takes a player out of its party, as party_leave. When the leader
// leaves, the party breaks up, as the char server does. It returns false
// when the player has no party.
func (r *Registry) Leave(p *Player) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := p.party
	if pt == nil {
		return false
	}
	r.remove(pt, p, PartyLeft, PartyLeft, ReasonLeft)
	return true
}

// Expel removes a member by name, as party_removemember. Only the leader
// may expel. A leader who expels itself breaks the party up.
func (r *Registry) Expel(leader *Player, name string) PartyOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := leader.party
	switch {
	case pt == nil:
		result(leader, PartyNoParty, "")
		return PartyNoParty
	case pt.leader != leader:
		result(leader, PartyNotLeader, "")
		return PartyNotLeader
	}
	m := r.memberNamed(pt, name)
	if m == nil {
		result(leader, PartyNotMember, name)
		return PartyNotMember
	}
	r.remove(pt, m, PartyExpelled, PartyExpelled, ReasonExpelled)
	return PartyExpelled
}

func (r *Registry) memberNamed(pt *Party, name string) *Player {
	k := Key(name)
	for _, m := range pt.slots {
		if m != nil && m.key == k {
			return m
		}
	}
	return nil
}

// SetLeader hands the lead to an online member, as party_changeleader.
// Only the leader may do it, and with change_party_leader_samemap the
// new leader must be on the leader's map.
func (r *Registry) SetLeader(leader *Player, name string) (PartyOutcome, *Change) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := leader.party
	switch {
	case pt == nil:
		result(leader, PartyNoParty, "")
		return PartyNoParty, nil
	case pt.leader != leader:
		result(leader, PartyNotLeader, "")
		return PartyNotLeader, nil
	}
	m := r.memberNamed(pt, name)
	switch {
	case m == nil || !m.online:
		// party_changeleader needs the new leader online in the party
		// (msg 283).
		result(leader, PartyNotMember, name)
		return PartyNotMember, nil
	case m == leader:
		return 0, nil
	case r.rules.LeaderSameMap && m.mapName != leader.mapName:
		result(leader, PartyLeaderNotHere, m.name)
		return PartyLeaderNotHere, nil
	}
	return PartyLeaderChanged, r.setLeader(pt, m)
}

func (r *Registry) setLeader(pt *Party, m *Player) *Change {
	pt.leader = m
	r.save(func(s Store) { s.SaveParty(pt.record()) })
	pt.broadcast(Notice{Kind: NoticePartyResult, PartyOutcome: PartyLeaderChanged, Name: m.name})
	pt.changed()
	return &Change{Party: pt.ID, Leader: m.name, Options: pt.Options}
}

// SetOptions changes a party's options, as party_changeoption. Only the
// leader may do it. Even exp share needs the online members' levels
// within party_share_level, as mapif_parse_PartyChangeOption; otherwise
// exp share stays each and the leader learns why.
func (r *Registry) SetOptions(leader *Player, opts Options) (PartyOutcome, *Change) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := leader.party
	switch {
	case pt == nil:
		result(leader, PartyNoParty, "")
		return PartyNoParty, nil
	case pt.leader != leader:
		result(leader, PartyNotLeader, "")
		return PartyNotLeader, nil
	}
	refused := opts.ExpEven && !r.canShareExp(pt)
	if refused {
		opts.ExpEven = false
		result(leader, PartyExpShareRefused, pt.Name)
	}
	return PartyOptionsChanged, r.setOptions(pt, opts)
}

func (r *Registry) setOptions(pt *Party, opts Options) *Change {
	pt.Options = opts
	r.save(func(s Store) { s.SaveParty(pt.record()) })
	pt.broadcast(Notice{Kind: NoticePartyResult, PartyOutcome: PartyOptionsChanged, Name: pt.Name})
	pt.changed()
	leader := ""
	if pt.leader != nil {
		leader = pt.leader.name
	}
	return &Change{Party: pt.ID, Leader: leader, Options: pt.Options}
}

// Chat sends a party chat line to every online member, the speaker too,
// as party_send_message. It returns false when the speaker has no party.
// Each zone hides the line from members who block the speaker.
func (r *Registry) Chat(from *Player, line chat.Line) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := from.party
	if pt == nil {
		return false
	}
	line.To = pt.Name
	pt.broadcast(Notice{Kind: NoticeChat, Line: line})
	return true
}

// Errors of the script calls (wa.parties).
var (
	ErrNoParty = errors.New("the party has disbanded")
)

// Script calls of wa.parties (review decision 27). They skip the
// leader's rights and the invite, as rAthena's party script commands.

// ScriptCreate makes a party for a leader, as party_create from a
// script. It returns 0 when the name is taken or the leader has a party.
func (r *Registry) ScriptCreate(leader *Player, name string) int64 {
	name = trimName(name)
	r.mu.Lock()
	taken := !ValidPartyName(name) || leader.party != nil || r.names[Key(name)] != nil
	r.mu.Unlock()
	if taken {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id, _ := r.create(leader, name, Options{})
	if id != 0 {
		deliver(leader, Notice{Kind: NoticeEvent, Event: Event{Kind: EventJoin, Party: id, Reason: ReasonScript}})
	}
	return id
}

// AddMember puts a player in a party, as party_addmember. It returns
// false when the player has a party, the party is full, or a character
// of the same account is in it.
func (r *Registry) AddMember(party int64, p *Player) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[party]
	if pt == nil {
		return false, ErrNoParty
	}
	if p.party != nil || r.full(pt) || r.sameAccount(pt, p) {
		return false, nil
	}
	r.join(pt, p)
	for _, m := range pt.slots {
		if m != nil && m != p {
			result(m, PartyJoined, p.name)
		}
	}
	deliver(p, Notice{Kind: NoticeEvent, Event: Event{Kind: EventJoin, Party: pt.ID, Reason: ReasonScript}})
	pt.changed()
	r.checkLevels(pt, p)
	return true, nil
}

// RemoveMember takes a player out of a party, as party_delmember. The
// leader's removal breaks the party up. It returns false when the player
// is not in the party.
func (r *Registry) RemoveMember(party int64, p *Player) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[party]
	if pt == nil {
		return false, ErrNoParty
	}
	if p.party != pt {
		return false, nil
	}
	r.remove(pt, p, PartyExpelled, PartyExpelled, ReasonScript)
	return true, nil
}

// ScriptSetLeader makes a member the leader, as party_changeleader from
// a script: the member must be online, and may be on any map.
func (r *Registry) ScriptSetLeader(party int64, p *Player) (bool, *Change, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[party]
	if pt == nil {
		return false, nil, ErrNoParty
	}
	if p.party != pt || !p.online || pt.leader == p {
		return false, nil, nil
	}
	return true, r.setLeader(pt, p), nil
}

// ScriptSetOptions changes a party's options, as party_changeoption from
// a script. Even exp share still needs the level range, as the char
// server checks it. It returns the options the party has after the call.
func (r *Registry) ScriptSetOptions(party int64, opts Options) (Options, *Change, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[party]
	if pt == nil {
		return Options{}, nil, ErrNoParty
	}
	if opts.ExpEven && !r.canShareExp(pt) {
		opts.ExpEven = false
	}
	if opts == pt.Options {
		return opts, nil, nil
	}
	ch := r.setOptions(pt, opts)
	return pt.Options, ch, nil
}

// Disband breaks a party up, as party_destroy.
func (r *Registry) Disband(party int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[party]
	if pt == nil {
		return ErrNoParty
	}
	r.breakUp(pt, nil, "")
	return nil
}

// Info is what scripts read of a party.
type Info struct {
	ID      int64
	Name    string
	Options Options
	// Leader is the leader's name, and LeaderOnline whether it is
	// online.
	Leader       string
	LeaderOnline bool
	// Members are every member's name in slot order, and Online the
	// online ones.
	Members []string
	Online  []string
	// Roster is every member in slot order, with its IDs.
	Roster []Member
	// Ended is set for a party that broke up.
	Ended bool
}

// Member is one member of a party, as rAthena's getpartymember reads it.
type Member struct {
	Name              string
	CharID, AccountID int64
	Online            bool
}

// Party returns what scripts read of a party, also for a party that
// broke up a short time ago. ok is false for an unknown ID.
func (r *Registry) Party(id int64) (Info, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[id]
	if pt == nil {
		pt = r.ended[id]
	}
	if pt == nil {
		return Info{}, false
	}
	in := Info{ID: pt.ID, Name: pt.Name, Options: pt.Options, Ended: pt.ended}
	if pt.leader != nil {
		in.Leader, in.LeaderOnline = pt.leader.name, pt.leader.online && !pt.ended
	}
	for _, m := range pt.members() {
		in.Members = append(in.Members, m.name)
		in.Roster = append(in.Roster, Member{Name: m.name, CharID: m.charID, AccountID: m.accountID, Online: m.online})
		if m.online {
			in.Online = append(in.Online, m.name)
		}
	}
	return in, true
}

// View returns what members see of a running party, or nil.
func (r *Registry) View(id int64) *PartyView {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pt := r.parties[id]; pt != nil {
		return pt.view()
	}
	return nil
}

// Find finds a party by name.
func (r *Registry) Find(name string) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pt := r.names[Key(trimName(name))]; pt != nil {
		return pt.ID, true
	}
	return 0, false
}

// PartyOf returns a player's party ID, or 0.
func (r *Registry) PartyOf(p *Player) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.party == nil {
		return 0
	}
	return p.party.ID
}

// Share is what the exp and item share of a kill or a pick-up read of a
// player's party.
type Share struct {
	Party   int64
	Options Options
	// Slots are the members' keys in slot order, with "" for a hole, as
	// p->data[i].
	Slots []string
	// ItemC is the round robin slot of the last shared item.
	ItemC int
}

// ShareOf returns the share state of a player's party. ok is false
// without a party.
func (r *Registry) ShareOf(p *Player) (Share, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil || p.party == nil {
		return Share{}, false
	}
	return p.party.share(), true
}

// ShareByID returns the share state of a party. ok is false for a party
// that does not run.
func (r *Registry) ShareByID(id int64) (Share, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[id]
	if pt == nil {
		return Share{}, false
	}
	return pt.share(), true
}

// ShareOfName returns the share state of the party of an online
// character, as map_charid2sd then status.party_id. ok is false when the
// character is offline or has no party.
func (r *Registry) ShareOfName(name string) (Share, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.online(name)
	if p == nil || p.party == nil {
		return Share{}, false
	}
	return p.party.share(), true
}

func (pt *Party) share() Share {
	s := Share{Party: pt.ID, Options: pt.Options, ItemC: pt.itemc, Slots: make([]string, len(pt.slots))}
	for i, m := range pt.slots {
		if m != nil {
			s.Slots[i] = m.key
		}
	}
	return s
}

// SetItemC records the slot that round robin item share gave an item.
func (r *Registry) SetItemC(party int64, slot int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pt := r.parties[party]; pt != nil {
		pt.itemc = slot
	}
}

// ItemShared tells the members on a map who got a shared item, as
// clif_party_show_picker to PARTY_SAMEMAP_WOS.
func (r *Registry) ItemShared(party int64, mapName string, s ItemShared) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pt := r.parties[party]
	if pt == nil {
		return
	}
	k := Key(s.Name)
	for _, m := range pt.slots {
		if m != nil && m.key != k && m.mapName == mapName {
			deliver(m, Notice{Kind: NoticeItemShared, Shared: s})
		}
	}
}
