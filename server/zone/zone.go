// Package zone runs one map: the players on it, their walks, and what each
// player sees. One goroutine owns all of a zone's state and advances it in
// 50 ms ticks (strategy section 4.1), so game logic needs no locks.
package zone

import (
	"context"
	"sync"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/modscript"
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

// Player is one connected player in one zone. Its Out channel is its
// session's, which carries messages to the connection.
type Player struct {
	Name string
	Look Look
	Out  <-chan *wathenav1.ServerMessage

	out    chan *wathenav1.ServerMessage
	entity *entity
	// netIDs are this connection's IDs for the entities it sees.
	netIDs  netIDs
	nextNet uint32
	ackSeq  uint32
	// fresh is set until the player's first frame, which shows the player
	// to itself.
	fresh bool
	// dropped is set once the zone closed the player's messages, or saw
	// that the sender closed them. Read it through gone.
	dropped bool
	// ownClosed is the closing state of out for a player without a
	// session, as in tests.
	ownClosed outClosed

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

	// load is the character the player enters as. When it is nil, the
	// player enters as a new character.
	load *character.Character
	// record holds the saved fields the zone does not change, such as
	// the database IDs and zeny.
	record character.Info
	// sess is the player's session, which holds its saver. It is nil for
	// players that tests add directly.
	sess *Session
	// saved is the last character handed to the saver. nextSave is when
	// the next autosave runs, in zone milliseconds.
	saved    *character.Character
	nextSave int64

	// arrival is set while a player that warped from another zone joins.
	arrival *handoff
	// mapChange is set until the first frame after a warp from another
	// zone, which starts with MapChange.
	mapChange bool
	// touchOnEnter is set until the zone checks the cell the player
	// entered on for warps, as clif_parse_LoadEndAck does.
	touchOnEnter bool
	// rewarp is rAthena's count_rewarp: warps in a row onto warp areas.
	rewarp int
}

type entity struct {
	// idx and slot are the entity's place in the zone's entitySet.
	idx, slot int32
	// player is set for players. Monsters have monster instead, items
	// on the ground have item, warp portals have warp, and NPCs that
	// mods place have npc.
	player  *Player
	monster *monster
	item    *groundItem
	warp    *Warp
	npc     *npc
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
	// packed is path in the protocol's steps, made for tick packedTick.
	// Every viewer's move event shares it.
	packed     []byte
	packedTick uint32
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
	entities entitySet
	// blocks finds the entities near a player during updates.
	blocks blockIndex
	// viewsForTest, when set, replaces viewEvents. A test checks
	// viewEvents against a scan of every entity with it.
	viewsForTest func(p *Player, events []*wathenav1.Event) []*wathenav1.Event
	rand         *rng.Rand
	combat

	// Status holds the job tables players' stats come from. Set it before
	// Run. Without it, every player has PlayerStatus.
	Status *status.Tables
	// Start is the character every new character starts as: its job,
	// raised stats and skills. Its levels and sex are unused, since new
	// characters start at level 1 with the sex of their look. Set it
	// before Run.
	Start status.Character
	// PlayerStatus is every player's battle status when Status is nil.
	PlayerStatus battle.Status
	// Progress holds the exp tables and drop settings. Without it, kills
	// give no exp and no items. Set it before Run.
	Progress *progression.Tables
	// Items is the item database. Without it, every item is an Etc item
	// with no weight. Set it before Run.
	Items *item.Table
	// StartItems are items every new character starts with, for
	// development. Set it before Run.
	StartItems []StartItem
	// SaveInterval is how often a player's changed character is saved.
	// It is DefaultSaveInterval when 0. Set it before Run.
	SaveInterval time.Duration
	// World finds the zones that warps lead to. Without it, only warps
	// within the map work. Set it before Run.
	World Directory
	// Metrics measures the zone's ticks. New makes it.
	Metrics *metrics.Zone
	// Mods are the mods' scripts and item names. Without them, no
	// scripts run. Set it before Run.
	Mods *Mods

	// scripts runs the mods' Lua in the zone's own state, made by Run.
	// scriptEvents wait for a point where scripts may run.
	scripts      *modscript.Runtime
	scriptEvents []scriptEvent
	// monsterHandles are the monsters that script handles point to, by
	// handle ID. A killed monster stays here until its event has run.
	monsterHandles    map[int64]*entity
	nextMonsterHandle int64
	// control gets work that must run on the zone's goroutine, such as a
	// mod reload.
	control chan func()

	// warps are the map's warps, in load order.
	warps []Warp
	// asleep is set while the zone has no players and its ticker is off.
	asleep bool

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
	// arrivals get players that warped from other zones. incoming counts
	// the sends to it that have not finished.
	arrivals chan *handoff
	incoming sync.WaitGroup
	// stale gets players whose saver lost the character to a newer
	// session.
	stale chan *Player
	// stopped closes when Run returns, after the last saves.
	stopped chan struct{}

	// outbox holds messages for the sender, which runs while Run runs.
	// See outbox.go.
	outbox     []outMsg
	sends      chan []outMsg
	spare      chan []outMsg
	senderDone chan struct{}
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
		players:  map[uint32]*Player{},
		joins:    make(chan joinRequest),
		leaves:   make(chan *Player, 16),
		commands: make(chan command, 1024),
		arrivals: make(chan *handoff),
		stale:    make(chan *Player),
		control:  make(chan func()),
		stopped:  make(chan struct{}),
		Metrics:  &metrics.Zone{Name: mapName},
	}
	z.spawn(spawns)
	return z
}

