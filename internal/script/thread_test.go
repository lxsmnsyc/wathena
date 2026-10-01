package script

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// dialogState has mes and menu host functions that pause the script, as
// strategy section 5.2 describes.
func dialogState(t testing.TB, opts Options) *State {
	t.Helper()
	s := newState(t, opts)
	must(t, s.Register("mes", func(c *Call) error {
		return c.Yield(String("mes"), String(c.String(0)))
	}))
	must(t, s.Register("menu", func(c *Call) error {
		opts := make([]Value, 0, c.NArgs()+1)
		opts = append(opts, String("menu"))
		for i := range c.NArgs() {
			opts = append(opts, String(c.String(i)))
		}
		return c.Yield(opts...)
	}))
	return s
}

func newThread(t testing.TB, s *State, src string) *Thread {
	t.Helper()
	res := run(t, s, src)
	th, err := s.NewThread(res[0])
	must(t, err)
	return th
}

func TestThreadDialog(t *testing.T) {
	s := dialogState(t, Options{})
	th := newThread(t, s, `return function(zeny)
		mes("[Healer]")
		local pick = menu("Yes", "No")
		if pick == 1 then
			mes("Healed. You paid " .. 500 .. ".")
			return zeny - 500
		end
		return zeny
	end`)
	var log []string
	vals, done, err := th.Resume(Int(1000))
	for !done {
		must(t, err)
		var parts []string
		for _, v := range vals {
			parts = append(parts, v.String())
		}
		log = append(log, strings.Join(parts, " "))
		if k, _ := vals[0].AsString(); k == "menu" {
			vals, done, err = th.Resume(Int(1))
		} else {
			vals, done, err = th.Resume()
		}
	}
	must(t, err)
	want := []string{`"mes" "[Healer]"`, `"menu" "Yes" "No"`, `"mes" "Healed. You paid 500."`}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", log, want)
	}
	if n, _ := vals[0].AsInt(); n != 500 || !th.Done() {
		t.Errorf("got %v done %v", vals, th.Done())
	}
	if _, _, err := th.Resume(); !errors.Is(err, ErrThreadDone) {
		t.Errorf("resume after the end: %v", err)
	}
}

// TestThreadOtherCalls checks that the state serves other calls while a
// dialog is paused, and that many dialogs can be paused at once.
func TestThreadOtherCalls(t *testing.T) {
	s := dialogState(t, Options{})
	run(t, s, `count = 0
		function talk(id) mes("hello " .. id) count = count + 1 return id end
		function bump() count = count + 10 return count end`)
	talk, err := s.Global("talk")
	must(t, err)
	tr, err := s.Keep(talk)
	must(t, err)
	var threads []*Thread
	for i := range 100 {
		th, err := s.NewThread(tr.Value())
		must(t, err)
		if _, done, err := th.Resume(Int(int64(i))); done || err != nil {
			t.Fatal(done, err)
		}
		threads = append(threads, th)
	}
	if _, err := s.Call(String("bump")); err != nil {
		t.Fatal(err)
	}
	for i := len(threads) - 1; i >= 0; i-- {
		vals, done, err := threads[i].Resume()
		must(t, err)
		if n, _ := vals[0].AsInt(); !done || n != int64(i) {
			t.Fatalf("thread %d: %v %v", i, vals, done)
		}
	}
	v, err := s.Call1(String("bump"))
	must(t, err)
	if n, _ := v.AsInt(); n != 120 {
		t.Errorf("count %d, want 120", n)
	}
}

