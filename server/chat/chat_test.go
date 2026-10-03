package chat

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestFilter(t *testing.T) {
	f := NewFilter([]Word{
		{Word: "darn", Action: Replace},
		{Word: "spamlink", Action: Reject},
		{Word: "gm", Action: Reserve},
		{Word: "heck", Action: Replace, Part: true},
		{Word: "bad phrase", Action: Replace},
	})
	cases := []struct {
		in, out string
		ok      bool
	}{
		{"hello", "hello", true},
		{"oh darn it", "oh **** it", true},
		{"DARN!", "****!", true},
		// A word alone: darned is another word.
		{"darned", "darned", true},
		{"what the heckity", "what the ****ity", true},
		{"visit spamlink now", "", false},
		{"a Bad Phrase here", "a ********** here", true},
		// Reserved words only apply to names.
		{"ask a gm", "ask a gm", true},
		{"élan darn", "élan ****", true},
	}
	for _, c := range cases {
		out, ok := f.Check(c.in)
		if out != c.out || ok != c.ok {
			t.Errorf("Check(%q) = %q, %v; want %q, %v", c.in, out, ok, c.out, c.ok)
		}
	}
	for name, want := range map[string]error{
		"Alice": nil, "GM_Bob": ErrNameReserved, "Pigment": nil, "gm": ErrNameReserved,
		"Darn Fan": ErrNameFiltered, "Heckler": ErrNameFiltered,
	} {
		if err := f.CheckName(name); !errors.Is(err, want) && (err != nil || want != nil) {
			t.Errorf("CheckName(%q) = %v, want %v", name, err, want)
		}
	}
}

func TestCoreData(t *testing.T) {
	d := Core()
	var names []string
	for _, c := range d.Channels {
		names = append(names, c.Full())
	}
	if !slices.Equal(names, []string{"#global", "#support", "#trade", "#map"}) {
		t.Fatalf("channels %v", names)
	}
	g := d.Channels[0]
	if g.DisplayName != "[Global]" || g.Color != 0xFFFFFF || g.CanLeave || !g.CanChat || g.Delay != 1000 {
		t.Errorf("global %+v", g)
	}
	if d.Channels[3].Kind != KindMap {
		t.Errorf("map %+v", d.Channels[3])
	}
	if err := d.Filter.CheckName("Admin"); !errors.Is(err, ErrNameReserved) {
		t.Errorf("Admin: %v", err)
	}
}

func TestBuildChannelErrors(t *testing.T) {
	for _, f := range []map[string]any{
		{"color": 0x1000000}, {"join": "closed"}, {"delay_milliseconds": -1}, {"kind": "guild"},
		{"autojoin": true, "join": "invite"}, {"colour": 1},
	} {
		if _, err := BuildChannel("lounge", f); err == nil {
			t.Errorf("%v: no error", f)
		}
	}
	if _, err := BuildChannel("#lounge", nil); err == nil {
		t.Error("a name with # is not a record name")
	}
}

// inbox records deliveries.
type inbox struct{ got []Delivery }

func (b *inbox) DeliverChat(d Delivery) { b.got = append(b.got, d) }

func (b *inbox) texts(kind DeliveryKind) []string {
	var out []string
	for _, d := range b.got {
		if d.Kind == kind {
			out = append(out, d.Line.Name+": "+d.Line.Text)
		}
	}
	return out
}

func TestHubWhispers(t *testing.T) {
	h := NewHub(Core().Channels, Core().Filter)
	var a, b inbox
	alice, _ := h.Online("Alice", 1, &a, "map/a", nil)
	bob, _ := h.Online("Bob", 2, &b, "map/b", []string{"carol"})
	carol, _ := h.Online("Carol", 3, &inbox{}, "map/a", nil)
	if to, o := h.Whisper(alice, Line{To: "bob", Name: "Alice", Text: "hi"}); o != WhisperSent || to != "Bob" {
		t.Fatalf("whisper: %q %v", to, o)
	}
	if got := b.texts(DeliverWhisper); !slices.Equal(got, []string{"Alice: hi"}) {
		t.Errorf("Bob got %v", got)
	}
	if _, o := h.Whisper(carol, Line{To: "Bob", Name: "Carol", Text: "hey"}); o != WhisperBlocked {
		t.Errorf("blocked whisper: %v", o)
	}
	if _, o := h.Whisper(alice, Line{To: "Dave", Text: "x"}); o != WhisperNotOnline {
		t.Errorf("offline whisper: %v", o)
	}
	// A newer login of Bob replaces the older one.
	var b2 inbox
	bob2, _ := h.Online("BOB", 2, &b2, "map/b", nil)
	h.Offline(bob)
	if _, o := h.Whisper(alice, Line{To: "bob", Name: "Alice", Text: "again"}); o != WhisperSent || len(b2.got) != 1 {
		t.Errorf("after relogin: %v %v", o, b2.got)
	}
	h.Offline(bob2)
	if _, o := h.Whisper(alice, Line{To: "bob", Text: "x"}); o != WhisperNotOnline {
		t.Errorf("after logout: %v", o)
	}
}

