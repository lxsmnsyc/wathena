package zone

import (
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/navi"
)

// Client navigation (proposal 0006, section 5; decision 0079):
//   - A route comes from server/navi.Graph: one node per map, an edge for
//     each visible warp that leaves its map, and the extra edges of
//     data/navi.yml (NaviExtra). A route is the list of cells to leave a
//     map at, fewest map changes first.
//   - player:navigate_to and a NaviCommand set the player's target and
//     send a NaviRoute. A target the graph cannot reach is sent alone
//     (found is unset).
//   - The route is made again when the player arrives on another map, and
//     the target is dropped when the player is on the target map or sends
//     a cancel. Nothing is saved.
//   - A NaviCommand within a second of the last is dropped, and one for a
//     map that does not run is ignored. A script's call has neither limit
//     and returns false for a map that does not run.
//   - Each zone keeps its edges in an atomic copy, since the graph is
//     made from the edges of every zone and zones run on their own
//     goroutines.

// naviInterval is the least time between two requests of a client.
const naviInterval = int64(1000)

// naviTarget is where a player's navigation points.
type naviTarget struct {
	// name is the map's name as the player asked, such as prontera.
	name       string
	x, y       int
	monster    int
	services   uint32
	hideWindow bool
}

// naviCommand is a NaviCommand.
type naviCommand struct {
	target naviTarget
	cancel bool
}

// zoneLister is the world's list of zones, which routes read. A
// Directory without it leaves routes to the zone's own warps.
type zoneLister interface {
	Zones() []*Zone
}

// Navi asks for a route to a map, or cancels the player's route, as a
// click on a <NAVI> link of a dialog does. The gateway has checked the
// values: cells to 65535, services to 255.
func (s *Session) Navi(seq uint32, name string, x, y, monster, services uint32, hide, cancel bool) bool {
	return s.send(command{seq: seq, navi: &naviCommand{cancel: cancel, target: naviTarget{
		name: name, x: int(x), y: int(y), monster: int(monster), services: services, hideWindow: hide,
	}}})
}

// setNaviEdges keeps the visible warps of the zone that lead to another
// map, for routes. It runs where the warps change.
func (z *Zone) setNaviEdges() {
	var edges []navi.Edge
	for i := range z.warps {
		w := &z.warps[i]
		if w.Hidden || w.To == z.Map {
			continue
		}
		edges = append(edges, navi.Edge{From: z.Map, To: w.To, X: w.X, Y: w.Y, ToX: w.ToX, ToY: w.ToY})
	}
	z.naviEdges.Store(&edges)
}

// naviGraph makes the graph of every zone's warps and the extra edges.
// The zones are in map order, so equal routes come out the same.
func (z *Zone) naviGraph() *navi.Graph {
	g := navi.New()
	zones := []*Zone{z}
	if l, ok := z.World.(zoneLister); ok {
		zones = l.Zones()
	}
	for _, o := range zones {
		if es := o.naviEdges.Load(); es != nil {
			for _, e := range *es {
				g.Add(e)
			}
		}
	}
	for _, e := range z.NaviExtra {
		g.Add(e)
	}
	return g
}

// naviCommand is a NaviCommand: the client's link or cancel.
func (z *Zone) naviCommand(p *Player, c *naviCommand) {
	if c.cancel {
		if p.navi != nil {
			p.navi = nil
			p.pending = append(p.pending, wathenav1.Event_builder{NaviRoute: wathenav1.NaviRoute_builder{Cancel: true}.Build()}.Build())
		}
		return
	}
	if p.naviAt != 0 && z.now-p.naviAt < naviInterval {
		return
	}
	p.naviAt = z.now
	if z.zoneOf(navi.Asset(c.target.name)) == nil || len(c.target.name) > 64 {
		return
	}
	t := c.target
	p.navi = &t
	z.sendNavi(p)
}

// startNavi starts a navigation from a script. It returns false when the
// map does not run.
func (z *Zone) startNavi(p *Player, t naviTarget) bool {
	if t.name == "" || z.zoneOf(navi.Asset(t.name)) == nil {
		return false
	}
	p.navi = &t
	z.sendNavi(p)
	return true
}

// sendNavi sends the player's route from this map, and drops the target
// once the player is on its map.
func (z *Zone) sendNavi(p *Player) {
	t := p.navi
	if t == nil {
		return
	}
	target := navi.Asset(t.name)
	// #nosec G115 -- cells are 0 to 65535 and IDs positive, checked by the callers.
	b := wathenav1.NaviRoute_builder{Map: strings.TrimPrefix(target, "map/"), X: uint32(t.x), Y: uint32(t.y), Monster: uint32(t.monster),
		Services: t.services, HideWindow: t.hideWindow}
	steps, ok := z.naviGraph().Route(z.Map, target, navi.FromFlag(t.services))
	b.Found = ok
	for _, s := range steps {
		b.Steps = append(b.Steps, wathenav1.NaviStep_builder{Map: strings.TrimPrefix(s.Map, "map/"), X: uint32(s.X), Y: uint32(s.Y)}.Build()) // #nosec G115 -- cells.
	}
	p.pending = append(p.pending, wathenav1.Event_builder{NaviRoute: b.Build()}.Build())
	if z.Map == target {
		p.navi = nil
	}
}

// Navigate is player:navigate_to.
func (h scriptHost) Navigate(player int64, to modscript.NaviTarget) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	return h.z.startNavi(p, naviTarget{name: to.Map, x: to.X, y: to.Y, monster: to.Monster,
		services: uint32(to.Services), hideWindow: to.HideWindow}), nil // #nosec G115 -- 0 to 255, checked by the runtime.
}

var _ modscript.NaviHost = scriptHost{}
