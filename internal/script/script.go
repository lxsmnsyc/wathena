package script

import (
	"encoding/binary"
	"errors"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Default limits, from decisions 0002 and 0054. The CPU budget measures
// a call's work. The time limit is only a backstop for a call that is
// stuck, so it is long: a machine that stalls, such as one that swaps,
// stretches the clock time of every call, and must not stop scripts
// that do little work.
const (
	DefaultCPU     = 10_000_000
	DefaultMemory  = 16 << 20
	DefaultTimeout = 30 * time.Second
)

// Limits stop scripts that run too long or use too much memory. A zero
// field takes the default. A negative field turns that limit off.
type Limits struct {
	// CPU is the instruction budget of each call and each resume. Work that
	// library functions do in C, and memory allocated, are charged too.
	// It is what limits a script's work.
	CPU int64
	// Memory is the most memory the state may hold, in bytes.
	Memory int64
	// Timeout is the wall-clock limit of each call and each resume. It is a
	// backstop for a call stuck in work the CPU budget does not see, such
	// as comparing long strings over and over. The clock starts after the
	// first 1,000 instructions, so short calls never read it. It counts
	// the time the machine stalls too, so it must be far above the time
	// any call takes.
	Timeout time.Duration
}

func (l Limits) values() (cpu, mem, timeout int64) {
	pick := func(v, def int64) int64 {
		switch {
		case v == 0:
			return def
		case v < 0:
			return 0
		}
		return v
	}
	return pick(l.CPU, DefaultCPU), pick(l.Memory, DefaultMemory), pick(int64(l.Timeout), int64(DefaultTimeout))
}

// Options configure a new State.
type Options struct {
	Limits Limits
	// Modules returns the source of a module for require. Modules are
	// loaded once per state. A nil Modules finds no modules.
	Modules func(name string) (src string, ok bool)
}

// Offsets of the fields of glue_hdr, in 64-bit words.
const (
	hVals = iota
	hVcap
	hBuf
	hBcap
	hNin
	hNres
	hFatal
	hMem
	hOwner
	hSpent
	hOwnerMem
	hOwnerLimit = hOwnerMem + MaxOwners
	hWords      = hOwnerLimit + MaxOwners
)

// MaxOwners is how many owners of memory a state has, GLUE_OWNERS.
// Owner 0 holds what no other owner does.
const MaxOwners = 512

// Types of glue_value.
const (
	tNil int32 = iota
	tBool
	tInt
	tFloat
	tString
	tTable
	tTmp
	tRef
)

// Operations of glue_op.
const (
	opCall int32 = iota + 1
	opResume
	opLoad
	opNewThread
	opKeep
	opUnref
	opCloseThread
	opSetGlobal
	opGetGlobal
	opHostFn
	opReserve
	opGC
	opNested
)

// Lua status codes and glue results.
const (
	statusOK     = 0
	statusYield  = 1
	statusSyntax = 3
	statusMem    = 4
	statusEncode = 10
	statusArg    = 11
)

// What a host function returns to C.
const (
	hostReturn int32 = iota
	hostError
	hostYield
)

const canYield = 1

// Limits of values passed between Go and Lua, the same as in glue.c.
const (
	maxDepth  = 16
	maxValues = 1 << 20
	slotSize  = 16
)

// States of State.busy.
const (
	idle int32 = iota
	running
	inHost
)

// MaxNested is the most nested calls (Call1Nested) that can be open at
// once.
const MaxNested = 8

// State is one Lua state. Each zone has its own.
//
// A State is used by one goroutine at a time. Using it from two goroutines
// at once panics. A host function may call Keep, Ref.Release, SetGlobal,
// Register, and Call1Nested on its own state, but not Call, Load, or
// Resume.
type State struct {
	e       *engine
	handle  int64
	busy    atomic.Int32
	gen     uint32
	funcs   []hostFunc
	modules func(string) (string, bool)
	hostErr error
	// calls holds the context of each host call that runs, one per
	// depth: a nested call can run host functions while one runs.
	calls  []*Call
	depth  int
	nested int

	pendingMu  sync.Mutex
	pending    []int32
	hasPending atomic.Bool
}

type hostFunc struct {
	name string
	fn   HostFunc
}

// New creates a state with the base, coroutine, math, string, table, and
// utf8 libraries, and the sandbox described in README.md.
func New(opts Options) (*State, error) {
	s := &State{modules: opts.Modules}
	s.funcs = []hostFunc{{name: "require", fn: s.require}}
	s.handle = handles.add(s)
	cpu, mem, timeout := opts.Limits.values()
	e, err := newEngine(s.handle, cpu, mem, timeout)
	if err != nil {
		handles.remove(s.handle)
		return nil, err
	}
	s.e = e
	return s, nil
}

// Close frees the state. Values and refs from it become invalid.
func (s *State) Close() {
	if s.e == nil {
		return
	}
	if !s.busy.CompareAndSwap(idle, running) {
		panic("script: Close while the state is running")
	}
	s.e.close()
	s.e = nil
	handles.remove(s.handle)
	s.busy.Store(idle)
}

// enter marks the state as running. Host functions cannot enter.
func (s *State) enter() error {
	if !s.busy.CompareAndSwap(idle, running) {
		if s.busy.Load() == inHost {
			return ErrReentrant
		}
		panic("script: a State is used by two goroutines at once")
	}
	if s.e == nil {
		s.busy.Store(idle)
		return ErrClosed
	}
	if s.hasPending.Load() {
		s.drain()
	}
	s.hostErr = nil
	return nil
}

func (s *State) leave() {
	s.busy.Store(idle)
}

// enterLight is enter for operations that a host function may run.
// It returns whether leave must be called.
func (s *State) enterLight() (bool, error) {
	if s.busy.Load() == inHost {
		if s.e == nil {
			return false, ErrClosed
		}
		return false, nil
	}
	if err := s.enter(); err != nil {
		return false, err
	}
	return true, nil
}

// queue releases a reference at the next call, from any goroutine.
func (s *State) queue(id int32) {
	s.pendingMu.Lock()
	s.pending = append(s.pending, id)
	s.pendingMu.Unlock()
	s.hasPending.Store(true)
}

// drain releases references that were queued. Releasing runs no Lua code.
func (s *State) drain() {
	s.pendingMu.Lock()
	ids := s.pending
	s.pending = nil
	s.hasPending.Store(false)
	s.pendingMu.Unlock()
	for _, id := range ids {
		_, _ = s.e.op(opUnref, int64(id), 0)
	}
}

// Memory returns the bytes the state has allocated.
func (s *State) Memory() int64 {
	if s.e == nil {
		return 0
	}
	return s.e.hdr(hMem)
}

// SetOwner sets the owner of the code that runs from now on, from 0 to
// MaxOwners - 1. Memory that grows is charged to it, within its limit
// while a call runs. A host function may call it. Other values are owner
// 0.
func (s *State) SetOwner(owner int) {
	if s.e == nil {
		return
	}
	if owner < 0 || owner >= MaxOwners {
		owner = 0
	}
	s.e.setHdr(hOwner, int64(owner))
}

// SetOwnerLimit sets an owner's memory limit in bytes, 0 for none. A
// call that would grow the owner's memory past it stops with
// ErrMemoryLimit, as the state's limit does. The state's own limit still
// applies.
func (s *State) SetOwnerLimit(owner int, bytes int64) {
	if s.e == nil || owner < 0 || owner >= MaxOwners {
		return
	}
	s.e.setHdr(hOwnerLimit+owner, max(bytes, 0))
}

// OwnerMemory returns the bytes an owner's blocks hold. A block belongs
// to the owner that ran when it last grew.
func (s *State) OwnerMemory(owner int) int64 {
	if s.e == nil || owner < 0 || owner >= MaxOwners {
		return 0
	}
	return s.e.hdr(hOwnerMem + owner)
}

// Spent returns the CPU units charged since the state began: about one
// per Lua instruction, as the per-call budget counts them. The
// difference of two readings is what ran between them.
func (s *State) Spent() int64 {
	if s.e == nil {
		return 0
	}
	return s.e.hdr(hSpent)
}

// GC runs a full garbage collection.
func (s *State) GC() error {
	if err := s.enter(); err != nil {
		return err
	}
	defer s.leave()
	_, err := s.e.op(opGC, 0, 0)
	return err
}

// Load runs a chunk of Lua source and returns what it returns. Only source
// text is accepted, never bytecode.
func (s *State) Load(name, src string) ([]Value, error) {
	if err := s.enter(); err != nil {
		return nil, err
	}
	defer s.leave()
	if err := s.write([]Value{String(name), String(src)}, nil); err != nil {
		return nil, err
	}
	status, err := s.e.op(opLoad, 0, 0)
	if err != nil {
		return nil, err
	}
	return s.results(status)
}

// Call calls fn with args and returns its results. fn is a function, or a
// string that names a global function.
func (s *State) Call(fn Value, args ...Value) ([]Value, error) {
	if err := s.enter(); err != nil {
		return nil, err
	}
	defer s.leave()
	status, err := s.callOp(fn, args)
	if err != nil {
		return nil, err
	}
	return s.results(status)
}

// Call1 is Call for a function with one result. It does not allocate a
// result slice.
func (s *State) Call1(fn Value, args ...Value) (Value, error) {
	if err := s.enter(); err != nil {
		return Nil, err
	}
	defer s.leave()
	status, err := s.callOp(fn, args)
	if err != nil {
		return Nil, err
	}
	if err := s.statusError(status); err != nil {
		return Nil, err
	}
	if s.e.hdr(hNres) == 0 {
		return Nil, nil
	}
	pos := 0
	return s.decode(&pos, 0), nil
}

// InHost reports whether a host function runs now.
func (s *State) InHost() bool {
	return s.busy.Load() == inHost
}

// Call1Nested is Call1 for code that only computes a value, such as a
// computed bonus. Outside a host function it is Call1. Inside one, it is
// a nested call on the thread that called the host function:
//   - The CPU budget, time limit, and memory limit of the outer call apply
//     to it. When it hits one, it returns the error, and the outer call
//     stops with that error once the host function returns.
//   - It cannot pause. Call.Yield and coroutine.yield on that thread raise
//     a Lua error inside it.
//   - Handles from the host function's arguments expire after it. Keep
//     them first to use them later.
//   - At most MaxNested nested calls can be open at once.
//
// wAthena only uses it to compute stats again (decision 0010). Every other
// call from a host function into its own state gets ErrReentrant.
func (s *State) Call1Nested(fn Value, args ...Value) (Value, error) {
	if s.busy.Load() != inHost {
		return s.Call1(fn, args...)
	}
	if s.e == nil {
		return Nil, ErrClosed
	}
	if s.nested >= MaxNested {
		return Nil, errors.New("script: too many nested calls")
	}
	s.nested++
	prevErr := s.hostErr
	s.hostErr = nil
	s.busy.Store(running)
	defer func() {
		s.busy.Store(inHost)
		s.hostErr = prevErr
		s.nested--
	}()
	if err := s.write([]Value{fn}, args); err != nil {
		return Nil, err
	}
	status, err := s.e.op(opNested, int64(len(args)), 0)
	if err != nil {
		return Nil, err
	}
	if err := s.statusError(status); err != nil {
		return Nil, err
	}
	if s.e.hdr(hNres) == 0 {
		return Nil, nil
	}
	pos := 0
	return s.decode(&pos, 0), nil
}

func (s *State) callOp(fn Value, args []Value) (int32, error) {
	if err := s.write([]Value{fn}, args); err != nil {
		return 0, err
	}
	return s.e.op(opCall, int64(len(args)), 0)
}

// Global returns the value of a global variable.
func (s *State) Global(name string) (Value, error) {
	if err := s.enter(); err != nil {
		return Nil, err
	}
	defer s.leave()
	if err := s.write([]Value{String(name)}, nil); err != nil {
		return Nil, err
	}
	status, err := s.e.op(opGetGlobal, 0, 0)
	if err != nil {
		return Nil, err
	}
	res, err := s.results(status)
	if err != nil || len(res) == 0 {
		return Nil, err
	}
	return res[0], nil
}

// SetGlobal sets a global variable.
func (s *State) SetGlobal(name string, v Value) error {
	leave, err := s.enterLight()
	if err != nil {
		return err
	}
	if leave {
		defer s.leave()
	}
	if err := s.write([]Value{String(name), v}, nil); err != nil {
		return err
	}
	r, err := s.e.op(opSetGlobal, 0, 0)
	if err == nil && r != 0 {
		err = errors.New("script: cannot set global " + name)
	}
	return err
}

// Ref keeps a function, thread, or userdata alive across calls.
type Ref struct {
	s       *State
	id      int32
	kind    Kind
	cleanup runtime.Cleanup
}

type pendingRef struct {
	s  *State
	id int32
}

func (s *State) newRef(id int32, kind Kind) *Ref {
	r := &Ref{s: s, id: id, kind: kind}
	r.cleanup = runtime.AddCleanup(r, func(p pendingRef) { p.s.queue(p.id) }, pendingRef{s, id})
	return r
}

// Keep returns a Ref to a handle, so it can be used after the next call.
// A Ref that is garbage collected without Release is released at a later
// call on the state.
func (s *State) Keep(v Value) (*Ref, error) {
	switch v.kind {
	case KindFunction, KindThread, KindUserdata:
	default:
		return nil, errors.New("script: only functions, threads, and userdata can be kept")
	}
	leave, err := s.enterLight()
	if err != nil {
		return nil, err
	}
	if leave {
		defer s.leave()
	}
	if err := s.write([]Value{v}, nil); err != nil {
		return nil, err
	}
	id, err := s.e.op(opKeep, 0, 0)
	if err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, errors.New("script: cannot keep value")
	}
	return s.newRef(id, v.kind), nil
}

