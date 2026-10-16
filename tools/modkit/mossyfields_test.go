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

// TestMossyFields runs the Mossy Fields sample mod's tests on both bases:
// its monsters with spawn tags, its spawn theme and its spawn_table hook,
// on the test world's field (decision 0042). The field's drawn list has
// the Moss Spore by its tags, and the hook's 12 Moss Porings come only
// when the case runs.
func TestMossyFields(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	for _, base := range []string{mod.Renewal, mod.PreRenewal} {
		l, err := e.Load(context.Background(), "../../mods/examples/mossy-fields", "", base, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(l.SpawnThemes, func(th spawns.Theme) bool { return th.Name == "example.mossy-fields:mossy" }) {
			t.Errorf("%s: no mossy theme in %v", base, l.SpawnThemes)
		}
		g, err := l.generatedMaps()
		if err != nil || g == nil {
			t.Fatalf("%s: generated maps: %v", base, err)
		}
		list := g.planned["gen:field:0:0"]
		if !slices.ContainsFunc(list, func(s spawns.Spawn) bool { return s.Monster == "example.mossy-fields:MOSS_SPORE" }) {
			t.Errorf("%s: the field's list %+v has no Moss Spore", base, list)
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
