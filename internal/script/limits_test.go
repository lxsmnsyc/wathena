package script

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func newState(t testing.TB, opts Options) *State {
	t.Helper()
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func run(t testing.TB, s *State, src string) []Value {
	t.Helper()
	res, err := s.Load(t.Name(), src)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return res
}

func runInt(t testing.TB, s *State, src string) int64 {
	t.Helper()
	res := run(t, s, src)
	if len(res) == 0 {
		t.Fatalf("%s returned nothing", src)
	}
	n, ok := res[0].AsInt()
	if !ok {
		t.Fatalf("%s returned %v", src, res[0])
	}
	return n
}

// TestLimitsStop runs scripts that must be stopped by a limit, with the
// default limits. pcall and coroutines must not catch them, and the state
// must work afterwards.
func TestLimitsStop(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want error
	}{
		{"infinite loop", `while true do end`, ErrCPULimit},
		{"1 GB table", `local t = {} for i = 1, 1e9 do t[i] = {i} end`, ErrMemoryLimit},
		{"huge string", `return string.rep("x", 1 << 30)`, ErrMemoryLimit},
		{"string doubling", `local s = "x" while true do s = s .. s end`, ErrMemoryLimit},
		{"pcall loop", `while true do pcall(function() while true do end end) end`, ErrCPULimit},
		{"pcall catches nothing", `pcall(function() while true do end end) return 1`, ErrCPULimit},
		{"xpcall", `xpcall(function() while true do end end, function(e) return e end) return 1`, ErrCPULimit},
		{"error handler loops", `xpcall(error, function() while true do end end) return 1`, ErrCPULimit},
		{"pcall memory", `pcall(function() local t = {} for i = 1, 1e9 do t[i] = {i} end end) return 1`, ErrMemoryLimit},
		{"coroutine.resume", `coroutine.resume(coroutine.create(function() while true do end end)) return 1`, ErrCPULimit},
		{"coroutine.resume memory", `coroutine.resume(coroutine.create(function() local t = {} for i = 1, 1e9 do t[i] = {i} end end)) return 1`, ErrMemoryLimit},
		{"coroutine.wrap memory", `pcall(coroutine.wrap(function() local t = {} for i = 1, 1e9 do t[i] = {i} end end)) return 1`, ErrMemoryLimit},
		{"load in a loop", `while true do load("return 1") end`, ErrCPULimit},
		{"allocation loop", `while true do local t = {1, 2, 3, 4, 5, 6, 7, 8} end`, ErrCPULimit},
		{"repeat in C", `local s = string.rep("x", 1 << 16) while true do local r = s:rep(8) end`, ErrCPULimit},
		{"pattern backtracking", `return string.rep("a", 1e5):find(".-.-.-b")`, ErrCPULimit},
		{"balanced match", `local s = string.rep("(", 1e5) return s:find("%b()")`, ErrCPULimit},
		{"back reference", `local s = string.rep("a", 1e5) return s:find("(a*)%1b")`, ErrCPULimit},
		{"plain find", `local s = string.rep("a", 1e6) local p = string.rep("a", 5e5) .. "b" return s:find(p, 1, true)`, ErrCPULimit},
		{"gsub", `local s = string.rep("a", 1e5) return s:gsub(".-.-b", "")`, ErrCPULimit},
		{"sort in a loop", `local t = {} for i = 1, 1e5 do t[i] = i end while true do table.sort(t) end`, ErrCPULimit},
		{"unpack in a loop", `local t = {} for i = 1, 1e5 do t[i] = i end while true do local _ = select("#", table.unpack(t)) end`, ErrCPULimit},
		{"byte in a loop", `local s = string.rep("x", 1e5) while true do local _ = select("#", s:byte(1, -1)) end`, ErrCPULimit},
		{"table.move", `table.move({}, 1, math.maxinteger - 1, 2)`, ErrCPULimit},
		{"table.insert in a loop", `local t = {} for i = 1, 1e5 do t[i] = i end while true do table.insert(t, 1, 0) table.remove(t, 1) end`, ErrCPULimit},
		{"tonumber in a loop", `local s = string.rep(" ", 1e6) while true do tonumber(s) end`, ErrCPULimit},
		{"utf8.len in a loop", `local s = string.rep("x", 1e6) while true do utf8.len(s) end`, ErrCPULimit},
	}
	s := newState(t, Options{})
	f, err := s.Func("ok", func(*Call) error { return nil })
	must(t, err)
	must(t, s.SetGlobal("ok", f))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			_, err := s.Load(tc.name, tc.src)
			took := time.Since(start)
			if err == nil {
				t.Fatal("not stopped")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if took > 2*time.Second {
				t.Errorf("stopped after %s", took)
			}
			t.Logf("stopped after %s: %v", took.Round(time.Millisecond), err)
			// The state works after a limit.
			if n := runInt(t, s, `local t = {} for i = 1, 100 do t[i] = i end ok() return #t`); n != 100 {
				t.Errorf("after the limit: got %d", n)
			}
		})
	}
}

