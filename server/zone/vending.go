package zone

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Vending shops, buying stores and the store search (phase 5 step 7,
// proposal 0002, review decision 30), as rAthena's vending.cpp,
// buyingstore.cpp and searchstore.cpp:
//   - The Vending skill opens the setup window. The shop sells cart
//     slots at the player's prices. A buyer pays the price, and the
//     seller gets it less the vending tax, vending_tax of purchases of
//     at least vending_tax_min.
//   - player:open_buying_store opens the buying store setup. The store
//     buys items the owner holds one of already, up to a zeny limit.
//   - Each sale moves the items and the zeny of both players in one
//     transfer, and joins one commit with both characters, the cart and
//     the store's row.
//   - The vend and buying_store_sale hooks may refuse a sale. The events
//     run after it.
//   - Each zone puts a copy of its stores in the world's market
//     registry, which the store search reads.
//   - player:autotrade closes the connection and keeps the store open.
//     The store's row is saved, so the store opens again after a
//     restart.

// Vending and buying store limits.
const (
	// maxVending is MAX_VENDING.
	maxVending = 12
	// maxStoreTitle is MESSAGE_SIZE less its ending zero.
	maxStoreTitle = 79
	// buyingStoreMaxPrice and buyingStoreMaxAmount are
	// BUYINGSTORE_MAX_PRICE and BUYINGSTORE_MAX_AMOUNT.
	buyingStoreMaxPrice  = 99_990_000
	buyingStoreMaxAmount = 9999
	// maxBuyingStoreSlots is MAX_BUYINGSTORE_SLOTS.
	maxBuyingStoreSlots = 5
	// storeRange is AREA_SIZE: how far a buyer may be from a store.
	storeRange = 14
	// searchPage is SEARCHSTORE_RESULTS_PER_PAGE.
	searchPage = 10
	// majorOverweightRate is battle_config's major_overweight_rate.
	majorOverweightRate = 90
)

// vendShop is a player's open vending shop.
type vendShop struct {
	id    uint32
	title string
	lines []vendLine
}

// vendLine is one line of a vending shop: a cart slot, the amount left
// and the price of one.
type vendLine struct {
	cartSlot int
	serial   item.Serial
	amount   int
	price    int64
}

// buyStore is a player's open buying store.
type buyStore struct {
	id        uint32
	title     string
	zenyLimit int64
	lines     []buyStoreLine
}

type buyStoreLine struct {
	item   uint32
	amount int
	price  int64
}

// storeSearch is the store search window.
type storeSearch struct {
	uses    int
	open    bool
	buying  bool
	next    time.Time
	results []market.Hit
	page    int
}

// hasStore reports whether the player has a vending shop or a buying
// store open.
func (p *Player) hasStore() bool { return p.vend != nil || p.buying != nil }

// entityShop tells a viewer a player's store board.
func entityShop(id uint32, p *Player) *wathenav1.Event {
	b := wathenav1.EntityShop_builder{Id: id}
	switch {
	case p.vend != nil:
		b.Title = p.vend.title
	case p.buying != nil:
		b.Title, b.Buying = p.buying.title, true
	default:
		b.Remove = true
	}
	return wathenav1.Event_builder{EntityShop: b.Build()}.Build()
}

// market is the world's store registry, made on first use for a zone
// without one.
func (z *Zone) market() *market.Registry {
	if z.Market == nil {
		z.Market = market.NewRegistry()
	}
	return z.Market
}

// marketRules are the world's vending, store search and mail rules.
func (z *Zone) marketRules() rules.Market { return rules.MarketOrDefault(z.worldRules()) }

// storeKey is the market key of a player's store.
func storeKey(p *Player, buying bool) market.Key {
	if p.record.ID > 0 {
		return market.Key{Owner: p.record.ID, Buying: buying}
	}
	return market.Key{Name: strings.ToLower(p.Name), Buying: buying}
}

// storeCopy is a copy of the player's store for the market registry.
func (z *Zone) storeCopy(p *Player, buying bool) *market.Store {
	s := &market.Store{Owner: p.record.ID, Name: p.Name, Buying: buying, Map: z.Map, X: p.entity.x, Y: p.entity.y,
		Dir: p.entity.dir, Autotrade: p.autotrade}
	if buying {
		b := p.buying
		s.ID, s.Title, s.ZenyLimit = b.id, b.title, b.zenyLimit
		for i, l := range b.lines {
			s.Lines = append(s.Lines, market.Line{Index: i, Slot: item.Slot{ID: l.item, Amount: l.amount}, Price: l.price})
		}
		return s
	}
	v := p.vend
	s.ID, s.Title = v.id, v.title
	cart := z.cartOf(p)
	if cart == nil {
		return s
	}
	cart.Lock()
	defer cart.Unlock()
	box := cart.Box()
	for i, l := range v.lines {
		c := box.Slot(l.cartSlot)
		if c == nil || c.ID == 0 {
			continue
		}
		cp := *c
		cp.Cards, cp.Extra, cp.Amount = slices.Clone(c.Cards), c.Clone(), l.amount
		s.Lines = append(s.Lines, market.Line{Index: i, Slot: cp, CartSlot: l.cartSlot, Price: l.price})
	}
	return s
}

// publishStore puts the player's store in the market registry and in the
// next commit, after it opened or changed.
func (z *Zone) publishStore(p *Player, buying bool) {
	s := z.storeCopy(p, buying)
	z.market().Put(s.Clone())
	z.storeWrite(market.Write{Key: s.KeyOf(), Store: s})
}

// unpublishStore takes the player's store out of the registry and the
// database.
func (z *Zone) unpublishStore(p *Player, buying bool) {
	k := storeKey(p, buying)
	z.market().Remove(k)
	z.storeWrite(market.Write{Key: k})
}

// storeWrite queues a store's row for the next commit. A later write of
// the store replaces an earlier one.
func (z *Zone) storeWrite(w market.Write) {
	if i := slices.IndexFunc(z.txStores, func(o market.Write) bool { return o.Key == w.Key }); i >= 0 {
		z.txStores = slices.Delete(z.txStores, i, i+1)
	}
	z.txStores = append(z.txStores, w)
}

// npcNear is npc_isnear: an NPC within min_npc_vendchat_distance.
func (z *Zone) npcNear(e *entity) bool {
	d := z.marketRules().NPCDistance
	if d <= 0 {
		return false
	}
	for o := range z.entities.all() {
		if o.npc != nil && abs(o.x-e.x) <= d && abs(o.y-e.y) <= d {
			return true
		}
	}
	return false
}

// vendingSkill is MC_VENDING: it opens the vending setup window with 2
// lines and 1 per level, as skill_castend_nodamage_id.
type vendingSkill struct{}

func (vendingSkill) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	p := src.player
	switch {
	case p == nil:
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	case !z.hasCart(p):
		return wathenav1.SkillFailure_SKILL_FAILURE_CART
	case p.hasStore() || p.trade != nil || z.npcNear(src):
		// skill_check_condition_castbegin: npc_isnear refuses it.
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (vendingSkill) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil || !z.hasCart(p) || p.hasStore() {
		return
	}
	p.prevend = min(2+lv, maxVending)
	z.skillEffect(src, src, d, lv, int64(lv), true)
	p.pending = append(p.pending, wathenav1.Event_builder{VendingSetup: wathenav1.VendingSetup_builder{
		MaxItems: uint32(p.prevend)}.Build()}.Build()) // #nosec G115 -- at most 12.
}

func vendingResult(o wathenav1.VendingOutcome, index uint32, msg string) *wathenav1.Event {
	return wathenav1.Event_builder{VendingResult: wathenav1.VendingResult_builder{Outcome: o, Index: index, Message: msg}.Build()}.Build()
}

func buyingResult(o wathenav1.BuyingStoreOutcome, item uint32, msg string) *wathenav1.Event {
	return wathenav1.Event_builder{BuyingStoreResult: wathenav1.BuyingStoreResult_builder{Outcome: o, ItemId: item, Message: msg}.Build()}.Build()
}

// vendingCommand runs a vending command.
func (z *Zone) vendingCommand(p *Player, c *wathenav1.VendingCommand) {
	switch {
	case c.HasOpen():
		z.openVending(p, c.GetOpen())
	case c.HasClose():
		if p.vend != nil {
			z.closeVending(p)
		}
	case c.HasBrowse():
		z.browseVending(p, c.GetBrowse())
	case c.HasBuy():
		z.vendingBuy(p, c.GetBuy())
	case c.HasCancel():
		p.prevend = 0
	}
}

// openVending is vending_openvending.
func (z *Zone) openVending(p *Player, o *wathenav1.VendingOpen) {
	fail := func(out wathenav1.VendingOutcome) {
		p.prevend = 0
		p.pending = append(p.pending, vendingResult(out, 0, ""))
	}
	if p.prevend == 0 || p.entity.dead || p.trade != nil || p.hasStore() {
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED)
		return
	}
	cart := z.cartOf(p)
	lines := o.GetItems()
	title := strings.TrimSpace(o.GetTitle())
	switch {
	case cart == nil || !z.hasCart(p) || p.skillLevel(skill.IDVending) < 1:
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED)
		return
	case len(lines) < 1 || len(lines) > p.prevend:
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED)
		return
	case title == "" || len(o.GetTitle()) > maxStoreTitle || !utf8.ValidString(title):
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED)
		return
	case z.npcNear(p.entity):
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_TOO_CLOSE)
		return
	}
	mr := z.marketRules()
	v := &vendShop{title: title}
	var total int64
	cart.Lock()
	box := z.prepareCart(cart)
	for _, l := range lines {
		slot, amount := int(l.GetCartSlot()), int(l.GetAmount())
		s := box.Slot(slot)
		// vending_openvending's filter, which fails the whole shop.
		if s == nil || s.ID == 0 || amount < 1 || amount > s.Amount || s.Unidentified || s.Broken || s.Expires != 0 ||
			!item.BoundAllows(s.Bound, item.PlaceTrade, false) || z.Items.Restricted(s, item.NoTrade) ||
			slices.ContainsFunc(v.lines, func(o vendLine) bool { return o.cartSlot == slot }) {
			cart.Unlock()
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_RESTRICTED)
			return
		}
		price := min(max(int64(l.GetPrice()), 1), mr.VendingMaxValue)
		v.lines = append(v.lines, vendLine{cartSlot: slot, serial: s.Serial, amount: amount, price: price})
		total += price * int64(amount)
	}
	cart.Unlock()
	if !mr.VendingOverMax && p.record.Zeny+total > transfer.MaxZeny {
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED)
		return
	}
	p.prevend = 0
	v.id = z.market().NewID()
	p.vend = v
	p.entity.shopChanged = true
	z.publishStore(p, false)
	z.sendOwnVending(p)
	p.pending = append(p.pending, vendingResult(wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED, 0, ""))
}

