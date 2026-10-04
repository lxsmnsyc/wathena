package modscript

import (
	"strings"
	"testing"
)

// fakeMarket is the fake host's state for the calls of review decision
// 30.
type fakeMarket struct {
	balances map[string]int64
	mails    []ScriptMail
	shops    map[string]ShopItemSpec
}

var markets = map[*fakeHost]*fakeMarket{}

func (h *fakeHost) market() *fakeMarket {
	m := markets[h]
	if m == nil {
		m = &fakeMarket{balances: map[string]int64{}, shops: map[string]ShopItemSpec{}}
		markets[h] = m
	}
	return m
}

func (h *fakeHost) fullCurrency(name, mod string) (string, string) {
	switch name {
	case "cash", "kafra":
		return name, "core"
	case "tokens", "example.a:tokens":
		return "example.a:tokens", "example.a"
	}
	return "", ""
}

func (h *fakeHost) Currency(name, mod string) (string, bool) {
	_, owner := h.fullCurrency(name, mod)
	return owner, owner != ""
}

func (h *fakeHost) CurrencyBalance(_ int64, name, mod string) (int64, error) {
	full, _ := h.fullCurrency(name, mod)
	return h.market().balances[full], nil
}

func (h *fakeHost) ChangeCurrency(_ int64, name, mod string, n int64) (bool, error) {
	full, _ := h.fullCurrency(name, mod)
	m := h.market()
	if m.balances[full]+n < 0 {
		return false, nil
	}
	m.balances[full] += n
	return true, nil
}

func (h *fakeHost) SendMail(_ int64, m ScriptMail) error {
	h.market().mails = append(h.market().mails, m)
	return nil
}

func (h *fakeHost) OpenMail(int64) (bool, error)                   { return true, nil }
func (h *fakeHost) Autotrade(int64) (bool, error)                  { return false, nil }
func (h *fakeHost) SearchStores(int64, int, bool) (bool, error)    { return true, nil }
func (h *fakeHost) OpenBuyingStore(int64, int) (bool, error)       { return true, nil }
func (h *fakeHost) ShopRemove(string, uint32) (bool, error)        { return true, nil }
func (h *fakeHost) ShopSetStock(string, uint32, int) (bool, error) { return true, nil }
func (h *fakeHost) ShopFind(name, mod string) (string, bool) {
	if name == "Desk" || name == "example.a:Desk" {
		return "example.a:Desk", true
	}
	return "", false
}

func (h *fakeHost) ShopAdd(shop string, it ShopItemSpec, _ string) (bool, error) {
	h.market().shops[shop] = it
	return true, nil
}

// TestCurrencyCalls checks the currency calls and their permission: a
// mod changes its own currency freely, and core's needs
// currencies.modify.
func TestCurrencyCalls(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    player:give_currency("tokens", 5)
    player:send_message("tokens " .. player:get_currency("tokens") .. " " .. tostring(player:take_currency("tokens", 6)))
    local ok, err = pcall(function() player:give_currency("cash", 1) end)
    player:send_message(tostring(ok) .. " " .. tostring(err))
  end)
end`))
	r.Fire("login", Player(1))
	got := strings.Join(h.messages, "|")
	if !strings.Contains(got, "tokens 5 false") || !strings.Contains(got, "needs the permission currencies.modify") {
		t.Fatalf("messages %q, logs %s", got, logs)
	}
	h2 := newHost()
	r2, logs2 := newRuntime(t, h2, modScripts("example.a", []string{"currencies.modify"}, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    player:give_currency("cash", 3)
    player:send_message("cash " .. player:get_currency("cash"))
  end)
end`))
	r2.Fire("login", Player(1))
	if got := strings.Join(h2.messages, "|"); got != "1:cash 3" {
		t.Fatalf("messages %q, logs %s", got, logs2)
	}
}

