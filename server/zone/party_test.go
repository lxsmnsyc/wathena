package zone

import (
	"slices"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
)

func partyInfos(p *Player) []*wathenav1.PartyInfo {
	return eventsOf(p, (*wathenav1.Event).GetPartyInfo, (*wathenav1.Event).HasPartyInfo)
}

func partyOutcomes(evs []*wathenav1.Event) []wathenav1.PartyOutcome {
	var out []wathenav1.PartyOutcome
	for _, e := range evs {
		if e.HasPartyResult() {
			out = append(out, e.GetPartyResult().GetOutcome())
		}
	}
	return out
}

func TestPartyAcrossZones(t *testing.T) {
	w := newSocialWorld(nil, "map/a", "map/b")
	alice := w.join(w.zones[0], "Alice")
	bob := w.join(w.zones[1], "Bob")
	carol := w.join(w.zones[0], "Carol")
	w.step(50)
	for _, p := range []*Player{alice, bob, carol} {
		events(p)
	}
	w.zones[0].handle(command{player: alice, seq: 1, party: &partyCommand{create: &partyCreate{name: "Wanderers"}}})
	w.zones[0].handle(command{player: alice, seq: 2, party: &partyCommand{invite: "bob"}})
	w.step(100)
	inv := eventsOf(bob, (*wathenav1.Event).GetPartyInvite, (*wathenav1.Event).HasPartyInvite)
	if len(inv) != 1 || inv[0].GetFrom() != "Alice" || inv[0].GetPartyName() != "Wanderers" {
		t.Fatalf("Bob's invites %v", inv)
	}
	w.zones[1].handle(command{player: bob, seq: 1, party: &partyCommand{reply: &partyReply{party: int64(inv[0].GetPartyId()), accept: true}}})
	w.step(150)
	ev := events(alice)
	if got := partyOutcomes(ev); !slices.Contains(got, wathenav1.PartyOutcome_PARTY_OUTCOME_ACCEPTED) {
		t.Errorf("Alice's outcomes %v", got)
	}
	var last *wathenav1.PartyInfo
	for _, e := range ev {
		if e.HasPartyInfo() {
			last = e.GetPartyInfo()
		}
	}
	if last == nil || len(last.GetMembers()) != 2 || last.GetMembers()[1].GetName() != "Bob" || last.GetMembers()[1].GetMap() != "map/b" ||
		!last.GetMembers()[0].GetLeader() || !last.GetMembers()[1].GetOnline() {
		t.Fatalf("Alice's party %v", last)
	}

	// Party chat reaches Bob on the other map, and not Carol.
	events(bob)
	w.zones[0].handle(command{player: alice, seq: 3, chat: &chatCommand{kind: chatParty, text: "to the dungeon"}})
	w.step(200)
	got := chats(bob)
	if len(got) != 1 || got[0].GetChannel() != wathenav1.ChatChannel_CHAT_CHANNEL_PARTY || got[0].GetFromName() != "Alice" {
		t.Fatalf("Bob hears %v", got)
	}
	if got := chats(carol); len(got) != 0 {
		t.Errorf("Carol hears %v", got)
	}
	if lines := w.log.Lines(); len(lines) != 1 || lines[0].Kind != "party" || lines[0].To != "Wanderers" {
		t.Errorf("log %+v", lines)
	}
	// Carol has no party: her party line is refused.
	w.zones[0].handle(command{player: carol, seq: 1, chat: &chatCommand{kind: chatParty, text: "hello?"}})
	w.step(250)
	if r := eventsOf(carol, (*wathenav1.Event).GetChatRefused, (*wathenav1.Event).HasChatRefused); len(r) != 1 {
		t.Errorf("Carol's refusals %v", r)
	}

	// Carol blocks Alice: Alice's invite is refused, and Carol sees none.
	w.zones[0].handle(command{player: carol, seq: 2, block: &blockCommand{name: "Alice"}})
	w.step(300)
	events(carol)
	w.zones[0].handle(command{player: alice, seq: 4, party: &partyCommand{invite: "Carol"}})
	w.step(350)
	if got := partyOutcomes(events(alice)); !slices.Equal(got, []wathenav1.PartyOutcome{wathenav1.PartyOutcome_PARTY_OUTCOME_REFUSED}) {
		t.Errorf("Alice's outcomes %v", got)
	}
	if inv := eventsOf(carol, (*wathenav1.Event).GetPartyInvite, (*wathenav1.Event).HasPartyInvite); len(inv) != 0 {
		t.Errorf("Carol saw %v", inv)
	}

	// The leader leaves: the party breaks up, and Bob has none.
	events(bob)
	w.zones[0].handle(command{player: alice, seq: 5, party: &partyCommand{leave: true}})
	w.step(400)
	infos := partyInfos(bob)
	if len(infos) == 0 || infos[len(infos)-1].GetId() != 0 {
		t.Fatalf("Bob's party %v", infos)
	}
	if bob.partyID() != 0 || alice.partyID() != 0 {
		t.Error("a party is left")
	}
}

func TestPartyHPOnTheSameMap(t *testing.T) {
	w := newSocialWorld(nil, "map/a")
	alice := w.join(w.zones[0], "Alice")
	bob := w.join(w.zones[0], "Bob")
	formParty(t, w.zones[0], social.Options{}, alice, bob)
	w.step(50)
	events(alice)
	bob.entity.hp = 7
	w.step(100)
	var hp []int64
	for _, u := range eventsOf(alice, (*wathenav1.Event).GetPartyMember, (*wathenav1.Event).HasPartyMember) {
		if u.GetMember().GetName() == "Bob" {
			hp = append(hp, u.GetMember().GetHp())
		}
	}
	if !slices.Contains(hp, 7) {
		t.Errorf("Alice sees Bob's HP %v", hp)
	}
}

