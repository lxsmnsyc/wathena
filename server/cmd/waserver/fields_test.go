package main

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestFieldsConfig: --fields, --field-size and --tilesets, from the file
// and the flags, and their checks.
func TestFieldsConfig(t *testing.T) {
	assets := assetDir(t)
	p := writeConfig(t, `
assets: `+assets+`
dev: true
procedural:
  fields: ["0:0", "1:0:desert"]
  field_size: 128
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p, "--field-size", "96"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(o.Procedural.Fields, []string{"0:0", "1:0:desert"}) || o.Procedural.FieldSize != 96 {
		t.Errorf("procedural %+v", o.Procedural)
	}
	if _, err := validate(o, secrets{}); err != nil {
		t.Error(err)
	}
	o, _, err = parseOptions("waserver all", []string{"--assets", assets, "--dev", "--fields", "2:3,4:x", "--field-size", "8", "--tilesets", "/no/such/file"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	_, err = validate(o, secrets{})
	for _, want := range []string{"procedural.field_size", "procedural.fields", "procedural.tilesets"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("no %s problem in %v", want, err)
		}
	}
	if defaultOptions().Procedural.FieldSize != 300 {
		t.Error("the default field size is not 300")
	}
}

// TestFieldMaps: which worlds run fields, and where their nodes come
// from (decision 0039).
func TestFieldMaps(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := settings{options: defaultOptions()}
	cfg.Dev = true
	cfg.Procedural.FieldSize = 48

	// A classic world, or none, without --fields runs no fields.
	if p, err := fieldMaps(cfg, store.World{}, log); err != nil || p.set != nil || p.mode.Procedural {
		t.Errorf("no fields: %+v %v", p, err)
	}
	cfg.Procedural.Fields = list{"0:0", "1:0"}
	classic := store.World{Name: "C", Mode: gen.ModeClassic}
	if _, err := fieldMaps(cfg, classic, log); err == nil || !strings.Contains(err.Error(), "procedural world") {
		t.Errorf("a classic world with fields: %v", err)
	}
	p, err := fieldMaps(cfg, store.World{}, log)
	if !gen.Procedural {
		if err == nil || !strings.Contains(err.Error(), "-tags procedural") {
			t.Errorf("fields without the tag: %v", err)
		}
		return
	}
	if err != nil || p.set == nil || len(p.set.Fields()) != 2 || !p.mode.Procedural {
		t.Fatalf("dev fields: %+v %v", p, err)
	}
	latest, _ := gen.Latest()
	if p.mode.GeneratorVersion != latest.Version || p.mode.RNGVersion != rng.Version {
		t.Errorf("dev mode %+v", p.mode)
	}
	// The dev world's fields follow --seed.
	root, _ := rng.NewRoot([]byte(devWorldLabel))
	if got, want := p.set.Get("gen:field:1:0").Key, gen.FieldNode(gen.PublicBranch(root), 1, 0).Key(); got != want {
		t.Errorf("dev key %x, want %x", got, want)
	}
	noDev := cfg
	noDev.Dev = false
	if _, err := fieldMaps(noDev, store.World{}, log); err == nil {
		t.Error("fields without --dev or a world ran")
	}

	// A procedural world row: fields from its public node, 0:0 by
	// default. The seed is never read.
	public := gen.PublicBranch(rng.FromKey([32]byte{4}))
	key := public.Key()
	row := store.World{Name: "P", Mode: gen.ModeProcedural, GeneratorVersion: 2, RNGVersion: rng.Version, PublicNode: key[:]}
	cfg.Procedural.Fields = nil
	p, err = fieldMaps(cfg, row, log)
	if err != nil || len(p.set.Fields()) != 1 || p.set.Fields()[0].Key != gen.FieldNode(public, 0, 0).Key() || p.mode.GeneratorVersion != 2 {
		t.Fatalf("world fields: %+v %v", p, err)
	}
	row.PublicNode = nil
	if _, err := fieldMaps(cfg, row, log); err == nil || !strings.Contains(err.Error(), "public node") {
		t.Errorf("a world without a public node: %v", err)
	}
	row.PublicNode, row.GeneratorVersion = key[:], 1
	if _, err := fieldMaps(cfg, row, log); err == nil || !strings.Contains(err.Error(), "no fields") {
		t.Errorf("generator 1: %v", err)
	}

	// worldMaps: the first field starts unless --map names a field, and
	// asset maps run only when --maps lists them.
	cfg.Procedural.Fields = list{"0:0", "1:0"}
	p, _ = fieldMaps(cfg, store.World{}, log)
	start, assetMaps, all, err := worldMaps(cfg, p)
	if err != nil || start != "gen:field:0:0" || len(assetMaps) != 0 || !slices.Equal(all, []string{"gen:field:0:0", "gen:field:1:0"}) {
		t.Errorf("maps: %s %v %v %v", start, assetMaps, all, err)
	}
	cfg.StartMap, cfg.Maps = "gen:field:1:0", list{"placeholder/map"}
	start, assetMaps, all, err = worldMaps(cfg, p)
	if err != nil || start != "gen:field:1:0" || !slices.Equal(assetMaps, []string{"placeholder/map"}) || len(all) != 3 {
		t.Errorf("maps: %s %v %v %v", start, assetMaps, all, err)
	}
	if g := fieldGrids(p); len(g) != 2 || g[0].Width != 48 || g[1].Height != 48 {
		t.Error("field grids")
	}
}
