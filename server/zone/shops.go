package zone

import (
	"maps"
	"slices"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// NPC shops, as rAthena's shop NPCs (npc_buylist and npc_selllist), and
// the one shop type of review decision 30, which also covers itemshop,
// pointshop, cashshop, marketshop and barter shops:
//   - A shop NPC's click, or a script's player:open_shop, opens the shop
//     window: the items with their costs for the player, their stock,
//     and the inventory slots the shop buys when it buys from players.
//   - An item's cost mixes zeny, items (as a barter or an itemshop) and
//     currencies (as a cashshop or a pointshop). Without a cost it is
//     the item's price in zeny.
//   - A stock limits an item for every player, as a marketshop's or a
//     barter's. The shop's lock is held through a purchase, so two
//     players never buy the same stock, and the stock saves with the
//     purchase's commit.
//   - A purchase or a sale is checked whole, then done in one transfer:
//     zeny, items, currencies and the inventory change together, or
//     nothing changes.
//   - Prices follow pc_modifybuyvalue and pc_modifysellvalue: Discount
//     or Compulsion Discount lowers the zeny a player pays when the shop
//     allows it, and Overcharge raises what the player gets. Items and
//     currencies are never discounted, as discount_item_point_shop 0.

// openShop is the shop window a player has open, as npc_shopid.
type openShop struct {
	shop *shop.Shop
	// npc is the shop NPC, which the player must stay near. It is nil
	// for a shop that a script opened.
	npc *entity
}

// Skills that change shop prices.
const (
	skillDiscount   = 37  // MC_DISCOUNT
	skillOvercharge = 38  // MC_OVERCHARGE
	skillCompulsion = 224 // RG_COMPULSION
)

// Shop price limits, as battle_config's min_shop_buy and min_shop_sell.
const (
	minShopBuy  = 1
	minShopSell = 0
)

// maxShopLines is the most lines one purchase or sale may have.
const maxShopLines = 100

// buyPrice is pc_modifybuyvalue.
func buyPrice(skills map[int]int, value int) int {
	rate := 0
	if lv := skills[skillDiscount]; lv > 0 {
		rate = 5 + lv*2
		if lv == 10 {
			rate--
		}
	}
	if lv := skills[skillCompulsion]; lv > 0 && 5+lv*4 > rate {
		rate = 5 + lv*4
	}
	val := value
	if rate > 0 {
		val = int(float64(value) * float64(100-rate) / 100)
	}
	return max(val, minShopBuy)
}

// sellPrice is pc_modifysellvalue.
func sellPrice(skills map[int]int, value int) int {
	rate := 0
	if lv := skills[skillOvercharge]; lv > 0 {
		rate = 5 + lv*2
		if lv == 10 {
			rate--
		}
	}
	val := value
	if rate > 0 {
		val = int(float64(value) * float64(100+rate) / 100)
	}
	return max(val, minShopSell)
}

// shopPrice is the zeny the player pays for one of an item. A cost with
// no zeny stays at 0: min_shop_buy applies to zeny prices.
func shopPrice(p *Player, s *shop.Shop, it *shop.Item) int {
	if it.Zeny <= 0 {
		return max(it.Zeny, 0)
	}
	if s.Discount {
		return buyPrice(p.char.Skills, it.Zeny)
	}
	return it.Zeny
}

// shopLocation is the time zone of daily and weekly restocks.
func (z *Zone) shopLocation() *time.Location {
	if z.Mods != nil && z.Mods.Shops != nil && z.Mods.Shops.Location != nil {
		return z.Mods.Shops.Location
	}
	return time.Local
}

// canSell is pc_can_sell_item's item checks: no_sell on the item or a
// card in it, and a bound copy, as allow_bound_sell 0.
func (z *Zone) canSell(s *item.Slot) bool {
	return !z.Items.Restricted(s, item.NoSellFlag) && item.BoundAllows(s.Bound, item.PlaceSell, false)
}

// openShop shows a shop's window, as clif_npcbuysell with both lists.
func (z *Zone) openShop(p *Player, s *shop.Shop, npc *entity) {
	p.shop = &openShop{shop: s, npc: npc}
	var id uint32
	if npc != nil {
		id, _ = p.netIDs.get(npc)
	}
	s.Lock()
	list := s.Items(z.wallClock(), z.shopLocation())
	items := make([]*wathenav1.ShopItem, 0, len(list))
	for i := range list {
		it := &list[i]
		b := wathenav1.ShopItem_builder{
			ItemId: it.ID, Price: uint32(shopPrice(p, s, it)), Weight: uint32(z.Items.Get(it.ID).Weight), // #nosec G115 -- prices are at most MAX_ZENY, and weights small.
		}
		for _, c := range it.Items {
			b.CostItems = append(b.CostItems, wathenav1.ShopCostItem_builder{ItemId: c.ID, Amount: uint32(c.Amount)}.Build()) // #nosec G115 -- at most 30000.
		}
		for _, c := range it.Currencies {
			r := z.currencies().ByID(c.ID)
			if r == nil {
				continue
			}
			b.CostCurrencies = append(b.CostCurrencies, wathenav1.ShopCostCurrency_builder{Currency: r.Name, DisplayName: r.DisplayName,
				Amount: uint32(c.Amount)}.Build()) // #nosec G115 -- at most MAX_ZENY.
		}
		if it.Limited() {
			b.Stock = new(uint32(it.Stock)) // #nosec G115 -- at most mod.MaxStock.
		}
		items = append(items, b.Build())
	}
	s.Unlock()
	var sellable []*wathenav1.ShopSellable
	if s.BuysFromPlayers {
		for slot, st := range p.inventory.Slots {
			// pc_can_sell_item: equipped items are not sold.
			if st.ID == 0 || st.Amount <= 0 || st.Equip != 0 {
				continue
			}
			d := z.Items.Get(st.ID)
			if !z.canSell(&st) {
				continue
			}
			sellable = append(sellable, wathenav1.ShopSellable_builder{
				Slot: uint32(slot), Price: uint32(sellPrice(p.char.Skills, d.Sell)), // #nosec G115 -- below MaxInventory, and prices are at most MAX_ZENY.
			}.Build())
		}
	}
	p.pending = append(p.pending, wathenav1.Event_builder{ShopOpen: wathenav1.ShopOpen_builder{
		Npc: id, Items: items, Sellable: sellable, Name: s.DisplayName, BuysFromPlayers: s.BuysFromPlayers,
	}.Build()}.Build())
	z.sendCurrencies(p)
}

func shopResult(o wathenav1.ShopOutcome) *wathenav1.Event {
	return wathenav1.Event_builder{ShopResult: wathenav1.ShopResult_builder{Outcome: o}.Build()}.Build()
}

// shopFor returns the shop a command names: the one the player has open,
// with its NPC still near. It is nil otherwise.
func (z *Zone) shopFor(p *Player, npc uint32) *openShop {
	o := p.shop
	if o == nil {
		return nil
	}
	if o.npc != nil {
		id, ok := p.netIDs.get(o.npc)
		if !ok || id != npc || !z.entities.has(o.npc) || !near(p, o.npc) {
			return nil
		}
	} else if npc != 0 {
		return nil
	}
	return o
}

// closeShop forgets the player's shop window. With reply set, the player
// gets SHOP_OUTCOME_CLOSED.
func (z *Zone) closeShop(p *Player, reply bool) {
	if p.shop == nil {
		return
	}
	p.shop = nil
	if reply {
		p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_CLOSED))
	}
}

