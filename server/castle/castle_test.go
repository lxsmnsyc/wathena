package castle

import (
	"slices"
	"testing"
)

type saved struct {
	name  string
	state State
}

type fakeStore struct{ saves []saved }

func (f *fakeStore) SaveCastle(name string, s State) { f.saves = append(f.saves, saved{name, s}) }

func records() []*Record {
	return []*Record{
		{ID: 15, Name: "Kriemhild", DisplayName: "Kriemhild", Map: "prtg_cas01", Edition: "fe", NPC: "Agit#prtg_cas01"},
		{ID: 0, Name: "Neuschwanstein", DisplayName: "Neuschwanstein", Map: "aldeg_cas01", Edition: "fe", NPC: "Agit#aldeg_cas01"},
		{ID: 10000, Name: "example.keep:keep", DisplayName: "keep", Map: "keep_cas", Edition: "se", NPC: "example.keep:steward"},
	}
}

func TestFind(t *testing.T) {
	r := New(records(), nil)
	for _, c := range []struct{ name, local, want string }{
		{"Kriemhild", "", "Kriemhild"},
		{"Kriemhild", "", "Kriemhild"},
		{"prtg_cas01", "", "Kriemhild"},
		{"keep", "example.keep", "example.keep:keep"},
		{"keep", "", ""},
		{"nowhere", "", ""},
	} {
		got := ""
		if rec := r.Find(c.name, c.local); rec != nil {
			got = rec.Name
		}
		if got != c.want {
			t.Errorf("Find(%q, %q) = %q, want %q", c.name, c.local, got, c.want)
		}
	}
	var ids []uint32
	for _, rec := range r.Records() {
		ids = append(ids, rec.ID)
	}
	if !slices.Equal(ids, []uint32{0, 15, 10000}) {
		t.Errorf("Records by ID: %v", ids)
	}
}

func TestWritesSave(t *testing.T) {
	st := &fakeStore{}
	r := New(records(), map[string]State{"Kriemhild": {Owner: 7, Economy: 30}})
	r.Store = st
	feed := r.Subscribe(nil)
	if s, _ := r.State("Kriemhild"); s.Owner != 7 || s.Economy != 30 {
		t.Fatalf("saved state not loaded: %+v", s)
	}
	if r.SetOwner("Kriemhild", 7, SourceScript) {
		t.Error("SetOwner to the same guild changed it")
	}
	if !r.SetOwner("Kriemhild", 9, SourceScript) {
		t.Error("SetOwner did not change the owner")
	}
	economy, kafra := 31, true
	if err := r.Set("Kriemhild", Values{Economy: &economy, Kafra: &kafra}); err != nil {
		t.Fatal(err)
	}
	negative := -1
	if err := r.Set("Kriemhild", Values{Defense: &negative}); err == nil {
		t.Error("a negative value was taken")
	}
	if err := r.SetGuardian("Kriemhild", 3, true); err != nil {
		t.Fatal(err)
	}
	if err := r.SetGuardian("Kriemhild", 8, true); err == nil {
		t.Error("guardian 8 was taken")
	}
	s, _ := r.State("Kriemhild")
	want := State{Owner: 9, Economy: 31, Kafra: true, Guardians: 1 << 3}
	if s != want || !s.Installed(3) || s.Installed(2) {
		t.Errorf("state %+v, want %+v", s, want)
	}
	if len(st.saves) != 3 || st.saves[2].state != want {
		t.Errorf("saves %+v", st.saves)
	}
	n := feed.Take()
	if len(n) != 1 || n[0].Change == nil || n[0].Change.Previous != 7 || n[0].Change.Owner != 9 {
		t.Errorf("feed %+v", n)
	}
	if len(feed.Take()) != 0 {
		t.Error("Take did not empty the feed")
	}
	if got := r.Of(9); len(got) != 1 || got[0].Name != "Kriemhild" {
		t.Errorf("Of(9) = %v", got)
	}
}

func TestGuildBroken(t *testing.T) {
	r := New(records(), map[string]State{"Kriemhild": {Owner: 7}, "example.keep:keep": {Owner: 7, Economy: 5}, "Neuschwanstein": {Owner: 8}})
	var events []string
	r.GuildBreak = func(npc, label string) {
		events = append(events, npc+"::"+label)
		// The label is sent before the owner is cleared.
		if npc == "Agit#prtg_cas01" && r.byMap["prtg_cas01"].state.Owner != 7 {
			t.Error("the owner was cleared before OnGuildBreak")
		}
	}
	feed := r.Subscribe(nil)
	r.GuildBroken(7)
	if !slices.Equal(events, []string{"Agit#prtg_cas01::OnGuildBreak", "example.keep:steward::OnGuildBreak"}) {
		t.Errorf("events %v", events)
	}
	for _, name := range []string{"Kriemhild", "example.keep:keep"} {
		if s, _ := r.State(name); s.Owner != 0 {
			t.Errorf("%s still owned: %+v", name, s)
		}
	}
	if s, _ := r.State("example.keep:keep"); s.Economy != 5 {
		t.Errorf("a guild break changed the economy: %+v", s)
	}
	if s, _ := r.State("Neuschwanstein"); s.Owner != 8 {
		t.Error("another guild's castle lost its owner")
	}
	n := feed.Take()
	if len(n) != 3 || n[2].Badge != 7 || n[0].Change.Owner != 0 {
		t.Errorf("feed %+v", n)
	}
}
