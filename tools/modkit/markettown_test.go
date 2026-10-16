//go:build procedural

package modkit

import (
	"context"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// TestMarketTown runs the Market Town sample mod's tests on both bases:
// its town template, its dungeon style and its NPCs placed by tag, on
// the test world's generated maps (decision 0040).
func TestMarketTown(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	for _, base := range []string{mod.Renewal, mod.PreRenewal} {
		l, err := e.Load(context.Background(), "../../mods/examples/market-town", "", base, nil)
		if err != nil {
			t.Fatal(err)
		}
		results, err := e.RunTests(context.Background(), l, "", func(CaseResult) {})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 4 {
			t.Errorf("%s: %d cases", base, len(results))
		}
		for _, r := range results {
			if r.Failed() {
				t.Errorf("%s: %s: %v %v", base, r.Name, r.Err, r.Logs)
			}
		}
	}
}

// TestGeneratedTestWorld: a test world has a field, a town per template
// and a dungeon per style, and the NPCs placed by tag stand at their
// places. A map it lacks is an error.
func TestGeneratedTestWorld(t *testing.T) {
	testrace.SkipUnderRace(t, "it generates maps on one goroutine")
	e := env(t)
	l, err := e.Load(context.Background(), "../../mods/examples/market-town", "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	g, err := l.generatedMaps()
	if err != nil || g == nil {
		t.Fatalf("generated maps: %v", err)
	}
	for _, id := range []string{"gen:field:0:0", "gen:town:t1", "gen:town:t4", "gen:dungeon:crypt:3", "gen:dungeon:cellar:2"} {
		if g.set.Get(id) == nil {
			t.Errorf("no %s", id)
		}
	}
	guides := 0
	for _, list := range g.npcs {
		for _, n := range list {
			if strings.HasSuffix(n.Name, "Town Guide") {
				guides++
			}
		}
	}
	if guides != 4 {
		t.Errorf("%d town guides, want one per town", guides)
	}
	w, err := e.newWorld(l, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.h.Close()
	if err := w.bind("gen:town:t9"); err == nil || !strings.Contains(err.Error(), "no generated map gen:town:t9") {
		t.Errorf("bind t9: %v", err)
	}
}
