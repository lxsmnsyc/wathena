package main

import (
	"context"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func init() {
	engines = append(engines,
		EngineDef{Name: "yuin/gopher-lua", Dialect: Lua51, New: newGopher},
		EngineDef{Name: "yuin/gopher-lua + limits", Dialect: Lua51, New: newGopher, Limits: defaultLimits},
	)
}

type gopher struct {
	L      *lua.LState
	limits Limits
}

func newGopher(h *Host, l Limits) (Engine, error) {
	L := lua.NewState()
	L.SetGlobal("wa_add", L.NewFunction(func(L *lua.LState) int {
		h.add(L.CheckInt64(1), L.CheckInt64(2))
		return 0
	}))
	L.SetGlobal("pc_get", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(h.get(L.CheckString(1))))
		return 1
	}))
	L.SetGlobal("pc_set", L.NewFunction(func(L *lua.LState) int {
		h.set(L.CheckString(1), L.CheckInt64(2))
		return 0
	}))
	L.SetGlobal("wa_chance", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LBool(h.chance(L.CheckInt64(1), L.CheckInt64(2))))
		return 1
	}))
	// Lua 5.1 has no bitwise operators. The converter would emit these.
	L.SetGlobal("bit_and", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(L.CheckInt64(1) & L.CheckInt64(2)))
		return 1
	}))
	L.SetGlobal("bit_or", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(L.CheckInt64(1) | L.CheckInt64(2)))
		return 1
	}))
	return &gopher{L: L, limits: l}, nil
}

// run applies a wall time limit, when limits are on, to one call into Lua.
// gopher-lua has no CPU or memory quota, only context cancellation.
func (g *gopher) run(f func() error) error {
	if !g.limits.On {
		return f()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(g.limits.Millis)*time.Millisecond)
	defer cancel()
	g.L.SetContext(ctx)
	defer g.L.RemoveContext()
	return f()
}

func (g *gopher) Load(name, src string) error {
	return g.run(func() error { return g.L.DoString(src) })
}

func (g *gopher) Call(fn string, arg int64) (int64, error) {
	var out int64
	err := g.run(func() error {
		if err := g.L.CallByParam(lua.P{Fn: g.L.GetGlobal(fn), NRet: 1, Protect: true}, lua.LNumber(arg)); err != nil {
			return err
		}
		out = int64(lua.LVAsNumber(g.L.Get(-1)))
		g.L.Pop(1)
		return nil
	})
	return out, err
}

func (g *gopher) Close() {
	g.L.Close()
}
