package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// KROAssetsEnv names a full asset build of the kRO client, made with
// `wactl assets build --source placeholder --source grf:<client>`. The
// kRO end-to-end check runs only when it is set, and with
// WATHENA_TEST_DATABASE_URL (review decision 55). client/scripts reads
// the same variable for its browser check.
const KROAssetsEnv = "WATHENA_KRO_ASSETS"

// kroConfig runs every map of the build, with new characters in
// Prontera, as a classic world ships (review decision 55).
const kroConfig = `classic:
  start_map: prontera
  maps: all
mode: renewal
`

// Places of rAthena's renewal NPC files that the kRO check goes to.
var (
	// kaf_prontera2 stands at prontera,151,29 and the Fruit Gardener at
	// prontera,104,49 (npc/kafras/kafras.txt, npc/merchants/shops.txt).
	southKafra  = kroPlace{"map/prontera", 151, 29}
	fruitSeller = kroPlace{"map/prontera", 104, 49}
	// prt_fild08 is south of Prontera. Its Porings, Lunatics and Fabres
	// spawn anywhere, and 15 of them in 20 cells of 271,249
	// (npc/re/mobs/fields/prontera.txt).
	field      = kroPlace{"map/prt_fild08", 170, 360}
	fieldHerd  = kroPlace{"map/prt_fild08", 271, 249}
	pronteraIn = kroPlace{"map/prontera", 156, 40}
	// The Phoenix, the Taekwon job changer, stands at payon_in01,62,10
	// (npc/re/jobs/1-1e/taekwon.txt).
	phoenix = kroPlace{"map/payon_in01", 62, 10}
	// The Swordman job changer stands at izlude_in,74,172
	// (npc/re/jobs/1-1/swordman.txt). It converts since the skill
	// command (review decision 57).
	swordmanGuild = kroPlace{"map/izlude_in", 74, 172}
)

// Skills, jobs and items of the kRO check.
const (
	skillBasic  uint32 = 1 // NV_BASIC
	jobTaekwon         = 4046
	jobSwordman        = 1
	itemApple   uint32 = 512
)

// fieldMonsters are the monsters of prt_fild08.
var fieldMonsters = []string{"Poring", "Lunatic", "Fabre"}