// Run advances the zone until ctx ends. It then hands every player's
// character to its saver and drops the players.
//
// A zone without players sleeps: its ticker stops, so its monsters stand
// still and it costs no CPU. It wakes when a player joins or arrives.
// rAthena similarly runs full monster AI only near players, and lets
// monsters no player has seen stand still (mob_ai_sub_lazy).
func (z *Zone) Run(ctx context.Context) {
	defer close(z.stopped)
	z.start = time.Now()
	z.startSender()
	z.startScripts()
	defer z.stopScripts()
	var t *time.Ticker
	var ticks <-chan time.Time
	defer func() {
		if t != nil {
			t.Stop()
		}
	}()
	z.asleep = true
	for {
		switch {
		case len(z.players) > 0 && ticks == nil:
			t = time.NewTicker(Tick)
			ticks = t.C
		case len(z.players) == 0 && ticks != nil:
			t.Stop()
			t, ticks = nil, nil
			z.asleep = true
			z.Metrics.Asleep()
		}
		// Handlers of what the last event did run now, and what they
		// queued goes to the sender in one batch.
		z.runScripts()
		z.flush()
		select {
		case <-ctx.Done():
			for _, p := range z.players {
				z.save(p, true)
				z.drop(p)
			}
			z.stopSender()
			return
		case j := <-z.joins:
			began := time.Now()
			z.wake()
			j.done <- z.join(j.player)
			z.Metrics.Handled(time.Since(began))
		case h := <-z.arrivals:
			began := time.Now()
			z.wake()
			z.arrive(h)
			z.Metrics.Handled(time.Since(began))
		case p := <-z.leaves:
			began := time.Now()
			z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
			z.Metrics.Handled(time.Since(began))
		case p := <-z.stale:
			z.loggedInElsewhere(p)
		case f := <-z.control:
			f()
		case c := <-z.commands:
			began := time.Now()
			z.handle(c)
			z.Metrics.Handled(time.Since(began))
		case <-ticks:
			began := time.Now()
			z.step()
			z.Metrics.Tick(time.Since(began), len(z.players))
		}
	}
}

// wake brings a sleeping zone's clock to now before a player enters.
// Monsters pick new wander times, and natural regen starts again, so the
// time asleep does not count.
func (z *Zone) wake() {
	if !z.asleep {
		return
	}
	z.asleep = false
	now := time.Since(z.start).Milliseconds()
	if z.scripts != nil {
		// Mod timers follow the zone clock, which stood still.
		z.scripts.Shift(now - z.now)
	}
	z.now = now
	z.prevHeal, z.nextHeal = z.now, z.now+naturalHealInterval
	for e := range z.entities.all() {
		if e.monster != nil && len(e.path) == 0 {
			e.monster.nextWalk = -1
		}
	}
}

// WaitIncoming waits until no player is on its way to the zone. Call it
// after every zone of the world has stopped: players on their way to a
// stopped zone get their last save then.
func (z *Zone) WaitIncoming() { z.incoming.Wait() }

