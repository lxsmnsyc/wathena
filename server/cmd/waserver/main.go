// Command waserver runs a game server in one process: it serves an asset
// build over HTTP, the account service's pages under /account/, and a
// world of zones over WebSocket, one zone per map.
//
//	waserver --assets <build> --map prontera --maps prontera,prt_fild08,prt_in
//
// --maps lists the maps to run, or is "all" for every map in the asset
// build. Warps between running maps work. --map is where new characters
// start, and must be one of them.
//
// With accounts, the default, waserver needs two PostgreSQL URLs, from
// environment variables so the passwords do not show in process lists:
//   - WATHENA_DATABASE_URL (or --database) connects as the game role.
//     Characters are loaded and saved there, and game tickets are used
//     up there.
//   - WATHENA_ACCOUNT_DATABASE_URL connects as the account role, which
//     alone can read password hashes and sessions.
//
// Two-factor login needs WATHENA_ACCOUNT_SECRET_KEY: 32 random bytes in
// base64, which encrypt TOTP secrets in the database. After a key
// change, WATHENA_ACCOUNT_OLD_SECRET_KEYS lists the older keys,
// comma-separated. Passkeys need --account-url with a domain name.
//
// waserver refuses to start while migrations are pending.
//
// --dev replaces accounts with development sessions: any valid name is a
// session token, and plays the character of that name. It only listens
// on a loopback address. With --dev, the database is optional, and
// characters only live in memory without it.
//
// --debug-addr serves the server's measurements and Go's profiles on a
// loopback address, such as 127.0.0.1:8301 (server/README.md, "Load
// tests and metrics"). --profile-dir is where a CPU profile is written
// when a zone is over the tick budget. waserver prints the measurements
// when it stops.
//
// --start-items gives every new character items, such as 501:10,512:20
// for 10 Red Potions and 20 Apples. It is for development only.
package main

import (
	"context"
	crand "crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/serve"
	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "waserver:", err)
		os.Exit(1)
	}
}

