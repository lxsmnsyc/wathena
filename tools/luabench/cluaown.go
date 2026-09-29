//go:build clua

package main

import "github.com/lxsmnsyc/wathena/tools/luabench/glue"

func init() {
	engines = append(engines,
		EngineDef{Name: "C Lua 5.4 (own cgo)", Dialect: Lua54, New: newOwnCgo},
		EngineDef{Name: "C Lua 5.4 (own cgo) + limits", Dialect: Lua54, New: newOwnCgo, Limits: defaultLimits, MemoryQuota: true},
	)
}

// glueHost adapts Host to glue.Host.
type glueHost struct{ h *Host }

func (g glueHost) Add(k, v int64)         { g.h.add(k, v) }
func (g glueHost) Get(f string) int64     { return g.h.get(f) }
func (g glueHost) Set(f string, v int64)  { g.h.set(f, v) }
func (g glueHost) Chance(n, d int64) bool { return g.h.chance(n, d) }

func newOwnCgo(h *Host, l Limits) (Engine, error) {
	var cpu, mem int64
	if l.On {
		cpu, mem = int64(l.Cpu), int64(l.Memory)
	}
	return glue.New(glueHost{h}, cpu, mem)
}
