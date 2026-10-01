package zone

import wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"

// chatLine is one line of local chat.
type chatLine struct {
	speaker *entity
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
			FromName: c.speaker.player.Name,
			Text:     c.text,
		}.Build()}.Build())
	}
	return events
}
