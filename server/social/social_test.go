package social

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// box is an Inbox that keeps what it gets.
type box struct{ got []Notice }

func (b *box) DeliverSocial(n Notice) { b.got = append(b.got, n) }

func (b *box) results() []PartyOutcome {
	var out []PartyOutcome
	for _, n := range b.got {
		if n.Kind == NoticePartyResult {
			out = append(out, n.PartyOutcome)
		}
	}
	return out
}

func (b *box) last(kind NoticeKind) (Notice, bool) {
	for i := len(b.got) - 1; i >= 0; i-- {
		if b.got[i].Kind == kind {
			return b.got[i], true
		}
	}
	return Notice{}, false
}

type world struct {
	r     *Registry
	boxes map[string]*box
	ps    map[string]*Player
}

func newWorld(t *testing.T, limits rules.Limits, party rules.Party, names ...string) *world {
	t.Helper()
	w := &world{r: New(limits, party), boxes: map[string]*box{}, ps: map[string]*Player{}}
	for i, n := range names {
		w.login(n, int64(i+1), 50)
	}
	return w
}

func (w *world) login(name string, account int64, level int) *Player {
	b := &box{}
	w.boxes[name] = b
	p := w.r.Online(Login{Name: name, AccountID: account, Inbox: b, Map: "map/prontera", Level: level})
	w.ps[name] = p
	return p
}

func (w *world) party(t *testing.T, names ...string) int64 {
	t.Helper()
	id, o := w.r.Create(w.ps[names[0]], "Team "+names[0], Options{})
	if id == 0 {
		t.Fatalf("create: %v", o)
	}
	for _, n := range names[1:] {
		if o := w.r.Invite(w.ps[names[0]], n); o != PartyInviteSent {
			t.Fatalf("invite %s: %v", n, o)
		}
		if o := w.r.Reply(w.ps[n], id, true); o != PartyAccepted {
			t.Fatalf("reply %s: %v", n, o)
		}
	}
	return id
}

func TestCreateInviteAccept(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob", "Cid")
	id := w.party(t, "Ann", "Bob")
	if got := w.boxes["Ann"].results(); !slices.Equal(got, []PartyOutcome{PartyCreated, PartyInviteSent, PartyAccepted}) {
		t.Errorf("Ann's outcomes %v", got)
	}
	n, _ := w.boxes["Bob"].last(NoticeParty)
	if n.Party == nil || n.Party.ID != id || len(n.Party.Members) != 2 || !n.Party.Members[0].Leader {
		t.Errorf("Bob's party %+v", n.Party)
	}
	ev, _ := w.boxes["Bob"].last(NoticeEvent)
	if ev.Event != (Event{Kind: EventJoin, Party: id, Reason: ReasonInvited}) {
		t.Errorf("Bob's event %+v", ev.Event)
	}
	// Only the leader invites; a member cannot.
	if o := w.r.Invite(w.ps["Bob"], "Cid"); o != PartyNotLeader {
		t.Errorf("Bob invites: %v", o)
	}
	// A taken name, and a player with a party.
	if _, o := w.r.Create(w.ps["Cid"], "team ann", Options{}); o != PartyNameTaken {
		t.Errorf("taken name: %v", o)
	}
	if _, o := w.r.Create(w.ps["Bob"], "Other", Options{}); o != PartyAlreadyInParty {
		t.Errorf("Bob creates: %v", o)
	}
	// A reply to an invite the player does not have does nothing.
	if o := w.r.Reply(w.ps["Cid"], id, true); o != 0 || w.r.PartyOf(w.ps["Cid"]) != 0 {
		t.Errorf("forged reply: %v", o)
	}
	// A refused invite tells the leader.
	w.r.Invite(w.ps["Ann"], "Cid")
	if o := w.r.Reply(w.ps["Cid"], id, false); o != PartyRefused {
		t.Errorf("refuse: %v", o)
	}
	if got := w.boxes["Ann"].results(); got[len(got)-1] != PartyRefused {
		t.Errorf("Ann's outcomes %v", got)
	}
}

func TestInviteRespectsBlocksAndLimits(t *testing.T) {
	limits := rules.DefaultLimits()
	limits.PartyMembers = 2
	w := newWorld(t, limits, rules.DefaultParty(), "Ann", "Bob", "Cid", "Dan")
	w.r.Blocks = func(target, from string) bool { return chat.Key(target) == "cid" && chat.Key(from) == "ann" }
	id := w.party(t, "Ann", "Bob")
	if o := w.r.Invite(w.ps["Ann"], "Dan"); o != PartyFull {
		t.Errorf("full: %v", o)
	}
	// A player who blocks the leader refuses without seeing the invite.
	w.r.limits.PartyMembers = 12
	if o := w.r.Invite(w.ps["Ann"], "Cid"); o != PartyRefused {
		t.Errorf("blocked: %v", o)
	}
	if _, ok := w.boxes["Cid"].last(NoticeInvite); ok {
		t.Error("Cid saw the invite")
	}
	if o := w.r.Invite(w.ps["Ann"], "Nobody"); o != PartyNotOnline {
		t.Errorf("offline: %v", o)
	}
	// A character of the same account is refused.
	w.login("Bob2", 2, 50)
	if o := w.r.Invite(w.ps["Ann"], "Bob2"); o != PartySameAccount {
		t.Errorf("same account: %v", o)
	}
	_ = id
}

