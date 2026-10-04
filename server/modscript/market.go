package modscript

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/item"
)

// Mail, vending, buying stores, shops and currencies (proposal 0002,
// review decision 30):
//   - player:get_currency(name), player:give_currency(name, n) and
//     player:take_currency(name, n) read and change the currencies of
//     data/currencies.yml. A mod changes its own freely. Another mod's
//     currency, and core's, need currencies.modify.
//   - player:send_mail({ to, sender, title, body, zeny, items }) sends a
//     mail to a character, online or not, as rAthena's mail command, and
//     needs mail.send. player:open_mail() opens the mailbox.
//   - player:autotrade() keeps the player's shop open without its
//     connection. player:search_stores({ uses, effect }) opens the store
//     search, and player:open_buying_store({ slots }) the buying store
//     setup.
//   - world.get_shop(name) returns a shop's handle, whose add_item,
//     remove_item and set_stock need shops.modify.
//   - The events shop_buy, shop_sell, vend, buying_store_sale and mail
//     run after a sale or a mail, and the hooks vend, buying_store_sale
//     and mail may refuse one. The hooks need rules.override.

// SaleItem is one line of a sale or a mail: the item, the amount and,
// for vending and buying stores, the price of one.
type SaleItem struct {
	Item   uint32
	Amount int
	Price  int64
}

// SaleEvent is the object of the shop_buy, shop_sell, vend,
// buying_store_sale and mail events and hooks. Player, Seller and Buyer
// are character IDs on the context's map; From and To are the names of a
// mail's sender and recipient.
type SaleEvent struct {
	Player, Seller, Buyer int64
	Shop                  string
	From, To              string
	Items                 []SaleItem
	Zeny                  int64
}

// saleFields are the object's fields for an event or a hook.
func saleFields(name string, ev SaleEvent) []script.Field {
	items := make([]script.Value, 0, len(ev.Items))
	for _, it := range ev.Items {
		f := []script.Field{script.KV("item", script.Int(int64(it.Item))), script.KV("amount", script.Int(int64(it.Amount)))}
		if name == "vend" || name == "buying_store_sale" {
			f = append(f, script.KV("price", script.Int(it.Price)))
		}
		items = append(items, script.NewTable(f...))
	}
	out := []script.Field{script.KV("event", script.String(name)), script.KV("items", script.List(items...)),
		script.KV("zeny", script.Int(ev.Zeny))}
	switch name {
	case "shop_buy", "shop_sell":
		out = append(out, script.KV("player", script.Int(ev.Player)), script.KV("shop", script.String(ev.Shop)))
	case "vend", "buying_store_sale":
		out = append(out, script.KV("seller", script.Int(ev.Seller)), script.KV("buyer", script.Int(ev.Buyer)))
	case "mail":
		out = append(out, script.KV("from", script.String(ev.From)), script.KV("to", script.String(ev.To)))
	}
	return out
}

// FireSale runs the handlers of a sale or mail event.
func (r *Runtime) FireSale(name string, ev SaleEvent) {
	list := r.handlers[name]
	if len(list) == 0 {
		return
	}
	obj := script.NewTable(saleFields(name, ev)...)
	for _, h := range slices.Clone(list) {
		if h.removed || h.ctx.ended {
			continue
		}
		if err := r.run(h.ctx, r.fns.invoke.Value(), h.fn.Value(), script.String("sale"), obj); err != nil {
			r.failed(h.ctx, "event "+name, err)
		}
	}
}

// RunSaleHook runs the vend, buying_store_sale or mail hooks before a
// sale or a mail. It returns false and the hook's message when a hook
// refused it. Without hooks, or when the chain fails, it goes on.
func (r *Runtime) RunSaleHook(name string, ev SaleEvent) (bool, string) {
	vals, ok := r.runHooks(name, saleFields(name, ev))
	if !ok || len(vals) == 0 {
		return true, ""
	}
	allowed, isBool := vals[0].AsBool()
	if !isBool || allowed {
		return true, ""
	}
	msg := ""
	if len(vals) > 1 {
		msg, _ = vals[1].AsString()
	}
	if len(msg) > MaxMessage {
		msg = msg[:MaxMessage]
	}
	return false, msg
}

