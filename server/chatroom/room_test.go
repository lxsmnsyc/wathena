package chatroom

import "testing"

func joiner(id int64, name string) Joiner {
	return Joiner{Member: Member{ID: id, Name: name}, Level: 50, Zeny: 1000}
}

// An NPC room counts the NPC: it takes limit-1 players, but full tests
// users >= limit (getwaitingroomstate(32)).
func TestNPCRoomCountsNPC(t *testing.T) {
	r := New(1, true)
	r.Public = true
	r.SetLimit(3)
	if got := r.Join(joiner(1, "a"), ""); got != OK {
		t.Fatalf("a: %v", got)
	}
	if got := r.Join(joiner(2, "b"), ""); got != OK {
		t.Fatalf("b: %v", got)
	}
	if got := r.Join(joiner(3, "c"), ""); got != Full {
		t.Fatalf("c: %v, want full", got)
	}
	if r.Users() != 2 || r.Full() {
		t.Fatalf("users %d full %v: full is users >= limit, 2 < 3", r.Users(), r.Full())
	}
	// A player room takes limit players.
	p := New(2, false)
	p.Public = true
	p.SetLimit(2)
	p.Add(Member{9, "owner"})
	if p.Join(joiner(1, "a"), "") != OK || p.Join(joiner(2, "b"), "") != Full || !p.Full() {
		t.Fatal("a player room of 2 holds its owner and one more")
	}
}

func TestLimitCut(t *testing.T) {
	r := New(1, false)
	r.SetLimit(99)
	r.SetTrigger(300)
	if r.Limit != MaxUsers || r.Trigger != MaxTrigger {
		t.Fatalf("limit %d trigger %d", r.Limit, r.Trigger)
	}
}