// TestClassicKRO is the full kRO run of review decision 55: a classic
// world from `wactl world new --mode classic` on PostgreSQL, a server
// with every map of a full kRO asset build, `mode: renewal`, and core's
// converted NPCs. A new Novice of base level 20 and job level 10:
//   - starts in Prontera and learns Basic Skill;
//   - saves and opens storage at a Kafra;
//   - buys Apples from a converted shop;
//   - walks into the south warp to prt_fild08, kills a monster of the
//     field's spawns there, and walks back through the warp;
//   - takes the Kafra's teleport to Payon, and becomes a Taekwon at the
//     Phoenix, a converted job change NPC.
//
// A second Novice walks from Prontera to the Swordman Guild in Izlude
// and becomes a Swordman there (review decision 57).
//
// The bot walks along routes over the build's cells, in legs short
// enough for one walk command each, through rAthena's warps of
// warps.json. Set WATHENA_KRO_ASSETS to the build's folder to run it.
func TestClassicKRO(t *testing.T) {
	assets := os.Getenv(KROAssetsEnv)
	if assets == "" {
		t.Skip("set " + KROAssetsEnv + " to a full kRO asset build to run the kRO check")
	}
	srv := Start(t, Options{Database: true, Classic: true, Assets: assets, Config: kroConfig,
		StartJob: "0:20:10", StartItems: "501:10,909:5"})
	k := &kroRun{t: t, assets: assets, grids: map[string]*path.Grid{}, warps: kroWarps(t)}
	p := srv.Join(t, "Wanderer")
	k.p = p

	k.step("start in Prontera", func() string {
		p.Until("in Prontera", func(v View, _ Game) bool { return v.Map == "map/prontera" })
		v := p.View()
		for range 9 {
			p.Learn(skillBasic)
		}
		p.MustCommand("!zeny 20000")
		return fmt.Sprintf("at %d,%d with Basic Skill %d and %d zeny", v.X, v.Y, p.Game().Skills[skillBasic].GetLevel(), p.Stat(wathenav1.StatId_STAT_ID_ZENY))
	})

	k.step("save at the Kafra", func() string {
		k.travel(southKafra, 2)
		c := p.Talk("Kafra Employee")
		c.Next()
		c.Choose("Save")
		c.Says("has been saved here")
		said := strings.Join(c.Lines, " ")
		c.Close()
		return said
	})

	k.step("open storage at the Kafra", func() string {
		zeny := p.Stat(wathenav1.StatId_STAT_ID_ZENY)
		c := p.Talk("Kafra Employee")
		c.Next()
		c.Choose("Use Storage")
		c.Says("let me open")
		c.Close()
		p.storageOpened()
		if got := p.TryStorePut(Jellopy, 2); got != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
			t.Fatalf("storing 2 Jellopies: %s", got)
		}
		p.Until("2 Jellopies in storage", func(_ View, g Game) bool { return g.Storage != nil && g.Storage.Count(Jellopy) == 2 })
		p.send("close the storage", func(ctx context.Context) error { return p.StorageClose(ctx) })
		p.Until("the storage closed", func(_ View, g Game) bool { return g.Storage == nil })
		return fmt.Sprintf("the fee was %d zeny, and 2 Jellopies are in storage", zeny-p.Stat(wathenav1.StatId_STAT_ID_ZENY))
	})

	k.step("buy from a converted shop", func() string {
		k.travel(fruitSeller, 2)
		zeny := p.Stat(wathenav1.StatId_STAT_ID_ZENY)
		shop := p.OpenShop("Fruit Gardener")
		if got := p.Buy(shop, map[uint32]int{itemApple: 3}); got != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT {
			t.Fatalf("buying 3 Apples: %s", got)
		}
		p.Until("3 Apples", func(_ View, g Game) bool { return g.Count(itemApple) == 3 })
		return fmt.Sprintf("3 Apples from the Fruit Gardener for %d zeny", zeny-p.Stat(wathenav1.StatId_STAT_ID_ZENY))
	})

	k.step("walk into the warp to prt_fild08", func() string {
		hops := k.travel(field, 0)
		v := p.View()
		return fmt.Sprintf("%s, now at %d,%d", strings.Join(hops, ", "), v.X, v.Y)
	})

	k.step("fight a spawned monster", func() string {
		target, ok := k.monsterInView()
		if !ok {
			// The field's spawns are spread over 400 × 400 cells. The bot
			// walks toward the herd until one is in view.
			k.travelUntil(fieldHerd, 3, func() bool { target, ok = k.monsterInView(); return ok })
		}
		if !ok {
			t.Fatal("no monster of the field came in view")
		}
		dealt := p.Kill(target)
		return fmt.Sprintf("killed a %s with %d damage", target.Name, dealt)
	})

	k.step("walk back to Prontera", func() string {
		hops := k.travel(pronteraIn, 0)
		v := p.View()
		return fmt.Sprintf("%s, now at %d,%d", strings.Join(hops, ", "), v.X, v.Y)
	})

	k.step("take the Kafra's teleport to Payon", func() string {
		k.travel(southKafra, 2)
		zeny := p.Stat(wathenav1.StatId_STAT_ID_ZENY)
		c := p.Talk("Kafra Employee")
		c.Next()
		c.Choose("Use Teleport Service")
		if !c.Step.HasMenu() {
			c.Next()
		}
		opts := c.Step.GetMenu().GetOptions()
		i := slices.IndexFunc(opts, func(o string) bool { return strings.HasPrefix(o, "Payon") })
		if i < 0 {
			t.Fatalf("no Payon in %q", opts)
		}
		c.Pick(opts[i])
		m := p.MapChange("map/payon")
		p.Until("in Payon", func(v View, _ Game) bool { return v.Map == "map/payon" })
		return fmt.Sprintf("%q, arrived at %d,%d, paid %d zeny", opts[i], m.GetX(), m.GetY(), zeny-p.Stat(wathenav1.StatId_STAT_ID_ZENY))
	})

	k.step("change job at the Phoenix", func() string {
		hops := k.travel(phoenix, 2)
		// The first talk takes the Novice as a student, the second asks
		// the questions, and the third changes the job.
		k.talkThrough("Phoenix", "Okay, I will join you.")
		k.talkThrough("Phoenix", "I face it head on.", "Accept differences and learn from them")
		k.talkThrough("Phoenix", "Yes.")
		p.Until("a Taekwon", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_JOB] == jobTaekwon })
		return fmt.Sprintf("%s, job %d at job level %d", strings.Join(hops, ", "), p.Stat(wathenav1.StatId_STAT_ID_JOB), p.Stat(wathenav1.StatId_STAT_ID_JOB_LEVEL))
	})

	k.step("change job at the Swordman Guild", func() string {
		s := srv.Join(t, "Swordsman")
		k.p = s
		s.Until("in Prontera", func(v View, _ Game) bool { return v.Map == "map/prontera" })
		for range 9 {
			s.Learn(skillBasic)
		}
		s.Until("Basic Skill 9", func(_ View, g Game) bool { return g.Skills[skillBasic].GetLevel() == 9 })
		hops := k.travel(swordmanGuild, 2)
		k.talkThrough("Swordman", "I want to be a Swordman.", "Yes, I do.")
		s.Until("a Swordman", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_JOB] == jobSwordman })
		return fmt.Sprintf("%s, job %d at job level %d", strings.Join(hops, ", "), s.Stat(wathenav1.StatId_STAT_ID_JOB), s.Stat(wathenav1.StatId_STAT_ID_JOB_LEVEL))
	})
	srv.CheckLogs()
}

