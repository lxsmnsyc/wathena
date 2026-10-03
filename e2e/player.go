package e2e

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// View and Game are what a bot knows, for the checks of Player.Until.
type (
	View = botclient.View
	Game = botclient.Game
)

// DefaultTimeout is how long a step waits for the server.
const DefaultTimeout = 15 * time.Second

// Player is a bot that a test drives. Each method sends a command and
// waits for the events that show the result. A step that fails or times
// out fails the test. Methods may be called from the test's goroutine
// only; RunBots gives each bot its own Player.
type Player struct {
	*botclient.Bot
	t testing.TB
	// Timeout is how long each step waits. It is DefaultTimeout.
	Timeout time.Duration
	// mark is the index of the first event the next step looks at.
	mark int
}

// Join connects a player whose name is its --dev session token.
func (s *Server) Join(t testing.TB, name string) *Player {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	b, err := botclient.Dial(ctx, s.WS, []byte(name), botclient.Options{Record: true})
	if err != nil {
		t.Fatalf("%s cannot join: %v", name, err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return &Player{Bot: b, t: t, Timeout: DefaultTimeout}
}

func (p *Player) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.Timeout)
}

// send runs a command and fails the test on an error.
func (p *Player) send(what string, f func(ctx context.Context) error) {
	p.t.Helper()
	ctx, cancel := p.ctx()
	defer cancel()
	if err := f(ctx); err != nil {
		p.t.Fatalf("%s: %s: %v", p.Name, what, err)
	}
}

// Expect waits for an event from the player's mark on that match
// accepts, moves the mark past it, and returns it.
func (p *Player) Expect(what string, match func(*wathenav1.Event) bool) *wathenav1.Event {
	p.t.Helper()
	ctx, cancel := p.ctx()
	defer cancel()
	e, next, err := p.Wait(ctx, p.mark, match)
	if err != nil {
		p.t.Fatalf("%s: waiting for %s: %v", p.Name, what, err)
	}
	p.mark = next
	return e
}

// Until waits until the player's state passes a check.
func (p *Player) Until(what string, ok func(botclient.View, botclient.Game) bool) {
	p.t.Helper()
	ctx, cancel := p.ctx()
	defer cancel()
	if err := p.WaitState(ctx, ok); err != nil {
		v := p.View()
		p.t.Fatalf("%s: waiting until %s: %v (on %s at %d,%d, walking %v)", p.Name, what, err, v.Map, v.X, v.Y, v.Walking)
	}
}

// Seen returns the first event so far that match accepts, or nil. It
// does not wait and does not move the mark.
func (p *Player) Seen(match func(*wathenav1.Event) bool) *wathenav1.Event {
	events, _ := p.Events(0)
	for _, e := range events {
		if match(e) {
			return e
		}
	}
	return nil
}

// Skip moves the mark past every event so far.
func (p *Player) Skip() { p.mark = p.Mark() }

// Self is the player's own entity ID on its map.
func (p *Player) Self() uint32 { return p.View().NetID }

// Find waits until an entity of a kind with a name is in view, and
// returns the nearest.
func (p *Player) Find(kind wathenav1.EntityKind, name string) botclient.Entity {
	p.t.Helper()
	var found botclient.Entity
	p.Until(fmt.Sprintf("a %s named %q is in view", kind, name), func(v botclient.View, _ botclient.Game) bool {
		best := -1
		for _, e := range v.Entities {
			if e.Kind != kind || e.Name != name {
				continue
			}
			if d := max(abs(e.X-v.X), abs(e.Y-v.Y)); best < 0 || d < best {
				best, found = d, e
			}
		}
		return best >= 0
	})
	return found
}

// FindItem waits until a ground item is in view, and returns the
// nearest.
func (p *Player) FindItem(id uint32) botclient.Entity {
	p.t.Helper()
	var found botclient.Entity
	p.Until(fmt.Sprintf("item %d lies in view", id), func(v botclient.View, _ botclient.Game) bool {
		best := -1
		for _, e := range v.Entities {
			if e.Kind != wathenav1.EntityKind_ENTITY_KIND_ITEM || e.ItemID != id {
				continue
			}
			if d := max(abs(e.X-v.X), abs(e.Y-v.Y)); best < 0 || d < best {
				best, found = d, e
			}
		}
		return best >= 0
	})
	return found
}