// vendingItems lists a shop's lines as the client shows them.
func (z *Zone) vendingItems(p *Player) []*wathenav1.VendingListItem {
	cart := z.cartOf(p)
	if cart == nil || p.vend == nil {
		return nil
	}
	cart.Lock()
	defer cart.Unlock()
	box := z.prepareCart(cart)
	var out []*wathenav1.VendingListItem
	for i, l := range p.vend.lines {
		s := box.Slot(l.cartSlot)
		if s == nil || s.ID == 0 {
			continue
		}
		out = append(out, wathenav1.VendingListItem_builder{Index: uint32(i), Item: z.itemMessage(l.cartSlot, s, l.amount), // #nosec G115 -- at most 12.
			Price: uint32(l.price)}.Build()) // #nosec G115 -- at most vending_max_value.
	}
	return out
}

// sendOwnVending shows the player its own shop, as clif_openvending.
func (z *Zone) sendOwnVending(p *Player) {
	p.pending = append(p.pending, wathenav1.Event_builder{VendingOpened: wathenav1.VendingOpened_builder{
		Title: p.vend.title, Items: z.vendingItems(p)}.Build()}.Build())
}

// closeVending is vending_closevending.
func (z *Zone) closeVending(p *Player) {
	if p.vend == nil {
		return
	}
	p.vend = nil
	p.entity.shopChanged = true
	z.unpublishStore(p, false)
	p.pending = append(p.pending, vendingResult(wathenav1.VendingOutcome_VENDING_OUTCOME_CLOSED, 0, ""))
	z.storeClosed(p)
}

