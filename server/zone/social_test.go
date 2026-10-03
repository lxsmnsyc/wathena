package zone

import (
	"slices"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// socialWorld is zones that share a chat hub and a clock, as a world.
type socialWorld struct {
	hub *chat.Hub
	// social is the world's party and friend registry.
	social *social.Registry
	log    *chat.MemoryLog
	now    time.Time
	zones  []*Zone
}

func newSocialWorld(words []chat.Word, maps ...string) *socialWorld {
	w := &socialWorld{now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC), log: &chat.MemoryLog{}}
	w.hub = chat.NewHub(chat.Core().Channels, chat.NewFilter(words))
	w.hub.Log = w.log
	w.hub.Clock = func() time.Time { return w.now }
	w.social = social.New(rules.DefaultLimits(), rules.DefaultParty())
	w.social.Blocks = w.hub.BlocksName
	for _, m := range maps {
		z := New(m, open(60), rng.FromKey([32]byte{}).Stream(), nil)
		z.Chat, z.Clock, z.Social = w.hub, w.hub.Clock, w.social
		w.zones = append(w.zones, z)
	}
	return w
}

// join adds a player with a session, as a login does.
func (w *socialWorld) join(z *Zone, name string) *Player {
	c := z.NewCharacter(name, Look{})
	out := make(chan *wathenav1.ServerMessage, 1024)
	s := &Session{Out: out, out: out, left: make(chan struct{})}
	p := newPlayer(s, &c)
	s.zone, s.player = z, p
	z.join(p)
	return p
}

// step runs every zone at a time: deliveries first, as Run does.
func (w *socialWorld) step(at int64) {
	for _, z := range w.zones {
		z.drainChat()
		z.drainSocial()
		z.stepAt(at)
	}
}

func eventsOf[T any](p *Player, get func(*wathenav1.Event) T, has func(*wathenav1.Event) bool) []T {
	var out []T
	for _, e := range events(p) {
		if has(e) {
			out = append(out, get(e))
		}
	}
	return out
}

func TestWhispersAcrossZones(t *testing.T) {
	w := newSocialWorld(nil, "map/a", "map/b")
	alice := w.join(w.zones[0], "Alice")
	bob := w.join(w.zones[1], "Bob")
	w.step(50)
	events(alice)
	events(bob)

	w.zones[0].handle(command{player: alice, seq: 1, chat: &chatCommand{kind: chatWhisper, to: "bob", text: "hi"}})
	w.zones[0].handle(command{player: alice, seq: 2, chat: &chatCommand{kind: chatWhisper, to: "Nobody", text: "hey"}})
	w.step(100)
	results := eventsOf(alice, (*wathenav1.Event).GetWhisperResult, (*wathenav1.Event).HasWhisperResult)
	if len(results) != 2 || results[0].GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_SENT || results[0].GetTo() != "Bob" ||
		results[0].GetText() != "hi" || len(results[0].GetLineId()) != 16 ||
		results[1].GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_NOT_ONLINE {
		t.Fatalf("Alice's results %v", results)
	}
	got := chats(bob)
	if len(got) != 1 || got[0].GetChannel() != wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER || got[0].GetFromName() != "Alice" || got[0].GetText() != "hi" {
		t.Fatalf("Bob hears %v", got)
	}
	if lines := w.log.Lines(); len(lines) != 1 || lines[0].Kind != chat.LineWhisper || lines[0].To != "Bob" || lines[0].Map != "map/a" {
		t.Errorf("log %+v", lines)
	}

	// Bob blocks Alice: her whispers stop, and her local chat is hidden.
	carol := w.join(w.zones[1], "Carol")
	w.zones[1].handle(command{player: bob, seq: 1, block: &blockCommand{name: "ALICE"}})
	w.step(150)
	if bl := eventsOf(bob, (*wathenav1.Event).GetBlockList, (*wathenav1.Event).HasBlockList); len(bl) != 1 || !slices.Equal(bl[0].GetNames(), []string{"ALICE"}) {
		t.Fatalf("block list %v", bl)
	}
	w.zones[0].handle(command{player: alice, seq: 3, chat: &chatCommand{kind: chatWhisper, to: "Bob", text: "again"}})
	w.step(200)
	if r := eventsOf(alice, (*wathenav1.Event).GetWhisperResult, (*wathenav1.Event).HasWhisperResult); len(r) != 1 || r[0].GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_BLOCKED {
		t.Fatalf("blocked whisper %v", r)
	}
	if got := chats(bob); len(got) != 0 {
		t.Errorf("Bob hears %v", got)
	}
	events(carol)
	// Alice comes to Bob's map in the test by joining there as Dave, who
	// Bob does not block, and Alice2, whose name Bob blocks.
	dave := w.join(w.zones[1], "Dave")
	alice2 := w.join(w.zones[1], "alice")
	w.step(250)
	for _, p := range []*Player{bob, dave, alice2} {
		events(p)
	}
	w.zones[1].handle(command{player: alice2, seq: 1, chat: &chatCommand{text: "can you hear me"}})
	w.zones[1].handle(command{player: dave, seq: 1, chat: &chatCommand{text: "I can"}})
	w.step(300)
	var heard []string
	for _, c := range chats(bob) {
		heard = append(heard, c.GetFromName()+": "+c.GetText())
	}
	if !slices.Equal(heard, []string{"Dave: I can"}) {
		t.Errorf("Bob hears %v", heard)
	}
	if got := chats(carol); len(got) != 2 {
		t.Errorf("Carol hears %v", got)
	}
	if !bob.blocking("Alice") || bob.blocking("Dave") {
		t.Error("blocking")
	}
	if c := w.zones[1].snapshot(bob); !slices.Equal(c.Blocks, []string{"ALICE"}) {
		t.Errorf("saved blocks %v", c.Blocks)
	}
}

