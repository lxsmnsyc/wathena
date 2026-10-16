package zone

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// layoutWorld is a procedural world of three maps on a line: the towns
// t1 Valheim and t2 Skarvik, and the field between them, Valheim Fields,
// with the nomemo flag. Each town has a square at 10,10 and a teleport
// place at 12,12, where the teleport service stands. Teleports cost 100
// zeny plus 50 a warp.
type layoutWorld struct {
	t1, field, t2 *Zone
	atlas         *layout.Atlas
	mu            sync.Mutex
	renames       [][]layout.Rename
	failRename    bool
}

const (
	t1ID    = "gen:town:t1"
	t2ID    = "gen:town:t2"
	fieldID = "gen:field:0:0"
)

func newLayoutWorld(t *testing.T) *layoutWorld {
	t.Helper()
	l := &layout.Layout{
		Maps: []layout.Map{
			{ID: fieldID, Kind: gen.KindField, Name: "Valheim Fields", Levels: [2]int{12, 21}, Distance: 1, NoMemo: true},
			{ID: t1ID, Kind: gen.KindTown, Name: "Valheim", Town: 1, Levels: [2]int{1, 10}},
			{ID: t2ID, Kind: gen.KindTown, Name: "Skarvik", Town: 2, X: 1, Levels: [2]int{23, 32}, Distance: 2},
		},
		Links: []layout.Link{
			{A: layout.End{Map: fieldID, Tag: "portal:west"}, B: layout.End{Map: t1ID, Tag: "portal"}},
			{A: layout.End{Map: fieldID, Tag: "portal:east"}, B: layout.End{Map: t2ID, Tag: "portal"}},
		},
		Home: t1ID,
	}
	w := &layoutWorld{}
	w.atlas = layout.NewAtlas(l, layout.AtlasOptions{
		Blocked: func(n string) bool { return strings.EqualFold(n, "Badtown") },
		Save: func(_ context.Context, names []layout.Rename) error {
			w.mu.Lock()
			defer w.mu.Unlock()
			if w.failRename {
				return errors.New("the database is down")
			}
			w.renames = append(w.renames, names)
			return nil
		},
	})
	town := []gen.Place{{Tags: []string{"square"}, X: 10, Y: 10}, {Tags: []string{"teleport"}, X: 12, Y: 12}}
	idx := places.New([]places.Map{{ID: t1ID, Places: town}, {ID: t2ID, Places: town}, {ID: fieldID}}, nil)
	mods := &Mods{Limits: testLimits, Log: slog.New(slog.DiscardHandler), Server: variables.NewServer(nil, nil), Places: idx,
		Atlas: w.atlas, Teleport: TeleportPrices{Base: 100, Step: 50}}
	w.t1, w.field, w.t2 = saveZone(), saveZone(), saveZone()
	w.t1.Map, w.field.Map, w.t2.Map = t1ID, fieldID, t2ID
	d := zones{t1ID: w.t1, fieldID: w.field, t2ID: w.t2}
	hub := chat.NewHub(nil, chat.NewFilter(nil))
	reg := staff.NewRegistry(staff.MustDefaultRoles(), nil)
	reg.Load(map[int64]string{1: "admin"})
	npcs := TeleportNPCs(w.atlas, idx, 112)
	for _, z := range []*Zone{w.t1, w.field, w.t2} {
		z.World, z.Mods, z.SaveMap, z.Chat, z.Staff = d, mods, t1ID, hub, reg
		z.SetNPCs(npcs[z.Map])
		t.Cleanup(z.stopScripts)
	}
	return w
}

// join puts a character on a zone at a cell, with what it visited.
func (w *layoutWorld) join(z *Zone, name string, x, y int, visited ...string) *Player {
	c := savedCharacter()
	c.Name, c.Map, c.X, c.Y, c.Visited = name, z.Map, x, y, visited
	c.AccountID = 1
	c.Staff = staff.Login{AccountName: name + "Account", TwoFactor: true}
	sess := newSession(&saves{})
	sess.staff = c.Staff
	p := newPlayer(sess, c)
	sess.zone, sess.player = z, p
	z.join(p)
	return p
}

