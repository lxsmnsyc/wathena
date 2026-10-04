package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Vending, shop, mail and buying store moves of the duplication tests
// (phase 5 step 7). The bots stay on their start cell, so every bot sees
// every shop, and the fixture's Exchange is in click range.

// MarketCounts count what the bots did with shops, stores and mail.
type MarketCounts struct {
	// ShopsOpened and ShopsClosed count the vending shops opened and
	// closed by their owner, and Bought the purchases from them.
	ShopsOpened, ShopsClosed, Bought atomic.Int64
	// Exchanged counts the Exchange purchases that paid Jellopy for
	// Jellopy, and TokenBuys the ones that paid tokens for Red Potions.
	Exchanged, TokenBuys atomic.Int64
	// Mailed and Taken count the mails sent and taken.
	Mailed, Taken atomic.Int64
	// StoresOpened counts the buying stores opened, and SoldToStores the
	// sales to them.
	StoresOpened, SoldToStores atomic.Int64
	// Refused counts the steps the server refused, such as a purchase of
	// a line another bot bought first.
	Refused atomic.Int64
	// Dropped counts the connections closed in the middle of a sale, a
	// purchase, a mail or a take, or with a shop or a store open.
	Dropped atomic.Int64
}

// drop closes a bot's connection after a command, at once or after up to
// one tick, as a player whose network drops.
func drop(b *botclient.Bot, rnd *rand.Rand, c *MarketCounts) error {
	if n := rnd.IntN(3); n > 0 {
		time.Sleep(time.Duration(n) * botTick / 2)
	}
	c.Dropped.Add(1)
	_ = b.CloseNow()
	return errLeft
}

// MarketSetup gives every bot, a Merchant, a cart and zeny as CartSetup
// does, then raises Pushcart to level 3 and teaches it Vending.
func MarketSetup(zeny int) func(ctx context.Context, bots []*botclient.Bot) error {
	cart := CartSetup(zeny)
	return func(ctx context.Context, bots []*botclient.Bot) error {
		if err := cart(ctx, bots); err != nil {
			return err
		}
		for _, b := range bots {
			for _, skill := range []uint32{SkillPushcart, SkillPushcart, SkillVending} {
				want := b.Game().Skills[skill].GetLevel() + 1
				if err := b.LearnSkill(ctx, skill); err != nil {
					return err
				}
				if err := b.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool { return g.Skills[skill].GetLevel() >= want }); err != nil {
					return fmt.Errorf("%s: learning skill %d: %w", b.Name, skill, err)
				}
			}
		}
		return nil
	}
}