// WalkTo walks to a cell and waits until the player stands on it.
func (p *Player) WalkTo(x, y int) {
	p.t.Helper()
	p.send("walk", func(ctx context.Context) error { return p.Bot.WalkTo(ctx, x, y) })
	p.Until(fmt.Sprintf("standing on %d,%d", x, y), func(v botclient.View, _ botclient.Game) bool {
		return v.X == x && v.Y == y && !v.Walking
	})
}

// Say says a line of local chat and waits until the player hears it
// back.
func (p *Player) Say(text string) {
	p.t.Helper()
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, text) })
	p.Hears(p.Name, text)
}

// Hears waits for a line of local chat from a speaker.
func (p *Player) Hears(from, text string) *wathenav1.ChatMessage {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("%s to say %q", from, text), func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL && c.GetFromName() == from && strings.HasSuffix(c.GetText(), text)
	}).GetChat()
}

// Kill attacks a monster until it dies, and returns the damage the
// player dealt it.
func (p *Player) Kill(target botclient.Entity) int64 {
	p.t.Helper()
	p.send("attack", func(ctx context.Context) error { return p.Attack(ctx, target.ID, true) })
	var dealt int64
	p.Expect(fmt.Sprintf("%s to die", target.Name), func(e *wathenav1.Event) bool {
		if d := e.GetDamage(); d != nil && d.GetSource() == p.Self() && d.GetTarget() == target.ID {
			dealt += d.GetAmount()
		}
		v := e.GetVanish()
		return v != nil && v.GetId() == target.ID && v.GetReason() == wathenav1.VanishReason_VANISH_REASON_DIED
	})
	return dealt
}

// PickUp picks up a ground item and returns the InventoryAdd the
// player got for it.
func (p *Player) PickUp(it botclient.Entity) *wathenav1.InventoryAdd {
	p.t.Helper()
	p.send("pick up", func(ctx context.Context) error { return p.Bot.PickUp(ctx, it.ID) })
	add := p.Expect(fmt.Sprintf("item %d in the inventory", it.ItemID), func(e *wathenav1.Event) bool {
		if f := e.GetPickUpFailed(); f != nil && f.GetItem() == it.ID {
			p.t.Fatalf("%s: picking up item %d failed: %s", p.Name, it.ItemID, f.GetReason())
		}
		a := e.GetInventoryAdd()
		return a != nil && a.GetItemId() == it.ItemID
	}).GetInventoryAdd()
	if int(add.GetAmount()) != it.Amount {
		p.t.Errorf("%s: picked up %d of item %d, the ground showed %d", p.Name, add.GetAmount(), it.ItemID, it.Amount)
	}
	return add
}

// Use uses the first slot that holds an item, and waits until the
// server shows the use and takes the item.
func (p *Player) Use(id uint32) {
	p.t.Helper()
	slot := p.Game().SlotOf(id)
	if slot < 0 {
		p.t.Fatalf("%s has no item %d to use", p.Name, id)
	}
	p.send("use", func(ctx context.Context) error { return p.UseItem(ctx, slot) })
	used, removed := false, false
	p.Expect(fmt.Sprintf("item %d to be used", id), func(e *wathenav1.Event) bool {
		if f := e.GetUseItemFailed(); f != nil {
			p.t.Fatalf("%s: using item %d failed: %s", p.Name, id, f.GetReason())
		}
		if u := e.GetItemUsed(); u != nil && u.GetSource() == p.Self() && u.GetItemId() == id {
			used = true
		}
		if r := e.GetInventoryRemove(); r != nil && int(r.GetSlot()) == slot && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED {
			removed = true
		}
		return used && removed
	})
}