// storeClosed lets an autotrader go once its store closed, as
// vending_purchasereq's map_quit.
func (z *Zone) storeClosed(p *Player) {
	if p.autotrade && !p.hasStore() {
		p.autotrade = false
		z.endAutotrade(p)
	}
}

// playerByNet finds a player of the zone by the viewer's entity ID.
func (z *Zone) playerByNet(p *Player, netID uint32) *Player {
	for e, id := range p.netIDs.all() {
		if id == netID && e.player != nil && z.players[e.player.charID] == e.player {
			return e.player
		}
	}
	return nil
}

// storeOwner finds the owner of a store a command names: by entity ID
// near the player, or the store a store search opened when id is 0.
func (z *Zone) storeOwner(p *Player, id uint32, buying bool) (*Player, bool) {
	if id == 0 {
		k := p.remote
		if k == (market.Key{}) || k.Buying != buying {
			return nil, false
		}
		for _, o := range z.players {
			if storeKey(o, buying) == k {
				return o, true
			}
		}
		return nil, false
	}
	o := z.playerByNet(p, id)
	if o == nil {
		return nil, false
	}
	remote := p.remote == storeKey(o, buying)
	if !remote && abs(o.entity.x-p.entity.x) > storeRange || !remote && abs(o.entity.y-p.entity.y) > storeRange {
		return nil, false
	}
	return o, remote
}

// browseVending is vending_vendinglistreq.
func (z *Zone) browseVending(p *Player, netID uint32) {
	v, _ := z.storeOwner(p, netID, false)
	if v == nil || v.vend == nil || v == p {
		return
	}
	z.sendVendingList(p, v, netID)
}

func (z *Zone) sendVendingList(p, v *Player, netID uint32) {
	p.pending = append(p.pending, wathenav1.Event_builder{VendingList: wathenav1.VendingList_builder{
		Vendor: netID, Shop: v.vend.id, Title: v.vend.title, Seller: v.Name, Items: z.vendingItems(v)}.Build()}.Build())
}

// txParty is one player of a transfer that changes two players: its
// inventory, its wallet, and its cart when the transfer touches it.
type txParty struct {
	p      *Player
	inv    *transfer.InventoryBox
	wallet *transfer.ZenyWallet
	cart   *transfer.ListBox
	reason wathenav1.InventoryRemoveReason
}

// applyShared runs a transfer of two players and tells each of its
// changes, as tradeCommit does. Both join the next commit.
func (z *Zone) applyShared(tx *transfer.Tx, parties ...*txParty) error {
	if z.Clock != nil {
		tx.Now = z.Clock()
	}
	logs, err := tx.Apply(func(c transfer.Change) {
		for _, t := range parties {
			switch {
			case c.Wallet != nil && c.Wallet == transfer.Wallet(t.wallet):
				if s := z.battleStats(t.p); s != nil {
					t.p.pending = append(t.p.pending, s)
				}
			case t.cart != nil && c.Box == transfer.Box(t.cart):
				ev := wathenav1.StorageRemove_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_CART, Slot: uint32(c.Slot), // #nosec G115 -- a cart slot.
					Amount: uint32(-c.Amount), Weight: uint32(max(t.cart.Weight, 0))} // #nosec G115 -- a stack and a weight.
				t.p.pending = append(t.p.pending, wathenav1.Event_builder{StorageRemove: ev.Build()}.Build())
			case c.Box == transfer.Box(t.inv) && c.Amount > 0:
				z.sendAdd(t.p, c.Slot, c.Amount)
			case c.Box == transfer.Box(t.inv):
				t.p.pending = append(t.p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
					Slot: uint32(c.Slot), Amount: uint32(-c.Amount), Reason: t.reason, // #nosec G115 -- a slot and a stack.
				}.Build()}.Build())
				z.updateWeight(t.p)
			default:
				continue
			}
			break
		}
	})
	if err != nil {
		return err
	}
	z.recordTransfer(parties[0].p, logs)
	for _, t := range parties[1:] {
		if !t.p.transferred {
			t.p.transferred = true
			z.txPlayers = append(z.txPlayers, t.p)
		}
	}
	return nil
}

