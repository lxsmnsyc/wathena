package mod

import (
	"strings"
	"testing"
)

// A manifest's budget, the owner's default and the owner's limits
// (review decision 52).
func TestBudgets(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "plain", map[string]string{"mod.yaml": manifest("example.plain")})
	writeMod(t, dir, "big", map[string]string{"mod.yaml": manifest("example.big", "budget: {memory_mb: 48, cpu_per_tick: 5000000}")})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if b := s.Mod("example.plain").Budget; b != (Budget{MemoryMB: DefaultMemoryMB, CPUPerTick: DefaultCPUPerTick}) {
		t.Errorf("plain budget = %+v", b)
	}
	if b := s.Mod("example.big").Budget; b != (Budget{MemoryMB: 48, CPUPerTick: 5_000_000}) {
		t.Errorf("big budget = %+v", b)
	}
	if b := s.Mod(Core).Budget; b != (Budget{}) {
		t.Errorf("core has a budget: %+v", b)
	}
	sc, err := s.Scripts("example.big")
	if err != nil || sc.Budget.MemoryMB != 48 {
		t.Errorf("scripts budget = %+v, %v", sc.Budget, err)
	}
	// The owner's default applies to the plain mod, and a lower limit
	// refuses the big one.
	_, err = resolve(t, dir, Config{Budgets: Budgets{Default: Budget{MemoryMB: 8}, Max: Budget{MemoryMB: 32}}})
	if err == nil || !strings.Contains(err.Error(), "example.big asks for 48 MB") {
		t.Errorf("err = %v, want the budget refused", err)
	}
	s, err = resolve(t, dir, Config{Budgets: Budgets{Default: Budget{MemoryMB: 8}, Max: Budget{MemoryMB: 128, CPUPerTick: 6_000_000}}})
	if err != nil {
		t.Fatal(err)
	}
	if b := s.Mod("example.plain").Budget; b.MemoryMB != 8 || b.CPUPerTick != DefaultCPUPerTick {
		t.Errorf("plain budget = %+v", b)
	}
}

func TestBudgetChecks(t *testing.T) {
	for _, c := range []struct {
		b    Budgets
		want string
	}{
		{Budgets{}, ""},
		{Budgets{Default: Budget{MemoryMB: 32}, Max: Budget{MemoryMB: 32}}, ""},
		{Budgets{Default: Budget{MemoryMB: -1}}, "mods.budget.memory_mb"},
		{Budgets{Max: Budget{CPUPerTick: CeilingCPUPerTick + 1}}, "mods.max_budget.cpu_per_tick"},
		{Budgets{Default: Budget{MemoryMB: 128}}, "is above"},
	} {
		err := c.b.Check("mods.budget", "mods.max_budget")
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%+v: err = %v, want %q", c.b, err, c.want)
		}
	}
	if _, err := ParseManifest("mod.yaml", []byte(manifest("example.a", "budget: {memory_mb: 5000}"))); err == nil || !strings.Contains(err.Error(), "budget.memory_mb") {
		t.Errorf("err = %v", err)
	}
}
