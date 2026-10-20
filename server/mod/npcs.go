package mod

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// NPCKind is the NPC placement overlay: data/npcs.yml. Core's NPCs are
// converted from rAthena's scripts. A name without a prefix is the mod's
// own NPC, so `name: Healer` in example.town is example.town:Healer.
var NPCKind = Kind{Name: "npcs", File: "data/npcs.yml", LocalNames: true}

// Directions are the facings an NPC can have, in the protocol's order:
// 0 is south, then clockwise.
var Directions = []string{"south", "southwest", "west", "northwest", "north", "northeast", "east", "southeast"}

// NPC is one placed NPC.
type NPC struct {
	// Name is the full record name, such as example.town:Healer#1.
	Name string
	// Mod added the NPC.
	Mod string
	// DisplayName is what players see: the display_name field, or the
	// name after the mod ID, before any "#".
	DisplayName string
	// Hidden is the part after "#" of the display_name field, or of the
	// record's name when the field has none, as rAthena's strnpcinfo(2)
	// reads nd->name. A converted NPC whose rAthena name had a "::"
	// unique name keeps its "#" part in display_name.
	Hidden string
	// Map is the map's asset name, such as map/prontera.
	Map  string
	X, Y int
	// Direction is 0 to 7, from Directions.
	Direction int
	// Sprite is the NPC sprite ID, as the protocol's Look.body.
	Sprite uint32
	// Art is the sprite's asset name when the sprite field names art,
	// such as mod/example.lantern/sprite/npc/Lamplighter. The server
	// then gives the sprite an ID of its own (decision 0033).
	Art string
	// Script is the NPC's entry script, relative to the folder of
	// ScriptMod.
	Script    string
	ScriptMod string
	// Shop is what the NPC sells, or nil. A record with a shop and no
	// map is not placed: scripts open it with player:open_shop.
	Shop *Shop
	// Placed is set for an NPC on a map, or at places.
	Placed bool
	// Place is set for an NPC that stands at every place of a scope with
	// a tag, in place of a map and a cell (review decision 42). Map, X
	// and Y are then unset.
	Place *NPCPlace
	// Touch is the NPC's touch area, or nil (review decision 55).
	Touch *NPCTouch
	// State is how the NPC starts: "", disabled, hidden or cloaked, as
	// rAthena's script(DISABLED), (HIDDEN) and (CLOAKED) lines (review
	// decision 58).
	State string
	// NoSprite is set for sprite: none, rAthena's view -1: no view and no
	// click, events and touch only.
	NoSprite bool
	// Floating is set for a script record without a map or a place. It
	// runs only through events, in the start map's zone (review decision
	// 58).
	Floating bool
}

// NPCStates are the values of an NPC record's state field.
var NPCStates = []string{"disabled", "hidden", "cloaked"}

// NPCTouch is the touch field: the cells within XSpan and YSpan of the
// NPC's cell, as rAthena's OnTouch area. A player who steps into it
// fires the NPC's touch event, and untouch when it leaves.
type NPCTouch struct {
	XSpan, YSpan int
}

// MaxTouchSpan is the widest span of a touch area. rAthena's NPCs reach
// at most 62 cells.
const MaxTouchSpan = 100

// NPCPlace is where an NPC record with place stands: every place of the
// scope with the tag, such as {scope: town:any, tag: blacksmith}.
type NPCPlace struct {
	Scope places.Scope
	Tag   string
}

// Shop is a shop of an NPC record (proposal 0002, review decision 30).
// The field shop is a list of items, or a table with items,
// buys_from_players and discount. The list form takes discount beside
// it, as before.
type Shop struct {
	Items []ShopItem
	// Discount lets Discount and Compulsion Discount lower the zeny part
	// of the prices. It is true unless the record sets discount: false.
	Discount bool
	// BuysFromPlayers gives the shop a sell tab, as rAthena's shop line.
	// It is true unless the record sets buys_from_players: false.
	BuysFromPlayers bool
}

// ShopItem is one item of a shop.
type ShopItem struct {
	Item string
	// Price is the zeny of an item without a cost: a number, or -1 for
	// the item's buy price.
	Price int
	// Cost is the item's cost, when the record gives one. It replaces
	// Price.
	Cost *ShopCost
	// Stock is how many the shop has for every player, or 0 for no
	// limit. Restock is when the stock fills again: "", "daily",
	// "weekly", or a time such as "6h" (RestockEvery).
	Stock   int
	Restock string
}