// VendMove opens a vending shop of part of a Jellopy stack in the cart,
// at 1 to 20 zeny, after putting Jellopy in the cart when it has none.
// With a shop open, the bot closes it one time in 4, and closes its
// connection with the shop open one time in 12.
func VendMove(id uint32, c *MarketCounts) Move {
	return Move{Name: "vend", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		g := b.Game()
		if g.Cart == nil || !g.Cart.Complete() {
			return false, pause(ctx)
		}
		if g.Vending != nil {
			switch rnd.IntN(12) {
			case 0:
				return true, drop(b, rnd, c)
			case 1, 2, 3:
				mark := b.Mark()
				if err := b.VendingClose(ctx); err != nil {
					return false, ignoreDone(ctx, err)
				}
				e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
					return e.GetVendingResult().GetOutcome() == wathenav1.VendingOutcome_VENDING_OUTCOME_CLOSED
				})
				if err != nil || e == nil {
					if err == nil {
						err = errors.New("the shop did not close")
					}
					return false, ignoreDone(ctx, err)
				}
				c.ShopsClosed.Add(1)
				return true, nil
			}
			return false, pause(ctx)
		}
		stacks := itemsOf(g.Cart.Items, id)
		if len(stacks) == 0 {
			held := itemsOf(g.Items, id)
			if len(held) == 0 {
				return false, pause(ctx)
			}
			it := held[rnd.IntN(len(held))]
			mark := b.Mark()
			if err := b.CartPut(ctx, it.Slot, 1+rnd.IntN(it.Amount)); err != nil {
				return false, ignoreDone(ctx, err)
			}
			_, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
				return e.HasStorageResult() || e.GetStorageAdd().GetKind() == wathenav1.StorageKind_STORAGE_KIND_CART && e.HasStorageAdd()
			})
			return false, ignoreDone(ctx, err)
		}
		mark := b.Mark()
		if err := b.UseSkill(ctx, SkillVending, 1, b.View().NetID); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
			return e.HasVendingSetup() || e.GetSkillFailed().GetSkillId() == SkillVending && e.HasSkillFailed()
		})
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case e == nil || e.HasSkillFailed():
			// Too little SP, or the cast did not end in time.
			c.Refused.Add(1)
			return false, nil
		}
		it := stacks[rnd.IntN(len(stacks))]
		line := botclient.VendLine{CartSlot: it.Slot, Amount: 1 + rnd.IntN(it.Amount), Price: uint32(1 + rnd.IntN(20))} // #nosec G115 -- at most 20.
		mark = b.Mark()
		if err := b.VendingOpen(ctx, b.Name+"'s shop", []botclient.VendLine{line}); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err = waitStep(ctx, b, mark, (*wathenav1.Event).HasVendingResult)
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case e == nil:
			return false, fmt.Errorf("opening a shop of %d Jellopy from cart slot %d: no answer in %s", line.Amount, line.CartSlot, stepWait)
		case e.GetVendingResult().GetOutcome() != wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED:
			c.Refused.Add(1)
			return false, nil
		}
		c.ShopsOpened.Add(1)
		return true, nil
	}}
}

// storeOwners are the players in view with a vending shop, or with a
// buying store when buying is set, by entity ID, so a seeded bot picks
// the same one.
func storeOwners(b *botclient.Bot, buying bool) []botclient.Entity {
	var out []botclient.Entity
	for _, e := range b.View().Entities {
		if e.Kind == wathenav1.EntityKind_ENTITY_KIND_PLAYER && e.ShopTitle != "" && e.ShopBuying == buying {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b botclient.Entity) int { return int(a.ID) - int(b.ID) })
	return out
}

// VendBuyMove browses a random vending shop in view and buys part of a
// random line. One time in 10 the bot sends the purchase and closes its
// connection at once: the sale may happen or not.
func VendBuyMove(c *MarketCounts) Move {
	return Move{Name: "vend_buy", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		shops := storeOwners(b, false)
		if len(shops) == 0 {
			return false, pause(ctx)
		}
		vendor := shops[rnd.IntN(len(shops))]
		mark := b.Mark()
		if err := b.VendingBrowse(ctx, vendor.ID); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
			return e.GetVendingList().GetVendor() == vendor.ID && e.HasVendingList()
		})
		if err != nil || e == nil {
			// The shop closed first.
			return false, ignoreDone(ctx, err)
		}
		list := e.GetVendingList()
		if len(list.GetItems()) == 0 {
			return false, nil
		}
		line := list.GetItems()[rnd.IntN(len(list.GetItems()))]
		amount := 1 + rnd.IntN(int(line.GetItem().GetAmount()))
		mark = b.Mark()
		if err := b.VendingBuy(ctx, vendor.ID, list.GetShop(), map[uint32]int{line.GetIndex(): amount}); err != nil {
			return false, ignoreDone(ctx, err)
		}
		if rnd.IntN(10) == 0 {
			return true, drop(b, rnd, c)
		}
		e, err = waitStep(ctx, b, mark, (*wathenav1.Event).HasVendingResult)
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case e == nil:
			// The shop closed with its owner's connection.
			c.Refused.Add(1)
			return false, nil
		case e.GetVendingResult().GetOutcome() != wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT:
			c.Refused.Add(1)
			return false, nil
		}
		c.Bought.Add(1)
		return true, nil
	}}
}

