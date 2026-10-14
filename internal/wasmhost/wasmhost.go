// Package wasmhost runs C libraries compiled to WebAssembly, such as the
// asset encoders, in wazero. Each library is a WASI reactor that exports
// wa_alloc and wa_free for buffers the host passes in and out.
package wasmhost

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// maxMemoryPages limits each instance's memory to 1 GiB.
const maxMemoryPages = 1 << 14

// maxIdle is how many idle instances a module keeps for reuse.
const maxIdle = 64

// Module is a compiled library. Instances are created as needed and reused.
type Module struct {
	wasm []byte
	once sync.Once
	err  error
	rt   wazero.Runtime
	code wazero.CompiledModule
	idle chan *Instance
}

// New returns a module for a wasm binary. It compiles on first use.
func New(wasm []byte) *Module {
	return &Module{wasm: wasm, idle: make(chan *Instance, maxIdle)}
}

// newRuntime serializes making runtimes. wazero 1.12 reads and sets its
// version string without a lock the first time, so two modules set up at
// once, such as a build's texture and sound encoders, race.
var newRuntime sync.Mutex

func (m *Module) setup() error {
	m.once.Do(func() {
		ctx := context.Background()
		newRuntime.Lock()
		m.rt = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithMemoryLimitPages(maxMemoryPages))
		newRuntime.Unlock()
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, m.rt); err != nil {
			m.err = err
			return
		}
		m.code, m.err = m.rt.CompileModule(ctx, m.wasm)
	})
	return m.err
}

// Instance is one instance of a module. It runs one call at a time.
type Instance struct {
	mod   api.Module
	alloc api.Function
	free  api.Function
}

// Get returns an idle instance or a new one.
func (m *Module) Get() (*Instance, error) {
	select {
	case in := <-m.idle:
		return in, nil
	default:
	}
	if err := m.setup(); err != nil {
		return nil, err
	}
	// Instances are anonymous, so many can exist at once. A WASI reactor
	// is set up by calling _initialize.
	cfg := wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize")
	mod, err := m.rt.InstantiateModule(context.Background(), m.code, cfg)
	if err != nil {
		return nil, err
	}
	in := &Instance{mod: mod, alloc: mod.ExportedFunction("wa_alloc"), free: mod.ExportedFunction("wa_free")}
	if in.alloc == nil || in.free == nil {
		in.Close()
		return nil, errors.New("wasmhost: module does not export wa_alloc and wa_free")
	}
	return in, nil
}

// Put returns an instance for reuse. Callers put back only instances whose
// last call succeeded, and close the others, since a failed call can leave
// an instance in a bad state.
func (m *Module) Put(in *Instance) {
	select {
	case m.idle <- in:
	default:
		in.Close()
	}
}

// Close releases an instance.
func (in *Instance) Close() {
	_ = in.mod.Close(context.Background())
}

// Call calls an exported function.
func (in *Instance) Call(name string, args ...uint64) ([]uint64, error) {
	f := in.mod.ExportedFunction(name)
	if f == nil {
		return nil, fmt.Errorf("wasmhost: no export %q", name)
	}
	return f.Call(context.Background(), args...)
}

// Alloc allocates n bytes in the instance's memory.
func (in *Instance) Alloc(n int) (uint32, error) {
	if n < 0 || n > 1<<30 {
		return 0, fmt.Errorf("wasmhost: cannot allocate %d bytes", n)
	}
	res, err := in.alloc.Call(context.Background(), uint64(n))
	if err != nil {
		return 0, err
	}
	p := uint32(res[0]) // #nosec G115 -- a wasm32 pointer.
	if p == 0 {
		return 0, errors.New("wasmhost: out of memory")
	}
	return p, nil
}

// Free releases memory from Alloc, or a buffer the library allocated.
func (in *Instance) Free(p uint32) {
	_, _ = in.free.Call(context.Background(), uint64(p))
}

// Write copies data into the instance's memory.
func (in *Instance) Write(p uint32, data []byte) error {
	if !in.mod.Memory().Write(p, data) {
		return errors.New("wasmhost: write outside memory")
	}
	return nil
}

// Read copies n bytes out of the instance's memory.
func (in *Instance) Read(p, n uint32) ([]byte, error) {
	data, ok := in.mod.Memory().Read(p, n)
	if !ok {
		return nil, errors.New("wasmhost: read outside memory")
	}
	return append([]byte(nil), data...), nil
}
