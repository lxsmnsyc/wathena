package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// chats returns the chat events in a player's messages.
func chats(p *Player) []*wathenav1.ChatMessage {
	var out []*wathenav1.ChatMessage
	for _, e := range events(p) {
		if c := e.GetChat(); c != nil {
			out = append(out, c)
		}
	}
	return out
}

func TestChat(t *testing.T) {
	z := New("map/test", open(60), rng.FromKey([32]byte{}).Stream(), nil)
	a := add(z, "Alice")
	b := add(z, "Bob")
	c := add(z, "Carol")
	// Bob stands at the edge of chat range. Carol stands one cell past
	// it, where she still sees Alice.
	b.entity.x = a.entity.x + ChatRange
	c.entity.x = a.entity.x - ChatRange - 1
	z.stepAt(50)
	for _, p := range []*Player{a, b, c} {
		events(p)
	}

	z.handle(command{player: a, seq: 1, chat: new("hello")})
	z.stepAt(100)
	self := chats(a)
	if len(self) != 1 || self[0].GetFrom() != a.netIDs.id(a.entity) || self[0].GetFromName() != "Alice" ||
		self[0].GetText() != "hello" || self[0].GetChannel() != wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL {
		t.Errorf("Alice hears %v", self)
	}
	heard := chats(b)
	if len(heard) != 1 || heard[0].GetFrom() != b.netIDs.id(a.entity) || heard[0].GetText() != "hello" {
		t.Errorf("Bob hears %v", heard)
	}
	if got := chats(c); len(got) != 0 {
		t.Errorf("Carol, out of range, hears %v", got)
	}

	// Lines go out once.
	z.stepAt(150)
	if got := chats(b); len(got) != 0 {
		t.Errorf("Bob hears %v again", got)
	}

	// A dead player still talks, as in rAthena.
	a.entity.dead = true
	z.handle(command{player: a, seq: 2, chat: new("ouch")})
	z.stepAt(200)
	if got := chats(b); len(got) != 1 || got[0].GetText() != "ouch" {
		t.Errorf("Bob hears %v from the dead", got)
	}
}
