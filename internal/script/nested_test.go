package script

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// nestedState has a host function, recalc, that makes a nested call to
// the global function computed, as the zone does when a script starts a
// status. It returns what the call returned, or the error as a string
// when swallow is set. Otherwise it raises the error.
func nestedState(t testing.TB, opts Options, swallow bool) *State {
	t.Helper()
	s := newState(t, opts)
	must(t, s.Register("recalc", func(c *Call) error {
		v, err := c.State().Call1Nested(String("computed"), c.args...)
		if err != nil {
			if swallow {
				c.Return(String("error: " + err.Error()))
				return nil
			}
			return err
		}
		c.Return(v)
		return nil
	}))
	return s
}

// TestNestedCall checks that a host function can call Lua on its own state
// for a computed value, and that the outer call goes on after it.
func TestNestedCall(t *testing.T) {
	s := nestedState(t, Options{}, false)
	must(t, s.Register("double", func(c *Call) error { c.Return(Int(c.Int(0) * 2)); return nil }))
	run(t, s, `
		stat = 10
		function computed(add) return double(stat) + (add or 0) end`)
	t.Run("value", func(t *testing.T) {
		res := run(t, s, `local before = recalc(1) stat = 20 return before, recalc(1), stat`)
		if n, _ := res[0].AsInt(); n != 21 {
			t.Errorf("before %v", res[0])
		}
		if n, _ := res[1].AsInt(); n != 41 {
			t.Errorf("after %v", res[1])
		}
		if n, _ := res[2].AsInt(); n != 20 {
			t.Errorf("outer locals %v", res[2])
		}
	})
	t.Run("many in one host call", func(t *testing.T) {
		must(t, s.Register("sum", func(c *Call) error {
			var total int64
			for i := range 50 {
				v, err := c.State().Call1Nested(String("computed"), Int(int64(i)))
				if err != nil {
					return err
				}
				n, _ := v.AsInt()
				total += n
			}
			c.Return(Int(total))
			return nil
		}))
		run(t, s, `stat = 1`)
		if n := runInt(t, s, `return sum()`); n != 50*2+49*50/2 {
			t.Errorf("sum %d", n)
		}
	})
	t.Run("inside a thread", func(t *testing.T) {
		th := newThread(t, s, `return function() stat = 3 local v = recalc() local w = coroutine.yield(v) return w + recalc() end`)
		vals, done, err := th.Resume()
		must(t, err)
		if done || len(vals) != 1 || vals[0].String() != "6" {
			t.Fatalf("first resume: %v %v", vals, done)
		}
		vals, done, err = th.Resume(Int(100))
		must(t, err)
		if !done || vals[0].String() != "106" {
			t.Fatalf("second resume: %v %v", vals, done)
		}
	})
	t.Run("error", func(t *testing.T) {
		run(t, s, `function computed() error("no stat") end`)
		_, err := s.Load("e", `recalc()`)
		if err == nil || !strings.Contains(err.Error(), "no stat") {
			t.Fatalf("got %v", err)
		}
		res := run(t, s, `local ok, err = pcall(recalc) return ok, err, 5`)
		if res[0].Truthy() || !strings.Contains(res[1].String(), "no stat") {
			t.Errorf("pcall: %v", res)
		}
	})
	t.Run("outside a host function", func(t *testing.T) {
		run(t, s, `function computed(x) return x end`)
		v, err := s.Call1Nested(String("computed"), Int(7))
		if err != nil || v.String() != "7" {
			t.Errorf("got %v %v", v, err)
		}
	})
}

// TestNestedCallKeepsReentrancy checks that only Call1Nested enters a
// state from its host function.
func TestNestedCallKeepsReentrancy(t *testing.T) {
	s := newState(t, Options{})
	var errs []error
	th := newThread(t, s, `return function() end`)
	must(t, s.Register("again", func(c *Call) error {
		_, err := c.State().Call(String("tostring"), Int(1))
		errs = append(errs, err)
		_, err = c.State().Call1(String("tostring"), Int(1))
		errs = append(errs, err)
		_, err = c.State().Load("x", "return 1")
		errs = append(errs, err)
		_, _, err = th.Resume()
		errs = append(errs, err)
		_, err = c.State().NewThread(String("tostring"))
		errs = append(errs, err)
		_, err = c.State().Global("x")
		errs = append(errs, err)
		errs = append(errs, c.State().GC())
		return nil
	}))
	run(t, s, `again()`)
	for i, err := range errs {
		if !errors.Is(err, ErrReentrant) {
			t.Errorf("call %d: got %v", i, err)
		}
	}
}

// TestNestedCallCannotYield checks that a nested call raises an error,
// and does not hang, when it tries to pause the outer thread.
func TestNestedCallCannotYield(t *testing.T) {
	s := nestedState(t, Options{}, true)
	must(t, s.Register("pause", func(c *Call) error { return c.Yield(Int(1)) }))
	for _, src := range []string{
		`function computed() return pause() end`,
		`function computed() return coroutine.yield(1) end`,
	} {
		run(t, s, src)
		th := newThread(t, s, `return function() local r = recalc() return r, "done" end`)
		vals, done, err := th.Resume()
		if err != nil || !done || len(vals) != 2 {
			t.Fatalf("%s: %v %v %v", src, vals, done, err)
		}
		if msg, _ := vals[0].AsString(); !strings.HasPrefix(msg, "error: ") || !strings.Contains(msg, "yield") && !strings.Contains(msg, "cannot pause") {
			t.Errorf("%s: got %q", src, msg)
		}
	}
	// A coroutine that the nested call makes may still yield inside it.
	run(t, s, `function computed() local co = coroutine.wrap(function() coroutine.yield(4) end) return co() end`)
	th := newThread(t, s, `return function() return recalc() end`)
	vals, _, err := th.Resume()
	if err != nil || vals[0].String() != "4" {
		t.Errorf("own coroutine: %v %v", vals, err)
	}
}