func run() error {
	assets := flag.String("assets", "", "asset build folder")
	mapName := flag.String("map", "prontera", "map where new characters start, such as prontera or placeholder/map")
	mapList := flag.String("maps", "", "maps to run, comma-separated, or all for every map in the asset build; empty runs --map only")
	addr := flag.String("addr", "127.0.0.1:8300", "address to listen on")
	dev := flag.Bool("dev", false, "accept any name as a session token, for development only; needs a loopback --addr")
	origins := flag.String("origins", "http://localhost:5173,http://127.0.0.1:5173", "trusted web client origins, as scheme://host[:port], comma-separated. They may open the WebSocket and fetch game tickets")
	accountURL := flag.String("account-url", "", "the account service's public URL, such as https://play.example.com. Its scheme decides Secure cookies. Empty takes the origin from each request")
	playURL := flag.String("play-url", "", "the web client URL that Play opens when the account page was not opened from a client")
	proxies := flag.String("trusted-proxies", "", "proxy addresses or CIDR blocks whose X-Forwarded-For is believed, comma-separated")
	captcha := flag.String("captcha", "none", "captcha after repeated failed logins: none, or dev for a development checkbox")
	mode := flag.String("mode", "renewal", "rAthena mode whose recorded spawns to use: renewal or pre-renewal, or none")
	build := flag.String("build", "all_1", "raised stats every novice starts with: all_1, melee, critical, magic, ranged, or tank. Other builds than all_1 are for testing")
	startItems := flag.String("start-items", "", "items every new character starts with, as id:amount pairs such as 501:10,512:20, for development")
	debugAddr := flag.String("debug-addr", "", "loopback address to serve measurements and profiles on, such as 127.0.0.1:8301; empty serves none")
	profileDir := flag.String("profile-dir", "", "folder for the CPU profiles taken when a zone is over the tick budget; empty takes none")
	database := flag.String("database", "", "PostgreSQL URL to keep characters in; prefer the "+databaseEnv+" environment variable. Without either, characters only live in memory")
	flag.Parse()
	if *database == "" {
		*database = os.Getenv(databaseEnv)
	}
	accountDatabase := os.Getenv(accountDatabaseEnv)
	if *assets == "" {
		return errors.New("--assets is required")
	}
	if *dev {
		if !loopback(*addr) {
			return errors.New("--dev lets anyone play any character, so it only listens on a loopback address such as 127.0.0.1:8300")
		}
	} else if *database == "" || accountDatabase == "" {
		return errors.New("accounts need " + databaseEnv + " (the game role) and " + accountDatabaseEnv + " (the account role); use --dev for development without accounts")
	}
	if *debugAddr != "" && !loopback(*debugAddr) {
		return errors.New("--debug-addr has no access control, so it only listens on a loopback address such as 127.0.0.1:8301")
	}
	clientOrigins := splitList(*origins)
	trustedProxies, err := parseProxies(*proxies)
	if err != nil {
		return err
	}
	var captchaProvider account.Captcha
	switch *captcha {
	case "none":
		captchaProvider = account.NoCaptcha{}
	case "dev":
		captchaProvider = account.DevCaptcha{}
	default:
		return fmt.Errorf("unknown --captcha %q", *captcha)
	}
	name := assetName(*mapName)
	names, err := selectMaps(*assets, name, *mapList)
	if err != nil {
		return err
	}
	files, err := serve.New(*assets)
	if err != nil {
		return err
	}
	defer func() { _ = files.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var tables *progression.Tables
	var data *progression.Data
	if *mode != "none" {
		if tables, data, err = loadProgression(reference.Mode(*mode)); err != nil {
			return err
		}
	}
	spawns, err := loadSpawns(reference.Mode(*mode), tables, data)
	if err != nil {
		return err
	}
	statusMode := reference.Mode(*mode)
	if *mode == "none" {
		statusMode = reference.Renewal
	}
	warps, err := world.LoadWarps(statusMode)
	if err != nil {
		return err
	}
	root, err := rng.NewRoot(secret())
	if err != nil {
		return err
	}
	jobs, start, err := startCharacter(statusMode, *build)
	if err != nil {
		return err
	}
	items, err := loadItems(statusMode)
	if err != nil {
		return err
	}
	given, err := parseStartItems(*startItems)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := metrics.NewRegistry()
	reg.Log, reg.ProfileDir = log, *profileDir
	started := time.Now()
	grids, err := world.LoadGrids(*assets, names)
	if err != nil {
		return err
	}
	zones := make([]*zone.Zone, len(names))
	monsters, warpCount := 0, 0
	for i, n := range names {
		id := strings.TrimPrefix(n, "map/")
		// Each zone has its own secret stream from the world's root, so
		// no zone's randomness depends on another's (rng-v1, label
		// zone:<map ID>).
		z := zone.New(n, grids[i], root.Child("zone:"+id).SecretStream(), spawns[id])
		z.Progress, z.Status, z.Start, z.Items, z.StartItems = tables, jobs, start, items, given
		z.SetWarps(warps[n])
		reg.AddZone(z.Metrics)
		zones[i] = z
		monsters += count(spawns[id])
		warpCount += len(warps[n])
	}
	w, err := world.New(name, zones...)
	if err != nil {
		return err
	}
	gw := &gateway.Gateway{World: w, Sessions: gateway.DevSessions{}, Origins: clientOrigins, Log: log, Metrics: &reg.Conns}
	go reg.Conns.Run(ctx)
	var st *store.Store
	if *database != "" {
		pool, err := openDatabase(ctx, *database, "waserver")
		if err != nil {
			return err
		}
		defer pool.Close()
		st = store.New(pool, store.Options{Log: log})
		gw.Characters = dbCharacters{store: st, world: w}
	}
	var accounts *account.Service
	if !*dev {
		pool, err := openDatabase(ctx, accountDatabase, "waserver-account")
		if err != nil {
			return err
		}
		defer pool.Close()
		sealer, err := loadSealer()
		if err != nil {
			return err
		}
		if sealer == nil {
			log.Warn("two-factor codes are off: " + secretKeyEnv + " is not set")
		}
		accounts, err = account.New(account.Config{
			Pool: pool, PublicURL: *accountURL, ClientOrigins: clientOrigins, PlayURL: *playURL,
			NewCharacter: func(name string, look account.Look) character.Character {
				sex := uint32(0)
				if look.Male {
					sex = 1
				}
				return w.NewCharacter(name, zone.Look{Sex: sex, Hair: uint32(look.Hair), HairColor: uint32(look.HairColor)}) // #nosec G115 -- the account service keeps hair values small.
			},
			Captcha: captchaProvider, TrustedProxies: trustedProxies, Log: log,
			Sealer: sealer,
			// A logout closes the game connections of its session.
			Ended: gw,
		})
		if err != nil {
			return err
		}
		gw.Sessions = account.GameTickets{Store: st}
		go accounts.Run(ctx)
	}
	go w.Run(ctx)
	if st != nil {
		// On shutdown every zone hands its players' last saves to the
		// store, which then gets 10 seconds to write them.
		defer func() {
			<-w.Stopped()
			drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := st.Close(drain); err != nil {
				log.Error("saves lost at shutdown", "err", err)
			}
		}()
	}

	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets", files))
	mux.Handle("/ws", gw)
	if accounts != nil {
		mux.Handle(account.Prefix, accounts)
	}
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	servers := []*http.Server{srv}
	if *debugAddr != "" {
		servers = append(servers, &http.Server{Addr: *debugAddr, Handler: reg.Handler(), ReadHeaderTimeout: 10 * time.Second})
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, s := range servers {
			_ = s.Shutdown(shutdown)
		}
	}()
	if *debugAddr != "" {
		debug := servers[1]
		ln, err := net.Listen("tcp", *debugAddr)
		if err != nil {
			return err
		}
		go func() {
			if err := debug.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
				log.Error("debug server stopped", "err", err)
			}
		}()
		log.Info("measurements", "metrics", "http://"+*debugAddr+"/debug/wathena/metrics", "profiles", "http://"+*debugAddr+"/debug/pprof/")
	}
	var mem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&mem)
	log.Info("serving", "maps", len(names), "start", name, "monsters", monsters, "warps", warpCount,
		"startup", time.Since(started).Round(time.Millisecond), "heap_mb", mem.HeapAlloc>>20, "database", st != nil, "accounts", accounts != nil, "assets", "http://"+*addr+"/assets/", "websocket", "ws://"+*addr+"/ws")
	if accounts != nil {
		base := *accountURL
		if base == "" {
			base = "http://" + *addr
		}
		log.Info("account pages", "login", strings.TrimSuffix(base, "/")+account.Prefix+"login")
	} else {
		log.Warn("development sessions: any name plays the character of that name")
	}
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	fmt.Fprint(os.Stderr, "\nmeasurements since the start or the last reset:\n", reg.Snapshot().Text())
	return nil
}

