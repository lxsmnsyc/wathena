// Package zone runs one map: the players on it, their walks, and what each
// player sees. One goroutine owns all of a zone's state and advances it in
// 50 ms ticks (strategy section 4.1), so game logic needs no locks.
package zone

import (
	"context"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/path"
)

const (
	// Tick is the time between zone updates.
	Tick = 50 * time.Millisecond
	// ViewRange is how far a player sees, in cells, as rAthena's
	// area_size.
	ViewRange = 14
	// WalkSpeed is RO's default walk speed, in milliseconds per cell.
	WalkSpeed = 150
	// outBuffer is how many messages may wait for a slow connection
	// before the zone drops it.
	outBuffer = 64
)

// Look is a player's appearance, as IDs from the client data tables.
type Look struct {
	Job, Sex, Hair, HairColor uint32
}

// Player is one connected player. Its Out channel carries messages to
// the connection, and closes when the zone drops the player.
type Player struct {
	Name string
	Look Look
	Out  <-chan *wathenav1.ServerMessage

	out    chan *wathenav1.ServerMessage
	entity *entity
	// netIDs are this connection's IDs for the entities it sees.
	netIDs  map[*entity]uint32
	nextNet uint32
	ackSeq  uint32
	// fresh is set until the player's first frame, which shows the player
	// to itself.
	fresh   bool
	dropped bool
}

type entity struct {
	player *Player
	x, y   int
	// dir is the facing, 0 south, clockwise.
	dir   int
	speed int
	// path holds the steps left. stepStart is when path[0] began, in zone
	// milliseconds.
	path      []path.Dir
	stepStart int64
	// walkChanged is set when a walk starts or changes, so viewers get a
	// move event.
	walkChanged bool
}

type command struct {
	player *Player
	seq    uint32
	walkTo *[2]int
}

// Zone is one running map.
type Zone struct {
	Map  string
	grid *path.Grid

	start    time.Time
	now      int64
	tick     uint32
	entities map[*entity]bool

	joins    chan joinRequest
	leaves   chan *Player
	commands chan command
}

type joinRequest struct {
	player *Player
	done   chan joinResult
}

type joinResult struct {
	netID uint32
	tick  uint32
}

// New makes a zone for a map, given its asset name and cells.
func New(mapName string, grid *path.Grid) *Zone {
	return &Zone{
		Map:      mapName,
		grid:     grid,
		entities: map[*entity]bool{},
		joins:    make(chan joinRequest),
		leaves:   make(chan *Player, 16),
		commands: make(chan command, 1024),
	}
}

// Run advances the zone until ctx ends.
func (z *Zone) Run(ctx context.Context) {
	z.start = time.Now()
	t := time.NewTicker(Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			for e := range z.entities {
				z.drop(e.player)
			}
			return
		case j := <-z.joins:
			j.done <- z.join(j.player)
		case p := <-z.leaves:
			z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
		case c := <-z.commands:
			z.handle(c)
		case <-t.C:
			z.step()
		}
	}
}

// Join adds a player at the walkable cell nearest the map's center, and
// returns the player's own entity ID and the current tick.
func (z *Zone) Join(ctx context.Context, name string, look Look) (*Player, uint32, uint32, error) {
	out := make(chan *wathenav1.ServerMessage, outBuffer)
	p := &Player{Name: name, Look: look, Out: out, out: out, netIDs: map[*entity]uint32{}, fresh: true}
	done := make(chan joinResult, 1)
	select {
	case z.joins <- joinRequest{p, done}:
	case <-ctx.Done():
		return nil, 0, 0, ctx.Err()
	}
	r := <-done
	return p, r.netID, r.tick, nil
}

// Leave removes a player. The player's Out channel closes.
func (z *Zone) Leave(p *Player) { z.leaves <- p }

// WalkTo asks for the player to walk to a cell. seq is the client
// message's sequence number, which frames acknowledge. It returns false
// when the zone's queue is full, which the caller should treat as too
// many commands.
func (z *Zone) WalkTo(p *Player, seq uint32, x, y int) bool {
	select {
	case z.commands <- command{player: p, seq: seq, walkTo: &[2]int{x, y}}:
		return true
	default:
		return false
	}
}

// Ack records a processed message that needs no game action, such as a
// ping.
func (z *Zone) Ack(p *Player, seq uint32) bool {
	select {
	case z.commands <- command{player: p, seq: seq}:
		return true
	default:
		return false
	}
}

func (z *Zone) join(p *Player) joinResult {
	x, y := z.startCell()
	e := &entity{player: p, x: x, y: y, speed: WalkSpeed}
	p.entity = e
	z.entities[e] = true
	// The player sees itself from the first frame.
	return joinResult{netID: p.netID(e), tick: z.tick}
}

func (z *Zone) startCell() (int, int) {
	cx, cy := z.grid.Width/2, z.grid.Height/2
	for r := range max(z.grid.Width, z.grid.Height) {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if !z.grid.NoPass(cx+dx, cy+dy) {
					return cx + dx, cy + dy
				}
			}
		}
	}
	return cx, cy
}

func (p *Player) netID(e *entity) uint32 {
	if id, ok := p.netIDs[e]; ok {
		return id
	}
	p.nextNet++
	p.netIDs[e] = p.nextNet
	return p.nextNet
}

func (z *Zone) leave(p *Player, reason wathenav1.VanishReason) {
	if p.entity == nil || !z.entities[p.entity] {
		return
	}
	delete(z.entities, p.entity)
	for e := range z.entities {
		if id, ok := e.player.netIDs[p.entity]; ok {
			delete(e.player.netIDs, p.entity)
			e.player.queue(wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
				Tick: z.tick, AckSeq: e.player.ackSeq, Events: []*wathenav1.Event{vanish(id, reason)},
			}.Build()}.Build())
		}
	}
	z.drop(p)
}

