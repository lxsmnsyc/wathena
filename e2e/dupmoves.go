package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Storage, cart and trade moves of the duplication tests (phase 5 step
// 6). Each bot uses its own account storage and its own cart, and trades
// with the other bots on the start map, while the test audits.

// stepWait is how long a move waits for one answer of the server.
const stepWait = 3 * time.Second

// waitStep waits up to stepWait for an event from mark on. It returns nil
// and no error when the time is up while ctx is not done.
func waitStep(ctx context.Context, b *botclient.Bot, mark int, match func(*wathenav1.Event) bool) (*wathenav1.Event, error) {
	wait, cancel := context.WithTimeout(ctx, stepWait)
	defer cancel()
	e, _, err := b.Wait(wait, mark, match)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return nil, nil
	}
	return e, err
}

// itemsOf are the slots of an item in a set of slots, unequipped, in slot
// order.
func itemsOf(slots map[int]botclient.Item, id uint32) []botclient.Item {
	var out []botclient.Item
	for _, it := range slots {
		if it.ID == id && it.Equip == 0 {
			out = append(out, it)
		}
	}
	slices.SortFunc(out, func(a, b botclient.Item) int { return a.Slot - b.Slot })
	return out
}

// CartSetup teaches every bot, a Merchant, Enlarge Weight Limit 5 and
// Pushcart, gives it a cart with the fixture's "!cart", and zeny with
// "!zeny" when zeny is above 0.
func CartSetup(zeny int) func(ctx context.Context, bots []*botclient.Bot) error {
	return func(ctx context.Context, bots []*botclient.Bot) error {
		for _, b := range bots {
			for i, skill := range []uint32{SkillEnlargeWeight, SkillEnlargeWeight, SkillEnlargeWeight, SkillEnlargeWeight, SkillEnlargeWeight, SkillPushcart} {
				want := uint32(i + 1) // #nosec G115 -- a small count.
				if skill == SkillPushcart {
					want = 1
				}
				if err := b.LearnSkill(ctx, skill); err != nil {
					return err
				}
				if err := b.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool {
					return g.Skills[skill].GetLevel() >= want
				}); err != nil {
					return fmt.Errorf("%s: learning skill %d: %w", b.Name, skill, err)
				}
			}
			if err := b.Chat(ctx, "!cart"); err != nil {
				return err
			}
			if err := b.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool { return g.Cart != nil && g.Cart.Complete() }); err != nil {
				return fmt.Errorf("%s: no cart: %w", b.Name, err)
			}
			if zeny > 0 {
				if err := b.Chat(ctx, "!zeny "+strconv.Itoa(zeny)); err != nil {
					return err
				}
				if err := b.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool {
					return g.Stats[wathenav1.StatId_STAT_ID_ZENY] >= int64(zeny)
				}); err != nil {
					return fmt.Errorf("%s: no zeny: %w", b.Name, err)
				}
			}
		}
		return nil
	}
}

// PersonalCounts count what the bots did with their storages and carts.
type PersonalCounts struct {
	// Opened counts the times a bot opened its storage.
	Opened atomic.Int64
	// Moves counts the moves by kind: put, take, cart_put, cart_take,
	// cart_to_storage and storage_to_cart. Refused counts the moves the
	// server refused as full or overweight.
	Put, Take, CartPut, CartTake, CartToStorage, StorageToCart, Refused atomic.Int64
	// Dropped counts the connections closed with the storage open or in
	// the middle of a cart move.
	Dropped atomic.Int64
	// Restored counts the moves that got no answer because a restore
	// took the stack away first (decision 0029).
	Restored atomic.Int64
}

// restoredSince reports whether a restore took an item from the bot's
// inventory or cart since a mark: the server sends those as taken, and
// as a cart removal the bot did not ask for.
func restoredSince(b *botclient.Bot, mark int) bool {
	events, _ := b.Events(mark)
	for _, e := range events {
		if e.GetInventoryRemove().GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TAKEN ||
			e.HasStorageRemove() && e.GetStorageRemove().GetKind() == wathenav1.StorageKind_STORAGE_KIND_CART {
			return true
		}
	}
	return false
}

// kindMove is one move between the inventory, the open storage and the
// cart.
type kindMove struct {
	name  string
	count *atomic.Int64
	// from is the slots the amount leaves, and send sends the command.
	from map[int]botclient.Item
	send func(ctx context.Context, slot, amount int) error
	// to is the kind of storage the amount enters, or 0 for the
	// inventory.
	to wathenav1.StorageKind
}