// Drop drops an amount from the first slot that holds an item, and
// waits until it leaves the inventory.
func (p *Player) Drop(id uint32, amount int) {
	p.t.Helper()
	slot := p.Game().SlotOf(id)
	if slot < 0 {
		p.t.Fatalf("%s has no item %d to drop", p.Name, id)
	}
	p.send("drop", func(ctx context.Context) error { return p.DropItem(ctx, slot, amount) })
	p.Expect(fmt.Sprintf("item %d to be dropped", id), func(e *wathenav1.Event) bool {
		if f := e.GetDropItemFailed(); f != nil {
			p.t.Fatalf("%s: dropping item %d failed: %s", p.Name, id, f.GetReason())
		}
		r := e.GetInventoryRemove()
		return r != nil && int(r.GetSlot()) == slot && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_DROPPED
	})
}

// Learn spends a skill point on a skill and waits for the new level in
// the skill list.
func (p *Player) Learn(skill uint32) uint32 {
	p.t.Helper()
	before := uint32(0)
	if s := p.Game().Skills[skill]; s != nil {
		before = s.GetLevel()
	}
	p.send("learn", func(ctx context.Context) error { return p.LearnSkill(ctx, skill) })
	var level uint32
	p.Expect(fmt.Sprintf("skill %d to be learned", skill), func(e *wathenav1.Event) bool {
		for _, s := range e.GetSkillList().GetSkills() {
			if s.GetSkillId() == skill && s.GetLevel() > before {
				level = s.GetLevel()
				return true
			}
		}
		return false
	})
	return level
}

// Cast casts a skill on a target and returns the damage it showed.
func (p *Player) Cast(skill, level uint32, target botclient.Entity) *wathenav1.Damage {
	p.t.Helper()
	p.send("cast", func(ctx context.Context) error { return p.UseSkill(ctx, skill, level, target.ID) })
	return p.Expect(fmt.Sprintf("skill %d to hit %s", skill, target.Name), func(e *wathenav1.Event) bool {
		if f := e.GetSkillFailed(); f != nil && f.GetSkillId() == skill {
			p.t.Fatalf("%s: skill %d failed: %s", p.Name, skill, f.GetReason())
		}
		d := e.GetDamage()
		return d != nil && d.GetSkillId() == skill && d.GetSource() == p.Self() && d.GetTarget() == target.ID
	}).GetDamage()
}

// Stat is a stat the server sent last, such as STAT_ID_ZENY.
func (p *Player) Stat(id wathenav1.StatId) int64 { return p.Game().Stats[id] }

// Count is the amount of an item in the inventory.
func (p *Player) Count(id uint32) int { return p.Game().Count(id) }

// Conversation is a talk with an NPC.
type Conversation struct {
	p   *Player
	npc botclient.Entity
	// Lines are the lines shown since the last prompt, and Step the step
	// with the prompt.
	Lines []string
	Step  *wathenav1.NpcDialog
}

// Talk clicks an NPC by name and waits for its first prompt.
func (p *Player) Talk(name string) *Conversation {
	p.t.Helper()
	npc := p.Find(wathenav1.EntityKind_ENTITY_KIND_NPC, name)
	p.send("click", func(ctx context.Context) error { return p.Click(ctx, npc.ID) })
	c := &Conversation{p: p, npc: npc}
	c.wait()
	return c
}

// wait collects lines until a step with a prompt.
func (c *Conversation) wait() {
	c.p.t.Helper()
	c.Lines = nil
	ev := c.p.Expect(fmt.Sprintf("%s to talk", c.npc.Name), func(e *wathenav1.Event) bool {
		d := e.GetNpcDialog()
		if d == nil || d.GetNpc() != c.npc.ID {
			return false
		}
		c.Lines = append(c.Lines, d.GetLines()...)
		return d.HasNext() || d.HasMenu() || d.HasInputText() || d.HasInputNumber() || d.HasClose() || d.HasEnd()
	})
	c.Step = ev.GetNpcDialog()
}

// Says fails the test unless a shown line has text.
func (c *Conversation) Says(text string) {
	c.p.t.Helper()
	if !slices.ContainsFunc(c.Lines, func(l string) bool { return strings.Contains(l, text) }) {
		c.p.t.Fatalf("%s: %s said %q, not %q", c.p.Name, c.npc.Name, c.Lines, text)
	}
}