// drop closes a player's messages. Its connection then closes.
func (z *Zone) drop(p *Player) {
	if !p.dropped {
		p.dropped = true
		close(p.out)
	}
}

func (z *Zone) handle(c command) {
	p := c.player
	if p.dropped || !z.entities[p.entity] {
		return
	}
	if c.seq > p.ackSeq {
		p.ackSeq = c.seq
	}
	if c.walkTo != nil {
		z.walk(p.entity, c.walkTo[0], c.walkTo[1])
	}
}

// walk starts or changes a walk. A unit in the middle of a step finishes
// it first, so the new path starts from the cell it is stepping into.
func (z *Zone) walk(e *entity, x, y int) {
	fromX, fromY := e.x, e.y
	var current []path.Dir
	if len(e.path) > 0 {
		dx, dy := e.path[0].Step()
		fromX, fromY = e.x+dx, e.y+dy
		current = e.path[:1]
	}
	steps, ok := path.Search(z.grid, fromX, fromY, x, y, false)
	if !ok || len(steps) == 0 {
		return
	}
	if len(current) == 0 {
		e.stepStart = z.now
	}
	e.path = append(append([]path.Dir(nil), current...), steps...)
	e.walkChanged = true
}

// step advances one tick: walks, then what each player sees.
func (z *Zone) step() { z.stepAt(time.Since(z.start).Milliseconds()) }

// stepAt advances one tick to a time in zone milliseconds.
func (z *Zone) stepAt(now int64) {
	z.tick++
	z.now = now
	for e := range z.entities {
		for len(e.path) > 0 {
			d := e.path[0]
			end := e.stepStart + int64(path.StepTime(d, e.speed))
			if z.now < end {
				break
			}
			dx, dy := d.Step()
			e.x += dx
			e.y += dy
			e.dir = facing(d)
			e.path = e.path[1:]
			e.stepStart = end
		}
	}
	for e := range z.entities {
		z.update(e.player)
	}
	for e := range z.entities {
		e.walkChanged = false
	}
}

// update sends a player what changed in view this tick.
func (z *Zone) update(p *Player) {
	var events []*wathenav1.Event
	me := p.entity
	for e := range z.entities {
		visible := abs(e.x-me.x) <= ViewRange && abs(e.y-me.y) <= ViewRange
		id, known := p.netIDs[e]
		switch {
		case visible && (!known || e == me && p.fresh):
			id = p.netID(e)
			events = append(events, z.appear(id, e))
			if len(e.path) > 0 {
				events = append(events, z.move(id, e))
			}
		case visible && e.walkChanged:
			events = append(events, z.move(id, e))
		case !visible && known:
			delete(p.netIDs, e)
			events = append(events, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT))
		}
	}
	if len(events) == 0 {
		return
	}
	p.queue(wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
		Tick: z.tick, AckSeq: p.ackSeq, Events: events,
	}.Build()}.Build())
	p.fresh = false
}

func (z *Zone) appear(id uint32, e *entity) *wathenav1.Event {
	p := e.player
	return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
		Id:   id,
		Kind: wathenav1.EntityKind_ENTITY_KIND_PLAYER,
		X:    uint32(e.x),   // #nosec G115 -- map cells are at most 1024.
		Y:    uint32(e.y),   // #nosec G115 -- as above.
		Dir:  uint32(e.dir), // #nosec G115 -- 0 to 7.
		Look: wathenav1.Look_builder{
			Body: p.Look.Job, Sex: p.Look.Sex, Hair: p.Look.Hair, HairColor: p.Look.HairColor,
		}.Build(),
		Name:       p.Name,
		WalkMs:     uint32(e.speed), // #nosec G115 -- a small speed.
		HpPermille: 1000,
	}.Build()}.Build()
}

func (z *Zone) move(id uint32, e *entity) *wathenav1.Event {
	tickStart := z.now
	return wathenav1.Event_builder{Move: wathenav1.EntityMove_builder{
		Id:      id,
		FromX:   uint32(e.x), // #nosec G115 -- map cells are at most 1024.
		FromY:   uint32(e.y), // #nosec G115 -- as above.
		Steps:   PackSteps(e.path),
		StartMs: int32(e.stepStart - tickStart), // #nosec G115 -- within one step of now.
		WalkMs:  uint32(e.speed),                // #nosec G115 -- a small speed.
	}.Build()}.Build()
}

func vanish(id uint32, reason wathenav1.VanishReason) *wathenav1.Event {
	return wathenav1.Event_builder{Vanish: wathenav1.EntityVanish_builder{Id: id, Reason: reason}.Build()}.Build()
}

// queue sends a message to the player's connection. A connection that
// falls behind is dropped, so one slow client cannot stall the zone.
func (p *Player) queue(m *wathenav1.ServerMessage) {
	if p.dropped {
		return
	}
	select {
	case p.out <- m:
	default:
		p.dropped = true
		close(p.out)
	}
}

// facing turns a walk direction (0 north, counterclockwise) into the
// protocol's facing (0 south, clockwise).
func facing(d path.Dir) int { return (4 - int(d) + 8) % 8 }

// PackSteps encodes walk directions as the protocol's steps: one facing
// per 4 bits, low bits first, padded with 0xF.
func PackSteps(steps []path.Dir) []byte {
	out := make([]byte, (len(steps)+1)/2)
	for i, d := range steps {
		f := byte(facing(d)) // #nosec G115 -- 0 to 7.
		if i%2 == 0 {
			out[i/2] = f | 0xF0
		} else {
			out[i/2] = out[i/2]&0x0F | f<<4
		}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
