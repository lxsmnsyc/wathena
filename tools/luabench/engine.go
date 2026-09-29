package main

// Host is the game state the scripts read and change.
type Host struct {
	Mods      int64
	Zeny      int64
	BaseLevel int64
	rolls     int64
}

func newHost() *Host {
	return &Host{Zeny: 1 << 40, BaseLevel: 95}
}

func (h *Host) add(_, v int64) { h.Mods += v }

func (h *Host) get(field string) int64 {
	switch field {
	case "zeny":
		return h.Zeny
	case "base_level":
		return h.BaseLevel
	}
	return 0
}

func (h *Host) set(field string, v int64) {
	if field == "zeny" {
		h.Zeny = v
	}
}

// chance is deterministic, so every engine gives the same results.
func (h *Host) chance(n, d int64) bool {
	h.rolls++
	return h.rolls%d < n
}

// Limits are the sandbox limits applied to each call.
type Limits struct {
	On bool
	// Cpu is the golua CPU quota, and the instruction limit for C Lua.
	Cpu uint64
	// Memory is the golua memory quota in bytes.
	Memory uint64
	// Millis is the wall time limit for gopher-lua.
	Millis int64
}

var defaultLimits = Limits{On: true, Cpu: 10_000_000, Memory: 16 << 20, Millis: 50}

// Engine runs Lua scripts against a Host.
type Engine interface {
	// Load runs a chunk in the global environment.
	Load(name, src string) error
	// Call calls a global function with one integer and returns one integer.
	Call(fn string, arg int64) (int64, error)
	Close()
}

// EngineDef describes one runtime configuration.
type EngineDef struct {
	Name    string
	Dialect Dialect
	New     func(h *Host, l Limits) (Engine, error)
	Limits  Limits
	// MemoryQuota is true when the runtime can stop a script that uses
	// too much memory.
	MemoryQuota bool
}

// engines is filled by the files for each runtime.
var engines []EngineDef
