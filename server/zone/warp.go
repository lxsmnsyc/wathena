package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/social"
)

// Warps and map changes, as rAthena's warp NPCs and pc_setpos:
//   - A player who steps onto a cell in a warp's area goes to the warp's
//     target (npc_touch_areanpc). The walk stops there, since the player
//     leaves the map.
//   - A warp to another map hands the player to that map's zone as a
//     message. The zone the player leaves forgets it before the message
//     goes, so the character is in one zone at a time.
//   - The new zone places the player at the target cell, saves the
//     character, and sends MapChange in the player's first frame.
//   - A player who enters on a cell in a warp's area is warped again, as
//     clif_parse_LoadEndAck does after a map loads. count_rewarp stops
//     loops after 10 warps in a row.

// WarpSprite is JT_WARPNPC, the NPC sprite ID of warp portals. Visible
// warps appear as ENTITY_KIND_NPC with this look.
const WarpSprite = 45

// maxRewarp is the most warps in a row that npc_touch_areanpc allows
// (count_rewarp).
const maxRewarp = 10

// Warp is a warp portal, as rAthena's NPCTYPE_WARP.
type Warp struct {
	// Name is the warp's unique name in rAthena. Players never see it.
	Name string
	X, Y int
	// XS and YS are how far the touch area reaches from X and Y.
	XS, YS int
	// To is the target map's asset name, such as map/prt_in.
	To       string
	ToX, ToY int
	// Hidden warps work but do not show, as rAthena's JT_HIDDEN_WARP_NPC.
	Hidden bool
}

// covers reports whether a warp's touch area holds a cell.
func (w *Warp) covers(x, y int) bool {
	return x >= w.X-w.XS && x <= w.X+w.XS && y >= w.Y-w.YS && y <= w.Y+w.YS
}

// Directory finds the zones of other maps. The world implements it.
type Directory interface {
	// Zone returns the zone that runs a map, by asset name, or nil.
	Zone(mapName string) *Zone
	// Announce shows a line to every player of every zone. It never
	// waits on a zone.
	Announce(text string, color uint32)
}

// SetWarps sets the map's warps, in rAthena's load order: when areas
// overlap, the first warp wins. Visible warps become entities that
// players see. It replaces the warps set before. Call it before Run.
func (z *Zone) SetWarps(warps []Warp) {
	for e := range z.entities.all() {
		if e.warp != nil {
			z.entities.remove(e)
		}
	}
	z.warps = warps
	for i := range warps {
		w := &warps[i]
		if !w.Hidden {
			z.entities.add(&entity{warp: w, x: w.X, y: w.Y})
		}
	}
}

// warpAt is the first warp whose area holds a cell, as
// npc_touch_area_allnpc checks warps in load order.
func (z *Zone) warpAt(x, y int) *Warp {
	for i := range z.warps {
		if z.warps[i].covers(x, y) {
			return &z.warps[i]
		}
	}
	return nil
}

// warpNear reports whether any warp's area comes within r cells of a
// cell, as npc_check_areanpc with flag 1.
func (z *Zone) warpNear(x, y, r int) bool {
	for i := range z.warps {
		w := &z.warps[i]
		if x+r >= w.X-w.XS && x-r <= w.X+w.XS && y+r >= w.Y-w.YS && y-r <= w.Y+w.YS {
			return true
		}
	}
	return false
}

// target returns the zone a warp leads to, or nil when the warp cannot
// take the player now, as npc_touch_areanpc and pc_setpos refuse it:
//   - The player is dead, or its connection was dropped.
//   - The player was warped more than maxRewarp times in a row. The
//     count starts again.
//   - The target map does not run in this world (SETPOS_NO_MAPSERVER).
//   - The session is ending.
func (z *Zone) target(p *Player, w *Warp) *Zone {
	if p.entity.dead || p.gone() || p.sess == nil || z.jailed(p) {
		// A jailed player cannot leave through a warp, as the jail map's
		// flags in rAthena.
		return nil
	}
	if p.rewarp > maxRewarp {
		p.rewarp = 0
		return nil
	}
	if w.To == z.Map {
		return z
	}
	if z.World == nil {
		return nil
	}
	return z.World.Zone(w.To)
}