// Value returns the kept value. It is valid until the Ref is released.
func (r *Ref) Value() Value {
	return Value{kind: r.kind, loc: locRef, ref: r}
}

// Release frees the Ref. Values from it become invalid.
func (r *Ref) Release() {
	if r.id == 0 {
		return
	}
	r.cleanup.Stop()
	id := r.id
	r.id = 0
	s := r.s
	if s.e == nil {
		return
	}
	leave, err := s.enterLight()
	if err != nil {
		s.queue(id)
		return
	}
	_, _ = s.e.op(opUnref, int64(id), 0)
	if leave {
		s.leave()
	}
}

// results converts the outcome of an operation.
func (s *State) results(status int32) ([]Value, error) {
	if err := s.statusError(status); err != nil {
		return nil, err
	}
	n := int(s.e.hdr(hNres))
	if n == 0 {
		return nil, nil
	}
	out := make([]Value, n)
	pos := 0
	for i := range out {
		out[i] = s.decode(&pos, 0)
	}
	return out, nil
}

func (s *State) statusError(status int32) error {
	if status == statusOK || status == statusYield {
		s.gen++
		return nil
	}
	s.gen++
	e := &Error{Message: "unknown error"}
	if s.e.hdr(hNres) > 0 {
		pos := 0
		if msg, ok := s.decode(&pos, 0).AsString(); ok {
			e.Message = msg
		}
	}
	switch s.e.hdr(hFatal) {
	case 1:
		e.limit = ErrCPULimit
	case 2:
		e.limit = ErrMemoryLimit
	case 3:
		e.limit = ErrTimeLimit
	}
	switch status {
	case statusMem:
		if e.limit == nil {
			e.limit = ErrMemoryLimit
		}
	case statusSyntax:
		e.Syntax = true
	}
	if s.hostErr != nil && e.limit == nil && strings.HasSuffix(e.Message, s.hostErr.Error()) {
		e.host = s.hostErr
	}
	return e
}