// databaseEnv holds the game role's database URL when --database is not
// given. accountDatabaseEnv holds the account role's. secretKeyEnv holds
// the key that seals TOTP secrets, and oldSecretKeysEnv the keys it
// replaced.
const (
	databaseEnv        = "WATHENA_DATABASE_URL"
	accountDatabaseEnv = "WATHENA_ACCOUNT_DATABASE_URL"
	secretKeyEnv       = "WATHENA_ACCOUNT_SECRET_KEY"
	oldSecretKeysEnv   = "WATHENA_ACCOUNT_OLD_SECRET_KEYS"
)

// loadSealer reads the TOTP sealing keys from the environment. It
// returns nil when no key is set. Its errors never quote a key.
func loadSealer() (*account.Sealer, error) {
	text := os.Getenv(secretKeyEnv)
	if text == "" {
		if os.Getenv(oldSecretKeysEnv) != "" {
			return nil, errors.New(oldSecretKeysEnv + " is set without " + secretKeyEnv)
		}
		return nil, nil
	}
	current, err := account.ParseSecretKey(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", secretKeyEnv, err)
	}
	var old [][]byte
	for i, v := range splitList(os.Getenv(oldSecretKeysEnv)) {
		k, err := account.ParseSecretKey(v)
		if err != nil {
			return nil, fmt.Errorf("%s, key %d: %w", oldSecretKeysEnv, i+1, err)
		}
		old = append(old, k)
	}
	return account.NewSealer(current, old...)
}

