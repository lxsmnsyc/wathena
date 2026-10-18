package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/reference/rathena"
	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// genMaps reads a procedural world's generated maps from the server's
// HTTP address, as the web client does: each map's cells from its WMAP
// and its places from its info. It keeps what it read.
type genMaps struct {
	base string

	mu    sync.Mutex
	grids map[string]*path.Grid
	infos map[string]*mapInfo
	// links are the warps' ends by map, from the database, for portals.
	links map[string][]linkEnd
}

type mapInfo struct {
	Kind   string `json:"kind"`
	Places []struct {
		Tags []string `json:"tags"`
		X    int      `json:"x"`
		Y    int      `json:"y"`
	} `json:"places"`
}

type linkEnd struct {
	tag   string
	index int
}

// httpBase turns the WebSocket URL into the server's HTTP address.
func httpBase(ws string) (string, error) {
	u, err := url.Parse(ws)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return "", fmt.Errorf("the URL %q is not a WebSocket URL", ws)
	}
	u.Path, u.RawQuery = "", ""
	return strings.TrimSuffix(u.String(), "/"), nil
}

func newGenMaps(ws string) (*genMaps, error) {
	base, err := httpBase(ws)
	if err != nil {
		return nil, err
	}
	return &genMaps{base: base, grids: map[string]*path.Grid{}, infos: map[string]*mapInfo{}}, nil
}

func (m *genMaps) get(ctx context.Context, id, ext string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.base+"/gen/maps/"+url.PathEscape(id)+ext, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req) // #nosec G704 -- the server the operator named.
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s%s: %s", id, ext, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, wmap.MaxBytes+1))
}

// grid returns a generated map's cells, or nil for a map that is not
// generated or cannot load.
func (m *genMaps) grid(id string) *path.Grid {
	if !strings.HasPrefix(id, "gen:") {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if g, ok := m.grids[id]; ok {
		return g
	}
	var g *path.Grid
	b, err := m.get(context.Background(), id, ".wmap")
	if err == nil {
		var w, h int
		var flags []uint8
		if w, h, flags, err = wmap.DecodeCells(b); err == nil {
			g = &path.Grid{Width: w, Height: h, Cells: make([]uint8, len(flags))}
			for i, f := range flags {
				g.Cells[i] = f & (path.Walkable | path.Shootable)
			}
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "wabots:", err)
	}
	m.grids[id] = g
	return g
}

func (m *genMaps) info(id string) *mapInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if in, ok := m.infos[id]; ok {
		return in
	}
	var in *mapInfo
	b, err := m.get(context.Background(), id, ".json")
	if err == nil {
		in = &mapInfo{}
		if err = json.Unmarshal(b, in); err != nil {
			in = nil
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "wabots:", err)
	}
	m.infos[id] = in
	return in
}

// portals returns the cells of a generated map's warps: the places at
// the ends of the world's links on it.
func (m *genMaps) portals(id string) [][2]int {
	in := m.info(id)
	if in == nil {
		return nil
	}
	m.mu.Lock()
	ends := m.links[id]
	m.mu.Unlock()
	var out [][2]int
	for _, e := range ends {
		k := 0
		for _, p := range in.Places {
			if slices.Contains(p.Tags, e.tag) {
				if k == e.index {
					out = append(out, [2]int{p.X, p.Y})
					break
				}
				k++
			}
		}
	}
	return out
}

// reachable returns the cells reachable from a map's first warp, or
// every walkable cell of a map without warps.
func (m *genMaps) reachable(id string) [][2]int {
	g := m.grid(id)
	if g == nil {
		return nil
	}
	var out [][2]int
	ports := m.portals(id)
	if len(ports) == 0 {
		for y := range g.Height {
			for x := range g.Width {
				if !g.NoPass(x, y) {
					out = append(out, [2]int{x, y})
				}
			}
		}
		return out
	}
	seen := make([]bool, g.Width*g.Height)
	queue := [][2]int{ports[0]}
	seen[ports[0][1]*g.Width+ports[0][0]] = true
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		out = append(out, c)
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				x, y := c[0]+dx, c[1]+dy
				if g.NoPass(x, y) || seen[y*g.Width+x] {
					continue
				}
				seen[y*g.Width+x] = true
				queue = append(queue, [2]int{x, y})
			}
		}
	}
	return out
}

// worldMap is a row of world_maps.
type worldMap struct {
	id, kind string
	band     [2]int
}

// placement is where one bot starts, and as whom.
type placement struct {
	mapID string
	x, y  int
	model economy.Player
}

