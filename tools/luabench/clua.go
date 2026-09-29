//go:build clua

package main

import (
	"errors"

	"github.com/aarzilli/golua/lua"
)

// C Lua needs cgo and Lua 5.4. See main.go for the build command.

func init() {
	engines = append(engines,
		EngineDef{Name: "C Lua 5.4 (cgo)", Dialect: Lua54, New: newCLua},
		EngineDef{Name: "C Lua 5.4 (cgo) + limits", Dialect: Lua54, New: newCLua, Limits: defaultLimits},
	)
}

type clua struct {
	L      *lua.State
	limits Limits
}

func newCLua(h *Host, l Limits) (Engine, error) {
	L := lua.NewState()
	L.OpenLibs()
	L.Register("wa_add", func(L *lua.State) int {
		h.add(int64(L.ToInteger(1)), int64(L.ToInteger(2)))
		return 0
	})
	L.Register("pc_get", func(L *lua.State) int {
		L.PushInteger(h.get(L.ToString(1)))
		return 1
	})
	L.Register("pc_set", func(L *lua.State) int {
		h.set(L.ToString(1), int64(L.ToInteger(2)))
		return 0
	})
	L.Register("wa_chance", func(L *lua.State) int {
		L.PushBoolean(h.chance(int64(L.ToInteger(1)), int64(L.ToInteger(2))))
		return 1
	})
	return &clua{L: L, limits: l}, nil
}

func (c *clua) Load(name, src string) error {
	if err := c.L.DoString(src); err != nil {
		return errors.New(name + ": " + err.Error())
	}
	return nil
}

func (c *clua) Call(fn string, arg int64) (int64, error) {
	if c.limits.On {
		// The count hook stops the script after Cpu instructions.
		// Setting it again before each call restarts the count.
		c.L.SetExecutionLimit(int(c.limits.Cpu))
	}
	c.L.GetGlobal(fn)
	c.L.PushInteger(arg)
	if err := c.L.Call(1, 1); err != nil {
		return 0, err
	}
	out := int64(c.L.ToInteger(-1))
	c.L.Pop(1)
	return out, nil
}

func (c *clua) Close() {
	c.L.Close()
}