// MarketHost is what the calls of review decision 30 need from the zone.
type MarketHost interface {
	// Currency finds a currency by a name a mod gave: a full name, or
	// the mod's own record, then core's. owner is the mod that added it.
	Currency(name, mod string) (owner string, ok bool)
	// CurrencyBalance is a player's amount of a currency.
	CurrencyBalance(player int64, name, mod string) (int64, error)
	// ChangeCurrency gives (n above 0) or takes (below 0) a currency. It
	// returns false, and changes nothing, when the player has too little.
	ChangeCurrency(player int64, name, mod string, n int64) (bool, error)
	// SendMail sends a script's mail. Delivery to a character that is not
	// online may finish after the call.
	SendMail(player int64, m ScriptMail) error
	// OpenMail opens the mailbox. It returns false when the player is
	// busy, as mail_openmail.
	OpenMail(player int64) (bool, error)
	// Autotrade keeps the player's shop open without its connection. It
	// returns false when the player has no shop open.
	Autotrade(player int64) (bool, error)
	// SearchStores opens the store search window. It returns false when
	// it is open already.
	SearchStores(player int64, uses int, open bool) (bool, error)
	// OpenBuyingStore opens the buying store setup. It returns false when
	// the player cannot open one now, as buyingstore_setup.
	OpenBuyingStore(player int64, slots int) (bool, error)
	// ShopFind finds a shop of data/npcs.yml by a name a mod gave.
	ShopFind(name, mod string) (full string, ok bool)
	// ShopAdd adds or replaces an item of a shop, ShopRemove takes one
	// away, and ShopSetStock sets an item's stock, -1 for no limit.
	ShopAdd(shop string, it ShopItemSpec, mod string) (bool, error)
	ShopRemove(shop string, item uint32) (bool, error)
	ShopSetStock(shop string, item uint32, n int) (bool, error)
}

// ScriptMail is a mail of player:send_mail.
type ScriptMail struct {
	// To is the recipient's name, or ToID its character ID when above 0,
	// or neither for the player itself.
	To     string
	ToID   int64
	Sender string
	Title  string
	Body   string
	Zeny   int64
	Items  []ScriptMailItem
}

// ScriptMailItem is one attachment of a script's mail.
type ScriptMailItem struct {
	Item   uint32
	Amount int
	Roll   GiveRoll
	Bound  item.Bound
}

// ShopItemSpec is an item that shop:add_item adds.
type ShopItemSpec struct {
	Item uint32
	// Zeny is the zeny of one, or -1 for the item's buy price.
	Zeny       int64
	Items      [][2]int64
	Currencies map[string]int64
	// Stock is the item's stock, or -1 for no limit.
	Stock   int
	Restock string
}

func (r *Runtime) marketHost(what string) (MarketHost, error) {
	h, ok := r.host.(MarketHost)
	if !ok {
		return nil, errors.New(what + ": mail, shops and currencies do not exist here")
	}
	return h, nil
}

// currencyName reads a currency's name argument.
func currencyName(what string, v script.Value) (string, error) {
	s, ok := v.AsString()
	if !ok || s == "" || len(s) > 128 {
		return "", errors.New(what + ": want a currency's name of data/currencies.yml, such as cash")
	}
	return s, nil
}

// playerGetCurrency is player:get_currency(name).
func (r *Runtime) playerGetCurrency(c *script.Call) error {
	const what = "player:get_currency"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.marketHost(what)
	if err != nil {
		return err
	}
	name, err := currencyName(what, c.Arg(1))
	if err != nil {
		return err
	}
	n, err := h.CurrencyBalance(id, name, r.actor())
	if err != nil {
		return err
	}
	c.Return(script.Int(n))
	return nil
}

