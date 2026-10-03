package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// TestWhispers: a whisper reaches a player on the same map and on
// another map, and a whisper to a name that is not online says so.
func TestWhispers(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")

	r := alice.Whisper("bob", "hello there")
	if r.GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_SENT || r.GetTo() != "Bob" || r.GetText() != "hello there" {
		t.Fatalf("whisper result %v", r)
	}
	if w := bob.HearsWhisper("Alice", "hello there"); !bytes.Equal(w.GetLineId(), r.GetLineId()) {
		t.Errorf("line IDs %x and %x", w.GetLineId(), r.GetLineId())
	}

	// Bob goes to the other map, which another zone runs.
	bob.Talk("Guide").Pick("Go to the field")
	bob.MapChange(FieldMap)
	if r := alice.Whisper("Bob", "still there?"); r.GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_SENT {
		t.Fatalf("whisper across maps %v", r)
	}
	bob.HearsWhisper("Alice", "still there?")
	if r := bob.Whisper("Alice", "yes"); r.GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_SENT {
		t.Fatalf("answer %v", r)
	}
	alice.HearsWhisper("Bob", "yes")

	if r := alice.Whisper("Nobody", "hi"); r.GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_NOT_ONLINE || r.GetTo() != "Nobody" {
		t.Errorf("whisper to Nobody %v", r)
	}
	srv.CheckLogs()
}

// TestChannels: two players on different maps join #trade, talk there,
// and see the member list.
func TestChannels(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	bob.Talk("Guide").Pick("Go to the field")
	bob.MapChange(FieldMap)

	j := alice.JoinChannel("#trade")
	if j.GetDisplayName() != "[Trade]" || j.GetColor() != 0xB6FF00 || !j.GetCanChat() || !j.GetCanLeave() {
		t.Errorf("joined %v", j)
	}
	bob.JoinChannel("#trade")
	line := alice.SayIn("#trade", "selling potions")
	if line.GetColor() != 0xB6FF00 || len(line.GetLineId()) != 16 {
		t.Errorf("line %v", line)
	}
	bob.HearsIn("#trade", "Alice", "selling potions")

	bob.send("members", func(ctx context.Context) error {
		return bob.Channel(ctx, wathenav1.ChannelAction_CHANNEL_ACTION_MEMBERS, "#trade")
	})
	m := bob.Expect("the members of #trade", (*wathenav1.Event).HasChannelMembers).GetChannelMembers()
	if !slices.Equal(m.GetNames(), []string{"Alice", "Bob"}) {
		t.Errorf("members %v", m.GetNames())
	}
	alice.send("list", func(ctx context.Context) error {
		return alice.Channel(ctx, wathenav1.ChannelAction_CHANNEL_ACTION_LIST, "")
	})
	list := alice.Expect("the channel list", (*wathenav1.Event).HasChannelList).GetChannelList()
	var names []string
	for _, c := range list.GetChannels() {
		names = append(names, c.GetName())
		if c.GetName() == "#trade" && (c.GetMembers() != 2 || !c.GetJoined()) {
			t.Errorf("#trade in the list: %v", c)
		}
	}
	if !slices.Equal(names, []string{"#global", "#map", "#support", "#trade"}) {
		t.Errorf("channels %v", names)
	}
	// A line in a channel the player is not in is refused.
	alice.send("say in #support", func(ctx context.Context) error { return alice.ChannelChat(ctx, "#support", "anyone?") })
	if r := alice.Expect("a refused line", (*wathenav1.Event).HasChatRefused).GetChatRefused(); r.GetReason() != wathenav1.ChatRefusal_CHAT_REFUSAL_NOT_IN_CHANNEL {
		t.Errorf("refused %v", r)
	}
	srv.CheckLogs()
}

// TestBlocking: a blocked player cannot whisper, and its local chat is
// hidden from the player who blocks it, but not from others.
func TestBlocking(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	for _, p := range []*Player{alice, bob, carol} {
		for _, other := range []string{"Alice", "Bob", "Carol"} {
			if other != p.Name {
				p.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, other)
			}
		}
	}
	if bl := bob.BlockName("Alice", false); !slices.Equal(bl.GetNames(), []string{"Alice"}) || bl.GetFull() {
		t.Fatalf("block list %v", bl)
	}
	if r := alice.Whisper("Bob", "hi Bob"); r.GetOutcome() != wathenav1.WhisperOutcome_WHISPER_OUTCOME_BLOCKED {
		t.Fatalf("whisper %v", r)
	}
	alice.Say("can anyone hear me")
	carol.Hears("Alice", "can anyone hear me")
	// Carol speaks after Alice. Bob hears Carol, and nothing from Alice
	// came before.
	mark := bob.Mark()
	carol.Say("I can")
	bob.Hears("Carol", "I can")
	events, _ := bob.Events(mark)
	for _, e := range events {
		if c := e.GetChat(); c != nil && c.GetFromName() == "Alice" {
			t.Errorf("Bob heard Alice: %v", c)
		}
	}
	// Unblocked, Alice is heard again.
	if bl := bob.BlockName("alice", true); len(bl.GetNames()) != 0 {
		t.Fatalf("block list after unblock %v", bl)
	}
	alice.Say("now?")
	bob.Hears("Alice", "now?")
	srv.CheckLogs()
}