// Join adds a new character that only lives in memory, at the walkable
// cell nearest the map's center.
func (z *Zone) Join(ctx context.Context, name string, look Look) (*Session, error) {
	c := z.NewCharacter(name, look)
	return z.JoinCharacter(ctx, &c, nil)
}

// JoinCharacter adds a player as a loaded character, and returns its
// session, which holds the player's own entity ID and the current tick.
// saver, when not nil, saves the character while the player is in the
// world and when it leaves. Zones never wait on saver.
func (z *Zone) JoinCharacter(ctx context.Context, c *character.Character, saver Saver) (*Session, error) {
	s := newSession(saver)
	p := newPlayer(s, c)
	s.zone, s.player = z, p
	if n, ok := saver.(StaleNotifier); ok {
		n.OnStale(s.stale)
	}
	done := make(chan joinResult, 1)
	select {
	case z.joins <- joinRequest{p, done}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-z.stopped:
		return nil, ErrStopped
	}
	r := <-done
	s.Map, s.NetID, s.Tick = z.Map, r.netID, r.tick
	return s, nil
}

func (z *Zone) join(p *Player) joinResult {
	c := p.load
	if c == nil {
		n := z.NewCharacter(p.Name, p.Look)
		c = &n
	}
	p.load = nil
	p.record = c.Info
	p.Look = Look{Job: uint32(c.Job), Sex: c.Sex(), Hair: uint32(c.Hair), HairColor: uint32(c.HairColor)} // #nosec G115 -- IDs from the data tables, which the database keeps at 0 or more.
	x, y := c.X, c.Y
	h := p.arrival
	p.arrival = nil
	switch {
	case h != nil:
		// A warp places the player as pc_setpos does.
		x, y = z.arrivalCell(x, y)
		p.ackSeq, p.rewarp, p.mapChange = h.ackSeq, h.rewarp, true
	case c.Map != z.Map || c.HP <= 0 || z.grid.NoPass(x, y):
		// A character from another map, one that logged out dead, or
		// one on a cell that is no longer walkable starts at the start
		// cell, as a revived player does.
		x, y = z.startCell()
	}
	e := &entity{player: p, x: x, y: y, speed: WalkSpeed}
	p.entity = e
	z.entities.add(e)
	z.nextChar++
	p.charID = z.nextChar
	z.players[p.charID] = p
	p.inventory = restoreInventory(z.Items, c)
	// pc_authok starts the regen waits at login.
	p.regenTicks = regenState{hp: z.now, sp: z.now}
	p.itemDelays = map[uint32]int64{}
	p.statusDelays = map[string]int64{}
	if h != nil {
		// Item waits go on across the map change.
		p.canUseItem = z.now + h.canUseItem
		for id, left := range h.itemDelays {
			p.itemDelays[id] = z.now + left
		}
		for st, left := range h.statusDelays {
			p.statusDelays[st] = z.now + left
		}
	}
	p.sent = map[wathenav1.StatId]int64{}
	p.char = z.Start
	p.char.Job, p.char.Male, p.char.Stats = c.Job, c.Male, c.Stats
	if z.Progress != nil {
		p.progress = progression.Character{
			Job: c.Job, BaseLevel: c.BaseLevel, JobLevel: c.JobLevel,
			BaseExp: c.BaseExp, JobExp: c.JobExp,
			StatusPoints: c.StatusPoints, SkillPoints: c.SkillPoints,
		}
		// The first frame carries the levels and exp.
		p.pending = append(p.pending, z.progressStats(p, nil))
	}
	z.recalc(p)
	// HP and SP are the saved ones, up to their maximum. A character that
	// logged out dead comes back with full HP, as a revived player does.
	// hpChanged sends the player its HP in the first frame.
	e.hp, p.sp = min(c.HP, int64(e.status.MaxHP)), min(c.SP, int64(e.status.MaxSP))
	if c.HP <= 0 {
		e.hp = int64(e.status.MaxHP)
	}
	e.hpChanged = e.status.MaxHP > 0
	if e.status.Speed > 0 {
		e.speed = e.status.Speed
	}
	z.updateWeight(p)
	p.pending = append(p.pending, inventoryList(p))
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
	// The first save comes one interval after the player enters. A
	// warp saves at once, as the map change checkpoint of strategy
	// section 3.8.
	if s := p.sess; s != nil && s.saver != nil {
		p.saved = z.snapshot(p)
		if h != nil {
			s.saver.Save(p.saved, false)
		}
	}
	p.nextSave = z.now + z.saveInterval().Milliseconds()
	if h == nil {
		p.rewarp = 0
		p.touchOnEnter = true
		z.scriptEventFor("login", modscript.Player(int64(p.charID)))
	} else {
		z.entered(p)
	}
	z.scriptEventFor("map_enter", modscript.Player(int64(p.charID)))
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
	if id, ok := p.netIDs.get(e); ok {
		return id
	}
	p.nextNet++
	p.netIDs.set(e, p.nextNet)
	return p.nextNet
}

