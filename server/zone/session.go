package zone

import (
	"sync"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Session is one player's connection to the world. It stays the same
// when the player changes maps, while each zone the player enters makes
// its own Player. The gateway sends commands through it, and reads the
// player's messages from Out.
//
// A session belongs to one zone at a time. While the player moves between
// zones, it belongs to none, and commands are dropped, as rAthena ignores
// most packets during a map change.
type Session struct {
	// Out carries messages to the connection. It closes when the player
	// leaves the world, or when a zone drops the player.
	Out <-chan *wathenav1.ServerMessage
	// Map, NetID and Tick are the map, the player's own entity ID and the
	// zone tick when the session started, for Welcome.
	Map         string
	NetID, Tick uint32

	out   chan *wathenav1.ServerMessage
	saver Saver
	// outClosed closes out once, from whichever zone or handoff ends
	// the session.
	outClosed outClosed

	mu sync.Mutex
	// zone and player are where the session is. Both are nil while the
	// player moves between zones.
	zone   *Zone
	player *Player
	// end is set once the session must end: the gateway called Leave, or
	// the saver went stale.
	end ending

	// left closes once the player's last save went to the saver.
	left     chan struct{}
	leftOnce sync.Once
}

// ending is why a session ends.
type ending int

const (
	endNone ending = iota
	// endLeave is a logout or a closed connection.
	endLeave
	// endStale is a login elsewhere that took the character over.
	endStale
)

func newSession(saver Saver) *Session {
	out := make(chan *wathenav1.ServerMessage, outBuffer)
	return &Session{Out: out, out: out, saver: saver, left: make(chan struct{})}
}

// newPlayer makes the player a session plays in one zone, entering as c.
func newPlayer(s *Session, c *character.Character) *Player {
	return &Player{Name: c.Name, Out: s.Out, out: s.out, fresh: true, load: c, sess: s}
}

// FellBehind reports whether a zone closed Out because the connection
// did not take its messages fast enough.
func (s *Session) FellBehind() bool { return s.outClosed.behind.Load() }

// done records that the session's last save went to its saver.
func (s *Session) done() {
	s.leftOnce.Do(func() { close(s.left) })
}

// where returns the zone and player the session is in, or nil while the
// player moves between zones.
func (s *Session) where() (*Zone, *Player) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.zone, s.player
}

// send queues a command in the session's zone. It returns false when the
// zone's queue is full, which the caller should treat as too many
// commands. A command sent while the player moves between zones is
// dropped.
func (s *Session) send(c command) bool {
	z, p := s.where()
	if z == nil {
		return true
	}
	c.player = p
	select {
	case z.commands <- c:
		return true
	default:
		return false
	}
}

// WalkTo asks for the player to walk to a cell. seq is the client
// message's sequence number, which frames acknowledge. It returns false
// when the zone's queue is full.
func (s *Session) WalkTo(seq uint32, x, y int) bool {
	return s.send(command{seq: seq, walkTo: &[2]int{x, y}})
}

// Ack records a processed message that needs no game action, such as a
// ping.
func (s *Session) Ack(seq uint32) bool { return s.send(command{seq: seq}) }

// Attack asks for the player to attack an entity, by the connection's ID
// for it, once or until the target dies or leaves.
func (s *Session) Attack(seq, target uint32, repeat bool) bool {
	return s.send(command{seq: seq, attack: &attackCommand{target, repeat}})
}

// PickUp asks for the player to walk to a ground item, by the
// connection's ID for it, and pick it up.
func (s *Session) PickUp(seq, item uint32) bool {
	return s.send(command{seq: seq, pickUp: &item})
}

// RaiseStat asks to spend status points on a stat, as rAthena's
// pc_statusup.
func (s *Session) RaiseStat(seq uint32, stat status.Stat, amount int) bool {
	return s.send(command{seq: seq, raise: &raiseCommand{stat, amount}})
}

// Chat says a line of local chat, as rAthena's clif_parse_GlobalMessage.
// The gateway has checked the text.
func (s *Session) Chat(seq uint32, text string) bool {
	return s.send(command{seq: seq, chat: &chatCommand{kind: chatLocal, text: text}})
}

// UseItem asks to use the item in an inventory slot, as rAthena's
// clif_parse_UseItem. The gateway has checked the slot is below
// item.MaxInventory.
func (s *Session) UseItem(seq uint32, slot int) bool {
	return s.send(command{seq: seq, use: &slot})
}

// DropItem asks to drop an amount from an inventory slot, as rAthena's
// clif_parse_DropItem. The gateway has checked the slot is below
// item.MaxInventory.
func (s *Session) DropItem(seq uint32, slot, amount int) bool {
	return s.send(command{seq: seq, drop: &dropCommand{slot, amount}})
}