// worldMaps are the WorldMap events of the player's next frames.
func worldMaps(z *Zone, p *Player) []*wathenav1.WorldMap {
	z.stepAt(z.now + 50)
	var out []*wathenav1.WorldMap
	for _, e := range events(p) {
		if m := e.GetWorldMap(); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// TestVisitedMaps: a login records the map and sends the world map, a
// new map sends it again, and a map the character saw does not.
func TestVisitedMaps(t *testing.T) {
	w := newLayoutWorld(t)
	p := w.join(w.t1, "Walker", 5, 5)
	got := worldMaps(w.t1, p)
	if len(got) != 1 || !slices.Equal(p.visited, []string{t1ID}) {
		t.Fatalf("login: %d world maps, visited %v", len(got), p.visited)
	}
	m := got[0]
	if len(m.GetMaps()) != 2 || len(m.GetLinks()) != 1 {
		t.Fatalf("world map %v", m)
	}
	for _, n := range m.GetMaps() {
		switch n.GetId() {
		case t1ID:
			if !n.GetVisited() || n.GetName() != "Valheim" || n.GetMinLevel() != 1 || n.GetMaxLevel() != 10 || n.GetKind() != "town" {
				t.Errorf("t1 %v", n)
			}
		case fieldID:
			if n.GetVisited() || n.GetName() != "" || n.GetMaxLevel() != 0 {
				t.Errorf("the field %v", n)
			}
		}
	}
	w.t1.departAt(p, w.field, 20, 20)
	q := arrive(t, w.field)
	got = worldMaps(w.field, q)
	if len(got) != 1 || len(got[0].GetMaps()) != 3 || !slices.Equal(q.visited, []string{t1ID, fieldID}) {
		t.Fatalf("the field: %d world maps, visited %v", len(got), q.visited)
	}
	if c := w.field.snapshot(q); !slices.Equal(c.Visited, []string{t1ID, fieldID}) {
		t.Errorf("the save has %v", c.Visited)
	}
	w.field.departAt(q, w.t1, 5, 5)
	r := arrive(t, w.t1)
	if got := worldMaps(w.t1, r); len(got) != 0 {
		t.Errorf("a map seen before sent %d world maps", len(got))
	}
}

// departAt moves a player to another zone, as a warp does.
func (z *Zone) departAt(p *Player, to *Zone, x, y int) {
	z.depart(p, to, x, y)
}

// teleportNPC finds the teleport service on a zone.
func teleportNPC(t *testing.T, z *Zone) *entity {
	t.Helper()
	for e := range z.entities.all() {
		if e.npc != nil && e.npc.Service == ServiceTeleport {
			return e
		}
	}
	t.Fatal("no teleport service")
	return nil
}

// TestTeleportService: the service lists the towns the player visited
// with prices that rise with the warps, saves the player's point, and
// teleports for zeny.
func TestTeleportService(t *testing.T) {
	w := newLayoutWorld(t)
	n := teleportNPC(t, w.t1)
	if n.x != 12 || n.y != 12 || n.npc.DisplayName != TeleportNPCName {
		t.Fatalf("the service stands at %d,%d as %q", n.x, n.y, n.npc.DisplayName)
	}
	p := w.join(w.t1, "Traveler", 12, 11, t1ID, fieldID, t2ID)
	w.t1.stepAt(w.t1.now + 50)
	events(p)
	ev := click(w.t1, p, n)
	d := lastDialog(ev)
	if want := []string{"Save", "Skarvik - 200 z", "Cancel"}; d == nil || !slices.Equal(d.GetMenu().GetOptions(), want) {
		t.Fatalf("menu %v", d)
	}
	ev = answer(w.t1, p, reply(d.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(1)) }))
	if p.record.SaveMap != t1ID || p.record.SaveX != 12 || p.record.SaveY != 11 || !strings.Contains(lines(ev), "save point is set") {
		t.Errorf("save: %s %d,%d %q", p.record.SaveMap, p.record.SaveX, p.record.SaveY, lines(ev))
	}
	d = lastDialog(ev)
	answer(w.t1, p, reply(d.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	if p.talk != nil {
		t.Fatal("the talk did not end")
	}
	// Too little zeny.
	p.record.Zeny = 150
	d = lastDialog(click(w.t1, p, n))
	ev = answer(w.t1, p, reply(d.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(2)) }))
	if !strings.Contains(lines(ev), "not have enough zeny") || p.record.Zeny != 150 {
		t.Errorf("without zeny: %q, %d zeny", lines(ev), p.record.Zeny)
	}
	answer(w.t1, p, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	p.record.Zeny = 1000
	d = lastDialog(click(w.t1, p, n))
	answer(w.t1, p, reply(d.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(2)) }))
	q := arrive(t, w.t2)
	if q.record.Zeny != 800 {
		t.Errorf("%d zeny after the teleport", q.record.Zeny)
	}
	if dx, dy := q.entity.x-12, q.entity.y-12; dx < -1 || dx > 1 || dy < -1 || dy > 1 {
		t.Errorf("arrived at %d,%d, not next to the teleport place", q.entity.x, q.entity.y)
	}
	// A town the player has not visited is not listed.
	r := w.join(w.t1, "Newcomer", 12, 11, t1ID)
	w.t1.stepAt(w.t1.now + 50)
	events(r)
	if d := lastDialog(click(w.t1, r, n)); !slices.Equal(d.GetMenu().GetOptions(), []string{"Save", "Cancel"}) {
		t.Errorf("a newcomer's menu %v", d.GetMenu().GetOptions())
	}
	if got := (TeleportPrices{Base: 500, Step: 100}).TeleportPrice(3); got != 800 {
		t.Errorf("price %d", got)
	}
}

