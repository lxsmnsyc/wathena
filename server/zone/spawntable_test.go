package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestSpawnClearCells: a stored spawn line places its monsters in its
// area, never within its clear cells, and keeps them clear when they
// come back (decision 0042).
func TestSpawnClearCells(t *testing.T) {
	spawns := []Spawn{
		{Mob: 1002, Name: "Poring", Amount: 150, RespawnMs: 1000, Avoid: []Clear{{X: 20, Y: 20, R: 15}, {X: 55, Y: 5, R: 4}}},
		{Mob: 1004, Name: "Hornet", Amount: 30, X: 30, Y: 30, W: 8, H: 8, RespawnMs: 1000, Avoid: []Clear{{X: 30, Y: 30, R: 3}}},
	}
	z := New("map/test", open(60), rng.FromKey([32]byte{7}).Stream(), spawns)
	counts := map[uint32]int{}
	for e := range z.entities.all() {
		if e.monster == nil {
			continue
		}
		counts[e.monster.spawn.Mob]++
		for _, c := range e.monster.spawn.Avoid {
			if max(abs(e.x-c.X), abs(e.y-c.Y)) <= c.R {
				t.Errorf("%s at %d,%d is within %d of %d,%d", e.monster.spawn.Name, e.x, e.y, c.R, c.X, c.Y)
			}
		}
		if s := e.monster.spawn; s.Mob == 1004 && (abs(e.x-30) > 8 || abs(e.y-30) > 8) {
			t.Errorf("a Hornet at %d,%d is outside its area", e.x, e.y)
		}
	}
	if counts[1002] != 150 || counts[1004] != 30 {
		t.Errorf("counts %v", counts)
	}
	// A line whose clear cells cover the whole map places nothing.
	z = New("map/test", open(10), rng.FromKey([32]byte{7}).Stream(), []Spawn{{Mob: 1002, Amount: 3, Avoid: []Clear{{X: 5, Y: 5, R: 10}}}})
	if z.entities.len() != 0 {
		t.Errorf("%d monsters on a cleared map", z.entities.len())
	}
}

// TestSpawnHooksSecretStream: the spawn_table hooks draw from the secret
// ChaCha8 stream of their node, not the xoshiro stream whose state is
// the node's key (review finding L36).
func TestSpawnHooksSecretStream(t *testing.T) {
	seed := [32]byte{9, 8, 7}
	got, want := spawnHooksStream(seed), rng.FromKey(seed).SecretStream()
	for i := range 4 {
		if got, w := got.Uint64(), want.Uint64(); got != w {
			t.Fatalf("draw %d: %#x, want the secret stream's %#x", i, got, w)
		}
	}
}