// handoff is a player moving to another zone: its character at the
// target cell, and the state that the character does not save.
type handoff struct {
	sess *Session
	char *character.Character
	// ackSeq is the last command the player's old zone processed.
	ackSeq uint32
	// canUseItem, itemDelays and statusDelays are the item waits left, in
	// milliseconds.
	canUseItem   int64
	itemDelays   map[uint32]int64
	statusDelays map[string]int64
	// rewarp is count_rewarp.
	rewarp int
	// temp are the player's temporary variables, which last until
	// logout.
	temp map[character.Variable]string
	// cooldowns are the skill cooldowns left, in milliseconds, by skill.
	cooldowns map[int]int64
	// spiritBalls are the spirit spheres' times left, oldest first.
	spiritBalls []int64
	// member is the player in the chat hub, and seen the lines it saw,
	// for reports.
	member *chat.Member
	hub    *chat.Hub
	seen   []chat.Line
	// soc is the player in the social registry, and social the registry.
	soc    *social.Player
	social *social.Registry
	// gmHidden is @hide, which lasts across map changes.
	gmHidden bool
}

// warp sends a player through a warp, as npc_touch_areanpc and
// pc_setpos. It returns false when the warp cannot take the player, so
// the walk goes on.
func (z *Zone) warp(p *Player, w *Warp) bool {
	to := z.target(p, w)
	if to == nil {
		return false
	}
	if to == z {
		z.teleport(p, w.ToX, w.ToY)
		return true
	}
	return z.depart(p, to, w.ToX, w.ToY)
}

// depart hands a player to another zone. The player leaves this zone
// first: viewers see it go out of sight, as pc_setpos with CLR_OUTSIGHT.
// It returns false when the session is ending, since the zone's leave
// then saves the player here.
func (z *Zone) depart(p *Player, to *Zone, x, y int) bool {
	s := p.sess
	s.mu.Lock()
	if s.end != endNone {
		s.mu.Unlock()
		return false
	}
	s.zone, s.player = nil, nil
	s.mu.Unlock()
	// The player's transfers are committed before the new zone saves it.
	if p.transferred {
		z.commitTransfers()
	}

	// The conversation and the shop window stay behind. pc_setpos ends
	// the statuses that end on a map change; the others go with the
	// character.
	z.closeTalk(p)
	p.shop = nil
	z.closeStores(p)
	p.mail.open = false
	z.closeGuildStorage(p, true)
	z.storagesLeave(p, false)
	z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
	z.mapWarpStatuses(p)
	// unit_remove_map: the cast stops and the player's ground skills end.
	z.castCancel(p.entity, false)
	p.entity.pending = nil
	z.clearUnitGroups(p.entity)
	z.removeMap(p.entity)
	z.stopPassives(p)
	h := &handoff{sess: s, char: z.snapshot(p), ackSeq: p.ackSeq, rewarp: p.rewarp, temp: p.temp,
		member: p.member, hub: z.Chat, seen: p.seen, soc: p.soc, social: z.Social, gmHidden: p.gmHidden,
		canUseItem:   max(p.canUseItem-z.now, 0),
		itemDelays:   map[uint32]int64{},
		statusDelays: map[string]int64{},
	}
	h.char.Map, h.char.X, h.char.Y = to.Map, x, y
	for id, until := range p.itemDelays {
		if until > z.now {
			h.itemDelays[id] = until - z.now
		}
	}
	for st, until := range p.statusDelays {
		if until > z.now {
			h.statusDelays[st] = until - z.now
		}
	}
	for id, until := range p.cooldowns {
		if until > z.now {
			if h.cooldowns == nil {
				h.cooldowns = map[int]int64{}
			}
			h.cooldowns[id] = until - z.now
		}
	}
	h.spiritBalls = z.spheresLeft(p)
	// What the player has not seen yet goes out before it leaves, such
	// as the answer of a GM command that moved it.
	if len(p.pending) > 0 {
		z.queue(p, wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
			Tick: z.tick, AckSeq: p.ackSeq, Events: p.pending,
		}.Build()}.Build())
		p.pending = nil
	}
	z.remove(p, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT)
	z.dropStatuses(p.entity)
	// Worn items end their equip contexts here, and start them again in
	// the new zone.
	z.unwearAll(p)
	// The send runs on its own goroutine, so two zones that hand players
	// to each other never wait on each other. It starts once this zone's
	// messages to the player went out, so they come before the new
	// zone's.
	to.incoming.Add(1)
	z.then(func() {
		go func() {
			defer to.incoming.Done()
			select {
			case to.arrivals <- h:
			case <-to.stopped:
				h.finish(endLeave)
			}
		}()
	})
	return true
}

