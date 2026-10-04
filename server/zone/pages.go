package zone

import (
	"google.golang.org/protobuf/proto"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Pages (decision 0023): a server message is at most 64 KiB
// (schema.MaxMessageBytes), and a zone sends one frame per player per
// tick. Lists that grow with a limit, such as a storage of 10,000 slots,
// go out in pages:
//   - A page holds at most pageItems slots and pageBytes bytes.
//   - The first page goes in the tick's frame. Each later page goes in a
//     frame of its own, after it: at most pagesPerTick each tick, so a
//     long list never fills the connection's buffer of outBuffer
//     messages, which would drop the player.
//   - While pages wait, storage moves do nothing, so every page comes
//     before any change to its list.

const (
	// pageItems is the most slots one page holds.
	pageItems = 100
	// pageBytes is the most bytes of slots one page holds, which leaves
	// room in its frame for the tick's other events.
	pageBytes = 32 << 10
	// pagesPerTick is the most pages a player gets in one tick.
	pagesPerTick = outBuffer / 4
)

// pageItemsOf splits slots into pages.
func pageItemsOf(items []*wathenav1.InventoryItem) [][]*wathenav1.InventoryItem {
	var out [][]*wathenav1.InventoryItem
	var cur []*wathenav1.InventoryItem
	size := 0
	for _, it := range items {
		n := proto.Size(it) + 4
		if len(cur) > 0 && (len(cur) >= pageItems || size+n > pageBytes) {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, it)
		size += n
	}
	if len(cur) > 0 || len(out) == 0 {
		out = append(out, cur)
	}
	return out
}

// sendPages queues the first event with the tick's frame and each later
// one in a frame of its own.
func (p *Player) sendPages(events []*wathenav1.Event) {
	if len(events) == 0 {
		return
	}
	p.pending = append(p.pending, events[0])
	p.pages = append(p.pages, events[1:]...)
}

// flushPages queues up to pagesPerTick of the player's waiting pages, one
// frame each, after the tick's frame.
func (z *Zone) flushPages(p *Player) {
	n := min(len(p.pages), pagesPerTick)
	for _, e := range p.pages[:n] {
		z.queue(p, wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
			Tick: z.tick, AckSeq: p.ackSeq, Events: []*wathenav1.Event{e},
		}.Build()}.Build())
	}
	p.pages = p.pages[n:]
	if len(p.pages) == 0 {
		p.pages = nil
	}
}
