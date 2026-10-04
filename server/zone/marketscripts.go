package zone

import (
	"fmt"
	"strings"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/shop"
)

// The script calls of review decision 30: currencies, mail, autotrade,
// the store search, buying stores and shop changes.

var _ modscript.MarketHost = scriptHost{}

// Currency finds a currency by a name a mod gave.
func (h scriptHost) Currency(name, modID string) (string, bool) {
	r := h.z.currencies().Resolve(name, modID)
	if r == nil {
		return "", false
	}
	return r.Mod, true
}

// CurrencyBalance is player:get_currency.
func (h scriptHost) CurrencyBalance(player int64, name, modID string) (int64, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	r, err := h.z.currencyRecord(name, modID)
	if err != nil {
		return 0, err
	}
	return h.z.balance(p, r), nil
}

// ChangeCurrency is player:give_currency and player:take_currency.
func (h scriptHost) ChangeCurrency(player int64, name, modID string, n int64) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	r, err := h.z.currencyRecord(name, modID)
	if err != nil {
		return false, err
	}
	return h.z.changeCurrency(p, r, n), nil
}

// SendMail is player:send_mail.
func (h scriptHost) SendMail(player int64, m modscript.ScriptMail) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if len(m.Items) > h.z.limits().MailItems {
		return fmt.Errorf("player:send_mail: a mail holds at most %d items, the mail_items limit", h.z.limits().MailItems)
	}
	for _, it := range m.Items {
		if _, ok := h.z.Items.Lookup(it.Item); !ok {
			return fmt.Errorf("player:send_mail: there is no item %d", it.Item)
		}
	}
	return h.z.sendScriptMail(p, m)
}

// OpenMail is player:open_mail.
func (h scriptHost) OpenMail(player int64) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	return h.z.openMail(p), nil
}

// Autotrade is player:autotrade.
func (h scriptHost) Autotrade(player int64) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	if !p.hasStore() {
		return false, nil
	}
	// The connection closes once the script's call returned.
	h.z.later(func() { h.z.startAutotrade(p) })
	return true, nil
}

// SearchStores is player:search_stores.
func (h scriptHost) SearchStores(player int64, uses int, open bool) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	return h.z.openSearch(p, uses, open), nil
}

// OpenBuyingStore is player:open_buying_store.
func (h scriptHost) OpenBuyingStore(player int64, slots int) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	return h.z.setupBuyingStore(p, slots), nil
}

// shopNamed finds a shop by a name a mod gave: a full name, or the mod's
// own record, then core's.
func (h scriptHost) shopNamed(name, modID string) *shop.Shop {
	if h.z.Mods == nil || h.z.Mods.Shops == nil {
		return nil
	}
	reg := h.z.Mods.Shops
	if strings.Contains(name, ":") {
		return reg.Get(name)
	}
	if s := reg.Get(modID + ":" + name); s != nil {
		return s
	}
	return reg.Get("core:" + name)
}

// ShopFind is world.get_shop.
func (h scriptHost) ShopFind(name, modID string) (string, bool) {
	s := h.shopNamed(name, modID)
	if s == nil {
		return "", false
	}
	return s.Name, true
}

// ShopAdd is shop:add_item.
func (h scriptHost) ShopAdd(name string, spec modscript.ShopItemSpec, modID string) (bool, error) {
	s := h.z.Mods.Shops.Get(name)
	if s == nil {
		return false, fmt.Errorf("shop:add_item: there is no shop %q", name)
	}
	d, ok := h.z.Items.Lookup(spec.Item)
	if !ok {
		return false, nil
	}
	it := shop.Item{ID: spec.Item, Zeny: int(spec.Zeny), Stock: spec.Stock, Initial: spec.Stock}
	if spec.Zeny < 0 {
		it.Zeny = d.Buy
	}
	for _, c := range spec.Items {
		if _, ok := h.z.Items.Lookup(uint32(c[0])); !ok { // #nosec G115 -- checked by the runtime.
			return false, nil
		}
		it.Items = append(it.Items, shop.CostItem{ID: uint32(c[0]), Amount: int(c[1])}) // #nosec G115 -- as above.
	}
	for _, cname := range sortedNames(spec.Currencies) {
		r := h.z.currencies().Resolve(cname, modID)
		if r == nil {
			return false, nil
		}
		it.Currencies = append(it.Currencies, shop.CostCurrency{ID: r.ID, Amount: spec.Currencies[cname]})
	}
	var err error
	if it.Restock, err = mod.ParseRestock(spec.Restock); err != nil {
		return false, fmt.Errorf("shop:add_item: %w", err)
	}
	s.Lock()
	s.Add(it, h.z.wallClock())
	s.Unlock()
	h.z.shopChanged(s)
	return true, nil
}

// ShopRemove is shop:remove_item.
func (h scriptHost) ShopRemove(name string, itemID uint32) (bool, error) {
	s := h.z.Mods.Shops.Get(name)
	if s == nil {
		return false, fmt.Errorf("shop:remove_item: there is no shop %q", name)
	}
	s.Lock()
	ok := s.Remove(itemID)
	s.Unlock()
	if ok {
		h.z.shopChanged(s)
	}
	return ok, nil
}

// ShopSetStock is shop:set_stock.
func (h scriptHost) ShopSetStock(name string, itemID uint32, n int) (bool, error) {
	s := h.z.Mods.Shops.Get(name)
	if s == nil {
		return false, fmt.Errorf("shop:set_stock: there is no shop %q", name)
	}
	s.Lock()
	ok := s.SetStock(itemID, n, h.z.wallClock())
	s.Unlock()
	if ok {
		h.z.shopChanged(s)
	}
	return ok, nil
}

func sortedNames(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