// personalStep moves part of a random Jellopy stack one of the ways the
// bot has: inventory to storage or cart, and back, and between the cart
// and the open storage.
func personalStep(ctx context.Context, b *botclient.Bot, id uint32, rnd *rand.Rand, c *PersonalCounts) error {
	g := b.Game()
	inv := map[int]botclient.Item{}
	for s, it := range g.Items {
		inv[s] = it
	}
	var moves []kindMove
	personal, cart := wathenav1.StorageKind_STORAGE_KIND_PERSONAL, wathenav1.StorageKind_STORAGE_KIND_CART
	if g.Storage != nil && g.Storage.Complete() {
		moves = append(moves,
			kindMove{"put", &c.Put, inv, b.StoragePut, personal},
			kindMove{"take", &c.Take, g.Storage.Items, b.StorageTake, 0})
	}
	if g.Cart != nil && g.Cart.Complete() {
		moves = append(moves,
			kindMove{"cart_put", &c.CartPut, inv, b.CartPut, cart},
			kindMove{"cart_take", &c.CartTake, g.Cart.Items, b.CartTake, 0})
		if g.Storage != nil && g.Storage.Complete() {
			moves = append(moves,
				kindMove{"cart_to_storage", &c.CartToStorage, g.Cart.Items, b.CartToStorage, personal},
				kindMove{"storage_to_cart", &c.StorageToCart, g.Storage.Items, b.StorageToCart, cart})
		}
	}
	rnd.Shuffle(len(moves), func(i, j int) { moves[i], moves[j] = moves[j], moves[i] })
	for _, m := range moves {
		held := itemsOf(m.from, id)
		if len(held) == 0 {
			continue
		}
		it := held[rnd.IntN(len(held))]
		amount := 1 + rnd.IntN(it.Amount)
		mark := b.Mark()
		if err := m.send(ctx, it.Slot, amount); err != nil {
			return err
		}
		e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
			if e.HasStorageResult() || e.GetStorageClose().GetKind() == personal && e.HasStorageClose() {
				return true
			}
			if m.to == 0 {
				a := e.GetInventoryAdd()
				return a != nil && a.GetItemId() == id
			}
			a := e.GetStorageAdd()
			return a != nil && a.GetKind() == m.to && a.GetItem().GetItemId() == id
		})
		switch {
		case err != nil:
			return err
		case e == nil && restoredSince(b, mark):
			// The server ignores a move of a stack that is gone, as
			// rAthena does.
			c.Restored.Add(1)
			return nil
		case e == nil:
			return fmt.Errorf("%s of %d Jellopy from slot %d: no answer in %s", m.name, amount, it.Slot, stepWait)
		case e.HasStorageResult():
			switch o := e.GetStorageResult().GetOutcome(); o {
			case wathenav1.StorageOutcome_STORAGE_OUTCOME_FULL, wathenav1.StorageOutcome_STORAGE_OUTCOME_CANNOT_CARRY,
				wathenav1.StorageOutcome_STORAGE_OUTCOME_CART_OVERWEIGHT,
				// A trade that another bot opened, or one whose close a
				// stalled zone has not sent yet, refuses the move and
				// changes nothing.
				wathenav1.StorageOutcome_STORAGE_OUTCOME_BUSY:
				c.Refused.Add(1)
				return nil
			default:
				return fmt.Errorf("%s of %d Jellopy from slot %d: %s", m.name, amount, it.Slot, o)
			}
		case e.HasStorageClose():
			return errors.New("the storage closed")
		}
		m.count.Add(1)
		return nil
	}
	return nil
}