// ExchangeMove opens the fixture's Exchange and buys 1 to 3 Jellopy,
// which cost a Jellopy each. One time in 10 the bot sends the purchase
// and closes its connection at once.
func ExchangeMove(c *MarketCounts) Move {
	return Move{Name: "exchange", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		return exchangeBuy(ctx, b, rnd, c, Jellopy, b.Game().Count(Jellopy), &c.Exchanged)
	}}
}

// TokenMove opens the fixture's Exchange and buys 1 to 3 Red Potions,
// which cost 5 tokens each, while the bot has the tokens, at most once
// in every. One time in 10 the bot sends the purchase and closes its
// connection at once. Only this move makes Red Potions or takes tokens,
// so the tokens plus 5 for each Red Potion stay constant.
func TokenMove(c *MarketCounts, every time.Duration) Move {
	var mu sync.Mutex
	last := map[string]time.Time{}
	return Move{Name: "tokens", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		mu.Lock()
		soon := time.Since(last[b.Name]) < every
		if !soon {
			last[b.Name] = time.Now()
		}
		mu.Unlock()
		if soon {
			return false, pause(ctx)
		}
		return exchangeBuy(ctx, b, rnd, c, RedPotion, int(b.Game().Currencies[Tokens]/TokensPerPotion), &c.TokenBuys)
	}}
}

// TokensPerPotion is the Exchange's price of a Red Potion in tokens.
const TokensPerPotion = 5

// exchangeBuy opens the Exchange and buys 1 to 3 of an item, at most
// can. It counts a purchase the server confirmed in bought.
func exchangeBuy(ctx context.Context, b *botclient.Bot, rnd *rand.Rand, c *MarketCounts, item uint32, can int, bought *atomic.Int64) (bool, error) {
	if can <= 0 {
		return false, pause(ctx)
	}
	var npc uint32
	for _, e := range b.View().Entities {
		if e.Kind == wathenav1.EntityKind_ENTITY_KIND_NPC && e.Name == ExchangeName {
			npc = e.ID
		}
	}
	if npc == 0 {
		return false, pause(ctx)
	}
	mark := b.Mark()
	if err := b.Click(ctx, npc); err != nil {
		return false, ignoreDone(ctx, err)
	}
	e, err := waitStep(ctx, b, mark, (*wathenav1.Event).HasShopOpen)
	if err != nil || e == nil {
		if err == nil {
			err = errors.New("the Exchange did not open")
		}
		return false, ignoreDone(ctx, err)
	}
	n := 1 + rnd.IntN(min(can, 3))
	mark = b.Mark()
	if err := b.Buy(ctx, npc, map[uint32]int{item: n}); err != nil {
		return false, ignoreDone(ctx, err)
	}
	if rnd.IntN(10) == 0 {
		return true, drop(b, rnd, c)
	}
	e, err = waitStep(ctx, b, mark, (*wathenav1.Event).HasShopResult)
	switch {
	case err != nil:
		return false, ignoreDone(ctx, err)
	case e == nil:
		return false, fmt.Errorf("buying %d of item %d at the Exchange: no answer in %s", n, item, stepWait)
	case e.GetShopResult().GetOutcome() != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT:
		c.Refused.Add(1)
		return false, nil
	}
	bought.Add(1)
	return true, nil
}

