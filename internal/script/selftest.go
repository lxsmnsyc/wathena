package script

import (
	"errors"
	"fmt"
	"runtime"
)

// LuaVersion is the vendored Lua release.
const LuaVersion = "Lua 5.4.9"

// selfTestSource exercises what the binding must do, with small limits.
// The recursion through pcall, gsub, and the parser reaches Lua's limit of
// 200 nested C calls, so it also checks that the C stack is large enough.
const selfTestSource = `
local function viaPcall(n) local ok, err = pcall(viaPcall, n + 1) if not ok then error(err, 0) end end
assert(not pcall(viaPcall, 1))
local function viaGsub(s) return (s:gsub(".", viaGsub)) end
assert(not pcall(viaGsub, "x"))
assert(not load("return " .. string.rep("(", 300) .. "1" .. string.rep(")", 300)))
assert(math.maxinteger + 1 == math.mininteger and 7 // -2 == -4)
assert(("%d:%s"):format(42, "x") == "42:x" and ("a,b"):find(",", 1, true) == 2)
return function(x) return x + pause(x) end
`

// SelfTest checks the runtime in this build: a state, a host function, a
// thread that pauses, the CPU and memory limits, and deep recursion in C.
// It runs on a thread other than the caller's, since threads that Go
// starts can have smaller C stacks than the main thread.
func SelfTest() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	done := make(chan error)
	go func() { done <- selfTest() }()
	return <-done
}

func selfTest() error {
	s, err := New(Options{Limits: Limits{CPU: 1_000_000, Memory: 4 << 20}})
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Register("pause", func(c *Call) error { return c.Yield(c.Arg(0)) }); err != nil {
		return err
	}
	res, err := s.Load("selftest", selfTestSource)
	if err != nil {
		return err
	}
	th, err := s.NewThread(res[0])
	if err != nil {
		return err
	}
	vals, done, err := th.Resume(Int(20))
	if err != nil || done || len(vals) != 1 || vals[0].String() != "20" {
		return fmt.Errorf("pausing a thread: %v %v %v", vals, done, err)
	}
	vals, done, err = th.Resume(Int(22))
	if err != nil || !done || len(vals) != 1 || vals[0].String() != "42" {
		return fmt.Errorf("resuming a thread: %v %v %v", vals, done, err)
	}
	if _, err := s.Load("spin", "while true do end"); !errors.Is(err, ErrCPULimit) {
		return fmt.Errorf("CPU limit: %v", err)
	}
	if _, err := s.Load("hog", "local t = {} for i = 1, 1e9 do t[i] = i end"); !errors.Is(err, ErrMemoryLimit) {
		return fmt.Errorf("memory limit: %v", err)
	}
	return nil
}
