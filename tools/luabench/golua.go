package main

import (
	"fmt"
	"io"

	"github.com/arnodel/golua/lib"
	rt "github.com/arnodel/golua/runtime"
)

func init() {
	engines = append(engines,
		EngineDef{Name: "arnodel/golua", Dialect: Lua54, New: newGolua},
		EngineDef{Name: "arnodel/golua + limits", Dialect: Lua54, New: newGolua, Limits: defaultLimits, MemoryQuota: true},
		EngineDef{Name: "arnodel/golua + CPU limit", Dialect: Lua54, New: newGolua, Limits: Limits{On: true, Cpu: defaultLimits.Cpu}},
	)
}

type golua struct {
	r      *rt.Runtime
	limits Limits
}

func newGolua(h *Host, l Limits) (Engine, error) {
	r := rt.New(io.Discard)
	lib.LoadAll(r)
	env := r.GlobalEnv()
	// Host functions do a fixed small amount of work, so they are safe to
	// call under CPU and memory quotas.
	safe := func(f *rt.GoFunction) { f.SolemnlyDeclareCompliance(rt.ComplyCpuSafe | rt.ComplyMemSafe) }
	safe(r.SetEnvGoFunc(env, "wa_add", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		k, err := c.IntArg(0)
		if err != nil {
			return nil, err
		}
		v, err := c.IntArg(1)
		if err != nil {
			return nil, err
		}
		h.add(k, v)
		return c.Next(), nil
	}, 2, false))
	safe(r.SetEnvGoFunc(env, "pc_get", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		f, err := c.StringArg(0)
		if err != nil {
			return nil, err
		}
		return c.PushingNext1(t.Runtime, rt.IntValue(h.get(f))), nil
	}, 1, false))
	safe(r.SetEnvGoFunc(env, "pc_set", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		f, err := c.StringArg(0)
		if err != nil {
			return nil, err
		}
		v, err := c.IntArg(1)
		if err != nil {
			return nil, err
		}
		h.set(f, v)
		return c.Next(), nil
	}, 2, false))
	safe(r.SetEnvGoFunc(env, "wa_chance", func(t *rt.Thread, c *rt.GoCont) (rt.Cont, error) {
		n, err := c.IntArg(0)
		if err != nil {
			return nil, err
		}
		d, err := c.IntArg(1)
		if err != nil {
			return nil, err
		}
		return c.PushingNext1(t.Runtime, rt.BoolValue(h.chance(n, d))), nil
	}, 2, false))
	return &golua{r: r, limits: l}, nil
}

// run applies the limits, when they are on, to one call into Lua.
// A panic inside golua is returned as an error.
func (g *golua) run(f func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("golua panic: %v", p)
		}
	}()
	if !g.limits.On {
		return f()
	}
	def := rt.RuntimeContextDef{HardLimits: rt.RuntimeResources{Cpu: g.limits.Cpu, Memory: g.limits.Memory}}
	_, err = g.r.MainThread().CallContext(def, f)
	return err
}

func (g *golua) Load(name, src string) error {
	return g.run(func() error {
		chunk, err := g.r.CompileAndLoadLuaChunk(name, []byte(src), rt.TableValue(g.r.GlobalEnv()))
		if err != nil {
			return err
		}
		_, err = rt.Call1(g.r.MainThread(), rt.FunctionValue(chunk))
		return err
	})
}

func (g *golua) Call(fn string, arg int64) (int64, error) {
	var out int64
	err := g.run(func() error {
		f := g.r.GlobalEnv().Get(rt.StringValue(fn))
		v, err := rt.Call1(g.r.MainThread(), f, rt.IntValue(arg))
		if err != nil {
			return err
		}
		out, _ = v.TryInt()
		return nil
	})
	return out, err
}

func (g *golua) Close() {
	g.r.Close(nil)
}