func TestHubChannels(t *testing.T) {
	h := NewHub(Core().Channels, Core().Filter)
	var a, b, c inbox
	alice, _ := h.Online("Alice", 1, &a, "map/a", nil)
	bob, _ := h.Online("Bob", 2, &b, "map/b", nil)
	carol, _ := h.Online("Carol", 3, &c, "map/a", []string{"alice"})
	now := time.Unix(1000, 0)
	for _, m := range []*Member{alice, bob, carol} {
		if _, f := h.Join(m, "#trade", false); f != 0 {
			t.Fatalf("join: %v", f)
		}
	}
	if _, f := h.Join(alice, "trade", false); f != ChannelAlreadyJoined {
		t.Errorf("join twice: %v", f)
	}
	if _, f := h.Send(alice, "#trade", Line{Name: "Alice", Text: "selling"}, now); f != 0 {
		t.Fatalf("send: %v", f)
	}
	// Everyone hears it but Carol, who blocks Alice. The sender hears
	// itself, as rAthena.
	if len(a.got) != 1 || len(b.got) != 1 || len(c.got) != 0 || b.got[0].Channel.Name != "#trade" || b.got[0].Line.To != "#trade" {
		t.Errorf("deliveries %v %v %v", a.got, b.got, c.got)
	}
	if _, f := h.Send(alice, "#trade", Line{Name: "Alice", Text: "again"}, now.Add(500*time.Millisecond)); f != ChannelTooFast {
		t.Errorf("too fast: %v", f)
	}
	if _, f := h.Send(alice, "#support", Line{}, now); f != ChannelNotJoined {
		t.Errorf("not joined: %v", f)
	}
	names, _ := h.Members(bob, "#trade")
	if !slices.Equal(names, []string{"Alice", "Bob", "Carol"}) {
		t.Errorf("members %v", names)
	}
	// #global cannot be left, as rAthena's leave: false.
	h.Join(alice, "global", false)
	if _, f := h.Leave(alice, "#global", false); f != ChannelCannotLeave {
		t.Errorf("leave global: %v", f)
	}
	// The map channel: members hear those on their own map, and a map
	// change leaves it.
	h.Join(alice, "map", false)
	h.Join(bob, "map", false)
	a.got, b.got = nil, nil
	h.Send(alice, "map", Line{Name: "Alice", Text: "here"}, now)
	if len(a.got) != 1 || len(b.got) != 0 {
		t.Errorf("map channel: %v %v", a.got, b.got)
	}
	if left := h.Move(alice, &a, "map/c"); len(left) != 1 || left[0].Name != "#map" {
		t.Errorf("move left %v", left)
	}
	list := h.List(alice)
	if len(list) != 4 || !list[0].Joined || list[1].Joined || !list[3].Joined {
		t.Errorf("list %+v", list)
	}
}

func TestHubModChannels(t *testing.T) {
	h := NewHub(Core().Channels, nil)
	if _, err := h.Create(ChannelDef{Name: "trade", Mod: "example.a"}, 1); !errors.Is(err, ErrChannelTaken) {
		t.Errorf("core name: %v", err)
	}
	c, err := h.Create(ChannelDef{Name: "Lounge", Mod: "example.a", Join: JoinInvite}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Create(ChannelDef{Name: "lounge", Mod: "example.b"}, 2); !errors.Is(err, ErrChannelTaken) {
		t.Errorf("other mod: %v", err)
	}
	if c2, err := h.Create(ChannelDef{Name: "lounge", Mod: "example.a", Join: JoinInvite}, 3); err != nil || c2 != c {
		t.Fatalf("same mod: %v", err)
	}
	var a inbox
	alice, _ := h.Online("Alice", 1, &a, "map/a", nil)
	if _, f := h.Join(alice, "lounge", false); f != ChannelInviteOnly {
		t.Errorf("invite only: %v", f)
	}
	if !h.AddMember(c, alice) || h.AddMember(c, alice) {
		t.Error("add")
	}
	h.Announce(c, "welcome", time.Now())
	if len(a.got) != 2 || a.got[0].Kind != DeliverJoined || a.got[1].Line.Text != "welcome" || a.got[1].Line.Name != "" {
		t.Errorf("deliveries %+v", a.got)
	}
	// The channel ends with its last owner.
	h.Release(c, 1)
	if !h.Running(c) {
		t.Fatal("ended with an owner left")
	}
	h.Release(c, 3)
	if h.Running(c) || h.Channel("lounge") != nil || a.got[len(a.got)-1].Kind != DeliverLeft {
		t.Errorf("after release: running %v, %+v", h.Running(c), a.got)
	}
}

func TestLimits(t *testing.T) {
	l := DefaultLimits()
	made := time.Unix(1000, 0)
	if b, r := l.For(made, made.Add(time.Minute)); b != 3 || r != 3*time.Second {
		t.Errorf("first hour: %d %v", b, r)
	}
	if b, r := l.For(made, made.Add(time.Hour)); b != 5 || r != time.Second {
		t.Errorf("after: %d %v", b, r)
	}
	if err := l.Check(); err != nil {
		t.Error(err)
	}
	l.NewBurst = 9
	if err := l.Check(); err == nil {
		t.Error("new_character_burst above burst")
	}
}

func TestReportsLimit(t *testing.T) {
	h := NewHub(nil, nil)
	m, _ := h.Online("Alice", 1, &inbox{}, "map/a", nil)
	now := time.Unix(1000, 0)
	for i := range MaxReports {
		if !h.AllowReport(m, now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("report %d refused", i)
		}
	}
	if h.AllowReport(m, now.Add(time.Minute)) {
		t.Error("one report too many")
	}
	if !h.AllowReport(m, now.Add(ReportWindow+time.Second)) {
		t.Error("the window did not move")
	}
}

func TestLineID(t *testing.T) {
	id := NewLineID(time.Unix(1700000000, 0))
	back, ok := LineIDFrom(id[:])
	if !ok || back != id || id.IsZero() || len(id.String()) != 36 || id.String()[14] != '7' {
		t.Errorf("id %s", id)
	}
	if _, ok := LineIDFrom([]byte{1, 2}); ok {
		t.Error("short id accepted")
	}
}