// playerChangeCurrency is player:give_currency(name, n) and
// player:take_currency(name, n): Arg(3) is true for take.
func (r *Runtime) playerChangeCurrency(c *script.Call) error {
	take := c.Arg(3).Truthy()
	what := "player:give_currency"
	if take {
		what = "player:take_currency"
	}
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.marketHost(what)
	if err != nil {
		return err
	}
	name, err := currencyName(what, c.Arg(1))
	if err != nil {
		return err
	}
	n, ok := c.Arg(2).AsInt()
	if !ok || n < 1 || n > 2_147_483_647 {
		return errors.New(what + ": the amount is 1 to 2147483647")
	}
	owner, found := h.Currency(name, r.actor())
	if !found {
		return fmt.Errorf("%s: there is no currency %q in data/currencies.yml", what, name)
	}
	if owner != r.actor() && (!r.opts.Testing || r.current != nil) {
		if err := r.need(what+" of another mod's currency", "currencies.modify"); err != nil {
			return err
		}
	}
	if take {
		n = -n
	}
	done, err := h.ChangeCurrency(id, name, r.actor(), n)
	if err != nil {
		return err
	}
	if take {
		c.Return(script.Bool(done))
	}
	return nil
}

// Mail limits of a script's mail.
const (
	// MaxMailItems is the engine's ceiling of mail_items.
	MaxMailItems = 20
	maxMailTitle = 39
	maxMailBody  = 499
	maxMailName  = 23
)

// playerSendMail is player:send_mail(spec). prelude passes the spec's
// fields: to, sender, title, body, zeny, then items as a list.
func (r *Runtime) playerSendMail(c *script.Call) error {
	const what = "player:send_mail"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "mail.send"); err != nil {
		return err
	}
	h, err := r.marketHost(what)
	if err != nil {
		return err
	}
	m := ScriptMail{}
	switch to := c.Arg(1); {
	case to.IsNil():
	default:
		if n, ok := to.AsInt(); ok {
			if n <= 0 {
				return errors.New(what + ": to is a character's name or a character ID above 0")
			}
			m.ToID = n
			break
		}
		s, ok := to.AsString()
		if !ok || s == "" || utf8.RuneCountInString(s) > maxMailName {
			return fmt.Errorf("%s: to is a character's name of 1 to %d characters, or a character ID", what, maxMailName)
		}
		m.To = s
	}
	m.Sender, _ = c.Arg(2).AsString()
	if m.Sender == "" {
		m.Sender = r.whisperName()
	}
	if utf8.RuneCountInString(m.Sender) > maxMailName {
		m.Sender = string([]rune(m.Sender)[:maxMailName])
	}
	m.Title, _ = c.Arg(3).AsString()
	if strings.TrimSpace(m.Title) == "" || len(m.Title) > maxMailTitle || !utf8.ValidString(m.Title) {
		return fmt.Errorf("%s: title is 1 to %d bytes", what, maxMailTitle)
	}
	m.Body, _ = c.Arg(4).AsString()
	if len(m.Body) > maxMailBody || !utf8.ValidString(m.Body) {
		return fmt.Errorf("%s: body is at most %d bytes", what, maxMailBody)
	}
	if !c.Arg(5).IsNil() {
		z, ok := c.Arg(5).AsInt()
		if !ok || z < 0 || z > 2_147_483_647 {
			return errors.New(what + ": zeny is 0 to 2147483647")
		}
		m.Zeny = z
	}
	if t, ok := c.Arg(6).AsTable(); ok {
		for i, f := range t.Fields {
			e, ok := f.Value.AsTable()
			if !ok {
				return fmt.Errorf("%s: items[%d] is { item, amount, roll, bound }", what, i+1)
			}
			var it ScriptMailItem
			it.Amount = 1
			var roll, bound script.Value
			for _, g := range e.Fields {
				k, _ := g.Key.AsString()
				switch k {
				case "item":
					n, ok := g.Value.AsInt()
					if !ok || n <= 0 || n > 1<<32-1 {
						return fmt.Errorf("%s: items[%d]: item is an item ID from items.id", what, i+1)
					}
					it.Item = uint32(n) // #nosec G115 -- checked above.
				case "amount":
					n, ok := g.Value.AsInt()
					if !ok || n < 1 || n > 30000 {
						return fmt.Errorf("%s: items[%d]: amount is 1 to 30000", what, i+1)
					}
					it.Amount = int(n)
				case "roll":
					roll = g.Value
				case "bound":
					bound = g.Value
				default:
					return fmt.Errorf("%s: items[%d]: %q is not a field; an item has item, amount, roll and bound", what, i+1, k)
				}
			}
			if it.Item == 0 {
				return fmt.Errorf("%s: items[%d] needs item", what, i+1)
			}
			if _, isTable := roll.AsTable(); isTable && (!r.opts.Testing || r.current != nil) {
				if err := r.need(what+" with a table of rolls", "items.modify_rolls"); err != nil {
					return err
				}
			}
			if it.Roll, err = ReadGiveRoll(what, roll); err != nil {
				return err
			}
			if _, it.Bound, err = giveOptions(what, script.Nil, bound); err != nil {
				return err
			}
			m.Items = append(m.Items, it)
		}
		if len(m.Items) > MaxMailItems {
			return fmt.Errorf("%s: a mail holds at most %d items", what, MaxMailItems)
		}
	} else if !c.Arg(6).IsNil() {
		return errors.New(what + ": items is a list of { item, amount, roll, bound }")
	}
	return h.SendMail(id, m)
}