func (z *Zone) leave(p *Player, reason wathenav1.VanishReason) {
	if p.entity == nil || !z.entities.has(p.entity) {
		return
	}
	if reason == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT {
		// Handlers see the player before it goes.
		z.scriptEventFor("logout", modscript.Player(int64(p.charID)))
		z.runScripts()
	}
	z.save(p, true)
	z.remove(p, reason)
	z.drop(p)
}

// remove takes a player out of the zone. Viewers see it vanish for a
// reason.
func (z *Zone) remove(p *Player, reason wathenav1.VanishReason) {
	z.entities.remove(p.entity)
	delete(z.players, p.charID)
	z.forget(p.entity)
	for e := range z.entities.all() {
		if e.player == nil {
			continue
		}
		if id, ok := e.player.netIDs.get(p.entity); ok {
			e.player.netIDs.del(p.entity)
			z.queue(e.player, wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
				Tick: z.tick, AckSeq: e.player.ackSeq, Events: []*wathenav1.Event{vanish(id, reason)},
			}.Build()}.Build())
		}
	}
}

// drop closes a player's messages. Its connection then closes.
func (z *Zone) drop(p *Player) {
	if !p.dropped {
		p.dropped = true
		z.closeOut(p)
	}
}

func (z *Zone) handle(c command) {
	p := c.player
	if p.gone() || !z.entities.has(p.entity) {
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
		for e, id := range p.netIDs.all() {
			if id == c.attack.target && e != me && e.status != nil && !e.dead {
				me.target, me.repeat, me.pickItem = e, c.attack.repeat, nil
			}
		}
	}
	if c.pickUp != nil {
		z.startPickUp(p, *c.pickUp)
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
	began := time.Now()
	z.entities.compact()
	z.tick++
	z.now = now
	var warped []warpTouch
	for e := range z.entities.all() {
		touched := false
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
			// unit_walktoxy_timer touches the warp on each cell a
			// player enters. A warp that takes the player ends the
			// walk.
			if p := e.player; p != nil {
				if w := z.warpAt(e.x, e.y); w != nil && z.target(p, w) != nil {
					e.path = e.path[:0]
					warped = append(warped, warpTouch{p, w})
					touched = true
				}
			}
		}
		// clif_parse_LoadEndAck touches the cell a player entered on,
		// once its first frame went out.
		if p := e.player; p != nil && p.touchOnEnter && !p.fresh {
			p.touchOnEnter = false
			if w := z.warpAt(e.x, e.y); w != nil && !touched && len(e.path) == 0 && z.target(p, w) != nil {
				warped = append(warped, warpTouch{p, w})
			}
		}
	}
	for _, t := range warped {
		z.warp(t.p, t.w)
	}
	moved := time.Now()
	z.Metrics.Phase(metrics.PhaseMove, moved.Sub(began))
	z.revive()
	z.think()
	z.fight()
	z.pickUps()
	z.dropItems()
	z.expireItems()
	z.naturalHeal()
	z.autosave()
	z.runScripts()
	if z.scripts != nil {
		z.scripts.Advance(z.now)
	}
	acted := time.Now()
	z.Metrics.Phase(metrics.PhaseAct, acted.Sub(moved))
	if len(z.players) > 0 {
		z.blocks.build(z.grid.Width, z.grid.Height, &z.entities)
	}
	for e := range z.entities.all() {
		if e.player != nil {
			z.update(e.player)
		}
	}
	z.Metrics.Phase(metrics.PhaseSend, time.Since(acted))
	for e := range z.entities.all() {
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

// warpTouch is a player that stepped onto a warp this tick.
type warpTouch struct {
	p *Player
	w *Warp
}

// update sends a player what changed in view this tick.
func (z *Zone) update(p *Player) {
	var events []*wathenav1.Event
	if p.mapChange {
		p.mapChange = false
		events = append(events, z.mapChange(p))
	}
	if z.viewsForTest != nil {
		events = z.viewsForTest(p, events)
	} else {
		events = z.viewEvents(p, events)
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
	z.queue(p, wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
		Tick: z.tick, AckSeq: p.ackSeq, Events: events,
	}.Build()}.Build())
	p.fresh = false
}

// viewEvents adds the appear, move, stop and vanish events of what the
// player sees this tick.
func (z *Zone) viewEvents(p *Player, events []*wathenav1.Event) []*wathenav1.Event {
	me := p.entity
	// Entities in view: those of the blocks the view touches. No entity
	// moves while players update, so the blocks built for this tick
	// hold every entity where it is.
	bx0, bx1 := span(me.x, ViewRange, z.blocks.w)
	by0, by1 := span(me.y, ViewRange, z.blocks.h)
	for by := by0; by <= by1; by++ {
		for _, block := range z.blocks.blocks[by*z.blocks.w+bx0 : by*z.blocks.w+bx1+1] {
			for _, e := range block {
				if abs(e.x-me.x) > ViewRange || abs(e.y-me.y) > ViewRange {
					continue
				}
				id, known := p.netIDs.get(e)
				switch {
				case !known || e == me && p.fresh:
					id = p.netID(e)
					events = append(events, z.appear(id, e))
					if len(e.path) > 0 {
						events = append(events, z.move(id, e))
					}
				case e.walkChanged:
					events = append(events, z.move(id, e))
				case e.stopped:
					events = append(events, stop(id, e))
				}
			}
		}
	}
	// Entities the player knew that are still in the zone and went out
	// of view.
	for i := 0; i < len(p.netIDs.known); {
		e := p.netIDs.known[i]
		if (abs(e.x-me.x) <= ViewRange && abs(e.y-me.y) <= ViewRange) || !z.entities.has(e) {
			i++
			continue
		}
		// del moves the last known entity to i.
		id := p.netIDs.id(e)
		p.netIDs.del(e)
		events = append(events, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT))
	}
	return events
}

