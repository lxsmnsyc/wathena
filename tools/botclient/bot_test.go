package botclient

import (
	"context"
	"math/rand/v2"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func open(size int) *path.Grid {
	g := &path.Grid{Width: size, Height: size, Cells: make([]uint8, size*size)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	return g
}

type server struct {
	ws, debug string
	reg       *metrics.Registry
	grid      *path.Grid
}

// serve runs one zone with a gateway and a metrics registry.
func serve(t *testing.T, spawns []zone.Spawn) *server {
	t.Helper()
	g := open(40)
	z := zone.New("map/test", g, rng.FromKey([32]byte{3}).Stream(), spawns)
	z.PlayerStatus = battle.Status{MaxHP: 1000, Hit: 200, BAtk: 50, ADelay: 300, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	reg := metrics.NewRegistry()
	reg.AddZone(z.Metrics)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	go reg.Conns.Run(ctx)
	s := httptest.NewServer(&gateway.Gateway{World: z, Sessions: gateway.DevSessions{}, Metrics: &reg.Conns})
	t.Cleanup(s.Close)
	d := httptest.NewServer(reg.Handler())
	t.Cleanup(d.Close)
	return &server{ws: "ws" + strings.TrimPrefix(s.URL, "http"), debug: d.URL, reg: reg, grid: g}
}

func dial(t *testing.T, s *server, name string) *Bot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := Dial(ctx, s.ws, []byte(name), Options{Ack: &metrics.Histogram{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// eventually waits for a condition.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestWalkAndChat(t *testing.T) {
	s := serve(t, nil)
	a := dial(t, s, "Alice")
	b := dial(t, s, "Bob")
	ctx := context.Background()
	eventually(t, "Bob in Alice's view", func() bool { return len(a.View().Entities) == 1 })
	v := a.View()
	if v.Map != "map/test" || v.X == 0 || v.Y == 0 {
		t.Fatalf("Alice's view: %+v", v)
	}
	if err := a.WalkTo(ctx, v.X+3, v.Y+2); err != nil {
		t.Fatal(err)
	}
	// Alice follows her own walk, and Bob sees where it ends.
	eventually(t, "the walk", func() bool {
		return bobSees(b, "Alice").DestX == v.X+3 && !a.View().Walking
	})
	end := a.View()
	if end.X != v.X+3 || end.Y != v.Y+2 {
		t.Errorf("Alice is at %d,%d, want %d,%d", end.X, end.Y, v.X+3, v.Y+2)
	}
	if err := a.Chat(ctx, chatLine("Alice", 1, 40)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the chat line", func() bool { return b.Counts().ChatsHeard == 1 && a.Counts().ChatsHeard == 1 })
	if a.ack.Summary().Count != 2 {
		t.Errorf("%d acknowledged commands, want 2", a.ack.Summary().Count)
	}

	// The bots and the server count the same bytes once nothing is in
	// flight.
	time.Sleep(200 * time.Millisecond)
	snap := s.reg.Conns.Snapshot()
	ca, cb := a.Counts(), b.Counts()
	if snap.BytesOut != ca.BytesIn+cb.BytesIn || snap.BytesIn != ca.BytesOut+cb.BytesOut {
		t.Errorf("server counted %d out and %d in, bots %d in and %d out", snap.BytesOut, snap.BytesIn, ca.BytesIn+cb.BytesIn, ca.BytesOut+cb.BytesOut)
	}
}

func bobSees(b *Bot, name string) Entity {
	for _, e := range b.View().Entities {
		if e.Name == name {
			return e
		}
	}
	return Entity{}
}

func TestFight(t *testing.T) {
	mob := &battle.Status{MaxHP: 100, Hit: 1, ADelay: 2000, AMotion: 500, DMotion: 300, RHW: battle.Weapon{Atk: 1, Range: 1}}
	s := serve(t, []zone.Spawn{{Mob: 1002, Name: "Poring", Amount: 3, X: 20, Y: 20, W: 3, H: 3, Speed: 400, Status: mob, RespawnMs: 1000}})
	b := dial(t, s, "Fighter")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- b.Play(ctx, Behaviour{Fight: true, WalkEvery: time.Second, Grids: func(string) *path.Grid { return s.grid }}, rand.New(rand.NewPCG(1, 2)))
	}()
	eventually(t, "a kill", func() bool { return b.Counts().Kills > 0 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestScenario(t *testing.T) {
	s := serve(t, nil)
	sc := Scenario{
		Name: "test", URL: s.ws, MetricsURL: s.debug, Bots: 10, Ramp: 50,
		Warmup: 500 * time.Millisecond, Duration: 2 * time.Second,
		Behaviour: Behaviour{WalkEvery: 500 * time.Millisecond, ChatEvery: time.Second, Stay: true, Grids: func(string) *path.Grid { return s.grid }},
	}
	rep, err := sc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + rep.Text())
	if rep.Joined != 10 || rep.Maps["map/test"] != 10 {
		t.Errorf("joined %d, maps %v", rep.Joined, rep.Maps)
	}
	if rep.Ack.Count == 0 || rep.ChatsHeard == 0 || rep.Out.Mean == 0 || rep.In.Mean == 0 {
		t.Errorf("report: %+v", rep)
	}
	if rep.Server == nil || len(rep.Server.Zones) != 1 || rep.Server.Zones[0].Players != 10 || rep.Server.Conns.Live != 10 {
		t.Fatalf("server: %+v", rep.Server)
	}
	if v := rep.Check(Limits{Errors: 0}); len(v) > 0 {
		t.Error(v)
	}
}