// EquipItem asks to put on the item in an inventory slot, as rAthena's
// clif_parse_EquipItem. position holds the EQP_ bits asked for, or 0 for
// every position the item takes. The gateway has checked the slot is
// below item.MaxInventory.
func (s *Session) EquipItem(seq uint32, slot int, position uint32) bool {
	return s.send(command{seq: seq, equip: &equipCommand{slot, position}})
}

// UnequipItem asks to take off the item in an inventory slot, as
// clif_parse_UnequipItem.
func (s *Session) UnequipItem(seq uint32, slot int) bool {
	return s.send(command{seq: seq, unequip: &slot})
}

// InsertCard asks to put a card into an item, as clif_parse_InsertCard.
func (s *Session) InsertCard(seq uint32, cardSlot, equipSlot int) bool {
	return s.send(command{seq: seq, insertCard: &insertCardCommand{cardSlot, equipSlot}})
}

// SkillTarget is the target of a UseSkill: an entity, by the
// connection's ID, or a cell. Neither is set for a self skill.
type SkillTarget struct {
	Entity    uint32
	HasEntity bool
	X, Y      int
	HasCell   bool
}

// UseSkill casts a skill, as clif_parse_UseSkillToId and
// clif_parse_UseSkillToPos.
func (s *Session) UseSkill(seq uint32, skillID, level int, target SkillTarget) bool {
	return s.send(command{seq: seq, useSkill: &useSkillCommand{skill: skillID, level: level, target: target}})
}

// LearnSkill spends a skill point on a skill, as pc_skillup.
func (s *Session) LearnSkill(seq uint32, skillID int) bool {
	return s.send(command{seq: seq, learnSkill: &skillID})
}

// Leave removes the player from the world. Out closes. Leave returns once
// the player's last save went to its saver: from the zone the player is
// in, from the zone it was moving to, or at shutdown.
func (s *Session) Leave() {
	z, p := s.markEnd(endLeave)
	if z != nil {
		select {
		case z.leaves <- p:
		case <-z.stopped:
		}
	}
	<-s.left
}

// stale ends a session whose saver lost the character to a newer session.
// It blocks until the zone takes it or stops, so it must not run on a
// zone goroutine or a save worker.
func (s *Session) stale() {
	z, p := s.markEnd(endStale)
	if z == nil {
		return
	}
	select {
	case z.stale <- p:
	case <-z.stopped:
	}
}

// markEnd records why the session ends, unless it already ends, and
// returns where the session is.
func (s *Session) markEnd(why ending) (*Zone, *Player) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.end == endNone {
		s.end = why
	}
	return s.zone, s.player
}

// NpcClick talks to an NPC, or opens its shop, by the connection's ID
// for it, as rAthena's npc_click.
func (s *Session) NpcClick(seq, npc uint32) bool {
	return s.send(command{seq: seq, npcClick: &npc})
}

// NpcReply answers the player's open NPC dialog step, as rAthena's
// npc_scriptcont. The zone owns r afterwards.
func (s *Session) NpcReply(seq uint32, r *wathenav1.NpcReply) bool {
	return s.send(command{seq: seq, npcReply: r})
}

// BuyLine is one line of ShopBuy: an item ID and an amount.
type BuyLine struct {
	Item   uint32
	Amount int
}

// SellLine is one line of ShopSell: an inventory slot and an amount.
type SellLine struct {
	Slot, Amount int
}

// ShopBuy buys from the open shop, as rAthena's npc_buylist. The
// gateway has checked the line count and amounts.
func (s *Session) ShopBuy(seq, npc uint32, lines []BuyLine) bool {
	c := &shopBuyCommand{npc: npc}
	for _, l := range lines {
		c.lines = append(c.lines, buyLine{id: l.Item, amount: l.Amount})
	}
	return s.send(command{seq: seq, shopBuy: c})
}

// ShopSell sells to the open shop, as rAthena's npc_selllist. The
// gateway has checked the line count, slots and amounts.
func (s *Session) ShopSell(seq, npc uint32, lines []SellLine) bool {
	c := &shopSellCommand{npc: npc}
	for _, l := range lines {
		c.lines = append(c.lines, sellLine{slot: l.Slot, amount: l.Amount})
	}
	return s.send(command{seq: seq, shopSell: c})
}

// ShopClose closes the open shop window.
func (s *Session) ShopClose(seq, npc uint32) bool {
	return s.send(command{seq: seq, shopClose: &npc})
}
