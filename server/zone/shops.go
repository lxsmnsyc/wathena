package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
)

// NPC shops, as rAthena's shop NPCs (npc_buylist and npc_selllist):
//   - A shop NPC's click, or a script's player:open_shop, opens the shop
//     window: the items with their prices for the player, and the
//     inventory slots the shop buys.
//   - A purchase or a sale is checked whole, then done in one step:
//     zeny and inventory change together, or nothing changes.
//   - Prices follow pc_modifybuyvalue and pc_modifysellvalue: Discount
//     or Compulsion Discount lowers what the player pays when the shop
//     allows it, and Overcharge raises what the player gets.

// Shop is a shop from mods' data, as rAthena's shop line.
type Shop struct {
	// Name is the shop's full record name, such as core:Tool Dealer#prt1.
	Name  string
	Items []ShopItem
	// Discount lets Discount and Compulsion Discount lower the prices,
	// as npc_shop_discount.
	Discount bool
}

// ShopItem is one item a shop sells, and its price in zeny.
type ShopItem struct {
	ID    uint32
	Price int
}

// openShop is the shop window a player has open, as npc_shopid.
type openShop struct {
	shop *Shop
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

// shopPrice is what the player pays for one of an item.
func shopPrice(p *Player, s *Shop, it ShopItem) int {
	if s.Discount {
		return buyPrice(p.char.Skills, it.Price)
	}
	return it.Price
}

// canSell is the part of pc_can_sell_item that applies: the item's trade
// restriction. Equipment, rentals and bound items do not exist yet.
func canSell(d *item.Def) bool { return !d.NoSell }

// openShop shows a shop's window, as clif_npcbuysell with both lists.
func (z *Zone) openShop(p *Player, s *Shop, npc *entity) {
	p.shop = &openShop{shop: s, npc: npc}
	var id uint32
	if npc != nil {
		id, _ = p.netIDs.get(npc)
	}
	items := make([]*wathenav1.ShopItem, 0, len(s.Items))
	for _, it := range s.Items {
		items = append(items, wathenav1.ShopItem_builder{
			ItemId: it.ID, Price: uint32(shopPrice(p, s, it)), Weight: uint32(z.Items.Get(it.ID).Weight), // #nosec G115 -- prices are at most MAX_ZENY, and weights small.
		}.Build())
	}
	var sellable []*wathenav1.ShopSellable
	for slot, st := range p.inventory.Slots {
		if st.ID == 0 || st.Amount <= 0 {
			continue
		}
		d := z.Items.Get(st.ID)
		if !canSell(d) {
			continue
		}
		sellable = append(sellable, wathenav1.ShopSellable_builder{
			Slot: uint32(slot), Price: uint32(sellPrice(p.char.Skills, d.Sell)), // #nosec G115 -- below MaxInventory, and prices are at most MAX_ZENY.
		}.Build())
	}
	p.pending = append(p.pending, wathenav1.Event_builder{ShopOpen: wathenav1.ShopOpen_builder{
		Npc: id, Items: items, Sellable: sellable,
	}.Build()}.Build())
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

// buy is npc_buylist. The order of the checks follows it: an item the
// shop does not sell, a stack that would pass its limit, the zeny, the
// weight, then the free slots. The window closes after it.
func (z *Zone) buy(p *Player, npc uint32, lines []buyLine) {
	o := z.shopFor(p, npc)
	if o == nil {
		p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED))
		return
	}
	p.shop = nil
	fail := func(out wathenav1.ShopOutcome) {
		p.pending = append(p.pending, shopResult(out))
	}
	if len(lines) == 0 || len(lines) > maxShopLines {
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED)
		return
	}
	var total, weight int64
	slots := 0
	for i := range lines {
		l := &lines[i]
		var price = -1
		for _, it := range o.shop.Items {
			if it.ID == l.id {
				price = shopPrice(p, o.shop, it)
				break
			}
		}
		d, known := z.Items.Lookup(l.id)
		if price < 0 || !known || l.amount < 1 || l.amount > item.MaxAmount {
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED)
			return
		}
		if !d.Stackable && l.amount > 1 {
			// A packet asking for more than one piece of equipment
			// buys one.
			l.amount = 1
		}
		switch checkAddItem(p.inventory, d, l.amount) {
		case addNew:
			if d.Stackable && !d.UniqueID {
				slots++
			} else {
				slots += l.amount
			}
		case addOverAmount:
			fail(wathenav1.ShopOutcome_SHOP_OUTCOME_OVERWEIGHT)
			return
		}
		total += int64(price) * int64(l.amount)
		weight += int64(d.Weight) * int64(l.amount)
	}
	switch {
	case total > p.record.Zeny:
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ZENY)
		return
	case weight+int64(p.inventory.Weight) > int64(p.maxWeight):
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_OVERWEIGHT)
		return
	case p.inventory.Blank() < slots:
		fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED)
		return
	}
	// The items go in a copy first. Lines for the same item can pass
	// one by one and fail together, and then nothing changes.
	trial := *p.inventory
	for _, l := range lines {
		d := z.Items.Get(l.id)
		step := l.amount
		if d.UniqueID || !d.Stackable {
			step = 1
		}
		for given := 0; given < l.amount; given += step {
			if _, r := trial.Add(d, step, p.maxWeight); r != item.AddSuccess {
				fail(wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED)
				return
			}
		}
	}
	z.setZeny(p, p.record.Zeny-total)
	for _, l := range lines {
		d := z.Items.Get(l.id)
		step := l.amount
		if d.UniqueID || !d.Stackable {
			step = 1
		}
		for given := 0; given < l.amount; given += step {
			z.addItem(p, l.id, step)
		}
	}
	p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT))
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
	if len(lines) == 0 || len(lines) > maxShopLines {
		fail()
		return
	}
	var total int64
	seen := map[int]bool{}
	for _, l := range lines {
		if l.slot < 0 || l.slot >= item.MaxInventory || seen[l.slot] || l.amount < 1 {
			fail()
			return
		}
		seen[l.slot] = true
		s := p.inventory.Slots[l.slot]
		if s.ID == 0 || s.Amount < l.amount {
			fail()
			return
		}
		d := z.Items.Get(s.ID)
		if !canSell(d) {
			fail()
			return
		}
		total += int64(sellPrice(p.char.Skills, d.Sell)) * int64(l.amount)
	}
	for _, l := range lines {
		z.deleteItem(p, l.slot, l.amount, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_SOLD)
	}
	total = min(total, character.MaxZeny)
	z.setZeny(p, min(p.record.Zeny+total, character.MaxZeny))
	p.pending = append(p.pending, shopResult(wathenav1.ShopOutcome_SHOP_OUTCOME_SOLD))
}
