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
//   - the admin API under /admin/api/v1/, and the admin screens at
//     /admin when the client is served,
//   - the asset build under /assets/, the mods' builds under
//     /assets/mods/<mod id>/<version>/, and the client data at
//     /client-data.json,
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
// on a loopback address. With --dev, the database is optional. Without
// it, characters live in memory until the server stops, and a name that
// logs in again plays the character as it was when it left.
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
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	// The time zone database, for time_zone on systems without one,
	// such as the distroless image.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	webclient "github.com/lxsmnsyc/wathena/client"
	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/serve"
	"github.com/lxsmnsyc/wathena/internal/buildinfo"
	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/reference/rathena"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modassets"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/restart"
	"github.com/lxsmnsyc/wathena/server/retention"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
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
  version   print the version and the world modes of the build

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
	case cmd == "version" || cmd == "--version":
		fmt.Println(buildinfo.Line(buildVersion))
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
	var planned *plannedRestart
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0, nil
	case errors.As(err, &planned):
		// main prints the error, so the log says why it exited.
		return restart.ExitCode, err
	case err != nil:
		return 1, err
	}
	return 0, nil
}

// buildVersion is set by release builds, such as scripts/release.sh
// and the container image, with -ldflags "-X main.buildVersion=...".
var buildVersion string