// arrive takes a player from another zone. A session that ended on the
// way is closed here instead.
func (z *Zone) arrive(h *handoff) {
	s := h.sess
	s.mu.Lock()
	if s.end != endNone {
		end := s.end
		s.mu.Unlock()
		h.finish(end)
		return
	}
	if z.forward(h) {
		// The map was removed: the player goes on to its town.
		s.mu.Unlock()
		return
	}
	p := newPlayer(s, h.char)
	p.arrival = h
	s.zone, s.player = z, p
	s.mu.Unlock()
	z.join(p)
}

// finish ends a session whose player is between zones: the last save,
// then the connection closes. A stale session gets its reason first.
func (h *handoff) finish(why ending) {
	s := h.sess
	if why == endStale && !s.outClosed.closed.Load() {
		select {
		case s.out <- loggedInElsewhereMessage():
		default:
		}
	}
	if s.saver != nil {
		s.saver.Save(h.char, true)
	}
	if h.hub != nil {
		h.hub.Offline(h.member)
	}
	if h.social != nil {
		h.social.Offline(h.soc, s)
	}
	s.outClosed.close(s.out)
	s.done()
}

// arrivalCell is where pc_setpos places a player on this map:
//   - A cell outside the map, or 0,0, becomes a random walkable cell at
//     least one cell from the edge and not next to a warp area
//     (teleport_on_portal is off by default). When none is found, the
//     start cell is used instead of refusing the warp.
//   - Any other cell is used as it is, even when it is not walkable, as
//     pc_setpos does.
func (z *Zone) arrivalCell(x, y int) (int, int) {
	w, h := z.grid.Width, z.grid.Height
	if x >= 0 && x < w && y >= 0 && y < h && (x != 0 || y != 0) {
		return x, y
	}
	if w > 2 && h > 2 {
		for range w * h * 3 {
			x, y = z.rand.IntN(w-2)+1, z.rand.IntN(h-2)+1
			if !z.grid.NoPass(x, y) && !z.warpNear(x, y, 1) {
				return x, y
			}
		}
	}
	return z.startCell()
}

// teleport moves a player within the zone, for a warp whose target is
// the same map. Viewers see it go out of sight and appear at the new
// cell. The player sees itself appear there, as after a revive.
func (z *Zone) teleport(p *Player, x, y int) {
	z.closeTalk(p)
	p.shop = nil
	e := p.entity
	x, y = z.arrivalCell(x, y)
	e.x, e.y = x, y
	e.path = e.path[:0]
	e.target, e.pickItem = nil, nil
	z.castCancel(e, false)
	e.pending = nil
	z.removeMap(e)
	z.forget(e)
	for o := range z.entities.all() {
		if o.player == nil || o == e {
			continue
		}
		if id, ok := o.player.netIDs.get(e); ok {
			o.player.netIDs.del(e)
			z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT)))
		}
	}
	p.fresh = true
	z.entered(p)
}

// entered records a player's arrival on a cell: count_rewarp goes up when
// the cell is next to a warp area, and the cell is touched once the
// player's map has loaded.
func (z *Zone) entered(p *Player) {
	if z.warpNear(p.entity.x, p.entity.y, 0) {
		p.rewarp++
	} else {
		p.rewarp = 0
	}
	p.touchOnEnter = true
}

// mapChange is the event that starts a player's first frame in a zone it
// warped to.
func (z *Zone) mapChange(p *Player) *wathenav1.Event {
	e := p.entity
	return wathenav1.Event_builder{MapChange: wathenav1.MapChange_builder{
		Map:   z.Map,
		X:     uint32(e.x), // #nosec G115 -- map cells are at most 1024.
		Y:     uint32(e.y), // #nosec G115 -- as above.
		NetId: p.netID(e),
	}.Build()}.Build()
}

func loggedInElsewhereMessage() *wathenav1.ServerMessage {
	return wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
		Reason:  wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE,
		Message: "Your character logged in somewhere else.",
	}.Build()}.Build()
}
