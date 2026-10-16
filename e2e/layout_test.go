package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// layoutConfig is a small world layout: 2 × 2 fields of 64 cells, two
// towns and a dungeon, with cheap teleports.
const layoutConfig = `procedural:
  field_size: 64
  layout: {grid: [2, 2], towns: 2, dungeons: 1}
  teleport_base: 100
  teleport_step: 10
`

// mapInfo is the part of /gen/maps/<id>.json the test reads.
type mapInfo struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
	Levels      []int  `json:"levels"`
	Places      []struct {
		Tags []string `json:"tags"`
		X, Y int
	} `json:"places"`
}

// get reads a path of the game server.
func get(t *testing.T, srv *Server, p string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.HTTP()+p, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %v", p, res.StatusCode, err)
	}
	return b
}

func readMap(t *testing.T, srv *Server, id string) mapInfo {
	t.Helper()
	var m mapInfo
	if err := json.Unmarshal(get(t, srv, "/gen/maps/"+url.PathEscape(id)+".json"), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// place returns a map's first place with a tag.
func (m mapInfo) place(t *testing.T, tag string) (int, int) {
	t.Helper()
	for _, p := range m.Places {
		if slices.Contains(p.Tags, tag) {
			return p.X, p.Y
		}
	}
	t.Fatalf("%s has no place %s", m.ID, tag)
	return 0, 0
}

// route finds a walk on a generated map's cells, by 8 directions with
// rAthena's rule for diagonals, from the player's cell to the first
// walkable cell within reach cells of a target, and walks it in legs
// the server's path search takes. With onto, the last leg steps onto
// the target, such as a portal, and the walk does not wait to stand
// there.
func route(t *testing.T, srv *Server, p *Player, tx, ty, reach int, onto bool) {
	t.Helper()
	v := p.View()
	w, h, flags, err := wmap.DecodeCells(get(t, srv, "/gen/maps/"+url.PathEscape(v.Map)+".wmap"))
	if err != nil {
		t.Fatal(err)
	}
	ok := func(x, y int) bool { return x >= 0 && y >= 0 && x < w && y < h && flags[y*w+x]&path.Walkable != 0 }
	prev := make([]int, w*h)
	for i := range prev {
		prev[i] = -2
	}
	start := v.Y*w + v.X
	prev[start] = -1
	goal := -1
	for q := []int{start}; len(q) > 0 && goal < 0; q = q[1:] {
		c := q[0]
		cx, cy := c%w, c/w
		if abs(cx-tx) <= reach && abs(cy-ty) <= reach && (reach > 0 || onto) {
			goal = c
			break
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				nx, ny := cx+dx, cy+dy
				if (dx == 0 && dy == 0) || !ok(nx, ny) || prev[ny*w+nx] != -2 || (dx != 0 && dy != 0 && (!ok(cx+dx, cy) || !ok(cx, cy+dy))) {
					continue
				}
				prev[ny*w+nx] = c
				q = append(q, ny*w+nx)
			}
		}
	}
	if goal < 0 {
		t.Fatalf("no walk on %s from %d,%d to %d,%d", v.Map, v.X, v.Y, tx, ty)
	}
	var cells []int
	for c := goal; c >= 0; c = prev[c] {
		cells = append(cells, c)
	}
	slices.Reverse(cells)
	for i := 20; i < len(cells)-1; i += 20 {
		p.WalkTo(cells[i]%w, cells[i]/w)
	}
	last := cells[len(cells)-1]
	if !onto {
		p.WalkTo(last%w, last/w)
		return
	}
	p.send("walk", func(ctx context.Context) error { return p.Bot.WalkTo(ctx, last%w, last/w) })
}

// TestWorldLayout plays a procedural world that wactl world new made,
// with PostgreSQL (decision 0041):
//   - A new character starts on the Home Village's start place.
//   - It walks through the home map's portal into the first town, and
//     the world map shows both.
//   - It saves its point at the first town's teleport service, visits
//     the second town, and teleports there for its price.
//   - It dies there, and comes back at its save point.
//   - The maps it entered are saved.
func TestWorldLayout(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, Procedural: true, Config: layoutConfig})
	home := readMap(t, srv, "gen:home:village")
	if home.Kind != "home" || home.DisplayName != "Willow Village" || len(home.Levels) != 2 || home.Levels[0] != 1 {
		t.Fatalf("the home map %+v", home)
	}
	p := srv.Join(t, "Settler")
	sx, sy := home.place(t, "start")
	p.Until("standing on the home map's start", func(v View, _ Game) bool { return v.Map == "gen:home:village" && v.X == sx && v.Y == sy })
	p.Expect("the world map at login", func(e *wathenav1.Event) bool { return e.HasWorldMap() })

	// Through the portal into the first town.
	px, py := home.place(t, "portal")
	route(t, srv, p, px, py, 0, true)
	p.MapChange("gen:town:t1")
	wm := p.Expect("the world map with the town", func(e *wathenav1.Event) bool {
		m := e.GetWorldMap()
		return m != nil && slices.ContainsFunc(m.GetMaps(), func(n *wathenav1.WorldMapNode) bool { return n.GetId() == "gen:town:t1" && n.GetVisited() })
	}).GetWorldMap()
	t1 := readMap(t, srv, "gen:town:t1")
	t2 := readMap(t, srv, "gen:town:t2")
	for _, n := range wm.GetMaps() {
		if n.GetId() == "gen:town:t1" && n.GetName() != t1.DisplayName {
			t.Errorf("the world map names t1 %q, the map %q", n.GetName(), t1.DisplayName)
		}
	}
	if t1.DisplayName == "" || t2.DisplayName == "" || t1.DisplayName == t2.DisplayName {
		t.Fatalf("town names %q and %q", t1.DisplayName, t2.DisplayName)
	}

	// Save at the teleport service.
	tx, ty := t1.place(t, "teleport")
	route(t, srv, p, tx, ty, 2, false)
	saveX, saveY := p.View().X, p.View().Y
	c := p.Talk("Teleport Service")
	c.Choose("Save")
	c.Says("save point is set")
	c.Close()

	// Visit the second town with @go, and come back.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.GiveStaffRole(ctx, "Settler", "admin"); err != nil {
		t.Fatal(err)
	}
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, "@go 2") })
	p.MapChange("gen:town:t2")
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, "@go 1") })
	p.MapChange("gen:town:t1")
	p.Until("back in the first town", func(v View, _ Game) bool { return v.Map == "gen:town:t1" && !v.Walking })

	// Teleport to the second town for its price.
	p.Say("!zeny 1000")
	route(t, srv, p, tx, ty, 2, false)
	before := p.Stat(wathenav1.StatId_STAT_ID_ZENY)
	c = p.Talk("Teleport Service")
	var option string
	for _, o := range c.Step.GetMenu().GetOptions() {
		if strings.HasPrefix(o, t2.DisplayName+" - ") {
			option = o
		}
	}
	if option == "" {
		t.Fatalf("the menu %v has no %s", c.Step.GetMenu().GetOptions(), t2.DisplayName)
	}
	price, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(option, t2.DisplayName+" - "), " z"), 10, 64)
	if err != nil || price <= 100 {
		t.Fatalf("the price of %q: %v", option, err)
	}
	c.Pick(option)
	p.MapChange("gen:town:t2")
	p.Until("paid for the teleport", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == before-price })

	// Die in the second town, and come back at the save point.
	p.Say("!slayer")
	p.MapChange("gen:town:t1")
	p.Until("back at the save point", func(v View, _ Game) bool {
		return v.Map == "gen:town:t1" && v.X == saveX && v.Y == saveY && v.HP > 0
	})

	// The maps it entered are saved.
	srv.Stop()
	var visited []string
	rows, err := srv.DB.Query(ctx, `SELECT m.map_id FROM character_maps m JOIN characters c ON c.id = m.character_id
		WHERE c.name = 'Settler' ORDER BY m.seq`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			t.Fatal(err)
		}
		visited = append(visited, m)
	}
	if !slices.Equal(visited, []string{"gen:home:village", "gen:town:t1", "gen:town:t2"}) {
		t.Errorf("visited %v", visited)
	}
	var saveMap string
	if err := srv.DB.QueryRow(ctx, "SELECT save_map FROM characters WHERE name = 'Settler'").Scan(&saveMap); err != nil || saveMap != "gen:town:t1" {
		t.Errorf("the save map %q, %v", saveMap, err)
	}
	srv.CheckLogs()
}