// vendingBuy is vending_purchasereq. The order of the checks follows it.
func (z *Zone) vendingBuy(p *Player, b *wathenav1.VendingBuy) {
	fail := func(o wathenav1.VendingOutcome, index uint32, msg string) {
		p.pending = append(p.pending, vendingResult(o, index, msg))
	}
	v, _ := z.storeOwner(p, b.GetVendor(), false)
	if v == nil || v.vend == nil || v == p || p.entity.dead {
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED, 0, "")
		return
	}
	if v.vend.id != b.GetShop() {
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_STORE_CHANGED, 0, "")
		return
	}
	lines := b.GetItems()
	shop := v.vend
	if len(lines) < 1 || len(lines) > maxVending || len(lines) > len(shop.lines) {
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED, 0, "")
		return
	}
	cart := z.cartOf(v)
	if cart == nil {
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED, 0, "")
		return
	}
	cart.Lock()
	locked := true
	defer func() {
		if locked {
			cart.Unlock()
		}
	}()
	cbox := z.prepareCart(cart)
	left := make([]int, len(shop.lines))
	for i, l := range shop.lines {
		left[i] = l.amount
	}
	var total int64
	weight := 0
	newSlots := 0
	var sold []modscript.SaleItem
	for _, l := range lines {
		i, amount := int(l.GetIndex()), int(l.GetAmount())
		if amount <= 0 || i < 0 || i >= len(shop.lines) {
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED, l.GetIndex(), "")
			return
		}
		sl := shop.lines[i]
		s := cbox.Slot(sl.cartSlot)
		if s == nil || s.ID == 0 || s.Serial != sl.serial {
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_OUT_OF_STOCK, l.GetIndex(), "")
			return
		}
		total += sl.price * int64(amount)
		if total > p.record.Zeny || total > transfer.MaxZeny {
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_NOT_ENOUGH_ZENY, l.GetIndex(), "")
			return
		}
		if total+v.record.Zeny > transfer.MaxZeny {
			// Too much zeny for the seller.
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_OUT_OF_STOCK, l.GetIndex(), "")
			return
		}
		d := z.Items.Get(s.ID)
		weight += d.Weight * amount
		if weight+p.inventory.Weight > p.maxWeight {
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_OVERWEIGHT, l.GetIndex(), "")
			return
		}
		// The cart and the shop may disagree: the cart wins.
		left[i] = min(left[i], s.Amount)
		if left[i] < amount {
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_OUT_OF_STOCK, l.GetIndex(), "")
			return
		}
		left[i] -= amount
		switch checkAddItem(p.inventory, d, amount) {
		case addNew:
			newSlots++
			if newSlots > p.inventory.Blank() {
				fail(wathenav1.VendingOutcome_VENDING_OUTCOME_OVERWEIGHT, l.GetIndex(), "")
				return
			}
		case addOverAmount:
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_OVERWEIGHT, l.GetIndex(), "")
			return
		}
		sold = append(sold, modscript.SaleItem{Item: s.ID, Amount: amount, Price: sl.price})
	}
	ev := modscript.SaleEvent{Seller: int64(v.charID), Buyer: int64(p.charID), Items: sold, Zeny: total}
	if r := z.scripts; r != nil && r.HasHooks("vend") {
		if ok, msg := r.RunSaleHook("vend", ev); !ok {
			fail(wathenav1.VendingOutcome_VENDING_OUTCOME_HOOK, 0, msg)
			return
		}
	}
	// pc_payzeny, vending_calc_tax and pc_getzeny, then pc_additem and
	// pc_cart_delitem, in one transfer.
	kept := z.marketRules().VendingTaxOf(total)
	buyer := &txParty{p: p, inv: z.box(p), wallet: z.wallet(p)}
	seller := &txParty{p: v, inv: z.box(v), wallet: z.wallet(v), cart: cbox, reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_SOLD}
	tx := transfer.New(transfer.ActionVend)
	if kept > 0 {
		tx.MoveZeny(buyer.wallet, kept, seller.wallet)
	}
	if total > kept {
		tx.Pay(buyer.wallet, total-kept, transfer.End(transfer.KindFee))
	}
	for _, l := range lines {
		sl := shop.lines[l.GetIndex()]
		tx.Move(cbox, sl.cartSlot, sl.serial, int(l.GetAmount()), buyer.inv)
	}
	if err := z.applyShared(tx, buyer, seller); err != nil {
		fail(wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED, 0, "")
		return
	}
	z.personalChanged(cart)
	for _, l := range lines {
		i := l.GetIndex()
		sl := &shop.lines[i]
		sl.amount -= int(l.GetAmount())
		// vending_purchasereq reports each line's taxed price.
		zeny := z.marketRules().VendingTaxOf(sl.price * int64(l.GetAmount()))
		v.pending = append(v.pending, wathenav1.Event_builder{VendingSold: wathenav1.VendingSold_builder{
			Index: i, Amount: l.GetAmount(), Buyer: p.Name, Zeny: uint32(zeny), // #nosec G115 -- at most MAX_ZENY.
		}.Build()}.Build())
		if s := cbox.Slot(sl.cartSlot); s != nil && s.ID != 0 {
			sl.serial = s.Serial
		}
	}
	// Compact the list, as vending_purchasereq.
	shop.lines = slices.DeleteFunc(shop.lines, func(l vendLine) bool { return l.amount <= 0 })
	p.pending = append(p.pending, vendingResult(wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT, 0, ""))
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "vend", sale: &ev})
	cart.Unlock()
	locked = false
	if len(shop.lines) == 0 {
		z.closeVending(v)
	} else {
		z.publishStore(v, false)
		z.sendOwnVending(v)
		z.sendVendingList(p, v, b.GetVendor())
	}
}