func TestLeaderLeavingBreaksUp(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob", "Cid")
	id := w.party(t, "Ann", "Bob", "Cid")
	// A member leaves: the others see it.
	w.r.Leave(w.ps["Cid"])
	if got := w.boxes["Ann"].results(); got[len(got)-1] != PartyLeft {
		t.Errorf("Ann's outcomes %v", got)
	}
	// The leader leaves: the party breaks up, as the char server does.
	w.r.Leave(w.ps["Ann"])
	if w.r.PartyOf(w.ps["Bob"]) != 0 {
		t.Error("Bob is still in a party")
	}
	ev, _ := w.boxes["Bob"].last(NoticeEvent)
	if ev.Event.Reason != ReasonDisbanded || ev.Event.Kind != EventLeave {
		t.Errorf("Bob's event %+v", ev.Event)
	}
	ev, _ = w.boxes["Ann"].last(NoticeEvent)
	if ev.Event.Reason != ReasonLeft {
		t.Errorf("Ann's event %+v", ev.Event)
	}
	if in, ok := w.r.Party(id); !ok || !in.Ended || in.Name != "Team Ann" {
		t.Errorf("ended party %+v %v", in, ok)
	}
	if _, ok := w.r.Find("Team Ann"); ok {
		t.Error("the name is still taken")
	}
}

func TestSlotsAndExpel(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob", "Cid", "Dan")
	id := w.party(t, "Ann", "Bob", "Cid")
	if o := w.r.Expel(w.ps["Bob"], "Cid"); o != PartyNotLeader {
		t.Errorf("Bob expels: %v", o)
	}
	if o := w.r.Expel(w.ps["Ann"], "Bob"); o != PartyExpelled {
		t.Errorf("expel: %v", o)
	}
	// Dan takes Bob's slot, as party.member's first empty slot.
	w.r.Invite(w.ps["Ann"], "Dan")
	w.r.Reply(w.ps["Dan"], id, true)
	sh, _ := w.r.ShareByID(id)
	if !slices.Equal(sh.Slots, []string{"ann", "dan", "cid"}) {
		t.Errorf("slots %v", sh.Slots)
	}
}

func TestSetLeaderSameMap(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob")
	w.party(t, "Ann", "Bob")
	w.r.Move(w.ps["Bob"], w.boxes["Bob"], "map/izlude", 1, 1)
	if o, _ := w.r.SetLeader(w.ps["Ann"], "Bob"); o != PartyLeaderNotHere {
		t.Errorf("other map: %v", o)
	}
	w.r.Move(w.ps["Bob"], w.boxes["Bob"], "map/prontera", 1, 1)
	o, ch := w.r.SetLeader(w.ps["Ann"], "Bob")
	if o != PartyLeaderChanged || ch == nil || ch.Leader != "Bob" {
		t.Errorf("set leader: %v %+v", o, ch)
	}
	if o := w.r.Invite(w.ps["Ann"], "Cid"); o != PartyNotLeader {
		t.Errorf("old leader invites: %v", o)
	}
}

func TestEvenShareLevelRange(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob")
	id := w.party(t, "Ann", "Bob")
	// 50 and 50: even share is allowed.
	if o, _ := w.r.SetOptions(w.ps["Ann"], Options{ExpEven: true}); o != PartyOptionsChanged {
		t.Fatalf("options: %v", o)
	}
	if in, _ := w.r.Party(id); !in.Options.ExpEven {
		t.Fatal("even share is off")
	}
	// Bob reaches 66: 16 levels apart, above party_share_level 15.
	w.r.Update(w.ps["Bob"], 1, 1, 0, 0, 66, 0)
	if in, _ := w.r.Party(id); in.Options.ExpEven {
		t.Error("even share stayed on")
	}
	if got := w.boxes["Bob"].results(); got[len(got)-1] != PartyExpShareOff {
		t.Errorf("Bob's outcomes %v", got)
	}
	// Asking for even share now is refused.
	w.r.SetOptions(w.ps["Ann"], Options{ExpEven: true, ItemEven: true})
	in, _ := w.r.Party(id)
	if in.Options.ExpEven || !in.Options.ItemEven {
		t.Errorf("options %+v", in.Options)
	}
	// Offline members do not count: with Bob offline, Ann alone may share.
	w.r.Offline(w.ps["Bob"], w.boxes["Bob"])
	w.r.SetOptions(w.ps["Ann"], Options{ExpEven: true})
	if in, _ := w.r.Party(id); !in.Options.ExpEven {
		t.Error("even share refused with one member online")
	}
	// Bob logs in at 66: even share turns off.
	w.login("Bob", 2, 66)
	if in, _ := w.r.Party(id); in.Options.ExpEven {
		t.Error("even share stayed on after Bob's login")
	}
}