// version returns buildVersion, or the module version and VCS revision
// built into the binary.
func version() string { return buildinfo.Version(buildVersion) }

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
	files, err := serve.New(cfg.Assets)
	if err != nil {
		return err
	}
	defer func() { _ = files.Close() }()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	for _, w := range cfg.Warnings {
		log.Warn("config: " + w)
	}
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
	if cfg.Seed != "" {
		log.Warn("the random streams follow --seed; use it only for tests")
	}
	root, err := runRoot(cfg.Seed)
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
		if err := checkDatabaseRole(ctx, pool, true, cfg.Dev, log); err != nil {
			return err
		}
		if worldRow, err = checkWorld(ctx, pool, cfg); err != nil {
			return err
		}
	}
	// A procedural world generates its fields now, from its public
	// branch (decision 0039). Its towns and dungeons follow the mods,
	// which give their templates and styles (decision 0040).
	proc, err := fieldMaps(cfg, worldRow, log)
	if err != nil {
		return err
	}
	if err := loadLayout(ctx, pool, worldRow, &proc); err != nil {
		return err
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
	core, cache, err := loadCore(cfg.options, statusMode, log)
	if err != nil {
		return fmt.Errorf("item and NPC scripts: %w", err)
	}
	log.Info("item and npc scripts converted", "mode", statusMode, "items", len(core.Result.Items), "cache", cache, "took", time.Since(converted).Round(time.Millisecond))
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
	loaded, err := loadMods(ctx, cfg.Mods, rulesetBase(cfg.Mode, worldRow.Ruleset), statusMode, itemData, monsterData, tables, jobs, ids, core, log)
	if err != nil {
		return fmt.Errorf("mods:\n%w", indent(err))
	}
	if err := addMaps(cfg, &proc, loaded.set, pool, log); err != nil {
		return err
	}
	if err := worldQuests(proc, loaded, log); err != nil {
		return err
	}
	name, assetMaps, names, err := worldMaps(cfg, proc)
	if err != nil {
		return err
	}
	if w := preRenewalClassicWarning(cfg.Mode, proc.set == nil); w != "" {
		log.Warn(w)
	}
	// The mods' asset builds are checked now, and served with the client
	// data (decision 0033).
	modArt, err := modassets.Load(loaded.all, modassets.Options{MaxBytes: int64(cfg.Mods.MaxAssetMB) << 20, Log: log})
	if err != nil {
		return fmt.Errorf("mods:\n%w", indent(err))
	}
	defer func() { _ = modArt.Close() }()
	// The job tables come from the job records: core's and the mods'.
	// LoadMods set the ruleset's flags that player stats read on them.
	jobs = loaded.all.JobTables
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
	if spawns == nil {
		spawns = map[string][]zone.Spawn{}
	}
	// A procedural world's fields and floors spawn the lists that wactl
	// world new stored (decision 0042), and its rotating floors and
	// changed regions the lists of their derived keys (decision 0045).
	if err := changedSpawns(cfg, &proc, loaded.all, log); err != nil {
		return err
	}
	zoneSpawns(proc, loaded.all.Monsters, spawns, log)
	// Every event, with whether it is on: the config's settings, which
	// staff changes from the database go over below (decision 0031).
	eventConfig, err := worldevent.Settings(loaded.all.Events, cfg.Mods.Events)
	if err != nil {
		return fmt.Errorf("mods:\n%w", indent(err))
	}
	loaded.zone.Location = cfg.Location
	// With --dev, the server clock can be moved forward from the debug
	// address, for tests (decision 0018).
	var clock func() time.Time
	var dclock *devClock
	if cfg.Dev {
		dclock = &devClock{}
		clock = dclock.Now
	}
	// One chat hub for the world: whispers, channels and block lists.
	hub := chat.NewHub(loaded.all.Chat.Channels, loaded.all.Chat.Filter)
	hub.Limits, hub.Clock = cfg.Chat.Limits, clock
	// One social registry for the world: parties, friend lists and party
	// booking (decision 0020). Every saved party loads now.
	worldRules := loaded.zone.Rules
	if worldRules == nil {
		worldRules = rules.Of(statusMode == "renewal")
	}
	socials := social.New(rules.LimitsOrDefault(worldRules), rules.PartyOrDefault(worldRules))
	socials.Blocks, socials.Clock = hub.BlocksName, clock
	// Guilds use the tables of the world's base (decision 0021).
	guildTables := social.CoreGuildTables(worldRules.Renewal)
	loaded.zone.Guilds = guildTables
	socials.SetGuildData(guildTables, rules.GuildOrDefault(worldRules))
	var socialStore *store.SocialStore
	if pool != nil {
		parties, err := store.LoadParties(ctx, pool)
		if err != nil {
			return fmt.Errorf("parties: %w", err)
		}
		socialStore = store.NewSocialStore(pool, log)
		socials.Store = socialStore
		socials.Load(parties)
		log.Info("parties loaded", "parties", len(parties))
		guilds, storageLogs, err := store.LoadGuilds(ctx, pool)
		if err != nil {
			return fmt.Errorf("guilds: %w", err)
		}
		socials.LoadGuilds(guilds)
		floor, err := store.GuildIDFloor(ctx, pool)
		if err != nil {
			return fmt.Errorf("guilds: %w", err)
		}
		socials.SetGuildIDFloor(floor)
		for id, l := range storageLogs {
			socials.SetStorageLog(id, l)
		}
		log.Info("guilds loaded", "guilds", len(guilds))
	}
	jail := zone.Jail{Map: assetName(cfg.Jail.Map), Cells: cfg.Jail.Cells}
	if !slices.Contains(names, jail.Map) {
		log.Warn("the jail map does not run: jailed players stay where they are and cannot warp", "jail", jail.Map)
	}
	items := item.New(loaded.items.Data)
	// Where the points of removed maps go, which every zone shares
	// (decision 0045).
	moved := &zone.Relocations{}
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
	// The variables are written until the server stops. A return before
	// it serves ends ctx first, so the wait cannot block the return.
	defer background(ctx, stop, serverVars.Run)()
	given, err := parseStartItems(cfg.StartItems)
	if err != nil {
		return err
	}
	reg := metrics.NewRegistry()
	reg.Log, reg.ProfileDir = log, cfg.ProfileDir
	started := time.Now()
	grids, err := world.LoadGrids(cfg.Assets, assetMaps)
	if err != nil {
		return err
	}
	grids = append(grids, fieldGrids(proc)...)
	if proc.set != nil {
		// The warps of the world layout's links, the places of its maps
		// for wa.places and the NPCs placed by tag, the atlas, and the
		// teleport service at each town's teleport place (decision
		// 0041).
		maps.Copy(warps, proc.warps)
		idx := proc.set.Places(proc.warps)
		loaded.zone.Places, loaded.zone.Atlas = idx, proc.atlas
		proc.host.places, proc.host.moved = idx, moved
		loaded.zone.Teleport = zone.TeleportPrices{Base: int64(cfg.Procedural.TeleportBase), Step: int64(cfg.Procedural.TeleportStep)}
		loaded.zone.DropRollLevels = cfg.Procedural.dropRollLevels()
		for m, list := range placeNPCs(idx, loaded.all.PlaceNPCs, log) {
			loaded.npcs[m] = append(loaded.npcs[m], list...)
		}
		for m, list := range zone.TeleportNPCs(proc.atlas, idx, teleportSprite) {
			loaded.npcs[m] = append(loaded.npcs[m], list...)
		}
	} else if len(loaded.all.PlaceNPCs) > 0 {
		log.Info("NPCs placed by tag do not appear in a classic world", "npcs", len(loaded.all.PlaceNPCs))
	}
	// Storages and carts load at login and leave memory after their last
	// commit (decision 0023). Without a database they stay in memory.
	storages := storage.NewRegistry(nil)
	// Mailboxes load at login too, and the market lists every store of
	// the world, for the store search (decision 0024).
	mails := mail.NewRegistry(nil, items)
	stores := market.NewRegistry()
	autotraders := zone.NewAutotraders()
	if loaded.zone.Shops != nil {
		loaded.zone.Shops.Location = cfg.Location
	}
	// The staff roles: the world's owner account, the config's owners,
	// and the roles the admin screens gave, which the database keeps
	// (decision 0028, review decision 54).
	staffRoles := staff.NewRegistry(cfg.Roles, cfg.Staff.Owners)
	staffRoles.SetOwnerAccount(worldRow.OwnerAccount)
	if len(cfg.Staff.Owners) > 0 {
		log.Warn(staff.OwnersByNameWarning, "owners", cfg.Staff.Owners)
	}
	if pool != nil {
		given, err := store.LoadStaffRoles(ctx, pool)
		if err != nil {
			return fmt.Errorf("staff roles: %w", err)
		}
		for account, role := range given {
			if !cfg.Roles.Exists(role) {
				log.Warn("an account has a staff role the config does not have; it counts as no role", "account", account, "role", role)
			}
		}
		staffRoles.Load(given)
	}
	// newZone makes a map's zone, at the start and for the maps that
	// rotations and world changes add (decision 0045). late applies the
	// settings that come after the world exists.
	var late []func(z *zone.Zone)
	newZone := func(n string, grid *path.Grid, sp []zone.Spawn, ws []zone.Warp, npcs []zone.NPC) *zone.Zone {
		id := strings.TrimPrefix(n, "map/")
		// Each zone has its own secret stream from the world's root, so
		// no zone's randomness depends on another's (rng-v1, label
		// zone:<map ID>).
		z := zone.New(n, grid, zoneStream(root, id), sp)
		z.Progress, z.Status, z.Start, z.Items, z.StartItems = tables, jobs, start, items, given
		z.Statuses = statuses
		z.Skills, z.SkillTrees, z.SkillSettings = loaded.all.Skills, loaded.all.SkillTrees, skill.Settings{}
		if sj, err := parseStartJob(cfg.StartJob); err == nil && cfg.StartJob != "" {
			z.Start.Job, z.StartBaseLevel, z.StartJobLevel = sj.job, sj.base, sj.jobLevel
		}
		z.SaveMap = name
		if n == name && proc.hasSpawn {
			z.Spawn, z.HasSpawn = proc.spawn, true
		}
		z.Chat, z.Jail, z.Clock, z.Social, z.Storages = hub, jail, clock, socials, storages
		z.Mail, z.Market, z.Autotraders = mails, stores, autotraders
		z.Staff, z.Moved = staffRoles, moved
		z.SetWarps(ws)
		z.SetNPCs(npcs)
		z.Mods = loaded.zone
		reg.AddZone(z.Metrics)
		for _, f := range late {
			f(z)
		}
		return z
	}
	zones := make([]*zone.Zone, len(names))
	monsters, warpCount, npcCount := 0, 0, 0
	for i, n := range names {
		id := strings.TrimPrefix(n, "map/")
		zones[i] = newZone(n, grids[i], spawns[id], warps[n], loaded.npcs[n])
		npcCount += len(loaded.npcs[n])
		monsters += count(spawns[id])
		warpCount += len(warps[n])
	}
	w, err := world.New(name, zones...)
	if err != nil {
		return err
	}
	// Rotations and world changes (decision 0045): the changes that came
	// while the server was stopped are recorded before anyone joins, and
	// the manager makes the later ones on the server clock.
	var changes *rotation.Manager
	if proc.host != nil {
		h := proc.host
		h.world, h.newZone = w, newZone
		h.retire = func(z *zone.Zone) {
			w.Retire(z)
			reg.RemoveZone(z.Metrics)
		}
		if changes, err = startChanges(ctx, proc, pool, moved, log); err != nil {
			return err
		}
	}
	// World events start without staff action, on the server clock.
	// Typed events pick fields by the level of their monsters (decision
	// 0025).
	var runStore interface {
		worldevent.Store
		worldevent.ProgressStore
	} = &worldevent.MemoryStore{}
	if pool != nil {
		runStore = store.NewEventRuns(pool)
	}
	for _, s := range eventConfig {
		if !s.On {
			continue
		}
		if err := w.CheckEvent(s.Def); err != nil {
			return fmt.Errorf("mods: %s: events %s: %w", s.Def.Where, s.Def.Name, err)
		}
	}
	// Staff changes of events and mods, which the database keeps over the
	// config (decision 0031).
	events := eventConfig
	var eventOverrides []store.EventOverride
	var modSwitches []store.ModSwitch
	if pool != nil {
		if eventOverrides, err = store.LoadEventOverrides(ctx, pool); err != nil {
			return fmt.Errorf("event changes: %w", err)
		}
		var warnings []string
		events, warnings = adminworld.ApplyOverrides(eventConfig, eventOverrides)
		for _, m := range warnings {
			log.Warn(m)
		}
		for i, s := range events {
			if s.On {
				if err := w.CheckEvent(s.Def); err != nil {
					log.Warn("an event that staff turned on stays off: its maps do not run", "event", s.Def.Name, "err", err)
					events[i].On = false
				}
			}
		}
		if modSwitches, err = store.LoadModSwitches(ctx, pool); err != nil {
			return fmt.Errorf("mod switches: %w", err)
		}
		offAt := adminworld.OffAt(modSwitches)
		for _, sw := range modSwitches {
			if loaded.set.Mod(sw.Mod) == nil {
				log.Warn("staff turned off a mod that does not load", "mod", sw.Mod)
			}
			if sw.Map != "" && w.Zone(sw.Map) == nil {
				log.Warn("staff turned off a mod on a map that does not run", "mod", sw.Mod, "map", sw.Map)
			}
		}
		for _, z := range zones {
			off := append(slices.Clone(offAt[""]), offAt[z.Map]...)
			z.SetModsOff(off)
			if len(off) > 0 {
				log.Info("mods turned off by staff", "map", z.Map, "mods", strings.Join(off, ","))
			}
		}
		late = append(late, func(z *zone.Zone) { z.SetModsOff(append(slices.Clone(offAt[""]), offAt[z.Map]...)) })
	}
	levels := map[string]int{}
	for id, l := range gamedata.FieldLevels(spawns) {
		if w.Zone("map/"+id) != nil {
			levels["map/"+id] = l
		}
	}
	eventRand := root.Child("events").SecretStream()
	runs := worldevent.NewRuns(w.Events(), worldevent.RunsOptions{Levels: levels, Rand: eventRand, Progress: runStore, Log: log})
	for _, z := range zones {
		z.WorldEvents = runs
	}
	late = append(late, func(z *zone.Zone) { z.WorldEvents = runs })
	scheduler := worldevent.NewFrom(events, worldevent.Options{Runner: runs, Store: runStore, Location: cfg.Location,
		Rand: eventRand, Log: log, Changed: runs.SetEvents})
	runs.SetEvents(scheduler.Defs())
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
	// Planned restarts (decision 0031): announcements, then no new
	// logins in the last minute, then a save, a disconnect and an exit
	// that the process manager restarts.
	restartNow := make(chan restart.Plan, 1)
	planner := restart.New(restart.Options{Clock: clock, Log: log,
		Announce: func(text string) { w.Announce(text, 0xFFFF00) },
		Notify: func(p *restart.Plan) {
			ev := modscript.RestartPlanned{Cancelled: p == nil}
			if p != nil {
				ev.At, ev.Reason = p.At, p.Reason
			}
			w.RestartPlanned(ev)
		},
		Due: func(p restart.Plan) {
			select {
			case restartNow <- p:
			default:
			}
		},
	})
	gw := &gateway.Gateway{World: w, Sessions: gateway.DevSessions{}, Origins: cfg.Origins, Log: log, Metrics: &reg.Conns,
		TrustedProxies: cfg.TrustedProxies, Conns: cfg.Connections.limits(),
		Chat: cfg.Chat.Limits, CheckName: hub.Filter.CheckName, Limits: rules.LimitsOrDefault(worldRules), Closed: planner.Closed,
		Mode: proc.mode}
	go reg.Conns.Run(ctx)
	hc := &health{}
	var st *store.Store
	worldName := worldRow.Name
	if pool != nil {
		st = store.New(pool, store.Options{Log: log})
		storages.Loader = st
		mails.Loader = st
		floor, err := st.MailIDFloor(ctx)
		if err != nil {
			return fmt.Errorf("mail: %w", err)
		}
		mails.SetIDFloor(floor)
		if loaded.zone.Shops != nil {
			rows, err := st.ShopStock(ctx)
			if err != nil {
				return fmt.Errorf("shop stock: %w", err)
			}
			log.Info("shop stock loaded", "items", loaded.zone.Shops.LoadStock(rows))
		}
		gw.Characters = dbCharacters{store: st, world: w}
		hc.checks = append(hc.checks, pool.Ping)
		// Transfers commit the characters they change with their log
		// lines, in one transaction (decision 0017).
		for _, z := range zones {
			z.Committer = storeCommitter{st}
		}
		late = append(late, func(z *zone.Zone) { z.Committer = storeCommitter{st} })
		// Guild writes run in order with the commits, so a guild's row is
		// there before a commit writes its storage (decision 0021).
		if socialStore != nil {
			socialStore.Lane = st.Lane
		}
	}
	// The chat log and reports go to the database, or stay in memory
	// without one.
	var chatLog *store.ChatLog
	memoryLog := &chat.MemoryLog{}
	if pool != nil {
		chatLog = store.NewChatLog(pool, store.ChatLogOptions{Log: log})
		hub.Log = chatLog
	} else {
		hub.Log = memoryLog
	}
	var accounts *account.Service
	var adminAPI *admin.Handler
	// The retention job deletes old login records, chat lines, log
	// partitions and audit rows once a day (decision 0049). It needs the
	// account role's pool for the login records and the audit log, so it
	// starts once that pool is open, or without it with --dev.
	var retentionJob *retention.Job
	if pool != nil {
		retentionJob = &retention.Job{Game: pool, Periods: cfg.Retention, Log: log}
	}
	// The world side of the admin API: online players from their zones,
	// offline ones and the logs from the database, and restores
	// (decision 0029).
	var scriptMods []string
	for _, sc := range loaded.zone.Scripts {
		scriptMods = append(scriptMods, sc.Mod)
	}
	// The events tool lists the rotations and seasons, and forces them
	// (decision 0045).
	var changeTool adminworld.ChangeManager
	if changes != nil {
		changeTool = changes
	}
	staffWorld := adminworld.New(adminworld.Config{Staff: staffRoles, World: w, Store: st, Chat: hub, Items: items, Jobs: jobs,
		StorageRecords: loaded.zone.Storages, Currencies: loaded.zone.Currencies, Quests: loaded.zone.Quests, Storages: storages, Mail: mails, Social: socials,
		Kicker: kicker{gw, w}, Clock: clock, Log: log,
		Tools: adminworld.Tools{Metrics: reg, Started: started, Version: version(), Restart: planner, Scheduler: scheduler, Runs: runs,
			Events: eventConfig, Overrides: eventOverrides, Mods: loaded.set, ScriptMods: scriptMods, Switches: modSwitches,
			Changes: changeTool, Atlas: proc.atlas}})
	// GM commands give their sanctions through the same world side, and
	// banned accounts cannot join (decision 0030).
	for _, z := range zones {
		z.Moderator = staffWorld
	}
	late = append(late, func(z *zone.Zone) { z.Moderator = staffWorld })
	if st != nil {
		gw.Bans = gameBans{st}
		gw.Deletions = gameDeletions{st}
	}
	if !cfg.Dev {
		pool, err := openDatabase(ctx, cfg.Secrets.AccountDatabase, "waserver-account")
		if err != nil {
			return err
		}
		defer pool.Close()
		if err := checkDatabaseRole(ctx, pool, false, cfg.Dev, log); err != nil {
			return err
		}
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
			CheckName: hub.Filter.CheckName, Guilds: accountGuilds{socials}, Closed: planner.Closed,
			// The account page asks the world about block lists, reports,
			// sanctions and exports (decision 0032).
			Game: staffWorld.AccountPage(),
			Privacy: account.Privacy{Contact: cfg.Privacy.Contact, Text: cfg.Privacy.Text, LoginIPDays: cfg.Retention.LoginIPDays,
				ChatLogDays: cfg.Retention.ChatDays, ItemLogDays: cfg.Retention.ItemLogDays, AuditDays: cfg.Retention.AuditDays},
			Owners: cfg.Staff.Owners, OwnerAccount: worldRow.OwnerAccount,
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
		// A ban ends the account's sessions on the account pages
		// (decision 0036).
		staffWorld.SetSessions(accounts)
		go accounts.Run(ctx)
		// The retention job deletes login records and audit rows as the
		// account role, and the admin API shows it.
		var retentionStatus admin.Retention
		if retentionJob != nil {
			retentionJob.Account = pool
			retentionStatus = retentionJob
		}
		// The admin API runs on the account service's origin and pool. It
		// reaches the world in this process through staffWorld.
		adminAPI, err = admin.New(admin.Config{Pool: pool, Auth: accounts, World: staffWorld, Roles: cfg.Roles,
			Owners: cfg.Staff.Owners, OwnerAccount: worldRow.OwnerAccount, Retention: retentionStatus, LoginIPDays: cfg.Retention.LoginIPDays, Log: log})
		if err != nil {
			return err
		}
		adminAPI.WarnMissingOwners(ctx)
	}
	go w.Run(ctx)
	go scheduler.Run(ctx)
	if changes != nil {
		go changes.Run(ctx, clock)
	}
	go planner.Run(ctx)
	if retentionJob != nil {
		go retentionJob.Run(ctx)
	}
	go expireMail(ctx, mails, st, worldRules, clock, log)
	if st != nil {
		restoreAutotraders(ctx, st, gw.Characters, w, rules.MarketOrDefault(worldRules), log)
		// The account deletion job runs every hour (decision 0032).
		go staffWorld.RunDeletions(ctx)
	}
	if socialStore != nil {
		// Party and friend changes are written once every zone stopped.
		defer func() {
			stop()
			<-w.Stopped()
			drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := socialStore.Close(drain); err != nil {
				log.Error("party and friend changes lost at shutdown", "err", err)
			}
		}()
	}
	if chatLog != nil {
		// The chat log writes what waits once every zone stopped.
		defer func() {
			stop()
			<-w.Stopped()
			drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := chatLog.Close(drain); err != nil {
				log.Error("chat lines lost at shutdown", "err", err)
			}
		}()
	}
	if st != nil {
		// On shutdown every zone hands its players' last saves to the
		// store, which then gets 10 seconds to write them.
		defer func() {
			stop()
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
	mux.Handle(modassets.Prefix, modArt)
	mux.Handle(modassets.ClientDataPath, modArt)
	mux.Handle("/ws", gw)
	if proc.set != nil {
		mux.Handle(fields.Prefix, proc.set)
	}
	if accounts != nil {
		mux.Handle(account.Prefix, accounts)
		mux.Handle(admin.Prefix, adminAPI)
	}
	mux.HandleFunc("GET /emblems/{guild}/{file}", emblemHandler(socials))
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
		if retentionJob != nil {
			dbg.Handle("GET /debug/wathena/metrics/prometheus", prometheusHandler(retentionJob))
		}
		if cfg.Dev {
			// Mod reloads and the item audit are for development only.
			dbg.Handle("POST /debug/wathena/mods/reload", reloadHandler(loaded.set, w, log))
			dbg.Handle("GET /debug/wathena/items", auditHandler(w))
			dbg.Handle("GET /debug/wathena/reports", reportsHandler(memoryLog, chatLog))
			dbg.Handle("POST /debug/wathena/clock", clockHandler(dclock))
			dbg.Handle("POST /debug/wathena/emblem", devEmblemHandler(socials))
			dbg.Handle("POST /debug/wathena/guild-storage", devGuildStorageHandler(w))
			dbg.Handle("POST /debug/wathena/storage", devStorageHandler(w))
			dbg.Handle("POST /debug/wathena/cart", devCartHandler(w))
			dbg.Handle("POST /debug/wathena/stall", devStallHandler(w))
			dbg.Handle("POST /debug/wathena/staff", devStaffHandler(hub, staffRoles))
			dbg.Handle("POST /debug/wathena/announce", devAnnounceHandler(staffWorld))
			dbg.Handle("POST /debug/wathena/restart", devRestartHandler(staffWorld))
			dbg.Handle("DELETE /debug/wathena/restart", devRestartHandler(staffWorld))
			if st != nil {
				dbg.Handle("POST /debug/wathena/restore", devRestoreHandler(staffWorld, st))
				dbg.Handle("POST /debug/wathena/sanction", devSanctionHandler(staffWorld, st))
				dbg.Handle("POST /debug/wathena/deletions", devDeletionsHandler(staffWorld))
			}
		}
		servers = append(servers, &http.Server{Addr: cfg.DebugAddr, Handler: debugGuard(dbg), ReadHeaderTimeout: 10 * time.Second})
	}
	// planned is the restart that stopped the server, if one did.
	var planned atomic.Pointer[restart.Plan]
	go func() {
		select {
		case <-signals:
		case <-ctx.Done():
		case p := <-restartNow:
			planned.Store(&p)
		}
		// Not ready from now, so a proxy stops sending new players.
		hc.ready.Store(false)
		p := planned.Load()
		if p == nil {
			// A planned restart counted down already.
			countdown(cfg.ShutdownCountdown, func(text string) { w.Announce(text, 0xFFFF00) }, signals, log)
		} else {
			// The players get the restart's reason before the zones
			// stop, since a zone that stops closes its connections
			// without one. Each leaves its zone, which saves it.
			gw.ShutdownWith(wathenav1.DisconnectReason_DISCONNECT_REASON_RESTART, restart.DisconnectText(*p))
			time.Sleep(time.Second)
		}
		stop()
		log.Info("shutting down: saving characters")
		// Players see why the game stopped, and each session's last
		// save is queued as it leaves its zone.
		if p == nil {
			gw.Shutdown()
		}
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
	if p := planned.Load(); p != nil {
		// The saves are written as runAll returns. Then waserver exits
		// with restart.ExitCode, and the process manager starts it again.
		return &plannedRestart{reason: p.Reason}
	}
	return nil
}

// plannedRestart is runAll's result after a planned restart, which
// dispatch turns into restart.ExitCode.
type plannedRestart struct{ reason string }

func (p *plannedRestart) Error() string { return "restarting as planned: " + p.reason }

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
	if err := checkWorldMode(w); err != nil {
		return store.World{}, err
	}
	return w, nil
}

// checkWorldMode refuses a world this build cannot run: a procedural
// world in a build without the procedural tag, or a generator version or
// rng spec the build lacks (strategy sections 8.1 and 8.4).
func checkWorldMode(w store.World) error {
	switch {
	case w.Mode == gen.ModeProcedural && !gen.HasMode(w.Mode):
		return fmt.Errorf("the world %q is procedural, but this waserver was built without -tags procedural", w.Name)
	case !gen.HasMode(w.Mode):
		return fmt.Errorf("the world %q has the unknown mode %q", w.Name, w.Mode)
	case w.Mode == gen.ModeClassic:
		return nil
	}
	g, ok := gen.Lookup(w.GeneratorVersion)
	if !ok {
		return fmt.Errorf("the world %q uses map generator %d, which this waserver lacks; it has %v", w.Name, w.GeneratorVersion, gen.Versions())
	}
	if g.RNG != w.RNGVersion {
		return fmt.Errorf("the world %q uses %s, but map generator %d uses %s", w.Name, w.RNGVersion, g.Version, g.RNG)
	}
	return nil
}

// worldMode is what Welcome tells clients about the world. A dev server
// without a world runs classic maps.
func worldMode(w store.World) gateway.WorldMode {
	if w.Mode != gen.ModeProcedural {
		return gateway.WorldMode{}
	}
	return gateway.WorldMode{Procedural: true, GeneratorVersion: w.GeneratorVersion, RNGVersion: w.RNGVersion}
}

// preRenewalClassicWarning is the warning for a classic world in
// pre-renewal mode, which is experimental until core's pre-renewal
// ruleset exists (review decision 55, phase 10 item 1). It is "" for any
// other world.
func preRenewalClassicWarning(mode string, classic bool) string {
	if !classic || mode != string(reference.PreRenewal) {
		return ""
	}
	return "pre-renewal classic worlds are experimental until core.ruleset.pre_renewal matches rAthena's pre-renewal (phase 10); the first release supports classic worlds with mode: renewal"
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
// kicker closes game connections for the staff tools: the gateway's,
// and the characters' autotrade.
type kicker struct {
	gw *gateway.Gateway
	w  *world.World
}

func (k kicker) Kick(ctx context.Context, ids []int64, reason wathenav1.DisconnectReason, text string) []int64 {
	kicked := k.gw.Kick(ids, reason, text)
	for _, id := range ids {
		k.w.EndAutotrade(ctx, id)
	}
	return kicked
}

// gameBans refuses the game logins of banned accounts.
type gameBans struct{ store *store.Store }

func (b gameBans) Banned(ctx context.Context, character int64) (string, bool, error) {
	ban, ok, err := b.store.ActiveBan(ctx, character)
	if err != nil || !ok {
		return "", false, err
	}
	return staff.BanText(ban.Reason, ban.End, ban.Permanent), true, nil
}

// gameDeletions refuses the game logins of accounts set to be deleted
// (decision 0032).
type gameDeletions struct{ store *store.Store }

func (d gameDeletions) Deleting(ctx context.Context, character int64) (string, bool, error) {
	del, ok, err := d.store.DeletionOf(ctx, character)
	if err != nil || !ok {
		return "", false, err
	}
	return account.DeletionText(del.After), true, nil
}

type dbCharacters struct {
	store *store.Store
	world *world.World
}

func (d dbCharacters) Find(ctx context.Context, name string, look zone.Look) (int64, error) {
	c := d.world.NewCharacter(name, look)
	return d.store.DevCharacter(ctx, &c)
}

// storeCommitter hands the zones' transfer commits to the store.
type storeCommitter struct{ store *store.Store }

func (c storeCommitter) Commit(saves []zone.CommitSave, storages []zone.StorageCommit, logs transfer.Logs) {
	c.CommitWrites(saves, storages, zone.Writes{}, logs)
}

// CommitWrites hands a commit with mails, shop stock and store rows to
// the store.
func (c storeCommitter) CommitWrites(saves []zone.CommitSave, storages []zone.StorageCommit, w zone.Writes, logs transfer.Logs) {
	entries := make([]store.Entry, 0, len(saves))
	for _, s := range saves {
		if x, ok := s.Saver.(*store.Session); ok {
			entries = append(entries, store.Entry{Session: x, Char: s.Character})
		}
	}
	writes := make([]store.StorageWrite, 0, len(storages))
	for _, s := range storages {
		writes = append(writes, store.StorageWrite{Guild: s.Guild, Key: s.Key, Base: s.Base, Cur: s.Cur,
			CurrencyBase: s.CurrencyBase, Currencies: s.Currencies, HasCurrencies: s.HasCurrencies, SizeBase: s.SizeBase, Size: s.Size})
	}
	units := make([]store.EventUnit, 0, len(w.Events))
	for _, u := range w.Events {
		units = append(units, store.EventUnit{Event: u.Event, Scheduled: u.Scheduled, Unit: u.Unit})
	}
	c.store.CommitAll(entries, writes, store.Writes{Mails: w.Mails, Stock: w.Stock, Stores: w.Stores, Events: units, Audit: w.Audit}, logs)
}

// Load waits until the character's earlier sessions ended, so an older
// session still in its zone cannot commit after this one read the
// character (store.Login).
func (d dbCharacters) Load(ctx context.Context, id int64) (*character.Character, zone.Saver, error) {
	c, s, err := d.store.Login(ctx, id)
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
// runRoot is the root of the zones' and world events' random streams,
// which roll loot, rolled values, hits and monster moves: a fresh secret
// at each start, or the text of --seed, which needs --dev, so a test
// repeats. It is not the world's seed tree, so neither the public branch
// that clients get nor the stored secret branch predicts it (the public
// seed test, decision 0046).
func runRoot(seedText string) (rng.Node, error) {
	seed := secret()
	if seedText != "" {
		seed = []byte(seedText)
	}
	return rng.NewRoot(seed)
}

// zoneStream is a map's zone's stream from the run's root: each zone
// has its own, so no zone's draws depend on another's (rng-v1, label
// zone:<map ID>).
//
// Rules:
//   - The first zone of a map in a run uses the label alone.
//   - A zone that replaces it, such as after a season change, uses the
//     label with index n, where n counts the map's earlier zones. So a
//     replaced zone never repeats an earlier zone's draws.
func zoneStream(root rng.Node, id string) *rng.Rand {
	zoneInstances.Lock()
	k := zoneInstanceKey{root, id}
	n := zoneInstances.made[k]
	zoneInstances.made[k] = n + 1
	zoneInstances.Unlock()
	if n == 0 {
		return root.Child("zone:" + id).SecretStream()
	}
	return root.Child("zone:"+id, n).SecretStream()
}

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
