package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// luaWasm is C Lua 5.4 and glue/glue.c built by wasm/build.sh.
//
//go:embed wasm/lua.wasm
var luaWasm []byte

func init() {
	engines = append(engines,
		EngineDef{Name: "C Lua 5.4 (wazero)", Dialect: Lua54, New: newWasm},
		EngineDef{Name: "C Lua 5.4 (wazero) + limits", Dialect: Lua54, New: newWasm, Limits: defaultLimits, MemoryQuota: true},
	)
}

// wasmPages caps each instance's memory at 64 MB, as a backstop for the
// allocator limit in the glue.
const wasmPages = 1024

var wasmRuntime = sync.OnceValues(func() (*wasmShared, error) {
	ctx := context.Background()
	// Lua's setjmp and longjmp are compiled to WebAssembly exceptions.
	cfg := wazero.NewRuntimeConfigCompiler().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
		WithMemoryLimitPages(wasmPages)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, err
	}
	// GoModuleFunc avoids the reflection that WithFunc uses on every call.
	i64 := api.ValueTypeI64
	i32 := api.ValueTypeI32
	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, m api.Module, st []uint64) {
		wasmHost(int64(st[0])).add(int64(st[1]), int64(st[2]))
	}), []api.ValueType{i64, i64, i64}, nil).Export("host_add").
		NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, m api.Module, st []uint64) {
		b, _ := m.Memory().Read(uint32(st[1]), uint32(st[2]))
		st[0] = uint64(wasmHost(int64(st[0])).get(string(b)))
	}), []api.ValueType{i64, i32, i32}, []api.ValueType{i64}).Export("host_get").
		NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, m api.Module, st []uint64) {
		b, _ := m.Memory().Read(uint32(st[1]), uint32(st[2]))
		wasmHost(int64(st[0])).set(string(b), int64(st[3]))
	}), []api.ValueType{i64, i32, i32, i64}, nil).Export("host_set").
		NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(ctx context.Context, m api.Module, st []uint64) {
		ok := wasmHost(int64(st[0])).chance(int64(st[1]), int64(st[2]))
		st[0] = 0
		if ok {
			st[0] = 1
		}
	}), []api.ValueType{i64, i64, i64}, []api.ValueType{i32}).Export("host_chance").
		Instantiate(ctx)
	if err != nil {
		return nil, err
	}
	compiled, err := r.CompileModule(ctx, luaWasm)
	if err != nil {
		return nil, err
	}
	return &wasmShared{r: r, compiled: compiled}, nil
})

type wasmShared struct {
	r        wazero.Runtime
	compiled wazero.CompiledModule
}

var wasmHosts struct {
	sync.Mutex
	list []*Host
}

func wasmHost(h int64) *Host {
	return wasmHosts.list[h]
}

type wasmEngine struct {
	ctx    context.Context
	mod    api.Module
	mem    api.Memory
	L      uint64
	out    uint32
	names  map[string]uint32
	stack  []uint64
	call   api.Function
	load   api.Function
	errf   api.Function
	malloc api.Function
	free   api.Function
	closef api.Function
	memf   api.Function
}

