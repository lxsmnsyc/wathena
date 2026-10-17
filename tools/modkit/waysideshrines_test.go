//go:build procedural

package modkit

import (
	"context"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/spawns"
)

// TestWaysideShrines runs the Wayside Shrines sample mod's tests on both
// bases: its generation layer's places, objects and boss, and its quest
// template, in the test world (decision 0044). The field's ring of
// stones reaches the map's info as objects, and the Vagabond Wolf joins
// the field's spawns after its hooks.
func TestWaysideShrines(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	for _, base := range []string{mod.Renewal, mod.PreRenewal} {
		l, err := e.Load(context.Background(), "../../mods/examples/wayside-shrines", "", base, nil)
		if err != nil {
			t.Fatal(err)
		}
		g, err := l.generatedMaps()
		if err != nil || g == nil {
			t.Fatalf("%s: generated maps: %v", base, err)
		}
		if len(g.notes) > 0 {
			t.Errorf("%s: notes %v", base, g.notes)
		}
		field := g.set.Get("gen:field:0:0")
		if info := field.Info(); len(info.Objects) != 4 || info.Objects[0].Model == "" {
			t.Errorf("%s: the field's objects %+v", base, info.Objects)
		}
		if !slices.ContainsFunc(g.bosses["gen:field:0:0"], func(s spawns.Spawn) bool {
			return s.Monster == "VAGABOND_WOLF" && s.Role == spawns.MiniBoss && s.Count == 1
		}) {
			t.Errorf("%s: the field's bosses %+v", base, g.bosses["gen:field:0:0"])
		}
		results, err := e.RunTests(context.Background(), l, "", func(CaseResult) {})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 3 {
			t.Errorf("%s: %d cases", base, len(results))
		}
		for _, r := range results {
			if r.Failed() {
				t.Errorf("%s: %s: %v %v", base, r.Name, r.Err, r.Logs)
			}
		}
	}
}