// MailMove mails parts of up to two Jellopy stacks, and sometimes zeny,
// to a random other bot. One time in 10 the bot sends the mail and
// closes its connection at once. The world's rules must have no mail
// fees, as e2e.marketrules, or the zeny total falls.
func MailMove(prefix string, bots int, c *MarketCounts) Move {
	return Move{Name: "mail", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		to := fmt.Sprintf("%s%d", prefix, 1+rnd.IntN(bots))
		if to == b.Name {
			return false, nil
		}
		g := b.Game()
		held := itemsOf(g.Items, Jellopy)
		rnd.Shuffle(len(held), func(i, j int) { held[i], held[j] = held[j], held[i] })
		var items []botclient.Attachment
		for _, it := range held[:min(len(held), 1+rnd.IntN(2))] {
			items = append(items, botclient.Attachment{Slot: it.Slot, Amount: 1 + rnd.IntN(it.Amount)})
		}
		var zeny uint32
		if z := g.Stats[wathenav1.StatId_STAT_ID_ZENY]; z > 0 && rnd.IntN(2) == 0 {
			zeny = uint32(1 + rnd.IntN(int(min(z, 300)))) // #nosec G115 -- at most 300.
		}
		if len(items) == 0 && zeny == 0 {
			return false, pause(ctx)
		}
		mark := b.Mark()
		if err := b.MailSend(ctx, to, "Jellopy", "", zeny, items); err != nil {
			return false, ignoreDone(ctx, err)
		}
		if rnd.IntN(10) == 0 {
			return true, drop(b, rnd, c)
		}
		e, err := waitStep(ctx, b, mark, (*wathenav1.Event).HasMailResult)
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case e == nil:
			return false, fmt.Errorf("mailing %s: no answer in %s", to, stepWait)
		case e.GetMailResult().GetOutcome() != wathenav1.MailOutcome_MAIL_OUTCOME_SENT:
			// Too soon after the last, or the recipient's mailbox is full.
			c.Refused.Add(1)
			return false, nil
		}
		c.Mailed.Add(1)
		return true, nil
	}}
}

// MailTakeMove opens the bot's mailbox, and takes the zeny and items of
// a random mail that holds some, or deletes an empty mail. One time in
// 10 the bot sends the take and closes its connection at once.
func MailTakeMove(c *MarketCounts) Move {
	return Move{Name: "mail_take", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		mark := b.Mark()
		if err := b.MailOpen(ctx); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool { return e.HasMailBox() || e.HasMailResult() })
		if err != nil || e == nil || e.HasMailResult() {
			// Busy with a shop.
			return false, ignoreDone(ctx, err)
		}
		var full, empty []uint64
		for _, m := range e.GetMailBox().GetMails() {
			if m.GetZeny() > 0 || m.GetItems() > 0 {
				full = append(full, m.GetId())
			} else {
				empty = append(empty, m.GetId())
			}
		}
		if len(empty) > 0 && (len(full) == 0 || rnd.IntN(2) == 0) {
			if err := b.MailDelete(ctx, empty[rnd.IntN(len(empty))]); err != nil {
				return false, ignoreDone(ctx, err)
			}
			return false, nil
		}
		if len(full) == 0 {
			return false, pause(ctx)
		}
		id := full[rnd.IntN(len(full))]
		mark = b.Mark()
		if err := b.MailTake(ctx, id, true, true); err != nil {
			return false, ignoreDone(ctx, err)
		}
		if rnd.IntN(10) == 0 {
			return true, drop(b, rnd, c)
		}
		e, err = waitStep(ctx, b, mark, func(e *wathenav1.Event) bool { return e.GetMailResult().GetMail() == id && e.HasMailResult() })
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case e == nil:
			return false, fmt.Errorf("taking mail %d: no answer in %s", id, stepWait)
		case e.GetMailResult().GetOutcome() != wathenav1.MailOutcome_MAIL_OUTCOME_TAKEN:
			// Too heavy, or the inventory is full.
			c.Refused.Add(1)
			return false, nil
		}
		c.Taken.Add(1)
		return true, nil
	}}
}