// buyLine is one line of a purchase.
type buyLine struct {
	id     uint32
	amount int
}

// costSlot is an inventory slot a cost takes items from.
type costSlot struct {
	slot   int
	serial item.Serial
	amount int
}

// costSlots picks the inventory slots that pay amount of an item, as
// npc_barter_purchase: unequipped copies, in slot order, leaving out
// refined copies and copies with cards, so a cost never takes a
// player's upgraded gear. taken holds what earlier lines took. It
// returns nil when the player has too few.
func costSlots(inv *item.Inventory, id uint32, amount int, taken map[int]int) []costSlot {
	var out []costSlot
	for i := range inv.Slots {
		if amount <= 0 {
			break
		}
		s := &inv.Slots[i]
		if s.ID != id || s.Equip != 0 || s.Refine > 0 || len(s.Cards) > 0 && hasCard(s.Cards) {
			continue
		}
		free := s.Amount - taken[i]
		if free <= 0 {
			continue
		}
		n := min(free, amount)
		out = append(out, costSlot{slot: i, serial: s.Serial, amount: n})
		taken[i] += n
		amount -= n
	}
	if amount > 0 {
		return nil
	}
	return out
}

// hasCard reports whether a slot list holds a card.
func hasCard(cards []uint32) bool {
	for _, c := range cards {
		if c != 0 {
			return true
		}
	}
	return false
}