// The checks run in rAthena's order: busy or full, password, level low,
// level high, zeny, kicked.
func TestJoinOrder(t *testing.T) {
	r := New(1, false)
	r.SetLimit(5)
	r.Password = "pw"
	r.MinLevel, r.MaxLevel = 10, 60
	r.Zeny = 500
	r.Add(Member{9, "owner"})
	r.kicked[7] = true

	tests := []struct {
		name string
		j    Joiner
		pw   string
		want Outcome
	}{
		{"busy beats everything", Joiner{Member: Member{7, "k"}, Busy: true}, "bad", Full},
		{"in a room", Joiner{Member: Member{7, "k"}, InRoom: true}, "pw", Full},
		{"password before level", Joiner{Member: Member{7, "k"}, Level: 1}, "bad", WrongPassword},
		{"level low before zeny", Joiner{Member: Member{7, "k"}, Level: 5}, "pw", LevelLow},
		{"level high", Joiner{Member: Member{7, "k"}, Level: 61, Zeny: 9999}, "pw", LevelHigh},
		{"zeny before kicked", Joiner{Member: Member{7, "k"}, Level: 20, Zeny: 10}, "pw", NoZeny},
		{"kicked", Joiner{Member: Member{7, "k"}, Level: 20, Zeny: 500}, "pw", Kicked},
		{"ok", Joiner{Member: Member{8, "g"}, Level: 20, Zeny: 500}, "pw", OK},
		{"permission skips the password", Joiner{Member: Member{6, "gm"}, Level: 20, Zeny: 500, Any: true}, "", OK},
	}
	for _, tt := range tests {
		if got := r.Join(tt.j, tt.pw); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
	if r.Users() != 3 {
		t.Fatalf("users %d", r.Users())
	}
}

// A public room needs no password; an NPC room has no kick list.
func TestPublicAndNPCKickList(t *testing.T) {
	r := New(1, true)
	r.Public = true
	r.SetLimit(5)
	r.Password = "x"
	if r.Join(joiner(1, "a"), "") != OK {
		t.Fatal("public room refused")
	}
	if _, empty, ok := r.Leave(1); !ok || empty {
		t.Fatal("an NPC room stays when empty")
	}
	if r.Join(joiner(1, "a"), "") != OK {
		t.Fatal("an NPC room has no kick list")
	}
}

func TestLeaveOwnerAndEmpty(t *testing.T) {
	r := New(1, false)
	r.Public = true
	r.SetLimit(5)
	r.Add(Member{1, "a"})
	r.Join(joiner(2, "b"), "")
	r.Join(joiner(3, "c"), "")
	if o, empty, ok := r.Leave(2); !ok || empty || o != nil {
		t.Fatalf("a middle member leaves: %v %v %v", o, empty, ok)
	}
	o, empty, ok := r.Leave(1)
	if !ok || empty || o == nil || o.Name != "c" {
		t.Fatalf("the owner leaves: %v %v %v", o, empty, ok)
	}
	if _, empty, _ := r.Leave(3); !empty {
		t.Fatal("an empty player room is deleted")
	}
	if _, _, ok := r.Leave(3); ok {
		t.Fatal("a non-member left")
	}
}

func TestKickKeepsOut(t *testing.T) {
	r := New(1, false)
	r.Public = true
	r.SetLimit(5)
	r.Add(Member{1, "owner"})
	r.Join(joiner(2, "b"), "")
	r.Join(joiner(3, "gm"), "")
	if _, ok := r.Kick(2, "owner", nil); ok {
		t.Fatal("a member kicked the owner")
	}
	if _, ok := r.Kick(1, "gm", func(m Member) bool { return m.Name == "gm" }); ok {
		t.Fatal("a protected member was kicked")
	}
	m, ok := r.Kick(1, "b", nil)
	if !ok || m.ID != 2 {
		t.Fatalf("kick: %v %v", m, ok)
	}
	r.Leave(2)
	if r.Join(joiner(2, "b"), "") != Kicked {
		t.Fatal("a kicked player came back")
	}
	if _, ok := r.Kick(1, "nobody", nil); ok {
		t.Fatal("kicked a name that is not here")
	}
}

func TestTransferAndChange(t *testing.T) {
	r := New(1, false)
	r.Public = true
	r.SetLimit(5)
	r.Add(Member{1, "a"})
	r.Join(joiner(2, "b"), "")
	r.Join(joiner(3, "c"), "")
	if r.Transfer(2, "c") || r.Transfer(1, "a") || r.Transfer(1, "z") {
		t.Fatal("a bad transfer worked")
	}
	if !r.Transfer(1, "c") {
		t.Fatal("transfer")
	}
	if o, _ := r.Owner(); o.Name != "c" {
		t.Fatalf("owner %v", o)
	}
	if got := r.Members(); got[0].Name != "c" || got[2].Name != "a" {
		t.Fatalf("members %v: the owners swap places", got)
	}
	if r.Change(1, "t", "p", 3, false) {
		t.Fatal("a former owner changed the room")
	}
	if !r.Change(3, "t", "p", 99, false) || r.Title != "t" || r.Password != "p" || r.Public || r.Limit != MaxUsers {
		t.Fatalf("change: %+v", r)
	}
}

// The trigger runs after a join once users reach it; disable sets the
// 0x80 flag, so state 33 is false; enable clears it.
func TestTrigger(t *testing.T) {
	r := New(1, true)
	r.Public = true
	r.SetLimit(10)
	r.SetTrigger(2)
	r.Event = "core:Arena::OnStart"
	r.Join(joiner(1, "a"), "")
	if r.ShouldRun() {
		t.Fatal("one user triggered a trigger of two")
	}
	r.Join(joiner(2, "b"), "")
	if !r.ShouldRun() || !r.Triggered() {
		t.Fatal("two users did not trigger")
	}
	r.Disabled = true
	if r.ShouldRun() || r.Triggered() {
		t.Fatal("a disabled trigger ran")
	}
	r.Disabled = false
	if !r.ShouldRun() {
		t.Fatal("enable does not run the check at once: the zone does")
	}
	r.Event = ""
	if r.ShouldRun() {
		t.Fatal("no event, no run")
	}
}

func TestNextID(t *testing.T) {
	rooms := map[uint32]*Room{1: nil, 2: nil}
	if got := NextID(rooms, 0); got != 3 {
		t.Fatalf("next %d", got)
	}
	if got := NextID(rooms, 3); got != 4 {
		t.Fatalf("after 3: %d", got)
	}
}