// buyingCommand runs a buying store command.
func (z *Zone) buyingCommand(p *Player, c *wathenav1.BuyingStoreCommand) {
	switch {
	case c.HasOpen():
		z.openBuyingStore(p, c.GetOpen())
	case c.HasClose():
		if p.buying != nil {
			z.closeBuyingStore(p, wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_CLOSED)
		}
	case c.HasBrowse():
		o, _ := z.storeOwner(p, c.GetBrowse(), true)
		if o != nil && o.buying != nil && o != p {
			z.sendBuyingList(p, o, c.GetBrowse())
		}
	case c.HasSell():
		z.buyingStoreSell(p, c.GetSell())
	case c.HasCancel():
		p.buySetup = 0
	}
}

// setupBuyingStore is buyingstore_setup.
func (z *Zone) setupBuyingStore(p *Player, slots int) bool {
	if p.hasStore() || p.trade != nil || slots <= 0 {
		return false
	}
	p.buySetup = min(slots, maxBuyingStoreSlots)
	p.pending = append(p.pending, wathenav1.Event_builder{BuyingStoreSetup: wathenav1.BuyingStoreSetup_builder{
		Slots: uint32(p.buySetup)}.Build()}.Build()) // #nosec G115 -- at most 5.
	return true
}

// openBuyingStore is buyingstore_create. The order of the checks follows
// it.
func (z *Zone) openBuyingStore(p *Player, o *wathenav1.BuyingStoreOpen) {
	fail := func(out wathenav1.BuyingStoreOutcome) {
		p.buySetup = 0
		p.pending = append(p.pending, buyingResult(out, 0, ""))
	}
	lines := o.GetItems()
	limit := int64(o.GetZenyLimit())
	title := strings.TrimSpace(o.GetTitle())
	if len(lines) == 0 || p.trade != nil || p.buySetup == 0 || len(lines) > p.buySetup || limit <= 0 || limit > p.record.Zeny ||
		title == "" || len(o.GetTitle()) > maxStoreTitle || !utf8.ValidString(title) || p.hasStore() || z.npcNear(p.entity) {
		fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED)
		return
	}
	b := &buyStore{title: title, zenyLimit: limit}
	weight := int64(p.inventory.Weight)
	for _, l := range lines {
		d, ok := z.Items.Lookup(l.GetItemId())
		amount, price := int(l.GetAmount()), int64(l.GetPrice())
		if !ok || amount == 0 || price <= 0 || price > buyingStoreMaxPrice || !d.BuyingStore ||
			z.Items.Restricted(&item.Slot{ID: d.ID}, item.NoTrade) {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED)
			return
		}
		held := inventoryCount(p.inventory, d.ID)
		if held == 0 || held+amount > buyingStoreMaxAmount ||
			slices.ContainsFunc(b.lines, func(o buyStoreLine) bool { return o.item == d.ID }) {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED)
			return
		}
		weight += int64(d.Weight) * int64(amount)
		b.lines = append(b.lines, buyStoreLine{item: d.ID, amount: amount, price: price})
	}
	if p.maxWeight > 0 && weight*100/int64(p.maxWeight) >= majorOverweightRate {
		fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_OVERWEIGHT)
		return
	}
	p.buySetup = 0
	b.id = z.market().NewID()
	p.buying = b
	p.entity.shopChanged = true
	z.publishStore(p, true)
	z.sendOwnBuying(p)
	p.pending = append(p.pending, buyingResult(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_OPENED, 0, ""))
}

func (z *Zone) buyingItems(b *buyStore) []*wathenav1.BuyingStoreItem {
	var out []*wathenav1.BuyingStoreItem
	for _, l := range b.lines {
		out = append(out, wathenav1.BuyingStoreItem_builder{ItemId: l.item, Amount: uint32(l.amount), // #nosec G115 -- at most 9999.
			Price: uint32(l.price)}.Build()) // #nosec G115 -- at most BUYINGSTORE_MAX_PRICE.
	}
	return out
}

// sendOwnBuying shows the owner its store, as
// clif_buyingstore_myitemlist.
func (z *Zone) sendOwnBuying(p *Player) {
	p.pending = append(p.pending, wathenav1.Event_builder{BuyingStoreOpened: wathenav1.BuyingStoreOpened_builder{
		Title: p.buying.title, ZenyLimit: uint32(p.buying.zenyLimit), Items: z.buyingItems(p.buying), // #nosec G115 -- at most MAX_ZENY.
	}.Build()}.Build())
}

func (z *Zone) sendBuyingList(p, o *Player, netID uint32) {
	b := o.buying
	p.pending = append(p.pending, wathenav1.Event_builder{BuyingStoreList: wathenav1.BuyingStoreList_builder{
		Buyer: netID, Store: b.id, Title: b.title, Owner: o.Name, ZenyLimit: uint32(min(b.zenyLimit, o.record.Zeny)), // #nosec G115 -- at most MAX_ZENY.
		Items: z.buyingItems(b)}.Build()}.Build())
}

// closeBuyingStore is buyingstore_close.
func (z *Zone) closeBuyingStore(p *Player, why wathenav1.BuyingStoreOutcome) {
	if p.buying == nil {
		return
	}
	p.buying = nil
	p.entity.shopChanged = true
	z.unpublishStore(p, true)
	p.pending = append(p.pending, buyingResult(why, 0, ""))
	z.storeClosed(p)
}