// TestSendMailAndShopCalls checks player:send_mail's spec and mail.send,
// and world.get_shop with shops.modify.
func TestSendMailAndShopCalls(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", []string{"mail.send", "shops.modify"}, nil, "scripts/main.lua", `
local events = require "wa.events"
local world = require "wa.world"
return function()
  events.on("login", function(player)
    player:send_mail({ to = "Bob", title = "Hi", body = "Text", zeny = 10, items = { { item = 501, amount = 2, bound = "account" } } })
    local ok, err = pcall(function() player:send_mail({ title = "" }) end)
    player:send_message(tostring(ok) .. " " .. tostring(err))
    local desk = world:get_shop("Desk")
    player:send_message(tostring(world.get_shop("None")) .. " " ..
      tostring(desk:add_item(501, { cost = { zeny = 5, currency = { tokens = 1 } }, stock = 3, restock = "daily" })))
  end)
end`))
	r.Fire("login", Player(1))
	m := h.market()
	if len(m.mails) != 1 || m.mails[0].To != "Bob" || m.mails[0].Sender == "" || m.mails[0].Items[0].Amount != 2 || m.mails[0].Zeny != 10 {
		t.Fatalf("mails %+v, logs %s", m.mails, logs)
	}
	got := strings.Join(h.messages, "|")
	if !strings.Contains(got, "false") || !strings.Contains(got, "title is 1 to 39 bytes") || !strings.Contains(got, "nil true") {
		t.Errorf("messages %q", got)
	}
	if it := m.shops["example.a:Desk"]; it.Zeny != 5 || it.Currencies["tokens"] != 1 || it.Stock != 3 || it.Restock != "daily" {
		t.Errorf("shop item %+v", it)
	}
	h2 := newHost()
	r2, _ := newRuntime(t, h2, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local world = require "wa.world"
return function()
  events.on("login", function(player)
    local ok, err = pcall(function() player:send_mail({ title = "Hi" }) end)
    player:send_message(tostring(err))
    ok, err = pcall(function() world.get_shop("Desk"):set_stock(501, 1) end)
    player:send_message(tostring(err))
  end)
end`))
	r2.Fire("login", Player(1))
	if got := strings.Join(h2.messages, "|"); !strings.Contains(got, "needs the permission mail.send") || !strings.Contains(got, "needs the permission shops.modify") {
		t.Errorf("without permissions: %q", got)
	}
}

// TestSaleEventsAndHooks checks the object of the vend event and hook,
// and that the hook refuses with a message.
func TestSaleEventsAndHooks(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", []string{"rules.override"}, nil, "scripts/main.lua", `
local events = require "wa.events"
local hooks = require "wa.hooks"
return function()
  hooks.on("vend", function(sale, next)
    if sale.zeny > 100 then
      return false, "too much"
    end
    return next(sale)
  end)
  events.on("vend", function(sale)
    sale.seller:send_message("sold " .. sale.items[1].amount .. " for " .. sale.items[1].price .. " to " .. sale.buyer.name)
  end)
  events.on("mail", function(mail)
    sale = nil
  end)
end`))
	ev := SaleEvent{Seller: 1, Buyer: 2, Items: []SaleItem{{Item: 501, Amount: 2, Price: 30}}, Zeny: 60}
	if ok, _ := r.RunSaleHook("vend", ev); !ok {
		t.Errorf("refused a small sale, logs %s", logs)
	}
	ev.Zeny = 200
	if ok, msg := r.RunSaleHook("vend", ev); ok || msg != "too much" {
		t.Errorf("hook: %v %q", ok, msg)
	}
	r.FireSale("vend", SaleEvent{Seller: 1, Buyer: 2, Items: []SaleItem{{Item: 501, Amount: 2, Price: 30}}, Zeny: 60})
	if got := strings.Join(h.messages, "|"); got != "1:sold 2 for 30 to Bob" {
		t.Errorf("messages %q, logs %s", got, logs)
	}
}
