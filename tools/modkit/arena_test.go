package modkit

import (
	"context"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// TestArenaMarket runs the Arena Market sample mod's tests on both bases.
func TestArenaMarket(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	for _, base := range []string{mod.Renewal, mod.PreRenewal} {
		l, err := e.Load(context.Background(), "../../mods/examples/arena-market", "", base, nil)
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