// ShopCost mixes zeny, items and currencies.
type ShopCost struct {
	Zeny  int
	Items []ShopCostItem
	// Currencies are amounts by currency name: a plain name is the
	// shop's mod's record, then core's, and mod:name another mod's.
	Currencies map[string]int
}

// ShopCostItem is an item a cost takes, for one bought.
type ShopCostItem struct {
	Item   string
	Amount int
}

// npcFields are the fields of an NPC record.
var npcFields = []string{"map", "x", "y", "place", "direction", "sprite", "script", "display_name", "shop", "discount", "touch", "state"}

// shopFields are the fields of a shop table.
var shopFields = []string{"items", "buys_from_players", "discount"}

// MaxShopItems is the most items one shop may sell.
const MaxShopItems = 200

// NPCs merges the mods' NPC placements. sprite turns a sprite name, such
// as 4_F_KAFRA1, into its ID.
func (s *Set) NPCs(sprite func(name string) (uint32, bool)) ([]NPC, *Overlay, error) {
	o, err := s.Overlay(NPCKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var out []NPC
	var errs []error
	for _, r := range o.Records {
		n, err := s.npc(r, sprite)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: npcs %s: %w", r.Where, r.Name, err))
			continue
		}
		out = append(out, n)
	}
	return out, o, errors.Join(errs...)
}

func (s *Set) npc(r *Record, sprite func(string) (uint32, bool)) (NPC, error) {
	n := NPC{Name: r.Name, Mod: r.Mod}
	var errs []string
	for k := range r.Fields {
		if !slices.Contains(npcFields, k) {
			errs = append(errs, fmt.Sprintf("%q is not a field; NPCs have %s", k, strings.Join(npcFields, ", ")))
		}
	}
	str := func(k string) (string, bool) {
		v, ok := r.Fields[k].(string)
		return v, ok
	}
	num := func(k string) (int, bool) {
		v, ok := r.Fields[k].(int)
		return v, ok
	}
	if shop, ok := r.Fields["shop"]; ok {
		sh, err := readShop(shop)
		if err != nil {
			errs = append(errs, err.Error())
		}
		n.Shop = sh
	}
	if v, ok := r.Fields["discount"]; ok {
		b, isBool := v.(bool)
		_, table := r.Fields["shop"].(map[string]any)
		switch {
		case !isBool || n.Shop == nil:
			errs = append(errs, "discount is true or false, on a record with a shop")
		case table:
			errs = append(errs, "discount goes in the shop table, beside items")
		default:
			n.Shop.Discount = b
		}
	}
	_, hasMap := r.Fields["map"]
	_, hasPlace := r.Fields["place"]
	if !hasMap && !hasPlace && n.Shop != nil {
		// A shop that scripts open. It has no place.
		for _, k := range []string{"x", "y", "direction", "sprite", "script", "display_name", "touch"} {
			if _, ok := r.Fields[k]; ok {
				errs = append(errs, fmt.Sprintf("%s needs a map: a shop without a map is only opened by scripts", k))
			}
		}
		if len(errs) > 0 {
			return NPC{}, errors.New(strings.Join(errs, "; "))
		}
		return n, nil
	}
	n.Placed = true
	_, hasScript := r.Fields["script"]
	if !hasMap && !hasPlace && hasScript {
		// An NPC without a map: it runs only through events.
		n.Floating = true
		for _, k := range []string{"x", "y", "direction", "sprite", "touch", "state"} {
			if _, ok := r.Fields[k]; ok {
				errs = append(errs, fmt.Sprintf("%s needs a map: an NPC without a map has no place, and runs only through events", k))
			}
		}
	} else if hasPlace {
		for _, k := range []string{"map", "x", "y"} {
			if _, ok := r.Fields[k]; ok {
				errs = append(errs, fmt.Sprintf("%s does not go with place: the NPC stands at each place", k))
			}
		}
		p, err := readPlace(r.Fields["place"])
		if err != nil {
			errs = append(errs, err.Error())
		}
		n.Place = p
	} else {
		if m, ok := str("map"); ok && m != "" && !strings.Contains(m, "/") {
			n.Map = "map/" + m
		} else if ok && strings.Contains(m, "/") {
			n.Map = m
		} else {
			errs = append(errs, "map is required, such as prontera, or place, such as {scope: town:any, tag: blacksmith}")
		}
		var okX, okY bool
		n.X, okX = num("x")
		n.Y, okY = num("y")
		if !okX || !okY || n.X < 0 || n.Y < 0 || n.X > 1023 || n.Y > 1023 {
			errs = append(errs, "x and y are required: cells from 0 to 1023")
		}
	}
	if v, ok := r.Fields["direction"]; ok {
		d, isText := v.(string)
		n.Direction = slices.Index(Directions, d)
		if !isText || n.Direction < 0 {
			errs = append(errs, fmt.Sprintf("direction is one of %s", strings.Join(Directions, ", ")))
			n.Direction = 0
		}
	}
	switch v := r.Fields["sprite"].(type) {
	case nil:
		if !n.Floating {
			errs = append(errs, "sprite is required: an NPC sprite name, such as 4_F_KAFRA1, or none")
		}
	case string:
		if v == "none" {
			// rAthena's view -1: no view and no click.
			n.NoSprite = true
			break
		}
		if IsArtName(v) {
			if err := CheckArtName(v, "sprite"); err != nil {
				errs = append(errs, fmt.Sprintf("sprite: %v", err))
			}
			n.Art = v
			break
		}
		id, ok := sprite(v)
		if !ok {
			if num, err := strconv.ParseUint(v, 10, 32); err == nil {
				id, ok = uint32(num), true
			}
		}
		if !ok {
			errs = append(errs, fmt.Sprintf("sprite %q is not an NPC sprite name, such as 4_F_KAFRA1", v))
		}
		n.Sprite = id
	case int:
		if v <= 0 || v > 1<<31 {
			errs = append(errs, "sprite is an NPC sprite name or a positive number")
		}
		n.Sprite = uint32(v) // #nosec G115 -- checked above.
	default:
		errs = append(errs, "sprite is an NPC sprite name, such as 4_F_KAFRA1, or none")
	}
	if v, ok := r.Fields["state"]; ok {
		st, isText := v.(string)
		if !isText || !slices.Contains(NPCStates, st) {
			errs = append(errs, "state is disabled, hidden or cloaked: how the NPC starts, until a script shows it")
		}
		n.State = st
	}
	if d, ok := r.Fields["display_name"]; ok {
		name, isText := d.(string)
		// An empty name shows no name, as rAthena's names that start
		// with #.
		if !isText || name != "" && validLocalName(name) != nil {
			errs = append(errs, "display_name is 0 to 64 characters")
		}
		n.DisplayName = name
		_, n.Hidden, _ = strings.Cut(Local(r.Name), "#")
		if before, after, ok := strings.Cut(name, "#"); ok {
			n.DisplayName, n.Hidden = before, after
		}
	} else {
		n.DisplayName, n.Hidden, _ = strings.Cut(Local(r.Name), "#")
	}
	if v, ok := r.Fields["script"]; ok {
		p, isText := v.(string)
		if err := checkScriptPath(p); !isText || err != nil {
			errs = append(errs, fmt.Sprintf("script: %v", err))
		} else {
			n.Script, n.ScriptMod = p, r.SetBy["script"]
			owner := s.Mod(n.ScriptMod)
			if _, err := owner.readFile(p); errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Sprintf("script: %s has no %s", n.ScriptMod, p))
			}
		}
	}
	if v, ok := r.Fields["touch"]; ok {
		t, err := readTouch(v)
		switch {
		case err != nil:
			errs = append(errs, err.Error())
		case n.Script == "":
			errs = append(errs, "touch needs a script, whose touch and untouch handlers run")
		}
		n.Touch = t
	}
	if len(errs) > 0 {
		return NPC{}, errors.New(strings.Join(errs, "; "))
	}
	return n, nil
}