// write puts head and then tail in the exchange, for the next operation.
func (s *State) write(head, tail []Value) error {
	if len(head)+len(tail) == 0 {
		s.e.setHdr(hNin, 0)
		s.e.setHdr(hNres, 0)
		return nil
	}
	var slots, bytes int
	for _, v := range head {
		if err := s.measure(v, 0, &slots, &bytes); err != nil {
			return err
		}
	}
	for _, v := range tail {
		if err := s.measure(v, 0, &slots, &bytes); err != nil {
			return err
		}
	}
	if slots > maxValues || bytes > math.MaxInt32 {
		return errors.New("script: values too large to pass to Lua")
	}
	if int64(slots) > s.e.hdr(hVcap) || int64(bytes) > s.e.hdr(hBcap) {
		r, err := s.e.op(opReserve, int64(slots), int64(bytes))
		if err != nil {
			return err
		}
		if r != 0 {
			return errors.New("script: out of memory")
		}
	}
	w := writer{s: s, vals: s.e.vals(), buf: s.e.buf(), bufAddr: s.e.hdr(hBuf)}
	for _, v := range head {
		w.put(v)
	}
	for _, v := range tail {
		w.put(v)
	}
	s.e.setHdr(hNin, int64(w.slot))
	s.e.setHdr(hNres, int64(len(head)+len(tail)))
	return nil
}

