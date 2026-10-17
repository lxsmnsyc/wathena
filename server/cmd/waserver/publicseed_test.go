//go:build procedural

package main

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestPublicSeedLoot is the loot half of the public seed test (the phase
// 7 exit criteria, decision 0046); spawns' TestPublicSeed is the spawn
// half. A player who knows the world's public branch, every node a
// client derives from it, and even the stored secret branch, cannot
// predict the server's loot:
//
//   - The zones roll drops and rolled values from runRoot, a fresh secret
//     at each start, which no world node gives. Two starts differ, and
//     --seed, which needs --dev, repeats a test's draws.
//   - Drops of 2,000 kills and 2,000 rolled values from each node the
//     player can derive, used as a zone's stream, match the server's no
//     better than two independent streams do.
func TestPublicSeedLoot(t *testing.T) {
	root, err := rng.NewRoot([]byte("public-seed-world"))
	if err != nil {
		t.Fatal(err)
	}
	public := gen.PublicBranch(root)
	pk := public.Key()
	asRoot, err := rng.NewRoot(pk[:])
	if err != nil {
		t.Fatal(err)
	}
	const mapID = "gen:field:1:2"
	guesses := map[string]rng.Node{
		"the public branch":          public,
		"the public key as a root":   asRoot,
		"the field's public node":    gen.FieldNode(public, 1, 2),
		"the layout node":            layout.LayoutNode(public),
		"the world's secret branch":  spawns.SecretBranch(root),
		"the world's seed tree root": root,
	}
	a, err := runRoot("")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := runRoot("")
	if a.Key() == b.Key() {
		t.Fatal("two starts have one root")
	}
	if x, _ := runRoot("test"); x.Key() != mustRoot(t, "test").Key() {
		t.Error("--seed does not repeat the streams")
	}

	tables, prog, err := gamedata.LoadProgression(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := gamedata.LoadMonsters(reference.Renewal, tables, prog)
	if err != nil {
		t.Fatal(err)
	}
	var poring *progression.Mob
	for _, m := range monsters {
		if m.AegisName == "PORING" {
			poring = m.Reward
		}
	}
	if poring == nil || len(poring.Drops) < 3 {
		t.Fatal("no Poring with drops")
	}
	roll := &item.RollDef{Min: 1, Max: 100, Default: 50, Distribution: item.Uniform}
	// draws is a zone stream's loot: each kill's drops as a bit set, and
	// rolled values.
	draws := func(r *rng.Rand) ([]uint64, []int) {
		var kills []uint64
		var rolls []int
		killer := &progression.Killer{Player: true, Luk: 1}
		for range 2000 {
			var bits uint64
			for _, d := range tables.RollDrops(poring, 100, killer, false, r) {
				for i, x := range poring.Drops {
					if x.Item == d.Item {
						bits |= 1 << i
					}
				}
			}
			kills = append(kills, bits)
			rolls = append(rolls, roll.Random(r))
		}
		return kills, rolls
	}
	same := func(x, y []uint64, u, v []int) (int, int) {
		k, r := 0, 0
		for i := range x {
			if x[i] == y[i] {
				k++
			}
			if u[i] == v[i] {
				r++
			}
		}
		return k, r
	}
	realKills, realRolls := draws(zoneStream(a, mapID))
	otherKills, otherRolls := draws(zoneStream(b, mapID))
	chanceKills, chanceRolls := same(realKills, otherKills, realRolls, otherRolls)
	t.Logf("two starts: %d of 2,000 kills drop the same, %d of 2,000 rolls are equal", chanceKills, chanceRolls)
	for name, g := range guesses {
		for how, s := range map[string]*rng.Rand{
			"its zone stream":   zoneStream(g, mapID),
			"its secret stream": g.SecretStream(),
			"its public stream": g.Stream(),
		} {
			k, r := draws(s)
			sk, sr := same(realKills, k, realRolls, r)
			// Independent streams agree by chance; a stream that predicts
			// the server would agree every time.
			if sk > chanceKills+200 || sr > chanceRolls+40 {
				t.Errorf("%s, %s: %d of 2,000 kills drop the same and %d rolls are equal, by chance %d and %d", name, how, sk, sr, chanceKills, chanceRolls)
			}
		}
	}
}

func mustRoot(t *testing.T, s string) rng.Node {
	t.Helper()
	n, err := rng.NewRoot([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return n
}