// readTouch reads a touch field: {x_span, y_span}.
func readTouch(v any) (*NPCTouch, error) {
	m, ok := v.(map[string]any)
	x, okX := m["x_span"].(int)
	y, okY := m["y_span"].(int)
	if !ok || len(m) != 2 || !okX || !okY || x < 0 || y < 0 || x > MaxTouchSpan || y > MaxTouchSpan {
		return nil, fmt.Errorf("touch is {x_span, y_span}, each 0 to %d cells, such as {x_span: 1, y_span: 1}", MaxTouchSpan)
	}
	return &NPCTouch{XSpan: x, YSpan: y}, nil
}

// readPlace reads a place field: {scope, tag}.
func readPlace(v any) (*NPCPlace, error) {
	m, ok := v.(map[string]any)
	scope, okScope := m["scope"].(string)
	tag, okTag := m["tag"].(string)
	if !ok || len(m) != 2 || !okScope || !okTag {
		return nil, errors.New("place is {scope, tag}, such as {scope: town:any, tag: blacksmith}")
	}
	sc, err := places.ParseScope(scope)
	if err != nil {
		return nil, fmt.Errorf("place: %w", err)
	}
	if !gen.ValidTag(tag) {
		return nil, fmt.Errorf("place: the tag %q is not 1 to 64 lowercase letters, digits and the signs : . - _", tag)
	}
	return &NPCPlace{Scope: sc, Tag: tag}, nil
}

