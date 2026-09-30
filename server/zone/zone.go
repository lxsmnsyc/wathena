// Package zone runs one map: the players on it, their walks, and what each
// player sees. One goroutine owns all of a zone's state and advances it in
// 50 ms ticks (strategy section 4.1), so game logic needs no locks.
package zone

import (
	"context"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
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

	// charID stands for rAthena's char_id in damage logs and pick-up
	// rights, until characters are stored. It is unique within the zone.
	charID uint32
	// progress is the player's levels and exp. It is only used when the
	// zone has progression tables.
	progress progression.Character
	// char is what the player's stats come from. Its levels are unused:
	// they come from progress.
	char status.Character
	// sp is the player's SP. HP is on its entity.
	sp int64
	// maxWeight and window are from the last stat calculation.
	maxWeight int
	window    status.Window
	// sent holds the stat values last sent to the player.
	sent map[wathenav1.StatId]int64
	// inventory is the player's items and their weight.
	inventory *item.Inventory
	// overweight is the level pc_updateweightstatus last set, from
	// item.WeightNormal to item.Weight90.
	overweight int
	// canUseItem is canuseitem_tick: when the player may use an item
	// again, in zone milliseconds.
	canUseItem int64
	// itemDelays and statusDelays are when item reuse delays end, by
	// item ID and by delay status, as item_delay and the Reuse_Limit
	// statuses.
	itemDelays   map[uint32]int64
	statusDelays map[string]int64
	// regen is what natural regen heals, from the last stat calculation.
	regen      status.Regen
	regenTicks regenState
	// pending are events for the player alone, sent with the next frame.
	pending []*wathenav1.Event
}

type entity struct {
	// player is set for players. Monsters have monster instead, and items
	// on the ground have item.
	player  *Player
	monster *monster
	item    *groundItem
	x, y    int
	// dir is the facing, 0 south, clockwise.
	dir   int
	speed int
	// path holds the steps left. stepStart is when path[0] began, in zone
	// milliseconds.
	path      []path.Dir
	stepStart int64
	// walkChanged is set when a walk starts or changes, so viewers get a
	// move event. stopped is set when a walk ends early.
	walkChanged bool
	stopped     bool
	fight
	pickUp
}

type command struct {
	player *Player
	seq    uint32
	walkTo *[2]int
	attack *attackCommand
	pickUp *uint32
	raise  *raiseCommand
	chat   *string
	use    *int
	drop   *dropCommand
}

type raiseCommand struct {
	stat   status.Stat
	amount int
}

type attackCommand struct {
	target uint32
	repeat bool
}