func TestThreadErrors(t *testing.T) {
	s := dialogState(t, Options{Limits: Limits{CPU: 100_000}})
	t.Run("error after a pause", func(t *testing.T) {
		th := newThread(t, s, `return function() mes("a") error("boom") end`)
		if _, done, err := th.Resume(); done || err != nil {
			t.Fatal(done, err)
		}
		_, done, err := th.Resume()
		if !done || err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("got %v %v", done, err)
		}
	})
	t.Run("CPU limit per resume", func(t *testing.T) {
		th := newThread(t, s, `return function(n)
			for _ = 1, 3 do
				local x = 0
				for i = 1, n do x = x + i end
				n = mes("step")
			end
			while true do end
		end`)
		// Each resume runs 20,000 iterations, within the budget only if the
		// budget starts again at each resume.
		vals, done, err := th.Resume(Int(20_000))
		for i := 0; i < 2; i++ {
			if done || err != nil {
				t.Fatal(vals, done, err)
			}
			vals, done, err = th.Resume(Int(20_000))
		}
		if done || err != nil {
			t.Fatal(vals, done, err)
		}
		_, done, err = th.Resume(Int(1))
		if !done || !errors.Is(err, ErrCPULimit) {
			t.Fatalf("got %v %v", done, err)
		}
	})
	t.Run("memory limit during a resume", func(t *testing.T) {
		s := dialogState(t, Options{Limits: Limits{Memory: 1 << 20}})
		th := newThread(t, s, `return function() mes("a") local t = {} for i = 1, 1e9 do t[i] = i end end`)
		if _, done, err := th.Resume(); done || err != nil {
			t.Fatal(done, err)
		}
		if _, done, err := th.Resume(); !done || !errors.Is(err, ErrMemoryLimit) {
			t.Fatalf("got %v %v", done, err)
		}
	})
	t.Run("pause outside a thread", func(t *testing.T) {
		_, err := s.Load("x", `mes("hi")`)
		if err == nil || !strings.Contains(err.Error(), "mes cannot pause here") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("pause inside table.sort", func(t *testing.T) {
		th := newThread(t, s, `return function() table.sort({3, 2, 1}, function(a, b) mes("x") return a < b end) end`)
		_, done, err := th.Resume()
		if !done || err == nil || !strings.Contains(err.Error(), "cannot pause here") {
			t.Fatalf("got %v %v", done, err)
		}
	})
	t.Run("pause inside a script coroutine", func(t *testing.T) {
		th := newThread(t, s, `return function() return coroutine.wrap(function() mes("x") end)() end`)
		_, done, err := th.Resume()
		if !done || err == nil || !strings.Contains(err.Error(), "cannot pause here") {
			t.Fatalf("got %v %v", done, err)
		}
	})
	t.Run("pause inside pcall", func(t *testing.T) {
		// Lua 5.4 can yield across pcall, so a dialog can use it.
		th := newThread(t, s, `return function() local ok, v = pcall(function() return mes("x") end) return ok, v end`)
		if _, done, err := th.Resume(); done || err != nil {
			t.Fatal(done, err)
		}
		vals, done, err := th.Resume(Int(7))
		if !done || err != nil || len(vals) != 2 || vals[1].String() != "7" {
			t.Fatalf("got %v %v %v", vals, done, err)
		}
	})
	t.Run("not a function", func(t *testing.T) {
		if _, err := s.NewThread(Int(1)); err == nil {
			t.Fatal("no error")
		}
	})
	t.Run("script yields directly", func(t *testing.T) {
		th := newThread(t, s, `return function(a) local b = coroutine.yield(a + 1) return b * 2 end`)
		vals, done, err := th.Resume(Int(1))
		if done || err != nil || vals[0].String() != "2" {
			t.Fatal(vals, done, err)
		}
		vals, done, err = th.Resume(Int(5))
		if !done || err != nil || vals[0].String() != "10" {
			t.Fatal(vals, done, err)
		}
	})
}

// TestThreadClose checks that closing a paused thread runs its
// to-be-closed variables, as when a player disconnects.
func TestThreadClose(t *testing.T) {
	s := dialogState(t, Options{})
	run(t, s, `closed = 0`)
	th := newThread(t, s, `return function()
		local guard <close> = setmetatable({}, {__close = function() closed = closed + 1 end})
		mes("waiting")
	end`)
	if _, done, err := th.Resume(); done || err != nil {
		t.Fatal(done, err)
	}
	must(t, th.Close())
	if n := runInt(t, s, `return closed`); n != 1 {
		t.Errorf("closed %d times", n)
	}
	if _, _, err := th.Resume(); !errors.Is(err, ErrThreadDone) {
		t.Errorf("resume after close: %v", err)
	}
	must(t, th.Close())
}

// TestAbandonedThreads checks that threads and refs dropped without Close
// are freed.
func TestAbandonedThreads(t *testing.T) {
	s := dialogState(t, Options{})
	run(t, s, `function talk() local big = string.rep("x", 10000) mes(big) end`)
	talk, err := s.Global("talk")
	must(t, err)
	tr, err := s.Keep(talk)
	must(t, err)
	start := func() []*Thread {
		var threads []*Thread
		for range 200 {
			th, err := s.NewThread(tr.Value())
			must(t, err)
			if _, done, err := th.Resume(); done || err != nil {
				t.Fatal(done, err)
			}
			threads = append(threads, th)
		}
		return threads
	}
	must(t, s.GC())
	before := s.Memory()
	threads := start()
	must(t, s.GC())
	paused := s.Memory()
	runtime.KeepAlive(threads)
	if paused < before+200*8000 {
		t.Fatalf("200 paused threads use %d bytes", paused-before)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		runtime.GC()
		must(t, s.GC()) // drains released refs, then collects
		must(t, s.GC())
		if s.Memory() < before+100_000 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if after := s.Memory(); after > before+100_000 {
		t.Errorf("memory %d before, %d with paused threads, %d after they were dropped", before, paused, after)
	}
}