// playerSimple runs player calls of one optional argument that return a
// boolean: open_mail, autotrade, search_stores and open_buying_store.
func (r *Runtime) playerMarketCall(c *script.Call) error {
	kind := c.String(1)
	what := "player:" + kind
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.marketHost(what)
	if err != nil {
		return err
	}
	var ok bool
	switch kind {
	case "open_mail":
		ok, err = h.OpenMail(id)
	case "autotrade":
		ok, err = h.Autotrade(id)
	case "search_stores":
		uses, _ := c.Arg(2).AsInt()
		if uses < 1 || uses > 1000 {
			return errors.New(what + ": uses is 1 to 1000")
		}
		effect, _ := c.Arg(3).AsString()
		if effect != "show" && effect != "open" {
			return errors.New(what + `: effect is "show" or "open"`)
		}
		ok, err = h.SearchStores(id, int(uses), effect == "open")
	case "open_buying_store":
		slots, _ := c.Arg(2).AsInt()
		if slots < 1 || slots > 5 {
			return errors.New(what + ": slots is 1 to 5, as MAX_BUYINGSTORE_SLOTS")
		}
		ok, err = h.OpenBuyingStore(id, int(slots))
	default:
		return fmt.Errorf("%s is not a call", what)
	}
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

// worldGetShop is world.get_shop(name). It returns the shop's full name,
// which prelude turns into a handle, or nil.
func (r *Runtime) worldGetShop(c *script.Call) error {
	const what = "world.get_shop"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.marketHost(what)
	if err != nil {
		return err
	}
	name, ok := c.Arg(0).AsString()
	if !ok || name == "" || len(name) > 128 {
		return errors.New(what + ": want a shop's name of data/npcs.yml")
	}
	if full, found := h.ShopFind(name, r.actor()); found {
		c.Return(script.String(full))
	}
	return nil
}

// shopCall is the methods of a shop handle: Arg(0) is the shop's full
// name, Arg(1) the method, then its arguments.
func (r *Runtime) shopCall(c *script.Call) error {
	shop := c.String(0)
	method := c.String(1)
	what := "shop:" + method
	if err := r.mutating(what); err != nil {
		return err
	}
	if !r.opts.Testing || r.current != nil {
		if err := r.need(what, "shops.modify"); err != nil {
			return err
		}
	}
	h, err := r.marketHost(what)
	if err != nil {
		return err
	}
	itemID, ok := c.Arg(2).AsInt()
	if !ok || itemID <= 0 || itemID > 1<<32-1 {
		return errors.New(what + ": want an item ID from items.id")
	}
	var done bool
	switch method {
	case "remove_item":
		done, err = h.ShopRemove(shop, uint32(itemID)) // #nosec G115 -- checked above.
	case "set_stock":
		n, ok := c.Arg(3).AsInt()
		if !ok || n < -1 || n > 2_147_483_647 {
			return errors.New(what + ": the stock is 0 to 2147483647, or -1 for no limit")
		}
		done, err = h.ShopSetStock(shop, uint32(itemID), int(n)) // #nosec G115 -- checked above.
	case "add_item":
		spec := ShopItemSpec{Item: uint32(itemID), Zeny: -1, Stock: -1} // #nosec G115 -- checked above.
		if err := readShopSpec(what, c.Arg(3), &spec); err != nil {
			return err
		}
		done, err = h.ShopAdd(shop, spec, r.actor())
	default:
		return fmt.Errorf("%s is not a method", what)
	}
	if err != nil {
		return err
	}
	c.Return(script.Bool(done))
	return nil
}

// readShopSpec reads the options of shop:add_item: { cost, stock,
// restock }, where cost is { zeny, items, currency } as in
// data/npcs.yml.
func readShopSpec(what string, v script.Value, spec *ShopItemSpec) error {
	if v.IsNil() {
		return nil
	}
	t, ok := v.AsTable()
	if !ok {
		return errors.New(what + ": the options are a table of cost, stock and restock")
	}
	for _, f := range t.Fields {
		k, _ := f.Key.AsString()
		switch k {
		case "cost":
			ct, ok := f.Value.AsTable()
			if !ok {
				return errors.New(what + ": cost is a table of zeny, items and currency")
			}
			spec.Zeny = 0
			for _, g := range ct.Fields {
				ck, _ := g.Key.AsString()
				switch ck {
				case "zeny":
					n, ok := g.Value.AsInt()
					if !ok || n < 0 || n > 2_147_483_647 {
						return errors.New(what + ": cost.zeny is 0 to 2147483647")
					}
					spec.Zeny = n
				case "items":
					lt, ok := g.Value.AsTable()
					if !ok {
						return errors.New(what + ": cost.items is a list of { item, amount }")
					}
					for _, e := range lt.Fields {
						et, ok := e.Value.AsTable()
						if !ok {
							return errors.New(what + ": cost.items is a list of { item, amount }")
						}
						var id, amount int64 = 0, 1
						for _, x := range et.Fields {
							xk, _ := x.Key.AsString()
							n, isInt := x.Value.AsInt()
							switch {
							case xk == "item" && isInt && n > 0 && n <= 1<<32-1:
								id = n
							case xk == "amount" && isInt && n >= 1 && n <= 30000:
								amount = n
							default:
								return errors.New(what + ": a cost item is { item = <items.id>, amount = 1 to 30000 }")
							}
						}
						if id == 0 {
							return errors.New(what + ": a cost item needs item")
						}
						spec.Items = append(spec.Items, [2]int64{id, amount})
					}
				case "currency":
					mt, ok := g.Value.AsTable()
					if !ok {
						return errors.New(what + ": cost.currency is a table of currency names and amounts, such as { cash = 100 }")
					}
					spec.Currencies = map[string]int64{}
					for _, x := range mt.Fields {
						name, _ := x.Key.AsString()
						n, isInt := x.Value.AsInt()
						if name == "" || !isInt || n < 1 || n > 2_147_483_647 {
							return errors.New(what + ": cost.currency maps currency names to amounts from 1 to 2147483647")
						}
						spec.Currencies[name] = n
					}
				default:
					return fmt.Errorf("%s: %q is not a field; a cost has zeny, items and currency", what, ck)
				}
			}
			if len(spec.Items)+len(spec.Currencies) > 5 {
				return errors.New(what + ": a cost takes at most 5 items and currencies together")
			}
		case "stock":
			n, ok := f.Value.AsInt()
			if !ok || n < 1 || n > 2_147_483_647 {
				return errors.New(what + ": stock is 1 to 2147483647")
			}
			spec.Stock = int(n)
		case "restock":
			s, ok := f.Value.AsString()
			if !ok {
				return errors.New(what + `: restock is "daily", "weekly", or a time such as "6h"`)
			}
			spec.Restock = s
		default:
			return fmt.Errorf("%s: %q is not an option; the options are cost, stock and restock", what, k)
		}
	}
	if spec.Restock != "" && spec.Stock < 0 {
		return errors.New(what + ": restock needs a stock")
	}
	return nil
}
