package variables

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Temporary values keep the mods' limits, core has no count limit, and
// an NPC's values go with it (review decision 58).
func TestTemporaryLimits(t *testing.T) {
	v := NewTemporary()
	for i := range MaxModValues {
		if err := v.Set(TempKey{Mod: "example.a", Name: fmt.Sprintf("v%d", i)}, "1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.Set(TempKey{Mod: "example.a", NPC: "example.a:Guard", Name: "more"}, "1"); err == nil || !strings.Contains(err.Error(), "the most a mod may") {
		t.Fatalf("err = %v, want the count limit", err)
	}
	for i := range MaxModValues + 10 {
		if err := v.Set(TempKey{Mod: Core, NPC: "core:Guard", Name: fmt.Sprintf("v%d", i)}, "1"); err != nil {
			t.Fatalf("core value %d: %v", i, err)
		}
	}
	v.DropNPC("core:Guard")
	if _, ok := v.Get(TempKey{Mod: Core, NPC: "core:Guard", Name: "v0"}); ok {
		t.Fatal("an NPC's value stayed after DropNPC")
	}
	if err := v.Set(TempKey{Mod: Core, Name: "x"}, "2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := v.Get(TempKey{Mod: Core, Name: "x"}); got != "2" {
		t.Fatalf("x = %q, want 2", got)
	}
}

// Server variables of core have no count limit either.
func TestServerCoreLimit(t *testing.T) {
	s := NewServer(nil, nil)
	for i := range MaxModValues + 10 {
		if err := s.Set(Key{Mod: Core, Name: fmt.Sprintf("v%d", i)}, "1"); err != nil {
			t.Fatalf("core value %d: %v", i, err)
		}
	}
}

// Zones update one value at a time: no add is lost.
func TestTemporaryUpdateRace(t *testing.T) {
	v := NewTemporary()
	k := TempKey{Mod: Core, NPC: "core:Counter", Name: "n"}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				_, _ = v.Update(k, func(old string) (string, error) {
					n := 0
					_, _ = fmt.Sscan(old, &n)
					return fmt.Sprint(n + 1), nil
				})
			}
		})
	}
	wg.Wait()
	if got, _ := v.Get(k); got != "800" {
		t.Fatalf("n = %s, want 800", got)
	}
}