// kroPlace is a map and a cell.
type kroPlace struct {
	Map  string
	X, Y int
}

// kroWarp is a warp of warps.json: an area of XS and YS cells around its
// cell that takes a player to its target.
type kroWarp struct {
	Map   string `json:"map"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	XS    int    `json:"xs"`
	YS    int    `json:"ys"`
	ToMap string `json:"to_map"`
	ToX   int    `json:"to_x"`
	ToY   int    `json:"to_y"`
	State string `json:"state"`
}

// kroWarps are renewal's enabled warps by map, as the server loads them.
func kroWarps(t *testing.T) map[string][]kroWarp {
	t.Helper()
	data, err := reference.ReadFile("extracts/renewal/warps.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Warps []kroWarp `json:"warps"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string][]kroWarp{}
	for _, w := range doc.Warps {
		if w.State == "enabled" {
			out["map/"+w.Map] = append(out["map/"+w.Map], w)
		}
	}
	return out
}

// kroRun is the state of the kRO check.
type kroRun struct {
	t      *testing.T
	p      *Player
	assets string
	grids  map[string]*path.Grid
	warps  map[string][]kroWarp
}

// step runs one step and logs its result, so the test's log lists each
// step.
func (k *kroRun) step(name string, f func() string) {
	k.t.Helper()
	k.t.Logf("step %q: start", name)
	got := f()
	k.t.Logf("step %q: ok: %s", name, got)
}

// grid is a map's cells from the asset build, with every warp area
// closed, so routes never step onto a warp by chance.
func (k *kroRun) grid(mapName string) *path.Grid {
	k.t.Helper()
	g, err := k.loadGrid(mapName)
	if err != nil {
		k.t.Fatal(err)
	}
	return g
}

// loadGrid is grid, with an error for a map the build does not have.
func (k *kroRun) loadGrid(mapName string) (*path.Grid, error) {
	g, ok := k.grids[mapName]
	if !ok {
		var err error
		if g, err = world.LoadGrid(k.assets, mapName); err != nil {
			k.grids[mapName] = nil
			return nil, err
		}
		for _, w := range k.warps[mapName] {
			for y := w.Y - w.YS; y <= w.Y+w.YS; y++ {
				for x := w.X - w.XS; x <= w.X+w.XS; x++ {
					if x >= 0 && y >= 0 && x < g.Width && y < g.Height {
						g.Cells[y*g.Width+x] &^= path.Walkable
					}
				}
			}
		}
		k.grids[mapName] = g
	}
	if g == nil {
		return nil, fmt.Errorf("the build has no map %s", mapName)
	}
	return g, nil
}