func TestPartyChatAndReads(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob", "Cid")
	id := w.party(t, "Ann", "Bob")
	if !w.r.Chat(w.ps["Bob"], chat.Line{Name: "Bob", Text: "hi"}) {
		t.Fatal("chat failed")
	}
	for _, n := range []string{"Ann", "Bob"} {
		if c, ok := w.boxes[n].last(NoticeChat); !ok || c.Line.Text != "hi" || c.Line.To != "Team Ann" {
			t.Errorf("%s got %+v", n, c)
		}
	}
	if _, ok := w.boxes["Cid"].last(NoticeChat); ok {
		t.Error("Cid got the line")
	}
	if w.r.Chat(w.ps["Cid"], chat.Line{Name: "Cid", Text: "x"}) {
		t.Error("Cid has no party")
	}
	in, _ := w.r.Party(id)
	if in.Leader != "Ann" || !in.LeaderOnline || !slices.Equal(in.Members, []string{"Ann", "Bob"}) {
		t.Errorf("info %+v", in)
	}
}

func TestScriptCalls(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob", "Cid")
	id := w.r.ScriptCreate(w.ps["Ann"], "Scripted")
	if id == 0 || w.r.ScriptCreate(w.ps["Bob"], "scripted") != 0 {
		t.Fatal("script create")
	}
	if ok, _ := w.r.AddMember(id, w.ps["Bob"]); !ok {
		t.Fatal("add")
	}
	if ok, _ := w.r.AddMember(id, w.ps["Bob"]); ok {
		t.Error("added twice")
	}
	ev, _ := w.boxes["Bob"].last(NoticeEvent)
	if ev.Event.Reason != ReasonScript {
		t.Errorf("event %+v", ev.Event)
	}
	if ok, ch, _ := w.r.ScriptSetLeader(id, w.ps["Bob"]); !ok || ch == nil {
		t.Error("set leader")
	}
	if opts, _, _ := w.r.ScriptSetOptions(id, Options{ExpEven: true, PickupShared: true}); !opts.ExpEven || !opts.PickupShared {
		t.Errorf("options %+v", opts)
	}
	if ok, _ := w.r.RemoveMember(id, w.ps["Ann"]); !ok {
		t.Error("remove")
	}
	if err := w.r.Disband(id); err != nil {
		t.Error(err)
	}
	if err := w.r.Disband(id); err != ErrNoParty {
		t.Errorf("second disband: %v", err)
	}
}

func TestFriends(t *testing.T) {
	limits := rules.DefaultLimits()
	limits.Friends = 1
	w := newWorld(t, limits, rules.DefaultParty(), "Ann", "Bob", "Cid")
	if o := w.r.FriendAdd(w.ps["Ann"], "Bob"); o != FriendRequestSent {
		t.Fatalf("add: %v", o)
	}
	if req, ok := w.boxes["Bob"].last(NoticeFriendRequest); !ok || req.Name != "Ann" {
		t.Fatalf("Bob's request %+v", req)
	}
	if o := w.r.FriendReply(w.ps["Bob"], "Ann", true); o != FriendAdded {
		t.Fatalf("reply: %v", o)
	}
	// friend_auto_add puts each on the other's list.
	if !w.r.IsFriend(w.ps["Ann"], "bob") || !w.r.IsFriend(w.ps["Bob"], "Ann") {
		t.Error("not friends")
	}
	// The list holds one name.
	if o := w.r.FriendAdd(w.ps["Ann"], "Cid"); o != FriendListFull {
		t.Errorf("full: %v", o)
	}
	// Bob logs out and in: Ann sees it.
	w.r.Offline(w.ps["Bob"], w.boxes["Bob"])
	if st, _ := w.boxes["Ann"].last(NoticeFriendStatus); st.Name != "Bob" || st.Online {
		t.Errorf("status %+v", st)
	}
	w.login("Bob", 2, 50)
	if st, _ := w.boxes["Ann"].last(NoticeFriendStatus); !st.Online {
		t.Errorf("status %+v", st)
	}
	// The registry keeps Bob's list over the login.
	if !slices.Equal(w.r.FriendNames(w.ps["Bob"]), []string{"Ann"}) {
		t.Errorf("Bob's list %v", w.r.FriendNames(w.ps["Bob"]))
	}
	// Removing takes both sides away, also with Bob offline.
	w.r.Offline(w.ps["Bob"], w.boxes["Bob"])
	if o := w.r.FriendRemove(w.ps["Ann"], "Bob"); o != FriendRemoved {
		t.Errorf("remove: %v", o)
	}
	if w.r.IsFriend(w.ps["Bob"], "Ann") || w.r.IsFriend(w.ps["Ann"], "Bob") {
		t.Error("still friends")
	}
	// A refused request tells the requester.
	w.r.FriendAdd(w.ps["Ann"], "Cid")
	w.r.FriendReply(w.ps["Cid"], "Ann", false)
	if res, _ := w.boxes["Ann"].last(NoticeFriendResult); res.FriendOutcome != FriendRefused {
		t.Errorf("result %+v", res)
	}
}

