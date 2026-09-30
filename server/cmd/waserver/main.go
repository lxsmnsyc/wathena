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
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
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
	z := zone.New(name, grid)
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
	log.Info("serving", "map", name, "assets", "http://"+*addr+"/assets/", "websocket", "ws://"+*addr+"/ws")
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
