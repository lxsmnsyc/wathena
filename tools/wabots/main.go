// Command wabots runs headless bots against a game server and reports
// what they and the server measured (server/README.md, "Load tests and
// metrics").
//
//	wabots --scenario town --assets <build> --metrics http://127.0.0.1:8301
//
// A scenario is one of botclient.Scenarios, or a YAML file with
// --file. Flags override the scenario's settings. With --check, wabots
// exits with status 1 when a budget of strategy section 3.8 is broken.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "wabots:", err)
		os.Exit(1)
	}
}

func run() error {
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
		}
	})
	if sc.URL == "" || isSet("url") {
		sc.URL = *url
	}
	if sc.MetricsURL == "" || isSet("metrics") {
		sc.MetricsURL = strings.TrimSuffix(*metricsURL, "/")
	}
	if *assets != "" {
		sc.Behaviour.Grids = grids(*assets)
	}
	sc.Progress = func(s string) { fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format("15:04:05"), s) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
