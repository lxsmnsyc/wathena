package script

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// fuzzLimits keep each fuzz input fast.
var fuzzLimits = Limits{CPU: 200_000, Memory: 4 << 20, Timeout: time.Second}

// fuzzState has host functions that return, fail, panic, and pause.
func fuzzState(t testing.TB) *State {
	s := newState(t, Options{Limits: fuzzLimits, Modules: func(name string) (string, bool) {
		switch name {
		case "m":
			return "return {v = 1}", true
		case "loop":
			return "return require('loop')", true
		}
		return "", false
	}})
	must(t, s.Register("echo", func(c *Call) error { c.Return(c.args...); return nil }))
	must(t, s.Register("fail", func(c *Call) error { return errors.New("failed: " + c.Arg(0).String()) }))
	must(t, s.Register("boom", func(c *Call) error { panic(c.Arg(0).String()) }))
	must(t, s.Register("pause", func(c *Call) error { return c.Yield(c.args...) }))
	must(t, s.Register("keep", func(c *Call) error {
		for _, a := range c.args {
			if a.Kind() == KindFunction {
				r, err := c.State().Keep(a)
				if err != nil {
					return err
				}
				r.Release()
			}
		}
		return nil
	}))
	return s
}

// checkAlive fails when the state no longer runs a simple script.
func checkAlive(t *testing.T, s *State) {
	t.Helper()
	res, err := s.Load("alive", "return 1 + 1")
	if err != nil || len(res) != 1 || res[0].String() != "2" {
		t.Fatalf("state broken: %v %v", res, err)
	}
}

var sourceSeeds = []string{
	`return 1`,
	`while true do end`,
	`local t = {} for i = 1, 1e9 do t[i] = {i} end`,
	`return string.rep("a", 1e5):find(".-.-.-b")`,
	`return echo(1, "x", {a = {b = 2}}, 2.5, nil, true)`,
	`fail("x")`,
	`boom("y")`,
	`return pause(1, 2)`,
	`return pcall(pause)`,
	`keep(function() end, print)`,
	`return require("m").v, require("loop")`,
	`return coroutine.wrap(function() coroutine.yield(1) end)()`,
	`setmetatable({}, {__gc = print})`,
	`return load("\27Lua")`,
	`return getmetatable("").__index`,
	`local s = string.rep("x", 1 << 20) return #s:rep(4)`,
	`xpcall(error, function() while true do end end)`,
	`local t = setmetatable({}, {__index = function(t, k) return t[k] end}) return t.x`,
	`local co = coroutine.create(function() pause() end) return coroutine.resume(co)`,
	`return table.concat({1, 2, 3}, ","), utf8.char(72, 228, 8364)`,
	`return string.format("%q %5.2f %d", "a\0b", 1.5, math.mininteger)`,
}

// FuzzSource runs random Lua source, alone and as a thread that pauses.
// It must never crash, and the state must keep working.
func FuzzSource(f *testing.F) {
	for _, s := range sourceSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		s := fuzzState(t)
		_, _ = s.Load("fuzz", src)
		checkAlive(t, s)
		res, err := s.Load("fuzz", "return function(...) "+src+"\nend")
		if err != nil || len(res) == 0 || res[0].Kind() != KindFunction {
			return
		}
		th, err := s.NewThread(res[0])
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4 && !th.Done(); i++ {
			_, _, _ = th.Resume(Int(int64(i)), String("x"))
		}
		_ = th.Close()
		checkAlive(t, s)
	})
}

// valueGen builds Go values from fuzz bytes.
type valueGen struct {
	data []byte
}

func (g *valueGen) byte() byte {
	if len(g.data) == 0 {
		return 0
	}
	b := g.data[0]
	g.data = g.data[1:]
	return b
}

func (g *valueGen) u64() uint64 {
	var buf [8]byte
	for i := range buf {
		buf[i] = g.byte()
	}
	return binary.LittleEndian.Uint64(buf[:])
}

func (g *valueGen) value(depth int) Value {
	switch g.byte() % 8 {
	case 1:
		return Bool(g.byte()%2 == 0)
	case 2:
		return Int(int64(g.u64())) // #nosec G115 -- any bits.
	case 3:
		return Float(math.Float64frombits(g.u64()))
	case 4:
		n := int(g.byte())
		if n > len(g.data) {
			n = len(g.data)
		}
		s := string(g.data[:n])
		g.data = g.data[n:]
		return String(s)
	case 5:
		if depth > 4 {
			return Nil
		}
		n := int(g.byte() % 6)
		fields := make([]Field, n)
		for i := range fields {
			fields[i] = Field{Key: g.value(depth + 1), Value: g.value(depth + 1)}
		}
		return NewTable(fields...)
	case 6:
		return String(strings.Repeat("ab", int(g.byte())*64))
	}
	return Nil
}

