package script

import (
	"math"
	"strconv"
)

// Kind is the type of a Value.
type Kind uint8

// The kinds of Value. Tables are copied between Go and Lua. Functions,
// threads, and userdata are handles to the Lua value.
const (
	KindNil Kind = iota
	KindBool
	KindInt
	KindFloat
	KindString
	KindTable
	KindFunction
	KindThread
	KindUserdata
)

var kindNames = [...]string{"nil", "boolean", "integer", "float", "string", "table", "function", "thread", "userdata"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "unknown"
}

// Where a handle's Lua value is kept.
const (
	// locTmp is the temporary table. The value is valid until the next
	// call on the state, which the generation checks.
	locTmp uint8 = iota + 1
	// locRef is a Ref the caller owns.
	locRef
	// locFunc is a host function, kept for the life of the state.
	locFunc
)

// Value is a Lua value in Go. The zero Value is nil.
//
// Handles to functions, threads, and userdata that come out of a call are
// valid until the next call on the state. State.Keep turns one into a Ref
// that stays valid until it is released.
type Value struct {
	n    uint64 // integer, boolean, or float bits
	s    string
	t    *Table
	ref  *Ref
	id   int32 // temporary index or host function reference
	gen  uint32
	kind Kind
	loc  uint8
}

// Field is one key and value of a Table.
type Field struct {
	Key   Value
	Value Value
}

// Table is a copy of a Lua table. Fields are in Lua's traversal order,
// which is not stable.
type Table struct {
	Fields []Field
}

// Get returns the value at a string key, or nil.
func (t *Table) Get(key string) Value {
	for _, f := range t.Fields {
		if f.Key.kind == KindString && f.Key.s == key {
			return f.Value
		}
	}
	return Nil
}

// Index returns the value at an integer key, or nil.
func (t *Table) Index(i int64) Value {
	for _, f := range t.Fields {
		if k, ok := f.Key.AsInt(); ok && k == i {
			return f.Value
		}
	}
	return Nil
}

// Nil is the Lua nil.
var Nil = Value{}

// Bool returns a Lua boolean.
func Bool(b bool) Value {
	if b {
		return Value{kind: KindBool, n: 1}
	}
	return Value{kind: KindBool}
}

// Int returns a Lua integer.
func Int(i int64) Value {
	return Value{kind: KindInt, n: uint64(i)} // #nosec G115 -- bits are kept as they are.
}

// Float returns a Lua float.
func Float(f float64) Value {
	return Value{kind: KindFloat, n: math.Float64bits(f)}
}

// String returns a Lua string.
func String(s string) Value {
	return Value{kind: KindString, s: s}
}

// NewTable returns a Lua table with the given fields. Nil keys are skipped.
func NewTable(fields ...Field) Value {
	return Value{kind: KindTable, t: &Table{Fields: fields}}
}

// List returns a Lua table with the values at keys 1 to n.
func List(vals ...Value) Value {
	fields := make([]Field, len(vals))
	for i, v := range vals {
		fields[i] = Field{Key: Int(int64(i) + 1), Value: v}
	}
	return NewTable(fields...)
}

// KV is a Field with a string key.
func KV(key string, v Value) Field {
	return Field{Key: String(key), Value: v}
}

// Kind returns the type of v.
func (v Value) Kind() Kind { return v.kind }

// IsNil reports whether v is nil.
func (v Value) IsNil() bool { return v.kind == KindNil }

// Truthy reports whether Lua treats v as true: anything but nil and false.
func (v Value) Truthy() bool {
	return v.kind != KindNil && (v.kind != KindBool || v.n != 0)
}

// AsBool returns a boolean.
func (v Value) AsBool() (b, ok bool) {
	return v.n != 0, v.kind == KindBool
}

// AsInt returns an integer. A float with an exact integer value converts,
// as in Lua. Strings do not.
func (v Value) AsInt() (int64, bool) {
	switch v.kind {
	case KindInt:
		return int64(v.n), true // #nosec G115 -- bits are kept as they are.
	case KindFloat:
		f := math.Float64frombits(v.n)
		if f >= -9223372036854775808 && f < 9223372036854775808 && f == math.Trunc(f) {
			return int64(f), true
		}
	}
	return 0, false
}

// AsFloat returns a number as a float.
func (v Value) AsFloat() (float64, bool) {
	switch v.kind {
	case KindFloat:
		return math.Float64frombits(v.n), true
	case KindInt:
		return float64(int64(v.n)), true // #nosec G115 -- bits are kept as they are.
	}
	return 0, false
}

// AsString returns a string. Numbers do not convert.
func (v Value) AsString() (string, bool) {
	return v.s, v.kind == KindString
}

// AsTable returns a copied table.
func (v Value) AsTable() (*Table, bool) {
	return v.t, v.kind == KindTable
}

// String formats v for logs and tests.
func (v Value) String() string {
	switch v.kind {
	case KindNil:
		return "nil"
	case KindBool:
		return strconv.FormatBool(v.n != 0)
	case KindInt:
		return strconv.FormatInt(int64(v.n), 10) // #nosec G115 -- bits are kept as they are.
	case KindFloat:
		return strconv.FormatFloat(math.Float64frombits(v.n), 'g', -1, 64)
	case KindString:
		return strconv.Quote(v.s)
	case KindTable:
		return "table"
	}
	return v.kind.String()
}

// Lua type numbers from lua.h, for handles.
const (
	luaTLightUserdata = 2
	luaTFunction      = 6
	luaTUserdata      = 7
	luaTThread        = 8
)

func handleKind(luaType int32) Kind {
	switch luaType {
	case luaTFunction:
		return KindFunction
	case luaTThread:
		return KindThread
	}
	return KindUserdata
}