// readShop reads a shop field: a list of items, or a table with items,
// buys_from_players and discount.
func readShop(v any) (*Shop, error) {
	sh := &Shop{Discount: true, BuysFromPlayers: true}
	list, ok := v.([]any)
	m, isTable := v.(map[string]any)
	if isTable {
		for k := range m {
			if !slices.Contains(shopFields, k) {
				return sh, fmt.Errorf("shop: %q is not a field; a shop has %s", k, strings.Join(shopFields, ", "))
			}
		}
		for _, k := range []string{"buys_from_players", "discount"} {
			if b, set := m[k]; set {
				val, isBool := b.(bool)
				if !isBool {
					return sh, fmt.Errorf("shop: %s is true or false", k)
				}
				if k == "discount" {
					sh.Discount = val
				} else {
					sh.BuysFromPlayers = val
				}
			}
		}
		list, ok = m["items"].([]any)
	}
	if !ok || len(list) == 0 || len(list) > MaxShopItems {
		return sh, fmt.Errorf("shop is a list of 1 to %d items, each {item: Red_Potion, price: 50}, or a table with items", MaxShopItems)
	}
	sh.Items = make([]ShopItem, 0, len(list))
	seen := map[string]bool{}
	for i, e := range list {
		im, ok := e.(map[string]any)
		if !ok {
			return sh, fmt.Errorf("shop[%d] is {item: <name>, price: <zeny>}", i)
		}
		name, ok := im["item"].(string)
		if !ok || name == "" {
			return sh, fmt.Errorf("shop[%d]: item is an item's name, such as Red_Potion", i)
		}
		if seen[name] && isTable {
			// A list may repeat an item, as rAthena's shop lines do; a
			// purchase finds the first. A table names each item once.
			return sh, fmt.Errorf("shop[%d]: %s is in the shop twice", i, name)
		}
		seen[name] = true
		it := ShopItem{Item: name, Price: -1}
		for _, k := range slices.Sorted(maps.Keys(im)) {
			val := im[k]
			switch k {
			case "item":
			case "price":
				p, ok := val.(int)
				if !ok || p < -1 || p > 2_147_483_647 {
					return sh, fmt.Errorf("shop[%d]: price is 0 to 2147483647 zeny, or -1 for the item's buy price", i)
				}
				it.Price = p
			case "cost":
				c, err := readCost(val)
				if err != nil {
					return sh, fmt.Errorf("shop[%d]: cost: %w", i, err)
				}
				it.Cost = c
			case "stock":
				n, ok := val.(int)
				if !ok || n < 1 || n > MaxStock {
					return sh, fmt.Errorf("shop[%d]: stock is 1 to %d", i, MaxStock)
				}
				it.Stock = n
			case "restock":
				r, ok := val.(string)
				if _, err := ParseRestock(r); !ok || err != nil {
					return sh, fmt.Errorf("shop[%d]: restock is daily, weekly, or a time such as 6h or 30m, at least 1m", i)
				}
				it.Restock = r
			default:
				return sh, fmt.Errorf("shop[%d]: %q is not a field; shop items have item, price, cost, stock and restock", i, k)
			}
		}
		switch {
		case it.Cost != nil && it.Price != -1:
			return sh, fmt.Errorf("shop[%d]: price and cost do not go together; put zeny in the cost", i)
		case it.Restock != "" && it.Stock == 0:
			return sh, fmt.Errorf("shop[%d]: restock needs a stock", i)
		}
		sh.Items = append(sh.Items, it)
	}
	return sh, nil
}

// MaxStock is the most stock a shop item has, as rAthena's market and
// barter stock.
const MaxStock = 2_147_483_647

// MaxCostLines is the most items and currencies one cost takes, as the
// five required items of a barter (MAX_BARTER_REQUIREMENTS).
const MaxCostLines = 5

