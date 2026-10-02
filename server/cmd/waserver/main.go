// Command waserver runs every wAthena service in one process, the
// all-in-one mode of strategy section 3.9.
//
//	waserver all --config waserver.yml
//	waserver all --assets <build> --map prontera --maps all --public-url https://game.example.com
//
// It serves:
//   - the gateway and a world of zones, one zone per map, over WebSocket
//     at /ws,
//   - the account service's pages under /account/,
//   - the asset build under /assets/,
//   - the web client at /, when the binary embeds it (the webclient
//     build tag) or --client-dir names a build.
//
// waserver with flags and no command is the same as waserver all.
// Settings come from defaults, then the --config YAML file, then flags.
// server/README.md, "All-in-one", lists them.
//
// Secrets never come from flags or the config file. Each comes from an
// environment variable, or from a file that the same name with _FILE
// names, such as a Docker secret:
//   - WATHENA_DATABASE_URL: the game role. Characters are loaded and
//     saved there, and game tickets are used up there.
//   - WATHENA_ACCOUNT_DATABASE_URL: the account role, which alone can
//     read password hashes and sessions.
//   - WATHENA_ACCOUNT_SECRET_KEY: 32 random bytes in base64, which
//     encrypt TOTP secrets. WATHENA_ACCOUNT_OLD_SECRET_KEYS lists older
//     keys after a key change.
//
// With a database, waserver refuses to start while migrations are
// pending or before `wactl world new` made the world.
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
// waserver health checks a running server's /readyz, for container
// health checks.
package main

import (
	"context"
	crand "crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	// The time zone database, for time_zone on systems without one,
	// such as the distroless image.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	webclient "github.com/lxsmnsyc/wathena/client"
	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/serve"
	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/reference/rathena"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/rng"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

func main() {
	code, err := dispatch(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "waserver:", err)
	}
	os.Exit(code)
}

const usage = `usage: waserver <command> [flags]

commands:
  all       run every service in one process: the game, the account
            pages, the asset build and the web client
  health    check a running server's /readyz, for container health checks
  version   print the version

waserver [flags], with no command, is the same as waserver all [flags].
Run waserver all -h for the flags.
`

// dispatch runs a command and returns the exit status.
func dispatch(args []string) (int, error) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2, nil
	}
	var err error
	switch cmd := args[0]; {
	case cmd == "all":
		err = runAll("waserver all", args[1:])
	case cmd == "health":
		err = checkHealth(args[1:])
	case cmd == "version":
		fmt.Println(version())
		return 0, nil
	case cmd == "help" || cmd == "-h" || cmd == "--help":
		fmt.Print(usage)
		return 0, nil
	case strings.HasPrefix(cmd, "-"):
		// The flags alone, as before there were commands.
		err = runAll("waserver", args)
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2, fmt.Errorf("unknown command %q", cmd)
	}
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0, nil
	case err != nil:
		return 1, err
	}
	return 0, nil
}

// buildVersion is set by release builds, such as the container image,
// with -ldflags "-X main.buildVersion=...".
var buildVersion string

// version returns buildVersion, or the module version and VCS revision
// built into the binary.
func version() string {
	if buildVersion != "" {
		return buildVersion
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	v := info.Main.Version
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			v += " " + s.Value
		case "vcs.modified":
			if s.Value == "true" {
				v += " (modified)"
			}
		}
	}
	return v
}

// checkHealth asks a running server whether it is ready. Container
// images have no curl, so their health check runs this.
func checkHealth(args []string) error {
	flags := flag.NewFlagSet("waserver health", flag.ContinueOnError)
	target := flags.String("url", "http://127.0.0.1:8300/readyz", "the server's /readyz or /healthz URL")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, *target, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G107 G704 -- the URL the operator named.
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", *target, resp.Status)
	}
	return nil
}