// PersonalStorageMove opens the bot's account storage with the fixture's
// "!storage", and moves parts of Jellopy stacks one to four times between
// the inventory, the storage and the cart. Then it closes the storage.
// One time in 8 the bot closes its connection instead, with the storage
// open, and logs in again.
func PersonalStorageMove(id uint32, c *PersonalCounts) Move {
	return Move{Name: "storage", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		if b.Game().Storage != nil {
			// A move that ended with the test's time left it open.
			return false, ignoreDone(ctx, closeStorage(ctx, b))
		}
		mark := b.Mark()
		if err := b.Chat(ctx, "!storage"); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
			c := e.GetChat()
			return e.GetStorageOpen().GetKind() == wathenav1.StorageKind_STORAGE_KIND_PERSONAL && e.HasStorageOpen() ||
				c != nil && c.GetText() == "e2e storage: false"
		})
		if err != nil || e == nil {
			if err == nil {
				err = errors.New("the storage did not open")
			}
			return false, ignoreDone(ctx, err)
		}
		if e.HasChat() {
			return false, errors.New("the fixture could not open the storage")
		}
		wait, cancel := context.WithTimeout(ctx, stepWait)
		defer cancel()
		if err := b.WaitState(wait, func(_ botclient.View, g botclient.Game) bool { return g.Storage != nil && g.Storage.Complete() }); err != nil {
			return false, ignoreDone(ctx, err)
		}
		c.Opened.Add(1)
		for range 1 + rnd.IntN(4) {
			if err := personalStep(ctx, b, id, rnd, c); err != nil {
				return true, ignoreDone(ctx, err)
			}
		}
		if rnd.IntN(8) == 0 {
			c.Dropped.Add(1)
			_ = b.CloseNow()
			return true, errLeft
		}
		return true, ignoreDone(ctx, closeStorage(ctx, b))
	}}
}

// CartMove puts part of a Jellopy stack into the cart or takes part of
// one out. One time in 15 the bot sends the move and closes its
// connection at once: the move may happen or not.
func CartMove(id uint32, c *PersonalCounts) Move {
	return Move{Name: "cart", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		g := b.Game()
		if g.Cart == nil || !g.Cart.Complete() || g.Storage != nil {
			return false, pause(ctx)
		}
		if rnd.IntN(15) == 0 {
			held, put := itemsOf(g.Items, id), true
			if rnd.IntN(2) == 0 || len(held) == 0 {
				held, put = itemsOf(g.Cart.Items, id), false
			}
			if len(held) == 0 {
				return false, pause(ctx)
			}
			it := held[rnd.IntN(len(held))]
			send := b.CartTake
			if put {
				send = b.CartPut
			}
			if err := send(ctx, it.Slot, 1+rnd.IntN(it.Amount)); err != nil {
				return false, ignoreDone(ctx, err)
			}
			if n := rnd.IntN(3); n > 0 {
				time.Sleep(time.Duration(n) * botTick / 2)
			}
			c.Dropped.Add(1)
			_ = b.CloseNow()
			return true, errLeft
		}
		return true, ignoreDone(ctx, personalStep(ctx, b, id, rnd, c))
	}}
}

// TradeCounts count the trades of a TradeMove.
type TradeCounts struct {
	// Requested counts the requests sent, and Refused the ones refused:
	// by the other bot, as busy, or with no answer in time.
	Requested, Refused atomic.Int64
	// Opened counts the trade windows that opened, Completed the trades
	// that completed, and Cancelled the ones that closed otherwise.
	Opened, Completed, Cancelled atomic.Int64
	// Dropped counts the connections closed in the middle of a trade:
	// before the lock, after it, or right after the confirm.
	Dropped atomic.Int64
}

// botIndex is a bot's number in its test, from 1.
func botIndex(name, prefix string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(name, prefix))
	return n
}

// endTrade cancels a trade window that a move left open, and waits for
// it to close.
func endTrade(ctx context.Context, b *botclient.Bot) error {
	if b.Game().Trade == nil {
		return nil
	}
	if err := b.TradeCancel(ctx); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, stepWait)
	defer cancel()
	return b.WaitState(wait, func(_ botclient.View, g botclient.Game) bool { return g.Trade == nil })
}