// buy is npc_buylist, with the costs of npc_barter_purchase and
// npc_cashshop_buylist. The order of the checks follows them: an item
// the shop does not sell, the stock, a stack that would pass its limit,
// the zeny, the items, the currencies, the weight, then the free slots.
// The window closes after it.
func (z *Zone) buy(p *Player, npc uint32, lines []buyLine) {
	o := z.shopFor(p, npc)
	if o == nil {
		p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED))
		return
	}
	p.shop = nil
	fail := func(out wathenav1.ShopOutcome, id uint32) {
		p.pending = append(p.pending, wathenav1.Event_builder{ShopResult: wathenav1.ShopResult_builder{Outcome: out, ItemId: id}.Build()}.Build())
	}
	if len(lines) == 0 || len(lines) > maxShopLines {
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED, 0)
		return
	}
	s := o.shop
	s.Lock()
	defer s.Unlock()
	list := s.Items(z.wallClock(), z.shopLocation())
	var total, weight int64
	slots := 0
	idx := make([]int, len(lines))
	asked := map[uint32]int{}
	costItems := map[uint32]int{}
	costCurrencies := map[uint32]int64{}
	for i := range lines {
		l := &lines[i]
		idx[i] = s.Find(l.id)
		d, known := z.Items.Lookup(l.id)
		if idx[i] < 0 || !known || l.amount < 1 || l.amount > item.MaxAmount {
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED, l.id)
			return
		}
		it := &list[idx[i]]
		if !d.Stackable && l.amount > 1 {
			// A packet asking for more than one piece of equipment
			// buys one.
			l.amount = 1
		}
		asked[l.id] += l.amount
		if it.Limited() && asked[l.id] > it.Stock {
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_OUT_OF_STOCK, l.id)
			return
		}
		switch checkAddItem(p.inventory, d, l.amount) {
		case addNew:
			if d.Stackable && !d.UniqueID {
				slots++
			} else {
				slots += l.amount
			}
		case addOverAmount:
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_OVERWEIGHT, l.id)
			return
		}
		total += int64(shopPrice(p, s, it)) * int64(l.amount)
		weight += int64(d.Weight) * int64(l.amount)
		for _, c := range it.Items {
			costItems[c.ID] += c.Amount * l.amount
		}
		for _, c := range it.Currencies {
			costCurrencies[c.ID] += c.Amount * int64(l.amount)
		}
	}
	if total > p.record.Zeny {
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ZENY, 0)
		return
	}
	// The items a cost takes, in the order of the shop's items.
	taken := map[int]int{}
	var pay []costSlot
	for _, id := range sortedKeys(costItems) {
		got := costSlots(p.inventory, id, costItems[id], taken)
		if got == nil {
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ITEMS, id)
			return
		}
		pay = append(pay, got...)
		weight -= int64(z.Items.Get(id).Weight) * int64(costItems[id])
	}
	var wallets []*transfer.CurrencyWallet
	for _, id := range sortedKeys(costCurrencies) {
		r := z.currencies().ByID(id)
		if r == nil || z.balance(p, r) < costCurrencies[id] {
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_CURRENCY, 0)
			return
		}
		w, _, _ := z.currencyWallet(p, r)
		if w == nil {
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_CURRENCY, 0)
			return
		}
		wallets = append(wallets, w)
	}
	switch {
	case weight+int64(p.inventory.Weight) > int64(p.maxWeight):
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_OVERWEIGHT, 0)
		return
	case p.inventory.Blank() < slots && len(pay) == 0:
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED, 0)
		return
	}
	// Zeny, costs and items move in one transfer, which checks every
	// move first. The costs leave before the items come, so a stack a
	// cost empties frees its slot. Lines for the same item can pass one
	// by one and fail together, and then nothing changes.
	box := z.box(p)
	tx := transfer.New(transfer.ActionBuy)
	if total > 0 {
		tx.Pay(z.wallet(p), total, transfer.End(transfer.KindShop))
	}
	for _, c := range pay {
		tx.Destroy(box, c.slot, c.serial, c.amount, transfer.End(transfer.KindShop))
	}
	for _, w := range wallets {
		tx.Pay(w, costCurrencies[w.ID], transfer.End(transfer.KindShop))
	}
	var bought []modscript.SaleItem
	for _, l := range lines {
		d := z.Items.Get(l.id)
		step := l.amount
		if d.UniqueID || !d.Stackable {
			step = 1
		}
		for given := 0; given < l.amount; given += step {
			x := z.newExtra(d, false, rollSource{kind: sourceShop}, "")
			if len(x.Rolls) == 0 && len(d.Rolls) > 0 {
				x.Rolls = d.Defaults()
			}
			tx.Create(transfer.End(transfer.KindShop), item.Slot{ID: l.id, Extra: x}, step, box, item.Serial{})
		}
		bought = append(bought, modscript.SaleItem{Item: l.id, Amount: l.amount})
	}
	if err := z.applyWithCurrencies(p, tx, wallets, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_PAID); err != nil {
		if te, ok := err.(*transfer.Error); ok && te.Reason == transfer.ReasonOverweight {
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_OVERWEIGHT, 0)
			return
		}
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED, 0)
		return
	}
	for i, l := range lines {
		s.Take(idx[i], l.amount)
	}
	z.shopChanged(s)
	p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT))
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "shop_buy", sale: &modscript.SaleEvent{
		Player: int64(p.charID), Shop: s.Name, Items: bought, Zeny: total}})
}

