package adminworld_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/restart"
	"github.com/lxsmnsyc/wathena/server/schedule"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// brokenMod is a mod whose setup fails, so each zone that runs it counts
// one error, and one that does not run it counts none.
const brokenMod = `
local events = require "wa.events"
return function()
  error("broken setup")
end`

// writeMod writes a mod folder and returns its loaded set and scripts.
func writeMod(t *testing.T, id, src string) (*mod.Set, *mod.Scripts) {
	t.Helper()
	dir := t.TempDir()
	m := filepath.Join(dir, id)
	if err := os.MkdirAll(filepath.Join(m, "scripts"), 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := "id: " + id + "\nname: Broken\nversion: 1.2.0\napi: \"1.0\"\nlicense: MIT\nscripts: [scripts/main.lua]\n"
	if err := os.WriteFile(filepath.Join(m, "mod.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m, "scripts", "main.lua"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := mod.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	set, err := mod.Resolve(found, mod.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := set.Scripts(id)
	if err != nil {
		t.Fatal(err)
	}
	return set, sc
}

// fakeRunner starts and ends runs without zones.
type fakeRunner struct {
	mu  sync.Mutex
	log []string
}

func (f *fakeRunner) Start(_ context.Context, d worldevent.Def, _ worldevent.Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, "start "+d.Name)
	return nil
}

func (f *fakeRunner) End(_ context.Context, d worldevent.Def, _ worldevent.Run) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, "end "+d.Name)
	return nil
}

func (f *fakeRunner) Condition(context.Context, worldevent.Def) (bool, error) { return false, nil }

var night = worldevent.Def{Name: "example.broken:Night", Mod: "example.broken", Duration: time.Hour,
	Trigger: worldevent.Trigger{Kind: worldevent.Clock, Spec: schedule.Spec{Kind: schedule.Daily, Hour: 20}}}

// toolsWorld is a running world of two zones with the broken mod, and
// its admin world with the tools, on a database. A restart makes a new
// one on the same database, as waserver does at start.
type toolsWorld struct {
	t       *testing.T
	ctx     context.Context
	admin   *pgxpool.Pool
	st      *store.Store
	w       *world.World
	aw      *adminworld.World
	sched   *worldevent.Scheduler
	runner  *fakeRunner
	planner *restart.Planner
	stop    func()
}