// TradeMove trades Jellopy and zeny between the bots. The bots with odd
// numbers ask a random bot with an even number to trade. The bots with
// even numbers answer the request they got, and refuse one time in 10.
// Each side offers up to two parts of Jellopy stacks and sometimes zeny,
// locks and confirms. One time in 15 at each of three points, before
// the lock, after it and right after the confirm, a bot closes its
// connection instead and logs in again. Every bot must stay within 2
// cells of the others.
func TradeMove(prefix string, bots int, id uint32, c *TradeCounts) Move {
	// The zone takes one trade request a second from a player (review
	// 2026-10). An asker that asked less than a second ago does another
	// move instead.
	var asked sync.Map
	return Move{Name: "trade", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		if err := endTrade(ctx, b); err != nil {
			return false, ignoreDone(ctx, err)
		}
		if b.Game().Storage != nil {
			return false, ignoreDone(ctx, closeStorage(ctx, b))
		}
		var opened int
		var ok bool
		var err error
		if botIndex(b.Name, prefix)%2 == 1 {
			if last, ok := asked.Load(b.Name); ok && time.Since(last.(time.Time)) < 1100*time.Millisecond {
				return false, pause(ctx)
			}
			asked.Store(b.Name, time.Now())
			opened, ok, err = askTrade(ctx, b, prefix, bots, rnd, c)
		} else {
			opened, ok, err = answerTrade(ctx, b, rnd, c)
		}
		if err != nil || !ok {
			return false, ignoreDone(ctx, err)
		}
		c.Opened.Add(1)
		done, err := tradeSide(ctx, b, id, opened, rnd, c)
		return done, ignoreDone(ctx, err)
	}}
}

// askTrade asks a bot with an even number to trade. It returns the mark
// of the open window, and false when the window did not open.
func askTrade(ctx context.Context, b *botclient.Bot, prefix string, bots int, rnd *rand.Rand, c *TradeCounts) (int, bool, error) {
	partner := fmt.Sprintf("%s%d", prefix, 2*(1+rnd.IntN(bots/2)))
	var target uint32
	for _, e := range b.View().Entities {
		if e.Kind == wathenav1.EntityKind_ENTITY_KIND_PLAYER && e.Name == partner {
			target = e.ID
		}
	}
	if target == 0 {
		// The partner is logging in again.
		return 0, false, pause(ctx)
	}
	mark := b.Mark()
	if err := b.TradeRequest(ctx, target); err != nil {
		return 0, false, err
	}
	c.Requested.Add(1)
	e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
		r := e.GetTradeResult()
		return e.HasTradeOpen() || e.HasTradeClose() || r != nil && r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT
	})
	switch {
	case err != nil:
		return 0, false, err
	case e == nil:
		// No answer: take the request back.
		c.Refused.Add(1)
		return 0, false, b.TradeCancel(ctx)
	case e.HasTradeOpen():
		return mark, true, nil
	case e.HasTradeClose():
		c.Refused.Add(1)
		return 0, false, nil
	}
	switch o := e.GetTradeResult().GetOutcome(); o {
	case wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED, wathenav1.TradeOutcome_TRADE_OUTCOME_BUSY,
		wathenav1.TradeOutcome_TRADE_OUTCOME_NOT_FOUND, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED:
		c.Refused.Add(1)
		return 0, false, nil
	case wathenav1.TradeOutcome_TRADE_OUTCOME_FAILED:
		// Too soon after the last request: TradeMove waits a second
		// between requests, and this covers the rest.
		if e.GetTradeResult().GetName() == "" {
			return 0, false, fmt.Errorf("asking %s to trade: %s", partner, o)
		}
		c.Refused.Add(1)
		return 0, false, pause(ctx)
	default:
		return 0, false, fmt.Errorf("asking %s to trade: %s", partner, o)
	}
}

// answerTrade answers the bot's last trade request. It returns the mark
// of the open window, and false when there was no request or the window
// did not open.
func answerTrade(ctx context.Context, b *botclient.Bot, rnd *rand.Rand, c *TradeCounts) (int, bool, error) {
	if b.Game().TradeRequest == nil {
		return 0, false, pause(ctx)
	}
	mark := b.Mark()
	if rnd.IntN(10) == 0 {
		return 0, false, b.TradeRespond(ctx, false)
	}
	if err := b.TradeRespond(ctx, true); err != nil {
		return 0, false, err
	}
	e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
		return e.HasTradeOpen() || e.HasTradeClose() || e.HasTradeResult()
	})
	if err != nil || e == nil || !e.HasTradeOpen() {
		// The request was taken back, or the other bot is busy or gone.
		return 0, false, err
	}
	return mark, true, nil
}

