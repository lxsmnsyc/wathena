package botclient

import (
	"context"
	"errors"
	"maps"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Item is one filled inventory slot, as the server last told the bot.
type Item struct {
	Slot   int
	ID     uint32
	Amount int
	// Equip holds the positions the item is equipped in, or 0.
	Equip uint32
}

// gameState is what a bot knows about its own character. The caller of
// apply holds the bot's mu.
type gameState struct {
	items  map[int]Item
	stats  map[wathenav1.StatId]int64
	skills map[uint32]*wathenav1.SkillInfo
	// dialog is the last NPC dialog step, and shop the last shop window.
	dialog *wathenav1.NpcDialog
	shop   *wathenav1.ShopOpen
}

func newGameState() gameState {
	return gameState{items: map[int]Item{}, stats: map[wathenav1.StatId]int64{}, skills: map[uint32]*wathenav1.SkillInfo{}}
}

func (g *gameState) apply(e *wathenav1.Event) {
	switch {
	case e.HasInventory():
		clear(g.items)
		for _, it := range e.GetInventory().GetItems() {
			g.items[int(it.GetSlot())] = Item{Slot: int(it.GetSlot()), ID: it.GetItemId(), Amount: int(it.GetAmount()), Equip: it.GetEquip()}
		}
	case e.HasInventoryAdd():
		a := e.GetInventoryAdd()
		slot := int(a.GetSlot())
		it := g.items[slot]
		it.Slot, it.ID = slot, a.GetItemId()
		it.Amount += int(a.GetAmount())
		g.items[slot] = it
	case e.HasInventoryRemove():
		r := e.GetInventoryRemove()
		slot := int(r.GetSlot())
		it := g.items[slot]
		it.Amount -= int(r.GetAmount())
		if it.Amount <= 0 {
			delete(g.items, slot)
		} else {
			g.items[slot] = it
		}
	case e.HasItemEquipped():
		q := e.GetItemEquipped()
		if it, ok := g.items[int(q.GetSlot())]; ok {
			it.Equip = q.GetPosition()
			g.items[it.Slot] = it
		}
	case e.HasItemUnequipped():
		if it, ok := g.items[int(e.GetItemUnequipped().GetSlot())]; ok {
			it.Equip = 0
			g.items[it.Slot] = it
		}
	case e.HasStats():
		for _, s := range e.GetStats().GetStats() {
			g.stats[s.GetId()] = s.GetValue()
		}
	case e.HasSkillList():
		clear(g.skills)
		for _, s := range e.GetSkillList().GetSkills() {
			g.skills[s.GetSkillId()] = s
		}
	case e.HasNpcDialog():
		g.dialog = e.GetNpcDialog()
	case e.HasShopOpen():
		g.shop = e.GetShopOpen()
	case e.HasShopResult():
		g.shop = nil
	}
}

// Game is what a bot knows about its own character.
type Game struct {
	// Items are the inventory's filled slots, by slot.
	Items map[int]Item
	// Stats are the stats the server sent last, such as STAT_ID_ZENY and
	// STAT_ID_JOB.
	Stats map[wathenav1.StatId]int64
	// Skills are the skill list, by skill ID.
	Skills map[uint32]*wathenav1.SkillInfo
	// Dialog is the last NPC dialog step, and Shop the shop window open,
	// or nil.
	Dialog *wathenav1.NpcDialog
	Shop   *wathenav1.ShopOpen
	// Party is the bot's party, or nil. Invite is the last party invite
	// the bot has not answered, or nil.
	Party  *Party
	Invite *wathenav1.PartyInvite
	// Friends is the friend list, and FriendRequests the names that
	// asked to be friends and got no answer yet.
	Friends        []Friend
	FriendRequests []string
	// Guild is the bot's guild, or nil. GuildInvite is the last guild
	// invite the bot has not answered, or nil. Storage is the storage
	// window the bot has open, or nil.
	Guild       *Guild
	GuildInvite *wathenav1.GuildInvite
	Storage     *Storage
}

// Count is the amount of an item in the inventory.
func (g Game) Count(id uint32) int {
	n := 0
	for _, it := range g.Items {
		if it.ID == id {
			n += it.Amount
		}
	}
	return n
}

// SlotOf is the first slot that holds an item, or -1.
func (g Game) SlotOf(id uint32) int {
	best := -1
	for slot, it := range g.Items {
		if it.ID == id && (best < 0 || slot < best) {
			best = slot
		}
	}
	return best
}

// Game returns what the bot knows about its character now.
func (b *Bot) Game() Game {
	b.mu.Lock()
	defer b.mu.Unlock()
	party, invite, friends, requests := b.social.clone()
	guild, guildInvite, storage := b.guildSt.clone()
	return Game{
		Items: maps.Clone(b.game.items), Stats: maps.Clone(b.game.stats), Skills: maps.Clone(b.game.skills),
		Dialog: b.game.dialog, Shop: b.game.shop,
		Party: party, Invite: invite, Friends: friends, FriendRequests: requests,
		Guild: guild, GuildInvite: guildInvite, Storage: storage,
	}
}

// Events returns the events the bot got from index from on, and the
// index after them. It needs Options.Record.
func (b *Bot) Events(from int) ([]*wathenav1.Event, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	from = min(max(from, 0), len(b.events))
	return append([]*wathenav1.Event(nil), b.events[from:]...), len(b.events)
}

// Mark is the index of the next event, for Wait.
func (b *Bot) Mark() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// ErrClosed is a wait that ended because the connection closed.
var ErrClosed = errors.New("botclient: the connection closed")

// Wait waits for an event from index from on that match accepts, and
// returns it with the index after it. It needs Options.Record.
func (b *Bot) Wait(ctx context.Context, from int, match func(*wathenav1.Event) bool) (*wathenav1.Event, int, error) {
	from = max(from, 0)
	for {
		// match runs without the lock, so it may read the bot's state.
		b.mu.Lock()
		events := b.events[min(from, len(b.events)):]
		ch := b.changed
		b.mu.Unlock()
		for i, e := range events {
			if match(e) {
				return e, from + i + 1, nil
			}
		}
		from += len(events)
		select {
		case <-ch:
		case <-b.done:
			// Events that came with the last frame are read above first.
			b.mu.Lock()
			n := len(b.events)
			b.mu.Unlock()
			if n > from {
				continue
			}
			return nil, from, ErrClosed
		case <-ctx.Done():
			return nil, from, ctx.Err()
		}
	}
}

// WaitState waits until the bot's state passes a check. It checks again
// at each event, and every 50 ms, since the bot's own cell follows its
// walk over time.
func (b *Bot) WaitState(ctx context.Context, ok func(View, Game) bool) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		b.mu.Lock()
		ch := b.changed
		b.mu.Unlock()
		if ok(b.View(), b.Game()) {
			return nil
		}
		select {
		case <-t.C:
		case <-ch:
		case <-b.done:
			if ok(b.View(), b.Game()) {
				return nil
			}
			return ErrClosed
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (b *Bot) command(ctx context.Context, c wathenav1.Command_builder) error {
	return b.send(ctx, wathenav1.ClientMessage_builder{Command: c.Build()}, false)
}

// Click talks to an NPC or opens its shop.
func (b *Bot) Click(ctx context.Context, npc uint32) error {
	return b.command(ctx, wathenav1.Command_builder{NpcClick: wathenav1.NpcClick_builder{Npc: npc}.Build()})
}

// Next answers a dialog step's Next button.
func (b *Bot) Next(ctx context.Context, dialog uint32) error {
	return b.command(ctx, wathenav1.Command_builder{NpcReply: wathenav1.NpcReply_builder{Dialog: dialog, Next: new(true)}.Build()})
}

// Choose picks a menu option, from 1.
func (b *Bot) Choose(ctx context.Context, dialog, option uint32) error {
	return b.command(ctx, wathenav1.Command_builder{NpcReply: wathenav1.NpcReply_builder{Dialog: dialog, Menu: &option}.Build()})
}

// CloseDialog answers a Close button, or cancels a menu.
func (b *Bot) CloseDialog(ctx context.Context, dialog uint32) error {
	return b.command(ctx, wathenav1.Command_builder{NpcReply: wathenav1.NpcReply_builder{Dialog: dialog, Close: new(true)}.Build()})
}

// Buy buys from the open shop: amounts by item ID.
func (b *Bot) Buy(ctx context.Context, npc uint32, lines map[uint32]int) error {
	var l []*wathenav1.ShopBuyLine
	for id, n := range lines {
		l = append(l, wathenav1.ShopBuyLine_builder{ItemId: id, Amount: uint32(max(n, 0))}.Build()) // #nosec G115 -- small test amounts.
	}
	return b.command(ctx, wathenav1.Command_builder{ShopBuy: wathenav1.ShopBuy_builder{Npc: npc, Items: l}.Build()})
}

// Sell sells to the open shop: amounts by inventory slot.
func (b *Bot) Sell(ctx context.Context, npc uint32, lines map[int]int) error {
	var l []*wathenav1.ShopSellLine
	for slot, n := range lines {
		l = append(l, wathenav1.ShopSellLine_builder{Slot: uint32(max(slot, 0)), Amount: uint32(max(n, 0))}.Build()) // #nosec G115 -- slots and small test amounts.
	}
	return b.command(ctx, wathenav1.Command_builder{ShopSell: wathenav1.ShopSell_builder{Npc: npc, Items: l}.Build()})
}

// UseItem uses the item in an inventory slot.
func (b *Bot) UseItem(ctx context.Context, slot int) error {
	return b.command(ctx, wathenav1.Command_builder{UseItem: wathenav1.UseItem_builder{Slot: uint32(max(slot, 0))}.Build()}) // #nosec G115 -- inventory slots.
}

// DropItem drops an amount from an inventory slot.
func (b *Bot) DropItem(ctx context.Context, slot, amount int) error {
	return b.command(ctx, wathenav1.Command_builder{DropItem: wathenav1.DropItem_builder{
		Slot: uint32(max(slot, 0)), Amount: uint32(max(amount, 0)), // #nosec G115 -- inventory slots and amounts.
	}.Build()})
}

// PickUp walks to a ground item and picks it up.
func (b *Bot) PickUp(ctx context.Context, item uint32) error {
	return b.command(ctx, wathenav1.Command_builder{PickUp: wathenav1.PickUp_builder{Item: item}.Build()})
}

// LearnSkill spends a skill point on a skill.
func (b *Bot) LearnSkill(ctx context.Context, skill uint32) error {
	return b.command(ctx, wathenav1.Command_builder{LearnSkill: wathenav1.LearnSkill_builder{SkillId: skill}.Build()})
}

// UseSkill casts a skill on an entity, or on the caster with target 0.
func (b *Bot) UseSkill(ctx context.Context, skill, level, target uint32) error {
	u := wathenav1.UseSkill_builder{SkillId: skill, Level: level}
	if target != 0 {
		u.Entity = &target
	}
	return b.command(ctx, wathenav1.Command_builder{UseSkill: u.Build()})
}

// Whisper whispers a line to a character by name, or to a channel with
// a name that starts with #.
func (b *Bot) Whisper(ctx context.Context, to, text string) error {
	return b.command(ctx, wathenav1.Command_builder{Chat: wathenav1.Chat_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER, To: to, Text: text,
	}.Build()})
}

// ChannelChat says a line in a channel, such as #trade.
func (b *Bot) ChannelChat(ctx context.Context, channel, text string) error {
	return b.command(ctx, wathenav1.Command_builder{Chat: wathenav1.Chat_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_NAMED, ChannelName: channel, Text: text,
	}.Build()})
}

// Channel joins or leaves a channel, or asks for the channels or a
// channel's members.
func (b *Bot) Channel(ctx context.Context, action wathenav1.ChannelAction, name string) error {
	return b.command(ctx, wathenav1.Command_builder{Channel: wathenav1.ChannelCommand_builder{Action: action, Name: name}.Build()})
}

// Block adds a name to the block list, or removes it.
func (b *Bot) Block(ctx context.Context, name string, unblock bool) error {
	return b.command(ctx, wathenav1.Command_builder{Block: wathenav1.Block_builder{Name: name, Unblock: unblock}.Build()})
}

// Report reports a player, or the line with an ID when line is not
// empty.
func (b *Bot) Report(ctx context.Context, name string, line []byte, reason string) error {
	return b.command(ctx, wathenav1.Command_builder{Report: wathenav1.Report_builder{Name: name, LineId: line, Reason: reason}.Build()})
}