func startTools(t *testing.T, adminPool *pgxpool.Pool) *toolsWorld {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	game := storetest.As(t, adminPool, "wathena_game")
	st := store.New(game, store.Options{Log: slog.New(slog.DiscardHandler)})
	set, sc := writeMod(t, "example.broken", brokenMod)
	switches, err := store.LoadModSwitches(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	offAt := adminworld.OffAt(switches)
	var zones []*zone.Zone
	for i, name := range []string{"map/one", "map/two"} {
		z := zone.New(name, grid(), rng.FromKey([32]byte{byte(i + 1)}).Stream(), nil)
		z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300,
			RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
		z.Committer = committer{st}
		z.Mods = &zone.Mods{Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.broken"}, Log: slog.New(slog.DiscardHandler)}
		z.SetModsOff(append(append([]string(nil), offAt[""]...), offAt[name]...))
		zones = append(zones, z)
	}
	w, err := world.New("map/one", zones...)
	if err != nil {
		t.Fatal(err)
	}
	overrides, err := store.LoadEventOverrides(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	config := []worldevent.Setting{{Def: night, On: true}}
	settings, warnings := adminworld.ApplyOverrides(config, overrides)
	if len(warnings) > 0 {
		t.Fatalf("warnings %q", warnings)
	}
	runner := &fakeRunner{}
	sched := worldevent.NewFrom(settings, worldevent.Options{Runner: runner, Store: &worldevent.MemoryStore{}})
	if err := sched.Load(ctx, time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	planner := restart.New(restart.Options{})
	wctx, stop := context.WithCancel(context.Background())
	go w.Run(wctx)
	go sched.Run(wctx)
	tw := &toolsWorld{t: t, ctx: ctx, admin: adminPool, st: st, w: w, sched: sched, runner: runner, planner: planner}
	tw.aw = adminworld.New(adminworld.Config{World: w, Store: st, Log: slog.New(slog.DiscardHandler),
		Tools: adminworld.Tools{Metrics: metrics.NewRegistry(), Started: time.Now(), Version: "test", Restart: planner, Scheduler: sched,
			Events: config, Overrides: overrides, Mods: set, ScriptMods: []string{"example.broken"}, Switches: switches}})
	var once sync.Once
	tw.stop = func() {
		once.Do(func() {
			stop()
			<-w.Stopped()
			dctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := st.Close(dctx); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(tw.stop)
	return tw
}

// errorsByZone waits until the mod's error counts in the zones settle,
// and returns them by map.
func (tw *toolsWorld) errorsByZone(want map[string]int64) {
	tw.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var got map[string]int64
	for time.Now().Before(deadline) {
		list, err := tw.aw.Mods(tw.ctx)
		if err != nil {
			tw.t.Fatal(err)
		}
		got = map[string]int64{}
		for _, m := range list.GetMods() {
			if m.GetId() == "example.broken" {
				for _, z := range m.GetZones() {
					got[z.GetMap()] = z.GetErrors()
				}
			}
		}
		if equal(got, want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	tw.t.Fatalf("errors by zone %v, want %v", got, want)
}

func equal(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func order(m, mapName string, off bool, reason string) admin.ModOrder {
	return admin.ModOrder{Actor: admin.Actor{AccountID: 99, AccountName: "Admin1", Role: "admin"}, Mod: m, Map: mapName, Off: off, Reason: reason}
}

// TestModSwitchesSurviveRestart: a mod turned off in a zone is saved,
// runs no scripts there after a restart, and runs them once turned on.
func TestModSwitchesSurviveRestart(t *testing.T) {
	adminPool := storetest.Migrated(t)
	tw := startTools(t, adminPool)
	tw.errorsByZone(map[string]int64{"one": 1, "two": 1})
	list, err := tw.aw.Mods(tw.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var info *struct{ id, version, signature string }
	for _, m := range list.GetMods() {
		if m.GetId() == "example.broken" {
			info = &struct{ id, version, signature string }{m.GetId(), m.GetVersion(), m.GetSignature()}
			if !m.GetScripts() || m.GetErrors() != 2 || len(m.GetPending()) != 0 || !strings.Contains(m.GetZones()[0].GetLastError(), "broken setup") {
				t.Errorf("mod %v", m)
			}
		}
	}
	if info == nil || info.version != "1.2.0" || info.signature != adminworld.SignatureFolder || strings.Join(list.GetMaps(), ",") != "one,two" {
		t.Fatalf("mods %v", list)
	}
	if _, err := tw.aw.SwitchMod(tw.ctx, order("example.broken", "one", true, "quest gives too much")); err != nil {
		t.Fatal(err)
	}
	var we *admin.WorldError
	for _, c := range []struct {
		o    admin.ModOrder
		code string
	}{
		{order("example.broken", "one", true, "again"), "conflict"},
		{order("example.broken", "two", false, ""), "conflict"},
		{order("example.broken", "nowhere", true, "x"), "not_found"},
		{order("core", "", true, "x"), "bad_request"},
		{order("example.none", "", true, "x"), "not_found"},
	} {
		if _, err := tw.aw.SwitchMod(tw.ctx, c.o); !errors.As(err, &we) || we.Code != c.code {
			t.Errorf("%+v: %v", c.o, err)
		}
	}

	// After a restart, the mod's setup runs in zone two only.
	tw.stop()
	tw = startTools(t, adminPool)
	tw.errorsByZone(map[string]int64{"one": 0, "two": 1})
	// Turned on, its setup runs in zone one too.
	if _, err := tw.aw.SwitchMod(tw.ctx, order("example.broken", "one", false, "")); err != nil {
		t.Fatal(err)
	}
	tw.errorsByZone(map[string]int64{"one": 1, "two": 1})
	// Off everywhere, then on everywhere.
	if _, err := tw.aw.SwitchMod(tw.ctx, order("example.broken", "", true, "all broken")); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.aw.SwitchMod(tw.ctx, order("example.broken", "two", false, "")); !errors.As(err, &we) || we.Code != "conflict" {
		t.Errorf("on in one zone while off everywhere: %v", err)
	}
	if _, err := tw.aw.SwitchMod(tw.ctx, order("example.broken", "", false, "")); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.LoadModSwitches(tw.ctx, storetest.As(t, adminPool, "wathena_game")); len(got) != 0 {
		t.Errorf("switches after on everywhere: %+v", got)
	}
	var actions []string
	rows, err := adminPool.Query(tw.ctx, "SELECT action || ' ' || target || ' ' || (arguments->>'map') FROM staff_audit ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		actions = append(actions, s)
	}
	rows.Close()
	want := "mods.disable example.broken one|mods.enable example.broken one|mods.disable example.broken every zone|mods.enable example.broken every zone"
	if strings.Join(actions, "|") != want {
		t.Errorf("audit rows %q", actions)
	}
}

// TestEventOverridesSurviveRestart: staff changes of an event apply at
// once, are saved over the config with who made them, and apply again
// after a restart. A reset goes back to the config.
func TestEventOverridesSurviveRestart(t *testing.T) {
	adminPool := storetest.Migrated(t)
	tw := startTools(t, adminPool)
	actor := admin.Actor{AccountID: 99, AccountName: "Admin1", Role: "admin"}
	off := false
	trigger := map[string]any{"weekly": map[string]any{"day": "saturday", "time": float64(21)}}
	var we *admin.WorldError
	if _, err := tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventChange, Trigger: trigger}); !errors.As(err, &we) || we.Code != "bad_request" {
		t.Fatalf("a trigger that does not read: %v", err)
	}
	trigger["weekly"].(map[string]any)["time"] = "21:00"
	e, err := tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventChange, On: &off, Trigger: trigger,
		DurationMinutes: 90, Reason: "weekends only"})
	if err != nil {
		t.Fatal(err)
	}
	if e.GetOn() || e.GetTriggerText() != "weekly on saturday at 21:00" || e.GetDurationMinutes() != 90 || e.GetOverride().GetChangedBy() != "Admin1" ||
		!e.GetConfigOn() || e.GetConfigTriggerText() != "daily at 20:00" || e.HasNextRun() {
		t.Fatalf("after the change: %v", e)
	}
	// A second change keeps the first one's other fields.
	on := true
	if e, err = tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventChange, On: &on}); err != nil {
		t.Fatal(err)
	}
	if !e.GetOn() || e.GetDurationMinutes() != 90 || !e.HasNextRun() {
		t.Fatalf("after the second change: %v", e)
	}
	// Staff start and stop it.
	if _, err := tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventStart}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventStart}); !errors.As(err, &we) || we.Code != "conflict" {
		t.Errorf("a second start: %v", err)
	}
	list, err := tw.aw.Events(tw.ctx)
	if err != nil || len(list.GetEvents()) != 1 || len(list.GetEvents()[0].GetRuns()) != 1 {
		t.Fatalf("events while it runs: %v %v", list, err)
	}
	if _, err := tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventStop}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: "example.none:Day", Action: admin.EventStart}); !errors.As(err, &we) || we.Code != "not_found" {
		t.Errorf("an event that does not exist: %v", err)
	}

	// After a restart, the change applies again.
	tw.stop()
	tw = startTools(t, adminPool)
	list, err = tw.aw.Events(tw.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if e := list.GetEvents()[0]; !e.GetOn() || e.GetTriggerText() != "weekly on saturday at 21:00" || e.GetOverride().GetChangedBy() != "Admin1" {
		t.Fatalf("after the restart: %v", e)
	}
	if e, err = tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventReset}); err != nil {
		t.Fatal(err)
	}
	if e.GetTriggerText() != "daily at 20:00" || e.GetDurationMinutes() != 60 || e.HasOverride() {
		t.Fatalf("after the reset: %v", e)
	}
	if _, err := tw.aw.EventAction(tw.ctx, admin.EventOrder{Actor: actor, Name: night.Name, Action: admin.EventReset}); !errors.As(err, &we) || we.Code != "conflict" {
		t.Errorf("a second reset: %v", err)
	}
	var n int
	if err := adminPool.QueryRow(tw.ctx, "SELECT count(*) FROM staff_audit WHERE action LIKE 'events.%'").Scan(&n); err != nil || n != 5 {
		t.Errorf("audit rows %d %v", n, err)
	}
}