func (z *Zone) appear(id uint32, e *entity) *wathenav1.Event {
	if e.npc != nil {
		return z.npcAppear(id, e)
	}
	if e.warp != nil {
		// A warp portal is an NPC with the warp sprite, as rAthena's
		// JT_WARPNPC. It has no name to show.
		return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
			Id:   id,
			Kind: wathenav1.EntityKind_ENTITY_KIND_NPC,
			X:    uint32(e.x), // #nosec G115 -- map cells are at most 1024.
			Y:    uint32(e.y), // #nosec G115 -- as above.
			Look: wathenav1.Look_builder{Body: WarpSprite}.Build(),
		}.Build()}.Build()
	}
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
	// Paths do not change while players update, so the steps are packed
	// once per tick. Messages only read them.
	if e.packed == nil || e.packedTick != z.tick {
		e.packed, e.packedTick = PackSteps(e.path), z.tick
	}
	tickStart := z.now
	return wathenav1.Event_builder{Move: wathenav1.EntityMove_builder{
		Id:      id,
		FromX:   uint32(e.x), // #nosec G115 -- map cells are at most 1024.
		FromY:   uint32(e.y), // #nosec G115 -- as above.
		Steps:   e.packed,
		StartMs: int32(e.stepStart - tickStart), // #nosec G115 -- within one step of now.
		WalkMs:  uint32(e.speed),                // #nosec G115 -- a small speed.
	}.Build()}.Build()
}

func vanish(id uint32, reason wathenav1.VanishReason) *wathenav1.Event {
	return wathenav1.Event_builder{Vanish: wathenav1.EntityVanish_builder{Id: id, Reason: reason}.Build()}.Build()
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