// measure counts the slots and string bytes v needs, and checks handles.
func (s *State) measure(v Value, depth int, slots, bytes *int) error {
	*slots++
	switch v.kind {
	case KindString:
		*bytes += len(v.s)
	case KindTable:
		if depth >= maxDepth {
			return errors.New("script: table too deeply nested to pass to Lua")
		}
		if v.t == nil {
			return nil
		}
		for _, f := range v.t.Fields {
			if err := s.measure(f.Key, depth+1, slots, bytes); err != nil {
				return err
			}
			if err := s.measure(f.Value, depth+1, slots, bytes); err != nil {
				return err
			}
			if *slots > maxValues {
				return errors.New("script: table too large to pass to Lua")
			}
		}
	case KindFunction, KindThread, KindUserdata:
		switch v.loc {
		case locTmp:
			if v.gen != s.gen {
				return ErrExpired
			}
		case locRef:
			if v.ref.id == 0 || v.ref.s != s {
				return ErrClosed
			}
		case locFunc:
		default:
			return errors.New("script: not a valid handle")
		}
	}
	return nil
}

type writer struct {
	s       *State
	vals    []byte
	buf     []byte
	bufAddr int64
	slot    int
	off     int
}

func putSlot(b []byte, i int64, typ int32, n int32) {
	_ = b[15]
	binary.LittleEndian.PutUint64(b, uint64(i))       // #nosec G115 -- bits are kept as they are.
	binary.LittleEndian.PutUint32(b[8:], uint32(typ)) // #nosec G115 -- small constants.
	binary.LittleEndian.PutUint32(b[12:], uint32(n))  // #nosec G115 -- bits are kept as they are.
}

