//go:build clua

// Package glue binds C Lua 5.4 through cgo with glue.c, the same C code the
// WebAssembly build uses.
package glue

/*
#cgo pkg-config: lua5.4
#include <stdint.h>
#include <stdlib.h>

typedef struct lua_State lua_State;
lua_State *glue_new(int64_t host, int64_t cpu_limit, int64_t mem_limit);
void glue_close(lua_State *L);
int32_t glue_load(lua_State *L, char *src, int32_t len, char *name);
int32_t glue_call(lua_State *L, char *fn, int64_t arg, int64_t *out);
char *glue_error(lua_State *L, int32_t *len);
int64_t glue_memory(lua_State *L);
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// Host receives calls from Lua.
type Host interface {
	Add(k, v int64)
	Get(field string) int64
	Set(field string, v int64)
	Chance(n, d int64) bool
}

func host(h C.int64_t) Host {
	return cgo.Handle(h).Value().(Host)
}

//export host_add
func host_add(h, k, v C.int64_t) {
	host(h).Add(int64(k), int64(v))
}

//export host_get
func host_get(h C.int64_t, s *C.char, n C.int32_t) C.int64_t {
	return C.int64_t(host(h).Get(C.GoStringN(s, C.int(n))))
}

//export host_set
func host_set(h C.int64_t, s *C.char, n C.int32_t, v C.int64_t) {
	host(h).Set(C.GoStringN(s, C.int(n)), int64(v))
}

//export host_chance
func host_chance(h, n, d C.int64_t) C.int32_t {
	if host(h).Chance(int64(n), int64(d)) {
		return 1
	}
	return 0
}

// State is one Lua state.
type State struct {
	L      *C.lua_State
	handle cgo.Handle
	names  map[string]*C.char
}

// New creates a state. A limit of 0 means no limit.
func New(h Host, cpu, mem int64) (*State, error) {
	handle := cgo.NewHandle(h)
	L := C.glue_new(C.int64_t(handle), C.int64_t(cpu), C.int64_t(mem))
	if L == nil {
		handle.Delete()
		return nil, errors.New("glue_new: out of memory")
	}
	return &State{L: L, handle: handle, names: map[string]*C.char{}}, nil
}

func (s *State) luaError(status C.int32_t) error {
	var n C.int32_t
	p := C.glue_error(s.L, &n)
	return fmt.Errorf("lua error %d: %s", status, C.GoStringN(p, C.int(n)))
}

// Load runs a chunk.
func (s *State) Load(name, src string) error {
	cs := C.CString(src)
	defer C.free(unsafe.Pointer(cs))
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	if status := C.glue_load(s.L, cs, C.int32_t(len(src)), cn); status != 0 {
		return s.luaError(status)
	}
	return nil
}

// Call calls a global function with one integer.
func (s *State) Call(fn string, arg int64) (int64, error) {
	cn, ok := s.names[fn]
	if !ok {
		cn = C.CString(fn)
		s.names[fn] = cn
	}
	var out C.int64_t
	if status := C.glue_call(s.L, cn, C.int64_t(arg), &out); status != 0 {
		return 0, s.luaError(status)
	}
	return int64(out), nil
}

// Memory returns the bytes the state has allocated.
func (s *State) Memory() int64 {
	return int64(C.glue_memory(s.L))
}

// Close frees the state.
func (s *State) Close() {
	C.glue_close(s.L)
	s.handle.Delete()
	for _, p := range s.names {
		C.free(unsafe.Pointer(p))
	}
}
