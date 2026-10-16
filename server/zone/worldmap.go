package zone

import (
	"fmt"
	"slices"
	"strconv"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// The world layout in play (proposal 0002, review decision 43, decision
// 0041):
//
//   - Each character records the generated maps it enters. The world
//     map comes at login and each time it enters a map for the first
//     time: the maps it visited and the maps their warps lead to.
//   - /memo saves a Warp Portal memo point by map ID, as rAthena's
//     pc_memo. A map with the nomemo flag refuses it.
//   - The teleport service, an NPC at each town's teleport place, saves
//     the player's point and sends it to the towns it visited, for a
//     price that rises with the warps between the towns.

// ServiceTeleport is the NPC service of the teleport service.
const ServiceTeleport = "teleport"

// TeleportPrices are the teleport service's prices: Base, plus Step for
// each warp between the towns.
type TeleportPrices struct {
	Base, Step int64
}

// Default teleport prices.
const (
	DefaultTeleportBase = 500
	DefaultTeleportStep = 100
)

// TeleportNPCName is the display name of the teleport service's NPCs.
const TeleportNPCName = "Teleport Service"

// atlas returns the world layout, or nil.
func (z *Zone) atlas() *layout.Atlas {
	if z.Mods == nil {
		return nil
	}
	return z.Mods.Atlas
}

// visit records that a player entered the zone's map, and sends the
// world map when the map is new to it, or at login.
func (z *Zone) visit(p *Player, login bool) {
	a := z.atlas()
	if a == nil {
		return
	}
	_, known := a.Map(z.Map)
	fresh := known && !slices.Contains(p.visited, z.Map)
	if fresh {
		p.visited = append(p.visited, z.Map)
	}
	if fresh || login {
		z.sendWorldMap(p)
	}
}

// sendWorldMap sends the part of the map graph a player found.
func (z *Zone) sendWorldMap(p *Player) {
	nodes, links := z.atlas().Discovered(p.visited)
	b := wathenav1.WorldMap_builder{}
	for _, n := range nodes {
		nb := wathenav1.WorldMapNode_builder{Id: n.ID, Kind: n.Kind, X: int32(n.X), Y: int32(n.Y), Floor: uint32(n.Floor), Visited: n.Visited} // #nosec G115 -- grid coordinates and floors are small.
		if n.Visited {
			nb.Name, nb.MinLevel, nb.MaxLevel = n.Name, uint32(n.Levels[0]), uint32(n.Levels[1]) // #nosec G115 -- levels are 1 to 999.
		}
		b.Maps = append(b.Maps, nb.Build())
	}
	for _, l := range links {
		b.Links = append(b.Links, wathenav1.WorldMapLink_builder{A: l[0], B: l[1]}.Build())
	}
	p.pending = append(p.pending, wathenav1.Event_builder{WorldMap: b.Build()}.Build())
}

// HasVisited reports whether the player of a script handle entered a
// map, for player:has_visited.
func (h scriptHost) HasVisited(id int64, mapID string) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return slices.Contains(p.visited, mapID), nil
}

// Memo asks for a memo point at the player's cell (worldmap.go).
func (s *Session) Memo(seq uint32) bool {
	return s.send(command{seq: seq, memo: true})
}

// Memo answers, as rAthena's clif_skill_memomessage and
// clif_skill_teleportmessage.
const (
	MemoSaved      = "Saved the location as a memo point for Warp Portal."
	MemoNotLearned = "You haven't learned Warp Portal."
	MemoLowLevel   = "Warp Portal's level is not high enough."
	MemoRefused    = "This location cannot be memorized."
)

// memo is pc_memo with the position -1: the newest memo goes first, and
// an older memo of the same map goes. It needs Warp Portal at level 2 or
// higher, and a map without the nomemo flag.
func (z *Zone) memo(p *Player) {
	if p.entity.dead {
		return
	}
	if m, ok := z.atlas().Map(z.Map); ok && m.NoMemo {
		z.answer(p, MemoRefused)
		return
	}
	lv := p.char.Skills[int(skill.IDWarp)]
	switch {
	case lv < 1:
		z.answer(p, MemoNotLearned)
		return
	case lv < 2:
		z.answer(p, MemoLowLevel)
		return
	}
	memos := []character.Memo{{Map: z.Map, X: p.entity.x, Y: p.entity.y}}
	for _, m := range p.memos {
		if m.Map != z.Map && len(memos) < character.MaxMemos {
			memos = append(memos, m)
		}
	}
	p.memos = memos
	z.answer(p, MemoSaved)
}

// teleportTalk is a player's conversation with the teleport service.
type teleportTalk struct {
	// towns are the menu's towns, after "Save", and prices their prices.
	towns  []layout.Map
	prices []int64
}

// teleportPrices returns the zone's prices, with the defaults for 0.
func (z *Zone) teleportPrices() TeleportPrices {
	t := TeleportPrices{Base: DefaultTeleportBase, Step: DefaultTeleportStep}
	if z.Mods != nil {
		if z.Mods.Teleport.Base > 0 {
			t.Base = z.Mods.Teleport.Base
		}
		if z.Mods.Teleport.Step > 0 {
			t.Step = z.Mods.Teleport.Step
		}
	}
	return t
}