// buyingStoreSell is buyingstore_trade.
func (z *Zone) buyingStoreSell(p *Player, s *wathenav1.BuyingStoreSell) {
	fail := func(o wathenav1.BuyingStoreOutcome, id uint32, msg string) {
		p.pending = append(p.pending, buyingResult(o, id, msg))
	}
	lines := s.GetItems()
	if len(lines) == 0 {
		return
	}
	if p.trade != nil || p.entity.dead {
		fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, 0, "")
		return
	}
	b, _ := z.storeOwner(p, s.GetBuyer(), true)
	if b == nil || b == p || b.buying == nil || b.buying.id != s.GetStore() {
		fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, 0, "")
		return
	}
	store := b.buying
	if b.record.Zeny < store.zenyLimit {
		store.zenyLimit = b.record.Zeny
	}
	weight := b.inventory.Weight
	var zeny int64
	seen := map[int]bool{}
	// taken is the amount the lines so far sell of each store line. Two
	// slots of the same item count together, so a sale cannot pass a
	// line's amount. rAthena checks each slot alone.
	taken := map[int]int{}
	var sold []modscript.SaleItem
	for _, l := range lines {
		slot, id, amount := int(l.GetSlot()), l.GetItemId(), int(l.GetAmount())
		if seen[slot] {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, id, "")
			return
		}
		seen[slot] = true
		if slot < 0 || slot >= len(p.inventory.Slots) || amount < 1 {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, id, "")
			return
		}
		it := &p.inventory.Slots[slot]
		if it.ID != id || it.Amount < amount || it.Equip != 0 {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, id, "")
			return
		}
		// Bound copies, untradeable items and items with cards are not
		// sold, as buyingstore_trade.
		if !item.BoundAllows(it.Bound, item.PlaceTrade, false) || z.Items.Restricted(it, item.NoTrade) || hasCard(it.Cards) || it.Expires != 0 {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, id, "")
			return
		}
		i := slices.IndexFunc(store.lines, func(o buyStoreLine) bool { return o.item == id })
		if i < 0 || store.lines[i].amount == 0 {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, id, "")
			return
		}
		if store.lines[i].amount-taken[i] < amount {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_COUNT, id, "")
			return
		}
		taken[i] += amount
		d := z.Items.Get(id)
		if checkAddItem(b.inventory, d, taken[i]) == addOverAmount || amount*d.Weight > b.maxWeight-weight {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, id, "")
			return
		}
		weight += amount * d.Weight
		if int64(amount)*store.lines[i].price > store.zenyLimit-zeny {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_ZENY, id, "")
			return
		}
		zeny += int64(amount) * store.lines[i].price
		sold = append(sold, modscript.SaleItem{Item: id, Amount: amount, Price: store.lines[i].price})
	}
	ev := modscript.SaleEvent{Seller: int64(p.charID), Buyer: int64(b.charID), Items: sold, Zeny: zeny}
	if r := z.scripts; r != nil && r.HasHooks("buying_store_sale") {
		if ok, msg := r.RunSaleHook("buying_store_sale", ev); !ok {
			fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_HOOK, 0, msg)
			return
		}
	}
	seller := &txParty{p: p, inv: z.box(p), wallet: z.wallet(p), reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_SOLD_TO_STORE}
	buyer := &txParty{p: b, inv: z.box(b), wallet: z.wallet(b)}
	tx := transfer.New(transfer.ActionBuyingStore)
	for _, l := range lines {
		slot := int(l.GetSlot())
		tx.Move(seller.inv, slot, p.inventory.Slots[slot].Serial, int(l.GetAmount()), buyer.inv)
	}
	if zeny > 0 {
		tx.MoveZeny(buyer.wallet, zeny, seller.wallet)
	}
	if err := z.applyShared(tx, seller, buyer); err != nil {
		fail(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, 0, "")
		return
	}
	for _, l := range lines {
		i := slices.IndexFunc(store.lines, func(o buyStoreLine) bool { return o.item == l.GetItemId() })
		line := &store.lines[i]
		line.amount -= int(l.GetAmount())
		paid := int64(l.GetAmount()) * line.price
		store.zenyLimit -= paid
		b.pending = append(b.pending, wathenav1.Event_builder{BuyingStoreBought: wathenav1.BuyingStoreBought_builder{
			ItemId: line.item, Amount: l.GetAmount(), Seller: p.Name, Zeny: uint32(paid), // #nosec G115 -- at most MAX_ZENY.
			ZenyLimit: uint32(max(store.zenyLimit, 0)), // #nosec G115 -- as above.
		}.Build()}.Build())
	}
	p.pending = append(p.pending, buyingResult(wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_SOLD, 0, ""))
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "buying_store_sale", sale: &ev})
	store.lines = slices.DeleteFunc(store.lines, func(l buyStoreLine) bool { return l.amount <= 0 })
	switch {
	case len(store.lines) == 0:
		z.closeBuyingStore(b, wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_ALL_BOUGHT)
	case store.zenyLimit <= 0:
		z.closeBuyingStore(b, wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_LIMIT_REACHED)
	default:
		z.publishStore(b, true)
		z.sendOwnBuying(b)
		z.sendBuyingList(p, b, s.GetBuyer())
	}
}

// closeStores closes a player's stores, as unit_remove_map and a map
// change do.
func (z *Zone) closeStores(p *Player) {
	p.prevend, p.buySetup, p.search = 0, 0, nil
	if p.vend != nil {
		z.closeVending(p)
	}
	if p.buying != nil {
		z.closeBuyingStore(p, wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_CLOSED)
	}
}

// openSearch is searchstore_open.
func (z *Zone) openSearch(p *Player, uses int, open bool) bool {
	if p.search != nil {
		return false
	}
	p.search = &storeSearch{uses: uses, open: open}
	p.pending = append(p.pending, wathenav1.Event_builder{StoreSearchOpen: wathenav1.StoreSearchOpen_builder{
		Uses: uint32(uses), Open: open}.Build()}.Build()) // #nosec G115 -- at most 1000.
	return true
}

