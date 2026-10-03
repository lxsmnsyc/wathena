package zone

import (
	"errors"
	"fmt"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
)

// Item scripts: a used item runs its Lua entry script in an item context
// (server/modscript). The calls it makes on the player land here:
// heals, items, item boxes, effects and warps.

// scriptWarp is a warp that a script asked for. The zone makes it after
// the script's event, so the item is spent first and the handler never
// sees the player leave under it.
type scriptWarp struct {
	to   *Zone
	x, y int
}

// systemMessage is a line of server text in the player's chat log.
func systemMessage(text string) *wathenav1.Event {
	return wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM,
		Text:    text,
	}.Build()}.Build()
}

// giveItem is getitem's loop: a stackable item goes in one add, others
// one at a time. It stops at the first add that fails, and the player
// sees why, as clif_additem with a failure flag.
func (z *Zone) giveItem(p *Player, id uint32, amount int, stacked bool) bool {
	step := amount
	if !stacked || !z.Items.Get(id).Stackable {
		step = 1
	}
	for given := 0; given < amount; given += step {
		if r := z.addItem(p, id, step); r != item.AddSuccess {
			p.pending = append(p.pending, pickUpFailed(0, pickUpFailure(r)))
			return false
		}
	}
	return true
}

// GiveGroup is getgroupitem: one use of an item group, with
// pc_get_itemgroup's rules. An item that does not fit is lost, and the
// player sees why, as in rAthena. Rentals are given as normal items.
func (h scriptHost) GiveGroup(id int64, group int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	z := h.z
	if z.Mods == nil || z.Mods.Groups == nil {
		return errors.New("player:give_item_from_group: the item groups are not loaded")
	}
	gr, ok := z.Mods.Groups.ByID(group)
	if !ok {
		return fmt.Errorf("player:give_item_from_group: there is no item group %d", group)
	}
	if z.pool == nil {
		z.pool = item.NewPool()
	}
	for _, g := range z.Mods.Groups.Open(gr, z.rand, z.pool) {
		// pc_get_itemgroup_sub gives a stackable item in one add when the
		// entry is stacked, and one at a time otherwise, carrying on after
		// a failure.
		step := g.Amount
		if !g.Stacked || !z.Items.Get(g.Item).Stackable {
			step = 1
		}
		d := z.Items.Get(g.Item)
		group := ""
		if g.Entry != nil && d.IsEquipment() {
			// pc_get_itemgroup_sub rolls an equipment item's random
			// options from the entry's group, for each copy.
			group = g.Entry.RandomOptionGroup
		}
		for given := 0; given < g.Amount; given += step {
			x := z.newExtra(d, false, rollSource{kind: sourceScript}, group)
			if r := z.addCopy(p, g.Item, step, 0, nil, x); r != item.AddSuccess {
				p.pending = append(p.pending, pickUpFailed(0, pickUpFailure(r)))
			}
		}
	}
	return nil
}

// Heal runs itemheal through pc_itemheal, or percentheal through
// pc_percentheal.
func (h scriptHost) Heal(id int64, percent bool, hp, sp [2]int) error {
	// With potion_flag, a thrown potion keeps the heal.
	if h.z.potionHeal(percent, hp, sp) {
		return nil
	}
	p, err := h.player(id)
	if err != nil {
		return err
	}
	cmd := "itemheal"
	if percent {
		cmd = "percentheal"
	}
	h.z.heal(p, &item.Heal{Command: cmd, HP: hp, SP: sp})
	return nil
}

// Warp asks for a warp after the event: to a map and cell, a random cell
// of the player's map (Fly Wing), or the save point (Butterfly Wing).
func (h scriptHost) Warp(id int64, where, mapName string, x, y int) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	z := h.z
	var w *scriptWarp
	switch where {
	case "random":
		if x, y, ok := z.randomCell(); ok {
			w = &scriptWarp{to: z, x: x, y: y}
		}
	case "save_point":
		if to, sx, sy := z.savePoint(p); to != nil {
			w = &scriptWarp{to: to, x: sx, y: sy}
		}
	case "map":
		if to := z.zoneOf("map/" + strings.TrimPrefix(mapName, "map/")); to != nil {
			w = &scriptWarp{to: to, x: x, y: y}
		}
	}
	if w == nil || p.entity.dead {
		return false, nil
	}
	p.warpTo = w
	return true, nil
}