func TestChannelsAndFilter(t *testing.T) {
	w := newSocialWorld([]chat.Word{{Word: "darn", Action: chat.Replace}, {Word: "spamlink", Action: chat.Reject}}, "map/a", "map/b")
	alice := w.join(w.zones[0], "Alice")
	bob := w.join(w.zones[1], "Bob")
	w.step(50)
	events(alice)
	events(bob)
	for i, p := range []*Player{alice, bob} {
		p.sess.zone.handle(command{player: p, seq: 1, channel: &channelCommand{wathenav1.ChannelAction_CHANNEL_ACTION_JOIN, "#trade"}})
		_ = i
	}
	w.step(100)
	if j := eventsOf(bob, (*wathenav1.Event).GetChannelJoined, (*wathenav1.Event).HasChannelJoined); len(j) != 1 || j[0].GetName() != "#trade" || j[0].GetDisplayName() != "[Trade]" {
		t.Fatalf("joined %v", j)
	}
	events(alice)
	w.zones[0].handle(command{player: alice, seq: 2, chat: &chatCommand{kind: chatChannel, to: "#trade", text: "darn cheap potions"}})
	w.step(150)
	w.step(200)
	got := chats(bob)
	if len(got) != 1 || got[0].GetChannel() != wathenav1.ChatChannel_CHAT_CHANNEL_NAMED || got[0].GetChannelName() != "#trade" ||
		got[0].GetText() != "**** cheap potions" || got[0].GetFromName() != "Alice" || got[0].GetColor() != 0xB6FF00 {
		t.Fatalf("Bob hears %v", got)
	}
	if mine := chats(alice); len(mine) != 1 {
		t.Errorf("Alice hears %v", mine)
	}
	// A rejected word stops the line, and the sender learns why.
	w.zones[0].handle(command{player: alice, seq: 3, chat: &chatCommand{text: "go to spamlink"}})
	w.step(250)
	if r := eventsOf(alice, (*wathenav1.Event).GetChatRefused, (*wathenav1.Event).HasChatRefused); len(r) != 1 || r[0].GetReason() != wathenav1.ChatRefusal_CHAT_REFUSAL_FILTERED {
		t.Errorf("refused %v", r)
	}
	// A whisper to #trade speaks there; to #support it joins first.
	w.zones[0].handle(command{player: alice, seq: 4, chat: &chatCommand{kind: chatWhisper, to: "#support", text: "help"}})
	w.step(300)
	ev := events(alice)
	if !slices.ContainsFunc(ev, func(e *wathenav1.Event) bool { return e.GetChannelJoined().GetName() == "#support" }) ||
		!slices.ContainsFunc(ev, func(e *wathenav1.Event) bool { return e.GetChat().GetChannelName() == "#support" }) {
		t.Errorf("whisper to #support: %v", ev)
	}
	w.zones[1].handle(command{player: bob, seq: 2, channel: &channelCommand{wathenav1.ChannelAction_CHANNEL_ACTION_MEMBERS, "trade"}})
	w.step(350)
	if m := eventsOf(bob, (*wathenav1.Event).GetChannelMembers, (*wathenav1.Event).HasChannelMembers); len(m) != 1 || !slices.Equal(m[0].GetNames(), []string{"Alice", "Bob"}) {
		t.Errorf("members %v", m)
	}
	// Leaving: the player leaves the channel, and logging out leaves
	// every channel.
	w.zones[1].handle(command{player: bob, seq: 3, channel: &channelCommand{wathenav1.ChannelAction_CHANNEL_ACTION_LEAVE, "#trade"}})
	w.step(400)
	if l := eventsOf(bob, (*wathenav1.Event).GetChannelLeft, (*wathenav1.Event).HasChannelLeft); len(l) != 1 {
		t.Errorf("left %v", l)
	}
	w.zones[0].leave(alice, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	if c := w.hub.Channel("trade"); c == nil || len(w.hub.ChannelMembers(c)) != 0 {
		t.Errorf("members after logout: %v", w.hub.ChannelMembers(c))
	}
}

func TestMuteAndReport(t *testing.T) {
	w := newSocialWorld(nil, "map/a")
	z := w.zones[0]
	alice := w.join(z, "Alice")
	bob := w.join(z, "Bob")
	w.step(50)
	events(alice)
	events(bob)

	z.addSanction(alice, character.SanctionMute, 1, "spam", "test")
	z.handle(command{player: alice, seq: 1, chat: &chatCommand{text: "can I talk"}})
	w.step(100)
	ev := events(alice)
	notices := 0
	refused := false
	for _, e := range ev {
		if s := e.GetSanction(); s != nil && s.GetKind() == wathenav1.SanctionKind_SANCTION_KIND_MUTE && s.GetReason() == "spam" && !s.GetEnded() &&
			s.GetEndsAtMs() == w.now.Add(time.Minute).UnixMilli() {
			notices++
		}
		if e.GetChatRefused().GetReason() == wathenav1.ChatRefusal_CHAT_REFUSAL_MUTED {
			refused = true
		}
	}
	if notices != 2 || !refused || len(chats(bob)) != 0 {
		t.Fatalf("muted: %d notices, refused %v; %v", notices, refused, ev)
	}
	if c := z.snapshot(alice); len(c.Sanctions) != 1 || c.Sanctions[0].Reason != "spam" {
		t.Errorf("saved %+v", c.Sanctions)
	}
	// The mute ends on the server clock.
	w.now = w.now.Add(61 * time.Second)
	w.step(1200)
	if s := eventsOf(alice, (*wathenav1.Event).GetSanction, (*wathenav1.Event).HasSanction); len(s) != 1 || !s[0].GetEnded() {
		t.Fatalf("end notice %v", s)
	}
	z.handle(command{player: alice, seq: 2, chat: &chatCommand{text: "darn spammers"}})
	w.step(1250)
	heard := chats(bob)
	if len(heard) != 1 {
		t.Fatalf("Bob hears %v", heard)
	}
	id, _ := chat.LineIDFrom(heard[0].GetLineId())

	// Bob reports the line. The report keeps what Bob saw.
	z.handle(command{player: bob, seq: 1, report: &reportCommand{name: "alice", line: id, reason: "rude"}})
	z.handle(command{player: bob, seq: 2, report: &reportCommand{name: "Carol", line: id}})
	w.step(1300)
	res := eventsOf(bob, (*wathenav1.Event).GetReportResult, (*wathenav1.Event).HasReportResult)
	if len(res) != 2 || res[0].GetOutcome() != wathenav1.ReportOutcome_REPORT_OUTCOME_SENT || res[1].GetOutcome() != wathenav1.ReportOutcome_REPORT_OUTCOME_LINE_NOT_FOUND {
		t.Fatalf("results %v", res)
	}
	reports := w.log.Reports()
	if len(reports) != 1 || reports[0].Target != "Alice" || reports[0].Line != id || reports[0].Reason != "rude" ||
		len(reports[0].Context) != 1 || reports[0].Context[0].Text != "darn spammers" {
		t.Fatalf("reports %+v", reports)
	}
	// Lines older than 5 minutes are not part of a report.
	w.now = w.now.Add(6 * time.Minute)
	z.handle(command{player: bob, seq: 3, report: &reportCommand{name: "Alice"}})
	w.step(1350)
	if r := w.log.Reports(); len(r) != 2 || len(r[1].Context) != 0 {
		t.Errorf("late report %+v", r[1])
	}
}

func TestJail(t *testing.T) {
	w := newSocialWorld(nil, "map/a")
	z := w.zones[0]
	z.Jail = Jail{Map: "map/a", Cells: [][2]int{{5, 5}}}
	z.SetWarps([]Warp{{X: 30, Y: 30, To: "map/a", ToX: 40, ToY: 40}})
	alice := w.join(z, "Alice")
	w.step(50)
	alice.entity.x, alice.entity.y = 20, 21
	events(alice)
	z.addSanction(alice, character.SanctionJail, 2, "botting", "test")
	w.step(100)
	if e := alice.entity; e.x != 5 || e.y != 5 {
		t.Fatalf("jailed at %d,%d", e.x, e.y)
	}
	// A warp does not take a jailed player.
	alice.entity.x, alice.entity.y = 30, 30
	if z.warp(alice, &z.warps[0]) {
		t.Error("a warp took the jailed player")
	}
	alice.entity.x, alice.entity.y = 5, 5
	w.now = w.now.Add(2*time.Minute + time.Second)
	w.step(1200)
	if e := alice.entity; e.x != 20 || e.y != 21 || z.jailed(alice) {
		t.Errorf("freed at %d,%d, jailed %v", e.x, e.y, z.jailed(alice))
	}
}