// TestChatFilter: the fixture's filter word becomes asterisks for
// everyone, the speaker too.
func TestChatFilter(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	bob.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Alice")
	alice.send("chat", func(ctx context.Context) error { return alice.Chat(ctx, "that is a BadWord indeed") })
	alice.Hears("Alice", "that is a ******* indeed")
	bob.Hears("Alice", "that is a ******* indeed")
	if r := alice.Whisper("Bob", "badword"); r.GetText() != "*******" {
		t.Errorf("whisper %v", r)
	}
	srv.CheckLogs()
}

// TestMute: the Warden mutes a player for a minute. Its chat is refused
// with the end time, and works again once the server clock passes it.
func TestMute(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	bob.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Alice")
	c := alice.Talk("Warden")
	c.Choose("Mute me")
	c.Says("muted")
	c.Close()
	// The notice came with the dialog.
	e := alice.Seen(func(e *wathenav1.Event) bool { return e.GetSanction() != nil && !e.GetSanction().GetEnded() })
	if e == nil {
		t.Fatal("no sanction notice")
	}
	n := e.GetSanction()
	if n.GetKind() != wathenav1.SanctionKind_SANCTION_KIND_MUTE || n.GetReason() != "e2e mute" {
		t.Fatalf("notice %v", n)
	}
	if left := time.Until(time.UnixMilli(n.GetEndsAtMs())); left < 30*time.Second || left > 90*time.Second {
		t.Errorf("the mute ends in %v", left)
	}
	if r := alice.SayRefused("let me talk"); r.GetReason() != wathenav1.ChatRefusal_CHAT_REFUSAL_MUTED {
		t.Fatalf("refused %v", r)
	}
	alice.send("whisper", func(ctx context.Context) error { return alice.Bot.Whisper(ctx, "Bob", "psst") })
	if r := alice.Expect("a refused whisper", (*wathenav1.Event).HasChatRefused).GetChatRefused(); r.GetReason() != wathenav1.ChatRefusal_CHAT_REFUSAL_MUTED ||
		r.GetChannel() != wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER {
		t.Fatalf("whisper refused %v", r)
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	if err := srv.AdvanceClock(ctx, 61*time.Second); err != nil {
		t.Fatal(err)
	}
	alice.Sanction(true)
	alice.Say("I am back")
	bob.Hears("Alice", "I am back")
	heard, _ := bob.Events(0)
	for _, e := range heard {
		if c := e.GetChat(); c != nil && (c.GetText() == "let me talk" || c.GetText() == "psst") {
			t.Errorf("Bob heard a muted line: %v", c)
		}
	}
	srv.CheckLogs()
}

// TestReport: a player reports a line. The report keeps the line and
// the other chat the reporter saw.
func TestReport(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	want := chatToReport(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	reports, err := srv.Reports(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkReport(t, reports, want)
	srv.CheckLogs()
}

// TestReportInTheDatabase is TestReport with PostgreSQL: the report, its
// lines and the chat log are saved.
func TestReportInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	want := chatToReport(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var reports []ReportJSON
	deadline := time.Now().Add(10 * time.Second)
	for {
		var err error
		if reports, err = srv.Reports(ctx); err != nil {
			t.Fatal(err)
		}
		if len(reports) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	checkReport(t, reports, want)
	var lines, linked int
	if err := srv.DB.QueryRow(ctx, `SELECT count(*) FROM chat_log`).Scan(&lines); err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.QueryRow(ctx, `SELECT count(*) FROM report_lines`).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	// Three local lines and one whisper; the report holds the three
	// lines Bob saw.
	if lines != 4 || linked != 3 {
		t.Errorf("chat_log has %d lines and report_lines %d, want 4 and 3", lines, linked)
	}
	srv.CheckLogs()
}

// chatToReport plays the chat that Bob reports: Alice says two lines and
// whispers to Carol, Carol answers in local chat, and Bob reports
// Alice's second line.
func chatToReport(t *testing.T, srv *Server) *reported {
	t.Helper()
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	for _, p := range []*Player{bob, carol} {
		p.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Alice")
	}
	alice.Say("hello all")
	alice.Say("you are all terrible")
	alice.Whisper("Carol", "a secret")
	carol.Say("that was rude")
	bob.Hears("Alice", "hello all")
	rude := bob.Hears("Alice", "you are all terrible")
	bob.Hears("Carol", "that was rude")
	if o := bob.ReportLine("Alice", rude.GetLineId(), "insults"); o != wathenav1.ReportOutcome_REPORT_OUTCOME_SENT {
		t.Fatalf("report outcome %v", o)
	}
	if o := bob.ReportLine("Carol", rude.GetLineId(), ""); o != wathenav1.ReportOutcome_REPORT_OUTCOME_LINE_NOT_FOUND {
		t.Errorf("report of the wrong speaker: %v", o)
	}
	return &reported{line: hex.EncodeToString(rude.GetLineId())}
}

// reported is what the report scenario reported.
type reported struct{ line string }

func checkReport(t *testing.T, reports []ReportJSON, want *reported) {
	t.Helper()
	if len(reports) != 1 {
		t.Fatalf("reports %+v", reports)
	}
	r := reports[0]
	if r.Reporter != "Bob" || r.Target != "Alice" || r.Reason != "insults" || strings.ReplaceAll(r.Line, "-", "") != want.line || r.Status != "open" {
		t.Errorf("report %+v", r)
	}
	var got []string
	for _, l := range r.Lines {
		got = append(got, l.Name+": "+l.Text)
	}
	// Bob saw the three local lines, and not the whisper to Carol.
	if !slices.Equal(got, []string{"Alice: hello all", "Alice: you are all terrible", "Carol: that was rude"}) {
		t.Errorf("report lines %v", got)
	}
}