// savePoint is where a player's save point is: its zone and cell, as
// rAthena's save_point. A character without one, or whose save map does
// not run, goes to the start map's start cell. It is nil when even the
// start map does not run, as in tests without a world.
func (z *Zone) savePoint(p *Player) (*Zone, int, int) {
	if m := p.record.SaveMap; m != "" {
		if to := z.zoneOf(m); to != nil {
			return to, p.record.SaveX, p.record.SaveY
		}
	}
	to := z.zoneOf(z.SaveMap)
	if to == nil {
		return nil, 0, 0
	}
	x, y := to.StartCell()
	return to, x, y
}

// zoneOf finds the zone of a map by asset name: this zone, or another
// one of the world.
func (z *Zone) zoneOf(mapName string) *Zone {
	if mapName == "" {
		return nil
	}
	if mapName == z.Map {
		return z
	}
	if z.World == nil {
		return nil
	}
	return z.World.Zone(mapName)
}

// randomCell is pc_randomwarp's search: up to 1000 tries for a walkable
// cell at least one cell from the edge and not next to a warp area
// (teleport_on_portal is off by default). Map flags such as noteleport
// do not exist yet.
func (z *Zone) randomCell() (int, int, bool) {
	w, h := z.grid.Width, z.grid.Height
	if w <= 2 || h <= 2 {
		return 0, 0, false
	}
	for range 1000 {
		x := z.rand.IntN(w-2) + 1
		y := z.rand.IntN(h-2) + 1
		if !z.grid.NoPass(x, y) && !z.warpNear(x, y, 1) {
			return x, y, true
		}
	}
	return 0, 0, false
}

// StartCell is the cell new characters start on. It reads only the
// map's cells, which never change, so other zones may call it.
func (z *Zone) StartCell() (int, int) { return z.startCell() }

// finishWarp makes the warp a script asked for.
func (z *Zone) finishWarp(p *Player) {
	w := p.warpTo
	p.warpTo = nil
	if w == nil || p.gone() || p.entity.dead {
		return
	}
	if w.to == z {
		z.teleport(p, w.x, w.y)
		return
	}
	if p.sess == nil {
		return
	}
	z.depart(p, w.to, w.x, w.y)
}

// ShowEffect plays an effect on the player for everyone who sees it, as
// specialeffect2. Headgear effects have no message yet, so they show
// nothing.
func (h scriptHost) ShowEffect(id int64, effectID int64, headgear bool) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	if !headgear {
		h.z.effects = append(h.z.effects, effect{p.entity, uint32(effectID)}) // #nosec G115 -- checked by the runtime.
	}
	return nil
}

// SkillLevel is pc_checkskill.
func (h scriptHost) SkillLevel(id int64, skill int64) (int, error) {
	p, err := h.player(id)
	if err != nil {
		return 0, err
	}
	return p.char.Skills[int(skill)], nil
}

// RandomN draws from the zone's secret stream.
func (h scriptHost) RandomN(n int64) int64 {
	if n <= 1 {
		return 0
	}
	return int64(h.z.rand.IntN(int(n)))
}

// ItemGroupID finds an item group by name.
func (h scriptHost) ItemGroupID(name string) (int64, bool) {
	if h.z.Mods == nil {
		return 0, false
	}
	g, ok := h.z.Mods.Groups.ByName(name)
	if !ok {
		return 0, false
	}
	return g.ID, true
}

// MonsterID finds a monster by AegisName.
func (h scriptHost) MonsterID(name string) (int64, bool) {
	if h.z.Mods == nil {
		return 0, false
	}
	id, ok := h.z.Mods.MonsterIDs[strings.ToUpper(name)]
	return id, ok
}
