package e2e

import (
	"context"
	"slices"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// TestPartyInviteAndAccept: the limits come with Welcome. A leader
// makes a party and invites a player, who accepts. Both see the
// members, and the leader sees the member walk. Invites that cannot
// work say why.
func TestPartyInviteAndAccept(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	if l := alice.WorldLimits(); l.PartyMembers != 12 || l.Friends != 40 || l.BlockList != 20 {
		t.Errorf("limits %+v", l)
	}

	info := alice.CreateParty("Crew", false, false)
	if info.GetId() == 0 || len(info.GetMembers()) != 1 || !info.GetMembers()[0].GetLeader() || info.GetOptions().GetExpEven() {
		t.Fatalf("new party %v", info)
	}
	if r := alice.Invite("Bob"); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_INVITE_SENT || r.GetName() != "Bob" {
		t.Fatalf("invite %v", r)
	}
	inv := bob.Invited("Alice")
	if inv.GetPartyId() != info.GetId() || inv.GetPartyName() != "Crew" {
		t.Errorf("invite %v", inv)
	}
	joined := bob.Accept(inv)
	if r := alice.PartyResult(); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_ACCEPTED || r.GetName() != "Bob" {
		t.Errorf("the leader got %v", r)
	}
	seen := alice.PartyInfo(func(i *wathenav1.PartyInfo) bool { return len(i.GetMembers()) == 2 })
	for who, got := range map[string]*wathenav1.PartyInfo{"Alice": seen, "Bob": joined} {
		var names []string
		for _, m := range got.GetMembers() {
			names = append(names, m.GetName())
			if !m.GetOnline() || m.GetMap() != StartMap || m.GetJob() != 1 || m.GetBaseLevel() != 50 || m.GetHp() <= 0 || m.GetMaxHp() < m.GetHp() || m.GetX() == 0 {
				t.Errorf("%s sees member %v", who, m)
			}
			if m.GetLeader() != (m.GetName() == "Alice") {
				t.Errorf("%s sees member %v", who, m)
			}
		}
		if !slices.Equal(names, []string{"Alice", "Bob"}) {
			t.Errorf("%s sees members %v", who, names)
		}
	}

	// The leader sees Bob's cell within a second of his walk.
	bob.WalkTo(14, 10)
	alice.Until("Bob on 14,10 in the party window", func(_ View, g Game) bool {
		m, ok := g.Party.Member("Bob")
		return ok && m.X == 14 && m.Y == 10
	})

	for _, c := range []struct {
		who  *Player
		name string
		want wathenav1.PartyOutcome
	}{
		{alice, "Nobody", wathenav1.PartyOutcome_PARTY_OUTCOME_NOT_ONLINE},
		{alice, "Bob", wathenav1.PartyOutcome_PARTY_OUTCOME_TARGET_IN_PARTY},
		{bob, "Carol", wathenav1.PartyOutcome_PARTY_OUTCOME_NOT_LEADER},
	} {
		if r := c.who.Invite(c.name); r.GetOutcome() != c.want {
			t.Errorf("%s invites %s: %v, want %s", c.who.Name, c.name, r, c.want)
		}
	}
	if carol.Seen((*wathenav1.Event).HasPartyInvite) != nil {
		t.Error("Carol got an invite from a member who is not the leader")
	}
	srv.CheckLogs()
}

// TestPartyInviteBlocked: a player who blocks the leader refuses its
// invite at once and sees nothing.
func TestPartyInviteBlocked(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	bob.BlockName("Alice", false)
	alice.CreateParty("Crew", false, false)
	if r := alice.Invite("Bob"); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_REFUSED || r.GetName() != "Bob" {
		t.Fatalf("invite %v", r)
	}
	// A whisper from Bob comes after the invite was refused. Bob has
	// seen nothing of the invite by then.
	bob.Whisper("Alice", "no thanks")
	alice.HearsWhisper("Bob", "no thanks")
	if e := bob.Seen((*wathenav1.Event).HasPartyInvite); e != nil {
		t.Errorf("Bob saw %v", e)
	}
	if bob.Game().Party != nil {
		t.Errorf("Bob is in %+v", bob.Game().Party)
	}
	srv.CheckLogs()
}

// TestPartyChat: a party line reaches a member on another map, and not
// a player outside the party on the speaker's map.
func TestPartyChat(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	carol.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Alice")
	MakeParty("Crew", false, false, alice, bob)
	bob.Talk("Guide").Pick("Go to the field")
	bob.MapChange(FieldMap)
	alice.Until("Bob on the field in the party window", func(_ View, g Game) bool {
		m, ok := g.Party.Member("Bob")
		return ok && m.Map == FieldMap
	})

	line := alice.SayToParty("meet me at the field")
	if len(line.GetLineId()) != 16 {
		t.Errorf("line %v", line)
	}
	bob.HearsParty("Alice", "meet me at the field")
	bob.SayToParty("on my way")
	alice.HearsParty("Bob", "on my way")
	// Carol hears Alice's next local line, and no party line before it.
	alice.Say("hello Carol")
	carol.Hears("Alice", "hello Carol")
	if e := carol.Seen(func(e *wathenav1.Event) bool {
		return e.GetChat().GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_PARTY
	}); e != nil {
		t.Errorf("Carol heard %v", e)
	}
	// Carol has no party to talk to.
	carol.send("party chat", func(ctx context.Context) error { return carol.PartyChat(ctx, "anyone?") })
	if r := carol.Expect("a refused line", (*wathenav1.Event).HasChatRefused).GetChatRefused(); r.GetReason() != wathenav1.ChatRefusal_CHAT_REFUSAL_NOT_IN_CHANNEL {
		t.Errorf("refused %v", r)
	}
	srv.CheckLogs()
}

// TestPartyEvenExpShare: a solo kill gives the killer the Dummy's exp.
// With even exp share, a kill gives each member on the map half, as
// both are at base level 50, the Dummy's level, so renewal's level
// penalty is 100%.
func TestPartyEvenExpShare(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	baseExp, jobExp := wathenav1.StatId_STAT_ID_BASE_EXP, wathenav1.StatId_STAT_ID_JOB_EXP

	gain := func(p *Player, before map[wathenav1.StatId]int64, id wathenav1.StatId, want int64) {
		t.Helper()
		p.Until("the exp of the kill", func(_ View, g Game) bool { return g.Stats[id] >= before[id]+want })
		if got := p.Stat(id) - before[id]; got != want {
			t.Errorf("%s gained %d of stat %s, want %d", p.Name, got, id, want)
		}
	}
	stats := func(p *Player) map[wathenav1.StatId]int64 {
		return map[wathenav1.StatId]int64{baseExp: p.Stat(baseExp), jobExp: p.Stat(jobExp)}
	}

	// Alone, Alice gets the Dummy's 10 base and 10 job exp.
	a := stats(alice)
	alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
	gain(alice, a, baseExp, 10)
	gain(alice, a, jobExp, 10)

	MakeParty("Crew", false, false, alice, bob)
	if info := alice.SetPartyOptions(true, false, false); !info.GetOptions().GetExpEven() {
		t.Fatalf("options %v", info.GetOptions())
	}
	a, b := stats(alice), stats(bob)
	alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
	for _, c := range []struct {
		p      *Player
		before map[wathenav1.StatId]int64
	}{{alice, a}, {bob, b}} {
		gain(c.p, c.before, baseExp, 5)
		gain(c.p, c.before, jobExp, 5)
	}
	srv.CheckLogs()
}

// TestPartyItemPickUp: with shared pick-up, a member who did not fight
// takes a fresh drop at once; without it, the server refuses. With even
// item share, each pick-up goes to a member on the map, the party sees
// who got the charm, and no item is lost or made.
func TestPartyItemPickUp(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	MakeParty("Crew", false, true, alice, bob)

	alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
	bob.PickUp(bob.FindItem(Jellopy))
	if bob.Count(Jellopy) != 21 {
		t.Errorf("Bob holds %d Jellopy", bob.Count(Jellopy))
	}
	alice.PickUp(alice.FindItem(Charm))

	alice.SetPartyOptions(false, false, false)
	alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
	if r := bob.PickUpRefused(bob.FindItem(Jellopy)); r != wathenav1.PickUpFailure_PICK_UP_FAILURE_NOT_YOURS {
		t.Errorf("Bob's pick-up failed with %s", r)
	}
	alice.PickUp(alice.FindItem(Jellopy))
	alice.PickUp(alice.FindItem(Charm))

	// shared lists the names in the PartyItemShared notices a player
	// got from an event on. As clif_party_show_picker, a notice goes to
	// the members on the map other than the one who got the item, and
	// only for armor, weapons and cards: charms, not Jellopy.
	shared := func(p *Player, from int) []string {
		events, _ := p.Events(from)
		var out []string
		for _, e := range events {
			if s := e.GetPartyItemShared(); s != nil {
				if s.GetItemId() != Charm || s.GetAmount() != 1 {
					t.Errorf("%s saw %v", p.Name, s)
				}
				out = append(out, s.GetName())
			}
		}
		return out
	}
	// Alice took both charms so far, and Bob saw it each time.
	bob.Until("two charms shown", func(View, Game) bool { return len(shared(bob, 0)) == 2 })
	if got := shared(bob, 0); !slices.Equal(got, []string{"Alice", "Alice"}) || len(shared(alice, 0)) != 0 {
		t.Errorf("before even item share, Bob saw %v and Alice %v", got, shared(alice, 0))
	}

	alice.SetPartyOptions(false, true, true)
	bob.PartyInfo(func(i *wathenav1.PartyInfo) bool { return i.GetOptions().GetItemEven() })
	held := func(id uint32) int { return alice.Count(id) + bob.Count(id) }
	jellopy, aliceCharms, bobCharms := held(Jellopy), alice.Count(Charm), bob.Count(Charm)
	markA, markB := alice.Mark(), bob.Mark()
	const kills = 3
	for range kills {
		alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
		// Bob did not fight, so shared pick-up lets him take it.
		bob.TakeShared(bob.FindItem(Charm))
		alice.TakeShared(alice.FindItem(Jellopy))
	}
	deadline := time.Now().Add(DefaultTimeout)
	for held(Jellopy) != jellopy+kills || alice.Count(Charm)+bob.Count(Charm) != aliceCharms+bobCharms+kills {
		if time.Now().After(deadline) {
			t.Fatalf("Alice and Bob hold %d Jellopy and %d charms, want %d and %d (Alice %v, Bob %v)",
				held(Jellopy), held(Charm), jellopy+kills, aliceCharms+bobCharms+kills, alice.Game().Items, bob.Game().Items)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Each charm that went to Bob shows to Alice, and each that went to
	// Alice shows to Bob.
	toBob, toAlice := bob.Count(Charm)-bobCharms, alice.Count(Charm)-aliceCharms
	alice.Until("the charms Bob got shown", func(View, Game) bool { return len(shared(alice, markA)) == toBob })
	bob.Until("the charms Alice got shown", func(View, Game) bool { return len(shared(bob, markB)) == toAlice })
	seenA, seenB := shared(alice, markA), shared(bob, markB)
	if slices.ContainsFunc(seenA, func(n string) bool { return n != "Bob" }) || slices.ContainsFunc(seenB, func(n string) bool { return n != "Alice" }) {
		t.Errorf("Alice saw %v and Bob %v", seenA, seenB)
	}
	t.Logf("charms: %d to Alice, %d to Bob; Jellopy: Alice %d, Bob %d", toAlice, toBob, alice.Count(Jellopy), bob.Count(Jellopy))

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	zones, err := srv.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	totals, twice := TotalsOf(zones)
	if len(twice) > 0 || totals.Items[Jellopy] != held(Jellopy) || totals.Items[Charm] != held(Charm) {
		t.Errorf("the server holds %v and %d on the ground, serials held twice %v; the players see %d Jellopy and %d charms",
			totals.Items, totals.Ground, twice, held(Jellopy), held(Charm))
	}
	srv.CheckLogs()
}

// TestPartyLeaderLeaves: when the leader leaves, the party breaks up
// and every member learns it.
func TestPartyLeaderLeaves(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	MakeParty("Crew", false, false, alice, bob, carol)
	for _, p := range []*Player{alice, bob, carol} {
		p.Skip()
	}
	alice.send("leave", func(ctx context.Context) error { return alice.PartyLeave(ctx) })
	if r := alice.PartyResult(); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_LEFT {
		t.Errorf("Alice got %v", r)
	}
	alice.PartyInfo(func(i *wathenav1.PartyInfo) bool { return i.GetId() == 0 })
	for _, p := range []*Player{bob, carol} {
		if r := p.PartyResult(); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_DISBANDED {
			t.Errorf("%s got %v", p.Name, r)
		}
		p.PartyInfo(func(i *wathenav1.PartyInfo) bool { return i.GetId() == 0 })
		if p.Game().Party != nil {
			t.Errorf("%s still has %+v", p.Name, p.Game().Party)
		}
	}
	// The name is free again.
	bob.CreateParty("Crew", false, false)
	srv.CheckLogs()
}

// TestFriends: a request that is accepted puts each player on the
// other's list. A friend who logs out and in again shows on the list.
func TestFriends(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	if r := alice.AddFriend("Bob"); r.GetOutcome() != wathenav1.FriendOutcome_FRIEND_OUTCOME_REQUEST_SENT {
		t.Fatalf("add %v", r)
	}
	if r := bob.AcceptFriend("Alice"); r.GetOutcome() != wathenav1.FriendOutcome_FRIEND_OUTCOME_ADDED || r.GetName() != "Alice" {
		t.Errorf("Bob got %v", r)
	}
	if r := alice.FriendResult(); r.GetOutcome() != wathenav1.FriendOutcome_FRIEND_OUTCOME_ADDED || r.GetName() != "Bob" {
		t.Errorf("Alice got %v", r)
	}
	alice.FriendOnline("Bob", true)
	bob.FriendOnline("Alice", true)
	if r := alice.AddFriend("Bob"); r.GetOutcome() != wathenav1.FriendOutcome_FRIEND_OUTCOME_ALREADY_FRIENDS {
		t.Errorf("adding again %v", r)
	}

	if err := bob.Close(); err != nil {
		t.Logf("closing Bob: %v", err)
	}
	alice.FriendStatus("Bob", false)
	alice.FriendOnline("Bob", false)
	bob = srv.Join(t, "Bob")
	alice.FriendStatus("Bob", true)
	alice.FriendOnline("Bob", true)
	bob.FriendOnline("Alice", true)
	srv.CheckLogs()
}

// TestPartySkill: Angelus cast by an Acolyte in a party reaches a member
// near it, and not a player near it outside the party.
func TestPartySkill(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartJob: "4:50:40"})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	if job := alice.Stat(wathenav1.StatId_STAT_ID_JOB); job != JobAcolyte {
		t.Fatalf("Alice's job is %d", job)
	}
	MakeParty("Crew", false, false, alice, bob)
	for range 3 {
		alice.Learn(SkillDivineProtection)
	}
	alice.Learn(SkillAngelus)
	alice.Skip()
	bob.Skip()
	carol.Skip()

	// The icon may come before or after the caster's own effect.
	mark := alice.Mark()
	if fx := alice.CastOnSelf(SkillAngelus, 1); !fx.GetOk() {
		t.Fatalf("Angelus: %v", fx)
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	e, _, err := alice.Wait(ctx, mark, func(e *wathenav1.Event) bool { return e.GetStatusIcon().GetOn() })
	if err != nil {
		t.Fatalf("Alice's Angelus icon: %v", err)
	}
	icon := e.GetStatusIcon().GetIcon()
	bob.Expect("Angelus's icon", func(e *wathenav1.Event) bool {
		s := e.GetStatusIcon()
		return s.GetOn() && s.GetIcon() == icon
	})
	// Carol hears Alice's next line, and got no icon before it.
	alice.Say("blessed")
	carol.Hears("Alice", "blessed")
	if e := carol.Seen((*wathenav1.Event).HasStatusIcon); e != nil {
		t.Errorf("Carol got %v", e)
	}
	srv.CheckLogs()
}

// TestPartyAndFriendsInTheDatabase: a party and a friendship are saved,
// and come back after the server restarts.
func TestPartyAndFriendsInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	id := MakeParty("Crew", true, false, alice, bob)
	alice.AddFriend("Bob")
	bob.AcceptFriend("Alice")
	alice.FriendOnline("Bob", true)
	_ = alice.Close()
	_ = bob.Close()

	srv.Restart()
	alice = srv.Join(t, "Alice")
	info := alice.PartyInfo(func(i *wathenav1.PartyInfo) bool { return i.GetId() != 0 })
	var names []string
	for _, m := range info.GetMembers() {
		names = append(names, m.GetName())
	}
	if info.GetId() != id || info.GetName() != "Crew" || !info.GetOptions().GetItemEven() || !slices.Equal(names, []string{"Alice", "Bob"}) {
		t.Errorf("after the restart: %v", info)
	} else if m := info.GetMembers(); !m[0].GetLeader() || !m[0].GetOnline() || m[1].GetOnline() {
		t.Errorf("after the restart: members %v", m)
	}
	alice.FriendOnline("Bob", false)
	bob = srv.Join(t, "Bob")
	alice.FriendStatus("Bob", true)
	bob.FriendOnline("Alice", true)
	bob.Until("the party", func(_ View, g Game) bool { return g.Party != nil && g.Party.ID == id && len(g.Party.Members) == 2 })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var parties, members, friends int
	for q, n := range map[string]*int{
		`SELECT count(*) FROM parties WHERE name = 'Crew' AND item_even`: &parties,
		`SELECT count(*) FROM party_members`:                             &members,
		`SELECT count(*) FROM character_friends`:                         &friends,
	} {
		if err := srv.DB.QueryRow(ctx, q).Scan(n); err != nil {
			t.Fatal(err)
		}
	}
	if parties != 1 || members != 2 || friends != 2 {
		t.Errorf("%d parties, %d members, %d friend rows; want 1, 2 and 2", parties, members, friends)
	}
	srv.CheckLogs()
}
