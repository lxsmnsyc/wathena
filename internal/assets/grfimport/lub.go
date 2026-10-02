package grfimport

import (
	"encoding/binary"
	"fmt"
	"math"
)

// This file reads the data tables in the client's .lub files. They are
// compiled Lua 5.1 chunks that only build tables, such as
//
//	JobNameTable = { [pcJobTbl.JT_NOVICE] = "초보자", ... }
//
// evalLub runs only the instructions that load constants, build tables,
// and read or write globals. Any other instruction, such as a function
// call or a jump, stops evaluation with an error. No client code runs.
//
// One call is allowed. Some tables end with setmetatable(T, T), which
// only guards T against new keys, as efstids.lub does. That call does
// nothing here and returns T.

// Lua 5.1 opcodes used by table-building chunks.
const (
	opMove     = 0
	opLoadK    = 1
	opLoadBool = 2
	opLoadNil  = 3
	opGetGlob  = 5
	opGetTable = 6
	opSetGlob  = 7
	opSetTable = 9
	opNewTable = 10
	opAdd      = 12
	opSub      = 13
	opMul      = 14
	opUnm      = 18
	opConcat   = 21
	opCall     = 28
	opReturn   = 30
	opSetList  = 34
	opClosure  = 36
)

const (
	lubMaxSteps    = 20_000_000
	lubMaxStack    = 256
	lubMaxConsts   = 1 << 20
	lubMaxCode     = 1 << 22
	lubMaxEntries  = 1 << 22
	fieldsPerFlush = 50
)

// LubTable is a Lua table. Keys are float64, string, or bool.
type LubTable struct {
	M map[any]any
}

// lubFunction stands for a function defined in a chunk. It is never run.
type lubFunction struct{}

// lubSetMetatable stands for Lua's setmetatable when a chunk reads it
// and has not defined a global of that name.
type lubSetMetatable struct{}

type lubProto struct {
	code      []uint32
	consts    []any
	protos    []*lubProto
	maxStack  int
	numParams int
	upvalues  int
}

type lubReader struct {
	b   []byte
	p   int
	err error
}

func (r *lubReader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.b)-r.p {
		r.err = fmt.Errorf("%w: lub is truncated", ErrFormat)
		return nil
	}
	out := r.b[r.p : r.p+n]
	r.p += n
	return out
}

