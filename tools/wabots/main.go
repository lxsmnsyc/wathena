// Command wabots runs headless bots against a game server and reports
// what they and the server measured (server/README.md, "Load tests and
// metrics").
//
//	wabots --scenario town --assets <build> --metrics http://127.0.0.1:8301
//	wabots version
//
// A scenario is one of botclient.Scenarios, or a YAML file with
// --file. Flags override the scenario's settings. With --check, wabots
// exits with status 1 when a budget of strategy section 3.8 is broken.
//
// The procedural scenarios, fields and graph, place the bots through the
// database before they join, so they need --database or
// WATHENA_DATABASE_URL. Run them against a test world, never a live one:
// they change the bots' characters. Bots read generated maps' cells and
// portals from the server, as the web client does.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/buildinfo"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// databaseEnv holds the database URL, so the password stays out of
// process lists. It is waserver's variable.
const databaseEnv = "WATHENA_DATABASE_URL"

var stderr io.Writer = os.Stderr

// buildVersion is set by release builds with
// -ldflags "-X main.buildVersion=...", as scripts/release.sh does.
var buildVersion string

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "wabots:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println("wabots", buildinfo.Version(buildVersion))
		return nil
	}
	names := make([]string, 0, len(botclient.Scenarios))
	for n := range botclient.Scenarios {
		names = append(names, n)
	}
	slices.Sort(names)
	scenario := flag.String("scenario", "smoke", "scenario to run: "+strings.Join(names, ", "))
	file := flag.String("file", "", "YAML file with a scenario, instead of --scenario")
	url := flag.String("url", "ws://127.0.0.1:8300/ws", "the server's WebSocket URL")
	metricsURL := flag.String("metrics", "", "the server's --debug-addr as a URL, such as http://127.0.0.1:8301, to reset and read its measurements")
	assets := flag.String("assets", "", "asset build to read map cells from, so bots only walk to cells they can reach")
	bots := flag.Int("bots", 0, "how many bots join")
	ramp := flag.Float64("ramp", 0, "bots that join per second")
	warmup := flag.Duration("warmup", 0, "time after the last bot joined before measuring")
	duration := flag.Duration("duration", 0, "how long to measure")
	prefix := flag.String("prefix", "", "character name prefix; each bot adds its number")
	walkEvery := flag.Duration("walk-every", 0, "mean time between walks")
	walkRadius := flag.Int("walk-radius", 0, "how far a walk goes, in cells")
	chatRate := flag.Float64("chat-rate", 0, "chat lines per second of all bots together")
	fight := flag.Bool("fight", false, "attack monsters in view")
	stay := flag.Bool("stay", false, "walk around the cell where the bot entered its map")
	warpChance := flag.Float64("warp-chance", 0, "chance that a walk goes to a warp portal in view")
	seed := flag.Uint64("seed", 0, "seed for the bots' choices")
	database := flag.String("database", "", "PostgreSQL URL of the world, for the scenarios that place bots (fields, graph); prefer the "+databaseEnv+" environment variable")
	fieldCount := flag.Int("fields", 8, "how many fields the fields scenario spreads the bots over, from the lowest band to the highest")
	travel := flag.Bool("travel", false, "walk routes to the portals of generated maps")
	place := flag.String("place", "", "where to put the bots through the database before they join: fields or towns of a procedural world")
	jsonOut := flag.String("json", "", "file to write the report to as JSON")
	check := flag.Bool("check", false, "exit with status 1 when a budget of strategy section 3.8 is broken")
	flag.Parse()

	var sc botclient.Scenario
	if *file != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		if sc, err = parseScenario(data); err != nil {
			return fmt.Errorf("%s: %w", *file, err)
		}
	} else {
		var ok bool
		if sc, ok = botclient.Scenarios[*scenario]; !ok {
			return fmt.Errorf("unknown scenario %q", *scenario)
		}
		sc.Name = *scenario
	}
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "bots":
			sc.Bots = *bots
		case "ramp":
			sc.Ramp = *ramp
		case "warmup":
			sc.Warmup = *warmup
		case "duration":
			sc.Duration = *duration
		case "prefix":
			sc.NamePrefix = *prefix
		case "walk-every":
			sc.Behaviour.WalkEvery = *walkEvery
		case "walk-radius":
			sc.Behaviour.WalkRadius = *walkRadius
		case "chat-rate":
			sc.ChatRate = *chatRate
		case "fight":
			sc.Behaviour.Fight = *fight
		case "stay":
			sc.Behaviour.Stay = *stay
		case "warp-chance":
			sc.Behaviour.WarpChance = *warpChance
		case "seed":
			sc.Seed = *seed
		case "travel":
			sc.Behaviour.Travel = *travel
		case "place":
			sc.Place = *place
		}
	})
	if sc.URL == "" || isSet("url") {
		sc.URL = *url
	}
	if sc.MetricsURL == "" || isSet("metrics") {
		sc.MetricsURL = strings.TrimSuffix(*metricsURL, "/")
	}
	gen, err := newGenMaps(sc.URL)
	if err != nil {
		return err
	}
	var classic func(string) *path.Grid
	if *assets != "" {
		classic = grids(*assets)
	}
	sc.Behaviour.Grids = func(name string) *path.Grid {
		if strings.HasPrefix(name, "gen:") {
			return gen.grid(name)
		}
		if classic != nil {
			return classic(name)
		}
		return nil
	}
	sc.Behaviour.Portals = gen.portals
	sc.Progress = func(s string) { fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format("15:04:05"), s) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dbURL := *database
	if dbURL == "" {
		dbURL = os.Getenv(databaseEnv)
	}
	if sc.Place != "" || sc.Behaviour.Travel {
		if dbURL == "" {
			return fmt.Errorf("the scenario places bots or reads portals through the database: give --database or %s", databaseEnv)
		}
		db, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			return err
		}
		defer db.Close()
		p := &placer{db: db, maps: gen, fields: *fieldCount}
		if sc.Place != "" {
			if err := p.place(ctx, &sc, sc.Names(), sc.Progress); err != nil {
				return err
			}
		} else if _, err := p.load(ctx); err != nil {
			return err
		}
	}
	rep, err := sc.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Print(rep.Text())
	if *jsonOut != "" {
		data, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*jsonOut, data, 0o600); err != nil { // #nosec G703 -- the file the operator named.
			return err
		}
	}
	broken := rep.Check(botclient.Budgets)
	for _, b := range broken {
		fmt.Println("over budget:", b)
	}
	if len(broken) == 0 {
		fmt.Println("all budgets met")
	}
	if *check && len(broken) > 0 {
		return errors.New("budgets broken")
	}
	return nil
}

// parseScenario reads a scenario from YAML. Unknown fields are errors.
func parseScenario(data []byte) (botclient.Scenario, error) {
	var sc botclient.Scenario
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	err := dec.Decode(&sc)
	return sc, err
}

func isSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// grids loads maps' cells from an asset build the first time a bot asks.
// A map that cannot load is nil, and bots on it pick cells blindly.
func grids(dir string) func(string) *path.Grid {
	var mu sync.Mutex
	cache := map[string]*path.Grid{}
	return func(name string) *path.Grid {
		mu.Lock()
		defer mu.Unlock()
		g, ok := cache[name]
		if !ok {
			var err error
			if g, err = world.LoadGrid(dir, name); err != nil {
				fmt.Fprintln(os.Stderr, "wabots:", err)
			}
			cache[name] = g
		}
		return g
	}
}