// put writes v. measure has checked it, and the exchange is large enough.
func (w *writer) put(v Value) {
	b := w.vals[w.slot*slotSize:]
	w.slot++
	switch v.kind {
	case KindNil:
		putSlot(b, 0, tNil, 0)
	case KindBool:
		putSlot(b, int64(v.n), tBool, 0) // #nosec G115 -- 0 or 1.
	case KindInt:
		putSlot(b, int64(v.n), tInt, 0) // #nosec G115 -- bits are kept as they are.
	case KindFloat:
		putSlot(b, int64(v.n), tFloat, 0) // #nosec G115 -- bits are kept as they are.
	case KindString:
		copy(w.buf[w.off:], v.s)
		putSlot(b, w.bufAddr+int64(w.off), tString, int32(len(v.s))) // #nosec G115 -- write checks the size.
		w.off += len(v.s)
	case KindTable:
		if v.t == nil {
			putSlot(b, 0, tTable, 0)
			return
		}
		putSlot(b, 0, tTable, int32(len(v.t.Fields))) // #nosec G115 -- write checks the size.
		for _, f := range v.t.Fields {
			w.put(f.Key)
			w.put(f.Value)
		}
	default:
		switch v.loc {
		case locTmp:
			putSlot(b, int64(v.id), tTmp, 0)
		case locRef:
			putSlot(b, int64(v.ref.id), tRef, 0)
		case locFunc:
			putSlot(b, int64(v.id), tRef, 0)
		}
	}
}

// decode reads the value at slot *pos of the exchange, from C.
func (s *State) decode(pos *int, depth int) Value {
	return s.decodeFrom(s.e.vals(), pos, depth)
}

func (s *State) decodeFrom(vals []byte, pos *int, depth int) Value {
	b := vals[*pos*slotSize : *pos*slotSize+slotSize]
	*pos++
	i := int64(binary.LittleEndian.Uint64(b))       // #nosec G115 -- bits are kept as they are.
	typ := int32(binary.LittleEndian.Uint32(b[8:])) // #nosec G115 -- bits are kept as they are.
	n := int32(binary.LittleEndian.Uint32(b[12:]))  // #nosec G115 -- bits are kept as they are.
	switch typ {
	case tBool:
		return Bool(i != 0)
	case tInt:
		return Int(i)
	case tFloat:
		return Value{kind: KindFloat, n: uint64(i)} // #nosec G115 -- bits are kept as they are.
	case tString:
		return String(s.e.str(i, int(n)))
	case tTable:
		if depth >= maxDepth {
			return Nil
		}
		fields := make([]Field, n)
		for j := range fields {
			fields[j].Key = s.decodeFrom(vals, pos, depth+1)
			fields[j].Value = s.decodeFrom(vals, pos, depth+1)
		}
		return NewTable(fields...)
	case tTmp:
		return Value{kind: handleKind(n), loc: locTmp, id: int32(i), gen: s.gen} // #nosec G115 -- a table index.
	}
	return Nil
}

// handles maps the handle C holds to its State, without locks on lookup.
var handles handleTable

type handleTable struct {
	mu   sync.Mutex
	list atomic.Pointer[[]*State]
	free []int64
}

func (h *handleTable) add(s *State) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	var old []*State
	if p := h.list.Load(); p != nil {
		old = *p
	}
	list := append([]*State(nil), old...)
	var id int64
	if n := len(h.free); n > 0 {
		id = h.free[n-1]
		h.free = h.free[:n-1]
		list[id] = s
	} else {
		id = int64(len(list))
		list = append(list, s)
	}
	h.list.Store(&list)
	return id
}

func (h *handleTable) remove(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := append([]*State(nil), *h.list.Load()...)
	list[id] = nil
	h.list.Store(&list)
	h.free = append(h.free, id)
}

func (h *handleTable) get(id int64) *State {
	list := *h.list.Load()
	if id < 0 || id >= int64(len(list)) {
		return nil
	}
	return list[id]
}