// sortedKeys are a map's keys in order.
func sortedKeys[V any](m map[uint32]V) []uint32 { return slices.Sorted(maps.Keys(m)) }

// shopChanged puts a shop's stock in the next commit.
func (z *Zone) shopChanged(s *shop.Shop) {
	for _, o := range z.txShops {
		if o == s {
			return
		}
	}
	z.txShops = append(z.txShops, s)
}

// stockWrites are the stock rows of the shops the zone changed since the
// last commit.
func (z *Zone) stockWrites() []shop.StockRow {
	var out []shop.StockRow
	for _, s := range z.txShops {
		s.Lock()
		out = append(out, s.Dirty()...)
		s.Unlock()
	}
	z.txShops = nil
	return out
}

// sellLine is one line of a sale.
type sellLine struct {
	slot, amount int
}

// sell is npc_selllist. Every line is checked before anything changes.
// Zeny over MAX_ZENY is lost, as pc_getzeny. The window closes after it.
func (z *Zone) sell(p *Player, npc uint32, lines []sellLine) {
	o := z.shopFor(p, npc)
	fail := func() {
		p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED))
	}
	if o == nil {
		fail()
		return
	}
	p.shop = nil
	if len(lines) == 0 || len(lines) > maxShopLines || !o.shop.BuysFromPlayers {
		fail()
		return
	}
	var total int64
	seen := map[int]bool{}
	var sold []modscript.SaleItem
	for _, l := range lines {
		if l.slot < 0 || l.slot >= len(p.inventory.Slots) || seen[l.slot] || l.amount < 1 {
			fail()
			return
		}
		seen[l.slot] = true
		s := p.inventory.Slots[l.slot]
		if s.ID == 0 || s.Amount < l.amount || s.Equip != 0 {
			fail()
			return
		}
		d := z.Items.Get(s.ID)
		if !z.canSell(&s) {
			fail()
			return
		}
		total += int64(sellPrice(p.char.Skills, d.Sell)) * int64(l.amount)
		sold = append(sold, modscript.SaleItem{Item: s.ID, Amount: l.amount})
	}
	// The items and the zeny move in one transfer. Zeny over MAX_ZENY is
	// lost.
	box := z.box(p)
	tx := transfer.New(transfer.ActionSell)
	for _, l := range lines {
		tx.Destroy(box, l.slot, p.inventory.Slots[l.slot].Serial, l.amount, transfer.End(transfer.KindShop))
	}
	tx.Earn(transfer.End(transfer.KindShop), min(total, character.MaxZeny), z.wallet(p), true)
	if err := z.apply(p, tx, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_SOLD); err != nil {
		fail()
		return
	}
	p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_SOLD))
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "shop_sell", sale: &modscript.SaleEvent{
		Player: int64(p.charID), Shop: o.shop.Name, Items: sold, Zeny: min(total, character.MaxZeny)}})
}
