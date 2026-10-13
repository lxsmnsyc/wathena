package store_test

import (
	"context"
	"testing"

	"github.com/lxsmnsyc/wathena/server/store"
)

// TestEventOverrides: staff changes of world events are saved over the
// config with who made them, and a reset deletes them, each with its
// audit row (decision 0031).
func TestEventOverrides(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	audit := func(action string) store.AuditRow {
		return store.AuditRow{AccountID: 7, AccountName: "Admin", Role: "admin", Action: action, Source: "admin", Target: "example.a:Night"}
	}
	off := false
	o := store.EventOverride{Event: "example.a:Night", On: &off, Trigger: map[string]any{"daily": "21:00"}, DurationMinutes: 30,
		By: "Admin", ByID: 7}
	if err := s.SetEventOverride(ctx, o, audit("events.change")); err != nil {
		t.Fatal(err)
	}
	// A second change replaces the first.
	o.On, o.Trigger = nil, map[string]any{"hourly": map[string]any{"minute": 5}}
	if err := s.SetEventOverride(ctx, o, audit("events.change")); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadEventOverrides(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].On != nil || got[0].DurationMinutes != 30 || got[0].By != "Admin" || got[0].At.IsZero() {
		t.Fatalf("overrides %+v", got)
	}
	if h, ok := got[0].Trigger["hourly"].(map[string]any); !ok || h["minute"] != float64(5) {
		t.Fatalf("trigger %v", got[0].Trigger)
	}
	if found, err := s.ResetEventOverride(ctx, "example.a:Night", audit("events.reset")); err != nil || !found {
		t.Fatalf("reset: %v %v", found, err)
	}
	if found, err := s.ResetEventOverride(ctx, "example.a:Night", audit("events.reset")); err != nil || found {
		t.Fatalf("a second reset: %v %v", found, err)
	}
	if got, _ := store.LoadEventOverrides(ctx, game); len(got) != 0 {
		t.Fatalf("after the reset: %+v", got)
	}
	var n int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM staff_audit WHERE action LIKE 'events.%'").Scan(&n); err != nil || n != 3 {
		t.Fatalf("audit rows %d %v", n, err)
	}
}

// TestModSwitches: mods turned off by zone or everywhere, and on again,
// with their audit rows.
func TestModSwitches(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	audit := store.AuditRow{AccountID: 7, AccountName: "Admin", Role: "admin", Action: "mods.disable", Source: "admin", Target: "example.a"}
	sw := func(m string) store.ModSwitch {
		return store.ModSwitch{Mod: "example.a", Map: m, Reason: "broken quest", By: "Admin", ByID: 7}
	}
	load := func() []store.ModSwitch {
		t.Helper()
		got, err := store.LoadModSwitches(ctx, game)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	for _, m := range []string{"map/prontera", "map/geffen"} {
		if err := s.SwitchMod(ctx, sw(m), true, audit); err != nil {
			t.Fatal(err)
		}
	}
	if got := load(); len(got) != 2 || got[0].Map != "map/geffen" || got[0].Reason != "broken quest" {
		t.Fatalf("by zone: %+v", got)
	}
	if err := s.SwitchMod(ctx, sw("map/geffen"), false, audit); err != nil {
		t.Fatal(err)
	}
	if got := load(); len(got) != 1 || got[0].Map != "map/prontera" {
		t.Fatalf("after one zone on: %+v", got)
	}
	// Off everywhere replaces the zones' rows.
	if err := s.SwitchMod(ctx, sw(""), true, audit); err != nil {
		t.Fatal(err)
	}
	if got := load(); len(got) != 1 || got[0].Map != "" {
		t.Fatalf("everywhere: %+v", got)
	}
	if err := s.SwitchMod(ctx, sw(""), false, audit); err != nil {
		t.Fatal(err)
	}
	if got := load(); len(got) != 0 {
		t.Fatalf("after on everywhere: %+v", got)
	}
	// Core cannot be turned off, even in the database.
	core := sw("")
	core.Mod = "core"
	if err := s.SwitchMod(ctx, core, true, audit); err == nil {
		t.Fatal("core was turned off")
	}
	var n int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM staff_audit WHERE action = 'mods.disable'").Scan(&n); err != nil || n != 5 {
		t.Fatalf("audit rows %d %v", n, err)
	}
}

// TestWorldToolsRights: the account role has no rights on the tools'
// tables.
func TestWorldToolsRights(t *testing.T) {
	admin, _ := migrated(t)
	ctx := timeout(t)
	for _, q := range []string{"SELECT * FROM event_overrides", "SELECT * FROM mod_switches"} {
		tx, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE wathena_account"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, q); err == nil {
			t.Errorf("the account role ran %s", q)
		}
		_ = tx.Rollback(context.Background())
	}
}
