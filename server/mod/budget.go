package mod

import "fmt"

// Budget is a mod's Lua budget in each zone (proposal 0002, review
// decision 52), on top of the limits of each call:
//   - MemoryMB is the most memory, in MiB, that the mod's Lua objects may
//     hold in one zone. An object belongs to the mod whose code made it
//     or last made it grow.
//   - CPUPerTick is the most CPU the mod's calls may use in one zone tick,
//     in the units of the per-call CPU limit: about one Lua instruction
//     each. A mod over it has its event handlers, timers and hooks wait
//     for the next tick.
//
// A zero field takes the server's default.
type Budget struct {
	MemoryMB   int   `yaml:"memory_mb"`
	CPUPerTick int64 `yaml:"cpu_per_tick"`
}

// Default budgets and the most a manifest may ask for, when the server
// owner sets none. The example mods and core's scripts fit well within
// the defaults (docs/security/review-2026-10.md).
const (
	DefaultMemoryMB      = 16
	DefaultCPUPerTick    = 2_000_000
	DefaultMaxMemoryMB   = 64
	DefaultMaxCPUPerTick = 10_000_000
)

// Ceilings of any budget, for the server config.
const (
	CeilingMemoryMB   = 1024
	CeilingCPUPerTick = 1_000_000_000
)

// Budgets are the server owner's budget settings: the budget of a mod
// whose manifest asks for none, and the most a manifest may ask for.
type Budgets struct {
	Default Budget
	Max     Budget
}

// orDefault fills the zero fields of b from def.
func (b Budget) orDefault(def Budget) Budget {
	if b.MemoryMB == 0 {
		b.MemoryMB = def.MemoryMB
	}
	if b.CPUPerTick == 0 {
		b.CPUPerTick = def.CPUPerTick
	}
	return b
}

// resolved returns the server's default budget and the most a manifest
// may ask for, with the package defaults for fields the owner left at 0.
func (b Budgets) resolved() (def, most Budget) {
	most = b.Max.orDefault(Budget{MemoryMB: DefaultMaxMemoryMB, CPUPerTick: DefaultMaxCPUPerTick})
	def = b.Default.orDefault(Budget{MemoryMB: min(DefaultMemoryMB, most.MemoryMB), CPUPerTick: min(DefaultCPUPerTick, most.CPUPerTick)})
	return def, most
}

// Check checks the owner's settings. field names them in errors, such
// as mods.budget.
func (b Budgets) Check(field, maxField string) error {
	for _, c := range []struct {
		name string
		v    Budget
	}{{field, b.Default}, {maxField, b.Max}} {
		if c.v.MemoryMB < 0 || c.v.MemoryMB > CeilingMemoryMB {
			return fmt.Errorf("%s.memory_mb is 0 for the default, or 1 to %d", c.name, CeilingMemoryMB)
		}
		if c.v.CPUPerTick < 0 || c.v.CPUPerTick > CeilingCPUPerTick {
			return fmt.Errorf("%s.cpu_per_tick is 0 for the default, or 1 to %d", c.name, CeilingCPUPerTick)
		}
	}
	def, most := b.resolved()
	if def.MemoryMB > most.MemoryMB || def.CPUPerTick > most.CPUPerTick {
		return fmt.Errorf("%s is above %s", field, maxField)
	}
	return nil
}

// budgetFor returns a mod's budget: what its manifest asks for, within
// the owner's settings, or the default. It fails when the manifest asks
// for more than the owner allows.
func (b Budgets) budgetFor(id string, asked Budget) (Budget, error) {
	def, most := b.resolved()
	if asked.MemoryMB > most.MemoryMB {
		return Budget{}, fmt.Errorf("%s asks for %d MB of Lua memory, more than the %d MB the server allows (mods.max_budget.memory_mb)", id, asked.MemoryMB, most.MemoryMB)
	}
	if asked.CPUPerTick > most.CPUPerTick {
		return Budget{}, fmt.Errorf("%s asks for %d CPU per tick, more than the %d the server allows (mods.max_budget.cpu_per_tick)", id, asked.CPUPerTick, most.CPUPerTick)
	}
	return asked.orDefault(def), nil
}