// runAll runs the all-in-one server: the gateway, the account service,
// the world with its zones, the asset build and the web client, in one
// process (strategy section 3.9).
func runAll(name string, args []string) error {
	o, database, err := parseOptions(name, args, os.Stderr)
	if err != nil {
		return err
	}
	sec, err := loadSecrets()
	if err != nil {
		return err
	}
	if database != "" {
		sec.Database = database
	}
	cfg, err := validate(o, sec)
	if err != nil {
		return fmt.Errorf("the configuration has problems:\n%w", indent(err))
	}
	// Checking the Lua runtime at startup proves that this build's
	// binding works, including the C stack it needs, before any mod
	// script runs.
	if err := script.SelfTest(); err != nil {
		return fmt.Errorf("lua runtime: %w", err)
	}
	// The reference data built into the binary, so it runs anywhere.
	reference.UseFS(rathena.Files)
	captchaProvider := account.Captcha(account.NoCaptcha{})
	if cfg.Captcha == "dev" {
		captchaProvider = account.DevCaptcha{}
	}
	name = assetName(cfg.StartMap)
	names, err := selectMaps(cfg.Assets, name, strings.Join(cfg.Maps, ","))
	if err != nil {
		return err
	}
	files, err := serve.New(cfg.Assets)
	if err != nil {
		return err
	}
	defer func() { _ = files.Close() }()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	log.Info("lua runtime ready", "lua", script.LuaVersion, "backend", script.Backend)
	web, clientFrom, err := loadClient(cfg)
	if err != nil {
		return err
	}

	// A signal starts the shutdown countdown. ctx ends after it, which
	// stops the zones and saves every character. A second signal skips
	// the rest of the countdown.
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var tables *progression.Tables
	var data *progression.Data
	if cfg.Mode != "none" {
		if tables, data, err = loadProgression(reference.Mode(cfg.Mode)); err != nil {
			return err
		}
	}
	statusMode := reference.Mode(cfg.Mode)
	if cfg.Mode == "none" {
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
	jobs, start, err := startCharacter(statusMode, cfg.Build)
	if err != nil {
		return err
	}
	// The database opens before the zones, since the IDs of mod items
	// are kept there, and the world row fixes the ruleset's base.
	var pool *pgxpool.Pool
	var worldRow store.World
	if cfg.Secrets.Database != "" {
		if pool, err = openDatabase(ctx, cfg.Secrets.Database, "waserver"); err != nil {
			return err
		}
		defer pool.Close()
		if worldRow, err = checkWorld(ctx, pool, cfg); err != nil {
			return err
		}
	}
	itemData, err := loadItemData(statusMode)
	if err != nil {
		return err
	}
	var ids mod.IDStore = &mod.MemoryIDs{}
	if pool != nil {
		ids = store.NewModIDs(pool)
	}
	converted := time.Now()
	core, err := convert.LoadCore(statusMode)
	if err != nil {
		return fmt.Errorf("item and NPC scripts: %w", err)
	}
	log.Info("item and npc scripts converted", "mode", statusMode, "items", len(core.Result.Items), "npcs", len(core.NPCs.NPCs), "took", time.Since(converted).Round(time.Millisecond))
	statuses, failedStatuses, err := core.LoadStatuses(statusMode)
	if err != nil {
		return fmt.Errorf("statuses: %w", err)
	}
	for _, s := range failedStatuses {
		log.Warn("status script not converted; the status gives no bonuses", "status", s.Status, "err", s.Err)
	}
	jobs.SetStatuses(statuses)
	var monsterData map[uint32]gamedata.Monster
	if cfg.Mode != "none" {
		if monsterData, err = gamedata.LoadMonsters(statusMode, tables, data); err != nil {
			return err
		}
	}
	loaded, err := loadMods(ctx, cfg.Mods, rulesetBase(cfg.Mode, worldRow.Ruleset), statusMode, itemData, monsterData, tables, ids, core, log)
	if err != nil {
		return fmt.Errorf("mods:\n%w", indent(err))
	}
	// The ruleset's flags pick the formulas that player stats use.
	if err := gamedata.ApplyRuleset(jobs, loaded.set.Ruleset); err != nil {
		return fmt.Errorf("ruleset: %w", err)
	}
	// A "cast" bonus without a target picks it from the skill's data.
	if loaded.all.Skills != nil {
		jobs.SetAutoSpellSelf(loaded.all.Skills.AutoSpellSelf)
	}
	// Monsters and drops come with the mods' overlays.
	tables = loaded.all.Tables
	spawns, err := gamedata.SpawnsOf(reference.Mode(cfg.Mode), loaded.all.Monsters)
	if err != nil {
		return err
	}
	events, err := worldevent.Apply(loaded.all.Events, cfg.Mods.Events)
	if err != nil {
		return fmt.Errorf("mods:\n%w", indent(err))
	}
	loaded.zone.Location = cfg.Location
	items := item.New(loaded.items.Data)
	// Server variables are shared by every zone, and written soon after
	// they change.
	var varStore variables.Store
	if pool != nil {
		varStore = store.NewServerVariables(pool)
	}
	serverVars := variables.NewServer(varStore, log)
	if err := serverVars.Load(ctx); err != nil {
		return fmt.Errorf("server variables: %w", err)
	}
	loaded.zone.Server = serverVars
	varsDone := make(chan struct{})
	go func() {
		defer close(varsDone)
		serverVars.Run(ctx)
	}()
	defer func() { <-varsDone }()
	given, err := parseStartItems(cfg.StartItems)
	if err != nil {
		return err
	}
	reg := metrics.NewRegistry()
	reg.Log, reg.ProfileDir = log, cfg.ProfileDir
	started := time.Now()
	grids, err := world.LoadGrids(cfg.Assets, names)
	if err != nil {
		return err
	}
	zones := make([]*zone.Zone, len(names))
	monsters, warpCount, npcCount := 0, 0, 0
	for i, n := range names {
		id := strings.TrimPrefix(n, "map/")
		// Each zone has its own secret stream from the world's root, so
		// no zone's randomness depends on another's (rng-v1, label
		// zone:<map ID>).
		z := zone.New(n, grids[i], root.Child("zone:"+id).SecretStream(), spawns[id])
		z.Progress, z.Status, z.Start, z.Items, z.StartItems = tables, jobs, start, items, given
		z.Statuses = statuses
		z.Skills, z.SkillTrees, z.SkillSettings = loaded.all.Skills, loaded.all.SkillTrees, skill.Settings{}
		if sj, err := parseStartJob(cfg.StartJob); err == nil && cfg.StartJob != "" {
			z.Start.Job, z.StartBaseLevel, z.StartJobLevel = sj.job, sj.base, sj.jobLevel
		}
		z.SaveMap = name
		z.SetWarps(warps[n])
		z.SetNPCs(loaded.npcs[n])
		z.Mods = loaded.zone
		npcCount += len(loaded.npcs[n])
		reg.AddZone(z.Metrics)
		zones[i] = z
		monsters += count(spawns[id])
		warpCount += len(warps[n])
	}
	w, err := world.New(name, zones...)
	if err != nil {
		return err
	}
	// World events start without staff action, on the server clock.
	var runStore worldevent.Store
	if pool != nil {
		runStore = store.NewEventRuns(pool)
	}
	for _, d := range events {
		if _, err := w.EventZone(d); err != nil {
			return fmt.Errorf("mods: %s: events %s: %w", d.Where, d.Name, err)
		}
	}
	scheduler := worldevent.New(events, worldevent.Options{Runner: w.Events(), Store: runStore, Location: cfg.Location,
		Rand: root.Child("events").SecretStream(), Log: log})
	if err := scheduler.Load(ctx, time.Now()); err != nil {
		return fmt.Errorf("world events: %w", err)
	}
	next := scheduler.Next()
	for _, d := range scheduler.Defs() {
		attrs := []any{"event", d.Name, "trigger", d.Trigger.Describe(), "minutes", int(d.Duration.Minutes())}
		if at, ok := next[d.Name]; ok {
			attrs = append(attrs, "next", at.In(cfg.Location).Format("2006-01-02 15:04 MST"))
		}
		log.Info("world event", attrs...)
	}
	gw := &gateway.Gateway{World: w, Sessions: gateway.DevSessions{}, Origins: cfg.Origins, Log: log, Metrics: &reg.Conns}
	go reg.Conns.Run(ctx)
	hc := &health{}
	var st *store.Store
	worldName := worldRow.Name
	if pool != nil {
		st = store.New(pool, store.Options{Log: log})
		gw.Characters = dbCharacters{store: st, world: w}
		hc.checks = append(hc.checks, pool.Ping)
	}
	var accounts *account.Service
	if !cfg.Dev {
		pool, err := openDatabase(ctx, cfg.Secrets.AccountDatabase, "waserver-account")
		if err != nil {
			return err
		}
		defer pool.Close()
		hc.checks = append(hc.checks, pool.Ping)
		sealer, err := loadSealer(cfg.Secrets)
		if err != nil {
			return err
		}
		if sealer == nil {
			log.Warn("two-factor codes are off: " + secretKeyEnv + " is not set")
		}
		playURL := cfg.PlayURL
		if playURL == "" && web != nil && cfg.Public != "" {
			// Play opens the client this server serves.
			playURL = cfg.Public + "/"
		}
		accounts, err = account.New(account.Config{
			Pool: pool, PublicURL: cfg.Public, ClientOrigins: cfg.Origins, PlayURL: playURL,
			NewCharacter: func(name string, look account.Look) character.Character {
				sex := uint32(0)
				if look.Male {
					sex = 1
				}
				return w.NewCharacter(name, zone.Look{Sex: sex, Hair: uint32(look.Hair), HairColor: uint32(look.HairColor)}) // #nosec G115 -- the account service keeps hair values small.
			},
			Captcha: captchaProvider, TrustedProxies: cfg.TrustedProxies, Log: log,
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
	go scheduler.Run(ctx)
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
	mux.HandleFunc("GET /healthz", hc.live)
	mux.HandleFunc("GET /readyz", hc.readyz)
	if web != nil {
		mux.Handle("/", web)
	}
	srv := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	servers := []*http.Server{srv}
	if cfg.DebugAddr != "" {
		dbg := http.NewServeMux()
		dbg.Handle("/", reg.Handler())
		if cfg.Dev {
			// Mod reloads are for development only.
			dbg.Handle("POST /debug/wathena/mods/reload", reloadHandler(loaded.set, w, log))
		}
		servers = append(servers, &http.Server{Addr: cfg.DebugAddr, Handler: dbg, ReadHeaderTimeout: 10 * time.Second})
	}
	go func() {
		select {
		case <-signals:
		case <-ctx.Done():
		}
		// Not ready from now, so a proxy stops sending new players.
		hc.ready.Store(false)
		countdown(cfg.ShutdownCountdown, func(text string) { w.Announce(text, 0xFFFF00) }, signals, log)
		stop()
		log.Info("shutting down: saving characters")
		// Players see why the game stopped, and each session's last
		// save is queued as it leaves its zone.
		gw.Shutdown()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, s := range servers {
			_ = s.Shutdown(shutdown)
		}
	}()
	if cfg.DebugAddr != "" {
		debug := servers[1]
		ln, err := net.Listen("tcp", cfg.DebugAddr)
		if err != nil {
			return err
		}
		go func() {
			if err := debug.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
				log.Error("debug server stopped", "err", err)
			}
		}()
		log.Info("measurements", "metrics", "http://"+cfg.DebugAddr+"/debug/wathena/metrics", "profiles", "http://"+cfg.DebugAddr+"/debug/pprof/")
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	var mem runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&mem)
	base := cfg.Public
	if base == "" {
		base = "http://" + cfg.Listen
	}
	log.Info("serving", "version", version(), "world", worldName, "listen", cfg.Listen, "public_url", base, "client", clientFrom,
		"maps", len(names), "start", name, "monsters", monsters, "warps", warpCount, "npcs", npcCount,
		"startup", time.Since(started).Round(time.Millisecond), "heap_mb", mem.HeapAlloc>>20, "database", st != nil, "accounts", accounts != nil)
	if accounts != nil {
		log.Info("account pages", "login", base+account.Prefix+"login")
	} else {
		log.Warn("development sessions: any name plays the character of that name")
	}
	hc.ready.Store(true)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	fmt.Fprint(os.Stderr, "\nmeasurements since the start or the last reset:\n", reg.Snapshot().Text())
	return nil
}

// loadClient returns the web client's handler, and where the client
// comes from for the log: client_dir, the build inside the binary, or
// none.
func loadClient(cfg settings) (http.Handler, string, error) {
	var fsys fs.FS
	from := "none"
	switch {
	case cfg.ClientDir != "":
		root, err := os.OpenRoot(cfg.ClientDir)
		if err != nil {
			return nil, "", err
		}
		defer func() { _ = root.Close() }()
		fsys, from = root.FS(), cfg.ClientDir
	case webclient.Dist() != nil:
		fsys, from = webclient.Dist(), "built in"
	default:
		return nil, from, nil
	}
	h, err := newClientHandler(fsys, clientCSP(wsOrigin(cfg.Public)), strings.HasPrefix(cfg.Public, "https://"))
	if err != nil {
		return nil, "", fmt.Errorf("client_dir: %w", err)
	}
	return h, from, nil
}

// indent puts each line of a joined error under a dash.
func indent(err error) error {
	lines := strings.Split(err.Error(), "\n")
	for i, l := range lines {
		lines[i] = "  - " + l
	}
	return errors.New(strings.Join(lines, "\n"))
}

// loadSealer makes the TOTP sealer from the secret keys. It returns nil
// when no key is set. Its errors never quote a key.
func loadSealer(s secrets) (*account.Sealer, error) {
	if s.SecretKey == "" {
		return nil, nil
	}
	current, err := account.ParseSecretKey(s.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", secretKeyEnv, err)
	}
	var old [][]byte
	for i, v := range splitList(s.OldSecretKeys) {
		k, err := account.ParseSecretKey(v)
		if err != nil {
			return nil, fmt.Errorf("%s, key %d: %w", oldSecretKeysEnv, i+1, err)
		}
		old = append(old, k)
	}
	return account.NewSealer(current, old...)
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

// checkWorld reads the world that `wactl world new` made. Outside dev, a
// database without a world is an error. The world's ruleset base must
// match the mode, unless the mode is none.
func checkWorld(ctx context.Context, pool *pgxpool.Pool, cfg settings) (store.World, error) {
	w, err := store.LoadWorld(ctx, pool)
	switch {
	case errors.Is(err, store.ErrNoWorld) && cfg.Dev:
		return store.World{}, nil
	case err != nil:
		return store.World{}, err
	case cfg.Mode != "none" && w.Ruleset != cfg.Mode:
		return store.World{}, fmt.Errorf("the world %q uses the %s ruleset, but mode is %s", w.Name, w.Ruleset, cfg.Mode)
	}
	return w, nil
}

// rulesetBase is the ruleset base that the mode or the world row fixes:
// --mode renewal or pre-renewal, else the world row's, else none.
func rulesetBase(mode, worldRuleset string) string {
	if b, ok := mod.ModeBase(mode); ok {
		return b
	}
	b, _ := mod.ModeBase(worldRuleset)
	return b
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

// loadProgression is gamedata.LoadProgression.
func loadProgression(mode reference.Mode) (*progression.Tables, *progression.Data, error) {
	return gamedata.LoadProgression(mode)
}

// loadItemData is gamedata.LoadItemData.
func loadItemData(mode reference.Mode) (*item.Data, error) { return gamedata.LoadItemData(mode) }

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

func count(spawns []zone.Spawn) int {
	n := 0
	for _, s := range spawns {
		n += s.Amount
	}
	return n
}

// secret returns random bytes for the zones' random streams. Each start
// gets new ones: play randomness does not come from the world seed
// (strategy section 8.3).
func secret() []byte {
	b := make([]byte, 32)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// startCharacter is gamedata.StartCharacter.
func startCharacter(mode reference.Mode, build string) (*status.Tables, status.Character, error) {
	return gamedata.StartCharacter(mode, build)
}
