package e2e

import (
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// economyConfig is a default world layout with fields of 200 cells, so
// the server fits a small machine. The density per cell, which the hunt
// depends on, is a real world's.
const economyConfig = `procedural:
  field_size: 200
`

// economyBand is a sampled field's band: the fields whose lowest level
// is closest to it.
var economyBands = []int{1, 25, 49, 73}

// botsPerField hunt each sampled field at once, from different cells.
const botsPerField = 3

// TestEconomyBots plays the bot runs of strategy section 8.13 (decision
// 0046): bots of the model player hunt a sample of fields across the
// bands of a world that wactl world new made, in a real waserver with
// PostgreSQL, and their exp and zeny per hour must be within the
// tolerance of the simulator's (20%, the phase 7 exit criteria). It runs
// only with WATHENA_ECONOMY_MINUTES, the minutes the bots hunt, as
// scripts/check.sh economy sets it.
//
// Each bot is the model player of its field's band: the job, levels,
// stats, gear and potions the simulator fights with, written in the
// database. It hunts the nearest monster of the field's lines that are
// not bosses, picks up the drops of its kills, and drinks a potion below
// half HP. Zeny is the NPC sell value of what it picks up. Its exp counts
// across level ups. The character goes back to the model's levels and
// items in the database every segment, so it does not outgrow the model;
// time between segments does not count.
func TestEconomyBots(t *testing.T) {
	minutes, _ := strconv.Atoi(os.Getenv("WATHENA_ECONOMY_MINUTES"))
	if minutes <= 0 {
		t.Skip("WATHENA_ECONOMY_MINUTES is not set; scripts/check.sh economy runs the bot runs")
	}
	sim, monsters := economySim(t)
	srv := Start(t, Options{Database: true, Procedural: true, Config: economyConfig})
	fields := economyFields(t, srv, sim, monsters)
	for i := range fields {
		fields[i].maxFight = time.Duration(sim.Config.Hunt.MaxFightMs) * time.Millisecond
	}
	var mu sync.Mutex
	results := map[string]*huntTotals{}
	var wg sync.WaitGroup
	for fi, f := range fields {
		m, err := sim.Model(f.level)
		if err != nil {
			t.Fatal(err)
		}
		cells := f.grid.cells()
		for i := range botsPerField {
			wg.Add(1)
			go func() {
				defer wg.Done()
				name := fmt.Sprintf("Hunter%d%d", fi, i)
				tot := runHunter(t, srv, name, f, m, cells, monsters, time.Duration(minutes)*time.Minute, uint64(fi*10+i))
				mu.Lock()
				if results[f.id] == nil {
					results[f.id] = &huntTotals{}
				}
				results[f.id].add(tot)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	tol := sim.Config.Tolerance.Bots
	var report strings.Builder
	fmt.Fprintf(&report, "bot runs: %d bots per field, %d minutes each, against the simulator within %d%%\n", botsPerField, minutes, tol)
	for _, f := range fields {
		r, err := sim.Simulate(f.level, f.hunt)
		if err != nil {
			t.Fatal(err)
		}
		got := results[f.id]
		if got == nil || got.ms == 0 {
			t.Errorf("%s: no bot hunted", f.id)
			continue
		}
		per := func(v int64) int64 { return v * 3_600_000 / got.ms }
		exp, zeny, kills, job := per(got.base), per(got.zeny), per(got.kills), per(got.job)
		line := fmt.Sprintf("%s band %d-%d level %d: %.1f bot minutes, %d kills, %d deaths; kills/h %d (sim %d), base exp/h %d (sim %d, %d%%), job exp/h %d (sim %d), zeny/h %d (sim %d, %d%%), potions %d (sim supplies %d zeny/h, bots %d zeny/h)",
			f.id, f.band[0], f.band[1], f.level, float64(got.ms)/60000, got.kills, got.deaths, kills, r.Kills,
			exp, r.BaseExp, economy.Ratio(exp, r.BaseExp), job, r.JobExp, zeny, r.Zeny, economy.Ratio(zeny, r.Zeny),
			got.potions, r.Supplies, per(got.potions*int64(m0(sim, f.level).PotionPrice)))
		t.Log(line)
		if got.kills > 0 {
			t.Logf("  per kill: approach %d ms, fight %d ms, loot %d ms, given up %d ms, idle %d ms; items fell %d, picked %d, value fell %d zeny/h",
				got.approachMs/got.kills, got.fightMs/got.kills, got.lootMs/got.kills, got.lostMs/got.kills, got.idleMs/got.kills,
				got.fell, got.picked, per(got.fellValue))
		}
		var fightMs, k int64
		for i, l := range f.hunt.Lines {
			mon, _ := sim.Monster(l.Monster)
			fg, _ := sim.Fight(f.level, mon)
			fightMs += r.Lines[i] * fg.FightMs
			k += r.Lines[i]
		}
		hunted := 0
		for _, l := range f.hunt.Lines {
			mon, _ := sim.Monster(l.Monster)
			if alone, _ := sim.LeftAlone(f.level, mon); !alone {
				hunted += l.Count
			}
		}
		if k > 0 {
			t.Logf("  sim per kill: %d ms in all, fight %d ms; %d monsters hunted on %d cells, a side of %d cells each",
				int64(3_600_000_000)/max(k, 1), fightMs/k, hunted, f.hunt.Walkable, isqrt64(int64(f.hunt.Walkable)/int64(max(hunted, 1))))
		}
		report.WriteString(line + "\n")
		if !economy.Within(exp, r.BaseExp, tol) {
			t.Errorf("%s: the bots' base exp %d/h is %d%% of the simulator's %d/h", f.id, exp, economy.Ratio(exp, r.BaseExp), r.BaseExp)
		}
		// The drops are rolled, so a run's zeny has a spread: the
		// expected value of the kills the bots made, and its standard
		// error, from the simulator's drop rates.
		// Kills count by the names players see, which some monsters
		// share, such as two kinds of Goblin: each name's kills share
		// out over its lines by count.
		var expZeny, variance int64
		for shown, n := range got.byMonster {
			var weight, zeny, vari int64
			for _, l := range f.hunt.Lines {
				m, ok := monsters[l.Monster]
				mon, known := sim.Monster(l.Monster)
				if !ok || !known || m.Name != shown {
					continue
				}
				if alone, _ := sim.LeftAlone(f.level, mon); alone {
					continue
				}
				k, err := sim.KillOf(f.level, mon)
				if err != nil {
					t.Fatal(err)
				}
				weight += int64(l.Count)
				zeny += int64(l.Count) * k.Zeny
				vari += int64(l.Count) * k.ZenyVariance
			}
			if weight > 0 {
				expZeny += n * zeny / weight / 10000
				variance += n * vari / weight
			}
		}
		se := per(isqrt64(variance))
		for i, l := range f.hunt.Lines {
			t.Logf("  %s x%d, respawn %d ms: the simulator %d.%03d kills an hour", l.Monster, l.Count, l.RespawnMs, r.Lines[i]/1000, r.Lines[i]%1000)
		}
		for shown, n := range got.byMonster {
			t.Logf("  the bots killed %d of %s", n, shown)
		}
		t.Logf("  zeny of the kills made: expected %d/h (%d%% of the simulator's), standard error %d/h", per(expZeny), economy.Ratio(per(expZeny), r.Zeny), se)
		fmt.Fprintf(&report, "  zeny expected of the kills made %d/h (%d%%), the run's standard error %d/h\n", per(expZeny), economy.Ratio(per(expZeny), r.Zeny), se)
		// The run's zeny passes within the tolerance, or within two
		// standard errors of its kills' expected value when the drops'
		// spread is wider than the tolerance.
		if !economy.Within(zeny, r.Zeny, tol) && (abs64(zeny-per(expZeny)) > 2*se || !economy.Within(per(expZeny), r.Zeny, tol)) {
			t.Errorf("%s: the bots' zeny %d/h is %d%% of the simulator's %d/h", f.id, zeny, economy.Ratio(zeny, r.Zeny), r.Zeny)
		}
	}
	if p := os.Getenv("WATHENA_ECONOMY_REPORT"); p != "" {
		if err := os.WriteFile(p, []byte(report.String()), 0o600); err != nil {
			t.Error(err)
		}
	}
	srv.CheckLogs()
}

// m0 is the model of a level, which the test already made.
func m0(sim *economy.World, level int) *economy.Model {
	m, _ := sim.Model(level)
	return m
}

// economySim is the simulator on core's renewal data, as wactl world new
// uses it, and the monsters by name.
func economySim(t *testing.T) (*economy.World, map[string]gamedata.Monster) {
	t.Helper()
	cfg, err := economy.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	d, err := gamedata.LoadItemData(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	tables, prog, err := gamedata.LoadProgression(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _, err := gamedata.StartCharacter(reference.Renewal, "all_1")
	if err != nil {
		t.Fatal(err)
	}
	mons, err := gamedata.LoadMonsters(reference.Renewal, tables, prog)
	if err != nil {
		t.Fatal(err)
	}
	if err := gamedata.ExtractSkills(reference.Renewal, mons); err != nil {
		t.Fatal(err)
	}
	byName := map[string]gamedata.Monster{}
	for _, m := range mons {
		byName[m.AegisName] = m
	}
	return gamedata.EconomyWorld(cfg, item.New(d), jobs, tables, nil, mons), byName
}

// huntField is a sampled field: its band, the model level, its hunt, and
// the names players see of the monsters the bots hunt.
type huntField struct {
	id    string
	band  [2]int
	level int
	hunt  economy.Field
	// names maps the names players see to the monsters' names.
	names map[string]string
	// maxFight is the longest fight the model player takes on, and
	// leftAlone the names of the monsters it leaves alone.
	maxFight  time.Duration
	leftAlone map[string]bool
	// grid is the field's walkable cells, by row.
	grid fieldGrid
}

// fieldGrid is a map's walkable cells.
type fieldGrid struct {
	w, h int
	walk []bool
}

func (g fieldGrid) ok(x, y int) bool {
	return x >= 0 && y >= 0 && x < g.w && y < g.h && g.walk[y*g.w+x]
}

// maxSteps is the longest walk the server's path search finds, as
// rAthena's MAX_WALKPATH, less a margin for a monster that moves.
const maxSteps = 28

// steps returns the walking steps from a cell to every cell within
// maxSteps, by 8 directions with rAthena's rule for diagonals, as
// route does; cells out of reach are -1.
func (g fieldGrid) steps(x0, y0 int) map[[2]int]int {
	out := map[[2]int]int{{x0, y0}: 0}
	q := [][2]int{{x0, y0}}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		d := out[c]
		if d >= maxSteps {
			continue
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				n := [2]int{c[0] + dx, c[1] + dy}
				if dx == 0 && dy == 0 || !g.ok(n[0], n[1]) || dx != 0 && dy != 0 && (!g.ok(c[0]+dx, c[1]) || !g.ok(c[0], c[1]+dy)) {
					continue
				}
				if _, seen := out[n]; !seen {
					out[n] = d + 1
					q = append(q, n)
				}
			}
		}
	}
	return out
}

// economyFields picks a field for each of economyBands from the world's
// stored spawns: the one whose lowest level is closest.
func economyFields(t *testing.T, srv *Server, sim *economy.World, monsters map[string]gamedata.Monster) []huntField {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := srv.DB.Query(ctx, `SELECT id, min_level, max_level FROM world_maps WHERE kind = 'field' ORDER BY ord`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id     string
		lo, hi int
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.lo, &r.hi); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
	}
	rows.Close()
	var out []huntField
	for _, want := range economyBands {
		best := -1
		for i, r := range all {
			if best < 0 || abs(r.lo-want) < abs(all[best].lo-want) {
				best = i
			}
		}
		r := all[best]
		if slices.ContainsFunc(out, func(f huntField) bool { return f.id == r.id }) {
			continue
		}
		f := huntField{id: r.id, band: [2]int{r.lo, r.hi}, level: economy.Middle([2]int{r.lo, r.hi}), names: map[string]string{}, leftAlone: map[string]bool{}}
		lines, err := srv.DB.Query(ctx, `SELECT monster, count, respawn_ms, role FROM world_spawns WHERE map_id = $1 ORDER BY ord`, r.id)
		if err != nil {
			t.Fatal(err)
		}
		for lines.Next() {
			var name, role string
			var count, respawn int
			if err := lines.Scan(&name, &count, &respawn, &role); err != nil {
				t.Fatal(err)
			}
			m, ok := monsters[name]
			if role == "mini_boss" || role == "field_boss" || !ok {
				// Bosses and the sample mods' monsters, which the
				// simulator of core's data does not know.
				continue
			}
			f.hunt.Lines = append(f.hunt.Lines, economy.Line{Monster: name, Count: count, RespawnMs: respawn})
			f.names[m.Name] = name
			em, _ := sim.Monster(name)
			if alone, err := sim.LeftAlone(f.level, em); err != nil {
				t.Fatal(err)
			} else if alone {
				// The model player leaves it alone, and so does the bot.
				f.leftAlone[m.Name] = true
			}
		}
		lines.Close()
		f.grid = readGrid(t, srv, r.id)
		for _, w := range f.grid.walk {
			if w {
				f.hunt.Walkable++
			}
		}
		out = append(out, f)
	}
	return out
}

// readGrid reads a generated map's walkable cells.
func readGrid(t *testing.T, srv *Server, id string) fieldGrid {
	t.Helper()
	w, h, flags, err := wmap.DecodeCells(get(t, srv, "/gen/maps/"+url.PathEscape(id)+".wmap"))
	if err != nil {
		t.Fatal(err)
	}
	g := fieldGrid{w: w, h: h, walk: make([]bool, w*h)}
	for i := range g.walk {
		g.walk[i] = flags[i]&path.Walkable != 0
	}
	return g
}

// ahead returns the cell the path search reaches from a cell that lies
// farthest along a heading, at least 8 steps away, and whether there is
// one. A zero heading takes the farthest cell.
func (g fieldGrid) ahead(x, y int, heading [2]int) (int, int, bool) {
	best, bestScore := [2]int{}, -1
	for c, d := range g.steps(x, y) {
		if d < 8 {
			continue
		}
		score := (c[0]-x)*heading[0] + (c[1]-y)*heading[1]
		if heading == [2]int{} {
			score = d
		}
		if score > bestScore || score == bestScore && (c[1] < best[1] || c[1] == best[1] && c[0] < best[0]) {
			best, bestScore = c, score
		}
	}
	return best[0], best[1], bestScore > 0
}

// cells lists a grid's walkable cells.
func (g fieldGrid) cells() [][2]int {
	var out [][2]int
	for y := range g.h {
		for x := range g.w {
			if g.ok(x, y) {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// huntTotals are what bots got while hunting.
type huntTotals struct {
	ms              int64
	kills, deaths   int64
	base, job, zeny int64
	potions         int64
	// The time of the kills' parts: from the attack to the first hit,
	// from it to the kill, and the loot, in milliseconds, and the items
	// that fell near kills and those picked up, with the value of those
	// that fell.
	approachMs, fightMs, lootMs int64
	fell, picked, fellValue     int64
	// lostMs is the time spent on targets the bot gave up, and idleMs
	// the time with nothing in sight.
	lostMs, idleMs int64
	// byMonster counts the kills by the names players see.
	byMonster map[string]int64
}

func (h *huntTotals) add(o huntTotals) {
	if h.byMonster == nil {
		h.byMonster = map[string]int64{}
	}
	for k, n := range o.byMonster {
		h.byMonster[k] += n
	}
	h.lostMs += o.lostMs
	h.idleMs += o.idleMs
	h.approachMs += o.approachMs
	h.fightMs += o.fightMs
	h.lootMs += o.lootMs
	h.fell += o.fell
	h.picked += o.picked
	h.fellValue += o.fellValue
	h.ms += o.ms
	h.kills += o.kills
	h.deaths += o.deaths
	h.base += o.base
	h.job += o.job
	h.zeny += o.zeny
	h.potions += o.potions
}

// segmentFor is how long a bot hunts before its character goes back to
// the model: short where levels come fast.
func segmentFor(level int) time.Duration {
	if level < 30 {
		return 30 * time.Second
	}
	return 2 * time.Minute
}

// runHunter makes a bot of the model player and hunts a field for d.
func runHunter(t *testing.T, srv *Server, name string, f huntField, m *economy.Model, cells [][2]int,
	monsters map[string]gamedata.Monster, d time.Duration, seed uint64) huntTotals {
	t.Helper()
	rnd := rand.New(rand.NewPCG(seed, 46)) // #nosec G404 -- a test's bot.
	// Join once so the character exists, then make it the model.
	b := dialBot(t, srv, name)
	if err := b.WaitState(context.Background(), func(_ botclient.View, g botclient.Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_BASE_LEVEL] > 0
	}); err != nil {
		t.Fatal(err)
	}
	first := b.Game().Stats
	_ = b.Close()
	waitSaved(t, srv, name, first)
	var total huntTotals
	for left := d; left > 0; {
		start := cells[rnd.IntN(len(cells))]
		resetHunter(t, srv, name, m, f.id, start)
		b = dialBot(t, srv, name)
		seg := min(segmentFor(m.Level), left)
		got := hunt(t, b, f, m, seg, rnd)
		total.add(got)
		left -= seg
		lastSeen := b.Game().Stats
		_ = b.Close()
		waitSaved(t, srv, name, lastSeen)
	}
	return total
}

// dialBot joins as a recording bot, trying three times, since a busy
// machine may be slow to answer.
func dialBot(t *testing.T, srv *Server, name string) *botclient.Bot {
	t.Helper()
	var err error
	for range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var b *botclient.Bot
		b, err = botclient.Dial(ctx, srv.WS, []byte(name), botclient.Options{Record: true})
		cancel()
		if err == nil {
			return b
		}
	}
	t.Fatalf("%s cannot join: %v", name, err)
	return nil
}

// waitSaved waits until the server saved the character after its bot
// left: its base level and exp in the database are the last the bot saw.
func waitSaved(t *testing.T, srv *Server, name string, stats map[wathenav1.StatId]int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		var level, exp int64
		var online bool
		err := srv.DB.QueryRow(ctx, `SELECT base_level, base_exp, saved_at >= login_at FROM characters WHERE name = $1`, name).Scan(&level, &exp, &online)
		if err == nil && level == stats[wathenav1.StatId_STAT_ID_BASE_LEVEL] && exp == stats[wathenav1.StatId_STAT_ID_BASE_EXP] && online {
			// The save that matches may be one from before the logout;
			// a moment more lets the logout's save land.
			time.Sleep(500 * time.Millisecond)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s was not saved after its bot left", name)
}

// resetHunter writes the model player in the database while the
// character is offline: its job, levels, stats, full HP, gear and
// potions, on a cell of the field.
func resetHunter(t *testing.T, srv *Server, name string, m *economy.Model, field string, at [2]int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var id int64
	if err := srv.DB.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	st := m.Stats
	if _, err := srv.DB.Exec(ctx, `UPDATE characters SET job = $2, base_level = $3, job_level = $4, base_exp = 0, job_exp = 0,
		str = $5, agi = $6, vit = $7, int = $8, dex = $9, luk = $10, status_points = 0, skill_points = 0, hp = 0,
		map = $11, x = $12, y = $13 WHERE id = $1`,
		id, m.Job, m.Level, m.JobLevel, st.Str, st.Agi, st.Vit, st.Int, st.Dex, st.Luk, field, at[0], at[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(ctx, `DELETE FROM inventory WHERE character_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	for i, it := range m.Gear {
		if _, err := srv.DB.Exec(ctx, `INSERT INTO inventory (character_id, slot, item_id, amount, equip, refine) VALUES ($1, $2, $3, 1, $4, $5)`,
			id, i, it, m.Positions[i], m.Refines[i]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.DB.Exec(ctx, `INSERT INTO inventory (character_id, slot, item_id, amount) VALUES ($1, $2, $3, $4)`,
		id, len(m.Gear), m.Potion, huntPotions); err != nil {
		t.Fatal(err)
	}
}

// huntPotions is how many potions a bot starts each segment with: few
// enough that the weight stays under half, where natural regen stops.
const huntPotions = 50

// hunt plays one segment: the bot hunts the field for d and returns
// what it got. A bot that dies ends its segment.
func hunt(t *testing.T, b *botclient.Bot, f huntField, m *economy.Model, d time.Duration, rnd *rand.Rand) huntTotals {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d+time.Minute)
	defer cancel()
	if err := b.WaitState(ctx, func(v botclient.View, g botclient.Game) bool {
		return v.Map == f.id && g.Stats[wathenav1.StatId_STAT_ID_NEXT_BASE_EXP] > 0
	}); err != nil {
		t.Errorf("%s did not enter %s: %v", b.Name, f.id, err)
		return huntTotals{}
	}
	out := huntTotals{byMonster: map[string]int64{}}
	start := time.Now()
	end := start.Add(d)
	stats := b.Game().Stats
	potionsAt := b.Game().Count(m.Potion)
	ignore := map[uint32]bool{}
	skip := maps.Clone(f.leftAlone)
	heading, idle := [2]int{1, 0}, 0
	maxFight := f.maxFight
	mark := b.Mark()
	countExp := func() {
		now := b.Game().Stats
		out.base += gained(stats, now, wathenav1.StatId_STAT_ID_BASE_LEVEL, wathenav1.StatId_STAT_ID_BASE_EXP, wathenav1.StatId_STAT_ID_NEXT_BASE_EXP)
		out.job += gained(stats, now, wathenav1.StatId_STAT_ID_JOB_LEVEL, wathenav1.StatId_STAT_ID_JOB_EXP, wathenav1.StatId_STAT_ID_NEXT_JOB_EXP)
		stats = now
	}
	for time.Now().Before(end) {
		v := b.View()
		if v.Map != f.id || v.HP <= 0 && v.MaxHP > 0 {
			out.deaths++
			break
		}
		if v.MaxHP > 0 && v.HP*2 < v.MaxHP {
			if slot := b.Game().SlotOf(m.Potion); slot >= 0 {
				_ = b.UseItem(ctx, slot)
				time.Sleep(150 * time.Millisecond)
				continue
			}
		}
		target, ok := nearestHuntable(v, f.grid, f.names, ignore, skip)
		if !ok {
			// Nothing in sight: walk on in one direction to a cell the
			// path search reaches, and turn now and then or when no cell
			// lies that way.
			idle++
			if idle%6 == 0 {
				heading = [2]int{rnd.IntN(3) - 1, rnd.IntN(3) - 1}
			}
			x, y, ok := f.grid.ahead(v.X, v.Y, heading)
			for tries := 0; !ok && tries < 8; tries++ {
				heading = [2]int{rnd.IntN(3) - 1, rnd.IntN(3) - 1}
				x, y, ok = f.grid.ahead(v.X, v.Y, heading)
			}
			_ = b.WalkTo(ctx, x, y)
			time.Sleep(time.Second)
			out.idleMs += 1000
			continue
		}
		idle = 0
		mark = b.Mark()
		sent := time.Now()
		if err := b.Attack(ctx, target.ID, true); err != nil {
			break
		}
		killed, at, firstHit := waitKill(ctx, b, target, m.Potion, &mark, maxFight, end.Add(30*time.Second))
		if !killed {
			ignore[target.ID] = true
			if !firstHit.IsZero() && time.Since(firstHit) >= maxFight {
				// A fight longer than the longest the player takes on:
				// it leaves the monster's kind alone, as the simulator
				// does.
				skip[target.Name] = true
			}
			out.lostMs += time.Since(sent).Milliseconds()
			continue
		}
		dead := time.Now()
		out.kills++
		out.byMonster[target.Name]++
		if !firstHit.IsZero() {
			out.approachMs += firstHit.Sub(sent).Milliseconds()
			out.fightMs += dead.Sub(firstHit).Milliseconds()
		}
		l := loot(ctx, b, at, &mark, m.Potion)
		out.zeny += l.value
		out.fell += l.fell
		out.picked += l.picked
		out.fellValue += l.fellValue
		out.lootMs += time.Since(dead).Milliseconds()
		countExp()
	}
	countExp()
	out.ms = time.Since(start).Milliseconds()
	out.potions = int64(max(potionsAt-b.Game().Count(m.Potion), 0))
	return out
}

// gained is the exp gained between two snapshots, across one level up:
// the rest of the old level, then the new exp.
func gained(before, after map[wathenav1.StatId]int64, levelID, expID, nextID wathenav1.StatId) int64 {
	if after[levelID] > before[levelID] {
		return max(before[nextID]-before[expID], 0) + after[expID]
	}
	return max(after[expID]-before[expID], 0)
}

// nearestHuntable is the nearest monster of the hunted names by the
// walk to a cell next to it, as a player picks a monster it can reach.
func nearestHuntable(v botclient.View, g fieldGrid, names map[string]string, ignore map[uint32]bool, skip map[string]bool) (botclient.Entity, bool) {
	var best botclient.Entity
	bestD := -1
	var dist map[[2]int]int
	for _, e := range v.Entities {
		if e.Kind != wathenav1.EntityKind_ENTITY_KIND_MONSTER || names[e.Name] == "" || ignore[e.ID] || skip[e.Name] {
			continue
		}
		if dist == nil {
			dist = g.steps(v.X, v.Y)
		}
		d := -1
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if s, ok := dist[[2]int{e.DestX + dx, e.DestY + dy}]; ok && (d < 0 || s < d) {
					d = s
				}
			}
		}
		if d >= 0 && (bestD < 0 || d < bestD) {
			best, bestD = e, d
		}
	}
	return best, bestD >= 0
}

// waitKill waits until the target dies, and returns where it was and
// the time of the first hit. Meanwhile the bot drinks a potion below
// half HP, and sends the attack again when it has not hit for a moment,
// as a player clicks again on a monster that walked away. It gives up
// after 10 seconds without a hit, a fight longer than maxFight, or at
// the hard end.
func waitKill(ctx context.Context, b *botclient.Bot, target botclient.Entity, potion uint32, mark *int, maxFight time.Duration, hardEnd time.Time) (bool, [2]int, time.Time) {
	at := [2]int{target.X, target.Y}
	var first time.Time
	lastHit, lastSent, lastPotion := time.Now(), time.Now(), time.Time{}
	for time.Now().Before(hardEnd) {
		evs, next := b.Events(*mark)
		*mark = next
		for _, e := range evs {
			if dm := e.GetDamage(); dm != nil && dm.GetTarget() == target.ID {
				lastHit = time.Now()
				if first.IsZero() {
					first = lastHit
				}
			}
			if v := e.GetVanish(); v != nil && v.GetId() == target.ID {
				return v.GetReason() == wathenav1.VanishReason_VANISH_REASON_DIED, at, first
			}
		}
		v := b.View()
		for _, e := range v.Entities {
			if e.ID == target.ID {
				at = [2]int{e.DestX, e.DestY}
			}
		}
		if v.HP <= 0 && v.MaxHP > 0 {
			return false, at, first
		}
		now := time.Now()
		if v.MaxHP > 0 && v.HP*2 < v.MaxHP && now.Sub(lastPotion) > 300*time.Millisecond {
			if slot := b.Game().SlotOf(potion); slot >= 0 {
				_ = b.UseItem(ctx, slot)
				lastPotion = now
			}
		}
		if now.Sub(lastHit) > 1500*time.Millisecond && now.Sub(lastSent) > 1500*time.Millisecond {
			_ = b.Attack(ctx, target.ID, true)
			lastSent = now
		}
		if now.Sub(lastHit) > 10*time.Second || !first.IsZero() && now.Sub(first) > maxFight {
			return false, at, first
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false, at, first
}

// looted is what a kill's loot gave.
type looted struct {
	value, fell, picked, fellValue int64
}

// loot picks up the items that fall within 2 cells of a kill in the
// next moment, and returns their NPC sell value.
func loot(ctx context.Context, b *botclient.Bot, at [2]int, mark *int, potion uint32) looted {
	var out looted
	time.Sleep(lootWait)
	for _, e := range b.View().Entities {
		if e.Kind != wathenav1.EntityKind_ENTITY_KIND_ITEM || abs(e.X-at[0]) > 2 || abs(e.Y-at[1]) > 2 {
			continue
		}
		out.fell++
		out.fellValue += int64(e.Amount) * sellPrice(e.ItemID)
		*mark = b.Mark()
		if err := b.PickUp(ctx, e.ID); err != nil {
			return out
		}
		deadline := time.Now().Add(3 * time.Second)
	wait:
		for time.Now().Before(deadline) {
			evs, next := b.Events(*mark)
			*mark = next
			for _, ev := range evs {
				if a := ev.GetInventoryAdd(); a != nil && a.GetItemId() == e.ItemID && a.GetItemId() != potion {
					out.value += int64(a.GetAmount()) * sellPrice(a.GetItemId())
					out.picked++
					break wait
				}
				if f := ev.GetPickUpFailed(); f != nil && f.GetItem() == e.ID {
					break wait
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return out
}

// lootWait is how long a bot waits after a kill for its drops to fall.
const lootWait = 300 * time.Millisecond

var (
	pricesOnce sync.Once
	prices     *item.Table
)

// sellPrice is what an NPC pays for an item, 0 for an item NPCs do not
// buy.
func sellPrice(id uint32) int64 {
	pricesOnce.Do(func() {
		d, err := gamedata.LoadItemData(reference.Renewal)
		if err == nil {
			prices = item.New(d)
		}
	})
	if prices == nil {
		return 0
	}
	d, ok := prices.Lookup(id)
	if !ok || d.NoSell {
		return 0
	}
	return int64(d.Sell)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// isqrt64 is the integer square root.
func isqrt64(n int64) int64 {
	if n <= 0 {
		return 0
	}
	x := n
	for y := (x + 1) / 2; y < x; y = (x + n/x) / 2 {
		x = y
	}
	return x
}