// tradeSide plays one side of an open trade. It returns true when the
// trade completed.
func tradeSide(ctx context.Context, b *botclient.Bot, id uint32, mark int, rnd *rand.Rand, c *TradeCounts) (bool, error) {
	closed := func(e *wathenav1.Event) bool { return e.HasTradeClose() }
	// step sends a command and waits for done or the close. It returns
	// false when the trade is over.
	step := func(send func() error, done func(e *wathenav1.Event) bool) (bool, error) {
		at := b.Mark()
		if err := send(); err != nil {
			return false, err
		}
		if seenSince(b, mark, closed) {
			return false, nil
		}
		e, err := waitStep(ctx, b, at, func(e *wathenav1.Event) bool { return closed(e) || done(e) })
		if err != nil {
			return false, err
		}
		if e == nil || e.HasTradeClose() {
			return false, nil
		}
		return true, nil
	}
	drop := func() (bool, error) {
		c.Dropped.Add(1)
		_ = b.CloseNow()
		return false, errLeft
	}
	finish := func() (bool, error) {
		// The trade closed, or a step had no answer: end it and count
		// how it closed.
		e, err := waitStep(ctx, b, mark, closed)
		if err != nil {
			return false, err
		}
		if e == nil {
			if err := b.TradeCancel(ctx); err != nil {
				return false, err
			}
			if e, err = waitStep(ctx, b, mark, closed); err != nil || e == nil {
				if err == nil {
					err = errors.New("the trade did not close")
				}
				return false, err
			}
		}
		if e.GetTradeClose().GetOutcome() == wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED {
			c.Completed.Add(1)
			return true, nil
		}
		c.Cancelled.Add(1)
		return false, nil
	}

	g := b.Game()
	held := itemsOf(g.Items, id)
	rnd.Shuffle(len(held), func(i, j int) { held[i], held[j] = held[j], held[i] })
	for _, it := range held[:min(len(held), rnd.IntN(3))] {
		amount := 1 + rnd.IntN(it.Amount)
		more, err := step(func() error { return b.TradeAddItem(ctx, it.Slot, amount) }, func(e *wathenav1.Event) bool {
			o := e.GetTradeOffer()
			// A refusal, such as no room left, is an answer too.
			return e.HasTradeResult() || o != nil && o.GetMine() && o.HasItem() && int(o.GetItem().GetSlot()) == it.Slot
		})
		if err != nil || !more {
			return finishOr(err, finish)
		}
	}
	if z := g.Stats[wathenav1.StatId_STAT_ID_ZENY]; z > 0 && rnd.IntN(2) == 0 {
		amount := uint32(1 + rnd.IntN(int(min(z, 500)))) // #nosec G115 -- at most 500.
		more, err := step(func() error { return b.TradeAddZeny(ctx, amount) }, func(e *wathenav1.Event) bool {
			o := e.GetTradeOffer()
			return e.HasTradeResult() || o != nil && o.GetMine() && o.HasZeny()
		})
		if err != nil || !more {
			return finishOr(err, finish)
		}
	}
	if rnd.IntN(15) == 0 {
		return drop()
	}
	more, err := step(func() error { return b.TradeLock(ctx) }, func(e *wathenav1.Event) bool {
		return e.HasTradeLock() && e.GetTradeLock().GetMine()
	})
	if err != nil || !more {
		return finishOr(err, finish)
	}
	// The partner's lock may have come already.
	e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
		return closed(e) || e.HasTradeLock() && !e.GetTradeLock().GetMine()
	})
	if err != nil || e == nil || e.HasTradeClose() {
		return finishOr(err, finish)
	}
	if rnd.IntN(15) == 0 {
		return drop()
	}
	if err := b.TradeConfirm(ctx); err != nil {
		return false, err
	}
	if rnd.IntN(15) == 0 {
		if n := rnd.IntN(3); n > 0 {
			time.Sleep(time.Duration(n) * botTick / 2)
		}
		return drop()
	}
	return finish()
}

// seenSince reports whether the bot got an event from mark on that match
// accepts.
func seenSince(b *botclient.Bot, mark int, match func(*wathenav1.Event) bool) bool {
	events, _ := b.Events(mark)
	return slices.ContainsFunc(events, match)
}

// finishOr returns err, or else what finish returns.
func finishOr(err error, finish func() (bool, error)) (bool, error) {
	if err != nil {
		return false, err
	}
	return finish()
}

// SettleTrades ends a trade or closes a storage a bot left open.
func SettleTrades(ctx context.Context, b *botclient.Bot, _ *rand.Rand) (bool, error) {
	if b.Game().Trade != nil {
		return true, endTrade(ctx, b)
	}
	if b.Game().Storage != nil {
		return true, closeStorage(ctx, b)
	}
	return false, nil
}
