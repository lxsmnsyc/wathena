package zone

import wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"

// chatLine is one line of local chat, from a player or an NPC.
type chatLine struct {
	speaker *entity
	name    string
	text    string
}

// ChatRange is how far local chat reaches, in cells on both axes. It is
// rAthena's AREA_SIZE-5, which AREA_CHAT_WOC uses.
const ChatRange = ViewRange - 5

// chatEvents are the chat lines a player hears this tick: those of every
// player within ChatRange that it sees, itself included. rAthena sends a
// line to the players in range (AREA_CHAT_WOC) and echoes it to the
// speaker.
func (z *Zone) chatEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	me := p.entity
	for _, c := range z.chats {
		id, ok := p.netIDs.get(c.speaker)
		if !ok || max(abs(c.speaker.x-me.x), abs(c.speaker.y-me.y)) > ChatRange {
			continue
		}
		events = append(events, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
			Channel:  wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL,
			From:     id,
			FromName: c.name,
			Text:     c.text,
		}.Build()}.Build())
	}
	return events
}

// announcement is a line to every player of the zone, as rAthena's
// announce.
type announcement struct {
	text  string
	color uint32
}

// Announce shows a line to every player of the zone. Any goroutine may
// call it. It never waits: when the zone is far behind, the line is
// dropped.
func (z *Zone) Announce(text string, color uint32) {
	select {
	case z.announces <- announcement{text, color}:
	default:
	}
}

// announce adds an announcement to every player's next frame.
func (z *Zone) announce(a announcement) {
	for _, p := range z.players {
		p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
			Channel: wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT,
			Text:    a.text,
			Color:   a.color,
		}.Build()}.Build())
	}
}