func TestOverFullListsKeepEntries(t *testing.T) {
	limits := rules.DefaultLimits()
	limits.Friends = 1
	w := New(limits, rules.DefaultParty())
	b := &box{}
	p := w.Online(Login{Name: "Ann", Inbox: b, Friends: []Friend{{Name: "Bob"}, {Name: "Cid"}}})
	if !slices.Equal(w.FriendNames(p), []string{"Bob", "Cid"}) {
		t.Errorf("list %v", w.FriendNames(p))
	}
}

func TestBooking(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob", "Cid")
	w.r.BookingRegister(w.ps["Ann"], Ad{Level: 50, Map: "map/prt_fild08", Jobs: []int{4, 5}})
	w.r.BookingRegister(w.ps["Bob"], Ad{Level: 30, Jobs: []int{7}})
	if w.r.BookingRegister(w.ps["Ann"], Ad{Level: 1}) {
		t.Error("a second ad")
	}
	names := func(v BookingView) []string {
		var out []string
		for _, e := range v.Entries {
			out = append(out, e.Name)
		}
		return out
	}
	if got := names(w.r.BookingSearch(w.ps["Cid"], 0, "", 0)); !slices.Equal(got, []string{"Ann", "Bob"}) {
		t.Errorf("all: %v", got)
	}
	if got := names(w.r.BookingSearch(w.ps["Cid"], 55, "", 0)); !slices.Equal(got, []string{"Ann"}) {
		t.Errorf("level 55: %v", got)
	}
	if got := names(w.r.BookingSearch(w.ps["Cid"], 0, "", 7)); !slices.Equal(got, []string{"Bob"}) {
		t.Errorf("job 7: %v", got)
	}
	if got := names(w.r.BookingSearch(w.ps["Cid"], 0, "map/prt_fild08", 4)); len(got) != 0 {
		t.Errorf("map and job: %v", got)
	}
	w.r.Offline(w.ps["Ann"], w.boxes["Ann"])
	if got := names(w.r.BookingSearch(w.ps["Cid"], 0, "", 0)); !slices.Equal(got, []string{"Bob"}) {
		t.Errorf("after logout: %v", got)
	}
}

func TestLoad(t *testing.T) {
	r := New(rules.DefaultLimits(), rules.DefaultParty())
	r.Load([]PartyRecord{{ID: 7, Name: "Old", Leader: "Bob", Options: Options{ItemEven: true}, Members: []MemberRecord{
		{Slot: 0, Name: "Ann", CharID: 1}, {Slot: 2, Name: "Bob", CharID: 2},
	}}, {ID: 9, Name: "Empty"}})
	b := &box{}
	p := r.Online(Login{Name: "Bob", CharID: 2, Inbox: b})
	n, _ := b.last(NoticeParty)
	if n.Party == nil || n.Party.ID != 7 || !n.Party.Members[1].Leader || n.Party.Members[0].Online {
		t.Errorf("party %+v", n.Party)
	}
	if id, _ := r.Create(p, "x", Options{}); id != 0 {
		t.Error("Bob made a second party")
	}
	q := r.Online(Login{Name: "Cid", CharID: 3, Inbox: &box{}})
	if id, _ := r.Create(q, "New", Options{}); id != 10 {
		t.Errorf("new ID %d, want 10", id)
	}
	if _, ok := r.Find("Empty"); ok {
		t.Error("an empty party loaded")
	}
}

func TestFriendAutoAddOff(t *testing.T) {
	party := rules.DefaultParty()
	party.FriendAutoAdd = false
	w := newWorld(t, rules.DefaultLimits(), party, "Ann", "Bob")
	w.r.FriendAdd(w.ps["Ann"], "Bob")
	w.r.FriendReply(w.ps["Bob"], "Ann", true)
	if !w.r.IsFriend(w.ps["Ann"], "Bob") || w.r.IsFriend(w.ps["Bob"], "Ann") {
		t.Error("friend_auto_add off still added both")
	}
}
