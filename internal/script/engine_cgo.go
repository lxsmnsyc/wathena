//go:build cgo && !luawasm && (amd64 || arm64)

package script

/*
#cgo CFLAGS: -I${SRCDIR}/lua/src -I${SRCDIR}/csrc -O2
#cgo !windows LDFLAGS: -lm
#include "glue_abi.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Backend names the build of the binding: "cgo" or "wasm".
const Backend = "cgo"

// engine is the cgo build of the binding. It calls glue.c directly.
type engine struct {
	h *C.glue_hdr
}

func newEngine(handle, cpu, mem, timeout int64) (*engine, error) {
	h := C.glue_new(C.int64_t(handle), C.int64_t(cpu), C.int64_t(mem), C.int64_t(timeout))
	if h == nil {
		return nil, errors.New("script: cannot create a Lua state: out of memory")
	}
	return &engine{h: h}, nil
}

func (e *engine) close() {
	C.glue_close(e.h)
	e.h = nil
}

func (e *engine) op(op int32, a, b int64) (int32, error) {
	return int32(C.glue_op(e.h, C.int32_t(op), C.int64_t(a), C.int64_t(b))), nil
}

// words views the header as 64-bit fields.
func (e *engine) words() *[8]int64 {
	return (*[8]int64)(unsafe.Pointer(e.h)) // #nosec G103 -- glue_hdr is eight int64_t fields.
}

func (e *engine) hdr(i int) int64       { return e.words()[i] }
func (e *engine) setHdr(i int, v int64) { e.words()[i] = v }

// pointer turns an address that C stored in an int64_t into a pointer.
// The binding builds only for 64-bit targets, where both are 8 bytes.
func pointer(addr int64) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) // #nosec G103 -- an address of C memory.
}

func (e *engine) vals() []byte {
	w := e.words()
	return unsafe.Slice((*byte)(pointer(w[hVals])), w[hVcap]*slotSize) // #nosec G103 -- the C value array.
}

func (e *engine) buf() []byte {
	w := e.words()
	return unsafe.Slice((*byte)(pointer(w[hBuf])), w[hBcap]) // #nosec G103 -- the C byte buffer.
}

// str copies n bytes of C memory at addr, a string Lua holds.
func (e *engine) str(addr int64, n int) string {
	if n <= 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(pointer(addr)), n)) // #nosec G103 -- a Lua string.
}

//export host_call
func host_call(handle C.int64_t, id, nargs, flags C.int32_t) C.int32_t {
	return C.int32_t(hostCall(int64(handle), int32(id), int32(nargs), int32(flags)))
}
