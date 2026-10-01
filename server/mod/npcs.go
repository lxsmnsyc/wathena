package mod

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
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
	// name after the mod ID and before any "#".
	DisplayName string
	// Map is the map's asset name, such as map/prontera.
	Map  string
	X, Y int
	// Direction is 0 to 7, from Directions.
	Direction int
	// Sprite is the NPC sprite ID, as the protocol's Look.body.
	Sprite uint32
	// Script is the NPC's entry script, relative to the folder of
	// ScriptMod.
	Script    string
	ScriptMod string
	// Shop lists what the NPC sells, or nil. A record with a shop and no
	// map is not placed: scripts open it with player:open_shop.
	Shop []ShopItem
	// Discount lets Discount and Compulsion Discount lower the shop's
	// prices. It is true unless the record sets discount: false.
	Discount bool
	// Placed is set for an NPC on a map.
	Placed bool
}

// ShopItem is one item of a shop: its name and its price in zeny, or -1
// for the item's buy price.
type ShopItem struct {
	Item  string
	Price int
}

// npcFields are the fields of an NPC record.
var npcFields = []string{"map", "x", "y", "direction", "sprite", "script", "display_name", "shop", "discount"}

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
		items, err := readShop(shop)
		if err != nil {
			errs = append(errs, err.Error())
		}
		n.Shop = items
		n.Discount = true
	}
	if v, ok := r.Fields["discount"]; ok {
		b, isBool := v.(bool)
		if !isBool || n.Shop == nil {
			errs = append(errs, "discount is true or false, on a record with a shop")
		}
		n.Discount = b
	}
	_, hasMap := r.Fields["map"]
	if !hasMap && n.Shop != nil {
		// A shop that scripts open. It has no place.
		for _, k := range []string{"x", "y", "direction", "sprite", "script", "display_name"} {
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
	if m, ok := str("map"); ok && m != "" && !strings.Contains(m, "/") {
		n.Map = "map/" + m
	} else if ok && strings.Contains(m, "/") {
		n.Map = m
	} else {
		errs = append(errs, "map is required, such as prontera")
	}
	var okX, okY bool
	n.X, okX = num("x")
	n.Y, okY = num("y")
	if !okX || !okY || n.X < 0 || n.Y < 0 || n.X > 1023 || n.Y > 1023 {
		errs = append(errs, "x and y are required: cells from 0 to 1023")
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
	case string:
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
		errs = append(errs, "sprite is required: an NPC sprite name, such as 4_F_KAFRA1")
	}
	if d, ok := r.Fields["display_name"]; ok {
		name, isText := d.(string)
		// An empty name shows no name, as rAthena's names that start
		// with #.
		if !isText || name != "" && validLocalName(name) != nil {
			errs = append(errs, "display_name is 0 to 64 characters")
		}
		n.DisplayName = name
	} else {
		n.DisplayName, _, _ = strings.Cut(Local(r.Name), "#")
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
	if len(errs) > 0 {
		return NPC{}, errors.New(strings.Join(errs, "; "))
	}
	return n, nil
}

// readShop reads a shop field: a list of {item, price}, where price is
// optional and -1 or a missing price is the item's buy price.
func readShop(v any) ([]ShopItem, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 || len(list) > MaxShopItems {
		return nil, fmt.Errorf("shop is a list of 1 to %d items, each {item: Red_Potion, price: 50}", MaxShopItems)
	}
	out := make([]ShopItem, 0, len(list))
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("shop[%d] is {item: <name>, price: <zeny>}", i)
		}
		name, ok := m["item"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("shop[%d]: item is an item's name, such as Red_Potion", i)
		}
		it := ShopItem{Item: name, Price: -1}
		for k, val := range m {
			switch k {
			case "item":
			case "price":
				p, ok := val.(int)
				if !ok || p < -1 || p > 2_147_483_647 {
					return nil, fmt.Errorf("shop[%d]: price is 0 to 2147483647 zeny, or -1 for the item's buy price", i)
				}
				it.Price = p
			default:
				return nil, fmt.Errorf("shop[%d]: %q is not a field; shop items have item and price", i, k)
			}
		}
		out = append(out, it)
	}
	return out, nil
}