// FuzzHostCalls runs random sequences of API calls: values through Lua and
// back, host functions that fail, panic, or pause, threads, and refs.
func FuzzHostCalls(f *testing.F) {
	f.Add([]byte{0, 2, 1, 2, 3, 4, 5})
	f.Add([]byte{1, 4, 3, 'a', 'b', 'c', 2, 5, 3, 1, 2, 4, 1, 'k'})
	f.Add([]byte{3, 3, 3, 4, 4, 5, 6, 7, 8, 9})
	f.Add([]byte{5, 5, 1, 0, 2, 7, 7, 7, 7})
	f.Fuzz(func(t *testing.T, data []byte) {
		s := fuzzState(t)
		run(t, s, `
			function id(...) return ... end
			function count(...) return select("#", ...) end
			function talk(...) local a = {pause(...)} return pause(table.unpack(a)) end
			function make() local n = 0 return function() n = n + 1 return n end end`)
		g := &valueGen{data: data}
		var threads []*Thread
		var refs []*Ref
		for steps := 0; len(g.data) > 0 && steps < 64; steps++ {
			switch g.byte() % 9 {
			case 0, 1: // Values through Lua and back.
				var args []Value
				for range int(g.byte() % 5) {
					args = append(args, g.value(0))
				}
				res, err := s.Call(String("id"), args...)
				if err != nil {
					t.Fatalf("id: %v", err)
				}
				if len(res) != len(args) {
					t.Fatalf("id returned %d values for %d", len(res), len(args))
				}
				for i := range args {
					if args[i].Kind() != KindTable && !sameScalar(res[i], args[i]) {
						t.Fatalf("value %d: got %v, want %v", i, res[i], args[i])
					}
				}
			case 2: // Through a host function from Lua.
				src := []string{`return echo(1, "a", {x = {y = 2}})`, `return pcall(fail, {})`, `return pcall(boom, 1)`, `fail(1)`, `boom()`}[g.byte()%5]
				_, _ = s.Load("host", src)
			case 3: // Start a dialog.
				th, err := s.NewThread(String("talk"))
				if err != nil {
					// A string is not a function, so NewThread looks it up.
					t.Fatal(err)
				}
				threads = append(threads, th)
			case 4: // Resume one.
				if len(threads) > 0 {
					th := threads[int(g.byte())%len(threads)]
					_, _, err := th.Resume(g.value(0), g.value(0))
					if err != nil && !errors.Is(err, ErrThreadDone) {
						t.Fatalf("resume: %v", err)
					}
				}
			case 5: // Close one.
				if len(threads) > 0 {
					i := int(g.byte()) % len(threads)
					_ = threads[i].Close()
					threads = append(threads[:i], threads[i+1:]...)
				}
			case 6: // Keep a closure.
				v, err := s.Call1(String("make"))
				if err != nil {
					t.Fatal(err)
				}
				r, err := s.Keep(v)
				if err != nil {
					t.Fatal(err)
				}
				refs = append(refs, r)
			case 7: // Call or release a ref.
				if len(refs) > 0 {
					i := int(g.byte()) % len(refs)
					if g.byte()%2 == 0 {
						if _, err := s.Call1(refs[i].Value()); err != nil {
							t.Fatal(err)
						}
					} else {
						refs[i].Release()
						refs = append(refs[:i], refs[i+1:]...)
					}
				}
			case 8: // Globals.
				v := g.value(0)
				must(t, s.SetGlobal("g", v))
				got, err := s.Global("g")
				must(t, err)
				if v.Kind() != KindTable && !sameScalar(got, v) {
					t.Fatalf("global: got %v, want %v", got, v)
				}
			}
		}
		checkAlive(t, s)
	})
}

func sameScalar(a, b Value) bool {
	if a.Kind() != b.Kind() {
		return false
	}
	if a.Kind() == KindString {
		return a.s == b.s
	}
	return a.n == b.n
}

// FuzzExchange writes random slots into the exchange and calls C with
// them, bypassing the Go encoder. glue.c must check what it reads.
func FuzzExchange(f *testing.F) {
	f.Add(uint8(2), []byte{2, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0})
	f.Add(uint8(1), []byte{1, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 255, 255, 0, 0})
	f.Add(uint8(3), []byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 4, 0, 0, 0, 16, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0})
	f.Add(uint8(4), []byte{7, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, op uint8, raw []byte) {
		s := fuzzState(t)
		run(t, s, `function id(...) return ... end`)
		nslots := len(raw)/slotSize + 1
		if nslots > 256 {
			return
		}
		must(t, s.enter())
		defer s.leave()
		// Slot 0 names the function, and the fuzz bytes fill the rest.
		if _, err := s.e.op(opReserve, int64(nslots), 64); err != nil {
			t.Fatal(err)
		}
		buf := s.e.buf()
		copy(buf, "id")
		bufAddr := s.e.hdr(hBuf)
		vals := s.e.vals()
		putSlot(vals, bufAddr, tString, 2)
		copy(vals[slotSize:], raw[:(nslots-1)*slotSize])
		// Strings point somewhere in the buffer, or anywhere.
		for i := 1; i < nslots; i++ {
			b := vals[i*slotSize:]
			if int32(binary.LittleEndian.Uint32(b[8:])) == tString && b[0]%2 == 0 {
				binary.LittleEndian.PutUint64(b, uint64(bufAddr)+uint64(b[1]%64)) // #nosec G115 -- an address.
			}
		}
		s.e.setHdr(hNin, int64(nslots))
		switch op % 4 {
		case 0, 1:
			_, err := s.e.op(opCall, int64(op/4)%int64(nslots), 0)
			if err != nil {
				t.Fatal(err)
			}
		case 2:
			_, _ = s.e.op(opSetGlobal, 0, 0)
		case 3:
			if id, _ := s.e.op(opKeep, 0, 0); id > 0 {
				_, _ = s.e.op(opUnref, int64(id), 0)
			}
		}
		s.gen++
		s.leave()
		checkAlive(t, s)
		must(t, s.enter())
	})
}