// Choose picks a menu option by its text and waits for the next prompt.
func (c *Conversation) Choose(option string) {
	c.p.t.Helper()
	opts := c.Step.GetMenu().GetOptions()
	i := slices.Index(opts, option)
	if i < 0 {
		c.p.t.Fatalf("%s: %s offers %q, not %q", c.p.Name, c.npc.Name, opts, option)
	}
	d := c.Step.GetDialog()
	c.p.send("choose", func(ctx context.Context) error { return c.p.Choose(ctx, d, uint32(i+1)) }) // #nosec G115 -- a menu index.
	c.wait()
}

// Pick picks a menu option by its text without waiting for another
// prompt, for an option that ends the conversation, such as a warp.
func (c *Conversation) Pick(option string) {
	c.p.t.Helper()
	opts := c.Step.GetMenu().GetOptions()
	i := slices.Index(opts, option)
	if i < 0 {
		c.p.t.Fatalf("%s: %s offers %q, not %q", c.p.Name, c.npc.Name, opts, option)
	}
	d := c.Step.GetDialog()
	c.p.send("choose", func(ctx context.Context) error { return c.p.Choose(ctx, d, uint32(i+1)) }) // #nosec G115 -- a menu index.
}

// Next answers a Next button and waits for the next prompt.
func (c *Conversation) Next() {
	c.p.t.Helper()
	d := c.Step.GetDialog()
	c.p.send("next", func(ctx context.Context) error { return c.p.Next(ctx, d) })
	c.wait()
}

// Close answers the Close button, which ends the conversation.
func (c *Conversation) Close() {
	c.p.t.Helper()
	if c.Step.HasEnd() {
		return
	}
	if !c.Step.HasClose() {
		c.p.t.Fatalf("%s: %s shows no Close button", c.p.Name, c.npc.Name)
	}
	d := c.Step.GetDialog()
	c.p.send("close", func(ctx context.Context) error { return c.p.CloseDialog(ctx, d) })
}

// OpenShop clicks a shop NPC and returns its window.
func (p *Player) OpenShop(name string) *wathenav1.ShopOpen {
	p.t.Helper()
	npc := p.Find(wathenav1.EntityKind_ENTITY_KIND_NPC, name)
	p.send("click", func(ctx context.Context) error { return p.Click(ctx, npc.ID) })
	return p.Expect(name+"'s shop to open", func(e *wathenav1.Event) bool { return e.HasShopOpen() }).GetShopOpen()
}

// Buy buys from an open shop and returns the outcome.
func (p *Player) Buy(shop *wathenav1.ShopOpen, lines map[uint32]int) wathenav1.ShopOutcome {
	p.t.Helper()
	p.send("buy", func(ctx context.Context) error { return p.Bot.Buy(ctx, shop.GetNpc(), lines) })
	return p.shopResult()
}

// Sell sells to an open shop, amounts by item ID from their first
// slots, and returns the outcome.
func (p *Player) Sell(shop *wathenav1.ShopOpen, lines map[uint32]int) wathenav1.ShopOutcome {
	p.t.Helper()
	slots := map[int]int{}
	for id, n := range lines {
		slot := p.Game().SlotOf(id)
		if slot < 0 {
			p.t.Fatalf("%s has no item %d to sell", p.Name, id)
		}
		slots[slot] = n
	}
	p.send("sell", func(ctx context.Context) error { return p.Bot.Sell(ctx, shop.GetNpc(), slots) })
	return p.shopResult()
}

func (p *Player) shopResult() wathenav1.ShopOutcome {
	p.t.Helper()
	return p.Expect("the shop's answer", func(e *wathenav1.Event) bool { return e.HasShopResult() }).GetShopResult().GetOutcome()
}

// MapChange waits for a warp to another map.
func (p *Player) MapChange(mapName string) *wathenav1.MapChange {
	p.t.Helper()
	return p.Expect("a warp to "+mapName, func(e *wathenav1.Event) bool {
		m := e.GetMapChange()
		return m != nil && m.GetMap() == mapName
	}).GetMapChange()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
