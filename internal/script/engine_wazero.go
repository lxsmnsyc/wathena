//go:build !cgo || luawasm || !(amd64 || arm64)

package script

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

// Backend names the build of the binding: "cgo" or "wasm".
const Backend = "wasm"

// luaWasm is csrc/all.c built by wasm/build.sh.
//
//go:embed wasm/lua.wasm
var luaWasm []byte

// wasmPages caps each instance's memory at 1 GiB. The Lua allocator's
// limit is the real one.
const wasmPages = 1 << 14

type wasmRuntime struct {
	r        wazero.Runtime
	compiled wazero.CompiledModule
}

var runtimeOnce = sync.OnceValues(func() (*wasmRuntime, error) {
	ctx := context.Background()
	// Lua's setjmp and longjmp are compiled to WebAssembly exceptions.
	cfg := wazero.NewRuntimeConfigCompiler().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
		WithMemoryLimitPages(wasmPages)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return nil, err
	}
	i32, i64 := api.ValueTypeI32, api.ValueTypeI64
	// GoModuleFunc avoids the reflection that WithFunc uses on every call.
	_, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(_ context.Context, _ api.Module, st []uint64) {
		st[0] = uint64(uint32(hostCall(int64(st[0]), int32(st[1]), int32(st[2]), int32(st[3])))) // #nosec G115 -- wasm values.
	}), []api.ValueType{i64, i32, i32, i32}, []api.ValueType{i32}).Export("host_call").
		Instantiate(ctx)
	if err != nil {
		return nil, err
	}
	compiled, err := r.CompileModule(ctx, luaWasm)
	if err != nil {
		return nil, err
	}
	return &wasmRuntime{r: r, compiled: compiled}, nil
})

// engine is the WebAssembly build of the binding. Each state is its own
// module instance, with its own memory.
type engine struct {
	ctx context.Context
	mod api.Module
	mem api.Memory
	h   uint32
	// fop holds a glue_op function for each depth of nested calls: an
	// api.Function cannot be called again while it runs, and host
	// functions can call glue_op during a call.
	fop   []api.Function
	stack [][]uint64
	depth int
	// broken is set when wazero fails. The instance is not used again.
	broken bool
}

func newEngine(handle, cpu, mem, timeout int64) (*engine, error) {
	rt, err := runtimeOnce()
	if err != nil {
		return nil, fmt.Errorf("script: wazero: %w", err)
	}
	ctx := context.Background()
	// Instances are anonymous, so many can exist at once. Lua reads the
	// real clocks for the time limit and table.sort.
	cfg := wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize").
		WithSysNanotime().WithSysWalltime()
	mod, err := rt.r.InstantiateModule(ctx, rt.compiled, cfg)
	if err != nil {
		return nil, fmt.Errorf("script: wazero: %w", err)
	}
	e := &engine{ctx: ctx, mod: mod, mem: mod.Memory()}
	res, err := mod.ExportedFunction("glue_new").Call(ctx, uint64(handle), uint64(cpu), uint64(mem), uint64(timeout)) // #nosec G115 -- non-negative.
	if err != nil {
		_ = mod.Close(ctx)
		return nil, fmt.Errorf("script: wazero: %w", err)
	}
	if res[0] == 0 {
		_ = mod.Close(ctx)
		return nil, errors.New("script: cannot create a Lua state: out of memory")
	}
	e.h = uint32(res[0]) // #nosec G115 -- a wasm32 address.
	return e, nil
}

func (e *engine) close() {
	_, _ = e.mod.ExportedFunction("glue_close").Call(e.ctx, uint64(e.h))
	_ = e.mod.Close(e.ctx)
}

// op calls glue_op. Host functions can call it again while it runs, so
// each depth has its own stack.
func (e *engine) op(op int32, a, b int64) (int32, error) {
	if e.broken {
		return 0, ErrBroken
	}
	d := e.depth
	if d == len(e.fop) {
		e.fop = append(e.fop, e.mod.ExportedFunction("glue_op"))
		e.stack = append(e.stack, make([]uint64, 4))
	}
	st := e.stack[d]
	st[0], st[1], st[2], st[3] = uint64(e.h), uint64(uint32(op)), uint64(a), uint64(b) // #nosec G115 -- bits are kept as they are.
	e.depth++
	err := e.fop[d].CallWithStack(e.ctx, st)
	e.depth--
	if err != nil {
		e.broken = true
		return 0, fmt.Errorf("%w: %w", ErrBroken, err)
	}
	return int32(uint32(st[0])), nil // #nosec G115 -- an i32 result.
}

func (e *engine) hdr(i int) int64 {
	v, _ := e.mem.ReadUint64Le(e.h + uint32(i)*8) // #nosec G115 -- a small index.
	return int64(v)                               // #nosec G115 -- bits are kept as they are.
}

func (e *engine) setHdr(i int, v int64) {
	e.mem.WriteUint64Le(e.h+uint32(i)*8, uint64(v)) // #nosec G115 -- bits are kept as they are.
}

// view returns n bytes of memory at addr. It is valid until the next call
// into the module, which can grow the memory.
func (e *engine) view(addr, n int64) []byte {
	b, ok := e.mem.Read(uint32(addr), uint32(n)) // #nosec G115 -- wasm32 addresses and sizes.
	if !ok {
		return nil
	}
	return b
}

func (e *engine) vals() []byte { return e.view(e.hdr(hVals), e.hdr(hVcap)*slotSize) }
func (e *engine) buf() []byte  { return e.view(e.hdr(hBuf), e.hdr(hBcap)) }

func (e *engine) str(addr int64, n int) string {
	if n <= 0 {
		return ""
	}
	return string(e.view(addr, int64(n)))
}
