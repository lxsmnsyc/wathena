package modkit

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// TestFamiliars runs the Familiars sample mod's tests, then checks in Go
// what wa.testing cannot read: Owl Strike, whose requirements name the
// falcon, fails without a companion and hits with the Owl.
func TestFamiliars(t *testing.T) {
	e := env(t)
	l, err := e.Load(context.Background(), "../../mods/examples/familiars", "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(context.Background(), l, "", func(CaseResult) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}
	owlStrike := l.Skills.ByName("example.familiars:OWL_STRIKE").ID
	falconry := l.Skills.ByName("HT_FALCON").ID
	owl := l.Companions.Companions.ByName("example.familiars:Owl")
	if owl == nil || owl.ID < 10000 || !owl.Is("falcon") {
		t.Fatalf("owl %+v", owl)
	}
	logs := &capture{}
	w, err := e.newWorld(l, slog.New(logs))
	if err != nil {
		t.Fatal(err)
	}
	defer w.h.Close()
	vals, err := w.Do("new_player", []script.Value{script.NewTable(script.KV("map", script.String("prontera")),
		script.KV("x", script.Int(143)), script.KV("y", script.Int(192)), script.KV("job", script.Int(11)),
		script.KV("base_level", script.Int(70)), script.KV("job_level", script.Int(40)),
		script.KV("skills", script.List(script.List(script.Int(int64(falconry)), script.Int(1)), script.List(script.Int(int64(owlStrike)), script.Int(1)))))})
	if err != nil {
		t.Fatal(err)
	}
	hunter, _ := vals[0].AsInt()
	m, err := w.Do("spawn_monster", []script.Value{script.NewTable(script.KV("monster", script.Int(l.Zone.MonsterIDs["GOLEM"])),
		script.KV("x", script.Int(146)), script.KV("y", script.Int(192)))})
	if err != nil {
		t.Fatal(err)
	}
	golem, _ := m[0].AsInt()
	full, err := w.h.MonsterHP(golem)
	if err != nil {
		t.Fatal(err)
	}
	cast := func() int64 {
		if err := w.h.UseSkill(hunter, owlStrike, 1, golem); err != nil {
			t.Fatal(err)
		}
		w.h.Advance(1000)
		hp, _ := w.h.MonsterHP(golem)
		return hp
	}
	if hp := cast(); hp != full {
		t.Errorf("Owl Strike hit without a companion: HP %d of %d", hp, full)
	}
	// The keeper hands out the Owl.
	if said, err := w.h.Talk(hunter, "Keeper#prontera", "example.familiars", []script.Value{script.Int(1)}); err != nil {
		t.Fatalf("keeper: %v %q", err, said)
	}
	if hp := cast(); hp >= full {
		t.Errorf("Owl Strike did not hit with the Owl: HP %d of %d", hp, full)
	}
	if lines := logs.about("familiars"); len(lines) > 0 {
		t.Errorf("logs: %v", lines)
	}
}
