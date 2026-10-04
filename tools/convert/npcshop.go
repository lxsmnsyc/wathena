package convert

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Shops, currencies and mail of NPC scripts (proposal 0002, review
// decision 30).
//
// Shop lines become shops of data/npcs.yml:
//   - shop keeps the list form, {item, price}, with discount beside it.
//   - itemshop, pointshop, cashshop, marketshop and barter shops use the
//     table form. Players cannot sell to them, as in rAthena.
//   - An itemshop's price is an amount of its currency item, a
//     pointshop's an amount of the currency its variable becomes, and a
//     cashshop's an amount of cash. A marketshop has a zeny price and a
//     stock. A barter item costs zeny and items, with a stock.
//   - Parts the shop format cannot hold are left out, and the shop
//     converts partly: a barter item that is refined or takes refined
//     items, one that takes more than 5 items, an item a table shop sells
//     twice, and a marketshop item that starts with no stock.
//
// A pointshop's variable becomes a currency: #CASHPOINTS is cash,
// #KAFRAPOINTS is kafra, and any other variable is a new record of
// core's data/currencies.yml. Reads of these variables become
// player:get_currency, and changes give_currency and take_currency.
//
// The commands:
//   - mail becomes player:send_mail, with its item arrays read when the
//     script runs. Refine, bound, card and random option arrays do not
//     convert.
//   - openmail becomes player:open_mail, searchstores
//     player:search_stores, and buyingstore player:open_buying_store.
//   - npcshopadditem, npcshopdelitem and npcshopupdate become the methods
//     of world.get_shop's handle for a shop that converts. npcshopitem,
//     npcshopattach, sellitem and stopselling do not convert.

// shopsInput is shops.json.
type shopsInput struct {
	Barters []barterLine `json:"barters"`
}

// barterLine is one barter shop of shops.json.
type barterLine struct {
	Name   string  `json:"name"`
	File   string  `json:"file"`
	Map    string  `json:"map"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Dir    int     `json:"dir"`
	Sprite *string `json:"sprite"`
	Items  []struct {
		Index    int    `json:"index"`
		Item     string `json:"item"`
		ID       uint32 `json:"id"`
		Stock    int    `json:"stock"`
		Zeny     int64  `json:"zeny"`
		Refine   int    `json:"refine"`
		Required []struct {
			Item   string `json:"item"`
			Amount int    `json:"amount"`
			Refine int    `json:"refine"`
		} `json:"required"`
	} `json:"items"`
}

// readShops reads shops.json. A nil reader has no barters.
func readShops(r io.Reader) (*shopsInput, error) {
	out := &shopsInput{}
	if r == nil {
		return out, nil
	}
	if err := json.NewDecoder(r).Decode(out); err != nil {
		return nil, fmt.Errorf("shops.json: %w", err)
	}
	return out, nil
}

// shopRef is a shop that scripts name: the converted shop, and the
// items its line sells.
type shopRef struct {
	npc   *NPC
	line  *npcLine
	items map[uint32]bool
}

// currencyRecord is a currency the converter adds to core's
// data/currencies.yml for a pointshop's variable.
type currencyRecord struct {
	name, display, owner string
	id                   int
}

// maxCostLines is the most items and currencies one cost takes, as
// server/mod's MaxCostLines.
const maxCostLines = 5

// maxShopItems is the most items a shop sells, as server/mod's
// MaxShopItems.
const maxShopItems = 200

// prepareCurrencies maps #CASHPOINTS and #KAFRAPOINTS to core's
// currencies, and each pointshop's variable to a currency.
func (c *npcConverter) prepareCurrencies(lines []npcLine) {
	c.currencyVars = map[string]string{"#cashpoints": "cash", "#kafrapoints": "kafra"}
	c.currencyErr = map[string]string{}
	used := map[string]bool{"cash": true, "kafra": true}
	for i := range lines {
		l := &lines[i]
		if l.Kind != "pointshop" || l.Shop == nil || l.Shop.Currency == nil || l.Shop.Currency.Variable == "" {
			continue
		}
		v := l.Shop.Currency.Variable
		key := strings.ToLower(v)
		if _, ok := c.currencyVars[key]; ok {
			continue
		}
		if _, ok := c.currencyErr[key]; ok {
			continue
		}
		prefix, bare, str := varName(v)
		owner := ""
		switch {
		case str:
			c.currencyErr[key] = v + " is a string variable"
		case prefix == "#" || prefix == "##":
			owner = "account"
		case prefix == "":
			owner = "character"
		default:
			c.currencyErr[key] = v + " is a temporary variable, which a currency cannot be"
		}
		if owner == "" {
			continue
		}
		var b strings.Builder
		for _, r := range strings.ToLower(bare) {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
				b.WriteRune(r)
			}
		}
		name := b.String()
		if name == "" || name[0] >= '0' && name[0] <= '9' {
			name = "points_" + name
		}
		base := name
		for i := 2; used[name]; i++ {
			name = base + "_" + strconv.Itoa(i)
		}
		used[name] = true
		display := bare
		if len(display) > 24 {
			display = display[:24]
		}
		// Core's own records use IDs 1 and 2. Converted ones follow.
		c.newCurrencies = append(c.newCurrencies, currencyRecord{name: name, display: display, owner: owner, id: 3 + len(c.newCurrencies)})
		c.currencyVars[key] = name
	}
}

// currencyVar returns the currency of a variable, or false for a
// variable that is not one.
func (c *npcConverter) currencyVar(name string) (string, bool) {
	cur, ok := c.currencyVars[strings.ToLower(name)]
	return cur, ok
}

// currencyFile is core's data/currencies.yml with the converted
// currencies after core's own.
func (r *NPCResult) currencyFile(core string) string {
	if len(r.conv.newCurrencies) == 0 {
		return core
	}
	var b strings.Builder
	b.WriteString(core)
	b.WriteString("\n# Currencies converted from rAthena's pointshop variables by wactl convert npcs.\n")
	for _, cur := range r.conv.newCurrencies {
		fmt.Fprintf(&b, "- name: %s\n  id: %d\n  display_name: %s\n  owner: %s\n  max: 2147483647\n", cur.name, cur.id, yamlString(cur.display), cur.owner)
	}
	return b.String()
}

// shopCost is the cost of one item of a table shop, for data/npcs.yml,
// or "" for a shop that sells for zeny. why is set when the price does
// not convert.
func (c *npcConverter) shopCost(l *npcLine, price int) (cost, why string) {
	switch l.Kind {
	case "itemshop":
		if l.Shop.Currency == nil || l.Shop.Currency.Item == "" {
			return "", "the shop has no currency item"
		}
		if price <= 0 {
			return "{zeny: 0}", ""
		}
		return fmt.Sprintf("{items: [{item: %s, amount: %d}]}", yamlString(l.Shop.Currency.Item), min(price, 30000)), ""
	case "pointshop":
		if l.Shop.Currency == nil {
			return "", "the shop has no currency variable"
		}
		cur, ok := c.currencyVar(l.Shop.Currency.Variable)
		if !ok {
			if why := c.currencyErr[strings.ToLower(l.Shop.Currency.Variable)]; why != "" {
				return "", why
			}
			return "", "the shop's variable is not a currency"
		}
		if price <= 0 {
			return "{zeny: 0}", ""
		}
		return fmt.Sprintf("{currency: {%s: %d}}", cur, price), ""
	case "cashshop":
		if price <= 0 {
			return "{zeny: 0}", ""
		}
		return fmt.Sprintf("{currency: {cash: %d}}", price), ""
	}
	return "", ""
}

// convertShop makes a shop's record: placed when it has a map and a
// sprite, and opened only by scripts otherwise.
func (c *npcConverter) convertShop(n *NPC, l *npcLine) *NPC {
	n.Shop, n.Status = true, Full
	n.Features[l.Kind] = true
	if l.Shop == nil || len(l.Shop.Items) == 0 {
		n.Status = None
		n.Unsupported = append(n.Unsupported, l.Kind+": no items")
		return n
	}
	var items []string
	seen := map[string]bool{}
	for _, it := range l.Shop.Items {
		switch l.Kind {
		case "shop":
			items = append(items, fmt.Sprintf("{item: %s, price: %d}", yamlString(it.Item), it.Price))
			continue
		case "marketshop":
			if seen[it.Item] {
				continue
			}
			seen[it.Item] = true
			fields := fmt.Sprintf("{item: %s, price: %d", yamlString(it.Item), it.Price)
			switch {
			case it.Stock != nil && *it.Stock == 0:
				n.Unsupported = append(n.Unsupported, "marketshop: "+it.Item+" starts with no stock, and a stock in data/npcs.yml is 1 or more")
				continue
			case it.Stock != nil && *it.Stock > 0:
				fields += fmt.Sprintf(", stock: %d", *it.Stock)
			}
			items = append(items, fields+"}")
			continue
		}
		if seen[it.Item] {
			// rAthena lists the item twice and sells the first one.
			n.Unsupported = append(n.Unsupported, l.Kind+": "+it.Item+" is in the shop twice, and a shop table names each item once")
			continue
		}
		seen[it.Item] = true
		cost, why := c.shopCost(l, it.Price)
		if why != "" {
			n.Status = None
			n.Unsupported = append(n.Unsupported, l.Kind+": "+why)
			n.Features[l.Kind] = false
			return n
		}
		items = append(items, fmt.Sprintf("{item: %s, cost: %s}", yamlString(it.Item), cost))
	}
	if len(items) == 0 || len(items) > maxShopItems {
		n.Status = None
		n.Features[l.Kind] = false
		n.Unsupported = append(n.Unsupported, fmt.Sprintf("%s: %d items, and a shop has 1 to %d", l.Kind, len(items), maxShopItems))
		return n
	}
	if len(n.Unsupported) > 0 {
		n.Status = Partial
	}
	fields := []string{"name: " + yamlString(n.Name)}
	view, ok := c.sprite(l.Sprite)
	if l.Map != "" && ok && (l.State == "" || l.State == "enabled") {
		n.Placed = true
		fields = append(fields,
			"map: "+yamlString(l.Map), fmt.Sprintf("x: %d", l.X), fmt.Sprintf("y: %d", l.Y),
			"direction: "+direction(l.Dir), "sprite: "+yamlString(view),
			"display_name: "+yamlString(displayName(l.Display)))
	}
	if l.Kind == "shop" {
		fields = append(fields, "shop: ["+strings.Join(items, ", ")+"]")
		if !l.Shop.Discount {
			fields = append(fields, "discount: false")
		}
	} else {
		fields = append(fields, fmt.Sprintf("shop: {buys_from_players: false, discount: %t, items: [%s]}", l.Shop.Discount, strings.Join(items, ", ")))
	}
	n.record = "- {" + strings.Join(fields, ", ") + "}"
	return n
}

// convertBarter makes a barter shop's record. rAthena opens a barter with
// callshop and its name. It is placed when it has a map and a sprite.
func (c *npcConverter) convertBarter(bl *barterLine, name string) *NPC {
	n := &NPC{Name: name, Source: bl.Name, Kind: "barter", File: bl.File, Map: bl.Map, Shop: true, Status: Full,
		Features: map[string]bool{"barter": true}}
	var items []string
	seen := map[string]bool{}
	for _, it := range bl.Items {
		skip := func(format string, args ...any) {
			n.Unsupported = append(n.Unsupported, "barter: "+fmt.Sprintf(format, args...))
		}
		if it.Refine > 0 {
			skip("%s (index %d) is sold refined to +%d, and a shop item has no refine", it.Item, it.Index, it.Refine)
			continue
		}
		if len(it.Required) > maxCostLines {
			skip("%s (index %d) takes %d kinds of items, and a cost takes at most %d", it.Item, it.Index, len(it.Required), maxCostLines)
			continue
		}
		refined := ""
		for _, r := range it.Required {
			if r.Refine > 0 {
				refined = fmt.Sprintf("%s refined to +%d or more", r.Item, r.Refine)
				break
			}
		}
		if refined != "" {
			skip("%s (index %d) takes %s, and a cost cannot ask for a refine", it.Item, it.Index, refined)
			continue
		}
		if seen[it.Item] {
			skip("%s (index %d) is sold a second time, and a shop table names each item once", it.Item, it.Index)
			continue
		}
		seen[it.Item] = true
		var cost []string
		if it.Zeny > 0 || len(it.Required) == 0 {
			cost = append(cost, fmt.Sprintf("zeny: %d", it.Zeny))
		}
		if len(it.Required) > 0 {
			var req []string
			for _, r := range it.Required {
				req = append(req, fmt.Sprintf("{item: %s, amount: %d}", yamlString(r.Item), max(r.Amount, 1)))
			}
			cost = append(cost, "items: ["+strings.Join(req, ", ")+"]")
		}
		fields := fmt.Sprintf("{item: %s, cost: {%s}", yamlString(it.Item), strings.Join(cost, ", "))
		if it.Stock > 0 {
			fields += fmt.Sprintf(", stock: %d", it.Stock)
		}
		items = append(items, fields+"}")
	}
	if len(items) == 0 || len(items) > maxShopItems {
		n.Status = None
		n.Features["barter"] = false
		n.Unsupported = append(n.Unsupported, fmt.Sprintf("barter: %d items convert, and a shop has 1 to %d", len(items), maxShopItems))
		return n
	}
	if len(n.Unsupported) > 0 {
		n.Status = Partial
	}
	fields := []string{"name: " + yamlString(n.Name)}
	if bl.Map != "" && bl.Sprite != nil {
		if view, ok := c.sprite(*bl.Sprite); ok {
			n.Placed = true
			fields = append(fields,
				"map: "+yamlString(bl.Map), fmt.Sprintf("x: %d", bl.X), fmt.Sprintf("y: %d", bl.Y),
				"direction: "+direction(bl.Dir), "sprite: "+yamlString(view),
				"display_name: "+yamlString(displayName(bl.Name)))
		}
	}
	fields = append(fields, fmt.Sprintf("shop: {buys_from_players: false, discount: false, items: [%s]}", strings.Join(items, ", ")))
	n.record = "- {" + strings.Join(fields, ", ") + "}"
	return n
}

// isShopKind reports whether an NPC line is a shop.
func isShopKind(kind string) bool {
	switch kind {
	case "shop", "cashshop", "itemshop", "pointshop", "marketshop":
		return true
	}
	return false
}

// shopNamed finds the shop a command names, for callshop and the npcshop
// commands. The npcshop commands do not change barter shops in rAthena.
func (b *body) shopNamed(name string, e Expr) (*shopRef, error) {
	v, err := b.expr(e)
	if err != nil {
		return nil, err
	}
	if v.str == nil {
		return nil, unsupported(name, "a computed shop")
	}
	ref := b.n.c.shops[*v.str]
	if ref == nil {
		return nil, unsupported(name, "%s is not a shop", *v.str)
	}
	if ref.npc.Status == None {
		return nil, unsupported(name, "the shop %s does not convert", *v.str)
	}
	if name != "callshop" && ref.line.Kind == "barter" {
		return nil, unsupported(name, "%s is a barter shop, which rAthena's npcshop commands do not change", *v.str)
	}
	return ref, nil
}

// staticItem returns an item argument's ID when it is fixed.
func (b *body) staticItem(e Expr) (uint32, bool) {
	v, err := b.expr(e)
	if err != nil {
		return 0, false
	}
	if v.static != nil {
		return uint32(*v.static), true // #nosec G115 -- item IDs are 32 bits.
	}
	if v.str != nil {
		id, ok := b.n.c.itemIDs[strings.ToLower(*v.str)]
		return id, ok
	}
	return 0, false
}

// luaCost is a cost for shop:add_item, for a fixed price in a shop of a
// kind, or "" for the item's buy price.
func (b *body) luaCost(name string, ref *shopRef, price int64) (string, error) {
	l := ref.line
	switch l.Kind {
	case "shop", "marketshop":
		if price < 0 {
			return "", nil
		}
		return fmt.Sprintf("{ zeny = %d }", price), nil
	}
	if price < 0 {
		return "", unsupported(name, "a price of -1, which rAthena refuses outside shop and marketshop lines")
	}
	if price == 0 {
		return "{ zeny = 0 }", nil
	}
	switch l.Kind {
	case "itemshop":
		if l.Shop.Currency == nil || l.Shop.Currency.Item == "" {
			return "", unsupported(name, "the shop has no currency item")
		}
		id, ok := b.n.c.itemIDs[strings.ToLower(l.Shop.Currency.Item)]
		if !ok {
			return "", unsupported(name, "%s is not an item", l.Shop.Currency.Item)
		}
		return fmt.Sprintf("{ items = { { item = %s, amount = %d } } }", b.n.file.record("items.id", b.n.c.items[id]), min(price, 30000)), nil
	case "pointshop":
		cur, ok := b.n.c.currencyVar(l.Shop.Currency.Variable)
		if !ok {
			return "", unsupported(name, "the shop's variable is not a currency")
		}
		return fmt.Sprintf("{ currency = { %s = %d } }", cur, price), nil
	case "cashshop":
		return fmt.Sprintf("{ currency = { cash = %d } }", price), nil
	}
	return "", unsupported(name, "a %s", l.Kind)
}

// shopHandle is the Lua that finds a converted shop.
func (b *body) shopHandle(ref *shopRef) string {
	b.n.file.need("world")
	return fmt.Sprintf("world.get_shop(%s)", luaString(ref.npc.Name))
}

// shopCommand converts the shop, mail and store commands written as
// statements. ok is false for a name that is not one.
func (b *body) shopCommand(name string, args []Expr) (ok bool, err error) {
	switch name {
	case "openmail":
		if len(args) > 0 {
			return true, unsupported(name, "for another character")
		}
		b.line("player:open_mail()")
		return true, nil
	case "buyingstore":
		if len(args) != 1 {
			return true, unsupported(name, "arguments")
		}
		v, err := b.expr(args[0])
		if err != nil {
			return true, err
		}
		b.line("player:open_buying_store({ slots = %s })", v.asInt())
		return true, nil
	case "searchstores":
		return true, b.searchstores(args)
	case "mail":
		return true, b.mail(args)
	case "npcshopupdate":
		return true, b.npcshopupdate(args)
	case "npcshopadditem":
		return true, b.npcshopadditem(args)
	case "npcshopdelitem":
		if len(args) < 2 {
			return true, unsupported(name, "arguments")
		}
		ref, err := b.shopNamed(name, args[0])
		if err != nil {
			return true, err
		}
		var items []string
		for _, a := range args[1:] {
			it, err := b.item(a)
			if err != nil {
				return true, err
			}
			items = append(items, it)
		}
		h := b.shopHandle(ref)
		if len(items) == 1 {
			b.line("%s:remove_item(%s)", h, items[0])
			return true, nil
		}
		shop := b.hide("shop")
		b.line("%s = %s", shop, h)
		for _, it := range items {
			b.line("%s:remove_item(%s)", shop, it)
		}
		return true, nil
	}
	if why := shopUnsupported(name); why != "" {
		return true, unsupported(name, "%s", why)
	}
	return false, nil
}

// shopRead converts the shop commands that a script reads the result of.
// They return 1 or 0 in rAthena. ok is false for a name that is not one.
func (b *body) shopRead(name string) (v val, ok bool, err error) {
	switch name {
	case "npcshopupdate", "npcshopadditem", "npcshopdelitem", "openmail", "searchstores", "buyingstore", "mail":
		return val{}, true, unsupported(name, "its result, which converts only as a statement")
	}
	if why := shopUnsupported(name); why != "" {
		return val{}, true, unsupported(name, "%s", why)
	}
	return val{}, false, nil
}

// shopUnsupported is the reason a shop command does not convert, or ""
// for a name that is not one of them.
func shopUnsupported(name string) string {
	switch name {
	case "npcshopitem":
		return "it replaces every item of a shop, and the API cannot list a shop's items to remove the ones it does not name"
	case "npcshopattach":
		return "rAthena lets the script take over the shop's sales (OnBuyItem and OnSellItem), and the shop_buy and shop_sell events run after a sale and cannot replace it"
	case "sellitem", "stopselling":
		return "rAthena has no such command"
	}
	return ""
}

// searchstores converts searchstores <uses>,<effect>{,"<map>"}. The
// effect is SEARCHSTORE_EFFECT_NORMAL (0, show) or
// SEARCHSTORE_EFFECT_REMOTE (1, open). The search covers the player's
// map, rAthena's default "this"; another map does not convert.
func (b *body) searchstores(args []Expr) error {
	const name = "searchstores"
	if len(args) < 2 || len(args) > 3 {
		return unsupported(name, "arguments")
	}
	uses, err := b.expr(args[0])
	if err != nil {
		return err
	}
	effect, err := b.expr(args[1])
	if err != nil {
		return err
	}
	if effect.static == nil || *effect.static < 0 || *effect.static > 1 {
		return unsupported(name, "an effect other than SEARCHSTORE_EFFECT_NORMAL or SEARCHSTORE_EFFECT_REMOTE")
	}
	if len(args) == 3 {
		m, err := b.expr(args[2])
		if err != nil {
			return err
		}
		if m.str == nil || !strings.EqualFold(*m.str, "this") {
			return unsupported(name, "a map, as player:search_stores searches the player's own map")
		}
	}
	b.line("player:search_stores({ uses = %s, effect = %s })", uses.asInt(), luaString([]string{"show", "open"}[*effect.static]))
	return nil
}

// mail converts mail <char id>,"<sender>","<title>","<body>"{,<zeny>
// {,<item id array>,<amount array>}}. The item arrays are read when the
// script runs, from index 0, at most 5 items as MAIL_MAX_ITEM. The
// refine, bound, card and random option arrays after them do not
// convert.
func (b *body) mail(args []Expr) error {
	const name = "mail"
	switch {
	case len(args) < 4 || len(args) == 6:
		return unsupported(name, "arguments")
	case len(args) > 7:
		return unsupported(name, "refine, bound, card and random option arrays, which player:send_mail does not set")
	}
	vals := make([]val, 0, 5)
	for _, a := range args[:min(len(args), 5)] {
		v, err := b.expr(a)
		if err != nil {
			return err
		}
		vals = append(vals, v)
	}
	fields := []string{"to = " + vals[0].asInt(), "sender = " + vals[1].asText(), "title = " + vals[2].asText(), "body = " + vals[3].asText()}
	if len(vals) > 4 {
		fields = append(fields, "zeny = "+vals[4].asInt())
	}
	if len(args) == 7 {
		ids, err := b.arrayTable(name, args[5])
		if err != nil {
			return err
		}
		amounts, err := b.arrayTable(name, args[6])
		if err != nil {
			return err
		}
		fields = append(fields, fmt.Sprintf("items = R.mail_items(%s, %s)", ids, amounts))
	}
	b.line("player:send_mail({ %s })", strings.Join(fields, ", "))
	return nil
}

// arrayTable is the Lua table, with keys from 0, of a whole array that a
// command takes, such as mail's item IDs. A variable that is not an
// array is a table of one element.
func (b *body) arrayTable(name string, e Expr) (string, error) {
	target, ok := asVar(e)
	if !ok {
		return "", unsupported(name, "an array argument that is not a variable")
	}
	if target.Index != nil {
		if n, ok := static(target.Index); !ok || n != 0 {
			return "", unsupported(name, "an array from an index other than 0")
		}
	}
	p, _, _ := varName(target.Name)
	if p == ".@" {
		l := b.local(target.Name)
		if l.array {
			return l.lua, nil
		}
		return "{ [0] = " + l.lua + " }", nil
	}
	if _, ok := b.n.c.currencyVar(target.Name); ok {
		return "", unsupported(name, "%s, a currency, as an array", target.Name)
	}
	v, err := b.variable(target.Name)
	if err != nil {
		return "", err
	}
	if v.scope == "npc" {
		if v.array {
			return v.local, nil
		}
		return "{ [0] = " + v.local + " }", nil
	}
	b.n.file.need("variables")
	if v.array {
		return fmt.Sprintf("R.load(%s, %s)", v.local, v.owner()), nil
	}
	if v.scope == "server" {
		return "{ [0] = " + v.local + ":get() }", nil
	}
	return "{ [0] = " + v.local + ":get(player) }", nil
}

// npcshopupdate converts npcshopupdate "<shop>",<item>,<price>{,<stock>}:
//   - A price of 0 keeps the price. In a marketshop the stock changes,
//     to no limit when it is -1 or missing; in other shops nothing
//     changes.
//   - Another price replaces the item with shop:add_item, for an item the
//     shop's line sells. rAthena changes only items the shop has, and
//     add_item would add a new one.
func (b *body) npcshopupdate(args []Expr) error {
	const name = "npcshopupdate"
	if len(args) < 3 || len(args) > 4 {
		return unsupported(name, "arguments")
	}
	ref, err := b.shopNamed(name, args[0])
	if err != nil {
		return err
	}
	it, err := b.item(args[1])
	if err != nil {
		return err
	}
	price, ok := static(args[2])
	if !ok {
		return unsupported(name, "a computed price")
	}
	market := ref.line.Kind == "marketshop"
	stock := "-1"
	if len(args) == 4 {
		v, err := b.expr(args[3])
		if err != nil {
			return err
		}
		if v.static != nil && *v.static < -1 {
			return unsupported(name, "a stock below -1, which rAthena refuses")
		}
		stock = v.asInt()
	}
	h := b.shopHandle(ref)
	if price == 0 {
		if !market {
			b.line("-- npcshopupdate with a price of 0 changes nothing in a %s.", ref.line.Kind)
			return nil
		}
		b.line("%s:set_stock(%s, %s)", h, it, stock)
		return nil
	}
	id, ok := b.staticItem(args[1])
	if !ok || !ref.items[id] {
		return unsupported(name, "a new price for an item that the shop's line does not sell, which shop:add_item would add")
	}
	cost, err := b.luaCost(name, ref, price)
	if err != nil {
		return err
	}
	opts := ""
	if cost != "" {
		opts = ", { cost = " + cost + " }"
	}
	if !market {
		b.line("%s:add_item(%s%s)", h, it, opts)
		return nil
	}
	shop := b.hide("shop")
	b.line("%s = %s", shop, h)
	b.line("%s:add_item(%s%s)", shop, it, opts)
	b.line("%s:set_stock(%s, %s)", shop, it, stock)
	return nil
}

// npcshopadditem converts npcshopadditem "<shop>",<item>,<price>{,...},
// and the form with a stock after each price for a marketshop. rAthena
// adds a second entry for an item a shop other than a marketshop sells
// already, which a shop cannot hold, so such an item does not convert.
func (b *body) npcshopadditem(args []Expr) error {
	const name = "npcshopadditem"
	if len(args) < 3 {
		return unsupported(name, "arguments")
	}
	ref, err := b.shopNamed(name, args[0])
	if err != nil {
		return err
	}
	market := ref.line.Kind == "marketshop"
	step := 2
	if market {
		step = 3
	}
	if (len(args)-1)%step != 0 {
		return unsupported(name, "arguments")
	}
	var lines []string
	for i := 1; i < len(args); i += step {
		it, err := b.item(args[i])
		if err != nil {
			return err
		}
		if !market {
			id, ok := b.staticItem(args[i])
			if !ok {
				return unsupported(name, "a computed item, which may be in the shop already")
			}
			if ref.items[id] {
				return unsupported(name, "%s is in the shop already, and rAthena lists it twice", b.n.c.items[id])
			}
		}
		price, ok := static(args[i+1])
		if !ok {
			return unsupported(name, "a computed price")
		}
		cost, err := b.luaCost(name, ref, price)
		if err != nil {
			return err
		}
		opts := ""
		if cost != "" {
			opts = ", { cost = " + cost + " }"
		}
		lines = append(lines, fmt.Sprintf("add_item(%s%s)", it, opts))
		if market {
			v, err := b.expr(args[i+2])
			if err != nil {
				return err
			}
			if v.static != nil && *v.static < -1 {
				return unsupported(name, "a stock below -1, which rAthena refuses")
			}
			if v.static == nil || *v.static != -1 {
				lines = append(lines, fmt.Sprintf("set_stock(%s, %s)", it, v.asInt()))
			}
		}
	}
	h := b.shopHandle(ref)
	if len(lines) == 1 {
		b.line("%s:%s", h, lines[0])
		return nil
	}
	shop := b.hide("shop")
	b.line("%s = %s", shop, h)
	for _, l := range lines {
		b.line("%s:%s", shop, l)
	}
	return nil
}

// readCurrency reads a currency variable.
func (b *body) readCurrency(name, cur string, index Expr) (val, error) {
	if index != nil {
		if n, ok := static(index); !ok || n != 0 {
			return val{}, unsupported(strings.ToUpper(name), "read as an array")
		}
	}
	b.n.ok(strings.ToUpper(name))
	return val{code: fmt.Sprintf("player:get_currency(%s)", luaString(cur)), t: tInt}, nil
}

// assignCurrency converts a change to a currency variable:
//   - x += n, x -= n, x = x + n and x = x - n give or take n. A computed
//     n goes through R.add_currency, which gives or takes by its sign.
//   - Any other assignment sets the amount with R.set_currency, which
//     gives or takes the difference.
//
// rAthena lets the variable go below 0. take_currency takes nothing then
// and returns false.
func (b *body) assignCurrency(s *Assign, cur string) error {
	t := s.Target
	feature := strings.ToUpper(t.Name)
	if t.Index != nil {
		if n, ok := static(t.Index); !ok || n != 0 {
			return b.unsupported(unsupported(feature, "written as an array"))
		}
	}
	op, amount := s.Op, s.Value
	if op == "=" {
		if bin, ok := s.Value.(*Binary); ok && (bin.Op == "+" || bin.Op == "-") {
			if v, ok := asVar(bin.X); ok && strings.EqualFold(v.Name, t.Name) && v.Index == nil {
				op, amount = bin.Op+"=", bin.Y
			}
		}
	}
	if op == "+=" || op == "-=" {
		d, err := b.expr(amount)
		if err != nil {
			return b.unsupported(err)
		}
		switch {
		case d.static != nil:
			n := *d.static
			if op == "-=" {
				n = -n
			}
			switch {
			case n > 0:
				b.line("player:give_currency(%s, %d)", luaString(cur), n)
			case n < 0:
				b.line("player:take_currency(%s, %d)", luaString(cur), -n)
			default:
				b.line("-- %s changes by 0.", t.Name)
			}
		case op == "-=":
			b.line("R.add_currency(player, %s, -%s)", luaString(cur), d.asInt())
		default:
			b.line("R.add_currency(player, %s, %s)", luaString(cur), d.asInt())
		}
		b.n.ok(feature)
		return nil
	}
	value := Expr(s.Value)
	if op != "=" {
		value = &Binary{Op: strings.TrimSuffix(op, "="), X: t, Y: s.Value}
	}
	v, err := b.expr(value)
	if err != nil {
		return b.unsupported(err)
	}
	b.line("R.set_currency(player, %s, %s)", luaString(cur), v.asInt())
	b.n.ok(feature)
	return nil
}
