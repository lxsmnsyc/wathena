package script

import (
	"errors"
	"fmt"
)

// HostFunc is a Go function that scripts call. An error it returns is
// raised as a Lua error with its message. A panic is caught and raised as
// a Lua error too, so it does not stop the process.
type HostFunc func(c *Call) error

// Call is the context of one call from Lua into a host function. It is
// valid only until the host function returns.
type Call struct {
	s     *State
	name  string
	flags int32
	args  []Value
	res   []Value
}

// errYield is returned by Call.Yield.
var errYield = errors.New("script: yield")

// argError is a bad argument, raised by the Call accessors.
type argError struct{ msg string }

// Func returns a Lua function that calls fn. The name is used in error
// messages. The function lives as long as the state.
func (s *State) Func(name string, fn HostFunc) (Value, error) {
	leave, err := s.enterLight()
	if err != nil {
		return Nil, err
	}
	if leave {
		defer s.leave()
	}
	id := len(s.funcs)
	s.funcs = append(s.funcs, hostFunc{name: name, fn: fn})
	ref, err := s.e.op(opHostFn, int64(id), 0)
	if err != nil {
		return Nil, err
	}
	if ref <= 0 {
		return Nil, errors.New("script: cannot create function " + name)
	}
	return Value{kind: KindFunction, loc: locFunc, id: ref}, nil
}

// Register sets a global function that calls fn.
func (s *State) Register(name string, fn HostFunc) error {
	f, err := s.Func(name, fn)
	if err != nil {
		return err
	}
	return s.SetGlobal(name, f)
}

// State returns the state that called the host function.
func (c *Call) State() *State { return c.s }

// Name returns the name the host function was registered with.
func (c *Call) Name() string { return c.name }

// NArgs returns the number of arguments.
func (c *Call) NArgs() int { return len(c.args) }

// Arg returns argument i, counting from 0. Missing arguments are nil.
func (c *Call) Arg(i int) Value {
	if i < 0 || i >= len(c.args) {
		return Nil
	}
	return c.args[i]
}

func (c *Call) argError(i int, want string) {
	got := "no value"
	if i < len(c.args) {
		got = c.args[i].kind.String()
	}
	panic(argError{fmt.Sprintf("bad argument #%d to '%s' (%s expected, got %s)", i+1, c.name, want, got)})
}

// Int returns argument i as an integer, or raises a Lua error. A float
// with an exact integer value is accepted.
func (c *Call) Int(i int) int64 {
	n, ok := c.Arg(i).AsInt()
	if !ok {
		c.argError(i, "integer")
	}
	return n
}

// String returns argument i as a string, or raises a Lua error.
func (c *Call) String(i int) string {
	v, ok := c.Arg(i).AsString()
	if !ok {
		c.argError(i, "string")
	}
	return v
}

// Bool returns whether argument i is true in Lua: anything but nil and false.
func (c *Call) Bool(i int) bool {
	return c.Arg(i).Truthy()
}

// Table returns argument i as a copied table, or raises a Lua error.
func (c *Call) Table(i int) *Table {
	t, ok := c.Arg(i).AsTable()
	if !ok {
		c.argError(i, "table")
	}
	return t
}

// Return adds results to the call.
func (c *Call) Return(vals ...Value) {
	c.res = append(c.res, vals...)
}

// CanYield reports whether Yield can pause the script here. It can when
// the host function runs in a thread that Go resumed, and not inside a C
// function such as table.sort or a require.
func (c *Call) CanYield() bool {
	return c.flags&canYield != 0
}

// Yield pauses the thread Go resumed. The host function returns what Yield
// returns. Thread.Resume returns vals, and the values given to the next
// Resume become the results of the host function in Lua.
func (c *Call) Yield(vals ...Value) error {
	if !c.CanYield() {
		return fmt.Errorf("%s cannot pause here: it must run in a thread that Go resumed, and not inside a C function such as table.sort", c.name)
	}
	c.res = append(c.res[:0], vals...)
	return errYield
}

// hostCall runs host function id for C. It is the only way Lua calls Go.
func hostCall(handle int64, id, nargs, flags int32) int32 {
	s := handles.get(handle)
	if s == nil {
		// The state is closing. Nothing can be written back safely.
		return hostReturn
	}
	return s.dispatch(id, nargs, flags)
}

func (s *State) dispatch(id, nargs, flags int32) (r int32) {
	s.gen++
	c := &s.call
	c.flags = flags
	c.res = c.res[:0]
	c.args = c.args[:0]
	if nargs > 0 {
		vals, pos := s.e.vals(), 0
		for range nargs {
			c.args = append(c.args, s.decodeFrom(vals, &pos, 0))
		}
	}
	if id < 0 || int(id) >= len(s.funcs) {
		return s.raise("unknown host function", nil)
	}
	f := s.funcs[id]
	c.name = f.name
	prev := s.busy.Swap(inHost)
	defer func() {
		s.busy.Store(prev)
		if p := recover(); p != nil {
			if ae, ok := p.(argError); ok {
				r = s.raise(ae.msg, nil)
				return
			}
			r = s.raise(fmt.Sprintf("host function '%s' panicked: %v", f.name, p), nil)
		}
	}()
	err := f.fn(c)
	switch {
	case err == errYield:
		if werr := s.write(c.res, nil); werr != nil {
			return s.raise(werr.Error(), nil)
		}
		return hostYield
	case err != nil:
		return s.raise(err.Error(), err)
	}
	if werr := s.write(c.res, nil); werr != nil {
		return s.raise(werr.Error(), nil)
	}
	return hostReturn
}

// raise passes msg back to C as a Lua error.
func (s *State) raise(msg string, err error) int32 {
	s.hostErr = err
	if s.write([]Value{String(msg)}, nil) != nil {
		_ = s.write([]Value{String("host function failed")}, nil)
	}
	return hostError
}

// require is host function 0. It returns a module's source, or nil.
func (s *State) require(c *Call) error {
	name := c.String(0)
	if s.modules != nil {
		if src, ok := s.modules(name); ok {
			c.Return(String(src))
			return nil
		}
	}
	c.Return(Nil)
	return nil
}
