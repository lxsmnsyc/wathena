// Command waserver runs a development game server in one process: it
// serves an asset build over HTTP and one zone over WebSocket.
//
//	waserver --assets <build> --map prontera --dev
//
// There is no account service yet, so --dev accepts any valid name as a
// session token. Never expose a --dev server to the internet.
package main

import (
	"context"
	crand "crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/serve"
	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
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
	mapName := flag.String("map", "prontera", "map to run, such as prontera or placeholder/map")
	addr := flag.String("addr", "127.0.0.1:8300", "address to listen on")
	dev := flag.Bool("dev", false, "accept any name as a session token, for development")
	origins := flag.String("origins", "localhost:*,127.0.0.1:*", "hosts pages may connect from, comma-separated")
	mode := flag.String("mode", "renewal", "rAthena mode whose recorded spawns to use: renewal or pre-renewal, or none")
	flag.Parse()
	if *assets == "" {
		return errors.New("--assets is required")
	}
	if !*dev {
		return errors.New("only --dev is supported until the account service exists")
	}
	name := *mapName
	if !strings.Contains(name, "/") {
		name = "map/" + name
	}
	grid, err := loadGrid(*assets, name)
	if err != nil {
		return err
	}
	files, err := serve.New(*assets)
	if err != nil {
		return err
	}
	defer func() { _ = files.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	spawns, err := loadSpawns(reference.Mode(*mode), strings.TrimPrefix(name, "map/"))
	if err != nil {
		return err
	}
	root, err := rng.NewRoot(secret())
	if err != nil {
		return err
	}
	z := zone.New(name, grid, root.Child("zone", 0).SecretStream(), spawns)
	go z.Run(ctx)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets", files))
	mux.Handle("/ws", &gateway.Gateway{Zone: z, Sessions: gateway.DevSessions{}, Origins: strings.Split(*origins, ","), Log: log})
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("serving", "map", name, "monsters", count(spawns), "assets", "http://"+*addr+"/assets/", "websocket", "ws://"+*addr+"/ws")
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// loadGrid reads a map's cells from the asset build.
func loadGrid(dir, name string) (*path.Grid, error) {
	data, typ, err := build.ReadAsset(dir, name)
	if err != nil {
		return nil, err
	}
	if typ != "map" {
		return nil, fmt.Errorf("%s is a %s, not a map", name, typ)
	}
	m, err := wmap.Decode(data)
	if err != nil {
		return nil, err
	}
	g := &path.Grid{Width: m.Width, Height: m.Height, Cells: make([]uint8, len(m.Cells))}
	for i, c := range m.Cells {
		g.Cells[i] = c.Flags & (path.Walkable | path.Shootable)
	}
	return g, nil
}

// loadSpawns reads a map's monsters from the spawns and monster stats that
// reference/rathena recorded from rAthena, until the core mod has its own.
func loadSpawns(mode reference.Mode, mapName string) ([]zone.Spawn, error) {
	if mode == "none" {
		return nil, nil
	}
	var spawns struct {
		Maps []struct {
			Map    string `json:"map"`
			Spawns []struct {
				Mob    uint32 `json:"mob"`
				Name   string `json:"name"`
				Amount int    `json:"amount"`
				Area   [4]int `json:"area"`
			} `json:"spawns"`
		} `json:"maps"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/spawns.json", &spawns); err != nil {
		return nil, err
	}
	var mobs struct {
		Mobs []struct {
			ID     uint32 `json:"id"`
			Status struct {
				Speed int    `json:"speed"`
				Mode  uint32 `json:"mode"`
			} `json:"status"`
		} `json:"mobs"`
	}
	if err := reference.Load(mode, "mob_status", &mobs); err != nil {
		return nil, err
	}
	type stats struct {
		speed   int
		canMove bool
	}
	byID := map[uint32]stats{}
	for _, m := range mobs.Mobs {
		// Mode bit 1 is MD_CANMOVE.
		byID[m.ID] = stats{m.Status.Speed, m.Status.Mode&1 != 0}
	}
	var out []zone.Spawn
	for _, m := range spawns.Maps {
		if m.Map != mapName {
			continue
		}
		for _, s := range m.Spawns {
			st := byID[s.Mob]
			out = append(out, zone.Spawn{
				Mob: s.Mob, Name: s.Name, Amount: s.Amount,
				// rAthena searches up to xs - 1 cells from the center.
				X: s.Area[0], Y: s.Area[1], W: max(s.Area[2]-1, 0), H: max(s.Area[3]-1, 0),
				Speed: st.speed, CanMove: st.canMove,
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
