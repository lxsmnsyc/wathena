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
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/progression"
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
	build := flag.String("build", "all_1", "recorded novice stat build players fight with: all_1, melee, critical, magic, ranged, or tank")
	level := flag.Int("level", 1, "recorded novice base level whose stats players fight with: 1, 50, or 99")
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
	var tables *progression.Tables
	var data *progression.Data
	if *mode != "none" {
		if tables, data, err = loadProgression(reference.Mode(*mode)); err != nil {
			return err
		}
	}
	spawns, err := loadSpawns(reference.Mode(*mode), strings.TrimPrefix(name, "map/"), tables, data)
	if err != nil {
		return err
	}
	root, err := rng.NewRoot(secret())
	if err != nil {
		return err
	}
	z := zone.New(name, grid, root.Child("zone", 0).SecretStream(), spawns)
	z.Progress = tables
	if *mode != "none" {
		if z.PlayerStatus, err = playerStatus(reference.Mode(*mode), *level, *build); err != nil {
			return err
		}
	} else {
		z.PlayerStatus = battle.Status{MaxHP: 40, ADelay: 880, AMotion: 440, DMotion: 796, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	}
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

// loadSpawns reads a map's monsters from the spawns and monster stats that
// reference/rathena recorded from rAthena, until the core mod has its own.
// With tables, monsters also get their DamageTaken, exp and drops.
func loadSpawns(mode reference.Mode, mapName string, tables *progression.Tables, data *progression.Data) ([]zone.Spawn, error) {
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
	var out []zone.Spawn
	for _, m := range spawns.Maps {
		if m.Map != mapName {
			continue
		}
		for _, s := range m.Spawns {
			st := byID[s.Mob]
			if st == nil {
				continue
			}
			out = append(out, zone.Spawn{
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

// playerStatus is a novice with no equipment and no skills, as the
// reference recorded, until characters have their own. The level and
// build pick a recorded novice. Only its battle stats are used, so a
// player still starts at level 1.
func playerStatus(mode reference.Mode, level int, build string) (battle.Status, error) {
	var pcs struct {
		Players []struct {
			Job         int           `json:"job"`
			BaseLevel   int           `json:"base_level"`
			Build       string        `json:"build"`
			AllSkills   bool          `json:"all_skills"`
			BattleState battle.Status `json:"battle_status"`
		} `json:"players"`
	}
	if err := reference.Load(mode, "pc_status", &pcs); err != nil {
		return battle.Status{}, err
	}
	for _, p := range pcs.Players {
		if p.Job == 0 && p.BaseLevel == level && p.Build == build && !p.AllSkills {
			st := p.BattleState
			st.Level = level
			// Bare hands and no skills.
			st.Player = &battle.Player{}
			return st, nil
		}
	}
	return battle.Status{}, fmt.Errorf("reference: no level %d %s novice in pc_status", level, build)
}
