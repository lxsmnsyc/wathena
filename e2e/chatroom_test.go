package e2e

import (
	"context"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// roomShown waits for the bubble of a room by its title.
func (p *Player) roomShown(title string) *wathenav1.ChatRoomShow {
	p.t.Helper()
	return p.Expect("the room "+title, func(e *wathenav1.Event) bool { return e.GetChatRoomShow().GetTitle() == title }).GetChatRoomShow()
}

// joinRoom joins a room and waits for the player's view of it.
func (p *Player) joinRoom(room uint32, password string) *wathenav1.ChatRoomJoined {
	p.t.Helper()
	p.send("join the room", func(ctx context.Context) error { return p.ChatRoomJoin(ctx, room, password) })
	if r := p.Expect("the join's answer", func(e *wathenav1.Event) bool { return e.HasChatRoomResult() }).GetChatRoomResult(); r.GetOutcome() != wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_OK {
		p.t.Fatalf("%s: joining: %v", p.Name, r.GetOutcome())
	}
	return p.Expect("the room", func(e *wathenav1.Event) bool { return e.HasChatRoomJoined() }).GetChatRoomJoined()
}

// TestNPCChatRoomWarps: the fixture's Arena is an NPC room of 4 that
// takes 3 players, with a trigger of 2 and a fee of 10 zeny. Two players
// join it, one speaks in it, and the second join runs the Arena's event,
// which warps both to the field and takes the fee from each.
func TestNPCChatRoomWarps(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	// The bubbles come with the first frames, before any command.
	room := alice.roomShown("E2E Arena")
	if !room.GetNpc() || !room.GetPublic() || room.GetLimit() != 4 || room.GetUsers() != 0 || room.GetZeny() != 10 {
		t.Fatalf("the bubble %v", room)
	}
	bob.roomShown("E2E Arena")
	alice.MustCommand("!zeny 100")
	bob.MustCommand("!zeny 100")

	a := alice.joinRoom(room.GetRoom(), "")
	if a.GetTitle() != "E2E Arena" || len(a.GetMembers()) != 1 || a.GetMembers()[0] != "Alice" {
		t.Fatalf("Alice's view %v", a)
	}
	bob.Expect("the room's new count", func(e *wathenav1.Event) bool {
		return e.GetChatRoomShow().GetTitle() == "E2E Arena" && e.GetChatRoomShow().GetUsers() == 1
	})
	alice.send("say", func(ctx context.Context) error { return alice.RoomChat(ctx, "ready?") })
	alice.Expect("her own line", func(e *wathenav1.Event) bool {
		return e.GetChat().GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_ROOM && e.GetChat().GetText() == "ready?"
	})

	b := bob.joinRoom(room.GetRoom(), "")
	if len(b.GetMembers()) != 2 || b.GetMembers()[0] != "Alice" {
		t.Fatalf("Bob's view %v", b)
	}
	alice.MapChange(FieldMap)
	bob.MapChange(FieldMap)
	for _, p := range []*Player{alice, bob} {
		p.Until("the fee was paid", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 90 })
	}
	srv.CheckLogs()
}