// Zone is one running map.
type Zone struct {
	Map  string
	grid *path.Grid

	start    time.Time
	now      int64
	tick     uint32
	entities map[*entity]bool
	rand     *rng.Rand
	combat

	// Status holds the job tables players' stats come from. Set it before
	// Run. Without it, every player has PlayerStatus.
	Status *status.Tables
	// Start is the character every player starts as, until characters
	// are stored: its job, sex, raised stats and skills. Its levels are
	// unused, since players start at level 1. Set it before Run.
	Start status.Character
	// PlayerStatus is every player's battle status when Status is nil.
	PlayerStatus battle.Status
	// Progress holds the exp tables and drop settings. Without it, kills
	// give no exp and no items. Set it before Run.
	Progress *progression.Tables
	// Items is the item database. Without it, every item is an Etc item
	// with no weight. Set it before Run.
	Items *item.Table
	// StartItems are items every player starts with, for development.
	// Set it before Run.
	StartItems []StartItem

	// players maps charID to the players in the zone.
	players  map[uint32]*Player
	nextChar uint32
	loot
	// chats are this tick's local chat lines.
	chats []chatLine
	// used are this tick's item uses.
	used []used
	// prevHeal and nextHeal are when natural regen last ran and next
	// runs, in zone milliseconds.
	prevHeal, nextHeal int64

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

// New makes a zone for a map, given its asset name and cells. rand is the
// zone's random stream, which must be secret, since it decides where
// monsters go. spawns are the map's monsters.
func New(mapName string, grid *path.Grid, rand *rng.Rand, spawns []Spawn) *Zone {
	z := &Zone{
		Map:      mapName,
		grid:     grid,
		rand:     rand,
		entities: map[*entity]bool{},
		players:  map[uint32]*Player{},
		joins:    make(chan joinRequest),
		leaves:   make(chan *Player, 16),
		commands: make(chan command, 1024),
	}
	z.spawn(spawns)
	return z
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
				if e.player != nil {
					z.drop(e.player)
				}
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
	z.nextChar++
	p.charID = z.nextChar
	z.players[p.charID] = p
	p.inventory = item.NewInventory()
	// pc_authok starts the regen waits at login.
	p.regenTicks = regenState{hp: z.now, sp: z.now}
	p.itemDelays = map[uint32]int64{}
	p.statusDelays = map[string]int64{}
	p.sent = map[wathenav1.StatId]int64{}
	p.char = z.Start
	if t := z.Progress; t != nil {
		// A new character at level 1. The first frame carries its levels
		// and exp.
		p.progress = t.NewCharacter(p.char.Job)
		p.pending = append(p.pending, z.progressStats(p, nil))
	}
	z.recalc(p)
	// A new character has full HP and SP. hpChanged sends the player its
	// HP in the first frame.
	e.hp, p.sp = int64(e.status.MaxHP), int64(e.status.MaxSP)
	e.hpChanged = e.status.MaxHP > 0
	if e.status.Speed > 0 {
		e.speed = e.status.Speed
	}
	z.giveStartItems(p)
	p.pending = append(p.pending, inventoryList(p))
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
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
	delete(z.players, p.charID)
	z.forget(p.entity)
	for e := range z.entities {
		if e.player == nil {
			continue
		}
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
	if c.raise != nil {
		// rAthena lets a dead player raise stats.
		z.raiseStat(p, c.raise.stat, c.raise.amount)
	}
	if c.chat != nil {
		// Dead players can talk too: clif_process_message only checks
		// for mutes.
		z.chats = append(z.chats, chatLine{p.entity, *c.chat})
	}
	me := p.entity
	if me.dead {
		return
	}
	if c.walkTo != nil {
		me.target, me.pickItem = nil, nil
		z.walk(me, c.walkTo[0], c.walkTo[1])
	}
	if c.use != nil {
		z.useItem(p, *c.use)
	}
	if c.drop != nil {
		z.dropItem(p, c.drop.slot, c.drop.amount)
	}
	// The game client does not attack while SC_WEIGHT90 is on, which
	// rAthena relies on. The zone checks it itself.
	if c.attack != nil && p.overweight != item.Weight90 {
		for e, id := range p.netIDs {
			if id == c.attack.target && e != me && e.status != nil && !e.dead {
				me.target, me.repeat, me.pickItem = e, c.attack.repeat, nil
			}
		}
	}
	if c.pickUp != nil {
		z.startPickUp(p, *c.pickUp)
	}
}

// Attack asks for the player to attack an entity, by the connection's ID
// for it, once or until the target dies or leaves. It returns false when
// the zone's queue is full.
func (z *Zone) Attack(p *Player, seq, target uint32, repeat bool) bool {
	select {
	case z.commands <- command{player: p, seq: seq, attack: &attackCommand{target, repeat}}:
		return true
	default:
		return false
	}
}

// PickUp asks for the player to walk to a ground item, by the
// connection's ID for it, and pick it up. It returns false when the zone's
// queue is full.
func (z *Zone) PickUp(p *Player, seq, item uint32) bool {
	select {
	case z.commands <- command{player: p, seq: seq, pickUp: &item}:
		return true
	default:
		return false
	}
}

// RaiseStat asks to spend status points on a stat, as rAthena's
// pc_statusup. It returns false when the zone's queue is full.
func (z *Zone) RaiseStat(p *Player, seq uint32, stat status.Stat, amount int) bool {
	select {
	case z.commands <- command{player: p, seq: seq, raise: &raiseCommand{stat, amount}}:
		return true
	default:
		return false
	}
}

// Chat says a line of local chat, as rAthena's clif_parse_GlobalMessage.
// The gateway has checked the text. It returns false when the zone's
// queue is full.
func (z *Zone) Chat(p *Player, seq uint32, text string) bool {
	select {
	case z.commands <- command{player: p, seq: seq, chat: &text}:
		return true
	default:
		return false
	}
}

// UseItem asks to use the item in an inventory slot, as rAthena's
// clif_parse_UseItem. The gateway has checked the slot is below
// item.MaxInventory. It returns false when the zone's queue is full.
func (z *Zone) UseItem(p *Player, seq uint32, slot int) bool {
	select {
	case z.commands <- command{player: p, seq: seq, use: &slot}:
		return true
	default:
		return false
	}
}

// DropItem asks to drop an amount from an inventory slot, as rAthena's
// clif_parse_DropItem. The gateway has checked the slot is below
// item.MaxInventory. It returns false when the zone's queue is full.
func (z *Zone) DropItem(p *Player, seq uint32, slot, amount int) bool {
	select {
	case z.commands <- command{player: p, seq: seq, drop: &dropCommand{slot, amount}}:
		return true
	default:
		return false
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
	z.revive()
	z.think()
	z.fight()
	z.pickUps()
	z.dropItems()
	z.expireItems()
	z.naturalHeal()
	for e := range z.entities {
		if e.player != nil {
			z.update(e.player)
		}
	}
	for e := range z.entities {
		e.walkChanged = false
		e.stopped = false
		e.hpChanged = false
		if e.item != nil {
			e.item.fresh = false
		}
	}
	z.hits = z.hits[:0]
	z.removed = z.removed[:0]
	z.takes = z.takes[:0]
	z.effects = z.effects[:0]
	z.used = z.used[:0]
	clear(z.chats)
	z.chats = z.chats[:0]
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
		case visible && e.stopped:
			events = append(events, stop(id, e))
		case !visible && known:
			delete(p.netIDs, e)
			events = append(events, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT))
		}
	}
	events = append(events, z.combatEvents(p)...)
	events = append(events, z.effectEvents(p)...)
	events = append(events, z.usedEvents(p)...)
	events = append(events, z.chatEvents(p)...)
	events = append(events, p.pending...)
	p.pending = nil
	if len(events) == 0 {
		return
	}
	p.queue(wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
		Tick: z.tick, AckSeq: p.ackSeq, Events: events,
	}.Build()}.Build())
	p.fresh = false
}