// TeleportPrice is the price of a teleport between two towns: the base
// plus the step for each warp between them.
func (t TeleportPrices) TeleportPrice(warps int) int64 { return t.Base + t.Step*int64(max(warps, 0)) }

// startTeleport opens the teleport service's menu: Save, each town the
// player visited but this one, with its price, and Cancel.
func (z *Zone) startTeleport(p *Player, e *entity) {
	a := z.atlas()
	tt := &teleportTalk{}
	prices := z.teleportPrices()
	options := []string{"Save"}
	for _, t := range a.Towns() {
		if t.ID == z.Map || !slices.Contains(p.visited, t.ID) {
			continue
		}
		d := a.Distance(z.Map, t.ID)
		if d < 0 {
			continue
		}
		price := prices.TeleportPrice(d)
		tt.towns, tt.prices = append(tt.towns, t), append(tt.prices, price)
		options = append(options, t.Name+" - "+strconv.FormatInt(price, 10)+" z")
	}
	options = append(options, "Cancel")
	p.talk = &talk{npc: e, teleport: tt}
	t := p.talk
	t.shown = true
	z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) {
		b.Lines = []string{"[" + TeleportNPCName + "]", "Where would you like to go?"}
	})
	z.serviceStep(p, t, modscript.Step{Kind: modscript.StepMenu, Options: options})
}

// serviceStep shows a step of a service's conversation that waits for
// an answer.
func (z *Zone) serviceStep(p *Player, t *talk, s modscript.Step) {
	p.dialogSeq++
	t.dialog, t.step, t.waiting = p.dialogSeq, s, true
	switch s.Kind {
	case modscript.StepMenu:
		t.deadline = z.now + timeoutMenu
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) {
			b.Menu = wathenav1.NpcMenu_builder{Options: s.Options}.Build()
		})
	default:
		t.deadline = z.now + timeoutNext
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Close = new(true) })
	}
}

// serviceSay ends a service's conversation with a line and the Close
// button.
func (z *Zone) serviceSay(p *Player, t *talk, line string) {
	z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Lines = []string{"[" + TeleportNPCName + "]", line} })
	p.dialogSeq++
	t.dialog, t.ended, t.waiting = p.dialogSeq, true, false
	t.deadline = z.now + timeoutNext
	z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Close = new(true) })
}

// teleportReply runs the menu choice of the teleport service: 1 saves
// the point at the player's cell, the towns teleport for their price,
// and the last option ends the talk.
func (z *Zone) teleportReply(p *Player, t *talk, choice int) {
	tt := t.teleport
	switch {
	case choice == 1:
		p.record.SaveMap, p.record.SaveX, p.record.SaveY = z.Map, p.entity.x, p.entity.y
		z.serviceSay(p, t, "Your save point is set here.")
	case choice >= 2 && choice-2 < len(tt.towns):
		town, price := tt.towns[choice-2], tt.prices[choice-2]
		to := z.zoneOf(town.ID)
		if to == nil {
			z.serviceSay(p, t, "That town cannot be reached now.")
			return
		}
		if p.record.Zeny < price || !z.payZeny(p, price, transfer.ActionTake, transfer.End(transfer.KindFee)) {
			z.serviceSay(p, t, "You do not have enough zeny.")
			return
		}
		x, y := to.townArrival()
		p.warpTo = &scriptWarp{to: to, x: x, y: y}
		z.closeTalk(p)
		z.finishWarp(p)
	default:
		z.closeTalk(p)
	}
}

// townArrival is where a teleport puts a player in this town: a walkable
// cell next to its teleport place, else its square, else its start cell.
func (z *Zone) townArrival() (int, int) {
	idx := z.Mods.Places
	scope, err := places.ParseScope("map:" + z.Map)
	if err == nil {
		for _, tag := range []string{"teleport", "square"} {
			if found := idx.Find(scope, tag); len(found) > 0 {
				pl := idx.Place(found[0])
				for _, d := range [][2]int{{0, -1}, {1, 0}, {-1, 0}, {0, 1}, {1, -1}, {-1, -1}, {1, 1}, {-1, 1}} {
					if x, y := pl.X+d[0], pl.Y+d[1]; !z.grid.NoPass(x, y) && z.warpAt(x, y) == nil {
						return x, y
					}
				}
				if !z.grid.NoPass(pl.X, pl.Y) {
					return pl.X, pl.Y
				}
			}
		}
	}
	return z.startCell()
}

// TeleportNPCs returns the teleport service's NPCs: one at each town's
// teleport place, by map ID. sprite is its look.
func TeleportNPCs(a *layout.Atlas, idx *places.Index, sprite uint32) map[string][]NPC {
	out := map[string][]NPC{}
	if a == nil {
		return out
	}
	scope, _ := places.ParseScope("town:any")
	for _, i := range idx.Find(scope, "teleport") {
		pl := idx.Place(i)
		if _, ok := a.Map(pl.Map); !ok || !gen.IsGenerated(pl.Map) {
			continue
		}
		out[pl.Map] = append(out[pl.Map], NPC{
			Name:        fmt.Sprintf("teleport_service#%s#%d", gen.Area(pl.Map), len(out[pl.Map])),
			DisplayName: TeleportNPCName, X: pl.X, Y: pl.Y, Sprite: sprite, Service: ServiceTeleport,
		})
	}
	return out
}