// TestWorldViewAndRestart: the world view lists the zones, the players
// and the planned restart, and announcements reach a map's players.
func TestWorldViewAndRestart(t *testing.T) {
	e := setup(t)
	planner := restart.New(restart.Options{})
	aw := adminworld.New(adminworld.Config{World: e.w, Store: e.st, Log: slog.New(slog.DiscardHandler),
		Tools: adminworld.Tools{Metrics: metrics.NewRegistry(), Started: time.Now().Add(-time.Hour), Version: "test", Restart: planner}})
	s := e.login(e.character("Alice"))
	defer s.Leave()
	v, err := aw.WorldView(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.GetZones()) != 2 || v.GetZones()[0].GetMap() != "one" || v.GetZones()[0].GetPlayers() != 1 || !v.GetZones()[1].GetAsleep() {
		t.Errorf("zones %v", v.GetZones())
	}
	if v.GetPlayersOnline() != 1 || v.GetPlayers()[0].GetName() != "Alice" || v.GetPlayers()[0].GetMap() != "one" {
		t.Errorf("players %v", v.GetPlayers())
	}
	if p := v.GetProcess(); p.GetVersion() != "test" || p.GetUptimeSeconds() < 3599 || p.GetGoroutines() == 0 {
		t.Errorf("process %v", p)
	}
	if err := aw.Announce(e.ctx, "Hello one", "one"); err != nil {
		t.Fatal(err)
	}
	if err := aw.Announce(e.ctx, "Hello", "nowhere"); !errors.Is(err, admin.ErrNotFound) {
		t.Errorf("a map that does not run: %v", err)
	}
	p, err := aw.PlanRestart(e.ctx, admin.RestartOrder{Actor: admin.Actor{AccountName: "Admin1"}, At: time.Now().Add(time.Hour), Reason: "update"})
	if err != nil || p.GetPlannedBy() != "Admin1" || p.GetLoginsClosed() {
		t.Fatalf("plan %v %v", p, err)
	}
	var we *admin.WorldError
	if _, err := aw.PlanRestart(e.ctx, admin.RestartOrder{At: time.Now().Add(2 * time.Hour), Reason: "again"}); !errors.As(err, &we) || we.Code != "conflict" {
		t.Errorf("a second plan: %v", err)
	}
	if v, _ := aw.WorldView(e.ctx); v.GetRestart().GetReason() != "update" {
		t.Errorf("the world view has no restart: %v", v.GetRestart())
	}
	if _, err := aw.CancelRestart(e.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := aw.CancelRestart(e.ctx); !errors.As(err, &we) || we.Code != "not_found" {
		t.Errorf("a second cancel: %v", err)
	}
	if _, err := aw.PlanRestart(e.ctx, admin.RestartOrder{At: time.Now().Add(10 * time.Second), Reason: "soon"}); !errors.As(err, &we) || we.Code != "bad_request" {
		t.Errorf("a plan too soon: %v", err)
	}
}