// TestNestedCallLimits checks that a nested call is charged to the outer
// call, and that a limit it hits stops the outer call even when the host
// function ignores the error.
func TestNestedCallLimits(t *testing.T) {
	cases := []struct {
		name     string
		computed string
		want     error
	}{
		{"CPU", `function computed() while true do end end`, ErrCPULimit},
		{"CPU caught by pcall", `function computed() pcall(function() while true do end end) return 1 end`, ErrCPULimit},
		{"memory", `function computed() local t = {} for i = 1, 1e9 do t[i] = {i} end end`, ErrMemoryLimit},
		{"memory caught by pcall", `function computed() pcall(function() local t = {} for i = 1, 1e9 do t[i] = {i} end end) return 1 end`, ErrMemoryLimit},
	}
	for _, swallow := range []bool{false, true} {
		for _, tc := range cases {
			name := tc.name
			if swallow {
				name += ", error ignored"
			}
			t.Run(name, func(t *testing.T) {
				s := nestedState(t, Options{Limits: Limits{CPU: 2_000_000, Memory: 8 << 20}}, swallow)
				run(t, s, tc.computed)
				start := time.Now()
				_, err := s.Load("outer", `local ok = pcall(recalc) local x = 0 for i = 1, 100 do x = x + i end return x`)
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v, want %v", err, tc.want)
				}
				if took := time.Since(start); took > 2*time.Second {
					t.Errorf("stopped after %s", took)
				}
				// The state works after the limit.
				if n := runInt(t, s, `local t = {} for i = 1, 100 do t[i] = i end return #t`); n != 100 {
					t.Errorf("after the limit: %d", n)
				}
			})
		}
	}
	t.Run("budget is shared", func(t *testing.T) {
		// Each half of the work fits the budget alone, but not both.
		s := nestedState(t, Options{Limits: Limits{CPU: 1_000_000}}, false)
		run(t, s, `function computed() local x = 0 for i = 1, 200000 do x = x + i end return x end`)
		if _, err := s.Load("one", `return recalc()`); err != nil {
			t.Fatalf("one nested call: %v", err)
		}
		if _, err := s.Load("work", `local x = 0 for i = 1, 200000 do x = x + i end return x`); err != nil {
			t.Fatalf("the outer work alone: %v", err)
		}
		_, err := s.Load("both", `local x = 0 for i = 1, 200000 do x = x + i end return recalc() + recalc() + x`)
		if !errors.Is(err, ErrCPULimit) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("thread", func(t *testing.T) {
		s := nestedState(t, Options{Limits: Limits{CPU: 1_000_000}}, true)
		run(t, s, `function computed() while true do end end`)
		th := newThread(t, s, `return function() recalc() return 1 end`)
		_, done, err := th.Resume()
		if !done || !errors.Is(err, ErrCPULimit) {
			t.Fatalf("got %v, done %v", err, done)
		}
	})
	t.Run("time", func(t *testing.T) {
		s := nestedState(t, Options{Limits: Limits{CPU: -1, Timeout: 50 * time.Millisecond}}, true)
		run(t, s, `function computed() while true do end end`)
		_, err := s.Load("t", `recalc() return 1`)
		if !errors.Is(err, ErrTimeLimit) {
			t.Fatalf("got %v", err)
		}
	})
}

// TestNestedCallDepth checks nested calls inside nested calls, up to
// MaxNested.
func TestNestedCallDepth(t *testing.T) {
	s := nestedState(t, Options{}, true)
	run(t, s, `function computed(n) if n == 0 then return 0 end local r = recalc(n - 1) if type(r) == "string" then return r end return r + 1 end`)
	if n := runInt(t, s, `return recalc(`+strconv.Itoa(MaxNested-1)+`)`); n != MaxNested-1 {
		t.Errorf("depth %d: got %d", MaxNested, n)
	}
	res := run(t, s, `return recalc(`+strconv.Itoa(MaxNested+2)+`)`)
	if msg, _ := res[0].AsString(); !strings.Contains(msg, "too many nested calls") {
		t.Errorf("got %q", msg)
	}
}

// TestNestedCallHandles checks handles across a nested call: results of
// the nested call are valid until the next call, and the host function's
// arguments expire.
func TestNestedCallHandles(t *testing.T) {
	s := newState(t, Options{})
	run(t, s, `function computed() return function() return 9 end end`)
	var argErr error
	must(t, s.Register("check", func(c *Call) error {
		fn, err := c.State().Call1Nested(String("computed"))
		if err != nil {
			return err
		}
		_, argErr = c.State().Keep(c.Arg(0))
		c.Return(fn)
		return nil
	}))
	res := run(t, s, `local f = check(tostring) return f()`)
	if res[0].String() != "9" {
		t.Errorf("got %v", res)
	}
	if !errors.Is(argErr, ErrExpired) {
		t.Errorf("argument after the nested call: %v", argErr)
	}
}