func TestFriendsAcrossZones(t *testing.T) {
	w := newSocialWorld(nil, "map/a", "map/b")
	alice := w.join(w.zones[0], "Alice")
	bob := w.join(w.zones[1], "Bob")
	w.step(50)
	events(alice)
	events(bob)
	w.zones[0].handle(command{player: alice, seq: 1, friend: &friendCommand{add: "Bob"}})
	w.step(100)
	req := eventsOf(bob, (*wathenav1.Event).GetFriendRequest, (*wathenav1.Event).HasFriendRequest)
	if len(req) != 1 || req[0].GetFrom() != "Alice" {
		t.Fatalf("Bob's requests %v", req)
	}
	w.zones[1].handle(command{player: bob, seq: 1, friend: &friendCommand{reply: &friendReply{name: "Alice", accept: true}}})
	w.step(150)
	lists := eventsOf(alice, (*wathenav1.Event).GetFriendList, (*wathenav1.Event).HasFriendList)
	if len(lists) == 0 {
		t.Fatal("no friend list")
	}
	l := lists[len(lists)-1].GetFriends()
	if len(l) != 1 || l[0].GetName() != "Bob" || !l[0].GetOnline() {
		t.Fatalf("Alice's friends %v", l)
	}
	// Bob logs out: Alice sees it.
	w.zones[1].leave(bob, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	w.step(200)
	st := eventsOf(alice, (*wathenav1.Event).GetFriendStatus, (*wathenav1.Event).HasFriendStatus)
	if len(st) != 1 || st[0].GetName() != "Bob" || st[0].GetOnline() {
		t.Errorf("Alice sees %v", st)
	}
}

// TestEvenShareOnAKill kills a monster with one member of an even share
// party: both members on the map get half, as party_exp_share.
func TestEvenShareOnAKill(t *testing.T) {
	z, p, m := lootZone(t, &progression.Mob{ID: 1002, Level: 1, MaxHP: 30, BaseExp: 9, JobExp: 4})
	b := add(z, "b")
	b.entity.x, b.entity.y = m.x-3, m.y
	c := add(z, "c")
	formParty(t, z, social.Options{ExpEven: true}, p, b)
	z.stepAt(50)
	kill(t, z, p, m)
	for _, x := range []*Player{p, b} {
		if x.progress.BaseExp != 4 || x.progress.JobExp != 2 {
			t.Errorf("%s has %d/%d exp, want 4/2", x.Name, x.progress.BaseExp, x.progress.JobExp)
		}
	}
	if c.progress.BaseExp != 0 {
		t.Errorf("c got %d exp", c.progress.BaseExp)
	}
}

// TestPartyPickUpRules: with shared pick-up, a member takes what its
// party's first looter may take at once, and with even item share the
// item goes to a member at random.
func TestPartyPickUpRules(t *testing.T) {
	z, p, b, it := dropOne(t)
	formParty(t, z, social.Options{PickupShared: true}, p, b)
	z.handle(command{player: b, seq: 1, pickUp: new(b.netIDs.id(it))})
	for start, now := z.now, z.now+50; now <= start+1000 && z.entities.has(it); now += 50 {
		z.stepAt(now)
	}
	if z.entities.has(it) || b.inventory.Count(909) != 1 {
		t.Fatalf("b did not take it: %v", failures(b))
	}

	// Even item share: what p picks up goes to a member on the map.
	z2, p2, b2, it2 := dropOne(t)
	formParty(t, z2, social.Options{ItemEven: true}, p2, b2)
	z2.handle(command{player: p2, seq: 2, pickUp: new(p2.netIDs.id(it2))})
	for start, now := z2.now, z2.now+50; now <= start+1000 && z2.entities.has(it2); now += 50 {
		z2.stepAt(now)
	}
	if z2.entities.has(it2) || p2.inventory.Count(909)+b2.inventory.Count(909) != 1 {
		t.Fatalf("the item went nowhere: %d %d", p2.inventory.Count(909), b2.inventory.Count(909))
	}
}

// TestBlockListLimit applies the ruleset's block_list limit: a list at
// the limit refuses a name, and a list over a lowered limit keeps its
// names.
func TestBlockListLimit(t *testing.T) {
	w := newSocialWorld(nil, "map/a")
	z := w.zones[0]
	r := *rules.Of(true)
	r.Limits.BlockList = 2
	z.Mods = &Mods{Rules: &r}
	alice := w.join(z, "Alice")
	alice.blocks = []string{"A1", "A2", "A3"}
	w.step(50)
	events(alice)
	z.handle(command{player: alice, seq: 1, block: &blockCommand{name: "B"}})
	w.step(100)
	bl := eventsOf(alice, (*wathenav1.Event).GetBlockList, (*wathenav1.Event).HasBlockList)
	if len(bl) != 1 || !bl[0].GetFull() || len(bl[0].GetNames()) != 3 {
		t.Fatalf("block list %v", bl)
	}
	z.handle(command{player: alice, seq: 2, block: &blockCommand{name: "A1", unblock: true}})
	z.handle(command{player: alice, seq: 3, block: &blockCommand{name: "A3", unblock: true}})
	z.handle(command{player: alice, seq: 4, block: &blockCommand{name: "B"}})
	w.step(150)
	bl = eventsOf(alice, (*wathenav1.Event).GetBlockList, (*wathenav1.Event).HasBlockList)
	if got := bl[len(bl)-1]; got.GetFull() || !slices.Equal(got.GetNames(), []string{"A2", "B"}) {
		t.Errorf("block list %v", got)
	}
}