// loopback reports whether a listen address only accepts connections
// from this machine.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func splitList(s string) []string {
	var out []string
	for v := range strings.SplitSeq(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseProxies reads --trusted-proxies: addresses or CIDR blocks.
func parseProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, v := range splitList(s) {
		if p, err := netip.ParsePrefix(v); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("--trusted-proxies: %q is not an address or CIDR block", v)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// openDatabase connects to PostgreSQL and checks that no migrations are
// pending. The URL is never logged, since it may hold a password.
func openDatabase(ctx context.Context, url, app string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// pgx's parse errors can quote the URL, password included.
		return nil, errors.New("the database URL is not valid")
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = app
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := store.CheckMigrations(check, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// dbCharacters loads characters from the database. Game tickets name the
// character's ID. Find is only for --dev sessions, whose token is the
// character name: the first login with a name makes an account and a
// character of that name.
type dbCharacters struct {
	store *store.Store
	world *world.World
}

func (d dbCharacters) Find(ctx context.Context, name string, look zone.Look) (int64, error) {
	c := d.world.NewCharacter(name, look)
	return d.store.DevCharacter(ctx, &c)
}

func (d dbCharacters) Load(ctx context.Context, id int64) (*character.Character, zone.Saver, error) {
	c, s, err := d.store.Load(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return c, s, nil
}

// assetName turns a map name such as prontera into its asset name,
// map/prontera. Names with a folder, such as placeholder/map, stay.
func assetName(n string) string {
	if strings.Contains(n, "/") {
		return n
	}
	return "map/" + n
}

// selectMaps returns the asset names of the maps to run, sorted: the
// --maps list, or every map in the asset build for "all". The start map
// must be one of them.
func selectMaps(dir, start, list string) ([]string, error) {
	var names []string
	switch list {
	case "":
		names = []string{start}
	case "all":
		all, err := build.List(dir)
		if err != nil {
			return nil, err
		}
		for n, e := range all {
			if e.Type == "map" {
				names = append(names, n)
			}
		}
	default:
		for _, n := range splitList(list) {
			names = append(names, assetName(n))
		}
	}
	slices.Sort(names)
	names = slices.Compact(names)
	if !slices.Contains(names, start) {
		return nil, fmt.Errorf("the start map %s is not in --maps", start)
	}
	return names, nil
}

// loadProgression reads the exp tables, level penalty, monster drops and
// item types that reference/rathena extracted from rAthena's database.
func loadProgression(mode reference.Mode) (*progression.Tables, *progression.Data, error) {
	var d progression.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/progression.json", &d); err != nil {
		return nil, nil, err
	}
	var client struct {
		Items []struct {
			ID   uint32 `json:"id"`
			Type string `json:"type"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/client_tables.json", &client); err != nil {
		return nil, nil, err
	}
	items := make(map[uint32]string, len(client.Items))
	for _, it := range client.Items {
		items[it.ID] = it.Type
	}
	return progression.New(&d, items, progression.DefaultConfig), &d, nil
}

// loadItems reads the item database that reference/rathena extracted
// from rAthena's item_db.
func loadItems(mode reference.Mode) (*item.Table, error) {
	var d item.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
		return nil, err
	}
	return item.New(&d), nil
}

// parseStartItems reads --start-items: id:amount pairs, comma-separated.
func parseStartItems(s string) ([]zone.StartItem, error) {
	var out []zone.StartItem
	for pair := range strings.SplitSeq(s, ",") {
		if pair == "" {
			continue
		}
		id, amount, ok := strings.Cut(pair, ":")
		n, err1 := strconv.ParseUint(id, 10, 32)
		a, err2 := strconv.Atoi(amount)
		if !ok || err1 != nil || err2 != nil || n == 0 || a < 1 || a > item.MaxAmount {
			return nil, fmt.Errorf("--start-items: %q is not id:amount", pair)
		}
		out = append(out, zone.StartItem{ID: uint32(n), Amount: a})
	}
	return out, nil
}

// loadSpawns reads every map's monsters, by map name such as prontera,
// from the spawns and monster stats that reference/rathena recorded from
// rAthena, until the core mod has its own. With tables, monsters also get
// their DamageTaken, exp and drops.
func loadSpawns(mode reference.Mode, tables *progression.Tables, data *progression.Data) (map[string][]zone.Spawn, error) {
	if mode == "none" {
		return nil, nil
	}
	var spawns struct {
		Maps []struct {
			Map    string `json:"map"`
			Spawns []struct {
				Mob       uint32 `json:"mob"`
				Name      string `json:"name"`
				Amount    int    `json:"amount"`
				Area      [4]int `json:"area"`
				RespawnMs int    `json:"respawn_ms"`
			} `json:"spawns"`
		} `json:"maps"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/spawns.json", &spawns); err != nil {
		return nil, err
	}
	var mobs struct {
		Mobs []struct {
			ID      uint32        `json:"id"`
			Level   int           `json:"level"`
			BaseExp int64         `json:"base_exp"`
			JobExp  int64         `json:"job_exp"`
			Status  battle.Status `json:"status"`
		} `json:"mobs"`
	}
	if err := reference.Load(mode, "mob_status", &mobs); err != nil {
		return nil, err
	}
	extra := map[uint32]*progression.MobData{}
	if data != nil {
		for i := range data.Mobs {
			extra[data.Mobs[i].ID] = &data.Mobs[i]
		}
	}
	byID := map[uint32]*battle.Status{}
	rewards := map[uint32]*progression.Mob{}
	for _, m := range mobs.Mobs {
		st := m.Status
		st.Level = m.Level
		byID[m.ID] = &st
		x := extra[m.ID]
		if tables == nil || x == nil {
			continue
		}
		st.DamageTaken = x.DamageTaken
		// mob_status.json recorded the exp at the default rates of 100%.
		r := &progression.Mob{
			ID: m.ID, Level: m.Level, MaxHP: st.MaxHP,
			BaseExp: progression.MobExp(m.BaseExp, tables.BaseExpRate),
			JobExp:  progression.MobExp(m.JobExp, tables.JobExpRate),
			Mode:    st.Mode, Class: st.Class,
		}
		r.Drops = tables.AdjustDrops(r.Boss(), x.Drops)
		rewards[m.ID] = r
	}
	out := map[string][]zone.Spawn{}
	for _, m := range spawns.Maps {
		for _, s := range m.Spawns {
			st := byID[s.Mob]
			if st == nil {
				continue
			}
			out[m.Map] = append(out[m.Map], zone.Spawn{
				Mob: s.Mob, Name: s.Name, Amount: s.Amount,
				// rAthena searches up to xs - 1 cells from the center.
				X: s.Area[0], Y: s.Area[1], W: max(s.Area[2]-1, 0), H: max(s.Area[3]-1, 0),
				// Mode bit 1 is MD_CANMOVE.
				Speed: st.Speed, CanMove: st.Mode&1 != 0,
				Status: st, RespawnMs: s.RespawnMs,
				Reward: rewards[s.Mob],
			})
		}
	}
	return out, nil
}

func count(spawns []zone.Spawn) int {
	n := 0
	for _, s := range spawns {
		n += s.Amount
	}
	return n
}

// secret returns random bytes for the zone's random streams. Each run of
// the development server gets new ones.
func secret() []byte {
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// builds are the raised stats of the recorder's PC_SAMPLE_BUILDS in the
// rAthena fork's wathena_recorder.cpp: STR, AGI, VIT, INT, DEX and LUK
// before job bonuses.
var builds = map[string]status.Stats{
	"all_1":    {Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1},
	"melee":    {Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1},
	"critical": {Str: 70, Agi: 90, Vit: 1, Int: 1, Dex: 40, Luk: 50},
	"magic":    {Str: 1, Agi: 1, Vit: 30, Int: 99, Dex: 80, Luk: 1},
	"ranged":   {Str: 30, Agi: 40, Vit: 1, Int: 1, Dex: 99, Luk: 30},
	"tank":     {Str: 60, Agi: 1, Vit: 99, Int: 1, Dex: 30, Luk: 1},
}

// startCharacter loads the mode's job tables and makes the character
// every new character starts as: a novice with no equipment and no
// skills, and the raised stats of a build. The sex comes from the
// session's look. The
// default, all_1, is a new character as rAthena makes one for recent
// clients: every stat at 1 and the level 1 status points to spend. The
// other builds keep the same status points, for testing.
func startCharacter(mode reference.Mode, build string) (*status.Tables, status.Character, error) {
	stats, ok := builds[build]
	if !ok {
		return nil, status.Character{}, fmt.Errorf("unknown build %q", build)
	}
	var d status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &d); err != nil {
		return nil, status.Character{}, err
	}
	tables, err := status.New(&d)
	if err != nil {
		return nil, status.Character{}, err
	}
	c := status.Character{Job: 0, Male: true, BaseLevel: 1, JobLevel: 1, Stats: stats}
	// Check that the tables have the job.
	if _, err := tables.Calc(c); err != nil {
		return nil, status.Character{}, err
	}
	return tables, c, nil
}