func (r *lubReader) u8() byte {
	if b := r.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (r *lubReader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (r *lubReader) count(limit int) int {
	n := r.u32()
	if int64(n) > int64(limit) || int(n) > len(r.b)-r.p {
		r.err = fmt.Errorf("%w: lub count %d", ErrFormat, n)
		return 0
	}
	return int(n)
}

func (r *lubReader) str() string {
	n := r.count(len(r.b))
	if n == 0 {
		return ""
	}
	s := r.take(n)
	if s == nil {
		return ""
	}
	return string(s[:n-1]) // strip the trailing NUL
}

func (r *lubReader) proto(depth int) *lubProto {
	if depth > 16 {
		r.err = fmt.Errorf("%w: lub functions nest too deep", ErrFormat)
		return nil
	}
	p := &lubProto{}
	r.str()   // source name
	r.take(8) // line defined, last line defined
	p.upvalues = int(r.u8())
	p.numParams = int(r.u8())
	r.u8() // vararg flag
	p.maxStack = int(r.u8())
	n := r.count(lubMaxCode)
	p.code = make([]uint32, 0, n)
	for range n {
		p.code = append(p.code, r.u32())
	}
	n = r.count(lubMaxConsts)
	for range n {
		switch r.u8() {
		case 0:
			p.consts = append(p.consts, nil)
		case 1:
			p.consts = append(p.consts, r.u8() != 0)
		case 3:
			if b := r.take(8); b != nil {
				p.consts = append(p.consts, math.Float64frombits(binary.LittleEndian.Uint64(b)))
			}
		case 4:
			p.consts = append(p.consts, r.str())
		default:
			r.err = fmt.Errorf("%w: lub constant type", ErrFormat)
			return nil
		}
		if r.err != nil {
			return nil
		}
	}
	n = r.count(1 << 16)
	for range n {
		p.protos = append(p.protos, r.proto(depth+1))
		if r.err != nil {
			return nil
		}
	}
	// Debug information: line numbers, local names, upvalue names.
	r.take(4 * r.count(lubMaxCode))
	for range r.count(1 << 20) {
		r.str()
		r.take(8)
	}
	for range r.count(1 << 16) {
		r.str()
	}
	if r.err != nil {
		return nil
	}
	return p
}

// parseLub reads a compiled Lua 5.1 chunk for a little-endian target with
// 4-byte ints and size_t and 8-byte numbers, which is what the client uses.
func parseLub(b []byte) (*lubProto, error) {
	r := &lubReader{b: b}
	hdr := r.take(12)
	if hdr == nil || string(hdr[:4]) != "\x1bLua" || hdr[4] != 0x51 || hdr[5] != 0 ||
		hdr[6] != 1 || hdr[7] != 4 || hdr[8] != 4 || hdr[9] != 4 || hdr[10] != 8 || hdr[11] != 0 {
		return nil, fmt.Errorf("%w: not a Lua 5.1 chunk for this layout", ErrFormat)
	}
	p := r.proto(0)
	if r.err != nil {
		return nil, r.err
	}
	return p, nil
}

// tableKey checks that a value can be a table key.
func tableKey(v any) (any, bool) {
	switch k := v.(type) {
	case float64:
		if math.IsNaN(k) {
			return nil, false
		}
		return k, true
	case string, bool:
		return k, true
	}
	return nil, false
}

// evalLub runs a chunk's main function against globals, adding to them.
func evalLub(b []byte, globals map[string]any) error {
	p, err := parseLub(b)
	if err != nil {
		return err
	}
	if p.maxStack > lubMaxStack {
		return fmt.Errorf("%w: lub stack size %d", ErrFormat, p.maxStack)
	}
	reg := make([]any, lubMaxStack)
	// from records which global a register was read from, for errors.
	from := make([]string, lubMaxStack)
	entries := 0
	rk := func(x uint32) (any, error) {
		if x&0x100 != 0 {
			i := int(x & 0xff)
			if i >= len(p.consts) {
				return nil, fmt.Errorf("%w: lub constant %d", ErrFormat, i)
			}
			return p.consts[i], nil
		}
		return reg[x], nil
	}
	konst := func(i uint32) (any, error) {
		if int(i) >= len(p.consts) {
			return nil, fmt.Errorf("%w: lub constant %d", ErrFormat, i)
		}
		return p.consts[i], nil
	}
	for pc, steps := 0, 0; pc < len(p.code); pc, steps = pc+1, steps+1 {
		if steps > lubMaxSteps {
			return fmt.Errorf("%w: lub runs too long", ErrFormat)
		}
		ins := p.code[pc]
		op := ins & 0x3f
		a := (ins >> 6) & 0xff
		c := (ins >> 14) & 0x1ff
		bb := (ins >> 23) & 0x1ff
		bx := ins >> 14
		switch op {
		case opMove:
			reg[a] = reg[bb&0xff]
		case opLoadK:
			v, err := konst(bx)
			if err != nil {
				return err
			}
			reg[a] = v
		case opLoadBool:
			reg[a] = bb != 0
			if c != 0 {
				pc++ // skip the next instruction, as Lua does
			}
		case opLoadNil:
			for i := a; i <= bb && i < lubMaxStack; i++ {
				reg[i] = nil
			}
		case opGetGlob:
			k, err := konst(bx)
			if err != nil {
				return err
			}
			name, _ := k.(string)
			v, ok := globals[name]
			if !ok && name == "setmetatable" {
				v = lubSetMetatable{}
			}
			reg[a] = v
			from[a] = name
		case opSetGlob:
			k, err := konst(bx)
			if err != nil {
				return err
			}
			name, _ := k.(string)
			globals[name] = reg[a]
		case opGetTable:
			t, ok := reg[bb&0xff].(*LubTable)
			if !ok {
				return fmt.Errorf("%w: lub indexes %q, which is not a loaded table", ErrFormat, from[bb&0xff])
			}
			kv, err := rk(c)
			if err != nil {
				return err
			}
			key, _ := tableKey(kv)
			reg[a] = t.M[key]
		case opSetTable:
			t, ok := reg[a].(*LubTable)
			if !ok {
				return fmt.Errorf("%w: lub sets a field on a non-table", ErrFormat)
			}
			kv, err := rk(bb)
			if err != nil {
				return err
			}
			v, err := rk(c)
			if err != nil {
				return err
			}
			key, ok := tableKey(kv)
			if !ok {
				return fmt.Errorf("%w: lub table key", ErrFormat)
			}
			if entries++; entries > lubMaxEntries {
				return fmt.Errorf("%w: lub tables are too large", ErrFormat)
			}
			t.M[key] = v
		case opNewTable:
			reg[a] = &LubTable{M: map[any]any{}}
		case opAdd, opSub, opMul:
			x, err1 := rk(bb)
			y, err2 := rk(c)
			if err1 != nil || err2 != nil {
				return fmt.Errorf("%w: lub constant", ErrFormat)
			}
			xf, ok1 := x.(float64)
			yf, ok2 := y.(float64)
			if !ok1 || !ok2 {
				return fmt.Errorf("%w: lub arithmetic on non-numbers", ErrFormat)
			}
			switch op {
			case opAdd:
				reg[a] = xf + yf
			case opSub:
				reg[a] = xf - yf
			default:
				reg[a] = xf * yf
			}
		case opUnm:
			x, ok := reg[bb&0xff].(float64)
			if !ok {
				return fmt.Errorf("%w: lub negates a non-number", ErrFormat)
			}
			reg[a] = -x
		case opConcat:
			s := ""
			for i := bb; i <= c && i < lubMaxStack; i++ {
				switch v := reg[i].(type) {
				case string:
					s += v
				case float64:
					s += fmt.Sprint(v)
				default:
					return fmt.Errorf("%w: lub concatenates a non-string", ErrFormat)
				}
			}
			reg[a] = s
		case opSetList:
			t, ok := reg[a].(*LubTable)
			if !ok {
				return fmt.Errorf("%w: lub list on a non-table", ErrFormat)
			}
			block := int(c)
			if block == 0 {
				pc++
				if pc >= len(p.code) {
					return fmt.Errorf("%w: lub list block", ErrFormat)
				}
				block = int(p.code[pc])
			}
			if bb == 0 {
				return fmt.Errorf("%w: lub open list", ErrFormat)
			}
			for i := 1; i <= int(bb) && int(a)+i < lubMaxStack; i++ {
				if entries++; entries > lubMaxEntries {
					return fmt.Errorf("%w: lub tables are too large", ErrFormat)
				}
				t.M[float64((block-1)*fieldsPerFlush+i)] = reg[int(a)+i]
			}
		case opClosure:
			// Function definitions are recorded but never run. One
			// pseudo-instruction per upvalue follows, and is skipped.
			if int(bx) >= len(p.protos) || p.protos[bx] == nil {
				return fmt.Errorf("%w: lub closure", ErrFormat)
			}
			reg[a] = lubFunction{}
			pc += p.protos[bx].upvalues
		case opCall:
			// Only setmetatable(t, mt) with a fixed number of arguments
			// and results is allowed. It does not change t here.
			if _, ok := reg[a].(lubSetMetatable); !ok {
				return fmt.Errorf("%w: lub calls %q, which is not data", ErrFormat, from[a])
			}
			if bb < 2 || c == 0 || int(a)+int(bb) > lubMaxStack || int(a)+int(c) > lubMaxStack {
				return fmt.Errorf("%w: lub setmetatable call", ErrFormat)
			}
			t := reg[a+1]
			if _, ok := t.(*LubTable); !ok {
				return fmt.Errorf("%w: lub setmetatable on a non-table", ErrFormat)
			}
			for i := uint32(0); i+1 < c; i++ {
				reg[a+i] = nil
			}
			if c >= 2 {
				reg[a] = t
			}
		case opReturn:
			return nil
		default:
			// Calls, jumps, loops, and anything else are not data.
			return fmt.Errorf("%w: lub uses opcode %d, which is not data", ErrFormat, op)
		}
	}
	return nil
}