func newWasm(h *Host, l Limits) (Engine, error) {
	shared, err := wasmRuntime()
	if err != nil {
		return nil, err
	}
	wasmHosts.Lock()
	handle := int64(len(wasmHosts.list))
	wasmHosts.list = append(wasmHosts.list, h)
	wasmHosts.Unlock()

	e := &wasmEngine{names: map[string]uint32{}, stack: make([]uint64, 4)}
	e.ctx = context.Background()
	mod, err := shared.r.InstantiateModule(e.ctx, shared.compiled,
		wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		return nil, err
	}
	e.mod = mod
	e.mem = mod.Memory()
	e.call = mod.ExportedFunction("glue_call")
	e.load = mod.ExportedFunction("glue_load")
	e.errf = mod.ExportedFunction("glue_error")
	e.malloc = mod.ExportedFunction("glue_malloc")
	e.free = mod.ExportedFunction("glue_free")
	e.closef = mod.ExportedFunction("glue_close")
	e.memf = mod.ExportedFunction("glue_memory")

	var cpu, memLimit uint64
	if l.On {
		cpu, memLimit = l.Cpu, l.Memory
	}
	res, err := mod.ExportedFunction("glue_new").Call(e.ctx, uint64(handle), cpu, memLimit)
	if err != nil {
		return nil, err
	}
	if res[0] == 0 {
		return nil, errors.New("glue_new: out of memory")
	}
	e.L = res[0]
	if e.out, err = e.alloc(8); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *wasmEngine) alloc(n int) (uint32, error) {
	e.stack[0] = uint64(n) // #nosec G115 -- a non-negative size.
	if err := e.malloc.CallWithStack(e.ctx, e.stack); err != nil {
		return 0, err
	}
	if e.stack[0] == 0 {
		return 0, errors.New("glue_malloc: out of memory")
	}
	return uint32(e.stack[0]), nil // #nosec G115 -- a wasm32 address.
}

// cstring copies s into WebAssembly memory with a trailing zero byte.
func (e *wasmEngine) cstring(s string) (uint32, error) {
	p, err := e.alloc(len(s) + 1)
	if err != nil {
		return 0, err
	}
	e.mem.Write(p, append([]byte(s), 0))
	return p, nil
}

func (e *wasmEngine) release(p uint32) {
	e.stack[0] = uint64(p)
	_ = e.free.CallWithStack(e.ctx, e.stack)
}

func (e *wasmEngine) luaError(status uint64) error {
	e.stack[0] = e.L
	e.stack[1] = uint64(e.out)
	if err := e.errf.CallWithStack(e.ctx, e.stack); err != nil {
		return err
	}
	n, _ := e.mem.ReadUint32Le(e.out)
	msg, _ := e.mem.Read(uint32(e.stack[0]), n) // #nosec G115 -- a wasm32 address.
	return fmt.Errorf("lua error %d: %s", status, msg)
}

func (e *wasmEngine) Load(name, src string) error {
	p, err := e.cstring(src)
	if err != nil {
		return err
	}
	defer e.release(p)
	np, err := e.cstring(name)
	if err != nil {
		return err
	}
	defer e.release(np)
	e.stack[0], e.stack[1], e.stack[2], e.stack[3] = e.L, uint64(p), uint64(len(src)), uint64(np)
	if err := e.load.CallWithStack(e.ctx, e.stack); err != nil {
		return err
	}
	if status := e.stack[0]; status != 0 {
		return e.luaError(status)
	}
	return nil
}

func (e *wasmEngine) Call(fn string, arg int64) (int64, error) {
	np, ok := e.names[fn]
	if !ok {
		var err error
		if np, err = e.cstring(fn); err != nil {
			return 0, err
		}
		e.names[fn] = np
	}
	e.stack[0], e.stack[1], e.stack[2], e.stack[3] = e.L, uint64(np), uint64(arg), uint64(e.out) // #nosec G115 -- bits are kept as they are.
	if err := e.call.CallWithStack(e.ctx, e.stack); err != nil {
		return 0, err
	}
	if status := e.stack[0]; status != 0 {
		return 0, e.luaError(status)
	}
	v, _ := e.mem.ReadUint64Le(e.out)
	return int64(v), nil // #nosec G115 -- bits are kept as they are.
}

// Memory returns the bytes the Lua state has allocated.
func (e *wasmEngine) Memory() int64 {
	e.stack[0] = e.L
	if err := e.memf.CallWithStack(e.ctx, e.stack); err != nil {
		return -1
	}
	return int64(e.stack[0]) // #nosec G115 -- a byte count.
}

func (e *wasmEngine) Close() {
	e.stack[0] = e.L
	_ = e.closef.CallWithStack(e.ctx, e.stack)
	_ = e.mod.Close(e.ctx)
}