// readCost reads a cost: zeny, items and currency, each optional.
func readCost(v any) (*ShopCost, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, errors.New("a cost is a table of zeny, items and currency")
	}
	c := &ShopCost{}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		val := m[k]
		switch k {
		case "zeny":
			n, ok := val.(int)
			if !ok || n < 0 || n > 2_147_483_647 {
				return nil, errors.New("zeny is 0 to 2147483647")
			}
			c.Zeny = n
		case "items":
			list, ok := val.([]any)
			if !ok || len(list) == 0 || len(list) > MaxCostLines {
				return nil, fmt.Errorf("items is a list of 1 to %d {item, amount}", MaxCostLines)
			}
			for j, e := range list {
				im, ok := e.(map[string]any)
				name, isText := im["item"].(string)
				if !ok || !isText || name == "" {
					return nil, fmt.Errorf("items[%d] is {item: <name>, amount: <n>}", j)
				}
				amount := 1
				for ik, iv := range im {
					switch ik {
					case "item":
					case "amount":
						n, ok := iv.(int)
						if !ok || n < 1 || n > 30000 {
							return nil, fmt.Errorf("items[%d]: amount is 1 to 30000", j)
						}
						amount = n
					default:
						return nil, fmt.Errorf("items[%d]: %q is not a field; cost items have item and amount", j, ik)
					}
				}
				c.Items = append(c.Items, ShopCostItem{Item: name, Amount: amount})
			}
		case "currency":
			cm, ok := val.(map[string]any)
			if !ok || len(cm) == 0 || len(cm) > MaxCostLines {
				return nil, fmt.Errorf("currency is a table of 1 to %d currency names and amounts, such as {cash: 100}", MaxCostLines)
			}
			c.Currencies = map[string]int{}
			for name, cv := range cm {
				n, ok := cv.(int)
				if !ok || n < 1 || n > 2_147_483_647 {
					return nil, fmt.Errorf("currency %s: the amount is 1 to 2147483647", name)
				}
				c.Currencies[name] = n
			}
		default:
			return nil, fmt.Errorf("%q is not a field; a cost has zeny, items and currency", k)
		}
	}
	if len(c.Items)+len(c.Currencies) > MaxCostLines {
		return nil, fmt.Errorf("a cost takes at most %d items and currencies together", MaxCostLines)
	}
	return c, nil
}

// Restock is when a shop item's stock fills again.
type Restock struct {
	// Daily and Weekly fill it at the start of each day, or each week
	// from Monday, on the server's clock.
	Daily, Weekly bool
	// Every fills it this long after the last fill.
	Every time.Duration
}

// ParseRestock reads a restock: "", "daily", "weekly", or a time of
// whole minutes, hours or days, such as 30m, 6h or 2d.
func ParseRestock(s string) (Restock, error) {
	switch s {
	case "":
		return Restock{}, nil
	case "daily":
		return Restock{Daily: true}, nil
	case "weekly":
		return Restock{Weekly: true}, nil
	}
	if len(s) < 2 {
		return Restock{}, fmt.Errorf("restock %q is not a time", s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 1 || n > 1_000_000 {
		return Restock{}, fmt.Errorf("restock %q is not a time", s)
	}
	unit := map[byte]time.Duration{'m': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour}[s[len(s)-1]]
	if unit == 0 {
		return Restock{}, fmt.Errorf("restock %q is not a time: use m, h or d", s)
	}
	return Restock{Every: time.Duration(n) * unit}, nil
}

// IsJobView is rAthena's pcdb_checkid (pc.hpp 1219): the classes that an
// NPC's sprite shows as a character, as status_set_viewdata (review
// decision 58).
func IsJobView(c uint32) bool {
	switch {
	case c < 31, // JOB_MAX_BASIC
		c >= 4001 && c <= 4052,          // JOB_NOVICE_HIGH to JOB_DARK_COLLECTOR
		c >= 4054 && c <= 4087,          // JOB_RUNE_KNIGHT to JOB_MECHANIC_T2
		c >= 4096 && c <= 4112,          // JOB_BABY_RUNE_KNIGHT to JOB_BABY_MECHANIC2
		c >= 4190 && c <= 4191,          // JOB_SUPER_NOVICE_E to JOB_SUPER_BABY_E
		c >= 4211 && c <= 4212,          // JOB_KAGEROU to JOB_OBORO
		c == 4215, c == 4218, c == 4220, // JOB_REBELLION, JOB_SUMMONER, JOB_BABY_SUMMONER
		c >= 4222 && c <= 4229, // JOB_BABY_NINJA to JOB_BABY_REBELLION
		c >= 4238 && c <= 4244, // JOB_BABY_STAR_GLADIATOR2 to JOB_BABY_STAR_EMPEROR2
		c >= 4252 && c <= 4264, // JOB_DRAGON_KNIGHT to JOB_TROUVERE
		c >= 4278 && c <= 4281, // JOB_WINDHAWK2 to JOB_IMPERIAL_GUARD2
		c >= 4302 && c <= 4308, // JOB_SKY_EMPEROR to JOB_SPIRIT_HANDLER
		c == 4316:              // JOB_SKY_EMPEROR2
		return true
	}
	return false
}
