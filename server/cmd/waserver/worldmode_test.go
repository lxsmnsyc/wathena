package main

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestCheckWorldMode: a build runs classic worlds, and procedural worlds
// only with the procedural tag and a generator it has (decision 0038).
func TestCheckWorldMode(t *testing.T) {
	classic := store.World{Name: "C", Mode: gen.ModeClassic}
	if err := checkWorldMode(classic); err != nil {
		t.Errorf("classic: %v", err)
	}
	if m := worldMode(classic); m.Procedural {
		t.Errorf("classic sends %+v", m)
	}
	if m := worldMode(store.World{}); m.Procedural {
		t.Errorf("a dev server without a world sends %+v", m)
	}
	if err := checkWorldMode(store.World{Name: "X", Mode: "other"}); err == nil {
		t.Error("an unknown mode ran")
	}
	proc := store.World{Name: "P", Mode: gen.ModeProcedural, GeneratorVersion: 1, RNGVersion: rng.Version}
	err := checkWorldMode(proc)
	if !gen.Procedural {
		if err == nil || !strings.Contains(err.Error(), "-tags procedural") {
			t.Errorf("procedural without the tag: %v", err)
		}
		return
	}
	if err != nil {
		t.Errorf("procedural: %v", err)
	}
	if m := worldMode(proc); !m.Procedural || m.GeneratorVersion != 1 || m.RNGVersion != rng.Version {
		t.Errorf("procedural sends %+v", m)
	}
	for _, bad := range []store.World{
		{Name: "P", Mode: gen.ModeProcedural, GeneratorVersion: 999, RNGVersion: rng.Version},
		{Name: "P", Mode: gen.ModeProcedural, GeneratorVersion: 1, RNGVersion: "rng-v9"},
	} {
		if err := checkWorldMode(bad); err == nil {
			t.Errorf("%+v ran", bad)
		}
	}
}