func searchResult(o wathenav1.StoreSearchOutcome) *wathenav1.Event {
	return wathenav1.Event_builder{StoreSearchResult: wathenav1.StoreSearchResult_builder{Outcome: o}.Build()}.Build()
}

// searchCommand runs a store search command.
func (z *Zone) searchCommand(p *Player, c *wathenav1.StoreSearchCommand) {
	s := p.search
	if s == nil {
		return
	}
	switch {
	case c.HasQuery():
		z.searchQuery(p, c.GetQuery())
	case c.HasNext():
		if len(s.results) > s.page*searchPage {
			z.sendSearchPage(p)
		}
	case c.HasClick():
		z.searchClick(p, int(c.GetClick()))
	case c.HasClose():
		p.search, p.remote = nil, market.Key{}
	}
}

// searchQuery is searchstore_query.
func (z *Zone) searchQuery(p *Player, q *wathenav1.StoreSearchQuery) {
	s := p.search
	now := z.wallClock()
	if now.Before(s.next) {
		p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_TOO_FAST))
		return
	}
	if s.uses <= 0 {
		p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_NO_USES))
		return
	}
	if len(q.GetItemIds()) == 0 || len(q.GetItemIds()) > 10 || len(q.GetCardIds()) > 10 || len(q.GetRolls()) > 8 || len(q.GetOptions()) > 8 {
		return
	}
	mr := z.marketRules()
	s.uses--
	s.buying = q.GetBuying()
	s.next = now.Add(time.Duration(mr.SearchDelay) * time.Second)
	s.results, s.page = nil, 0
	p.remote = market.Key{}
	for _, id := range append(slices.Clone(q.GetItemIds()), q.GetCardIds()...) {
		if _, ok := z.Items.Lookup(id); !ok {
			p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_NOTHING))
			return
		}
	}
	lo, hi := int64(q.GetMinPrice()), int64(q.GetMaxPrice())
	if hi > 0 && hi < lo {
		lo, hi = hi, lo
	}
	query := market.Query{Buying: s.buying, Items: q.GetItemIds(), Cards: q.GetCardIds(), MinPrice: lo, MaxPrice: hi,
		Skip: storeKey(p, s.buying), Max: mr.SearchMax}
	for _, r := range q.GetRolls() {
		query.Rolls = append(query.Rolls, market.Bound{Name: r.GetName(), Min: int(r.GetMin()), Max: int(r.GetMax())})
	}
	for _, r := range q.GetOptions() {
		query.Options = append(query.Options, market.Bound{Name: r.GetName(), Min: int(r.GetMin()), Max: int(r.GetMax())})
	}
	hits, full := z.market().Search(query, func(id uint32) int { return z.Items.Get(id).Slots })
	if full {
		p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_TOO_MANY))
	}
	if len(hits) == 0 {
		p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_NOTHING))
	}
	s.results = hits
	z.sendSearchPage(p)
}

// sendSearchPage is clif_search_store_info_ack: the next page of
// results.
func (z *Zone) sendSearchPage(p *Player) {
	s := p.search
	start := s.page * searchPage
	end := min(start+searchPage, len(s.results))
	var list []*wathenav1.StoreSearchHit
	for i := start; i < end; i++ {
		h := s.results[i]
		sl := h.Line.Slot
		list = append(list, wathenav1.StoreSearchHit_builder{
			Index: uint32(i), Title: h.Store.Title, Owner: h.Store.Name, Map: h.Store.Map, // #nosec G115 -- at most searchstore_maxresults.
			X: uint32(max(h.Store.X, 0)), Y: uint32(max(h.Store.Y, 0)), // #nosec G115 -- map cells.
			Item:  z.itemMessage(h.Line.Index, &sl, sl.Amount),
			Price: uint32(h.Line.Price), Buying: h.Store.Buying, // #nosec G115 -- at most MAX_ZENY.
		}.Build())
	}
	if end > start {
		s.page++
	}
	p.pending = append(p.pending, wathenav1.Event_builder{StoreSearchResults: wathenav1.StoreSearchResults_builder{
		Results: list, Uses: uint32(max(s.uses, 0)), More: len(s.results) > s.page*searchPage, // #nosec G115 -- at most 1000.
	}.Build()}.Build())
}

// searchClick is searchstore_click.
func (z *Zone) searchClick(p *Player, i int) {
	s := p.search
	p.remote = market.Key{}
	if i < 0 || i >= len(s.results) {
		p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_GONE))
		return
	}
	h := s.results[i]
	cur := z.market().Get(h.Store.KeyOf())
	if cur == nil || cur.ID != h.Store.ID || !slices.ContainsFunc(cur.Lines, func(l market.Line) bool {
		return l.Slot.ID == h.Line.Slot.ID && l.Slot.Amount > 0
	}) {
		p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_GONE))
		return
	}
	if !s.open {
		p.pending = append(p.pending, wathenav1.Event_builder{StoreSearchResult: wathenav1.StoreSearchResult_builder{
			Outcome: wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_POSITION, Map: cur.Map,
			X: uint32(max(cur.X, 0)), Y: uint32(max(cur.Y, 0)), // #nosec G115 -- map cells.
		}.Build()}.Build())
		return
	}
	if cur.Map != z.Map {
		// The store's zone is another; a remote purchase across zones
		// is not built.
		p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_OTHER_MAP))
		return
	}
	k := cur.KeyOf()
	for _, o := range z.players {
		if storeKey(o, k.Buying) != k {
			continue
		}
		p.remote = k
		if k.Buying && o.buying != nil {
			z.sendBuyingList(p, o, 0)
		} else if o.vend != nil {
			z.sendVendingList(p, o, 0)
		}
		return
	}
	p.pending = append(p.pending, searchResult(wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_GONE))
}

