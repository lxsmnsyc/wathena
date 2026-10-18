package script

import (
	"errors"
	"testing"
)

// TestOwnerLimits checks the memory owners: each owner's limit stops
// only its own calls, freed memory goes back to the owner that held it,
// and the counts add up to the state's memory.
func TestOwnerLimits(t *testing.T) {
	s := newState(t, Options{Limits: Limits{Memory: 64 << 20}})
	s.SetOwnerLimit(1, 8<<20)
	run(t, s, `keep = {}`)
	s.SetOwner(1)
	// Owner 1 keeps about 4 MB in a global. It fits.
	run(t, s, `for i = 1, 4 do keep[i] = string.rep("x", 1 << 20) end`)
	if m := s.OwnerMemory(1); m < 4<<20 || m > 7<<20 {
		t.Fatalf("owner 1 holds %d bytes, want about 4 MB", m)
	}
	// More goes past its limit.
	_, err := s.Load("more", `for i = 5, 12 do keep[i] = string.rep("y", 1 << 20) end`)
	if !errors.Is(err, ErrMemoryLimit) {
		t.Fatalf("err = %v, want ErrMemoryLimit", err)
	}
	// Owner 2 has no limit and is not stopped by owner 1's memory.
	s.SetOwner(2)
	run(t, s, `local t = string.rep("z", 8 << 20)`)
	// Owner 1's memory goes back when another owner frees it.
	run(t, s, `keep = nil collectgarbage("count")`)
	if err := s.GC(); err != nil {
		t.Fatal(err)
	}
	if m := s.OwnerMemory(1); m > 64<<10 {
		t.Errorf("owner 1 holds %d bytes after its tables were freed", m)
	}
	var sum int64
	for i := range MaxOwners {
		sum += s.OwnerMemory(i)
	}
	if sum != s.Memory() {
		t.Errorf("owners hold %d bytes, the state %d", sum, s.Memory())
	}
	// Out of range owners are owner 0.
	s.SetOwner(MaxOwners)
	before := s.OwnerMemory(0)
	run(t, s, `big = string.rep("w", 1 << 20)`)
	if s.OwnerMemory(0) < before+1<<20 {
		t.Error("an owner out of range was not owner 0")
	}
}

// TestSpent checks that Spent counts the CPU units of calls.
func TestSpent(t *testing.T) {
	s := newState(t, Options{})
	before := s.Spent()
	run(t, s, `local n = 0 for i = 1, 100000 do n = n + i end`)
	if d := s.Spent() - before; d < 100000 || d > 1000000 {
		t.Errorf("a loop of 100,000 spent %d units", d)
	}
}