// reach is a flood fill of the walkable cells from a cell: each reached
// cell's distance, or -1.
func reach(g *path.Grid, x, y int) []int32 {
	dist := make([]int32, g.Width*g.Height)
	for i := range dist {
		dist[i] = -1
	}
	if g.NoPass(x, y) {
		return dist
	}
	dist[y*g.Width+x] = 0
	queue := []int{y*g.Width + x}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		cx, cy := c%g.Width, c/g.Width
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				nx, ny := cx+dx, cy+dy
				if (dx == 0 && dy == 0) || g.NoPass(nx, ny) || dx != 0 && dy != 0 && (g.NoPass(cx+dx, cy) || g.NoPass(cx, cy+dy)) {
					continue
				}
				if n := ny*g.Width + nx; dist[n] < 0 {
					dist[n] = dist[c] + 1
					queue = append(queue, n)
				}
			}
		}
	}
	return dist
}

// near is the reached cell nearest to a cell within r cells of it, or
// false.
func near(g *path.Grid, dist []int32, x, y, r int) ([2]int, bool) {
	best, at := int32(-1), [2]int{}
	for d := 0; d <= r; d++ {
		for _, c := range ring([2]int{x, y}, d) {
			if c[0] < 0 || c[1] < 0 || c[0] >= g.Width || c[1] >= g.Height {
				continue
			}
			if v := dist[c[1]*g.Width+c[0]]; v >= 0 && (best < 0 || v < best) {
				best, at = v, c
			}
		}
		if best >= 0 {
			return at, true
		}
	}
	return at, false
}

// hop is one warp of a trip: the cell next to it to walk to, and the
// warp.
type hop struct {
	from [2]int
	warp kroWarp
}

// plan finds the warps from the bot's cell to a place: a search over
// maps, where each map's flood fill says which warps the bot reaches.
// It returns the hops and the cell to walk to on the last map.
func (k *kroRun) plan(to kroPlace, r int) ([]hop, [2]int) {
	k.t.Helper()
	v := k.p.View()
	type node struct {
		at   kroPlace
		hops []hop
	}
	seen := map[kroPlace]bool{{v.Map, v.X, v.Y}: true}
	queue := []node{{at: kroPlace{v.Map, v.X, v.Y}}}
	for len(queue) > 0 && len(seen) < 400 {
		n := queue[0]
		queue = queue[1:]
		g := k.grid(n.at.Map)
		dist := reach(g, n.at.X, n.at.Y)
		if n.at.Map == to.Map {
			if c, ok := near(g, dist, to.X, to.Y, max(r, 3)); ok {
				return n.hops, c
			}
		}
		for _, w := range k.warps[n.at.Map] {
			// A cell beside the area that the bot reaches.
			c, ok := near(g, dist, w.X, w.Y, max(w.XS, w.YS)+1)
			if !ok {
				continue
			}
			next := kroPlace{"map/" + w.ToMap, w.ToX, w.ToY}
			if seen[next] || k.warps[next.Map] == nil && next.Map != to.Map {
				continue
			}
			if _, err := k.loadGrid(next.Map); err != nil {
				continue
			}
			seen[next] = true
			queue = append(queue, node{at: next, hops: append(slices.Clone(n.hops), hop{c, w})})
		}
	}
	k.t.Fatalf("no way from %s %d,%d to %s %d,%d", v.Map, v.X, v.Y, to.Map, to.X, to.Y)
	return nil, [2]int{}
}

// travel goes to within r cells of a place, through warps, and returns
// the warps it took.
func (k *kroRun) travel(to kroPlace, r int) []string {
	k.t.Helper()
	return k.travelUntil(to, r, func() bool { return false })
}