// BuyingStoreMove opens a buying store for 1 to 10 Jellopy at 1 to 30
// zeny, with the fixture's "!buyingstore". With a store open, the bot
// closes it one time in 4, and closes its connection with the store open
// one time in 12.
func BuyingStoreMove(c *MarketCounts) Move {
	return Move{Name: "buying_store", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		g := b.Game()
		if g.BuyingStore != nil {
			switch rnd.IntN(12) {
			case 0:
				return true, drop(b, rnd, c)
			case 1, 2, 3:
				mark := b.Mark()
				if err := b.BuyingStoreClose(ctx); err != nil {
					return false, ignoreDone(ctx, err)
				}
				_, err := waitStep(ctx, b, mark, (*wathenav1.Event).HasBuyingStoreResult)
				return false, ignoreDone(ctx, err)
			}
			return false, pause(ctx)
		}
		zeny := g.Stats[wathenav1.StatId_STAT_ID_ZENY]
		if zeny <= 0 || g.Count(Jellopy) == 0 {
			return false, pause(ctx)
		}
		mark := b.Mark()
		if err := b.Chat(ctx, "!buyingstore 1"); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
			return e.HasBuyingStoreSetup() || e.GetChat().GetText() == "e2e buyingstore: false"
		})
		if err != nil || e == nil || !e.HasBuyingStoreSetup() {
			c.Refused.Add(1)
			return false, ignoreDone(ctx, err)
		}
		line := botclient.BuyLine{ItemID: Jellopy, Amount: 1 + rnd.IntN(10), Price: uint32(1 + rnd.IntN(30))} // #nosec G115 -- at most 30.
		limit := uint32(min(zeny, 500))                                                                       // #nosec G115 -- at most 500.
		mark = b.Mark()
		if err := b.BuyingStoreOpen(ctx, b.Name+" buys", limit, []botclient.BuyLine{line}); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err = waitStep(ctx, b, mark, (*wathenav1.Event).HasBuyingStoreResult)
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case e == nil:
			return false, fmt.Errorf("opening a buying store: no answer in %s", stepWait)
		case e.GetBuyingStoreResult().GetOutcome() != wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_OPENED:
			// The bot sold its last Jellopy meanwhile, or spent its zeny.
			c.Refused.Add(1)
			return false, nil
		}
		c.StoresOpened.Add(1)
		return true, nil
	}}
}

// SellToStoreMove browses a random buying store in view and sells it
// part of a Jellopy stack. One time in 10 the bot sends the sale and
// closes its connection at once.
func SellToStoreMove(c *MarketCounts) Move {
	return Move{Name: "sell_to_store", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		stores := storeOwners(b, true)
		held := itemsOf(b.Game().Items, Jellopy)
		if len(stores) == 0 || len(held) == 0 {
			return false, pause(ctx)
		}
		owner := stores[rnd.IntN(len(stores))]
		mark := b.Mark()
		if err := b.BuyingStoreBrowse(ctx, owner.ID); err != nil {
			return false, ignoreDone(ctx, err)
		}
		e, err := waitStep(ctx, b, mark, func(e *wathenav1.Event) bool {
			return e.GetBuyingStoreList().GetBuyer() == owner.ID && e.HasBuyingStoreList()
		})
		if err != nil || e == nil {
			return false, ignoreDone(ctx, err)
		}
		list := e.GetBuyingStoreList()
		want := 0
		for _, it := range list.GetItems() {
			if it.GetItemId() == Jellopy {
				want = int(it.GetAmount())
			}
		}
		if want == 0 {
			return false, nil
		}
		it := held[rnd.IntN(len(held))]
		amount := 1 + rnd.IntN(min(it.Amount, want))
		mark = b.Mark()
		if err := b.BuyingStoreSell(ctx, owner.ID, list.GetStore(), []botclient.SellLine{{Slot: it.Slot, ItemID: Jellopy, Amount: amount}}); err != nil {
			return false, ignoreDone(ctx, err)
		}
		if rnd.IntN(10) == 0 {
			return true, drop(b, rnd, c)
		}
		e, err = waitStep(ctx, b, mark, (*wathenav1.Event).HasBuyingStoreResult)
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case e == nil:
			c.Refused.Add(1)
			return false, nil
		case e.GetBuyingStoreResult().GetOutcome() != wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_SOLD:
			c.Refused.Add(1)
			return false, nil
		}
		c.SoldToStores.Add(1)
		return true, nil
	}}
}