// placer puts the bots of a procedural scenario on their maps before
// they join, through the database, while their characters are offline.
type placer struct {
	db   *pgxpool.Pool
	maps *genMaps
	// fields is how many fields the fields scenario uses.
	fields int
}

// place makes each bot's character as its scenario wants: "fields"
// spreads the bots over fields of several bands, each as the model
// player of its field's band (strategy section 8.13), with its save
// point on its field; "towns" spreads them over the towns, as the model
// player of the highest band, so the fields they cross do not kill them.
func (p *placer) place(ctx context.Context, sc *botclient.Scenario, names []string, progress func(string)) error {
	var mode string
	if err := p.db.QueryRow(ctx, `SELECT ruleset FROM world`).Scan(&mode); err != nil {
		return fmt.Errorf("the world: %w", err)
	}
	all, err := p.load(ctx)
	if err != nil {
		return err
	}
	models, err := newModels(reference.Mode(mode))
	if err != nil {
		return err
	}
	var targets []worldMap
	top := 1
	for _, m := range all {
		top = max(top, m.band[1])
	}
	switch sc.Place {
	case "fields":
		var fields []worldMap
		for _, m := range all {
			if m.kind == "field" {
				fields = append(fields, m)
			}
		}
		targets = spreadBands(fields, p.fields)
	case "towns":
		for _, m := range all {
			if m.kind == "town" {
				targets = append(targets, m)
			}
		}
	default:
		return fmt.Errorf("unknown place %q: want fields or towns", sc.Place)
	}
	if len(targets) == 0 {
		return fmt.Errorf("the world has no maps for %q", sc.Place)
	}
	rnd := rand.New(rand.NewPCG(sc.Seed, 1)) // #nosec G404 -- bot placement, not a secret.
	cells := map[string][][2]int{}
	out := make([]placement, len(names))
	for i := range names {
		m := targets[i%len(targets)]
		c, ok := cells[m.id]
		if !ok {
			c = p.maps.reachable(m.id)
			cells[m.id] = c
		}
		if len(c) == 0 {
			return fmt.Errorf("%s: no walkable cells", m.id)
		}
		level := economy.Middle(m.band)
		if sc.Place == "towns" {
			level = top
		}
		model, err := models.at(level)
		if err != nil {
			return err
		}
		cell := c[rnd.IntN(len(c))]
		out[i] = placement{mapID: m.id, x: cell[0], y: cell[1], model: model}
	}
	var desc []string
	for _, m := range targets {
		desc = append(desc, fmt.Sprintf("%s (%d-%d)", m.id, m.band[0], m.band[1]))
	}
	progress(fmt.Sprintf("placing %d bots on %s", len(names), strings.Join(desc, ", ")))
	if err := p.makeCharacters(ctx, sc, names, progress); err != nil {
		return err
	}
	for i, name := range names {
		if err := p.write(ctx, name, out[i]); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// load reads the world's maps and links.
func (p *placer) load(ctx context.Context) ([]worldMap, error) {
	rows, err := p.db.Query(ctx, `SELECT id, kind, min_level, max_level FROM world_maps ORDER BY ord`)
	if err != nil {
		return nil, err
	}
	all, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (worldMap, error) {
		var m worldMap
		err := r.Scan(&m.id, &m.kind, &m.band[0], &m.band[1])
		return m, err
	})
	if err != nil {
		return nil, err
	}
	rows, err = p.db.Query(ctx, `SELECT a_map, a_tag, a_index, b_map, b_tag, b_index FROM world_links`)
	if err != nil {
		return nil, err
	}
	links := map[string][]linkEnd{}
	var am, at, bm, bt string
	var ai, bi int
	_, err = pgx.ForEachRow(rows, []any{&am, &at, &ai, &bm, &bt, &bi}, func() error {
		links[am] = append(links[am], linkEnd{at, ai})
		links[bm] = append(links[bm], linkEnd{bt, bi})
		return nil
	})
	if err != nil {
		return nil, err
	}
	p.maps.mu.Lock()
	p.maps.links = links
	p.maps.mu.Unlock()
	return all, nil
}

// spreadBands picks n fields with lowest levels spread evenly from the
// lowest band to the highest.
func spreadBands(fields []worldMap, n int) []worldMap {
	slices.SortStableFunc(fields, func(a, b worldMap) int { return a.band[0] - b.band[0] })
	if n <= 0 || n >= len(fields) {
		return fields
	}
	out := make([]worldMap, 0, n)
	for i := range n {
		out = append(out, fields[i*(len(fields)-1)/max(n-1, 1)])
	}
	return out
}

// makeCharacters logs in once each bot whose character does not exist
// yet, so the server makes it, and waits until the server saved it at
// logout.
func (p *placer) makeCharacters(ctx context.Context, sc *botclient.Scenario, names []string, progress func(string)) error {
	rows, err := p.db.Query(ctx, `SELECT name FROM characters WHERE name = ANY($1)`, names)
	if err != nil {
		return err
	}
	have, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	var missing []string
	for _, n := range names {
		if !slices.Contains(have, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	progress(fmt.Sprintf("making %d characters", len(missing)))
	ramp := sc.Ramp
	if ramp <= 0 {
		ramp = 50
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	start := time.Now()
	for i, name := range missing {
		select {
		case <-time.After(time.Until(start.Add(time.Duration(float64(i) / ramp * float64(time.Second))))):
		case <-ctx.Done():
			return ctx.Err()
		}
		wg.Go(func() {
			dial, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			b, err := botclient.Dial(dial, sc.URL, []byte(name), botclient.Options{})
			if err == nil {
				err = b.Close()
			}
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", name, err))
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	// The logout save comes after the connection closed. A save that
	// lands after write is refused, since write raises save_version, so
	// a short wait is enough: the server's clock and the database's may
	// differ, so saved_at and login_at cannot be compared.
	deadline := time.Now().Add(time.Minute)
	for {
		var waiting int
		err := p.db.QueryRow(ctx, `SELECT count(*) FROM characters WHERE name = ANY($1) AND saved_at IS NULL`, missing).Scan(&waiting)
		if err != nil {
			return err
		}
		if waiting == 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d new characters were not saved at logout", waiting)
		}
		time.Sleep(time.Second)
	}
	time.Sleep(3 * time.Second)
	return nil
}

// write sets a character's job, levels, stats, items, map, cell and save
// point. It raises save_version, so a save of an older session cannot
// overwrite it.
func (p *placer) write(ctx context.Context, name string, pl placement) error {
	return pgx.BeginFunc(ctx, p.db, func(tx pgx.Tx) error {
		var id int64
		if err := tx.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, name).Scan(&id); err != nil {
			return err
		}
		m, st := pl.model, pl.model.Stats
		if _, err := tx.Exec(ctx, `UPDATE characters SET job = $2, base_level = $3, job_level = $4, base_exp = 0, job_exp = 0,
			str = $5, agi = $6, vit = $7, int = $8, dex = $9, luk = $10, status_points = 0, skill_points = 0, hp = 0,
			map = $11, x = $12, y = $13, save_map = $11, save_x = $12, save_y = $13, save_version = save_version + 1 WHERE id = $1`,
			id, m.Job, m.Level, m.JobLevel, st.Str, st.Agi, st.Vit, st.Int, st.Dex, st.Luk, pl.mapID, pl.x, pl.y); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM inventory WHERE character_id = $1`, id); err != nil {
			return err
		}
		for i, it := range m.Gear {
			if _, err := tx.Exec(ctx, `INSERT INTO inventory (character_id, slot, item_id, amount, equip, refine) VALUES ($1, $2, $3, 1, $4, $5)`,
				id, i, it, m.Positions[i], m.Refines[i]); err != nil {
				return err
			}
		}
		if m.Potion != 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO inventory (character_id, slot, item_id, amount) VALUES ($1, $2, $3, 50)`,
				id, len(m.Gear), m.Potion); err != nil {
				return err
			}
		}
		return nil
	})
}

// models makes the economy simulator's model players (strategy section
// 8.13) from the reference data of a mode.
type models struct {
	cfg   *economy.Config
	items *item.Table
	sim   *economy.World
	cache map[int]economy.Player
}

func newModels(mode reference.Mode) (*models, error) {
	// The reference data built into the binary, so wabots runs outside
	// the repository, as wactl and waserver do.
	reference.UseFS(rathena.Files)
	cfg, err := economy.LoadConfig()
	if err != nil {
		return nil, err
	}
	d, err := gamedata.LoadItemData(mode)
	if err != nil {
		return nil, err
	}
	tables, _, err := gamedata.LoadProgression(mode)
	if err != nil {
		return nil, err
	}
	jobs, _, err := gamedata.StartCharacter(mode, "all_1")
	if err != nil {
		return nil, err
	}
	items := item.New(d)
	return &models{cfg: cfg, items: items, sim: gamedata.EconomyWorld(cfg, items, jobs, tables, nil, nil), cache: map[int]economy.Player{}}, nil
}

func (m *models) at(level int) (economy.Player, error) {
	if p, ok := m.cache[level]; ok {
		return p, nil
	}
	mod, err := m.sim.Model(level)
	if err != nil {
		return economy.Player{}, err
	}
	m.cache[level] = mod.Player
	return mod.Player, nil
}