// TestMemo: /memo needs Warp Portal at level 2, refuses a nomemo map,
// keeps 3 points with the newest first, and replaces a point of the
// same map.
func TestMemo(t *testing.T) {
	w := newLayoutWorld(t)
	p := w.join(w.t1, "Priest", 5, 6)
	memo := func(z *Zone) string {
		z.handle(command{player: p, memo: true})
		return lastSystem(p)
	}
	if got := memo(w.t1); got != MemoNotLearned {
		t.Errorf("without Warp Portal: %q", got)
	}
	p.char.Skills = map[int]int{int(skill.IDWarp): 1}
	if got := memo(w.t1); got != MemoLowLevel {
		t.Errorf("level 1: %q", got)
	}
	p.char.Skills[int(skill.IDWarp)] = 4
	if got := memo(w.t1); got != MemoSaved || len(p.memos) != 1 || p.memos[0].Map != t1ID || p.memos[0].X != 5 || p.memos[0].Y != 6 {
		t.Errorf("memo: %q %v", got, p.memos)
	}
	p.memos = append(p.memos, p.memos[0])
	p.memos[1].Map, p.memos[0].Map = "map/b", "map/a"
	p.entity.x = 7
	memo(w.t1)
	if len(p.memos) != 3 || p.memos[0].Map != t1ID || p.memos[0].X != 7 || p.memos[1].Map != "map/a" || p.memos[2].Map != "map/b" {
		t.Errorf("three memos %v", p.memos)
	}
	p.entity.x = 9
	memo(w.t1)
	if len(p.memos) != 3 || p.memos[0].X != 9 || slices.IndexFunc(p.memos[1:], func(m character.Memo) bool { return m.Map == t1ID }) >= 0 {
		t.Errorf("the same map again %v", p.memos)
	}
	if c := w.t1.snapshot(p); !slices.Equal(c.Memos, p.memos) {
		t.Errorf("the save has %v", c.Memos)
	}
	// The field has the nomemo flag.
	w.t1.departAt(p, w.field, 20, 20)
	p = arrive(t, w.field)
	if got := memo(w.field); got != MemoRefused {
		t.Errorf("a nomemo map: %q", got)
	}
}

// TestGoWarpAndRename: @go lists and visits towns, @warp takes a display
// name, and @renametown renames a town and the maps named after it, with
// an audit row.
func TestGoWarpAndRename(t *testing.T) {
	w := newLayoutWorld(t)
	gm := w.join(w.t1, "Admin", 5, 5)
	say(w.t1, gm, "@go")
	if a := answers(gm); !slices.Contains(a, "1: Valheim") || !slices.Contains(a, "2: Skarvik") {
		t.Errorf("@go lists %v", a)
	}
	say(w.t1, gm, "@go Moon")
	if got := lastSystem(gm); got != "Town not found." {
		t.Errorf("@go Moon: %q", got)
	}
	say(w.t1, gm, "@go skarvik")
	q := arrive(t, w.t2)
	if dx, dy := q.entity.x-12, q.entity.y-12; dx < -1 || dx > 1 || dy < -1 || dy > 1 {
		t.Errorf("@go put the staff member at %d,%d", q.entity.x, q.entity.y)
	}
	if rows := w.t1.txAudit; len(rows) != 1 || rows[0].Action != "commands.go" || rows[0].Arguments["map"] != t2ID {
		t.Errorf("audit %+v", rows)
	}
	say(w.t2, q, "@warp valheim fields 7 8")
	r := arrive(t, w.field)
	if r.entity.x != 7 || r.entity.y != 8 {
		t.Errorf("@warp by name to %d,%d", r.entity.x, r.entity.y)
	}
	say(w.field, r, "@warp Valheim")
	r = arrive(t, w.t1)
	say(w.t1, r, "@renametown t1 Ab")
	if got := lastSystem(r); !strings.Contains(got, "cannot take that name") {
		t.Errorf("a short name: %q", got)
	}
	say(w.t1, r, "@renametown t1 Badtown")
	if got := lastSystem(r); !strings.Contains(got, "does not allow") {
		t.Errorf("a blocked name: %q", got)
	}
	say(w.t1, r, "@renametown Valheim Newheim")
	deadline := time.Now().Add(5 * time.Second)
	for !slices.Contains(answers(r), "Renamed Valheim to Newheim.") {
		select {
		case c := <-w.t1.commands:
			w.t1.handle(c)
		case <-time.After(time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("the rename did not finish")
		}
	}
	if n, _ := w.atlas.Name(fieldID); n != "Newheim Fields" {
		t.Errorf("the field is %q", n)
	}
	w.mu.Lock()
	saved := w.renames
	w.mu.Unlock()
	if len(saved) != 1 || saved[0][0] != (layout.Rename{Map: t1ID, Name: "Newheim"}) {
		t.Errorf("saved %v", saved)
	}
	last := w.t1.txAudit[len(w.t1.txAudit)-1]
	if last.Action != "world.rename" || last.Arguments["from"] != "Valheim" || last.Arguments["to"] != "Newheim" {
		t.Errorf("audit %+v", last)
	}
}