// travelUntil is travel that stops when done says so, after a leg.
func (k *kroRun) travelUntil(to kroPlace, r int, done func() bool) []string {
	k.t.Helper()
	hops, last := k.plan(to, r)
	var took []string
	for _, h := range hops {
		if k.walk(h.from, done) {
			return took
		}
		w := h.warp
		in := k.warpCell(w)
		k.p.send("walk", func(ctx context.Context) error { return k.p.Bot.WalkTo(ctx, in[0], in[1]) })
		to := "map/" + w.ToMap
		k.p.MapChange(to)
		k.p.Until("on "+to, func(v View, _ Game) bool { return v.Map == to })
		took = append(took, fmt.Sprintf("%s %d,%d to %s", strings.TrimPrefix(w.Map, "map/"), w.X, w.Y, w.ToMap))
	}
	k.walk(last, done)
	return took
}

// warpCell is the walkable cell of a warp's area nearest to the bot. A
// warp's own cell may be a wall, such as at a map's edge.
func (k *kroRun) warpCell(w kroWarp) [2]int {
	k.t.Helper()
	g, err := world.LoadGrid(k.assets, "map/"+w.Map)
	if err != nil {
		k.t.Fatal(err)
	}
	v := k.p.View()
	best, at := -1, [2]int{w.X, w.Y}
	for y := w.Y - w.YS; y <= w.Y+w.YS; y++ {
		for x := w.X - w.XS; x <= w.X+w.XS; x++ {
			if g.NoPass(x, y) {
				continue
			}
			if d := max(abs(x-v.X), abs(y-v.Y)); best < 0 || d < best {
				best, at = d, [2]int{x, y}
			}
		}
	}
	return at
}

// walk walks to a cell of the bot's map along a route, in legs of up to
// 12 route cells, and reports whether done stopped it.
func (k *kroRun) walk(to [2]int, done func() bool) bool {
	k.t.Helper()
	v := k.p.View()
	route := botclient.Route(k.grid(v.Map), v.X, v.Y, to[0], to[1])
	if route == nil {
		k.t.Fatalf("no route on %s from %d,%d to %d,%d", v.Map, v.X, v.Y, to[0], to[1])
	}
	for i := 12; ; i += 12 {
		c := route[min(i, len(route)-1)]
		k.p.WalkTo(c[0], c[1])
		if done() {
			return true
		}
		if i >= len(route)-1 {
			return false
		}
	}
}

// ring are the cells at a distance from a cell.
func ring(c [2]int, r int) [][2]int {
	if r == 0 {
		return [][2]int{c}
	}
	var out [][2]int
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if max(abs(dx), abs(dy)) == r {
				out = append(out, [2]int{c[0] + dx, c[1] + dy})
			}
		}
	}
	return out
}

// monsterInView is the nearest monster of the field in view.
func (k *kroRun) monsterInView() (botclient.Entity, bool) {
	v := k.p.View()
	best, found := -1, botclient.Entity{}
	for _, e := range v.Entities {
		if e.Kind != wathenav1.EntityKind_ENTITY_KIND_MONSTER || !slices.Contains(fieldMonsters, e.Name) {
			continue
		}
		if d := max(abs(e.X-v.X), abs(e.Y-v.Y)); best < 0 || d < best {
			best, found = d, e
		}
	}
	return found, best >= 0
}

// talkThrough talks to an NPC until it ends: Next on each Next button,
// the answers in order on menus, and Close on the Close button.
func (k *kroRun) talkThrough(npc string, answers ...string) {
	k.t.Helper()
	c := k.p.Talk(npc)
	var said []string
	defer func() { k.t.Logf("%s said %q", npc, strings.Join(said, " ")) }()
	for {
		said = append(said, c.Lines...)
		switch {
		case c.Step.HasMenu():
			if len(answers) == 0 {
				k.t.Fatalf("%s asks %q, and no answer is left", npc, c.Step.GetMenu().GetOptions())
			}
			c.Choose(answers[0])
			answers = answers[1:]
		case c.Step.HasNext():
			c.Next()
		default:
			if len(answers) > 0 {
				k.t.Fatalf("%s ended with answers left: %q, saying %q", npc, answers, c.Lines)
			}
			c.Close()
			return
		}
	}
}
