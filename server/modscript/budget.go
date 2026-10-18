package modscript

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// Per-mod Lua budgets (proposal 0002, review decision 52). Each mod with
// scripts is an owner of memory in the zone's Lua state, with the limit
// of its budget: an allocation past it stops the mod's call, as the
// per-call limits do. Each mod also has a CPU budget per tick. Its code
// runs until the budget is used up, and then its event handlers, timers
// and hooks wait for the next tick, with a warning in the log at most
// once a minute. Setup functions, item, skill and NPC scripts, schedules
// and conditions still run, so a player's action is never lost, and
// their CPU counts.
//
// Core is the server's own content and has no budget. It is owner 0,
// whose memory limit is CoreMemory.

// CoreMemory is the most memory owner 0 may hold: core's scripts, the
// prelude, and what no mod's code made.
const CoreMemory = 64 << 20

// BudgetWarnEvery is the least time between two warnings of one mod
// over its CPU budget, in zone milliseconds.
const BudgetWarnEvery = 60_000

// errOverBudget is the error of a call refused for the CPU budget. It
// does not count as a failure of the mod.
var errOverBudget = errors.New("the mod used its Lua CPU budget for this tick")

// budget is one mod's budget in this zone and what it used.
type budget struct {
	owner int
	// cpu is the budget per tick, or 0 for none: core's.
	cpu int64
	// used is the CPU spent in the tick numbered tick.
	used, tick int64
	// refused counts the calls refused since the last warning, and
	// warned is when it was, or -1.
	refused int
	warned  int64
	// peakCPU is the most CPU spent in one tick, and peakMemory the most
	// memory held at a charge.
	peakCPU, peakMemory int64
}

// addOwner gives a mod with scripts its owner of memory and its budget.
// Core is owner 0, with no CPU budget, so its use can be read.
func (r *Runtime) addOwner(sc *mod.Scripts) error {
	if sc.Mod == mod.Core {
		// Core is owner 0, with no CPU budget.
		if r.budgets[mod.Core] == nil {
			r.budgets[mod.Core] = &budget{owner: 0, warned: -1}
		}
		return nil
	}
	// A zero field is the default, as mod.Resolve gives it.
	memory, cpu := int64(sc.Budget.MemoryMB), sc.Budget.CPUPerTick
	if memory <= 0 {
		memory = mod.DefaultMemoryMB
	}
	if cpu <= 0 {
		cpu = mod.DefaultCPUPerTick
	}
	b := r.budgets[sc.Mod]
	if b == nil {
		if r.nextOwner >= script.MaxOwners {
			return fmt.Errorf("%s: a zone runs the scripts of at most %d mods", sc.Mod, script.MaxOwners-1)
		}
		b = &budget{owner: r.nextOwner, warned: -1}
		r.nextOwner++
		r.budgets[sc.Mod] = b
	}
	b.cpu = cpu
	r.s.SetOwnerLimit(b.owner, memory<<20)
	return nil
}

// ownerOf is the owner of memory of a mod: its own, or 0 for core and
// for a mod without scripts.
func (r *Runtime) ownerOf(id string) int {
	if b := r.budgets[id]; b != nil {
		return b.owner
	}
	return 0
}

// setCurrent makes c the context whose code runs. The CPU spent since
// the last change counts toward the mod that ran, and new memory is
// charged to c's mod.
func (r *Runtime) setCurrent(c *Context) {
	r.charge()
	r.current = c
	r.setOwner()
}

// setLoading sets the mod whose file loads outside any context.
func (r *Runtime) setLoading(id string) {
	r.charge()
	r.loading = id
	r.setOwner()
}

func (r *Runtime) setOwner() {
	r.s.SetOwner(r.ownerOf(r.actor()))
}

// charge counts the CPU spent since the last call of charge toward the
// mod that ran.
func (r *Runtime) charge() {
	now := r.s.Spent()
	d := now - r.spent
	r.spent = now
	if d <= 0 {
		return
	}
	if b := r.budgets[r.actor()]; b != nil {
		r.window(b)
		b.used += d
		b.peakCPU = max(b.peakCPU, b.used)
		b.peakMemory = max(b.peakMemory, r.s.OwnerMemory(b.owner))
	}
}

// BeginTick starts a new tick: every mod's CPU budget is whole again.
// The zone calls it at each tick. A new zone clock time starts a tick
// too.
func (r *Runtime) BeginTick() {
	r.tick++
}

// window moves a budget to the current tick.
func (r *Runtime) window(b *budget) {
	if now := r.host.Now(); now != r.tickNow {
		r.tickNow = now
		r.tick++
	}
	if b.tick != r.tick {
		b.tick, b.used = r.tick, 0
	}
}

// overBudget reports whether a context's mod has used its CPU budget for
// this tick. The call is then refused: it is counted, and the mod gets
// a warning at most once every BudgetWarnEvery.
func (r *Runtime) overBudget(c *Context) bool {
	b := r.budgets[c.Mod]
	if b == nil || b.cpu <= 0 {
		return false
	}
	r.charge()
	r.window(b)
	if b.used < b.cpu {
		return false
	}
	b.refused++
	if now := r.host.Now(); b.warned < 0 || now-b.warned >= BudgetWarnEvery {
		r.log.Warn("mod used its Lua CPU budget for this tick; its handlers, timers and hooks wait for the next tick",
			"map", r.host.Map(), "mod", c.Mod, "used", b.used, "budget", b.cpu, "refused", b.refused)
		b.warned, b.refused = now, 0
	}
	return true
}

// runHandler runs an event handler, unless its mod is over its CPU
// budget for this tick.
func (r *Runtime) runHandler(c *Context, fn script.Value, args ...script.Value) error {
	if r.overBudget(c) {
		return errOverBudget
	}
	return r.run(c, fn, args...)
}

// Usage is a mod's use of its budget in this zone, for tests and the
// server's tools.
type Usage struct {
	// Memory is the bytes the mod's Lua objects hold. Core's is owner
	// 0's: what no mod's code made.
	Memory int64
	// CPU is what the mod spent in the current tick.
	CPU int64
	// PeakMemory and PeakCPU are the most memory seen and the most CPU
	// spent in one tick since the zone started.
	PeakMemory, PeakCPU int64
	// CPUBudget is the mod's CPU budget per tick, or 0 for core, which
	// has none.
	CPUBudget int64
}

// Usage returns a mod's use of its budget. ok is false for a mod without
// scripts in this zone.
func (r *Runtime) Usage(id string) (u Usage, ok bool) {
	b := r.budgets[id]
	if b == nil {
		return Usage{}, false
	}
	r.charge()
	r.window(b)
	m := r.s.OwnerMemory(b.owner)
	return Usage{Memory: m, CPU: b.used, PeakMemory: max(b.peakMemory, m), PeakCPU: b.peakCPU, CPUBudget: b.cpu}, true
}
