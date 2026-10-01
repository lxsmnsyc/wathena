package script

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestHostErrors(t *testing.T) {
	s := newState(t, Options{})
	errNoZeny := errors.New("not enough zeny")
	must(t, s.Register("pay", func(c *Call) error {
		if c.Int(0) > 100 {
			return errNoZeny
		}
		c.Return(Bool(true))
		return nil
	}))
	must(t, s.Register("crash", func(c *Call) error {
		var p *Table
		c.Return(p.Fields[0].Value) // a nil pointer, as a buggy host function would have
		return nil
	}))
	must(t, s.Register("crash_value", func(*Call) error { panic(42) }))

	t.Run("error", func(t *testing.T) {
		_, err := s.Load("shop", "\npay(500)")
		if !errors.Is(err, errNoZeny) {
			t.Fatalf("got %v, want it to wrap the host error", err)
		}
		if !strings.Contains(err.Error(), "shop:2: not enough zeny") {
			t.Errorf("message %q", err)
		}
	})
	t.Run("error caught by pcall", func(t *testing.T) {
		res := run(t, s, `local ok, err = pcall(pay, 500) return ok, err`)
		if res[0].Truthy() || !strings.Contains(res[1].String(), "not enough zeny") {
			t.Errorf("got %v", res)
		}
	})
	t.Run("bad argument", func(t *testing.T) {
		_, err := s.Load("bad", `pay("lots")`)
		if err == nil || !strings.Contains(err.Error(), "bad argument #1 to 'pay' (integer expected, got string)") {
			t.Fatalf("got %v", err)
		}
		_, err = s.Load("bad", `pay()`)
		if err == nil || !strings.Contains(err.Error(), "(integer expected, got no value)") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("float with an integer value", func(t *testing.T) {
		if res := run(t, s, `return pay(2.0)`); !res[0].Truthy() {
			t.Errorf("got %v", res)
		}
		if _, err := s.Load("bad", `pay(2.5)`); err == nil {
			t.Error("2.5 was accepted as an integer")
		}
	})
	t.Run("panic", func(t *testing.T) {
		for _, fn := range []string{"crash", "crash_value"} {
			_, err := s.Load("p", fn+"()")
			if err == nil || !strings.Contains(err.Error(), "host function '"+fn+"' panicked") {
				t.Fatalf("got %v", err)
			}
		}
		res := run(t, s, `local ok = pcall(crash) return ok, 1`)
		if res[0].Truthy() {
			t.Error("pcall(crash) succeeded")
		}
	})
	t.Run("state works after", func(t *testing.T) {
		if n := runInt(t, s, "return 1 + 1"); n != 2 {
			t.Error(n)
		}
	})
}

// TestValues checks values in both directions, including tables and
// strings with zero bytes.
func TestValues(t *testing.T) {
	s := newState(t, Options{})
	var got []Value
	must(t, s.Register("take", func(c *Call) error {
		got = append(got[:0], c.args...)
		c.Return(c.args...)
		return nil
	}))
	run(t, s, `function id(...) return ... end`)
	in := []Value{
		Nil, Bool(true), Bool(false), Int(-5), Int(math.MaxInt64), Float(1.5), Float(math.Inf(-1)),
		String(""), String("a\x00b"), String(strings.Repeat("x", 100_000)),
		NewTable(KV("name", String("Healer")), KV("x", Int(150)), Field{Key: Int(1), Value: String("one")}),
		List(Int(1), Int(2), NewTable(KV("deep", List(Bool(true))))),
	}
	res, err := s.Call(String("id"), in...)
	must(t, err)
	if len(res) != len(in) {
		t.Fatalf("%d results, want %d", len(res), len(in))
	}
	for i := range in {
		if !sameValue(res[i], in[i]) {
			t.Errorf("value %d: got %v, want %v", i, res[i], in[i])
		}
	}
	// And through a host function, from Lua.
	res2 := run(t, s, `return take(nil, 1, 2.5, "s", {a = {b = 3}})`)
	if len(got) != 5 || len(res2) != 5 {
		t.Fatalf("got %v and %v", got, res2)
	}
	tb, _ := got[4].AsTable()
	inner, _ := tb.Get("a").AsTable()
	if inner.Get("b").String() != "3" {
		t.Errorf("nested table: %v", got[4])
	}
	// Floats stay floats, and NaN keys are dropped.
	res = run(t, s, `return 3.0, math.type(3.0), {[0/0 ~= 0/0 and 1 or 2] = 1}`)
	if res[0].Kind() != KindFloat || res[1].String() != `"float"` {
		t.Errorf("got %v", res)
	}
}

func sameValue(a, b Value) bool {
	if a.Kind() != b.Kind() {
		return false
	}
	if a.Kind() != KindTable {
		if a.Kind() == KindString {
			return a.s == b.s
		}
		return a.n == b.n
	}
	ta, _ := a.AsTable()
	tb, _ := b.AsTable()
	if len(ta.Fields) != len(tb.Fields) {
		return false
	}
	for _, f := range tb.Fields {
		found := false
		for _, g := range ta.Fields {
			if sameValue(f.Key, g.Key) && sameValue(f.Value, g.Value) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TestHandles checks functions passed between Go and Lua, Keep, Release,
// and expired handles.
func TestHandles(t *testing.T) {
	s := newState(t, Options{})
	var kept *Ref
	must(t, s.Register("on_click", func(c *Call) error {
		r, err := c.State().Keep(c.Arg(0))
		if err != nil {
			return err
		}
		kept = r
		return nil
	}))
	res := run(t, s, `
		local n = 0
		on_click(function(x) n = n + x return n end)
		return {on_equip = function() return "equipped" end}`)
	tb, _ := res[0].AsTable()
	onEquip := tb.Get("on_equip")
	if onEquip.Kind() != KindFunction {
		t.Fatalf("on_equip is %v", onEquip)
	}
	// A handle from the last call can be used in the next one.
	v, err := s.Call1(onEquip)
	must(t, err)
	if v.String() != `"equipped"` {
		t.Errorf("got %v", v)
	}
	// But not after that.
	if _, err := s.Call1(onEquip); !errors.Is(err, ErrExpired) {
		t.Errorf("expired handle: %v", err)
	}
	// A kept function works across calls.
	for i := int64(1); i <= 3; i++ {
		v, err := s.Call1(kept.Value(), Int(i))
		must(t, err)
		if n, _ := v.AsInt(); n != i*(i+1)/2 {
			t.Errorf("call %d: %v", i, v)
		}
	}
	kept.Release()
	if _, err := s.Call1(kept.Value(), Int(1)); !errors.Is(err, ErrClosed) {
		t.Errorf("released ref: %v", err)
	}
	kept.Release()
	// Host functions are values too.
	f, err := s.Func("double", func(c *Call) error { c.Return(Int(2 * c.Int(0))); return nil })
	must(t, err)
	must(t, s.SetGlobal("lib", NewTable(KV("double", f))))
	if n := runInt(t, s, `return lib.double(21)`); n != 42 {
		t.Errorf("got %d", n)
	}
	// Scalars cannot be kept.
	if _, err := s.Keep(Int(1)); err == nil {
		t.Error("kept an integer")
	}
}

// TestReentrancy checks that a host function cannot call into its state.
func TestReentrancy(t *testing.T) {
	s := newState(t, Options{})
	var inner error
	must(t, s.Register("again", func(c *Call) error {
		_, inner = c.State().Load("inner", "return 1")
		return inner
	}))
	if _, err := s.Load("outer", "again()"); err == nil {
		t.Fatal("no error")
	}
	if !errors.Is(inner, ErrReentrant) {
		t.Errorf("got %v", inner)
	}
}

// TestConcurrentUsePanics checks the debug check for two goroutines.
func TestConcurrentUsePanics(t *testing.T) {
	s := newState(t, Options{})
	entered := make(chan struct{})
	release := make(chan struct{})
	must(t, s.Register("wait", func(*Call) error {
		close(entered)
		<-release
		return nil
	}))
	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = s.Load("a", "wait()")
	})
	<-entered
	// The state is in a host function: another goroutine gets
	// ErrReentrant, since the check cannot tell it from re-entry.
	if _, err := s.Load("b", "return 1"); !errors.Is(err, ErrReentrant) {
		t.Errorf("got %v", err)
	}
	close(release)
	wg.Wait()
	// While Lua runs, a second goroutine panics.
	s2 := newState(t, Options{Limits: Limits{CPU: -1}})
	started := make(chan struct{})
	must(t, s2.Register("started", func(*Call) error { close(started); return nil }))
	wg.Go(func() {
		_, _ = s2.Load("spin", "started() local x = 0 for i = 1, 3e8 do x = x + i end")
	})
	<-started
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
		wg.Wait()
	}()
	// started has returned once Lua is running again; wait for that.
	for s2.busy.Load() != running {
	}
	_, _ = s2.Load("c", "return 1")
}

// TestClosed checks calls on a closed state.
func TestClosed(t *testing.T) {
	s, err := New(Options{})
	must(t, err)
	s.Close()
	s.Close()
	if _, err := s.Load("x", "return 1"); !errors.Is(err, ErrClosed) {
		t.Errorf("got %v", err)
	}
}
