package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

func TestCheckWorld(t *testing.T) {
	admin := storetest.Migrated(t)
	game := storetest.As(t, admin, "wathena_game")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := settings{options: options{Mode: "renewal"}}
	if _, err := checkWorld(ctx, game, cfg); err == nil || !strings.Contains(err.Error(), "wactl world new") {
		t.Errorf("no world: %v", err)
	}
	cfg.Dev = true
	if name, err := checkWorld(ctx, game, cfg); err != nil || name != "" {
		t.Errorf("no world with dev: %q, %v", name, err)
	}
	cfg.Dev = false

	seed := make([]byte, 32)
	if _, err := store.NewWorld(ctx, admin, store.World{Name: "Test", Ruleset: "renewal", Seed: seed}, store.Admin{Name: "Admin1", Hash: "x"}); err != nil {
		t.Fatal(err)
	}
	if name, err := checkWorld(ctx, game, cfg); err != nil || name != "Test" {
		t.Errorf("world: %q, %v", name, err)
	}
	cfg.Mode = "pre-renewal"
	if _, err := checkWorld(ctx, game, cfg); err == nil {
		t.Error("a pre-renewal server started on a renewal world")
	}
	cfg.Mode = "none"
	if _, err := checkWorld(ctx, game, cfg); err != nil {
		t.Errorf("mode none: %v", err)
	}
}