// startAutotrade is @autotrade: the connection closes, and the store
// stays.
func (z *Zone) startAutotrade(p *Player) bool {
	if !p.hasStore() || p.autotrade || p.sess == nil {
		return false
	}
	p.autotrade = true
	z.closeTalk(p)
	z.publishStore(p, p.buying != nil)
	z.queue(p, wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
		Reason:  wathenav1.DisconnectReason_DISCONNECT_REASON_AUTOTRADE,
		Message: "Your shop stays open while you are away.",
	}.Build()}.Build())
	if t := z.Autotraders; t != nil {
		t.add(p.record.ID, p.sess)
	}
	// The messages before this one go out; then the connection closes
	// and the player stays.
	z.closeOut(p)
	p.detached = true
	return true
}

// endAutotrade lets an autotrader leave the world, as map_quit.
func (z *Zone) endAutotrade(p *Player) {
	p.autotrade = false
	if t := z.Autotraders; t != nil {
		t.remove(p.record.ID, p.sess)
	}
	z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
}

// detachedLeave handles the gateway's Leave of an autotrader: its
// connection closed, and the player stays with its store. It returns
// false for a player that must leave.
func (z *Zone) detachedLeave(p *Player) bool {
	if !p.autotrade || !p.hasStore() {
		return false
	}
	z.save(p, false)
	if p.sess != nil {
		p.sess.done()
	}
	return true
}

// inventoryCount is how many of an item an inventory holds.
func inventoryCount(inv *item.Inventory, id uint32) int {
	n := 0
	for i := range inv.Slots {
		if inv.Slots[i].ID == id {
			n += inv.Slots[i].Amount
		}
	}
	return n
}

// Autotraders are the world's players on autotrade, by character ID, so
// a new login of the character ends the autotrade first.
type Autotraders struct {
	mu   sync.Mutex
	sess map[int64]*Session
}

// NewAutotraders makes an empty list.
func NewAutotraders() *Autotraders { return &Autotraders{sess: map[int64]*Session{}} }

func (a *Autotraders) add(id int64, s *Session) {
	if id <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sess[id] = s
}

func (a *Autotraders) remove(id int64, s *Session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sess[id] == s {
		delete(a.sess, id)
	}
}

// Len is how many players are on autotrade.
func (a *Autotraders) Len() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.sess)
}

// End ends the autotrade of a character, if it has one, and waits until
// its last save went to its saver, so a new login loads what it saved.
func (a *Autotraders) End(ctx context.Context, id int64) {
	a.mu.Lock()
	s := a.sess[id]
	a.mu.Unlock()
	if s == nil {
		return
	}
	z, p := s.where()
	if z == nil {
		return
	}
	done := make(chan struct{})
	go z.post(func() {
		defer close(done)
		if z.players[p.charID] == p && p.autotrade {
			z.closeStores(p)
			if p.autotrade {
				z.endAutotrade(p)
			}
		}
	})
	// The leave hands the last save to the saver before done closes, so
	// the store's load waits for it.
	select {
	case <-done:
	case <-z.stopped:
	case <-ctx.Done():
	}
}

// RestoreStore opens an autotrade store again after a restart, as
// vending_reopen and buyingstore_reopen: the session's player opens st
// on autotrade, without a connection. A vending line names its cart
// stack by serial, and one whose stack is gone is left out. done gets
// whether the store opened; when it did not, the player leaves.
func (s *Session) RestoreStore(st *market.Store, done func(bool)) {
	z, p := s.where()
	if z == nil {
		done(false)
		return
	}
	z.post(func() {
		ok := z.players[p.charID] == p && z.restoreStore(p, st)
		if !ok && z.players[p.charID] == p {
			z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
		}
		done(ok)
	})
}

func (z *Zone) restoreStore(p *Player, st *market.Store) bool {
	p.detached = true
	if st.Buying {
		b := &buyStore{title: st.Title, zenyLimit: min(st.ZenyLimit, p.record.Zeny)}
		for _, l := range st.Lines {
			if _, ok := z.Items.Lookup(l.Slot.ID); ok && l.Slot.Amount > 0 && l.Price > 0 {
				b.lines = append(b.lines, buyStoreLine{item: l.Slot.ID, amount: l.Slot.Amount, price: l.Price})
			}
		}
		if len(b.lines) == 0 || b.zenyLimit <= 0 {
			return false
		}
		b.id = z.market().NewID()
		p.buying = b
	} else {
		cart := z.cartOf(p)
		if cart == nil || !z.hasCart(p) {
			return false
		}
		v := &vendShop{title: st.Title}
		cart.Lock()
		box := z.prepareCart(cart)
		for _, l := range st.Lines {
			for i := range box.Slots {
				c := &box.Slots[i]
				if c.ID != 0 && c.Serial == l.Slot.Serial {
					amount := min(l.Slot.Amount, c.Amount)
					if !z.Items.Get(c.ID).Stackable {
						amount = 1
					}
					v.lines = append(v.lines, vendLine{cartSlot: i, serial: c.Serial, amount: amount, price: l.Price})
					break
				}
			}
		}
		cart.Unlock()
		if len(v.lines) == 0 {
			return false
		}
		v.id = z.market().NewID()
		p.vend = v
	}
	if st.Dir >= 0 && st.Dir < 8 {
		p.entity.dir = st.Dir
	}
	p.autotrade = true
	p.entity.shopChanged = true
	if z.Autotraders != nil {
		z.Autotraders.add(p.record.ID, p.sess)
	}
	z.publishStore(p, st.Buying)
	return true
}