// TestMemoryReturns checks that memory a stopped call allocated is freed.
func TestMemoryReturns(t *testing.T) {
	s := newState(t, Options{})
	must(t, s.GC())
	before := s.Memory()
	_, err := s.Load("hog", `hog = {} for i = 1, 1e9 do hog[i] = {i} end`)
	if !errors.Is(err, ErrMemoryLimit) {
		t.Fatalf("got %v", err)
	}
	// The state is full: hog is still reachable, so even compiling
	// "hog = nil" fails. Go can clear it, since no limit applies between
	// calls.
	if _, err := s.Load("free", `hog = nil`); !errors.Is(err, ErrMemoryLimit) {
		t.Fatalf("got %v", err)
	}
	must(t, s.SetGlobal("hog", Nil))
	must(t, s.GC())
	if after := s.Memory(); after > before+64<<10 {
		t.Errorf("memory %d before, %d after", before, after)
	}
}

// TestCPULimitPerCall checks that each call has its own budget.
func TestCPULimitPerCall(t *testing.T) {
	s := newState(t, Options{Limits: Limits{CPU: 100_000}})
	run(t, s, `function work(n) local x = 0 for i = 1, n do x = x + i end return x end`)
	for range 100 {
		// About 4 instructions per iteration: 80,000 of the 100,000.
		if _, err := s.Call(String("work"), Int(20_000)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Call(String("work"), Int(1_000_000)); !errors.Is(err, ErrCPULimit) {
		t.Fatalf("got %v", err)
	}
}

// TestTimeLimit checks the wall-clock backstop. Comparing two long equal
// strings is one instruction that the CPU budget barely sees.
func TestTimeLimit(t *testing.T) {
	s := newState(t, Options{Limits: Limits{CPU: -1, Timeout: 50 * time.Millisecond}})
	start := time.Now()
	_, err := s.Load("cmp", `
		local a = string.rep("x", 1 << 20) .. "a"
		local b = string.rep("x", 1 << 20) .. "a"
		while a == b do end`)
	if !errors.Is(err, ErrTimeLimit) {
		t.Fatalf("got %v", err)
	}
	// The clock starts at the first hook and is read every 1,000
	// instructions, so it can overshoot by 2,000 slow instructions. Each
	// compare here takes about 0.05 ms with cgo and 1 ms in WebAssembly.
	bound := time.Second
	if Backend == "wasm" {
		bound = 10 * time.Second
	}
	if took := time.Since(start); took > bound {
		t.Errorf("stopped after %s", took)
	}
}

// TestNoLimits checks that negative limits turn limits off.
func TestNoLimits(t *testing.T) {
	s := newState(t, Options{Limits: Limits{CPU: -1, Memory: -1, Timeout: -1}})
	if n := runInt(t, s, `local x = 0 for i = 1, 3e7 do x = x + 1 end return x`); n != 3e7 {
		t.Errorf("got %d", n)
	}
	if n := runInt(t, s, `local t = {} for i = 1, 2e6 do t[i] = {i} end return #t`); n != 2e6 {
		t.Errorf("got %d", n)
	}
}

// TestLimitMessages checks that a limit error says where the script was.
func TestLimitMessages(t *testing.T) {
	s := newState(t, Options{})
	_, err := s.Load("spin", "\n\nwhile true do end")
	if err == nil || !strings.Contains(err.Error(), "at spin:3") {
		t.Errorf("got %v", err)
	}
}

// TestDeepRecursion checks that recursion in Lua, through C, and in the
// parser ends in a Lua error, not a crash. The C cases need a large enough
// C stack on the thread that runs Lua.
func TestDeepRecursion(t *testing.T) {
	s := newState(t, Options{})
	for _, tc := range []struct{ name, src, want string }{
		// Lua's stack of 1,000,000 slots is larger than 16 MB, so the
		// memory limit stops it first.
		{"lua", `local function f(n) return f(n + 1) + 1 end return f(1)`, "memory limit"},
		{"pcall", `local function f() local ok, err = pcall(f) if not ok then error(err, 0) end end f()`, "stack overflow"},
		{"metamethod", `local t = setmetatable({}, {__index = function(t, k) return t[k] end}) return t.x`, "stack overflow"},
		{"gsub", `local function f(s) return (s:gsub(".", f)) end return f("x")`, "stack overflow"},
		{"sort", `local function f(a, b) table.sort({3, 2, 1}, f) return a < b end table.sort({3, 2, 1}, f)`, "stack overflow"},
		{"coroutines", `local function f() coroutine.wrap(f)() end f()`, "stack overflow"},
		{"parser", `assert(load("return " .. string.rep("(", 1000) .. "1" .. string.rep(")", 1000)))`, "C stack overflow"},
		{"table", `local t = {} for i = 1, 1e5 do t = {t} end return t`, "too large or too deeply nested"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Load(tc.name, tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error with %q", err, tc.want)
			}
			if n := runInt(t, s, `return 1`); n != 1 {
				t.Fatal("state broken")
			}
		})
	}
}