func (z *Zone) appear(id uint32, e *entity) *wathenav1.Event {
	if it := e.item; it != nil {
		return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
			Id:   id,
			Kind: wathenav1.EntityKind_ENTITY_KIND_ITEM,
			X:    uint32(e.x), // #nosec G115 -- map cells are at most 1024.
			Y:    uint32(e.y), // #nosec G115 -- as above.
			Item: wathenav1.GroundItem_builder{
				ItemId:  it.id,
				Amount:  uint32(it.amount), // #nosec G115 -- a stack of at most 30000.
				SubX:    uint32(it.subX),   // #nosec G115 -- 0 to 15.
				SubY:    uint32(it.subY),   // #nosec G115 -- as above.
				Dropped: it.fresh,
			}.Build(),
		}.Build()}.Build()
	}
	if m := e.monster; m != nil {
		return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
			Id:         id,
			Kind:       wathenav1.EntityKind_ENTITY_KIND_MONSTER,
			X:          uint32(e.x),   // #nosec G115 -- map cells are at most 1024.
			Y:          uint32(e.y),   // #nosec G115 -- as above.
			Dir:        uint32(e.dir), // #nosec G115 -- 0 to 7.
			Look:       wathenav1.Look_builder{Body: m.spawn.Mob}.Build(),
			Name:       m.spawn.Name,
			WalkMs:     uint32(e.speed), // #nosec G115 -- a small speed.
			HpPermille: e.hpPermille(),
		}.Build()}.Build()
	}
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
		HpPermille: e.hpPermille(),
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
